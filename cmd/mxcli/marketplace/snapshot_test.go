// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// testBackend is the default engine, the one users get.
func testBackend() backend.FullBackend { return modelsdkbackend.New() }

const fixtureDir = "../../../testdata/expr-checker"

// copyFixture makes a throwaway copy of the vendored project. Snapshotting builds
// a catalog beside the .mpr, and the mutation test writes to the model, so
// neither may touch the checked-in fixture.
func copyFixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, fixtureDir); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return filepath.Join(dst, "minimal.mpr")
}

// execMDL applies MDL text to a project through the executor — the same path a
// user's edit takes. Written as MDL rather than hand-built AST so the test keeps
// exercising the real parser and does not silently rot when AST fields change.
func execMDL(t *testing.T, mprPath, mdl string) {
	t.Helper()
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", mdl, errs)
	}

	var sink bytes.Buffer
	exec := executor.New(&sink)
	exec.SetBackendFactory(testBackend)
	defer exec.Close()
	if err := exec.Execute(&ast.ConnectStmt{Path: mprPath}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	for _, s := range prog.Statements {
		if err := exec.Execute(s); err != nil {
			t.Fatalf("execute: %v\noutput: %s", err, sink.String())
		}
	}
}

// TestSnapshotModule_ReadsRealModule checks the enumeration and describe capture
// actually work against a real project before anything is compared.
func TestSnapshotModule_ReadsRealModule(t *testing.T) {
	snap, err := SnapshotModule(copyFixture(t), "Administration", testBackend)
	if err != nil {
		t.Fatalf("SnapshotModule: %v", err)
	}
	if len(snap.Elements) == 0 {
		t.Fatal("no elements captured for Administration")
	}

	// Spot-check that content was captured, not just keys: an empty MDL body
	// would satisfy a count assertion while comparing everything as equal.
	var described int
	for _, e := range snap.Elements {
		if e.Describable() && len(e.MDL) > 0 {
			described++
		}
	}
	if described == 0 {
		t.Fatalf("every element failed to describe: %+v", snap.Elements)
	}
	t.Logf("Administration: %d elements, %d described", len(snap.Elements), described)
}

// TestSnapshotModule_IsDeterministic is the control the whole design rests on.
//
// Two snapshots of the same unmodified module must be identical. If DESCRIBE
// output varies between runs — a map iterated without sorting, a fresh $ID
// leaking into the text — then every comparison is noise and no result from this
// tool can be believed. Snapshotting two *separate copies* rather than the same
// file twice also catches anything that depends on the project's path.
func TestSnapshotModule_IsDeterministic(t *testing.T) {
	a, err := SnapshotModule(copyFixture(t), "Administration", testBackend)
	if err != nil {
		t.Fatalf("snapshot A: %v", err)
	}
	b, err := SnapshotModule(copyFixture(t), "Administration", testBackend)
	if err != nil {
		t.Fatalf("snapshot B: %v", err)
	}

	rep := Compare(a, b)
	if !rep.Clean() {
		for _, f := range rep.Findings {
			if f.Verdict != Unchanged {
				t.Errorf("%s: %s (%s)", f.Key, f.Verdict, f.Reason)
				if f.Verdict == Modified {
					t.Errorf("  A: %s", f.InstalledMDL)
					t.Errorf("  B: %s", f.PackageMDL)
				}
			}
		}
		t.Fatal("two snapshots of the same module must compare clean")
	}
}

// TestSnapshotModule_DetectsARealEdit is the money test: the differ must notice a
// genuine local modification, and must not report anything else as changed.
//
// Both halves matter. Missing the edit makes the tool unsafe — it would clear a
// module for a destructive upgrade. Reporting unrelated elements as modified
// makes it useless, because a report full of false positives gets ignored.
func TestSnapshotModule_DetectsARealEdit(t *testing.T) {
	pristine := copyFixture(t)
	edited := copyFixture(t)

	// A minimal, unambiguous edit to one element of the module — the kind of local
	// change a user makes to a marketplace module and then forgets about.
	execMDL(t, edited, "alter entity Administration.Account add attribute LocalNote: String(100);")

	before, err := SnapshotModule(pristine, "Administration", testBackend)
	if err != nil {
		t.Fatalf("snapshot pristine: %v", err)
	}
	after, err := SnapshotModule(edited, "Administration", testBackend)
	if err != nil {
		t.Fatalf("snapshot edited: %v", err)
	}

	rep := Compare(after, before)
	if rep.Clean() {
		t.Fatal("an added attribute must show up as a local modification")
	}
	if !rep.LocallyModified() {
		t.Error("LocallyModified must be true after a real edit")
	}

	var modified []string
	for _, f := range rep.Findings {
		if f.Verdict == Modified || f.Verdict == OnlyInstalled {
			modified = append(modified, f.Key.String())
		}
	}
	if len(modified) != 1 {
		t.Fatalf("exactly one element should differ, got %d: %v", len(modified), modified)
	}
	if modified[0] != "ENTITY Account" {
		t.Errorf("the changed element should be ENTITY Account, got %s", modified[0])
	}
}
