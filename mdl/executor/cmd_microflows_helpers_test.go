// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestQualifiedNameToXPath_EnumValue(t *testing.T) {
	// 3-part names (Module.EnumName.Value) must be converted to string literals for database XPath
	expr := &ast.QualifiedNameExpr{
		QualifiedName: ast.QualifiedName{Module: "MyModule", Name: "ENUM_Status.Processing"},
	}
	got := qualifiedNameToXPath(expr)
	want := "'Processing'"
	if got != want {
		t.Errorf("qualifiedNameToXPath(%q) = %q, want %q", expr.QualifiedName.String(), got, want)
	}
}

func TestQualifiedNameToXPath_NonEnum(t *testing.T) {
	// 2-part names (Module.AssocName) should pass through as-is
	expr := &ast.QualifiedNameExpr{
		QualifiedName: ast.QualifiedName{Module: "MyModule", Name: "SomeAssoc"},
	}
	got := qualifiedNameToXPath(expr)
	want := "MyModule.SomeAssoc"
	if got != want {
		t.Errorf("qualifiedNameToXPath(%q) = %q, want %q", expr.QualifiedName.String(), got, want)
	}
}

func TestExpressionToXPath_EnumInComparison(t *testing.T) {
	// WHERE Status = Module.ENUM.Value — enum value must become a string literal for database XPath
	expr := &ast.BinaryExpr{
		Left:     &ast.IdentifierExpr{Name: "Status"},
		Operator: "=",
		Right: &ast.QualifiedNameExpr{
			QualifiedName: ast.QualifiedName{Module: "BST", Name: "ComplianceStatus.Rectified"},
		},
	}
	got := expressionToXPath(expr)
	want := "Status = 'Rectified'"
	if got != want {
		t.Errorf("expressionToXPath = %q, want %q", got, want)
	}
}

func TestExpressionToXPath_StringLiteralPreserved(t *testing.T) {
	// WHERE Status = 'Pending' should stay as Status = 'Pending'
	expr := &ast.BinaryExpr{
		Left:     &ast.IdentifierExpr{Name: "Status"},
		Operator: "=",
		Right:    &ast.LiteralExpr{Value: "Pending", Kind: ast.LiteralString},
	}
	got := expressionToXPath(expr)
	want := "Status = 'Pending'"
	if got != want {
		t.Errorf("expressionToXPath = %q, want %q", got, want)
	}
}

func TestExpressionToString_QualifiedNameUnchanged(t *testing.T) {
	// In expression context, qualified names should remain as-is (correct for enum refs)
	expr := &ast.QualifiedNameExpr{
		QualifiedName: ast.QualifiedName{Module: "MyModule", Name: "ENUM_Status.Processing"},
	}
	got := expressionToString(expr)
	want := "MyModule.ENUM_Status.Processing"
	if got != want {
		t.Errorf("expressionToString = %q, want %q", got, want)
	}
}

// TestMendixFunctionName_CanonicalSpelling ensures every built-in Mendix
// expression function is emitted with the runtime-accepted camelCase
// spelling regardless of how the AST stores the name. The visitor
// upper-cases list / aggregate operations for dispatch (e.g. FIND, COUNT);
// without normalisation the writer would emit `FIND(...)` which the Mendix
// expression runtime rejects with CE0117. This test pins the canonical
// spelling for the full documented built-in set.
//
// References:
//   - https://docs.mendix.com/refguide/expressions/
//   - https://docs.mendix.com/refguide/string-function-calls/
//   - https://docs.mendix.com/refguide/mathematical-function-calls/
//   - https://docs.mendix.com/refguide/add-date-function-calls/
//   - https://docs.mendix.com/refguide/between-date-function-calls/
//   - https://docs.mendix.com/refguide/parse-and-format-date-function-calls/
//   - https://docs.mendix.com/refguide/parse-and-format-decimal-function-calls/
func TestMendixFunctionName_CanonicalSpelling(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// List operations: visitor emits UPPERCASE, runtime requires lowercase
		{"HEAD", "head"}, {"TAIL", "tail"}, {"FIND", "find"},
		{"FILTER", "filter"}, {"SORT", "sort"}, {"UNION", "union"},
		{"INTERSECT", "intersect"}, {"SUBTRACT", "subtract"},
		{"CONTAINS", "contains"}, {"EQUALS", "equals"}, {"RANGE", "range"},
		// Aggregates: visitor maps AVG/MIN/MAX to AVERAGE/MINIMUM/MAXIMUM
		{"COUNT", "count"}, {"SUM", "sum"},
		{"AVERAGE", "average"}, {"MINIMUM", "minimum"}, {"MAXIMUM", "maximum"},
		// String functions
		{"LENGTH", "length"}, {"Length", "length"}, {"length", "length"},
		{"SUBSTRING", "substring"}, {"ToUpperCase", "toUpperCase"},
		{"TOLOWERCASE", "toLowerCase"}, {"TRIM", "trim"},
		{"FINDLAST", "findLast"}, {"REPLACEALL", "replaceAll"},
		{"STARTSWITH", "startsWith"}, {"ENDSWITH", "endsWith"},
		{"ISMATCH", "isMatch"}, {"URLENCODE", "urlEncode"},
		// Math
		{"ABS", "abs"}, {"CEIL", "ceil"}, {"FLOOR", "floor"},
		{"ROUND", "round"}, {"POW", "pow"}, {"SQRT", "sqrt"},
		// Parse / format
		{"PARSEINTEGER", "parseInteger"}, {"ParseInteger", "parseInteger"},
		{"PARSEDECIMAL", "parseDecimal"}, {"PARSELONG", "parseLong"},
		{"FORMATDECIMAL", "formatDecimal"}, {"FORMATDATETIME", "formatDateTime"},
		{"PARSEDATETIME", "parseDateTime"}, {"PARSEDATETIMEUTC", "parseDateTimeUTC"},
		{"TOSTRING", "toString"}, {"TOBOOLEAN", "toBoolean"},
		// Add / subtract / between date
		{"ADDDAYS", "addDays"}, {"ADDMONTHS", "addMonths"},
		{"SUBTRACTDAYS", "subtractDays"},
		{"DAYSBETWEEN", "daysBetween"}, {"SECONDSBETWEEN", "secondsBetween"},
		{"MILLISECONDSBETWEEN", "millisecondsBetween"},
		// Begin/end/trim-to date
		{"BEGINOFDAY", "beginOfDay"}, {"ENDOFMONTH", "endOfMonth"},
		{"TRIMTODAYS", "trimToDays"}, {"TRIMTOMINUTES", "trimToMinutes"},
		// Misc
		{"EMPTY", "empty"}, {"ISNEW", "isNew"},
		{"GETCAPTION", "getCaption"}, {"GETKEY", "getKey"},
		// Custom user-defined names pass through unchanged (no normalisation)
		{"MyJavaAction", "MyJavaAction"},
		{"MyModule_DoSomething", "MyModule_DoSomething"},
		{"SOMEUNKNOWNFUNC", "SOMEUNKNOWNFUNC"},
	}
	for _, tc := range cases {
		got := mendixFunctionName(tc.input)
		if got != tc.want {
			t.Errorf("mendixFunctionName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestExpressionToString_FunctionCallCaseFixed ensures that a FunctionCallExpr
// built from a list operation (visitor canonicalises FIND in uppercase) is
// serialised with the Mendix-accepted lowercase spelling, with arguments
// separated by ", " to match the pristine BSON serialisation format.
func TestExpressionToString_FunctionCallCaseFixed(t *testing.T) {
	expr := &ast.FunctionCallExpr{
		Name: "FIND",
		Arguments: []ast.Expression{
			&ast.VariableExpr{Name: "DisplayVersion"},
			&ast.LiteralExpr{Value: ".", Kind: ast.LiteralString},
		},
	}
	got := expressionToString(expr)
	want := "find($DisplayVersion, '.')"
	if got != want {
		t.Errorf("expressionToString = %q, want %q", got, want)
	}
}

// TestExpressionToString_NestedBuiltins exercises the common parseInteger(
// substring(..., find(..., '.'))) pattern found in pristine Mendix microflows
// (Apps.GetOrCreateMendixVersionFromString). Before the fix mxcli emitted
// `FIND` uppercase here, triggering CE0117 on roundtrip.
func TestExpressionToString_NestedBuiltins(t *testing.T) {
	// parseInteger(substring($DisplayVersion, 0, find($DisplayVersion, '.')))
	findCall := &ast.FunctionCallExpr{
		Name: "FIND",
		Arguments: []ast.Expression{
			&ast.VariableExpr{Name: "DisplayVersion"},
			&ast.LiteralExpr{Value: ".", Kind: ast.LiteralString},
		},
	}
	substringCall := &ast.FunctionCallExpr{
		Name: "substring",
		Arguments: []ast.Expression{
			&ast.VariableExpr{Name: "DisplayVersion"},
			&ast.LiteralExpr{Value: int64(0), Kind: ast.LiteralInteger},
			findCall,
		},
	}
	parseInt := &ast.FunctionCallExpr{
		Name:      "parseInteger",
		Arguments: []ast.Expression{substringCall},
	}
	got := expressionToString(parseInt)
	want := "parseInteger(substring($DisplayVersion, 0, find($DisplayVersion, '.')))"
	if got != want {
		t.Errorf("expressionToString = %q, want %q", got, want)
	}
}

// --- RT-1: not() with parenthesized operand should not have space ---

func TestExpressionToString_NotWithParens(t *testing.T) {
	// not($x) should remain not($x), not "not ($x)"
	expr := &ast.UnaryExpr{
		Operator: "not",
		Operand: &ast.ParenExpr{
			Inner: &ast.IdentifierExpr{Name: "$IsActive"},
		},
	}
	got := expressionToString(expr)
	want := "not($IsActive)"
	if got != want {
		t.Errorf("expressionToString = %q, want %q", got, want)
	}
}

func TestExpressionToString_NotWithoutParens(t *testing.T) {
	// AST-level: not(IdentifierExpr) must emit not($IsActive) — parens always added
	// (Mendix CE0117 rejects bare "not expr"; grammar now enforces parens at parse time)
	expr := &ast.UnaryExpr{
		Operator: "not",
		Operand:  &ast.IdentifierExpr{Name: "$IsActive"},
	}
	got := expressionToString(expr)
	want := "not($IsActive)"
	if got != want {
		t.Errorf("expressionToString = %q, want %q", got, want)
	}
}

// TestExpressionToString_NotWithFunctionCall ensures that "not contains(...)"
// is emitted as "not(contains(...))" — CE0117 rejects the form with a space.
func TestExpressionToString_NotWithFunctionCall(t *testing.T) {
	expr := &ast.UnaryExpr{
		Operator: "not",
		Operand: &ast.FunctionCallExpr{
			Name: "contains",
			Arguments: []ast.Expression{
				&ast.VariableExpr{Name: "Name"},
				&ast.LiteralExpr{Value: "demo", Kind: ast.LiteralString},
			},
		},
	}
	got := expressionToString(expr)
	want := "not(contains($Name, 'demo'))"
	if got != want {
		t.Errorf("expressionToString = %q, want %q", got, want)
	}
}

// TestNormalizeXPathEnumRefs ensures that 3-part qualified enum value references in a raw
// XPath string (from the bracketed SourceExpr path) are converted to string literals.
// This is needed because XPath constraints are database-level SQL WHERE clauses; the DB
// only knows string values, not enum qualified names.
func TestNormalizeXPathEnumRefs(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// Single enum value
		{"[Status = XpathTest.OrderStatus.Open]", "[Status = 'Open']"},
		// Negation
		{"[Status != XpathTest.OrderStatus.Cancelled]", "[Status != 'Cancelled']"},
		// OR with two enum values
		{"[Status = XpathTest.OrderStatus.Open or Status = XpathTest.OrderStatus.Processing]",
			"[Status = 'Open' or Status = 'Processing']"},
		// AND with non-enum part preserved
		{"[Status = XpathTest.OrderStatus.Completed and TotalAmount >= $MinAmount]",
			"[Status = 'Completed' and TotalAmount >= $MinAmount]"},
		// 2-part association name not affected
		{"[XpathTest.Order_Customer = $Customer]", "[XpathTest.Order_Customer = $Customer]"},
		// String literal already correct — not double-quoted
		{"[Status = 'Active']", "[Status = 'Active']"},
	}
	for _, tc := range cases {
		got := normalizeXPathEnumRefs(tc.input)
		if got != tc.want {
			t.Errorf("normalizeXPathEnumRefs(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
