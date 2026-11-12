// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// pageArgContext is what a widget-tree walk knows about the object Mendix would
// pass to a page opened by a widget's SHOW_PAGE action.
//
// mxcli stores a widget's show-page action with an EMPTY ParameterMappings array
// and lets Mendix infer the argument from the enclosing widget's context object.
// That is deliberate and twice-confirmed: an explicit Forms$PageParameterMapping
// whose Argument is "$currentObject" makes Studio Pro report CE0115 "parameters do
// not match", because a widget's current-row object is an inferred WidgetValue and
// not an Argument expression (issue #296, re-confirmed against mxbuild 11.12.1 for
// mxcli-formula1 §56). See formSettingsToGen in mdl/backend/modelsdk/widget_write.go.
//
// So the argument is honoured only when it names the context object, either as
// $currentObject or by the name of the variable the enclosing data widget is bound
// to. Anything else is refused by the caller rather than silently re-pointed.
//
// Three states, not two, and the third is what mendixlabs/mxcli#1029 reported:
//
//   - known=false — the pass cannot say what encloses the widget. ALTER PAGE's
//     SET/INSERT build an action against a stored page they never traverse, so an
//     empty varName means nothing about the argument. The guard stands down: it
//     only ever refuses what it can prove is discarded.
//   - known=true, present=true — a data-bound widget encloses this one, so there
//     IS a context object. The argument is honoured when it names that object.
//   - known=true, present=false — the walk started at the root of a page, layout
//     or snippet and never entered a data widget. There is NO context object:
//     $currentObject is unbound and the empty mapping mxcli writes is not an
//     inferred one, it is a missing one. EVERY argument is discarded here,
//     whatever its form, and mxbuild reports CE1571 per parameter of the target
//     page.
type pageArgContext struct {
	known   bool
	present bool
	// varName is the name the data source gives the context object, without the
	// "$" (e.g. "Car" for `dataview dv (DataSource: $Car)`). Empty when the
	// context object has no name of its own — a database, association, microflow
	// or selection source supplies a row object addressable only as
	// $currentObject.
	varName string
	// entity is the qualified entity of that context object, for the message.
	entity string
	// docParams are the document's entity-typed parameters (lower case), known
	// only to the check-time walk of a whole CREATE PAGE / SNIPPET. A show page
	// argument naming one is bound by name and needs no context object
	// (mendixlabs/mxcli#1403); the executor asks classifyFlowArgValue instead.
	docParams *documentParams
	// dataWidget is the name of the data widget whose context this is, so a
	// control bar below it knows whose selection it holds.
	dataWidget string
	// selectionOf is set inside a control bar: the name of its grid, whose
	// selection `$<grid>` passes to a page as an explicit mapping naming the
	// grid (mendixlabs/mxcli#122). The row object is not the control bar's.
	selectionOf string
	// rowSelectionOf is set inside a selectable data grid's rows: the grid's
	// name, whose selection `$<grid>` a show page in a column passes as the
	// same explicit mapping. Studio Pro stores exactly that (Variable {Widget:
	// grid1}) on FactoryManagement's MxModelReflection.MxObjectType_Select,
	// and it builds (ako/mxcli#2109). The row object stays $currentObject.
	rowSelectionOf string
}

// inDataWidget names the data widget whose context c is.
func (c pageArgContext) inDataWidget(name string) pageArgContext {
	c.dataWidget = name
	return c
}

// namesSelection reports whether value is `$<grid>` written in that grid's
// control bar: the grid's selection, bound by name like a page parameter.
func (c pageArgContext) namesSelection(value string) bool {
	name, ok := strings.CutPrefix(value, "$")
	return ok && c.selectionOf != "" && strings.EqualFold(name, c.selectionOf)
}

// namesRowSelection reports whether value is `$<grid>` written in that grid's
// rows, where the grid has a selection: a show page argument only.
func (c pageArgContext) namesRowSelection(value string) bool {
	name, ok := strings.CutPrefix(value, "$")
	return ok && c.rowSelectionOf != "" && strings.EqualFold(name, c.rowSelectionOf)
}

// selectableDataGrid reports whether w is a data grid with a selection, whose
// name is then a variable in its rows as well as in its control bar.
func selectableDataGrid(w *ast.WidgetV3) bool {
	switch strings.ToLower(w.Type) {
	case "datagrid", "datagrid2":
	default:
		return false
	}
	sel := w.GetSelection()
	return sel != "" && !strings.EqualFold(sel, "None")
}

// enteringDataWidget is the context below a data-bound widget bound to ds and
// yielding entity.
func enteringDataWidget(ds *ast.DataSourceV3, entity string) pageArgContext {
	return pageArgContext{known: true, present: true, varName: contextVarFor(ds), entity: entity}
}

// enteringDataWidgetOf is enteringDataWidget for the rows of data widget w, by
// name: a selectable data grid's name is its selection there too. The one
// constructor the check walk and both builders use, so the three agree.
func enteringDataWidgetOf(w *ast.WidgetV3, ds *ast.DataSourceV3, entity string) pageArgContext {
	c := enteringDataWidget(ds, entity).inDataWidget(w.Name)
	if selectableDataGrid(w) {
		c.rowSelectionOf = w.Name
	}
	return c
}

// atDocumentRoot is the context at the top of a page, snippet or layout the pass
// walks in full: knowable, and empty.
func atDocumentRoot() pageArgContext {
	return pageArgContext{known: true}
}

// contextVarFor returns the name a data source gives its context object, without
// the "$". Only a parameter/variable source names it; a database, association,
// microflow or selection source yields a row object addressable only as
// $currentObject, so the name is empty.
func contextVarFor(ds *ast.DataSourceV3) string {
	if ds == nil || ds.Type != "parameter" {
		return ""
	}
	return strings.TrimPrefix(ds.Reference, "$")
}

// contextFreeContainers are the widget kinds that hold other widgets and bind no
// data of their own, so their children see exactly the context they do.
//
// The list is an allow-list on purpose. Concluding "there is no context object
// here" is only safe when every widget between the page root and this one is
// known to bind nothing, and a widget's data source is not always readable from
// the AST: a pluggable widget names its source under its own key, and a
// DataSource value that is not a parsed *ast.DataSourceV3 cannot be read. (The
// bare-entity shorthand `DataSource: Mod.Entity` was that case until it parsed
// as DATABASE, ako/mxcli#576.) Anything not listed here therefore
// degrades the context to UNKNOWN rather than to ABSENT, so a row-scoped button
// is never refused (mdl-examples/bug-tests/295-showpage-null-variable.mdl is that
// case, and it is exactly what the first cut of #1029 broke).
var contextFreeContainers = map[string]bool{
	"container": true, "customcontainer": true, "slot": true,
	"layoutgrid": true, "row": true, "column": true,
	"tabcontainer": true, "tabpage": true,
	"groupbox": true, "scrollcontainer": true, "region": true,
	"header": true, "footer": true, "placeholder": true,
}

// argContextForChildren is the context the children of w see.
func argContextForChildren(w *ast.WidgetV3, parent pageArgContext) pageArgContext {
	if ds := w.GetDataSource(); ds != nil {
		// The entity is only used to word the refusal; the executor's builder
		// overwrites this with one that carries it.
		c := enteringDataWidgetOf(w, ds, "")
		c.docParams = parent.docParams
		return c
	}
	if strings.EqualFold(w.Type, "controlbar") && parent.dataWidget != "" {
		c := parent
		c.selectionOf = parent.dataWidget
		return c
	}
	if parent.known && !parent.present && !contextFreeContainers[strings.ToLower(w.Type)] {
		return pageArgContext{}
	}
	return parent
}

// argContextForOwnAction is the context a widget's OWN action is judged in.
//
// A button's action runs in the context its parent supplies. A list widget's does
// not: `onClick` on a data grid, list view or gallery fires PER ROW, and the row
// it renders is the context object — so a widget that binds a source of its own
// supplies the context for its own action. Judging it in the parent's context
// refused `datagrid dg (DataSource: DATABASE M.E, onClick: SHOW_PAGE M.Edit(E:
// $currentObject))` against an mxbuild that accepts it at 0 errors, and on a list
// view the refusal contradicted its own wording (ako/mxcli#552).
//
// `Action:` and `onClick:` are aliases that both land on Properties["Action"], so
// this covers every spelling of a widget's own action.
func argContextForOwnAction(w *ast.WidgetV3, parent pageArgContext) pageArgContext {
	if w == nil {
		return parent
	}
	if ds := w.GetDataSource(); ds != nil {
		// The entity is only used to word a refusal; the executor's builder
		// overwrites this with one that carries it.
		c := enteringDataWidget(ds, "").inDataWidget(w.Name)
		c.docParams = parent.docParams
		return c
	}
	if bindsDataInAnUnreadableShape(w) {
		// The widget plainly binds data, so a context object EXISTS — but this
		// pass cannot say what it is called. Unknown, not absent, and the guard
		// stands down exactly as it does for ALTER PAGE.
		return pageArgContext{}
	}
	return parent
}

// bindsDataInAnUnreadableShape reports whether w names a data source this pass
// cannot parse into a *ast.DataSourceV3. The visitor no longer produces one
// under `DataSource` (the bare-entity shorthand parses as DATABASE since
// ako/mxcli#576); this stays as the defensive half of the doctrine that the
// guard refuses only what it can prove is discarded.
func bindsDataInAnUnreadableShape(w *ast.WidgetV3) bool {
	for name, v := range w.Properties {
		if !strings.EqualFold(name, "DataSource") {
			continue
		}
		if _, parsed := v.(*ast.DataSourceV3); !parsed && v != nil {
			return true
		}
	}
	return false
}

// argContextForSubtreeOf is argContextForChildren for the executor's builder,
// which builds a widget AND its children in one call. A widget with no children
// carries nothing but its own action, and that action is judged in the context
// its parent supplies — degrading there would stand the guard down on exactly the
// page-level button #1029 is about. A data-bound widget is the exception, and the
// reason is argContextForOwnAction's: its own action is row-scoped, so it needs
// the context it creates whether or not it has children to give it to.
func argContextForSubtreeOf(w *ast.WidgetV3, parent pageArgContext) pageArgContext {
	if own := argContextForOwnAction(w, parent); own != parent {
		return own
	}
	if len(w.Children) == 0 {
		return parent
	}
	return argContextForChildren(w, parent)
}

// binds reports whether the argument `value`, written on a SHOW_PAGE widget
// action, denotes the object Mendix will actually pass to the target page.
//
// Arguments that are not a $-reference (a literal or an expression) are left
// alone WHERE A CONTEXT OBJECT EXISTS: they cannot be checked against it, and
// refusing them would be guesswork. Where none exists there is nothing to guess
// about — the mapping is empty either way — so they are refused too.
func (c pageArgContext) binds(value string) bool {
	if !c.known {
		return true
	}
	if !c.present {
		return false
	}
	if !strings.HasPrefix(value, "$") {
		return true
	}
	name := strings.TrimPrefix(value, "$")
	// A path expression ($obj/Module.Assoc) is not a plain variable reference.
	if strings.ContainsAny(name, "/.") {
		return true
	}
	if strings.EqualFold(name, "currentObject") {
		return true
	}
	return c.varName != "" && strings.EqualFold(name, c.varName)
}

// describeContextObject names the object Mendix will actually pass, for the
// refusal message.
func (c pageArgContext) describeContextObject() string {
	if c.varName != "" {
		return "$" + c.varName
	}
	if c.entity != "" {
		return "the row object of the enclosing widget (" + c.entity + ")"
	}
	return "the enclosing context object"
}

// contextVarAlternative offers the context variable by name when it has one, so
// the message names a spelling that works rather than only one that does not.
func (c pageArgContext) contextVarAlternative() string {
	if c.varName == "" {
		return ""
	}
	return " (or $" + c.varName + ")"
}

// refuseShowPageArgument is the single wording `mxcli check` and `exec` share.
// Two copies in two currencies is how a resolver drifts, and this one already
// had to be told about a third context state.
//
// The rule ID is NOT in the text: `mxcli check` renders it from the violation,
// so embedding it here prints it twice. The exec path, which has no violation to
// render, appends it itself.
func refuseShowPageArgument(widgetName, target, argName, argValue string, c pageArgContext) string {
	where := "widget `" + widgetName + "`"
	if widgetName == "" {
		where = "this widget"
	}
	if !c.present {
		return fmt.Sprintf(
			"show page %s: argument %s: %s cannot be stored — %s is not inside a data view, list view or grid row, "+
				"so there is no context object at all, and a widget's page argument is always that object. "+
				"The page would be opened with no argument, which mxbuild reports as CE1571 \"No argument has been "+
				"selected for parameter '%s'\". Put the button inside a data widget bound to the object, pass a "+
				"parameter of this page or snippet (bound by name), or call a microflow that shows the page with it",
			target, argName, argValue, where, argName)
	}
	if c.selectionOf != "" {
		return fmt.Sprintf(
			"show page %s: argument %s: %s cannot be stored — in the control bar of `%s` a page argument is the "+
				"grid's selection, $%s, or a parameter of this page or snippet; %s names neither",
			target, argName, argValue, c.selectionOf, c.selectionOf, argValue)
	}
	return fmt.Sprintf(
		"show page %s: argument %s: %s cannot be stored — a widget's page argument is always the enclosing context "+
			"object, which mxcli records by leaving the mapping empty (an explicit one is rejected as CE0115). "+
			"Writing %s here would silently open the page with %s instead. Use $currentObject%s, or call a microflow "+
			"that shows the page with the object you want",
		target, argName, argValue, argValue, c.describeContextObject(), c.contextVarAlternative())
}

// namedParameterArgument reports whether a show page argument is a page or
// snippet parameter of the document being built, which Mendix binds by name
// rather than from a context object (mendixlabs/mxcli#1403) — or, in a grid's
// control bar, the grid's selection, kind "widget" (mendixlabs/mxcli#122). It
// returns the variable and its PageVariable kind, or "" when the argument is
// anything else
// — including a page variable, which is never an object and so never a page
// argument, and any project older than 9.7, whose page settings have no
// ParameterMappings to hold the binding.
func (pb *pageBuilder) namedParameterArgument(value string) (variable, kind string) {
	variable, kind = pb.classifyFlowArgValue(value)
	if kind != "parameter" && kind != "snippet" {
		if pb.argCtx.namesSelection(value) || pb.argCtx.namesRowSelection(value) {
			variable, kind = value, "widget"
		} else {
			return "", ""
		}
	}
	if pb.backend != nil {
		if pv := pb.backend.ProjectVersion(); pv != nil && pv.MajorVersion > 0 && !pv.IsAtLeast(9, 7) {
			return "", ""
		}
	}
	return variable, kind
}

// documentParams is held by pointer so pageArgContext stays comparable: the
// walks compare contexts to tell whether a widget changed it.
type documentParams struct {
	names map[string]bool
}

// withDocumentParameters is the context at a document root that also knows the
// document's entity-typed parameters, which a show page argument may name
// wherever it sits (mendixlabs/mxcli#1403).
func (c pageArgContext) withDocumentParameters(params []string) pageArgContext {
	if len(params) == 0 {
		return c
	}
	c.docParams = &documentParams{names: make(map[string]bool, len(params))}
	for _, p := range params {
		c.docParams.names[strings.ToLower(strings.TrimPrefix(p, "$"))] = true
	}
	return c
}

// namesDocumentParameter reports whether value is a bare `$Param` naming one of
// the document's entity-typed parameters.
func (c pageArgContext) namesDocumentParameter(value string) bool {
	name, ok := strings.CutPrefix(value, "$")
	if !ok || name == "" || strings.ContainsAny(name, "/.") {
		return false
	}
	return c.docParams != nil && c.docParams.names[strings.ToLower(name)]
}

// validateShowPageArguments is the check-time mirror of the executor guard, so
// `mxcli check` reports the ignored argument without needing a project — the same
// pairing as MDL-WIDGET09.
func validateShowPageArguments(w *ast.WidgetV3, c pageArgContext, locationPrefix string) []linter.Violation {
	if w == nil {
		return nil
	}
	action := w.GetAction()
	if action == nil || action.Type != "showPage" {
		return nil
	}
	var out []linter.Violation
	for _, arg := range action.Args {
		strVal, ok := arg.Value.(string)
		if !ok || c.binds(strVal) || c.namesDocumentParameter(strVal) || c.namesSelection(strVal) || c.namesRowSelection(strVal) {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-PAGEARG01",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf("%s: %s",
				locationPrefix, refuseShowPageArgument(w.Name, action.Target, arg.Name, strVal, c)),
		})
	}
	return out
}
