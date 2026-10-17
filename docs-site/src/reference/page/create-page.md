# CREATE PAGE

## Synopsis

```sql
CREATE [ OR REPLACE ] PAGE module.Name [ FOLDER 'path' ]
(
    [ Params: ( $param : Module.Entity | Type [, ...] ), ]
    Title: 'title',
    Layout: Module.LayoutName
    [, Variables: ( $name : type = <expression> [, ...] ) ]
)
{
    widget_tree
}
```

## Description

Creates a new page in the specified module. A page defines a user interface screen with a title, a layout, optional parameters, and a hierarchical widget tree.

If `OR REPLACE` is specified and a page with the same qualified name already exists, it is replaced. Otherwise, creating a page with an existing name is an error.

### Page Properties

The page declaration block `( ... )` contains comma-separated key-value properties:

- **Params** -- Page parameters. Each parameter has a `$`-prefixed name and a type — either an entity (`Module.Entity`) or a primitive (`String`, `Integer`, `Decimal`, `Boolean`, `DateTime`). Parameters are passed when the page is opened (e.g., from a microflow `SHOW PAGE` or from a data view). Primitive parameters require Mendix 11.6+.
- **Title** -- The page title displayed in the browser tab or page header.
- **Layout** -- The layout the page is based on. Must be a qualified name referring to an existing layout (e.g., `Atlas_Core.Atlas_Default`, `Atlas_Core.PopupLayout`).
- **Folder** -- Optional folder path within the module. Nested folders use `/` separator.
- **Variables** -- Optional page-level variables with default expressions. Variables can be used for conditional visibility and dynamic behavior.

### Widget Tree

The widget tree inside `{ ... }` defines the page content. Widgets are nested hierarchically. Each widget has:

- A **type keyword** (e.g., `DATAVIEW`, `TEXTBOX`, `ACTIONBUTTON`)
- A **name** (unique within the page, used for ALTER PAGE references)
- **Properties** in parentheses `(Key: value, ...)`
- Optional **children** in braces `{ ... }`

### Widget Types

**Layout Widgets**

| Widget | Description | Key Properties |
|--------|-------------|----------------|
| `LAYOUTGRID` | Responsive grid container | `Class`, `Style`, `Width` (`FullWidth` default, `FixedWidth`) |
| `ROW` | Grid row | `Class` |
| `COLUMN` | Grid column | `Class` |
| `CONTAINER` | Generic div container | `Class`, `Style`, `DesignProperties` |
| `CUSTOMCONTAINER` | Custom container widget | `Class`, `Style` |

**Data Widgets**

| Widget | Description | Key Properties |
|--------|-------------|----------------|
| `DATAVIEW` | Displays/edits a single object | `DataSource` |
| `LISTVIEW` | Renders a list of objects | `DataSource`, `PageSize` |
| `DATAGRID` | Tabular data display with columns | `DataSource`, `PageSize`, `Pagination` |
| `GALLERY` | Card-based list display | `DataSource`, `DesktopColumns`, `TabletColumns`, `PhoneColumns` |

**Input Widgets**

| Widget | Description | Key Properties |
|--------|-------------|----------------|
| `TEXTBOX` | Single-line text input | `Label`, `Attribute`, `Password`, `SubmitBehaviour` (`OnEndEditing` default, `WhileEditing`), `SubmitOnInputDelay` (ms, default 300) |
| `TEXTAREA` | Multi-line text input | `Label`, `Attribute`, `SubmitBehaviour`, `SubmitOnInputDelay` |
| `CHECKBOX` | Boolean toggle | `Label`, `Attribute` |
| `RADIOBUTTONS` | Radio button group | `Label`, `Attribute` |
| `DATEPICKER` | Date/time selector | `Label`, `Attribute` |
| `COMBOBOX` | Dropdown selector | `Label`, `Attribute` |
| `FILEMANAGER` | File upload/download, inside a data view on a `System.FileDocument` specialization | `Label`, `Type` (`Upload`, `Download`, `Both` default), `MaxFileSize` (MB, default 5), `AllowedExtensions`, `ShowFileInBrowser` |

**Display Widgets**

| Widget | Description | Key Properties |
|--------|-------------|----------------|
| `DYNAMICTEXT` | Display-only text bound to an attribute | `Attribute` |
| `LABEL` | Studio Pro's Label widget (`Forms$Label`): a fixed, translatable caption. `describe` emits it for existing labels; prefer `DYNAMICTEXT` for new text | `Content` |
| `IMAGE` | Generic image | `Width`, `Height` |
| `STATICIMAGE` | Fixed image from project resources | `Width`, `Height` |
| `DYNAMICIMAGE` | Image from an entity attribute | `Width`, `Height` |

**Action Widgets**

| Widget | Description | Key Properties |
|--------|-------------|----------------|
| `ACTIONBUTTON` | Button that triggers an action | `Caption`, `Action`, `ButtonStyle`, `Icon` |
| `LINKBUTTON` | Hyperlink-styled action | `Caption`, `Action`, `Icon` |

**Structure Widgets**

| Widget | Description | Key Properties |
|--------|-------------|----------------|
| `HEADER` | Page header area | -- |
| `FOOTER` | Page footer area (typically for buttons) | -- |
| `CONTROLBAR` | Control bar for data grids | -- |
| `SNIPPETCALL` | Embeds a snippet | `Snippet: Module.SnippetName` |
| `NAVIGATIONLIST` | Navigation sidebar list | -- |

### DataSource Types

The `DataSource` property determines how a data widget obtains its data:

| DataSource | Syntax | Description |
|------------|--------|-------------|
| Parameter | `DataSource: $ParamName` | Binds to a page parameter or enclosing data view |
| Database | `DataSource: DATABASE Module.Entity` | Retrieves from database |
| Selection | `DataSource: SELECTION widgetName` | Listens to selection on another widget |
| Microflow | `DataSource: MICROFLOW Module.MFName` | Calls a microflow for data |

### Action Types

The `Action` property determines what happens when the widget is clicked. It is
not a button-only property: a `CONTAINER` is clickable too and takes the same
values, including the argument list.

| Action | Syntax | Description |
|--------|--------|-------------|
| Save | `Action: save changes` | Commits and closes |
| Cancel | `Action: cancel changes` | Rolls back and closes |
| Microflow | `Action: MICROFLOW Module.Name(Param = val)` | Calls a microflow |
| Nanoflow | `Action: NANOFLOW Module.Name(Param = val)` | Calls a nanoflow |
| Page | `Action: PAGE Module.PageName` | Opens a page |
| Close | `Action: CLOSE PAGE` | Closes the current page |
| Delete | `Action: DELETE` | Deletes the context object |
| Nothing | `Action: NOTHING` | Deliberately no action — a decorative button, a card that is not clickable |

The set is closed. Anything else in an action slot is an error
(**MDL-WIDGET28**), and that includes a real action keyword **missing its
argument** — `Action: OPEN LINK` with no URL, `Action: SHOW PAGE` with no page.
Such a widget used to be written with no action at all: it rendered, carried its
caption, and did nothing, while `mxcli check`, `exec` and mxbuild all reported
success, because a no-action widget is perfectly legal Mendix. Write `NOTHING`
when a control is genuinely meant to be inert, so that a dead one always means a
mistake.

The same values serve `OnClick:` (an alias of `Action:`) and `OnChange:`.

#### Action settings

Any action but `NOTHING` takes a `with ( … )` list of the settings Studio Pro
shows under the event:

| Setting | Applies to | Default |
|---------|-----------|---------|
| `DisabledDuringExecution: true\|false` | every action | `true` |
| `ProgressBar: None\|NonBlocking\|Blocking` | microflow and nanoflow calls | `None` |
| `ProgressMessage: 'text'` | microflow and nanoflow calls | none |
| `Confirmation: 'question'` | microflow and nanoflow calls | no confirmation |
| `ProceedCaption: 'text'`, `CancelCaption: 'text'` | a confirmation's buttons | `Proceed`, `Cancel` |
| `Asynchronous: true\|false` | microflow calls | `false` |
| `FormValidations: All\|Widget\|None` | microflow calls | `All` |

```sql
ACTIONBUTTON btnDelete (Caption: 'Delete',
  Action: CALL MICROFLOW Shop.ACT_Order_Delete(Order = $currentObject) with (
    ProgressBar: Blocking, ProgressMessage: 'Deleting…',
    Confirmation: 'Delete this order?', ProceedCaption: 'Delete', CancelCaption: 'Keep'))
```

A setting left out keeps its default, and `DESCRIBE` prints only the settings
that differ from it. An unknown key, or a setting the action does not have, is
an error. For `CREATE OBJECT … THEN SHOW PAGE …` the list goes at the end and
belongs to the create action.

A microflow or nanoflow action is a **call**: every parameter the flow declares
needs an argument, or Mendix rejects the page with **CE1571**. An enclosing data
container of the parameter's type supplies it without one — but a data grid's
**control bar** does not, because it is not row-scoped. Pass the grid's selection
there, addressed by the widget's own name:

```sql
DATAGRID dgOrders (DataSource: DATABASE FROM Sales.Order, Selection: Single) {
  COLUMN (Attribute: Number, Caption: 'Order #')
  CONTROLBAR {
    CONTAINER cShip (Class: 'command',
      Action: CALL NANOFLOW Sales.ACT_Ship(Order = $dgOrders)) {
      ACTIONBUTTON btnShip (Caption: 'Ship')
    }
  }
}
```

`mxcli check -p <app.mpr>` reports a missing argument before mxbuild does.

### ButtonStyle Values

`Primary`, `Default`, `Success`, `Danger`, `Warning`, `Info`.

### DataGrid Columns

Inside a `DATAGRID`, child widgets define columns. Each column widget specifies:

| Property | Values | Description |
|----------|--------|-------------|
| `Attribute` | attribute name | The entity attribute to display |
| `Caption` | string | Column header (defaults to attribute name) |
| `Alignment` | `left`, `center`, `right` | Text alignment |
| `WrapText` | `true`, `false` | Whether text wraps |
| `Sortable` | `true`, `false` | Whether column is sortable |
| `Resizable` | `true`, `false` | Whether column is resizable |
| `Draggable` | `true`, `false` | Whether column is draggable |
| `Hidable` | `yes`, `hidden`, `no` | Column visibility control |
| `ColumnWidth` | `autoFill`, `autoFit`, `manual` | Width mode |
| `Size` | integer (px) | Manual width in pixels |
| `Visible` | expression string | Conditional visibility |
| `Tooltip` | text string | Column tooltip |

### Common Widget Properties

These properties are available on most widget types:

| Property | Description | Example |
|----------|-------------|---------|
| `Class` | CSS class names | `Class: 'card mx-spacing-top-large'` |
| `Style` | Inline CSS | `Style: 'padding: 16px;'` |
| `DynamicClasses` | Runtime-computed CSS classes (expression; stacks on `Class`) | `DynamicClasses: if $currentObject/IsActive then 'is-active' else ''` |
| `Editable` | Edit control: `Always`, `Never`, or a condition expression (bare or quoted) | `Editable: NEVER` or `Editable: $currentObject/Locked = false` |
| `Visible` | Visibility expression | `Visible: '$showField'` |
| `DesignProperties` | Atlas design properties | `DesignProperties: ('Spacing top': 'Large')` |

## Parameters

`module.Name`
:   The qualified name of the page (`Module.PageName`). The module must already exist.

`Params: ( ... )`
:   Optional page parameters. Each parameter has a `$`-prefixed name and an entity type.

`Title: 'title'`
:   The display title of the page. Required.

`Layout: Module.LayoutName`
:   The page layout. Must reference an existing layout. Required.

`FOLDER 'path'`
:   Optional folder within the module, as a clause after the name. Nested folders use `/`. The `Folder: 'path'` property is its deprecated alias (`MDL-DEPR105`).

`Variables: ( ... )`
:   Optional page variables with type and default expression.

## Examples

Edit page with a data view and form fields:

```sql
CREATE PAGE MyModule.Customer_Edit
(
    Params: ( $Customer: MyModule.Customer ),
    Title: 'Edit Customer',
    Layout: Atlas_Core.PopupLayout
)
{
    DATAVIEW dvCustomer (DataSource: $Customer) {
        TEXTBOX txtName (Label: 'Name', Attribute: Name)
        TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
        COMBOBOX cbStatus (Label: 'Status', Attribute: Status)

        FOOTER {
            ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)
            ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)
        }
    }
};
```

Overview page with a data grid:

```sql
CREATE PAGE Sales.Order_Overview FOLDER 'Orders'
(
    Title: 'Orders',
    Layout: Atlas_Core.Atlas_Default
)
{
    DATAGRID dgOrders (DataSource: DATABASE Sales.Order, PageSize: 20) {
        COLUMN (Attribute: OrderId, Caption: 'Order #')
        COLUMN (Attribute: OrderDate, Caption: 'Date')
        COLUMN (Attribute: Status, Caption: 'Status')
        COLUMN (Attribute: TotalAmount, Caption: 'Amount', Alignment: right)
        CONTROLBAR {
            ACTIONBUTTON btnNew (Caption: 'New Order', Action: SHOW PAGE Sales.Order_Edit, ButtonStyle: Primary)
        }
    }
};
```

Page with layout grid and containers:

```sql
CREATE PAGE MyModule.Dashboard
(
    Title: 'Dashboard',
    Layout: Atlas_Core.Atlas_Default
)
{
    LAYOUTGRID lgMain {
        ROW {
            COLUMN (Class: 'col-md-8') {
                CONTAINER cntRecent (Class: 'card') {
                    LISTVIEW lvRecent (DataSource: DATABASE MyModule.RecentActivity, PageSize: 5) {
                        DYNAMICTEXT txtDesc (Attribute: Description)
                    }
                }
            }
            COLUMN (Class: 'col-md-4') {
                CONTAINER cntStats (Class: 'card') {
                    DATAVIEW dvStats (DataSource: MICROFLOW MyModule.DS_DashboardStats) {
                        DYNAMICTEXT txtCount (Attribute: TotalCount)
                    }
                }
            }
        }
    }
};
```

Page with snippet call:

```sql
CREATE PAGE MyModule.Customer_Detail
(
    Params: ( $Customer: MyModule.Customer ),
    Title: 'Customer Detail',
    Layout: Atlas_Core.Atlas_Default
)
{
    DATAVIEW dvCustomer (DataSource: $Customer) {
        SNIPPETCALL snpHeader (Snippet: MyModule.CustomerHeader)
        TEXTBOX txtName (Label: 'Name', Attribute: Name)
        TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
    }
};
```

Page with a gallery and selection-driven detail:

```sql
CREATE PAGE Sales.Product_Gallery
(
    Title: 'Products',
    Layout: Atlas_Core.Atlas_Default
)
{
    LAYOUTGRID lgMain {
        ROW {
            COLUMN (Class: 'col-md-6') {
                GALLERY galProducts (DataSource: DATABASE Sales.Product, PageSize: 12) {
                    DYNAMICTEXT txtName (Attribute: Name)
                    DYNAMICTEXT txtPrice (Attribute: Price)
                }
            }
            COLUMN (Class: 'col-md-6') {
                DATAVIEW dvDetail (DataSource: SELECTION galProducts) {
                    TEXTBOX txtName (Label: 'Product', Attribute: Name)
                    TEXTBOX txtDesc (Label: 'Description', Attribute: Description)
                }
            }
        }
    }
};
```

Page with page variables and conditional visibility:

```sql
CREATE PAGE MyModule.AdvancedForm
(
    Params: ( $Item: MyModule.Item ),
    Title: 'Advanced Form',
    Layout: Atlas_Core.Atlas_Default,
    Variables: ( $showAdvanced: Boolean = false )
)
{
    DATAVIEW dvItem (DataSource: $Item) {
        TEXTBOX txtName (Label: 'Name', Attribute: Name)
        ACTIONBUTTON btnToggle (Caption: 'Show Advanced', Action: CALL NANOFLOW MyModule.NAV_Toggle)
        CONTAINER cntAdvanced (Visible: '$showAdvanced') {
            TEXTAREA taNotes (Label: 'Notes', Attribute: Notes)
        }
        FOOTER {
            ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)
            ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)
        }
    }
};
```

Page with microflow data source:

```sql
CREATE PAGE Reports.MonthlySummary
(
    Title: 'Monthly Summary',
    Layout: Atlas_Core.Atlas_Default
)
{
    DATAVIEW dvReport (DataSource: MICROFLOW Reports.DS_GetMonthlySummary) {
        DYNAMICTEXT txtPeriod (Attribute: Period)
        DYNAMICTEXT txtRevenue (Attribute: TotalRevenue)
        DYNAMICTEXT txtOrders (Attribute: OrderCount)
    }
};
```

## See Also

[CREATE SNIPPET](create-snippet.md), [ALTER PAGE](alter-page.md), [DROP PAGE](drop-page.md), [DESCRIBE PAGE](/reference/query/describe-page.md), [GRANT VIEW ON PAGE](/reference/security/grant.md)
