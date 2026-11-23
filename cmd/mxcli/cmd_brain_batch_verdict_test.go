// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
)

type stagedVerdictJSON struct {
	Staged []struct {
		ID      string `json:"id"`
		Accepts []struct {
			Rule    string `json:"rule"`
			Element string `json:"element"`
			Reason  string `json:"reason"`
		} `json:"accepts"`
		Verdicts []struct {
			Requirement string `json:"requirement"`
			Kind        string `json:"kind"`
		} `json:"verdicts"`
	} `json:"staged"`
}

// ako/mxcli#2524 end to end: the reporter's item is captured by --from-json
// instead of failing the batch, `staged --json` shows what each queued decision
// accepts and changes, and they survive promotion into what `list --json` reads.
func TestBrainBatchCapturesAcceptancesAndVerdicts(t *testing.T) {
	mpr := brainJSONProject(t)
	dir := filepath.Dir(mpr)
	store := brain.NewStore(dir)
	req, err := brain.NewRequirement("Orders must be approvable", []string{"@Sales.ACT_Approve"}, "01-approvals", brainTestDay())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Promote(req, req.Shard()); err != nil {
		t.Fatal(err)
	}

	in := filepath.Join(t.TempDir(), "decisions.json")
	if err := os.WriteFile(in, []byte(`[
		{"text":"Kept deliberately simple: ACT_ microflows do their own object changes.",
		 "anchors":["@FieldService.ACT_Triage_Complete"],
		 "accepts":["CONV010:one-happy-path demo app"]},
		{"text":"Approval is per team, not per person", "anchors":["@Sales.ACT_Approve"],
		 "changes":["`+req.ID+`:per team"]},
		{"text":"Is the export needed for v1?", "open":true, "partly_meets":["`+req.ID+`"]}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"brain", "capture", "--from-json", in, "-p", mpr, "--json"}
	if c := decodeJSON[captureBatchJSON](t, args, runBrainForTest(t, args)); c.Added != 3 {
		t.Fatalf("capture --from-json = %+v, want all three queued", c)
	}

	sargs := []string{"brain", "staged", "-p", mpr, "--json"}
	st := decodeJSON[stagedVerdictJSON](t, sargs, runBrainForTest(t, sargs))
	if len(st.Staged) != 3 {
		t.Fatalf("staged: %+v", st)
	}
	if a := st.Staged[0].Accepts; len(a) != 1 || a[0].Rule != "CONV010" || a[0].Element != "FieldService.ACT_Triage_Complete" || a[0].Reason != "one-happy-path demo app" {
		t.Errorf("staged accepts = %+v", a)
	}
	if v := st.Staged[1].Verdicts; len(v) != 1 || v[0].Requirement != req.ID || v[0].Kind != brain.VerdictChanges {
		t.Errorf("staged changes = %+v", v)
	}
	if v := st.Staged[2].Verdicts; len(v) != 1 || v[0].Kind != brain.VerdictPartly {
		t.Errorf("staged partly_meets = %+v", v)
	}

	pargs := []string{"brain", "promote", st.Staged[0].ID, st.Staged[1].ID, st.Staged[2].ID, "-p", mpr, "--json"}
	if p := decodeJSON[promoteBatchJSON](t, pargs, runBrainForTest(t, pargs)); p.Promoted != 3 {
		t.Fatalf("promote = %+v", p)
	}
	accepted, _, err := store.LoadAcceptances()
	if err != nil {
		t.Fatal(err)
	}
	if len(accepted) != 1 || accepted[0].Rule != "CONV010" {
		t.Errorf("after promote, lint would read acceptances %+v", accepted)
	}
}
