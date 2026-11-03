# Mendix Version Compatibility

Supported Mendix Studio Pro versions, feature availability matrix, and known limitations.

## Supported Versions

mxcli supports Mendix Studio Pro versions **9.x through 11.x**. Development and nightly testing targets five versions:

| Studio Pro Version | MPR Format | Nightly Tested | Status |
|-------------------|------------|----------------|--------|
| 9.x | v1 | No | Read-only support |
| 10.0 -- 10.17 | v1 | No | Supported |
| 10.18 -- 10.23 | v2 | No | Supported |
| 10.24 (LTS) | v2 | **Yes** | Supported |
| 11.0 -- 11.5 | v2 | No | Supported |
| 11.6 | v2 | **Yes** | Primary development target |
| 11.7 -- 11.11 | v2 | No | Supported |
| 11.12 | v2 | **Yes** | Supported |
| 11.13 | v2 | **Yes** | Supported |
| 11.14 | v2 | **Yes** | Latest tested |

The exact patch versions are in the `mendix-version` matrix in
[`.github/workflows/nightly.yml`](https://github.com/ako/mxcli/blob/main/.github/workflows/nightly.yml),
which is authoritative -- this table names the minors only.

## MPR Format Versions

### v1 (Mendix < 10.18)

- Single `.mpr` SQLite database file
- All documents stored as BSON blobs in the `Contents` column of the `Unit`
  table -- there is no separate contents table
- Self-contained -- one file holds the entire project

### v2 (Mendix >= 10.18)

- `.mpr` SQLite file for metadata only -- the `Unit` table has no `Contents`
  column
- `mprcontents/<XX>/<YY>/<UUID>.mxunit` -- one file per document
- Better suited for Git version control (smaller, per-document diffs)

The library auto-detects the format. No configuration is needed.

## Feature Availability Matrix

The tables below show exactly which features are available on each Mendix version. Data is sourced from `sdk/versions/mendix-{9,10,11}.yaml`.

### Domain Model

| Feature | MDL Syntax | 9.x | 10.0+ | 10.18+ | 11.0+ |
|---------|-----------|-----|-------|--------|-------|
| Persistent entities | `CREATE PERSISTENT ENTITY` | Yes | Yes | Yes | Yes |
| Non-persistent entities | `CREATE NON-PERSISTENT ENTITY` | Yes | Yes | Yes | Yes |
| Calculated attributes | `CALCULATED BY Module.Microflow` | Yes | Yes | Yes | Yes |
| Entity generalization | `EXTENDS Module.ParentEntity` | Yes | Yes | Yes | Yes |
| ALTER ENTITY | `ALTER ENTITY ... ADD/DROP/RENAME` | -- | Yes | Yes | Yes |
| View entities | `CREATE VIEW ENTITY ... AS SELECT` | -- | -- | 10.21+ | Yes |

### Microflows

| Feature | MDL Syntax | 9.x | 10.0+ | 10.6+ | 11.0+ |
|---------|-----------|-----|-------|-------|-------|
| Basic microflows | `CREATE MICROFLOW ... BEGIN ... END` | Yes | Yes | Yes | Yes |
| Loop in branches | `LOOP inside IF/ELSE` | Yes | Yes | Yes | Yes |
| Loops over a list, while loops | `LOOP $Item IN $List ...`, `WHILE ...` | 9.0.4+ | Yes | Yes | Yes |
| Show page with arguments | `SHOW PAGE Module.Page (Param = $Value)` | 9.7+ | Yes | Yes | Yes |
| Calls in a task queue | `CALL MICROFLOW ... IN QUEUE ... [RETRY FIXED (...)], [RETRY EXPONENTIAL (...)]`, `CALL JAVA ACTION ... IN QUEUE ... [RETRY ...]` | 9.10+ | Yes | Yes | Yes |
| Find and filter by expression | `FIND $List WHERE ...`, `FILTER $List WHERE ...` | 9.0.3+ | Yes | Yes | Yes |
| Call external action | `CALL EXTERNAL ACTION Module.Service.Action (...)` | 9.19+ (with arguments: --) | Yes (with arguments: 10.2+) | Yes | Yes |
| Range of a list | `RANGE $List OFFSET ... LIMIT ...` | 9.24+ | Yes | Yes | Yes |
| SEND REST REQUEST | `SEND REST REQUEST Module.Service.Op` | -- | 10.1+ | Yes | Yes |
| SEND REST REQUEST arguments, result, body | `... WITH (...)`, `$R = SEND REST REQUEST ...`, `... BODY $Var` | -- | 10.2+, 10.4+, 10.5+ | Yes | Yes |
| Transform JSON with a data transformer | `$Out = TRANSFORM $Json WITH Module.Transformer` | -- | -- | -- | 11.4+ |
| Execute database query | `EXECUTE DATABASE QUERY ...` | 9.24+ | Yes | Yes | Yes |
| REST query parameters | `SEND REST REQUEST with QUERY params` | -- | -- | -- | Yes |
| DB query runtime connection | `EXECUTE DATABASE QUERY ... CONNECTION (...)` | -- | -- | -- | 11.1+ |

Below its version, each of these is refused by `check -p` and by `exec` before
anything is written, with MDL-VERSION01, wherever it sits in the flow and in what
an `alter microflow` or `alter nanoflow` inserts. The versions are the Mendix
Model SDK's own for the type each writes; before this, `check -p` passed them and
the write refused them part-way through the script
([ako/mxcli#1811](https://github.com/ako/mxcli/issues/1811)). A `FIND` or
`FILTER` `BY Member = value` writes the older find and filter every version has.

### Pages

| Feature | MDL Syntax | 9.x | 10.0+ | 10.18+ | 11.0+ |
|---------|-----------|-----|-------|--------|-------|
| Basic pages | `CREATE PAGE ... { ... }` | Yes | Yes | Yes | Yes |
| ALTER PAGE | `ALTER PAGE ... SET/INSERT/DROP` | -- | Yes | Yes | Yes |
| Pluggable widgets | `DATAGRID`, `GALLERY`, `COMBOBOX`, `IMAGE` | -- | Yes | Yes | Yes |
| Conditional visibility | `Visible: <expression>` | -- | -- | -- | Yes |
| Conditional editability | `Editable: <expression>` | -- | -- | -- | Yes |
| Responsive column widths | `TabletWidth: 6, PhoneWidth: 12` | -- | -- | -- | Yes |
| Page parameters (entity) | `Params: ( $Item: Module.Entity )` | 9.4+ | Yes | Yes | Yes |
| Page parameters (primitive) | `Params: ( $Qty: Integer )` | -- | -- | -- | 11.6+ |
| Page variables | `Variables: ( ... )` | -- | 10.20+ | 10.20+ | Yes |
| Snippet parameters | `CREATE SNIPPET ... (Params: ( ... ))` | 9.21+ | Yes | Yes | Yes |
| Workflow actions on a button | `action: open user task`, `open workflow`, `call workflow ...`; `complete task '...'` | 9.0.2+; 9.19+ | Yes | Yes | Yes |
| Design properties (Atlas v3) | `DesignProperties: (...)` | -- | -- | -- | Yes |

::: tip Widget Templates
Pluggable widget templates are currently extracted from Mendix 11.6. When used on 10.x projects, the [MPK augmentation system](../internals/widget-templates.md) reconciles property differences. Some CE0463 ("widget definition changed") errors may still occur.
:::

### Security

| Feature | MDL Syntax | 9.x | 10.0+ |
|---------|-----------|-----|-------|
| Module roles | `CREATE MODULE ROLE` | Yes | Yes |
| User roles | `CREATE USER ROLE` | Yes | Yes |
| Entity access rules | `GRANT READ/WRITE ON ...` | Yes | Yes |
| Demo users | `CREATE DEMO USER` | Yes | Yes |

### Integration

| Feature | MDL Syntax | 10.0+ | 10.1+ | 10.4+ | 10.6+ | 11.0+ |
|---------|-----------|-------|-------|-------|-------|-------|
| OData client | `CREATE CONSUMED ODATA SERVICE` | Yes | Yes | Yes | Yes | Yes |
| Business events (9.24+) | `CREATE BUSINESS EVENT SERVICE` | Yes | Yes | Yes | Yes | Yes |
| REST client with no operation | `CREATE CONSUMED REST SERVICE ... ( ... ) { }` | -- | Yes | Yes | Yes | Yes |
| REST client basic authentication | `authentication: basic (...)` | -- | 10.2+ | Yes | Yes | Yes |
| REST client operations | `... { OPERATION ... }` | -- | -- | Yes | Yes | Yes |
| REST client from an OpenAPI document | `CREATE CONSUMED REST SERVICE ... ( OpenAPI: ... )` | -- | -- | -- | 10.21+ | Yes |
| REST client headers | `Headers: ('Name': 'Value')` | -- | -- | Yes | Yes | Yes |
| Database Connector | `CREATE DATABASE CONNECTION` | Yes | Yes | Yes | Yes | Yes |
| Database Connector queries | `CREATE DATABASE CONNECTION ... { QUERY ... }` | -- | -- | -- | 10.12+ | Yes |
| Scheduled events | `CREATE SCHEDULED EVENT ... ( Repeat: ... )` | Yes (9.12+) | Yes | Yes | Yes | Yes |
| Module JAR dependencies, exclusions | `ALTER MODULE ... ADD JAR DEPENDENCY`, `... ADD EXCLUSION` | Yes; 10.12+ | Yes | Yes | Yes | Yes |
| REST client query params | `QUERY $param: Type` | -- | -- | -- | -- | Yes |

A REST client operation's `body: json from $X` is stored as Mendix 11's JSON
body (`Rest$JsonBody`) on 11.0+. Mendix 10 has no JSON body, so there it is
written as the string body Mendix 10 has, holding the same text, and
`describe` reads it back as `body: template`. Before this, mxcli wrote the JSON
body on Mendix 10 too, and Studio Pro could not open the project.

### Workflows

| Feature | MDL Syntax | 9.x | 10.0+ |
|---------|-----------|-----|-------|
| Basic workflows | `CREATE WORKFLOW`, `ALTER WORKFLOW` | 9.0.2+ | Yes |
| User tasks | `USER TASK ... OUTCOMES (...)` | 9.19+ | Yes |
| A workflow parameter | `PARAMETER $WorkflowContext: Module.Entity` | 9.18+ | Yes |
| A jump | `JUMP TO ...` | 9.0.5+ | Yes |
| Calling a workflow from a workflow | `CALL WORKFLOW ...`; with `(WorkflowContext = ...)` | 9.6+; 9.18+ | Yes |
| An overview page | `OVERVIEW PAGE ...` | 9.11+ | Yes |
| Annotations | `ANNOTATION '...'` | 9.15+ | Yes |
| Multi-user tasks | `MULTI USER TASK ...` | 9.22+ | Yes |
| Multi-user task rules majority and threshold | `DECIDE BY MAJORITY ...`, `DECIDE BY THRESHOLD ...` | No | Yes |
| Waiting for all users | `AWAIT ALL USERS` | No | 10.2+ |
| Deciding by a microflow | `DECIDE BY MICROFLOW ...` | No | 10.3+ |
| Group targeting | `TARGETING GROUPS XPATH ...`, `TARGETING GROUPS MICROFLOW ...` | No | 11.2+ |
| Parallel splits | `PARALLEL SPLIT` | Yes | Yes |
| Waiting for a notification | `WAIT FOR NOTIFICATION` | No | Yes |
| Waiting for a timer | `WAIT FOR TIMER` | No | 10.6+ |
| A timer boundary event of no kind | `BOUNDARY EVENT TIMER ...` | No | 10.14+ (MDL-WF07 refuses it from 11.0) |
| Interrupting and non-interrupting timer boundary events | `BOUNDARY EVENT [NON] INTERRUPTING TIMER ...` | No | 10.20+ |
| Locking and unlocking a workflow | `LOCK WORKFLOW ...`, `UNLOCK WORKFLOW ...` | No | Yes |
| Starting a workflow, setting a task outcome, opening a user task or a workflow (microflow) | `CALL WORKFLOW`, `SET TASK OUTCOME`, `OPEN USER TASK`, `OPEN WORKFLOW` | 9.0.2+ | Yes |
| Aborting and restarting a workflow (microflow) | `WORKFLOW OPERATION ABORT ...`, `WORKFLOW OPERATION RESTART ...` | 9.2+ | Yes |
| Pausing and continuing a workflow (microflow) | `WORKFLOW OPERATION PAUSE ...`, `WORKFLOW OPERATION CONTINUE ...` | 9.3+ | Yes |
| Getting workflow data (microflow) | `GET WORKFLOW DATA ... AS ...` | 9.10+ | Yes |
| Retrying and unpausing a workflow (microflow) | `WORKFLOW OPERATION RETRY ...`, `WORKFLOW OPERATION UNPAUSE ...` | 9.12+ | Yes |
| Notifying a workflow (microflow) | `NOTIFY WORKFLOW ... TARGET ...` | No | Yes |
| Getting an object's workflows, or a workflow's activity records (microflow) | `GET WORKFLOWS FOR ...`, `GET WORKFLOW ACTIVITY RECORDS ...` | No | 10.4+ |

Every write refuses an element whose type the project's Mendix version does not
have yet, naming the type, the version that introduced it and the project's
version ("this Mendix 9.24.30 project cannot store
Workflows$WaitForNotificationActivity (Mendix 10.0.0+)"). The versions are each
type's own, from the Mendix Model SDK. Below their versions, the wait
activities, the timer boundary events, a lock or unlock naming a workflow, and
the workflow actions of a microflow (in `create microflow` and in what `alter
microflow` inserts) are also refused up front, by `check -p` and by `exec`
before anything is written, with MDL-VERSION01
([ako/mxcli#1787](https://github.com/ako/mxcli/issues/1787)). So is a workflow itself on 9.0.0 and 9.0.1, which
predate Workflows$Workflow (9.0.2): `create workflow`, `create or modify
workflow` and `alter workflow` ([ako/mxcli#1809](https://github.com/ako/mxcli/issues/1809)), and what a
workflow states whose version is later still, listed above: on a stamped 9.x
project the write refuses every workflow with a parameter below 9.18 and every
user task below 9.19 ([ako/mxcli#1811](https://github.com/ako/mxcli/issues/1811)). Before this, such a type was written, and the project did not load
(`TypeCacheUnknownTypeException`, [ako/mxcli#1774](https://github.com/ako/mxcli/issues/1774)).
Where an older version stores the same thing in another shape (the parallel
split's end marker, user tasks), that shape is written instead, as listed under
"Properties Stored in an Older Shape".

Below Mendix 11.3 a workflow's main flow has no stored start or end activity:
11.1 and 11.2 keep them implicit, and 10.x and 11.0 do not have the start
activity type at all. mxcli writes the shape the project's version stores, and
the header's `annotation '…'` is then the workflow's own note rather than the
start activity's; `describe` prints it the same way on every version. Before
this, mxcli wrote the 11.3 shape on every version: a 10.24 or 11.0 project no
longer loaded, and 11.1 and 11.2 failed `mx check` with CE6671 and CE6689
(ako/mxcli#1754).

### Navigation

| Feature | MDL Syntax | 9.x | 10.0+ |
|---------|-----------|-----|-------|
| Navigation profiles | `ALTER NAVIGATION ...` | Yes | Yes |
| Menu items | `MENU ITEM ...` | Yes | Yes |
| Home pages | `HOME PAGE Module.Page` | Yes | Yes |
| A progressive web app's settings | `PROGRESSIVE WEB APP ( ... )` | 9.0.3+ | Yes |
| A native profile's home page | `CREATE OR MODIFY NAVIGATION NativePhone HOME PAGE ...` | 9.4+ | Yes |
| Menu item icons | `MENU ITEM '...' ( ..., Icon: Module.Collection.icon )` | 9.20+ | Yes |
| A not-found page | `NOT FOUND PAGE Module.Page` | No | 10.13+ |

Each "+" version above that is not a whole major is the Model SDK's version of a
type or property mxcli writes for the feature, with no older shape; below it,
`check -p` and `exec` refuse the statement with MDL-VERSION01 before anything is
written ([ako/mxcli#1811](https://github.com/ako/mxcli/issues/1811)).

### OQL (View Entity queries)

| Feature | MDL Syntax | 10.18+ | 11.0+ |
|---------|-----------|--------|-------|
| Basic SELECT | `SELECT, FROM, WHERE` | Yes | Yes |
| Aggregate functions | `COUNT, SUM, AVG, MIN, MAX, GROUP BY` | Yes | Yes |
| String aggregate | `STRING_AGG(expr, 'separator')` | 10.24.22+ only | 11.2+ |
| Subqueries | Inline subqueries in SELECT/WHERE | Yes | Yes |
| JOIN types | `INNER/LEFT/RIGHT/FULL JOIN` | Yes | Yes |

### MPR Format & Infrastructure

| Feature | Minimum Version | Notes |
|---------|----------------|-------|
| MPR v2 (mprcontents/) | 10.18 | Per-document files for Git compatibility |
| Association storage format | 11.0 | New BSON encoding for associations |
| Portable app format | 11.6 | New deployment format |

## BSON Differences Across Versions

The Mendix metamodel evolves across versions. The reflection data shows ~42% type growth from Mendix 9.0 to 11.6. Key structural differences:

### View Entities (10.x vs 11.x)

View entities arrived in Mendix 10.18 with the query stored inline. mxcli stores it in a separate `ViewEntitySourceDocument`, which the entity's source names; both are 10.21 types, so `create view entity` needs 10.21 and is refused by `check -p` below it ([ako/mxcli#1811](https://github.com/ako/mxcli/issues/1811)); before, `check -p` passed it on 10.18 to 10.20 and the write refused it. The BSON structure also differs between 10.x and 11.x. In Mendix 10.x, the `OqlViewEntitySource` object has an `Oql` field that stores the OQL query inline (in addition to the separate `ViewEntitySourceDocument`). Mendix 11.0 removed the inline `Oql` field. The writer detects the project version and includes the inline field for 10.x projects.

### Page Parameters (9.4+, with an 11.5 tail)

Page parameters are not an 11.x feature. `Pages$PageParameter` and `Page.parameters` were introduced in **9.4.0**, and `Pages$PageSettings.parameterMappings` — the arguments a `SHOW PAGE` passes — in **9.7.0** (measured from the Mendix Model SDK's `StructureVersionInfo` records). mxcli refused them on 10.x until [mendixlabs/mxcli#1121](https://github.com/mendixlabs/mxcli/issues/1121); the 11.0 floor in the version registry came from an illustrative sample table in a proposal, not from a measurement.

What *is* version-specific is the optional/default-value half: `PageParameter.IsRequired` and `PageParameter.DefaultValue` arrived in **11.5.0**. MDL cannot express either — every parameter it writes is required with no default — so the writer omits both below 11.5. Emitting them anyway is the "never invent a key" failure: mxbuild accepts unknown properties, and Studio Pro throws `InvalidOperationException` at `MprProperty.cs`.

### Design Properties (Atlas v2 vs v3)

Atlas UI v3 (Mendix 11.0+) introduced new design properties like "Card style" and "Disable row wrap" for containers. These don't exist in the Atlas v2 theme bundled with 10.x, causing CE6083.

### Properties Stored in an Older Shape

Some properties kept their meaning and changed how they are stored. mxcli builds the current shape and, on every write, rewrites it into the shape the project's version stores. `describe` reads both shapes ([ako/mxcli#1209](https://github.com/ako/mxcli/issues/1209), [ako/mxcli#1163](https://github.com/ako/mxcli/issues/1163)). The older shapes were measured on Studio Pro-authored 9.24 documents and with `mx convert` 9.24.30:

| Property | Below | Stored as |
|---|---|---|
| A widget's design property | 10.2 | `Type` (`Toggle` / `DropDown`), `StringValue`, `BooleanValue` on the entry, instead of a typed `Value` |
| A data view's page or snippet parameter | 10.0 | `PageParameter` / `SnippetParameter` names on the data source, instead of a `SourceVariable` |
| A data view's editability | 10.7 | `Editable: true\|false`, instead of `Editability` |
| A text box's, text area's or date picker's placeholder | 10.11 | `Placeholder` text, instead of a `PlaceholderTemplate` |
| Any data source's `ForceFullObjects` | 10.8 | Not stored |
| The `PersistentId` of a workflow, activity, outcome or boundary event | 10.21 | Not stored |
| The end of a parallel split path (`Workflows$EndOfParallelSplitPathActivity`, a type introduced in 10.21) | 10.21 | Not stored: a path holds its activities and nothing more (Studio Pro 9.4) |
| A user task (`Workflows$SingleUserTaskActivity` / `MultiUserTaskActivity`, types introduced in 10.12) | 10.12 | A `Workflows$UserTask` whose `UserTaskCompletion` is a `SingleInputCompletion`, or a `MultiInputCompletion` holding the rule, the participants and, from 10.2, `AwaitAllUsers` (Studio Pro 9.24, [ako/mxcli#1771](https://github.com/ako/mxcli/issues/1771)) |
| A user task's targeting (`targeting users xpath […]`, `targeting users microflow …`, none) | 11.2 | A `UserSource` (`XPathBasedUserSource`, `MicroflowBasedUserSource`, `EmptyUserSource`) holding the same value, instead of a `UserTargeting` ([ako/mxcli#1771](https://github.com/ako/mxcli/issues/1771)) |
| A constant's value in a configuration (`alter settings constant … in configuration`) | 10.9 | A flat `Value` string on the override, instead of a `Settings$SharedValue` under `SharedOrPrivateValue` |
| A page's flow call argument that is the enclosing object (`call microflow M.F(P = $currentObject)`, `call nanoflow …`) | 10.10 | Not stored: Studio Pro infers it. A page parameter, snippet parameter or widget argument is stored as before, as a `Variable` |

What an older shape cannot hold is refused, naming the element, rather than dropped:
- a Spacing, colour-picker or toggle-button-group design property below 10.2;
- conditional data view editability below 10.7;
- a placeholder with parameters below 10.11;
- a data view fed by another widget below 10.0;
- a private constant value in a configuration below 10.9;
- a page's flow call argument that is any other expression (a literal, an attribute path) below 10.10;
- a user task targeting groups (`targeting groups xpath …`, `targeting groups microflow …`) below 11.2, where a task targets users only;
- a multi-user task below 9.22, `decide by majority` or `decide by threshold` below 10.0, `decide by microflow` below 10.3, `await all users` below 10.2, and a boundary event on a user task below 10.14. The user-task features are also refused up front, by `check -p` and by `exec` before anything is written, with MDL-VERSION01 ([ako/mxcli#1771](https://github.com/ako/mxcli/issues/1771));
- an aggregate over an expression (`sum $L of …`), `reduce`, `all` or `any` below 10.0. The plain `count`, `sum`, `average`, `minimum` and `maximum` are written on every version ([ako/mxcli#1163](https://github.com/ako/mxcli/issues/1163)).

Before this, `describe` on 9.24 printed none of these properties, so a describe → exec round trip removed them. Writing a data view on 9.24 was refused outright.

A published OData service is not an older shape of the same document. Before 10.4 it is a different document (`Rest$PublishedODataService`, not `ODataPublish$PublishedODataService2`), which mxcli does not write. `create published odata service` is refused below 10.4 with MDL-VERSION01, by `check -p` and by `exec` before anything is written. Mendix 9.24 cannot load a project that holds the 10.4 document.

### Widget Type Definitions

Pluggable widget PropertyTypes change between Mendix versions. A widget template extracted from 11.6 may have more or fewer properties than the widget installed in a 10.24 project. The [widget augmentation system](../internals/widget-templates.md) handles this by reconciling templates against the project's `.mpk` files.

The schema itself also gained keys over time. A key the project's version does not declare is left out of the schema mxcli writes, by the same version data the property floor uses. For example, a return type's `AssignableTo` arrived in 9.20, so a 9.0–9.19 schema stores `EntityProperty`, `IsList` and `Type`, as Studio Pro 9.1 and 9.4 do ([ako/mxcli#1163](https://github.com/ako/mxcli/issues/1163)).

## Widget Template Versions

Embedded widget templates are extracted from Mendix 11.6 and cover:

| Widget | Template | Widget ID |
|--------|----------|-----------|
| Combo box | combobox.json | com.mendix.widget.web.combobox.Combobox |
| Data grid 2 | datagrid.json | com.mendix.widget.web.datagrid.Datagrid |
| Gallery | gallery.json | com.mendix.widget.web.gallery.Gallery |
| Image | image.json | com.mendix.widget.web.image.Image |
| Text filter | datagrid-text-filter.json | com.mendix.widget.web.datagridtextfilter.DatagridTextFilter |
| Number filter | datagrid-number-filter.json | com.mendix.widget.web.datagridnumberfilter.DatagridNumberFilter |
| Date filter | datagrid-date-filter.json | com.mendix.widget.web.datagriddatefilter.DatagridDateFilter |
| Dropdown filter | datagrid-dropdown-filter.json | com.mendix.widget.web.datagriddropdownfilter.DatagridDropdownFilter |

### MPK Augmentation

When opening a project, mxcli checks the project's `widgets/` folder for `.mpk` packages. If a widget's installed version differs from the embedded template, the augmentation system:

1. Extracts the XML property definition from the `.mpk`
2. Adds properties found in `.mpk` but missing from the template
3. Removes properties in the template but not in the `.mpk`
4. Preserves the BSON structure (IDs, nesting, cross-references)

This reduces CE0463 ("widget definition changed") errors from widget version drift. See [Widget Templates](../internals/widget-templates.md) for details.

## Version Gates in MDL Scripts

MDL scripts can use `-- @version:` directives to conditionally execute features based on the target Mendix version. This is used in the doctype integration tests to skip features incompatible with older versions:

```mdl
-- Runs on all versions
CREATE ENTITY MyModule.Customer (Name: String(200));

-- @version: 11.0+
-- Only runs on Mendix 11.0 and later
CREATE VIEW ENTITY MyModule.ActiveCustomers (...) AS
  SELECT c.Name FROM MyModule.Customer AS c WHERE c.IsActive;

-- @version: 10.6..10.24
-- Only runs on Mendix 10.6 through 10.24
CREATE ENTITY MyModule.LegacyConfig (...);

-- @version: any
-- Resets to unconditional (runs on all versions)
CREATE ENTITY MyModule.Universal (...);
```

**Directive formats:**
- `-- @version: 11.0+` -- minimum version (run on 11.0 and later)
- `-- @version: 10.6..10.24` -- version range (inclusive)
- `-- @version: ..10.24` -- maximum version only
- `-- @version: any` -- reset to unconditional

The directive applies to all lines until the next `-- @version:` directive.

## MxBuild Compatibility

The `mx` validation tool must match the project's Mendix version:

```bash
# Auto-download the correct MxBuild version
mxcli setup mxbuild -p app.mpr

# Check the project
mxcli docker check -p app.mpr
```

MxBuild is downloaded on demand and cached in `~/.mxcli/mxbuild/{version}/`.

## Platform Support

mxcli runs on:

| Platform | Architecture |
|----------|-------------|
| Linux | amd64, arm64 |
| macOS | amd64, arm64 (Apple Silicon) |
| Windows | amd64, arm64 |

No CGO or C compiler is required -- the binary is fully statically linked using pure Go dependencies.
