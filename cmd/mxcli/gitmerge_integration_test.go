//go:build integration

// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// The engine path against a REAL mx: `mx merge` over three .mpr paths with
// MINE as a clean worktree, the result copied back — the part the fake-runner
// unit tests cannot see. One scaffold, both engine verdicts:
//
//	TestGitMergeIntegration_EngineAutoResolvesAddAdd   exit 0: two sides each
//	                                                   add a page; both kept.
//	TestGitMergeIntegration_PropertyConflictNeedsDecisionAborts
//	                                                   exit 2: both sides
//	                                                   change the same page's
//	                                                   title; clean abort.
//
// Requires a resolvable mx for the scaffolded project's version (provided by
// the CI integration job's `mxcli setup mxbuild`); skips otherwise.
//
//	MXCLI_BIN=/path/to/mxcli go test -tags integration ./cmd/mxcli/ -run TestGitMergeIntegration -v
func TestGitMergeIntegration_EngineAutoResolvesAddAdd(t *testing.T) {
	dir, mpr, bin := mergeIntegrationRepo(t)

	applyPage(t, bin, dir, mpr, "PageMain", "Main page")
	commitAll(t, dir, "main adds PageMain")
	gitIn(t, dir, "switch", "-q", "-c", "side", "HEAD~1")
	applyPage(t, bin, dir, mpr, "PageSide", "Side page")
	commitAll(t, dir, "side adds PageSide")
	gitIn(t, dir, "switch", "-q", "main")

	var out bytes.Buffer
	runners := mxRunners{merge: realMxMerge, diff: realMxDiff}
	if err := mergeRun(mpr, "side", "", "", "", false, runners, &out); err != nil {
		t.Fatalf("mergeRun: %v\n%s", err, out.String())
	}
	parents := strings.Fields(gitOutT(t, dir, "rev-list", "--parents", "-n", "1", "HEAD"))
	if len(parents) != 3 {
		t.Fatalf("want a two-parent merge commit, got %v", parents)
	}
	if st := gitOutT(t, dir, "status", "--porcelain"); strings.TrimSpace(st) != "" {
		t.Errorf("working tree not clean after the merge:\n%s", st)
	}
	var out2 bytes.Buffer
	if err := fixHashes(mpr, false, &out2); err != nil {
		t.Errorf("the merged .mpr index is inconsistent: %v\n%s", err, out2.String())
	}
	for _, name := range []string{"PageMain", "PageSide"} {
		if unitFilesContaining(t, dir, name) == 0 {
			t.Errorf("no unit file carries %s: the engine's result lost a side", name)
		}
	}
}

func TestGitMergeIntegration_PropertyConflictNeedsDecisionAborts(t *testing.T) {
	dir, mpr, bin := mergeIntegrationRepo(t)

	// The shared page exists in the base, so both sides MODIFY the same unit:
	// side branches from the commit that already has the page.
	applyPage(t, bin, dir, mpr, "Shared", "Base title")
	commitAll(t, dir, "base adds Shared")
	gitIn(t, dir, "switch", "-q", "-c", "side")
	applyPage(t, bin, dir, mpr, "Shared", "Side title")
	commitAll(t, dir, "side retitles Shared")
	gitIn(t, dir, "switch", "-q", "main")
	applyPage(t, bin, dir, mpr, "Shared", "Main title")
	commitAll(t, dir, "main retitles Shared")

	before := gitOutT(t, dir, "rev-parse", "HEAD")
	var out bytes.Buffer
	runners := mxRunners{merge: realMxMerge, diff: realMxDiff}
	err := mergeRun(mpr, "side", "", "", "", false, runners, &out)
	if err == nil || !strings.Contains(err.Error(), "Studio Pro") {
		t.Fatalf("a property conflict must end in the Studio Pro referral, got %v\n%s", err, out.String())
	}
	if got := strings.TrimSpace(gitOutT(t, dir, "rev-parse", "HEAD")); got != strings.TrimSpace(before) {
		t.Errorf("HEAD moved despite the abort: %s", got)
	}
	if _, mhErr := gitOut(dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); mhErr == nil {
		t.Error("MERGE_HEAD still present: the merge was not aborted")
	}
	if uu := gitOutT(t, dir, "diff", "--name-only", "--diff-filter=U"); strings.TrimSpace(uu) != "" {
		t.Errorf("working tree not restored:\n%s", uu)
	}
}

// --plan over the engine's own result: each side adds an entity to
// MyFirstModule's domain model — a unit the engine merges by itself — and
// both retitle the same page, which it leaves for a decision. The plan the
// first run writes names only the page; applying it must keep both entities
// and commit an index with no conflict marker left (`mx check` reports
// CE0137 while one is). Resolving from git's two sides instead took the
// domain model from ours and lost theirs' entity, with the plan never
// naming that unit.
func TestGitMergeIntegration_PlanKeepsWhatTheEngineMerged(t *testing.T) {
	dir, mpr, bin := mergeIntegrationRepo(t)

	applyPage(t, bin, dir, mpr, "Shared", "Base title")
	commitAll(t, dir, "base adds Shared")
	gitIn(t, dir, "switch", "-q", "-c", "side")
	applyMDL(t, bin, dir, mpr, "TheirsOnly", "mdl 1;\ncreate non-persistent entity MyFirstModule.TheirsOnly (Code: String(50));\n")
	applyPage(t, bin, dir, mpr, "Shared", "Theirs retitle")
	commitAll(t, dir, "side adds TheirsOnly, retitles Shared")
	gitIn(t, dir, "switch", "-q", "main")
	applyMDL(t, bin, dir, mpr, "OursOnly", "mdl 1;\ncreate non-persistent entity MyFirstModule.OursOnly (Name: String(50));\n")
	applyPage(t, bin, dir, mpr, "Shared", "Ours retitle")
	commitAll(t, dir, "main adds OursOnly, retitles Shared")
	// Control: the title is distinctive — only the page carries it (the
	// scaffold's own units say "Main title", for one).
	if n := unitFilesContaining(t, dir, "Ours retitle"); n != 1 {
		t.Fatalf("%d unit files carry ours' title before the merge, want 1 (the page)", n)
	}

	runners := mxRunners{merge: realMxMerge, diff: realMxDiff}
	var first bytes.Buffer
	if err := mergeRun(mpr, "side", "", "", "", false, runners, &first); err == nil {
		t.Fatalf("the page conflict must stop for a decision\n%s", first.String())
	}
	raw, err := os.ReadFile(mpr + ".merge-plan.json")
	if err != nil {
		t.Fatalf("no plan template: %v\n%s", err, first.String())
	}
	_ = os.Remove(mpr + ".merge-plan.json") // working material, never committed
	var plan mergePlanTemplate
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Units) != 1 {
		t.Fatalf("the template must name only the unit the engine left open (the page), got %+v\n%s", plan.Units, first.String())
	}
	plan.Units[0].Take = "theirs"
	edited, _ := json.Marshal(plan)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(planPath, edited, 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := mergeRun(mpr, "side", "", "", planPath, false, runners, &out); err != nil {
		t.Fatalf("mergeRun --plan: %v\n%s", err, out.String())
	}
	parents := strings.Fields(gitOutT(t, dir, "rev-list", "--parents", "-n", "1", "HEAD"))
	if len(parents) != 3 {
		t.Fatalf("want a two-parent merge commit, got %v", parents)
	}
	if st := gitOutT(t, dir, "status", "--porcelain"); strings.TrimSpace(st) != "" {
		t.Errorf("working tree not clean after the merge:\n%s", st)
	}
	for _, name := range []string{"OursOnly", "TheirsOnly", "Theirs retitle"} {
		if unitFilesContaining(t, dir, name) == 0 {
			t.Errorf("no unit file carries %q: the plan's merge lost it", name)
		}
	}
	if unitFilesContaining(t, dir, "Ours retitle") != 0 {
		t.Error("the page still carries ours' title: the plan said theirs")
	}
	if marks, err := conflictMarkers(mpr); err != nil || len(marks) != 0 {
		t.Errorf("merge conflict markers committed: %+v (%v)", marks, err)
	}
	mxPath, err := docker.ResolveMxForVersion("", projectMendixVersion(mpr))
	if err != nil {
		t.Fatal(err)
	}
	check := exec.Command(mxPath, "check", mpr)
	docker.PrepareMxCommand(check)
	// The scaffold's own errors make the exit code meaningless here; what
	// matters is that the check loaded the project and found no conflict.
	checkOut, _ := check.CombinedOutput()
	if !bytes.Contains(checkOut, []byte("Loading the mpr file")) {
		t.Fatalf("mx check never loaded the project, so its silence on CE0137 proves nothing:\n%s", checkOut)
	}
	if bytes.Contains(checkOut, []byte("CE0137")) {
		t.Errorf("mx check still reports the merge conflict (CE0137):\n%s", checkOut)
	}
}

// mergeIntegrationRepo scaffolds a real Mendix project (`mx create-project`)
// in a hermetic git repository and returns its dir, .mpr path and a built
// mxcli binary for applying MDL. The project and the binary live in SHORT
// temp dirs: the test names make t.TempDir() paths long enough for
// create-project's template extraction to hit PathTooLongException.
func mergeIntegrationRepo(t *testing.T) (string, string, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "mxmrg")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	// The command's own git merge and commit run in this process: with the
	// global config gone, they need an identity a hostname cannot supply.
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")

	bin := os.Getenv("MXCLI_BIN")
	if bin == "" {
		binDir, err := os.MkdirTemp("", "mxbin")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(binDir) })
		bin = filepath.Join(binDir, "mxcli.exe")
		build := exec.Command("go", "build", "-o", bin, ".")
		build.Dir = "." // the package dir: cmd/mxcli
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("go build: %v\n%s", err, out)
		}
	}

	scaffoldProject(t, dir)
	mpr := filepath.Join(dir, "App.mpr")
	if _, err := docker.ResolveMxForVersion("", projectMendixVersion(mpr)); err != nil {
		t.Skipf("no mx for the scaffolded project: %v", err)
	}
	// mxcli's own unit index is working material, not project content: it is
	// rewritten on every exec and would conflict on both sides.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".mxcli/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "config", "core.autocrlf", "false")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "scaffold")
	return dir, mpr, bin
}

// scaffoldProject runs `mx create-project` into dir like the docker suite
// does; its failure skips the test (no mx, no fixture).
func scaffoldProject(t *testing.T, dir string) {
	t.Helper()
	mxPath, err := docker.ResolveMxForVersion("", "")
	if err != nil {
		t.Skipf("no mx at all: %v", err)
	}
	create := exec.Command(mxPath, "create-project")
	create.Dir = dir
	docker.PrepareMxCommand(create)
	if out, err := create.CombinedOutput(); err != nil {
		t.Skipf("mx create-project: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "App.mpr")); err != nil {
		t.Skipf("mx create-project did not produce App.mpr: %v", err)
	}
}

// applyPage writes and applies one page change through the real mxcli exec
// path (in-process model write, --no-check skips the semantic preflight).
// The script lives outside the repository: it is test scaffolding, not
// project content, and committing it would conflict on both sides.
func applyPage(t *testing.T, bin, dir, mpr, name, title string) {
	t.Helper()
	applyMDL(t, bin, dir, mpr, name, fmt.Sprintf("mdl 1;\ncreate or modify page MyFirstModule.%s (title: '%s', layout: Atlas_Core.Atlas_Default) {\n  dynamictext t1 (content: '%s')\n};\n", name, title, title))
}

// applyMDL applies one MDL script the same way; what names the change in a
// failure message.
func applyMDL(t *testing.T, bin, dir, mpr, what, mdl string) {
	t.Helper()
	f, err := os.CreateTemp("", "mxpage")
	if err != nil {
		t.Fatal(err)
	}
	script := f.Name()
	t.Cleanup(func() { os.Remove(script) })
	if _, err := f.WriteString(mdl); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "exec", "-p", mpr, "--no-check", script)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("apply %s: %v\n%s", what, err, stderr.String())
	}
}

func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", msg)
}

// unitFilesContaining counts the .mxunit files whose bytes carry needle
// (page names are ASCII in a unit's BSON).
func unitFilesContaining(t *testing.T, dir, needle string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(filepath.Join(dir, "mprcontents"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if bytes.Contains(raw, []byte(needle)) {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}
