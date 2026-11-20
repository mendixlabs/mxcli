# CREATE PAGE

## Synopsis

```sql
CREATE [ OR REPLACE ] PAGE module.Name [ FOLDER 'path' ]
(
    [ Params: ( $param : Module.Entity | Type [optional] [, ...] ), ]
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

A replace (`OR REPLACE` or `OR MODIFY`) is refused when the stored page holds a widget MDL cannot author — a classic data grid, a reference selector, a drop-down, a reference set selector — that the statement does not declare by name, because the rebuilt page would not contain it. `DESCRIBE PAGE` marks each such widget with a `-- NOT re-executable` comment, and the error lists them. Change the page with [ALTER PAGE](alter-page.md), which keeps them, or remove them on purpose first (`ALTER PAGE M.P { DROP grid1; };`). The same applies to snippets and layouts, and `mxcli check -p` reports it before anything is written.

### Page Properties

The page declaration block `( ... )` contains comma-separated key-value properties:

- **Params** -- Page parameters. Each parameter has a `$`-prefixed name and a type — either an entity (`Module.Entity`) or a primitive (`String`, `Integer`, `Decimal`, `Boolean`, `DateTime`). Parameters are passed when the page is opened (e.g., from a microflow `SHOW PAGE` or from a data view). Primitive parameters require Mendix 11.6+. A parameter is required unless it says `optional` (`$Sale: Shop.Sale optional`, Studio Pro's unticked *Required*, Mendix 11.5+): an optional parameter lets the page be opened with no argument, from a menu item for example, where a required one is CE1571. `describe page` prints it, and a snippet parameter cannot be optional. A parameter cannot be named with a platform-reserved word — `Context`, `Type`, `Id`, `Owner`, `Object`, `CurrentUser` or a Java keyword, in any case and even quoted: Mendix reports CE7247 "The name 'Context' is a reserved word.", and `mxcli check` refuses it (MDL-PARAM01). The same holds for snippet, Java action and JavaScript action parameters.
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
| `TEXTBOX` | Single-line text input | `Label`, `Attribute`, `Password`, `OnChange`, `OnEnter`, `OnLeave`, `OnEnterKeyPress` (an action run when the user presses Enter in the box; the stored name `OnEnterKeyPressAction` is accepted too), `SubmitBehaviour` (`OnEndEditing` default, `WhileEditing`), `SubmitOnInputDelay` (ms, default 300), `KeyboardType` (the keyboard a native app opens: `Default`, `NumberPad`, `DecimalPad`, `EmailAddress`, `PhonePad`, `URL`) |
| `TEXTAREA` | Multi-line text input | `Label`, `Attribute`, `OnChange`, `OnEnter`, `OnLeave`, `SubmitBehaviour`, `SubmitOnInputDelay` |
| `CHECKBOX` | Boolean toggle | `Label`, `Attribute`, `OnChange`, `OnEnter`, `OnLeave` |
| `RADIOBUTTONS` | Radio button group | `Label`, `Attribute`, `OnChange`, `OnEnter`, `OnLeave` |
| `DATEPICKER` | Date/time selector | `Label`, `Attribute`, `OnChange`, `OnEnter`, `OnLeave` |
| `COMBOBOX` | Dropdown selector | `Label`, `Attribute`; association mode adds `DataSource`, `CaptionAttribute`; database mode `optionsSourceDatabaseDataSource`, `CaptionAttribute`, `optionsSourceDatabaseValueAttribute`; custom content as the mode's body block (`optionssourceassociationcustomcontent { … }`, `optionssourcedatabasecustomcontent { … }`) |
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
| Microflow | `DataSource: MICROFLOW Module.MFName` | Calls a microflow for data. Takes `with (FormValidations: All\|Widget\|None)` like a microflow action (default `All`); no other setting |

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

The same values serve `OnClick:` (an alias of `Action:`), `OnChange:` and a
text box's `OnEnterKeyPress:` — Studio Pro's *On enter key press*, which runs
when the user presses Enter in the box (a search field that searches on Enter):

```sql
TEXTBOX tbSearch (Label: 'Search', Attribute: SearchText,
  OnEnterKeyPress: CALL MICROFLOW Module.ACT_Search(Query = $currentObject))
```

Only a text box has that action — Mendix stores it on no other widget, a text
area included — so on any other widget the key is refused with that reason.

Every input widget (text box, text area, date picker, check box, radio
buttons, drop-down) also has Studio Pro's *On enter* and *On leave* — focus
moving into and out of the widget, not the Enter key — spelled `OnEnter:` and
`OnLeave:`, with the same values. The stored names `OnEnterAction:`,
`OnLeaveAction:` and `OnEnterKeyPressAction:` are accepted as the same
properties; `DESCRIBE` prints the short ones.

A flow or page that takes no parameters may be called with an empty argument
list, as a flow body writes it: `Action: CALL MICROFLOW Module.Name()` stores
exactly what `Action: CALL MICROFLOW Module.Name` stores, and `DESCRIBE` writes
it without the brackets. The same holds for a microflow or nanoflow data source
and for `USE FRAGMENT Name()`.

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

#### Return value mappings

A microflow or nanoflow call can map what the flow returns back onto the page —
Studio Pro's **Return value mappings** (Mendix 11.5+). The `returns ( … )` list
goes after the arguments and before `with ( … )`, one `target = source` per row:

```sql
DATAVIEW dvVehicle (DataSource: $Vehicle) {
  ACTIONBUTTON btnRefresh (Caption: 'Refresh',
    Action: CALL MICROFLOW Shop.ACT_Vehicle_Get returns (
      Brand = $ActionReturnValue/Brand,     -- an attribute of the returned object
      $stringResult = $ActionReturnValue,   -- the return value itself
      Brand = 'Demo brand'))                -- any expression
}
```

| Target | Means | Needs |
|--------|-------|-------|
| `Attr` (bare) | an attribute of the enclosing data container's object | Mendix 11.15 — mx 11.13 and 11.14 report CE7006 on it |
| `$name` | a page variable (`Variables: ( … )`) | Mendix 11.5 |

The source is `$ActionReturnValue`, `$ActionReturnValue/Attr` (the flow must
return an object that has `Attr`), or any expression. `DESCRIBE` prints the
stored mappings, and re-running a statement without the list removes them — the
statement is the authority. A mapping `DESCRIBE` cannot spell (a path over an
association) is printed as a comment under the widget, saying a re-run drops it.

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

A `SHOW PAGE` in the control bar takes the selection the same way —
`Action: SHOW PAGE Sales.Order_Detail(Order = $dgOrders)` opens the page with
the selected row, and the button is disabled until a row is selected. In a grid
with `Selection`, a `SHOW PAGE` in a column's custom content may pass the
grid's selection by name as well, which is what Studio Pro stores there;
`$currentObject` is the row.

A control bar takes its object from above its grid, but sits inside the grid,
so the data view around the grid is an outer one there: `$dvOrder` from the
control bar of a grid in data view `dvOrder` is valid.

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

### Attributes of a Page or Snippet Parameter

A bare attribute name binds to the enclosing data container's object. To bind
an attribute of a page or snippet **parameter** instead, whatever container the
widget is in, write `$Param.Attr`. This works for a pluggable widget's
attribute property set by its own key too, such as a Custom chart's
`eventDataAttribute: $Ctx.EventData`. The qualified name of another entity's
attribute (`Module.Other.Attr`) is refused, because stored without the
parameter it is mx check CE7247.

An **enclosing data view** is named the same way: inside a data view nested in
`dvOuter`, `$dvOuter.Attr` reads the outer data view's object, and
`$dvOuter/Assoc/Attr` reads over an association from it. This works on input
widgets, on a pluggable widget's attribute property
(`markdown md (stringAttribute: $dvOuter.Output)`) and in an attribute-based
visibility condition (`Visible: $dvOuter.HasError in (true)`). It is the form
Studio Pro stores for these bindings.

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
