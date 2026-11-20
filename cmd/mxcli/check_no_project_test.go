// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#2135 (mendixlabs/mxcli#1817): `mxcli check --help` offered
//
//	# Check syntax only (no project needed)
//	mxcli check script.mdl
//
// but run from a directory holding an .mpr, that invocation printed
// "Using project: ...\project.mpr (auto-detected in the current directory; pass
// -p to use another)" and resolved every reference against it — the global
// auto-detection sets -p before check runs — and nothing said how to get the
// syntax-only check the help advertised. Auto-detection stays (it is the CLI's
// documented resolution, mendixlabs/mxcli#1444); --no-project is the opt-out.

// noProjectFixture is a working directory holding exactly one .mpr, which
// auto-detection would pick, and a syntactically valid script. The .mpr is not
// a project, so any attempt to open it shows up as an error.
func noProjectFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Stray.mpr"), []byte("not a project"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "create persistent entity Shop.Customer (Name: string(100));\n"
	if err := os.WriteFile(filepath.Join(dir, "script.mdl"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCheckNoProject_SkipsAutoDetectedProject(t *testing.T) {
	dir := noProjectFixture(t)

	r := runMainStreams(t, dir, "check", "script.mdl", "--no-project")
	all := r.stdout + r.stderr
	if strings.Contains(all, "Using project:") {
		t.Errorf("--no-project still auto-detected the project:\n%s", all)
	}
	if strings.Contains(all, "Validating references against") {
		t.Errorf("--no-project still resolved references:\n%s", all)
	}
	if r.exitCode != 0 {
		t.Errorf("exit %d, want 0 for a syntactically valid script:\n%s", r.exitCode, all)
	}
	if !strings.Contains(r.stdout, "no project given") {
		t.Errorf("verdict does not say no project was used:\n%s", r.stdout)
	}
}

// Control for the test above: the same directory without the flag is the
// reported behaviour, and it is kept — the auto-detected project is used and
// named. If this stops printing "Using project:", the fixture no longer
// exercises auto-detection and the test above proves nothing.
func TestCheckNoProject_DefaultStillAutoDetects(t *testing.T) {
	dir := noProjectFixture(t)

	r := runMainStreams(t, dir, "check", "script.mdl")
	if !strings.Contains(r.stderr, "Using project:") {
		t.Fatalf("default check no longer auto-detects the project:\n%s%s", r.stdout, r.stderr)
	}
	// The notice is where the user meets the guess, so it names the way out.
	if !strings.Contains(r.stderr, "--no-project") {
		t.Errorf("auto-detection notice on check does not mention --no-project:\n%s", r.stderr)
	}
}

// --no-project with a flag that needs a project is a contradiction, refused
// before anything is checked rather than letting one silently win.
func TestCheckNoProject_ConflictsWithProjectFlags(t *testing.T) {
	dir := noProjectFixture(t)
	for _, extra := range [][]string{
		{"-p", "Stray.mpr"},
		{"--references"},
		{"--post-migration"},
	} {
		args := append([]string{"check", "script.mdl", "--no-project"}, extra...)
		r := runMainStreams(t, dir, args...)
		if r.exitCode == 0 {
			t.Errorf("%v: exit 0, want a refusal", extra)
		}
		if !strings.Contains(r.stderr, "--no-project") {
			t.Errorf("%v: refusal does not name --no-project:\n%s", extra, r.stderr)
		}
		if strings.Contains(r.stdout, "Checking syntax") {
			t.Errorf("%v: checked the script before refusing:\n%s", extra, r.stdout)
		}
	}
}

// The help is the contract: its syntax-only example has to be the invocation
// that is syntax-only wherever it is run, and it has to say a project in the
// current directory is otherwise picked up.
func TestCheckHelp_SyntaxOnlyExampleMatchesBehaviour(t *testing.T) {
	long := checkCmd.Long
	if strings.Contains(long, "no project needed)\n  mxcli check script.mdl\n") {
		t.Errorf("help still offers a bare `mxcli check script.mdl` as syntax-only")
	}
	if !strings.Contains(long, "mxcli check script.mdl --no-project") {
		t.Errorf("help has no --no-project example")
	}
	if !strings.Contains(long, "auto-detect") {
		t.Errorf("help does not say a project in the current directory is auto-detected")
	}
	if checkCmd.Flags().Lookup("no-project") == nil {
		t.Errorf("check has no --no-project flag")
	}
}
