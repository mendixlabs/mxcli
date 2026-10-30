// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func bsonKeyValue(key, value string) bson.E {
	return bson.E{Key: key, Value: value}
}

// `mxcli git merge <ref>` — merge a branch with model-aware resolution: git
// merge first, and on a model-file conflict `mx merge` (the same engine
// Studio Pro uses). The mx run is injected as runMx so the orchestration is
// testable without a Mendix installation; the real runner shells out to mx.

// modelConflictRepo makes a repository whose merge of "side" into "feature"
// conflicts on both model files: each side changed the same DomainModel unit.
func modelConflictRepo(t *testing.T) (string, string) {
	t.Helper()
	dir, mprPath := doctorNoteRepo(t)
	units := baseUnits(t)
	units[tDM] = tUnit{tMod, unitDoc(t, "DomainModels$DomainModel", "", mainSideKey)}
	writeProject(t, dir, "11.13.0", units)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "main side")
	gitIn(t, dir, "switch", "-q", "-c", "side", "HEAD~1")
	side := baseUnits(t)
	side[tDM] = tUnit{tMod, unitDoc(t, "DomainModels$DomainModel", "", sideKey)}
	writeProject(t, dir, "11.13.0", side)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "side change")
	gitIn(t, dir, "switch", "-q", "feature")
	mergeOut, err := gitInE(t, dir, "merge", "side")
	if err != nil && !strings.Contains(err.Error(), "exit status") {
		t.Fatalf("fixture: expected a conflicted merge, got %v\n%s", err, mergeOut)
	}
	if uu := gitOutT(t, dir, "diff", "--name-only", "--diff-filter=U"); !strings.Contains(uu, ".mxunit") {
		t.Fatalf("fixture: expected unresolved model files, got %q\nmerge said:\n%s", uu, mergeOut)
	}
	return dir, mprPath
}

// twoUnitConflictRepo is a modelConflictRepo where BOTH the domain model and
// the microflow unit were changed on each side — two conflicted units, so a
// plan can take one unit from ours and the other from theirs.
func twoUnitConflictRepo(t *testing.T) (string, string) {
	t.Helper()
	dir, mprPath := doctorNoteRepo(t)
	units := baseUnits(t)
	units[tDM] = tUnit{tMod, unitDoc(t, "DomainModels$DomainModel", "", mainSideKey)}
	units[tMf] = tUnit{tMod, unitDoc(t, "Microflows$Microflow", "ACT_Save", mainSideKey)}
	writeProject(t, dir, "11.13.0", units)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "main side")
	gitIn(t, dir, "switch", "-q", "-c", "side", "HEAD~1")
	side := baseUnits(t)
	side[tDM] = tUnit{tMod, unitDoc(t, "DomainModels$DomainModel", "", sideKey)}
	side[tMf] = tUnit{tMod, unitDoc(t, "Microflows$Microflow", "ACT_Save", sideKey)}
	writeProject(t, dir, "11.13.0", side)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "side change")
	gitIn(t, dir, "switch", "-q", "feature")
	mergeOut, err := gitInE(t, dir, "merge", "side")
	if err != nil && !strings.Contains(err.Error(), "exit status") {
		t.Fatalf("fixture: expected a conflicted merge, got %v\n%s", err, mergeOut)
	}
	return dir, mprPath
}

// addConflictRepo is a repository where each side ADDED a different unit: only
// the .mpr conflicts, and the sides' document sets differ — the class a whole
// side must refuse, never commit.
func addConflictRepo(t *testing.T) (string, string) {
	t.Helper()
	dir, mprPath := doctorNoteRepo(t)
	ours := baseUnits(t)
	oursAdd := "11111111-0000-0000-0000-000000000009"
	ours[oursAdd] = tUnit{tMod, unitDoc(t, "Microflows$Microflow", "ACT_Ours")}
	writeProject(t, dir, "11.13.0", ours)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "ours adds a microflow")
	gitIn(t, dir, "switch", "-q", "-c", "side", "HEAD~1")
	theirs := baseUnits(t)
	theirsAdd := "11111111-0000-0000-0000-00000000000a"
	theirs[theirsAdd] = tUnit{tMod, unitDoc(t, "Microflows$Microflow", "ACT_Theirs")}
	writeProject(t, dir, "11.13.0", theirs)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "theirs adds a microflow")
	gitIn(t, dir, "switch", "-q", "feature")
	mergeOut, err := gitInE(t, dir, "merge", "side")
	if err != nil && !strings.Contains(err.Error(), "exit status") {
		t.Fatalf("fixture: expected a conflicted merge, got %v\n%s", err, mergeOut)
	}
	if uu := gitOutT(t, dir, "diff", "--name-only", "--diff-filter=U"); !strings.Contains(uu, ".mpr") {
		t.Fatalf("fixture: expected the .mpr conflict, got %q\nmerge said:\n%s", uu, mergeOut)
	}
	return dir, mprPath
}

// unitRel is the mprcontents path of a unit id inside a work tree.
func unitRel(id string) string {
	return "mprcontents/" + id[:2] + "/" + id[2:4] + "/" + id + ".mxunit"
}

var (
	mainSideKey = bsonKeyValue("DocA", "main")
	sideKey     = bsonKeyValue("DocB", "side")
)

func okRunner(out string) mxMergeRunner {
	return func(base, mine, theirs string, shas [3]string) (int, string, error) {
		return 0, out, nil
	}
}

func TestGitMerge_CleanMergeTextFileWritesNote(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	gitIn(t, dir, "branch", "docs", "HEAD~1")
	gitIn(t, dir, "switch", "-q", "docs")
	if err := os.WriteFile(filepath.Join(dir, "NOTES.txt"), []byte("docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "NOTES.txt")
	gitIn(t, dir, "commit", "-q", "-m", "docs side")
	gitIn(t, dir, "switch", "-q", "feature")

	noMx := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		t.Fatal("mx must not run on a merge git resolves itself")
		return 0, "", nil
	}}
	var out bytes.Buffer
	if err := mergeRun(mprPath, "docs", "", "", "", false, noMx, &out); err != nil {
		t.Fatalf("mergeRun: %v\n%s", err, out.String())
	}
	parents := strings.Fields(gitOutT(t, dir, "rev-list", "--parents", "-n", "1", "HEAD"))
	if len(parents) != 3 {
		t.Fatalf("want a two-parent merge commit, got %v", parents)
	}
	var n mxNote
	if err := json.Unmarshal([]byte(noteOn(t, dir, "HEAD")), &n); err != nil {
		t.Fatalf("merge commit has no valid note: %v", err)
	}
	if !n.HasModelerVersion || n.BranchName != "feature" {
		t.Errorf("note header: %+v", n)
	}
}

func TestGitMerge_ModelConflictAutoResolvedByMx(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)

	var gotBase, gotMine, gotTheirs string
	var gotShas [3]string
	var baseHasMpr, theirsHasMpr bool
	runner := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		gotBase, gotMine, gotTheirs, gotShas = base, mine, theirs, shas
		// The worktrees are removed when the merge finishes; check inside.
		_, baseHasMprErr := os.Stat(filepath.Join(base, "App.mpr"))
		_, theirsHasMprErr := os.Stat(filepath.Join(theirs, "App.mpr"))
		baseHasMpr, theirsHasMpr = baseHasMprErr == nil, theirsHasMprErr == nil
		return 0, "Merge successful.", nil
	}}
	var out bytes.Buffer
	if err := mergeRun(mprPath, "side", "", "", "", false, runner, &out); err != nil {
		t.Fatalf("mergeRun: %v\n%s", err, out.String())
	}
	// Folder mode: the three args are project folders, and MINE is the
	// working tree — the engine writes its result there.
	if gotMine != dir {
		t.Errorf("runner's mine = %q, want the working project folder %q", gotMine, dir)
	}
	if !baseHasMpr {
		t.Errorf("runner's base folder %q must hold the project's .mpr", gotBase)
	}
	if !theirsHasMpr {
		t.Errorf("runner's theirs folder %q must hold the project's .mpr", gotTheirs)
	}
	if gotShas[0] == "" || gotShas[1] == gotShas[0] || gotShas[2] == gotShas[1] {
		t.Errorf("shas must be base/ours/theirs distinct: %v", gotShas)
	}
	parents := strings.Fields(gitOutT(t, dir, "rev-list", "--parents", "-n", "1", "HEAD"))
	if len(parents) != 3 {
		t.Fatalf("want a two-parent merge commit, got %v", parents)
	}
	uu := gitOutT(t, dir, "diff", "--name-only", "--diff-filter=U")
	if strings.TrimSpace(uu) != "" {
		t.Errorf("conflicts still unresolved:\n%s", uu)
	}
	if !strings.Contains(noteOn(t, dir, "HEAD"), `"HasModelerVersion":true`) {
		t.Errorf("merge commit has no note:\n%s", noteOn(t, dir, "HEAD"))
	}
}

func TestGitMerge_ModelConflictUnresolvedAborts(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)
	before := commitSubject(t, dir)

	fails := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		return 2, `[{"ConflictResolutionDetails":"ManualResolutionRequired"}]`, nil
	}}
	var out bytes.Buffer
	err := mergeRun(mprPath, "side", "", "", "", false, fails, &out)
	if err == nil || !strings.Contains(err.Error(), "Studio Pro") {
		t.Fatalf("want a Studio Pro referral, got %v", err)
	}
	if got := commitSubject(t, dir); got != before {
		t.Errorf("HEAD moved despite the abort: %q", got)
	}
	if _, err := gitOut(dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); err == nil {
		t.Error("MERGE_HEAD still present: the merge was not aborted")
	}
	uu := gitOutT(t, dir, "diff", "--name-only", "--diff-filter=U")
	if strings.TrimSpace(uu) != "" {
		t.Errorf("conflicted files remain:\n%s", uu)
	}
}

func TestGitMerge_NonModelConflictLeftAlone(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	gitIn(t, dir, "checkout", "-q", "-b", "side", "HEAD~1")
	if err := os.WriteFile(filepath.Join(dir, "NOTES.txt"), []byte("side\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "NOTES.txt")
	gitIn(t, dir, "commit", "-q", "-m", "side notes")
	gitIn(t, dir, "checkout", "-q", "feature")
	if err := os.WriteFile(filepath.Join(dir, "NOTES.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "NOTES.txt")
	gitIn(t, dir, "commit", "-q", "-m", "main notes")
	_, err := gitInE(t, dir, "merge", "side")
	if err != nil && !strings.Contains(err.Error(), "exit status") {
		t.Fatalf("fixture: expected a conflict, got %v", err)
	}

	calls := 0
	counting := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		calls++
		return 0, "", nil
	}}
	var out bytes.Buffer
	merr := mergeRun(mprPath, "side", "", "", "", false, counting, &out)
	if merr == nil {
		t.Fatal("a non-model conflict must be reported, not silently passed")
	}
	if calls != 0 {
		t.Errorf("mx must not run for non-model conflicts (ran %d times)", calls)
	}
	uu := gitOutT(t, dir, "diff", "--name-only", "--diff-filter=U")
	if !strings.Contains(uu, "NOTES.txt") {
		t.Errorf("the conflicted text file must be left for manual resolution:\n%s", uu)
	}
	gitInE(t, dir, "merge", "--abort") // clean up the fixture
}

func TestGitMerge_AlreadyUpToDate(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	before := commitSubject(t, dir)

	noMx := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		t.Fatal("mx must not run")
		return 0, "", nil
	}}
	var out bytes.Buffer
	if err := mergeRun(mprPath, "HEAD~1", "", "", "", false, noMx, &out); err != nil {
		t.Fatalf("mergeRun: %v", err)
	}
	if got := commitSubject(t, dir); got != before {
		t.Errorf("HEAD moved: %q", got)
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Errorf("must say so:\n%s", out.String())
	}
}

// The conflict report is the review material: on ManualResolutionRequired the
// merge is aborted and both sides' changes are printed, so a human can decide
// before anything lands. mx is not even needed for the report.
func TestGitMerge_ConflictReportShowsBothSidesAndAborts(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)
	before := commitSubject(t, dir)

	noMx := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		return 2, `[{"ConflictResolutionDetails":"ManualResolutionRequired"}]`, nil
	}}
	var out bytes.Buffer
	err := mergeRun(mprPath, "side", "", "", "", false, noMx, &out)
	if err == nil || !strings.Contains(err.Error(), "--plan") {
		t.Fatalf("the report must point at the resolution flags, got %v", err)
	}
	report := out.String()
	for _, want := range []string{"Plan template", "merge-plan.json", "Merge aborted"} {
		if !strings.Contains(report, want) {
			t.Errorf("report must contain %q:\n%s", want, report)
		}
	}
	if got := commitSubject(t, dir); got != before {
		t.Errorf("HEAD moved: %q", got)
	}
	if _, err := gitOut(dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); err == nil {
		t.Error("MERGE_HEAD still present")
	}
	if uu := gitOutT(t, dir, "diff", "--name-only", "--diff-filter=U"); strings.TrimSpace(uu) != "" {
		t.Errorf("working tree not restored:\n%s", uu)
	}
}

// --ours / --theirs take the decided side for every conflicted model file,
// commit the merge and write the note — deterministically, without mx.
func TestGitMerge_SideTakingResolves(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)

	noMx := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		t.Fatal("--ours must not consult mx")
		return 0, "", nil
	}}
	var out bytes.Buffer
	if err := mergeRun(mprPath, "side", "", "ours", "", false, noMx, &out); err != nil {
		t.Fatalf("mergeRun --ours: %v\n%s", err, out.String())
	}
	parents := strings.Fields(gitOutT(t, dir, "rev-list", "--parents", "-n", "1", "HEAD"))
	if len(parents) != 3 {
		t.Fatalf("want a two-parent merge commit, got %v", parents)
	}
	if !strings.Contains(noteOn(t, dir, "HEAD"), `"HasModelerVersion":true`) {
		t.Errorf("merge commit has no note")
	}
	page := filepath.Join(dir, "mprcontents", tDM[:2], tDM[2:4], tDM+".mxunit")
	raw, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("DocA")) || bytes.Contains(raw, []byte("DocB")) {
		t.Error("--ours must leave our side of the unit on disk")
	}

	// --theirs flips to the other side, deterministically.
	dir2, mprPath2 := modelConflictRepo(t)
	theirs := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		t.Fatal("--theirs must not consult mx")
		return 0, "", nil
	}}
	var out2 bytes.Buffer
	if err := mergeRun(mprPath2, "side", "", "theirs", "", false, theirs, &out2); err != nil {
		t.Fatalf("mergeRun --theirs: %v\n%s", err, out2.String())
	}
	page2 := filepath.Join(dir2, "mprcontents", tDM[:2], tDM[2:4], tDM+".mxunit")
	raw2, err := os.ReadFile(page2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw2, []byte("DocB")) || bytes.Contains(raw2, []byte("DocA")) {
		t.Error("--theirs must leave their side of the unit on disk")
	}
}

// A plan resolves per unit: tDM from ours, tMf from theirs — one merge commit,
// hashes repaired from the mixed files, note attached.
func TestGitMerge_PlanMixesPerUnit(t *testing.T) {
	dir, mprPath := twoUnitConflictRepo(t)
	doctorEnv(t, dir)
	base := strings.TrimSpace(gitOutT(t, dir, "merge-base", "HEAD", "side"))
	ours := strings.TrimSpace(gitOutT(t, dir, "rev-parse", "HEAD"))
	theirs := strings.TrimSpace(gitOutT(t, dir, "rev-parse", "side"))
	plan := fmt.Sprintf(`{"base":%q,"ours":%q,"theirs":%q,"units":[{"id":%q,"take":"ours"},{"id":%q,"take":"theirs"}]}`,
		base, ours, theirs, tDM, tMf)
	planPath := filepath.Join(dir, "merge-plan.json")
	if err := os.WriteFile(planPath, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}

	noMx := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		t.Fatal("--plan must not consult mx")
		return 0, "", nil
	}}
	var out bytes.Buffer
	if err := mergeRun(mprPath, "side", "", "", planPath, false, noMx, &out); err != nil {
		t.Fatalf("mergeRun --plan: %v\n%s", err, out.String())
	}
	assertUnitHas := func(id, key string, want bool) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(dir, unitRel(id)))
		if err != nil {
			t.Fatal(err)
		}
		if got := bytes.Contains(raw, []byte(key)); got != want {
			t.Errorf("%s contains %q = %v, want %v", id, key, got, want)
		}
	}
	assertUnitHas(tDM, "DocA", true)
	assertUnitHas(tDM, "DocB", false)
	assertUnitHas(tMf, "DocB", true)
	assertUnitHas(tMf, "DocA", false)
	parents := strings.Fields(gitOutT(t, dir, "rev-list", "--parents", "-n", "1", "HEAD"))
	if len(parents) != 3 {
		t.Fatalf("want a two-parent merge commit, got %v", parents)
	}
	var out2 bytes.Buffer
	if err := fixHashes(mprPath, false, &out2); err != nil {
		t.Errorf("hashes drift after the plan merge: %v\n%s", err, out2.String())
	}
}

// A plan whose revisions no longer match the merge state is stale and refused.
func TestGitMerge_PlanStaleRefused(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)
	plan := `{"base":"0000000000000000000000000000000000000000","ours":"0000000000000000000000000000000000000001","theirs":"0000000000000000000000000000000000000002","units":[]}`
	planPath := filepath.Join(dir, "merge-plan.json")
	if err := os.WriteFile(planPath, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	noMx := mxRunners{}
	var out bytes.Buffer
	err := mergeRun(mprPath, "side", "", "", planPath, false, noMx, &out)
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("want a stale-plan refusal, got %v", err)
	}
	gitInE(t, dir, "merge", "--abort") // clean up the fixture
}

// On ManualResolutionRequired the report is property-level (from mx diff) and
// a plan template is written beside the .mpr.
func TestGitMerge_ManualWritesPlanAndPropertyReport(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)
	fixture := `{"unitDifferences":[{"id":"` + tDM + `","type":"DomainModels$DomainModel","change":"Modified","changeDetails":[{"change":"Modified","changeDescription":"Attribute 'MyFirstModule.CreatedByMxcli.Nickname'","changeDetails":[{"property":"New type > Length","baseValue":"100","mineValue":"150"}]}]}]}`
	mx := mxRunners{
		merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
			return 2, `[{"ConflictResolutionDetails":"ManualResolutionRequired"}]`, nil
		},
		diff: func(base, mine, outJSON string) error {
			return os.WriteFile(outJSON, []byte(fixture), 0o644)
		},
	}
	var out bytes.Buffer
	err := mergeRun(mprPath, "side", "", "", "", false, mx, &out)
	if err == nil {
		t.Fatal("a manual conflict must end with the decision error")
	}
	report := out.String()
	for _, want := range []string{"Length", "100", "150", "Nickname"} {
		if !strings.Contains(report, want) {
			t.Errorf("report must contain %q:\n%s", want, report)
		}
	}
	planRaw, err := os.ReadFile(mprPath + ".merge-plan.json")
	if err != nil {
		t.Fatalf("plan template not written: %v", err)
	}
	var plan struct {
		Units []struct {
			Id   string `json:"id"`
			Take string `json:"take"`
		} `json:"units"`
	}
	if err := json.Unmarshal(planRaw, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Units) != 1 || plan.Units[0].Id != tDM || plan.Units[0].Take != "ours" {
		t.Errorf("plan template: %+v", plan)
	}
	gitInE(t, dir, "merge", "--abort") // clean up the fixture
}

// A whole side of a project includes the .mpr's list of documents. When the
// sides' document sets differ (one side added a unit), taking --theirs would
// leave ours' unit on disk with no .mpr entry: the commit is refused and the
// merge aborted, never silently half-committed.
func TestGitMerge_SideTakingRefusesWhenDocumentSetsDiffer(t *testing.T) {
	dir, mprPath := addConflictRepo(t)
	doctorEnv(t, dir)
	before := commitSubject(t, dir)

	noMx := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		t.Fatal("--theirs must not consult mx")
		return 0, "", nil
	}}
	var out bytes.Buffer
	err := mergeRun(mprPath, "side", "", "theirs", "", false, noMx, &out)
	if err == nil || !strings.Contains(err.Error(), "refusing to commit") {
		t.Fatalf("--theirs must refuse a side that drops ours' unit, got %v\n%s", err, out.String())
	}
	if got := commitSubject(t, dir); got != before {
		t.Errorf("HEAD moved: %q", got)
	}
	if _, err := gitOut(dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); err == nil {
		t.Error("MERGE_HEAD still present: the refusal must abort the merge")
	}
	if uu := gitOutT(t, dir, "diff", "--name-only", "--diff-filter=U"); strings.TrimSpace(uu) != "" {
		t.Errorf("working tree not restored:\n%s", uu)
	}
}

// Exit 129 is the engine failing, not a merge decision: report it, abort
// cleanly, and never write a plan template.
func TestGitMerge_EngineCrashAborts(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)
	before := commitSubject(t, dir)

	crash := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		return 129, "Exception in MergeLib", nil
	}}
	var out bytes.Buffer
	err := mergeRun(mprPath, "side", "", "", "", false, crash, &out)
	if err == nil || !strings.Contains(err.Error(), "exit 129") {
		t.Fatalf("want the engine-failure report, got %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "Plan template") {
		t.Errorf("a crash must not produce a plan:\n%s", out.String())
	}
	if got := commitSubject(t, dir); got != before {
		t.Errorf("HEAD moved: %q", got)
	}
	if _, err := gitOut(dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); err == nil {
		t.Error("MERGE_HEAD still present: the merge was not aborted")
	}
	if uu := gitOutT(t, dir, "diff", "--name-only", "--diff-filter=U"); strings.TrimSpace(uu) != "" {
		t.Errorf("working tree not restored:\n%s", uu)
	}
	if _, err := os.Stat(mprPath + ".merge-plan.json"); !os.IsNotExist(err) {
		t.Errorf("a crash must not write a plan template: %v", err)
	}
}

// mx not being found or not starting is the engine failing to run: the merge
// aborts with the reason, and nothing is left behind.
func TestGitMerge_EngineStartFailureAborts(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)
	before := commitSubject(t, dir)

	missing := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		return 0, "", errors.New("no mx for Mendix 11.13.0: not on PATH or in the cache (model conflicts need it)")
	}}
	var out bytes.Buffer
	err := mergeRun(mprPath, "side", "", "", "", false, missing, &out)
	if err == nil || !strings.Contains(err.Error(), "failed to run") {
		t.Fatalf("want the mx failure, got %v\n%s", err, out.String())
	}
	if _, err := gitOut(dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); err == nil {
		t.Error("MERGE_HEAD still present: the merge was not aborted")
	}
	if got := commitSubject(t, dir); got != before {
		t.Errorf("HEAD moved: %q", got)
	}
}

// A plan whose take value is not ours/theirs is refused, and the refusal
// aborts the merge: nothing may be left half-done.
func TestGitMerge_PlanRefusalAborts(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)
	base := strings.TrimSpace(gitOutT(t, dir, "merge-base", "HEAD", "side"))
	ours := strings.TrimSpace(gitOutT(t, dir, "rev-parse", "HEAD"))
	theirs := strings.TrimSpace(gitOutT(t, dir, "rev-parse", "side"))
	plan := fmt.Sprintf(`{"base":%q,"ours":%q,"theirs":%q,"units":[{"id":%q,"take":"sideways"}]}`,
		base, ours, theirs, tDM)
	planPath := filepath.Join(dir, "merge-plan.json")
	if err := os.WriteFile(planPath, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	noMx := mxRunners{}
	var out bytes.Buffer
	err := mergeRun(mprPath, "side", "", "", planPath, false, noMx, &out)
	if err == nil || !strings.Contains(err.Error(), "ours|theirs") {
		t.Fatalf("want the invalid-take refusal, got %v\n%s", err, out.String())
	}
	if _, err := gitOut(dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); err == nil {
		t.Error("MERGE_HEAD still present: the refusal must abort the merge")
	}
	if uu := gitOutT(t, dir, "diff", "--name-only", "--diff-filter=U"); strings.TrimSpace(uu) != "" {
		t.Errorf("working tree not restored:\n%s", uu)
	}
}

// Re-entry: with the merge already in progress (a previous run stopped and
// the human resolved what they handle by hand), re-running resolves the model
// and finishes — it does not re-run git merge and die on MERGE_HEAD.
func TestGitMerge_ResumesAMergeAlreadyInProgress(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)

	calls := 0
	runner := mxRunners{merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
		calls++
		return 0, "Merge successful.", nil
	}}
	var out bytes.Buffer
	if err := mergeRun(mprPath, "side", "", "", "", false, runner, &out); err != nil {
		t.Fatalf("mergeRun: %v\n%s", err, out.String())
	}
	if calls != 1 {
		t.Errorf("engine runs = %d, want 1", calls)
	}
	parents := strings.Fields(gitOutT(t, dir, "rev-list", "--parents", "-n", "1", "HEAD"))
	if len(parents) != 3 {
		t.Fatalf("want a two-parent merge commit, got %v", parents)
	}
}

// The plan template is working material: `mxcli git commit` never stages it.
func TestJunkPath_CoversMergePlanTemplate(t *testing.T) {
	if !isJunkPath("App.mpr.merge-plan.json", ".") {
		t.Error("App.mpr.merge-plan.json must be junk for staging")
	}
	if !isJunkPath("app/Project.mpr.merge-plan.json", "app") {
		t.Error("a project-folder plan template must be junk too")
	}
}
