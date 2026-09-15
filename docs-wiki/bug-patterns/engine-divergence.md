---
title: Two Implementations, One Interface
category: bug-pattern
last-synced: 038f810e
sources:
  - .claude/skills/fix-issue/findings/mdl-backend.jsonl
  - mdl/backend/modelsdk/microflow.go
  - docs/13-decisions/0004-full-codec-engine.md
  - docs/plans/2026-09-14-retire-legacy-engine.md
---

> **Do not duplicate**: the abstraction's rationale is canonical in ADR-0002,
> ADR-0004 and ADR-0005 and framed in [[backend-abstraction]]; the retirement's
> sequence, measurements and open decisions live in
> `docs/plans/2026-09-14-retire-legacy-engine.md`; the per-field fixes live in the
> findings. This page describes what goes wrong while — and after — two
> implementations of one interface exist.

## What this is

The backend interface has had more than one implementation for most of mxcli's
life: the hand-written `sdk/mpr` engine and the codec-based `modelsdk` engine, and
beside them the MCP backend, which writes to a live Studio Pro. A large share of the
`mdl/backend` findings are one implementation doing something another does not.
The legacy engine has since been deleted, and this class did not go with it: the MCP
backend is a second implementation of the same interface, ADR-0005 anticipates a
third storage format, and the retirement itself produced a cluster of findings of
its own. Read "the two engines" as "any two backends".

A gap in one implementation is **invisible from inside it**. Everything is
self-consistent: the write stores what the read returns, the round trip is stable,
and the field that never existed is never missed.

## How it fits

### While two exist

**Two failure modes, and only one of them is honest.** A refusal costs the user a
step and tells the truth. The alternative is a placeholder: `-- Empty action` from a
describer that does not recognise an activity type, a sort clause silently absent,
an argument list quietly dropped. The placeholder is worse in every way, because
`describe → edit → exec` then deletes the construct and everything reports success.
When a reader has to give up on a type, carrying the stored `$Type` into the output
is the minimum honesty: it keeps "an action mxcli cannot read" distinguishable from
"an activity with no action".

**The worst instance was a read that under-reported access.** A restricted page
came back as "no roles" on one engine, so `SHOW ACCESS` and the security matrix both
understated who could reach it. A missing field is a cosmetic bug almost everywhere
and a security-relevant one here.

**The check that finds these is the cross-implementation matrix.** Write with A,
read with B, every combination. Every other test shape is implementation-local and
therefore blind to exactly this class.

**Read against the keys the writer builds, not against gen's accessors.** The
generated bindings and the stored document disagree in places — a notify action's
output variable was bound as `VariableName` where the model stores
`OutputVariableName` — so an accessor-based reader silently returned nothing while
the hand-built writer wrote the right key. A write-then-read test on one codec
cannot see it, because both halves agree with themselves; a Studio Pro-saved
document as the fixture can. The fix is by type against `generated/metamodel`,
never by search-and-replace: most of the gen types binding the same key are right.
A related trap: a reader's fallback path documented as "for types gen does not
model" goes on catching a type after gen starts modelling it, and under-reads with
no error. When gen is re-vendored, the typed switches need re-auditing for types
that newly have structs.

**Count the gap before fixing the instance.** Decoding every `$Type` through the
real reader turned one reported unsupported action into twenty-seven, one of which
was a shipped feature that could not be described back. Several activity types live
in **their own sub-metamodel** — `DatabaseConnector$…`, not `Microflows$…` — so a
probe or a grep under one prefix systematically under-counts what is covered.

**Fixing one field and not its sibling is the characteristic mistake.** A reader
restored for `TableMappings` and not `Parameters`, a source type ported without its
key parts. Enumerate every child of the element rather than the one the report named.

**Guard the write side with a round trip, not with `mx check`.** Several of these
produced models mxbuild accepts: a box size of zero renders every activity as a
one-pixel sliver in Studio Pro and validates at 0 errors, because `mx check` does
not look at geometry.

### When one is removed

**Deleting an implementation is not finished when the code compiles.** Three things
reference the deleted one where the compiler cannot reach them. *Runtime strings* —
an error message that names the removed fallback is worse than none, because the
user follows it into a second failure. *Test defaults* — a shared setup helper's
choice of implementation decides what a whole package covers, and one had been
pointing most of the executor's integration tests at the engine being retired, so
deleting it gained coverage rather than losing it. *Differential tests* — a test
comparing A with B has no meaning with one implementation, but the property it was a
means to usually still does; drop the comparison, keep the property.

**What a swap loses is what the old implementation did implicitly.** Two defects in
the retirement had no test that could have seen them: a create that no longer copied
minted IDs back onto the caller's element, and a listing that no longer included the
System module's built-in Java actions — elements with no stored unit, which the old
reader had synthesised in code. A synthesised element lives in one implementation's
code rather than in the data, so it is precisely what a port drops; grep the old
reader for "virtual" and `Build*` helpers before trusting one. The two detectors that
worked were outside the test suite: **a byte diff of a command's output against a
binary built from the pre-port commit** (131 bytes of 78KB), and **running an example
program** as an outside caller would, with no pre-assigned IDs.

**Suites can verify nothing, and a refactor is when it costs.** Before the port that
depended on them: a library's integration suite had only ever *skipped*, because its
fixture path was never in the repository; a package of 190 tests had not one that
called `Connect`, the method being replaced; and a test looping over a listing passed
having executed nothing, because the fixture held zero of the thing listed. A skip is
printed and an empty loop is not. Before trusting a suite to verify a swap, print the
counts it loops over, and make a missing committed fixture fatal rather than a skip.

**A reachability census answers a narrower question than it appears to.** A probe that
removes an interface method and rebuilds reports *dead* for "nothing calls this through
the interface", and that one verdict has three causes wanting opposite fixes: a caller
that wants the method but holds a concrete type (port the caller), no caller anywhere
(delete), or callers through a narrower local interface with a different signature
(delete). Only a grep for callers under any type separates them — and the census
cannot see a bypass at all when the bypassing caller uses only *implemented* methods.
The complete list of bypasses is the importer list of the concrete package.

### A live server as an implementation

**The MCP backend's contract moves with Studio Pro, independently of the model.**
Its create payloads are constructor shapes, and those changed within one Studio Pro
release — a workflow's context entity, a multi-user task's page — while the element
shapes used by updates did not. The server reports a frozen version, so the shape has
to be read off the live constructor schema rather than gated on a number, and when one
constructor in a document family changes, the others in that family deserve the same
check.

**A literal default in a mapper becomes silent loss the day the model grows.** A mapper
written when a construct was not authorable sends a fixed value for it — `NoEvent`, no
handlers — and keeps sending it after the semantic model gains the property, so the
construct is dropped with everything green. The same audit as for rewrites applies:
grep the mappers for literals whenever a construct becomes authorable. The server's own
error check can also lag a fresh write, so a clean verdict needs a second look.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  fields, sub-metamodels, round-trip guards, and the retirement's findings
- `docs/plans/2026-09-14-retire-legacy-engine.md` — the retirement as it happened,
  including the two places its own estimates were wrong
- [[backend-abstraction]] — why the seam exists
- [[describe-round-trip-gaps]] — the same read-side failure, implementation-independent
- [[rewrite-drops-unauthored-state]] — the constant-defaults audit, applied to
  rebuilds rather than mappers
- [[test-runner-cannot-fail]] — the neighbouring class where a test reports success
  for something it never evaluated
