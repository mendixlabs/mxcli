// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ifOperandRule is an inline `if … then … else …` used as the operand of an
// operator without parentheses. Mendix's expression grammar admits an
// if-expression only where a whole expression goes; MDL's parses it anywhere.
// Measured on mxbuild 11.12.2 (mendixlabs/mxcli#1710):
//
//	'x' + if $c then 'a' else 'b'          CE0117 "Error(s) in expression"
//	$c and if $c then true else false      CE0117
//	$c = if $c then true else false        CE0117
//	-if $c then 1 else 2                   CE0117
//	return 'x' + if $c then 'a' else 'b'   CE0117
//	'x' + (if $c then 'a' else 'b')        clean — parenthesised
//	if $c then 'a' else 'b' + 'x'          clean — the else branch takes the `+`
//	toUpperCase(if $c then 'a' else 'b')   clean — a function argument
//	not(if $c then true else false)        clean
//	if $c then 'a' else if $c then …       clean — an if as the else branch
const ifOperandRule = "MDL-IFOPERAND01"

// ifOperands returns the operators that take an unparenthesised if-expression
// as an operand, in the order they appear.
func ifOperands(expr ast.Expression) []string {
	var out []string
	var walk func(ast.Expression)
	walk = func(e ast.Expression) {
		switch n := e.(type) {
		case *ast.BinaryExpr:
			if isBareIf(n.Left) || isBareIf(n.Right) {
				out = append(out, strings.ToLower(n.Operator))
			}
			walk(n.Left)
			walk(n.Right)
		case *ast.UnaryExpr:
			// not(…) is always written with its parentheses.
			if !strings.EqualFold(n.Operator, "not") && isBareIf(n.Operand) {
				out = append(out, "unary "+n.Operator)
			}
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
		}
	}
	walk(expr)
	return out
}

// isBareIf reports an if-expression that is not inside parentheses. A
// SourceExpr is only a wrapper, unless its source carries the parentheses.
func isBareIf(e ast.Expression) bool {
	for {
		switch n := e.(type) {
		case *ast.SourceExpr:
			if strings.HasPrefix(strings.TrimSpace(n.Source), "(") {
				return false
			}
			e = n.Expression
		case *ast.IfThenElseExpr:
			return true
		default:
			return false
		}
	}
}

// checkIfOperand flags an if-expression used as an operand without
// parentheses (mendixlabs/mxcli#1710).
func (v *microflowValidator) checkIfOperand(label string, expr ast.Expression) {
	for _, op := range ifOperands(expr) {
		v.addViolation(ifOperandRule, linter.SeverityError,
			fmt.Sprintf("%s uses an `if … then … else …` as an operand of `%s` without parentheses — Mendix "+
				"rejects it (CE0117 \"Error(s) in expression\")", label, op),
			"Put the if-expression in parentheses: `'x' + (if $c then 'a' else 'b')`.")
	}
}
