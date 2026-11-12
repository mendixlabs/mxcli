// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"go.mongodb.org/mongo-driver/v2/bson"
	"io"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mdl0Quote writes s as an mdl 0 string literal: a backslash, a line break, a
// carriage return and a tab are backslash escapes, and an apostrophe is
// doubled. Describe reaches it only through mdlQuote.
func mdl0Quote(s string) string {
	escaped := strings.NewReplacer(
		"\\", "\\\\",
		"\n", "\\n",
		"\r", "\\r",
		"\t", "\\t",
		"'", "''",
	).Replace(s)
	return "'" + escaped + "'"
}

// explicitPropValue renders a pluggable widget's property value as MDL.
//
// Every value used to be emitted raw, so a string lost its quotes and a JSON
// spec broke the re-parse at its first '{' — which took any page carrying a
// pluggable widget out of the DESCRIBE-edit-CREATE OR REPLACE workflow that
// mxcli itself documents (ledger #104).
//
// The DECLARED type decides, not the value's shape: a String property holding
// "30" or "true" must still come back quoted, or re-executing it writes a
// number where the author wrote text. Where no type is declared — the widget's
// schema is not in the document — the shape is the only signal left, and the
// fallback quotes anything that is not plainly a number or boolean, because an
// unquoted arbitrary string may not parse at all while a quoted literal still
// round-trips as text.
func explicitPropValue(ctx *ExecContext, p rawExplicitProp) string {
	if p.IsRef {
		return p.Value // an attribute name is an identifier, never a literal
	}
	switch p.ValueType {
	case "Boolean", "Integer", "Decimal":
		return p.Value
	case "":
		if isBareLiteral(p.Value) {
			return p.Value
		}
		return mdlQuote(ctx, p.Value)
	default:
		return mdlQuote(ctx, p.Value)
	}
}

// isBareLiteral reports whether a value can be emitted without quotes when the
// property's declared type is unknown: a boolean, or a plain decimal number.
func isBareLiteral(s string) bool {
	if s == "true" || s == "false" {
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		// ParseFloat also accepts "NaN", "Inf" and hex/exponent forms, none of
		// which is a number an MDL author would have written; require the text
		// to be made only of digits, one sign and one point.
		return looksNumeric(s)
	}
	return false
}

func looksNumeric(s string) bool {
	if s == "" {
		return false
	}
	body := strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	if body == "" {
		return false
	}
	dots := 0
	for _, r := range body {
		switch {
		case r >= '0' && r <= '9':
		case r == '.':
			dots++
			if dots > 1 {
				return false
			}
		default:
			return false
		}
	}
	return body != "."
}

// appendExplicitProps appends a pluggable widget's settings printed under their
// own key: `key: value`, and a text template's `{N}` bindings as the
// `<key>Params` companion — a `{1}` re-executed without its parameter is CE0720,
// so the companion travels with the text it belongs to (#575).
func appendExplicitProps(ctx *ExecContext, props []string, explicit []rawExplicitProp) []string {
	for _, ep := range explicit {
		props = append(props, fmt.Sprintf("%s: %s", ep.Key, explicitPropValue(ctx, ep)))
		if len(ep.Params) > 0 {
			props = append(props, fmt.Sprintf("%sParams: (%s)",
				ep.Key, strings.Join(formatParametersV3(ep.Params), ", ")))
		}
	}
	return props
}

// appendDataGridPagingProps appends non-default paging properties for DataGrid2.
func appendDataGridPagingProps(props []string, w rawWidget) []string {
	if w.PageSize != "" && w.PageSize != "20" {
		props = append(props, fmt.Sprintf("PageSize: %s", w.PageSize))
	}
	if w.Pagination != "" && w.Pagination != "buttons" {
		props = append(props, fmt.Sprintf("Pagination: %s", w.Pagination))
	}
	if w.PagingPosition != "" && w.PagingPosition != "bottom" {
		props = append(props, fmt.Sprintf("PagingPosition: %s", w.PagingPosition))
	}
	if w.ShowPagingButtons != "" && w.ShowPagingButtons != "always" {
		props = append(props, fmt.Sprintf("ShowPagingButtons: %s", w.ShowPagingButtons))
	}
	// showNumberOfRows: not yet fully supported in DataGrid2, skip to avoid CE0463
	return props
}

// widgetConditionMDL writes a conditional Visible / Editable as the bare
// expression it stores (R5, ako/mxcli#753). One that would not read back bare
// as itself — `true`, a lone name, text the MDL expression grammar cannot
// parse — is written in the deprecated bracketed form, as before.
func widgetConditionMDL(key, expr string) string {
	expr = strings.TrimSpace(expr)
	if visitor.BareWidgetCondition(key, expr) {
		return key + ": " + expr
	}
	return fmt.Sprintf("%s: [%s]", key, expr)
}

// appendAppearanceProps appends Class, Style, DesignProperties, and conditional
// settings if present — and editability, which every input widget carries.
//
// This lived in the CheckBox case alone, so `describe page` printed no
// editability for a textbox. That made a describe/exec round-trip silently
// normalise `Editable: Never` back to Always, and it meant a describe/diff could
// not reveal the CREATE-path loss reported in ako/mxcli-maintenance-2 — both
// sides printed nothing, so the documents compared equal while the model was
// wrong.
// appendInputValidationProps emits the input-widget properties a round trip used
// to drop: the password flag and the per-widget validation (ako/mxcli#550).
//
// Both are emitted only when set. `Password: false` is every ordinary text box,
// and an empty validation is what Studio Pro stores on a widget that has none,
// so emitting either would put a clause in the user's script that they never
// wrote — the "invents" shape in docs-wiki/bug-patterns/describe-round-trip-gaps.md.
//
// The message rides with the expression rather than standing alone: Mendix has
// nowhere to show a message for a validation that never fails.
func appendInputValidationProps(ctx *ExecContext, props []string, w rawWidget) []string {
	if w.IsPassword {
		props = append(props, "Password: true")
	}
	if w.ValidationExpression != "" {
		// Quoted, not bracketed. `[...]` is the XPath-constraint spelling and the
		// grammar parses it as an ARRAY of expressions (propertyValueV3), so a
		// bracketed expression comes back to the builder as []any and
		// GetStringProp yields "" — the round trip looked right in the emitter's
		// own test and still lost the value on a real page. mdlQuote doubles any
		// embedded quote, which a Mendix expression over $value may well carry.
		props = append(props, fmt.Sprintf("Validation: %s", mdlQuote(ctx, w.ValidationExpression)))
		if w.ValidationMessage != "" {
			props = append(props, fmt.Sprintf("ValidationMessage: %s", mdlQuote(ctx, w.ValidationMessage)))
		}
	}
	return props
}

func appendAppearanceProps(ctx *ExecContext, props []string, w rawWidget) []string {
	// Only when it deviates from Mendix's default, so unchanged widgets keep a
	// quiet round-trip. Empty means the widget type has no editability at all.
	// "Conditional" is not authorable: the `Editable: <expr>` printed below is
	// what makes it conditional, and printing both gives the key twice.
	if w.Editable != "" && w.Editable != "Always" && !(w.Editable == "Conditional" && w.EditableIf != "") {
		props = append(props, fmt.Sprintf("Editable: %s", w.Editable))
	}
	if w.Class != "" {
		props = append(props, fmt.Sprintf("Class: %s", mdlQuote(ctx, w.Class)))
	}
	if w.Style != "" {
		props = append(props, fmt.Sprintf("Style: %s", mdlQuote(ctx, w.Style)))
	}
	if w.DynamicClasses != "" {
		props = append(props, fmt.Sprintf("DynamicClasses: %s", w.DynamicClasses)) // an expression, printed as-is
	}
	if len(w.DesignProperties) > 0 {
		props = append(props, formatDesignPropertiesMDL(ctx, w.DesignProperties))
	}
	if w.VisibleIf != "" {
		props = append(props, widgetConditionMDL("Visible", w.VisibleIf))
	}
	if prop := visibleWhenProp(w); prop != "" {
		props = append(props, prop)
	}
	if prop := visibleRolesProp(w); prop != "" {
		props = append(props, prop)
	}
	if w.EditableIf != "" {
		props = append(props, widgetConditionMDL("Editable", w.EditableIf))
	}
	return props
}

// formatDesignPropertiesMDL formats design properties as MDL V3 syntax.
// Toggle → 'Key': ON, Option → 'Key': 'Value'
func formatDesignPropertiesMDL(ctx *ExecContext, dps []rawDesignProp) string {
	return fmt.Sprintf("DesignProperties: (%s)", joinDesignPropertyEntries(ctx, dps))
}

// joinDesignPropertyEntries renders design-property entries as comma-separated
// MDL. Compound properties recurse into a nested list (issue #668):
// 'Spacing': ('margin-top': 'Large', 'margin-bottom': 'Medium').
func joinDesignPropertyEntries(ctx *ExecContext, dps []rawDesignProp) string {
	var entries []string
	for _, dp := range dps {
		switch dp.ValueType {
		case "toggle":
			entries = append(entries, fmt.Sprintf("%s: on", mdlQuote(ctx, dp.Key)))
		case "option":
			entries = append(entries, fmt.Sprintf("%s: %s", mdlQuote(ctx, dp.Key), mdlQuote(ctx, dp.Option)))
		case "compound":
			entries = append(entries, fmt.Sprintf("%s: (%s)", mdlQuote(ctx, dp.Key), joinDesignPropertyEntries(ctx, dp.Nested)))
		}
	}
	return strings.Join(entries, ", ")
}

// formatWidgetProps writes a widget line with automatic multi-line wrapping.
// If the single-line form exceeds 120 chars, each property is written on its own line.
// header is the widget keyword + name (e.g. "DATAGRID ProductGrid"),
// suffix is the trailing content (e.g. "\n" or " {\n").
func formatWidgetProps(w io.Writer, prefix string, header string, props []string, suffix string) {
	if len(props) == 0 {
		fmt.Fprintf(w, "%s%s%s", prefix, header, suffix)
		return
	}
	// A `--` comment entry runs to the end of its line, so it must sit on a line
	// of its own and never carry a separator: on the single-line form it
	// swallowed the rest of the list and the `)`, and as the last entry it left
	// the previous line's `,` dangling. Either way the output did not parse.
	lastProp := -1
	for i, p := range props {
		if !isCommentProp(p) {
			lastProp = i
		}
	}
	if lastProp == len(props)-1 {
		singleLine := fmt.Sprintf("%s%s (%s)%s", prefix, header, strings.Join(props, ", "), suffix)
		if len(singleLine) <= 120 && !containsCommentProp(props) {
			fmt.Fprint(w, singleLine)
			return
		}
	}
	if lastProp < 0 {
		// Only comments: there is no property list to write.
		for _, p := range props {
			fmt.Fprintf(w, "%s%s\n", prefix, p)
		}
		fmt.Fprintf(w, "%s%s%s", prefix, header, suffix)
		return
	}
	// Multi-line
	indent := prefix + "  "
	fmt.Fprintf(w, "%s%s (\n", prefix, header)
	for i, p := range props {
		if i < lastProp && !isCommentProp(p) {
			fmt.Fprintf(w, "%s%s,\n", indent, p)
		} else {
			fmt.Fprintf(w, "%s%s\n", indent, p)
		}
	}
	fmt.Fprintf(w, "%s)%s", prefix, suffix)
}

func isCommentProp(p string) bool { return strings.HasPrefix(p, "--") }

func containsCommentProp(props []string) bool {
	for _, p := range props {
		if isCommentProp(p) {
			return true
		}
	}
	return false
}

// outputDataContainerContext writes a comment showing available variables inside a data container.
// isList indicates list containers (DataGrid2, ListView, Gallery) where a selection variable is available.
func outputDataContainerContext(w io.Writer, prefix string, widgetName string, entityRef string, isList bool) {
	if entityRef == "" {
		return
	}
	parts := []string{fmt.Sprintf("$currentObject (%s)", entityRef)}
	if isList && widgetName != "" {
		parts = append(parts, fmt.Sprintf("$%s (selection)", widgetName))
	}
	fmt.Fprintf(w, "%s-- Context: %s\n", prefix, strings.Join(parts, ", "))
}

// outputWidgetMDLV3 outputs a widget in MDL V3 syntax.
// V3 syntax uses WIDGET Name (Props) { children } format.
func outputWidgetMDLV3(ctx *ExecContext, w rawWidget, indent int) {
	if describeDanglingSelection(ctx, w, indent) {
		return // listens to a widget this description cannot emit (#1415)
	}
	prefix := strings.Repeat("  ", indent)

	switch w.Type {
	case "Forms$ScrollContainer", "Pages$ScrollContainer":
		header := fmt.Sprintf("scrollcontainer %s", mdlIdent(w.Name))
		var props []string
		// Only Sidebar: Headline is what an unset scroll container is, so a
		// layout that never set it describes as before (ako/mxcli#1703).
		if w.ScrollLayoutMode != "" && w.ScrollLayoutMode != pages.DefaultScrollContainerLayoutMode {
			props = append(props, fmt.Sprintf("LayoutMode: %s", mdlQuote(ctx, w.ScrollLayoutMode)))
		}
		props = appendAppearanceProps(ctx, props, w)
		if len(w.Children) > 0 {
			formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
			for _, child := range w.Children {
				outputWidgetMDLV3(ctx, child, indent+1)
			}
			fmt.Fprintf(ctx.Output, "%s}\n", prefix)
		} else {
			formatWidgetProps(ctx.Output, prefix, header, props, "\n")
		}

	case "Forms$Header", "Pages$Header", "Forms$HeaderSide":
		// A layout header and its two sides (the latter synthetic, like a
		// scroll container's regions).
		header := fmt.Sprintf("layoutheader %s", mdlIdent(w.Name))
		var props []string
		if w.Type == "Forms$HeaderSide" {
			header = "region " + w.Name
		} else {
			props = appendAppearanceProps(ctx, nil, w)
		}
		if len(w.Children) == 0 {
			formatWidgetProps(ctx.Output, prefix, header, props, "\n")
			break
		}
		formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
		for _, child := range w.Children {
			outputWidgetMDLV3(ctx, child, indent+1)
		}
		fmt.Fprintf(ctx.Output, "%s}\n", prefix)

	case "Forms$ScrollContainerRegion":
		// A synthetic wrapper: the slot is in Name, and `region top` is how MDL
		// spells it. 200/Auto is Studio Pro's untouched default, so emitting it
		// would put noise in every describe — and re-executing without it lands
		// back on the same value.
		header := fmt.Sprintf("region %s", w.Name)
		var props []string
		if w.RegionSize != 0 && w.RegionSize != 200 {
			props = append(props, fmt.Sprintf("Size: %d", w.RegionSize))
		}
		if w.RegionSizeMode != "" && w.RegionSizeMode != "Auto" {
			props = append(props, fmt.Sprintf("SizeMode: %s", mdlQuote(ctx, w.RegionSizeMode)))
		}
		if w.RegionToggleMode != "" && w.RegionToggleMode != "None" {
			props = append(props, fmt.Sprintf("ToggleMode: %s", mdlQuote(ctx, w.RegionToggleMode)))
		}
		// Class is the one appearance key a region is read with and written
		// with (buildScrollContainerRegionV3); printing the rest would be
		// printing what re-execution refuses (ako/mxcli#1698).
		if w.Class != "" {
			props = append(props, fmt.Sprintf("Class: %s", mdlQuote(ctx, w.Class)))
		}
		if len(w.Children) > 0 {
			formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
			for _, child := range w.Children {
				outputWidgetMDLV3(ctx, child, indent+1)
			}
			fmt.Fprintf(ctx.Output, "%s}\n", prefix)
		} else {
			formatWidgetProps(ctx.Output, prefix, header, props, "\n")
		}

	case "Forms$Placeholder", "Pages$Placeholder":
		// No properties and no body: the name is the whole thing, and it is API
		// — a page binds to it as Module.Layout.<Name>.
		fmt.Fprintf(ctx.Output, "%splaceholder %s\n", prefix, mdlIdent(w.Name))

	case "Forms$NavigationTree", "Pages$NavigationTree", "Forms$MenuBar", "Pages$MenuBar",
		"Forms$SimpleMenuBar", "Pages$SimpleMenuBar":
		keyword := "navigationtree"
		switch {
		case strings.HasSuffix(w.Type, "$SimpleMenuBar"):
			keyword = "simplemenubar"
		case strings.HasSuffix(w.Type, "$MenuBar"):
			keyword = "menubar"
		}
		header := fmt.Sprintf("%s %s", keyword, mdlIdent(w.Name))
		var props []string
		if w.Menu != "" {
			props = append(props, fmt.Sprintf("Menu: %s", w.Menu))
		} else if w.NavigationProfile != "" {
			props = append(props, fmt.Sprintf("Profile: %s", mdlQuote(ctx, w.NavigationProfile)))
		}
		// Horizontal is the default the builder applies, so only Vertical
		// needs saying.
		if w.MenuOrientation == "Vertical" {
			props = append(props, "Orientation: Vertical")
		}
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$TabControl", "Pages$TabControl":
		header := fmt.Sprintf("tabcontainer %s", mdlIdent(w.Name))
		props := appendAppearanceProps(ctx, nil, w)
		if len(w.Children) > 0 {
			formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
			for _, child := range w.Children {
				outputWidgetMDLV3(ctx, child, indent+1)
			}
			fmt.Fprintf(ctx.Output, "%s}\n", prefix)
		} else {
			formatWidgetProps(ctx.Output, prefix, header, props, "\n")
		}

	case "Pages$TabPage":
		header := fmt.Sprintf("tabpage %s", mdlIdent(w.Name))
		var props []string
		if w.TabCaption != "" {
			props = append(props, fmt.Sprintf("Caption: %s", mdlQuote(ctx, w.TabCaption)))
		}
		props = describeTabPageBadge(ctx, props, w)
		// A tab page's visibility (#1465). Its Forms$TabPage has no Appearance,
		// so appendAppearanceProps would print nothing else either.
		if w.VisibleIf != "" {
			props = append(props, widgetConditionMDL("Visible", w.VisibleIf))
		}
		if prop := visibleWhenProp(w); prop != "" {
			props = append(props, prop)
		}
		if prop := visibleRolesProp(w); prop != "" {
			props = append(props, prop)
		}
		if len(w.Children) > 0 {
			formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
			for _, child := range w.Children {
				outputWidgetMDLV3(ctx, child, indent+1)
			}
			fmt.Fprintf(ctx.Output, "%s}\n", prefix)
		} else {
			formatWidgetProps(ctx.Output, prefix, header, props, "\n")
		}

	case "Forms$DivContainer", "Pages$DivContainer":
		header := fmt.Sprintf("container %s", mdlIdent(w.Name))
		var props []string
		if w.RenderMode != "" && w.RenderMode != "Div" {
			props = append(props, fmt.Sprintf("RenderMode: %s", w.RenderMode))
		}
		props = appendAppearanceProps(ctx, props, w)
		if w.Action != "" {
			props = append(props, actionProp("Action", w.Action))
		}
		if len(w.Children) > 0 {
			formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
			for _, child := range w.Children {
				outputWidgetMDLV3(ctx, child, indent+1)
			}
			fmt.Fprintf(ctx.Output, "%s}\n", prefix)
		} else {
			formatWidgetProps(ctx.Output, prefix, header, props, "\n")
		}

	case "Forms$GroupBox", "Pages$GroupBox":
		header := fmt.Sprintf("groupbox %s", mdlIdent(w.Name))
		props := []string{}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Caption: %s", mdlQuote(ctx, w.Caption)))
		}
		if w.HeaderMode != "" && w.HeaderMode != "Div" {
			props = append(props, fmt.Sprintf("HeaderMode: %s", w.HeaderMode))
		}
		if w.Collapsible != "" && w.Collapsible != "No" {
			switch w.Collapsible {
			case "YesInitiallyExpanded":
				props = append(props, "Collapsible: YesExpanded")
			case "YesInitiallyCollapsed":
				props = append(props, "Collapsible: YesCollapsed")
			default:
				props = append(props, fmt.Sprintf("Collapsible: %s", w.Collapsible))
			}
		}
		props = appendAppearanceProps(ctx, props, w)
		if len(w.Children) > 0 {
			formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
			for _, child := range w.Children {
				outputWidgetMDLV3(ctx, child, indent+1)
			}
			fmt.Fprintf(ctx.Output, "%s}\n", prefix)
		} else {
			formatWidgetProps(ctx.Output, prefix, header, props, "\n")
		}

	case "Forms$LayoutGrid", "Pages$LayoutGrid":
		header := "layoutgrid"
		if w.Name != "" {
			header += " " + mdlIdent(w.Name)
		}
		props := describeLayoutGridWidth(nil, w)
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
		for _, row := range w.Rows {
			// Mendix stores no name on a row or a column (R12, #749), so
			// describe writes none: an invented `row1` / `col3` churned when a
			// row or column was inserted, and meant nothing on re-execution.
			// Alignment and appearance only when set, so a plain row stays `row {`.
			var rowProps []string
			if a := layoutGridAlignmentMDL(row.VerticalAlignment); a != "" {
				rowProps = append(rowProps, "VerticalAlignment: "+a)
			}
			if a := layoutGridAlignmentMDL(row.HorizontalAlignment); a != "" {
				rowProps = append(rowProps, "HorizontalAlignment: "+a)
			}
			if !row.SpacingBetweenColumns {
				rowProps = append(rowProps, "SpacingBetweenColumns: false")
			}
			rowProps = appendAppearanceProps(ctx, rowProps, row.Appearance)
			formatWidgetProps(ctx.Output, prefix+"  ", "row", rowProps, " {\n")
			for _, col := range row.Columns {
				// The desktop width is always printed. A tablet or phone width
				// is printed unless it is auto-fill, which is what the builder
				// writes when the property is left out.
				colProps := []string{"DesktopWidth: " + layoutGridWidthMDL(col.Width)}
				if w := layoutGridWidthMDL(col.TabletWidth); w != "AutoFill" {
					colProps = append(colProps, "TabletWidth: "+w)
				}
				if w := layoutGridWidthMDL(col.PhoneWidth); w != "AutoFill" {
					colProps = append(colProps, "PhoneWidth: "+w)
				}
				if a := layoutGridAlignmentMDL(col.VerticalAlignment); a != "" {
					colProps = append(colProps, "VerticalAlignment: "+a)
				}
				colProps = appendAppearanceProps(ctx, colProps, col.Appearance)
				formatWidgetProps(ctx.Output, prefix+"    ", "column", colProps, " {\n")
				for _, cw := range col.Widgets {
					outputWidgetMDLV3(ctx, cw, indent+3)
				}
				fmt.Fprintf(ctx.Output, "%s    }\n", prefix)
			}
			fmt.Fprintf(ctx.Output, "%s  }\n", prefix)
		}
		fmt.Fprintf(ctx.Output, "%s}\n", prefix)

	case "Forms$DynamicText", "Pages$DynamicText":
		header := fmt.Sprintf("dynamictext %s", mdlIdent(w.Name))
		props := []string{}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Content: %s", mdlQuote(ctx, w.Content)))
		}
		if w.RenderMode != "" && w.RenderMode != "Text" {
			props = append(props, fmt.Sprintf("RenderMode: %s", w.RenderMode))
		}
		// Text is what every dynamic text stores, so only a heading is printed.
		if w.NativeTextStyle != "" && w.NativeTextStyle != "Text" {
			props = append(props, fmt.Sprintf("NativeTextStyle: %s", w.NativeTextStyle))
		}
		if len(w.Parameters) > 0 {
			props = append(props, fmt.Sprintf("ContentParams: (%s)", strings.Join(formatParametersV3(w.Parameters), ", ")))
		}
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$ActionButton", "Pages$ActionButton", "Forms$SidebarToggleButton", "Pages$SidebarToggleButton":
		// RenderType "Link" round-trips as the `linkbutton` keyword. A sidebar
		// toggle has no keyword of its own for it, so says it as a property.
		toggle := strings.HasSuffix(w.Type, "$SidebarToggleButton")
		keyword := "actionbutton"
		switch {
		case toggle:
			keyword = "sidebartoggle"
		case w.RenderMode == "Link":
			keyword = "linkbutton"
		}
		header := fmt.Sprintf("%s %s", keyword, mdlIdent(w.Name))
		props := []string{}
		if toggle && w.RenderMode == "Link" {
			props = append(props, "RenderMode: Link")
		}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Caption: %s", mdlQuote(ctx, w.Caption)))
		}
		if len(w.Parameters) > 0 {
			props = append(props, fmt.Sprintf("CaptionParams: (%s)", strings.Join(formatParametersV3(w.Parameters), ", ")))
		}
		if w.Tooltip != "" {
			props = append(props, fmt.Sprintf("Tooltip: %s", mdlQuote(ctx, w.Tooltip)))
		}
		if w.Action != "" {
			props = append(props, actionProp("Action", w.Action))
		}
		if w.ButtonStyle != "" && w.ButtonStyle != "Default" {
			props = append(props, fmt.Sprintf("ButtonStyle: %s", w.ButtonStyle))
		}
		if clause := widgetIconMDL(ctx, w); clause != "" {
			props = append(props, clause)
		}
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")
		if note := widgetIconNote(ctx, w); note != "" {
			fmt.Fprintf(ctx.Output, "%s%s\n", prefix, note)
		}
		if note := outputMappingsNote(w); note != "" {
			fmt.Fprintf(ctx.Output, "%s%s\n", prefix, note)
		}

	case "Forms$Text", "Pages$Text":
		props := []string{}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Content: %s", mdlQuote(ctx, w.Content)))
		}
		props = appendAppearanceProps(ctx, props, w)
		// Forms$Text only survives in a project converted up from an old
		// Mendix; writing one is refused (MDL-WIDGET29). Keep the name anyway,
		// so the output parses and that refusal — not a parse error — is what
		// the reader sees.
		formatWidgetProps(ctx.Output, prefix, fmt.Sprintf("statictext %s", mdlIdent(w.Name)), props, "\n")

	case "Forms$Title", "Pages$Title":
		header := fmt.Sprintf("title %s", mdlIdent(w.Name))
		props := []string{}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Content: %s", mdlQuote(ctx, w.Caption)))
		}
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$DataView", "Pages$DataView":
		header := fmt.Sprintf("dataview %s", mdlIdent(w.Name))
		props := []string{}
		props = appendWidgetDataSources(ctx, props, w)
		switch {
		case w.LabelWidth == 0:
			props = append(props, "FormOrientation: Vertical")
		case w.LabelWidth > 0 && w.LabelWidth != 3:
			props = append(props, fmt.Sprintf("LabelWidth: %d", w.LabelWidth))
		}
		// A `footer { … }` block already implies ShowFooter: true, so emit the property
		// only when the implicit rule would not reproduce the stored value — an empty
		// shown footer, or declared footer widgets that are hidden (#813).
		if hasFooter := dataViewHasFooterBlock(w); hasFooter != w.ShowFooter {
			props = append(props, fmt.Sprintf("ShowFooter: %t", w.ShowFooter))
		}
		// A DataView has its own ReadOnlyStyle, distinct from a CheckBox's and
		// wired nowhere until ako/mxcli#550. Inherit is Studio Pro's default, so
		// only the other two are emitted.
		if w.ReadOnlyStyle != "" && w.ReadOnlyStyle != "Inherit" {
			props = append(props, fmt.Sprintf("ReadOnlyStyle: %s", w.ReadOnlyStyle))
		}
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
		outputDataContainerContext(ctx.Output, prefix+"  ", w.Name, w.EntityContext, false)
		for _, child := range w.Children {
			outputWidgetMDLV3(ctx, child, indent+1)
		}
		fmt.Fprintf(ctx.Output, "%s}\n", prefix)

	case "Forms$TextBox", "Pages$TextBox":
		header := fmt.Sprintf("textbox %s", mdlIdent(w.Name))
		props := []string{}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Label: %s", mdlQuote(ctx, w.Caption)))
		}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Attribute: %s", w.Content))
		}
		props = appendReadOnlyStyle(props, w)
		if w.Placeholder != "" {
			props = append(props, fmt.Sprintf("Placeholder: %s", mdlQuote(ctx, w.Placeholder)))
		}
		props = describeKeyboardType(props, w)
		if w.OnChange != "" {
			props = append(props, actionProp("OnChange", w.OnChange))
		}
		props = appendFocusEventProps(props, w)
		if w.OnEnterKeyPress != "" {
			props = append(props, actionProp("OnEnterKeyPress", w.OnEnterKeyPress))
		}
		props = append(props, w.Formatting...)
		props = appendInputValidationProps(ctx, props, w)
		props = describeSubmitBehaviour(props, w)
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$TextArea", "Pages$TextArea":
		header := fmt.Sprintf("textarea %s", mdlIdent(w.Name))
		props := []string{}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Label: %s", mdlQuote(ctx, w.Caption)))
		}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Attribute: %s", w.Content))
		}
		props = appendReadOnlyStyle(props, w)
		if w.Placeholder != "" {
			props = append(props, fmt.Sprintf("Placeholder: %s", mdlQuote(ctx, w.Placeholder)))
		}
		if w.OnChange != "" {
			props = append(props, actionProp("OnChange", w.OnChange))
		}
		props = appendFocusEventProps(props, w)
		props = describeSubmitBehaviour(props, w)
		// Printed when not Studio Pro's default — 5 lines, Autocomplete on — so
		// a text area authored without them describes as it did (ako/mxcli#977).
		if w.NumberOfLines != nil && *w.NumberOfLines != 5 {
			props = append(props, fmt.Sprintf("NumberOfLines: %d", *w.NumberOfLines))
		}
		if w.Autocomplete != nil && !*w.Autocomplete {
			props = append(props, "Autocomplete: false")
		}
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$FileManager", "Pages$FileManager":
		header := fmt.Sprintf("filemanager %s", mdlIdent(w.Name))
		props := []string{}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Label: %s", mdlQuote(ctx, w.Caption)))
		}
		props = append(props, w.FileManagerProps...)
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$DatePicker", "Pages$DatePicker":
		header := fmt.Sprintf("datepicker %s", mdlIdent(w.Name))
		props := []string{}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Label: %s", mdlQuote(ctx, w.Caption)))
		}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Attribute: %s", w.Content))
		}
		props = appendReadOnlyStyle(props, w)
		// The picker's mode — without it a date-time picker described as a
		// date-only one, and describe → exec wrote it that way (ako/mxcli#968).
		props = append(props, w.Formatting...)
		if w.OnChange != "" {
			props = append(props, actionProp("OnChange", w.OnChange))
		}
		props = appendFocusEventProps(props, w)
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$RadioButtons", "Pages$RadioButtons":
		header := fmt.Sprintf("radiobuttons %s", mdlIdent(w.Name))
		props := []string{}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Label: %s", mdlQuote(ctx, w.Caption)))
		}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Attribute: %s", w.Content))
		}
		props = appendReadOnlyStyle(props, w)
		if w.RenderVertical {
			props = append(props, "RenderHorizontal: false")
		}
		if w.OnChange != "" {
			props = append(props, actionProp("OnChange", w.OnChange))
		}
		props = appendFocusEventProps(props, w)
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$CheckBox", "Pages$CheckBox":
		header := fmt.Sprintf("checkbox %s", mdlIdent(w.Name))
		props := []string{}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Label: %s", mdlQuote(ctx, w.Caption)))
		}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Attribute: %s", w.Content))
		}
		props = appendReadOnlyStyle(props, w)
		// Show ShowLabel if false (not showing label)
		if !w.ShowLabel {
			props = append(props, "ShowLabel: No")
		}
		if w.LabelPosition != "" && w.LabelPosition != "Default" {
			props = append(props, "LabelPosition: "+w.LabelPosition)
		}
		if w.OnChange != "" {
			props = append(props, actionProp("OnChange", w.OnChange))
		}
		props = appendFocusEventProps(props, w)
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "CustomWidgets$CustomWidget":
		widgetType := w.RenderMode // We stored widget type in RenderMode
		if widgetType == "" {
			widgetType = "customwidget"
		}
		// Handle DataGrid2 specially with datasource and columns
		if widgetType == "datagrid2" && (w.DataSource != nil || len(w.DataGridColumns) > 0) {
			header := fmt.Sprintf("datagrid %s", mdlIdent(w.Name))
			props := []string{}
			props = appendWidgetDataSources(ctx, props, w)
			// Add selection mode if specified
			if w.Selection != "" {
				props = append(props, fmt.Sprintf("Selection: %s", w.Selection))
			}
			// onClick action (ledger #67)
			if w.OnClick != "" {
				props = append(props, actionProp("onClick", w.OnClick))
			}
			props = appendNamedActionProps(props, w)
			// Add paging properties if non-default
			props = appendDataGridPagingProps(props, w)
			// Every other setting that differs from its default, under the
			// widget's own key (configurationStorageType, columnsHidable, …).
			props = appendExplicitProps(ctx, props, w.ExplicitProperties)
			props = appendAppearanceProps(ctx, props, w)
			// Output CONTROLBAR, the widget slots and columns as children
			hasContent := len(w.ControlBar) > 0 || len(w.DataGridColumns) > 0 ||
				len(w.ChildSlots) > 0 || len(w.OmittedProps) > 0
			if hasContent {
				formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
				outputDataContainerContext(ctx.Output, prefix+"  ", w.Name, w.EntityContext, true)
				writeOmittedPropsNote(ctx.Output, prefix+"  ", w.OmittedProps)
				// Output CONTROLBAR section if control bar widgets present
				if len(w.ControlBar) > 0 {
					fmt.Fprintf(ctx.Output, "%s  controlbar {\n", prefix)
					for _, cb := range w.ControlBar {
						outputWidgetMDLV3(ctx, cb, indent+2)
					}
					fmt.Fprintf(ctx.Output, "%s  }\n", prefix)
				}
				// The empty-state placeholder, and any other slot the grid declares.
				outputChildSlots(ctx, w.ChildSlots, prefix+"  ", indent+1)
				// Output columns. A DataGrid 2 column has no name in the model,
				// so none is written: the one derived from its attribute gave two
				// columns over one attribute the same name (#749). ALTER PAGE
				// addresses a column by what it shows: `grid column(Attr)`.
				for _, col := range w.DataGridColumns {
					outputDataGrid2ColumnV3(ctx, prefix+"  ", col)
				}
				fmt.Fprintf(ctx.Output, "%s}\n", prefix)
			} else {
				formatWidgetProps(ctx.Output, prefix, header, props, "\n")
			}
		} else if widgetType == "gallery" {
			// Handle Gallery specially with datasource, selection, filter and content widgets
			header := fmt.Sprintf("gallery %s", mdlIdent(w.Name))
			props := []string{}
			props = appendWidgetDataSources(ctx, props, w)
			// Add column counts if non-default
			if w.DesktopColumns != "" && w.DesktopColumns != "1" {
				props = append(props, fmt.Sprintf("DesktopColumns: %s", w.DesktopColumns))
			}
			if w.TabletColumns != "" && w.TabletColumns != "1" {
				props = append(props, fmt.Sprintf("TabletColumns: %s", w.TabletColumns))
			}
			if w.PhoneColumns != "" && w.PhoneColumns != "1" {
				props = append(props, fmt.Sprintf("PhoneColumns: %s", w.PhoneColumns))
			}
			// Add Selection mode if specified
			if w.Selection != "" {
				props = append(props, fmt.Sprintf("Selection: %s", w.Selection))
			}
			// The row action and the named action slots, as on the data grid
			// (#842).
			if w.OnClickTrigger != "" {
				props = append(props, fmt.Sprintf("onClickTrigger: %s", w.OnClickTrigger))
			}
			if w.OnClick != "" {
				props = append(props, actionProp("onClick", w.OnClick))
			}
			props = appendNamedActionProps(props, w)
			// Every other setting that differs from its default, under the
			// widget's own key (pagination, pageSize, …).
			props = appendExplicitProps(ctx, props, w.ExplicitProperties)
			props = appendAppearanceProps(ctx, props, w)
			// Output filter and content widgets
			hasContent := len(w.Children) > 0 || len(w.FilterWidgets) > 0 ||
				len(w.ChildSlots) > 0 || len(w.OmittedProps) > 0
			if hasContent {
				formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
				outputDataContainerContext(ctx.Output, prefix+"  ", w.Name, w.EntityContext, true)
				writeOmittedPropsNote(ctx.Output, prefix+"  ", w.OmittedProps)
				// Output FILTER section if filter widgets present
				if len(w.FilterWidgets) > 0 {
					fmt.Fprintf(ctx.Output, "%s  filter {\n", prefix)
					for _, filter := range w.FilterWidgets {
						outputWidgetMDLV3(ctx, filter, indent+2)
					}
					fmt.Fprintf(ctx.Output, "%s  }\n", prefix)
				}
				// Output TEMPLATE section if content widgets present
				if len(w.Children) > 0 {
					fmt.Fprintf(ctx.Output, "%s  template {\n", prefix)
					for _, child := range w.Children {
						outputWidgetMDLV3(ctx, child, indent+2)
					}
					fmt.Fprintf(ctx.Output, "%s  }\n", prefix)
				}
				// The empty-state placeholder, and any other slot the gallery declares.
				outputChildSlots(ctx, w.ChildSlots, prefix+"  ", indent+1)
				fmt.Fprintf(ctx.Output, "%s}\n", prefix)
			} else {
				formatWidgetProps(ctx.Output, prefix, header, props, "\n")
			}
		} else if widgetType == "image" {
			header := fmt.Sprintf("image %s", mdlIdent(w.Name))
			props := describeImageWidgetProps(ctx, w)
			// appendAppearanceProps prints the visibility and editability; the
			// conditional settings appended a second time before it printed each
			// twice (#721 C).
			props = appendAppearanceProps(ctx, props, w)
			formatWidgetProps(ctx.Output, prefix, header, props, "\n")
		} else if ((len(w.ExplicitProperties) > 0 && !isKnownCustomWidgetType(widgetType)) || len(w.ObjectLists) > 0 || w.OnClick != "" ||
			(w.OnChange != "" && !isKnownCustomWidgetType(widgetType)) ||
			(len(w.NamedActions) > 0 && !isKnownCustomWidgetType(widgetType)) ||
			(len(w.ChildSlots) > 0 && !isKnownCustomWidgetType(widgetType)) || len(w.OmittedContainers) > 0) && w.WidgetID != "" {
			// Generic pluggable widget with explicit properties, object-list child
			// blocks (chart series/lines/scaleColors), and/or an onClick action.
			// The widget's own MDL name where that round-trips, else the
			// explicit id form. See pluggableWidgetHeader.
			//
			// OnChange, the named action slots and the child slots count only
			// for a widget without its own branch below: a combo box's branch
			// emits them itself (its custom content, ako/mxcli#1744), and routed
			// here it lost its Attribute: and CaptionAttribute: on describe.
			//
			// Child slots and omitted containers count towards "has content"
			// too: a widget whose only non-default content is a populated slot
			// took the bare branch below, which emits a head and no body — so
			// the slot's widgets, and even the note naming what could not be
			// reconstructed, were dropped from the description
			// (mendixlabs/mxcli#1057).
			header := pluggableWidgetHeader(ctx.GetWidgetRegistry(), w.WidgetID, w.Name)
			props := []string{}
			if w.Caption != "" {
				props = append(props, fmt.Sprintf("Label: %s", mdlQuote(ctx, w.Caption)))
			}
			// A pluggable widget's own datasource. Without this the branch
			// emitted every property EXCEPT the datasource, so a rewrite dropped
			// it — measured on a Studio Pro-authored File Uploader 2.5.0, whose
			// `associatedFiles` is a Forms$AssociationSource: a page at 0 errors
			// came back as CE0642 "Property 'Associated files' is required" plus
			// two CE1571, because the action's parameter loses its default once
			// the datasource is gone. The DESCRIBE text was byte-identical before
			// and after, so only mx check separated them (#956).
			props = appendWidgetDataSources(ctx, props, w)
			props = appendExplicitProps(ctx, props, w.ExplicitProperties)
			// onClick action (ledger #67 — reported on CustomChart)
			if w.OnClick != "" {
				props = append(props, actionProp("onClick", w.OnClick))
			}
			// OnChange too — a Slider/RangeSlider/StarRating reaches describe
			// through this branch, and its action slot is the only one it has.
			if w.OnChange != "" {
				props = append(props, actionProp("OnChange", w.OnChange))
			}
			props = appendNamedActionProps(props, w)
			props = appendAppearanceProps(ctx, props, w)
			if len(w.ObjectLists) == 0 && len(w.ChildSlots) == 0 && len(w.OmittedContainers) == 0 {
				formatWidgetProps(ctx.Output, prefix, header, props, "\n")
			} else if len(w.ObjectLists) == 0 {
				// Child slots and/or a gap to name, but no object lists.
				formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
				outputChildSlots(ctx, w.ChildSlots, prefix+"  ", indent+1)
				writeOmittedContainerNote(ctx.Output, prefix+"  ", w.OmittedContainers)
				fmt.Fprintf(ctx.Output, "%s}\n", prefix)
			} else {
				// Emit the widget with a body holding its object-list items.
				formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
				childPrefix := prefix + "  "
				for _, ol := range w.ObjectLists {
					for _, item := range ol.Items {
						// An object-list item is a WidgetObject with no name (#749).
						itemHeader := ol.Keyword
						itemProps := []string{}
						// An object-list item's datasource goes through the same
						// renderer as a widget's. This site used to have no type
						// switch at all, so a chart series bound to a microflow
						// described as `database from Module.TheMicroflow` (#941).
						itemProps = appendDataSourceProp(ctx, itemProps, item.DataSource)
						for _, p := range item.Props {
							if p.IsRef {
								// An action that renders as a note stands on its own line.
								itemProps = append(itemProps, actionProp(p.Key, p.Value))
							} else {
								itemProps = append(itemProps, fmt.Sprintf("%s: %s", p.Key, mdlQuote(ctx, p.Value)))
							}
						}
						// An item holding child widgets (an Accordion group's `content`
						// slot) needs a body, or the children have nowhere to go and the
						// description silently drops them on re-exec (#891).
						if len(item.Children) > 0 || len(item.Slots) > 0 {
							formatWidgetProps(ctx.Output, childPrefix, itemHeader, itemProps, " {\n")
							// The item's other slots as blocks (headercontent { … }),
							// then its body — CREATE routes each by keyword (#1453).
							outputChildSlots(ctx, item.Slots, childPrefix+"  ", indent+2)
							for _, child := range item.Children {
								outputWidgetMDLV3(ctx, child, indent+2)
							}
							fmt.Fprintf(ctx.Output, "%s}\n", childPrefix)
							continue
						}
						formatNamelessProps(ctx.Output, childPrefix, itemHeader, itemProps)
					}
				}
				// A widget can carry both kinds of container — HTML Element has
				// `attributes`/`events` AND `tagContentContainer` — so the slots
				// belong in this branch too, not only the one above.
				outputChildSlots(ctx, w.ChildSlots, childPrefix, indent+1)
				writeOmittedContainerNote(ctx.Output, childPrefix, w.OmittedContainers)
				fmt.Fprintf(ctx.Output, "%s}\n", prefix)
			}
		} else {
			header := fmt.Sprintf("%s %s", widgetType, mdlIdent(w.Name))
			props := []string{}
			if w.Caption != "" {
				props = append(props, fmt.Sprintf("Label: %s", mdlQuote(ctx, w.Caption)))
			}
			if w.Content != "" {
				props = append(props, fmt.Sprintf("Attribute: %s", w.Content))
			}
			// Show DataSource and CaptionAttribute for the association modes.
			// The drop-down filter's ref mode has the same three parts as the
			// ComboBox's (reference + option list + caption), so it re-emits
			// through the same branch — without it the filter described back as a
			// bare `dropdownfilter name` and the mode was lost on re-exec (#830).
			if (w.DataSource != nil || len(w.NamedDataSources) > 0) &&
				(widgetType == "combobox" || widgetType == "dropdownfilter") {
				props = appendWidgetDataSources(ctx, props, w)
				if w.CaptionAttribute != "" {
					props = append(props, fmt.Sprintf("CaptionAttribute: %s", w.CaptionAttribute))
				}
				// An expression caption has no MDL alias; its storage keys are the
				// spelling the explicit-property pass writes, as for any
				// pluggable-widget property (#664).
				if w.CaptionExpression != "" {
					props = append(props,
						"optionsSourceAssociationCaptionType: expression",
						fmt.Sprintf("optionsSourceAssociationCaptionExpression: %s", mdlQuote(ctx, w.CaptionExpression)))
				}
			}
			// A pluggable widget's on-change action (ComboBox `onChangeEvent`).
			// Emitted for the same reason as the built-in inputs above: without
			// it a describe→edit→exec cycle silently drops the action.
			if w.OnChange != "" {
				props = append(props, actionProp("OnChange", w.OnChange))
			}
			// A combo box's onEnterEvent / onLeaveEvent /
			// onChangeFilterInputEvent, by key (ako/mxcli#854).
			props = appendNamedActionProps(props, w)
			// Show filter attributes for filter widgets
			if len(w.FilterAttributes) > 0 {
				props = append(props, fmt.Sprintf("Attributes: [%s]", strings.Join(w.FilterAttributes, ", ")))
			}
			// Show filter expression if not default
			if w.FilterExpression != "" && w.FilterExpression != "contains" {
				props = append(props, fmt.Sprintf("FilterType: %s", w.FilterExpression))
			}
			// The settings with no keyword, by the widget's own key
			// (mendixlabs/mxcli#1586), and a note for those not reproduced.
			props = appendExplicitProps(ctx, props, w.ExplicitProperties)
			props = appendAppearanceProps(ctx, props, w)
			writeOmittedPropsNote(ctx.Output, prefix, w.OmittedProps)
			writeHiddenContentNote(ctx.Output, prefix, w.Name, w.HiddenContent)
			if len(w.ChildSlots) == 0 {
				formatWidgetProps(ctx.Output, prefix, header, props, "\n")
			} else {
				// A combo box's custom content (ako/mxcli#1744), as the block
				// that writes it.
				formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
				outputChildSlots(ctx, w.ChildSlots, prefix+"  ", indent+1)
				fmt.Fprintf(ctx.Output, "%s}\n", prefix)
			}
		}

	case "Forms$NavigationList", "Pages$NavigationList":
		fmt.Fprintf(ctx.Output, "%snavigationlist %s {\n", prefix, mdlIdent(w.Name))
		for _, child := range w.Children {
			itemHeader := "item" // Studio Pro leaves an item unnamed
			if child.Name != "" {
				itemHeader += " " + mdlIdent(child.Name)
			}
			props := []string{}
			if child.Action != "" {
				props = append(props, actionProp("Action", child.Action))
			}
			if child.Class != "" {
				props = append(props, fmt.Sprintf("Class: %s", mdlQuote(ctx, child.Class)))
			}
			if child.Style != "" {
				props = append(props, fmt.Sprintf("Style: %s", mdlQuote(ctx, child.Style)))
			}
			if child.DynamicClasses != "" {
				props = append(props, fmt.Sprintf("DynamicClasses: %s", child.DynamicClasses)) // an expression, printed as-is
			}
			if child.VisibleIf != "" {
				props = append(props, widgetConditionMDL("Visible", child.VisibleIf))
			}
			if prop := visibleWhenProp(child); prop != "" {
				props = append(props, prop)
			}
			if prop := visibleRolesProp(child); prop != "" {
				props = append(props, prop)
			}
			formatWidgetProps(ctx.Output, prefix+"  ", itemHeader, props, " {\n")
			for _, cw := range child.Children {
				outputWidgetMDLV3(ctx, cw, indent+2)
			}
			fmt.Fprintf(ctx.Output, "%s  }\n", prefix)
		}
		fmt.Fprintf(ctx.Output, "%s}\n", prefix)

	case "Forms$Label", "Pages$Label":
		// Studio Pro's Label widget. This used to print `statictext (Content: …)`
		// — no name, so the output did not parse, and `statictext` writes
		// Forms$Text, which Mendix 11 cannot load (MDL-WIDGET29). Measured on
		// Administration.Account_Edit and FeedbackModule.ShareFeedback(_Logo):
		// each Forms$Label there is named (`label4`), so the name was dropped
		// here, not missing in the model.
		header := fmt.Sprintf("label %s", mdlIdent(w.Name))
		props := []string{}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Content: %s", mdlQuote(ctx, w.Content)))
		}
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$Gallery", "Pages$Gallery":
		header := fmt.Sprintf("gallery %s", mdlIdent(w.Name))
		props := []string{}
		props = appendWidgetDataSources(ctx, props, w)
		props = appendAppearanceProps(ctx, props, w)
		if len(w.Children) > 0 {
			formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
			outputDataContainerContext(ctx.Output, prefix+"  ", w.Name, w.EntityContext, true)
			for _, child := range w.Children {
				outputWidgetMDLV3(ctx, child, indent+1)
			}
			fmt.Fprintf(ctx.Output, "%s}\n", prefix)
		} else {
			formatWidgetProps(ctx.Output, prefix, header, props, "\n")
		}

	case "Forms$StaticImageViewer", "Pages$StaticImageViewer":
		// The `staticimage` keyword the executor has always dispatched, now with
		// the one thing MDL had no spelling for: WHICH image it shows. Both
		// halves are load-bearing — emitting the keyword without the reference
		// turns a visible "NOT re-executable" note into a silent drop, which is
		// what #512 and mxcli-formula1 FINDINGS §142 each cost a round to learn
		// (mendixlabs/mxcli#1057).
		header := fmt.Sprintf("staticimage %s", mdlIdent(w.Name))
		props := []string{}
		if w.ImageObject != "" {
			props = append(props, fmt.Sprintf("Image: %s", mdlQuote(ctx, w.ImageObject)))
		}
		if w.ImageWidth != "" {
			props = append(props, fmt.Sprintf("Width: %s", w.ImageWidth))
		}
		if w.WidthUnit != "" && w.WidthUnit != "auto" {
			props = append(props, fmt.Sprintf("WidthUnit: %s", w.WidthUnit))
		}
		if w.ImageHeight != "" {
			props = append(props, fmt.Sprintf("Height: %s", w.ImageHeight))
		}
		if w.HeightUnit != "" && w.HeightUnit != "auto" {
			props = append(props, fmt.Sprintf("HeightUnit: %s", w.HeightUnit))
		}
		// Only the non-default is emitted: Responsive is true unless it was
		// switched off, and printing a value the writer re-derives is the
		// "invents" half of the describe failure class.
		if w.Responsive == "false" {
			props = append(props, "Responsive: false")
		}
		if w.Action != "" {
			props = append(props, actionProp("Action", w.Action))
		}
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$ImageViewer", "Pages$ImageViewer":
		// The DYNAMIC image's sibling case. Property names are shared with the
		// static and the pluggable image on purpose — Width/Height, the units,
		// Responsive, DisplayAs, OnClickType — so one spelling means one thing
		// across all three. `DefaultImage:` is its own, because the fallback is
		// a different property from the image a static viewer shows.
		header := fmt.Sprintf("dynamicimage %s", mdlIdent(w.Name))
		props := []string{}
		props = appendWidgetDataSources(ctx, props, w)
		if w.DefaultImage != "" {
			props = append(props, fmt.Sprintf("DefaultImage: %s", mdlQuote(ctx, w.DefaultImage)))
		}
		if w.ImageWidth != "" {
			props = append(props, fmt.Sprintf("Width: %s", w.ImageWidth))
		}
		if w.WidthUnit != "" && w.WidthUnit != "auto" {
			props = append(props, fmt.Sprintf("WidthUnit: %s", w.WidthUnit))
		}
		if w.ImageHeight != "" {
			props = append(props, fmt.Sprintf("Height: %s", w.ImageHeight))
		}
		if w.HeightUnit != "" && w.HeightUnit != "auto" {
			props = append(props, fmt.Sprintf("HeightUnit: %s", w.HeightUnit))
		}
		if w.Responsive == "false" {
			props = append(props, "Responsive: false")
		}
		// Only the non-defaults: full size and no enlarge are Mendix's own, and
		// the writer re-derives them.
		if w.DisplayAs == "thumbnail" {
			props = append(props, "DisplayAs: thumbnail")
		}
		if w.OnClickType == "enlarge" {
			props = append(props, "OnClickType: enlarge")
		}
		if w.Action != "" {
			props = append(props, actionProp("Action", w.Action))
		}
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$SnippetCallWidget", "Pages$SnippetCallWidget":
		header := fmt.Sprintf("snippetcall %s", mdlIdent(w.Name))
		props := []string{}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Snippet: %s", w.Content))
		}
		if w.SnippetParams != "" {
			props = append(props, fmt.Sprintf("Params: (%s)", w.SnippetParams))
		}
		props = appendAppearanceProps(ctx, props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$ListViewTemplate":
		// A List View specialization template. It has no name — the entity it
		// renders is what identifies it — so the header is `template for <Entity>`
		// rather than the `template <name>` a Gallery content slot uses.
		fmt.Fprintf(ctx.Output, "%stemplate for %s {\n", prefix, w.Specialization)
		for _, child := range w.Children {
			outputWidgetMDLV3(ctx, child, indent+1)
		}
		fmt.Fprintf(ctx.Output, "%s}\n", prefix)

	case "Footer":
		if w.Name == "" {
			fmt.Fprintf(ctx.Output, "%sfooter {\n", prefix)
		} else {
			fmt.Fprintf(ctx.Output, "%sfooter %s {\n", prefix, mdlIdent(w.Name))
		}
		for _, child := range w.Children {
			outputWidgetMDLV3(ctx, child, indent+1)
		}
		fmt.Fprintf(ctx.Output, "%s}\n", prefix)

	case "Forms$ListView", "Pages$ListView":
		// ListView (also used for Gallery serialization)
		header := fmt.Sprintf("listview %s", mdlIdent(w.Name))
		props := []string{}
		props = appendWidgetDataSources(ctx, props, w)
		// Emit a non-default PageSize so it round-trips (Studio Pro's default is 20).
		if w.PageSize != "" && w.PageSize != "20" {
			props = append(props, fmt.Sprintf("PageSize: %s", w.PageSize))
		}
		// Pages$ListView.ClickAction. Written since ako/mxcli#512; without this
		// the action is dropped on the next describe -> exec, which is the
		// half-shell trap: valid BSON, clean build, construct silently gone.
		if w.Action != "" {
			props = append(props, actionProp("Action", w.Action))
		}
		props = appendAppearanceProps(ctx, props, w)
		if len(w.Children) > 0 {
			formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
			outputDataContainerContext(ctx.Output, prefix+"  ", w.Name, w.EntityContext, true)
			for _, child := range w.Children {
				outputWidgetMDLV3(ctx, child, indent+1)
			}
			fmt.Fprintf(ctx.Output, "%s}\n", prefix)
		} else {
			formatWidgetProps(ctx.Output, prefix, header, props, "\n")
		}

	case "Forms$DataGrid":
		outputLegacyGridV3(ctx, prefix, w)

	default:
		// A widget MDL cannot spell. Emitted as a comment naming it, and
		// labelled with what that costs: describe output is re-executable, so a
		// commented widget is one the round trip drops. Measured on
		// Atlas_Core.Atlas_SideBar, whose two Forms$SidebarToggleButton widgets
		// do not survive describe -> exec — silent without this note, because a
		// bare `-- Forms$X (name)` line reads as informational.
		describeDroppedWidget(ctx, w)
		fmt.Fprintf(ctx.Output, "%s-- %s", prefix, w.Type)
		if w.Name != "" {
			fmt.Fprintf(ctx.Output, " (%s)", w.Name)
		}
		fmt.Fprint(ctx.Output, "  -- NOT re-executable: mxcli cannot author this widget, so re-running this script would drop it\n")
		for _, c := range w.TableCells {
			contents := "(empty)"
			if len(c.Widgets) > 0 {
				contents = strings.Join(c.Widgets, ", ")
			}
			fmt.Fprintf(ctx.Output, "%s--   cell(%d,%d): %s\n", prefix, c.Row, c.Column, contents)
		}
	}
}

// outputDataGrid2ColumnV3 outputs a single DataGrid2 column in V3 MDL syntax.
func outputDataGrid2ColumnV3(ctx *ExecContext, prefix string, col rawDataGridColumn) {
	// Build the main column properties
	var props []string
	if col.Attribute != "" {
		props = append(props, fmt.Sprintf("Attribute: %s", col.Attribute))
	}
	if col.Caption != "" {
		props = append(props, fmt.Sprintf("Caption: %s", mdlQuote(ctx, col.Caption)))
	} else if col.Attribute == "" {
		// An untitled actions column (mendixlabs/mxcli#1520): say the empty
		// header outright rather than leave the column with no identifying
		// property. The writer stores `Caption: ''` as the empty template
		// Studio Pro writes — the shape that was read here.
		props = append(props, "Caption: ''")
	}
	if len(col.CaptionParams) > 0 {
		props = append(props, fmt.Sprintf("CaptionParams: (%s)", strings.Join(formatParametersV3(col.CaptionParams), ", ")))
	}
	// Add ShowContentAs if not default "attribute"
	if col.ShowContentAs != "" && col.ShowContentAs != "attribute" {
		props = append(props, fmt.Sprintf("ShowContentAs: %s", col.ShowContentAs))
	}
	// Add DynamicText content when ShowContentAs is dynamicText
	if col.ShowContentAs == "dynamicText" && col.DynamicText != "" {
		props = append(props, fmt.Sprintf("Content: %s", mdlQuote(ctx, col.DynamicText)))
		if len(col.DynamicTextParams) > 0 {
			props = append(props, fmt.Sprintf("ContentParams: (%s)", strings.Join(formatParametersV3(col.DynamicTextParams), ", ")))
		}
	}
	// Add column styling properties if non-default
	if col.Alignment != "" && col.Alignment != "left" {
		props = append(props, fmt.Sprintf("Alignment: %s", col.Alignment))
	}
	if col.WrapText == "true" {
		props = append(props, "WrapText: true")
	}
	// Sortable: default depends on whether attribute is bound
	if col.Sortable != "" {
		defaultSortable := "true"
		if col.Attribute == "" {
			defaultSortable = "false"
		}
		if col.Sortable != defaultSortable {
			props = append(props, fmt.Sprintf("Sortable: %s", col.Sortable))
		}
	}
	if col.Resizable == "false" {
		props = append(props, "Resizable: false")
	}
	if col.Draggable == "false" {
		props = append(props, "Draggable: false")
	}
	if col.Hidable != "" && col.Hidable != "yes" {
		props = append(props, fmt.Sprintf("Hidable: %s", col.Hidable))
	}
	if col.ColumnWidth != "" && col.ColumnWidth != "autoFill" {
		props = append(props, fmt.Sprintf("ColumnWidth: %s", col.ColumnWidth))
	}
	if col.ColumnWidth == "manual" && col.Size != "" && col.Size != "1" {
		props = append(props, fmt.Sprintf("Size: %s", col.Size))
	}
	if col.Visible != "" && col.Visible != "true" {
		// The expression as written, as for a page widget's conditional
		// visibility; `false` is the plain value, which a column reads as the
		// expression false. The quoted text this printed before was the
		// one spelling a column's Visible did not drop.
		if strings.EqualFold(strings.TrimSpace(col.Visible), "false") {
			props = append(props, "Visible: false")
		} else {
			props = append(props, widgetConditionMDL("Visible", col.Visible))
		}
	}
	if col.DynamicCellClass != "" {
		props = append(props, fmt.Sprintf("DynamicCellClass: %s", col.DynamicCellClass)) // an expression, printed as-is
	}
	if col.Tooltip != "" {
		props = append(props, fmt.Sprintf("Tooltip: %s", mdlQuote(ctx, col.Tooltip)))
	}
	// Any other column setting that differs from its default, by its own key.
	props = appendExplicitProps(ctx, props, col.ExtraProps)

	// No name: Mendix stores none on a DataGrid 2 column (#749).
	header := "column"
	// A column body carries the `content` slot's widgets AND the `filter` slot's;
	// the builder routes a filter back to its own slot by widget type. Emitting
	// only the content widgets deleted the filter of every custom-content column
	// on a describe→exec round trip (ako/mxcli#489).
	hasContent := len(col.ContentWidgets) > 0 || len(col.FilterWidgets) > 0 || len(col.OmittedProps) > 0

	if hasContent {
		// Output column with content block
		formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
		writeOmittedPropsNote(ctx.Output, prefix+"  ", col.OmittedProps)
		for _, widget := range col.ContentWidgets {
			outputWidgetMDLV3(ctx, widget, len(prefix)/2+1)
		}
		for _, widget := range col.FilterWidgets {
			outputWidgetMDLV3(ctx, widget, len(prefix)/2+1)
		}
		fmt.Fprintf(ctx.Output, "%s}\n", prefix)
	} else {
		// Output simple column line
		formatNamelessProps(ctx.Output, prefix, header, props)
	}
}

// formatNamelessProps writes a body-less element that has no name, such as a
// DataGrid 2 column or a chart series. With no properties it writes `()`: a
// bare keyword would read the NEXT element's keyword as its name, since the
// name is optional and element keywords are also valid names
// (`column\ncolumn (…)` is one column named "column").
func formatNamelessProps(w io.Writer, prefix, header string, props []string) {
	if len(props) == 0 {
		fmt.Fprintf(w, "%s%s ()\n", prefix, header)
		return
	}
	formatWidgetProps(w, prefix, header, props, "\n")
}

func extractTextContent(ctx *ExecContext, w map[string]any, field string) string {
	content, ok := w[field].(map[string]any)
	if !ok {
		return ""
	}
	// Path: Content.Template.Items[] where Items contains Translation objects
	// Structure: Content -> Template -> Items -> [version, Translation{Text: "value"}]
	template, ok := content["Template"].(map[string]any)
	if !ok {
		return ""
	}
	// Select the project default language's translation (issue #702), not just
	// whichever Items entry happens to be first.
	items := getBsonArrayElements(template["Items"])
	return selectTranslationText(items, describeDefaultLanguage(ctx))
}

func extractButtonCaption(ctx *ExecContext, w map[string]any) string {
	// Try Caption first (legacy format)
	if caption := extractTextContent(ctx, w, "Caption"); caption != "" {
		return caption
	}
	// Try CaptionTemplate (modern format used by ActionButton)
	return extractTextContent(ctx, w, "CaptionTemplate")
}

// extractButtonCaptionParameters extracts parameters from ActionButton caption.
// Tries CaptionTemplate first (modern format), then Caption (legacy format).
func extractButtonCaptionParameters(ctx *ExecContext, w map[string]any) []string {
	// Try CaptionTemplate first (modern format used by ActionButton)
	if params := extractClientTemplateParameters(ctx, w, "CaptionTemplate"); params != nil {
		return params
	}
	// Fall back to Caption (legacy format)
	return extractClientTemplateParameters(ctx, w, "Caption")
}

func extractButtonStyle(ctx *ExecContext, w map[string]any) string {
	if style, ok := w["ButtonStyle"].(string); ok {
		return style
	}
	return "Default"
}

// extractButtonIcon reads a button's Icon element: the qualified name it points
// at, the stored $Type, and a glyph icon's Code. All three are needed, and the
// reason is in the storage (mendixlabs/mxcli#1059):
//
//	Forms$IconCollectionIcon{Image} -> CustomIcons$CustomIcon
//	Forms$ImageIcon{Image}          -> Images$Image   (a DIFFERENT document)
//	Forms$GlyphIcon{Code}           -> a font code point, with no name at all
//
// The two named variants are indistinguishable by their payload, so a reader
// that returns only `Image` hands the emitter a name it cannot place. That is
// what made an image icon re-execute as a custom-icon reference and fail the
// build with CE1613. Returning the $Type is what lets the emitter decline.
func extractButtonIcon(w map[string]any) (name, iconType string, code int) {
	icon, ok := w["Icon"].(map[string]any)
	if !ok {
		return "", "", 0
	}
	iconType, _ = icon["$Type"].(string)
	name, _ = icon["Image"].(string)
	return name, iconType, bsonInt(icon["Code"])
}

// widgetIconMDL renders the `Icon:` property for a widget, or "" when there is
// nothing CREATE PAGE can reproduce.
//
// All three of Mendix's icon elements have a form now. Only the collection one
// used to, so an image icon came out spelled exactly like a collection
// reference and re-executed as one (CE1613), and a glyph icon — which has no
// name at all — came out as nothing and was deleted on replay
// (mendixlabs/mxcli#1059).
//
// The kind word is what rebuilds the same ELEMENT rather than something that
// merely parses. The bare form stays the collection icon, so existing output
// still means what it did.
func widgetIconMDL(ctx *ExecContext, w rawWidget) string {
	switch types.MenuIconKindOf(w.IconType) {
	case types.MenuIconGlyph:
		// The code is the whole identity of a glyph. Without it there is nothing
		// to emit that would rebuild the same icon, so fall through to the note.
		if w.IconCode == 0 {
			return ""
		}
		return fmt.Sprintf("Icon: glyph %d", w.IconCode)
	case types.MenuIconImage:
		if w.Icon == "" {
			return ""
		}
		return fmt.Sprintf("Icon: image %s", mdlQuote(ctx, w.Icon))
	case types.MenuIconCollection:
		if w.Icon == "" {
			return ""
		}
		return fmt.Sprintf("Icon: %s", mdlQuote(ctx, w.Icon))
	}
	return ""
}

// widgetIconNote flags an icon DESCRIBE still cannot round-trip, so re-running
// the output loses it visibly rather than silently.
//
// CREATE OR REPLACE PAGE is a full replacement, which is what makes silence
// expensive: an icon the writer will not emit is an icon the replay DELETES,
// with an exit 0 and a success message.
//
// All three icon elements are reproducible now, so this fires only on what is
// genuinely beyond the language: a stored $Type this build does not know, or a
// variant whose payload is missing (a glyph with no Code, a named icon with no
// name) — where emitting a clause would rebuild a DIFFERENT icon rather than the
// same one. Reporting a variant this build does not recognise as "no icon" is
// how the next one Mendix adds would be dropped in turn.
func widgetIconNote(ctx *ExecContext, w rawWidget) string {
	if w.IconType == "" || widgetIconMDL(ctx, w) != "" {
		return ""
	}
	target := w.Icon
	if target == "" && w.IconCode != 0 {
		target = fmt.Sprintf("glyph %d", w.IconCode)
	}
	if target == "" {
		target = "(no reference stored)"
	}
	return fmt.Sprintf("-- NOT re-executable: icon %s (%s) — mxcli cannot author this "+
		"icon element, so re-running this script would drop it", target, w.IconType)
}

func extractButtonAction(ctx *ExecContext, w map[string]any) string {
	return renderClientActionMDL(ctx, actionMapForKey(w, "Action"))
}

// extractOnChangeAction renders a widget's OnChangeAction (input widgets) as MDL,
// reusing the same client-action renderer as buttons — OnChangeAction is the same
// client-action type, just under a different BSON key.
func extractOnChangeAction(ctx *ExecContext, w map[string]any) string {
	return renderClientActionMDL(ctx, actionMapForKey(w, "OnChangeAction"))
}

// extractFocusEventActions renders an input widget's OnEnterAction and
// OnLeaveAction ("On enter" / "On leave"). Unread, describe → exec replaced a
// stored one with the empty action (ako/mxcli#1743).
func extractFocusEventActions(ctx *ExecContext, w map[string]any) (string, string) {
	return renderClientActionMDL(ctx, actionMapForKey(w, "OnEnterAction")),
		renderClientActionMDL(ctx, actionMapForKey(w, "OnLeaveAction"))
}

// appendFocusEventProps prints an input widget's On enter / On leave actions,
// each only when it holds one.
func appendFocusEventProps(props []string, w rawWidget) []string {
	if w.OnEnter != "" {
		props = append(props, actionProp("OnEnter", w.OnEnter))
	}
	if w.OnLeave != "" {
		props = append(props, actionProp("OnLeave", w.OnLeave))
	}
	return props
}

// actionMapForKey unwraps a client-action node stored under key into a plain map
// (handling both map[string]any and bson.M), or nil when absent.
func actionMapForKey(w map[string]any, key string) map[string]any {
	if action, ok := w[key].(map[string]any); ok {
		return action
	}
	if actionM, ok := w[key].(bson.M); ok {
		return map[string]any(actionM)
	}
	return nil
}

// renderClientActionMDL renders a client-action map (a Forms$*ClientAction) back
// to its MDL form (call microflow/call nanoflow/show page/save changes/…). Returns "" for a
// nil action or a NoClientAction.
func renderClientActionMDL(ctx *ExecContext, action map[string]any) string {
	out := renderClientActionBaseMDL(ctx, action)
	if out == "" || strings.HasPrefix(out, "--") {
		return out
	}
	return out + renderClientActionSettingsMDL(ctx, action)
}

// renderClientActionBaseMDL renders the action itself; renderClientActionMDL
// adds its `with ( … )` settings.
// unsetTargetNoteTail ends every note unsetTargetNote writes, so a caller that
// says something else about such an action can tell it apart.
const unsetTargetNoteTail = "; MDL cannot name a missing target, so re-running this script drops the action"

// unsetTargetNote describes an action whose flow, page or entity is not
// selected yet. Studio Pro stores that state for a placeholder: its page
// templates ship them, and a page made from one inherits them until a target
// is picked. mx check reports each (CE0571 no nanoflow or microflow, CE0566 no
// page, CE0460 no entity). The bare `call nanoflow` describe used to print does
// not parse, so one unwired button made the whole page's description unusable
// (ako/mxcli#1728); a note, which actionProp puts on its own line, does.
func unsetTargetNote(what string) string {
	return "-- NOT re-executable: the action " + what + unsetTargetNoteTail
}

func renderClientActionBaseMDL(ctx *ExecContext, action map[string]any) string {
	if action == nil {
		return ""
	}
	typeName, _ := action["$Type"].(string)
	switch typeName {
	case "Forms$SaveChangesClientAction", "Pages$SaveChangesClientAction":
		result := "save changes"
		if closePage, ok := action["ClosePage"].(bool); ok && closePage {
			result += " close page"
		}
		return result
	case "Forms$CancelChangesClientAction", "Pages$CancelChangesClientAction":
		result := "cancel changes"
		if closePage, ok := action["ClosePage"].(bool); ok && closePage {
			result += " close page"
		}
		return result
	case "Forms$ClosePageClientAction", "Pages$ClosePageClientAction":
		return "close page"
	case "Forms$DeleteClientAction", "Pages$DeleteClientAction":
		result := "delete"
		// A grid's selection (a control-bar button) or a page/snippet
		// parameter; null is the context object (ako/mxcli#1932).
		if src := flowArgVariableValue(action["SourceVariable"]); src != "" {
			result += " " + src
		}
		if closePage, ok := action["ClosePage"].(bool); ok && closePage {
			result += " close page"
		}
		return result
	case "Forms$CreateObjectClientAction", "Pages$CreateObjectClientAction":
		entityName := ""
		if entityRef, ok := action["EntityRef"].(map[string]any); ok {
			entityName, _ = entityRef["Entity"].(string)
		}
		if entityName == "" {
			return unsetTargetNote("creates an object but no entity is selected (CE0460)")
		}
		result := "create object " + entityName
		// Extract page reference from PageSettings (Forms$FormSettings)
		if pageSettings, ok := action["PageSettings"].(map[string]any); ok {
			// The page is stored in "Form" field as a qualified name string (BY_NAME_REFERENCE)
			if pageName, ok := pageSettings["Form"].(string); ok && pageName != "" {
				pageAction := "show page " + pageName
				// Extract page parameters
				params := extractPageParameters(ctx, pageSettings)
				if params != "" {
					pageAction += "(" + params + ")"
				}
				result += " then " + pageAction
			}
		}
		return result
	case "Forms$FormAction", "Pages$FormAction":
		// SHOW_PAGE action - page reference is in FormSettings.Form (string name)
		// or PageSettings.Form, or Page field (binary ID for legacy)
		if formSettings, ok := action["FormSettings"].(map[string]any); ok {
			if pageName, ok := formSettings["Form"].(string); ok && pageName != "" {
				result := "show page " + pageName
				params := pageActionParameters(ctx, formSettings, pageName)
				if params != "" {
					result += "(" + params + ")"
				}
				return result
			}
		}
		if pageSettings, ok := action["PageSettings"].(map[string]any); ok {
			if pageName, ok := pageSettings["Form"].(string); ok && pageName != "" {
				result := "show page " + pageName
				params := pageActionParameters(ctx, pageSettings, pageName)
				if params != "" {
					result += "(" + params + ")"
				}
				return result
			}
		}
		// Fall back to Page field (binary ID from legacy serialization)
		if pageID := extractBinaryID(action["Page"]); pageID != "" {
			pageName := getPageQualifiedName(ctx, model.ID(pageID))
			if pageName != "" {
				return "show page " + pageName
			}
		}
		return unsetTargetNote("shows a page but none is selected (CE0566)")
	case "Forms$MicroflowAction", "Pages$MicroflowClientAction":
		// Extract microflow reference from MicroflowSettings
		if settings, ok := action["MicroflowSettings"].(map[string]any); ok {
			if mfName, ok := settings["Microflow"].(string); ok && mfName != "" {
				result := "call microflow " + mfName
				// Extract parameter mappings
				params := extractMicroflowParameters(ctx, settings)
				if params != "" {
					result += "(" + params + ")"
				}
				if returns, ok := storedFlowReturns(settings); ok {
					result += returns
				}
				return result
			}
		}
		return unsetTargetNote("calls a microflow but none is selected (CE0571)")
	case "Forms$CallNanoflowClientAction", "Pages$CallNanoflowClientAction":
		if nfName, ok := action["Nanoflow"].(string); ok && nfName != "" {
			result := "call nanoflow " + nfName
			// Extract parameter mappings (directly in the action)
			params := extractNanoflowParameters(ctx, action)
			if params != "" {
				result += "(" + params + ")"
			}
			if returns, ok := storedFlowReturns(action); ok {
				result += returns
			}
			return result
		}
		return unsetTargetNote("calls a nanoflow but none is selected (CE0571)")
	case "Forms$SetTaskOutcomeClientAction", "Pages$SetTaskOutcomeClientAction":
		outcomeValue, _ := action["OutcomeValue"].(string)
		return "complete task '" + strings.ReplaceAll(outcomeValue, "'", "''") + "'"
	// The workflow events beside it printed nothing, and a rewrite then wrote
	// a Forms$NoAction in their place (ako/mxcli#832).
	case "Forms$OpenUserTaskClientAction", "Pages$OpenUserTaskClientAction":
		return "open user task"
	case "Forms$OpenWorkflowClientAction", "Pages$OpenWorkflowClientAction":
		// The page is required (CE1834), so `open workflow` without one does
		// not parse; settings print it.
		if page, _ := action["DefaultPage"].(string); page == "" {
			return unsetTargetNote("shows a workflow admin page but no default page is selected (CE1834)")
		}
		return "open workflow"
	case "Forms$CallWorkflowClientAction", "Pages$CallWorkflowClientAction":
		if wf, _ := action["Workflow"].(string); wf != "" {
			return "call workflow " + wf
		}
		return unsetTargetNote("calls a workflow but none is selected")
	case "Forms$SignOutClientAction", "Pages$SignOutClientAction":
		return "sign out"
	case "Forms$OpenLinkClientAction", "Pages$OpenLinkClientAction":
		// The address is a nested Forms$StaticOrDynamicString: a literal, or —
		// DYNAMIC, 6 of the 31 Studio Pro references — an attribute read at
		// runtime, spelled `open link $currentObject/Attr`. It used to render
		// as an inline `--` note, which left `Action:` without a value and made
		// the describe output unparseable.
		addr := actionMapForKey(action, "Address")
		if addr == nil {
			return "open link ''"
		}
		if isDynamic, _ := addr["IsDynamic"].(bool); isDynamic {
			attr := ""
			overAssociation := false
			if ref := actionMapForKey(addr, "AttributeRef"); ref != nil {
				attr, _ = ref["Attribute"].(string)
				overAssociation = actionMapForKey(ref, "EntityRef") != nil
			}
			if attr != "" && !overAssociation {
				return "open link $currentObject/" + shortAttributeName(attr)
			}
			// No MDL spelling: a note, which actionProp puts on its own line.
			// CREATE OR REPLACE PAGE rebuilds the page, so say plainly that
			// re-running drops the action rather than implying it survives.
			return "-- NOT re-executable: open link with a dynamic address over an association (" +
				attr + ") — re-running this script would drop the button's action"
		}
		value, _ := addr["Value"].(string)
		return "open link '" + strings.ReplaceAll(value, "'", "''") + "'"
	case "Forms$NoClientAction", "Pages$NoClientAction":
		return ""
	default:
		return ""
	}
}

// getPageQualifiedName resolves a page ID to its qualified name.
func getPageQualifiedName(ctx *ExecContext, pageID model.ID) string {
	if pageID == "" {
		return ""
	}
	// Headers, not decoded pages (#1850), and not the cached name map, which a
	// page created earlier in the same script is missing from.
	hs, err := pageHeaders(ctx)
	if err != nil {
		return ""
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return ""
	}
	for _, p := range hs {
		if p.ID == pageID {
			return h.GetQualifiedName(p.ContainerID, p.Name)
		}
	}
	return ""
}

// pageActionParameters renders a SHOW_PAGE action's argument list, recovering the
// implicit mapping when the model stores none.
//
// A page action written by mxcli deliberately stores ParameterMappings as an
// empty array: Studio Pro infers the current row object from the enclosing
// widget, and an explicit mapping whose Argument is "$currentObject" is rejected
// as CE0115 "parameters do not match" (issue #296). Nothing was wrong with that
// decision — what was missing is its other half. DESCRIBE read only the explicit
// mappings, so `SHOW_PAGE P(Race: $currentObject)` came back as `show_page P`,
// and the description read as a diagnosis: the mapping was dropped, that is why
// the page gets an empty object. It was not dropped, and Mendix could not have
// built the page if it were — an unmapped required page parameter is a
// consistency error. Three debugging cycles were spent replacing a button that
// was correct (mxcli-formula1 §39).
//
// DESCRIBE is what you reach for once you have stopped trusting the model, so a
// lossy DESCRIBE is costliest exactly when it is most used.
func pageActionParameters(ctx *ExecContext, settings map[string]any, pageName string) string {
	if explicit := extractPageParameters(ctx, settings); explicit != "" {
		return explicit
	}
	// No stored mapping: the target page's own required parameters are the
	// mapping, each bound to the row object the enclosing widget supplies. An
	// optional one (11.5+) with no mapping is simply not passed — a menu item
	// opens such a page with no argument, and inventing `= $currentObject`
	// there made the description one exec refuses (MDL-PAGEARG01,
	// mendixlabs/mxcli#1640).
	var params []string
	for _, name := range targetPageParameterNames(ctx, pageName) {
		params = append(params, visitor.ParameterNameSpelling(name)+" = $currentObject")
	}
	return strings.Join(params, ", ")
}

// targetPageParameterNames returns the required parameter names declared by a
// page, by qualified name. Returns nil when the page cannot be resolved — a
// description that omits an argument is better than one that invents a name.
func targetPageParameterNames(ctx *ExecContext, qualifiedName string) []string {
	module, name, ok := strings.Cut(qualifiedName, ".")
	if !ok || module == "" || name == "" {
		return nil
	}
	allPages, err := pagesWhere(ctx.Backend, func(n string) bool { return strings.EqualFold(n, name) })
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	for _, p := range allPages {
		if p == nil || !strings.EqualFold(p.Name, name) {
			continue
		}
		if !strings.EqualFold(h.GetModuleName(h.FindModuleID(p.ContainerID)), module) {
			continue
		}
		var names []string
		for _, param := range p.Parameters {
			if param != nil && param.Name != "" && param.IsRequired {
				names = append(names, param.Name)
			}
		}
		return names
	}
	return nil
}

// extractPageParameters extracts page parameter mappings from a FormSettings/PageSettings object.
// Returns formatted string like "Product = $currentObject" or empty string if no params.
func extractPageParameters(ctx *ExecContext, settings map[string]any) string {
	mappings := getBsonArrayElements(settings["ParameterMappings"])
	if len(mappings) == 0 {
		return ""
	}

	var params []string
	for _, mapping := range mappings {
		mappingMap, ok := mapping.(map[string]any)
		if !ok {
			continue
		}

		// Get parameter name from Parameter field (BY_NAME_REFERENCE: "PageName.ParamName")
		paramRef := extractString(mappingMap["Parameter"])
		if paramRef == "" {
			continue
		}
		// Extract just the parameter name (last part after the dot)
		parts := strings.Split(paramRef, ".")
		paramName := parts[len(parts)-1]
		if paramName == "" {
			continue
		}

		// Get the value - check for $currentObject (WidgetValue), Argument (variable or expression)
		value := ""

		// Check for WidgetValue (represents $currentObject in list widgets)
		if widgetVal, ok := mappingMap["WidgetValue"].(map[string]any); ok && widgetVal != nil {
			// $Type is Pages$WidgetValue or similar - this represents current row object
			if valType := extractString(widgetVal["$Type"]); valType != "" {
				value = "$currentObject"
			}
		}

		// Check for Argument (variable reference or expression stored as string)
		if value == "" {
			if arg := extractString(mappingMap["Argument"]); arg != "" {
				value = strings.TrimSpace(arg) // e.g., "$Product" or an expression
			}
		}

		// Check for a Forms$PageVariable binding. One naming only a widget is a
		// control-bar button passing its grid's selection (mendixlabs/mxcli#122),
		// written `$grid`.
		if value == "" {
			value = flowArgVariableValue(mappingMap["Variable"])
		}

		if value != "" {
			params = append(params, visitor.ParameterNameSpelling(paramName)+" = "+value)
		}
	}

	return strings.Join(params, ", ")
}

// extractMicroflowParameters extracts microflow parameter mappings from a MicroflowSettings object.
// Returns formatted string like "Product = $currentObject" or empty string if no params.
func extractMicroflowParameters(ctx *ExecContext, settings map[string]any) string {
	mappings := getBsonArrayElements(settings["ParameterMappings"])
	if len(mappings) == 0 {
		return ""
	}

	var params []string
	for _, mapping := range mappings {
		mappingMap, ok := mapping.(map[string]any)
		if !ok {
			continue
		}

		// Get parameter name from Parameter field (BY_NAME_REFERENCE: "Module.Microflow.ParamName")
		paramRef := extractString(mappingMap["Parameter"])
		if paramRef == "" {
			continue
		}
		// Extract just the parameter name (last part after the dots)
		parts := strings.Split(paramRef, ".")
		paramName := parts[len(parts)-1]
		if paramName == "" {
			continue
		}

		// Get the value - check for $currentObject (WidgetValue), Expression, or Variable
		value := ""

		// Check for WidgetValue (represents $currentObject in list widgets)
		if widgetVal, ok := mappingMap["WidgetValue"].(map[string]any); ok && widgetVal != nil {
			if valType := extractString(widgetVal["$Type"]); valType != "" {
				value = "$currentObject"
			}
		}

		// Check for Expression (used in Pages$MicroflowParameterMapping)
		if value == "" {
			if expr := extractString(mappingMap["Expression"]); expr != "" {
				value = strings.TrimSpace(expr) // e.g., "$Product" or an expression
			}
		}

		// Check for a Forms$PageVariable binding.
		if value == "" {
			value = flowArgVariableValue(mappingMap["Variable"])
		}

		if value != "" {
			// Canonical microflowArgV3 form is `Param: $value` (IDENTIFIER COLON expr);
			// emitting `Param = $value` is IDENTIFIER EQUALS, which doesn't re-parse (#640).
			params = append(params, visitor.ParameterNameSpelling(paramName)+" = "+value)
		}
	}

	return strings.Join(params, ", ")
}

// extractNanoflowParameters extracts nanoflow parameter mappings from an action object.
// Returns formatted string like "Product = $currentObject" or empty string if no params.
func extractNanoflowParameters(ctx *ExecContext, action map[string]any) string {
	mappings := getBsonArrayElements(action["ParameterMappings"])
	if len(mappings) == 0 {
		return ""
	}

	var params []string
	for _, mapping := range mappings {
		mappingMap, ok := mapping.(map[string]any)
		if !ok {
			continue
		}

		// Get parameter name from Parameter field (BY_NAME_REFERENCE: "Module.Nanoflow.ParamName")
		paramRef := extractString(mappingMap["Parameter"])
		if paramRef == "" {
			continue
		}
		// Extract just the parameter name (last part after the dots)
		parts := strings.Split(paramRef, ".")
		paramName := parts[len(parts)-1]
		if paramName == "" {
			continue
		}

		// Get the value - check for $currentObject (WidgetValue), Expression, or Variable
		value := ""

		// Check for WidgetValue (represents $currentObject in list widgets)
		if widgetVal, ok := mappingMap["WidgetValue"].(map[string]any); ok && widgetVal != nil {
			if valType := extractString(widgetVal["$Type"]); valType != "" {
				value = "$currentObject"
			}
		}

		// Check for Expression (used in Pages$NanoflowParameterMapping)
		if value == "" {
			if expr := extractString(mappingMap["Expression"]); expr != "" {
				value = strings.TrimSpace(expr) // e.g., "$Product" or an expression
			}
		}

		// Check for a Forms$PageVariable binding.
		if value == "" {
			value = flowArgVariableValue(mappingMap["Variable"])
		}

		if value != "" {
			// Canonical microflowArgV3 form is `Param: $value` (IDENTIFIER COLON expr);
			// emitting `Param = $value` is IDENTIFIER EQUALS, which doesn't re-parse (#640).
			params = append(params, visitor.ParameterNameSpelling(paramName)+" = "+value)
		}
	}

	return strings.Join(params, ", ")
}

func extractTextCaption(ctx *ExecContext, w map[string]any) string {
	return extractTextsText(ctx, w, "Caption")
}

// extractTextsText reads the default-language translation of the Texts$Text
// stored under field — a widget's Caption, or an action button's Tooltip.
func extractTextsText(ctx *ExecContext, w map[string]any, field string) string {
	text, ok := w[field].(map[string]any)
	if !ok {
		return ""
	}
	items := getBsonArrayElements(text["Items"])
	return selectTranslationText(items, describeDefaultLanguage(ctx))
}

// extractClientTemplateParameters extracts parameter values from a ClientTemplate field (Content or Caption).
//
// One reader for every stored Forms$ClientTemplateParameter: the pluggable
// describer's extractTextTemplateParameters. This used to be a second copy of
// that loop, and the two drifted twice — a local-variable binding read here as
// `<unbound>` (upstream #977), and an association path was read only here, so a
// pluggable widget's `{1} = Assoc/Attr` described as the bare attribute
// (mendixlabs/mxcli#1494).
func extractClientTemplateParameters(ctx *ExecContext, w map[string]any, fieldName string) []string {
	template, ok := w[fieldName].(map[string]any)
	if !ok {
		return nil
	}
	return extractTextTemplateParameters(ctx, template)
}

// formatParamFormatSuffix renders a dynamic-text parameter's FormattingInfo back
// to the MDL format block " (decimalPrecision: 2, groupDigits: true, …)",
// emitting only the fields that differ from the Mendix defaults (DateFormat=Date,
// DecimalPrecision=2, EnumFormat=Text, GroupDigits=false, CustomDateFormat=""),
// so an unformatted parameter round-trips as before (empty string). Mirrors the
// writer defaults in formattingInfoFromParamFormat / formattingInfoToGen.
func formatParamFormatSuffix(ctx *ExecContext, pMap map[string]any) string {
	fi, ok := pMap["FormattingInfo"].(map[string]any)
	if !ok || fi == nil {
		return ""
	}
	var parts []string
	if dp := extractInt(fi["DecimalPrecision"]); dp != 2 {
		parts = append(parts, fmt.Sprintf("decimalPrecision: %d", dp))
	}
	if gd, ok := fi["GroupDigits"].(bool); ok && gd {
		parts = append(parts, "groupDigits: true")
	}
	if df := extractString(fi["DateFormat"]); df != "" && df != "Date" {
		parts = append(parts, "dateFormat: "+df)
	}
	if cdf := extractString(fi["CustomDateFormat"]); cdf != "" {
		// Quoted like every other string: a Java date pattern quotes literal
		// text with apostrophes (ako/mxcli#977).
		parts = append(parts, "customDateFormat: "+mdlQuote(ctx, cdf))
	}
	if ef := extractString(fi["EnumFormat"]); ef != "" && ef != "Text" {
		parts = append(parts, "enumFormat: "+ef)
	}
	if len(parts) == 0 {
		return ""
	}
	return " format (" + strings.Join(parts, ", ") + ")"
}

// associationTemplateParamPath reconstructs the "Assoc/.../Attr" navigation of a
// template parameter whose AttributeRef binds an attribute over one or more
// associations (AttributeRef.EntityRef = DomainModels$IndirectEntityRef). Returns
// "" when the parameter is a plain (direct-attribute) binding.
func associationTemplateParamPath(ctx *ExecContext, attrRef map[string]any) string {
	return associationPathMDL(attrRef, false, describeAssociationEnds(ctx))
}

// associationEndFunc answers whether entity is one of the association's two
// ends; known is false when the association cannot be looked up.
type associationEndFunc func(assocQN, entityQN string) (isEnd, known bool)

// associationPathMDL renders an IndirectEntityRef attribute binding as
// "Assoc/…/Attr" (shortAssoc drops each association's module), or "" when the
// binding is not over associations.
//
// A step whose DestinationEntity is not an end of its association is a cast to
// a specialization: Studio Pro stores Administration.Account after
// System.WorkflowUserTaskOutcome_User, whose end is System.User (TestApp
// WorkflowCommons.Snip_WorkflowUserTaskView_Details). It prints as
// "Assoc/Module.Entity/…", the Mendix expression spelling the builder reads
// back; dropped, the path re-executed against the association's end and the
// attribute was CE1613 "The selected attribute 'System.User.FullName' no
// longer exists". isEnd nil (no project) prints the associations only.
func associationPathMDL(attrRef map[string]any, shortAssoc bool, isEnd associationEndFunc) string {
	entityRef, ok := attrRef["EntityRef"].(map[string]any)
	if !ok || entityRef == nil || extractString(entityRef["$Type"]) != "DomainModels$IndirectEntityRef" {
		return ""
	}
	steps := getBsonArrayElements(entityRef["Steps"])
	if len(steps) == 0 {
		return ""
	}
	segs := make([]string, 0, len(steps))
	for _, st := range steps {
		sm, ok := st.(map[string]any)
		if !ok {
			return ""
		}
		assoc := extractString(sm["Association"])
		if assoc == "" {
			return ""
		}
		if shortAssoc {
			segs = append(segs, shortAttributeName(assoc))
		} else {
			segs = append(segs, assoc)
		}
		if dest := extractString(sm["DestinationEntity"]); dest != "" && isEnd != nil {
			if end, known := isEnd(assoc, dest); known && !end {
				segs = append(segs, dest)
			}
		}
	}
	attr := shortAttributeName(extractString(attrRef["Attribute"]))
	if attr == "" {
		return ""
	}
	return strings.Join(segs, "/") + "/" + attr
}

// describeAssociationEnds is the association-end lookup describe uses: the
// page builder's own, so describe and exec resolve an association one way.
func describeAssociationEnds(ctx *ExecContext) associationEndFunc {
	if ctx == nil || ctx.Backend == nil || !ctx.Connected() {
		return nil
	}
	if ctx.Cache == nil {
		ctx.Cache = &executorCache{}
	}
	pb := &pageBuilder{backend: ctx.Backend, execCache: ctx.Cache}
	return pb.associationHasEnd
}

// associationDataSourceExpr renders an association data source as the MDL
// navigation expression `$currentObject/Module.Assoc` (or `$Param/…`). The
// executor re-resolves the destination entity on re-parse, so the emitted form
// carries only the association path.
func associationDataSourceExpr(ds *rawDataSource) string {
	ctx := ds.ContextVariable
	if ctx == "" {
		ctx = "currentObject"
	}
	return "$" + ctx + "/" + ds.Reference
}

func (e *Executor) outputWidgetMDLV3(w rawWidget, indent int) {
	outputWidgetMDLV3(e.newExecContext(context.Background()), w, indent)
}

// dataViewHasFooterBlock reports whether describe will emit a `footer { … }` child
// for this DataView, which by itself implies ShowFooter: true on re-exec.
func dataViewHasFooterBlock(w rawWidget) bool {
	for _, child := range w.Children {
		if child.Type == "Footer" {
			return true
		}
	}
	return false
}

// appendNamedActionProps emits the widget's action slots that MDL addresses by
// the widget's own property key: `createFileAction: microflow Module.Flow`. The
// click and change slots are not among them — they have MDL names and are
// emitted as `onClick:` / `OnChange:` (#956).
func appendNamedActionProps(props []string, w rawWidget) []string {
	for _, na := range w.NamedActions {
		props = append(props, actionProp(na.Key, na.MDL))
	}
	return props
}

// describeImageWidgetProps renders an image widget's own properties, separated
// from the output loop so the round trip can be asserted directly.
//
// `Image:` is the one §142 turns on: it names the image collection entry the
// widget shows, and until the `image` operation existed there was nothing to
// emit, so describe -> exec copied an Atlas layout and lost its brand image. An
// ABSENT entry emits nothing rather than an empty `Image: ”`, which would
// re-execute into a reference to nothing.
func describeImageWidgetProps(ctx *ExecContext, w rawWidget) []string {
	props := []string{}
	if w.ImageType != "" && w.ImageType != "image" {
		props = append(props, fmt.Sprintf("ImageType: %s", w.ImageType))
	}
	switch {
	case w.ImageFromObject:
		// Bound to the object in context, unquoted: a quoted value is a
		// collection entry's name (#1476).
		props = append(props, "Image: "+imageObjectVariable)
	case w.ImageObject != "":
		props = append(props, fmt.Sprintf("Image: %s", mdlQuote(ctx, w.ImageObject)))
	}
	if w.DefaultImage != "" {
		props = append(props, fmt.Sprintf("DefaultImage: %s", mdlQuote(ctx, w.DefaultImage)))
	}
	if w.ImageUrl != "" {
		props = append(props, fmt.Sprintf("ImageUrl: %s", mdlQuote(ctx, w.ImageUrl)))
		if len(w.ImageUrlParams) > 0 {
			props = append(props, fmt.Sprintf("ImageUrlParams: (%s)",
				strings.Join(formatParametersV3(w.ImageUrlParams), ", ")))
		}
	}
	if w.AlternativeText != "" {
		props = append(props, fmt.Sprintf("AlternativeText: %s", mdlQuote(ctx, w.AlternativeText)))
		if len(w.AlternativeTextParams) > 0 {
			props = append(props, fmt.Sprintf("AlternativeTextParams: (%s)",
				strings.Join(formatParametersV3(w.AlternativeTextParams), ", ")))
		}
	}
	if w.WidthUnit != "" && w.WidthUnit != "auto" {
		props = append(props, fmt.Sprintf("WidthUnit: %s", w.WidthUnit))
	}
	if w.ImageWidth != "" && w.ImageWidth != "100" {
		props = append(props, fmt.Sprintf("Width: %s", w.ImageWidth))
	}
	if w.HeightUnit != "" && w.HeightUnit != "auto" {
		props = append(props, fmt.Sprintf("HeightUnit: %s", w.HeightUnit))
	}
	if w.ImageHeight != "" && w.ImageHeight != "100" {
		props = append(props, fmt.Sprintf("Height: %s", w.ImageHeight))
	}
	if w.DisplayAs != "" && w.DisplayAs != "fullImage" {
		props = append(props, fmt.Sprintf("DisplayAs: %s", w.DisplayAs))
	}
	if w.Responsive != "" && w.Responsive != "true" {
		props = append(props, fmt.Sprintf("Responsive: %s", w.Responsive))
	}
	if w.OnClickType == "enlarge" {
		props = append(props, "OnClickType: enlarge")
	}
	if w.Action != "" {
		props = append(props, actionProp("OnClick", w.Action))
	}
	return props
}

// actionProp renders an action slot as `Key: <action>`, or — when the action
// renderer returned a `--` note because MDL cannot spell the action — as the
// bare note, which formatWidgetProps places on its own line. Written inline,
// `Action: -- …` left the slot without a value and swallowed the separator
// after it, so the describe output did not parse.
func actionProp(key, rendered string) string {
	if isCommentProp(rendered) {
		return rendered
	}
	return key + ": " + rendered
}

// visibleWhenProp renders "Visible: based on attribute value" as
// `Visible: Attr in (v1, …)`, naming the values that show the widget.
//
// A setting that shows the widget for NO value has no spelling in that form
// (the list would be empty); Studio Pro allows it, so say what it is rather
// than drop it — a dropped setting is an always-visible widget.
func visibleWhenProp(w rawWidget) string {
	if w.VisibleAttr == "" {
		return ""
	}
	if len(w.VisibleValues) == 0 {
		return "-- NOT re-executable: visible for no value of " + w.VisibleAttr +
			" (never shown) — MDL cannot spell an empty value list, so re-running this script would make it always visible"
	}
	vals := make([]string, len(w.VisibleValues))
	for i, v := range w.VisibleValues {
		// `empty` (Studio Pro's "(empty)") and the boolean values are keywords
		// the value list takes bare; quoting them would read as identifiers.
		if v == "empty" || v == "true" || v == "false" {
			vals[i] = v
			continue
		}
		vals[i] = mdlIdent(v)
	}
	attr := mdlIdent(w.VisibleAttr)
	if strings.Contains(w.VisibleAttr, ".") {
		// Qualified (Module.Entity.Attr) where no entity is in scope — written
		// as a qualified name, which takes keyword segments bare.
		attr = w.VisibleAttr
	}
	return fmt.Sprintf("Visible: %s in (%s)", attr, strings.Join(vals, ", "))
}

// visibleRolesProp renders "Visible for module roles" as `Visible: roles
// (Mod.A, Mod.B)`. It is a second `Visible:` line beside an expression or
// attribute condition, as Studio Pro stores both on one settings element.
func visibleRolesProp(w rawWidget) string {
	if len(w.VisibleRoles) == 0 {
		return ""
	}
	return fmt.Sprintf("Visible: roles (%s)", strings.Join(w.VisibleRoles, ", "))
}

// layoutGridWidthMDL spells a stored layout-grid column weight: 1..12 is
// printed as the number, -2 (auto-fit content) as AutoFit, and anything else —
// -1, or absent on a model older than the property — as AutoFill.
func layoutGridWidthMDL(weight int) string {
	switch {
	case weight > 0:
		return fmt.Sprintf("%d", weight)
	case weight == layoutGridWeightAutoFit:
		return "AutoFit"
	}
	return "AutoFill"
}

// layoutGridAlignmentMDL spells a stored layout-grid row or column alignment,
// or "" for Mendix's default ("None", or absent on an older model).
func layoutGridAlignmentMDL(a string) string {
	if a == "" || a == "None" {
		return ""
	}
	return a
}

// appendReadOnlyStyle prints an input widget's ReadOnlyStyle unless it is
// Inherit, Studio Pro's default. Read on the check box only until
// ako/mxcli#977, so a text box, text area, date picker or radio button group
// stored with Text or Control came back from describe → exec as Inherit.
func appendReadOnlyStyle(props []string, w rawWidget) []string {
	if w.ReadOnlyStyle != "" && w.ReadOnlyStyle != "Inherit" {
		props = append(props, fmt.Sprintf("ReadOnlyStyle: %s", w.ReadOnlyStyle))
	}
	return props
}

// outputMappingsNote says a button's flow call has output mappings (Mendix
// 11.5) that describe could not print as `returns ( … )` — a shape that syntax
// does not write, such as a path over an association. Nothing carries them
// through a rewrite, so the note says re-running drops them (ako/mxcli#977).
func outputMappingsNote(w rawWidget) string {
	switch w.UnsayableOutputMappings {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("-- %s: 1 output mapping has no MDL form; re-running this statement drops it", w.Name)
	}
	return fmt.Sprintf("-- %s: %d output mappings have no MDL form; re-running this statement drops them", w.Name, w.UnsayableOutputMappings)
}
