// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func renderReport(t *testing.T, format string, c *ConsistencyCheck) string {
	t.Helper()
	r := BuildReport("Shop", "now", nil)
	r.Consistency = c
	var b bytes.Buffer
	if err := GetReportFormatter(format).FormatReport(r, &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// ako/mxcli#2423: a project with consistency errors scores 100 on the lint
// rules (none fire here), and the report must still say it does not build.
func TestReportSaysAProjectWithConsistencyErrorsDoesNotBuild(t *testing.T) {
	c := &ConsistencyCheck{Ran: true, Source: "mx check", Errors: 1, Warnings: 1, Findings: []ConsistencyFinding{
		{Severity: "error", Code: "CE0117", Message: "Error(s) in expression.", Module: "Shop", Document: "Microflow 'ACT_X'", Element: "Change object"},
		{Severity: "warning", Code: "CW0001", Message: "a <b>warning</b> | with a pipe", Module: "Shop", Document: "Page 'P'"},
	}}
	md := renderReport(t, "markdown", c)
	for _, want := range []string{"**Builds:** no — 1 consistency error(s)", "100/100", "## Consistency Check",
		"The project does not build", "| error | CE0117 | 1 | Error(s) in expression. | Shop: Microflow 'ACT_X' |", `with a pipe`} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "a <b>warning</b> | with") {
		t.Error("a pipe in a message must not split the markdown cell")
	}
	h := renderReport(t, "html", c)
	if !strings.Contains(h, "no — 1 consistency error(s)") || strings.Contains(h, "<b>warning</b>") {
		t.Errorf("html must state the verdict and escape mx's text:\n%s", h)
	}
	var j map[string]any
	if err := json.Unmarshal([]byte(renderReport(t, "json", c)), &j); err != nil {
		t.Fatal(err)
	}
	if j["builds"] != false || j["consistency"].(map[string]any)["errors"].(float64) != 1 {
		t.Errorf("json builds=%v consistency=%v", j["builds"], j["consistency"])
	}
}

// A check that was not run is never shown as clean, and one whose output could
// not be read in full is not "builds" either.
func TestReportNeverCallsAnUncheckedProjectClean(t *testing.T) {
	cases := []struct {
		name  string
		c     *ConsistencyCheck
		want  string
		build any
	}{
		{"not run", &ConsistencyCheck{NotRun: "mx not found."}, "**Builds:** not checked", nil},
		{"incomplete", &ConsistencyCheck{Ran: true, Incomplete: "parsed 0 errors but mx check reported 2"}, "**Builds:** unknown", nil},
		{"clean", &ConsistencyCheck{Ran: true, Source: "mx check"}, "**Builds:** yes", true},
	}
	for _, tc := range cases {
		md := renderReport(t, "markdown", tc.c)
		if !strings.Contains(md, tc.want) {
			t.Errorf("%s: markdown lacks %q:\n%s", tc.name, tc.want, md)
		}
		var j map[string]any
		_ = json.Unmarshal([]byte(renderReport(t, "json", tc.c)), &j)
		if j["builds"] != tc.build {
			t.Errorf("%s: json builds = %v, want %v", tc.name, j["builds"], tc.build)
		}
	}
	if md := renderReport(t, "markdown", &ConsistencyCheck{NotRun: "mx not found."}); !strings.Contains(md, "mx not found.") {
		t.Errorf("the reason it was not run is shown:\n%s", md)
	}
	// A caller that did not ask for the check gets the report as before.
	if md := renderReport(t, "markdown", nil); strings.Contains(md, "Builds") || strings.Contains(md, "Consistency") {
		t.Errorf("no check requested, no section:\n%s", md)
	}
}

// A stale theme gives hundreds of one code (measured: 924 CE6083 of 1055 on
// testdata/pedapp). The page shows one row per code, errors first and the
// largest first, with a few locations; JSON keeps every finding.
func TestConsistencyFindingsAreGroupedByCode(t *testing.T) {
	c := &ConsistencyCheck{Ran: true}
	add := func(sev, code, doc string, n int) {
		for i := 0; i < n; i++ {
			c.Findings = append(c.Findings, ConsistencyFinding{Severity: sev, Code: code, Message: "m " + code, Module: "M", Document: doc, Element: "e"})
		}
	}
	add("warning", "CW0001", "Page 'A'", 50)
	add("error", "CE0463", "Page 'Home'", 3)
	for i := 0; i < 8; i++ {
		add("error", "CE6083", "Page 'P"+string(rune('a'+i))+"'", 2)
	}
	c.Errors = 19
	gs := c.Groups()
	if len(gs) != 3 || gs[0].Code != "CE6083" || gs[0].Count != 16 || gs[1].Code != "CE0463" || gs[2].Code != "CW0001" {
		t.Fatalf("groups: %+v", gs)
	}
	if len(gs[0].Locations) != maxGroupLocations || gs[0].More != 3 || len(gs[1].Locations) != 1 {
		t.Errorf("locations are distinct and capped: %+v", gs[:2])
	}
	md := renderReport(t, "markdown", c)
	for _, want := range []string{"| error | CE6083 | 16 | m CE6083 |", "and 3 more", "`mxcli fix widgets`", "--format json"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
	if n := strings.Count(md, "| error | CE6083"); n != 1 {
		t.Errorf("one row per code, got %d", n)
	}
	var j map[string]any
	_ = json.Unmarshal([]byte(renderReport(t, "json", c)), &j)
	if got := len(j["consistency"].(map[string]any)["findings"].([]any)); got != 69 {
		t.Errorf("json keeps every finding: %d", got)
	}
}

// --modules narrows the lint findings, not the check: the page says so.
func TestConsistencyCheckIsWholeProjectUnderModules(t *testing.T) {
	r := BuildReport("Shop", "now", nil)
	r.Modules = []string{"Sales"}
	r.Consistency = &ConsistencyCheck{Ran: true}
	var b bytes.Buffer
	_ = (&MarkdownReportFormatter{}).FormatReport(r, &b)
	if !strings.Contains(b.String(), wholeProjectNote) {
		t.Errorf("missing the whole-project note:\n%s", b.String())
	}
}
