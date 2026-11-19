// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bytes"
	"strings"
	"testing"
)

func renderDeps(t *testing.T, d DependencyData, ann *Annotations) string {
	t.Helper()
	env, _ := NewEnvelope("dependencies", "Shop", "now", "t", d, DependenciesMethod, DependenciesLimits, nil)
	doc, err := RenderDependencies(env, ann)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	_ = RenderMarkdown(&b, doc)
	return b.String()
}

// ako/mxcli#2425: high and critical advisories lead the page and count as
// blocking; an accepted one does not; low and moderate never do.
func TestDependenciesVerdictCountsHighAndCritical(t *testing.T) {
	d := DependencyData{MendixVersion: "10.24.3", Line: "LTS", LineName: "10.24", VulnChecked: true, VulnSource: "OSV (osv.dev)", MarketplaceChecked: true,
		Libraries: []DepLibrary{
			{File: "userlib/a.jar", Group: "g", Name: "a", Version: "1", Checked: true, Vulns: []DepVuln{
				{ID: "GHSA-crit", Severity: "CRITICAL", Fixed: []string{"1.2"}}, {ID: "GHSA-high", Severity: "HIGH"}, {ID: "GHSA-mod", Severity: "MODERATE"}}},
			{File: "userlib/b.jar", Group: "g", Name: "b", Version: "2", Checked: true},
		}}
	md := renderDeps(t, d, nil)
	mustContain(t, md, "2 high or critical vulnerability(ies)", "LTS line 10.24", "`g:a:1`", "GHSA-crit (critical)", "none known", "| 1.2 (no fixed version known for GHSA-high, GHSA-mod) |")
	// The version that fixes every advisory: the lowest fix above the
	// installed version per advisory, the highest of those.
	l := DepLibrary{Version: "2.9.8", Vulns: []DepVuln{{ID: "a", Fixed: []string{"2.9.9", "2.10.0"}}, {ID: "b", Fixed: []string{"2.9.10.1", "2.10.5.1", "2.6.7"}}}}
	if got := upgradeTo(l); got != "2.9.10.1" {
		t.Errorf("upgradeTo = %q, want 2.9.10.1", got)
	}
	if got := upgradeTo(DepLibrary{Version: "1.0", Vulns: []DepVuln{{ID: "x"}}}); !strings.Contains(got, "no fixed version known for x") {
		t.Errorf("an advisory with no fix: %q", got)
	}
	md = renderDeps(t, d, &Annotations{Items: map[string]Note{"vuln:GHSA-crit": {Status: "accepted", Note: "the vulnerable class is not on the classpath"}}})
	mustContain(t, md, "1 high or critical vulnerability(ies)", "GHSA-crit accepted: the vulnerable class is not on the classpath")
}

// What was not checked is said, and never shown as clean or up to date.
func TestDependenciesNotCheckedIsNeverClean(t *testing.T) {
	d := DependencyData{MendixVersion: "10.19.1", Line: "monthly", LineName: "10.19",
		VulnNote: "--offline.", MarketplaceNote: "no Marketplace token. Run 'mxcli auth login' to compare with published versions.",
		Libraries: []DepLibrary{{File: "userlib/a.jar", Group: "g", Name: "a", Version: "1", Note: "the advisory database was not asked"}},
		Modules:   []DepModule{{Name: "Administration", Installed: "4.3.2", Status: DepNotChecked}}}
	md := renderDeps(t, d, nil)
	mustContain(t, md, "Java libraries were not checked for vulnerabilities: --offline.", "Marketplace versions were not checked: no Marketplace token",
		"not checked: the advisory database was not asked", "| high/critical vulnerabilities | — |", "monthly release, not on an LTS or MTS line")
	for _, bad := range []string{"none known", "No high or critical vulnerability", DepUpToDate} {
		if strings.Contains(md, bad) {
			t.Errorf("an unchecked report must not say %q:\n%s", bad, md)
		}
	}
}
