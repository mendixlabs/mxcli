// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// planByName runs Plan and indexes the result by entity name.
func planByName(t *testing.T, dm *domainmodel.DomainModel) map[string]model.Point {
	t.Helper()
	pos := Plan(dm)
	out := map[string]model.Point{}
	for _, e := range dm.Entities {
		p, ok := pos[e.ID]
		if !ok {
			t.Fatalf("%s got no position", e.Name)
		}
		out[e.Name] = p
	}
	return out
}

// gapBetween is the empty horizontal space between a box at left and one at
// right: what the association's label and its two multiplicity markers share.
func gapBetween(left, right model.Point) int {
	return right.X - (left.X + boxWidth)
}

// Rule 5 of mendixlabs/mxcli#1929: a line at least as long as its label plus
// both 1/* markers, or Studio Pro cuts the label off or draws it on a box.
// Measured: about 5 px per character plus ~12, and ~40 for the markers.
func TestPlan_GapFitsTheLongestLabel(t *testing.T) {
	name := "OrderLineModifier_OrderLineWithLongName" // 39 characters
	dm := &domainmodel.DomainModel{
		Entities:     []*domainmodel.Entity{ent("Order"), ent("OrderLine")},
		Associations: []*domainmodel.Association{assoc(name, "OrderLine", "Order")},
	}
	p := planByName(t, dm)
	need := labelWidth(name) + markerAllowance
	if got := gapBetween(p["Order"], p["OrderLine"]); got < need {
		t.Fatalf("gap = %d px, want at least %d for a %d-character label", got, need, len(name))
	}
	// The label is measured, not just "wide": 5 px per character is what
	// Studio Pro draws, and the estimate must cover it.
	if labelWidth(name) < 5*len(name)+12 {
		t.Fatalf("labelWidth(%d chars) = %d, below the measured 5 px per character + 12", len(name), labelWidth(name))
	}
}

// The control: a short name keeps the ordinary gutter, so the gap is the
// label's doing and the diagram is no wider than it has to be.
func TestPlan_ShortLabelKeepsTheGutter(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities:     []*domainmodel.Entity{ent("Order"), ent("OrderLine")},
		Associations: []*domainmodel.Association{assoc("OL", "OrderLine", "Order")},
	}
	p := planByName(t, dm)
	if got := gapBetween(p["Order"], p["OrderLine"]); got != columnGutter {
		t.Fatalf("gap = %d px for a 2-character label, want the gutter %d", got, columnGutter)
	}
}

// Each column boundary is sized by the labels that cross IT: a long name
// between columns 1 and 2 does not widen the gap between 0 and 1.
func TestPlan_GapIsPerBoundary(t *testing.T) {
	long := strings.Repeat("X", 40)
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{ent("A"), ent("B"), ent("C")},
		Associations: []*domainmodel.Association{
			assoc("B_A", "B", "A"),
			assoc(long, "C", "B"),
		},
	}
	p := planByName(t, dm)
	if got := gapBetween(p["A"], p["B"]); got != columnGutter {
		t.Errorf("gap A|B = %d, want the gutter %d: no long label crosses it", got, columnGutter)
	}
	if got, need := gapBetween(p["B"], p["C"]), labelWidth(long)+markerAllowance; got < need {
		t.Errorf("gap B|C = %d, want at least %d for the 40-character label", got, need)
	}
}

// Rule 4: a referrer sits on its target's row, so the line between them is
// horizontal instead of a diagonal from wherever the column stack put it.
// Product is the second lookup in column 0; OrderLine, which references only
// Product, must be level with it, not at the top of column 1.
func TestPlan_ReferrerSharesItsTargetsRow(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{
			ent("Customer", "a", "b", "c", "d", "e"),
			ent("Product", "a"),
			ent("Order", "a"),
			ent("OrderLine", "a"),
		},
		Associations: []*domainmodel.Association{
			assoc("Order_Customer", "Order", "Customer"),
			assoc("OrderLine_Product", "OrderLine", "Product"),
		},
	}
	p := planByName(t, dm)
	if p["Product"].Y == OriginY {
		t.Fatalf("precondition: Product should be second in its column, is at the top")
	}
	if p["OrderLine"].Y != p["Product"].Y {
		t.Errorf("OrderLine y = %d, Product y = %d: want the same row", p["OrderLine"].Y, p["Product"].Y)
	}
	if p["Order"].Y != p["Customer"].Y {
		t.Errorf("Order y = %d, Customer y = %d: want the same row", p["Order"].Y, p["Customer"].Y)
	}
}

// With several targets, the row is the one it has most associations to.
func TestPlan_RowFollowsTheMostLinkedTarget(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{ent("Alpha"), ent("Beta"), ent("Ref")},
		Associations: []*domainmodel.Association{
			assoc("Ref_Alpha", "Ref", "Alpha"),
			assoc("Ref_Beta", "Ref", "Beta"),
			assoc("Ref_Beta2", "Ref", "Beta"),
		},
	}
	p := planByName(t, dm)
	if p["Ref"].Y != p["Beta"].Y {
		t.Errorf("Ref y = %d, want Beta's %d (two associations, against one to Alpha at %d)",
			p["Ref"].Y, p["Beta"].Y, p["Alpha"].Y)
	}
}

// Two referrers of one target cannot both have its row: the second goes below
// the first, clear of it, rather than on top of it.
func TestPlan_RowConflictGoesBelow(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{ent("Customer"), ent("Invoice", "a", "b", "c", "d"), ent("Order")},
		Associations: []*domainmodel.Association{
			assoc("Invoice_Customer", "Invoice", "Customer"),
			assoc("Order_Customer", "Order", "Customer"),
		},
	}
	p := planByName(t, dm)
	first, second := p["Invoice"], p["Order"]
	if first.Y != p["Customer"].Y {
		t.Fatalf("Invoice y = %d, want Customer's row %d", first.Y, p["Customer"].Y)
	}
	_, h := boxSize(dm.Entities[1])
	if second.Y < first.Y+h+rowGutter {
		t.Errorf("Order y = %d overlaps or crowds Invoice (y %d, height %d, gutter %d)",
			second.Y, first.Y, h, rowGutter)
	}
}
