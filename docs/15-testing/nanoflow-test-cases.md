# Nanoflow Test Cases — Manual Testing

**Updated:** 2026-04-28
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/301)

## Test Projects

Demo apps from [Mendix App Gallery](https://appgallery.mendixcloud.com/):

| App | Studio Pro | Nanoflows |
|-----|-----------|-----------|
| Lato Enquiry Management | 11.4.0 | 79 |
| Evora - Factory Management | 10.24.15 | 93 |
| Lato Product Inventory | 11.2.0 | 51 |

Total: 223 nanoflows across 3 projects.

---

## Setup

### 1. Download test apps

1. Go to [Mendix App Gallery](https://appgallery.mendixcloud.com/)
2. Download each demo app listed above
3. Open each `.mpk` in Studio Pro to extract the `.mpr` file

### 2. Build mxcli

```bash
git checkout pr4-nanoflows-all
make build && make test && make lint-go
```

### 3. Smoke test

```bash
APPS_DIR=<path-to-extracted-apps>
for mpr in "$APPS_DIR"/*/*.mpr; do
  echo "=== $(basename $(dirname $mpr)) ==="
  echo "show nanoflows;" > /tmp/show-nf.mdl
  mxcli exec /tmp/show-nf.mdl -p "$mpr" 2>&1 | tail -1
done
```

Expected: 79, 93, 51 nanoflows respectively.

### 4. Interactive testing

```bash
mxcli repl -p <path-to-app>/EnquiriesManagement.mpr
```

### 5. Script-based testing

```bash
mxcli exec test-sequence.mdl -p <mpr>
```

Write operations (CREATE, DROP, GRANT/REVOKE) modify the `.mpr` file **in place**.

> **IMPORTANT:** Always run destructive tests against a **copy** of the project folder,
> never the original. The `.mpr` file references other files in the project directory,
> and nanoflows that are DROPped cannot be recovered — there is no undo, no git history,
> and no Studio Pro autosave for `.mpr` files.
>
> ```bash
> # Before each destructive test session
> cp -r MyProject MyProject-test
> mxcli repl -p MyProject-test/MyProject.mpr
> ```

---

## 1. SHOW NANOFLOWS

### 1.1 List all nanoflows
```
show nanoflows;
```
**Expected:** All nanoflows listed. Count matches Studio Pro.

### 1.2 Filter by module
```
show nanoflows in MyModule;
```
**Expected:** Only nanoflows from `MyModule`. No microflows.

### 1.3 Empty module
```
show nanoflows in ModuleWithNoNanoflows;
```
**Expected:** Empty result, no error.

### 1.4 Non-existent module
```
show nanoflows in NonExistentModule;
```
**Expected:** Error message.

### 1.5 Activity count accuracy
Pick 5+ nanoflows with known activity counts (verified in Studio Pro). Verify `show nanoflows` column matches.

### 1.6 Complexity values
Verify complexity values for nanoflows with varying numbers of decisions, loops, and nested paths.

---

## 2. DESCRIBE NANOFLOW

### 2.1 Simple nanoflow (no parameters, no return)
```
describe nanoflow Module.SimpleNanoflow;
```
**Expected:** Valid `create or modify nanoflow` MDL output.

### 2.2 Nanoflow with parameters and return type
```
describe nanoflow Module.NanoflowWithParams;
```
**Expected:** Parameters with correct types. Return type shown.

### 2.3 Parameter format variants
Find nanoflows across different Studio Pro versions. The BSON parser handles multiple parameter storage formats (`MicroflowParameterCollection`, `MicroflowParameters`, `Parameters`, and `ObjectCollection.Objects` fallback).

### 2.4 Activity coverage

Test DESCRIBE on nanoflows containing each allowed action type:

| # | Activity | Verify |
|---|----------|--------|
| 1 | CreateVariable | Variable name, type, initial value |
| 2 | ChangeVariable | Target variable, new value expression |
| 3 | CreateObject | Entity name, member assignments |
| 4 | ChangeObject | Target object, changed members |
| 5 | CommitObject | With/without events |
| 6 | DeleteObject | Target object |
| 7 | RollbackObject | Target object |
| 8 | Retrieve | Source (association/database), XPath constraint |
| 9 | AggregateList | List, function (sum/avg/count/min/max) |
| 10 | ChangeList | Target list, operation |
| 11 | CreateList | Entity type |
| 12 | ListOperation | Operation type, lists involved |
| 13 | CastObject | Source, target type |
| 14 | ShowPage | Page reference, page parameter object |
| 15 | ClosePage | No arguments |
| 16 | ShowMessage | Message template, blocking/non-blocking |
| 17 | ValidationFeedback | Object, member, message |
| 18 | CallNanoflow | `call nanoflow Module.Name (args)` — NOT `call microflow` |
| 19 | CallMicroflow | `call microflow Module.Name (args)` |
| 20 | CallJavaScriptAction | `call javascript action` syntax |
| 21 | Synchronize | No arguments (nanoflow-only) |
| 22 | LogMessage | Level, message template |
| 23 | ExclusiveSplit | Decision expression, true/false paths |
| 24 | Loop | Iterator variable, list variable |
| 25 | MergeNode | Multiple incoming paths converge |

### 2.5 Error handling
```
describe nanoflow Module.NanoflowWithErrorHandling;
```
**Expected:** Error handler flow shown. `$latestError` predefined variable preserved. Verify on IfStmt, LoopStmt, WhileStmt.

### 2.6 Nested control flow
Test nanoflows with: if inside loop, loop inside if, nested if/else chains, error handling inside loop body.

### 2.7 Non-existent nanoflow
```
describe nanoflow Module.DoesNotExist;
```
**Expected:** Clear error message.

### 2.8 Activity count regression
Pick 5+ nanoflows with known activity counts from Studio Pro. Verify DESCRIBE body contains correct number.

### 2.9 Documentation and MarkAsUsed properties
Test nanoflow with Documentation string set and MarkAsUsed=true. Verify both appear in DESCRIBE output.

### 2.10 Excluded nanoflow
Test nanoflow with Excluded=true. Verify property appears in output.

---

## 3. CREATE NANOFLOW

> Parentheses required even for parameterless nanoflows: `create nanoflow M.N () begin end;`.

### 3.1 Minimal nanoflow
```
create nanoflow MyModule.TestNano ()
begin
end;
```
**Expected:** Created. Listed in `show nanoflows`. Roundtrips via DESCRIBE.

### 3.2 With parameters — primitive types
```
create nanoflow MyModule.TestParams (
  Name : String,
  Count : Integer,
  Active : Boolean,
  Amount : Decimal,
  StartDate : DateTime
) returns String
begin
end;
```
**Expected:** All parameter types preserved.

### 3.3 With entity parameter
```
create nanoflow MyModule.TestEntity (
  Input : MyModule.MyEntity
) returns MyModule.MyEntity
begin
end;
```
**Expected:** Entity reference resolved. Error if entity doesn't exist.

### 3.4 With enumeration parameter
```
create nanoflow MyModule.TestEnum (
  Status : MyModule.StatusEnum
) returns MyModule.StatusEnum
begin
end;
```
**Expected:** Enum reference resolved. Error if enum doesn't exist.

### 3.5 With activities
```
create nanoflow MyModule.TestActivities ()
begin
  log info 'hello';
  log warning 'world';
end;
```
**Expected:** Activities preserved in DESCRIBE. `log info 'text'` renders as `log info node 'Application' 'text'`.

### 3.6 With call nanoflow action
```
create nanoflow MyModule.Caller () returns Boolean
begin
  $Result = call nanoflow MyModule.Target ();
end;
```
**Expected:** `NanoflowCallAction` stored (not `MicroflowCallAction`).

### 3.7 Create or modify — existing nanoflow
```
create or modify nanoflow MyModule.Existing ()
begin
end;
```
**Expected:** Existing nanoflow updated (ID reused). No error.

### 3.8 Create duplicate — without OR MODIFY
```
create nanoflow MyModule.TestNano () begin end;
create nanoflow MyModule.TestNano () begin end;
```
**Expected:** Second CREATE fails with "already exists" error.

### 3.9 Module auto-creation
```
create nanoflow NewModule.TestNano () begin end;
```
**Expected:** `NewModule` created automatically if it doesn't exist.

### 3.10 Folder placement
```
create nanoflow MyModule.TestNano () folder 'SubFolder/Nested' begin end;
```
**Expected:** Nanoflow placed in correct folder.

### 3.11 ID reuse after drop
```
create nanoflow MyModule.A () begin end;
drop nanoflow MyModule.A;
create nanoflow MyModule.A () begin end;
```
**Expected:** Second CREATE reuses the ID from the dropped nanoflow.

### 3.12 Default return type
Create nanoflow without explicit return type. DESCRIBE should show VoidType or omit return.

### 3.13 Write guard
Attempt CREATE without opening a project for writing.
**Expected:** Error about not being connected.

---

## 4. CREATE NANOFLOW — Validation

### 4.1 Disallowed actions
Each must be rejected with clear error:

| # | Disallowed action | Notes |
|---|-------------------|-------|
| 1 | ErrorEvent | |
| 2 | Java action call | |
| 3 | Database query | |
| 4 | REST call | |
| 5 | Web service call | No MDL AST type — cannot be written in MDL |
| 6 | Import mapping | |
| 7 | Export mapping | |
| 8 | Generate document | No MDL AST type — cannot be written in MDL |
| 9 | Show home page | |
| 10 | Download file | |
| 11 | External action | |
| 12 | Send external object | No MDL AST type — cannot be written in MDL |
| 13 | Delete external object | No MDL AST type — cannot be written in MDL |
| 14 | Transform JSON | |
| 15 | All workflow actions (11 types) | |

### 4.2 Binary return type rejected
```
create nanoflow MyModule.Bad () returns Binary begin end;
```
**Expected:** Validation error.

### 4.3 Disallowed actions in nested control flow
```
create nanoflow MyModule.Nested ()
begin
  if (true) then
    call java action SomeModule.JavaAction ();
  end if;
end;
```
**Expected:** Rejected — validation recurses into nested blocks.

### 4.4 Disallowed actions in error handling body
**Expected:** Rejected — validation checks error handling clauses.

### 4.5 Non-existent nanoflow target
```
create nanoflow MyModule.BadRef () begin
  call nanoflow NonExistent.Flow ();
end;
```
**Expected:** Error — target nanoflow not found.

### 4.6 Non-existent page target
```
create nanoflow MyModule.BadPage () begin
  show page NonExistent.Page ();
end;
```
**Expected:** Error — target page not found.

### 4.7 Non-existent microflow target
```
create nanoflow MyModule.BadMF () begin
  call microflow NonExistent.Flow ();
end;
```
**Expected:** Error — target microflow not found.

---

## 5. DROP NANOFLOW

### 5.1 Drop existing nanoflow
```
create nanoflow MyModule.ToDrop () begin end;
drop nanoflow MyModule.ToDrop;
```
**Expected:** Removed. Not in `show nanoflows`.

### 5.2 Drop non-existent nanoflow
```
drop nanoflow MyModule.DoesNotExist;
```
**Expected:** Clear error.

### 5.3 Drop referenced nanoflow
Create two nanoflows where one calls the other, drop the callee.
**Expected:** Warning or error about dangling reference.

### 5.4 Drop and recreate (ID reuse)
See §3.11.

### 5.5 Write guard
**Expected:** Error if no project open for writing.

---

## 6. CALL NANOFLOW (inside flow body)

`call nanoflow` is an action inside a flow body (`begin`/`end`), not a standalone MDL command.

### 6.1 Call with arguments
```
create nanoflow MyModule.Adder (A : Integer, B : Integer) returns Integer
begin
  $Result = $A + $B;
end;
create microflow MyModule.Caller () returns Integer
begin
  $Result = call nanoflow MyModule.Adder (A = 1, B = 2);
end;
```
**Expected:** Arguments mapped correctly.

### 6.2 Call nanoflow from nanoflow
**Expected:** Uses `NanoflowCallAction` (not `MicroflowCallAction`).

### 6.3 Call with return value assignment
```
$Result = call nanoflow MyModule.GetValue ();
```
**Expected:** Return value assigned to variable.

### 6.4 Call without return value (void nanoflow)
```
call nanoflow MyModule.DoSomething ();
```
**Expected:** No assignment. No error.

### 6.5 Call with error handling
```
$Result = call nanoflow MyModule.Risky () on error continue;
```
**Expected:** `on error continue` parsed and preserved in DESCRIBE.

### 6.6 Recursive call
```
create nanoflow MyModule.Recursive () returns Boolean
begin
  $Result = call nanoflow MyModule.Recursive ();
end;
```
**Expected:** Parses without error.

### 6.7 Call JavaScript action — simple
```
create nanoflow MyModule.JSTest () returns Boolean
begin
  $Result = call javascript action NanoflowCommons.HasConnectivity ();
end;
```
**Expected:** Parses, creates. DESCRIBE preserves `call javascript action` syntax.

### 6.8 Call JavaScript action — with parameters
```
create nanoflow MyModule.JSWithParams () returns Boolean
begin
  $Result = call javascript action NanoflowCommons.SignIn (userName = 'test', password = 'pass');
end;
```
**Expected:** Parameter mappings preserved in DESCRIBE.

### 6.9 Call JavaScript action — roundtrip
1. DESCRIBE an existing nanoflow that calls a JavaScript action (e.g. `Atlas_Web_Content.ACT_Login`)
2. Capture MDL output
3. DROP the nanoflow
4. Execute captured MDL (CREATE OR MODIFY)
5. DESCRIBE again
6. Compare — `call javascript action` syntax preserved. Only expected diff: `on error rollback` appended (default error handling)

### 6.10 Call JavaScript action — cross-module
Test calling a JS action defined in a different module (e.g. `NanoflowCommons.SignIn` from `Atlas_Web_Content`).

**Expected:** Qualified action name preserved across modules.

---

## 7. GRANT / REVOKE EXECUTE ON NANOFLOW

> Drop/recreate of the same nanoflow name preserves security roles by design. Use REVOKE after recreate if roles should change.

### 7.1 Grant to single role
```
grant execute on nanoflow MyModule.TestNano to MyModule.User;
```
**Expected:** Verifiable via `show access on nanoflow`.

### 7.2 Grant to multiple roles
```
grant execute on nanoflow MyModule.TestNano to MyModule.User, MyModule.Admin;
```
**Expected:** Both roles added.

### 7.3 Idempotent grant
Grant same role twice.
**Expected:** "already have access" on second grant. No duplicate entries.

### 7.4 Revoke from role
```
revoke execute on nanoflow MyModule.TestNano from MyModule.User;
```
**Expected:** Role removed.

### 7.5 Idempotent revoke
Revoke role that was never granted.
**Expected:** "none of the specified roles have access".

### 7.6 Grant on non-existent nanoflow
**Expected:** Clear error.

### 7.7 Grant with non-existent role
**Expected:** Clear error.

### 7.8 Write guard
**Expected:** Error if no project open for writing.

---

## 8. SHOW ACCESS ON NANOFLOW

### 8.1 Nanoflow with roles
```
show access on nanoflow MyModule.TestNano;
```
**Expected:** Lists all allowed module roles.

### 8.2 Nanoflow without roles
**Expected:** Empty list or "No access" message.

### 8.3 JSON output format
**Expected:** Valid JSON array of role objects.

### 8.4 Nil name
**Expected:** Validation error (not crash).

### 8.5 Non-existent nanoflow
**Expected:** Clear error.

### 8.6 Role ID display
Verify roles display as `Module.Role` format.

---

## 9. RENAME NANOFLOW

### 9.1 Simple rename
```
rename nanoflow MyModule.OldName to NewName;
```
**Expected:** Renamed. `show nanoflows` shows new name.

### 9.2 Rename with callers
Rename nanoflow called by another flow. Verify caller's reference updated.

### 9.3 Rename to existing name
**Expected:** Error — name collision.

---

## 10. MOVE NANOFLOW

### 10.1 Move to another module
```
move nanoflow MyModule.TestNano to TargetModule;
```
**Expected:** Qualified name becomes `TargetModule.TestNano`.

### 10.2 Move to non-existent module
**Expected:** Error: `failed to find target module: module not found`.

---

## 11. MERMAID OUTPUT (CLI `--format mermaid`)

Mermaid is a presentation format accessed via the CLI `--format mermaid` flag.

### 11.1 Simple nanoflow
```
mxcli describe nanoflow -p <mpr> --format mermaid Module.SimpleNanoflow
```
**Expected:**
- `flowchart TD` header
- Start node, end node, activity nodes
- Edges connecting nodes in correct order
- `%% nodeinfo` section with node metadata

### 11.2 Complex nanoflow with branching
```
mxcli describe nanoflow -p <mpr> --format mermaid Module.ComplexNanoflow
```
**Expected:**
- If/else branches with condition labels on edges
- Multiple activity types with correct labels
- Merge points where branches rejoin

### 11.3 Nanoflow with call actions
```
mxcli describe nanoflow -p <mpr> --format mermaid Module.NanoWithCalls
```
**Expected:** Call action nodes show qualified target names, not generic "Action" labels.

### 11.4 Non-existent nanoflow
```
mxcli describe nanoflow -p <mpr> --format mermaid Module.DoesNotExist
```
**Expected:** Clear error message. No empty output or crash.

---

## 12. BSON ROUNDTRIP

### 12.1 Simple roundtrip
1. DESCRIBE nanoflow → capture MDL
2. DROP nanoflow
3. Execute captured MDL
4. DESCRIBE again → capture
5. Diff the two outputs

**Expected:** Identical or cosmetic-only differences (expression whitespace normalization).

### 12.2 Complex roundtrip
Repeat §12.1 on nanoflows with: error handling, annotations, 10+ activities, multiple parameter types, nested control flow.

**Expected:** Structure preserved. Known cosmetic diffs:
- Expression whitespace: `find($x,'y')` → `find($x, 'y')`
- Association retrieve syntax: `from $X/Assoc` may become `from Entity where Assoc = $X`

### 12.3 Bulk roundtrip
Run §12.1 on all 223 nanoflows across 3 test projects. Record pass/fail counts.

---

## 13. CATALOG

### 13.1 Catalog query
```
select * from catalog.nanoflows;
```
**Expected:** All nanoflows listed with correct columns.

### 13.2 MicroflowType field
**Expected:** All entries show `MicroflowType = NANOFLOW`.

### 13.3 Filter by module
```
select * from catalog.nanoflows where ModuleName = 'MyModule';
```
**Expected:** Only nanoflows from specified module. Column names are PascalCase.

---

## 14. DIFF

### 14.1 Modified nanoflow
1. DESCRIBE nanoflow → save to file
2. Modify the nanoflow (CREATE OR MODIFY with different body)
3. `mxcli diff -p <mpr> <saved-file.mdl>`

**Expected:** Unified diff with `---`/`+++` headers and `@@` hunks.

### 14.2 New nanoflow
1. Create `.mdl` file with a new nanoflow definition
2. `mxcli diff -p <mpr> <new-file.mdl>`

**Expected:** Shows full addition.

---

## 15. MULTI-STEP WORKFLOWS

### 15.1 Scaffold module with nanoflows
1. CREATE 3 nanoflows in a new module
2. GRANT roles to each
3. DESCRIBE each — verify complete MDL output
4. `mxcli describe nanoflow -p <mpr> --format mermaid` on each — verify Mermaid output

### 15.2 Rename in call chain
1. CREATE nanoflow A calling nanoflow B
2. RENAME B
3. DESCRIBE A — verify reference updated

### 15.3 Move and reorganize
1. CREATE nanoflow in ModuleA
2. MOVE to ModuleB
3. Verify qualified name, folder, params preserved

### 15.4 Iterative CREATE OR MODIFY
1. `create nanoflow M.Evolving () begin end;`
2. `create or modify nanoflow M.Evolving ($Name : String) begin end;`
3. `create or modify nanoflow M.Evolving ($Name : String) returns String begin $Result = $Name; end;`
4. `create or modify nanoflow M.Evolving ($Name : String, $Count : Integer) returns String begin $Result = $Name; end;`
5. After each step: DESCRIBE and verify cumulative changes preserved
6. Final roundtrip: DESCRIBE → DROP → execute captured → DESCRIBE → compare

**Expected:** Each modification preserves prior state. Roundtrip matches last version.

### 15.5 Drop and recreate with different signature
1. CREATE nanoflow with `String` return and 2 params, GRANT roles
2. DROP
3. CREATE same name with `Integer` return and 0 params
4. `show access on nanoflow M.Name;` — verify roles NOT carried over
5. DESCRIBE — verify new signature, no remnant of old params/body

### 15.6 Cross-module call chain
1. CREATE `ModuleA.Entrypoint` calling `ModuleB.Processor`
2. CREATE `ModuleB.Processor` calling `microflow ModuleC.DataFetcher`
3. DESCRIBE `ModuleA.Entrypoint` — verify cross-module call shown
4. DROP `ModuleB.Processor`
5. DESCRIBE `ModuleA.Entrypoint` — verify dangling reference handling (no crash)
6. Recreate `ModuleB.Processor` — verify caller roundtrips again

---

## 16. FAILURE MODES & ERROR RECOVERY

### 16.1 Validation failure mid-batch
```
create nanoflow M.Good1 () begin end;
create nanoflow M.Bad () begin call java action SomeModule.JavaAction (); end;
create nanoflow M.Good3 () begin end;
```
**Expected:** Good1 created, Bad rejected, Good3 NOT created — batch aborts on first error.

> Batch mode (`mxcli exec`) is fail-fast. REPL mode continues on error per-line.

### 16.2 CREATE with non-existent entity parameter
```
create nanoflow M.BadParam (Input : NonExistent.Entity) begin end;
```
**Expected:** Clear error. No partial nanoflow in model.

### 16.3 CREATE with non-existent enum return type
```
create nanoflow M.BadReturn () returns NonExistent.MyEnum begin end;
```
**Expected:** Clear error. No partial nanoflow in model.

### 16.4 DESCRIBE after partial modification failure
1. CREATE nanoflow with valid body
2. Attempt `create or modify` with invalid body (disallowed action)
3. DESCRIBE — verify original version preserved

### 16.5 BSON roundtrip data integrity
For 10+ complex nanoflows (error handling, annotations, 10+ activities, multiple parameter types, JavaScript action calls, association retrieves):
1. DESCRIBE → capture
2. DROP
3. Execute captured MDL
4. DESCRIBE → capture again
5. Diff — any difference is a data loss bug

Include nanoflows with:
- `call javascript action` actions (verify syntax preserved, not lost)
- `retrieve $X from $Y/Module.Association` actions (verify association syntax preserved, not converted to database retrieve)

### 16.6 Double DROP
```
drop nanoflow M.X;
drop nanoflow M.X;
```
**Expected:** First succeeds, second gives "not found" error.

### 16.7 GRANT on just-dropped nanoflow
1. CREATE nanoflow, then DROP
2. `grant execute on nanoflow M.Dropped to M.User;`

**Expected:** Clear error. No phantom entry.

### 16.8 CREATE OR MODIFY — full body replacement
1. CREATE nanoflow with 5-activity body
2. CREATE OR MODIFY same name with different 3-activity body
3. DESCRIBE — verify old body fully replaced

### 16.9 Dangling cross-reference after callee drop
1. CREATE nanoflow A calling nanoflow B
2. DROP B
3. DESCRIBE A — verify no crash
4. `mxcli describe nanoflow -p <mpr> --format mermaid M.A` — verify graceful handling

**Expected:** Stale name rendered or error message. No panic.

### 16.10 Error message quality
For each error scenario, verify the message includes:
- **What** went wrong
- **Which** nanoflow (qualified name)
- **Actionable guidance** where applicable

Scenarios: not-found (DESCRIBE, DROP, GRANT, REVOKE, MOVE, SHOW ACCESS), not-connected (CREATE, DROP, GRANT, REVOKE), validation failure, duplicate CREATE.

### 16.11 Empty string and unicode names
```
create nanoflow MyModule.Nañoflow_テスト () begin end;
```
**Expected:** Consistent behavior — accepts and roundtrips, or rejects with clear error.

Also test empty name — should be rejected.

### 16.12 Very long MDL statement
CREATE nanoflow with 100-character name, 10 parameters, 20-line body with nested control flow.
**Expected:** Parses without truncation or buffer issues. DESCRIBE output complete.

---

## 17. SECURITY CASCADES

### 17.1 Multi-role accumulation
1. `grant execute on nanoflow M.N to M.RoleA;`
2. `grant execute on nanoflow M.N to M.RoleB;`
3. `grant execute on nanoflow M.N to M.RoleC;`
4. `show access on nanoflow M.N;` — verify all 3 roles present

### 17.2 Selective revoke
1. GRANT roles A, B, C
2. `revoke execute on nanoflow M.N from M.RoleB;`
3. `show access` — verify A and C remain, B removed

### 17.3 Revoke all then re-grant
1. GRANT A, B, C
2. REVOKE A, B, C individually
3. `show access` — verify empty
4. GRANT A
5. `show access` — verify only A

### 17.4 Cross-module role reference
```
grant execute on nanoflow ModuleA.Nano to ModuleB.UserRole;
```
**Expected:** Cross-module role reference accepted. SHOW ACCESS displays `ModuleB.UserRole`.

### 17.5 Security persistence
1. CREATE nanoflow, GRANT 2 roles
2. Disconnect from project
3. Reconnect
4. `show access` — verify roles persisted

### 17.6 Security after CREATE OR MODIFY
1. CREATE nanoflow, GRANT roles A and B
2. `create or modify nanoflow M.N () begin $x : String = 'changed'; end;`
3. `show access` — verify A and B preserved

### 17.7 Bulk grant in script
```
grant execute on nanoflow M.N1 to M.User, M.Admin;
grant execute on nanoflow M.N2 to M.User, M.Admin;
grant execute on nanoflow M.N3 to M.User, M.Admin;
show access on nanoflow M.N1;
show access on nanoflow M.N2;
show access on nanoflow M.N3;
```
**Expected:** All 3 nanoflows have both roles.

### 17.8 Grant with non-existent role
```
grant execute on nanoflow M.Nano to M.NonExistentRole;
```
**Expected:** Clear error. No partial grant. SHOW ACCESS unchanged.

---

## 18. BOUNDARY & STRESS

### 18.1 Maximum parameters (20)
```
create nanoflow M.ManyParams (
  P1 : String, P2 : Integer, P3 : Boolean, P4 : Decimal, P5 : DateTime,
  P6 : String, P7 : Integer, P8 : Boolean, P9 : Decimal, P10 : DateTime,
  P11 : String, P12 : Integer, P13 : Boolean, P14 : Decimal, P15 : DateTime,
  P16 : String, P17 : Integer, P18 : Boolean, P19 : Decimal, P20 : DateTime
) returns String
begin
end;
```
DESCRIBE — verify all 20 preserved. Roundtrip — compare output.

### 18.2 Deeply nested control flow (4+ levels)
```
create nanoflow M.DeepNest ()
begin
  if true then
    if true then
      if true then
        if true then
          log info 'level4';
        end if;
      end if;
    end if;
  end if;
end;
```
DESCRIBE — verify full 4-level nesting preserved.

### 18.3 Many activities (30+)
CREATE nanoflow with 30+ sequential log actions. DESCRIBE — verify all present. Check activity count in `show nanoflows`.

### 18.4 Empty body with complex signature
```
create nanoflow M.EmptyComplex (
  A : String, B : Integer, C : Boolean, D : Decimal, E : DateTime
) returns String
begin
end;
```
**Expected:** Empty body accepted. All parameters and return type roundtrip correctly.

### 18.5 Nanoflow calling 5+ other nanoflows
CREATE 5 target nanoflows, then one caller that calls all 5:
```
create nanoflow M.Caller () returns Boolean
begin
  call nanoflow M.Target1 ();
  call nanoflow M.Target2 ();
  call nanoflow M.Target3 ();
  call nanoflow M.Target4 ();
  call nanoflow M.Target5 ();
end;
```
DESCRIBE — verify all 5 call targets preserved.
`mxcli describe nanoflow -p <mpr> --format mermaid M.Caller` — verify all 5 nodes rendered.

### 18.6 Multiple error handling clauses
```
create nanoflow M.MultiError () returns Boolean
begin
  $R1 = call nanoflow M.Risky1 () on error continue;
  $R2 = call nanoflow M.Risky2 () on error continue;
  $R3 = call nanoflow M.Risky3 () on error continue;
end;
```
DESCRIBE — verify all 3 `on error continue` clauses preserved.

### 18.7 Annotations on every statement type
CREATE nanoflow with `@annotation`, `@caption`, `@color`, `@position` on various statement types.
DESCRIBE — verify all annotations roundtrip.
`mxcli describe nanoflow -p <mpr> --format mermaid` — verify annotations rendered.

### 18.8 100+ results listing
Run `show nanoflows;` on Evora project (93 nanoflows). CREATE 10 additional to push past 100.
**Expected:** No truncation. All listed.

### 18.9 Rapid CREATE/DROP cycle (10 iterations)
```
-- repeat 10 times:
create nanoflow M.Temp () begin end;
drop nanoflow M.Temp;
```
After all 10 cycles: `show nanoflows` shows zero temp nanoflows.
**Expected:** No resource leak, no ID collision, no catalog corruption.

### 18.10 All 25 allowed action types
CREATE single nanoflow with one instance of each allowed action type (where grammar supports it). DESCRIBE → compare to original.

> Some action types may not be expressible in current MDL grammar. Document which work and which don't.

---

## 19. ELK DIAGRAM OUTPUT (CLI `--format elk`)

ELK (Eclipse Layout Kernel) JSON output is used by the VS Code extension to render interactive SVG diagrams for nanoflows.

### 19.1 Simple nanoflow — ELK JSON structure
```bash
mxcli describe nanoflow -p <mpr> --format elk <Module.SimpleNanoflow>
```
**Expected:** Valid JSON with keys: `format` (`"elk"`), `type` (`"nanoflow"`), `name`, `parameters`, `returnType`, `nodes`, `edges`, `mdlSource`, `sourceMap`. At least one start node and one end node.

### 19.2 Complex nanoflow — nodes and edges
```bash
mxcli describe nanoflow -p <mpr> --format elk <Module.ComplexNanoflow>
```
Use a nanoflow with 5+ activities (if/else, retrieve, call, log, etc.).
**Expected:** One node per activity. Edges connect activities in correct order. Decision nodes have multiple outgoing edges. `mdlSource` contains full MDL text. `sourceMap` maps node IDs to `{startLine, endLine}`.

### 19.3 Empty nanoflow — minimal ELK
```bash
echo 'create nanoflow M.Empty () begin end;' > /tmp/elk-test.mdl
mxcli exec /tmp/elk-test.mdl -p <mpr>
mxcli describe nanoflow -p <mpr> --format elk M.Empty
```
**Expected:** Valid JSON with start node and end node only. Zero intermediate nodes. `mdlSource` shows `CREATE NANOFLOW M.Empty()`.

### 19.4 Nanoflow with parameters and return type
```bash
mxcli describe nanoflow -p <mpr> --format elk <Module.NanoflowWithParams>
```
**Expected:** `parameters` array in JSON lists all parameters with names and types. `returnType` populated. These fields match DESCRIBE output.

### 19.5 Cross-project ELK — verify on all test projects
Run `--format elk` on one complex nanoflow from each test project:
- EnquiriesManagement
- Evora-FactoryManagement
- LatoProductInventory

**Expected:** All produce valid JSON. Entity names in `mdlSource` resolve correctly (qualified `Module.Entity` format).

### 19.6 Non-existent nanoflow — error
```bash
mxcli describe nanoflow -p <mpr> --format elk M.DoesNotExist
```
**Expected:** Error message: `nanoflow not found: M.DoesNotExist`

### 19.7 Microflow ELK still works (no regression)
```bash
mxcli describe microflow -p <mpr> --format elk <Module.SomeMicroflow>
```
**Expected:** Valid ELK JSON, same structure as before. Confirms `buildEntityNames` refactoring did not break microflow path.

### 19.8 ELK source map correctness
For a nanoflow with 3+ activities, verify `sourceMap` entries:
- Each node ID in `nodes` has a corresponding `sourceMap` entry
- `startLine` < `endLine` for multi-line activities
- Line numbers correspond to actual lines in `mdlSource`

---

## Test Project Coverage Matrix

| Category | Enquiries (79) | Evora Factory (93) | Lato Inventory (51) |
|---|---|---|---|
| SHOW count | Verify: 79 | Verify: 93 | Verify: 51 |
| DESCRIBE (sample 10+) | Diverse activities | Diverse activities | Diverse activities |
| Mermaid (sample 5) | Complex flows | Complex flows | Complex flows |
| SHOW ACCESS (sample 5) | With/without roles | With/without roles | With/without roles |
| Catalog query | Full table | Full table | Full table |
| Roundtrip (sample 10+) | Describe→Drop→Create→Describe | Same | Same |
| Activity coverage | Track 25 allowed types | Same | Same |
| Multi-step workflows (§15) | Project entities for call chains | Same | Same |
| BSON data integrity (§16.5) | 10+ complex nanoflows | Same | Same |
| Security cascades (§17) | Project roles | Same | Same |
| 100+ listing (§18.8) | N/A (79) | CREATE extras to reach 100+ | N/A (51) |
| ELK diagram (§19) | Sample complex | Sample complex | Sample complex |

---

## Automated Test Coverage

| Area | Tests | Status |
|---|---|---|
| Catalog: activity count | `TestCountNanoflowActivities` | Covered |
| Catalog: complexity | `TestCalculateNanoflowComplexity` | Covered |
| Registry: handler registration | `registry_test.go` | Covered |
| CREATE NANOFLOW | 13 integration + 4 mock | Covered |
| DROP NANOFLOW | 2 integration + 1 mock | Covered |
| GRANT/REVOKE | 3 integration + 5 mock | Covered |
| SHOW NANOFLOWS | 2 integration + 2 mock | Covered |
| DESCRIBE NANOFLOW | 2 integration + 2 mock | Covered |
| SHOW ACCESS | 3 mock | Covered |
| MOVE NANOFLOW | 1 integration + 1 mock | Covered |
| Mermaid output | 1 integration | Covered |
| Nanoflow validation | 6 mock | Covered |
| BSON parser | 5 roundtrip | Covered |
| BSON writer | 5 roundtrip | Covered |
| Diff output | None | **Gap** |
| ELK diagram | None | **Manual only** |
| Roundtrip (integration) | 3 integration | Covered |
| Multi-step workflows (§15) | None | **Manual only** |
| Failure modes (§16) | Partial | **Mostly manual** |
| Security cascades (§17) | Partial | **Mostly manual** |
| Boundary cases (§18) | None | **Manual only** |

Manual testing priority:
1. Roundtrip all 223 nanoflows (bulk DESCRIBE→DROP→CREATE→DESCRIBE)
2. Activity type coverage (all 25 allowed actions)
3. Multi-step workflows (§15) — highest interaction bug risk
4. Failure modes (§16) — especially §16.5 and §16.8
5. Diff output with nanoflow changes

---

## Manual Test Report Template

Copy and fill in after running manual tests.

```markdown
## Manual Testing

**Date:** YYYY-MM-DD
**Branch:** pr4-nanoflows-all
**Build:** `make build && make test && make lint-go` — PASS

### Test Projects

| App | Studio Pro | Nanoflows | SHOW count | DESCRIBE sample | Mermaid (`--format mermaid`) | Roundtrip |
|-----|-----------|-----------|------------|-----------------|------------------------------|-----------|
| Lato Enquiry Management | 11.4.0 | 79 | ✅ 79 | ✅ _n_/79 | ✅ _n_/79 | ✅ _n_/79 |
| Evora Factory Management | 10.24.15 | 93 | ✅ 93 | ✅ _n_/93 | ✅ _n_/93 | ✅ _n_/93 |
| Lato Product Inventory | 11.2.0 | 51 | ✅ 51 | ✅ _n_/51 | ✅ _n_/51 | ✅ _n_/51 |

### Command Coverage

| Command | Tested | Notes |
|---------|--------|-------|
| SHOW NANOFLOWS | ✅/❌ | |
| SHOW NANOFLOWS IN module | ✅/❌ | |
| DESCRIBE NANOFLOW | ✅/❌ | |
| `--format mermaid` | ✅/❌ | |
| CREATE NANOFLOW | ✅/❌ | |
| CREATE OR MODIFY NANOFLOW | ✅/❌ | |
| DROP NANOFLOW | ✅/❌ | |
| CALL NANOFLOW (in body) | ✅/❌ | |
| GRANT EXECUTE ON NANOFLOW | ✅/❌ | |
| REVOKE EXECUTE ON NANOFLOW | ✅/❌ | |
| SHOW ACCESS ON NANOFLOW | ✅/❌ | |
| RENAME NANOFLOW | ✅/❌ | |
| MOVE NANOFLOW | ✅/❌ | |

### Bulk Roundtrip Results

> Expression whitespace is normalized during roundtrip: `find($x,'y')` → `find($x, 'y')`. This is by-design.

```
Total: _n_ nanoflows tested
Passed: _n_
Failed: _n_ (list failures below)
```

### Activity Type Coverage

| # | Activity | Found in test project | Roundtrip OK |
|---|----------|-----------------------|-------------|
| 1 | CreateVariable | | |
| 2 | ChangeVariable | | |
| ... | ... | | |

### Validation Tests

| Scenario | Result | Notes |
|----------|--------|-------|
| Disallowed action rejected | ✅/❌ | |
| Binary return type rejected | ✅/❌ | |
| Nested disallowed action rejected | ✅/❌ | |
| Cross-ref to non-existent target | ✅/❌ | |

### Multi-Step Workflows (§15)

| Scenario | Result | Notes |
|----------|--------|-------|
| 15.1 Scaffold module | ✅/❌ | |
| 15.2 Rename in call chain | ✅/❌ | |
| 15.3 Move and reorganize | ✅/❌ | |
| 15.4 Iterative CREATE OR MODIFY | ✅/❌ | |
| 15.5 Drop/recreate different sig | ✅/❌ | |
| 15.6 Cross-module call chain | ✅/❌ | |

### Failure Modes (§16)

| Scenario | Result | Notes |
|----------|--------|-------|
| 16.1 Validation mid-batch | ✅/❌ | |
| 16.4 DESCRIBE after failed modify | ✅/❌ | |
| 16.5 BSON data integrity (10+) | ✅/❌ | |
| 16.8 Full body replacement | ✅/❌ | |
| 16.9 Dangling cross-reference | ✅/❌ | |

### Security Cascades (§17)

| Scenario | Result | Notes |
|----------|--------|-------|
| 17.1 Multi-role accumulation | ✅/❌ | |
| 17.5 Persistence through save | ✅/❌ | |
| 17.6 Preserved after modify | ✅/❌ | |

### Boundary Cases (§18)

| Scenario | Result | Notes |
|----------|--------|-------|
| 18.1 20 parameters | ✅/❌ | |
| 18.2 4-level nesting | ✅/❌ | |
| 18.9 Rapid CREATE/DROP x10 | ✅/❌ | |
| 18.10 All 25 action types | ✅/❌ | |

### Issues Found

1. (none / describe issues here)
```
