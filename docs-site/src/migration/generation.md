# Phase 3: Generation

With the transformation plan in hand, the agent generates the Mendix application using MDL scripts. Skills guide the agent toward correct, idiomatic MDL.

## Key Skills

| Skill | Purpose |
|-------|---------|
| `generate-domain-model` | Entity, association, and enumeration syntax with naming conventions |
| `write-microflows` | Microflow syntax, 60+ activity types, common patterns |
| `create-page` | Page and widget syntax for 50+ widget types |
| `overview-pages` | CRUD page patterns (list + detail) |
| `master-detail-pages` | Master-detail page layouts |
| `manage-security` | Module roles, user roles, GRANT/REVOKE, demo users |
| `manage-navigation` | Navigation profiles, menu items, home pages |
| `organize-project` | Folder structure, MOVE command, project conventions |

## Generation Workflow

```bash
# 1. Create a new Mendix project in Studio Pro (or use an existing one)
# 2. Execute MDL scripts in dependency order
mxcli exec domain-model.mdl -p app.mpr
mxcli exec microflows.mdl -p app.mpr
mxcli exec pages.mdl -p app.mpr
mxcli exec security.mdl -p app.mpr

# Or work interactively in the REPL
mxcli -p app.mpr
```

## Example: Domain Model

```sql
mdl 1;
-- Enumerations first (referenced by entities)
CREATE ENUMERATION Sales.OrderStatus (
  Draft 'Draft',
  Pending 'Pending',
  Confirmed 'Confirmed',
  Shipped 'Shipped',
  Delivered 'Delivered',
  Cancelled 'Cancelled'
);

-- Entities
/** Customer master data */
@Position(100, 100)
CREATE PERSISTENT ENTITY CRM.Customer (
  Name: String(200) NOT NULL ERROR MESSAGE 'Customer name is required',
  Email: String(200) UNIQUE ERROR MESSAGE 'Email already exists',
  Phone: String(50),
  IsActive: Boolean DEFAULT TRUE
)
INDEX (Name)
INDEX (Email);

/** Sales order */
@Position(300, 100)
CREATE PERSISTENT ENTITY Sales.Order (
  OrderNumber: String(50) NOT NULL UNIQUE,
  OrderDate: DateTime NOT NULL,
  TotalAmount: Decimal DEFAULT 0,
  Status: Enumeration(Sales.OrderStatus) DEFAULT 'Draft'
)
INDEX (OrderNumber)
INDEX (OrderDate DESC);

-- Associations
CREATE ASSOCIATION Sales.Order_Customer
  FROM Sales.Order TO CRM.Customer
  TYPE Reference OWNER Default;

```

## Example: Microflow

```sql
CREATE MICROFLOW Sales.ACT_Order_CalculateTotal
BEGIN
  DECLARE $Order Sales.Order;
  RETRIEVE $Lines FROM Sales.OrderLine
    WHERE [Sales.OrderLine_Order = $Order];
  DECLARE $Total Decimal = 0;
  LOOP $Line IN $Lines
  BEGIN
    SET $Total = $Total + $Line/Price * $Line/Quantity;
  END;
  CHANGE $Order (TotalAmount = $Total);
  COMMIT $Order;
END;
/
```

## Example: Page

```sql
CREATE PAGE CRM.Customer_Overview (
  Title: 'Customers',
  Layout: Atlas_Core.Atlas_Default
) {
  DATAGRID dgCustomers (DataSource: DATABASE CRM.Customer, Selection: Single) {
    COLUMN (Attribute: Name, Caption: 'Name') { TEXTFILTER fName }
    COLUMN (Attribute: Email, Caption: 'Email') { TEXTFILTER fEmail }
    COLUMN (Attribute: Phone, Caption: 'Phone')
    COLUMN (Attribute: IsActive, Caption: 'Active')
    CONTROLBAR {
      ACTIONBUTTON btnNew (Caption: 'New', Action: SHOW PAGE CRM.Customer_NewEdit, ButtonStyle: Primary)
    }
  }
};

```

## Validation Between Steps

Validate after each script to catch errors early:

```bash
# Syntax check (fast, no project used)
mxcli check domain-model.mdl --no-project

# Reference validation (checks entity/microflow names exist)
mxcli check pages.mdl -p app.mpr --references
```
