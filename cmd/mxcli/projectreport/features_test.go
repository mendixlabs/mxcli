// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
)

func resolved(anchor, kind string) brain.AnchorResult {
	return brain.AnchorResult{Anchor: anchor, State: brain.Resolved, Kind: kind}
}

// The feature view is derived from the plan: a slice reaches what its
// requirements' anchors lead to, and what no slice reaches is listed apart.
func TestFeatureView(t *testing.T) {
	f := mapFixture(t)
	d, err := CollectMap(f.cat, ScopeOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	orders := TraceRequirement{ID: "r1", Slice: "01-orders", Title: "Place orders", Status: StatusBuilt,
		Anchors: []brain.AnchorResult{resolved("@Sales.Order_Overview", "page")}}
	invoice := TraceRequirement{ID: "r2", Slice: "02-billing", Title: "Invoice nightly", Status: StatusBuilt,
		Anchors: []brain.AnchorResult{resolved("@Billing.ACT_Invoice", "microflow")}, Footprint: []string{"Billing.ACT_Invoice"}}
	later := TraceRequirement{ID: "r3", Slice: "02-billing", Title: "Credit notes", Status: StatusNotBuilt,
		Anchors: []brain.AnchorResult{{Anchor: "@Billing.ACT_Credit", State: brain.NotFound}}}
	trace := &TraceabilityData{Milestones: []TraceMilestone{
		{Name: "M1", Requirements: []TraceRequirement{orders}},
		{Name: "M2", Requirements: []TraceRequirement{invoice}},
		{Name: "M3", Requirements: []TraceRequirement{later}},
	}}
	AddFeatures(d, trace, map[string]string{"01-orders": "M1", "02-billing": "M2"})
	if len(d.Features) != 2 {
		t.Fatalf("one feature per slice: %+v", d.Features)
	}
	o, b := d.Features[0], d.Features[1]

	// 01-orders: the anchored page, the flows behind its buttons and their
	// closure, the page one of them opens, the entity one writes.
	if !reflect.DeepEqual(o.Pages, []string{"Admin.Purge", "Sales.Order_Overview"}) {
		t.Errorf("orders pages = %v (Order_Edit is opened by the page itself, which reach does not follow)", o.Pages)
	}
	if !reflect.DeepEqual(o.Flows, []string{"Admin.ACT_Purge", "Admin.SUB_Audit", "Core.SUB_Log", "Sales.ACT_Place", "Sales.SUB_Price"}) {
		t.Errorf("orders flows = %v", o.Flows)
	}
	if !reflect.DeepEqual(o.Entities, []string{"Sales.Order"}) || len(o.EntryPoints) != 0 {
		t.Errorf("orders entities %v, entry points %v (its own page's buttons are not a way in)", o.Entities, o.EntryPoints)
	}
	if !reflect.DeepEqual(o.Roles, []string{"Admin", "User"}) && !reflect.DeepEqual(o.Roles, []string{"User", "Admin"}) {
		t.Errorf("orders roles = %v", o.Roles)
	}

	// 02-billing: the anchored flow is core, the schedule is its way in, it
	// shares the logging helper with orders, and the unbuilt requirement keeps
	// its own milestone.
	if !reflect.DeepEqual(b.Core, []string{"Billing.ACT_Invoice"}) || b.Milestone != "M2" {
		t.Errorf("billing core %v milestone %q", b.Core, b.Milestone)
	}
	if len(b.EntryPoints) != 1 || b.EntryPoints[0].Kind != "schedule" || b.EntryPoints[0].Flow != "Billing.ACT_Invoice" {
		t.Errorf("billing entry points = %+v", b.EntryPoints)
	}
	if !reflect.DeepEqual(b.SharesWith, []string{"01-orders"}) {
		t.Errorf("billing shares with %v", b.SharesWith)
	}
	if b.Requirements[0].Milestone != "" || b.Requirements[1].Milestone != "M3" {
		t.Errorf("only a requirement placed apart names its milestone: %+v", b.Requirements)
	}
	if b.Status[StatusBuilt] != 1 || b.Status[StatusNotBuilt] != 1 {
		t.Errorf("billing status %v", b.Status)
	}

	// What no feature reaches.
	u := d.Unowned
	if !reflect.DeepEqual(u.Pages, []string{"Sales.Order_Edit", "Sales.Task_Approve"}) {
		t.Errorf("unowned pages = %v", u.Pages)
	}
	for _, want := range []string{"Core.SSO_Provision", "Billing.Worker", "Sales.REST_GetOrder"} {
		if !containsStr(u.EntryPoints, want) {
			t.Errorf("unowned entry points %v lack %s", u.EntryPoints, want)
		}
	}
	if containsStr(u.EntryPoints, "Billing.ACT_Invoice") || !reflect.DeepEqual(u.Entities, []string{"Core.Account"}) {
		t.Errorf("unowned entry points %v, entities %v", u.EntryPoints, u.Entities)
	}

	// Rendered: the markdown section and the HTML tab are there.
	env, _ := NewEnvelope("map", "Shop", "now", "t", d, MapMethod, MapLimits, nil)
	doc, err := RenderMap(env, nil)
	if err != nil {
		t.Fatal(err)
	}
	var md bytes.Buffer
	_ = RenderMarkdown(&md, doc)
	mustContain(t, md.String(), "## Features", "| 02-billing | M2 | 2 (1 built, 1 not built) |", "schedule `Billing.ACT_Invoice` (Billing.SE_Nightly)", "Reached by no feature")
	var h bytes.Buffer
	if err := RenderMapHTML(&h, env, nil); err != nil {
		t.Fatal(err)
	}
	mustContain(t, h.String(), `"features":[`, "['features','Features']")
}

// Without a plan the map is unchanged: no feature tab, no unowned list.
func TestNoPlanNoFeatureView(t *testing.T) {
	f := mapFixture(t)
	d, err := CollectMap(f.cat, ScopeOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	AddFeatures(d, nil, nil)
	AddFeatures(d, &TraceabilityData{}, nil)
	if d.Features != nil || d.Unowned != nil {
		t.Errorf("no requirements, no feature view: %+v %+v", d.Features, d.Unowned)
	}
}
