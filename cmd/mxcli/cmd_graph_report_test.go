// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

func TestRenderGraphMarkdown(t *testing.T) {
	sections := []graphSectionResult{
		{Title: "God nodes", Columns: []string{"Asset", "Degree"}, Rows: [][]any{
			{"M.Order", int64(42)},
			{"M.Pipe|Sep", int64(7)}, // pipe must be escaped
		}},
		{Title: "Empty", Columns: []string{"X"}, Rows: nil},
	}
	out := renderGraphMarkdown("app.mpr", 15, false, sections)

	for _, want := range []string{
		"# Graph report — app.mpr",
		"framework modules excluded",
		"## God nodes",
		"| Asset | Degree |",
		"| M.Order | 42 |",
		`| M.Pipe\|Sep | 7 |`, // escaped pipe
		"## Empty",
		"_(none)_",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown missing %q:\n%s", want, out)
		}
	}
}

func TestRenderGraphJSON(t *testing.T) {
	sections := []graphSectionResult{
		{Title: "God nodes", Columns: []string{"Asset", "Degree"}, Rows: [][]any{{"M.Order", int64(42)}}},
	}
	out := renderGraphJSON("app.mpr", sections)

	var parsed struct {
		Project  string `json:"project"`
		Sections []struct {
			Title string           `json:"title"`
			Rows  []map[string]any `json:"rows"`
		} `json:"sections"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if parsed.Project != "app.mpr" || len(parsed.Sections) != 1 {
		t.Fatalf("unexpected shape: %+v", parsed)
	}
	if got := parsed.Sections[0].Rows[0]["Asset"]; got != "M.Order" {
		t.Errorf("Asset = %v, want M.Order", got)
	}
}

// An --exclude'd module must not appear in the coupling section on EITHER end
// of an edge. The coupling query used to carry no filter at all, so an
// excluded OIDC still showed as a coupling source (ako/mxcli#2222).
func TestGraphReportExcludeFiltersCouplingBothEnds(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	for _, r := range [][3]string{
		{"OIDC.Login", "App.User", "call"},
		{"App.Home", "OIDC.Config", "call"},
		{"App.Home", "Core.Order", "call"},
	} {
		if _, err := cat.CatalogDB().Exec(`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetName, RefKind)
			VALUES ('MICROFLOW', 'x', ?, 'MICROFLOW', ?, ?)`, r[0], r[1], r[2]); err != nil {
			t.Fatal(err)
		}
	}
	var coupling string
	for _, s := range graphReportSections(false, []string{"OIDC"}) {
		if strings.HasPrefix(s.title, "Module coupling") {
			coupling = strings.Replace(s.query, "?", "50", 1)
		}
	}
	res, err := cat.Query(coupling)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("want only App→Core, got %v", res.Rows)
	}
	if res.Rows[0][0] != "App" || res.Rows[0][1] != "Core" {
		t.Errorf("got %v", res.Rows[0])
	}
}
