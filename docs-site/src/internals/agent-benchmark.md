# Agent Dev-Loop Benchmark

An agent building an app runs mxcli hundreds of times, and waits for each run.
The agent dev-loop benchmark measures that wait directly: a fixed session of
mxcli commands, **one process per command**, timed from outside. There is no
model in the loop, so the same commit gives the same session every time.

It complements two other things rather than replacing them:

- the **read and decode budget tests** in the unit suite, which fail a pull
  request that makes a command open more unit files than a bound. They are
  exact, but they count reads, on a 370-unit project, for the commands someone
  already found slow;
- **`mxcli eval run`** with `diag session-report`, which measures a whole agent
  session — model calls, tokens, wall time — and therefore varies from run to
  run.

Design and rationale: `docs/11-proposals/PROPOSAL_agent_devloop_benchmark.md`.

## Running it

```bash
make bench                                  # time this checkout's binary
make bench BASELINE=/path/to/other/mxcli    # A/B against another build
make bench BENCH_FLAGS='-sizes small -reps 3 -out record.json'
make bench BENCH_RUNTIME=11.13.0            # also the running-app session
```

Without `BENCH_RUNTIME` it needs no mxbuild and no Docker: every session
starts from a copy of the committed `testdata/pedapp` project. A default run with a baseline took 14
minutes on a 4-CPU machine; `-sizes small -reps 1` takes under a minute and is enough to see
whether a change moved anything.

To check a suspected regression, build the commit before it, keep that binary,
and pass it as `BASELINE`:

```bash
git stash && make build && cp bin/mxcli /tmp/mxcli-before && git stash pop
make bench BASELINE=/tmp/mxcli-before
```

## The session

The steps run in order against one project, each on the state the previous
ones left. The scripts are generated (`scripts/agentbench/gen`) and identical
on every machine.

| Step | Command | What it shows |
|---|---|---|
| `open-cold` | `-c "show modules"`, first command on the project | building the unit index |
| `open` | the same command again | the floor under every command: start-up and project open |
| `domain` | `exec`: 25 entities and their associations | |
| `microflows` | `exec`: 50 microflows | |
| `pages` | `exec`: 50 pages | |
| `describe` | `describe` of one entity, microflow and page | |
| `catalog` | `refresh catalog` | |
| `catalog-full` | `refresh catalog full` | |
| `query` | a `select` on the catalog | |
| `check` | `check --references` of a one-microflow script | |
| `single-edit` | `exec` of that script | the most frequent thing an agent does |
| `catalog-after-edit` | `refresh catalog full` after one change | incremental refresh |
| `bulk-alter` | `exec` re-creating all 50 microflows with a changed body | |
| `re-exec` | the same script again | must change nothing |
| `rename` | `rename entity`, with references in microflows and pages | |

It runs at two project sizes. **small** is PedApp as committed. **large** is
PedApp with a generated app executed into it first (the `seed-*` lines, run
once). The session's own scripts are the same at both sizes, so the *Scaling*
table at the end of the output shows what each command costs more purely
because the project around it is bigger.

## The scale session

The two sizes above top out below a thousand units. What a statement costs in
a project of thousands is a different question, and the one
[mendixlabs/mxcli#1842](https://github.com/mendixlabs/mxcli/issues/1842)
reported: `exec` of an update cost time linear in the project size, about
0.27 s more per statement for every 1,000 units, and `check` cost seconds per
run before it had looked at a statement. At a few hundred units both are
inside the noise.

**scale** is PedApp with 4,000 filler microflows executed into it (the
`seed-filler` line, run once; `-scale-microflows` changes the number). Each has
six activities and refers to nothing, so the project grows by exactly that
many units. The session then runs:

| Step | Command | What it shows |
|---|---|---|
| `check-cold` | `check --references` of one update | a check's cost per run, on a project whose files have not been read yet |
| `check-one` | the same again | a check's cost per run |
| `check-batch` | `check --references` of 100 updates | |
| `exec-batch` | `exec --no-check` of those 100 updates | the update path of the report: an existing flow, one text changed |
| `re-exec-batch` | the same script again | must change nothing |
| `new-batch` | `exec --no-check` of 100 new microflows | |

`-scale-batch` changes the 100. `exec` runs with `--no-check` so that the
check is timed in its own steps and not twice.

The **Per statement** table at the end divides each step by the statements in
its script, and then by the project's size in thousands of units. The last
column is the one to watch: a cost that follows the project shows there, and
one that does not stays near zero however large the project is. On v0.31.0 an
update measured 307 ms per statement at 2,374 units, about 130 ms per
statement per 1,000 units; on the build that fixed it, 15 ms and 6 ms.

`BENCH_FLAGS='-sizes small,large'` leaves the session out.

## The running-app session

`BENCH_RUNTIME=<Mendix version>` adds a fourth session: validating and building
the project with mx, and what an agent waits for once the app is up. Measured sessions spend more wall time here than in every
mxcli-only command together.

| Step | Commands | What it shows |
|---|---|---|
| `mx-check` | `docker check` | validation by mx itself, on a temporary copy of the project |
| `build` | `docker build --skip-check` | the full build to a deployable package |
| `boot` | `run --local --watch --detach --db-type hsqldb --test-endpoint --allow-call` | until the app serves: build, runtime start, schema |
| `page-check` | `playwright check` | a text verdict for the home page |
| `hot-apply` | `exec` of a one-microflow change, then `run wait` | a behavioural change, live without a restart |
| `test-attach` | `test --attach` | a test run in the running app, nothing built |
| `apply-entity` | `exec` adding an attribute, then `run wait` | a structural change: runtime restart and schema update |
| `stop` | `run stop` | |
| `test-cold` | `test --local --db-type hsqldb` | a test run that boots its own runtime |

It differs from the other sessions in four ways:

- **The project is created by `mxcli new`**, with the session's entities,
  microflows and pages authored into it. PedApp cannot be built — the fixture
  carries no theme. `new` and the authoring are timed once, by the candidate
  (the `new` and `author` lines).
- **mxcli installs what it needs.** mxbuild and the runtime for the version
  are downloaded when they are not cached, before anything is timed. The
  machine has to bring JDK 21, and node with the `playwright` package and a
  Chromium for `page-check`; `BENCH_FLAGS=-page-check=false` leaves that step
  out.
- **The arms take turns.** A boot leaves a runtime up, and two at once would
  share the ports and the processor they are timed on. So one binary runs its
  whole session and is stopped, then the other. The pairing is by repetition,
  minutes apart instead of seconds.
- **Read the wall ratio only.** The time is spent in mxbuild and the runtime,
  which are the same in both arms; the CPU column covers the mxcli process
  alone and says little here. A real mxcli regression is a smaller share of a
  noisier figure than in the other sessions — with the same code in both arms
  the ratios ran from 0.92 to 1.04 over three repetitions.
- **Neither `mx-check` nor `build` needs a Docker daemon**, despite the
  command names: both run mx and MxBuild directly.

`boot` and `stop` each report 7 files changed: the test endpoint's documents,
added when the app starts and removed when it stops.

The database is the runtime's built-in HSQLDB, so no PostgreSQL is needed.
Figures are for one Mendix version and do not carry over to another.

## Reading the output

With a baseline, each step has these columns:

- **new / old** — the median wall time of each binary.
- **wall ×** — new divided by old. This is the figure to act on.
- **CPU ×** — the same ratio for user plus system CPU time. CPU time is less
  disturbed by a busy machine than wall time, and blind to waiting on the disk.
  A step that is slower in both columns is slower; one that is slower in wall
  time only is more often noise or I/O.
- **reads** — unit files the command read, as counted by mxcli itself.
- **files changed** — unit files added, rewritten with different bytes, or
  removed, measured by comparing the project before and after the command.

The last two do not depend on the machine at all. A difference between the
arms is exact, and is flagged `counts-changed` whatever the time did.

### Why a ratio and not a time

A shared CI runner varies from run to run by more than most regressions, so
tonight's absolute time cannot be compared with last night's. The benchmark
therefore runs the two binaries **interleaved on the same machine**, one step
at a time, each on its own copy of the project, and alternates which goes
first. The ratio is the median over the repetitions of each pair's ratio, so a
slow minute on the machine slows both halves of a pair and cancels.

Running the same code in both arms gives the noise floor. On the machine this
page was written on (4 CPUs, 5 repetitions), most steps stayed within ±3 % and
the widest were 0.90 and 1.12; the session-weighted total read × 0.93 and
× 0.98. The default flag threshold is × 1.20, and a single night just beyond
it is worth a second run before it is worth an investigation.

### The session-weighted total

Each step's median is multiplied by how often a real session runs that kind of
command, and summed. The weights are rounded from the sessions measured in
`PROPOSAL_agent_loop_efficiency.md`: a one-document `exec`, a `check` and a
read each stand for many calls, the bulk steps for themselves. A record
carries `weights_version`, so a change of weights is never read as a change of
speed.

### Flags

| Flag | Meaning |
|---|---|
| `slower` / `faster` | wall ratio beyond the threshold |
| `counts-changed` | the two binaries read or changed a different number of files |
| `counts-unstable` | one binary's counts differed between repetitions — a deterministic session should never show this |
| `wrote-on-re-exec` | re-running a settled script changed a file. This is a defect ([Idempotent Writes](idempotent-writes.md)), and the only finding that makes the run exit non-zero besides a failing command |

A baseline that cannot run a step — an older build that does not know a
statement the session uses — stops there. The steps before keep their ratio;
it and the later ones have none, because the two projects no longer hold the
same thing.

## The counts on any command

The read and write counts come from an environment variable that works for
any mxcli command:

```bash
MXCLI_STATS_FILE=/tmp/stats.jsonl mxcli exec bulk-alter.mdl -p app.mpr
cat /tmp/stats.jsonl
# {"unit_file_reads":73,"writes_offered":49,"writes_written":49}
```

One line is appended per project connection closed, so a command that
reconnects, or starts child mxcli processes, appends several. `writes_written`
counts stored units rewritten after reconciliation; a newly created document
is not in it, which is why the benchmark also counts changed files from
outside.

## The nightly job

The `bench` job in `.github/workflows/nightly.yml` downloads the previous
nightly release's binary, runs all three sessions against it, writes the table to
the job summary, and commits the JSON record to the `benchmarks` branch as
`results/YYYY/MM/<timestamp>-<sha>.json`. That branch holds records only and
is never merged.

The job does not gate the nightly release. It fails only when a command of
a session fails or a `wrote-on-re-exec` is found. The running-app session runs
last, and a session that fails does not take the completed ones with it: their
record is still written and committed.

## What it does not measure

- **A check or build after a change** — `mx-check` and `build` run once, on
  the project as authored, before the app boots.
- **A PostgreSQL-backed app** — the running-app session uses HSQLDB.
- **`mxcli new` as an A/B** — it is timed once, by the candidate, as a trend
  figure.
- **Tokens and model calls** — see `mxcli eval run`.
