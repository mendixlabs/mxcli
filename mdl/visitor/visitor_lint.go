// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ExitLintStatement handles LINT and SHOW LINT RULES statements
func (b *Builder) ExitLintStatement(ctx *parser.LintStatementContext) {
	stmt := &ast.LintStmt{
		Format: ast.LintFormatText, // Default format
	}

	// Check for SHOW LINT RULES
	if ctx.SHOW() != nil && ctx.RULES() != nil {
		stmt.ShowRules = true
		b.statements = append(b.statements, stmt)
		return
	}

	// Check for target
	if target := ctx.LintTarget(); target != nil {
		if target.STAR() != nil && target.QualifiedName() == nil {
			// Just STAR - lint all (target stays nil)
		} else if target.STAR() != nil && target.QualifiedName() != nil {
			// Module.* - lint all in module
			qn := buildQualifiedName(target.QualifiedName())
			stmt.Target = &qn
			stmt.ModuleOnly = true
		} else if target.QualifiedName() != nil {
			// Specific element
			qn := buildQualifiedName(target.QualifiedName())
			stmt.Target = &qn
		}
	}

	// Check for format
	if format := ctx.LintFormat(); format != nil {
		if format.JSON() != nil {
			stmt.Format = ast.LintFormatJSON
		} else if format.SARIF() != nil {
			stmt.Format = ast.LintFormatSARIF
		} else if format.TEXT() != nil {
			stmt.Format = ast.LintFormatText
		}
	}

	b.statements = append(b.statements, stmt)
}
