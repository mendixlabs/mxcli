// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// The reported symptom: a generated domain model came out as one horizontal
// line. mxcli placed every entity at y=100, stepping x by 150 — so a 40-entity
// model was a 6,000px row, and since 150px is narrower than an entity box the
// boxes touched as well (ako/CapTrackV2, Mendix 11.13).

func ent(name string, attrs ...string) *domainmodel.Entity {
	e := &domainmodel.Entity{Name: name}
	e.ID = model.ID("id-" + name)
	for _, a := range attrs {
		e.Attributes = append(e.Attributes, &domainmodel.Attribute{Name: a})
	}
	return e
}

func assoc(name, from, to string) *domainmodel.Association {
	// ParentID is the FROM entity and ChildID the TO entity — Mendix's inverted
	// naming, per CLAUDE.md.
	return &domainmodel.Association{
		Name:     name,
		ParentID: model.ID("id-" + from),
		ChildID:  model.ID("id-" + to),
	}
}

// GridSlot must wrap. A default that only ever increments x is the bug.
func TestGridSlot_Wraps(t *testing.T) {
	first := GridSlot(0)
	if first.X != OriginX || first.Y != OriginY {
		t.Errorf("slot 0 = %+v, want the origin", first)
	}
	// The old default put entity 39 at x=100+39*150=6950, y=100.
	last := GridSlot(39)
	if last.Y == OriginY {
		t.Error("slot 39 is still on the first row — the grid does not wrap")
	}
	if last.X > OriginX+GridColumns*GridColumnPitch {
		t.Errorf("slot 39 x=%d is past the last column; the row never wrapped", last.X)
	}
}

// The width of a 40-entity model is the number the report was about.
func TestGridSlot_KeepsAFortyEntityModelOnScreen(t *testing.T) {
	widest := 0
	for n := 0; n < 40; n++ {
		if x := GridSlot(n).X; x > widest {
			widest = x
		}
	}
	if widest > 2000 {
		t.Errorf("40 entities span %dpx horizontally; the single-row default spanned 6950", widest)
	}
}

// Slot n must depend on nothing but n: an entity written by an earlier statement
// must not move when a later one is added, or every re-run rewrites the unit.
func TestGridSlot_IsStableAsEntitiesAreAdded(t *testing.T) {
	for n := 0; n < 50; n++ {
		a, b := GridSlot(n), GridSlot(n)
		if a != b {
			t.Fatalf("slot %d is not a function of n: %+v vs %+v", n, a, b)
		}
	}
}

// capTrack is the reported model's shape: six lookups nothing points out of, a
// chain four deep, and two unconnected helpers.
func capTrack() *domainmodel.DomainModel {
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{
			ent("Department", "Name", "Code"),
			ent("Region", "Code", "Name"),
			ent("PlanType", "Code", "Name"),
			ent("PlanningYear", "YearNr"),
			ent("EmploymentType", "Name"),
			ent("MovementReason", "Name"),
			ent("Team", "Name"),
			ent("GoalBucket", "Name", "Scope"),
			ent("PlanScope", "ESPreviousYear"),
			ent("ScopeMonth", "MonthNr", "Value"),
			ent("Employee", "Name", "BaseFTE"),
			ent("EmployeeMonth", "MonthNr", "FTE"),
			ent("Movement", "MovementType", "Status", "MonthNr"),
			ent("NP_NewEmployee", "Name"),
			ent("NP_UserDeptToggle", "Active"),
		},
		Associations: []*domainmodel.Association{
			assoc("Team_Department", "Team", "Department"),
			assoc("GoalBucket_Department", "GoalBucket", "Department"),
			assoc("GoalBucket_PlanningYear", "GoalBucket", "PlanningYear"),
			assoc("PlanScope_Department", "PlanScope", "Department"),
			assoc("PlanScope_Team", "PlanScope", "Team"),
			assoc("PlanScope_Region", "PlanScope", "Region"),
			assoc("PlanScope_PlanType", "PlanScope", "PlanType"),
			assoc("PlanScope_PlanningYear", "PlanScope", "PlanningYear"),
			assoc("ScopeMonth_PlanScope", "ScopeMonth", "PlanScope"),
			assoc("Employee_PlanScope", "Employee", "PlanScope"),
			assoc("Employee_EmploymentType", "Employee", "EmploymentType"),
			assoc("Employee_GoalBucket", "Employee", "GoalBucket"),
			assoc("EmployeeMonth_Employee", "EmployeeMonth", "Employee"),
			assoc("Movement_PlanScope", "Movement", "PlanScope"),
			assoc("Movement_Employee", "Movement", "Employee"),
			assoc("Movement_MovementReason", "Movement", "MovementReason"),
		},
	}
	return dm
}

// The point of the graph layout: an entity sits to the right of everything it
// references, so association lines run one way instead of across the diagram.
func TestPlan_ReferencedEntitiesSitLeftOfTheirReferrers(t *testing.T) {
	dm := capTrack()
	pos := Plan(dm)

	byName := map[string]model.Point{}
	for _, e := range dm.Entities {
		p, ok := pos[e.ID]
		if !ok {
			t.Fatalf("%s got no position", e.Name)
		}
		byName[e.Name] = p
	}

	for _, c := range []struct{ from, to string }{
		{"Team", "Department"},
		{"PlanScope", "Region"},
		{"ScopeMonth", "PlanScope"},
		{"Employee", "PlanScope"},
		{"EmployeeMonth", "Employee"},
		{"Movement", "Employee"},
	} {
		if byName[c.from].X <= byName[c.to].X {
			t.Errorf("%s (x=%d) should sit right of %s (x=%d) — it references it",
				c.from, byName[c.from].X, c.to, byName[c.to].X)
		}
	}
}

// The lookups nothing points out of share the leftmost column, and the deepest
// entity is well clear of it.
func TestPlan_LayersTheModel(t *testing.T) {
	dm := capTrack()
	pos := Plan(dm)
	byName := map[string]model.Point{}
	for _, e := range dm.Entities {
		byName[e.Name] = pos[e.ID]
	}

	lookups := []string{"Department", "Region", "PlanType", "PlanningYear", "EmploymentType", "MovementReason"}
	first := byName[lookups[0]].X
	for _, n := range lookups[1:] {
		if byName[n].X != first {
			t.Errorf("%s is at x=%d, not in the lookup column x=%d", n, byName[n].X, first)
		}
	}
	if byName["EmployeeMonth"].X <= byName["PlanScope"].X {
		t.Error("the deepest entity did not end up past the middle of the graph")
	}
}

// No two entities may overlap. This is the half the old default got wrong even
// ignoring the single row: a 150px step is narrower than a box.
func TestPlan_NoOverlap(t *testing.T) {
	dm := capTrack()
	assertNoOverlap(t, dm, Plan(dm))
}

// Location is the box's top-left corner, so a tall box stacked above a short one
// must end above it, not run into it. Centring each box on its position put the
// pair half a box too low and let them collide.
func TestPlan_TallEntityAboveShortOneDoesNotOverlap(t *testing.T) {
	// Layer members are ordered by name, so A_Tall is stacked above B_Short.
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{ent("A_Tall", attrs(40)...), ent("B_Short", "x"), ent("Target", "x")},
		Associations: []*domainmodel.Association{
			assoc("Tall_Target", "A_Tall", "Target"),
			assoc("Short_Target", "B_Short", "Target"),
		},
	}
	pos := Plan(dm)
	tall, short := pos["id-A_Tall"], pos["id-B_Short"]
	if tall.X != short.X || tall.Y >= short.Y {
		t.Fatalf("want A_Tall stacked above B_Short in one column, got %+v and %+v", tall, short)
	}
	_, h := boxSize(dm.Entities[0])
	if short.Y < tall.Y+h {
		t.Errorf("B_Short at y=%d starts inside A_Tall, which spans y=%d..%d", short.Y, tall.Y, tall.Y+h)
	}
	assertNoOverlap(t, dm, pos)
}

// Unconnected entities are not lookups. Putting them in layer 0 would mix the
// non-persistent helpers in with the real reference data.
func TestPlan_IsolatedEntitiesGoInTheirOwnBand(t *testing.T) {
	dm := capTrack()
	pos := Plan(dm)
	byName := map[string]model.Point{}
	for _, e := range dm.Entities {
		byName[e.Name] = pos[e.ID]
	}
	for _, n := range []string{"NP_NewEmployee", "NP_UserDeptToggle"} {
		if byName[n].Y <= byName["Department"].Y {
			t.Errorf("%s (y=%d) is level with the graph, not in the band below (Department y=%d)",
				n, byName[n].Y, byName["Department"].Y)
		}
	}
}

// Determinism is load-bearing, not cosmetic: a layout that shuffles rewrites the
// domain-model unit on every run, which is the churn ADR-0008 exists to prevent.
// Go randomises map iteration, so this would fail on an unsorted walk.
func TestPlan_IsDeterministic(t *testing.T) {
	want := fmt.Sprint(Plan(capTrack()))
	for i := 0; i < 50; i++ {
		if got := fmt.Sprint(Plan(capTrack())); got != want {
			t.Fatalf("run %d produced a different layout", i)
		}
	}
}

// A cycle has no longest path to a sink. The layout must still terminate and
// place everything — a self-reference or a mutual pair is legal in Mendix.
func TestPlan_SurvivesCycles(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{ent("A", "x"), ent("B", "y"), ent("C", "z")},
		Associations: []*domainmodel.Association{
			assoc("A_B", "A", "B"),
			assoc("B_A", "B", "A"), // mutual
			assoc("C_C", "C", "C"), // self
			assoc("C_A", "C", "A"),
		},
	}
	pos := Plan(dm)
	if len(pos) != 3 {
		t.Fatalf("got %d positions, want 3: %v", len(pos), pos)
	}
	seen := map[model.Point]string{}
	for _, e := range dm.Entities {
		if prev, dup := seen[pos[e.ID]]; dup {
			t.Errorf("%s and %s were placed at the same point %+v", prev, e.Name, pos[e.ID])
		}
		seen[pos[e.ID]] = e.Name
	}
}

// CONTROL: an empty or nil model must not panic and must not invent positions.
func TestPlan_EmptyModel(t *testing.T) {
	if got := Plan(nil); got != nil {
		t.Errorf("Plan(nil) = %v, want nil", got)
	}
	if got := Plan(&domainmodel.DomainModel{}); len(got) != 0 {
		t.Errorf("Plan(empty) = %v, want no positions", got)
	}
}

// Adding an entity must perturb the diagram locally, not reshuffle it. A layout
// that moved everything whenever the model changed would make every domain-model
// commit an unreadable diff, which is the practical reason to care about this
// beyond aesthetics.
//
// Measured on the real thing: adding one entity with one association to
// CapTrack's 16 moved 3 of 17 — the new entity and the two below it in its
// column.
func TestPlan_AddingAnEntityMovesFewOthers(t *testing.T) {
	before := capTrack()
	posBefore := Plan(before)

	after := capTrack()
	extra := ent("AuditEntry", "Note")
	after.Entities = append(after.Entities, extra)
	after.Associations = append(after.Associations, assoc("AuditEntry_Department", "AuditEntry", "Department"))
	posAfter := Plan(after)

	movedNames := []string{}
	for _, e := range before.Entities {
		if posBefore[e.ID] != posAfter[e.ID] {
			movedNames = append(movedNames, e.Name)
		}
	}
	// A quarter of the model is a generous ceiling; the real figure here is 2.
	if len(movedNames) > len(before.Entities)/4 {
		t.Errorf("adding one entity moved %d of %d existing entities (%v) — the layout is not local",
			len(movedNames), len(before.Entities), movedNames)
	}
	if _, ok := posAfter[extra.ID]; !ok {
		t.Error("the added entity got no position")
	}
}
