// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

func (b *Builder) ExitCreateModuleStatement(ctx *parser.CreateModuleStatementContext) {
	name := ""
	if iok := ctx.IdentifierOrKeyword(); iok != nil {
		name = identifierOrKeywordText(iok)
	}
	stmt := &ast.CreateModuleStmt{Name: name}
	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		stmt.CreateOrModify = createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil)
	}
	// The doc comment is the domain model's documentation, the only
	// documentation a module has (mendixlabs/mxcli#1314).
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)
	if body := ctx.ModuleSettingsBody(); body != nil {
		stmt.Settings = b.moduleSettings("module "+name, body)
	}
	b.statements = append(b.statements, stmt)
}

// ----------------------------------------------------------------------------
// Enumeration Statements
// ----------------------------------------------------------------------------

// ExitCreateEnumerationStatement is called when exiting the createEnumerationStatement production.
