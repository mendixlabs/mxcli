// SPDX-License-Identifier: Apache-2.0

// Package executor - JSON structure commands (SHOW/DESCRIBE/CREATE/DROP JSON STRUCTURE)
package executor

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/mpr"
)

// showJsonStructures handles SHOW JSON STRUCTURES [IN module].
func (e *Executor) showJsonStructures(moduleName string) error {
	structures, err := e.reader.ListJsonStructures()
	if err != nil {
		return fmt.Errorf("failed to list JSON structures: %w", err)
	}

	h, err := e.getHierarchy()
	if err != nil {
		return err
	}

	fmt.Fprintf(e.output, "| %-40s | %-8s | %-12s |\n", "JSON Structure", "Elements", "Source")
	fmt.Fprintf(e.output, "|%-42s|%-10s|%-14s|\n", strings.Repeat("-", 42), strings.Repeat("-", 10), strings.Repeat("-", 14))

	count := 0
	for _, js := range structures {
		modID := h.FindModuleID(js.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && modName != moduleName {
			continue
		}

		qualifiedName := fmt.Sprintf("%s.%s", modName, js.Name)

		// Count top-level elements (children of root, or all elements if no root)
		elemCount := 0
		if len(js.Elements) > 0 {
			root := js.Elements[0]
			elemCount = len(root.Children)
		}

		source := "Manual"
		if js.JsonSnippet != "" {
			source = "JSON Snippet"
		}

		fmt.Fprintf(e.output, "| %-40s | %8d | %-12s |\n", qualifiedName, elemCount, source)
		count++
	}

	fmt.Fprintf(e.output, "\n(%d JSON structure(s))\n", count)
	return nil
}

// describeJsonStructure handles DESCRIBE JSON STRUCTURE Module.Name.
// Output is re-executable CREATE OR REPLACE MDL followed by the element tree as comments.
func (e *Executor) describeJsonStructure(name ast.QualifiedName) error {
	js := e.findJsonStructure(name.Module, name.Name)
	if js == nil {
		return fmt.Errorf("JSON structure not found: %s", name)
	}

	h, err := e.getHierarchy()
	if err != nil {
		return err
	}
	modID := h.FindModuleID(js.ContainerID)
	modName := h.GetModuleName(modID)

	qualifiedName := fmt.Sprintf("%s.%s", modName, js.Name)

	// Documentation as doc comment
	if js.Documentation != "" {
		fmt.Fprintf(e.output, "/**\n * %s\n */\n", js.Documentation)
	}

	// Re-executable CREATE OR REPLACE statement
	fmt.Fprintf(e.output, "CREATE OR REPLACE JSON STRUCTURE %s", qualifiedName)
	if js.Documentation != "" {
		fmt.Fprintf(e.output, "\n  COMMENT '%s'", strings.ReplaceAll(js.Documentation, "'", "''"))
	}

	if js.JsonSnippet != "" {
		snippet := mpr.PrettyPrintJSON(js.JsonSnippet)
		if strings.Contains(snippet, "'") || strings.Contains(snippet, "\n") {
			fmt.Fprintf(e.output, "\n  SNIPPET $$%s$$", snippet)
		} else {
			fmt.Fprintf(e.output, "\n  SNIPPET '%s'", snippet)
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

		fmt.Fprintf(e.output, "\n  CUSTOM NAME MAP (\n")
		for i, jsonKey := range keys {
			sep := ","
			if i == len(keys)-1 {
				sep = ""
			}
			fmt.Fprintf(e.output, "    '%s' AS '%s'%s\n", jsonKey, customMappings[jsonKey], sep)
		}
		fmt.Fprintf(e.output, "  )")
	}

	fmt.Fprintln(e.output, ";")

	// Element tree as informational comments
	fmt.Fprintln(e.output)
	fmt.Fprintln(e.output, "-- Element tree:")
	for _, elem := range js.Elements {
		renderJsonElementComment(e.output, elem, 0)
	}

	fmt.Fprintln(e.output, "/")
	return nil
}

// renderJsonElementComment renders an element tree as `-- ` prefixed comment lines.
func renderJsonElementComment(w io.Writer, elem *mpr.JsonElement, depth int) {
	indent := strings.Repeat("  ", depth)

	// Determine type display
	typeStr := elem.ElementType
	if elem.ElementType == "Value" {
		typeStr = elem.PrimitiveType
	}

	// Show occurrence bounds for arrays
	suffix := ""
	if elem.MaxOccurs != 1 {
		maxStr := fmt.Sprintf("%d", elem.MaxOccurs)
		if elem.MaxOccurs == -1 {
			maxStr = "*"
		}
		suffix = fmt.Sprintf("[%d..%s]", elem.MinOccurs, maxStr)
	}

	fmt.Fprintf(w, "-- %s%s: %s%s\n", indent, elem.ExposedName, typeStr, suffix)

	for _, child := range elem.Children {
		renderJsonElementComment(w, child, depth+1)
	}
}

// collectCustomNameMappings walks the element tree and returns JSON key → ExposedName
// mappings where the ExposedName differs from the auto-generated default (capitalizeFirst).
func collectCustomNameMappings(elements []*mpr.JsonElement) map[string]string {
	mappings := make(map[string]string)
	for _, elem := range elements {
		collectCustomNames(elem, mappings)
	}
	return mappings
}

func collectCustomNames(elem *mpr.JsonElement, mappings map[string]string) {
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
func (e *Executor) execCreateJsonStructure(s *ast.CreateJsonStructureStmt) error {
	if e.reader == nil {
		return fmt.Errorf("not connected to a project")
	}

	// Find or auto-create module
	module, err := e.findOrCreateModule(s.Name.Module)
	if err != nil {
		return err
	}

	// Check if already exists
	existing := e.findJsonStructure(s.Name.Module, s.Name.Name)
	if existing != nil {
		if s.CreateOrReplace {
			// Delete existing before recreating
			if err := e.writer.DeleteJsonStructure(string(existing.ID)); err != nil {
				return fmt.Errorf("failed to delete existing JSON structure: %w", err)
			}
		} else {
			return fmt.Errorf("JSON structure already exists: %s.%s", s.Name.Module, s.Name.Name)
		}
	}

	// Build element tree from JSON snippet, applying custom name mappings
	elements, err := mpr.BuildJsonElementsFromSnippet(s.JsonSnippet, s.CustomNameMap)
	if err != nil {
		return fmt.Errorf("failed to build element tree: %w", err)
	}

	js := &mpr.JsonStructure{
		ContainerID:   module.ID,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		JsonSnippet:   mpr.PrettyPrintJSON(s.JsonSnippet),
		Elements:      elements,
	}

	if err := e.writer.CreateJsonStructure(js); err != nil {
		return fmt.Errorf("failed to create JSON structure: %w", err)
	}

	// Invalidate hierarchy cache
	e.invalidateHierarchy()

	action := "Created"
	if existing != nil {
		action = "Replaced"
	}
	fmt.Fprintf(e.output, "%s JSON structure: %s\n", action, s.Name)
	return nil
}

// execDropJsonStructure handles DROP JSON STRUCTURE statements.
func (e *Executor) execDropJsonStructure(s *ast.DropJsonStructureStmt) error {
	if e.reader == nil {
		return fmt.Errorf("not connected to a project")
	}

	js := e.findJsonStructure(s.Name.Module, s.Name.Name)
	if js == nil {
		return fmt.Errorf("JSON structure not found: %s", s.Name)
	}

	if err := e.writer.DeleteJsonStructure(string(js.ID)); err != nil {
		return fmt.Errorf("failed to delete JSON structure: %w", err)
	}

	fmt.Fprintf(e.output, "Dropped JSON structure: %s\n", s.Name)
	return nil
}

// findJsonStructure finds a JSON structure by module and name.
func (e *Executor) findJsonStructure(moduleName, structName string) *mpr.JsonStructure {
	structures, err := e.reader.ListJsonStructures()
	if err != nil {
		return nil
	}

	h, _ := e.getHierarchy()
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
