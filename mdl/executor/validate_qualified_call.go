// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A Mendix expression has no user-callable functions. Its function library is
// built-in and unqualified (`length`, `toString`, `contains`, …); microflows,
// rules and Java actions are called by ACTIVITIES, never from an expression.
//
// So `Module.Name(arg = $x)` in a value position is not a Mendix expression at
// all, and mxcli used to write it as literal expression text on BOTH engines.
// Measured on mxbuild 11.13.0 (upstream #939):
//
//	declare $b Boolean = Sample.Rule_IsActive(IsActive = $IsActive);
//	  → [CE0117] "Error(s) in expression." at Create variable activity
//	declare $n Integer = Sample.MF_Callee(N = $N);
//	  → [CE0117] — a microflow is no more callable there than a rule
//
// Both have a working spelling, which is what makes this worth a diagnostic
// rather than a shrug: a microflow or Java action is `$r = CALL MICROFLOW …`,
// and a rule belongs in a decision (`if Module.SomeRule(…) then`).
//
// The one legal home for a bare qualified call is that decision condition,
// which mxcli serializes as a Microflows$RuleSplitCondition — but only as the
// WHOLE condition. Inside a larger one (`$x != empty and Rule(…)`, `not
// Rule(…)`) it is expression text again, and mxbuild fails CE0117
// (ako/mxcli#2625): checkQualifiedCallInCondition. Whether the name in that position resolves to a
// real rule needs the project, and is checked by the flow builder
// (tryBuildRuleSplitCondition) where the backend is at hand.

// checkQualifiedCallInExpression flags a qualified call in a value position.
// label describes the site (e.g. "declare '$r'"), matching checkExprFunctions.
func (v *microflowValidator) checkQualifiedCallInExpression(label string, expr ast.Expression) {
	for _, name := range qualifiedCallNames(expr) {
		v.addViolation("MDL066", linter.SeverityError,
			fmt.Sprintf("%s calls '%s(...)', but a Mendix expression cannot call a microflow, "+
				"rule or Java action — the build fails CE0117 \"Error(s) in expression\"", label, name),
			fmt.Sprintf("Use an activity: $Result = CALL MICROFLOW %s (...); "+
				"a rule can only be called from a decision — if %s (...) then ... end if;", name, name))
	}
}

// qualifiedCallNames returns the names of every qualified function call in expr,
// in source order. A name is qualified when it contains a '.', which is what
// separates `Module.Name(...)` from the built-in library — no built-in Mendix
// expression function has a dot in its name.
func qualifiedCallNames(expr ast.Expression) []string {
	var out []string
	var walk func(ast.Expression)
	walk = func(e ast.Expression) {
		switch n := e.(type) {
		case *ast.FunctionCallExpr:
			if strings.Contains(n.Name, ".") {
				out = append(out, n.Name)
			}
			for _, arg := range n.Arguments {
				walk(arg)
			}
		case *ast.BinaryExpr:
			walk(n.Left)
			walk(n.Right)
		case *ast.UnaryExpr:
			walk(n.Operand)
		case *ast.ParenExpr:
			walk(n.Inner)
		case *ast.IfThenElseExpr:
			walk(n.Condition)
			walk(n.ThenExpr)
			walk(n.ElseExpr)
		case *ast.SourceExpr:
			walk(n.Expression)
		}
	}
	walk(expr)
	return out
}

// checkQualifiedCallInCondition flags a qualified call in an IF condition that
// is not the whole condition. A condition that is one call (parenthesized or
// not) is the rule-based decision; only its arguments are judged.
func (v *microflowValidator) checkQualifiedCallInCondition(cond ast.Expression) {
	if call := unwrapParenCall(cond); call != nil && strings.Contains(call.Name, ".") {
		for _, arg := range call.Arguments {
			v.checkQualifiedCallInExpression(fmt.Sprintf("argument of decision rule '%s'", call.Name), arg)
		}
		return
	}
	v.checkQualifiedCallInExpression("if condition", cond)
}
