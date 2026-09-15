# MDL Syntax Quick Reference

Complete syntax reference for MDL (Mendix Definition Language). This is the authoritative reference for all MDL statement syntax.

For task-specific guidance, see the skill files listed in [CLAUDE.md](../CLAUDE.md#important-before-writing-mdl-scripts-or-working-with-data).

## DESCRIBE — type is optional

Every `describe <type> Module.Name` statement also accepts a **bare** form with the type omitted — `describe Module.Name` — and the document type is auto-detected from the project (via the catalog `objects` index, built on demand). Use it anywhere: the REPL, `exec` scripts, and `mxcli describe Module.Name`.

```sql
describe Sales.Order;              -- resolves to entity / page / microflow / agent / … automatically
describe page Sales.Order;        -- the explicit form still works and is required when a name is ambiguous
```

If a name matches more than one document (e.g. an entity and a snippet share a name), the bare form reports the candidates and asks you to specify the type. The explicit form is also still required for things that have no single qualified name (module role, user role, settings, navigation).

## Entity Generalization (EXTENDS)

**CRITICAL: EXTENDS goes BEFORE the opening parenthesis, not after!**

```sql
-- Correct: EXTENDS before (
create persistent entity Module.ProductPhoto extends System.Image (
  PhotoCaption: string(200)
);

-- Wrong: EXTENDS after ) = parse error!
create persistent entity Module.Photo (
  PhotoCaption: string(200)
) extends System.Image;
```

## Modules

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show modules | `show modules;` | List all modules |
| Describe module | `describe module ModuleName;` | All contents (entities, microflows, pages, etc.) |
| Create module | `create module ModuleName;` | |
| Drop module | `drop module ModuleName;` | |
| Rename module | `rename module OldName to NewName;` | Updates all qualified name references |

## Module JAR Dependencies

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List dependencies | `list jar dependencies [in ModuleName];` | All modules or filtered by module |
| Describe dependency | `describe jar dependency ModuleName 'group:artifact';` | Full MDL output (roundtrippable) |
| Add dependency | `alter module Name add jar dependency (group = '...', artifact = '...', version = '...', included = true);` | `included` defaults to `true` |
| Update version | `alter module Name set jar dependency 'group:artifact' version '...';` | Change version string |
| Toggle inclusion | `alter module Name set jar dependency 'group:artifact' included true\|false;` | Enable/disable in classpath |
| Add exclusion | `alter module Name set jar dependency 'group:artifact' add exclusion 'group:artifact';` | Transitive exclusion |
| Drop exclusion | `alter module Name set jar dependency 'group:artifact' drop exclusion 'group:artifact';` | Remove transitive exclusion |
| Drop dependency | `alter module Name drop jar dependency 'group:artifact';` | Remove dependency entirely |

## Domain Model

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Create entity | `create [or modify] persistent\|non-persistent entity Module.Name (attrs);` | Persistent is default |
| Create with extends | `create persistent entity Module.Name extends Parent.Entity (attrs);` | EXTENDS before `(` |
| Create with auditing | `create persistent entity Module.Name (attrs, owner: autoowner, ChangedBy: autochangedby, CreatedDate: autocreateddate, ChangedDate: autochangeddate);` | Pseudo-types like AutoNumber |
| Create view entity | `create view entity Module.Name (attrs) as select ...;` | OQL-backed read-only |
| View entity clause order | `... as select … from …;` **or** `... as from … group by … select …;` | Both are Mendix OQL and both are checked. The second is what **Studio Pro stores**, so it is what `DESCRIBE ENTITY` emits — describe → edit → exec round-trips. The declared attributes are matched to the select columns **by position**, in either order |
| View entity → persistent entity | `select t.ID as MyRef, …` in the OQL | Selecting the target's **id** under an alias gives the view entity an **association** named after the alias. It is not an attribute and gets no declaration: the column *is* the declaration, so mxcli creates the member (with the `OqlViewAssociationSource` mxbuild requires — without it, CE6771 + CE6770). A plain `create association` with a view entity at either end is **refused**. The alias must be free in the module, case-insensitively. `cast(t.ID as string) as MyId` is a plain String attribute instead — one query rather than two, no objects in the client |
| Create external entity | `create external entity Module.Name from odata client Module.Client (...) (attrs);` | From consumed OData |
| Create external entities | `create [or modify] external entities from Module.Client [into module] [entities (...)];` | Bulk from $metadata |
| Drop entity | `drop entity Module.Name;` | |
| Describe entity | `describe entity Module.Name;` | Full MDL output |
| Describe enumeration | `describe enumeration Module.Name;` | Full MDL output |
| Rename entity | `rename entity Module.Old to New;` | Updates all references |
| Rename enumeration | `rename enumeration Module.Old to New;` | Updates attribute type refs |
| Rename association | `rename association Module.Old to New;` | Updates all references |
| Show entities | `show entities [in module];` | List all or filter by module |
| Create enumeration | `create [or modify] enumeration Module.Name (Value1 'caption', ...);` | |
| Alter enumeration values | `alter enumeration Module.Name add value [if not exists] X [caption '..'] \| rename value X to Y \| modify value X caption '..' \| drop value [if exists] X;` | `modify value … caption` re-captions in place (works while referenced). `if not exists` / `if exists` make the script re-runnable — the bare forms error and stop the run |
| Drop enumeration | `drop enumeration Module.Name;` | |
| Create association | `create [or modify] association Module.Name from Parent to Child type reference\|ReferenceSet [owner default\|both] [delete_behavior ...];` | OR MODIFY updates existing association in-place. **The FROM entity must live in `Module`** — Mendix stores an association in its FROM entity's module, so a remote FROM writes a dangling pointer and the project stops OPENING (**MDL070**). The TO entity may be remote; that direction is stored BY NAME |
| Drop association | `drop association Module.Name;` | |
| Association line anchors | `@anchor(from: (0, 54), to: (100, 54))` above `create association …` | Where the connector attaches to each entity box, as a **percentage** of the box (0..100, whole numbers). `from` = the FROM entity's box, `to` = the TO entity's. Omitting an end preserves what is stored, so a `create or modify` about something else never flattens a hand-tuned line. Cross-module associations have no anchors — Mendix stores none |
| Retune anchors in place | `alter association Module.Name set anchor from (50, 100) to (50, 0);` | `(0, 50)` left-middle, `(100, 50)` right-middle, `(50, 100)` bottom-centre. `describe association` re-emits a non-default pair as the same `@anchor(...)`, so describe → edit → exec round-trips |

## ALTER ENTITY

Modifies an existing entity without full replacement.

| Operation | Syntax | Notes |
|-----------|--------|-------|
| Add attribute | `alter entity Module.Name add attribute [if not exists] attr: type [constraints];` | Comma-separate the whole action to add several: `add attribute A: integer, add attribute B: string(20)`. `if not exists` skips instead of erroring, so the script re-runs |
| Drop attribute | `alter entity Module.Name drop attribute [if exists] AttrName;` | `if exists` skips when it is already gone |
| Modify attributes | `alter entity Module.Name modify (attr: NewType [constraints]);` | Change type/constraints |
| Rename attribute | `alter entity Module.Name rename attribute OldName to NewName;` | Also rewrites stored references (microflow members, page widgets, validation/access rules) and XPath constraints. Microflow expressions are free text and are **not** rewritten |
| Add index | `alter entity Module.Name add index [if not exists] [name] [on] (Col1 [asc\|desc], ...);` | `on` is optional (SQL-like). **Without `if not exists`, re-running is an error** — a second identical index fails the build with CE0072 |
| Document an association | `/** What it links. */`<br>`create association Mod.C_P from Mod.C to Mod.P;`<br>or `... to Mod.P comment 'What it links.';` | Both spellings work on create; the doc comment wins when both are present. `comment` survives here — and only here among the CREATE statements — because it is an association's **only inline** spelling |
| Create if absent | `create entity if not exists Module.Name (...);`<br>`create association if not exists Module.Assoc from ... to ...;` | Skips when it already exists, leaving the stored definition untouched. Unlike `create or modify`, which rebuilds the element from the statement and drops any attribute the statement omits |
| Add index (SQL form) | `create index IdxName on Module.Name (Col1 [asc\|desc], ...);` | Same effect as `alter entity … add index`. The index name is accepted and discarded — a Mendix index is identified by its columns |
| Drop index | `alter entity Module.Name drop index [if exists] (Col1 [asc\|desc], ...);` | Selected by its columns — a Mendix index stores no name, so the columns are its identity, and they are what `describe entity` prints. The legacy positional form `drop index idx1` still works but shifts when an earlier index is dropped |
| Add event handler | `alter entity Module.Name add event handler on before commit call Mod.MF($currentObject) [raise error];` | `($currentObject)` or `()`, RAISE ERROR only on BEFORE |
| Drop event handler | `alter entity Module.Name drop event handler on before commit;` | |
| Set documentation | `alter entity Module.Name set documentation 'text';` | |
| Set position | `alter entity Module.Name set position (100, 200);` | Canvas position |
| Add system attribute | `alter entity Module.Name add attribute owner: autoowner;` | Same syntax as regular attributes |
| Drop system attribute | `alter entity Module.Name drop attribute owner;` | Drop by system attribute name |

> **Re-running domain scripts.** `IF NOT EXISTS` / `IF EXISTS` make an individual
> create/add/drop a no-op when already applied — accepted on `create entity`,
> `create association`, `add attribute`, `add index`, `add event handler` and
> their drops. A script built from guarded statements re-runs to a byte-identical
> project.
>
> Prefer them to `CREATE OR MODIFY`, which is not the same thing: `or modify`
> rebuilds the element from the statement and **drops any attribute the statement
> omits**, so it is only safe when the statement is the element's complete
> definition. Writing both (`create or modify … if not exists`) is refused as
> **MDL067**.
>
> To re-run a whole script that is partly applied and not guarded, use
> `mxcli exec script.mdl --continue-on-error`: every statement is attempted, each
> failure is reported with its statement number, and the command exits non-zero if
> any failed (a real error is still surfaced, never masked).

**Example:**
```sql
alter entity Sales.Customer
  add attribute Phone: string(50),
  add attribute Notes: string(unlimited);

alter entity Sales.Customer
  rename attribute Phone to PhoneNumber;

alter entity Sales.Customer
  add index (Email);
```

> An `INDEX` on `create entity` goes **after** the closing parenthesis of the
> attribute list, not inside it — `create entity M.Cell (Row: Integer) index (Row);`.
> Written inside, it parses as an attribute missing its type.

## Constants

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show constants | `show constants [in module];` | List all or filter by module |
| Show constant values | `show constant values [in module];` | Compare values across configurations |
| Describe constant | `describe constant Module.Name;` | Full MDL output |
| Create constant | `create [or modify] constant Module.Name type DataType default 'value';` | String, Integer, Boolean, etc. |
| Drop constant | `drop constant Module.Name;` | |

A per-configuration override holds either a **shared** value (in the model, so in
version control) or a **private** one (on the developer's own workstation, out of the
repo). MDL preserves that choice but never changes it: `alter settings constant … value`
is refused on a private override, `show constant values` reports it as `(private)`, and
`describe settings` emits a comment rather than a re-executable statement.
`alter settings drop constant` still works.

**Example:**
```sql
create constant MyModule.ApiBaseUrl type string default 'https://api.example.com';
create constant MyModule.MaxRetries type integer default 3;
create constant MyModule.EnableLogging type boolean default true;
```

## Task Queues

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show queues | `show queues [in module];` (`list queues` too) | Parallelism + cluster-wide flag |
| Describe queue | `describe queue Module.Name;` | Re-executable MDL |
| Create queue | `create [or modify] queue Module.Name [folder 'path'] ( Parallelism: 3, ClusterWide: true );` | Body optional; defaults `1` / `false` |
| Drop queue | `drop queue Module.Name;` | |

`Parallelism` is an **expression**, not a number — Mendix stores it as a string
(`Queues$BasicQueueConfig.ParallelismExpression`). A bare integer is the common
case; quote anything else.

Binding a microflow **call** to a queue is not yet expressible in MDL. Because a
rebuild would drop an existing binding, `create or replace|modify microflow` is
**refused** when the stored microflow has a queued call — change those in Studio
Pro. (Without the refusal the binding was written back as null and `mx check`
stopped reporting CE1613, so the project looked healthy while the configuration
was gone.)

**Example:**
```sql
create queue Ops.OrderProcessing ( Parallelism: 3, ClusterWide: true );
create queue Ops.Mail;
create or modify queue Ops.OrderProcessing ( Parallelism: '$MyModule.Workers' );
drop queue Ops.Mail;
```

## Regular Expressions

Named patterns, shared by attribute validation rules.

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show regular expressions | `show regular expressions [in module];` (`list` too) | Pattern + documentation |
| Describe regular expression | `describe regular expression Module.Name;` | Re-executable MDL |
| Create regular expression | `create [or modify] regular expression Module.Name [folder 'path'] ( Expression: '<pattern>' );` | `Expression` required |
| Drop regular expression | `drop regular expression Module.Name;` | |

A regex is a **document**, not a string on a rule: Mendix stores a validation
rule's reference to it by qualified name, so one pattern is shared by every
attribute that validates against it. `show references to <regex>` lists the
entities that use it.

The pattern is an ordinary MDL string, so a single quote inside it is doubled
(`'^it''s$'`). Backslashes are **not** escape characters — write the regex
exactly as Mendix should see it.

Mendix validates with .NET's regex engine, which accepts constructs Go does not
(lookaround, backreferences). mxcli stores such a pattern unchanged; `describe`
notes that it could not verify it rather than calling it invalid.

Bind a pattern to an attribute with `create validation rule` — see below.

## Validation Rules

Constrain a single attribute. The rule is anonymous and lives on the entity, so
the statement names the **attribute**, not the rule.

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Create regex rule | `create validation rule for Module.Entity.Attribute regex Module.Pattern feedback '<msg>';` | Pattern must already exist |
| Create range rule | `create validation rule for Module.Entity.Attribute range from <lit> to <lit> feedback '<msg>';` | Bounds inclusive |
| Lower bound only | `... range from <lit> ...` | Mendix `GreaterThanOrEqualTo` |
| Upper bound only | `... range to <lit> ...` | Mendix `SmallerThanOrEqualTo` |

Mendix has **no strict `<` or `>`**, so there is no exclusive form.

Re-running a rule replaces the one of the **same type** on that attribute and
leaves the others alone — an attribute can carry a Required rule and a RegEx
rule at once.

`regex` takes the **qualified name of a regular expression document**, never an
inline pattern. A name that does not resolve is refused, because Mendix stores
the reference by name and the build would otherwise report CE0135 "No regular
expression specified".

**Required and Unique are attribute constraints, not this statement:**

```sql
create entity Shop.Product ( Email: String(200) not null error 'Required' );
alter entity Shop.Product modify attribute Code String(20) unique error 'Unique';
```

A range bounded by another *attribute* cannot be authored in MDL, but survives a
rewrite untouched; `describe entity` marks it with a comment rather than
rendering it wrong. `MaxLength` and `EqualsTo` cannot be represented at all —
mxcli refuses to rewrite an entity carrying one rather than downgrading it to a
Required rule.

**Example:**
```sql
create regular expression Val.EmailAddress (
  Expression: '\w+((-|\+|\.)\w+)*@\w+([\.-]?\w+)*(\.\w{2,})+',
  Documentation: 'A, not too restrictive, email address regular expression'
);

-- .NET lookbehind: legal in Mendix, not verifiable by mxcli
create regular expression Val.NoTrailingSlash ( Expression: '.*(?<!/)$' );

show references to Val.EmailAddress;
```

## Scheduled Events

Mendix's cron: run a microflow on a repeating schedule.

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show scheduled events | `show scheduled events [in module];` (`list` too) | Repeat, microflow, enabled |
| Describe scheduled event | `describe scheduled event Module.Name;` | Re-executable MDL |
| Create scheduled event | `create [or modify] scheduled event Module.Name [folder 'path'] ( Microflow: ..., Repeat: ..., ... );` | |
| Drop scheduled event | `drop scheduled event Module.Name;` | |

`Microflow` and `Repeat` are always required. Each repeat takes **only** its own
fields — anything else is refused by `mxcli check` (MDL-SCHED01) and by `exec`:

| Repeat | Fields |
|--------|--------|
| `Minutely` | `Multiplier` |
| `Hourly` | `Multiplier`, `MinuteOffset` |
| `Daily` | `HourOfDay`, `MinuteOfHour` |
| `Weekly` | `Weekdays`, `HourOfDay`, `MinuteOfHour` |
| `MonthlyByDate` | `Multiplier`, `MonthOffset`, `DayOfMonth`, `HourOfDay`, `MinuteOfHour` |
| `MonthlyByWeekday` | `Multiplier`, `MonthOffset`, `DaySelector`, `Weekday`, `HourOfDay`, `MinuteOfHour` |
| `YearlyByDate` | `Month`, `DayOfMonth`, `HourOfDay`, `MinuteOfHour` |
| `YearlyByWeekday` | `Month`, `DaySelector`, `Weekday`, `HourOfDay`, `MinuteOfHour` |

Optional on any repeat: `Enabled` (default false), `OnOverlap`
(`DelayNext` default / `SkipNext`), `TimeZone` (`UTC` default / `Server`),
`StartDateTime` (RFC 3339), `Documentation`.

`OnOverlap` is a scheduled event's own concurrency control — scheduled events do
**not** go through a task queue.

**Example:**
```sql
create scheduled event Ops.NightlyCleanup (
  Microflow: Ops.SE_Cleanup,
  Repeat: Daily,
  HourOfDay: 4,
  MinuteOfHour: 0,
  TimeZone: Server,
  Enabled: true
);

create scheduled event Ops.WeeklyReport (
  Microflow: Ops.SE_Report,
  Repeat: Weekly,
  Weekdays: 'Monday, Friday',
  HourOfDay: 9,
  MinuteOfHour: 30
);
```

## OData Clients, Services & External Entities

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show OData clients | `show odata clients [in module];` | Consumed OData services |
| Describe OData client | `describe odata client Module.Name;` | Full MDL output |
| Create OData client | `create [or modify] odata client Module.Name (...);` | Version, MetadataUrl, Timeout, etc. |
| Alter OData client | `alter odata client Module.Name set key = value;` | |
| Drop OData client | `drop odata client Module.Name;` | |
| Show OData services | `show odata services [in module];` | Published OData services |
| Describe OData service | `describe odata service Module.Name;` | Full MDL output |
| Create OData service | `create [or modify] odata service Module.Name (...) authentication ... { publish entity ... };` | |
| Publish as GraphQL too | `create odata service Module.Name (SupportsGraphQL: Yes) {...};` | Mendix 10.14+. Same location, clients POST a query. Exposed names must be unique beyond case (CE2881); query fields are camelCased |
| Alter OData service | `alter odata service Module.Name set key = value;` | |
| Drop OData service | `drop odata service Module.Name;` | |
| Show external entities | `show external entities [in module];` | OData-backed entities |
| Show external actions | `show external actions [in module];` | Actions used in microflows |
| Create external entity | `create [or modify] external entity Module.Name from odata client Module.Client (...) (attrs);` | |
| Create external entities | `create [or modify] external entities from Module.Client [into module] [entities (...)];` | Bulk from $metadata |
| Grant OData access | `grant access on odata service Module.Name to Module.Role, ...;` | |
| Revoke OData access | `revoke access on odata service Module.Name from Module.Role, ...;` | |
| Show contract entities | `show contract entities from Module.Client;` | Browse cached $metadata |
| Show contract actions | `show contract actions from Module.Client;` | Browse cached $metadata |
| Describe contract entity | `describe contract entity Module.Client.Entity [format mdl];` | Properties, types, keys |
| Describe contract action | `describe contract action Module.Client.Action [format mdl];` | Parameters, return type |
| Show contract channels | `show contract channels from Module.Service;` | Browse cached AsyncAPI |
| Show contract messages | `show contract messages from Module.Service;` | Browse cached AsyncAPI |
| Describe contract message | `describe contract message Module.Service.Message;` | Message payload properties |
| Query contract entities | `select * from CATALOG.CONTRACT_ENTITIES;` | Requires REFRESH CATALOG |
| Query contract actions | `select * from CATALOG.CONTRACT_ACTIONS;` | Requires REFRESH CATALOG |
| Query contract messages | `select * from CATALOG.CONTRACT_MESSAGES;` | Requires REFRESH CATALOG |

**OData Client Example:**
```sql
-- HTTP(S) URL (fetches metadata from remote service)
create odata client MyModule.ExternalAPI (
  Version: '1.0',
  ODataVersion: OData4,
  MetadataUrl: 'https://api.example.com/odata/v4/$metadata',
  timeout: 300
);

-- Local file with absolute file:// URI
CREATE ODATA CLIENT MyModule.LocalService (
  Version: '1.0',
  ODataVersion: OData4,
  MetadataUrl: 'file:///path/to/metadata.xml',
  Timeout: 300
);

-- Local file with relative path (normalized to absolute file:// in model)
CREATE ODATA CLIENT MyModule.LocalService2 (
  Version: '1.0',
  ODataVersion: OData4,
  MetadataUrl: './metadata/service.xml',
  Timeout: 300,
  ServiceUrl: '@MyModule.ServiceLocation'  -- Must be a constant reference
);
```

**Note:** `MetadataUrl` supports three formats:
- `https://...` or `http://...` — fetches from HTTP(S) endpoint
- `file:///abs/path` — reads from local absolute path
- `./path` or `path/file.xml` — reads from local relative path, **normalized to absolute `file://` in the model** for Studio Pro compatibility

**Important:** `ServiceUrl` must always be a constant reference starting with `@` (e.g., `@Module.ConstantName`). Create a constant first:
```sql
CREATE CONSTANT MyModule.ServiceLocation TYPE String DEFAULT 'https://api.example.com/odata/v4/';
```

**OData Service Example:**
```sql
create odata service MyModule.CustomerAPI (
  path: 'odata/customers/',     -- no leading slash (CE6550); trailing slash required (CE6552)
  version: '1.0.0',
  ODataVersion: OData4,
  namespace: 'MyModule.Customers'
)
authentication basic, session
-- Inside the { } body, alongside `publish entity`, a microflow can be published
-- as an OData action (an ActionImport in $metadata):
--   publish microflow Module.DoThing as 'DoThing'
--     expose ( Note as 'note', Amount as 'amount' (CanBeEmpty) );
-- Parameter types and the return type are read off the microflow, not restated.
-- or: authentication microflow Module.Authenticate
--   Custom authentication. The microflow takes a List of System.HttpHeader and
--   returns a System.User (empty denies). It removes the per-request password
--   hash that `basic` pays on every call. Requires app security on (CE6600) and
--   a microflow to be named (CE0333, flagged as MDL-ODATA04).
{
  publish entity MyModule.Customer as 'Customers' (
    ReadMode: source,
    InsertMode: source,
    UpdateMode: not_supported,
    DeleteMode: not_supported,
    UsePaging: Yes,
    PageSize: 100
  )
  -- One exposed member must be marked KEY. Use a business attribute with
  -- a UNIQUE + REQUIRED validation rule on the entity rather than the
  -- system Id (which leaks internal storage IDs). Multi-entity publish
  -- and navigation properties work the same way — bare names resolve
  -- against the module's associations.
  expose (
    Email as 'customerId' (KEY, Filterable, Sortable),
    Name (Filterable, Sortable),
    Phone,
    Customer_Order as 'Orders'   -- navigation property to associated entity
  );
};
```

The "Configuration source" dropdown on consumed OData services has three
options — Constants only, Configuration microflow, Headers microflow.
The two microflow options share a single BSON field: write either
`ConfigurationMicroflow:` or the alias `HeadersMicroflow:` in MDL.
Studio Pro picks the dropdown label from the referenced microflow's
return type (`System.ConsumedODataConfiguration` vs
`list of System.HttpHeader`).

## Domain-Model Annotations

The note boxes Studio Pro draws on the canvas. They belong to a module's domain
model, not to a document, so the statements name a module.

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show annotations | `show annotations [in Module];` | Module, Title, Position, Width, Lines. **Title** is the first line of the caption — the addressable key |
| Create | `create annotation in Module ( Caption: 'text' );` | New notes get Studio Pro's defaults: position (60, 240), width 440 |
| Create with layout | `create annotation in Module ( Caption: $$Orders\nmulti-line$$, Position: (60, 40), Width: 400 );` | Caption accepts `$$…$$` for a multi-line note; an MDL string literal is single-line |
| Update in place | `create or modify annotation in Module ( Caption: 'new wording', Position: (60, 40) );` | **Position is the identity when given** — reword and re-run without duplicating. An omitted `Width` keeps the stored one |
| Drop by title | `drop annotation 'Orders' in Module;` | Refused when two notes share a first line |
| Drop by position | `drop annotation at (60, 40) in Module;` | The unambiguous form |

**There is no colour.** A domain model holds exactly four child collections
(Annotations, Associations, CrossAssociations, Entities), and
`DomainModels$Annotation` stores only Caption, ExportLevel, Location and Width.
A "coloured section box" is this element in Studio Pro's own styling — nothing
about that styling is in the model, so nothing can author it.

**Give a note a Position if you intend to re-run the script.** Without one the
only handle is the caption's first line, so rewording creates a second note
rather than updating the first.

## Microflows, Nanoflows & Rules

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show microflows | `show microflows [in module];` | List all or filter by module |
| Show nanoflows | `show nanoflows [in module];` | List all or filter by module |
| Describe microflow | `describe microflow Module.Name;` | Full MDL with activities |
| Describe microflow (normalized) | `describe microflow Module.Name normalized;` | Folds crossed branches into one condition instead of flattening them. Opt-in: the output re-executes to an equivalent graph with fewer nodes and a different layout |
| Describe nanoflow | `describe nanoflow Module.Name;` | Full MDL with activities |
| Rename microflow | `rename microflow Module.Old to New;` | Updates all references |
| Rename nanoflow | `rename nanoflow Module.Old to New;` | Updates all references |
| Rename page | `rename page Module.Old to New;` | Updates all references |
| Rename constant | `rename constant Module.Old to New;` | Updates all references |
| Drop microflow | `drop microflow Module.Name;` | |
| Drop nanoflow | `drop nanoflow Module.Name;` | |
| Create nanoflow | `create [or modify] nanoflow Module.Name (params) returns type [folder 'path'] begin ... end;` | Same body syntax as microflows |
| Expose in the toolbox | `create microflow Module.Name () exposed as microflow action 'Caption' in 'Category' begin ... end;` | Studio Pro's "Expose as microflow action". A microflow has **two** toolbox entries, so the clause names which |
| Expose to the workflow editor | `... exposed as workflow action 'Caption' in 'Category' ...` | The second entry; both may be set on one microflow |
| Toolbox bitmaps | `... exposed as microflow action 'C' in 'Cat' icon 'i.png' image 'm.png' ...` | Icon 64x64 PNG, image 256x192; `icon dark`/`image dark` for the dark variants. Paths relative to the .mdl file's own directory |
| Remove a toolbox entry | `... not exposed as workflow action ...` | An **omitted** clause preserves what is stored — icon and image included — so removal is explicit. Nanoflows and rules refuse the clause: only a microflow stores one |
| Move nanoflow | `move nanoflow Module.Name to folder 'path';` | |
| Nanoflow restrictions | N/A | No Java actions, ErrorEvent, REST calls, database queries, external actions, download file, workflow actions, import/export mappings, JSON transformation, show home page |
| Show rules | `show rules [in module];` | `list rules` is the same statement |
| Describe rule | `describe rule Module.Name;` | Round-trippable |
| Create rule | `create [or modify] rule Module.Name (params) returns Boolean\|enum Module.Enum [folder 'path'] begin ... end;` | Same body syntax as microflows |
| Drop rule | `drop rule Module.Name;` | |
| Move rule | `move rule Module.Name to folder 'path';` | |
| Call a rule | `if Module.Rule_Name(Param = $Value) then ... end if;` | A decision is the ONLY place a rule can be called |
| Rule restrictions | N/A | Return type must be Boolean or an enumeration (mxbuild CE0103/CE0139); no create/change/delete/commit/rollback, no client interaction, no web-service calls (CE0009). There is no `grant execute on rule` — a rule stores no module-role security |

## Microflows - Supported Statements

**Semicolons are mandatory inside a microflow or nanoflow body.** Every statement ends
with `;`, including block terminators (`end if;`, `end loop;`, `end while;`, `end case;`).
Omitting one is a parse error (`missing ';' at 'return'`). The terminator on the
*definition* itself (`end;` / `end` followed by `/`) is unchanged and still optional, as
it is for pages.

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Variable declaration | `declare $Var type = value;` | Primitives: String, Integer, Boolean, Decimal, DateTime. The value is **required** — Mendix has no uninitialized variable and a bare `declare` is CE0038 / MDL061. Lists (MDL040) and objects (MDL043) cannot be declared at all |
| Entity declaration | `declare $entity Module.Entity;` | No AS keyword, no = empty |
| List declaration | `declare $list list of Module.Entity = empty;` | |
| Assignment | `set $Var = expression;` | Variable must be declared first |
| Create object | `$Var = create Module.Entity (attr = value) [commit [without events]] [refresh];` | `commit` = Commit Yes, `commit without events` = YesWithoutEvents; omitted = No (the default) |
| Change object | `change $entity (attr = value) [commit [without events]] [refresh];` | `commit` as above, before `refresh`; `refresh` updates the changed object in the client |
| Commit | `commit $entity [without events] [refresh];` | **Omitted = with events**, matching Studio Pro's default. `without events` is the deviation and the only form that changes the stored value; `with events` still parses and means the default |
| Delete | `delete $entity [refresh];` | |
| Rollback | `rollback $entity [refresh];` | Reverts uncommitted changes |
| Retrieve (DB) | `retrieve $Var from Module.Entity [where condition];` | Database XPath retrieve |
| Retrieve (Assoc) | `retrieve $list from $Parent/Module.AssocName;` | Retrieve by association |
| Add to list | `add expression to $list;` | Also accepts existing `add $item to $list;` form |
| Aggregate a list | `$Total = sum($list.Attr);` / `$Total = sum($list, expression);` | `count` (list only), `sum`, `average`, `minimum`, `maximum` — attribute or expression over `$currentObject` |
| All / any | `$AllMatch = all($list, boolean-expression);` | And `any(...)`. No seed, always Boolean — Mendix stores a Boolean return type for both |
| Reduce a list | `$Folded = reduce($list, expression, initial: value, returns: Type);` | `$currentResult` is the accumulator. `initial` and `returns` are **required** — Mendix stores both and neither is inferable, so MDL will not guess (#1004) |
| Call microflow | `$Result = call microflow Module.Name (Param = $value);` | A Mendix **expression** cannot call anything — `declare $r Boolean = Module.Name(...)` is CE0117 (MDL066) |
| Call a rule | `if Module.SomeRule (Param = $value) then ... end if;` | A decision is the **only** place a rule can be evaluated; there is no call activity for one. The name must resolve to a rule — a microflow there is CE0117 |
| Call microflow on a queue | `call microflow Module.Name (Param = $value) in queue Module.Queue;` | Background execution; the queue must exist (CE1613) |
| Call Java action on a queue | `call java action Module.Name (Param = $value) in queue Module.Queue;` | The Java action must `returns void`, else CE7038 |
| Call nanoflow | `$Result = call nanoflow Module.Name (Param = $value);` | |
| Call JS action | `$Result = call javascript action Module.Name (Param = $value);` | JavaScript action (nanoflow/microflow) |
| Call Java action | `$Result = call java action Module.Name (Param = $value);` | Java action (microflow only) |
| Call web service | `$Result = call web service Module.Service operation OperationName;` | Legacy SOAP; quoted refs are fallback for dangling raw IDs |
| Call web service (arguments) | `$Result = call web service Module.Service operation GetOrder (OrderId = $Id) receive mapping Module.IMM;` | Binds the operation's parameters, same `(Name = value)` form as every other call. mxcli builds the stored `ParameterPath` from the operation, so the script names only the parameter. Needs the consumed service present — an operation it cannot resolve is refused, not guessed. Without them an operation that takes parameters is **CE0178** |
| Call web service (send mapping) | `call web service Module.Service operation SaveOrder send mapping Module.EMM from $Order;` | Request body built by an export mapping. `from $var` is **required** — Mendix stores which object is mapped, and without it the call is **CE0369** |
| Call web service raw | `$Result = call web service raw 'base64-bson';` | Escape hatch for byte-for-byte legacy SOAP round-trip |

> **A call has ONE request body.** Arguments and a send mapping are alternatives —
> Mendix stores one `RequestBodyHandling` — so a statement asking for both is
> refused as **MDL-SOAP01** by `mxcli check` and by `exec`, which call the same
> function.
| REST call (string) | `$Var = rest call get '<url>' returns string;` | Body as string |
| REST call (response) | `$Var = rest call get '<url>' returns response;` | `System.HttpResponse` object. There is no specialization form — Mendix does not allow HttpResponse to be specialized (CE1540) |
| REST call (file document) | `$Var = rest call get '<url>' returns Module.MyFile;` | Stores the body in a file document. Must be a **specialization** of `System.FileDocument` — the base type is rejected as a return type (CE0362 / MDL064) |
| REST call (binary body) | `rest call post '<url>' header 'ContentType' = 'application/pdf' body binary $Doc/Contents returns response;` | Uploads raw bytes (`Microflows$BinaryRequestHandling`). The expression is the FileDocument's **Contents member**, not the document. A consumed REST **client document** has no binary body — `Body: FILE FROM $Doc` there is refused as MDL-REST02 |
| REST call (mapping single) | `$Var = rest call get '<url>' returns mapping Module.IMM as Module.Entity;` | Single object — Studio Pro emits `ForceSingleOccurrence=true` |
| REST call (mapping list) | `$Var = rest call get '<url>' returns mapping Module.IMM as list of Module.Entity;` | List result |
| REST call (none) | `rest call get '<url>' returns nothing;` | Discard response |
| Show page | `show page Module.PageName ($Param = $value);` | Also accepts `(Param: $value)` |
| Close page | `close page;` | |
| Download file | `download file $FileDocument [show in browser];` | Streams a `System.FileDocument` |
| Show message | `show message 'text' [type Information\|Warning\|Error] [objects [$a, $b]] [blocking];` | `blocking` halts the client until the user dismisses it — Studio Pro's checkbox. It goes after `objects` and before `on error`. Without it, a describe → exec round trip turned a blocking message into a non-blocking one (16 microflows measured) |
| Database connection credentials | `connection string @Mod.Const`, `username @Mod.Const`, `password @Mod.Const` | Constant **references** only. A literal writes an unopenable project — MDL058 |
| Synchronize (nanoflow only) | `synchronize all;` / `synchronize unsynchronized;` / `synchronize $Obj, $List;` | Offline sync. `unsynchronized` needs Mendix 9.4+. In a microflow this is MDL057 / CE0009 |
| Validation | `validation feedback $entity/attribute message 'message';` | Requires attribute path + MESSAGE |
| Log | `log info\|warning\|error [node 'name'] 'message';` | |
| Apply entity access | `@applyentityaccess` / `@applyentityaccess(false)` before `create microflow` or `create rule` | Runs the flow under the **current user's** entity access rules instead of with full access. A **security** setting and only ever narrowing, so an ABSENT annotation **preserves** what is stored rather than clearing it — the same rule as `@excluded`. Not available on a nanoflow: it runs in the client and Mendix stores no such property |
| Position | `@position(x, y)` | Canvas position (before activity) |
| Unknown annotation | — | **MDL059**. An annotation that parses and does nothing loses whatever it was meant to express, so a name the target does not read is refused — on a statement *and* before a `create`. Covers a typo (`@applyentityacces`), an annotation on a document kind that reads none (`@excluded` on a queue), and an activity annotation written at document level. The message names what that document does accept |
| Parameter position | `@position(x, y)` before a parameter, **inside** the `( … )` list | The only annotation a parameter takes. Omit it and parameters form a row at 200;53, 300;53, …; a parameter off that row is treated as hand-placed, survives a rewrite, and is emitted by DESCRIBE (#993) |
| Start event | `@start(x, y)` | Canvas position of the start, on the **first** statement. Omit it and the start is placed one spacing unit left of the first activity and MOVES with it on a rewrite; a start that is not at that derived spot is treated as hand-placed, survives a rewrite, and is emitted by DESCRIBE (#951) |
| Caption | `@caption 'text'` | Custom caption (before activity) |
| Color | `@color Green` | Background color (before activity) |
| Annotation | `@annotation 'text'` | Visual note attached to next activity. **Repeatable** — an activity can carry several, and each is its own note |
| Shared annotation | `@annotation(id: n1, text: 'note')` then `@annotation(id: n1)` | ONE note wired to several activities, which is how Mendix stores it. Without the `id:` the two lines are two separate notes, even with identical text. The id is scoped to the flow being authored and is not stored (#1077) |
| Annotation geometry | `@annotation(text: 'note', position: (x, y), size: (w, h))` | The note's own place and box on the canvas. Both are omitted whenever they match what a rewrite re-derives — 100px above the activity, stacked 60px per extra note, at 200×50 — so an ordinary note stays on the short form |
| Free annotation | `@annotation 'text'` before `@position(...)` | Free-floating visual note preserved by order. A free note has no activity to be placed relative to, so DESCRIBE always emits its `position:` |
| IF | `if condition then ... [else ...] end if;` | |
| Enum split | `case $Var when Value then ... end case;` | Enumeration decision branches. Bare enum values (never quoted or qualified), one branch per value **including `(empty)`** (MDL056), no `else` (MDL008), no `AS` alias |
| Type split | `split type $Var when Module.Entity then ... when (empty) then ... end split;` | Runtime specialization branches. Same `when ... then` shape as the enum split. Needs a branch per subtype **and** the base entity (CE0090); `when (empty) then` is the **null-object** flow, not a default, and cannot be omitted (CE0089). Legacy `case Module.Entity` / `else` still parse (MDL065 warns) |
| Cast | `cast $SpecificVar;` | Downcast inside a type split branch |
| LOOP | `loop $item in $list begin ... end loop;` | FOR EACH over list. No `return` inside — an End event cannot sit in a loop (CE0068 / MDL062); use `break` and return after the loop |
| WHILE | `while condition begin ... end while;` | Condition-based loop |
| Return | `return $value;` | Required at end of every flow path, and never inside a loop (MDL062) |
| List range (paging) | `$Page = range($List, <offset>, <amount>);` | **Offset first, then amount.** `range($L, 0, $N)` = first N; `range($L, $Off)` = skip and take the rest. At least one bound is required — a bare `range($L)` is CE6520 / MDL068. Stored as a `CustomRange` nested inside the `ListRange` |
| Execute DB query | `$Result = execute database query Module.Conn.Query;` | 3-part name; supports DYNAMIC, params, CONNECTION override |
| Import mapping | `[$Var =] import from mapping Module.IMM($SourceVar) [all\|first\|limit <e> [offset <e>]];` | Apply import mapping to string variable. Trailing clause is Studio Pro's Range; omitted = infer from the mapping's root. `first` binds one OBJECT (`limit 1` is a one-element LIST). Mendix rejects `offset` on a non-list mapping (CE6100) |
| Export mapping | `$Var = export to mapping Module.EMM($EntityVar);` | Apply export mapping to entity, returns string |
| Error handling | `... on error continue\|rollback\|{ handler }\|without rollback { handler };` | Goes on the activity that may fail — including `declare`, `set`, `change`, `log`, `show page`, `close page`, `show message` and `validation feedback`, which gained it in mendixlabs/mxcli#1078 so a Studio Pro handler survives DESCRIBE. `on error continue` is refused (MDL076) where Mendix raises CE6035: create, change, commit, log, show page, close page, show message, validation feedback — a custom `{ handler }` is accepted on all of them. The list-operation and aggregate forms of `set` have no error handling at all (MDL077). Not supported on EXECUTE DATABASE QUERY. **In a nanoflow** only `declare` and `set` take a clause at all — `change`, `log`, `show page`, `close page`, `show message` and `validation feedback` are CE6035 there in every form, and are refused. A handler that does not end in `return`/`throw` merges back into the main flow, so a later variable is out of scope on the error path (CE0108) |
| Named join point | `merge <label>;` / `join <label>;` | Declares an ExclusiveMerge and sends a path to it. The label is MDL-only — a Mendix merge stores no name, so it is resolved at build and at describe time and never written to the model. Forward and backward references both resolve, so `merge attempt; … on error { join attempt; }` is a retry loop. This is how an **error path that rejoins the normal one** is written: without it the only spellings are "terminate" and "fall through to the enclosing branch's continuation", and DESCRIBE emitted an empty `{ }` for anything else — MDL that re-executes to a different graph with nothing reporting it. Also covers **crossed branches**, where an inner split's branch lands where an outer split's branch lands. Refused inside a `loop`/`while` body (MDL-FLOW04): a LoopedActivity owns its own object collection and a sequence flow cannot leave it. An unresolved or unjoined label is MDL-FLOW02; a duplicate declaration MDL-FLOW03. A path that already ended does not fall through into a following `merge` |

**Activity defaults.** An omitted modifier always means Mendix's own default, so a
bare MDL statement produces the same activity as dragging a fresh one onto the
canvas in Studio Pro:

| Activity | Commit | With events | Refresh in client |
|----------|--------|-------------|-------------------|
| `create` | No | — | No |
| `change` | No | — | No |
| `commit` | — | **Yes** | No |
| `delete` | — | — | No |
| `rollback` | — | — | No |

`commit` is the only one whose default is ON, which is why it is the only one
with a `without` form. Before #895 mxcli wrote it OFF: the commit event handlers
silently did not run, and no tool reported it — a commit that skips its handlers
is a valid model, so `mxcli check`, `mxcli lint`, Studio Pro's consistency check
and `mxbuild` were all clean. Only the running app showed it.

## Microflows - NOT Supported (Will Cause Parse Errors)

| Unsupported | Use Instead | Notes |
|-------------|-------------|-------|
| `case ... when 'String' ... else ...` | Bare enum values, one branch per value | `case` itself IS supported for **enum splits** (see above); what fails is quoted/qualified values, an `else` branch, and an `AS` alias |
| `TRY ... CATCH ... end TRY` | `on error { ... }` blocks | Use error handlers on specific activities |

**Notes:**
- `retrieve ... limit n` IS supported. `limit 1` returns a single entity, otherwise returns a list.
- `rollback $entity [refresh];` IS supported. Rolls back uncommitted changes to an object.

## Project Organization

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List folders | `list folders [in module];` | The folder layout, with the documents in each folder |
| Microflow folder | `folder 'path'` (before BEGIN) | `create microflow ... folder 'ACT' begin ... end;` |
| Page folder | `folder: 'path'` (in properties) | `create page ... (folder: 'pages/Detail') { ... }` |
| Drop folder | `drop folder 'path' in module;` | Folder must be empty |
| Move folder | `move folder Module.FolderName to folder 'path';` | Target folders auto-created |
| Move to folder | `move <doctype> Module.Name to folder 'path';` | Folders created automatically. Any top-level doctype, spelled as `describe` spells it |
| Move a mapping / structure | `move import mapping\|export mapping\|json structure Module.Name to folder 'path';` | |
| Place while creating | `create <doctype> Module.Name folder 'path' ...` | Every doctype. Pages/snippets use `folder: 'path'` as a property; microflows/nanoflows a keyword before `begin` |
| Place an existing document | `create or modify ... folder 'path' ...` | Moves it; omitting the clause leaves placement alone |
| Move to module root | `move page Module.Name to module;` | Removes from folder |
| Move across modules | `move page Old.Name to NewModule;` | **Breaks by-name references** — use `show impact of` first |
| Move to folder in other module | `move page Old.Name to folder 'path' in NewModule;` | |
| Move entity to module | `move entity Old.Name to NewModule;` | Entities don't support folders |

Nested folders use `/` separator: `'Parent/Child/Grandchild'`. Missing folders are auto-created.

## Security Management

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show project security | `show project security;` | Displays security level, admin, demo users |
| Show module roles | `show module roles [in module];` | All roles or filtered by module |
| Show user roles | `show user roles;` | Project-level user roles |
| Show demo users | `show demo users;` | Configured demo users |
| Show access on element | `show access on microflow\|nanoflow\|page\|entity Mod.Name;` | Which roles can access |
| Show security matrix | `show security matrix [in module];` | Full access overview |
| Create module role | `create [or modify] module role Mod.Role [description 'text'];` | `or modify` updates an existing role instead of failing, so a security script can be re-run |
| Drop module role | `drop module role Mod.Role;` | |
| Create user role | `create user role Name (Mod.Role, ...) [manage all roles];` | Aggregates module roles |
| Alter user role | `alter user role Name add\|remove module roles (Mod.Role, ...);` | |
| Drop user role | `drop user role [if exists] Name;` | `if exists` makes a cleanup script re-runnable |
| Grant microflow access | `grant execute on microflow Mod.MF to Mod.Role, ...;` | |
| Revoke microflow access | `revoke execute on microflow Mod.MF from Mod.Role, ...;` | |
| Grant nanoflow access | `grant execute on nanoflow Mod.NF to Mod.Role, ...;` | |
| Revoke nanoflow access | `revoke execute on nanoflow Mod.NF from Mod.Role, ...;` | |
| Grant page access | `grant view on page Mod.Page to Mod.Role, ...;` | |
| Revoke page access | `revoke view on page Mod.Page from Mod.Role, ...;` | |
| Grant entity access | `grant Mod.Role on Mod.Entity (create, delete, read *, write *);` | Additive — merges with existing. A module role must be qualified: a bare `Role` parses but is refused (MDL-GRANT02). Inherited members are named like the entity's own (`read *` covers them); an unknown name is an error. Entities extending `System.User` are the exception — their platform members must not be granted |
| Access for members added later | — | A rule's default for new members is derived from the grant: `write *` → ReadWrite, `read *` → ReadOnly, member lists alone → **None**. So an attribute added later is granted None on a member-listed rule — clean build, blank field. `alter entity … add attribute` warns and prints the widening grant. The rule's *default* decides this, not how narrow its member list is |
| Revoke entity access | `revoke Mod.Role on Mod.Entity;` | Full revoke — removes entire rule |
| Revoke entity access (partial) | `revoke Mod.Role on Mod.Entity (read (attr));` | Partial — downgrades specific rights |
| Set security level | `alter project security level off\|prototype\|production;` | |
| Toggle demo users | `alter project security demo users on\|off;` | |
| Enable guest access | `alter project security guest access on role UserRole;` | Anonymous users. The role is what visitors get — its entity access is the public surface. Mendix fails the build without one (CE0133), so `on` is refused unless a role is given or already stored. mxcli validates the role exists; Mendix does not |
| Disable guest access | `alter project security guest access off;` | Keeps the stored role, so re-enabling needs no `role` clause |
| Create demo user | `create demo user 'name' password 'pass' [entity Module.Entity] (UserRole, ...);` | |
| Drop demo user | `drop demo user [if exists] 'name';` | `if exists` makes a cleanup script re-runnable |
| Update security | `update security [[in] Module];` | Re-syncs access rules with their domain model — Studio Pro's **Update security** button, headless. Repairs **CE0066** "Entity access is out of date", which a model authored elsewhere can carry (a module imported or updated outside Studio Pro). Not needed after mxcli's own writes: every write path reconciles as it writes. Writes nothing when the rules already match, and skips `System` |

## Workflows

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show workflows | `show workflows [in module];` | List all or filter by module |
| Describe workflow | `describe workflow Module.Name;` | Full MDL output |
| Create workflow | `create [or modify] workflow Module.Name [folder 'path'] parameter $Ctx: Module.Entity [on workflow events (<type>, ...) microflow Mod.MF [as '<text>']] [on any workflow event microflow Mod.MF [as '<text>']] begin ... end workflow;` | See activity types and event handlers below |
| Drop workflow | `drop workflow Module.Name;` | |

**Workflow Activity Types:**
- `[multi] user task <name> '<caption>' [page Mod.Page] [targeting [users|groups] microflow Mod.MF] [targeting [users|groups] xpath '<expr>'] [on created microflow Mod.MF] [participants all|<n>|<n> percent] [decide by <rule>] [await all users] [outcomes '<out>' { } ...];`
  - **Multi-user only:** `decide by consensus|majority more than half|majority most chosen|threshold <n> percent|votes fallback '<outcome>'`, `decide by veto '<outcome>'`, `decide by microflow Mod.MF`. A fallback is required for consensus, majority and threshold (CE1866), a veto needs its outcome (CE1867), and a decision microflow returns String (CE5012) — all `MDL-WF13` / check. Omitted: all participants, consensus on the first outcome, not waiting.
  - The **task page** must take a `System.WorkflowUserTask` parameter — none at all is CE7410, none of that type is CE7412; extra parameters are allowed.
  - A **targeting microflow** takes exactly `System.Workflow` + the context entity (or a generalization of it), in either order — anything else is CE6677. Users targeting returns a list of `System.User`, groups a list of `System.WorkflowGroup`.
  - An **on-created microflow** takes exactly `System.WorkflowUserTask` + the context entity, in either order (CE6683), and returns nothing (CE5012).
  - `check --references` reports these before anything is written; `exec` refuses the workflow statement itself (Mendix 11+).
- `call microflow Mod.MF [as <name>] [comment '<text>'] [with (<Param> = '<expr>', ...)] [outcomes '<out>' -> { } ...];`
- `call agent microflow Mod.MF [as <name>] [comment '<text>'] [with (<Param> = '<expr>', ...)] [outcomes … -> { } ...];` — an **AI agent task** (Mendix 11.9+): the call-microflow statement stored as `Workflows$AIAgentTaskActivity`. Its microflow must take at least one parameter (CE1590).
- `call workflow Mod.WF [as <name>] [comment '<text>'] [with (<Param> = '<expr>', ...)];`
- `decision [<name>] ['<expression>'] outcomes <true|false|'Module.Enum.Value'> -> { } ...;`
- `parallel split [<name>] path 1 { } path 2 { };`
- `jump to <activity-name>;`
- `wait for timer [<name>] ['<expr>'];`
- `wait for notification [<name>];`
- `notification [<name>] [comment '<caption>'];` — an intermediate notification event (Mendix 11.11+)
- `end workflow [comment '<caption>'];` — only inside a `{ }` block; ends the whole workflow
- Boundary events, after `outcomes`: `boundary event [non] interrupting timer '<expr>' { … }` or `boundary event [non] interrupting notification <name> ['<caption>'] { … }` (11.11+). One interrupting event per activity (CE6697, MDL-WF15).

**Notifying a workflow** (a microflow statement): `[$Notified =] notify workflow $Workflow target Module.Workflow.ElementName;` — the element is a notification-started event sub-process's start, a notification activity, a notification boundary event or a wait for notification, and mxcli resolves which. The target is required (CE0166, MDL-WF16).

**Event sub-processes**, after the main body: `event subprocess <name> ['<caption>'] on [non] interrupting notification [<start>] ['<caption>'] { … };` (11.8+) or `… on [non] interrupting timer '<first-execution-time>' [as <start>] [comment '<caption>'] { … };` (11.13+). The body's End is implicit; a `jump to` stays in its own sub-process (CE6682, MDL-WF05); a timer needs its expression (CE0126, MDL-WF14).

**Workflow event handlers.** `on workflow events (UserTaskStarted, UserTaskEnded)
microflow Mod.MF as 'Task audit'` in the header runs the microflow for each listed
event; the microflow takes exactly `System.WorkflowEvent`, `System.WorkflowRecord`
and `System.WorkflowActivityRecord` (CE6691). The build does not check event type
names — an invented one builds and never fires — so mxcli refuses an unknown name
(MDL-WF12) and one the project's Mendix version lacks. `on any workflow event`
stores every type the version has (Studio Pro stores the list, not a flag) and
needs 11.6+. Types: `mxcli syntax workflow.event-handlers`.

**Ending a workflow early.** `end workflow` inside an outcome, a decision branch, a
call-microflow outcome or an interrupting boundary-event path ends the whole
workflow — the workflow counterpart of a microflow's `return` (which a workflow
refuses, MDL-WF11). It must be the last statement of its block (MDL-WF09, CE6671),
is refused under a parallel split or a non-interrupting boundary path (MDL-WF08,
CE1844), and when every path of an activity ends — in `end workflow` or `jump to` —
nothing may follow it, including the end of the main flow (MDL-WF10, CE6689). The
main flow needs none: the body's closing `end workflow` is its End.

**Activity names.** Every activity has a name, and `jump to` resolves against it
— Mendix stores `JumpToActivity.TargetActivity` as a name string, not a pointer.
Without an explicit name mxcli derives one (from the caption, or from the called
document for `call microflow` / `call workflow`), which is fine for a workflow
written from scratch. Name activities explicitly when a `jump to` targets them,
and when reproducing a workflow Studio Pro authored: Studio Pro names activities
by type and ordinal (`decision1`, `split1`, `callMicroflow1`) regardless of
caption, so `describe workflow` emits the name whenever it is not derivable.

**Decision outcomes** are `true` / `false` for a boolean decision, and a **fully
qualified** enumeration value identifier — `Module.Enumeration.Value` — for an
enum decision, plus one `'' -> { }` outcome for "none of the above" (without it
the build fails `CE6686`). Anything shorter is refused as `MDL-WF03`, and by
`exec`: Mendix parses the value when the project is **loaded**, so a bare
`'Approved'` — or `'Status.Approved'`, even when the enumeration is in the same
module — is not a build error but a `StorageLoadException` that leaves the
project unopenable in Studio Pro and mxbuild.

**Parameter values in `with (...)` are quoted strings**, not bare variables:
`call microflow Mod.MF with (Request = '$WorkflowContext')`.

**An enumeration decision also needs an empty outcome.** Mendix generates one
outcome per enumeration value **plus one for the empty value**, and MxBuild
compares the stored set against that: anything else is CE6686 ("Regenerate the
outcomes"). Write it as `'' -> { }` alongside the named values — `check` reports
a missing one as `MDL-WF06`. It applies to `call microflow` outcomes branching on
an enumeration return as well, and a required (`not null`) attribute does **not**
exempt it. Boolean decisions (`true`/`false`) do not take one.

```sql
  decision '$WorkflowContext/Kind'
    outcomes
      'Module.Kind.Standard' -> { }
      'Module.Kind.Priority' -> { }
      '' -> { }
  ;
```

**Example:**
```sql
create workflow Module.ApprovalFlow
  parameter $context: Module.Request
  overview page Module.WorkflowOverview
begin
  user task ReviewTask 'Review the request'
    page Module.ReviewPage
    outcomes 'Approve' { } 'Reject' { };
end workflow;
```

## ALTER WORKFLOW

Modify an existing workflow's properties, activities, outcomes, paths, conditions, and boundary events without full replacement.

| Operation | Syntax | Notes |
|-----------|--------|-------|
| Set display name | `set display 'name'` | Workflow-level display name |
| Set description | `set description 'text'` | Workflow-level description |
| Set export level | `set export level api\|Hidden` | Visibility level |
| Set due date | `set due date 'expr'` | Workflow-level due date expression |
| Set overview page | `set overview page Module.Page` | Workflow overview page |
| Set parameter | `set parameter $Var: Module.Entity` | Workflow context parameter |
| Set activity page | `set activity name page Module.Page` | Change user task page |
| Set activity description | `set activity name description 'text'` | Activity description |
| Set activity targeting | `set activity name targeting [users\|groups] microflow Module.MF` | Target user/group assignment |
| Set activity XPath | `set activity name targeting [users\|groups] xpath '[expr]'` | XPath targeting |
| Set activity due date | `set activity name due date 'expr'` | Activity-level due date |
| Insert activity | `insert after name call microflow Module.MF` | Insert after named activity |
| Drop activity | `drop activity name` | Remove activity by name |
| Replace activity | `replace activity name with activity` | Replace activity in-place |
| Insert outcome | `insert outcome 'name' on activity { body }` | Add outcome to user task/decision |
| Drop outcome | `drop outcome 'name' on activity` | Remove outcome |
| Insert path | `insert path on activity { body }` | Add path to parallel split |
| Drop path | `drop path 'name' on activity` | Remove parallel split path |
| Insert condition | `insert condition 'name' on activity { body }` | Add decision branch |
| Drop condition | `drop condition 'name' on activity` | Remove decision branch |
| Insert boundary event | `insert boundary event on activity interrupting timer ['expr'] { body }` | Add boundary timer |
| Drop boundary event | `drop boundary event on activity` | Remove boundary event |

**Activity references** can be identifiers (`ReviewOrder`) or string literals (`'Review the order'`). Use `@N` suffix for positional disambiguation when multiple activities share a name (e.g., `ACT_Process@2`).

**Multiple actions** can be combined in a single ALTER statement.

**Example:**
```sql
-- Set workflow-level properties
alter workflow Module.OrderApproval
  set display 'Updated Order Approval'
  set description 'Updated description';

-- Modify an activity
alter workflow Module.OrderApproval
  set activity ReviewOrder page Module.AlternatePage;

-- Insert and drop activities
alter workflow Module.OrderApproval
  insert after ReviewOrder call microflow Module.ACT_Escalate;
alter workflow Module.OrderApproval
  drop activity ACT_Notify@1;

-- Manage outcomes on a user task
alter workflow Module.OrderApproval
  insert outcome 'Escalate' on ReviewOrder {
    call microflow Module.ACT_Review;
  };
alter workflow Module.OrderApproval
  drop outcome 'Hold' on ReviewOrder;

-- Boundary events
alter workflow Module.OrderApproval
  insert boundary event on ReviewOrder interrupting timer 'addHours([%CurrentDateTime%], 2)' {
    call microflow Module.ACT_BoundaryHandler;
    jump to ReviewOrder;
  };
alter workflow Module.OrderApproval
  drop boundary event on ReviewOrder;
```

**Tip:** Run `describe workflow Module.Name` first to see activity names.

## Project Structure

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Structure overview | `show structure;` | Depth 2 (elements with signatures), user modules only |
| Module counts | `show structure depth 1;` | One line per module with element counts |
| Full types | `show structure depth 3;` | Typed attributes, named parameters |
| Filter by module | `show structure in ModuleName;` | Single module only |
| Include all modules | `show structure depth 1 all;` | Include system/marketplace modules |
| Folder layout | `list folders [in module];` | `show structure` is by document type at every depth and never shows folders — use this to read back where a `move` put something |

## Navigation

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show navigation | `show navigation;` | Summary of all profiles |
| Show menu tree | `show navigation menu [Profile];` | Menu tree for profile or all |
| Show home pages | `show navigation homes;` | Home page assignments across profiles |
| Describe navigation | `describe navigation [Profile];` | Full MDL output (round-trippable) |
| Create/replace navigation | `create or replace navigation Profile ...;` | Full replacement — and **creates** the profile if the project does not have it |
| Offline sync | `sync ( sync Mod.Entity all; ... )` | A clause of CREATE NAVIGATION. Modes: `online`, `all`, `where '<xpath>'`, `never`, `none`, `none preserve data`. **Not** Studio Pro's captions — its "All Objects" is `all`, its "By XPath" is `where`. An offline profile downloads nothing without this |
| Profile kinds | `Responsive` · `Phone` · `Tablet` · `ResponsiveOffline` · `PhoneOffline` · `TabletOffline` | A closed set. An invented name (`Mobile`) is an error, not a new profile: the runtime routes on User-Agent to Mendix's own kinds. Native profiles are a different document type and are not creatable |
| Offline profiles | `create or replace navigation TabletOffline ...;` | Same properties as the online twin, but every page the profile can reach may bind an attribute across **at most one** association hop (**CE6206**). Creating one reports the documents that already exceed that |

**Navigation Example:**
```sql
create or replace navigation Responsive
  home page MyModule.Home_Web
  home page MyModule.AdminHome for Administrator
  login page Administration.Login
  not found page MyModule.Custom404
  menu (
    menu item 'Home' page MyModule.Home_Web icon Atlas_Core.Atlas.home;
    menu 'Admin' icon Atlas_Core.Atlas."align-center" (
      menu item 'Users' page Administration.Account_Overview;
    );
  );
```

**An item with no icon is reported (MDL077, a warning).** The navigation sidebar
collapses to an icon rail, and that is the state most users leave it in: a
collapsed item shows its icon, and one without falls back to the first few
characters of its caption — rarely enough to tell `Orders` from `Order lines`.
The menu still builds and `mx check` passes, so the only symptom is in a browser.
The rule covers every item at every depth, in both `create navigation`'s `menu`
block and `create menu`, and needs no project.

`icon` is optional and is a **qualified name** into an **icon collection** —
`Atlas_Core.Atlas`, `Atlas_Core.Atlas_Filled`, `Atlas_Core.Atlas_Styling`, or one
of your own — written like any other model reference. Hyphenated Atlas names
(`align-center`) are double-quoted, the same way a keyword-colliding name is:
`Atlas_Core.Atlas."align-center"`. List the available names with `describe icon
collection Atlas_Core.Atlas`.

Mendix stores **three different icon elements**, and each has its own form,
because they are not spellings of one value — a collection icon and an image icon
each hold a qualified name (into an icon collection and an *image* collection,
different documents), while a glyph icon holds a numeric character code and no
name at all:

| form | element | holds |
|------|---------|-------|
| `icon Atlas_Core.Atlas.home` | `Forms$IconCollectionIcon` | a name in an icon collection |
| `icon glyph 57377` | `Forms$GlyphIcon` | a numeric character code |
| `icon image MyModule.Images.logo` | `Forms$ImageIcon` | a name in an image collection |

**Browse the glyph codes with `show glyphs`.** A glyph is a character code in a
font, not a document in the project, so there is nothing to scope with `IN` and
no connection is needed:

```sql
show glyphs;                  -- all 247, with names
show glyphs like 'star';      -- 57350 star, 57351 star-empty
describe glyph 57350;         -- by code
describe glyph 'star';        -- or by name
```

**A glyph code the font does not define is reported (MDL078, a warning).** A
glyph code is a bare integer, so nothing resolves it: `mxcli check` and `mx check`
both pass at 0 errors and the failure lands at `mxbuild --target=deploy`, as
*"An exception occurred while exporting layout '<some layout>'"* — naming a
document that is not the cause. Measured on 11.14.0: mxbuild resolves the code
through a LINQ `.First(...)` in `GlyphFont.GetClass`, which throws on an absent
one. The rule checks the 247 codes the shipped font actually defines. Prefer an
icon collection reference, which `check --references` resolves before anything is
written.

The bare form is the icon-collection icon, so every existing script keeps its
meaning. The keyword forms exist because writing a bare name for an image icon
would rebuild it as a collection icon — a silent variant swap.

`describe navigation` emits all three, so describe → exec is lossless. It
previously wrote a comment for the other two, and since `create or replace
navigation` is a **full replacement**, re-running that output DELETED the icon
the comment had just declined to describe. A `$Type` this build does not know is
still flagged rather than guessed at.

## Project Settings

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show settings | `show settings;` | Overview of all settings parts |
| Describe settings | `describe settings;` | Full MDL output (round-trippable) |
| Alter model settings | `alter settings model key = value;` | AfterStartupMicroflow, HashAlgorithm, JavaVersion, etc. |
| Alter configuration | `alter settings configuration 'Name' key = value;` | DatabaseType, DatabaseUrl, HttpPortNumber, etc. |
| Alter constant | `alter settings constant 'Name' value 'val' in configuration 'cfg';` | Override constant per configuration |
| Drop constant override | `alter settings drop constant 'Name' in configuration 'cfg';` | Reset to default value |
| Create or modify configuration | `create or modify configuration 'Name' [key = value, ...];` | Upsert — what `describe settings` emits, so a described project replays onto a target that already has `Default` |
| Create configuration | `create configuration 'Name' [key = value, ...];` | New server configuration. `DatabaseType` must be `Db2`, `Hsqldb`, `MySql`, `Oracle`, `PostgreSql`, `SapHana` or `SqlServer` (case-insensitive) |
| Drop configuration | `drop configuration 'Name';` | Remove a configuration |
| Alter language | `alter settings LANGUAGE key = value;` | DefaultLanguageCode (must already be enabled). Set it **before** creating pages — it decides what language their captions are stored in |
| Enable a language | `alter settings LANGUAGE add 'de_DE' [(CheckCompleteness: true, CustomDateFormat: 'yyyy-MM-dd')];` | Adds to the enabled list — the only languages a build emits translations for. A language is identified by its code; Studio Pro's "German, Germany" is derived for display and not stored |
| Enable or modify (upsert) | `alter settings LANGUAGE add or modify 'de_DE' (CheckCompleteness: true);` | What `describe settings` emits, so a described project replays onto itself or onto one that already has the language |
| Modify a language | `alter settings LANGUAGE modify 'de_DE' (CheckCompleteness: true);` | Changes only the options it names. `CheckCompleteness` turns on error reporting for texts with no translation in that language (the default language is always checked regardless) |
| Disable a language | `alter settings LANGUAGE remove 'de_DE';` | The **default** language is refused (every missing translation falls back on it). Translations are NOT deleted — they stay in the model and stop being built; the run reports how many |
| Alter workflows | `alter settings workflows key = value;` | UserEntity, DefaultTaskParallelism |
| List languages | `show languages;` | ⚠️ languages that have TRANSLATIONS, not enabled ones (a stock app reports 8 while 1 is enabled). For the enabled list use `describe settings`. Requires `refresh catalog full` |

## Business Events

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show services | `show business events;` | List all business event services |
| Show in module | `show business events in module;` | Filter by module |
| Describe service | `describe business event service Module.Name;` | Full MDL output |
| Create service | `create business event service Module.Name (...) { message ... };` | See help topic for full syntax |
| Create or modify | `create or modify business event service Module.Name (...) { ... };` | Preserves UUID — preferred for AI agents |
| Drop service | `drop business event service Module.Name;` | Delete a service |

## Agents

AI agent document types (Model, Knowledge Base, Consumed MCP Service, Agent) require
the `AgentEditorCommons` marketplace module and Mendix 11.9+.

**Model**

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List models | `list models [in module];` | Also `show models` |
| Describe model | `describe model Module.Name;` | Full MDL output |
| Create model | `create [or modify] model Module.Name (Provider: MxCloudGenAI, key: Module.Const);` | OR MODIFY updates existing model, preserves UUID |
| Drop model | `drop model Module.Name;` | |

**Knowledge Base**

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List knowledge bases | `list knowledge bases [in module];` | Also `show knowledge bases` |
| Describe knowledge base | `describe knowledge base Module.Name;` | Full MDL output |
| Create knowledge base | `create [or modify] knowledge base Module.Name (Provider: MxCloudGenAI, key: Module.Const);` | OR MODIFY updates existing KB, preserves UUID |
| Drop knowledge base | `drop knowledge base Module.Name;` | |

**Consumed MCP Service**

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List MCP services | `list consumed mcp services [in module];` | Also `show consumed mcp services` |
| Describe MCP service | `describe consumed mcp service Module.Name;` | Full MDL output |
| Create MCP service | `create [or modify] consumed mcp service Module.Name (ProtocolVersion: v2025_03_26, version: '1.0', ConnectionTimeoutSeconds: 30, documentation: 'text');` | OR MODIFY updates existing service, preserves UUID |
| Drop MCP service | `drop consumed mcp service Module.Name;` | |

**Agent**

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List agents | `list agents [in module];` | Also `show agents` |
| Describe agent | `describe agent Module.Name;` | Full MDL output, re-executable |
| Create agent | See example below | Requires a Model document |
| Create or modify | `create or modify agent Module.Name (...) { ... };` | Updates existing agent, preserves UUID |
| Drop agent | `drop agent Module.Name;` | Drop agents before their Model/KB/MCP dependencies |

```sql
create agent Module.MyAgent (
  UsageType: task,
  model: Module.MyModel,
  MaxTokens: 4096,
  Temperature: 0.7,
  TopP: 0.9,
  ToolChoice: Auto,
  description: 'Agent description',
  variables: ("Language": EntityAttribute),
  SystemPrompt: $$You are a helpful assistant.
Respond in {{Language}}.$$,
  UserPrompt: 'Ask me anything.'
)
{
  mcp service Module.WebSearch {
    Enabled: true
  }

  knowledge base KBAlias {
    source: Module.ProductDocs,
    collection: 'product-docs',
    MaxResults: 5,
    description: 'Product documentation',
    Enabled: true
  }

  tool MyMicroflowTool {
    description: 'Fetch customer data',
    Enabled: true
  }
};
```

**Notes:**
- `variables: ("key": EntityAttribute)` binds entity attributes; `("key": string)` binds a plain string.
- Use `$$...$$` dollar-quoting for multi-line SystemPrompt/UserPrompt values.
- Drop agents before dropping their referenced Model, Knowledge Base, or MCP Service.
- Portal-populated metadata fields (`DisplayName`, `KeyName`, `KeyID`, `Environment`, `ResourceName`, `DeepLinkURL`) are managed by the portal and should not be set manually.

## Image Collections

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show collections | `show image collection [in module];` | List all or filter by module |
| Describe collection | `describe image collection Module.Name;` | Full MDL output with embedded images |
| Create collection | `create image collection Module.Name [folder 'path'] [export level 'Hidden'\|'Public'] [comment 'text'] [(image Name from file 'path', ...)];` | With or without images |
| Create or modify | `create or modify image collection Module.Name [...];` | Preserves UUID — preferred for AI agents |
| Drop collection | `drop image collection Module.Name;` | Removes collection and all embedded images |
| Show an image on a page | `image imgLogo (Image: 'Module.Collection.ImageName');` | Three-part name, like an icon reference. `describe image collection` lists the names |

An `image` widget's default source **is** an image collection entry, so a bare
`image imgLogo (...)` with no `Image:` builds into a model mxbuild refuses ("No
image selected."); `mxcli check` reports it as MDL-WIDGET22. A name that does not
resolve is reported by `mxcli check --references` rather than by the build
(CE1613). The other two sources are `ImageType: imageUrl, ImageUrl: '…'` and
`ImageType: icon`.

## Icon Collections (read-only)

Icon collections (`CustomIcons$CustomIconCollection`, e.g. `Atlas_Core.Atlas_Filled`) ship with the theme/Atlas. Their icons are referenced from a widget as `Module.Collection.IconName` (a button's `icon:`). Use these to discover valid icon names — icons have non-obvious names (it's `add`, not `plus`).

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show collections | `show icon collections [in module];` | Name, prefix, export level, icon count |
| Describe collection | `describe icon collection Module.Name;` | Lists every icon + its ready-to-use `Module.Collection.IconName` reference |

**Export levels:** `'Hidden'` (default, internal to module), `'Public'` (accessible from other modules).

## Consumed REST Services

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show clients | `show rest clients [in module];` | List all or filter by module |
| Describe client | `describe rest client Module.Name;` | Re-executable CREATE |
| Create client | See syntax below | Property-based `{}` syntax |
| Create or modify | `create or modify rest client ...` | Replaces existing |
| Drop client | `drop rest client Module.Name;` | |
| Import from OpenAPI | See OpenAPI import below | Auto-generate from spec |
| Preview OpenAPI | `describe contract operation from openapi 'path';` | Preview without writing |

```sql
create rest client Module.Api (
  BaseUrl: 'https://api.example.com',
  authentication: none
)
{
  operation GetItems {
    method: get,
    path: '/items/{id}',
    parameters: ($id: string),
    query: ($filter: string),
    headers: ('Accept' = 'application/json'),
    timeout: 30,
    response: json as $Result
  }

  operation CreateItem {
    method: post,
    path: '/items',
    headers: ('Content-Type' = 'application/json'),
    body: mapping Module.ItemRequest {
      name = Name,
      price = Price,
    },
    response: mapping Module.ItemResponse {
      Id = id,
      status = status,
    }
  }
};
```

**Body types:** `json from $var`, `template '...'`, `mapping entity { jsonField = attr, ... }`
**Response types:** `json as $var`, `string as $var`, `file as $var`, `status as $var`, `none`, `mapping entity { attr = jsonField, ... }`
**Authentication:** `none`, `basic (username: '...', password: '...')`

### OpenAPI Import

Generate a consumed REST service document directly from an OpenAPI 3.0 spec (JSON or YAML):

```sql
-- From a local file (relative to the .mpr file)
create or modify rest client CapitalModule.CapitalAPI (
  OpenAPI: 'specs/capital.json'
);

-- From a URL
create or modify rest client PetStoreModule.PetStoreAPI (
  OpenAPI: 'https://petstore3.swagger.io/api/v3/openapi.json'
);

-- Override the base URL from the spec (e.g. point at staging instead of prod)
create or modify rest client PetStoreModule.PetStoreStaging (
  OpenAPI: 'https://petstore3.swagger.io/api/v3/openapi.json',
  BaseUrl: 'https://staging.petstore.example.com/api/v3'
);

-- Preview without writing to the project
describe contract operation from openapi 'specs/capital.json';
```

Operations, path/query parameters, headers, request body, response type, resource groups (from OpenAPI `tags`), and Basic auth are all derived automatically. The spec is stored inside the REST client document for Studio Pro parity.

`BaseUrl` is optional. When omitted, the base URL is taken from `servers[0].url` in the spec. When provided, it overrides that value — useful when the spec points at production but you want to import against a different environment.

## Published REST Services

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show services | `show published rest services [in module];` | List all or filter by module |
| Describe service | `describe published rest service Module.Name;` | Re-executable CREATE statement |
| Create service | See below | |
| Create or modify | `create or modify published rest service Module.Name (...) { ... };` | Preserves UUID — preferred for AI agents |
| Alter service | `alter published rest service Module.Name set path = '...', version = '...';` | SET supports Path, Version, ServiceName |
| Add resource | `alter published rest service Module.Name add resource 'name' { ... };` | Operation block uses CREATE syntax |
| Drop resource | `alter published rest service Module.Name drop resource 'name';` | |
| Drop service | `drop published rest service Module.Name;` | |
| Grant access | `grant access on published rest service Module.Name to Module.Role, ...;` | Adds module roles to AllowedRoles |
| Revoke access | `revoke access on published rest service Module.Name from Module.Role, ...;` | |

```sql
create published rest service Module.MyAPI (
  path: 'rest/api/v1',
  version: '1.0.0',
  ServiceName: 'My API',
  folder: 'Integration/REST'
)
{
  resource 'orders' {
    get '' microflow Module.GetAllOrders;
    get '{id}' microflow Module.GetOrderById;
    post '' microflow Module.CreateOrder;
    put '{id}' microflow Module.UpdateOrder;
    delete '{id}' microflow Module.DeleteOrder;
  }
  resource 'customers' {
    get '' microflow Module.GetAllCustomers;
  }
};
```

**Properties:** `path` (required), `version`, `ServiceName`, `folder`
**HTTP methods:** `get`, `post`, `put`, `delete`, `patch`
**Operation paths:** Empty string `''` for the root, `'{paramName}'` for path parameters. Do NOT start or end with `/`.
**Path parameters:** Must match a microflow parameter exactly (case-sensitive). E.g., `'{id}'` requires the microflow to have parameter `$id`.
**Operation modifiers:** `deprecated`, `import mapping Module.Name`, `export mapping Module.Name`, `commit Yes|No`

## Data Transformers

Requires Mendix 11.9+. Steps: `jslt`, `xslt`. Single-line: `jslt '...'`. Multi-line: `jslt $$ ... $$`.

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List transformers | `list data transformers [in module];` | |
| Describe transformer | `describe data transformer Module.Name;` | Re-executable CREATE |
| Create transformer | See syntax below | |
| Create or modify | `create or modify data transformer Module.Name ...;` | Updates existing transformer, preserves UUID |
| Drop transformer | `drop data transformer Module.Name;` | |

```sql
create data transformer Module.WeatherTransform
source json '{"latitude": 51.9, "current": {"temp": 12.8}}'
{
  jslt $$
{
  "lat": .latitude,
  "temp": .current.temp
}
  $$;
};
```

## JSON Structures

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show structures | `show json structures [in module];` | List all or filter by module |
| Describe structure | `describe json structure Module.Name;` | Re-executable CREATE OR MODIFY + element tree |
| Create structure | `create json structure Module.Name [comment 'text'] snippet '...json...';` | Element tree auto-built from snippet |
| Create (multi-line) | `create json structure Module.Name snippet $${ "key": "value" }$$;` | Dollar-quoted snippet for readability |
| Create or modify | `create or modify json structure Module.Name snippet '...';` | Preserves UUID — preferred for AI agents |
| Create with name map | `create json structure Module.Name snippet '...' CUSTOM NAME map ('jsonKey' as 'CustomName', ...);` | Override auto-generated ExposedNames |
| Name an array's item | `CUSTOM NAME map (item of 'lines' as 'OrderLine')` | An item has no JSON key; `item of 'Root'` for a root array |
| Message definition collection | `create [or modify] message definition collection M.Name [folder '...'] ( definition D for M.Entity [as 'X'] ( members ) );` | A selection over the domain model — the one non-JSON mapping source MDL can create |
| Message definition member | attribute: `OrderId [as 'X'] [example '...']`; association: `M.Assoc/M.Entity [as 'X'] ( ... )` | Naming the target sets the traversal direction, which decides the cardinality |
| Alter a definition's members | `alter message definition M.Coll.Def add\|drop\|set member X [in path] [as 'Y']` | Addressed as Module.Collection.Definition; `set` changes only the exposed name |
| Alter a collection | `alter message definition collection M.Coll add\|drop\|rename definition ...` | |
| Browse | `show message definition collections [in M]`, `describe message definition collection M.Name` | |
| Drop structure | `drop json structure Module.Name;` | |

## Import Mappings

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show mappings | `show import mappings [in module];` | List all or filter by module |
| Describe mapping | `describe import mapping Module.Name;` | Re-executable CREATE statement |
| Create mapping | See below | Assignment syntax: `attr = jsonField`, or `attr = a/b/c` to reach a nested leaf with **no entity per level** — the shape Studio Pro produces. The path may not cross a `0..*` element (CE0256) |
| Create or modify | `create or modify import mapping Module.Name ...;` | Updates existing mapping, preserves UUID |
| Place in a folder | `create [or modify] import mapping Module.Name folder 'path' ...;` | Clause goes after the name. On `or modify` it **moves** the mapping; omitting it leaves placement alone |
| Drop mapping | `drop import mapping Module.Name;` | |
| Schema source | `with json structure Module.JSON_X` / `with message definition Module.Collection.Definition` / `with xml schema Module.Schema` | A **message definition** is derived from the domain model rather than a payload sample, so its members are the definition's exposed names and the reference is **three parts** — the definitions live inside a collection document. Read-only: map over one that already exists |
| Nested schema root | `with json structure Module.JSON_X root choices/message` | Starts the mapping at a nested element instead of the structure's root. Written in member names; the path may pass through an array, and the mapping is then rooted at the item |
| Array-rooted structure | no special syntax | The root is taken from the structure, so `[{...}]` and `{...}` are written the same way |
| Export grouping node | `group as wrapper { Assoc/Entity as items { ... } }` | A JSON object with no Mendix object behind it. **Object children only** — a value there has no entity to bind to, and Mendix reports CE0061 |
| Array of primitives | `Assoc/Entity = tags { Value = Value }` (import) / `Assoc/Entity as tags { Value = Value }` (export) | `["a","b"]` maps to one entity per string. Written like any other array — the wrapper level is generated — and the primitive is the reserved member `Value` |
| Export array | `Assoc/Entity as items { values }` | Declared like a nested object; mxcli generates the bare Array container Studio Pro stores. The older two-level form `Assoc/Entity as items { ItemAssoc/ItemEntity as ItemsItem { ... } }` still works and names an entity per level |
| Custom object handling | `find Module.Entity by Module.Microflow ( Param: parent ) = member { ... }` | A microflow resolves the object instead of Create/Find. Parameter sources: `parent` (the enclosing mapped object), `parameter` (the mapping's own input), `parent(2)` (an ancestor N levels up), or a member path such as `idx` (a value from the payload — mxcli adds the value element Mendix requires for it). modelsdk engine only |
| Value transform | `Attr = Module.Microflow(jsonField)` (import) / `jsonField = Module.Microflow(Attr)` (export) | The value passes through a microflow. The stored element carries only the microflow — its input **is** the member the element binds, which is why the member is named inside the call. An unresolvable microflow is refused |

```sql
create import mapping Module.IMM_Pet
  with json structure Module.JSON_Pet
{
  create Module.PetResponse {
    PetId = id key,
    Name = name,
    status = status
  }
};
```

**Input object:** an import mapping may take an object as a parameter, which a
custom handler binds with `Param: parameter`:

```sql
create import mapping Module.IMM_Response
  with json structure Module.JSON_Response
  parameter GenAICommons.ChunkCollection
{ ... }
```

Import only — an export mapping's parameter is its root object.

**Object handling:** `create` (default), or `find` — which requires a KEY *and*
must say what happens when nothing is found:

| Syntax | When the object is not found |
|--------|------------------------------|
| `find Module.Entity or create` | Create one (same as `find or create Module.Entity`) |
| `find Module.Entity or error` | Fail the import |
| `find Module.Entity or ignore` | Skip the element |

Add `overridable` (`find Module.Entity or create overridable`) to let the caller
override the choice at import time. A bare `find` is refused: the three
behaviours are not interchangeable, and mxcli used to pick one silently.

**Nested objects:** Use association path `Assoc/entity = jsonKey`:
```sql
create Module.OrderResponse_CustomerInfo/Module.CustomerInfo = customer {
  Email = email,
  Name = name
}
```

## Export Mappings

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show mappings | `show export mappings [in module];` | List all or filter by module |
| Describe mapping | `describe export mapping Module.Name;` | Re-executable CREATE statement |
| Create mapping | See below | Assignment syntax: `jsonField = attr`. **No nested `a/b/c` form**: an export has to produce the intermediate node, so Mendix rejects a collapsed member with CE5015 — give it its own element |
| Create or modify | `create or modify export mapping Module.Name ...;` | Updates existing mapping, preserves UUID |
| Place in a folder | `create [or modify] export mapping Module.Name folder 'path' ...;` | Clause goes after the name. On `or modify` it **moves** the mapping; omitting it leaves placement alone |
| Drop mapping | `drop export mapping Module.Name;` | |

```sql
create export mapping Module.EMM_Pet
  with json structure Module.JSON_Pet
  null values LeaveOutElement
{
  Module.PetResponse {
    id = PetId,
    name = Name,
    status = status
  }
};
```

**Nested objects:** Use association path `Assoc/entity as jsonKey`:
```sql
Module.OrderResponse_CustomerInfo/Module.CustomerInfo as customer {
  email = Email,
  name = Name
}
```

## Java Actions

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show Java actions | `show java actions [in module];` | List all or filtered by module |
| Describe Java action | `describe java action Module.Name;` | Full MDL output with signature |
| Create Java action | `create [or modify] java action Module.Name [folder 'path'](params) returns type as $$ ... $$;` | OR MODIFY updates signature/body, preserves UUID |
| Create with type params | `create java action Module.Name(EntityType: entity <pEntity>, Obj: pEntity) ...;` | Generic type parameters |
| Create exposed action | `... exposed as 'caption' in 'Category' as $$ ... $$;` | Toolbox-visible in Studio Pro |
| Exposed action with bitmaps | `... exposed as 'caption' in 'Category' icon 'i.png' icon dark 'id.png' image 'm.png' image dark 'md.png' as $$ ... $$;` | Icon 64x64 PNG, image 256x192; paths relative to the .mdl file's own directory. A wrong size warns and is written; a non-PNG is refused |
| Remove a toolbox entry | `... not exposed as $$ ... $$;` | An **omitted** clause preserves the stored entry (bitmaps included), so removal is explicit |
| Clear one bitmap | `... exposed as 'c' in 'C' drop icon dark as $$ ... $$;` | `drop icon\|image [dark]` clears exactly one; the others are untouched |
| Rename Java action | `rename java action Module.Old to New;` | Renames BSON unit and .java source file |
| Rename Java action (dry run) | `rename java action Module.Old to New dry run;` | Preview reference changes without modifying |
| Drop Java action | `drop java action Module.Name;` | Deletes MPR unit and .java source file |
| Call from microflow | `$Result = call java action Module.Name(Param = value);` | Inside BEGIN...END |
| Empty argument | `call java action Module.Name(Param = empty);` | Unbound code-action parameter preserved as empty mapping |
| Show JavaScript actions | `show javascript actions [in module];` | List all or filtered by module |
| Describe JavaScript action | `describe javascript action Module.Name;` | Re-executable MDL with signature + body |
| Create JavaScript action | `create [or modify] javascript action Module.Name [folder 'path'](params) returns type [platform Web] as $$ ... $$;` | Writes the unit + `javascriptsource/<Module>/actions/<Name>.js`; OR MODIFY preserves UUID |
| Create exposed/native | `... exposed as 'caption' in 'Category' platform Native as $$ ... $$;` | `platform` is Web (default), Native, Hybrid, or All |
| Drop JavaScript action | `drop javascript action Module.Name;` | Deletes MPR unit and .js source file |
| Call from nanoflow | `$Result = call javascript action Module.Name(Param = value);` | Inside a nanoflow |

**`AS $$ ... $$` is mandatory** — the body cannot be omitted. Omitting it causes `no viable alternative at input '...'`. Use `as $$ return false; $$;` as a stub.

**Parameter Types:** `string`, `integer`, `long`, `decimal`, `boolean`, `datetime`, `Module.Entity`, `list of Module.Entity`, `enum Module.EnumName`, `enumeration(Module.EnumName)`, `stringtemplate(sql)`, `stringtemplate(Oql)`, `entity <pEntity>` (type parameter declaration), bare `pEntity` (type parameter reference).

**Type Parameters** allow generic entity handling. `entity <pEntity>` declares the type parameter inline and becomes the entity type selector; bare `pEntity` parameters receive entity instances:
```sql
create java action Module.Validate(
  EntityType: entity <pEntity> not null,
  InputObject: pEntity not null
) returns boolean
exposed as 'Validate Entity' in 'Validation'
as $$
return InputObject != null;
$$;
```

## Pages

MDL uses explicit property declarations for pages:

| Element | Syntax | Example |
|---------|-----------|---------|
| Page properties | `(key: value, ...)` | `(title: 'Edit', layout: Atlas_Core.Atlas_Default)` |
| Pop-up dimensions | `PopupWidth: n, PopupHeight: n, PopupResizable: bool` | `(Layout: Atlas_Core.PopupLayout, PopupWidth: 800, PopupHeight: 480, PopupResizable: true)` — case-sensitive; default 600×600 |
| Page CSS class / style | `Class: 'css-class', Style: 'css: rule'` | `(Title: 'Home', Class: 'container-fluid bg-light', Style: 'min-height: 100vh')` — the page's Appearance |
| Page variables | `variables: { $name: type = 'expr' }` | `variables: { $show: boolean = 'true' }` |
| Repeated widget entries | `<container> <name> ( … )` **in the widget body** | A repeatable property (FileUploader `allowedFileFormats`, HTML Element `attributes`, a chart's `series`) is a block, never a property value. `attributes: [(attributeName: 'x')]` is **MDL-WIDGET27** — it used to check clean, exec, and vanish from storage. `describe widget <name> -p app.mpr` lists the container keywords |
| Data grid 2 column filter | `column c (attribute: A) { textfilter f }` | **Inside the column's braces.** `column c (…) filter f { … }` is the GALLERY form — the grammar reads it as a column with no body plus a sibling `filter` widget, which the grid has nowhere to put; it used to be dropped on write and is now **MDL-WIDGET30**. A grid-wide filter bar is `controlbar`; a gallery spells that same slot `filter`. Match the filter to the column's type (String → `textfilter`, number → `numberfilter`, DateTime → `datefilter`, Enumeration → `dropdownfilter`, Boolean → none) |
| Widget with nowhere to go | any widget in a pluggable widget's body | A child matching no container, slot or `template` catch-all is **MDL-WIDGET30** at check time and refused by `exec`. `describe widget <name> -p app.mpr` lists what the parent declares. Needs the parent's definition, so it is silent without `-p` |
| Inspect a widget | `describe widget <keyword\|'widget id'>;` | `describe widget combobox;` — properties, enum values, defaults and the editor rules that HIDE properties under some configurations. **Body containers** names what the widget's body takes, and for an object list the widgets-typed slots *inside one item* plus the widget types that route into each — that is where `column … { textfilter }` is spelled out. Works with no project open; with one, reads the installed `.mpk` (version-accurate, and the only place a Marketplace widget appears). Same output as `mxcli widget describe` |
| Widget name | Required after type | `textbox txtName (...)` |
| Attribute binding | `attribute: AttrName` | `textbox txt (label: 'Name', attribute: Name)` |
| Variable binding | `datasource: $Var` | `dataview dv (datasource: $Product) { ... }` |
| Action binding | `action: type` | `actionbutton btn (caption: 'Save', action: save_changes)` — the forms are a closed set (`mxcli syntax page.action`); anything else is **MDL-WIDGET28** |
| No action | `action: nothing` | `actionbutton btn (caption: 'Decorative', action: nothing)` — an explicitly inert control. Write it deliberately: an action keyword **short its argument** (`action: open_link` with no URL) is now an error rather than a widget silently written with no action at all |
| Microflow action | `action: microflow Name(Param: val)` | `action: microflow Mod.ACT_Process(Order: $Order)` |
| Button icon | `icon: 'Module.IconCollection.IconName'` | `linkbutton btn (caption: 'Edit', action: nothing, icon: 'Atlas_Core.Atlas_Filled.pencil')` — the **icon-collection** icon; MxBuild rejects an unknown name (CE1613) |
| Image icon | `icon: image Module.ImageCollection.Name` | `actionbutton btn (caption: 'Logo', action: nothing, icon: image MyMod.Images.logo)` — an **image** collection is a different document from an icon collection, and the names are spelled the same, so the keyword is what separates them. Written without `image` it is stored as a custom-icon reference and the build fails **CE1613** |
| Glyph icon | `icon: glyph <code>` | `actionbutton btn (caption: 'Home', action: nothing, icon: glyph 57377)` — a font code point with no name. Codes are sparse; an undefined one fails only at `mxbuild --target=deploy`, naming the **page**, so **MDL078** checks it. Browse with `show glyphs` |
| Clickable container | `onclick: action` (alias of `action:`) | `container card (onclick: microflow Mod.ACT_Open) { ... }` — takes an argument list like a button: `action: nanoflow Mod.ACT_Ship($Order = $dgOrders)` |
| Action arguments | every parameter needs one | A flow action with an unfilled parameter is **CE1571**. An enclosing data container of its type supplies it; a data grid's **control bar** does not (not row-scoped) — pass the grid's selection, `$dgOrders` |
| Database source | `datasource: database entity` | `datagrid dg (datasource: database Module.Entity)` |
| Selection binding | `datasource: selection widget` | `dataview dv (datasource: selection galleryList)` |
| Association source ("data from context") | `datasource: $currentObject/Module.Assoc` | nested `dataview dvCust (datasource: $currentObject/Order_Customer)` shows the to-one referenced object; a list widget shows the to-many collection |
| CSS class | `class: 'classes'` | `container c (class: 'card mx-spacing-top-large')` |
| Inline style | `style: 'css'` | `container c (style: 'padding: 16px;')` |
| Dynamic classes | `dynamicclasses: 'expr'` | `container c (dynamicclasses: 'if $currentObject/IsActive then ''is-active'' else ''''')` — runtime-computed classes; stacks on `class` |
| Design properties | `designproperties: [...]` | `container c (designproperties: ['Spacing top': 'Large', 'full width': on])` |
| Width (pixels) | `width: integer` | `image img (width: 200)` |
| Height (pixels) | `height: integer` | `image img (height: 150)` |
| Page size | `PageSize: integer` | `datagrid dg (PageSize: 25)` |
| Pagination mode | `Pagination: mode` | `datagrid dg (Pagination: virtualScrolling)` |
| Paging position | `PagingPosition: pos` | `datagrid dg (PagingPosition: both)` |
| Paging buttons | `ShowPagingButtons: mode` | `datagrid dg (ShowPagingButtons: auto)` |

**Layouts:**

| Operation | Syntax | Notes |
|-----------|--------|-------|
| List layouts | `show layouts [in module];` | |
| Describe layout | `describe layout Module.Name;` | Round-trippable MDL — describe an Atlas layout, rename it, run it to get a copy in your own module |
| Create layout | `create [or replace] layout Module.Name ( layouttype: 'X' ) { <widgets> };` | modelsdk engine only. Refused in a Marketplace module: an update replaces the module and the edit is gone |
| Drop layout | `drop layout Module.Name;` | Pages still bound to it are named in a warning and the drop proceeds; left dropped they fail **CE1613**, which names the *page* |
| Declare a placeholder | `placeholder Main` | **No body.** Exactly one must be named `Main` — mxbuild enforces it (**CE0848**/**CE0849**), and names must be unique (**CE0495**). `placeholder X { … }` is the page-side form and declares nothing (MDL083) |
| Alter layout | `alter layout Module.Name { <alter-page operations> };` | Edits the stored document, so widgets MDL cannot spell survive. Refused for a Marketplace target |
| Repoint one page | `alter page Module.Page { set Layout = Module.Layout [map (Old as New, …)]; };` | Rewrites the layout reference **and** every placeholder binding |
| Repoint many pages | `alter pages [in <module>] set layout = Module.Layout [map (…)] [where layout = Module.Old];` | The migration form. Marketplace pages are skipped and named. A `where layout` that names no real layout is an error, not a 0-page success |

| Layout element | Syntax | Notes |
|----------------|--------|-------|
| Layout type | `layouttype: 'Responsive' \| 'Phone' \| 'Tablet' \| 'ModalPopup'` (web) · `'Default' \| 'Popup'` (native) | The two vocabularies are disjoint, so the platform is inferred — there is no `native:` flag |
| Layout class | `class: 'layout-atlas layout-atlas-responsive-topbar'` | **Load-bearing.** Atlas scopes ~24 layout rules to `.layout-atlas`; without it the layout builds clean and renders with no topbar bar or sidebar rail. Use `-responsive-default` for sidebar navigation; popups are bare |
| Scroll container | `scrollcontainer name { <regions> }` | The layout's root; its children are regions, not widgets |
| Region | `region top \| right \| bottom \| left \| center [( size: N, sizemode: 'Fixed'\|'Pixels'\|'Auto', class: '…' )] { <widgets> }` | Five named slots, not a list. One region per slot |
| Placeholder | `placeholder Main` | The slot a page's content goes into. The name is API — a page binds as `Module.Layout.<Name>`. Name one `Main`: that is how Mendix picks the main placeholder (`Forms$Layout` has no property for it). At least one is required |
| Navigation tree | `navigationtree name (profile: 'Responsive')` | The sidebar menu (vertical); the profile is a navigation profile name |
| Menu bar | `menubar name (profile: 'Responsive')` | The topbar menu (horizontal); same stored shape as a navigation tree |
| Region as ALTER target | `<scrollContainerName>.<slot>` | A region has no name — its slot is its identity. `INSERT INTO layoutContainer.top { … }`. Only `INSERT INTO`; use a widget name for `BEFORE`/`AFTER` |

**Snippets & Building Blocks (read-only discovery):**

| Operation | Syntax | Notes |
|-----------|--------|-------|
| List snippets | `show snippets [in module];` | Editable via `create/alter snippet` |
| Describe snippet | `describe snippet Module.Name;` | Round-trippable MDL output |
| List building blocks | `show building blocks [in module];` | Read-only; cannot be authored via MDL |
| Describe building block | `describe building block Module.Name;` | Informational (header comment + widget tree), not a `create` statement |
| Create menu | `create [or modify] menu Module.Name [folder 'path'] ( <items> );` | Standalone `Menus$MenuDocument`. Full replacement: the item list is the document's complete contents |
| Describe menu | `describe menu Module.Name;` | Round-trippable MDL. Not the navigation-profile menu — see `show navigation menu` |
| Drop menu | `drop menu Module.Name;` | |
| Create menu | `create [or modify] menu Module.Name [folder 'path'] ( <items> );` | Standalone `Menus$MenuDocument`. Full replacement: the item list is the document's complete contents |
| Describe menu | `describe menu Module.Name;` | Round-trippable MDL. Not the navigation-profile menu — see `show navigation menu` |
| Drop menu | `drop menu Module.Name;` | |

**DataGrid Column Properties:**

| Property | Values | Default | Example |
|----------|--------|---------|---------|
| `attribute` | attribute name, or association path `Assoc/Attr` | (required) | `attribute: Price` · `attribute: Order_Customer/Name` (associated attr; bare association name, multi-hop OK) |
| `caption` | string | attribute name | `caption: 'Unit Price'` |
| `Alignment` | `left`, `center`, `right` | `left` | `Alignment: right` |
| `WrapText` | `true`, `false` | `false` | `WrapText: true` |
| `Sortable` | `true`, `false` | `true`/`false` | `Sortable: false` |
| `Resizable` | `true`, `false` | `true` | `Resizable: false` |
| `Draggable` | `true`, `false` | `true` | `Draggable: false` |
| `Hidable` | `yes`, `hidden`, `no` | `yes` | `Hidable: no` |
| `ColumnWidth` | `autofill`, `autoFit`, `manual` | `autofill` | `ColumnWidth: manual` |
| `Size` | integer (px) | `1` | `Size: 200` |
| `visible` | expression string | `true` | `visible: '$showColumn'` (page variable, not $currentObject) |
| `DynamicCellClass` | expression string | (empty) | `DynamicCellClass: 'if(...) then ... else ...'` |
| `tooltip` | text string | (empty) | `tooltip: 'Price in USD'` |

**Page Example:**
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
    combobox cbStatus (label: 'Status', attribute: status)

    footer footer1 {
      actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
      actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
    }
  }
}
```

**Widget Properties:**

| Property | Syntax | Notes |
|----------|--------|-------|
| DesktopWidth | `column col (desktopwidth: 8)` | 1-12 or AutoFill |
| TabletWidth | `column col (tabletwidth: 6)` | 1-12 or AutoFill (default: auto) |
| PhoneWidth | `column col (phonewidth: 12)` | 1-12 or AutoFill (default: auto) |
| Visible | `textbox txt (visible: [IsActive])` | Conditional visibility (XPath expression) |
| Editable | `textbox txt (editable: [status != 'Closed'])` | Conditional editability (XPath expression) |

**Supported Widgets:**
- Layout: `layoutgrid`, `row`, `column`, `container`, `customcontainer`
- Input: `textbox`, `textarea`, `checkbox`, `radiobuttons`, `datepicker`, `combobox`
- Display: `dynamictext`, `datagrid`, `gallery`, `listview`, `image`, `staticimage`, `dynamicimage`

### List View specialization templates

A List View over a generalization can render a different body per specialization.
The template is identified by the **entity** it renders — it has no name:

```sql
listview vehicleListView (DataSource: database from Pages.Vehicle) {
  dynamictext defaultVehicle (Content: '{1}', ContentParams: [{1} = Brand])

  template for Pages.Bus {
    dynamictext busLabel (Content: 'Bus, capacity {1}', ContentParams: [{1} = PassengerCapacity])
  }
  template for Pages.Truck {
    dynamictext truckLabel (Content: 'Truck, max load {1} kg', ContentParams: [{1} = MaxLoadKg])
  }
}
```

Widgets in the list view body are the **default** rendering, used for an object no
template matches. Templates keep their source order — Mendix stores and matches in
that order, so it is authored, not derived.

`template for Module.Entity` is not the same statement as a Gallery's
`template <name>`, which is a named content slot. The entity must be the list
view's entity or a specialization of it, and at most one template per entity is
allowed. Inside a template the context object is the specialization, so an
attribute only that specialization has still resolves.

Editing them with `alter page` — adding reuses `insert into` with the same block;
removing needs its own form, because a template has no name:

```sql
alter page Pages.Vehicle_Overview {
  insert into vehicleListView {
    template for Pages.Motorcycle { dynamictext mcLabel (Content: 'M') }
  };
  drop template for Pages.SUV in vehicleListView
};
```

Naming the list view in the `drop` is required: one page can hold two list views
with a template for the same entity. Widgets inside a template are ordinary named
widgets, so `set … on busLabel` and `insert after busLabel { … }` need nothing new.
- Actions: `actionbutton`, `linkbutton`, `navigationlist`
- Structure: `dataview`, `header`, `footer`, `controlbar`, `snippetcall`

**Drop-down filter, association mode** — filter a datagrid by a reference instead of an attribute. Giving the filter a `datasource:` (the OPTION list) selects the mode; all three parts are required:
```sql
column colCustomer (attribute: Order_Customer/Name, caption: 'Customer') {
  dropdownfilter ddfCustomer (
    Association: Sales.Order_Customer,     -- the reference on the GRID entity
    datasource: database Sales.Customer,   -- the option list (associated entity)
    CaptionAttribute: Name                 -- what each option shows
  )
}
```
A column cannot bind the association itself: `column c (attribute: Order_Customer)` is refused, because Mendix has nowhere to store a reference in an attribute-typed widget property (the build fails CE1613). Traverse it (`attribute: Assoc/Attr`) to show a value; use the mode above to filter by it.

**DynamicText parameter formatting** — append a `format (…)` block to a content parameter (the `format` keyword is required):
```sql
dynamictext amt (content: '{1}', contentparams: [{1} = Amount format (decimalPrecision: 2, groupDigits: true)])
dynamictext due (content: '{1}', contentparams: [{1} = DueOn  format (dateFormat: DateTime)])
```
Keys: `decimalPrecision` (int), `groupDigits` (bool), `dateFormat` (`Date`|`DateTime`|`Time`|`Custom`), `customDateFormat` (pattern, with `dateFormat: Custom`), `enumFormat` (`Text`|`Image`).

## ALTER PAGE / ALTER SNIPPET

Modify an existing page or snippet's widget tree in-place without full `create or replace`. Works directly on the raw BSON tree, preserving unsupported widget types.

| Operation | Syntax | Notes |
|-----------|--------|-------|
| Set property | `set caption = 'New' on widgetName` | Single property on a widget |
| Set multiple | `set (caption = 'Save', buttonstyle = success) on btn` | Multiple properties at once |
| Page-level set | `set Title = 'New title'` | No ON clause; page-level names are case-sensitive |
| Pop-up dimensions | `set PopupWidth = 800` / `set PopupHeight = 480` / `set PopupResizable = true` | Page-level; apply when the page opens in a pop-up |
| Page CSS class / style | `set Class = 'css-class'` / `set Style = 'css: rule'` | Page-level (no ON clause); sets the page's Appearance |
| Widget dynamic classes | `set DynamicClasses = 'expr' on widgetName` | Runtime-computed classes on a widget — the surgical alternative to a bulk `update widgets` |
| Insert after | `insert after widgetName { widgets }` | Add widgets after target |
| Insert before | `insert before widgetName { widgets }` | Add widgets before target |
| Insert into | `insert into containerName { widgets }` | Append as the container's last child (fills an empty container; dataview children take its entity) |
| Drop widgets | `drop widget name1, name2` | Remove widgets by name |
| Replace widget | `replace widgetName with { widgets }` | Replace widget subtree |
| Pluggable prop | `set 'showLabel' = false on cbStatus` | Quoted name for pluggable widgets |
| Set column prop | `set caption = 'New' on dgGrid.colName` | Dotted ref targets DataGrid column |
| Drop column | `drop widget dgGrid.colName` | Remove a DataGrid column |
| Insert column | `insert after dgGrid.colName { column ... }` | Add column to DataGrid |
| Add variable | `add variables $name: type = 'expr'` | Add a page variable |
| Drop variable | `drop variables $name` | Remove a page variable |
| Set layout | `set layout = Module.LayoutName` | Change page layout, auto-maps placeholders |
| Set layout + map | `set layout = Module.Layout map (Old as New)` | Explicit placeholder mapping |

**Supported SET properties:** Caption, Label, ButtonStyle, Class, Style, DynamicClasses, Editable, Visible, Name, Title (page-level), Layout (page-level), PopupWidth / PopupHeight / PopupResizable (page-level), and quoted pluggable widget properties.

**Example:**
```sql
alter page Module.EditPage {
  set (caption = 'Save & Close', buttonstyle = success) on btnSave;
  drop widget txtUnused;
  insert after txtEmail {
    textbox txtPhone (label: 'Phone', attribute: Phone)
  }
};

alter snippet Module.NavMenu {
  set caption = 'Dashboard' on btnHome
};
```

**Tip:** Run `describe page Module.PageName` first to see widget names.

## Reserved Words and Quoted Identifiers

Most MDL keywords now work **unquoted** as entity names, attribute names, parameter names, and module names. Common words like `caption`, `check`, `content`, `format`, `index`, `label`, `range`, `select`, `source`, `status`, `text`, `title`, `type`, `value`, `item`, `version`, `production`, etc. are all valid without quoting.

Only structural MDL keywords require quoting: `create`, `delete`, `begin`, `end`, `return`, `entity`, `module`.

**Quoted identifiers** escape any reserved word (double-quotes or backticks):
```sql
describe entity "combobox"."CategoryTreeVE";
show entities in "combobox";
create persistent entity Module.VATRate ("create": datetime, Rate: decimal);
```

Both double-quote (ANSI SQL) and backtick (MySQL) styles are supported. You can mix quoted and unquoted parts: `"combobox".CategoryTreeVE`.

### A third grammar: OQL

Quoting means different things in three grammars, and they do not agree:

| Grammar | Example collision | Quoting escapes it? | mxcli rule |
|---|---|---|---|
| MDL parser | `create`, `end`, `entity` | **Yes** — `"create"` | parse error |
| Mendix platform | `Type`, `ID`, `CurrentUser` | **No** — the check strips quotes | MDL021 (CE7247) |
| **Mendix OQL** | `Year`, `Month`, `Quarter`, `Day`, `Hour`… | **Yes** — `"Month"`, as in SQL | **MDL071** (warning) |

An OQL-reserved name is *legal Mendix* — the entity builds and runs. It bites
only when a **view entity**'s OQL references it **unquoted**, which fails with
**CE0174** ("The 'Month' part is incomplete or incorrect"). OQL takes
double-quoted identifiers exactly like SQL, and mxcli writes them through
unchanged, so the fix is normally a quote rather than a rename:

```sql
-- both build at 0 errors
select s."Month" as MonthNo, sum(s.Amount) as Total
from MyFirstModule."Year" as s
group by s."Month";
```

**The exception is an alias, and that limit is OQL's own.** The alias position
takes a bare identifier for *any* name — `as "Total"`, reserved nowhere, is
CE0174 as well. The two CE0174 texts say so precisely: a source position lists
`ASTERISK, AT_SIGN, OPEN_QUOTE, or IDENTIFIER`, the alias position lists only
`IDENTIFIER`. A view entity's own attribute name **is** its alias (they must
match), so a view column cannot be called `Month` at all — that is the one case
that needs a rename, and **MDL072** says so when you try the quoted spelling.

MDL071 warns at `CREATE` / `ALTER … ADD ATTRIBUTE` / `RENAME ATTRIBUTE` so the
choice is made while the name still has few references. MDL032 reports the same
collision from inside a view's OQL, where quoting is the immediate fix.

**Boolean attributes** auto-default to `false` when no `default` is specified.

**CALCULATED** marks an attribute as calculated (not stored). Use `calculated by Module.Microflow` to specify the calculation microflow. Calculated attributes derive their value from a microflow at runtime.

**ButtonStyle** supports all values: `primary`, `default`, `success`, `danger`, `warning`, `info`.

## External SQL Statements

Direct SQL query execution against external databases (PostgreSQL, Oracle, SQL Server). Credentials are isolated — DSN never appears in session output or logs.

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Connect | `sql connect <driver> '<dsn>' as <alias>;` | Drivers: `postgres`, `oracle`, `sqlserver` |
| Disconnect | `sql disconnect <alias>;` | Closes connection |
| List connections | `sql connections;` | Shows alias + driver only (no DSN) |
| Show tables | `sql <alias> show tables;` | Lists user tables |
| Show views | `sql <alias> show views;` | Lists user views |
| Show functions | `sql <alias> show FUNCTIONS;` | Lists functions and procedures |
| Describe table | `sql <alias> describe <table>;` | Shows columns, types, nullability |
| Query | `sql <alias> <any-sql>;` | Raw SQL passthrough |
| Import | `import from <alias> query '<sql>' into Module.Entity map (...) [link (...)] [batch n] [limit n];` | Insert external data into Mendix app DB |
| Generate connector | `sql <alias> generate connector into <module> [tables (...)] [views (...)] [exec];` | Generate Database Connector MDL from schema |

```sql
-- Connect to PostgreSQL
sql connect postgres 'postgres://user:pass@localhost:5432/mydb' as source;

-- Explore schema
sql source show tables;
sql source describe users;

-- Query data
sql source select * from users where active = true limit 10;

-- Import external data into Mendix app database
import from source query 'SELECT name, email FROM employees'
  into HRModule.Employee
  map (name as Name, email as Email);

-- Import with association linking
import from source query 'SELECT name, dept_name FROM employees'
  into HR.Employee
  map (name as Name)
  link (dept_name to Employee_Department on Name);

-- Generate Database Connector from schema
sql source generate connector into HRModule;
sql source generate connector into HRModule tables (employees, departments) exec;

-- Manage connections
sql connections;
sql disconnect source;
```

CLI subcommand: `mxcli sql --driver postgres --dsn '...' "select 1"` (see `mxcli syntax sql`). Supported drivers: `postgres` (pg, postgresql), `oracle` (ora), `sqlserver` (mssql).

## Translations

Bulk translation of every user-visible string, one file per language. Entries use
`as`, not a colon: a translation maps a user-provided name to another name.

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Describe | `describe translations [in Module] for <lang>;` | Emits the CREATE form; an untranslated string comes back with an **empty** target, which is what makes the output an LLM prompt |
| Create | `create translations [in Module] for <lang> ( 'src' as 'target', ... );` | The language is the thing that exists — **errors** if it already has translations |
| Merge | `create or modify translations ...` | A source the file does not name is left alone |
| Replace | `create or replace translations ...` | The file is authoritative: a translation whose source it does not name is **REMOVED**, and the run says which. `in Module` **bounds** the deletion |
| Remove a language's translations | `create or replace translations [in Module] for <lang> ( );` | An empty file is authoritative over nothing, so everything in scope goes — the only way to take a language's translations out of the model |
| Show languages | `show languages;` | ⚠️ languages that **have translations**, not enabled ones — a stock app reports 8 while 1 is enabled. The enabled list is in `describe settings`. Needs `refresh catalog full` |
| Default language | `alter settings LANGUAGE DefaultLanguageCode = 'en_US';` | The language a translation file's left column is written in — **and the language a new `Caption:`/`Title:` is stored under**, so set it before authoring content. Changing it later does not move existing text and nothing warns |

**A translation for a language the project has not enabled is discarded at build
time** — it is stored in the model, passes `mx check`, and produces no
`translations_<code>.properties` at all. The run warns; enable the language in
project settings first.

Keyed on the **source string**, so one entry translates every occurrence. A key
that matches nothing is **reported**, not skipped — a source edited after the
file was written stops matching, and the run names the string it has probably
become.

```bash
mxcli -p app.mpr -c "describe translations for de_DE" > de_DE.mdl
# fill in the right-hand side (by hand or with an LLM)
mxcli exec de_DE.mdl -p app.mpr
```

## Catalog & Search

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Refresh catalog | `refresh catalog;` | Rebuild basic metadata tables |
| Refresh with refs | `refresh catalog full;` | Include cross-references and source |
| Show catalog tables | `show catalog tables;` | List available queryable tables |
| Query catalog | `select ... from CATALOG.<table> [where ...];` | SQL against project metadata |
| Show callers | `show callers of Module.Name;` | What INVOKES this element: microflow call activities, page action buttons and other widget actions, calculated attributes, and navigation entries. A page counts as a caller of the microflow its button runs, and of the page that button opens |
| Show callees | `show callees of Module.Name;` | What this element calls |
| Show references | `show references of Module.Name;` | All references to/from |
| Show impact | `show impact of Module.Name;` | Impact analysis |
| Show context | `show context of Module.Name;` | Surrounding context |
| Full-text search | `search '<keyword>';` | Search across all strings and source |

Cross-reference commands require `refresh catalog full` to populate reference data.

`show callers` covers invocation only. A document that merely *uses a type* — an entity as a page datasource, a microflow parameter, an entity's generalization — is not a caller of it; `show references to` lists those.

A **pluggable or custom widget** is a reference target too, so "which pages use this widget?" is one query — the same question about a Java action always was:

```mdl
show references to combobox;      -- pages and snippets that place a Combo box
show impact of htmlelement;       -- the same, grouped by document type
```

Name the widget the way you write it in a page body. The target is stored as the widget's MDL name and matched case-insensitively when the exact spelling finds nothing, so `combobox`, `ComboBox` and `COMBOBOX` all resolve; the resolved spelling is printed. A built-in Mendix widget (`textbox`, `dynamictext`) has no definition and therefore no edge — use `show widgets` for those.

## Connection & Session

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Connect | `connect local '/path/to/app.mpr';` | Open a Mendix project |
| Disconnect | `disconnect;` | Close current project |
| Status | `status;` | Show connection info |
| Refresh | `refresh;` | Reload project from disk |
| Commit | `commit [message 'text'];` | Save changes to MPR |
| Set variable | `set key = value;` | Session variable (e.g., `output_format = 'json'`) |
| Exit | `EXIT;` | Close REPL session |

## CLI Commands

| Command | Syntax | Notes |
|---------|--------|-------|
| Interactive REPL | `mxcli` | Interactive MDL shell |
| Execute command | `mxcli -p app.mpr -c "show entities"` | Single command |
| JSON output | `mxcli -p app.mpr -c "show entities" --json` | JSON for any command |
| Execute script | `mxcli exec script.mdl -p app.mpr` | Script file |
| Check syntax | `mxcli check script.mdl` | Parse-only validation |
| Check references | `mxcli check script.mdl -p app.mpr --references` | With reference validation |
| Lint project | `mxcli lint -p app.mpr [--format json\|sarif]` | 15 built-in + 27 Starlark rules |
| Report | `mxcli report -p app.mpr [--format markdown\|json\|html]` | Best practices report |
| Test | `mxcli test tests/ -p app.mpr` | `.test.mdl` / `.test.md` files |
| Diff script | `mxcli diff -p app.mpr changes.mdl` | Compare script vs project |
| Diff local | `mxcli diff-local -p app.mpr --ref head` | Git diff for MPR v2 |
| OQL | `mxcli oql -p app.mpr "select ..."` | Query running Mendix runtime |
| External SQL | `mxcli sql --driver postgres --dsn '...' "select 1"` | Direct database query |
| Docker build | `mxcli docker build -p app.mpr` | Build with PAD patching |
| Docker check | `mxcli docker check -p app.mpr` | Validate with `mx check` |
| Diagnostics | `mxcli diag [--bundle]` | Session logs, version info |
| New project | `mxcli new <name> --version X.Y.Z` | Create project from scratch with all tooling |
| Init project | `mxcli init /path/to/project` | Add AI tooling to existing project |
| Setup mxcli | `mxcli setup mxcli [--os linux]` | Download platform-specific mxcli binary |
| LSP server | `mxcli lsp --stdio` | Language server for VS Code |

Set `MXCLI_EXEC_TIMEOUT` to override the per-statement execution timeout (default 5m) used by `mxcli exec` (for example `MXCLI_EXEC_TIMEOUT=12m` or `MXCLI_EXEC_TIMEOUT=900`). `REFRESH CATALOG` is exempt from the default guard — it runs without a wall-clock limit on large projects — unless you set `MXCLI_EXEC_TIMEOUT` explicitly (which then applies to it too, letting you cap it).

## ANTLR4 Parser Architecture

The MDL parser uses ANTLR4 for grammar definition, enabling cross-language grammar sharing (Go, TypeScript, Java, Python).

**Regenerating the parser** (after modifying `MDLLexer.g4` or `MDLParser.g4`):
```bash
# Option 1: use make from project root (recommended)
make grammar

# Option 2: run directly in grammar directory
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
