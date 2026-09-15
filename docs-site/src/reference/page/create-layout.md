# CREATE LAYOUT

## Synopsis

```sql
CREATE [OR REPLACE] LAYOUT module.Name
(
    layouttype: 'Responsive',
    class: 'layout-atlas layout-atlas-responsive-topbar'
)
{
    SCROLLCONTAINER name {
        REGION top | right | bottom | left | center ( region_properties ) {
            widgets
        }
    }
}
```

## Description

Creates a layout — the frame every page renders inside: the topbar, the
navigation sidebar, and the hole a page's own content drops into. A page names
one with its `Layout` property, and its widgets land in the layout's `Main`
placeholder.

A layout's body uses the whole page widget vocabulary plus four element types
that only a layout has: `SCROLLCONTAINER`, `REGION`, `PLACEHOLDER` and
`NAVIGATIONTREE` (with `MENUBAR` for the topbar's horizontal menu).

`OR REPLACE` rewrites an existing layout. To change one without rebuilding it,
use [ALTER LAYOUT](alter-layout.md) instead — it edits the stored document and
leaves alone anything it was not asked about, including widgets MDL cannot
express.

### Do not write into a Marketplace module

Mendix's own guidance is *"Do not change the supplied layouts. Either create a
separate module with the custom layouts … or create your own."* A Marketplace
update replaces the module wholesale and every local edit is gone, silently.

`CREATE LAYOUT` refuses a Marketplace target for that reason:

```
Error: layout Atlas_Core.T2: Atlas_Core is a marketplace module — a layout
written there is overwritten by the next module update. Create the layout in a
module of your own (Mendix's own guidance) and point pages at it with
ALTER PAGE … SET LAYOUT.
```

The usual starting point is a copy of an Atlas layout. `DESCRIBE LAYOUT` emits
re-executable MDL, so describe → rename → run *is* the copy operation:

```bash
mxcli -p app.mpr -c "describe layout Atlas_Core.Atlas_Default" > mine.mdl
# change the qualified name to your own module, then:
mxcli exec mine.mdl -p app.mpr
```

Read the describe output before running it. A widget MDL cannot express is
emitted as a comment ending `-- NOT re-executable`, naming exactly what a
re-run would drop (`Atlas_Core.Atlas_SideBar`, for instance, loses both of its
`Forms$SidebarToggleButton` widgets).

## Parameters

`module.Name`
:   The qualified name of the layout (`Module.LayoutName`). Must be a module you
    own.

`layouttype`
:   **Required.** Omitting it is an error, not a default:
    `Error: layout needs a layouttype`.

    | Platform | Values |
    |----------|--------|
    | Web | `Responsive`, `Phone`, `Tablet`, `ModalPopup` |
    | Native | `Default`, `Popup` |

    The two sets are disjoint, so the platform is **inferred** from the type —
    there is no separate `native:` flag to contradict it.

`class`
:   The layout's own CSS class. Not decoration — Atlas scopes around two dozen
    of its layout rules to `.layout-atlas` and its variants, and every Atlas
    layout with chrome carries one. A layout written without it builds cleanly,
    passes `mx check`, and renders with **no topbar bar and no sidebar rail**.

    | Shape | Class |
    |-------|-------|
    | Topbar navigation | `layout-atlas layout-atlas-responsive-topbar` |
    | Sidebar navigation | `layout-atlas layout-atlas-responsive-default` |
    | Popup | *(none — `PopupLayout` is bare)* |

`style`
:   Inline CSS on the layout element.

`layouttype`, `class` and `style` are the **only** header properties. Anything
else is an error rather than an ignored key:

```
Error: layout MyModule.T1: unknown property "bogus" (a layout header takes
layouttype, class and style; which placeholder is "main" is set by naming one
Main, not by a property)
```

### Region properties

| Property | Values | Notes |
|----------|--------|-------|
| `size` | integer | Unset is Studio Pro's `200` |
| `sizemode` | `Fixed`, `Pixels`, `Auto` | Unset is `Auto` |
| `class` | CSS class | e.g. `region-topbar`, `region-content` |

## The four elements only a layout has

| Element | MDL | Notes |
|---------|-----|-------|
| Scroll container | `SCROLLCONTAINER name { … }` | The layout's root. Its children are **regions**, never widgets |
| Region | `REGION top \| right \| bottom \| left \| center` | Five **named slots**, not a list. One region per slot |
| Placeholder | `PLACEHOLDER Main` | The hole a page's content goes into. No properties, no body |
| Navigation tree | `NAVIGATIONTREE name (profile: 'Responsive')` | The sidebar menu — vertical |
| Menu bar | `MENUBAR name (profile: 'Responsive')` | The topbar menu — horizontal |

## Examples

A topbar layout, which is the shape `mxcli new` scaffolds:

```sql
CREATE OR REPLACE LAYOUT MyModule.App_Default
(
    layouttype: 'Responsive',
    class: 'layout-atlas layout-atlas-responsive-topbar'
)
{
    SCROLLCONTAINER layoutContainer {
        REGION top (size: 60, sizemode: 'Fixed', class: 'region-topbar') {
            MENUBAR mainMenu (profile: 'Responsive')
        }
        REGION center (class: 'region-content') {
            PLACEHOLDER Main
        }
    }
};
```

A sidebar layout, with a navigation tree in the left region:

```sql
CREATE LAYOUT MyModule.App_Sidebar
(
    layouttype: 'Responsive',
    class: 'layout-atlas layout-atlas-responsive-default'
)
{
    SCROLLCONTAINER layoutContainer {
        REGION left (size: 232, sizemode: 'Pixels', class: 'region-sidebar') {
            NAVIGATIONTREE navMenu (profile: 'Responsive')
        }
        REGION center (class: 'region-content') {
            PLACEHOLDER Main
        }
    }
};
```

A page then names the layout, and its widgets land in `Main`:

```sql
CREATE PAGE MyModule.Dashboard
(
    Title: 'Dashboard',
    Layout: MyModule.App_Default
)
{
    CONTAINER cntMain {
        DYNAMICTEXT txtWelcome (Content: 'Welcome')
    }
};
```

## Notes

- **A layout must declare at least one placeholder.** Without one a page's
  content has nowhere to go, and the statement is refused:
  `layout "X" declares no placeholder`.
- **Name one placeholder `Main`.** `Forms$Layout` has no property recording
  which placeholder is the main one — the convention is the mechanism, and all
  22 layouts Atlas ships follow it. There is deliberately no
  `mainplaceholder:` property: writing the underlying key produces a layout
  that builds cleanly and that Studio Pro cannot open.
- **A placeholder's name is API.** Pages bind to it as `Module.Layout.Name`.
  Renaming one unbinds every page that used it — those pages still build, and
  their content vanishes.

## See Also

[ALTER LAYOUT](alter-layout.md), [CREATE PAGE](create-page.md),
[ALTER PAGE / ALTER SNIPPET](alter-page.md)
