# ALTER NAVIGATION

## Synopsis

```sql
CREATE OR REPLACE NAVIGATION profile
    HOME { PAGE module.PageName | MICROFLOW module.Flow | NANOFLOW module.Flow }
    [ HOME { PAGE | MICROFLOW | NANOFLOW } module.Name FOR UserRole ]
    [ LOGIN PAGE module.PageName ]
    [ NOT FOUND PAGE module.PageName ]
    [ ON SYNC ERROR { THROW | CONTINUE } ]
    [ PROGRESSIVE WEB APP [ ( Precaching: bool, InstallPrompt: bool ) ] | PROGRESSIVE WEB APP OFF ]
    [ SYNC (
        sync_rules
    ) ]
    [ {
        menu_items
    } ]
```

## Description

Creates or replaces a navigation profile. Each profile defines the home page, optional role-specific home pages, optional login page, optional 404 page, and a hierarchical menu structure.

The statement fully replaces the existing profile configuration. To modify only part of a profile's navigation, use DESCRIBE NAVIGATION to export the current configuration, edit the MDL, and re-execute.

### Profile Types

Mendix supports the following navigation profile types:

| Profile | Description |
|---------|-------------|
| `Responsive` | Web browser (desktop and mobile responsive) |
| `Tablet` | Tablet-optimized web |
| `Phone` | Phone-optimized web |
| `ResponsiveOffline` | Responsive web, offline-capable (PWA) |
| `TabletOffline` | Tablet web, offline-capable (PWA) |
| `PhoneOffline` | Phone web, offline-capable (PWA) |
| `NativePhone` | Native mobile application |

The web kinds are a closed set, and the profile is **created** if the project
does not have it yet. The three offline kinds are the online names plus
`Offline`, and each one needs a [`SYNC` block](#offline-synchronization) to
download anything.

### Menu Items

Menu items form a hierarchy. They are the profile's children, in `{ ... }` after its clauses, like a page's widgets. Top-level items appear in the main navigation bar. Nested submenus are created with the `MENU 'label' { ... }` syntax.

Each `MENU ITEM` specifies a label and, in its property list, an action and an icon: `MENU ITEM 'Home' ( OnClick: SHOW PAGE M.Home, Icon: Atlas_Core.Atlas.home )`. A child ends in `)` or `}`, so no separator is written between items.

The old spelling — `MENU ( MENU ITEM 'Home' PAGE M.Home; … )` — still parses and warns (MDL-DEPR121, MDL-DEPR122); `mxcli fmt --upgrade` rewrites it.

## Parameters

`profile`
:   The navigation profile type — see [Profile Types](#profile-types). One of
    `Responsive`, `Tablet`, `Phone`, `ResponsiveOffline`, `TabletOffline`,
    `PhoneOffline` or `NativePhone`.

`HOME PAGE module.PageName`
:   The default home page for the profile. Required. The page must already exist.

`HOME MICROFLOW module.Flow` / `HOME NANOFLOW module.Flow`
:   A flow run as the home instead of a page: a microflow on a web profile, a
    nanoflow on a native one. Each is refused on the other kind of profile
    (`HOME MICROFLOW` on a native profile, which DESCRIBE used to print for its
    nanoflow home, still runs with a warning).

`HOME PAGE module.PageName FOR UserRole`
:   Optional role-specific home page. Users with this role see a different home page than the default. Multiple role-specific home pages can be specified.

    The role is a **user role**, written as a bare name (`FOR Administrator`). User roles are project-level and have no module part. A module-qualified name here — a module role, which often shares the name — produces a project Mendix cannot load: `StorageLoadException: … is not a valid UserRoleIdentifier`, raised before checking runs. `mxcli check --references` refuses it.

`LOGIN PAGE module.PageName`
:   Optional custom login page. If omitted, the default system login page is used.

`NOT FOUND PAGE module.PageName`
:   Optional custom 404 page shown when a requested page is not found.

`{ menu_items }`
:   Optional menu structure. Contains `MENU ITEM` and nested `MENU` entries. An empty `{ }` clears the menu; omitting the block leaves it alone.

`MENU ITEM 'label' ( OnClick: action, Icon: icon )`
:   A leaf menu item. `OnClick` is `SHOW PAGE module.PageName`, `CALL MICROFLOW module.Microflow`, `CALL NANOFLOW module.Nanoflow`, `OPEN LINK 'url'`, `CREATE OBJECT module.Entity [THEN SHOW PAGE module.PageName]`, `SIGN OUT` or `NOTHING`, with a button action's `WITH ( … )` settings; `Icon` is `Module.Collection.icon`, `GLYPH n` or `IMAGE Module.Images.name`. Both are optional. DESCRIBE prints every stored action this way, and flags what it cannot spell (a page title override, a link type other than Web), which a rewrite keeps while the item is unchanged.

`MENU 'label' [( Icon: icon )] { ... }`
:   A submenu containing nested menu items and/or further submenus.

### Offline Synchronization

`SYNC ( ... )` configures which entities an offline profile downloads. **An
offline profile downloads nothing without it** — the app builds, routes and
installs as a PWA, and shows an empty screen.

```sql
SYNC (
    SYNC Sales.Setting ONLINE;
    SYNC Sales.Order ALL;
    SYNC Sales.Trip WHERE [Distance > 0];
    SYNC Sales.Audit NEVER;
    SYNC Sales.Lookup NONE;
    SYNC Sales.Draft NONE PRESERVE DATA;
)
```

| Mode | Meaning |
|------|---------|
| `ONLINE` | Fetched from the server; never held on the device |
| `ALL` | Every object downloaded |
| `WHERE [ xpath ]` | Only the objects the XPath selects |
| `NEVER` | Not synchronized |
| `NONE` | Not downloaded; anything already on the device is dropped |
| `NONE PRESERVE DATA` | Not downloaded; what is on the device stays |

These are the values Mendix stores, **not** the captions Studio Pro shows in
its *Customize offline synchronization* dialog: its "All Objects" is `ALL` and
its "By XPath" is `WHERE`. A caption is refused rather than written.

`WHERE` implies the constrained mode rather than naming it, so a constraint
without a mode and a mode without a constraint are both unspellable. The XPath
goes in **brackets** and is taken verbatim — nothing inside is escaped. A
quoted `WHERE 'xpath'` still parses, but every quote inside it must be doubled.

The block replaces the stored list, the way `MENU` replaces the menu. Omitting
it leaves the stored configuration alone.

`ON SYNC ERROR THROW | CONTINUE` is Studio Pro's *"Throw error when server
rejects objects during synchronization"*, and defaults to `THROW`. It reuses the
phrase MDL already has for failure handling — a microflow's `ON ERROR CONTINUE`
— rather than introducing a keyword of its own. Omitting the clause leaves the
stored value alone, and `DESCRIBE NAVIGATION` emits it only when it is not the
default.

`PROGRESSIVE WEB APP` is Studio Pro's *"Progressive web app"* settings. A profile
stores none until they are set, and an **offline** profile without them gets no
service worker: the built `index.js` has `"registerServiceWorker": false`, and no
page opens without a network, while `check`, `exec` and `mx check` all pass
(mendixlabs/mxcli#1377). `Precaching` pre-loads the app's pages and resources
(default `false`); `InstallPrompt` allows the *Add to home screen* prompt (default
`true`); a key the clause leaves out takes its default, so a bare
`PROGRESSIVE WEB APP` writes the defaults, `PROGRESSIVE WEB APP OFF`
stores none, and omitting the clause leaves the stored settings alone.
`DESCRIBE NAVIGATION` emits it when settings are stored, naming only the keys that
differ from the defaults, and `CREATE NAVIGATION` warns about an offline profile
left without them.

```sql
CREATE OR MODIFY NAVIGATION PhoneOffline
    HOME PAGE Field.WorkOrder_List
    PROGRESSIVE WEB APP ( Precaching: true )
    SYNC ( SYNC Field.WorkOrder ALL; );
```

An entity's *compatibility mode* flag has no MDL syntax. It is read, preserved
across a rewrite, and reported by `DESCRIBE NAVIGATION` — never silently
dropped.

## Examples

Minimal navigation with just a home page:

```sql
CREATE OR MODIFY NAVIGATION Responsive
    HOME PAGE MyModule.Home_Web;
```

Full navigation with role-specific homes and menus:

```sql
CREATE OR MODIFY NAVIGATION Responsive
    HOME PAGE MyModule.Home_Web
    HOME PAGE MyModule.AdminHome FOR Administrator
    LOGIN PAGE Administration.Login
    NOT FOUND PAGE MyModule.Custom404
    {
        MENU ITEM 'Home' ( OnClick: SHOW PAGE MyModule.Home_Web )
        MENU 'Admin' {
            MENU ITEM 'Users' ( OnClick: SHOW PAGE Administration.Account_Overview )
            MENU ITEM 'Settings' ( OnClick: SHOW PAGE MyModule.Settings )
        }
        MENU ITEM 'About' ( OnClick: SHOW PAGE MyModule.About )
    };
```

Native mobile navigation. A native profile's home is a page or a nanoflow
(`HOME NANOFLOW`); mxcli writes its home pages and `SYNC` block only, and refuses a
`{ }` menu block (the bottom bar), `LOGIN PAGE`, `NOT FOUND PAGE`, `ON SYNC ERROR` and
`PROGRESSIVE WEB APP` on one — set those in Studio Pro. DESCRIBE lists the bottom bar as comments:

```sql
CREATE OR MODIFY NAVIGATION NativePhone
    HOME NANOFLOW Mobile.NAV_Home
    HOME PAGE Mobile.Dashboard FOR User;
```

## See Also

[LIST NAVIGATION](list-navigation.md)
