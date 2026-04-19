// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// execShowCatalogTables handles SHOW CATALOG TABLES.
func execShowCatalogTables(ctx *ExecContext) error {
	// Build catalog if not already built (fast mode by default)
	if ctx.Catalog == nil || !ctx.Catalog.IsBuilt() {
		if err := ensureCatalog(ctx, false); err != nil {
			return err
		}
	}

	tables := ctx.Catalog.Tables()
	fmt.Fprintf(ctx.Output, "\nFound %d catalog table(s)\n", len(tables))

	// Get row counts for each table
	type tableInfo struct {
		name  string
		count int
	}
	infos := make([]tableInfo, 0, len(tables))

	for _, t := range tables {
		// Get count for this table
		actualTable := strings.TrimPrefix(strings.ToLower(t), "catalog.")
		result, err := ctx.Catalog.Query(fmt.Sprintf("SELECT COUNT(*) FROM %s", actualTable))
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
	"widgets":     true,
	"activities":  true,
	"refs":        true,
	"strings":     true,
	"permissions": true,
}

// sourceOnlyTables are catalog tables only populated by REFRESH CATALOG FULL SOURCE.
var sourceOnlyTables = map[string]bool{
	"source": true,
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

	// Determine current build mode
	buildMode := "fast"
	if ctx.Catalog != nil {
		if info, err := ctx.Catalog.GetCacheInfo(); err == nil && info.BuildMode != "" {
			buildMode = info.BuildMode
		}
	}

	modeRank := map[string]int{"fast": 1, "full": 2, "source": 3}
	currentRank := modeRank[buildMode]

	if sourceOnlyTables[table] && currentRank < modeRank["source"] {
		fmt.Fprintf(ctx.Output, "Warning: CATALOG.%s requires REFRESH CATALOG FULL SOURCE (current mode: %s)\n", strings.ToUpper(table), buildMode)
	} else if fullOnlyTables[table] && currentRank < modeRank["full"] {
		fmt.Fprintf(ctx.Output, "Warning: CATALOG.%s requires REFRESH CATALOG FULL (current mode: %s)\n", strings.ToUpper(table), buildMode)
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
	fmt.Fprintf(ctx.Output, "Found %d result(s)\n", result.Count)
	if result.Count == 0 {
		fmt.Fprintln(ctx.Output, "(no results)")
		return nil
	}

	outputCatalogResults(ctx, result)
	return nil
}

// tableRequiredMode returns the minimum catalog build mode for a table.
func tableRequiredMode(table string) string {
	if sourceOnlyTables[table] {
		return "REFRESH CATALOG FULL SOURCE"
	}
	if fullOnlyTables[table] {
		return "REFRESH CATALOG FULL"
	}
	return "REFRESH CATALOG"
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

	// Print table header
	fmt.Fprintf(ctx.Output, "\nCATALOG.%s\n", strings.ToUpper(tableName))
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

	// Build fresh catalog
	return buildCatalog(ctx, full)
}

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
	modeRank := map[string]int{"fast": 1, "full": 2, "source": 3}
	cachedRank := modeRank[info.BuildMode]
	requiredRank := modeRank[requiredMode]
	if requiredRank > cachedRank {
		return false, fmt.Sprintf("%s mode requested but cache is %s mode", requiredMode, info.BuildMode)
	}

	return true, ""
}

// loadCachedCatalog loads a catalog from the cache file.
func loadCachedCatalog(ctx *ExecContext, cachePath string) error {
	e := ctx.executor
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
	e.catalog = cat
	if !ctx.Quiet {
		age := time.Since(info.BuildTime)
		fmt.Fprintf(ctx.Output, "Loading cached catalog (built %s ago, %s mode)...\n",
			formatDuration(age), info.BuildMode)
		fmt.Fprintf(ctx.Output, "✓ Catalog ready (from cache)\n")
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

// buildCatalog builds the catalog from the project.
func buildCatalog(ctx *ExecContext, full bool, source ...bool) error {
	e := ctx.executor
	isSource := len(source) > 0 && source[0]
	if isSource {
		full = true // source implies full
	}

	if !ctx.Quiet {
		if isSource {
			fmt.Fprintln(ctx.Output, "Building catalog (source mode - includes MDL source)...")
		} else if full {
			fmt.Fprintln(ctx.Output, "Building catalog (full mode - includes activities/widgets)...")
		} else {
			fmt.Fprintln(ctx.Output, "Building catalog (fast mode)...")
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
	if isSource {
		builder.SetSourceMode(true)
		preWarmCache(ctx)
		builder.SetDescribeFunc(func(objectType string, qualifiedName string) (string, error) {
			return captureDescribeParallel(ctx, objectType, qualifiedName)
		})
	}
	err = builder.Build(func(table string, count int) {
		fmt.Fprintf(ctx.Output, "✓ %s: %d\n", table, count)
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
	e.catalog = cat

	if !ctx.Quiet {
		fmt.Fprintf(ctx.Output, "✓ Catalog ready (%.1fs)\n", elapsed.Seconds())
	}

	// Save to cache file
	cachePath := getCachePath(ctx)
	if cachePath != "" {
		cacheDir := filepath.Dir(cachePath)
		if err := os.MkdirAll(cacheDir, 0755); err == nil {
			// Remove existing cache file first
			os.Remove(cachePath)
			if err := cat.SaveToFile(cachePath); err != nil {
				fmt.Fprintf(ctx.Output, "Warning: failed to save catalog cache: %v\n", err)
			} else {
				fmt.Fprintf(ctx.Output, "✓ Catalog cached to %s\n", cachePath)
			}
		}
	}

	return nil
}

// execRefreshCatalogStmt handles REFRESH CATALOG [FULL] [SOURCE] [FORCE] [BACKGROUND] command.
func execRefreshCatalogStmt(ctx *ExecContext, stmt *ast.RefreshCatalogStmt) error {
	e := ctx.executor
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	requiredMode := "fast"
	if stmt.Full {
		requiredMode = "full"
	}
	if stmt.Source {
		requiredMode = "source"
	}

	// Check cache unless FORCE is specified
	if !stmt.Force {
		cachePath := getCachePath(ctx)
		if cachePath != "" {
			valid, reason := isCacheValid(ctx, cachePath, requiredMode)
			if valid {
				// Close existing catalog if any
				if ctx.Catalog != nil {
					ctx.Catalog.Close()
					ctx.Catalog = nil
					e.catalog = nil
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

	// Close existing catalog if any
	if ctx.Catalog != nil {
		ctx.Catalog.Close()
		ctx.Catalog = nil
		e.catalog = nil
	}

	// Handle background mode
	if stmt.Background {
		go func() {
			if err := buildCatalog(ctx, stmt.Full, stmt.Source); err != nil {
				fmt.Fprintf(ctx.Output, "Background catalog build failed: %v\n", err)
			}
		}()
		fmt.Fprintln(ctx.Output, "Catalog build started in background...")
		return nil
	}

	// Rebuild the catalog
	return buildCatalog(ctx, stmt.Full, stmt.Source)
}

// execRefreshCatalog handles REFRESH CATALOG [FULL] command (legacy signature).
func execRefreshCatalog(ctx *ExecContext, full bool) error {
	return execRefreshCatalogStmt(ctx, &ast.RefreshCatalogStmt{Full: full, Source: false})
}

// execShowCatalogStatus handles SHOW CATALOG STATUS command.
func execShowCatalogStatus(ctx *ExecContext) error {
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
		fmt.Fprintln(ctx.Output, "Full mode: ✗ Not cached (use REFRESH CATALOG FULL)")
	}
	if info.BuildMode == "source" {
		fmt.Fprintln(ctx.Output, "Source mode: ✓ Available")
	} else {
		fmt.Fprintln(ctx.Output, "Source mode: ✗ Not cached (use REFRESH CATALOG SOURCE)")
	}

	return nil
}

// convertCatalogTableNames converts CATALOG.xxx to actual table names.
func convertCatalogTableNames(query string) string {
	// Case-insensitive replacement
	replacements := map[string]string{
		"catalog.modules":                   "modules",
		"catalog.entities":                  "entities",
		"catalog.attributes":                "attributes",
		"catalog.microflows":                "microflows",
		"catalog.nanoflows":                 "nanoflows",
		"catalog.pages":                     "pages",
		"catalog.snippets":                  "snippets",
		"catalog.layouts":                   "layouts",
		"catalog.enumerations":              "enumerations",
		"catalog.activities":                "activities",
		"catalog.widgets":                   "widgets",
		"catalog.xpath_expressions":         "xpath_expressions",
		"catalog.refs":                      "refs",
		"catalog.permissions":               "permissions",
		"catalog.projects":                  "projects",
		"catalog.snapshots":                 "snapshots",
		"catalog.objects":                   "objects",
		"catalog.strings":                   "strings",
		"catalog.source":                    "source",
		"catalog.workflows":                 "workflows",
		"catalog.odata_clients":             "odata_clients",
		"catalog.odata_services":            "odata_services",
		"catalog.business_event_services":   "business_event_services",
		"catalog.rest_clients":              "rest_clients",
		"catalog.rest_operations":           "rest_operations",
		"catalog.published_rest_services":   "published_rest_services",
		"catalog.published_rest_operations": "published_rest_operations",
		"catalog.external_entities":         "external_entities",
		"catalog.external_actions":          "external_actions",
		"catalog.business_events":           "business_events",
		"catalog.database_connections":      "database_connections",
		"catalog.contract_entities":         "contract_entities",
		"catalog.contract_actions":          "contract_actions",
		"catalog.contract_messages":         "contract_messages",
		"catalog.json_structures":           "json_structures",
		"catalog.import_mappings":           "import_mappings",
		"catalog.export_mappings":           "export_mappings",
	}

	result := query
	for from, to := range replacements {
		// Case-insensitive replace
		result = replaceIgnoreCase(result, from, to)
	}
	return result
}

// replaceIgnoreCase replaces all occurrences of old with new, ignoring case.
func replaceIgnoreCase(s, old, new string) string {
	lower := strings.ToLower(s)
	oldLower := strings.ToLower(old)

	var result strings.Builder
	i := 0
	for {
		idx := strings.Index(lower[i:], oldLower)
		if idx == -1 {
			result.WriteString(s[i:])
			break
		}
		result.WriteString(s[i : i+idx])
		result.WriteString(new)
		i += idx + len(old)
	}
	return result.String()
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
	_ = writeResult(ctx, tr)
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
	switch strings.ToUpper(objectType) {
	case "ENTITY":
		err = describeEntity(ctx, qn)
	case "MICROFLOW", "NANOFLOW":
		err = describeMicroflow(ctx, qn)
	case "PAGE":
		err = describePage(ctx, qn)
	case "SNIPPET":
		err = describeSnippet(ctx, qn)
	case "ENUMERATION":
		err = describeEnumeration(ctx, qn)
	case "WORKFLOW":
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
// sharing the reader and pre-warmed cache. Call preWarmCache() before using
// this from multiple goroutines.
func captureDescribeParallel(ctx *ExecContext, objectType string, qualifiedName string) (string, error) {
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
	}
	// If a backing Executor exists, create a local one for handlers that still
	// need e.backend/e.output (e.g., describeMicroflow via writeDescribeJSON).
	if ctx.executor != nil {
		local := &Executor{
			backend: ctx.executor.backend,
			output:  &buf,
			cache:   ctx.Cache,
		}
		localCtx.executor = local
	}

	var err error
	switch strings.ToUpper(objectType) {
	case "ENTITY":
		err = describeEntity(localCtx, qn)
	case "MICROFLOW", "NANOFLOW":
		err = describeMicroflow(localCtx, qn)
	case "PAGE":
		err = describePage(localCtx, qn)
	case "SNIPPET":
		err = describeSnippet(localCtx, qn)
	case "ENUMERATION":
		err = describeEnumeration(localCtx, qn)
	case "WORKFLOW":
		err = describeWorkflow(localCtx, qn)
	default:
		return "", mdlerrors.NewUnsupported("object type for describe: " + objectType)
	}

	if err != nil {
		return "", err
	}
	return buf.String(), nil
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
}

// execSearch handles SEARCH 'query' command.
func execSearch(ctx *ExecContext, stmt *ast.SearchStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Ensure catalog is built (at least full mode for strings table)
	if err := ensureCatalog(ctx, true); err != nil {
		return err
	}

	query := stmt.Query
	found := false

	// Search strings table
	stringsQuery := fmt.Sprintf(
		"SELECT QualifiedName, ObjectType, snippet(strings, 2, '>>>', '<<<', '...', 32) AS Match, StringContext, ModuleName FROM strings WHERE strings MATCH '%s' LIMIT 50",
		escapeFTSQuery(query))
	strResult, err := ctx.Catalog.Query(stringsQuery)
	if err == nil && strResult.Count > 0 {
		found = true
		fmt.Fprintf(ctx.Output, "\nString Matches (%d)\n", strResult.Count)
		fmt.Fprintln(ctx.Output, strings.Repeat("─", 40))
		outputCatalogResults(ctx, strResult)
	}

	// Search source table (if available)
	sourceQuery := fmt.Sprintf(
		"SELECT QualifiedName, ObjectType, snippet(source, 2, '>>>', '<<<', '...', 48) AS Match, ModuleName FROM source WHERE source MATCH '%s' LIMIT 50",
		escapeFTSQuery(query))
	srcResult, err := ctx.Catalog.Query(sourceQuery)
	if err == nil && srcResult.Count > 0 {
		found = true
		fmt.Fprintf(ctx.Output, "\nSource Matches (%d)\n", srcResult.Count)
		fmt.Fprintln(ctx.Output, strings.Repeat("─", 40))
		outputCatalogResults(ctx, srcResult)
	}

	if !found {
		fmt.Fprintln(ctx.Output, "No matches found.")
		fmt.Fprintln(ctx.Output, "Tip: Use REFRESH CATALOG SOURCE to enable source-level search.")
	}

	return nil
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
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Ensure catalog is built (at least full mode for strings table)
	if err := ensureCatalog(ctx, true); err != nil {
		return err
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

	// Search strings table
	stringsQuery := fmt.Sprintf(
		"SELECT QualifiedName, ObjectType, snippet(strings, 2, '>>>', '<<<', '...', 32) AS Match, StringContext, ModuleName FROM strings WHERE strings MATCH '%s' LIMIT 50",
		escapeFTSQuery(query))
	strResult, err := ctx.Catalog.Query(stringsQuery)
	if err == nil && strResult.Count > 0 {
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
	sourceQuery := fmt.Sprintf(
		"SELECT QualifiedName, ObjectType, snippet(source, 2, '>>>', '<<<', '...', 48) AS Match, ModuleName FROM source WHERE source MATCH '%s' LIMIT 50",
		escapeFTSQuery(query))
	srcResult, err := ctx.Catalog.Query(sourceQuery)
	if err == nil && srcResult.Count > 0 {
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
			fmt.Fprintln(ctx.Output, "Tip: Use REFRESH CATALOG SOURCE to enable source-level search.")
		} else {
			fmt.Fprintln(ctx.Output, "[]")
		}
		return nil
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
			return err
		}
		fmt.Fprintln(ctx.Output, string(jsonBytes))
	default: // "table"
		return execSearch(ctx, &ast.SearchStmt{Query: query})
	}

	return nil
}

// Search performs a full-text search with the specified output format.
func (e *Executor) Search(query, format string) error {
	return search(e.newExecContext(context.Background()), query, format)
}
