// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
)

// A legacy (Dojo) Forms$DataGrid. MDL cannot author one (MDL-WIDGET38), so
// describe still prints it as comments — but structurally, with the stored name
// of every column, search field and control-bar button, because those names are
// what `alter page … drop <name>` and `set (Caption: …) on <name>` address on
// the stored page. A single opaque line left them unaddressable and made the
// only remedy replacing the whole grid (mendixlabs/mxcli#1459).

// rawLegacyGrid is what describe reads from a Forms$DataGrid.
type rawLegacyGrid struct {
	ControlBar   []rawLegacyGridItem
	Columns      []rawLegacyGridItem
	SearchFields []rawLegacyGridItem
}

// rawLegacyGridItem is one named element of a legacy grid: a control-bar
// button, a column or a search field. Kind is the $Type without its
// `Forms$` prefix; Type is a search field's visibility (Normal/Hidden/ReadOnly).
type rawLegacyGridItem struct {
	Kind      string
	Name      string
	Caption   string
	Attribute string
	Type      string
}

// parseLegacyGrid reads the named parts of a Forms$DataGrid: ControlBar.Items,
// Columns and the source's SearchBar.Items.
func parseLegacyGrid(ctx *ExecContext, w map[string]any) *rawLegacyGrid {
	g := &rawLegacyGrid{}
	if cb, ok := w["ControlBar"].(map[string]any); ok {
		g.ControlBar = parseLegacyGridItems(ctx, cb["Items"])
	}
	g.Columns = parseLegacyGridItems(ctx, w["Columns"])
	if ds, ok := w["DataSource"].(map[string]any); ok {
		if sb, ok := ds["SearchBar"].(map[string]any); ok {
			g.SearchFields = parseLegacyGridItems(ctx, sb["Items"])
		}
	}
	return g
}

func parseLegacyGridItems(ctx *ExecContext, v any) []rawLegacyGridItem {
	var out []rawLegacyGridItem
	for _, e := range getBsonArrayElements(v) {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		item := rawLegacyGridItem{
			Kind:    strings.TrimPrefix(extractString(m["$Type"]), "Forms$"),
			Name:    extractString(m["Name"]),
			Caption: extractTextCaption(ctx, m),
			Type:    extractString(m["Type"]),
		}
		if ref, ok := m["AttributeRef"].(map[string]any); ok && ref != nil {
			item.Attribute = columnAttributeFromRef(ctx, ref)
		}
		if item.Attribute == "" {
			// Older models store the binding as a path string instead.
			item.Attribute = shortAttributeName(extractString(m["AttributePath"]))
		}
		out = append(out, item)
	}
	return out
}

// outputLegacyGridV3 prints a legacy data grid. Every line is a comment: the
// widget cannot be built, so executing the describe output must not create
// anything in its place — it drops the grid, which the header line says.
func outputLegacyGridV3(ctx *ExecContext, prefix string, w rawWidget) {
	describeDroppedWidget(ctx, w)
	fmt.Fprintf(ctx.Output, "%s-- %s", prefix, w.Type)
	if w.Name != "" {
		fmt.Fprintf(ctx.Output, " (%s)", w.Name)
	}
	fmt.Fprint(ctx.Output, "  -- NOT re-executable: mxcli cannot author this widget, so re-running this script would drop it\n")
	g := w.LegacyGrid
	if g == nil {
		return
	}
	fmt.Fprintf(ctx.Output, "%s--   its columns, search fields and buttons are addressable by name: alter page … drop <name> | set (Caption: '…') on <name>\n", prefix)
	if expr := dataSourceExpr(ctx, w.DataSource); expr != "" {
		fmt.Fprintf(ctx.Output, "%s--   DataSource: %s\n", prefix, legacyCommentText(expr))
	}
	for _, b := range g.ControlBar {
		fmt.Fprintf(ctx.Output, "%s--   button %s (%s)\n", prefix, legacyItemName(b), legacyItemProps(b, false))
	}
	for _, c := range g.Columns {
		fmt.Fprintf(ctx.Output, "%s--   column %s (%s)\n", prefix, legacyItemName(c), legacyItemProps(c, false))
	}
	for _, s := range g.SearchFields {
		fmt.Fprintf(ctx.Output, "%s--   searchfield %s (%s)\n", prefix, legacyItemName(s), legacyItemProps(s, true))
	}
}

func legacyItemName(it rawLegacyGridItem) string {
	if it.Name == "" {
		return "<unnamed>"
	}
	return mdlIdent(it.Name)
}

func legacyItemProps(it rawLegacyGridItem, withType bool) string {
	props := []string{it.Kind}
	if it.Caption != "" {
		props = append(props, "Caption: "+mdlQuoted(legacyCommentText(it.Caption)))
	}
	if it.Attribute != "" {
		props = append(props, "Attribute: "+it.Attribute)
	}
	if withType && it.Type != "" {
		props = append(props, "Type: "+it.Type)
	}
	return strings.Join(props, ", ")
}

// legacyCommentText keeps a value on its comment line: a line break in a
// caption would end the comment and leave the rest as a statement.
func legacyCommentText(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}
