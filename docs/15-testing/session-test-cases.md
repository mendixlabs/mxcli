# Session Management Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/301)

## Test Projects

Demo apps from [Mendix App Gallery](https://appgallery.mendixcloud.com/):

| App | Studio Pro | Modules |
|-----|-----------|---------|
| Lato Enquiry Management | 11.4.0 | <N> |
| Evora - Factory Management | 10.24.15 | <N> |
| Lato Product Inventory | 11.2.0 | <N> |

---

## Setup

### 1. Download test apps

1. Go to [Mendix App Gallery](https://appgallery.mendixcloud.com/)
2. Download each demo app listed above
3. Open each `.mpk` in Studio Pro to extract the `.mpr` file

### 2. Build mxcli

```bash
make build && make test && make lint-go
```

### 3. Interactive testing

```bash
mxcli repl -p <path-to-app>/EnquiriesManagement.mpr
```

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

### 1.4 Connect with no backend factory

Start REPL without a backend factory configured.

```
CONNECT LOCAL 'project.mpr';
```

**Expected:** Error indicating no backend factory available.

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

## 5. SET

### 5.1 Set output format

```
SET format = json;
```

**Expected:** `Set format = json`

### 5.2 Set default module

```
SET default module = MyModule;
```

**Expected:** `Set default module = MyModule`

### 5.3 Set arbitrary key

```
SET custom_key = custom_value;
```

**Expected:** `Set custom_key = custom_value`

---

## 6. UPDATE / REFRESH

### 6.1 Update reloads project

```
CONNECT LOCAL 'project.mpr';
UPDATE;
STATUS;
```

**Expected:** Disconnects and reconnects to the same MPR. STATUS shows the reloaded project.

### 6.2 Update when not connected

```
UPDATE;
```

**Expected:** Error indicating not connected.

---

## 7. EXECUTE SCRIPT

### 7.1 Execute a valid script

```
EXECUTE SCRIPT 'test-session.mdl';
```

Where `test-session.mdl` contains:

```
STATUS;
SHOW VERSION;
```

**Expected:** Strips `/` separators, parses all statements, executes sequentially. Output from both commands.

### 7.2 Script with EXIT stops only the script

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

### 7.3 Script file not found

```
EXECUTE SCRIPT 'missing.mdl';
```

**Expected:** Error indicating file not found.

### 7.4 Script with parse errors

```
EXECUTE SCRIPT 'bad-syntax.mdl';
```

**Expected:** Parse error message with location details.

---

## 8. HELP

### 8.1 General help

```
HELP;
```

**Expected:** List of available commands.

### 8.2 Help on specific topic

```
HELP CONNECT;
```

**Expected:** Usage details for CONNECT.

---

## 9. EXIT / QUIT

### 9.1 Exit from REPL

```
EXIT;
```

**Expected:** REPL terminates.

### 9.2 Quit from REPL

```
QUIT;
```

**Expected:** REPL terminates (alias for EXIT).

---

## 10. Multi-Step Workflow

### 10.1 Full session lifecycle

```
CONNECT LOCAL 'project.mpr';
STATUS;
SET default module = MyFirstModule;
SHOW VERSION;
UPDATE;
STATUS;
DISCONNECT;
```

**Expected:** Each command succeeds in sequence. STATUS after UPDATE reflects reloaded project. DISCONNECT confirms path.

### 10.2 Operations without connection

```
STATUS;
SHOW VERSION;
UPDATE;
```

**Expected:** Each command returns a "not connected" error.

---

## 11. FAILURE MODES

### 11.1 Connect to corrupt MPR

```
CONNECT LOCAL 'corrupt-file.mpr';
```

**Expected:** Backend error describing parse/open failure.

### 11.2 Execute script with runtime error

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

### 11.3 Double disconnect

```
CONNECT LOCAL 'project.mpr';
DISCONNECT;
DISCONNECT;
```

**Expected:** First DISCONNECT shows path. Second shows "Not connected".

### 11.4 SHOW VERSION without connection

```
SHOW VERSION;
```

**Expected:** Error — not connected.

### 11.5 SET with empty value

```
SET format = ;
```

**Expected:** Parse error or stored as empty string.

### 11.6 EXECUTE SCRIPT with recursive include

Create `recursive.mdl` containing:
```
EXECUTE SCRIPT 'recursive.mdl';
```

```
EXECUTE SCRIPT 'recursive.mdl';
```

**Expected:** Stack overflow protection or max recursion depth error.

### 11.7 Large script execution

Create a script with 1000+ statements:

```bash
for i in $(seq 1 1000); do echo "STATUS;" >> big-script.mdl; done
```

```
EXECUTE SCRIPT 'big-script.mdl';
```

**Expected:** All statements execute. No memory issues.

### 11.8 Connect to directory instead of file

```
CONNECT LOCAL '/tmp/';
```

**Expected:** Error — not a valid MPR file.

---

## 12. BOUNDARY & STRESS

### 12.1 Rapid connect/disconnect cycle

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

### 12.2 Many SET operations

```
SET key1 = value1;
SET key2 = value2;
...
SET key100 = value100;
```

**Expected:** All 100 settings stored. No map overflow.

### 12.3 Very long setting value

```
SET description = <10000-character string>;
```

**Expected:** Value stored and retrievable.

---

## Test Project Coverage Matrix

| # | Command | Scenario | Lato 11.4 | Evora 10.24 | Lato 11.2 |
|---|---------|----------|-----------|-------------|-----------|
| 1.1 | CONNECT LOCAL | Valid project | x | x | x |
| 1.2 | CONNECT LOCAL | Replace existing | x | | |
| 1.3 | CONNECT LOCAL | File not found | x | | |
| 1.4 | CONNECT LOCAL | No backend factory | x | | |
| 2.1 | DISCONNECT | When connected | x | x | x |
| 2.2 | DISCONNECT | When not connected | x | | |
| 3.1 | STATUS | When connected | x | x | x |
| 3.2 | STATUS | When not connected | x | | |
| 4.1 | SHOW VERSION | Connected | x | x | x |
| 5.1 | SET | Output format | x | | |
| 5.2 | SET | Default module | x | | |
| 5.3 | SET | Arbitrary key | x | | |
| 6.1 | UPDATE | Reload project | x | x | x |
| 6.2 | UPDATE | Not connected | x | | |
| 7.1 | EXECUTE SCRIPT | Valid script | x | | |
| 7.2 | EXECUTE SCRIPT | EXIT in script | x | | |
| 7.3 | EXECUTE SCRIPT | File not found | x | | |
| 7.4 | EXECUTE SCRIPT | Parse errors | x | | |
| 8.1 | HELP | General | x | | |
| 8.2 | HELP | Specific topic | x | | |
| 9.1 | EXIT | Terminate REPL | x | | |
| 9.2 | QUIT | Terminate REPL | x | | |
| 10.1 | MULTI-STEP | Full lifecycle | x | x | x |
| 10.2 | MULTI-STEP | No connection | x | | |
| 11.1 | FAILURE | Corrupt MPR | x | | |
| 11.2 | FAILURE | Script runtime error | x | | |
| 11.3 | FAILURE | Double disconnect | x | | |
| 11.4 | FAILURE | Version no connection | x | | |
| 11.5 | FAILURE | SET empty value | x | | |
| 11.6 | FAILURE | Recursive script | x | | |
| 11.7 | FAILURE | Large script | x | | |
| 11.8 | FAILURE | Directory as MPR | x | | |
| 12.1 | BOUNDARY | Rapid connect/disconnect | x | | |
| 12.2 | BOUNDARY | Many SET ops | x | | |
| 12.3 | BOUNDARY | Long value | x | | |

---

## Automated Test Coverage

| Area | Unit Tests | Integration Tests |
|------|-----------|-------------------|
| CONNECT LOCAL | Mock tests | Docker integration |
| DISCONNECT | Mock tests | — |
| STATUS | Mock tests | — |
| SHOW VERSION | Mock tests | — |
| SET | Mock tests | — |
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
| 1.4 | CONNECT | No backend factory | | | | |
| 2.1 | DISCONNECT | When connected | | | | |
| 2.2 | DISCONNECT | Not connected | | | | |
| 3.1 | STATUS | Connected | | | | |
| 3.2 | STATUS | Not connected | | | | |
| 4.1 | VERSION | Connected | | | | |
| 5.1 | SET | Format | | | | |
| 5.2 | SET | Default module | | | | |
| 5.3 | SET | Arbitrary key | | | | |
| 6.1 | UPDATE | Reload project | | | | |
| 6.2 | UPDATE | Not connected | | | | |
| 7.1 | SCRIPT | Valid script | | | | |
| 7.2 | SCRIPT | EXIT in script | | | | |
| 7.3 | SCRIPT | File not found | | | | |
| 7.4 | SCRIPT | Parse errors | | | | |
| 8.1 | HELP | General | | | | |
| 8.2 | HELP | Specific topic | | | | |
| 9.1 | EXIT | Terminate | | | | |
| 9.2 | QUIT | Terminate | | | | |
| 10.1 | MULTI-STEP | Full lifecycle | | | | |
| 10.2 | MULTI-STEP | No connection | | | | |
| 11.1 | FAILURE | Corrupt MPR | | | | |
| 11.2 | FAILURE | Script runtime error | | | | |
| 11.3 | FAILURE | Double disconnect | | | | |
| 11.4 | FAILURE | Version no connection | | | | |
| 11.5 | FAILURE | SET empty value | | | | |
| 11.6 | FAILURE | Recursive script | | | | |
| 11.7 | FAILURE | Large script | | | | |
| 11.8 | FAILURE | Directory as MPR | | | | |
| 12.1 | BOUNDARY | Rapid connect/disconnect | | | | |
| 12.2 | BOUNDARY | Many SET ops | | | | |
| 12.3 | BOUNDARY | Long value | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
