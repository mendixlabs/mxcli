# Session Management Test Cases — Manual Testing

**Updated:** 2026-05-01
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/386)

## Setup

> See [AGENT-TESTING.md](./AGENT-TESTING.md) for build, execution methods, and verification patterns.

**REPL statement terminator:** These tests are primarily designed for REPL execution. In REPL mode, use `/` on its own line to terminate write statements — not `;`. Read commands (`SHOW`, `DESCRIBE`, `STATUS`, `HELP`) execute on Enter without any terminator. The `;` shown in examples below is MDL syntax valid for `-c` mode; in REPL, omit it or replace with `/` on a new line.

---

## 1. CONNECT LOCAL (OPEN PROJECT)

### 1.1 Connect to a valid project

```
CONNECT LOCAL 'path/to/project.mpr';
```

**Expected:** `Connected to: /path/to/project.mpr (Mendix 10.x.y)`

### 1.2 Connect replaces existing connection

```
CONNECT LOCAL 'first.mpr';
CONNECT LOCAL 'second.mpr';
```

**Expected:** First connection disconnects silently. Second connection message shows `second.mpr`.

### 1.3 Connect to non-existent file

```
CONNECT LOCAL 'does/not/exist.mpr';
```

**Expected:** Error message. No active connection.

---

## 2. DISCONNECT (CLOSE PROJECT)

### 2.1 Disconnect when connected

```
CONNECT LOCAL 'project.mpr';
DISCONNECT;
```

**Expected:** `Disconnected from: /path/to/project.mpr`

### 2.2 Disconnect when not connected

```
DISCONNECT;
```

**Expected:** `Not connected`

---

## 3. STATUS (SHOW STATUS)

### 3.1 Status when connected

```
CONNECT LOCAL 'project.mpr';
STATUS;
```

**Expected:** Output includes: Status, Project path, Mendix Version, MPR Format, Modules list.

### 3.2 Status when not connected

```
STATUS;
```

**Expected:** `Status: Not connected`

---

## 4. SHOW VERSION

### 4.1 Show version of connected project

```
CONNECT LOCAL 'project.mpr';
SHOW VERSION;
```

**Expected:** Output includes: Mendix Version, Build Version, MPR Format, Schema Hash.

---

## 5. UPDATE / REFRESH

### 5.1 Update reloads project

```
CONNECT LOCAL 'project.mpr';
UPDATE;
STATUS;
```

**Expected:** Disconnects and reconnects to the same MPR. STATUS shows the reloaded project.

### 5.2 Update when not connected

```
UPDATE;
```

**Expected:** Error indicating not connected.

---

## 6. EXECUTE SCRIPT

### 6.1 Execute a valid script

```
EXECUTE SCRIPT 'test-session.mdl';
```

Where `test-session.mdl` contains:

```
STATUS;
SHOW VERSION;
```

**Expected:** Strips `/` separators, parses all statements, executes sequentially. Output from both commands.

### 6.2 Script with EXIT stops only the script

```
EXECUTE SCRIPT 'early-exit.mdl';
STATUS;
```

Where `early-exit.mdl` contains:

```
STATUS;
EXIT;
SHOW VERSION;
```

**Expected:** Script executes STATUS, hits EXIT, stops script. SHOW VERSION in script does not run. The REPL STATUS after the script runs normally.

### 6.3 Script file not found

```
EXECUTE SCRIPT 'missing.mdl';
```

**Expected:** Error indicating file not found.

### 6.4 Script with parse errors

```
EXECUTE SCRIPT 'bad-syntax.mdl';
```

**Expected:** Parse error message with location details.

---

## 7. HELP

### 7.1 General help

```
HELP;
```

**Expected:** List of available commands.

### 7.2 Help on specific topic

```
HELP CONNECT;
```

**Expected:** Usage details for CONNECT.

---

## 8. EXIT / QUIT

### 8.1 Exit from REPL

```
EXIT;
```

**Expected:** REPL terminates.

### 8.2 Quit from REPL

```
QUIT;
```

**Expected:** REPL terminates (alias for EXIT).

---

## 9. Multi-Step Workflow

### 9.1 Full session lifecycle

```
CONNECT LOCAL 'project.mpr';
STATUS;
SHOW VERSION;
UPDATE;
STATUS;
DISCONNECT;
```

**Expected:** Each command succeeds in sequence. STATUS after UPDATE reflects reloaded project. DISCONNECT confirms path.

### 9.2 Operations without connection

```
STATUS;
SHOW VERSION;
UPDATE;
```

**Expected:** Each command returns a "not connected" error.

---

## 10. FAILURE MODES

### 10.1 Connect to corrupt MPR

```
CONNECT LOCAL 'corrupt-file.mpr';
```

**Expected:** Backend error describing parse/open failure.

### 10.2 Execute script with runtime error

Create `error-script.mdl`:
```
STATUS;
SHOW ENTITIES;
```

Without connecting first:
```
EXECUTE SCRIPT 'error-script.mdl';
```

**Expected:** First statement outputs "Not connected". Script continues to next statement (fail-fast in script mode stops on first error).

### 10.3 Double disconnect

```
CONNECT LOCAL 'project.mpr';
DISCONNECT;
DISCONNECT;
```

**Expected:** First DISCONNECT shows path. Second shows "Not connected".

### 10.4 SHOW VERSION without connection

```
SHOW VERSION;
```

**Expected:** Error — not connected.

### 10.5 EXECUTE SCRIPT with recursive include

Create `recursive.mdl` containing:
```
EXECUTE SCRIPT 'recursive.mdl';
```

```
EXECUTE SCRIPT 'recursive.mdl';
```

**Expected:** Stack overflow protection or max recursion depth error.

### 10.6 Large script execution

Create a script with 1000+ statements:

```bash
for i in $(seq 1 1000); do echo "STATUS;" >> big-script.mdl; done
```

```
EXECUTE SCRIPT 'big-script.mdl';
```

**Expected:** All statements execute. No memory issues.

### 10.7 Connect to directory instead of file

```
CONNECT LOCAL '/tmp/';
```

**Expected:** Error — not a valid MPR file.

---

## 11. BOUNDARY & STRESS

### 11.1 Rapid connect/disconnect cycle

```
CONNECT LOCAL 'project.mpr';
DISCONNECT;
CONNECT LOCAL 'project.mpr';
DISCONNECT;
CONNECT LOCAL 'project.mpr';
DISCONNECT;
```

Repeat 10 times.

**Expected:** Each cycle succeeds. No connection leaks.

---

## Test Project Coverage Matrix

| # | Command | Scenario | Lato 11.4 | Evora 10.24 | Lato 11.2 |
|---|---------|----------|-----------|-------------|-----------|
| 1.1 | CONNECT LOCAL | Valid project | x | x | x |
| 1.2 | CONNECT LOCAL | Replace existing | x | | |
| 1.3 | CONNECT LOCAL | File not found | x | | |
| 2.1 | DISCONNECT | When connected | x | x | x |
| 2.2 | DISCONNECT | When not connected | x | | |
| 3.1 | STATUS | When connected | x | x | x |
| 3.2 | STATUS | When not connected | x | | |
| 4.1 | SHOW VERSION | Connected | x | x | x |
| 5.1 | UPDATE | Reload project | x | x | x |
| 5.2 | UPDATE | Not connected | x | | |
| 6.1 | EXECUTE SCRIPT | Valid script | x | | |
| 6.2 | EXECUTE SCRIPT | EXIT in script | x | | |
| 6.3 | EXECUTE SCRIPT | File not found | x | | |
| 6.4 | EXECUTE SCRIPT | Parse errors | x | | |
| 7.1 | HELP | General | x | | |
| 7.2 | HELP | Specific topic | x | | |
| 8.1 | EXIT | Terminate REPL | x | | |
| 8.2 | QUIT | Terminate REPL | x | | |
| 9.1 | MULTI-STEP | Full lifecycle | x | x | x |
| 9.2 | MULTI-STEP | No connection | x | | |
| 10.1 | FAILURE | Corrupt MPR | x | | |
| 10.2 | FAILURE | Script runtime error | x | | |
| 10.3 | FAILURE | Double disconnect | x | | |
| 10.4 | FAILURE | Version no connection | x | | |
| 10.5 | FAILURE | Recursive script | x | | |
| 10.6 | FAILURE | Large script | x | | |
| 10.7 | FAILURE | Directory as MPR | x | | |
| 11.1 | BOUNDARY | Rapid connect/disconnect | x | | |

---

## Automated Test Coverage

| Area | Unit Tests | Integration Tests |
|------|-----------|-------------------|
| CONNECT LOCAL | Mock tests | Docker integration |
| DISCONNECT | Mock tests | — |
| STATUS | Mock tests | — |
| SHOW VERSION | Mock tests | — |
| UPDATE | Mock tests | — |
| EXECUTE SCRIPT | Mock tests | — |
| HELP | None | — |
| EXIT / QUIT | Mock tests | — |
| Failure modes | Partial | — |
| Boundary | — | All manual |

---

## Manual Test Report Template

**Tester:** _______________
**Date:** _______________
**mxcli version:** `mxcli --version` → _______________

| # | Section | Test | Pass | Fail | Skip | Notes |
|---|---------|------|:----:|:----:|:----:|-------|
| 1.1 | CONNECT | Valid project | | | | |
| 1.2 | CONNECT | Replace existing | | | | |
| 1.3 | CONNECT | File not found | | | | |
| 2.1 | DISCONNECT | When connected | | | | |
| 2.2 | DISCONNECT | Not connected | | | | |
| 3.1 | STATUS | Connected | | | | |
| 3.2 | STATUS | Not connected | | | | |
| 4.1 | VERSION | Connected | | | | |
| 5.1 | UPDATE | Reload project | | | | |
| 5.2 | UPDATE | Not connected | | | | |
| 6.1 | SCRIPT | Valid script | | | | |
| 6.2 | SCRIPT | EXIT in script | | | | |
| 6.3 | SCRIPT | File not found | | | | |
| 6.4 | SCRIPT | Parse errors | | | | |
| 7.1 | HELP | General | | | | |
| 7.2 | HELP | Specific topic | | | | |
| 8.1 | EXIT | Terminate | | | | |
| 8.2 | QUIT | Terminate | | | | |
| 9.1 | MULTI-STEP | Full lifecycle | | | | |
| 9.2 | MULTI-STEP | No connection | | | | |
| 10.1 | FAILURE | Corrupt MPR | | | | |
| 10.2 | FAILURE | Script runtime error | | | | |
| 10.3 | FAILURE | Double disconnect | | | | |
| 10.4 | FAILURE | Version no connection | | | | |
| 10.5 | FAILURE | Recursive script | | | | |
| 10.6 | FAILURE | Large script | | | | |
| 10.7 | FAILURE | Directory as MPR | | | | |
| 11.1 | BOUNDARY | Rapid connect/disconnect | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
