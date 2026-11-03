// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"testing"
)

// With no model there is nothing to check an anchor against. The honest
// answers are "nothing is built" for a requirement and "could not check" for a
// decision — not "built" and not "stale" (ako/mxcli#1948).
//
// The requirement half is the trap: checkSlice counts every state other than
// NotFound as built, so that a NotIndexable anchor does not report finished
// work as outstanding. A new state added without touching that branch reports
// a project that does not exist yet as 100% built.
func TestNoModelReportsNothingBuiltAndFailsNothing(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	req := mustRequirement(t, "A planner can create a visit", "01-scheduling", "@Scheduling.Visit")
	unanchored := mustRequirement(t, "It should feel fast", "01-scheduling")
	dec := mustEntry(t, "Visits are committed by the planner", "@Scheduling.Visit")
	for _, e := range []Entry{req, unanchored} {
		if err := s.Promote(e, e.Shard()); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Promote(dec, "Scheduling"); err != nil {
		t.Fatal(err)
	}
	shards := []string{"Scheduling", req.Shard()}

	rep, err := Check(s, NoModel{}, shards)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Slices) != 1 {
		t.Fatalf("slices = %+v", rep.Slices)
	}
	if got := rep.Slices[0]; got.Built != 0 || got.Planned != 1 || got.Unanchored != 1 {
		t.Errorf("no model: %+v, want Built 0, Planned 1, Unanchored 1", got)
	}
	if rep.Failed() {
		t.Errorf("an unverifiable anchor must not fail the check: %+v", rep)
	}
	if rep.Unverifiable != 2 {
		t.Errorf("unverifiable = %d, want 2 (one requirement anchor, one decision anchor)", rep.Unverifiable)
	}
	for _, f := range rep.Findings {
		if f.State == NotFound {
			t.Errorf("no model must not be reported as NOT FOUND: %+v", f)
		}
	}
	if rep.ResolvedN != 0 {
		t.Errorf("resolved = %d with no model, want 0", rep.ResolvedN)
	}

	// Control: the same store against a model that lacks the element. The
	// decision is stale and fails — so the pass above is the new state's
	// doing, not a check that has stopped failing anything.
	rep, err = Check(s, stubResolver{}, shards)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Failed() {
		t.Error("control: a decision anchored at a missing element must still fail when there is a model")
	}
	if rep.Unverifiable != 0 {
		t.Errorf("control: unverifiable = %d with a model, want 0", rep.Unverifiable)
	}

	// And the requirement flips to built once the model has the element.
	rep, err = Check(s, stubResolver{"@Scheduling.Visit": {State: Resolved, Module: "Scheduling", Kind: "entity"}}, shards)
	if err != nil {
		t.Fatal(err)
	}
	if got := rep.Slices[0]; got.Built != 1 || got.Planned != 0 {
		t.Errorf("with the element present: %+v, want Built 1", got)
	}
}

// The state names itself and round-trips, rather than falling through to
// "not indexable" in String() and being rejected by UnmarshalJSON.
func TestUnverifiableStateRoundTrips(t *testing.T) {
	if got := Unverifiable.String(); got != "unverifiable" {
		t.Errorf("String() = %q", got)
	}
	b, err := json.Marshal(Unverifiable)
	if err != nil {
		t.Fatal(err)
	}
	var back AnchorState
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal %s: %v", b, err)
	}
	if back != Unverifiable {
		t.Errorf("round trip gave %v", back)
	}
}
