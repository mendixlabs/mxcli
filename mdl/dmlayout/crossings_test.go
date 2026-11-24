// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// figure builds a model from placed entities and associations given as
// "from>to" with explicit anchors, so the geometry is fixed by the test.
type figAssoc struct {
	name, from, to string
	pa, ca         model.Point
}

func figure(ents []*domainmodel.Entity, assocs []figAssoc) (*domainmodel.DomainModel, map[model.ID]model.Point, map[model.ID]AnchorPair) {
	dm := &domainmodel.DomainModel{Entities: ents}
	pos := map[model.ID]model.Point{}
	for _, e := range ents {
		pos[e.ID] = e.Location
	}
	anchors := map[model.ID]AnchorPair{}
	for _, f := range assocs {
		a := named(f.name, f.from, f.to)
		dm.Associations = append(dm.Associations, a)
		anchors[a.ID] = AnchorPair{Parent: f.pa, Child: f.ca}
	}
	return dm, pos, anchors
}

var (
	rightMid = model.Point{X: 100, Y: 50}
	leftMid  = model.Point{X: 0, Y: 50}
)

// Two lines in an X cross once; the same lines parallel do not.
func TestCrossings_XCrossesOnceParallelNever(t *testing.T) {
	ents := []*domainmodel.Entity{
		placed("A", 100, 100, 1), placed("C", 100, 400, 1),
		placed("B", 600, 400, 1), placed("D", 600, 100, 1),
	}
	dm, pos, anc := figure(ents, []figAssoc{
		{"A_B", "A", "B", rightMid, leftMid},
		{"C_D", "C", "D", rightMid, leftMid},
	})
	if lines, boxes := Crossings(dm, pos, anc, nil); lines != 1 || boxes != 0 {
		t.Fatalf("X: lines %d boxes %d, want 1 and 0", lines, boxes)
	}
	dm, pos, anc = figure(ents, []figAssoc{
		{"A_D", "A", "D", rightMid, leftMid},
		{"C_B", "C", "B", rightMid, leftMid},
	})
	if lines, boxes := Crossings(dm, pos, anc, nil); lines != 0 || boxes != 0 {
		t.Fatalf("parallel: lines %d boxes %d, want 0 and 0", lines, boxes)
	}
}

// Lines that share an entity meet at its box: that is not a crossing.
func TestCrossings_SharedEndIsNotACrossing(t *testing.T) {
	ents := []*domainmodel.Entity{placed("Hub", 100, 100, 6), placed("S1", 600, 100, 1), placed("S2", 600, 400, 1)}
	dm, pos, anc := figure(ents, []figAssoc{
		{"S1_Hub", "S1", "Hub", leftMid, rightMid},
		{"S2_Hub", "S2", "Hub", leftMid, rightMid},
	})
	if lines, _ := Crossings(dm, pos, anc, nil); lines != 0 {
		t.Fatalf("lines %d, want 0: both end at Hub", lines)
	}
}

// A straight line through a box in between; and a cross-module line, drawn
// from the entity's left side to x = 0, through a box to its left.
func TestCrossings_LineThroughABox(t *testing.T) {
	ents := []*domainmodel.Entity{placed("A", 100, 100, 1), placed("M", 400, 100, 1), placed("B", 700, 100, 1)}
	dm, pos, anc := figure(ents, []figAssoc{{"B_A", "B", "A", leftMid, rightMid}})
	if _, boxes := Crossings(dm, pos, anc, nil); boxes != 1 {
		t.Fatalf("association: boxes %d, want 1 (through M)", boxes)
	}
	dm, pos, anc = figure(ents, nil)
	cross := CrossLines{ents[2].ID: {"B_Account"}}
	if _, boxes := Crossings(dm, pos, anc, cross); boxes != 2 {
		t.Fatalf("cross-module: boxes %d, want 2 (through M and A)", boxes)
	}
}

// StoredAnchors reads what is stored, and Studio Pro's default where nothing
// is: the FROM end at 0;50, the TO end at 100;50.
func TestStoredAnchors(t *testing.T) {
	stored := named("Stored", "A", "B")
	stored.ParentConnection = &model.Point{X: 50, Y: 0}
	stored.ChildConnection = &model.Point{X: 50, Y: 100}
	unset := named("Unset", "A", "B")
	got := StoredAnchors(&domainmodel.DomainModel{Associations: []*domainmodel.Association{stored, unset}})
	if got[stored.ID] != (AnchorPair{Parent: model.Point{X: 50, Y: 0}, Child: model.Point{X: 50, Y: 100}}) {
		t.Errorf("stored: %v", got[stored.ID])
	}
	if got[unset.ID] != (AnchorPair{Parent: model.Point{X: 0, Y: 50}, Child: model.Point{X: 100, Y: 50}}) {
		t.Errorf("unset: %v, want the 0;50 / 100;50 default", got[unset.ID])
	}
}
