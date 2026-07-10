// SPDX-License-Identifier: Apache-2.0

// Package executor - DROP MICROFLOW command
package executor

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
)

// execDropMicroflow handles DROP MICROFLOW statements.
func execDropMicroflow(ctx *ExecContext, s *ast.DropMicroflowStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find and delete the microflow
	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return mdlerrors.NewBackend("list microflows", err)
	}

	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == s.Name.Module && mf.Name == s.Name.Name {
			qualifiedName := s.Name.Module + "." + s.Name.Name
			// Remember the UnitID and ContainerID *before* deletion so that a
			// subsequent CREATE OR REPLACE/MODIFY for the same qualified name
			// can reuse them. This keeps Studio Pro compatible by turning
			// delete+insert into an in-place update from the file's
			// perspective — same UnitID, same folder, just new bytes.
			rememberDroppedMicroflow(ctx, qualifiedName, mf.ID, mf.ContainerID, mf.AllowedModuleRoles)
			if err := ctx.Backend.DeleteMicroflow(mf.ID); err != nil {
				return mdlerrors.NewBackend("delete microflow", err)
			}
			// Clear executor-level caches so subsequent CREATE sees fresh state
			if ctx.Cache != nil && ctx.Cache.createdMicroflows != nil {
				delete(ctx.Cache.createdMicroflows, qualifiedName)
			}
			invalidateHierarchy(ctx)
			fmt.Fprintf(ctx.Output, "Dropped microflow: %s.%s\n", s.Name.Module, s.Name.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFound("microflow", s.Name.Module+"."+s.Name.Name)
}
