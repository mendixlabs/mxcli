// SPDX-License-Identifier: Apache-2.0

// Package executor executes MDL AST statements against a Mendix project.
package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/diaglog"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	sqllib "github.com/mendixlabs/mxcli/sql"
)

// executorCache holds cached data for performance across multiple operations.
type executorCache struct {
	// modulesMu guards modules, the one cache a parallel describe fills
	// lazily (see getModulesFromCache, ako/mxcli#765).
	modulesMu    sync.Mutex
	modules      []*model.Module
	units        []*types.UnitInfo
	folders      []*types.FolderInfo
	domainModels []*domainmodel.DomainModel
	enumerations []*model.Enumeration
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
	mfs, err := microflowHeaders(ctx)
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

// executeTimeoutDefault is the limit effectiveExecuteTimeout applies when no
// MXCLI_EXEC_TIMEOUT is set. A variable so a test can bring it below the
// duration of a catalog build.
var executeTimeoutDefault = defaultExecuteTimeout

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
	return executeTimeoutDefault
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
	scriptDir      string           // directory of the .mdl file being executed (see SetScriptDir)
	langVersion    langver.Version  // `mdl <n>;` of the program being run (see enterLanguage)
	describeLang   *langver.Version // the language DESCRIBE writes, when set (SetDescribeLanguage)
	// forwardFlowCalls is set when the run is all-or-nothing (SetForwardFlowCalls);
	// scriptFlows then holds every flow the program being run declares.
	forwardFlowCalls bool
	scriptFlows      map[string]*selfFlow
	scriptTargets    map[defKey]bool // pages and flows the program declares, same run
	settings         map[string]any
	cache            *executorCache
	catalog          *catalog.Catalog
	quiet            bool            // suppress connection and status messages
	tally            *mutationTally  // collapses a program run's "Unchanged" reports into one line
	accessRun        *accessRuleRun  // the open run of access-rule statements, if any (#872, #890)
	format           OutputFormat    // output format (table, json)
	logger           *diaglog.Logger // session diagnostics logger (nil = no logging)
	tracer           *backend.Tracer // MCP tool-call tracer (--mcp-trace; nil = off)
	stmtGuard        func(ast.Statement) (ast.Statement, error)
	fragments        map[string]*ast.DefineFragmentStmt // script-scoped fragment definitions
	sqlMgr           *sqllib.Manager                    // external SQL connection manager (lazy init)
	themeRegistry    *ThemeRegistry                     // cached theme design property definitions (lazy init)
	registry         *Registry                          // statement dispatch registry
	catalogMu        sync.RWMutex                       // protects catalog field from background goroutine writes
	untimedMu        sync.Mutex                         // protects untimed
	untimed          *untimedWork                       // the running statement's untimed-work tracker (see untimed_work.go)
	catalogGen       uint64                             // monotonic generation counter for catalog swaps
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

// SetScriptDir records the directory of the .mdl file being executed, so a
// relative path written inside a script names a file next to that script rather
// than next to whoever invoked mxcli.
//
// Empty for a -c one-liner, the REPL or stdin, where there is no script to be
// relative to; resolution then falls back to the working directory.
func (e *Executor) SetScriptDir(dir string) {
	e.scriptDir = dir
}

// SetDescribeLanguage sets the MDL language describe writes for the rest of
// the session: `mxcli describe --mdl 0`, `--mdl` on the REPL and `-c`, and the
// REPL's `mdl 0;` / `mdl 1;` (ako/mxcli#840). Unset, describe writes the
// frozen language, mdl 1, headed by `mdl 1;` (describeLanguage).
func (e *Executor) SetDescribeLanguage(v langver.Version) {
	e.describeLang = &v
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

// SetStatementGuard installs a function every statement passes through before
// it runs — including the statements of a nested EXECUTE SCRIPT, which are
// dispatched through Execute too. It returns the statement to run in its place,
// or an error to refuse it. `mxcli diff` uses it to keep a script it runs on a
// scratch copy from reaching anything but the copy.
func (e *Executor) SetStatementGuard(g func(ast.Statement) (ast.Statement, error)) {
	e.stmtGuard = g
}

// Execute runs a single MDL statement with output-line and wall-clock guards.
// Each statement gets a fresh line budget. If the statement exceeds maxOutputLines
// lines of output or runs longer than the configured timeout, it is aborted with an error.
func (e *Executor) Execute(stmt ast.Statement) error {
	start := time.Now()

	if e.stmtGuard != nil {
		s, err := e.stmtGuard(stmt)
		if err != nil {
			return err
		}
		stmt = s
	}

	// Announce the MDL command so the PED calls it triggers (reported by the MCP
	// client) group under it (--mcp-trace, level 2; no-op otherwise).
	e.tracer.Statement(stmtSummary(stmt))

	// Reset per-statement line counter.
	if e.guard != nil {
		e.guard.reset()
	}

	// Enforce the wall-clock timeout with a timer that skips untimed work.
	// The goroutine pattern is retained because handlers are not yet
	// context-aware; threading context through handlers is a follow-up.
	executeTimeout := effectiveExecuteTimeout(stmt)

	// Time spent rebuilding the catalog implicitly is not counted against the
	// default limit, for the reason REFRESH CATALOG is exempt from it
	// (mendixlabs/mxcli#1329). An explicit MXCLI_EXEC_TIMEOUT caps it, as it
	// caps REFRESH CATALOG.
	untimed := e.pushUntimed(os.Getenv("MXCLI_EXEC_TIMEOUT") == "")
	defer e.popUntimed(untimed)
	baseCtx := withUntimedWork(context.Background(), untimed)

	var err error
	if executeTimeout <= 0 {
		// No wall-clock guard for exempt statements (e.g. REFRESH CATALOG on a
		// large project). The output-line guard still bounds runaway output.
		err = e.executeInner(baseCtx, stmt)
	} else {
		ctx, cancel := context.WithCancel(baseCtx)
		defer cancel()

		type result struct{ err error }
		ch := make(chan result, 1)
		begun := time.Now()
		go func() {
			ch <- result{e.executeInner(ctx, stmt)}
		}()

		timer := time.NewTimer(executeTimeout)
		defer timer.Stop()
	wait:
		for {
			select {
			case r := <-ch:
				err = r.err
				break wait
			case <-timer.C:
				now := time.Now()
				counted := now.Sub(begun) - untimed.excluded(now)
				if remaining := executeTimeout - counted; remaining > 0 {
					timer.Reset(remaining)
					continue
				}
				cancel()
				err = executeTimeoutError(executeTimeout, untimed.inProgress())
				break wait
			}
		}
	}

	// A handler that ignored the guard's Write error still "succeeded" with its
	// output cut off at the limit; that must fail the statement, not exit 0
	// (mendixlabs/mxcli#1425).
	if err == nil && e.guard != nil {
		err = e.guard.exceededError()
	}

	if e.logger != nil {
		e.logger.Command(stmtTypeName(stmt), stmtSummary(stmt), time.Since(start), err)
	}
	return err
}

// executeTimeoutError is the error for a statement the wall-clock guard
// stopped. When it stopped an implicit catalog rebuild it says so: the build
// is only saved once it finishes, so the next statement that needs the catalog
// starts the same rebuild over — without the hint, every retry looks like a new
// failure (mendixlabs/mxcli#1329).
func executeTimeoutError(limit time.Duration, building string) error {
	if building == "" {
		return mdlerrors.NewValidationf("statement timed out after %v (raise the limit with MXCLI_EXEC_TIMEOUT, e.g. MXCLI_EXEC_TIMEOUT=30m)", limit)
	}
	return mdlerrors.NewValidationf("statement timed out after %v while rebuilding the catalog (%s); the catalog cache was not updated, so the next statement that needs it rebuilds again. Rebuild it once with REFRESH CATALOG FULL SOURCE (with MXCLI_EXEC_TIMEOUT unset, so the default limit does not apply), or raise the limit with MXCLI_EXEC_TIMEOUT, e.g. MXCLI_EXEC_TIMEOUT=30m", limit, building)
}

// pushUntimed installs a fresh untimed-work tracker for the statement about to
// run, chained to the enclosing statement's when Execute is re-entered by
// EXECUTE SCRIPT.
func (e *Executor) pushUntimed(exclude bool) *untimedWork {
	e.untimedMu.Lock()
	defer e.untimedMu.Unlock()
	w := newUntimedWork(e.untimed, exclude)
	e.untimed = w
	return w
}

// popUntimed restores the enclosing statement's tracker.
func (e *Executor) popUntimed(w *untimedWork) {
	e.untimedMu.Lock()
	defer e.untimedMu.Unlock()
	if e.untimed == w {
		e.untimed = w.parent
	}
}

// ExecuteProgram runs all statements in a program.
func (e *Executor) ExecuteProgram(prog *ast.Program) error {
	_, err := e.ExecuteProgramReportingStop(prog)
	return err
}

// ExecuteProgramReportingStop is ExecuteProgram that also says where it
// stopped: the 0-based index of the statement whose error ended the run, or
// -1 when no statement failed (an error from the run's end-of-program work —
// the access-rule flush, reconciliation — is not one statement's). A caller
// running several statements uses it to say which one failed and how many
// were not run, rather than leaving the rest silently skipped
// (mendixlabs/mxcli#1218).
func (e *Executor) ExecuteProgramReportingStop(prog *ast.Program) (stoppedAt int, err error) {
	stoppedAt = -1
	if e.beginTally() {
		defer e.flushTally()
	}
	defer e.enterLanguage(prog.LanguageVersion)()
	defer e.enterScriptFlows(prog)()

	// Collect all names defined in the script for forward-reference hints.
	allDefined := newScriptContext()
	allDefined.collectDefinitions(prog)

	// Track which names have been created so far.
	created := newScriptContext()

	// A run of access-rule statements is written once, at its end (#872). The
	// deferred end also covers a statement that fails mid-run: what the run's
	// earlier statements did still lands, as it did when each wrote itself. If
	// it cannot land, that is returned with the statement's error — those
	// statements already reported success.
	var rules accessRuleRun
	defer func() {
		if ferr := rules.end(); ferr != nil {
			err = errors.Join(err, ferr)
		}
	}()

	for i, stmt := range prog.Statements {
		if err := rules.step(e, stmt); err != nil {
			return -1, err
		}
		if err := e.Execute(stmt); err != nil {
			return i, annotateForwardRef(err, stmt, created, allDefined)
		}
		created.collectSingle(stmt)
	}
	if err := rules.end(); err != nil {
		return -1, err
	}
	return -1, e.finalizeProgramExecution()
}

// SetForwardFlowCalls says whether a call may name a microflow or nanoflow a
// LATER statement of the script creates (mendixlabs/mxcli#1405). A call is
// stored by name, so it can be written before its callee; what makes it unsafe
// is a later statement failing and leaving the call dangling. Only a run that
// rolls back on failure — plain `mxcli exec` under the #1408 journal — may
// allow it, and `mxcli check`, which cannot know how the script will run.
// Off by default: --continue-on-error, the REPL and -c refuse it.
func (e *Executor) SetForwardFlowCalls(on bool) { e.forwardFlowCalls = on }

// enterScriptFlows makes every flow the program declares resolvable by a call
// in it, wherever the declaration stands, and returns the function that
// restores the previous set. Only a run that can roll back does this
// (SetForwardFlowCalls); otherwise a call still resolves in statement order.
// A nested EXECUTE SCRIPT sees its caller's flows as well as its own.
func (e *Executor) enterScriptFlows(prog *ast.Program) func() {
	if !e.forwardFlowCalls {
		return func() {}
	}
	prev, prevTargets := e.scriptFlows, e.scriptTargets
	flows := make(map[string]*selfFlow, len(prev))
	for k, v := range prev {
		flows[k] = v
	}
	targets := make(map[defKey]bool, len(prevTargets))
	for k, v := range prevTargets {
		targets[k] = v
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			declareScriptFlow(flows, false, s.Name, s.ReturnType)
		case *ast.CreateNanoflowStmt:
			declareScriptFlow(flows, true, s.Name, s.ReturnType)
		}
		// What a navigation target may name, wherever the declaration stands.
		if key, _ := definedBy(stmt); key.name != "" {
			switch key.kind {
			case defPage, defMicroflow, defNanoflow:
				targets[key] = true
			}
		}
	}
	e.scriptFlows, e.scriptTargets = flows, targets
	return func() { e.scriptFlows, e.scriptTargets = prev, prevTargets }
}

// enterLanguage runs the following statements under a program's language
// version and returns the function that restores the previous one. The version
// belongs to the script, so a nested EXECUTE SCRIPT runs under its own header
// and its caller's is back afterwards. A statement run outside a program (the
// REPL, a -c one-liner) is mdl 0, the same as a headerless script.
func (e *Executor) enterLanguage(v langver.Version) func() {
	prev := e.langVersion
	e.langVersion = v
	return func() { e.langVersion = prev }
}

// ExecuteProgramResult reports the outcome of a continue-on-error run.
type ExecuteProgramResult struct {
	Total     int // statements attempted
	Succeeded int
	Failed    int
}

// ExecuteProgramContinueOnError runs every statement in the program, reporting
// each failure to w instead of halting on the first one, and returns a summary.
// This makes a partially-applied domain script re-runnable: the already-applied
// statements error individually (e.g. "attribute already exists") while the not-
// yet-applied ones still run. Unlike the default ExecuteProgram, a failure never
// masks later work — but each error is surfaced, so a real mistake is still
// visible (the caller is expected to exit non-zero when Failed > 0). ErrExit is
// honoured (stops the run) and returned, so `exit`/`quit` still work.
func (e *Executor) ExecuteProgramContinueOnError(prog *ast.Program, w io.Writer) (ExecuteProgramResult, error) {
	if e.beginTally() {
		defer e.flushTally()
	}
	defer e.enterLanguage(prog.LanguageVersion)()

	allDefined := newScriptContext()
	allDefined.collectDefinitions(prog)
	created := newScriptContext()

	var rules accessRuleRun // see ExecuteProgram
	defer func() { _ = rules.end() }()

	var res ExecuteProgramResult
	for i, stmt := range prog.Statements {
		res.Total++
		if err := rules.step(e, stmt); err != nil {
			return res, err
		}
		if err := e.Execute(stmt); err != nil {
			if errors.Is(err, ErrExit) {
				return res, err
			}
			res.Failed++
			fmt.Fprintf(w, "statement %d: %v\n", i+1, annotateForwardRef(err, stmt, created, allDefined))
			continue
		}
		res.Succeeded++
		created.collectSingle(stmt)
	}
	if err := rules.end(); err != nil {
		return res, err
	}
	if err := e.finalizeProgramExecution(); err != nil {
		return res, err
	}
	return res, nil
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

// CacheUnitReads holds the units the backend reads in memory until release is
// called, for a pass that writes nothing (see ExecContext.CacheUnitReads).
func (e *Executor) CacheUnitReads() (release func()) {
	return (&ExecContext{Backend: e.Backend()}).CacheUnitReads()
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

// beginTally starts collapsing this program run's "Unchanged" reports, and
// reports whether this call owns the tally (see mutationTally.begin).
func (e *Executor) beginTally() bool {
	if e.tally == nil {
		e.tally = &mutationTally{}
	}
	return e.tally.begin()
}

// flushTally writes the run's one-line summary and stands the tally down.
func (e *Executor) flushTally() {
	e.tally.flush(e.output)
	e.tally.end()
}
