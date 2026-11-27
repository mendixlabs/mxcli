// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// paired is a SetCaseValues edit whose declared branches are the stored ones
// at the same places.
func paired(stored, declared [][]string) []backend.CaseBranchEdit {
	out := make([]backend.CaseBranchEdit, len(declared))
	for i := range declared {
		out[i] = backend.CaseBranchEdit{Values: declared[i], Stored: stored[i]}
	}
	return out
}

// ako/mxcli#2618: a `case` whose values move between its `when` branches, or
// gain a new one, keeps every node: only which branch a value's flow enters
// changes. A branch that ends up with several values enters through a merge
// (an activity takes one incoming flow, CE0709), and a branch left with one
// loses the merge that only joined its values.
//
// caseUnit is split -(A)-> a1; split -(B)-> bm, -(D)-> bm, bm -> b1;
// split -(C)-> c1; a1, b1, c1 -> merge -> end.
func caseUnit() bson.D {
	objs := []bson.D{
		obj("start", "Microflows$StartEvent", 100, 200),
		obj("split", "Microflows$ExclusiveSplit", 250, 200),
		obj("a1", "Microflows$ActionActivity", 450, 100),
		obj("bm", "Microflows$ExclusiveMerge", 380, 200),
		obj("b1", "Microflows$ActionActivity", 450, 200),
		obj("c1", "Microflows$ActionActivity", 450, 300),
		obj("merge", "Microflows$ExclusiveMerge", 600, 200),
		obj("end", "Microflows$EndEvent", 750, 200),
	}
	flows := []bson.D{
		flow("fs", "start", "split", 1, 3, false),
		caseFlow("fA", "split", "a1", 0, 3, "A"),
		caseFlow("fB", "split", "bm", 1, 3, "B"),
		caseFlow("fD", "split", "bm", 1, 3, "D"),
		flow("fbm", "bm", "b1", 1, 3, false),
		caseFlow("fC", "split", "c1", 2, 3, "C"),
		flow("fa", "a1", "merge", 1, 0, false),
		flow("fb", "b1", "merge", 1, 3, false),
		flow("fc", "c1", "merge", 1, 2, false),
		flow("fm", "merge", "end", 1, 3, false),
	}
	return unit(objs, flows)
}

func TestSetCaseValues_MovesAValueBetweenBranches_2618(t *testing.T) {
	m, deps := newMutator(t, caseUnit())
	stored := [][]string{{"A"}, {"B", "D"}, {"C"}}
	declared := [][]string{{"A", "D"}, {"B"}, {"C"}}
	if err := m.SetCaseValues(model.ID(uid("split")), paired(stored, declared), nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	pos, _ := geometry(t, deps.saved)
	if _, ok := pos[uid("bm")]; ok {
		t.Error("the merge that joined B and D is still there with one value left")
	}
	for _, kept := range []string{"split", "a1", "b1", "c1", "merge", "end"} {
		if _, ok := pos[uid(kept)]; !ok {
			t.Errorf("%s was taken out", kept)
		}
	}
	e := edges(t, deps.saved)
	if !e[[2]string{uid("split"), uid("b1")}] {
		t.Error("B does not enter b1 directly")
	}
	if e[[2]string{uid("split"), uid("a1")}] {
		t.Error("A and D enter a1 directly: an activity takes one incoming flow (CE0709)")
	}
	// A and D enter a new merge in front of a1.
	var newMerge string
	for id := range pos {
		if e[[2]string{uid("split"), id}] && e[[2]string{id, uid("a1")}] {
			newMerge = id
		}
	}
	if newMerge == "" {
		t.Fatalf("no merge in front of a1; edges %v", e)
	}
	g := graphOf(t, deps.saved)
	if n := len(g.incoming(newMerge)); n != 2 {
		t.Errorf("%d flows enter the new merge, want A's and D's", n)
	}
	// D's flow is the stored one, re-pointed: its $ID is kept.
	if f := flowByID(g, uid("fD")); f == nil || f.dest != newMerge {
		t.Errorf("D's stored flow was not re-pointed to the new merge: %+v", f)
	}
}

// The control: the same values on the same branches change nothing.
func TestSetCaseValues_SameValuesChangeNothing_2618(t *testing.T) {
	m, deps := newMutator(t, caseUnit())
	values := [][]string{{"A"}, {"B", "D"}, {"C"}}
	if err := m.SetCaseValues(model.ID(uid("split")), paired(values, values), nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	before, _ := bson.Marshal(caseUnit())
	if deps.saved != nil && string(deps.saved) != string(before) {
		e := edges(t, deps.saved)
		if !e[[2]string{uid("split"), uid("bm")}] || !e[[2]string{uid("split"), uid("a1")}] {
			t.Errorf("unchanged values changed the flow: %v", e)
		}
	}
}

// A value no branch keeps loses its flow; a value on two branches is refused.
func TestSetCaseValues_DropAndRefuse_2618(t *testing.T) {
	m, deps := newMutator(t, caseUnit())
	if err := m.SetCaseValues(model.ID(uid("split")), paired([][]string{{"A"}, {"B", "D"}, {"C"}}, [][]string{{"A"}, {"B"}, {"C"}}), nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if flowByID(graphOf(t, deps.saved), uid("fD")) != nil {
		t.Error("D, on no branch any more, kept its flow")
	}
	m2, _ := newMutator(t, caseUnit())
	if err := m2.SetCaseValues(model.ID(uid("split")), paired([][]string{{"A"}, {"B", "D"}, {"C"}}, [][]string{{"A", "D"}, {"B", "D"}, {"C"}}), nil); err == nil {
		t.Error("a value on two branches was accepted")
	}
}

func graphOf(t *testing.T, saved []byte) *graph {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(saved, &d); err != nil {
		t.Fatal(err)
	}
	m, _ := New(d, "unit", &fakeDeps{})
	return m.graph()
}

func flowByID(g *graph, id string) *flowRef {
	for i := range g.flows {
		if g.flows[i].id == id {
			return &g.flows[i]
		}
	}
	return nil
}

// The issue's case: B and D join A's branch and B's own `when` is taken out —
// its activity and its merge go, and A's branch gets a merge for three values.
func TestSetCaseValues_TakesABranchOut_2618(t *testing.T) {
	m, deps := newMutator(t, caseUnit())
	branches := []backend.CaseBranchEdit{
		{Values: []string{"A", "B", "D"}, Stored: []string{"A"}},
		{Values: []string{"C"}, Stored: []string{"C"}},
	}
	if err := m.SetCaseValues(model.ID(uid("split")), branches, [][]string{{"B", "D"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	pos, _ := geometry(t, deps.saved)
	for _, gone := range []string{"b1", "bm"} {
		if _, ok := pos[uid(gone)]; ok {
			t.Errorf("%s, on the branch taken out, is still there", gone)
		}
	}
	g := graphOf(t, deps.saved)
	if n := len(g.incoming(uid("merge"))); n != 2 {
		t.Errorf("%d flows enter the case's merge, want a1's and c1's", n)
	}
	if f := flowByID(g, uid("fB")); f == nil || g.nodes[f.dest] == nil || g.nodes[f.dest].typ != "Microflows$ExclusiveMerge" {
		t.Errorf("B's stored flow does not enter the merge in front of a1: %+v", f)
	}
}

// A new `when` with a body gets a path of its own into the case's merge.
func TestSetCaseValues_AddsABranch_2618(t *testing.T) {
	m, deps := newMutator(t, caseUnit())
	body := &backend.MicroflowFragment{
		Objects: []microflows.MicroflowObject{&microflows.ActionActivity{BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{BaseElement: model.BaseElement{ID: model.ID(uid("e1"))}}}}},
		Entry: model.ID(uid("e1")), Exit: model.ID(uid("e1")),
	}
	branches := append(paired([][]string{{"A"}, {"B", "D"}, {"C"}}, [][]string{{"A"}, {"B", "D"}, {"C"}}),
		backend.CaseBranchEdit{Values: []string{"E"}, Body: body})
	if err := m.SetCaseValues(model.ID(uid("split")), branches, nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	e := edges(t, deps.saved)
	if !e[[2]string{uid("split"), uid("e1")}] || !e[[2]string{uid("e1"), uid("merge")}] {
		t.Errorf("want split -> e1 -> merge for the new branch, got %v", e)
	}
}
