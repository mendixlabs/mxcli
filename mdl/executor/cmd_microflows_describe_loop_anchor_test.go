// SPDX-License-Identifier: Apache-2.0

// Tests for LOOP/WHILE @anchor emission in the describer.
// A LoopedActivity with iterator and/or tail flows (loop ↔ inner body) must
// produce an @anchor(..., iterator: (...), tail: (...)) line. Without such
// flows only the outer from/to keywords should appear (and nothing when the
// loop has no attached flows at all).
package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestEmitLoopAnchorAnnotation_IteratorAndTail(t *testing.T) {
	inner := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: mkID("body-1")},
			},
		},
	}
	loop := &microflows.LoopedActivity{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: mkID("loop-1")},
		},
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{inner},
		},
	}

	// Outer flows (sibling activities connect to/from the loop).
	outerIn := &microflows.SequenceFlow{
		OriginID:                   mkID("prev"),
		DestinationID:              mkID("loop-1"),
		DestinationConnectionIndex: AnchorLeft,
	}
	outerOut := &microflows.SequenceFlow{
		OriginID:              mkID("loop-1"),
		DestinationID:         mkID("next"),
		OriginConnectionIndex: AnchorRight,
	}
	// Iterator: loop → first body.
	iter := &microflows.SequenceFlow{
		OriginID:                   mkID("loop-1"),
		DestinationID:              mkID("body-1"),
		OriginConnectionIndex:      AnchorBottom,
		DestinationConnectionIndex: AnchorTop,
	}
	// Tail: body → loop.
	tail := &microflows.SequenceFlow{
		OriginID:                   mkID("body-1"),
		DestinationID:              mkID("loop-1"),
		OriginConnectionIndex:      AnchorRight,
		DestinationConnectionIndex: AnchorBottom,
	}

	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		mkID("loop-1"): {outerOut, iter},
		mkID("body-1"): {tail},
	}
	flowsByDest := map[model.ID][]*microflows.SequenceFlow{
		mkID("loop-1"): {outerIn, tail},
		mkID("body-1"): {iter},
	}

	var lines []string
	emitLoopAnchorAnnotation(loop, flowsByOrigin, flowsByDest, &lines, "")

	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	got := lines[0]
	want := "@anchor(from: right, to: left, iterator: (from: bottom, to: top), tail: (from: right, to: bottom))"
	if got != want {
		t.Errorf("\n got: %s\nwant: %s", got, want)
	}
}

func TestEmitLoopAnchorAnnotation_NoFlowsProducesNothing(t *testing.T) {
	loop := &microflows.LoopedActivity{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: mkID("loop-1")},
		},
	}

	var lines []string
	emitLoopAnchorAnnotation(loop, nil, nil, &lines, "")
	if len(lines) != 0 {
		t.Errorf("expected no anchor emission, got: %v", lines)
	}
}

func TestEmitLoopAnchorAnnotation_OuterOnlyNoIteratorOrTail(t *testing.T) {
	inner := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: mkID("body-1")},
			},
		},
	}
	loop := &microflows.LoopedActivity{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: mkID("loop-1")},
		},
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{inner},
		},
	}
	outerIn := &microflows.SequenceFlow{
		OriginID:                   mkID("prev"),
		DestinationID:              mkID("loop-1"),
		DestinationConnectionIndex: AnchorLeft,
	}
	outerOut := &microflows.SequenceFlow{
		OriginID:              mkID("loop-1"),
		DestinationID:         mkID("next"),
		OriginConnectionIndex: AnchorRight,
	}
	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		mkID("loop-1"): {outerOut},
	}
	flowsByDest := map[model.ID][]*microflows.SequenceFlow{
		mkID("loop-1"): {outerIn},
	}

	var lines []string
	emitLoopAnchorAnnotation(loop, flowsByOrigin, flowsByDest, &lines, "")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	got := lines[0]
	want := "@anchor(from: right, to: left)"
	if got != want {
		t.Errorf("\n got: %s\nwant: %s", got, want)
	}
}

func TestEmitLoopAnchorAnnotation_IteratorOnly(t *testing.T) {
	inner := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: mkID("body-1")},
			},
		},
	}
	loop := &microflows.LoopedActivity{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: mkID("loop-1")},
		},
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{inner},
		},
	}
	iter := &microflows.SequenceFlow{
		OriginID:                   mkID("loop-1"),
		DestinationID:              mkID("body-1"),
		OriginConnectionIndex:      AnchorBottom,
		DestinationConnectionIndex: AnchorLeft,
	}

	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		mkID("loop-1"): {iter},
	}
	flowsByDest := map[model.ID][]*microflows.SequenceFlow{
		mkID("body-1"): {iter},
	}

	var lines []string
	emitLoopAnchorAnnotation(loop, flowsByOrigin, flowsByDest, &lines, "")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	got := lines[0]
	want := "@anchor(iterator: (from: bottom, to: left))"
	if got != want {
		t.Errorf("\n got: %s\nwant: %s", got, want)
	}
}
