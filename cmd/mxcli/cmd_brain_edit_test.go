// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
)

// The issue's proof, through the CLI (ako/mxcli#1950): edit a promoted
// requirement, and the id is unchanged, the shard holds the new text once and
// the old nowhere, and check still passes.
func TestBrainEditKeepsTheHandleEndToEnd(t *testing.T) {
	dir := t.TempDir()
	runBrainForTest(t, []string{"brain", "init", "-C", dir})
	const before = "A planner can create a visit for a site with a window and one engineer"
	const after = "A planner can create a visit for a site with a window and one named engineer"

	out := runBrainForTest(t, []string{"brain", "capture", before, "-a", "@Scheduling.Visit", "-s", "01-scheduling", "-C", dir})
	id := strings.Fields(out)[1]
	runBrainForTest(t, []string{"brain", "promote", id, "-C", dir})

	var edited struct {
		ID    string `json:"id"`
		Shard string `json:"shard"`
	}
	out = runBrainForTest(t, []string{"brain", "edit", id, after, "-C", dir, "--json"})
	if err := json.Unmarshal([]byte(out), &edited); err != nil {
		t.Fatalf("edit --json: %v\n%s", err, out)
	}
	if edited.ID != id || edited.Shard != brain.PlanShard("01-scheduling") {
		t.Errorf("edit --json = %+v", edited)
	}

	raw, err := os.ReadFile(brain.NewStore(dir).ShardPath(brain.PlanShard("01-scheduling")))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), after) != 1 || strings.Contains(string(raw), "and one engineer") || !strings.Contains(string(raw), id) {
		t.Errorf("shard after edit:\n%s", raw)
	}

	out = runBrainForTest(t, []string{"brain", "check", "-C", dir})
	if !strings.Contains(out, "OK") {
		t.Errorf("check after edit:\n%s", out)
	}

	// The edited wording again is a duplicate of what is committed...
	out = runBrainForTest(t, []string{"brain", "capture", after, "-a", "@Scheduling.Visit", "-s", "01-scheduling", "-C", dir})
	if !strings.Contains(out, "Already committed as "+id) {
		t.Errorf("re-capturing the edited wording:\n%s", out)
	}
	// ...and the original wording is a new fact with an id of its own, not
	// the edited entry's handle, which it hashes to.
	out = runBrainForTest(t, []string{"brain", "capture", before, "-a", "@Scheduling.Visit", "-s", "01-scheduling", "-C", dir})
	fresh := strings.Fields(out)[1]
	if !strings.HasPrefix(out, "Queued") || fresh == id {
		t.Errorf("re-capturing the original wording:\n%s", out)
	}
	runBrainForTest(t, []string{"brain", "promote", fresh, "-C", dir})

	// A staged entry is edited in the queue, in place.
	out = runBrainForTest(t, []string{"brain", "capture", "Visits are planned by the planner", "-a", "@Scheduling.Visit", "-C", dir})
	staged := strings.Fields(out)[1]
	out = runBrainForTest(t, []string{"brain", "edit", staged, "Visits are planned by the planner, never the engineer", "-C", dir})
	if !strings.Contains(out, "Edited staged "+staged) {
		t.Errorf("editing a staged entry:\n%s", out)
	}

	// -a with no text re-anchors and keeps the wording.
	out = runBrainForTest(t, []string{"brain", "capture", "Visits are committed by the planner", "-a", "@Scheduling.Visit", "-C", dir})
	dec := strings.Fields(out)[1]
	runBrainForTest(t, []string{"brain", "promote", dec, "-C", dir})
	out = runBrainForTest(t, []string{"brain", "edit", dec, "-a", "@Dispatch.ACT_Plan", "-C", dir})
	if !strings.Contains(out, "moved it from Scheduling.md to Dispatch.md") {
		t.Errorf("re-anchor without text:\n%s", out)
	}
	if e, _, _ := brain.NewStore(dir).Find(dec); e.Title != "Visits are committed by the planner" {
		t.Errorf("re-anchoring changed the wording to %q", e.Title)
	}

	// move keeps the id and changes the file.
	out = runBrainForTest(t, []string{"brain", "move", id, "--slice", "02-dispatch", "-C", dir})
	if !strings.Contains(out, "moved it from plan/01-scheduling.md to plan/02-dispatch.md") {
		t.Errorf("move:\n%s", out)
	}
	if _, shard, _ := brain.NewStore(dir).Find(id); shard != brain.PlanShard("02-dispatch") {
		t.Errorf("after move, %s is in %q", id, shard)
	}
}
