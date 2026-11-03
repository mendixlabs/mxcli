// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// NOTE: The full Check() logic requires ctx.Reader().GetMicroflow() to read microflow
// positions from a real MPR file. The overlap detection algorithm (collect positions →
// pairwise distance check) is inline in Check() and cannot be unit-tested without
// building a mock mpr.Reader. This rule currently lacks end-to-end coverage;
// behavioral testing requires a real .mpr project with overlapping activities.

func TestOverlappingActivitiesRule_NilReader(t *testing.T) {
	r := NewOverlappingActivitiesRule()
	ctx := linter.NewLintContextFromDB(nil)
	violations := r.Check(ctx)
	if violations != nil {
		t.Errorf("expected nil with nil reader, got %v", violations)
	}
}

func TestOverlappingActivitiesRule_Metadata(t *testing.T) {
	r := NewOverlappingActivitiesRule()
	if r.ID() != "MPR008" {
		t.Errorf("ID = %q, want MPR008", r.ID())
	}
	if r.Category() != "correctness" {
		t.Errorf("Category = %q, want correctness", r.Category())
	}
	if r.Name() != "OverlappingActivities" {
		t.Errorf("Name = %q, want OverlappingActivities", r.Name())
	}
}

// The Check() method walks microflow positions via ctx.Reader().GetMicroflow() and detects
// overlapping activities using pairwise distance checks against internal heuristic constants
// (activityBoxWidth, activityBoxHeight). Since the collect function is defined inline in
// Check(), behavioral testing requires a real *mpr.Reader with positioned activities.

// upstream #884. MPR008 flattened a loop's children into the same list as the
// microflow's own canvas and compared every pair. A LoopedActivity's children are
// positioned RELATIVE to the container, so that compares two different coordinate
// spaces and reports overlaps that cannot happen on screen.
//
// Verified against a real project before and after: an outer activity at
// (150,150) and a loop child at (141,130) were reported as overlapping by the
// pre-fix binary and are not by the fixed one, while a genuine same-canvas pair
// at (300,200)/(310,210) is still reported by both.
func TestOverlapPlanesSeparatesContainerCanvases(t *testing.T) {
	inner := &microflows.ActionActivity{}
	inner.Position = model.Point{X: 141, Y: 130}
	inner.Caption = "inner"

	loop := &microflows.LoopedActivity{
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{inner},
		},
	}
	loop.Position = model.Point{X: 520, Y: 230}
	loop.Caption = "loop"

	outer := &microflows.ActionActivity{}
	outer.Position = model.Point{X: 150, Y: 150}
	outer.Caption = "outer"

	planes := overlapPlanes([]microflows.MicroflowObject{outer, loop})

	if len(planes) != 2 {
		t.Fatalf("got %d planes, want 2 (the microflow canvas and the loop's own)", len(planes))
	}

	// The container sits on the parent's plane; only its children move.
	captions := map[string]int{}
	for i, plane := range planes {
		for _, a := range plane {
			captions[a.caption] = i
		}
	}
	if captions["outer"] != captions["loop"] {
		t.Errorf("the loop container must share the parent canvas with 'outer': outer=%d loop=%d",
			captions["outer"], captions["loop"])
	}
	if captions["inner"] == captions["outer"] {
		t.Error("the loop's CHILD must not share a plane with the microflow's own canvas — " +
			"its coordinates are relative to the container, so comparing them is meaningless")
	}
}

// A container with no children must not produce a phantom plane holding nothing,
// and a flat microflow must produce exactly one.
func TestOverlapPlanesFlatMicroflow(t *testing.T) {
	a := &microflows.ActionActivity{}
	a.Position = model.Point{X: 300, Y: 200}
	b := &microflows.ActionActivity{}
	b.Position = model.Point{X: 310, Y: 210}

	planes := overlapPlanes([]microflows.MicroflowObject{a, b})
	if len(planes) != 1 {
		t.Fatalf("got %d planes, want 1", len(planes))
	}
	if len(planes[0]) != 2 {
		t.Errorf("got %d activities on the canvas, want 2 — both must still be compared "+
			"against each other, or the fix trades a false positive for a false negative", len(planes[0]))
	}
}

// MPR008 compared every node as a 120x60 activity box, so two 40x40 merges
// that auto-layout placed edge to edge — inner at (980,200), outer at
// (1020,200), touching at x=1000 — were reported as overlapping, and
// `mxcli layout flows` (which measures with the stored sizes) said the flow
// was already laid out. Overlap is judged with each node's stored size.
func TestOverlapUsesStoredSizes(t *testing.T) {
	merge := func(x, y int) *microflows.ExclusiveMerge {
		m := &microflows.ExclusiveMerge{}
		m.Position = model.Point{X: x, Y: y}
		m.Size = model.Size{Width: 40, Height: 40}
		return m
	}
	act := func(x, y int) *microflows.ActionActivity {
		a := &microflows.ActionActivity{}
		a.Position = model.Point{X: x, Y: y}
		a.Size = model.Size{Width: 120, Height: 60}
		return a
	}
	for _, tc := range []struct {
		name string
		objs []microflows.MicroflowObject
		want bool
	}{
		{"abutting merges (the report)", []microflows.MicroflowObject{merge(980, 200), merge(1020, 200)}, false},
		{"overlapping merges", []microflows.MicroflowObject{merge(980, 200), merge(1000, 200)}, true},
		{"activities 10px apart overlap (control)", []microflows.MicroflowObject{act(300, 200), act(310, 210)}, true},
		{"activities side by side", []microflows.MicroflowObject{act(300, 200), act(490, 200)}, false},
		{"merge inside an activity", []microflows.MicroflowObject{act(300, 200), merge(330, 200)}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := overlapPlanes(tc.objs)[0]
			if got := boxesOverlap(p[0], p[1]); got != tc.want {
				t.Errorf("overlap = %v, want %v (%+v, %+v)", got, tc.want, p[0], p[1])
			}
		})
	}
}

// ako/mxcli#1768: two error handler rows laid on top of each other put the first
// handler's end event and the second handler's first activity both on 680;330.
// MPR008 compared activities, splits and merges only, so it reported nothing —
// and with one more statement in the first handler it reported one of the two
// overlaps, the one that happened to be activity on activity. Events are nodes
// on the canvas and are compared too, with their own 20x20 box.
func TestOverlapComparesEvents(t *testing.T) {
	act := func(x, y int) *microflows.ActionActivity {
		a := &microflows.ActionActivity{}
		a.Position = model.Point{X: x, Y: y}
		a.Size = model.Size{Width: 120, Height: 60}
		return a
	}
	end := func(x, y int, sized bool) *microflows.EndEvent {
		e := &microflows.EndEvent{}
		e.Position = model.Point{X: x, Y: y}
		if sized {
			e.Size = model.Size{Width: 20, Height: 20}
		}
		return e
	}
	for _, tc := range []struct {
		name string
		objs []microflows.MicroflowObject
		want bool
	}{
		{"end event on an activity (the report)", []microflows.MicroflowObject{end(680, 330, true), act(680, 330)}, true},
		{"end event one pitch after an activity", []microflows.MicroflowObject{act(520, 330), end(680, 330, true)}, false},
		// An event that stores no size is an event, not an activity: judged as a
		// 120x60 box it would overlap an activity 80px away.
		{"unsized end event 80px from an activity", []microflows.MicroflowObject{act(600, 200), end(680, 200, false)}, false},
		{"unsized end event inside an activity", []microflows.MicroflowObject{act(600, 200), end(640, 200, false)}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := overlapPlanes(tc.objs)[0]
			if len(p) != 2 {
				t.Fatalf("got %d nodes on the canvas, want 2 — the event was left out", len(p))
			}
			if got := boxesOverlap(p[0], p[1]); got != tc.want {
				t.Errorf("overlap = %v, want %v (%+v, %+v)", got, tc.want, p[0], p[1])
			}
		})
	}

	// Every kind of event, and a type split, is on the canvas.
	nodes := []microflows.MicroflowObject{
		&microflows.StartEvent{}, &microflows.EndEvent{}, &microflows.ErrorEvent{},
		&microflows.BreakEvent{}, &microflows.ContinueEvent{}, &microflows.InheritanceSplit{},
	}
	for i, n := range nodes {
		n.SetPosition(model.Point{X: 100 + 200*i, Y: 200})
	}
	if got := len(overlapPlanes(nodes)[0]); got != len(nodes) {
		t.Errorf("%d of %d nodes are on the canvas", got, len(nodes))
	}
}

// ako/mxcli#1839 (mendixlabs/mxcli#1690): when one of the overlapping boxes is a
// loop, the overlap is the loop's BOX — sized by its content — reaching into the
// activity before or after it, not two statements sharing one @position. The
// generic "each statement needs its own @position" sent users adding
// annotations that were already there.
func TestOverlapMessageNamesTheLoop(t *testing.T) {
	act := &microflows.ActionActivity{}
	act.Position = model.Point{X: 300, Y: 200}
	act.Caption = "Retrieve orders"
	loop := &microflows.LoopedActivity{}
	loop.Position = model.Point{X: 420, Y: 200}
	loop.Size = model.Size{Width: 400, Height: 200}
	loop.Caption = "Iterate orders"

	planes := overlapPlanes([]microflows.MicroflowObject{act, loop})
	a, b := planes[0][0], planes[0][1]
	if !boxesOverlap(a, b) {
		t.Fatal("fixture does not overlap")
	}
	msg, sug := overlapViolationText(a, b, "microflow", "M", "ProcessOrders")
	for _, want := range []string{"loop 'Iterate orders'", "400x200", "overlaps 'Retrieve orders'"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q does not contain %q", msg, want)
		}
	}
	if strings.Contains(msg, "needs its own @position") {
		t.Errorf("loop overlap still blames missing @position: %q", msg)
	}
	if !strings.Contains(sug, "gap") || !strings.Contains(sug, "@position") {
		t.Errorf("suggestion %q should say to widen the gap or drop the @position", sug)
	}
	// Order of the pair must not matter.
	if msg2, _ := overlapViolationText(b, a, "microflow", "M", "ProcessOrders"); !strings.Contains(msg2, "loop 'Iterate orders'") {
		t.Errorf("loop hint lost when the loop is the first box: %q", msg2)
	}
}

// Control: two plain activities keep the generic @position message.
func TestOverlapMessageGenericForActivities(t *testing.T) {
	a := actInfo{x: 100, y: 100, w: 120, h: 60, caption: "Declare"}
	b := actInfo{x: 150, y: 100, w: 120, h: 60, caption: "Set"}
	msg, sug := overlapViolationText(a, b, "microflow", "M", "MF")
	if !strings.Contains(msg, "needs its own @position") || strings.Contains(msg, "loop") {
		t.Errorf("generic overlap message changed: %q", msg)
	}
	if !strings.Contains(sug, "190px") {
		t.Errorf("generic suggestion changed: %q", sug)
	}
}
