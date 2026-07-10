// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for button action context on pages and
// snippets. A DataGrid/Gallery control bar is not row-scoped, so $currentObject
// has no value there; passing it to a button action builds to CE1571 "No
// argument has been selected for parameter …". This heuristic catches it before
// MxBuild does. See docs/11-proposals/PROPOSAL_check_mxbuild_gap_heuristics.md.
package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

// ValidatePageButtonContext warns (MDL-BUTTON01) when a button inside a control
// bar passes $currentObject to its action. A control bar sits above the grid and
// is not bound to a row, so $currentObject is unbound there — MxBuild reports
// CE1571. Row-scoped buttons (inside a grid column / list item) are fine.
func ValidatePageButtonContext(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreatePageStmtV3:
			out = append(out, checkButtonContextTree(s.Widgets, false, "page "+s.Name.String())...)
		case *ast.CreateSnippetStmtV3:
			out = append(out, checkButtonContextTree(s.Widgets, false, "snippet "+s.Name.String())...)
		}
	}
	return out
}

func checkButtonContextTree(widgets []*ast.WidgetV3, underControlBar bool, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, w := range widgets {
		if w == nil {
			continue
		}
		if underControlBar {
			if a := w.GetAction(); a != nil {
				out = append(out, checkControlBarAction(a, w.Name, locationPrefix)...)
			}
		}
		childUnder := underControlBar || strings.EqualFold(w.Type, "controlbar")
		out = append(out, checkButtonContextTree(w.Children, childUnder, locationPrefix)...)
	}
	return out
}

// checkControlBarAction flags any $currentObject argument on an action (and its
// chained THEN action) that sits inside a control bar.
func checkControlBarAction(a *ast.ActionV3, widgetName, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for a != nil {
		for _, arg := range a.Args {
			if s, ok := arg.Value.(string); ok && strings.EqualFold(s, "$currentObject") {
				out = append(out, linter.Violation{
					RuleID:   "MDL-BUTTON01",
					Severity: linter.SeverityError,
					Message: fmt.Sprintf(
						"%s: control-bar button `%s` passes $currentObject to its %s action, but a control bar is not row-scoped — $currentObject is unbound there (CE1571)",
						locationPrefix, widgetName, a.Type),
					Suggestion: "Move the button into a grid column (row-scoped) so it has a current row, or pass a page parameter instead of $currentObject.",
				})
			}
		}
		a = a.ThenAction
	}
	return out
}
