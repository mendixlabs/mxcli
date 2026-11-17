// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestNew(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}
	defer cat.Close()

	if cat.IsBuilt() {
		t.Error("New catalog should not be built")
	}
	if cat.ProjectID() != "default" {
		t.Errorf("Expected default project ID, got %q", cat.ProjectID())
	}
}

func TestSetProject(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}
	defer cat.Close()

	cat.SetProject("proj-1", "MyApp", "10.0.0")

	if cat.ProjectID() != "proj-1" {
		t.Errorf("Expected project ID 'proj-1', got %q", cat.ProjectID())
	}
	if cat.ProjectName() != "MyApp" {
		t.Errorf("Expected project name 'MyApp', got %q", cat.ProjectName())
	}
}

func TestSetBuilt(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}
	defer cat.Close()

	if cat.IsBuilt() {
		t.Error("New catalog should not be built")
	}

	cat.SetBuilt(true)
	if !cat.IsBuilt() {
		t.Error("Catalog should be built after SetBuilt(true)")
	}

	cat.SetBuilt(false)
	if cat.IsBuilt() {
		t.Error("Catalog should not be built after SetBuilt(false)")
	}
}

func TestTables(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}
	defer cat.Close()

	tables := cat.Tables()
	if len(tables) == 0 {
		t.Fatal("Expected non-empty table list")
	}

	// Verify some key tables exist
	expected := []string{
		"CATALOG.MODULES",
		"CATALOG.ENTITIES",
		"CATALOG.ATTRIBUTES",
		"CATALOG.MICROFLOWS",
		"CATALOG.PAGES",
		"CATALOG.WIDGETS",
		"CATALOG.DATABASE_CONNECTIONS",
	}
	tableSet := make(map[string]bool)
	for _, tbl := range tables {
		tableSet[tbl] = true
	}
	for _, exp := range expected {
		if !tableSet[exp] {
			t.Errorf("Expected table %q not found in Tables()", exp)
		}
	}
}

// TestTables_CoversAllViews is a drift guard: every catalog VIEW created by the
// schema must be listed in Tables(), so SHOW CATALOG TABLES never silently omits
// a document type. The churn is always in views — each new cataloged document
// type adds a viewWithFullSnapshot — so enforcing "every view is listed" catches
// the omission at test time (the failure mode that hid javascript_actions,
// image_collections, data_transformers, the agent-editor views, and
// navigation_profiles from SHOW CATALOG TABLES). Base tables (projects,
// snapshots, FTS shadows, catalog_meta) are type='table' and intentionally
// excluded from this invariant.
func TestTables_CoversAllViews(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}
	defer cat.Close()

	listed := make(map[string]bool)
	for _, tbl := range cat.Tables() {
		listed[strings.ToLower(strings.TrimPrefix(tbl, "CATALOG."))] = true
	}

	rows, err := cat.CatalogDB().Query(
		"SELECT name FROM sqlite_master WHERE type = 'view' ORDER BY name")
	if err != nil {
		t.Fatalf("Failed to enumerate views: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		// A retired name kept as an alias is deliberately unlisted (#1395).
		if !listed[name] && LegacyTableAliases[name] == "" {
			t.Errorf("catalog view %q is not listed in Tables() — add CATALOG.%s "+
				"so SHOW CATALOG TABLES includes it", name, strings.ToUpper(name))
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
}

func TestQueryEmptyTable(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}
	defer cat.Close()

	result, err := cat.Query("SELECT * FROM modules")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if result.Count != 0 {
		t.Errorf("Expected 0 rows in empty table, got %d", result.Count)
	}
	if len(result.Columns) == 0 {
		t.Error("Expected column names even for empty result")
	}
}

func TestQueryWithData(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}
	defer cat.Close()

	// Insert test data
	_, err = cat.CatalogDB().Exec(
		"INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName) VALUES (?, ?, ?, ?)",
		"mod-1", "TestModule", "TestModule", "TestModule",
	)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	result, err := cat.Query("SELECT Name FROM modules WHERE Id = 'mod-1'")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if result.Count != 1 {
		t.Fatalf("Expected 1 row, got %d", result.Count)
	}
	if result.Rows[0][0] != "TestModule" {
		t.Errorf("Expected 'TestModule', got %v", result.Rows[0][0])
	}
}

// TestObjectsView_IncludesAssociations verifies that associations are part of
// the unified objects index (catalog schema v3). They were previously only in
// the standalone associations table, forcing consumers to query it separately.
func TestObjectsView_IncludesAssociations(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}
	defer cat.Close()

	_, err = cat.CatalogDB().Exec(
		"INSERT INTO associations_data (Id, Name, QualifiedName, ModuleName) VALUES (?, ?, ?, ?)",
		"assoc-1", "Order_Customer", "Sales.Order_Customer", "Sales",
	)
	if err != nil {
		t.Fatalf("Failed to insert association: %v", err)
	}

	result, err := cat.Query("SELECT ObjectType FROM objects WHERE QualifiedName = 'Sales.Order_Customer'")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if result.Count != 1 {
		t.Fatalf("Expected 1 row in objects view, got %d", result.Count)
	}
	if got := result.Rows[0][0]; got != "ASSOCIATION" {
		t.Errorf("ObjectType = %v, want ASSOCIATION", got)
	}
}

// TestObjectsView_IncludesNewDocumentTypes verifies the document types added to
// the catalog in schema v4 (image collections, JavaScript actions, data
// transformers) surface in the unified objects index.
func TestObjectsView_IncludesNewDocumentTypes(t *testing.T) {
	cases := []struct {
		table      string
		objectType string
	}{
		{"javascript_actions", "JAVASCRIPT_ACTION"},
		{"image_collections", "IMAGE_COLLECTION"},
		// Both were built into their own tables but never joined to the objects
		// view, so bare `DESCRIBE Module.Name` could not resolve them.
		{"building_blocks", "BUILDING_BLOCK"},
		{"icon_collections", "ICON_COLLECTION"},
		{"menus", "MENU"},
		{"page_templates", "PAGE_TEMPLATE"},
		{"data_transformers", "DATA_TRANSFORMER"},
		{"agents", "AGENT"},
		{"ai_models", "AI_MODEL"},
		{"knowledge_bases", "KNOWLEDGE_BASE"},
		{"consumed_mcp_services", "CONSUMED_MCP_SERVICE"},
	}
	for _, tc := range cases {
		t.Run(tc.objectType, func(t *testing.T) {
			cat, err := New()
			if err != nil {
				t.Fatalf("Failed to create catalog: %v", err)
			}
			defer cat.Close()

			qn := "Mod." + tc.table
			if _, err := cat.CatalogDB().Exec(
				"INSERT INTO "+tc.table+"_data (Id, Name, QualifiedName, ModuleName) VALUES (?, ?, ?, ?)",
				"id-1", tc.table, qn, "Mod",
			); err != nil {
				t.Fatalf("insert: %v", err)
			}

			result, err := cat.Query("SELECT ObjectType FROM objects WHERE QualifiedName = '" + qn + "'")
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			if result.Count != 1 {
				t.Fatalf("expected 1 row in objects view, got %d", result.Count)
			}
			if got := result.Rows[0][0]; got != tc.objectType {
				t.Errorf("ObjectType = %v, want %s", got, tc.objectType)
			}
		})
	}
}

func TestQueryError(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}
	defer cat.Close()

	_, err = cat.Query("SELECT * FROM nonexistent_table")
	if err == nil {
		t.Error("Expected error for nonexistent table")
	}
}

func TestMetadata(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}
	defer cat.Close()

	// Set and get metadata
	if err := cat.SetMeta("test_key", "test_value"); err != nil {
		t.Fatalf("SetMeta failed: %v", err)
	}

	val, err := cat.GetMeta("test_key")
	if err != nil {
		t.Fatalf("GetMeta failed: %v", err)
	}
	if val != "test_value" {
		t.Errorf("Expected 'test_value', got %q", val)
	}

	// Get nonexistent key returns empty string
	val, err = cat.GetMeta("nonexistent")
	if err != nil {
		t.Fatalf("GetMeta for nonexistent key failed: %v", err)
	}
	if val != "" {
		t.Errorf("Expected empty string for nonexistent key, got %q", val)
	}

	// Overwrite metadata
	if err := cat.SetMeta("test_key", "updated_value"); err != nil {
		t.Fatalf("SetMeta overwrite failed: %v", err)
	}
	val, _ = cat.GetMeta("test_key")
	if val != "updated_value" {
		t.Errorf("Expected 'updated_value', got %q", val)
	}
}

func TestCacheInfo(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}
	defer cat.Close()

	modTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	buildDuration := 2 * time.Second

	err = cat.SetCacheInfo("/path/to/app.mpr", modTime, "10.0.0", "full", buildDuration)
	if err != nil {
		t.Fatalf("SetCacheInfo failed: %v", err)
	}

	info, err := cat.GetCacheInfo()
	if err != nil {
		t.Fatalf("GetCacheInfo failed: %v", err)
	}

	if info.MprPath != "/path/to/app.mpr" {
		t.Errorf("Expected MprPath '/path/to/app.mpr', got %q", info.MprPath)
	}
	if info.MendixVersion != "10.0.0" {
		t.Errorf("Expected MendixVersion '10.0.0', got %q", info.MendixVersion)
	}
	if info.BuildMode != "full" {
		t.Errorf("Expected BuildMode 'full', got %q", info.BuildMode)
	}
	if info.BuildDuration != buildDuration {
		t.Errorf("Expected BuildDuration %v, got %v", buildDuration, info.BuildDuration)
	}
}

func TestSnapshots(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}
	defer cat.Close()

	// No active snapshot initially
	if snap := cat.GetActiveSnapshot(); snap != nil {
		t.Error("Expected no active snapshot initially")
	}

	// Create snapshot
	snap := cat.CreateSnapshot("test-snap", SnapshotSourceLive)
	if snap == nil {
		t.Fatal("CreateSnapshot returned nil")
	}
	if snap.Name != "test-snap" {
		t.Errorf("Expected snapshot name 'test-snap', got %q", snap.Name)
	}
	if snap.Source != SnapshotSourceLive {
		t.Errorf("Expected source LIVE, got %q", snap.Source)
	}

	// Active snapshot should be set
	active := cat.GetActiveSnapshot()
	if active == nil {
		t.Fatal("Expected active snapshot after CreateSnapshot")
	}
	if active.ID != snap.ID {
		t.Errorf("Expected active snapshot ID %q, got %q", snap.ID, active.ID)
	}

	// Create second snapshot replaces active
	snap2 := cat.CreateSnapshot("snap-2", SnapshotSourceGit)
	active = cat.GetActiveSnapshot()
	if active.ID != snap2.ID {
		t.Errorf("Expected active snapshot to be snap-2, got %q", active.ID)
	}
}

func TestSaveAndLoadFromFile(t *testing.T) {
	// Create and populate catalog
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}

	cat.SetProject("proj-1", "TestApp", "10.0.0")
	_, err = cat.CatalogDB().Exec(
		"INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName) VALUES (?, ?, ?, ?)",
		"mod-1", "MyModule", "MyModule", "MyModule",
	)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	if err := cat.SetMeta("test_key", "test_value"); err != nil {
		t.Fatalf("SetMeta failed: %v", err)
	}

	// Save to file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "catalog.db")
	if err := cat.SaveToFile(filePath); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}
	cat.Close()

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("Catalog file was not created")
	}

	// Load from file
	loaded, err := NewFromFile(filePath)
	if err != nil {
		t.Fatalf("NewFromFile failed: %v", err)
	}
	defer loaded.Close()

	// Loaded catalog should be marked as built
	if !loaded.IsBuilt() {
		t.Error("Loaded catalog should be marked as built")
	}

	// Query data from loaded catalog
	result, err := loaded.Query("SELECT Name FROM modules WHERE Id = 'mod-1'")
	if err != nil {
		t.Fatalf("Query on loaded catalog failed: %v", err)
	}
	if result.Count != 1 {
		t.Fatalf("Expected 1 row from loaded catalog, got %d", result.Count)
	}
	if result.Rows[0][0] != "MyModule" {
		t.Errorf("Expected 'MyModule' from loaded catalog, got %v", result.Rows[0][0])
	}

	// Check metadata survived
	val, err := loaded.GetMeta("test_key")
	if err != nil {
		t.Fatalf("GetMeta on loaded catalog failed: %v", err)
	}
	if val != "test_value" {
		t.Errorf("Expected metadata 'test_value' from loaded catalog, got %q", val)
	}
}

func TestRoleMappingsTable(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}
	defer cat.Close()

	// Insert role mappings
	mappings := []struct {
		userRole   string
		moduleRole string
		module     string
	}{
		{"Administrator", "MyModule.Admin", "MyModule"},
		{"Administrator", "System.Administrator", "System"},
		{"User", "MyModule.User", "MyModule"},
		{"User", "System.User", "System"},
	}

	for _, m := range mappings {
		_, err := cat.CatalogDB().Exec(
			"INSERT INTO role_mappings (UserRoleName, ModuleRoleName, ModuleName) VALUES (?, ?, ?)",
			m.userRole, m.moduleRole, m.module,
		)
		if err != nil {
			t.Fatalf("Failed to insert role mapping: %v", err)
		}
	}

	// Query all mappings for Administrator
	result, err := cat.Query("SELECT ModuleRoleName FROM role_mappings WHERE UserRoleName = 'Administrator'")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if result.Count != 2 {
		t.Errorf("Expected 2 mappings for Administrator, got %d", result.Count)
	}

	// Query distinct module roles
	result, err = cat.Query("SELECT DISTINCT ModuleRoleName FROM role_mappings WHERE ModuleName = 'MyModule'")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if result.Count != 2 {
		t.Errorf("Expected 2 module roles in MyModule, got %d", result.Count)
	}

	// Verify ROLE_MAPPINGS is in Tables() list
	found := false
	for _, tbl := range cat.Tables() {
		if tbl == "CATALOG.ROLE_MAPPINGS" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected CATALOG.ROLE_MAPPINGS in Tables() list")
	}
}

func TestCreateTablesAreQueryable(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}
	defer cat.Close()

	// Verify all core tables can be queried
	coreTables := []string{
		"modules", "entities", "attributes", "microflows",
		"pages", "snippets", "building_blocks", "layouts", "enumerations",
		"java_actions", "projects", "snapshots", "catalog_meta",
		"workflows", "odata_clients", "odata_services",
		"business_event_services", "database_connections",
		"jar_dependencies", "role_mappings",
	}
	for _, tbl := range coreTables {
		_, err := cat.Query("SELECT * FROM " + tbl)
		if err != nil {
			t.Errorf("Failed to query table %q: %v", tbl, err)
		}
	}
}

// TestNormalizedViewsExposeSnapshotColumns checks that the per-table views
// (entities, modules, etc.) still expose ProjectName / SnapshotDate /
// SnapshotSource via JOIN on snapshots, even though those columns are no
// longer stored on the row itself.
func TestNormalizedViewsExposeSnapshotColumns(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cat.Close()

	// Seed a snapshot.
	_, err = cat.CatalogDB().Exec(`
		INSERT INTO snapshots (SnapshotId, SnapshotName, ProjectId, ProjectName,
			SnapshotDate, SnapshotSource, SourceBranch, SourceRevision)
		VALUES ('snap-1', 'Initial', 'proj-1', 'MyApp',
			'2026-05-20 10:00:00', 'GIT', 'main', 'abc123')`)
	if err != nil {
		t.Fatalf("seed snapshots: %v", err)
	}

	// Seed a module pointing at that snapshot.
	_, err = cat.CatalogDB().Exec(`
		INSERT INTO modules_data (Id, Name, QualifiedName, ProjectId, SnapshotId)
		VALUES ('mod-1', 'Sales', 'Sales', 'proj-1', 'snap-1')`)
	if err != nil {
		t.Fatalf("seed modules_data: %v", err)
	}

	// The view should surface columns sourced from the snapshots JOIN.
	row := cat.CatalogDB().QueryRow(`
		SELECT ProjectName, SnapshotDate, SnapshotSource, SourceBranch, SourceRevision
		FROM modules WHERE Id = 'mod-1'`)
	var projectName, snapshotDate, snapshotSource, sourceBranch, sourceRevision string
	if err := row.Scan(&projectName, &snapshotDate, &snapshotSource, &sourceBranch, &sourceRevision); err != nil {
		t.Fatalf("scan view row: %v", err)
	}
	if projectName != "MyApp" || snapshotSource != "GIT" || sourceBranch != "main" || sourceRevision != "abc123" {
		t.Errorf("view did not surface snapshot columns: got (%q, %q, %q, %q, %q)",
			projectName, snapshotDate, snapshotSource, sourceBranch, sourceRevision)
	}

	// Updating the snapshot should propagate through the view immediately.
	if _, err := cat.CatalogDB().Exec(
		`UPDATE snapshots SET SourceRevision = 'def456' WHERE SnapshotId = 'snap-1'`,
	); err != nil {
		t.Fatalf("update snapshots: %v", err)
	}
	row = cat.CatalogDB().QueryRow(`SELECT SourceRevision FROM modules WHERE Id = 'mod-1'`)
	if err := row.Scan(&sourceRevision); err != nil {
		t.Fatalf("scan after update: %v", err)
	}
	if sourceRevision != "def456" {
		t.Errorf("snapshot update did not propagate via view: got %q, want %q", sourceRevision, "def456")
	}
}

// TestCatalogSchemaVersionForcesRebuild checks that opening a cache that was
// written against an older schema version drops the old tables/views and also
// clears the cache-info metadata so upstream isCacheValid() / GetCacheInfo()
// callers don't mistake the empty cache for a valid one.
func TestCatalogSchemaVersionForcesRebuild(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cache.sqlite"

	// Save a cache to disk with the current schema and realistic cache-info
	// metadata (the kind that isCacheValid checks), then tamper with the
	// stored schema version in-place to simulate an old cache.
	cat, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := cat.SetCacheInfo("/tmp/app.mpr", time.Unix(1700000000, 0), "11.8.0", "full", time.Second); err != nil {
		t.Fatalf("SetCacheInfo: %v", err)
	}
	if _, err := cat.CatalogDB().Exec(
		`INSERT INTO modules_data (Id, Name, QualifiedName, ProjectId, SnapshotId)
		 VALUES ('mod-x', 'Stale', 'Stale', 'proj', 'snap')`,
	); err != nil {
		t.Fatalf("seed stale row: %v", err)
	}
	if err := cat.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}
	cat.Close()

	// Open the saved file directly (bypassing NewFromFile's migration check)
	// and downgrade the recorded schema version. NewFromFile next time should
	// notice the mismatch and drop everything.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open downgraded cache: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE catalog_meta SET Value = '0-old' WHERE Key = ?`, MetaSchemaVersion,
	); err != nil {
		db.Close()
		t.Fatalf("downgrade schema_version: %v", err)
	}
	// Confirm the seeded row survived the downgrade.
	var preCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM modules_data`).Scan(&preCount); err != nil {
		db.Close()
		t.Fatalf("count before migration: %v", err)
	}
	if preCount != 1 {
		db.Close()
		t.Fatalf("expected 1 seeded row before migration, got %d", preCount)
	}
	db.Close()

	// Reopen — migration should fire, dropping the seeded row.
	cat2, err := NewFromFile(path)
	if err != nil {
		t.Fatalf("NewFromFile (after downgrade): %v", err)
	}
	defer cat2.Close()

	var count int
	if err := cat2.CatalogDB().QueryRow(`SELECT COUNT(*) FROM modules_data`).Scan(&count); err != nil {
		t.Fatalf("count after migration: %v", err)
	}
	if count != 0 {
		t.Errorf("expected migration to drop seeded rows, got %d", count)
	}

	stored, err := cat2.GetMeta(MetaSchemaVersion)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if stored != CatalogSchemaVersion {
		t.Errorf("schema version not bumped after migration: got %q, want %q", stored, CatalogSchemaVersion)
	}

	// Cache-info keys must be cleared so callers re-build instead of
	// "loading" the now-empty cache. This is what mdl/executor's
	// isCacheValid relies on to detect the wiped state.
	info, err := cat2.GetCacheInfo()
	if err != nil {
		t.Fatalf("GetCacheInfo: %v", err)
	}
	if info.MprPath != "" {
		t.Errorf("MprPath should be cleared after migration, got %q", info.MprPath)
	}
	if !info.MprModTime.IsZero() {
		t.Errorf("MprModTime should be zero after migration, got %v", info.MprModTime)
	}
	if info.BuildMode != "" {
		t.Errorf("BuildMode should be cleared after migration, got %q", info.BuildMode)
	}
	if !info.BuildTime.IsZero() {
		t.Errorf("BuildTime should be zero after migration, got %v", info.BuildTime)
	}
}

// TestPartialViewsExposeSnapshotColumns checks the two partial-denormalization
// view helpers — viewWithSnapshotDateSource (used by rest_operations, etc.)
// and viewWithProjectNameAndSnapshotDateSource (used by external_entities,
// constants, etc.) — surface exactly the historical columns from their
// pre-refactor shape, not more and not less.
func TestPartialViewsExposeSnapshotColumns(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cat.Close()

	_, err = cat.CatalogDB().Exec(`
		INSERT INTO snapshots (SnapshotId, SnapshotName, ProjectId, ProjectName,
			SnapshotDate, SnapshotSource, SourceBranch, SourceRevision)
		VALUES ('snap-1', 'Initial', 'proj-1', 'MyApp',
			'2026-05-22 10:00:00', 'GIT', 'main', 'abc123')`)
	if err != nil {
		t.Fatalf("seed snapshots: %v", err)
	}

	// viewWithSnapshotDateSource — business_events historically had only
	// SnapshotDate + SnapshotSource (no ProjectName, no Source*).
	_, err = cat.CatalogDB().Exec(`
		INSERT INTO business_events_data (Id, ServiceId, ChannelName, MessageName,
			ProjectId, SnapshotId)
		VALUES ('ev-1', 'svc-1', 'ch', 'msg', 'proj-1', 'snap-1')`)
	if err != nil {
		t.Fatalf("seed business_events_data: %v", err)
	}

	row := cat.CatalogDB().QueryRow(
		`SELECT SnapshotDate, SnapshotSource FROM business_events WHERE Id = 'ev-1'`)
	var snapshotDate, snapshotSource string
	if err := row.Scan(&snapshotDate, &snapshotSource); err != nil {
		t.Fatalf("scan business_events view: %v", err)
	}
	if snapshotSource != "GIT" || snapshotDate == "" {
		t.Errorf("business_events view did not expose snapshot columns: (%q, %q)", snapshotDate, snapshotSource)
	}

	// The pre-refactor business_events table did not have ProjectName or
	// Source*. Confirm the view honors that — querying them should fail.
	if _, err := cat.CatalogDB().Exec(`SELECT ProjectName FROM business_events`); err == nil {
		t.Errorf("business_events view unexpectedly exposes ProjectName (drift from original schema)")
	}
	if _, err := cat.CatalogDB().Exec(`SELECT SourceBranch FROM business_events`); err == nil {
		t.Errorf("business_events view unexpectedly exposes SourceBranch (drift from original schema)")
	}

	// viewWithProjectNameAndSnapshotDateSource — external_entities
	// historically had ProjectName + SnapshotDate + SnapshotSource but no
	// Source*.
	_, err = cat.CatalogDB().Exec(`
		INSERT INTO external_entities_data (Id, Name, ServiceName, ProjectId, SnapshotId)
		VALUES ('ext-1', 'Customer', 'CRM', 'proj-1', 'snap-1')`)
	if err != nil {
		t.Fatalf("seed external_entities_data: %v", err)
	}

	row = cat.CatalogDB().QueryRow(
		`SELECT ProjectName, SnapshotDate, SnapshotSource FROM external_entities WHERE Id = 'ext-1'`)
	var projectName string
	if err := row.Scan(&projectName, &snapshotDate, &snapshotSource); err != nil {
		t.Fatalf("scan external_entities view: %v", err)
	}
	if projectName != "MyApp" || snapshotSource != "GIT" {
		t.Errorf("external_entities view did not expose snapshot columns: (%q, %q, %q)",
			projectName, snapshotDate, snapshotSource)
	}

	if _, err := cat.CatalogDB().Exec(`SELECT SourceBranch FROM external_entities`); err == nil {
		t.Errorf("external_entities view unexpectedly exposes SourceBranch (drift from original schema)")
	}
}

// A write that lands in the same second as the build that made the cache must
// still make the cache stale: the modification time is compared as an instant,
// not as whole seconds. A write one second later was always seen; one inside the
// same second was answered from the stale catalog.
func TestModTimeUnchanged(t *testing.T) {
	base := time.Date(2026, 11, 17, 19, 25, 30, 250_000_000, time.UTC)
	cases := []struct {
		name            string
		cached, current time.Time
		want            bool
	}{
		{"same instant", base, base, true},
		{"same instant, other zone", base, base.In(time.FixedZone("x", 3600)), true},
		{"later in the same second", base, base.Add(300 * time.Millisecond), false},
		{"earlier in the same second", base, base.Add(-200 * time.Millisecond), false},
		{"one nanosecond later", base, base.Add(time.Nanosecond), false},
		{"a second later", base, base.Add(time.Second), false},
		// A cache written before sub-second times were kept holds whole seconds; it
		// cannot tell whether the file changed inside that second, so it rebuilds once.
		{"legacy whole-second cache against a fractional time", base.Truncate(time.Second), base, false},
		{"legacy whole-second cache against a whole-second file", base.Truncate(time.Second), base.Truncate(time.Second), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ModTimeUnchanged(c.cached, c.current); got != c.want {
				t.Errorf("ModTimeUnchanged(%v, %v) = %v, want %v", c.cached, c.current, got, c.want)
			}
		})
	}
}

// The recorded time survives the round trip through the cache file to the
// nanosecond; RFC3339 kept only whole seconds, which is what made the check above
// impossible.
func TestCacheInfoKeepsModTimeToTheNanosecond(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	mt := time.Date(2026, 11, 17, 19, 25, 30, 123_456_789, time.UTC)
	if err := cat.SetCacheInfo("/p/app.mpr", mt, "11.8.0", "fast", time.Second); err != nil {
		t.Fatal(err)
	}
	info, err := cat.GetCacheInfo()
	if err != nil {
		t.Fatal(err)
	}
	if !ModTimeUnchanged(info.MprModTime, mt) {
		t.Errorf("recorded %v, read back %v", mt, info.MprModTime)
	}
}
