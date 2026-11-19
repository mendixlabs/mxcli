// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// everythingGiven is a readiness input where every report that decides a
// blocking question was given and found nothing.
func everythingGiven() ReadinessInput {
	return ReadinessInput{
		Quality:       &linter.JSONReport{Consistency: &linter.ConsistencyCheck{Ran: true}},
		Release:       &ReleaseData{},
		Dependencies:  &DependencyData{VulnChecked: true, Line: "LTS", MendixVersion: "10.24.3"},
		Configuration: &ConfigurationData{},
		Security:      &SecurityData{},
		Tests:         &TestsData{HasResults: true},
		Traceability:  &TraceabilityData{},
		Annotations:   map[string]*Annotations{},
	}
}

// The verdict follows the stated rule (ako/mxcli#2427).
func TestReadinessVerdict(t *testing.T) {
	if d := ComputeReadiness(everythingGiven()); d.Verdict != VerdictReady || len(d.Blocking)+len(d.Risks) != 0 {
		t.Errorf("all clear: verdict %q, blocking %v, risks %v", d.Verdict, d.Blocking, d.Risks)
	}

	// A report that decides a blocking question and was not given is never passing.
	in := everythingGiven()
	in.Release = nil
	if d := ComputeReadiness(in); d.Verdict != VerdictIncomplete {
		t.Errorf("release not given: verdict %q, want %q", d.Verdict, VerdictIncomplete)
	}
	in = everythingGiven()
	in.Quality.Consistency = &linter.ConsistencyCheck{Ran: false, NotRun: "no mx for 11.16.1"}
	if d := ComputeReadiness(in); d.Verdict != VerdictIncomplete {
		t.Errorf("mx check did not run: verdict %q, want %q", d.Verdict, VerdictIncomplete)
	}
	// Performance and architecture decide no blocking question.
	in = everythingGiven()
	in.Architecture = &ArchitectureData{Cycles: []Cycle{{Modules: []string{"Billing", "Sales"}}}}
	if d := ComputeReadiness(in); d.Verdict != VerdictReadyRisks || len(d.Risks) != 1 {
		t.Errorf("a cycle is a risk: verdict %q, risks %v", d.Verdict, d.Risks)
	}
}

func TestReadinessBlockingItems(t *testing.T) {
	in := everythingGiven()
	in.Quality.Consistency.Errors = 3
	in.Release.Changes = []ReleaseChange{
		{Effect: EffectDestroys, Kind: "entity", Name: "Sales.Order", What: "Sales.Order is removed"},
		{Effect: EffectDestroys, Kind: "attribute", Name: "Sales.Order.Note", What: "Sales.Order.Note is removed"},
	}
	in.Dependencies.Libraries = []DepLibrary{{Group: "org.x", Name: "lib", Version: "1.0",
		Vulns: []DepVuln{{ID: "GHSA-1", Severity: "HIGH"}, {ID: "GHSA-2", Severity: "LOW"}}}}
	in.Configuration.Endpoints = []ConfigEndpoint{{Where: "Sales.Rates", Kind: "REST client base URL", Value: "http://localhost:9000", Fixed: true, Host: "localhost"},
		{Where: "Sales.Api", Kind: "REST client base URL", Value: "https://api.partner.com", Fixed: true, Host: "external"}}
	in.Security.Findings = []Finding{{ID: "demo-users", Severity: "high", Title: "Demo users are enabled"},
		{ID: "key", Severity: "high", Title: "A committed key"}}
	in.Tests.Tests = []TestInfo{{Name: "t_order", Status: "failed"}, {Name: "t_ok", Status: "passed"}}
	in.Traceability.Milestones = []TraceMilestone{{Name: "M2", Requirements: []TraceRequirement{
		{ID: "US-1", Title: "Pay", Status: StatusNotBuilt}, {ID: "US-2", Title: "Track", Status: StatusBuilt, Tests: []string{"t"}}}},
		{Name: "M3", Requirements: []TraceRequirement{{ID: "US-9", Title: "Later", Status: StatusNotBuilt}}}}
	in.Milestone = "M2"
	// Verdicts recorded on the other reports, and one sign-off here.
	in.Annotations["release"] = &Annotations{Items: map[string]Note{"release:attribute:Sales.Order.Note": {Status: "accepted", Note: "never filled"}}}
	in.Annotations["security"] = &Annotations{Items: map[string]Note{"finding:key": {Status: "fixed"}}}
	in.SignOff = &Annotations{Items: map[string]Note{"readiness:test:t_order": {Status: "signed off", Note: "known flake, ticket 12"}}}

	d := ComputeReadiness(in)
	got := map[string]string{}
	for _, b := range d.Blocking {
		got[b.Key] = b.Source
	}
	for key, src := range map[string]string{
		"builds": "quality", "release:entity:Sales.Order": "release", "vuln:GHSA-1": "dependencies",
		"endpoint:Sales.Rates": "configuration", "finding:demo-users": "security", "requirement:US-1": "traceability",
	} {
		if got[key] != src {
			t.Errorf("want blocking %s from %s; blocking: %v", key, src, got)
		}
	}
	for _, key := range []string{"release:attribute:Sales.Order.Note", "vuln:GHSA-2", "endpoint:Sales.Api", "finding:key", "test:t_order", "requirement:US-9"} {
		if _, ok := got[key]; ok {
			t.Errorf("%s is not blocking", key)
		}
	}
	accepted := map[string]string{}
	for _, a := range d.Accepted {
		accepted[a.Key] = a.Note
	}
	if accepted["release:attribute:Sales.Order.Note"] != "accepted: never filled" || accepted["test:t_order"] != "signed off: known flake, ticket 12" {
		t.Errorf("accepted items carry what accepts them: %v", accepted)
	}
	if d.Verdict != VerdictNotReady {
		t.Errorf("verdict %q, want %q", d.Verdict, VerdictNotReady)
	}

	env, _ := NewEnvelope("readiness", "App", "now", "test", d, ReadinessMethod, ReadinessLimits, nil)
	doc, err := RenderReadiness(env, nil)
	if err != nil {
		t.Fatal(err)
	}
	var html bytes.Buffer
	_ = RenderHTML(&html, doc)
	mustContain(t, html.String(), `<a href="release.html">Release</a>`, "readiness:vuln:GHSA-1", "A penetration test", "not ready",
		// a reader with many prose decisions should not think they were not read (ako/mxcli#2451)
		"A decision written only as prose in the brain is not read as accepting anything",
		"any one makes it not ready")
}

// A link in a cell is rendered only to a report page in the same folder:
// annotation text is written by people.
func TestInlineLinksOnlyToReportPages(t *testing.T) {
	got := inline("see [Security](security.html) and [x](javascript:alert(1)) and [y](https://evil.example/a.html)")
	if !strings.Contains(got, `<a href="security.html">Security</a>`) {
		t.Errorf("a page link renders: %s", got)
	}
	if strings.Contains(got, `href="javascript`) || strings.Contains(got, `href="https`) {
		t.Errorf("only relative .html pages are linked: %s", got)
	}
}
