// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// oqlComparisonFixes maps each OQL comparison the MDL expression grammar admits
// (it is shared with OQL) to the XPath spelling of the same test. Neither an
// XPath constraint nor a Mendix expression has any of them: measured on mxbuild
// 11.12.2, `[Name like 'A-%']` is CE0161 while `[starts-with(Name, 'A-')]`
// builds clean (mendixlabs/mxcli#1686). Until this rule the visitor dropped the
// operator and its operands, so the constraint was stored as `[Name]` — the
// same CE0161, with nothing in check to say why.
var oqlComparisonFixes = map[string]string{
	"like":        "Use starts-with(Name, 'A-'), ends-with(Name, '-A') or contains(Name, 'A') — `like` is OQL.",
	"not like":    "Negate the XPath function: not(starts-with(Name, 'A-')) or not(contains(Name, 'A')) — `like` is OQL.",
	"between":     "Compare both bounds: N >= 1 and N <= 5 — `between` is OQL.",
	"not between": "Compare outside the bounds: N < 1 or N > 5 — `between` is OQL.",
	"in":          "Compare each value: N = 1 or N = 2 — `in (…)` is OQL.",
	"match":       "Use contains() or starts-with() — `match` is OQL.",
	"is null":     "Compare with empty: Name = empty — `is null` is OQL.",
	"is not null": "Compare with empty: Name != empty — `is not null` is OQL.",
}

// oqlComparisonExpressionFixes is oqlComparisonFixes for a Mendix expression,
// whose string functions are spelled startsWith/endsWith/contains.
var oqlComparisonExpressionFixes = map[string]string{
	"like":        "Use startsWith($S, 'A-'), endsWith($S, '-A') or contains($S, 'A') — `like` is OQL.",
	"not like":    "Negate the function: not(startsWith($S, 'A-')) — `like` is OQL.",
	"between":     "Compare both bounds: $N >= 1 and $N <= 5 — `between` is OQL.",
	"not between": "Compare outside the bounds: $N < 1 or $N > 5 — `between` is OQL.",
	"in":          "Compare each value: $N = 1 or $N = 2 — `in (…)` is OQL.",
	"match":       "Use contains() or startsWith() — `match` is OQL.",
	"is null":     "Compare with empty: $S = empty — `is null` is OQL.",
	"is not null": "Compare with empty: $S != empty — `is not null` is OQL.",
}

// oqlOnlyComparisons returns the OQL comparison operators an expression uses,
// in the order they appear.
func oqlOnlyComparisons(expr ast.Expression) []string {
	var out []string
	var walk func(ast.Expression)
	walk = func(e ast.Expression) {
		switch n := e.(type) {
		case *ast.BinaryExpr:
			if _, ok := oqlComparisonFixes[strings.ToLower(n.Operator)]; ok {
				out = append(out, strings.ToLower(n.Operator))
			}
			walk(n.Left)
			walk(n.Right)
		case *ast.UnaryExpr:
			walk(n.Operand)
		case *ast.ParenExpr:
			walk(n.Inner)
		case *ast.SourceExpr:
			walk(n.Expression)
		case *ast.FunctionCallExpr:
			for _, a := range n.Arguments {
				walk(a)
			}
		case *ast.IfThenElseExpr:
			walk(n.Condition)
			walk(n.ThenExpr)
			walk(n.ElseExpr)
		case *ast.XPathPathExpr:
			for _, s := range n.Steps {
				walk(s.Expr)
				for _, p := range s.Predicates {
					walk(p)
				}
			}
		}
	}
	walk(expr)
	return out
}

// checkXPathOQLComparisons flags (MDL091) an OQL comparison in a retrieve
// constraint — MDL091 because it is the same mistake as `startsWith()` there:
// a construct from the language next door, CE0161 at build time.
func (v *microflowValidator) checkXPathOQLComparisons(variable string, where ast.Expression) {
	for _, op := range oqlOnlyComparisons(where) {
		v.addViolation("MDL091", linter.SeverityError,
			fmt.Sprintf("retrieve '$%s' constraint uses `%s`, which is OQL — XPath does not have it, and mxbuild "+
				"reports CE0161 \"Error(s) in XPath constraint\"", variable, op),
			oqlComparisonFixes[op])
	}
}

// checkExpressionOQLComparisons is the same rule in a Mendix expression, where
// the build reports CE0117 instead. The visitor dropped these operators there
// too: `declare $b Boolean = $s like 'A%'` was stored as `$s`.
func (v *microflowValidator) checkExpressionOQLComparisons(label string, expr ast.Expression) {
	for _, op := range oqlOnlyComparisons(expr) {
		v.addViolation("MDL091", linter.SeverityError,
			fmt.Sprintf("%s uses `%s`, which is OQL — a Mendix expression does not have it, and mxbuild "+
				"reports CE0117 \"Error(s) in expression\"", label, op),
			oqlComparisonExpressionFixes[op])
	}
}
