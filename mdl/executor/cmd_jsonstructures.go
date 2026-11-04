// SPDX-License-Identifier: Apache-2.0

// Package executor - JSON structure commands (SHOW/DESCRIBE/CREATE/DROP JSON STRUCTURE)
package executor

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
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

	// Documentation as doc comment (R9). A text with `*/` in it would end the
	// doc comment early, so it keeps the deprecated `comment '…'` clause.
	docClause := false
	if js.Documentation != "" {
		if !strings.Contains(js.Documentation, "*/") {
			fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", strings.ReplaceAll(js.Documentation, "\n", "\n * "))
		} else {
			docClause = true
		}
	}

	// Re-executable CREATE OR MODIFY statement
	fmt.Fprintf(ctx.Output, "create or modify json structure %s", qualifiedName)
	if folderPath := h.BuildFolderPath(js.ContainerID); folderPath != "" {
		fmt.Fprintf(ctx.Output, "\n  folder %s", mdlQuote(ctx, folderPath))
	}
	if docClause {
		fmt.Fprintf(ctx.Output, "\n  comment '%s'", strings.ReplaceAll(js.Documentation, "'", "''"))
	}

	if js.JsonSnippet != "" {
		snippet := types.PrettyPrintJSON(js.JsonSnippet)
		if strings.Contains(snippet, "'") || strings.Contains(snippet, "\n") {
			fmt.Fprintf(ctx.Output, "\n  sample %s", mdlDollarQuote(ctx, snippet))
		} else {
			fmt.Fprintf(ctx.Output, "\n  sample '%s'", snippet)
		}
	}

	// Detect custom name mappings by comparing ExposedName to auto-generated names
	customMappings := collectCustomNameMappings(js.Elements)
	itemMappings := collectCustomItemNames(js.Elements)
	if len(customMappings) > 0 || len(itemMappings) > 0 {
		// Sort keys for deterministic DESCRIBE output
		keys := sortedMapKeys(customMappings)
		itemKeys := sortedMapKeys(itemMappings)
		last := len(keys) + len(itemKeys)

		fmt.Fprintf(ctx.Output, "\n  CUSTOM NAME map (\n")
		n := 0
		for _, jsonKey := range keys {
			n++
			fmt.Fprintf(ctx.Output, "    '%s' as '%s'%s\n", jsonKey, customMappings[jsonKey], commaUnlessLast(n, last))
		}
		// `item of` entries come last so the plain renames stay grouped; both
		// orders re-execute identically.
		for _, jsonKey := range itemKeys {
			n++
			fmt.Fprintf(ctx.Output, "    item of '%s' as '%s'%s\n", jsonKey, itemMappings[jsonKey], commaUnlessLast(n, last))
		}
		fmt.Fprintf(ctx.Output, "  )")
	}

	fmt.Fprintln(ctx.Output, ";")
	return nil
}

// sortedMapKeys returns a map's keys in a deterministic order, which DESCRIBE
// output depends on.
func sortedMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func commaUnlessLast(n, last int) string {
	if n == last {
		return ""
	}
	return ","
}

// collectCustomItemNames returns array JSON key → its ITEM element's ExposedName,
// for items whose name differs from the generated default.
//
// It exists because an item element has no JSON key, so collectCustomNames skips
// it (its path segment is the structural marker "(Object)" or "(Wrapper)"). Without
// this, a named item was written on CREATE and lost on DESCRIBE — so
// describe → exec silently renamed it back (ako/mxcli#272).
func collectCustomItemNames(elements []*types.JsonElement) map[string]string {
	out := make(map[string]string)
	for _, elem := range elements {
		collectItemNames(elem, out, true)
	}
	return out
}

func collectItemNames(elem *types.JsonElement, out map[string]string, isRoot bool) {
	if elem.ElementType == "Array" {
		key := arrayJsonKey(elem, isRoot)
		for _, child := range elem.Children {
			if key != "" && child.ExposedName != "" && child.ExposedName != defaultItemName(elem, isRoot) {
				out[key] = child.ExposedName
			}
		}
	}
	for _, child := range elem.Children {
		collectItemNames(child, out, false)
	}
}

// arrayJsonKey is the key `item of` addresses this array by: its own JSON key,
// or the root sentinel when the array IS the root and has none.
func arrayJsonKey(elem *types.JsonElement, isRoot bool) string {
	if isRoot {
		return types.RootArrayItemKey
	}
	parts := strings.Split(elem.Path, "|")
	last := parts[len(parts)-1]
	if last == "" || last[0] == '(' {
		return ""
	}
	return last
}

// defaultItemName is what the builder would have generated, so DESCRIBE emits an
// entry only for a name someone actually chose.
func defaultItemName(arrayElem *types.JsonElement, isRoot bool) string {
	if isRoot {
		return "JsonObject"
	}
	for _, child := range arrayElem.Children {
		if child.ElementType == "Wrapper" {
			return types.SingularizeExposedName(arrayElem.ExposedName)
		}
	}
	return arrayElem.ExposedName + "Item"
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
	elements, err := types.BuildJsonElementsFromSnippet(s.JsonSnippet, s.CustomNameMap, s.CustomItemNameMap)
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
	// Keep the stored snippet's FORMATTING when the content is the same. mxcli
	// pretty-prints on describe, so describe -> exec — how a document is copied
	// — otherwise rewrote a snippet Studio Pro had stored on one line into a
	// multi-line one. Same JSON, different bytes, and a diff against the
	// original for nothing (ako/mxcli#379).
	if existing != nil && sameJSONContent(existing.JsonSnippet, js.JsonSnippet) {
		js.JsonSnippet = existing.JsonSnippet
	}
	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if existing != nil {
		js.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, existing.Documentation)
	}
	if existing != nil {
		// Excluded is model state, not script state (#914).
		js.Excluded = existing.Excluded
	}

	if existing != nil {
		js.ID = existing.ID
		if err := ctx.Backend.UpdateJsonStructure(js); err != nil {
			return mdlerrors.NewBackend("update json structure", err)
		}
		if _, err := applyDocumentFolder(ctx, js.ID, existing.ContainerID, containerID); err != nil {
			return err
		}
		ctx.ReportMutation("Modified", "json structure: %s", s.Name)
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

// sameJSONContent reports whether two snippets carry the same JSON, ignoring
// whitespace. Comparing the decoded values rather than the strings is the point:
// the question is whether a rewrite would change anything that matters.
//
// Anything that does not parse is treated as different, so a malformed snippet
// is replaced rather than silently kept.
func sameJSONContent(a, b string) bool {
	if a == b {
		return true
	}
	var va, vb any
	if err := json.Unmarshal([]byte(a), &va); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &vb); err != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
}
