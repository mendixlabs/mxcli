// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ako/mxcli#2054: `capture --from-json` and `promote <id>...` end to end, in
// the JSON shapes a caller reads.

type captureBatchJSON struct {
	Results []captureJSON `json:"results"`
	Added   int           `json:"added"`
	Skipped int           `json:"skipped"`
	Total   int           `json:"total"`
}

type promoteBatchJSON struct {
	Results  []promoteJSON `json:"results"`
	Promoted int           `json:"promoted"`
	Total    int           `json:"total"`
}

func TestBrainBatchCaptureAndPromote(t *testing.T) {
	mpr := brainJSONProject(t)
	in := filepath.Join(t.TempDir(), "entries.json")
	if err := os.WriteFile(in, []byte(`[
		{"text": "Orders are committed by Finance", "anchors": ["@Sales.Order"]},
		{"title": "Invoices are immutable", "body": "Corrections are credit notes.", "anchors": ["@Sales.Order"]},
		{"text": "Orders are committed by Finance", "anchors": ["@Sales.Order"]}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}

	args := []string{"brain", "capture", "--from-json", in, "-p", mpr, "--json"}
	c := decodeJSON[captureBatchJSON](t, args, runBrainForTest(t, args))
	if c.Total != 3 || c.Added != 2 || c.Skipped != 1 || len(c.Results) != 3 {
		t.Fatalf("capture --from-json = %+v, want 2 added and the in-batch duplicate skipped", c)
	}
	if c.Results[2].Added || c.Results[2].ID != c.Results[0].ID {
		t.Errorf("in-batch duplicate = %+v, want added false with id %s", c.Results[2], c.Results[0].ID)
	}

	// The same file again is a no-op.
	again := decodeJSON[captureBatchJSON](t, args, runBrainForTest(t, args))
	if again.Added != 0 {
		t.Errorf("re-capture added %d, want 0", again.Added)
	}

	ids := []string{c.Results[0].ID, c.Results[1].ID}
	pargs := append([]string{"brain", "promote"}, append(ids, "-p", mpr, "--json")...)
	p := decodeJSON[promoteBatchJSON](t, pargs, runBrainForTest(t, pargs))
	if p.Promoted != 2 || p.Total != 2 || p.Results[0].Shard != "Sales" {
		t.Fatalf("promote --json = %+v, want both promoted into Sales", p)
	}

	// Run again: done already, not an error.
	p = decodeJSON[promoteBatchJSON](t, pargs, runBrainForTest(t, pargs))
	if p.Promoted != 0 || p.Total != 2 || p.Results[0].Status != "already_promoted" {
		t.Errorf("re-promote --json = %+v, want both already_promoted", p)
	}
}
