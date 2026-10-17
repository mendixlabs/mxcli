// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// syncWriter wraps an io.Writer with a mutex so concurrent goroutines
// (e.g. background catalog build) can write without racing.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (sw *syncWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}

// execShowCatalogTables handles SHOW CATALOG TABLES.
func execShowCatalogTables(ctx *ExecContext) error {
	// Build catalog if not already built (fast mode by default)
	if ctx.Catalog == nil || !ctx.Catalog.IsBuilt() {
		if err := ensureCatalog(ctx, false); err != nil {
			return err
		}
	}

	tables := ctx.Catalog.Tables()
	fmt.Fprintf(ctx.progress(), "\nFound %d catalog table(s)\n", len(tables))

	// Get row counts for each table
	type tableInfo struct {
		name  string
		count int
	}
	infos := make([]tableInfo, 0, len(tables))

	for _, t := range tables {
		// Get count for this table
		actualTable := strings.TrimPrefix(strings.ToLower(t), "catalog.")
		result, err := ctx.Catalog.Query(fmt.Sprintf("select count(*) from %s", actualTable))
		count := 0
		if err == nil && len(result.Rows) > 0 {
			if v, ok := result.Rows[0][0].(int64); ok {
				count = int(v)
			}
		}
		infos = append(infos, tableInfo{name: t, count: count})
	}

	tr := &TableResult{
		Columns: []string{"Table", "Count"},
	}
	for _, info := range infos {
		tr.Rows = append(tr.Rows, []any{info.name, info.count})
	}
	return writeResult(ctx, tr)
}

// fullOnlyTables are catalog tables only populated by REFRESH CATALOG FULL.
var fullOnlyTables = map[string]bool{
	"widgets":           true,
	"activities":        true,
	"refs":              true,
	"xpath_expressions": true,
	"strings":           true,
	"permissions":       true,
	// graph_* analysis views read the refs graph, so they need full mode too.
	"graph_god_nodes":            true,
	"graph_module_coupling":      true,
	"graph_module_cohesion":      true,
	"graph_dead_assets":          true,
	"graph_refkind_distribution": true,
	"graph_entity_hotspots":      true,
	"graph_analysis_scope":       true,
}

// sourceOnlyTables are catalog tables only populated by REFRESH CATALOG FULL SOURCE.
var sourceOnlyTables = map[string]bool{
	"source": true,
}

// communitiesOnlyTables are populated by the graph-analysis pass (REFRESH
// CATALOG COMMUNITIES), not by any build mode. They were the ONLY graph tables
// missing from the lists above, which is exactly why mendixlabs/mxcli#1060 was
// reported the way it was: GRAPH_MODULE_COUPLING is a plain view over refs, so
// it answered from any full catalog, while GRAPH_CYCLES answered "0 rows" — the
// same output as "no cycles found". A query against these is a mode question no
// rank can settle, since the pass augments fast/full/source alike; it is
// answered by the graph-analysis flag on the catalog.
var communitiesOnlyTables = map[string]bool{
	"communities":               true,
	"community_summary":         true,
	"graph_cycles":              true,
	"graph_module_cycles":       true,
	"graph_layers":              true,
	"graph_centrality":          true,
	"graph_integration_surface": true,
}

// extractTableFromQuery extracts the table name after FROM in a converted catalog query.
func extractTableFromQuery(query string) string {
	lower := strings.ToLower(query)
	idx := strings.Index(lower, "from ")
	if idx == -1 {
		return ""
	}
	rest := strings.TrimSpace(lower[idx+5:])
	// Table name is the next word
	end := strings.IndexAny(rest, " \t\n;,)")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

// warnIfCatalogModeInsufficient checks if the query targets a table that requires
// a higher catalog build mode than what's currently cached, and prints a warning.
func warnIfCatalogModeInsufficient(ctx *ExecContext, query string) {
	table := extractTableFromQuery(query)
	if table == "" {
		return
	}

	// Determine current build mode, and whether the graph pass has run.
	buildMode := "fast"
	graphAnalysis := false
	if ctx.Catalog != nil {
		if info, err := ctx.Catalog.GetCacheInfo(); err == nil {
			if info.BuildMode != "" {
				buildMode = info.BuildMode
			}
			graphAnalysis = info.GraphAnalysis
		}
	}

	modeRank := map[string]int{"fast": 1, "full": 2, "source": 3}
	currentRank := modeRank[buildMode]

	if communitiesOnlyTables[table] && !graphAnalysis {
		fmt.Fprintf(ctx.progress(), "Warning: CATALOG.%s requires refresh catalog communities (not run for this catalog)\n", strings.ToUpper(table))
		return
	}
	if sourceOnlyTables[table] && currentRank < modeRank["source"] {
		fmt.Fprintf(ctx.progress(), "Warning: CATALOG.%s requires refresh catalog full source (current mode: %s)\n", strings.ToUpper(table), buildMode)
	} else if fullOnlyTables[table] && currentRank < modeRank["full"] {
		fmt.Fprintf(ctx.progress(), "Warning: CATALOG.%s requires refresh catalog full (current mode: %s)\n", strings.ToUpper(table), buildMode)
	}
}

// execCatalogQuery handles SELECT ... FROM CATALOG.xxx queries.
func execCatalogQuery(ctx *ExecContext, query string) error {
	// Build catalog if not already built (fast mode by default)
	if ctx.Catalog == nil || !ctx.Catalog.IsBuilt() {
		if err := ensureCatalog(ctx, false); err != nil {
			return err
		}
	}

	// Convert CATALOG.xxx table names to actual table names
	query = convertCatalogTableNames(query)

	// Warn if querying a table that requires a higher build mode
	warnIfCatalogModeInsufficient(ctx, query)

	// Execute query
	result, err := ctx.Catalog.Query(query)
	if err != nil {
		return mdlerrors.NewBackend("execute catalog query", err)
	}

	// Output results
	fmt.Fprintf(ctx.progress(), "Found %d result(s)\n", result.Count)
	if result.Count == 0 {
		return writeEmptyResult(ctx, result.Columns, "(no results)")
	}

	outputCatalogResults(ctx, result)
	return nil
}

// tableRequiredMode returns the command that populates a table.
func tableRequiredMode(table string) string {
	if communitiesOnlyTables[table] {
		return "refresh catalog communities"
	}
	if sourceOnlyTables[table] {
		return "refresh catalog full source"
	}
	if fullOnlyTables[table] {
		return "refresh catalog full"
	}
	return "refresh catalog"
}

// execDescribeCatalogTable handles DESCRIBE CATALOG.tablename.
func execDescribeCatalogTable(ctx *ExecContext, stmt *ast.DescribeCatalogTableStmt) error {
	// Build catalog if not already built (fast mode by default)
	if ctx.Catalog == nil || !ctx.Catalog.IsBuilt() {
		if err := ensureCatalog(ctx, false); err != nil {
			return err
		}
	}

	tableName := stmt.TableName

	// Query column info using PRAGMA
	result, err := ctx.Catalog.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil || result.Count == 0 {
		return mdlerrors.NewNotFoundMsg("catalog table", strings.ToUpper(tableName), "unknown catalog table: CATALOG."+strings.ToUpper(tableName))
	}

	// Print table header. A catalog table's columns are a definition, not MDL
	// (R6): SQL's DESCRIBE TABLE.
	fmt.Fprintf(ctx.Output, "-- catalog table definition (not executable)\nCATALOG.%s\n", strings.ToUpper(tableName))
	fmt.Fprintf(ctx.Output, "Requires: %s\n\n", tableRequiredMode(tableName))

	// PRAGMA table_info returns: cid, name, type, notnull, dflt_value, pk
	tr := &TableResult{
		Columns: []string{"Column", "Type", "PK"},
	}
	for _, row := range result.Rows {
		name := fmt.Sprintf("%v", row[1])
		typ := fmt.Sprintf("%v", row[2])
		pkMark := ""
		if v, ok := row[5].(int64); ok && v > 0 {
			pkMark = "*"
		}
		tr.Rows = append(tr.Rows, []any{name, typ, pkMark})
	}
	return writeResult(ctx, tr)
}

// Catalog build modes nest: source ⊃ full ⊃ fast. A source catalog answers
// everything a full one does and adds the MDL source index; a full one answers
// everything a fast one does and adds activities, widgets, refs, permissions,
// strings and xpath. catalogModeRank orders them so the two questions that
// matter — "does this cache satisfy what I need?" and "would writing this cache
// lose something?" — are both comparisons rather than case analysis.
//
// An unknown mode ranks 0, below every real one, so it never masquerades as
// something richer than it is.
func catalogModeRank(mode string) int {
	switch mode {
	case "fast":
		return 1
	case "full":
		return 2
	case "source":
		return 3
	}
	return 0
}

// cachedCatalogMode reports the build mode recorded in the on-disk cache, or ""
// when there is no readable cache. Deliberately indifferent to whether that
// cache is still *valid*: a stale source-mode cache is stale data but a live
// statement of the level this project is set up for, and that is what callers
// here are asking about.
func cachedCatalogMode(ctx *ExecContext) string {
	info := cachedCatalogInfo(ctx)
	if info == nil {
		return ""
	}
	return info.BuildMode
}

// cachedCatalogInfo reads the on-disk cache's metadata, or nil when there is no
// readable cache. Same indifference to validity as cachedCatalogMode.
func cachedCatalogInfo(ctx *ExecContext) *catalog.CacheInfo {
	cachePath := getCachePath(ctx)
	if cachePath == "" {
		return nil
	}
	if _, err := os.Stat(cachePath); err != nil {
		return nil
	}
	cat, err := catalog.NewFromFile(cachePath)
	if err != nil {
		return nil
	}
	defer cat.Close()
	info, err := cat.GetCacheInfo()
	if err != nil {
		return nil
	}
	return info
}

// ensureCatalog ensures a catalog is available, using cache if possible.
func ensureCatalog(ctx *ExecContext, full bool) error {
	requiredMode := "fast"
	if full {
		requiredMode = "full"
	}

	// Try to load from cache first
	cachePath := getCachePath(ctx)
	if cachePath != "" {
		valid, _ := isCacheValid(ctx, cachePath, requiredMode)
		if valid {
			return loadCachedCatalog(ctx, cachePath)
		}
	}

	// Guard against building without a connection.
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// A rebuild that is going to pay the full-mode cost anyway rebuilds at the
	// level the project is set up for, so the cache comes back current instead
	// of being refused by buildCatalog's never-narrow guard on every call.
	// Without this, `search` / `show references` on a source-mode project would
	// rebuild full, decline to save, and do it again next time.
	//
	// A *fast* consumer is deliberately not upgraded. It is the hot path — the
	// tier `mxcli check -p`, `show structure` and `describe` reach on every
	// invocation, and the cache goes stale on every project save — so making it
	// pay for a source reindex would trade one papercut for a worse one. It
	// builds narrow in memory and the guard keeps its result off disk.
	isSource := false
	if full && cachedCatalogMode(ctx) == "source" {
		isSource = true
	}

	// An implicit rebuild is a bounded build that happens to run inside some
	// other statement, so the statement's wall-clock guard does not count it —
	// as it does not count REFRESH CATALOG (#651). Counted, the default 5m
	// fired mid-build on a 3,000-document app, the unfinished build was never
	// saved, and every later SHOW REFERENCES / SEARCH started it over
	// (mendixlabs/mxcli#1329).
	mode := "fast"
	if isSource {
		mode = "source"
	} else if full {
		mode = "full"
	}
	defer beginUntimed(ctx.Context, mode+" mode")()
	if implicitCatalogBuildHook != nil {
		defer implicitCatalogBuildHook()()
	}

	// Build fresh catalog
	return buildCatalog(ctx, full, isSource, false, 0)
}

// implicitCatalogBuildHook, when set, runs at the start of every implicit
// catalog rebuild, and the function it returns at the end. Tests use it to
// make a build outlast a short timeout and to wait for an abandoned one.
var implicitCatalogBuildHook func() (done func())

// getCachePath returns the path to the catalog cache file for the current project.
func getCachePath(ctx *ExecContext) string {
	if ctx.MprPath == "" {
		return ""
	}
	dir := filepath.Dir(ctx.MprPath)
	cacheDir := filepath.Join(dir, ".mxcli")
	return filepath.Join(cacheDir, "catalog.db")
}

// getMprModTime returns the modification time of the current MPR file.
func getMprModTime(ctx *ExecContext) time.Time {
	if ctx.MprPath == "" {
		return time.Time{}
	}
	info, err := os.Stat(ctx.MprPath)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// isCacheValid checks if the cached catalog is still valid.
func isCacheValid(ctx *ExecContext, cachePath string, requiredMode string) (bool, string) {
	// Check if cache file exists
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return false, "cache file does not exist"
	}

	// Load cache to check metadata
	cat, err := catalog.NewFromFile(cachePath)
	if err != nil {
		return false, fmt.Sprintf("failed to open cache: %v", err)
	}
	defer cat.Close()

	info, err := cat.GetCacheInfo()
	if err != nil {
		return false, fmt.Sprintf("failed to read cache info: %v", err)
	}

	// Check MPR path matches
	if info.MprPath != ctx.MprPath {
		return false, "MPR path changed"
	}

	// Check MPR modification time (compare Unix seconds to handle timezone/precision issues)
	currentModTime := getMprModTime(ctx)
	if info.MprModTime.Unix() != currentModTime.Unix() {
		return false, "project file modified"
	}

	// Check build mode hierarchy: source > full > fast
	cachedRank := catalogModeRank(info.BuildMode)
	requiredRank := catalogModeRank(requiredMode)
	if requiredRank > cachedRank {
		return false, fmt.Sprintf("%s mode requested but cache is %s mode", requiredMode, info.BuildMode)
	}

	return true, ""
}

// loadCachedCatalog loads a catalog from the cache file.
func loadCachedCatalog(ctx *ExecContext, cachePath string) error {
	cat, err := catalog.NewFromFile(cachePath)
	if err != nil {
		return err
	}

	info, err := cat.GetCacheInfo()
	if err != nil {
		cat.Close()
		return err
	}

	ctx.Catalog = cat
	if !ctx.Quiet {
		age := time.Since(info.BuildTime)
		fmt.Fprintf(ctx.progress(), "Loading cached catalog (built %s ago, %s mode)...\n",
			formatDuration(age), info.BuildMode)
		fmt.Fprintf(ctx.progress(), "✓ Catalog ready (from cache)\n")
	}
	return nil
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}

// carryGraphAnalysis decides whether a rebuild should re-run the graph pass the
// cache it is about to replace already had, and at which resolution.
//
// A rebuild starts from an empty catalog, so it drops the graph-analysis tables
// — and the recorded mode stays "full", so nothing downstream could tell. That
// is the same shape as #1081 (a narrower build overwriting a richer cache) and
// the reason mendixlabs/mxcli#1060's GRAPH_CYCLES was empty on a project whose
// GRAPH_MODULE_COUPLING had rows. Re-running re-reads the refs the rebuild just
// wrote and re-parses nothing.
//
// Only for a build that is already full. A fast build must NOT be promoted: it
// never overwrites a full cache (the #1081 guard in buildCatalog), so it has
// nothing to preserve, and promoting it would make `mxcli check -p` pay for a
// full parse plus a graph pass.
func carryGraphAnalysis(prior *catalog.CacheInfo, full, communities bool) (bool, float64) {
	if communities || !full || prior == nil || !prior.GraphAnalysis {
		return false, 0
	}
	return true, prior.GraphResolution
}

// buildCatalog builds the catalog from the project.
func buildCatalog(ctx *ExecContext, full, isSource, communities bool, resolution float64) error {
	if isSource || communities {
		full = true // both imply full (they read refs/source built in full mode)
	}

	if carry, res := carryGraphAnalysis(cachedCatalogInfo(ctx), full, communities); carry {
		communities, resolution = true, res
		if !ctx.Quiet {
			fmt.Fprintln(ctx.progress(), "Carrying over graph analysis (this catalog has it)...")
		}
	}

	if !ctx.Quiet {
		if isSource {
			fmt.Fprintln(ctx.progress(), "Building catalog (source mode - includes MDL source)...")
		} else if full {
			fmt.Fprintln(ctx.progress(), "Building catalog (full mode - includes activities/widgets)...")
		} else {
			fmt.Fprintln(ctx.progress(), "Building catalog (fast mode)...")
		}
	}
	start := time.Now()

	// Create new catalog
	cat, err := catalog.New()
	if err != nil {
		return mdlerrors.NewBackend("create catalog", err)
	}

	// Set project metadata
	version, _ := ctx.Backend.GetMendixVersion()
	cat.SetProject("default", "Current Project", version)

	// Build catalog
	builder := catalog.NewBuilder(cat, ctx.Backend)
	builder.SetFullMode(full)
	if communities {
		builder.SetCommunitiesMode(true, resolution)
	}
	if isSource {
		builder.SetSourceMode(true)
		preWarmCache(ctx)
		builder.SetDescribeFunc(func(objectType string, qualifiedName string, id string) (string, error) {
			return captureDescribeParallel(ctx, objectType, qualifiedName, model.ID(id))
		})
	}
	// Supply built-in widget definitions (hand-crafted COMBOBOX, GALLERY,
	// DATAGRID, filter widgets, …) for the widget_definitions catalog table.
	// Project-level widgets (widgets/*.mpk) are added by the catalog builder
	// itself by reading the filesystem.
	builder.SetBuiltinWidgetMetas(builtinWidgetMetas())
	// Per-table lines are the only sign of life during a build that takes
	// minutes on a large app (18 on a 140MB one), so quiet mode moves them off
	// stdout rather than dropping them: `search -q --format names` promises one
	// name per line, and forty "✓ Table: N" lines ahead of the names on a cold
	// cache broke that promise.
	buildProgress := ctx.progress()
	if ctx.Quiet {
		buildProgress = ctx.diagnostics()
	}
	err = builder.Build(func(table string, count int) {
		fmt.Fprintf(buildProgress, "✓ %s: %d\n", table, count)
	})
	if err != nil {
		cat.Close()
		return mdlerrors.NewBackend("build catalog", err)
	}

	elapsed := time.Since(start)

	// Save cache metadata
	buildMode := "fast"
	if isSource {
		buildMode = "source"
	} else if full {
		buildMode = "full"
	}
	cat.SetCacheInfo(ctx.MprPath, getMprModTime(ctx), version, buildMode, elapsed)

	cat.SetBuilt(true)
	ctx.Catalog = cat

	if !ctx.Quiet {
		fmt.Fprintf(ctx.progress(), "✓ Catalog ready (%.1fs)\n", elapsed.Seconds())
	}

	// Save to cache file — unless doing so would narrow it.
	//
	// The cache records the mode it was built in, and a build that needs less
	// than the cache holds must not become the new cache. mendixlabs/mxcli#1081:
	// once the project file changed, a `mxcli check -p` rebuilt fast (all it
	// needs is attribute types and enum cases) and saved over a source-mode
	// cache, taking source, refs, permissions, strings and xpath_expressions
	// with it. The commands that read those tables then answered "requires
	// refresh catalog full source" instead of answering.
	//
	// This is the choke point on purpose. `refresh catalog communities` was
	// given a local fix for the identical hazard — its comment below still
	// describes it — and every other caller kept the bug, which is exactly the
	// shape of failure a guard at one call site produces.
	cachePath := getCachePath(ctx)
	if cachePath != "" {
		if existing := cachedCatalogMode(ctx); catalogModeRank(existing) > catalogModeRank(buildMode) {
			if !ctx.Quiet {
				fmt.Fprintf(ctx.progress(), "Keeping the existing %s-mode catalog cache (this build was %s mode)\n", existing, buildMode)
			}
			return nil
		}
		cacheDir := filepath.Dir(cachePath)
		if err := os.MkdirAll(cacheDir, 0755); err == nil {
			// SaveToFile replaces the cache atomically. Do not remove it first:
			// that opened a window in which a parallel process found no cache,
			// and the in-place write that followed raced it (ako/mxcli#951).
			if err := cat.SaveToFile(cachePath); err != nil {
				fmt.Fprintf(ctx.progress(), "Warning: failed to save catalog cache: %v\n", err)
			} else {
				fmt.Fprintf(buildProgress, "✓ Catalog cached to %s\n", cachePath)
			}
		}
	}

	return nil
}

// execRefreshCatalogStmt handles REFRESH CATALOG [FULL] [SOURCE] [FORCE] [BACKGROUND] command.
func execRefreshCatalogStmt(ctx *ExecContext, stmt *ast.RefreshCatalogStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	requiredMode := "fast"
	if stmt.Full || stmt.Communities {
		requiredMode = "full"
	}
	if stmt.Source {
		requiredMode = "source"
	}

	// Check cache unless FORCE is specified. Communities mode always rebuilds: a
	// cached full catalog does not contain the graph-analysis tables.
	if !stmt.Force && !stmt.Communities {
		cachePath := getCachePath(ctx)
		if cachePath != "" {
			valid, reason := isCacheValid(ctx, cachePath, requiredMode)
			if valid {
				// Close existing catalog if any
				if ctx.Catalog != nil {
					ctx.Catalog.Close()
					ctx.Catalog = nil
				}
				return loadCachedCatalog(ctx, cachePath)
			}
			if reason != "cache file does not exist" {
				fmt.Fprintf(ctx.Output, "Cache invalid: %s\n", reason)
				// If project file was modified, reconnect to get fresh database connection
				if reason == "project file modified" {
					if err := reconnect(ctx); err != nil {
						return mdlerrors.NewBackend("reconnect after project modification", err)
					}
				}
			}
		}
	}

	// Communities: augment the EXISTING catalog with the graph-analysis tables
	// rather than rebuilding it. A rebuild would downgrade a source-mode catalog
	// to full (dropping the source FTS data) and needlessly re-parse the project —
	// the graph pass only needs the refs table. ensureCatalog loads the current
	// full/source cache (preserving its mode) or builds full if none is valid.
	if stmt.Communities {
		cachePath := getCachePath(ctx)
		// If a refs-bearing cache already exists, ensureCatalog loads it
		// file-backed — AddGraphAnalysis then writes straight to disk, so no
		// re-save is needed and the existing mode (incl. source FTS) is kept. Only
		// a freshly-built (in-memory) catalog needs persisting.
		loadedFromCache := false
		if cachePath != "" {
			if valid, _ := isCacheValid(ctx, cachePath, "full"); valid {
				loadedFromCache = true
			}
		}
		if err := ensureCatalog(ctx, true); err != nil {
			return err
		}
		if err := ctx.Catalog.AddGraphAnalysis(stmt.Resolution); err != nil {
			return mdlerrors.NewBackend("graph analysis", err)
		}
		if !loadedFromCache && cachePath != "" {
			if err := ctx.Catalog.SaveToFile(cachePath); err != nil {
				fmt.Fprintf(ctx.Output, "Warning: failed to save catalog cache: %v\n", err)
			}
		}
		fmt.Fprintln(ctx.Output, "✓ Graph analysis complete")
		return nil
	}

	// REFRESH CATALOG means "bring what this project has up to date", not
	// "change its level". There is no syntax for lowering the level, so reading
	// a bare REFRESH CATALOG as a request to drop the source index would make
	// the loss both silent and unaskable-for. Refresh at least what is cached.
	full, source := stmt.Full, stmt.Source
	switch cachedCatalogMode(ctx) {
	case "source":
		full, source = true, true
	case "full":
		full = true
	}
	if (full != stmt.Full || source != stmt.Source) && !ctx.Quiet {
		mode := "full"
		if source {
			mode = "source"
		}
		fmt.Fprintf(ctx.Output, "Refreshing at %s mode to match the existing cache\n", mode)
	}

	// Close existing catalog if any
	if ctx.Catalog != nil {
		ctx.Catalog.Close()
		ctx.Catalog = nil
	}

	// Handle background mode — clone ctx so the goroutine doesn't race
	// with the main dispatch loop (which may syncBack and mutate fields).
	if stmt.Background {
		bgCtx := *ctx                    // shallow copy — isolates scalar fields
		bgCtx.Cache = nil                // detach shared cache so preWarmCache writes stay local
		sw := &syncWriter{w: ctx.Output} // shared mutex-wrapped writer
		bgCtx.Output = sw                // background goroutine writes through sw
		syncCatalog := ctx.SyncCatalog   // capture callback before returning
		go func() {
			if err := buildCatalog(&bgCtx, full, source, stmt.Communities, stmt.Resolution); err != nil {
				fmt.Fprintf(bgCtx.Output, "Background catalog build failed: %v\n", err)
				return
			}
			// Propagate the built catalog back to the Executor so the next
			// command picks it up. syncBack has already run for this statement,
			// so we use the SyncCatalog callback to write directly to e.catalog.
			if bgCtx.Catalog != nil {
				if syncCatalog != nil {
					syncCatalog(bgCtx.Catalog)
				} else {
					bgCtx.Catalog.Close()
				}
			}
		}()
		fmt.Fprintln(sw, "Catalog build started in background...")
		return nil
	}

	// Refresh widget definitions alongside the catalog rebuild. Same staleness
	// detection as `mxcli widget init` — auto-fixes the case where an mxcli
	// upgrade introduced new .def.json fields (e.g. `objectLists`) that the
	// existing files don't have yet. Silent when everything is already
	// up to date; never fatal — catalog refresh is the primary intent.
	// Also regenerates the per-widget skill .md docs when defs changed, so
	// AI-assisted workflows see the latest properties / object lists.
	if ctx.MprPath != "" {
		if stats, err := RefreshWidgetDefinitions(ctx.MprPath, false, nil); err == nil {
			if stats.Extracted > 0 || stats.Refreshed > 0 {
				fmt.Fprintf(ctx.Output, "Widget definitions: %d new, %d refreshed\n",
					stats.Extracted, stats.Refreshed)
				if _, docErr := RegenerateWidgetDocs(ctx.MprPath); docErr != nil {
					fmt.Fprintf(ctx.Output, "Warning: widget doc regeneration failed: %v\n", docErr)
				}
			}
		}
	}

	// Rebuild the catalog
	return buildCatalog(ctx, full, source, stmt.Communities, stmt.Resolution)
}

// execRefreshCatalog handles REFRESH CATALOG [FULL] command (legacy signature).
func execRefreshCatalog(ctx *ExecContext, full bool) error {
	return execRefreshCatalogStmt(ctx, &ast.RefreshCatalogStmt{Full: full, Source: false})
}

// execShowCatalogStatus handles SHOW CATALOG STATUS command.
func execShowCatalogStatus(ctx *ExecContext) error {
	if ctx.Format == FormatJSON {
		return writeCatalogStatusJSON(ctx)
	}
	cachePath := getCachePath(ctx)
	if cachePath == "" {
		fmt.Fprintln(ctx.Output, "No project connected")
		return nil
	}

	fmt.Fprintf(ctx.Output, "\nCatalog Cache Status\n")
	fmt.Fprintf(ctx.Output, "────────────────────\n")
	fmt.Fprintf(ctx.Output, "Cache path: %s\n", cachePath)

	// Check if cache exists
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		fmt.Fprintln(ctx.Output, "Status: No cache file")
		return nil
	}

	// Load cache info
	cat, err := catalog.NewFromFile(cachePath)
	if err != nil {
		fmt.Fprintf(ctx.Output, "Status: Cache file corrupt (%v)\n", err)
		return nil
	}
	defer cat.Close()

	info, err := cat.GetCacheInfo()
	if err != nil {
		fmt.Fprintf(ctx.Output, "Status: Cannot read cache info (%v)\n", err)
		return nil
	}

	// Display cache info
	fmt.Fprintf(ctx.Output, "Build mode: %s\n", info.BuildMode)
	fmt.Fprintf(ctx.Output, "Build time: %s\n", info.BuildTime.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(ctx.Output, "Build duration: %s\n", info.BuildDuration)
	fmt.Fprintf(ctx.Output, "Mendix version: %s\n", info.MendixVersion)

	// Check validity
	valid, reason := isCacheValid(ctx, cachePath, "fast")
	if valid {
		fmt.Fprintln(ctx.Output, "Status: ✓ Valid")
	} else {
		fmt.Fprintf(ctx.Output, "Status: ✗ Invalid (%s)\n", reason)
	}

	// Also check full mode validity
	if info.BuildMode == "full" || info.BuildMode == "source" {
		fmt.Fprintln(ctx.Output, "Full mode: ✓ Available")
	} else {
		fmt.Fprintln(ctx.Output, "Full mode: ✗ Not cached (use refresh catalog full)")
	}
	if info.BuildMode == "source" {
		fmt.Fprintln(ctx.Output, "Source mode: ✓ Available")
	} else {
		fmt.Fprintln(ctx.Output, "Source mode: ✗ Not cached (use refresh catalog source)")
	}
	// Reported separately from the mode because it IS separate: the graph pass
	// augments whatever mode is cached, so "full" says nothing about whether
	// COMMUNITIES / GRAPH_CYCLES / GRAPH_LAYERS / GRAPH_CENTRALITY have anything
	// in them.
	if info.GraphAnalysis {
		fmt.Fprintf(ctx.Output, "Graph analysis: ✓ Available (resolution %g)\n", info.GraphResolution)
	} else {
		fmt.Fprintln(ctx.Output, "Graph analysis: ✗ Not run (use refresh catalog communities)")
	}

	return nil
}

// writeCatalogStatusJSON is SHOW CATALOG STATUS for --json: the same facts as
// the text report, as one object. The report is prose ("Status: ✓ Valid"), so
// under --json it used to be the only thing on stdout and did not parse.
func writeCatalogStatusJSON(ctx *ExecContext) error {
	cachePath := getCachePath(ctx)
	if cachePath == "" {
		return mdlerrors.NewNotConnected()
	}
	status := map[string]any{"cachePath": cachePath, "exists": false}
	if _, err := os.Stat(cachePath); err == nil {
		status["exists"] = true
		cat, err := catalog.NewFromFile(cachePath)
		if err != nil {
			return mdlerrors.NewBackend("open catalog cache", err)
		}
		defer cat.Close()
		info, err := cat.GetCacheInfo()
		if err != nil {
			return mdlerrors.NewBackend("read catalog cache info", err)
		}
		valid, reason := isCacheValid(ctx, cachePath, "fast")
		status["buildMode"] = info.BuildMode
		status["buildTime"] = info.BuildTime
		status["buildDuration"] = info.BuildDuration.String()
		status["mendixVersion"] = info.MendixVersion
		status["valid"] = valid
		if !valid {
			status["invalidReason"] = reason
		}
		status["fullMode"] = info.BuildMode == "full" || info.BuildMode == "source"
		status["sourceMode"] = info.BuildMode == "source"
		status["graphAnalysis"] = info.GraphAnalysis
		if info.GraphAnalysis {
			status["graphResolution"] = info.GraphResolution
		}
	}
	enc := json.NewEncoder(ctx.Output)
	enc.SetIndent("", "  ")
	return enc.Encode(status)
}

// catalogTableRefRe matches a `CATALOG.<name>` table reference (case-insensitive).
var catalogTableRefRe = regexp.MustCompile(`(?i)\bcatalog\.([a-z_][a-z0-9_]*)`)

// convertCatalogTableNames strips the CATALOG. prefix from table references so the
// query runs against the underlying SQLite views/tables. Generic by design: any
// CATALOG.<name> is rewritten, so new catalog views (e.g. the graph_* analysis
// views) work without maintaining a per-name allowlist. An unknown name simply
// yields SQLite's "no such table" rather than a silent passthrough.
func convertCatalogTableNames(query string) string {
	return catalogTableRefRe.ReplaceAllString(query, "$1")
}

// outputCatalogResults outputs query results in table or JSON format.
func outputCatalogResults(ctx *ExecContext, result *catalog.QueryResult) {
	tr := &TableResult{
		Columns: result.Columns,
	}
	for _, row := range result.Rows {
		outRow := make([]any, len(row))
		for i, val := range row {
			outRow[i] = formatValue(val)
		}
		tr.Rows = append(tr.Rows, outRow)
	}
	if err := writeResult(ctx, tr); err != nil {
		log.Printf("warning: failed to write catalog results: %v", err)
	}
}

// formatValue formats a value for display.
func formatValue(val any) string {
	if val == nil {
		return ""
	}
	s := fmt.Sprintf("%v", val)
	// Replace newlines with spaces for table display
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	// Collapse multiple spaces
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

// captureDescribe generates MDL source for a given object type and qualified name.
// It temporarily redirects ctx.Output to capture the describe output.
// NOT safe for concurrent use — use captureDescribeParallel instead.
func captureDescribe(ctx *ExecContext, objectType string, qualifiedName string) (string, error) {
	// Parse qualified name into ast.QualifiedName
	parts := strings.SplitN(qualifiedName, ".", 2)
	if len(parts) != 2 {
		return "", mdlerrors.NewValidationf("invalid qualified name: %s", qualifiedName)
	}
	qn := ast.QualifiedName{Module: parts[0], Name: parts[1]}

	// Save original output and redirect to buffer
	origOutput := ctx.Output
	var buf bytes.Buffer
	ctx.Output = &buf
	defer func() { ctx.Output = origOutput }()

	var err error
	switch strings.ToLower(objectType) {
	case "entity":
		err = describeEntity(ctx, qn)
	case "microflow", "nanoflow":
		err = describeMicroflow(ctx, qn)
	case "page":
		err = describePage(ctx, qn)
	case "snippet":
		err = describeSnippet(ctx, qn)
	case "enumeration":
		err = describeEnumeration(ctx, qn)
	case "workflow":
		err = describeWorkflow(ctx, qn)
	default:
		return "", mdlerrors.NewUnsupported("object type for describe: " + objectType)
	}

	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// captureDescribeParallel is a goroutine-safe version of captureDescribe.
// It creates a lightweight ExecContext clone per call with its own output buffer,
// sharing the backend and pre-warmed cache. Call preWarmCache() before using
// this from multiple goroutines.
func captureDescribeParallel(ctx *ExecContext, objectType string, qualifiedName string, id model.ID) (string, error) {
	parts := strings.SplitN(qualifiedName, ".", 2)
	if len(parts) != 2 {
		return "", mdlerrors.NewValidationf("invalid qualified name: %s", qualifiedName)
	}
	qn := ast.QualifiedName{Module: parts[0], Name: parts[1]}

	// Create a goroutine-local context: shared backend + cache, own output buffer.
	var buf bytes.Buffer
	localCtx := &ExecContext{
		Context: ctx.Context,
		Output:  &buf,
		Format:  ctx.Format,
		Quiet:   ctx.Quiet,
		Logger:  ctx.Logger,
		Backend: ctx.Backend,
		Cache:   ctx.Cache,
		MprPath: ctx.MprPath,
		// Pins the describe to this document rather than to whichever
		// document of this name the lookup prefers (#1185).
		describeID: id,
		// The text feeds the search index, where layout annotations do not
		// matter, and telling derived layout from authored rebuilds each flow
		// several times — it made a full source refresh 2.3x slower (#766).
		describeFullLayout: true,
	}

	describe := describeDispatch(objectType)
	if describe == nil {
		return "", mdlerrors.NewUnsupported("object type for describe: " + objectType)
	}

	if err := describe(localCtx, qn); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// describeDocFunc renders one document as MDL into ctx.Output.
type describeDocFunc func(ctx *ExecContext, name ast.QualifiedName) error

// describeDispatch maps a catalog source object type to its describe function,
// or nil when the type cannot be described.
//
// Kept as a table rather than a switch inside captureDescribeParallel so the
// mapping is assertable without a project — see
// TestDescribeDispatchCoversEverySourceObjectType, which checks it against
// catalog.SourceObjectTypes. #912 was this table drifting behind the catalog's
// collectors: nanoflows and rules were collected with no case here, and
// "nanoflow" was routed to describeMicroflow, which searches ListMicroflows()
// only and so returns NotFound for every nanoflow. Both failures were swallowed
// by the caller, so the types simply had no rows.
func describeDispatch(objectType string) describeDocFunc {
	switch strings.ToUpper(objectType) {
	case catalog.SourceEntity:
		return describeEntity
	case catalog.SourceMicroflow:
		return describeMicroflow
	case catalog.SourceNanoflow:
		return describeNanoflow
	case catalog.SourceRule:
		return describeRule
	case catalog.SourcePage:
		return describePage
	case catalog.SourceSnippet:
		return describeSnippet
	case catalog.SourceEnumeration:
		return describeEnumeration
	case catalog.SourceWorkflow:
		return describeWorkflow
	case catalog.SourceJsonStructure:
		return describeJsonStructure
	case catalog.SourceImportMapping:
		return describeImportMapping
	case catalog.SourceExportMapping:
		return describeExportMapping
	default:
		return nil
	}
}

// preWarmCache ensures all caches are populated before parallel operations.
// Must be called from the main goroutine before using captureDescribeParallel.
// This avoids O(n²) re-parsing in describe functions by building name lookup
// maps once and sharing them across all goroutines.
func preWarmCache(ctx *ExecContext) {
	h, _ := getHierarchy(ctx)
	if h == nil || ctx.Cache == nil {
		return
	}

	// Build entity name lookup
	ctx.Cache.entityNames = make(map[model.ID]string)
	dms, _ := ctx.Backend.ListDomainModels()
	for _, dm := range dms {
		modName := h.GetModuleName(dm.ContainerID)
		for _, ent := range dm.Entities {
			ctx.Cache.entityNames[ent.ID] = modName + "." + ent.Name
		}
	}

	// Build microflow name lookup
	ctx.Cache.microflowNames = make(map[model.ID]string)
	mfs, _ := ctx.Backend.ListMicroflows()
	for _, mf := range mfs {
		ctx.Cache.microflowNames[mf.ID] = h.GetQualifiedName(mf.ContainerID, mf.Name)
	}

	// Build page name lookup
	ctx.Cache.pageNames = make(map[model.ID]string)
	pgs, _ := ctx.Backend.ListPages()
	for _, pg := range pgs {
		ctx.Cache.pageNames[pg.ID] = h.GetQualifiedName(pg.ContainerID, pg.Name)
	}

	// Pre-warm the project default language so language-aware text selection in
	// (possibly parallel) describe is a race-free cached read (issue #702).
	describeDefaultLanguage(ctx)
}

// SearchOptions narrow or harden a search. The zero value is what the SEARCH
// statement uses.
type SearchOptions struct {
	// ExcludeMarketplace drops hits in modules downloaded from the Marketplace,
	// so an audit of the app's own text does not report a vendor's.
	ExcludeMarketplace bool
	// Strict returns the index's error for a query it cannot run (an FTS5 syntax
	// error, say) instead of reporting "no matches". A caller that gates on the
	// absence of hits needs the difference: "nothing found" and "could not look"
	// are not the same answer.
	Strict bool
}

// marketplaceModuleFilter is the WHERE fragment that keeps a search to modules
// the project owns. A module's Source is empty unless it came from the
// Marketplace (see the modules table).
//
// Only the current snapshot's modules count: the modules view holds a row per
// snapshot, and a module that came from the Marketplace in an older one would
// otherwise be left out of every later search. The current snapshot is the
// latest one recorded, as AddGraphAnalysis reads it.
const marketplaceModuleFilter = " and ModuleName not in (select Name from modules" +
	" where COALESCE(Source, '') <> ''" +
	" and SnapshotId = (select SnapshotId from snapshots order by rowid desc limit 1))"

// searchSQL returns the strings-index and source-index queries for a search.
func searchSQL(query string, opts SearchOptions) (stringsSQL, sourceSQL string) {
	filter := ""
	if opts.ExcludeMarketplace {
		filter = marketplaceModuleFilter
	}
	q := escapeFTSQuery(query)
	stringsSQL = fmt.Sprintf(
		"select QualifiedName, ObjectType, snippet(strings, 2, '>>>', '<<<', '...', 32) as Match, StringContext, ModuleName from strings where strings match '%s'%s limit 50",
		q, filter)
	sourceSQL = fmt.Sprintf(
		"select QualifiedName, ObjectType, snippet(source, 2, '>>>', '<<<', '...', 48) as Match, ModuleName from source where source match '%s'%s limit 50",
		q, filter)
	return stringsSQL, sourceSQL
}

// runSearchQuery runs one of the two search queries. An error is swallowed and
// reported as no rows unless opts.Strict is set — the long-standing behaviour
// of SEARCH, kept for it.
func runSearchQuery(ctx *ExecContext, sql string, opts SearchOptions) (*catalog.QueryResult, error) {
	res, err := ctx.Catalog.Query(sql)
	if err != nil {
		if opts.Strict {
			return nil, err
		}
		return nil, nil
	}
	return res, nil
}

// execSearch handles SEARCH 'query' command.
func execSearch(ctx *ExecContext, stmt *ast.SearchStmt) error {
	_, err := searchTable(ctx, stmt.Query, SearchOptions{})
	return err
}

// searchTable prints the human-readable result of a search and returns how many
// hits it printed.
func searchTable(ctx *ExecContext, query string, opts SearchOptions) (int, error) {
	if !ctx.Connected() {
		return 0, mdlerrors.NewNotConnected()
	}

	// Ensure catalog is built (at least full mode for strings table)
	if err := ensureCatalog(ctx, true); err != nil {
		return 0, err
	}

	stringsSQL, sourceSQL := searchSQL(query, opts)
	hits := 0

	// Search strings table
	strResult, err := runSearchQuery(ctx, stringsSQL, opts)
	if err != nil {
		return 0, err
	}
	if strResult != nil && strResult.Count > 0 {
		hits += strResult.Count
		fmt.Fprintf(ctx.Output, "\nString Matches (%d)\n", strResult.Count)
		fmt.Fprintln(ctx.Output, strings.Repeat("─", 40))
		outputCatalogResults(ctx, strResult)
	}

	// Search source table (if available)
	srcResult, err := runSearchQuery(ctx, sourceSQL, opts)
	if err != nil {
		return 0, err
	}
	if srcResult != nil && srcResult.Count > 0 {
		hits += srcResult.Count
		fmt.Fprintf(ctx.Output, "\nSource Matches (%d)\n", srcResult.Count)
		fmt.Fprintln(ctx.Output, strings.Repeat("─", 40))
		outputCatalogResults(ctx, srcResult)
	}

	if hits == 0 {
		fmt.Fprintln(ctx.Output, "No matches found.")
	}
	warnIfSourceIndexMissing(ctx)

	return hits, nil
}

// warnIfSourceIndexMissing says, on the diagnostics channel, that a search did
// not look at MDL source because CATALOG.SOURCE was never built. `search` needs
// only a full catalog, which has the strings index but not the source one, so
// without this the source half of every answer came back empty in silence —
// indistinguishable from "no microflow, page or expression mentions this".
//
// Keyed off the build mode the catalog records, never the row count: a
// source-mode catalog with no matching (or no) rows is a real answer, and
// counting rows would make "built, nothing matched" warn too.
func warnIfSourceIndexMissing(ctx *ExecContext) {
	mode := ""
	if ctx.Catalog != nil {
		if info, err := ctx.Catalog.GetCacheInfo(); err == nil {
			mode = info.BuildMode
		}
	}
	if catalogModeRank(mode) >= catalogModeRank("source") {
		return
	}
	if mode == "" {
		mode = "unknown"
	}
	w := ctx.diagnostics()
	fmt.Fprintf(w, "Warning: the source index (CATALOG.SOURCE) is not built (catalog mode: %s), so only string literals were searched — MDL source (microflow/nanoflow bodies, expressions, page and entity definitions) was not.\n", mode)
	if ctx.MprPath != "" {
		fmt.Fprintf(w, "Build it with: refresh catalog full source   (CLI: mxcli -p %q -c \"refresh catalog full source\")\n", ctx.MprPath)
	} else {
		fmt.Fprintln(w, "Build it with: refresh catalog full source")
	}
}

// escapeFTSQuery escapes special characters in FTS5 queries.
// FTS5 treats characters like '/', '.', '-' as token separators. To make
// queries like 'rest/companies' or 'Module.Entity' usable, we replace these
// with spaces (treated as AND between terms).
func escapeFTSQuery(q string) string {
	// Escape single quotes for SQL
	q = strings.ReplaceAll(q, "'", "''")
	// Replace common path/qualified-name separators with spaces so each segment
	// becomes a separate token that FTS5 ANDs together.
	for _, sep := range []string{"/", ".", "-", ":"} {
		q = strings.ReplaceAll(q, sep, " ")
	}
	return q
}

// search performs a full-text search with the specified output format.
// Format can be: "table" (default), "names" (just qualified names), "json"
func search(ctx *ExecContext, query, format string) error {
	_, err := searchWith(ctx, query, format, SearchOptions{})
	return err
}

// searchWith is search with options. It returns how many hits it found, which
// is the number a caller gating on "no match" needs.
func searchWith(ctx *ExecContext, query, format string, opts SearchOptions) (int, error) {
	if !ctx.Connected() {
		return 0, mdlerrors.NewNotConnected()
	}

	if format != "names" && format != "json" { // "table" (default)
		return searchTable(ctx, query, opts)
	}

	// Ensure catalog is built (at least full mode for strings table)
	if err := ensureCatalog(ctx, true); err != nil {
		return 0, err
	}

	type searchResult struct {
		QualifiedName string `json:"qualifiedName"`
		ObjectType    string `json:"objectType"`
		Match         string `json:"match,omitempty"`
		StringContext string `json:"stringContext,omitempty"`
		ModuleName    string `json:"moduleName"`
		Source        string `json:"source,omitempty"` // "strings" or "source"
	}

	var allResults []searchResult
	seen := make(map[string]bool) // For deduplicating names format

	stringsSQL, sourceSQL := searchSQL(query, opts)

	// Search strings table
	strResult, err := runSearchQuery(ctx, stringsSQL, opts)
	if err != nil {
		return 0, err
	}
	if strResult != nil && strResult.Count > 0 {
		for _, row := range strResult.Rows {
			r := searchResult{
				QualifiedName: fmt.Sprintf("%v", row[0]),
				ObjectType:    fmt.Sprintf("%v", row[1]),
				Match:         formatValue(row[2]),
				StringContext: fmt.Sprintf("%v", row[3]),
				ModuleName:    fmt.Sprintf("%v", row[4]),
				Source:        "strings",
			}
			allResults = append(allResults, r)
		}
	}

	// Search source table (if available)
	srcResult, err := runSearchQuery(ctx, sourceSQL, opts)
	if err != nil {
		return 0, err
	}
	if srcResult != nil && srcResult.Count > 0 {
		for _, row := range srcResult.Rows {
			r := searchResult{
				QualifiedName: fmt.Sprintf("%v", row[0]),
				ObjectType:    fmt.Sprintf("%v", row[1]),
				Match:         formatValue(row[2]),
				ModuleName:    fmt.Sprintf("%v", row[3]),
				Source:        "source",
			}
			allResults = append(allResults, r)
		}
	}

	if len(allResults) == 0 {
		if format != "json" {
			fmt.Fprintln(ctx.Output, "No matches found.")
		} else {
			fmt.Fprintln(ctx.Output, "[]")
		}
		warnIfSourceIndexMissing(ctx)
		return 0, nil
	}

	switch format {
	case "names":
		// Output just qualified names, one per line, deduplicated
		for _, r := range allResults {
			key := r.ObjectType + ":" + r.QualifiedName
			if !seen[key] {
				seen[key] = true
				fmt.Fprintf(ctx.Output, "%s\t%s\n", strings.ToLower(r.ObjectType), r.QualifiedName)
			}
		}
	case "json":
		jsonBytes, err := json.MarshalIndent(allResults, "", "  ")
		if err != nil {
			return 0, err
		}
		fmt.Fprintln(ctx.Output, string(jsonBytes))
	}

	warnIfSourceIndexMissing(ctx)
	return len(allResults), nil
}

// Search performs a full-text search with the specified output format.
func (e *Executor) Search(query, format string) error {
	_, err := e.SearchWith(query, format, SearchOptions{})
	return err
}

// SearchWith performs a full-text search and returns the number of hits it
// found (at most 50 per index), so a caller can act on "found something" without
// parsing the output it just printed.
func (e *Executor) SearchWith(query, format string, opts SearchOptions) (int, error) {
	return searchWith(e.newExecContext(context.Background()), query, format, opts)
}

// builtinWidgetMetas adapts the built-in WidgetRegistry's definitions into
// the catalog package's WidgetDefinitionMeta shape. Used by REFRESH CATALOG
// to populate the widget_definitions table alongside project widgets/.
func builtinWidgetMetas() []catalog.WidgetDefinitionMeta {
	reg, err := NewWidgetRegistry()
	if err != nil || reg == nil {
		return nil
	}
	defs := reg.All()
	if len(defs) == 0 {
		return nil
	}
	out := make([]catalog.WidgetDefinitionMeta, 0, len(defs))
	for _, def := range defs {
		meta := catalog.WidgetDefinitionMeta{
			WidgetId:    def.WidgetID,
			MdlName:     def.MDLName,
			DisplayName: def.MDLName,
			WidgetKind:  "builtin",
		}
		for _, pm := range def.PropertyMappings {
			meta.Properties = append(meta.Properties, catalog.WidgetDefinitionPropMeta{
				PropertyKey:  pm.PropertyKey,
				Type:         pm.Operation,
				DefaultValue: pm.Default,
			})
		}
		for _, cs := range def.ChildSlots {
			meta.ChildSlots = append(meta.ChildSlots, catalog.WidgetDefinitionSlotMeta{
				PropertyKey: cs.PropertyKey,
				MdlKeyword:  cs.MDLContainer,
			})
		}
		for _, ol := range def.ObjectLists {
			meta.ObjectLists = append(meta.ObjectLists, catalog.WidgetDefinitionSlotMeta{
				PropertyKey: ol.PropertyKey,
				MdlKeyword:  ol.MDLContainer,
			})
		}
		out = append(out, meta)
	}
	return out
}
