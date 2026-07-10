// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// listConstants handles SHOW CONSTANTS command.
func listConstants(ctx *ExecContext, moduleName string) error {
	constants, err := ctx.Backend.ListConstants()
	if err != nil {
		return mdlerrors.NewBackend("list constants", err)
	}

	// Use hierarchy for proper module resolution (handles constants inside folders)
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Collect rows
	type row struct {
		qualifiedName string
		module        string
		name          string
		folderPath    string
		typeStr       string
		defaultStr    string
		exposed       string
	}
	var rows []row

	for _, c := range constants {
		modID := h.FindModuleID(c.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && !strings.EqualFold(modName, moduleName) {
			continue
		}

		qualifiedName := modName + "." + c.Name
		folderPath := h.BuildFolderPath(c.ContainerID)
		typeStr := formatConstantType(c.Type)
		defaultStr := c.DefaultValue
		if len(defaultStr) > 40 {
			defaultStr = defaultStr[:37] + "..."
		}
		exposed := "No"
		if c.ExposedToClient {
			exposed = "Yes"
		}

		rows = append(rows, row{qualifiedName, modName, c.Name, folderPath, typeStr, defaultStr, exposed})
	}

	if len(rows) == 0 && ctx.Format != FormatJSON {
		fmt.Fprintln(ctx.Output, "No constants found.")
		return nil
	}

	// Sort by qualified name (module.name)
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Folder", "Type", "Default", "Exposed"},
		Summary: fmt.Sprintf("(%d constants)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.module, r.name, r.folderPath, r.typeStr, r.defaultStr, r.exposed})
	}
	return writeResult(ctx, result)
}

// describeConstant handles DESCRIBE CONSTANT command.
func describeConstant(ctx *ExecContext, name ast.QualifiedName) error {
	constants, err := ctx.Backend.ListConstants()
	if err != nil {
		return mdlerrors.NewBackend("list constants", err)
	}

	// Use hierarchy for proper module resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the constant
	for _, c := range constants {
		modID := h.FindModuleID(c.ContainerID)
		modName := h.GetModuleName(modID)
		if strings.EqualFold(modName, name.Module) && strings.EqualFold(c.Name, name.Name) {
			return outputConstantMDL(ctx, c, modName)
		}
	}

	return mdlerrors.NewNotFound("constant", name.String())
}

// outputConstantMDL outputs a constant definition in MDL format.
func outputConstantMDL(ctx *ExecContext, c *model.Constant, moduleName string) error {
	// Format default value based on type
	defaultValueStr := formatDefaultValue(c.Type, c.DefaultValue)

	fmt.Fprintf(ctx.Output, "create or modify constant %s.%s\n", moduleName, c.Name)
	fmt.Fprintf(ctx.Output, "  type %s\n", formatConstantTypeForMDL(c.Type))
	fmt.Fprintf(ctx.Output, "  default %s", defaultValueStr)

	// Add folder if present
	h, _ := getHierarchy(ctx)
	if h != nil {
		if folderPath := h.BuildFolderPath(c.ContainerID); folderPath != "" {
			fmt.Fprintf(ctx.Output, "\n  folder '%s'", folderPath)
		}
	}

	// Add options if present
	if c.Documentation != "" {
		escaped := strings.ReplaceAll(c.Documentation, "'", "''")
		fmt.Fprintf(ctx.Output, "\n  comment '%s'", escaped)
	}
	if c.ExposedToClient {
		fmt.Fprintf(ctx.Output, "\n  exposed to client")
	}

	fmt.Fprintln(ctx.Output, ";")
	fmt.Fprintln(ctx.Output, "/")

	return nil
}

// outputConstantMDL is an Executor method wrapper for callers not yet migrated.
func (e *Executor) outputConstantMDL(c *model.Constant, moduleName string) error {
	return outputConstantMDL(e.newExecContext(context.Background()), c, moduleName)
}

// formatConstantType returns a human-readable type string.
func formatConstantType(dt model.ConstantDataType) string {
	switch dt.Kind {
	case "String":
		return "String"
	case "Integer":
		return "Integer"
	case "Long":
		return "Long"
	case "Decimal":
		return "Decimal"
	case "Boolean":
		return "Boolean"
	case "DateTime":
		return "DateTime"
	case "Date":
		return "Date"
	case "Binary":
		return "Binary"
	case "Enumeration":
		if dt.EnumRef != "" {
			return fmt.Sprintf("Enumeration(%s)", dt.EnumRef)
		}
		return "Enumeration"
	case "Object":
		if dt.EntityRef != "" {
			return dt.EntityRef
		}
		return "Object"
	case "List":
		if dt.EntityRef != "" {
			return fmt.Sprintf("List of %s", dt.EntityRef)
		}
		return "List"
	default:
		if dt.Kind == "" {
			return "Unknown"
		}
		return dt.Kind
	}
}

// formatConstantTypeForMDL returns the MDL syntax for the type.
func formatConstantTypeForMDL(dt model.ConstantDataType) string {
	switch dt.Kind {
	case "String":
		return "String"
	case "Integer":
		return "Integer"
	case "Long":
		return "Long"
	case "Decimal":
		return "Decimal"
	case "Boolean":
		return "Boolean"
	case "DateTime":
		return "DateTime"
	case "Date":
		return "Date"
	case "Binary":
		return "Binary"
	case "Enumeration":
		if dt.EnumRef != "" {
			return fmt.Sprintf("Enumeration(%s)", dt.EnumRef)
		}
		return "Enumeration"
	default:
		if dt.Kind == "" {
			return "String"
		}
		return dt.Kind
	}
}

// formatDefaultValue formats the default value for MDL output.
func formatDefaultValue(dt model.ConstantDataType, value string) string {
	if value == "" {
		switch dt.Kind {
		case "String":
			return "''"
		case "Boolean":
			return "false"
		case "Integer", "Long", "Decimal":
			return "0"
		default:
			return "''"
		}
	}

	switch dt.Kind {
	case "String":
		// Quote the string value, escaping single quotes
		escaped := strings.ReplaceAll(value, "'", "''")
		return fmt.Sprintf("'%s'", escaped)
	case "Boolean":
		if strings.EqualFold(value, "true") {
			return "true"
		}
		return "false"
	case "Integer", "Long", "Decimal":
		return value
	case "Enumeration":
		// Enumeration values are stored as qualified names
		return value
	default:
		escaped := strings.ReplaceAll(value, "'", "''")
		return fmt.Sprintf("'%s'", escaped)
	}
}

// createConstant handles CREATE CONSTANT command.
func createConstant(ctx *ExecContext, stmt *ast.CreateConstantStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Validate module name is specified
	if stmt.Name.Module == "" {
		return mdlerrors.NewValidation("module name required for constant: use create constant Module.ConstantName")
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, stmt.Name.Module)
	if err != nil {
		return err
	}

	// Convert AST data type to model constant data type
	constType := astDataTypeToConstantDataType(stmt.DataType)

	// Format default value as string
	defaultValue := ""
	if stmt.DefaultValue != nil {
		defaultValue = fmt.Sprintf("%v", stmt.DefaultValue)
	}

	// Check if constant already exists in this module
	existingConstants, err := ctx.Backend.ListConstants()
	if err == nil {
		h, _ := getHierarchy(ctx)
		for _, c := range existingConstants {
			modID := h.FindModuleID(c.ContainerID)
			modName := h.GetModuleName(modID)
			if strings.EqualFold(modName, stmt.Name.Module) && strings.EqualFold(c.Name, stmt.Name.Name) {
				if stmt.CreateOrModify {
					// Update existing constant — COMMENT takes precedence over doc-comment
					if stmt.Comment != "" {
						c.Documentation = stmt.Comment
					} else {
						c.Documentation = stmt.Documentation
					}
					c.Type = constType
					c.DefaultValue = defaultValue
					c.ExposedToClient = stmt.ExposedToClient
					if err := ctx.Backend.UpdateConstant(c); err != nil {
						return mdlerrors.NewBackend("update constant", err)
					}
					invalidateHierarchy(ctx)
					fmt.Fprintf(ctx.Output, "Modified constant: %s.%s\n", modName, c.Name)
					return nil
				}
				return mdlerrors.NewAlreadyExistsMsg("constant", modName+"."+c.Name, fmt.Sprintf("constant already exists: %s.%s (use create or modify to update)", modName, c.Name))
			}
		}
	}

	// COMMENT 'text' takes precedence; fall back to /** */ doc-comment
	doc := stmt.Comment
	if doc == "" {
		doc = stmt.Documentation
	}

	containerID := module.ID
	if stmt.Folder != "" {
		folderID, err := resolveFolder(ctx, module.ID, stmt.Folder)
		if err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("resolve folder %s", stmt.Folder), err)
		}
		containerID = folderID
	}

	constant := &model.Constant{
		ContainerID:     containerID,
		Name:            stmt.Name.Name,
		Documentation:   doc,
		Type:            constType,
		DefaultValue:    defaultValue,
		ExposedToClient: stmt.ExposedToClient,
	}

	if err := ctx.Backend.CreateConstant(constant); err != nil {
		return mdlerrors.NewBackend("create constant", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Created constant: %s.%s\n", stmt.Name.Module, stmt.Name.Name)
	return nil
}

// listConstantValues handles SHOW CONSTANT VALUES command.
// Displays one row per constant per configuration for easy comparison.
func listConstantValues(ctx *ExecContext, moduleName string) error {
	constants, err := ctx.Backend.ListConstants()
	if err != nil {
		return mdlerrors.NewBackend("list constants", err)
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Build constant list with qualified names
	type constInfo struct {
		qualifiedName string
		defaultValue  string
		typeStr       string
	}
	var consts []constInfo
	for _, c := range constants {
		modID := h.FindModuleID(c.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && !strings.EqualFold(modName, moduleName) {
			continue
		}
		consts = append(consts, constInfo{
			qualifiedName: modName + "." + c.Name,
			defaultValue:  c.DefaultValue,
			typeStr:       formatConstantType(c.Type),
		})
	}

	if len(consts) == 0 && ctx.Format != FormatJSON {
		fmt.Fprintln(ctx.Output, "No constants found.")
		return nil
	}

	sort.Slice(consts, func(i, j int) bool {
		return strings.ToLower(consts[i].qualifiedName) < strings.ToLower(consts[j].qualifiedName)
	})

	// Build configuration constant value lookup: configName -> constantId -> value
	configValues := make(map[string]map[string]string)
	var configNames []string
	if ps.Configuration != nil {
		for _, cfg := range ps.Configuration.Configurations {
			configNames = append(configNames, cfg.Name)
			m := make(map[string]string)
			for _, cv := range cfg.ConstantValues {
				m[cv.ConstantId] = cv.Value
			}
			configValues[cfg.Name] = m
		}
	}

	// Build rows: one per constant + "(default)" row, then one per configuration override
	type row struct {
		constant      string
		configuration string
		value         string
	}
	var rows []row

	for _, c := range consts {
		// Default value row
		rows = append(rows, row{c.qualifiedName, "(default)", c.defaultValue})

		// Per-configuration rows
		for _, cfgName := range configNames {
			if val, ok := configValues[cfgName][c.qualifiedName]; ok {
				rows = append(rows, row{c.qualifiedName, cfgName, val})
			}
		}
	}

	result := &TableResult{
		Columns: []string{"Constant", "Configuration", "Value"},
		Summary: fmt.Sprintf("(%d rows)", len(rows)),
	}
	for _, r := range rows {
		val := r.value
		if len(val) > 60 {
			val = val[:57] + "..."
		}
		result.Rows = append(result.Rows, []any{r.constant, r.configuration, val})
	}
	return writeResult(ctx, result)
}

// dropConstant handles DROP CONSTANT command.
func dropConstant(ctx *ExecContext, stmt *ast.DropConstantStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	constants, err := ctx.Backend.ListConstants()
	if err != nil {
		return mdlerrors.NewBackend("list constants", err)
	}

	// Use hierarchy for proper module resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the constant
	for _, c := range constants {
		modID := h.FindModuleID(c.ContainerID)
		modName := h.GetModuleName(modID)
		if strings.EqualFold(modName, stmt.Name.Module) && strings.EqualFold(c.Name, stmt.Name.Name) {
			if err := ctx.Backend.DeleteConstant(c.ID); err != nil {
				return mdlerrors.NewBackend("drop constant", err)
			}
			invalidateHierarchy(ctx)
			fmt.Fprintf(ctx.Output, "Dropped constant: %s.%s\n", modName, c.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFound("constant", stmt.Name.String())
}

// astDataTypeToConstantDataType converts AST DataType to model.ConstantDataType.
func astDataTypeToConstantDataType(dt ast.DataType) model.ConstantDataType {
	switch dt.Kind {
	case ast.TypeString:
		return model.ConstantDataType{Kind: "String"}
	case ast.TypeInteger:
		return model.ConstantDataType{Kind: "Integer"}
	case ast.TypeLong:
		return model.ConstantDataType{Kind: "Long"}
	case ast.TypeDecimal:
		return model.ConstantDataType{Kind: "Decimal"}
	case ast.TypeBoolean:
		return model.ConstantDataType{Kind: "Boolean"}
	case ast.TypeDateTime:
		return model.ConstantDataType{Kind: "DateTime"}
	case ast.TypeDate:
		return model.ConstantDataType{Kind: "Date"}
	case ast.TypeBinary:
		return model.ConstantDataType{Kind: "Binary"}
	case ast.TypeEnumeration:
		enumRef := ""
		if dt.EnumRef != nil {
			enumRef = dt.EnumRef.String()
		}
		return model.ConstantDataType{Kind: "Enumeration", EnumRef: enumRef}
	default:
		return model.ConstantDataType{Kind: "String"}
	}
}
