// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"io"
	"path/filepath"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
	"github.com/JordtenBulte-OLC/mxcli/mdl/diaglog"
	"github.com/JordtenBulte-OLC/mxcli/model"
	sqllib "github.com/JordtenBulte-OLC/mxcli/sql"
)

// ExecContext carries all dependencies a statement handler needs.
//
// Design notes:
//   - Embeds context.Context for cancellation and timeout propagation.
//   - Holds a FullBackend for domain operations (handlers narrow to
//     the sub-interface they need via type assertion or accessor).
//   - Ancillary fields (output, format, cache, etc.) are lifted from
//     the Executor struct so handlers don't depend on *Executor.
type ExecContext struct {
	context.Context

	// Backend provides all domain operations (read/write/connect).
	// Nil when not connected.
	Backend backend.FullBackend

	// Output is the writer for user-visible output (with line-limit guard).
	Output io.Writer

	// Format controls output formatting (table, json, etc.).
	Format OutputFormat

	// Quiet suppresses connection and status messages.
	Quiet bool

	// Logger is the session diagnostics logger (nil = no logging).
	Logger *diaglog.Logger

	// Fragments holds script-scoped fragment definitions.
	Fragments map[string]*ast.DefineFragmentStmt

	// Catalog provides MDL name resolution.
	Catalog *catalog.Catalog

	// Cache holds per-session cached data for performance.
	Cache *executorCache

	// MprPath is the filesystem path to the connected .mpr file.
	// Empty when not connected.
	MprPath string

	// SqlMgr manages external SQL database connections (lazy init).
	SqlMgr *sqllib.Manager

	// ThemeRegistry holds cached theme design property definitions (lazy init).
	ThemeRegistry *ThemeRegistry

	// Settings holds session-scoped key-value settings (SET command).
	Settings map[string]any

	// BackendFactory creates new backend instances (used by connect/reconnect).
	BackendFactory BackendFactory

	// ExecuteFn dispatches a single statement through the Executor's full
	// pipeline (line-limit reset, wall-clock timeout, logging). Set by
	// newExecContext; used by script execution and generated-MDL dispatch.
	ExecuteFn func(ast.Statement) error

	// ExecuteProgramFn dispatches a full program (all statements + finalization).
	ExecuteProgramFn func(*ast.Program) error

	// FinalizeFn runs post-execution reconciliation (security rule sync).
	FinalizeFn func() error

	// SyncCatalog propagates an asynchronously built catalog back to the
	// Executor. Used by REFRESH CATALOG BACKGROUND so the goroutine can
	// deliver the result after syncBack has already run.
	SyncCatalog func(*catalog.Catalog)

	// DescribingMicroflowHasReturnValue is set while rendering a microflow body.
	// It lets activity formatting distinguish a terminal void EndEvent from an
	// empty EndEvent in a value-returning microflow, where bare `return;` is invalid.
	DescribingMicroflowHasReturnValue bool

	// ScriptDepth tracks the current EXECUTE SCRIPT nesting level.
	// Incremented on each recursive call; execExecuteScript rejects calls
	// that exceed maxScriptDepth to prevent infinite self-referencing scripts.
	ScriptDepth int
}

// Connected returns true if a project is connected via the Backend.
func (ctx *ExecContext) Connected() bool {
	return ctx.Backend != nil && ctx.Backend.IsConnected()
}

// ConnectedForWrite returns true if a project is connected and the backend
// supports write operations. Currently equivalent to Connected() since
// MprBackend always supports writes.
func (ctx *ExecContext) ConnectedForWrite() bool {
	return ctx.Connected()
}

// InvalidateCache clears the hierarchy/entity/microflow cache so that
// subsequent statements see fresh data.
func (ctx *ExecContext) InvalidateCache() {
	ctx.Cache = nil
}

// GetThemeRegistry returns the cached theme registry, loading it lazily
// from the project's theme sources on first access.
func (ctx *ExecContext) GetThemeRegistry() *ThemeRegistry {
	if ctx.ThemeRegistry != nil {
		return ctx.ThemeRegistry
	}
	if ctx.MprPath == "" {
		return nil
	}
	projectDir := filepath.Dir(ctx.MprPath)
	registry, err := loadThemeRegistry(projectDir)
	if err != nil {
		if ctx.Logger != nil {
			ctx.Logger.Warn("failed to load theme registry", "error", err)
		}
		return nil
	}
	ctx.ThemeRegistry = registry
	return ctx.ThemeRegistry
}

// ensureCache initializes the ExecContext cache if nil.
func (ctx *ExecContext) ensureCache() {
	if ctx.Cache == nil {
		ctx.Cache = &executorCache{}
	}
}

// trackModifiedDomainModel records a domain model that was modified during
// execution, so it can be reconciled at the end of the program.
func (ctx *ExecContext) trackModifiedDomainModel(moduleID model.ID, moduleName string) {
	if ctx.Backend == nil || !ctx.Backend.IsConnected() {
		return
	}
	ctx.ensureCache()
	if ctx.Cache.modifiedDomainModels == nil {
		ctx.Cache.modifiedDomainModels = make(map[model.ID]string)
	}
	ctx.Cache.modifiedDomainModels[moduleID] = moduleName
}

// trackCreatedMicroflow registers a microflow created during this session.
func (ctx *ExecContext) trackCreatedMicroflow(moduleName, mfName string, id, containerID model.ID, returnEntityName string) {
	ctx.ensureCache()
	if ctx.Cache.createdMicroflows == nil {
		ctx.Cache.createdMicroflows = make(map[string]*createdMicroflowInfo)
	}
	qualifiedName := moduleName + "." + mfName
	ctx.Cache.createdMicroflows[qualifiedName] = &createdMicroflowInfo{
		ID:               id,
		Name:             mfName,
		ModuleName:       moduleName,
		ContainerID:      containerID,
		ReturnEntityName: returnEntityName,
	}
}

// trackCreatedNanoflow registers a nanoflow created during this session.
// The cache is consumed by execDropNanoflow (cleanup on DROP) and will be
// used by future resolvers for session-local nanoflow lookups (matching
// the createdMicroflows pattern).
func (ctx *ExecContext) trackCreatedNanoflow(moduleName, nfName string, id, containerID model.ID, returnEntityName string) {
	ctx.ensureCache()
	if ctx.Cache.createdNanoflows == nil {
		ctx.Cache.createdNanoflows = make(map[string]*createdNanoflowInfo)
	}
	qualifiedName := moduleName + "." + nfName
	ctx.Cache.createdNanoflows[qualifiedName] = &createdNanoflowInfo{
		ID:               id,
		Name:             nfName,
		ModuleName:       moduleName,
		ContainerID:      containerID,
		ReturnEntityName: returnEntityName,
	}
}

// trackCreatedPage registers a page created during this session.
func (ctx *ExecContext) trackCreatedPage(moduleName, pageName string, id, containerID model.ID) {
	ctx.ensureCache()
	if ctx.Cache.createdPages == nil {
		ctx.Cache.createdPages = make(map[string]*createdPageInfo)
	}
	qualifiedName := moduleName + "." + pageName
	ctx.Cache.createdPages[qualifiedName] = &createdPageInfo{
		ID:          id,
		Name:        pageName,
		ModuleName:  moduleName,
		ContainerID: containerID,
	}
}

// trackCreatedSnippet registers a snippet created during this session.
func (ctx *ExecContext) trackCreatedSnippet(moduleName, snippetName string, id, containerID model.ID) {
	ctx.ensureCache()
	if ctx.Cache.createdSnippets == nil {
		ctx.Cache.createdSnippets = make(map[string]*createdSnippetInfo)
	}
	qualifiedName := moduleName + "." + snippetName
	ctx.Cache.createdSnippets[qualifiedName] = &createdSnippetInfo{
		ID:          id,
		Name:        snippetName,
		ModuleName:  moduleName,
		ContainerID: containerID,
	}
}

// getCreatedMicroflow returns info about a microflow created during this
// session, or nil if not found.
func (ctx *ExecContext) getCreatedMicroflow(qualifiedName string) *createdMicroflowInfo {
	if ctx.Cache == nil || ctx.Cache.createdMicroflows == nil {
		return nil
	}
	return ctx.Cache.createdMicroflows[qualifiedName]
}

// getCreatedPage returns info about a page created during this session,
// or nil if not found.
func (ctx *ExecContext) getCreatedPage(qualifiedName string) *createdPageInfo {
	if ctx.Cache == nil || ctx.Cache.createdPages == nil {
		return nil
	}
	return ctx.Cache.createdPages[qualifiedName]
}

// ensureSqlMgr lazily initializes and returns the SQL connection manager.
func (ctx *ExecContext) ensureSqlMgr() *sqllib.Manager {
	if ctx.SqlMgr == nil {
		ctx.SqlMgr = sqllib.NewManager()
	}
	return ctx.SqlMgr
}
