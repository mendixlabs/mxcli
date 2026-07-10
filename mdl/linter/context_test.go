// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"database/sql"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"

	_ "modernc.org/sqlite"
)

// setupModuleFilterDB creates an in-memory SQLite database seeded with entities
// spread across three modules: ModA, ModB, ModC.
func setupModuleFilterDB(t *testing.T) catalog.CatalogDB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE modules (Name TEXT PRIMARY KEY, Source TEXT)`)
	if err != nil {
		t.Fatalf("create modules table: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE entities (
		Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
		EntityType TEXT, Description TEXT, Generalization TEXT,
		AttributeCount INTEGER, AccessRuleCount INTEGER, ValidationRuleCount INTEGER,
		HasEventHandlers INTEGER, IsExternal INTEGER
	)`)
	if err != nil {
		t.Fatalf("create entities table: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE microflows (
		Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
		MicroflowType TEXT, Description TEXT, ReturnType TEXT,
		ParameterCount INTEGER, ActivityCount INTEGER, Complexity INTEGER
	)`)
	if err != nil {
		t.Fatalf("create microflows table: %v", err)
	}

	modules := []string{"ModA", "ModB", "ModC"}
	for _, mod := range modules {
		if _, err := db.Exec(`INSERT INTO modules VALUES (?, '')`, mod); err != nil {
			t.Fatalf("insert module %s: %v", mod, err)
		}
		if _, err := db.Exec(`INSERT INTO entities VALUES (?, ?, ?, ?, '', 'PERSISTENT', '', '', 0, 0, 0, 0, 0)`,
			mod+"_e", mod+"_Entity", mod+".Entity", mod); err != nil {
			t.Fatalf("insert entity for %s: %v", mod, err)
		}
		if _, err := db.Exec(`INSERT INTO microflows VALUES (?, ?, ?, ?, '', 'Microflow', '', '', 0, 0, 0)`,
			mod+"_mf", mod+"_Flow", mod+".Flow", mod); err != nil {
			t.Fatalf("insert microflow for %s: %v", mod, err)
		}
	}

	return catalog.WrapSqlDB(db)
}

// collectEntityModules iterates Entities() and returns the set of module names seen.
func collectEntityModules(ctx *LintContext) map[string]bool {
	seen := map[string]bool{}
	for e := range ctx.Entities() {
		seen[e.ModuleName] = true
	}
	return seen
}

// collectMicroflowModules iterates Microflows() and returns the set of module names seen.
func collectMicroflowModules(ctx *LintContext) map[string]bool {
	seen := map[string]bool{}
	for mf := range ctx.Microflows() {
		seen[mf.ModuleName] = true
	}
	return seen
}

// --- IsExcluded unit tests ---

func TestIsExcluded_NoFilters(t *testing.T) {
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))
	for _, mod := range []string{"ModA", "ModB", "ModC"} {
		if ctx.IsExcluded(mod) {
			t.Errorf("IsExcluded(%q) = true with no filters, want false", mod)
		}
	}
}

func TestIsExcluded_ExcludeOnly(t *testing.T) {
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))
	ctx.SetExcludedModules([]string{"ModB"})

	if ctx.IsExcluded("ModA") {
		t.Error("ModA should not be excluded")
	}
	if !ctx.IsExcluded("ModB") {
		t.Error("ModB should be excluded")
	}
	if ctx.IsExcluded("ModC") {
		t.Error("ModC should not be excluded")
	}
}

func TestIsExcluded_IncludeOnly(t *testing.T) {
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))
	ctx.SetIncludedModules([]string{"ModA"})

	if ctx.IsExcluded("ModA") {
		t.Error("ModA is in the inclusion list and should not be excluded")
	}
	if !ctx.IsExcluded("ModB") {
		t.Error("ModB is not in the inclusion list and should be excluded")
	}
	if !ctx.IsExcluded("ModC") {
		t.Error("ModC is not in the inclusion list and should be excluded")
	}
}

func TestIsExcluded_IncludeAndExcludeCombined(t *testing.T) {
	// Include ModA and ModB, but explicitly exclude ModB — ModB should be excluded.
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))
	ctx.SetIncludedModules([]string{"ModA", "ModB"})
	ctx.SetExcludedModules([]string{"ModB"})

	if ctx.IsExcluded("ModA") {
		t.Error("ModA should pass: it is included and not excluded")
	}
	if !ctx.IsExcluded("ModB") {
		t.Error("ModB should be excluded: explicit exclude wins over include")
	}
	if !ctx.IsExcluded("ModC") {
		t.Error("ModC should be excluded: not in inclusion list")
	}
}

func TestIsExcluded_EmptyInclusionListNoOp(t *testing.T) {
	// SetIncludedModules with an empty slice should behave as if no filter was set.
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))
	ctx.SetIncludedModules([]string{})

	for _, mod := range []string{"ModA", "ModB", "ModC"} {
		if ctx.IsExcluded(mod) {
			t.Errorf("IsExcluded(%q) = true after empty SetIncludedModules, want false", mod)
		}
	}
}

func TestIsExcluded_MultipleIncludedModules(t *testing.T) {
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))
	ctx.SetIncludedModules([]string{"ModA", "ModC"})

	if ctx.IsExcluded("ModA") {
		t.Error("ModA should not be excluded")
	}
	if !ctx.IsExcluded("ModB") {
		t.Error("ModB should be excluded: not in inclusion list")
	}
	if ctx.IsExcluded("ModC") {
		t.Error("ModC should not be excluded")
	}
}

// --- Iterator integration tests ---

func TestEntitiesIterator_InclusionFilter(t *testing.T) {
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))
	ctx.SetIncludedModules([]string{"ModA"})

	seen := collectEntityModules(ctx)
	if !seen["ModA"] {
		t.Error("expected ModA entities to be yielded")
	}
	if seen["ModB"] || seen["ModC"] {
		t.Error("expected ModB and ModC entities to be filtered out")
	}
}

func TestEntitiesIterator_ExclusionFilter(t *testing.T) {
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))
	ctx.SetExcludedModules([]string{"ModB"})

	seen := collectEntityModules(ctx)
	if !seen["ModA"] || !seen["ModC"] {
		t.Error("expected ModA and ModC entities to be yielded")
	}
	if seen["ModB"] {
		t.Error("expected ModB entities to be filtered out")
	}
}

func TestEntitiesIterator_NoFilters(t *testing.T) {
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))

	seen := collectEntityModules(ctx)
	for _, mod := range []string{"ModA", "ModB", "ModC"} {
		if !seen[mod] {
			t.Errorf("expected %s entities with no filters", mod)
		}
	}
}

func TestMicroflowsIterator_InclusionFilter(t *testing.T) {
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))
	ctx.SetIncludedModules([]string{"ModB"})

	seen := collectMicroflowModules(ctx)
	if !seen["ModB"] {
		t.Error("expected ModB microflows to be yielded")
	}
	if seen["ModA"] || seen["ModC"] {
		t.Error("expected ModA and ModC microflows to be filtered out")
	}
}
