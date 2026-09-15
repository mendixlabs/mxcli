# Agent Testing Instructions

This document defines test execution methodology. Each `*-test-cases.md` file in this directory contains domain-specific test scenarios. This document tells you **how** to execute them.

---

<!-- TOC manually maintained -->
## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Test Session Workflow](#test-session-workflow)
4. [Reading Test Case Documents](#reading-test-case-documents)
5. [Execution Methods](#execution-methods)
6. [Working with Test Fixture Copies](#working-with-test-fixture-copies)
7. [Enriching the Test Project](#enriching-the-test-project)
8. [Verification Patterns](#verification-patterns)
9. [REPL and Interactive Testing via tmux](#repl-and-interactive-testing-via-tmux)
10. [Docker-Dependent Tests](#docker-dependent-tests)
11. [Auth-Required Tests](#auth-required-tests)
12. [Stress and Boundary Tests](#stress-and-boundary-tests)
13. [Handling Known Failures](#handling-known-failures)
14. [Result Reporting](#result-reporting)
15. [Known Limitations and Pitfalls](#known-limitations-and-pitfalls)
16. [Test Category Quick Reference](#test-category-quick-reference)

---

## Overview

mxcli is a CLI tool for working with Mendix `.mpr` project files. It provides:
- **MDL language** — a SQL-like DSL for creating/modifying Mendix app models
- **REPL mode** — interactive session for issuing MDL commands
- **CLI commands** — docker, test, playwright, auth, marketplace, eval, fmt, check, etc.

Tests verify that MDL statements and CLI commands produce correct results against real `.mpr` project files.

### What you're testing

- **MDL statements:** CREATE, DROP, ALTER, SHOW, DESCRIBE, GRANT, REVOKE, SET, CONNECT, DISCONNECT
- **CLI subcommands:** `mxcli docker`, `mxcli test`, `mxcli eval`, `mxcli fmt`, `mxcli check`, `mxcli auth`, etc.
- **Roundtrip fidelity:** DESCRIBE output → DROP → re-CREATE from output → DESCRIBE again = identical

### Test domains (19 documents)

| # | Document | Domain |
|---|----------|--------|
| 01 | entity-test-cases.md | Entities, associations, constants |
| 02 | enumeration-test-cases.md | Enumerations |
| 03 | microflow-test-cases.md | Microflows |
| 04 | nanoflow-test-cases.md | Nanoflows |
| 05 | page-test-cases.md | Pages, snippets, layouts |
| 06 | integration-test-cases.md | REST, OData, consumed/published services |
| 07 | security-test-cases.md | Module/user roles, entity/page/microflow access |
| 08 | navigation-settings-test-cases.md | Navigation profiles, menu items, project settings |
| 09 | organization-test-cases.md | Modules, folders, MOVE operations |
| 10 | workflow-test-cases.md | Workflows, user tasks, decisions |
| 11 | catalog-test-cases.md | MDL catalog queries |
| 12 | tooling-test-cases.md | Mermaid diagrams, diff, lint, formatter |
| 13 | cli-commands-test-cases.md | All CLI subcommands |
| 14 | sql-integration-test-cases.md | SQL queries against running app |
| 15 | session-test-cases.md | REPL session management |
| 16 | mapping-test-cases.md | Import/export mappings, JSON structures |
| 17 | business-event-test-cases.md | Business event services |
| 18 | image-collection-test-cases.md | Image collections |
| 19 | agent-editor-test-cases.md | Agent/editor integration |

---

## Prerequisites

### Required software

| Tool | Purpose | Install |
|------|---------|---------|
| `mxcli` | The tool under test | `make build` (binary at `./bin/mxcli`) |
| `tmux` | Interactive REPL testing | `brew install tmux` (macOS) / pre-installed (Linux) |
| Docker Desktop | SQL integration, docker commands | https://docker.com |
| Go 1.26+ | Building from source | `brew install go` or mise |

### Required files

| File | Purpose | Location |
|------|---------|----------|
| Test `.mpr` project | Primary test fixture | Downloaded from Mendix App Gallery |
| mxcli binary | Tool under test | `./bin/mxcli` after `make build` |

### Test projects

Demo apps from [Mendix App Gallery](https://appgallery.mendixcloud.com/):

| App | Studio Pro Version | Purpose |
|-----|-------------------|---------|
| Lato Enquiry Management | 11.4.0 | Primary fixture — entities, microflows, pages, security, navigation |
| Evora - Factory Management | 10.24.15 | Cross-version testing, widgets, workflows |
| Lato Product Inventory | 11.2.0 | Additional coverage — constants, enumerations |

### Obtaining test projects

```bash
# Option 1: Download .mpk from App Gallery, extract with mxcli
mxcli new --from-mpk EnquiriesManagement.mpk

# Option 2: If .mpr files already exist in a known location
export APPS_DIR=~/test-apps
export MPR="$APPS_DIR/EnquiriesManagement/EnquiriesManagement.mpr"
```

### Build and verify

```bash
make build && make test && make lint-go
./bin/mxcli version
./bin/mxcli -c "SHOW ENTITIES" -p "$MPR"  # quick smoke test
```

If `make test` fails, there are unit test regressions — investigate before proceeding with manual tests.

---

## Test Session Workflow

### 0. Load MDL syntax knowledge

Before executing tests, load the relevant MDL syntax references for the domain under test. This ensures you have correct statement syntax, known limitations, and valid examples — consult these references before marking a test as FAIL due to parse errors.

At minimum, load the general MDL overview. For domain-specific tests (entities, microflows, pages, security, workflows, integrations, etc.), also load the matching domain reference.

Follow this sequence for a complete test session:

### 1. Prepare environment

```bash
# Build fresh
make build

# Verify binary works
./bin/mxcli version

# Set project path
export MPR=~/test-apps/EnquiriesManagement/EnquiriesManagement.mpr
```

For write tests, create a working copy per [Working with Test Fixture Copies](#working-with-test-fixture-copies).

### 2. Execute test cases document by document

For each `*-test-cases.md` file:
1. Read the document to understand all test cases
2. Identify which execution method each test needs (see [Execution Methods](#execution-methods))
3. Execute tests in order — test IDs are hierarchical (1.1, 1.2, ..., 2.1, ...)
4. Record results in a table (see [Result Reporting](#result-reporting))
5. If a test requires features not in the project, see [Enriching the Test Project](#enriching-the-test-project)

### 3. Report results

After completing each document, produce a summary:
```
| Total | Pass | Fail | Skip |
|-------|------|------|------|
| 45    | 38   | 4    | 3    |
```

Plus the detailed results table and notes on any failures or crashes.

### 4. Cleanup

```bash
rm -rf "$WORK_DIR"
# Kill any lingering tmux sessions
tmux kill-server 2>/dev/null
# Stop Docker containers if started
mxcli docker down -p "$MPR" 2>/dev/null
```

---

## Reading Test Case Documents

Each test case document follows a consistent structure:

### Test case format

```markdown
### 1.1 Short description of the test

\`\`\`
MDL STATEMENT OR COMMAND HERE;
\`\`\`

**Expected:** Description of correct output or behavior.
```

### Interpreting "Expected"

- Literal text: output must contain that exact string
- Pattern descriptions ("Displays X, Y, Z"): output should show those elements
- Error expectations ("Error: entity not found"): verify error message appears
- Behavioral ("Silently succeeds"): verify exit code 0 and no error output

### Test ID numbering

- `§N` or `## N.` — test section (group of related tests)
- `N.M` — individual test case within section N
- Sections correspond to MDL statement types or command groups

---

## Execution Methods

### Method 1: Non-Interactive (`-c` flag) — PREFERRED

For single MDL statements that don't require session state:

```bash
./bin/mxcli -c "SHOW ENTITIES" -p "$MPR"
```

Multiple statements in one call (semicolons separate):

```bash
./bin/mxcli -c "CREATE ENTITY Mod.Foo (Name : String); SHOW ENTITIES;" -p "$WORK_MPR"
```

**When to use:** Most CREATE, DROP, SHOW, DESCRIBE, ALTER, GRANT, REVOKE statements. This is the fastest, most deterministic method.

**Limitations:**
- Cannot test SET commands (REPL-only)
- Cannot test session state persistence across commands
- Cannot test history navigation or tab completion
- Cannot test multi-step REPL roundtrips where intermediate state matters

### Method 2: MDL Script File

For multi-statement sequences:

```bash
cat > /tmp/test.mdl << 'EOF'
CREATE ENTITY MyModule.TestEntity (
  Name : String,
  Age : Integer
);

DESCRIBE ENTITY MyModule.TestEntity;

DROP ENTITY MyModule.TestEntity;

SHOW ENTITIES;
EOF

./bin/mxcli exec /tmp/test.mdl -p "$WORK_MPR"
```

**When to use:** Roundtrip tests, multi-step create/alter/verify sequences, any test requiring ordered operations.

### Method 3: Direct CLI Commands

For non-MDL CLI subcommands:

```bash
./bin/mxcli check script.mdl
./bin/mxcli fmt script.mdl
./bin/mxcli eval -p "$MPR" "expression"
./bin/mxcli docker init -p "$MPR"
./bin/mxcli bson dump -p "$MPR" --path "Module.Entity"
```

**When to use:** CLI command tests (doc 13), tooling tests (doc 12).

### Method 4: REPL via tmux (Interactive)

See dedicated section: [REPL and Interactive Testing via tmux](#repl-and-interactive-testing-via-tmux)

### Method 5: Docker Runtime

See dedicated section: [Docker-Dependent Tests](#docker-dependent-tests)

### Method 6: Auth-Dependent

See dedicated section: [Auth-Required Tests](#auth-required-tests)

---

## Working with Test Fixture Copies

**CRITICAL RULE:** Never run write operations (CREATE, DROP, ALTER, GRANT, REVOKE) against the original test project. Always work on a copy.

### Standard pattern

```bash
# Create isolated copy
WORK_DIR=$(mktemp -d)
cp -R "$(dirname $MPR)" "$WORK_DIR/"
WORK_MPR="$WORK_DIR/$(basename $(dirname $MPR))/$(basename $MPR)"

# Run write tests against copy
./bin/mxcli -c "CREATE ENTITY MyModule.Temp (Name : String);" -p "$WORK_MPR"

# Cleanup when done
rm -rf "$WORK_DIR"
```

### When you need a fresh copy mid-session

Some tests assume a clean state. If a prior test modified the project in a way that affects subsequent tests:

```bash
# Reset: re-copy from original
rm -rf "$WORK_DIR"
WORK_DIR=$(mktemp -d)
cp -R "$(dirname $MPR)" "$WORK_DIR/"
WORK_MPR="$WORK_DIR/$(basename $(dirname $MPR))/$(basename $MPR)"
```

### Read-only tests don't need copies

SHOW, DESCRIBE, and catalog queries are read-only — they can run against the original:

```bash
./bin/mxcli -c "SHOW ENTITIES" -p "$MPR"  # safe, no copy needed
```

---

## Enriching the Test Project

Some tests require features absent from the default test apps (workflows, business events, OData services, certain entity configurations).

### Identifying enrichment needs

Tests are skipped when they reference objects not in the project. Common indicators:
- Test expects to DESCRIBE/ALTER/DROP an object that doesn't exist
- Error: "not found", "no X in module Y"
- Test doc explicitly states "requires X in project"

### Bootstrap pattern

Create an MDL script that adds the required fixtures, run it before the dependent tests:

```bash
WORK_DIR=$(mktemp -d)
cp -R "$(dirname $MPR)" "$WORK_DIR/"
WORK_MPR="$WORK_DIR/$(basename $(dirname $MPR))/$(basename $MPR)"

# Bootstrap: add missing features
./bin/mxcli exec bootstrap.mdl -p "$WORK_MPR"

# Now run tests that depend on those features
./bin/mxcli -c "DESCRIBE WORKFLOW TestModule.ApprovalWorkflow" -p "$WORK_MPR"
```

### Example bootstrap scripts

**For workflow tests:**
```sql
CREATE ENTITY TestModule.ApprovalRequest (
  Title : String,
  Status : String DEFAULT 'Pending'
);

CREATE MICROFLOW TestModule.WF_CheckApproval (
  Request : TestModule.ApprovalRequest
)
  RETURNS Boolean
BEGIN
  RETURN $Request/Status = 'Approved';
END;
```

**For nanoflow tests:**
```sql
CREATE NANOFLOW TestModule.NF_ValidateInput (
  Input : String
)
  RETURNS Boolean
BEGIN
  RETURN $Input != '';
END;

CREATE NANOFLOW TestModule.NF_FormatName (
  FirstName : String,
  LastName : String
)
  RETURNS String
BEGIN
  RETURN $FirstName + ' ' + $LastName;
END;
```

**For integration tests:**
```sql
CREATE ENTITY TestModule.Customer (
  Name : String,
  Email : String,
  Active : Boolean DEFAULT true
);

CREATE PUBLISHED_ODATA_SERVICE TestModule.CustomerAPI
  PATH '/odata/v1/customers';
```

**For business event tests:**
```sql
CREATE BUSINESS_EVENT_SERVICE TestModule.OrderEvents;
```

### When to enrich vs. skip

| Situation | Action |
|-----------|--------|
| Test exercises MDL CRUD on an object type | Enrich: create a fixture object |
| Test exercises runtime behavior (execution, client-side logic) | Skip: can't validate from MDL |
| Test requires a specific Mendix version feature | Skip: version mismatch |
| Test requires external service (database, API) | Skip unless Docker available |

### Which test docs need enrichment

| Document | What to bootstrap |
|----------|-------------------|
| workflow-test-cases.md | Entity with workflow context + trigger microflow |
| business-event-test-cases.md | Business event service definition |
| nanoflow-test-cases.md | Sample nanoflows for DESCRIBE/ALTER tests |
| integration-test-cases.md | Published OData service + consumed service entity |
| sql-integration-test-cases.md | Database connection entity (also needs Docker) |

---

## Verification Patterns

### Exit code check

```bash
./bin/mxcli -c "SHOW ENTITIES" -p "$MPR"
echo "Exit code: $?"  # 0 = success
```

**Caveat:** Some commands incorrectly return exit 0 on error (BUG-028). Always check output text too.

### Output contains expected text

```bash
OUTPUT=$(./bin/mxcli -c "DESCRIBE ENTITY MyModule.Customer" -p "$MPR" 2>&1)
if echo "$OUTPUT" | grep -q "Name : String"; then
  echo "PASS"
else
  echo "FAIL"
  echo "Actual output:"
  echo "$OUTPUT"
fi
```

### Output does NOT contain text (verify deletion)

```bash
OUTPUT=$(./bin/mxcli -c "SHOW ENTITIES" -p "$WORK_MPR" 2>&1)
if echo "$OUTPUT" | grep -q "DeletedEntity"; then
  echo "FAIL: entity still exists"
else
  echo "PASS: entity removed"
fi
```

### Exact error message check

```bash
OUTPUT=$(./bin/mxcli -c "DROP ENTITY MyModule.NonExistent;" -p "$WORK_MPR" 2>&1)
if echo "$OUTPUT" | grep -qi "not found\|does not exist"; then
  echo "PASS: correct error"
else
  echo "FAIL: unexpected output: $OUTPUT"
fi
```

### Roundtrip test (DESCRIBE → DROP → re-CREATE → DESCRIBE = identical)

```bash
# 1. Capture original description
BEFORE=$(./bin/mxcli -c "DESCRIBE ENTITY MyModule.Customer" -p "$WORK_MPR" 2>&1)

# 2. Extract the MDL (skip status lines and REPL / terminators)
MDL=$(echo "$BEFORE" | grep -v "^WARNING\|^Connected\|^$" | grep -v "^/$")

# 3. Drop the entity
./bin/mxcli -c "DROP ENTITY MyModule.Customer;" -p "$WORK_MPR"

# 4. Recreate from captured MDL
echo "$MDL" | ./bin/mxcli exec /dev/stdin -p "$WORK_MPR"

# 5. Describe again
AFTER=$(./bin/mxcli -c "DESCRIBE ENTITY MyModule.Customer" -p "$WORK_MPR" 2>&1)

# 6. Compare
if [ "$BEFORE" = "$AFTER" ]; then
  echo "PASS: roundtrip identical"
else
  echo "FAIL: roundtrip mismatch"
  diff <(echo "$BEFORE") <(echo "$AFTER")
fi
```

### Count-based verification

```bash
# Verify entity count changed
BEFORE_COUNT=$(./bin/mxcli -c "SHOW ENTITIES" -p "$WORK_MPR" 2>&1 | wc -l)
./bin/mxcli -c "CREATE ENTITY MyModule.NewEntity (Name : String);" -p "$WORK_MPR"
AFTER_COUNT=$(./bin/mxcli -c "SHOW ENTITIES" -p "$WORK_MPR" 2>&1 | wc -l)
if [ "$AFTER_COUNT" -gt "$BEFORE_COUNT" ]; then
  echo "PASS: entity count increased"
else
  echo "FAIL: count unchanged ($BEFORE_COUNT → $AFTER_COUNT)"
fi
```

### Crash detection

```bash
OUTPUT=$(./bin/mxcli -c "SOME COMMAND" -p "$MPR" 2>&1)
EXIT=$?
if echo "$OUTPUT" | grep -qi "panic\|SIGSEGV\|runtime error\|nil pointer"; then
  echo "ERROR: crash detected"
  echo "$OUTPUT"
elif [ $EXIT -ne 0 ]; then
  echo "FAIL: non-zero exit ($EXIT)"
else
  echo "PASS"
fi
```

---

## REPL and Interactive Testing via tmux

For tests that require an interactive terminal session (SET commands, multi-step REPL workflows, history, tab completion, TUI commands).

### Why tmux

mxcli uses Bubble Tea for its TUI — it detects whether stdin is a TTY and disables interactive features if not. Piping input directly won't work. tmux provides a real PTY that mxcli accepts.

### Session lifecycle

```bash
# Create session with explicit dimensions
SESSION="mxcli-test-$$"
tmux new-session -d -s "$SESSION" -x 120 -y 40

# Start mxcli REPL
tmux send-keys -t "$SESSION" "./bin/mxcli repl -p $WORK_MPR" Enter

# Wait for REPL prompt to appear
sleep 2

# Verify REPL is ready
OUTPUT=$(tmux capture-pane -t "$SESSION" -p)
echo "$OUTPUT" | grep -q "mxcli>" || echo "ERROR: REPL not ready"
```

### Statement terminator: `/` (not `;`)

The REPL uses `/` (slash on its own line) as the statement terminator, following the Oracle SQL*Plus convention. Semicolons are stripped and ignored. Read commands (`SHOW`, `DESCRIBE`) execute on Enter without a terminator. Write commands (`CREATE`, `DROP`, `GRANT`, `REVOKE`, `ALTER`, `EXECUTE SCRIPT`) enter multi-line mode (`...>`) and require `/` on a separate line to execute.

```bash
# Read command — executes immediately on Enter
mdl> SHOW ENTITIES IN Administration
# (output appears)

# Write command — requires / terminator
mdl> CREATE ENTITY MyModule.Test (Name : String)
...> /
Created entity: MyModule.Test

# Multi-line write command
mdl> CREATE ENTITY MyModule.MultiLine (
...>   Name : String,
...>   Email : String
...> )
...> /
Created entity: MyModule.MultiLine
```

### Sending commands

```bash
# Read command (no terminator needed)
tmux send-keys -t "$SESSION" "SHOW ENTITIES" Enter
sleep 1

# Write command (/ terminator on separate line)
tmux send-keys -t "$SESSION" "CREATE ENTITY MyModule.Test (Name : String)" Enter
sleep 0.5
tmux send-keys -t "$SESSION" "/" Enter
sleep 1

# Multi-line command (send each line separately, then /)
tmux send-keys -t "$SESSION" "CREATE ENTITY MyModule.Test (" Enter
sleep 0.3
tmux send-keys -t "$SESSION" "  Name : String," Enter
sleep 0.3
tmux send-keys -t "$SESSION" "  Age : Integer" Enter
sleep 0.3
tmux send-keys -t "$SESSION" ")" Enter
sleep 0.3
tmux send-keys -t "$SESSION" "/" Enter
sleep 1
```

### Capturing and verifying output

```bash
# Capture visible screen + scrollback
OUTPUT=$(tmux capture-pane -t "$SESSION" -p -S -200)

# Check for expected content
echo "$OUTPUT" | grep -q "Expected text" && echo "PASS" || echo "FAIL"

# Capture just the last N lines (most recent output)
RECENT=$(tmux capture-pane -t "$SESSION" -p -S -20)
```

### Special keys for TUI testing

```bash
tmux send-keys -t "$SESSION" Up        # History: previous command
tmux send-keys -t "$SESSION" Down      # History: next command
tmux send-keys -t "$SESSION" Tab       # Tab completion
tmux send-keys -t "$SESSION" C-c       # Ctrl+C: cancel current input
tmux send-keys -t "$SESSION" C-d       # Ctrl+D: exit REPL
tmux send-keys -t "$SESSION" C-l       # Ctrl+L: clear screen
tmux send-keys -t "$SESSION" Escape    # Escape key
tmux send-keys -t "$SESSION" C-a       # Ctrl+A: beginning of line
tmux send-keys -t "$SESSION" C-e       # Ctrl+E: end of line
tmux send-keys -t "$SESSION" C-u       # Ctrl+U: clear line
tmux send-keys -t "$SESSION" C-w       # Ctrl+W: delete word
```

### Testing CONNECT/DISCONNECT

```bash
# Connect to a different project
tmux send-keys -t "$SESSION" "CONNECT LOCAL '$OTHER_MPR'" Enter
sleep 2
OUTPUT=$(tmux capture-pane -t "$SESSION" -p -S -5)
echo "$OUTPUT" | grep -q "Connected" && echo "PASS" || echo "FAIL"

# Disconnect
tmux send-keys -t "$SESSION" "DISCONNECT" Enter
sleep 0.5
```

### Teardown

```bash
# Exit REPL gracefully
tmux send-keys -t "$SESSION" C-d
sleep 0.5

# Kill session
tmux kill-session -t "$SESSION" 2>/dev/null
```

### Common pitfalls

- **Timing:** Always add a sleep after sending commands. REPL needs time to process and render. Use 1s for simple commands, 2-3s for operations that read/write the `.mpr`.
- **Scrollback:** Use `-S -200` (or larger) with `capture-pane` to get enough history. Default is only the visible screen.
- **Prompt detection:** Wait for `mxcli>` prompt before sending next command for reliable sequencing.
- **Multi-line commands:** Write commands enter multi-line mode (`...>`). Type `/` on its own line to execute. `Ctrl+C` cancels and returns to `mdl>`.
- **`/` not `;`:** The REPL uses `/` as statement terminator. Semicolons are stripped and ignored. Read commands execute on Enter; write commands require `/`.

---

## Docker-Dependent Tests

Required for: SQL integration (110 tests), `mxcli docker *` commands, `mxcli test`, `mxcli playwright`.

### Prerequisites

```bash
# Verify Docker is running
docker info > /dev/null 2>&1 || echo "ERROR: Docker not running"
```

### Start Mendix app in Docker

```bash
# Initialize Docker config (first time only)
./bin/mxcli docker init -p "$WORK_MPR"

# Start the app
./bin/mxcli docker run -p "$WORK_MPR"

# Wait for app to be ready (may take 30-60 seconds)
echo "Waiting for app startup..."
for i in $(seq 1 60); do
  if curl -sf http://localhost:8080/xas/ > /dev/null 2>&1; then
    echo "App ready after ${i}s"
    break
  fi
  sleep 1
done
```

### SQL integration tests

Once the app is running:

```bash
# Execute SQL queries
./bin/mxcli -c "SELECT * FROM MyModule.Customer;" -p "$WORK_MPR"
./bin/mxcli -c "SELECT COUNT(*) FROM MyModule.Customer WHERE Active = true;" -p "$WORK_MPR"
```

### Docker command tests

```bash
./bin/mxcli docker status -p "$WORK_MPR"
./bin/mxcli docker logs -p "$WORK_MPR"
./bin/mxcli docker reload -p "$WORK_MPR"
```

### Test runner

```bash
./bin/mxcli test spec.test.mdl -p "$WORK_MPR"
```

### Teardown

```bash
./bin/mxcli docker down -p "$WORK_MPR"
# Verify container is gone
docker ps | grep -q mxcli && echo "WARNING: container still running"
```

### If Docker is not available

Skip all tests in:
- `sql-integration-test-cases.md` (entire document)
- `cli-commands-test-cases.md` §1-§3 (docker, test, playwright sections)

Report as: `SKIP — Docker not available`

---

## Auth-Required Tests

Required for: `mxcli auth *`, `mxcli marketplace *`, and any command that contacts the Mendix platform.

### Setup

```bash
export MENDIX_TOKEN="<your-personal-access-token>"
./bin/mxcli auth login
./bin/mxcli auth status  # verify logged in
```

### Auth-dependent commands

```bash
./bin/mxcli marketplace search "Atlas"
./bin/mxcli marketplace info "Atlas_UI_Resources"
./bin/mxcli auth whoami
./bin/mxcli auth logout
```

### If auth is not available

Skip all tests that require platform connectivity. Report as: `SKIP — MENDIX_TOKEN not set`

Affected sections across test docs:
- `cli-commands-test-cases.md` §7 (auth), §8 (marketplace)
- Any test that explicitly states "requires authentication"

---

## Stress and Boundary Tests

These tests are opt-in — they exercise scale and edge cases. Do NOT run as part of routine test sessions.

### When to run

- Validating performance before a release
- Investigating a specific scalability report
- Explicitly requested by test plan

### Always use a fresh copy

```bash
WORK_DIR=$(mktemp -d)
cp -R "$(dirname $MPR)" "$WORK_DIR/"
WORK_MPR="$WORK_DIR/$(basename $(dirname $MPR))/$(basename $MPR)"
```

### Example patterns

**Bulk creation:**
```bash
for i in $(seq 1 100); do
  ./bin/mxcli -c "CREATE ENTITY StressModule.Entity_$i (Name : String);" -p "$WORK_MPR"
done
echo "Created 100 entities"
./bin/mxcli -c "SHOW ENTITIES" -p "$WORK_MPR" | wc -l
```

**Wide entity (many attributes):**
```bash
ATTRS=""
for i in $(seq 1 200); do
  [ -n "$ATTRS" ] && ATTRS="$ATTRS, "
  ATTRS="${ATTRS}Attr_$i : String"
done
./bin/mxcli -c "CREATE ENTITY StressModule.WideEntity ($ATTRS);" -p "$WORK_MPR"
```

**Large MDL script:**
```bash
# Generate a 10,000-line script
for i in $(seq 1 5000); do
  echo "CREATE ENTITY StressModule.E_$i (Name : String);"
  echo "DROP ENTITY StressModule.E_$i;"
done > /tmp/stress.mdl
./bin/mxcli exec /tmp/stress.mdl -p "$WORK_MPR"
```

**Rapid connect/disconnect (REPL):**
```bash
SESSION="stress-$$"
tmux new-session -d -s "$SESSION" -x 120 -y 40
tmux send-keys -t "$SESSION" "./bin/mxcli repl" Enter
sleep 1
for i in $(seq 1 50); do
  tmux send-keys -t "$SESSION" "CONNECT LOCAL '$WORK_MPR';" Enter
  sleep 0.5
  tmux send-keys -t "$SESSION" "DISCONNECT;" Enter
  sleep 0.3
done
tmux send-keys -t "$SESSION" C-d
tmux kill-session -t "$SESSION"
```

### Categories

| Type | What it tests | Pass criteria |
|------|--------------|---------------|
| Bulk creation | Many entities/microflows in sequence | No crash, all created |
| Wide entities | Max attributes per entity | No crash, correct count |
| Deep nesting | Deeply nested page widgets | No stack overflow |
| Large scripts | Script file size handling | Completes without timeout |
| Rapid operations | Session stability under load | No panic, no corruption |
| Long values | String length boundaries | Correct storage or clean error |

### Cleanup

```bash
rm -rf "$WORK_DIR"
```

---

## Handling Known Failures

### Purpose

Distinguish between known bugs (expected failures) and new regressions. **Always run known-failing tests** to detect regressions.

### Known bugs reference

Bug reports live in `docs/12-bug-reports/`. Each has:
- Bug ID (BUG-NNN)
- Severity (Critical / High / Medium / Low)
- Status (Open / Fixed / Won't Fix)
- Affected commands/statements

### Classification

| Situation | Report as | Action |
|-----------|-----------|--------|
| Fails exactly as known bug describes | FAIL (known) | Note BUG-NNN |
| Fails differently from known bug | FAIL (new) | File new bug |
| Crashes (SIGSEGV, panic, nil pointer) | ERROR | Always report |
| Known bug now passes | PASS (fixed?) | Note "was BUG-NNN, now passes" |
| Previously unknown failure | FAIL (new) | File new bug |

### Checking against known bugs

Before reporting a new failure:
1. Check `docs/12-bug-reports/` for matching bug
2. Check test session notes for prior occurrences
3. Compare error message/behavior with known bug description

### Filing new bugs

If a test reveals a new failure, document:
```markdown
## BUG-NNN: Short description

**Severity:** Critical | High | Medium | Low
**Command:** `mxcli exec -c "FAILING COMMAND"`
**Expected:** What should happen
**Actual:** What happened (include full output)
**Reproducible:** Yes / Sometimes / Once
**Version:** Output of `mxcli version`
```

---

## Result Reporting

### Per-test-case results table

```markdown
| ID | Name | Result | Notes |
|----|------|--------|-------|
| 1.1 | Create basic entity | PASS | |
| 1.2 | Create with all attribute types | PASS | |
| 1.3 | Create duplicate entity | PASS | Correct error shown |
| 2.1 | Describe entity | FAIL (new) | Missing AutoNumber type in output |
| 2.2 | Describe with associations | FAIL (known) | BUG-015 |
| 3.1 | SQL query | SKIP | Docker not available |
| 4.1 | DROP with cascade | ERROR | PANIC: nil pointer at executor.go:445 |
```

### Result values

- **PASS** — output matches expected behavior
- **FAIL (new)** — unexpected failure, not matching any known bug
- **FAIL (known)** — matches a known bug (include BUG-NNN)
- **SKIP** — prerequisite not met (state: Docker/Auth/enrichment needed)
- **ERROR** — crash, panic, SIGSEGV (always include stack trace)

### Per-document summary

```markdown
## Summary
| Total | Pass | Fail | Skip | Error |
|-------|------|------|------|-------|
| 45    | 38   | 3    | 3    | 1     |

### Key Findings
- BUG-NEW: [description] — affects tests 2.1, 2.3
- BUG-015 confirmed still present (test 2.2)
- PANIC in test 4.1 — [brief description]
```

### Session-level summary

After all documents are completed:
```markdown
## Test Session Summary — [DATE]

| Document | Total | Pass | Fail | Skip | Error |
|----------|-------|------|------|------|-------|
| 01-entity | 45 | 38 | 3 | 3 | 1 |
| 02-enumeration | 20 | 20 | 0 | 0 | 0 |
| ... | | | | | |
| **TOTAL** | **1099** | **617** | **91** | **393** | **2** |

### New Bugs Filed
- BUG-NNN: ...
- BUG-NNN: ...

### Known Bugs Confirmed
- BUG-001: still present
- BUG-015: still present

### Previously Failing, Now Passing
- BUG-022: appears fixed (test 5.3)
```

---

## Known Limitations and Pitfalls

1. **`-c` flag cannot test session-only features** — History navigation, tab completion, and CONNECT/DISCONNECT require REPL. Most write operations work via `-c` too.
2. **Bubble Tea TTY detection** — mxcli disables TUI features when stdin is not a TTY. Piping input won't trigger interactive behavior. Use tmux.
3. **Docker startup time** — Mendix runtime takes 30-60s to fully start. Always wait for port 8080 before running Docker-dependent tests.
4. **Concurrent writes corrupt .mpr** — Never run parallel test sessions against the same `.mpr` file. Always copy to tmpdir first.
5. **Exit codes unreliable** — BUG-028: some commands return exit 0 on error. Always verify output text, not just exit code.
6. **DESCRIBE output contains `/` terminators** — DESCRIBE output includes REPL-style `/` terminators between statements. When saving output to a script file for roundtrip testing, strip standalone `/` lines: `grep -v "^/$"`. Without this, the parser rejects the extra `/` tokens.
7. **Entity auto-creation** — `CREATE ENTITY MyModule.Foo (...)` auto-creates `MyModule` if it doesn't exist. No need to create modules manually.
8. **Statement terminators** — MDL statements in `-c` mode and script files require trailing semicolons. In REPL, use `/` on its own line to terminate write statements; read commands (`SHOW`, `DESCRIBE`) execute on Enter without any terminator. Semicolons are stripped and ignored in REPL.
9. **Case sensitivity** — MDL keywords are case-insensitive (`SHOW` = `show`). Module/entity names are case-sensitive (`MyModule.Foo` ≠ `mymodule.foo`).
10. **Working directory** — `mxcli exec` doesn't change working directory. Relative paths in `-p` are relative to where you invoke the command.
11. **Count only documented tests** — Never add ad-hoc experiments to test statistics. Only tests with an ID in a test case document count toward pass/fail/skip totals. If an ad-hoc test reveals something interesting, either add it to the appropriate test case doc first, or record it as a note outside the statistics.
12. **Add new scenarios to test case docs** — When you discover a new testable scenario during execution (e.g., edge case, undocumented behavior, regression check), add it to the appropriate `*-test-cases.md` file with the next available test ID in the relevant section BEFORE recording it in session results. This ensures the test corpus grows over time and scenarios are reproducible by future sessions.
13. **`mxcli new` requires Linux** — The `mxcli new` command invokes the `mx` binary from the MxBuild download, which is Linux-only. It fails with "exec format error" on macOS. Test on Linux or in a devcontainer.

---

## Test Category Quick Reference

| Category | Execution Method | Prerequisite | Enrichment Needed |
|----------|-----------------|--------------|-------------------|
| Entity, Enumeration, Association | `-c` flag (Method 1) | Build only | No |
| Microflow | `-c` flag | Build only | No |
| Nanoflow | `-c` flag | Build only | Some (for ALTER/DESCRIBE) |
| Page, Snippet | `-c` flag | Build only | No |
| Security | `-c` flag or REPL | Build only | No |
| Navigation, Settings | `-c` flag or REPL | Build only | No |
| Organization | `-c` flag | Build only | No |
| Session management | tmux REPL (Method 4) | Build + tmux | No |
| Catalog | `-c` flag | Build only | No |
| Tooling (mermaid, diff, lint) | `-c` flag or CLI (Method 3) | Build only | No |
| Integration (REST, OData) | `-c` flag | Build only | Some (publish) |
| Workflow | `-c` flag | Build only | Yes (workflow entity) |
| Business Events | `-c` flag | Build only | Yes (event service) |
| Mappings | `-c` flag | Build only | No (read-only) |
| Image Collections | `-c` flag | Build only | No |
| Agent/Editor | `-c` flag | Build only | No |
| SQL Integration | Docker (Method 5) | Docker running | Yes (DB connection) |
| CLI: docker, test, playwright | Docker (Method 5) | Docker running | No |
| CLI: auth, marketplace | Auth (Method 6) | MENDIX_TOKEN | No |
| CLI: eval, check, fmt, bson | Direct CLI (Method 3) | Build only | No |
| CLI: tui, serve, lsp | tmux REPL (Method 4) | Build + tmux | No |
| Roundtrip tests | Script file (Method 2) | Build only | No |
| Stress/Boundary | Fixture copy + script | Build only | No |
