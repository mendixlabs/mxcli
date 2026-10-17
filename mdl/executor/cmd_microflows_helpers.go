// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow helper functions
package executor

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/mendixexpr"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// convertASTToMicroflowDataType converts an AST DataType to a microflows.DataType.
// entityResolver is optional - if provided, it resolves entity qualified names to IDs.
func convertASTToMicroflowDataType(dt ast.DataType, entityResolver func(ast.QualifiedName) model.ID) microflows.DataType {
	switch dt.Kind {
	case ast.TypeBoolean:
		return &microflows.BooleanType{}
	case ast.TypeInteger:
		return &microflows.IntegerType{}
	case ast.TypeLong:
		return &microflows.LongType{}
	case ast.TypeDecimal:
		return &microflows.DecimalType{}
	case ast.TypeString:
		return &microflows.StringType{}
	case ast.TypeDateTime:
		return &microflows.DateTimeType{}
	case ast.TypeDate:
		return &microflows.DateType{}
	case ast.TypeBinary:
		return &microflows.BinaryType{}
	case ast.TypeVoid:
		return &microflows.VoidType{}
	case ast.TypeEntity:
		lt := &microflows.ObjectType{}
		if dt.EntityRef != nil {
			// Set qualified name for BY_NAME_REFERENCE serialization
			lt.EntityQualifiedName = dt.EntityRef.Module + "." + dt.EntityRef.Name
			if entityResolver != nil {
				lt.EntityID = entityResolver(*dt.EntityRef)
			}
		}
		return lt
	case ast.TypeListOf:
		lt := &microflows.ListType{}
		if dt.EntityRef != nil {
			// Set qualified name for BY_NAME_REFERENCE serialization
			lt.EntityQualifiedName = dt.EntityRef.Module + "." + dt.EntityRef.Name
			if entityResolver != nil {
				lt.EntityID = entityResolver(*dt.EntityRef)
			}
		}
		return lt
	case ast.TypeEnumeration:
		et := &microflows.EnumerationType{}
		if dt.EnumRef != nil {
			// Set qualified name for BY_NAME_REFERENCE serialization
			et.EnumerationQualifiedName = dt.EnumRef.Module + "." + dt.EnumRef.Name
		}
		return et
	default:
		return &microflows.VoidType{}
	}
}

// bareEnumerationType resolves the entity/enumeration ambiguity of a declared
// type. The parser cannot tell an entity from an enumeration for a bare
// `Module.Name` and always records an entity reference, so a caller that has
// found no entity of that name asks here whether an enumeration has it. It
// returns dt retyped as that enumeration, or false when none does.
//
// Parameters and return types of microflows, nanoflows and rules all resolve
// through this, so the three kinds accept the same spellings; `check` makes the
// same decision for a project that is not yet written in declaredTypeResolver.
func bareEnumerationType(ctx *ExecContext, dt ast.DataType) (ast.DataType, bool) {
	if dt.EntityRef == nil || findEnumeration(ctx, dt.EntityRef.Module, dt.EntityRef.Name) == nil {
		return dt, false
	}
	return ast.DataType{Kind: ast.TypeEnumeration, EnumRef: dt.EntityRef}, true
}

// The Mendix expression renderer lives in mdl/mendixexpr, where the visitor
// can reach it too (fmt --upgrade, ako/mxcli#804). These names are the
// executor's handles on it.

func mendixFunctionName(name string) string         { return mendixexpr.FunctionName(name) }
func quoteExpressionLiteral(s string) string        { return mendixexpr.QuoteLiteral(s) }
func expressionToString(expr ast.Expression) string { return mendixexpr.String(expr) }
func normalizeMendixOperatorCase(src string) string { return mendixexpr.NormalizeOperatorCase(src) }
func isWordByte(c byte) bool                        { return mendixexpr.IsWordByte(c) }

// expressionToXPath converts an AST Expression to an XPath constraint string.
// Unlike expressionToString (for Mendix expressions), XPath requires Mendix
// tokens like [%CurrentDateTime%] to be quoted: '[%CurrentDateTime%]'.
func expressionToXPath(expr ast.Expression) string {
	return xpathOf(expr, false)
}

// expressionToXPathNames is expressionToXPath with every qualified name left as
// written, for a writer that decides what a three-part name is itself — an
// attribute of the constrained entity or an enumeration value — with
// storedXPathConstraint (ako/mxcli#874).
func expressionToXPathNames(expr ast.Expression) string {
	return xpathOf(expr, true)
}

func xpathOf(expr ast.Expression, keepNames bool) string {
	if expr == nil {
		return ""
	}
	if reflect.ValueOf(expr).IsNil() {
		return ""
	}

	switch e := expr.(type) {
	case *ast.TokenExpr:
		return "'[%" + e.Token + "%]'"
	case *ast.BinaryExpr:
		left := xpathOf(e.Left, keepNames)
		right := xpathOf(e.Right, keepNames)
		op := strings.ToLower(e.Operator)
		return left + " " + op + " " + right
	case *ast.UnaryExpr:
		operand := xpathOf(e.Operand, keepNames)
		op := strings.ToLower(e.Operator)
		// For 'not' with parenthesized operand, output as not(expr)
		if op == "not" {
			if p, ok := e.Operand.(*ast.ParenExpr); ok {
				return "not(" + xpathOf(p.Inner, keepNames) + ")"
			}
			return "not(" + operand + ")"
		}
		return op + " " + operand
	case *ast.ParenExpr:
		return "(" + xpathOf(e.Inner, keepNames) + ")"
	case *ast.XPathPathExpr:
		return xpathPathOf(e, keepNames)
	case *ast.FunctionCallExpr:
		var args []string
		for _, arg := range e.Arguments {
			args = append(args, xpathOf(arg, keepNames))
		}
		return mendixFunctionName(e.Name) + "(" + strings.Join(args, ", ") + ")"
	case *ast.LiteralExpr:
		if e.Kind == ast.LiteralEmpty {
			return "empty"
		}
		return expressionToString(expr)
	case *ast.QualifiedNameExpr:
		if keepNames {
			return e.QualifiedName.String()
		}
		return qualifiedNameToXPath(e)
	case *ast.SourceExpr:
		if e.Source != "" {
			return e.Source
		}
		return xpathOf(e.Expression, keepNames)
	default:
		// For all other expression types, the standard serialization is correct
		return expressionToString(expr)
	}
}

// qualifiedNameToXPath converts a QualifiedNameExpr to XPath format.
// XPath constraints are evaluated at the database level where enum values are stored as plain strings.
// 3-part names (Module.EnumName.Value) must be converted to string literals ('Value').
// 2-part names (Module.AssocName) are association references and are passed through as-is.
func qualifiedNameToXPath(e *ast.QualifiedNameExpr) string {
	if dotIdx := strings.LastIndex(e.QualifiedName.Name, "."); dotIdx >= 0 {
		return "'" + e.QualifiedName.Name[dotIdx+1:] + "'"
	}
	return e.QualifiedName.String()
}

// xpathEnumRefRe matches 3-part qualified enum value references like Module.EnumName.Value
// in a raw XPath string. These must be replaced with string literals for database queries.
//
// The value may start with an underscore: Studio Pro names a True/False
// enumeration's values `_True` and `_False`. Requiring a letter there wrote
// `[A = M.E._True]` verbatim, which mxbuild rejects with CE0161.
var xpathEnumRefRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]*\.[A-Za-z][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_]*`)

// normalizeXPathEnumRefs converts 3-part qualified enum value references in a raw XPath
// string to the string literal format that Mendix database queries require.
// Example: "[Status = XpathTest.OrderStatus.Open]" → "[Status = 'Open']".
// This handles the SourceExpr (bracketed) path where qualifiedNameToXPath is bypassed.
//
// INSIDE A STRING LITERAL the letters are data and are left alone, the same rule
// NormalizeXPathOperators follows. Running the regex over the whole constraint
// rewrote a ref the author had already quoted into a DOUBLED quote:
//
//	'Mod.Enum.Value'  ->  ''Value''
//
// which every downstream reader lexes as
// an empty string literal followed by a stray bare word. The stored constraint
// then read `Status = Mod.Enum.` with the value, the closing quote and the ENTIRE
// following clause gone, `mxcli check` passing, and mxbuild reporting only a
// generic CE0161 that does not say which clause (ako/CapTrackV3 FINDINGS §29).
//
// A quoted ref is left exactly as written rather than repaired. Consuming the
// author's quotes would make `Name = 'a.b.c'` — an ordinary literal that happens
// to have two dots — silently match something else, and telling those two apart
// needs the project's enumerations, which this string-level pass does not have.
// MDL-XPATH01 reports the quoted spelling at check time, where the enum can
// actually be resolved.
func normalizeXPathEnumRefs(xpath string) string {
	var b strings.Builder
	b.Grow(len(xpath))
	for i := 0; i < len(xpath); {
		if xpath[i] != '\'' {
			j := strings.IndexByte(xpath[i:], '\'')
			if j < 0 {
				b.WriteString(xpathEnumRefRe.ReplaceAllStringFunc(xpath[i:], enumRefToLiteral))
				break
			}
			b.WriteString(xpathEnumRefRe.ReplaceAllStringFunc(xpath[i:i+j], enumRefToLiteral))
			i += j
			continue
		}
		b.WriteString(xpath[i:xpathLiteralEnd(xpath, i)])
		i = xpathLiteralEnd(xpath, i)
	}
	return b.String()
}

// enumRefToLiteral turns `Module.Enum.Value` into the `'Value'` a database query
// compares against.
func enumRefToLiteral(match string) string {
	return "'" + match[strings.LastIndex(match, ".")+1:] + "'"
}

// xpathLiteralEnd returns the index just past the string literal starting at
// start. A doubled quote inside one is an escaped quote, not the end — the same
// rule the MDL expression scanner and NormalizeXPathOperators use. An unclosed
// literal runs to the end of the constraint, which keeps the caller's loop
// terminating on malformed input instead of stalling.
func xpathLiteralEnd(s string, start int) int {
	for j := start + 1; j < len(s); j++ {
		if s[j] != '\'' {
			continue
		}
		if j+1 < len(s) && s[j+1] == '\'' {
			j++
			continue
		}
		return j + 1
	}
	return len(s)
}

// memberExpressionToString converts an AST Expression to a Mendix expression string,
// resolving enum string literals to qualified enum names when the attribute type is known.
// For example, 'Processing' becomes MyModule.ENUM_Status.Processing when the attribute
// is of type Enumeration(MyModule.ENUM_Status).
func (fb *flowBuilder) memberExpressionToString(expr ast.Expression, entityQN, attrName string) string {
	// Only transform string literals for enum attributes
	if lit, ok := expr.(*ast.LiteralExpr); ok && lit.Kind == ast.LiteralString {
		if enumRef := fb.lookupEnumRef(entityQN, attrName); enumRef != "" {
			// Convert 'Value' to Module.EnumName.Value
			return enumRef + "." + fmt.Sprintf("%v", lit.Value)
		}
	}
	return fb.exprToString(expr)
}

// lookupEnumRef returns the enumeration qualified name (e.g., "MyModule.ENUM_Status")
// for an attribute if it is an enumeration type. Returns "" if the attribute is not
// an enumeration or if the domain model is not available.
func (fb *flowBuilder) lookupEnumRef(entityQN, attrName string) string {
	if fb.backend == nil || entityQN == "" || attrName == "" {
		return ""
	}
	parts := strings.SplitN(entityQN, ".", 2)
	if len(parts) != 2 {
		return ""
	}
	mod, err := fb.backend.GetModuleByName(parts[0])
	if err != nil || mod == nil {
		return ""
	}
	dm, err := fb.backend.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return ""
	}
	for _, entity := range dm.Entities {
		if entity.Name == parts[1] {
			for _, attr := range entity.Attributes {
				if attr.Name == attrName {
					if enumType, ok := attr.Type.(*domainmodel.EnumerationAttributeType); ok {
						return enumType.EnumerationRef
					}
					return ""
				}
			}
			return ""
		}
	}
	return ""
}

// ============================================================================
// XPath Enum Enrichment (DESCRIBE output)
// ============================================================================

// enrichXPathExprWithEnums walks an XPath AST and replaces string-literal comparisons
// against known enum attributes with QualifiedNameExpr references, so DESCRIBE output
// reads as Module.EnumName.Value rather than 'Value'.
//
// enumAttrs maps bare attribute name → enumeration qualified name, e.g.
// "Status" → "XpathTest.OrderStatus".
func enrichXPathExprWithEnums(expr ast.Expression, enumAttrs map[string]string) ast.Expression {
	if expr == nil || len(enumAttrs) == 0 {
		return expr
	}
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		// attr = 'Value' or 'Value' = attr
		if attr := xpathBareAttrName(e.Left); attr != "" {
			if enumRef, ok := enumAttrs[attr]; ok {
				if lit, ok2 := e.Right.(*ast.LiteralExpr); ok2 && lit.Kind == ast.LiteralString {
					return &ast.BinaryExpr{
						Left:     e.Left,
						Operator: e.Operator,
						Right:    enumStringToQN(enumRef, fmt.Sprintf("%v", lit.Value)),
					}
				}
			}
		} else if attr := xpathBareAttrName(e.Right); attr != "" {
			if enumRef, ok := enumAttrs[attr]; ok {
				if lit, ok2 := e.Left.(*ast.LiteralExpr); ok2 && lit.Kind == ast.LiteralString {
					return &ast.BinaryExpr{
						Left:     enumStringToQN(enumRef, fmt.Sprintf("%v", lit.Value)),
						Operator: e.Operator,
						Right:    e.Right,
					}
				}
			}
		}
		return &ast.BinaryExpr{
			Left:     enrichXPathExprWithEnums(e.Left, enumAttrs),
			Operator: e.Operator,
			Right:    enrichXPathExprWithEnums(e.Right, enumAttrs),
		}
	case *ast.UnaryExpr:
		return &ast.UnaryExpr{Operator: e.Operator, Operand: enrichXPathExprWithEnums(e.Operand, enumAttrs)}
	case *ast.ParenExpr:
		return &ast.ParenExpr{Inner: enrichXPathExprWithEnums(e.Inner, enumAttrs)}
	case *ast.FunctionCallExpr:
		enriched := make([]ast.Expression, len(e.Arguments))
		for i, arg := range e.Arguments {
			enriched[i] = enrichXPathExprWithEnums(arg, enumAttrs)
		}
		return &ast.FunctionCallExpr{Name: e.Name, Arguments: enriched}
	}
	return expr
}

// xpathBareAttrName returns the bare attribute name if expr is a simple
// IdentifierExpr (e.g. "Status"), otherwise "".
func xpathBareAttrName(expr ast.Expression) string {
	if id, ok := expr.(*ast.IdentifierExpr); ok {
		return id.Name
	}
	return ""
}

// enumStringToQN builds a QualifiedNameExpr for an enum value reference.
// enumRef = "Module.EnumName", valueKey = "Open" → Module: "Module", Name: "EnumName.Open"
//
// A value that is ALREADY the qualified name is returned as it stands. Mendix
// stores an enum comparison as the bare value, so a stored `'Module.Enum.Value'`
// is a constraint the author quoted by mistake (ako/CapTrackV3 FINDINGS §29) —
// prefixing it again rendered `Xp.Status.Xp.Status.Confirmed`, which a
// describe → exec round-trip would then write back into the model. Describing an
// invalid constraint should show what is stored, not compound it.
func enumStringToQN(enumRef, valueKey string) *ast.QualifiedNameExpr {
	if strings.HasPrefix(valueKey, enumRef+".") {
		valueKey = strings.TrimPrefix(valueKey, enumRef+".")
	}
	parts := strings.SplitN(enumRef, ".", 2)
	if len(parts) != 2 {
		return &ast.QualifiedNameExpr{QualifiedName: ast.QualifiedName{Module: enumRef, Name: valueKey}}
	}
	return &ast.QualifiedNameExpr{
		QualifiedName: ast.QualifiedName{Module: parts[0], Name: parts[1] + "." + valueKey},
	}
}

// xpathExprToMDLString serializes an XPath expression for MDL DESCRIBE output.
// Unlike expressionToXPath (which converts enum QualifiedNameExpr → 'Value' for BSON),
// this preserves enum QualifiedNameExpr as Module.EnumName.Value for readability.
func xpathExprToMDLString(expr ast.Expression) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.QualifiedNameExpr:
		// Output qualified names as-is (including 3-part enum references)
		return e.QualifiedName.String()
	case *ast.BinaryExpr:
		left := xpathExprToMDLString(e.Left)
		right := xpathExprToMDLString(e.Right)
		op := strings.ToLower(e.Operator)
		return left + " " + op + " " + right
	case *ast.UnaryExpr:
		operand := xpathExprToMDLString(e.Operand)
		op := strings.ToLower(e.Operator)
		if op == "not" {
			if p, ok := e.Operand.(*ast.ParenExpr); ok {
				return "not(" + xpathExprToMDLString(p.Inner) + ")"
			}
			return "not(" + operand + ")"
		}
		return op + " " + operand
	case *ast.ParenExpr:
		return "(" + xpathExprToMDLString(e.Inner) + ")"
	case *ast.FunctionCallExpr:
		var args []string
		for _, arg := range e.Arguments {
			args = append(args, xpathExprToMDLString(arg))
		}
		return mendixFunctionName(e.Name) + "(" + strings.Join(args, ", ") + ")"
	case *ast.XPathPathExpr:
		var parts []string
		for _, step := range e.Steps {
			s := xpathExprToMDLString(step.Expr)
			for _, pred := range step.Predicates {
				s += "[" + xpathExprToMDLString(pred) + "]"
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, "/")
	default:
		// For all other types (literals, variables, tokens, etc.) use the BSON serializer —
		// they don't need MDL-specific output.
		return expressionToXPath(expr)
	}
}

// xpathPathExprToString serializes an XPathPathExpr to an XPath path string.
func xpathPathExprToString(path *ast.XPathPathExpr) string {
	return xpathPathOf(path, false)
}

func xpathPathOf(path *ast.XPathPathExpr, keepNames bool) string {
	var parts []string
	for _, step := range path.Steps {
		s := xpathOf(step.Expr, keepNames)
		for _, pred := range step.Predicates {
			s += "[" + xpathOf(pred, keepNames) + "]"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "/")
}

// countMicroflowActivities counts the number of meaningful activities in a microflow.
// Excludes structural elements like StartEvent, EndEvent, and merge nodes.
func countMicroflowActivities(mf *microflows.Microflow) int {
	if mf.ObjectCollection == nil {
		return 0
	}

	count := 0
	for _, obj := range mf.ObjectCollection.Objects {
		switch obj.(type) {
		case *microflows.StartEvent, *microflows.EndEvent:
			// Don't count start/end events
		case *microflows.ExclusiveMerge:
			// Don't count merge nodes (they're structural)
		default:
			// Count all other activities (ActionActivity, ExclusiveSplit, LoopedActivity, etc.)
			count++
		}
	}
	return count
}

// calculateMicroflowComplexity calculates the McCabe cyclomatic complexity of a microflow.
// McCabe complexity = 1 + number of decision points (IF, LOOP, error handlers)
// A higher complexity indicates more paths through the code and higher testing burden.
// Typical thresholds: 1-10 (simple), 11-20 (moderate), 21-50 (complex), 50+ (untestable)
func calculateMicroflowComplexity(mf *microflows.Microflow) int {
	// Base complexity is 1 (the main path through the microflow)
	complexity := 1

	if mf.ObjectCollection == nil {
		return complexity
	}

	// Count decision points in the main flow
	complexity += countMicroflowDecisionPoints(mf.ObjectCollection.Objects)

	return complexity
}

// countMicroflowDecisionPoints counts decision points in a list of microflow objects.
// This recursively processes nested structures like LoopedActivity.
func countMicroflowDecisionPoints(objects []microflows.MicroflowObject) int {
	count := 0

	for _, obj := range objects {
		switch activity := obj.(type) {
		case *microflows.ExclusiveSplit:
			// Each IF/decision adds 1 to complexity
			count++

		case *microflows.InheritanceSplit:
			// Type check split adds 1 to complexity
			count++

		case *microflows.LoopedActivity:
			// Each loop adds 1 to complexity
			count++
			// Also count decision points inside the loop body
			if activity.ObjectCollection != nil {
				count += countMicroflowDecisionPoints(activity.ObjectCollection.Objects)
			}

		case *microflows.ErrorEvent:
			// Error handling path adds complexity
			count++
		}
	}

	return count
}
