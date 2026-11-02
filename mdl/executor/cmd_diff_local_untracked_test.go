// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A unit the working copy has added but git has not been told about is still a
// new document. `git diff <ref>` lists tracked files only, so diff-local
// against the working tree reported "0 new" while new modules, pages and flows
// sat untracked under mprcontents/ (mendixlabs/mxcli#1709).
func TestDiffLocal_UntrackedUnitIsNew(t *testing.T) {
	root, proj := gitProjectRepo(t)
	gitIn(t, root, "checkout", "-q", "--", ".") // drop gitProjectRepo's tracked edit
	const added = "7c7c7c7c-1111-2222-3333-444444444444"
	src := filepath.Join(proj, "mprcontents", "5a", "5a", gitUnit+".mxunit")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(proj, "mprcontents", "7c", "7c", added+".mxunit")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, out := gitDiffContext(filepath.Join(proj, "mprcontents"))
	if err := diffLocal(ctx, "HEAD", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "Summary: 1 new, 0 modified, 0 deleted") {
		t.Errorf("an untracked unit is not reported as new (#1709):\n%s", got)
	}
}

// A range compares two commits, so the working tree's untracked files are no
// part of it.
func TestDiffLocal_RangeIgnoresUntrackedUnits(t *testing.T) {
	root, proj := gitProjectRepo(t)
	gitIn(t, root, "commit", "-q", "-am", "after")
	dst := filepath.Join(proj, "mprcontents", "7c", "7c", "7c7c7c7c-1111-2222-3333-444444444444.mxunit")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("not bson"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, out := gitDiffContext(filepath.Join(proj, "mprcontents"))
	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "Summary: 0 new, 1 modified, 0 deleted") {
		t.Errorf("a range picked up an untracked working-tree unit:\n%s", got)
	}
}
