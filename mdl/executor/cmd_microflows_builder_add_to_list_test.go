// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestAddToListBuilderUsesExpressionValue(t *testing.T) {
	fb := &flowBuilder{}

	fb.addAddToListAction(&ast.AddToListStmt{
		Value: &ast.AttributePathExpr{
			Variable: "Order",
			Path:     []string{"Number"},
		},
		List: "Numbers",
	})

	action := lastChangeListAction(t, fb)
	if action.Value != "$Order/Number" {
		t.Fatalf("Value = %q, want $Order/Number", action.Value)
	}
}

func TestAddToListBuilderKeepsSimpleVariableFallback(t *testing.T) {
	fb := &flowBuilder{}

	fb.addAddToListAction(&ast.AddToListStmt{
		Item: "Order",
		List: "Orders",
	})

	action := lastChangeListAction(t, fb)
	if action.Value != "$Order" {
		t.Fatalf("Value = %q, want $Order", action.Value)
	}
}

func TestCollectObjectInputVariablesSeesAddExpressionValue(t *testing.T) {
	inputs := collectObjectInputVariables([]ast.MicroflowStatement{
		&ast.AddToListStmt{
			Value: &ast.FunctionCallExpr{
				Name: "head",
				Arguments: []ast.Expression{
					&ast.VariableExpr{Name: "SourceItems"},
				},
			},
			List: "Items",
		},
	})

	if !inputs["SourceItems"] {
		t.Fatalf("SourceItems was not collected from add expression: %#v", inputs)
	}
}

func TestErrorHandlerStatementVarRefsSeesAddExpressionValue(t *testing.T) {
	stmt := &ast.AddToListStmt{
		Value: &ast.FunctionCallExpr{
			Name: "head",
			Arguments: []ast.Expression{
				&ast.VariableExpr{Name: "SourceItems"},
			},
		},
		List: "Items",
	}

	refs := errorHandlerStatementVarRefs(stmt)

	seenSource := false
	seenList := false
	for _, r := range refs {
		if r == "SourceItems" {
			seenSource = true
		}
		if r == "Items" {
			seenList = true
		}
	}
	if !seenSource {
		t.Errorf("expected $SourceItems to be tracked from add expression: %v", refs)
	}
	if !seenList {
		t.Errorf("expected $Items (list) to be tracked: %v", refs)
	}
}

func lastChangeListAction(t *testing.T, fb *flowBuilder) *microflows.ChangeListAction {
	t.Helper()

	if len(fb.objects) == 0 {
		t.Fatal("Expected builder to create an action activity")
	}
	activity, ok := fb.objects[len(fb.objects)-1].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("Last object = %T, want ActionActivity", fb.objects[len(fb.objects)-1])
	}
	action, ok := activity.Action.(*microflows.ChangeListAction)
	if !ok {
		t.Fatalf("Action = %T, want ChangeListAction", activity.Action)
	}
	return action
}
