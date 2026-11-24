# Pluggable Widgets Across Mendix Versions

Modern Mendix UI is built from **pluggable widgets** — DataGrid 2, Combo box, Gallery,
the data-grid filters, and any widget you install from the Marketplace. mxcli can create
and configure these widgets in a page, and it does so in a way that stays correct as the
installed widget versions change from project to project. This guide explains how that
works and how to inspect what mxcli has discovered about a widget.

## The problem: "the definition of this widget has changed" (CE0463)

Every pluggable-widget instance stored in an `.mpr` embeds a copy of the widget's
**definition** — the full list of properties, their types, default values, captions,
categories, and declared order. When you open the app, Studio Pro (and `mx check`)
compares that embedded definition against the widget package (`.mpk`) actually installed
in the project. If they differ in any way, you get:

> **CE0463** — "The definition of this widget has changed. Update this widget by
> right-clicking it and selecting 'Update widget'…"

Different Mendix versions — and different Marketplace releases of the same widget — ship
slightly different definitions: a property is added, an enumeration option is renamed, a
category moves, the property order changes. A tool that emitted one fixed definition would
trip CE0463 the moment the project's widget version didn't match.

## How mxcli stays version-correct

mxcli never hard-codes one widget shape. It reconciles a known-good template against the
widget package **installed in your project**, so the definition it writes matches the
version you actually have at the moment you create the widget.

1. **Embedded template.** mxcli ships a known-good template for each built-in widget
   (extracted from Studio Pro). This provides the correct nested BSON structure that is
   hard to build from scratch. A widget with **no** embedded template — anything from
   the Marketplace, and the Charts family — is instead generated whole from the
   package (`GenerateFromMPK`), so an embedded template is not a prerequisite for
   support.

2. **Reconcile against the project `.mpk`.** When you pass a project (`-p`), mxcli finds
   the widget's `.mpk` in the project's `widgets/` folder, parses its definition, and
   updates the template to match: it adds and removes properties, rewrites enumeration
   option sets, fixes categories/captions/defaults, fills in per-property metadata, and —
   importantly — **re-orders the properties to the package's declared order**, including
   the system properties (Label / Visibility / Editability) at their real position. This
   is exactly what Studio Pro's own "Update widget" does, derived generically from the
   package with no widget-specific code.

3. **Apply the widget's own dynamic rules.** Which properties a widget shows depends on
   its *configuration* — a Combo box in enumeration mode hides the association properties;
   a DataGrid 2 with selection off hides the selection labels. That logic lives in each
   widget's compiled `editorConfig.js`. mxcli **runs** that file's `getProperties` the
   way Studio Pro does — in an embedded JavaScript interpreter, for the widget's actual
   settings — and applies the answer, so a
   freshly-created widget carries exactly the defaults Studio Pro would give it. The
   script has no access to files, the network or processes, and a call that does not
   return within a second is stopped. A property hidden only for one `target` (`"web"`
   or `"desktop"`, as TreeNode's `advancedMode` is) does not count as hidden: mxbuild
   enforces neither branch, so only what both hide must hold its default.

   A property the widget hides must hold its default, or the build fails with CE0463.
   So a hidden property you leave out is written at its default; one you set to its
   default draws an MDL-WIDGET10 warning (the value is ignored); and one you set to
   anything else is an MDL-WIDGET10 **error**, which `exec` refuses before writing.
   `describe` leaves out a hidden property that holds its default. For example, a
   Timeline grouped by `'day'` hides `groupByMonthOptions`, and one with
   `customVisualization: true` hides both format enumerations.

   This covers the widget's own properties, whatever JavaScript the widget uses to hide
   them. The properties of an object-list **item** (a chart series, a Data grid column)
   still go by the rules mxcli can read from the source: `mxcli widget describe <name>
   -p app.mpr` lists them and ends with "*N* of *M* editor hide-rules recognized as
   rules". A value set behind an item rule mxcli cannot read reaches `mx check`
   unwarned. Without the widget's `.mpk` in the project (a widget described from
   mxcli's embedded templates), only the read rules apply.

The result: widgets created by mxcli open cleanly across Mendix 10.x and 11.x with the
widget packages bundled with those releases — verified on Mendix 11.12 with Data
Widgets 3.4: a project authored entirely by mxcli reports **0** CE0463.

### Two things this does *not* cover

**Upgrading a widget package does not update widgets you already created.** A package
that drops a property leaves every *stored* instance carrying a property the new
definition no longer has — which is what CE0463 reports, and what its message
("Update all widgets") asks you to fix. This is normal Mendix behaviour and it is not
specific to mxcli: Studio Pro's own widgets are flagged identically. Measured on
Mendix 11.12, upgrading Data Widgets 3.4 → 3.11.3:

| Project | as authored | after the upgrade | after `mx update-widgets` |
|---|---|---|---|
| a real mxcli-built app | 0 errors | 36 CE0463 (7 mxcli-authored, 29 Studio Pro's own) | 0 errors |

Apply that update with `mxcli fix widgets -p app.mpr`: it runs Mendix's own `mx
update-widgets` and writes the result back in the MPR v2 layout (see
[Repairing the model](marketplace.md#repairing-the-model-mxcli-fix)). Do not run
bare `mx update-widgets` on an MPR v2 project — it collapses `mprcontents/` back to
the single-file v1 layout. Studio Pro's "Update all widgets" does the same job.
`mxcli widget sync` reconciles schemas in mxcli's own code and is partial today (see
[below](#reconciling-stored-widgets-after-a-package-upgrade-mxcli-widget-sync)).

**Re-running a page's description does not upgrade its widgets either.** A
`create or modify` (or `create or replace`) of a stored page, snippet or layout
keeps each pluggable widget as Studio Pro stored it, schema included — as Studio
Pro itself does. A widget the statement leaves unchanged is kept whole; one it
changes keeps every property the statement does not change, so editing a column
caption leaves the other columns, the row click and the grid's data source as
they were. (A widget whose data context the statement changes is rebuilt from
mxcli's template instead, since its stored bindings no longer apply.) So a widget
stays at its stored package version until you update it with `mxcli fix widgets`.

This changed during the beta. Earlier builds rebuilt every pluggable widget of a
rewritten page from the installed `.mpk`, so re-running `describe` output also
upgraded it — and usually cleared its CE0463 — while silently resetting whatever the
statement could not say (a specialized column binding, a row click). If a script
relied on that side effect to clear CE0463, run `mxcli fix widgets` instead.
`mxcli docker check` checks the project as stored and labels each CE0463 a widget
resync would clear as a *stale widget definition*, so it shows which ones
`mxcli fix widgets` fixes (`--update-widgets` checks a resynced copy only, and
hides them).

**Two widgets do not yet author cleanly against Data Widgets 3.10+.** Freshly created
`gallery` and `dropdownfilter` widgets produce CE0463 on those packages; `datagrid2`,
`textfilter`, `datefilter` and `numberfilter` are clean. Tracked as
[mendixlabs/mxcli#716](https://github.com/mendixlabs/mxcli/issues/716). On the
bundled 3.4 package all of them are clean.

> For the internal mechanics (template extraction, BSON cross-references, the augment
> pipeline), see [Widget Template System](../internals/widget-templates.md) and
> [Pluggable Widget Engine](../internals/widget-engine.md).

## Inspecting the discovered widget format

Two commands let you see exactly what mxcli knows about the widgets available to a project.

### List available widgets

```bash
mxcli widget list                 # built-in widget definitions
mxcli widget list -p app.mpr      # also loads project-specific definitions
```

### Describe one widget

`mxcli widget describe` shows a widget's **expected properties** and its **dynamic property
rules**. Name the widget by its MDL keyword (`COMBOBOX`, `DATAGRID2`, `GALLERY`, …) or its
full widget id.

```bash
# mxcli's built-in view of the widget:
mxcli widget describe COMBOBOX

# the version-accurate view from the package installed in the project:
mxcli widget describe COMBOBOX -p app.mpr

# machine-readable:
mxcli widget describe DATAGRID2 -p app.mpr --format json
```

With `-p`, the properties and rules come from the widget package actually installed in the
project (`widgets/*.mpk`) — the version-accurate "discovered" format, and the only way to
inspect a Marketplace widget mxcli has no built-in knowledge of. Without `-p`, they come
from mxcli's embedded template.

Example (abridged):

```text
Widget: Combo box (combobox)
  ID:      com.mendix.widget.web.combobox.Combobox
  Version: 2.5.0
  Kind:    pluggable
  Source:  project .mpk

Properties (58):
  source                 enumeration required default=context {context|database|static}  (General::Data source)
  optionsSourceType      enumeration required default=association {association|enumeration|boolean}  (General::Data source)
  attributeEnumeration   attribute required  (General::Data source)
  …
  selectAllButtonCaption textTemplate required  (General::Multiple-selection (reference set))
  Label                  system  [system]
  Visibility             system  [system]
  Editability            system  [system]
  customEditability      enumeration required default=default {default|never|conditionally}  (General::Editability)
  …

Dynamic property rules (10):
  itemSelectionMethod       hidden when itemSelection = "None"
  keepSelection             hidden when itemSelection ≠ "Multi"
  loadMoreButtonCaption     hidden when pagination ≠ "loadMore"
  …
  — 7 of 23 editor hide-rules recognized
```

The property list is in the widget's **declared order** (system properties appear where the
package declares them, not appended at the end), each with its type, whether it's required,
its default, enumeration options, and category. Object-list widgets (e.g. DataGrid 2
columns) show their nested item properties indented. The dynamic rules read as
"property *X* is hidden when *Y*", and the trailing coverage line reports how many of the
widget's editor hide-rules mxcli recognized.

### `--format json`

Both the properties and the rules are available as JSON for scripting:

```json
{
  "widgetId": "com.mendix.widget.web.combobox.Combobox",
  "version": "2.5.0",
  "source": "project .mpk",
  "properties": [
    { "key": "source", "type": "enumeration", "required": true, "default": "context",
      "enum": ["context", "database", "static"], "category": "General::Data source" }
  ],
  "dynamicRules": [
    { "property": "loadMoreButtonCaption", "hiddenWhen": "pagination ≠ \"loadMore\"" }
  ]
}
```

## Reconciling stored widgets after a package upgrade (`mxcli widget sync`)

Everything above keeps a widget correct **as it is authored**. When you later upgrade
the widget package, the instances already stored in your pages go stale and Mendix
reports CE0463 on each one. Studio Pro fixes this with "Update all widgets"; mxbuild
has `mx update-widgets`, which works but **destroys `mprcontents/`** on an MPR v2
project, collapsing it to a single-file v1 layout. `mxcli fix widgets` runs it and
restores the v2 layout, and is the complete fix.

`mxcli widget sync` is the alternative that does the reconciliation in mxcli's own
code:

```bash
mxcli widget sync -p app.mpr --dry-run     # preview; names every property
mxcli widget sync -p app.mpr               # apply
```

It reconciles **schema** — properties the package added, dropped or redefined — and
never changes a property value you set. It skips any widget whose `.mpk` is not
installed rather than guessing.

**This is partial today.** On the reference fixture (Data Widgets 3.4 → 3.11.3) it
clears 7 of 40 CE0463 errors where `mx update-widgets` clears all 40. The widget
*type* it writes is byte-identical to Mendix's own output; a value-level difference
that has not yet been identified keeps DataGrid2 and Gallery instances erroring.
Preview with `--dry-run` and confirm with `mx check` before relying on it. It runs on
either engine — the modelsdk and legacy backends produce structurally identical
output.

Each unit is verified for duplicate GUIDs before being written, and the run aborts
rather than writing one. This matters because Mendix validates GUID uniqueness only
when *saving*: a duplicate loads fine and passes `mx check`, then breaks the next save
— and `mx update-widgets` collapses `mprcontents/` before it discovers the problem,
leaving the project both flattened and unloadable.

## Marketplace and custom widgets

`mxcli widget describe -p app.mpr <widget-id>` works for **any** widget installed in the
project, including Marketplace and custom widgets mxcli has no embedded template for — it
reads the definition and editor rules straight from the installed `.mpk`. To teach the
page builder to author a custom widget from MDL, extract a definition for it first:

```bash
mxcli widget extract --mpk widgets/MyWidget.mpk    # writes a .def.json
mxcli widget init -p app.mpr                        # extract defs for all project widgets
```

A widget's MDL keyword is the last segment of its ID (`com.mendix.widget.web.combobox.Combobox`
→ `COMBOBOX`). When two installed widgets derive the same keyword, the pluggable one keeps it
and the other is named with its vendor in front (`itvisors.keyboardshortcut.KeyboardShortcut`
→ `ITVISORSKEYBOARDSHORTCUT`), or with `CUSTOM` in front for a legacy custom widget
(`KeyboardShortcut.widget.KeyboardShortcut` → `CUSTOMKEYBOARDSHORTCUT`). `mxcli widget list`
shows which name each widget got.

`refresh catalog` refreshes these definitions too, but skips that step when no `.mpk` or
`.def.json` file has changed since the last refresh by the same mxcli build.

See [Marketplace Content](marketplace.md) for downloading and installing packages.
