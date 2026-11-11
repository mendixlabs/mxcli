---
title: Nightly Agent Dev-Loop Performance Benchmark
status: partial
date: 2026-11-11
author: AI-assisted design
related:
  - PROPOSAL_agent_loop_efficiency
  - PROPOSAL_mcp_bson_benchmark
---

# Proposal: Nightly Agent Dev-Loop Performance Benchmark

**Status:** Partial — Phases 1 and 2 are implemented (see "Phase 1 as built", "Phase 2 as built")
**Date:** 2026-11-11

## Problem Statement

An agent building a Mendix app runs mxcli hundreds of times per session — the
four builds measured in
[PROPOSAL_agent_loop_efficiency](PROPOSAL_agent_loop_efficiency.md) made 301 to
936 invocations each — and waits for every one. That proposal's measurements
say where the waiting goes: in the largest session roughly 6 of 9 active hours
were spent waiting on commands rather than on the model, led by app restarts
and `run wait` (≥74 min), browser suites (~70 min), `docker check` (~25–35 min)
and microflow tests (~34 min), with a `run --local` boot at a 70–90 s median in
two other projects.

Those figures were taken by hand, from individual sessions, after the fact.
Nothing tracks them continuously, so a regression in any of them ships
unnoticed:

- The recent performance fixes each left a **budget test** behind: a unit test
  that counts unit-file reads or document decodes for one command on the PedApp
  fixture and fails above a bound (`unit_reads_per_command_pedapp_test.go`,
  `describe_decode_budget_pedapp_test.go`,
  `catalog_incremental_budget_pedapp_test.go`,
  `flow_build_decodes_2094_test.go`, the `reader_*` tests in `modelsdk/mpr`).
  They run on every PR and are noise-free, and this proposal keeps them as the
  first line of defence. They have four gaps by construction: they count reads
  and decodes, not time, so CPU, parsing, SQLite and process start-up are
  outside them; they run on a 375-unit fixture, while the defects they pin were
  measured on 3,400- to 4,753-unit projects; they exist only for the paths
  someone already found slow; and their bounds are deliberately loose, so a
  partial regression passes.
- The nightly workflow (`.github/workflows/nightly.yml`) runs correctness
  suites only. A change that adds ten seconds to a hot apply, or makes project
  open slower without one extra file read, passes every job.
- The Go benchmarks that exist (`modelsdk/codec/codec_bench_test.go`,
  `modelsdk/property/property_bench_test.go`, `BenchmarkRoundtripEntity`) are
  micro-benchmarks of one function. They are not run in CI, and they exclude
  what the agent actually waits for: process start, opening the MPR, parsing,
  reconciling and writing.
- `mxcli eval run` on the fixed `BENCH-001` brief
  (`docs/14-eval/eval-bench-001.md`) measures a whole agent session — model
  calls, tokens, wall time. It answers "what does a session cost", but it has
  an LLM in the loop: its own write-up calls a single run "one sample of a
  stochastic process with unmeasured variance". It cannot attribute a 15%
  change to a commit.
- `mxcli diag loop-report` and `diag session-report` explain a session that
  already happened. They are instruments for diagnosis, not a regression gate.

This proposal adds the missing piece: a **deterministic** nightly benchmark of
the tool-side time — the same commands an agent runs, on a fixed script, with
no model involved — reporting per step how long mxcli takes and how that
compares to the previous nightly, with enough noise control that a reported
regression is worth acting on.

It is the complement of the loop-efficiency work, not a replacement. That
proposal shows most of a session's *token* bill is the conversation being
re-read (`N × S`), which no tool-speed benchmark can see. This one covers the
wall-clock share that is the tool's, and the commands it identified as the
wall-time levers.

### Non-goals

- Measuring model quality, token use or number of agent turns. An LLM in the
  loop adds variance far larger than any tool regression, and `mxcli eval run`
  with `diag session-report` already covers that ground.
- Gating pull requests or the nightly release on timing.
- Benchmarking mxbuild or the Mendix runtime as such. Their time is recorded,
  because the agent waits for it, but reported separately from mxcli's own.

## BSON Structure

Not applicable. The benchmark adds no document types and changes no write
path; it drives the existing CLI from outside. The generated fixture is
produced through ordinary MDL scripts, so it exercises the same writers as any
user project.

## Proposed Design

No MDL syntax is added. The feature is a benchmark driver, a fixture
generator, a nightly job and a results branch.

### Principle: measure at the CLI boundary

Each measured step is **one mxcli process**, started the way an agent starts
it (`mxcli exec script.mdl -p app.mpr`, `mxcli -p app.mpr -c "…"`). The
recorded time is the process's wall-clock time. This deliberately includes
start-up and project open, which a `testing.B` benchmark excludes and which
dominate short commands.

The session is a fixed set of committed MDL scripts — deterministic, no LLM.

### The scripted session

| Phase | Step | What it times | Needs |
|---|---|---|---|
| Authoring | `new` | `mxcli new` with `--skip-build`, mxbuild already cached | mxbuild |
| | `domain` | one `exec` creating ~50 entities with attributes and associations | — |
| | `microflows` | one `exec` creating ~100 microflows | — |
| | `pages` | one `exec` creating ~50 pages (overview + edit pairs) | — |
| | `single-edit` | one `exec` changing one microflow — the most frequent agent action | — |
| | `check` | `mxcli check script.mdl -p app.mpr --references` | — |
| Reading | `open` | a trivial `-c` command: pure start-up + project open | — |
| | `describe` | `describe` of one entity, one microflow, one page | — |
| | `catalog` | `refresh catalog;` | — |
| | `catalog-full` | `refresh catalog full;` | — |
| | `catalog-after-edit` | `refresh catalog full;` after `single-edit` | — |
| | `query` | a catalog `select` and a search | — |
| Bulk change | `bulk-alter` | one `exec` altering several hundred documents | — |
| | `rename` | renaming an entity referenced from many microflows and pages | — |
| | `re-exec` | re-running the `domain`/`microflows`/`pages` scripts unchanged | — |
| Validate | `mx-check` | `mxcli docker check` | mxbuild |
| | `build` | one full build | mxbuild |
| Running app | `boot` | `mxcli run --local --detach --db-type hsqldb` until ready | runtime |
| | `hot-apply-flow` | microflow change, then `mxcli run wait` | runtime |
| | `apply-entity` | entity change (schema update), then `mxcli run wait` | runtime |
| | `page-check` | `mxcli playwright check` on one page | runtime |
| Tests | `test-cold` | `mxcli test --local --db-type hsqldb` on a small suite | runtime |
| | `test-attach` | `mxcli test --attach` against the app from `boot` | runtime |

The step list follows the verbs the measured sessions actually ran (`exec`,
`-c`, `check`, `run wait`, `docker check`, `test`). Each step's time is also
reported multiplied by a fixed per-session call count taken from those
sessions, giving one **session-weighted total**: a step that is 50 ms slower
and runs 200 times outranks one that is 2 s slower and runs once.

`re-exec` doubles as a correctness canary: per
[ADR-0008](../13-decisions/0008-identity-and-idempotence.md) it must write
nothing, so a non-zero write count there is a defect, not a slowdown.

### Two project sizes

The steps marked "—" run against two projects:

- **small** — the blank app that `mxcli new` produces.
- **large** — a generated app of a few thousand documents.

Project open, catalog refresh and reference checks scale with project size.
Reporting both sizes shows the *slope*, which is what an agent feels on a real
customer project and what a blank-app benchmark hides.

### The generated fixture

A generator emits MDL for a parameterised app — `M` modules, each with `E`
entities, `F` microflows and `P` pages, cross-module associations and calls at
a fixed density — and executes it against a blank project.

- **Deterministic.** Names and structure derive from the parameters and a fixed
  seed, so two runs produce the same model and the deterministic counters below
  are comparable night to night.
- **Not committed.** Only the generator and its parameters are in the repo; the
  `.mpr` is built in the job. Building it is itself the bulk-create measurement.
- **Realistic shape, not just volume.** The templates are taken from the
  existing example apps under `mdl-examples/` (CRUD pages, validation
  microflows, list processing), so the documents have typical size and
  reference fan-out rather than being thousands of empty shells.
- **Sized by parameter.** The nightly uses one "large" preset; a local run can
  pass smaller or larger values to study scaling.

Caveat to keep in view: a fixture mxcli created has `GUID == $ID` everywhere
and contains only documents mxcli can write. It is a valid subject for timing,
and an invalid one for any identity-preservation claim (see the GUID section
of `CLAUDE.md`).

### Noise control

GitHub-hosted runners differ by 10–20% from run to run, so comparing tonight's
absolute time to last night's produces false alarms. Three mechanisms:

1. **A/B against the previous nightly on the same runner.** The job downloads
   the binary from the existing `nightly` release (published by the `nightly`
   job) before that release is replaced, and runs both binaries interleaved —
   A, B, A, B — on the same machine. The reported figure is the ratio
   new/old. Runner speed cancels out. This applies to every step whose time is
   mxcli's own.
2. **Median of five.** Each step runs five times against a fresh copy of the
   project; the median is reported and the spread recorded, so a noisy step is
   visible as noisy.
3. **Deterministic counters.** Alongside time, record values that do not depend
   on the machine: unit files read (the backend's `UnitFileReads`, the counter
   the existing budget tests assert on), units offered and written
   (`backend.WriteStats`), bytes written to the project, and output size. These can be compared exactly
   between nights. A step that suddenly writes 400 units instead of 40 is a
   regression whether or not the runner was slow.

Steps dominated by mxbuild or the runtime (`mx-check`, `build`, `boot`,
`apply-*`, `test-*`) get the same A/B treatment but need more care in reading:
the external tool is identical in both arms and contributes most of the time
and most of the noise, so a real mxcli regression is a smaller fraction of a
noisier number. These are also the steps the measured sessions spend the most
wall time in, so they are not optional — their thresholds are simply set wider,
from their own measured noise floor (see Test Plan).

The driver times each process itself. mxcli's existing per-invocation session
log (`mdl/diaglog`, the source for `diag loop-report`) is collected alongside
as a cross-check and to split a step's time into its statements.

### Results and history

- **One JSON record per run**: commit, date, Mendix version, runner details,
  fixture parameters, and per step the median, spread, A/B ratio and counters.
- **Dedicated `benchmarks` branch.** The job commits each record as its own
  file (`results/YYYY/MM/<date>-<sha>.json`). The branch is an orphan: it holds
  only results, never merges into `main`, and one file per run means concurrent
  runs cannot conflict. `nightly.yml` already has `contents: write`.
- **Job summary.** A table in `$GITHUB_STEP_SUMMARY` with tonight's ratios,
  regressions highlighted.
- **Trend chart.** A page on the docs site reads the branch's records and plots
  each step over time. This is where slow drift — invisible night to night —
  shows up.

### Alerting

Reporting only at first. After roughly two weeks of data, set a per-step
threshold from the observed spread and have the job open (or update) one issue
when a step's A/B ratio exceeds it on two consecutive nights. A counter change
is flagged on the first night, since it carries no noise.

### Nightly job

A new `bench` job in `nightly.yml`:

- runs on one Mendix version (the newest in the `test` matrix);
- is **not** in `needs:` of the `nightly` release job — the same arrangement as
  `upgrade-full-corpus` and `tunnel-seam-macos` — so a benchmark failure never
  holds back a release;
- downloads the previous nightly binary as its first step. The release job
  waits on the `test` matrix, which takes far longer, but the job should not
  rely on that ordering: if the download does not match an earlier commit, the
  A/B arm is skipped and the record says so.

### Local use

`make bench` runs the same driver against a locally built binary, optionally
with `BASELINE=<path-to-other-mxcli>` for an A/B, and prints the same table.
A regression reported by the nightly must be reproducible this way, or it
cannot be bisected.

## Implementation Plan

Delivered in phases; each is useful on its own.

### Phase 1 — driver, fixture, mxcli-only steps

The driver, the generator, the `benchmarks` branch, the job, and every step in
the table marked "—". No runtime needed, so the job is short and the A/B signal
is clean. This phase exists to get the machinery and its noise floor right on
the easy case; it is **not** where most of the measured wall time is.

#### Phase 1 as built

Implemented as `scripts/agentbench` (`make bench`), the `bench` job in
`nightly.yml`, and `MXCLI_STATS_FILE`. User-facing description:
`docs-site/src/internals/agent-benchmark.md`. Where it differs from the design
above, and what building it showed:

- **The base project is the committed PedApp fixture, not `mxcli new`.**
  Creating a blank project needs `mx create-project`, i.e. mxbuild. Starting
  from `testdata/pedapp` keeps Phase 1 free of mxbuild, as intended, and the
  job short. The `new` step moves to the phase that brings mxbuild into the
  job. PedApp is 370 units, so "small" is a small real app, not an empty one.
- **Both arms start from identical bytes with no caches.** The project copy
  leaves `.mxcli/` out, so each arm builds its own unit index and catalog. That
  added a step the design did not have, `open-cold`, the first command on a
  project.
- **The large fixture is seeded once, by the candidate only.** The seed is
  timed and recorded but has no A/B; the session that then runs on it does.
- **The large preset is about 900 units, not a few thousand.** Seeding is the
  reason: on the development machine 492 generated documents take about 90 s,
  while a 3,240-document seed took 4 min 46 s for its entities, 3 min 48 s for
  its microflows, and had not finished its pages after 15 minutes. The cost
  per statement grows with the size of the project, which is itself the kind
  of finding this benchmark exists for — and is visible at 900 units already:
  the same 50-page script takes 2.6 times as long there as on 370. The preset
  is two flags (`-large-modules`, `-large-entities`) and should grow as that
  cost comes down.
- **Counters (Open Question 1) are an environment variable.**
  `MXCLI_STATS_FILE` makes every project connection append one JSON line with
  its unit-file reads and its writes offered and landed. It turned out that
  the write count covers reconciled rewrites of stored units but not newly
  created documents, so the driver also counts changed unit files from outside,
  by digest. That count needs nothing from the binary and so exists for the
  baseline as well; the idempotence step asserts on it.
- **CPU time and peak memory are recorded alongside wall time.** CPU time is
  the steadier of the two ratios on a shared machine.
- **The ratio is the median of per-repetition pair ratios**, not the ratio of
  two medians, and the arm that goes first alternates.
- **Not built yet:** the trend chart, and the issue-opening alert. Both need
  records to exist first; the alert thresholds are to be set from the first
  weeks of data, as planned.

### Phase 2 — running app and tests

Adds `boot`, `hot-apply-flow`, `apply-entity`, `page-check`, `test-cold`,
`test-attach`, using `--db-type hsqldb` so the job needs no PostgreSQL service.
This is ordered second, ahead of validate/build, because restarts, waits and
tests are the largest measured share of command wall time.

#### Phase 2 as built

The running-app session of `scripts/agentbench`, switched on with
`-runtime-version` (`make bench BENCH_RUNTIME=11.13.0`) and run by the nightly
job. Steps: `boot`, `page-check`, `hot-apply`, `test-attach`, `apply-entity`,
`stop`, `test-cold`. Where it differs from the design, and what it showed:

- **The project comes from `mxcli new`.** The PedApp fixture cannot be built
  (it has no theme), so the `new` step deferred from Phase 1 arrives here, as
  preparation: timed once, by the candidate. The session's app is authored
  into it before the snapshot.
- **mxcli installs mxbuild and the runtime**, before anything is timed. The
  job adds JDK 21 and a Playwright Chromium.
- **The arms take turns** instead of alternating step by step, because a boot
  leaves a runtime up. A teardown stops each arm's app whatever happened.
- **A change to the running app is one step of two commands**, the `exec` and
  the `run wait`: that pair is what the agent waits through.
- **Runtime steps do get an A/B**, as the revised design said, and it is
  usable: with the same code in both arms, ratios ran from 0.92 to 1.04 over
  three repetitions on the development machine.
- **`test-attach` uses `--allow-call`**, the path that runs a test in the
  running app without building it. The inject-and-build path of `--attach` is
  not measured separately.
- **First figures** (Mendix 11.13.0, 4 CPUs, 25 authored entities): boot 27 s,
  hot apply 7.7 s, entity change 19 s, cold test run 25 s, attached test run
  0.13 s, page check 3.1 s, `mxcli new` 37 s. They also settle a Phase 1
  caveat: the generated app builds and boots, so it is valid for mxbuild, not
  only for `mxcli check`.
- **One version.** The nightly pins the Mendix version the session was
  developed against. Open Question 8 (the licence session cap) did not show in
  these short sessions.

### Phase 3 — validate and build

Adds `mx-check` and `build`.

### Later, separately — a scheduled agent-level run

`mxcli eval run` on `BENCH-001` is the agent-level counterpart and already
exists. Running it on a schedule and committing its session report to the same
`benchmarks` branch would put both views on one chart. It needs model
credentials in CI and enough repetitions to have a variance, so it is its own
decision and not part of this proposal.

### Files to modify/create

| File | Change |
|------|--------|
| `scripts/agentbench/` (new, Go) | Driver: runs steps, interleaves A/B, takes medians, collects counters, writes the JSON record and the summary table |
| `scripts/agentbench/gen/` (new) | Generator: parameters → the session's MDL scripts and the large fixture's seed |
| `Makefile` | `bench` target |
| `.github/workflows/nightly.yml` | `bench` job; commit of the record to the `benchmarks` branch |
| `mdl/executor/process_stats.go` (new) | `MXCLI_STATS_FILE`: read and write counters per closed connection |
| `docs-site/src/internals/` | Page describing the benchmark and showing the trend chart |

## Version Compatibility

No version gating. The benchmark runs on one Mendix version per night. The
session scripts must stay within syntax that version supports; running a
second version is a matrix entry, not a code change.

## Test Plan

- **Driver unit tests** — median and ratio arithmetic; record schema; the A/B
  arm being skipped cleanly when no baseline binary is available.
- **Generator determinism** — generating twice with the same parameters yields
  byte-identical MDL.
- **Session validity** — the session scripts and a small generated fixture pass
  `mxcli check`, and after exec pass `mx check`, in an integration test, so the
  benchmark cannot silently measure a failing script. A step that exits
  non-zero fails the run rather than recording a (fast) time.
- **The benchmark detects a regression** — a control, per the repo's evidence
  rule: build a binary with a deliberate delay in one step and confirm the A/B
  ratio and the summary flag exactly that step. Without this, a benchmark that
  always reports "no change" is indistinguishable from a working one.
- **Noise floor** — run A/B with the *same* binary in both arms and record the
  ratio spread. This is the measured basis for the alert thresholds.

## Open Questions

1. **Counters on the CLI.** `backend.WriteStats` (`Offered`, `Written`) and
   the backend's `UnitFileReads` exist — the first drives the "Unchanged …"
   reporting, the second the budget tests — but this proposal has not found a
   machine-readable form of either on a command. The driver needs one — a flag
   or an environment variable that emits a final stats line. Bytes written can
   be measured from outside by diffing the project directory.
2. **Large-fixture size.** A few thousand documents is a guess at "typical
   customer project". The preset should be set from real numbers if they are
   available.
3. **Job duration.** Five interleaved repetitions of every step at two sizes
   must fit a reasonable budget (target: under 30 minutes for Phase 1). If the
   large fixture makes that impossible, reduce repetitions for the slowest
   steps rather than dropping the A/B.
4. **Baseline across a format change.** If a commit changes what a script
   writes, the old binary runs the same script against the same starting
   project, so the A/B stays valid — but if the session scripts use syntax the
   previous nightly does not have, the baseline arm fails. The driver should
   skip A/B for that step and say so, not fail the run.
5. **`mxcli new` in the session.** It downloads mxbuild and by default runs a
   build. The benchmark pre-caches mxbuild and passes `--skip-build` so the
   step times project creation only; whether that is the number worth tracking,
   or the full default `new`, is a judgement call.
6. **Chart hosting.** Whether the docs-site build may read from the
   `benchmarks` branch at build time, or the chart page should fetch the
   records client-side.
7. **Session weights.** The per-session call counts behind the weighted total
   come from four measured sessions of different kinds of work. They should be
   fixed in the driver and versioned in the record, so a change of weights is
   never read as a change of speed.
8. **Licence session cap.** The loop-efficiency proposal records, as an
   unverified lead, that many restarts were forced by the development licence's
   session cap. If true, a benchmark that boots, logs in and runs tests
   repeatedly may hit the same cap and measure it.
