// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/projectreport"
)

// The loader reads GUIDs from a Studio Pro-authored model, where a GUID is not
// the element's $ID: only such a model can show a re-minted GUID (CLAUDE.md,
// "Before trusting any GUID test, check the subject").
func TestLoadReleaseModelReadsStudioProGUIDs(t *testing.T) {
	m, err := loadReleaseModel(filepath.Join("..", "..", "testdata", "pedapp", "PedApp.mpr"))
	if err != nil {
		t.Fatal(err)
	}
	var account *projectreport.RelEntity
	stored := 0
	for i, e := range m.Entities {
		if e.Stored {
			stored++
		}
		if e.Name == "Administration.Account" {
			account = &m.Entities[i]
		}
	}
	if account == nil || !account.Stored || account.Generalization != "System.User" || !account.Marketplace {
		t.Fatalf("Administration.Account is stored through System.User: %+v", account)
	}
	if account.GUID == "" || account.GUID == account.ID {
		t.Fatalf("a Studio Pro entity's GUID is its own, not its $ID: guid %q id %q", account.GUID, account.ID)
	}
	for _, a := range account.Attributes {
		if a.GUID == "" || a.Type == "" {
			t.Errorf("attribute %s: guid %q type %q", a.Name, a.GUID, a.Type)
		}
	}
	// show entities lists one persistent entity in this app.
	if stored != 1 {
		t.Errorf("stored entities = %d, want 1", stored)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// --against takes a git revision of the project, in a subdirectory of the
// repository, or a .mpr path; an unchanged model reports nothing either way.
func TestReleaseBaselineFromGitAndPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	app := filepath.Join(repo, "apps", "ped")
	if err := copyTree(filepath.Join("..", "..", "testdata", "pedapp"), app); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-qm", "v1")
	git(t, repo, "tag", "v1")
	mprPath := filepath.Join(app, "PedApp.mpr")

	base, label, cleanup, err := resolveReleaseBaseline(mprPath, "v1")
	defer cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(label, "git v1 (") || strings.HasPrefix(base, repo) {
		t.Errorf("label %q, baseline %q: a revision is exported to a temporary directory", label, base)
	}
	deployed, err := loadReleaseModel(base)
	if err != nil {
		t.Fatal(err)
	}
	next, err := loadReleaseModel(mprPath)
	if err != nil {
		t.Fatal(err)
	}
	if d := projectreport.CompareReleases(deployed, next); len(d.Changes) != 0 || d.Tables != 1 {
		t.Errorf("unchanged: tables %d, changes %+v", d.Tables, d.Changes)
	}
	cleanup()
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Errorf("the export is removed by cleanup: %v", err)
	}

	if p, _, _, err := resolveReleaseBaseline(mprPath, mprPath); err != nil || p != mprPath {
		t.Errorf("a .mpr path is used as given: %q %v", p, err)
	}
	if _, _, _, err := resolveReleaseBaseline(mprPath, "no-such-tag"); err == nil || !strings.Contains(err.Error(), "not a revision") {
		t.Errorf("an unknown revision is an error naming it: %v", err)
	}
	if _, _, _, err := resolveReleaseBaseline(mprPath, "/nowhere/App.mpr"); err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Errorf("a missing .mpr is an error: %v", err)
	}
}
