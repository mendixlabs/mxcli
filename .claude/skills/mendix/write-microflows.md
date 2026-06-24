# Mendix Microflow Skill

This skill provides comprehensive guidance for writing Mendix microflows in MDL (Mendix Definition Language) syntax.

## When to Use This Skill

Use this skill when:
- Writing CREATE MICROFLOW statements
- Debugging microflow syntax errors
- Converting Studio Pro microflows to MDL
- Understanding microflow control flow and structure

If you're not sure whether the logic belongs in a microflow or a nanoflow, read the next section first. The mirror lives in [write-nanoflows.md](./write-nanoflows.md) — keep both copies in sync.

## When to Use a Microflow vs a Nanoflow

| Scenario | Use |
|----------|-----|
| Querying the database | Microflow |
| Calling REST services or external actions | Microflow |
| Running Java actions | Microflow |
| File generation or download | Microflow |
| Transactional commits (rollback on error) | Microflow |
| Background scheduled logic | Microflow |
| Client-side form validation before save | Nanoflow |
| UI navigation and page routing | Nanoflow |
| Calling device features (GPS, phone, camera) | Nanoflow |
| Offline data access and local storage | Nanoflow |
| Calling JavaScript actions (NanoflowCommons) | Nanoflow |
| Showing progress indicators / confirmation dialogs | Nanoflow |

**Rule of thumb:** A nanoflow runs before the server call. A microflow IS the server call.

## Key Differences from Nanoflows

| Aspect | Microflow | Nanoflow |
|--------|-----------|----------|
| **Execution** | Server-side | Client-side (browser/mobile) |
| **Database access** | Full | No direct access |
| **Transactions** | Supported | Not supported |
| **Java actions** | Supported | Not supported |
| **JavaScript actions** | Not supported | Supported |
| **SYNCHRONIZE** | Not available | Available (offline sync) |
| **File downloads** | Supported | Not supported |
| **Error handling** | Full `ON ERROR` blocks + `RAISE ERROR` | Per-action `ON ERROR` supported; `RAISE ERROR` / `ErrorEvent` forbidden |
| **Offline** | Not available | Available |
| **Binary return type** | Supported | Not supported |

For nanoflow-specific authoring guidance, see [write-nanoflows.md](./write-nanoflows.md).

## Microflow Structure

**CRITICAL: All microflows MUST have JavaDoc-style documentation**

```mdl
/**
 * Microflow description explaining what it does
 *
 * Detailed explanation of the business logic, use cases,
 * and any important implementation notes.
 *
 * @param $Parameter1 Description of first parameter
 * @param $Parameter2 Description of second parameter
 * @returns Description of return value
 * @since 1.0.0
 * @author Team Name
 */
create microflow Module.MicroflowName (
  $Parameter1: type,
  $Parameter2: type
)
returns ReturnType as $ReturnVariable
[folder 'FolderPath']
begin
  -- Microflow logic here
  return $ReturnVariable;
end;
```

### FOLDER Option

Place microflows in folders for organization:

```mdl
create microflow MyModule.ACT_ProcessOrder ($Order: MyModule.Order)
returns boolean as $success
folder 'Orders/Processing'
begin
  -- logic
  return true;
end;
```

**Key Rules:**
- Parameters start with `$` prefix
- Return variable must be declared or used
- Every microflow must end with `return` statement
- Statements end with semicolon `;`
- Microflow ends with `/` separator

### Parameter Types

```mdl
-- Primitive types
$Name: string
$count: integer
$Amount: decimal
$IsActive: boolean
$date: datetime

-- Entity types
$Customer: Module.Entity

-- List types
$ProductList: list of Module.Product

-- Enumeration types
$status: enum Module.OrderStatus
```

## Variable Declarations

### ✅ CORRECT Syntax

```mdl
-- Primitive types with initialization
declare $Counter integer = 0;
declare $message string = 'Hello';
declare $IsValid boolean = true;
declare $Today datetime = [%CurrentDateTime%];

-- Entity types (no initialization needed)
declare $Product Test.Product;
declare $Order Shop.Order;
```

> **You cannot `declare` a list.** `declare` becomes a *Create Variable* activity,
> which Mendix does not allow to produce a list — Studio Pro rejects it with
> CE0053 ("type not allowed") and CE0038 ("value required"), and `mxcli check`
> now flags it as **MDL040**. Get lists from one of these instead:
> - a microflow **parameter**: `create microflow M.Process ($Items: list of Test.Product) ...`
> - a **retrieve**: `retrieve $Products from Test.Product where IsActive = true;`
> - a **create list**: `$Products = create list of Test.Product;`

### ❌ INCORRECT Syntax

```mdl
-- WRONG: Declaring a list variable (CE0053/CE0038, MDL040)
declare $ProductList list of Test.Product = empty;  -- use a parameter, retrieve, or create list

-- WRONG: Using AS keyword (not supported in mxcli)
declare $Product as Test.Product;  -- ERROR: parse error

-- WRONG: Missing type
declare $Counter = 0;  -- Type inference not always supported

-- WRONG: Using 'OF' instead of 'of'
declare $list list of Test.Product;  -- Case sensitive

-- WRONG: Using = empty for entity types
declare $Product Test.Product = empty;  -- Use without initialization
```

## Common Pitfalls

### 1. Entity Type Declarations

**Error**: Parse error or CE0053 - "Selected type is not allowed"

❌ **INCORRECT:**
```mdl
declare $Product as Test.Product;      -- AS keyword not supported
declare $Product Test.Product = empty; -- = empty not needed for entities
```

✅ **CORRECT:**
```mdl
declare $Product Test.Product;
declare $Order Shop.Order;
```

**Explanation**: Entity types are declared with just the type name, no `as` keyword and no `= empty` initialization.

### 2. XPath Association Navigation

**Error**: CE0117 - "Error in expression"

❌ **INCORRECT:**
```mdl
-- Using simple association name
declare $CustomerName string = $Order/Customer/Name;
set $Name = $Product/Category/Name;
```

✅ **CORRECT:**
```mdl
-- Use fully qualified association name: Module.AssociationName
declare $CustomerName string = $Order/Shop.Order_Customer/Name;
set $Name = $Product/Shop.Product_Category/Name;
```

**Explanation**: XPath navigation requires the full qualified association name in the format `Module.AssociationName`.

### 3. Missing Attributes

**Error**: Attribute references must exist in entity definition

❌ **INCORRECT:**
```mdl
-- Referencing Status when it doesn't exist in Order entity
change $Order (
  status = 'PROCESSING',
  ProcessedDate = [%CurrentDateTime%]);
```

✅ **CORRECT:**
```mdl
-- First, ensure entity has the attributes
create persistent entity Shop.Order (
  OrderNumber: string(50),
  status: string(50),          -- ← Must be defined
  ProcessedDate: datetime       -- ← Must be defined
);

-- Then reference them
change $Order (
  status = 'PROCESSING',
  ProcessedDate = [%CurrentDateTime%]);
```

### 4. Flow Must End with RETURN

**Error**: CE0105 - "Activity cannot be the last object of a flow"

❌ **INCORRECT:**
```mdl
begin
  declare $success boolean = true;
  log info 'Done';
  -- Missing RETURN!
end;
```

✅ **CORRECT:**
```mdl
begin
  declare $success boolean = true;
  log info 'Done';
  return $success;  -- ← Always required
end;
```

### 5. Unreachable Code After RETURN

**Error**: CE0104 - "Action activity is unreachable"

❌ **INCORRECT:**
```mdl
if $value < 0 then
  return false;
  log info 'This will never execute';  -- ← Unreachable!
end if;
```

✅ **CORRECT:**
```mdl
if $value < 0 then
  log info 'Value is negative';
  return false;
end if;
```

### 6. Unused Variables

**Warning**: CW0094 - "Variable 'X' is never used"

```mdl
-- Studio Pro will warn if parameters/variables are declared but never used
create microflow Test.Example (
  $ProductCode: string  -- ← Warning if never referenced
)
returns boolean as $success
begin
  set $success = true;  -- ProductCode never used
  return $success;
end;
```

### 7. Using SET on Undeclared Variables

**Error**: MDL executor validates that all variables used with `set` are declared first.

❌ **INCORRECT:**
```mdl
begin
  if $value > 10 then
    set $message = 'High';  -- ERROR: $Message not declared!
  end if;
  return true;
end;
```

✅ **CORRECT:**
```mdl
begin
  declare $message string = '';  -- Declare first
  if $value > 10 then
    set $message = 'High';  -- Now SET works
  end if;
  return true;
end;
```

**Note**: Parameters are automatically declared by the parameter list. The `returns type as $Var` syntax names the return variable but does NOT declare it - you must still use `declare $Var type = value;` if you want to use SET on it.

## Control Flow

### IF Statements

```mdl
-- Simple IF
if $value > 10 then
  set $message = 'Greater than 10';
end if;

-- IF/ELSE
if $value > 100 then
  set $Category = 'High';
else
  set $Category = 'Low';
end if;

-- Nested IF
if $Score >= 90 then
  set $Grade = 'A';
else
  if $Score >= 80 then
    set $Grade = 'B';
  else
    set $Grade = 'C';
  end if;
end if;
```

**Important**: Always close with `end if` (not just `end`).

### Enumeration Comparisons

**CRITICAL**: When comparing enumeration values, use the fully qualified enumeration value, NOT a string literal.

```mdl
-- CORRECT: Use fully qualified enumeration value
if $task/status = Module.TaskStatus.Completed then
  set $IsComplete = true;
end if;

if $Order/OrderStatus != Module.OrderStatus.Cancelled then
  -- Process the order
end if;

-- WRONG: Do NOT use string literals
-- IF $Task/Status = 'Completed' THEN  -- INCORRECT!
```

**Checking for empty enumeration:**
```mdl
if $entity/status = empty then
  -- Enumeration is not set
end if;
```

### CASE Statements (Enum Split)

Use `case` when a microflow branches on an enumeration value.

```mdl
case $Status
  when Open, Pending then
    return true;
  when (empty) then
    return false;
  else
    return false;
end case;
```

`(empty)` represents an unset enumeration value. Multiple values can share one `when` branch by separating them with commas. Case values are bare identifiers — do **not** quote them.

### Type Split And Cast Statements

Use `split type` when a microflow branches on an object's runtime specialization.
Use `cast` inside a type branch to create the specialized variable used by the branch body.

```mdl
split type $Input
case Sample.SpecializedInput
  cast $SpecificInput;
  return true;
else
  return false;
end split;
```

`case` values are qualified entity names. The optional `else` branch handles objects that do not match any listed specialization.

**`cast` only stores the output variable.** Studio Pro persists Microflows$CastAction with a single `VariableName` field — the source variable is implicit (the type-split's input). Use `cast $SpecificName;` to give the specialized variable its name. The two-variable form `$Output = cast $Source;` parses but `$Source` is dropped on roundtrip; prefer the single-variable form.

### LOOP Statements

```mdl
-- Basic loop
loop $Product in $ProductList
begin
  set $count = $count + 1;
end loop;

-- Loop with object modification
loop $Product in $ProductList
begin
  change $Product (IsActive = true);
  commit $Product;
end loop;

-- Loop with conditional logic
loop $Product in $ProductList
begin
  if $Product/IsActive then
    set $ActiveCount = $ActiveCount + 1;
  end if;
end loop;
```

**Note**:
- Loop variable (`$Product`) is scoped to the loop body
- The loop variable type is **automatically derived** from the list type (e.g., `list of Test.Product` → `Test.Product`)
- CHANGE statements inside loops use the derived type to resolve attribute names

## Object Operations

### CREATE Object

```mdl
$NewProduct = create Test.Product (
  Name = $Name,
  Code = $Code,
  IsActive = true,
  CreateDate = [%CurrentDateTime%]);
```

**Syntax Rules:**
- Variable assignment on left side (`$NewProduct =`)
- Entity type is fully qualified
- Attributes in parentheses, comma separated
- Closing `)` followed by semicolon
- Syntax aligned with CALL MICROFLOW/CALL JAVA ACTION

### CHANGE Object

```mdl
change $Product (
  Name = $NewName,
  ModifiedDate = [%CurrentDateTime%]);

-- Refresh the changed object in the client
change $Product (Name = $NewName) refresh;
```

**Note**: Only specify attributes you want to change. Syntax aligned with CREATE.

### COMMIT Object

```mdl
-- Commit without events
commit $Product;

-- Commit with events (triggers event handlers)
commit $Product with events;

-- Commit with refresh in client (updates UI after commit)
commit $Product refresh;

-- Commit with events and refresh
commit $Product with events refresh;
```

**Best Practice**: Use `with events` when you want before/after commit event handlers to execute. Use `refresh` when the committed object is displayed in the client and you want the UI to update immediately.

> **Binding a microflow to an entity event is MDL — you do NOT need to map it manually in Studio Pro.** After writing a handler microflow (e.g. a `BeforeCommit` validation), wire it directly:
> ```mdl
> alter entity Sales.Order
>   add event handler on before commit call Sales.ACT_ValidateOrder($currentObject) raise error;
> ```
> Works for `before`/`after` × `create`/`commit`/`delete`/`rollback`, and inside `create entity` too. See [generate-domain-model.md](generate-domain-model.md) for the full syntax. (There is no need for an `EVT_*` naming convention plus a manual Studio Pro mapping step.)

## List Operations

```mdl
-- Existing variable form
add $Item to $Items;

-- Expression-valued add, useful when round-tripping Studio Pro list-add values
add head($SourceItems) to $Items;
```

Use expression-valued `add` only when the expression returns an object compatible with the target list element type.

## Database Operations

### RETRIEVE Statement

```mdl
-- Retrieve all
retrieve $ProductList from Test.Product;

-- Retrieve with WHERE
retrieve $ProductList from Test.Product
  where Code = $SearchCode;

-- Retrieve with multiple conditions
retrieve $ProductList from Test.Product
  where IsActive = true
    and Price > 100;

-- Retrieve single object
retrieve $Product from Test.Product
  where Code = $ProductCode;
```

### RETRIEVE from Association (Non-Persistent Entities)

**CRITICAL**: Non-persistent entities (NPE) exist only in memory — they have no database rows. Using `retrieve from Entity where Association = $obj` issues a database query and will always return empty for NPEs.

**Always use association traversal syntax for NPEs:**

```mdl
-- ✅ CORRECT: traverse association from object (in-memory)
retrieve $Results from $Root/Module.Results_Root limit 1;
retrieve $ItemList from $Results/Module.BindingsItem_Results;

-- Inside a loop, traverse from the loop variable:
loop $Binding in $ItemList begin
  retrieve $FieldObj from $Binding/Module.Field_BindingsItem limit 1;
end loop;

-- ❌ WRONG: database query — always returns empty for NPEs
retrieve $ItemList from Module.BindingsItem
  where Module.BindingsItem_Results/Module.Results_Root = $Root;

retrieve $FieldObj from Module.Field
  where Module.Field_BindingsItem = $Binding
  limit 1;
```

**Rule of thumb**: If the entity is `Non-Persistent`, always use `retrieve from $object/Association`. Reserve `retrieve from Module.Entity where ...` for persistent entities only.

This pattern applies wherever NPEs are produced by Import Mappings (e.g. SPARQL JSON responses parsed into an NPE object tree).

**Important**:
- Use `from Module.Entity` (fully qualified)
- RETRIEVE with `limit 1` returns a **single entity**
- RETRIEVE without `limit 1` returns a **list** (`list of Module.Entity`)
- Use `limit 1` when you expect exactly one result (e.g., lookup by unique key)

**Sorting and paging** — use `sort by`, **not** `order by`:

```mdl
retrieve $Recent from Sales.Order
  where Status = Sales.OrderStatus.Open
  sort by Sales.Order.OrderDate desc, Sales.Order.OrderNumber asc
  limit $PageSize
  offset $Offset;
```

- The keyword is **`sort by`** (one or more `Module.Entity.Attr asc|desc`, comma-separated). `order by` is **not** valid on a microflow `retrieve` — it's reserved for `select ... from CATALOG.*` queries and will cause a parse error here.
- `limit` and `offset` accept **a variable or expression**, not only a literal — `limit $PageSize`, `offset $Offset`, even `limit $Base + 5` all work. A bare literal (`limit 20`) is just the simplest case.

**Enumeration attributes in WHERE**: XPath is a database query, so enum values are stored as plain strings. Both forms are valid — mxcli converts the qualified name to a string literal in BSON:

```mdl
-- Preferred: qualified name (mxcli converts to 'Open' in BSON)
retrieve $Open from Module.Order
  where [Status = Module.OrderStatus.Open];

-- Also accepted: string literal (the value key, case-sensitive)
retrieve $Open from Module.Order
  where [Status = 'Open'];

-- Multiple enum values with OR
retrieve $InProgress from Module.Order
  where [Status = Module.OrderStatus.Open or Status = Module.OrderStatus.Processing];
```

This is different from IF/SET expressions — see "Enumeration Comparisons" section above.

## XPath Navigation

### Attribute Access

```mdl
-- Read attribute
declare $ProductName string = $Product/Name;
declare $Price decimal = $Product/Price;

-- Write attribute (alternative to CHANGE)
set $Product/Price = $NewPrice;
set $Product/ModifiedDate = [%CurrentDateTime%];
```

### Association Navigation

```mdl
-- Navigate to related object
declare $CustomerName string = $Order/Shop.Order_Customer/Name;
declare $CategoryName string = $Product/Shop.Product_Category/Name;

-- Set association
set $Order/Shop.Order_Customer = $Customer;
set $Order/Shop.Order_Product = $Product;
```

**Critical**: Always use fully qualified association names (`Module.AssociationName`).

### XPath in Expressions

```mdl
-- Use in calculations
declare $MonthlyTotal decimal = $Product/MonthlyTotal;
declare $DailyAverage decimal = $MonthlyTotal div 30;

-- Use in conditions
if $Product/IsActive then
  set $count = $count + 1;
end if;

-- Combine with operators
set $TotalPrice = $Product/Price * $Quantity;
```

## Operators

### Arithmetic

```mdl
$Result = $A + $B;      -- Addition
$Result = $A - $B;      -- Subtraction
$Result = $A * $B;      -- Multiplication
$Result = $A div $B;    -- Division (use 'div', not '/')
```

**Important**: Use `div` for division, NOT `/`.

### Comparison

```mdl
$A = $B       -- Equals
$A != $B      -- Not equals
$A > $B       -- Greater than
$A >= $B      -- Greater than or equal
$A < $B       -- Less than
$A <= $B      -- Less than or equal
$A = empty    -- Check if empty/null
$A != empty   -- Check if not empty
```

### Boolean Logic

```mdl
$Result = $A and $B;    -- Logical AND
$Result = $A or $B;     -- Logical OR
$Result = not $A;       -- Logical NOT

-- Complex expressions
if $IsActive and $IsValid and $HasStock then
  set $CanProcess = true;
end if;
```

## Logging

```mdl
-- Log levels
log info 'Information message';
log warning 'Warning message';
log error 'Error message';

-- With node name
log info node 'OrderService' 'Processing order';
log warning node 'ValidationService' 'Invalid data detected';

-- With variables (use concatenation)
log info node 'OrderService' 'Order processed: ' + $OrderNumber;
log error node 'Service' 'Error: ' + $ErrorMessage;
```

## Activity Annotations

Annotations use `@` prefix syntax placed before the activity they apply to:

```mdl
-- Canvas position (always shown in DESCRIBE output)
@position(200, 200)
commit $Order with events;

-- Custom caption (overrides auto-generated caption)
@caption 'Save the order'
commit $Order with events;

-- Background color (Blue, Green, Red, Yellow, Purple, Gray)
@color Green
log info node 'App' 'Success';

-- Visual note attached to the next activity (creates AnnotationFlow)
@annotation 'Validate the order before processing'
commit $Order with events;

-- Multiple annotations stacked on a single activity
@position(400, 200)
@caption 'Persist product'
@color Blue
@annotation 'Step 2: Save to database'
commit $Product;
```

**Rules:**
- `@annotation` before an activity attaches the note to that activity
- `@annotation` before activity-binding metadata such as `@position`, `@caption`, `@color`, `@excluded`, or `@anchor` stays free-floating when later metadata binds the following activity
- `@annotation` at the end (no following activity) creates a free-floating note
- Escape single quotes by doubling: `@annotation 'Don''t forget'`
- `@position` always appears in DESCRIBE output; `@caption` only when custom; `@color` only when not Default
- DESCRIBE MICROFLOW shows `@` annotations before their activities

## Type Conversion Functions

**CRITICAL**: Use the correct parse/convert functions — wrong names cause runtime errors.

```mdl
-- String → number
parseInteger($str)         -- ✅ String to Integer (e.g. '42' → 42)
parseDecimal($str)         -- ✅ String to Decimal (e.g. '99.5' → 99.5)
parseDateTime($str, 'yyyy-MM-dd')  -- ✅ String to DateTime

-- Any → String
toString($value)           -- ✅ Any type to String

-- ❌ WRONG — these do NOT exist in Mendix expressions:
-- toInteger($str)         -- does not exist, use parseInteger()
-- toDecimal($str)         -- does not exist, use parseDecimal()
```

Common pattern when reading string values from NPE fields (e.g. SPARQL Import Mapping results):

```mdl
$NewBatch = create Module.MaterialBatch (
  QuantityKg = parseInteger($QuantityKgObj/Value),   -- ✅
  PurityLevel = parseDecimal($PurityLevelObj/Value), -- ✅
  ProductionDate = parseDateTime($DateObj/Value, 'yyyy-MM-dd')  -- ✅
);
```

## Special Values

```mdl
empty                      -- Null/empty value
[%CurrentDateTime%]        -- Current date/time
[%CurrentUser%]            -- Current user object
toString($value)           -- Convert to string
randomInt($max)            -- Random integer
```

## Complete Example

```mdl
create microflow Shop.ProcessOrder (
  $OrderNumber: string
)
returns boolean as $success
comment 'Process order with validation and status update'
begin
  declare $success boolean = false;
  declare $Order Shop.Order;

  -- Find the order
  retrieve $Order from Shop.Order
    where OrderNumber = $OrderNumber;

  -- Validate order exists
  if $Order = empty then
    log warning node 'OrderService' 'Order not found: ' + $OrderNumber;
    return false;
  end if;

  -- Validate customer association
  if $Order/Shop.Order_Customer = empty then
    log error node 'OrderService' 'Order has no customer';
    return false;
  end if;

  -- Update order status
  change $Order (
    status = 'PROCESSING',
    ProcessedDate = [%CurrentDateTime%]);

  commit $Order with events;

  -- Log success
  log info node 'OrderService' 'Order processed: ' + $OrderNumber;
  set $success = true;
  return $success;
end;
/
```

## Calling Microflows

### ✅ CORRECT Syntax

```mdl
-- Call with result assignment (no SET keyword)
$Result = call microflow Module.ProcessOrder(Order = $Order);

-- Call without result (void microflow)
call microflow Module.SendNotification(message = $message);

-- Call with error handling
$Result = call microflow Module.ExternalService(data = $data) on error continue;
```

### ❌ INCORRECT Syntax

```mdl
-- WRONG: Do NOT use SET with CALL MICROFLOW
set $Result = call microflow Module.ProcessOrder(Order = $Order);  -- ERROR!

-- CORRECT: Direct variable assignment
$Result = call microflow Module.ProcessOrder(Order = $Order);
```

**Important**: The `set` keyword is for changing existing variable values, NOT for capturing microflow return values. Use direct assignment (`$var = call microflow ...`).

### Parameter Name Matching

**CRITICAL**: Parameter names in `call microflow` must **exactly match** the parameter names declared in the target microflow's signature (without the `$` prefix). A mismatch causes a build error (MxBuild) but may fail silently at MDL execution time.

```mdl
-- Target microflow declaration:
create microflow Module.SendEmail ($Recipient: string, $Subject: string)
begin ... end;

-- CORRECT: parameter names match the declaration
call microflow Module.SendEmail(Recipient = $Email, Subject = $title);

-- WRONG: parameter name does not match (EmailAddress vs Recipient)
call microflow Module.SendEmail(EmailAddress = $Email, Subject = $title);  -- BUILD ERROR!
```

When calling microflows, always check the target's parameter list. Use `describe microflow Module.Name` to see the exact parameter names.

## Page Navigation

### SHOW PAGE

```mdl
-- Open page with parameter (canonical syntax)
show page Module.EditPage($Product = $Product);

-- Widget-style syntax also accepted in microflows
show page Module.EditPage(Product: $Product);
```

Both `($Param = $value)` and `(Param: $value)` syntaxes are accepted in microflow SHOW PAGE statements. Similarly, widget Action: properties accept both `show_page Module.Page(Param: $value)` and `show_page Module.Page($Param = $value)`.

### CLOSE PAGE

```mdl
close page;
```

### SHOW HOME PAGE

```mdl
show home page;
```

## Implicit Variable Creation (CE0111 Duplicate Variable)

These statements **implicitly create a new variable** with the name on the left side:

- `$Var = call microflow ...`
- `$Var = call java action ...`
- `$Var = call nanoflow ...`
- `$Var = create Module.Entity (...)`
- `retrieve $Var from Module.Entity ...`

**Do NOT use `declare` before these** — it creates a duplicate variable (CE0111):

```mdl
-- WRONG: Duplicate variable — DECLARE + CALL both create $Result
declare $Result boolean = false;
$Result = call java action Module.DoSomething();  -- CE0111!

-- CORRECT: Let CALL create the variable, use a different name if you need a default
declare $success boolean = false;
$CallResult = call java action Module.DoSomething();
set $success = $CallResult;

-- CORRECT: Simple pass-through (no default needed)
$Result = call java action Module.DoSomething();
return $Result;
```

The same applies to RETRIEVE:

```mdl
-- WRONG: declaring a list at all (CE0053/CE0038, MDL040) — and duplicate variable
declare $Items list of Module.Entity = empty;
retrieve $Items from Module.Entity where Active = true;  -- CE0111!

-- CORRECT: Let RETRIEVE create the variable
retrieve $Items from Module.Entity where Active = true;
```

**Note**: `returns type as $Var` in the microflow signature does NOT create an activity variable — it only names the return value. So `$Var = call java action ...` after `returns as $Var` is fine (one creation).

## Legacy SOAP Web Service Calls

`call web service` preserves legacy Mendix SOAP activities. Prefer REST clients
for new integrations; this syntax exists mainly so existing projects can
round-trip without dropping SOAP actions.

```mdl
-- Structured form. Resolved SOAP references use normal qualified names.
$Root = call web service SampleSOAP.OrderService
operation FetchSampleItems
send mapping SampleSOAP.OrderRequest
receive mapping SampleSOAP.OrderResponse
timeout 30
on error rollback;

-- Quoted raw IDs are accepted when old project references are dangling or unavailable.
$Root = call web service 'sample-service-id'
operation FetchSampleItems
send mapping 'sample-send-mapping-id'
receive mapping 'sample-receive-mapping-id';

-- Raw escape hatch emitted for unsupported SOAP fields.
$Root = call web service raw 'AQID';
```

**Design note:** the raw payload is base64-encoded BSON for the complete action
and is authoritative on re-exec. Treat this as round-trip support, not a
recommended authoring format for new integrations.

## REST Service Calls

MDL supports two patterns for calling REST APIs from microflows:

### SEND REST REQUEST — Consumed REST Service Operations

Calls an operation defined in a consumed REST service (created via `create rest client`). The URL, headers, authentication, and response mapping are configured in the REST client document — the microflow only references the operation.

```mdl
-- Fire and forget (RESPONSE NONE operation)
send rest request Module.ServiceName.OperationName;

-- With output variable (RESPONSE JSON operation — maps to entity)
$Result = send rest request Module.ServiceName.OperationName;

-- With request body (POST/PUT operations)
$Result = send rest request Module.ServiceName.CreateItem
    body $NewItem;
```

**CRITICAL: `$latestHttpResponse` system variable**

After every `send rest request`, Mendix automatically populates `$latestHttpResponse` (type `System.HttpResponse`). Use this to check call success — do **NOT** check the output variable directly:

```mdl
-- ✅ CORRECT: check $latestHttpResponse
$RootResult = send rest request Module.Service.GetData;
if $latestHttpResponse/content != empty then
  -- Process $RootResult (the mapped entity)
end if;

-- ❌ WRONG: checking the output variable directly causes CE0117
if $RootResult != empty then  -- ERROR!
```

**Key attributes on `$latestHttpResponse`:**
- `content` (String) — response body as string
- `StatusCode` (Integer) — HTTP status code (200, 404, etc.)

**Restrictions:**
- `send rest request` does **NOT** support custom error handling (`on error continue/rollback` causes CE6035). Errors are always handled by aborting.
- The operation must be defined via `create rest client` with a three-part qualified name: `Module.ServiceDocument.OperationName`.

### REST CALL — Inline HTTP Calls

Direct HTTP call with URL, headers, auth, body, and response handling specified inline. Useful for one-off calls or when no REST client document exists.

```mdl
-- Simple GET returning string
$response = rest call get 'https://api.example.com/data'
    header Accept = 'application/json'
    timeout 30
    returns string;

-- POST with JSON body
$response = rest call post 'https://api.example.com/items'
    header 'Content-Type' = 'application/json'
    header Accept = 'application/json'
    body '{{"name": "{1}", "value": {2}}' with (
        {1} = $ItemName,
        {2} = toString($ItemValue)
    )
    timeout 30
    returns string
    on error continue;

-- GET with URL template parameters
$response = rest call get 'https://api.example.com/users/{1}' with (
    {1} = toString($UserId)
)
    header Accept = 'application/json'
    returns string;

-- With basic authentication
$response = rest call get 'https://api.example.com/secure'
    header Accept = 'application/json'
    auth basic $username password $password
    timeout 30
    returns string;

-- DELETE (no response)
rest call delete 'https://api.example.com/items/{1}' with (
    {1} = $ItemId
)
    returns nothing
    on error continue;
```

**REST CALL response types:**
- `returns string` — response body as string variable
- `returns nothing` / `returns none` — ignore response
- `returns response` — returns `System.HttpResponse` object
- `returns mapping Module.ImportMapping as Module.Entity` — single object result
- `returns mapping Module.ImportMapping as list of Module.Entity` — list result

**Pick `as` vs `as list of` based on the call site, not the mapping shape.** The same import mapping can yield either a single object or a list — Studio Pro stores the cardinality on the microflow's `ImportMappingCall` (`Range.SingleObject` + `ForceSingleOccurrence`). Use `as Module.Entity` when the response is a single object (the mapping may still be list-typed; Studio Pro binds the first item). Use `as list of Module.Entity` when the response should bind a list. Mismatching the cardinality with the surrounding code produces `mx check` `CE0117` at the End event or `CE0013` / `CE0100` on downstream loop / aggregate / list-operation activities.

**REST CALL supports full error handling** (`on error continue`, `on error rollback`, custom error handlers).

## File Downloads

Use `download file` to stream a `System.FileDocument` from a microflow. Add
`show in browser` when the action should open the file inline instead of forcing
a download.

```mdl
download file $GeneratedReport show in browser;
download file $GeneratedExport;
```

## Empty Java-Action Argument (`empty`)

When `describe` round-trips a Java-action call that has an unbound parameter
in Studio Pro, it emits `empty` as the argument value. In this Java-action
argument context, `empty` preserves the
underlying empty `BasicCodeActionParameterValue.Argument` so that the next
`describe → exec → describe` cycle stays symmetric.

```mdl
$Total = call java action SampleModule.Recalculate(
  CompanyId       = empty,
  RecalculateAll  = true,
  ItemList        = empty
);
```

New scripts should bind every parameter to a real expression. Use `empty`
for a Java-action argument only when regenerating MDL from an existing project
that already had an unbound parameter.

## Error Handling

MDL supports error handling for activities that may fail (microflow calls, commits, external service calls, etc.).

### Error Handling Types

```mdl
-- ON ERROR CONTINUE: Ignore error and continue execution
call microflow Module.RiskyOperation() on error continue;

-- ON ERROR ROLLBACK: Rollback transaction and propagate error
commit $Order with events on error rollback;

-- ON ERROR { ... }: Custom error handler with rollback
$Result = call microflow Module.ExternalService(data = $data) on error {
  log error node 'ServiceError' 'External service failed';
  return $DefaultResult;
};

-- ON ERROR WITHOUT ROLLBACK { ... }: Custom handler, keep changes
commit $Order on error without rollback {
  log warning node 'CommitError' 'Commit failed, using fallback';
  change $Order (status = 'PENDING');
};
```

### Error Handling Semantics

| Syntax | Behavior |
|--------|----------|
| `on error continue` | Catch error silently, continue normal flow |
| `on error rollback` | Rollback database changes, propagate error |
| `on error { ... }` | Execute handler block, then continue (with rollback) |
| `on error without rollback { ... }` | Execute handler block, keep database changes |

### When to Use Each Type

- **CONTINUE**: Non-critical operations where failure is acceptable
- **ROLLBACK**: Critical operations where data integrity must be preserved
- **Custom handlers**: When you need to log errors, set fallback values, or notify users

### Example: Robust External Call

```mdl
/**
 * Calls external service with error handling
 */
create microflow Module.SafeExternalCall (
  $RequestData: string
)
returns Module.Response as $response
begin
  declare $response Module.Response;

  -- Try external call with custom error handler
  $response = call microflow Module.CallExternalAPI(data = $RequestData)
    on error without rollback {
      log error node 'ExternalAPI' 'API call failed for: ' + $RequestData;
      -- Create error response
      $response = create Module.Response (
        success = false,
        message = 'External service unavailable');
    };

  return $response;
end;
/
```

## UNSUPPORTED Syntax (Will Cause Parse Errors)

**CRITICAL**: The following syntax is NOT implemented and will cause parse errors. Do NOT use these patterns:

### ROLLBACK Statement (Supported!)

```mdl
-- CORRECT: ROLLBACK is now supported
rollback $Order;

-- With REFRESH to update client UI
rollback $Order refresh;
```

**Use Case**: Revert uncommitted changes to an object. Useful when validation fails and you want to restore the object to its database state.

### RETRIEVE with LIMIT (Supported!)

```mdl
-- CORRECT: LIMIT is supported
retrieve $Product from Module.Product where IsActive = true limit 1;

-- LIMIT 1 returns a single entity (not a list)
-- Without LIMIT, returns a list
retrieve $ProductList from Module.Product where IsActive = true;
```

### WHILE Loop

```mdl
-- WHILE loops iterate while a condition is true
while $Counter < 10
begin
  set $Counter = $Counter + 1;
end while;

-- FOR EACH loops iterate over a list
loop $item in $ItemList
begin
  -- Process each item
end loop;
```

### CASE/SWITCH Statement

```mdl
-- WRONG: CASE/SWITCH not supported
case $status
  when 'Active' then set $Result = 1;
  when 'Inactive' then set $Result = 2;
  else set $Result = 0;
end case;

-- CORRECT: Use nested IF statements
if $status = 'Active' then
  set $Result = 1;
else
  if $status = 'Inactive' then
    set $Result = 2;
  else
    set $Result = 0;
  end if;
end if;
```

### TRY/CATCH Block

```mdl
-- WRONG: TRY/CATCH not supported
TRY
  commit $Order;
CATCH
  log error 'Commit failed';
end TRY;

-- CORRECT: Use ON ERROR on specific activities
commit $Order on error {
  log error 'Commit failed';
};
```

### BREAK/CONTINUE in Loops

```mdl
-- WRONG: BREAK/CONTINUE not supported
loop $item in $ItemList
begin
  if $item/Skip = true then
    continue;  -- NOT SUPPORTED
  end if;
  if $item/Stop = true then
    break;     -- NOT SUPPORTED
  end if;
end loop;

-- CORRECT: Use conditional logic
loop $item in $ItemList
begin
  if $item/Skip = false and $item/Stop = false then
    -- Process item
  end if;
end loop;
```

### Reserved Words as Identifiers

**Best practice: Always quote all identifiers** (attribute names, parameter names, entity names) with double quotes. This eliminates all reserved keyword conflicts and is always safe — quotes are stripped automatically by the parser.

> **Exception — never quote `$`-prefixed variable/parameter references.** The quote
> rule is for *bare* names (entities, attributes, associations, declared parameter
> names). Variable/parameter **references** in expressions stay **unquoted**:
> `$Customer/Name`, `$currentObject`, `retrieve … from $List`. Quoting the `$` token
> (`"$Customer"`) breaks resolution.

```mdl
create persistent entity Module."item" (
  "check": boolean default false,
  "text": string(500),
  "format": string(50),
  "value": decimal,
  "create": datetime,
  "delete": datetime
);
```

Quoted identifiers also work for microflow parameter names:
```mdl
create microflow Module."Process" ("select": string, "type": integer)
begin
  log info 'Processing';
  return;
end;
```

## Validation Checklist

Before executing a microflow script, verify:

- [ ] All entity types use `declare $var as EntityType` (not `EntityType = empty`)
- [ ] **All primitive variables are declared before SET** (`declare $var type = value;`)
- [ ] XPath association navigation uses qualified names (`Module.AssociationName`)
- [ ] All referenced attributes exist in entity definitions
- [ ] Every flow path ends with `return`
- [ ] No code appears after `return` statements
- [ ] Division uses `div` operator (not `/`)
- [ ] All entity/association names are fully qualified
- [ ] **CALL MICROFLOW parameter names exactly match target signature** (use `describe microflow` to verify)
- [ ] Microflow ends with `/` separator
- [ ] Parameters start with `$` prefix
- [ ] Proper closing for control structures (`end if`, `end loop`)

## Common Studio Pro Errors

| Error Code | Message | Fix |
|------------|---------|-----|
| CE0053 | Selected type is not allowed | Use `declare $var EntityType;` (no AS, no = empty) |
| CE0117 | Error in expression | Use qualified association names; use `not(expr)` not bare `not expr` |
| CE0104 | Action activity is unreachable | Remove code after RETURN |
| CE0105 | Must end with end event | Add RETURN statement |
| CE0008 | No action defined | Define action for activity |
| CW0094 | Variable never used | Remove unused variables or use them |
| MDL | Variable not declared | Use `declare $var type = value;` before SET |

## Tips for Success

1. **Always use fully qualified names**: `Module.Entity`, `Module.Association`
2. **Test incrementally**: Create simple microflows first, then add complexity
3. **Check entity definitions**: Ensure all attributes exist before referencing
4. **Use meaningful variable names**: `$Customer` not `$c`, `$ProductList` not `$list`
5. **Comment complex logic**: Use `--` for inline comments
6. **Log important events**: Help with debugging and auditing
7. **Handle empty cases**: Check for `= empty` before using objects
8. **Use WITH EVENTS appropriately**: Only when you need event handlers
9. **Validate before executing**: Use `mxcli check script.mdl -p app.mpr --references` to catch errors

## Related Documentation

- [MDL Syntax Guide](../../docs/02-features/mdl-syntax.md)
- [OQL Syntax Guide](../../docs/syntax-proposals/OQL_SYNTAX_GUIDE.md)
- [Microflow Examples](../../examples/doctype-tests/microflow-examples.mdl)
- [Mendix Microflow Documentation](https://docs.mendix.com/refguide/microflows/)

## Quick Reference

### Variable Declaration Pattern
```mdl
declare $primitive type = value;   -- Primitives
declare $entity Module.Entity;     -- Entities (no AS, no = empty)
-- Lists: never declare. Use a parameter, retrieve, or `$list = create list of Module.Entity;`
```

### Object Operation Pattern
```mdl
$var = create Module.Entity (attr = value);
change $var (attr = value);
commit $var [with events] [refresh];
```

### Flow Control Pattern
```mdl
if condition then ... [else ...] end if;
loop $var in $list begin ... end loop;
return $value;
```

### XPath Pattern
```mdl
$var/attributename                      -- Attribute
$var/Module.AssociationName             -- Association
$var/Module.AssociationName/attribute   -- Chained
```

### Annotation Pattern
```mdl
@position(200, 200)
@caption 'Persist order'
@color Green
@annotation 'Note about the next activity'
commit $Order;                                          -- Annotations apply here
```

### Execute Database Query Pattern
```mdl
-- Static query (3-part name: Module.Connection.Query)
$Results = execute database query Module.Conn.QueryName;

-- Dynamic SQL override
$Results = execute database query Module.Conn.QueryName
  dynamic 'SELECT * FROM table LIMIT 10';

-- Parameterized query (names must match query PARAMETER definitions)
$Results = execute database query Module.Conn.QueryName
  (paramName = $Variable);

-- Runtime connection override
$Results = execute database query Module.Conn.QueryName
  connection (DBSource = $url, DBUsername = $user, DBPassword = $Pass);

-- Fire-and-forget (no output variable)
execute database query Module.Conn.QueryName;
```
**Note:** Only `on error rollback` is supported (the default). `on error continue` is not available for this action.

### Page Navigation Pattern
```mdl
show page Module.Page($Param = $value);               -- Canonical
show page Module.Page(Param: $value);                  -- Widget-style (also valid)
close page;
show home page;
```

### Error Handling Pattern
```mdl
call microflow ... on error continue;                  -- Ignore error
call microflow ... on error rollback;                  -- Rollback on error
call microflow ... on error { log ...; return ...; };  -- Custom handler
call microflow ... on error without rollback { ... };  -- No rollback
```
