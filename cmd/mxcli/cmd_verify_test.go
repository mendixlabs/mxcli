// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// The JSON is what `mxcli lint --format json` and `mxcli check --format json`
// print (recorded from a run on a scaffolded project).
const lintJSON = `{
  "violations": [
    {"ruleId": "SEC013", "severity": "warning", "message": "Security level is Off", "module": "", "document": "ProjectSecurity", "documentType": "security", "suggestion": "ALTER APP SECURITY"},
    {"ruleId": "MDL098", "severity": "error", "message": "return reads $Nope", "module": "", "document": "Sp.A", "documentType": "microflow"}
  ],
  "summary": {"total": 2, "errors": 1, "warnings": 1, "infos": 0, "hints": 0}
}`

func TestParseViolations(t *testing.T) {
	fs, err := parseViolations("lint", []byte(lintJSON))
	if err != nil || len(fs) != 2 {
		t.Fatalf("got %v, %v", fs, err)
	}
	if fs[1].Code != "MDL098" || fs[1].Severity != "error" || fs[1].Document != "Sp.A" || fs[1].Stage != "lint" || fs[1].MxStage != -1 {
		t.Errorf("finding = %+v", fs[1])
	}
	if fs[0].Key == fs[1].Key {
		t.Error("two different findings share a key")
	}
	// An empty report is an empty list, not nil: it must encode as [] for a reader.
	empty, err := parseViolations("lint", []byte(`{"violations": [], "summary": {}}`))
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("empty report = %v, %v", empty, err)
	}
	// Control: output that is not the report (a crash, a banner) is an error, never "no findings".
	if _, err := parseViolations("lint", []byte("Error: boom")); err == nil {
		t.Error("unparseable output was read as an empty report")
	}
}

func TestParseLintBaselineSummary(t *testing.T) {
	stderr := "Connected to: x\nLint baseline /p/lint.json: 2 new, 140 already known, 3 fixed\n"
	if n, ok := parseLintBaselineSummary(stderr); !ok || n != 140 {
		t.Errorf("got %d, %v, want 140", n, ok)
	}
	if n, ok := parseLintBaselineSummary("Lint baseline lint.json: 0 new, 5 already known (a scoped run: fixed findings are not counted)"); !ok || n != 5 {
		t.Errorf("scoped form: got %d, %v", n, ok)
	}
	if _, ok := parseLintBaselineSummary("nothing"); ok {
		t.Error("a missing summary line must not read as zero")
	}
}

func TestMxVersionOf(t *testing.T) {
	for in, want := range map[string]string{
		"/root/.mxcli/mxbuild/11.16.1/modeler/mx": "11.16.1",
		`C:\mx\11.12.1.5\modeler\mx.exe`:          "",
		"/usr/bin/mx":                             "",
	} {
		if in == `C:\mx\11.12.1.5\modeler\mx.exe` {
			continue // a Windows path is split on its own separator by filepath.ToSlash only on Windows
		}
		if got := verifyMxVersionOf(in); got != want {
			t.Errorf("verifyMxVersionOf(%q) = %q, want %q", in, got, want)
		}
	}
}

// mx check warnings and deprecations are not part of the gate, and a finding's
// module is part of its document name.
func TestMxFindingsKeepsErrorsOnly(t *testing.T) {
	data := `{"serialization_version": 1,
	 "errors": [{"code": "CE0109", "message": "Undefined variable 'X'.", "locations": [{"module-name": "Sp", "document-name": "Microflow 'F'", "element-name": "End event", "element-id": "e1", "unit-id": "u1"}]}],
	 "warnings": [{"code": "CW0055", "message": "No on click", "locations": []}],
	 "deprecations": [], "total-problems": 2}`
	r, err := docker.ParseCheckJSON([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	fs := mxFindings(r)
	if len(fs) != 1 {
		t.Fatalf("got %d findings, want the error only", len(fs))
	}
	f := fs[0]
	if f.Code != "CE0109" || f.Document != "Sp.Microflow 'F'" || f.Element != "End event" || f.Unit != "u1" || f.MxStage != 2 {
		t.Errorf("finding = %+v", f)
	}
	if !strings.Contains(f.Key, "u1/e1") {
		t.Errorf("the key must carry the IDs, got %q", f.Key)
	}
}

// With nothing runnable the command still prints exactly one JSON document on
// stdout, exits 0, and reports why each stage was skipped. Flag errors exit 1
// with nothing on stdout.
func TestVerifyOutputContract(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli")
	}
	mpr := jsonPurityFixture(t)
	dir := filepath.Dir(mpr)

	r := runMainStreams(t, dir, "verify", "-p", mpr, "--stages", "check", "--format", "json")
	if r.exitCode != 0 {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", r.exitCode, r.stdout, r.stderr)
	}
	dec := json.NewDecoder(strings.NewReader(r.stdout))
	var doc struct {
		Exit   int `json:"exit"`
		Stages []struct {
			Name, Status, Note string
		} `json:"stages"`
	}
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, r.stdout)
	}
	if dec.More() {
		t.Errorf("more than one document on stdout:\n%s", r.stdout)
	}
	if len(doc.Stages) != 4 || doc.Stages[0].Name != "check" || doc.Stages[0].Status != "skipped" || doc.Stages[0].Note != "no scripts given" {
		t.Errorf("stages = %+v", doc.Stages)
	}

	for _, args := range [][]string{
		{"verify", "-p", mpr, "--stages", "smoke"},
		{"verify", "-p", mpr, "--format", "sarif"},
		{"verify", "-p", mpr, "--fail-on", "info"},
	} {
		r := runMainStreams(t, dir, args...)
		if r.exitCode != 1 || strings.TrimSpace(r.stdout) != "" {
			t.Errorf("mxcli %v: exit %d, stdout %q; want exit 1 and nothing on stdout\nstderr:\n%s", args, r.exitCode, r.stdout, r.stderr)
		}
	}
}

// The baselines have to be committed to be any use: a fresh clone, or a reaped
// cloud session, starts with none, and every run then reports every finding as
// new. So the default location must not be one the generated .gitignore hides.
func TestDefaultVerifyBaselineDirIsNotGitIgnored(t *testing.T) {
	dir := defaultVerifyBaselineDir(filepath.Join("proj", "App.mpr"))
	if got := filepath.Base(dir); got != "verify-baseline" || filepath.Base(filepath.Dir(dir)) != "proj" {
		t.Fatalf("default dir = %q", dir)
	}
	for _, line := range strings.Split(mendixGitignore, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pattern := strings.Trim(line, "/")
		if pattern == "verify-baseline" || pattern == "*" || strings.HasPrefix(pattern, "verify") {
			t.Errorf(".gitignore pattern %q hides the default baseline directory", line)
		}
	}
}

func TestVerifyStageSelection(t *testing.T) {
	join := func(s []string, err error) string {
		if err != nil {
			return "ERR: " + err.Error()
		}
		return strings.Join(s, ",")
	}
	cases := []struct {
		name         string
		stages       []string
		stagesSet    bool
		through      string
		apply, files bool
		want         string
	}{
		{"default without scripts", nil, false, "", false, false, "check,lint,mxcheck"},
		{"default with --apply goes through mxcheck", nil, false, "", true, true, "check,exec,lint,mxcheck"},
		{"--through exec", nil, false, "exec", true, true, "check,exec"},
		{"--through lint", nil, false, "lint", true, true, "check,exec,lint"},
		{"--stages is an explicit set", []string{"lint"}, true, "", false, false, "lint"},
		{"--stages and --through are two answers", []string{"lint"}, true, "lint", false, false, "ERR"},
		{"--apply needs scripts", nil, false, "", true, false, "ERR"},
		{"exec never runs without --apply (through)", nil, false, "exec", false, true, "ERR"},
		{"exec never runs without --apply (stages)", []string{"exec"}, true, "", false, true, "ERR"},
		{"unknown stage", nil, false, "smoke", true, true, "ERR"},
	}
	for _, c := range cases {
		got := join(verifyStageSelection(c.stages, c.stagesSet, c.through, c.apply, c.files))
		if strings.HasPrefix(c.want, "ERR") != strings.HasPrefix(got, "ERR") || (!strings.HasPrefix(c.want, "ERR") && got != c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// Bad flag combinations are usage errors: exit 1 and nothing on stdout.
func TestVerifyApplyFlagErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli")
	}
	mpr := jsonPurityFixture(t)
	dir := filepath.Dir(mpr)
	for _, args := range [][]string{
		{"verify", "-p", mpr, "--apply"},
		{"verify", "-p", mpr, "--stages", "lint", "--through", "lint"},
		{"verify", "-p", mpr, "--through", "exec"},
		{"verify", "-p", mpr, "--apply", "x.mdl", "--update-baseline"},
	} {
		r := runMainStreams(t, dir, args...)
		if r.exitCode != 1 || strings.TrimSpace(r.stdout) != "" {
			t.Errorf("mxcli %v: exit %d, stdout %q; want exit 1 and nothing on stdout\nstderr:\n%s", args, r.exitCode, r.stdout, r.stderr)
		}
	}
}

// What `mxcli check a.mdl d.mdl` prints when the second file fails (recorded from a
// real run): the finding keeps the violation and the verdict, not the progress or
// the colour codes.
func TestVerifyCleanCheckText(t *testing.T) {
	raw := "Checking syntax: c.mdl\n\x1b[32m✓\x1b[0m Syntax OK (1 statements)\nConnected to: /tmp/x/Spike.mpr (Mendix 11.16.1)\n\n" +
		"Validating references against: Spike.mpr\n(Note: References to objects created within the script are skipped)\n✓ All references valid\n" +
		"Building catalog (fast mode)...\n✓ Catalog ready (0.1s)\n\nCheck passed!\n\n" +
		"Checking d.mdl against a scratch copy of the project holding what the 1 file(s) before it write (the project itself is not changed)\n" +
		"1 of 2 files failed: d.mdl\n\nWARNING: This is a vibe-coded PoC, alpha quality, use with caution.\n\n\x1b[36m(no module)\x1b[0m\n-----------\n" +
		"  \x1b[31m✗\x1b[0m return reads $Nope, which nothing in this microflow defines \x1b[2m[MDL098]\x1b[0m\n      at \x1b[2mSp.ACT_D\x1b[0m\n\n1 issues: 1 errors, 0 warnings, 0 info\n"
	got := verifyCleanCheckText(raw)
	for _, want := range []string{"1 of 2 files failed: d.mdl", "return reads $Nope", "[MDL098]", "at Sp.ACT_D", "1 issues: 1 errors"} {
		if !strings.Contains(got, want) {
			t.Errorf("cleaned text lacks %q:\n%s", want, got)
		}
	}
	for _, drop := range []string{"\x1b[", "Connected to", "Catalog ready", "Check passed", "vibe-coded"} {
		if strings.Contains(got, drop) {
			t.Errorf("cleaned text still has %q:\n%s", drop, got)
		}
	}
}
