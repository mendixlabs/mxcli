// SPDX-License-Identifier: Apache-2.0

// Regression tests for anchor preservation inside IF branches — the original
// @anchor implementation only handled the top-level flow between statements
// at the microflow body level. Anchors on statements inside THEN/ELSE bodies
// (including the flow between a branch's first statement and its successors,
// and the flow leaving the last branch statement to the merge) were silently
// dropped, so real-world microflows like the attempt #35 repro case lost
// every vertical anchor on roundtrip.
package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// buildWithAnchors is a test helper that builds the flow graph for a simple
// microflow body and returns the collection for inspection.
func buildWithAnchors(body []ast.MicroflowStatement) (oc *struct {
	Flows   []anchorFlow
	Objects int
}) {
	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing}
	col := fb.buildFlowGraph(body, nil)
	oc = &struct {
		Flows   []anchorFlow
		Objects int
	}{
		Objects: len(col.Objects),
	}
	for _, f := range col.Flows {
		oc.Flows = append(oc.Flows, anchorFlow{
			OriginIdx: f.OriginConnectionIndex,
			DestIdx:   f.DestinationConnectionIndex,
		})
	}
	return oc
}

type anchorFlow struct {
	OriginIdx int
	DestIdx   int
}

// hasFlow returns true when at least one flow has the given anchor pair.
func hasFlow(flows []anchorFlow, origin, dest int) bool {
	for _, f := range flows {
		if f.OriginIdx == origin && f.DestIdx == dest {
			return true
		}
	}
	return false
}

// countFlows counts how many flows have the given anchor pair.
func countFlows(flows []anchorFlow, origin, dest int) int {
	n := 0
	for _, f := range flows {
		if f.OriginIdx == origin && f.DestIdx == dest {
			n++
		}
	}
	return n
}

func TestBuilder_AnchorInsideElseBranch(t *testing.T) {
	// Reproduces the pattern from attempt #35:
	//   if cond then { set ... }
	//   else {
	//     @anchor(from: bottom, to: top)
	//     log ...
	//     @anchor(to: top)
	//     return empty
	//   }
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			ThenBody: []ast.MicroflowStatement{
				&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "a"}},
			},
			ElseBody: []ast.MicroflowStatement{
				&ast.LogStmt{
					Level:   ast.LogInfo,
					Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "b"},
					Annotations: &ast.ActivityAnnotations{
						Anchor: &ast.FlowAnchors{From: ast.AnchorSideBottom, To: ast.AnchorSideTop},
					},
				},
				&ast.ReturnStmt{
					Value: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "done"},
					Annotations: &ast.ActivityAnnotations{
						Anchor: &ast.FlowAnchors{To: ast.AnchorSideTop, From: ast.AnchorSideUnset},
					},
				},
			},
		},
	}

	oc := buildWithAnchors(body)

	// Two distinct Bottom→Top flows must exist:
	//   1. split → log   (from the user's @anchor on the log statement)
	//   2. log   → return (propagating the log's From=Bottom and the return's To=Top)
	// A single hasFlow check would pass with just one match, so count explicitly
	// to pin the regression — see ako review note on TestBuilder_AnchorInsideElseBranch.
	if got := countFlows(oc.Flows, AnchorBottom, AnchorTop); got != 2 {
		t.Errorf("expected 2 Bottom→Top flows (split→log and log→return), got %d: %+v", got, oc.Flows)
	}
}

// TestBuilder_AnchorFalseBranchTo_IfWithoutElse pins a regression: when the
// describer emits
//
//	@anchor(to: left, true: (from: right, to: left), false: (from: bottom, to: top))
//
// on an IF-without-ELSE split, the writer used to apply only the FROM side of
// the false-branch anchor to the split→merge flow, letting the default (Left)
// overwrite the intended `to: top`. Re-describing produced `false: (from: bottom, to: left)`.
func TestBuilder_AnchorFalseBranchTo_IfWithoutElse(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			ThenBody: []ast.MicroflowStatement{
				&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "inside"}},
			},
			Annotations: &ast.ActivityAnnotations{
				FalseBranchAnchor: &ast.FlowAnchors{From: ast.AnchorSideBottom, To: ast.AnchorSideTop},
			},
		},
	}

	oc := buildWithAnchors(body)

	// The split → merge false flow must land on merge's Top side, not the default Left.
	if !hasFlow(oc.Flows, AnchorBottom, AnchorTop) {
		t.Errorf("expected false branch split→merge flow Bottom→Top, got %+v", oc.Flows)
	}
}

// TestBuilder_AnchorTrueBranchTo_EmptyThenIfWithElse is the ELSE-branch mirror
// of the cluster B4 regression: an empty THEN body connecting straight to the
// merge must honor the trueBranchAnchor.To.
func TestBuilder_AnchorTrueBranchTo_EmptyThenIfWithElse(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			ThenBody:  nil, // empty THEN body
			ElseBody: []ast.MicroflowStatement{
				&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "else"}},
			},
			Annotations: &ast.ActivityAnnotations{
				TrueBranchAnchor: &ast.FlowAnchors{From: ast.AnchorSideRight, To: ast.AnchorSideTop},
			},
		},
	}

	oc := buildWithAnchors(body)

	// Empty THEN → merge: must honor the user's Right→Top anchor, not the default Right→Left.
	if !hasFlow(oc.Flows, AnchorRight, AnchorTop) {
		t.Errorf("expected true branch split→merge flow Right→Top, got %+v", oc.Flows)
	}
}

func TestDescribe_FalseBranchFromTop_IfWithoutElseIsDefault(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			ThenBody: []ast.MicroflowStatement{
				&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "inside"}},
			},
			Annotations: &ast.ActivityAnnotations{
				FalseBranchAnchor: &ast.FlowAnchors{From: ast.AnchorSideTop, To: ast.AnchorSideUnset},
			},
		},
		&ast.ReturnStmt{},
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing}
	oc := fb.buildFlowGraph(body, nil)

	var split microflows.MicroflowObject
	for _, obj := range oc.Objects {
		if _, ok := obj.(*microflows.ExclusiveSplit); ok {
			split = obj
			break
		}
	}
	if split == nil {
		t.Fatal("expected exclusive split")
	}

	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{}
	flowsByDest := map[model.ID][]*microflows.SequenceFlow{}
	for _, flow := range oc.Flows {
		flowsByOrigin[flow.OriginID] = append(flowsByOrigin[flow.OriginID], flow)
		flowsByDest[flow.DestinationID] = append(flowsByDest[flow.DestinationID], flow)
	}

	var lines []string
	emitAnchorAnnotation(split, flowsByOrigin, flowsByDest, &lines, "")
	if len(lines) != 0 {
		t.Fatalf("expected false branch from-top default anchor to be omitted, got %q", strings.Join(lines, "\n"))
	}
}

func TestBuilder_AnchorToTopOnReturnPreservedInsideElse(t *testing.T) {
	// Minimal case: single-statement ELSE whose only statement is a RETURN
	// carrying @anchor(to: top). The flow from the split to that return's
	// EndEvent must land on DestinationConnectionIndex = AnchorTop.
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			ThenBody: []ast.MicroflowStatement{
				&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "a"}},
			},
			ElseBody: []ast.MicroflowStatement{
				&ast.ReturnStmt{
					Value: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "no"},
					Annotations: &ast.ActivityAnnotations{
						Anchor: &ast.FlowAnchors{To: ast.AnchorSideTop, From: ast.AnchorSideUnset},
					},
				},
			},
		},
	}

	oc := buildWithAnchors(body)

	// Default downward flow from split has OriginConnectionIndex=Bottom; with
	// @anchor(to: top) on the return, DestinationConnectionIndex must be Top.
	if !hasFlow(oc.Flows, AnchorBottom, AnchorTop) {
		t.Errorf("expected split→return flow with Bottom→Top, got %+v", oc.Flows)
	}
}
