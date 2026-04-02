// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// execShowCatalogTables handles SHOW CATALOG TABLES.
func (e *Executor) execShowCatalogTables() error {
	// Build catalog if not already built (fast mode by default)
	if e.catalog == nil || !e.catalog.IsBuilt() {
		if err := e.ensureCatalog(false); err != nil {
			return err
		}
	}

	tables := e.catalog.Tables()
	fmt.Fprintf(e.output, "\nFound %d catalog table(s)\n", len(tables))

	// Get row counts for each table
	type tableInfo struct {
		name  string
		count int
	}
	infos := make([]tableInfo, 0, len(tables))

	// Calculate max table name width
	maxNameWidth := len("Table")
	for _, t := range tables {
		if len(t) > maxNameWidth {
			maxNameWidth = len(t)
		}
		// Get count for this table
		actualTable := strings.TrimPrefix(strings.ToLower(t), "catalog.")
		result, err := e.catalog.Query(fmt.Sprintf("SELECT COUNT(*) FROM %s", actualTable))
		count := 0
		if err == nil && len(result.Rows) > 0 {
			if v, ok := result.Rows[0][0].(int64); ok {
				count = int(v)
			}
		}
		infos = append(infos, tableInfo{name: t, count: count})
	}

	// Calculate max count width (minimum 5 for "Count" header)
	maxCountWidth := len("Count")
	for _, info := range infos {
		countStr := fmt.Sprintf("%d", info.count)
		if len(countStr) > maxCountWidth {
			maxCountWidth = len(countStr)
		}
	}

	// Output table with aligned columns
	fmt.Fprintf(e.output, "| %-*s | %*s |\n", maxNameWidth, "Table", maxCountWidth, "Count")
	fmt.Fprintf(e.output, "|-%s-|-%s-|\n", strings.Repeat("-", maxNameWidth), strings.Repeat("-", maxCountWidth))
	for _, info := range infos {
		fmt.Fprintf(e.output, "| %-*s | %*d |\n", maxNameWidth, info.name, maxCountWidth, info.count)
	}

	return nil
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
func (e *Executor) warnIfCatalogModeInsufficient(query string) {
	table := extractTableFromQuery(query)
	if table == "" {
		return
	}

	// Determine current build mode
	buildMode := "fast"
	if e.catalog != nil {
		if info, err := e.catalog.GetCacheInfo(); err == nil && info.BuildMode != "" {
			buildMode = info.BuildMode
		}
	}

	modeRank := map[string]int{"fast": 1, "full": 2, "source": 3}
	currentRank := modeRank[buildMode]

	if sourceOnlyTables[table] && currentRank < modeRank["source"] {
		fmt.Fprintf(e.output, "Warning: CATALOG.%s requires REFRESH CATALOG FULL SOURCE (current mode: %s)\n", strings.ToUpper(table), buildMode)
	} else if fullOnlyTables[table] && currentRank < modeRank["full"] {
		fmt.Fprintf(e.output, "Warning: CATALOG.%s requires REFRESH CATALOG FULL (current mode: %s)\n", strings.ToUpper(table), buildMode)
	}
}

// execCatalogQuery handles SELECT ... FROM CATALOG.xxx queries.
func (e *Executor) execCatalogQuery(query string) error {
	// Build catalog if not already built (fast mode by default)
	if e.catalog == nil || !e.catalog.IsBuilt() {
		if err := e.ensureCatalog(false); err != nil {
			return err
		}
	}

	// Convert CATALOG.xxx table names to actual table names
	query = convertCatalogTableNames(query)

	// Warn if querying a table that requires a higher build mode
	e.warnIfCatalogModeInsufficient(query)

	// Execute query
	result, err := e.catalog.Query(query)
	if err != nil {
		return fmt.Errorf("failed to execute catalog query\n%v", err)
	}

	// Output results
	fmt.Fprintf(e.output, "Found %d result(s)\n", result.Count)
	if result.Count == 0 {
		fmt.Fprintln(e.output, "(no results)")
		return nil
	}

	e.outputCatalogResults(result)
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
func (e *Executor) execDescribeCatalogTable(stmt *ast.DescribeCatalogTableStmt) error {
	// Build catalog if not already built (fast mode by default)
	if e.catalog == nil || !e.catalog.IsBuilt() {
		if err := e.ensureCatalog(false); err != nil {
			return err
		}
	}

	tableName := stmt.TableName

	// Query column info using PRAGMA
	result, err := e.catalog.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil || result.Count == 0 {
		return fmt.Errorf("unknown catalog table: CATALOG.%s", strings.ToUpper(tableName))
	}

	// Print table header
	fmt.Fprintf(e.output, "\nCATALOG.%s\n", strings.ToUpper(tableName))
	fmt.Fprintf(e.output, "Requires: %s\n\n", tableRequiredMode(tableName))

	// PRAGMA table_info returns: cid, name, type, notnull, dflt_value, pk
	// Build column table
	type colInfo struct {
		name string
		typ  string
		isPK bool
	}
	var cols []colInfo
	maxNameWidth := len("Column")
	maxTypeWidth := len("Type")

	for _, row := range result.Rows {
		name := fmt.Sprintf("%v", row[1])
		typ := fmt.Sprintf("%v", row[2])
		pk := false
		if v, ok := row[5].(int64); ok && v > 0 {
			pk = true
		}
		if len(name) > maxNameWidth {
			maxNameWidth = len(name)
		}
		if len(typ) > maxTypeWidth {
			maxTypeWidth = len(typ)
		}
		cols = append(cols, colInfo{name: name, typ: typ, isPK: pk})
	}

	pkWidth := 2 // "PK"
	fmt.Fprintf(e.output, "| %-*s | %-*s | %-*s |\n", maxNameWidth, "Column", maxTypeWidth, "Type", pkWidth, "PK")
	fmt.Fprintf(e.output, "|-%s-|-%s-|-%s-|\n", strings.Repeat("-", maxNameWidth), strings.Repeat("-", maxTypeWidth), strings.Repeat("-", pkWidth))
	for _, col := range cols {
		pkMark := "  "
		if col.isPK {
			pkMark = "* "
		}
		fmt.Fprintf(e.output, "| %-*s | %-*s | %s |\n", maxNameWidth, col.name, maxTypeWidth, col.typ, pkMark)
	}

	return nil
}

// ensureCatalog ensures a catalog is available, using cache if possible.
func (e *Executor) ensureCatalog(full bool) error {
	requiredMode := "fast"
	if full {
		requiredMode = "full"
	}

	// Try to load from cache first
	cachePath := e.getCachePath()
	if cachePath != "" {
		valid, _ := e.isCacheValid(cachePath, requiredMode)
		if valid {
			return e.loadCachedCatalog(cachePath)
		}
	}

	// Build fresh catalog
	return e.buildCatalog(full)
}

// getCachePath returns the path to the catalog cache file for the current project.
func (e *Executor) getCachePath() string {
	if e.mprPath == "" {
		return ""
	}
	dir := filepath.Dir(e.mprPath)
	cacheDir := filepath.Join(dir, ".mxcli")
	return filepath.Join(cacheDir, "catalog.db")
}

// getMprModTime returns the modification time of the current MPR file.
func (e *Executor) getMprModTime() time.Time {
	if e.mprPath == "" {
		return time.Time{}
	}
	info, err := os.Stat(e.mprPath)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// isCacheValid checks if the cached catalog is still valid.
func (e *Executor) isCacheValid(cachePath string, requiredMode string) (bool, string) {
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
	if info.MprPath != e.mprPath {
		return false, "MPR path changed"
	}

	// Check MPR modification time (compare Unix seconds to handle timezone/precision issues)
	currentModTime := e.getMprModTime()
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
func (e *Executor) loadCachedCatalog(cachePath string) error {
	cat, err := catalog.NewFromFile(cachePath)
	if err != nil {
		return err
	}

	info, err := cat.GetCacheInfo()
	if err != nil {
		cat.Close()
		return err
	}

	e.catalog = cat
	if !e.quiet {
		age := time.Since(info.BuildTime)
		fmt.Fprintf(e.output, "Loading cached catalog (built %s ago, %s mode)...\n",
			formatDuration(age), info.BuildMode)
		fmt.Fprintf(e.output, "✓ Catalog ready (from cache)\n")
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
func (e *Executor) buildCatalog(full bool, source ...bool) error {
	isSource := len(source) > 0 && source[0]
	if isSource {
		full = true // source implies full
	}

	if !e.quiet {
		if isSource {
			fmt.Fprintln(e.output, "Building catalog (source mode - includes MDL source)...")
		} else if full {
			fmt.Fprintln(e.output, "Building catalog (full mode - includes activities/widgets)...")
		} else {
			fmt.Fprintln(e.output, "Building catalog (fast mode)...")
		}
	}
	start := time.Now()

	// Create new catalog
	cat, err := catalog.New()
	if err != nil {
		return fmt.Errorf("failed to create catalog: %w", err)
	}

	// Set project metadata
	version, _ := e.reader.GetMendixVersion()
	cat.SetProject("default", "Current Project", version)

	// Build catalog
	builder := catalog.NewBuilder(cat, e.reader)
	builder.SetFullMode(full)
	if isSource {
		builder.SetSourceMode(true)
		e.PreWarmCache()
		builder.SetDescribeFunc(e.captureDescribeParallel)
	}
	err = builder.Build(func(table string, count int) {
		fmt.Fprintf(e.output, "✓ %s: %d\n", table, count)
	})
	if err != nil {
		cat.Close()
		return fmt.Errorf("failed to build catalog: %w", err)
	}

	elapsed := time.Since(start)

	// Save cache metadata
	buildMode := "fast"
	if isSource {
		buildMode = "source"
	} else if full {
		buildMode = "full"
	}
	cat.SetCacheInfo(e.mprPath, e.getMprModTime(), version, buildMode, elapsed)

	cat.SetBuilt(true)
	e.catalog = cat

	if !e.quiet {
		fmt.Fprintf(e.output, "✓ Catalog ready (%.1fs)\n", elapsed.Seconds())
	}

	// Save to cache file
	cachePath := e.getCachePath()
	if cachePath != "" {
		cacheDir := filepath.Dir(cachePath)
		if err := os.MkdirAll(cacheDir, 0755); err == nil {
			// Remove existing cache file first
			os.Remove(cachePath)
			if err := cat.SaveToFile(cachePath); err != nil {
				fmt.Fprintf(e.output, "Warning: failed to save catalog cache: %v\n", err)
			} else {
				fmt.Fprintf(e.output, "✓ Catalog cached to %s\n", cachePath)
			}
		}
	}

	return nil
}

// execRefreshCatalogStmt handles REFRESH CATALOG [FULL] [SOURCE] [FORCE] [BACKGROUND] command.
func (e *Executor) execRefreshCatalogStmt(stmt *ast.RefreshCatalogStmt) error {
	if e.reader == nil {
		return fmt.Errorf("not connected to a project")
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
		cachePath := e.getCachePath()
		if cachePath != "" {
			valid, reason := e.isCacheValid(cachePath, requiredMode)
			if valid {
				// Close existing catalog if any
				if e.catalog != nil {
					e.catalog.Close()
					e.catalog = nil
				}
				return e.loadCachedCatalog(cachePath)
			}
			if reason != "cache file does not exist" {
				fmt.Fprintf(e.output, "Cache invalid: %s\n", reason)
				// If project file was modified, reconnect to get fresh database connection
				if reason == "project file modified" {
					if err := e.reconnect(); err != nil {
						return fmt.Errorf("failed to reconnect after project modification: %w", err)
					}
				}
			}
		}
	}

	// Close existing catalog if any
	if e.catalog != nil {
		e.catalog.Close()
		e.catalog = nil
	}

	// Handle background mode
	if stmt.Background {
		go func() {
			if err := e.buildCatalog(stmt.Full, stmt.Source); err != nil {
				fmt.Fprintf(e.output, "Background catalog build failed: %v\n", err)
			}
		}()
		fmt.Fprintln(e.output, "Catalog build started in background...")
		return nil
	}

	// Rebuild the catalog
	return e.buildCatalog(stmt.Full, stmt.Source)
}

// execRefreshCatalog handles REFRESH CATALOG [FULL] command (legacy signature).
func (e *Executor) execRefreshCatalog(full bool) error {
	return e.execRefreshCatalogStmt(&ast.RefreshCatalogStmt{Full: full, Source: false})
}

// execShowCatalogStatus handles SHOW CATALOG STATUS command.
func (e *Executor) execShowCatalogStatus() error {
	cachePath := e.getCachePath()
	if cachePath == "" {
		fmt.Fprintln(e.output, "No project connected")
		return nil
	}

	fmt.Fprintf(e.output, "\nCatalog Cache Status\n")
	fmt.Fprintf(e.output, "────────────────────\n")
	fmt.Fprintf(e.output, "Cache path: %s\n", cachePath)

	// Check if cache exists
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		fmt.Fprintln(e.output, "Status: No cache file")
		return nil
	}

	// Load cache info
	cat, err := catalog.NewFromFile(cachePath)
	if err != nil {
		fmt.Fprintf(e.output, "Status: Cache file corrupt (%v)\n", err)
		return nil
	}
	defer cat.Close()

	info, err := cat.GetCacheInfo()
	if err != nil {
		fmt.Fprintf(e.output, "Status: Cannot read cache info (%v)\n", err)
		return nil
	}

	// Display cache info
	fmt.Fprintf(e.output, "Build mode: %s\n", info.BuildMode)
	fmt.Fprintf(e.output, "Build time: %s\n", info.BuildTime.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(e.output, "Build duration: %s\n", info.BuildDuration)
	fmt.Fprintf(e.output, "Mendix version: %s\n", info.MendixVersion)

	// Check validity
	valid, reason := e.isCacheValid(cachePath, "fast")
	if valid {
		fmt.Fprintln(e.output, "Status: ✓ Valid")
	} else {
		fmt.Fprintf(e.output, "Status: ✗ Invalid (%s)\n", reason)
	}

	// Also check full mode validity
	if info.BuildMode == "full" || info.BuildMode == "source" {
		fmt.Fprintln(e.output, "Full mode: ✓ Available")
	} else {
		fmt.Fprintln(e.output, "Full mode: ✗ Not cached (use REFRESH CATALOG FULL)")
	}
	if info.BuildMode == "source" {
		fmt.Fprintln(e.output, "Source mode: ✓ Available")
	} else {
		fmt.Fprintln(e.output, "Source mode: ✗ Not cached (use REFRESH CATALOG SOURCE)")
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

// outputCatalogResults outputs query results in table format.
func (e *Executor) outputCatalogResults(result *catalog.QueryResult) {
	// Calculate column widths
	widths := make([]int, len(result.Columns))
	for i, col := range result.Columns {
		widths[i] = len(col)
	}
	for _, row := range result.Rows {
		for i, val := range row {
			s := formatValue(val)
			if len(s) > widths[i] {
				widths[i] = len(s)
			}
		}
	}

	// Cap column widths at 50 characters
	for i := range widths {
		if widths[i] > 50 {
			widths[i] = 50
		}
	}

	// Print header
	fmt.Fprint(e.output, "|")
	for i, col := range result.Columns {
		fmt.Fprintf(e.output, " %-*s |", widths[i], truncate(col, widths[i]))
	}
	fmt.Fprintln(e.output)

	// Print separator
	fmt.Fprint(e.output, "|")
	for _, w := range widths {
		fmt.Fprintf(e.output, "-%s-|", strings.Repeat("-", w))
	}
	fmt.Fprintln(e.output)

	// Print rows
	for _, row := range result.Rows {
		fmt.Fprint(e.output, "|")
		for i, val := range row {
			s := formatValue(val)
			fmt.Fprintf(e.output, " %-*s |", widths[i], truncate(s, widths[i]))
		}
		fmt.Fprintln(e.output)
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

// truncate truncates a string to max length.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// captureDescribe generates MDL source for a given object type and qualified name.
// It temporarily redirects e.output to capture the describe output.
// NOT safe for concurrent use — use captureDescribeParallel instead.
func (e *Executor) captureDescribe(objectType string, qualifiedName string) (string, error) {
	// Parse qualified name into ast.QualifiedName
	parts := strings.SplitN(qualifiedName, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid qualified name: %s", qualifiedName)
	}
	qn := ast.QualifiedName{Module: parts[0], Name: parts[1]}

	// Save original output and redirect to buffer
	origOutput := e.output
	var buf bytes.Buffer
	e.output = &buf
	defer func() { e.output = origOutput }()

	var err error
	switch strings.ToUpper(objectType) {
	case "ENTITY":
		err = e.describeEntity(qn)
	case "MICROFLOW", "NANOFLOW":
		err = e.describeMicroflow(qn)
	case "PAGE":
		err = e.describePage(qn)
	case "SNIPPET":
		err = e.describeSnippet(qn)
	case "ENUMERATION":
		err = e.describeEnumeration(qn)
	case "WORKFLOW":
		err = e.describeWorkflow(qn)
	default:
		return "", fmt.Errorf("unsupported object type for describe: %s", objectType)
	}

	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// captureDescribeParallel is a goroutine-safe version of captureDescribe.
// It creates a lightweight executor clone per call with its own output buffer,
// sharing the reader and pre-warmed cache. Call PreWarmCache() before using
// this from multiple goroutines.
func (e *Executor) captureDescribeParallel(objectType string, qualifiedName string) (string, error) {
	parts := strings.SplitN(qualifiedName, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid qualified name: %s", qualifiedName)
	}
	qn := ast.QualifiedName{Module: parts[0], Name: parts[1]}

	// Create a goroutine-local executor: shared reader + cache, own output buffer
	var buf bytes.Buffer
	local := &Executor{
		reader: e.reader,
		output: &buf,
		cache:  e.cache,
	}

	var err error
	switch strings.ToUpper(objectType) {
	case "ENTITY":
		err = local.describeEntity(qn)
	case "MICROFLOW", "NANOFLOW":
		err = local.describeMicroflow(qn)
	case "PAGE":
		err = local.describePage(qn)
	case "SNIPPET":
		err = local.describeSnippet(qn)
	case "ENUMERATION":
		err = local.describeEnumeration(qn)
	case "WORKFLOW":
		err = local.describeWorkflow(qn)
	default:
		return "", fmt.Errorf("unsupported object type for describe: %s", objectType)
	}

	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// PreWarmCache ensures the hierarchy cache is populated before parallel operations.
// Must be called from the main goroutine before using captureDescribeParallel.
func (e *Executor) PreWarmCache() {
	e.getHierarchy()
}

// execSearch handles SEARCH 'query' command.
func (e *Executor) execSearch(stmt *ast.SearchStmt) error {
	if e.reader == nil {
		return fmt.Errorf("not connected to a project")
	}

	// Ensure catalog is built (at least full mode for strings table)
	if err := e.ensureCatalog(true); err != nil {
		return err
	}

	query := stmt.Query
	found := false

	// Search strings table
	stringsQuery := fmt.Sprintf(
		"SELECT QualifiedName, ObjectType, snippet(strings, 2, '>>>', '<<<', '...', 32) AS Match, StringContext, ModuleName FROM strings WHERE strings MATCH '%s' LIMIT 50",
		escapeFTSQuery(query))
	strResult, err := e.catalog.Query(stringsQuery)
	if err == nil && strResult.Count > 0 {
		found = true
		fmt.Fprintf(e.output, "\nString Matches (%d)\n", strResult.Count)
		fmt.Fprintln(e.output, strings.Repeat("─", 40))
		e.outputCatalogResults(strResult)
	}

	// Search source table (if available)
	sourceQuery := fmt.Sprintf(
		"SELECT QualifiedName, ObjectType, snippet(source, 2, '>>>', '<<<', '...', 48) AS Match, ModuleName FROM source WHERE source MATCH '%s' LIMIT 50",
		escapeFTSQuery(query))
	srcResult, err := e.catalog.Query(sourceQuery)
	if err == nil && srcResult.Count > 0 {
		found = true
		fmt.Fprintf(e.output, "\nSource Matches (%d)\n", srcResult.Count)
		fmt.Fprintln(e.output, strings.Repeat("─", 40))
		e.outputCatalogResults(srcResult)
	}

	if !found {
		fmt.Fprintln(e.output, "No matches found.")
		fmt.Fprintln(e.output, "Tip: Use REFRESH CATALOG SOURCE to enable source-level search.")
	}

	return nil
}

// escapeFTSQuery escapes special characters in FTS5 queries.
func escapeFTSQuery(q string) string {
	// Escape single quotes for SQL
	return strings.ReplaceAll(q, "'", "''")
}

// Search performs a full-text search with the specified output format.
// Format can be: "table" (default), "names" (just qualified names), "json"
func (e *Executor) Search(query, format string) error {
	if e.reader == nil {
		return fmt.Errorf("not connected to a project")
	}

	// Ensure catalog is built (at least full mode for strings table)
	if err := e.ensureCatalog(true); err != nil {
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
	strResult, err := e.catalog.Query(stringsQuery)
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
	srcResult, err := e.catalog.Query(sourceQuery)
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
			fmt.Fprintln(e.output, "No matches found.")
			fmt.Fprintln(e.output, "Tip: Use REFRESH CATALOG SOURCE to enable source-level search.")
		} else {
			fmt.Fprintln(e.output, "[]")
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
				fmt.Fprintf(e.output, "%s\t%s\n", strings.ToLower(r.ObjectType), r.QualifiedName)
			}
		}
	case "json":
		jsonBytes, err := json.MarshalIndent(allResults, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(e.output, string(jsonBytes))
	default: // "table"
		return e.execSearch(&ast.SearchStmt{Query: query})
	}

	return nil
}
