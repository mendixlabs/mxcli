// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A rule call is a decision's condition only when it is the WHOLE condition:
// that is what becomes a Microflows$RuleSplitCondition. Combined with anything
// else — `$Item != empty and Rule(…)` — it is expression text, and mxbuild
// fails CE0117 "Error(s) in expression." (ako/mxcli#2625, upstream
// mendixlabs/mxcli#1967). check exempted the whole IF condition from MDL066.
func TestQualifiedCallInsideACondition_2625(t *testing.T) {
	count := func(cond ast.Expression) int {
		stmt := &ast.CreateMicroflowStmt{
			Name: ast.QualifiedName{Module: "Sample", Name: "MF"},
			Body: []ast.MicroflowStatement{&ast.IfStmt{Condition: cond}},
		}
		return len(mdl066(ValidateMicroflow(stmt)))
	}
	notEmpty := &ast.BinaryExpr{Left: &ast.VariableExpr{Name: "Item"}, Operator: "!=", Right: &ast.LiteralExpr{Kind: ast.LiteralEmpty}}

	combined := &ast.BinaryExpr{Left: notEmpty, Operator: "and", Right: qualifiedCall("Sample.Rule_IsActive")}
	if n := count(combined); n != 1 {
		t.Errorf("`$Item != empty and Rule(…)`: %d MDL066, want 1", n)
	}
	if n := count(&ast.UnaryExpr{Operator: "not", Operand: qualifiedCall("Sample.Rule_IsActive")}); n != 1 {
		t.Errorf("`not Rule(…)`: %d MDL066, want 1", n)
	}

	// Controls: the rule call as the whole condition, bare or parenthesized,
	// is the rule-based decision.
	if n := count(qualifiedCall("Sample.Rule_IsActive")); n != 0 {
		t.Errorf("`if Rule(…) then`: %d MDL066, want 0", n)
	}
	if n := count(&ast.ParenExpr{Inner: qualifiedCall("Sample.Rule_IsActive")}); n != 0 {
		t.Errorf("`if (Rule(…)) then`: %d MDL066, want 0", n)
	}
}
