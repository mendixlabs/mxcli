// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// These tests run the splice on hand-built documents, for the shapes the
// Studio Pro fixture does not have (loops, error handlers, vertical flows).
// The acceptance test on a Studio Pro-drawn flow is in the executor package
// (cmd_alter_flow_pedapp_test.go).

// uid returns a deterministic element id for a short name.
func uid(name string) string {
	b := make([]byte, 16)
	copy(b, name)
	return types.BlobToUUID(b)
}

func bin(name string) bson.Binary {
	return bson.Binary{Subtype: 0, Data: types.UUIDToBlob(uid(name))}
}

func obj(name, typ string, x, y int) bson.D {
	w, h := 120, 60
	if typ != "Microflows$ActionActivity" && typ != "Microflows$LoopedActivity" {
		w, h = 20, 20
	}
	return bson.D{
		{Key: "$ID", Value: bin(name)},
		{Key: "$Type", Value: typ},
		{Key: "RelativeMiddlePoint", Value: fmt.Sprintf("%d;%d", x, y)},
		{Key: "Size", Value: fmt.Sprintf("%d;%d", w, h)},
	}
}

func flow(name, from, to string, fromSide, toSide int32, isErr bool) bson.D {
	return bson.D{
		{Key: "$ID", Value: bin(name)},
		{Key: "$Type", Value: "Microflows$SequenceFlow"},
		{Key: "CaseValues", Value: bson.A{int32(2), bson.D{{Key: "$ID", Value: bin(name + "c")}, {Key: "$Type", Value: "Microflows$NoCase"}}}},
		{Key: "DestinationConnectionIndex", Value: toSide},
		{Key: "DestinationPointer", Value: bin(to)},
		{Key: "IsErrorHandler", Value: isErr},
		{Key: "Line", Value: bson.D{
			{Key: "$ID", Value: bin(name + "l")},
			{Key: "$Type", Value: "Microflows$BezierCurve"},
			{Key: "DestinationControlVector", Value: "-30;0"},
			{Key: "OriginControlVector", Value: "30;0"},
		}},
		{Key: "OriginConnectionIndex", Value: fromSide},
		{Key: "OriginPointer", Value: bin(from)},
	}
}

func unit(objects []bson.D, flows []bson.D) bson.D {
	objs := bson.A{int32(3)}
	for _, o := range objects {
		objs = append(objs, o)
	}
	fl := bson.A{int32(3)}
	for _, f := range flows {
		fl = append(fl, f)
	}
	return bson.D{
		{Key: "$ID", Value: bin("unit")},
		{Key: "$Type", Value: "Microflows$Microflow"},
		{Key: "Flows", Value: fl},
		{Key: "ObjectCollection", Value: bson.D{
			{Key: "$ID", Value: bin("oc")},
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: objs},
		}},
	}
}

// fakeDeps serializes the way the codec does for the properties the splice
// reads, and records what was saved.
type fakeDeps struct{ saved []byte }

func (d *fakeDeps) SerializeObject(o microflows.MicroflowObject) (bson.D, error) {
	typ := "Microflows$ActionActivity"
	switch o.(type) {
	case *microflows.ExclusiveMerge:
		typ = "Microflows$ExclusiveMerge"
	case *microflows.Annotation:
		typ = "Microflows$Annotation"
	}
	p := o.GetPosition()
	sz := objectSize(o)
	doc := bson.D{
		{Key: "$ID", Value: bson.Binary{Data: types.UUIDToBlob(string(o.GetID()))}},
		{Key: "$Type", Value: typ},
		{Key: "RelativeMiddlePoint", Value: fmt.Sprintf("%d;%d", p.X, p.Y)},
		{Key: "Size", Value: fmt.Sprintf("%d;%d", sz.X, sz.Y)},
	}
	// A loop carries its body, as the codec writes it (ReplaceLoopBody reads
	// it from there).
	if l, ok := o.(*microflows.LoopedActivity); ok {
		doc[1].Value = "Microflows$LoopedActivity"
		body := bson.A{int32(3)}
		if l.ObjectCollection != nil {
			for _, b := range l.ObjectCollection.Objects {
				bd, err := d.SerializeObject(b)
				if err != nil {
					return nil, err
				}
				body = append(body, bd)
			}
		}
		doc = append(doc, bson.E{Key: "ObjectCollection", Value: bson.D{
			{Key: "$ID", Value: bson.Binary{Data: types.UUIDToBlob(types.GenerateID())}},
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: body},
		}})
	}
	// The error handling is stored on the action, as the codec writes it.
	if eh := microflows.ObjectErrorHandlingType(o); eh != "" {
		doc = append(doc, bson.E{Key: "Action", Value: bson.D{
			{Key: "$Type", Value: "Microflows$MicroflowCallAction"},
			{Key: "ErrorHandlingType", Value: string(eh)},
		}})
	}
	return doc, nil
}

func (d *fakeDeps) SerializeSequenceFlow(f *microflows.SequenceFlow) (bson.D, error) {
	doc := bson.D{
		{Key: "$ID", Value: bson.Binary{Data: types.UUIDToBlob(string(f.ID))}},
		{Key: "$Type", Value: "Microflows$SequenceFlow"},
		{Key: "DestinationConnectionIndex", Value: int32(f.DestinationConnectionIndex)},
		{Key: "DestinationPointer", Value: bson.Binary{Data: types.UUIDToBlob(string(f.DestinationID))}},
		{Key: "IsErrorHandler", Value: f.IsErrorHandler},
		{Key: "Line", Value: bson.D{
			{Key: "DestinationControlVector", Value: f.DestinationControlVector},
			{Key: "OriginControlVector", Value: f.OriginControlVector},
		}},
		{Key: "OriginConnectionIndex", Value: int32(f.OriginConnectionIndex)},
		{Key: "OriginPointer", Value: bson.Binary{Data: types.UUIDToBlob(string(f.OriginID))}},
	}
	// An enumeration case, as the real serializer writes one (ako/mxcli#2618).
	if c, ok := f.CaseValue.(*microflows.EnumerationCase); ok {
		doc = append(doc, bson.E{Key: "CaseValues", Value: bson.A{int32(2), bson.D{
			{Key: "$ID", Value: bson.Binary{Data: types.UUIDToBlob(types.GenerateID())}},
			{Key: "$Type", Value: "Microflows$EnumerationCase"},
			{Key: "Value", Value: c.Value},
		}}})
	}
	return doc, nil
}

func (d *fakeDeps) SerializeAnnotationFlow(f *microflows.AnnotationFlow) (bson.D, error) {
	return bson.D{
		{Key: "$ID", Value: bson.Binary{Data: types.UUIDToBlob(string(f.ID))}},
		{Key: "$Type", Value: "Microflows$AnnotationFlow"},
		{Key: "DestinationPointer", Value: bson.Binary{Data: types.UUIDToBlob(string(f.DestinationID))}},
		{Key: "OriginPointer", Value: bson.Binary{Data: types.UUIDToBlob(string(f.OriginID))}},
	}, nil
}

func (d *fakeDeps) SerializeDocument(declared any) (bson.D, error) {
	doc, ok := declared.(bson.D)
	if !ok {
		return nil, fmt.Errorf("fakeDeps encodes a bson.D, not a %T", declared)
	}
	return doc, nil
}

func (d *fakeDeps) SaveUnit(_ string, contents []byte) error {
	d.saved = contents
	return nil
}

func newMutator(t *testing.T, doc bson.D) (*Mutator, *fakeDeps) {
	t.Helper()
	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	deps := &fakeDeps{}
	m, err := New(d, "unit", deps)
	if err != nil {
		t.Fatal(err)
	}
	return m, deps
}

// oneDecision is a fragment holding a decision, in builder coordinates.
func oneDecision() *backend.MicroflowFragment {
	id := model.ID(types.GenerateID())
	split := &microflows.ExclusiveSplit{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: id},
		Position:    model.Point{X: 360, Y: 200},
		Size:        model.Size{Width: 60, Height: 60},
	}}
	return &backend.MicroflowFragment{Objects: []microflows.MicroflowObject{split}, Entry: id, Exit: id}
}

// oneActivity is a single-activity fragment in builder coordinates.
func oneActivity() *backend.MicroflowFragment {
	id := model.ID(types.GenerateID())
	act := &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: id},
		Position:    model.Point{X: 360, Y: 200},
		Size:        model.Size{Width: 120, Height: 60},
	}}}
	return &backend.MicroflowFragment{Objects: []microflows.MicroflowObject{act}, Entry: id, Exit: id}
}

func line() []bson.D {
	return []bson.D{
		obj("start", "Microflows$StartEvent", 100, 200),
		obj("a", "Microflows$ActionActivity", 250, 200),
		obj("b", "Microflows$ActionActivity", 420, 200),
		obj("end", "Microflows$EndEvent", 600, 200),
	}
}

func lineFlows() []bson.D {
	return []bson.D{
		flow("f1", "start", "a", 1, 3, false),
		flow("f2", "a", "b", 1, 3, false),
		flow("f3", "b", "end", 1, 3, false),
	}
}

// The control every splice test leans on: decoding a unit and encoding it
// again with nothing spliced gives back the stored bytes exactly.
func TestSplice_UntouchedUnitRoundTripsExactly(t *testing.T) {
	raw, _ := bson.Marshal(unit(line(), lineFlows()))
	m, _ := newMutator(t, unit(line(), lineFlows()))
	out, err := m.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, raw) {
		t.Error("an unmodified unit did not round-trip byte for byte")
	}
}

func TestSplice_InsertAfterKeepsTheErrorHandlerFlow(t *testing.T) {
	objs := append(line(), obj("handler", "Microflows$ActionActivity", 250, 350))
	flows := append(lineFlows(), flow("err", "a", "handler", 2, 0, true))
	m, _ := newMutator(t, unit(objs, flows))
	errBefore, _ := bson.Marshal(flow("err", "a", "handler", 2, 0, true))

	frag := oneActivity()
	if err := m.InsertAfter(model.ID(uid("a")), frag); err != nil {
		t.Fatal(err)
	}
	g := m.graph()
	for _, f := range g.flows {
		switch f.id {
		case uid("err"):
			got, _ := bson.Marshal(f.doc)
			if !bytes.Equal(got, errBefore) {
				t.Error("the error-handler flow changed")
			}
		case uid("f2"):
			if f.dest != string(frag.Entry) {
				t.Error("the normal flow out of a was not rewired to the fragment")
			}
		}
	}
}

func TestSplice_VerticalFlowUsesTopAndBottom(t *testing.T) {
	objs := []bson.D{
		obj("a", "Microflows$ActionActivity", 200, 100),
		obj("b", "Microflows$ActionActivity", 200, 200),
		obj("below", "Microflows$ActionActivity", 200, 300),
		obj("beside", "Microflows$ActionActivity", 500, 100),
	}
	flows := []bson.D{flow("f", "a", "b", 2, 0, false), flow("g", "b", "below", 2, 0, false)}
	m, _ := newMutator(t, unit(objs, flows))
	frag := oneActivity()
	if err := m.InsertAfter(model.ID(uid("a")), frag); err != nil {
		t.Fatal(err)
	}
	g := m.graph()
	if p := g.nodes[uid("b")].pos; p.X != 200 || p.Y <= 200 {
		t.Errorf("b should have moved down, is at %v", p)
	}
	if p := g.nodes[uid("beside")].pos; p.X != 500 || p.Y != 100 {
		t.Errorf("an object above the cut moved to %v", p)
	}
	for _, f := range g.flows {
		if f.id == uid("f") {
			if idx, _ := flowEnd(f.doc, "Destination"); idx != sideTop {
				t.Errorf("the rewired flow enters the fragment on side %d, want top", idx)
			}
		}
		if f.origin == string(frag.Exit) {
			if idx, _ := flowEnd(f.doc, "Origin"); idx != sideBottom {
				t.Errorf("the new flow leaves the fragment on side %d, want bottom", idx)
			}
		}
	}
}

func TestSplice_Refusals(t *testing.T) {
	loop := obj("loop", "Microflows$LoopedActivity", 420, 200)
	loop = append(loop, bson.E{Key: "ObjectCollection", Value: bson.D{
		{Key: "$ID", Value: bin("loopoc")},
		{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
		{Key: "Objects", Value: bson.A{int32(3), obj("inner", "Microflows$ActionActivity", 100, 60), obj("inner2", "Microflows$ActionActivity", 260, 60)}},
	}})
	withLoop := []bson.D{obj("start", "Microflows$StartEvent", 100, 200), obj("a", "Microflows$ActionActivity", 250, 200), loop}
	loopFlows := []bson.D{flow("f1", "start", "a", 1, 3, false), flow("f2", "a", "loop", 1, 3, false), flow("fi", "inner", "inner2", 1, 3, false)}

	twoIn := append(line(), obj("c", "Microflows$ActionActivity", 250, 350))
	twoInFlows := append(lineFlows(), flow("f4", "c", "b", 0, 2, false))

	withHandler := append(line(), obj("handler", "Microflows$ActionActivity", 250, 350))
	handlerFlows := append(lineFlows(), flow("err", "a", "handler", 2, 0, true))

	cases := []struct {
		name  string
		objs  []bson.D
		flows []bson.D
		op    func(m *Mutator) error
		want  string
	}{
		// Inside a loop body the splice puts plain activities only
		// (ako/mxcli#1693); a decision changes the body's shape.
		{"insert a decision inside a loop", withLoop, loopFlows,
			func(m *Mutator) error { return m.InsertAfter(model.ID(uid("inner")), oneDecision()) }, "inside a loop body"},
		{"insert before a join", twoIn, twoInFlows,
			func(m *Mutator) error { return m.InsertBefore(model.ID(uid("b")), oneActivity()) }, "2 flows enter"},
		{"insert after the end", line(), lineFlows(),
			func(m *Mutator) error { return m.InsertAfter(model.ID(uid("end")), oneActivity()) }, "ends the flow"},
		{"drop an activity with an error handler", withHandler, handlerFlows,
			func(m *Mutator) error { return m.Drop(model.ID(uid("a"))) }, "error handler"},
		{"replace an activity with an error handler", withHandler, handlerFlows,
			func(m *Mutator) error { return m.Replace(model.ID(uid("a")), oneActivity()) }, "error handler"},
		{"drop the end event", line(), lineFlows(),
			func(m *Mutator) error { return m.Drop(model.ID(uid("end"))) }, "cannot drop"},
		{"an empty fragment", line(), lineFlows(),
			func(m *Mutator) error { return m.InsertAfter(model.ID(uid("a")), &backend.MicroflowFragment{}) }, "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newMutator(t, unit(tc.objs, tc.flows))
			before, _ := bson.Marshal(m.doc)
			err := tc.op(m)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
			after, _ := bson.Marshal(m.doc)
			if !bytes.Equal(before, after) {
				t.Error("a refused operation changed the document")
			}
			// The in-loop refusal advises the alter that makes any change of
			// a body: replace the loop's body (ako/mxcli#1693).
			if tc.want == "inside a loop body" {
				if !strings.Contains(err.Error(), "replace the loop's body") || strings.Contains(err.Error(), "create or modify") {
					t.Errorf("want advice to replace the loop's body, not create or modify: %v", err)
				}
			}
		})
	}
}

// CLAUDE.md rule 1: an element is never removed while anything still points
// at it. A pointer the splice does not know about (here, a made-up property
// on another object) is found by value, and the write is refused.
func TestSplice_DropRefusesADanglingReference(t *testing.T) {
	objs := line()
	objs[3] = append(objs[3], bson.E{Key: "SomePointer", Value: bin("a")})
	m, deps := newMutator(t, unit(objs, lineFlows()))
	if err := m.Drop(model.ID(uid("a"))); err != nil {
		t.Fatalf("drop: %v", err)
	}
	err := m.Save()
	if err == nil || !strings.Contains(err.Error(), "still points at a removed element") {
		t.Fatalf("want the dangling-reference refusal, got %v", err)
	}
	if deps.saved != nil {
		t.Error("a unit with a dangling reference was saved")
	}
}

// Control for the test above: the same drop without the stray pointer saves.
func TestSplice_DropJoinsTheFlows(t *testing.T) {
	m, deps := newMutator(t, unit(line(), lineFlows()))
	if err := m.Drop(model.ID(uid("a"))); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if bytes.Contains(deps.saved, types.UUIDToBlob(uid("a"))) {
		t.Error("the dropped activity's $ID is still in the unit")
	}
	g := m.graph()
	for _, f := range g.flows {
		if f.id == uid("f1") && f.dest != uid("b") {
			t.Errorf("the flow into the dropped activity now enters %s, want b", f.dest)
		}
		if f.id == uid("f2") {
			t.Error("the flow out of the dropped activity is still there")
		}
	}
}

// A loop's body flows are stored in the unit's Flows list, not in the loop.
// Dropping or replacing the loop takes them with it; left behind, they point at
// the removed body objects and Save refuses the unit (a Studio Pro-authored
// loop with two body activities, ACT_ConflictedWorkflowHelper_ApplyJumpTo in
// TestApp, hit exactly that).
func TestSplice_DropOrReplaceALoopTakesItsBodyFlows(t *testing.T) {
	build := func() ([]bson.D, []bson.D) {
		loop := obj("loop", "Microflows$LoopedActivity", 420, 200)
		loop = append(loop, bson.E{Key: "ObjectCollection", Value: bson.D{
			{Key: "$ID", Value: bin("loopoc")},
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A{int32(3), obj("inner", "Microflows$ActionActivity", 100, 60), obj("inner2", "Microflows$ActionActivity", 260, 60)}},
		}})
		objs := []bson.D{
			obj("start", "Microflows$StartEvent", 100, 200),
			obj("a", "Microflows$ActionActivity", 250, 200),
			loop,
			obj("end", "Microflows$EndEvent", 700, 200),
		}
		flows := []bson.D{
			flow("f1", "start", "a", 1, 3, false),
			flow("f2", "a", "loop", 1, 3, false),
			flow("fi", "inner", "inner2", 1, 3, false),
			flow("f3", "loop", "end", 1, 3, false),
		}
		return objs, flows
	}
	ops := map[string]func(m *Mutator) error{
		"drop":    func(m *Mutator) error { return m.Drop(model.ID(uid("loop"))) },
		"replace": func(m *Mutator) error { return m.Replace(model.ID(uid("loop")), oneActivity()) },
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			objs, flows := build()
			m, deps := newMutator(t, unit(objs, flows))
			if err := op(m); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if err := m.Save(); err != nil {
				t.Fatalf("save: %v", err)
			}
			for _, gone := range []string{"loop", "inner", "inner2", "fi"} {
				if bytes.Contains(deps.saved, types.UUIDToBlob(uid(gone))) {
					t.Errorf("%s is still in the unit", gone)
				}
			}
		})
	}
}

// A changed return value is set on the stored end event in place (#805): the
// value changes and nothing else in the unit does. Only an end event takes
// one.
func TestSplice_SetReturnValueEditsTheEndEventOnly(t *testing.T) {
	objs := line()
	objs[3] = append(objs[3], bson.E{Key: "ReturnValue", Value: "7"})
	m, deps := newMutator(t, unit(objs, lineFlows()))
	if err := m.SetReturnValue(model.ID(uid("end")), "8"); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	want := line()
	want[3] = append(want[3], bson.E{Key: "ReturnValue", Value: "8"})
	raw, _ := bson.Marshal(unit(want, lineFlows()))
	if !bytes.Equal(deps.saved, raw) {
		t.Error("setting the return value changed more than the end event's ReturnValue")
	}

	if err := m.SetReturnValue(model.ID(uid("a")), "8"); err == nil || !strings.Contains(err.Error(), "only an end event") {
		t.Errorf("want a refusal for an activity, got %v", err)
	}
}

func annotationFlow(name, from, to string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bin(name)},
		{Key: "$Type", Value: "Microflows$AnnotationFlow"},
		{Key: "DestinationPointer", Value: bin(to)},
		{Key: "OriginPointer", Value: bin(from)},
	}
}

// ako/mxcli#859 (rehearsal M3): create or modify re-annotating an activity
// takes the notes attached to it out before the replace (RemoveNotes), instead
// of the replace moving them onto the fragment's entry, where the fragment's
// own notes would make two of each. A note another activity shares is refused:
// taking it out would take it from that activity too. Without RemoveNotes the
// stored notes move onto the entry as before (the control).
func TestSplice_RemoveNotesBeforeAReplace(t *testing.T) {
	build := func(shared bool) bson.D {
		objs := append(line(), obj("note", "Microflows$Annotation", 250, 330))
		flows := append(lineFlows(), annotationFlow("af", "note", "a"))
		if shared {
			flows = append(flows, annotationFlow("af2", "note", "b"))
		}
		return unit(objs, flows)
	}
	t.Run("the notes go, the replacement has none of them", func(t *testing.T) {
		m, deps := newMutator(t, build(false))
		if err := m.RemoveNotes(model.ID(uid("a"))); err != nil {
			t.Fatalf("remove notes: %v", err)
		}
		if err := m.Replace(model.ID(uid("a")), oneActivity()); err != nil {
			t.Fatalf("replace: %v", err)
		}
		if err := m.Save(); err != nil {
			t.Fatalf("save: %v", err)
		}
		for _, gone := range []string{"a", "note", "af"} {
			if bytes.Contains(deps.saved, types.UUIDToBlob(uid(gone))) {
				t.Errorf("%s is still in the unit", gone)
			}
		}
	})
	t.Run("control: without it they move onto the entry", func(t *testing.T) {
		m, deps := newMutator(t, build(false))
		frag := oneActivity()
		if err := m.Replace(model.ID(uid("a")), frag); err != nil {
			t.Fatalf("replace: %v", err)
		}
		if err := m.Save(); err != nil {
			t.Fatalf("save: %v", err)
		}
		if !bytes.Contains(deps.saved, types.UUIDToBlob(uid("note"))) {
			t.Error("the stored note was taken out")
		}
		for _, f := range m.graph().flows {
			if f.id == uid("af") && f.dest != string(frag.Entry) {
				t.Errorf("the note's line points at %s, want the fragment's entry", f.dest)
			}
		}
	})
	t.Run("a shared note is refused", func(t *testing.T) {
		m, _ := newMutator(t, build(true))
		err := m.RemoveNotes(model.ID(uid("a")))
		if err == nil || !strings.Contains(err.Error(), "also attached to") {
			t.Fatalf("replace of an activity whose note another shares: got %v, want a refusal", err)
		}
	})
}

// An activity inside a loop body is replaced by one activity in place
// (ako/mxcli#1106): the new one takes the stored one's slot in the loop's
// collection, its position and its size (a Studio Pro activity may have been
// resized), and the body flow through it now runs through the new one. The
// last body activity has no flow out. Nothing else in the unit changes.
func TestSplice_ReplaceInsideALoopIsInPlace(t *testing.T) {
	build := func() ([]bson.D, []bson.D) {
		inner := obj("inner", "Microflows$ActionActivity", 100, 60)
		dSet(inner, "Size", "200;60")
		loop := obj("loop", "Microflows$LoopedActivity", 420, 200)
		loop = append(loop, bson.E{Key: "ObjectCollection", Value: bson.D{
			{Key: "$ID", Value: bin("loopoc")},
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A{int32(3), inner, obj("inner2", "Microflows$ActionActivity", 260, 60)}},
		}})
		return []bson.D{obj("start", "Microflows$StartEvent", 100, 200), obj("a", "Microflows$ActionActivity", 250, 200), loop},
			[]bson.D{flow("f1", "start", "a", 1, 3, false), flow("f2", "a", "loop", 1, 3, false), flow("fi", "inner", "inner2", 1, 3, false)}
	}
	for _, tc := range []struct{ target, pos, size string }{
		{"inner", "100;60", "200;60"},
		{"inner2", "260;60", "120;60"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			objs, flows := build()
			m, deps := newMutator(t, unit(objs, flows))
			frag := oneActivity()
			if err := m.Replace(model.ID(uid(tc.target)), frag); err != nil {
				t.Fatal(err)
			}
			if err := m.Save(); err != nil {
				t.Fatalf("save: %v", err)
			}
			if bytes.Contains(deps.saved, types.UUIDToBlob(uid(tc.target))) {
				t.Errorf("%s is still in the unit", tc.target)
			}
			// What the replace should leave: the stored unit with the
			// target's document swapped, and the body flow repointed.
			newID := bson.Binary{Data: types.UUIDToBlob(string(frag.Entry))}
			wantObjs, wantFlows := build()
			oc := dDoc(wantObjs[2], "ObjectCollection")
			body := arrayElements(dGet(oc, "Objects"))
			for i, el := range body {
				if d := el.(bson.D); binaryID(dGet(d, "$ID")) == uid(tc.target) {
					body[i] = bson.D{
						{Key: "$ID", Value: newID},
						{Key: "$Type", Value: "Microflows$ActionActivity"},
						{Key: "RelativeMiddlePoint", Value: tc.pos},
						{Key: "Size", Value: tc.size},
					}
				}
			}
			setArray(oc, "Objects", body)
			key := "OriginPointer"
			if tc.target == "inner2" {
				key = "DestinationPointer"
			}
			dSet(wantFlows[2], key, newID)
			raw, _ := bson.Marshal(unit(wantObjs, wantFlows))
			if !bytes.Equal(deps.saved, raw) {
				var got bson.D
				_ = bson.Unmarshal(deps.saved, &got)
				t.Errorf("the replace changed more than the activity's document and the flow through it:\n got %v\nwant %v", got, unit(wantObjs, wantFlows))
			}
		})
	}

	// A decision for an activity would change the body's shape: refused.
	t.Run("a decision for an activity refused", func(t *testing.T) {
		objs, flows := build()
		m, _ := newMutator(t, unit(objs, flows))
		err := m.Replace(model.ID(uid("inner")), oneDecision())
		if err == nil || !strings.Contains(err.Error(), "replace the loop's body") {
			t.Fatalf("want the in-loop refusal, got %v", err)
		}
	})
}
