// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"strings"
	"testing"
	"time"
)

// verdictFixture is the #2301 fixture: a verdict on a decision, one on an
// open question in a module shard, one on an open question filed on a slice,
// two of one kind on one requirement, a conflicting pair, and a verdict
// written on a requirement.
func verdictFixture(t *testing.T) (*Store, map[string]string) {
	t.Helper()
	s := NewStore(t.TempDir())
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	ids := map[string]string{}
	promote := func(e Entry, shard string) {
		t.Helper()
		if err := s.Promote(e, shard); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"decided", "questioned", "slicequestion", "twice", "conflict", "plain", "misplacer"} {
		r, err := NewRequirement("Requirement "+name, []string{"@Shop.ACT_" + name}, "01-shop", now)
		if err != nil {
			t.Fatal(err)
		}
		ids[name] = r.ID
	}
	line := func(kind, name, reason string) string {
		l, err := VerdictLine(kind, ids[name], reason)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	misplacer, _ := NewRequirement("Requirement misplacer\n"+line(VerdictPartly, "plain", "says so itself"), []string{"@Shop.ACT_misplacer"}, "01-shop", now)
	for _, name := range []string{"decided", "questioned", "slicequestion", "twice", "conflict", "plain"} {
		r, _ := NewRequirement("Requirement "+name, []string{"@Shop.ACT_" + name}, "01-shop", now)
		promote(r, PlanShard("01-shop"))
	}
	promote(misplacer, PlanShard("01-shop"))
	ids["misplacer"] = misplacer.ID

	dec, _ := NewEntry("Priced once\n"+line(VerdictPartly, "decided", "no discounts yet"), []string{"@Shop.ACT_decided"}, now)
	q, _ := NewQuestion("Which feeds?\n"+line(VerdictPartly, "questioned", "until answered: only the public feed"), []string{"@Shop.ACT_questioned"}, "", now)
	sq, _ := NewQuestion("Is mobile in scope?\n"+line(VerdictChanges, "slicequestion", "web only until decided"), nil, "01-shop", now)
	t1, _ := NewEntry("Partly A\n"+line(VerdictPartly, "twice", "reason A"), []string{"@Shop.ACT_twice"}, now)
	t2, _ := NewEntry("Partly B\n"+line(VerdictPartly, "twice", "reason B"), []string{"@Shop.ACT_twice"}, now)
	c1, _ := NewEntry("Changed\n"+line(VerdictChanges, "conflict", "per team"), []string{"@Shop.ACT_conflict"}, now)
	c2, _ := NewEntry("Partly\n"+line(VerdictPartly, "conflict", "web only"), []string{"@Shop.ACT_conflict"}, now)
	for _, e := range []Entry{dec, q, t1, t2, c1, c2} {
		promote(e, "Shop")
	}
	promote(sq, PlanShard("01-shop"))
	ids["slicequestion-entry"] = sq.ID
	return s, ids
}

func TestVerdictsDeriveTheEffectiveStatus(t *testing.T) {
	s, ids := verdictFixture(t)
	shards, _ := s.ListShards()
	rep, err := Check(s, resolveAll{}, shards)
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]string{}
	for _, r := range rep.Requirements {
		status[r.ID] = r.Status + " | " + r.StatusReason
	}
	for name, want := range map[string]string{
		"decided":       "partial | decision ",
		"questioned":    "partial | open question ",
		"slicequestion": "changed | open question ",
		"twice":         "partial | decision ",
		"conflict":      "changed | decision ",
		"plain":         "built | ",
		"misplacer":     "built | ",
	} {
		if !strings.HasPrefix(status[ids[name]], want) {
			t.Errorf("%s: status %q, want prefix %q", name, status[ids[name]], want)
		}
	}
	// Same kind twice is allowed; changes + partly is a conflict; a verdict on
	// a requirement is misplaced and does not count.
	if len(rep.ConflictingVerdicts) != 1 || rep.ConflictingVerdicts[0].RequirementID != ids["conflict"] {
		t.Errorf("conflicts = %+v", rep.ConflictingVerdicts)
	}
	if len(rep.MisplacedVerdicts) != 1 || rep.MisplacedVerdicts[0].EntryID != ids["misplacer"] {
		t.Errorf("misplaced = %+v", rep.MisplacedVerdicts)
	}
	if len(rep.UnknownRequirementRefs) != 0 {
		t.Errorf("unknown = %+v", rep.UnknownRequirementRefs)
	}
	if !rep.Failed() {
		t.Error("a conflict and a misplaced verdict fail the check")
	}
}

func TestListExposesVerdicts(t *testing.T) {
	s, ids := verdictFixture(t)
	res, err := s.List(ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range res.Entries {
		if e.ID == ids["slicequestion-entry"] {
			found = true
			if len(e.Verdicts) != 1 || e.Verdicts[0].Requirement != ids["slicequestion"] || e.Verdicts[0].Kind != VerdictChanges ||
				e.Verdicts[0].Reason != "web only until decided" {
				t.Errorf("verdicts = %+v", e.Verdicts)
			}
		}
	}
	if !found {
		t.Fatal("the slice question is not listed")
	}
}

func TestAddAndStripVerdicts(t *testing.T) {
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	e, _ := NewEntry("Per team approval", []string{"@Shop.ACT_Approve"}, now)
	e, err := AddVerdicts(e, VerdictChanges, []string{"A1B2C3:per team, not per person"})
	if err != nil {
		t.Fatal(err)
	}
	again, _ := AddVerdicts(e, VerdictChanges, []string{"a1b2c3:per team, not per person"})
	if again.Body != e.Body || e.Body != "Changes requirement `a1b2c3`: per team, not per person" {
		t.Errorf("body = %q / re-run %q", e.Body, again.Body)
	}
	if _, err := AddVerdicts(e, VerdictPartly, []string{"nothex"}); err == nil {
		t.Error("a malformed id is refused")
	}
	req, _ := NewRequirement("A requirement", []string{"@Shop.X"}, "01", now)
	if _, err := AddVerdicts(req, VerdictPartly, []string{"a1b2c3"}); err == nil {
		t.Error("a verdict on a requirement is refused at capture")
	}

	// Resolving a slice question turns it into a requirement: its verdict
	// lines are dropped, the rest of the body kept.
	q, _ := NewQuestion("Is mobile in scope?\nContext line.\nChanges requirement `a1b2c3`: web only", nil, "01", now)
	resolved, _ := q.Resolve("Yes, in M3", now)
	stripped, dropped := StripVerdicts(resolved)
	if len(dropped) != 1 || strings.Contains(stripped.Body, "requirement `a1b2c3`") || !strings.Contains(stripped.Body, "Context line.") {
		t.Errorf("stripped %q, dropped %+v", stripped.Body, dropped)
	}
}
