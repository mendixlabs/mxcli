// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

func (b *Builder) ExitCreateModuleStatement(ctx *parser.CreateModuleStatementContext) {
	name := ""
	if iok := ctx.IdentifierOrKeyword(); iok != nil {
		name = identifierOrKeywordText(iok)
	}
	b.statements = append(b.statements, &ast.CreateModuleStmt{
		Name: name,
	})
}

// ----------------------------------------------------------------------------
// Enumeration Statements
// ----------------------------------------------------------------------------

// ExitCreateEnumerationStatement is called when exiting the createEnumerationStatement production.
