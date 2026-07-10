// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// DataViewLayoutGridRule (MPR010) flags a form — a DataView that contains input
// widgets — that is not nested inside a layout grid. A DataView's label width and
// input-control width are expressed in Bootstrap grid columns, which only render
// correctly when the form sits inside a layoutgrid (the Studio Pro NewEdit page
// template wraps it in layoutgrid → row → column → dataview). A form DataView
// placed directly on the page renders with misaligned labels/inputs — regardless
// of whether its data source is a page parameter, a database retrieve, or a
// microflow.
//
// The trigger is "the DataView contains an input widget", not its data source: a
// display-only DataView (only dynamictext) or a container DataView (wrapping a
// nested datagrid, e.g. master-detail) has no label/input-width concern and is
// not flagged. "Inside a layout grid" means any layoutgrid ancestor — a DataView
// under grid → column → container → dataview is fine; only a form DataView with no
// layoutgrid ancestor at all is flagged.
type DataViewLayoutGridRule struct{}

// NewDataViewLayoutGridRule creates a new dataview-layout-grid rule.
func NewDataViewLayoutGridRule() *DataViewLayoutGridRule {
	return &DataViewLayoutGridRule{}
}

func (r *DataViewLayoutGridRule) ID() string                       { return "MPR010" }
func (r *DataViewLayoutGridRule) Name() string                     { return "DataViewLayoutGrid" }
func (r *DataViewLayoutGridRule) Category() string                 { return "design" }
func (r *DataViewLayoutGridRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *DataViewLayoutGridRule) Description() string {
	return "A DataView containing input widgets (a form) should be wrapped in a layout grid so label and input widths render correctly"
}

// Check walks each page and snippet, flagging parameter-bound DataViews that have
// no layoutgrid ancestor.
func (r *DataViewLayoutGridRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}
	var violations []linter.Violation

	check := func(id, qualifiedName, moduleName, docType string) {
		raw, err := reader.GetRawUnit(model.ID(id))
		if err != nil || raw == nil {
			return
		}
		for _, root := range rootWidgetNodes(raw) {
			walkForUngridedDataView(root, false, func(dv map[string]any) {
				violations = append(violations, linter.Violation{
					RuleID:   r.ID(),
					Severity: r.DefaultSeverity(),
					Message: fmt.Sprintf("DataView '%s' contains input fields but is not inside a layout grid — label and input widths only render correctly inside a layoutgrid",
						widgetName(dv)),
					Location: linter.Location{
						Module:       moduleName,
						DocumentType: docType,
						DocumentName: docNameFromQualified(qualifiedName),
						DocumentID:   id,
					},
					Suggestion: fmt.Sprintf("Wrap dataview '%s' in `layoutgrid { row { column (desktopwidth: autofill) { … } } }`", widgetName(dv)),
				})
			})
		}
	}

	for p := range ctx.Pages() {
		if ctx.IsExcluded(p.ModuleName) {
			continue
		}
		check(p.ID, p.QualifiedName, p.ModuleName, "page")
	}
	for s := range ctx.Snippets() {
		if ctx.IsExcluded(s.ModuleName) {
			continue
		}
		check(s.ID, s.QualifiedName, s.ModuleName, "snippet")
	}
	return violations
}

// rootWidgetNodes returns the top-level widget nodes of a page (under
// FormCall.Arguments[].Widgets) or snippet (top-level Widgets).
func rootWidgetNodes(rawData map[string]any) []map[string]any {
	var roots []map[string]any
	if formCall, ok := rawData["FormCall"].(map[string]any); ok {
		for _, arg := range getBsonArray(formCall["Arguments"]) {
			if argMap, ok := arg.(map[string]any); ok {
				for _, w := range getBsonArray(argMap["Widgets"]) {
					if wMap, ok := w.(map[string]any); ok {
						roots = append(roots, wMap)
					}
				}
			}
		}
	}
	for _, w := range getBsonArray(rawData["Widgets"]) {
		if wMap, ok := w.(map[string]any); ok {
			roots = append(roots, wMap)
		}
	}
	return roots
}

// walkForUngridedDataView walks the widget subtree rooted at w, calling report for
// every parameter-bound DataView reached without crossing a Forms$LayoutGrid.
// underGrid latches true once a LayoutGrid ancestor is seen.
func walkForUngridedDataView(w map[string]any, underGrid bool, report func(map[string]any)) {
	if !underGrid && isFormDataView(w) {
		report(w)
	}
	nowUnder := underGrid || extractStr(w["$Type"]) == "Forms$LayoutGrid"
	for _, child := range childWidgetNodes(w) {
		walkForUngridedDataView(child, nowUnder, report)
	}
}

// childWidgetNodes returns the direct child widget maps of w across every
// container shape (mirrors collectWidgetsRecursive's edges).
func childWidgetNodes(w map[string]any) []map[string]any {
	var out []map[string]any
	add := func(arr any) {
		for _, c := range getBsonArray(arr) {
			if cm, ok := c.(map[string]any); ok {
				out = append(out, cm)
			}
		}
	}
	add(w["Widgets"])
	add(w["FooterWidgets"])
	for _, row := range getBsonArray(w["Rows"]) {
		if rowMap, ok := row.(map[string]any); ok {
			for _, col := range getBsonArray(rowMap["Columns"]) {
				if colMap, ok := col.(map[string]any); ok {
					add(colMap["Widgets"])
				}
			}
		}
	}
	for _, tp := range getBsonArray(w["TabPages"]) {
		if tpMap, ok := tp.(map[string]any); ok {
			add(tpMap["Widgets"])
		}
	}
	if cr, ok := w["CenterRegion"].(map[string]any); ok {
		add(cr["Widgets"])
	}
	if obj, ok := w["Object"].(map[string]any); ok {
		for _, prop := range getBsonArray(obj["Properties"]) {
			if propMap, ok := prop.(map[string]any); ok {
				if value, ok := propMap["Value"].(map[string]any); ok {
					add(value["Widgets"])
				}
			}
		}
	}
	return out
}

// isFormDataView reports whether w is a DataView containing at least one input
// widget — i.e. a form, which is what needs the layout-grid context for its
// label/input widths (regardless of data source).
func isFormDataView(w map[string]any) bool {
	if extractStr(w["$Type"]) != "Forms$DataView" {
		return false
	}
	return subtreeHasInput(childWidgetNodes(w))
}

// subtreeHasInput reports whether any node in the given widget subtrees is an
// input widget. It stops descending at a nested DataView so a nested form's
// inputs aren't attributed to the outer DataView.
func subtreeHasInput(nodes []map[string]any) bool {
	for _, n := range nodes {
		if isInputWidget(n) {
			return true
		}
		if extractStr(n["$Type"]) == "Forms$DataView" {
			continue
		}
		if subtreeHasInput(childWidgetNodes(n)) {
			return true
		}
	}
	return false
}

// isInputWidget reports whether w is a form input widget (native or the pluggable
// ComboBox) — one whose label/control width is laid out in grid columns.
func isInputWidget(w map[string]any) bool {
	switch extractStr(w["$Type"]) {
	case "Forms$TextBox", "Forms$TextArea", "Forms$DatePicker", "Forms$DropDown",
		"Forms$CheckBox", "Forms$RadioButtonGroup", "Forms$ReferenceSelector",
		"Forms$InputReferenceSetSelector":
		return true
	}
	// Pluggable ComboBox.
	if extractStr(w["$Type"]) == "CustomWidgets$CustomWidget" {
		if typeObj, ok := w["Type"].(map[string]any); ok {
			return extractStr(typeObj["WidgetId"]) == "com.mendix.widget.web.combobox.Combobox"
		}
	}
	return false
}
