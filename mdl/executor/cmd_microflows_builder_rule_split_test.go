// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// TestIfWithRuleCall_EmitsRuleSplitCondition is the regression test for the
// Rule vs Expression subtype preservation bug. Prior to the fix, an IF whose
// condition was a call into a rule (e.g. ControlCenterCommons.IsNotEmptyString)
// was serialized as ExpressionSplitCondition, causing Mendix Studio Pro to
// raise CE0117 "Error(s) in expression" and demoting the decision's subtype
// from Rule to Expression on every describe → exec roundtrip.
func TestIfWithRuleCall_EmitsRuleSplitCondition(t *testing.T) {
	mb := &mock.MockBackend{
		IsRuleFunc: func(qualifiedName string) (bool, error) {
			return qualifiedName == "Module.IsEligible", nil
		},
	}

	// if Module.IsEligible(Customer = $Customer) then return true else return false
	ifStmt := &ast.IfStmt{
		Condition: &ast.FunctionCallExpr{
			Name: "Module.IsEligible",
			Arguments: []ast.Expression{
				&ast.BinaryExpr{
					Left:     &ast.IdentifierExpr{Name: "Customer"},
					Operator: "=",
					Right:    &ast.VariableExpr{Name: "Customer"},
				},
			},
		},
		ThenBody: []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: &ast.LiteralExpr{Value: true, Kind: ast.LiteralBoolean}},
		},
		ElseBody: []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: &ast.LiteralExpr{Value: false, Kind: ast.LiteralBoolean}},
		},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		backend:      mb,
		varTypes:     map[string]string{"Customer": "Module.Customer"},
		declaredVars: map[string]string{"Customer": "Module.Customer"},
	}
	fb.buildFlowGraph([]ast.MicroflowStatement{ifStmt}, nil)

	var split *microflows.ExclusiveSplit
	for _, obj := range fb.objects {
		if sp, ok := obj.(*microflows.ExclusiveSplit); ok {
			split = sp
			break
		}
	}
	if split == nil {
		t.Fatal("expected an ExclusiveSplit, found none")
	}

	rule, ok := split.SplitCondition.(*microflows.RuleSplitCondition)
	if !ok {
		t.Fatalf("split condition: got %T, want *microflows.RuleSplitCondition", split.SplitCondition)
	}
	if rule.RuleQualifiedName != "Module.IsEligible" {
		t.Errorf("rule name: got %q, want %q", rule.RuleQualifiedName, "Module.IsEligible")
	}
	if len(rule.ParameterMappings) != 1 {
		t.Fatalf("parameter mappings: got %d, want 1", len(rule.ParameterMappings))
	}
	pm := rule.ParameterMappings[0]
	if pm.ParameterName != "Module.IsEligible.Customer" {
		t.Errorf("parameter name: got %q, want %q", pm.ParameterName, "Module.IsEligible.Customer")
	}
	if pm.Argument != "$Customer" {
		t.Errorf("argument: got %q, want %q", pm.Argument, "$Customer")
	}
}

// TestIfWithNonRuleCall_EmitsExpressionSplitCondition confirms that a plain
// expression-level function call (built-in or sub-microflow, not a rule) still
// produces an ExpressionSplitCondition — the fix must not over-trigger.
func TestIfWithNonRuleCall_EmitsExpressionSplitCondition(t *testing.T) {
	mb := &mock.MockBackend{
		IsRuleFunc: func(qualifiedName string) (bool, error) {
			return false, nil
		},
	}

	ifStmt := &ast.IfStmt{
		Condition: &ast.FunctionCallExpr{
			Name:      "empty",
			Arguments: []ast.Expression{&ast.VariableExpr{Name: "S"}},
		},
		ThenBody: []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: &ast.LiteralExpr{Value: true, Kind: ast.LiteralBoolean}},
		},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		backend:      mb,
		varTypes:     map[string]string{"S": "String"},
		declaredVars: map[string]string{"S": "String"},
	}
	fb.buildFlowGraph([]ast.MicroflowStatement{ifStmt}, nil)

	var split *microflows.ExclusiveSplit
	for _, obj := range fb.objects {
		if sp, ok := obj.(*microflows.ExclusiveSplit); ok {
			split = sp
			break
		}
	}
	if split == nil {
		t.Fatal("expected an ExclusiveSplit, found none")
	}
	if _, ok := split.SplitCondition.(*microflows.ExpressionSplitCondition); !ok {
		t.Fatalf("split condition: got %T, want *microflows.ExpressionSplitCondition", split.SplitCondition)
	}
}

// TestIfWithoutBackend_FallsBackToExpression confirms that when the flow
// builder has no backend (e.g. disconnected check mode), it can't tell whether
// a qualified call is a rule — it must default to ExpressionSplitCondition so
// that syntax-only checks don't crash.
func TestIfWithoutBackend_FallsBackToExpression(t *testing.T) {
	ifStmt := &ast.IfStmt{
		Condition: &ast.FunctionCallExpr{
			Name:      "Module.IsEligible",
			Arguments: []ast.Expression{},
		},
		ThenBody: []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: &ast.LiteralExpr{Value: true, Kind: ast.LiteralBoolean}},
		},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
	}
	fb.buildFlowGraph([]ast.MicroflowStatement{ifStmt}, nil)

	for _, obj := range fb.objects {
		if sp, ok := obj.(*microflows.ExclusiveSplit); ok {
			if _, ok := sp.SplitCondition.(*microflows.ExpressionSplitCondition); !ok {
				t.Fatalf("without backend, split condition: got %T, want *microflows.ExpressionSplitCondition", sp.SplitCondition)
			}
			return
		}
	}
	t.Fatal("expected an ExclusiveSplit, found none")
}
