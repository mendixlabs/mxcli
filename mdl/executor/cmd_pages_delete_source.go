// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A page Delete action deletes the context object, or — Studio Pro's
// Forms$DeleteClientAction.SourceVariable — a widget's selection or a page or
// snippet parameter. A Delete button in a Data grid 2 control bar stores
// SourceVariable = Forms$PageVariable{Widget: "grid1"}; without it mx check
// reports CE7005 "No value selection has been made" (ako/mxcli#1932,
// mendixlabs/mxcli#1730). MDL spells it `delete $grid1`, as a flow argument
// names a grid's selection (#1560).

// deleteSource resolves the `$source` of `delete $source` to the
// SourceVariable it stores, or refuses a bare `delete` that has no object to
// delete. name is written without the "$"; "" is the context object.
func (pb *pageBuilder) deleteSource(name string) (*pages.WidgetVariable, error) {
	if name == "" || strings.EqualFold(name, "currentObject") {
		if msg := deleteWithoutObjectProblem(pb.currentWidget, pb.argCtx); msg != "" {
			return nil, mdlerrors.NewValidation(msg + " [MDL-PAGEDEL01]")
		}
		return nil, nil
	}
	variable, kind := pb.classifyFlowArgValue("$" + name)
	if kind == "" && pb.argCtx.namesSelection("$"+name) {
		variable, kind = "$"+name, "widget"
	}
	variable = strings.TrimPrefix(variable, "$")
	switch kind {
	case "widget":
		return &pages.WidgetVariable{Widget: variable}, nil
	case "parameter", "snippet":
		return &pages.WidgetVariable{Variable: variable, Kind: kind}, nil
	}
	return nil, mdlerrors.NewValidation(fmt.Sprintf(
		"delete $%s: $%s is neither a widget of this document (whose selection is deleted) nor an "+
			"object parameter of this page or snippet — write `delete` to delete the context object [MDL-PAGEDEL01]",
		name, name))
}

// deleteWithoutObjectProblem is the single wording `mxcli check` and `exec`
// share for a bare `delete` with nothing to delete: in a grid's control bar,
// or outside every data widget. "" when there is an object, or when the pass
// cannot see what encloses the widget (ALTER PAGE), where it stands down.
func deleteWithoutObjectProblem(widgetName string, c pageArgContext) string {
	where := "widget `" + widgetName + "`"
	if widgetName == "" {
		where = "this widget"
	}
	if c.selectionOf != "" {
		return fmt.Sprintf(
			"%s: `delete` in the control bar of `%s` has no object to delete — a control bar has no context "+
				"object, and the button would be stored with no source, which mx check reports as CE7005 "+
				"\"No value selection has been made\". Write `delete $%s` to delete the grid's selection",
			where, c.selectionOf, c.selectionOf)
	}
	if c.known && !c.present {
		return fmt.Sprintf(
			"%s: `delete` is not inside a data view, list view or grid row, so there is no context object to "+
				"delete, which mx check reports as CE7005 \"No value selection has been made\". Write "+
				"`delete $<widget>` to delete a widget's selection or `delete $<Param>` for an object parameter "+
				"of this page or snippet, or put the button inside a data view",
			where)
	}
	return ""
}

// validateDeleteSource is the check-time mirror of the executor guard.
func validateDeleteSource(w *ast.WidgetV3, c pageArgContext, locationPrefix string) []linter.Violation {
	if w == nil {
		return nil
	}
	action := w.GetAction()
	if action == nil || action.Type != "delete" {
		return nil
	}
	if action.SourceVariable != "" && !strings.EqualFold(action.SourceVariable, "currentObject") {
		return nil
	}
	msg := deleteWithoutObjectProblem(w.Name, c)
	if msg == "" {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL-PAGEDEL01",
		Severity: linter.SeverityError,
		Message:  locationPrefix + ": " + msg,
	}}
}
