---
title: Rewrites That Drop What They Did Not Author
category: bug-pattern
last-synced: 038f810e
sources:
  - .claude/skills/fix-issue/findings/mdl-executor.jsonl
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

**Partial statements are the honest hazard.** `create or modify entity` with a
subset of attributes drops the rest, which is arguably what "modify to this shape"
means. The remedy there was not refusal but telling the truth loudly: diff the
members, print what is being dropped, and point at the incremental spelling. Where
the statement *is* a full replace, the user asked for it; where the loss is of
something MDL cannot express at all, they did not.

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
