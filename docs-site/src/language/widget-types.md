# Widget Types

A widget is declared with a type keyword, a unique name, properties in parentheses, and optional child widgets in braces.

```sql
WIDGET_TYPE widgetName (Property: value, ...) [{ children }]
```

**The list below is not the boundary.** The built-in Mendix widgets are documented here because they have fixed, hand-written property sets. Every **pluggable or custom widget** installed in the project is also written by its own name, with a body derived from the widget's definition — see [Any installed widget](#any-installed-widget) below. If a widget is in `widgets/`, MDL can name it.

## Widget Categories

| Category | Widgets |
|----------|---------|
| Layout | `LAYOUTGRID`, `ROW`, `COLUMN`, `CONTAINER`, `CUSTOMCONTAINER` |
| Data | `DATAVIEW`, `LISTVIEW`, `DATAGRID`, `GALLERY` |
| Input | `TEXTBOX`, `TEXTAREA`, `CHECKBOX`, `RADIOBUTTONS`, `DATEPICKER`, `COMBOBOX`, `FILEMANAGER` |
| Display | `DYNAMICTEXT`, `IMAGE`, `STATICIMAGE`, `DYNAMICIMAGE` |
| Action | `ACTIONBUTTON`, `LINKBUTTON` |
| Navigation | `NAVIGATIONLIST` |
| Structure | `HEADER`, `FOOTER`, `CONTROLBAR`, `SNIPPETCALL` |

## Layout Widgets

### LAYOUTGRID

Creates a responsive grid with rows and columns. The primary layout mechanism for arranging widgets on a page:

```sql
LAYOUTGRID grid1 {
  ROW {
    COLUMN {
      TEXTBOX txtName (Label: 'Name', Attribute: Name)
    }
    COLUMN {
      TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
    }
  }
  ROW {
    COLUMN {
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
| `DynamicClasses` | Runtime-computed CSS classes (expression; stacks on `Class`) | `DynamicClasses: if $currentObject/IsActive then 'is-active' else ''` |
| `DesignProperties` | Design property values | `DesignProperties: ('Spacing top': 'Large')` |

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
  FOOTER {
    ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)
    ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)
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
  COLUMN (Attribute: OrderId, Caption: 'Order #')
  COLUMN (Attribute: OrderDate, Caption: 'Date')
  COLUMN (Attribute: Amount, Caption: 'Amount', Alignment: right)
  COLUMN (Attribute: Status, Caption: 'Status')
  CONTROLBAR {
    ACTIONBUTTON btnNew (Caption: 'New', Action: CALL MICROFLOW Sales.ACT_CreateOrder, ButtonStyle: Primary)
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

#### Database source over an association

Inside a data container, a list view can retrieve **from the database** the
objects an association of the context object points to. Studio Pro calls it a
Database source with an entity path; it keeps an XPath constraint, a sort order
and a search bar:

```sql
create snippet MyModule.TaskAssignees (Params: ($Task: System.WorkflowUserTask)) {
  listview lvAssignees (
    DataSource: database from $Task/System.WorkflowUserTask_Assignees/Administration.Account
      where [Active = true()] sort by FullName asc search by FullName
  ) {
    dynamictext txtName (Content: '{1}', ContentParams: ({1} = FullName))
  }
};
```

The path pairs each association with the entity it arrives at. Name the entity:
it may be a specialization of the association's own end (here
`Administration.Account` for `System.User`), and `DESCRIBE` always prints it. A
trailing association with no entity gets the end opposite the context.

It is a different source from `$Task/System.WorkflowUserTask_Assignees` — an
**association** source, which follows the association in memory and has no
XPath, sort or search. List views only; on any other widget it is refused.

#### Specialization templates

When the list view's entity is a **generalization**, it can render a different
body per specialization. A template is identified by the entity it renders — it
has no name, hence `TEMPLATE FOR <entity>`:

```sql
LISTVIEW vehicleListView (DataSource: DATABASE Pages.Vehicle) {
  -- the default body, used for an object no template matches
  DYNAMICTEXT defaultVehicle (Content: '{1} {2}', ContentParams: ({1} = Brand, {2} = Model))

  TEMPLATE FOR Pages.Bus {
    DYNAMICTEXT busLabel (Content: 'Bus, capacity {1}', ContentParams: ({1} = PassengerCapacity))
  }
  TEMPLATE FOR Pages.Truck {
    DYNAMICTEXT truckLabel (Content: 'Truck, max load {1} kg', ContentParams: ({1} = MaxLoadKg))
  }
}
```

Widgets written directly in the list view body are the default rendering.
Templates keep their **source order**: Mendix stores and matches in that order,
so it is authored rather than derived, and `DESCRIBE PAGE` emits them as stored.

Inside a template the context object is the specialization, so an attribute only
that specialization has still resolves — `PassengerCapacity` above exists on
`Pages.Bus`, not on `Pages.Vehicle`.

The entity must be the list view's entity or a specialization of it, and there
may be at most one template per entity; both are refused rather than written,
because a template that cannot match never renders.

> A Gallery's `TEMPLATE <name>` is a different construct — a named content slot,
> not a per-specialization body.

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
| `Editable` | Editability: `Always`, `Never`, `true`/`false`, or a condition expression (bare or quoted) | `Editable: Never` |
| `Visible` | Visibility expression | `Visible: '$showField'` |

Each input widget takes only some attribute types. Measured with mxbuild 11.14.0,
every other pairing is CE2421 ("Only attributes of type … are allowed here."):

| Widget | Attribute types |
|--------|-----------------|
| `TEXTBOX` | String, Integer, Long, Decimal, AutoNumber (and Hashed string) |
| `TEXTAREA` | String |
| `DATEPICKER` | DateTime |
| `CHECKBOX` | Boolean |
| `RADIOBUTTONS` | Boolean, Enumeration |
| `DROPDOWN` | Enumeration |

With `-p`, `check --references` reports a mismatch as **MDL-WIDGET39**, judging the
attribute at the end of an association path and on the generalization that declares
it. It also reports a bare association bound where an attribute belongs
(`Attribute: Order_Customer`), which mxbuild answers with CE1613 and which stops
`mx check` from reporting anything else. An enumeration takes `RADIOBUTTONS` or
`COMBOBOX`.

The classic drop-down (`DROPDOWN`, stored as `Forms$DropDown`) is not supported by
the React client: on a project whose Web UI setting is `OptimizedClient: Yes`
(`show settings`), mxbuild reports CE0582 and `check --references` reports
**MDL-WIDGET40**. Use `COMBOBOX` over the same attribute. Under `No` and
`MigrationMode` the drop-down builds, and nothing is reported.

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

The classic reference selector (`Forms$ReferenceSelector`) is a built-in Forms
widget mxcli has no writer for. The keyword parses, and `check` refuses it as
**MDL-WIDGET38** rather than letting `exec` stop at the page. Select an
associated object with a `COMBOBOX` over the association instead; it needs its
own `datasource:` (CE0642 without one) and a caption attribute.

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
ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)
ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)
ACTIONBUTTON btnDelete (Caption: 'Delete', Action: DELETE, ButtonStyle: Danger)
```

**Action types:**

| Action | Description |
|--------|-------------|
| `SAVE CHANGES` | Commit and close the page |
| `CANCEL CHANGES` | Roll back and close the page |
| `DELETE` | Delete the current object |
| `CLOSE PAGE` | Close the page without saving |
| `CALL MICROFLOW Module.MF_Name` | Call a microflow |
| `CALL NANOFLOW Module.NF_Name` | Call a nanoflow |
| `SHOW PAGE Module.PageName` | Open a page |
| `CREATE OBJECT Module.Entity THEN SHOW PAGE Module.PageName` | Create an object and open a page for it |
| `OPEN LINK 'https://…'` | Open a web address |
| `SIGN OUT` | Sign the user out |
| `COMPLETE TASK 'Outcome'` | Complete a workflow user task |

The snake-case spellings (`SAVE_CHANGES`, `SHOW_PAGE`, …) and `MICROFLOW …` without `CALL` are deprecated aliases (MDL-DEPR020).

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
  Action: CALL MICROFLOW Sales.ACT_ProcessOrder(Order = $Order),
  ButtonStyle: Primary
)
```

**Icon** — `Icon: 'Module.IconCollection.IconName'` gives the button an icon from
an icon collection (the modern Atlas icon set). The name must exist in the
collection, or MxBuild rejects it (CE1613):

```sql
ACTIONBUTTON btnEdit (Caption: 'Edit', Action: PAGE App.Edit,
  Icon: 'Atlas_Core.Atlas_Filled.pencil')
```

### LINKBUTTON

Renders as a hyperlink instead of a button. Same action types, styles, and
`Icon:` as `ACTIONBUTTON`:

```sql
LINKBUTTON lnkDetails (Caption: 'View Details', Action: PAGE MyModule.Customer_Detail)
LINKBUTTON lnkEdit (Caption: 'Edit', Action: PAGE App.Edit, Icon: 'Atlas_Core.Atlas_Filled.pencil')
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
  FOOTER { ... }
}
```

### FOOTER

Footer section of a DataView. Typically contains save/cancel buttons. It
stores no name of its own; `ALTER PAGE` addresses it as `<dataview>.footer`:

```sql
FOOTER {
  ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)
  ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)
}
```

### CONTROLBAR

Control bar for DataGrid widgets. Contains action buttons for the grid:

```sql
CONTROLBAR bar1 {
  ACTIONBUTTON btnNew (Caption: 'New', Action: CALL MICROFLOW Module.ACT_Create, ButtonStyle: Primary)
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

## Any installed widget

Everything above is a **built-in** widget: its keyword and properties are fixed
by Mendix and by mxcli. A **pluggable or custom widget** — DataGrid 2, Combo box,
Gallery, HTML Element, the charts, anything from the Marketplace, anything your
team built — is named the same way, by its own MDL name:

```sql
htmlelement frame (tagName: 'div', tagContentMode: 'container') {
  attribute a1 (attributeName: 'data-testid', attributeValueType: 'expression')
  tagcontentcontainer body {
    dynamictext caption (Content: 'Inside the element')
  }
}
```

Three things there come from the widget's own definition rather than from any
list in mxcli:

- **The keyword** `htmlelement` — the widget's MDL name, which is the last
  segment of its widget id.
- **The properties** `tagName`, `tagContentMode` — written with the widget's own
  spelling, exactly as `DESCRIBE WIDGET` reports them.
- **The body containers** `attribute` (an object list, one entry per repetition)
  and `tagcontentcontainer` (a child slot, holding widgets).

A property whose type in the widget's definition is **Expression** — an HTML
element attribute's `attributeValueExpression`, a chart series'
`dynamicBarColor` — takes a Mendix expression written as-is:

```sql
attribute a1 (attributeName: 'data-hot', attributeValueType: 'expression',
              attributeValueExpression: if $currentObject/Hot then 'hot' else 'cold')
```

The same unquoted expression on a property of any other type (a text template,
a plain value) is refused by `check` as **MDL-WIDGET42** rather than written
empty.

### Finding the names

Ask the widget:

```sql
DESCRIBE WIDGET TYPE htmlelement;
```

It lists every property with its type, default and enumeration members, every
body container and whether MDL can express it, and a complete example that
parses and checks as written. See
[DESCRIBE WIDGET](../reference/query/describe-widget.md).

### The explicit form

A widget can also be named by its full id. This is the fallback, not the norm —
use it when two installed packages ship the same MDL name, or when you have the
id in hand and not the name:

```sql
pluggablewidget 'com.mendix.widget.web.htmlelement.HTMLElement' frame (
  tagName: 'div'
)
```

`DESCRIBE PAGE` emits the short form wherever it round-trips, and falls back to
the id form when the name would be ambiguous.

### When a widget is not found

A name that resolves to no installed definition is an error, not a silently
accepted widget — MDL-WIDGET25, with the nearest known names suggested. A
container keyword the parent widget does not declare is MDL-WIDGET26. Both need
a project open (`-p`), since without one mxcli knows only its embedded widgets.

A third, MDL-WIDGET29, needs no project: `statictext` writes `Forms$Text`, and
Mendix has no such type. That is not a build error but a **load** error — `mx
check` and Studio Pro both stop at `TypeCacheUnknownTypeException` before any
validation runs, so the page cannot even be opened to repair it. Use
`DYNAMICTEXT` with a literal `Content:`.

If a widget you have installed is not found, its definition has not been
extracted yet:

```bash
mxcli widget init -p app.mpr     # extract definitions for every widget in widgets/
```

### Binding a text-template property

Many pluggable widgets expose **text-template** properties — an Image's
`ImageUrl` and `AlternativeText`, a TreeNode's `headerCaption`, a Timeline's
`title` / `description` / `timeIndication`. They take *text*, so a bare value is
stored as a literal and renders the same string on every row, with `check`,
`exec` and `mx check` all clean.

Bind one with the property's own `<Name>Params` companion:

```sql
image cardImage (
  ImageType: imageUrl,
  ImageUrl: '{1}',        ImageUrlParams: ({1} = PictureUrl),
  AlternativeText: '{1}', AlternativeTextParams: ({1} = Name)
)
```

The companion is the property's own name + `Params`, in whichever spelling the
template itself was written. It takes the same per-parameter `format (...)`
block a `dynamictext` does, and `DESCRIBE PAGE` emits both the template and its
companion — for the Image and for every other pluggable widget — so describe →
exec keeps the binding. Before ako/mxcli#575 the generic describe path read
attribute references and primitives only, so a TreeNode's `headerCaption` and a
Timeline's `title` were missing from its output altogether.

Two shorter spellings remain:

| Spelling | Use it for |
|----------|-----------|
| `'{AttrName}'` | one attribute, no formatting block |
| `contentparams: (...)` | a widget with a **single** text template — it is one list shared by every template on the widget |

Every `{N}` needs a matching parameter (Mendix rejects a shortfall with
`CE0720`), and parameters with no `{N}` to fill are reported as MDL-WIDGET21
rather than dropped in silence.

## Common Widget Properties

These properties are shared across many widget types:

| Property | Description | Example |
|----------|-------------|---------|
| `Class` | CSS class names | `Class: 'card p-3'` |
| `Style` | Inline CSS styles | `Style: 'margin-top: 8px;'` |
| `DynamicClasses` | Runtime-computed CSS classes (expression; stacks on `Class`) | `DynamicClasses: if $currentObject/IsActive then 'is-active' else ''` |
| `DesignProperties` | Atlas design properties | `DesignProperties: ('Spacing top': 'Large', 'Full width': ON)` |
| `Visible` | Visibility expression | `Visible: '$showSection'` |
| `Editable` | Editability: `Always`, `Never`, `true`/`false`, or a condition expression (bare or quoted) | `Editable: Never` |

## See Also

- [Pages](./pages.md) -- page overview and CREATE PAGE basics
- [Page Structure](./page-structure.md) -- layout selection and data sources
- [Data Binding](./data-binding.md) -- connecting widgets to attributes
- [ALTER PAGE](./alter-page.md) -- modifying widgets in existing pages
- [DESCRIBE WIDGET](../reference/query/describe-widget.md) -- inspect any installed widget's properties and body containers
- [Pluggable Widgets Across Versions](../guides/pluggable-widgets.md) -- how mxcli keeps widget definitions version-correct
