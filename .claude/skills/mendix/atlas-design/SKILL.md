---
name: atlas-design
description: "Make a Mendix app look designed rather than default-Atlas: layout, spacing, typography, colour and design properties that reach a finished standard. Use when asked to make an app look professional, branded or less bland, when styling pages, or when matching a design mock."
---

# Atlas Design — Make a Mendix App Look Designed, Not Bland

## Reference files

`SKILL.md` covers the thesis, the layer architecture, the workflow and the
gotchas. The inventories are next door:

- [`reference/building-blocks.md`](reference/building-blocks.md) — what Atlas
  ships out of the box (layouts, page templates, building blocks, widgets) and the
  appearance vocabulary: the classes and design properties available on each.
  **Look here before writing custom SCSS** — most of what people hand-roll already
  exists as a class.
- [`reference/dark-mode-and-charts.md`](reference/dark-mode-and-charts.md) — a
  dataviz-grade theme for the Mendix chart widgets, and the optional per-widget
  overrides that make dark mode look deliberate rather than inverted.

## When to Use This Skill

Use this skill when:
- The user asks to make an app "look good / professional / branded / less bland"
- You are about to style a Mendix web app or a group of pages
- You are matching a design mock and want it to reach "designed product" quality
- You are re-branding an existing app to a new identity (palette, type, corners)

This is the **taste + workflow** layer. It sits on top of the styling mechanics
(`theme-styling`), the widget syntax (`create-page`), the composition
primitives (`fragments`), and the design-handoff pipeline
(`migrate-design-prototype`). It does **not** re-teach SCSS compilation or
`Class:`/`DesignProperties:` syntax — those skills own that. It adds **which**
tokens/classes to use, **when**, and the **discover → inspect → use** method
built on the Atlas building blocks every Mendix project already ships.

## Contents

1. [The thesis: be Atlas-first](#the-thesis-be-atlas-first)
2. [The 4-layer architecture](#the-4-layer-architecture)
3. [The workflow: discover → inspect → use](#the-workflow-discover--inspect--use)
4. [Brand re-tune (Layer 1) — where most of the win is](#brand-re-tune-layer-1--where-most-of-the-win-is)
5. [Layer 1 in practice — start from the shipped theme](#layer-1-in-practice--start-from-the-shipped-theme)
6. [Dark mode — Mendix 11 makes this cheap](#dark-mode--mendix-11-makes-this-cheap)
7. [Verify at runtime — this is mandatory](#verify-at-runtime--this-is-mandatory)
8. [Gotchas catalog](#gotchas-catalog)
9. [Validation checklist](#validation-checklist)
10. [Related skills](#related-skills)

Inventories and the two long theming sections live beside this file — see
[Reference files](#reference-files) above.

---

## The thesis: be Atlas-first

Every Mendix project ships **Atlas** — a rich appearance system (`Atlas_Core`
classes + typed design properties) and **39 out-of-the-box building blocks**
(`Atlas_Web_Content`: cards, headers, forms, lists, timelines, wizards, alerts).
The single biggest mistake is hand-rolling `.panel` / `.trip-card` / `.stat`
SCSS that **reinvents what Atlas already gives you for free**.

Live testing proved the point: a page of **pure Atlas classes, zero custom CSS**
renders real cards, brand-coloured backgrounds and buttons, and flex layouts —
and those Atlas utilities **inherit your retuned brand tokens automatically**
(`background-primary` resolves to *your* `--brand-primary`).

**Reach *down* the stack first.** Need a card? `class:'card'` (or `'Card style': on`)
before writing a `.panel` rule. Brand blue on a button? Retune `--brand-primary`
before overriding `.btn-primary`. Custom CSS is the **last** resort — for identity
only (a mono metric type, a timeline spine, a bespoke elevation curve).

---

## The 4-layer architecture

Style from the bottom up. Each layer only does what the layer below can't.

```
Layer 3  VERIFY      run --local --watch  +  Playwright screenshot   (mx check is NOT enough)
Layer 2  IDENTITY    theme/web/_<name>.scss, imported from theme/web/main.scss — recipe
                     classes (mono type, status pills, timeline spine) — ONLY what Atlas can't do
Layer 1  BRAND       theme/web/custom-variables.scss — retune Atlas tokens (--brand-primary,
                     backgrounds, semantic colors, radius) so Atlas components inherit the palette
Layer 0  ATLAS       Atlas classes / design properties / building blocks — structure & base look
```

- **Layer 0 — Atlas.** Compose with the Atlas vocabulary (the class cheat-sheet and
  the building-block inventory below).
- **Layer 1 — Brand.** Retune Atlas tokens in `theme/web/custom-variables.scss` so
  the whole framework (buttons, backgrounds, form inputs, pluggable widgets like
  Switch/Slider/ProgressBar) picks up your palette. Start from the shipped theme
  rather than a blank file — see below.
- **Layer 2 — Identity.** Only the handful of shapes Atlas genuinely can't express.
  Put them in a partial imported from **`theme/web/main.scss`**, which compiles
  *last* — after Atlas Core and after every module theme source — so your rules win
  without `!important`. Use `themesource/<mod>/web/main.scss` only when the styling
  belongs to that module: a theme source folder whose name does not match a real
  module is **silently not compiled**. See `theme-styling`.
- **Layer 3 — Verify.** Non-negotiable. `mx check` misses client-side crashes; you
  must screenshot a *running* build.

**Start from the shipped default, don't start from nothing.** `mxcli new` applies
the `signal` theme, and `mxcli theme apply -p app.mpr` adds one (`signal`,
`ledger` or `console`) to an existing project. Each carries a full palette in
both light and dark, vendored fonts, the focus ring, the density scale and the
`num` / `pill` / `stat` recipe classes. `mxcli theme show <name>` lists exactly
which files it writes, and the `--mxt-*` vocabulary a palette is made of.

**Two ways to re-brand, and picking the wrong one costs you the theme.** The
generated blocks are digest-fenced: an edit inside one is refused on the next
`apply` rather than discarded. That protects your work, but it also means the
project has taken the theme out of mxcli's hands.

- **Changing one or two values** — a brand colour, a radius: edit them in the
  palette block and accept that `theme apply` will now report the file as
  modified. Fine for a tweak.
- **A real brand** — your palette, your type, your density: `mxcli theme create`.
  It scaffolds a theme the project owns, which `theme list -p` shows, `theme
  apply <name>` installs and `theme remove` takes out — no fence to fight.

```bash
mxcli theme create acme -p app.mpr --from design/tokens.css
mxcli theme apply acme -p app.mpr
```

A Layer-1 token retune **cascades down** into Atlas components and pluggable
widgets for free — that is the headline payoff. A full re-brand (new palette, type,
corners) is **theme-only**: retune `custom-variables.scss` + `main.scss`, zero
page/MDL edits, and it hot-applies under `--watch`.

---

## The workflow: discover → inspect → use

Building blocks are the Mendix-native recipe library. mxcli can **read and
instantiate** them, so the workflow is:

**1. Discover what your project ships.**
```bash
mxcli -p app.mpr -c "show building blocks"
mxcli -p app.mpr -c "show building blocks in Atlas_Web_Content"
mxcli -p app.mpr -c "select QualifiedName, Category from CATALOG.building_blocks"
```

**2. Inspect the block you want to reproduce.** `describe` prints its real widget
tree — the exact classes and typed design properties Mendix itself uses:
```bash
mxcli -p app.mpr -c "describe building block Atlas_Web_Content.Card"
```
```
{
  container container2 (DesignProperties: ['Card style': on]) {
    dynamictext text22 (Content: 'Card title', RenderMode: H4, Class: 'card-title',
      DesignProperties: ['Spacing': ['margin-bottom': 'L']])
  }
}
```
Note the **two styling channels** Atlas uses side by side: the `Class:` vocabulary
(`card-title`) *and* typed `DesignProperties:` (`'Card style': on`, `Spacing`).

**3. Use it — one line.** `use building block` deep-copies the block's widget tree
onto your page, exactly like dragging it in from the Studio Pro toolbox. Add
`as <prefix>` to rename the copied widgets (so you can drop the same block in twice):

```mdl
use building block Atlas_Web_Content.Card as cust_
```

That expands to the exact tree `DESCRIBE` showed — here `cust_container2` +
`cust_text22`, carrying the `card-title` class and the `Card style` design property.
It's a page-body element: put it inside a `create page` / `alter page` container,
anywhere a widget or `use fragment` can go.

**4. Configure the copy afterwards.** A building block has no parameters — it's a raw
widget-tree template — so you bind data / set text by editing the *copied* widgets
with `alter page` (their names are deterministic thanks to the prefix):

```mdl
alter page Sales.CustomerOverview set cust_text22 (content: 'Customers');
```

> **Capability reality.** Discovery (`SHOW`/`DESCRIBE BUILDING BLOCK`,
> `CATALOG.building_blocks`) **and** instantiation (`USE BUILDING BLOCK`) both work
> today. `use building block` v1 is **deep-copy + optional `as <prefix>`**; configure
> the copy afterwards with `alter page` (an inline override block is a proposed v1.1).

**When to *mirror* instead.** *Mirroring* — reproducing a block's tree by hand with
`create page`/`alter page` + the same classes and design properties (see below) — is
the fallback: reach for it only to hand-tune a shape Atlas doesn't quite give you.
Otherwise prefer the one-line `use building block`.

---

## Brand re-tune (Layer 1) — where most of the win is

Retune the palette in `theme/web/custom-variables.scss` — the file
`mxcli theme apply` writes (see the next section; do not hand-roll one). Because
Atlas utilities and pluggable widgets read these tokens, one retune re-skins the
whole app:

- `--brand-primary` → buttons, `background-primary`, links, Switch/Slider/ProgressBar
- background + semantic (`success`/`warning`/`danger`) tokens → alerts, group boxes,
  status backgrounds
- `--card-border-radius` and radius tokens → cards, inputs, popups (drop to `0` for a
  sharp, industrial identity; raise for a soft, friendly one)

Only after the token retune, reach for Layer-2 identity classes in `main.scss` — and
only for shapes Atlas can't provide.

---

## Layer 1 in practice — start from the shipped theme

**Do not hand-roll a brand scaffold.** `mxcli theme apply -p app.mpr` writes a
complete, verified Layer 1 (and Layer 2) into `theme/web/`, and `mxcli new`
applies one by default. Re-brand it instead of competing with it — the generated
blocks are digest-fenced, so a hand-written palette in the same file will either
be refused on the next apply or silently fight the theme in the cascade.

```bash
mxcli theme list -p app.mpr            # built-ins + this project's own themes
mxcli theme show signal                # palette, files it writes, token vocabulary
mxcli theme apply signal -p app.mpr    # --variant auto | light | dark
```

When the brand is genuinely yours, make it a theme rather than an edit:

```bash
mxcli theme create acme -p app.mpr                    # scaffold from signal
mxcli theme create acme -p app.mpr --from console     # ...or from console
mxcli theme create acme -p app.mpr --from design.css  # ...and seed the palette
mxcli theme apply acme -p app.mpr
```

It lands in `theme/mxcli-themes/<name>/` — committed, and not compiled until
`apply` copies it into `theme/web/`. Scaffolding copies an existing theme, so the
Atlas map, the recipe layer and the widget layer come across byte for byte; what
you edit is the palette. `--from <file>` reads `--mxt-*` declarations out of any
CSS-shaped text, filing a `prefers-color-scheme: dark` block into the dark
palette. A `--mxt-*` name the base theme does not declare is **refused**, because
nothing would read it — the theme would apply cleanly and render unchanged.

Several themes can be installed at once and switched by a class on `<html>`:

```bash
mxcli theme apply signal ledger console -p app.mpr   # first named is the default
mxcli theme switcher install -p app.mpr --module MyFirstModule
```

### The token architecture it gives you

A theme separates the palette from the wiring, and that split is the whole reason
a light/dark flip or a re-brand is cheap:

| File | Holds | You edit |
|---|---|---|
| `theme/web/custom-variables.scss` | the palette — `--mxt-*` tokens for the default variant | **yes, this one** |
| `theme/web/_mxcli-atlas-map.scss` | ~60 Atlas variables expressed as `var(--mxt-*)` | no |
| `theme/web/_mxcli-<name>.scss` | the other palette, variant blocks, `@font-face`, recipe classes | rarely |

To re-brand, change one line in the palette:

```scss
:root {
  --mxt-brand: #0f6e6b;      /* the one colour that defines the app */
  --mxt-ground: #f4f6f8;     /* app background */
  --mxt-surface: #ffffff;    /* cards, modals, panels */
  --mxt-ink: #14181f;        /* primary text */
  --mxt-line: #dce1e7;       /* hairlines */
}
```

Atlas derives `--brand-primary-50` … `-900` from `--brand-primary` with CSS
`color-mix()`, so buttons, links, active navigation, alerts, group boxes and the
brand-aware pluggable widgets (Switch, Slider, RangeSlider, ProgressBar,
ProgressCircle, BadgeButton) all follow — in **both** palettes, with no
per-widget CSS.

### Two rules that decide whether your styling survives

1. **Mendix 11 Atlas is CSS-custom-property-first.** Write `:root { --x: … }`
   declarations, not SCSS `$x: … !default;`. The stock `custom-variables.scss` is
   a `:root` block plus a few SCSS switches (`$font-family-import`,
   `$btn-bordered`, `$use-css-variables`); legacy Sass variables are still mapped
   for old modules, but they are not the idiom.
2. **Never pin an Atlas variable to a literal colour.** Map it to a token
   (`--bg-color: var(--mxt-ground)`), which is what the Atlas map does. A
   hardcoded `--font-color-default` is near-black on a near-black ground the
   moment anything flips the palette — the failure is total and silent.

If you genuinely need a token the theme does not expose, add it to the palette
block and reference it from your own Layer-2 rules. See `theme-styling` for
the compile order and for why `theme/web/main.scss` is the only correct home for
app-level rules.

---

## Dark mode — Mendix 11 makes this cheap

Older guidance here said to commit to a single theme, because a
`prefers-color-scheme` flip repainted your own classes but left Atlas widgets
light. **That was Atlas 3. It does not hold on Mendix 11.**

Measured by adding `theme-dark` to `<html>` on a running 11.13 app and changing
nothing else: the page ground, cards, form controls, sidebar, buttons and
DataGrid2 all followed. Atlas is CSS-custom-property-first now, so the token
cascade genuinely propagates. And because the class lands on `<html>`, popups and
modals — which Mendix renders at `<body>`, outside any page container — follow it
too, which was the other half of the old objection.

The practical route is `mxcli theme apply <name>` with the default
`--variant auto`: it ships both palettes, follows the OS before first paint, and
honours a `theme-light` / `theme-dark` class when a switcher sets one. Add
`mxcli theme switcher install` for a user-facing toggle.

Three things to know if you build this by hand:

1. **Mendix ships the slot, not the switcher.** `theme/web/_theme-dark.scss`
   declares `:root.theme-dark`; nothing in Atlas ever applies the class.
2. **Your dark block must come after Mendix's** — same specificity, later wins.
   Otherwise its stock Mendix blue overrides your brand the moment the class
   appears.
3. **Anything you pinned to a literal colour breaks.** This is the whole reason
   Layer 1 maps Atlas variables to tokens instead of to hex values.

The rail is the one place Atlas still assumes: several topbar widgets paint text
with `--color-base`, expecting white because they expect a dark navigation rail.
Keep the rail dark in both palettes, or force `color: inherit` on those widgets.

Charts remain the exception — series colour lives in the model
(`customSeriesOptions`), not CSS, so it does not follow a runtime flip. Use the
transparent `paper_bgcolor` trick above, which is correct in both palettes.

The override sheet below is still useful for a hand-rolled dark theme, or for
Atlas corners a token flip misses.

---

## Verify at runtime — this is mandatory

**Runtime verification is not optional.** `mx check` (and `mxcli check --references`)
validate the *model* — they pass MDL the **browser client still crashes on**:

- an old ListView carrying `SearchRefs` the client can't render;
- the Slider / RangeSlider tooltip calling React's removed `findDOMNode` — this only
  throws **on drag**, so a static check (even a static screenshot) misses it;
- a structural change that leaves the client bundle unbuilt (blank `<noscript>` shell).

A model that checks clean can still render a white page. **Never ship on `mx check`
alone.** Keep the app hot and screenshot every change:

```bash
mxcli run --local -p app.mpr --watch --screenshot
```

- **SCSS / theme edits hot-apply** (~1 s) — no restart. Layer-1
  (`custom-variables.scss`) and Layer-2 (`main.scss`) both reflect on the next shot.
- **Page / microflow / text edits hot-apply** too (`reload_model`, ~1 s).
- **Structural changes restart + DDL** (~9 s): a new entity, view entity, or
  association is reconciled only at runtime startup, so `run --local` restarts
  automatically. A hot `reload` won't see a new entity — expect the restart.
- `--screenshot` writes a Playwright PNG (default `<projectDir>/.mxcli/run-local.png`)
  after boot and after **each** applied change.
- `--screenshot-url /p/customers` targets a specific page (repeatable — one PNG each).
- `--screenshot-user` / `--screenshot-password` log in once for pages behind login.

**From an egress-only environment (Claude Code web):** `--hub <url>` reverse-tunnels
the local app out over a single 443 connection to a relay, giving a public URL you can
open in a real browser. `--hub` implies `--local`. See `run-local` for the flags.

**What a screenshot can't catch — drive the interaction.** A single screenshot is a
static frame; the Slider `findDOMNode` throw fires on drag, a filter popover's white
gradient only shows when opened. For interactive widgets, either screenshot the
interacted state or set the safe default up front (Slider `showTooltip: false`).

The rhythm: keep terminal 1 hot (`run --local --watch --screenshot`); in terminal 2
apply one slice (`mxcli exec 06-redesign.mdl -p app.mpr`) and look at the PNG. A
designed result is reached by looking at the running app, not by trusting the checker.

---

## Gotchas catalog

Each cost real time in the builds this skill was distilled from. Match a symptom to a
row before opening files.

### Styling & pages

| Gotcha | Fix |
|---|---|
| `$` in `dynamictext content:` breaks the parser (starts a variable token) | put the `$` in CSS `::before`; bind only the number |
| Enum `dynamictext` renders the **key**, not the caption | accept it, or map the enum to a class via `dynamicclasses` |
| `sort by` not allowed on **association-sourced** listviews | sort the parent, or use a DB datasource |
| Reserved widget identifiers exist (e.g. `v3`) | prefix names (`sv3`); avoid bare `v<n>` |
| Pluggable widgets impose their own DOM (charts / timeline / treenode) | for pixel-fidelity use a native `listview` / `gallery` you fully style |
| Inline `style:` on a `dynamictext` crashes MxBuild (NullReferenceException) | use `class:`, or wrap the text in a styled `container` |
| `alter styling` can't find widgets in MDL-builder-created pages | apply classes via `Class:` / `DynamicClasses:` in `create page` / `alter page` |
| Full-screen page wanted (no Atlas sidebar) but no blank layout resolves | keep a normal Atlas layout; hide the shell per-page with `.mx-page:has(.my-app) .region-sidebar { display:none }` |
| "Colour by state" (status pills / cards) | one `dynamicclasses` enum→class expression + one `--st` CSS var cascaded into pill/number/dot/border |

### Charts

| Gotcha | Fix |
|---|---|
| Chart widgets render **raw Plotly defaults** (flat colour, floating mode-bar, white paper, heavy grid) | `customLayout` (transparent bg + system font + faint grid) + `customConfigurations` `displayModeBar:false` + per-series `customSeriesOptions` (colour, `cornerradius`, spline) |
| Horizontal **BarChart** with `aggregationType: sum` prepends a `0` group-key to category ticks (`"0Tokyo Spring"`) | use `aggregationType: none` when the datasource is already one row per category |
| **Chart colours don't re-skin** — series colour lives in the model (`customSeriesOptions`), not CSS | accept it's model config; a palette pivot needs an MDL edit + gen-2 restart, not a theme edit |

### Dark mode & widgets

| Gotcha | Fix |
|---|---|
| Atlas widgets + Plotly **aren't dark-aware** — a `prefers-color-scheme` flip leaves them light on a dark page | ship the dark-mode override block above (form controls, datagrid + filters/popovers, accordion, fieldset, **treenode white rows**, transparent charts), or ship light-only |
| `.widget-dropdown-filter-menu` paints a **hardcoded white scroll-fade gradient** even after bg is themed | override `background-image:none` and brighten menu-item text |
| **Edit popup has a white title bar** — `.mx-window` / `.modal-content` renders at `<body>`, outside your scoped class | theme `.mx-window-content` / `.modal-content` + header + form controls/buttons **globally**, not scoped |
| **Slider / RangeSlider** throw "Could not render widget" on drag (tooltip calls React `findDOMNode`, removed in MX 11) | set `showTooltip: false` |
| Half-dark clash (your chrome dark, Atlas widgets light) | **commit to one theme**: for a dark app drop the media gate and make overrides unconditional + global; ship **light-only** if you can't fund the override recipe |

### Theme / SCSS

| Gotcha | Fix |
|---|---|
| Google-fonts `@import url()` silently dropped | make it the **first line** of `main.scss` (before the partial import and any rule); keep a system fallback stack |
| Full re-skin desired (new identity) | it's **theme-only** — retune `custom-variables.scss` (Atlas leaves) + `main.scss` (custom tokens + classes); no page/MDL edits, hot-applies under `--watch` |
| "SCSS cache" — edits don't show | it's never a cache: use `--watch` (watches theme source) or a clean restart; kill any stale process first |
| Stale process serves old output, looks like a cache | `run --local` refuses occupied ports; free them (`pgrep`/`kill`, `curl` returns 000 when down) |

### Data / microflows behind the design (styling depends on real data)

| Gotcha | Fix |
|---|---|
| Seed microflow data doesn't appear (queries empty) | **`create` doesn't persist — add `commit $obj;`**; the miss is silent (no error) |
| Bare `$x = avg(...)` or `$x = 2` fails to parse | bare `$x = …` accepts only `count`/`sum` aggregates; use `declare $x T = expr` for other expressions, `set $x = expr` to reassign |
| Aggregates can't be inlined in a create-object assignment (CE0117) | compute into vars first |
| Integer/integer division `$a / $b` → CE0117 | Mendix `/` needs a decimal operand; compute upstream or store decimals |
| View entity flagged CE6770 "out of sync" | the view's declared attribute types must match its OQL source columns; a grouped enum column must be typed `enumeration(Module.Enum)`, not `string` |

### Verify

| Gotcha | Fix |
|---|---|
| **`mx check` passes but the browser client crashes** (old ListView `SearchRefs`; the Slider `findDOMNode` throw only fires on interaction) | **always Playwright-verify a running build; never ship on `mx check` alone** |
| `ALTER PAGE SET layout … map(…)` swaps a page onto a sidebar shell | it does so without rebuilding the widget tree — use it to re-parent, not to rebuild |

---

## Validation Checklist

- [ ] **Atlas-first** — reached for `class:`/design properties (Layer 0) and brand
      tokens (Layer 1) before any custom CSS
- [ ] **Discovered** the project's building blocks (`show building blocks`) and
      **inspected** the target block (`describe building block …`) before using it
- [ ] **Instantiated** with `use building block Mod.Name [as prefix_]` (the one-liner),
      then configured the copied widgets with `alter page` — mirrored by hand only as a
      deliberate fallback
- [ ] **Brand tokens retuned** in `theme/web/custom-variables.scss` so Atlas
      components inherit the palette; custom SCSS reserved for identity only
- [ ] **Committed to a theme count** up front (light-only or dark-only beats half-dark)
- [ ] **Charts themed** with transparent `customLayout` + `displayModeBar:false`
      when the app has charts
- [ ] **Runtime-verified** with `run --local --watch --screenshot` — never shipped
      on `mx check` alone
- [ ] Every MDL snippet passes `mxcli check`

## Related skills

- `theme-styling` — SCSS compilation chain, hot-reload, styling caveats
- `migrate-design-prototype` — turning a Claude Design handoff into a theme + pages
- `create-page` — page/widget syntax
- `alter-page` — in-place widget edits
- `fragments` — reusable widget groups (how the mirror recipes stay DRY)
- `run-local` — the warm dev loop and screenshot flags
