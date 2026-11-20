---
title: DESCRIBE Round-Trip Gaps
category: bug-pattern
last-synced: a20932c1
covers:
  - mdl/executor
sources:
  - .claude/skills/fix-issue/findings/mdl-executor/
  - mdl/executor/cmd_workflows.go
  - mdl/executor/cmd_pages_describe_pluggable.go
  - mdl/executor/cmd_pages_describe_bykey.go
  - mdl/executor/cmd_microflows_show_crossed.go
  - mdl/executor/cmd_microflows_normalize.go
  - mdl/executor/describe_graph_invariant_test.go
  - mdl/microflowgraph/structure.go
  - mdl/executor/cmd_microflows_derived_layout.go
  - mdl/roundtrip/
  - docs/13-decisions/0008-identity-and-idempotence.md
  - docs/11-proposals/PROPOSAL_structured_microflow_description.md
---

> **Do not duplicate**: the per-construct fix recipes live in the findings
> (`grep -rl describe .claude/skills/fix-issue/findings/`), the MDL syntax
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

**The verb is the gate, and flipping it turns every refusal into a write.** A
describer that emits a plain `create` does not just fail on an existing document
— it **hides every loss behind it**, because nothing downstream runs. Changing
those describers to `create or modify` is what makes their output execute at
all, and it immediately surfaced the next layer: a Java action's parameter
descriptions deleted, a JavaScript source file overwritten with the placeholder
body, an association's storage flipped from table to column — a *database schema
change* — because describe omits the clause for `Table` and the modify arm
assigned the statement's unstated default. So audit each property the describer
**omits** against the writer's default before flipping the verb, and expect
another round of losses the moment it executes rather than treating the first
fix as the end.

**A `--` comment in describe output is a data-loss marker, not a rendering
choice.** Every one of them turned out to be a property the grammar could not
state — a user role's description and check-security flag, a Java action
parameter's description and category, a published REST header, workflow
properties. The parser drops the comment, so the round trip drops the value, and
the note reads as documentation rather than as the admission it is. Grep the
describers for `-- ` emits of *model values* before believing a doctype
round-trips. A note that promises preservation is worse still: a
full-replacement write cannot deliver it, because omitting a slot is
indistinguishable from the author deleting it.

**What the language cannot spell has to be carried, and a carry has
conditions.** The general remedy for an unspellable property is to merge the
stored value onto the rebuild by name — but a carried value is valid only under
the inputs it was stored under. A published member's `CanBeEmpty` carried by
name turned a legitimate `(KEY)` edit into an invalid model, because the key
status is one of its inputs; a stored menu action kept "because MDL cannot say
it" was kept onto an element kind that cannot legally hold one. And enumerate
the paths: there were two statements rebuilding one document kind, and a carry
wired into `create or modify` alone was missed by `alter … modify attribute`
rebuilding the same element through the same converter. Grep the **converter's**
call sites, not the reported statement. Carrying an element's *identity* is not
carrying its *content*: an activity edited in place kept every `$ID` and still
wrote a REST call's proxy at the default, because the content came from the
statement and the statement cannot spell a proxy.

**Where a field-by-field carry cannot be trusted, make replaying the description
a no-op.** If the statement parses to exactly what the stored document's own
description parses to, it denotes the stored document — keep the stored bytes
and report `Unchanged` — and refuse any other rewrite that would have to invent
what the language cannot say. An XML-schema mapping's root element and element
paths (no XSD reader to rebuild them) and a view query's whitespace outside its
token span both take this form. Compute "what the description parses to" with
the real describer and parser, never a model of them, or the two drift. And a
member another statement *declares* — a view entity's association, declared by
its OQL column — must be skipped by every describer of its kind, or the
description re-declares it in a form the platform refuses.

**Present-but-empty is a stored value, and a readability fallback is a write.**
A describer that falls back to another language when the preferred one is present
but empty prints the Dutch caption into the English slot, and `exec` writes
whatever describe printed — so the fallback *invents* in one language and
*deletes* in the other. Fall back only when the language has no entry at all.
The same third state appears without translations: an argument field Studio Pro
stores as `Argument: ""` is neither the value `empty` nor absence, and a
describer that conflates it with either cannot round-trip to `Unchanged`.

**"Default" means what exec writes for the key left out, not the schema's
`DefaultValue`.** A by-key reader prints what differs from the default, and the
two disagree wherever the embedded template stores something else — a combo
box's `readOnlyStyle` is `bordered` in the template against a default of
`text`. Compared with the schema, every mxcli-written widget prints a value
nobody authored (*invents*), and every Studio Pro one storing the schema default
prints nothing and replays as the template's value (*silently drops*). A reader
for a widget with a dedicated describer has to run alongside it, or every key
outside its keyword set is lost — and what it prints must also pass `check`
without `-p`, where only the built-in definition knows the widget's keys.

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

**For a flow, the oracle exists: rebuild the description and compare graphs.**
The *means something else* shape is invisible to every check that reads the
text, but not to the builder. Parse the description, build it the way `exec`
would, and compare the result with the stored graph modulo exclusive merges —
every stored activity exactly once, every stored sequence flow present
(`assertDescriptionRebuildsGraph`). A duplicated region shows up as two rebuilt
activities on one stored position, a dropped back-edge as a missing flow. Run it
over every flow of a real app, on the baseline and the branch, rather than over
the flows someone reported: the reported ones are the ones whose symptom happened
to be loud. And do not take `create or modify`'s `Unchanged` on a describe output
as proof — that is the splice verdict, "the script equals the stored flow's own
description", which a wrong description that parses also earns.

**A flow describer is several searches that have to agree.** Which merges get
a label, where an error handler stops, which merge closes which split, and
whether anything is printed after a branch are each decided by a separate walk
over the same graph. A node class one walk looks past and another stops at
silently drops what lies beyond it — a merge with a single way in, which joins
nothing, was the stop for one walk and invisible to the labelling one, so the
rest of a handler vanished. And a statement's spelling can depend on what is
printed *after its enclosing branch*, not on the node: an activity with no way
out ends a loop iteration, and needs `continue` exactly when the description
goes on after the branch closes — at any level up to the loop body. When a
sweep turns up a new shape, ask which two walks disagree about it before
adding a case to one of them. Inside a `while true` every node
is on a cycle, so a walk asked on behalf of one split must stop at that split:
past it, it has gone round the loop and "reaches" the split's own branches. The
rule has to hold for **all** the walks for that split at once — applying it to
the one the report exercised moved the step after a join back out of the THEN
arm and pulled another flow's loop exit out of its ELSE arm, which only the
whole-corpus baseline diff showed.

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

**Two guards that look general are not.** The source scan for hand-rolled
`'%s'` quoting covers **the files on its list**, and two describers escaped it by
living in files dominated by validation prose — so adding the file is the first
step of any quoting fix, and the guard that does not depend on a list is a
re-parse of the emitted statement under both describe languages. And **the
empty-collection branch is the one no fixture exercises**: a published REST
service with no resources emitted a bare `;` against a grammar that requires the
block, a demo user with no roles emitted a clause form that cannot say "none".
Feeding every `Describe*_Mock` output through the parser finds these at once.

**The corpus is free, and it is Studio Pro-authored.** The expensive-sounding
prerequisite — a real project's documents, without committing a 15 MB fixture —
is already in the test run: `mx create-project`'s Blank template ships
`Administration`, `FeedbackModule` and Atlas documents that are Studio Pro's own
output, byte-identical to the audited ones in real apps. Pointing the round-trip
harness at it, and then at marketplace modules and a 1,875-flow app, is what
turned up the graph-spelling classes above; they were not reachable from flows
mxcli had written, because a describer and a builder agree by construction on
those. Snapshot the **source tree** as well as `mprcontents/` when a doctype owns
files on disk, or the describe placeholder body overwriting a real `.js` file is
invisible.

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
- [[binding-context]] — the read half of a shortened name: which object a bare
  attribute is re-derived against
- [[reports-that-outrun-the-write]] — why `Unchanged` on a re-run is the signal
  a round trip is judged by
- `.claude/skills/verify-in-runtime.md` — for the cases where neither the model
  nor its description is the thing that is wrong
