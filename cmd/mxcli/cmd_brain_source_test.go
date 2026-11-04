// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
)

// ako/mxcli#1951 through the CLI: --source is stored relative to the project,
// rendered into the shard, present in the JSON, and kept by an edit.
func TestCaptureRecordsWhereARequirementCameFrom(t *testing.T) {
	dir := t.TempDir()
	prd := filepath.Join(dir, "docs", "sample-prd.md")
	if err := os.MkdirAll(filepath.Dir(prd), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prd, []byte("# PRD\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runBrainForTest(t, []string{"brain", "init", "-C", dir})

	// An absolute path in, a project-relative one stored: the shard must mean
	// the same thing on another machine.
	out := runBrainForTest(t, []string{"brain", "capture", "A planner can create a visit",
		"-a", "@Scheduling.Visit", "-s", "01-scheduling", "--source", prd + "#L18-21", "-C", dir})
	id := strings.Fields(out)[1]

	var staged struct {
		Staged []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"staged"`
	}
	out = runBrainForTest(t, []string{"brain", "staged", "-C", dir, "--json"})
	if err := json.Unmarshal([]byte(out), &staged); err != nil {
		t.Fatal(err)
	}
	if len(staged.Staged) != 1 || staged.Staged[0].Source != "docs/sample-prd.md#L18-21" {
		t.Fatalf("staged --json: %s", out)
	}

	runBrainForTest(t, []string{"brain", "promote", id, "-C", dir})
	shard := brain.NewStore(dir).ShardPath(brain.PlanShard("01-scheduling"))
	raw, err := os.ReadFile(shard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Source: `docs/sample-prd.md#L18-21`") || strings.Contains(string(raw), dir) {
		t.Errorf("shard:\n%s", raw)
	}
	if out := runBrainForTest(t, []string{"brain", "check", "-C", dir}); !strings.Contains(out, "OK") {
		t.Errorf("check with the source present:\n%s", out)
	}

	// An edit of the wording keeps the source; --source replaces it.
	runBrainForTest(t, []string{"brain", "edit", id, "A planner can create a visit for one engineer", "-C", dir})
	if e, _, _ := brain.NewStore(dir).Find(id); e.Source != "docs/sample-prd.md#L18-21" {
		t.Errorf("edit dropped the source: %q", e.Source)
	}
	runBrainForTest(t, []string{"brain", "edit", id, "--source", prd + "#Goals", "-C", dir})
	if e, _, _ := brain.NewStore(dir).Find(id); e.Source != "docs/sample-prd.md#Goals" || e.Title != "A planner can create a visit for one engineer" {
		t.Errorf("edit --source: %+v", e)
	}
}
