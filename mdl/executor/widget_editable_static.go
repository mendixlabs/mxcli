// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The static (non-bracket, non-bare-expression) reading of `Editable:`.
//
// `Visible` and `Editable` share a grammar slot: a string literal lands in the
// widget's plain `Visible` / `Editable` property, and what it means is the
// consumer's business. Visible's consumer (pages.StaticVisibleExpression) reads
// any string that is not true or false as the condition. Editable's used to read
// only the three enum words and skip every other string without a word, so
// `Editable: '$currentObject/Locked = false'` was stored as Always — a field
// meant to be locked stayed editable, with check clean and exec reporting
// success. One rule now serves both: a string that is not the property's plain
// value is its condition (pages.StaticEditability).

// staticEditability resolves the plain `Editable:` value of w, reporting whether
// the author wrote one.
//
// A list view and a grid column are the exception to "a boolean is Always or
// Never": their Editable is a plain bool of their own (ako/mxcli#510), read by
// their builders from GetBoolProp, and not the editability enum. A boolean there
// is that property's value, not a request for the enum.
func staticEditability(w *ast.WidgetV3) (enum, expr string, ok bool) {
	v, present := lookupPropCI(w, "Editable")
	if !present {
		return "", "", false
	}
	if plainEditableWidgetTypes[strings.ToLower(w.Type)] && isBooleanValue(v) {
		return "", "", false
	}
	return pages.StaticEditability(v)
}

// isBooleanValue reports whether v is a boolean or the string spelling of one.
func isBooleanValue(v any) bool {
	switch t := v.(type) {
	case bool:
		return true
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "true" || s == "false"
	}
	return false
}

// editableCondition is the client expression of w's conditional editability
// however it was written: the bare or bracketed form (EditableIf), or a quoted
// `Editable: '<expr>'`. Every check that reads EditableIf asks this, so the
// quoted spelling gets the checks the unquoted one gets.
func editableCondition(w *ast.WidgetV3) (string, bool) {
	if expr := w.GetStringProp("EditableIf"); expr != "" {
		return expr, true
	}
	_, expr, ok := staticEditability(w)
	return expr, ok && expr != ""
}

// newConditionalEditability is the settings element of a conditional editability.
func newConditionalEditability(expr string) *pages.ConditionalEditabilitySettings {
	return &pages.ConditionalEditabilitySettings{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$ConditionalEditabilitySettings",
		},
		Expression: expr,
	}
}

var loneWordRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// editableValueProblem reports why the plain `Editable:` value of w cannot be
// stored, or "" when it can. exec stores every other value, so this is the
// whole list of what check must refuse.
func editableValueProblem(w *ast.WidgetV3) string {
	enum, expr, ok := staticEditability(w)
	if !ok {
		return ""
	}
	switch {
	case enum == "Conditional":
		return "`Editable: Conditional` names no condition"
	case enum != "":
		return ""
	case loneWordRe.MatchString(expr):
		// A client expression has no bare identifiers, so a lone word is a
		// misspelt enum word (`Neever`, `ReadOnly`) and not a condition.
		return "`Editable: " + expr + "` is not an Editable value"
	case !visitor.ParsesAsExpression(expr):
		return "`Editable: " + expr + "` is not a valid expression"
	}
	return ""
}
