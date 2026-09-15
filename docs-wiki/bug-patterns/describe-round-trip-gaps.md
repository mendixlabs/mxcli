---
title: DESCRIBE Round-Trip Gaps
category: bug-pattern
last-synced: 888e78cf
sources:
  - .claude/skills/fix-issue/findings/mdl-executor.jsonl
  - mdl/executor/cmd_workflows.go
  - mdl/executor/cmd_pages_describe_pluggable.go
  - mdl/executor/cmd_microflows_show_crossed.go
  - mdl/executor/cmd_microflows_normalize.go
  - mdl/microflowgraph/structure.go
  - docs/11-proposals/PROPOSAL_structured_microflow_description.md
---

> **Do not duplicate**: the per-construct fix recipes live in the findings
> (`grep -l describe .claude/skills/fix-issue/findings/*.jsonl`), the MDL syntax
> in `docs/01-project/MDL_QUICK_REFERENCE.md`, the round-trip requirement in
> CLAUDE.md's PR checklist, and the irreducible-graph design in
> [its proposal](../../docs/11-proposals/PROPOSAL_structured_microflow_description.md).
> This page describes the failure class only. Counts are computed
> (`make digest-status`, or grep the findings), never quoted here.

## What this is

`DESCRIBE` is a **second implementation of MDL**, written in the opposite
direction and validated by nothing. Every construct has a writer (MDL → BSON)
and a describer (BSON → MDL) built separately, and no mechanism forces them to
agree. It is the single largest class of defect in the executor — roughly two
findings in five for `mdl/executor` touch a describe path.

The reason it accumulates is that the write path has an oracle and the read path
does not. `mxbuild` validates the *model*, and it never sees DESCRIBE output at
all — so a describer that drops a property produces a perfectly valid model, at
0 errors before and after. The only thing that notices is a person replaying the
output and finding their work gone.

That matters most where the feature is used most. `describe → edit → exec` is
mxcli's copy operation, and `DESCRIBE LAYOUT` emitting re-executable MDL is
explicitly why there is no `COPY DOCUMENT` verb. A lossy describer is costliest
exactly when someone is trying to reuse work.

## How it fits

**Five shapes, in increasing order of how long they survive.**

*Won't parse.* The emitter produces text MDL's own grammar rejects — a
reserved-word name emitted bare, an internal spelling (`call_microflow X`,
`ReadMode: CallMicroflow:…`) that no rule accepts, `Param = $v` where the
grammar wants `Param: $v`, a quote inside a string that was never doubled. Loud
and cheap: running `mxcli check` on the output finds it. The recurring cause is
that **storage form is not input form** — whenever DESCRIBE prints a value read
back from the backend, the question is whether the *parser* accepts that
spelling, and nothing else in the toolchain asks it.

*Silently drops.* A property is written correctly, present in the `.mxunit`, live
in the app — and absent from the description. The round trip deletes it. Output
parses, executes, and the model stays valid, so every automated signal is green.

*Invents.* The describer emits a clause the author never wrote: `comment 'Review'`
on a jump whose caption was only ever a default, `on error rollback` on an
activity with no error handling, a bare `else` on a split that has none. Each
round trip accumulates another, so the model drifts toward the emitter's
defaults. The governing rule is an **asymmetry**: omitting a value the writer
re-derives is lossless, while emitting it is lossy in the direction that
matters — it puts something in the user's script that they did not write. When a
formatter renders an enum whose fallback value is also a legal authored value,
read-back cannot invert the write; render only the values that are never
defaults.

*Destroys.* The round trip removes structure rather than losing a field. A list
view's specialization templates went 4 → 0; an accordion group's contents
vanished; a loop body was emitted empty because an annotation sat to its left.
`mx check` reported 0 errors on both sides of each.

*Means something else.* The worst, and the one that hides longest, because every
other shape leaves evidence. Here the description parses, executes, yields a
valid model, drops nothing and invents nothing — and **denotes a different
program**. A microflow whose activity ran on `¬c1 ∨ c2` described as `c1 ∧ c2`;
with the reporter's actual expressions, one that always logged described as one
that never did. Nothing in the toolchain can see it: there is no missing field to
notice and no error to raise, so the only detector is someone comparing
behaviour.

**Some documents cannot be described faithfully at all**, and that is a different
diagnosis from a careless describer. MDL's `if` is a single-entry/single-exit
block; a Mendix microflow is an arbitrary directed graph. When branches cross, no
amount of care in the describer helps, because the target language cannot spell
the graph — the remedy is to **extend the language** (named join points) rather
than to fix a bug. One sub-class provably has no faithful rendering at any
effort: per Böhm–Jacopini, nesting genuinely interleaved branches requires either
duplicating an activity the user drew once or inventing a boolean they never
wrote, and both are model rewrites rather than descriptions. Recognising that a
gap is a *vocabulary* limit tells you whether to reach for a fix or a proposal.

**The defect is usually the copy, not the case.** Describers get written
per-widget and per-container, so one lookup exists four or five times and some of
the copies are wrong. Patching the switch named in the report leaves the others to
drift again. A datasource means the same thing wherever it sits; so does an
action slot, and so does a text template. The fix that holds is one reader and one
renderer, with the type set taken from `generated/metamodel` rather than from the
copies. The same logic applies to a second *rendering mode*: build it by rewriting
the graph into one the existing describer already handles, never by writing a
second describer, or every activity renderer exists twice and drifts.

**Fixing one half is worse than the bug.** Where a describer has two defects at
once — say, quoting *and* a missing property — shipping the quoting fix alone
turns unparseable output into output that parses cleanly while silently dropping
something. That is strictly worse: a wrong page that validates.

**A round trip that is a fixed point on the corpus you have is not evidence that
it is faithful.** This is the trap that makes the *means something else* shape
survive. Where a describer rests on an inference rule — "an empty error handler
means the path falls through to here" — the rule was reverse-engineered from the
corpus, so the corpus agrees with it by construction. The test that matters is one
where the stored document means something *other* than the rule assumes, and you
have to **construct** it: repoint one pointer in a copy and re-describe. The real
microflow that motivated all of this round-tripped correctly by luck.

Three further measurement rules, each of which hid a defect until it was applied:

- **Compare identities, not counts.** A count cannot tell "preserved" from
  "deleted and recreated", and it hid an equal-sized swap of which pages failed.
- **Diff the whole corpus against a baseline binary.** Keep a pre-change build,
  describe every document with both, and diff. A pure refactor must come out
  byte-identical; a feature must change only the documents it targets. Anything
  else moving is the bug — this is what caught a warning being retired on the
  strength of a label nothing ever emitted.
- **Algebra is not behaviour.** When a change reasons about equivalence, the
  proof is two documents side by side on a real runtime over the whole input
  space, not a convincing derivation. See `.claude/skills/verify-in-runtime.md`.

One consequence worth knowing: other code re-parses DESCRIBE output.
`use building block … (datasource: …)` matches against the rendered form, so
fixing a renderer can break a consumer that never reads the model. Grep for
callers of the emitter before changing what it emits.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the per-construct
  recipes; `grep -l describe *.jsonl` reaches this class
- [[flow-graph-geometry]] — the sibling class where the generated graph's
  coordinates and wiring are wrong, rather than its rendering
- [[widget-type-object-drift]] — the neighbouring class where the *written* widget
  is wrong rather than the described one
- [[silent-property-drop]] — the write-side twin of *silently drops*
- `.claude/skills/verify-in-runtime.md` — for the cases where neither the model
  nor its description is the thing that is wrong
