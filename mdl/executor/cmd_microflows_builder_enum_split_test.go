// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestEnumSplitBuilderCreatesEnumerationCaseFlows(t *testing.T) {
	fb := &flowBuilder{
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
	}

	fb.addEnumSplit(&ast.EnumSplitStmt{
		Variable: "Status",
		Cases: []ast.EnumSplitCase{
			{
				Values: []string{"Open", "Pending"},
				Body: []ast.MicroflowStatement{
					&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "open"}},
				},
			},
		},
		ElseBody: []ast.MicroflowStatement{
			&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "other"}},
		},
	})

	var split *microflows.ExclusiveSplit
	for _, obj := range fb.objects {
		if candidate, ok := obj.(*microflows.ExclusiveSplit); ok {
			split = candidate
			break
		}
	}
	if split == nil {
		t.Fatal("Expected ExclusiveSplit")
	}
	cond, ok := split.SplitCondition.(*microflows.ExpressionSplitCondition)
	if !ok {
		t.Fatalf("SplitCondition = %T, want ExpressionSplitCondition", split.SplitCondition)
	}
	if cond.Expression != "$Status" {
		t.Fatalf("Expression = %q, want $Status", cond.Expression)
	}

	var cases []string
	for _, flow := range fb.flows {
		if flow.OriginID != split.ID {
			continue
		}
		if value, ok := enumCaseValue(flow); ok {
			cases = append(cases, value)
		}
	}
	if len(cases) != 2 || cases[0] != "Open" || cases[1] != "Pending" {
		t.Fatalf("enum case flows = %v, want [Open Pending]", cases)
	}
}

func TestEnumSplitNestedEmptyThenBranchKeepsContinuationCase(t *testing.T) {
	fb := &flowBuilder{
		spacing:      HorizontalSpacing,
		declaredVars: map[string]string{"MemberProvided": "Boolean"},
		measurer:     &layoutMeasurer{},
	}

	oc := fb.buildFlowGraph([]ast.MicroflowStatement{
		&ast.EnumSplitStmt{
			Variable: "SubjectType",
			Cases: []ast.EnumSplitCase{
				{
					Value: "member",
					Body: []ast.MicroflowStatement{
						&ast.IfStmt{
							Condition: &ast.VariableExpr{Name: "MemberProvided"},
							ElseBody:  []ast.MicroflowStatement{&ast.ReturnStmt{}},
						},
					},
				},
				{
					Value: "unknown",
					Body:  []ast.MicroflowStatement{&ast.ReturnStmt{}},
				},
			},
		},
		&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "shared tail"}},
	}, nil)

	objects := map[model.ID]microflows.MicroflowObject{}
	var nestedSplitID model.ID
	for _, obj := range oc.Objects {
		objects[obj.GetID()] = obj
		split, ok := obj.(*microflows.ExclusiveSplit)
		if !ok {
			continue
		}
		if condition, ok := split.SplitCondition.(*microflows.ExpressionSplitCondition); ok && condition.Expression == "$MemberProvided" {
			nestedSplitID = split.ID
		}
	}
	if nestedSplitID == "" {
		t.Fatal("expected nested decision split")
	}
	for _, flow := range oc.Flows {
		if flow.OriginID != nestedSplitID {
			continue
		}
		if flowCaseString(flow.CaseValue) == "true" {
			if _, ok := objects[flow.DestinationID].(*microflows.ExclusiveMerge); ok {
				return
			}
		}
	}
	t.Fatal("nested empty-then enum branch must carry CaseValue=true to the enum merge")
}

func TestEnumSplitAllBranchesReturnDoesNotCreateDanglingMerge(t *testing.T) {
	fb := &flowBuilder{
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
	}

	oc := fb.buildFlowGraph([]ast.MicroflowStatement{
		&ast.EnumSplitStmt{
			Variable: "Status",
			Cases: []ast.EnumSplitCase{
				{
					Value: "Open",
					Body:  []ast.MicroflowStatement{&ast.ReturnStmt{}},
				},
				{
					Value: "Closed",
					Body:  []ast.MicroflowStatement{&ast.ReturnStmt{}},
				},
			},
			ElseBody: []ast.MicroflowStatement{&ast.ReturnStmt{}},
		},
	}, nil)

	for _, obj := range oc.Objects {
		if _, ok := obj.(*microflows.ExclusiveMerge); ok {
			t.Fatalf("all-terminal enum split created dangling merge %#v", obj.GetID())
		}
	}
}

func TestEnumSplitAllCasesReturnWithoutElseDoesNotCreateFallthrough(t *testing.T) {
	fb := &flowBuilder{
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
	}

	oc := fb.buildFlowGraph([]ast.MicroflowStatement{
		&ast.IfStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			ThenBody: []ast.MicroflowStatement{
				&ast.EnumSplitStmt{
					Variable: "Status",
					Cases: []ast.EnumSplitCase{
						{Value: "Open", Body: []ast.MicroflowStatement{&ast.ReturnStmt{}}},
						{Value: "Closed", Body: []ast.MicroflowStatement{&ast.ReturnStmt{}}},
					},
				},
			},
		},
		&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "after"}},
		&ast.ReturnStmt{},
	}, nil)

	for _, flow := range oc.Flows {
		if flow.CaseValue == nil {
			continue
		}
		if _, ok := flow.CaseValue.(*microflows.EnumerationCase); ok && flow.DestinationID != "" {
			dest := objectByID(oc.Objects, flow.DestinationID)
			if logActivityHasMessage(dest, "after") {
				t.Fatalf("outer IF continuation was attached as enum default flow: %#v", flow.CaseValue)
			}
		}
	}
}

func TestEnumSplitBuilderRejectsMoreThanSupportedBranches(t *testing.T) {
	fb := &flowBuilder{
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
	}

	if id := fb.addEnumSplit(enumSplitWithBranchCount(maxEnumSplitBranches + 1)); id != "" {
		t.Fatalf("unsupported enum split returned split ID %q", id)
	}

	errors := strings.Join(fb.GetErrors(), "\n")
	if !strings.Contains(errors, "enum split has 17 branches; at most 16 branches are supported") {
		t.Fatalf("expected unsupported branch count error, got %q", errors)
	}
}

func objectByID(objects []microflows.MicroflowObject, id model.ID) microflows.MicroflowObject {
	for _, obj := range objects {
		if obj.GetID() == id {
			return obj
		}
	}
	return nil
}

func logActivityHasMessage(obj microflows.MicroflowObject, message string) bool {
	activity, ok := obj.(*microflows.ActionActivity)
	if !ok {
		return false
	}
	logAction, ok := activity.Action.(*microflows.LogMessageAction)
	if !ok || logAction.MessageTemplate == nil {
		return false
	}
	for _, text := range logAction.MessageTemplate.Translations {
		if text == message {
			return true
		}
	}
	return false
}

func enumSplitWithBranchCount(count int) *ast.EnumSplitStmt {
	cases := make([]ast.EnumSplitCase, 0, count)
	for i := 0; i < count; i++ {
		cases = append(cases, ast.EnumSplitCase{
			Value: fmt.Sprintf("Value%d", i+1),
			Body: []ast.MicroflowStatement{
				&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "branch"}},
			},
		})
	}
	return &ast.EnumSplitStmt{
		Variable: "SyntheticStatus",
		Cases:    cases,
	}
}

// TestEnumSplitBuilderPreservesFirstStatementAnchor guards against silent
// loss of @anchor(from:..., to:...) on the first statement inside an enum
// split case. Before the fix the enum split builder never read
// stmtOwnAnchor(stmt) for case bodies, so any round-tripped anchor dropped
// on re-exec — describe → exec → describe lost the FlowAnchor entirely.
func TestEnumSplitBuilderPreservesFirstStatementAnchor(t *testing.T) {
	fb := &flowBuilder{
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
	}

	// @anchor(to: bottom) on the first case statement — bottom is a
	// non-default destination anchor (AnchorSideBottom == 2) so we can
	// distinguish it from the layout default.
	anchor := &ast.FlowAnchors{
		From: ast.AnchorSideUnset,
		To:   ast.AnchorSideBottom,
	}
	fb.addEnumSplit(&ast.EnumSplitStmt{
		Variable: "Status",
		Cases: []ast.EnumSplitCase{
			{
				Values: []string{"Open"},
				Body: []ast.MicroflowStatement{
					&ast.LogStmt{
						Level:       ast.LogInfo,
						Message:     &ast.LiteralExpr{Kind: ast.LiteralString, Value: "open"},
						Annotations: &ast.ActivityAnnotations{Anchor: anchor},
					},
				},
			},
		},
	})

	var split *microflows.ExclusiveSplit
	for _, obj := range fb.objects {
		if s, ok := obj.(*microflows.ExclusiveSplit); ok {
			split = s
			break
		}
	}
	if split == nil {
		t.Fatal("expected ExclusiveSplit")
	}

	var firstCaseFlow *microflows.SequenceFlow
	for _, f := range fb.flows {
		if f.OriginID == split.ID {
			firstCaseFlow = f
		}
	}
	if firstCaseFlow == nil {
		t.Fatal("expected split→case flow")
	}
	if firstCaseFlow.DestinationConnectionIndex != int(ast.AnchorSideBottom) {
		t.Errorf("DestinationConnectionIndex = %d, want %d — @anchor(to: bottom) was dropped",
			firstCaseFlow.DestinationConnectionIndex, int(ast.AnchorSideBottom))
	}
}
