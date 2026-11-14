// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

// ako/mxcli#2240 (mendixlabs/mxcli#1854): `git merge` exited 1 both when it
// stopped for a decision a person must take and when it failed, so a script
// could not tell the two apart. A decision is now exit 3; a failure stays 1.

func TestGitMerge_DecisionExitsThree(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)
	decide := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		return 2, `[{"ConflictResolutionDetails":"ManualResolutionRequired"}]`, nil
	}}
	var out bytes.Buffer
	err := mergeRun(mprPath, "side", "", "", "", false, decide, &out)
	if err == nil {
		t.Fatal("a pending decision must not read as success")
	}
	if got := exitCodeFor(err); got != exitDecisionNeeded {
		t.Errorf("a pending model decision exits %d, want %d: %v", got, exitDecisionNeeded, err)
	}
}

// Control: the engine failing is an error, not a decision.
func TestGitMerge_EngineFailureExitsOne(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)
	crash := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		return 129, "Exception in MergeLib", nil
	}}
	var out bytes.Buffer
	err := mergeRun(mprPath, "side", "", "", "", false, crash, &out)
	if err == nil || exitCodeFor(err) != 1 {
		t.Errorf("an engine crash exits %d, want 1: %v", exitCodeFor(err), err)
	}
}

func TestGitMerge_TextConflictExitsThree(t *testing.T) {
	err := reportTextConflicts(&bytes.Buffer{}, []string{"NOTES.txt"})
	if got := exitCodeFor(err); got != exitDecisionNeeded {
		t.Errorf("text conflicts left for a person exit %d, want %d", got, exitDecisionNeeded)
	}
	if exitCodeFor(errors.New("plain")) != 1 || exitCodeFor(nil) != 0 {
		t.Error("exitCodeFor: a plain error must be 1 and no error 0")
	}
}

// The plan template written beside the .mpr while a decision was pending is
// stale once the merge has committed; it was never removed and turned up in
// the next `git add -A`. A plan file the caller passed from elsewhere is
// theirs, and stays.
func TestGitMerge_CommitRemovesTheStalePlanTemplate(t *testing.T) {
	dir, mprPath := twoUnitConflictRepo(t)
	doctorEnv(t, dir)
	result := baseUnits(t)
	result[tDM] = tUnit{tMod, unitDoc(t, "DomainModels$DomainModel", "", mainSideKey)}
	result[tMf] = tUnit{tMod, unitDoc(t, "Microflows$Microflow", "ACT_Save", mainSideKey)}
	planPath := writePlanFor(t, dir, map[string]string{tDM: "ours", tMf: "theirs"})
	template := mprPath + ".merge-plan.json"
	if err := os.WriteFile(template, []byte(`{"units":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	mx := mxRunners{merge: engineDecides(t, result, []string{tDM, tMf}, nil)}
	var out bytes.Buffer
	if err := mergeRun(mprPath, "side", "", "", planPath, false, mx, &out); err != nil {
		t.Fatalf("mergeRun --plan: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(template); !os.IsNotExist(err) {
		t.Errorf("the plan template beside the .mpr survived the commit: %v", err)
	}
	if _, err := os.Stat(planPath); err != nil {
		t.Errorf("the caller's own plan file was removed: %v", err)
	}
	if st := gitOutT(t, dir, "status", "--porcelain"); strings.Contains(st, "merge-plan") {
		t.Errorf("the template is still in git status:\n%s", st)
	}
}
