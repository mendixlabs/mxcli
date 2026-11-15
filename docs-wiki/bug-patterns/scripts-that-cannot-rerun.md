---
title: Scripts That Cannot Be Re-Run
category: bug-pattern
last-synced: ced830e0
covers:
  - mdl/backend
  - mdl/executor
  - mdl/grammar
sources:
  - .claude/skills/fix-issue/findings/mdl-grammar/
  - .claude/skills/fix-issue/findings/mdl-executor/
  - .claude/skills/fix-issue/findings/mdl-backend/
  - mdl/grammar/domains/MDLDomainModel.g4
  - docs/13-decisions/0003-mdl-is-sql-shaped.md
---

> **Do not duplicate**: the `IF NOT EXISTS` / `create or modify` spellings live
> in `MDL_QUICK_REFERENCE.md`; write-level idempotence is ADR-0008 and
> [[element-identity]]. This page is about idempotence at the *statement* level,
> which is a different thing.

## What this is

mxcli's writes are idempotent — a unit whose content has not changed is not
written at all. Its *statements* are not, and the two get conflated. `create`
follows SQL convention and fails on an existing object; `alter entity … add
attribute` fails when the attribute is already there. Five `mdl/grammar` findings
are the consequence, and the compounding one is that `exec` halts on the first
error.

Put together: **a domain script that is 90% already applied applies none of the
remaining 10%.** The first `add attribute` that already exists ends the run, and
every later statement — including the new ones — is skipped. The user's reading is
usually that mxcli is broken, because re-running a script is the natural way to
converge a project on a description of it.

## How it fits

**The silent variant is worse than the error.** `alter entity … add index` did
not fail on a re-run: it reported "Added index" and appended a duplicate, and the
build then rejected the project. An error that stops the script is recoverable; a
success that corrupts is not, and the two live in the same statement family.

**Non-idempotence is a design decision, not a bug — but it has to be
discoverable.** MDL is SQL-shaped deliberately, and `CREATE TABLE` failing on an
existing table is the behaviour that shape implies. What was missing was the
signpost: the bare "already exists" error did not name `create or modify`, and
the idempotent spellings existed without being reachable from the failure.

**Two remedies, and they answer different questions.** `IF NOT EXISTS` /
`IF EXISTS` on a statement says *this one is optional*, which is what a
converging domain script wants. `exec --continue-on-error` says *attempt
everything and tell me what failed*, which is what a partially-applied script
wants — and it must still exit non-zero, or it trades a stopped run for a hidden
failure.

**Watch for the pair that cannot both be re-run.** `ADD EVENT HANDLER` errors
when the handler exists and `DROP EVENT HANDLER` errors when it does not, so
neither a plain script nor a defensive drop-then-add is re-runnable. A statement
family needs the guard on both halves or the workaround is unavailable too.

**Put the guard where the family is, not on each member.** `IF EXISTS` arrived
one statement at a time — attribute, index, enum value, user role, demo user —
each with its own AST field and handler branch, and the document-level `DROP`
(35 doctypes) still had none. It now lives once: the grammar rule on every
alternative, one flag the visitor sets, and one check in the executor's
dispatch. That is safe only because every drop handler reports a missing
target the same way (`NotFoundError`, at lookup, before mutating), which a
table test over every bare `DROP` measures. A new drop handler that says "not
found" some other way fails that test rather than silently erroring under the
guard.

**A list rule without a separator reads as a value error.** Several parse
failures in this area were reported against the *value* in the second item of a
list — `add attribute A: integer default 9, add attribute B: …` — when the list
rule simply had no comma alternative. When a parse error names something that is
plainly valid in the first position, suspect the list before the value.

**`create or modify` of a flow is re-runnable only if it matches its own
output.** A flow statement is diff-then-patch: it is compared with the stored
flow, and under `mdl 1` a difference the splice cannot make is refused rather
than rebuilt. So every way the comparison can fail to recognise *the flow the
same statement built* turns into a script that cannot be run twice (#839,
#859: 36 refused statements in one project's second pass). Comparing statements
with describe's rendering of the stored graph kept failing on spellings — a
guard clause printed as if/else, a shared merge printed as `join`/`merge`, a
wrapped row printed as crossed sections. What settled it was comparing what the
statement *builds* with what is stored (read back through the codec, element
IDs aside): the builder is deterministic, so an identical re-run matches by
construction. The property that catches the class runs **authored** scripts
twice (`TestFlowRerunProperty`); a describe → exec property cannot, because it
only ever feeds describe's own spelling back. The comparison is only as
good as the reader both sides pass through: a property the reader drops
compares equal on both sides, so an edit to it was reported Unchanged (a
`show page … with title` override). The read back therefore proves itself
lossless — written again, it must be the document first written — and falls
back to the statement diff when it is not.

**"Deterministic" holds within one release.** The match paired objects by
where they are drawn, and where the script states no `@position` the layout
pass decides that — and the layout pass improves between releases. One correct
measurer fix (#1773) moved enough unpositioned nodes that three flows a project
had re-run cleanly for weeks were refused after the upgrade, each on a
different statement-diff gap the match had been hiding (#2259). Geometry the
script leaves to the layout is not something a re-run changes, so the match
now falls back to pairing by flows with that geometry set aside. To test the
class without the old release, move the stored nodes and re-run the script.

**The statement diff still decides every flow that does change**, and there
two more ways to be unrunnable showed up (#859). A default describe omits —
`on error rollback` — has to be read as absent on the declared side too, or the
activity is re-spliced with every sibling edit; the built-graph match hides that
on an identical re-run, so the probe is a sibling edit and the splice summary.
And a refusal of a *legitimate* change is, to someone re-running a script,
indistinguishable from non-idempotence: re-annotating an activity was refused on
every run until the splice could replace an activity's notes. The header-only
upgrade (a mdl 0 description under `mdl 1;`) is where both surface on Studio
Pro content, because a backslash escape in a note is a real change under mdl 1.

The same failure has a structural cousin: an AST field that records how a
statement was **spelled**, kept for a deprecation or lint note, is compared like
a stored property unless it is named as spelling (`spellingFields`; an empty
`else` goes through `canonicalFlow` instead, because the `if` shell compare
needs `HasElse`). `commit … with events` never matched its stored bare commit
(#942). It is not cosmetic. The diff groups adjacent unmatched statements into
one run, and the loop-body refusal was asked only of a run of one. So a
spelling difference next to a loop turned a refused loop-body edit into a silent
replace of the loop, with every `$ID` in it re-minted. When you add an AST flag,
ask whether describe can print it. Refusals have to hold for any run that pairs
a stored loop with a new body, whatever else changes next to it.

**Some re-runs only reach a no-op across two statements.** `revoke all` followed
by the grant that should hold ends with the rules it started with, but each
statement is a real write of its own. The grant's write was compared with a unit
the revoke had already emptied, so it had no stored rule to carry an identity
from, and the rule was minted again on every run (#872). Making each statement
more idempotent does not help here. What fixed it was judging the run by where
it ends: the writes are held, and they are reconciled once against the state
from before the run. Read the transaction id as well as the unit bytes to see
that a run wrote nothing. Check the identity carry on a Studio Pro-authored rule,
because an mxcli-created one has whatever `$ID` the previous run minted.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the guards, the
  flags and the bug-tests
- [[mdl-as-sql]] — why `create` is non-idempotent on purpose
- [[element-identity]] — write-level idempotence, which is unrelated and is often
  what people mean when they say mxcli is idempotent
