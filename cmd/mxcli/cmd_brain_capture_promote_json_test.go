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

// ako/mxcli#1953: `brain capture` and `brain promote` printed prose with
// --json set, so a caller scraped the new id out of a sentence. They emit a
// JSON object, the duplicate paths included, as staged, plan, check, show and
// brief already do; without --json they still print for a person.

type captureJSON struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Slice  string `json:"slice"`
	Open   bool   `json:"open"`
	Shard  string `json:"shard"`
	Added  bool   `json:"added"`
	Status string `json:"status"`
}

type promoteJSON struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Shard  string `json:"shard"`
	Path   string `json:"path"`
	Status string `json:"status"`
}

func brainJSONProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := brain.NewStore(dir).Init(); err != nil {
		t.Fatal(err)
	}
	return mpr
}

func decodeJSON[T any](t *testing.T, args []string, out string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("`mxcli %v` did not emit JSON on stdout: %v\ngot:\n%s", args, err, out)
	}
	return v
}

func TestBrainCaptureAndPromoteHonourJSON(t *testing.T) {
	mpr := brainJSONProject(t)

	args := []string{"brain", "capture", "Orders are committed by Finance", "-a", "@Sales.Order", "-p", mpr, "--json"}
	c := decodeJSON[captureJSON](t, args, runBrainForTest(t, args))
	if c.ID == "" || c.Kind != "decision" || c.Shard == "" || !c.Added || c.Status != "queued" {
		t.Fatalf("capture --json = %+v, want a queued decision with an id and a shard", c)
	}

	// The same capture again: not added, the same id, still JSON.
	again := decodeJSON[captureJSON](t, args, runBrainForTest(t, args))
	if again.Added || again.Status != "already_queued" || again.ID != c.ID {
		t.Errorf("duplicate capture --json = %+v, want added false, status already_queued, id %s", again, c.ID)
	}

	// The prose names the queued entry's id too, not a freshly minted one.
	human := []string{"brain", "capture", "Orders are committed by Finance", "-a", "@Sales.Order", "-p", mpr}
	if out := runBrainForTest(t, human); !strings.Contains(out, "Already queued as "+c.ID) {
		t.Errorf("duplicate capture without --json does not name the queued id %s:\n%s", c.ID, out)
	}

	// A requirement carries its slice.
	rargs := []string{"brain", "capture", "A planner can create a visit", "-a", "@Scheduling.Visit", "-s", "01-scheduling", "-p", mpr, "--json"}
	r := decodeJSON[captureJSON](t, rargs, runBrainForTest(t, rargs))
	if r.Kind != "requirement" || r.Slice != "01-scheduling" || !r.Added {
		t.Errorf("requirement capture --json = %+v", r)
	}

	pargs := []string{"brain", "promote", c.ID, "-p", mpr, "--json"}
	p := decodeJSON[promoteJSON](t, pargs, runBrainForTest(t, pargs))
	if p.ID != c.ID || p.Kind != "decision" || p.Shard != c.Shard || p.Path == "" {
		t.Fatalf("promote --json = %+v, want id %s into shard %s with its path", p, c.ID, c.Shard)
	}
	if _, err := os.Stat(p.Path); err != nil {
		t.Errorf("promote --json names a shard path that does not exist: %v", err)
	}

	// Captured again once committed: found by content, not added.
	committed := decodeJSON[captureJSON](t, args, runBrainForTest(t, args))
	if committed.Added || committed.Status != "already_committed" || committed.ID != c.ID {
		t.Errorf("capture of a committed entry --json = %+v, want added false, status already_committed, id %s", committed, c.ID)
	}

	// Control: without --json both still print for a person.
	for _, human := range [][]string{
		{"brain", "capture", "Invoices are numbered per year", "-a", "@Sales.Invoice", "-p", mpr},
		{"brain", "promote", r.ID, "-p", mpr},
	} {
		var v map[string]any
		if out := runBrainForTest(t, human); json.Unmarshal([]byte(out), &v) == nil {
			t.Errorf("`mxcli %v` without --json emitted JSON:\n%s", human, out)
		}
	}
}
