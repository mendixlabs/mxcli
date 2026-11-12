// SPDX-License-Identifier: Apache-2.0

// Package catalog provides SQL querying over Mendix project metadata.
package catalog

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Catalog provides SQL querying over Mendix project metadata.
type Catalog struct {
	db            CatalogDB
	projectID     string
	projectName   string
	mendixVersion string
	snapshots     map[string]*Snapshot
	activeSnap    string
	built         bool
}

// Snapshot represents a point-in-time view of project data.
type Snapshot struct {
	ID          string
	Name        string
	Date        time.Time
	Source      SnapshotSource
	SourceID    string
	Branch      string
	Revision    string
	ObjectCount int
	IsActive    bool
}

// SnapshotSource indicates where snapshot data came from.
type SnapshotSource string

const (
	SnapshotSourceLive    SnapshotSource = "LIVE"
	SnapshotSourceGit     SnapshotSource = "GIT"
	SnapshotSourceImport  SnapshotSource = "IMPORT"
	SnapshotSourceUnknown SnapshotSource = "UNKNOWN"
)

// QueryResult holds query results.
type QueryResult struct {
	Columns []string
	Rows    [][]any
	Count   int
}

// ProgressFunc is called during catalog building to report progress.
type ProgressFunc func(table string, count int)

// New creates a new catalog with an in-memory SQLite database.
func New() (*Catalog, error) {
	db, err := NewSqliteCatalogDB()
	if err != nil {
		return nil, fmt.Errorf("failed to open in-memory database: %w", err)
	}
	return NewFromDB(db)
}

// NewFromDB creates a catalog from an existing CatalogDB.
func NewFromDB(db CatalogDB) (*Catalog, error) {
	c := &Catalog{
		db:        db,
		projectID: "default",
		snapshots: make(map[string]*Snapshot),
	}

	// Create tables
	if err := c.createTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return c, nil
}

// SetProject sets the project metadata.
func (c *Catalog) SetProject(id, name, mendixVersion string) {
	c.projectID = id
	c.projectName = name
	c.mendixVersion = mendixVersion
}

// IsBuilt returns true if the catalog has been built.
func (c *Catalog) IsBuilt() bool {
	return c.built
}

// SetBuilt marks the catalog as built.
func (c *Catalog) SetBuilt(built bool) {
	c.built = built
}

// Tables returns the list of available catalog tables.
func (c *Catalog) Tables() []string {
	return []string{
		"CATALOG.MODULES",
		"CATALOG.FOLDERS",
		"CATALOG.ANNOTATIONS",
		"CATALOG.ENTITIES",
		"CATALOG.ASSOCIATIONS",
		"CATALOG.ATTRIBUTES",
		"CATALOG.MICROFLOWS",
		"CATALOG.NANOFLOWS",
		"CATALOG.RULES",
		"CATALOG.PAGES",
		"CATALOG.PAGE_TEMPLATES",
		"CATALOG.SNIPPETS",
		"CATALOG.BUILDING_BLOCKS",
		"CATALOG.LAYOUTS",
		"CATALOG.ENUMERATIONS",
		"CATALOG.ENUMERATION_VALUES",
		"CATALOG.MICROFLOW_PARAMETERS",
		"CATALOG.JAVA_ACTIONS",
		"CATALOG.JAVA_ACTION_PARAMETERS",
		"CATALOG.JAVASCRIPT_ACTIONS",
		"CATALOG.IMAGE_COLLECTIONS",
		"CATALOG.ICON_COLLECTIONS",
		"CATALOG.MENUS",
		"CATALOG.SCHEDULED_EVENTS",
		"CATALOG.TASK_QUEUES",
		"CATALOG.REGULAR_EXPRESSIONS",
		"CATALOG.DATA_TRANSFORMERS",
		"CATALOG.MESSAGE_DEFINITION_COLLECTIONS",
		"CATALOG.MESSAGE_DEFINITIONS",
		"CATALOG.AGENTS",
		"CATALOG.AI_MODELS",
		"CATALOG.KNOWLEDGE_BASES",
		"CATALOG.CONSUMED_MCP_SERVICES",
		"CATALOG.PUBLISHED_MCP_SERVICES",
		"CATALOG.AGENT_TOOLS",
		"CATALOG.AGENT_MCP_SERVICES",
		"CATALOG.AGENT_KNOWLEDGE_BASES",
		"CATALOG.NAVIGATION_PROFILES",
		"CATALOG.OFFLINE_ENTITY_CONFIGS",
		"CATALOG.ENTITY_EVENT_HANDLERS",
		"CATALOG.ACTIVITIES",
		"CATALOG.FLOWS",
		"CATALOG.WIDGETS",
		"CATALOG.WIDGET_DEFINITIONS",
		"CATALOG.WIDGET_DEFINITION_PROPERTIES",
		"CATALOG.XPATH_EXPRESSIONS",
		"CATALOG.REFS",
		"CATALOG.USER_ROLES",
		"CATALOG.ROLE_MAPPINGS",
		"CATALOG.PERMISSIONS",
		"CATALOG.PROJECTS",
		"CATALOG.SNAPSHOTS",
		"CATALOG.OBJECTS",
		"CATALOG.WORKFLOWS",
		"CATALOG.WORKFLOW_ACTIVITIES",
		"CATALOG.CONSUMED_ODATA_SERVICES",
		"CATALOG.PUBLISHED_ODATA_SERVICES",
		"CATALOG.BUSINESS_EVENT_SERVICES",
		"CATALOG.DATABASE_CONNECTIONS",
		"CATALOG.JAR_DEPENDENCIES",
		"CATALOG.CONSTANTS",
		"CATALOG.CONSTANT_VALUES",
		"CATALOG.CONSUMED_REST_SERVICES",
		"CATALOG.CONSUMED_REST_OPERATIONS",
		"CATALOG.PUBLISHED_REST_SERVICES",
		"CATALOG.PUBLISHED_REST_OPERATIONS",
		"CATALOG.EXTERNAL_ENTITIES",
		"CATALOG.EXTERNAL_ACTIONS",
		"CATALOG.BUSINESS_EVENTS",
		"CATALOG.CONTRACT_ENTITIES",
		"CATALOG.CONTRACT_ACTIONS",
		"CATALOG.CONTRACT_MESSAGES",
		"CATALOG.JSON_STRUCTURES",
		"CATALOG.IMPORT_MAPPINGS",
		"CATALOG.EXPORT_MAPPINGS",
		"CATALOG.STRINGS",
		"CATALOG.SOURCE",
		"CATALOG.GRAPH_GOD_NODES",
		"CATALOG.GRAPH_MODULE_COUPLING",
		"CATALOG.GRAPH_MODULE_COHESION",
		"CATALOG.GRAPH_DEAD_ASSETS",
		"CATALOG.GRAPH_REFKIND_DISTRIBUTION",
		"CATALOG.GRAPH_ENTITY_HOTSPOTS",
		"CATALOG.COMMUNITIES",
		"CATALOG.COMMUNITY_SUMMARY",
		"CATALOG.GRAPH_CYCLES",
		"CATALOG.GRAPH_MODULE_CYCLES",
		"CATALOG.GRAPH_ANALYSIS_SCOPE",
		"CATALOG.GRAPH_LAYERS",
		"CATALOG.GRAPH_CENTRALITY",
		"CATALOG.GRAPH_MODULE_DEPENDENCIES",
		"CATALOG.GRAPH_INTEGRATION_SURFACE",
	}
}

// Query executes a SQL query and returns results.
func (c *Catalog) Query(sqlQuery string) (*QueryResult, error) {
	rows, err := c.db.Query(sqlQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	result := &QueryResult{
		Columns: columns,
		Rows:    make([][]any, 0),
	}

	// Scan rows
	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		// Convert []byte to string for readability
		for i, v := range values {
			if b, ok := v.([]byte); ok {
				values[i] = string(b)
			}
		}

		result.Rows = append(result.Rows, values)
		result.Count++
	}

	return result, rows.Err()
}

// CatalogDB returns the underlying database abstraction.
func (c *Catalog) CatalogDB() CatalogDB {
	return c.db
}

// ProjectID returns the current project ID.
func (c *Catalog) ProjectID() string {
	return c.projectID
}

// ProjectName returns the current project name.
func (c *Catalog) ProjectName() string {
	return c.projectName
}

// Close releases catalog resources.
func (c *Catalog) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// GetActiveSnapshot returns the active snapshot.
func (c *Catalog) GetActiveSnapshot() *Snapshot {
	if c.activeSnap == "" {
		return nil
	}
	return c.snapshots[c.activeSnap]
}

// CreateSnapshot creates a new snapshot with the given parameters.
func (c *Catalog) CreateSnapshot(name string, source SnapshotSource) *Snapshot {
	id := fmt.Sprintf("snapshot-%d", len(c.snapshots)+1)
	snap := &Snapshot{
		ID:       id,
		Name:     name,
		Date:     time.Now(),
		Source:   source,
		IsActive: true,
	}
	c.snapshots[id] = snap
	c.activeSnap = id
	return snap
}

// Metadata keys for cache validation
const (
	MetaMprPath       = "mpr_path"
	MetaMprModTime    = "mpr_mod_time"
	MetaMendixVersion = "mendix_version"
	MetaBuildMode     = "build_mode"
	MetaBuildTime     = "build_time"
	MetaBuildDuration = "build_duration"

	// MetaGraphAnalysis records that AddGraphAnalysis has run against this
	// catalog, and MetaGraphResolution the Leiden resolution it used.
	//
	// Deliberately NOT another build mode. fast/full/source is a LEVEL — each
	// contains the one below, so a rank comparison decides everything. The graph
	// pass is orthogonal: it augments whatever level is there (that is the whole
	// point of AddGraphAnalysis being additive), so folding it into the rank
	// would make "source" and "full+graph" incomparable. It is a flag.
	//
	// Without it an un-run graph pass is indistinguishable from a graph with
	// nothing in it: the tables are empty either way, and the recorded mode still
	// says "full" (mendixlabs/mxcli#1060).
	MetaGraphAnalysis   = "graph_analysis"
	MetaGraphResolution = "graph_resolution"
)

// CacheInfo contains information about the cached catalog.
type CacheInfo struct {
	MprPath       string
	MprModTime    time.Time
	MendixVersion string
	BuildMode     string // "fast" or "full"
	BuildTime     time.Time
	BuildDuration time.Duration
	IsValid       bool
	InvalidReason string

	// GraphAnalysis reports whether the communities/cycles/layers/centrality
	// pass has run against this catalog, and at which resolution.
	GraphAnalysis   bool
	GraphResolution float64
}

// NewFromFile opens a catalog from a persisted SQLite file.
//
// Applies the current schema to the loaded database via CREATE TABLE IF NOT
// EXISTS so cache files written by older mxcli builds gain newly-added tables
// (e.g. widget_definitions) without forcing a full catalog rebuild. The new
// tables will be empty until the next REFRESH CATALOG runs, but they're
// queryable and won't trigger "no such table" errors.
//
// If the cached file was written against a different CatalogSchemaVersion,
// every table, view, and index is dropped before recreation. The cache will
// be empty until the next REFRESH CATALOG runs — the alternative (silently
// querying stale rows with the wrong column shape) is worse.
func NewFromFile(path string) (*Catalog, error) {
	db, err := NewSqliteCatalogDBFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open catalog file: %w", err)
	}

	c := &Catalog{
		db:        db,
		projectID: "default",
		snapshots: make(map[string]*Snapshot),
		built:     true, // Assume built if loading from file
	}

	if err := c.migrateIfSchemaMismatch(); err != nil {
		c.Close()
		return nil, fmt.Errorf("failed to migrate cached catalog: %w", err)
	}

	// A cache already at the current schema version was saved from a catalog
	// that ran createTables, so it is complete — open it without writing. Opening
	// must stay read-only: a parallel process may rename a fresh cache over this
	// path at any moment (SaveToFile is atomic, ako/mxcli#951), and SQLite refuses
	// a write to a file that has been moved ("attempt to write a readonly
	// database"), while concurrent openers writing the version row contend for
	// the lock.
	if stored, err := c.GetMeta(MetaSchemaVersion); err == nil && stored == CatalogSchemaVersion {
		return c, nil
	}

	// Idempotent schema upgrade — adds any tables/indexes the cached file
	// doesn't have yet. Existing data is untouched (unless dropped above).
	// createTables also records the current schema version in catalog_meta.
	if err := c.createTables(); err != nil {
		c.Close()
		return nil, fmt.Errorf("failed to apply schema to cached catalog: %w", err)
	}

	return c, nil
}

// migrateIfSchemaMismatch drops every user table, view, and index when the
// cache's recorded CatalogSchemaVersion differs from the current one. The
// catalog_meta table itself is preserved so callers can still introspect what
// happened. If the catalog_meta table doesn't exist yet (fresh cache), this
// is a no-op.
func (c *Catalog) migrateIfSchemaMismatch() error {
	var hasMeta int
	if err := c.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='catalog_meta'`,
	).Scan(&hasMeta); err != nil {
		return fmt.Errorf("probing catalog_meta: %w", err)
	}
	if hasMeta == 0 {
		return nil // fresh cache, nothing to migrate from
	}

	stored, err := c.GetMeta(MetaSchemaVersion)
	if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	if stored == CatalogSchemaVersion {
		return nil
	}

	// Drop every user object except catalog_meta itself.
	//
	// FTS5 virtual tables (strings, source) are intentionally skipped — they
	// are queried via FTS module hooks that can fail to load across
	// reconnections in modernc.org/sqlite, and their schema is stable across
	// catalog versions. They'll be reused as-is; their rows are derived from
	// the dropped tables and get rebuilt on the next REFRESH CATALOG anyway.
	for _, kind := range []string{"view", "index", "table"} {
		rows, err := c.db.Query(
			`SELECT name, sql FROM sqlite_master
			 WHERE type=? AND name NOT LIKE 'sqlite_%' AND name != 'catalog_meta'`,
			kind,
		)
		if err != nil {
			return fmt.Errorf("listing %ss: %w", kind, err)
		}
		type item struct{ name, sqlText string }
		var items []item
		for rows.Next() {
			var it item
			var sqlText sql.NullString
			if err := rows.Scan(&it.name, &sqlText); err != nil {
				rows.Close()
				return err
			}
			it.sqlText = sqlText.String
			items = append(items, it)
		}
		rows.Close()
		for _, it := range items {
			if kind == "table" && strings.Contains(strings.ToUpper(it.sqlText), "VIRTUAL TABLE") {
				continue
			}
			stmt := fmt.Sprintf(`DROP %s IF EXISTS %q`, strings.ToUpper(kind), it.name)
			if _, err := c.db.Exec(stmt); err != nil {
				return fmt.Errorf("dropping %s %s: %w", kind, it.name, err)
			}
		}
	}

	// Clear cache-info metadata so upstream isCacheValid() (in mdl/executor)
	// sees this as an unbuilt cache and triggers a rebuild on next access.
	// Without this, the file still claims a valid MprPath / MprModTime /
	// BuildMode and callers happily "load" an empty catalog.
	for _, key := range []string{
		MetaMprPath, MetaMprModTime, MetaMendixVersion,
		MetaBuildMode, MetaBuildTime, MetaBuildDuration,
		MetaGraphAnalysis, MetaGraphResolution, MetaSourceFingerprint,
	} {
		if _, err := c.db.Exec(`DELETE FROM catalog_meta WHERE Key = ?`, key); err != nil {
			return fmt.Errorf("clearing cache-info key %s: %w", key, err)
		}
	}

	return nil
}

// SaveToFile saves the catalog to a SQLite file.
// This copies the in-memory database to a file for persistence.
// Requires the underlying CatalogDB to be a *SqliteCatalogDB.
//
// The save is atomic: the database is written to a temporary file in the same
// directory and renamed over path. Parallel mxcli processes on one project all
// save the same cache (ako/mxcli#951: eight parallel `lint` runs), and writing in
// place let them collide — VACUUM INTO refused the file another process had just
// created, the manual fallback then failed with "table catalog_meta already
// exists" or "database is locked", and a reader could open a half-written file.
// With a rename every writer succeeds (the last one wins) and a reader sees the
// old cache or a complete new one, never a partial file.
func (c *Catalog) SaveToFile(path string) error {
	sdb, ok := c.db.(*SqliteCatalogDB)
	if !ok {
		return fmt.Errorf("SaveToFile requires a SQLite-backed catalog")
	}
	rawDB := sdb.RawDB()

	// Same directory, so the rename cannot cross a filesystem boundary.
	tmpFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := tmpFile.Name()
	tmpFile.Close()
	// VACUUM INTO requires the target to be absent or empty; it is empty.
	committed := false
	defer func() {
		if !committed {
			os.Remove(tmp)
		}
	}()

	// Use SQLite backup API via VACUUM INTO (SQLite 3.27+)
	// Fall back to manual copy if not available
	safePath := strings.ReplaceAll(tmp, "'", "''")
	if _, err := rawDB.Exec(fmt.Sprintf("VACUUM INTO '%s'", safePath)); err != nil {
		// Fall back: export and import, into a fresh empty temp file.
		if err := os.Truncate(tmp, 0); err != nil {
			return err
		}
		if err := c.saveToFileManual(tmp, rawDB); err != nil {
			return err
		}
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace catalog cache %s: %w", path, err)
	}
	committed = true
	return nil
}

// saveToFileManual saves by dumping and restoring (fallback method).
func (c *Catalog) saveToFileManual(path string, rawDB *sql.DB) error {
	// Open destination
	destDB, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer destDB.Close()

	// Get list of tables
	rows, err := rawDB.Query("SELECT name, sql FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return err
	}

	var tables []struct{ name, sql string }
	for rows.Next() {
		var t struct{ name, sql string }
		if err := rows.Scan(&t.name, &t.sql); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, t)
	}
	rows.Close()

	// Create tables and copy data
	for _, t := range tables {
		// Create table
		if _, err := destDB.Exec(t.sql); err != nil {
			return fmt.Errorf("failed to create table %s: %w", t.name, err)
		}

		// Copy data
		srcRows, err := rawDB.Query(fmt.Sprintf("SELECT * FROM %s", t.name))
		if err != nil {
			return err
		}

		cols, _ := srcRows.Columns()
		if len(cols) == 0 {
			srcRows.Close()
			continue
		}

		// Build insert statement
		placeholders := make([]string, len(cols))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		insertSQL := fmt.Sprintf("INSERT INTO %s VALUES (%s)", t.name,
			join(placeholders, ","))

		for srcRows.Next() {
			values := make([]any, len(cols))
			valuePtrs := make([]any, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := srcRows.Scan(valuePtrs...); err != nil {
				srcRows.Close()
				return err
			}
			if _, err := destDB.Exec(insertSQL, values...); err != nil {
				srcRows.Close()
				return err
			}
		}
		srcRows.Close()
	}

	// Copy views
	rows, err = rawDB.Query("SELECT sql FROM sqlite_master WHERE type='view'")
	if err != nil {
		return err
	}
	for rows.Next() {
		var viewSQL string
		if err := rows.Scan(&viewSQL); err != nil {
			rows.Close()
			return err
		}
		if viewSQL != "" {
			destDB.Exec(viewSQL) // Ignore errors for views
		}
	}
	rows.Close()

	// Copy indexes
	rows, err = rawDB.Query("SELECT sql FROM sqlite_master WHERE type='index' AND sql IS NOT NULL")
	if err != nil {
		return err
	}
	for rows.Next() {
		var indexSQL string
		if err := rows.Scan(&indexSQL); err != nil {
			rows.Close()
			return err
		}
		if indexSQL != "" {
			destDB.Exec(indexSQL) // Ignore errors for indexes
		}
	}
	rows.Close()

	return nil
}

// join is a simple string join helper.
func join(s []string, sep string) string {
	if len(s) == 0 {
		return ""
	}
	var result strings.Builder
	result.WriteString(s[0])
	for i := 1; i < len(s); i++ {
		result.WriteString(sep + s[i])
	}
	return result.String()
}

// SetMeta sets a metadata value in the catalog.
func (c *Catalog) SetMeta(key, value string) error {
	_, err := c.db.Exec(
		"INSERT OR REPLACE INTO catalog_meta (Key, Value) VALUES (?, ?)",
		key, value)
	return err
}

// GetMeta gets a metadata value from the catalog.
func (c *Catalog) GetMeta(key string) (string, error) {
	var value string
	err := c.db.QueryRow("SELECT Value FROM catalog_meta WHERE Key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// GetCacheInfo returns information about the cached catalog.
func (c *Catalog) GetCacheInfo() (*CacheInfo, error) {
	info := &CacheInfo{}

	info.MprPath, _ = c.GetMeta(MetaMprPath)
	info.MendixVersion, _ = c.GetMeta(MetaMendixVersion)
	info.BuildMode, _ = c.GetMeta(MetaBuildMode)

	if modTimeStr, _ := c.GetMeta(MetaMprModTime); modTimeStr != "" {
		info.MprModTime, _ = time.Parse(time.RFC3339, modTimeStr)
	}
	if buildTimeStr, _ := c.GetMeta(MetaBuildTime); buildTimeStr != "" {
		info.BuildTime, _ = time.Parse(time.RFC3339, buildTimeStr)
	}
	if durationStr, _ := c.GetMeta(MetaBuildDuration); durationStr != "" {
		info.BuildDuration, _ = time.ParseDuration(durationStr)
	}
	if v, _ := c.GetMeta(MetaGraphAnalysis); v != "" {
		info.GraphAnalysis = true
	}
	if v, _ := c.GetMeta(MetaGraphResolution); v != "" {
		info.GraphResolution, _ = strconv.ParseFloat(v, 64)
	}

	return info, nil
}

// SetCacheInfo stores cache metadata after building.
func (c *Catalog) SetCacheInfo(mprPath string, mprModTime time.Time, mendixVersion, buildMode string, buildDuration time.Duration) error {
	if err := c.SetMeta(MetaMprPath, mprPath); err != nil {
		return err
	}
	if err := c.SetMeta(MetaMprModTime, mprModTime.Format(time.RFC3339)); err != nil {
		return err
	}
	if err := c.SetMeta(MetaMendixVersion, mendixVersion); err != nil {
		return err
	}
	if err := c.SetMeta(MetaBuildMode, buildMode); err != nil {
		return err
	}
	if err := c.SetMeta(MetaBuildTime, time.Now().Format(time.RFC3339)); err != nil {
		return err
	}
	if err := c.SetMeta(MetaBuildDuration, buildDuration.String()); err != nil {
		return err
	}
	return nil
}
