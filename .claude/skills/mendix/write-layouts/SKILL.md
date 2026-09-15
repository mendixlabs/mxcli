---
name: write-layouts
description: "CREATE LAYOUT syntax — the frame a page is built on: scroll-container regions, the navigation tree, and the placeholders pages bind to. Use when a topbar, sidebar or page frame has to change, or when copying an Atlas layout into a module you own."
---

# CREATE LAYOUT — the frame a page is built on

A **layout** is the frame every page renders inside: the topbar, the navigation
sidebar, and the hole the page's own content drops into. Until now it was the one
document mxcli could not author, which put the topbar out of reach of MDL.

## Do not edit Atlas_Core

Mendix's own documentation says it plainly: *"Do not change the supplied layouts.
Either create a separate module with the custom layouts, page templates, and
building blocks or create your own."* An Atlas layout lives in a Marketplace
module, and a Marketplace update replaces the module wholesale — every local edit
is gone, silently.

`CREATE LAYOUT` **refuses** a Marketplace module for that reason. Put your layout
in a module you own.

The usual starting point is a copy of an Atlas layout, and `describe` already
gives you one:

```bash
mxcli -p app.mpr -c "describe layout Atlas_Core.Atlas_Default" > mine.mdl
# edit the qualified name to your own module, then:
mxcli exec mine.mdl -p app.mpr
```

`DESCRIBE LAYOUT` emits re-executable MDL, so describe → rename → exec *is* the
copy operation. There is no `COPY DOCUMENT` verb and none is needed.

**A copy is only as good as what MDL can spell.** A widget describe cannot render
comes out as a comment ending `-- NOT re-executable`, and re-running the script
drops it. Measured on `Atlas_Core.Atlas_SideBar`: its two
`Forms$SidebarToggleButton` widgets do not survive the round trip, and an
`image` widget copied this way loses its image reference (CE0463 until
`mxcli fix widgets`, then "No image selected"). **Read the describe output
before running it** — the comments name exactly what will be lost. To change a
layout without that risk, use `ALTER LAYOUT`, which edits the stored document in
place and leaves everything it was not asked to touch alone.

### Removing a layout

```sql
DROP LAYOUT MyModule.App_Old;
```

Pages still bound to it are **named in a warning and the drop proceeds** — it is
not refused. Left dropped, each of those pages fails the build with **CE1613**
("The selected layout … no longer exists"), which names the *page* and never the
layout, so repoint them first:

```sql
ALTER PAGES SET LAYOUT = MyModule.App_New WHERE LAYOUT = MyModule.App_Old;
DROP LAYOUT MyModule.App_Old;
```

To *correct* a layout rather than remove it, re-create it under the same name
(`CREATE OR REPLACE LAYOUT`): the pages stay bound by qualified name and rebind
to the new document — verified end to end, the pages go back to 0 errors.

### Repointing pages

A new layout that no page uses changes nothing:

```sql
-- one page
alter page MyModule.Home { set Layout = MyModule.App_Default; };

-- the migration: every page currently on Atlas_Default
alter pages set layout = MyModule.App_Default
  where layout = Atlas_Core.Atlas_Default;

-- scoped to one module, whatever each page is on now
alter pages in MyModule set layout = MyModule.App_Default;
```

Both rewrite the layout reference **and** every placeholder binding. Pages in
Marketplace modules are skipped and named — they are not yours to edit either.

A page bound to a placeholder the new layout does not declare is **refused**
before anything is written; `map (Old as New)` is how you rebind it:

```sql
alter page MyModule.Split { set Layout = MyModule.Minimal map (HeaderLeft as Main); };
```

### ALTER LAYOUT

Edits the stored document rather than rewriting it, so widgets MDL cannot spell
survive. Same operations as `ALTER PAGE`:

```sql
alter layout MyModule.App_Default {
  insert into layoutContainer.top { snippetcall bar (snippet: MyModule.SNIPPET_ThemeBar) };
  set Content = 'My App' on brandText;
  drop widget oldBanner;
};
```

A region has no name of its own — its slot *is* its identity — so it is addressed
as `<scrollContainerName>.<slot>`, reusing the dotted widget reference. Only
`INSERT INTO` takes a region; `BEFORE`/`AFTER` position a widget among siblings,
so name the widget instead. An empty slot has no stored region document to insert
into: add it with `create or replace layout`.

`ALTER LAYOUT` refuses a Marketplace target, and names the copy-then-repoint
route in the error.

## Syntax

```sql
create [or replace] layout MyModule.App_Default (
  layouttype: 'Responsive',
  class: 'layout-atlas layout-atlas-responsive-topbar'
) {
  scrollcontainer layoutContainer {
    region top (size: 60, sizemode: 'Fixed', class: 'region-topbar') {
      snippetcall topbar (snippet: MyModule.SNIPPET_TopBar)
    }
    region left (size: 232, sizemode: 'Pixels', class: 'region-sidebar') {
      navigationtree navMenu (profile: 'Responsive')
    }
    region center (class: 'region-content') {
      placeholder Main
    }
  }
}
```

A page then names the layout, and its widgets land in the `Main` placeholder:

```sql
create page MyModule.Home (title: 'Home', layout: MyModule.App_Default) {
  dynamictext welcome (content: 'Hello')
}
```

## The four things only a layout has

| Element | MDL | Notes |
|---------|-----|-------|
| Scroll container | `scrollcontainer name { … }` | The layout's root. Its children are **regions**, never widgets |
| Region | `region top \| right \| bottom \| left \| center` | Five **named slots**, not a list. One region per slot; a repeat is refused |
| Placeholder | `placeholder Main` | The hole a page's content goes into. No properties, no body |
| Navigation tree | `navigationtree name (profile: 'Responsive')` | The sidebar menu — vertical. The profile is a navigation profile name |
| Menu bar | `menubar name (profile: 'Responsive')` | The topbar menu — horizontal. Same stored shape as a navigation tree |

Region properties: `size` (integer), `sizemode` (`Fixed` / `Pixels` / `Auto`),
`class`. Unset is Studio Pro's `200` / `Auto`.

## Layout type, and why there is no `native:` flag

| Platform | Values |
|----------|--------|
| Web | `Responsive`, `Phone`, `Tablet`, `ModalPopup` |
| Native | `Default`, `Popup` |

Measured across all 22 layouts Atlas ships. The two sets are **disjoint**, so the
platform is inferred from the type — a `native:` property could only ever
contradict it. A cross-platform value (`Default` on a web layout) is refused, not
silently accepted.

## The layout class is load-bearing

`class:` sets the layout's own CSS class, and Atlas scopes **~24 of its layout
rules** to `.layout-atlas` and its variants. Every Atlas layout with chrome
carries one — `layout-atlas layout-atlas-responsive-topbar`,
`layout-atlas layout-atlas-responsive-default`, and so on; only `PopupLayout`,
which has no chrome, is bare.

Leave it off and the layout builds cleanly, passes `mx check`, and renders with
**no topbar bar and no sidebar rail** — a difference only a browser shows. Use
the Atlas class that matches the shape you are building:

| Shape | Class |
|-------|-------|
| Topbar navigation | `layout-atlas layout-atlas-responsive-topbar` |
| Sidebar navigation | `layout-atlas layout-atlas-responsive-default` |
| Popup | *(none)* |

`layouttype`, `class` and `style` are the **only** header properties. Anything
else is an error rather than an ignored key, which is the point of the next
section.

## Gotchas

- **A placeholder's name is API.** A page binds to it as
  `Module.Layout.<Name>`, stored as a qualified name. Renaming one unbinds every
  page that used it — the page still builds, and its content vanishes.
- **Exactly one placeholder must be named `Main` — this is a rule, not a
  convention.** `Forms$Layout` has no property saying which placeholder is the
  main one, but mxbuild validates the NAME. Measured on 11.12.1 against a layout
  **no page uses**, so none of it depends on a page binding:

  | Declares | mxbuild |
  |---|---|
  | `Main` | 0 errors |
  | `Main` + `Content` | 0 errors — extra names are fine |
  | `Content` only | **CE0848** "No placeholder with the name 'Main' found. There should be exactly one." |
  | `Main` + `Main` | **CE0849** + CE0495 |
  | `Main` + `Side` + `Side` | **CE0495** "Duplicate name 'Side'." — uniqueness is general |

  `mxcli check` reports these as **MDL081** (the Main rule) and **MDL082**
  (duplicate names). Earlier versions said this was a convention and checked
  only that *some* placeholder existed, so a layout naming it anything else
  passed `check` and `exec` and failed the build (mendixlabs/mxcli#1063).
- **There is no `mainplaceholder:` property, on purpose.** `modelsdk/gen`
  declares `MainPlaceholderName` on `Layout` so the setter compiles, and mxbuild
  accepts the result — measured 0 errors. But `generated/metamodel` does not
  declare it and no Studio Pro layout carries it, and Studio Pro resolves every
  stored property against the type's list. Writing it gives you a layout that
  builds and cannot be opened.
- **A placeholder is declared with NO body.** `placeholder Main { … }` is the
  page-side spelling — in a page it *fills* a layout's slot; in a layout it
  declares nothing and is dropped, leaving the layout with no placeholder at all.
  Reaching for it here is the natural mistake, since every other layout element
  takes a body. Reported as **MDL083**.
- **A layout that declares no placeholder is refused at write time** as well as
  at check time — no page could use it.
- **The sidebar toggle, the menu bar's logo and Atlas's `Forms$Header` are not
  authorable.** A topbar layout that needs a collapsible sidebar therefore has to
  keep Atlas's, or do without the toggle — which is why `mxcli new` scaffolds a
  topbar-navigation layout with no sidebar rather than an always-open one.
- **Authoring is modelsdk-only.** The legacy writer cannot produce the `Content`
  wrapper the widget tree hangs off; it refuses rather than writing a layout with
  no tree.
- **`create or replace layout` rewrites the whole document; `alter layout` does
  not.** Prefer ALTER for an edit to a layout you did not author from MDL — a
  rewrite is only ever as complete as the describe it came from.

## Validate

```bash
mxcli check layout.mdl                       # syntax
mxcli check layout.mdl -p app.mpr --references
mxcli exec layout.mdl -p app.mpr
mxcli -p app.mpr -c "describe layout MyModule.App_Default"   # round-trip
```

A layout is a rendering artifact, so a clean `mx check` proves little on its own.
To see it, boot the app: `mxcli run --local -p app.mpr --screenshot` and look at
the regions.

## What `mxcli new` scaffolds

A new project gets `<YourModule>.App_Default` — this layout, in a module it
owns — and its pages moved onto it, so the documented practice is the default
rather than something to discover. `mxcli new --layout none` keeps Atlas's.

It is **not** a copy of Atlas_TopBar, and could not be: that layout carries a
`Forms$MenuBar` (now authorable), a `Forms$SidebarToggleButton` and a pluggable
image, so a describe → exec copy renders with no navigation and no logo. The
scaffold reproduces the *result* instead — same layout class, same region
classes, navigation in the topbar, `Main` for page content — and omits the
toggle button and the stock logo.
