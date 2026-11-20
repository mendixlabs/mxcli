# CREATE ENTITY

## Synopsis

    CREATE [ OR MODIFY ] entity_type ENTITY module.name
        [ EXTENDS parent_module.parent_entity ]
    (
        attribute_name : data_type [ NOT LOCALIZED | LOCALIZED ]
                                   [ NOT NULL [ ERROR 'message' ] ]
                                   [ UNIQUE [ ERROR 'message' ] ]
                                   [ DEFAULT value ]
                                   [ CALCULATED [ BY module.microflow ] ]
        [, ...]
    )
    [ INDEX ( column [ ASC | DESC ] [, ...] ) ]
    [, ...]

    CREATE VIEW ENTITY module.name (
        attribute_name : data_type [, ...]
    ) AS
        oql_query

Where *entity_type* is one of:

    PERSISTENT
    NON-PERSISTENT
    EXTERNAL

## Description

`CREATE ENTITY` adds a new entity to a module's domain model. The entity type determines how instances are stored:

- **PERSISTENT** entities are backed by a database table. This is the most common type and is required for data that must survive a server restart.
- **NON-PERSISTENT** entities exist only in memory for the duration of a user session. They are commonly used for search filters, wizard state, and transient view models.
- **VIEW** entities are read-only entities backed by an OQL query. They appear in the domain model but do not have their own database table.
- **EXTERNAL** entities are sourced from an external service (e.g., OData). See `CREATE EXTERNAL ENTITY` for the full syntax.

If `OR MODIFY` is specified, the statement is idempotent: if an entity with the same qualified name already exists, its attributes are updated to match the definition. New attributes are added, and existing attributes have their types and constraints updated. This is useful for repeatable scripts.

The optional `EXTENDS` clause establishes generalization (inheritance). The child entity inherits all attributes from the parent and can add its own. Common parent entities include `System.User`, `System.FileDocument`, and `System.Image`. The `EXTENDS` clause must appear **before** the opening parenthesis. A specialization inherits its persistability from the root of the generalization chain, which is the only place Mendix stores it: `PERSISTENT` or `NON-PERSISTENT` on it may restate the inherited value, and one that contradicts it is an error from both `check` and `exec`. Leave the keyword out to take whatever the chain says.

Each attribute definition specifies a name, a data type, and optional constraints. Attributes are separated by commas. The supported data types are:

| Type | Syntax | Notes |
|------|--------|-------|
| String | `String(length)` | Length in characters; use `String(unlimited)` for unbounded |
| Integer | `Integer` | 32-bit signed integer |
| Long | `Long` | 64-bit signed integer |
| Decimal | `Decimal` | Arbitrary-precision decimal |
| Boolean | `Boolean` | `TRUE` or `FALSE`; defaults to `FALSE` when no `DEFAULT` is given |
| DateTime | `DateTime` | Date and time combined |
| AutoNumber | `AutoNumber` | Auto-incrementing integer (persistent entities only) |
| Binary | `Binary` | Binary data |
| HashedString | `HashedString` | One-way hashed string (for passwords) |
| Enumeration | `Enumeration(Module.EnumName)` | Reference to an enumeration type |

Documentation comments (`/** ... */`) placed immediately before the entity or an individual attribute are preserved as documentation in the domain model.

The `@Position(x, y)` annotation controls the entity's visual position in the domain model diagram in Mendix Studio Pro.

One or more `INDEX` clauses may follow the attribute list to create database indexes. Each index lists one or more columns with an optional sort direction (`ASC` or `DESC`, defaulting to `ASC`).

## Parameters

**OR MODIFY**
: Makes the statement idempotent. If the entity already exists, its definition is updated to match. New attributes are added and existing attributes are modified. Without this clause, creating a duplicate entity is an error.

**entity_type**
: One of `PERSISTENT`, `NON-PERSISTENT`, `VIEW`, or `EXTERNAL`. Determines how entity instances are stored. `PERSISTENT` is the most common choice.

**module.name**
: The qualified name of the entity in the form `Module.EntityName`. The module must already exist.

**EXTENDS parent_module.parent_entity**
: Establishes generalization (inheritance). The new entity inherits all attributes from the parent. Must appear before the opening parenthesis.

**attribute_name**
: The name of an attribute. Must be a valid identifier and unique within the entity.

**data_type**
: The attribute's data type. See the table above for all supported types. `String` requires an explicit length argument.

**NOT LOCALIZED | LOCALIZED**
: DateTime only: Studio Pro's **Localize** setting (`LocalizeDate`). `NOT LOCALIZED` stores and shows the value without time-zone conversion (a calendar date such as a birth date); `LOCALIZED` states the default. On a new attribute no clause means localized; with `OR MODIFY`, no clause keeps the stored value and a stated one wins. On any other type it is an error. See [Constraints](../../language/constraints.md#localized--not-localized-datetime-only).

**NOT NULL**
: Marks the attribute as required. An optional `ERROR 'message'` clause provides a custom validation message shown when the constraint is violated.

**UNIQUE**
: Marks the attribute as having a uniqueness constraint. An optional `ERROR 'message'` clause provides a custom validation message.

**DEFAULT value**
: Sets the default value assigned to the attribute when a new object is created. String defaults are quoted (`'value'`), numeric defaults are bare (`0`, `3.14`), and boolean defaults are `TRUE` or `FALSE`.

**CALCULATED**
: Marks the attribute as calculated (not stored in the database). The attribute's value is derived at runtime. Use `CALCULATED BY Module.Microflow` to specify the microflow that computes the value.

**INDEX ( column [, ...] )**
: Creates a database index on the listed columns. Each column may have an optional `ASC` or `DESC` sort direction. Multiple `INDEX` clauses create multiple indexes.

## Examples

### Basic persistent entity

```sql
CREATE PERSISTENT ENTITY Sales.Product (
    Name: String(200) NOT NULL,
    Price: Decimal DEFAULT 0,
    InStock: Boolean DEFAULT TRUE
);
```

### Entity with all constraint types

```sql
/** Customer master data */
@Position(100, 200)
CREATE PERSISTENT ENTITY Sales.Customer (
    /** Unique customer identifier */
    CustomerId: AutoNumber NOT NULL UNIQUE DEFAULT 1,

    /** Customer full name */
    Name: String(200) NOT NULL ERROR MESSAGE 'Name is required',

    Email: String(200) UNIQUE ERROR MESSAGE 'Email must be unique',

    Balance: Decimal DEFAULT 0,

    IsActive: Boolean DEFAULT TRUE,

    CreatedDate: AutoCreatedDate,

    Status: Enumeration(Sales.CustomerStatus) DEFAULT 'Active'
)
INDEX (Name)
INDEX (Email);
```

### Entity with generalization (EXTENDS)

```sql
/** Employee extends the system user entity */
CREATE PERSISTENT ENTITY HR.Employee EXTENDS System.User (
    EmployeeNumber: String(20) NOT NULL UNIQUE,
    Department: String(100),
    HireDate: DateTime
);
```

### File and image entities

```sql
mdl 1;
CREATE PERSISTENT ENTITY Catalog.ProductPhoto EXTENDS System.Image (
    Caption: String(200),
    SortOrder: Integer DEFAULT 0
);

CREATE PERSISTENT ENTITY Docs.Attachment EXTENDS System.FileDocument (
    Description: String(500)
);
```

### Non-persistent entity (search filter)

```sql
CREATE NON-PERSISTENT ENTITY Sales.CustomerFilter (
    SearchName: String(200),
    MinBalance: Decimal,
    MaxBalance: Decimal,
    IncludeInactive: Boolean DEFAULT FALSE
);
```

### Idempotent script with OR MODIFY

```sql
CREATE OR MODIFY PERSISTENT ENTITY Sales.Customer (
    CustomerId: AutoNumber NOT NULL UNIQUE DEFAULT 1,
    Name: String(200) NOT NULL,
    Email: String(200),
    Phone: String(50)
);
```

If `Sales.Customer` already exists, the `Phone` attribute is added and existing attributes are updated. If it does not exist, it is created.

### View entity with OQL query

```sql
CREATE VIEW ENTITY Reports.CustomerSummary (
    CustomerName: String,
    TotalOrders: Integer,
    TotalAmount: Decimal
) AS
    SELECT
        c.Name AS CustomerName,
        COUNT(o.OrderId) AS TotalOrders,
        SUM(o.Amount) AS TotalAmount
    FROM Sales.Customer c
    LEFT JOIN Sales.Order o ON o.Customer = c
    GROUP BY c.Name;
```

### Entity with calculated attribute

```sql
CREATE PERSISTENT ENTITY Sales.Order (
    OrderNumber: String(50) NOT NULL UNIQUE,
    Subtotal: Decimal DEFAULT 0,
    TaxRate: Decimal DEFAULT 0,
    TotalAmount: Decimal CALCULATED BY Sales.CalcOrderTotal
);
```

### Entity with composite index

```sql
CREATE PERSISTENT ENTITY Sales.Order (
    OrderId: AutoNumber NOT NULL UNIQUE DEFAULT 1,
    OrderNumber: String(50) NOT NULL UNIQUE,
    CustomerId: Long,
    OrderDate: DateTime,
    Status: Enumeration(Sales.OrderStatus)
)
INDEX (OrderNumber)
INDEX (CustomerId, OrderDate DESC)
INDEX (Status);
```

### Entity with documentation

```sql
/**
 * Represents a customer order.
 * Orders track purchases from initial draft through delivery.
 */
@Position(300, 100)
CREATE PERSISTENT ENTITY Sales.Order (
    /** Auto-generated order identifier */
    OrderId: AutoNumber NOT NULL UNIQUE DEFAULT 1,

    /** Human-readable order number */
    OrderNumber: String(50) NOT NULL UNIQUE,

    OrderDate: DateTime NOT NULL,
    TotalAmount: Decimal DEFAULT 0,
    Status: Enumeration(Sales.OrderStatus) DEFAULT 'Draft',
    Notes: String(unlimited)
)
INDEX (OrderNumber)
INDEX (OrderDate DESC);
```

## Notes

- `String` requires an explicit length: `String(200)`. Use `String(unlimited)` for unbounded text.
- `EXTENDS` must appear **before** the opening parenthesis, not after the closing one.
- A member name must be unique along the generalization chain: a specialization cannot declare an attribute or own an association named like a member it inherits (from `System.User` and the other System entities too), and a generalization cannot gain one a specialization already has. An entity's attributes and associations share that name space; names differing only in case do not collide. `mxcli check` refuses it (MDL-MEMBER01); Mendix reports it as CE0069 "Duplicate member name".
- Boolean attributes without an explicit `DEFAULT` automatically default to `FALSE`.
- `AutoNumber` attributes are only valid on persistent entities.
- The `@Position` annotation is optional and only affects the visual layout in Mendix Studio Pro.
- Every statement ends with `;`.

## See Also

[ALTER ENTITY](alter-entity.md), [DROP ENTITY](drop-entity.md), [CREATE ASSOCIATION](create-association.md), [CREATE ENUMERATION](create-enumeration.md)
