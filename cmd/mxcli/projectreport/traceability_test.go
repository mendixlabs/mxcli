// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
)

func req(id, slice, title string, built bool, anchors ...brain.AnchorResult) brain.RequirementResult {
	return brain.RequirementResult{ID: id, Slice: slice, Title: title, Built: built, Anchors: anchors}
}

func anchor(a, kind string, st brain.AnchorState) brain.AnchorResult {
	return brain.AnchorResult{Anchor: a, Kind: kind, State: st}
}

func listed(id, shard string, kind brain.Kind, open bool, title, body string, anchors ...string) brain.ListedEntry {
	return brain.ListedEntry{Entry: brain.Entry{ID: id, Kind: kind, Open: open, Title: title, Body: body, Anchors: anchors}, Shard: shard}
}

func TestTraceabilityStatusFootprintAndProof(t *testing.T) {
	f := newFixture(t)
	f.module("Shop", "")
	for _, m := range []string{"Shop.ACT_Order", "Shop.SUB_Price", "Shop.SUB_Log", "Shop.A", "Shop.B", "Shop.C", "Shop.D", "Shop.ACT_Pay"} {
		f.microflow(m, true, "")
	}
	f.call("Shop.ACT_Order", "Shop.SUB_Price")
	f.call("Shop.ACT_Order", "Shop.SUB_Log")
	for _, c := range []string{"Shop.A", "Shop.B", "Shop.C", "Shop.D"} {
		f.call(c, "Shop.SUB_Log") // SUB_Log has 5 callers: a helper, not footprint
	}
	f.entity("Shop.Payment", "PERSISTENT", "")
	f.ref("Shop.ACT_Pay", "MICROFLOW", "Shop.Payment", "ENTITY", "create")

	in := TraceabilityInput{
		Requirements: []brain.RequirementResult{
			req("aaaaaa", "01-order", "Place an order (slide 3)", true, anchor("@Shop.ACT_Order", "microflow", brain.Resolved)),
			req("bbbbbb", "01-order", "Pay", true, anchor("@Shop.Payment", "entity", brain.Resolved)),
			req("cccccc", "02-admin", "Refunds", false, anchor("@Shop.ACT_Refund", "", brain.NotFound)),
			req("dddddd", "02-admin", "Admins exist", true, anchor("@Shop.Admin", "module role", brain.Resolved)),
			req("eeeeee", "02-admin", "Nice to have", false),
		},
		Entries: []brain.ListedEntry{
			listed("aaaaaa", "plan/01-order", brain.KindRequirement, false, "Place an order (slide 3)", ""),
			listed("d00001", "Shop", "", false, "Orders are priced once", "Partly meets requirement `aaaaaa`: no discounts yet.", "@Shop.ACT_Order"),
			listed("q00001", "Shop", "", true, "Do refunds need approval?", "Affects cccccc.", "@Shop.Other"),
		},
		Evidence: &TestsData{
			Tests:    []TestInfo{{Name: "prices an order", Reached: []string{"Shop.SUB_Price"}}},
			Journeys: []JourneyInfo{{Name: "checkout", Executed: []string{"Shop.Unrelated"}}},
		},
		Milestones: &Milestones{Milestones: []MilestoneSpec{
			{Name: "M1", Slices: []string{"01-order"}},
			{Name: "M2", Slices: []string{"02-admin"}, Requirements: []string{"zzzzzz"}},
		}},
	}
	d, err := CollectTraceability(f.cat, in)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]TraceRequirement{}
	for _, m := range d.Milestones {
		for _, r := range m.Requirements {
			got[r.ID] = r
		}
	}
	status := map[string]string{}
	for id, r := range got {
		status[id] = r.Status
	}
	want := map[string]string{"aaaaaa": StatusPartial, "bbbbbb": StatusBuilt, "cccccc": StatusNotBuilt, "dddddd": StatusBuilt, "eeeeee": StatusNotTraceable}
	if !reflect.DeepEqual(status, want) {
		t.Errorf("status = %v\nwant %v", status, want)
	}
	if fp := got["aaaaaa"].Footprint; !reflect.DeepEqual(fp, []string{"Shop.ACT_Order", "Shop.SUB_Price"}) {
		t.Errorf("footprint = %v (the 5-caller helper must be left out)", fp)
	}
	if !reflect.DeepEqual(got["aaaaaa"].Tests, []string{"prices an order"}) || len(got["aaaaaa"].Journeys) != 0 {
		t.Errorf("proof = %v / %v", got["aaaaaa"].Tests, got["aaaaaa"].Journeys)
	}
	if !reflect.DeepEqual(got["bbbbbb"].Footprint, []string{"Shop.ACT_Pay"}) || len(got["bbbbbb"].Tests) != 0 {
		t.Errorf("entity footprint = %v, tests %v", got["bbbbbb"].Footprint, got["bbbbbb"].Tests)
	}
	if got["dddddd"].FootprintNote == "" || len(got["dddddd"].Footprint) != 0 {
		t.Errorf("a role anchor has no microflow footprint and says so: %+v", got["dddddd"])
	}
	if len(got["aaaaaa"].Decisions) != 1 || len(got["cccccc"].Questions) != 1 {
		t.Errorf("context: decisions %v, questions %v", got["aaaaaa"].Decisions, got["cccccc"].Questions)
	}
	if got["aaaaaa"].Spec != "slide 3" {
		t.Errorf("spec = %q", got["aaaaaa"].Spec)
	}
	if len(d.Uncaptured) != 1 {
		t.Errorf("a milestone naming an unknown requirement is reported: %v", d.Uncaptured)
	}
}
