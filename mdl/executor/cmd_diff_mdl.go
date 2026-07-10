// SPDX-License-Identifier: Apache-2.0

// Package executor - MDL generation functions for diff (statement→text and project→text converters)
package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// ============================================================================
// Statement to MDL Converters
// ============================================================================

// entityStmtToMDL converts a CreateEntityStmt to MDL text
func entityStmtToMDL(ctx *ExecContext, s *ast.CreateEntityStmt) string {
	var lines []string

	// Documentation
	if s.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+s.Documentation)
		lines = append(lines, " */")
	}

	// Position annotation
	if s.Position != nil {
		lines = append(lines, fmt.Sprintf("@Position(%d, %d)", s.Position.X, s.Position.Y))
	}

	// Entity type
	entityType := s.Kind.String()
	lines = append(lines, fmt.Sprintf("create %s entity %s (", entityType, s.Name))

	// Attributes
	for i, attr := range s.Attributes {
		// Attribute documentation
		if attr.Documentation != "" {
			lines = append(lines, fmt.Sprintf("  /** %s */", attr.Documentation))
		}

		typeStr := dataTypeToString(ctx, attr.Type)
		constraints := ""

		if attr.NotNull {
			constraints += " not null"
			if attr.NotNullError != "" {
				constraints += fmt.Sprintf(" error '%s'", attr.NotNullError)
			}
		}
		if attr.Unique {
			constraints += " unique"
			if attr.UniqueError != "" {
				constraints += fmt.Sprintf(" error '%s'", attr.UniqueError)
			}
		}
		if attr.HasDefault {
			defaultVal := fmt.Sprintf("%v", attr.DefaultValue)
			if attr.Type.Kind == ast.TypeString {
				defaultVal = fmt.Sprintf("'%s'", attr.DefaultValue)
			}
			constraints += fmt.Sprintf(" default %s", defaultVal)
		}

		comma := ","
		if i == len(s.Attributes)-1 {
			comma = ""
		}
		lines = append(lines, fmt.Sprintf("  %s: %s%s%s", attr.Name, typeStr, constraints, comma))
	}

	lines = append(lines, ")")

	// Indexes
	for _, idx := range s.Indexes {
		var cols []string
		for _, col := range idx.Columns {
			colStr := col.Name
			if col.Descending {
				colStr += " desc"
			}
			cols = append(cols, colStr)
		}
		lines = append(lines, fmt.Sprintf("index (%s)", strings.Join(cols, ", ")))
	}

	lines = append(lines, ";")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// viewEntityStmtToMDL converts a CreateViewEntityStmt to MDL text
func viewEntityStmtToMDL(ctx *ExecContext, s *ast.CreateViewEntityStmt) string {
	var lines []string

	if s.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+s.Documentation)
		lines = append(lines, " */")
	}

	if s.Position != nil {
		lines = append(lines, fmt.Sprintf("@Position(%d, %d)", s.Position.X, s.Position.Y))
	}

	lines = append(lines, fmt.Sprintf("create view entity %s (", s.Name))

	for i, attr := range s.Attributes {
		typeStr := dataTypeToString(ctx, attr.Type)
		comma := ","
		if i == len(s.Attributes)-1 {
			comma = ""
		}
		lines = append(lines, fmt.Sprintf("  %s: %s%s", attr.Name, typeStr, comma))
	}

	lines = append(lines, ") as (")
	// Indent OQL query
	for line := range strings.SplitSeq(s.Query.RawQuery, "\n") {
		lines = append(lines, "  "+line)
	}
	lines = append(lines, ");")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// enumerationStmtToMDL converts a CreateEnumerationStmt to MDL text
func enumerationStmtToMDL(ctx *ExecContext, s *ast.CreateEnumerationStmt) string {
	var lines []string

	if s.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+s.Documentation)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("create enumeration %s (", s.Name))

	for i, v := range s.Values {
		comma := ","
		if i == len(s.Values)-1 {
			comma = ""
		}
		lines = append(lines, fmt.Sprintf("  %s '%s'%s", v.Name, v.Caption, comma))
	}

	lines = append(lines, ");")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// associationStmtToMDL converts a CreateAssociationStmt to MDL text
func associationStmtToMDL(ctx *ExecContext, s *ast.CreateAssociationStmt) string {
	var lines []string

	if s.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+s.Documentation)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("create association %s", s.Name))
	lines = append(lines, fmt.Sprintf("from %s to %s", s.Parent, s.Child))

	assocType := "Reference"
	if s.Type == ast.AssocReferenceSet {
		assocType = "ReferenceSet"
	}
	lines = append(lines, fmt.Sprintf("type %s", assocType))

	owner := "Default"
	if s.Owner == ast.OwnerBoth {
		owner = "Both"
	}
	lines = append(lines, fmt.Sprintf("owner %s", owner))

	deleteBehavior := "DELETE_BUT_KEEP_REFERENCES"
	switch s.DeleteBehavior {
	case ast.DeleteCascade:
		deleteBehavior = "DELETE_CASCADE"
	case ast.DeleteIfNoReferences:
		deleteBehavior = "DELETE_IF_NO_REFERENCES"
	}
	lines = append(lines, fmt.Sprintf("delete_behavior %s;", deleteBehavior))
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// microflowStmtToMDL converts a CreateMicroflowStmt to MDL text
func microflowStmtToMDL(ctx *ExecContext, s *ast.CreateMicroflowStmt) string {
	var lines []string

	// Annotations
	if s.Excluded {
		lines = append(lines, "@excluded")
	}

	// Documentation
	if s.Documentation != "" {
		lines = append(lines, "/**")
		for docLine := range strings.SplitSeq(s.Documentation, "\n") {
			lines = append(lines, " * "+docLine)
		}
		lines = append(lines, " */")
	}

	// CREATE [OR MODIFY] MICROFLOW header with parameters
	header := "create"
	if s.CreateOrModify {
		header = "create or modify"
	}
	if len(s.Parameters) > 0 {
		lines = append(lines, fmt.Sprintf("%s microflow %s (", header, s.Name))
		for i, param := range s.Parameters {
			paramType := dataTypeToString(ctx, param.Type)
			comma := ","
			if i == len(s.Parameters)-1 {
				comma = ""
			}
			lines = append(lines, fmt.Sprintf("  $%s: %s%s", param.Name, paramType, comma))
		}
		lines = append(lines, ")")
	} else {
		lines = append(lines, fmt.Sprintf("%s microflow %s ()", header, s.Name))
	}

	// Folder
	if s.Folder != "" {
		lines = append(lines, fmt.Sprintf("folder '%s'", s.Folder))
	}

	// Comment
	if s.Comment != "" {
		lines = append(lines, fmt.Sprintf("comment '%s'", s.Comment))
	}

	// Return type
	if s.ReturnType != nil {
		returnType := dataTypeToString(ctx, s.ReturnType.Type)
		if returnType != "Void" && returnType != "" {
			returnLine := fmt.Sprintf("returns %s", returnType)
			if s.ReturnType.Variable != "" {
				returnLine += fmt.Sprintf(" as $%s", s.ReturnType.Variable)
			}
			lines = append(lines, returnLine)
		}
	}

	// BEGIN block
	lines = append(lines, "begin")

	// Body statements
	for _, stmt := range s.Body {
		stmtLines := microflowStatementToMDL(ctx, stmt, 1)
		lines = append(lines, stmtLines...)
	}

	lines = append(lines, "end;")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// nanoflowStmtToMDL converts a CreateNanoflowStmt to MDL text
func nanoflowStmtToMDL(ctx *ExecContext, s *ast.CreateNanoflowStmt) string {
	var lines []string

	// Annotations
	if s.Excluded {
		lines = append(lines, "@excluded")
	}

	// Documentation
	if s.Documentation != "" {
		lines = append(lines, "/**")
		for docLine := range strings.SplitSeq(s.Documentation, "\n") {
			lines = append(lines, " * "+docLine)
		}
		lines = append(lines, " */")
	}

	// CREATE [OR MODIFY] NANOFLOW header with parameters
	header := "create"
	if s.CreateOrModify {
		header = "create or modify"
	}
	if len(s.Parameters) > 0 {
		lines = append(lines, fmt.Sprintf("%s nanoflow %s (", header, s.Name))
		for i, param := range s.Parameters {
			paramType := dataTypeToString(ctx, param.Type)
			comma := ","
			if i == len(s.Parameters)-1 {
				comma = ""
			}
			lines = append(lines, fmt.Sprintf("  $%s: %s%s", param.Name, paramType, comma))
		}
		lines = append(lines, ")")
	} else {
		lines = append(lines, fmt.Sprintf("%s nanoflow %s ()", header, s.Name))
	}

	// Folder
	if s.Folder != "" {
		lines = append(lines, fmt.Sprintf("folder '%s'", s.Folder))
	}

	// Comment
	if s.Comment != "" {
		lines = append(lines, fmt.Sprintf("comment '%s'", s.Comment))
	}

	// Return type
	if s.ReturnType != nil {
		returnType := dataTypeToString(ctx, s.ReturnType.Type)
		if returnType != "Void" && returnType != "" {
			returnLine := fmt.Sprintf("returns %s", returnType)
			if s.ReturnType.Variable != "" {
				returnLine += fmt.Sprintf(" as $%s", s.ReturnType.Variable)
			}
			lines = append(lines, returnLine)
		}
	}

	// BEGIN block
	lines = append(lines, "begin")

	// Body statements
	for _, stmt := range s.Body {
		stmtLines := microflowStatementToMDL(ctx, stmt, 1)
		lines = append(lines, stmtLines...)
	}

	lines = append(lines, "end;")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// microflowStatementToMDL converts a microflow statement to MDL lines
func microflowStatementToMDL(ctx *ExecContext, stmt ast.MicroflowStatement, indent int) []string {
	indentStr := strings.Repeat("  ", indent)
	var lines []string

	switch s := stmt.(type) {
	case *ast.DeclareStmt:
		typeStr := dataTypeToString(ctx, s.Type)
		initVal := "empty"
		if s.InitialValue != nil {
			initVal = diffExpressionToString(ctx, s.InitialValue)
		}
		lines = append(lines, fmt.Sprintf("%sdeclare $%s %s = %s;", indentStr, s.Variable, typeStr, initVal))

	case *ast.MfSetStmt:
		lines = append(lines, fmt.Sprintf("%sset $%s = %s;", indentStr, s.Target, diffExpressionToString(ctx, s.Value)))

	case *ast.ReturnStmt:
		if s.Value != nil {
			lines = append(lines, fmt.Sprintf("%sreturn %s;", indentStr, diffExpressionToString(ctx, s.Value)))
		} else {
			lines = append(lines, fmt.Sprintf("%sreturn;", indentStr))
		}

	case *ast.CreateObjectStmt:
		if len(s.Changes) > 0 {
			var members []string
			for _, c := range s.Changes {
				members = append(members, fmt.Sprintf("%s = %s", c.Attribute, diffExpressionToString(ctx, c.Value)))
			}
			lines = append(lines, fmt.Sprintf("%s$%s = create %s (%s);", indentStr, s.Variable, s.EntityType, strings.Join(members, ", ")))
		} else {
			lines = append(lines, fmt.Sprintf("%s$%s = create %s;", indentStr, s.Variable, s.EntityType))
		}

	case *ast.ChangeObjectStmt:
		if len(s.Changes) > 0 {
			var members []string
			for _, c := range s.Changes {
				members = append(members, fmt.Sprintf("%s = %s", c.Attribute, diffExpressionToString(ctx, c.Value)))
			}
			lines = append(lines, fmt.Sprintf("%schange $%s (%s);", indentStr, s.Variable, strings.Join(members, ", ")))
		} else {
			lines = append(lines, fmt.Sprintf("%schange $%s;", indentStr, s.Variable))
		}

	case *ast.MfCommitStmt:
		suffix := ""
		if s.WithEvents {
			suffix += " with events"
		}
		if s.RefreshInClient {
			suffix += " refresh"
		}
		lines = append(lines, fmt.Sprintf("%scommit $%s%s;", indentStr, s.Variable, suffix))

	case *ast.DeleteObjectStmt:
		lines = append(lines, fmt.Sprintf("%sdelete $%s;", indentStr, s.Variable))

	case *ast.RetrieveStmt:
		var stmt string
		if s.StartVariable != "" {
			stmt = fmt.Sprintf("%sretrieve $%s from $%s/%s", indentStr, s.Variable, s.StartVariable, s.Source)
		} else {
			stmt = fmt.Sprintf("%sretrieve $%s from %s", indentStr, s.Variable, s.Source)
		}
		if s.Where != nil {
			stmt += fmt.Sprintf("\n%s    where %s", indentStr, diffExpressionToString(ctx, s.Where))
		}
		if s.Limit != "" {
			stmt += fmt.Sprintf("\n%s    limit %s", indentStr, s.Limit)
		}
		lines = append(lines, stmt+";")

	case *ast.IfStmt:
		lines = append(lines, fmt.Sprintf("%sif %s then", indentStr, diffExpressionToString(ctx, s.Condition)))
		for _, thenStmt := range s.ThenBody {
			lines = append(lines, microflowStatementToMDL(ctx, thenStmt, indent+1)...)
		}
		if s.HasElse || len(s.ElseBody) > 0 {
			lines = append(lines, indentStr+"else")
			for _, elseStmt := range s.ElseBody {
				lines = append(lines, microflowStatementToMDL(ctx, elseStmt, indent+1)...)
			}
		}
		lines = append(lines, indentStr+"end if;")

	case *ast.EnumSplitStmt:
		lines = append(lines, fmt.Sprintf("%scase $%s", indentStr, s.Variable))
		for _, c := range s.Cases {
			lines = append(lines, fmt.Sprintf("%s  when %s then", indentStr, formatEnumSplitCaseValues(enumSplitCaseValues(c))))
			for _, caseStmt := range c.Body {
				lines = append(lines, microflowStatementToMDL(ctx, caseStmt, indent+1)...)
			}
		}
		if len(s.ElseBody) > 0 {
			lines = append(lines, indentStr+"  else")
			for _, elseStmt := range s.ElseBody {
				lines = append(lines, microflowStatementToMDL(ctx, elseStmt, indent+1)...)
			}
		}
		lines = append(lines, indentStr+"end case;")

	case *ast.InheritanceSplitStmt:
		lines = append(lines, fmt.Sprintf("%ssplit type $%s", indentStr, s.Variable))
		for _, c := range s.Cases {
			lines = append(lines, fmt.Sprintf("%scase %s", indentStr, c.Entity.String()))
			for _, caseStmt := range c.Body {
				lines = append(lines, microflowStatementToMDL(ctx, caseStmt, indent+1)...)
			}
		}
		if len(s.ElseBody) > 0 {
			lines = append(lines, indentStr+"else")
			for _, elseStmt := range s.ElseBody {
				lines = append(lines, microflowStatementToMDL(ctx, elseStmt, indent+1)...)
			}
		}
		lines = append(lines, indentStr+"end split;")

	case *ast.CastObjectStmt:
		if s.ObjectVariable == "" {
			lines = append(lines, fmt.Sprintf("%scast $%s;", indentStr, s.OutputVariable))
		} else {
			lines = append(lines, fmt.Sprintf("%s$%s = cast $%s;", indentStr, s.OutputVariable, s.ObjectVariable))
		}

	case *ast.LoopStmt:
		lines = append(lines, fmt.Sprintf("%sloop $%s in $%s", indentStr, s.LoopVariable, s.ListVariable))
		for _, bodyStmt := range s.Body {
			lines = append(lines, microflowStatementToMDL(ctx, bodyStmt, indent+1)...)
		}
		lines = append(lines, indentStr+"end loop;")

	case *ast.LogStmt:
		nodeStr := defaultLogNodeExpression
		if s.Node != nil {
			nodeStr = diffExpressionToString(ctx, s.Node)
		}
		msgStr := diffExpressionToString(ctx, s.Message)
		stmt := fmt.Sprintf("%slog %s node %s %s", indentStr, strings.ToLower(s.Level.String()), nodeStr, msgStr)
		if len(s.Template) > 0 {
			var params []string
			for _, p := range s.Template {
				params = append(params, fmt.Sprintf("{%d} = %s", p.Index, diffExpressionToString(ctx, p.Value)))
			}
			stmt += fmt.Sprintf(" with (%s)", strings.Join(params, ", "))
		}
		lines = append(lines, stmt+";")

	case *ast.CallMicroflowStmt:
		var params []string
		for _, arg := range s.Arguments {
			params = append(params, fmt.Sprintf("%s = %s", arg.Name, diffExpressionToString(ctx, arg.Value)))
		}
		paramStr := strings.Join(params, ", ")
		if s.OutputVariable != "" {
			lines = append(lines, fmt.Sprintf("%s$%s = call microflow %s(%s);", indentStr, s.OutputVariable, s.MicroflowName, paramStr))
		} else {
			lines = append(lines, fmt.Sprintf("%scall microflow %s(%s);", indentStr, s.MicroflowName, paramStr))
		}

	case *ast.CallNanoflowStmt:
		var params []string
		for _, arg := range s.Arguments {
			params = append(params, fmt.Sprintf("%s = %s", arg.Name, diffExpressionToString(ctx, arg.Value)))
		}
		paramStr := strings.Join(params, ", ")
		if s.OutputVariable != "" {
			lines = append(lines, fmt.Sprintf("%s$%s = call nanoflow %s(%s);", indentStr, s.OutputVariable, s.NanoflowName, paramStr))
		} else {
			lines = append(lines, fmt.Sprintf("%scall nanoflow %s(%s);", indentStr, s.NanoflowName, paramStr))
		}

	case *ast.BreakStmt:
		lines = append(lines, indentStr+"break;")

	case *ast.ContinueStmt:
		lines = append(lines, indentStr+"continue;")
	}

	return lines
}

// ============================================================================
// Project to MDL Converters
// ============================================================================

// entityToMDL converts a project entity to MDL text
func entityToMDL(ctx *ExecContext, moduleName string, entity *domainmodel.Entity, dm *domainmodel.DomainModel) string {
	var lines []string

	// Documentation
	if entity.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+entity.Documentation)
		lines = append(lines, " */")
	}

	// Position
	lines = append(lines, fmt.Sprintf("@Position(%d, %d)", entity.Location.X, entity.Location.Y))

	// Entity type
	entityType := "persistent"
	if strings.Contains(entity.Source, "OqlView") {
		entityType = "view"
	} else if !entity.Persistable {
		entityType = "non-persistent"
	}

	lines = append(lines, fmt.Sprintf("create %s entity %s.%s (", entityType, moduleName, entity.Name))

	// Build validation rules map
	validationsByAttr := make(map[model.ID][]*domainmodel.ValidationRule)
	validationsByName := make(map[string][]*domainmodel.ValidationRule)
	for _, vr := range entity.ValidationRules {
		validationsByAttr[vr.AttributeID] = append(validationsByAttr[vr.AttributeID], vr)
		attrName := extractAttrNameFromQualified(string(vr.AttributeID))
		if attrName != "" {
			validationsByName[attrName] = append(validationsByName[attrName], vr)
		}
	}

	// Attributes
	for i, attr := range entity.Attributes {
		// Documentation
		if attr.Documentation != "" {
			lines = append(lines, fmt.Sprintf("  /** %s */", attr.Documentation))
		}

		typeStr := formatAttributeType(attr.Type)
		var constraints strings.Builder

		// Check for validation rules
		attrValidations := validationsByAttr[attr.ID]
		if len(attrValidations) == 0 {
			attrValidations = validationsByName[attr.Name]
		}
		for _, vr := range attrValidations {
			if vr.Type == "Required" {
				constraints.WriteString(" not null")
				if vr.ErrorMessage != nil {
					errMsg := vr.ErrorMessage.GetTranslation("en_US")
					if errMsg != "" {
						constraints.WriteString(fmt.Sprintf(" error '%s'", errMsg))
					}
				}
			}
			if vr.Type == "Unique" {
				constraints.WriteString(" unique")
				if vr.ErrorMessage != nil {
					errMsg := vr.ErrorMessage.GetTranslation("en_US")
					if errMsg != "" {
						constraints.WriteString(fmt.Sprintf(" error '%s'", errMsg))
					}
				}
			}
		}

		// Default value
		if attr.Value != nil && attr.Value.DefaultValue != "" {
			defaultVal := attr.Value.DefaultValue
			if _, ok := attr.Type.(*domainmodel.StringAttributeType); ok {
				defaultVal = fmt.Sprintf("'%s'", defaultVal)
			}
			constraints.WriteString(fmt.Sprintf(" default %s", defaultVal))
		}

		comma := ","
		if i == len(entity.Attributes)-1 {
			comma = ""
		}
		lines = append(lines, fmt.Sprintf("  %s: %s%s%s", attr.Name, typeStr, constraints.String(), comma))
	}

	lines = append(lines, ")")

	// Build attr name map for indexes
	attrNames := make(map[model.ID]string)
	for _, attr := range entity.Attributes {
		attrNames[attr.ID] = attr.Name
	}

	// Indexes
	for _, idx := range entity.Indexes {
		var cols []string
		for _, ia := range idx.Attributes {
			colName := attrNames[ia.AttributeID]
			if !ia.Ascending {
				colName += " desc"
			}
			cols = append(cols, colName)
		}
		if len(cols) > 0 {
			lines = append(lines, fmt.Sprintf("index (%s)", strings.Join(cols, ", ")))
		}
	}

	lines = append(lines, ";")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// viewEntityFromProjectToMDL converts a view entity from project to MDL
func viewEntityFromProjectToMDL(ctx *ExecContext, moduleName string, entity *domainmodel.Entity, dm *domainmodel.DomainModel) string {
	var lines []string

	if entity.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+entity.Documentation)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("@Position(%d, %d)", entity.Location.X, entity.Location.Y))
	lines = append(lines, fmt.Sprintf("create view entity %s.%s (", moduleName, entity.Name))

	for i, attr := range entity.Attributes {
		typeStr := formatAttributeType(attr.Type)
		comma := ","
		if i == len(entity.Attributes)-1 {
			comma = ""
		}
		lines = append(lines, fmt.Sprintf("  %s: %s%s", attr.Name, typeStr, comma))
	}

	lines = append(lines, ") as (")
	if entity.OqlQuery != "" {
		for line := range strings.SplitSeq(entity.OqlQuery, "\n") {
			lines = append(lines, "  "+line)
		}
	}
	lines = append(lines, ");")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// enumerationToMDL converts a project enumeration to MDL text
func enumerationToMDL(ctx *ExecContext, moduleName string, enum *model.Enumeration) string {
	var lines []string

	if enum.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+enum.Documentation)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("create enumeration %s.%s (", moduleName, enum.Name))

	for i, v := range enum.Values {
		comma := ","
		if i == len(enum.Values)-1 {
			comma = ""
		}
		caption := ""
		if v.Caption != nil {
			caption = v.Caption.GetTranslation("en_US")
		}
		lines = append(lines, fmt.Sprintf("  %s '%s'%s", v.Name, caption, comma))
	}

	lines = append(lines, ");")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// associationToMDL converts a project association to MDL text
func associationToMDL(ctx *ExecContext, moduleName string, assoc *domainmodel.Association, dm *domainmodel.DomainModel) string {
	var lines []string

	// Build entity name map
	entityNames := make(map[model.ID]string)
	for _, entity := range dm.Entities {
		entityNames[entity.ID] = entity.Name
	}

	if assoc.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+assoc.Documentation)
		lines = append(lines, " */")
	}

	fromEntity := entityNames[assoc.ParentID]
	toEntity := entityNames[assoc.ChildID]

	lines = append(lines, fmt.Sprintf("create association %s.%s", moduleName, assoc.Name))
	lines = append(lines, fmt.Sprintf("from %s.%s to %s.%s", moduleName, fromEntity, moduleName, toEntity))

	assocType := "Reference"
	if assoc.Type == domainmodel.AssociationTypeReferenceSet {
		assocType = "ReferenceSet"
	}
	lines = append(lines, fmt.Sprintf("type %s", assocType))

	owner := "Default"
	if assoc.Owner == domainmodel.AssociationOwnerBoth {
		owner = "Both"
	}
	lines = append(lines, fmt.Sprintf("owner %s", owner))

	deleteBehavior := "DELETE_BUT_KEEP_REFERENCES"
	if assoc.ChildDeleteBehavior != nil {
		switch assoc.ChildDeleteBehavior.Type {
		case domainmodel.DeleteBehaviorTypeDeleteMeAndReferences:
			deleteBehavior = "DELETE_CASCADE"
		case domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences:
			deleteBehavior = "DELETE_IF_NO_REFERENCES"
		}
	}
	lines = append(lines, fmt.Sprintf("delete_behavior %s;", deleteBehavior))
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// ============================================================================
// Helper Functions
// ============================================================================

// dataTypeToString converts a DataType to its string representation
func dataTypeToString(_ *ExecContext, dt ast.DataType) string {
	switch dt.Kind {
	case ast.TypeString:
		if dt.Length > 0 {
			return fmt.Sprintf("String(%d)", dt.Length)
		}
		return "String"
	case ast.TypeInteger:
		return "Integer"
	case ast.TypeLong:
		return "Long"
	case ast.TypeDecimal:
		return "Decimal"
	case ast.TypeBoolean:
		return "Boolean"
	case ast.TypeDateTime:
		return "DateTime"
	case ast.TypeDate:
		return "Date"
	case ast.TypeAutoNumber:
		return "AutoNumber"
	case ast.TypeBinary:
		return "Binary"
	case ast.TypeEnumeration:
		if dt.EnumRef != nil {
			return fmt.Sprintf("Enumeration(%s)", dt.EnumRef.String())
		}
		return "Enumeration"
	case ast.TypeEntity:
		if dt.EntityRef != nil {
			return dt.EntityRef.String()
		}
		return "Object"
	case ast.TypeListOf:
		if dt.EntityRef != nil {
			return fmt.Sprintf("List of %s", dt.EntityRef.String())
		}
		return "List"
	case ast.TypeVoid:
		return "Void"
	default:
		return "Unknown"
	}
}

// diffExpressionToString converts an expression to its string representation for diff output
func diffExpressionToString(ctx *ExecContext, expr ast.Expression) string {
	if expr == nil {
		return "empty"
	}

	switch ex := expr.(type) {
	case *ast.LiteralExpr:
		if ex.Kind == ast.LiteralString {
			return fmt.Sprintf("'%v'", ex.Value)
		}
		if ex.Kind == ast.LiteralEmpty {
			return "empty"
		}
		if ex.Kind == ast.LiteralNull {
			return "null"
		}
		return fmt.Sprintf("%v", ex.Value)
	case *ast.VariableExpr:
		return "$" + ex.Name
	case *ast.AttributePathExpr:
		return "$" + ex.Variable + "/" + strings.Join(ex.Path, "/")
	case *ast.BinaryExpr:
		return fmt.Sprintf("%s %s %s", diffExpressionToString(ctx, ex.Left), ex.Operator, diffExpressionToString(ctx, ex.Right))
	case *ast.UnaryExpr:
		return fmt.Sprintf("%s%s", ex.Operator, diffExpressionToString(ctx, ex.Operand))
	case *ast.FunctionCallExpr:
		var args []string
		for _, arg := range ex.Arguments {
			args = append(args, diffExpressionToString(ctx, arg))
		}
		return fmt.Sprintf("%s(%s)", ex.Name, strings.Join(args, ", "))
	case *ast.TokenExpr:
		return fmt.Sprintf("[%%%s%%]", ex.Token)
	case *ast.ParenExpr:
		return fmt.Sprintf("(%s)", diffExpressionToString(ctx, ex.Inner))
	case *ast.QualifiedNameExpr:
		return ex.QualifiedName.String()
	case *ast.ConstantRefExpr:
		return "@" + ex.QualifiedName.String()
	default:
		return fmt.Sprintf("%v", expr)
	}
}
