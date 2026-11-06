// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1751 (ako/mxcli#1982), as reported:
//
//  1. `git merge <ref>` stops on a non-model conflict ("resolve by hand…").
//  2. The human resolves it, `git add`s it and re-runs: the engine needs a
//     decision (exit 2) and the merge is aborted — and the text resolution
//     goes with the abort.
//  3. `--plan` then wrote the model, repaired the hashes and failed at the
//     commit on "unmerged files": a half-done merge, the resolution lost.
//
// The staged resolution must survive the abort and come back on the --plan
// run; and a --plan run with a text conflict still open must stop before it
// touches the model.

// modelAndTextConflictRepo is a modelConflictRepo where each side also changed
// NOTES.txt, so the merge conflicts on both the domain model and a text file.
func modelAndTextConflictRepo(t *testing.T) (string, string) {
	t.Helper()
	dir, mprPath := doctorNoteRepo(t)
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	units := baseUnits(t)
	units[tDM] = tUnit{tMod, unitDoc(t, "DomainModels$DomainModel", "", mainSideKey)}
	writeProject(t, dir, "11.13.0", units)
	write("NOTES.txt", "main\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "main side")
	gitIn(t, dir, "switch", "-q", "-c", "side", "HEAD~1")
	side := baseUnits(t)
	side[tDM] = tUnit{tMod, unitDoc(t, "DomainModels$DomainModel", "", sideKey)}
	writeProject(t, dir, "11.13.0", side)
	write("NOTES.txt", "side\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "side change")
	gitIn(t, dir, "switch", "-q", "feature")
	return dir, mprPath
}

// engineResult is what the fake engine writes for the undecided unit: ours'
// content, marked.
func engineResult(t *testing.T) map[string]tUnit {
	result := baseUnits(t)
	result[tDM] = tUnit{tMod, unitDoc(t, "DomainModels$DomainModel", "", mainSideKey)}
	return result
}

func TestGitMerge_TextResolutionSurvivesTheAbortAndThePlan(t *testing.T) {
	dir, mprPath := modelAndTextConflictRepo(t)
	doctorEnv(t, dir)
	mx := mxRunners{merge: engineDecides(t, engineResult(t), []string{tDM}, nil)}

	// 1. The text conflict stops the run before mx.
	var out bytes.Buffer
	if err := mergeRun(mprPath, "side", "", "", "", false, mx, &out); err == nil ||
		!strings.Contains(out.String(), "NOTES.txt") {
		t.Fatalf("step 1: want the non-model conflict reported, got %v\n%s", err, out.String())
	}

	// 2. Resolved by hand and staged; the re-run needs a model decision and
	// aborts.
	if err := os.WriteFile(filepath.Join(dir, "NOTES.txt"), []byte("resolved by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "NOTES.txt")
	out.Reset()
	if err := mergeRun(mprPath, "side", "", "", "", false, mx, &out); err == nil {
		t.Fatalf("step 2: want the exit-2 abort, got success\n%s", out.String())
	}
	planPath := mprPath + ".merge-plan.json"
	if _, err := os.Stat(planPath); err != nil {
		t.Fatalf("step 2: no plan template written: %v\n%s", err, out.String())
	}

	// 3. --plan resolves the model and commits — with the text resolution.
	out.Reset()
	if err := mergeRun(mprPath, "side", "", "", planPath, false, mx, &out); err != nil {
		t.Fatalf("step 3: mergeRun --plan: %v\n%s", err, out.String())
	}
	if got := gitOutT(t, dir, "show", "HEAD:NOTES.txt"); got != "resolved by hand\n" {
		t.Errorf("the merge commit's NOTES.txt = %q, want the hand resolution", got)
	}
	os.Remove(planPath) // the template is not part of the merge
	assertPlanMergeCommitted(t, dir, mprPath)
}

// With a text conflict open and no resolution to restore, --plan stops before
// the model: the engine does not run, nothing is written, and the merge stays
// in progress for the human to resolve and re-run.
func TestGitMerge_PlanStopsOnAnOpenTextConflict(t *testing.T) {
	dir, mprPath := modelAndTextConflictRepo(t)
	doctorEnv(t, dir)
	plan := mergePlanTemplate{
		Base:   strings.TrimSpace(gitOutT(t, dir, "merge-base", "HEAD", "side")),
		Ours:   strings.TrimSpace(gitOutT(t, dir, "rev-parse", "HEAD")),
		Theirs: strings.TrimSpace(gitOutT(t, dir, "rev-parse", "side")),
		Units:  []planUnit{{ID: tDM, Take: "ours"}},
	}
	raw, _ := json.Marshal(plan)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(planPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	mx := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		calls++
		return 0, "", nil
	}}
	var out bytes.Buffer
	err := mergeRun(mprPath, "side", "", "", planPath, false, mx, &out)
	if err == nil || !strings.Contains(out.String(), "NOTES.txt") {
		t.Fatalf("want the open text conflict reported, got %v\n%s", err, out.String())
	}
	if calls != 0 {
		t.Errorf("the engine ran %d time(s) with a text conflict open", calls)
	}
	if _, mhErr := gitInE(t, dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); mhErr != nil {
		t.Error("the merge was aborted; it must stay in progress for the human to resolve")
	}
	uu := gitOutT(t, dir, "diff", "--name-only", "--diff-filter=U")
	if !strings.Contains(uu, "NOTES.txt") || !strings.Contains(uu, ".mxunit") {
		t.Errorf("the conflicts must be left as they were, got:\n%s", uu)
	}
	gitInE(t, dir, "merge", "--abort")
}
