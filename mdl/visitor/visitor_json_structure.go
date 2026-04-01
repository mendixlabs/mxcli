// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateJsonStructureStatement is called when exiting the createJsonStructureStatement production.
func (b *Builder) ExitCreateJsonStructureStatement(ctx *parser.CreateJsonStructureStatementContext) {
	stmt := &ast.CreateJsonStructureStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	// Grammar: JSON STRUCTURE qualifiedName (FOLDER STRING_LITERAL)? FROM STRING_LITERAL
	// The STRING_LITERALs are: first = folder (if FOLDER present), last = json snippet
	literals := ctx.AllSTRING_LITERAL()
	if ctx.FOLDER() != nil && len(literals) >= 2 {
		stmt.Folder = unquoteString(literals[0].GetText())
		stmt.JsonSnippet = unquoteString(literals[len(literals)-1].GetText())
	} else if len(literals) >= 1 {
		stmt.JsonSnippet = unquoteString(literals[len(literals)-1].GetText())
	}

	b.statements = append(b.statements, stmt)
}

// ExitDropJsonStructureStatement handles DROP JSON STRUCTURE qualifiedName in the dropStatement rule.
// This is handled in the generic drop statement visitor via context inspection.

// ExitCreateImportMappingStatement is called when exiting the createImportMappingStatement production.
func (b *Builder) ExitCreateImportMappingStatement(ctx *parser.CreateImportMappingStatementContext) {
	stmt := &ast.CreateImportMappingStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	// Parse schema clause
	if schemaCtx := ctx.ImportMappingSchemaClause(); schemaCtx != nil {
		sc := schemaCtx.(*parser.ImportMappingSchemaClauseContext)
		if sc.JSON() != nil {
			stmt.SchemaKind = "JSON_STRUCTURE"
		} else {
			stmt.SchemaKind = "XML_SCHEMA"
		}
		// Schema ref is the qualifiedName inside the schema clause
		if sc.QualifiedName() != nil {
			stmt.SchemaRef = buildQualifiedName(sc.QualifiedName())
		}
	}

	// Parse the root mapping element
	if elemCtx := ctx.ImportMappingElement(); elemCtx != nil {
		stmt.RootElement = buildImportMappingElement(elemCtx.(*parser.ImportMappingElementContext))
	}

	b.statements = append(b.statements, stmt)
}

// buildImportMappingElement converts an importMappingElement context to an AST node.
func buildImportMappingElement(ctx *parser.ImportMappingElementContext) *ast.ImportMappingElementDef {
	elem := &ast.ImportMappingElementDef{}

	allQN := ctx.AllQualifiedName()
	allIdent := ctx.AllIdentifierOrKeyword()

	// JSON field name (left side of ->): strip QUOTED_IDENTIFIER delimiters (e.g. "" → "").
	if len(allIdent) > 0 {
		elem.JsonName = identifierOrKeywordText(allIdent[0])
	}

	// Check if this is an object mapping (has qualifiedName RHS with entity)
	// or value mapping (has identifierOrKeyword RHS with attribute name + type in parens)
	if ctx.ImportMappingHandling() != nil {
		// Object mapping: IDENTIFIER ARROW qualifiedName LPAREN handling RPAREN
		if len(allQN) >= 1 {
			elem.Entity = allQN[0].GetText()
		}
		handlingCtx := ctx.ImportMappingHandling().(*parser.ImportMappingHandlingContext)
		elem.ObjectHandling = extractImportMappingHandling(handlingCtx)

		// VIA clause: second qualifiedName
		if ctx.VIA() != nil && len(allQN) >= 2 {
			elem.Association = allQN[1].GetText()
		}

		// Nested children
		for _, childCtx := range ctx.AllImportMappingElement() {
			child := buildImportMappingElement(childCtx.(*parser.ImportMappingElementContext))
			elem.Children = append(elem.Children, child)
		}
	} else {
		// Value mapping: IDENTIFIER ARROW identifierOrKeyword LPAREN type (COMMA KEY)? RPAREN
		if len(allIdent) >= 2 {
			elem.Attribute = identifierOrKeywordText(allIdent[1])
		}
		if vtCtx := ctx.ImportMappingValueType(); vtCtx != nil {
			elem.DataType = extractImportValueType(vtCtx.(*parser.ImportMappingValueTypeContext))
		}
		if ctx.KEY() != nil {
			elem.IsKey = true
		}
	}

	return elem
}

// extractImportMappingHandling extracts the handling string from the grammar context.
func extractImportMappingHandling(ctx *parser.ImportMappingHandlingContext) string {
	if ctx.CREATE() != nil {
		return "Create"
	}
	if ctx.FIND() != nil {
		return "Find"
	}
	if ctx.UPDATE() != nil {
		return "FindOrCreate"
	}
	if ctx.IDENTIFIER() != nil {
		return ctx.IDENTIFIER().GetText()
	}
	return "Create"
}

// extractImportValueType maps a grammar type keyword to a string.
func extractImportValueType(ctx *parser.ImportMappingValueTypeContext) string {
	if ctx.STRING_TYPE() != nil {
		return "String"
	}
	if ctx.INTEGER_TYPE() != nil {
		return "Integer"
	}
	if ctx.LONG_TYPE() != nil {
		return "Long"
	}
	if ctx.DECIMAL_TYPE() != nil {
		return "Decimal"
	}
	if ctx.BOOLEAN_TYPE() != nil {
		return "Boolean"
	}
	if ctx.DATETIME_TYPE() != nil {
		return "DateTime"
	}
	if ctx.DATE_TYPE() != nil {
		return "Date"
	}
	if ctx.BINARY_TYPE() != nil {
		return "Binary"
	}
	return "String"
}
