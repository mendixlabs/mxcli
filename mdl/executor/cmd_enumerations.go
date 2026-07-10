// SPDX-License-Identifier: Apache-2.0

// Package executor - Enumeration commands (SHOW/DESCRIBE/CREATE/ALTER/DROP ENUMERATION)
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// execCreateEnumeration handles CREATE ENUMERATION statements.
func execCreateEnumeration(ctx *ExecContext, s *ast.CreateEnumerationStmt) error {

	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Validate enumeration values for reserved words
	if violations := ValidateEnumeration(s); len(violations) > 0 {
		var msgs []string
		for _, v := range violations {
			msgs = append(msgs, v.Message)
		}
		return mdlerrors.NewValidationf("invalid enumeration '%s':\n  - %s",
			s.Name.String(), strings.Join(msgs, "\n  - "))
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	// Check if enumeration already exists
	existingEnum := findEnumeration(ctx, s.Name.Module, s.Name.Name)
	if existingEnum != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExistsMsg("enumeration", s.Name.Module+"."+s.Name.Name, fmt.Sprintf("enumeration already exists: %s.%s (use create or modify to update)", s.Name.Module, s.Name.Name))
	}

	// Create enumeration values
	var values []model.EnumerationValue
	for _, v := range s.Values {
		values = append(values, model.EnumerationValue{
			Name: v.Name,
			Caption: &model.Text{
				Translations: map[string]string{"en_US": v.Caption},
			},
		})
	}

	// Create or update enumeration
	enum := &model.Enumeration{
		ContainerID:   module.ID,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		Values:        values,
	}

	if existingEnum != nil && s.CreateOrModify {
		// In-place update: preserve the existing UUID so BSON git-diff sees a
		// modification rather than a delete+insert pair.
		enum.ID = existingEnum.ID
		if err := ctx.Backend.UpdateEnumeration(enum); err != nil {
			return mdlerrors.NewBackend("update enumeration", err)
		}
		invalidateHierarchy(ctx)
		fmt.Fprintf(ctx.Output, "Modified enumeration: %s\n", s.Name)
		return nil
	}

	if err := ctx.Backend.CreateEnumeration(enum); err != nil {
		return mdlerrors.NewBackend("create enumeration", err)
	}

	// Invalidate hierarchy cache so the new enumeration's container is visible
	invalidateHierarchy(ctx)

	fmt.Fprintf(ctx.Output, "Created enumeration: %s\n", s.Name)
	return nil
}

// findEnumeration finds an enumeration by module and name.
func findEnumeration(ctx *ExecContext, moduleName, enumName string) *model.Enumeration {

	enums, err := ctx.Backend.ListEnumerations()
	if err != nil {
		return nil
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}

	for _, enum := range enums {
		modID := h.FindModuleID(enum.ContainerID)
		modName := h.GetModuleName(modID)
		if enum.Name == enumName && modName == moduleName {
			return enum
		}
	}
	return nil
}

// execAlterEnumeration handles ALTER ENUMERATION ADD/DROP/RENAME VALUE by
// read-modify-writing the enumeration through the backend (engine-agnostic).
func execAlterEnumeration(ctx *ExecContext, s *ast.AlterEnumerationStmt) error {
	enum := findEnumeration(ctx, s.Name.Module, s.Name.Name)
	if enum == nil {
		return mdlerrors.NewNotFound("enumeration", s.Name.String())
	}

	switch s.Operation {
	case ast.AlterEnumAdd:
		for _, v := range enum.Values {
			if v.Name == s.ValueName {
				return mdlerrors.NewAlreadyExistsMsg("enumeration value", s.ValueName,
					fmt.Sprintf("value '%s' already exists on enumeration %s", s.ValueName, s.Name))
			}
		}
		enum.Values = append(enum.Values, model.EnumerationValue{
			Name:    s.ValueName,
			Caption: &model.Text{Translations: map[string]string{"en_US": s.Caption}},
		})

	case ast.AlterEnumDrop:
		idx := -1
		for i, v := range enum.Values {
			if v.Name == s.ValueName {
				idx = i
				break
			}
		}
		if idx < 0 {
			return mdlerrors.NewNotFound("enumeration value", s.ValueName)
		}
		enum.Values = append(enum.Values[:idx], enum.Values[idx+1:]...)

	case ast.AlterEnumRename:
		idx := -1
		for i, v := range enum.Values {
			if v.Name == s.ValueName {
				idx = i
				break
			}
		}
		if idx < 0 {
			return mdlerrors.NewNotFound("enumeration value", s.ValueName)
		}
		enum.Values[idx].Name = s.NewName

	default:
		return mdlerrors.NewUnsupported("unknown ALTER ENUMERATION operation")
	}

	if err := ctx.Backend.UpdateEnumeration(enum); err != nil {
		return mdlerrors.NewBackend("alter enumeration", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Altered enumeration: %s\n", s.Name)
	return nil
}

// execDropEnumeration handles DROP ENUMERATION statements.
func execDropEnumeration(ctx *ExecContext, s *ast.DropEnumerationStmt) error {

	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Find enumeration
	enums, err := ctx.Backend.ListEnumerations()
	if err != nil {
		return mdlerrors.NewBackend("list enumerations", err)
	}

	// Collect all enumerations matching the name (and optional module).
	type match struct {
		enum   *model.Enumeration
		module *model.Module
	}
	var matches []match
	for _, enum := range enums {
		if enum.Name != s.Name.Name {
			continue
		}
		module, err := findModuleByID(ctx, enum.ContainerID)
		if err != nil {
			continue
		}
		if s.Name.Module == "" || module.Name == s.Name.Module {
			matches = append(matches, match{enum, module})
		}
	}

	switch len(matches) {
	case 0:
		return mdlerrors.NewNotFound("enumeration", s.Name.String())
	case 1:
		if err := ctx.Backend.DeleteEnumeration(matches[0].enum.ID); err != nil {
			return mdlerrors.NewBackend("delete enumeration", err)
		}
		fmt.Fprintf(ctx.Output, "Dropped enumeration: %s.%s\n", matches[0].module.Name, s.Name.Name)
		return nil
	default:
		var names []string
		for _, m := range matches {
			names = append(names, m.module.Name+"."+s.Name.Name)
		}
		return mdlerrors.NewValidationf("ambiguous enumeration name %q — found in: %s; use a fully-qualified name",
			s.Name.Name, strings.Join(names, ", "))
	}
}

// listEnumerations handles SHOW ENUMERATIONS command.
func listEnumerations(ctx *ExecContext, moduleName string) error {

	enums, err := ctx.Backend.ListEnumerations()
	if err != nil {
		return mdlerrors.NewBackend("list enumerations", err)
	}

	// Get hierarchy for module/folder resolution
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
		values        int
	}
	var rows []row

	for _, enum := range enums {
		modID := h.FindModuleID(enum.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName == "" || modName == moduleName {
			qualifiedName := modName + "." + enum.Name
			folderPath := h.BuildFolderPath(enum.ContainerID)

			rows = append(rows, row{qualifiedName, modName, enum.Name, folderPath, len(enum.Values)})
		}
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	// Build TableResult
	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Folder", "Values"},
		Summary: fmt.Sprintf("(%d enumerations)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.module, r.name, r.folderPath, r.values})
	}
	return writeResult(ctx, result)
}

// describeEnumeration handles DESCRIBE ENUMERATION command.
func describeEnumeration(ctx *ExecContext, name ast.QualifiedName) error {

	enums, err := ctx.Backend.ListEnumerations()
	if err != nil {
		return mdlerrors.NewBackend("list enumerations", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, enum := range enums {
		modID := h.FindModuleID(enum.ContainerID)
		modName := h.GetModuleName(modID)
		if enum.Name == name.Name && (name.Module == "" || modName == name.Module) {
			// Output JavaDoc documentation if present
			if enum.Documentation != "" {
				fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", enum.Documentation)
			}

			fmt.Fprintf(ctx.Output, "create or modify enumeration %s.%s (\n", modName, enum.Name)
			for i, v := range enum.Values {
				comma := ","
				if i == len(enum.Values)-1 {
					comma = ""
				}
				caption := ""
				if v.Caption != nil {
					caption = v.Caption.GetTranslation("en_US")
				}
				fmt.Fprintf(ctx.Output, "  %s '%s'%s\n", v.Name, caption, comma)
			}
			fmt.Fprintln(ctx.Output, ");")
			fmt.Fprintln(ctx.Output, "/")
			return nil
		}
	}

	return mdlerrors.NewNotFound("enumeration", name.String())
}

// mendixReservedWords contains words that cannot be used as enumeration value names.
// These are Java reserved words plus Mendix-specific reserved identifiers.
// Using any of these triggers CE7247: "The name 'X' is a reserved word."
var mendixReservedWords = map[string]bool{
	// Java reserved words
	"abstract": true, "assert": true, "boolean": true, "break": true,
	"byte": true, "case": true, "catch": true, "char": true,
	"class": true, "const": true, "continue": true, "default": true,
	"do": true, "double": true, "else": true, "enum": true,
	"extends": true, "false": true, "final": true, "finally": true,
	"float": true, "for": true, "goto": true, "if": true,
	"implements": true, "import": true, "instanceof": true, "int": true,
	"interface": true, "long": true, "native": true, "new": true,
	"null": true, "package": true, "private": true, "protected": true,
	"public": true, "return": true, "short": true, "static": true,
	"strictfp": true, "super": true, "switch": true, "synchronized": true,
	"this": true, "throw": true, "throws": true, "transient": true,
	"true": true, "try": true, "type": true, "void": true, "volatile": true,
	"while": true,
	// Mendix-specific reserved identifiers
	"changedby": true, "changeddate": true, "con": true, "context": true,
	"createddate": true, "currentuser": true, "guid": true,
	"id": true, "mendixobject": true, "owner": true, "submetaobjectname": true,
}

// ValidateEnumeration checks enumeration value names for reserved words.
// Returns a list of structured violations with rule IDs (CE7247 equivalent).
// This function does not require a project connection.
func ValidateEnumeration(stmt *ast.CreateEnumerationStmt) []linter.Violation {
	var violations []linter.Violation
	seen := make(map[string]bool)
	for _, v := range stmt.Values {
		lower := strings.ToLower(v.Name)
		if seen[lower] {
			violations = append(violations, linter.Violation{
				RuleID:   "MDL011",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"duplicate enumeration value name '%s'", v.Name),
				Location: linter.Location{
					DocumentType: "enumeration",
					DocumentName: stmt.Name.String(),
				},
				Suggestion: "Each enumeration value must have a unique name",
			})
		}
		seen[lower] = true
		if mendixReservedWords[lower] {
			violations = append(violations, linter.Violation{
				RuleID:   "MDL010",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"enumeration value '%s' is a reserved word (CE7247)",
					v.Name),
				Location: linter.Location{
					DocumentType: "enumeration",
					DocumentName: stmt.Name.String(),
				},
				Suggestion: fmt.Sprintf("Rename to a non-reserved name (e.g., '%s_' or 'Is%s')", v.Name, v.Name),
			})
		}
	}
	return violations
}

// mendixSystemAttributeNames are attribute names reserved by the Mendix runtime.
// These are auto-managed system attributes on persistent entities and cannot be
// used as user-defined attribute names.
var mendixSystemAttributeNames = map[string]bool{
	"createddate": true,
	"changeddate": true,
	"owner":       true,
	"changedby":   true,
}

// ValidateEntity checks entity attribute names for reserved system names.
// Returns a list of structured violations with rule IDs. This function does not require a project connection.
//
// Two distinct checks run here:
//   - MDL020 (mendixSystemAttributeNames): only applies to persistent entities.
//     Owner/ChangedBy/CreatedDate/ChangedDate are auto-managed there and should
//     be declared with the AutoX pseudo-types.
//   - MDL021 (mendixReservedWords / CE7247): applies to ALL entity kinds.
//     Mendix rejects these names at runtime regardless of persistence.
func ValidateEntity(stmt *ast.CreateEntityStmt) []linter.Violation {
	var violations []linter.Violation
	for _, attr := range stmt.Attributes {
		// Skip pseudo-types — these ARE the system attributes (persistent entities only).
		if attr.Type.Kind == ast.TypeAutoOwner || attr.Type.Kind == ast.TypeAutoChangedBy ||
			attr.Type.Kind == ast.TypeAutoCreatedDate || attr.Type.Kind == ast.TypeAutoChangedDate {
			continue
		}
		lower := strings.ToLower(attr.Name)
		// MDL020: system-attribute conflict — only meaningful on persistent entities,
		// where the suggested fix is to use the AutoX pseudo-type.
		if stmt.Kind == ast.EntityPersistent && mendixSystemAttributeNames[lower] {
			violations = append(violations, linter.Violation{
				RuleID:   "MDL020",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"attribute '%s' conflicts with a Mendix system attribute name. "+
						"Mendix automatically manages '%s' on persistent entities",
					attr.Name, attr.Name),
				Location: linter.Location{
					DocumentType: "entity",
					DocumentName: stmt.Name.String(),
				},
				Suggestion: fmt.Sprintf("To use the Mendix built-in audit field, declare it with the pseudo-type: '%s: Auto%s'. To store an unrelated date, choose a different name (e.g., 'EntryDate', 'RecordDate')", attr.Name, attr.Name),
			})
			continue
		}
		// MDL021: CE7247 runtime reserved words — apply to all entity kinds
		// including non-persistent and view entities.
		if mendixReservedWords[lower] {
			violations = append(violations, linter.Violation{
				RuleID:   "MDL021",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"attribute '%s' is a reserved word (CE7247)",
					attr.Name),
				Location: linter.Location{
					DocumentType: "entity",
					DocumentName: stmt.Name.String(),
				},
				Suggestion: fmt.Sprintf("Rename to a non-reserved name (e.g., '%sValue' or '%sField')", attr.Name, attr.Name),
			})
		}
	}
	return violations
}
