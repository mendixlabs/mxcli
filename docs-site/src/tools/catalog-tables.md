# Available Tables

The catalog provides several tables that can be queried using standard SQL syntax. Use `LIST CATALOG TABLES` to list all available tables in your catalog.

## Discovering Tables

```sql
LIST CATALOG TABLES;
```

## Core Tables

### CATALOG.ENTITIES

Information about all entities in the project.

| Column | Description |
|--------|-------------|
| `Id` | Unique identifier |
| `Name` | Entity name |
| `ModuleName` | Module containing the entity |
| `QualifiedName` | Full qualified name (Module.Entity) |
| `EntityType` | Persistent, Non-Persistent, View, External |
| `Description` | Documentation text |
| `AttributeCount` | Number of attributes |
| `AccessRuleCount` | Number of access rules defined |

```sql
SELECT Name, EntityType, AttributeCount
FROM CATALOG.ENTITIES
WHERE ModuleName = 'Sales'
ORDER BY Name;
```

### CATALOG.ATTRIBUTES

Information about entity attributes.

| Column | Description |
|--------|-------------|
| `Id` | Unique identifier |
| `Name` | Attribute name |
| `EntityId` | Parent entity ID |
| `AttributeType` | String, Integer, Decimal, Boolean, DateTime, etc. |

```sql
SELECT a.Name, a.AttributeType
FROM CATALOG.ATTRIBUTES a
JOIN CATALOG.ENTITIES e ON a.EntityId = e.Id
WHERE e.QualifiedName = 'Sales.Customer';
```

### CATALOG.MODULES

One row per module, including System and Marketplace modules.

| Column | Description |
|--------|-------------|
| `Id` | Module UUID |
| `Name` | Module name |
| `Source` | `""` for your own modules and System; `"Marketplace v1.2.3"` for a downloaded module |
| `AppStoreVersion` | Marketplace version, when downloaded |
| `Description` | Always empty: a Mendix module has no documentation property |
| `DomainModelDocumentation` | The module's domain model's documentation — the module-level text an author can write |

```sql
SELECT Name FROM CATALOG.MODULES
WHERE Source = '' AND COALESCE(DomainModelDocumentation, '') = '';
```

### CATALOG.ASSOCIATIONS

Information about entity associations, same-module and cross-module.

| Column | Description |
|--------|-------------|
| `Id` | Unique identifier |
| `Name` | Association name |
| `QualifiedName` | `Module.Association` |
| `ModuleName` | Module that owns the association |
| `FromEntity` | FROM entity qualified name (the one that owns the reference) |
| `ToEntity` | TO entity qualified name; for a cross-module association, the other module's entity |
| `AssociationType` | `Reference` or `ReferenceSet` |
| `Owner` | `Default` or `Both` |
| `StorageFormat` | `Column` or `Table` |
| `Description` | Documentation |
| `ToDeleteBehavior` | Delete behaviour of the TO end — Mendix's `ChildDeleteBehavior`, the end MDL's `on delete` clause sets: `DeleteMeButKeepReferences` (default), `DeleteMeAndReferences`, `DeleteMeIfNoReferences` |
| `FromDeleteBehavior` | Delete behaviour of the FROM end — Mendix's `ParentDeleteBehavior` (Studio Pro only) |
| `ToDeleteErrorMessage` / `FromDeleteErrorMessage` | Message shown when a `DeleteMeIfNoReferences` delete on that end is refused |

Mendix's `Parent`/`Child` pointer names are inverted relative to MDL's FROM/TO:
`ParentPointer` is the FROM entity and `ChildPointer` the TO entity, so the
`Child*` delete behaviour belongs to the TO end. Mendix always stores both ends,
so an explicitly set behaviour is one that is not the default. System module
associations have no stored delete behaviour and read as `''`.

```sql
SELECT QualifiedName, ToDeleteBehavior, FromDeleteBehavior
FROM CATALOG.ASSOCIATIONS
WHERE ToDeleteBehavior <> 'DeleteMeButKeepReferences'
   OR FromDeleteBehavior <> 'DeleteMeButKeepReferences';
```

### CATALOG.MICROFLOWS

Information about microflows, nanoflows and rules (`CATALOG.NANOFLOWS` is the
nanoflow subset).

| Column | Description |
|--------|-------------|
| `Id` | Unique identifier |
| `Name` | Microflow name |
| `ModuleName` | Module containing the microflow |
| `QualifiedName` | Full qualified name |
| `Folder` | Folder path within the module |
| `MicroflowType` | `MICROFLOW`, `NANOFLOW` or `RULE` |
| `ReturnType` | Return type of the microflow |
| `Description` | Documentation text |
| `ParameterCount` | Number of parameters |
| `ActivityCount` | Activities at the top level of the flow, excluding start/end events and merges. A loop counts as one |
| `TotalActivityCount` | `ActivityCount` plus every activity inside a loop, at any depth |
| `Complexity` | McCabe cyclomatic complexity |

```sql
SELECT Name, ReturnType, ActivityCount, TotalActivityCount
FROM CATALOG.MICROFLOWS
WHERE ModuleName = 'Sales'
ORDER BY Name;
```

### CATALOG.ACTIVITIES

One row per object in a microflow, nanoflow or rule body — activities, splits,
merges, events, loops and annotations. Populated by `REFRESH CATALOG FULL`.

**Loop bodies are included.** The objects inside a loop, at any depth, are rows
too, with `ParentLoopId` naming the loop. Earlier releases left them out; a
query written then that should keep its old result filters on
`ParentLoopId = ''`.

| Column | Description |
|--------|-------------|
| `Id` | The object's ID |
| `MicroflowId`, `MicroflowQualifiedName`, `ModuleName` | The flow it belongs to |
| `Sequence` | Pre-order position in the flow: a loop, then its body, then the loop's next sibling |
| `ParentLoopId` | `Id` of the enclosing loop; empty at the top level |
| `LoopDepth` | Number of enclosing loops; 0 at the top level |
| `ActivityType` | `ActionActivity`, `ExclusiveSplit`, `InheritanceSplit`, `ExclusiveMerge`, `LoopedActivity`, `Annotation`, `StartEvent`, `EndEvent`, … |
| `ActionType` | The action of an `ActionActivity`, e.g. `RetrieveAction`, `JavaActionCallAction`, `WebServiceCallAction` |
| `Name` | `ActionType` for an action activity, otherwise `ActivityType` |
| `Caption` | The stored caption: an activity's, a split's, or an annotation's text. Empty for events, merges and loops. With `AutoGenerateCaption` it holds Studio Pro's stored placeholder (often `Activity`) |
| `AutoGenerateCaption` | 1 when Studio Pro generates the activity's caption |
| `Description` | Documentation of an action activity, split or loop |
| `EntityRef` | Entity of a create object, a database retrieve, or a delete (the deleted variable's entity, when the flow types it) |
| `ServiceRef`, `ActionRef` | Called service and operation: REST, web service, OData action. For a microflow, nanoflow, Java or JavaScript action call, `ActionRef` is the called document and `ServiceRef` is empty |
| `QueueRef` | Task queue a microflow or Java action call runs in; empty when it runs in place |
| `UseRequestTimeout`, `TimeoutExpression` | "Use a timeout" and its seconds, for a REST or web service call |
| `ConditionExpression` | An exclusive split's expression |
| `ConditionRule` | The rule a rule-based split calls |
| `ErrorHandlingType` | `Rollback`, `Custom`, `CustomWithoutRollBack` (capital B), `Continue` or `Abort` — read from the action for an action activity |
| `LogLevel`, `LogNodeExpression`, `LogMessage` | A log message action's level, node expression (`'MyNode'` or `getKey(…)`) and template |
| `CommitType` | Create/change object: `Yes`, `YesWithoutEvents` or `No` |
| `WithEvents` | 1 for a commit with events, and a create/change with `CommitType` `Yes` |
| `RetrieveSource` | `database` or `association` |

```sql
-- Database retrieves inside a loop (N+1 queries)
SELECT MicroflowQualifiedName, EntityRef, LoopDepth
FROM CATALOG.ACTIVITIES
WHERE ActionType = 'RetrieveAction' AND RetrieveSource = 'database' AND ParentLoopId <> '';

-- Microflows called from inside a loop, synchronously (a queued call runs outside it)
SELECT MicroflowQualifiedName, ActionRef
FROM CATALOG.ACTIVITIES
WHERE ActionType = 'MicroflowCallAction' AND ParentLoopId <> '' AND QueueRef = '';
```

### CATALOG.PAGES

Information about pages and their properties.

| Column | Description |
|--------|-------------|
| `Id` | Unique identifier |
| `Name` | Page name |
| `QualifiedName` | Full qualified name |
| `ModuleName` | Module containing the page |
| `Folder` | Folder path within the module |
| `Title` | Page title in the project's default language (else en_US, else the lowest-sorted non-empty language). Every translation is in `CATALOG.STRINGS` (`StringContext = 'Forms$Page.Title'`) |
| `URL` | Page URL if configured |
| `LayoutRef` | Qualified name of the layout (full build only) |
| `Description` | Documentation text |
| `ParameterCount` | Number of page parameters |
| `WidgetCount` | Number of widgets (full build only; 0 otherwise) |
| `Excluded` | 1 when the page is excluded from the project |
| `Class`, `Style`, `DynamicClasses` | The page's own Appearance: CSS class, inline style and dynamic-classes expression (full build only) |

```sql
SELECT Name, Title, URL
FROM CATALOG.PAGES
WHERE ModuleName = 'Sales'
ORDER BY Name;
```

Page **templates** are not pages and are not in this table — see
`CATALOG.PAGE_TEMPLATES`.

### CATALOG.WIDGETS

One row per widget **instance** on a page or snippet. Full build only
(`refresh catalog full`); a fast build leaves the table empty.

| Column | Description |
|--------|-------------|
| `Id` | The widget's element ID |
| `Name` | Widget name |
| `WidgetType` | Storage type (`Forms$DataView`, `Forms$ActionButton`, …); a pluggable widget's id (`com.mendix.widget.web.datagrid.Datagrid`) |
| `ContainerId`, `ContainerQualifiedName`, `ContainerType` | The page, snippet or layout holding the widget; `ContainerType` is `PAGE`, `SNIPPET` or `LAYOUT`. `UPDATE WIDGETS` changes page and snippet widgets only |
| `ModuleName`, `Folder` | The container's module and folder |
| `EntityRef`, `AttributeRef`, `MicroflowRef`, `NanoflowRef`, `PageRef` | What the widget's own content references: datasource entity, bound attribute, action or datasource flow, the page its action opens |
| `ParentWidgetId` | `Id` of the nearest widget **in this table** that encloses it; empty at the page or snippet root. What the catalog skips is transparent: the synthetic `conditionalVisibilityWidget…` container, layout grid rows and columns, tab pages, a pluggable widget's properties and object-list items. A widget in a layout grid column or a data grid 2 column has the grid as its parent |
| `Depth` | Number of ancestors in this table: 0 at the root. A list view **template** is a row of its own, so its widgets are two below the list view. A snippet call is not entered: a snippet's widgets have their own rows, depth 0 at the snippet root |
| `Class`, `Style`, `DynamicClasses` | The widget's Appearance |
| `ActionType` | Stored type of the primary action — `Action` (buttons), else `OnClickAction` (containers), else `ClickAction` (list views, images): `Forms$DeleteClientAction`, `Forms$MicroflowAction`, `Forms$CallNanoflowClientAction`, `Forms$FormAction` (show page), `Forms$NoAction`, …; empty for a widget without one. A pluggable widget's actions are not read |
| `HasConfirmation` | 1 when the primary action asks for confirmation. Only microflow, nanoflow and workflow calls have that setting; a delete action never does |

```sql
-- Widgets nested more than five deep, deepest first
SELECT ContainerQualifiedName, Name, WidgetType, Depth
FROM CATALOG.WIDGETS
WHERE Depth > 5
ORDER BY Depth DESC;

-- Buttons that delete without going through a flow
SELECT ContainerQualifiedName, Name
FROM CATALOG.WIDGETS
WHERE ActionType = 'Forms$DeleteClientAction';
```

### CATALOG.PAGE_TEMPLATES

The starting points Studio Pro's "new page" dialog offers (`Forms$PageTemplate`).
A separate document type from pages: Atlas_Web_Content ships 46 templates and no
pages at all.

| Column | Description |
|--------|-------------|
| `Id` | Unique identifier |
| `Name` | Template name |
| `ModuleName` | Module containing the template |
| `QualifiedName` | Full qualified name |
| `Folder` | Folder path within the module |
| `Description` | Documentation |

There is no `DESCRIBE PAGE TEMPLATE`, so a template is indexed but not
describable — tools that walk a module report it as *unknown*, never as
unchanged.

### CATALOG.LAYOUTS

Page layouts (`Forms$Layout`), including those in Marketplace modules such as
Atlas_Core.

| Column | Description |
|--------|-------------|
| `Id` | Unique identifier |
| `Name` | Layout name |
| `QualifiedName` | Full qualified name |
| `ModuleName` | Module containing the layout |
| `Folder` | Folder path within the module |
| `LayoutType` | `Responsive`, `Phone`, `Tablet`, `ModalPopup` (web); `Default`, `Popup` (native) |
| `Platform` | `Web` or `Native` — the platform every page on this layout renders on. React-client errors such as CE0582 apply to `Web` pages only |
| `Description` | Documentation text |
| `Class`, `Style`, `DynamicClasses` | The layout's own Appearance (full build only). Its widgets are in `CATALOG.WIDGETS` with `ContainerType = 'LAYOUT'` |

```sql
-- pages on a native layout (needs a full build for LayoutRef)
SELECT p.QualifiedName
FROM CATALOG.PAGES p JOIN CATALOG.LAYOUTS l ON l.QualifiedName = p.LayoutRef
WHERE l.Platform = 'Native';
```

### CATALOG.ACCESS_RULES

Information about entity access rules (available after full refresh).

| Column | Description |
|--------|-------------|
| `Id` | Unique identifier |
| `EntityId` | Entity this rule applies to |
| `UserRole` | Role this rule grants access to |
| `AllowRead` | Whether read access is granted |
| `AllowWrite` | Whether write access is granted |

```sql
SELECT e.QualifiedName, ar.UserRole, ar.AllowRead, ar.AllowWrite
FROM CATALOG.ACCESS_RULES ar
JOIN CATALOG.ENTITIES e ON ar.EntityId = e.Id
WHERE e.ModuleName = 'Sales';
```

### CATALOG.SCHEDULED_EVENTS

Scheduled events — Mendix's cron.

| Column | Description |
|--------|-------------|
| `Name`, `QualifiedName`, `ModuleName`, `Folder` | Identity |
| `Microflow` | Qualified name of the microflow the event runs |
| `Repeat` | Schedule variant: `Minute`, `Hour`, `Day`, `Week`, `MonthDate`, `MonthWeekday`, `YearDate`, `YearWeekday` |
| `RepeatDescription` | The schedule as a phrase, e.g. `weekly Mon/Fri at 09:30` |
| `IntervalSeconds` | Gap between runs, derived from the schedule |
| `Enabled` | 1 if the event runs |
| `TimeZone` | `UTC` or `Server` |
| `OnOverlap` | `DelayNext` or `SkipNext` |

`IntervalSeconds` comes from the schedule, **not** from the stored
`Interval`/`IntervalType` pair. Those are a legacy sibling that Studio Pro writes
and does not keep in sync — a shipped Mendix module stores `0`/`Minute` beside a
daily schedule — so a query keyed on them would read a nightly job as firing
every 0 seconds. Month and year figures are averages (30 and 365 days): the
column is for thresholds and ordering, not calendar arithmetic.

```sql
-- Anything that fires more often than once a minute
select QualifiedName, RepeatDescription, Microflow
from CATALOG.SCHEDULED_EVENTS
where Enabled = 1 and IntervalSeconds < 60;

-- Scheduled events whose microflow no longer exists
select se.QualifiedName, se.Microflow
from CATALOG.SCHEDULED_EVENTS se
left join CATALOG.MICROFLOWS m on m.QualifiedName = se.Microflow
where m.Id is null;
```

A scheduled event also produces a `schedule` row in `CATALOG.REFS`, so
`list callers of <microflow>` and the dead-asset analysis both see it. Without
that edge a microflow run only by a scheduled event looked unreferenced.

### CATALOG.TASK_QUEUES

Task queues. (`CATALOG.QUEUES` is the old name, still accepted.)

| Column | Description |
|--------|-------------|
| `Name`, `QualifiedName`, `ModuleName`, `Folder` | Identity |
| `Parallelism` | How many tasks run at once — an **expression string**, not a number |
| `ClusterWide` | 1 if the limit applies across the cluster |

`Parallelism` is stored as text because Mendix stores an expression: a query must
not assume it parses as an integer.

```sql
select QualifiedName, Parallelism, ClusterWide from CATALOG.TASK_QUEUES;
```

### Offline synchronization

`CATALOG.OFFLINE_ENTITY_CONFIGS` — one row per entity an offline navigation
profile synchronizes.

```sql
select ProfileName, EntityQualifiedName, SyncMode, XPathConstraint
  from CATALOG.OFFLINE_ENTITY_CONFIGS
 where SyncMode = 'All';
```

`CATALOG.NAVIGATION_PROFILES.OfflineEntityCount` says how many and nothing
else; this table is what makes "which entities does this profile sync, and
how?" answerable — the first question anyone auditing an offline app asks.

`SyncMode` is the value Mendix stores, not the caption Studio Pro shows: `All`,
`Constrained`, `Never`, `None`, `NoneAndPreserveData`, `Online`. Its dialog's
"All Objects" is `All` and "By XPath" is `Constrained`.

`CompatibilityMode` is indexed although MDL cannot author it. The catalog
reports what is stored, and a flag invisible to every query is one nobody
discovers until it matters.

A configured entity also produces a `sync` row in `CATALOG.REFS`, so
`list references to Sales.Order` names the profiles that download it:

```sql
select SourceName, TargetName from CATALOG.REFS where RefKind = 'sync';
```

**Every mode gets an edge, including the ones that download nothing.** A
profile with `sync Sales.Audit never` still *names* that entity, so renaming or
dropping it leaves the configuration dangling — which is precisely what a
reference edge exists to reveal.

### Entity event handlers

`CATALOG.ENTITY_EVENT_HANDLERS` — one row per entity event handler: which
moment, which event, and which microflow runs.

```sql
select EntityQualifiedName, Moment, Event, Microflow
  from CATALOG.ENTITY_EVENT_HANDLERS
 where Moment = 'Before' and Event = 'Commit';
```

`CATALOG.ENTITIES.HasEventHandlers` says some exist and nothing else — the same
shape `NavigationProfile.OfflineEntityCount` had. The distinction the flag loses
is the one that matters: a `Before` handler with `RaiseErrorOnFalse` can **veto**
the commit, an `After` handler cannot.

`Moment` is `Before` or `After`. `Event` is the value Mendix stores, not the
caption Studio Pro shows: `Create`, `Commit`, `Delete`, `RollBack` — note the
**capital B**, which `generated/metamodel` confirms
(`DomainModelsEventRollBack = "RollBack"`) and which disagrees with every
neighbouring enum in that file, where the same word is `Rollback`. A query
spelling it the expected way returns zero rows rather than an error.

A handler also produces an `event` row in `CATALOG.REFS`, so a microflow that
runs only as a handler has callers:

```sql
select SourceName, TargetName from CATALOG.REFS where RefKind = 'event';
```

Without that edge the handler microflow was reported dead from three directions
at once — `list callers` said `(no callers found)`, `CATALOG.GRAPH_DEAD_ASSETS`
listed it, and `mxcli lint` emitted **QUAL004** "is not called from anywhere"
with the suggestion *Remove if unused* — on code that runs on every commit
([mendixlabs/mxcli#1127](https://github.com/mendixlabs/mxcli/issues/1127)).

The edge carries neither the moment nor the event; `refs` has no column for
them, and a kind per combination would put eight kinds into every consumer's
list to say one thing. Which moment and which event is this table's question.

## Graph-Analysis Tables

The dependency graph (`CATALOG.REFS`, full refresh) is analysed by a family of
`graph_*` views and tables — god nodes, module coupling/cohesion, dead documents,
communities, cycles, layers, centrality, and the integration surface. The
community/cycle/layer/centrality tables are populated by `REFRESH CATALOG
COMMUNITIES` — a pass, not a build mode, so a full-mode catalog has them empty
until it runs and a query against one says so rather than returning `0 rows`.
See **[Graph Analysis](graph-analysis.md)** for the full reference and the
`mxcli graph-report` command.

Cycles come in two flavours, because they are two questions:
`CATALOG.GRAPH_CYCLES` for tangled **documents** and `CATALOG.GRAPH_MODULE_CYCLES`
for tangled **modules** — the second is not a rollup of the first, since modules
reference each other through documents that need form no cycle themselves.
`CATALOG.GRAPH_ANALYSIS_SCOPE` reports which reference kinds reach the computed
asset graph, which is what to check when a `graph_module_coupling` edge seems to
have no effect on cycles or layers.

```sql
select * from CATALOG.graph_god_nodes order by Degree desc limit 20;
select * from CATALOG.graph_module_coupling order by Edges desc;
select * from CATALOG.graph_module_cycles order by CycleSize desc;
list communities;
```

## Listing All Tables

To see the complete list of available tables in your catalog (which may vary by project and refresh level):

```sql
LIST CATALOG TABLES;
```

```bash
mxcli -p app.mpr -c "LIST CATALOG TABLES"
```

### Renamed tables

The integration and queue tables follow Studio Pro's document names. The old
names still answer as aliases, but `LIST CATALOG TABLES` shows only the new ones,
and `CATALOG.OBJECTS` reports the new `ObjectType` values:

| Table | Old name | `ObjectType` (old) |
|-------|----------|--------------------|
| `CATALOG.CONSUMED_REST_SERVICES` | `REST_CLIENTS` | `CONSUMED_REST_SERVICE` (`REST_CLIENT`) |
| `CATALOG.CONSUMED_REST_OPERATIONS` | `REST_OPERATIONS` | — |
| `CATALOG.CONSUMED_ODATA_SERVICES` | `ODATA_CLIENTS` | `CONSUMED_ODATA_SERVICE` (`ODATA_CLIENT`) |
| `CATALOG.PUBLISHED_ODATA_SERVICES` | `ODATA_SERVICES` | `PUBLISHED_ODATA_SERVICE` (`ODATA_SERVICE`) |
| `CATALOG.TASK_QUEUES` | `QUEUES` | `TASK_QUEUE` (`QUEUE`) |

A query that filters `OBJECTS` on an old `ObjectType` value returns no rows;
change it to the new value.
