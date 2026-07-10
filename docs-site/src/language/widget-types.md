# Widget Types

MDL supports a comprehensive set of widget types for building Mendix pages. Each widget is declared with a type keyword, a unique name, properties in parentheses, and optional child widgets in braces.

```sql
WIDGET_TYPE widgetName (Property: value, ...) [{ children }]
```

## Widget Categories

| Category | Widgets |
|----------|---------|
| Layout | `LAYOUTGRID`, `ROW`, `COLUMN`, `CONTAINER`, `CUSTOMCONTAINER` |
| Data | `DATAVIEW`, `LISTVIEW`, `DATAGRID`, `GALLERY` |
| Input | `TEXTBOX`, `TEXTAREA`, `CHECKBOX`, `RADIOBUTTONS`, `DATEPICKER`, `COMBOBOX` |
| Display | `DYNAMICTEXT`, `IMAGE`, `STATICIMAGE`, `DYNAMICIMAGE` |
| Action | `ACTIONBUTTON`, `LINKBUTTON` |
| Navigation | `NAVIGATIONLIST` |
| Structure | `HEADER`, `FOOTER`, `CONTROLBAR`, `SNIPPETCALL` |

## Layout Widgets

### LAYOUTGRID

Creates a responsive grid with rows and columns. The primary layout mechanism for arranging widgets on a page:

```sql
LAYOUTGRID grid1 {
  ROW row1 {
    COLUMN col1 {
      TEXTBOX txtName (Label: 'Name', Attribute: Name)
    }
    COLUMN col2 {
      TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
    }
  }
  ROW row2 {
    COLUMN colFull {
      TEXTAREA txtNotes (Label: 'Notes', Attribute: Notes)
    }
  }
}
```

### ROW

A row within a `LAYOUTGRID`. Contains one or more `COLUMN` children.

### COLUMN

A column within a `ROW`. Contains any number of child widgets. Column width is determined by the layout grid system.

### CONTAINER

A generic div container. Used to group widgets and apply CSS classes or styles:

```sql
CONTAINER cCard (Class: 'card mx-spacing-top-large') {
  DYNAMICTEXT txtTitle (Content: 'Section Title')
  TEXTBOX txtValue (Label: 'Value', Attribute: Value)
}
```

**Properties:**

| Property | Description | Example |
|----------|-------------|---------|
| `Class` | CSS class names | `Class: 'card p-3'` |
| `Style` | Inline CSS styles | `Style: 'padding: 16px;'` |
| `DesignProperties` | Design property values | `DesignProperties: ['Spacing top': 'Large']` |

### CUSTOMCONTAINER

Similar to `CONTAINER` but used for custom-styled containers.

## Data Widgets

### DATAVIEW

Displays a single object. The central widget for detail and edit pages. Must have a data source:

```sql
DATAVIEW dvCustomer (DataSource: $Customer) {
  TEXTBOX txtName (Label: 'Name', Attribute: Name)
  TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
  COMBOBOX cbStatus (Label: 'Status', Attribute: Status)
  FOOTER footer1 {
    ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)
    ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
  }
}
```

**Data source options:**
- `DataSource: $ParamName` -- page parameter
- `DataSource: MICROFLOW Module.MF_Name` -- microflow returning a single object
- `DataSource: NANOFLOW Module.NF_Name` -- nanoflow returning a single object
- `DataSource: SELECTION widgetName` -- currently selected item from a list widget
- `DataSource: ASSOCIATION AssocName` -- follow an association from a parent context

### DATAGRID

Displays a list of objects in a tabular format with columns, sorting, and pagination:

```sql
DATAGRID dgOrders (DataSource: DATABASE Sales.Order, PageSize: 20) {
  COLUMN colId (Attribute: OrderId, Caption: 'Order #')
  COLUMN colDate (Attribute: OrderDate, Caption: 'Date')
  COLUMN colAmount (Attribute: Amount, Caption: 'Amount', Alignment: right)
  COLUMN colStatus (Attribute: Status, Caption: 'Status')
  CONTROLBAR bar1 {
    ACTIONBUTTON btnNew (Caption: 'New', Action: MICROFLOW Sales.ACT_CreateOrder, ButtonStyle: Primary)
  }
}
```

**DataGrid properties:**

| Property | Description | Example |
|----------|-------------|---------|
| `DataSource` | Data source (DATABASE, MICROFLOW, etc.) | `DataSource: DATABASE Module.Entity` |
| `PageSize` | Number of rows per page | `PageSize: 25` |
| `Pagination` | Pagination mode | `Pagination: virtualScrolling` |
| `PagingPosition` | Position of paging controls | `PagingPosition: both` |
| `ShowPagingButtons` | When to show paging buttons | `ShowPagingButtons: auto` |

**Column properties:**

| Property | Values | Default | Description |
|----------|--------|---------|-------------|
| `Attribute` | attribute name, or association path `Assoc/Attr` | (required) | The attribute to display. Use an association path to show an attribute over a reference — e.g. `Attribute: Order_Customer/Name` shows the associated Customer's Name. Association names are bare (resolved against the grid's entity module); multi-hop paths (`A/B/Attr`) are supported. |
| `Caption` | string | attribute name | Column header text |
| `Alignment` | `left`, `center`, `right` | `left` | Text alignment |
| `WrapText` | `true`, `false` | `false` | Allow text wrapping |
| `Sortable` | `true`, `false` | varies | Allow sorting by this column |
| `Resizable` | `true`, `false` | `true` | Allow column resizing |
| `Draggable` | `true`, `false` | `true` | Allow column reordering |
| `Hidable` | `yes`, `hidden`, `no` | `yes` | User visibility toggle |
| `ColumnWidth` | `autoFill`, `autoFit`, `manual` | `autoFill` | Width mode |
| `Size` | integer (px) | `1` | Width in pixels (manual mode) |
| `Visible` | expression string | `true` | Visibility expression |
| `DynamicCellClass` | expression string | (empty) | Dynamic CSS class expression |
| `Tooltip` | text string | (empty) | Column tooltip |

### LISTVIEW

Displays a list of objects using a repeating template. More flexible than DataGrid for custom layouts:

```sql
LISTVIEW lvProducts (DataSource: DATABASE MyModule.Product) {
  CONTAINER cItem (Class: 'list-item') {
    DYNAMICTEXT txtName (Content: '{1}', Attribute: Name)
    DYNAMICTEXT txtPrice (Content: '${1}', Attribute: Price)
  }
}
```

### GALLERY

A pluggable widget that displays items in a card/grid layout:

```sql
GALLERY galProducts (DataSource: DATABASE MyModule.Product) {
  CONTAINER cCard {
    DYNAMICTEXT txtName (Content: '{1}', Attribute: Name)
  }
}
```

**Responsive column properties** control how many columns the grid uses per breakpoint:

| Property | Description | Default |
|----------|-------------|---------|
| `DesktopColumns` | Number of columns on desktop | `1` |
| `TabletColumns` | Number of columns on tablet | `1` |
| `PhoneColumns` | Number of columns on phone | `1` |

```sql
GALLERY galBoard (
  DataSource: DATABASE MyModule.Cell,
  DesktopColumns: 9,
  TabletColumns: 4,
  PhoneColumns: 2
) {
  DYNAMICTEXT txtVal (Content: '{1}', Attribute: Value)
}
```

## Input Widgets

All input widgets share common properties:

| Property | Description | Example |
|----------|-------------|---------|
| `Label` | Field label text | `Label: 'Customer Name'` |
| `Attribute` | Entity attribute to bind to | `Attribute: Name` |
| `Editable` | Editability mode | `Editable: ReadOnly` |
| `Visible` | Visibility expression | `Visible: '$showField'` |

### TEXTBOX

Single-line text input. The most common input widget:

```sql
TEXTBOX txtName (Label: 'Name', Attribute: Name)
TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
```

### TEXTAREA

Multi-line text input for longer text:

```sql
TEXTAREA txtDescription (Label: 'Description', Attribute: Description)
```

### CHECKBOX

Boolean (true/false) input:

```sql
CHECKBOX cbActive (Label: 'Active', Attribute: IsActive)
```

### RADIOBUTTONS

Displays enumeration or boolean values as radio buttons:

```sql
RADIOBUTTONS rbStatus (Label: 'Status', Attribute: Status)
```

### DATEPICKER

Date and/or time input:

```sql
DATEPICKER dpBirthDate (Label: 'Birth Date', Attribute: BirthDate)
```

### COMBOBOX

Dropdown selection for enumeration values or associations. Uses the pluggable ComboBox widget:

```sql
COMBOBOX cbStatus (Label: 'Status', Attribute: Status)
```

### REFERENCESELECTOR

Dropdown for selecting an associated object via a reference association:

```sql
REFERENCESELECTOR rsCategory (Label: 'Category', Attribute: Category)
```

## Display Widgets

### DYNAMICTEXT

Displays dynamic text content, often with attribute values:

```sql
DYNAMICTEXT txtGreeting (Content: 'Welcome, {1}', Attribute: Name)
```

### IMAGE / STATICIMAGE / DYNAMICIMAGE

Display images on a page:

```sql
-- Static image from the project
STATICIMAGE imgLogo (Image: 'MyModule.Logo')

-- Dynamic image from an entity attribute (entity must extend System.Image)
DYNAMICIMAGE imgPhoto (DataSource: $Photo, Width: 200, Height: 150)

-- Generic image widget
IMAGE imgBanner (Width: 800, Height: 200)
```

**Image properties:**

| Property | Description | Example |
|----------|-------------|---------|
| `Width` | Width in pixels | `Width: 200` |
| `Height` | Height in pixels | `Height: 150` |

## Action Widgets

### ACTIONBUTTON

A button that triggers an action. The primary interactive element:

```sql
ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)
ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
ACTIONBUTTON btnDelete (Caption: 'Delete', Action: DELETE, ButtonStyle: Danger)
```

**Action types:**

| Action | Description |
|--------|-------------|
| `SAVE_CHANGES` | Commit and close the page |
| `CANCEL_CHANGES` | Roll back and close the page |
| `DELETE` | Delete the current object |
| `CLOSE_PAGE` | Close the page without saving |
| `MICROFLOW Module.MF_Name` | Call a microflow |
| `NANOFLOW Module.NF_Name` | Call a nanoflow |
| `PAGE Module.PageName` | Open a page |

**Button styles:**

| Style | Typical appearance |
|-------|--------------------|
| `Default` | Standard button |
| `Primary` | Blue/highlighted button |
| `Success` | Green button |
| `Warning` | Yellow/amber button |
| `Danger` | Red button |
| `Info` | Light blue button |

**Microflow action with parameters:**

```sql
ACTIONBUTTON btnProcess (
  Caption: 'Process',
  Action: MICROFLOW Sales.ACT_ProcessOrder(Order: $Order),
  ButtonStyle: Primary
)
```

### LINKBUTTON

Renders as a hyperlink instead of a button. Same action types as `ACTIONBUTTON`:

```sql
LINKBUTTON lnkDetails (Caption: 'View Details', Action: PAGE MyModule.Customer_Detail)
```

## Structure Widgets

### HEADER

Header section of a DataView, placed before the main content:

```sql
DATAVIEW dvOrder (DataSource: $Order) {
  HEADER hdr1 {
    DYNAMICTEXT txtOrderTitle (Content: 'Order #{1}', Attribute: OrderId)
  }
  TEXTBOX txtStatus (Label: 'Status', Attribute: Status)
  FOOTER ftr1 { ... }
}
```

### FOOTER

Footer section of a DataView. Typically contains save/cancel buttons:

```sql
FOOTER footer1 {
  ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)
  ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
}
```

### CONTROLBAR

Control bar for DataGrid widgets. Contains action buttons for the grid:

```sql
CONTROLBAR bar1 {
  ACTIONBUTTON btnNew (Caption: 'New', Action: MICROFLOW Module.ACT_Create, ButtonStyle: Primary)
  ACTIONBUTTON btnEdit (Caption: 'Edit', Action: PAGE Module.Entity_Edit)
  ACTIONBUTTON btnDelete (Caption: 'Delete', Action: DELETE, ButtonStyle: Danger)
}
```

### SNIPPETCALL

Embeds a snippet (reusable page fragment) into the page:

```sql
SNIPPETCALL scNav (Snippet: MyModule.NavigationMenu)
```

## Navigation Widgets

### NAVIGATIONLIST

Renders a navigation list with clickable items:

```sql
NAVIGATIONLIST navMain {
  -- navigation items
}
```

## Common Widget Properties

These properties are shared across many widget types:

| Property | Description | Example |
|----------|-------------|---------|
| `Class` | CSS class names | `Class: 'card p-3'` |
| `Style` | Inline CSS styles | `Style: 'margin-top: 8px;'` |
| `DesignProperties` | Atlas design properties | `DesignProperties: ['Spacing top': 'Large', 'Full width': ON]` |
| `Visible` | Visibility expression | `Visible: '$showSection'` |
| `Editable` | Editability mode | `Editable: ReadOnly` |

## See Also

- [Pages](./pages.md) -- page overview and CREATE PAGE basics
- [Page Structure](./page-structure.md) -- layout selection and data sources
- [Data Binding](./data-binding.md) -- connecting widgets to attributes
- [ALTER PAGE](./alter-page.md) -- modifying widgets in existing pages
