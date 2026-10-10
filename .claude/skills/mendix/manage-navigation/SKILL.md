---
name: manage-navigation
description: "Inspect and change navigation profiles in MDL — home pages, menus, login and not-found pages, and role-based routing. Use when asked to change where the app opens, restructure the menu, or route roles to different home pages."
---

# Navigation Management Skill

This skill covers inspecting and modifying Mendix navigation profiles via MDL: home pages, menu items, login pages, role-based routing, and navigation catalog queries.

## When to Use This Skill

Use when the user asks to:
- View or change navigation home pages
- View or modify the navigation menu structure
- Set login or not-found pages
- Configure role-based home page routing
- Discover which pages are navigation entry points
- Set up navigation for a new project

## Navigation Concepts

- **Navigation Profiles** — Every Mendix project has navigation profiles: Responsive, Phone, Tablet, and optionally Native. Each profile has its own home page, menu, and login page. The web kinds are a **closed set** and `create or replace navigation` will create one that does not exist yet:

  | Online | Offline twin |
  |---|---|
  | `Responsive` | `ResponsiveOffline` |
  | `Phone` | `PhoneOffline` |
  | `Tablet` | `TabletOffline` |

  An invented name (`Mobile`, `Desktop`) is an error, not a new profile: Mendix routes on **User-Agent** to its own kinds, so a profile the platform does not define can never be reached. Native profiles are a different document type and are not creatable from MDL.
- **Offline Profiles** — An offline profile makes the app work without a connection (a PWA with a local database that syncs). It is not a different document — same properties as its online twin — but it **constrains every page it can reach**, see below.
- **Home Page** — The default page shown after login. Can be a PAGE or MICROFLOW.
- **Role-Based Home Pages** — Override the default home page per user role (e.g., admins see a dashboard, users see a task list).
- **Menu Items** — Hierarchical menu tree. Each item has a caption and optionally targets a PAGE or MICROFLOW. Sub-menus nest with `menu 'caption' { ... }`.
- **Menu Documents** — A *separate* document type (`Menus$MenuDocument`) holding a reusable menu that a menu widget points at, e.g. Atlas_Core's `Phone_Menu`. Not the same thing as a profile's menu, though both are built from the same items, so the item syntax is identical. Managed with `create/describe/drop menu` — see below.
- **Login Page** — Custom login page (optional; Mendix provides a default).
- **Not-Found Page** — Custom 404 page (optional).

## Show Commands (Read-Only)

```sql
-- Summary of all navigation profiles (home pages, menu counts)
list navigation;

-- Full MDL description of a profile (round-trippable output)
describe navigation Responsive;
describe navigation;              -- all profiles

-- Menu tree for a specific profile
list navigation menu Responsive;
list navigation menu;             -- all profiles

-- Home page assignments across all profiles and roles
list navigation homes;
```

## CREATE OR MODIFY NAVIGATION (Full Replacement)

This command fully replaces a navigation profile's configuration. All clauses are optional — omitted clauses clear that section. The output from `describe navigation` can be pasted back directly.

### Basic: Set Home and Login Page

```sql
mdl 1;
create or modify navigation Responsive
  home page MyModule.Home_Web
  login page Administration.Login;
```

### Role-Based Home Pages

Add `for <UserRole>` to override the home page for specific user roles. The
role is a **bare name** — user roles are project-level and have no module part:

```sql
mdl 1;
create or modify navigation Responsive
  home page MyModule.Home_Web
  home page MyModule.AdminDashboard for Administrator
  home page MyModule.CustomerPortal for Customer
  login page Administration.Login;
```

**Never write a module-qualified role here.** `for Administration.Administrator`
is a *module role*, and Mendix cannot load a project containing one in this
position:

```
StorageLoadException: Role based home page has an invalid value '' for property
UserRole. The text 'Administration.Administrator' is not a valid UserRoleIdentifier.
```

That is worse than a build error — it happens before checking runs, so there is
no error code and no line number, and `mx check` exits non-zero *without* the
"The app contains: N errors" line. The two role kinds are easy to confuse
because they share names: a blank app has a user role `Administrator` and module
roles called `Administrator` in three modules. List the real ones with
`list user roles`; `mxcli check --references` refuses the wrong form.

### Full Menu Tree

The menu items are the profile's children, in `{ ... }` after its clauses, like a page's widgets — no `;` between them. The block replaces the entire menu. Use `menu item 'Caption' ( OnClick: … )` for leaf items and `menu 'caption' { ... }` for sub-menus. `OnClick:` takes the action expression a button's `Action:` takes, limited to what a menu item can do: `show page M.P`, `call microflow M.F`, `call nanoflow M.N`, `open link 'https://…'`, `create object M.E then show page M.E_New`, `sign out` or `nothing` — each with the button's `with ( … )` settings (`ProgressBar`, `ProgressMessage`, `Confirmation`/`ProceedCaption`/`CancelCaption`, `Asynchronous` and `FormValidations` for a microflow, `DisabledDuringExecution`). Save, delete, close page and complete task act on a page's object and are refused on a menu item:

```sql
mdl 1;
create or modify navigation Responsive
  home page MyModule.Home_Web
  login page Administration.Login
  {
    menu item 'Home' ( OnClick: show page MyModule.Home_Web )
    menu 'Orders' {
      menu item 'All Orders' ( OnClick: show page Orders.Order_Overview )
      menu item 'New Order' ( OnClick: show page Orders.Order_New )
    }
    menu 'Admin' {
      menu item 'Users' ( OnClick: show page Administration.Account_Overview )
      menu item 'Run Report' ( OnClick: call microflow Reports.ACT_GenerateReport with (ProgressBar: Blocking, ProgressMessage: 'Generating…') )
    }
    menu item 'Reports' ( OnClick: call nanoflow Reports.NAV_ShowReports )
    menu item 'Docs' ( OnClick: open link 'https://docs.mendix.com' )
    menu item 'New order' ( OnClick: create object Orders.Order then show page Orders.Order_New )
  };
```

Describe prints every stored action this way, so describe → exec writes nothing. What
the expression cannot spell — a show page's **page title override**, an open link's
**link type** other than Web — is flagged with a `--` comment; a rewrite keeps it
while the item's action is unchanged (same caption, same action), and loses it if
the item is renamed or its action changed. An action describe cannot print at all
is flagged too, and an item that states no `OnClick` keeps it.

The old spelling — `menu ( menu item 'Home' page M.Home; menu 'Admin' ( … ); )`,
the action and icon as clauses and `;` after each item — still parses and warns
(MDL-DEPR121, MDL-DEPR122); `mxcli fmt --upgrade` rewrites it.

### Menu Icons

**Give every menu item an icon.** It is optional in the grammar and `mxcli check`
warns when it is missing (**MDL077**), because the navigation sidebar collapses
to an icon rail and that is the state most users leave it in: a collapsed item
shows its icon, and one without falls back to the first few characters of its
caption — rarely enough to tell `Orders` from `Order lines`. Nothing else catches
it. The model builds, `mx check` passes, and the menu is simply hard to use.

Both `menu item` and `menu 'caption' { ... }` take an `Icon:` in their property
list, in one of three forms — Mendix stores three different icon **elements**, not
three spellings of one value:

```sql
menu item 'Home'  ( OnClick: show page M.Home,  Icon: Atlas_Core.Atlas.home )   -- icon collection
menu item 'Close' ( OnClick: show page M.Close, Icon: glyph 57377 )             -- numeric glyph code
menu item 'Logo'  ( OnClick: show page M.Logo,  Icon: image M.Images.logo )     -- image collection
menu 'Admin' ( Icon: Atlas_Core.Atlas.user ) { … }                               -- a sub-menu's icon
```

The **bare** form is the icon-collection icon and is what you normally want. Use
`glyph` only to reproduce a legacy icon a project already has — `describe
navigation` emits it for you — and `image` for a picture from an image
collection, which is a different document from an icon collection.

**Do not invent a glyph code.** It is a bare integer that nothing resolves, so a
code the Mendix font does not define passes `mxcli check` AND `mx check` at 0
errors and then breaks `mxbuild --target=deploy` with *"An exception occurred
while exporting layout '<some layout>'"* — a message naming a document that is
not the cause. `mxcli check` now warns (**MDL078**) against the 247 codes the
shipped font defines, but a glyph is still an unchecked number where an icon
collection reference is a resolved model reference. Browse the codes with `list glyphs`
(`list glyphs like 'star'` searches by name, `describe glyph 57350` goes the
other way), or use `Icon: Atlas_Core.Atlas.<name>` and list the names with
`describe icon collection Atlas_Core.Atlas`.

The icon-collection form is a **qualified name** — a model reference, written
like every other reference in MDL, not a string:

```sql
mdl 1;
create or modify navigation Responsive
  home page MyModule.Home_Web
  {
    menu item 'Home' ( OnClick: show page MyModule.Home_Web, Icon: Atlas_Core.Atlas.home )
    menu 'Orders' ( Icon: Atlas_Core.Atlas."shopping-cart" ) {
      menu item 'All Orders' ( OnClick: show page Orders.Order_Overview,
        Icon: Atlas_Core.Atlas."list-bullets" )
    }
  };
```

**Hyphenated names are double-quoted** (`Atlas_Core.Atlas."align-center"`) —
a hyphen does not lex as an identifier, so the segment is quoted exactly as a
keyword-colliding name would be. Plain names need no quotes, including ones that
happen to be MDL keywords (`home`, `user`, `add`). Atlas_Core ships three
collections: `Atlas` (outline), `Atlas_Filled`, and `Atlas_Styling`. List what
is actually available in your project rather than guessing a name:

```sql
list icon collections;
describe icon collection Atlas_Core.Atlas;
```

**Only the icon-collection form is writable.** Studio Pro can also attach a
*glyph* icon (a numeric character code) or an *image* icon (pointing into an
image collection). Those are different elements with different fields, so MDL
does not write them — and `describe navigation` reports them as a comment rather
than emitting an `Icon:` that would silently convert one into the other on
replay:

```
menu item 'Close' ( OnClick: show page MyModule.Close )
-- icon System.Images.Close (Forms$ImageIcon) is not reproducible by CREATE NAVIGATION; set it in Studio Pro
```

### Offline Synchronization

An offline profile downloads **nothing** until its entities are given a sync
mode. Without a `SYNC` block the app builds, routes and installs as a PWA — and
shows an empty screen. That is the single most common way an offline profile
looks broken while every check passes.

```sql
mdl 1;
create or modify navigation PhoneOffline
  home page MyModule.Mobile_Dashboard
  sync (
    sync MyModule.Setting online;
    sync MyModule.Vehicle all;
    sync MyModule.Trip where [Distance > 0];
    sync MyModule.AuditEntry never;
    sync MyModule.Lookup none;
    sync MyModule.Draft none preserve data;
  );
```

| MDL | Meaning |
|---|---|
| `online` | fetched from the server, never held on the device |
| `all` | every object downloaded |
| `where [<xpath>]` | only the objects the XPath selects |
| `never` | not synchronized |
| `none` | not downloaded; anything already on the device is dropped |
| `none preserve data` | not downloaded; what is on the device stays |

**The words are not Studio Pro's captions.** Its dialog shows "All Objects" and
"By XPath"; neither is a value Mendix stores. `all` and `where` are. Copying a
caption out of the UI gives a parse error rather than a broken document, which
is deliberate.

**`where` implies the constrained mode** rather than naming it, so a constraint
without a mode and a mode without a constraint are both unspellable.

**Use the bracket form.** It takes the XPath verbatim — nothing inside is
escaped, so quoted literals stay readable:

```sql
sync MyModule.Team where [contains(Name, 'abc')];
```

A quoted `where '<xpath>'` still parses, but every quote inside it must be
doubled — and a stored constraint already carries Mendix's own escaping, so the
two compose into runs of six quotes. `describe navigation` emits the bracket
form. This is the general problem tracked as `mendixlabs/mxcli#750`.

**The block replaces the stored list**, the way the `{ ... }` menu block replaces the menu.
Omitting it leaves the stored configuration alone.

**Ask the catalog which entities sync, rather than reading the profile.**

```sql
select EntityQualifiedName, SyncMode, XPathConstraint
  from CATALOG.OFFLINE_ENTITY_CONFIGS where ProfileName = 'PhoneOffline';
```

And before changing an entity, ask which profiles download it — an offline
change reaches every device that already synced:

```
list references to MyModule.Order;
```

The `sync` row names the profile. Every mode produces one, **including the
modes that download nothing**: a profile with `sync X never` still names `X`,
so renaming or dropping it leaves the configuration dangling.

**Errors when the server rejects an object.** Studio Pro's *"Throw error when
server rejects objects during synchronization"* checkbox:

```sql
mdl 1;
create or modify navigation PhoneOffline
  home page MyModule.Mobile_Dashboard
  on sync error continue;      -- default is `throw`
```

It uses the phrase MDL already has for failure handling — a microflow's
`on error continue` — rather than a keyword of its own. Omitting the clause
leaves the stored value alone; `describe navigation` emits it only when it is
not the default, so existing scripts stay quiet.

**Compatibility mode has no syntax.** mxcli reads it, preserves it across a
rewrite, and `describe navigation` flags any entity that has it on — it is never
silently dropped.

### Clear the Menu

An empty `{ }` menu block removes all menu items:

```sql
mdl 1;
create or modify navigation Responsive
  home page MyModule.Home_Web
  {};
```

### Not-Found Page

```sql
mdl 1;
create or modify navigation Responsive
  home page MyModule.Home_Web
  not found page MyModule.Custom404;
```

### Microflow as Home Page

Use `home microflow` instead of `home page` to run a microflow on login:

```sql
mdl 1;
create or modify navigation Responsive
  home microflow MyModule.ACT_ShowHome;
```

A **native** profile's flow home is a nanoflow: `home nanoflow MyModule.NAV_Home`
(`home nanoflow` on a web profile is refused). On a native profile mxcli writes the
home pages and the `sync ( … )` block only — a `{ }` menu block (the bottom bar),
`login page`, `not found page` and `on sync error` are refused, and describe lists
the bottom bar as comments.

## Round-Trip Workflow

The DESCRIBE output is directly executable. Use this pattern to inspect, modify, and re-apply:

```sql
mdl 1;
-- Step 1: Inspect current state
describe navigation Responsive;

-- Step 2: Copy the output, modify as needed, paste back
create or modify navigation Responsive
  home page MyModule.Home_Web
  login page Administration.Login
  {
    menu item 'Home' ( OnClick: show page MyModule.Home_Web )
    menu item 'New Feature' ( OnClick: show page MyModule.NewFeature )
  };

-- Step 3: Verify
describe navigation Responsive;
```

## Catalog Queries

After `refresh catalog full`, navigation references appear in the `REFS` table:

```sql
refresh catalog full;

-- Find all pages that are navigation entry points
select SourceName, TargetName, RefKind
from CATALOG.REFS
where RefKind in ('home_page', 'menu_item', 'login_page');

-- What references point to a specific page?
list references to MyModule.Home_Web;

-- Impact analysis: what breaks if I change this page?
list impact of MyModule.Home_Web;

-- Full context for a page (includes navigation references)
describe context of MyModule.Home_Web;
```

## Common Patterns

### New Project Setup

Set up navigation for a freshly created project:

```sql
mdl 1;
-- Create home page
create page MyModule.Home_Web
(
  title: 'Home',
  layout: Atlas_Core.Atlas_Default
)
{
  container ctnMain {
    dynamictext txtWelcome (content: 'Welcome!')
  }
};

-- Configure navigation
create or modify navigation Responsive
  home page MyModule.Home_Web
  {
    menu item 'Home' ( OnClick: show page MyModule.Home_Web )
  };
```

### Adding a New Page to Navigation

After creating a new page, add it to the menu:

```sql
mdl 1;
-- First inspect current menu
describe navigation Responsive;

-- Then re-apply with the new item added (copy existing + add new)
create or modify navigation Responsive
  home page MyModule.Home_Web
  login page Administration.Login
  {
    menu item 'Home' ( OnClick: show page MyModule.Home_Web )
    menu item 'Customers' ( OnClick: show page MyModule.Customer_Overview )  -- new
    menu 'Admin' {
      menu item 'Users' ( OnClick: show page Administration.Account_Overview )
    }
  };
```

## Menu Documents (standalone, reusable)

A profile menu lives *inside* a navigation profile and is edited with
`create or modify navigation`. A **menu document** is its own document, and a
menu widget on a page points at it. Atlas_Core ships `Phone_Menu` and
`Tablet_Menu`.

Tell them apart by which command reads them:

```sql
list navigation menu;                    -- the menu inside each profile
describe menu Atlas_Core.Phone_Menu;     -- a standalone menu document
```

Menu documents use the same item syntax as the profile's `{ ... }` menu block:

```sql
mdl 1;
create or modify menu MyModule.Main_Menu {
  menu item 'Home' ( OnClick: show page MyModule.Home_Web, Icon: Atlas_Core.Atlas.home )
  menu item 'Run' ( OnClick: call microflow MyModule.DoThing )
  menu 'Admin' {
    menu item 'Accounts' ( OnClick: show page Administration.Account_Overview )
  }
  menu item 'Plain'
};

drop menu MyModule.Main_Menu;
```

`describe menu` emits a re-executable `create or modify` statement, so
describe → edit → exec is the normal editing loop for a menu your MDL scripts own.
Menus and navigation have no patch statement, so for one maintained in Studio Pro
keep the edit to the items you mean to change, then `describe` it again after
`exec` and compare with the original; a difference you did not make is a loss (see
[choose-edit-mode](../choose-edit-mode/SKILL.md)).

**`or modify` replaces the whole item list.** An omitted item is a removed item,
exactly as with `create or modify navigation`. The document's identity and
export level are preserved, so menu widgets pointing at it keep working.

### Gotchas

- **A menu item cannot open a page that takes a required parameter.** There is
  nowhere to supply the argument, and Mendix reports **CE1571** ("No argument has
  been selected for parameter …") against `Menu item`. Point the item at a
  parameterless page, or call a microflow that opens the page.
- **Only icon-collection icons round-trip.** A glyph icon (numeric code) or an
  image icon cannot be written by MDL; `describe` flags those on their own
  comment line rather than dropping them silently, so re-running the output
  loses that icon visibly.

## Offline Profiles

An offline profile is created the same way as any other:

```sql
mdl 1;
create or modify navigation TabletOffline
  home page Maintenance.Request_Overview
  {
    menu item 'Requests' ( OnClick: show page Maintenance.Request_Overview )
  };
```

Two things about it are worth knowing before you do.

**It breaks pages the statement never mentions.** Mendix restricts every page an
offline profile can reach: an attribute may be bound across **at most one
association hop**. A longer path is **CE6206** — *"Attribute paths with multiple
steps cannot be used on pages that are accessible through an offline-based
navigation."*

```
Request_Asset/AssetName                one hop    fine
Request_Asset/Asset_Site/SiteName      two hops   CE6206
```

Those pages were valid before. Adding the profile is what invalidated them, and
the build fails somewhere with no obvious connection to the statement you ran.
mxcli reports them when it creates the profile:

```
Navigation profile 'TabletOffline' created.

Warning: TabletOffline is an offline profile, and 3 documents in this project bind
an attribute across more than one association.
Mendix rejects a multi-step attribute path with CE6206 on any page reachable from
an offline profile — one hop is allowed, two are not.
  page Maintenance.Request_Overview: Maintenance.Site.SiteName (2 steps)
  ...
```

It is a **warning**, not a refusal: whether a page is actually *reachable* from
the profile takes the whole page graph, which mxcli does not walk. The fix is
either to keep the page off the offline profile, or to bring the value one hop
closer — add an attribute to the intermediate entity and keep it in step.

**It needs Progressive web app settings, or nothing works offline.** A profile
stores none until "Progressive web app" is ticked, and then Mendix registers no
service worker for it: the built `index.js` carries `"registerServiceWorker": false`
and the first page opened without a network fails to load, while `check`, `exec`
and `mx check` all pass (mendixlabs/mxcli#1377). Set them in the statement:

```sql
mdl 1;
create or modify navigation PhoneOffline
  home page Field.WorkOrder_List
  progressive web app ( Precaching: true )
  sync ( sync Field.WorkOrder all; );
```

`Precaching` pre-loads the app's pages and resources (default `false`),
`InstallPrompt` allows "Add to home screen" (default `true`). A key the clause
leaves out takes its default, so a bare `progressive web app` writes both, `progressive web app off` stores none
again, and leaving the clause out keeps what is stored. Exec warns about an
offline profile left without them.

**The synchronization config is derived, not authored.** `offlineEntityConfigs`
starts empty and Studio Pro fills it from the entities the reachable pages use;
only rows that differ from the defaults (`syncMode: Online`, no constraint) are
stored. MDL does not author per-entity sync modes — set those in Studio Pro.

## Checklist

- [ ] Profile name is one of Mendix's web kinds (`Responsive`/`Phone`/`Tablet`, or their `*Offline` twins) — an invented name is an error, not a new profile
- [ ] For an **offline** profile, no page it can reach binds an attribute across more than one association (CE6206)
- [ ] For an **offline** profile, `progressive web app ( Precaching: true )` is set — without it no page opens offline
- [ ] All PAGE/MICROFLOW targets are fully qualified (`Module.Name`)
- [ ] Role references in `for` clauses are fully qualified (`Module.Role`)
- [ ] Menu items are in `{ }` with no `;` between them; sub-menu items in `menu 'caption' { ... }`
- [ ] A menu item's action is `( OnClick: … )` with show page, call microflow, call nanoflow, open link, create object, sign out or nothing — not save / delete / close page
- [ ] `Icon:` is a qualified name (not a string); hyphenated segments are double-quoted
- [ ] The icon exists — check with `describe icon collection Module.Name`, do not guess
- [ ] Use `describe navigation` to verify changes after applying
- [ ] For a **menu document**, confirm you want `create menu` and not a profile menu — `list navigation menu` vs `describe menu` tells them apart
- [ ] No menu item targets a page with required parameters (CE1571)

## Offline synchronization (`CREATE NAVIGATION … SYNC (…)`)

an offline navigation profile downloads **nothing** until each entity has a sync mode, so a profile mxcli created built, routed and installed as a PWA and showed an **empty app** — with `mxcli check`, `exec` and `mx check` all clean. The six mode words are the members Mendix stores, **not** Studio Pro's captions (its "All Objects" is `ALL`, its "By XPath" is `WHERE`), and a caption is refused rather than written — the CE0463 gallery defect wearing a different hat. `WHERE` takes the XPath in **brackets**, verbatim: the quoted form doubles every quote, and a stored constraint already carries Mendix's own escaping, so the two compose into runs of six (mendixlabs/mxcli#750, and `PROPOSAL_first_class_expressions.md`). The write is an **overlay keyed by entity**, so `CompatibilityMode` — stored, unauthorable — survives a rewrite; every reference config carries `false`, so only a synthetic `true` case distinguishes a correct writer from one that always emits `false`. `DownloadMode`/`ShouldDownload` are deliberately **not** written though gen declares them: zero occurrences in ako/TestApp, and a property Studio Pro fills in on load is one whose emission makes a document Studio Pro cannot open. Creating the *profile* stays modelsdk-only (a fourteen-key document pinned to a Studio Pro reference); the SYNC block works on both engines. `ON SYNC ERROR THROW|CONTINUE` writes `ThrowPartialSyncError`, a property **neither generated source declares** (zero occurrences in gen and in generated/metamodel), so it is read from `element.Base.Raw()` and written as a raw key. The spec field is a **pointer**: the property is a bare bool with no unset value, so a non-pointer would reset it on every rewrite that never mentions the clause. Absent reads as **true**, matching every reference profile and Studio Pro's checked-by-default box. Both halves are in the catalog: `CATALOG.OFFLINE_ENTITY_CONFIGS` holds one row per configured entity (the profile's `OfflineEntityCount` said how many and nothing else), and a configured entity emits a **`sync` edge** into `CATALOG.REFS` so `list references to Mod.Entity` names the profiles that download it. Every mode gets an edge, **including the ones that download nothing** — a profile with `sync X never` still names X, so renaming or dropping it leaves the config dangling, which is exactly what the edge exists to reveal. and `docs/11-proposals/PROPOSAL_offline_sync_configuration.md`

## Menu documents (CREATE OR MODIFY/DESCRIBE/DROP MENU)

standalone `Menus$MenuDocument`, the reusable menu a menu widget points at (Atlas_Core's `Phone_Menu`/`Tablet_Menu`) — **not** the menu inside a navigation profile, though both are built from the same items, so the item syntax is shared with `CREATE NAVIGATION`'s `{ ... }` menu block. DESCRIBE is round-trippable. Written through gen+codec, which is load-bearing: Studio Pro's menu documents carry typed-array marker **3** on the item collection and each item's sub-items (the codec default), while the navigation writers hand-build items with marker **1** — unverified whether that is a latent navigation bug or a real difference, so navigation is left alone. Authoring is modelsdk-only; legacy refuses. A menu item cannot open a page with required parameters (**CE1571**). Its actions and icons (all three kinds) round-trip as a navigation menu's do
