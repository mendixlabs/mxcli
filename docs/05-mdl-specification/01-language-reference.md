# MDL Language Reference

This document provides a complete reference for MDL (Mendix Definition Language) syntax and semantics.

## Table of Contents

1. [Lexical Structure](#lexical-structure)
2. [Identifiers and Names](#identifiers-and-names)
3. [Connection Statements](#connection-statements)
4. [Query Statements](#query-statements)
5. [Module Statements](#module-statements)
6. [Enumeration Statements](#enumeration-statements)
7. [Entity Statements](#entity-statements)
8. [ALTER ENTITY Statements](#alter-entity-statements)
9. [Association Statements](#association-statements)
10. [Microflow Statements](#microflow-statements)
11. [Page Statements](#page-statements)
12. [ALTER PAGE / ALTER SNIPPET](#alter-page--alter-snippet)
13. [MOVE Statements](#move-statements)
14. [Security Statements](#security-statements)
15. [Navigation Statements](#navigation-statements)
16. [Settings Statements](#settings-statements)
17. [External SQL Statements](#external-sql-statements)
18. [Import Statements](#import-statements)
19. [Catalog and Search Statements](#catalog-and-search-statements)
20. [Business Event Statements](#business-event-statements)
21. [Java Action Statements](#java-action-statements)
22. [Session Statements](#session-statements)

---

## Lexical Structure

### Keywords

MDL keywords are **case-insensitive**. The following are reserved keywords:

```
access, actions, add, after, all, alter, and, annotation, as, asc,
ASCENDING, association, autonumber, batch, before, begin, binary,
boolean, both, business, by, call, cancel, caption, cascade,
catalog, change, CHILD, close, column, combobox, commit, connect,
configuration, connector, constant, constraint, container, create,
CRUD, datagrid, dataview, date, datetime, declare, default, delete,
delete_behavior, DELETE_BUT_KEEP_REFERENCES, DELETE_CASCADE, demo,
depth, desc, DESCENDING, describe, DIFF, disconnect, drop, else,
empty, end, entity, enumeration, error, event, events, execute,
exec, EXIT, export, extends, external, false, folder, footer,
for, format, from, full, gallery, generate, grant, header, HELP,
home, if, import, in, index, info, insert, integer, into, java,
KEEP_REFERENCES, label, LANGUAGE, layout, layoutgrid, level, limit,
link, list, listview, local, log, login, long, loop, manage, map,
matrix, menu, message, microflow, microflows, model, modify, module,
modules, move, nanoflow, nanoflows, navigation, node, NON_PERSISTENT,
not, null, of, on, or, ORACLE, overview, owner, page, pages, PARENT,
password, persistent, position, POSTGRES, production, project,
prototype, query, QUIT, reference, REFERENCESET, refresh, remove,
replace, REPORT, RESPONSIVE, retrieve, return, revoke, role, roles,
rollback, row, SAVE, script, search, security, selection, set, show,
snippet, snippets, sql, SQLSERVER, status, string, structure,
tables, textbox, textarea, then, to, true, type, unique, update,
user, validation, value, view, views, visible, warning, where, widget,
widgets, with, workflows, write
```

Most MDL keywords work **unquoted** as identifiers (entity names, attribute names, etc.).
Only structural keywords like `create`, `delete`, `begin`, `end`, `return`, `entity`, `module`
require quoting when used as identifiers. See [MDL Quick Reference](../MDL_QUICK_REFERENCE.md#reserved-words-and-quoted-identifiers) for details.

### Literals

#### String Literals
```sql
'single quoted string'
```

Escape sequences:
- `''` - Single quote (doubled)

Note: Mendix expression strings use doubled single quotes for escaping (`'it''s here'`), not backslash escaping.

#### Quoted Identifiers

Use double-quotes (ANSI SQL) or backticks (MySQL) to escape reserved words in identifiers:
```sql
"combobox"."CategoryTreeVE"
`Order`.`status`
"combobox".CategoryTreeVE    -- mixed is fine
```

#### Numeric Literals
```sql
42          -- Integer
3.14        -- Decimal
-100        -- Negative integer
1.5e10      -- Scientific notation
```

#### Boolean Literals
```sql
true
false
```

### Comments

```sql
-- Single line comment
// Also single line comment

/* Multi-line
   comment */

/** Documentation comment
 *  Used for entity/attribute documentation
 */
```

### Statement Terminators

Statements can be terminated with:
- `;` - Standard SQL terminator
- `/` - Oracle-style terminator (useful for multi-line statements)

Simple commands (HELP, EXIT, STATUS, SHOW, DESCRIBE) don't require terminators.

---

## Identifiers and Names

### Simple Identifiers

Valid identifier characters:
- Letters: `A-Z`, `a-z`
- Digits: `0-9` (not as first character)
- Underscore: `_`

```sql
MyEntity
my_attribute
Attribute123
```

### Qualified Names

Format: `Module.Element` or `Module.Entity.Attribute`

```sql
MyModule.Customer           -- Entity in module
MyModule.OrderStatus        -- Enumeration in module
MyModule.Customer.Name      -- Attribute in entity
```

---

## Connection Statements

### CONNECT

Establishes a connection to a Mendix project file.

**Syntax:**
```sql
connect local '<path>'
```

**Parameters:**
- `<path>` - Path to `.mpr` file (absolute or relative)

**Examples:**
```sql
connect local '/Users/dev/projects/MyApp/MyApp.mpr';
connect local './mx-test-projects/test1-go-app/test1-go.mpr';
```

### DISCONNECT

Closes the current project connection.

**Syntax:**
```sql
disconnect
```

### STATUS

Shows current connection status and project information.

**Syntax:**
```sql
status
```

**Output:**
```
status: Connected
project: /path/to/project.mpr
modules: 5
```

---

## Query Statements

### SHOW

Lists elements of a specific type.

**Syntax:**
```sql
show modules
show entities [in <module>]
show entity <qualified-name>
show enumerations [in <module>]
show associations [in <module>]
show association <qualified-name>
show microflows [in <module>]
show nanoflows [in <module>]
show pages [in <module>]
show snippets [in <module>]
show java actions [in <module>]
show widgets
show structure [depth 1|2|3] [in <module>] [all]
show business events [in <module>]
```

**Examples:**
```sql
show modules
show entities in MyFirstModule
show entity MyModule.Customer
show microflows in Administration
show structure depth 1          -- Module counts
show structure in MyModule      -- Single module at depth 2
show structure depth 3 all      -- Full detail, all modules
```

### DESCRIBE

Shows detailed definition of an element in MDL syntax.

**Syntax:**
```sql
describe entity <qualified-name>
describe enumeration <qualified-name>
describe association <qualified-name>
describe microflow <qualified-name>
describe nanoflow <qualified-name>
describe page <qualified-name>
describe snippet <qualified-name>
describe java action <qualified-name>
describe business event service <qualified-name>
describe navigation [<profile>]
describe settings
```

**Examples:**
```sql
describe entity MyModule.Customer
describe enumeration MyModule.OrderStatus
describe microflow MyModule.CreateOrder
describe page MyModule.Customer_Edit
describe navigation Responsive
```

**Output Example:**
```sql
/**
 * Customer entity stores customer information.
 */
@position(100, 200)
create persistent entity MyModule.Customer (
  /** Customer identifier */
  CustomerId: autonumber not null unique default 1,
  Name: string(200) not null,
  Email: string(200),
  status: enumeration(MyModule.CustomerStatus) default 'Active'
)
index (Name);
/
```

---

## Module Statements

### CREATE MODULE

Creates a new module in the project.

**Syntax:**
```sql
create module <name>
```

**Example:**
```sql
create module OrderManagement;
```

### DROP MODULE

Removes a module from the project.

**Syntax:**
```sql
drop module <name>
```

**Example:**
```sql
drop module OrderManagement;
```

---

## Enumeration Statements

### CREATE ENUMERATION

Creates a new enumeration type.

**Syntax:**
```sql
[/** <documentation> */]
create enumeration <qualified-name> (
  <value-name> '<caption>' [, ...]
)
```

**Parameters:**
- `<qualified-name>` - Module.EnumerationName
- `<value-name>` - Identifier for the enum value
- `<caption>` - Display caption for the value

**Example:**
```sql
/** Order status enumeration */
create enumeration OrderModule.OrderStatus (
  Draft 'Draft',
  Pending 'Pending Approval',
  Approved 'Approved',
  Shipped 'Shipped',
  Delivered 'Delivered',
  Cancelled 'Cancelled'
);
```

### ALTER ENUMERATION

Modifies an existing enumeration.

**Syntax:**
```sql
alter enumeration <qualified-name>
  add value <value-name> '<caption>'

alter enumeration <qualified-name>
  remove value <value-name>
```

**Examples:**
```sql
alter enumeration OrderModule.OrderStatus
  add value OnHold 'On Hold';

alter enumeration OrderModule.OrderStatus
  remove value Draft;
```

### DROP ENUMERATION

Removes an enumeration.

**Syntax:**
```sql
drop enumeration <qualified-name>
```

---

## Entity Statements

### CREATE ENTITY

Creates a new entity in the domain model.

**Syntax:**
```sql
[/** <documentation> */]
[@position(<x>, <y>)]
create [or modify] <entity-type> entity <qualified-name> (
  [<attribute-definition> [, ...]]
)
[index (<column-list>)]
[index (<column-list>)]
```

**Entity Types:**
- `persistent` - Stored in database
- `non-persistent` - In-memory only
- `view` - Based on OQL query (see CREATE VIEW ENTITY)
- `external` - From external data source

**Attribute Definition:**
```sql
[/** <documentation> */]
<name>: <type> [not null [error '<message>']] [unique [error '<message>']] [default <value>] [calculated]
```

**Examples:**

```sql
/** Customer entity */
@position(100, 200)
create persistent entity Sales.Customer (
  /** Unique customer ID */
  CustomerId: autonumber not null unique,

  /** Customer full name */
  Name: string(200) not null error 'Name is required',

  Email: string(200) unique error 'Email must be unique',

  Balance: decimal default 0,

  IsActive: boolean default true,

  CreatedDate: datetime,

  status: enumeration(Sales.CustomerStatus) default 'Active'
)
index (Name)
index (Email);
/
```

```sql
create non-persistent entity Sales.CustomerFilter (
  SearchName: string(200),
  MinBalance: decimal,
  MaxBalance: decimal
);
```

### CREATE OR MODIFY ENTITY

Creates an entity if it doesn't exist, or modifies it if it does.

```sql
create or modify persistent entity Sales.Customer (
  CustomerId: autonumber not null unique,
  Name: string(200) not null,
  Email: string(200),
  Phone: string(50)  -- New attribute added
);
```

### CREATE VIEW ENTITY

Creates a view entity based on an OQL query.

**Syntax:**
```sql
create view entity <qualified-name> (
  <attribute-definition> [, ...]
) as
  <oql-query>
```

**Example:**
```sql
create view entity Reports.CustomerSummary (
  CustomerName: string,
  TotalOrders: integer,
  TotalAmount: decimal
) as
  select
    c.Name as CustomerName,
    count(o.OrderId) as TotalOrders,
    sum(o.Amount) as TotalAmount
  from Sales.Customer c
  left join Sales.Order o on o.Customer = c
  GROUP by c.Name;
/
```

### DROP ENTITY

Removes an entity from the domain model.

**Syntax:**
```sql
drop entity <qualified-name>
```

---

## ALTER ENTITY Statements

### ALTER ENTITY

Modifies an existing entity's attributes, indexes, or documentation without recreating it.

**Syntax:**
```sql
alter entity <qualified-name>
  add (<attribute-definition> [, ...])

alter entity <qualified-name>
  drop (<attribute-name> [, ...])

alter entity <qualified-name>
  modify (<attribute-definition> [, ...])

alter entity <qualified-name>
  rename <old-name> to <new-name>

alter entity <qualified-name>
  add index (<column-list>)

alter entity <qualified-name>
  drop index (<column-list>)

alter entity <qualified-name>
  set documentation '<text>'

alter entity <qualified-name>
  set position (<x>, <y>)
```

**Examples:**
```sql
-- Add new attributes
alter entity Sales.Customer
  add (Phone: string(50), Notes: string(unlimited));

-- Drop attributes
alter entity Sales.Customer
  drop (Notes);

-- Rename attribute
alter entity Sales.Customer
  rename Phone to PhoneNumber;

-- Add index
alter entity Sales.Customer
  add index (Email);

-- Set documentation
alter entity Sales.Customer
  set documentation 'Customer master data';

-- Reposition entity on domain model canvas
alter entity Sales.Customer
  set position (100, 200);
```

---

## Association Statements

### CREATE ASSOCIATION

Creates an association between two entities.

**Syntax:**
```sql
[/** <documentation> */]
create association <qualified-name>
  from <parent-entity>
  to <child-entity>
  type <association-type>
  [owner <owner>]
  [delete_behavior <behavior>]
```

**Association Types:**
- `reference` - One-to-many (child has reference to parent)
- `ReferenceSet` - Many-to-many

**Owner Options:**
- `default` - Child owns the association
- `both` - Both ends can modify
- `Parent` - Parent owns the association
- `Child` - Child owns the association

**Delete Behavior:**
- `DELETE_BUT_KEEP_REFERENCES` - Delete object but keep references
- `DELETE_CASCADE` - Delete associated objects

**Example:**
```sql
/** Links orders to customers */
create association Sales.Order_Customer
  from Sales.Customer
  to Sales.Order
  type reference
  owner default
  delete_behavior DELETE_BUT_KEEP_REFERENCES;
/
```

```sql
create association Sales.Order_Product
  from Sales.Order
  to Sales.Product
  type ReferenceSet
  owner both;
/
```

### DROP ASSOCIATION

Removes an association.

**Syntax:**
```sql
drop association <qualified-name>
```

---

## Microflow Statements

### CREATE MICROFLOW

Creates a microflow with activities, parameters, return type, and control flow.

**Syntax:**
```sql
create [or modify] microflow <qualified-name>
  [folder '<path>']
begin
  [<statements>]
end
```

**Statement Types:**
```sql
-- Variable declaration
declare $Var type = value;
declare $entity Module.Entity;
declare $list list of Module.Entity = empty;

-- Object operations
$Var = create Module.Entity (attr = value);
change $entity (attr = value);
commit $entity [with events] [refresh];
delete $entity;
rollback $entity [refresh];

-- Retrieval
retrieve $Var from Module.Entity [where condition] [limit n];

-- Calls
$Result = call microflow Module.Name (Param = $value);
$Result = call nanoflow Module.Name (Param = $value);
$Result = call java action Module.Name (Param = value);

-- UI actions
show page Module.PageName ($Param = $value);
close page;

-- Validation and logging
validation feedback $entity/attribute message 'message';
log info|warning|error [node 'name'] 'message';

-- Control flow
if condition then ... [else ...] end if;
loop $item in $list begin ... end loop;
return $value;

-- Error handling (suffix on any activity)
... on error continue|rollback|{ handler };

-- Annotations (before any activity)
@position(x, y)
@caption 'text'
@color Green
@annotation 'text'
```

**Example:**
```sql
create microflow Sales.ACT_CreateOrder
folder 'Orders'
begin
  declare $Order Sales.Order;
  $Order = create Sales.Order (
    OrderDate = [%CurrentDateTime%],
    status = 'Draft'
  );
  commit $Order;
  show page Sales.Order_Edit ($Order = $Order);
  return $Order;
end;
```

### DESCRIBE MICROFLOW

Shows the full MDL definition of an existing microflow (round-trippable output).

### DROP MICROFLOW

```sql
drop microflow <qualified-name>
```

### CREATE NANOFLOW

Creates a nanoflow (client-side flow). Uses the same body syntax as microflows, but only client-side activities are allowed (no Java actions, REST calls, workflow actions, etc.).

**Syntax:**
```sql
create [or modify] nanoflow <qualified-name>
  [folder '<path>']
begin
  [<statements>]
end;
```

**Restrictions:**
- No `ErrorEvent`, Java action calls, REST/web service calls, workflow actions, import/export mapping actions, or database queries
- Return type cannot be `Binary`
- Activities are the same as microflows minus server-side-only actions (see `PROPOSAL_nanoflow_support.md` for the full list of 22 disallowed action types)

**Example:**
```sql
create nanoflow Shop.ACT_ValidateCart
folder 'Cart'
begin
  declare $Valid boolean = true;
  if $Cart/ItemCount = 0 then
    validation feedback $Cart/ItemCount message 'Cart is empty';
    set $Valid = false;
  end if;
  return $Valid;
end;
```

### DESCRIBE NANOFLOW

Shows the full MDL definition of an existing nanoflow (round-trippable output as `CREATE OR MODIFY NANOFLOW`).

### DROP NANOFLOW

```sql
drop nanoflow <qualified-name>
```

---

## Page Statements

### CREATE PAGE

Creates a page with a widget tree.

**Syntax:**
```sql
create [or replace] page <qualified-name>
(
  [params: { $Param: Module.Entity | type [, ...] },]
  title: '<title>',
  layout: <Module.LayoutName>
  [, folder: '<path>']
)
{
  <widget-tree>
}
```

**Widget Syntax:**
```sql
WIDGET_TYPE widgetName (Property: value, ...) [{ children }]
```

**Supported Widget Types:**
- Layout: `layoutgrid`, `row`, `column`, `container`, `customcontainer`
- Input: `textbox`, `textarea`, `checkbox`, `radiobuttons`, `datepicker`, `combobox`
- Display: `dynamictext`, `datagrid`, `gallery`, `listview`, `image`, `staticimage`, `dynamicimage`
- Actions: `actionbutton`, `linkbutton`, `navigationlist`
- Structure: `dataview`, `header`, `footer`, `controlbar`, `snippetcall`

**Example:**
```sql
create page MyModule.Customer_Edit
(
  params: { $Customer: MyModule.Customer },
  title: 'Edit Customer',
  layout: Atlas_Core.PopupLayout
)
{
  dataview dvCustomer (datasource: $Customer) {
    textbox txtName (label: 'Name', attribute: Name)
    textbox txtEmail (label: 'Email', attribute: Email)
    footer footer1 {
      actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
      actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
    }
  }
}
```

### DROP PAGE

```sql
drop page <qualified-name>
```

---

## ALTER PAGE / ALTER SNIPPET

Modifies an existing page or snippet's widget tree in-place without full `create or replace`.

**Syntax:**
```sql
alter page <qualified-name> {
  <operations>
}

alter snippet <qualified-name> {
  <operations>
}
```

**Operations:**
```sql
-- Set property on widget
set caption = 'New' on widgetName;
set (caption = 'Save', buttonstyle = success) on btn;

-- Set page-level property
set title = 'New Title';

-- Insert widgets
insert after widgetName { <widgets> };
insert before widgetName { <widgets> };

-- Remove widgets
drop widget name1, name2;

-- Replace widget
replace widgetName with { <widgets> };

-- Pluggable widget properties (quoted)
set 'showLabel' = false on cbStatus;
```

**Example:**
```sql
alter page Module.EditPage {
  set (caption = 'Save & Close', buttonstyle = success) on btnSave;
  drop widget txtUnused;
  insert after txtEmail {
    textbox txtPhone (label: 'Phone', attribute: Phone)
  }
};
```

---

## MOVE Statements

### MOVE

Moves documents (pages, microflows, snippets, nanoflows, entities, enumerations) between folders and modules.

**Syntax:**
```sql
-- Move to folder within same module
move page <qualified-name> to folder '<folder-path>';
move microflow <qualified-name> to folder '<folder-path>';
move snippet <qualified-name> to folder '<folder-path>';
move nanoflow <qualified-name> to folder '<folder-path>';
move enumeration <qualified-name> to folder '<folder-path>';

-- Move to module root
move page <qualified-name> to <module-name>;

-- Move entity to different module (no folder support)
move entity <qualified-name> to <module-name>;

-- Move to folder in different module
move page <qualified-name> to folder '<folder-path>' in <module-name>;
```

**Parameters:**
- `<qualified-name>` - The current Module.Name of the document
- `<folder-path>` - Target folder path (nested: 'Parent/Child')
- `<module-name>` - Target module name (for cross-module moves)

**Examples:**
```sql
-- Move page to folder
move page MyModule.CustomerEdit to folder 'Customers';

-- Move microflow to nested folder
move microflow MyModule.ACT_ProcessOrder to folder 'Orders/Processing';

-- Move snippet to different module
move snippet OldModule.NavigationMenu to Common;

-- Move entity to different module
move entity OldModule.Customer to NewModule;

-- Move enumeration to different module
move enumeration OldModule.OrderStatus to NewModule;

-- Move page to folder in different module
move page OldModule.CustomerPage to folder 'Screens' in NewModule;
```

**Warning:** Cross-module moves change the qualified name and may break by-name references. Use `show impact of <name>` to check before moving.

**Note:** `move entity` only supports moving to a module (not to a folder), since entities are embedded in domain model documents.

---

## Security Statements

### SHOW PROJECT SECURITY

Displays project-wide security settings.

**Syntax:**
```sql
show project security
```

### SHOW MODULE ROLES

Lists module roles, optionally filtered by module.

**Syntax:**
```sql
show module roles
show module roles in <module>
```

### SHOW USER ROLES

Lists project-level user roles.

**Syntax:**
```sql
show user roles
```

### SHOW DEMO USERS

Lists configured demo users.

**Syntax:**
```sql
show demo users
```

### SHOW ACCESS ON

Shows which roles have access to a specific element.

**Syntax:**
```sql
show access on microflow <module>.<name>
show access on nanoflow <module>.<name>
show access on page <module>.<name>
show access on <module>.<entity>
```

### SHOW SECURITY MATRIX

Displays a comprehensive access matrix for all or one module.

**Syntax:**
```sql
show security matrix
show security matrix in <module>
```

### CREATE MODULE ROLE

Creates a new module role within a module.

**Syntax:**
```sql
create module role <module>.<role> [description '<text>']
```

**Example:**
```sql
create module role Shop.Admin description 'Full administrative access';
create module role Shop.Viewer;
```

### DROP MODULE ROLE

Removes a module role.

**Syntax:**
```sql
drop module role <module>.<role>
```

### GRANT EXECUTE ON MICROFLOW

Grants execute access on a microflow to one or more module roles.

**Syntax:**
```sql
grant execute on microflow <module>.<name> to <module>.<role> [, ...]
```

**Example:**
```sql
grant execute on microflow Shop.ACT_Order_Process to Shop.User, Shop.Admin;
```

### REVOKE EXECUTE ON MICROFLOW

Removes execute access on a microflow from one or more module roles.

**Syntax:**
```sql
revoke execute on microflow <module>.<name> from <module>.<role> [, ...]
```

### GRANT EXECUTE ON NANOFLOW

Grants execute access on a nanoflow to one or more module roles.

**Syntax:**
```sql
grant execute on nanoflow <module>.<name> to <module>.<role> [, ...]
```

### REVOKE EXECUTE ON NANOFLOW

Removes execute access on a nanoflow from one or more module roles.

**Syntax:**
```sql
revoke execute on nanoflow <module>.<name> from <module>.<role> [, ...]
```

### GRANT VIEW ON PAGE

Grants view access on a page to one or more module roles.

**Syntax:**
```sql
grant view on page <module>.<name> to <module>.<role> [, ...]
```

### REVOKE VIEW ON PAGE

Removes view access on a page from one or more module roles.

**Syntax:**
```sql
revoke view on page <module>.<name> from <module>.<role> [, ...]
```

### GRANT (Entity Access)

Creates or updates an access rule on an entity for one or more module roles with CRUD permissions. **GRANT is additive** — if the role already has an access rule, new rights are merged without removing existing permissions.

**Syntax:**
```sql
grant <module>.<role> on <module>.<entity> (<rights>) [where '<xpath>']
```

Where `<rights>` is a comma-separated list of:
- `create` — allow creating instances
- `delete` — allow deleting instances
- `read *` — read all members, or `read (<attr>, ...)` for specific attributes
- `write *` — write all members, or `write (<attr>, ...)` for specific attributes

**Examples:**
```sql
-- Full access
grant Shop.Admin on Shop.Customer (create, delete, read *, write *);

-- Read-only
grant Shop.Viewer on Shop.Customer (read *);

-- Selective member access
grant Shop.User on Shop.Customer (read (Name, Email), write (Email));

-- With XPath constraint
grant Shop.User on Shop.Order (read *, write *) where '[Status = ''Open'']';

-- Additive: adds Phone to existing read access (Name, Email preserved)
grant Shop.User on Shop.Customer (read (Phone));
```

### REVOKE (Entity Access)

Removes an entity access rule entirely, or revokes specific rights.

**Syntax:**
```sql
-- Full revoke (removes entire rule)
revoke <module>.<role> on <module>.<entity>

-- Partial revoke (downgrades specific rights)
revoke <module>.<role> on <module>.<entity> (<rights>)
```

Partial revoke semantics: `revoke read (x)` sets member x to no access. `revoke write (x)` downgrades from ReadWrite to ReadOnly. `revoke create` / `revoke delete` removes the structural permission.

**Examples:**
```sql
-- Remove all access
revoke Shop.Viewer on Shop.Customer;

-- Remove read on specific attribute
revoke Shop.User on Shop.Customer (read (Phone));

-- Downgrade write to read-only
revoke Shop.User on Shop.Customer (write (Email));
```

### CREATE USER ROLE

Creates a project-level user role that aggregates module roles.

**Syntax:**
```sql
create user role <name> (<module>.<role> [, ...]) [manage all roles]
```

**Example:**
```sql
create user role AppAdmin (Shop.Admin, System.Administrator) manage all roles;
create user role AppUser (Shop.User);
```

### ALTER USER ROLE

Adds or removes module roles from a user role.

**Syntax:**
```sql
alter user role <name> add module roles (<module>.<role> [, ...])
alter user role <name> remove module roles (<module>.<role> [, ...])
```

### DROP USER ROLE

Removes a project-level user role.

**Syntax:**
```sql
drop user role <name>
```

### ALTER PROJECT SECURITY

Changes project-wide security settings.

**Syntax:**
```sql
alter project security level off | prototype | production
alter project security demo users on | off
```

### CREATE DEMO USER

Creates a demo user for development/testing.

**Syntax:**
```sql
create demo user '<username>' password '<password>' [entity <Module.Entity>] (<userrole> [, ...])
```

The optional `entity` clause specifies the entity that generalizes `System.User` (e.g., `Administration.Account`). If omitted, the system auto-detects the unique `System.User` subtype.

**Example:**
```sql
create demo user 'demo_admin' password 'Admin123!' (AppAdmin);
create demo user 'demo_admin' password 'Admin123!' entity Administration.Account (AppAdmin);
```

### DROP DEMO USER

Removes a demo user.

**Syntax:**
```sql
drop demo user '<username>'
```

---

## Navigation Statements

### SHOW NAVIGATION

```sql
show navigation                    -- Summary of all profiles
show navigation menu [<profile>]   -- Menu tree
show navigation homes              -- Home page assignments
```

### DESCRIBE NAVIGATION

```sql
describe navigation [<profile>]    -- Full MDL output (round-trippable)
```

### CREATE OR REPLACE NAVIGATION

```sql
create or replace navigation <profile>
  home page Module.HomePage
  [home page Module.AdminHome for Module.AdminRole]
  [login page Module.LoginPage]
  [not found page Module.Custom404]
  [menu (
    menu item 'Label' page Module.Page;
    menu 'Submenu' (
      menu item 'Label' page Module.Page;
    );
  )]
```

**Example:**
```sql
create or replace navigation Responsive
  home page MyModule.Home_Web
  home page MyModule.AdminHome for MyModule.Administrator
  login page Administration.Login
  menu (
    menu item 'Home' page MyModule.Home_Web;
    menu 'Admin' (
      menu item 'Users' page Administration.Account_Overview;
    );
  );
```

---

## Settings Statements

### SHOW / DESCRIBE SETTINGS

```sql
show settings              -- Overview of all settings
describe settings           -- Full MDL output (round-trippable)
```

### SHOW CONSTANT VALUES

```sql
show constant values [in module]   -- Compare constant values across configurations
```

Displays one row per constant per configuration. Shows the default value followed by any per-configuration overrides.

### ALTER SETTINGS

```sql
alter settings model key = value;
alter settings configuration 'Name' key = value;
alter settings constant 'Name' value 'val' in configuration 'cfg';
alter settings drop constant 'Name' in configuration 'cfg';
alter settings LANGUAGE key = value;
alter settings workflows key = value;
```

### CREATE / DROP CONFIGURATION

```sql
create configuration 'Name' [key = value, ...];
drop configuration 'Name';
```

**Example:**
```sql
alter settings model AfterStartupMicroflow = 'MyModule.ACT_Startup';
alter settings configuration 'default' DatabaseType = 'POSTGRESQL';
alter settings LANGUAGE DefaultLanguageCode = 'en_US';

-- View constant values across all configurations
show constant values;

-- Create a new configuration
create configuration 'Staging' DatabaseType = 'POSTGRESQL', DatabaseUrl = 'staging-db:5432';

-- Remove a constant override
alter settings drop constant 'MyModule.ApiKey' in configuration 'Default';

-- Drop a configuration
drop configuration 'Staging';
```

---

## External SQL Statements

Direct SQL query execution against external databases (PostgreSQL, Oracle, SQL Server). Credentials are isolated from session output.

### SQL CONNECT

```sql
sql connect <driver> '<dsn>' as <alias>
```

Drivers: `postgres` (pg, postgresql), `oracle` (ora), `sqlserver` (mssql).

### SQL Commands

```sql
sql connections                     -- List active connections (alias + driver only)
sql disconnect <alias>              -- Close connection
sql <alias> show tables             -- List user tables
sql <alias> show views              -- List user views
sql <alias> show FUNCTIONS          -- List functions/procedures
sql <alias> describe <table>        -- Column details
sql <alias> <any-sql>               -- Raw SQL passthrough
sql <alias> generate connector into <module> [tables (...)] [views (...)] [exec]
```

**Example:**
```sql
sql connect postgres 'postgres://user:pass@localhost:5432/mydb' as source;
sql source show tables;
sql source select * from users where active = true limit 10;
sql source generate connector into HRModule tables (employees, departments) exec;
sql disconnect source;
```

---

## Import Statements

### IMPORT

Imports data from an external database into a Mendix application database.

**Syntax:**
```sql
import from <alias> query '<sql>'
  into <Module.Entity>
  map (<source-col> as <AttrName> [, ...])
  [link (<source-col> to <AssocName> on <MatchAttr>) [, ...]]
  [batch <size>]
  [limit <count>]
```

**Example:**
```sql
import from source query 'SELECT name, email, dept_name FROM employees'
  into HR.Employee
  map (name as Name, email as Email)
  link (dept_name to Employee_Department on Name)
  batch 500
  limit 1000;
```

---

## Catalog and Search Statements

The catalog provides SQLite-based cross-reference queries over project metadata.

### REFRESH CATALOG

```sql
refresh catalog             -- Rebuild basic catalog
refresh catalog full        -- Rebuild including cross-references and source
```

### Catalog Queries

```sql
show catalog tables                           -- List available catalog tables
select ... from CATALOG.<table> [where ...]   -- SQL query against catalog
```

### Cross-Reference Navigation

Requires `refresh catalog full` to populate reference data.

```sql
show callers of <qualified-name>       -- What calls this element
show callees of <qualified-name>       -- What this element calls
show references of <qualified-name>    -- All references to/from
show impact of <qualified-name>        -- Impact analysis
show context of <qualified-name>       -- Surrounding context
```

### Full-Text Search

```sql
search '<keyword>'                     -- Search across all strings and source
```

---

## Business Event Statements

```sql
show business events [in <module>]
describe business event service <qualified-name>
create business event service <qualified-name> (...) { message ... }
drop business event service <qualified-name>
```

---

## Java Action Statements

```sql
-- List and inspect
show java actions [in <module>]
describe java action <qualified-name>

-- Create with inline Java code
create java action <qualified-name>(<params>) returns <type>
  [exposed as '<caption>' in '<category>']
  as $$ <java-code> $$

-- Type parameters for generic entity handling
create java action Module.Validate(
  EntityType: entity <pEntity> not null,
  InputObject: pEntity not null
) returns boolean as $$ return InputObject != null; $$

-- Drop
drop java action <qualified-name>
```

Parameter types: `string`, `integer`, `long`, `decimal`, `boolean`, `datetime`, `Module.Entity`, `list of Module.Entity`, `stringtemplate(sql)`, `stringtemplate(Oql)`, `entity <typeParam>`.

---

## Session Statements

### SET

Sets a session variable.

**Syntax:**
```sql
set <key> = <value>
```

**Example:**
```sql
set output_format = 'json'
set verbose = true
```

### REFRESH / UPDATE

Reloads the project from disk.

**Syntax:**
```sql
refresh
update
```

### EXECUTE SCRIPT

Executes an MDL script file.

**Syntax:**
```sql
execute script '<path>'
```

**Example:**
```sql
execute script './scripts/setup_domain_model.mdl';
```

### HELP

Shows available commands.

**Syntax:**
```sql
HELP
?
```

### EXIT / QUIT

Exits the REPL.

**Syntax:**
```sql
EXIT
QUIT
```

---

## Annotations

### @Position

Specifies the visual position in the domain model diagram.

**Syntax:**
```sql
@position(<x>, <y>)
```

**Example:**
```sql
@position(100, 200)
create persistent entity MyModule.Customer (
  ...
);
```

---

## Grammar (EBNF Summary)

This is a simplified overview. The complete formal grammar is defined in the ANTLR4 files:
- `mdl/grammar/MDLLexer.g4` - Lexer tokens
- `mdl/grammar/MDLParser.g4` - Parser rules

```ebnf
program         = { statement } ;

statement       = connect_stmt | disconnect_stmt | status_stmt
                | show_stmt | describe_stmt
                | create_module_stmt | drop_module_stmt
                | create_enum_stmt | alter_enum_stmt | drop_enum_stmt
                | create_entity_stmt | alter_entity_stmt | drop_entity_stmt
                | create_assoc_stmt | drop_assoc_stmt
                | create_microflow_stmt | drop_microflow_stmt
                | create_nanoflow_stmt | drop_nanoflow_stmt
                | create_page_stmt | alter_page_stmt | drop_page_stmt
                | move_stmt
                | security_stmt | grant_stmt | revoke_stmt
                | navigation_stmt | settings_stmt
                | sql_stmt | import_stmt
                | catalog_stmt | search_stmt
                | business_event_stmt | java_action_stmt
                | set_stmt | refresh_stmt | execute_script_stmt
                | help_stmt | exit_stmt ;

qualified_name  = IDENTIFIER [ '.' IDENTIFIER [ '.' IDENTIFIER ] ] ;

data_type       = 'String' [ '(' integer ')' ]
                | 'Integer' | 'Long' | 'Decimal' | 'Boolean'
                | 'DateTime' | 'Date' | 'AutoNumber' | 'Binary'
                | 'HashedString'
                | 'Enumeration' '(' qualified_name ')'
                | 'List' 'of' qualified_name
                | qualified_name ;

entity_type     = 'PERSISTENT' | 'NON-PERSISTENT' | 'VIEW' | 'EXTERNAL' ;

literal         = string | integer | decimal | 'TRUE' | 'FALSE' | 'NULL' ;
```
