// SPDX-License-Identifier: Apache-2.0

// Package executor executes MDL AST statements against a Mendix project.
package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
	"github.com/JordtenBulte-OLC/mxcli/mdl/diaglog"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	sqllib "github.com/JordtenBulte-OLC/mxcli/sql"
)

// executorCache holds cached data for performance across multiple operations.
type executorCache struct {
	modules      []*model.Module
	units        []*types.UnitInfo
	folders      []*types.FolderInfo
	domainModels []*domainmodel.DomainModel
	hierarchy    *ContainerHierarchy
	// pages, layouts, microflows are cached separately as they may change during execution

	// Track items created during this session (not yet visible via reader)
	createdMicroflows map[string]*createdMicroflowInfo // qualifiedName -> info
	createdNanoflows  map[string]*createdNanoflowInfo  // qualifiedName -> info
	createdPages      map[string]*createdPageInfo      // qualifiedName -> info
	createdSnippets   map[string]*createdSnippetInfo   // qualifiedName -> info

	// Track items dropped during this session so that a subsequent
	// CREATE OR REPLACE/MODIFY with the same qualified name can reuse the
	// original UnitID and ContainerID. Studio Pro treats Unit rows with a
	// different UnitID (or the same UnitID under a different container) as
	// unrelated documents, producing broken projects on delete+insert
	// rewrites. Reusing both keeps the rewrite semantically equivalent to an
	// in-place update.
	droppedMicroflows map[string]*droppedUnitInfo // qualifiedName -> original IDs
	droppedNanoflows  map[string]*droppedUnitInfo // qualifiedName -> original IDs

	// Track domain models modified during this session for finalization
	modifiedDomainModels map[model.ID]string // domain model unit ID -> module name

	// Project default language for describe text selection (issue #702). Loaded
	// once (pre-warmed before any parallel widget extraction) so the getter is a
	// race-free read during describe.
	defaultLang       string
	defaultLangLoaded bool

	// Pre-warmed name lookup maps for parallel describe (goroutine-safe after init)
	entityNames    map[model.ID]string // entity ID -> "Module.EntityName"
	microflowNames map[model.ID]string // microflow ID -> "Module.MicroflowName"
	pageNames      map[model.ID]string // page ID -> "Module.PageName"
}

// createdMicroflowInfo tracks a microflow created during this session.
type createdMicroflowInfo struct {
	ID               model.ID
	Name             string
	ModuleName       string
	ContainerID      model.ID
	ReturnEntityName string // Qualified entity name from return type (e.g., "Module.Entity")
}

// createdNanoflowInfo tracks a nanoflow created during this session.
type createdNanoflowInfo struct {
	ID               model.ID
	Name             string
	ModuleName       string
	ContainerID      model.ID
	ReturnEntityName string
}

// createdPageInfo tracks a page created during this session.
type createdPageInfo struct {
	ID          model.ID
	Name        string
	ModuleName  string
	ContainerID model.ID
}

// createdSnippetInfo tracks a snippet created during this session.
type createdSnippetInfo struct {
	ID          model.ID
	Name        string
	ModuleName  string
	ContainerID model.ID
}

// droppedUnitInfo remembers the original UnitID and ContainerID of a document
// dropped during this session so that a subsequent CREATE OR REPLACE/MODIFY
// with the same qualified name can reuse them instead of generating new UUIDs.
type droppedUnitInfo struct {
	ID           model.ID
	ContainerID  model.ID
	AllowedRoles []model.ID
}

// getEntityNames returns the entity name lookup map, using the pre-warmed cache if available.
func getEntityNames(ctx *ExecContext, h *ContainerHierarchy) map[model.ID]string {
	if ctx.Cache != nil && len(ctx.Cache.entityNames) > 0 {
		return ctx.Cache.entityNames
	}
	entityNames := make(map[model.ID]string)
	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		if ctx.Logger != nil {
			ctx.Logger.Warn("getEntityNames: ListDomainModels failed", "error", err)
		}
		return entityNames
	}
	for _, dm := range dms {
		modName := h.GetModuleName(dm.ContainerID)
		for _, ent := range dm.Entities {
			entityNames[ent.ID] = modName + "." + ent.Name
		}
	}
	if ctx.Cache != nil {
		ctx.Cache.entityNames = entityNames
	}
	return entityNames
}

// getMicroflowNames returns the microflow name lookup map, using the pre-warmed cache if available.
func getMicroflowNames(ctx *ExecContext, h *ContainerHierarchy) map[model.ID]string {
	if ctx.Cache != nil && len(ctx.Cache.microflowNames) > 0 {
		return ctx.Cache.microflowNames
	}
	microflowNames := make(map[model.ID]string)
	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		if ctx.Logger != nil {
			ctx.Logger.Warn("getMicroflowNames: ListMicroflows failed", "error", err)
		}
		return microflowNames
	}
	for _, mf := range mfs {
		microflowNames[mf.ID] = h.GetQualifiedName(mf.ContainerID, mf.Name)
	}
	if ctx.Cache != nil {
		ctx.Cache.microflowNames = microflowNames
	}
	return microflowNames
}

// getPageNames returns the page name lookup map, using the pre-warmed cache if available.
func getPageNames(ctx *ExecContext, h *ContainerHierarchy) map[model.ID]string {
	if ctx.Cache != nil && len(ctx.Cache.pageNames) > 0 {
		return ctx.Cache.pageNames
	}
	pageNames := make(map[model.ID]string)
	pgs, err := ctx.Backend.ListPages()
	if err != nil {
		if ctx.Logger != nil {
			ctx.Logger.Warn("getPageNames: ListPages failed", "error", err)
		}
		return pageNames
	}
	for _, pg := range pgs {
		pageNames[pg.ID] = h.GetQualifiedName(pg.ContainerID, pg.Name)
	}
	if ctx.Cache != nil {
		ctx.Cache.pageNames = pageNames
	}
	return pageNames
}

const (
	// maxOutputLines is the per-statement line limit. Statements that produce more
	// lines than this are aborted to prevent runaway output from infinite loops.
	maxOutputLines = 10_000
	// defaultExecuteTimeout is the maximum wall-clock time allowed for a single
	// statement when MXCLI_EXEC_TIMEOUT is not set.
	defaultExecuteTimeout = 5 * time.Minute
)

// configuredExecuteTimeout returns the per-statement wall-clock timeout. The
// value is read from the MXCLI_EXEC_TIMEOUT environment variable on every call
// so long-running audits can opt into a higher ceiling without recompiling.
//
// Accepts either a Go duration ("12m", "2h30m") or a bare number of seconds
// ("900"). Falls back to defaultExecuteTimeout when the variable is unset,
// empty, or fails to parse.
func configuredExecuteTimeout() time.Duration {
	raw := os.Getenv("MXCLI_EXEC_TIMEOUT")
	if raw == "" {
		return defaultExecuteTimeout
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultExecuteTimeout
}

// effectiveExecuteTimeout returns the wall-clock timeout to apply to a single
// statement. An explicit MXCLI_EXEC_TIMEOUT always wins (so users can both raise
// and cap the limit). Otherwise, known bounded-but-slow statements — currently
// REFRESH CATALOG, which on a large project legitimately runs past the default
// 5m — are exempt from the guard (returns 0 = no wall-clock limit); the per-
// statement output-line guard still bounds runaway output. The guard exists to
// abort runaway loops, which a deterministic catalog build is not. See #651.
func effectiveExecuteTimeout(stmt ast.Statement) time.Duration {
	if os.Getenv("MXCLI_EXEC_TIMEOUT") != "" {
		return configuredExecuteTimeout()
	}
	if isExemptFromExecuteTimeout(stmt) {
		return 0
	}
	return defaultExecuteTimeout
}

// isExemptFromExecuteTimeout reports whether a statement is a known long-running
// but bounded operation that should not be capped by the default wall-clock guard.
func isExemptFromExecuteTimeout(stmt ast.Statement) bool {
	switch stmt.(type) {
	case *ast.RefreshCatalogStmt:
		return true
	default:
		return false
	}
}

// BackendFactory creates a new backend instance for connecting to a project.
type BackendFactory func() backend.FullBackend

// Executor executes MDL statements against a Mendix project.
type Executor struct {
	backend        backend.FullBackend // domain backend (populated on Connect)
	backendFactory BackendFactory      // factory for creating new backend instances
	output         io.Writer
	guard          *outputGuard // line-limit wrapper around output
	mprPath        string
	settings       map[string]any
	cache          *executorCache
	catalog        *catalog.Catalog
	quiet          bool                               // suppress connection and status messages
	format         OutputFormat                       // output format (table, json)
	logger         *diaglog.Logger                    // session diagnostics logger (nil = no logging)
	tracer         *backend.Tracer                    // MCP tool-call tracer (--mcp-trace; nil = off)
	fragments      map[string]*ast.DefineFragmentStmt // script-scoped fragment definitions
	sqlMgr         *sqllib.Manager                    // external SQL connection manager (lazy init)
	themeRegistry  *ThemeRegistry                     // cached theme design property definitions (lazy init)
	registry       *Registry                          // statement dispatch registry
	catalogMu      sync.RWMutex                       // protects catalog field from background goroutine writes
	catalogGen     uint64                             // monotonic generation counter for catalog swaps
}

// New creates a new executor with the given output writer.
func New(output io.Writer) *Executor {
	guard := newOutputGuard(output, maxOutputLines)
	return &Executor{
		output:   guard,
		guard:    guard,
		settings: make(map[string]any),
		registry: NewRegistry(),
	}
}

// SetBackendFactory sets the factory function used to create backend instances on Connect.
func (e *Executor) SetBackendFactory(f BackendFactory) {
	e.backendFactory = f
}

// SetQuiet enables or disables quiet mode (suppresses connection/status messages).
func (e *Executor) SetQuiet(quiet bool) {
	e.quiet = quiet
}

// SetFormat sets the output format (table or json).
func (e *Executor) SetFormat(f OutputFormat) {
	e.format = f
}

// SetLogger sets the diagnostics logger for session logging.
func (e *Executor) SetLogger(l *diaglog.Logger) {
	e.logger = l
}

// SetTracer attaches an MCP tool-call tracer. At level 2 the executor prints
// each MDL command before it runs, so the PED calls reported by the backend's
// client interleave underneath it. The same tracer must be given to the MCP
// backend (WithTracer) for the call lines to appear.
func (e *Executor) SetTracer(t *backend.Tracer) {
	e.tracer = t
}

// Execute runs a single MDL statement with output-line and wall-clock guards.
// Each statement gets a fresh line budget. If the statement exceeds maxOutputLines
// lines of output or runs longer than the configured timeout, it is aborted with an error.
func (e *Executor) Execute(stmt ast.Statement) error {
	start := time.Now()

	// Announce the MDL command so the PED calls it triggers (reported by the MCP
	// client) group under it (--mcp-trace, level 2; no-op otherwise).
	e.tracer.Statement(stmtSummary(stmt))

	// Reset per-statement line counter.
	if e.guard != nil {
		e.guard.reset()
	}

	// Enforce wall-clock timeout via context.WithTimeout.
	// The goroutine pattern is retained because handlers are not yet
	// context-aware; threading context through handlers is a follow-up.
	executeTimeout := effectiveExecuteTimeout(stmt)

	var err error
	if executeTimeout <= 0 {
		// No wall-clock guard for exempt statements (e.g. REFRESH CATALOG on a
		// large project). The output-line guard still bounds runaway output.
		err = e.executeInner(context.Background(), stmt)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), executeTimeout)
		defer cancel()

		type result struct{ err error }
		ch := make(chan result, 1)
		go func() {
			ch <- result{e.executeInner(ctx, stmt)}
		}()

		select {
		case r := <-ch:
			err = r.err
		case <-ctx.Done():
			err = mdlerrors.NewValidationf("statement timed out after %v (raise the limit with MXCLI_EXEC_TIMEOUT, e.g. MXCLI_EXEC_TIMEOUT=30m)", executeTimeout)
		}
	}

	if e.logger != nil {
		e.logger.Command(stmtTypeName(stmt), stmtSummary(stmt), time.Since(start), err)
	}
	return err
}

// ExecuteProgram runs all statements in a program.
func (e *Executor) ExecuteProgram(prog *ast.Program) error {
	// Collect all names defined in the script for forward-reference hints.
	allDefined := newScriptContext()
	allDefined.collectDefinitions(prog)

	// Track which names have been created so far.
	created := newScriptContext()

	for _, stmt := range prog.Statements {
		if err := e.Execute(stmt); err != nil {
			return annotateForwardRef(err, stmt, created, allDefined)
		}
		created.collectSingle(stmt)
	}
	return e.finalizeProgramExecution()
}

// finalizeProgramExecution runs post-execution reconciliation on modified domain models.
func (e *Executor) finalizeProgramExecution() error {
	if e.backend == nil || !e.backend.IsConnected() || e.cache == nil || len(e.cache.modifiedDomainModels) == 0 {
		return nil
	}

	for moduleID, moduleName := range e.cache.modifiedDomainModels {
		dm, err := e.backend.GetDomainModel(moduleID)
		if err != nil || dm == nil {
			continue // module was dropped, or has no domain model
		}

		count, err := e.backend.ReconcileMemberAccesses(dm.ID, moduleName)
		if err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("reconcile security for module %s", moduleName), err)
		}
		if count > 0 && !e.quiet {
			fmt.Fprintf(e.output, "Reconciled %d access rule(s) in module %s\n", count, moduleName)
		}
	}

	// Clear tracking
	e.cache.modifiedDomainModels = nil
	return nil
}

// Catalog returns the catalog, or nil if not built.
func (e *Executor) Catalog() *catalog.Catalog {
	e.catalogMu.RLock()
	c := e.catalog
	e.catalogMu.RUnlock()
	return c
}

// IsConnected returns true if connected to a project.
func (e *Executor) IsConnected() bool {
	return e.backend != nil && e.backend.IsConnected()
}

// Backend returns the full backend, or nil if not connected.
func (e *Executor) Backend() backend.FullBackend {
	if e.backend == nil || !e.backend.IsConnected() {
		return nil
	}
	return e.backend
}

// Close closes the connection to the project and all SQL connections.
func (e *Executor) Close() error {
	var closeErr error
	if e.backend != nil && e.backend.IsConnected() {
		closeErr = e.backend.Disconnect()
		e.backend = nil
	}
	if e.sqlMgr != nil {
		e.sqlMgr.CloseAll()
		e.sqlMgr = nil
	}
	return closeErr
}

// ----------------------------------------------------------------------------
// Cache and Tracking
// ----------------------------------------------------------------------------

// rememberDroppedMicroflow records the UnitID and ContainerID of a microflow
// that is about to be deleted via DROP MICROFLOW. A follow-up CREATE OR
// REPLACE/MODIFY for the same qualified name will reuse both instead of
// generating a fresh UUID and defaulting to the module root, so Studio Pro
// continues to see the unit as "updated in place" rather than a delete+insert
// pair.
func rememberDroppedMicroflow(ctx *ExecContext, qualifiedName string, id, containerID model.ID, allowedRoles []model.ID) {
	if ctx == nil || qualifiedName == "" || id == "" {
		return
	}
	if ctx.Cache == nil {
		ctx.Cache = &executorCache{}
	}
	if ctx.Cache.droppedMicroflows == nil {
		ctx.Cache.droppedMicroflows = make(map[string]*droppedUnitInfo)
	}
	ctx.Cache.droppedMicroflows[qualifiedName] = &droppedUnitInfo{
		ID:           id,
		ContainerID:  containerID,
		AllowedRoles: cloneRoleIDs(allowedRoles),
	}
}

// consumeDroppedMicroflow returns the original IDs of a microflow dropped
// earlier in this session (if any) and removes the entry so repeated CREATEs
// don't collide on the same ID. Returns nil when nothing was remembered.
func consumeDroppedMicroflow(ctx *ExecContext, qualifiedName string) *droppedUnitInfo {
	if ctx == nil || ctx.Cache == nil || ctx.Cache.droppedMicroflows == nil {
		return nil
	}
	info, ok := ctx.Cache.droppedMicroflows[qualifiedName]
	if !ok {
		return nil
	}
	delete(ctx.Cache.droppedMicroflows, qualifiedName)
	return info
}

// rememberDroppedNanoflow records the UnitID and ContainerID of a nanoflow
// that is about to be deleted so a subsequent CREATE OR REPLACE/MODIFY can reuse them.
func rememberDroppedNanoflow(ctx *ExecContext, qualifiedName string, id, containerID model.ID, allowedRoles []model.ID) {
	if ctx == nil || qualifiedName == "" || id == "" {
		return
	}
	if ctx.Cache == nil {
		ctx.Cache = &executorCache{}
	}
	if ctx.Cache.droppedNanoflows == nil {
		ctx.Cache.droppedNanoflows = make(map[string]*droppedUnitInfo)
	}
	ctx.Cache.droppedNanoflows[qualifiedName] = &droppedUnitInfo{
		ID:           id,
		ContainerID:  containerID,
		AllowedRoles: cloneRoleIDs(allowedRoles),
	}
}

// consumeDroppedNanoflow returns the original IDs of a nanoflow dropped
// earlier in this session (if any) and removes the entry.
func consumeDroppedNanoflow(ctx *ExecContext, qualifiedName string) *droppedUnitInfo {
	if ctx == nil || ctx.Cache == nil || ctx.Cache.droppedNanoflows == nil {
		return nil
	}
	info, ok := ctx.Cache.droppedNanoflows[qualifiedName]
	if !ok {
		return nil
	}
	delete(ctx.Cache.droppedNanoflows, qualifiedName)
	return info
}
