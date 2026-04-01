// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/mpr"
)

// showExportMappings prints a table of all export mapping documents.
func (e *Executor) showExportMappings(inModule string) error {
	if e.reader == nil {
		return fmt.Errorf("not connected to a project")
	}

	all, err := e.reader.ListExportMappings()
	if err != nil {
		return fmt.Errorf("failed to list export mappings: %w", err)
	}

	h, err := e.getHierarchy()
	if err != nil {
		return err
	}

	type row struct {
		module, qualifiedName, name, schemaSource string
		elementCount                              int
	}
	var rows []row
	modWidth, qnWidth, nameWidth, srcWidth := len("Module"), len("QualifiedName"), len("Name"), len("Schema Source")

	for _, em := range all {
		modID := h.FindModuleID(em.ContainerID)
		moduleName := h.GetModuleName(modID)
		if inModule != "" && !strings.EqualFold(moduleName, inModule) {
			continue
		}
		qn := moduleName + "." + em.Name
		src := em.JsonStructure
		if src == "" {
			src = em.XmlSchema
		}
		if src == "" {
			src = em.MessageDefinition
		}
		if src == "" {
			src = "(none)"
		}
		r := row{
			module:        moduleName,
			qualifiedName: qn,
			name:          em.Name,
			schemaSource:  src,
			elementCount:  len(em.Elements),
		}
		if len(moduleName) > modWidth {
			modWidth = len(moduleName)
		}
		if len(qn) > qnWidth {
			qnWidth = len(qn)
		}
		if len(em.Name) > nameWidth {
			nameWidth = len(em.Name)
		}
		if len(src) > srcWidth {
			srcWidth = len(src)
		}
		rows = append(rows, r)
	}

	if len(rows) == 0 {
		if inModule != "" {
			fmt.Fprintf(e.output, "No export mappings found in module %s\n", inModule)
		} else {
			fmt.Fprintln(e.output, "No export mappings found")
		}
		return nil
	}

	fmt.Fprintf(e.output, "%-*s  %-*s  %-*s  %-*s  %s\n",
		modWidth, "Module", qnWidth, "QualifiedName", nameWidth, "Name", srcWidth, "Schema Source", "Elements")
	fmt.Fprintf(e.output, "%s  %s  %s  %s  %s\n",
		strings.Repeat("-", modWidth), strings.Repeat("-", qnWidth), strings.Repeat("-", nameWidth),
		strings.Repeat("-", srcWidth), strings.Repeat("-", 8))
	for _, r := range rows {
		fmt.Fprintf(e.output, "%-*s  %-*s  %-*s  %-*s  %d\n",
			modWidth, r.module, qnWidth, r.qualifiedName, nameWidth, r.name, srcWidth, r.schemaSource, r.elementCount)
	}
	return nil
}

// describeExportMapping prints the MDL representation of an export mapping.
func (e *Executor) describeExportMapping(name ast.QualifiedName) error {
	if e.reader == nil {
		return fmt.Errorf("not connected to a project")
	}

	em, err := e.reader.GetExportMappingByQualifiedName(name.Module, name.Name)
	if err != nil {
		return fmt.Errorf("export mapping %s not found", name)
	}

	if em.Documentation != "" {
		fmt.Fprintf(e.output, "/**\n * %s\n */\n", strings.ReplaceAll(em.Documentation, "\n", "\n * "))
	}

	h, err := e.getHierarchy()
	if err != nil {
		return err
	}
	modID := h.FindModuleID(em.ContainerID)
	moduleName := h.GetModuleName(modID)

	fmt.Fprintf(e.output, "CREATE EXPORT MAPPING %s.%s\n", moduleName, em.Name)

	if em.JsonStructure != "" {
		fmt.Fprintf(e.output, "  TO JSON STRUCTURE %s\n", em.JsonStructure)
	} else if em.XmlSchema != "" {
		fmt.Fprintf(e.output, "  TO XML SCHEMA %s\n", em.XmlSchema)
	}

	if em.NullValueOption != "" && em.NullValueOption != "LeaveOutElement" {
		fmt.Fprintf(e.output, "  NULL VALUES %s\n", em.NullValueOption)
	}

	if len(em.Elements) > 0 {
		fmt.Fprintln(e.output, "{")
		for _, elem := range em.Elements {
			printExportMappingElement(e, elem, 1)
			fmt.Fprintln(e.output, ";")
		}
		fmt.Fprintln(e.output, "};")
	}
	return nil
}

func printExportMappingElement(e *Executor, elem *model.ExportMappingElement, depth int) {
	indent := strings.Repeat("  ", depth)
	if elem.Kind == "Object" {
		via := ""
		if elem.Association != "" {
			via = " VIA " + elem.Association
		}
		if len(elem.Children) > 0 {
			fmt.Fprintf(e.output, "%s%s%s -> %s {\n", indent, elem.Entity, via, elem.ExposedName)
			for _, child := range elem.Children {
				printExportMappingElement(e, child, depth+1)
				fmt.Fprintln(e.output, ";")
			}
			fmt.Fprintf(e.output, "%s}", indent)
		} else {
			fmt.Fprintf(e.output, "%s%s%s -> %s", indent, elem.Entity, via, elem.ExposedName)
		}
	} else {
		// Value mapping
		attrName := elem.Attribute
		// Strip module prefix if present (Module.Entity.Attr → Attr)
		if parts := strings.Split(attrName, "."); len(parts) == 3 {
			attrName = parts[2]
		}
		dt := elem.DataType
		if dt == "" {
			dt = "String"
		}
		fmt.Fprintf(e.output, "%s%s -> %s (%s)", indent, attrName, elem.ExposedName, dt)
	}
}

// execCreateExportMapping creates a new export mapping.
func (e *Executor) execCreateExportMapping(s *ast.CreateExportMappingStmt) error {
	if e.writer == nil {
		return fmt.Errorf("not connected to a project in write mode")
	}

	module, err := e.findModule(s.Name.Module)
	if err != nil {
		return fmt.Errorf("module %s not found", s.Name.Module)
	}
	containerID := module.ID

	em := &model.ExportMapping{
		ContainerID:     containerID,
		Name:            s.Name.Name,
		ExportLevel:     "Hidden",
		NullValueOption: s.NullValueOption,
	}
	if em.NullValueOption == "" {
		em.NullValueOption = "LeaveOutElement"
	}

	// Set schema source reference
	switch s.SchemaKind {
	case "JSON_STRUCTURE":
		em.JsonStructure = s.SchemaRef.String()
	case "XML_SCHEMA":
		em.XmlSchema = s.SchemaRef.String()
	}

	// Build a path→ElementType map from the JSON structure so we can compute correct JsonPaths.
	// Array elements in the JSON structure require |(Object) appended to the entity's JsonPath.
	jsPathTypes := map[string]string{}
	if s.SchemaKind == "JSON_STRUCTURE" && s.SchemaRef.Module != "" {
		if js, err2 := e.reader.GetJsonStructureByQualifiedName(s.SchemaRef.Module, s.SchemaRef.Name); err2 == nil {
			buildJsonPathTypeMap(js.Elements, jsPathTypes)
		}
	}

	// Build element tree from the AST definition
	if s.RootElement != nil {
		root := buildExportMappingElementModel(s.Name.Module, s.RootElement, "", "(Object)", jsPathTypes)
		em.Elements = append(em.Elements, root)
	}

	if err := e.writer.CreateExportMapping(em); err != nil {
		return fmt.Errorf("failed to create export mapping: %w", err)
	}

	if !e.quiet {
		fmt.Fprintf(e.output, "Created export mapping %s.%s\n", s.Name.Module, s.Name.Name)
	}
	return nil
}

// buildJsonPathTypeMap recursively walks a JSON structure element tree and populates
// a map of JSON path → ElementType ("Object", "Array", "Value").
func buildJsonPathTypeMap(elems []*model.JsonElement, m map[string]string) {
	for _, e := range elems {
		if e == nil {
			continue
		}
		m[e.Path] = e.ElementType
		buildJsonPathTypeMap(e.Children, m)
	}
}

// buildExportMappingElementModel converts an AST element definition to a model element.
// parentEntity is the fully-qualified entity name of the enclosing object element (for
// qualifying attribute names). parentPath is the JSON path of the parent element.
// jsPathTypes maps JSON structure paths to their ElementType ("Array"/"Object"/"Value").
func buildExportMappingElementModel(moduleName string, def *ast.ExportMappingElementDef, parentEntity, parentPath string, jsPathTypes map[string]string) *model.ExportMappingElement {
	elem := &model.ExportMappingElement{
		BaseElement: model.BaseElement{
			ID:       model.ID(mpr.GenerateID()),
			TypeName: "ExportMappings$ObjectMappingElement",
		},
		ExposedName: def.JsonName,
	}

	if def.Entity != "" {
		// Object mapping
		elem.Kind = "Object"
		entity := def.Entity
		if !strings.Contains(entity, ".") {
			entity = moduleName + "." + entity
		}
		elem.Entity = entity
		if def.Association != "" {
			assoc := def.Association
			if !strings.Contains(assoc, ".") {
				assoc = moduleName + "." + assoc
			}
			elem.Association = assoc
		}

		// Compute JsonPath using the JSON structure type map.
		// Root entity (parentPath == "(Object)" and no association): maps to "(Object)".
		// Other entities: look up parentPath+"|"+ExposedName; if Array → append "|(Object)".
		var jsonPath string
		if elem.Association == "" {
			// Root entity — always maps to the JSON structure root
			jsonPath = parentPath // "(Object)"
		} else {
			candidatePath := parentPath + "|" + def.JsonName
			if jsPathTypes[candidatePath] == "Array" {
				jsonPath = candidatePath + "|(Object)"
			} else {
				jsonPath = candidatePath
			}
		}
		elem.JsonPath = jsonPath

		for _, child := range def.Children {
			elem.Children = append(elem.Children, buildExportMappingElementModel(moduleName, child, entity, jsonPath, jsPathTypes))
		}
	} else {
		// Value mapping — qualify attribute name as Module.Entity.Attribute
		elem.Kind = "Value"
		elem.TypeName = "ExportMappings$ValueMappingElement"
		elem.DataType = def.DataType
		if elem.DataType == "" {
			elem.DataType = "String"
		}
		attr := def.Attribute
		if parentEntity != "" && !strings.Contains(attr, ".") {
			attr = parentEntity + "." + attr
		}
		elem.Attribute = attr
		elem.JsonPath = parentPath + "|" + def.JsonName
	}

	return elem
}

// execDropExportMapping deletes an export mapping.
func (e *Executor) execDropExportMapping(s *ast.DropExportMappingStmt) error {
	if e.writer == nil {
		return fmt.Errorf("not connected to a project in write mode")
	}

	em, err := e.reader.GetExportMappingByQualifiedName(s.Name.Module, s.Name.Name)
	if err != nil {
		return fmt.Errorf("export mapping %s not found", s.Name)
	}

	if err := e.writer.DeleteExportMapping(em.ID); err != nil {
		return fmt.Errorf("failed to drop export mapping: %w", err)
	}

	if !e.quiet {
		fmt.Fprintf(e.output, "Dropped export mapping %s.%s\n", s.Name.Module, s.Name.Name)
	}
	return nil
}
