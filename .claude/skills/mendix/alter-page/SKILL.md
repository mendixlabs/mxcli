---
name: alter-page
description: "Modify an existing page or snippet's widget tree in place with ALTER PAGE / ALTER SNIPPET — SET, INSERT, DROP, REPLACE and SET Layout. Use when changing a caption, style or property, adding or removing a widget, or reordering a form, instead of rewriting the whole page with CREATE OR REPLACE."
---

# ALTER PAGE / ALTER SNIPPET - Modify Existing Pages and Snippets

## Overview

ALTER PAGE and ALTER SNIPPET modify an existing page or snippet's widget tree **in-place** without requiring a full `create or replace`. Operations work directly on the raw BSON tree, preserving widget types and properties that MDL doesn't explicitly model.

## When to Use

| Scenario | Use |
|----------|-----|
| Change a button caption, label, or style | `alter page` with `set` |
| Add a field to an existing form | `alter page` with `insert` |
| Remove unused widgets | `alter page` with `drop` |
| Replace a footer or section | `alter page` with `replace` |
| Several related changes on the same page | `alter page` with multiple operations in one block |
| Same property across many pages (e.g., add `Class` to every Container) | `update widgets` — see `bulk-widget-updates` |
| Rebuild entire page from scratch | `create or replace page` |
| Create a new page | `create page` |

**Rule of thumb:**
- `alter page` — targeted edits to one page. Combine multiple ops in one block when they belong together.
- `update widgets` — cross-page bulk updates with `WHERE` filtering and `DRY RUN`.
- `create or replace page` — redefining the full page structure.

## Syntax

```sql
alter page Module.PageName {
  operation1;
  operation2;
  ...
};

alter snippet Module.SnippetName {
  operation1;
  operation2;
  ...
};
```

Multiple operations can be combined in a single ALTER statement. They are applied sequentially; later operations see the page state produced by earlier ones, so you can `set` on a widget you just `insert`ed.

```sql
-- Rename a column, add a sibling, drop an obsolete one — all in one block.
alter page MyMod.Product_Overview {
  set caption = 'Product Name' on dgProducts.Name;
  insert after dgProducts.Lifecycle {
    column NewCol (attribute: Sku, caption: 'SKU')
  };
  drop widget dgProducts.OldCol
};
```

For changes that should be applied across **many pages** (e.g., "add `Class='card'` to every Container in `MyMod`"), use `UPDATE WIDGETS` instead — see `bulk-widget-updates`.

## Operations

### List View Specialization Templates

A List View template has no name, so it cannot be reached by a widget ref like
every other target. Adding one reuses `INSERT INTO` with the same
`template for` block `create page` uses — a template has one spelling
everywhere. Removing one has its own form:

```sql
alter page Pages.Vehicle_Overview {
  insert into vehicleListView {
    template for Pages.Motorcycle {
      dynamictext mcLabel (content: 'Motorcycle {1}', contentparams: [{1} = Brand])
    }
  };
  drop template for Pages.SUV in vehicleListView
};
```

Naming the list view in the `drop` is required, not optional: one page can hold
two list views with a template for the same entity.

Most template edits need none of this. The widgets **inside** a template are
ordinary named widgets, so `set content = '…' on busLabel` and
`insert after busLabel { … }` already work and land in the right template. To
replace a whole template, `drop` it and `insert` the new one in the same block —
operations apply in order.

Refused, each naming the problem:

- `insert before` / `insert after` a template — templates are not siblings of the
  widgets in the list view's body, so only `insert into` makes sense.
- mixing `template for …` blocks with ordinary widgets in one `insert` — they go
  to different places (the Templates array and the default body). Use two inserts.
- a template for an entity that is not the list view's entity or a specialization
  of it — it could never match an object the list view shows.
- a second template for an entity that already has one.
- `drop template for` an entity with no template — the error names the ones that
  are there, because dropping nothing and reporting success is how a typo becomes
  a silent no-op.

### SET - Modify Widget Properties

```sql
-- Single property
set caption = 'New Caption' on widgetName

-- Multiple properties
set (caption = 'Save & Close', buttonstyle = success) on btnSave

-- Page-level property (no ON clause). Page-level property names are
-- case-sensitive and must match the Mendix property exactly.
set Title = 'New Page Title'

-- Pop-up dimensions (apply when the page is opened in a pop-up)
set PopupWidth = 800
set PopupHeight = 480
set PopupResizable = true
set Documentation = 'What this page is for.'

-- Retarget a button's on-click action. Any form `create page` accepts works
-- here, including the combined ones.
set Action = microflow Module.ACT_Other on btnSave
set Action = SAVE_CHANGES CLOSE_PAGE on btnSave
set Action = SHOW_PAGE Module.DetailPage on btnEdit

-- Retarget ONE named action slot of a pluggable widget, by the widget's own
-- property key (the same key `create page` takes: `createFileAction: …`).
set 'createFileAction' = microflow Module.ACT_CreateFile on fileUploader1
set 'onSelectionChange' = show_page Module.Detail on dgOrders

-- Rebind a data-bound widget
set DataSource = $OrderParam on dvOrder
set DataSource = microflow Module.MF_Get on dvOrder
```

**Prefer `set Action` over `replace` when only the action changes.** `replace`
rebuilds the widget from what the statement says, so any property you do not
restate — `ButtonStyle`, `Class`, design properties, tooltip — is dropped. `set`
edits the one property and leaves the rest of the widget alone.

`set Action` is refused on a widget that has no action (a plain container, say),
rather than writing a property the widget type does not define — Studio Pro
refuses to open a document with an unknown property while MxBuild tolerates it,
so a silent write would build cleanly and then fail to open.

**Supported SET properties:**

| Property | Widget Types | Value Type | Example |
|----------|-------------|------------|---------|
| `Action` | Widgets with an on-click action (ACTIONBUTTON, LINKBUTTON, clickable containers) | Any `create page` action expression | `set Action = microflow M.ACT_Go on btnSave` |
| `'<slotKey>'` | Pluggable widgets — any **action-typed** property (File Uploader `createFileAction`, DataGrid 2 `onSelectionChange`, …) | Any `create page` action expression | `set 'createFileAction' = microflow M.ACT_Create on fileUploader1` — refused, naming the widget's action slots, if the key is not action-typed |
| `caption` | ACTIONBUTTON, LINKBUTTON | String | `set caption = 'Submit' on btnSave` |
| `content` | DYNAMICTEXT | String | `set content = 'New Heading' on txtTitle` |
| `RenderMode` | DYNAMICTEXT | Text, Paragraph, H1–H6 (any case; anything else is refused) | `set RenderMode = H2 on txtTitle` |
| `label` | TEXTBOX, TEXTAREA, DATEPICKER, COMBOBOX, CHECKBOX, RADIOBUTTONS | String | `set label = 'full Name' on txtName` |
| `buttonstyle` | ACTIONBUTTON, LINKBUTTON | Primary, Default, Success, Danger, Warning, Info | `set buttonstyle = danger on btnDelete` |
| `class` | Any widget | CSS class string | `set class = 'card mx-2' on container1` |
| `style` | Any widget (see warning below) | Inline CSS string | `set style = 'padding: 16px;' on container1` |
| `editable` | Input widgets | String | `set editable = 'Never' on txtReadOnly` |
| `visible` | Any widget | String or Boolean | `set visible = false on txtHidden` |
| `Name` | Any widget | String | `set Name = 'newName' on oldName` |
| `Title` | Page-level only (case-sensitive) | String | `set Title = 'Edit Customer'` |
| `Documentation` | Page-level only (case-sensitive) | String (`''` clears) | `set Documentation = 'Coordinator triage step.'` |
| `layout` | Page-level only | Qualified name | `set layout = Atlas_Core.Atlas_Default` |
| `PopupWidth` | Page-level only (case-sensitive) | Positive integer (pixels) | `set PopupWidth = 800` |
| `PopupHeight` | Page-level only (case-sensitive) | Positive integer (pixels) | `set PopupHeight = 480` |
| `PopupResizable` | Page-level only (case-sensitive) | Boolean | `set PopupResizable = true` |
| `Class` | Page-level (case-sensitive, no ON) | CSS class string | `set Class = 'container-fluid bg-light'` |
| `Style` | Page-level (case-sensitive, no ON) | Inline CSS string | `set Style = 'min-height: 100vh'` |
| `Visible` (conditional) | Any widget | `[expression]` | `set Visible = [Name != ''] on ctnDetails` |
| `Editable` (conditional) | Input widgets | `[expression]` | `set Editable = [Active] on txtName` |
| `'quotedProp'` | Pluggable widgets | String, Boolean, Number | `set 'showLabel' = false on cbStatus` |

> **Conditional visibility/editability** — `set Visible = [expr] on widget` (and
> `Editable`) attach a per-object expression. Bare attributes are rooted in the
> widget data context automatically: `[Name != '']` becomes
> `$currentObject/Name != ''` (paths you write with `$currentObject/…`/`$Param/…`
> pass through). Setting `Editable` on a non-input widget is rejected. This mirrors
> CREATE PAGE's `visible: [...]` — see the create-page skill for enum-value rules.

**Pluggable widget properties** use quoted names to set values in the widget's `Object.Properties[]`. Boolean values are stored as `"yes"`/`"no"` in BSON.

**Column property names are case-insensitive** in MDL — `set caption = …` and `set Caption = …` both work. The internal BSON keys are dictated by the widget schema and stay case-sensitive on the storage side.

> **Warning: Style on DYNAMICTEXT** — Setting `style` directly on a DYNAMICTEXT widget crashes MxBuild with a NullReferenceException. Wrap the DYNAMICTEXT in a CONTAINER and apply styling to the container instead:
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

#### Changing a widget's DataSource

`SET DataSource` retypes a data source in place — including across shapes, e.g.
from a microflow to a page parameter:

```sql
ALTER PAGE MyModule.OrderPage {
  SET DataSource = $Order ON dvOrder;                       -- page/snippet parameter
  SET DataSource = microflow MyModule.MF_Get ON dvOrder;     -- microflow
  SET DataSource = nanoflow MyModule.NF_Get ON dvOrder;      -- nanoflow
  SET DataSource = selection dgOrders ON dvDetail;           -- listen to widget
}
```

The parameter must exist on the page (or snippet) being altered — its entity is
read from the container's own parameter list, and an unknown name is refused
rather than written as an unresolved reference.

`association` and `database` sources are **not** supported by SET. Use REPLACE
for those, which rebuilds the widget through the CREATE PAGE path and handles
every datasource type; the error message says so.

A `database` source has no single stored shape — the widget holding it decides
which element Mendix writes (a list view, a data grid and a pluggable widget
each store a different one), and SET writes the property directly rather than
rebuilding the widget, so it has nothing to choose from. This used to be
accepted and half-applied: the widget was left with a source that DESCRIBE read
back as absent and mxbuild rejected as **CE7007**, on a page `exec` had just
reported as altered (mendixlabs/mxcli#1032).

A **data view** is the one case REPLACE does not rescue: it binds to a single
object, so Mendix gives it no database form at all and the CREATE PAGE path
refuses one too. Point it at a context parameter, a microflow, a nanoflow or
`selection <widget>`, and use a list view or a data grid to show a query. The
refusal says which of the two situations you are in.

### INSERT - Add Widgets

```sql
-- Insert after a widget
insert after txtName {
  textbox txtMiddleName (label: 'Middle Name', attribute: MiddleName)
}

-- Insert before a widget
insert before btnSave {
  actionbutton btnPreview (caption: 'Preview', action: microflow Module.ACT_Preview)
}

-- Insert INTO a container — append as its last child (works on an EMPTY container)
insert into ctnToolbar {
  actionbutton btnNew (caption: 'New', action: nothing, buttonstyle: primary)
}
```

Inserted widgets use the same syntax as `create page`. Multiple widgets can be inserted in a single block.

`insert into <container>` appends as the last child of the named container — the
only way to fill an **empty** container, and handy for adding to a container/dataview
without needing a sibling to anchor to. Widgets inserted into a dataview take that
dataview's entity as their context. Supported on simple containers (container,
dataview, groupbox, scroll-container region); for a layout grid or tab container,
insert relative to a widget inside the target column/tab instead.

**The context comes from the nearest enclosing data source, whatever kind it is**
— a database or association source, a microflow/nanoflow source (the entity is
the flow's return type), or `datasource: selection <list>`, which takes the
entity of the list it listens to. A bare attribute in the inserted or replaced
widget resolves against that entity, exactly as it would in `create page`. When
no enclosing source can be resolved, the binding is written unset rather than
guessed at — `describe page` then prints `<unbound>`, and mxbuild reports
`CE0402 "No value specified."`, so re-describe the page after an ALTER that
moves data-bound widgets.

### DROP - Remove Widgets

```sql
-- Drop a single widget
drop widget txtUnused

-- Drop multiple widgets
drop widget txtOldField, lblOldLabel, container2
```

Removes widgets and their entire subtree from the page.

### REPLACE - Replace Widget Subtree

```sql
-- Replace a single widget with new content
replace footer1 with {
  footer newFooter {
    actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
    actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
  }
}
```

Replaces the target widget with one or more new widgets. The new widgets use the same syntax as `create page`.

### DataGrid Column Operations

DataGrid2 columns are addressable using dotted notation: `gridName.columnName`. The column name is derived from the attribute short name or caption (same as shown by `describe page`).

```sql
-- SET a column property
set caption = 'Product SKU' on dgProducts.Code

-- DROP a column
drop widget dgProducts.OldColumn

-- INSERT a column after an existing one
insert after dgProducts.Price {
  column Margin (attribute: Margin, caption: 'Margin')
}

-- REPLACE a column
replace dgProducts.Description with {
  column Notes (attribute: Notes, caption: 'Notes')
}
```

To discover column names, run `describe page Module.PageName` and look at the COLUMN names inside the DATAGRID.

**Troubleshooting: column operation succeeds but does nothing**
If an ALTER targeting a DataGrid column completes without error but makes no change, the column name used in the statement didn't match any column. The most common cause is a mismatch between what DESCRIBE shows and what ALTER resolves internally. Derivation rules:
- Attribute-bound column → short attribute name (last segment after `.`): `Module.Entity.Description` → `Description`
- Caption-only column → sanitized caption (non-alphanumeric replaced with `_`, leading/trailing `_` trimmed): `"Order Status"` → `Order_Status`
- Caption with only special chars (e.g. `"---"`) → falls back to `col1`, `col2`, … (1-based index)

If the column name you copied from DESCRIBE still doesn't work, check whether the column has an attribute binding — attribute names take priority over captions.

**The authored `column colFoo (...)` name is NOT how you address it.** A column carries no stored name in the Mendix model, so the name you wrote in `create page` is dropped on write — always address a column by its *derived* name (the one `describe page` shows). Using the authored name now fails with an error that lists the available column names, rather than a bare "not found".

**Duplicate captions are ambiguous and rejected.** Two dynamic-text (or custom-content) columns with the same caption derive the same name, so `ON "Amount"` can't tell them apart. mxcli now refuses the operation with an ambiguity error instead of silently mutating the first and leaving the second unreachable. Give such columns distinct captions to address them individually. (Non-attribute column handles are the caption, so `set Caption = ...` also *renames* the handle — plan multi-step caption edits accordingly.)

### ADD Variables - Add a Page Variable

```sql
add variables $showStockColumn: boolean = 'true'
```

Adds a new page variable (`Forms$LocalVariable`) to the page/snippet. DataType can be `boolean`, `string`, `integer`, `decimal`, `datetime`, or an entity type. Default value is a Mendix expression in single quotes.

### DROP Variables - Remove a Page Variable

```sql
drop variables $showStockColumn
```

Removes a page variable by name.

### SET Layout - Change Page Layout

```sql
-- Auto-map placeholders by name (most common case)
set layout = Atlas_Core.Atlas_Default

-- Explicit mapping when placeholder names differ
set layout = Atlas_Core.Atlas_SideBar map (Main as content, Extra as Sidebar)
```

Changes the page's layout without rebuilding the widget tree. Only rewrites the `FormCall.Form` and `FormCall.Arguments[].Parameter` BSON fields — all widget content is preserved. Not supported for snippets.

When placeholders have the same names in both layouts (e.g., both have `Main`), auto-mapping works. Use `map` when placeholder names differ between the old and new layout.

## Examples

### Change button text and style

```sql
alter page MyModule.Customer_Edit {
  set (caption = 'Save & Close', buttonstyle = success) on btnSave
};
```

### Add a field to a form

```sql
alter page MyModule.Customer_Edit {
  insert after txtEmail {
    textbox txtPhone (label: 'Phone', attribute: Phone)
  }
};
```

### Add a page variable for column visibility

```sql
alter page MyModule.ProductOverview {
  add variables $showStockColumn: boolean = 'if (3 < 4) then true else false'
};
```

### Remove unused fields and update title

```sql
alter page MyModule.Customer_Edit {
  set title = 'Edit Customer Details';
  drop widget txtLegacyField, lblOldNote;
  set label = 'Email Address' on txtEmail
};
```

### Replace a footer section

```sql
alter page MyModule.Customer_Edit {
  replace footer1 with {
    footer newFooter {
      actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: success)
      actionbutton btnDelete (caption: 'Delete', action: delete, buttonstyle: danger)
      actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
    }
  }
};
```

### Modify a snippet

```sql
alter snippet MyModule.NavigationMenu {
  set caption = 'Dashboard' on btnHome;
  insert after btnHome {
    actionbutton btnReports (caption: 'Reports', action: show_page MyModule.Reports_Overview)
  }
};
```

### Set pluggable widget properties

```sql
alter page MyModule.Customer_Edit {
  set 'showLabel' = false on cbStatus;
  set 'labelWidth' = 4 on cbCategory
};
```

## DataGrid 2 columns: how to address them, and what you can set

**Mendix stores no column name.** A DataGrid 2 column's schema has no name or
identifier key — the only human-facing label is its caption — so the name you
write in MDL is dropped:

```mdl
create or replace page Mod.P (...) {
  datagrid dg1 (datasource: database Mod.Item) {
    column colLabel (attribute: Label, caption: 'The Label')   -- "colLabel" is not stored
  }
};
```

`describe page` shows that column as `Label`, and that is the name `ALTER PAGE`
answers to:

```mdl
alter page Mod.P { SET Caption = 'Renamed' ON dg1.colLabel }   -- WRONG: column not found
alter page Mod.P { SET Caption = 'Renamed' ON dg1.Label }      -- correct
```

The derived name is, in order: **the bound attribute's short name**, else the
**sanitized caption**, else **`colN`** by position. `mxcli check` reports
**MDL-WIDGET16** when the name you wrote differs from the one that will address
the column, so you find out at authoring time rather than from a failed ALTER.

Two columns that derive the same name are ambiguous and ALTER refuses rather than
picking one — give them distinct captions.

### Setting column properties

Property names resolve against the keys the installed widget declares, so both
the schema key and mxcli's MDL alias work (`DynamicCellClass` and `ColumnClass`
both reach `columnClass`). An unknown name lists what *is* settable on that grid.

**`DynamicCellClass` (and a widget's `DynamicClasses`) take a Mendix expression,
written as-is.** A quoted value is a Mendix string, so a literal CSS class is just
the quoted class name, and a computed one is the expression itself:

```mdl
-- a literal class: the string 'highlight'
alter page Mod.P { SET DynamicCellClass = 'highlight' ON dg1.Label }

-- a computed class
alter page Mod.P { SET DynamicCellClass = if $currentObject/Price > 100 then 'highlight' else '' ON dg1.Label }

-- WRONG: a bare name is an identifier, not a string — mxbuild reports CE0117
alter page Mod.P { SET DynamicCellClass = highlight ON dg1.Label }
```

The old spelling — the expression's text in quotes, `'if … then ''a'' else '''''`
— is refused as MDL-WIDGET33, because it would now store that text as a class
name. This applies equally to `create page`; the two paths behave identically.

A column's pluggable `Visible` expression is not converted yet: there a quoted
value is still the expression's text, so a literal needs the doubled quotes.

Properties holding a **structured** value — `attribute`, `filter`, `content`,
actions — cannot be set by ALTER at all. It refuses them and points at
`create or replace page`, rather than writing a string where Mendix expects a
reference.

**Widget property names are matched case-insensitively**, pluggable ones
included, so a spelling `CREATE PAGE` accepts is a spelling `ALTER PAGE` accepts
— `set PageSize = 10 on dgProducts` and `set pageSize = 10 on dgProducts` are the
same statement. This is what makes DESCRIBE output re-executable: `describe page`
prints the capitalised `PageSize:`, while the widget template stores `pageSize`
(mendixlabs/mxcli#1069). A property the widget does not declare is still an
error — and `mxcli check … --references` reports it **before** the script runs,
so a typo no longer lands halfway through. The pre-flight resolves the name
against the stored document rather than a list, so it is right about whatever
widget package this project has installed; the error names the widget's own
property keys. `ON` a widget the page does not have is caught the same way.

Two things it deliberately stays quiet about, because it cannot answer them: a
page the script itself creates (nothing is stored yet — the widgets there are
checked where they are written), and a widget an `INSERT` in the same script
adds. Both still fail at exec if they are genuinely wrong.

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Missing `on widgetName` for widget SET | Add `on widgetName` (only page-level properties — `Title`, `Documentation`, `PopupWidth`, `PopupHeight`, `PopupResizable`, `Class`, `Style` — omit ON) |
| `unsupported page-level property: title` | Page-level property names are case-sensitive — use `Title`, `PopupWidth`, `PopupHeight`, `PopupResizable`, `Class`, `Style` |
| Using unquoted pluggable property names | Quote pluggable props: `set 'showLabel' = false on cb` |
| `pluggable property "X" not found` | The widget does not declare it — casing is not the problem (any casing resolves). The error lists the keys it does declare; `describe widget <type>` or `describe page` shows them in context. Run `mxcli check … --references` to get this before the script runs |
| Wrong widget name | Use `describe page Module.Name` to see widget names |
| SET on non-existent widget | Widget names are case-sensitive; check with DESCRIBE |
| Missing semicolons between operations | Each operation inside `{ }` ends with `;` |

## Limitations — prefer binding at page creation (ledger finding #45)

`ALTER PAGE` is best for *content* edits (add/remove/retitle widgets). Three things
it cannot do; when you hit them, define the referenced microflows **before** the
page and bind the buttons at creation time instead of rewiring afterwards:

1. **`SET` cannot rewire a button's action.** `set` accepts a fixed property list
   (`caption`, `class`, `visible`, …) — `action` is not on it, so
   `set Action = microflow … on btnSave` is a parse error. Set the button's action
   when the button is created (or `REPLACE` the button subtree).

2. **`REPLACE` cannot reuse a widget name that lives inside the subtree being
   replaced.** The replacement is *built* (registering its widget names) before the
   old subtree is removed, so reusing e.g. `btnSave` collides with the still-present
   old `btnSave` ("duplicate widget name 'btnSave'"). Give the replacement widgets
   fresh names, or rebuild the whole page with `create or replace page`.

3. **A footer is not addressable by its author-given name.** A `footer myName { … }`
   is a *marker*: its children are hoisted into the data view's footer and the
   footer itself is serialized as `footer1`, so `drop widget myName` (and even
   `drop widget footer1`) report "not found". To change footer contents, edit the
   children by their own names, or `create or replace page`.

**Recommended pattern**: put save/reset microflows in a file that runs *before* the
page definition, and bind the popup/footer buttons to them at creation. The
apparent "page needs microflow, microflow needs page" cycle usually exists only
between *different* microflows, not within the page itself.

## Validation Checklist

1. **Get widget names first**: Run `describe page Module.PageName` to see all widget names
2. **Check syntax**: `mxcli check script.mdl`
3. **Check references**: `mxcli check script.mdl -p app.mpr --references`
4. **Verify result**: Run `describe page Module.PageName` after ALTER to confirm changes
5. **Validate project**: `mxcli docker check -p app.mpr` (or `mxcli docker check -p app.mpr`)

## Related Commands

- `describe page Module.PageName` - View current page structure (get widget names)
- `describe snippet Module.SnippetName` - View current snippet structure
- `create [or replace] page` - Create or fully rebuild a page
- `create [or replace] snippet` - Create or fully rebuild a snippet
- `update widgets set ... where ...` - Bulk update widget properties across pages
- `drop page Module.PageName` - Delete a page
- `drop snippet Module.SnippetName` - Delete a snippet

## Related Skills

- [Create Page](../create-page/SKILL.md) - Full page creation syntax
- [Overview Pages](../overview-pages/SKILL.md) - CRUD page patterns
- [Master-Detail Pages](../master-detail-pages/SKILL.md) - Selection binding pattern
