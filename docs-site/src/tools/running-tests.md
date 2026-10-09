# Running Tests

The `mxcli test` command executes test files and reports results.

## Prerequisites

Tests need a Mendix runtime to execute against. There are two ways to get one,
and **Docker is only needed for the first**:

- **Docker** — the container path. Requires a running Docker daemon.
- **`--local`** — mxcli's own runtime, no daemon involved. It uses its own ports
  (8081/8091) and its own `<project>_test` database, so a `mxcli run --local`
  dev loop can keep serving the same project while tests run.

`--local` provisions that scratch database on PostgreSQL by default. On a machine
without PostgreSQL — where it would stop with *no local PostgreSQL superuser
available to create the role/database* — pass `--db-type hsqldb` to boot on the
runtime's built-in file database instead: no server and no provisioning. It
takes the same values as `run --local --db-type` (`postgresql`, the default, or
`hsqldb`), keeps the `<project>_test` scratch name so it never shares files with
a `run --local --db-type hsqldb` dev loop, and is refused with `--attach` and on
the Docker path, which do not boot a database of their own.

```bash
mxcli test tests/ -p app.mpr --local --db-type hsqldb
```

A `--local` run boots the app with the same **constant values** `mxcli run --local`
uses — the project configuration's shared overrides layered over each constant's
default — and prints what it applied. Use `--configuration <name>` to choose
between several, and `--constant Module.Name=value` (repeatable) to set one for
this run only — it wins over everything and is never written anywhere. For a
value that should persist on this machine without being committed, use
`mxcli constant set` (see [Constant values](#constant-values) below). A constant
the project does not declare is refused before anything boots. `--attach` takes
none of these, since it inherits the constants of the app it attaches to. This
matters whenever a test asserts on something a constant feeds: the two modes
used to disagree silently.

`--local` also downloads what it needs on first use. To pre-cache it:

```bash
mxcli setup mxbuild -p app.mpr
```

The `mx` binary, when you need it directly:

| Environment | Path |
|-------------|------|
| Dev container | `~/.mxcli/mxbuild/{version}/modeler/mx` |
| Repository | `reference/mxbuild/modeler/mx` |

## Constant values

Highest layer wins:

| Layer | Set with | In git? |
|---|---|---|
| this run | `--constant Module.Name=value` | no |
| this machine | `mxcli constant set Module.Name value` | no — gitignored, 0600 |
| this configuration | `alter settings constant … in configuration 'X'` | yes |
| default | `create constant … default '…'` | yes |

`mxcli constant list` shows the winner for each constant and which layer set it,
masking machine-local values unless `--show-values` is passed.

A machine-local value normally takes effect at the next boot. `mxcli constant
set … --apply` pushes it into a `mxcli run --local` that is already up, as
`update_configuration` followed by `reload_model` — both are needed, since the
first call only stages the change.

## Basic Usage

```bash
# Run all tests in a directory, including its subfolders
mxcli test tests/ -p app.mpr

# Run a specific test file
mxcli test tests/sales.test.mdl -p app.mpr

# Run markdown test files
mxcli test tests/integration.test.md -p app.mpr
```

## Choosing a mode

|  | Boot cost per run | Database | Needs Docker |
|---|---|---|---|
| `mxcli test …` | container restart | the container's | yes |
| `--local` | ~30s | `<project>_test` | no |
| `--local --watch` | ~30s once, then ~2s | `<project>_test` | no |
| `--attach` | none | **the running app's** | no |

```bash
# No Docker daemon needed
mxcli test tests/ -p app.mpr --local

# Keep the runtime warm; re-runs on every test or model change (Ctrl-C to stop)
mxcli test tests/ -p app.mpr --local --watch

# Attach to an app you already have running — no boot at all
mxcli run  --local --test-endpoint -p app.mpr    # terminal 1
mxcli test tests/ -p app.mpr --attach            # terminal 2
```

`--local` builds with the same mxbuild `run --local` resolves: Studio Pro's
bundled one on macOS and Windows, the cached CDN download on Linux (the CDN
publishes Linux binaries only). To point it somewhere else — a Studio Pro
installed outside `Program Files`, say — pass `--mxbuild-path`, or set
`MXCLI_MXBUILD_PATH` for a wrapper or CI step that cannot add a flag. The flag
wins when both are set.

```bash
mxcli test tests/ -p app.mpr --local --mxbuild-path "C:\Program Files\Mendix\11.11.0\modeler\mxbuild.exe"
```

`--watch` is the everyday loop: edit a test *or* the microflow under test, and
the verdict lands in about two seconds.

`--attach` skips even the first boot, at one cost worth knowing: the tests run
against the running app's database rather than a scratch one, so they can leave
data behind in the app you are looking at. It needs the dev loop to have been
started with `--test-endpoint`, because the endpoint's handler is registered by
the after-startup microflow and cannot be added to an app that is already up.

## How tests execute

**`--local` — the test endpoint.** One microflow is generated per test, plus a
Java action that registers a token-guarded HTTP endpoint. The app boots once;
startup only registers the endpoint and runs no tests. Each test is then invoked
by name over HTTP and returns its verdict in the response.

Two consequences when reading a failing run:

- A test that throws fails **only itself** and is reported as an error with the
  root-cause message; the next test still runs.
- Results are **returned**, not recovered from the runtime log.

The endpoint executes microflows under a system context, so it is gated: it is
not registered at all without a per-run token in the runtime's environment,
every request must present that token, non-loopback callers are refused, and it
will only ever invoke the generated `MxTest.Test_*` microflows. The token is
never written into your project.

### A run leaves the project byte-identical

`mxcli test` injects an `MxTest` module, builds, runs, and removes it again. When
cleanup succeeds the `.mpr` is restored **byte-for-byte**, so `git status` is
clean after a run and a "run the tests, then assert the tree is clean" CI step
holds.

Restoring the model is not enough on its own: every unit write stamps a fresh
UUID into the `.mpr`'s `_Transaction` row, and the inject/remove cycle relays
SQLite's pages, so the file differs even once its content matches. Version
control compares bytes, and a `.mpr` diff is opaque.

The restore is declined when cleanup failed, or when the `mprcontents/` tree
changed during the run — in both cases the project is not in the state the
snapshot describes, and putting the old file back would hide that.

### `@expect`: what an assertion may say, and what happens when it cannot

An `@expect` is a **Mendix expression that must evaluate to true**. Any
expression the Mendix engine accepts works — built-in functions, every
comparison operator, `and` / `or` / `not(...)`, attribute paths and enumeration
values:

```mdl
/**
 * @test the dealt board is a full grid with blanks
 * @expect length($result) = 81
 * @expect find($result, '0') >= 0
 * @expect substring($result, 0, 9) != substring($result, 9, 18)
 */
$result = CALL MICROFLOW Sudoku.SUB_BlankSquares(Grid = $solved);
/
```

`<>` is accepted and rewritten to `!=`, which is the spelling Mendix's
expression engine accepts — `<>` fails the build with CE0117.

**An assertion the runner cannot compile is an ERROR, not a pass.** Unknown
functions, wrong arity, unbalanced parentheses, and expressions that produce a
value rather than a condition are each reported against the test that carries
them, and an ERROR counts with the failures, so the run exits non-zero:

```
ERROR  a self-evident falsehood
       @expect randomInt($result) = 1: randomInt() is not a Mendix expression
       function at column 1 ("randomInt")
```

A failing assertion reports **what came back**, not only what was wanted:

```
FAIL  the board is 81 squares
      expected length($result) = 81, actual: 27
```

The observed value is omitted rather than guessed when nothing in the assertion
pins down its type (`@expect $a = $b`, where both sides are variables). Mendix's
expression engine is typed, and a wrong guess would fail the build rather than
the test.

### A file that does not parse is one ERROR, not a dead run

A malformed test file is reported the same way an uncompilable assertion is:
against itself, as an ERROR that counts with the failures. The tests in every
other file still run.

```
  PASS  the board is 81 squares (6ms, 2 assertions)
  ERROR  broken.test.mdl (file could not be parsed)
         test "first" is followed by another @test doc comment ("second") with
         no '/' separator between them
```

`--list` prints the same thing under the listing and exits non-zero, so a
directory that was only partly readable cannot look like a clean one. A path
that does not *exist* is still a hard error — that is a mistake in the
invocation, not a malformed test, and continuing would run a different suite
than the one that was asked for.

### A test that asserts nothing, and `@verify`

Every result line reports what the test actually checked:

```
  PASS  the board is 81 squares (6ms, 2 assertions)
  FAIL  the mix keeps the shared block (8ms, 1 assertion)
         expected length($result) = 81, actual: 27
  PASS  asserts nothing at all (4ms, no assertions)
------------------------------------------------------------
1 test(s) asserted nothing beyond "did not throw". Run with
--require-assertions to make that an error.
```

A test with no `@expect` and no `@throws` is a smoke test — it reports only that
the body did not throw. It still passes, because that is a legitimate thing to
write; what it may not do is look the same as a test with six assertions.
`--require-assertions` makes every vacuous test an ERROR for projects that want
CI to enforce it.

`@verify` asserts on the database instead of the return value — see below.

The JUnit report (`--junit`) carries the assertion count as a
`<property name="assertions">` on each case, and `classname`/`file` identify the
source test file so a failure in a multi-file run says where it lives.

### `@verify`: asserting on what the microflow wrote

`@expect` sees only what a microflow returned. Most Mendix microflows are side
effects, so `@verify` is how you assert on the rows one left behind — an OQL
query, a comparison, and the value it must satisfy:

```mdl
/**
 * @test dealing a board writes 81 cells
 * @cleanup none
 * @expect $result = 'ok'
 * @verify select count(*) as n from Sudoku.Cell = 81
 * @verify select count(*) as n from Sudoku.Cell where Value = 0 > 0
 */
$result = CALL MICROFLOW Sudoku.ACT_DealGame();
/
```

The query runs after the microflow returns, over the same admin API `mxcli oql`
uses. Three rules follow, each enforced rather than left to trip you up:

- **`@cleanup none` is required.** `rollback` is the default and undoes the
  test's writes before the query could see them, so a `@verify` on a rollback
  test is refused rather than run against the pre-test state.
- **The query must return exactly one row and one column** — aggregate it, or
  select one attribute of one row. Picking a cell out of a table would be a
  guess.
- **The expected value is a literal**: a number, a quoted string, `true`/`false`
  or `empty`. It is split off at the last comparison operator outside quotes and
  parentheses, so a `where Value = 5` inside the query is left alone.
- **Every selected column needs a name.** Mendix's OQL rejects a bare
  `select count(*)` with *"All OQL select columns must have a name"* — write
  `select count(*) as n`.

Operators are `=`, `!=` (`<>` accepted), `<`, `<=`, `>`, `>=`; numbers compare
numerically even though the runtime returns them as strings.

A `@verify` that cannot be evaluated — unknown entity, malformed OQL, a
non-scalar result, or something that was never a query — is an **ERROR**, never
a pass. A false one fails with the value that came back:

```
FAIL   dealing a board writes 81 cells
       expected select count(*) as n from Sudoku.Cell = 81, actual: 27
ERROR  cells exist for a game that does not
       @verify select count(*) as n from Sudoku.NoSuch = 1: OQL error: Unknown entity
```

`@verify` needs the test endpoint, so it works under `--local` and `--attach`.
The Docker / `--legacy-runner` path refuses a suite that uses it: its tests run
during boot, so there is no point at which to query the app.

### The app's own after-startup microflow

Boot registers the endpoint and then runs the project's own after-startup
microflow, so tests see the app in the state it really boots into. The run
prints which of the two happened, and `--skip-app-startup` opts out when a suite
wants an empty, deterministic baseline.

This keeps a suite behaving the same under `--local` and `--attach`. Note that
what the startup microflow writes is not covered by `@cleanup rollback` — it
runs at boot, outside any test's transaction.

### `@cleanup`: what happens to a test's data

`rollback` is the **default**, so a test's database writes do not survive it.
The endpoint opens a transaction around the call and rolls it back afterwards,
including when the test throws.

```mdl
/**
 * @test creating an order does not leak
 * @expect $result = 'ok'
 */
$result = CALL MICROFLOW Sales.CreateOrder(Amount = 100);
/

/**
 * @test seed a fixture the app should keep
 * @cleanup none
 */
$result = CALL MICROFLOW Sales.SeedCatalogue();
/
```

| Strategy | Effect |
|---|---|
| `rollback` (default) | The test's writes are rolled back, even if it throws |
| `none` | The writes commit and persist |

Rollback needs the test endpoint, so it applies to `--local` and `--attach`.
The Docker / `--legacy-runner` path runs tests inside the after-startup action
and has no context of its own to roll back, so it always commits.

A rollback that fails is reported per test and summarised at the end — data
left behind while the suite still says PASS is exactly what this is for.
`--verbose` tags every test `[rolled back]`, `[committed]` or
`[ROLLBACK FAILED]`. A misspelled strategy is a parse error, not a silent
commit.

**Docker — the after-startup runner.** The whole suite is compiled into the
project's after-startup microflow, the container is restarted, and results are
parsed out of its log. `--legacy-runner` selects this on a local run too.

Both paths restore the project when they finish, and both report loudly if that
restore fails — a modified project must never read as a clean pass.

## Isolated Testing Pattern

For manual testing or debugging, you can replicate the test runner's workflow:

```bash
# Create a fresh project
cd /tmp/test-workspace
~/.mxcli/mxbuild/<version>/modeler/mx create-project

# Apply MDL changes
mxcli exec script.mdl -p /tmp/test-workspace/App.mpr

# Validate
mxcli docker check -p /tmp/test-workspace/App.mpr
```

Expected output for a passing test:

```
Checking app for errors...
The app contains: 0 errors.
```

## Test Validation Checklist

Before marking tests as complete:

- MDL script executes without errors
- `mx check` passes with 0 errors
- Created elements appear correctly if inspected with `DESCRIBE`
- Security rules are valid (no CE0066 errors)

## Debugging Test Failures

### TypeCacheUnknownTypeException

```
The type cache does not contain a type with qualified name DomainModels$Index
```

This means a BSON `$Type` field uses the wrong name. Check the reflection data for the correct `storageName`.

### CE0066 "Entity access is out of date"

Security access rules don't match the entity's current structure. Make sure:
- All attributes in the entity are included in access rules
- Association member access is only on the FROM entity
- Run `GRANT` after adding new attributes

### Common Fixes

```bash
# Re-run with verbose output
mxcli exec script.mdl -p app.mpr --verbose

# Inspect the generated BSON
mxcli dump-bson -p app.mpr --doc "Module.EntityName"
```
