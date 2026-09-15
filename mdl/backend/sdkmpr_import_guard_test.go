// SPDX-License-Identifier: Apache-2.0

package backend_test

// Phase 4a of docs/plans/2026-09-14-retire-legacy-engine.md took sdk/mpr from 27
// importers to 0. This keeps it there.
//
// Without a guard the count creeps back one import at a time, and the reason it
// matters is not tidiness: sdk/mpr is the legacy engine, and a caller holding a
// concrete *sdk/mpr.Reader is INVISIBLE to the unimplemented-method census in
// mdl/backend/modelsdk (that census lists methods with no implementation; a
// caller reaching one through a concrete reader never appears). That blind spot
// is what hid project_tree.go's 36 semantic reads and cmd_extract_templates.go's
// FindCustomWidgetType until each was found by hand.
//
// Modelled on scripts/check-tunnel-deps.sh: assert a positive control first, so
// a scan that silently examined nothing cannot pass.

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	legacyEngine = `"github.com/mendixlabs/mxcli/sdk/mpr"`
	// The backend abstraction — something the repo definitely imports widely.
	// Used only as the detector's positive control.
	backendPkg = `"github.com/mendixlabs/mxcli/mdl/backend"`
)

func TestNothingImportsTheLegacyEngine(t *testing.T) {
	root := repoRoot(t)

	var offenders []string
	var scanned, sawControl int

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "reference", "vendor":
				return filepath.SkipDir
			}
			// sdk/mpr's own files are the package itself, not importers of it.
			if filepath.ToSlash(strings.TrimPrefix(path, root)) == "/sdk/mpr" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if perr != nil {
			// Generated parser sources can be enormous but still parse; a file that
			// does not is not something to fail the guard on, so it is skipped
			// loudly rather than silently.
			t.Logf("skipping unparseable %s: %v", path, perr)
			return nil
		}
		scanned++
		rel := strings.TrimPrefix(filepath.ToSlash(strings.TrimPrefix(path, root)), "/")
		for _, imp := range f.Imports {
			switch imp.Path.Value {
			case legacyEngine:
				offenders = append(offenders, rel)
			case backendPkg:
				sawControl++
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	// Positive controls. Without these a broken walk, a wrong root or an
	// import-parsing mistake reports "0 importers" and reads as success.
	if scanned < 500 {
		t.Fatalf("scanned only %d Go files — the walk is not covering the repo, "+
			"so a clean result here would mean nothing", scanned)
	}
	if sawControl == 0 {
		t.Fatalf("scanned %d files and saw no import of %s — the detector cannot "+
			"see imports at all, so it cannot see sdk/mpr either", scanned, backendPkg)
	}

	if len(offenders) > 0 {
		t.Errorf("%d file(s) import the legacy engine sdk/mpr, which Phase 4a "+
			"emptied:\n  %s\n\nUse mdl/backend (backend.FullBackend) instead. If a "+
			"method you need is missing there, implement it on the codec backend "+
			"rather than reaching past the abstraction — a concrete reader is "+
			"invisible to the unimplemented-method census.",
			len(offenders), strings.Join(offenders, "\n  "))
	}
	t.Logf("scanned %d Go files, %d import mdl/backend, 0 import sdk/mpr", scanned, sawControl)
}

// repoRoot walks up from the test's directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found above the test directory")
		}
		dir = parent
	}
}
