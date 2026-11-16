---
title: mxcli verify — one gate that reports only what a change made worse
status: draft
date: 2026-11-16
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
gate must include `mx check` and stop early when a cheaper stage already fails.

## Proposed behaviour

```
mxcli verify -p app.mpr [--base <git ref> | --baseline <file>]
                        [--stages check,lint,mxcheck,build,test,smoke]
                        [--keep-going] [--json]
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

- Stages run in cost order and **stop at the first stage with new findings**;
  `--keep-going` runs all.
- Findings are classified **new / resolved / pre-existing** against the baseline,
  keyed by `(code, document, element path)` — never by message text or line
  number, which drift between runs (the problem #1313 describes).
- **Exit code:** 0 when nothing is new; otherwise a fixed per-stage code
  (documented table, treated as a contract: e.g. 10 check, 11 lint, 12 mxcheck,
  13 build, 14 test, 15 smoke; 2 for usage/infrastructure errors such as no `mx`
  found, so "gate failed" and "gate could not run" are never confused).
- **`--json`:** one object `{stages:[{name,status,durationMs,new:[],resolved:[],
  preExisting:int}], exit}`, so an agent reads one result instead of six formats.

### Baseline sources

1. `--baseline <file>` — saved findings (cheap; ships first). `--update-baseline`
   records the current state.
2. `--base <git ref>` — run the same stages on a scratch export of that ref, then
   compare. Roughly doubles `mxcheck` cost; results are cacheable per commit.

## Existing pieces and gaps (verified in code)

- `mdl/linter/baseline.go`: `WriteBaseline`/`LoadBaseline`/`Compare` give
  new/unchanged/fixed for lint. Reusable as is.
- `cmd/mxcli/docker/check.go`: `Check()` runs `mx` on a temp copy and **writes free
  text**; it returns only an error and an app-error count. **There are no
  structured `mx check` findings to baseline — this is the central gap.**
  `mxCheckCmd`/`resolveMxForCheck` are test seams worth keeping; the CE0463
  re-check labelling must survive.
- `cmd_check.go` supports `--format json` for script checks; a project-wide
  reference-check entry point needs a thin adapter.
- `test --local` and `playwright check` have their own reporting; each needs an
  adapter to the common finding shape.
- `diff-local --ref` already resolves git refs for mxunit diffs; exporting an MPR
  at a ref (v1 needs an extraction step, v2 is `mprcontents/`) is new.
- **Name clash:** `verify` is already a subcommand of `test` and of `playwright`
  (`playwright verify <file|dir>` runs test scripts). See open questions.

## Design

New package `cmd/mxcli/verify/` (orchestrator, no business logic):

```go
type Finding struct { Stage, Code, Document, Path, Message string }
type Stage interface {
    Name() string
    Run(ctx context.Context, p Project) ([]Finding, error)
}
```

- `Finding.Key()` = `Stage + Code + Document + Path`; message is display only.
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
   `--update-baseline`, `--json`, exit-code table, `--keep-going`.
3. **`--base <git ref>`** via scratch export.
4. **Opt-in `build`, `test`, `smoke` stages.**

## Testing

- Unit: finding key normalisation (same finding, different line/message → same
  key); classification new/resolved/pre-existing; early stop; exit-code table.
- Parser: golden `mx check` outputs (CE0109, CE0844, CE7811, CE0463 labelled).
- Control (per CLAUDE.md): a "nothing new" test paired with one that injects a
  finding and must fail.
- Executor-level orchestration with stub stages (same seam style as
  `mxCheckCmd`); integration (`-tags integration`) for real `mx`.
- Runtime stages follow `.claude/skills/verify-in-runtime.md`.

## Open questions

1. **Name.** `verify` collides with `test verify` / `playwright verify`. Keep
   `mxcli verify` and document the distinction, or use `gate`/`validate`?
2. **One baseline file or one per stage?** One file is simpler for agents;
   per-stage keeps `lint --baseline` files valid unchanged.
3. **Baseline normalisation for `mx check`:** is `(code, document, element path)`
   stable enough when elements are renamed/moved, or is the element `$ID` needed
   (as `lint` uses `documentId`)?
4. **`--base` for MPR v1** — extract via `git show` of the single `.mpr`, or require v2?
5. **Should `check` stage fail on warnings,** or only errors? (Lint severities
   already differ.)
6. **Default stage set:** is `mxcheck` on by default given its 8 s, or is that
   exactly the cost the gate exists to pay?
