// SPDX-License-Identifier: Apache-2.0

// Package executor - Snippet commands (SHOW/DESCRIBE SNIPPETS)
package executor

import (
	"fmt"
	"sort"
	"strings"

	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
)

// listSnippets handles SHOW SNIPPETS command.
func listSnippets(ctx *ExecContext, moduleName string) error {
	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Get all snippets
	snippets, err := ctx.Backend.ListSnippets()
	if err != nil {
		return mdlerrors.NewBackend("list snippets", err)
	}

	// Collect rows
	type row struct {
		qualifiedName string
		module        string
		name          string
		folderPath    string
		params        int
	}
	var rows []row

	for _, s := range snippets {
		modID := h.FindModuleID(s.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName == "" || modName == moduleName {
			qualifiedName := modName + "." + s.Name
			folderPath := h.BuildFolderPath(s.ContainerID)

			rows = append(rows, row{qualifiedName, modName, s.Name, folderPath, len(s.Parameters)})
		}
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Folder", "Params"},
		Summary: fmt.Sprintf("(%d snippets)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.module, r.name, r.folderPath, r.params})
	}
	return writeResult(ctx, result)
}
