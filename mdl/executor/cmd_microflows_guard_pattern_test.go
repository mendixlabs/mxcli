// SPDX-License-Identifier: Apache-2.0

// Regression test for the guard-pattern IF anchor leak. When an IF without
// ELSE has `thenReturns=true`, addIfStatement returns the splitID and sets
// nextFlowCase="false" so the OUTER loop in buildFlowGraph creates the
// splitID→nextActivity flow one iteration later. That flow needs the
// falseBranchAnchor from the IF's @anchor annotation — which addIfStatement
// now passes through fb.nextFlowAnchor.
package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestBuilder_GuardPatternPreservesFalseBranchAnchor(t *testing.T) {
	// Pattern from AcademyIntegration.GetOrCreateCertificate:
	//   retrieve; if cond then return X end if; create; return X
	//
	// The IF has no else, the then body returns — so the flow that runs
	// when the condition is FALSE connects split → create. That flow must
	// carry @anchor(from: bottom, to: top) (the continuation path drops
	// vertically to the next activity beneath the split).
	body := []ast.MicroflowStatement{
		&ast.LogStmt{
			Level:   ast.LogInfo,
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "start"},
		},
		&ast.IfStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			ThenBody: []ast.MicroflowStatement{
				&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "yes"}},
			},
			Annotations: &ast.ActivityAnnotations{
				FalseBranchAnchor: &ast.FlowAnchors{From: ast.AnchorSideBottom, To: ast.AnchorSideTop},
			},
		},
		&ast.LogStmt{
			Level:   ast.LogInfo,
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "tail"},
		},
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing}
	oc := fb.buildFlowGraph(body, nil)

	// Find the flow from the split to the tail log. It's the only one with
	// a false branch case that doesn't target an EndEvent.
	var found *microflows.SequenceFlow
	for _, f := range oc.Flows {
		if flowCaseString(f.CaseValue) != "false" {
			continue
		}
		// Exclude flows pointing at an EndEvent.
		isEnd := false
		for _, obj := range oc.Objects {
			if obj.GetID() == f.DestinationID {
				if _, e := obj.(*microflows.EndEvent); e {
					isEnd = true
				}
				break
			}
		}
		if !isEnd {
			found = f
			break
		}
	}
	if found == nil {
		t.Fatal("expected a split→tail flow with false case, got none")
	}
	if found.OriginConnectionIndex != AnchorBottom {
		t.Errorf("origin: got %d, want %d (Bottom)", found.OriginConnectionIndex, AnchorBottom)
	}
	if found.DestinationConnectionIndex != AnchorTop {
		t.Errorf("destination: got %d, want %d (Top)", found.DestinationConnectionIndex, AnchorTop)
	}
}

func TestCaseValueForFlowUsesExpressionCaseForBooleanBranches(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  string
	}{
		{value: "true", want: "true"},
		{value: "false", want: "false"},
	} {
		got, ok := caseValueForFlow(tc.value).(*microflows.ExpressionCase)
		if !ok {
			t.Fatalf("caseValueForFlow(%q) = %T, want *ExpressionCase", tc.value, caseValueForFlow(tc.value))
		}
		if got.Expression != tc.want {
			t.Fatalf("caseValueForFlow(%q).Expression = %q, want %q", tc.value, got.Expression, tc.want)
		}
	}
}

func TestCaseValueForFlowKeepsEnumValuesAsEnumerationCase(t *testing.T) {
	got, ok := caseValueForFlow("Submitted").(microflows.EnumerationCase)
	if !ok {
		t.Fatalf("caseValueForFlow(enum) = %T, want EnumerationCase", caseValueForFlow("Submitted"))
	}
	if got.Value != "Submitted" {
		t.Fatalf("enum case value = %q, want Submitted", got.Value)
	}
}
