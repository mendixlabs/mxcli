// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// ako/mxcli#1952: a structured read of the committed store, so a UI renders
// entries instead of parsing markdown whose format mxcli owns.

func listFixture(t *testing.T) (s *Store, dec, question, reqA, reqB Entry) {
	t.Helper()
	s = NewStore(t.TempDir())
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	dec = mustEntry(t, "Orders are committed by Finance", "@Sales.Order")
	q, err := NewQuestion("Do approvers see rejected orders?", nil, "", day)
	if err != nil {
		t.Fatal(err)
	}
	question = q
	reqA = mustRequirement(t, "A planner can create a visit", "01-scheduling", "@Scheduling.Visit")
	reqB = mustRequirement(t, "A dispatcher can assign a visit", "02-dispatch", "@Dispatch.ACT_Assign")
	if err := s.Promote(dec, "Sales"); err != nil {
		t.Fatal(err)
	}
	if err := s.Promote(question, ProjectShard); err != nil {
		t.Fatal(err)
	}
	promoteAll(t, s, reqA, reqB)
	return
}

func ids(r ListResult) []string {
	var out []string
	for _, e := range r.Entries {
		out = append(out, e.ID)
	}
	return out
}

func TestListFiltersTheCommittedStore(t *testing.T) {
	s, dec, question, reqA, reqB := listFixture(t)
	yes, no := true, false

	for _, tc := range []struct {
		name string
		f    ListFilter
		want []string
	}{
		// Store order: project, modules, slices; file order within each.
		{"everything", ListFilter{}, []string{question.ID, dec.ID, reqA.ID, reqB.ID}},
		{"one slice", ListFilter{Slice: "02-dispatch"}, []string{reqB.ID}},
		{"requirements", ListFilter{Kind: KindRequirement}, []string{reqA.ID, reqB.ID}},
		{"decisions", ListFilter{Kind: KindDecision}, []string{question.ID, dec.ID}},
		{"open only", ListFilter{Open: &yes}, []string{question.ID}},
		{"settled only", ListFilter{Open: &no}, []string{dec.ID, reqA.ID, reqB.ID}},
		{"no such slice", ListFilter{Slice: "99-none"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.List(tc.f)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(ids(got), tc.want) {
				t.Errorf("ids = %v, want %v", ids(got), tc.want)
			}
		})
	}

	// Each entry carries its shard and an explicit kind — a consumer must not
	// have to know that an empty kind means "decision".
	got, err := s.List(ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range got.Entries {
		if e.Kind == "" || e.Shard == "" {
			t.Errorf("%s: kind %q shard %q", e.ID, e.Kind, e.Shard)
		}
	}
	if got.Entries[1].Shard != "Sales" || got.Entries[2].Shard != PlanShard("01-scheduling") {
		t.Errorf("shards: %+v", got.Entries)
	}
}

// A block whose metadata line cannot be read is reported, not dropped — or a
// UI silently shows fewer entries than the file holds.
func TestListReportsMalformedBlocks(t *testing.T) {
	s, _, _, reqA, _ := listFixture(t)
	path := s.ShardPath(reqA.Shard())
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, []byte("\n## Hand-written, no metadata\n\nSome prose.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.List(ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Malformed) != 1 || !strings.Contains(got.Malformed[0], "Hand-written") || !strings.HasPrefix(got.Malformed[0], reqA.Shard()) {
		t.Errorf("malformed = %v", got.Malformed)
	}
	if len(got.Entries) != 4 {
		t.Errorf("the well-formed entries must still be listed: %v", ids(got))
	}

	// Malformed blocks are reported only for the shards the filter covers.
	got, err = s.List(ListFilter{Slice: "02-dispatch"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Malformed) != 0 {
		t.Errorf("a malformed block outside the filtered slice was reported: %v", got.Malformed)
	}
}

func TestListRefusesAFilterThatCannotMatch(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.List(ListFilter{Slice: "01-scheduling", Kind: KindDecision}); err == nil {
		t.Error("--slice with --kind decision must be refused: a decision has no slice")
	}
	if _, err := s.List(ListFilter{Kind: "story"}); err == nil {
		t.Error("an unknown kind must be refused, not match nothing")
	}
	// An uninitialised store lists nothing; it is not an error.
	if got, err := s.List(ListFilter{}); err != nil || len(got.Entries) != 0 {
		t.Errorf("empty store: %+v, %v", got, err)
	}
}
