// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow helper functions
package executor

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
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

// mendixBuiltinFunctions is the canonical spelling of every built-in Mendix
// expression function. The expression runtime is case-sensitive: it only
// recognises these names as spelt here (lower-case with camelCase for
// compound words). Emitting an alternative spelling causes CE0117
// ("Error(s) in expression.") on Studio Pro validation.
//
// Source: https://docs.mendix.com/refguide/expressions/ and the linked
// function-specific pages (string, math, date arithmetic, parse/format,
// trim-to-date, list operations, aggregates, type conversions).
//
// The map key is the upper-case spelling for case-insensitive lookup; the
// value is the runtime-accepted canonical spelling. Custom user-defined
// java actions, sub-microflows, and unknown function names pass through
// unchanged so user case is preserved.
var mendixBuiltinFunctions = func() map[string]string {
	canonical := []string{
		// List operations
		"head", "tail", "find", "filter", "sort", "union",
		"intersect", "subtract", "contains", "equals", "range",
		// List aggregates
		"count", "sum", "average", "minimum", "maximum",
		"allTrue", "anyTrue",
		// String functions (docs.mendix.com/refguide/string-function-calls)
		"toUpperCase", "toLowerCase", "trim", "length", "substring",
		"findLast", "replaceAll", "replaceFirst", "startsWith", "endsWith",
		"isMatch", "isInvariantMatch", "stringFromRegex", "stringListFromRegex",
		"urlEncode", "urlDecode", "reverse", "indexOf",
		// Math functions (docs.mendix.com/refguide/mathematical-function-calls)
		"abs", "ceil", "floor", "round", "max", "min", "pow",
		"sqrt", "ln", "log10", "random", "rand",
		// Date creation (docs.mendix.com/refguide/date-creation)
		"dateTime", "dateTimeUTC",
		// Begin-of-date / end-of-date / trim-to-date
		"trimToDays", "trimToHours", "trimToMinutes", "trimToSeconds",
		"trimToDaysUTC", "trimToHoursUTC", "trimToMinutesUTC", "trimToSecondsUTC",
		"beginOfDay", "beginOfWeek", "beginOfMonth", "beginOfYear",
		"beginOfDayUTC", "beginOfWeekUTC", "beginOfMonthUTC", "beginOfYearUTC",
		"endOfDay", "endOfWeek", "endOfMonth", "endOfYear",
		"endOfDayUTC", "endOfWeekUTC", "endOfMonthUTC", "endOfYearUTC",
		// Between-date functions
		"millisecondsBetween", "secondsBetween", "minutesBetween",
		"hoursBetween", "daysBetween", "weeksBetween", "monthsBetween",
		"yearsBetween", "calendarDaysBetween", "calendarMonthsBetween",
		"calendarYearsBetween",
		// Add-date functions
		"addMilliseconds", "addSeconds", "addMinutes", "addHours",
		"addDays", "addWeeks", "addMonths", "addYears",
		"addDaysUTC", "addWeeksUTC", "addMonthsUTC", "addYearsUTC",
		// Subtract-date functions
		"subtractMilliseconds", "subtractSeconds", "subtractMinutes",
		"subtractHours", "subtractDays", "subtractWeeks", "subtractMonths",
		"subtractYears", "subtractDaysUTC", "subtractWeeksUTC",
		"subtractMonthsUTC", "subtractYearsUTC",
		// Day-of / timestamp conversion helpers
		"dayOfWeek", "dayOfWeekFromDateTime", "weekOfYearFromDateTime",
		"dayOfYearFromDateTime", "daysInMonth", "daysInYear",
		"dateTimeToEpoch", "epochToDateTime",
		// Parse / format (parse-and-format-date, parse-and-format-decimal)
		"formatDateTime", "formatDateTimeUTC", "parseDateTime", "parseDateTimeUTC",
		"parseInteger", "parseLong", "parseDecimal", "formatDecimal",
		// To-string / length  (to-string, length refguide pages)
		"toString", "toBoolean", "toFloat",
		// Enumeration helpers
		"getCaption", "getKey",
		// Miscellaneous
		"if", "empty", "isNew", "isAnonymous",
		// Boolean operators expressed as functions (true(), false())
		"true", "false",
		// Not / and / or appear as operators, not function calls — omitted.
	}
	m := make(map[string]string, len(canonical))
	for _, c := range canonical {
		m[strings.ToUpper(c)] = c
	}
	return m
}()

// mendixFunctionName normalises the case of built-in Mendix expression
// functions. The visitor canonicalises list / aggregate operations in
// UPPERCASE for AST dispatch; the expression runtime only recognises the
// documented camelCase spelling. For every built-in Mendix function we
// always emit the canonical spelling so that:
//
//   - round-tripping a pristine microflow never mutates `find(...)` into
//     `FIND(...)` (which Studio Pro rejects with CE0117).
//   - LLM-generated MDL with accidental capitalisation (`LENGTH(...)`,
//     `ToString(...)`) still validates when executed.
//
// Custom (user-defined) java actions, sub-microflows and entity member
// references pass through unchanged so user case is preserved.
func mendixFunctionName(name string) string {
	if canonical, ok := mendixBuiltinFunctions[strings.ToUpper(name)]; ok {
		return canonical
	}
	return name
}

// quoteExpressionLiteral wraps a Mendix expression string literal in single
// quotes and applies the narrowest possible escaping needed for MDL roundtrip:
//
//   - ASCII control characters that STRING_LITERAL does not accept raw — 0x0A
//     (newline), 0x0D (carriage return), 0x09 (tab) — are written as `\n`, `\r`,
//     `\t`. The MDL lexer rejects raw newlines inside single-quoted literals,
//     so emitting them verbatim produces parse errors on re-execute.
//   - Apostrophes are doubled (MDL's own delimiter-escape convention).
//   - Backslashes followed by one of the recognised escape letters (n/r/t/\/')
//     are doubled so the visitor's unquoteString preserves them — without this,
//     the source literal `\\n` would come back as a real newline on reparse.
//   - For any other backslash-prefixed byte (e.g. `\d`, `\w`, `\p{...}` inside
//     regexes) the backslash is emitted as-is and the follower is written by the
//     next loop iteration via the default arm, so the two bytes end up in the
//     output unchanged. This keeps Mendix regular-expression escape sequences
//     bit-exact across describe→exec roundtrips; the output is byte-identical
//     to passthrough even though the implementation walks the bytes separately.
//
// This is narrower than mdlQuote (used for @annotation / @caption text where
// the AST value is a plain string): mdlQuote unconditionally doubles every
// backslash, which would break expression literals containing regex escape
// sequences that the Mendix engine consumes literally.
func quoteExpressionLiteral(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\'':
			b.WriteString(`''`)
		case '\\':
			// Double the backslash only when the next byte would otherwise be
			// interpreted as an escape by unquoteString — that is, n/r/t/\/'.
			// For any other follower (letters like d/w, punctuation) the
			// backslash can pass through verbatim so regex escape characters
			// roundtrip without mutation.
			if i+1 < len(s) {
				switch s[i+1] {
				case 'n', 'r', 't':
					b.WriteString(`\\`)
					b.WriteByte(s[i+1])
					i++
					continue
				case '\\':
					// Literal backslash-backslash in AST. To survive roundtrip
					// it must be written as four backslashes: unquoteString
					// decodes `\\` twice, producing two backslashes again.
					b.WriteString(`\\\\`)
					i++
					continue
				case '\'':
					// Literal backslash-apostrophe: double the backslash and
					// double the apostrophe, so the reparsed value stays
					// [\, '].
					b.WriteString(`\\`)
					b.WriteString(`''`)
					i++
					continue
				}
				b.WriteByte('\\')
				continue
			}
			// Trailing backslash at end-of-string: the lexer's `'\\' .` escape
			// rule requires a following character, so emitting a bare `\'`
			// terminator would be reinterpreted as an escape pair and never
			// close the literal. Double the backslash — unquoteString decodes
			// `\\` back to a single backslash.
			b.WriteString(`\\`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// expressionToString converts an AST Expression to a Mendix expression string.
// Note: string literals are quoted via mdlQuote, which escapes backslashes,
// newlines, tabs, and carriage returns for MDL round-trip safety. Mendix's
// expression engine does not treat `\n` etc. as escapes, so a string literal
// with an embedded raw newline round-trips as `\n` in the MDL source (parseable)
// but is re-serialised into BSON as a two-character `\n` sequence rather than a
// real newline. This is the correct trade-off for describe→re-execute flows;
// the alternative (emitting raw control chars in MDL) would break the parser.
func expressionToString(expr ast.Expression) string {
	// Check for nil interface
	if expr == nil {
		return ""
	}

	// Use reflection to check for nil pointer inside interface
	// This handles the Go interface gotcha where the type is set but pointer is nil
	if reflect.ValueOf(expr).IsNil() {
		return ""
	}

	switch e := expr.(type) {
	case *ast.LiteralExpr:
		switch e.Kind {
		case ast.LiteralString:
			return quoteExpressionLiteral(fmt.Sprintf("%v", e.Value))
		case ast.LiteralBoolean:
			if e.Value.(bool) {
				return "true"
			}
			return "false"
		case ast.LiteralNull:
			return "empty"
		default:
			return fmt.Sprintf("%v", e.Value)
		}
	case *ast.VariableExpr:
		return "$" + e.Name
	case *ast.AttributePathExpr:
		return "$" + e.Variable + "/" + strings.Join(e.Path, "/")
	case *ast.BinaryExpr:
		left := expressionToString(e.Left)
		right := expressionToString(e.Right)
		// Mendix expressions use lowercase operators (and, or, div, mod)
		op := strings.ToLower(e.Operator)
		return left + " " + op + " " + right
	case *ast.UnaryExpr:
		// Mendix expressions use lowercase operators (not)
		op := strings.ToLower(e.Operator)
		// not(expr) — always emit parens; Mendix CE0117 rejects bare "not expr"
		if op == "not" {
			inner := e.Operand
			if paren, ok := inner.(*ast.ParenExpr); ok {
				// Unwrap double-parens: not((expr)) → not(expr)
				return "not(" + expressionToString(paren.Inner) + ")"
			}
			return "not(" + expressionToString(inner) + ")"
		}
		operand := expressionToString(e.Operand)
		return op + " " + operand
	case *ast.FunctionCallExpr:
		var args []string
		for _, arg := range e.Arguments {
			args = append(args, expressionToString(arg))
		}
		return mendixFunctionName(e.Name) + "(" + strings.Join(args, ", ") + ")"
	case *ast.TokenExpr:
		return "[%" + e.Token + "%]"
	case *ast.ParenExpr:
		return "(" + expressionToString(e.Inner) + ")"
	case *ast.IdentifierExpr:
		// Unquoted identifier (attribute name in XPath)
		return e.Name
	case *ast.QualifiedNameExpr:
		// Qualified name (association name, entity reference) - unquoted
		return e.QualifiedName.String()
	case *ast.ConstantRefExpr:
		return "@" + e.QualifiedName.String()
	case *ast.IfThenElseExpr:
		cond := expressionToString(e.Condition)
		thenStr := expressionToString(e.ThenExpr)
		elseStr := expressionToString(e.ElseExpr)
		return "if " + cond + " then " + thenStr + " else " + elseStr
	case *ast.SourceExpr:
		if e.Source != "" {
			return e.Source
		}
		return expressionToString(e.Expression)
	default:
		return ""
	}
}

// expressionToXPath converts an AST Expression to an XPath constraint string.
// Unlike expressionToString (for Mendix expressions), XPath requires Mendix
// tokens like [%CurrentDateTime%] to be quoted: '[%CurrentDateTime%]'.
func expressionToXPath(expr ast.Expression) string {
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
		left := expressionToXPath(e.Left)
		right := expressionToXPath(e.Right)
		op := strings.ToLower(e.Operator)
		return left + " " + op + " " + right
	case *ast.UnaryExpr:
		operand := expressionToXPath(e.Operand)
		op := strings.ToLower(e.Operator)
		// For 'not' with parenthesized operand, output as not(expr)
		if op == "not" {
			if p, ok := e.Operand.(*ast.ParenExpr); ok {
				return "not(" + expressionToXPath(p.Inner) + ")"
			}
			return "not(" + operand + ")"
		}
		return op + " " + operand
	case *ast.ParenExpr:
		return "(" + expressionToXPath(e.Inner) + ")"
	case *ast.XPathPathExpr:
		return xpathPathExprToString(e)
	case *ast.FunctionCallExpr:
		var args []string
		for _, arg := range e.Arguments {
			args = append(args, expressionToXPath(arg))
		}
		return mendixFunctionName(e.Name) + "(" + strings.Join(args, ", ") + ")"
	case *ast.LiteralExpr:
		if e.Kind == ast.LiteralEmpty {
			return "empty"
		}
		return expressionToString(expr)
	case *ast.QualifiedNameExpr:
		return qualifiedNameToXPath(e)
	case *ast.SourceExpr:
		if e.Source != "" {
			return e.Source
		}
		return expressionToXPath(e.Expression)
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
var xpathEnumRefRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]*\.[A-Za-z][A-Za-z0-9_]*\.[A-Za-z][A-Za-z0-9_]*`)

// normalizeXPathEnumRefs converts 3-part qualified enum value references in a raw XPath
// string to the string literal format that Mendix database queries require.
// Example: "[Status = XpathTest.OrderStatus.Open]" → "[Status = 'Open']".
// This handles the SourceExpr (bracketed) path where qualifiedNameToXPath is bypassed.
func normalizeXPathEnumRefs(xpath string) string {
	return xpathEnumRefRe.ReplaceAllStringFunc(xpath, func(match string) string {
		lastDot := strings.LastIndex(match, ".")
		return "'" + match[lastDot+1:] + "'"
	})
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
func enumStringToQN(enumRef, valueKey string) *ast.QualifiedNameExpr {
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
			if step.Predicate != nil {
				s += "[" + xpathExprToMDLString(step.Predicate) + "]"
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
	var parts []string
	for _, step := range path.Steps {
		s := expressionToXPath(step.Expr)
		if step.Predicate != nil {
			s += "[" + expressionToXPath(step.Predicate) + "]"
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
