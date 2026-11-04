// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"io/fs"
	"os"
)

// CopyFixture copies the fixture project at src into dst the way os.CopyFS
// does, leaving out every .mxcli directory.
//
// .mxcli holds mxcli's own caches, never anything a fixture commits: nothing
// under it is tracked. But a test that opens a fixture in place writes them
// there (the unit-contents store, ako/mxcli#1804, a SQLite file whose journal
// comes and goes with each write), and go test runs packages in parallel. A
// whole-directory copy in another package then lists a journal that is gone by
// the time it opens it, and fails with
//
//	open .mxcli/cache/unit-contents.db-journal: no such file or directory
//
// (seen on the nightly in mdl/backend/mcp). Leaving .mxcli out also makes the
// copy what a fresh checkout holds, whichever tests ran first.
func CopyFixture(dst, src string) error {
	return copyFixtureFS(dst, os.DirFS(src))
}

func copyFixtureFS(dst string, fsys fs.FS) error {
	return os.CopyFS(dst, withoutMxcli{fsys})
}

// withoutMxcli hides every .mxcli entry from the directory listings os.CopyFS
// walks.
type withoutMxcli struct{ fs.FS }

func (f withoutMxcli) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(f.FS, name)
	kept := entries[:0]
	for _, e := range entries {
		if e.Name() != ".mxcli" {
			kept = append(kept, e)
		}
	}
	return kept, err
}
