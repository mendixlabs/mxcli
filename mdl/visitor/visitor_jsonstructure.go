// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateJsonStructureStatement is called when exiting the createJsonStructureStatement production.
//
// Grammar: JSON STRUCTURE qualifiedName (COMMENT STRING_LITERAL)? SNIPPET (STRING_LITERAL | DOLLAR_STRING) (CUSTOM_NAME_MAP LPAREN customNameMapping (COMMA customNameMapping)* RPAREN)?
func (b *Builder) ExitCreateJsonStructureStatement(ctx *parser.CreateJsonStructureStatementContext) {
	stmt := &ast.CreateJsonStructureStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	// Parse COMMENT if present (always a STRING_LITERAL).
	allStrings := ctx.AllSTRING_LITERAL()
	if ctx.COMMENT() != nil && len(allStrings) >= 1 {
		stmt.Documentation = unquoteString(allStrings[0].GetText())
	}

	// Parse SNIPPET value — can be STRING_LITERAL or DOLLAR_STRING.
	if ds := ctx.DOLLAR_STRING(); ds != nil {
		stmt.JsonSnippet = unquoteDollarString(ds.GetText())
	} else {
		// SNIPPET is a STRING_LITERAL; it's the last one (after COMMENT if present)
		idx := len(allStrings) - 1
		if idx >= 0 {
			stmt.JsonSnippet = unquoteString(allStrings[idx].GetText())
		}
	}

	// Parse CUSTOM NAME MAP if present
	if ctx.CUSTOM_NAME_MAP() != nil {
		stmt.CustomNameMap = make(map[string]string)
		for _, mapping := range ctx.AllCustomNameMapping() {
			mappingCtx := mapping.(*parser.CustomNameMappingContext)
			strings := mappingCtx.AllSTRING_LITERAL()
			if len(strings) == 2 {
				jsonKey := unquoteString(strings[0].GetText())
				customName := unquoteString(strings[1].GetText())
				stmt.CustomNameMap[jsonKey] = customName
			}
		}
	}

	// Check for CREATE OR REPLACE and doc comment
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.REPLACE() != nil || createStmt.MODIFY() != nil) {
			stmt.CreateOrReplace = true
		}
	}
	if doc := findDocCommentText(ctx); doc != "" {
		stmt.Documentation = doc
	}

	b.statements = append(b.statements, stmt)
}
