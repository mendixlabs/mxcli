// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// crossViolations lists every cross-module line that would run through a box:
// Studio Pro draws each from the left side of its entity, at i/(n+1) of the
// box's height for n lines, straight to the canvas's left edge (x = 0), and
// the label needs labelHalf above and below. Nothing may sit in that strip.
func crossViolations(dm *domainmodel.DomainModel, pos map[model.ID]model.Point, cross CrossLines) []string {
	var out []string
	for _, e := range dm.Entities {
		names := cross[e.ID]
		if len(names) == 0 {
			continue
		}
		p := pos[e.ID]
		_, h := boxSize(e)
		for i := range names {
			ly := p.Y + (i+1)*h/(len(names)+1)
			for _, o := range dm.Entities {
				if o == e {
					continue
				}
				q := pos[o.ID]
				w, oh := boxSize(o)
				if q.X < p.X && q.X+w > 0 && q.Y < ly+labelHalf && q.Y+oh > ly-labelHalf {
					out = append(out, fmt.Sprintf("%s's line %q at y %d runs through %s", e.Name, names[i], ly, o.Name))
				}
			}
		}
	}
	return out
}

// Fulfilment's shape: Order references Customer, so phase 1 puts it on
// Customer's row, and Order's cross-module line (Cart_ActiveOrder) then runs
// left straight through Customer. The control is the same model without the
// cross-module line: Order on Customer's row. With it, Order moves to where
// its line is clear.
func TestPlanWith_CrossModuleLineGetsAClearBand(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{ent("Customer", "a", "b", "c", "d", "e", "f"), ent("Order", "a")},
		Associations: []*domainmodel.Association{
			assoc("Order_Customer", "Order", "Customer"),
		},
	}
	order := dm.Entities[1]

	plain := Plan(dm)
	if plain[order.ID].Y != plain[dm.Entities[0].ID].Y {
		t.Fatalf("control: without cross-module lines Order should share Customer's row")
	}
	cross := CrossLines{order.ID: {"Cart_ActiveOrder"}}
	if v := crossViolations(dm, plain, cross); len(v) == 0 {
		t.Fatalf("control: the plain plan should put the line through Customer")
	}

	pos := PlanWith(dm, cross)
	if v := crossViolations(dm, pos, cross); len(v) > 0 {
		t.Fatalf("cross-module line not clear:\n%s", strings.Join(v, "\n"))
	}
}

// Several lines on one entity, each with its own strip.
func TestPlanWith_EveryLineOfAnEntityIsClear(t *testing.T) {
	dm := capTrack()
	byName := map[string]*domainmodel.Entity{}
	for _, e := range dm.Entities {
		byName[e.Name] = e
	}
	cross := CrossLines{
		byName["Employee"].ID:      {"Employee_Account", "Employee_Manager", "Employee_Photo"},
		byName["PlanScope"].ID:     {"PlanScope_Owner"},
		byName["EmployeeMonth"].ID: {"EmployeeMonth_Payroll"},
	}
	if v := crossViolations(dm, Plan(dm), cross); len(v) == 0 {
		t.Fatalf("control: the plain CapTrack plan should block at least one line")
	}
	pos := PlanWith(dm, cross)
	if v := crossViolations(dm, pos, cross); len(v) > 0 {
		t.Fatalf("lines not clear:\n%s", strings.Join(v, "\n"))
	}
	// Moving entities for their bands must not make them overlap.
	for i, a := range dm.Entities {
		for _, b := range dm.Entities[i+1:] {
			ra, rb := EntityBoxAt(a, pos[a.ID]), EntityBoxAt(b, pos[b.ID])
			if ra.Intersects(rb) {
				t.Errorf("%s and %s overlap", a.Name, b.Name)
			}
		}
	}
}

// The label of a line from column 0 sits between the canvas edge and the
// entity: column 0 starts far enough right for the longest one. The control,
// no cross-module lines, keeps OriginX.
func TestPlanWith_LeftMarginFitsTheLabel(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities:     []*domainmodel.Entity{ent("Customer"), ent("Order")},
		Associations: []*domainmodel.Association{assoc("Order_Customer", "Order", "Customer")},
	}
	customer := dm.Entities[0]
	if got := Plan(dm)[customer.ID].X; got != OriginX {
		t.Fatalf("control: column 0 at x %d, want OriginX %d", got, OriginX)
	}
	name := "Customer_AdministrationAccount" // 30 characters
	pos := PlanWith(dm, CrossLines{customer.ID: {name}})
	if need := labelWidth(name) + markerAllowance; pos[customer.ID].X < need {
		t.Fatalf("column 0 at x %d, want at least %d for the %d-character label", pos[customer.ID].X, need, len(name))
	}
}

// An entity with no association in its module but a cross-module one is in the
// band underneath; it gets a row of its own at the left, where nothing is in
// the way of its line.
func TestPlanWith_IsolatedEntityWithCrossLineHasItsOwnRow(t *testing.T) {
	var ents []*domainmodel.Entity
	for _, n := range []string{"A", "B", "C", "D", "E"} {
		ents = append(ents, ent(n))
	}
	dm := &domainmodel.DomainModel{Entities: ents}
	cross := CrossLines{ents[3].ID: {"D_Account"}} // D, fourth in the grid
	if v := crossViolations(dm, Plan(dm), cross); len(v) == 0 {
		t.Fatalf("control: in the plain grid D's line should run through A, B and C")
	}
	pos := PlanWith(dm, cross)
	if v := crossViolations(dm, pos, cross); len(v) > 0 {
		t.Fatalf("not clear:\n%s", strings.Join(v, "\n"))
	}
}

func TestPlanWith_NilIsPlan(t *testing.T) {
	if fmt.Sprint(PlanWith(capTrack(), nil)) != fmt.Sprint(Plan(capTrack())) {
		t.Fatal("PlanWith(dm, nil) differs from Plan(dm)")
	}
}

// CrossLinesFor finds both ends that are drawn in a module: its own
// associations' local FROM entities, and the TO entities other modules'
// associations name, by qualified name.
func TestCrossLinesFor(t *testing.T) {
	order := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "o"}, Name: "Order"}
	customer := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "c"}, Name: "Customer"}
	shop := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-shop"},
		Entities:    []*domainmodel.Entity{order, customer},
		CrossAssociations: []*domainmodel.CrossModuleAssociation{
			{Name: "Order_Account", ParentID: "o", ChildRef: "Administration.Account"},
		},
	}
	other := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-crm"},
		CrossAssociations: []*domainmodel.CrossModuleAssociation{
			{Name: "Lead_Customer", ParentID: "lead", ChildRef: "Shop.Customer"},
			{Name: "Lead_Order", ParentID: "lead", ChildRef: "Shop.Order"},
			{Name: "Lead_Other", ParentID: "lead", ChildRef: "Shopping.Order"}, // another module
		},
	}
	got := CrossLinesFor("Shop", shop, []*domainmodel.DomainModel{shop, other})
	if fmt.Sprint(got["o"]) != "[Lead_Order Order_Account]" {
		t.Errorf("Order's lines = %v, want [Lead_Order Order_Account]", got["o"])
	}
	if fmt.Sprint(got["c"]) != "[Lead_Customer]" {
		t.Errorf("Customer's lines = %v, want [Lead_Customer]", got["c"])
	}
}
