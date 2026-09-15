# ALTER LAYOUT

## Synopsis

```sql
ALTER LAYOUT module.Name {
    operations
}
```

Where each operation is one of:

```sql
-- Insert widgets as the last children of a container
INSERT INTO containerName { widget_definitions };

-- Insert widgets into a scroll-container region (addressed by slot)
INSERT INTO scrollContainerName.top { widget_definitions };
INSERT INTO scrollContainerName.left { widget_definitions };

-- Insert widgets before or after a target
INSERT BEFORE widgetName { widget_definitions };
INSERT AFTER widgetName { widget_definitions };

-- Set a property on a widget
SET property = value ON widgetName;

-- Remove widgets
DROP WIDGET widgetName1, widgetName2;

-- Replace a widget with new widgets
REPLACE widgetName WITH { widget_definitions };
```

Repointing pages onto a different layout:

```sql
-- One page
ALTER PAGE module.Page { SET Layout = module.Layout [MAP (Old AS New, …)]; };

-- Many at once — the migration form
ALTER PAGES [IN module] SET LAYOUT = module.Layout
    [MAP (Old AS New, …)] [WHERE LAYOUT = module.OldLayout];
```

## Description

Modifies an existing layout in place. `ALTER LAYOUT` takes the whole
[ALTER PAGE](alter-page.md) operation vocabulary, because a layout's widget
tree *is* a page's plus four element types.

This is the tool for changing a layout you did not write. `CREATE OR REPLACE
LAYOUT` rebuilds the document from your MDL, which means it can only reproduce
what MDL can express; `ALTER LAYOUT` edits what is stored and leaves everything
it was not asked about untouched — including widgets MDL has no syntax for.
Measured on `Atlas_Core.Atlas_SideBar`, a describe → rename → run copy loses
both of its `Forms$SidebarToggleButton` widgets. An `ALTER LAYOUT` against it
does not.

Like `CREATE LAYOUT`, it refuses a Marketplace target and names the
copy-then-repoint route in the error.

### Addressing a region

A scroll-container region has no name of its own — its slot *is* its identity —
so it is addressed as `<scrollContainerName>.<slot>`, reusing the dotted widget
reference that also serves DataGrid 2 columns. Which one is meant is decided by
the named widget's type.

Only `INSERT INTO` accepts a region. `BEFORE` and `AFTER` position a widget
among its siblings, so name a widget for those.

An empty slot has no stored region to insert into. Add the region with
`CREATE OR REPLACE LAYOUT`.

### SET LAYOUT and the placeholder guard

A page binds to its layout's placeholders by qualified name, so repointing
rewrites both the layout reference and every placeholder binding.

Both forms **refuse** a repoint that would leave a page bound to a placeholder
the target layout does not declare:

```
Error: layout MyModule.Sidebarish does not declare the placeholder Main that
this page binds to; it has Content. Add the placeholder to the layout, or remap
the binding: `set Layout = MyModule.Sidebarish map (Main as Content)`
```

Without the check the rewrite produces a dangling binding that mxbuild reports
much later as CE1613, naming the page rather than the statement. The check runs
**after** `MAP` is applied, since `MAP` is the remedy.

In the bulk form, pages in Marketplace modules are **skipped and named** rather
than refused — a project-wide repoint that stopped dead on Administration's
pages would be unusable. A `WHERE LAYOUT` that names a layout which does not
exist is an **error**, not a "0 pages" success for a typo:

```
Error: layout not found: MyModule.NoSuchLayout
```

## Examples

Put a brand caption in the topbar of a layout you own:

```sql
ALTER LAYOUT MyModule.App_Default {
    INSERT INTO layoutContainer.top { DYNAMICTEXT brand (Content: 'My App') };
};
```

Change it, then remove it:

```sql
ALTER LAYOUT MyModule.App_Default {
    SET Content = 'Renamed' ON brand;
};

ALTER LAYOUT MyModule.App_Default {
    DROP WIDGET brand;
};
```

Add a theme switcher snippet to the topbar:

```sql
ALTER LAYOUT MyModule.App_Default {
    INSERT INTO layoutContainer.top {
        SNIPPETCALL themeBar (Snippet: MyModule.SNIPPET_ThemeBar)
    };
};
```

Move one page onto a new layout:

```sql
ALTER PAGE MyModule.Home {
    SET Layout = MyModule.App_Default;
};
```

Move every page that is still on the Atlas default — the migration:

```sql
ALTER PAGES SET LAYOUT = MyModule.App_Default
    WHERE LAYOUT = Atlas_Core.Atlas_Default;
```

Scoped to one module, whatever each page is on now:

```sql
ALTER PAGES IN MyModule SET LAYOUT = MyModule.App_Default;
```

Repoint onto a layout whose placeholder has a different name:

```sql
ALTER PAGE MyModule.Split {
    SET Layout = MyModule.Minimal MAP (HeaderLeft AS Main);
};
```

## Notes

- `DESCRIBE LAYOUT` emits re-executable MDL, so it is the way to see what a
  layout currently contains before altering it.

## See Also

[CREATE LAYOUT](create-layout.md), [ALTER PAGE / ALTER SNIPPET](alter-page.md),
[CREATE PAGE](create-page.md)
