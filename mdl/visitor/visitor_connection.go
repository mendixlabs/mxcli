// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ----------------------------------------------------------------------------
// Connection Statements
// ----------------------------------------------------------------------------

// ExitConnectStatement handles CONNECT LOCAL/PROJECT statements
func (b *Builder) ExitConnectStatement(ctx *parser.ConnectStatementContext) {
	if ctx.LOCAL() != nil {
		// CONNECT LOCAL 'path'
		strings := ctx.AllSTRING_LITERAL()
		if len(strings) > 0 {
			path := unquoteString(strings[0].GetText())
			b.statements = append(b.statements, &ast.ConnectStmt{Path: path})
		}
	}
}

// ExitDisconnectStatement is called when exiting the disconnectStatement production.
func (b *Builder) ExitDisconnectStatement(ctx *parser.DisconnectStatementContext) {
	b.statements = append(b.statements, &ast.DisconnectStmt{})
}

// ExitStatusStatement is called when exiting the statusStatement production (bare STATUS).
func (b *Builder) ExitStatusStatement(ctx *parser.StatusStatementContext) {
	b.statements = append(b.statements, &ast.StatusStmt{})
}

// ----------------------------------------------------------------------------
// Module Statements
// ----------------------------------------------------------------------------

// ExitCreateModuleStatement is called when exiting the createModuleStatement production.
