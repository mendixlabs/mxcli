# Master-Detail Pages

## Overview

Master-Detail is a common UI pattern showing:
- **Master list** (left): Selectable list of items (Gallery widget)
- **Detail form** (right): Form showing selected item details (DataView with SELECTION source)

## MDL Syntax

### Basic Structure

```sql
create page Module.Entity_MasterDetail
(
  title: 'Entity Master-Detail',
  layout: Atlas_Core.Atlas_Default
)
{
  layoutgrid mainGrid {
    row row1 {
      -- Master list (4 columns)
      column colMaster (desktopwidth: 4) {
        gallery entityList (datasource: database Module.Entity, selection: single) {
          template template1 {
            dynamictext name (content: '{1}', contentparams: [{1} = Name], rendermode: H4)
          }
        }
      }

      -- Detail form (8 columns)
      column colDetail (desktopwidth: 8) {
        dataview entityDetail (datasource: selection entityList) {
          textbox txtName (label: 'Name', attribute: Name)

          footer footer1 {
            actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: success)
          }
        }
      }
    }
  }
}
```

### Key Components

#### 1. GALLERY Widget (Master List)

```sql
gallery widgetName (
  datasource: database from Module.Entity sort by Name asc,
  selection: single|multiple|none
) {
  template template1 {
    -- Widgets for each item
    dynamictext name (content: '{1}', contentparams: [{1} = AttrName], rendermode: H4)
  }
}
```

**Properties:**
- `datasource: database from entity sort by attr asc|desc` - Entity data source with optional sorting
- `selection: single` - Selection mode (Single for master-detail)
- Template content inside TEMPLATE widget (requires name)

#### 2. DataView with SELECTION Source

```sql
dataview widgetName (datasource: selection sourceWidgetName) {
  -- Form widgets
}
```

The `selection` source creates a binding to another widget's selection. When the user selects an item in the Gallery, the DataView displays that item.

#### 3. LISTVIEW Widget (Nested Data)

```sql
listview widgetName (datasource: database Module.Entity, PageSize: 10) {
  template template1 {
    -- Widgets for each associated item
  }
}
```

Used inside the detail form to show related/associated data.

**Nested list by association (to-many):** Use `datasource: $currentObject/Module.Assoc` (or the explicit `datasource: association path` form) on a **list** widget inside a parent DATAVIEW. Example: `datagrid lines (datasource: $currentObject/Order_OrderLine)` inside a `dataview dv (datasource: database Order)`.

**Referenced object by association (to-one) — "data from context":** To show a single *referenced* object's attributes, use the same `datasource: $currentObject/Module.Assoc` on a nested **DATAVIEW**. Its children bind to the referenced (destination) entity. Example: an inner `dataview dvEmployee (datasource: $currentObject/Expense_Employee) { textbox (attribute: Name) }` inside a `dataview dvExpense (datasource: $Expense)` shows the Expense's Employee. (A DataView association source is stored as a `Forms$DataViewSource` with an association-navigating EntityRef — do **not** confuse it with the list-widget `Forms$AssociationSource`; that one is invalid on a DataView.)

## Complete Example

```sql
create page CRM.Customer_MasterDetail
(
  title: 'Customer Management',
  layout: Atlas_Core.Atlas_Default
)
{
  layoutgrid mainGrid {
    row row1 {
      column colMaster (desktopwidth: 4) {
        dynamictext heading (content: 'Customers', rendermode: H3)
        gallery customerList (datasource: database from CRM.Customer sort by Name asc, selection: single) {
          template template1 {
            dynamictext name (content: '{1}', contentparams: [{1} = Name], rendermode: H4)
            dynamictext email (content: '{1}', contentparams: [{1} = Email])
          }
        }
      }

      column colDetail (desktopwidth: 8) {
        dataview customerDetail (datasource: selection customerList) {
          dynamictext detailHeading (content: 'Customer Details', rendermode: H3)
          textbox txtName (label: 'Name', attribute: Name)
          textbox txtEmail (label: 'Email', attribute: Email)
          textbox txtPhone (label: 'Phone', attribute: Phone)

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

## Key Patterns

### Selection Binding

The core of master-detail is the selection binding:
1. Gallery has `selection: single` - enables single item selection
2. DataView uses `datasource: selection galleryName` - listens to Gallery selection
3. When user clicks an item in Gallery, DataView automatically updates

### Widget Names

The selection binding uses widget names to connect:
- Gallery widget name: `customerList`
- DataView references: `datasource: selection customerList`

### Template Content with ContentParams

Inside Gallery templates, use `contentparams` to reference current item attributes:
```sql
template template1 {
  dynamictext name (content: '{1}', contentparams: [{1} = Name], rendermode: H4)
  dynamictext email (content: '{1}', contentparams: [{1} = Email])
}
```

## Syntax Summary

| Element | Syntax |
|---------|-----------|
| Page properties | `(title: 'title', layout: Module.Layout)` |
| Widget name | Required after type: `gallery myGallery (...)` |
| Database source | `datasource: database from Module.Entity` |
| Selection binding | `datasource: selection widgetName` |
| Sort by | `datasource: database from entity sort by Name asc` |
| Where filter | `datasource: database from entity where [IsActive = true]` |
| Selection mode | `selection: single` |
| Attribute binding | `attribute: attributename` |
| Action binding | `action: save_changes` |
| Button style | `buttonstyle: success` |
| Text content | `content: 'text'` with `contentparams: [{1} = attr]` |
| Render mode | `rendermode: H4` |
| Template content | `template template1 { ... }` |

## Related Skills

- [Overview Pages](./overview-pages.md) - CRUD page patterns
- [Create Page](./create-page.md) - Basic page syntax
- [ALTER PAGE/SNIPPET](./alter-page.md) - Modify existing pages in-place (SET, INSERT, DROP, REPLACE)

## Implementation Notes

- Gallery is a pluggable widget (similar to DataGrid2)
- Selection binding uses `ListenTargetSource` in the Model SDK
- ListView is a built-in Mendix widget
- All widget properties use explicit `(key: value)` syntax
