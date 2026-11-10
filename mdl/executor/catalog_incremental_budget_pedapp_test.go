// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// ako/mxcli#1804: an incremental catalog build is only fast while every pass
// keeps to its rules, and breaking one is silent — the build stays correct
// and gets slower. These tests make the two ways it gets slower fail: rows
// no document owns (rebuilt on every refresh) and documents decoded outside
// the scope (every refresh decodes the project again).

// projectWideTables are the tables whose rows may belong to no document,
// and why. Such rows are dropped and written again on every refresh, so a
// table belongs here only when its rows cannot come from one document.
var projectWideTables = map[string]string{
	"projects":                          "the build's own record",
	"snapshots":                         "the build's own record",
	"modules_data":                      "a module is no per-document pass's unit; a module change rebuilds everything",
	"folders_data":                      "a folder's counts change when a document moves in or out, which rebuilds only that document",
	"navigation_profiles_data":          "the navigation document; a change to it rebuilds everything",
	"navigation_menu_items":             "the navigation document",
	"navigation_role_homes":             "the navigation document",
	"constant_values":                   "the project settings; a change to them rebuilds everything",
	"user_roles":                        "the project security; a change to it rebuilds everything",
	"role_mappings":                     "the project security",
	"widget_definitions_data":           "widgets/*.mpk and the built-in definitions, not the model",
	"widget_definition_properties_data": "widgets/*.mpk and the built-in definitions, not the model",
	"external_actions_data":             "aggregated over every flow that calls an action",
	"refs":                              "projections over other tables (widgets, XPath, enumeration types) and the navigation and settings edges",
	"strings":                           "the documentation projection over the object tables",
	"communities_data":                  "the graph analysis, over all refs",
	"graph_cycles_data":                 "the graph analysis",
	"graph_module_cycles_data":          "the graph analysis",
	"graph_layers_data":                 "the graph analysis",
	"graph_centrality_data":             "the graph analysis",
}

func TestCatalog_RowsBelongToADocument(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	ctx := exec.newExecContext(context.Background())
	ctx.Output = out
	ctx.catalogForce = true
	if err := buildCatalog(ctx, true, true, false, 0); err != nil {
		t.Fatalf("build: %v\n%s", err, out.String())
	}
	defer func() { ctx.Catalog.Close(); ctx.Catalog = nil }()
	res, err := ctx.Catalog.Query(`SELECT TableName, SUM(LastRow - FirstRow + 1) FROM catalog_row_owners
		WHERE UnitId = '' GROUP BY TableName`)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := ctx.Catalog.Query(`SELECT COUNT(*) FROM catalog_row_owners WHERE UnitId != ''`)
	if err != nil || len(owned.Rows) == 0 || fmt.Sprint(owned.Rows[0][0]) == "0" {
		t.Fatalf("control: the build recorded no document's rows (%v)", err)
	}
	var stray []string
	for _, r := range res.Rows {
		table := fmt.Sprint(r[0])
		if _, ok := projectWideTables[table]; !ok {
			stray = append(stray, fmt.Sprintf("%s (%v rows)", table, r[1]))
		}
	}
	sort.Strings(stray)
	if len(stray) > 0 {
		t.Errorf("rows that belong to no document, so every refresh writes them again:\n  %s\n"+
			"Call b.own(unitID) at the head of the pass's per-document loop (builder_incremental.go), "+
			"or, if the rows really cannot come from one document, add the table to projectWideTables with the reason.",
			strings.Join(stray, "\n  "))
	}
}

func TestCatalog_IncrementalRefreshDecodesOnlyWhatItRebuilds(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if _, stats := catalogRows(t, exec, true, true); stats.Incremental {
		t.Fatal("control: the forced build was incremental")
	}
	if err := agreeExec(t, exec, strings.Replace(redeclareChain(2, "create"), "length($Title) + $Prev", "length($Title) * 3 + $Prev", 1)); err != nil {
		t.Fatalf("edit: %v\n%s", err, out.String())
	}
	before := mprread.UnitsDecoded()
	var stats catalog.BuildStats
	catalogBuildHook = func(s catalog.BuildStats) { stats = s }
	defer func() { catalogBuildHook = nil }()
	ctx := exec.newExecContext(context.Background())
	ctx.Output = out
	if err := buildCatalog(ctx, true, false, false, 0); err != nil {
		t.Fatalf("build: %v\n%s", err, out.String())
	}
	ctx.Catalog.Close()
	ctx.Catalog = nil
	decoded := mprread.UnitsDecoded() - before
	if !stats.Incremental || stats.Rebuilt == 0 {
		t.Fatalf("control: the refresh after an edit was not incremental (%+v)", stats)
	}
	// The project-wide passes read the project settings, the project
	// security and the navigation, a few times between them: a constant,
	// not a share of the project.
	const singletons = 6
	if budget := int64(stats.Rebuilt + singletons); decoded > budget {
		t.Errorf("an incremental refresh that rebuilt %d units decoded %d, want at most %d.\n"+
			"A pass lists documents through the reader instead of the build's scoped list: use b.lister() "+
			"or the cached accessors (builder.go), and look other documents up in the catalog tables.",
			stats.Rebuilt, decoded, budget)
	}
	t.Logf("decoded %d units, rebuilt %d", decoded, stats.Rebuilt)
}
