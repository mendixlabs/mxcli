# Pages

Pages define the user interface of a Mendix application. Each page consists of a widget tree arranged within a layout, with data sources that connect widgets to the domain model.

## Core Concepts

| Concept | Description |
|---------|-------------|
| **Layout** | A reusable page template that defines content regions (e.g., header, sidebar, main content) |
| **Widget tree** | A hierarchical structure of widgets that defines the page's visual content |
| **Data source** | Determines how a widget obtains its data (page parameter, database query, microflow, etc.) |
| **Widget name** | Every widget has a unique name within the page, used for ALTER PAGE operations. Layout-grid rows and columns, data-grid columns, and slot blocks such as a gallery's `template` have none (Mendix stores none); a data-grid column is addressed as `grid column(Attr)` |

## CREATE PAGE

The basic syntax for creating a page:

```sql
CREATE [OR REPLACE] PAGE <Module>.<Name> [FOLDER '<path>']
(
  [Params: ( $Param: Module.Entity | Type [, ...] ),]
  Title: '<title>',
  Layout: <Module.LayoutName>
  [, Class: '<css-class>', Style: '<css: rule>']
)
{
  <widget-tree>
}
```

### Minimal Example

```sql
CREATE PAGE MyModule.Home
(
  Title: 'Welcome',
  Layout: Atlas_Core.Atlas_Default
)
{
  CONTAINER cMain {
    DYNAMICTEXT txtWelcome (Content: 'Welcome to the application')
  }
};
```

### Page with Parameters

Pages can receive entity objects or primitive values as parameters from the calling context:

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
    FOOTER {
      ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)
      ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)
    }
  }
};
```

## Page Properties

| Property | Description | Example |
|----------|-------------|---------|
| `Params` | Page parameters (entity objects or primitives) | `Params: ( $Order: Sales.Order, $Qty: Integer )` |
| `Title` | Page title shown in the browser/tab | `Title: 'Edit Customer'` |
| `Layout` | Layout to use for the page | `Layout: Atlas_Core.PopupLayout` |
| `Variables` | Page-level variables for conditional logic | `Variables: ( $show: Boolean = true )` |
| `Class` | CSS class applied to the page (Forms$Appearance) | `Class: 'container-fluid bg-light'` |
| `Style` | Inline CSS style applied to the page | `Style: 'min-height: 100vh'` |

The folder is a clause after the name, not a property: `CREATE PAGE MyModule.Customer_Edit FOLDER 'Pages/Customers' (…)`. The `Folder: '…'` property still parses as a deprecated alias (`MDL-DEPR105`); `mxcli fmt --upgrade` moves it.

## Widget Properties

### Responsive Column Widths

Layout grid columns support responsive widths for desktop, tablet, and phone:

```sql
COLUMN col1 (DesktopWidth: 8, TabletWidth: 6, PhoneWidth: 12) { ... }
```

Values are 1-12 (grid units), `AutoFill` or `AutoFit` (Studio Pro's "Auto-fit content"). TabletWidth and PhoneWidth default to `AutoFill` when omitted.

### Conditional Visibility

Any widget can be conditionally visible. The condition is a Mendix client expression, written bare and stored as written, so an attribute of the context object is `$currentObject/Attr`:

```sql
TEXTBOX txtName (Label: 'Name', Attribute: Name, Visible: $currentObject/IsActive)
```

Static values also work: `Visible: false` hides the widget unconditionally.

The older bracketed form, `Visible: [IsActive]`, still parses — it roots a bare attribute in `$currentObject` — and warns `MDL-DEPR081`; `mxcli fmt --upgrade` rewrites it to the expression it stores. A constant condition such as `Editable: [false]` has no bare spelling and keeps its brackets.

Studio Pro's **"based on attribute value"** form lists the values of a Boolean or
enumeration attribute (of the enclosing data container's entity) that show the
widget; `empty` is Studio Pro's "(empty)" choice:

```sql
CONTAINER cntRunning (Visible: Status in (Running, empty)) { ... }
TEXTBOX txtPassword (Label: 'Password', Attribute: Password, Visible: IsLocalUser in (true))
```

mxcli writes one condition per value of the attribute, as Studio Pro does, so a
value not listed hides the widget. `describe page` emits this form for widgets
set up that way in Studio Pro.

Studio Pro's **"Visible for module roles"** restricts a widget to users holding
one of the named module roles. It is a separate setting, so it combines with an
expression or attribute condition — the widget then carries two `Visible:` lines:

```sql
mdl 1;
create page MyModule.Request_Review (Title: 'Review', Layout: Atlas_Core.Atlas_Default) {
  container cntAdmin (Visible: roles (MyModule.Administrator)) {
    dynamictext txtAdmin (Content: 'Administrators only')
  }
  actionbutton btnClose (Caption: 'Close', Action: close page,
    Visible: false, Visible: roles (MyModule.Administrator, MyModule.Editor))
};
```

It is accepted on widgets, tab pages and navigation-list items. `mxcli check
--references` refuses a role the project (or the script, above the statement)
does not define. `alter page … set Visible` changes only the condition and keeps
the stored roles.

### Conditional Editability

Input widgets can be conditionally editable:

```sql
TEXTBOX txtStatus (Label: 'Status', Attribute: Status, Editable: $currentObject/Status != 'Closed')
```

Static values: `Editable: Never`, `Editable: Always`; `Editable: false` and `Editable: true` mean `Never` and `Always`.

The condition may also be written as a quoted string, exactly as for `Visible:` — `Editable: '$currentObject/Status != ''Closed'''` stores the same condition as the bare form above (a quote inside the string is doubled). Any other value, such as a misspelt `Neever`, is refused by `mxcli check` (`MDL-WIDGET19`) instead of leaving the field editable.

### Pluggable Widgets

A pluggable widget takes `Visible:` in all the forms above, and `Editable:` when
its widget package declares the Editability system property —
`<systemProperty key="Editability"/>` in its widget XML. The combo box declares it:

```sql
COMBOBOX cmbStatus (Label: 'Status', Attribute: Status, Editable: Never, Visible: $currentObject/Title != empty)
```

Visibility is available on every pluggable widget, as in Studio Pro: a declared
`<systemProperty key="Visibility"/>` only places the setting in the widget's own
tabs, and a data grid, which declares none, still takes `Visible:`. A widget whose
package does not declare Editability (a data grid, an image) has no such setting
in Studio Pro either, so `mxcli check` refuses `Editable:` on it as `MDL-WIDGET41`.

## Layouts

Layouts are referenced by their qualified name (`Module.LayoutName`). Common Atlas layouts include:

| Layout | Usage |
|--------|-------|
| `Atlas_Core.Atlas_Default` | Full-page layout with navigation sidebar |
| `Atlas_Core.PopupLayout` | Modal popup dialog |
| `Atlas_Core.Atlas_TopBar` | Layout with top navigation bar |

## DROP PAGE

Removes a page from the project:

```sql
DROP PAGE MyModule.Customer_Edit;
```

## Inspecting Pages

Use `SHOW` and `DESCRIBE` to examine existing pages:

```sql
-- List all pages in a module
LIST PAGES IN MyModule;

-- Show the full MDL definition of a page (round-trippable)
DESCRIBE PAGE MyModule.Customer_Edit;
```

The output of `DESCRIBE PAGE` can be used as input to `CREATE OR REPLACE PAGE` for round-trip editing.

## See Also

- [Page Structure](./page-structure.md) -- layout selection, content areas, and data sources
- [Widget Types](./widget-types.md) -- full catalog of available widgets
- [Data Binding](./data-binding.md) -- connecting widgets to entity attributes
- [Snippets](./snippets.md) -- reusable page fragments
- [ALTER PAGE](./alter-page.md) -- modifying existing pages in-place
- [Common Patterns](./page-patterns.md) -- list page, edit page, master-detail patterns
