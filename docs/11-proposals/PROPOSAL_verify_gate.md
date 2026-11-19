---
title: mxcli verify — one gate that reports only what a change made worse
status: draft
date: 2026-11-16
related:
  - PROPOSAL_check_mxbuild_gap_heuristics.md
  - PROPOSAL_check_diagnostics_catalog.md
  - PROPOSAL_agent_loop_efficiency.md
---

# Proposal: `mxcli verify` — a staged gate with baseline comparison

**Status:** Draft
**Date:** 2026-11-16 (updated 2026-11-19)
**Issue:** ako/mxcli#2333
**Relates to:** ako/mxcli#1313 (`mx check` baseline), #1312 (`diff --mx-check`),
#1258 (`check --full`), #2308 (`replace()` passes `check`, fails `mx check`)

## Problem

An agent or CI job that changes a project must know which of six commands to run,
in which order, and how to separate its own new findings from pre-existing ones.
Only `lint` can compare against a baseline (`--baseline`); every other stage has
its own output format and no notion of "new".

Measured in #2333: `mxcli check -p` said "Check passed!" for five of eight scripts
that an independent checker rejected (undefined variable CE0109, assignment type
mismatch CE0844, branch without return CE0109, Decimal returned as String CE0844,
`replace()` CE7811). `lint` added nothing for those. `mxcli docker check` caught
the undefined variable in 7.6 s. So the fast checks cannot be the whole gate; the
gate must include `mx check`.

**Relation to the gap programme.** These five misses are intake for
`PROPOSAL_check_mxbuild_gap_heuristics.md` (a standing programme that adds a static
heuristic each time `check` passes what `mx check` rejects) and
`PROPOSAL_check_diagnostics_catalog.md`. `verify` fixes none of them. It exists so
that the gap, whatever its size at any moment, cannot pass unnoticed: it makes the
~8 s authoritative stage part of the loop. Closing the gap stays with `check`, and
that work is **step one and keeps going** regardless of this proposal: each `mx
check` validation moved into `mxcli check` shrinks what only the slow stage can
catch. It never reaches zero, which is why the gate keeps `mxcheck` as a stage.

## Evidence that steps get skipped

An external benchmark (mxbench.mecowhy.dev, 2026-11-17; summarised in a comment on
#2333) built the same 111-story app nine times (three tools by three models).
Its maintainability critique (microflows with overlapping actions, saves in a
loop, no business key, unreachable template modules, over-wide entities, missing
access rules) maps almost entirely onto **existing `lint` rules** (MPR008,
CONV011, ARCH003, QUAL004, DESIGN001, SEC001 and others). None of it appears as a
caught problem because no agent ran `mxcli lint`; three agents on three tools and
three models each skipped a gate that was installed and correct. Two consequences
for the design:

- **A stage nothing invokes is indistinguishable from one that does not exist.**
  Discoverability is part of the deliverable, not a follow-up (see Phasing).
- **`lint` needs its baseline from the first release.** Run against an AI-built
  app with no baseline, lint fires in the hundreds; a gate whose first run
  reports 400 findings gets switched off.

The same evidence bounds what the gate can promise. The benchmark's non-lint
findings (a guest who can write a price, a look-up limit that resets with a new
browser session, paying one order emptying the cart, a home page that overflows
on a phone) need a model reader plus probing a running app. No static stage
reaches them; only `smoke` and beyond can, and `verify` should say so rather than
imply a clean result means "correct".

## Proposed behaviour

```
mxcli verify -p app.mpr [script.mdl...] [--base <git ref>]
                        [--stages check,lint,mxcheck,build,test,smoke]
                        [--fail-fast] [--fail-on error|warning] [--strict]
                        [--baseline-dir <dir>] [--update-baseline]
                        [--format text|json]
```

**First release (step 2, implemented):** stages `check`, `lint`, `mxcheck`;
`--base`, `build`, `test`, `smoke` and `--format sarif` are later steps.

| Stage | Reuses | Default | Cost |
|-------|--------|---------|------|
| `check` | `mxcli check <scripts> -p` on the **script files given as arguments**; skipped (and reported skipped) without any. `check` has no project-wide mode | on with scripts | ~0.2 s |
| `lint` | `mxcli lint --format json` against its own baseline file | on | ~0.2–0.9 s |
| `mxcheck` | `docker.CheckFindings` (`mx check --json` on a copy) | on | ~8 s |
| `build` | `run --local` build/boot | opt-in | 30–60 s |
| `test` | `test --local` | opt-in | ~24 s |
| `smoke` | `playwright check --role X` | opt-in | ~24 s |

- Stages run in cost order. **Default: run every selected stage** and report all
  new findings in one result. The consumer is an agent, and stopping at the first
  failing stage costs one round trip per stage (fix, rerun, next stage fails),
  each re-reading the whole conversation (the cost model in
  `PROPOSAL_agent_loop_efficiency.md`). Spending ~8 s of `mxcheck` once saves two
  reruns. `--fail-fast` stops at the first stage with new findings, for CI.
  Opt-in stages (`build`, `test`, `smoke`) are still skipped unless selected, and
  are skipped when an earlier stage failed, since they need a model that builds.
- Findings are classified **new / resolved / pre-existing** against the baseline (`mxcheck` adds **unmasked** and **hidden**, see the staging section),
  keyed by IDs for `mxcheck` (see Finding identity) and by lint's own key for
  `lint` — never by message text or line number, which drift between runs (the
  problem #1313 describes). A stage without a baseline reports every finding as
  new and says so (`note`), so a first run is honest, not silently green.
- **Exit code** (a contract, printed in `--help`): `0` nothing new; `1` usage
  error; `2` a stage could not run (no `mx`, ...) and nothing failed; **`16` plus
  one bit per failed stage** (1 `check`, 2 `lint`, 4 `mxcheck`) when a stage
  failed, so `18` is lint, `22` is lint and mxcheck, and a failure can never be
  read as "could not run" or as a usage error. When one stage failed and another
  could not run, the failure code wins (it is the actionable one) and the
  stage's `status: error` is in the output. `check` exits with a variable code
  and `lint` exits 1, so an agent running them directly sees different codes for
  the same finding; `verify`'s table is its own. `--fail-on warning` makes a new
  warning fail a stage too (default: errors, as `lint` does).
- **Output:** `--format text|json` (the global `--json` is the existing alias via
  `resolveFormat`); `sarif`, which `check` and `lint` also offer, is a later step. JSON is one
  object `{stages:[{name,status,durationMs,new:[...],resolved:<list or count>,
  preExisting:int}], exit}`, so an agent reads one result instead of six formats.
  `resolved` is typed honestly per stage: a list where the key identifies the
  element, a count where it cannot (see Finding identity).
- **Machine-output discipline.** `verify` must register in `machineReadableFormats`
  (`output_stream.go`), send progress through `progressSink(format)` so stdout
  carries only the payload, and get a row in `json_output_purity_test.go`
  (modelled on `TestCheckStructuredOutputIsOneDocumentOnStdout`). `lint --format
  json` once printed "Connected to:" ahead of the JSON; a new command must not
  repeat that.
- **SARIF.** `check` and `lint` already emit SARIF, which has `partialFingerprints`
  and `baselineState: new | unchanged | absent` — the same three states as this
  proposal. `--format sarif` is therefore supported rather than a seventh bespoke
  format being the only option. The proposal still defines the JSON shape because
  SARIF is verbose and awkward for an agent to read in one call, and because it
  has no place for per-stage durations or "could not run"; the JSON is a
  projection of the same internal findings and the SARIF writer is its second
  rendering.

### Baseline sources

1. `--baseline-dir <dir>` — saved findings, one file per stage (cheap; shipped
   first). `--update-baseline` records the current state.
2. `--base <git ref>` — run the same stages on a scratch export of that ref, then
   compare. Roughly doubles `mxcheck` cost; results are cacheable per commit.

## Existing pieces and gaps (checked against `main` at `ecdd9b7c`; see also the update below)

**Update 2026-11-19.** Since this section was written, `main` gained about twenty
`check` validations (step 0: MDL097/MDL098, the `replace()` fix, DECLARE/SET/RETURN
type checks, and others), the `mdl-examples/check-gap/` parity harness,
`scripts/diagcoverage` and `scripts/maskmatrix`. None of it read `mx check`
output into findings (the harness compares codes in shell, `diagcoverage` reads
only the code catalog); step 1 (`docker.ParseCheckJSON`) now does, from
`mx check --json`. The work also supplied the fixtures for its golden files and
two measurements that change the design (next sections). The gaps listed below
predate step 1 and the `docker.Check` one is closed for the stage's purposes.

- `mdl/linter/baseline.go` has `WriteBaseline`/`LoadBaseline`/`Compare`, but it is
  **not reusable as is** against the `Finding` below:
  - its key prefers `DocumentID` (`baselineKey`: `rule \0 id \0 <id>` when an ID is
    present), so a verify finding needs an `ID` field to reproduce it;
  - `Compare` pairs multiplicities on identical **message text** within a key group
    (pass 1), then treats an unmatched one as a reworded form (pass 2), so
    "message is display only" cannot hold for the lint stage unless `Compare` is
    wrapped or replaced;
  - `BaselineComparison` is `{New []Violation, Unchanged int, Fixed int}`, and the
    file stores messages per key, not whole violations: lint's `resolved` is a
    **count**, not a list.
  The lint stage therefore either keeps lint's own file and `Compare` and adapts
  the result, or `Finding` grows an `ID` and the matching rules are restated.
  Step 2 decides; it is not "reuse as is".
- `docker.Check(opts) error` (`cmd/mxcli/docker/check.go`) runs `mx` on a temporary
  copy and **writes free text** to `opts.Stdout`; it returns only an error.
  (`appErrorCount` is an unexported helper.) **There are no structured `mx check`
  findings to baseline — this is the central gap.** The `mxCheckCmd` and
  `resolveMxForCheck` test seams are worth keeping.
- `check` and `lint` take `--format text|json|sarif` (`main.go`). A project-wide
  reference-check entry point for the `check` stage needs a thin adapter.
- `test --local` (`testRunCmd`) and `playwright check` have their own reporting;
  each needs an adapter to the common shape.
- `diff-local --ref` resolves git refs for mxunit diffs; exporting an MPR at a ref
  (v1 needs an extraction step, v2 is `mprcontents/`) is new.
- **Name:** the only existing `verify` is `playwright verify <file|dir>` (runs
  Playwright test scripts). `mxcli verify` next to `mxcli playwright verify` is the
  ordinary noun-under-a-namespace case, not a collision of two top-level commands.

## MxBuild reports in stages: new, resolved and unmasked

Measured in `PROPOSAL_check_diagnostics_catalog.md` (MxBuild 11.16.1, six defects
and every ordered pair, `make mask-matrix`): `mx check` does not report every
error, it reports the earliest stage that has any.

| stage | reported | hides |
|---|---|---|
| 0 | an unresolved reference (CE1613) | every other error in every document |
| 1 | a duplicate variable (CE0111) | every other error in the same microflow |
| 2 | an undefined variable (CE0109) | the stage 3 group, in the same microflow |
| 3 | CE0117, CE0068, CE0038, CE0067 | nothing |

For a baseline diff this is a correctness problem, in both directions:

- **False "new".** The baseline has a stage 1 error in microflow `M.F`. The change
  fixes it, and a stage 3 error that was always there appears. A plain diff blames
  the change for it.
- **False "resolved".** The change introduces a stage 0 error. Every other finding
  in the project disappears from the report and a plain diff calls them all
  resolved.

So `verify` classifies the `mxcheck` stage's findings into four states, not three:

| state | meaning |
|---|---|
| **new** | in the report and not in the baseline. This includes a later-stage error uncovered in a microflow whose earlier-stage baseline error is gone (see below) |
| **unmasked** | new in the report, but a **stage 0** error in the baseline (an unresolved reference) was hiding the whole project. Fixing that one error uncovers everything else, and none of it is the change's |
| **resolved** | in the baseline, absent from the report, and nothing in the report hides it |
| **hidden** | in the baseline, absent from the report, and the report has a finding that hides it. Not resolved |

Rules that follow:

- **Scope decides.** A stage 1 or 2 error hides only the later stages of its own
  microflow, and it can only disappear if the change edited that microflow, so a
  later error uncovered there is the change's to fix: **new**, and it fails. Only
  a stage 0 error hides other documents, so only what it hid is **unmasked**; it
  does not fail the gate by default (it is almost certainly old) and `--strict`
  makes it fail. The first draft made every uncovered error unmasked; a real run
  (a flow whose duplicate variable was replaced by a type error) passed the gate
  with an error the change had just written, which is what this rule fixes.
- A new stage 0 finding always fails the gate, and the `hidden`
  count is printed next to it so a reader does not read "400 resolved".
- Each finding carries its **stage** (`StageOf`, from the measured table). A code not in the measured set is treated as stage 3 (hides
  nothing, hidden by earlier stages): the conservative reading for `unmasked`, and
  recorded as a limit until `maskmatrix` covers more codes. The measurement covers
  microflows and pages only.
- The `check` stage (`mxcli check`) and `lint` do not stage: `mxcli check`
  deliberately reports every real error (the catalog proposal's "report every real
  error" rule), so new, resolved and unchanged are exact there.
- Staging is the main reason to run `check` first and `mxcheck` second: the more
  `mxcli check` reports, the less the slow stage can hide.

## An optional fast `mxcheck` pre-stage: Mendix CLI 2

The same spike measured Mendix CLI 2's `mendix check` at 1.3 to 1.7 s against 6 to
8.5 s for `mx check`, with no staging. It is proprietary (never committed, path
supplied by the user) and gave false errors on some real apps, so it cannot be the
authoritative stage. Design consequences:

- `mxcheck` keeps `mx check` as the authority. `--mxcheck-with mendix` (name open)
  runs `mendix check` as an **advisory pre-stage**: its findings are reported and
  never gate on their own; anything that would gate must be confirmed by `mx check`.
- Codes differ between the checkers for one finding (CE1613 in `mx` is CE7710 in
  `mendix check`), and it reports more errors in a project whose references do not
  resolve. A finding therefore records its **checker**, and a baseline is only
  compared with findings from the same checker. A baseline file records the
  checker and its version, and a mismatch is an error, not a diff.
- Off by default; revisit once there is evidence about its false-positive rate on
  real projects.

## Finding identity: read `mx check --json`, not its text

An earlier draft keyed findings on `(code, document, element path)` parsed from
the text, and asked for which codes that could be built. Run against real MxBuild
11.16.1 (the `mdl-examples/check-gap/` fixtures; the output is committed as
`cmd/mxcli/docker/testdata/mxcheck/`), the premise was wrong:

- **The text never names the document of a flow element.** A finding reads
  `[error] [CE0109] "Undefined variable 'Nope'." at End event`. The same defect in
  two microflows prints two identical lines. A default caption such as `Create
  variable activity 'Create Integer/Long variable'` is not unique either. No
  classification table over the text can fix that.
- **`mx check --json <path>` carries the identity.** Each problem has
  `module-name`, `document-name` (`Microflow 'F05'`), `element-name`,
  `element-id` and `unit-id`, and the two microflows above differ in both IDs. The
  IDs were identical across two runs and a copy of the project. `-w -d -b` add
  warnings, deprecations and best-practice findings (a fresh project already has
  16 warnings and 1 deprecation); the file declares `total-problems`, which the
  reader checks against what it read.

So the `mxcheck` stage **reads the JSON** and keys a finding on severity, code and
the `(unit-id, element-id)` of its locations; names and messages are display only
and may change without making a finding new. A finding with no location (the
runtime-settings deprecation) has nothing to key on but its code and is matched by
count, as `recheckCE0463` does today.

Not yet measured: a CE0463 (the widget case that motivated the count fallback)
could not be produced in the spike, so whether its JSON carries IDs is **open**.
`recheckCE0463` keeps its text-based comparison until it is. Stability of the IDs
across a Studio Pro module update or an `mxcli` rewrite is also unmeasured; they
are `$ID`s, which `canon` carries across writes but Studio Pro renumbers.

## Design

New package `cmd/mxcli/verify/` (orchestrator, no business logic):

```go
type Finding struct {
    Stage, Checker, Code, Document, DocumentID, Path, Message string
    MxStage int // MxBuild report stage 0-3 for `mxcheck` findings, else -1
}
type Stage interface {
    Name() string
    Run(ctx context.Context, p Project) ([]Finding, error)
}
```

- `Finding.Key()` is built from IDs (see Finding identity): `Stage + Code + UnitID/ElementID`, by count for a finding with no location. The lint stage's key additionally uses `ID` when present, as `baselineKey` does. Message is display only except where a stage's existing matcher (lint `Compare`) uses it, and then that is stated.
- Shared **`mx check --json` reader** (`docker.ParseCheckJSON`, `docker.CheckFindings`)
  producing `[]Finding` with module, document and element IDs.
  Also unblocks #1313 and #1312; lands as its own PR.
- Baseline file is versioned JSON, sorted for stable diffs, one section per stage
  (lint reuses its existing format inside it or keeps its own file — see questions).
- Infrastructure failure (no `mx`, no Docker/runtime for opt-in stages) is a
  distinct status `error`, never `passed`.

## Out of scope

- Fixing findings; running stages in parallel (stages run in cost order; `--fail-fast` is the CI mode);
  replacing `docker check`, `lint`, `test` or `playwright check`.
- `check --full` (#1258): `mxcheck` stage calls `docker check` until it exists,
  then may be swapped or made optional.

## Phasing

0. **Standing: close the `check` / `mx check` gap** per
   `PROPOSAL_check_mxbuild_gap_heuristics.md`. Already under way (each validation
   moved into `mxcli check` removes a class of late failures); independent of, and
   not blocked by, the steps below.
1. **Structured `mx check` findings** (done): `docker.ParseCheckJSON` and
   `docker.CheckFindings`, reading `mx check --json`, with golden files from a real
   run of the `mdl-examples/check-gap/` fixtures and a control (no defect). An
   earlier text parser was replaced because the text does not identify the
   document (see Finding identity). Remaining: a CE0463 golden file, and wiring the
   `mxcheck` stage to it in step 2.
2. **`mxcli verify`, first release** (implemented): stages `check`, `lint`,
   `mxcheck`; `--format text|json`, the exit-code table, `--fail-fast`,
   `--fail-on`, `--strict`; and a **`lint` baseline from day one**. Baselines live
   in `--baseline-dir` (default `.mxcli/verify` next to the project): `lint.json`
   is a plain `mxcli lint --baseline` file (so existing ones stay valid) and
   `mxcheck.json` records mx check's findings by element ID, with the checker and
   mx version, which a run must match. `lint` and `check` run as subprocesses of
   the same binary, so they behave exactly as run by hand and `lint` is not
   refactored. Checked on real MxBuild 11.16.1: a new defect fails (exit 20), an
   accepted one does not, a stage 0 fix is unmasked, a same-flow uncovered error
   is new.
3. **Make `verify` the thing everything points at**: skills, `mxcli init` output
   and the project `CLAUDE.md` guidance name `verify` as the single "did my change
   break anything?" command. **Not in the first release, and a decision is
   needed:** `projectGates` is the definition of done in every generated
   `CLAUDE.md`. Adding `verify` as a seventh gate runs lint and `mx check` twice
   and puts the file 77 bytes over its test-enforced budget; the alternative is
   for `verify` to *replace* the `lint` and `docker check` gates, which changes
   every project's default and needs `bootstrap-app` to record the baselines at
   provisioning. Either way the gate tests (`init_claudemd_gates_test.go`) already
   keep the three descriptions in step.
4. **`--base <git ref>`** via scratch export.
5. **Opt-in `build`, `test`, `smoke` stages.** `smoke` is also the only stage that
   can reach the runtime-only findings listed under Evidence.

## Testing

- Unit: finding keys are IDs, not text (same finding, different message or name → same
  key; same text, different element → different key); classification new/resolved/pre-existing; default run-all vs `--fail-fast`.
- Staging: a stage 0 baseline error fixed → what it hid is unmasked, not new;
  a same-microflow earlier error replaced by a later one → new (the case a real
  run showed); a new stage 0 error → the baseline's findings are hidden, not
  resolved; **control:** the same finding with no hiding baseline finding is new
  and fails the gate.
- Checker identity: a baseline from one checker compared against the other is an
  error.
- Reader: golden `mx check --json` files from real runs (committed under `cmd/mxcli/docker/testdata/mxcheck/`), the same defect in two microflows (two keys), a clean control, a lost-finding control for `Verify`, and an integration test against a real `mx`.
- Control (per CLAUDE.md): a "nothing new" test paired with one that injects a
  finding and must fail.
- Output purity row in `json_output_purity_test.go`; exit-code table incl. several failing stages at once and "could not run".
- Executor-level orchestration with stub stages (same seam style as
  `mxCheckCmd`); integration (`-tags integration`) for real `mx`.
- Runtime stages follow `.claude/skills/verify-in-runtime.md`.

## Open questions

1. **Name.** `mxcli verify` alongside `mxcli playwright verify`: implemented as
   `verify`; rename if the maintainers prefer `gate`/`validate`.
2. ~~One baseline file or one per stage?~~ **Settled in step 2:** one directory,
   one file per stage, so `lint --baseline` files stay valid.
3. **ID stability.** Are `unit-id` / `element-id` stable across a Studio Pro
   module update and an `mxcli` rewrite (a `create or modify` kept the unit ID in
   the real run), and does a CE0463's JSON carry them?
4. **`--base` for MPR v1** — extract via `git show` of the single `.mpr`, or require v2?
5. ~~Should `check` stage fail on warnings?~~ **Settled:** errors by default,
   `--fail-on warning` to include warnings.
6. ~~Exit-code encoding.~~ **Settled:** 0, 1, 2, and 16 plus a bit per failed stage.
7. ~~Unmasked policy.~~ **Settled by scope:** only what a stage 0 error hid is
   unmasked (non-failing, `--strict` fails); an error uncovered in a microflow the
   change edited is new.
8. **Staging coverage.** The measurement covers six codes in microflows and
   pages. How far must `maskmatrix` be extended before `unmasked` is trusted
   beyond them?
9. **`mendix check` pre-stage:** worth a flag now, or defer until its
   false-positive rate is measured on real projects?
10. **Default stage set** — `mxcheck` costs ~8 s once per run and is on by default.
11. **Step 3:** add `verify` as a seventh gate, or replace `lint` and `docker check`
    with it (see Phasing)?
