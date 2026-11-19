// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The files in testdata/mxcheck are `mx check --json` output from MxBuild
// 11.16.1 run on a clean `mxcli new` project with one
// mdl-examples/check-gap fixture applied (f10 is the control, which has no
// defect), and two_microflows_same_defect.json is the same defect written in two
// microflows. They are real output, not hand-written.

func load(t *testing.T, name string) CheckReport {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "mxcheck", name))
	if err != nil {
		t.Fatal(err)
	}
	r, err := ParseCheckJSON(data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if err := r.Verify(); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return r
}

func TestParseCheckJSONFixtures(t *testing.T) {
	for _, c := range []struct {
		file, code, document, element string
		stage                         int
	}{
		{"f01_dupvar.json", "CE0111", "Microflow 'F01'", "Create variable activity 'Create Integer/Long variable'", 1},
		{"f02_badparam.json", "CE1613", "Microflow 'F02'", "Property 'Parameter' of call microflow activity 'Helper'", 0},
		{"f05_undef.json", "CE0109", "Microflow 'F05'", "End event", 2},
		{"f06_assigntype.json", "CE0117", "Microflow 'F06'", "Create variable activity 'Create Integer/Long variable'", 3},
		{"f07_noreturn.json", "CE0109", "Microflow 'F07'", "End event", 2},
		{"f08_rettype.json", "CE0117", "Microflow 'F08'", "End event", 3},
	} {
		r := load(t, c.file)
		if len(r.Findings) != 1 {
			t.Fatalf("%s: %d findings, want 1", c.file, len(r.Findings))
		}
		f := r.Findings[0]
		l := f.Locations[0]
		if f.Severity != "error" || f.Code != c.code || l.DocumentName != c.document || l.ElementName != c.element ||
			l.ModuleName != "Sp" || f.Stage != c.stage || !f.Identified() {
			t.Errorf("%s: got %+v, want %s in %s at %s, stage %d", c.file, f, c.code, c.document, c.element, c.stage)
		}
	}
}

// The control: a project with no defect reports nothing and still verifies.
func TestParseCheckJSONClean(t *testing.T) {
	if r := load(t, "f10_clean.json"); len(r.Findings) != 0 || !r.HasDeclared || r.Declared != 0 {
		t.Errorf("clean report = %+v", r)
	}
}

// The reason findings are read from the JSON: two microflows with the same defect
// print the same text line, and only the IDs tell them apart.
func TestSameDefectInTwoMicroflowsHasTwoKeys(t *testing.T) {
	r := load(t, "two_microflows_same_defect.json")
	if len(r.Findings) != 2 {
		t.Fatalf("%d findings, want 2", len(r.Findings))
	}
	a, b := r.Findings[0], r.Findings[1]
	if a.Code != b.Code || a.Message != b.Message || a.Locations[0].ElementName != b.Locations[0].ElementName {
		t.Fatalf("the two findings are not textually identical: %+v / %+v", a, b)
	}
	if a.Key() == b.Key() {
		t.Error("two findings in different microflows must have different keys")
	}
	if a.Locations[0].DocumentName == b.Locations[0].DocumentName {
		t.Error("the documents must differ")
	}
}

// The key ignores the message and the names: the same finding reworded or in a
// renamed document is one finding. A different element is a different key.
func TestKeyIsIDsNotText(t *testing.T) {
	base := CheckFinding{Severity: "error", Code: "CE0109", Message: "one", Locations: []CheckLocation{
		{DocumentName: "Microflow 'A'", ElementName: "End event", UnitID: "u1", ElementID: "e1"}}}
	reworded := base
	reworded.Message = "another"
	reworded.Locations = []CheckLocation{{DocumentName: "Microflow 'Renamed'", ElementName: "End", UnitID: "u1", ElementID: "e1"}}
	other := base
	other.Locations = []CheckLocation{{UnitID: "u1", ElementID: "e2"}}
	if base.Key() != reworded.Key() {
		t.Error("message and names must not be part of the key")
	}
	if base.Key() == other.Key() {
		t.Error("a different element must be a different key")
	}
}

// Warnings and deprecations come with -w -d: the file has 16 warnings and one
// deprecation on an empty project. Some have no location at all.
func TestParseCheckJSONAllKinds(t *testing.T) {
	r := load(t, "f05_undef_all_kinds.json")
	if r.Count("error") != 1 || r.Count("warning") != 16 || r.Count("deprecation") != 1 {
		t.Errorf("counts = %d/%d/%d", r.Count("error"), r.Count("warning"), r.Count("deprecation"))
	}
	unidentified := 0
	for _, f := range r.Findings {
		if !f.Identified() {
			unidentified++
		}
	}
	if unidentified != 1 {
		t.Errorf("%d findings without an ID, want 1", unidentified)
	}
}

// The control for Verify: a list the reader does not know about, or a lost
// finding, must be reported rather than absorbed.
func TestVerifyCatchesAMismatch(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "mxcheck", "f05_undef.json"))
	if err != nil {
		t.Fatal(err)
	}
	lost := strings.Replace(string(data), `"total-problems": 1`, `"total-problems": 2`, 1)
	r, err := ParseCheckJSON([]byte(lost))
	if err != nil {
		t.Fatal(err)
	}
	if r.Verify() == nil {
		t.Error("Verify() = nil although mx declared a problem the report does not have")
	}
	if _, err := ParseCheckJSON([]byte(`{"serialization_version": 2}`)); err == nil {
		t.Error("an unknown serialization_version must be refused")
	}
	if _, err := ParseCheckJSON([]byte(`not json`)); err == nil {
		t.Error("invalid json must be an error")
	}
}

func TestStageOf(t *testing.T) {
	for code, want := range map[string]int{"CE1613": 0, "CE0111": 1, "CE0109": 2, "CE0067": 3, "CE9999": 3} {
		if got := StageOf(code); got != want {
			t.Errorf("StageOf(%s) = %d, want %d", code, got, want)
		}
	}
}

func TestCheckKindsArgs(t *testing.T) {
	got := strings.Join(CheckKinds{Warnings: true, BestPractice: true}.args("/x.json"), " ")
	if got != "-j /x.json -w -b" {
		t.Errorf("args = %q", got)
	}
}
