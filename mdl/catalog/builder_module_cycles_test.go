// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type probeRef struct{ src, tgt, kind string }

// moduleCycleCatalog builds a catalog holding nothing but a snapshot and the
// given refs, then runs the graph pass over it.
func moduleCycleCatalog(t *testing.T, refs []probeRef) *Catalog {
	t.Helper()
	cat, err := NewFromFile(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	db := cat.CatalogDB()
	if _, err := db.Exec(`INSERT INTO snapshots (SnapshotId, ProjectId) VALUES ('s1','default')`); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}
	for _, r := range refs {
		if _, err := db.Exec(`INSERT INTO refs
			(SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
			VALUES ('MICROFLOW','',?,'MICROFLOW','',?,?,?,'default','s1')`,
			r.src, r.tgt, r.kind, moduleOf(r.src)); err != nil {
			t.Fatalf("insert ref: %v", err)
		}
	}
	if err := cat.AddGraphAnalysis(1.0); err != nil {
		t.Fatalf("AddGraphAnalysis: %v", err)
	}
	return cat
}

func moduleCycleRows(t *testing.T, cat *Catalog) map[string]string {
	t.Helper()
	rows, err := cat.CatalogDB().Query(`SELECT ModuleName, CycleSize, RefKinds FROM graph_module_cycles_data`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, kinds string
		var size int
		if err := rows.Scan(&name, &size, &kinds); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[name] = kinds
	}
	return out
}

func assetCycleCount(t *testing.T, cat *Catalog) int {
	t.Helper()
	var n int
	if err := cat.CatalogDB().QueryRow(`SELECT COUNT(*) FROM graph_cycles_data`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// TestModuleCycleSurvivesWhereAssetCycleCannotExist is mendixlabs/mxcli#1060
// reduced to its skeleton, and the reason graph_module_cycles is not a rollup of
// graph_cycles.
//
// Administration references Atlas_Core from one document and Atlas_Core
// references Administration from a DIFFERENT one. The modules depend on each
// other; no document does. graph_cycles is therefore correctly empty — and was
// read as "no circular dependencies" while graph_module_coupling listed the pair
// in both directions.
func TestModuleCycleSurvivesWhereAssetCycleCannotExist(t *testing.T) {
	cat := moduleCycleCatalog(t, []probeRef{
		{"Administration.A", "Atlas_Core.B", "call"},
		{"Atlas_Core.C", "Administration.D", "call"},
	})

	if n := assetCycleCount(t, cat); n != 0 {
		t.Fatalf("graph_cycles has %d rows; the fixture has no document-level cycle, so this test would not be about anything", n)
	}
	got := moduleCycleRows(t, cat)
	want := []string{"Administration", "Atlas_Core"}
	var names []string
	for m := range got {
		names = append(names, m)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("graph_module_cycles = %v, want %v", names, want)
	}
}

// TestModuleCycleSeesKindsTheAssetGraphExcludes is the other half of the report.
// graph_module_coupling counts every reference kind; the computed pass is
// restricted to graphRefKinds. On ako/TestApp that is 110 of 316 edges, and
// Administration -> Atlas_Core there is `layout`-only — one of the excluded
// ones. A module-cycle table built on the structural subset would still have
// answered "none" for the pair that was reported.
func TestModuleCycleSeesKindsTheAssetGraphExcludes(t *testing.T) {
	cat := moduleCycleCatalog(t, []probeRef{
		{"Administration.APage", "Atlas_Core.SomeLayout", "layout"},
		{"Atlas_Core.CPage", "Administration.DLayout", "layout"},
	})

	if n := assetCycleCount(t, cat); n != 0 {
		t.Fatalf("graph_cycles has %d rows; `layout` is not in graphRefKinds, so it must not reach the asset graph", n)
	}
	got := moduleCycleRows(t, cat)
	if len(got) != 2 {
		t.Fatalf("graph_module_cycles = %v, want both modules", got)
	}
	if got["Administration"] != "layout" {
		t.Errorf("RefKinds for Administration = %q, want \"layout\" — the row should name what keeps the cycle alive", got["Administration"])
	}
}

// TestOneWayModuleDependencyIsNotACycle is the control that keeps the table
// meaningful: cross-module edges alone are not circularity, or every app with a
// dependency would be reported as tangled.
func TestOneWayModuleDependencyIsNotACycle(t *testing.T) {
	cat := moduleCycleCatalog(t, []probeRef{
		{"Administration.A", "Atlas_Core.B", "call"},
		{"Administration.E", "Atlas_Core.F", "layout"},
	})
	if got := moduleCycleRows(t, cat); len(got) != 0 {
		t.Errorf("graph_module_cycles = %v, want empty for a one-way dependency", got)
	}
}

// TestModuleCycleThroughAThirdModule: A -> B -> C -> A is a cycle even though no
// two of them reference each other directly. Tarjan gets this for free; the test
// exists because the tempting cheap implementation — self-joining
// graph_module_coupling on the reversed pair, which is how the issue reporter
// spotted the problem — finds only the two-module case.
func TestModuleCycleThroughAThirdModule(t *testing.T) {
	cat := moduleCycleCatalog(t, []probeRef{
		{"A.One", "B.Two", "call"},
		{"B.Three", "C.Four", "call"},
		{"C.Five", "A.Six", "call"},
		// A fourth module that only depends on the tangle is NOT part of it.
		{"D.Seven", "A.Eight", "call"},
	})
	got := moduleCycleRows(t, cat)
	var names []string
	for m := range got {
		names = append(names, m)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "A,B,C" {
		t.Errorf("graph_module_cycles = %v, want [A B C] — D depends on the cycle but is not in it", names)
	}
}

// TestRefKindsNamesOnlyTheEdgesInsideTheCycle: a module's references to the
// world outside the tangle are not what makes it circular, so listing them in
// RefKinds would send a reader after the wrong edge.
func TestRefKindsNamesOnlyTheEdgesInsideTheCycle(t *testing.T) {
	cat := moduleCycleCatalog(t, []probeRef{
		{"A.One", "B.Two", "call"},
		{"B.Three", "A.Four", "call"},
		// A -> Outside, a kind that appears nowhere in the cycle.
		{"A.Five", "Outside.Six", "associate"},
	})
	got := moduleCycleRows(t, cat)
	if got["A"] != "call" {
		t.Errorf("RefKinds for A = %q, want \"call\" — `associate` leaves the cycle", got["A"])
	}
}

// TestGraphAnalysisScopeRecordsWhatTheFilterDrops makes the disagreement between
// the two graph families answerable from SQL. Before this there was no way to
// find out that a third of the edges graph_module_coupling counts never reach
// the cycles/communities/layers computation, short of reading Go source.
func TestGraphAnalysisScopeRecordsWhatTheFilterDrops(t *testing.T) {
	cat := moduleCycleCatalog(t, []probeRef{
		{"A.One", "B.Two", "call"},      // structural
		{"B.Three", "A.Four", "layout"}, // navigational
		{"A.Five", "B.Six", "layout"},
	})
	rows, err := cat.CatalogDB().Query(`SELECT RefKind, Edges, InAssetGraph FROM graph_analysis_scope ORDER BY RefKind`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	got := map[string][2]int{}
	for rows.Next() {
		var kind string
		var edges, in int
		if err := rows.Scan(&kind, &edges, &in); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[kind] = [2]int{edges, in}
	}
	if got["call"] != [2]int{1, 1} {
		t.Errorf("call = %v, want 1 edge inside the asset graph", got["call"])
	}
	if got["layout"] != [2]int{2, 0} {
		t.Errorf("layout = %v, want 2 edges outside the asset graph", got["layout"])
	}
}

// TestGraphAnalysisScopeTracksGraphRefKinds proves the view is generated from
// the filter rather than restating it: every kind the loader admits must report
// InAssetGraph = 1. A kind added to graphRefKinds without the view noticing
// would leave the documentation of the scope lying about the scope.
func TestGraphAnalysisScopeTracksGraphRefKinds(t *testing.T) {
	var refs []probeRef
	for i, kind := range graphRefKinds {
		refs = append(refs, probeRef{
			src:  "A.S" + strings.Repeat("x", i+1),
			tgt:  "B.T" + strings.Repeat("x", i+1),
			kind: kind,
		})
	}
	cat := moduleCycleCatalog(t, refs)
	rows, err := cat.CatalogDB().Query(`SELECT RefKind FROM graph_analysis_scope WHERE InAssetGraph = 0`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var excluded []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatalf("scan: %v", err)
		}
		excluded = append(excluded, k)
	}
	if len(excluded) != 0 {
		t.Errorf("kinds in graphRefKinds reported as outside the asset graph: %v", excluded)
	}
}

// A queue in module A running flows of B is not A using B: the caller in B
// uses A's queue (`enqueue`). Counting the `queue` edge closed a false cycle
// across every module whose flow the queue runs — 9 modules on one app — and a
// constant in A naming a flow in B is configuration, not use (ako/mxcli#2503).
func TestQueueAndByNameCloseNoModuleCycle(t *testing.T) {
	cat := moduleCycleCatalog(t, []probeRef{
		{"Ruilen.WFA_Goedkeuren", "Rooster.TQ_RoosterMutaties", "enqueue"},
		{"Rooster.TQ_RoosterMutaties", "Ruilen.SUB_Ruiling_Doorvoeren", "queue"},
		{"Verlof.SUB_Plan", "OIDC.Hook", "call"},
		{"OIDC.ProvisioningMicroflow", "Verlof.SUB_Provision", "by_name"},
	})
	if got := moduleCycleRows(t, cat); len(got) != 0 {
		t.Errorf("graph_module_cycles = %v, want empty: queue and by_name edges are not module use", got)
	}
	// Control: a real use the other way closes the cycle.
	cat = moduleCycleCatalog(t, []probeRef{
		{"Ruilen.WFA_Goedkeuren", "Rooster.TQ_RoosterMutaties", "enqueue"},
		{"Rooster.SUB_Rooster", "Ruilen.SUB_Ruiling_Doorvoeren", "call"},
	})
	if got := moduleCycleRows(t, cat); len(got) != 2 {
		t.Errorf("control: a call back from Rooster closes the cycle, got %v", got)
	}
}
