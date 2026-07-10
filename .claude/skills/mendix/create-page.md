# CREATE PAGE - MDL Syntax Guide

## Overview
Guide for writing CREATE PAGE statements in Mendix Definition Language (MDL).

## Syntax

```sql
create [or replace] page Module.PageName
(
  [params: { $ParamName: Module.EntityType | PrimitiveType, ... },]
  [variables: { $varName: DataType = 'defaultExpression', ... },]
  title: 'Page Title',
  layout: Module.LayoutName,
  [url: 'page-url',]
  [folder: 'FolderPath',]
  [PopupWidth: 800, PopupHeight: 480, PopupResizable: true,]
  [Class: 'css-class', Style: 'css: rule']
)
{
  -- Widget definitions using explicit properties
}
```

**Pop-up dimensions** (`PopupWidth` / `PopupHeight` / `PopupResizable`) apply when the
page is opened in a pop-up. They are optional — omitting them uses the Mendix defaults
(600 × 600, not resizable). Unlike the other header keywords, these property names are
**case-sensitive** and must be written exactly as shown. They can also be changed later
with `alter page … { set PopupWidth = …; }` (see the alter-page skill).

**Page CSS class / style** (`Class` / `Style`) set the page's Appearance — a CSS class
and inline style applied to the whole page (e.g. `Class: 'container-fluid bg-light'`).
Both are optional and can be changed later with `alter page … { set Class = '…'; }`.

**Page Variables**: Local variables at the page level for use in expressions (e.g., column visibility).
- DataType: `boolean`, `string`, `integer`, `decimal`, `datetime`
- Default value: Mendix expression in single quotes
- Referenced in expressions as `$varName`
- Use for DataGrid2 column `visible:` (which hides/shows entire column, NOT per-row)

### Key Syntax Elements

| Element | Syntax | Example |
|---------|--------|---------|
| Properties | `(key: value, ...)` | `(title: 'Edit', layout: Atlas_Core.Atlas_Default)` |
| Widget name | Required after type | `textbox txtName (...)` |
| Attribute binding | `attribute: AttrName` | `textbox txt (label: 'Name', attribute: Name)` |
| Variable binding | `datasource: $Var` | `dataview dv (datasource: $Product) { ... }` |
| Action binding | `action: type` | `actionbutton btn (caption: 'Save', action: save_changes)` |
| Database source | `datasource: database entity` | `datagrid dg (datasource: database Module.Entity)` |
| Selection binding | `datasource: selection widget` | `dataview dv (datasource: selection galleryList)` |
| CSS class | `class: 'classes'` | `container c (class: 'card mx-spacing-top-large')` |
| Inline style | `style: 'css'` | `container c (style: 'padding: 16px;')` |
| Design properties | `designproperties: [...]` | `container c (designproperties: ['Spacing top': 'Large', 'full width': on])` |

### FOLDER Option

Place pages in folders for better organization:

```sql
create page MyModule.CustomerEdit
(
  title: 'Edit Customer',
  layout: Atlas_Core.PopupLayout,
  folder: 'Customers'
)
{
  -- widgets
}

-- Nested folders (created automatically if they don't exist)
create page MyModule.OrderDetail
(
  title: 'Order Details',
  layout: Atlas_Core.Atlas_Default,
  folder: 'Orders/Details'
)
{
  -- widgets
}
```

### Styling: Class, Style, and DesignProperties

Three styling mechanisms can be applied to any widget:

**CSS Class** — Atlas UI utility classes or custom CSS classes:
```sql
container c (class: 'card mx-spacing-top-large') { ... }
actionbutton btn (caption: 'Save', class: 'btn-lg')
```

**Inline Style** — One-off CSS styles (use sparingly):
```sql
container c (style: 'background-color: #f8f9fa; padding: 16px;') { ... }
```

> **Warning:** Do NOT use `style` directly on DYNAMICTEXT widgets — it crashes MxBuild with a NullReferenceException. Wrap the DYNAMICTEXT in a styled CONTAINER instead.

**Design Properties** — Atlas UI structured properties (spacing, colors, toggles):
```sql
-- Option property: 'Key': 'Value'
container c (designproperties: ['Spacing top': 'Large', 'Background color': 'Brand Primary']) { ... }

-- Toggle property: 'Key': ON (enabled) or OFF (disabled/omitted)
container c (designproperties: ['Full width': on]) { ... }

-- Multiple types combined
actionbutton btn (caption: 'Save', designproperties: ['Size': 'Large', 'Full width': on])
```

**Dynamic Classes** — a Mendix expression evaluated at runtime that returns a
class list (applied on top of the static `class`). Root attributes in
`$currentObject` and escape single quotes by doubling them (`''`):
```sql
dynamictext ovChip (
  content: 'chip',
  class: 'ss-chip',
  dynamicclasses: 'if $currentObject/VesselClass = Mod.BoatClass.Astute then ''ss-chip--astute'' else '''''
)
```

**All can be combined on a single widget:**
```sql
container ctnHero (
  class: 'card',
  style: 'border-left: 4px solid #264AE5;',
  dynamicclasses: 'if $currentObject/Featured then ''is-featured'' else ''''',
  designproperties: ['Spacing top': 'Large', 'Full width': on]
) {
  dynamictext txtTitle (content: 'Styled Container', rendermode: H3)
}
```

> `mxcli check` **warns** (MDL-WIDGET07) when a built-in widget carries an
> unrecognized property (a typo, or a property mxcli doesn't persist) — it would
> otherwise be silently dropped on write. It is a warning, not an error, so the
> check still passes; fix the spelling or remove the property.

## Basic Examples

### Simple Page with Title

```sql
create page MyModule.HomePage
(
  title: 'Home Page',
  layout: Atlas_Core.Atlas_Default
)
{
  dynamictext welcomeText (content: 'Welcome to My App', rendermode: H1)
}
```

### Page with Multiple Widgets

```sql
create page MyModule.CustomerPage
(
  title: 'Customer Details',
  layout: Atlas_Core.Atlas_Default
)
{
  layoutgrid mainGrid {
    row row1 {
      column col1 (desktopwidth: 12) {
        dynamictext heading (content: 'Customer Information', rendermode: H2)
      }
    }
    row row2 {
      column col2a (desktopwidth: 6) {
        actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
      }
      column col2b (desktopwidth: 6) {
        actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
      }
    }
  }
}
```

## Supported Widgets

### DYNAMICTEXT Widget

Display dynamic or static text:

```sql
-- Simple text
dynamictext heading (content: 'Heading Text', rendermode: H2)

-- Text bound to page parameter attribute (use $ParamName.Attribute)
-- This preserves the parameter reference for pages with multiple parameters of the same type
dynamictext productName (content: '$Product.Name', rendermode: H3)

-- Explicit template with page parameter binding
dynamictext greeting (content: 'Welcome, {1}!', contentparams: [{1} = $Customer.Name])

-- Template with attribute from current DataView context (simple attribute name)
dynamictext email (content: 'Email: {1}', contentparams: [{1} = Email])

-- Bind directly to an attribute of the surrounding DataView/ListView/Gallery
-- entity. `Attribute: X` is shorthand for `content: '{1}', contentparams: [{1} = X]`.
dynamictext title (Attribute: Title)
```

**ContentParams Reference Types:**
| Syntax | Context | Example |
|--------|---------|---------|
| `$ParamName.Attr` | Page parameter attribute | `$Product.Name` |
| `AttrName` | Current DataView/Gallery entity | `Name`, `Email` |
| `'literal'` | String literal expression | `'Hello'` |

> **Never leave a `{N}` placeholder unbound.** `content: '{1}'` with no
> `Attribute:`/`ContentParams:` is an orphaned template — `mxcli check` rejects it
> (MDL-WIDGET04), MxBuild fails with CE0720, and Studio Pro throws a
> NullReferenceException when the widget is opened. Bind every placeholder, or use
> a plain static `content: 'text'`.

### ACTIONBUTTON Widget

Create a button with action binding:

```sql
actionbutton widgetName (caption: 'Caption', action: ACTION_TYPE [, buttonstyle: style])
```

**Action Bindings:**
- `action: save_changes` - Save changes to object
- `action: save_changes close_page` - Save and close page
- `action: cancel_changes` - Cancel changes
- `action: close_page` - Close the page
- `action: delete` - Delete object
- `action: microflow Module.MicroflowName` - Call microflow
- `action: microflow Module.MicroflowName(Param: $value)` - Call microflow with parameters
- `action: nanoflow Module.NanoflowName` - Call nanoflow (client-side)
- `action: nanoflow Module.NanoflowName(Param: $value)` - Call nanoflow with parameters
- `action: show_page Module.PageName` - Navigate to page
- `action: show_page Module.PageName(Param: $value)` - Navigate with parameters
- `action: show_page Module.PageName($Param = $value)` - Also accepted (microflow-style)
- `action: create_object Module.Entity then show_page Module.PageName` - Create and navigate

**Button Styles:** `default`, `primary`, `success`, `info`, `warning`, `danger`, `inverse`
- Case-insensitive (`primary` and `Primary` both work).
- These are the only values Mendix recognizes — anything else (a typo, or `secondary`/`link`, which Mendix has no button style for) is rejected by `mxcli check` (MDL-WIDGET02). Previously an unknown value was silently rendered as `btn-default`.

**Examples:**
```sql
-- Save with style
actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)

-- Navigate with parameter (inside DATAVIEW)
actionbutton btnEdit (caption: 'Edit', action: show_page Module.EditPage(Product: $Product))

-- Navigate with $currentObject (inside DATAGRID column)
actionbutton btnEdit (caption: 'Edit', action: show_page Module.EditPage(Product: $currentObject))

-- Call microflow with page/dataview parameter
actionbutton btnProcess (caption: 'Process', action: microflow Module.ACT_Process(Order: $Order), buttonstyle: success)

-- Call microflow with $currentObject (inside DATAGRID/LISTVIEW column)
actionbutton btnDelete (caption: 'Delete', action: microflow Module.ACT_Delete(Target: $currentObject), buttonstyle: danger)

-- Create object and show page
actionbutton btnNew (caption: 'New', action: create_object Module.Product then show_page Module.Product_Edit, buttonstyle: primary)
```

**Using `$currentObject`:**
Use `$currentObject` inside DATAGRID, LISTVIEW, or GALLERY columns to reference the current row's object. This is typically used in columns with `ShowContentAs: customContent` for action buttons.

### LINKBUTTON Widget

Similar to ActionButton but rendered as link:

```sql
linkbutton linkName (caption: 'Caption', action: ACTION_TYPE)
```

### LAYOUTGRID Widget

Create responsive grid layout:

```sql
layoutgrid gridName {
  row rowName {
    column colName (desktopwidth: 8) {
      -- Nested widgets
    }
    column col2 (desktopwidth: 4) {
      -- Nested widgets
    }
  }
}
```

**Column Width Properties:**

| Property | Values | Default | Description |
|----------|--------|---------|-------------|
| `desktopwidth` | 1-12 or `autofill` | `autofill` | Desktop column width |
| `tabletwidth` | 1-12 or `autofill` | auto | Tablet column width |
| `phonewidth` | 1-12 or `autofill` | auto | Phone column width |

```sql
column col1 (desktopwidth: 8, tabletwidth: 6, phonewidth: 12) { ... }
```

Example:
```sql
layoutgrid mainGrid {
  row row1 {
    column colMain (desktopwidth: 8) {
      dynamictext heading (content: 'Main Content', rendermode: H3)
    }
    column colSide (desktopwidth: 4) {
      dynamictext sideHeading (content: 'Sidebar', rendermode: H3)
    }
  }
}
```

### DATAGRID Widget

Display list of objects using DataGrid widget:

```sql
datagrid gridName (
  datasource: database from Module.Entity where [condition] sort by attributename asc|desc,
  selection: Multi
) {
  column colName (attribute: attributename, caption: 'Label')
}
```

> **Reserved keyword column names:** If the attribute name is an MDL reserved keyword (e.g. `Status`, `Type`), you must quote the attribute value and use a distinct widget name for the column:
> ```sql
> column colStatus (attribute: "Status", caption: 'Status')
> column colType   (attribute: "Type",   caption: 'Type')
> ```
> Writing `COLUMN Status (attribute: Status)` fails silently — `Status` and `Type` are parsed as keywords. Always use a `col`-prefixed widget name when the attribute name is reserved.

**Column Properties:**

| Property | Values | Default | Description |
|----------|--------|---------|-------------|
| `attribute` | attribute name | (required) | Attribute binding |
| `caption` | string | attribute name | Column header text |
| `Alignment` | `left`, `center`, `right` | `left` | Text alignment |
| `WrapText` | `true`, `false` | `false` | Wrap text in cell |
| `Sortable` | `true`, `false` | `true` (if attribute), `false` (if not) | Can sort column |
| `Resizable` | `true`, `false` | `true` | Can resize column |
| `Draggable` | `true`, `false` | `true` | Can reorder column |
| `Hidable` | `yes`, `hidden`, `no` | `yes` | Can hide column |
| `ColumnWidth` | `autofill`, `autoFit`, `manual` | `autofill` | Column width mode |
| `Size` | integer (px) | `1` | Width in pixels (when `ColumnWidth: manual`) |
| `visible` | expression string | `true` | Column-level visibility — hides/shows the whole column, so use page variables, NOT `$currentObject` (per-object widget visibility is different — see "Conditional Visibility and Editability") |
| `DynamicCellClass` | expression string | (empty) | Dynamic CSS class per cell |
| `tooltip` | text string | (empty) | Cell tooltip text |

Only non-default column properties appear in `describe page` output.

```sql
column colPrice (
  attribute: Price, caption: 'Unit Price',
  Alignment: right, WrapText: true,
  Sortable: false, Resizable: false,
  Hidable: hidden,
  ColumnWidth: manual, Size: 150,
  DynamicCellClass: 'if($currentObject/Price > 100) then ''highlight'' else '''' ',
  tooltip: 'Price in USD'
)
```

**Associated-attribute columns:**

A column can bind an attribute *over an association*, not just an own-entity
attribute — use a bare association path `Assoc/Attr`:

```sql
datagrid dgOrders (datasource: database from Sales.Order) {
  column colNumber   (attribute: Number, caption: 'Order #')
  column colCustomer (attribute: Order_Customer/Name, caption: 'Customer')  -- associated attr
}
```

The association name is bare (resolved against the grid's entity module);
multi-hop paths (`A/B/Attr`) are supported. Module-qualified associations
(`Module.Assoc/Attr`) are **not** accepted — use the bare name.

**Custom Content Columns:**

Columns can contain nested widgets instead of attribute bindings. These build
correctly on the default engine (mxbuild-verified, 0 errors) — an earlier CE0463
(column property ordering) was fixed:

```sql
column colActions (caption: 'Actions') {
  actionbutton btnView (caption: 'View', action: close_page)
}
```

**Supported Datasource Types:**

| Syntax | Description |
|--------|-------------|
| `datasource: database from Module.Entity` | Direct database query |
| `datasource: $Variable` | Variable bound (requires DATAVIEW parent with entity) |
| `datasource: microflow Module.GetData()` | Microflow datasource |
| `datasource: nanoflow Module.GetData()` | Nanoflow datasource (client-side, no server roundtrip) |
| `datasource: selection widgetName` | Listen to selection from another widget |
| `datasource: association path` | Retrieve by association from context (ByAssociation) |
| `datasource: $currentObject/Module.Assoc` | Sugar for `association` — same semantics, reads more naturally |

**With WHERE and SORT BY (inline in DataSource):**
```sql
datagrid dgActive (
  datasource: database from Module.Product where [IsActive = true] sort by Name asc
) {
  column colName (attribute: Name, caption: 'Name')
  column colPrice (attribute: Price, caption: 'Price')
}
```

**Complex WHERE conditions:**
```sql
datagrid dgFiltered (
  datasource: database from Module.Product
    where [IsActive = true and contains(Code, 'a') and Price > 10] or [Stock < 2]
    sort by Name asc, Price desc
) {
  column colName (attribute: Name, caption: 'Name')
}
```

**Paging Properties:**

| Property | Values | Default | Description |
|----------|--------|---------|-------------|
| `PageSize` | Any positive integer | 20 | Number of rows per page |
| `Pagination` | `buttons`, `virtualScrolling`, `loadMore` | `buttons` | Paging mode |
| `PagingPosition` | `bottom`, `top`, `both` | `bottom` | Position of paging controls |
| `ShowPagingButtons` | `always`, `auto` | `always` | When to show paging buttons |

```sql
-- Paging buttons above and below, 25 rows per page
datagrid dgProducts (
  datasource: database Module.Product,
  PageSize: 25,
  PagingPosition: both
) {
  column colName (attribute: Name, caption: 'Name')
}

-- Virtual scrolling (no paging buttons)
datagrid dgLargeList (
  datasource: database Module.Product,
  PageSize: 50,
  Pagination: virtualScrolling
) {
  column colName (attribute: Name, caption: 'Name')
}
```

Only non-default paging properties appear in `describe page` output.

### DATAVIEW Widget

Display single object with nested input widgets:

```sql
dataview dvName (datasource: $VariableName) {
  -- Nested input widgets
  textbox txtName (label: 'Name', attribute: Name)
  textarea txtDescription (label: 'Description', attribute: description)

  footer footer1 {
    actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
    actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
  }
}
```

### Input Widgets

Input widgets must be inside a DATAVIEW context. Use `attribute:` to bind to attributes:

**TEXTBOX** - Single-line text input:
```sql
textbox txtName (label: 'Label', attribute: attributename)
```

**TEXTAREA** - Multi-line text input:
```sql
textarea txtDescription (label: 'Description', attribute: description)
```

**CHECKBOX** - Boolean checkbox:
```sql
checkbox cbActive (label: 'Active', attribute: IsActive)
```

**RADIOBUTTONS** - Boolean or enum selection:
```sql
radiobuttons rbStatus (label: 'Status', attribute: status)
```

**DATEPICKER** - Date/time selection:
```sql
datepicker dpCreated (label: 'Created Date', attribute: CreatedDate)
```

**COMBOBOX** - Combo box (pluggable widget):
```sql
-- Enumeration mode (attribute is an enum type):
combobox cbCountry (label: 'Country', attribute: Country)

-- Association mode (Attribute = association, DataSource = target entity, CaptionAttribute = display attr):
combobox cmbCustomer (label: 'Customer', attribute: Order_Customer, datasource: database MyModule.Customer, CaptionAttribute: Name)
```

### DataView with Form Layout

```sql
dataview dataView1 (datasource: $Customer) {
  textbox txtName (label: 'Name', attribute: Name)
  textbox txtEmail (label: 'Email', attribute: Email)
  textarea txtAddress (label: 'Address', attribute: Address)
  combobox cbStatus (label: 'Status', attribute: status)
  checkbox cbActive (label: 'Active', attribute: IsActive)
  datepicker dpCreated (label: 'Created', attribute: CreateDate)

  footer footer1 {
    actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
    actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
  }
}
```

**Form Orientation (label placement):** the DataView's Studio Pro "Form
Orientation" radio is stored as `LabelWidth` in BSON. Specify either form in MDL:

```sql
dataview dv (datasource: $Customer, FormOrientation: Vertical)    -- label above
dataview dv (datasource: $Customer, FormOrientation: Horizontal)  -- label beside (default, LabelWidth=3)
dataview dv (datasource: $Customer, LabelWidth: 4)                -- explicit, 0..12 columns of 12
```

`LabelWidth: 0` ⇔ `FormOrientation: Vertical`. If both are given, `LabelWidth` wins.

### GALLERY Widget

Display items in card layout with selection and responsive columns:

```sql
gallery galleryName (
  datasource: database from Module.Entity sort by Name asc,
  selection: Single,
  DesktopColumns: 3,
  TabletColumns: 2,
  PhoneColumns: 1
) {
  template template1 {
    dynamictext name (content: '{1}', contentparams: [{1} = Name], rendermode: H4)
    dynamictext email (content: '{1}', contentparams: [{1} = Email])
  }
}
```

**With Filter:**
```sql
gallery productGallery (datasource: database Module.Product, selection: single) {
  filter filter1 {
    textfilter searchName (attribute: Name)
  }
  template template1 {
    dynamictext prodName (content: '{1}', contentparams: [{1} = Name], rendermode: H4)
    dynamictext prodCode (content: 'SKU: {1}', contentparams: [{1} = Code])
  }
}
```

### Filter Widgets

Filter widgets are used inside GALLERY FILTER containers to enable search/filtering:

**TEXTFILTER** - Text search filter:
```sql
-- Simple binding to single attribute
textfilter searchName (attribute: Name)

-- Multiple attributes with explicit list
textfilter textFilter1 (attributes: [Module.Entity.Name, Module.Entity.Code, Module.Entity.Description])

-- With filter type
textfilter textFilter1 (attributes: [Module.Entity.Description], filtertype: startsWith)
```

**FilterType Options:**
- `contains` (default) - Matches if attribute contains text
- `startsWith` - Matches if attribute starts with text
- `endsWith` - Matches if attribute ends with text
- `equal` - Exact match

**NUMBERFILTER** - Numeric range filter:
```sql
numberfilter priceFilter (attributes: [Module.Entity.Price])
```

**DATEFILTER** - Date range filter:
```sql
datefilter datefilter (attributes: [Module.Entity.CreateDate])
```

**DROPDOWNFILTER** - Dropdown selection filter:
```sql
dropdownfilter statusFilter (attributes: [Module.Entity.Status])
```

### NAVIGATIONLIST Widget

Create a menu with action items:

```sql
navigationlist navName {
  item itemEdit (caption: 'Edit', action: show_page Module.EditPage(entity: $EntityParameter))
  item itemDelete (caption: 'Delete', action: delete)
  item itemBack (caption: 'Back', action: close_page)
}
```

### SNIPPETCALL Widget

Embed a reusable snippet:

```sql
-- Simple snippet call
snippetcall snippetName (snippet: Module.SnippetName)

-- With parameters
snippetcall actions (snippet: Module.EntityActions, params: {entity: $currentObject})
```

### IMAGE / STATICIMAGE / DYNAMICIMAGE Widgets

Display images on pages:

```sql
-- Image with dimensions (responsive by default)
image imgLogo (width: 200, height: 100)
staticimage imgBanner (width: 400, height: 120)

-- Dynamic image (from entity data source, e.g. inside a DataView)
dynamicimage imgProduct (width: 300, height: 200)

-- Image without explicit dimensions
image imgIcon
```

**Properties:** `width: integer`, `height: integer`, `AlternativeText: 'text'`, `WidthUnit: pixels | percentage | auto`, `HeightUnit: pixels | percentage | auto`, `Responsive: true | false`, `DisplayAs: fullImage | thumbnail | icon`, `class: 'css'`, `style: 'css'`

#### Setting Image Source (PLUGGABLEWIDGET syntax)

The IMAGE shorthand creates a pluggable Image widget. For advanced properties like image source, use PLUGGABLEWIDGET syntax:

| Mode | Property | Use Case |
|------|----------|----------|
| `datasource: image` | `imageObject` | Dynamic image from entity (default) |
| `datasource: imageUrl` | `imageUrl: 'path'` | Static image from URL or file path |
| `datasource: icon` | `imageIcon` | Icon-based image |

```sql
-- Static image from file (logos, branding)
pluggablewidget 'com.mendix.widget.web.image.Image' imgLogo (
  datasource: imageUrl,
  imageUrl: 'img/logo.svg',
  widthUnit: pixels, width: 48,
  heightUnit: pixels, height: 48
)

-- Update existing IMAGE via ALTER PAGE
alter page Mod.Home {
  replace imgLogo with {
    pluggablewidget 'com.mendix.widget.web.image.Image' imgLogo (
      datasource: imageUrl, imageUrl: 'img/logo_dark.svg',
      widthUnit: pixels, width: 48, heightUnit: pixels, height: 48
    )
  }
};
```

For theme images, use paths relative to `theme/web/` (e.g., `img/logo.svg` → `theme/web/img/logo.svg`).

### CONTAINER / CUSTOMCONTAINER Widgets

Generic container for grouping widgets. `customcontainer` is an alias for `container` (both map to `Forms$DivContainer`):

```sql
-- Basic container with CSS class
container card1 (class: 'card', style: 'padding: 16px;') {
  dynamictext title (content: 'Card Title', rendermode: H4)
  dynamictext body (content: 'Card body content')
}

-- Container with design properties
container spaced1 (designproperties: ['Spacing top': 'Large', 'Full width': on]) {
  dynamictext text1 (content: 'Spaced full-width content')
}

-- Nested containers with combined styling
customcontainer outer1 (class: 'section') {
  container inner1 (class: 'card', designproperties: ['Spacing top': 'Medium']) {
    dynamictext text1 (content: 'Nested content')
  }
}
```

**Clickable container (On click action).** A container can trigger an action when
clicked — use `OnClick:` (or the equivalent `Action:` keyword) with any client
action (`microflow`, `nanoflow`, `show_page`, `save_changes`, …):

```sql
container card1 (OnClick: microflow MyModule.ACT_OpenDetails, class: 'clickable-card') {
  dynamictext title (content: 'Open details')
}
```

This maps to the Mendix Container's **On click** event (`Forms$DivContainer.OnClickAction`).
A container with no `OnClick:`/`Action:` is non-clickable (a no-op action), exactly as in Studio Pro.

### FOOTER Widget

Container for form action buttons:

```sql
footer footerName {
  actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
  actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
}
```

### HEADER Widget

Container for header content:

```sql
header headerName {
  dynamictext title (content: 'Form Title', rendermode: H3)
}
```

### CONTROLBAR Widget

Control bar for data widgets:

```sql
controlbar controlBar1 {
  actionbutton btnNew (caption: 'New', action: create_object Module.Entity then show_page Module.EditPage, buttonstyle: primary)
}
```

### Charts (Charts.mpk — ColumnChart / BarChart / AreaChart / PieChart)

Charts are pluggable widgets whose data lives in one or more `series` object-list
items. Each series binds a datasource (an OQL **view entity** is the natural feed —
one row per category) and picks X/Y attributes on that datasource. Requires the
Charts widget installed (`widgets/Charts.mpk`); run `mxcli widget init -p app.mpr`.

```sql
-- Aggregated view entity = the chart's data source
create view entity Sales.ByRegion (Region: string(100), Total: decimal) as
  select s.Region as Region, sum(s.Amount) as Total
  from Sales.Sale as s group by s.Region;

create page Sales.Dashboard (Title: 'Revenue', Layout: Atlas_Core.Atlas_Default) {
  pluggablewidget 'com.mendix.widget.web.columnchart.ColumnChart' revenueChart {
    series sRevenue (
      dataSet: static,
      DataSource: database from Sales.ByRegion,   -- or: staticDataSource: database from Sales.ByRegion
      StaticXAttribute: Region,
      StaticYAttribute: Total,
      StaticName: 'Revenue'
    )
  }
}
```

Notes:
- `DataSource:` inside a series is a friendly alias for `staticDataSource:` /
  `dynamicDataSource:` (chosen by `dataSet`); either form works.
- X/Y attributes resolve against the **series' own** datasource entity, not the page.
- Add multiple `series ( ... )` blocks for multi-series charts. BarChart/AreaChart/
  PieChart use the same `series` shape.
- **CE0463 at `mx check`**: charts can report "widget definition changed" from
  widget-version drift (embedded template vs the installed `Charts.mpk`), even for a
  chart with no series. `mxcli docker check`/`build` fix this automatically by
  running `mx update-widgets` first; if you invoke `mx check` directly, run
  `mx update-widgets <app.mpr>` (absolute path) beforehand.
- LineChart/BubbleChart/HeatMap (the `line`/`scalecolor` object-lists) are not yet
  authorable via MDL — use Studio Pro for those.

## Complete Examples

### Customer Edit Page

```sql
create or replace page CRM.CustomerEdit
(
  params: { $Customer: CRM.Customer },
  title: 'Edit Customer',
  layout: Atlas_Core.PopupLayout
)
{
  -- Wrap the form's DataView in a layout grid. Label width and input-control
  -- width are expressed in Bootstrap grid columns and only render correctly
  -- inside a layoutgrid → row → column. A DataView with input fields placed
  -- directly on the page (no grid) is flagged by lint rule MPR010 / mxcli check.
  layoutgrid mainGrid {
    row row1 {
      column col1 (desktopwidth: autofill) {
        dataview dvCustomer (datasource: $Customer) {
          textbox txtName (label: 'Name', attribute: Name)
          textbox txtEmail (label: 'Email', attribute: Email)
          textbox txtPhone (label: 'Phone', attribute: Phone)
          checkbox cbActive (label: 'Active', attribute: IsActive)

          footer footer1 {
            actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
            actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
          }
        }
      }
    }
  }
}
```

### Order Overview Page

```sql
create page Orders.OrderOverview
(
  title: 'Orders',
  layout: Atlas_Core.Atlas_Default
)
{
  layoutgrid mainGrid {
    row row1 {
      column col1 (desktopwidth: 12) {
        dynamictext heading (content: 'Order Overview', rendermode: H2)
      }
    }
    row row2 {
      column col2 (desktopwidth: 12) {
        datagrid dgOrders (datasource: database from Orders.Order sort by OrderDate desc) {
          column colNumber (attribute: OrderNumber, caption: 'Order #')
          column colDate (attribute: OrderDate, caption: 'Date')
          column colTotal (attribute: TotalAmount, caption: 'Total')
        }
      }
    }
  }
}
```

### Master-Detail Page

```sql
create page CRM.Customer_MasterDetail
(
  title: 'Customer Management',
  layout: Atlas_Core.Atlas_Default
)
{
  layoutgrid mainGrid {
    row row1 {
      -- Master list (left column)
      column colMaster (desktopwidth: 4) {
        dynamictext heading (content: 'Customers', rendermode: H3)
        gallery customerList (datasource: database from CRM.Customer sort by Name asc, selection: single) {
          template template1 {
            dynamictext name (content: '{1}', contentparams: [{1} = Name], rendermode: H4)
            dynamictext email (content: '{1}', contentparams: [{1} = Email])
          }
        }
      }

      -- Detail form (right column)
      column colDetail (desktopwidth: 8) {
        dataview customerDetail (datasource: selection customerList) {
          dynamictext detailHeading (content: 'Customer Details', rendermode: H3)
          textbox txtName (label: 'Name', attribute: Name)
          textbox txtEmail (label: 'Email', attribute: Email)
          textbox txtPhone (label: 'Phone', attribute: Phone)

          footer footer1 {
            actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
            actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
          }
        }
      }
    }
  }
}
```

## Modifying Existing Pages

To make targeted changes to an existing page (change a label, add a field, remove a widget), use `alter page` instead of `create or replace page`. ALTER PAGE modifies the widget tree in-place, preserving properties that MDL doesn't model.

```sql
-- Change a button caption and add a field
alter page Module.Customer_Edit {
  set caption = 'Save & Close' on btnSave;
  insert after txtEmail {
    textbox txtPhone (label: 'Phone', attribute: Phone)
  }
};
```

See the dedicated skill file: [ALTER PAGE/SNIPPET](./alter-page.md)

## Conditional Visibility and Editability

Any widget (including CONTAINER) can have conditional visibility. Input widgets can also have conditional editability. Use bracket syntax `[expression]`:

```sql
-- Conditionally visible widget (boolean attribute)
textbox txtName (label: 'Name', attribute: Name, visible: [IsActive])

-- Conditionally visible container
container ctnDetails (visible: [Name != '']) { dynamictext t (content: '...') }

-- Conditionally editable input (boolean)
textbox txtStatus (label: 'Status', attribute: status, editable: [CanEdit])

-- Enum comparison: use the QUALIFIED enum value. Attributes are rooted for you,
-- but a bare enum VALUE would be treated as an attribute — always qualify it.
textbox txtNotes (label: 'Notes', attribute: Notes,
  visible: [Status = MES.EquipmentStatus.Running])

-- Combined
textbox txtEmail (label: 'Email', attribute: Email,
  visible: [ShowEmail],
  editable: [CanEdit])

-- Static values still work
textbox txtReadOnly (label: 'Read Only', attribute: Name, editable: Never)
textbox txtHidden (label: 'Hidden', attribute: Name, visible: false)

-- A quoted-string expression is also accepted (CREATE and ALTER). Unlike the
-- bracket form, it is NOT auto-rooted — write $currentObject/ yourself.
dynamictext ovChip (content: 'chip', visible: '$currentObject/Name != empty')
```

> **Attribute rooting is automatic** — a bare attribute in a widget
> visibility/editability expression (`[Name != '']`, `[IsActive]`) is rooted in the
> widget data context as `$currentObject/Name != ''` for you, so it no longer
> triggers CE0117. Paths you write with an explicit `$currentObject/…` or `$Param/…`
> prefix pass through unchanged.
>
> **Enum comparison differs by context:**
> - **Widget visibility/editability expression** (per-object): qualified enum value —
>   `[Status = MES.EquipmentStatus.Running]` (the `Status` attribute is rooted for you;
>   the *value* must stay qualified, or it would be mistaken for an attribute).
> - **XPath datasource constraint** (`where […]`): the string key works —
>   `where [Status = 'Running']` (see [xpath-constraints.md](./xpath-constraints.md)).
> - **Microflow expression**: qualified value —
>   `$obj/Status = MES.EquipmentStatus.Running` (see [write-microflows.md](./write-microflows.md)).
>
> Widget-level visibility does **not** apply to **DataGrid2 column** `visible:` (next
> section), which hides/shows the whole column and must use page variables.

## Known Limitations

The following features are NOT implemented in mxcli and require manual configuration in Studio Pro:

| Feature | Workaround |
|---------|------------|
| Nested dataviews filtering by parent | Use microflow datasource or configure in Studio Pro |
| Complex conditional visibility | Configure visibility rules in Studio Pro |
| Widget-level security | Configure access rules in Studio Pro |

### Runtime Pitfalls

> **Empty CONTAINER crashes at runtime.** A CONTAINER with no child widgets compiles and builds successfully but crashes when the page loads with "Did not expect an argument to be undefined". Always include at least one child widget:
> ```sql
> -- Wrong: crashes at runtime
> CONTAINER spacer1 (Style: 'height: 6px;')
>
> -- Correct: include a child (even a space)
> CONTAINER spacer1 (Style: 'height: 6px;') {
>   DYNAMICTEXT spacerText (Content: ' ', RenderMode: Paragraph)
> }
> ```

> **`content: ''` (empty string) fails MxBuild.** An empty Content on DYNAMICTEXT causes a misleading error: "Place holder index 1 is greater than 0, the number of parameter(s)." Use a single space instead:
> ```sql
> -- Wrong: MxBuild error
> DYNAMICTEXT spacer (Content: '')
>
> -- Correct: use a space
> DYNAMICTEXT spacer (Content: ' ')
> ```

**Script Execution Note:** Script execution stops on the first error. If a page fails to create (e.g., invalid widget syntax), earlier statements in the script will have already been committed. Plan scripts with uncertain syntax in phases.

## Tips

1. **OR REPLACE**: Use to recreate existing pages
2. **Widget Names**: Required - use descriptive camelCase names
3. **Layout Requirement**: Layout must exist in the project
4. **Nesting**: Use `{ }` blocks for all widget children
5. **Properties**: Use `(key: value)` syntax for all widget properties
6. **Bindings**: Use `attribute:` for attributes, `datasource:` for data, `action:` for buttons

## Related Commands

- `alter page Module.PageName { ... }` - Modify page widgets in-place (SET, INSERT, DROP, REPLACE)
- `alter snippet Module.SnippetName { ... }` - Modify snippet widgets in-place
- `describe page Module.PageName` - View page source in MDL format (shows Class, Style, DesignProperties)
- `describe snippet Module.SnippetName` - View snippet source in MDL format
- `show pages [in module]` - List all pages
- `show widgets [where ...] [in module]` - Discover widgets across pages/snippets
- `update widgets set ... where ... [dry run]` - Bulk update widget properties (see below)
- `drop page Module.PageName` - Delete a page

### Bulk Widget Updates

Use `update widgets` to change properties across many widgets at once:

```sql
-- Preview changes first (always use DRY RUN)
update widgets set 'Class' = 'card' where widgettype like '%Container%' in MyModule dry run;

-- Apply changes
update widgets set 'showLabel' = false where widgettype like '%combobox%';

-- Multiple properties
update widgets set 'Class' = 'btn-lg', 'Style' = 'margin-top: 8px;' where widgettype like '%ActionButton%';
```

## PLUGGABLEWIDGET Escape Hatch

All shorthand widgets (IMAGE, COMBOBOX, GALLERY, DATAGRID, etc.) are pluggable widgets under the hood. When the shorthand doesn't expose a property you need, use `pluggablewidget 'widget.id' name (properties)` for full access to all widget properties.

```sql
-- Shorthand (common properties only)
image imgLogo (width: 48, height: 48)

-- Full PLUGGABLEWIDGET syntax (all properties available)
pluggablewidget 'com.mendix.widget.web.image.Image' imgLogo (
  datasource: imageUrl, imageUrl: 'img/logo.svg',
  widthUnit: pixels, width: 48, heightUnit: pixels, height: 48
)
```

Run `mxcli widget docs -p app.mpr` to generate complete property documentation for all pluggable widgets in the project. Output is saved to `.ai-context/skills/widgets/`.

## See Also

- [Overview Pages](./overview-pages.md) - CRUD page patterns
- [Master-Detail Pages](./master-detail-pages.md) - Selection binding pattern
