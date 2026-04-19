// SPDX-License-Identifier: Apache-2.0

// Package executor - DROP/MOVE FOLDER commands
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// findFolderByPath walks a folder path under a module and returns the folder ID.
func findFolderByPath(ctx *ExecContext, moduleID model.ID, folderPath string, folders []*types.FolderInfo) (model.ID, error) {
	parts := strings.Split(folderPath, "/")
	currentContainerID := moduleID

	var targetFolderID model.ID
	for i, part := range parts {
		if part == "" {
			continue
		}

		var found bool
		for _, f := range folders {
			if f.ContainerID == currentContainerID && f.Name == part {
				currentContainerID = f.ID
				if i == len(parts)-1 {
					targetFolderID = f.ID
				}
				found = true
				break
			}
		}

		if !found {
			return "", mdlerrors.NewNotFound("folder", folderPath)
		}
	}

	if targetFolderID == "" {
		return "", mdlerrors.NewNotFound("folder", folderPath)
	}

	return targetFolderID, nil
}

// execDropFolder handles DROP FOLDER 'path' IN Module statements.
// The folder must be empty (no child documents or sub-folders).
func execDropFolder(ctx *ExecContext, s *ast.DropFolderStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnected()
	}

	module, err := findModule(ctx, s.Module)
	if err != nil {
		return mdlerrors.NewNotFound("module", s.Module)
	}

	folders, err := ctx.Backend.ListFolders()
	if err != nil {
		return mdlerrors.NewBackend("list folders", err)
	}

	folderID, err := findFolderByPath(ctx, module.ID, s.FolderPath, folders)
	if err != nil {
		return fmt.Errorf("%w in %s", err, s.Module)
	}

	if err := ctx.Backend.DeleteFolder(folderID); err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("delete folder '%s'", s.FolderPath), err)
	}

	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Dropped folder: '%s' in %s\n", s.FolderPath, s.Module)
	return nil
}

// execMoveFolder handles MOVE FOLDER Module.FolderName TO ... statements.
func execMoveFolder(ctx *ExecContext, s *ast.MoveFolderStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnected()
	}

	// Find the source module
	sourceModule, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewNotFound("source module", s.Name.Module)
	}

	// Find the source folder
	folders, err := ctx.Backend.ListFolders()
	if err != nil {
		return mdlerrors.NewBackend("list folders", err)
	}

	folderID, err := findFolderByPath(ctx, sourceModule.ID, s.Name.Name, folders)
	if err != nil {
		return fmt.Errorf("%w in %s", err, s.Name.Module)
	}

	// Determine target module
	var targetModule *model.Module
	if s.TargetModule != "" {
		targetModule, err = findModule(ctx, s.TargetModule)
		if err != nil {
			return mdlerrors.NewNotFound("target module", s.TargetModule)
		}
	} else {
		targetModule = sourceModule
	}

	// Resolve target container
	var targetContainerID model.ID
	if s.TargetFolder != "" {
		targetContainerID, err = resolveFolder(ctx, targetModule.ID, s.TargetFolder)
		if err != nil {
			return mdlerrors.NewBackend("resolve target folder", err)
		}
	} else {
		targetContainerID = targetModule.ID
	}

	// Move the folder
	if err := ctx.Backend.MoveFolder(folderID, targetContainerID); err != nil {
		return mdlerrors.NewBackend("move folder", err)
	}

	invalidateHierarchy(ctx)

	target := targetModule.Name
	if s.TargetFolder != "" {
		target += "/" + s.TargetFolder
	}
	fmt.Fprintf(ctx.Output, "Moved folder %s to %s\n", s.Name.String(), target)
	return nil
}
