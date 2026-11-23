// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func entAt(id, name string, p model.Point) *domainmodel.Entity {
	return &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: model.ID(id)},
		Name:        name,
		Location:    p,
	}
}

func link(from, to string) *domainmodel.Association {
	return &domainmodel.Association{ParentID: model.ID(from), ChildID: model.ID(to)}
}

func entityNames(es []*domainmodel.Entity) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Name)
	}
	return out
}

// A script that creates three linked entities and never runs `mxcli layout`
// leaves them on slots 0, 1 and 2. Slot 0 is also where Plan puts its first
// entity, so it says nothing; slots 1 and 2 are the signature.
func TestNotLaidOut_ScriptPlacedModelIsReported(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{
			entAt("c", "Customer", GridSlot(0)),
			entAt("o", "Order", GridSlot(1)),
			entAt("l", "OrderLine", GridSlot(2)),
		},
		Associations: []*domainmodel.Association{link("o", "c"), link("l", "o")},
	}
	got := entityNames(NotLaidOut(dm))
	if len(got) != 2 || got[0] != "Order" || got[1] != "OrderLine" {
		t.Fatalf("NotLaidOut = %v, want [Order OrderLine]", got)
	}
}

// The control that makes the test above mean something: the same model after
// Plan has run reports nothing. If Plan's output could land on a grid slot
// unnoticed, `mxcli layout` would not clear the finding it is the repair for.
func TestNotLaidOut_PlannedModelIsClean(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{
			entAt("c", "Customer", GridSlot(0)),
			entAt("o", "Order", GridSlot(1)),
			entAt("l", "OrderLine", GridSlot(2)),
		},
		Associations: []*domainmodel.Association{link("o", "c"), link("l", "o")},
	}
	if len(NotLaidOut(dm)) == 0 {
		t.Fatal("precondition: the script-placed model must be reported")
	}
	for id, p := range Plan(dm) {
		for _, e := range dm.Entities {
			if e.ID == id {
				e.Location = p
			}
		}
	}
	if got := NotLaidOut(dm); len(got) != 0 {
		t.Fatalf("after Plan, NotLaidOut = %v, want none", entityNames(got))
	}
}

// A laid-out module that gained one linked entity afterwards: the new one is on
// a grid slot, its partner is not. That is the "forgot to re-run layout" case.
func TestNotLaidOut_NewEntityInALaidOutModuleIsReported(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{
			entAt("c", "Customer", model.Point{X: 140, Y: 20}),
			entAt("o", "Order", model.Point{X: 420, Y: 20}),
			entAt("i", "Invoice", GridSlot(2)),
		},
		Associations: []*domainmodel.Association{link("o", "c"), link("i", "o")},
	}
	got := entityNames(NotLaidOut(dm))
	if len(got) != 1 || got[0] != "Invoice" {
		t.Fatalf("NotLaidOut = %v, want [Invoice]", got)
	}
}

// A generalization is a link: a specialisation belongs beside its base.
func TestNotLaidOut_GeneralizationCountsAsALink(t *testing.T) {
	spec := entAt("s", "Employee", GridSlot(1))
	spec.GeneralizationID = "p"
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{entAt("p", "Person", model.Point{X: 140, Y: 20}), spec},
	}
	if got := entityNames(NotLaidOut(dm)); len(got) != 1 || got[0] != "Employee" {
		t.Fatalf("NotLaidOut = %v, want [Employee]", got)
	}
}

// An entity with no local link has nothing to be laid out relative to: a
// helper on a grid slot is not a badly laid-out diagram.
func TestNotLaidOut_UnlinkedEntitiesAreIgnored(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{
			entAt("a", "Helper", GridSlot(1)),
			entAt("b", "OtherHelper", GridSlot(2)),
		},
	}
	if got := NotLaidOut(dm); len(got) != 0 {
		t.Fatalf("NotLaidOut = %v, want none", entityNames(got))
	}
}

// Positions off the grid, the ordinary case for anything placed by hand in
// Studio Pro, are never reported, even one pixel off a slot.
func TestNotLaidOut_OffGridPositionsAreIgnored(t *testing.T) {
	s1, s2 := GridSlot(1), GridSlot(2)
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{
			entAt("c", "Customer", model.Point{X: s1.X + 1, Y: s1.Y}),
			entAt("o", "Order", model.Point{X: s2.X, Y: s2.Y - 1}),
		},
		Associations: []*domainmodel.Association{link("o", "c")},
	}
	if got := NotLaidOut(dm); len(got) != 0 {
		t.Fatalf("NotLaidOut = %v, want none", entityNames(got))
	}
}

func TestOnGridSlot(t *testing.T) {
	for n := 0; n < 3*GridColumns; n++ {
		if !onGridSlot(GridSlot(n)) {
			t.Errorf("GridSlot(%d) = %v not recognised", n, GridSlot(n))
		}
	}
	for _, p := range []model.Point{
		{X: 0, Y: 0},
		{X: OriginX + GridColumns*GridColumnPitch, Y: OriginY}, // one column past the grid
		{X: OriginX - GridColumnPitch, Y: OriginY},             // left of the origin
		{X: OriginX, Y: OriginY - GridRowPitch},                // above the origin
	} {
		if onGridSlot(p) {
			t.Errorf("%v reported as a grid slot", p)
		}
	}
}

func TestNotLaidOut_NilModel(t *testing.T) {
	if got := NotLaidOut(nil); got != nil {
		t.Fatalf("NotLaidOut(nil) = %v, want nil", got)
	}
}

// Plan's coordinates are sums of box heights and gutters, so they can land on a
// grid slot by arithmetic. Six lookups stacked in column 0 with heights
// 60, 60, 60, 66, 114 put the sixth at y = 660 = GridSlot(12). Plan placed it,
// so it must not be reported: the finding names `mxcli layout` as its repair,
// and a repair that leaves its own finding behind can never pass a gate.
func TestNotLaidOut_PlanThatLandsOnASlotIsNotReported(t *testing.T) {
	withAttrs := func(e *domainmodel.Entity, n int) *domainmodel.Entity {
		for i := 0; i < n; i++ {
			e.Attributes = append(e.Attributes, &domainmodel.Attribute{Name: "A"})
		}
		return e
	}
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{
			withAttrs(entAt("a", "A", model.Point{}), 1), // h = 60 (minimum)
			withAttrs(entAt("b", "B", model.Point{}), 1), // h = 60
			withAttrs(entAt("c", "C", model.Point{}), 1), // h = 60
			withAttrs(entAt("d", "D", model.Point{}), 2), // h = 66
			withAttrs(entAt("e", "E", model.Point{}), 5), // h = 114
			entAt("f", "F", model.Point{}),
			entAt("r", "Referrer", model.Point{}),
		},
	}
	for _, to := range []string{"a", "b", "c", "d", "e", "f"} {
		dm.Associations = append(dm.Associations, link("r", to))
	}
	plan := Plan(dm)
	for _, e := range dm.Entities {
		e.Location = plan[e.ID]
	}
	if f := dm.Entities[5]; f.Location != GridSlot(12) {
		t.Fatalf("precondition: Plan put F at %v, want GridSlot(12) = %v", f.Location, GridSlot(12))
	}
	if got := NotLaidOut(dm); len(got) != 0 {
		t.Fatalf("NotLaidOut = %v, want none: Plan placed every entity", entityNames(got))
	}
}

// (100, 100) is GridSlot(0), but it is also the first position anyone writes by
// hand (`@position(100, 100)` is the usual first line of a script) and Plan's
// own origin. An entity there says nothing about whether the module was laid
// out, even when Plan would put a different entity in that spot.
func TestNotLaidOut_OriginIsNotEvidence(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{
			entAt("o", "Order", GridSlot(0)), // Plan would put Customer here
			entAt("c", "Customer", model.Point{X: 420, Y: 20}),
		},
		Associations: []*domainmodel.Association{link("o", "c")},
	}
	if got := NotLaidOut(dm); len(got) != 0 {
		t.Fatalf("NotLaidOut = %v, want none", entityNames(got))
	}
}
