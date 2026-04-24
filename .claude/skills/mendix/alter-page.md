# ALTER PAGE / ALTER SNIPPET - Modify Existing Pages and Snippets

## Overview

ALTER PAGE and ALTER SNIPPET modify an existing page or snippet's widget tree **in-place** without requiring a full `CREATE OR REPLACE`. Operations work directly on the raw BSON tree, preserving widget types and properties that MDL doesn't explicitly model.

## When to Use

| Scenario | Use |
|----------|-----|
| Change a button caption, label, or style | `ALTER PAGE` with `SET` |
| Add a field to an existing form | `ALTER PAGE` with `INSERT` |
| Remove unused widgets | `ALTER PAGE` with `DROP` |
| Replace a footer or section | `ALTER PAGE` with `REPLACE` |
| Rebuild entire page from scratch | `CREATE OR REPLACE PAGE` |
| Create a new page | `CREATE PAGE` |

**Rule of thumb:** Use `ALTER PAGE` for targeted edits to a few widgets. Use `CREATE OR REPLACE PAGE` when redefining the full page structure.

## Syntax

```sql
ALTER PAGE Module.PageName {
  operation1;
  operation2;
  ...
};

ALTER SNIPPET Module.SnippetName {
  operation1;
  operation2;
  ...
};
```

Multiple operations can be combined in a single ALTER statement. They are applied sequentially.

## Operations

### SET - Modify Widget Properties

```sql
-- Single property
SET Caption = 'New Caption' ON widgetName

-- Multiple properties
SET (Caption = 'Save & Close', ButtonStyle = Success) ON btnSave

-- Page-level property (no ON clause)
SET Title = 'New Page Title'
```

**Supported SET properties:**

| Property | Widget Types | Value Type | Example |
|----------|-------------|------------|---------|
| `Caption` | ACTIONBUTTON, LINKBUTTON | String | `SET Caption = 'Submit' ON btnSave` |
| `Content` | DYNAMICTEXT | String | `SET Content = 'New Heading' ON txtTitle` |
| `Label` | TEXTBOX, TEXTAREA, DATEPICKER, COMBOBOX, CHECKBOX, RADIOBUTTONS | String | `SET Label = 'Full Name' ON txtName` |
| `ButtonStyle` | ACTIONBUTTON, LINKBUTTON | Primary, Default, Success, Danger, Warning, Info | `SET ButtonStyle = Danger ON btnDelete` |
| `Class` | Any widget | CSS class string | `SET Class = 'card mx-2' ON container1` |
| `Style` | Any widget (see warning below) | Inline CSS string | `SET Style = 'padding: 16px;' ON container1` |
| `Editable` | Input widgets | String | `SET Editable = 'Never' ON txtReadOnly` |
| `Visible` | Any widget | String or Boolean | `SET Visible = false ON txtHidden` |
| `Name` | Any widget | String | `SET Name = 'newName' ON oldName` |
| `Title` | Page-level only | String | `SET Title = 'Edit Customer'` |
| `Layout` | Page-level only | Qualified name | `SET Layout = Atlas_Core.Atlas_Default` |
| `Visible` | Any widget | Boolean or `[xpath]` | `SET Visible = false ON txtHidden` |
| `Editable` | Input widgets | Never/Always or `[xpath]` | `SET Editable = Never ON txtReadOnly` |
| `'quotedProp'` | Pluggable widgets | String, Boolean, Number | `SET 'showLabel' = false ON cbStatus` |

**Pluggable widget properties** use quoted names to set values in the widget's `Object.Properties[]`. Boolean values are stored as `"yes"`/`"no"` in BSON.

> **Warning: Style on DYNAMICTEXT** — Setting `Style` directly on a DYNAMICTEXT widget crashes MxBuild with a NullReferenceException. Wrap the DYNAMICTEXT in a CONTAINER and apply styling to the container instead:
> ```sql
> -- Wrong: crashes MxBuild
> SET Style = 'color: red;' ON txtHeading
>
> -- Correct: style the container
> REPLACE txtHeading WITH {
>   CONTAINER ctnHeading (Style: 'color: red;') {
>     DYNAMICTEXT txtHeading (Content: 'Heading', RenderMode: H2)
>   }
> }
> ```

### INSERT - Add Widgets

```sql
-- Insert after a widget
INSERT AFTER txtName {
  TEXTBOX txtMiddleName (Label: 'Middle Name', Attribute: MiddleName)
}

-- Insert before a widget
INSERT BEFORE btnSave {
  ACTIONBUTTON btnPreview (Caption: 'Preview', Action: MICROFLOW Module.ACT_Preview)
}
```

Inserted widgets use the same syntax as `CREATE PAGE`. Multiple widgets can be inserted in a single block.

### DROP - Remove Widgets

```sql
-- Drop a single widget
DROP WIDGET txtUnused

-- Drop multiple widgets
DROP WIDGET txtOldField, lblOldLabel, container2
```

Removes widgets and their entire subtree from the page.

### REPLACE - Replace Widget Subtree

```sql
-- Replace a single widget with new content
REPLACE footer1 WITH {
  FOOTER newFooter {
    ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)
    ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
  }
}
```

Replaces the target widget with one or more new widgets. The new widgets use the same syntax as `CREATE PAGE`.

### DataGrid Column Operations

DataGrid2 columns are addressable using dotted notation: `gridName.columnName`. The column name is derived from the attribute short name or caption (same as shown by `DESCRIBE PAGE`).

```sql
-- SET a column property
SET Caption = 'Product SKU' ON dgProducts.Code

-- DROP a column
DROP WIDGET dgProducts.OldColumn

-- INSERT a column after an existing one
INSERT AFTER dgProducts.Price {
  COLUMN Margin (Attribute: Margin, Caption: 'Margin')
}

-- REPLACE a column
REPLACE dgProducts.Description WITH {
  COLUMN Notes (Attribute: Notes, Caption: 'Notes')
}
```

To discover column names, run `DESCRIBE PAGE Module.PageName` and look at the COLUMN names inside the DATAGRID.

**Troubleshooting: column operation succeeds but does nothing**
If an ALTER targeting a DataGrid column completes without error but makes no change, the column name used in the statement didn't match any column. The most common cause is a mismatch between what DESCRIBE shows and what ALTER resolves internally. Derivation rules:
- Attribute-bound column → short attribute name (last segment after `.`): `Module.Entity.Description` → `Description`
- Caption-only column → sanitized caption (non-alphanumeric replaced with `_`, leading/trailing `_` trimmed): `"Order Status"` → `Order_Status`
- Caption with only special chars (e.g. `"---"`) → falls back to `col1`, `col2`, … (1-based index)

If the column name you copied from DESCRIBE still doesn't work, check whether the column has an attribute binding — attribute names take priority over captions.

### ADD Variables - Add a Page Variable

```sql
ADD Variables $showStockColumn: Boolean = 'true'
```

Adds a new page variable (`Forms$LocalVariable`) to the page/snippet. DataType can be `Boolean`, `String`, `Integer`, `Decimal`, `DateTime`, or an entity type. Default value is a Mendix expression in single quotes.

### DROP Variables - Remove a Page Variable

```sql
DROP Variables $showStockColumn
```

Removes a page variable by name.

### SET Layout - Change Page Layout

```sql
-- Auto-map placeholders by name (most common case)
SET Layout = Atlas_Core.Atlas_Default

-- Explicit mapping when placeholder names differ
SET Layout = Atlas_Core.Atlas_SideBar MAP (Main AS Content, Extra AS Sidebar)
```

Changes the page's layout without rebuilding the widget tree. Only rewrites the `FormCall.Form` and `FormCall.Arguments[].Parameter` BSON fields — all widget content is preserved. Not supported for snippets.

When placeholders have the same names in both layouts (e.g., both have `Main`), auto-mapping works. Use `MAP` when placeholder names differ between the old and new layout.

## Examples

### Change button text and style

```sql
ALTER PAGE MyModule.Customer_Edit {
  SET (Caption = 'Save & Close', ButtonStyle = Success) ON btnSave
};
```

### Add a field to a form

```sql
ALTER PAGE MyModule.Customer_Edit {
  INSERT AFTER txtEmail {
    TEXTBOX txtPhone (Label: 'Phone', Attribute: Phone)
  }
};
```

### Add a page variable for column visibility

```sql
ALTER PAGE MyModule.ProductOverview {
  ADD Variables $showStockColumn: Boolean = 'if (3 < 4) then true else false'
};
```

### Remove unused fields and update title

```sql
ALTER PAGE MyModule.Customer_Edit {
  SET Title = 'Edit Customer Details';
  DROP WIDGET txtLegacyField, lblOldNote;
  SET Label = 'Email Address' ON txtEmail
};
```

### Replace a footer section

```sql
ALTER PAGE MyModule.Customer_Edit {
  REPLACE footer1 WITH {
    FOOTER newFooter {
      ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Success)
      ACTIONBUTTON btnDelete (Caption: 'Delete', Action: DELETE, ButtonStyle: Danger)
      ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
    }
  }
};
```

### Modify a snippet

```sql
ALTER SNIPPET MyModule.NavigationMenu {
  SET Caption = 'Dashboard' ON btnHome;
  INSERT AFTER btnHome {
    ACTIONBUTTON btnReports (Caption: 'Reports', Action: SHOW_PAGE MyModule.Reports_Overview)
  }
};
```

### Set pluggable widget properties

```sql
ALTER PAGE MyModule.Customer_Edit {
  SET 'showLabel' = false ON cbStatus;
  SET 'labelWidth' = 4 ON cbCategory
};
```

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Missing `ON widgetName` for widget SET | Add `ON widgetName` (only page-level Title omits ON) |
| Using unquoted pluggable property names | Quote pluggable props: `SET 'showLabel' = false ON cb` |
| Wrong widget name | Use `DESCRIBE PAGE Module.Name` to see widget names |
| SET on non-existent widget | Widget names are case-sensitive; check with DESCRIBE |
| Missing semicolons between operations | Each operation inside `{ }` ends with `;` |

## Validation Checklist

1. **Get widget names first**: Run `DESCRIBE PAGE Module.PageName` to see all widget names
2. **Check syntax**: `mxcli check script.mdl`
3. **Check references**: `mxcli check script.mdl -p app.mpr --references`
4. **Verify result**: Run `DESCRIBE PAGE Module.PageName` after ALTER to confirm changes
5. **Validate project**: `~/.mxcli/mxbuild/*/modeler/mx check app.mpr` (or `mxcli docker check -p app.mpr`)

## Related Commands

- `DESCRIBE PAGE Module.PageName` - View current page structure (get widget names)
- `DESCRIBE SNIPPET Module.SnippetName` - View current snippet structure
- `CREATE [OR REPLACE] PAGE` - Create or fully rebuild a page
- `CREATE [OR REPLACE] SNIPPET` - Create or fully rebuild a snippet
- `UPDATE WIDGETS SET ... WHERE ...` - Bulk update widget properties across pages
- `DROP PAGE Module.PageName` - Delete a page
- `DROP SNIPPET Module.SnippetName` - Delete a snippet

## Related Skills

- [Create Page](./create-page.md) - Full page creation syntax
- [Overview Pages](./overview-pages.md) - CRUD page patterns
- [Master-Detail Pages](./master-detail-pages.md) - Selection binding pattern
