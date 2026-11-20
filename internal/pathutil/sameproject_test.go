// SPDX-License-Identifier: Apache-2.0

package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func mkProject(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return mpr
}

func TestSameProject(t *testing.T) {
	root := t.TempDir()
	orig := mkProject(t, root, "app")

	t.Run("same path", func(t *testing.T) {
		if !SameProject(orig, orig) {
			t.Error("a path is not the same project as itself")
		}
	})
	t.Run("copied folder is another project", func(t *testing.T) {
		copyMpr := mkProject(t, root, "app-copy")
		if SameProject(orig, copyMpr) {
			t.Error("a copy of the folder was taken for the original")
		}
		if SameProject(copyMpr, orig) {
			t.Error("the original was taken for the copy")
		}
	})
	t.Run("symlinked folder is the same project", func(t *testing.T) {
		link := filepath.Join(root, "link")
		if err := os.Symlink(filepath.Dir(orig), link); err != nil {
			t.Skip("cannot create a symlink here:", err)
		}
		if !SameProject(orig, filepath.Join(link, "App.mpr")) {
			t.Error("a symlink to the project folder was taken for another project")
		}
	})
	t.Run("dot segments", func(t *testing.T) {
		if !SameProject(orig, filepath.Join(filepath.Dir(orig), "..", "app", "App.mpr")) {
			t.Error("an unclean path was taken for another project")
		}
	})
	t.Run("legacy file without a project", func(t *testing.T) {
		if !SameProject("", orig) {
			t.Error("a state written before the project was recorded must still be honoured")
		}
	})
	t.Run("recorded folder is gone", func(t *testing.T) {
		gone := filepath.Join(root, "moved", "App.mpr")
		if SameProject(gone, orig) {
			t.Error("a vanished folder was taken for this project")
		}
		if !SameProject(gone, gone) {
			t.Error("an unresolvable path is not the same project as itself")
		}
	})
}
