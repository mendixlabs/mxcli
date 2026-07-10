# Overview Pages - CRUD Page Pattern

## Overview

Standard pattern for creating CRUD (Create, Read, Update, Delete) pages in Mendix using MDL syntax. This pattern consists of:

1. **Navigation Snippet** - Reusable menu for consistent navigation
2. **Overview Page** - Lists all objects with a DataGrid and navigation snippet
3. **NewEdit Page** - Form for creating/editing a single object

## Pattern Summary

| Component | Type | Purpose | Key Widgets |
|-----------|------|---------|-------------|
| `Entity_Menu` | Snippet | Vertical sidebar navigation | NAVIGATIONLIST with ITEM actions |
| `Entity_Overview` | Page | List all records | SNIPPETCALL (sidebar), DATAGRID, Heading |
| `Entity_NewEdit` | Page | Create/Edit form | DataView, Input widgets, Save/Cancel |

## Navigation Menu Snippet

Create a reusable navigation snippet using NAVIGATIONLIST for vertical sidebar menus:

```sql
create snippet Module.Entity_Menu
{
  navigationlist navMenu {
    item itemCustomers (caption: 'Customers', action: show_page Module.Customer_Overview)
    item itemOrders (caption: 'Orders', action: show_page Module.Order_Overview)
    item itemProducts (caption: 'Products', action: show_page Module.Product_Overview)
  }
}
```

### Snippet Syntax

```sql
create [or replace] snippet Module.SnippetName
[(
  params: { $ParamName: Module.EntityType }
)]
[folder 'path']
{
  -- Widget definitions (same as pages)
}
```

### NAVIGATIONLIST Syntax

The NAVIGATIONLIST widget creates a vertical menu with navigation items:

```sql
navigationlist widgetName {
  item itemName (caption: 'Caption', action: show_page Module.PageName)
  item itemName (caption: 'Caption', action: microflow Module.MicroflowName)
  item itemName (caption: 'Caption', action: close_page)
}
```

## Overview Page Template

Lists all objects of an entity type with a data grid and navigation menu in a sidebar layout.

**Layout Structure:**
```
┌─────────────────────────────────────────────┐
│ layoutgrid                                  │
│ ┌────────┬──────────────────────────────────┤
│ │ COL 2  │ COL 10                           │
│ │ menu   │ Heading + datagrid               │
│ │snippet │                                  │
│ └────────┴──────────────────────────────────┤
└─────────────────────────────────────────────┘
```

```sql
create page Module.Entity_Overview
(
  title: 'Entity Overview',
  layout: Atlas_Core.Atlas_Default,
  folder: 'OverviewPages'
)
{
  layoutgrid mainGrid {
    row row1 {
      column colNav (desktopwidth: 2) {
        snippetcall navMenu (snippet: Module.Entity_Menu)
      }
      column colContent (desktopwidth: 10) {
        dynamictext heading (content: 'Entities', rendermode: H2)
        datagrid EntityGrid (
          datasource: database Module.Entity,
          selection: Multi,
          PagingPosition: both,
          designproperties: ['Compact': on, 'Hover': on, 'Striped': on]
        ) {
          column colName (attribute: Name, caption: 'Name') {
            textfilter textFilter1
          }
          column colDescription (attribute: Description, caption: 'Description') {
            textfilter textFilter2
          }
          column colActions (caption: 'Actions') {
            actionbutton btnEdit (caption: 'Edit', action: show_page Module.Entity_NewEdit("entity": $currentObject))
            actionbutton btnDelete (caption: 'Delete', action: delete, buttonstyle: danger)
          }
        }
      }
    }
  }
}
```

### SNIPPETCALL Syntax

Include a snippet in a page using SNIPPETCALL:

```sql
-- Simple snippet call
snippetcall widgetName (snippet: Module.SnippetName)

-- With parameters (for parameterized snippets):
snippetcall widgetName (snippet: Module.SnippetName, params: {Customer: $Customer})
```

### Overview Page Components

1. **Navigation Snippet**: `snippetcall` referencing `Module.NavigationMenu`
2. **Layout**: `Atlas_Core.Atlas_Default` - Full page with header/footer
3. **Heading**: `dynamictext` with `rendermode: H2`
4. **Data Grid**: `datagrid` with `datasource: database` binding

### DATAGRID Syntax

```sql
datagrid GridName (
  datasource: database from Module.Entity where [IsActive = true] sort by Name asc,
  selection: Multi,
  PagingPosition: both,
  designproperties: ['Compact': on, 'Hover': on, 'Striped': on]
) {
  column colName (attribute: Name, caption: 'Name') {
    textfilter textFilter1
  }
  column colActions (caption: 'Actions') {
    actionbutton btnEdit (caption: 'Edit', action: show_page Module.Entity_NewEdit("entity": $currentObject))
  }
}
```

**Properties:**
- `datasource: database from Module.Entity` - Entity data source (required)
- `where [condition]` - Optional XPath filter (inline after entity in DataSource)
- `sort by attr asc|desc` - Optional sorting (inline after WHERE: `sort by Name asc, Price desc`)
- `selection: Multi` - Multi-selection (`Multi`, `Single`, or omit for none)
- `PagingPosition: both` - Pagination bar position (`top`, `bottom`, `both`)
- `designproperties: ['Compact': on, 'Hover': on, 'Striped': on]` - Atlas design tokens

**Column Types:**
- `column colName (attribute: attribute, caption: 'label')` - Attribute column (own-entity attribute)
- `column colName (attribute: Assoc/Attr, caption: 'label')` - Associated-attribute column (attribute over a reference; bare association name, e.g. `attribute: Order_Customer/Name`; multi-hop `A/B/Attr` supported)
- `column colName (caption: 'label') { ... }` - Custom content column (nested widgets)

> **Custom-content columns build correctly** on the default engine (a nested
> `actionbutton`/`dynamictext` in `column (caption: …) { … }` — mxbuild-verified,
> 0 errors). An earlier CE0463 (column property ordering) was fixed. A row-level
> `onclick` (open the NewEdit page on row click) is still a fine alternative for a
> row-open affordance.

> **Reserved keyword column names:** If the attribute name is a reserved MDL keyword (e.g. `Status`, `Type`), you must quote it and use a distinct column widget name:
> ```sql
> column colStatus (attribute: "Status", caption: 'Status')
> column colType   (attribute: "Type",   caption: 'Type')
> ```
> Using `COLUMN Status (attribute: Status)` fails silently — the column won't sort or filter correctly because `Status` is parsed as a keyword. Always prefix the widget name (`colStatus`) when the attribute name is reserved.

**Column Properties (non-default only in DESCRIBE output):**

| Property | Values | Default |
|----------|--------|---------|
| `Sortable` | `true`/`false` | `true` (with attribute) |
| `Resizable` | `true`/`false` | `true` |
| `Draggable` | `true`/`false` | `true` |
| `Hidable` | `yes`/`hidden`/`no` | `yes` |
| `ColumnWidth` | `autofill`/`autoFit`/`manual` | `autofill` |
| `Size` | integer (px) | `1` (when manual) |
| `visible` | expression | `true` |
| `DynamicCellClass` | expression | (empty) |
| `tooltip` | text | (empty) |

### Column Filters (match the attribute's data type)

A filter widget must match the column attribute's type, or MxBuild fails with
*"The text filter is not compatible with the … data type"*. Do **not** apply
`textfilter` to every column — it only works on String attributes. Pick by type:

| Attribute type | Filter widget |
|----------------|---------------|
| String | `textfilter` |
| Integer / Long / Decimal / Autonumber | `numberfilter` |
| Date and time | `datefilter` |
| Enumeration | `dropdownfilter` |
| Boolean | *(no filter — every filter widget errors on Boolean)* |

```sql
column colName   (attribute: Name)      { textfilter f1 }      -- String
column colQty    (attribute: Quantity)  { numberfilter f2 }    -- Integer/Decimal
column colDate   (attribute: OrderDate) { datefilter f3 }      -- Date and time
column colStatus (attribute: "Status")  { dropdownfilter f4 }  -- Enumeration
-- Boolean columns: omit the filter entirely
```

## NewEdit Page Template

Form for creating or editing a single entity. **Requires a page parameter** to receive the object.

```sql
create page Module.Entity_NewEdit
(
  params: { $entity: Module.Entity },
  title: 'Edit Entity',
  layout: Atlas_Core.PopupLayout,
  folder: 'OverviewPages'
)
{
  layoutgrid mainGrid {
    row row1 {
      column col1 (desktopwidth: autofill) {
        dataview dataView1 (datasource: $entity) {
          -- Input fields for each attribute
          textbox txtName (label: 'Name', attribute: Name)
          textbox txtDescription (label: 'Description', attribute: description)
          datepicker dpDueDate (label: 'Due Date', attribute: DueDate)
          combobox cbStatus (label: 'Status', attribute: status)

          footer footer1 {
            actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: success)
            actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
          }
        }
      }
    }
  }
}
```

### Page Parameter Syntax

```sql
create page Module.PageName
(
  params: { $ParamName: Module.EntityName },
  title: '...',
  layout: ...
)
```

- Parameter name conventionally matches the entity name (e.g., `$store`, `$Customer`)
- The DataView's binding references this parameter (`datasource: $ParamName`)
- When calling the page via SHOW_PAGE, pass the object by parameter name: `show_page Module.PageName(ParamName: $value)`
- **Reserved-word parameter names must be quoted in the SHOW_PAGE args.** The generic examples above name the parameter `entity`, which is a reserved keyword, so the call quotes it: `show_page Module.Entity_NewEdit("entity": $currentObject)`. A non-reserved name (`$store`, `$Customer`) needs no quotes: `show_page Module.Store_NewEdit(store: $currentObject)`.

### NewEdit Page Components

1. **Page Parameter**: `params: { $entity: Module.Entity }` - Receives the object to edit
2. **Layout**: `Atlas_Core.PopupLayout` - Popup/modal style
3. **DataView**: Container bound to page parameter (`datasource: $entity`)
4. **Input Widgets**: Match entity attributes with `attribute:` property
5. **Footer**: Save and Cancel buttons

## Complete Example: Store Entity

### Step 1: Create the Navigation Snippet

First, create a navigation menu snippet that will be shared across all overview pages:

```sql
create snippet MdlTemplates.NavigationMenu
{
  layoutgrid navGrid {
    row row1 {
      column col1 (desktopwidth: 12) {
        actionbutton btnStores (caption: 'Stores', action: show_page MdlTemplates.Store_Overview)
        actionbutton btnCars (caption: 'Cars', action: show_page MdlTemplates.Car_Overview)
      }
    }
  }
}
```

### Step 2: Create the Entity

```sql
create persistent entity MdlTemplates.Store (
  Name: string(200) not null,
  Location: string(200)
);
```

### Step 3: Create the Overview Page

```sql
create page MdlTemplates.Store_Overview
(
  title: 'Store Overview',
  layout: Atlas_Core.Atlas_Default,
  folder: 'OverviewPages'
)
{
  layoutgrid mainGrid {
    row row1 {
      column col1 (desktopwidth: 12) {
        snippetcall navMenu (snippet: MdlTemplates.NavigationMenu)
      }
    }
    row row2 {
      column col2 (desktopwidth: 12) {
        dynamictext heading (content: 'Stores', rendermode: H2)
      }
    }
    row row3 {
      column col3 (desktopwidth: 12) {
        datagrid StoreGrid (datasource: database MdlTemplates.Store) {
          column colName (attribute: Name, caption: 'Name')
          column colLocation (attribute: Location, caption: 'Location')
        }
      }
    }
  }
}
```

### Store NewEdit Page

```sql
create page MdlTemplates.Store_NewEdit
(
  params: { $store: MdlTemplates.Store },
  title: 'Edit Store',
  layout: Atlas_Core.PopupLayout,
  folder: 'OverviewPages'
)
{
  layoutgrid mainGrid {
    row row1 {
      column col1 (desktopwidth: autofill) {
        dataview dataView1 (datasource: $store) {
          textbox txtName (label: 'Name', attribute: Name)
          textbox txtLocation (label: 'Location', attribute: Location)

          footer footer1 {
            actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: success)
            actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
          }
        }
      }
    }
  }
}
```

## Complete Example: Car Entity

### Entity Definition

```sql
create persistent entity MdlTemplates.Car (
  Brand: string(200) not null,
  model: string(200),
  Price: decimal,
  PurchaseYear: integer,
  PurchaseDate: datetime,
  CarType: enumeration(MdlTemplates.CarType)
);

create enumeration MdlTemplates.CarType (
  Sedan 'Sedan',
  SUV 'SUV',
  Truck 'Truck',
  Sports 'Sports Car'
);
```

### Car NewEdit Page

Shows various input widget types:

```sql
create page MdlTemplates.Car_NewEdit
(
  params: { $Car: MdlTemplates.Car },
  title: 'Edit Car',
  layout: Atlas_Core.PopupLayout,
  folder: 'OverviewPages'
)
{
  layoutgrid mainGrid {
    row row1 {
      column col1 (desktopwidth: autofill) {
        dataview dataView1 (datasource: $Car) {
          textbox txtBrand (label: 'Brand', attribute: Brand)
          textbox txtModel (label: 'Model', attribute: model)
          textbox txtPrice (label: 'Price', attribute: Price)
          textbox txtYear (label: 'Purchase year', attribute: PurchaseYear)
          datepicker dpDate (label: 'Purchase date', attribute: PurchaseDate)
          radiobuttons rbType (label: 'Car type', attribute: CarType)

          footer footer1 {
            actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: success)
            actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
          }
        }
      }
    }
  }
}
```

## Widget Selection Guide

Choose input widgets based on attribute type:

| Attribute Type | Widget | Example |
|----------------|--------|---------|
| String | `textbox` | Name, Description |
| String (long) | `textarea` | Comments, Notes |
| Integer, Long, Decimal | `textbox` | Price, Quantity |
| Boolean | `checkbox` or `radiobuttons` | IsActive, IsPublished |
| DateTime | `datepicker` | DueDate, OrderDate |
| Enumeration | `combobox` or `radiobuttons` | Status, Type |
| Association (reference) | `combobox` with DataSource | Category, Owner |

**Note:** `dropdown` is deprecated. Use `combobox` for enumeration attributes.

**ComboBox modes:**
- Enum mode: `combobox cb (label: 'status', attribute: status)`
- Association mode: `combobox cb (label: 'Customer', attribute: Order_Customer, datasource: database MyModule.Customer, CaptionAttribute: Name)`

**Reserved Attribute Names:** Do not use `CreatedDate`, `ChangedDate`, `owner`, `ChangedBy` as attribute names - these are system attributes automatically added to all entities.

## Naming Conventions

| Item | Convention | Example |
|------|------------|---------|
| Navigation Snippet | `NavigationMenu` | `MdlTemplates.NavigationMenu` |
| Overview Page | `Entity_Overview` | `Customer_Overview` |
| NewEdit Page | `Entity_NewEdit` | `Customer_NewEdit` |
| Folder | `OverviewPages` | — |
| DataView | `dataView1` or `dv{entity}` | `dvCustomer` |
| DataGrid | `dataGrid1` or `dg{entity}` | `dgCustomer` |
| SnippetCall | `navMenu` or descriptive name | `navMenu`, `headerSnippet` |

## Button Styles

| Style | Use Case | Color |
|-------|----------|-------|
| `success` | Save, Confirm | Green |
| `default` | Cancel, Back | Gray |
| `primary` | Primary action | Blue |
| `danger` | Delete | Red |
| `warning` | Caution actions | Yellow |

## Folder Organization

```
module/
├── snippets/
│   └── NavigationMenu
├── OverviewPages/
│   ├── Customer_Overview
│   ├── Customer_NewEdit
│   ├── Order_Overview
│   ├── Order_NewEdit
│   └── ...
├── microflows/
└── entities/
```

## Parameterized Snippets

Snippets can accept parameters to display context-specific data:

```sql
-- Create a snippet with a parameter
create snippet Module.CustomerDetails
(
  params: { $Customer: Module.Customer }
)
{
  layoutgrid detailsGrid {
    row row1 {
      column col1 (desktopwidth: 12) {
        dynamictext heading (content: 'Customer Details', rendermode: H3)
      }
    }
  }
}

-- Use the snippet with parameter passing
snippetcall customerDetails (snippet: Module.CustomerDetails, params: {Customer: $Customer})
```

## Entity Menu Snippets with NavigationList

For entity-specific action menus (Edit, Delete, etc.), use the `navigationlist` widget:

```sql
create snippet Module.Entity_Menu
(
  params: { $EntityParameter: Module.Entity }
)
{
  navigationlist EntityMenuNav {
    item itemEdit (caption: 'Edit', action: show_page Module.Entity_NewEdit("entity": $EntityParameter))
    item itemDelete (caption: 'Delete', action: delete)
    item itemBack (caption: 'Back', action: close_page)
  }
}
```

### NavigationList Syntax

```sql
navigationlist widgetName {
  item itemName (caption: 'Caption', action: ACTION_TYPE)
}
```

**Supported Actions:**
- `action: save_changes` - Save changes
- `action: cancel_changes` - Cancel changes
- `action: close_page` - Close current page
- `action: delete` - Delete object
- `action: microflow Module.MicroflowName` - Call microflow
- `action: microflow Module.MicroflowName(Param: $value)` - Call microflow with parameters
- `action: show_page Module.PageName` - Navigate to page
- `action: show_page Module.PageName(Param: $value)` - Navigate with parameters

## Handling Circular Dependencies

When a navigation snippet references pages (via `show_page`) and those pages reference the snippet (via `snippetcall`), you have a circular dependency. Use the **placeholder pattern**:

### Creation Order

1. **Create placeholder snippet first** (before pages)
2. **Create all pages** (which reference the snippet via SNIPPETCALL)
3. **Replace snippet with full content** (which can now reference existing pages)

### Example Pattern

```sql
-- Step 1: Create placeholder snippet (pages can reference this)
create snippet Module.NavigationMenu
{
  layoutgrid navGrid {
    row row1 {
      column col1 (desktopwidth: 12) {
        dynamictext loading (content: 'Loading...')
      }
    }
  }
}
/

-- Step 2: Create all pages (they reference the snippet via SNIPPETCALL)
create page Module.Customer_NewEdit
(
  params: { $Customer: Module.Customer },
  title: 'Edit Customer',
  layout: Atlas_Core.PopupLayout
)
{
  -- ... page content with SNIPPETCALL navMenu (Snippet: Module.NavigationMenu)
}
/

create page Module.Customer_Overview
(
  title: 'Customer Overview',
  layout: Atlas_Core.Atlas_Default
)
{
  -- ... page content with SNIPPETCALL navMenu (Snippet: Module.NavigationMenu)
}
/

-- Step 3: Fill in the snippet with real content (pages now exist)
-- Use CREATE OR MODIFY (preserves the snippet's ID → page bindings stay valid)
-- Do NOT use CREATE OR REPLACE — that would assign a new ID and break existing page references
create or modify snippet Module.NavigationMenu
{
  layoutgrid navGrid {
    row row1 {
      column col1 (desktopwidth: 12) {
        actionbutton btnCustomers (caption: 'Customers', action: show_page Module.Customer_Overview)
      }
    }
  }
}
/
```

### Key Points

- The placeholder snippet must exist before pages are created (for `snippetcall` to resolve)
- Use `create or modify snippet` for the fill-in step — it preserves the snippet's UUID so pages that already reference it remain valid
- **Do not use `create or replace snippet`** — that deletes the placeholder and creates a fresh UUID, silently breaking every page that references the old one
- Page references in the final snippet resolve correctly because pages already exist

See [Resolve Forward References](./resolve-forward-references.md) for the full pattern including page→page and microflow→page cases, declaration ordering rules, and the choice between `CREATE OR MODIFY` and `ALTER SNIPPET`.

## Related Skills

- [Create Page](./create-page.md) - Basic page creation syntax
- [ALTER PAGE/SNIPPET](./alter-page.md) - Modify existing pages/snippets in-place (SET, INSERT, DROP, REPLACE)
- [Master-Detail Pages](./master-detail-pages.md) - Selection binding pattern
- [Resolve Forward References](./resolve-forward-references.md) - Placeholder pattern, declaration ordering

## Snippet Commands Reference

| Command | Description |
|---------|-------------|
| `show snippets [in module]` | List all snippets |
| `show snippet Module.Name` | Show snippet summary |
| `describe snippet Module.Name` | Show snippet MDL source |
| `create snippet Module.Name { ... }` | Create a new snippet |
| `create or replace snippet Module.Name { ... }` | Create or update snippet |
| `alter snippet Module.Name { ... }` | Modify snippet widgets in-place |
| `drop snippet Module.Name` | Delete a snippet |
