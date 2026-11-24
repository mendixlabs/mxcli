# DESCRIBE WIDGET TYPE

## Synopsis

    DESCRIBE WIDGET TYPE <keyword>

    DESCRIBE WIDGET TYPE '<widget id>'

## Description

Shows the format mxcli has discovered for a pluggable or custom widget: its
properties (key, type, caption, category, required, default, enumeration
members), the **body containers** it accepts, the editor rules that hide a
property under some configurations, and a complete MDL example.

The answer is a report on the widget package installed in this project, not
MDL: its first line is `-- widget type definition (not executable)`, and with
`--json` the document carries `"executable": false`. `DESCRIBE GLYPH`,
`DESCRIBE CONTRACT …` and `DESCRIBE CATALOG.<table>` are marked the same way.
`DESCRIBE WIDGET <name>` without `TYPE` is the old spelling (`MDL-DEPR095`).

A widget was the only MDL extension point without a `DESCRIBE`. That is why
`mxcli widget init` writes markdown documentation at all — and why the two could
drift. `DESCRIBE WIDGET` and `mxcli widget describe` are the same function, so
they cannot disagree.

Unlike the other `DESCRIBE` statements, this one **works with no project open**:
"what can I write here?" is a question asked before anything is open. With `-p`,
the properties and rules come from the widget package actually installed in the
project (`widgets/*.mpk`) — version-accurate, and the only place a Marketplace
widget appears at all. Without it, they come from mxcli's embedded template.

## Parameters

*keyword*
: The widget's MDL name, as written in a page body — `combobox`, `htmlelement`,
  `datagrid`. Case-insensitive.

*widget id*
: The full widget id as a quoted string —
  `'com.mendix.widget.web.htmlelement.HTMLElement'`. Use this form for a widget
  whose MDL name is ambiguous, or when you have the id in hand from a `.mpk`.

## Examples

```sql
DESCRIBE WIDGET TYPE htmlelement;
```

Example output, abbreviated:

```
Widget: HTML Element (htmlelement)
  ID:      com.mendix.widget.web.htmlelement.HTMLElement
  Version: 1.2.2
  Kind:    pluggable
  Source:  project .mpk

Properties (23):
  tagName                    enumeration required default=div {div|span|p|ul|…}
  tagContentMode             enumeration required default=container {container|innerHTML}
  attributes                 object (General::HTML attributes)
    attributeName              string required
    attributeValueType         enumeration required default=expression {expression|template}

Body containers (4):
  attribute                  object list  -> attributes                 authorable
                               items: attributeName, attributeValueType, …
  event                      object list  -> events                     authorable
                               items: eventName, eventAction, …
  tagcontentcontainer        child slot   -> tagContentContainer        authorable
  tagcontentrepeatcontainer  child slot   -> tagContentRepeatContainer  authorable

MDL example (parses as written):
  htmlelement widget1 (
    tagName: 'div',
    tagUseRepeat: false,
    tagContentMode: 'container'
  ) {
    attribute item1 (attributeValueType: 'expression')   -- one entry of `attributes`
    event item2 (eventName: 'onClick')   -- one entry of `events`
    tagcontentcontainer slot3 {
      -- widgets for `tagContentContainer`
    }
  }
```

By widget id:

```sql
DESCRIBE WIDGET TYPE 'com.mendix.widget.web.htmlelement.HTMLElement';
```

## Notes

**Body containers report whether MDL can express them.** `authorable` is derived
by parsing a probe against the live grammar, never read from a list — so the mark
is correct by construction rather than by maintenance. A container reported as
not authorable is one to set in Studio Pro.

**The MDL example parses and checks as written.** The whole block is parsed
before it is emitted, and its property values are real members rather than
placeholders. Bindings it cannot fill — a datasource, an attribute, an action —
are **named rather than invented**, under `-- omitted:`, because a generic
example cannot know a name from your project.

**The example is narrowed by the widget's own editor rules.** A property the
widget hides under the configuration the example picked is left out, so what you
see is what that configuration actually supports. With the widget's `.mpk` in the
project, which properties are hidden is decided by running its `editorConfig.js`,
as Studio Pro does. The listed rules are the ones mxcli could also read from the
source; the footer reports how many, and those are what an object-list item's
properties still go by.

The count is of **hide-calls whose condition was read**, which is not the same
as every property being accounted for. A widget editor often builds one call's
property list by concatenating shared arrays onto a literal one — Combo box
hides ten database properties that way in a single call. Those are resolved, so
the rules appear; but where the concatenated argument is computed at runtime
(`.concat(n(b.static))`) it cannot be resolved statically, and only the literal
names are reported. Nothing is guessed: an unresolvable list contributes its
literal members and no more.

A rule can carry several conditions, joined with `and`:

```
clearable   hidden when optionsSourceType = "boolean"
                   and optionsSourceType is one of enumeration, boolean
                   and source = "context"
```

That is not verbosity — it is the rule. Widget editors nest their branches, and
a property hidden three levels in is hidden only where **all** three hold.
Reading out the innermost condition alone would claim the property is hidden far
more often than it is, so every term is shown and every term must hold before a
binding is pruned. A rule with one indeterminable term prunes nothing: the
binding is listed and you decide.

**`LIST WIDGETS` does not exist**, deliberately. `LIST WIDGETS` already means
widget *instances placed on pages*, and the definitions are
`SELECT * FROM CATALOG.WIDGET_DEFINITIONS`.

## See Also

[LIST WIDGETS](list-widgets.md), [DESCRIBE PAGE](describe-page.md),
[Pluggable Widgets Across Versions](../../guides/pluggable-widgets.md)
