# Migrate a Claude Design Prototype into a Mendix App (Theme + Pages)

## When to Use This Skill

Use this skill when you are given a **Claude Design prototype / design handoff** (an
HTML/CSS prototype, a `*.dc.html` design-console export, a tokens file, a PRD, and/or
screenshots) and need to reproduce that look in a Mendix app using **mxcli + MDL**.

It covers the two halves of the job:

1. **Build the SCSS theme** — turn the prototype's design language (colours, fonts,
   spacing, component styles) into a Mendix theme in `theme/web/main.scss`.
2. **Apply it in pages** — attach the theme's classes to widgets with MDL
   (`Class:` / `DynamicClasses:` on `create page` / `alter page`).

Related skills: `theme-styling.md` (SCSS compilation chain, hot-reload, styling caveats),
`create-page.md` (widget syntax), `alter-page.md` (in-place widget edits),
`bulk-widget-updates.md` (apply a class across many widgets).

---

## The Pipeline at a Glance

```
Claude Design handoff                         Mendix app
─────────────────────                         ──────────────────────────────
*.dc.html / prototype  ──①  extract  ──►  :root { --ss-* }  design tokens
tokens / CSS / PRD          tokens        + map onto Atlas  (--brand-primary …)
                                                    │
component styles       ──②  rebuild  ──►  .ss-* component classes in main.scss
(cards, chips, …)           as classes            │
screenshots            ──③  reference ──►  widgets get Class: / DynamicClasses:
(per screen)                per screen          via create page / alter page
                                                    │
                            ──④  build   ──►  docker build → docker reload --css
                                                    │
                            ──⑤  verify  ──►  compare running screen to screenshot
```

**Golden rule:** the prototype is the source of truth. Before building or polishing any
screen, open the matching screenshot/handoff for that screen and match it — colours,
spacing, font, component shapes. Do not invent styling the prototype doesn't show.

---

## Where the Theme Lives (read this first — it avoids the main friction)

- **Custom styles go inline in `theme/web/main.scss`, AFTER the `@import`s.** mxcli/the
  agent generally **cannot create new SCSS partials** (only existing files are writable),
  and styles placed after the imports win the cascade over Atlas defaults. Put all your
  design tokens and component classes there.
- Use a **project prefix** for every custom class and CSS variable so they never collide
  with Atlas or widget CSS. This project uses `ss-` (e.g. `.ss-panel`, `--ss-primary`).
  Pick one prefix and use it everywhere.
- `theme/web/custom-variables.scss` is the right place for **Atlas brand variables** you
  want to override globally (`--brand-primary`, `--brand-success`, …). Deep look-and-feel
  (bespoke chrome, component classes) belongs in `main.scss`.
- Do **not** hand-edit `theme-cache/web/` — that is the compiled build artifact.

---

## ① Extract Design Tokens

Pull the raw values out of the handoff (CSS custom properties, a Figma/tokens JSON, or by
reading the prototype's stylesheet) and declare them once as a prefixed `:root` block, then
**map the important ones onto Atlas variables** so the standard Atlas shell inherits the
look for free.

```scss
// main.scss — after the @imports
@import url("https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600;700&display=swap");

:root {
  /* --- Palette straight from the handoff --- */
  --ss-app-bg: #eef1f4;
  --ss-surface: #ffffff;
  --ss-ink: #1a2129;
  --ss-text-secondary: #5c6a78;
  --ss-border: #dde3ea;
  --ss-primary: #2b5170;
  --ss-chrome: #0f1720;

  /* Status tokens (background / text / border triples) */
  --ss-ok-bg: #eef4ef;     --ss-ok-text: #4a7a5c;     --ss-ok-border: #d3e5d9;
  --ss-danger-bg: #fbf0ee; --ss-danger-text: #a13a2c; --ss-danger-border: #ecc9c2;

  /* --- Map onto Atlas so the built-in shell matches --- */
  --brand-primary: var(--ss-primary);
  --topbar-bg: var(--ss-chrome);
  --sidebar-bg: var(--ss-chrome);
  --navigation-bg: var(--ss-chrome);
  --bg-color: var(--ss-app-bg);
  --font-family-base: "IBM Plex Sans", sans-serif;
  --font-color-default: var(--ss-ink);
  --font-color-header: var(--ss-ink);
}
```

Token checklist to extract from the handoff:

| Category | Typical tokens |
|----------|----------------|
| Palette | surface, app background, ink/text, secondary/muted text, borders, primary/brand |
| Status | ok / warning / danger — as background + text + border triples |
| Chrome | topbar/sidebar background + border, nav idle/active/active-bg colours |
| Typography | font family (import the webfont), header/body sizes, weights, a mono family for labels/metrics |
| Shape | border-radius scale, shadow(s), panel/card padding |

> **Fonts:** if the design uses a non-Atlas font (e.g. IBM Plex), `@import` the webfont at
> the top of your custom block **and** set `--font-family-base`. Keep a helper class for
> any secondary family, e.g. `.ss-mono { font-family: "IBM Plex Mono", monospace; }`.

---

## ② Rebuild Components as Classes

For each repeated element in the prototype (panel, stat tile, chip, card, table row,
progress bar…) write **one reusable class** driven by the tokens from step ①. Keep classes
small and composable so a widget can stack several (`Class: 'ss-panel ss-grid-lv'`).

```scss
// White surface panel
.ss-panel {
  background: var(--ss-surface);
  border: 1px solid var(--ss-border);
  border-radius: 8px;
  box-shadow: 0 1px 2px rgba(20, 33, 45, 0.05);
}

// Status chip — one base + colour modifiers
.ss-chip {
  display: inline-block; border-radius: 11px; padding: 2px 10px;
  font-family: "IBM Plex Mono", monospace; font-size: 11px; font-weight: 600;
  border: 1px solid transparent;
  white-space: nowrap;              // status chips must never wrap to 2 lines
}
.ss-chip--ok     { background: var(--ss-ok-bg);     color: var(--ss-ok-text);     border-color: var(--ss-ok-border); }
.ss-chip--danger { background: var(--ss-danger-bg); color: var(--ss-danger-text); border-color: var(--ss-danger-border); }
```

**Base + modifier convention.** Give each component a base class and add `--variant`
modifiers for state/colour (`.ss-chip` + `.ss-chip--danger`, `.ss-heat--ok/--warn/--over`).
Widgets then combine base + modifier: `Class: 'ss-chip ss-chip--danger'`.

**Reshaping Mendix chrome.** To make Atlas widgets read like the prototype you often need
to override Mendix's own DOM classes. Common targets:

- Sidebar / topbar shell: `.region-topbar`, `.mx-header`, `.region-sidebar`,
  `.mx-scrollcontainer-left`, and nav items under `.mx-navigationtree`.
- ListView rows are the workhorse for grids/tables — neutralise Atlas's default row
  chrome (padding/border/background) so rows read as your design's grid lines:

  ```scss
  .ss-grid-lv > ul > li,
  .ss-grid-lv .mx-listview-item {
    padding: 12px 16px !important;
    margin: 0 !important;
    border-bottom: 1px solid var(--ss-border-light);
  }
  ```

- `::before` / `::after` on `.mx-navigationtree` can inject brand blocks / section labels
  the design shows but the Mendix nav model doesn't produce.

Use `!important` sparingly but expect to need it when overriding Atlas widget CSS.

---

## Component → Mendix widget map

The lookup that removes the guesswork: for each component in the prototype, pick the widget
here **first**, then style it with your `--<prefix>` classes. Validated across the BAE Resource
Scheduling and Expense Approval designs.

| Design component | Mendix widget | Notes |
|---|---|---|
| Page / screen canvas | `container` | one per page, e.g. `Class: 'ea-page'` |
| Card / panel / section | `container` | + a panel class |
| KPI / stat tile | `container` | label + value + delta as child `dynamictext` |
| Multi-column / dashboard layout | `layoutgrid` + `row` + `column` | for exact fractional tracks (`2.4fr 1.2fr …`) use a `container` styled `display:grid` instead — see Layout techniques |
| Heading / title | `dynamictext` (RenderMode H1/H2) | |
| Body / label / caption / table cell | `dynamictext` | the workhorse — text is inline, see techniques |
| Metric / big number | `dynamictext` | mono class |
| Chip / badge / tag / status pill | `dynamictext` | base class + colour modifier; leading dot via CSS `::before` |
| Data table / grid / row list | `listview` (database source; row = `layoutgrid` or grid `container`) | preferred for bespoke row layouts — full control of the row markup; the `datagrid` pluggable widget exists but is heavier to style to a custom design |
| Table header row | static header band (`container`/`layoutgrid`) above the listview | |
| Tabs / segmented / filter-chip row | `tabcontainer` styled as pills (one `tabpage` per XPath-filtered view) | for static/decorative chips use `dynamictext` |
| Master list + detail pane | `listview` (Selection) + `dataview` (DataSource: SELECTION) | |
| Detail / read view | `dataview` | |
| Create / edit form | `dataview` + inputs + `footer` | Save/Cancel in the footer |
| Text input / multiline | `textbox` / `textarea` | |
| Dropdown / enum select | `combobox` | bound to an enum or association |
| Date field | `datepicker` | |
| Boolean / toggle | `checkbox` | |
| Button (primary/secondary) | `actionbutton` | `ButtonStyle` or a class |
| Link / text button | `linkbutton` | |
| Search box | listview built-in search bar | hoist/restyle via CSS |
| Avatar / initials | `dynamictext` | styled as a circle |
| Image / logo / thumbnail | `image` / `dynamicimage` / `staticimage` | |
| Icon / colour dot | CSS `::before` on a class | |
| Chart (line/bar/column/pie/area/bubble) | chart **pluggable widget** (Mendix Charts / ChartJS) | via `PLUGGABLEWIDGET '<id>'` — see Pluggable widgets below; needs a datasource + series config |
| Donut / gauge | ProgressCircle **pluggable widget** | via `PLUGGABLEWIDGET '<id>'`; static or attribute-driven — worked example below |
| Progress bar / meter | `progressbar` widget, or a `container` (track + fill) | a styled track+fill container needs no widget package |
| Sparkline / bespoke SVG | HTMLElement **pluggable widget**, or a `container` with a CSS SVG background | embed the design's inline SVG directly |

### Layout techniques

- **Exact fractional columns.** Atlas's `layoutgrid` is a 12-column system and can't express
  ratios like `2.4fr 1.2fr 1fr 1.4fr 0.6fr`. For pixel-faithful tables/dashboards, style a plain
  `container` as `display:grid; grid-template-columns: …` in its class and put the cell widgets
  as its **direct children** — each widget becomes a grid item.
- **`dynamictext` is inline by default.** For stacked text (a title over a subtitle) set
  `display:block` in the class, or the lines run together.

### Pluggable widgets (charts, donut, HTML/SVG)

Pluggable widgets **do round-trip through MDL** — but not by bare name. `progresscircle` /
`piechart` / `CUSTOMWIDGET` are all rejected by the builder (*"unsupported widget type"*). The
working form uses the widget's **full package id** as a quoted string:

```
PLUGGABLEWIDGET '<widget.package.id>' widgetName ( prop: value, … ) { childslots }
```

**One-time registration.** The widget package must be present in the project's `widgets/`
before you can reference its id:

```bash
mxcli widget init    -p baedemo.mpr                 # scaffold pluggable-widget support (run once)
mxcli widget extract -p baedemo.mpr --mpk widgets/ProgressCircle.mpk   # register a package
mxcli widget list    -p baedemo.mpr                 # list available widget ids + their props
```

`mxcli widget list` prints each widget's id and property names — copy the id **verbatim** into
the `PLUGGABLEWIDGET '…'` string, and use the property names it reports as the widget's props.

**Worked example — the status donut (Expense Dashboard).** A ProgressCircle in static mode,
with a text label overlaid via a sibling `container` (the widget draws only the ring):

```sql
container donutWrap (Class: 'ea-donut') {
  PLUGGABLEWIDGET 'com.mendix.widget.custom.progresscircle.ProgressCircle' donut (
    type: 'static', staticCurrentValue: 67, staticMinValue: 0, staticMaxValue: 100, showLabel: false
  ) { }
  container donutLabel (Class: 'ea-donut-label') {
    dynamictext donutPct (Content: '67%', Class: 'ea-donut-pct')
  }
}
```

This passed `mx check` with 0 errors, survived `docker build`, and renders its SVG arc at
runtime (verified on the dashboard). Charts follow the same shape but need a **datasource +
series config inside the child slot** rather than static values.

**Pluggable-widget gotchas:**
- **Reach for built-ins first.** `listview`, `dynamictext`, `container`, `gallery`, `combobox`
  need no registration. Drop to a pluggable widget only when the design genuinely needs one
  (charts, gauges, embedded SVG, maps, sliders). Many "charts" in a handoff are just static
  SVG — a `container` with a CSS `background` SVG (KPI sparklines, area trends here) is lighter
  than a real chart widget and needs no datasource.
- **Reserved keywords can't be widget names** — `activity`, `legend`, etc. are rejected by the
  parser; rename (`actCard`, `legendCol`).
- **Empty slot is `{ }`.** Always close the child-slot braces, even when empty.

---

## The App Shell: Navigation & Layout (built once, not per page)

Most Claude Design prototypes render a **persistent sidebar + topbar** on every screen — a
brand block, a menu, sometimes a footer tag. It is tempting to rebuild that chrome inside each
page. **Don't.** In Mendix the shell is not a page — it comes from two shared places:

- **The layout** (`Atlas_Core.Atlas_Default` in this project) provides the topbar + left
  sidebar regions. Every page sets `Layout: Atlas_Core.Atlas_Default`, so they all inherit the
  same shell; the page's own widgets render only in the content region.
- **The navigation profile** supplies the menu items. One `Responsive` profile drives the whole
  app — home page, login page, and the flat/nested menu. Menu items point at pages, not at
  widgets you place.

So the prototype's sidebar maps to **navigation config + layout styling**, configured once, and
its menu grows by adding navigation items — never by editing pages.

### Add a screen to the menu

```bash
mxcli -p baedemo.mpr -c "SHOW NAVIGATION"              # profiles, home page, item count
mxcli -p baedemo.mpr -c "SHOW NAVIGATION MENU Responsive"   # the menu tree
```

Add or reorder items with `CREATE OR REPLACE NAVIGATION <Profile> …` (full-replacement — dump
the current profile first with `DESCRIBE NAVIGATION <Profile>`, edit, re-apply). See
`manage-navigation.md` for the item syntax, home/login pages, and role-based homes.

### Style the shell to match the design

The menu items and regions are standard Atlas DOM, so the prototype's look is reproduced with
CSS in `main.scss` (step ②) — you do **not** model the sidebar's chrome as widgets:

- Recolour the regions via the mapped Atlas vars (`--sidebar-bg`, `--topbar-bg`, `--navigation-bg`)
  or by overriding `.region-sidebar` / `.region-topbar` / `.mx-header` directly.
- Restyle menu entries under `.mx-navigationtree` (idle / hover / active states, spacing, the
  active-item accent bar).
- **Inject chrome the nav model can't express** — a brand logo block, a `WORKSPACE` section
  label, an `ITERATION 1 · DEMO` footer tag — with `::before` / `::after` on `.mx-navigationtree`
  (or the sidebar region). The Mendix navigation model has no field for these, so CSS
  pseudo-elements are the right tool; keep their text in the SCSS with the rest of the theme.

> Rule of thumb: if a design element is **the same on every screen**, it belongs to the shell
> (navigation + layout + CSS), not to a page. Only the content region is built per-page in ③.

### Restructuring the shell to match the prototype (full-height sidebar, fixed topbar)

Recolouring is rarely enough — most prototypes put a **full-height sidebar** (brand block at the
very top) with the topbar **only over the content**, whereas `Atlas_Default` renders the topbar
full-width *above* a sidebar+content row. Reproduce the prototype layout with CSS, no custom
layout document needed:

- **Full-height sidebar:** pin it out of the flex flow and offset the rest.
  ```scss
  .mx-scrollcontainer-left.region-sidebar { position: fixed; top: 0; left: 0; height: 100vh; z-index: 100; }
  .region-topbar, .mx-scrollcontainer-top,
  .region-content, .mx-scrollcontainer-center { margin-left: 256px !important; }  /* = sidebar width */
  ```
- **Force region sizes with `flex-basis`, not `width`/`height`.** Atlas sizes the topbar and
  sidebar as **flex items**, so plain `width`/`height` is ignored — use
  `flex: 0 0 256px !important` (sidebar) and `flex: 0 0 62px !important` (topbar).
- **Hide the collapse toggle** when the design has no collapse: `.toggle-btn { display: none !important; }`.
- The sidebar's inner scroll area (`.mx-scrollcontainer-wrapper`) should be `flex: 1 1 auto` so a
  `::after` footer tag pins to the bottom.

### Pixel-exact multi-part chrome via inline-SVG backgrounds

A `::before`/`::after` pseudo-element is a **single box with one text style** — it cannot render a
brand block (logo square + title + differently-styled subtitle) or a user chip (avatar circle +
two text lines) faithfully. For those, render the whole element as an **inline-SVG `background`
image**, which gives you exact sub-shapes, fonts, and colours:

```scss
.region-sidebar::before {
  content: ''; height: 80px; border-bottom: 1px solid rgba(255,255,255,.07);
  background: url("data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' width='256' height='80'>\
<defs><linearGradient id='g' x1='0' y1='0' x2='1' y2='1'><stop offset='0' stop-color='%233a6fd6'/>\
<stop offset='1' stop-color='%231d3f86'/></linearGradient></defs>\
<rect x='22' y='21' width='38' height='38' rx='10' fill='url(%23g)'/>\
<text x='41' y='45' fill='%23fff' font-family='Public Sans' font-size='15' font-weight='800' text-anchor='middle'>EA</text>\
<text x='72' y='37' fill='%23fff' font-family='Public Sans' font-size='15' font-weight='700'>Expense Approval</text>\
<text x='72' y='55' fill='%237d8ba4' font-family='IBM Plex Mono' font-size='11'>Finance workflow</text></svg>") no-repeat;
}
```

Encode `#` as `%23` in the data URI. Use the same trick for the topbar's decorative search field
(a `::before` box with a magnifier-icon SVG background) and the user chip (`::after`). Simpler
single-style chrome — the `WORKSPACE` label, the `PROTOTYPE · DEMO` footer, per-item nav **dots**
(`.mx-navigationtree a::before { content:''; width:9px; height:9px; border-radius:3px; background: … }`,
coloured per item by its `.mx-name-*` anchor class) — stay as plain pseudo-elements.

> Decorative-only: an injected search box / avatar / "New" button is **not interactive** (it's
> CSS). The prototype's are usually mockups too; if the design needs a *working* search or profile
> menu, that's a real widget in a custom layout (Studio Pro), not CSS on the Atlas shell.

---

## ③ Apply Classes in Pages (MDL)

Every Mendix widget takes a `Class:` (static) and `DynamicClasses:` (expression) property.
This is how the theme reaches the page. Prefer **native Mendix widgets** styled with your
classes — `container`, `listview`, `dataview`, `dynamictext`, `tabcontainer` — over custom
widgets, which are far harder to drive from MDL.

### Static classes — the primary mechanism

Space-join base + modifiers in a single `Class:` string:

```sql
create or replace page ResourceScheduling.ResourceHeatmap (
  Title: 'Resource Heatmap', Layout: Atlas_Core.Atlas_Default
) {
  container heatmapPage (Class: 'ss-page') {
    dynamictext heatmapTitle (Content: 'Resource Heatmap', RenderMode: H1, Class: 'ss-page-title')

    listview loadLV (
      DataSource: database from ResourceScheduling.Resource where LoadSeries != empty,
      Class: 'ss-panel ss-heat-lv'
    ) {
      container heatRow (Class: 'ss-heat-row') {
        dynamictext hc01 (Content: '{1}', ContentParams: [{1} = M01], Class: 'ss-heat-cell')
      }
    }
  }
}
```

### State-driven styling — `DynamicClasses:` expression

For colour/state that depends on data (over-capacity cell, conflict card, load bucket),
use a `DynamicClasses:` expression returning a space-separated class string. It **stacks on
top of** `Class:`.

```sql
container heatCell (
  Class: 'ss-heat-cell',
  DynamicClasses: 'if $currentObject/M01 >= 100 then ''ss-heat--over''
                   else if $currentObject/M01 >= 80 then ''ss-heat--warn''
                   else ''ss-heat--ok'''
)
```

(Note the doubled single-quotes for string literals inside an MDL expression.)

### Adding classes to an existing page

Use `alter page` to attach a class without rewriting the page (see `alter-page.md`):

```sql
alter page ResourceScheduling.Approvals {
  set Class = 'ss-appr-card ss-appr-card--conflict' on queueCard;
}
```

To apply the same class across many widgets/pages at once, see `bulk-widget-updates.md`
(`update widgets ... dry run` first).

---

## ④ Build & Preview

SCSS is **not** live — you must compile before the theme shows. See `theme-styling.md`.

```bash
mxcli docker build -p baedemo.mpr      # compiles SCSS into the deployment package (~55s)
mxcli docker reload -p baedemo.mpr --css   # pushes compiled CSS to browsers (instant)
```

- `--css` only pushes already-compiled CSS — always `docker build` first after editing SCSS.
- For widget-property changes (a new `Class:` on a page), use a normal `docker reload`.

---

## ⑤ Verify Against the Prototype

Put the running screen next to the handoff screenshot and reconcile the diff — spacing,
colours, font, component shapes. The project already has Playwright wired in (`test-app.md`)
for screenshotting the running app. Iterate ②–④ per screen until it matches.

---

## Gotchas (learned building this app)

- **Never put `Style:` (inline style) on a `DYNAMICTEXT`** — it crashes MxBuild with a
  NullReferenceException. `Class:` on a DYNAMICTEXT is fine; for inline style, wrap it in a
  styled `container` instead. (Same applies to `alter styling`/`alter page set style`.)
- **Prefer `Class:` + a real CSS class over inline `Style:`.** It keeps the design system in
  one place and dodges the DYNAMICTEXT crash.
- **`DynamicClasses:` for state, not duplicate widgets.** One widget + an expression beats
  cloning a widget per state.
- **Status chips: `white-space: nowrap`** so labels like "Fully allocated" never wrap.
- **ListView rows carry Atlas padding/border/background** — neutralise them in your
  `.ss-*-lv` class or rows won't read as the design's grid.
- **For bespoke tables, prefer a styled `listview`** over the `datagrid` pluggable widget —
  you control the full row markup, which a pixel-faithful design usually needs.
- **Design-property keys are case-sensitive** — see `theme-styling.md` if you use
  `DesignProperties:` instead of raw classes.
- **`alter styling` can't find widgets in MDL-builder-created pages** — apply classes via
  `Class:`/`DynamicClasses:` in `create page` / `alter page` instead.

## MDL gotchas that block a faithful build

Distilled from a full prototype build. Some are covered in depth by the linked skills — this is
the fast index so a design migration doesn't rediscover them.

- **A real `docker build` is the only trustworthy check.** `mxcli check --references` passes MDL
  that `mxbuild` rejects. Build after every slice before you screenshot.
- **Quote to escape keywords in bindings; the unquoted form is cleaner in expressions.** Quote
  identifiers in `create entity`, `attribute:` bindings, and `create`/`change` targets when they
  collide with an MDL keyword. Inside expressions — microflow `if`/decisions, `contentparams`,
  `visible`, `dynamicclasses`, and inline-bracket XPath `where` — mxcli now **strips** stray
  identifier quotes, so a quoted attribute no longer breaks the build or makes an XPath `where`
  silently return 0 rows; still prefer the unquoted form there for readability. See
  `write-microflows.md`.
- **Show a related (to-one) object's attributes** — two ways, both persist now:
  - A nested **"data from context" DataView** for a full read view of the referenced object; its
    children bind to the related entity:
    ```
    dataview dvEmp (datasource: $currentObject/Module.Entity_Related) {
      dynamictext n (content: 'By {1}', contentparams: [{1} = Name])   -- own attr of the related entity
    }
    ```
  - An **association path** for a single inline value: `contentparams: [{1} = Entity_Related/Name]`
    in a `dynamictext`, or `attribute: Entity_Related/Name` on a DataGrid2 column — both persist as
    an AttributeRef over the association (use a **bare** association name; a module-qualified one is
    rejected on a column).
- **Status chips = one `dynamictext` + a `DynamicClasses:` expression** mapping the enum/value to
  a colour modifier (`ea-chip--ok/--warn/--danger`). Base `.ea-chip` + modifiers; `white-space: nowrap`.
- **Charts:** `ProgressCircle` / `ProgressBar` build and work (donut, meters). **Mendix Charts
  (`Charts.mpk`: column/bar/line/area/pie)** now author via MDL — each `series` (an object-list
  item inside the chart) binds a datasource plus X/Y attributes:
  `series s1 (staticDataSource: database from Module.View, staticXAttribute: "X", staticYAttribute: "Y")`
  (a per-series OQL view works too). `mxcli docker check`/`build` run `mx update-widgets`, which
  clears the widget-version-drift CE0463. Still lighter when the design allows: a **CSS-background
  SVG** container (or `HTMLElement`) for sparklines/trends — no datasource — and `ProgressCircle`
  (`type: expression`, `expressionCurrentValue: '$currentObject/Rate'`, min `'0'` / max `'100'`,
  `labelType: percentage`) for a single-value gauge.
- **Dashboard aggregates → OQL view entities** (`write-oql-queries.md`). A grouped enum column in
  the view must be typed `enumeration(Module.Enum)`, not `string`, or you get CE6770. Test with
  `mxcli oql` first.
- **Seed demo data via an after-startup microflow** guarded to run once (skip if data exists); it
  must `return true`. Direct-SQL seeding doesn't apply to the default **HSQLDB**. See `demo-data.md`.
- **New entities / view entities need a runtime restart** (`docker up --detach --wait`) to
  register — a hot `reload` won't see them. Hot `reload` also occasionally crashes the runtime; if
  the app stops responding, `docker up --detach --wait` recovers it with data intact.
- **mxcli behaviour shifts between versions** — the chart-build and a few persistence quirks are
  version-dependent. If a documented property silently vanishes on write, check `mxcli version`
  and confirm with `describe page`.

---

## Checklist

- [ ] Read the handoff/screenshot for the screen **before** building or polishing it
- [ ] Design tokens declared once as prefixed `:root` vars, **after** the `@import`s in `main.scss`
- [ ] Key tokens mapped onto Atlas variables (`--brand-primary`, `--topbar-bg`, `--font-family-base`, …)
- [ ] Non-Atlas fonts `@import`ed and set via `--font-family-base`
- [ ] Components are base classes + `--variant` modifiers, all using the project prefix
- [ ] Persistent sidebar/topbar built as **navigation profile + layout + CSS**, not per-page widgets; new screens added via `CREATE OR REPLACE NAVIGATION`
- [ ] Shell **restructured** to the design (full-height sidebar via `position:fixed` + `margin-left` offset; region sizes forced with `flex-basis`; collapse toggle hidden if unused)
- [ ] Multi-part chrome (brand block, user chip) rendered as **inline-SVG backgrounds**; single-style chrome (labels, dots, footer) as plain `::before`/`::after`
- [ ] Related (to-one) object attributes shown via a **nested "data from context" DataView** (full read view) or an **association path** (`contentparams: [{1} = Assoc/Attr]` / column `attribute: Assoc/Attr`) for a single inline value — both persist
- [ ] A real **`docker build`** run after each slice (not just `mxcli check`) before screenshotting
- [ ] Widgets styled with `Class:`; data-driven state via `DynamicClasses:`
- [ ] No inline `Style:` on any DYNAMICTEXT
- [ ] Grids/tables built as styled `listview`s, not custom Datagrid widgets
- [ ] Charts/gauges use `PLUGGABLEWIDGET '<id>' …` with the package registered via `mxcli widget extract`; static SVG (sparklines) done as CSS-background containers
- [ ] `docker build` then `docker reload --css` after SCSS edits
- [ ] Running screen verified against the prototype screenshot
