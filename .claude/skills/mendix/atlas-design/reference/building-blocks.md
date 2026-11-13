# Atlas building blocks and appearance vocabulary

Supporting reference for [atlas-design](../SKILL.md).

## Atlas building blocks — the out-of-the-box inventory

Every Mendix project ships **`Atlas_Web_Content`**, a library of **39 building
blocks**: pre-composed widget shapes that Mendix itself uses. They are the canonical
reference for "what a well-made X looks like in Atlas."

### The inventory (real names, grouped by category)

| Category | Blocks |
|---|---|
| **Cards** | `Card`, `Card_Action`, `Card_ActionWithImage`, `Card_Background`, `Card_WithImage` |
| **Headers** | `Heroheader`, `Heroheader_Background`, `Heroheader_WithAction`, `Pageheader`, `Pageheader_WithBack`, `Pageheader_WithControls`, `Pageheader_WithSearch`, `PageheaderImage`, `PageheaderImage_WithBack`, `PageheaderImage_WithControls` |
| **Forms** | `Form_Horizontal`, `Form_Horizontal_WithTitle`, `Form_Horizontal_WithAction`, `Form_Vertical`, `Form_Vertical_WithTitle`, `Form_Vertical_WithAction` |
| **Lists** | `List_Cards`, `List_WithImage`, `ListItem_SingleLine`, `ListItem_DoubleLine`, `ListItem_WithImage` |
| **Master Detail** | `Master_Detail` |
| **Timeline** | `Timeline`, `Timeline_WithImage` |
| **Wizards** | `Wizard_Arrow`, `Wizard_Arrow_Step`, `Wizard_Circle`, `Wizard_Circle_Step` |
| **Notifications** | `Alert`, `Alert_WithAction`, `AlertIcon`, `AlertIcon_WithAction` |
| **Breadcrumbs** | `Breadcrumb`, `Breadcrumb_Underline` |

All are `Platform: Web`, all live in module `Atlas_Web_Content`, referenced as
`Atlas_Web_Content.<Name>`.

> Your project may ship more blocks from installed modules (e.g. a feedback widget).
> Always `list building blocks` on the actual project rather than trusting this list —
> it is the standard Atlas baseline, not an exhaustive per-project inventory.

### Capability reality: discover, inspect, and instantiate

| Capability | State |
|---|---|
| **Discover** — `LIST BUILDING BLOCKS`, `CATALOG.building_blocks` | ✅ shipped |
| **Inspect** — `DESCRIBE BUILDING BLOCK Mod.Name` (full widget tree) | ✅ shipped |
| **Instantiate** — `use building block Mod.Name [as prefix_]` onto a page | ✅ v1 (deep-copy; configure afterwards with `alter page`; legacy engine today) |
| **Author** — `CREATE BUILDING BLOCK` | ❌ not yet (proposed) |

The one-line `use building block` (above) is the normal path — deep-copy the block,
then configure the copy. **Mirroring** — reproducing a block's widget tree by hand —
is the fallback for hand-tuning or the modelsdk engine; the how-to is below.

### How to mirror a block

1. **Inspect it.** `describe building block Atlas_Web_Content.<Name>`.
2. **Read both channels.** Atlas blocks style with `Class:` strings *and* typed
   `DesignProperties:` — copy both.
3. **Reproduce the tree** on your page, binding real data where the block has
   placeholder text (`'Card title'` → your attribute/content).
4. **DRY it** — if the shape repeats, put it in a `create fragment` and `use` it.

### Worked example — `Card`

`describe building block Atlas_Web_Content.Card` yields the tree shown above. Mirror
it onto a page, binding real content:

```mdl
mdl 1;
create page MyModule.CardDemo
(
  title: 'Card demo',
  layout: Atlas_Core.Atlas_Default
)
{
  container myCard (designproperties: ('Card style': on)) {
    dynamictext cardTitle
    (
      content: 'Customers',
      rendermode: H4,
      class: 'card-title',
      designproperties: ('Spacing': ('margin-bottom': 'L'))
    )
  }
};
```

Reusable version — put the card **shell** in a fragment with a `slot`, then fill
the slot with each card's own content. This is the key idiom: one card wrapper,
arbitrary bodies, no copy-paste of the wrapper markup.

```mdl
mdl 1;
create fragment SectionCard as {
  container card1 (designproperties: ('Card style': on, 'Spacing': ('margin-bottom': 'Large'))) {
    container cardBody (class: 'card-body') {
      slot content            -- each page's widgets land here
    }
  }
};

create page MyModule.Dashboard
(
  title: 'Dashboard',
  layout: Atlas_Core.Atlas_Default
)
{
  container page1 (class: 'flex-column') {
    use fragment SectionCard {
      dynamictext custTitle (content: 'Customers', rendermode: H4, class: 'card-title')
      dynamictext custBody (content: 'Recent customer activity')
    }
    use fragment SectionCard {
      dynamictext ordTitle (content: 'Orders', rendermode: H4, class: 'card-title')
      datagrid ordGrid (datasource: database MyModule.Order) { }
    }
  }
};
```

The `slot` marker is resolved at expansion — `describe page` shows the fully
wrapped tree (`card1 > cardBody > custTitle, custBody`), and `mx check` is clean.
The slot name is optional (defaults to `content`); a fragment supports one slot.
Use `as prefix_` when the wrapper's *own* widget names would collide across uses
(the payload keeps the names you give it). For a fixed, content-invariant group
(a footer, a button pair) a plain slotless fragment is still the right tool.

**Binding data and behaviour (experimental).** A slot varies *what widgets* go
inside; typed **parameters** vary *which entity* and *which microflow*. Declare a
`datasource` and/or `action` parameter and the card becomes a real component:

```mdl
create fragment EntityCard($data: datasource, $onOpen: action) as {
  container card1 (designproperties: ('Card style': on)) {
    listview lv (datasource: $data) {
      slot content
      actionbutton open (caption: 'Open', action: $onOpen, buttonstyle: primary)
    }
  }
};
use fragment EntityCard ($data: database Sales.Order, $onOpen: microflow Sales.Open) {
  dynamictext cardTitle (content: 'Orders', rendermode: H4, class: 'card-title')
}
```

Atlas **building blocks** can't declare params, but `use building block` takes
rebind overrides that rewrite the block's outermost datasource / first button:

```mdl
use building block Atlas_Web_Content.List_Cards
  (datasource: database Sales.Order, action: call microflow Sales.Open) as orders_;
```

For a binding the override rule can't reach, copy the block in (`as prefix_`) and
`alter page … set datasource/action on prefix_widget`.

### Worked example — `Pageheader`

`describe building block Atlas_Web_Content.Pageheader`:

```
{
  container container1 (Class: 'pageheader', DesignProperties: ('Item gap': 'None')) {
    dynamictext text40 (Content: 'Page header title', RenderMode: H1, Class: 'pageheader-title')
    dynamictext text39 (Content: 'Supporting text', RenderMode: Paragraph, Class: 'pageheader-subtitle',
      DesignProperties: ('Color': 'Detail color', 'Spacing': ('margin-bottom': 'None')))
  }
}
```

Mirror:

```mdl
mdl 1;
create page MyModule.CustomersHeaderDemo
(
  title: 'Customers',
  layout: Atlas_Core.Atlas_Default
)
{
  container pageHeader (class: 'pageheader', designproperties: ('Item gap': 'None')) {
    dynamictext headerTitle (content: 'Customers', rendermode: H1, class: 'pageheader-title')
    dynamictext headerSubtitle
    (
      content: 'All active accounts',
      rendermode: Paragraph,
      class: 'pageheader-subtitle',
      designproperties: ('Color': 'Detail color', 'Spacing': ('margin-bottom': 'None'))
    )
  }
};
```

### Block → screen map (which block to reach for)

| You want | Mirror this block |
|---|---|
| A titled surface panel | `Card` / `Card_Action` (with a trailing action) / `Card_WithImage` |
| A page title + subtitle band | `Pageheader` (+ `_WithBack` / `_WithControls` / `_WithSearch`) |
| A big splash header | `Heroheader` (+ `_Background` / `_WithAction`) |
| A vertical / horizontal form | `Form_Vertical*` / `Form_Horizontal*` |
| A card/list feed | `List_Cards`, `List_WithImage`, `ListItem_*` |
| A master list + detail pane | `Master_Detail` |
| An activity/history feed | `Timeline` / `Timeline_WithImage` |
| A multi-step flow | `Wizard_Arrow` / `Wizard_Circle` (+ their `_Step`) |
| An inline notice | `Alert`, `AlertIcon` (+ `_WithAction`) |
| A path/breadcrumb trail | `Breadcrumb` / `Breadcrumb_Underline` |

---
## Atlas appearance vocabulary — classes & design properties

Atlas exposes its whole appearance system through the styling channels mxcli can
write today: raw `class:` strings and typed `designproperties:`. **Reach for these
before writing custom CSS.**

### The cheat-sheet

Apply via `class:` on any widget (space-join several: `class:'card flex-column'`).

| Concern | Atlas classes |
|---|---|
| **Cards** | `card`, `cards` (+ Card-style variants) — real CSS, `.card` is ~19 rules |
| **Backgrounds** | `background-{default,main,primary,secondary,success,warning,danger}` |
| **Buttons** | `btn-{primary,secondary,success,warning,danger}`, `btn-{lg,sm,bordered,block,icon-right,icon-top}` |
| **Flex / align** | `flex-{row,column,nowrap,items-grow,items-shrink}`, `align-x-{left,center,right,between,around,evenly}`, `align-y-*` |
| **Spacing utils** | `spacing-{outer,inner}-{top,right,bottom,left}` (+ `-medium` / `-large` / `-none` sizes) |
| **Borders / overflow** | `div-border-toggle-{all,top,…,none}`, `div-overflow-{auto,hidden,visible}` (+ border radius/color/style/width) |
| **Elevation** | `Shadow` toggle |
| **Data grids** | `datagrid-{bordered,hover,striped,lined,lg,sm}` |
| **Group boxes** | `groupbox-{primary,danger,secondary,callout}` |

Source: `atlas_core/web/design-properties.json` (verified in-project). To see what a
specific widget offers, run `list design properties` / `describe styling`
(`theme-styling`).

### When to reach for each

- **`card` / `Card style`** — any titled surface panel. This is the workhorse; a
  dashboard is mostly cards on a `background-main` page.
- **`background-primary` / `background-success` / …** — coloured section/hero/status
  surfaces. These resolve to your **retuned brand tokens** (Layer 1), so a hero band
  set to `background-primary` turns *your* brand colour automatically.
- **`btn-*`** — prefer `buttonstyle: primary` on `actionbutton` for the semantic
  style; add `btn-lg` / `btn-bordered` / `btn-block` as classes for size and shape.
- **`flex-row` / `flex-column` + `align-x-*` / `align-y-*`** — layout inside a
  container without a `layoutgrid`. `flex-row` + `align-x-between` is the standard
  "title on the left, action on the right" header row.
- **`spacing-inner-*` / `spacing-outer-*`** — padding/margin without inline `style:`.
- **`datagrid-*`** — reach for these on data grids before overriding grid CSS.
- **`groupbox-*`** — callouts / grouped sections with a semantic tint.

### Typed design properties — the alternative channel

Atlas building blocks use **both** channels side by side. The typed channel is what
Studio Pro's Appearance tab reads, so mirror it when you want the block to round-trip
cleanly into Studio Pro. Common mappings:

| Class-style | Typed design-property equivalent |
|---|---|
| `class:'card'` | `designproperties: ('Card style': on)` |
| `class:'background-primary'` | `designproperties: ('Background color': 'Brand Primary')` |
| `class:'flex-column'` | `designproperties: ('Flex container': 'Vertical (column)')` |
| `class:'flex-row'` | `designproperties: ('Flex container': 'Horizontal (row)')` |
| `class:'align-x-center'` | `designproperties: ('Align items X': 'Center')` |
| `class:'Shadow'` | `designproperties: ('Shadow': 'None' / 'Small' / …)` |
| spacing utilities | `designproperties: ('Spacing': ('margin-bottom': 'L', 'padding-top': 'S'))` |

**Both channels render identically at runtime** — raw `class:` is sufficient for the
visual result today. The typed channel matters for Studio Pro round-trip and is the
more idiomatic form to mirror from a `describe building block`. Notes:
- Design-property **keys are case-sensitive** — match the `describe` output exactly.
- Compound properties (Spacing, Border) take a **nested list**:
  `['Spacing': ['margin-top': 'Large', 'margin-bottom': 'None']]`.
- A block's own stored inline styles (a `dynamictext` included) are placed as
  stored. For your own changes prefer `class:` over inline `style:` (`theme-styling`).
- A block's list view whose data source is not configured is placed unconfigured
  (`exec` warns); `mx check` then reports CE0488 until you set one — bind it with
  `use building block … (datasource: database from Module.Entity)`.

---
