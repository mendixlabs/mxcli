// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// placed builds an entity with an ID, a position and n attributes.
func placed(name string, x, y, attrs int) *domainmodel.Entity {
	e := withAttributes(name, attrs)
	e.ID = model.ID("id-" + name)
	e.Location = model.Point{X: x, Y: y}
	return e
}

// absolute is where an anchor sits on the canvas, by the same box estimate
// Anchors uses.
func absolute(e *domainmodel.Entity, p model.Point) (float64, float64) {
	w, h := boxSize(e)
	return float64(e.Location.X) + float64(p.X)*float64(w)/100,
		float64(e.Location.Y) + float64(p.Y)*float64(h)/100
}

func anchorsFor(t *testing.T, dm *domainmodel.DomainModel) map[string]AnchorPair {
	t.Helper()
	pos := map[model.ID]model.Point{}
	for _, e := range dm.Entities {
		pos[e.ID] = e.Location
	}
	got := Anchors(dm, pos)
	out := map[string]AnchorPair{}
	for _, a := range dm.Associations {
		if p, ok := got[a.ID]; ok {
			out[a.Name] = p
		}
	}
	return out
}

func named(name, from, to string) *domainmodel.Association {
	a := assoc(name, from, to)
	a.ID = model.ID("assoc-" + name)
	return a
}

// Two boxes on one row, different heights: the line leaves the facing sides at
// the same absolute y, in the header (which is the same height on every box),
// so it is horizontal whatever the attribute counts. Studio Pro's default
// (50 % on both) slants it by half the height difference.
func TestAnchors_SameRowIsHorizontal(t *testing.T) {
	customer := placed("Customer", 100, 100, 1) // h 84
	order := placed("Order", 400, 100, 9)       // h 184
	dm := &domainmodel.DomainModel{
		Entities:     []*domainmodel.Entity{customer, order},
		Associations: []*domainmodel.Association{named("Order_Customer", "Order", "Customer")},
	}
	a := anchorsFor(t, dm)["Order_Customer"]
	// Order holds the reference (ParentConnection) and sits on the right, so it
	// connects on its LEFT side; Customer on its RIGHT.
	if a.Parent.X != 0 || a.Child.X != 100 {
		t.Fatalf("sides: parent x %d (want 0, left), child x %d (want 100, right)", a.Parent.X, a.Child.X)
	}
	_, py := absolute(order, a.Parent)
	_, cy := absolute(customer, a.Child)
	if d := py - cy; d < -1 || d > 1 {
		t.Fatalf("line not horizontal: parent end y %.1f, child end y %.1f", py, cy)
	}
	if py > 100+40 {
		t.Errorf("line at y %.1f, want it in the header rows (top 100)", py)
	}
}

// A referrer beside a TALL target, lower down, still gets a straight line: both
// ends at a y inside the band the two boxes share. This is Fulfilment's shape,
// where every referrer of the 51-attribute Order converged on one point.
func TestAnchors_OverlappingBandIsHorizontal(t *testing.T) {
	order := placed("Order", 100, 100, 51) // h 856
	var ents = []*domainmodel.Entity{order}
	var assocs []*domainmodel.Association
	for i, y := range []int{100, 300, 600} {
		n := fmt.Sprintf("R%d", i)
		ents = append(ents, placed(n, 450, y, 4))
		assocs = append(assocs, named(n+"_Order", n, "Order"))
	}
	dm := &domainmodel.DomainModel{Entities: ents, Associations: assocs}
	got := anchorsFor(t, dm)
	seen := map[int]string{}
	for i := range assocs {
		a := got[assocs[i].Name]
		_, py := absolute(ents[i+1], a.Parent)
		_, cy := absolute(order, a.Child)
		// Rounding a percentage of an 856-px box is worth up to ~4 px; anchors
		// are chosen on the tall box's whole-percent grid so it is ~0.5.
		if d := py - cy; d < -1.5 || d > 1.5 {
			t.Errorf("%s: not horizontal, referrer end y %.1f, Order end y %.1f", assocs[i].Name, py, cy)
		}
		if prev, dup := seen[a.Child.Y]; dup {
			t.Errorf("%s and %s meet Order at the same point (y %d%%)", prev, assocs[i].Name, a.Child.Y)
		}
		seen[a.Child.Y] = assocs[i].Name
	}
}

// Boxes in one column: bottom of the upper to top of the lower, straight down.
func TestAnchors_SameColumnIsVertical(t *testing.T) {
	order := placed("Order", 100, 100, 3)
	line := placed("OrderLine", 100, 300, 3)
	dm := &domainmodel.DomainModel{
		Entities:     []*domainmodel.Entity{order, line},
		Associations: []*domainmodel.Association{named("OrderLine_Order", "OrderLine", "Order")},
	}
	a := anchorsFor(t, dm)["OrderLine_Order"]
	if a.Parent != (model.Point{X: 50, Y: 0}) || a.Child != (model.Point{X: 50, Y: 100}) {
		t.Fatalf("got parent %v child %v, want top-centre of OrderLine to bottom-centre of Order", a.Parent, a.Child)
	}
}

// Lines that cannot be straight still must not meet at one point: they are
// spread down the side, in the order of their far ends, so they do not cross
// at the box either.
func TestAnchors_DiagonalsAreSpreadInOrder(t *testing.T) {
	hub := placed("Hub", 100, 100, 1) // h 84: shares no band with any below
	ents := []*domainmodel.Entity{hub}
	var assocs []*domainmodel.Association
	for i, y := range []int{400, 700, 1000} {
		n := fmt.Sprintf("S%d", i)
		ents = append(ents, placed(n, 450, y, 1))
		assocs = append(assocs, named(n+"_Hub", n, "Hub"))
	}
	dm := &domainmodel.DomainModel{Entities: ents, Associations: assocs}
	got := anchorsFor(t, dm)
	prev := -1
	for _, a := range assocs {
		p := got[a.Name].Child
		if p.X != 100 {
			t.Errorf("%s: Hub end on x %d, want the right side (100)", a.Name, p.X)
		}
		if p.Y <= prev {
			t.Errorf("%s: Hub end at %d%%, not below the previous line's %d%%: they meet or cross", a.Name, p.Y, prev)
		}
		prev = p.Y
	}
}

// What Anchors must never return: a point off the box edge, a fraction (Mendix
// refuses a non-integer "x;y", StorageLoadException on 11.13), or anything for
// a self-association or an association to an entity that is not here.
func TestAnchors_OnlyEdgePointsAndOnlyWhatItCanPlace(t *testing.T) {
	dm := capTrack()
	pos := Plan(dm)
	for _, e := range dm.Entities {
		e.Location = pos[e.ID]
	}
	self := named("Dept_Parent", "Department", "Department")
	dangling := named("Ghost", "Department", "NotInModel")
	for i, a := range dm.Associations {
		a.ID = model.ID(fmt.Sprintf("a%d", i))
	}
	dm.Associations = append(dm.Associations, self, dangling)

	got := Anchors(dm, pos)
	if _, ok := got[self.ID]; ok {
		t.Error("a self-association was re-anchored; its loop is the author's to place")
	}
	if _, ok := got[dangling.ID]; ok {
		t.Error("an association to an entity not in the model got anchors")
	}
	if len(got) != len(dm.Associations)-2 {
		t.Errorf("anchored %d associations, want %d", len(got), len(dm.Associations)-2)
	}
	for id, p := range got {
		for _, pt := range []model.Point{p.Parent, p.Child} {
			onEdge := pt.X == 0 || pt.X == 100 || pt.Y == 0 || pt.Y == 100
			inRange := pt.X >= 0 && pt.X <= 100 && pt.Y >= 0 && pt.Y <= 100
			if !onEdge || !inRange {
				t.Errorf("%s: anchor %v is not on the box edge", id, pt)
			}
		}
	}
}

func TestAnchors_IsDeterministic(t *testing.T) {
	dm := capTrack()
	for i, a := range dm.Associations {
		a.ID = model.ID(fmt.Sprintf("a%d", i))
	}
	pos := Plan(dm)
	want := fmt.Sprint(Anchors(dm, pos))
	for i := 0; i < 30; i++ {
		if got := fmt.Sprint(Anchors(dm, pos)); got != want {
			t.Fatalf("run %d gave different anchors", i)
		}
	}
}

// Two associations between the same pair of entities (Fulfilment's
// Order_Courier and Courier_CurrentOrder) must not be drawn as one line: each
// is straight, at its own height, far enough apart for two labels.
func TestAnchors_ParallelAssociationsAreOffset(t *testing.T) {
	order := placed("Order", 100, 100, 20)
	courier := placed("Courier", 450, 100, 12)
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{order, courier},
		Associations: []*domainmodel.Association{
			named("Order_Courier", "Order", "Courier"),
			named("Courier_CurrentOrder", "Courier", "Order"),
		},
	}
	got := anchorsFor(t, dm)
	var ys []float64
	for _, a := range dm.Associations {
		p := got[a.Name]
		from, to := order, courier
		if a.Name == "Courier_CurrentOrder" {
			from, to = courier, order
		}
		_, py := absolute(from, p.Parent)
		_, cy := absolute(to, p.Child)
		if d := py - cy; d < -1 || d > 1 {
			t.Errorf("%s: not horizontal (%.1f vs %.1f)", a.Name, py, cy)
		}
		ys = append(ys, py)
	}
	if d := ys[0] - ys[1]; d > -16 && d < 16 {
		t.Fatalf("the two lines are %.1f px apart, want room for two labels (16+)", d)
	}
}

// A straight line may not run through a box between its two ends. Catalog's
// Product_Category spans a column: straight, it crossed the whole Option box.
// Such a line is spread like any other instead.
func TestAnchors_StraightLineDoesNotCrossABox(t *testing.T) {
	category := placed("Category", 100, 100, 4)
	option := placed("Option", 400, 60, 16) // between them, spanning their rows
	product := placed("Product", 700, 100, 10)
	dm := &domainmodel.DomainModel{
		Entities:     []*domainmodel.Entity{category, option, product},
		Associations: []*domainmodel.Association{named("Product_Category", "Product", "Category")},
	}
	a := anchorsFor(t, dm)["Product_Category"]
	_, py := absolute(product, a.Parent)
	_, cy := absolute(category, a.Child)
	_, oh := boxSize(option)
	lo, hi := py, cy
	if lo > hi {
		lo, hi = hi, lo
	}
	straight := hi-lo <= 1.5 // rounding a percentage leaves a fraction of a pixel
	if straight && hi >= float64(option.Location.Y) && lo <= float64(option.Location.Y+oh) {
		t.Fatalf("Product_Category is a straight line at y %.1f through Option (y %d..%d)", py, option.Location.Y, option.Location.Y+oh)
	}
}
