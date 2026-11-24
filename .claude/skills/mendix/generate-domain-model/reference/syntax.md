# Domain model syntax reference

Supporting reference for [generate-domain-model](../SKILL.md).

## MDL Syntax Reference

**CRITICAL: All CREATE statements MUST have JavaDoc-style documentation**

Every CREATE statement (modules, entities, associations, enumerations, microflows) should have a /** ... */ comment explaining its purpose. This is essential for:
- Team collaboration and knowledge transfer
- Understanding domain model structure
- Long-term maintainability
- Auto-generated documentation

### Module Creation

```sql
mdl 1;
/**
 * Module for financial transaction management
 *
 * Handles accounts, transactions, budgets, and reporting.
 *
 * @since 1.0.0
 */
create module Finance;
```

### Minimap Section Headers (MARK Comments)

**IMPORTANT: Large MDL files (300+ lines) MUST use MARK comments for navigation**

Use `-- MARK: Section Name` comments to create collapsible sections in code editors. This dramatically improves navigation and organization in large domain model files.

**Format**: `-- MARK: Section Name`

**Required for files:**
- 300+ lines: At least 3 MARK comments
- 500+ lines: At least 5 MARK comments

**Recommended sections:**
```sql
-- MARK: ENUMERATIONS

-- MARK: CORE ENTITIES

-- MARK: ASSOCIATIONS

-- MARK: VIEW ENTITIES

-- MARK: MICROFLOWS
```

**With subsections:**
```sql
-- MARK: - Core Entities (Persistent)

-- MARK: - View Entities for Reporting
```

**Benefits:**
- Creates outline/minimap view in VS Code, Xcode-style editors
- Makes large files navigable with jump-to-section
- Groups related code logically
- Improves team collaboration on complex models

### Enumerations

```sql
mdl 1;
/**
 * Transaction type classification
 *
 * Categorizes financial transactions as income or expense
 * for proper accounting and reporting.
 *
 * @since 1.0.0
 */
create enumeration Module.TransactionType (
  INCOME 'Income',
  EXPENSE 'Expense'
);
```

**Editing an existing enumeration** — use `alter enumeration`, never drop + recreate
(a drop is blocked while the enum is referenced by an attribute):

```sql
mdl 1;
alter enumeration Module.TransactionType add value REFUND caption 'Refund';
alter enumeration Module.TransactionType rename value INCOME to CREDIT;      -- changes the name/key
alter enumeration Module.TransactionType modify value EXPENSE caption 'Expense / Debit'; -- caption only, name unchanged
alter enumeration Module.TransactionType drop value REFUND;
```

**Add `if not exists` in a script you will re-run.** The bare `add value` errors
on the second run, and `exec` stops at the failing statement — so one
already-present value leaves every later statement in the file unapplied. The
same pair as `add attribute` / `add index`:

```sql
mdl 1;
alter enumeration Module.TransactionType add value if not exists REFUND caption 'Refund';
alter enumeration Module.TransactionType drop value if exists REFUND;
```

A defensive drop-then-add is not a substitute: the drop fails when the value is
absent and the add when it is present.

`modify value … caption` re-captions in place — the value keeps its identity, so it
works even while the enumeration is in use. (Value names in `alter` must be plain
identifiers; a value whose name is a reserved word can't be targeted by `alter`.)

### Entities

**Positions are optional. Prefer `mxcli layout` over hand-placing them.**

`@position(x, y)` sets where the entity sits in the domain-model diagram. An
entity without one is NOT lost — it takes the next slot in a wrapping grid — but
a grid is a default, not a layout: it knows nothing about which entities are
related, so association lines still cross the diagram.

The better answer for a generated domain model is to write no positions at all
and arrange the module once the script has run:

```bash
mxcli layout -p app.mpr --module MyModule --dry-run   # see the moves
mxcli layout -p app.mpr --module MyModule             # apply
```

That lays entities out from the association graph — each lookup just left of
what uses it, each entity one column past the furthest thing it references, on the row of the
entity it is most linked to — so the lines mostly run one way and straight
across, with room between columns for the association names. It is idempotent (a second run moves nothing) and local (adding an
entity later moves a handful, not the model). It REPLACES positions you set by
hand in the module it touches, which is the reason it is a separate command
rather than something `exec` does on its own. To fix only boxes that collide
and keep every other position — a hand-arranged model — add
`--only-overlapping`.

Write `@position` when you want explicit control of a particular entity, and
remember the coordinate is the box's **TOP-LEFT corner**, not its centre:

- 250+ apart horizontally, to clear a typical box; vertically, leave the box's
  height — about 35px plus 16px per attribute — plus a margin, so a
  20-attribute entity needs ~400px before the next one below it
- group related entities, and keep the lookups together
- `alter entity Mod.Name set position (x, y)` moves one without restating it

**Association line anchors** — where the connector attaches to each entity box —
are set with `@anchor`, as a **percentage of the box** (0..100, whole numbers):

```sql
mdl 1;
@anchor(from: (0, 54), to: (100, 54))
create association Sales.Order_Customer
  from Sales.Order to Sales.Customer;
```

`from` is the anchor on the FROM entity's box, `to` the anchor on the TO
entity's. `(0, 50)` is the middle of the left edge, `(100, 50)` the middle of the
right, `(50, 100)` the bottom centre.

Retune a line without restating the association:

```sql
mdl 1;
alter association Sales.Order_Customer set anchor from (50, 100) to (50, 0);
```

**Naming an end sets it; not naming one preserves what is stored.** An
association written without `@anchor` keeps whatever the line was dragged to in
Studio Pro, so a `create or modify association` about the delete behaviour never
flattens someone's layout. `describe association` re-emits a non-default pair as
the same `@anchor(...)` annotation, so describe → edit → exec round-trips.

Cross-module associations have no anchors at all — Mendix stores none, and
`set anchor` on one is refused.

#### Persistent Entity

```sql
mdl 1;
/**
 * Entity description
 *
 * Detailed explanation of what this entity represents.
 *
 * @since 1.0.0
 * @see Module.RelatedEntity
 */
@position(100, 100)
create persistent entity Module.EntityName (
  /** Unique business key (`Id` itself is reserved by Mendix) */
  Code: long not null error message 'Code is required' unique error message 'Code must be unique',
  /** Attribute description */
  attributename: string(200) not null error message 'Attribute name is required',
  /** Numeric value */
  Amount: decimal,
  /** Date field (there is no date-only type) */
  CreationDate: datetime,
  /** Boolean flag */
  IsActive: boolean not null error message 'IsActive flag is required' default true,
  /** Enumeration field */
  status: enumeration(Module.StatusEnum) not null error message 'Status is required'
);
```

#### Entity Indexes (Performance Optimization)

**CRITICAL: INDEX syntax goes AFTER the closing parenthesis, with NO comma before**

**Re-running an `alter entity ... add index` needs `if not exists`.** An index has
no name — its columns, in order and direction, are its identity — so a second
`add index on (Level)` is the same index twice, which mxbuild rejects with
**CE0072 "Duplicate indexes"**. The bare form refuses it; `add index if not
exists on (Level)` skips. Dropping works the same way and takes the same
selector: `drop index if exists (Level)`.

Indexes improve query performance for frequently filtered or sorted columns. Add them to persistent entities when:
- Column is used in WHERE clauses frequently
- Column is used for sorting (ORDER BY)
- Composite indexes for multi-column filters

**Syntax:**
```sql
mdl 1;
create persistent entity Module.Transaction (
  TransactionDate: datetime not null,
  status: enumeration(Module.Status) not null,
  Amount: decimal not null,
  IsRecurring: boolean default false
)
index (TransactionDate desc)
index (status, TransactionDate)
index (IsRecurring);
```

**Index Guidelines:**
- **Position**: AFTER closing parenthesis, NO comma before first INDEX
- **No names**: Unlike SQL CREATE INDEX, MDL indexes don't have names
- **Sort direction**: ASC or DESC are optional (default is ASC)
- **Composite indexes**: Order matters - put most selective columns first
- **Limit**: Don't over-index - each index has storage/write overhead

**Common index patterns:**
- Date fields: `index (CreatedDate desc)` - for recent-first queries
- Status filters: `index (status, CreatedDate desc)` - for filtered date ranges
- Boolean flags: `index (IsActive)` - for active/inactive filtering
- Foreign keys: Automatically indexed by associations

#### Entity Generalization (EXTENDS)

**CRITICAL: EXTENDS goes BEFORE the opening parenthesis, not after!**

Use `extends` to inherit from a parent entity. Common for file/image storage using System entities.

```sql
mdl 1;
-- Correct: EXTENDS before (
create persistent entity Module.ProductPhoto extends System.Image (
  PhotoCaption: string(200),
  SortOrder: integer default 0
);

-- Correct: File document specialization
create persistent entity Module.Attachment extends System.FileDocument (
  AttachmentDescription: string(500)
);

-- Correct: Custom entity inheritance
create persistent entity Module.Employee extends Module.Person (
  EmployeeNumber: string(20)
);
```

**Wrong** (parse error):
```sql
-- EXTENDS after ) = parse error!
create persistent entity Module.Photo (
  PhotoCaption: string(200)
) extends System.Image;
```

**Note:** `mxcli syntax entity` output may show EXTENDS after `)` — this is misleading. Always place EXTENDS before `(`.

**The parent must exist, and must be qualified.** The generalization is stored by
NAME, so mxcli cannot make one up for you and Mendix reports an unresolved one at
build time. `mxcli check --references` resolves it against the project plus the
script:

```sql
mdl 1;
-- Fine: the parent is created LATER in the same script. A generalization is
-- resolved lazily, so order does not matter.
create persistent entity Module.Manager extends Module.Person (Reports: integer);
create persistent entity Module.Person (Name: string(100));
```

```text
-- Refused: no such entity.        CE1613 "The selected entity … no longer exists"
create persistent entity Module.X extends System.Thumbnail (N: integer);

-- Refused: unqualified.           MDL069 — and this one is worse. A bare name is
-- stored as-is and Mendix cannot load the project AT ALL, so `mx check` dies
-- before reporting anything. Write `extends Module.Person`, even when the parent
-- is in the same module.
create persistent entity Module.Manager extends Person (Reports: integer);
```

**Security follows inheritance.** Mendix inheritance is multi-table: all of the
parent's attributes are members of the child, so a specialized entity's access rule
must cover them. Grant an inherited member exactly like one of the entity's own —
`grant read (AttachmentDescription, "Name", Size) on entity Module.Attachment to Module.Viewer;`
— and `read *` / `write *` cover them too. Skipping them is Mendix CE0066 "Entity
access is out of date". The one exception is entities extending `System.User`, whose
inherited platform members Mendix manages and which must not be granted. See
`manage-security`.

**Member names are unique along the chain.** An inherited member *is* a member of
the child, so the child cannot declare one of the same name — not an attribute, and
not an association it owns (the association's name is a member of its FROM entity,
and under `owner both` of its TO entity too). That holds up the chain to the System
entities (`System.User` has `Name` and the association `UserRoles`;
`System.FileDocument` has `Name` and `Size`) and down it: a generalization cannot
gain a member a specialization already declares. An entity's own attribute and
association cannot share a name either. Names that differ only in case do not
collide. `mxcli check` refuses each (MDL-MEMBER01); Mendix reports CE0069
"Duplicate member name".

```sql
mdl 1;
-- ❌ WRONG: Employee inherits Name from Person — CE0069 "Duplicate member name 'Name'."
create persistent entity Module.Person (Name: string(100));
create persistent entity Module.Employee extends Module.Person (Name: string(100));
```

Drop the child's copy and use the inherited one, or give it another name
(`DisplayName`).

#### System Attributes (Auditing)

Mendix supports four built-in auditing properties on persistent entities. Declare them as regular attributes using pseudo-types (like `autonumber`):

| Pseudo-Type | System Attribute | Set When |
|-------------|-----------------|----------|
| `autoowner` | `System.owner` (→ System.User) | Object created |
| `autochangedby` | `System.changedBy` (→ System.User) | Every commit |
| `autocreateddate` | `CreatedDate` (DateTime) | Object created |
| `autochangeddate` | `ChangedDate` (DateTime) | Every commit |

```sql
mdl 1;
/**
 * Order with full audit trail
 */
create persistent entity Sales.Order (
  OrderNumber: autonumber default 1,
  TotalAmount: decimal not null,
  status: enumeration(Sales.OrderStatus) not null,
  owner: autoowner,
  ChangedBy: autochangedby,
  CreatedDate: autocreateddate,
  ChangedDate: autochangeddate
);
```

To enable/disable on existing entities, use ALTER ENTITY ADD/DROP ATTRIBUTE:

```sql
mdl 1;
alter entity Sales.Order add attribute owner: autoowner;
alter entity Sales.Order add attribute ChangedDate: autochangeddate;
alter entity Sales.Order drop attribute ChangedBy;
```

**When to use auditing:**
- Compliance/regulated domains (finance, healthcare) — use all four
- User-generated content — use AutoOwner for ownership-based access rules
- "Recently modified" lists — use AutoChangedDate
- Avoid on high-volume system tables (every write touches the audit columns)

#### Non-Persistent Entity

**IMPORTANT: Non-persistent entities cannot have validation rules** (`not null error`, `unique error`) on attributes. They can only have `default` values.

```sql
mdl 1;
/**
 * Non-persistent entity description
 *
 * @since 1.0.0
 */
@position(200, 100)
create non-persistent entity Module.TemporaryData (
  SessionId: string(100),
  data: string(1000),
  IsActive: boolean default false
);
```

#### View Entity (with OQL)

```sql
mdl 1;
/**
 * View entity description
 *
 * @since 1.0.0
 */
@position(300, 500)
create view entity Module.ViewName (
  Attribute1: type,
  Attribute2: type
) as (
  select
    e.Id as Id,
    e.Name as Name,
    e.Amount as Amount
  from Module.Entity as e
  where e.IsActive = true
);
```

**Enumeration Comparisons in OQL:**

When comparing enumeration attributes in OQL WHERE clauses, use the **enumeration value** (identifier), not the caption:

```sql
-- Enumeration definition
create enumeration Module.OrderStatus (
  PENDING 'Pending',
  PROCESSING 'Processing',
  CANCELLED 'Cancelled'
);

-- OQL comparison - use the VALUE, not the caption
where e.Status != 'CANCELLED'   -- Correct: uses enum value
where e.Status != 'Cancelled'   -- Wrong: this is the caption
```

### Entity Event Handlers

Microflows can run before/after entity Create, Commit, Delete, or Rollback. Use the optional `raise error` clause to make a handler act as a validation microflow — if it returns false, the operation is aborted.

```sql
mdl 1;
-- In CREATE ENTITY (handlers go after attributes/indexes)
create persistent entity Sales.Order (
  Total: decimal,
  status: string(50)
)
on before commit call Sales.ACT_ValidateOrder raise error
on after create call Sales.ACT_InitDefaults;

-- Add via ALTER ENTITY
alter entity Sales.Order
  add event handler on before delete call Sales.ACT_CheckCanDelete raise error;

-- Drop via ALTER ENTITY (call names which, when the event has several)
alter entity Sales.Order
  drop event handler on before commit call Sales.ACT_ValidateOrder;
```

**Moments**: `before`, `after`
**Events**: `create`, `commit`, `delete`, `rollback`

An entity may have several handlers for one (Moment, Event), as in Studio Pro: a handler is identified by its moment, event and microflow, so `add event handler` with another microflow adds a second one, and `drop event handler … call Mod.MF` says which to remove (without `call`, a drop matching several is refused). The microflow must exist (the executor validates the reference). `raise error` is optional — without it, the handler runs but its return value doesn't affect the operation.

### Associations

**CRITICAL: Association Directionality**

In Mendix, associations are defined **FROM the entity that contains the foreign key TO the entity that is referenced**.

Think of it like this:
- A `Transaction` knows which `Account` it belongs to → Transaction contains the foreign key
- Therefore: `from Transaction to Account`
- **NOT** `from Account to Transaction` ❌

**Common Patterns**:

```text
-- ❌ INCORRECT: Account doesn't store transaction references
create association Finance.Account_Transaction
from Finance.Account to Finance.Transaction
type reference;
```

```sql
mdl 1;
-- ✅ CORRECT: Transaction stores the account reference (foreign key)
create association Finance.Transaction_Account
from Finance.Transaction to Finance.Account
type reference;

-- ✅ One-to-Many: Customer has many Orders (each order knows its customer)
create association Sales.Order_Customer
from Sales.Order to Sales.Customer
type reference;

-- ✅ Many-to-Many: Use ReferenceSet and choose which side stores the relationship
create association Sales.Order_Products
from Sales.Order to Sales.Product
type ReferenceSet
owner both;
```

**Full Association Syntax**:

```sql
mdl 1;
/**
 * Association description
 *
 * Explain the relationship and directionality.
 *
 * @since 1.0.0
 */
create association Module.EntityWithFK_ReferencedEntity
from Module.EntityWithFK to Module.ReferencedEntity
type reference
owner default
on delete set null;
```

**Idempotency**: plain `create association` is **not** idempotent — re-running it
errors with `association already exists`, which aborts the rest of the script (and
any associations defined *after* it are never created). Two spellings fix that,
and they mean different things:

```sql
mdl 1;
-- Converge on this definition, replacing whatever is stored.
create or modify association Module.Child_Parent
from Module.Child to Module.Parent
type reference;

-- Leave an existing association exactly as it is.
create association if not exists Module.Child_Parent
from Module.Child to Module.Parent
type reference;
```

Prefer `if not exists` when the statement is a *delta* rather than the element's
complete definition. `or modify` rebuilds the element from the statement, so a
partial `create or modify entity` drops every attribute it does not list. Writing
both is refused as **MDL085**.

**Association Types**:
- `reference` - One-to-one or many-to-one (foreign key on FROM entity)
- `ReferenceSet` - One-to-many or many-to-many (collection)

**Owner Options**:
- `default` - Standard ownership (FROM entity owns the reference)
- `both` - Both sides own the association (bidirectional)
- `Parent` - Only parent (TO) entity owns
- `Child` - Only child (FROM) entity owns

> **Persistent ↔ non-persistent: FROM the non-persistent entity, owner `default`.**
> That is the only shape Mendix accepts between a persistable and a
> non-persistable entity; a persistent FROM end or `owner both` is **CE0001**, and
> `mxcli check` refuses it as **MDL-ASSOC01** (`alter association … set owner
> both` too).

> **Use `default` ownership for a normal to-one reference.** Reserve `owner both`
> for a `ReferenceSet` (many-to-many). On a plain `type reference`, `owner both`
> makes the association navigable **to-one from *both* sides** — so the reverse
> direction is a single object, not a collection. A **list** widget (listview/
> datagrid/gallery) over the reverse then fails MxBuild **CE8812** "A grid
> association path must result in a list." (a DataView over the reverse is fine —
> it's a single object). With `default` ownership, the reverse is the expected
> to-many collection and a list widget works. See master-detail-pages for the
> widget patterns.

**Delete Behaviors** (`on delete …`; the old `delete_behavior …` clause is deprecated, MDL-DEPR022):
- `on delete cascade` - Delete object and all referencing objects
- `on delete set null` - Delete object, keep references (nullify) — the default
- `on delete restrict [error message '…']` - Only delete if no objects reference it

**Naming Convention**: `{FromEntity}_{ToEntity}` (e.g., `Order_Customer`, `Transaction_Account`)

#### Calculated Attributes

Calculated attributes derive their value from a microflow at runtime. Use `calculated by Module.Microflow` to specify the calculation microflow.

**IMPORTANT: CALCULATED attributes are only supported on PERSISTENT entities.** Using CALCULATED on non-persistent entities will produce a validation error.

```sql
mdl 1;
@position(100, 100)
create persistent entity Module.OrderLine (
  /** Unit price */
  UnitPrice: decimal not null,
  /** Quantity ordered */
  Quantity: integer not null,
  /** Total price, calculated by microflow */
  TotalPrice: decimal calculated by Module.CalcTotalPrice
);
```

**Syntax variants:**
- `calculated by Module.Microflow` — recommended, binds the calculation microflow directly
- `calculated Module.Microflow` — also valid (`by` keyword is optional)
- `calculated` — bare form, marks as calculated but requires manual microflow binding in Studio Pro

**The microflow's signature is checked, and mxcli refuses a mismatch before
writing** — Mendix reports these as **CE7247** at build time (verified on 11.13.0):

| Microflow | Result |
|-----------|--------|
| takes the owning entity (`$Order: Module.Order`) | ✅ stored with `PassEntity = true` |
| takes **no** parameter | ✅ stored with `PassEntity = false` — equally valid |
| takes a *different* entity | ❌ refused: CE7247 *"Microflow parameter 'X' should be of type Module.Order."* |
| takes two or more parameters | ❌ refused |
| returns the wrong type | ❌ refused: CE7247 *"Microflow return type should be …"* |
| returns `Long` for an `integer` attribute (or vice versa) | ✅ accepted — Integer and Long are one family here |

A microflow **created earlier in the same script** cannot be inspected yet, so
its signature is not checked; the build has the last word on those.

> **Before mxcli 0.17 the binding was silently discarded** on the default
> engine: the attribute was written as an ordinary stored value, `mx check`
> reported 0 errors, and the attribute stayed empty at runtime (#917). If you
> have attributes that were declared `calculated by` and never calculated, they
> need re-running through a current mxcli — re-executing the same statement is
> enough.

### Data Types

| Type | Example | Description |
|------|---------|-------------|
| `string(length)` | `string(200)` | Text field with max length |
| `integer` | `integer` | 32-bit integer |
| `long` | `long` | 64-bit integer (use for IDs) |
| `decimal` | `decimal` | Decimal number |
| `boolean` | `boolean` | True/false |
| `datetime` | `datetime` | Date and time, shown in the user's time zone |
| `datetime not localized` | `BirthDate: datetime not localized` | A calendar date / time with no time-zone conversion (Studio Pro's "Localize" unticked). There is no date-only type: `date` is deprecated (MDL-DEPR160) and is a *localized* datetime |
| `binary` | `binary` | Binary data |
| `autonumber` | `autonumber default 1` | Auto-incrementing number (requires DEFAULT start value) |
| `enumeration(Module.Enum)` | `enumeration(Shop.Status)` | Enumeration reference |

### Constraints

**Basic Constraints:**
- `not null` - Field is required
- `unique` - Value must be unique
- `default value` - Default value
- `not localized` / `localized` - DateTime only: Studio Pro's "Localize" setting (see Data Types)

**Validation Error Messages:**

Each constraint can have a custom error message using `error 'message'` syntax:

```sql
mdl 1;
create persistent entity Module.Customer (
  /** Customer name - required with custom error */
  Name: string(200) not null error message 'Name is required',
  /** Email - required and unique with separate error messages */
  Email: string(200) not null error message 'Email is required' unique error message 'Email must be unique',
  /** Age with default value */
  Age: integer default 0,
  /** Active status flag */
  IsActive: boolean not null error message 'IsActive flag is required' default true
);
```

**Error Message Guidelines:**
- Place `error 'message'` immediately after the constraint
- Multiple constraints can each have their own error message
- Keep messages clear and user-friendly
- Follow the pattern: `not null error message 'X is required'` for required fields
- For UNIQUE: `unique error message 'X must be unique'`
- Error messages are shown to end users during validation

**Common patterns:**
```sql
-- Required field
Name: string(200) not null error message 'Name is required',

-- Required and unique
Email: string(200) not null error message 'Email is required' unique error message 'Email must be unique',

-- Required with default
IsActive: boolean not null error message 'IsActive flag is required' default true,

-- Enum with required error
status: enumeration(Module.Status) not null error message 'Status is required',

-- Enum with default value (use fully qualified Module.Enum.Value)
Priority: enumeration(Module.Priority) default Module.Priority.Normal
```
## Reserved Keywords

**Best practice: Always quote all identifiers** (entity names, attribute names) with double quotes. This escapes every **MDL parser** keyword conflict — quotes are stripped automatically by the parser. So `"create"`, `"status"`, `"end"` become valid attribute names.

```sql
mdl 1;
create persistent entity Module."VATRate" (
  "create": datetime,
  "Rate": decimal,
  "status": string(50)
);
```

> **Caveat — quoting does not exempt *platform*-reserved member names.** Some names are
> reserved by the Mendix *platform*, not just the MDL parser, and are rejected **even when
> quoted** (the check strips the quotes and still flags them): `Type` (CE7247, MDL021),
> the audit attributes `CreatedDate` / `ChangedDate` / `Owner` / `ChangedBy` (MDL020 — use
> the `AutoCreatedDate` / `AutoChangedDate` / `AutoOwner` / `AutoChangedBy` pseudo-types
> instead), plus `ID`, `GUID`, `CurrentUser`, `Object` and the Java-keyword list. `"Type": String`
> fails MDL021 — rename to `ResourceType` / `TypeValue`. The same list applies to page,
> snippet, Java action and JavaScript action **parameters** (MDL-PARAM01).

> **Caveat 2 — a third grammar: OQL, where quoting DOES work.** `Year`, `Month`,
> `Quarter`, `Week`, `Day`, `Hour`, `Minute`, `Second`, `Weekday`, `DayOfYear`,
> `DayOfMonth` and `DayOfWeek` are **Mendix OQL** keywords. Unlike the platform-reserved
> names above they are perfectly legal Mendix — the entity builds and runs — so mxcli
> reports them as a **warning** (**MDL071**) rather than refusing them. What breaks is a
> **view entity** whose OQL references the name **unquoted**: **CE0174** ("The 'Month'
> part is incomplete or incorrect"). OQL takes double-quoted identifiers exactly like
> SQL, and mxcli passes them through, so quoting in the view is normally the whole fix:
> `select s."Month" as MonthNo`, `from Module."Year" as s` — both 0 errors. The
> **entity** name counts as well as its attributes.
>
> The one place quoting is unavailable is an **alias**, and that limit is OQL's: it takes
> a bare identifier there for any name (`as "Total"` is CE0174 too, reported as MDL072).
> Since a view entity's attribute name *is* its select alias, a view column cannot be
> called `Month` at all — rename that one.

Both `"Name"` and `` `Name` `` syntax are supported. Prefer double quotes for consistency.

**Boolean attributes** auto-default to `false` when no `default` is specified:
```sql
mdl 1;
create persistent entity Module.Item (
  IsActive: boolean,           -- auto-defaults to false
  IsPublished: boolean default true
);
```
## Entity Positioning

For a generated domain model, write **no** `@position` and run
`mxcli layout --module <Module>` after `exec` (see "Entities" above). Lint rule
`MPR020` fails `mxcli lint` / `mxcli verify` while linked entities still sit on the
default placement grid.

Use `@position(x, y)` only for a few entities added to a module someone arranged
by hand in Studio Pro, where `mxcli layout` would move the hand-placed ones. Put
each beside the entities it links to, leaving room for the association name
between the boxes:

```sql
@position(620, 40)     -- right of Order, which sits at (340, 40)
create persistent entity Module.Invoice (...);
```
