// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ExitCreateDataTransformerStatement handles CREATE DATA TRANSFORMER.
func (b *Builder) ExitCreateDataTransformerStatement(ctx *parser.CreateDataTransformerStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.CreateDataTransformerStmt{
		Name: buildQualifiedName(qn),
	}

	// Source type: JSON or XML
	if ctx.JSON() != nil {
		stmt.SourceType = "JSON"
	} else if ctx.XML() != nil {
		stmt.SourceType = "XML"
	}

	// Source content — the STRING_LITERAL after JSON/XML
	if sl := ctx.STRING_LITERAL(); sl != nil {
		stmt.SourceJSON = unquoteString(sl.GetText())
	}

	// Steps
	for _, stepCtx := range ctx.AllDataTransformerStep() {
		sc, ok := stepCtx.(*parser.DataTransformerStepContext)
		if !ok || sc == nil {
			continue
		}
		step := ast.DataTransformerStepDef{}
		if sc.JSLT() != nil {
			step.Technology = "JSLT"
		} else if sc.XSLT() != nil {
			step.Technology = "XSLT"
		}
		if sl := sc.STRING_LITERAL(); sl != nil {
			step.Expression = unquoteString(sl.GetText())
		} else if ds := sc.DOLLAR_STRING(); ds != nil {
			step.Expression = unquoteDollarString(ds.GetText())
		}
		stmt.Steps = append(stmt.Steps, step)
	}

	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	b.statements = append(b.statements, stmt)
}
