// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"strings"
	"testing"
)

// ako/mxcli#1950: an entry's id is a handle, so changing its wording must not
// change its identity, orphan it, or leave the old text behind.

func promoteAll(t *testing.T, s *Store, es ...Entry) {
	t.Helper()
	for _, e := range es {
		if err := s.Promote(e, e.Shard()); err != nil {
			t.Fatal(err)
		}
	}
}

func editTo(text string) func(Entry) (Entry, error) {
	return func(e Entry) (Entry, error) { return e.Edit(text, nil, false) }
}

func TestEditKeepsTheIdAndReplacesTheTextOnce(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	const before = "A planner can create a visit for a site with a window and one engineer"
	const after = "A planner can create a visit for a site with a window and one named engineer"
	req := mustRequirement(t, before, "01-scheduling", "@Scheduling.Visit")
	other := mustRequirement(t, "A planner can cancel a visit", "01-scheduling", "@Scheduling.ACT_Cancel")
	promoteAll(t, s, req, other)

	res, err := s.Update(req.ID, editTo(after))
	if err != nil {
		t.Fatal(err)
	}
	if res.From != req.Shard() || res.To != req.Shard() {
		t.Errorf("an edit that changes only wording must not move: %+v", res)
	}

	entries, _, err := s.LoadShard(req.Shard())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].ID != req.ID || entries[0].Title != after {
		t.Fatalf("after edit: %+v — want the same id, in the same position, with the new text", entries)
	}
	if entries[1].ID != other.ID {
		t.Errorf("the other entry was disturbed: %+v", entries[1])
	}
	raw, err := os.ReadFile(s.ShardPath(req.Shard()))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), after); n != 1 {
		t.Errorf("new text appears %d times, want 1", n)
	}
	if strings.Contains(string(raw), "and one engineer") {
		t.Error("the old text is still in the shard")
	}

	rep, err := Check(s, stubResolver{}, []string{req.Shard()})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed() || rep.Slices[0].Planned != 2 {
		t.Errorf("check after edit: %+v", rep)
	}
}

// Once edited, an entry's stored id no longer matches its text, so a duplicate
// check that compares stored ids is blind in both directions. Both are asserted.
func TestDuplicatesAreFoundByContentNotByStoredID(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	original := mustRequirement(t, "A planner can create a visit", "01-scheduling", "@Scheduling.Visit")
	promoteAll(t, s, original)
	if _, err := s.Update(original.ID, editTo("A planner can create a visit for one engineer")); err != nil {
		t.Fatal(err)
	}

	// 1. The edited wording, captured again, is a duplicate — though its
	//    natural id differs from the stored one.
	again := mustRequirement(t, "A planner can create a visit for one engineer", "01-scheduling", "@Scheduling.Visit")
	if again.ID == original.ID {
		t.Fatal("test premise: the edited text must hash to a different id")
	}
	if err := s.Promote(again, again.Shard()); err == nil {
		t.Error("promoting the edited wording a second time was accepted: duplicate check compares stored ids")
	}

	// 2. The ORIGINAL wording hashes to the edited entry's handle. It is a
	//    different fact now, so it is not a duplicate — but it must not
	//    share the id either, or one handle names two entries.
	old := mustRequirement(t, "A planner can create a visit", "01-scheduling", "@Scheduling.Visit")
	if old.ID != original.ID {
		t.Fatal("test premise: the original text must hash to the edited entry's id")
	}
	taken, err := TakenIDs(s, NewQueue(dir))
	if err != nil {
		t.Fatal(err)
	}
	fresh := AssignFreeID(old, taken)
	if fresh.ID == original.ID {
		t.Fatal("the original wording was given the edited entry's id")
	}
	if fresh.ContentKey() != old.ContentKey() {
		t.Error("assigning a free id must not change the content key")
	}
	if err := s.Promote(fresh, fresh.Shard()); err != nil {
		t.Errorf("the original wording, re-captured, is a new fact and must promote: %v", err)
	}

	// Control: with nothing edited, AssignFreeID is the identity — the
	// content-derived id still IS the id for every entry never changed.
	untouched := mustRequirement(t, "A planner can delete a visit", "01-scheduling", "@Scheduling.ACT_Delete")
	if got := AssignFreeID(untouched, taken); got.ID != untouched.ID {
		t.Errorf("an untaken id was changed: %s -> %s", untouched.ID, got.ID)
	}
}

func TestEditRefusesToTurnAnEntryIntoAnotherOne(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	a := mustRequirement(t, "A planner can create a visit", "01-scheduling", "@Scheduling.Visit")
	b := mustRequirement(t, "A planner can cancel a visit", "01-scheduling", "@Scheduling.Visit")
	promoteAll(t, s, a, b)

	_, err := s.Update(b.ID, editTo("A planner can create a visit"))
	if err == nil || !strings.Contains(err.Error(), a.ID) {
		t.Errorf("err = %v, want a refusal naming %s", err, a.ID)
	}
}

// A decision's anchors decide its file. Re-anchoring one into another module
// must move it there, or `check` reports it misfiled the moment it runs.
func TestReanchoringADecisionMovesItToItsShard(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	dec := mustEntry(t, "Orders are committed by Finance", "@Sales.Order")
	promoteAll(t, s, dec)

	res, err := s.Update(dec.ID, func(e Entry) (Entry, error) {
		return e.Edit(e.Title, []string{"@Finance.ACT_Post"}, true)
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.From != "Sales" || res.To != "Finance" {
		t.Errorf("moved %s -> %s, want Sales -> Finance", res.From, res.To)
	}
	if e, shard, _ := s.Find(dec.ID); shard != "Finance" || e.ID != dec.ID {
		t.Errorf("Find after re-anchor: %q in %s", e.ID, shard)
	}
	rep, err := Check(s, stubResolver{"@Finance.ACT_Post": {State: Resolved, Module: "Finance"}}, []string{"Finance"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed() {
		t.Errorf("check after re-anchor: %+v", rep)
	}

	// Control: a decision put somewhere on purpose (promote --to project)
	// stays there when its anchors change. Following the anchors only applies
	// to an entry that was where its anchors put it.
	pinned := mustEntry(t, "Every module logs to the same node", "@Sales.Order")
	if err := s.Promote(pinned, ProjectShard); err != nil {
		t.Fatal(err)
	}
	res, err = s.Update(pinned.ID, func(e Entry) (Entry, error) {
		return e.Edit(e.Title, []string{"@Finance.Order"}, true)
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.To != ProjectShard {
		t.Errorf("a decision placed in project.md on purpose moved to %s", res.To)
	}
}

func TestMoveChangesARequirementsSliceAndKeepsItsID(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	req := mustRequirement(t, "A planner can create a visit", "01-scheduling", "@Scheduling.Visit")
	promoteAll(t, s, req)

	res, err := s.Update(req.ID, func(e Entry) (Entry, error) { return e.MoveToSlice("02-dispatch") })
	if err != nil {
		t.Fatal(err)
	}
	if res.To != PlanShard("02-dispatch") {
		t.Errorf("moved to %s", res.To)
	}
	e, shard, _ := s.Find(req.ID)
	if shard != PlanShard("02-dispatch") || e.Slice != "02-dispatch" {
		t.Errorf("after move: %+v in %s", e, shard)
	}
	if _, err := os.Stat(s.ShardPath(req.Shard())); !os.IsNotExist(err) {
		t.Error("the emptied slice file was left behind")
	}

	// A decision has no slice; turning one into a requirement is a change of
	// meaning (its anchors would stop failing when they go stale), not a move.
	dec := mustEntry(t, "Orders are committed by Finance", "@Sales.Order")
	if _, err := dec.MoveToSlice("01-scheduling"); err == nil {
		t.Error("moving a decision into a slice must be refused")
	}
}

func TestEditValidatesLikeCapture(t *testing.T) {
	req := mustRequirement(t, "A planner can create a visit", "01-scheduling", "@Scheduling.Visit")
	if _, err := req.Edit("  ", nil, false); err == nil {
		t.Error("an empty title must be refused")
	}
	if _, err := req.Edit(req.Title, []string{"@Scheduling"}, true); err == nil {
		t.Error("a requirement re-anchored at a bare module must be refused, as capture refuses it")
	}
	got, err := req.Edit("New wording\n\nWith a body.", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != req.ID || got.Slice != req.Slice || got.Kind != req.Kind || got.Date != req.Date ||
		got.Title != "New wording" || got.Body != "With a body." || len(got.Anchors) != 1 {
		t.Errorf("edit changed more than the text: %+v", got)
	}
}

func TestQueueUpdateEditsAStagedEntryInPlace(t *testing.T) {
	q := NewQueue(t.TempDir())
	a := mustEntry(t, "Orders are committed by Finance", "@Sales.Order")
	b := mustEntry(t, "Invoices are numbered per year", "@Sales.Invoice")
	for _, e := range []Entry{a, b} {
		if _, err := q.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	ok, err := q.Update(a.ID, editTo("Orders are committed by Finance, never by Sales"))
	if err != nil || !ok {
		t.Fatalf("Update = %v, %v", ok, err)
	}
	got, err := q.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != a.ID || got[0].Title != "Orders are committed by Finance, never by Sales" || got[1].ID != b.ID {
		t.Errorf("queue after edit: %+v", got)
	}
	// The queue's own duplicate check is by content too.
	if added, _ := q.Append(mustEntry(t, "Orders are committed by Finance, never by Sales", "@Sales.Order")); added {
		t.Error("the edited wording was queued a second time")
	}
}
