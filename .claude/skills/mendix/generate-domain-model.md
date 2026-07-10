# Creating Mendix Domain Model MDL Scripts

Use this skill to generate Mendix domain model scripts in MDL (Mendix Definition Language) format and validate them with the linter.

## When to Use This Skill

- User asks to create a domain model for a specific use case
- User wants to generate entities, associations, and enumerations
- User requests a complete e-commerce, HR, CRM, or other business domain model
- User needs validation of generated MDL scripts

## MDL Syntax Reference

**CRITICAL: All CREATE statements MUST have JavaDoc-style documentation**

Every CREATE statement (modules, entities, associations, enumerations, microflows) should have a /** ... */ comment explaining its purpose. This is essential for:
- Team collaboration and knowledge transfer
- Understanding domain model structure
- Long-term maintainability
- Auto-generated documentation

### Module Creation

```sql
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

### Entities

**IMPORTANT: All entities MUST have @Position annotation**

The `@position(x, y)` annotation specifies where the entity appears in the domain model diagram. Without it, entities appear at (0,0) or random locations.

**Position Guidelines:**
- Use increments of 50 or 100 for spacing (e.g., 100, 200, 300)
- Leave space between entities (at least 200 pixels)
- Organize related entities in logical groups
- Example layout: Categories at y=100, Transactions at y=300, Reports at y=500

#### Persistent Entity

```sql
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
  /** Unique identifier */
  Id: long not null error 'ID is required' unique error 'ID must be unique',
  /** Attribute description */
  attributename: string(200) not null error 'Attribute name is required',
  /** Numeric value */
  Amount: decimal,
  /** Date field */
  CreationDate: date,
  /** Boolean flag */
  IsActive: boolean not null error 'IsActive flag is required' default true,
  /** Enumeration field */
  status: enumeration(Module.StatusEnum) not null error 'Status is required'
);
```

#### Entity Indexes (Performance Optimization)

**CRITICAL: INDEX syntax goes AFTER the closing parenthesis, with NO comma before**

Indexes improve query performance for frequently filtered or sorted columns. Add them to persistent entities when:
- Column is used in WHERE clauses frequently
- Column is used for sorting (ORDER BY)
- Composite indexes for multi-column filters

**Syntax:**
```sql
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

#### System Attributes (Auditing)

Mendix supports four built-in auditing properties on persistent entities. Declare them as regular attributes using pseudo-types (like `autonumber`):

| Pseudo-Type | System Attribute | Set When |
|-------------|-----------------|----------|
| `autoowner` | `System.owner` (→ System.User) | Object created |
| `autochangedby` | `System.changedBy` (→ System.User) | Every commit |
| `autocreateddate` | `CreatedDate` (DateTime) | Object created |
| `autochangeddate` | `ChangedDate` (DateTime) | Every commit |

```sql
/**
 * Order with full audit trail
 */
create persistent entity Sales.Order (
  OrderNumber: autonumber,
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

-- Drop via ALTER ENTITY
alter entity Sales.Order
  drop event handler on before commit;
```

**Moments**: `before`, `after`
**Events**: `create`, `commit`, `delete`, `rollback`

Each (Moment, Event) combination can only have one handler per entity. The microflow must exist (the executor validates the reference). `raise error` is optional — without it, the handler runs but its return value doesn't affect the operation.

### Associations

**CRITICAL: Association Directionality**

In Mendix, associations are defined **FROM the entity that contains the foreign key TO the entity that is referenced**.

Think of it like this:
- A `Transaction` knows which `Account` it belongs to → Transaction contains the foreign key
- Therefore: `from Transaction to Account`
- **NOT** `from Account to Transaction` ❌

**Common Patterns**:

```sql
-- ❌ INCORRECT: Account doesn't store transaction references
create association Finance.Account_Transaction
from Finance.Account to Finance.Transaction
type reference;

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
delete_behavior DELETE_BUT_KEEP_REFERENCES
comment 'Additional documentation';
```

**Association Types**:
- `reference` - One-to-one or many-to-one (foreign key on FROM entity)
- `ReferenceSet` - One-to-many or many-to-many (collection)

**Owner Options**:
- `default` - Standard ownership (FROM entity owns the reference)
- `both` - Both sides own the association (bidirectional)
- `Parent` - Only parent (TO) entity owns
- `Child` - Only child (FROM) entity owns

> **Use `default` ownership for a normal to-one reference.** Reserve `owner both`
> for a `ReferenceSet` (many-to-many). On a plain `type reference`, `owner both`
> makes the association navigable **to-one from *both* sides** — so the reverse
> direction is a single object, not a collection. A **list** widget (listview/
> datagrid/gallery) over the reverse then fails MxBuild **CE8812** "A grid
> association path must result in a list." (a DataView over the reverse is fine —
> it's a single object). With `default` ownership, the reverse is the expected
> to-many collection and a list widget works. See master-detail-pages.md for the
> widget patterns.

**Delete Behaviors**:
- `DELETE_AND_REFERENCES` - Delete object and all referencing objects
- `DELETE_BUT_KEEP_REFERENCES` - Delete object, keep references (nullify)
- `DELETE_IF_NO_REFERENCES` - Only delete if no objects reference it
- `cascade` - Cascade delete to associated objects
- `prevent` - Prevent deletion if references exist

**Naming Convention**: `{FromEntity}_{ToEntity}` (e.g., `Order_Customer`, `Transaction_Account`)

#### Calculated Attributes

Calculated attributes derive their value from a microflow at runtime. Use `calculated by Module.Microflow` to specify the calculation microflow.

**IMPORTANT: CALCULATED attributes are only supported on PERSISTENT entities.** Using CALCULATED on non-persistent entities will produce a validation error.

```sql
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

### Data Types

| Type | Example | Description |
|------|---------|-------------|
| `string(length)` | `string(200)` | Text field with max length |
| `integer` | `integer` | 32-bit integer |
| `long` | `long` | 64-bit integer (use for IDs) |
| `decimal` | `decimal` | Decimal number |
| `boolean` | `boolean` | True/false |
| `datetime` | `datetime` | Date and time |
| `date` | `date` | Date only |
| `binary` | `binary` | Binary data |
| `autonumber` | `autonumber default 1` | Auto-incrementing number (requires DEFAULT start value) |
| `enumeration(Module.Enum)` | `enumeration(Shop.Status)` | Enumeration reference |

### Constraints

**Basic Constraints:**
- `not null` - Field is required
- `unique` - Value must be unique
- `default value` - Default value

**Validation Error Messages:**

Each constraint can have a custom error message using `error 'message'` syntax:

```sql
create persistent entity Module.Customer (
  /** Customer name - required with custom error */
  Name: string(200) not null error 'Name is required',
  /** Email - required and unique with separate error messages */
  Email: string(200) not null error 'Email is required' unique error 'Email must be unique',
  /** Age with default value */
  Age: integer default 0,
  /** Active status flag */
  IsActive: boolean not null error 'IsActive flag is required' default true
);
```

**Error Message Guidelines:**
- Place `error 'message'` immediately after the constraint
- Multiple constraints can each have their own error message
- Keep messages clear and user-friendly
- Follow the pattern: `not null error 'X is required'` for required fields
- For UNIQUE: `unique error 'X must be unique'`
- Error messages are shown to end users during validation

**Common patterns:**
```sql
-- Required field
Name: string(200) not null error 'Name is required',

-- Required and unique
Email: string(200) not null error 'Email is required' unique error 'Email must be unique',

-- Required with default
IsActive: boolean not null error 'IsActive flag is required' default true,

-- Enum with required error
status: enumeration(Module.Status) not null error 'Status is required',

-- Enum with default value (use fully qualified Module.Enum.Value)
Priority: enumeration(Module.Priority) default Module.Priority.Normal
```

## Reserved Keywords

**Best practice: Always quote all identifiers** (entity names, attribute names) with double quotes. This escapes every **MDL parser** keyword conflict — quotes are stripped automatically by the parser. So `"create"`, `"status"`, `"end"` become valid attribute names.

```sql
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
> instead), plus `ID`, `GUID`, `CurrentUser` and the Java-keyword list. `"Type": String`
> fails MDL021 — rename to `ResourceType` / `TypeValue`.

Both `"Name"` and `` `Name` `` syntax are supported. Prefer double quotes for consistency.

**Boolean attributes** auto-default to `false` when no `default` is specified:
```sql
create persistent entity Module.Item (
  IsActive: boolean,           -- auto-defaults to false
  IsPublished: boolean default true
);
```

## Entity Positioning

Use `@position(x, y)` to control layout in Studio Pro:
- Place related entities near each other
- Use consistent spacing (e.g., 250 pixels horizontal, 200 vertical)
- Group by domain concept

Example layout:
```sql
@position(50, 50)      -- Top-left: Core entity
create persistent entity Module.Customer (...);

@position(300, 50)     -- Same row: Related entity
create persistent entity Module.Address (...);

@position(50, 250)     -- Below: Dependent entity
create persistent entity Module.Order (...);
```

## Documentation Best Practices

### Entity Documentation

```sql
/**
 * Brief one-line summary
 *
 * Detailed multi-line description explaining:
 * - What the entity represents
 * - Key business rules
 * - Relationships to other entities
 *
 * @since 1.0.0
 * @see Module.RelatedEntity
 */
```

### Attribute Documentation

```sql
/** Brief description of what this attribute stores */
attributename: type,
```

### Association Documentation

```sql
/**
 * Relationship description
 *
 * Explains the business meaning of this association.
 *
 * @since 1.0.0
 */
```

## Step-by-Step Process

### 1. Analyze Requirements

When user requests a domain model:
1. Identify core entities (nouns)
2. Identify enumerations (status, types, categories)
3. Identify relationships (associations)
4. Identify attributes for each entity
5. Check for reserved keyword conflicts

### 2. Generate MDL Script

Create script with this structure:
```sql
-- ============================================================================
-- Domain Model Name
-- ============================================================================
-- Description of the domain
-- ============================================================================

-- MARK: ENUMERATIONS

create enumeration Module.Enum1 (...);
create enumeration Module.Enum2 (...);

-- MARK: CORE ENTITIES

-- MARK: - Entity Group 1

create persistent entity Module.Entity1 (...);
create persistent entity Module.Entity2 (...);

-- MARK: - Entity Group 2

create persistent entity Module.Entity3 (...);

-- MARK: VIEW ENTITIES

create view entity Module.View1 as ...;

-- MARK: ASSOCIATIONS

-- MARK: - Entity Group 1 Associations

create association Module.Assoc1 ...;
create association Module.Assoc2 ...;
```

### 3. Validate with Linter

Run the linter to check for issues:

```bash
# Standalone test
node dist/test-linter-standalone.js

# or create a custom test file
```

The linter will detect:
- ✅ Reserved keywords (CE7247)
- ✅ Duplicate names (CE0065)
- ✅ OQL syntax errors (CE0174)

### 4. Review and Fix Issues

**Common Issues**:

1. **Reserved Keyword Error**:
   ```
   error: Reserved keyword 'CreatedDate' used as attribute name
   💡 rename to 'CreationDate'
   ```
   Fix: Rename to suggested alternative

2. **Duplicate Name Error**:
   ```
   error: Duplicate name 'Status' in module 'Shop'
   💡 rename one of the enumeration, entity to avoid conflict
   ```
   Fix: Rename entity to `OrderStatus` or similar

3. **OQL Syntax Error**:
   ```
   error: ORDER by requires limit or offset
   💡 add limit clause to query
   ```
   Fix: Add `limit 100` to view entity query

### 5. Generate Complete Script

Ensure:
- ✅ All entities have JavaDoc documentation
- ✅ All attributes have inline comments
- ✅ All associations have descriptions
- ✅ Position annotations for all entities
- ✅ No reserved keywords
- ✅ No duplicate names
- ✅ Valid OQL queries

## Example: E-Commerce Domain Model

```sql
-- ============================================================================
-- E-Commerce Domain Model
-- ============================================================================

create module ECommerce;

-- Enumerations
-- ============================================================================

/**
 * Order status enumeration
 *
 * @since 1.0.0
 */
create enumeration ECommerce.OrderStatus (
  Draft 'Draft',
  Submitted 'Submitted',
  Paid 'Paid',
  Shipped 'Shipped',
  Delivered 'Delivered',
  Cancelled 'Cancelled'
);

-- Entities
-- ============================================================================

-- Customer Management
-- ----------------------------------------------------------------------------

/**
 * Customer entity
 *
 * Stores customer information for e-commerce platform.
 *
 * @since 1.0.0
 * @see ECommerce.SalesOrder
 */
@position(50, 50)
create persistent entity ECommerce.Customer (
  /** Unique customer identifier */
  CustomerId: long not null error 'Customer ID is required' unique error 'Customer ID must be unique',
  /** Customer full name */
  FullName: string(200) not null error 'Full name is required',
  /** Email address */
  Email: string(200) not null error 'Email is required' unique error 'Email must be unique',
  /** Registration date */
  RegistrationDate: datetime not null error 'Registration date is required'
);

/**
 * Product entity
 *
 * Catalog of products available for purchase.
 *
 * @since 1.0.0
 */
@position(50, 250)
create persistent entity ECommerce.Product (
  /** Unique product identifier */
  ProductId: long not null error 'Product ID is required' unique error 'Product ID must be unique',
  /** Product name */
  ProductName: string(200) not null error 'Product name is required',
  /** Product SKU */
  SKU: string(50) not null error 'SKU is required' unique error 'SKU must be unique',
  /** Unit price */
  Price: decimal not null error 'Price is required',
  /** Stock quantity */
  StockQuantity: integer not null error 'Stock quantity is required'
);

/**
 * Sales order entity
 *
 * Customer orders for products.
 *
 * @since 1.0.0
 */
@position(300, 150)
create persistent entity ECommerce.SalesOrder (
  /** Unique order identifier */
  OrderId: long not null error 'Order ID is required' unique error 'Order ID must be unique',
  /** Order number */
  OrderNumber: string(50) not null error 'Order number is required' unique error 'Order number must be unique',
  /** Order date */
  OrderDate: datetime not null error 'Order date is required',
  /** Total amount */
  TotalAmount: decimal not null error 'Total amount is required',
  /** Order status */
  status: enumeration(ECommerce.OrderStatus) not null error 'Status is required'
);

-- Associations
-- ============================================================================

/**
 * Customer orders
 *
 * Links customers to their orders.
 *
 * @since 1.0.0
 */
create association ECommerce.Customer_Orders
from ECommerce.Customer to ECommerce.SalesOrder
type ReferenceSet
owner both;
```

## Testing the Script

1. **Save to file**: Save as `examples/my-domain-model.mdl`

2. **Run standalone linter**:
   ```bash
   node dist/test-linter-standalone.js
   ```

3. **Execute in REPL**:
   ```sql
   mendix> connect to FILESYSTEM 'path/to/project.mpr';
   mendix> execute script 'examples/my-domain-model.mdl';
   ```

4. **Check Studio Pro**: Open project and verify entities appear correctly

## Common Patterns

### One-to-Many Relationship
```sql
-- Parent entity
create persistent entity Module.Parent (Id: long not null unique);

-- Child entity
create persistent entity Module.Child (
  Id: long not null unique,
  ChildData: string(200)
);

-- Association (Parent has many Children)
create association Module.Parent_Children
from Module.Parent to Module.Child
type ReferenceSet
owner both;
```

### Many-to-Many Relationship
```sql
-- Entity A
create persistent entity Module.EntityA (Id: long not null unique);

-- Entity B
create persistent entity Module.EntityB (Id: long not null unique);

-- Bidirectional association
create association Module.EntityA_EntityB
from Module.EntityA to Module.EntityB
type ReferenceSet
owner both;
```

### Hierarchical Relationship (Self-Reference)

**IMPORTANT: Self-referencing associations must use `owner default`** (one-to-many). Using `owner both` is not supported for self-references.

```sql
/**
 * Category with parent-child hierarchy
 */
create persistent entity Module.Category (
  Id: long not null unique,
  CategoryName: string(200) not null
);

/**
 * Parent category link (self-reference)
 */
create association Module.Category_ParentCategory
from Module.Category to Module.Category
type reference
owner default;
```

### ALTER ENTITY (Incremental Modifications)

Use `alter entity` to make targeted changes to existing entities without redefining the entire entity:

```sql
-- Add a new attribute
alter entity Module.Customer
  add attribute PhoneNumber: string(20);

-- Add multiple attributes at once
alter entity Module.Order
  add attribute VATRate: decimal
  add attribute VATAmount: decimal;

-- Rename an attribute (preserves data)
alter entity Module.Order
  rename attribute CreatedDate to OrderDate;

-- Drop an attribute
alter entity Module.Product
  drop attribute LegacyCode;

-- Modify attribute type
alter entity Module.Customer
  modify attribute Address: string(500);

-- Set entity documentation
alter entity Module.Customer
  set documentation 'Core customer entity for CRM module';

-- Add an index
alter entity Module.Customer
  add index idx_email (Email asc);

-- Reposition entity on domain model canvas
alter entity Module.Customer
  set position (100, 200);
```

**Supported operations:** ADD ATTRIBUTE, RENAME ATTRIBUTE, MODIFY ATTRIBUTE, DROP ATTRIBUTE, SET DOCUMENTATION, SET COMMENT, ADD INDEX, DROP INDEX, SET POSITION.

### Entity Positioning Guidelines

When creating or repositioning entities, follow these layout rules for readable domain models:

- **Horizontal spacing:** 350px between columns (x = 50, 400, 750, 1100, ...)
- **Vertical spacing:** calculate per-column based on the entity above: `y = previous_y + 50 + (previous_entity_attribute_count * 20)`
- Entity header is ~40px, each attribute adds ~20px of height, plus ~50px padding
- **Position column-by-column**, not in rigid rows — avoids wasting space when entities have different attribute counts
- **Place related entities** in the same column or adjacent columns so associations are short

Example layout for entities with varying attribute counts:

```
column 1 (x=50):          column 2 (x=400):
  entity A (4 attrs)        entity C (14 attrs)
  y=50                      y=50

  entity B (10 attrs)       entity D (3 attrs)
  y=180 (50+50+4*20)        y=380 (50+50+14*20)
```

```sql
-- Position entities after creation
alter entity Module.EntityA set position (50, 50);
alter entity Module.EntityB set position (50, 180);
alter entity Module.EntityC set position (400, 50);
alter entity Module.EntityD set position (400, 380);
```

### Entity Migration with CREATE OR MODIFY

Use `create or modify` to update existing entities without losing data. The REPL computes differences and applies incremental changes.

```sql
/**
 * Customer entity migration - rename CustomerName to FullName
 */
create or modify persistent entity Module.Customer (
  /** Unique identifier (unchanged) */
  CustomerId: long not null unique,

  /** Renamed from CustomerName - data preserved */
  @RenamedFrom('CustomerName')
  FullName: string(200) not null,

  /** New field */
  Email: string(255) unique,

  /** Type widened from String(100) to String(200) */
  Address: string(200)
);
```

**Key features:**
- `@RenamedFrom('oldName')` - renames attribute, preserves data
- Auto-removes attributes not in new definition
- Allows compatible type changes (e.g., String length increase)
- Preserves entity UUID (no data loss)

### Status-Driven Entity
```sql
-- Status enumeration
create enumeration Module.TaskStatus (
  Todo 'To Do',
  InProgress 'In Progress',
  Done 'Done'
);

-- Entity with status
create persistent entity Module.Task (
  Id: long not null unique,
  TaskName: string(200) not null,
  status: enumeration(Module.TaskStatus) not null
);
```

## Checklist

Before finalizing an MDL script:

- [ ] All entities have JavaDoc documentation
- [ ] All attributes have inline comments
- [ ] All associations have descriptions
- [ ] Position annotations on all entities
- [ ] MARK comments for files 300+ lines (at least 3 sections)
- [ ] All identifiers quoted with double quotes
- [ ] No duplicate names (run linter)
- [ ] Valid OQL queries in view entities (run linter)
- [ ] Consistent naming conventions (PascalCase)
- [ ] Appropriate data types and lengths
- [ ] Required fields marked with NOT NULL
- [ ] Validation error messages added for NOT NULL and UNIQUE constraints
- [ ] IDs marked with NOT NULL UNIQUE
- [ ] Email/unique fields marked with UNIQUE

## References

- **Reserved Keywords**: `packages/mendix-repl/docs/reference/reserved-keywords.md`
- **Linter Proposal**: `packages/mendix-repl/docs/proposals/mdl-linter-proposal.md`
- **Example Scripts**:
  - `packages/mendix-repl/examples/shop-domain-model.mdl`
  - `packages/mendix-repl/examples/pet-store-domain-model.mdl`
- **Linter Test**: `packages/mendix-repl/src/test-linter-standalone.ts`

## Tips for AI Assistants

1. **Always quote all identifiers** with double quotes to avoid MDL parser keyword conflicts — but note quoting does **not** exempt platform-reserved member names (`Type`, `CreatedDate`, `ChangedDate`, `Owner`, `ChangedBy`, `ID`, …); rename those
2. **Use descriptive names** (ServiceType, CustomerOrder)
3. **Run linter** on generated scripts before presenting to user
4. **Fix all errors** reported by linter before finalizing
5. **Follow examples** in shop-domain-model.mdl and pet-store-domain-model.mdl
6. **Document thoroughly** - Studio Pro users benefit from good documentation
7. **Position thoughtfully** - Related entities should be visually grouped
8. **Test incrementally** - Generate in sections and validate each part
