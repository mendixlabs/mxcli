// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// mdlQuote wraps a string in single quotes and escapes MDL-sensitive characters.
func mdlQuote(s string) string {
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
func explicitPropValue(p rawExplicitProp) string {
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
		return mdlQuote(p.Value)
	default:
		return mdlQuote(p.Value)
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

// appendConditionalProps appends VISIBLE IF and EDITABLE IF if present.
func appendConditionalProps(props []string, w rawWidget) []string {
	if w.VisibleIf != "" {
		props = append(props, fmt.Sprintf("Visible: [%s]", w.VisibleIf))
	}
	if w.EditableIf != "" {
		props = append(props, fmt.Sprintf("Editable: [%s]", w.EditableIf))
	}
	return props
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
func appendAppearanceProps(props []string, w rawWidget) []string {
	// Only when it deviates from Mendix's default, so unchanged widgets keep a
	// quiet round-trip. Empty means the widget type has no editability at all.
	if w.Editable != "" && w.Editable != "Always" {
		props = append(props, fmt.Sprintf("Editable: %s", w.Editable))
	}
	if w.Class != "" {
		props = append(props, fmt.Sprintf("Class: %s", mdlQuote(w.Class)))
	}
	if w.Style != "" {
		props = append(props, fmt.Sprintf("Style: %s", mdlQuote(w.Style)))
	}
	if w.DynamicClasses != "" {
		props = append(props, fmt.Sprintf("DynamicClasses: %s", mdlQuote(w.DynamicClasses)))
	}
	if len(w.DesignProperties) > 0 {
		props = append(props, formatDesignPropertiesMDL(w.DesignProperties))
	}
	if w.VisibleIf != "" {
		props = append(props, fmt.Sprintf("Visible: [%s]", w.VisibleIf))
	}
	if w.EditableIf != "" {
		props = append(props, fmt.Sprintf("Editable: [%s]", w.EditableIf))
	}
	return props
}

// formatDesignPropertiesMDL formats design properties as MDL V3 syntax.
// Toggle → 'Key': ON, Option → 'Key': 'Value'
func formatDesignPropertiesMDL(dps []rawDesignProp) string {
	return fmt.Sprintf("DesignProperties: [%s]", joinDesignPropertyEntries(dps))
}

// joinDesignPropertyEntries renders design-property entries as comma-separated
// MDL. Compound properties recurse into a nested list (issue #668):
// 'Spacing': ['margin-top': 'Large', 'margin-bottom': 'Medium'].
func joinDesignPropertyEntries(dps []rawDesignProp) string {
	var entries []string
	for _, dp := range dps {
		switch dp.ValueType {
		case "toggle":
			entries = append(entries, fmt.Sprintf("%s: on", mdlQuote(dp.Key)))
		case "option":
			entries = append(entries, fmt.Sprintf("%s: %s", mdlQuote(dp.Key), mdlQuote(dp.Option)))
		case "compound":
			entries = append(entries, fmt.Sprintf("%s: [%s]", mdlQuote(dp.Key), joinDesignPropertyEntries(dp.Nested)))
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
	singleLine := fmt.Sprintf("%s%s (%s)%s", prefix, header, strings.Join(props, ", "), suffix)
	if len(singleLine) <= 120 {
		fmt.Fprint(w, singleLine)
		return
	}
	// Multi-line
	indent := prefix + "  "
	fmt.Fprintf(w, "%s%s (\n", prefix, header)
	for i, p := range props {
		if i < len(props)-1 {
			fmt.Fprintf(w, "%s%s,\n", indent, p)
		} else {
			fmt.Fprintf(w, "%s%s\n", indent, p)
		}
	}
	fmt.Fprintf(w, "%s)%s", prefix, suffix)
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
	prefix := strings.Repeat("  ", indent)

	switch w.Type {
	case "Forms$ScrollContainer", "Pages$ScrollContainer":
		header := fmt.Sprintf("scrollcontainer %s", mdlIdent(w.Name))
		props := appendAppearanceProps(nil, w)
		if len(w.Children) > 0 {
			formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
			for _, child := range w.Children {
				outputWidgetMDLV3(ctx, child, indent+1)
			}
			fmt.Fprintf(ctx.Output, "%s}\n", prefix)
		} else {
			formatWidgetProps(ctx.Output, prefix, header, props, "\n")
		}

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
			props = append(props, fmt.Sprintf("SizeMode: %s", mdlQuote(w.RegionSizeMode)))
		}
		props = appendAppearanceProps(props, w)
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

	case "Forms$NavigationTree", "Pages$NavigationTree", "Forms$MenuBar", "Pages$MenuBar":
		keyword := "navigationtree"
		if strings.HasSuffix(w.Type, "$MenuBar") {
			keyword = "menubar"
		}
		header := fmt.Sprintf("%s %s", keyword, mdlIdent(w.Name))
		var props []string
		if w.NavigationProfile != "" {
			props = append(props, fmt.Sprintf("Profile: %s", mdlQuote(w.NavigationProfile)))
		}
		props = appendAppearanceProps(props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$TabControl", "Pages$TabControl":
		header := fmt.Sprintf("tabcontainer %s", mdlIdent(w.Name))
		props := appendAppearanceProps(nil, w)
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
			props = append(props, fmt.Sprintf("Caption: %s", mdlQuote(w.TabCaption)))
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
		props := appendAppearanceProps(nil, w)
		if w.Action != "" {
			props = append(props, fmt.Sprintf("Action: %s", w.Action))
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
			props = append(props, fmt.Sprintf("Caption: %s", mdlQuote(w.Caption)))
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
		props = appendAppearanceProps(props, w)
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
		props := appendAppearanceProps(nil, w)
		formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
		for rowIdx, row := range w.Rows {
			fmt.Fprintf(ctx.Output, "%s  row row%d {\n", prefix, rowIdx+1)
			for colIdx, col := range row.Columns {
				var colProps []string
				widthStr := "AutoFill"
				if col.Width > 0 && col.Width <= 12 {
					widthStr = fmt.Sprintf("%d", col.Width)
				}
				colProps = append(colProps, "DesktopWidth: "+widthStr)
				if col.TabletWidth > 0 && col.TabletWidth <= 12 {
					colProps = append(colProps, fmt.Sprintf("TabletWidth: %d", col.TabletWidth))
				}
				if col.PhoneWidth > 0 && col.PhoneWidth <= 12 {
					colProps = append(colProps, fmt.Sprintf("PhoneWidth: %d", col.PhoneWidth))
				}
				fmt.Fprintf(ctx.Output, "%s    column col%d (%s) {\n", prefix, colIdx+1, strings.Join(colProps, ", "))
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
			props = append(props, fmt.Sprintf("Content: %s", mdlQuote(w.Content)))
		}
		if w.RenderMode != "" && w.RenderMode != "Text" {
			props = append(props, fmt.Sprintf("RenderMode: %s", w.RenderMode))
		}
		if len(w.Parameters) > 0 {
			props = append(props, fmt.Sprintf("ContentParams: [%s]", strings.Join(formatParametersV3(w.Parameters), ", ")))
		}
		props = appendAppearanceProps(props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$ActionButton", "Pages$ActionButton":
		// RenderType "Link" round-trips as the `linkbutton` keyword.
		keyword := "actionbutton"
		if w.RenderMode == "Link" {
			keyword = "linkbutton"
		}
		header := fmt.Sprintf("%s %s", keyword, mdlIdent(w.Name))
		props := []string{}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Caption: %s", mdlQuote(w.Caption)))
		}
		if len(w.Parameters) > 0 {
			props = append(props, fmt.Sprintf("ContentParams: [%s]", strings.Join(formatParametersV3(w.Parameters), ", ")))
		}
		if w.Action != "" {
			props = append(props, fmt.Sprintf("Action: %s", w.Action))
		}
		if w.ButtonStyle != "" && w.ButtonStyle != "Default" {
			props = append(props, fmt.Sprintf("ButtonStyle: %s", w.ButtonStyle))
		}
		if clause := widgetIconMDL(w); clause != "" {
			props = append(props, clause)
		}
		props = appendAppearanceProps(props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")
		if note := widgetIconNote(w); note != "" {
			fmt.Fprintf(ctx.Output, "%s%s\n", prefix, note)
		}

	case "Forms$Text", "Pages$Text":
		props := []string{}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Content: %s", mdlQuote(w.Content)))
		}
		props = appendAppearanceProps(props, w)
		formatWidgetProps(ctx.Output, prefix, "statictext", props, "\n")

	case "Forms$Title", "Pages$Title":
		header := fmt.Sprintf("title %s", mdlIdent(w.Name))
		props := []string{}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Content: %s", mdlQuote(w.Caption)))
		}
		props = appendAppearanceProps(props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$DataView", "Pages$DataView":
		header := fmt.Sprintf("dataview %s", mdlIdent(w.Name))
		props := []string{}
		props = appendDataSourceProp(props, w.DataSource)
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
		props = appendAppearanceProps(props, w)
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
			props = append(props, fmt.Sprintf("Label: %s", mdlQuote(w.Caption)))
		}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Attribute: %s", w.Content))
		}
		if w.Placeholder != "" {
			props = append(props, fmt.Sprintf("Placeholder: %s", mdlQuote(w.Placeholder)))
		}
		if w.OnChange != "" {
			props = append(props, fmt.Sprintf("OnChange: %s", w.OnChange))
		}
		props = appendAppearanceProps(props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$TextArea", "Pages$TextArea":
		header := fmt.Sprintf("textarea %s", mdlIdent(w.Name))
		props := []string{}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Label: %s", mdlQuote(w.Caption)))
		}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Attribute: %s", w.Content))
		}
		if w.OnChange != "" {
			props = append(props, fmt.Sprintf("OnChange: %s", w.OnChange))
		}
		props = appendAppearanceProps(props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$DatePicker", "Pages$DatePicker":
		header := fmt.Sprintf("datepicker %s", mdlIdent(w.Name))
		props := []string{}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Label: %s", mdlQuote(w.Caption)))
		}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Attribute: %s", w.Content))
		}
		if w.OnChange != "" {
			props = append(props, fmt.Sprintf("OnChange: %s", w.OnChange))
		}
		props = appendAppearanceProps(props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$RadioButtons", "Pages$RadioButtons":
		header := fmt.Sprintf("radiobuttons %s", mdlIdent(w.Name))
		props := []string{}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Label: %s", mdlQuote(w.Caption)))
		}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Attribute: %s", w.Content))
		}
		if w.OnChange != "" {
			props = append(props, fmt.Sprintf("OnChange: %s", w.OnChange))
		}
		props = appendAppearanceProps(props, w)
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")

	case "Forms$CheckBox", "Pages$CheckBox":
		header := fmt.Sprintf("checkbox %s", mdlIdent(w.Name))
		props := []string{}
		if w.Caption != "" {
			props = append(props, fmt.Sprintf("Label: %s", mdlQuote(w.Caption)))
		}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Attribute: %s", w.Content))
		}
		// Show ReadOnlyStyle if not default "Inherit"
		if w.ReadOnlyStyle != "" && w.ReadOnlyStyle != "Inherit" {
			props = append(props, fmt.Sprintf("ReadOnlyStyle: %s", w.ReadOnlyStyle))
		}
		// Show ShowLabel if false (not showing label)
		if !w.ShowLabel {
			props = append(props, "ShowLabel: No")
		}
		if w.OnChange != "" {
			props = append(props, fmt.Sprintf("OnChange: %s", w.OnChange))
		}
		props = appendAppearanceProps(props, w)
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
			props = appendDataSourceProp(props, w.DataSource)
			// Add selection mode if specified
			if w.Selection != "" {
				props = append(props, fmt.Sprintf("Selection: %s", w.Selection))
			}
			// onClick action (ledger #67)
			if w.OnClick != "" {
				props = append(props, fmt.Sprintf("onClick: %s", w.OnClick))
			}
			props = appendNamedActionProps(props, w)
			// Add paging properties if non-default
			props = appendDataGridPagingProps(props, w)
			props = appendAppearanceProps(props, w)
			// Output CONTROLBAR and columns as children
			hasContent := len(w.ControlBar) > 0 || len(w.DataGridColumns) > 0
			if hasContent {
				formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
				outputDataContainerContext(ctx.Output, prefix+"  ", w.Name, w.EntityContext, true)
				// Output CONTROLBAR section if control bar widgets present
				if len(w.ControlBar) > 0 {
					fmt.Fprintf(ctx.Output, "%s  controlbar controlBar1 {\n", prefix)
					for _, cb := range w.ControlBar {
						outputWidgetMDLV3(ctx, cb, indent+2)
					}
					fmt.Fprintf(ctx.Output, "%s  }\n", prefix)
				}
				// Output columns — derive name from attribute or caption, fall back to col%d
				for i, col := range w.DataGridColumns {
					colName := deriveColumnName(col, i)
					outputDataGrid2ColumnV3(ctx, prefix+"  ", colName, col)
				}
				fmt.Fprintf(ctx.Output, "%s}\n", prefix)
			} else {
				formatWidgetProps(ctx.Output, prefix, header, props, "\n")
			}
		} else if widgetType == "gallery" {
			// Handle Gallery specially with datasource, selection, filter and content widgets
			header := fmt.Sprintf("gallery %s", mdlIdent(w.Name))
			props := []string{}
			props = appendDataSourceProp(props, w.DataSource)
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
			props = appendAppearanceProps(props, w)
			// Output filter and content widgets
			hasContent := len(w.Children) > 0 || len(w.FilterWidgets) > 0
			if hasContent {
				formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
				outputDataContainerContext(ctx.Output, prefix+"  ", w.Name, w.EntityContext, true)
				// Output FILTER section if filter widgets present
				if len(w.FilterWidgets) > 0 {
					fmt.Fprintf(ctx.Output, "%s  filter filter1 {\n", prefix)
					for _, filter := range w.FilterWidgets {
						outputWidgetMDLV3(ctx, filter, indent+2)
					}
					fmt.Fprintf(ctx.Output, "%s  }\n", prefix)
				}
				// Output TEMPLATE section if content widgets present
				if len(w.Children) > 0 {
					fmt.Fprintf(ctx.Output, "%s  template template1 {\n", prefix)
					for _, child := range w.Children {
						outputWidgetMDLV3(ctx, child, indent+2)
					}
					fmt.Fprintf(ctx.Output, "%s  }\n", prefix)
				}
				fmt.Fprintf(ctx.Output, "%s}\n", prefix)
			} else {
				formatWidgetProps(ctx.Output, prefix, header, props, "\n")
			}
		} else if widgetType == "image" {
			header := fmt.Sprintf("image %s", mdlIdent(w.Name))
			props := describeImageWidgetProps(w)
			props = appendConditionalProps(props, w)
			props = appendAppearanceProps(props, w)
			formatWidgetProps(ctx.Output, prefix, header, props, "\n")
		} else if (len(w.ExplicitProperties) > 0 || len(w.NamedDataSources) > 0 || len(w.ObjectLists) > 0 || w.OnClick != "" ||
			w.OnChange != "" || len(w.NamedActions) > 0) && w.WidgetID != "" {
			// Generic pluggable widget with explicit properties, object-list child
			// blocks (chart series/lines/scaleColors), and/or an onClick action.
			// The widget's own MDL name where that round-trips, else the
			// explicit id form. See pluggableWidgetHeader.
			header := pluggableWidgetHeader(ctx.GetWidgetRegistry(), w.WidgetID, w.Name)
			props := []string{}
			if w.Caption != "" {
				props = append(props, fmt.Sprintf("Label: %s", mdlQuote(w.Caption)))
			}
			// A pluggable widget's own datasource. Without this the branch
			// emitted every property EXCEPT the datasource, so a rewrite dropped
			// it — measured on a Studio Pro-authored File Uploader 2.5.0, whose
			// `associatedFiles` is a Forms$AssociationSource: a page at 0 errors
			// came back as CE0642 "Property 'Associated files' is required" plus
			// two CE1571, because the action's parameter loses its default once
			// the datasource is gone. The DESCRIBE text was byte-identical before
			// and after, so only mx check separated them (#956).
			props = appendDataSourceProp(props, w.DataSource)
			props = appendNamedDataSourceProps(props, w.NamedDataSources)
			for _, ep := range w.ExplicitProperties {
				props = append(props, fmt.Sprintf("%s: %s", ep.Key, explicitPropValue(ep)))
			}
			// onClick action (ledger #67 — reported on CustomChart)
			if w.OnClick != "" {
				props = append(props, fmt.Sprintf("onClick: %s", w.OnClick))
			}
			// OnChange too — a Slider/RangeSlider/StarRating reaches describe
			// through this branch, and its action slot is the only one it has.
			if w.OnChange != "" {
				props = append(props, fmt.Sprintf("OnChange: %s", w.OnChange))
			}
			props = appendNamedActionProps(props, w)
			props = appendAppearanceProps(props, w)
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
					for i, item := range ol.Items {
						itemHeader := fmt.Sprintf("%s %s", ol.Keyword, mdlIdent(fmt.Sprintf("%s%d", ol.Keyword, i+1)))
						itemProps := []string{}
						// An object-list item's datasource goes through the same
						// renderer as a widget's. This site used to have no type
						// switch at all, so a chart series bound to a microflow
						// described as `database from Module.TheMicroflow` (#941).
						itemProps = appendDataSourceProp(itemProps, item.DataSource)
						for _, p := range item.Props {
							if p.IsRef {
								itemProps = append(itemProps, fmt.Sprintf("%s: %s", p.Key, p.Value))
							} else {
								itemProps = append(itemProps, fmt.Sprintf("%s: %s", p.Key, mdlQuote(p.Value)))
							}
						}
						// An item holding child widgets (an Accordion group's `content`
						// slot) needs a body, or the children have nowhere to go and the
						// description silently drops them on re-exec (#891).
						if len(item.Children) > 0 {
							formatWidgetProps(ctx.Output, childPrefix, itemHeader, itemProps, " {\n")
							for _, child := range item.Children {
								outputWidgetMDLV3(ctx, child, indent+2)
							}
							fmt.Fprintf(ctx.Output, "%s}\n", childPrefix)
							continue
						}
						formatWidgetProps(ctx.Output, childPrefix, itemHeader, itemProps, "\n")
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
				props = append(props, fmt.Sprintf("Label: %s", mdlQuote(w.Caption)))
			}
			if w.Content != "" {
				props = append(props, fmt.Sprintf("Attribute: %s", w.Content))
			}
			// Show DataSource and CaptionAttribute for the association modes.
			// The drop-down filter's ref mode has the same three parts as the
			// ComboBox's (reference + option list + caption), so it re-emits
			// through the same branch — without it the filter described back as a
			// bare `dropdownfilter name` and the mode was lost on re-exec (#830).
			if w.DataSource != nil && (widgetType == "combobox" || widgetType == "dropdownfilter") {
				props = appendDataSourceProp(props, w.DataSource)
				if w.CaptionAttribute != "" {
					props = append(props, fmt.Sprintf("CaptionAttribute: %s", w.CaptionAttribute))
				}
			}
			// A pluggable widget's on-change action (ComboBox `onChangeEvent`).
			// Emitted for the same reason as the built-in inputs above: without
			// it a describe→edit→exec cycle silently drops the action.
			if w.OnChange != "" {
				props = append(props, fmt.Sprintf("OnChange: %s", w.OnChange))
			}
			// Show filter attributes for filter widgets
			if len(w.FilterAttributes) > 0 {
				props = append(props, fmt.Sprintf("Attributes: [%s]", strings.Join(w.FilterAttributes, ", ")))
			}
			// Show filter expression if not default
			if w.FilterExpression != "" && w.FilterExpression != "contains" {
				props = append(props, fmt.Sprintf("FilterType: %s", w.FilterExpression))
			}
			props = appendAppearanceProps(props, w)
			formatWidgetProps(ctx.Output, prefix, header, props, "\n")
		}

	case "Forms$NavigationList", "Pages$NavigationList":
		fmt.Fprintf(ctx.Output, "%snavigationlist %s {\n", prefix, mdlIdent(w.Name))
		for _, child := range w.Children {
			itemHeader := fmt.Sprintf("item %s", mdlIdent(child.Name))
			props := []string{}
			if child.Action != "" {
				props = append(props, fmt.Sprintf("Action: %s", child.Action))
			}
			if child.ButtonStyle != "" && child.ButtonStyle != "Default" {
				props = append(props, fmt.Sprintf("ButtonStyle: %s", child.ButtonStyle))
			}
			formatWidgetProps(ctx.Output, prefix+"  ", itemHeader, props, " {\n")
			for _, cw := range child.Children {
				outputWidgetMDLV3(ctx, cw, indent+2)
			}
			fmt.Fprintf(ctx.Output, "%s  }\n", prefix)
		}
		fmt.Fprintf(ctx.Output, "%s}\n", prefix)

	case "Forms$Label", "Pages$Label":
		fmt.Fprintf(ctx.Output, "%sstatictext (Content: %s)\n", prefix, mdlQuote(w.Content))

	case "Forms$Gallery", "Pages$Gallery":
		header := fmt.Sprintf("gallery %s", mdlIdent(w.Name))
		props := []string{}
		props = appendDataSourceProp(props, w.DataSource)
		props = appendAppearanceProps(props, w)
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

	case "Forms$SnippetCallWidget", "Pages$SnippetCallWidget":
		header := fmt.Sprintf("snippetcall %s", mdlIdent(w.Name))
		props := []string{}
		if w.Content != "" {
			props = append(props, fmt.Sprintf("Snippet: %s", w.Content))
		}
		props = appendAppearanceProps(props, w)
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
		fmt.Fprintf(ctx.Output, "%sfooter %s {\n", prefix, mdlIdent(w.Name))
		for _, child := range w.Children {
			outputWidgetMDLV3(ctx, child, indent+1)
		}
		fmt.Fprintf(ctx.Output, "%s}\n", prefix)

	case "Forms$ListView", "Pages$ListView":
		// ListView (also used for Gallery serialization)
		header := fmt.Sprintf("listview %s", mdlIdent(w.Name))
		props := []string{}
		props = appendDataSourceProp(props, w.DataSource)
		// Emit a non-default PageSize so it round-trips (Studio Pro's default is 20).
		if w.PageSize != "" && w.PageSize != "20" {
			props = append(props, fmt.Sprintf("PageSize: %s", w.PageSize))
		}
		props = appendAppearanceProps(props, w)
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

	default:
		// A widget MDL cannot spell. Emitted as a comment naming it, and
		// labelled with what that costs: describe output is re-executable, so a
		// commented widget is one the round trip drops. Measured on
		// Atlas_Core.Atlas_SideBar, whose two Forms$SidebarToggleButton widgets
		// do not survive describe -> exec — silent without this note, because a
		// bare `-- Forms$X (name)` line reads as informational.
		fmt.Fprintf(ctx.Output, "%s-- %s", prefix, w.Type)
		if w.Name != "" {
			fmt.Fprintf(ctx.Output, " (%s)", w.Name)
		}
		fmt.Fprint(ctx.Output, "  -- NOT re-executable: mxcli cannot author this widget, so re-running this script would drop it\n")
	}
}

// deriveColumnName produces a semantic column name from the column's attribute
// or caption. Falls back to "col%d" when neither is available.
func deriveColumnName(col rawDataGridColumn, index int) string {
	if col.Attribute != "" {
		// Use the short attribute name (last segment after dot)
		parts := strings.Split(col.Attribute, ".")
		return parts[len(parts)-1]
	}
	if col.Caption != "" {
		// Sanitize caption to a valid identifier: keep alphanumeric, replace rest with underscore
		sanitized := strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
				return r
			}
			return '_'
		}, col.Caption)
		// Trim leading/trailing underscores and collapse multiples
		result := strings.TrimFunc(sanitized, func(r rune) bool { return r == '_' })
		if result != "" {
			return result
		}
	}
	return fmt.Sprintf("col%d", index+1)
}

// outputDataGrid2ColumnV3 outputs a single DataGrid2 column in V3 MDL syntax.
func outputDataGrid2ColumnV3(ctx *ExecContext, prefix, colName string, col rawDataGridColumn) {
	// Build the main column properties
	var props []string
	if col.Attribute != "" {
		props = append(props, fmt.Sprintf("Attribute: %s", col.Attribute))
	}
	if col.Caption != "" {
		props = append(props, fmt.Sprintf("Caption: %s", mdlQuote(col.Caption)))
	}
	if len(col.CaptionParams) > 0 {
		props = append(props, fmt.Sprintf("CaptionParams: [%s]", strings.Join(formatParametersV3(col.CaptionParams), ", ")))
	}
	// Add ShowContentAs if not default "attribute"
	if col.ShowContentAs != "" && col.ShowContentAs != "attribute" {
		props = append(props, fmt.Sprintf("ShowContentAs: %s", col.ShowContentAs))
	}
	// Add DynamicText content when ShowContentAs is dynamicText
	if col.ShowContentAs == "dynamicText" && col.DynamicText != "" {
		props = append(props, fmt.Sprintf("Content: %s", mdlQuote(col.DynamicText)))
		if len(col.DynamicTextParams) > 0 {
			props = append(props, fmt.Sprintf("ContentParams: [%s]", strings.Join(formatParametersV3(col.DynamicTextParams), ", ")))
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
		props = append(props, fmt.Sprintf("Visible: %s", mdlQuote(col.Visible)))
	}
	if col.DynamicCellClass != "" {
		props = append(props, fmt.Sprintf("DynamicCellClass: %s", mdlQuote(col.DynamicCellClass)))
	}
	if col.Tooltip != "" {
		props = append(props, fmt.Sprintf("Tooltip: %s", mdlQuote(col.Tooltip)))
	}

	// Check if we have content widgets to display
	// Quote the column name when it collides with a reserved keyword (e.g. a column
	// named Title/Description), mirroring the general widget-name path so DESCRIBE
	// output re-parses. #619 added mdlIdent for widgets but missed columns (#638).
	header := fmt.Sprintf("column %s", mdlIdent(colName))
	hasContent := len(col.ContentWidgets) > 0

	if hasContent {
		// Output column with content block
		formatWidgetProps(ctx.Output, prefix, header, props, " {\n")
		for _, widget := range col.ContentWidgets {
			outputWidgetMDLV3(ctx, widget, len(prefix)/2+1)
		}
		fmt.Fprintf(ctx.Output, "%s}\n", prefix)
	} else {
		// Output simple column line
		formatWidgetProps(ctx.Output, prefix, header, props, "\n")
	}
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
func widgetIconMDL(w rawWidget) string {
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
		return fmt.Sprintf("Icon: image %s", mdlQuote(w.Icon))
	case types.MenuIconCollection:
		if w.Icon == "" {
			return ""
		}
		return fmt.Sprintf("Icon: %s", mdlQuote(w.Icon))
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
func widgetIconNote(w rawWidget) string {
	if w.IconType == "" || widgetIconMDL(w) != "" {
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

// actionMapForKey unwraps a client-action node stored under key into a plain map
// (handling both map[string]any and primitive.M), or nil when absent.
func actionMapForKey(w map[string]any, key string) map[string]any {
	if action, ok := w[key].(map[string]any); ok {
		return action
	}
	if actionM, ok := w[key].(primitive.M); ok {
		return map[string]any(actionM)
	}
	return nil
}

// renderClientActionMDL renders a client-action map (a Forms$*ClientAction) back
// to its MDL form (microflow/nanoflow/show_page/save_changes/…). Returns "" for a
// nil action or a NoClientAction.
func renderClientActionMDL(ctx *ExecContext, action map[string]any) string {
	if action == nil {
		return ""
	}
	typeName, _ := action["$Type"].(string)
	switch typeName {
	case "Forms$SaveChangesClientAction", "Pages$SaveChangesClientAction":
		result := "save_changes"
		if closePage, ok := action["ClosePage"].(bool); ok && closePage {
			result += " close_page"
		}
		return result
	case "Forms$CancelChangesClientAction", "Pages$CancelChangesClientAction":
		result := "cancel_changes"
		if closePage, ok := action["ClosePage"].(bool); ok && closePage {
			result += " close_page"
		}
		return result
	case "Forms$ClosePageClientAction", "Pages$ClosePageClientAction":
		return "close_page"
	case "Forms$DeleteClientAction", "Pages$DeleteClientAction":
		result := "delete_object"
		if closePage, ok := action["ClosePage"].(bool); ok && closePage {
			result += " close_page"
		}
		return result
	case "Forms$CreateObjectClientAction", "Pages$CreateObjectClientAction":
		result := "create_object"
		// Extract entity reference
		if entityRef, ok := action["EntityRef"].(map[string]any); ok {
			if entityName, ok := entityRef["Entity"].(string); ok && entityName != "" {
				result += " " + entityName
			}
		}
		// Extract page reference from PageSettings (Forms$FormSettings)
		if pageSettings, ok := action["PageSettings"].(map[string]any); ok {
			// The page is stored in "Form" field as a qualified name string (BY_NAME_REFERENCE)
			if pageName, ok := pageSettings["Form"].(string); ok && pageName != "" {
				pageAction := "show_page " + pageName
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
				result := "show_page " + pageName
				params := pageActionParameters(ctx, formSettings, pageName)
				if params != "" {
					result += "(" + params + ")"
				}
				return result
			}
		}
		if pageSettings, ok := action["PageSettings"].(map[string]any); ok {
			if pageName, ok := pageSettings["Form"].(string); ok && pageName != "" {
				result := "show_page " + pageName
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
				return "show_page " + pageName
			}
		}
		return "show_page"
	case "Forms$MicroflowAction", "Pages$MicroflowClientAction":
		// Extract microflow reference from MicroflowSettings
		if settings, ok := action["MicroflowSettings"].(map[string]any); ok {
			if mfName, ok := settings["Microflow"].(string); ok && mfName != "" {
				result := "microflow " + mfName
				// Extract parameter mappings
				params := extractMicroflowParameters(ctx, settings)
				if params != "" {
					result += "(" + params + ")"
				}
				return result
			}
		}
		return "microflow"
	case "Forms$CallNanoflowClientAction", "Pages$CallNanoflowClientAction":
		if nfName, ok := action["Nanoflow"].(string); ok && nfName != "" {
			result := "nanoflow " + nfName
			// Extract parameter mappings (directly in the action)
			params := extractNanoflowParameters(ctx, action)
			if params != "" {
				result += "(" + params + ")"
			}
			return result
		}
		return "nanoflow"
	case "Forms$SetTaskOutcomeClientAction", "Pages$SetTaskOutcomeClientAction":
		outcomeValue, _ := action["OutcomeValue"].(string)
		return "complete_task '" + strings.ReplaceAll(outcomeValue, "'", "''") + "'"
	case "Forms$SignOutClientAction", "Pages$SignOutClientAction":
		return "sign_out"
	case "Forms$OpenLinkClientAction", "Pages$OpenLinkClientAction":
		// The address is a nested Forms$StaticOrDynamicString. MDL can spell the
		// static form only; a DYNAMIC address (6 of the 31 Studio Pro references
		// use one) reads its value from an attribute at runtime, so rendering it
		// as a literal would round-trip into a different link. Say so instead.
		addr := actionMapForKey(action, "Address")
		if addr == nil {
			return "open_link ''"
		}
		if isDynamic, _ := addr["IsDynamic"].(bool); isDynamic {
			attr := ""
			if ref := actionMapForKey(addr, "AttributeRef"); ref != nil {
				attr, _ = ref["Attribute"].(string)
			}
			return "-- open_link with a dynamic address (" + attr + ") — MDL cannot author this; " +
				"the button is left as-is"
		}
		value, _ := addr["Value"].(string)
		return "open_link '" + strings.ReplaceAll(value, "'", "''") + "'"
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
	allPages, err := ctx.Backend.ListPages()
	if err != nil {
		return ""
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return ""
	}
	for _, p := range allPages {
		if p.ID == pageID {
			modName := h.GetModuleName(h.FindModuleID(p.ContainerID))
			return modName + "." + p.Name
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
	// No stored mapping: the target page's own parameters are the mapping, each
	// bound to the row object the enclosing widget supplies.
	var params []string
	for _, name := range targetPageParameterNames(ctx, pageName) {
		params = append(params, mdlIdent(name)+": $currentObject")
	}
	return strings.Join(params, ", ")
}

// targetPageParameterNames returns the parameter names declared by a page, by
// qualified name. Returns nil when the page cannot be resolved — a description
// that omits an argument is better than one that invents a name.
func targetPageParameterNames(ctx *ExecContext, qualifiedName string) []string {
	module, name, ok := strings.Cut(qualifiedName, ".")
	if !ok || module == "" || name == "" {
		return nil
	}
	allPages, err := ctx.Backend.ListPages()
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
			if param != nil && param.Name != "" {
				names = append(names, param.Name)
			}
		}
		return names
	}
	return nil
}

// extractPageParameters extracts page parameter mappings from a FormSettings/PageSettings object.
// Returns formatted string like "Product: $currentObject" or empty string if no params.
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
				value = arg // e.g., "$Product" or an expression
			}
		}

		// Check for Variable reference (older format - Variable as a map with Name)
		if value == "" {
			if varRef, ok := mappingMap["Variable"].(map[string]any); ok && varRef != nil {
				if varName := extractString(varRef["Name"]); varName != "" {
					value = "$" + varName
				}
			}
		}

		if value != "" {
			params = append(params, mdlIdent(paramName)+": "+value)
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
				value = expr // e.g., "$Product" or an expression
			}
		}

		// Check for Variable reference (older format - Variable as a map with Name)
		if value == "" {
			if varRef, ok := mappingMap["Variable"].(map[string]any); ok && varRef != nil {
				if varName := extractString(varRef["Name"]); varName != "" {
					value = "$" + varName
				}
			}
		}

		if value != "" {
			// Canonical microflowArgV3 form is `Param: $value` (IDENTIFIER COLON expr);
			// emitting `Param = $value` is IDENTIFIER EQUALS, which doesn't re-parse (#640).
			params = append(params, mdlIdent(paramName)+": "+value)
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
				value = expr // e.g., "$Product" or an expression
			}
		}

		// Check for Variable reference (older format - Variable as a map with Name)
		if value == "" {
			if varRef, ok := mappingMap["Variable"].(map[string]any); ok && varRef != nil {
				if varName := extractString(varRef["Name"]); varName != "" {
					value = "$" + varName
				}
			}
		}

		if value != "" {
			// Canonical microflowArgV3 form is `Param: $value` (IDENTIFIER COLON expr);
			// emitting `Param = $value` is IDENTIFIER EQUALS, which doesn't re-parse (#640).
			params = append(params, mdlIdent(paramName)+": "+value)
		}
	}

	return strings.Join(params, ", ")
}

func extractTextCaption(ctx *ExecContext, w map[string]any) string {
	caption, ok := w["Caption"].(map[string]any)
	if !ok {
		return ""
	}
	items := getBsonArrayElements(caption["Items"])
	return selectTranslationText(items, describeDefaultLanguage(ctx))
}

// extractClientTemplateParameters extracts parameter values from a ClientTemplate field (Content or Caption).
// toStringCurrentObjectParamRe matches the exact `toString($currentObject/<path>)`
// form the write side emits for a non-String own-attribute binding. The path may
// carry association hops (Module.Assoc/Attr).
var toStringCurrentObjectParamRe = regexp.MustCompile(`^toString\(\$currentObject/([A-Za-z_][A-Za-z0-9_.]*(?:/[A-Za-z_][A-Za-z0-9_.]*)*)\)$`)

// toStringParamRefRe matches the `toString($<param>/<attr>)` form emitted for a
// non-String page/snippet-parameter binding.
var toStringParamRefRe = regexp.MustCompile(`^toString\(\$([A-Za-z_][A-Za-z0-9_]*)/([A-Za-z_][A-Za-z0-9_.]*)\)$`)

// unwrapToStringAttrParam reverses the write side's non-String attribute→toString
// transform so a ContentParam round-trips as a bare attribute (or $param.attr)
// rather than a rendered expression that re-applies as a bogus attribute name.
// Only the exact auto-generated forms are unwrapped; any other expression (extra
// text, operators, a hand-written toString) is left untouched.
func unwrapToStringAttrParam(expr string) (string, bool) {
	if m := toStringCurrentObjectParamRe.FindStringSubmatch(expr); m != nil {
		return m[1], true
	}
	if m := toStringParamRefRe.FindStringSubmatch(expr); m != nil {
		// $param/attr → $param.attr (the round-trip form for a parameter ref)
		return "$" + m[1] + "." + m[2], true
	}
	return "", false
}

func extractClientTemplateParameters(ctx *ExecContext, w map[string]any, fieldName string) []string {
	template, ok := w[fieldName].(map[string]any)
	if !ok {
		return nil
	}
	params := getBsonArrayElements(template["Parameters"])
	if params == nil {
		return nil
	}
	var result []string
	var suffixes []string // per-param format block " (decimalPrecision: 2, …)", "" when default
	for _, p := range params {
		pMap, ok := p.(map[string]any)
		if !ok {
			continue
		}
		// One suffix per emitted param, in the same order, so it can be zipped in
		// after the value string is chosen (round-trips per-parameter formatting).
		suffixes = append(suffixes, formatParamFormatSuffix(pMap))
		// Check for Expression first (literal value)
		if expr, ok := pMap["Expression"].(string); ok && expr != "" {
			// A non-String attribute binding (Integer/DateTime/…) is written as
			// `toString($currentObject/Attr)` / `toString($param/Attr)` — see
			// resolveTemplateAttributePathFull. Emit it back as the bare attribute
			// (or $param.Attr) so DESCRIBE round-trips; re-applying the bare name
			// re-derives the same toString expression on the write side. Otherwise
			// the rendered expression re-applies as a bogus attribute NAME (CE1613).
			if unwrapped, ok := unwrapToStringAttrParam(expr); ok {
				result = append(result, unwrapped)
			} else {
				result = append(result, expr)
			}
			continue
		}

		// Which variable the parameter is bound to, if any. This read only
		// PageParameter, so a binding to a page-level LOCAL variable came back as
		// `<unbound>` here while the pluggable describer rendered the same bytes
		// as `$Name` (upstream #977) — hence the shared resolver.
		sourceVarName, isLocalVariable := sourceVariableBinding(pMap)

		// A local variable renders bare: it has no AttributeRef to hang off.
		if isLocalVariable {
			result = append(result, "$"+sourceVarName)
			continue
		}

		// Check for AttributeRef
		if attrRef, ok := pMap["AttributeRef"].(map[string]any); ok && attrRef != nil {
			// Attribute navigated over associations (AttributeRef.EntityRef):
			// reconstruct the "Assoc/.../Attr" path so DESCRIBE round-trips.
			if assocPath := associationTemplateParamPath(attrRef); assocPath != "" {
				result = append(result, assocPath)
				continue
			}
			if attr, ok := attrRef["Attribute"].(string); ok {
				if sourceVarName != "" {
					// Has SourceVariable - this is a page parameter reference
					// Extract just the attribute name from the path
					// attr is like "Module.Entity.Attribute", we want just "Attribute"
					parts := strings.Split(attr, ".")
					attrName := parts[len(parts)-1]
					// Use $ParamName.Attribute format to indicate parameter reference
					result = append(result, "$"+sourceVarName+"."+attrName)
				} else {
					// No SourceVariable - use short attribute name
					result = append(result, shortAttributeName(attr))
				}
				continue
			}
		}
		// Parameter exists but has no binding - mark as unbound
		result = append(result, "<unbound>")
	}
	// Append each param's format block to its value string. result and suffixes are
	// aligned (one of each per param that had a valid pMap).
	for i := range result {
		if i < len(suffixes) && suffixes[i] != "" {
			result[i] += suffixes[i]
		}
	}
	return result
}

// formatParamFormatSuffix renders a dynamic-text parameter's FormattingInfo back
// to the MDL format block " (decimalPrecision: 2, groupDigits: true, …)",
// emitting only the fields that differ from the Mendix defaults (DateFormat=Date,
// DecimalPrecision=2, EnumFormat=Text, GroupDigits=false, CustomDateFormat=""),
// so an unformatted parameter round-trips as before (empty string). Mirrors the
// writer defaults in formattingInfoFromParamFormat / formattingInfoToGen.
func formatParamFormatSuffix(pMap map[string]any) string {
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
		parts = append(parts, "customDateFormat: '"+cdf+"'")
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
func associationTemplateParamPath(attrRef map[string]any) string {
	entityRef, ok := attrRef["EntityRef"].(map[string]any)
	if !ok || entityRef == nil {
		return ""
	}
	if extractString(entityRef["$Type"]) != "DomainModels$IndirectEntityRef" {
		return ""
	}
	steps := getBsonArrayElements(entityRef["Steps"])
	if len(steps) == 0 {
		return ""
	}
	assocs := make([]string, 0, len(steps))
	for _, s := range steps {
		sm, ok := s.(map[string]any)
		if !ok {
			return ""
		}
		assoc := extractString(sm["Association"])
		if assoc == "" {
			return ""
		}
		assocs = append(assocs, assoc)
	}
	attr := shortAttributeName(extractString(attrRef["Attribute"]))
	if attr == "" {
		return ""
	}
	return strings.Join(assocs, "/") + "/" + attr
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
		props = append(props, fmt.Sprintf("%s: %s", na.Key, na.MDL))
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
func describeImageWidgetProps(w rawWidget) []string {
	props := []string{}
	if w.ImageType != "" && w.ImageType != "image" {
		props = append(props, fmt.Sprintf("ImageType: %s", w.ImageType))
	}
	if w.ImageObject != "" {
		props = append(props, fmt.Sprintf("Image: %s", mdlQuote(w.ImageObject)))
	}
	if w.ImageUrl != "" {
		props = append(props, fmt.Sprintf("ImageUrl: %s", mdlQuote(w.ImageUrl)))
	}
	if w.AlternativeText != "" {
		props = append(props, fmt.Sprintf("AlternativeText: %s", mdlQuote(w.AlternativeText)))
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
		props = append(props, fmt.Sprintf("OnClick: %s", w.Action))
	}
	return props
}
