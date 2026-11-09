// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"sort"

	"github.com/mendixlabs/mxcli/model"
)

// buildFolders writes one row per folder, empty folders included
// (ako/mxcli#1436). Every other table names a folder only through the Folder
// column of a document in it, so a folder holding nothing appeared nowhere and
// a rule checking a module's folder layout reported it missing.
//
// The rows are project-wide, like the modules pass: a change to a folder is
// one no per-unit pass reads, which already makes the next build a full one.
// The folders come from the hierarchy, which listed them once for every
// document's folder path; listing them again would read the whole project on
// each incremental refresh.
func (b *Builder) buildFolders() error {
	h := b.hierarchy
	folders := make([]model.ID, 0, len(h.folderNames))
	for id := range h.folderNames {
		folders = append(folders, id)
	}

	// Direct children of each folder: the units it holds and the folders
	// nested in it. A folder is listed as a unit too, so folderNames is
	// checked first.
	docs := map[model.ID]int{}
	subs := map[model.ID]int{}
	for child, parent := range h.containerParent {
		if _, isFolder := h.folderNames[child]; isFolder {
			subs[parent]++
		} else if t := h.unitTypes[child]; t != "" && t != "Projects$Folder" {
			docs[parent]++
		}
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO folders_data (Id, Name, ModuleName, Path, ParentPath, Depth,
			DocumentCount, FolderCount, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// Sorted, so two builds of one project write the same bytes.
	sort.Slice(folders, func(i, j int) bool { return folders[i] < folders[j] })
	projectID, snapshotID := b.snapshotMeta()
	for _, id := range folders {
		name, container := h.folderNames[id], h.containerParent[id]
		moduleName := h.getModuleName(h.findModuleID(container))
		parentPath := h.buildFolderPath(container)
		path := name
		if parentPath != "" {
			path = parentPath + "/" + name
		}
		if _, err := stmt.Exec(
			string(id), name, moduleName, path, parentPath,
			h.folderDepth(id),
			docs[id], subs[id],
			projectID, snapshotID,
		); err != nil {
			return err
		}
	}

	b.report("Folders", len(folders))
	return nil
}

// folderDepth counts the folders from id up to its module, id included. It
// walks the parents rather than counting "/" in the path: a folder name may
// itself hold a slash (DeepLink ships "Pages/Snippets"), which the path, like
// every document's Folder column, does not escape.
func (h *hierarchy) folderDepth(id model.ID) int {
	depth := 0
	current := id
	for range 100 {
		if _, ok := h.folderNames[current]; !ok {
			break
		}
		depth++
		parent, ok := h.containerParent[current]
		if !ok || parent == current {
			break
		}
		current = parent
	}
	return depth
}
