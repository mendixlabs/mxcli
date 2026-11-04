// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
)

type listedJSON struct {
	Count   int `json:"count"`
	Entries []struct {
		ID      string   `json:"id"`
		Kind    string   `json:"kind"`
		Slice   string   `json:"slice"`
		Title   string   `json:"title"`
		Anchors []string `json:"anchors"`
		Shard   string   `json:"shard"`
	} `json:"entries"`
	Malformed []string `json:"malformed"`
}

// ako/mxcli#1952 through the CLI: the promoted side as entry objects, the same
// shape as `staged --json`, filterable, with no model needed.
func TestBrainListGivesTheCommittedStoreAsData(t *testing.T) {
	dir := t.TempDir()

	// Before init: still JSON, with an empty list rather than prose.
	var got listedJSON
	out := runBrainForTest(t, []string{"brain", "list", "-C", dir, "--json"})
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.Count != 0 || got.Entries == nil || got.Malformed == nil {
		t.Fatalf("list --json on no store: %v\n%s", err, out)
	}

	runBrainForTest(t, []string{"brain", "init", "-C", dir})
	store, queue := brain.NewStore(dir), brain.NewQueue(dir)
	dec, err := brain.NewEntry("Orders are committed by Finance", []string{"@Sales.Order"}, brainTestDay())
	if err != nil {
		t.Fatal(err)
	}
	req, err := brain.NewRequirement("A planner can create a visit", []string{"@Scheduling.Visit"}, "01-scheduling", brainTestDay())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []brain.Entry{dec, req} {
		if err := store.Promote(e, e.Shard()); err != nil {
			t.Fatal(err)
		}
	}
	// Staged, not committed: must not appear in list.
	pending, err := brain.NewEntry("Invoices are numbered per year", []string{"@Sales.Invoice"}, brainTestDay())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Append(pending); err != nil {
		t.Fatal(err)
	}

	out = runBrainForTest(t, []string{"brain", "list", "-C", dir, "--json"})
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("list --json: %v\n%s", err, out)
	}
	if got.Count != 2 || len(got.Entries) != 2 {
		t.Fatalf("want the two committed entries, not the staged one:\n%s", out)
	}
	if e := got.Entries[0]; e.ID != dec.ID || e.Kind != "decision" || e.Shard != "Sales" || len(e.Anchors) != 1 {
		t.Errorf("decision: %+v", e)
	}
	if e := got.Entries[1]; e.ID != req.ID || e.Kind != "requirement" || e.Slice != "01-scheduling" || e.Shard != brain.PlanShard("01-scheduling") {
		t.Errorf("requirement: %+v", e)
	}

	out = runBrainForTest(t, []string{"brain", "list", "-C", dir, "--slice", "01-scheduling", "--json"})
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.Count != 1 || got.Entries[0].ID != req.ID {
		t.Errorf("--slice: %v\n%s", err, out)
	}
	out = runBrainForTest(t, []string{"brain", "list", "-C", dir, "--kind", "decision", "--json"})
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.Count != 1 || got.Entries[0].ID != dec.ID {
		t.Errorf("--kind decision: %v\n%s", err, out)
	}

	// staged --json now names the kind too, so both sides read alike.
	var staged struct {
		Staged []struct {
			Kind string `json:"kind"`
		} `json:"staged"`
	}
	out = runBrainForTest(t, []string{"brain", "staged", "-C", dir, "--json"})
	if err := json.Unmarshal([]byte(out), &staged); err != nil || len(staged.Staged) != 1 || staged.Staged[0].Kind != "decision" {
		t.Errorf("staged --json kind: %v\n%s", err, out)
	}

	// Control: without --json it prints for a person.
	out = runBrainForTest(t, []string{"brain", "list", "-C", dir})
	var anyJSON map[string]any
	if json.Unmarshal([]byte(out), &anyJSON) == nil || !strings.Contains(out, "2 committed.") {
		t.Errorf("human output:\n%s", out)
	}
}
