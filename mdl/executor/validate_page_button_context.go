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

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidatePageButtonContext warns (MDL-BUTTON01) when a button inside a control
// bar passes $currentObject to its action. A control bar sits above the grid and
// is not bound to a row, so $currentObject is unbound there — MxBuild reports
// CE1571. Row-scoped buttons (inside a grid column / list item) are fine, and so
// is a control bar of a grid nested inside a data view or list view item, where
// $currentObject is the enclosing object (ako/mxcli#953).
func ValidatePageButtonContext(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		if label, widgets, ok := documentWidgets(stmt); ok {
			out = append(out, checkButtonContextTree(widgets, "", false, "", label)...)
		}
	}
	return out
}

// checkButtonContextTree walks the widget tree, carrying the name of the data
// widget whose control bar it is inside ("" when it is not inside one), and
// whether an enclosing data container already supplies an object context.
//
// The name is carried rather than just a flag because it IS the remedy: a data
// widget's selection is addressed by the widget's own name, so `$dgMaterials` is
// only spellable from here. Advice that stops at "move it into a column" sends
// an author looking for syntax that does not need to exist — which is how
// mendixlabs/mxcli#1082 was filed.
//
// inContext is the enclosing-container half (ako/mxcli#953): a grid nested in a
// data view, a list view item or a grid column sits in that container's object
// context, and its control bar's $currentObject is that object — mxbuild builds
// it clean. The grid's OWN data source never scopes its control bar, so the
// control bar inherits the context from above the grid, not from the grid.
//
// nearest is the name of the data container whose object the widget sits in
// ("" at the top), for MDL-BUTTON02 (mendixlabs/mxcli#1324). It moves with the
// same rule as inContext: a control bar keeps the context from above its grid.
func checkButtonContextTree(widgets []*ast.WidgetV3, controlBarOf string, inContext bool, nearest, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, w := range widgets {
		if w == nil {
			continue
		}
		if a := w.GetAction(); a != nil && controlBarOf != "" && !inContext {
			out = append(out, checkControlBarAction(a, w.Name, controlBarOf, locationPrefix)...)
		}
		if nearest != "" {
			out = append(out, checkOwnContainerName(w, nearest, locationPrefix)...)
		}
		childContext := inContext || isObjectContextContainer(w)
		childNearest := nearest
		if isObjectContextContainer(w) && w.Name != "" {
			childNearest = w.Name
		}
		for _, c := range w.Children {
			childOf, ctx, near := controlBarOf, childContext, childNearest
			if c != nil && strings.EqualFold(c.Type, "controlbar") {
				childOf, ctx, near = w.Name, inContext, nearest
			}
			out = append(out, checkButtonContextTree([]*ast.WidgetV3{c}, childOf, ctx, near, locationPrefix)...)
		}
	}
	return out
}

// ownNameExprProps are the expression-valued widget properties evaluated in
// the widget's enclosing context, keyed as the visitor stores them (`Visible:`
// and `Editable:` expressions land under the *If keys) and named as written.
var ownNameExprProps = []struct{ key, written string }{
	{"VisibleIf", "Visible"}, {"EditableIf", "Editable"}, {"DynamicClasses", "DynamicClasses"},
}

// checkOwnContainerName flags a widget that reads the nearest data container
// by its widget name — `$dvGate` from a widget directly inside dvGate. A
// container's name is a variable only for the containers nested BELOW it; in
// its own context the object is $currentObject (mendixlabs/mxcli#1324).
//
// Every slot here is evaluated in the widget's enclosing context, and each was
// measured on mxbuild 11.14.0 against a control one data view deeper: action
// arguments (and a chained THEN action's), a data source's flow arguments,
// Visible, Editable and DynamicClasses are CE0117 "Error(s) in expression.",
// and a data source's XPath `where` is CE0161 "Error(s) in XPath constraint.".
func checkOwnContainerName(w *ast.WidgetV3, nearest, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	flag := func(slot, expr, code string) {
		if !exprReadsVariable(expr, nearest) {
			return
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-BUTTON02",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: `%s` reads `%s` in its %s, but `%s` is the data container the widget sits in directly — its name is a variable only inside a data container nested below it (%s)",
				locationPrefix, w.Name, expr, slot, nearest, code),
			Suggestion: fmt.Sprintf(
				"Use $currentObject for the object of `%s` here (`$currentObject/Attr` for an attribute); `$%s` works from a data view, list or grid nested inside it.",
				nearest, nearest),
		})
	}
	for a := w.GetAction(); a != nil; a = a.ThenAction {
		for _, arg := range a.Args {
			if v, ok := arg.Value.(string); ok {
				flag(a.Type+" action argument", v, "CE0117")
			}
		}
	}
	if ds := w.GetDataSource(); ds != nil {
		for _, arg := range ds.Args {
			if v, ok := arg.Value.(string); ok {
				flag("data source argument", v, "CE0117")
			}
		}
		flag("data source XPath", ds.Where, "CE0161")
	}
	for _, p := range ownNameExprProps {
		if v, ok := w.Properties[p.key].(string); ok {
			flag(p.written+" expression", v, "CE0117")
		}
	}
	// A quoted `Editable: '<expr>'` is the same condition as the bare one.
	if _, routed := w.Properties["EditableIf"]; !routed {
		if expr, ok := editableCondition(w); ok {
			flag("Editable expression", expr, "CE0117")
		}
	}
	return out
}

// exprReadsVariable reports whether expression source reads $name as a
// variable: a `$name` token outside a string literal, ending at a
// non-identifier character. Case-insensitive, as widget names are unique
// case-insensitively on a page.
func exprReadsVariable(expr, name string) bool {
	inString := false
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if c == '\'' {
			inString = !inString // '' inside a literal toggles twice
			continue
		}
		if inString || c != '$' {
			continue
		}
		j := i + 1
		for j < len(expr) && isExprIdentByte(expr[j]) {
			j++
		}
		if strings.EqualFold(expr[i+1:j], name) {
			return true
		}
		i = j - 1
	}
	return false
}

func isExprIdentByte(c byte) bool {
	return c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
}

// isObjectContextContainer reports whether w gives its (non-control-bar)
// children a current object: a data view's object, a list view or gallery
// item, a grid row (column content).
func isObjectContextContainer(w *ast.WidgetV3) bool {
	switch strings.ToLower(w.Type) {
	case "dataview", "listview", "gallery", "templategrid", "datagrid", "datagrid2":
		return true
	}
	return false
}

// checkControlBarAction flags any $currentObject argument on an action (and its
// chained THEN action) that sits inside a control bar.
func checkControlBarAction(a *ast.ActionV3, widgetName, controlBarOf, locationPrefix string) []linter.Violation {
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
					Suggestion: controlBarSuggestion(controlBarOf),
				})
			}
		}
		a = a.ThenAction
	}
	return out
}

// controlBarSuggestion names the remedy that actually applies to a control bar.
//
// The selection comes first because it is the one that keeps the button where
// the author put it: a data widget with `Selection:` set exposes the selected
// object as `$<widgetName>`, which an action takes as an ordinary argument.
func controlBarSuggestion(controlBarOf string) string {
	if controlBarOf == "" {
		return "Move the button into a grid column (row-scoped) so it has a current row, or pass a page parameter instead of $currentObject."
	}
	return fmt.Sprintf(
		"Pass the selection of `%s` instead — `Action: microflow M.F(Param = $%s)`, with `Selection:` set on the widget. "+
			"Or move the button into a grid column (row-scoped) so it has a current row, or pass a page parameter.",
		controlBarOf, controlBarOf)
}
