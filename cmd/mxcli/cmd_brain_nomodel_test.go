// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
)

// A brain can be filled before the project exists, and `plan` must then report
// the truth — nothing built — instead of failing to connect (ako/mxcli#1948).
// The .mpr is deliberately never created.
func TestPlanAndCheckAnswerWhenTheModelDoesNotExistYet(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "Nothing.mpr")

	store := brain.NewStore(dir)
	if _, err := store.Init(); err != nil {
		t.Fatal(err)
	}
	req, err := brain.NewRequirement("A planner can create a visit", []string{"@Scheduling.Visit"}, "01-scheduling", brainTestDay())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Promote(req, req.Shard()); err != nil {
		t.Fatal(err)
	}

	out := runBrainForTest(t, []string{"brain", "plan", "-p", mpr})
	if !strings.Contains(out, "0 of 1 requirements built") {
		t.Errorf("plan without a model:\n%s", out)
	}
	if !strings.Contains(out, "No model") {
		t.Errorf("plan must say the model is absent, not leave it to be inferred:\n%s", out)
	}

	var plan struct {
		Built       int  `json:"built"`
		Total       int  `json:"total"`
		ModelAbsent bool `json:"model_absent"`
	}
	out = runBrainForTest(t, []string{"brain", "plan", "-p", mpr, "--json"})
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("plan --json: %v\n%s", err, out)
	}
	if plan.Built != 0 || plan.Total != 1 || !plan.ModelAbsent {
		t.Errorf("plan --json = %+v, want built 0 of 1 with model_absent", plan)
	}

	var check struct {
		Failed       bool `json:"failed"`
		Unverifiable int  `json:"unverifiable"`
		ModelAbsent  bool `json:"model_absent"`
	}
	out = runBrainForTest(t, []string{"brain", "check", "-p", mpr, "--json"})
	if err := json.Unmarshal([]byte(out), &check); err != nil {
		t.Fatalf("check --json: %v\n%s", err, out)
	}
	if check.Failed || check.Unverifiable != 1 || !check.ModelAbsent {
		t.Errorf("check --json = %+v, want not failed, 1 unverifiable, model_absent", check)
	}
}
