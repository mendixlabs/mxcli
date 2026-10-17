// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// moduleSeed is one row of the modules table: which snapshot it belongs to, the
// module, and its Source (empty unless the module came from the Marketplace).
type moduleSeed struct{ snapshot, name, source string }

// seedSearchCatalog writes snapshots (oldest first, so the last one is the
// current snapshot), the given module rows and a documentation hit in each of
// Sales, Vendor and the module rows themselves, as the catalog builder writes
// them: a module's own documentation carries the module's name in ModuleName.
func seedSearchCatalog(t *testing.T, snapshots []string, modules []moduleSeed) func(catalog.CatalogDB) {
	return func(db catalog.CatalogDB) {
		t.Helper()
		var stmts []string
		for _, s := range snapshots {
			stmts = append(stmts, `INSERT INTO snapshots (SnapshotId, ProjectId) VALUES ('`+s+`', 'p')`)
		}
		for i, m := range modules {
			stmts = append(stmts, `INSERT INTO modules_data (Id, Name, QualifiedName, Source, SnapshotId, ProjectId)
				VALUES ('m`+string(rune('a'+i))+`', '`+m.name+`', '`+m.name+`', '`+m.source+`', '`+m.snapshot+`', 'p')`)
		}
		stmts = append(stmts,
			`INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, ModuleName)
				VALUES ('Sales.Limit', 'CONSTANT', 'Upper bound for the quuxwidget', 'documentation', 'Sales')`,
			`INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, ModuleName)
				VALUES ('Vendor.Limit', 'CONSTANT', 'Upper bound for the quuxwidget', 'documentation', 'Vendor')`,
			// A module's own documentation: the objects view holds '' as the
			// module's ModuleName, and the builder stores the module's Name instead.
			`INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, ModuleName)
				VALUES ('Vendor', 'MODULE', 'Vendor notes on the quuxwidget', 'documentation', 'Vendor')`,
			`INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, ModuleName)
				VALUES ('Vendor', 'MODULE', 'Vendor domain notes on the quuxwidget', 'domain_model_documentation', 'Vendor')`,
		)
		for _, q := range stmts {
			if _, err := db.Exec(q); err != nil {
				t.Fatalf("seed: %v\n%s", err, q)
			}
		}
	}
}

// searchHits runs a JSON search and returns "Qualified.Name/context" per hit.
func searchHits(t *testing.T, seed func(catalog.CatalogDB), opts SearchOptions) []string {
	t.Helper()
	// One fixture per search: a second search on the same context made the
	// TempDir cleanup fail on Windows (catalog.db still in use).
	ctx, out, _ := searchCacheFixtureSeeded(t, "source", nil, seed)
	if _, err := searchWith(ctx, "quuxwidget", "json", opts); err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		QualifiedName string `json:"qualifiedName"`
		StringContext string `json:"stringContext"`
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	var hits []string
	for _, r := range rows {
		hits = append(hits, r.QualifiedName+"/"+r.StringContext)
	}
	return hits
}

func sameSet(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
	seen := map[string]bool{}
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Fatalf("hits = %v, want %v", got, want)
		}
	}
}

var sales = moduleSeed{"s1", "Sales", ""}
var vendor = moduleSeed{"s1", "Vendor", "Marketplace v1.0.0"}

// The default keeps every module, as it always has; the option drops a
// Marketplace module's hits and nothing else - the module's own documentation
// and domain model documentation included, which a filter on ModuleName missed
// while the builder stored an empty one for a module.
func TestSearchWith_ExcludeMarketplace(t *testing.T) {
	seed := seedSearchCatalog(t, []string{"s1"}, []moduleSeed{sales, vendor})

	sameSet(t, searchHits(t, seed, SearchOptions{}), []string{
		"Sales.Limit/documentation", "Vendor.Limit/documentation",
		"Vendor/documentation", "Vendor/domain_model_documentation",
	})
	sameSet(t, searchHits(t, seed, SearchOptions{ExcludeMarketplace: true}), []string{
		"Sales.Limit/documentation",
	})
}

// The same option reaches the table and names formats, not only JSON.
func TestSearchWith_ExcludeMarketplace_AllFormats(t *testing.T) {
	seed := seedSearchCatalog(t, []string{"s1"}, []moduleSeed{sales, vendor})
	for _, format := range []string{"table", "names"} {
		t.Run(format, func(t *testing.T) {
			ctx, out, _ := searchCacheFixtureSeeded(t, "source", nil, seed)
			if _, err := searchWith(ctx, "quuxwidget", format, SearchOptions{ExcludeMarketplace: true}); err != nil {
				t.Fatal(err)
			}
			got := out.String()
			if !strings.Contains(got, "Sales.Limit") || strings.Contains(got, "Vendor") {
				t.Errorf("want Sales.Limit only, got:\n%s", got)
			}
		})
	}
}

// The catalog can hold more than one snapshot. Only the current one (the latest
// recorded) says which modules are Marketplace modules: a module that was one in
// an older snapshot is the project's own now, and the other way round.
func TestSearchWith_ExcludeMarketplace_UsesTheCurrentSnapshot(t *testing.T) {
	// Vendor came from the Marketplace in s1 and is the project's own in s2.
	seed := seedSearchCatalog(t, []string{"s1", "s2"}, []moduleSeed{
		sales, vendor,
		{"s2", "Sales", ""}, {"s2", "Vendor", ""},
	})
	sameSet(t, searchHits(t, seed, SearchOptions{ExcludeMarketplace: true}), []string{
		"Sales.Limit/documentation", "Vendor.Limit/documentation",
		"Vendor/documentation", "Vendor/domain_model_documentation",
	})

	// Vendor is a Marketplace module in the current snapshot only.
	seed = seedSearchCatalog(t, []string{"s1", "s2"}, []moduleSeed{
		sales, {"s1", "Vendor", ""},
		{"s2", "Sales", ""}, {"s2", "Vendor", "Marketplace v2.0.0"},
	})
	sameSet(t, searchHits(t, seed, SearchOptions{ExcludeMarketplace: true}), []string{
		"Sales.Limit/documentation",
	})
}
