// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1408: "Exec is not atomic after the pre-check passes". A
// statement that fails at write time left every statement before it written — a
// created module, and a dropped document the failing statement was meant to
// replace. exec is all or nothing now: the project must come back byte-identical.

type atomicFixture struct {
	name, src, mpr, drop string // drop: a document the fixture already holds
}

var atomicFixtures = []atomicFixture{
	{"v1", "../../modelsdk/mpr/testdata/v1-project", "App.mpr", ""},
	{"v2", "../../testdata/testapp-views", "TestApp.mpr", "drop entity MyFirstModule.Car;\ndrop microflow MyFirstModule.MyFirstLogic;\n"},
}

func TestExecRollsBackWhenAStatementFails(t *testing.T) {
	for _, fx := range atomicFixtures {
		t.Run(fx.name, func(t *testing.T) {
			dir := copyFixtureTree(t, fx.src)
			before := hashTree(t, dir)

			script := "create module Atom1408;\n" +
				"create persistent entity Atom1408.Thing (Name: String(100));\n" +
				fx.drop +
				"drop entity Atom1408.DoesNotExist;\n"
			err := runScriptOn(t, filepath.Join(dir, fx.mpr), script)
			if err == nil {
				t.Fatal("the script's last statement fails; exec reported success")
			}
			if !strings.Contains(err.Error(), "Nothing was written") {
				t.Errorf("error does not say the writes were rolled back: %v", err)
			}
			if diff := diffTrees(before, hashTree(t, dir)); diff != "" {
				t.Errorf("a failed exec left the project changed:\n%s", diff)
			}
		})
	}
}

// THE CONTROL: the same script without the failing statement is written, so the
// test above is not passing because nothing ever reaches disk.
func TestExecWritesAScriptThatSucceeds(t *testing.T) {
	for _, fx := range atomicFixtures {
		t.Run(fx.name, func(t *testing.T) {
			dir := copyFixtureTree(t, fx.src)
			before := hashTree(t, dir)

			script := "create module Atom1408;\n" +
				"create persistent entity Atom1408.Thing (Name: String(100));\n" + fx.drop
			if err := runScriptOn(t, filepath.Join(dir, fx.mpr), script); err != nil {
				t.Fatalf("exec: %v", err)
			}
			if diffTrees(before, hashTree(t, dir)) == "" {
				t.Fatal("a successful exec wrote nothing")
			}
			var out bytes.Buffer
			exec := executor.New(&out)
			exec.SetBackendFactory(newBackendFactory())
			defer exec.Close()
			for _, cmd := range []string{
				"connect local '" + visitor.QuoteString(filepath.Join(dir, fx.mpr)) + "';",
				"show entities in Atom1408;",
			} {
				prog, _ := visitor.Build(cmd)
				if err := exec.ExecuteProgram(prog); err != nil {
					t.Fatalf("%s: %v", cmd, err)
				}
			}
			if !strings.Contains(out.String(), "Atom1408.Thing") {
				t.Errorf("the created entity is not in the project:\n%s", out.String())
			}
		})
	}
}

func runScriptOn(t *testing.T, mprPath, script string) error {
	t.Helper()
	exec := executor.New(io.Discard)
	exec.SetBackendFactory(newBackendFactory())
	defer exec.Close()
	conn, _ := visitor.Build("connect local '" + visitor.QuoteString(mprPath) + "';")
	if err := exec.ExecuteProgram(conn); err != nil {
		t.Fatalf("connect: %v", err)
	}
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	return runScriptAtomically(exec, prog)
}

func copyFixtureTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	// Not .mxcli: other packages' tests write their caches into the fixture in
	// place (testutil.CopyFixture), and hashTree ignores it anyway.
	if err := testutil.CopyFixture(dst, src); err != nil {
		t.Fatal(err)
	}
	return dst
}

func hashTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		// .mxcli holds mxcli's own caches — the catalog, the unit-type index
		// any listing may refresh — not the project, as mdl/scriptdiff's
		// snapshot treats it.
		if d.IsDir() && rel == ".mxcli" {
			return filepath.SkipDir
		}
		if d.IsDir() {
			// A directory a created unit needed must go when it is rolled back.
			out[rel+"/"] = "dir"
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func diffTrees(before, after map[string]string) string {
	var b strings.Builder
	for p, h := range before {
		switch a, ok := after[p]; {
		case !ok:
			b.WriteString("  deleted  " + p + "\n")
		case a != h:
			b.WriteString("  modified " + p + "\n")
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			b.WriteString("  created  " + p + "\n")
		}
	}
	return b.String()
}

// The rollback summary agrees in number: a single statement "was" rolled back.
// It read "statement 1 were rolled back" (found while exercising #1408).
func TestRolledBackSummary(t *testing.T) {
	for stoppedAt, want := range map[int]string{
		-1: "the script's statements were rolled back",
		0:  "the failing statement's partial writes were rolled back",
		1:  "statement 1 was rolled back",
		3:  "statements 1-3 were rolled back",
	} {
		if got := rolledBackSummary(stoppedAt); got != want {
			t.Errorf("rolledBackSummary(%d) = %q, want %q", stoppedAt, got, want)
		}
	}
}
