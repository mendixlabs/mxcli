// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for the layout-grid wrapping of edit/new
// forms. Mirrors the MPR010 lint rule (mdl/linter/rules/dataview_layout_grid.go)
// but works on the MDL AST so `mxcli check` warns while authoring, before the
// page is written. A parameter-bound DataView's label/input widths are expressed
// in Bootstrap grid columns and only render correctly inside a layoutgrid.
package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

// ValidatePageLayoutGrid warns (MPR010) when a form DataView — one containing
// input widgets — is not nested inside a layout grid, since its label/input widths
// are laid out in grid columns (independent of the data source). Any layoutgrid
// ancestor satisfies the rule (grid → column → container → dataview is fine); a
// display-only / container DataView (no inputs) is not flagged.
func ValidatePageLayoutGrid(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreatePageStmtV3:
			out = append(out, checkLayoutGridTree(s.Widgets, false, "page "+s.Name.String())...)
		case *ast.CreateSnippetStmtV3:
			out = append(out, checkLayoutGridTree(s.Widgets, false, "snippet "+s.Name.String())...)
		}
	}
	return out
}

func checkLayoutGridTree(widgets []*ast.WidgetV3, underGrid bool, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, w := range widgets {
		if w == nil {
			continue
		}
		if !underGrid && isFormDataViewAST(w) {
			out = append(out, linter.Violation{
				RuleID:   "MPR010",
				Severity: linter.SeverityWarning,
				Message: fmt.Sprintf(
					"%s: DataView `%s` contains input fields but is not inside a layout grid — label and input widths only render correctly inside a layoutgrid",
					locationPrefix, w.Name),
				Suggestion: fmt.Sprintf("Wrap dataview `%s` in `layoutgrid { row { column (desktopwidth: autofill) { … } } }`", w.Name),
			})
		}
		childUnder := underGrid || strings.EqualFold(w.Type, "layoutgrid")
		out = append(out, checkLayoutGridTree(w.Children, childUnder, locationPrefix)...)
	}
	return out
}

// isFormDataViewAST reports whether w is a DataView containing at least one input
// widget — a form, which needs the layout-grid context for its label/input widths
// (independent of the data source).
func isFormDataViewAST(w *ast.WidgetV3) bool {
	if !strings.EqualFold(w.Type, "dataview") {
		return false
	}
	return astSubtreeHasInput(w.Children)
}

// astSubtreeHasInput reports whether any widget in the subtrees is an input
// widget, stopping at a nested DataView boundary.
func astSubtreeHasInput(widgets []*ast.WidgetV3) bool {
	for _, c := range widgets {
		if c == nil {
			continue
		}
		if isInputWidgetAST(c) {
			return true
		}
		if strings.EqualFold(c.Type, "dataview") {
			continue
		}
		if astSubtreeHasInput(c.Children) {
			return true
		}
	}
	return false
}

// isInputWidgetAST reports whether w is a form input widget (native or the
// pluggable combobox).
func isInputWidgetAST(w *ast.WidgetV3) bool {
	switch strings.ToLower(w.Type) {
	case "textbox", "textarea", "datepicker", "dropdown", "checkbox",
		"radiobuttons", "combobox", "referenceselector", "inputreferencesetselector":
		return true
	}
	return false
}
