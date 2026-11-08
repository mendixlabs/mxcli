# MDL Quick Reference

Complete syntax reference for MDL (Mendix Definition Language). This is the authoritative reference for all MDL statement syntax.

## Entity Generalization (EXTENDS)

**CRITICAL: EXTENDS goes BEFORE the opening parenthesis, not after!**

```sql
-- Correct: EXTENDS before (
CREATE PERSISTENT ENTITY Module.ProductPhoto EXTENDS System.Image (
  PhotoCaption: String(200)
);

-- Wrong: EXTENDS after ) = parse error!
CREATE PERSISTENT ENTITY Module.Photo (
  PhotoCaption: String(200)
) EXTENDS System.Image;
```

## Domain Model

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Create entity | `CREATE [OR MODIFY] PERSISTENT\|NON-PERSISTENT ENTITY Module.Name (attrs);` | Persistent is default |
| Create with extends | `CREATE PERSISTENT ENTITY Module.Name EXTENDS Parent.Entity (attrs);` | EXTENDS before `(` |
| Create view entity | `CREATE VIEW ENTITY Module.Name (attrs) AS SELECT ...;` | OQL-backed; no storage, edit-in-memory, write back via source |
| Create external entity | `CREATE EXTERNAL ENTITY Module.Name FROM CONSUMED ODATA SERVICE Module.Client (...) (attrs);` | From consumed OData |
| Create external entities | `CREATE [OR MODIFY] EXTERNAL ENTITIES FROM Module.Client [INTO Module] [ENTITIES (...)];` | Bulk from $metadata |
| Drop entity | `DROP ENTITY Module.Name;` | |
| Describe entity | `DESCRIBE ENTITY Module.Name;` | Full MDL output |
| List entities | `LIST ENTITIES [IN Module];` | List all or filter by module |
| Create enumeration | `CREATE [OR MODIFY] ENUMERATION Module.Name (Value1 'Caption', ...);` | |
| Drop enumeration | `DROP ENUMERATION Module.Name;` | |
| Create association | `CREATE ASSOCIATION Module.Name FROM Parent TO Child TYPE Reference\|ReferenceSet [OWNER Default\|Both] [ON DELETE CASCADE\|RESTRICT\|SET NULL];` | |
| Drop association | `DROP ASSOCIATION Module.Name;` | |

## ALTER ENTITY

Modifies an existing entity without full replacement.

| Operation | Syntax | Notes |
|-----------|--------|-------|
| Add attribute | `ALTER ENTITY Module.Name ADD ATTRIBUTE Attr: Type [constraints];` | One action per statement |
| Drop attribute | `ALTER ENTITY Module.Name DROP ATTRIBUTE AttrName;` | |
| Modify attribute | `ALTER ENTITY Module.Name MODIFY ATTRIBUTE Attr: NewType [constraints];` | Change type/constraints |
| Rename attribute | `ALTER ENTITY Module.Name RENAME ATTRIBUTE OldName TO NewName;` | Also rewrites stored references and XPath constraints; microflow expressions are not rewritten |
| Add index | `ALTER ENTITY Module.Name ADD INDEX [IdxName] (Col1 [ASC\|DESC], ...);` | Name optional |
| Drop index | `ALTER ENTITY Module.Name DROP INDEX IdxName;` | By index name |
| Set documentation | `ALTER ENTITY Module.Name SET DOCUMENTATION 'text';` | |
| Set position | `ALTER ENTITY Module.Name SET POSITION (100, 200);` | Canvas position |

**Example:**
```sql
mdl 1;
ALTER ENTITY Sales.Customer ADD ATTRIBUTE Phone: String(50);
ALTER ENTITY Sales.Customer ADD ATTRIBUTE Notes: String(unlimited);

ALTER ENTITY Sales.Customer RENAME ATTRIBUTE Phone TO PhoneNumber;

ALTER ENTITY Sales.Customer ADD INDEX (Email);
```

## Constants

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List constants | `LIST CONSTANTS [IN Module];` | List all or filter by module |
| Describe constant | `DESCRIBE CONSTANT Module.Name;` | Full MDL output |
| Create constant | `CREATE [OR MODIFY] CONSTANT Module.Name [FOLDER 'path'] ( Type: DataType, DefaultValue: 'value' [, ExposedToClient: true] );` | The clause form `TYPE … DEFAULT …` warns MDL-DEPR136 |
| Drop constant | `DROP CONSTANT Module.Name;` | |

**Example:**
```sql
mdl 1;
CREATE CONSTANT MyModule.ApiBaseUrl ( Type: String, DefaultValue: 'https://api.example.com' );
CREATE CONSTANT MyModule.MaxRetries ( Type: Integer, DefaultValue: 3 );
CREATE CONSTANT MyModule.EnableLogging ( Type: Boolean, DefaultValue: true );
```

## OData Clients, Services & External Entities

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List consumed odata services | `LIST CONSUMED ODATA SERVICES [IN Module];` | Consumed OData services |
| Describe consumed odata service | `DESCRIBE CONSUMED ODATA SERVICE Module.Name;` | Full MDL output |
| Create consumed odata service | `CREATE [OR MODIFY] CONSUMED ODATA SERVICE Module.Name (...);` | Version, MetadataUrl, Timeout, etc. |
| Alter consumed odata service | `ALTER CONSUMED ODATA SERVICE Module.Name SET (Key: Value, ...);` | |
| Drop consumed odata service | `DROP CONSUMED ODATA SERVICE Module.Name;` | |
| List published odata services | `LIST PUBLISHED ODATA SERVICES [IN Module];` | Published OData services |
| Describe published odata service | `DESCRIBE PUBLISHED ODATA SERVICE Module.Name;` | Full MDL output |
| Create published odata service | `CREATE [OR MODIFY] PUBLISHED ODATA SERVICE Module.Name (...) AUTHENTICATION ... { PUBLISH ENTITY ... };` | |
| Alter published odata service | `ALTER PUBLISHED ODATA SERVICE Module.Name SET (Key: Value, ...);` | |
| Drop published odata service | `DROP PUBLISHED ODATA SERVICE Module.Name;` | |
| List external entities | `LIST EXTERNAL ENTITIES [IN Module];` | OData-backed entities |
| List external actions | `LIST EXTERNAL ACTIONS [IN Module];` | Actions used in microflows |
| Create external entity | `CREATE [OR MODIFY] EXTERNAL ENTITY Module.Name FROM CONSUMED ODATA SERVICE Module.Client (...) (attrs);` | |
| Create external entities | `CREATE [OR MODIFY] EXTERNAL ENTITIES FROM Module.Client [INTO Module] [ENTITIES (...)];` | Bulk from $metadata |
| Grant OData access | `GRANT ACCESS ON PUBLISHED ODATA SERVICE Module.Name TO Module.Role, ...;` | |
| Revoke OData access | `REVOKE ACCESS ON PUBLISHED ODATA SERVICE Module.Name FROM Module.Role, ...;` | |
| List contract entities | `LIST CONTRACT ENTITIES FROM Module.Client;` | Browse cached $metadata |
| List contract actions | `LIST CONTRACT ACTIONS FROM Module.Client;` | Browse cached $metadata |
| Describe contract entity | `DESCRIBE CONTRACT ENTITY Module.Client.Entity [FORMAT mdl];` | Properties, types, keys |
| Describe contract action | `DESCRIBE CONTRACT ACTION Module.Client.Action [FORMAT mdl];` | Parameters, return type |
| List contract channels | `LIST CONTRACT CHANNELS FROM Module.Service;` | Browse cached AsyncAPI |
| List contract messages | `LIST CONTRACT MESSAGES FROM Module.Service;` | Browse cached AsyncAPI |
| Describe contract message | `DESCRIBE CONTRACT MESSAGE Module.Service.Message;` | Message payload properties |
| Query contract entities | `SELECT * FROM CATALOG.CONTRACT_ENTITIES;` | Requires REFRESH CATALOG |
| Query contract actions | `SELECT * FROM CATALOG.CONTRACT_ACTIONS;` | Requires REFRESH CATALOG |
| Query contract messages | `SELECT * FROM CATALOG.CONTRACT_MESSAGES;` | Requires REFRESH CATALOG |

**OData Client Example:**
```sql
CREATE CONSUMED ODATA SERVICE MyModule.ExternalAPI (
  Version: '1.0',
  ODataVersion: OData4,
  MetadataUrl: 'https://api.example.com/odata/v4/$metadata',
  Timeout: 300
);
```

**OData Service Example:**
```sql
CREATE PUBLISHED ODATA SERVICE MyModule.CustomerAPI (
  Path: 'odata/customers/',
  Version: '1.0.0',
  ODataVersion: OData4,
  Namespace: 'MyModule.Customers',
  Authentication: (basic, session)
)
{
  PUBLISH ENTITY MyModule.Customer AS 'Customers' (
    ReadMode: SOURCE,
    InsertMode: SOURCE,
    UpdateMode: NOT_SUPPORTED,
    DeleteMode: NOT_SUPPORTED,
    UsePaging: Yes,
    PageSize: 100
  )
  -- The KEY attribute must be unique on the entity
  -- (Email: String(200) UNIQUE), or mxbuild reports CE6624.
  EXPOSE (Email (KEY), Name, Phone);
};
```

## Microflows - Supported Statements

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Variable declaration | `DECLARE $Var Type = value;` | Primitives: String, Integer, Boolean, Decimal, DateTime |
| Entity declaration | `DECLARE $Entity Module.Entity;` | No AS keyword, no = empty |
| List declaration | `DECLARE $List List of Module.Entity = empty;` | |
| Assignment | `SET $Var = expression;` | Variable must be declared first |
| Create object | `$Var = CREATE Module.Entity (Attr = value) [COMMIT [WITHOUT EVENTS]] [REFRESH];` | |
| Change object | `CHANGE $Entity (Attr = value) [COMMIT [WITHOUT EVENTS]] [REFRESH];` | |
| Commit | `COMMIT $Entity [WITHOUT EVENTS] [REFRESH];` | Omitted = with events, matching Studio Pro |
| Delete | `DELETE $Entity [REFRESH];` | |
| Rollback | `ROLLBACK $Entity [REFRESH];` | Reverts uncommitted changes |
| Retrieve (DB) | `RETRIEVE $Var FROM Module.Entity [WHERE condition];` | Database XPath retrieve |
| Retrieve (Assoc) | `RETRIEVE $List FROM $Parent/Module.AssocName;` | Retrieve by association |
| Call microflow | `$Result = CALL MICROFLOW Module.Name (Param = $value);` | |
| Call nanoflow | `$Result = CALL NANOFLOW Module.Name (Param = $value);` | |
| Show page | `SHOW PAGE Module.PageName (Param = $value);` | `($Param = …)` and `(Param: …)` are deprecated |
| Close page | `CLOSE PAGE [n];` | `n` pages, default 1 |
| Validation | `VALIDATION FEEDBACK $Entity/Attribute MESSAGE 'message';` | Requires attribute path + MESSAGE |
| Log | `LOG INFO\|WARNING\|ERROR [NODE 'name'] 'message';` | |
| Log with stack trace | `LOG ERROR 'message' [WITH ({1} = $v)] WITH STACKTRACE;` | "Include latest stack trace" |
| Position | `@position(x, y)` | Canvas position (before activity) |
| Start event | `@start(x, y)` | Canvas position of the start, on the **first** statement. Omit it and the start is placed one spacing unit left of the first activity and MOVES with it on a rewrite; a start that is not at that derived spot is treated as hand-placed, survives a rewrite, and is emitted by DESCRIBE (#951) |
| Caption | `@caption 'text'` | Custom caption (before activity) |
| Color | `@color Green` | Background color (before activity) |
| Annotation | `@annotation 'text'` | Visual note attached to next activity |
| IF | `IF condition THEN ... [ELSE ...] END IF;` | |
| LOOP | `LOOP $Item IN $List BEGIN ... END LOOP;` | FOR EACH over list |
| WHILE | `WHILE condition BEGIN ... END WHILE;` | Condition-based loop |
| Return | `RETURN $value;` | Required at end of every flow path |
| Execute DB query | `$Result = EXECUTE DATABASE QUERY Module.Conn.Query;` | 3-part name; supports DYNAMIC, params, CONNECTION override |
| Error handling | `... ON ERROR CONTINUE\|ROLLBACK\|[WITHOUT ROLLBACK] BEGIN handler END ERROR;` | Not supported on EXECUTE DATABASE QUERY |

## Microflows - NOT Supported (Will Cause Parse Errors)

| Unsupported | Use Instead | Notes |
|-------------|-------------|-------|
| `CASE ... WHEN 'String' ... ELSE ...` | Bare enum values, one branch per value | `CASE` itself IS supported for **enum splits**; what fails is quoted/qualified values, an `ELSE` branch (MDL008), and an `AS` alias |
| `TRY ... CATCH ... END TRY` | `ON ERROR BEGIN ... END ERROR` blocks | Use error handlers on specific activities |

**Notes:**
- `RETRIEVE ... FIRST` returns a single entity; `RETRIEVE ... LIMIT n [OFFSET n]` returns a list. A bare `LIMIT 1` is a list of one.
- `ROLLBACK $Entity [REFRESH];` IS supported. Rolls back uncommitted changes to an object.

## Project Organization

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Microflow folder | `FOLDER 'path'` (before BEGIN) | `CREATE MICROFLOW ... FOLDER 'ACT' BEGIN ... END;` |
| Page folder | `FOLDER 'path'` (after the name) | `CREATE PAGE ... FOLDER 'Pages/Detail' (...) { ... }` |
| Move to folder | `MOVE <doctype> Module.Name TO FOLDER 'path';` | Folders created automatically. Any top-level doctype, spelled as `DESCRIBE` spells it |
| Move a mapping / structure | `MOVE IMPORT MAPPING\|EXPORT MAPPING\|JSON STRUCTURE Module.Name TO FOLDER 'path';` | |
| Place while creating | `CREATE <doctype> Module.Name FOLDER 'path' ...` | Every doctype: a clause after the name (after the signature on microflows/nanoflows, before `BEGIN`). The `Folder: 'path'` property on pages, snippets and REST/OData services is a deprecated alias (`MDL-DEPR105`) |
| Place an existing document | `CREATE OR MODIFY ... FOLDER 'path' ...` | Moves it; omitting the clause leaves placement alone |
| Move to module root | `MOVE PAGE Module.Name TO Module;` | Removes from folder |
| Move across modules | `MOVE PAGE Old.Name TO NewModule;` | **Breaks by-name references** -- use `LIST IMPACT OF` first |
| Move to folder in other module | `MOVE PAGE Old.Name TO FOLDER 'path' IN NewModule;` | |
| Move entity to module | `MOVE ENTITY Old.Name TO NewModule;` | Entities don't support folders |

Nested folders use `/` separator: `'Parent/Child/Grandchild'`. Missing folders are auto-created.

## Security Management

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Describe app security | `DESCRIBE APP SECURITY;` | Displays security level, admin, demo users |
| List module roles | `LIST MODULE ROLES [IN Module];` | All roles or filtered by module |
| List user roles | `LIST USER ROLES;` | Project-level user roles |
| List demo users | `LIST DEMO USERS;` | Configured demo users |
| List access on element | `LIST ACCESS ON [ENTITY\|MICROFLOW\|PAGE\|NANOFLOW] Mod.Name;` | Which roles can access; a bare name means the entity |
| Describe security matrix | `DESCRIBE SECURITY MATRIX [IN Module];` | Full access overview |
| Create module role | `CREATE MODULE ROLE Mod.Role [DESCRIPTION 'text'];` | |
| Drop module role | `DROP MODULE ROLE Mod.Role;` | |
| Create user role | `CREATE USER ROLE Name ( ModuleRoles: (Mod.Role, ...), Description: '...', ManageAllRoles: true, CheckSecurity: true );` | Aggregates module roles; every property optional |
| Alter user role | `ALTER USER ROLE Name ADD\|DROP MODULE ROLES (Mod.Role, ...);` | |
| Drop user role | `DROP USER ROLE [IF EXISTS] Name;` | `IF EXISTS` makes a cleanup script re-runnable |
| Grant microflow access | `GRANT EXECUTE ON MICROFLOW Mod.MF TO Mod.Role, ...;` | |
| Revoke microflow access | `REVOKE EXECUTE ON MICROFLOW Mod.MF FROM Mod.Role, ...;` | |
| Grant page access | `GRANT VIEW ON PAGE Mod.Page TO Mod.Role, ...;` | |
| Revoke page access | `REVOKE VIEW ON PAGE Mod.Page FROM Mod.Role, ...;` | |
| Grant entity access | `GRANT CREATE, DELETE, READ *, WRITE * ON ENTITY Mod.Entity TO Mod.Role;` | Supports member lists and WHERE |
| Revoke entity access | `REVOKE ALL ON ENTITY Mod.Entity FROM Mod.Role;` | |
| Set security level | `ALTER APP SECURITY ( SecurityLevel: OFF\|PROTOTYPE\|PRODUCTION );` | |
| Toggle demo users | `ALTER APP SECURITY ( EnableDemoUsers: TRUE\|FALSE );` | |
| Toggle guest access | `ALTER APP SECURITY ( EnableGuestAccess: TRUE, GuestUserRole: UserRole );` / `( EnableGuestAccess: FALSE )` | Anonymous users; role required (CE0133) |
| Rename admin user | `ALTER APP SECURITY ( AdminUserName: 'appadmin' );` | Default `MxAdmin` |
| Set admin user role | `ALTER APP SECURITY ( AdminUserRole: AppAdmin );` | Must exist; `DROP USER ROLE` refuses the administrator's role, so move it first |
| Set admin password | `ALTER APP SECURITY ( AdminPassword: 'N3w-Admin-Passw0rd' );` | Resolves CE0129 "Administrator password has not been set"; `''` is refused |
| Create demo user | `CREATE DEMO USER 'name' ( Password: 'pass', [Entity: Module.Entity,] UserRoles: (UserRole, ...) );` | The clause form `PASSWORD … (…)` warns MDL-DEPR137 |
| Drop demo user | `DROP DEMO USER [IF EXISTS] 'name';` | `IF EXISTS` makes a cleanup script re-runnable |

## Workflows

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List workflows | `LIST WORKFLOWS [IN Module];` | List all or filter by module |
| Describe workflow | `DESCRIBE WORKFLOW Module.Name;` | Full MDL output |
| Create workflow | `CREATE [OR MODIFY] WORKFLOW Module.Name PARAMETER $Ctx: Module.Entity BEGIN ... END WORKFLOW;` | See activity types below |
| Drop workflow | `DROP WORKFLOW Module.Name;` | |
| Alter workflow | `ALTER WORKFLOW Module.Name { SET (Key: value, ...) [ON <activity>]; INSERT BEFORE\|AFTER <activity> { ... } INSERT INTO <activity> { OUTCOMES ... \| PATH { ... } \| BOUNDARY EVENT ... } REPLACE <activity> WITH { ... } DROP <activity> [OUTCOME '<x>' \| PATH <n> \| BOUNDARY EVENT]; };` | An activity is its name or `'caption'`, `@n` for one of several matches; the old per-action form warns MDL-DEPR140–149 |

**Workflow Activity Types:**
- `USER TASK <name> '<caption>' [PAGE Mod.Page] [TARGETING MICROFLOW Mod.MF] [OUTCOMES '<out>' { } ...];`
- `CALL MICROFLOW Mod.MF [CAPTION '<text>'] [OUTCOMES '<out>' { } ...];`
- `CALL WORKFLOW Mod.WF [CAPTION '<text>'];`
- `DECISION ['<caption>'] OUTCOMES '<out>' { } ...;`
- `PARALLEL SPLIT PATH 1 { } PATH 2 { };`
- `JUMP TO <activity-name>;`
- `WAIT FOR TIMER ['<expr>'];`
- `WAIT FOR NOTIFICATION;`
- `END;`

**Example:**
```sql
CREATE WORKFLOW Module.ApprovalFlow
  PARAMETER $Context: Module.Request
  OVERVIEW PAGE Module.WorkflowOverview
BEGIN
  USER TASK ReviewTask 'Review the request'
    PAGE Module.ReviewPage
    OUTCOMES 'Approve' { } 'Reject' { };
END WORKFLOW;
```

## Project Structure

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Structure overview | `DESCRIBE STRUCTURE;` | Depth 2 (elements with signatures), user modules only |
| Module counts | `DESCRIBE STRUCTURE DEPTH 1;` | One line per module with element counts |
| Full types | `DESCRIBE STRUCTURE DEPTH 3;` | Typed attributes, named parameters |
| Filter by module | `DESCRIBE STRUCTURE IN ModuleName;` | Single module only |
| Include all modules | `DESCRIBE STRUCTURE DEPTH 1 ALL;` | Include system/marketplace modules |

## Navigation

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List navigation | `LIST NAVIGATION;` | Summary of all profiles |
| List menu tree | `LIST NAVIGATION MENU [Profile];` | Menu tree for profile or all |
| List home pages | `LIST NAVIGATION HOMES;` | Home page assignments across profiles |
| Describe navigation | `DESCRIBE NAVIGATION [Profile];` | Full MDL output (round-trippable) |
| Create/replace navigation | `CREATE OR REPLACE NAVIGATION Profile ...;` | Sets the stated clauses; an omitted clause keeps its stored value, a `{ }` menu replaces the whole menu |

**Navigation Example:**
```sql
CREATE OR MODIFY NAVIGATION Responsive
  HOME PAGE MyModule.Home_Web
  HOME PAGE MyModule.AdminHome FOR Administrator
  LOGIN PAGE Administration.Login
  NOT FOUND PAGE MyModule.Custom404
  {
    MENU ITEM 'Home' ( OnClick: SHOW PAGE MyModule.Home_Web )
    MENU 'Admin' {
      MENU ITEM 'Users' ( OnClick: SHOW PAGE Administration.Account_Overview )
    }
  };
```

## Project Settings

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Describe settings | `DESCRIBE SETTINGS;` | All settings parts, as MDL |
| Describe settings | `DESCRIBE SETTINGS;` | Full MDL output (round-trippable) |
| Alter runtime settings | `ALTER SETTINGS RUNTIME (Key: Value, ...);` | AfterStartupMicroflow, HashAlgorithm, JavaVersion, etc. |
| Alter configuration | `ALTER SETTINGS CONFIGURATION 'Name' (Key: Value, ...);` | DatabaseType, DatabaseUrl, HttpPortNumber, etc. |
| Alter constant | `ALTER SETTINGS CONSTANT @Module.Name VALUE 'val' IN CONFIGURATION 'cfg';` | Override constant per configuration |
| Alter language | `ALTER SETTINGS LANGUAGE (Key: Value);` | DefaultLanguageCode |
| Alter workflows | `ALTER SETTINGS WORKFLOWS (Key: Value, ...);` | UserEntity, DefaultTaskParallelism |

## Business Events

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List services | `LIST BUSINESS EVENTS;` | List all business event services |
| List in module | `LIST BUSINESS EVENTS IN Module;` | Filter by module |
| Describe service | `DESCRIBE BUSINESS EVENT SERVICE Module.Name;` | Full MDL output |
| Create service | `CREATE BUSINESS EVENT SERVICE Module.Name (...) { MESSAGE ... };` | See help topic for full syntax |
| Drop service | `DROP BUSINESS EVENT SERVICE Module.Name;` | Delete a service |

## Java Actions

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List Java actions | `LIST JAVA ACTIONS [IN Module];` | List all or filtered by module |
| Describe Java action | `DESCRIBE JAVA ACTION Module.Name;` | Full MDL output with signature |
| Create Java action | `CREATE JAVA ACTION Module.Name(params) RETURNS type AS $$ ... $$;` | Inline Java code |
| Create with type params | `CREATE JAVA ACTION Module.Name(EntityType: ENTITY <pEntity>, Obj: pEntity) ...;` | Generic type parameters |
| Describe a parameter, name the return value | `CREATE JAVA ACTION Module.Name(/** The text. */ Input: String) RETURNS String AS $Result AS $$ ... $$;` | `/** … */` before a parameter is its description; `AS $Name` the return value's name |
| Create exposed action | `... EXPOSED AS 'Caption' IN 'Category' AS $$ ... $$;` | Toolbox-visible in Studio Pro |
| Drop Java action | `DROP JAVA ACTION Module.Name;` | Delete a Java action |
| Call from microflow | `$Result = CALL JAVA ACTION Module.Name(Param = value);` | Inside BEGIN...END |

**Parameter Types:** `String`, `Integer`, `Long`, `Decimal`, `Boolean`, `DateTime`, `Module.Entity`, `List of Module.Entity`, `ENUM Module.EnumName`, `Enumeration(Module.EnumName)`, `StringTemplate(Sql)`, `StringTemplate(Oql)`, `ENTITY <pEntity>` (type parameter declaration), bare `pEntity` (type parameter reference).

**Type Parameters** allow generic entity handling. `ENTITY <pEntity>` declares the type parameter inline and becomes the entity type selector; bare `pEntity` parameters receive entity instances:
```sql
CREATE JAVA ACTION Module.Validate(
  EntityType: ENTITY <pEntity> NOT NULL,
  InputObject: pEntity NOT NULL
) RETURNS Boolean
EXPOSED AS 'Validate Entity' IN 'Validation'
AS $$
return InputObject != null;
$$;
```

## Pages

MDL uses explicit property declarations for pages:

| Element | Syntax | Example |
|---------|-----------|---------|
| Page properties | `(Key: value, ...)` | `(Title: 'Edit', Layout: Atlas_Core.Atlas_Default)` |
| Page variables | `Variables: ( $name: Type = <expr> )` | `Variables: ( $show: Boolean = true )` |
| Widget name | Required after type | `TEXTBOX txtName (...)` |
| Attribute binding | `Attribute: AttrName` | `TEXTBOX txt (Label: 'Name', Attribute: Name)` |
| Variable binding | `DataSource: $Var` | `DATAVIEW dv (DataSource: $Product) { ... }` |
| Action binding | `Action: TYPE` | `ACTIONBUTTON btn (Caption: 'Save', Action: save changes)` |
| Microflow action | `Action: MICROFLOW Name(Param = val)` | `Action: MICROFLOW Mod.ACT_Process(Order = $Order)` |
| Database source | `DataSource: DATABASE Entity` | `DATAGRID dg (DataSource: DATABASE Module.Entity)` |
| Selection binding | `DataSource: SELECTION widget` | `DATAVIEW dv (DataSource: SELECTION galleryList)` |
| CSS class | `Class: 'classes'` | `CONTAINER c (Class: 'card mx-spacing-top-large')` |
| Inline style | `Style: 'css'` | `CONTAINER c (Style: 'padding: 16px;')` |
| Dynamic classes | `DynamicClasses: 'expr'` | `CONTAINER c (DynamicClasses: if $currentObject/IsActive then 'is-active' else '')` — runtime-computed; stacks on `Class` |
| Design properties | `DesignProperties: (...)` | `CONTAINER c (DesignProperties: ('Spacing top': 'Large', 'Full width': ON))` |
| Width (pixels) | `Width: integer` | `IMAGE img (Width: 200)` |
| Height (pixels) | `Height: integer` | `IMAGE img (Height: 150)` |
| Page size | `PageSize: integer` | `DATAGRID dg (PageSize: 25)` |
| Pagination mode | `Pagination: mode` | `DATAGRID dg (Pagination: virtualScrolling)` |
| Paging position | `PagingPosition: pos` | `DATAGRID dg (PagingPosition: both)` |
| Paging buttons | `ShowPagingButtons: mode` | `DATAGRID dg (ShowPagingButtons: auto)` |

**DataGrid Column Properties:**

| Property | Values | Default | Example |
|----------|--------|---------|---------|
| `Attribute` | attribute name | (required) | `Attribute: Price` |
| `Caption` | string | attribute name | `Caption: 'Unit Price'` |
| `Alignment` | `left`, `center`, `right` | `left` | `Alignment: right` |
| `WrapText` | `true`, `false` | `false` | `WrapText: true` |
| `Sortable` | `true`, `false` | `true`/`false` | `Sortable: false` |
| `Resizable` | `true`, `false` | `true` | `Resizable: false` |
| `Draggable` | `true`, `false` | `true` | `Draggable: false` |
| `Hidable` | `yes`, `hidden`, `no` | `yes` | `Hidable: no` |
| `ColumnWidth` | `autoFill`, `autoFit`, `manual` | `autoFill` | `ColumnWidth: manual` |
| `Size` | integer — a relative weight, not px | `1` | `Size: 2` — under `ColumnWidth: manual` only, rendered `2fr` against `1fr` per autoFill column |
| `minWidth` | `auto`, `minContent`, `manual` | `auto` | `minWidth: manual` — under `ColumnWidth: autoFit` only |
| `minWidthLimit` | integer (px) | `100` | `minWidthLimit: 220` — the minimum in pixels, under `ColumnWidth: autoFit, minWidth: manual` |
| `Visible` | expression | `true` | `Visible: $showColumn` — evaluated once for the grid: a page variable or parameter, never `$currentObject` (MDL-WIDGET43, CE0117) |
| `DynamicCellClass` | expression | (empty) | `DynamicCellClass: if $currentObject/Stock < 10 then 'text-danger' else ''` |
| `Tooltip` | text string | (empty) | `Tooltip: 'Price in USD'` |

**Page Example:**
```sql
CREATE PAGE MyModule.Customer_Edit
(
  Params: ( $Customer: MyModule.Customer ),
  Title: 'Edit Customer',
  Layout: Atlas_Core.PopupLayout
)
{
  DATAVIEW dvCustomer (DataSource: $Customer) {
    TEXTBOX txtName (Label: 'Name', Attribute: Name)
    TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
    COMBOBOX cbStatus (Label: 'Status', Attribute: Status)

    FOOTER {
      ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)
      ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)
    }
  }
};
```

**Supported Widgets:**
- Layout: `LAYOUTGRID`, `ROW`, `COLUMN`, `CONTAINER`, `CUSTOMCONTAINER`
- Input: `TEXTBOX`, `TEXTAREA`, `CHECKBOX`, `RADIOBUTTONS`, `DATEPICKER`, `COMBOBOX`
  - `COMBOBOX` modes: enumeration `(Attribute: EnumAttr)`; association `(Attribute: Assoc, DataSource: DATABASE Mod.Target, CaptionAttribute: Name)`; database `(optionsSourceDatabaseDataSource: <source>, Attribute: TargetAttr, CaptionAttribute: Name, optionsSourceDatabaseValueAttribute: ValueAttr)` — the datasource's key selects database mode, the generic `DataSource:` association mode. Custom content (several attributes per option) is a body block of the mode: `{ optionssourceassociationcustomcontent { … } }` in association mode, `{ optionssourcedatabasecustomcontent { … } }` in database mode, its widgets bound to the option entity
- Display: `DYNAMICTEXT`, `DATAGRID`, `GALLERY`, `LISTVIEW`, `IMAGE`, `STATICIMAGE`, `DYNAMICIMAGE`
- Actions: `ACTIONBUTTON`, `LINKBUTTON`, `NAVIGATIONLIST`
- Structure: `DATAVIEW`, `HEADER`, `FOOTER`, `CONTROLBAR`, `SNIPPETCALL`

## ALTER PAGE / ALTER SNIPPET

Modify an existing page or snippet's widget tree in-place without full `CREATE OR REPLACE`. Works directly on the raw BSON tree, preserving unsupported widget types.

| Operation | Syntax | Notes |
|-----------|--------|-------|
| Set property | `SET (Caption: 'New') ON widgetName` | Single property on a widget |
| Set multiple | `SET (Caption = 'Save', ButtonStyle = Success) ON btn` | Multiple properties at once |
| Page-level set | `SET (Title: 'New Title')` | No ON clause for page properties |
| Insert after | `INSERT AFTER widgetName { widgets }` | Add widgets after target |
| Insert before | `INSERT BEFORE widgetName { widgets }` | Add widgets before target |
| Insert into | `INSERT INTO containerName { widgets }` | Append as the container's last child (fills an empty container) |
| Drop widgets | `DROP name1, name2` | Remove widgets by name |
| Replace widget | `REPLACE widgetName WITH { widgets }` | Replace widget subtree |
| Pluggable prop | `SET ('showLabel': false) ON cbStatus` | Quoted name for pluggable widgets |
| Add variable | `ADD Variables $name: Type = <expr>` | Add a page variable |
| Drop variable | `DROP Variables $name` | Remove a page variable |
| Add parameter | `ADD Parameters $name: Type` | Add a page/snippet parameter (entity or primitive; snippet: entity only). A page with a `Url` needs a `{name}` segment — `SET (Url: …)` in the same statement |
| Drop parameter | `DROP Parameters $name` | Remove a parameter; refused while the page still uses it |

**Supported SET properties:** Caption, Label, ButtonStyle, Class, Style, Editable, Visible, Name, and quoted pluggable widget properties. Page-level (no `ON` clause, case-sensitive): Title, Class, Style, PopupWidth, PopupHeight, PopupResizable.

**Example:**
```sql
mdl 1;
ALTER PAGE Module.EditPage {
  SET (Caption: 'Save & Close', ButtonStyle: Success) ON btnSave;
  DROP txtUnused;
  INSERT AFTER txtEmail {
    TEXTBOX txtPhone (Label: 'Phone', Attribute: Phone)
  }
};

ALTER SNIPPET Module.NavMenu {
  SET (Caption: 'Dashboard') ON btnHome
};
```

**Tip:** Run `DESCRIBE PAGE Module.PageName` first to see widget names.

## External SQL Statements

Direct SQL query execution against external databases (PostgreSQL, Oracle, SQL Server). Credentials are isolated -- DSN never appears in session output or logs.

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Connect | `SQL CONNECT <driver> '<dsn>' AS <alias>;` | Drivers: `postgres`, `oracle`, `sqlserver` |
| Disconnect | `SQL DISCONNECT <alias>;` | Closes connection |
| List connections | `SQL CONNECTIONS;` | Shows alias + driver only (no DSN) |
| Show tables | `SQL <alias> SHOW TABLES;` | Lists user tables |
| Show views | `SQL <alias> SHOW VIEWS;` | Lists user views |
| Show functions | `SQL <alias> SHOW FUNCTIONS;` | Lists functions and procedures |
| Describe table | `SQL <alias> DESCRIBE <table>;` | Shows columns, types, nullability |
| Query | `SQL <alias> <any-sql>;` | Raw SQL passthrough |
| Import | `IMPORT FROM <alias> QUERY '<sql>' INTO Module.Entity MAP (...) [LINK (...)] [BATCH n] [LIMIT n];` | Insert external data into Mendix app DB |
| Generate connector | `SQL <alias> GENERATE CONNECTOR INTO <module> [TABLES (...)] [VIEWS (...)] [EXEC];` | Generate Database Connector MDL from schema |

```sql
-- Connect to PostgreSQL
SQL CONNECT postgres 'postgres://user:pass@localhost:5432/mydb' AS source;

-- Explore schema
SQL source SHOW TABLES;
SQL source DESCRIBE users;

-- Query data
SQL source SELECT * FROM users WHERE active = true LIMIT 10;

-- Import external data into Mendix app database
IMPORT FROM source QUERY 'SELECT name, email FROM employees'
  INTO HRModule.Employee
  MAP (name AS Name, email AS Email);

-- Import with association linking
IMPORT FROM source QUERY 'SELECT name, dept_name FROM employees'
  INTO HR.Employee
  MAP (name AS Name)
  LINK (dept_name TO Employee_Department ON Name);

-- Generate Database Connector from schema
SQL source GENERATE CONNECTOR INTO HRModule;
SQL source GENERATE CONNECTOR INTO HRModule TABLES (employees, departments) EXEC;

-- Manage connections
SQL CONNECTIONS;
SQL DISCONNECT source;
```

CLI subcommand: `mxcli sql --driver postgres --dsn '...' "SELECT 1"` (see `mxcli syntax sql`). Supported drivers: `postgres` (pg, postgresql), `oracle` (ora), `sqlserver` (mssql).

## Catalog & Search

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Refresh catalog | `REFRESH CATALOG;` | Rebuild basic metadata tables |
| Refresh with refs | `REFRESH CATALOG FULL;` | Include cross-references and source |
| List catalog tables | `LIST CATALOG TABLES;` | List available queryable tables |
| Query catalog | `SELECT ... FROM CATALOG.<table> [WHERE ...];` | SQL against project metadata |
| List callers | `LIST CALLERS OF Module.Name;` | What calls this element |
| List callees | `LIST CALLEES OF Module.Name;` | What this element calls |
| List references | `LIST REFERENCES OF Module.Name;` | All references to/from |
| List impact | `LIST IMPACT OF Module.Name;` | Impact analysis |
| Describe context | `DESCRIBE CONTEXT OF Module.Name;` | Surrounding context |
| Full-text search | `SEARCH '<keyword>';` | Search across all strings and source |

Cross-reference commands require `REFRESH CATALOG FULL` to populate reference data.

## Connection & Session

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Connect | `CONNECT LOCAL '/path/to/app.mpr';` | Open a Mendix project |
| Disconnect | `DISCONNECT;` | Close current project |
| Status | `STATUS;` | Show connection info |
| Refresh | `REFRESH;` | Reload project from disk |
| Commit | `COMMIT [MESSAGE 'text'];` | Save changes to MPR |
| Set variable | `SET key = value;` | Session variable (e.g., `output_format = 'json'`) |
| Exit | `EXIT;` | Close REPL session |

## CLI Commands

| Command | Syntax | Notes |
|---------|--------|-------|
| Interactive REPL | `mxcli` | Interactive MDL shell |
| Inline commands | `mxcli -p app.mpr -c "CMD1; CMD2; CMD3"` | Semicolons batch multiple commands |
| Inline, keep going | `mxcli -p app.mpr -c "CMD1; CMD2" --continue-on-error` | Without it a batch stops at the first failing statement and says how many were not run; an empty `-c` is an error |
| Execute script | `mxcli exec script.mdl -p app.mpr` | Script file |
| Stdin piping | `echo "CMD" \| mxcli -p app.mpr` | Quiet mode, pipe-friendly |
| Check syntax | `mxcli check script.mdl` | Parse-only validation |
| Check references | `mxcli check script.mdl -p app.mpr --references` | With reference validation |
| Lint project | `mxcli lint -p app.mpr [--format json\|sarif]` | 19 built-in + 31 Starlark rules |
| Report | `mxcli report -p app.mpr [--format markdown\|json\|html] [--modules A,B]` | Best practices report (`--modules` scores only those modules) |
| Test | `mxcli test tests/ -p app.mpr` | `.test.mdl` / `.test.md` files |
| Diff script | `mxcli diff -p app.mpr changes.mdl` | Compare script vs project |
| Diff local | `mxcli diff-local -p app.mpr --ref HEAD` | Git diff for MPR v2 |
| Diff revisions | `mxcli diff-local -p app.mpr --ref main..feature` | Compare two git revisions |
| OQL | `mxcli oql -p app.mpr "SELECT ..."` | Query running Mendix runtime |
| External SQL | `mxcli sql --driver postgres --dsn '...' "SELECT 1"` | Direct database query |
| Docker build | `mxcli docker build -p app.mpr` | Build with PAD patching |
| Docker check | `mxcli docker check -p app.mpr` | Validate with `mx check` |
| Version | `mxcli version` (or `--version`) | Build version and build time |
| Diagnostics | `mxcli diag [--bundle]` | Session logs, version info |
| Loop report | `mxcli diag loop-report [--json]` | Which mxcli commands a session actually ran, how often, how long |
| Session report | `mxcli diag session-report <transcript.jsonl>... [--top N] [--json]` | Where an agent session's tool calls and tokens went, from its Claude Code transcript |
| Eval run | `mxcli eval run <test.md> --version X.Y.Z` | Run Claude Code headless on an eval test, check the project, report the session |
| New project | `mxcli new <name> --version X.Y.Z` | Create project from scratch with all tooling |
| Init project | `mxcli init /path/to/project` | Add AI tooling to existing project |
| Setup mxcli | `mxcli setup mxcli [--os linux]` | Download platform-specific mxcli binary |
| LSP server | `mxcli lsp --stdio` | Language server for VS Code |
