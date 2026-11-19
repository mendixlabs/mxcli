// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// archFixture: Sales and Billing depend on each other through domain edges (a
// real cycle); the Shell module's menu links Sales' page and Sales' page uses
// Shell's layout (a navigation-only cycle); Sales also uses a Marketplace
// module.
func archFixture(t *testing.T) *fixture {
	f := newFixture(t)
	for _, m := range []string{"Shell", "Sales", "Billing"} {
		f.module(m, "")
	}
	f.module("Mkt", "Marketplace v1.0.0")
	f.module("System", "")
	f.microflow("Sales.ACT_Order", false, "")
	f.microflow("Sales.SUB_Total", false, "")
	f.microflow("Billing.ACT_Invoice", false, "")
	f.microflow("Billing.OnStartup", false, "")
	f.microflow("Billing.Unused", false, "")
	f.microflow("Billing.CalledByName", false, "")
	f.microflow("Mkt.Helper", false, "")
	f.page("Sales.Order_Overview")
	f.page("Shell.Home")

	f.call("Sales.ACT_Order", "Sales.SUB_Total")
	f.call("Sales.ACT_Order", "Billing.ACT_Invoice")
	f.call("Sales.SUB_Total", "Billing.ACT_Invoice")
	f.call("Billing.ACT_Invoice", "Sales.SUB_Total") // closes the cycle with one ref
	f.call("Sales.ACT_Order", "Mkt.Helper")
	f.ref("Shell.Home", "PAGE", "Sales.Order_Overview", "PAGE", "show_page")
	f.ref("Sales.Order_Overview", "PAGE", "Shell.Atlas_Default", "LAYOUT", "layout")
	f.ref("Navigation.Responsive", "NAVIGATION", "Shell.Home", "PAGE", "home_page")
	f.constant("Shell.StartupFlow", "String", "Billing.CalledByName", false)
	f.exec(`UPDATE modules_data SET DomainModelDocumentation = 'Invoices and dunning. Owns payment terms.' WHERE Name = 'Billing'`)
	return f
}

func TestArchitectureSeparatesNavigationFromDomain(t *testing.T) {
	f := archFixture(t)
	d, err := CollectArchitecture(f.cat, ArchitectureOptions{SettingsMicroflows: []string{"Billing.OnStartup"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Scope.App, []string{"Billing", "Sales", "Shell"}) {
		t.Errorf("app modules = %v (marketplace and System must be outside)", d.Scope.App)
	}
	// Only the domain cycle. With navigation counted Shell joins it — the
	// trap that put a whole app in one cycle.
	if len(d.Cycles) != 1 || !reflect.DeepEqual(d.Cycles[0].Modules, []string{"Billing", "Sales"}) {
		t.Fatalf("cycles = %+v, want one Billing↔Sales", d.Cycles)
	}
	if d.ModulesInCyclesWithNavigation != 3 {
		t.Errorf("modules in cycles with navigation = %d, want 3", d.ModulesInCyclesWithNavigation)
	}
	if !reflect.DeepEqual(d.Cycles[0].Lightest, []string{"Billing → Sales"}) {
		t.Errorf("lightest = %v", d.Cycles[0].Lightest)
	}
	for _, m := range d.Modules {
		want := map[string]string{"Billing": "Invoices and dunning."}[m.Name]
		if m.Purpose != want {
			t.Errorf("%s purpose = %q, want %q (the first sentence of its documentation)", m.Name, m.Purpose, want)
		}
	}
	if len(d.NavigationEdges) != 2 {
		t.Errorf("navigation edges = %+v", d.NavigationEdges)
	}
	for _, e := range append(d.NavigationEdges, d.OutsideEdges...) {
		if e.From == "Navigation" || e.To == "Navigation" {
			t.Errorf("a navigation document's prefix is not a module: %+v", e)
		}
	}
	if len(d.OutsideEdges) != 1 || d.OutsideEdges[0].To != "Mkt" {
		t.Errorf("outside edges = %+v", d.OutsideEdges)
	}
	verdict := map[string]string{}
	for _, u := range d.Unused {
		verdict[u.Name] = u.Verdict
	}
	want := map[string]string{"Billing.OnStartup": "used", "Billing.CalledByName": "verify", "Billing.Unused": "unused"}
	for k, v := range want {
		if verdict[k] != v {
			t.Errorf("unused %s = %q, want %q (all: %v)", k, verdict[k], v, verdict)
		}
	}
}

func TestArchitectureRenderUsesAnnotations(t *testing.T) {
	f := archFixture(t)
	d, err := CollectArchitecture(f.cat, ArchitectureOptions{})
	if err != nil {
		t.Fatal(err)
	}
	env, err := NewEnvelope("architecture", "App", "now", "test", d, ArchitectureMethod, ArchitectureLimits, nil)
	if err != nil {
		t.Fatal(err)
	}
	ann := &Annotations{Items: map[string]Note{
		"cycle:Billing+Sales":  {Status: "accidental", Note: "move SUB_Total to Billing"},
		"module:Shell":         {Note: "app shell"},
		"cluster:Gone.Element": {Name: "stale"},
	}}
	doc, err := RenderArchitecture(env, ann)
	if err != nil {
		t.Fatal(err)
	}
	var md, html bytes.Buffer
	if err := RenderMarkdown(&md, doc); err != nil {
		t.Fatal(err)
	}
	if err := RenderHTML(&html, doc); err != nil {
		t.Fatal(err)
	}
	mustContain(t, md.String(), "accidental: move SUB_Total to Billing", "app shell",
		"## How this was measured", "## What this does not cover", "`cluster:Gone.Element`")
	mustContain(t, html.String(), "<svg", "Layered module map")
	if !strings.Contains(html.String(), "<!doctype html>") {
		t.Error("not a standalone page")
	}
}

func TestDigraphLayersAndSCCs(t *testing.T) {
	g := newDigraph([]string{"A", "B", "C", "D"})
	g.add("A", "B", 1)
	g.add("B", "C", 1)
	g.add("C", "B", 1)
	g.add("C", "D", 1)
	l := g.layers()
	if l["D"] != 0 || l["B"] != 1 || l["C"] != 1 || l["A"] != 2 {
		t.Errorf("layers = %v", l)
	}
	var cyc [][]string
	for _, c := range g.sccs() {
		if len(c) > 1 {
			cyc = append(cyc, c)
		}
	}
	if !reflect.DeepEqual(cyc, [][]string{{"B", "C"}}) {
		t.Errorf("sccs = %v", cyc)
	}
}

// ako/mxcli#2299: a page placing another module's snippet is screen
// composition, not domain use. Counted as domain it closed a cycle that does
// not exist: here Orders uses Billing, and Billing's page shows an Orders
// snippet.
func TestSnippetCallIsNotADomainDependency(t *testing.T) {
	f := newFixture(t)
	f.module("Orders", "")
	f.module("Billing", "")
	f.microflow("Orders.ACT_Order", false, "")
	f.microflow("Billing.ACT_Invoice", false, "")
	f.page("Billing.Invoice_Overview")
	f.call("Orders.ACT_Order", "Billing.ACT_Invoice")
	f.ref("Billing.Invoice_Overview", "PAGE", "Orders.SNIP_Banner", "SNIPPET", "snippet_call")
	d, err := CollectArchitecture(f.cat, ArchitectureOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Cycles) != 0 {
		t.Errorf("snippet placement made a domain cycle: %+v", d.Cycles)
	}
	if len(d.NavigationEdges) != 1 || d.NavigationEdges[0].Kinds[0] != "snippet_call" {
		t.Errorf("navigation edges = %+v", d.NavigationEdges)
	}
}

// Cohesion is drawn as a meter, and each cluster as a bar split by module
// (ako/mxcli#2443).
func TestArchitectureCohesionBarsAndClusterComposition(t *testing.T) {
	d := ArchitectureData{
		Modules:             []ModuleHealth{{Name: "Sales", Cohesion: 72}, {Name: "Billing", Cohesion: 20}, {Name: "Empty", Cohesion: -1}},
		CommunitiesComputed: true,
		Communities: []Community{
			{Key: "cluster:Sales.Order", Label: "Order", Size: 10, Modules: []ModuleCount{{"Sales", 7}, {"Billing", 3}}},
			{Key: "cluster:Billing.Invoice", Label: "Invoice", Size: 4, Modules: []ModuleCount{{"Billing", 4}}},
		},
	}
	env, err := NewEnvelope("architecture", "App", "now", "test", d, ArchitectureMethod, ArchitectureLimits, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := RenderArchitecture(env, nil)
	if err != nil {
		t.Fatal(err)
	}
	var html bytes.Buffer
	if err := RenderHTML(&html, doc); err != nil {
		t.Fatal(err)
	}
	out := html.String()
	mustContain(t, out, "What each cluster is made of", `class="meter good"`, `class="meter warn"`)
	if strings.Count(out, `class="meter`) != 2 {
		t.Errorf("want a meter for the two modules with a cohesion, not for n/a: %d", strings.Count(out, `class="meter`))
	}
}

// --exclude-docs leaves matching documents out of the edges, the counts and
// the unused list, and the page says so (ako/mxcli#2443).
func TestArchitectureExcludeDocs(t *testing.T) {
	f := archFixture(t)
	base, err := CollectArchitecture(f.cat, ArchitectureOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(base.Cycles) != 1 {
		t.Fatalf("control: the fixture has one cycle, got %d", len(base.Cycles))
	}
	d, err := CollectArchitecture(f.cat, ArchitectureOptions{ExcludeDocs: []string{"Billing.ACT_*", "Billing.Unused"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Cycles) != 0 {
		t.Errorf("every edge of the cycle goes through Billing.ACT_Invoice, so excluding it removes the cycle: %+v", d.Cycles)
	}
	for _, u := range d.Unused {
		if u.Name == "Billing.Unused" {
			t.Error("an excluded document is not listed as unused")
		}
	}
	if d.ExcludedDocs != 2 {
		t.Errorf("excluded %d documents, want 2", d.ExcludedDocs)
	}
	docs := func(d *ArchitectureData) int {
		for _, m := range d.Modules {
			if m.Name == "Billing" {
				return m.Documents
			}
		}
		return -1
	}
	if docs(base)-docs(d) != 2 {
		t.Errorf("Billing documents %d → %d, want two fewer", docs(base), docs(d))
	}
	env, _ := NewEnvelope("architecture", "App", "now", "test", d, ArchitectureMethod, ArchitectureLimits, nil)
	doc, err := RenderArchitecture(env, nil)
	if err != nil {
		t.Fatal(err)
	}
	var md bytes.Buffer
	_ = RenderMarkdown(&md, doc)
	mustContain(t, md.String(), "2 app documents matching `Billing.ACT_*`, `Billing.Unused` are left out")
}
