// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// moduleRoot walks up from this file to the directory holding go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", file)
		}
		dir = parent
	}
}

// TestNoBareHomeSetenv fails for any test file that isolates the home
// directory with Setenv("HOME", ...) alone. On Windows os.UserHomeDir reads
// USERPROFILE, so such a test silently reads and writes the developer's real
// ~/.mxcli (#1256). SetHome sets both variables.
func TestNoBareHomeSetenv(t *testing.T) {
	root := moduleRoot(t)
	skipNames := map[string]bool{"vendor": true, "node_modules": true, ".git": true}
	skipRel := map[string]bool{
		filepath.Join("mdl", "grammar", "parser"): true, // generated, gitignored
		filepath.Join("internal", "testutil"):     true, // home of SetHome itself
	}
	needle := `Setenv("HOME"`

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if skipNames[d.Name()] || skipRel[rel] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		rel, _ := filepath.Rel(root, path)
		sc := bufio.NewScanner(f)
		for n := 1; sc.Scan(); n++ {
			if strings.Contains(sc.Text(), needle) {
				t.Errorf("%s:%d: bare Setenv(\"HOME\", ...): use testutil.SetHome(t, dir) instead; "+
					"os.UserHomeDir reads USERPROFILE on Windows, so a HOME-only override leaves the developer's real ~/.mxcli in use (#1256)",
					filepath.ToSlash(rel), n)
			}
		}
		return sc.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
}
