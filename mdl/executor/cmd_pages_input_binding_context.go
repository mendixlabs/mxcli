// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// An `Attribute:` binding is only storable when there is an object to bind to.
//
// The writer stores a binding as a DomainModels$AttributeRef holding
// Module.Entity.Attribute and writes a null reference for anything shorter
// (attributeRefToGen), so a bare name nothing could qualify — `textbox t
// (Attribute: FullName)` at the top of a page, where there is no entity in
// scope — went out as `AttributeRef: null`. `exec` said "Created page", and
// mxbuild 11.13.0 reported, per widget kind:
//
//	textbox/textarea/datepicker/checkbox/radiobuttons/dropdown
//	    CE0544 "This widget can only function inside a data context" + CE7005
//	dynamictext  CE0402 "No value specified."
//	combobox     CE0642 "Property 'Attribute' is required."
//
// Qualifying the name does not rescue it: the reference is stored, but outside a
// data container there is no object of that entity to edit, and mxbuild rejects
// that too — CE0544/CE2421 on a text box, CE1365 on a dynamic text, CE7247 on a
// combo box ("Move this widget into a data container"), each with CE7006.
//
// `Attribute: $P/Name` is a third shape of the same drop: it does not parse as
// an attribute path but as a data-source expression, which no input builder
// reads, so it was dropped INSIDE a data view as well as outside one.
//
// `mxcli check -p … --references` already refused the first two on CREATE
// PAGE/SNIPPET (validatePageContextTree), but plain `mxcli check`, `exec
// --no-check` and ALTER PAGE did not. The check-time rule below needs no
// project; the builder guard catches what reaches the writer by any route.

// inputBindingProblem says why w's `Attribute:` binding cannot be stored, or ""
// when it can (or the widget has none).
//
// c is the context the widget sits in. noEntity says that nothing could qualify
// a bare name here — the builder knows that; the check-time walk has no project
// to ask, passes false, and lets the context alone decide.
//
// isPageVariable answers whether a bare `$name` is one of the document's page
// variables, which an input binds to directly (mendixlabs/mxcli#1235); nil
// means the caller cannot know (an ALTER fragment at check time), and the
// builder, which can, decides.
func inputBindingProblem(w *ast.WidgetV3, c pageArgContext, noEntity bool, isPageVariable func(string) bool) string {
	raw, present := lookupPropCI(w, "Attribute")
	if !present || raw == nil {
		return ""
	}
	kind := strings.ToLower(w.Type)
	if name, ok := bareVariableReference(raw); ok {
		if isPageVariable == nil || isPageVariable(name) {
			return ""
		}
	}
	if _, _, ok := rootedAttributeValue(raw); ok {
		// `$name/Assoc/Attr` names its object outright, like `$name.Attr`;
		// whether the name is one is judged where the names are known
		// (ako/mxcli#833).
		return ""
	}
	attr, isString := raw.(string)
	if !isString {
		if name, ok := bareVariableReference(raw); ok {
			return fmt.Sprintf("%s `%s`: `Attribute: $%s` — `$%s` is not a page variable of this document, so the "+
				"widget would be written with no binding at all. Declare it (`Variables: ( $%s: Boolean = 'true' )`) to "+
				"bind the input to it, or bind an attribute by name inside a data container",
				kind, w.Name, name, name, name)
		}
		return nonPathBindingProblem(w, kind, c, nonStringAttributeText(raw))
	}
	if attr == "" {
		return ""
	}
	if _, _, ok := namedObjectBinding(attr); ok {
		// `$name.Attr` names its object outright — a parameter, or an
		// enclosing data view — so the context it sits in does not decide it.
		// Whether the name is one is judged where the names are known:
		// validateNamedObjectBindings at check time, resolveInputBinding and
		// the engine at build time.
		return ""
	}
	qualified := !strings.Contains(attr, "/") && strings.Count(attr, ".") >= 2
	if c.known && !c.present {
		if qualified {
			return fmt.Sprintf("%s `%s`: attribute `%s` is qualified, but the widget is not inside a data view, "+
				"list view, gallery or data grid, so there is no object of that entity for it to show or edit — "+
				"mxbuild rejects it (CE0544 \"This widget can only function inside a data context\", or \"Move "+
				"this widget into a data container\"). Place it inside a data container over that entity",
				kind, w.Name, attr)
		}
		return fmt.Sprintf("%s `%s`: attribute `%s` has no entity to bind against — the widget is not inside "+
			"a data view, list view, gallery or data grid, so it would be written with no binding "+
			"(AttributeRef: null; mxbuild: %s). "+
			"Place it inside a data container, e.g. `dataview dv (DataSource: $Param) { %s %s (Attribute: %s) }`",
			kind, w.Name, attr, unboundBuildError(kind), kind, w.Name, attr)
	}
	if noEntity && !qualified {
		return fmt.Sprintf("%s `%s`: attribute `%s` has no entity to bind against — no enclosing data source "+
			"with a resolvable entity was found, so it would be written with no binding (AttributeRef: null). "+
			"Place it inside a data container or qualify it (Module.Entity.Attribute)",
			kind, w.Name, attr)
	}
	return ""
}

// unboundBuildError is what mxbuild 11.13.0 reports for a widget of this kind
// written with a null binding at the top of a page — measured per kind.
func unboundBuildError(kind string) string {
	switch kind {
	case "dynamictext":
		return `CE0402 "No value specified."`
	case "combobox":
		return `CE0642 "Property 'Attribute' is required."`
	}
	return `CE0544 "This widget can only function inside a data context"`
}

// nonPathBindingProblem is the refusal for an `Attribute:` value that did not
// parse as an attribute path. It quotes the value as written, names the widget
// even when it has no name (a data grid column never does), and gives advice
// that fits where the widget sits: a value with a `/` is an association path
// written in a form the path syntax does not take, and inside a data container
// there is nothing to wrap (ChipCoV8 #10).
func nonPathBindingProblem(w *ast.WidgetV3, kind string, c pageArgContext, text string) string {
	head := fmt.Sprintf("%s: `Attribute: %s` is not an attribute binding MDL can store — the widget would be "+
		"written with no binding at all", widgetRefForMessage(w, kind), text)
	if strings.Contains(text, "/") && !strings.HasPrefix(strings.TrimSpace(text), "$") {
		return head + ". To bind an attribute over an association, write the association path as " +
			"Assoc/Attr or Module.Assoc/Attr (e.g. `Order_Customer/Name` or `Sales.Order_Customer/Name`) — " +
			"each name a plain identifier, with no spaces or operators"
	}
	if c.known && c.present {
		return head + ". Bind an attribute of the object the enclosing data container supplies by name — " +
			"`Name`, or `Assoc/Attr` over an association ($currentObject/Name is written `Name`)"
	}
	return head + fmt.Sprintf(", inside a data view or outside one. Bind the attribute by name inside a data "+
		"container over that object: `dataview dv (DataSource: $Param) { %s (Attribute: Name) }` "+
		"($currentObject/Name is written `Name`)", strings.TrimSpace(kind+" "+w.Name))
}

// widgetRefForMessage names a widget for a message: by its name, or — for one
// Mendix stores no name for, such as a data grid column — by its caption.
func widgetRefForMessage(w *ast.WidgetV3, kind string) string {
	if w.Name != "" {
		return fmt.Sprintf("%s `%s`", kind, w.Name)
	}
	if caption := w.GetCaption(); caption != "" {
		return fmt.Sprintf("%s '%s'", kind, caption)
	}
	return "unnamed " + kind
}

// nonStringAttributeText renders an `Attribute:` value that did not parse as an
// attribute path back into the spelling the author wrote, for the message.
func nonStringAttributeText(v any) string {
	if expr, ok := v.(*ast.WidgetExpressionV3); ok {
		// The source text the author wrote, not the struct (`&{…}`).
		return expr.Text
	}
	ds, ok := v.(*ast.DataSourceV3)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	switch {
	case ds.ContextVariable != "":
		return "$" + ds.ContextVariable + "/" + ds.Reference
	case strings.HasPrefix(ds.Reference, "$"):
		return ds.Reference
	}
	return ds.Type + " " + ds.Reference
}

// checkInputBinding is the builder's refusal: the widget being built must not
// reach the writer with a binding the writer will turn into nothing.
func (pb *pageBuilder) checkInputBinding(w *ast.WidgetV3, entity string) error {
	if msg := inputBindingProblem(w, pb.argCtx, entity == "", pb.isLocalVariable); msg != "" {
		return mdlerrors.NewValidation(msg)
	}
	return nil
}

// validateInputBindingContext is the check-time mirror (MDL-WIDGET34). It runs
// in the widget-tree walk, which has no project and so no entity: only the
// document-root context — known, and empty — refuses a string binding. ALTER PAGE's subtree walk has an unknown
// context and is left to the builder.
//
// A widget with a data source of its own binds its attributes against that
// source, so it is judged in the context it creates, not the one it sits in.
func validateInputBindingContext(w *ast.WidgetV3, c pageArgContext, locationPrefix string) []linter.Violation {
	if w == nil {
		return nil
	}
	if own := argContextForOwnAction(w, c); own != c {
		c = own
	}
	// A bare `$name` is judged against the document's Variables by
	// validatePageVariableBindings, which has them; this walk does not.
	msg := inputBindingProblem(w, c, false, nil)
	if msg == "" {
		return nil
	}
	suggestion := "An input or dynamic text shows an attribute of the object a data view, list view, gallery or data grid supplies — wrap it in one."
	if c.known && c.present {
		// Already inside one: the binding's spelling is the problem, not its place.
		suggestion = "Write the attribute by name (`Name`) or as an association path (`Assoc/Attr`, `Module.Assoc/Attr`)."
	}
	return []linter.Violation{{
		RuleID:     "MDL-WIDGET34",
		Severity:   linter.SeverityError,
		Message:    locationPrefix + ": " + msg,
		Suggestion: suggestion,
	}}
}

// bareVariableReference reads `Attribute: $name` — a variable named with no
// attribute after it — and returns the name without the "$". `$P/Name` is a
// different shape (ContextVariable set) and is not one.
func bareVariableReference(raw any) (string, bool) {
	ds, ok := raw.(*ast.DataSourceV3)
	if !ok || ds.ContextVariable != "" || len(ds.Args) > 0 || ds.Where != "" {
		return "", false
	}
	name, ok := strings.CutPrefix(ds.Reference, "$")
	if !ok || name == "" || strings.ContainsAny(name, "./ ") {
		return "", false
	}
	return name, true
}

func (pb *pageBuilder) isLocalVariable(name string) bool {
	return pb.localVariables[name]
}

// pageVariableInputBinding is the SourceVariable of an input bound directly to
// a page variable — `checkbox cb (Attribute: $ShowAll)` — or nil. Studio Pro
// stores that binding as a Forms$PageVariable naming the variable in its
// LocalVariable slot, with no AttributeRef (mendixlabs/mxcli#1235).
func (pb *pageBuilder) pageVariableInputBinding(w *ast.WidgetV3) *pages.WidgetVariable {
	raw, _ := lookupPropCI(w, "Attribute")
	name, ok := bareVariableReference(raw)
	if !ok || !pb.localVariables[name] {
		return nil
	}
	return &pages.WidgetVariable{Variable: name, Kind: "local"}
}

// validatePageVariableBindings is MDL-WIDGET34 for `Attribute: $name` on a
// whole page or snippet, where the document's Variables are known: a name that
// is not one of them is a binding the writer would drop.
func validatePageVariableBindings(widgets []*ast.WidgetV3, variables []ast.PageVariable, locationPrefix string) []linter.Violation {
	declared := make(map[string]bool, len(variables))
	for _, v := range variables {
		declared[strings.TrimPrefix(v.Name, "$")] = true
	}
	isDeclared := func(name string) bool { return declared[name] }
	var out []linter.Violation
	var walk func(ws []*ast.WidgetV3)
	walk = func(ws []*ast.WidgetV3) {
		for _, w := range ws {
			if w == nil {
				continue
			}
			// A page variable passed to a snippet parameter: page variables are
			// primitive and a snippet parameter is an object, so the call can
			// never match — mx check CE0115 "The arguments that are passed to
			// snippet '…' do not match the expected parameters" (ako/mxcli#833).
			for _, sp := range w.GetSnippetParams() {
				if name := strings.TrimPrefix(sp.Variable, "$"); declared[name] {
					out = append(out, linter.Violation{
						RuleID:   "MDL-WIDGET34",
						Severity: linter.SeverityError,
						Message: locationPrefix + ": " + snippetCallPageVariableMessage(w.Name,
							strings.TrimPrefix(sp.ParamName, "$"), name),
						Suggestion: "Pass an object: a parameter of this document, an enclosing data view (`$dvName`), or leave the parameter to the enclosing data context.",
					})
				}
			}
			raw, _ := lookupPropCI(w, "Attribute")
			if _, ok := bareVariableReference(raw); ok {
				if msg := inputBindingProblem(w, pageArgContext{}, false, isDeclared); msg != "" {
					out = append(out, linter.Violation{
						RuleID:     "MDL-WIDGET34",
						Severity:   linter.SeverityError,
						Message:    locationPrefix + ": " + msg,
						Suggestion: "Bind an input to a page variable by declaring it: `Variables: ( $name: Boolean = true )`, then `Attribute: $name`.",
					})
				}
			}
			walk(w.Children)
		}
	}
	walk(widgets)
	return out
}

// validateNamedObjectBindings is MDL-WIDGET34 for `$name.Attr` on a whole page
// or snippet, where the parameters are known: a name that is neither one of
// them nor — on a built-in input — an enclosing data view names nothing, and
// exec refuses it. `Visible: $name.Attr in (…)` reads a parameter only.
func validateNamedObjectBindings(widgets []*ast.WidgetV3, params []string, locationPrefix string) []linter.Violation {
	declared := make(map[string]bool, len(params))
	for _, p := range params {
		declared[strings.TrimPrefix(p, "$")] = true
	}
	var out []linter.Violation
	report := func(msg string) {
		out = append(out, linter.Violation{
			RuleID:     "MDL-WIDGET34",
			Severity:   linter.SeverityError,
			Message:    locationPrefix + ": " + msg,
			Suggestion: "`$Name.Attr` reads an attribute of the page or snippet parameter $Name; declare it in Params, or bind the attribute by name inside a data container.",
		})
	}
	// Every data view of the document, enclosing or not: a template parameter
	// `$dv1.Attr` naming one that does not enclose the widget was written as
	// SourceVariable { Widget: dv1 }, mx check CE7001 "Widget should be placed
	// inside Data view 'dv1' to use it as a source widget." (ako/mxcli#833).
	allDataViews := map[string]bool{}
	var collect func(ws []*ast.WidgetV3)
	collect = func(ws []*ast.WidgetV3) {
		for _, w := range ws {
			if w == nil {
				continue
			}
			if strings.EqualFold(w.Type, "dataview") && w.Name != "" {
				allDataViews[w.Name] = true
			}
			collect(w.Children)
		}
	}
	collect(widgets)
	var walk func(ws []*ast.WidgetV3, dataViews map[string]bool)
	walk = func(ws []*ast.WidgetV3, dataViews map[string]bool) {
		for _, w := range ws {
			if w == nil {
				continue
			}
			kind := strings.ToLower(w.Type)
			for _, key := range propertyKeys(w.Properties) {
				params, ok := w.Properties[key].([]ast.ParamAssignmentV3)
				if !ok {
					continue
				}
				for _, p := range params {
					v, _ := p.Value.(string)
					name, _, ok := namedObjectBinding(v)
					if !ok || declared[name] || !allDataViews[name] || dataViews[name] {
						continue
					}
					out = append(out, linter.Violation{
						RuleID:   "MDL-WIDGET34",
						Severity: linter.SeverityError,
						Message: fmt.Sprintf("%s: %s `%s`: `{%d} = %s` in %s reads data view `%s`, which does not enclose the "+
							"widget — a data view's name is a source only for the widgets inside it (mx check CE7001 "+
							"\"Widget should be placed inside Data view '%s' to use it as a source widget.\")",
							locationPrefix, kind, w.Name, p.Index, v, key, name, name),
						Suggestion: fmt.Sprintf("Move `%s` inside data view `%s`, or read the object it shows another way (its parameter, `$Param.Attr`).", w.Name, name),
					})
				}
			}
			if root, _, ok := rootedAttributeValue(w.Properties["Attribute"]); ok && !declared[root] &&
				!(namedObjectBindingKinds[kind] && dataViews[root]) {
				report(fmt.Sprintf("%s `%s`: `Attribute: $%s/…` — `$%s` is not a parameter of this document%s, "+
					"so the binding would read nothing", kind, w.Name, root, root, dataViewClause(kind)))
			}
			if attr, ok := w.Properties["Attribute"].(string); ok {
				if name, _, ok := namedObjectBinding(attr); ok && !declared[name] &&
					!(namedObjectBindingKinds[kind] && dataViews[name]) {
					report(fmt.Sprintf("%s `%s`: `Attribute: %s` — `$%s` is not a parameter of this document%s, "+
						"so the binding would read nothing", kind, w.Name, attr, name, dataViewClause(kind)))
				}
			}
			if vw, ok := w.Properties["VisibleWhen"].(*ast.VisibleWhenV3); ok && vw != nil {
				if name, rest, ok := namedObjectBinding(vw.Attribute); ok && (!declared[name] || strings.Contains(rest, ".")) {
					report(fmt.Sprintf("%s `%s`: `Visible: %s in (…)` — `$%s` is not a parameter of this document, "+
						"so the condition would read nothing", kind, w.Name, vw.Attribute, name))
				}
			}
			inner := dataViews
			if kind == "dataview" && w.Name != "" {
				inner = make(map[string]bool, len(dataViews)+1)
				for k := range dataViews {
					inner[k] = true
				}
				inner[w.Name] = true
			}
			walk(w.Children, inner)
		}
	}
	walk(widgets, map[string]bool{})
	return out
}

// propertyKeys is a widget's property keys in a stable order, so violations
// are reported deterministically.
func propertyKeys(props map[string]any) []string {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// snippetCallPageVariableMessage is the refusal for a snippet call passing a
// page variable, shared by check and the builder.
func snippetCallPageVariableMessage(call, param, variable string) string {
	return fmt.Sprintf("snippetcall `%s`: `%s = $%s` passes a page variable, but a snippet parameter is an object "+
		"and a page variable is not — mx check CE0115 \"The arguments that are passed to snippet … do not match the "+
		"expected parameters\"", call, param, variable)
}
