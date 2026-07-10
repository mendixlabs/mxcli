// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestAddValidationFeedbackAction_PreservesQualifiedAssociationTarget(t *testing.T) {
	fb := &flowBuilder{
		posX:     100,
		posY:     100,
		spacing:  HorizontalSpacing,
		varTypes: map[string]string{"FormObject": "Synthetic.FormObject"},
	}

	fb.addValidationFeedbackAction(&ast.ValidationFeedbackStmt{
		AttributePath: &ast.AttributePathExpr{
			Variable: "FormObject",
			Segments: []ast.PathSegment{
				{Name: "Synthetic.FormObject_Target", Separator: "/"},
			},
		},
		Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "Select a target"},
	})

	action := validationFeedbackActionFromBuilder(t, fb)
	if action.AssociationName != "Synthetic.FormObject_Target" {
		t.Fatalf("AssociationName = %q, want Synthetic.FormObject_Target", action.AssociationName)
	}
	if action.AttributeName != "" {
		t.Fatalf("AttributeName = %q, want empty", action.AttributeName)
	}
}

func TestAddValidationFeedbackAction_PreservesQualifiedAttributeTarget(t *testing.T) {
	fb := &flowBuilder{
		posX:     100,
		posY:     100,
		spacing:  HorizontalSpacing,
		varTypes: map[string]string{"FormObject": "Synthetic.FormObject"},
	}

	fb.addValidationFeedbackAction(&ast.ValidationFeedbackStmt{
		AttributePath: &ast.AttributePathExpr{
			Variable: "FormObject",
			Segments: []ast.PathSegment{
				{Name: "Synthetic.FormObject.Name", Separator: "/"},
			},
		},
		Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "Enter a name"},
	})

	action := validationFeedbackActionFromBuilder(t, fb)
	if action.AttributeName != "Synthetic.FormObject.Name" {
		t.Fatalf("AttributeName = %q, want Synthetic.FormObject.Name", action.AttributeName)
	}
	if action.AssociationName != "" {
		t.Fatalf("AssociationName = %q, want empty", action.AssociationName)
	}
}

func validationFeedbackActionFromBuilder(t *testing.T, fb *flowBuilder) *microflows.ValidationFeedbackAction {
	t.Helper()
	if len(fb.objects) != 1 {
		t.Fatalf("objects = %d, want 1", len(fb.objects))
	}
	activity, ok := fb.objects[0].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("object is %T, want *microflows.ActionActivity", fb.objects[0])
	}
	action, ok := activity.Action.(*microflows.ValidationFeedbackAction)
	if !ok {
		t.Fatalf("action is %T, want *microflows.ValidationFeedbackAction", activity.Action)
	}
	return action
}
