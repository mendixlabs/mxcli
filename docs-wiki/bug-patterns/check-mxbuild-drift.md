---
title: When `mxcli check` and mxbuild Disagree
category: bug-pattern
last-synced: 038f810e
sources:
  - .claude/skills/fix-issue/findings/mdl-executor.jsonl
  - .claude/skills/fix-issue/findings/mdl-backend.jsonl
  - .claude/skills/fix-issue/findings/mdl-grammar.jsonl
  - mdl/executor/validate_program.go
  - docs/11-proposals/PROPOSAL_check_mxbuild_gap_heuristics.md
---

> **Do not duplicate**: the rule catalogue and its rationale live in
> `PROPOSAL_check_mxbuild_gap_heuristics.md`; each rule's exact predicate and CE
> number live in the findings and in `mdl/executor/validate_*.go`. This page
> describes the failure class on both sides.

## What this is

`mxcli check` is a **model of mxbuild**, not a port of it. It runs before a
build — often before a project even exists — so every rule is a prediction, and
predictions drift in two directions:

- **A gap.** `check` passes, `exec` writes, and the build then fails with a `CE`
  code. The user has already changed their project before anything told them.
- **A false refusal.** `check` rejects MDL that mxbuild accepts at 0 errors.

Both appear repeatedly in the executor findings. The second used to be a mere
annoyance and is not any more: since `exec` began refusing scripts whose check
reports an **error**, a false positive is a blocker rather than a warning.

## How it fits

### Getting the prediction right

**Verify a rule against mxbuild, not against intuition.** Rules have been added
on a plausible reading of a CE code and later measured to be wrong: one flagged
format functions over association navigation as a build error and was deleted
outright; another demanded a `return` on every path from a microflow that builds
cleanly, because the builder synthesises one. A rule that predicts mxbuild has to
be checked against mxbuild.

**A rule reproduced from failing neighbours can still be mis-premised.** The
deleted rule was written after reproducing several failures that shared the
construct it flagged — and the construct was not the cause. They shared a
*different* hidden defect, in the write path. When a write-path fix lands,
re-validate the checks derived from the same symptoms; a correlation-based rule
outlives the correlation.

**The hardest gap to find is a documented belief.** A layout guard asked only
whether *some* placeholder existed, because mxcli's own documentation said naming it
`Main` was a convention — and mxbuild validates it as a rule. Grepping the code for a
missing check could never have found this: the check that existed matched the docs
exactly. When a guard looks deliberate and cites a rationale, verify the *rationale*
against the tool, not the guard against the rationale. The measurement that settles
such a question has to isolate the rule from its neighbours — a layout no page uses
separates a layout rule from a page-binding one.

**Syntax that nobody ever built is a gap by default.** A microflow statement shipped
for an action Mendix requires a target reference on, with no clause to supply one, so
every instance mxcli wrote failed the build — for as long as the statement existed,
because nothing in the example corpus or the skills used it and so nothing ever put it
through `mx check`. A statement's existence in the grammar is not evidence it builds.
For any statement that writes an action, compare a Studio Pro-saved instance against
what the writer sets: a required property the grammar cannot express is a build
failure waiting for its first user. Whether a property is required is cheap to settle
across versions — write the action without it into a blank project per mxbuild.

**Severity turns on what the builder's condition is actually about, and the
intuitive reading is often the wrong one.** The empty-outcome rule looked like a
warning — surely an enumeration decision only needs an empty branch when the
value can be empty — until a *required* attribute was measured and produced the
same build error. The condition was on the enumeration type, not on the value,
which makes it an error. Measure the exemption you are about to grant; do not
infer it.

**Severity is the design decision, not an afterthought.** A rule whose vocabulary
cannot be proven complete — anything about widget properties, or a name that
might be legal in a context the rule cannot see — must be a *warning*, or it
trades a silent defect for a false refusal. That only works if warnings genuinely
do not block: an exec guard written as `if len(violations) > 0` makes every
warning fatal, which is how a warning-severity rule silently became a blocker for
everything it touched.

**The remedy is part of the rule, and a wrong one is worse than silence.** Two
rules have shipped whose `Fix:` line walked a working project into a broken one —
a view-entity column told to change `Integer` to `Decimal` (the reverse of what
mxbuild wants), and a design-property warning whose suggested keys turned 16
warnings into 17 `CE6083` errors. A rule that cries wolf costs attention; a rule
that hands over a wrong remedy costs the build. Where a fallback has to guess,
return *unknown*: a skipped column is a missed error, a wrong guess is a
manufactured one.

### Three answers, not two

**A check needs three outcomes where `exec` needs two.** `exec` can answer
resolved / not-resolved, because it has a fallback either way. A check that turns
*I could not look* into *your attribute is missing* is a false error blocking a
script that builds cleanly — so **could not establish** is a third state, and only
the middle one is reported. The guards that implement it are never obvious from
the rule: a module the script itself creates has no listing to resolve against
yet; an **empty** listing means the backend could not answer, not that the project
has none; a qualified member path can only be judged once the base entity is
known. Each of those was a measured false positive, not a hypothetical.

**The third state has to be applied at every reporting site, not once.** One
member-reference rule inherited the discipline on its bare-name path and not on
its qualified path, so exactly half of it was safe.

**`check` normally runs with no project at all, and that is the CI default.**
`make check-mdl` sweeps the corpus without `-p`, where the widget registry holds
only the embedded definitions and every real project's widget looks unknown. A
rule measured only with a project is a rule measured in the minority case: one
went from clean to 14 violations on a single example and broke seven corpus files.

**For a rule keyed on a type, prefer a deny-list.** An allow-list makes the
unknown case an *error*; a deny-list makes it silence. A missed warning costs
nothing and a false one tells an author their working page is broken.

### Don't model what you can run

**Mirror the builder's own condition rather than inventing a second one.** Where
`check` predicts something the builder decides, the two conditions must be the
same expression, not two readings of the same intent — otherwise they drift on
the first edit to either. This is the [[duplicate-resolver-drift]] class pointed
at mxbuild.

**Better still, run the real operation against a throwaway copy.** The vocabulary
of an `ALTER … SET` is partly a switch in the mutator and partly the *stored*
widget's own PropertyTypes, which belong to whatever package the project
installed — no registry in this repo can state it for an arbitrary project. So
the check opens the document and runs the actual setter against a deep copy whose
`Save` is refused, keeping only the error. Check and exec cannot drift, because
there is one resolver, and the author gets exec's exact wording from the
pre-flight.

**Close a gap in both passes or in neither.** A check-time rule does not protect a
script that runs `exec` directly, and `--no-check` exists. Both call the same
function so they cannot diverge — the convention exists because they did.

**When probing a gap, probe every sibling.** A missing reference check is almost
never one reference kind: the workflow case turned out to have *nothing*
validated — called microflow, called workflow, user task page, targeting
microflow, context entity, and the workflow's own module. Fixing the reported one
leaves the class open and the next report looks new.

**Grade by consequence.** The same statement can produce a recoverable build error
or an unopenable project depending on one detail — a qualified-but-missing name
versus an unqualified one. Those want different gates: the first needs a project
and belongs in the reference check; the second is a static property of the
statement and can be refused with no project at all, which is also what makes it
testable as a `.fail.mdl` fixture in CI. The severe end of that scale is
[[unloadable-model-writes]].

### The instruments lie in specific ways

Every rule here is justified by a measurement, so the measurement apparatus is
part of the class.

**mxbuild reports one error per microflow, so a second defect in the same document
hides the one you are measuring.** An exemption justified by *measured: builds
clean* is only sound if the reproduction was otherwise valid. When a measurement
says a construct is clean, add the minimum that removes every *other* error from
that document and measure again; the differential is what settles it.

**A load failure suppresses the error-count line while still exiting non-zero.**
`mx check` prints a stack trace and no `The app contains: N errors.` — so a
harness grepping for that line reads the run as inconclusive, and a human reads it
as success. Never conclude success from the absence of that line; read the exit
code. This has now cost a diagnosis three times.

**The corpus sweep has a noise floor and a blind spot.** Running a new rule over
`mdl-examples/` before wiring it up is still the cheapest false-positive test
available — one candidate hit 4 files and 3 hits were false positives. But the
output was *nondeterministic* until the validators that emit per map key sorted
them, and 11 of 515 scripts differed between runs of the same binary; and a diff
of `check` output compares **diagnostics**, so it is blind to a construct that
parses into the wrong AST shape rather than being rejected. There, 515 scripts
said nothing and two visitor unit tests caught it immediately. Diff diagnostics to
find false positives; assert on the AST to find misparses.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — every rule's
  predicate, its CE code, and the measurement behind it
- [[unloadable-model-writes]] — the failures that have no CE code because the
  build never gets that far
- [[duplicate-resolver-drift]] — the general shape when the second answer is not
  mxbuild's
- [[misleading-diagnostics]] — what a wrong message costs once it is believed
- [[capability-gap-as-parse-error]] — the inverse: Mendix supports something MDL
  cannot spell, where here MDL spells something Mendix cannot build
- `PROPOSAL_check_mxbuild_gap_heuristics.md` — the design rationale for
  predicting mxbuild at all
