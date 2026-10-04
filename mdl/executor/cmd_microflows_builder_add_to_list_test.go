// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
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

func TestAddToAssociationTargetUsesChangeObjectWithAddMember(t *testing.T) {
	fb := &flowBuilder{}

	fb.addAddToListAction(&ast.AddToListStmt{
		Value: &ast.VariableExpr{Name: "Line"},
		List:  "Order/MyModule.Sales.Order_Line",
	})

	// An association target cannot be a ChangeListAction: `changeVariableName`
	// is a variable name, and mx rejects a path there with
	//   CE0109 "Undefined variable 'Order/MyModule.Sales.Order_Line'"
	// The platform models this as a Change-object action carrying an
	// association MemberChange of Type Add — the same shape Studio Pro writes
	// for the Add/Remove buttons on a many-to-many member.
	activity, ok := fb.objects[len(fb.objects)-1].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("Last object = %T, want ActionActivity", fb.objects[len(fb.objects)-1])
	}
	action, ok := activity.Action.(*microflows.ChangeObjectAction)
	if !ok {
		t.Fatalf("Action = %T, want ChangeObjectAction for an association target", activity.Action)
	}
	if action.ChangeVariable != "Order" {
		t.Fatalf("ChangeVariable = %q, want Order (the object, not the path)", action.ChangeVariable)
	}
	if len(action.Changes) != 1 {
		t.Fatalf("Changes = %d, want 1", len(action.Changes))
	}
	mc := action.Changes[0]
	if mc.Type != microflows.MemberChangeTypeAdd {
		t.Errorf("MemberChange.Type = %q, want Add", mc.Type)
	}
	if got := mc.AssociationQualifiedName; got != "MyModule.Sales.Order_Line" {
		t.Errorf("Association = %q, want MyModule.Sales.Order_Line", got)
	}
	if mc.Value != "$Line" {
		t.Errorf("Value = %q, want $Line", mc.Value)
	}
}

func TestRemoveFromAssociationTargetUsesChangeObjectWithRemoveMember(t *testing.T) {
	fb := &flowBuilder{}

	fb.addRemoveFromListAction(&ast.RemoveFromListStmt{
		Item: "Line",
		List: "Order/MyModule.Sales.Order_Line",
	})

	activity := fb.objects[len(fb.objects)-1].(*microflows.ActionActivity)
	action, ok := activity.Action.(*microflows.ChangeObjectAction)
	if !ok {
		t.Fatalf("Action = %T, want ChangeObjectAction for an association target", activity.Action)
	}
	if action.ChangeVariable != "Order" {
		t.Fatalf("ChangeVariable = %q, want Order", action.ChangeVariable)
	}
	if len(action.Changes) != 1 {
		t.Fatalf("Changes = %d, want 1", len(action.Changes))
	}
	if action.Changes[0].Type != microflows.MemberChangeTypeRemove {
		t.Errorf("MemberChange.Type = %q, want Remove", action.Changes[0].Type)
	}
	if got := action.Changes[0].AssociationQualifiedName; got != "MyModule.Sales.Order_Line" {
		t.Errorf("Association = %q, want MyModule.Sales.Order_Line", got)
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
