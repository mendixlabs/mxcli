// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
)

// executeInner dispatches a statement to its registered handler.
func (e *Executor) executeInner(ctx context.Context, stmt ast.Statement) error {
	ectx := e.newExecContext(ctx)
	// Remember the connection this outer context was snapshotted with. A handler
	// that runs nested statements (EXECUTE SCRIPT, generated-connector MDL) calls
	// e.Execute recursively; a nested reconnect — e.g. REFRESH CATALOG finding the
	// on-disk cache stale ("project file modified") — swaps e.backend for a fresh
	// connection and syncs it back. This outer ectx, snapshotted before that,
	// still points at the now-closed backend; without the adoption below, syncBack
	// would clobber the live connection with it and silently disconnect the
	// session (next statement fails with "not connected to a project").
	before := e.backend
	err := e.registry.Dispatch(ectx, stmt)
	// Only sync back when the context has not been cancelled. Execute() runs
	// executeInner in a goroutine with a wall-clock timeout; if the timeout
	// fires, the goroutine keeps running but Execute() has already returned.
	// Syncing stale state back at that point would race with subsequent calls.
	// Any handler-side state changes made after cancellation are intentionally
	// lost — this is expected behavior, not a regression.
	if ctx.Err() == nil {
		// If a nested statement replaced the connection and this handler didn't
		// change its own snapshot, adopt the live connection and its project-scoped
		// caches so syncBack preserves the reconnect instead of reverting it.
		if ectx.Backend == before && e.backend != before {
			ectx.Backend = e.backend
			ectx.MprPath = e.mprPath
			ectx.Cache = e.cache
			e.catalogMu.RLock()
			ectx.Catalog = e.catalog
			e.catalogMu.RUnlock()
		}
		e.syncBack(ectx)
	}
	return err
}

// syncBack copies mutated ExecContext fields back to the Executor so that
// the next newExecContext call picks up handler-side state changes.
//
// Fields intentionally NOT synced back (read-only from handler perspective):
//   - Output, Quiet, Logger — set once at Executor construction
//   - BackendFactory — set once at Executor construction
//   - OutputGuard — removed; writeDescribeJSON captures via Output swap only
//   - ExecuteFn, ExecuteProgramFn, FinalizeFn — bound to Executor methods, immutable
//
// Format IS synced back so that `SET format = json` takes effect for all
// subsequent statements in the same session.
func (e *Executor) syncBack(ctx *ExecContext) {
	e.backend = ctx.Backend
	e.mprPath = ctx.MprPath
	e.cache = ctx.Cache
	e.format = ctx.Format
	e.catalogMu.Lock()
	old := e.catalog
	e.catalog = ctx.Catalog
	if old != ctx.Catalog {
		e.catalogGen++
	}
	e.catalogMu.Unlock()
	// Close the previously installed catalog whenever ownership moved to a
	// different catalog value. This includes transitions to nil; otherwise the
	// previous catalog can leak if a handler clears ctx.Catalog without closing
	// the old instance itself.
	if old != nil && old != ctx.Catalog {
		old.Close()
	}
	e.settings = ctx.Settings
	e.fragments = ctx.Fragments
	e.sqlMgr = ctx.SqlMgr
	e.themeRegistry = ctx.ThemeRegistry
}

// newExecContext builds an ExecContext from the current Executor state.
func (e *Executor) newExecContext(ctx context.Context) *ExecContext {
	e.catalogMu.RLock()
	cat := e.catalog
	gen := e.catalogGen
	e.catalogMu.RUnlock()
	return &ExecContext{
		Context:          ctx,
		Backend:          e.backend,
		Output:           e.output,
		Format:           e.format,
		Quiet:            e.quiet,
		Logger:           e.logger,
		Fragments:        e.fragments,
		Catalog:          cat,
		Cache:            e.cache,
		MprPath:          e.mprPath,
		SqlMgr:           e.sqlMgr,
		ThemeRegistry:    e.themeRegistry,
		Settings:         e.settings,
		BackendFactory:   e.backendFactory,
		ExecuteFn:        e.Execute,
		ExecuteProgramFn: e.ExecuteProgram,
		FinalizeFn:       e.finalizeProgramExecution,
		SyncCatalog: func(cat *catalog.Catalog) {
			e.catalogMu.Lock()
			defer e.catalogMu.Unlock()
			// Only apply the background result if no newer catalog has been
			// installed since the build started (generation check). This
			// prevents an out-of-date background build from overwriting a
			// fresher foreground refresh.
			if e.catalogGen != gen {
				cat.Close()
				return
			}
			old := e.catalog
			e.catalog = cat
			e.catalogGen++
			if old != nil && old != cat {
				old.Close()
			}
		},
	}
}

// Ensure ast import is used via executeInner's stmt parameter.
var _ ast.Statement = (*ast.HelpStmt)(nil)
