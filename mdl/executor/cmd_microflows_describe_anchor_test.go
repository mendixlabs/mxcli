// SPDX-License-Identifier: Apache-2.0

// Tests that the describer emits @anchor lines for every activity with
// attached SequenceFlows, with the correct side keywords derived from the
// flow's OriginConnectionIndex / DestinationConnectionIndex.
package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestEmitAnchorAnnotation_FromAndTo(t *testing.T) {
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: "act-1"},
			},
		},
	}
	incoming := &microflows.SequenceFlow{
		DestinationID:              "act-1",
		DestinationConnectionIndex: AnchorTop,
	}
	outgoing := &microflows.SequenceFlow{
		OriginID:              "act-1",
		OriginConnectionIndex: AnchorBottom,
	}

	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		"act-1": {outgoing},
	}
	flowsByDest := map[model.ID][]*microflows.SequenceFlow{
		"act-1": {incoming},
	}

	var lines []string
	emitAnchorAnnotation(activity, flowsByOrigin, flowsByDest, &lines, "")

	if len(lines) != 1 {
		t.Fatalf("expected 1 anchor line, got %d: %v", len(lines), lines)
	}
	want := "@anchor(from: bottom, to: top)"
	if lines[0] != want {
		t.Errorf("anchor line: got %q, want %q", lines[0], want)
	}
}

func TestEmitAnchorAnnotation_OmitsDefaultRightToLeft(t *testing.T) {
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: "act-default"},
			},
		},
	}
	incoming := &microflows.SequenceFlow{
		DestinationID:              "act-default",
		DestinationConnectionIndex: AnchorLeft,
	}
	outgoing := &microflows.SequenceFlow{
		OriginID:              "act-default",
		OriginConnectionIndex: AnchorRight,
	}

	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		"act-default": {outgoing},
	}
	flowsByDest := map[model.ID][]*microflows.SequenceFlow{
		"act-default": {incoming},
	}

	var lines []string
	emitAnchorAnnotation(activity, flowsByOrigin, flowsByDest, &lines, "")

	if len(lines) != 0 {
		t.Fatalf("expected default anchor line to be omitted, got %v", lines)
	}
}

func TestEmitAnchorAnnotation_NoFlowsSkipsEmission(t *testing.T) {
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: "lonely"},
			},
		},
	}
	var lines []string
	emitAnchorAnnotation(activity, nil, nil, &lines, "")

	if len(lines) != 0 {
		t.Errorf("expected no lines emitted, got %v", lines)
	}
}

func TestEmitAnchorAnnotation_IgnoresLoopBodyTailOutgoingFlow(t *testing.T) {
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: "body-act"},
			},
		},
	}
	loop := &microflows.LoopedActivity{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: "loop-1"},
		},
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{activity},
		},
	}

	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		"body-act": {
			{
				OriginID:              "body-act",
				DestinationID:         "loop-1",
				OriginConnectionIndex: AnchorLeft,
			},
		},
	}
	flowsByDest := map[model.ID][]*microflows.SequenceFlow{
		"body-act": {
			{
				DestinationID:              "body-act",
				DestinationConnectionIndex: AnchorLeft,
			},
		},
	}
	activityMap := map[model.ID]microflows.MicroflowObject{
		"body-act": activity,
		"loop-1":   loop,
	}

	var lines []string
	emitAnchorAnnotationWithActivityMap(activity, flowsByOrigin, flowsByDest, activityMap, &lines, "")

	if len(lines) != 0 {
		t.Fatalf("expected loop body tail flow to be ignored, got %v", lines)
	}
}

func TestAnchorRoundtripViaParserBuilder(t *testing.T) {
	// Build an AST with an @anchor on a statement, run it through the builder,
	// and verify the resulting SequenceFlow has the right anchors. Then the
	// describer's anchor keyword should match what was requested.
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

	// Find log1 and its outgoing flow
	var log1ID model.ID
	objs := oc.Objects
	for _, o := range objs {
		if act, ok := o.(*microflows.ActionActivity); ok {
			if log, ok := act.Action.(*microflows.LogMessageAction); ok {
				if strings.Contains(log.MessageTemplate.GetTranslation("en_US"), "a") {
					log1ID = o.GetID()
					break
				}
			}
		}
	}
	if log1ID == "" {
		t.Fatal("could not locate log1 activity")
	}

	for _, f := range oc.Flows {
		if f.OriginID == log1ID {
			if f.OriginConnectionIndex != AnchorTop {
				t.Errorf("outgoing flow origin: got %d, want %d (Top)", f.OriginConnectionIndex, AnchorTop)
			}
			return
		}
	}
	t.Fatal("did not find outgoing flow from log1")
}
