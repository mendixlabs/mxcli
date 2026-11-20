---
title: Rewrites That Drop What They Did Not Author
category: bug-pattern
last-synced: a20932c1
covers:
  - mdl/executor
  - mdl/backend
sources:
  - .claude/skills/fix-issue/findings/mdl-executor/
  - .claude/skills/fix-issue/findings/mdl-backend/
  - mdl/backend/modelsdk/domainmodel_alter.go
  - docs/13-decisions/0005-semantic-model-interface-currency.md
  - docs/13-decisions/0008-identity-and-idempotence.md
  - mdl/executor/validate_workflow_rewrite.go
---

> **Do not duplicate**: the guard-don't-drop decision is canonical in ADR-0005,
> identity and idempotence in ADR-0008 and CLAUDE.md, and the per-document
> recipes in the findings. This page describes the recurring failure. Counts are
> computed (`make digest-status`, or grep the findings), never quoted here.

## What this is

`CREATE OR REPLACE` and `CREATE OR MODIFY` rebuild a document from a statement
that describes only part of it. Everything the statement does not mention — a
queued call binding, a toolbox entry, translated captions, an index, a folder, an
attribute's identity, a workflow task's on-created microflow — has to be carried
across, and each property that is not is lost silently. The class is unusually
expensive because the loss is invisible at every checkpoint: the run reports
success, `mx check` reports 0 errors, and the model is valid. It is simply
smaller than it was.

## How it fits

**A model is bigger than its MDL.** Mendix documents carry state MDL has no
spelling for, and a rebuild constructs the document from what the statement says.
Anything else defaults. That is why the safe posture is **guard, don't drop**: a
rewrite that would discard a construct mxcli cannot express should refuse the
statement rather than quietly produce a smaller document.

**The loss set is every constant the rebuild writes.** A writer that constructs an
element from the statement writes *something* for every property the statement
does not mention — `NoEvent`, an empty handler list, "all participants", "does not
wait" — and that list of literals is precisely what a rewrite resets. Auditing a
guard by asking which properties "look like configuration" misses the ones that do
not: participant counts and await-all-users sat beside a guarded completion rule
for a whole feature's life, unguarded, because only the rule looked configurable.
The reliable audit is mechanical — diff a Studio Pro-saved document key by key
against what the writer emits, and every constant is a candidate.

**Where the identities already survive, carry there.** A flow rebuild — `drop` +
`create` in one script, or a `create or modify` that cannot splice — reaches
`canon.Reconcile` with the stored unit, which has already paired the elements by
`$ID`. A property with no MDL spelling is then a table entry (element `$Type`,
key, the writer's default), carried only while the written value *is* that
default. Doing it in the executor instead means re-pairing activities by hand
and wiring each rebuild path separately — the dropped path carried nothing but the
unit id and grants.

**Read the stored side raw, not through the reader.** A guard that asks the
semantic model "what did the stored document hold?" inherits the reader's blind
spots, and the reader is usually why the construct was at risk in the first place.
Walking the raw unit's `$Type` strings is also the only way to guard a construct
the model does not carry at all. The describer's generic fallback is the work list:
a `-- [Workflows$…]` comment in `describe` output is text a rewrite from that
output will delete, so each one is a guard gap until shown otherwise.

**A guard has a lifecycle.** While MDL cannot express a construct, the guard refuses
any rewrite of a document that holds one. The day the construct becomes authorable
it must change to *restated?* — compare what is stored with what the statement
declares — or it goes on refusing the very scripts the feature exists to allow.
The threshold for "differs from the default" comes from a reference document, not
from intuition: a stored consensus rule that falls back to the first outcome is
exactly what the rebuild writes, so refusing every consensus rule would have refused
every multi-user rewrite.

**Counting has to match what the builder synthesises.** Restate-or-refuse compares a
stored count with an authored count, and the builder adds elements the author never
writes — an end-of-path marker on every boundary path, an End closing every flow.
Count those on the stored side and a faithful restatement is refused; both have
happened. A substring match on `$Type` is the usual cause, because synthesised
markers are named after the thing they close.

**One refusal can hide another's gap.** A reference workflow that also held an event
sub-process refused every rewrite outright, so the guard's silence about its
notification activity went unnoticed until the sub-process became authorable and the
refusal stopped firing. Measure each construct in a document that holds only it.

**Replacing one element is a rewrite at element scope.** `ALTER … REPLACE ACTIVITY`
rebuilds the activity from the statement and resets its unstated properties exactly
as a whole-document rewrite does, while `SET ACTIVITY` edits the stored document in
place and keeps them. That pair is also the control that proves the rebuild — not
ALTER as a whole — is the cause.

**The most expensive variant is identity, not content.** Re-minting an
attribute's ID on every rewrite produced a document that looked identical and
made the runtime's database synchroniser treat every column as new — rows
survived, values gone. Nothing in the model was wrong. This is the same concern
as `GUID` preservation and the reason `canon.Reconcile` exists; a codec that
mints fresh identities on rebuild is a data-loss bug wearing a clean `mx check`.

**Fixing the rewritten element does not fix its children.** The carry that saves
an ALTER target's own identity reaches the element and its untouched siblings —
siblings pass through as stored bytes, so they were never at risk — and stops
there. Everything the rebuild constructs *inside* the target arrives fresh, and
the identity default fires on each one. So the same defect returns one level
down, and the second time it is the attributes, which is where the database's
identity actually lives.

**The carry has to be fixed per rebuild SHAPE, not per element type.** There are
two, and fixing one says nothing about the other. A rebuild that swaps one element
into an otherwise untouched list leaves every sibling passing through as stored
bytes, which is why sibling elements kept reading as safe and why the first two
rounds of this only ever touched the target. A rebuild that empties the list and
reconstructs all of it has no passthrough siblings at all, so a statement naming one
association re-minted the identity of every entity, attribute, index and association
in the module — hundreds of elements for a one-word edit. The second shape is the
more dangerous by a wide margin and looked like the same bug already fixed.

**A raw carry saves only what the converter does not set.** Carrying the stored
bytes onto a rebuild keeps every unmodelled key, and still loses every property
the converter writes as a constant — the constant is dirty and wins. The identity
carries fixed the GUID and left an export level that every rebuild set to
`Hidden`, so a whole-unit rewrite hid every exported entity in the module. On
mxcli-authored content the stored value *is* the constant, so only a fixture where
Studio Pro chose otherwise can show it. The durable fix for the whole-list shape is
not a longer carry list: an element whose semantic form equals its stored form
read back is passed through as stored, and only the named element is rebuilt.

**A round-trip harness is blind to a constant that equals every fixture value.**
The same export-level constant sat in the document-level converters too, and the
round trip passed on every document because every fixture document already held
the constant. The measurement that found it patched each subject to the
non-default value first and ran two variants: the describe output as printed, and
one edit that forces the write — an unchanged statement is elided, and an elided
write passes a converter that still writes the constant. Ten kinds failed where
the harness had reported nothing.

**The statement in the report is rarely the whole blast radius; the call site is.**
The reported symptom named one command. What actually shared the defective rebuild
were six, and the costliest of them was a `RENAME`, which nobody connects to
data loss — an entity's name is its table name, so a re-minted identity makes the
platform drop the table and create an empty one instead of renaming it, losing a
whole table where the reported command lost a column. Enumerating every caller of
the converter closes a class in one pass and finds the sites no reproduction can
reach, including ones exposed only through a library API and not through the
command language at all. Reproducing the reported statement and stopping there
finds one of six.

**A guard built on an approximate pairing promotes that pairing's error rate into
refusals.** The identity transplant matches elements structurally and its correctness
bar is deliberately low, because a wrong match only makes a diff bigger. A guard that
reads the same pairing as "these are the same element" inherits every wrong match as a
blocked write: a statement that dropped six differently-named members and added one had
the new member paired with a removed one, and the guard refused a write that corrupted
nothing. Anything consuming an approximate correspondence has to add its own test of
identity — here the member's name — and the cost of that test is a narrower guard,
which is the right trade: a backstop that refuses correct work makes documented
operations unusable, while a backstop with a hole still catches everything it did
before.

**Test only for what the approximate pairing can actually get wrong.** The first
version of that identity test also compared `$Type`. The transplant never pairs
across a `$Type`, so that half caught nothing the pairing could produce. What it did
catch was the one writer that keeps an `$ID` through a type change on purpose: the
move that converts an association to a cross-association in place. That was the
guard's only view of that data loss, and the same arm had exposed it in the first
place. A restriction that excludes no error of the input only removes coverage, and
it does so silently, because a quiet guard reads as a clean write. Ask what each
clause of the test excludes, measured against the pairing's real failure modes, and
list every hole the test leaves.

**The same error message can carry two defects, and fixing one leaves it byte-identical.**
A refusal naming one element persisted unchanged after a real fix to the carry, same
element and same values, which reads as "the fix did nothing" and is actually "there are
two". The tell is that an identical failure after a genuine change means the
reproduction exercises a path the diagnosis did not. What settled it was describing the
real stored document rather than the statement: the member the statement declared shared
no name with anything stored, so there was nothing to carry and the pairing itself was
spurious.

**A fast local reproduction and a slow realistic one find different defects.** The unit
reproduction of this ran in half a second against a fixture with the identities stripped
the way the statement strips them; the integration reproduction took half a minute. Only
the slow one could expose the second defect, because a spurious pairing needs a real
document where several members are dropped and one is added. Build the fast one to
iterate and keep running the slow one to decide.

**A sweep that reports work it then discards looks exactly like a sweep that never
ran.** A rename printed "Updated 3 reference(s) in 1 document(s)" and the build then
raised exactly three errors naming exactly those three references. That coincidence
is the diagnosis: the scanner found them and a later write to the same unit put the
old values back — here the same read-modify-write persisting a model captured
*before* the sweep. When a fix-up and a rewrite both touch one unit, the rewrite
wins, so look for the second writer rather than for a gap in the first.

**A green build can cover one arm of a fix and not another, depending on who
authored the document.** The same stale qualified name raised an error on a
platform-authored element and none on one the tool had written itself, because the
tool's version still carried a valid element pointer beside the name and the
platform resolves the pointer. So a synthetic reproduction proved half the fix and
silently skipped the rest; the other half only shows against a document the real
modeler wrote. This is the same subject-dependence as the identity case, where an
element the tool created has its two identities equal from birth and cannot detect
a re-mint at all.

**An identity derived from another identity is invisible to every same-vs-same
check.** A fresh random value makes a document differ from itself, which elision
notices and a churn test catches. A value computed from a property that is itself
held stable does not: the first write corrupts, and the second produces
byte-identical bytes, so the write is elided and the run reports *Unchanged*. The
damage is a one-shot, and every diagnostic after it — re-running the script
included — agrees that nothing is wrong. The question that separates the two is
not "does this write change anything?" but "does it change the same thing twice?",
and only the first write answers it. Compare against a copy taken before any
write, never against the previous run.

**Restored data can be the default, not the data.** Where the platform recreates
a store rather than altering it, a column, field or setting that carries a default
comes back filled with that default. Counting non-empty values therefore reports
the loss as no loss, and the reconstructed value is plausible enough that nobody
looks again — a boolean with `default true` read back as fully populated on every
row, and only a run seeded entirely with the non-default value showed it had been
overwritten. A destructive-rewrite test has to compare values that could not have
been guessed, not presence.

**A carry keyed on structure is not the same tool as a carry keyed on identity.**
The pairing behind `$ID` transplantation is deliberately tolerant, because a
wrong match there only makes a diff larger. Reusing it to carry a database
identity converts that tolerance into data loss of the opposite kind: instead of
a dropped column, a new member silently adopts a removed one's data under a name
and type that no longer describe it. Carry a database identity only on the
identity the caller itself tracked through the statement; where no such identity
is in hand, refuse the write rather than guessing, and keep the repair in the
layer that knows which element is which.

**Delete-then-create defeats every protection.** One replace path removed the
stored document before writing the new one, so nothing was left for identity
preservation or elision to reconcile against, and translated captions in every
language reset to the source language. Whatever carrying mechanism exists, a path
that deletes first opts out of it.

**Stamping every field on both paths is the usual mechanism.** Where a create and
an update share a field-application helper, the update overwrites settings the
user changed by hand with values derived from somewhere else. A field set on the
`OR MODIFY` path is also not evidence it is set on the `CREATE` path — the two
construct the element separately, so both need checking, by grepping the struct
literal rather than the field name.

**A guard at one layer is not a guard for the property.** A round-trip test that
proves the codec reads a property back says nothing about whether the caller
above it overwrites the value first. Two execution flags had exactly such a test,
green since the day their codec bug was fixed, while the executor's rebuild
struct kept stamping literals over both before the codec ever saw them. When a
property is being reset, find the **last** writer on the path, not the first one
that looks responsible.

**Which way the reset goes decides whether anything catches it.** The same two
flags were lost twice in opposite directions. The codec wrote Go's zero value,
turning *allow concurrent execution* into *disallow* — and disallow without an
error message is CE4899, so it was caught at once. The executor's literal wrote
the opposite, turning *disallow* into *allow* — and CE4899 never fires on allow,
so the reset silently switched off the one error that covers this area. A checker
that catches a property's loss in one direction is not coverage for that
property; ask what the *other* direction produces.

**Order the candidates by what makes them findable, not by severity.** A
mechanical audit produces the candidate list; it does not say which candidate
gets found before a user hits it. Two things do that, and neither is severity. A
property is findable if some gate downstream complains — the concurrency flags
became CE4899, a cleared exclusion became CE0122 — or if it is *salient* enough
that someone thinks to audit it: "apply entity access" was caught in-house purely
because it is a security setting, with every checker silent. A microflow's
deep-link URL is neither. It breaks no build, it narrows no permission, nothing
in `describe` shows it missing, and the only place it exists after the rewrite is
Studio Pro's properties pane — so it sat there until a user reported it. The
properties that are neither checked nor interesting are not this class's
low-severity tail; they are the part of it that reaches users, and they are
exactly what an audit ordered by "what could go badly wrong?" leaves for last.

**The hardcoded constant is usually wrong on create as well.** A rewrite that
loses a property typically loses it to a literal, and the literal is often not
even a legal value: a Java action's `"Public"` and a snippet's `""` are not
members of their metamodel enums, so every document mxcli *created* carried them
too. Check the literal against `generated/metamodel/enums.go` and measure what
Studio Pro writes before deciding what to carry. Where a key's version floor is
unknown, carry only the keys the stored document has — never write one it lacks.

**A fallback in DESCRIBE is a write.** Whatever describe prints is written back
in the language (or shape) the statement authors, so a readability fallback —
showing another language when the default one is empty — becomes a silent edit
on the round trip. Present-but-empty is a stored value, not a gap.

**A describe that cannot re-execute hides everything behind it.** A plain
`create` on an existing document fails loudly and nothing after it runs; fixing
the verb is what exposes the losses — including ones outside the unit, where no
BSON comparison looks: once a Java action's describe output re-executed, it
overwrote the `.java` file's code and dropped its extra-code section. Studio
Pro's own file banner states what a regeneration must retain. The same goes for
measuring: a multiset of
lost values says nothing about *which* property went — address each value by the
element that owns it before diagnosing.

**A key can be REMOVED by a version, not only introduced.** The carry rule runs
in both directions: Mendix 11.15 took the mapping's `MessageDefinition` key away
when message definitions became their own documents, so a writer that keeps
emitting it adds a key Studio Pro never writes, and a pre-11.15 document
transplanted forward carries one. Version gating is usually written as a floor;
this class needs a ceiling as often. The oracle for a storage change is
`mx convert` — converting a known project of the older version produces a
Studio Pro-authored reference for the new shape *and* for how its references
move, so nothing has to be guessed.

**A whole-model update that rebuilds some lists and passes others through is
only as correct as its list of what the semantic model carries.** The domain
model's cross-associations were raw passthrough under a comment claiming they
were not in the semantic model, long after a read converter for them existed —
so six statements (alter owner, storage, comment, delete behaviour, create-or-
modify, rename) printed success and wrote nothing. The sibling shape is an
*overlay* that merges field by field onto preserved raw bytes: a child list
nothing rebuilds is carried through from disk unchanged, so adding a workflow
group reported "3 group(s)" and stored two. In both shapes the handler's
success message is reporting its in-memory model, which is why the assertion
has to be on the **re-read document**.

**A half-set element passes every check and crashes Studio Pro's Changes
panel.** An unset gen `Part` is simply omitted by the encoder, so an argument
bound one way and not the other produced two elements of one `$Type` with
different key sets: `mx check` 0 errors, and the Changes panel throwing
`Objects with ID … do not have the same properties`. The headless reproduction
is worth knowing — `mx diff base.mpr new.mpr out.mpr` runs the same MergeLib
comparer and prints the identical line — with the base made by a *fixed* binary
so the difference is the one under test.

**List markers are stored state too, and the encoder's default is not Studio
Pro's.** Mendix writes marker 2 for several property lists where the encoder
defaults to 3, so the first rewrite of any element in such a list silently
re-markers it. It hides until that first rewrite, which makes a census the right
instrument: the stored marker per (owner `$Type`, key) across the Studio
Pro-authored fixtures, registered rather than inferred.

**A registration reaches only the lists the encoder builds.** A rewrite passes
plenty of a stored document through as bytes — an unchanged pluggable widget's
Type and Object, an existing element's clean properties — and a marker an older
build wrote there outlived the registration that fixed it, on every later
rewrite (#1640: `drop page` + the same MDL gave 2, the rewrite kept 3). The
writer now applies each owner/key registration to the whole unit on every write;
that makes a wrong registration rewrite Studio Pro's bytes, so the census is a
test, not a one-off.

**A refusal keyed on "the stored document lacks the key" conflates two different
facts.** "This project's version has no such property" and "the writer that
created this document left it out" need opposite treatment, and only the
metamodel version data separates them — a refusal that cannot tell them apart
blocks a legitimate header change on every document mxcli itself wrote.

**Partial statements are the honest hazard.** `create or modify entity` with a
subset of attributes drops the rest, which is arguably what "modify to this shape"
means. The remedy there was not refusal but telling the truth loudly: diff the
members, print what is being dropped, and point at the incremental spelling. Where
the statement *is* a full replace, the user asked for it; where the loss is of
something MDL cannot express at all, they did not.

**A dropped key can break version control even when its value was the default.**
Every loss above is a lost *value*. There is a quieter variant where nothing is
lost: an empty `Documentation`, the empty half of an attribute/association pair.
Mendix fills the default back in when it loads the document, so `mx check`, the
build and the runtime are all clean. But Mendix's merge and diff engine compares
an element's property *names* between two revisions before it compares any value,
and it throws rather than defaulting. So the first time Studio Pro saves the
document, every merge, diff and Version Control status view between the two
revisions crashes. Carrying `$ID`s forward is what turns a missing key into that
crash, because the merge then sees the same element on both sides with different
names. A brand-new element crashes the same way, one save later. The reference
for "what Studio Pro writes" is `mx convert -p`, which re-serializes exactly the
non-canonical units. Its additions are the gaps, valued with Mendix's own
defaults. Its removals are keys the version does not declare, and those turned
out to hide real behaviour bugs.

**A carry has as many sites as the statement has write paths.** A `create or
modify` of a *stored* flow is applied as an in-place splice first, and rebuilt
from scratch only when the splice cannot make the change. A carry wired into the
rebuild alone is therefore skipped in the common case. The splice's replace
rebuilds the activity through the alter-flow fragment builder, which never reaches
it. Associations to include shipped that way: the carry was tested, and the tests
passed, because the mock's stored flow had no start or end event and always fell
back to the rebuild. Before trusting a carry, list the write paths (rebuild,
splice replace, explicit `alter … replace`), and test the splice against a real
project, where the stored flow is a whole graph.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  properties and the carrying mechanism each needed
- `mdl/executor/validate_workflow_rewrite.go` — the guard that exhibits every beat
  above in one file: raw-unit reading, restate-or-refuse counts, synthesised-element
  subtraction, and the REPLACE ACTIVITY variant
- [[describe-round-trip-gaps]] — the read-side half: what describe cannot state, a
  rewrite from its output deletes
- [[element-identity]] — `$ID` versus `GUID` versus `StableId`
- [[mpr-read-write]] — where the write choke points are, and what elision assumes
