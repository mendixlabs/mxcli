// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestBuildFlowGraph_NoMergeIfElseContinuesFromBranchTail(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			Condition: &ast.VariableExpr{Name: "Done"},
			ThenBody: []ast.MicroflowStatement{
				&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true}},
			},
			ElseBody: []ast.MicroflowStatement{
				&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "else branch"}},
			},
		},
		&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "after branch"}},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		declaredVars: map[string]string{"Done": "Boolean"},
		measurer:     &layoutMeasurer{},
	}
	oc := fb.buildFlowGraph(body, &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeBoolean}})

	elseID := findLogActivityIDByMessage(t, oc, "else branch")
	afterID := findLogActivityIDByMessage(t, oc, "after branch")

	if !hasSequenceFlow(oc.Flows, elseID, afterID) {
		t.Fatal("continuing ELSE branch tail must connect to the following statement")
	}
	for _, flow := range oc.Flows {
		if flow.DestinationID == afterID && flow.OriginID != elseID {
			t.Fatalf("following statement must not be wired from stale split origin %q", flow.OriginID)
		}
	}
}

func TestBuildFlowGraph_NestedNoMergeTailCarriesAnchorToParentMerge(t *testing.T) {
	anchoredTail := &ast.LogStmt{
		Level:   ast.LogInfo,
		Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "nested else tail"},
		Annotations: &ast.ActivityAnnotations{
			Anchor: &ast.FlowAnchors{From: ast.AnchorSideBottom, To: ast.AnchorSideTop},
		},
	}
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			Condition: &ast.VariableExpr{Name: "Outer"},
			ThenBody: []ast.MicroflowStatement{
				&ast.IfStmt{
					Condition: &ast.VariableExpr{Name: "Inner"},
					ThenBody: []ast.MicroflowStatement{
						&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true}},
					},
					ElseBody: []ast.MicroflowStatement{anchoredTail},
				},
			},
			ElseBody: []ast.MicroflowStatement{
				&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "outer else"}},
			},
		},
		&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "after outer"}},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		declaredVars: map[string]string{"Outer": "Boolean", "Inner": "Boolean"},
		measurer:     &layoutMeasurer{},
	}
	oc := fb.buildFlowGraph(body, &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeBoolean}})

	tailID := findLogActivityIDByMessage(t, oc, "nested else tail")
	for _, flow := range oc.Flows {
		if flow.OriginID != tailID {
			continue
		}
		if _, ok := noMergeObjectByID(oc, flow.DestinationID).(*microflows.ExclusiveMerge); !ok {
			continue
		}
		if flow.OriginConnectionIndex != AnchorBottom || flow.DestinationConnectionIndex != AnchorTop {
			t.Fatalf("nested no-merge tail anchor = from %d to %d, want bottom/top", flow.OriginConnectionIndex, flow.DestinationConnectionIndex)
		}
		return
	}
	t.Fatal("expected nested no-merge tail to connect to the parent merge")
}

func findLogActivityIDByMessage(t *testing.T, oc *microflows.MicroflowObjectCollection, message string) model.ID {
	t.Helper()
	for _, obj := range oc.Objects {
		activity, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		action, ok := activity.Action.(*microflows.LogMessageAction)
		if !ok || action.MessageTemplate == nil {
			continue
		}
		if action.MessageTemplate.GetTranslation("en_US") == message {
			return activity.ID
		}
	}
	t.Fatalf("missing log activity %q", message)
	return ""
}

func noMergeObjectByID(oc *microflows.MicroflowObjectCollection, id model.ID) microflows.MicroflowObject {
	for _, obj := range oc.Objects {
		if obj.GetID() == id {
			return obj
		}
	}
	return nil
}

func hasSequenceFlow(flows []*microflows.SequenceFlow, originID, destinationID model.ID) bool {
	for _, flow := range flows {
		if flow.OriginID == originID && flow.DestinationID == destinationID {
			return true
		}
	}
	return false
}
