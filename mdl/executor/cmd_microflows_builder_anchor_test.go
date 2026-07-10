// SPDX-License-Identifier: Apache-2.0

// Tests for @anchor annotation wiring — the builder must copy
// AST Anchor/TrueBranchAnchor/FalseBranchAnchor fields into the
// OriginConnectionIndex/DestinationConnectionIndex of emitted SequenceFlows.
package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestBuilder_AnchorOverridesFlowEndpoints(t *testing.T) {
	// Simple body: log statement with an @anchor on the next statement that
	// connects the log's outgoing flow at "bottom" instead of the default "right".
	body := []ast.MicroflowStatement{
		&ast.LogStmt{
			Level:   ast.LogInfo,
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "a"},
			Annotations: &ast.ActivityAnnotations{
				Anchor: &ast.FlowAnchors{From: ast.AnchorSideBottom, To: ast.AnchorSideUnset},
			},
		},
		&ast.LogStmt{
			Level:   ast.LogInfo,
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "b"},
			Annotations: &ast.ActivityAnnotations{
				Anchor: &ast.FlowAnchors{From: ast.AnchorSideUnset, To: ast.AnchorSideTop},
			},
		},
	}

	fb := &flowBuilder{
		posX:    100,
		posY:    100,
		spacing: HorizontalSpacing,
	}
	oc := fb.buildFlowGraph(body, nil)

	// Expect 3 flows: start→log1, log1→log2, log2→end.
	if len(oc.Flows) != 3 {
		t.Fatalf("expected 3 flows, got %d", len(oc.Flows))
	}

	log1ToLog2 := oc.Flows[1]
	if log1ToLog2.OriginConnectionIndex != AnchorBottom {
		t.Errorf("log1→log2 origin index: got %d, want %d (Bottom)",
			log1ToLog2.OriginConnectionIndex, AnchorBottom)
	}
	if log1ToLog2.DestinationConnectionIndex != AnchorTop {
		t.Errorf("log1→log2 destination index: got %d, want %d (Top)",
			log1ToLog2.DestinationConnectionIndex, AnchorTop)
	}
}

func TestBuilder_AnchorOmittedKeepsDefault(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.LogStmt{
			Level:   ast.LogInfo,
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "a"},
		},
		&ast.LogStmt{
			Level:   ast.LogInfo,
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "b"},
		},
	}

	fb := &flowBuilder{
		posX:    100,
		posY:    100,
		spacing: HorizontalSpacing,
	}
	oc := fb.buildFlowGraph(body, nil)

	log1ToLog2 := oc.Flows[1]
	// Default horizontal flow uses Right → Left.
	if log1ToLog2.OriginConnectionIndex != AnchorRight {
		t.Errorf("default origin: got %d, want %d (Right)",
			log1ToLog2.OriginConnectionIndex, AnchorRight)
	}
	if log1ToLog2.DestinationConnectionIndex != AnchorLeft {
		t.Errorf("default destination: got %d, want %d (Left)",
			log1ToLog2.DestinationConnectionIndex, AnchorLeft)
	}
}

func TestBuilder_AnchorPartialOverride(t *testing.T) {
	// Only the From side is overridden; the To side must remain at the default.
	body := []ast.MicroflowStatement{
		&ast.LogStmt{
			Level:   ast.LogInfo,
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "a"},
			Annotations: &ast.ActivityAnnotations{
				Anchor: &ast.FlowAnchors{From: ast.AnchorSideTop, To: ast.AnchorSideUnset},
			},
		},
		&ast.LogStmt{
			Level:   ast.LogInfo,
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "b"},
		},
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing}
	oc := fb.buildFlowGraph(body, nil)

	log1ToLog2 := oc.Flows[1]
	if log1ToLog2.OriginConnectionIndex != AnchorTop {
		t.Errorf("origin: got %d, want %d (Top)", log1ToLog2.OriginConnectionIndex, AnchorTop)
	}
	if log1ToLog2.DestinationConnectionIndex != AnchorLeft {
		t.Errorf("destination stayed default: got %d, want %d (Left)",
			log1ToLog2.DestinationConnectionIndex, AnchorLeft)
	}
}

func TestBuilder_IfBranchAnchorOverrides(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			ThenBody: []ast.MicroflowStatement{
				&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "y"}},
			},
			ElseBody: []ast.MicroflowStatement{
				&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "n"}},
			},
			Annotations: &ast.ActivityAnnotations{
				TrueBranchAnchor:  &ast.FlowAnchors{From: ast.AnchorSideTop, To: ast.AnchorSideLeft},
				FalseBranchAnchor: &ast.FlowAnchors{From: ast.AnchorSideBottom, To: ast.AnchorSideTop},
			},
		},
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing}
	oc := fb.buildFlowGraph(body, nil)

	// Identify the TRUE/FALSE split outgoing flows via the describer's own
	// helper so we match every CaseValue variant the builder can emit
	// (ExpressionCase, EnumerationCase — value and pointer forms, BooleanCase).
	trueF, falseF := findBranchFlows(oc.Flows)
	if trueF == nil {
		t.Fatalf("expected a TRUE split flow, got none among %+v", oc.Flows)
	}
	if falseF == nil {
		t.Fatalf("expected a FALSE split flow, got none among %+v", oc.Flows)
	}
	if trueF.OriginConnectionIndex != AnchorTop || trueF.DestinationConnectionIndex != AnchorLeft {
		t.Errorf("true branch anchors: got from=%d to=%d, want Top(%d)/Left(%d)",
			trueF.OriginConnectionIndex, trueF.DestinationConnectionIndex, AnchorTop, AnchorLeft)
	}
	if falseF.OriginConnectionIndex != AnchorBottom || falseF.DestinationConnectionIndex != AnchorTop {
		t.Errorf("false branch anchors: got from=%d to=%d, want Bottom(%d)/Top(%d)",
			falseF.OriginConnectionIndex, falseF.DestinationConnectionIndex, AnchorBottom, AnchorTop)
	}
}

func TestBranchDestinationAnchorPrefersBranchToSide(t *testing.T) {
	branchAnchor := &ast.FlowAnchors{From: ast.AnchorSideRight, To: ast.AnchorSideTop}
	stmtAnchor := &ast.FlowAnchors{From: ast.AnchorSideBottom, To: ast.AnchorSideLeft}

	got := branchDestinationAnchor(branchAnchor, stmtAnchor)
	if got != branchAnchor {
		t.Fatal("expected branch-level destination anchor to win for split-to-branch flow")
	}
	if got.To != ast.AnchorSideTop {
		t.Fatalf("destination side = %v, want top", got.To)
	}
}
