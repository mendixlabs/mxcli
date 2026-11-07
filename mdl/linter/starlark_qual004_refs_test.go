// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"database/sql"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	_ "modernc.org/sqlite"
)

// QUAL004 decided "has a caller" from a hand-kept list of reference kinds,
// MICROFLOW_ENTRY_KINDS, and the list drifted behind the catalog every time an
// edge kind was added. A microflow started only from the navigation — a
// profile's home microflow, or a menu item's `call microflow` — has a
// `home_page` / `menu_item` edge that `mxcli refs` lists, and QUAL004 still said
// "is not called from anywhere. Remove if unused" (ako/mxcli#1484, upstream
// mendixlabs/mxcli#1579; four such NAV_ microflows in ako/mxcli#1639's
// Formula1Frontend). PAGE_ENTRY_KINDS had both kinds; the microflow list did not.

// qual004Ref is one refs row pointing at a document.
type qual004Ref struct{ sourceType, sourceName, targetType, targetName, kind string }

// qual004FixtureDB is a catalog with microflows and pages in module M, and the
// given refs rows. Nothing references a document unless a row says so.
func qual004FixtureDB(t *testing.T, refs []qual004Ref) catalog.CatalogDB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	stmts := []string{
		`CREATE TABLE modules (Id TEXT, Name TEXT, Source TEXT)`,
		`INSERT INTO modules VALUES ('m1', 'M', '')`,
		`CREATE TABLE microflows (
			Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
			MicroflowType TEXT, Description TEXT, ReturnType TEXT,
			ParameterCount INTEGER, ActivityCount INTEGER, TotalActivityCount INTEGER DEFAULT 0, Complexity INTEGER,
			Excluded INTEGER DEFAULT 0, ApplyEntityAccess INTEGER DEFAULT 0)`,
		`INSERT INTO microflows (Id, Name, QualifiedName, ModuleName, Folder, MicroflowType, Description, ReturnType, ParameterCount, ActivityCount, TotalActivityCount, Complexity) VALUES
			('f1','NAV_Home','M.NAV_Home','M','','MICROFLOW','','',0,1,1,1),
			('f2','NAV_Orders','M.NAV_Orders','M','','MICROFLOW','','',0,1,1,1),
			('f3','Unused','M.Unused','M','','MICROFLOW','','',0,1,1,1),
			('f4','ACT_Entry','M.ACT_Entry','M','','MICROFLOW','','',0,1,1,1),
			('f5','Target','M.Target','M','','MICROFLOW','','',0,1,1,1),
			('f6','TargetNF','M.TargetNF','M','','NANOFLOW','','',0,1,1,1)`,
		`CREATE TABLE pages (Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
			Title TEXT, URL TEXT, Description TEXT, WidgetCount INTEGER,
			Class TEXT, Style TEXT, DynamicClasses TEXT)`,
		`INSERT INTO pages (Id, Name, QualifiedName, ModuleName, Folder, Title, URL, Description, WidgetCount) VALUES
			('p1','Orders_Overview','M.Orders_Overview','M','','','','',0),
			('p2','Unused_Page','M.Unused_Page','M','','','','',0),
			('p3','TargetPage','M.TargetPage','M','','','','',0)`,
		`CREATE TABLE refs (
			SourceType TEXT, SourceId TEXT, SourceName TEXT, TargetType TEXT,
			TargetId TEXT, TargetName TEXT, RefKind TEXT, ModuleName TEXT)`,
		// One unrelated row, so the refs table reads as populated.
		`INSERT INTO refs VALUES ('PAGE','','M.X','ENTITY','','M.E','parameter','M')`,
		`CREATE TABLE entities (
			Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
			EntityType TEXT, Description TEXT, Generalization TEXT,
			AttributeCount INTEGER, AccessRuleCount INTEGER, ValidationRuleCount INTEGER,
			HasEventHandlers INTEGER, IsExternal INTEGER,
			HasCreatedDate INTEGER, HasChangedDate INTEGER,
			HasOwner INTEGER, HasChangedBy INTEGER)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("fixture %q: %v", s, err)
		}
	}
	for _, r := range refs {
		if _, err := db.Exec(`INSERT INTO refs VALUES (?, '', ?, ?, '', ?, ?, 'M')`,
			r.sourceType, r.sourceName, r.targetType, r.targetName, r.kind); err != nil {
			t.Fatalf("insert ref %+v: %v", r, err)
		}
	}
	return catalog.WrapSqlDB(db)
}

// qual004Flagged runs the shipped QUAL004 rule and returns the qualified names
// it reports.
func qual004Flagged(t *testing.T, db catalog.CatalogDB) []string {
	t.Helper()
	r, err := linter.LoadStarlarkRule(filepath.Join(flowRulesDir, "orphaned_elements.star"))
	if err != nil {
		t.Fatalf("LoadStarlarkRule: %v", err)
	}
	var out []string
	for _, v := range r.Check(linter.NewLintContextFromDB(db)) {
		if v.RuleFailure {
			t.Fatalf("QUAL004 failed to run: %s", v.Message)
		}
		out = append(out, v.Location.QualifiedName())
	}
	sort.Strings(out)
	return out
}

// TestQUAL004CountsNavigationAsACaller is the reported case: a home microflow
// and a menu item's microflow, with the controls the report itself used.
func TestQUAL004CountsNavigationAsACaller(t *testing.T) {
	flagged := qual004Flagged(t, qual004FixtureDB(t, []qual004Ref{
		{"NAVIGATION", "Navigation.Responsive", "MICROFLOW", "M.NAV_Home", catalog.RefKindHomePage},
		{"NAVIGATION", "Navigation.Responsive", "MICROFLOW", "M.NAV_Orders", catalog.RefKindMenuItem},
		{"NAVIGATION", "Navigation.Responsive", "PAGE", "M.Orders_Overview", catalog.RefKindMenuItem},
	}))

	for _, live := range []string{"M.NAV_Home", "M.NAV_Orders", "M.Orders_Overview"} {
		if slices.Contains(flagged, live) {
			t.Errorf("QUAL004 reports %s, which the navigation starts — `mxcli refs` lists the edge, "+
				"and the rule advises deleting a live entry point", live)
		}
	}

	// CONTROLS: an unreferenced microflow and page are still reported — a rule
	// that reported nothing would pass the assertions above — and an
	// entry-point prefix still exempts by name.
	for _, dead := range []string{"M.Unused", "M.Unused_Page"} {
		if !slices.Contains(flagged, dead) {
			t.Errorf("control: %s has no reference and is not reported; got %v", dead, flagged)
		}
	}
	if slices.Contains(flagged, "M.ACT_Entry") {
		t.Errorf("control: M.ACT_Entry carries an entry-point prefix and is reported; got %v", flagged)
	}
}

// TestQUAL004CountsEveryRefKind is the drift guard. The rule does not keep its
// own list of the kinds that mean "something starts this" — every edge into a
// document counts, which is the graph `mxcli refs` and GRAPH_DEAD_ASSETS read —
// so a kind the catalog adds later counts without anyone remembering this rule.
//
// Every kind the catalog declares is driven at a microflow, a nanoflow and a
// page, one at a time. catalog.RefKinds is held complete against the RefKind
// constants by TestEveryObjectTypeConstantIsInAPublishedList, so a new kind is
// in this loop the moment it is declared; re-introducing an allowlist that
// omits it fails here, by name.
func TestQUAL004CountsEveryRefKind(t *testing.T) {
	// CONTROL: a scan of an empty vocabulary would pass whatever the rule did.
	if len(catalog.RefKinds) < 20 {
		t.Fatalf("catalog.RefKinds holds %d kinds — the vocabulary is broken, not the rule", len(catalog.RefKinds))
	}
	for _, kind := range catalog.RefKinds {
		t.Run(kind, func(t *testing.T) {
			flagged := qual004Flagged(t, qual004FixtureDB(t, []qual004Ref{
				{"MICROFLOW", "M.Caller", "MICROFLOW", "M.Target", kind},
				{"MICROFLOW", "M.Caller", "NANOFLOW", "M.TargetNF", kind},
				{"MICROFLOW", "M.Caller", "PAGE", "M.TargetPage", kind},
			}))
			for _, target := range []string{"M.Target", "M.TargetNF", "M.TargetPage"} {
				if slices.Contains(flagged, target) {
					t.Errorf("a %q edge into %s does not count: QUAL004 reports it as not called from "+
						"anywhere while `mxcli refs` lists the reference", kind, target)
				}
			}
			// CONTROL, per kind: the edge must not silence the unrelated document.
			if !slices.Contains(flagged, "M.Unused") {
				t.Errorf("control: M.Unused stopped being reported; got %v", flagged)
			}
		})
	}
}
