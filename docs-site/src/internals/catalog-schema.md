# SQLite Schema

The catalog uses an in-memory SQLite database with the following table definitions.

## Core Tables

### MODULES

```sql
CREATE TABLE modules_data (
    Id                       TEXT PRIMARY KEY,
    Name                     TEXT,
    QualifiedName            TEXT,
    ModuleName               TEXT,
    Folder                   TEXT,
    Description              TEXT,   -- always empty: a module has no documentation
    Source                   TEXT,   -- '' or 'Marketplace v…'
    AppStoreVersion          TEXT,
    AppStoreGuid             TEXT,
    DomainModelDocumentation TEXT,   -- the module's domain model's documentation
    ProjectId                TEXT,
    SnapshotId               TEXT
);
```

### ASSOCIATIONS

```text
CREATE TABLE associations_data (
    Id                     TEXT PRIMARY KEY,
    Name                   TEXT,
    QualifiedName          TEXT,
    ModuleName             TEXT,
    FromEntity             TEXT,   -- Mendix ParentPointer (owns the reference)
    ToEntity               TEXT,   -- Mendix ChildPointer
    AssociationType        TEXT,
    Owner                  TEXT,
    StorageFormat          TEXT,
    Description            TEXT,
    ToDeleteBehavior       TEXT,   -- Mendix DeleteBehavior.ChildDeleteBehavior
    FromDeleteBehavior     TEXT,   -- Mendix DeleteBehavior.ParentDeleteBehavior
    ToDeleteErrorMessage   TEXT,   -- ChildErrorMessage text
    FromDeleteErrorMessage TEXT,   -- ParentErrorMessage text
    ProjectId              TEXT,
    SnapshotId             TEXT
);
```

### ENTITIES

```sql
CREATE TABLE ENTITIES (
    Name            TEXT PRIMARY KEY,   -- Qualified: Module.Entity
    ModuleName      TEXT,
    EntityName      TEXT,
    Persistent      BOOLEAN,
    AttributeCount  INTEGER,
    Documentation   TEXT
);
```

### MICROFLOWS

Microflows, nanoflows and rules share one table; `NANOFLOWS` is a view of the
nanoflow rows.

```sql
CREATE TABLE microflows_data (
    Id                  TEXT PRIMARY KEY,
    Name                TEXT,
    QualifiedName       TEXT,       -- Module.Microflow
    ModuleName          TEXT,
    Folder              TEXT,
    MicroflowType       TEXT,       -- MICROFLOW, NANOFLOW or RULE
    Description         TEXT,
    ReturnType          TEXT,
    ParameterCount      INTEGER,
    ActivityCount       INTEGER,    -- top level only; a loop counts as one
    TotalActivityCount  INTEGER,    -- including loop bodies, at any depth
    Complexity          INTEGER,    -- McCabe
    Excluded            BOOLEAN,
    ProjectId           TEXT,
    SnapshotId          TEXT
);
```

### PAGES

```sql
CREATE TABLE PAGES (
    Name        TEXT PRIMARY KEY,
    ModuleName  TEXT,
    PageName    TEXT,
    Layout      TEXT,
    Url         TEXT,
    Documentation TEXT
);
```

### LAYOUTS

```text
CREATE TABLE LAYOUTS (
    Id            TEXT PRIMARY KEY,
    Name          TEXT,
    QualifiedName TEXT,
    ModuleName    TEXT,
    Folder        TEXT,
    LayoutType    TEXT,   -- Responsive / Phone / Tablet / ModalPopup / Default / Popup
    Platform      TEXT,   -- 'Web' or 'Native' (the content wrapper's type)
    Description   TEXT,
    Class         TEXT,   -- the layout's own Appearance (full build only)
    Style         TEXT,
    DynamicClasses TEXT
);
```

### SNIPPETS

```sql
CREATE TABLE SNIPPETS (
    Name        TEXT PRIMARY KEY,
    ModuleName  TEXT,
    SnippetName TEXT
);
```

### ENUMERATIONS

```sql
CREATE TABLE ENUMERATIONS (
    Name        TEXT PRIMARY KEY,
    ModuleName  TEXT,
    EnumName    TEXT,
    ValueCount  INTEGER
);
```

### CONSTANTS

```sql
CREATE TABLE CONSTANTS (
    Id              TEXT PRIMARY KEY,
    Name            TEXT,
    QualifiedName   TEXT,
    ModuleName      TEXT,
    Folder          TEXT,
    Description     TEXT,
    DataType        TEXT,               -- String, Integer, Boolean, etc.
    DefaultValue    TEXT,
    ExposedToClient INTEGER DEFAULT 0
);
```

### CONSTANT_VALUES

Per-configuration constant overrides. Join with `CONSTANTS` on `ConstantName = QualifiedName`.

```sql
CREATE TABLE CONSTANT_VALUES (
    Id                INTEGER PRIMARY KEY AUTOINCREMENT,
    ConstantName      TEXT NOT NULL,     -- Qualified: Module.Constant
    ConfigurationName TEXT NOT NULL,     -- e.g., "Default", "Production"
    Value             TEXT
);
```

### WORKFLOWS

```sql
CREATE TABLE WORKFLOWS (
    Name            TEXT PRIMARY KEY,
    ModuleName      TEXT,
    WorkflowName    TEXT,
    ActivityCount   INTEGER
);
```

## Full Refresh Tables

These tables are only populated by `REFRESH CATALOG FULL`.

### ACTIVITIES

One row per object of a flow body, loop bodies included (`ParentLoopId` names
the enclosing loop; filter on `ParentLoopId = ''` for the top level only).

```sql
CREATE TABLE activities_data (
    Id                      TEXT PRIMARY KEY,
    Name                    TEXT,       -- ActionType for an action, else ActivityType
    Caption                 TEXT,       -- stored caption / annotation text
    ActivityType            TEXT,       -- e.g. "ActionActivity", "ExclusiveSplit", "LoopedActivity"
    Sequence                INTEGER,    -- pre-order position within the flow
    MicroflowId             TEXT,
    MicroflowQualifiedName  TEXT,
    ModuleName              TEXT,
    Folder                  TEXT,
    EntityRef               TEXT,       -- create object / database retrieve / delete entity
    ActionType              TEXT,       -- e.g. "RetrieveAction", "MicroflowCallAction"
    ServiceRef              TEXT,       -- called service (REST, web service, OData)
    ActionRef               TEXT,       -- operation within it, or the called microflow/nanoflow/Java/JavaScript action
    UseRequestTimeout       INTEGER,
    TimeoutExpression       TEXT,
    Description             TEXT,       -- documentation
    ParentLoopId            TEXT,       -- enclosing loop's Id; '' at the top level
    LoopDepth               INTEGER,    -- 0 at the top level
    AutoGenerateCaption     INTEGER,
    ConditionExpression     TEXT,       -- exclusive split expression
    ConditionRule           TEXT,       -- rule called by a rule-based split
    ErrorHandlingType       TEXT,       -- Rollback / Custom / CustomWithoutRollBack / Continue / Abort
    LogLevel                TEXT,
    LogNodeExpression       TEXT,
    LogMessage              TEXT,
    CommitType              TEXT,       -- Yes / YesWithoutEvents / No
    WithEvents              INTEGER,
    RetrieveSource          TEXT,       -- database / association
    QueueRef                TEXT,       -- task queue a microflow/Java action call runs in
    ProjectId               TEXT,
    SnapshotId              TEXT
);
```

### WIDGETS

One row per widget instance; written by `refresh catalog full` only.

```sql
CREATE TABLE WIDGETS (
    Id                      TEXT PRIMARY KEY,
    Name                    TEXT,
    WidgetType              TEXT,    -- "Forms$TextBox", or a pluggable widget's id
    ContainerId             TEXT,    -- the page, snippet or layout
    ContainerQualifiedName  TEXT,
    ContainerType           TEXT,    -- "PAGE", "SNIPPET" or "LAYOUT"
    ModuleName              TEXT,
    Folder                  TEXT,
    EntityRef               TEXT,
    AttributeRef            TEXT,
    MicroflowRef            TEXT,
    NanoflowRef             TEXT,
    PageRef                 TEXT,
    Description             TEXT,
    ParentWidgetId          TEXT,    -- nearest catalogued ancestor; '' at the root
    Depth                   INTEGER, -- catalogued ancestors; 0 at the root
    Class                   TEXT,    -- Appearance
    Style                   TEXT,
    DynamicClasses          TEXT,
    ActionType              TEXT,    -- $Type of Action / OnClickAction / ClickAction
    HasConfirmation         INTEGER, -- 1 when that action has a ConfirmationInfo
    ProjectId               TEXT,
    SnapshotId              TEXT
);
```

The tree columns (schema 19) skip what the walk does not index: the synthetic
`conditionalVisibilityWidget…` container, layout grid rows and columns, tab
pages, and a pluggable widget's properties and object-list items, so a widget in
a data grid 2 column has the grid as its parent. A list view template is
indexed (it carries its own entity) and so is a level of its own. The walk does
not follow a snippet call; the snippet's widgets are rows of the snippet, depth
0 at its root. `HasConfirmation` can only be 1 for a microflow, nanoflow or
workflow call — `Forms$DeleteClientAction` has no confirmation property.

### REFS

The reference graph: one row per edge. Populated by `refresh catalog full`.

```sql
CREATE TABLE REFS (
    Id          INTEGER PRIMARY KEY AUTOINCREMENT,
    SourceType  TEXT NOT NULL,  -- "MICROFLOW", "PAGE", "ENTITY", ...
    SourceId    TEXT NOT NULL,  -- element $ID, or '' where the builder has no id
    SourceName  TEXT NOT NULL,  -- referencing document, module-qualified
    TargetType  TEXT NOT NULL,  -- "ENTITY", "MICROFLOW", "WIDGET", ...
    TargetId    TEXT,           -- element $ID, or the widget ID for a WIDGET target
    TargetName  TEXT NOT NULL,  -- referenced element
    RefKind     TEXT NOT NULL,  -- see the vocabulary below
    ModuleName  TEXT,
    ProjectId   TEXT,
    SnapshotId  TEXT
);

CREATE INDEX idx_refs_source ON refs(SourceType, SourceName);
CREATE INDEX idx_refs_target ON refs(TargetType, TargetName);
CREATE INDEX idx_refs_kind   ON refs(RefKind);
```

`RefKind` values are lower-case, and the current vocabulary is whatever
`CATALOG.GRAPH_REFKIND_DISTRIBUTION` reports for your project — query that
rather than trusting a list here:

| RefKind | Edge |
|---------|------|
| `call` | flow calls a microflow / nanoflow / rule / Java action / REST operation; a workflow calls a handler microflow; a consumed OData service calls its configuration / headers / error-handling microflow |
| `create` / `change` / `delete` / `retrieve` | flow acts on an entity object |
| `commit` | flow commits an entity object: a commit action, or a create / change that commits (`Yes` or `YesWithoutEvents`), beside its `create` / `change` edge |
| `return` | flow returns an entity type |
| `parameter` | page or flow parameter entity type |
| `generalize` | entity extends entity |
| `associate` | association targets entity |
| `layout` | page uses a layout |
| `datasource` | page or widget reads an entity |
| `action` | widget calls a microflow / nanoflow |
| `show_page` | flow or widget action opens a page |
| `home_page` / `login_page` / `menu_item` | navigation profile references a page, or a microflow (home microflow, menu item that calls one) |
| `calculate` | calculated attribute uses a microflow |
| `schedule` | scheduled event runs a microflow |
| `publish` | published service runs a microflow: a REST operation, an OData resource's read / insert / update / delete microflow, an OData action, a REST or OData service's custom authentication microflow |
| `event` | event handler runs a microflow: an entity event, or a business event a service subscribes to |
| `settings` | a project setting (after-startup / before-shutdown / health check) names a microflow |
| `sync` | offline navigation profile synchronizes an entity |
| `validate` | attribute validation rule uses a regular expression |
| `widget` | page or snippet uses a pluggable / custom widget |

`change`, `delete` and `commit` act on a *variable*, so the entity is resolved
within the flow — from a parameter, a create or retrieve output, or a loop
iterator over one of those. A variable whose entity the flow cannot tell (a
microflow call's result, for one) has no edge. `commit` is not in the analysis
graph: the variable it commits comes from a parameter, create or retrieve that
already links the flow to the entity (or to the association it was retrieved
over), so it would mostly double existing edges.

`schedule`, `publish`, `event` and `settings` are **entry points**: something
outside the call graph runs the microflow, so nothing in the model calls it.
They are what stops `GRAPH_DEAD_ASSETS`, `LIST CALLERS OF` and lint rule QUAL004
from reporting a live scheduled job, API handler or event handler as unused.
`GRAPH_DEAD_ASSETS` and QUAL004 count an edge of any kind; `LIST CALLERS OF`
counts the kinds that invoke. The source of a service's edge is the service
itself (`PUBLISHED_ODATA_SERVICE`, `PUBLISHED_REST_SERVICE`,
`BUSINESS_EVENT_SERVICE`, `CONSUMED_ODATA_SERVICE`) with its catalog id in
`SourceId`; a published REST operation's edge comes from the operation.
`sync` is not one — it names an entity a profile downloads, which is a use of a
type, so `SHOW CALLERS` excludes it for the same reason it excludes
`datasource`.

#### WIDGET targets

A `widget` edge is the odd one out and is worth knowing about before you join
against it:

- `TargetName` is the widget's **MDL name** (`COMBOBOX`), not its dotted widget
  ID. The ID is in `TargetId`. A dotted target would be mis-read as a module by
  `GRAPH_MODULE_COUPLING` and friends, which take everything before the first
  dot as the module name.
- It is therefore the only `TargetName` that is not module-qualified — a widget
  definition belongs to no Mendix module. `GRAPH_GOD_NODES` excludes `WIDGET`
  targets from its asset list for that reason, while still counting a page's
  out-degree towards the widgets it uses.
- Only widgets with a definition get an edge. A built-in Mendix widget
  (`Forms$DynamicText`) has none, so it produces no row; use `CATALOG.WIDGETS`
  for those.
- One edge per page x widget, not per widget instance.

### PERMISSIONS

```sql
CREATE TABLE PERMISSIONS (
    RoleName    TEXT,           -- Module role
    TargetName  TEXT,           -- Entity, microflow, or page
    TargetKind  TEXT,           -- "Entity", "Microflow", "Page"
    Permission  TEXT            -- "Create", "Read", "Write", "Delete", "Execute", "View"
);
```

## Full-Text Search Tables

### STRINGS (FTS5)

```sql
CREATE VIRTUAL TABLE STRINGS USING fts5(
    QualifiedName,  -- Document qualified name, e.g. MyModule.Home
    ObjectType,     -- Document type, derived from the unit $Type: PAGE,
                    -- PAGE_TEMPLATE, BUILDING_BLOCK, MICROFLOW, ENUMERATION, ...
    StringValue,    -- The string itself
    StringContext,  -- Where it lives. For translatable text this is
                    -- <owner $Type>.<property>, e.g. Forms$ActionButton.Caption.
                    -- Non-translatable strings keep a plain label: page_url,
                    -- log_node, log_message, documentation, rest_path,
                    -- task_name, domain_model_documentation, ...
    Language,       -- Language code, empty for non-translatable strings
    ElementId,      -- The owning element's $ID — what distinguishes an
                    -- enumeration's twelve values from each other
    ModuleName
);
```

Every `Texts$Text` in the project is indexed, found by a type-agnostic walk
rather than per-document-type extraction, so a caption in a document type mxcli
cannot otherwise read is still searchable. That includes Atlas's design
templates (`PAGE_TEMPLATE`, `BUILDING_BLOCK`), which are roughly 70% of a stock
project's text and never render in a running app — filter them out with
`ObjectType` when you want only the app's own strings.

The documentation of an element (an entity, attribute, constant, page, ...) is a
plain string, not a `Texts$Text`, so it is added from the catalog tables that
already hold it, not by the walk: `StringContext` is `documentation`,
`ObjectType` names the element (`ENTITY`, `ATTRIBUTE`, `CONSTANT`, `PAGE`, ...)
and `QualifiedName` is the element's own (`Sales.Order.Total` for an
attribute). Microflow and workflow documentation is indexed with the same
labels. See [Full-Text Search](../tools/search.md) for the full list.

An empty translation — a text that exists but is not translated yet — is **not**
a row, so a language's presence in this table means it is actually translated
somewhere.

### SOURCE (FTS5)

```sql
CREATE VIRTUAL TABLE SOURCE USING fts5(
    name,           -- Document qualified name
    kind,           -- Document type
    source,         -- MDL source representation
    tokenize='porter unicode61'
);
```

## Querying Examples

```sql
-- Find entities with many attributes
SELECT Name, AttributeCount FROM CATALOG.ENTITIES
WHERE AttributeCount > 20 ORDER BY AttributeCount DESC;

-- Find all references to an entity
SELECT SourceName, RefKind FROM CATALOG.REFS
WHERE TargetName = 'Sales.Customer';

-- Which pages use a given pluggable widget?
SELECT SourceType, SourceName FROM CATALOG.REFS
WHERE RefKind = 'widget' AND TargetName = 'COMBOBOX';

-- Which installed widget packages does nothing use?
-- (MDL's SELECT has no NOT EXISTS / NOT IN — use an anti-join.)
SELECT d.MdlName, d.WidgetId FROM CATALOG.WIDGET_DEFINITIONS d
LEFT JOIN CATALOG.REFS r ON r.TargetId = d.WidgetId AND r.RefKind = 'widget'
WHERE r.Id IS NULL;

-- Full-text search
SELECT name, kind, snippet(STRINGS, 2, '<b>', '</b>', '...', 20)
FROM CATALOG.STRINGS WHERE strings MATCH 'validation error';

-- Find constants exposed to client
SELECT QualifiedName, DataType, DefaultValue FROM CATALOG.CONSTANTS
WHERE ExposedToClient = 1;

-- Compare constant values across configurations
SELECT c.QualifiedName, cv.ConfigurationName, cv.Value
FROM CATALOG.CONSTANTS c
JOIN CATALOG.CONSTANT_VALUES cv ON c.QualifiedName = cv.ConstantName
ORDER BY c.QualifiedName, cv.ConfigurationName;
```
