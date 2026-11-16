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
**Date:** 2026-11-16
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
~8 s authoritative stage part of the loop. Closing the gap stays with `check`.

## Proposed behaviour

```
mxcli verify -p app.mpr [--base <git ref> | --baseline <file>]
                        [--stages check,lint,mxcheck,build,test,smoke]
                        [--fail-fast] [--format text|json|sarif]
                        [--update-baseline]
```

| Stage | Reuses | Default | Cost |
|-------|--------|---------|------|
| `check` | project-wide reference check (`check -p … --references`) | on | ~0.2 s |
| `lint` | `linter` + `Baseline` | on | ~0.2–0.9 s |
| `mxcheck` | `docker.Check` | on | ~8 s |
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
- Findings are classified **new / resolved / pre-existing** against the baseline,
  keyed by `(code, document, element path)` — never by message text or line
  number, which drift between runs (the problem #1313 describes).
- **Exit code:** 0 when nothing is new; 2 when the gate could not run (no `mx`,
  no runtime for a selected stage), so "gate failed" and "gate could not run" are
  never confused. When stages fail, the code is a **bitmask** (1 check, 2 lint,
  4 mxcheck, 8 build, 16 test, 32 smoke) offset so it cannot collide with 0 or 2
  — exact encoding to be fixed in phase 2 — because with run-all as the default
  several stages can fail at once. The per-stage detail is always in the output;
  the code is only a summary. **Relation to existing codes:** `check` exits with a
  variable code (`cmd_check.go`) and `lint` exits 1. `verify`'s table is its own
  contract and must say so in `--help`; an agent running `check` then `verify`
  will see different codes for the same finding.
- **Output:** `--format text|json|sarif`, matching `check` and `lint` (the global
  `--json` is the existing alias via `resolveFormat`), not a new flag. JSON is one
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

1. `--baseline <file>` — saved findings (cheap; ships first). `--update-baseline`
   records the current state.
2. `--base <git ref>` — run the same stages on a scratch export of that ref, then
   compare. Roughly doubles `mxcheck` cost; results are cacheable per commit.

## Existing pieces and gaps (checked against `main` at `ecdd9b7c`)

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
  Phase 2 decides; it is not "reuse as is".
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

## Finding identity: for which codes can the key be built at all?

The key `(code, document, element path)` is only as good as `mx check`'s text. In
`recheckCE0463` (`docker/check.go`): *"mx check's text output does not name a
CE0463's page, so two same-named widgets on different pages print the same line;
the lines are compared as counts."* For CE0463 the element identity is **not in
the output**. Phase 1 must therefore classify each code the parser sees:

1. **Located** — the line names document and element (e.g. CE0109 on a microflow
   activity). Key = `(code, document, path)`; `resolved` is a list.
2. **Document-only** — names the document but not the element. Key =
   `(code, document)` with multiplicity; matching is by count per key.
3. **Unlocated** — names neither (CE0463). Fallback: **count-based matching per
   distinct line**, exactly as `recheckCE0463` does today; `resolved` is a count
   delta, never a list, and `--format json` says so.

The CE0463 stale/persisting labelling from the re-check must survive unchanged as
a label on class 3 findings. The first question for phase 1 is the classification
table (code → class), built from real `mx check` output; stability under rename
or move (formerly the only question) applies only to class 1.

## Design

New package `cmd/mxcli/verify/` (orchestrator, no business logic):

```go
type Finding struct { Stage, Code, Document, DocumentID, Path, Message string }
type Stage interface {
    Name() string
    Run(ctx context.Context, p Project) ([]Finding, error)
}
```

- `Finding.Key()` is **per class** (see Finding identity): `Stage + Code + Document + Path` for located findings, down to count-per-line for unlocated ones. The lint stage's key additionally uses `ID` when present, as `baselineKey` does. Message is display only except where a stage's existing matcher (lint `Compare`) uses it, and then that is stated.
- Shared **`mx check` output parser** (`docker.ParseCheckOutput`) producing
  `[]Finding` from `mx` lines (`ERROR [CE0109] "…" at Microflow 'M.F', Activity …`).
  Also unblocks #1313 and #1312; lands as its own PR.
- Baseline file is versioned JSON, sorted for stable diffs, one section per stage
  (lint reuses its existing format inside it or keeps its own file — see questions).
- Infrastructure failure (no `mx`, no Docker/runtime for opt-in stages) is a
  distinct status `error`, never `passed`.

## Out of scope

- Fixing findings; running stages in parallel (cost order + early stop is the point);
  replacing `docker check`, `lint`, `test` or `playwright check`.
- `check --full` (#1258): `mxcheck` stage calls `docker check` until it exists,
  then may be swapped or made optional.

## Phasing

1. **Structured `mx check` findings** — parser + `docker.CheckFindings()`, tests
   against recorded `mx` output fixtures. Standalone PR; unblocks #1313.
2. **`mxcli verify`** with `check`, `lint`, `mxcheck`; `--baseline`,
   `--update-baseline`, `--format`, exit-code table, `--fail-fast`.
3. **`--base <git ref>`** via scratch export.
4. **Opt-in `build`, `test`, `smoke` stages.**

## Testing

- Unit: finding key normalisation per class (located / document-only / unlocated) (same finding, different line/message → same
  key); classification new/resolved/pre-existing; default run-all vs `--fail-fast`.
- Parser: golden `mx check` outputs (CE0109, CE0844, CE7811, CE0463 labelled).
- Control (per CLAUDE.md): a "nothing new" test paired with one that injects a
  finding and must fail.
- Output purity row in `json_output_purity_test.go`; exit-code table incl. several failing stages at once and "could not run".
- Executor-level orchestration with stub stages (same seam style as
  `mxCheckCmd`); integration (`-tags integration`) for real `mx`.
- Runtime stages follow `.claude/skills/verify-in-runtime.md`.

## Open questions

1. **Name.** `mxcli verify` alongside `mxcli playwright verify`: acceptable, or
   prefer `gate`/`validate`? (Much milder than first drafted: one existing
   `verify`, under a namespace.)
2. **One baseline file or one per stage?** One file is simpler for agents;
   per-stage keeps `lint --baseline` files valid unchanged.
3. **Code classification** (phase 1): which `mx check` codes are located,
   document-only, or unlocated, and is count-based matching acceptable for the
   unlocated ones?
4. **`--base` for MPR v1** — extract via `git show` of the single `.mpr`, or require v2?
5. **Should `check` stage fail on warnings,** or only errors?
6. **Exit-code encoding** — bitmask offsets, and whether to align with `check`'s
   and `lint`'s existing codes.
7. **Default stage set** — with run-all as the default, `mxcheck` costs ~8 s once
   per iteration; is on-by-default accepted?
