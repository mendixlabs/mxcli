// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func withN(name string, n int) *domainmodel.Entity {
	return ent(name, strings.Split(strings.Repeat("a,", n), ",")[:n]...)
}

// shopLike is PizzaV1's Shop in shape: a small graph (Cart and what references
// it) and unconnected entities, most with cross-module lines, one (TrackView)
// far taller than the graph.
func shopLike() (*domainmodel.DomainModel, CrossLines) {
	cart, line, home := withN("Cart", 29), withN("CartLine", 24), withN("HomeView", 16)
	track, checkout, lookup := withN("TrackView", 90), withN("CheckoutForm", 50), withN("LookupAttempt", 1)
	errView := withN("ErrorView", 1)
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{cart, line, home, track, checkout, lookup, errView},
		Associations: []*domainmodel.Association{
			named("CartLine_Cart", "CartLine", "Cart"),
			named("HomeView_Cart", "HomeView", "Cart"),
		},
	}
	cross := CrossLines{
		track.ID:    {"TrackView_Order"},
		checkout.ID: {"CheckoutForm_Zone"},
		lookup.ID:   {"LookupAttempt_User"},
		line.ID:     {"CartLine_Product"},
	}
	return dm, cross
}

func bounds(dm *domainmodel.DomainModel, pos map[model.ID]model.Point) (w, h int) {
	minX, minY, maxX, maxY := 1<<30, 1<<30, 0, 0
	for _, e := range dm.Entities {
		r := EntityBoxAt(e, pos[e.ID])
		minX, minY = min(minX, r.X), min(minY, r.Y)
		maxX, maxY = max(maxX, r.X+r.W), max(maxY, r.Y+r.H)
	}
	return maxX - minX, maxY - minY
}

// Unconnected entities fill the free space beside the graph instead of a band
// below it: TrackView alone is taller than the whole graph, so stacking it
// underneath doubled the height for nothing. Everything stays readable: no
// cross-module line blocked, no box touching another, no line through a box.
func TestPlanWith_UnconnectedEntitiesUseTheSpaceBesideTheGraph(t *testing.T) {
	dm, cross := shopLike()
	pos := PlanWith(dm, cross)
	if v := crossViolations(dm, pos, cross); len(v) > 0 {
		t.Fatalf("cross-module lines blocked:\n%s", strings.Join(v, "\n"))
	}
	for i, a := range dm.Entities {
		for _, b := range dm.Entities[i+1:] {
			if EntityBoxAt(a, pos[a.ID]).TooClose(EntityBoxAt(b, pos[b.ID])) {
				t.Errorf("%s and %s touch", a.Name, b.Name)
			}
		}
	}
	if _, boxes := Crossings(dm, pos, Anchors(dm, pos), cross); boxes != 0 {
		t.Errorf("%d lines run through a box", boxes)
	}
	// The graph ends where Cart (29 attributes) does; TrackView (90) starts
	// beside it, not under it.
	byName := map[string]model.Point{}
	for _, e := range dm.Entities {
		byName[e.Name] = pos[e.ID]
	}
	_, hCart := boxSize(dm.Entities[0])
	if byName["TrackView"].Y >= byName["Cart"].Y+hCart {
		t.Errorf("TrackView starts at y %d, under the graph (Cart ends at %d)", byName["TrackView"].Y, byName["Cart"].Y+hCart)
	}
	// Compact: no taller than the tallest entity plus the graph's top margin
	// and the stagger its line needs.
	_, h := bounds(dm, pos)
	_, hTrack := boxSize(dm.Entities[3])
	if h > hTrack+600 {
		t.Errorf("diagram is %d px tall for a %d-px TrackView", h, hTrack)
	}
}

// A box placed later must not sit in the lines of one placed earlier further
// right: P (no lines) fits under C1, exactly where C2's lines run left. The
// case a search over sizes found; it fails without the check.
func TestPlanWith_LaterBoxKeepsOutOfEarlierLines(t *testing.T) {
	c1, c2, p := withN("C1", 1), withN("C2", 10), withN("P", 10)
	dm := &domainmodel.DomainModel{Entities: []*domainmodel.Entity{c1, c2, p}}
	cross := CrossLines{c1.ID: {"C1_X"}, c2.ID: {"C2_X", "C2_Y", "C2_Z"}}
	pos := PlanWith(dm, cross)
	if v := crossViolations(dm, pos, cross); len(v) > 0 {
		t.Fatalf("lines blocked:\n%s", strings.Join(v, "\n"))
	}
}
