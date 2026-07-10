// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ExitCreateImportMappingStatement is called when exiting the createImportMappingStatement production.
func (b *Builder) ExitCreateImportMappingStatement(ctx *parser.CreateImportMappingStatementContext) {
	stmt := &ast.CreateImportMappingStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	// Parse WITH clause
	if wc := ctx.ImportMappingWithClause(); wc != nil {
		sc := wc.(*parser.ImportMappingWithClauseContext)
		if sc.JSON() != nil {
			stmt.SchemaKind = "JSON_STRUCTURE"
		} else {
			stmt.SchemaKind = "XML_SCHEMA"
		}
		if sc.QualifiedName() != nil {
			stmt.SchemaRef = buildQualifiedName(sc.QualifiedName())
		}
	}

	// Parse root element
	if root := ctx.ImportMappingRootElement(); root != nil {
		stmt.RootElement = buildImportRootElement(root.(*parser.ImportMappingRootElementContext))
	}

	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	b.statements = append(b.statements, stmt)
}

// buildImportRootElement builds the root element from the grammar context.
// Grammar: importMappingObjectHandling qualifiedName LBRACE importMappingChild (COMMA importMappingChild)* RBRACE
func buildImportRootElement(ctx *parser.ImportMappingRootElementContext) *ast.ImportMappingElementDef {
	elem := &ast.ImportMappingElementDef{}

	// Object handling: CREATE | FIND | FIND OR CREATE
	if hCtx := ctx.ImportMappingObjectHandling(); hCtx != nil {
		elem.ObjectHandling = extractObjectHandling(hCtx.(*parser.ImportMappingObjectHandlingContext))
	}

	// Entity name
	if ctx.QualifiedName() != nil {
		elem.Entity = ctx.QualifiedName().GetText()
	}

	// Children
	for _, childCtx := range ctx.AllImportMappingChild() {
		child := buildImportChild(childCtx.(*parser.ImportMappingChildContext))
		elem.Children = append(elem.Children, child)
	}

	return elem
}

// buildImportChild builds a child element from the grammar context.
// Four alternatives:
// 1. handling assocPath = jsonKey { children }   (nested object with children)
// 2. handling assocPath = jsonKey                 (leaf object)
// 3. attr = Module.MF(jsonField)                 (value transform)
// 4. attr = jsonField KEY?                        (value assignment)
func buildImportChild(ctx *parser.ImportMappingChildContext) *ast.ImportMappingElementDef {
	elem := &ast.ImportMappingElementDef{}

	// Check if this is an object mapping (has handling keyword)
	if hCtx := ctx.ImportMappingObjectHandling(); hCtx != nil {
		// Object mapping: CREATE/FIND/FIND OR CREATE Assoc/Entity = jsonKey
		elem.ObjectHandling = extractObjectHandling(hCtx.(*parser.ImportMappingObjectHandlingContext))

		// Association path: qualifiedName SLASH qualifiedName
		allQN := ctx.AllQualifiedName()
		if len(allQN) >= 2 {
			elem.Association = allQN[0].GetText()
			elem.Entity = allQN[1].GetText()
		}

		// JSON key after EQUALS
		allIdent := ctx.AllIdentifierOrKeyword()
		if len(allIdent) >= 1 {
			elem.JsonName = identifierOrKeywordText(allIdent[0])
		}

		// Nested children
		for _, childCtx := range ctx.AllImportMappingChild() {
			child := buildImportChild(childCtx.(*parser.ImportMappingChildContext))
			elem.Children = append(elem.Children, child)
		}
	} else if ctx.LPAREN() != nil {
		// Value transform: attr = Module.MF(jsonField)
		allIdent := ctx.AllIdentifierOrKeyword()
		if len(allIdent) >= 1 {
			elem.Attribute = identifierOrKeywordText(allIdent[0])
		}
		allQN := ctx.AllQualifiedName()
		if len(allQN) >= 1 {
			elem.Converter = allQN[0].GetText()
		}
		if len(allIdent) >= 2 {
			elem.ConverterParam = identifierOrKeywordText(allIdent[1])
		}
	} else {
		// Value assignment: attr = jsonField KEY?
		allIdent := ctx.AllIdentifierOrKeyword()
		if len(allIdent) >= 1 {
			elem.Attribute = identifierOrKeywordText(allIdent[0])
		}
		if len(allIdent) >= 2 {
			elem.JsonName = identifierOrKeywordText(allIdent[1])
		}
		if ctx.KEY() != nil {
			elem.IsKey = true
		}
	}

	return elem
}

// ExitCreateExportMappingStatement is called when exiting the createExportMappingStatement production.
func (b *Builder) ExitCreateExportMappingStatement(ctx *parser.CreateExportMappingStatementContext) {
	stmt := &ast.CreateExportMappingStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	// Parse WITH clause
	if wc := ctx.ExportMappingWithClause(); wc != nil {
		sc := wc.(*parser.ExportMappingWithClauseContext)
		if sc.JSON() != nil {
			stmt.SchemaKind = "JSON_STRUCTURE"
		} else {
			stmt.SchemaKind = "XML_SCHEMA"
		}
		if sc.QualifiedName() != nil {
			stmt.SchemaRef = buildQualifiedName(sc.QualifiedName())
		}
	}

	// Parse null values clause
	if nc := ctx.ExportMappingNullValuesClause(); nc != nil {
		ncc := nc.(*parser.ExportMappingNullValuesClauseContext)
		if ncc.IdentifierOrKeyword() != nil {
			stmt.NullValueOption = identifierOrKeywordText(ncc.IdentifierOrKeyword().(*parser.IdentifierOrKeywordContext))
		}
	}

	// Parse root element
	if root := ctx.ExportMappingRootElement(); root != nil {
		stmt.RootElement = buildExportRootElement(root.(*parser.ExportMappingRootElementContext))
	}

	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	b.statements = append(b.statements, stmt)
}

// buildExportRootElement builds the root element from the grammar context.
// Grammar: qualifiedName LBRACE exportMappingChild (COMMA exportMappingChild)* RBRACE
func buildExportRootElement(ctx *parser.ExportMappingRootElementContext) *ast.ExportMappingElementDef {
	elem := &ast.ExportMappingElementDef{}

	if ctx.QualifiedName() != nil {
		elem.Entity = ctx.QualifiedName().GetText()
	}

	for _, childCtx := range ctx.AllExportMappingChild() {
		child := buildExportChild(childCtx.(*parser.ExportMappingChildContext))
		elem.Children = append(elem.Children, child)
	}

	return elem
}

// buildExportChild builds a child element from the grammar context.
// Three alternatives:
// 1. Assoc/Entity AS jsonKey { children }   (nested object with children)
// 2. Assoc/Entity AS jsonKey                 (leaf object)
// 3. jsonField = Attr                        (value assignment)
func buildExportChild(ctx *parser.ExportMappingChildContext) *ast.ExportMappingElementDef {
	elem := &ast.ExportMappingElementDef{}

	allQN := ctx.AllQualifiedName()

	if len(allQN) >= 2 {
		// Object mapping: Assoc/Entity AS jsonKey
		elem.Association = allQN[0].GetText()
		elem.Entity = allQN[1].GetText()

		// JSON key after AS
		allIdent := ctx.AllIdentifierOrKeyword()
		if len(allIdent) >= 1 {
			elem.JsonName = identifierOrKeywordText(allIdent[0].(*parser.IdentifierOrKeywordContext))
		}

		// Nested children
		for _, childCtx := range ctx.AllExportMappingChild() {
			child := buildExportChild(childCtx.(*parser.ExportMappingChildContext))
			elem.Children = append(elem.Children, child)
		}
	} else {
		// Value mapping: jsonField = Attr
		allIdent := ctx.AllIdentifierOrKeyword()
		if len(allIdent) >= 1 {
			elem.JsonName = identifierOrKeywordText(allIdent[0].(*parser.IdentifierOrKeywordContext))
		}
		if len(allIdent) >= 2 {
			elem.Attribute = identifierOrKeywordText(allIdent[1].(*parser.IdentifierOrKeywordContext))
		}
	}

	return elem
}

// buildImportFromMappingStatement builds an ImportFromMappingStmt from the grammar context.
// Grammar: (VARIABLE EQUALS)? IMPORT FROM MAPPING qualifiedName LPAREN VARIABLE RPAREN onErrorClause?
func buildImportFromMappingStatement(ctx antlr.ParserRuleContext) ast.MicroflowStatement {
	c := ctx.(*parser.ImportFromMappingStatementContext)
	stmt := &ast.ImportFromMappingStmt{
		Mapping: buildQualifiedName(c.QualifiedName()),
	}

	vars := c.AllVARIABLE()
	if c.EQUALS() != nil && len(vars) >= 2 {
		stmt.OutputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
		stmt.SourceVariable = strings.TrimPrefix(vars[1].GetText(), "$")
	} else if len(vars) >= 1 {
		stmt.SourceVariable = strings.TrimPrefix(vars[0].GetText(), "$")
	}

	if ec := c.OnErrorClause(); ec != nil {
		stmt.ErrorHandling = buildOnErrorClause(ec)
	}

	return stmt
}

// buildExportToMappingStatement builds an ExportToMappingStmt from the grammar context.
// Grammar: (VARIABLE EQUALS)? EXPORT TO MAPPING qualifiedName LPAREN VARIABLE RPAREN onErrorClause?
func buildExportToMappingStatement(ctx antlr.ParserRuleContext) ast.MicroflowStatement {
	c := ctx.(*parser.ExportToMappingStatementContext)
	stmt := &ast.ExportToMappingStmt{
		Mapping: buildQualifiedName(c.QualifiedName()),
	}

	vars := c.AllVARIABLE()
	if c.EQUALS() != nil && len(vars) >= 2 {
		stmt.OutputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
		stmt.SourceVariable = strings.TrimPrefix(vars[1].GetText(), "$")
	} else if len(vars) >= 1 {
		stmt.SourceVariable = strings.TrimPrefix(vars[0].GetText(), "$")
	}

	if ec := c.OnErrorClause(); ec != nil {
		stmt.ErrorHandling = buildOnErrorClause(ec)
	}

	return stmt
}

// buildTransformJsonStatement builds a TransformJsonStmt from the grammar context.
// Grammar: (VARIABLE EQUALS)? TRANSFORM VARIABLE WITH qualifiedName onErrorClause?
func buildTransformJsonStatement(ctx antlr.ParserRuleContext) ast.MicroflowStatement {
	c := ctx.(*parser.TransformJsonStatementContext)
	stmt := &ast.TransformJsonStmt{
		Transformation: buildQualifiedName(c.QualifiedName()),
	}

	vars := c.AllVARIABLE()
	if c.EQUALS() != nil && len(vars) >= 2 {
		stmt.OutputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
		stmt.InputVariable = strings.TrimPrefix(vars[1].GetText(), "$")
	} else if len(vars) >= 1 {
		stmt.InputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
	}

	if ec := c.OnErrorClause(); ec != nil {
		stmt.ErrorHandling = buildOnErrorClause(ec)
	}

	return stmt
}

// extractObjectHandling extracts the handling mode from the grammar context.
func extractObjectHandling(ctx *parser.ImportMappingObjectHandlingContext) string {
	if ctx.FIND() != nil && ctx.OR() != nil {
		return "FindOrCreate"
	}
	if ctx.FIND() != nil {
		return "Find"
	}
	return "Create"
}
