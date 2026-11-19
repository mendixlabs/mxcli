// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// The reported symptom (mendixlabs/mxcli#1479): "entities with many attributes
// overlap the one below". A grid row is 280px; an entity with ~17+ attributes is
// taller than that, so the entity in the next row of its column is drawn on top
// of its lower half.

func attrs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("Attribute%02d", i)
	}
	return out
}

func overlapping(dm *domainmodel.DomainModel) []string {
	var hits []string
	for i, a := range dm.Entities {
		for _, b := range dm.Entities[i+1:] {
			if EntityBox(a).TooClose(EntityBox(b)) {
				hits = append(hits, a.Name+"/"+b.Name)
			}
		}
		for _, an := range dm.Annotations {
			if EntityBox(a).TooClose(AnnotationBox(an)) {
				hits = append(hits, a.Name+"/annotation")
			}
		}
	}
	return hits
}

// sevenEntities creates seven entities the way successive CREATE ENTITY
// statements do, the first one tall: slot 6 is directly beneath it.
func sevenEntities(place func(dm *domainmodel.DomainModel, e *domainmodel.Entity) model.Point) *domainmodel.DomainModel {
	dm := &domainmodel.DomainModel{}
	for i := 0; i < 7; i++ {
		var e *domainmodel.Entity
		if i == 0 {
			e = ent("Wide", attrs(20)...)
		} else {
			e = ent(fmt.Sprintf("E%d", i), "Name")
		}
		e.Location = place(dm, e)
		dm.Entities = append(dm.Entities, e)
	}
	return dm
}

func TestPlace_TallEntityDoesNotOverlapTheOneBelow(t *testing.T) {
	// CONTROL: the grid alone reproduces the report — slot 6 sits inside the
	// tall entity's box.
	grid := sevenEntities(func(dm *domainmodel.DomainModel, _ *domainmodel.Entity) model.Point {
		return GridSlot(len(dm.Entities))
	})
	if len(overlapping(grid)) == 0 {
		t.Fatal("control: GridSlot placement did not overlap — the test no longer reproduces #1479")
	}

	placed := sevenEntities(Place)
	if hits := overlapping(placed); len(hits) > 0 {
		t.Errorf("Place left overlapping entities: %v", hits)
	}
}

// The common case must not change: small entities land where GridSlot put them,
// so existing scripts produce the same diagram they always did.
func TestPlace_SmallEntitiesKeepTheirGridSlots(t *testing.T) {
	dm := &domainmodel.DomainModel{}
	for i := 0; i < 20; i++ {
		e := ent(fmt.Sprintf("E%02d", i), "Name", "Code")
		e.Location = Place(dm, e)
		if e.Location != GridSlot(i) {
			t.Errorf("entity %d placed at %+v, want grid slot %+v", i, e.Location, GridSlot(i))
		}
		dm.Entities = append(dm.Entities, e)
	}
}

// Contract and OData imports place into a module that already has entities, and
// the next slot may be occupied by a hand-placed one or an annotation.
func TestPlace_SkipsOccupiedSlots(t *testing.T) {
	existing := ent("Existing", "Name")
	existing.Location = GridSlot(1) // where entity #1 would go
	note := &domainmodel.Annotation{Caption: "Read me", Location: GridSlot(2), Width: 200}
	dm := &domainmodel.DomainModel{
		Entities:    []*domainmodel.Entity{existing},
		Annotations: []*domainmodel.Annotation{note},
	}
	e := ent("New", "Name")
	e.Location = Place(dm, e)
	dm.Entities = append(dm.Entities, e)
	if hits := overlapping(dm); len(hits) > 0 {
		t.Errorf("Place collided: %v (at %+v)", hits, e.Location)
	}
	if e.Location != GridSlot(3) {
		t.Errorf("placed at %+v, want the first free slot %+v", e.Location, GridSlot(3))
	}
}

// Plan's positions are top-left corners: the first entity of the first layer
// sits exactly on the origin, not half a box past it.
func TestPlan_PositionsAreTopLeft(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities:     []*domainmodel.Entity{ent("A", attrs(10)...), ent("B", "x")},
		Associations: []*domainmodel.Association{assoc("B_A", "B", "A")},
	}
	pos := Plan(dm)
	if got := pos["id-A"]; got != (model.Point{X: OriginX, Y: OriginY}) {
		t.Errorf("A at %+v, want the origin (%d, %d) — positions are still box centres", got, OriginX, OriginY)
	}
}

// A tall isolated helper in the band must not run into the row beneath it.
func TestPlan_IsolatedBandHandlesTallEntities(t *testing.T) {
	dm := &domainmodel.DomainModel{}
	for i := 0; i < 8; i++ {
		n := 2
		if i == 0 {
			n = 30
		}
		dm.Entities = append(dm.Entities, ent(fmt.Sprintf("NP%02d", i), attrs(n)...))
	}
	for id, p := range Plan(dm) {
		for _, e := range dm.Entities {
			if e.ID == id {
				e.Location = p
			}
		}
	}
	if hits := overlapping(dm); len(hits) > 0 {
		t.Errorf("Plan overlapped isolated entities: %v", hits)
	}
}

func applied(dm *domainmodel.DomainModel, moves map[model.ID]model.Point) {
	for _, e := range dm.Entities {
		if p, ok := moves[e.ID]; ok {
			e.Location = p
		}
	}
}

// The repair the issue asks for: separate the colliding entities without
// rearranging the rest.
func TestResolveOverlaps_MovesOnlyTheColliders(t *testing.T) {
	tall := ent("Tall", attrs(20)...)
	tall.Location = model.Point{X: 100, Y: 100}
	under := ent("Under", "Name") // inside Tall's lower half
	under.Location = model.Point{X: 120, Y: 300}
	apart := ent("Apart", "Name") // hand-placed far away
	apart.Location = model.Point{X: 900, Y: 137}
	note := &domainmodel.Annotation{Caption: "keep clear", Location: model.Point{X: 100, Y: 500}, Width: 200}
	dm := &domainmodel.DomainModel{
		Entities:    []*domainmodel.Entity{under, apart, tall},
		Annotations: []*domainmodel.Annotation{note},
	}
	if len(overlapping(dm)) == 0 {
		t.Fatal("control: the fixture does not overlap")
	}

	moves := ResolveOverlaps(dm)
	if len(moves) != 1 {
		t.Fatalf("moved %d entities (%v), want only Under", len(moves), moves)
	}
	if _, ok := moves[under.ID]; !ok {
		t.Fatalf("moved %v, want Under — the first entity of a collision stays put", moves)
	}
	applied(dm, moves)
	if hits := overlapping(dm); len(hits) > 0 {
		t.Errorf("still overlapping after repair: %v", hits)
	}
	if tall.Location != (model.Point{X: 100, Y: 100}) || apart.Location != (model.Point{X: 900, Y: 137}) {
		t.Errorf("a non-colliding entity moved: Tall %+v, Apart %+v", tall.Location, apart.Location)
	}

	// Running it again changes nothing: the repaired model is a fixed point.
	if again := ResolveOverlaps(dm); len(again) != 0 {
		t.Errorf("second run moved %v", again)
	}
}

// Many entities stacked on one point (what an import into a single slot would
// leave) all get separated, deterministically.
func TestResolveOverlaps_StackIsSeparatedDeterministically(t *testing.T) {
	build := func() *domainmodel.DomainModel {
		dm := &domainmodel.DomainModel{}
		for i := 0; i < 12; i++ {
			e := ent(fmt.Sprintf("S%02d", i), attrs(i)...)
			e.Location = model.Point{X: 100, Y: 100}
			dm.Entities = append(dm.Entities, e)
		}
		return dm
	}
	want := fmt.Sprint(ResolveOverlaps(build()))
	for i := 0; i < 20; i++ {
		if got := fmt.Sprint(ResolveOverlaps(build())); got != want {
			t.Fatalf("run %d produced different moves", i)
		}
	}
	dm := build()
	applied(dm, ResolveOverlaps(dm))
	if hits := overlapping(dm); len(hits) > 0 {
		t.Errorf("still overlapping: %v", hits)
	}
}
