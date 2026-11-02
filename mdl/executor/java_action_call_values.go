// SPDX-License-Identifier: Apache-2.0

// The values of a `call java action` parameter that is not an expression.
//
// A Java action parameter is mostly bound to an expression
// (Microflows$BasicCodeActionParameterValue). Two parameter types take
// something else, and mxcli wrote an expression for both, which mxbuild
// rejects with CE0115 "The arguments that are passed to Java action … do not
// match the expected parameters" (ako/mxcli#1451, mendixlabs/mxcli#1554):
//
//   - a text template, CodeActions$StringTemplateParameterType
//     (BusinessEvents.PublishBusinessEvent_V2's Subject), stores a
//     Microflows$StringTemplateParameterValue: a TypedTemplate with the text and
//     one Microflows$PrimitiveTypedTemplateArgument per {n} placeholder, each
//     an expression and the type of its result. MDL spells it the way log and
//     show message spell a template:
//
//     Subject = 'Order {1} shipped' with ({1} = $Order/Number)
//
//     A string literal is the text; any other expression is the template '{1}'
//     with the expression as its one argument, as for a REST call body
//     (restCallTemplate, ako/mxcli#1444).
//
//   - a document of a custom type, CodeActions$CustomBlobDocumentParameterType
//     (AgentEditorCommons' Agent), stores a
//     Microflows$CustomBlobDocumentCodeActionParameterValue naming the document.
//     Like a Microflow parameter (#1210) it takes a name, never an expression.
//
// No Studio Pro-authored document of either value exists in the repo's fixtures
// or on this machine; the shapes are the metamodel's (generated/metamodel,
// Microflows$TypedTemplate / PrimitiveTypedTemplateArgument /
// CustomBlobDocumentCodeActionParameterValue), and were measured against
// mxbuild 11.13: it accepts both, reports CE0117 when an argument's Type is not
// its expression's type (an absent Type reads as String, and an enumeration is
// accepted as none of the five), CE0720 for a placeholder with no argument,
// CE0115 for a document of the wrong type and CE1613 for a missing one.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// javaActionTemplateValue builds the value of a text-template parameter from
// its argument. The second result is why the argument cannot be written, or "".
func (fb *flowBuilder) javaActionTemplateValue(actionQN string, arg ast.CallArgument) (*microflows.StringTemplateParameterValue, string) {
	value := &microflows.StringTemplateParameterValue{
		BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
		TypedTemplate: &microflows.TypedTemplate{BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())}},
	}
	if isEmptyJavaActionArgument(arg.Value) && len(arg.TemplateParams) == 0 {
		return value, "" // an empty template: no text, no arguments
	}
	if msg := templateArgumentShapeRefusal(actionQN, arg); msg != "" {
		return value, msg
	}
	text, params := restCallTemplate(javaActionArgumentValue(arg.Value), arg.TemplateParams)
	s, _ := restTemplateText(text)
	value.TypedTemplate.Text = s
	// MDL-TEMPLATE02 (ako/mxcli#1397): the text's braces, judged by the
	// function check uses. Check sees a template only where `with (…)` says so;
	// here the parameter's type says so for a plain string too.
	if msg, fix := templateTextRefusal(javaActionTemplateWhere(actionQN, arg.Name), s, len(orderedTemplateParams(params))); msg != "" {
		return value, msg + "\n  " + fix
	}
	for i, p := range orderedTemplateParams(params) {
		if p == nil {
			return value, fmt.Sprintf("call java action %s: parameter %s: the template has no {%d} argument "+
				"in with (…); number the arguments {1}, {2}, … without gaps", actionQN, arg.Name, i+1)
		}
		typ, problem := fb.templateArgumentType(p.Value)
		if problem != "" {
			return value, fmt.Sprintf("call java action %s: parameter %s: {%d} = %s %s",
				actionQN, arg.Name, i+1, fb.exprToString(p.Value), problem)
		}
		value.TypedTemplate.Arguments = append(value.TypedTemplate.Arguments, &microflows.TypedTemplateArgument{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Expression:  fb.exprToString(p.Value),
			Type:        typ,
		})
	}
	return value, ""
}

// templateArgumentShapeRefusal returns why a text-template argument cannot be
// written, judged from the statement alone, or "": an expression that also
// states `with (…)` has no {n} to fill. The flow builder refuses the call with
// it and reference validation reports it, so the two cannot disagree.
func templateArgumentShapeRefusal(actionQN string, arg ast.CallArgument) string {
	if len(arg.TemplateParams) == 0 {
		return ""
	}
	if _, ok := restTemplateText(javaActionArgumentValue(arg.Value)); ok {
		return ""
	}
	return fmt.Sprintf("call java action %s: parameter %s is an expression, so `with (…)` has no template to "+
		"fill — the arguments fill the {n} placeholders of a text written as a string. Write %s = '…{1}…' "+
		"with ({1} = …), or %s = <expression> without `with (…)`", actionQN, arg.Name, arg.Name, arg.Name)
}

// notTemplateParamRefusal is the refusal for `with (…)` on a parameter that is
// not a text template.
func notTemplateParamRefusal(actionQN, param string) string {
	return fmt.Sprintf("call java action %s: parameter %s is not a text template, so `with (…)` has nothing "+
		"to fill; bind it to an expression", actionQN, param)
}

// orderedTemplateParams lays params out by placeholder index: element i is
// {i+1}, nil where no parameter states it.
func orderedTemplateParams(params []ast.TemplateParam) []*ast.TemplateParam {
	maxIndex := 0
	for _, p := range params {
		if p.Index > maxIndex {
			maxIndex = p.Index
		}
	}
	out := make([]*ast.TemplateParam, maxIndex)
	for i := range params {
		if p := &params[i]; p.Index > 0 {
			out[p.Index-1] = p
		}
	}
	return out
}

// templateArgumentType returns the Type a typed-template argument stores for
// expr — the type of its result, one of the five a typed template takes — or,
// as the second result, why expr cannot be an argument. mxbuild checks the
// expression against the stored Type (CE0117), so a guess is not good enough
// where the type is known; where it cannot be told it is String, which is
// also what an absent Type means.
func (fb *flowBuilder) templateArgumentType(expr ast.Expression) (string, string) {
	src := fb.exprToString(expr)
	kinds := flowExprTypes{fb: fb}
	kind := exprcheck.InferArgumentKind(src, exprcheck.Context{Scope: kinds, Entities: kinds, Catalog: kinds})
	switch kind {
	case exprcheck.KindInteger, exprcheck.KindLong:
		return "Integer", ""
	case exprcheck.KindDecimal:
		return "Decimal", ""
	case exprcheck.KindBoolean:
		return "Boolean", ""
	case exprcheck.KindDateTime:
		return "DateTime", ""
	case exprcheck.KindEnumeration:
		return "", "is an enumeration, which a text template cannot take — " +
			"pass getKey(…) or getCaption(…) of it"
	case exprcheck.KindObject, exprcheck.KindList, exprcheck.KindBinary:
		return "", "is not a String, Integer, Decimal, Boolean or DateTime value, " +
			"the five a text template takes"
	}
	return "String", ""
}

// flowExprTypes types a flow expression from what the flow builder knows: its
// declared primitive variables, its object variables, and the project's
// domain models through the backend. It is exprcheck's Scope, EntityScope and
// CatalogReader at once; what it cannot answer stays unknown.
type flowExprTypes struct{ fb *flowBuilder }

var (
	_ exprcheck.Scope         = flowExprTypes{}
	_ exprcheck.EntityScope   = flowExprTypes{}
	_ exprcheck.CatalogReader = flowExprTypes{}
)

func (t flowExprTypes) Lookup(name string) (exprcheck.TypeKind, bool) {
	typ, ok := t.fb.declaredVars[strings.TrimPrefix(name, "$")]
	if !ok {
		return exprcheck.KindUnknown, false
	}
	switch typ {
	case "String", "HashedString", "StringTemplate":
		return exprcheck.KindString, true
	case "Integer", "AutoNumber":
		return exprcheck.KindInteger, true
	case "Long":
		return exprcheck.KindLong, true
	case "Decimal":
		return exprcheck.KindDecimal, true
	case "Boolean":
		return exprcheck.KindBoolean, true
	case "DateTime", "Date":
		return exprcheck.KindDateTime, true
	case "Binary":
		return exprcheck.KindBinary, true
	case "Enumeration":
		return exprcheck.KindEnumeration, true
	}
	return exprcheck.KindUnknown, false
}

func (t flowExprTypes) VariableEntity(name string) (string, bool) {
	typ := t.fb.varTypes[strings.TrimPrefix(name, "$")]
	if typ == "" || strings.HasPrefix(typ, "List of ") {
		return "", false
	}
	return typ, true
}

func (t flowExprTypes) AssociationTarget(assocQN, fromEntityQN string) (string, bool) {
	module, name, ok := strings.Cut(assocQN, ".")
	if !ok {
		return "", false
	}
	info := t.fb.lookupAssociation(module, name)
	if info == nil {
		return "", false
	}
	if info.childEntityQN == fromEntityQN {
		return info.parentEntityQN, info.parentEntityQN != ""
	}
	return info.childEntityQN, info.childEntityQN != ""
}

func (t flowExprTypes) AttributeKind(entityQN, attrName string) (exprcheck.TypeKind, bool) {
	attr := t.fb.findAttribute(entityQN, attrName)
	if attr == nil {
		return exprcheck.KindUnknown, false
	}
	switch attr.Type.(type) {
	case *domainmodel.StringAttributeType, *domainmodel.HashedStringAttributeType:
		return exprcheck.KindString, true
	case *domainmodel.IntegerAttributeType, *domainmodel.AutoNumberAttributeType:
		return exprcheck.KindInteger, true
	case *domainmodel.LongAttributeType:
		return exprcheck.KindLong, true
	case *domainmodel.DecimalAttributeType:
		return exprcheck.KindDecimal, true
	case *domainmodel.BooleanAttributeType:
		return exprcheck.KindBoolean, true
	case *domainmodel.DateTimeAttributeType, *domainmodel.DateAttributeType:
		return exprcheck.KindDateTime, true
	case *domainmodel.EnumerationAttributeType:
		return exprcheck.KindEnumeration, true
	case *domainmodel.BinaryAttributeType:
		return exprcheck.KindBinary, true
	}
	return exprcheck.KindUnknown, false
}

func (t flowExprTypes) AttributeEnumQN(entityQN, attrName string) (string, bool) {
	if attr := t.fb.findAttribute(entityQN, attrName); attr != nil {
		if et, ok := attr.Type.(*domainmodel.EnumerationAttributeType); ok {
			return et.EnumerationRef, et.EnumerationRef != ""
		}
	}
	return "", false
}

func (flowExprTypes) EnumCases(string) ([]string, bool) { return nil, false }
func (flowExprTypes) MicroflowReturn(string) (exprcheck.TypeKind, bool) {
	return exprcheck.KindUnknown, false
}
func (flowExprTypes) MicroflowParam(string, string) (exprcheck.TypeKind, bool) {
	return exprcheck.KindUnknown, false
}

// findAttribute finds attrName on entityQN or on one of its generalizations.
func (fb *flowBuilder) findAttribute(entityQN, attrName string) *domainmodel.Attribute {
	if fb == nil || fb.backend == nil {
		return nil
	}
	seen := map[string]bool{}
	for current := entityQN; current != "" && !seen[current]; {
		seen[current] = true
		module, name, ok := strings.Cut(current, ".")
		if !ok {
			return nil
		}
		mod, err := fb.backend.GetModuleByName(module)
		if err != nil || mod == nil {
			return nil
		}
		dm, err := fb.backend.GetDomainModel(mod.ID)
		if err != nil || dm == nil {
			return nil
		}
		entity := dm.FindEntityByName(name)
		if entity == nil {
			return nil
		}
		for _, attr := range entity.Attributes {
			if attr != nil && attr.Name == attrName {
				return attr
			}
		}
		current = entity.GeneralizationRef
	}
	return nil
}

// javaActionDocumentValue builds the value of a custom-document parameter:
// the document's qualified name. `empty` leaves it unset.
func javaActionDocumentValue(actionQN string, arg ast.CallArgument) (*microflows.CustomBlobDocumentCodeActionParameterValue, string) {
	value := &microflows.CustomBlobDocumentCodeActionParameterValue{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
	}
	if isEmptyJavaActionArgument(arg.Value) {
		return value, ""
	}
	name, msg := documentParamArgName(actionQN, arg.Name, arg.Value)
	value.CustomDocument = name
	return value, msg
}

// documentParamArgName returns the document a custom-document parameter's
// argument names — `Module.Doc` or 'Module.Doc' — or why it names none. The
// flow builder refuses the call with it and reference validation reports it.
func documentParamArgName(action, param string, v ast.Expression) (string, string) {
	v = javaActionArgumentValue(v)
	switch e := v.(type) {
	case *ast.QualifiedNameExpr:
		return e.QualifiedName.String(), ""
	case *ast.LiteralExpr:
		if s, ok := e.Value.(string); ok && e.Kind == ast.LiteralString && microflowIdentifierRe.MatchString(s) {
			return s, ""
		}
	}
	return "", fmt.Sprintf("call java action %s: parameter %s takes a document, named as Module.Document, "+
		"not %s — Mendix stores a reference to the document, so there is no expression form",
		action, param, expressionToString(v))
}

// customDocumentExists reports whether module.name is a document of the custom
// type docType, and whether that could be judged at all.
func customDocumentExists(ctx *ExecContext, docType, qn string) (exists, judged bool) {
	module, name, ok := strings.Cut(qn, ".")
	if !ok {
		return false, false
	}
	switch docType {
	case "agenteditor.agent":
		return findAgentEditorAgent(ctx, module, name) != nil, true
	case "agenteditor.model":
		return findAgentEditorModel(ctx, module, name) != nil, true
	case "agenteditor.knowledgebase":
		return findAgentEditorKnowledgeBase(ctx, module, name) != nil, true
	case "agenteditor.consumedMCPService":
		return findAgentEditorConsumedMCPService(ctx, module, name) != nil, true
	}
	return false, false
}

// templateTypedParams names the parameters a CREATE JAVA ACTION declares as a
// text template (`StringTemplate(Text)` / `StringTemplate(Sql)`).
func templateTypedParams(params []ast.JavaActionParam) map[string]bool {
	out := map[string]bool{}
	for _, p := range params {
		if p.Type.Kind == ast.TypeStringTemplate {
			out[p.Name] = true
		}
	}
	return out
}

// javaActionCallValueErrors is reference validation's copy of the flow
// builder's refusals for one call's text-template and document arguments,
// given which of the action's parameters are templates and which documents
// (name -> CustomDocumentTypeName). A document argument is also resolved: a
// missing one is CE1613 at build time, one of another type CE0115. Documents
// the script creates count as present.
func javaActionCallValueErrors(ctx *ExecContext, sc *scriptContext, ref codeActionCallRef,
	templates map[string]bool, documents map[string]string) []string {
	var out []string
	for _, a := range ref.args {
		switch {
		case templates[a.Name]:
			if msg := templateArgumentShapeRefusal(ref.name, a); msg != "" {
				out = append(out, msg)
			}
		case documents[a.Name] != "":
			if isEmptyJavaActionArgument(a.Value) {
				continue
			}
			name, msg := documentParamArgName(ref.name, a.Name, a.Value)
			if msg != "" {
				out = append(out, msg)
				continue
			}
			docType := documents[a.Name]
			if sc != nil && sc.customDocuments[docType+" "+name] {
				continue
			}
			if exists, judged := customDocumentExists(ctx, docType, name); judged && !exists {
				out = append(out, fmt.Sprintf("call java action %s: parameter %s: there is no %s document %s "+
					"(a missing document is CE1613 at build time, one of another type CE0115)",
					ref.name, a.Name, docType, name))
			}
		default:
			if len(a.TemplateParams) > 0 {
				out = append(out, notTemplateParamRefusal(ref.name, a.Name))
			}
		}
	}
	return out
}

// checkJavaActionTemplateArgs is MDL-JA02: an argument written as an
// expression followed by `with (…)`. Whatever the parameter is, that cannot be
// written — a text template takes the parameters on a string, and any other
// parameter takes none — so it needs no project to judge; the builder refuses
// it with the same function.
func (v *microflowValidator) checkJavaActionTemplateArgs(stmt *ast.CallJavaActionStmt) {
	for _, arg := range stmt.Arguments {
		if msg := templateArgumentShapeRefusal(stmt.ActionName.String(), arg); msg != "" {
			v.addViolation("MDL-JA02", linter.SeverityError, msg,
				"See `mxcli syntax microflow.call` for text-template arguments")
		}
	}
}
