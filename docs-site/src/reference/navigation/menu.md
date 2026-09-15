# CREATE MENU

## Synopsis

```sql
CREATE [ OR MODIFY ] MENU module.name [ FOLDER 'folder_path' ] ( menu_item [ menu_item ... ] )
DESCRIBE MENU module.name
DROP MENU module.name
```

Where each `menu_item` is one of:

```sql
MENU ITEM 'caption' [ PAGE module.page | MICROFLOW module.microflow ] [ ICON module.collection.icon ] ;
MENU 'caption' [ ICON module.collection.icon ] ( nested_items ) ;
```

## Description

Manages standalone **menu documents** (`Menus$MenuDocument`) — reusable menus that a
menu widget on a page points at. Atlas_Core ships two of them, `Phone_Menu` and
`Tablet_Menu`.

A menu document is **not** the menu inside a navigation profile, although the two
are easy to confuse: both are built from the same menu items, which is why the item
syntax here is identical to the `MENU (...)` block of
[ALTER NAVIGATION](alter-navigation.md). They differ in where they live and how they
are read:

| | Profile menu | Menu document |
|---|---|---|
| Lives in | a navigation profile | its own document |
| Used by | the app's navigation | a menu widget you place on a page |
| Read with | `SHOW NAVIGATION MENU` | `DESCRIBE MENU module.name` |
| Written with | `CREATE OR REPLACE NAVIGATION` | `CREATE OR MODIFY MENU` |

`OR MODIFY` replaces the item list **wholesale**, exactly as `CREATE OR REPLACE
NAVIGATION` does: the list you give is the document's complete contents, so an
omitted item is a removed item. The document's identity, container and export level
are preserved, so menu widgets pointing at it keep working.

`DESCRIBE MENU` emits a re-executable `CREATE OR MODIFY MENU` statement, so
describe → edit → exec is the normal editing loop, and describe → exec → describe is
a fixed point.

## Parameters

`module.name`
:   Qualified name of the menu document.

`FOLDER 'folder_path'`
:   Optional. Places the document in the named module folder, creating missing folders in the path. On `CREATE OR MODIFY` this **moves** an existing document; omitting the clause leaves placement alone rather than returning the document to the module root. See [MOVE](../organization/move.md).

`'caption'`
:   The item's label, in single quotes.

`PAGE` / `MICROFLOW`
:   Optional target opened when the item is clicked. An item with neither is inert
    (stored as `Forms$NoAction`), which is normal for an item that only groups
    sub-items.

`ICON`
:   Optional icon, given as a qualified name into an icon collection — not a string.
    Hyphenated segments are double-quoted: `Atlas_Core.Atlas."layout-2"`.

## Examples

Create a menu with a nested sub-menu:

```sql
CREATE MENU MyModule.Main_Menu (
  menu item 'Home' page MyModule.Home_Web icon Atlas_Core.Atlas.home;
  menu item 'Run' microflow MyModule.DoThing;
  menu 'Admin' (
    menu item 'Accounts' page Administration.Account_Overview;
  );
  menu item 'Plain';
);
```

Replace its contents (the two omitted items are removed):

```sql
CREATE OR MODIFY MENU MyModule.Main_Menu (
  menu item 'Home' page MyModule.Home_Web icon Atlas_Core.Atlas.home;
  menu item 'Run' microflow MyModule.DoThing;
);
```

Read one back — including Atlas's own:

```sql
DESCRIBE MENU Atlas_Core.Phone_Menu;
```

The type is auto-detected too, so the bare form works:

```sql
DESCRIBE Atlas_Core.Phone_Menu;
```

From the command line:

```bash
mxcli describe menu Atlas_Core.Phone_Menu -p app.mpr
```

## Notes

**A menu item cannot open a page that takes a required parameter.** There is nowhere
to supply the argument, and Mendix rejects the model with **CE1571** ("No argument
has been selected for parameter …") reported against `Menu item`. Point the item at a
parameterless page, or call a microflow that opens the page with its argument.

**Only icon-collection icons round-trip.** `ICON` writes a
`Forms$IconCollectionIcon`. A glyph icon (which carries a numeric code) or an image
icon cannot be expressed in MDL, so `DESCRIBE` reports those on their own comment
line rather than dropping them silently:

```
-- icon a numeric glyph code (Forms$GlyphIcon) is not reproducible by CREATE MENU;
-- set it in Studio Pro
```

Re-running such output therefore loses that icon — visibly, not silently.

## See Also

- [ALTER NAVIGATION](alter-navigation.md) — the menu inside a navigation profile
- [SHOW NAVIGATION](show-navigation.md) — read profile menus and home pages
