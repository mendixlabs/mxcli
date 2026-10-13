// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// assertNoOverlap fails for every pair of entities whose estimated boxes
// intersect when each sits at pos[id]. It is the checker TestPlan_NoOverlap
// uses, shared so the two layouts are held to the same rule.
func assertNoOverlap(t *testing.T, dm *domainmodel.DomainModel, pos map[model.ID]model.Point) {
	t.Helper()
	type rect struct {
		name           string
		x1, y1, x2, y2 int
	}
	var boxes []rect
	for _, e := range dm.Entities {
		p := pos[e.ID]
		w, h := boxSize(e)
		boxes = append(boxes, rect{e.Name, p.X - w/2, p.Y - h/2, p.X + w/2, p.Y + h/2})
	}
	for i := range boxes {
		for j := i + 1; j < len(boxes); j++ {
			a, b := boxes[i], boxes[j]
			if a.x1 < b.x2 && b.x1 < a.x2 && a.y1 < b.y2 && b.y1 < a.y2 {
				t.Errorf("%s and %s overlap: %+v vs %+v", a.name, b.name, a, b)
			}
		}
	}
}

func locations(dm *domainmodel.DomainModel) map[model.ID]model.Point {
	out := make(map[model.ID]model.Point, len(dm.Entities))
	for _, e := range dm.Entities {
		out[e.ID] = e.Location
	}
	return out
}

// withLocation is ent() with a position.
func withLocation(e *domainmodel.Entity, x, y int) *domainmodel.Entity {
	e.Location = model.Point{X: x, Y: y}
	return e
}

// attrs returns n attribute names.
func attrs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("A%d", i)
	}
	return out
}

// apply returns the positions after the plan's moves.
func apply(dm *domainmodel.DomainModel, plan OverlapPlan) map[model.ID]model.Point {
	pos := locations(dm)
	for _, m := range plan.Moves {
		pos[m.ID] = m.To
	}
	return pos
}

// withPositions returns a copy of dm whose entities sit at pos, so a second
// pass can be run over the outcome of the first.
func withPositions(dm *domainmodel.DomainModel, pos map[model.ID]model.Point) *domainmodel.DomainModel {
	out := &domainmodel.DomainModel{Annotations: dm.Annotations}
	for _, e := range dm.Entities {
		c := *e
		c.Location = pos[e.ID]
		out.Entities = append(out.Entities, &c)
	}
	return out
}

// gridModel is what CREATE ENTITY leaves behind: n entities on GridSlot, every
// third one tall. Boxes are centred on the grid points, so from about 30
// attributes (34+16*attrs high) a box reaches into the box one row down — the
// reported symptom, and the reason a fixed pitch cannot be the whole answer.
func gridModel(n int) *domainmodel.DomainModel {
	dm := &domainmodel.DomainModel{}
	for i := 0; i < n; i++ {
		na := 3
		if i%3 == 0 {
			na = 20 + (i*7)%31 // 20..50 attributes
		}
		p := GridSlot(i)
		dm.Entities = append(dm.Entities, withLocation(ent(fmt.Sprintf("E%02d", i), attrs(na)...), p.X, p.Y))
	}
	return dm
}

func movedIDs(plan OverlapPlan) map[model.ID]bool {
	out := map[model.ID]bool{}
	for _, m := range plan.Moves {
		out[m.ID] = true
	}
	return out
}

// A box is as tall as its attribute list, so two entities one grid row apart
// collide only when the upper one is long.
func TestResolveOverlaps_DetectsCollisionsByAttributeCount(t *testing.T) {
	short := &domainmodel.DomainModel{Entities: []*domainmodel.Entity{
		withLocation(ent("Upper", attrs(3)...), 100, 100),
		withLocation(ent("Lower", attrs(3)...), 100, 380),
	}}
	if plan := ResolveOverlaps(short); len(plan.Pairs) != 0 || len(plan.Moves) != 0 {
		t.Fatalf("two short entities one row apart collide: %+v", plan)
	}

	tall := &domainmodel.DomainModel{Entities: []*domainmodel.Entity{
		withLocation(ent("Upper", attrs(40)...), 100, 100), // h = 34+40*16 = 674
		withLocation(ent("Lower", attrs(3)...), 100, 380),
	}}
	plan := ResolveOverlaps(tall)
	if len(plan.Pairs) != 1 || plan.Pairs[0].Entity != "Lower" || plan.Pairs[0].With != "Upper" {
		t.Fatalf("want the one Upper/Lower pair, got %+v", plan.Pairs)
	}
	if len(plan.Moves) != 1 || plan.Moves[0].Name != "Lower" {
		t.Fatalf("want only Lower to move (Upper comes first), got %+v", plan.Moves)
	}
}

// Boxes that only touch do not collide: the rule is intersection, not nearness.
func TestResolveOverlaps_TouchingBoxesAreNotACollision(t *testing.T) {
	a := ent("A", attrs(3)...)
	b := ent("B", attrs(3)...)
	wa, ha := boxSize(a)
	withLocation(a, 100, 100)
	withLocation(b, 100+wa, 100) // exactly one box width to the right
	_ = ha
	dm := &domainmodel.DomainModel{Entities: []*domainmodel.Entity{a, b}}
	if plan := ResolveOverlaps(dm); len(plan.Pairs) != 0 {
		t.Errorf("touching boxes reported as a collision: %+v", plan.Pairs)
	}
	b.Location.X-- // one pixel closer
	if plan := ResolveOverlaps(dm); len(plan.Pairs) != 1 {
		t.Errorf("one pixel of overlap not found: %+v", plan.Pairs)
	}
}

// The point of --only-overlapping: an entity nothing collides with keeps its
// position to the pixel, however odd, and a collision moves only the entity that
// comes later in the model.
func TestResolveOverlaps_MovesOnlyCollidingEntities(t *testing.T) {
	dm := gridModel(14)
	// A hand-placed entity far from everything, and not on any grid.
	pinned := withLocation(ent("Pinned", attrs(4)...), 4321, 1234)
	dm.Entities = append(dm.Entities, pinned)

	before := locations(dm)
	plan := ResolveOverlaps(dm)
	if len(plan.Pairs) == 0 || len(plan.Moves) == 0 {
		t.Fatalf("fixture has no collisions to resolve: %+v", plan)
	}

	inPair := map[string]bool{}
	for _, p := range plan.Pairs {
		inPair[p.Entity] = true
		inPair[p.With] = true
	}
	moved := movedIDs(plan)
	for _, e := range dm.Entities {
		switch {
		case !inPair[e.Name] && moved[e.ID]:
			t.Errorf("%s collides with nothing but was moved", e.Name)
		case !moved[e.ID] && apply(dm, plan)[e.ID] != before[e.ID]:
			t.Errorf("%s was not moved but its position changed", e.Name)
		}
	}
	if moved[pinned.ID] {
		t.Error("the hand-placed entity was moved")
	}
	if len(plan.Moves) >= len(dm.Entities) {
		t.Errorf("%d of %d entities moved — that is a re-layout, not a repair", len(plan.Moves), len(dm.Entities))
	}
	// Every move leaves a position, and says where it came from.
	for _, m := range plan.Moves {
		if m.From != before[m.ID] || m.To == m.From {
			t.Errorf("bad move %+v (stored %+v)", m, before[m.ID])
		}
	}
}

// ResolveOverlaps plans; it must not write into the model it was handed.
func TestResolveOverlaps_DoesNotModifyTheModel(t *testing.T) {
	dm := gridModel(14)
	before := locations(dm)
	ResolveOverlaps(dm)
	for id, p := range locations(dm) {
		if before[id] != p {
			t.Fatalf("%s was moved in the model itself", id)
		}
	}
}

// After the moves no two boxes intersect — the same check TestPlan_NoOverlap
// applies to the full layout — and a mover keeps the package's gutter to every
// neighbour, so the result is not boxes an exact pixel apart.
func TestResolveOverlaps_LeavesNoOverlap(t *testing.T) {
	for _, n := range []int{14, 40, 90} {
		dm := gridModel(n)
		plan := ResolveOverlaps(dm)
		assertNoOverlap(t, dm, apply(dm, plan))

		pos := apply(dm, plan)
		moved := movedIDs(plan)
		for _, a := range dm.Entities {
			if !moved[a.ID] {
				continue
			}
			wa, ha := boxSize(a)
			for _, b := range dm.Entities {
				if a == b {
					continue
				}
				wb, hb := boxSize(b)
				pa, pb := pos[a.ID], pos[b.ID]
				gapX := abs(pa.X-pb.X) - (wa+wb)/2
				gapY := abs(pa.Y-pb.Y) - (ha+hb)/2
				if gapX < placementGutter-1 && gapY < placementGutter-1 {
					t.Errorf("n=%d: moved %s is within the gutter of %s (gap %d,%d)", n, a.Name, b.Name, gapX, gapY)
				}
			}
		}
	}
}

// A mover is put NEAR where it was, not sent to the end of the diagram.
func TestResolveOverlaps_MovesToTheNearestFreeSpot(t *testing.T) {
	a := withLocation(ent("A", attrs(3)...), 400, 300)
	b := withLocation(ent("B", attrs(3)...), 400, 320) // sits almost on top of A
	dm := &domainmodel.DomainModel{Entities: []*domainmodel.Entity{a, b}}
	plan := ResolveOverlaps(dm)
	if len(plan.Moves) != 1 || plan.Moves[0].Name != "B" {
		t.Fatalf("want B to move, got %+v", plan.Moves)
	}
	m := plan.Moves[0]
	if d := abs(m.To.X-m.From.X) + abs(m.To.Y-m.From.Y); d > 160 {
		t.Errorf("B moved %dpx (%+v -> %+v) to clear a box it overlaps by a few pixels", d, m.From, m.To)
	}
}

// Of two colliding entities the one earlier in the model stays: it is the older,
// likelier hand-placed one, and the rule needs no information beyond the model.
func TestResolveOverlaps_TheEarlierEntityStays(t *testing.T) {
	first := withLocation(ent("Zeta", attrs(3)...), 100, 100)
	second := withLocation(ent("Alpha", attrs(3)...), 110, 100)
	dm := &domainmodel.DomainModel{Entities: []*domainmodel.Entity{first, second}}
	plan := ResolveOverlaps(dm)
	if len(plan.Moves) != 1 || plan.Moves[0].Name != "Alpha" {
		t.Fatalf("model order decides, not name: got %+v", plan.Moves)
	}
}

// Several entities on one point: all but the first leave, and none lands on
// another mover.
func TestResolveOverlaps_StackedEntitiesAreSeparated(t *testing.T) {
	dm := &domainmodel.DomainModel{}
	for i := 0; i < 5; i++ {
		dm.Entities = append(dm.Entities, withLocation(ent(fmt.Sprintf("S%d", i), attrs(5)...), 500, 500))
	}
	plan := ResolveOverlaps(dm)
	if len(plan.Moves) != 4 {
		t.Fatalf("want 4 of 5 to move, got %d: %+v", len(plan.Moves), plan.Moves)
	}
	assertNoOverlap(t, dm, apply(dm, plan))
}

// Same model, same answer — Go randomises map iteration, so an unsorted walk
// anywhere in the search would show here.
func TestResolveOverlaps_IsDeterministic(t *testing.T) {
	want := fmt.Sprint(ResolveOverlaps(gridModel(40)))
	for i := 0; i < 20; i++ {
		if got := fmt.Sprint(ResolveOverlaps(gridModel(40))); got != want {
			t.Fatalf("run %d planned differently", i)
		}
	}
}

// A second pass over the result moves nothing and finds nothing: the command can
// sit in a build script.
func TestResolveOverlaps_IsIdempotent(t *testing.T) {
	for _, n := range []int{14, 40, 90} {
		dm := gridModel(n)
		first := ResolveOverlaps(dm)
		if len(first.Moves) == 0 {
			t.Fatalf("n=%d: nothing to resolve in the fixture", n)
		}
		again := ResolveOverlaps(withPositions(dm, apply(dm, first)))
		if len(again.Pairs) != 0 || len(again.Moves) != 0 {
			t.Errorf("n=%d: second pass found %d collisions and planned %d moves", n, len(again.Pairs), len(again.Moves))
		}
	}
}

// A note on the canvas is an obstacle: an entity under it moves, the note stays.
func TestResolveOverlaps_AnnotationIsAnObstacle(t *testing.T) {
	note := &domainmodel.Annotation{Caption: "Billing\nEverything about invoices", Location: model.Point{X: 300, Y: 300}, Width: 440}
	under := withLocation(ent("Under", attrs(3)...), 300, 300)
	clear := withLocation(ent("Clear", attrs(3)...), 3000, 3000)
	dm := &domainmodel.DomainModel{
		Entities:    []*domainmodel.Entity{under, clear},
		Annotations: []*domainmodel.Annotation{note},
	}
	plan := ResolveOverlaps(dm)
	if len(plan.Pairs) != 1 || !plan.Pairs[0].WithAnnotation || plan.Pairs[0].Entity != "Under" || plan.Pairs[0].With != "Billing" {
		t.Fatalf("want the one Under/Billing pair, got %+v", plan.Pairs)
	}
	if len(plan.Moves) != 1 || plan.Moves[0].Name != "Under" {
		t.Fatalf("want only Under to move, got %+v", plan.Moves)
	}
	again := ResolveOverlaps(withPositions(dm, apply(dm, plan)))
	if len(again.Pairs) != 0 {
		t.Errorf("Under still collides with the note after moving: %+v", again.Pairs)
	}
	if note.Location != (model.Point{X: 300, Y: 300}) {
		t.Error("the annotation was moved")
	}
}

// CONTROL: nothing to do on an empty, nil or already-clean model.
func TestResolveOverlaps_NothingToDo(t *testing.T) {
	for name, dm := range map[string]*domainmodel.DomainModel{
		"nil":   nil,
		"empty": {},
		"one":   {Entities: []*domainmodel.Entity{withLocation(ent("Only", "x"), 10, 10)}},
		"clean": {Entities: []*domainmodel.Entity{
			withLocation(ent("A", "x"), 100, 100),
			withLocation(ent("B", "x"), 1000, 100),
		}},
	} {
		if plan := ResolveOverlaps(dm); len(plan.Pairs) != 0 || len(plan.Moves) != 0 {
			t.Errorf("%s: %+v", name, plan)
		}
	}
}

// A mover is not pushed off the top or left of the canvas, where a generated
// model starts: the entity in the first grid column is the one that would be.
func TestResolveOverlaps_NeverPlacesBeforeTheOrigin(t *testing.T) {
	for _, n := range []int{14, 40, 90} {
		dm := gridModel(n)
		byID := map[model.ID]*domainmodel.Entity{}
		for _, e := range dm.Entities {
			byID[e.ID] = e
		}
		for _, m := range ResolveOverlaps(dm).Moves {
			w, h := boxSize(byID[m.ID])
			if m.To.X-w/2 < 0 || m.To.Y-h/2 < 0 {
				t.Errorf("n=%d: %s moved to %+v, whose box starts before the origin", n, m.Name, m.To)
			}
		}
	}
}
