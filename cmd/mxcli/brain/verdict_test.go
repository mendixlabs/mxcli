// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"testing"
	"time"
)

func TestVerdictsParse(t *testing.T) {
	e := Entry{ID: "d1", Title: "Approval per team", Body: "Context first.\nChanges requirement `A1B2C3`: per team, not per person.\n- Partly meets requirement abcdef: web only"}
	vs := Verdicts(e)
	if len(vs) != 2 || vs[0].Verdict != VerdictChanges || vs[0].RequirementID != "a1b2c3" || vs[0].Reason != "per team, not per person." ||
		vs[1].Verdict != VerdictPartly || vs[1].RequirementID != "abcdef" {
		t.Errorf("%+v", vs)
	}
	if len(Verdicts(Entry{Body: "This changes nothing about requirement handling."})) != 0 {
		t.Error("prose is not a verdict")
	}
}

type resolveAll struct{}

func (resolveAll) Resolve(Anchor) (Resolution, error) {
	return Resolution{State: Resolved, Module: "Shop", Kind: "microflow"}, nil
}

func TestCheckReportsPerRequirementAndUnknownVerdicts(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	r1, _ := NewEntry("Place an order", []string{"@Shop.ACT_Order"}, now)
	r1.Kind, r1.Slice = KindRequirement, "01-order"
	r1.ID = r1.computeID()
	r2, _ := NewEntry("Unanchored wish", nil, now)
	r2.Kind, r2.Slice = KindRequirement, "01-order"
	r2.ID = r2.computeID()
	good, _ := NewEntry("Priced once\nPartly meets requirement `"+r1.ID+"`: no discounts", []string{"@Shop.ACT_Order"}, now)
	bad, _ := NewEntry("Bogus\nChanges requirement `ffffff`: nothing", []string{"@Shop.ACT_Order"}, now)
	for shard, es := range map[string][]Entry{PlanShard("01-order"): {r1, r2}, "Shop": {good, bad}} {
		for _, e := range es {
			if err := s.Promote(e, shard); err != nil {
				t.Fatal(err)
			}
		}
	}
	shards, _ := s.ListShards()
	rep, err := Check(s, resolveAll{}, shards)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Requirements) != 2 || !rep.Requirements[0].Built || rep.Requirements[1].Built || len(rep.Requirements[0].Anchors) != 1 {
		t.Errorf("requirements = %+v", rep.Requirements)
	}
	if len(rep.UnknownRequirementRefs) != 1 || rep.UnknownRequirementRefs[0].RequirementID != "ffffff" {
		t.Errorf("unknown refs = %+v", rep.UnknownRequirementRefs)
	}
	if !rep.Failed() {
		t.Error("a verdict on a requirement the plan does not have must fail the check")
	}
}

// ako/mxcli#2299: `brain check` passed a requirement that one decision said
// was changed and another partly met; a report then has to pick one.
func TestCheckRefusesConflictingVerdicts(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	r, _ := NewEntry("Approve leave", []string{"@Shop.ACT_Approve"}, now)
	r.Kind, r.Slice = KindRequirement, "01-leave"
	r.ID = r.computeID()
	changes, _ := NewEntry("Per team\nChanges requirement `"+r.ID+"`: per team, not per person", []string{"@Shop.ACT_Approve"}, now)
	partly, _ := NewEntry("Web only\nPartly meets requirement `"+r.ID+"`: web only", []string{"@Billing.ACT_Pay"}, now)
	again, _ := NewEntry("Also web only\nPartly meets requirement `"+r.ID+"`: no mobile", []string{"@Billing.ACT_Pay"}, now)
	for _, p := range []struct {
		e     Entry
		shard string
	}{{r, PlanShard("01-leave")}, {partly, "Billing"}, {again, "Billing"}} {
		if err := s.Promote(p.e, p.shard); err != nil {
			t.Fatal(err)
		}
	}
	shards, _ := s.ListShards()
	rep, err := Check(s, resolveAll{}, shards)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.ConflictingVerdicts) != 0 {
		t.Fatalf("two verdicts of one kind are not a conflict: %+v", rep.ConflictingVerdicts)
	}
	// The conflict spans two module files; checking only one still sees it.
	if err := s.Promote(changes, "Shop"); err != nil {
		t.Fatal(err)
	}
	rep, err = Check(s, resolveAll{}, []string{"Shop"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.ConflictingVerdicts) != 1 || rep.ConflictingVerdicts[0].RequirementID != r.ID || len(rep.ConflictingVerdicts[0].Verdicts) != 3 {
		t.Fatalf("conflicts = %+v", rep.ConflictingVerdicts)
	}
	if !rep.Failed() {
		t.Error("conflicting verdicts must fail the check")
	}
}
