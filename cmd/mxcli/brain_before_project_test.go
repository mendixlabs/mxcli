// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
)

// A brain can be born before its project (ako/mxcli#1949). These tests pin
// down the three halves that make that official rather than accidental:
// -C names the directory, `mxcli new` adopts a brain already there, and the
// move that installs the project never touches it.

// seedBrain makes a directory hold exactly what a brain-first workflow leaves:
// an initialised store with one promoted requirement, plus a queued capture.
func seedBrain(t *testing.T, dir string) (promoted brain.Entry) {
	t.Helper()
	store := brain.NewStore(dir)
	if _, err := store.Init(); err != nil {
		t.Fatal(err)
	}
	req, err := brain.NewRequirement("Accounts can be administered", []string{"@Administration.Account"}, "01-accounts", brainTestDay())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Promote(req, req.Shard()); err != nil {
		t.Fatal(err)
	}
	queued, err := brain.NewEntry("Accounts are created by an admin only", []string{"@Administration.Account"}, brainTestDay())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := brain.NewQueue(dir).Append(queued); err != nil {
		t.Fatal(err)
	}
	return req
}

func TestNewDestinationAcceptsOnlyAnEmptyDirOrABrain(t *testing.T) {
	write := func(t *testing.T, dir, rel, content string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name      string
		setup     func(t *testing.T, dir string)
		wantAdopt bool
		wantErr   string // substring; empty means accepted
	}{
		{"missing", func(t *testing.T, dir string) { _ = os.Remove(dir) }, false, ""},
		{"empty", func(t *testing.T, dir string) {}, false, ""},
		{"brain and queue", func(t *testing.T, dir string) { seedBrain(t, dir) }, true, ""},
		{"store only", func(t *testing.T, dir string) {
			if _, err := brain.NewStore(dir).Init(); err != nil {
				t.Fatal(err)
			}
		}, true, ""},
		// Controls: each is a brain plus ONE thing that is not, or a
		// docs/brain mxcli did not write. Adoption must stay this narrow, or
		// `new` starts merging a project into someone's files.
		{"foreign docs/brain", func(t *testing.T, dir string) {
			write(t, dir, "docs/brain/README.md", "my own notes\n")
		}, false, "not written by mxcli"},
		{"brain plus a stray file", func(t *testing.T, dir string) {
			seedBrain(t, dir)
			write(t, dir, "notes.txt", "x")
		}, false, "not empty"},
		{"brain plus other docs", func(t *testing.T, dir string) {
			seedBrain(t, dir)
			write(t, dir, "docs/design.md", "x")
		}, false, "not empty"},
		{"brain plus other .mxcli state", func(t *testing.T, dir string) {
			seedBrain(t, dir)
			write(t, dir, ".mxcli/catalog.db", "x")
		}, false, "not empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "App")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			tc.setup(t, dir)
			adopt, err := checkNewProjectDestination(dir)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				if adopt != tc.wantAdopt {
					t.Errorf("adopt = %v, want %v", adopt, tc.wantAdopt)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}
}

// The move must merge the project around the brain and leave the brain's bytes
// alone. Before this, moveProject's rename failed on the non-empty directory
// and its os.CopyFS fallback merged by accident — and stopped half way on the
// first file that already existed.
func TestMoveProjectMergesAroundAnAdoptedBrain(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "App")
	seedBrain(t, dest)
	before := snapshot(t, dest)

	stage := fakeStage(t)
	if err := moveProject(stage, dest); err != nil {
		t.Fatalf("moveProject: %v", err)
	}
	for _, f := range []string{"App.mpr", filepath.Join("mprcontents", "aa", "unit.mxunit"), ".gitignore"} {
		if _, err := os.Stat(filepath.Join(dest, f)); err != nil {
			t.Errorf("%s did not arrive: %v", f, err)
		}
	}
	after := snapshot(t, dest)
	for path, content := range before {
		if after[path] != content {
			t.Errorf("brain file %s changed by the move", path)
		}
	}
}

// A conflict is found before anything moves, so a refusal leaves both sides as
// they were — the guarantee `mxcli new` makes for every other failure.
func TestMoveProjectRefusesAConflictBeforeMovingAnything(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "App")
	seedBrain(t, dest)
	before := snapshot(t, dest)

	stage := fakeStage(t)
	clash := filepath.Join(stage, "docs", "brain", "README.md")
	if err := os.MkdirAll(filepath.Dir(clash), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clash, []byte("template"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := moveProject(stage, dest)
	if err == nil || !strings.Contains(err.Error(), filepath.Join("docs", "brain", "README.md")) {
		t.Fatalf("err = %v, want a conflict naming docs/brain/README.md", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "App.mpr")); !os.IsNotExist(err) {
		t.Error("App.mpr was moved although the move was refused")
	}
	if _, err := os.Stat(filepath.Join(stage, "App.mpr")); err != nil {
		t.Errorf("the staged project lost App.mpr on a refused move: %v", err)
	}
	if got := snapshot(t, dest); len(got) != len(before) {
		t.Errorf("destination changed: %d files, was %d", len(got), len(before))
	}
}

// The issue's proof, minus `mx create-project` (which needs mxbuild): a brain
// born in an empty directory, a project installed into that directory the way
// `mxcli new` installs one, and the same -C command going from "no model, 0 of
// 1" to counting the requirement against the model.
func TestBrainBornBeforeTheProjectIsCountedAfterIt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "PedApp")

	// Through the CLI, by directory: no .mpr is ever named.
	runBrainForTest(t, []string{"brain", "init", "-C", dir})
	out := runBrainForTest(t, []string{"brain", "capture", "Accounts can be administered",
		"-a", "@Administration.Account", "-s", "01-accounts", "-C", dir})
	id := strings.Fields(out)[1]
	runBrainForTest(t, []string{"brain", "promote", id, "-C", dir})

	out = runBrainForTest(t, []string{"brain", "plan", "-C", dir})
	if !strings.Contains(out, "0 of 1 requirements built") || !strings.Contains(out, "No model") {
		t.Fatalf("before the project:\n%s", out)
	}

	adopt, err := checkNewProjectDestination(dir)
	if err != nil || !adopt {
		t.Fatalf("checkNewProjectDestination = %v, %v; want adopt", adopt, err)
	}
	stage := filepath.Join(t.TempDir(), "stage")
	if err := os.CopyFS(stage, os.DirFS(filepath.Join("..", "..", "testdata", "pedapp"))); err != nil {
		t.Fatal(err)
	}
	if err := moveProject(stage, dir); err != nil {
		t.Fatal(err)
	}

	out = runBrainForTest(t, []string{"brain", "plan", "-C", dir})
	if !strings.Contains(out, "1 of 1 requirements built") || strings.Contains(out, "No model") {
		t.Errorf("after the project was installed:\n%s", out)
	}
}

func TestResolveBrainLocation(t *testing.T) {
	dir := t.TempDir()
	if _, err := resolveBrainLocation("", ""); err == nil {
		t.Error("neither -p nor -C must be refused")
	}
	if _, err := resolveBrainLocation(filepath.Join(dir, "App.mpr"), dir); err == nil {
		t.Error("-p and -C together must be refused, not silently resolved to one")
	}

	// -p is the shorthand: the directory of a path that need not exist.
	loc, err := resolveBrainLocation(filepath.Join(dir, "Nothing.mpr"), "")
	if err != nil || loc.Dir != dir || loc.MPR != filepath.Join(dir, "Nothing.mpr") {
		t.Errorf("-p: %+v, %v", loc, err)
	}

	// -C with no project yet: a directory and no model.
	loc, err = resolveBrainLocation("", dir)
	if err != nil || loc.Dir != dir || loc.MPR != "" {
		t.Errorf("-C, no .mpr: %+v, %v", loc, err)
	}

	// -C after `mxcli new`: the one .mpr there is the model.
	if err := os.WriteFile(filepath.Join(dir, "App.mpr"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	loc, err = resolveBrainLocation("", dir)
	if err != nil || loc.MPR != filepath.Join(dir, "App.mpr") {
		t.Errorf("-C, one .mpr: %+v, %v", loc, err)
	}

	// Two is ambiguous, and guessing would check the brain against the wrong app.
	if err := os.WriteFile(filepath.Join(dir, "Other.mpr"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveBrainLocation("", dir); err == nil || !strings.Contains(err.Error(), "Other.mpr") {
		t.Errorf("-C with two .mpr files: err = %v, want one naming them", err)
	}
}

// `brain init <dir>` used to accept the directory and ignore it, writing the
// store wherever -p pointed instead.
func TestBrainInitRejectsAPositionalDirectory(t *testing.T) {
	dir := t.TempDir()
	resetBrainFlagsForTest(t)
	rootCmd.SetArgs([]string{"brain", "init", filepath.Join(dir, "elsewhere"), "-C", dir})
	if err := rootCmd.Execute(); err == nil {
		t.Error("a positional argument to init must be an error, not ignored")
	}
	if brain.NewStore(dir).Exists() {
		t.Error("init ran despite the stray argument")
	}
}

func fakeStage(t *testing.T) string {
	t.Helper()
	stage := filepath.Join(t.TempDir(), "stage")
	for _, f := range []string{"App.mpr", filepath.Join("mprcontents", "aa", "unit.mxunit"), ".gitignore"} {
		p := filepath.Join(stage, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return stage
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		out[rel] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
