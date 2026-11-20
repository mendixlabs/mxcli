---
title: Generated Microflow Geometry and Wiring
category: bug-pattern
last-synced: 40d189164
covers:
  - mdl/executor
sources:
  - .claude/skills/fix-issue/findings/mdl-executor/
  - mdl/executor/layout.go
  - mdl/executor/cmd_microflows_builder_control.go
  - mdl/backend/mfmutator/splice.go
---

> **Do not duplicate**: the measurement formulas and per-construct fixes live in
> the findings and in `mdl/executor/layout.go`. This page describes why the class
> is easy to get wrong and hard to notice.

## What this is

A microflow is a **graph with coordinates**, and mxcli generates both. Ten
executor findings are in this class, and they split into two halves that fail
very differently: geometry that is merely ugly, and wiring that is invalid.

Neither is caught by anything automatic. A too-wide loop box builds at 0 errors —
it is a human looking at Studio Pro who notices. A missing outgoing flow *is* a
build error, but only sometimes, because which flows are required depends on the
shape around them.

## How it fits

**Geometry: the measurement runs before the thing it measures.** A container's
size came from a pre-pass over the AST, executed before the body was built — so
it was a function of statement *count*, not of the activities actually placed.
Varying only the children's size changed nothing. The related arithmetic error is
treating `HorizontalSpacing` as a gap when it is a centre-to-centre *pitch*: the
builder centres each activity and advances by exactly that, so adding it on top
of each width over-measures a run of n activities by `(n-1) × ActivityWidth`.

**Do not guess the advance for a compound element.** For a run of simple
activities the span is derivable. For an `if`, a split or a nested loop it comes
out of merge geometry, and guessing *under*-sizes the box so children land
outside it — worse than a box that is too wide. Those runs fall back to the
conservative measure deliberately. The containment check is the honest test:
every child of a looped activity must lie inside the container's box.

**The measurer and the builder must agree, and a loop box says whether they
do.** Whatever follows a body — an IF's closing merge, the next row, a loop's
box — is placed from the body's *measured* width, while the box around a loop
is fitted to what was *built*. Wrapping a run in a loop and comparing the two
widths is an exact oracle that needs no expected coordinates, and it catches
the over-measures no overlap check can see. Every "already laid out" report
from `layout flows` is the builder agreeing with itself, so it proves nothing
about this class. Keep the main-line cursor apart from the drawn extent: an IF
without a merge resumes at a point that is not the edge of anything.

**Wiring: the invalid shapes come from branches that do not merge.** A decision
whose non-terminal branch falls off the end of a loop body, a `break` nested
inside an `if`, a split followed by a statement — each is a place where a
sequence flow has to be synthesised or deferred, and getting it wrong produces
either a missing outgoing flow (`CE0079`, `CE0089`) or a dangling reference that
takes `mx check` down during **load**. The valid Mendix representation of
"nothing happened, go round again" is an explicit Continue event, not an absent
flow.

**A flow that looks redundant is often load-bearing.** Removing the empty-entity
branch of a type split to stop DESCRIBE emitting a phantom `else` failed the
build with `CE0089`: `(empty)` and the base type cover different things, and
`else` cannot substitute for either. That wrong fix was implemented first and
caught only because every shape was re-run through mxbuild — not because a unit
test failed.

**The read-side twin.** Several findings here pair with a describe defect,
because the same structure has to be recovered from coordinates on the way back
out: a loop body was rendered empty when an annotation sat to its left, and a
describe → exec round trip moved a Studio Pro-authored start event. Verify a
coordinate fix against the *whole* coordinate set — dump every point before and
after and diff — or a change that pins one element while shifting another reads
as success.

**The convention is measurable; measure it.** Most geometry findings are a
side, a pitch or a placement where mxcli chose a constant and Studio Pro does
something else. Studio Pro's own documents settle it in one pass — the
`mx-test-projects` apps, ako/TestApp and PedApp together hold hundreds of
note lines, handler rows, parameter rows and CASE outcomes. A guess written
into an issue is not evidence: a proposed note numbering was refuted by notes
level with their activity, which only the activity numbering explains, and a
"~140px" estimate turned out to be a 130px median. Write the measured number
into the test literally; a test that compares against the builder's own
constant passes at any value.

**A side can carry meaning, not just drawing.** Up to three CASE outcomes keep
their WHEN-order sides because DESCRIBE reads the case order back from them;
following the geometry there reorders a CASE whose branches were placed out of
order. Before making a default "look right", find every reader of the value.

**Removing a node needs a way to keep it.** When the builder stops drawing a
node (a merge at a loop tail), a stored one survives a rebuild only if the
description states it — and the canonical describe drops what it thinks is
derivable, so the statement has to be unconditional. Graph comparisons in the
tests are merge-blind; count merges.

**A splice has its own geometry.** A fragment is laid out alone and then
translated into the gap; anything that is not on the line (a note) must not
size the gap, and anything the script placed absolutely must not be
translated.

**Making room is a move the script did not ask for.** Shifting the row after
a gap is consistent only while nothing past the gap has a stated position:
once the script says -299 and the store says 40, the next run must move it back
or refuse, so "shift everything consistently" can never re-run Unchanged. A
node at a stated point is pinned instead, and a shift that would move one is
refused with the room it needs — under mdl 1 only, because under mdl 0 a
refusal becomes a whole-flow rebuild, which loses more than the shift did.
Check a splice fix by re-running the same script, not by describing once.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the formulas,
  the CE codes and the controls
- [[unloadable-model-writes]] — where a dangling sequence flow ends up
- [[describe-round-trip-gaps]] — the read side of the same coordinates
