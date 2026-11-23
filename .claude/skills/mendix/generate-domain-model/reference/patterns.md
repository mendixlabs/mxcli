# Domain model patterns and a worked example

Supporting reference for [generate-domain-model](../SKILL.md).

## Example: E-Commerce Domain Model

```sql
mdl 1;
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
create persistent entity ECommerce.Customer (
  /** Unique customer identifier */
  CustomerId: long not null error message 'Customer ID is required' unique error message 'Customer ID must be unique',
  /** Customer full name */
  FullName: string(200) not null error message 'Full name is required',
  /** Email address */
  Email: string(200) not null error message 'Email is required' unique error message 'Email must be unique',
  /** Registration date */
  RegistrationDate: datetime not null error message 'Registration date is required'
);

/**
 * Product entity
 *
 * Catalog of products available for purchase.
 *
 * @since 1.0.0
 */
create persistent entity ECommerce.Product (
  /** Unique product identifier */
  ProductId: long not null error message 'Product ID is required' unique error message 'Product ID must be unique',
  /** Product name */
  ProductName: string(200) not null error message 'Product name is required',
  /** Product SKU */
  SKU: string(50) not null error message 'SKU is required' unique error message 'SKU must be unique',
  /** Unit price */
  Price: decimal not null error message 'Price is required',
  /** Stock quantity */
  StockQuantity: integer not null error message 'Stock quantity is required'
);

/**
 * Sales order entity
 *
 * Customer orders for products.
 *
 * @since 1.0.0
 */
create persistent entity ECommerce.SalesOrder (
  /** Unique order identifier */
  OrderId: long not null error message 'Order ID is required' unique error message 'Order ID must be unique',
  /** Order number */
  OrderNumber: string(50) not null error message 'Order number is required' unique error message 'Order number must be unique',
  /** Order date */
  OrderDate: datetime not null error message 'Order date is required',
  /** Total amount */
  TotalAmount: decimal not null error message 'Total amount is required',
  /** Order status */
  status: enumeration(ECommerce.OrderStatus) not null error message 'Status is required'
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

No `@position` anywhere: after `./mxcli exec`, run
`./mxcli layout -p app.mpr --module ECommerce` to arrange the diagram from the
associations (see SKILL.md, step 5).

## Common Patterns

### One-to-Many Relationship
```sql
mdl 1;
-- Parent entity
create persistent entity Module.Parent (Number: long not null unique);

-- Child entity
create persistent entity Module.Child (
  Number: long not null unique,
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
mdl 1;
-- Entity A
create persistent entity Module.EntityA (Number: long not null unique);

-- Entity B
create persistent entity Module.EntityB (Number: long not null unique);

-- Bidirectional association
create association Module.EntityA_EntityB
from Module.EntityA to Module.EntityB
type ReferenceSet
owner both;
```

### Hierarchical Relationship (Self-Reference)

**IMPORTANT: Self-referencing associations must use `owner default`** (one-to-many). Using `owner both` is not supported for self-references.

```sql
mdl 1;
/**
 * Category with parent-child hierarchy
 */
create persistent entity Module.Category (
  Number: long not null unique,
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
mdl 1;
-- Add a new attribute
alter entity Module.Customer
  add attribute PhoneNumber: string(20);

-- Add multiple attributes at once
alter entity Module.Order
  add attribute VATRate: decimal
  add attribute VATAmount: decimal;

-- Rename an attribute (preserves data). Every stored reference follows it:
-- microflow create/change members, page attribute widgets, validation rules,
-- access rules -- and XPath constraints too ([CreatedDate > ...]), including
-- ones that reach the entity through an association. Microflow expressions
-- ($Order/CreatedDate) are NOT rewritten -- mxbuild reports those as CE0117,
-- so build afterwards.
alter entity Module.Order
  rename attribute CreatedDate to OrderDate;

-- Drop an attribute
alter entity Module.Product
  drop attribute LegacyCode;

-- Modify attribute type
alter entity Module.Customer
  modify attribute Address: string(500);

-- Modify attribute constraints. MODIFY applies the constraints you specify and
-- preserves the ones you don't:
--   NULLABLE   -> make a required attribute optional (removes NOT NULL)
--   NOT NULL   -> make an optional attribute required
--   UNIQUE     -> add a uniqueness constraint
--   DEFAULT x  -> set/replace the default
alter entity Module.Customer
  modify attribute Email: string(200) nullable;   -- Email is now optional
alter entity Module.Customer
  modify attribute Code: string(20) not null unique;

-- Set entity documentation
alter entity Module.Customer
  set documentation 'Core customer entity for CRM module';

-- Add an index
alter entity Module.Customer
  add index (Email asc);

-- Reposition entity on domain model canvas
alter entity Module.Customer
  set position (100, 200);
```

**Supported operations:** ADD ATTRIBUTE, RENAME ATTRIBUTE, MODIFY ATTRIBUTE (type + `NULLABLE`/`NOT NULL`/`UNIQUE`/`DEFAULT` constraints), DROP ATTRIBUTE, SET DOCUMENTATION (`SET COMMENT` is its deprecated alias, MDL-DEPR135), ADD INDEX, DROP INDEX, SET POSITION.

> **`MODIFY ATTRIBUTE` always takes a type** — restate it even when you only want
> to change a constraint. Its type slot accepts a bare qualified name, so a
> clause written in the type position is read as a type name:
> `MODIFY ATTRIBUTE X SET DEFAULT 0` treats `SET` as the type. mxcli now refuses
> that; before it did, the statement rewrote the attribute to an enumeration and
> produced a project Mendix could not open (#910).
>
> To clear a default value use **`DROP DEFAULT ON ATTRIBUTE <name>`**.

### Entity Positioning

Don't compute positions by hand. Run `./mxcli layout -p app.mpr --module <Module>`
after `exec`: it sizes each box from its attributes, keeps the boxes apart, and
puts related entities next to each other. Hand-written `@position` or
`alter entity … set position (x, y)` is for a few entities added to a module
someone arranged in Studio Pro, where `layout` would move the rest (SKILL.md,
step 5).

### Entity Migration with CREATE OR MODIFY

Use `create or modify` to update existing entities without losing data. The REPL computes differences and applies incremental changes.

```sql
mdl 1;
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
mdl 1;
-- Status enumeration
create enumeration Module.TaskStatus (
  Todo 'To Do',
  InProgress 'In Progress',
  Done 'Done'
);

-- Entity with status
create persistent entity Module.Task (
  Number: long not null unique,
  TaskName: string(200) not null,
  status: enumeration(Module.TaskStatus) not null
);
```
