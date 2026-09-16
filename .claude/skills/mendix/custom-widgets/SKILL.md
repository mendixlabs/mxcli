---
name: custom-widgets
description: "MDL syntax for pluggable widgets in CREATE PAGE / ALTER PAGE — any installed widget is named by its own name (`htmlelement frame (…) { … }`), with object lists and child slots read from its definition. Covers GALLERY, COMBOBOX, DataGrid2, charts and third-party widgets: datasource and column forms, child slots (TEMPLATE/FILTER), the `pluggablewidget '<id>'` fallback, and adding a widget via .def.json. Use when placing a pluggable widget on a page, or when `mxcli widget describe` output needs interpreting. For the widgets THIS project has, read the generated `widgets` skill."
---

# Custom & Pluggable Widgets in MDL

## Any installed widget is named by its own name

If a widget is installed in `widgets/`, MDL names it directly — no keyword list,
no widget id:

```sql
htmlelement frame (tagName: 'div', tagContentMode: 'container') {
  attribute a1 (attributeName: 'data-testid', attributeValueType: 'expression')
  tagcontentcontainer body {
    dynamictext caption (Content: 'Inside the element')
  }
}
```

Three things there are read from the widget's definition, not from anything
hardcoded: the **keyword** (`htmlelement`, the last segment of the widget id),
the **properties** (the widget's own spelling — `tagName`, not `TagName`), and
the **body containers** — `attribute` is an object list (one entry per
repetition), `tagcontentcontainer` a child slot (holds widgets).

**Ask the widget rather than guessing.** `describe widget <name>` lists every
property with its type, default and enumeration members; every body container
and whether MDL can express it; and a complete example that parses AND checks as
written:

```bash
mxcli widget describe htmlelement -p app.mpr
```

Do this first when placing an unfamiliar widget. It is faster than reading this
file and it cannot go stale, because it reads the `.mpk` the project actually
has.

### The id form is the fallback

```sql
pluggablewidget 'com.mendix.widget.web.htmlelement.HTMLElement' frame (tagName: 'div')
```

Use it only when two installed packages ship the same MDL name, or when you have
the id and not the name. Everything below that still shows the id form works
unchanged — the short form is simply the better default.

### Repeated entries are BLOCKS, never a property value

A widget's repeatable property — FileUploader `allowedFileFormats`, HTML Element
`attributes`, a chart's `series` — is written as container blocks in the body:

```sql
htmlelement frame ( tagName: 'div' ) {
  attribute a1 (attributeName: 'data-testid', attributeValueType: 'expression')
}
```

**Not** as a property value:

```sql
htmlelement frame ( attributes: [(attributeName: 'data-testid')] )   -- MDL-WIDGET27
```

That form is an error (`mendixlabs/mxcli#999`). It used to be worse than an
error: the single-key shape checked clean, exec'd successfully and the property
vanished from storage, while the multi-key shape died as `missing ')' at ','`.
The error now names the container keyword and rewrites your entry into the form
that works.

The same rule covers the two spellings that carry no entry to key on
(`mendixlabs/mxcli#1056`):

```sql
selectionhelper sh (renderStyle: 'custom', customAllSelected: [])          -- MDL-WIDGET27
selectionhelper sh (renderStyle: 'custom', customAllSelected: 'something') -- MDL-WIDGET27
```

A **widgets**-typed property such as `customAllSelected` holds child widgets, so
it is written as a block with widgets in it rather than entries:

```sql
selectionhelper sh (renderStyle: 'custom') {
  customallselected s1 { dynamictext d1 (Content: 'All') }
}
```

The empty form is reported from its shape, with no project needed. The scalar
form is reported only when the widget resolves, because without a definition
`p: 'x'` is the ordinary property form and flagging it would be a guess. Both
matter because a required slot left empty is not a silent no-op at build time —
it is `CE0642 "Property '…' is required."`, one per slot.

`describe widget <name> -p <project.mpr>` lists a widget's container keywords
under **Body containers**, and — for an object list — the widgets-typed **slots
inside one item**, with the widget types that route into each:

```
column        object list  -> columns  authorable
                items: showContentAs, attribute, dynamicText, …
                slot content -> content: any other widget in the item body
                slot filter  -> filter: textfilter | numberfilter | datefilter | dropdownfilter
```

Read that last line before guessing where something goes. It says a Data Grid 2
column filter is written directly in the **column's** braces — not in
`controlbar`, which is the grid-wide filter bar and renders "Unable to get
filter store" if you put a column filter there.

### When the name is not found

A name resolving to no installed definition is an **error** (MDL-WIDGET25, with
near-miss suggestions), and a container the parent does not declare is
MDL-WIDGET26. Both need `-p`: without a project, mxcli knows only its embedded
widgets, so it stays quiet rather than reporting every real widget as unknown.

MDL-WIDGET29 needs no project: `statictext` writes `Forms$Text`, a type Mendix
does not have, and the project that comes out cannot be *loaded* at all (`mx
check` and Studio Pro both stop at `TypeCacheUnknownTypeException` before
validation). Use `dynamictext` with a literal `Content:`.
If a widget you have installed is not found, extract its definition:

```bash
mxcli widget init -p app.mpr
```

## Built-in Pluggable Widgets

### GALLERY

Card-layout list with optional template content and filters.

```sql
gallery galleryName (
  datasource: database from Module.Entity sort by Name asc,
  selection: single | multiple | none,
  DesktopColumns: 3,
  TabletColumns: 2,
  PhoneColumns: 1
) {
  template template1 {
    dynamictext title (content: '{1}', contentparams: [{1} = Name], rendermode: H4)
    dynamictext info  (content: '{1}', contentparams: [{1} = Email])
  }
  filter filter1 {
    textfilter   searchName  (attribute: Name)
    numberfilter searchScore (attribute: Score)
    dropdownfilter searchStatus (attribute: status)
    datefilter   searchDate  (attribute: CreatedAt)
  }
}
```

- `template` block -> mapped to `content` property (child widgets rendered per row)
- `filter` block -> mapped to `filtersPlaceholder` property (shown above list)
- `selection: none` omits the selection property (default if omitted)
- `DesktopColumns`, `TabletColumns`, `PhoneColumns` control responsive grid columns (default: 1 each, omit if default)
- Children written directly under GALLERY (no container) go to the first slot with `mdlContainer: "template"`

### COMBOBOX

Two modes depending on the attribute type:

```sql
-- Enumeration mode (Attribute is an enum)
combobox cbStatus (label: 'Status', attribute: status)

-- Association mode (Attribute is an association)
combobox cmbCustomer (
  label: 'Customer',
  attribute: Order_Customer,
  datasource: database Module.Customer,
  CaptionAttribute: Name
)
```

- Engine detects association mode when `datasource` is present (`hasDataSource` condition)
- `CaptionAttribute` is the display attribute on the **target** entity
- In association mode, mapping order matters: DataSource must resolve before Association (sets entityContext)

## Charts (Mendix Charts.mpk)

Charts are pluggable widgets. Install `Charts.mpk` into the project's `widgets/`
folder first (any Charts-based app has it); `exec` auto-generates the
`.def.json`.

Each is authorable by its **own name** — `barchart`, `linechart`, `piechart`,
`heatmap` — and the examples below use the package id form, which also still
works. The id column is kept because it is what `describe widget` prints and
what identifies the widget unambiguously.

**Chart type → widget id → data container:**

| Chart | Widget id (`pluggablewidget '…'`) | Data block |
|-------|-----------------------------------|------------|
| Bar / Column / Area | `com.mendix.widget.web.{barchart.BarChart, columnchart.ColumnChart, areachart.AreaChart}` | `series` (one or more) |
| Line / TimeSeries / Bubble | `com.mendix.widget.web.{linechart.LineChart, timeseries.TimeSeries, bubblechart.BubbleChart}` | `line` (one or more) |
| HeatMap | `com.mendix.widget.web.heatmap.HeatMap` | widget-level attrs **+** `scalecolor` items |
| Pie | `com.mendix.widget.web.piechart.PieChart` | widget-level attrs (no object-list) |

**Series / line — each binds its OWN datasource + X/Y:**

```
pluggablewidget 'com.mendix.widget.web.barchart.BarChart' chart1 {
  series s1 (
    dataSet: 'static',
    DataSource: database from MyModule.SalesByRegion,  -- an OQL VIEW (aggregated)
    staticXAttribute: Region,      -- resolves against the series' own datasource
    staticYAttribute: Total,
    staticName: 'Revenue',
    interpolation: 'linear'        -- line/area only: linear | spline
  )
}
```

A series datasource takes any of the usual kinds — `database from …`,
`microflow …`, `nanoflow …`, `$Param`, `selection …` — not just `database`.
(Before #941 `describe page` rendered every series datasource as `database
from`, so a microflow-backed series described back as a missing entity.)

**Pie / HeatMap bind at the WIDGET level** (no series block). Both need `DataSource:`
+ `ValueAttribute:`; Pie also needs a required `SeriesName:`; HeatMap adds `scalecolor` items:

```
pluggablewidget 'com.mendix.widget.web.piechart.PieChart' pie1 (
  DataSource: database from MyModule.SalesByRegion,
  ValueAttribute: Total,
  seriesName: 'Sales by Region'   -- REQUIRED (CE4899 without it)
)

pluggablewidget 'com.mendix.widget.web.heatmap.HeatMap' heat1 (
  DataSource: database from MyModule.SalesByRegion,
  ValueAttribute: Total           -- REQUIRED (CE0642 without it)
) {
  scalecolor scLow  (valuePercentage: 0,   colorValue: '#f7fbff')
  scalecolor scHigh (valuePercentage: 100, colorValue: '#08306b')
}
```

**Per-chart required-property gotchas (all are mxbuild errors, not `check` errors):**

- **TimeSeries** — `StaticXAttribute` MUST be a **Date and time** attribute (CE7247 otherwise). Feed it a view with a datetime column.
- **BubbleChart** — the `line` needs a `StaticSizeAttribute:` (a numeric) in addition to X/Y.
- **PieChart** — `SeriesName:` is required (CE4899); `ValueAttribute:` is required (CE0642).
- **HeatMap** — `ValueAttribute:` is required (CE0642).

**Data feed = OQL view entities.** Charts want *aggregated* data (one row per
category). Build a `create view entity … as select … group by …` and point the
chart's `DataSource:` at it. **Never name a view column after an OQL keyword**
(`Quarter`/`Month`/`Year`/`Day` → CE0174); use `Period` etc. (`check` warns —
MDL032).

**CE0463 "update this widget" is EXPECTED after generating charts.** mxcli writes
the WidgetType from an embedded 11.6 baseline; the installed Charts.mpk is a
different version, so Studio Pro/mxbuild flags drift. Clear it with **`mxcli docker
check`/`build`** (they normalize the widgets and preserve your storage format). The
whole `mdl-examples/doctype-tests/34-chart-widget-examples.mdl` builds **0 errors**
after normalization.
**Do NOT run bare `mx update-widgets` on an MPRv2 project** (an `mprcontents/`-folder
project — what `mxcli new` creates): it converts the project to single-file v1 and
**deletes `mprcontents/`**, corrupting git, breaking a running `mxcli run --local`
loop, and sometimes making the project unopenable in Studio Pro. `mxcli docker
check`/`build` snapshot/restore the v2 files around the normalization; raw
`mx update-widgets` is only safe on a v1 project or a throwaway diagnostic copy.

**DESCRIBE round-trips** series/line/scalecolor object-lists (item names are
synthesized, e.g. `series1`); a Pie/HeatMap's widget-level `SeriesName`/datasource
are not yet reconstructed.

## Adding a Third-Party Widget

### Step 1 -- Extract .def.json from .mpk

```bash
mxcli widget extract --mpk widgets/MyWidget.mpk
# Output: .mxcli/widgets/mywidget.def.json

# Override MDL keyword
mxcli widget extract --mpk widgets/MyWidget.mpk --mdl-name MYWIDGET
```

The `extract` command parses the .mpk (ZIP archive containing `package.xml` + widget XML) and auto-infers operations from XML property types:

| XML Type | Operation | MDL Source Key |
|----------|-----------|----------------|
| attribute | attribute | `attribute` |
| association | association | `association` |
| datasource | datasource | `datasource` |
| selection | selection | `selection` |
| widgets | widgets (child slot) | container name (key uppercased) |
| boolean/string/enumeration/integer/decimal | primitive | hardcoded `value` from defaultValue |
| textTemplate | texttemplate | `TextTemplate` |
| action | action | `OnClick` / `OnChange`, else the property's own key |
| expression/object/icon/image/file | *skipped* | too complex for auto-mapping |

Skipped types require manual configuration in the .def.json.

**Action slots are matched by name, and the storage key is not the MDL name.**
Mendix's own widgets suffix theirs — a BadgeButton's click slot is `onClickEvent`,
a HeatMap's is `onClickAction`, a Combobox's change slot is `onChangeEvent` —
so `actionSourceForKey` strips one `Event`/`Action` suffix before matching
`onclick`/`onchange`. That is what lets `onClick:` and `OnChange:` reach those
widgets at all.

**Every other action slot is authored by the widget's own key** — a *named slot*:

```sql
FILEUPLOADER fu (
  createFileAction:    microflow MyModule.ACT_CreateFile,
  onUploadSuccessFile: microflow MyModule.ACT_AfterUpload
)
```

In the .def.json a named slot is a mapping with **no `source`**, the same shape
object-list item mappings use:

```json
{"propertyKey": "createFileAction", "operation": "action"}
```

`microflow`/`nanoflow` on a named slot parse as a *data source* — those forms
overlap with `dataSourceExprV3` and the datasource alternative has to win, or a
chart series' `staticDataSource: microflow M.X` would become an action. The
executor converts them, because the widget definition is the only layer that
knows the slot is action-typed. Every other action form (`show_page`,
`save_changes`, …) reaches the AST as an action directly.

**A slot may be conditional, and writing into a pruned one is CE0463.** DataGrid 2's
`onSelectionChange` is *hidden when `itemSelection` = None*, so it needs
`Selection: Multiple` (or `Single`) alongside it. `mxcli check` refuses the
statement with **MDL-WIDGET10** rather than letting the build fail. `mxcli widget
describe <name>` lists each slot's `hidden when` condition.

Object-list *item* action slots (chart series `staticOnClickAction`, popupmenu
item `action`) have mappings generated but the engine still skips them at apply
time. See upstream #956.

**Widgets with multiple datasources use each datasource property's own key.**
The friendly `DataSource:` spelling is for a widget with one logical source.
When a widget exposes independent lists, name them exactly as `describe widget`
does:

```sql
multisource dashboard (
  primarySource: microflow Dashboard.DS_PrimaryRows,
  primaryLabelAttribute: Label,

  secondarySource: microflow Dashboard.DS_SecondaryRows,
  secondaryDateAttribute: OccurredAt,

  summarySource: database from Dashboard.SummaryRow,
  summaryValueAttribute: Total
)
```

Each named property accepts the normal datasource expressions (`database`,
`microflow`, `nanoflow`, page parameter, selection, or association). Attribute
mappings resolve against the matching source's entity context; in `.def.json`,
put each datasource mapping before the attribute mappings that depend on it.
Do not replace the named properties with one generic `DataSource:`: that loses
which entity owns each attribute. `DESCRIBE PAGE` preserves the named form
whenever a widget has more than one populated datasource.

### Step 2 -- Extract BSON template from Studio Pro

The .def.json only describes mapping rules. The engine also needs a **template JSON** with the complete Type + Object BSON structure.

```bash
# 1. in Studio Pro: drag the widget onto a test page, save the project
# 2. Extract the widget's BSON:
mxcli bson dump -p App.mpr --type page --object "Module.TestPage" --format json
# 3. Extract the type and object fields from the customwidget, save as:
```

Place at: `project/.mxcli/widgets/mywidget.json`

Template JSON format:

```json
{
  "widgetId": "com.vendor.widget.MyWidget",
  "name": "My widget",
  "version": "1.0.0",
  "extractedFrom": "TestModule.TestPage",
  "type": {
    "$ID": "aa000000000000000000000000000001",
    "$type": "CustomWidgets$CustomWidgetType",
    "WidgetId": "com.vendor.widget.MyWidget",
    "PropertyTypes": [
      {
        "$ID": "aa000000000000000000000000000010",
        "$type": "CustomWidgets$WidgetPropertyType",
        "PropertyKey": "datasource",
        "ValueType": { "$ID": "...", "type": "datasource" }
      }
    ]
  },
  "object": {
    "$ID": "aa000000000000000000000000000100",
    "$type": "CustomWidgets$WidgetObject",
    "TypePointer": "aa000000000000000000000000000001",
    "properties": [
      2,
      {
        "$ID": "...",
        "$type": "CustomWidgets$WidgetProperty",
        "TypePointer": "aa000000000000000000000000000010",
        "value": {
          "$type": "CustomWidgets$WidgetValue",
          "datasource": null,
          "AttributeRef": null,
          "PrimitiveValue": "",
          "widgets": [2],
          "selection": "none"
        }
      }
    ]
  }
}
```

**CRITICAL**: Template must include both `type` (PropertyTypes schema) and `object` (default WidgetObject with all property values). Extract from a real Studio Pro MPR -- do NOT generate programmatically. Mismatched structure causes CE0463.

### Step 3 -- Place files

```
project/.mxcli/widgets/mywidget.def.json   <- project scope (highest priority)
project/.mxcli/widgets/mywidget.json       <- template json (same directory)
~/.mxcli/widgets/mywidget.def.json         <- global scope
```

Set `"templateFile": "mywidget.json"` in the .def.json. Project definitions override global ones; global overrides embedded.

### Step 4 -- Use in MDL

```sql
MYWIDGET myWidget1 (datasource: database Module.Entity, attribute: Name) {
  template content1 {
    dynamictext label1 (content: '{1}', contentparams: [{1}=Name])
  }
}
```

## Authoring over MCP (live Studio Pro)

When mxcli runs with `--mcp` (writes routed to a running Studio Pro), pluggable
widgets take a different, simpler path than the MPR writer:

- **No BSON template needed** -- skip Step 2 entirely. Only the `.def.json`
  (Step 1) is required. Studio Pro owns serialization over `pg_patch_page` and
  expands every default, so the CE0463 template-mismatch class does not exist
  on this path.
- **Any registry-resolved widget is accepted** -- same 3-tier resolution
  (project `.mxcli/widgets/` -> global -> embedded). There is no separate MCP
  whitelist.
- **Supported property operations**: attribute, association, primitive,
  selection, datasource, widgets (child slots), object lists, expression,
  texttemplate (including `{AttrName}` placeholders -> template parameters),
  and action (`microflow Module.Flow`, `show_page Module.Page`, or none).
- **Rejected loudly** (widget refused, nothing sent): actions *with argument
  mappings*, other action kinds (save/cancel/close/delete/create/open-link/
  nanoflow), and any operation the MCP builder does not translate. The error
  names each unsupported property.
- **Selector-primitive pruning gotcha**: Studio Pro prunes properties made
  irrelevant by a mode-selector primitive's default. Example: the Image widget
  drops `imageUrl` unless `ImageType: 'imageUrl'` is also set. If a property
  you set does not appear in Studio Pro, check the widget's mode selector.

## .def.json Reference

```json
{
  "widgetId":        "com.vendor.widget.web.mywidget.MyWidget",
  "mdlName":         "MYWIDGET",
  "templateFile":    "mywidget.json",
  "defaultEditable": "Always",
  "propertyMappings": [
    {"propertyKey": "datasource",  "source": "datasource", "operation": "datasource"},
    {"propertyKey": "attribute",   "source": "attribute",  "operation": "attribute"},
    {"propertyKey": "someFlag",    "value":  "true",       "operation": "primitive"}
  ],
  "childSlots": [
    {"propertyKey": "content", "mdlContainer": "template", "operation": "widgets"}
  ],
  "modes": [
    {
      "name": "association",
      "condition": "hasDataSource",
      "propertyMappings": [
        {"propertyKey": "optionsSource", "value": "association", "operation": "primitive"},
        {"propertyKey": "assocDS",       "source": "datasource",  "operation": "datasource"},
        {"propertyKey": "assoc",         "source": "association", "operation": "association"}
      ]
    },
    {
      "name": "default",
      "propertyMappings": [
        {"propertyKey": "attr", "source": "attribute", "operation": "attribute"}
      ]
    }
  ]
}
```

### Mode Conditions

| Condition | Checks |
|-----------|--------|
| `hasDataSource` | AST widget has a `datasource` property |
| `hasAttribute` | AST widget has an `attribute` property |
| `hasProp:XYZ` | AST widget has a property named `XYZ` |

Modes are evaluated in definition order -- first match wins. A mode with no `condition` is the default fallback.

### 6 Built-in Operations

| Operation | What it does | Typical Source |
|-----------|-------------|----------------|
| `attribute` | Sets `Value.AttributeRef` on a WidgetProperty | `attribute` |
| `association` | Sets `Value.AttributeRef` + `Value.EntityRef` | `association` |
| `primitive` | Sets `Value.PrimitiveValue` | static `value` or property name |
| `datasource` | Sets `Value.DataSource` (serialized BSON) | `datasource` |
| `selection` | Sets `Value.Selection` (mode string) | `selection` |
| `widgets` | Replaces `Value.Widgets` array with child widget BSON | child slot |
| `texttemplate` | Sets text in `Value.TextTemplate` (Forms$ClientTemplate) | property name (resolved as string) |
| `action` | Sets `Value.Action` with serialized client action BSON | `onclick` (resolved from AST Action) |

### Mapping Order Constraints

- **`association` source must come AFTER `datasource` source** in the mappings array. The association operation depends on `entityContext` set by a prior DataSource mapping. The registry validates this at load time.
- **`value` takes priority over `source`**: if both are set, the static `value` is used.

### Source Resolution

| Source | Resolution logic |
|--------|-----------------|
| `attribute` | `w.GetAttribute()` -> `pageBuilder.resolveAttributePath()` |
| `datasource` | Named datasource property matching the mapping key/alias, otherwise `w.GetDataSource()` -> `pageBuilder.buildDataSourceV3()` -> also updates `entityContext` |
| `association` | `w.GetAttribute()` -> `pageBuilder.resolveAssociationPath()` + uses current `entityContext` |
| `selection` | `w.GetSelection()` or `mapping.Default` fallback |
| `CaptionAttribute` | `w.GetStringProp("CaptionAttribute")` -> auto-prefixed with `entityContext` if relative |
| *(other)* | Treated as generic property name: `w.GetStringProp(source)` |

## Engine Internals

### Build Pipeline

When `buildWidgetV3()` encounters an unrecognized widget type:

```
1. Registry lookup: widgetRegistry.Get("MYWIDGET") -> WidgetDefinition
2. template loading: GetTemplateFullBSON(widgetID, idGenerator, projectPath)
   a. Load json from embed.FS (or .mxcli/widgets/)
   b. Augment from project's .mpk (if newer version available)
   c. Phase 1: Collect all $ID values -> generate new UUID mapping
   d. Phase 2: Convert type json -> BSON, extract PropertyTypeIDMap
   e. Phase 3: Convert object json -> BSON (TypePointer remapped via same mapping)
   f. placeholder leak check (aa000000-prefix IDs must all be remapped)
3. Mode selection: evaluateCondition() on each mode in order -> first match wins
4. Property mappings: for each mapping, resolveMapping() -> OperationFunc()
   Each operation locates the WidgetProperty by matching TypePointer against PropertyTypeIDMap
5. Child slots: group AST children by container name, build to BSON, embed via opWidgets
6. Assemble customwidget{RawType, RawObject, PropertyTypeIDMap, ObjectTypeID}
```

### PropertyTypeIDMap

The map links PropertyKey names (from .def.json) to their BSON IDs:

```
PropertyTypeIDMap["datasource"] = {
  PropertyTypeID: "a1b2c3d4...",   // $ID of WidgetPropertyType in type
  ValueTypeID:    "e5f6a7b8...",   // $ID of ValueType within PropertyType
  DefaultValue:   "",
  ValueType:      "datasource",    // type string
  ObjectTypeID:   "...",           // for nested object list properties
}
```

Operations use this map to locate the correct WidgetProperty in the Object's Properties array by comparing `TypePointer` (binary GUID) against `PropertyTypeID`.

### MPK Augmentation

At template load time, `augmentFromMPK()` checks if the project has a newer `.mpk` for the widget:

```
project/widgets/*.mpk -> FindMPK(projectDir, widgetID) -> ParseMPK()
-> AugmentTemplate(clone, mpkDef)
   -> add missing properties from newer .mpk version
   -> remove stale properties no longer in .mpk
```

This reduces CE0463 errors from widget version drift without requiring manual template re-extraction.

### 3-Tier Registry

| Priority | Location | Scope |
|----------|----------|-------|
| 1 (highest) | `<project>/.mxcli/widgets/*.def.json` | Project |
| 2 | `~/.mxcli/widgets/*.def.json` | Global (user) |
| 3 (lowest) | `sdk/widgets/definitions/*.def.json` (embedded) | Built-in |

Higher priority definitions override lower ones with the same MDL name (case-insensitive).

## Verify & Debug

```bash
# list registered widgets
mxcli widget list -p App.mpr

# check after creating a page
mxcli check script.mdl -p App.mpr --references

# full mx check (catches CE0463)
mxcli docker check -p App.mpr

# debug CE0463 -- compare NDSL dumps
mxcli bson dump -p App.mpr --type page --object "Module.PageName" --format ndsl
```

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| CE0463 after page creation | Template version mismatch -- extract fresh template from Studio Pro MPR, or ensure .mpk augmentation picks up new properties |
| Widget not recognized | Check `mxcli widget list`; .def.json must be in `.mxcli/widgets/` with `.def.json` extension |
| TEMPLATE content missing | Widget needs `childSlots` entry with `"mdlContainer": "template"` |
| Association COMBOBOX shows enum behavior | Add `datasource` to trigger association mode (`hasDataSource` condition) |
| Association mapping fails | Ensure DataSource mapping appears **before** Association mapping in the array |
| Custom widget not found | Place .def.json in `.mxcli/widgets/` inside the project directory |
| Placeholder ID leak error | Template JSON has unreferenced `$ID` values starting with `aa000000` -- ensure all IDs are in the `collectIDs` traversal path |

## Key Source Files

| File | Purpose |
|------|---------|
| `mdl/executor/widget_engine.go` | PluggableWidgetEngine, 6 operations, Build() pipeline |
| `mdl/executor/widget_registry.go` | 3-tier WidgetRegistry, definition validation |
| `sdk/widgets/loader.go` | Template loading, ID remapping, MPK augmentation |
| `sdk/widgets/mpk/mpk.go` | .mpk ZIP parsing, XML property extraction |
| `cmd/mxcli/cmd_widget.go` | `mxcli widget extract/list` CLI commands |
| `sdk/widgets/definitions/*.def.json` | Built-in widget definitions (ComboBox, Gallery) |
| `sdk/widgets/templates/mendix-11.6/*.json` | Embedded BSON templates |
| `mdl/executor/cmd_pages_builder_input.go` | `updateWidgetPropertyValue()` -- TypePointer matching |
