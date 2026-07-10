// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
)

// listPages handles SHOW PAGES command.
func listPages(ctx *ExecContext, moduleName string) error {
	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Get all pages
	pages, err := ctx.Backend.ListPages()
	if err != nil {
		return mdlerrors.NewBackend("list pages", err)
	}

	// Collect rows
	type row struct {
		qualifiedName string
		module        string
		name          string
		excluded      bool
		folderPath    string
		title         string
		url           string
		params        int
	}
	var rows []row

	for _, p := range pages {
		modID := h.FindModuleID(p.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName == "" || modName == moduleName {
			qualifiedName := modName + "." + p.Name
			folderPath := h.BuildFolderPath(p.ContainerID)
			title := ""
			if p.Title != nil {
				// Try to get English title first, then any available translation
				title = p.Title.GetTranslation("en_US")
				if title == "" {
					for _, t := range p.Title.Translations {
						title = t
						break
					}
				}
			}
			url := p.URL

			rows = append(rows, row{qualifiedName, modName, p.Name, p.Excluded, folderPath, title, url, len(p.Parameters)})
		}
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Excluded", "Folder", "Title", "url", "Params"},
		Summary: fmt.Sprintf("(%d pages)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.module, r.name, r.excluded, r.folderPath, r.title, r.url, r.params})
	}
	return writeResult(ctx, result)
}
