# Microflow Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/386)

## Setup

> See [AGENT-TESTING.md](./AGENT-TESTING.md) for build, execution methods, and verification patterns.

---

## 1. SHOW MICROFLOWS

### 1.1 List all microflows

```
show microflows;
```

**Expected:** Table with columns `Qualified Name | Module | Name | Excluded | Folder | Params | Actions | McCabe | Returns`. Summary `(N microflows)`.

### 1.2 List microflows in a module

```
show microflows in Administration;
```

**Expected:** Only microflows from `Administration` module.

### 1.3 Empty module

```
show microflows in NonExistentModule;
```

**Expected:** `Error: module not found: NonExistentModule`.

### 1.4 Verify excluded microflows

```
show microflows;
```

**Expected:** Excluded microflows show `true` in `Excluded` column (non-excluded show `false`).

---

## 2. DESCRIBE MICROFLOW

### 2.1 Simple microflow

```
describe microflow Administration.NewAccount;
```

**Expected:** Roundtrippable MDL:
```
create or modify microflow Administration.NewAccount (
  $AccountName: String
)
begin
  ...
end;
/
```

> **Note:** `returns Void` is implicit — `describe` omits it for Void microflows.

### 2.2 Microflow with parameters and return type

```
describe microflow <Module.MfWithReturn>;
```

**Expected:** Shows `returns <Type> as $VarName` with the AS clause when return variable name differs from default.

### 2.3 Microflow with security roles

```
describe microflow <Module.SecuredMf>;
```

**Expected:** Output ends with `grant execute on microflow Module.Name to Role1, Role2;` before `/`.

### 2.4 Microflow with error handling

```
describe microflow <Module.MfWithErrorHandling>;
```

**Expected:** Activities show `on error continue`, `on error rollback`, or `on error { ... }` blocks.

### 2.5 Microflow with annotations

```
describe microflow <Module.AnnotatedMf>;
```

**Expected:** Shows `@annotation 'text'`, `@position(x,y)`, `@caption 'text'`, `@color Color` on activities.

### 2.6 Excluded microflow

```
describe microflow <Module.ExcludedMf>;
```

**Expected:** Output starts with `@excluded` before `create or modify microflow`.

### 2.7 Microflow in folder

```
describe microflow <Module.FolderMf>;
```

**Expected:** Shows `folder 'path/to/folder'` after returns clause.

### 2.8 Non-existent microflow

```
describe microflow Fake.Missing;
```

**Expected:** Error — not found.

---

## 3. CREATE MICROFLOW

### 3.1 Minimal microflow (no-op)

```
create microflow MyModule.MF_DoNothing ()
returns Void
begin
end;
```

**Expected:** Microflow created. `show microflows in MyModule` includes it.

### 3.2 Microflow with parameters

```
create microflow MyModule.MF_Greet (
  $Name: String,
  $Count: Integer
)
returns String
begin
  return 'Hello ' + $Name;
end;
```

**Expected:** Created with 2 parameters, returns String.

### 3.3 Entity parameter (object)

```
create microflow MyModule.MF_ProcessOrder (
  $Order: MyModule.Order
)
returns Void
begin
  commit $Order with events;
end;
```

**Expected:** Parameter type is `MyModule.Order`. Activity uses `commit ... with events`.

### 3.4 Variable declaration and assignment

```
create microflow MyModule.MF_Variables ()
returns String
begin
  declare $greeting String = 'Hello';
  set $greeting = $greeting + ' World';
  return $greeting;
end;
```

**Expected:** Created. `describe` shows declare + set + return.

### 3.5 If/else control flow

```
create microflow MyModule.MF_CheckAge (
  $Age: Integer
)
returns String
begin
  if $Age >= 18 then
    return 'Adult';
  else
    return 'Minor';
  end if;
end;
```

**Expected:** Created with if/else structure.

### 3.6 Loop

```
create microflow MyModule.MF_SumPrices (
  $Products: List of MyModule.Product
)
returns Decimal
begin
  declare $Total Decimal = 0;
  loop $Item in $Products
  begin
    set $Total = $Total + $Item/Price;
  end loop;
  return $Total;
end;
```

**Expected:** Loop over list with accumulator.

### 3.7 Object create and change

```
create microflow MyModule.MF_CreateProduct (
  $Name: String,
  $Price: Decimal
)
returns MyModule.Product
begin
  $Product = create MyModule.Product (
    Name = $Name,
    Price = $Price,
    IsActive = true
  );
  commit $Product;
  return $Product;
end;
```

**Expected:** Create object + commit + return.

### 3.8 Retrieve by association

```
create microflow MyModule.MF_GetCustomer (
  $Order: MyModule.Order
)
returns MyModule.Customer
begin
  retrieve $Customer from $Order/MyModule.Order_Customer;
  return $Customer;
end;
```

**Expected:** Association retrieve.

### 3.9 Database retrieve (XPath)

```
create microflow MyModule.MF_FindActive ()
returns List of MyModule.Product
begin
  retrieve $Products from MyModule.Product where [IsActive = true] sort by Name limit 10;
  return $Products;
end;
```

**Expected:** XPath retrieve with sort, limit.

### 3.10 Call microflow

```
create microflow MyModule.MF_Orchestrate (
  $Order: MyModule.Order
)
returns Void
begin
  call microflow MyModule.MF_ProcessOrder(Order = $Order);
end;
```

**Expected:** Call action referencing another microflow.

### 3.11 Call Java action

```
create microflow MyModule.MF_WithJava (
  $Input: String
)
returns String
begin
  $Result = call java action MyModule.JA_Transform(Input = $Input);
  return $Result;
end;
```

**Expected:** Java action call (microflow-only — not available in nanoflows).

### 3.12 Show page

```
create microflow MyModule.MF_OpenDetail (
  $Product: MyModule.Product
)
returns Void
begin
  show page MyModule.Product_Detail($Product = $Product);
end;
```

**Expected:** Show page with object parameter.

### 3.13 Show message

```
create microflow MyModule.MF_Notify ()
returns Void
begin
  show message 'Operation completed successfully' type Info;
end;
```

**Expected:** Show message activity.

### 3.14 Log message

```
create microflow MyModule.MF_Logged ()
returns Void
begin
  log info node 'MyModule' 'Processing started';
end;
```

**Expected:** Log message with level and node.

### 3.15 Error handling

```
create microflow MyModule.MF_SafeCall ()
returns Boolean
begin
  call microflow MyModule.MF_Risky() on error continue;
  return true;
end;
```

**Expected:** Error handling annotation on individual activity.

> **Note:** Error handling block syntax `on error continue begin...end` is NOT supported. Error handling is an annotation on individual activities, e.g. `call microflow M.F(P = $V) on error continue;`

### 3.16 Workflow actions

```
create microflow MyModule.MF_StartWorkflow (
  $Context: MyModule.WorkflowContext
)
returns Void
begin
  call workflow MyModule.ApprovalWorkflow(Context = $Context);
end;
```

**Expected:** Workflow call activity (microflow-only).

> **Note:** The entity `MyModule.WorkflowContext` must exist before running this test. Create it as a prerequisite or substitute with any existing entity in the test project.

### 3.17 `create or modify` — existing microflow

```
create or modify microflow MyModule.MF_DoNothing ()
returns Void
begin
  log info 'Modified';
end;
```

**Expected:** Output says "Replaced microflow" (updates existing microflow body).

### 3.18 `create or replace` — drop and recreate

```
create or replace microflow MyModule.MF_DoNothing ()
returns Void
begin
  log info 'Replaced';
end;
```

**Expected:** Drops and recreates (new ID but preserves security roles via remember/consume pattern).

### 3.19 Duplicate (without modifier)

```
create microflow MyModule.MF_DoNothing ()
returns Void
begin
end;
```

**Expected:** Error — microflow already exists.

---

## 4. DROP MICROFLOW

### 4.1 Drop existing microflow

```
drop microflow MyModule.MF_DoNothing;
```

**Expected:** Microflow removed.

### 4.2 Drop non-existent microflow

```
drop microflow MyModule.Fake;
```

**Expected:** Error — not found.

---

## 5. SHOW JAVA ACTIONS

### 5.1 List all Java actions

```
show java actions;
```

**Expected:** Table with columns `Qualified Name | Module | Name | Folder`.

### 5.2 List Java actions in a module

```
show java actions in MyModule;
```

**Expected:** Only Java actions from specified module.

---

## 6. DESCRIBE JAVA ACTION

### 6.1 Basic Java action

```
describe java action <Module.JavaAction>;
```

**Expected:**
```
create java action Module.Name(
    ParamName: Type not null  -- description
)
returns Type
exposed as 'Caption' in 'Category'
as $$
  // Java code from javasource/
$$;
```

### 6.2 Java action with entity type parameter

```
describe java action <Module.GenericAction>;
```

**Expected:** Parameter shows `TypeParameter` for entity type selector (not `entity <T>` syntax).

> **Note:** The input syntax for entity type parameters (§7.2 `entity T`) is currently broken.

---

## 7. CREATE JAVA ACTION

### 7.1 Basic Java action

```
create java action MyModule.JA_Hash(
  Input: String not null
)
returns String
as $$
  return DigestUtils.sha256Hex(Input);
$$;
```

**Expected:** Java action created.

### 7.2 Java action with entity type parameter

```
create java action MyModule.JA_Export(
  Object: entity T
)
returns String
as $$
  return serialize(Object);
$$;
```

**Expected:** Entity type parameter `T` created.

### 7.3 Exposed Java action

```
create java action MyModule.JA_Public(
  Value: Integer
)
returns Integer
exposed as 'Double Value' in 'Math'
as $$
  return Value * 2;
$$;
```

**Expected:** Java action exposed in microflow toolbox under specified category.

---

## 8. SHOW JAVASCRIPT ACTIONS

### 8.1 List all JavaScript actions

```
show javascript actions;
```

**Expected:** Table with columns `Qualified Name | Module | Name | Platform | Folder`.

### 8.2 Platform values

Verify Platform column shows `Web`, `Native`, or `All`.

---

## 9. DESCRIBE JAVASCRIPT ACTION

### 9.1 Basic JavaScript action

```
describe javascript action <Module.JSAction>;
```

**Expected:**
```
create javascript action Module.Name<TypeParam>(
    ParamName: Type not null  -- description
)
  PLATFORM Web
  returns Type
  exposed as 'Caption' in 'Category'
as $$
  // JavaScript code
$$;
```

### 9.2 JavaScript action with type parameters

Verify `<T>` syntax after action name for generic type parameters.

> **Note:** CREATE JAVASCRIPT ACTION is not supported in MDL. JavaScript actions are authored externally in `.js` files under `javascriptsource/`. Only SHOW and DESCRIBE are available.

---

## 10. CALL MICROFLOW

### 10.1 Call with arguments

```
create microflow MyModule.MF_Caller ()
returns String
begin
  $Result = call microflow MyModule.MF_Greet(Name = 'World', Count = 1);
  return $Result;
end;
```

**Expected:** Arguments passed as named parameters.

### 10.2 Call with entity argument

```
create microflow MyModule.MF_CallWithObj (
  $Order: MyModule.Order
)
returns Void
begin
  call microflow MyModule.MF_ProcessOrder(Order = $Order);
end;
```

**Expected:** Entity variable passed to microflow.

### 10.3 Recursive call

```
create microflow MyModule.MF_Recursive (
  $N: Integer
)
returns Integer
begin
  if $N <= 1 then
    return 1;
  else
    $Prev = call microflow MyModule.MF_Recursive(N = $N - 1);
    return $N * $Prev;
  end if;
end;
```

**Expected:** Self-referencing call accepted.

---

## 11. GRANT / REVOKE EXECUTE ON MICROFLOW

### 11.1 Grant execute

```
grant execute on microflow MyModule.MF_ProcessOrder to MyModule.Manager, MyModule.Admin;
```

**Expected:** Roles granted. `describe` shows grant statement.

### 11.2 Revoke execute

```
revoke execute on microflow MyModule.MF_ProcessOrder from MyModule.Manager;
```

**Expected:** Role removed. `describe` shows remaining grants.

### 11.3 Grant to non-existent role

```
grant execute on microflow MyModule.MF_ProcessOrder to FakeRole;
```

**Expected:** Error — role not found.

---

## 12. SHOW ACCESS ON MICROFLOW

### 12.1 Show access

```
show access on microflow MyModule.MF_ProcessOrder;
```

**Expected:** List of roles with execute permission.

---

## 13. RENAME MICROFLOW

### 13.1 Rename microflow

```
rename microflow MyModule.MF_OldName to MF_NewName;
```

**Expected:** Microflow renamed. References updated.

---

## 14. MOVE MICROFLOW

### 14.1 Move to different module

```
move microflow MyModule.MF_Moved to OtherModule;
```

**Expected:** Microflow relocated.

### 14.2 Move to folder

```
move microflow MyModule.MF_Moved to folder 'SubFolder';
```

**Expected:** Microflow placed in folder.

---

## 15. ROUNDTRIP (BSON)

### 15.1 Simple microflow roundtrip

```
create microflow RtTest.MF_Simple (
  $Name: String
)
returns String
begin
  return 'Hello ' + $Name;
end;
```

1. `describe microflow RtTest.MF_Simple`
2. Drop: `drop microflow RtTest.MF_Simple`
3. Execute described MDL
4. `describe` again

**Expected:** Output identical between step 1 and step 4.

### 15.2 Complex microflow roundtrip

Create a microflow with multiple activity types (if/else, loop, retrieve, create object, log). Verify roundtrip preserves all activities and their order.

> **Known issue:** Negative position coordinates (`@position(-115, -20)`) snap to 0 on recreate. Caption annotation ordering may vary.

---

## 16. MULTI-STEP WORKFLOWS

### 16.1 Create entity → microflow → grant

```
create persistent entity MyModule.Order (
  Number: string(50) not null
);

create microflow MyModule.MF_CreateOrder (
  $Number: String
)
returns MyModule.Order
begin
  $Order = create MyModule.Order (Number = $Number);
  commit $Order;
  return $Order;
end;

create module role MyModule.OrderClerk;
grant execute on microflow MyModule.MF_CreateOrder to MyModule.OrderClerk;
```

**Expected:** All succeed. `describe microflow` shows grant.

### 16.2 Chain microflow calls

```
create microflow MyModule.MF_Step1 () returns Integer
begin return 1; end;

create microflow MyModule.MF_Step2 () returns Integer
begin
  $V = call microflow MyModule.MF_Step1();
  return $V + 1;
end;
```

**Expected:** Both created. Callers/callees relationship established.

---

## 17. FAILURE MODES & ERROR RECOVERY

### 17.1 Unknown activity type

```
create microflow MyModule.MF_Bad ()
returns Void
begin
  unknownaction;
end;
```

**Expected:** Parser error. No microflow created.

### 17.2 Missing parameter type

```
create microflow MyModule.MF_Bad (
  $X
)
returns Void
begin
end;
```

**Expected:** Error — parameter type required.

### 17.3 Call non-existent microflow

```
create microflow MyModule.MF_BadCall ()
returns Void
begin
  call microflow MyModule.MF_DoesNotExist();
end;
```

**Expected:** Microflow created without validation. Cross-reference validation not performed at create time.

### 17.4 Return type mismatch

```
create microflow MyModule.MF_WrongReturn ()
returns Integer
begin
  return 'not an integer';
end;
```

**Expected:** **Known issue:** Return type mismatch not validated at create time.

---

## 18. SECURITY CASCADES

### 18.1 Drop microflow with grants

```
drop microflow MyModule.MF_ProcessOrder;
```

**Expected:** Microflow and its access rules removed. `show access` no longer lists it.

### 18.2 Grant on non-existent microflow

```
grant execute on microflow MyModule.Fake to Admin;
```

**Expected:** Error — microflow not found.

---

## 19. BOUNDARY & STRESS

### 19.1 Microflow with many activities (50+)

Create a microflow with 50+ sequential activities. Verify `describe` handles large bodies.

### 19.2 Deeply nested control flow

```
create microflow MyModule.MF_Nested ()
returns Void
begin
  if true then
    if true then
      if true then
        log info 'Deep';
      end if;
    end if;
  end if;
end;
```

**Expected:** Created and described with correct indentation.

### 19.3 Many parameters (20+)

Create a microflow with 20 parameters. Verify `show` and `describe` display all.

---

## 20. DIAGRAM OUTPUT

### 20.1 ELK diagram

```
describe microflow MyModule.MF_Complex --format elk;
```

**Expected:** JSON layout data with nodes and edges for each activity.

> **Note:** ELK format flag not implemented in current build. Mark as SKIP until feature lands.

### 20.2 Mermaid diagram

Mermaid is a CLI-only output format (not MDL syntax):

```bash
mxcli describe microflow -p <mpr> --format mermaid MyModule.MF_Complex
```

**Expected:** Mermaid flowchart syntax showing activity flow.

---

## Activity Coverage Table

| Activity Type | Test Case | Section |
|---------------|-----------|---------|
| declare/set variable | 3.4 | CREATE |
| if/else | 3.5 | CREATE |
| loop | 3.6 | CREATE |
| create object | 3.7 | CREATE |
| commit (with events) | 3.3 | CREATE |
| retrieve (association) | 3.8 | CREATE |
| retrieve (XPath) | 3.9 | CREATE |
| call microflow | 3.10, 10.x | CREATE, CALL |
| call java action | 3.11 | CREATE |
| show page | 3.12 | CREATE |
| show message | 3.13 | CREATE |
| log message | 3.14 | CREATE |
| error handling | 3.15 | CREATE |
| workflow call | 3.16 | CREATE |
| return | 3.2, 3.4–3.9 | CREATE |

---

## Test Project Coverage Matrix

| Operation | Lato Enquiry | Evora Factory | Lato Product |
|-----------|:---:|:---:|:---:|
| SHOW MICROFLOWS | x | x | x |
| DESCRIBE MICROFLOW | x | x | x |
| CREATE MICROFLOW | x | | |
| DROP MICROFLOW | x | | |
| SHOW JAVA ACTIONS | x | x | x |
| DESCRIBE JAVA ACTION | x | x | |
| CREATE JAVA ACTION | x | | |
| SHOW JS ACTIONS | x | x | x |
| DESCRIBE JS ACTION | x | x | |
| CALL MICROFLOW | x | | |
| GRANT/REVOKE | x | | |
| RENAME/MOVE | x | | |

---

## Automated Test Coverage

| Section | Automated | Manual-only |
|---------|:---------:|:-----------:|
| 1. SHOW MICROFLOWS | Mock tests | |
| 2. DESCRIBE MICROFLOW | Mock tests | Error handling, annotations |
| 3. CREATE MICROFLOW | Mock tests | Complex activities |
| 4. DROP MICROFLOW | Mock tests | |
| 5–6. JAVA ACTIONS | Mock tests | |
| 7. CREATE JAVA ACTION | Mock tests | Entity type params |
| 8–9. JS ACTIONS | Mock tests | |
| 10. CALL MICROFLOW | Mock tests | |
| 11. GRANT/REVOKE | Mock tests | |
| 12. SHOW ACCESS | Mock tests | |
| 13–14. RENAME/MOVE | Mock tests | |
| 15. Roundtrip | | Complex flows |
| 16. Multi-step | | All manual |
| 17. Failure modes | Partial | Type mismatch |
| 18. Security cascades | | All manual |
| 19. Boundary | | All manual |
| 20. Diagram | | All manual |

---

## Manual Test Report Template

**Tester:** _______________
**Date:** _______________
**Project:** _______________

| # | Section | Test | Pass | Fail | Skip | Notes |
|---|---------|------|:----:|:----:|:----:|-------|
| 1.1 | SHOW MF | List all | | | | |
| 1.2 | SHOW MF | Filter module | | | | |
| 1.3 | SHOW MF | Empty module | | | | |
| 1.4 | SHOW MF | Excluded flag | | | | |
| 2.1 | DESCRIBE | Simple | | | | |
| 2.2 | DESCRIBE | Params + return | | | | |
| 2.3 | DESCRIBE | Security roles | | | | |
| 2.4 | DESCRIBE | Error handling | | | | |
| 2.5 | DESCRIBE | Annotations | | | | |
| 2.6 | DESCRIBE | Excluded | | | | |
| 2.7 | DESCRIBE | In folder | | | | |
| 2.8 | DESCRIBE | Not found | | | | |
| 3.1 | CREATE | Minimal | | | | |
| 3.2 | CREATE | Parameters | | | | |
| 3.3 | CREATE | Entity param | | | | |
| 3.4 | CREATE | Variables | | | | |
| 3.5 | CREATE | If/else | | | | |
| 3.6 | CREATE | Loop | | | | |
| 3.7 | CREATE | Create object | | | | |
| 3.8 | CREATE | Retrieve assoc | | | | |
| 3.9 | CREATE | Retrieve XPath | | | | |
| 3.10 | CREATE | Call microflow | | | | |
| 3.11 | CREATE | Call Java | | | | |
| 3.12 | CREATE | Show page | | | | |
| 3.13 | CREATE | Show message | | | | |
| 3.14 | CREATE | Log message | | | | |
| 3.15 | CREATE | Error handling | | | | |
| 3.16 | CREATE | Workflow call | | | | |
| 3.17 | CREATE | create or modify | | | | |
| 3.18 | CREATE | create or replace | | | | |
| 3.19 | CREATE | Duplicate error | | | | |
| 4.1 | DROP | Existing | | | | |
| 4.2 | DROP | Non-existent | | | | |
| 5.1 | JAVA SHOW | List all | | | | |
| 5.2 | JAVA SHOW | Filter module | | | | |
| 6.1 | JAVA DESC | Basic | | | | |
| 6.2 | JAVA DESC | Entity type param | | | | |
| 7.1 | JAVA CREATE | Basic | | | | |
| 7.2 | JAVA CREATE | Entity type | | | | |
| 7.3 | JAVA CREATE | Exposed | | | | |
| 8.1 | JS SHOW | List all | | | | |
| 8.2 | JS SHOW | Platform values | | | | |
| 9.1 | JS DESC | Basic | | | | |
| 9.2 | JS DESC | Type parameters | | | | |
| 10.1 | CALL | With arguments | | | | |
| 10.2 | CALL | Entity argument | | | | |
| 10.3 | CALL | Recursive | | | | |
| 11.1 | GRANT | Execute | | | | |
| 11.2 | REVOKE | Execute | | | | |
| 11.3 | GRANT | Non-existent role | | | | |
| 12.1 | ACCESS | Show access | | | | |
| 13.1 | RENAME | Rename microflow | | | | |
| 14.1 | MOVE | To module | | | | |
| 14.2 | MOVE | To folder | | | | |
| 15.1 | ROUNDTRIP | Simple | | | | |
| 15.2 | ROUNDTRIP | Complex | | | | |
| 16.1 | MULTI-STEP | Entity + mf + grant | | | | |
| 16.2 | MULTI-STEP | Chain calls | | | | |
| 17.1 | FAILURE | Unknown activity | | | | |
| 17.2 | FAILURE | Missing param type | | | | |
| 17.3 | FAILURE | Call non-existent | | | | |
| 17.4 | FAILURE | Return mismatch | | | | |
| 18.1 | SECURITY | Drop with grants | | | | |
| 18.2 | SECURITY | Grant on non-existent | | | | |
| 19.1 | BOUNDARY | Many activities | | | | |
| 19.2 | BOUNDARY | Deep nesting | | | | |
| 19.3 | BOUNDARY | Many parameters | | | | |
| 20.1 | DIAGRAM | ELK | | | | |
| 20.2 | DIAGRAM | Mermaid | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
