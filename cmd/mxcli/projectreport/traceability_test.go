// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"reflect"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
	"github.com/mendixlabs/mxcli/mdl/catalog"
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

// ako/mxcli#2299: requirements anchored on a published REST service, a Java
// action or an attribute got no footprint; and a verdict in an open question
// was ignored although `brain check` validates it there.
func TestTraceabilityFootprintKindsAndQuestionVerdicts(t *testing.T) {
	f := newFixture(t)
	f.module("Agenda", "")
	for _, m := range []string{"Agenda.REST_Feed", "Agenda.ACT_Search", "Agenda.ACT_Edit", "Agenda.ACT_Publish"} {
		f.microflow(m, true, "")
	}
	f.exec(`INSERT INTO published_rest_operations_data (Id, ServiceQualifiedName, ResourceName, HttpMethod, Path, Microflow, ModuleName)
		VALUES ('op', 'Agenda.PRS_Agenda', 'feed', 'GET', '/feed', 'Agenda.REST_Feed', 'Agenda')`)
	// The refs row as the catalog names it: space-separated.
	f.ref("Agenda.PRS_Agenda feed GET /feed", "PUBLISHED_REST_OPERATION", "Agenda.REST_Feed", "MICROFLOW", "publish")
	f.ref("Agenda.ACT_Search", "MICROFLOW", "Agenda.JA_Search", "JAVA_ACTION", "call")
	f.ref("Agenda.ACT_Edit", "MICROFLOW", "Agenda.Item.Title", "ATTRIBUTE", "member")
	f.ref("Agenda.ACT_Publish", "MICROFLOW", "Agenda.Item", "ENTITY", "change")

	in := TraceabilityInput{
		Requirements: []brain.RequirementResult{
			req("aaaaaa", "01", "Calendar feed", true, anchor("@Agenda.PRS_Agenda", "published_rest_service", brain.Resolved)),
			req("bbbbbb", "01", "Chain search", true, anchor("@Agenda.JA_Search", "java_action", brain.Resolved)),
			req("cccccc", "01", "Edit title", true, anchor("@Agenda.Item.Title", "attribute", brain.Resolved)),
			req("dddddd", "01", "Public text", true, anchor("@Agenda.Item.PublicText", "attribute", brain.Resolved)),
		},
		Entries: []brain.ListedEntry{
			listed("q00001", "Agenda", "", true, "Which feeds does HR need?", "Partly meets requirement `aaaaaa` until answered: only the public feed."),
		},
	}
	d, err := CollectTraceability(f.cat, in)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]TraceRequirement{}
	for _, r := range d.Milestones[0].Requirements {
		got[r.ID] = r
	}
	for id, want := range map[string][]string{
		"aaaaaa": {"Agenda.REST_Feed"},
		"bbbbbb": {"Agenda.ACT_Search"},
		"cccccc": {"Agenda.ACT_Edit"},
		"dddddd": {"Agenda.ACT_Publish"},
	} {
		if !reflect.DeepEqual(got[id].Footprint, want) {
			t.Errorf("%s footprint = %v, want %v", id, got[id].Footprint, want)
		}
	}
	if got["cccccc"].FootprintApprox || !got["dddddd"].FootprintApprox {
		t.Errorf("only the attribute no microflow names is approximate: %v / %v", got["cccccc"].FootprintApprox, got["dddddd"].FootprintApprox)
	}
	if got["aaaaaa"].Status != StatusPartial || got["aaaaaa"].StatusReason != "open question q00001: until answered: only the public feed." {
		t.Errorf("status = %q (%q)", got["aaaaaa"].Status, got["aaaaaa"].StatusReason)
	}
}

// recordingQuerier counts the queries a footprint makes.
type recordingQuerier struct {
	Querier
	n int
}

func (r *recordingQuerier) Query(sql string) (*catalog.QueryResult, error) {
	r.n++
	return r.Querier.Query(sql)
}

// ako/mxcli#2303: `brain check` warns on anchors no test can reach, using
// brain's list of provable kinds. Every kind on that list must have a footprint
// here, or the check and the report disagree about which requirements a test
// can prove. "microflow" uses the call graph, not a query.
func TestProvableKindsHaveAFootprint(t *testing.T) {
	f := newFixture(t)
	for _, kind := range brain.ProvableKinds() {
		if kind == "microflow" {
			continue
		}
		rq := &recordingQuerier{Querier: f.cat}
		if _, _, err := footprint(rq, anchor("@M.X", kind, brain.Resolved), map[string]string{}, CallGraph{}); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if rq.n == 0 {
			t.Errorf("kind %q is provable in brain but the report computes no footprint for it", kind)
		}
	}
	rq := &recordingQuerier{Querier: f.cat}
	_, _, _ = footprint(rq, anchor("@M.R", "module role", brain.Resolved), map[string]string{}, CallGraph{})
	if rq.n != 0 {
		t.Error("control: an unprovable kind has no footprint query")
	}
}

type resolveEverything struct{}

func (resolveEverything) Resolve(a brain.Anchor) (brain.Resolution, error) {
	return brain.Resolution{State: brain.Resolved, Module: a.Module, Kind: "microflow"}, nil
}

// ako/mxcli#2301: `brain plan --json` and `report traceability` report the
// same status for every requirement — the report reads brain's.
func TestTraceabilityAgreesWithBrainPlan(t *testing.T) {
	s := brain.NewStore(t.TempDir())
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	var reqs []brain.Entry
	for _, n := range []string{"Decided", "Questioned", "Plain"} {
		r, err := brain.NewRequirement("Requirement "+n, []string{"@Shop.ACT_" + n}, "01", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Promote(r, brain.PlanShard("01")); err != nil {
			t.Fatal(err)
		}
		reqs = append(reqs, r)
	}
	unanchored, _ := brain.NewRequirement("Wish", nil, "01", now)
	_ = s.Promote(unanchored, brain.PlanShard("01"))
	dec, _ := brain.NewEntry("No discounts\nPartly meets requirement `"+reqs[0].ID+"`: no discounts", []string{"@Shop.ACT_Decided"}, now)
	q, _ := brain.NewQuestion("Which feeds?\nChanges requirement `"+reqs[1].ID+"` until answered: public only", []string{"@Shop.ACT_Questioned"}, "", now)
	_ = s.Promote(dec, "Shop")
	_ = s.Promote(q, "Shop")

	slices, _ := s.ListSlices()
	rep, err := brain.Check(s, resolveEverything{}, slices)
	if err != nil {
		t.Fatal(err)
	}
	list, _ := s.List(brain.ListFilter{})
	f := newFixture(t)
	d, err := CollectTraceability(f.cat, TraceabilityInput{Requirements: rep.Requirements, Entries: list.Entries})
	if err != nil {
		t.Fatal(err)
	}
	fromReport := map[string]string{}
	for _, m := range d.Milestones {
		for _, r := range m.Requirements {
			fromReport[r.ID] = r.Status
		}
	}
	want := map[string]string{reqs[0].ID: StatusPartial, reqs[1].ID: StatusChanged, reqs[2].ID: StatusBuilt, unanchored.ID: StatusNotTraceable}
	for _, r := range rep.Requirements {
		if r.Status != want[r.ID] {
			t.Errorf("brain: %s = %q, want %q", r.Title, r.Status, want[r.ID])
		}
		if fromReport[r.ID] != r.Status {
			t.Errorf("report says %q, brain plan %q for %s", fromReport[r.ID], r.Status, r.Title)
		}
	}
}
