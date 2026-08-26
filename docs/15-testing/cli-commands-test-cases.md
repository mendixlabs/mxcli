# CLI Commands Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/386)

Covers: docker, test, playwright, eval, init, setup, auth, marketplace, widget, bson, fmt, new, serve, lsp, tui, check, report.

## Setup

> See [AGENT-TESTING.md](./AGENT-TESTING.md) for build, execution methods, and verification patterns.

**Prerequisites:** Docker Desktop must be running. Commands `docker`, `test`, and `playwright` require it.

---

## 1. mxcli docker

### 1.1 Init Docker configuration

```bash
mxcli docker init -p "$MPR"
```

**Expected:** Creates `.docker/docker-compose.yml`, `.env.example`, and `.env` in the project directory. Exit code `0`.

### 1.2 Build Docker image

```bash
mxcli docker build -p "$MPR"
```

**Expected:** Builds image from generated Dockerfile. Exit code `0`.

### 1.3 Run container

```bash
mxcli docker run -p "$MPR" --port 8080 --admin-port 8090
```

**Expected:** Starts Mendix runtime container. App reachable at `http://localhost:8080`. Admin at `http://localhost:8090`. Exit code `0`.

### 1.4 Docker Compose up

```bash
mxcli docker up -p "$MPR"
```

**Expected:** Starts services via `docker-compose up -d`. Exit code `0`.

### 1.5 Docker Compose down

```bash
mxcli docker down -p "$MPR"
```

**Expected:** Stops and removes containers. Exit code `0`.

### 1.6 Run MxBuild project validation

```bash
mxcli docker check -p "$MPR"
```

**Expected:** Runs `mx check` (MxBuild project validation) inside Docker. Prints errors and warnings. Exit code `0` if no errors, `1` if errors found.

### 1.7 View logs

```bash
mxcli docker logs -p "$MPR"
```

**Expected:** Streams container logs to stdout. Exit code `0`.

### 1.8 Container status

```bash
mxcli docker status -p "$MPR"
```

**Expected:** Prints container state, ports, uptime. Exit code `0`.

### 1.9 Open shell

```bash
mxcli docker shell -p "$MPR"
```

**Expected:** Opens interactive shell inside the running container.

### 1.10 Reload runtime

```bash
mxcli docker reload -p "$MPR"
```

**Expected:** Rebuilds and restarts the runtime without recreating the container. Exit code `0`.

---

## 2. mxcli test

### 2.1 Run all tests in directory

```bash
mxcli test tests/ -p "$MPR"
```

**Expected:** Discovers `.test.mdl` and `.test.md` files. Injects TestRunner microflow, builds via MxBuild, runs in Docker. Prints pass/fail summary. Exit code `0` if all pass, `1` if any fail.

### 2.2 Run single test file

```bash
mxcli test tests/my-flow.test.mdl -p "$MPR"
```

**Expected:** Runs only the specified test file. Exit code `0` on pass.

### 2.3 List tests without running

```bash
mxcli test tests/ -p "$MPR" --list
```

**Expected:** Prints test names from `@test`/`@expect` annotations. Does not build or run. Exit code `0`.

### 2.4 JUnit output

```bash
mxcli test tests/ -p "$MPR" --junit report.xml
```

**Expected:** Writes JUnit XML to `report.xml`. Exit code reflects pass/fail.

### 2.5 Skip build

```bash
mxcli test tests/ -p "$MPR" --skip-build
```

**Expected:** Skips MxBuild step. Uses existing build artifacts. Exit code `0` on pass.

### 2.6 Verbose output

```bash
mxcli test tests/ -p "$MPR" --verbose
```

**Expected:** Prints detailed output per test case including microflow execution logs.

### 2.7 Custom timeout

```bash
mxcli test tests/ -p "$MPR" --timeout 120
```

**Expected:** Fails tests that exceed 120 seconds. Exit code `1` on timeout.

### 2.8 Color output

```bash
mxcli test tests/ -p "$MPR" --color
```

**Expected:** Forces colored output even when piped. Green for pass, red for fail.

---

## 3. mxcli playwright verify

### 3.1 Run verification scripts

```bash
mxcli playwright verify tests/e2e/ -p "$MPR"
```

**Expected:** Discovers `.test.sh` scripts. Launches app in Docker, runs each script. Captures screenshots on failure. Exit code `0` if all pass.

### 3.2 List verification scripts

```bash
mxcli playwright verify tests/e2e/ -p "$MPR" --list
```

**Expected:** Prints discovered `.test.sh` filenames. Does not run. Exit code `0`.

### 3.3 JUnit output

```bash
mxcli playwright verify tests/e2e/ -p "$MPR" --junit pw-report.xml
```

**Expected:** Writes JUnit XML. Screenshots attached as test artifacts.

### 3.4 Verbose mode

```bash
mxcli playwright verify tests/e2e/ -p "$MPR" --verbose
```

**Expected:** Prints browser console logs and script output per test.

---

## 4. mxcli eval

### 4.1 Check evaluation results

```bash
mxcli eval check
```

**Expected:** Runs AI correctness evaluation. Prints pass/fail per check. Exit code `0` if all pass.

### 4.2 List evaluations

```bash
mxcli eval list
```

**Expected:** Lists available evaluation suites. Exit code `0`.

---

## 5. mxcli init

### 5.1 Scaffold AI/IDE config

```bash
mxcli init -p "$MPR"
```

**Expected:** Creates `CLAUDE.md`, `.vscode/settings.json`, and other AI config files in the project directory. Exit code `0`.

### 5.2 Init in empty directory

```bash
mkdir /tmp/empty-project && mxcli init -p /tmp/empty-project
```

**Expected:** Exit code `0`. The `-p` flag does not validate that the path contains an `.mpr` file — command succeeds regardless. **Known behavior:** no MPR validation on init.

---

## 6. mxcli setup

### 6.1 Setup MxBuild

```bash
mxcli setup mxbuild
```

**Expected:** Downloads and installs MxBuild for the project's Studio Pro version. Exit code `0`.

### 6.2 Setup MxRuntime

```bash
mxcli setup mxruntime
```

**Expected:** Downloads and installs MxRuntime. Exit code `0`.

### 6.3 Setup mxcli itself

```bash
mxcli setup mxcli
```

**Expected:** Updates mxcli to latest version. Exit code `0`.

---

## 7. mxcli auth

### 7.1 Login (interactive)

```bash
mxcli auth login
```

**Expected:** Prompts for PAT (or opens browser). Stores token locally. Prints confirmation. Exit code `0`.

### 7.1b Login (non-interactive)

```bash
mxcli auth login --token "$MENDIX_PAT"
```

**Expected:** Validates token against marketplace API. Stores credential. Prints confirmation. Exit code `0`.

### 7.2 Check status (not authenticated)

```bash
mxcli auth status
```

**Expected:** Reports "no credential for profile". Exit code `1`.

### 7.3 List accounts

```bash
mxcli auth list
```

**Expected:** Lists stored credentials. Exit code `0`.

### 7.4 Logout

```bash
mxcli auth logout
```

**Expected:** Removes stored token. Prints "Removed profile". Exit code `0` (succeeds even when not logged in).

### 7.5 Status after logout

```bash
mxcli auth logout && mxcli auth status
```

**Expected:** Status reports "Not logged in". Exit code `1`.

---

## 8. mxcli marketplace

> **Prerequisite:** Requires authentication (`mxcli auth login`). Commands fail with "no credential" error without auth.

### 8.1 Search modules

```bash
mxcli marketplace search "email" --limit 5
```

**Expected:** Prints table with ID, TYPE, PUBLISHER, SUPPORT, LATEST, NAME columns. Exit code `0`.

### 8.2 Module info

```bash
mxcli marketplace info 170
```

**Expected:** Prints module details: Content ID, Type, Publisher, Support, Categories, Latest version, Min Mendix, Published date. Exit code `0`.

> **Note:** Argument is a numeric content ID (not a name). Get IDs from `marketplace search`.

### 8.3 Search with JSON output

```bash
mxcli marketplace search "atlas" --limit 2 --json
```

**Expected:** JSON array with items containing contentId, publisher, type, supportCategory, latestVersion. Exit code `0`.

### 8.4 Search with no results

```bash
mxcli marketplace search "zzzznonexistent"
```

**Expected:** Empty result or "No results found." Exit code `0`.

---

## 9. mxcli widget

### 9.1 List widgets

```bash
mxcli widget list -p "$MPR"
```

**Expected:** Lists all custom widgets in the project with name and version. Exit code `0`.

### 9.2 Extract widget

```bash
mxcli widget extract --mpk path/to/widget.mpk
```

**Expected:** Extracts widget package contents. Exit code `0`.

### 9.3 Init new widget

```bash
mxcli widget init MyNewWidget
```

**Expected:** Scaffolds widget project structure. Exit code `0`.

### 9.4 Generate widget docs

```bash
mxcli widget docs -p "$MPR"
```

**Expected:** Generates documentation for all custom widgets. Exit code `0`.

---

## 10. mxcli bson

### 10.1 Dump MPR contents

```bash
mxcli bson dump -p "$MPR" --list
mxcli bson dump -p "$MPR" --object "Name"
```

**Expected:** `--list` prints object names in the MPR. `--object` dumps a specific named object. There is no simple "dump all" mode. Exit code `0`.

### 10.2 Discover document types

```bash
mxcli bson discover "$MPR"
```

**Expected:** Lists all document types and counts found in the MPR. Exit code `0`.

### 10.3 Compare objects within or across projects

```bash
# Compare objects within the same project by type
mxcli bson compare -p "$MPR" --type DomainModels$DomainModel

# Compare across two projects
mxcli bson compare -p "$MPR" --p2 "$APPS_DIR/FactoryManagement/FactoryManagement.mpr"
```

**Expected:** Prints structural diff. Exit code `0` if identical, `1` if different.

### 10.4 Dump nonexistent file

```bash
mxcli bson dump /tmp/nonexistent.mpr
```

**Expected:** Error: file not found. Exit code `1`.

---

## 11. mxcli fmt

### 11.1 Format MDL file

```bash
mxcli fmt myfile.mdl
```

**Expected:** Outputs formatted MDL to stdout (does NOT write in-place). Exit code `0`.

### 11.2 Format already-formatted file

```bash
mxcli fmt myfile.mdl
```

**Expected:** No changes. Exit code `0`.

### 11.3 Format invalid file

```bash
mxcli fmt broken.txt
```

**Expected:** Exit code `0`. Input is echoed unchanged to stdout (no parse error).

---

## 12. mxcli new

### 12.1 Create a new Mendix project

```bash
mxcli new my-app --version 10.24.15
```

**Expected:** Scaffolds a complete Mendix project named `my-app` for the specified Studio Pro version. Exit code `0`.

### 12.2 Create project without version

```bash
mxcli new my-app
```

**Expected:** Creates project using the latest available Studio Pro version. Exit code `0`.

---

## 13. mxcli serve

### 13.1 Start HTTP server

```bash
mxcli serve -p "$MPR" --port 9000 &
curl -s http://localhost:9000/
kill %1
```

**Expected:** Server starts and serves project visualization at root `/`. Exit code `0` on clean shutdown.

### 13.2 Port conflict

```bash
mxcli serve -p "$MPR" --port 9000 &
mxcli serve -p "$MPR" --port 9000
```

**Expected:** Second instance fails with "port already in use". Exit code `1`.

---

## 14. mxcli lsp

### 14.1 Start LSP server

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | mxcli lsp
```

**Expected:** Returns JSON-RPC `initialize` response with server capabilities (completion, hover, diagnostics, documentSymbol, foldingRange). Exit code `0`.

### 14.2 Verify capabilities

Inspect the `initialize` response `capabilities` object:

| Capability | Expected |
|------------|----------|
| `completionProvider` | Present |
| `hoverProvider` | `true` |
| `diagnosticProvider` | Present |
| `documentSymbolProvider` | `true` |
| `foldingRangeProvider` | `true` |

---

## 15. mxcli tui

### 15.1 Launch TUI

```bash
mxcli tui -p "$MPR"
```

**Expected:** Opens terminal UI explorer. Displays project tree. Arrow keys navigate. `q` exits. Exit code `0`.

### 15.2 TUI without project

```bash
mxcli tui
```

**Expected:** Error: `-p` flag required. Exit code `1`.

---

## 16. mxcli check

### 16.1 Validate MDL script syntax

```bash
mxcli check script.mdl
```

**Expected:** Validates MDL script syntax (not the project itself — use `docker check` for project validation). Prints errors and warnings. Exit code `0` if no errors, `1` if errors found.

### 16.2 SARIF output

```bash
mxcli check script.mdl --format sarif > results.sarif
```

**Expected:** Writes SARIF JSON to stdout. Exit code reflects validation result.

### 16.3 Check nonexistent file

```bash
mxcli check /tmp/nonexistent.mdl
```

**Expected:** Error: file not found. Exit code `1`.

---

## 17. mxcli report

### 17.1 Markdown report

```bash
mxcli report -p "$MPR" --format markdown
```

**Expected:** Prints project report in Markdown to stdout. Exit code `0`.

### 17.2 JSON report

```bash
mxcli report -p "$MPR" --format json
```

**Expected:** Prints valid JSON report. Exit code `0`.

### 17.3 HTML report

```bash
mxcli report -p "$MPR" --format html --output report.html
```

**Expected:** Writes HTML report to `report.html`. Exit code `0`.

---

## Failure Modes

| Scenario | Command | Expected error | Exit code |
|----------|---------|---------------|-----------|
| Missing `-p` flag | `mxcli test tests/` | "required flag: -p" | `1` |
| Invalid MDL path | `mxcli check /bad/path.mdl` | "file not found" | `1` |
| Docker not running | `mxcli docker run -p "$MPR"` | "cannot connect to Docker daemon" | `1` |
| MxBuild not installed | `mxcli docker check -p "$MPR"` | "MxBuild not found — run mxcli setup mxbuild" | `1` |
| Auth token expired | `mxcli marketplace search "x"` | "token expired — run mxcli auth login" | `1` |
| Port conflict | `mxcli serve --port <in-use>` | "address already in use" | `1` |
| Invalid test file | `mxcli test bad.txt -p "$MPR"` | "unsupported test file format" | `1` |
| Timeout exceeded | `mxcli test --timeout 1` | "test timed out after 1s" | `1` |
| BSON corrupt file | `mxcli bson dump corrupt.mpr` | "invalid BSON data" | `1` |

> **Note:** Several commands return exit `0` on error instead of `1`: `bson dump` (missing args), `docker build` (build failure), `docker check` (in some failure modes), `marketplace search` (auth failure), `serve` (port conflict), `fmt` (invalid input). Do not rely solely on exit codes for pass/fail in automation.

---

## Test Project Coverage Matrix

| Command | Subcommands | Happy path | Error path | Flags tested |
|---------|-------------|:----------:|:----------:|:------------:|
| `docker` | run, build, check, init, up, down, logs, status, shell, reload | 10 | 1 | -p, --port, --admin-port |
| `test` | — | 8 | 2 | -p, --list, --junit, --skip-build, --verbose, --color, --timeout |
| `playwright verify` | — | 4 | 0 | -p, --list, --junit, --verbose |
| `eval` | check, list | 2 | 0 | — |
| `init` | — | 1 | 1 | -p |
| `setup` | mxbuild, mxruntime, mxcli | 3 | 0 | — |
| `auth` | login, logout, status, list | 5 | 0 | — |
| `marketplace` | search, info | 4 | 0 | — |
| `widget` | extract, list, init, docs | 4 | 0 | -p |
| `bson` | dump, discover, compare | 3 | 2 | — |
| `fmt` | — | 2 | 1 | — |
| `new` | — | 2 | 0 | --version |
| `serve` | — | 1 | 1 | -p, --port |
| `lsp` | — | 2 | 0 | — |
| `tui` | — | 1 | 1 | -p |
| `check` | — | 2 | 1 | --format |
| `report` | — | 3 | 0 | -p, --format, --output |

**Total: 56 happy-path + 11 error-path = 67 test cases**

---

## Manual Test Report Template

**Tester:** _______________
**Date:** _______________
**mxcli version:** _______________

| # | Command | Case | Pass | Fail | Skip | Notes |
|---|---------|------|:----:|:----:|:----:|-------|
| 1.1 | `docker init` | Init Docker config | | | | |
| 1.2 | `docker build` | Build image | | | | |
| 1.3 | `docker run` | Run container | | | | |
| 1.4 | `docker check` | Runtime check | | | | |
| 1.5 | `docker up` | Compose up | | | | |
| 1.6 | `docker down` | Compose down | | | | |
| 1.7 | `docker logs` | View logs | | | | |
| 1.8 | `docker status` | Container status | | | | |
| 1.9 | `docker shell` | Interactive shell | | | | |
| 1.10 | `docker reload` | Hot reload | | | | |
| 2.1 | `test` | Run all tests | | | | |
| 2.2 | `test` | Single file | | | | |
| 2.3 | `test --list` | List tests | | | | |
| 2.4 | `test --junit` | JUnit output | | | | |
| 2.5 | `test --skip-build` | Skip build | | | | |
| 2.6 | `test --verbose` | Verbose output | | | | |
| 2.7 | `test --color` | Colored output | | | | |
| 2.8 | `test --timeout` | Custom timeout | | | | |
| 3.1 | `playwright` | Run scripts | | | | |
| 3.2 | `playwright --list` | List scripts | | | | |
| 3.3 | `playwright --junit` | JUnit output | | | | |
| 3.4 | `playwright --verbose` | Verbose output | | | | |
| 4.1 | `eval check` | Check correctness | | | | |
| 4.2 | `eval list` | List evaluations | | | | |
| 5.1 | `init` | Init AI config | | | | |
| 6.1 | `setup mxbuild` | Install MxBuild | | | | |
| 6.2 | `setup mxruntime` | Install runtime | | | | |
| 6.3 | `setup mxcli` | Self-update | | | | |
| 7.1 | `auth login` | Login flow | | | | |
| 7.2 | `auth logout` | Logout | | | | |
| 7.3 | `auth status` | Auth status | | | | |
| 7.4 | `auth list` | List accounts | | | | |
| 7.5 | `auth` | Token refresh | | | | |
| 8.1 | `marketplace search` | Search modules | | | | |
| 8.2 | `marketplace info` | Module info | | | | |
| 8.3 | `marketplace versions` | List versions | | | | |
| 8.4 | `marketplace` | Install module | | | | |
| 9.1 | `widget extract` | Extract widget | | | | |
| 9.2 | `widget list` | List widgets | | | | |
| 9.3 | `widget init` | Init widget project | | | | |
| 9.4 | `widget docs` | Generate docs | | | | |
| 10.1 | `bson dump` | Dump MPR | | | | |
| 10.2 | `bson discover` | Discover structure | | | | |
| 10.3 | `bson compare` | Compare MPRs | | | | |
| 11.1 | `fmt` | Format file | | | | |
| 11.2 | `fmt` | Format directory | | | | |
| 12.1 | `new` | Create project | | | | |
| 13.1 | `serve` | Start server | | | | |
| 14.1 | `lsp` | Start LSP | | | | |
| 14.2 | `lsp` | Diagnostics | | | | |
| 15.1 | `tui` | Terminal UI | | | | |
| 16.1 | `check` | Validate project | | | | |
| 16.2 | `check --sarif` | SARIF output | | | | |
| 17.1 | `report` | Markdown report | | | | |
| 17.2 | `report --format json` | JSON report | | | | |
| 17.3 | `report --output` | Output to file | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
