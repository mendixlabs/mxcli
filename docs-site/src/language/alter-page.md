# ALTER PAGE / ALTER SNIPPET

The `ALTER PAGE` and `ALTER SNIPPET` statements modify an existing page or snippet's widget tree in-place, without requiring a full `CREATE OR REPLACE`. This is especially useful for incremental changes: adding a field, changing a button caption, or removing an unused widget.

ALTER operates directly on the raw widget tree, preserving any widget types that MDL does not natively support (pluggable widgets, custom widgets, etc.).

## Syntax

```sql
ALTER PAGE <Module>.<Name> {
  <operations>
};

ALTER SNIPPET <Module>.<Name> {
  <operations>
};
```

## Operations

### SET -- Modify Widget Properties

Change one or more properties on a widget identified by name:

```sql
mdl 1;
-- Single property
ALTER PAGE Module.EditPage {
  SET (Caption: 'Save & Close') ON btnSave
};

-- Multiple properties at once
ALTER PAGE Module.EditPage {
  SET (Caption: 'Save & Close', ButtonStyle: Success) ON btnSave
};
```

**Supported SET properties:**

| Property | Description | Example |
|----------|-------------|---------|
| `Caption` | Button/link caption | `SET (Caption: 'Submit') ON btnSave` |
| `Label` | Input field label | `SET (Label: 'Full Name') ON txtName` |
| `RenderMode` | Dynamic text rendering: `Text`, `Paragraph`, `H1`–`H6` | `SET (RenderMode: H2) ON txtTitle` |
| `Placeholder` | Input hint text (text box, text area, date picker) | `SET (Placeholder: 'Enter your name') ON txtName` |
| `Password` | Mask a text box's input | `SET (Password: true) ON txtPin` |
| `Validation` | A text box's validation expression | `SET (Validation: '$value != empty') ON txtName` |
| `ValidationMessage` | Message shown when the validation fails | `SET (ValidationMessage: 'Name is required') ON txtName` |
| `ButtonStyle` | Button visual style | `SET (ButtonStyle: Danger) ON btnDelete` |
| `Class` | CSS class names | `SET (Class: 'card p-3') ON cMain` |
| `Style` | Inline CSS | `SET (Style: 'margin: 8px;') ON cBox` |
| `DynamicClasses` | Runtime-computed CSS classes | `SET (DynamicClasses: if $currentObject/IsActive then 'is-active' else '') ON cMain` |
| `Editable` | Editability: `Always`, `Never`, or a bare condition expression (the quoted form is refused in `SET`) | `SET (Editable: Never) ON txtEmail` |
| `Visible` | Visibility expression | `SET (Visible: '$showField') ON txtPhone` |
| `Name` | Widget name | `SET (Name: 'txtFullName') ON txtName` |

### SET -- Page-Level Properties

Omit the `ON` clause to set page-level properties. These names are case-sensitive:
`Title`, `Documentation`, `Class`, `Style`, `PopupWidth`, `PopupHeight`,
`PopupResizable`.

`Documentation` is the same property the `/** … */` doc comment on `CREATE PAGE`
writes. Before it was settable here, documenting an existing page meant re-running
its create — which for a real page means re-emitting its whole widget tree. Setting
it to `''` clears it.

```sql
ALTER PAGE Module.EditPage {
  SET (Title: 'Customer Details');
  SET (Class: 'container-fluid bg-light');  -- page CSS class (Forms$Appearance)
  SET (Style: 'min-height: 100vh')          -- page inline style
};
```

### SET -- Pluggable Widget Properties

Use quoted property names to set properties on pluggable widgets (ComboBox, DataGrid2, etc.):

```sql
ALTER PAGE Module.EditPage {
  SET ('showLabel': false) ON cbStatus
};
```

### SET Layout -- Change Page Layout

Switch a page's layout without rebuilding the widget tree. All widget content is preserved -- only the layout reference and placeholder mappings are updated.

```sql
mdl 1;
-- Auto-map placeholders by name (common case)
ALTER PAGE Module.EditPage {
  SET Layout = Atlas_Core.Atlas_Default
};

-- Explicit mapping when placeholder names differ
ALTER PAGE Module.EditPage {
  SET Layout = Atlas_Core.Atlas_SideBar MAP (Main AS Content, Extra AS Sidebar)
};
```

When both the old and new layouts share the same placeholder names (e.g., both have `Main`), no `MAP` clause is needed -- placeholders are matched automatically. Use `MAP` when the new layout has different placeholder names.

Not supported for snippets (snippets don't have layouts).

### INSERT -- Add Widgets

Insert new widgets before or after an existing widget:

```sql
mdl 1;
-- Insert after a widget
ALTER PAGE Module.EditPage {
  INSERT AFTER txtEmail {
    TEXTBOX txtPhone (Label: 'Phone', Attribute: Phone)
    TEXTBOX txtFax (Label: 'Fax', Attribute: Fax)
  }
};

-- Insert before a widget
ALTER PAGE Module.EditPage {
  INSERT BEFORE btnSave {
    ACTIONBUTTON btnPreview (Caption: 'Preview', Action: CALL MICROFLOW Module.ACT_Preview)
  }
};

-- Insert INTO a container — append as its last child (works on an empty container)
ALTER PAGE Module.EditPage {
  INSERT INTO ctnToolbar {
    ACTIONBUTTON btnNew (Caption: 'New', Action: NOTHING, ButtonStyle: Primary)
  }
};
```

`INSERT INTO <container>` appends widgets as the container's last children — the only way to fill an **empty** container, and handy for adding to a container or data view without a sibling to anchor to. Widgets inserted into a data view take that data view's entity. Supported on simple containers; for a layout grid or tab container, insert relative to a widget inside the target column/tab instead.

The inserted widgets use the same syntax as in `CREATE PAGE`. Multiple widgets can be inserted in a single block.

### DROP -- Remove Widgets

Remove one or more widgets by name:

```sql
mdl 1;
ALTER PAGE Module.EditPage {
  DROP txtUnused
};

-- Multiple widgets
ALTER PAGE Module.EditPage {
  DROP txtFax, txtPager, btnObsolete
};
```

Dropping a container widget also removes all of its children.

### REPLACE -- Replace a Widget

Replace a widget (and its subtree) with new widgets:

```sql
ALTER PAGE Module.EditPage {
  REPLACE txtOldField WITH {
    TEXTAREA txtNotes (Label: 'Notes', Attribute: Notes)
  }
};
```

### Page Variables

Add or remove page-level variables:

```sql
mdl 1;
-- Add a variable
ALTER PAGE Module.EditPage {
  ADD Variables $showAdvanced: Boolean = false
};

-- Remove a variable
ALTER PAGE Module.EditPage {
  DROP Variables $showAdvanced
};
```

## Parameters

Add or remove a page or snippet parameter without rewriting the page. A
parameter is declared exactly as in `CREATE PAGE`'s `Params:` — an entity or a
primitive (`String`, `Integer`, `Long`, `Decimal`, `Boolean`, `DateTime`). A
snippet parameter must be an entity (MDL087; mxbuild reports CE0046).

```sql
mdl 1;
-- A page with a Url needs a {Name} segment for every parameter (CE5601), so
-- set the URL in the same statement. A page with no Url needs no SET.
ALTER PAGE Module.Customer_Edit {
  SET (Url: 'customer-edit/{Customer}/{Order}');
  ADD Parameters $Order: Module.Order;
};

-- Remove a parameter. Refused while a data source or an expression on the
-- page still uses it.
ALTER PAGE Module.Customer_Edit {
  DROP Parameters $Order;
};
```

Callers are not updated: a page that opens this one, or a page that places
this snippet, has to pass the new parameter, and `mxcli docker check` reports
the ones that do not.

## Combining Operations

Multiple operations can be combined in a single ALTER statement. They are applied in order:

```sql
ALTER PAGE Module.Customer_Edit {
  -- Change button appearance
  SET (Caption: 'Save & Close', ButtonStyle: Success) ON btnSave;

  -- Remove unused fields
  DROP txtFax;

  -- Add new fields after email
  INSERT AFTER txtEmail {
    TEXTBOX txtPhone (Label: 'Phone', Attribute: Phone)
    TEXTBOX txtMobile (Label: 'Mobile', Attribute: Mobile)
  };

  -- Replace old status dropdown with combobox
  REPLACE ddStatus WITH {
    COMBOBOX cbStatus (Label: 'Status', Attribute: Status)
  }
};
```

## Workflow Tips

1. **Discover widget names first** -- Run `DESCRIBE PAGE Module.PageName` to see the current widget tree with all widget names.

2. **Use ALTER for small changes** -- For adding a field or changing a caption, ALTER is faster and safer than `CREATE OR REPLACE`, because it preserves widgets that MDL cannot round-trip (pluggable widgets with complex configurations). `CREATE OR REPLACE` / `CREATE OR MODIFY` keeps a pluggable widget exactly as stored only while its statement is unchanged from what `DESCRIBE` prints for it; editing any property of that widget rebuilds it from its template, and resets what MDL cannot express (translations, unmapped properties).

3. **Use CREATE OR REPLACE for major rewrites** -- When restructuring the entire page layout, a full replacement is cleaner.

## Examples

### Add a Field to an Edit Page

```sql
mdl 1;
-- First, check what's on the page
DESCRIBE PAGE MyModule.Customer_Edit;

-- Add a phone field after email
ALTER PAGE MyModule.Customer_Edit {
  INSERT AFTER txtEmail {
    TEXTBOX txtPhone (Label: 'Phone Number', Attribute: Phone)
  }
};
```

### Change Button Behavior

```sql
ALTER PAGE MyModule.Order_Edit {
  SET (Caption: 'Submit Order', ButtonStyle: Success) ON btnSave;
  SET (Caption: 'Discard') ON btnCancel
};
```

### DataGrid Column Operations

Mendix stores no name on a DataGrid 2 column, so `DESCRIBE PAGE` prints none and a
column is addressed by what it shows: `grid column(Attr)` for the column bound to
`Attr` (written as describe writes it, `Owner/Name` over an association), or
`grid column('Caption')` for the column with that caption.

```sql
mdl 1;
-- Add a column after an existing one
ALTER PAGE MyModule.Customer_Overview {
  INSERT AFTER dgCustomers column(Email) {
    COLUMN (Attribute: Phone, Caption: 'Phone')
  }
};

-- Remove a column
ALTER PAGE MyModule.Customer_Overview {
  DROP dgCustomers column('Old column')
};

-- Change a column's caption
ALTER PAGE MyModule.Customer_Overview {
  SET (Caption: 'E-mail Address') ON dgCustomers column(Email)
};

-- Replace a column
ALTER PAGE MyModule.Customer_Overview {
  REPLACE dgCustomers column(Notes) WITH {
    COLUMN (Attribute: Description, Caption: 'Description')
  }
};
```

Two columns over the same attribute, or with the same caption, share the address.
ALTER refuses it and lists the matches; `@n` picks one: `DROP dgCustomers column(Email)@2`.

The older dotted form `dgCustomers.Email` still works. It matches a name mxcli
derives — the attribute's short name, else the sanitized caption, else `colN` by
position — which describe no longer prints.

## See Also

- [Pages](./pages.md) -- page overview and CREATE PAGE basics
- [Widget Types](./widget-types.md) -- widgets available for INSERT and REPLACE
- [Snippets](./snippets.md) -- ALTER SNIPPET uses the same operations
- [Common Patterns](./page-patterns.md) -- page layout patterns
