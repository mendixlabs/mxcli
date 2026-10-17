// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/diaglog"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/model"
	sqllib "github.com/mendixlabs/mxcli/sql"
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

	// Diagnostics receives warnings about the answer rather than the answer
	// itself — "this result is incomplete because…" — so they never land in a
	// payload a caller parses (search --format json). Nil means os.Stderr; use
	// diagnostics() rather than reading the field.
	Diagnostics io.Writer

	// describeQualifyAttrs is set by DESCRIBE PAGE while it reads the widgets
	// inside a data container whose flow cannot be resolved: no entity is in
	// scope there, so an attribute binding keeps its stored Module.Entity.Attr
	// name rather than the bare one exec could not qualify. See describeAttr.
	describeQualifyAttrs bool

	// describeBody is set by describeWidgetBody while DESCRIBE renders a page's
	// or snippet's widgets: which widgets come out only as a comment, so a data
	// view selecting from one can be commented out with it (#1415). Nil
	// outside a describe body.
	describeBody *describeBodyState

	// scriptFlows are the flows earlier statements of a script declare,
	// keyed by scriptFlowKey, set only while check predicts a script's flow
	// verdicts (CheckFlowVerdicts): exec builds a flow after those statements
	// have run, so a call to one of them resolves against the declaration,
	// not the stored project, which does not hold it yet. Nil otherwise.
	scriptFlows map[string]*selfFlow

	// forwardFlowCalls mirrors Executor.forwardFlowCalls: whether a call to a
	// flow the script creates further down is accepted (mendixlabs/mxcli#1405).
	forwardFlowCalls bool

	// scriptTargets are the pages, microflows and nanoflows the program being
	// run declares, set under the same all-or-nothing run as scriptFlows
	// (Executor.enterScriptFlows). A navigation target is stored by name, so one
	// the script creates further down resolves there, as a flow call does.
	scriptTargets map[defKey]bool

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

	// widgetRegistry caches the widget definitions for this session. Loaded
	// once via GetWidgetRegistry, because DESCRIBE PAGE consults it per widget
	// and LoadWidgetRegistry reads .def.json files off disk.
	widgetRegistry       *WidgetRegistry
	widgetRegistryLoaded bool

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

	// describeLayout is set while a canonical DESCRIBE renders a flow body: the
	// layout annotations to keep, the rest being what the layout engine derives
	// on its own (derivedFlowLayout). nil — every other renderer — keeps them
	// all.
	describeLayout *flowLayoutKeep

	// flowAnalysis memoises a stored flow's graph analysis across the rounds
	// of one derivedFlowLayout; nil everywhere else.
	flowAnalysis *flowAnalysisMemo

	// describeFullLayout makes DESCRIBE of a flow keep every stored layout
	// annotation instead of the canonical subset (#748). Set by callers that
	// read the description back as an address map of the stored flow, such as
	// create or modify's diff (describedFlowStmt), which locates stored
	// activities by the @position printed above them.
	describeFullLayout bool

	// describeFallsOn is true while DESCRIBE walks a branch after which the
	// description goes on printing statements within the same loop iteration —
	// so an activity there with no way out (which ends the iteration in the
	// model) must say `continue;`, or it would fall into what follows. Reset at
	// every loop body; see traverseFlowUntilMerge.
	describeFallsOn bool

	// describeIn, when set, is the exact language DESCRIBE writes, in place of
	// describeLanguage's default. create or modify pins it to the script's own
	// version to describe the stored side of its diff, so both sides are read
	// by the same rules and a value compares as the same AST (ako/mxcli#804).
	describeIn *langver.Version

	// describeLang is the session's describe language (SetDescribeLanguage):
	// `--mdl 0` and the REPL's `mdl 0;`. describeIn, a pin for one internal
	// describe, takes precedence over it; nil means the frozen default.
	describeLang *langver.Version

	// describeID pins a describe to one stored document. A name is not a unique
	// key — a module may hold an excluded twin (#914) — so the catalog's source
	// build, which enumerates documents rather than names, sets it to describe
	// each twin as itself instead of the live one twice (#1185). Empty means
	// "by name", which is every interactive DESCRIBE.
	describeID model.ID

	// lastWriteStats is the storage write watermark as of the previous
	// ReportMutation call (or of this context's construction, i.e. the start of
	// the statement). Its only use is telling "Modified X" from "X was already
	// in sync"; see report_mutation.go.
	lastWriteStats backend.WriteStats

	// tally collapses a program run's "Unchanged" reports into one summary
	// line. Shared with the Executor (a pointer, so it survives across the
	// per-statement contexts) and nil outside a program run.
	tally *mutationTally

	// accessRun is the open run of access-rule statements this statement is
	// part of, which holds its report until the run is written; nil otherwise.
	accessRun *accessRuleRun

	// ScriptDepth tracks the current EXECUTE SCRIPT nesting level.
	// Incremented on each recursive call; execExecuteScript rejects calls
	// that exceed maxScriptDepth to prevent infinite self-referencing scripts.
	ScriptDepth int

	// ScriptDir is the directory of the .mdl file being executed, empty when the
	// statements did not come from a file (a -c one-liner, the REPL, a test).
	//
	// A relative path written INSIDE a script names a file next to that script,
	// not next to whoever happened to invoke mxcli. Resolving against the working
	// directory instead makes a script runnable only from one place — which is
	// exactly how the toolbox-bitmap example broke the doctype harness, whose
	// working directory is the package under test.
	ScriptDir string

	// LanguageVersion is the `mdl <n>;` header of the script being run, mdl 0
	// when it has none or when the statement is not from a script (ADR-0011).
	// A handler whose meaning depends on it declares a langver.Change and
	// branches on Change.Applies(ctx.LanguageVersion); it never assumes the
	// latest version.
	LanguageVersion langver.Version
}

// diagnostics returns the writer for warnings about a result (see the
// Diagnostics field): the configured one, or os.Stderr.
func (ctx *ExecContext) diagnostics() io.Writer {
	if ctx.Diagnostics != nil {
		return ctx.Diagnostics
	}
	return os.Stderr
}

// progress returns the writer for commentary about a run rather than its
// answer: "Connected to:", catalog load/build progress, a result header or
// count, "(no references found)". For a person that commentary is part of the
// reading, so in text mode it stays on Output exactly as before. When Output
// carries a JSON payload it goes to diagnostics() instead — every byte on
// Output is then handed to a parser, and one "Connected to:" line ahead of the
// array is enough to make `mxcli refs --json | jq .` fail.
//
// Route progress here, not to Output, in any code a JSON-mode command reaches.
func (ctx *ExecContext) progress() io.Writer {
	if ctx.Format == FormatJSON {
		return ctx.diagnostics()
	}
	return ctx.Output
}

// ResolveScriptRelative turns a path written inside an MDL script into an
// absolute one: relative to the script's own directory when that is known, and
// to the working directory otherwise.
func (ctx *ExecContext) ResolveScriptRelative(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return path, nil
	}
	if ctx != nil && ctx.ScriptDir != "" {
		if candidate := filepath.Join(ctx.ScriptDir, path); fileExists(candidate) {
			return candidate, nil
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, path), nil
}

// fileExists reports whether path names an existing file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Connected returns true if a project is connected via the Backend.
//
// Nil-safe on the receiver, which makes checkFeature's documented contract
// ("safe to call when not connected") true for a nil ctx as well. Every
// version-gated command reaches this through checkFeature, so without the guard
// each one panics rather than skipping when exercised without a context.
func (ctx *ExecContext) Connected() bool {
	return ctx != nil && ctx.Backend != nil && ctx.Backend.IsConnected()
}

// unitReadCacher is a backend that can hold the unit contents it reads in
// memory over a span (the modelsdk backend; see mpr.Reader.CacheUnitReads).
type unitReadCacher interface {
	CacheUnitReads() (release func())
}

// CacheUnitReads holds the units the backend reads in memory until release is
// called, for a span that writes nothing. check's passes, and exec's plan of a
// `create or modify`, look a flow, an entity or a module up again and again,
// and each lookup listed every document of its kind from disk: a `create or
// modify` of a stored microflow read every microflow file of the project ~9
// times (mendixlabs/mxcli#1272). A backend that cannot cache gets a no-op.
func (ctx *ExecContext) CacheUnitReads() (release func()) {
	if ctx != nil && ctx.Backend != nil {
		if c, ok := ctx.Backend.(unitReadCacher); ok && ctx.Backend.IsConnected() {
			return c.CacheUnitReads()
		}
	}
	return func() {}
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

// GetWidgetRegistry returns the session's widget registry, loading it on first
// use. It is cached because DESCRIBE PAGE asks per widget and the load reads
// every .def.json in the project — file I/O in a per-widget path is exactly
// what the review checklist warns against.
//
// A nil result is normal and means "no definitions available": with no project
// only the embedded widgets exist, and callers must degrade rather than treat
// it as an error.
func (ctx *ExecContext) GetWidgetRegistry() *WidgetRegistry {
	if ctx == nil {
		return nil
	}
	if ctx.widgetRegistryLoaded {
		return ctx.widgetRegistry
	}
	ctx.widgetRegistryLoaded = true
	// LoadWidgetRegistry wants the .mpr PATH, not its directory —
	// LoadUserDefinitions takes filepath.Dir of it internally. Passing the
	// directory looks one level too high and silently finds no definitions,
	// which shows up as DESCRIBE falling back to the widget-id form for every
	// widget rather than as an error.
	ctx.widgetRegistry = LoadWidgetRegistry(ctx.MprPath)
	return ctx.widgetRegistry
}
