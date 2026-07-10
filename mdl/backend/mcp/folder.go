// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// Folders over MCP work the way PED allows: a document created with a folderPath
// auto-creates the whole path, but PED can neither create an *empty* folder
// (Projects$Folder is off the create whitelist) nor re-parent an existing document
// (a document's folderPath is not a settable property). So:
//
//   - CreateFolder records the folder as pending; it materializes on disk when the
//     first document is created into it (the executor places documents into folders
//     it asked us to create, by setting their container to the folder's ID).
//   - The create paths resolve a document's container — module or folder — to a
//     (moduleName, folderPath) pair and pass folderPath to ped_create_document.
//   - DeleteFolder and MoveFolder (and the per-document Move* family) are rejected:
//     PED exposes no folder delete and no document re-parent.
//
// Why MOVE is not faked via delete-then-recreate: re-parenting must preserve the
// document's $ID so existing references (call-microflow actions, page/navigation
// references, …) stay valid — exactly what the MPR/modelsdk engine does in place.
// Recreating would mint a NEW $ID, dangling those references and producing a
// different project state for the same MDL. The backend abstraction requires both
// engines to yield equivalent results, so a lookalike MOVE is worse than an honest
// rejection.

// CreateFolder records a pending folder. PED can't create an empty one, but the
// folder is realized when a document is created into it (see resolveDocContainer).
// The executor has already assigned folder.ID, which it uses as the container of
// documents placed in this folder.
func (b *Backend) CreateFolder(folder *model.Folder) error {
	if folder.ID == "" {
		return fmt.Errorf("create folder %q: missing folder id", folder.Name)
	}
	b.sessionFolders = append(b.sessionFolders, &types.FolderInfo{
		ID:          folder.ID,
		ContainerID: folder.ContainerID,
		Name:        folder.Name,
	})
	return nil
}

// DeleteFolder is rejected: PED has no folder-delete (and an empty folder doesn't
// exist on disk in the first place).
func (b *Backend) DeleteFolder(id model.ID) error {
	return fmt.Errorf("DROP FOLDER is not supported by the MCP backend — PED cannot delete folders (a folder exists only while it holds documents); reorganize in Studio Pro or against a local .mpr")
}

// MoveFolder is rejected: PED cannot re-parent folders or documents.
func (b *Backend) MoveFolder(id, newContainerID model.ID) error {
	return fmt.Errorf("MOVE FOLDER is not supported by the MCP backend — PED cannot re-parent folders or documents; reorganize in Studio Pro or against a local .mpr")
}

// ListFolders returns the local reader's folders merged with folders created this
// session (so the executor's resolveFolder finds a just-created folder and builds
// nested paths against it instead of recreating segments).
func (b *Backend) ListFolders() ([]*types.FolderInfo, error) {
	local, err := b.reader.ListFolders()
	if err != nil {
		return nil, err
	}
	if len(b.sessionFolders) == 0 {
		return local, nil
	}
	return append(append([]*types.FolderInfo{}, local...), b.sessionFolders...), nil
}

// resolveDocContainer maps a document's container ID — a module or a folder — onto
// the PED moduleName and folderPath. folderPath is "" for a module-root document;
// for a foldered one it is the slash-joined folder path (e.g. "Processing/Archive").
func (b *Backend) resolveDocContainer(containerID model.ID) (moduleName, folderPath string, err error) {
	if mod, e := b.GetModule(containerID); e == nil {
		return mod.Name, "", nil
	}
	// Walk up the folder chain to the owning module, collecting names.
	var parts []string
	cur := containerID
	for range 64 { // depth guard
		fi := b.findFolder(cur)
		if fi == nil {
			return "", "", fmt.Errorf("container %s is neither a module nor a known folder", containerID)
		}
		parts = append([]string{fi.Name}, parts...)
		if mod, e := b.GetModule(fi.ContainerID); e == nil {
			return mod.Name, strings.Join(parts, "/"), nil
		}
		cur = fi.ContainerID
	}
	return "", "", fmt.Errorf("folder hierarchy for %s is too deep or cyclic", containerID)
}

// findFolder looks a folder up by ID, preferring session folders then the reader.
func (b *Backend) findFolder(id model.ID) *types.FolderInfo {
	for _, f := range b.sessionFolders {
		if f.ID == id {
			return f
		}
	}
	if b.reader == nil {
		return nil
	}
	if all, err := b.reader.ListFolders(); err == nil {
		for _, f := range all {
			if f.ID == id {
				return f
			}
		}
	}
	return nil
}
