// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// `--json` is a contract with a program, not a style: whatever reaches stdout is
// handed to a JSON parser. It was broken on the query family — refs, callers,
// select and friends printed "Connected to: …", catalog progress and a header
// ahead of the payload, `context` ignored the flag entirely, and an empty answer
// came back as the words "(no references found)". Each command looked fine to a
// person, which is why nothing caught it.
//
// These run the real main() in a child process (see TestRunMainHelper), so the
// two streams are genuinely separate and an error path that ends in os.Exit is
// observable. The table is the guard: a new subcommand that prints its payload
// through the executor belongs in it.

// jsonPurityFixture copies the small v1 test project into a fresh directory, so
// the first command against it builds the catalog cold — the path that prints
// the most progress — and nothing is written next to the checked-in fixture.
func jsonPurityFixture(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "modelsdk", "mpr", "testdata", "v1-project", "App.mpr")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("fixture project not available: %v", err)
	}
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return mpr
}

type mainResult struct {
	stdout, stderr string
	exitCode       int
}

// runMainStreams runs mxcli in a child process and returns both streams and the
// exit code.
func runMainStreams(t *testing.T, dir string, args ...string) mainResult {
	t.Helper()
	enc, _ := json.Marshal(args)
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunMainHelper$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		runMainEnv+"="+string(enc),
		"MXCLI_LOG_DIR="+t.TempDir(),
		"MXCLI_QUIET=1",
	)
	cmd.Stdin = strings.NewReader("")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run mxcli %v: %v", args, err)
	}
	return mainResult{stdout: out.String(), stderr: errb.String(), exitCode: code}
}

func TestJSONFlagKeepsStdoutPureJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	mpr := jsonPurityFixture(t)
	dir := filepath.Dir(mpr)

	cases := []struct {
		name string
		args []string
		// empty: the answer is "nothing", which must still be JSON ([]), not a sentence.
		empty bool
	}{
		// First in the list on purpose: the catalog is cold, so this run builds
		// it and prints every "✓ Table: N" progress line.
		{name: "refs (cold catalog)", args: []string{"refs", "System.User"}},
		{name: "refs", args: []string{"refs", "System.User"}},
		{name: "refs, nothing found", args: []string{"refs", "Nope.Nothing"}, empty: true},
		{name: "callers", args: []string{"callers", "System.User"}, empty: true},
		{name: "callers transitive", args: []string{"callers", "System.User", "--transitive"}, empty: true},
		{name: "callees", args: []string{"callees", "System.User"}, empty: true},
		{name: "impact", args: []string{"impact", "System.User"}},
		{name: "impact, nothing found", args: []string{"impact", "Nope.Nothing"}, empty: true},
		{name: "context", args: []string{"context", "System.User"}},
		{name: "structure", args: []string{"structure"}},
		{name: "show entities", args: []string{"show", "entities"}},
		{name: "show modules", args: []string{"show", "modules"}},
		{name: "describe", args: []string{"describe", "entity", "System.User"}},
		// A query on a source-only table builds the source index it needs
		// (ako/mxcli#2200); it used to answer [] from a catalog without one.
		// The build's progress must stay off stdout.
		{name: "select, source-only table", args: []string{"-c", "select QualifiedName from CATALOG.source limit 1"}},
		{name: "search", args: []string{"search", "User"}},
		{name: "search, nothing found", args: []string{"search", "zzqqxxnomatch"}, empty: true},
		{name: "select", args: []string{"-c", "select Name from CATALOG.entities order by Name limit 3"}},
		{name: "select, nothing found", args: []string{"-c", "select Name from CATALOG.entities where Name = 'zz'"}, empty: true},
		{name: "-c show references", args: []string{"-c", "show references to System.User"}},
		{name: "show catalog tables", args: []string{"-c", "show catalog tables"}},
		{name: "show catalog status", args: []string{"-c", "show catalog status"}},
		// Empty listings: each of these answered "No … found." under --json.
		{name: "show widgets, none", args: []string{"-c", "show widgets"}, empty: true},
		{name: "show data transformers, none", args: []string{"-c", "show data transformers"}, empty: true},
		{name: "show import mappings, none", args: []string{"-c", "show import mappings"}, empty: true},
		{name: "show export mappings, none", args: []string{"-c", "show export mappings"}, empty: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"-p", mpr, "--json"}, tc.args...)
			r := runMainStreams(t, dir, args...)
			if r.exitCode != 0 {
				t.Fatalf("mxcli %v exited %d\nstderr:\n%s", args, r.exitCode, r.stderr)
			}
			assertPureJSON(t, args, r)
			if tc.empty && strings.TrimSpace(r.stdout) != "[]" {
				t.Errorf("mxcli %v: an empty answer must be [] on stdout, got:\n%s", args, r.stdout)
			}
		})
	}
}

// search has spelled JSON two ways. `--json` is canonical (the one the whole
// query family takes); `--format json` must keep working for existing callers
// and be exactly as clean.
func TestSearchFormatJSONAliasIsAsPureAsJSONFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	mpr := jsonPurityFixture(t)
	dir := filepath.Dir(mpr)

	canonical := runMainStreams(t, dir, "search", "-p", mpr, "User", "--json")
	alias := runMainStreams(t, dir, "search", "-p", mpr, "User", "--format", "json")
	for _, r := range []mainResult{canonical, alias} {
		if r.exitCode != 0 {
			t.Fatalf("search exited %d\nstderr:\n%s", r.exitCode, r.stderr)
		}
		assertPureJSON(t, []string{"search", "User"}, r)
	}
	if canonical.stdout != alias.stdout {
		t.Errorf("--json and --format json disagree:\n--json:\n%s\n--format json:\n%s", canonical.stdout, alias.stdout)
	}
}

// An error under --json is a non-zero exit with the message on stderr. A text
// line on stdout would be read as a (corrupt) payload.
func TestJSONFlagErrorsGoToStderr(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	mpr := jsonPurityFixture(t)
	dir := filepath.Dir(mpr)
	script := filepath.Join(dir, "s.mdl")
	if err := os.WriteFile(script, []byte("show entities;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"context", "Nope.Nothing"},
		{"describe", "entity", "Nope.Nothing"},
		{"-c", "select Nope from CATALOG.no_such_table"},
		// --json is a root flag, so diff accepted it and printed a text diff.
		// A command with no JSON output must refuse, not ignore it.
		{"diff", script},
	} {
		full := append([]string{"-p", mpr, "--json"}, args...)
		r := runMainStreams(t, dir, full...)
		if r.exitCode == 0 {
			t.Errorf("mxcli %v: expected a non-zero exit, got 0\nstdout:\n%s", full, r.stdout)
		}
		if s := strings.TrimSpace(r.stdout); s != "" && !json.Valid([]byte(s)) {
			t.Errorf("mxcli %v: error path wrote non-JSON to stdout:\n%s", full, r.stdout)
		}
		if strings.TrimSpace(r.stderr) == "" {
			t.Errorf("mxcli %v: error path said nothing on stderr", full)
		}
	}
}

// Control: text mode is for a person and must be exactly as before — the
// progress and headers still on stdout. Without this, a fix that simply sent
// everything to stderr (or dropped it) would pass the tests above and silently
// degrade interactive use.
func TestTextModeKeepsProgressOnStdout(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	mpr := jsonPurityFixture(t)
	dir := filepath.Dir(mpr)

	cases := []struct {
		args []string
		want []string // lines a person has always seen on stdout
	}{
		{[]string{"refs", "System.User"}, []string{"Connected to:", "Building catalog", "References to System.User"}},
		{[]string{"refs", "Nope.Nothing"}, []string{"Connected to:", "(no references found"}},
		{[]string{"callers", "System.User"}, []string{"Connected to:", "Callers of System.User", "(no callers found)"}},
		{[]string{"impact", "System.User"}, []string{"Impact analysis for System.User", "Summary:"}},
		{[]string{"context", "System.User"}, []string{"Connected to:", "## Context: System.User"}},
		{[]string{"show", "entities"}, []string{"Connected to:", "| Entity"}},
		// The commands above cached a full catalog; the first search adds the
		// source index (ako/mxcli#2004), and its progress is on stdout too.
		{[]string{"search", "User"}, []string{"Building catalog (source mode", "String Matches"}},
		{[]string{"search", "User"}, []string{"Loading cached catalog", "String Matches"}},
	}
	for _, tc := range cases {
		args := append([]string{"-p", mpr}, tc.args...)
		r := runMainStreams(t, dir, args...)
		if r.exitCode != 0 {
			t.Fatalf("mxcli %v exited %d\nstderr:\n%s", args, r.exitCode, r.stderr)
		}
		for _, w := range tc.want {
			if !strings.Contains(r.stdout, w) {
				t.Errorf("mxcli %v: text mode lost %q from stdout\nstdout:\n%s\nstderr:\n%s", args, w, r.stdout, r.stderr)
			}
		}
		if json.Valid([]byte(strings.TrimSpace(r.stdout))) {
			t.Errorf("mxcli %v: text mode emitted JSON:\n%s", args, r.stdout)
		}
	}
}

// assertPureJSON fails unless stdout is one JSON document and the progress that
// used to precede it went to stderr rather than vanishing.
func assertPureJSON(t *testing.T, args []string, r mainResult) {
	t.Helper()
	out := strings.TrimSpace(r.stdout)
	if out == "" {
		t.Fatalf("mxcli %v: --json printed nothing on stdout\nstderr:\n%s", args, r.stderr)
	}
	if !json.Valid([]byte(out)) {
		t.Fatalf("mxcli %v: stdout is not JSON:\n%s", args, firstLines(r.stdout, 8))
	}
	for _, leak := range []string{"Connected to:", "Loading cached catalog", "Building catalog", "✓ "} {
		if strings.Contains(r.stdout, leak) {
			t.Errorf("mxcli %v: progress %q on stdout", args, leak)
		}
	}
}

// `check` is the command an agent reaches for before `exec`, and its structured
// formats are the ones it parses. They were written to stderr — one document
// per phase — so stdout held only "Connected to:" and a multi-phase run
// produced several documents. The payload is now one document on stdout, and a
// failing check still exits non-zero with its violations in that document.
func TestCheckStructuredOutputIsOneDocumentOnStdout(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	mpr := jsonPurityFixture(t)
	dir := filepath.Dir(mpr)
	good := filepath.Join(dir, "good.mdl")
	bad := filepath.Join(dir, "bad.mdl")
	if err := os.WriteFile(good, []byte("show entities;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Parses, but names an entity the project does not have: fails in the
	// reference phase, after the syntax phase has already contributed.
	if err := os.WriteFile(bad, []byte("alter entity Nope.Thing add attribute X: String;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		args     []string
		wantExit int
	}{
		{[]string{"check", good, "--json"}, 0},
		{[]string{"check", good, "--format", "json"}, 0},
		{[]string{"check", good, "-p", mpr, "--json"}, 0},
		{[]string{"check", bad, "-p", mpr, "--json"}, 1},
		{[]string{"check", good, "-p", mpr, "--format", "sarif"}, 0},
	}
	for _, tc := range cases {
		r := runMainStreams(t, dir, tc.args...)
		if r.exitCode != tc.wantExit {
			t.Errorf("mxcli %v: exit %d, want %d\nstdout:\n%s\nstderr:\n%s", tc.args, r.exitCode, tc.wantExit, r.stdout, r.stderr)
			continue
		}
		dec := json.NewDecoder(strings.NewReader(r.stdout))
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			t.Errorf("mxcli %v: stdout is not JSON: %v\nstdout:\n%s\nstderr:\n%s", tc.args, err, r.stdout, r.stderr)
			continue
		}
		if dec.More() {
			t.Errorf("mxcli %v: more than one document on stdout:\n%s", tc.args, r.stdout)
		}
		if strings.Contains(r.stdout, "Connected to:") {
			t.Errorf("mxcli %v: progress on stdout:\n%s", tc.args, r.stdout)
		}
	}
}

// `search -q --format names` is the documented way to pipe names into another
// command. On a cold catalog the build's "✓ Table: N" lines were not gated by
// -q and landed on stdout ahead of the names.
func TestSearchQuietNamesIsOnlyNamesOnAColdCatalog(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	mpr := jsonPurityFixture(t)
	r := runMainStreams(t, filepath.Dir(mpr), "search", "-p", mpr, "User", "-q", "--format", "names")
	if r.exitCode != 0 {
		t.Fatalf("exit %d\nstderr:\n%s", r.exitCode, r.stderr)
	}
	lines := strings.Split(strings.TrimRight(r.stdout, "\n"), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatalf("no names on stdout\nstderr:\n%s", r.stderr)
	}
	for _, l := range lines {
		if parts := strings.Split(l, "\t"); len(parts) != 2 {
			t.Errorf("not a type<TAB>name line: %q", l)
		}
	}
	// Nor does stderr carry a line per table (#1860): a slow build reports
	// a heartbeat there instead (catalogHeartbeat).
	if lines := perTableLines(r.stderr); len(lines) > 0 {
		t.Errorf("a quiet build printed %d per-table lines on stderr, e.g. %q", len(lines), lines[0])
	}
}
