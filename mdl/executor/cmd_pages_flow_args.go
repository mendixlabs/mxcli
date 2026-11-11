// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// classifyFlowArgValue decides how an action or data-source argument binds.
//
// Mendix stores a flow argument two ways and does not treat them as
// interchangeable: a reference to a page parameter, snippet parameter or page
// variable is a Forms$PageVariable under the mapping's Variable, while a literal
// or expression is text under Expression. Writing a $-reference as an Expression
// leaves the parameter unbound — Studio Pro reports CE1571 "No argument has been
// selected for parameter 'X' and no default is available" — and mxbuild builds
// that same document at 0 errors, so nothing catches it before the page is opened
// (mendixlabs/mxcli#1140).
//
// The four places that built a mapping each carried their own copy of the old
// `strings.HasPrefix(v, "$")` rule (show_page, microflow, nanoflow, data source).
// One classifier instead: the same argument must bind the same way wherever it is
// written, or the spelling decides the storage.
//
// kind is "" when the value is not a page-variable reference, which leaves the
// caller's existing behaviour untouched. Deliberately in that bucket:
//
//   - $currentObject — the enclosing context object. No Studio Pro reference for
//     the bare form was measured, and mxcli's show_page handling already depends
//     on the context object being inferred rather than named (MDL-PAGEARG01), so
//     changing it on a guess would risk the case that works.
//   - a path or dotted expression ($obj/Module.Assoc, $p.Attr) — an expression,
//     which is where Mendix stores it.
//   - a name that is not a declared object-typed parameter or variable of the
//     document being built — including a primitive parameter, which Mendix binds
//     as an expression (all six Expression-bound mappings in the Workflow Commons
//     reference are Boolean literals).
func (pb *pageBuilder) classifyFlowArgValue(value string) (variable, kind string) {
	name, ok := strings.CutPrefix(value, "$")
	if !ok || name == "" {
		return "", ""
	}
	if strings.ContainsAny(name, "/.") {
		return "", ""
	}
	if strings.EqualFold(name, "currentObject") {
		return "", ""
	}
	if pb.localVariables[name] {
		return value, "local"
	}
	// paramScope holds only the entity-typed parameters, which is the set Mendix
	// binds through a PageVariable.
	if _, isParam := pb.paramScope[name]; isParam {
		if pb.isSnippet {
			return value, "snippet"
		}
		return value, "parameter"
	}
	// A widget of the document: its selection, bound by naming the widget
	// (ako/mxcli#1560). After the parameters and variables, because that is
	// what `$name` means when both exist.
	if pb.isDocumentWidget != nil && pb.isDocumentWidget(name) {
		return value, "widget"
	}
	return "", ""
}

// flowArgWidgetParameter is the parameter a "widget" flow argument names beside
// the widget, when that widget is an enclosing data view showing a page or
// snippet parameter: the pair registerDataViewVariable records for the data
// view's `$dv.Attr` bindings (ako/mxcli#826). Studio Pro wants the same pair on
// a flow argument — Widget alone there is mx check CE0115 "The arguments that
// are passed to microflow '…' do not match the expected parameters"
// (ako/mxcli#2095), while hand-patching PageParameter beside it measured 0
// errors. A grid's or combo box's selection, and a data view whose object comes
// from a microflow, have no parameter behind them and keep Widget alone (#1560).
func (pb *pageBuilder) flowArgWidgetParameter(variable, kind string) (param, paramKind string) {
	if kind != "widget" {
		return "", ""
	}
	wv, ok := pb.dataViewVariables[strings.TrimPrefix(variable, "$")]
	if !ok || wv.Variable == "" {
		return "", ""
	}
	return wv.Variable, wv.Kind
}

// pageVariableArgValue renders a mapping's Variable — a Forms$PageVariable — as
// the MDL `$name` that produced it, or "" when there is no variable binding.
//
// This is the read half of #1140, and it was wrong in a way that hid the write
// half. The three describe readers looked for a `Name` key on the sub-document;
// Forms$PageVariable has no such property. Studio Pro names the reference in
// whichever of PageParameter / SnippetParameter / LocalVariable applies, so every
// argument in Studio Pro-authored content described as absent: measured on
// Workflow Commons 4.11.0, `DESCRIBE PAGE` printed
//
//	Action: microflow WorkflowCommons.ACT_ConflictedWorkflowHelper_ApplyJumpTo
//
// for a button whose stored mapping binds $ConflictedWorkflowHelper — the same
// describe → exec loss as #835, one storage form over.
//
// Widget is not read here. It names the widget whose SELECTION supplies the
// argument; a flow call reads it through flowArgVariableValue, and a snippet
// call, where no Studio Pro-authored mapping has one, not at all.
func pageVariableArgValue(raw any) string {
	v, ok := raw.(map[string]any)
	if !ok || v == nil {
		return ""
	}
	for _, key := range []string{"PageParameter", "SnippetParameter", "LocalVariable"} {
		if name := extractString(v[key]); name != "" {
			return "$" + name
		}
	}
	return ""
}

// flowArgVariableValue is pageVariableArgValue for a flow call's argument — an
// action's or a data source's — which may also be a widget's selection: a
// PageVariable naming only the widget, written `$widget`. Studio Pro stores it
// for a control-bar button passing its grid (TestApp UserTask_Assign: grid2)
// and for a button passing a combo box's selection (User_WorkflowGroup_Select:
// comboBox1). describe printed the call with the argument gone, so check
// reported CE1571 on a page that builds and exec wrote the call unbound
// (ako/mxcli#1560); the builder binds `$widget` back to the same slot.
//
// The Widget slot is read first: a data view showing a page or snippet
// parameter is stored as the pair, Widget plus that parameter, and naming the
// parameter instead would make describe -> exec drop the widget
// (ako/mxcli#2095). The builder writes `$widget` back as the same pair.
func flowArgVariableValue(raw any) string {
	if v, ok := raw.(map[string]any); ok {
		if w := extractString(v["Widget"]); w != "" {
			return "$" + w
		}
	}
	return pageVariableArgValue(raw)
}

// widgetNameSet is isDocumentWidget for a statement that declares the whole
// widget tree, as CREATE PAGE, SNIPPET and LAYOUT do: every name in it, wherever
// it sits — a combo box's selection is passed from a sibling's footer, not only
// from a grid's control bar.
func widgetNameSet(widgets []*ast.WidgetV3) func(name string) bool {
	names := map[string]bool{}
	collectWidgetNames(widgets, names)
	return func(name string) bool { return names[name] }
}
