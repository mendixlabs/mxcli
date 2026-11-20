// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	ds := []Defect{
		{Name: "A", Fragment: "@caption 'T_A' declare $a Integer = $Nope;"},
		{Name: "B", Fragment: "@caption 'T_B' declare $b Integer = 'x';"},
		{Name: "N", NoReturn: true},
	}
	g := Generate(ds)
	// 3 singles + 3*2 ordered pairs.
	if len(g.Docs) != 9 {
		t.Errorf("got %d documents, want 9: %v", len(g.Docs), g.Docs)
	}
	if !strings.HasPrefix(g.Script, "mdl 1;\n") {
		t.Errorf("the script must open with the language header (the conformance gate needs it):\n%s", g.Script)
	}
	if !strings.Contains(g.Script, "Z_baseline") {
		t.Error("no clean baseline document")
	}
	if got := g.Docs["X_B__A"]; !reflect.DeepEqual(got, []string{"B", "A"}) {
		t.Errorf("X_B__A holds %v, want [B A] in that order", got)
	}
	// The pair documents hold both fragments, in the order named.
	line := lineOf(g.Script, "Mk.X_B__A")
	if i, j := strings.Index(line, "T_B"), strings.Index(line, "T_A"); i < 0 || j < 0 || i > j {
		t.Errorf("X_B__A must hold the B fragment before the A fragment: %s", line)
	}
	// A no-return defect leaves out the final return; the others keep it.
	if strings.Contains(lineOf(g.Script, "Mk.S_N "), "return 1;") {
		t.Errorf("S_N must have no final return: %s", lineOf(g.Script, "Mk.S_N "))
	}
	if !strings.Contains(lineOf(g.Script, "Mk.S_A "), "return 1;") {
		t.Errorf("S_A must keep its final return: %s", lineOf(g.Script, "Mk.S_A "))
	}
	if strings.Contains(lineOf(g.Script, "Mk.X_A__N"), "return 1;") {
		t.Errorf("a pair holding the no-return defect has no final return either: %s", lineOf(g.Script, "Mk.X_A__N"))
	}
}

func lineOf(script, needle string) string {
	for _, l := range strings.Split(script, "\n") {
		if strings.Contains(l, needle) {
			return l
		}
	}
	return ""
}

func TestRefused(t *testing.T) {
	out := "Created microflow: Mk.S_UNDEF\nError: microflow 'Mk.S_FUNC' has validation errors:\n  - [MDL044] ...\nNothing was written"
	if got := Refused(out); got != "FUNC" {
		t.Errorf("Refused = %q, want FUNC", got)
	}
	if got := Refused("Error: no such project"); got != "" {
		t.Errorf("an unrelated failure must not name a defect, got %q", got)
	}
}

func TestParseFindings(t *testing.T) {
	raw := []byte(`{"errors":[
	  {"code":"CE0109","locations":[{"document-name":"Microflow 'S_UNDEF'","element-name":"Create variable activity 'T_UNDEF'"}]},
	  {"code":"CE0149","locations":[{"document-name":"Page 'P_role'","element-name":"Container 'c'"}]},
	  {"code":"CE1613","locations":[{"document-name":"Microflow 'M_ref'","element-name":"x"},{"document-name":"Microflow 'M_ref2'","element-name":"y"}]}
	]}`)
	f, err := ParseFindings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(f["S_UNDEF"]) != 1 || f["S_UNDEF"][0].Code != "CE0109" {
		t.Errorf("a microflow is keyed by its bare name: %v", f)
	}
	if len(f["Page 'P_role'"]) != 1 {
		t.Errorf("a document that is not a microflow keeps its full name: %v", f)
	}
	if len(f["M_ref"]) != 1 || len(f["M_ref2"]) != 1 {
		t.Errorf("an error with two locations is on both documents: %v", f)
	}
	if _, err := ParseFindings([]byte("not json")); err == nil {
		t.Error("a report that is not JSON must be refused")
	}
}

// staged builds the findings of a checker that reports in stages: a document shows only
// the errors of the earliest stage any of its defects belongs to.
func staged(ds []Defect, docs map[string][]string, stage map[string]int, code map[string]string) Findings {
	f := Findings{}
	for doc, defects := range docs {
		min := 99
		for _, d := range defects {
			if stage[d] < min {
				min = stage[d]
			}
		}
		for _, d := range defects {
			if stage[d] == min {
				f[doc] = append(f[doc], Finding{Code: code[d]})
			}
		}
	}
	return f
}

func defectsNamed(names ...string) []Defect {
	var ds []Defect
	for _, n := range names {
		ds = append(ds, Defect{Name: n})
	}
	return ds
}

func TestAnalyzeFindsTheStages(t *testing.T) {
	ds := defectsNamed("DUP", "UNDEF", "TYPE", "LOOP")
	g := Generate(ds)
	// DUP < UNDEF < {TYPE, LOOP}: the same shape MxBuild showed.
	f := staged(ds, g.Docs,
		map[string]int{"DUP": 1, "UNDEF": 2, "TYPE": 3, "LOOP": 3},
		map[string]string{"DUP": "CE1", "UNDEF": "CE2", "TYPE": "CE3", "LOOP": "CE4"})
	r := Analyze(ds, g.Docs, f)
	want := [][]string{{"DUP"}, {"UNDEF"}, {"TYPE", "LOOP"}}
	if !reflect.DeepEqual(r.Stages, want) {
		t.Errorf("Stages = %v, want %v", r.Stages, want)
	}
	if got := r.Hides["DUP"]; !reflect.DeepEqual(got, []string{"LOOP", "TYPE", "UNDEF"}) {
		t.Errorf("DUP hides %v", got)
	}
	if got := r.Hides["UNDEF"]; !reflect.DeepEqual(got, []string{"LOOP", "TYPE"}) {
		t.Errorf("UNDEF hides %v", got)
	}
	if len(r.Hides["TYPE"]) != 0 || len(r.Hides["LOOP"]) != 0 {
		t.Errorf("defects reported together hide nothing: %v", r.Hides)
	}
	if len(r.OrderDependent) != 0 || len(r.Silent) != 0 || len(r.SharedCodes) != 0 {
		t.Errorf("unexpected: order %v silent %v shared %v", r.OrderDependent, r.Silent, r.SharedCodes)
	}
}

func TestAnalyzeCheckerThatDoesNotStage(t *testing.T) {
	ds := defectsNamed("A", "B", "C")
	g := Generate(ds)
	f := Findings{}
	for doc, defects := range g.Docs {
		for _, d := range defects {
			f[doc] = append(f[doc], Finding{Code: "CE-" + d})
		}
	}
	r := Analyze(ds, g.Docs, f)
	if len(r.Hides) != 0 || !reflect.DeepEqual(r.Stages, [][]string{{"A", "B", "C"}}) {
		t.Errorf("a checker that reports everything has one stage and hides nothing: %v %v", r.Stages, r.Hides)
	}
}

func TestAnalyzeOrderDependence(t *testing.T) {
	ds := defectsNamed("A", "B")
	g := Generate(ds)
	// Whichever defect comes first in a document is the one reported.
	f := Findings{
		"S_A": {{Code: "CA"}}, "S_B": {{Code: "CB"}},
		"X_A__B": {{Code: "CA"}}, "X_B__A": {{Code: "CB"}},
	}
	r := Analyze(ds, g.Docs, f)
	if !reflect.DeepEqual(r.OrderDependent, [][2]string{{"A", "B"}}) {
		t.Errorf("OrderDependent = %v, want [[A B]]", r.OrderDependent)
	}
	if len(r.Hides) != 0 {
		t.Errorf("an order-dependent pair is not a 'hides' relation: %v", r.Hides)
	}
}

func TestAnalyzeSilentAndSharedCodes(t *testing.T) {
	ds := defectsNamed("A", "B", "C")
	g := Generate(ds)
	f := Findings{
		"S_A": {{Code: "CX"}}, "S_B": {{Code: "CX"}}, // A and B raise one code
		// C's own document is clean: the checker does not see it
	}
	r := Analyze(ds, g.Docs, f)
	if !reflect.DeepEqual(r.Silent, []string{"C"}) {
		t.Errorf("Silent = %v, want [C]", r.Silent)
	}
	if !reflect.DeepEqual(r.SharedCodes, []string{"CX"}) {
		t.Errorf("SharedCodes = %v, want [CX]", r.SharedCodes)
	}
	for _, st := range r.Stages {
		if contains(st, "C") {
			t.Errorf("a defect the checker leaves clean is not placed in a stage: %v", r.Stages)
		}
	}
}

func TestReport(t *testing.T) {
	ds := defectsNamed("DUP", "UNDEF")
	g := Generate(ds)
	f := staged(ds, g.Docs, map[string]int{"DUP": 1, "UNDEF": 2}, map[string]string{"DUP": "CE0111", "UNDEF": "CE0109"})
	var b bytes.Buffer
	Report(&b, ds, Analyze(ds, g.Docs, f))
	for _, want := range []string{"DUP       CE0111", "DUP       hides UNDEF", "no pair differs when swapped", "1  DUP", "2  UNDEF"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, b.String())
		}
	}
}

func TestCopyProjectLeavesOutTheBinaryAndGit(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "copy")
	for path, content := range map[string]string{
		"App.mpr": "mpr", "mprcontents/ab/unit.mxunit": "unit",
		"mxcli": "a 110 MB binary", ".git/HEAD": "ref", "modules/mxcli": "a file that merely shares the name",
	} {
		p := filepath.Join(src, path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyProject(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{"App.mpr", "mprcontents/ab/unit.mxunit", "modules/mxcli"} {
		if _, err := os.Stat(filepath.Join(dst, kept)); err != nil {
			t.Errorf("%s was not copied", kept)
		}
	}
	for _, left := range []string{"mxcli", ".git"} {
		if _, err := os.Stat(filepath.Join(dst, left)); err == nil {
			t.Errorf("%s was copied; only the project root's copy is skipped", left)
		}
	}
}

func TestSelectDefectsAndFlags(t *testing.T) {
	if _, err := selectDefects("UNDEF,NOPE"); err == nil || !strings.Contains(err.Error(), "NOPE") {
		t.Errorf("an unknown defect must be named, got %v", err)
	}
	if _, err := selectDefects("UNDEF"); err == nil {
		t.Error("one defect cannot be compared with anything")
	}
	if ds, err := selectDefects("TYPE,UNDEF"); err != nil || len(ds) != 2 || ds[0].Name != "TYPE" {
		t.Errorf("a subset keeps the order given: %v %v", ds, err)
	}
	var out, errOut bytes.Buffer
	if err := run(nil, &out, &errOut); err == nil || !strings.Contains(err.Error(), "--project") {
		t.Errorf("--project and --mx are required, got %v", err)
	}
}
