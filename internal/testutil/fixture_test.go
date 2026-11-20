// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// vanishingJournal is a fixture caught mid-write by another package's test: its
// listing still names the cache's journal, which is gone when the copy opens it.
type vanishingJournal struct{ fstest.MapFS }

const journal = ".mxcli/cache/unit-contents.db-journal"

func (v vanishingJournal) Open(name string) (fs.File, error) {
	if name == journal {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return v.MapFS.Open(name)
}

func fixtureWithJournal() vanishingJournal {
	return vanishingJournal{fstest.MapFS{
		"minimal.mpr":                       {Data: []byte("mpr")},
		"mprcontents/ab/cd/unit.mxunit":     {Data: []byte("unit")},
		".mxcli/cache/unit-contents.db":     {Data: []byte("cache")},
		journal:                             {Data: []byte("journal")},
		"widgets/.mxcli/widgets/x.def.json": {Data: []byte("def")},
	}}
}

// The nightly's symptom, reproduced deterministically: a plain os.CopyFS of the
// fixture fails on the journal another test's write has already removed.
func TestCopyFixture_Control_PlainCopyFailsOnAVanishedJournal(t *testing.T) {
	err := os.CopyFS(t.TempDir(), fixtureWithJournal())
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("os.CopyFS = %v, want the vanished journal's not-exist error", err)
	}
}

func TestCopyFixture_LeavesOutEveryMxcliDirectory(t *testing.T) {
	dst := t.TempDir()
	if err := copyFixtureFS(dst, fixtureWithJournal()); err != nil {
		t.Fatalf("copy: %v", err)
	}
	for _, rel := range []string{"minimal.mpr", "mprcontents/ab/cd/unit.mxunit"} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel))); err != nil {
			t.Errorf("the project's %s was not copied: %v", rel, err)
		}
	}
	for _, rel := range []string{".mxcli", "widgets/.mxcli"} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("%s was copied (stat err %v); it holds caches, never the fixture", rel, err)
		}
	}
}
