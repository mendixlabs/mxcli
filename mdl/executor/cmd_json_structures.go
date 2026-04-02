// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/mpr"
)

// showJsonStructures prints a table of all JSON structure documents.
func (e *Executor) showJsonStructures(inModule string) error {
	if e.reader == nil {
		return fmt.Errorf("not connected to a project")
	}

	all, err := e.reader.ListJsonStructures()
	if err != nil {
		return fmt.Errorf("failed to list JSON structures: %w", err)
	}

	h, err := e.getHierarchy()
	if err != nil {
		return err
	}

	type row struct {
		module, qualifiedName, name string
		elementCount                int
	}
	var rows []row
	modWidth, qnWidth, nameWidth := len("Module"), len("QualifiedName"), len("Name")

	for _, js := range all {
		modID := h.FindModuleID(js.ContainerID)
		moduleName := h.GetModuleName(modID)
		if inModule != "" && !strings.EqualFold(moduleName, inModule) {
			continue
		}
		qn := moduleName + "." + js.Name
		r := row{
			module:        moduleName,
			qualifiedName: qn,
			name:          js.Name,
			elementCount:  len(js.Elements),
		}
		if len(moduleName) > modWidth {
			modWidth = len(moduleName)
		}
		if len(qn) > qnWidth {
			qnWidth = len(qn)
		}
		if len(js.Name) > nameWidth {
			nameWidth = len(js.Name)
		}
		rows = append(rows, r)
	}

	if len(rows) == 0 {
		if inModule != "" {
			fmt.Fprintf(e.output, "No JSON structures found in module %s\n", inModule)
		} else {
			fmt.Fprintln(e.output, "No JSON structures found")
		}
		return nil
	}

	fmt.Fprintf(e.output, "%-*s  %-*s  %-*s  %s\n", modWidth, "Module", qnWidth, "QualifiedName", nameWidth, "Name", "Elements")
	fmt.Fprintf(e.output, "%s  %s  %s  %s\n", strings.Repeat("-", modWidth), strings.Repeat("-", qnWidth), strings.Repeat("-", nameWidth), strings.Repeat("-", 8))
	for _, r := range rows {
		fmt.Fprintf(e.output, "%-*s  %-*s  %-*s  %d\n", modWidth, r.module, qnWidth, r.qualifiedName, nameWidth, r.name, r.elementCount)
	}
	return nil
}

// describeJsonStructure prints the MDL representation of a JSON structure.
func (e *Executor) describeJsonStructure(name ast.QualifiedName) error {
	if e.reader == nil {
		return fmt.Errorf("not connected to a project")
	}

	js, err := e.reader.GetJsonStructureByQualifiedName(name.Module, name.Name)
	if err != nil {
		return fmt.Errorf("JSON structure %s not found", name)
	}

	if js.Documentation != "" {
		fmt.Fprintf(e.output, "/**\n * %s\n */\n", strings.ReplaceAll(js.Documentation, "\n", "\n * "))
	}

	h, err := e.getHierarchy()
	if err != nil {
		return err
	}
	modID := h.FindModuleID(js.ContainerID)
	moduleName := h.GetModuleName(modID)

	fmt.Fprintf(e.output, "CREATE JSON STRUCTURE %s.%s\n", moduleName, js.Name)
	if js.JsonSnippet != "" {
		fmt.Fprintf(e.output, "  FROM '%s';\n", strings.ReplaceAll(js.JsonSnippet, "'", "''"))
	} else {
		fmt.Fprintln(e.output, "  FROM '{}'  -- no snippet stored;")
	}

	if len(js.Elements) > 0 {
		fmt.Fprintln(e.output, "\n-- Element tree:")
		for _, elem := range js.Elements {
			printJsonElement(e, elem, 0)
		}
	}
	return nil
}

func printJsonElement(e *Executor, elem *model.JsonElement, depth int) {
	indent := strings.Repeat("  ", depth)
	typeInfo := elem.ElementType
	if elem.PrimitiveType != "" {
		typeInfo = elem.PrimitiveType
	}
	fmt.Fprintf(e.output, "-- %s%s: %s\n", indent, elem.ExposedName, typeInfo)
	for _, child := range elem.Children {
		printJsonElement(e, child, depth+1)
	}
}

// execCreateJsonStructure creates a new JSON structure from a JSON snippet.
func (e *Executor) execCreateJsonStructure(s *ast.CreateJsonStructureStmt) error {
	if e.writer == nil {
		return fmt.Errorf("not connected to a project in write mode")
	}

	module, err := e.findModule(s.Name.Module)
	if err != nil {
		return fmt.Errorf("module %s not found", s.Name.Module)
	}

	containerID := module.ID
	if s.Folder != "" {
		folderID, err := e.resolveFolder(module.ID, s.Folder)
		if err != nil {
			return fmt.Errorf("failed to resolve folder %s: %w", s.Folder, err)
		}
		containerID = folderID
	}

	// Step 1: Format (pretty-print) the snippet, matching Studio Pro's "Format" button
	formattedSnippet, err := mpr.FormatJsonSnippet(s.JsonSnippet)
	if err != nil {
		return fmt.Errorf("failed to format JSON snippet: %w", err)
	}

	// Step 2: Refresh — derive the element tree from the formatted snippet
	elements, err := mpr.DeriveJsonElementsFromSnippet(formattedSnippet)
	if err != nil {
		return fmt.Errorf("failed to parse JSON snippet: %w", err)
	}

	js := &model.JsonStructure{
		ContainerID: containerID,
		Name:        s.Name.Name,
		JsonSnippet: formattedSnippet,
		Elements:    elements,
		ExportLevel: "Hidden",
	}

	if err := e.writer.CreateJsonStructure(js); err != nil {
		return fmt.Errorf("failed to create JSON structure: %w", err)
	}

	if !e.quiet {
		fmt.Fprintf(e.output, "Created JSON structure %s.%s\n", s.Name.Module, s.Name.Name)
	}
	return nil
}

// execDropJsonStructure deletes a JSON structure.
func (e *Executor) execDropJsonStructure(s *ast.DropJsonStructureStmt) error {
	if e.writer == nil {
		return fmt.Errorf("not connected to a project in write mode")
	}

	js, err := e.reader.GetJsonStructureByQualifiedName(s.Name.Module, s.Name.Name)
	if err != nil {
		return fmt.Errorf("JSON structure %s not found", s.Name)
	}

	if err := e.writer.DeleteJsonStructure(js.ID); err != nil {
		return fmt.Errorf("failed to drop JSON structure: %w", err)
	}

	if !e.quiet {
		fmt.Fprintf(e.output, "Dropped JSON structure %s.%s\n", s.Name.Module, s.Name.Name)
	}
	return nil
}
