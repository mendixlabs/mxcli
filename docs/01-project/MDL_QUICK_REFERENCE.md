# MDL Syntax Quick Reference

Complete syntax reference for MDL (Mendix Definition Language). This is the authoritative reference for all MDL statement syntax.

For task-specific guidance, see the skill files listed in [CLAUDE.md](../CLAUDE.md#important-before-writing-mdl-scripts-or-working-with-data).

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
| Create view entity | `CREATE VIEW ENTITY Module.Name (attrs) AS SELECT ...;` | OQL-backed read-only |
| Create external entity | `CREATE EXTERNAL ENTITY Module.Name FROM ODATA CLIENT Module.Client (...) (attrs);` | From consumed OData |
| Drop entity | `DROP ENTITY Module.Name;` | |
| Describe entity | `DESCRIBE ENTITY Module.Name;` | Full MDL output |
| Show entities | `SHOW ENTITIES [IN Module];` | List all or filter by module |
| Create enumeration | `CREATE [OR MODIFY] ENUMERATION Module.Name (Value1 'Caption', ...);` | |
| Drop enumeration | `DROP ENUMERATION Module.Name;` | |
| Create association | `CREATE ASSOCIATION Module.Name FROM Parent TO Child TYPE Reference\|ReferenceSet [OWNER Default\|Both] [DELETE_BEHAVIOR ...];` | |
| Drop association | `DROP ASSOCIATION Module.Name;` | |

## ALTER ENTITY

Modifies an existing entity without full replacement.

| Operation | Syntax | Notes |
|-----------|--------|-------|
| Add attributes | `ALTER ENTITY Module.Name ADD (Attr: Type [constraints]);` | One or more attributes |
| Drop attributes | `ALTER ENTITY Module.Name DROP (AttrName, ...);` | |
| Modify attributes | `ALTER ENTITY Module.Name MODIFY (Attr: NewType [constraints]);` | Change type/constraints |
| Rename attribute | `ALTER ENTITY Module.Name RENAME OldName TO NewName;` | |
| Add index | `ALTER ENTITY Module.Name ADD INDEX (Col1 [ASC\|DESC], ...);` | |
| Drop index | `ALTER ENTITY Module.Name DROP INDEX (Col1, ...);` | |
| Set documentation | `ALTER ENTITY Module.Name SET DOCUMENTATION 'text';` | |
| Set position | `ALTER ENTITY Module.Name SET POSITION (100, 200);` | Canvas position |

**Example:**
```sql
ALTER ENTITY Sales.Customer
  ADD (Phone: String(50), Notes: String(unlimited));

ALTER ENTITY Sales.Customer
  RENAME Phone TO PhoneNumber;

ALTER ENTITY Sales.Customer
  ADD INDEX (Email);
```

## Constants

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show constants | `SHOW CONSTANTS [IN Module];` | List all or filter by module |
| Show constant values | `SHOW CONSTANT VALUES [IN Module];` | Compare values across configurations |
| Describe constant | `DESCRIBE CONSTANT Module.Name;` | Full MDL output |
| Create constant | `CREATE [OR MODIFY] CONSTANT Module.Name TYPE DataType DEFAULT 'value';` | String, Integer, Boolean, etc. |
| Drop constant | `DROP CONSTANT Module.Name;` | |

**Example:**
```sql
CREATE CONSTANT MyModule.ApiBaseUrl TYPE String DEFAULT 'https://api.example.com';
CREATE CONSTANT MyModule.MaxRetries TYPE Integer DEFAULT 3;
CREATE CONSTANT MyModule.EnableLogging TYPE Boolean DEFAULT true;
```

## OData Clients, Services & External Entities

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show OData clients | `SHOW ODATA CLIENTS [IN Module];` | Consumed OData services |
| Describe OData client | `DESCRIBE ODATA CLIENT Module.Name;` | Full MDL output |
| Create OData client | `CREATE [OR MODIFY] ODATA CLIENT Module.Name (...);` | Version, MetadataUrl, Timeout, etc. |
| Alter OData client | `ALTER ODATA CLIENT Module.Name SET Key = Value;` | |
| Drop OData client | `DROP ODATA CLIENT Module.Name;` | |
| Show OData services | `SHOW ODATA SERVICES [IN Module];` | Published OData services |
| Describe OData service | `DESCRIBE ODATA SERVICE Module.Name;` | Full MDL output |
| Create OData service | `CREATE [OR MODIFY] ODATA SERVICE Module.Name (...) AUTHENTICATION ... { PUBLISH ENTITY ... };` | |
| Alter OData service | `ALTER ODATA SERVICE Module.Name SET Key = Value;` | |
| Drop OData service | `DROP ODATA SERVICE Module.Name;` | |
| Show external entities | `SHOW EXTERNAL ENTITIES [IN Module];` | OData-backed entities |
| Show external actions | `SHOW EXTERNAL ACTIONS [IN Module];` | Actions used in microflows |
| Create external entity | `CREATE [OR MODIFY] EXTERNAL ENTITY Module.Name FROM ODATA CLIENT Module.Client (...) (attrs);` | |
| Grant OData access | `GRANT ACCESS ON ODATA SERVICE Module.Name TO Module.Role, ...;` | |
| Revoke OData access | `REVOKE ACCESS ON ODATA SERVICE Module.Name FROM Module.Role, ...;` | |
| Show contract entities | `SHOW CONTRACT ENTITIES FROM Module.Client;` | Browse cached $metadata |
| Show contract actions | `SHOW CONTRACT ACTIONS FROM Module.Client;` | Browse cached $metadata |
| Describe contract entity | `DESCRIBE CONTRACT ENTITY Module.Client.Entity [FORMAT mdl];` | Properties, types, keys |
| Describe contract action | `DESCRIBE CONTRACT ACTION Module.Client.Action [FORMAT mdl];` | Parameters, return type |
| Show contract channels | `SHOW CONTRACT CHANNELS FROM Module.Service;` | Browse cached AsyncAPI |
| Show contract messages | `SHOW CONTRACT MESSAGES FROM Module.Service;` | Browse cached AsyncAPI |
| Describe contract message | `DESCRIBE CONTRACT MESSAGE Module.Service.Message;` | Message payload properties |
| Query contract entities | `SELECT * FROM CATALOG.CONTRACT_ENTITIES;` | Requires REFRESH CATALOG |
| Query contract actions | `SELECT * FROM CATALOG.CONTRACT_ACTIONS;` | Requires REFRESH CATALOG |
| Query contract messages | `SELECT * FROM CATALOG.CONTRACT_MESSAGES;` | Requires REFRESH CATALOG |

**OData Client Example:**
```sql
CREATE ODATA CLIENT MyModule.ExternalAPI (
  Version: '1.0',
  ODataVersion: OData4,
  MetadataUrl: 'https://api.example.com/odata/v4/$metadata',
  Timeout: 300
);
```

**OData Service Example:**
```sql
CREATE ODATA SERVICE MyModule.CustomerAPI (
  Path: '/odata/customers',
  Version: '1.0.0',
  ODataVersion: OData4,
  Namespace: 'MyModule.Customers'
)
AUTHENTICATION Basic, Session
{
  PUBLISH ENTITY MyModule.Customer AS 'Customers' (
    ReadMode: SOURCE,
    InsertMode: SOURCE,
    UpdateMode: NOT_SUPPORTED,
    DeleteMode: NOT_SUPPORTED,
    UsePaging: Yes,
    PageSize: 100
  )
  EXPOSE (Name, Email, Phone);
};
```

## Microflows - Supported Statements

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Variable declaration | `DECLARE $Var Type = value;` | Primitives: String, Integer, Boolean, Decimal, DateTime |
| Entity declaration | `DECLARE $Entity Module.Entity;` | No AS keyword, no = empty |
| List declaration | `DECLARE $List List of Module.Entity = empty;` | |
| Assignment | `SET $Var = expression;` | Variable must be declared first |
| Create object | `$Var = CREATE Module.Entity (Attr = value);` | |
| Change object | `CHANGE $Entity (Attr = value);` | |
| Commit | `COMMIT $Entity [WITH EVENTS] [REFRESH];` | |
| Delete | `DELETE $Entity;` | |
| Rollback | `ROLLBACK $Entity [REFRESH];` | Reverts uncommitted changes |
| Retrieve (DB) | `RETRIEVE $Var FROM Module.Entity [WHERE condition];` | Database XPath retrieve |
| Retrieve (Assoc) | `RETRIEVE $List FROM $Parent/Module.AssocName;` | Retrieve by association |
| Call microflow | `$Result = CALL MICROFLOW Module.Name (Param = $value);` | |
| Call nanoflow | `$Result = CALL NANOFLOW Module.Name (Param = $value);` | |
| Show page | `SHOW PAGE Module.PageName ($Param = $value);` | Also accepts `(Param: $value)` |
| Close page | `CLOSE PAGE;` | |
| Validation | `VALIDATION FEEDBACK $Entity/Attribute MESSAGE 'message';` | Requires attribute path + MESSAGE |
| Log | `LOG INFO\|WARNING\|ERROR [NODE 'name'] 'message';` | |
| Position | `@position(x, y)` | Canvas position (before activity) |
| Caption | `@caption 'text'` | Custom caption (before activity) |
| Color | `@color Green` | Background color (before activity) |
| Annotation | `@annotation 'text'` | Visual note attached to next activity |
| IF | `IF condition THEN ... [ELSE ...] END IF;` | |
| LOOP | `LOOP $Item IN $List BEGIN ... END LOOP;` | FOR EACH over list |
| WHILE | `WHILE condition BEGIN ... END WHILE;` | Condition-based loop |
| Return | `RETURN $value;` | Required at end of every flow path |
| Execute DB query | `$Result = EXECUTE DATABASE QUERY Module.Conn.Query;` | 3-part name; supports DYNAMIC, params, CONNECTION override |
| Error handling | `... ON ERROR CONTINUE\|ROLLBACK\|{ handler };` | Not supported on EXECUTE DATABASE QUERY |

## Microflows - NOT Supported (Will Cause Parse Errors)

| Unsupported | Use Instead | Notes |
|-------------|-------------|-------|
| `CASE ... WHEN ... END CASE` | Nested `IF ... ELSE ... END IF` | Switch not implemented |
| `TRY ... CATCH ... END TRY` | `ON ERROR { ... }` blocks | Use error handlers on specific activities |

**Notes:**
- `RETRIEVE ... LIMIT n` IS supported. `LIMIT 1` returns a single entity, otherwise returns a list.
- `ROLLBACK $Entity [REFRESH];` IS supported. Rolls back uncommitted changes to an object.

## Project Organization

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Microflow folder | `FOLDER 'path'` (before BEGIN) | `CREATE MICROFLOW ... FOLDER 'ACT' BEGIN ... END;` |
| Page folder | `Folder: 'path'` (in properties) | `CREATE PAGE ... (Folder: 'Pages/Detail') { ... }` |
| Drop folder | `DROP FOLDER 'path' IN Module;` | Folder must be empty |
| Move folder | `MOVE FOLDER Module.FolderName TO FOLDER 'path';` | Target folders auto-created |
| Move to folder | `MOVE PAGE\|MICROFLOW\|SNIPPET\|NANOFLOW\|ENUMERATION Module.Name TO FOLDER 'path';` | Folders created automatically |
| Move to module root | `MOVE PAGE Module.Name TO Module;` | Removes from folder |
| Move across modules | `MOVE PAGE Old.Name TO NewModule;` | **Breaks by-name references** — use `SHOW IMPACT OF` first |
| Move to folder in other module | `MOVE PAGE Old.Name TO FOLDER 'path' IN NewModule;` | |
| Move entity to module | `MOVE ENTITY Old.Name TO NewModule;` | Entities don't support folders |

Nested folders use `/` separator: `'Parent/Child/Grandchild'`. Missing folders are auto-created.

## Security Management

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show project security | `SHOW PROJECT SECURITY;` | Displays security level, admin, demo users |
| Show module roles | `SHOW MODULE ROLES [IN Module];` | All roles or filtered by module |
| Show user roles | `SHOW USER ROLES;` | Project-level user roles |
| Show demo users | `SHOW DEMO USERS;` | Configured demo users |
| Show access on element | `SHOW ACCESS ON MICROFLOW\|PAGE\|Entity Mod.Name;` | Which roles can access |
| Show security matrix | `SHOW SECURITY MATRIX [IN Module];` | Full access overview |
| Create module role | `CREATE MODULE ROLE Mod.Role [DESCRIPTION 'text'];` | |
| Drop module role | `DROP MODULE ROLE Mod.Role;` | |
| Create user role | `CREATE USER ROLE Name (Mod.Role, ...) [MANAGE ALL ROLES];` | Aggregates module roles |
| Alter user role | `ALTER USER ROLE Name ADD\|REMOVE MODULE ROLES (Mod.Role, ...);` | |
| Drop user role | `DROP USER ROLE Name;` | |
| Grant microflow access | `GRANT EXECUTE ON MICROFLOW Mod.MF TO Mod.Role, ...;` | |
| Revoke microflow access | `REVOKE EXECUTE ON MICROFLOW Mod.MF FROM Mod.Role, ...;` | |
| Grant page access | `GRANT VIEW ON PAGE Mod.Page TO Mod.Role, ...;` | |
| Revoke page access | `REVOKE VIEW ON PAGE Mod.Page FROM Mod.Role, ...;` | |
| Grant entity access | `GRANT Mod.Role ON Mod.Entity (CREATE, DELETE, READ *, WRITE *);` | Supports member lists and WHERE |
| Revoke entity access | `REVOKE Mod.Role ON Mod.Entity;` | |
| Set security level | `ALTER PROJECT SECURITY LEVEL OFF\|PROTOTYPE\|PRODUCTION;` | |
| Toggle demo users | `ALTER PROJECT SECURITY DEMO USERS ON\|OFF;` | |
| Create demo user | `CREATE DEMO USER 'name' PASSWORD 'pass' [ENTITY Module.Entity] (UserRole, ...);` | |
| Drop demo user | `DROP DEMO USER 'name';` | |

## Workflows

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show workflows | `SHOW WORKFLOWS [IN Module];` | List all or filter by module |
| Describe workflow | `DESCRIBE WORKFLOW Module.Name;` | Full MDL output |
| Create workflow | `CREATE [OR MODIFY] WORKFLOW Module.Name PARAMETER $Ctx: Module.Entity BEGIN ... END WORKFLOW;` | See activity types below |
| Drop workflow | `DROP WORKFLOW Module.Name;` | |
| Grant workflow access | `GRANT EXECUTE ON WORKFLOW Module.Name TO Mod.Role, ...;` | |
| Revoke workflow access | `REVOKE EXECUTE ON WORKFLOW Module.Name FROM Mod.Role, ...;` | |

**Workflow Activity Types:**
- `USER TASK <name> '<caption>' [PAGE Mod.Page] [TARGETING MICROFLOW Mod.MF] [OUTCOMES '<out>' { } ...];`
- `CALL MICROFLOW Mod.MF [COMMENT '<text>'] [OUTCOMES '<out>' { } ...];`
- `CALL WORKFLOW Mod.WF [COMMENT '<text>'];`
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
| Structure overview | `SHOW STRUCTURE;` | Depth 2 (elements with signatures), user modules only |
| Module counts | `SHOW STRUCTURE DEPTH 1;` | One line per module with element counts |
| Full types | `SHOW STRUCTURE DEPTH 3;` | Typed attributes, named parameters |
| Filter by module | `SHOW STRUCTURE IN ModuleName;` | Single module only |
| Include all modules | `SHOW STRUCTURE DEPTH 1 ALL;` | Include system/marketplace modules |

## Navigation

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show navigation | `SHOW NAVIGATION;` | Summary of all profiles |
| Show menu tree | `SHOW NAVIGATION MENU [Profile];` | Menu tree for profile or all |
| Show home pages | `SHOW NAVIGATION HOMES;` | Home page assignments across profiles |
| Describe navigation | `DESCRIBE NAVIGATION [Profile];` | Full MDL output (round-trippable) |
| Create/replace navigation | `CREATE OR REPLACE NAVIGATION Profile ...;` | Full replacement of profile |

**Navigation Example:**
```sql
CREATE OR REPLACE NAVIGATION Responsive
  HOME PAGE MyModule.Home_Web
  HOME PAGE MyModule.AdminHome FOR MyModule.Administrator
  LOGIN PAGE Administration.Login
  NOT FOUND PAGE MyModule.Custom404
  MENU (
    MENU ITEM 'Home' PAGE MyModule.Home_Web;
    MENU 'Admin' (
      MENU ITEM 'Users' PAGE Administration.Account_Overview;
    );
  );
```

## Project Settings

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show settings | `SHOW SETTINGS;` | Overview of all settings parts |
| Describe settings | `DESCRIBE SETTINGS;` | Full MDL output (round-trippable) |
| Alter model settings | `ALTER SETTINGS MODEL Key = Value;` | AfterStartupMicroflow, HashAlgorithm, JavaVersion, etc. |
| Alter configuration | `ALTER SETTINGS CONFIGURATION 'Name' Key = Value;` | DatabaseType, DatabaseUrl, HttpPortNumber, etc. |
| Alter constant | `ALTER SETTINGS CONSTANT 'Name' VALUE 'val' IN CONFIGURATION 'cfg';` | Override constant per configuration |
| Drop constant override | `ALTER SETTINGS DROP CONSTANT 'Name' IN CONFIGURATION 'cfg';` | Reset to default value |
| Create configuration | `CREATE CONFIGURATION 'Name' [Key = Value, ...];` | New server configuration |
| Drop configuration | `DROP CONFIGURATION 'Name';` | Remove a configuration |
| Alter language | `ALTER SETTINGS LANGUAGE Key = Value;` | DefaultLanguageCode |
| Alter workflows | `ALTER SETTINGS WORKFLOWS Key = Value;` | UserEntity, DefaultTaskParallelism |

## Business Events

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show services | `SHOW BUSINESS EVENTS;` | List all business event services |
| Show in module | `SHOW BUSINESS EVENTS IN Module;` | Filter by module |
| Describe service | `DESCRIBE BUSINESS EVENT SERVICE Module.Name;` | Full MDL output |
| Create service | `CREATE BUSINESS EVENT SERVICE Module.Name (...) { MESSAGE ... };` | See help topic for full syntax |
| Drop service | `DROP BUSINESS EVENT SERVICE Module.Name;` | Delete a service |

## Image Collections

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show collections | `SHOW IMAGE COLLECTION [IN Module];` | List all or filter by module |
| Describe collection | `DESCRIBE IMAGE COLLECTION Module.Name;` | Full MDL output with embedded images |
| Create collection | `CREATE IMAGE COLLECTION Module.Name [EXPORT LEVEL 'Hidden'\|'Public'] [COMMENT 'text'] [(IMAGE Name FROM FILE 'path', ...)];` | With or without images |
| Drop collection | `DROP IMAGE COLLECTION Module.Name;` | Removes collection and all embedded images |

**Export levels:** `'Hidden'` (default, internal to module), `'Public'` (accessible from other modules).

## JSON Structures

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show structures | `SHOW JSON STRUCTURES [IN Module];` | List all or filter by module |
| Describe structure | `DESCRIBE JSON STRUCTURE Module.Name;` | Re-executable CREATE OR REPLACE + element tree |
| Create structure | `CREATE JSON STRUCTURE Module.Name [COMMENT 'text'] SNIPPET '...json...';` | Element tree auto-built from snippet |
| Create (multi-line) | `CREATE JSON STRUCTURE Module.Name SNIPPET $${ "key": "value" }$$;` | Dollar-quoted snippet for readability |
| Create or replace | `CREATE OR REPLACE JSON STRUCTURE Module.Name SNIPPET '...';` | Idempotent — preferred for AI agents |
| Create with name map | `CREATE JSON STRUCTURE Module.Name SNIPPET '...' CUSTOM NAME MAP ('jsonKey' AS 'CustomName', ...);` | Override auto-generated ExposedNames |
| Drop structure | `DROP JSON STRUCTURE Module.Name;` | |

## Java Actions

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show Java actions | `SHOW JAVA ACTIONS [IN Module];` | List all or filtered by module |
| Describe Java action | `DESCRIBE JAVA ACTION Module.Name;` | Full MDL output with signature |
| Create Java action | `CREATE JAVA ACTION Module.Name(params) RETURNS type AS $$ ... $$;` | Inline Java code |
| Create with type params | `CREATE JAVA ACTION Module.Name(EntityType: ENTITY <pEntity>, Obj: pEntity) ...;` | Generic type parameters |
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
| Page variables | `Variables: { $name: Type = 'expr' }` | `Variables: { $show: Boolean = 'true' }` |
| Widget name | Required after type | `TEXTBOX txtName (...)` |
| Attribute binding | `Attribute: AttrName` | `TEXTBOX txt (Label: 'Name', Attribute: Name)` |
| Variable binding | `DataSource: $Var` | `DATAVIEW dv (DataSource: $Product) { ... }` |
| Action binding | `Action: TYPE` | `ACTIONBUTTON btn (Caption: 'Save', Action: SAVE_CHANGES)` |
| Microflow action | `Action: MICROFLOW Name(Param: val)` | `Action: MICROFLOW Mod.ACT_Process(Order: $Order)` |
| Database source | `DataSource: DATABASE Entity` | `DATAGRID dg (DataSource: DATABASE Module.Entity)` |
| Selection binding | `DataSource: SELECTION widget` | `DATAVIEW dv (DataSource: SELECTION galleryList)` |
| CSS class | `Class: 'classes'` | `CONTAINER c (Class: 'card mx-spacing-top-large')` |
| Inline style | `Style: 'css'` | `CONTAINER c (Style: 'padding: 16px;')` |
| Design properties | `DesignProperties: [...]` | `CONTAINER c (DesignProperties: ['Spacing top': 'Large', 'Full width': ON])` |
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
| `Size` | integer (px) | `1` | `Size: 200` |
| `Visible` | expression string | `true` | `Visible: '$showColumn'` (page variable, not $currentObject) |
| `DynamicCellClass` | expression string | (empty) | `DynamicCellClass: 'if(...) then ... else ...'` |
| `Tooltip` | text string | (empty) | `Tooltip: 'Price in USD'` |

**Page Example:**
```sql
CREATE PAGE MyModule.Customer_Edit
(
  Params: { $Customer: MyModule.Customer },
  Title: 'Edit Customer',
  Layout: Atlas_Core.PopupLayout
)
{
  DATAVIEW dvCustomer (DataSource: $Customer) {
    TEXTBOX txtName (Label: 'Name', Attribute: Name)
    TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
    COMBOBOX cbStatus (Label: 'Status', Attribute: Status)

    FOOTER footer1 {
      ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)
      ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
    }
  }
}
```

**Widget Properties:**

| Property | Syntax | Notes |
|----------|--------|-------|
| DesktopWidth | `COLUMN col (DesktopWidth: 8)` | 1-12 or AutoFill |
| TabletWidth | `COLUMN col (TabletWidth: 6)` | 1-12 or AutoFill (default: auto) |
| PhoneWidth | `COLUMN col (PhoneWidth: 12)` | 1-12 or AutoFill (default: auto) |
| Visible | `TEXTBOX txt (Visible: [IsActive])` | Conditional visibility (XPath expression) |
| Editable | `TEXTBOX txt (Editable: [Status != 'Closed'])` | Conditional editability (XPath expression) |

**Supported Widgets:**
- Layout: `LAYOUTGRID`, `ROW`, `COLUMN`, `CONTAINER`, `CUSTOMCONTAINER`
- Input: `TEXTBOX`, `TEXTAREA`, `CHECKBOX`, `RADIOBUTTONS`, `DATEPICKER`, `COMBOBOX`
- Display: `DYNAMICTEXT`, `DATAGRID`, `GALLERY`, `LISTVIEW`, `IMAGE`, `STATICIMAGE`, `DYNAMICIMAGE`
- Actions: `ACTIONBUTTON`, `LINKBUTTON`, `NAVIGATIONLIST`
- Structure: `DATAVIEW`, `HEADER`, `FOOTER`, `CONTROLBAR`, `SNIPPETCALL`

## ALTER PAGE / ALTER SNIPPET

Modify an existing page or snippet's widget tree in-place without full `CREATE OR REPLACE`. Works directly on the raw BSON tree, preserving unsupported widget types.

| Operation | Syntax | Notes |
|-----------|--------|-------|
| Set property | `SET Caption = 'New' ON widgetName` | Single property on a widget |
| Set multiple | `SET (Caption = 'Save', ButtonStyle = Success) ON btn` | Multiple properties at once |
| Page-level set | `SET Title = 'New Title'` | No ON clause for page properties |
| Insert after | `INSERT AFTER widgetName { widgets }` | Add widgets after target |
| Insert before | `INSERT BEFORE widgetName { widgets }` | Add widgets before target |
| Drop widgets | `DROP WIDGET name1, name2` | Remove widgets by name |
| Replace widget | `REPLACE widgetName WITH { widgets }` | Replace widget subtree |
| Pluggable prop | `SET 'showLabel' = false ON cbStatus` | Quoted name for pluggable widgets |
| Add variable | `ADD Variables $name: Type = 'expr'` | Add a page variable |
| Drop variable | `DROP Variables $name` | Remove a page variable |
| Set layout | `SET Layout = Module.LayoutName` | Change page layout, auto-maps placeholders |
| Set layout + map | `SET Layout = Module.Layout MAP (Old AS New)` | Explicit placeholder mapping |

**Supported SET properties:** Caption, Label, ButtonStyle, Class, Style, Editable, Visible, Name, Title (page-level), Layout (page-level), and quoted pluggable widget properties.

**Example:**
```sql
ALTER PAGE Module.EditPage {
  SET (Caption = 'Save & Close', ButtonStyle = Success) ON btnSave;
  DROP WIDGET txtUnused;
  INSERT AFTER txtEmail {
    TEXTBOX txtPhone (Label: 'Phone', Attribute: Phone)
  }
};

ALTER SNIPPET Module.NavMenu {
  SET Caption = 'Dashboard' ON btnHome
};
```

**Tip:** Run `DESCRIBE PAGE Module.PageName` first to see widget names.

## Reserved Words and Quoted Identifiers

Most MDL keywords now work **unquoted** as entity names, attribute names, parameter names, and module names. Common words like `Caption`, `Check`, `Content`, `Format`, `Index`, `Label`, `Range`, `Select`, `Source`, `Status`, `Text`, `Title`, `Type`, `Value`, `Item`, `Version`, `Production`, etc. are all valid without quoting.

Only structural MDL keywords require quoting: `Create`, `Delete`, `Begin`, `End`, `Return`, `Entity`, `Module`.

**Quoted identifiers** escape any reserved word (double-quotes or backticks):
```sql
DESCRIBE ENTITY "ComboBox"."CategoryTreeVE";
SHOW ENTITIES IN "ComboBox";
CREATE PERSISTENT ENTITY Module.VATRate ("Create": DateTime, Rate: Decimal);
```

Both double-quote (ANSI SQL) and backtick (MySQL) styles are supported. You can mix quoted and unquoted parts: `"ComboBox".CategoryTreeVE`.

**Boolean attributes** auto-default to `false` when no `DEFAULT` is specified.

**CALCULATED** marks an attribute as calculated (not stored). Use `CALCULATED BY Module.Microflow` to specify the calculation microflow. Calculated attributes derive their value from a microflow at runtime.

**ButtonStyle** supports all values: `Primary`, `Default`, `Success`, `Danger`, `Warning`, `Info`.

## External SQL Statements

Direct SQL query execution against external databases (PostgreSQL, Oracle, SQL Server). Credentials are isolated — DSN never appears in session output or logs.

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
| Show catalog tables | `SHOW CATALOG TABLES;` | List available queryable tables |
| Query catalog | `SELECT ... FROM CATALOG.<table> [WHERE ...];` | SQL against project metadata |
| Show callers | `SHOW CALLERS OF Module.Name;` | What calls this element |
| Show callees | `SHOW CALLEES OF Module.Name;` | What this element calls |
| Show references | `SHOW REFERENCES OF Module.Name;` | All references to/from |
| Show impact | `SHOW IMPACT OF Module.Name;` | Impact analysis |
| Show context | `SHOW CONTEXT OF Module.Name;` | Surrounding context |
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
| Execute command | `mxcli -p app.mpr -c "SHOW ENTITIES"` | Single command |
| Execute script | `mxcli exec script.mdl -p app.mpr` | Script file |
| Check syntax | `mxcli check script.mdl` | Parse-only validation |
| Check references | `mxcli check script.mdl -p app.mpr --references` | With reference validation |
| Lint project | `mxcli lint -p app.mpr [--format json\|sarif]` | 14 built-in + 27 Starlark rules |
| Report | `mxcli report -p app.mpr [--format markdown\|json\|html]` | Best practices report |
| Test | `mxcli test tests/ -p app.mpr` | `.test.mdl` / `.test.md` files |
| Diff script | `mxcli diff -p app.mpr changes.mdl` | Compare script vs project |
| Diff local | `mxcli diff-local -p app.mpr --ref HEAD` | Git diff for MPR v2 |
| OQL | `mxcli oql -p app.mpr "SELECT ..."` | Query running Mendix runtime |
| External SQL | `mxcli sql --driver postgres --dsn '...' "SELECT 1"` | Direct database query |
| Docker build | `mxcli docker build -p app.mpr` | Build with PAD patching |
| Docker check | `mxcli docker check -p app.mpr` | Validate with `mx check` |
| Diagnostics | `mxcli diag [--bundle]` | Session logs, version info |
| Init project | `mxcli init -p app.mpr` | Create `.claude/` folder with skills |
| LSP server | `mxcli lsp --stdio` | Language server for VS Code |

## ANTLR4 Parser Architecture

The MDL parser uses ANTLR4 for grammar definition, enabling cross-language grammar sharing (Go, TypeScript, Java, Python).

**Regenerating the parser** (after modifying `MDLLexer.g4` or `MDLParser.g4`):
```bash
# Option 1: Use make from project root (recommended)
make grammar

# Option 2: Run directly in grammar directory
cd mdl/grammar
antlr4 -Dlanguage=Go -package parser -o parser MDLLexer.g4 MDLParser.g4
```

**Parser pipeline:**
1. `MDLLexer.g4` + `MDLParser.g4` → Split ANTLR4 grammar (tokens + rules, case-insensitive keywords)
2. `parser/` → Generated lexer/parser code
3. `visitor/` → ANTLR listener builds AST from parse tree
4. `ast/` → Strongly-typed AST nodes
5. `executor/` → Executes AST against modelsdk-go API

**Key design decisions:**
- ANTLR4 chosen over parser combinators for cross-language grammar sharing
- Case-insensitive keywords using ANTLR fragment rules
- Listener pattern (not visitor) for building AST
- Type assertions required for accessing concrete ANTLR context types
