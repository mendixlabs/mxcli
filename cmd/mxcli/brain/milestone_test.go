// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func milestoneStore(t *testing.T) (*Store, map[string]Entry) {
	t.Helper()
	s := NewStore(t.TempDir())
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	reqs := map[string]Entry{}
	for _, r := range []struct{ key, text, slice, anchor string }{
		{"login", "Users can log in", "01-accounts", "@Accounts.ACT_Login"},
		{"reset", "Users can reset a password", "01-accounts", "@Accounts.ACT_Reset"},
		{"offer", "Colleagues can offer a shift", "02-swaps", "@Swaps.ACT_Offer"},
		{"take", "Colleagues can take a shift", "02-swaps", "@Swaps.ACT_Take"},
		{"report", "Managers see swap totals", "03-reports", "@Swaps.Report"},
	} {
		e := mustRequirement(t, r.text, r.slice, r.anchor)
		if err := s.Promote(e, e.Shard()); err != nil {
			t.Fatal(err)
		}
		reqs[r.key] = e
	}
	return s, reqs
}

// A milestone is part of its slice, so it is written
// into the slice file, survives every rewrite of it, and comes back in the
// order the slices are sequenced.
func TestMilestonesLiveOnTheirSlices(t *testing.T) {
	s, reqs := milestoneStore(t)
	if ms, err := s.Milestones(); err != nil || ms != nil {
		t.Fatalf("a plan without milestones has none: %v %v", ms, err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.SetSliceMilestone("02-swaps", "M2 — swaps & trades"))
	must(s.SetSliceMilestone("01-accounts", "M1 — accounts"))
	must(s.SetSliceMilestone("03-reports", "M2 — swaps & trades"))
	// One requirement of slice 02 waits for the next milestone.
	must(s.SetRequirementMilestone(reqs["take"].ID, "M3 — later"))

	b, _ := os.ReadFile(s.ShardPath(PlanShard("02-swaps")))
	if !strings.HasPrefix(string(b), "---\nmilestone: \"M2 — swaps & trades\"\n---\n") {
		t.Errorf("the slice file carries its milestone in frontmatter, unescaped:\n%s", b)
	}
	// A later write to the slice keeps it.
	extra := mustRequirement(t, "Colleagues can withdraw an offer", "02-swaps", "@Swaps.ACT_Withdraw")
	must(s.Promote(extra, extra.Shard()))

	ms, err := s.Milestones()
	must(err)
	want := []Milestone{
		{Name: "M1 — accounts", Slices: []string{"01-accounts"}},
		{Name: "M2 — swaps & trades", Slices: []string{"02-swaps", "03-reports"}},
		{Name: "M3 — later", Requirements: []string{reqs["take"].ID}},
	}
	if !reflect.DeepEqual(ms, want) {
		t.Errorf("milestones:\n got %+v\nwant %+v", ms, want)
	}

	// Clearing: the slice key goes, and other frontmatter stays.
	must(s.SetRequirementMilestone(reqs["take"].ID, ""))
	e, _, _ := s.Find(reqs["take"].ID)
	if strings.Contains(e.Body, "Milestone") {
		t.Errorf("cleared requirement still names a milestone: %q", e.Body)
	}
	path := s.ShardPath(PlanShard("01-accounts"))
	b, _ = os.ReadFile(path)
	must(os.WriteFile(path, []byte(strings.Replace(string(b), "---\n", "---\ntags: [auth]\n", 1)), 0644))
	must(s.SetSliceMilestone("01-accounts", ""))
	b, _ = os.ReadFile(path)
	if !strings.HasPrefix(string(b), "---\ntags: [auth]\n---\n") || strings.Contains(string(b), "milestone") {
		t.Errorf("clearing must remove only the milestone key:\n%s", b)
	}
	must(s.SetSliceMilestone("02-swaps", ""))
	b, _ = os.ReadFile(s.ShardPath(PlanShard("02-swaps")))
	if strings.HasPrefix(string(b), "---") {
		t.Errorf("a frontmatter block with nothing left in it is dropped:\n%s", b)
	}
}

func TestMilestoneRefusals(t *testing.T) {
	s, _ := milestoneStore(t)
	if err := s.SetSliceMilestone("09-nothing", "M9"); err == nil {
		t.Error("a slice with no requirements cannot be put in a milestone")
	}
	d := mustEntry(t, "Swaps go through a manager", "@Swaps.ACT_Offer")
	if err := s.Promote(d, d.Shard()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRequirementMilestone(d.ID, "M1"); err == nil {
		t.Error("a decision is not in a milestone")
	}
	if err := s.SetSliceMilestone("01-accounts", "M`1"); err == nil {
		t.Error("a backtick would break the requirement line form")
	}
}

// The old traceability.json moves into the plan; a slice in two milestones is
// refused rather than half-imported, and what cannot be placed is reported.
func TestImportMilestones(t *testing.T) {
	s, reqs := milestoneStore(t)
	skipped, err := s.ImportMilestones([]Milestone{
		{Name: "M1", Slices: []string{"01-accounts", "07-gone"}},
		{Name: "M2", Slices: []string{"02-swaps"}, Requirements: []string{reqs["report"].ID, "ffffff"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 2 {
		t.Errorf("want the missing slice and the unknown id reported, got %v", skipped)
	}
	ms, _ := s.Milestones()
	want := []Milestone{{Name: "M1", Slices: []string{"01-accounts"}}, {Name: "M2", Slices: []string{"02-swaps"}, Requirements: []string{reqs["report"].ID}}}
	if !reflect.DeepEqual(ms, want) {
		t.Errorf("imported:\n got %+v\nwant %+v", ms, want)
	}

	s2, _ := milestoneStore(t)
	if _, err := s2.ImportMilestones([]Milestone{{Name: "A", Slices: []string{"01-accounts"}}, {Name: "B", Slices: []string{"01-accounts"}}}); err == nil {
		t.Error("a slice in two milestones must be refused")
	}
	if ms, _ := s2.Milestones(); ms != nil {
		t.Errorf("a refused import changes nothing: %+v", ms)
	}
}

func TestFrontmatterMilestoneQuoting(t *testing.T) {
	for fm, want := range map[string]string{
		"---\nmilestone: M1\n---\n":                   "M1",
		"---\nmilestone: \"M1 — a: b\"\n---\n":        "M1 — a: b",
		"---\nmilestone: 'it''s M2'\n---\n":           "it's M2",
		"---\ntitle: x\n---\n":                        "",
		"---\nmilestone: \"R&D \\u0026 more\"\n---\n": "R&D & more",
	} {
		if got := milestoneOfFrontmatter(fm); got != want {
			t.Errorf("%q: got %q, want %q", fm, got, want)
		}
	}
}
