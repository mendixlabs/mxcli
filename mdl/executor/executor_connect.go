// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
)

func execConnect(ctx *ExecContext, s *ast.ConnectStmt) error {
	if ctx.Backend != nil && ctx.Backend.IsConnected() {
		if err := ctx.Backend.Disconnect(); err != nil {
			fmt.Fprintf(ctx.Output, "Warning: disconnect error: %v\n", err)
		}
	}

	if ctx.BackendFactory == nil {
		return mdlerrors.NewBackend("connect", errors.New("no backend factory configured"))
	}
	b := ctx.BackendFactory()
	if err := b.Connect(s.Path); err != nil {
		return mdlerrors.NewBackend("connect", err)
	}

	ctx.Backend = b
	ctx.MprPath = s.Path
	ctx.Cache = &executorCache{} // Initialize fresh cache

	// Reset project-scoped caches — previous project's catalog and theme
	// registry are invalid for the new connection.
	ctx.Catalog = nil
	ctx.ThemeRegistry = nil

	// Display connection info with version
	pv := ctx.Backend.ProjectVersion()
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
	if ctx.MprPath == "" {
		return mdlerrors.NewNotConnected()
	}

	// Close existing connection
	if ctx.Backend != nil && ctx.Backend.IsConnected() {
		if err := ctx.Backend.Disconnect(); err != nil {
			fmt.Fprintf(ctx.Output, "Warning: disconnect error: %v\n", err)
		}
	}

	// Reopen connection
	if ctx.BackendFactory == nil {
		return mdlerrors.NewBackend("reconnect", fmt.Errorf("no backend factory configured"))
	}
	b := ctx.BackendFactory()
	if err := b.Connect(ctx.MprPath); err != nil {
		return mdlerrors.NewBackend("reconnect", err)
	}

	ctx.Backend = b
	ctx.Cache = &executorCache{} // Reset cache

	// Reset project-scoped caches — file may have changed externally.
	ctx.Catalog = nil
	ctx.ThemeRegistry = nil

	return nil
}

func execDisconnect(ctx *ExecContext) error {
	if ctx.Backend == nil || !ctx.Backend.IsConnected() {
		fmt.Fprintln(ctx.Output, "Not connected")
		return nil
	}

	// Reconcile any pending security changes before closing
	if ctx.FinalizeFn != nil {
		if err := ctx.FinalizeFn(); err != nil {
			fmt.Fprintf(ctx.Output, "Warning: finalization error: %v\n", err)
		}
	}

	if err := ctx.Backend.Disconnect(); err != nil {
		fmt.Fprintf(ctx.Output, "Warning: disconnect error: %v\n", err)
	}
	fmt.Fprintf(ctx.Output, "Disconnected from: %s\n", ctx.MprPath)
	ctx.MprPath = ""
	ctx.Cache = nil
	ctx.Backend = nil

	return nil
}

func execStatus(ctx *ExecContext) error {
	if ctx.Backend == nil || !ctx.Backend.IsConnected() {
		fmt.Fprintln(ctx.Output, "Status: Not connected")
		return nil
	}

	pv := ctx.Backend.ProjectVersion()
	fmt.Fprintf(ctx.Output, "Status: Connected\n")
	fmt.Fprintf(ctx.Output, "Project: %s\n", ctx.MprPath)
	fmt.Fprintf(ctx.Output, "Mendix Version: %s\n", pv.ProductVersion)
	fmt.Fprintf(ctx.Output, "MPR Format: v%d\n", pv.FormatVersion)

	// Show module count
	modules, err := ctx.Backend.ListModules()
	if err == nil {
		fmt.Fprintf(ctx.Output, "Modules: %d\n", len(modules))
	}

	return nil
}
