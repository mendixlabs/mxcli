// SPDX-License-Identifier: Apache-2.0

// A binding that names its object: `$Param.Attr`.
//
// A widget placed in a page or snippet outside every data container can still
// show or edit an attribute — of a page or snippet PARAMETER. Studio Pro stores
// the binding's AttributeRef (or a visibility setting's Attribute) beside a
// Forms$PageVariable that names the parameter in its PageParameter or
// SnippetParameter slot, with no Widget. Measured on TestApp's WorkflowCommons
// snippets: the combo boxes of Snip_TaskDashboard_Header /
// Snip_WorkflowDashboard_Header (attributeEnumeration and attributeAssociation
// alike), the image visibility of Snip_UserTask_NameColumnWithIcon, and the
// date picker, text areas and radio buttons of Snip_WorkflowUserTaskView_Details.
//
// MDL spells it `$Param.Attr` — the form a text template parameter already uses
// for a parameter, and the one ako/mxcli#826 gave an enclosing data view
// (`$dataView1.Attr`). Before, describe printed the attribute bare: exec then
// either refused it (there is no enclosing object) or, where it did not check,
// wrote a binding with no SourceVariable — a different document.
package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// namedObjectBinding splits `$name.rest` into name (without the "$") and rest.
// ok is false for anything else — a bare or qualified attribute, an
// association path, `$currentObject/…`.
func namedObjectBinding(s string) (name, rest string, ok bool) {
	after, isVar := strings.CutPrefix(s, "$")
	if !isVar {
		return "", "", false
	}
	name, rest, ok = strings.Cut(after, ".")
	if !ok || name == "" || rest == "" || strings.ContainsAny(name, "/ ") || strings.Contains(rest, "/") {
		return "", "", false
	}
	return name, rest, true
}

// parameterAttributeValue is the `$Param.Attr` text of a pluggable widget
// property authored by its own key. Unlike `Attribute:`, a generic key's value
// goes through the expression grammar, so `eventDataAttribute: $Ctx.EventData`
// arrives as an *ast.WidgetExpressionV3 — which every attribute property used
// to refuse as MDL-WIDGET42 (mendixlabs/mxcli#1591). Anything other than that
// one shape is still an expression.
func parameterAttributeValue(raw any) (string, bool) {
	e, ok := raw.(*ast.WidgetExpressionV3)
	if !ok || !isNamedObjectAttributeText(e.Text) {
		return "", false
	}
	return e.Text, true
}

// namedObjectBindingKinds are the widgets whose `Attribute: $name.Attr` may
// name an enclosing data view as well as a parameter (resolveInputBinding).
// Every other widget that takes the form — the combo box — reads a parameter
// only: the data-view pair is measured on the built-in inputs alone.
var namedObjectBindingKinds = map[string]bool{
	"textbox": true, "textarea": true, "checkbox": true,
	"datepicker": true, "radiobuttons": true, "dropdown": true,
}

func dataViewClause(kind string) string {
	if namedObjectBindingKinds[kind] {
		return " nor a data view enclosing the widget"
	}
	return ""
}

// parameterSourceName is the parameter a stored SourceVariable reads, when it
// is exactly a page or snippet parameter: a Forms$PageVariable with the name in
// PageParameter or SnippetParameter and no Widget or LocalVariable. "" for
// anything else — null (the enclosing object), a data view pair (#826), a page
// variable.
func parameterSourceName(sv any) string {
	m, ok := sv.(map[string]any)
	if !ok || m == nil {
		return ""
	}
	if extractString(m["Widget"]) != "" || extractString(m["LocalVariable"]) != "" {
		return ""
	}
	if name := extractString(m["SnippetParameter"]); name != "" {
		return name
	}
	return extractString(m["PageParameter"])
}

// namedObjectSourceName is the `$name` of the object a stored SourceVariable
// names: the data view of a data-view pair {Widget, PageParameter}, or else a
// page or snippet parameter — "" for none, or for a page variable. MDL spells
// the pair `$dataView1.Attr` (ako/mxcli#826, #2107).
func namedObjectSourceName(sv any) string {
	m, ok := sv.(map[string]any)
	if !ok || m == nil || extractString(m["LocalVariable"]) != "" {
		return ""
	}
	if widget := extractString(m["Widget"]); widget != "" {
		return widget
	}
	return parameterSourceName(sv)
}

// customWidgetPropertyParameter is the parameter a CustomWidget property's
// WidgetValue reads (its SourceVariable), or "".
func customWidgetPropertyParameter(w map[string]any, propertyKey string) string {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return ""
	}
	propTypeKeyMap := buildPropertyTypeKeyMap(w, false)
	for _, prop := range getBsonArrayElements(obj["Properties"]) {
		propMap, ok := prop.(map[string]any)
		if !ok || propTypeKeyMap[extractBinaryID(propMap["TypePointer"])] != propertyKey {
			continue
		}
		value, _ := propMap["Value"].(map[string]any)
		return parameterSourceName(value["SourceVariable"])
	}
	return ""
}

// parameterAttributeMDL spells an attribute read from parameter name:
// `$Param.Attr`, the attribute's own name quoted when it is a keyword.
func parameterAttributeMDL(name, attrQN string) string {
	return "$" + name + "." + mdlIdent(shortAttributeName(attrQN))
}

// isNamedObjectAttributeText reports whether text binds an attribute of a named
// object: `$Name.Attr`, or the rooted association path `$Name/Assoc/Attr`
// (ako/mxcli#833), which a pluggable attribute property takes as well
// (FactoryManagement ConversationalUI.Span_TextGeneration_SlideIn,
// ako/mxcli#2107). Neither is a Mendix expression to an attribute property.
func isNamedObjectAttributeText(text string) bool {
	if isParameterAttributeText(text) {
		return true
	}
	_, _, rooted := rootedAssociationPath(text)
	return rooted
}

// isParameterAttributeText reports whether text is the `$Param.Attr` shape.
// namedObjectBinding alone takes `$Ctx.Name + 'x'` too, which is an expression.
func isParameterAttributeText(text string) bool {
	return ast.IsParameterAttributeText(text)
}

// rootedAssociationPath splits `$Root/Assoc/…/Attr` — an association path read
// from a named object, Mendix's own expression spelling — into the root name
// and the context-relative path. At least one association hop is required:
// a direct attribute of a named object is spelled `$Root.Attr`. $currentObject
// is not a root; its path is the context-relative one.
//
// Studio Pro stores this as an AttributeRef with an IndirectEntityRef of steps
// beside a SourceVariable naming the object, e.g. a snippet parameter read
// outside every data container (TestApp WorkflowCommons.Snip_Workflow_Header,
// ako/mxcli#833).
func rootedAssociationPath(s string) (root, path string, ok bool) {
	after, isVar := strings.CutPrefix(strings.TrimSpace(s), "$")
	if !isVar {
		return "", "", false
	}
	root, path, ok = strings.Cut(after, "/")
	if !ok || root == "" || !strings.Contains(path, "/") || strings.EqualFold(root, "currentObject") ||
		strings.ContainsAny(root, ". ") || strings.ContainsAny(path, " ()=") {
		return "", "", false
	}
	return root, path, true
}

// rootedAttributeValue reads `Attribute: $Root/Assoc/Attr`, which the grammar
// parses as a data-source expression rather than an attribute path, back into
// its root and path.
func rootedAttributeValue(raw any) (root, path string, ok bool) {
	ds, isDS := raw.(*ast.DataSourceV3)
	if !isDS || ds.ContextVariable == "" || len(ds.Args) > 0 || ds.Where != "" || len(ds.OrderBy) > 0 {
		return "", "", false
	}
	return rootedAssociationPath("$" + ds.ContextVariable + "/" + ds.Reference)
}

// inputAttributeText is an input widget's `Attribute:` as the text the input
// builders resolve: the attribute path as written, or `$Root/Assoc/Attr` for a
// path read from a named object (ako/mxcli#833).
func inputAttributeText(w *ast.WidgetV3) string {
	if attr := w.GetAttribute(); attr != "" {
		return attr
	}
	raw, _ := lookupPropCI(w, "Attribute")
	if root, path, ok := rootedAttributeValue(raw); ok {
		return "$" + root + "/" + path
	}
	return ""
}
