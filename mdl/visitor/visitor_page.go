// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ============================================================================
// Page Statements
// ============================================================================

// ExitCreatePageStatement is called when exiting the createPageStatement production.
func (b *Builder) ExitCreatePageStatement(ctx *parser.CreatePageStatementContext) {
	stmt := b.buildPageV3(ctx)
	b.statements = append(b.statements, stmt)
}

// ExitCreateSnippetStatement is called when exiting the createSnippetStatement production.
func (b *Builder) ExitCreateSnippetStatement(ctx *parser.CreateSnippetStatementContext) {
	stmt := b.buildSnippetV3(ctx)
	b.statements = append(b.statements, stmt)
}

// buildPageParameters converts page parameter list to []ast.PageParameter.
func buildPageParameters(ctx parser.IPageParameterListContext) []ast.PageParameter {
	if ctx == nil {
		return nil
	}
	listCtx := ctx.(*parser.PageParameterListContext)
	var params []ast.PageParameter

	for _, param := range listCtx.AllPageParameter() {
		paramCtx := param.(*parser.PageParameterContext)
		name := ""
		if id := paramCtx.IDENTIFIER(); id != nil {
			name = id.GetText()
		} else if v := paramCtx.VARIABLE(); v != nil {
			// VARIABLE token is $name, strip the $ prefix
			name = strings.TrimPrefix(v.GetText(), "$")
		} else if qid := paramCtx.QUOTED_IDENTIFIER(); qid != nil {
			// Quoted name for reserved-keyword params, e.g. "List". See issue #114.
			name = unquoteIdentifier(qid.GetText())
		}
		var entityType ast.QualifiedName
		var dataType ast.DataType
		if dt := paramCtx.DataType(); dt != nil {
			dataType = buildDataType(dt)
			// For backward compatibility, also populate EntityType for entity/enum refs
			dtCtx := dt.(*parser.DataTypeContext)
			if qn := dtCtx.QualifiedName(); qn != nil {
				entityType = buildQualifiedName(qn)
			}
		}
		params = append(params, ast.PageParameter{
			Name:       name,
			EntityType: entityType,
			Type:       dataType,
		})
	}
	return params
}

// buildSnippetParameters converts snippet parameter list to []ast.PageParameter.
func buildSnippetParameters(ctx parser.ISnippetParameterListContext) []ast.PageParameter {
	if ctx == nil {
		return nil
	}
	listCtx := ctx.(*parser.SnippetParameterListContext)
	var params []ast.PageParameter

	for _, param := range listCtx.AllSnippetParameter() {
		paramCtx := param.(*parser.SnippetParameterContext)
		name := ""
		if id := paramCtx.IDENTIFIER(); id != nil {
			name = strings.TrimPrefix(id.GetText(), "$")
		}
		if v := paramCtx.VARIABLE(); v != nil {
			name = strings.TrimPrefix(v.GetText(), "$")
		}
		if qid := paramCtx.QUOTED_IDENTIFIER(); qid != nil {
			// Quoted name for reserved-keyword params, e.g. "List". See issue #114.
			name = unquoteIdentifier(qid.GetText())
		}
		var entityType ast.QualifiedName
		if dt := paramCtx.DataType(); dt != nil {
			dtCtx := dt.(*parser.DataTypeContext)
			if qn := dtCtx.QualifiedName(); qn != nil {
				entityType = buildQualifiedName(qn)
			}
		}
		params = append(params, ast.PageParameter{
			Name:       name,
			EntityType: entityType,
		})
	}
	return params
}
