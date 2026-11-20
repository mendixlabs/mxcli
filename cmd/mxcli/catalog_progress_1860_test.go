// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#1860 (mendixlabs/mxcli#1702): after a model change the next
// command rebuilt the catalog and printed a line per table — about fifty —
// ahead of its answer, and an agent paid for them in context every time.
// There was no way to turn them off: `-q` was honoured by the banner check and
// by `search`, and rejected by every other command.

// perTableLines are the build's "✓ Table: N" lines in out.
func perTableLines(out string) []string {
	var got []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "✓ ") && !strings.HasPrefix(l, "✓ Catalog") {
			got = append(got, l)
		}
	}
	return got
}

// Not a terminal (this test's pipes): a quick build prints no line per table.
func TestCatalogBuildOffTerminalPrintsNoLinePerTable_Issue1860(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	mpr := jsonPurityFixture(t)
	r := runMainStreams(t, filepath.Dir(mpr), "-p", mpr, "-c", "select count(*) from CATALOG.ENTITIES")
	if r.exitCode != 0 {
		t.Fatalf("exit %d\nstderr:\n%s", r.exitCode, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(mpr), ".mxcli", "catalog.db")); err != nil {
		t.Fatalf("the command built no catalog, so this proves nothing: %v", err)
	}
	if !strings.Contains(r.stdout, "count(*)") {
		t.Errorf("no answer on stdout:\n%s", r.stdout)
	}
	if lines := perTableLines(r.stdout + r.stderr); len(lines) > 0 {
		t.Errorf("a build off a terminal printed %d per-table lines, e.g. %q", len(lines), lines[0])
	}
}

// `-q` is a flag of every command, and `exec -q` says nothing about
// connecting. A run without it is the control: same script, same project.
func TestQuietFlagOnEveryCommand_Issue1860(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	mpr := jsonPurityFixture(t)
	dir := filepath.Dir(mpr)
	script := filepath.Join(dir, "q.mdl")
	if err := os.WriteFile(script, []byte("show modules;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	loud := runMainStreams(t, dir, "exec", script, "-p", mpr)
	if loud.exitCode != 0 || !strings.Contains(loud.stdout, "Connected to:") {
		t.Fatalf("control: exec without -q should report its connection (exit %d)\nstdout:\n%s\nstderr:\n%s",
			loud.exitCode, loud.stdout, loud.stderr)
	}
	for _, args := range [][]string{
		{"exec", script, "-p", mpr, "-q"},
		{"--quiet", "exec", script, "-p", mpr},
		{"-q", "-p", mpr, "-c", "show modules"},
	} {
		r := runMainStreams(t, dir, args...)
		if r.exitCode != 0 {
			t.Errorf("mxcli %v: exit %d\nstderr:\n%s", args, r.exitCode, r.stderr)
			continue
		}
		if strings.Contains(r.stdout+r.stderr, "Connected to:") {
			t.Errorf("mxcli %v: reported its connection under -q:\n%s%s", args, r.stdout, r.stderr)
		}
		if !strings.Contains(r.stdout, "System") {
			t.Errorf("mxcli %v: the answer went missing:\n%s", args, r.stdout)
		}
	}
}
