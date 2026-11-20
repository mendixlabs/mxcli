# ADR-0015: What mxcli keeps of a unit is keyed by the unit's content

- **Status**: Proposed
- **Date**: 2026-11-12
- **Related**: ako/mxcli#2160 (the defect), ako/mxcli#2159 (the change); [ADR-0008](0008-identity-and-idempotence.md) (content comparison for writes); [ADR-0014](0014-incremental-catalog-builds.md) (the same rule for catalog rows); ako/mxcli#2121 (a session sees another process's writes)

## Context

A script cost statements × project. The check before an `exec`, and the
`exec` itself, list whole document types for each statement: every microflow
to resolve one call, every domain model to resolve one entity. Each listing
read every unit file of the type and decoded every unit from its bytes.
Seeding 300 entities, 600 microflows and 600 pages into a 369-unit project
took 45 s, 42 s and 286 s per stage; 1,600 pages had not finished after 15
minutes. A user works on a project of 8,000 microflows.

One listing is cheap: 616 microflows decode in 0.11 s. The cost was the
count, about a thousand full microflow listings for 600 page statements, and
it was spread over more call sites than a profile shows at once. The fixes
before this one (#1789, #1791, #1850, #2094) each removed one call site and
left a budget test for one command. None of those tests covers many
statements in one script, and the next script shape finds the next site.

So the remedy had to be below the call sites, where every listing passes,
and it had to stay correct while the same process writes the project between
listings. That is the hard part of caching here. The reader had a byte cache
already, and every write emptied all of it, because a cache keyed by a
unit's identity has to be told about every write by every path, and the
simplest way to be sure is to throw everything away.

## Decision

What mxcli keeps that is derived from a unit — its bytes in memory, its
decoded form — is valid for the unit's **content** and for nothing else: an
entry records the content it was made from and is served only while the
unit's stored content is still that, so no write path invalidates it and
none can forget to. Three rules follow from handing the same thing to more
than one caller. A decoded element is shared only once reading it writes
nothing, which means every lazily decoded value in it has been loaded. An
element a caller has changed is never served to the next caller. And when
another process is known to have written the project, cached bytes are
dropped outright, because that writer is not obliged to move the hash the
bytes are keyed by. Separately, a check may list each document type once for
its whole run, because a check never writes; execution may not, because a
statement must see what the one before it wrote.

## Consequences

- The seed above takes 20 s, 14 s and 44 s. On the PedApp fixture an added
  page statement decodes 2 units where it decoded 240, and reads 1 unit file
  where it read 120. The check lists the domain models, microflows and
  nanoflows once per run, where it listed them four, three and two times per
  page statement. Commands that run one statement are unchanged, by design:
  a unit is only remembered the first time it is decoded and kept the
  second, so a single listing pays nothing.
- **Two keys, on purpose.** The decode cache holds the bytes an element was
  decoded from and compares them exactly. The byte cache is keyed by the
  `ContentsHash` in the unit's row of the Unit table, which the writer
  already updates with every unit it writes. The first needs nothing from
  anyone. The second trusts that hash for this process's own writes — every
  site that writes a unit file was read, and does move it — and trusts
  nothing for another process's, which is why a foreign change empties it.
  A future write path that rewrote a unit file without moving its hash would
  serve stale bytes for the rest of a script run, and no test would notice.
- **A listed element may be one another caller holds.** Before this, every
  listing returned objects of its own, and a caller could do as it liked
  with them. Now a change made through a setter is safe, because it marks
  the element dirty and the entry is decoded afresh. A change made by any
  route that does not mark it dirty — a field assignment, a slice edited in
  place, a write through `Raw()` — is seen by the next caller, and nothing
  detects it. `docs/03-development/MODELSDK_ENGINE_ARCHITECTURE.md` carries
  the rule for converter and write-path authors.
- **Each safeguard has a test that fails without it, and one of them needs
  the race detector.** Without the loading of lazy values, reading a shared
  element from several goroutines is 42 data races; `make test` does not run
  with `-race`, so that test only bites where the detector is on. Without
  the dirty check, a caller's change reaches the next caller. Without the
  foreign-write rule, a rewrite by another process that leaves the hash
  alone is not seen while a cache scope is held.
- **Within one check, the validators share what was listed.** None changes a
  listed document in place today. One that did would be seen by every
  validator after it in the run.
- **Memory is held longer.** Units of a type listed more than once stay
  decoded for the life of the connection. A script run keeps the bytes of
  every unit file it reads until it ends. A long-lived session does not keep
  bytes between statements, so that it cannot go on serving what Studio Pro
  has since rewritten.
- **`MXCLI_DECODE_CACHE=off`** turns the decode cache off for a process, to
  rule it in or out when a result looks wrong. There is no switch for the
  byte cache or for the check's listings.
- What this does not remove: the conversion of each listing into the
  semantic model, the per-statement index builds during execution, and the
  rewriting of one domain model once per statement. A script still grows
  faster than its statement count.

## Alternatives considered

- **Fix the call sites one at a time**, as the earlier fixes did: give each
  validator a by-name lookup or a narrower listing. Lowest risk per change,
  and it leaves the cost in every site nobody has profiled. The profile of
  one script showed eight sites; the fixes of the preceding weeks had each
  found one. Still the right answer where a site lists a type to find one
  document, and compatible with this decision.
- **Cache the semantic-model objects a listing returns** (`[]*Microflow`,
  `[]*DomainModel`), which would also save the conversion. They are plain
  structs that callers change freely — `create or modify` builds on what it
  reads — and they carry no dirty flag, so a shared one cannot tell that it
  was changed. Sound only inside a check, where nothing is written, which is
  where it was done.
- **Key by identity and invalidate on write**, as the reader's byte cache
  did. It needs every write path to say which unit it wrote; the paths that
  could not say emptied everything, which is how the cache came to do
  nothing during a script. It also cannot see another process's write.
- **Make lazy decoding safe to share instead of loading everything first**,
  with a lock or an atomic per property. That puts a cost on every property
  of every element, in the engine's hottest path, to save one walk of an
  element that is about to be read many times.
- **Hold decoded listings for the length of one command only**, with no
  validity rule at all. That is what a check gets from its memo, and it is
  correct there because nothing is written. During execution the listing
  changes under the statements that read it.
