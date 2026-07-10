// SPDX-License-Identifier: Apache-2.0

// Package executor - JSON structure commands (SHOW/DESCRIBE/CREATE/DROP JSON STRUCTURE)
package executor

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// listJsonStructures handles SHOW JSON STRUCTURES [IN module].
func listJsonStructures(ctx *ExecContext, moduleName string) error {
	structures, err := ctx.Backend.ListJsonStructures()
	if err != nil {
		return mdlerrors.NewBackend("list json structures", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	type row struct {
		qualifiedName string
		elemCount     int
		source        string
	}
	var rows []row

	for _, js := range structures {
		modID := h.FindModuleID(js.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && modName != moduleName {
			continue
		}

		qualifiedName := fmt.Sprintf("%s.%s", modName, js.Name)

		elemCount := 0
		if len(js.Elements) > 0 {
			elemCount = len(js.Elements[0].Children)
		}

		source := "Manual"
		if js.JsonSnippet != "" {
			source = "json Snippet"
		}

		rows = append(rows, row{qualifiedName: qualifiedName, elemCount: elemCount, source: source})
	}

	// Sort alphabetically
	sort.Slice(rows, func(i, j int) bool { return rows[i].qualifiedName < rows[j].qualifiedName })

	tr := &TableResult{
		Columns: []string{"json Structure", "Elements", "Source"},
		Summary: fmt.Sprintf("(%d json structure(s))", len(rows)),
	}
	for _, r := range rows {
		tr.Rows = append(tr.Rows, []any{r.qualifiedName, r.elemCount, r.source})
	}
	return writeResult(ctx, tr)
}

// describeJsonStructure handles DESCRIBE JSON STRUCTURE Module.Name.
// Output is re-executable CREATE OR REPLACE MDL followed by the element tree as comments.
func describeJsonStructure(ctx *ExecContext, name ast.QualifiedName) error {
	js := findJsonStructure(ctx, name.Module, name.Name)
	if js == nil {
		return mdlerrors.NewNotFound("json structure", name.String())
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	modID := h.FindModuleID(js.ContainerID)
	modName := h.GetModuleName(modID)

	qualifiedName := fmt.Sprintf("%s.%s", modName, js.Name)

	// Documentation as doc comment
	if js.Documentation != "" {
		fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", js.Documentation)
	}

	// Re-executable CREATE OR MODIFY statement
	fmt.Fprintf(ctx.Output, "create or modify json structure %s", qualifiedName)
	if folderPath := h.BuildFolderPath(js.ContainerID); folderPath != "" {
		fmt.Fprintf(ctx.Output, "\n  folder '%s'", folderPath)
	}
	if js.Documentation != "" {
		fmt.Fprintf(ctx.Output, "\n  comment '%s'", strings.ReplaceAll(js.Documentation, "'", "''"))
	}

	if js.JsonSnippet != "" {
		snippet := types.PrettyPrintJSON(js.JsonSnippet)
		if strings.Contains(snippet, "'") || strings.Contains(snippet, "\n") {
			fmt.Fprintf(ctx.Output, "\n  snippet $$%s$$", snippet)
		} else {
			fmt.Fprintf(ctx.Output, "\n  snippet '%s'", snippet)
		}
	}

	// Detect custom name mappings by comparing ExposedName to auto-generated names
	customMappings := collectCustomNameMappings(js.Elements)
	if len(customMappings) > 0 {
		// Sort keys for deterministic DESCRIBE output
		keys := make([]string, 0, len(customMappings))
		for k := range customMappings {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		fmt.Fprintf(ctx.Output, "\n  CUSTOM NAME map (\n")
		for i, jsonKey := range keys {
			sep := ","
			if i == len(keys)-1 {
				sep = ""
			}
			fmt.Fprintf(ctx.Output, "    '%s' as '%s'%s\n", jsonKey, customMappings[jsonKey], sep)
		}
		fmt.Fprintf(ctx.Output, "  )")
	}

	fmt.Fprintln(ctx.Output, ";")
	return nil
}

// collectCustomNameMappings walks the element tree and returns JSON key → ExposedName
// mappings where the ExposedName differs from the auto-generated default (capitalizeFirst).
func collectCustomNameMappings(elements []*types.JsonElement) map[string]string {
	mappings := make(map[string]string)
	for _, elem := range elements {
		collectCustomNames(elem, mappings)
	}
	return mappings
}

func collectCustomNames(elem *types.JsonElement, mappings map[string]string) {
	// Extract the JSON key from the last segment of the Path.
	// Path format: "(Object)|fieldName" or "(Object)|parent|(Object)|child"
	if parts := strings.Split(elem.Path, "|"); len(parts) > 1 {
		jsonKey := parts[len(parts)-1]
		// Skip structural markers like (Object), (Array)
		if jsonKey != "" && jsonKey[0] != '(' {
			expected := capitalizeFirstRune(jsonKey)
			if elem.ExposedName != expected && elem.ExposedName != "" {
				mappings[jsonKey] = elem.ExposedName
			}
		}
	}
	for _, child := range elem.Children {
		collectCustomNames(child, mappings)
	}
}

// capitalizeFirstRune capitalizes the first rune of s (for ExposedName comparison).
func capitalizeFirstRune(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// execCreateJsonStructure handles CREATE [OR REPLACE] JSON STRUCTURE statements.
func execCreateJsonStructure(ctx *ExecContext, s *ast.CreateJsonStructureStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	// Resolve folder if specified
	containerID := module.ID
	if s.Folder != "" {
		folderID, err := resolveFolder(ctx, module.ID, s.Folder)
		if err != nil {
			return mdlerrors.NewBackend("resolve folder "+s.Folder, err)
		}
		containerID = folderID
	}

	// Check if already exists
	existing := findJsonStructure(ctx, s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("json structure", s.Name.Module+"."+s.Name.Name)
	}

	// Build element tree from JSON snippet, applying custom name mappings
	elements, err := types.BuildJsonElementsFromSnippet(s.JsonSnippet, s.CustomNameMap)
	if err != nil {
		return mdlerrors.NewBackend("build element tree", err)
	}

	// On OR MODIFY, keep original folder unless a new one is explicitly specified
	if existing != nil && s.Folder == "" {
		containerID = existing.ContainerID
	}

	js := &types.JsonStructure{
		ContainerID:   containerID,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		JsonSnippet:   types.PrettyPrintJSON(s.JsonSnippet),
		Elements:      elements,
	}

	if existing != nil {
		js.ID = existing.ID
		if err := ctx.Backend.UpdateJsonStructure(js); err != nil {
			return mdlerrors.NewBackend("update json structure", err)
		}
		fmt.Fprintf(ctx.Output, "Modified json structure: %s\n", s.Name)
	} else {
		if err := ctx.Backend.CreateJsonStructure(js); err != nil {
			return mdlerrors.NewBackend("create json structure", err)
		}
		fmt.Fprintf(ctx.Output, "Created json structure: %s\n", s.Name)
	}

	// Invalidate hierarchy cache
	invalidateHierarchy(ctx)
	return nil
}

// execDropJsonStructure handles DROP JSON STRUCTURE statements.
func execDropJsonStructure(ctx *ExecContext, s *ast.DropJsonStructureStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	js := findJsonStructure(ctx, s.Name.Module, s.Name.Name)
	if js == nil {
		return mdlerrors.NewNotFound("json structure", s.Name.String())
	}

	if err := ctx.Backend.DeleteJsonStructure(string(js.ID)); err != nil {
		return mdlerrors.NewBackend("delete json structure", err)
	}

	fmt.Fprintf(ctx.Output, "Dropped json structure: %s\n", s.Name)
	return nil
}

// findJsonStructure finds a JSON structure by module and name.
func findJsonStructure(ctx *ExecContext, moduleName, structName string) *types.JsonStructure {
	structures, err := ctx.Backend.ListJsonStructures()
	if err != nil {
		return nil
	}

	h, _ := getHierarchy(ctx)
	if h == nil {
		return nil
	}

	for _, js := range structures {
		modID := h.FindModuleID(js.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == moduleName && js.Name == structName {
			return js
		}
	}
	return nil
}
