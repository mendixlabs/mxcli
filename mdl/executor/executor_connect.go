// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

func execConnect(ctx *ExecContext, s *ast.ConnectStmt) error {
	e := ctx.executor
	if e.backend != nil && e.backend.IsConnected() {
		e.backend.Disconnect()
	}

	if e.backendFactory == nil {
		return mdlerrors.NewBackend("connect", fmt.Errorf("no backend factory configured"))
	}
	b := e.backendFactory()
	if err := b.Connect(s.Path); err != nil {
		return mdlerrors.NewBackend("connect", err)
	}

	e.backend = b
	e.mprPath = s.Path
	e.cache = &executorCache{} // Initialize fresh cache

	// Propagate connection state back to ctx so subsequent code in this
	// dispatch cycle sees the updated values.
	ctx.Backend = e.backend
	ctx.Cache = e.cache
	ctx.MprPath = e.mprPath

	// Reset project-scoped caches — previous project's catalog and theme
	// registry are invalid for the new connection.
	e.catalog = nil
	e.themeRegistry = nil
	ctx.Catalog = nil
	ctx.ThemeRegistry = nil

	// Display connection info with version
	pv := e.backend.ProjectVersion()
	if !ctx.Quiet {
		fmt.Fprintf(ctx.Output, "Connected to: %s (Mendix %s)\n", s.Path, pv.ProductVersion)
	}
	if ctx.Logger != nil {
		ctx.Logger.Connect(s.Path, pv.ProductVersion, pv.FormatVersion)
	}
	return nil
}

// reconnect closes the current connection and reopens it.
// This is needed when the project file has been modified externally.
func reconnect(ctx *ExecContext) error {
	e := ctx.executor
	if e.mprPath == "" {
		return mdlerrors.NewNotConnected()
	}

	// Close existing connection
	if e.backend != nil && e.backend.IsConnected() {
		e.backend.Disconnect()
	}

	// Reopen connection
	b := e.backendFactory()
	if err := b.Connect(e.mprPath); err != nil {
		return mdlerrors.NewBackend("reconnect", err)
	}

	e.backend = b
	e.cache = &executorCache{} // Reset cache

	// Propagate reconnection state back to ctx.
	ctx.Backend = e.backend
	ctx.Cache = e.cache

	// Reset project-scoped caches — file may have changed externally.
	e.catalog = nil
	e.themeRegistry = nil
	ctx.Catalog = nil
	ctx.ThemeRegistry = nil

	return nil
}

func execDisconnect(ctx *ExecContext) error {
	e := ctx.executor
	if e.backend == nil || !e.backend.IsConnected() {
		fmt.Fprintln(ctx.Output, "Not connected")
		return nil
	}

	// Reconcile any pending security changes before closing
	if err := e.finalizeProgramExecution(); err != nil {
		fmt.Fprintf(ctx.Output, "Warning: finalization error: %v\n", err)
	}

	e.backend.Disconnect()
	fmt.Fprintf(ctx.Output, "Disconnected from: %s\n", e.mprPath)
	e.mprPath = ""
	e.cache = nil
	e.backend = nil

	// Propagate disconnection state back to ctx so subsequent code in this
	// dispatch cycle sees the cleared values.
	ctx.Backend = nil
	ctx.MprPath = ""
	ctx.Cache = nil

	return nil
}

// Executor method wrappers — kept during migration for callers not yet
// converted to free functions. Remove once all callers are migrated.

func execStatus(ctx *ExecContext) error {
	e := ctx.executor
	if e.backend == nil || !e.backend.IsConnected() {
		fmt.Fprintln(ctx.Output, "Status: Not connected")
		return nil
	}

	pv := e.backend.ProjectVersion()
	fmt.Fprintf(ctx.Output, "Status: Connected\n")
	fmt.Fprintf(ctx.Output, "Project: %s\n", e.mprPath)
	fmt.Fprintf(ctx.Output, "Mendix Version: %s\n", pv.ProductVersion)
	fmt.Fprintf(ctx.Output, "MPR Format: v%d\n", pv.FormatVersion)

	// Show module count
	modules, err := e.backend.ListModules()
	if err == nil {
		fmt.Fprintf(ctx.Output, "Modules: %d\n", len(modules))
	}

	return nil
}
