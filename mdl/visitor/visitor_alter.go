// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ExitAlterStatement dispatches ALTER statements to their specific handlers.
// Sub-types (PAGE, SNIPPET, STYLING, WORKFLOW) are handled by dedicated visitor files;
// OData ALTER is handled inline below.
func (b *Builder) ExitAlterStatement(ctx *parser.AlterStatementContext) {
	// Handle ALTER PAGE / ALTER SNIPPET
	if (ctx.PAGE() != nil || ctx.SNIPPET() != nil) && len(ctx.AllAlterPageOperation()) > 0 {
		b.exitAlterPageStatement(ctx)
		return
	}

	// Handle ALTER STYLING
	if ctx.STYLING() != nil {
		b.exitAlterStylingStatement(ctx)
		return
	}

	// Handle ALTER WORKFLOW
	if ctx.WORKFLOW() != nil && len(ctx.AllAlterWorkflowAction()) > 0 {
		b.exitAlterWorkflowStatement(ctx)
		return
	}

	// Handle ALTER PUBLISHED REST SERVICE
	if ctx.PUBLISHED() != nil && ctx.REST() != nil && ctx.SERVICE() != nil {
		b.exitAlterPublishedRestServiceStatement(ctx)
		return
	}

	// Handle agent-editor ALTER statements
	if ctx.MODEL() != nil || ctx.AGENT() != nil ||
		(ctx.KNOWLEDGE() != nil && ctx.BASE() != nil) ||
		(ctx.CONSUMED() != nil && ctx.MCP() != nil && ctx.SERVICE() != nil) {
		b.exitAlterAgentEditorStatement(ctx)
		return
	}

	if ctx.ODATA() == nil {
		return // Not an OData alter - handled elsewhere
	}

	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	changes := make(map[string]any)
	for _, propCtx := range ctx.AllOdataAlterAssignment() {
		prop := propCtx.(*parser.OdataAlterAssignmentContext)
		name := identifierOrKeywordText(prop.IdentifierOrKeyword())
		val := prop.OdataPropertyValue()
		if val != nil {
			changes[name] = odataValueText(val.(*parser.OdataPropertyValueContext))
		}
	}

	if ctx.CLIENT() != nil {
		b.statements = append(b.statements, &ast.AlterODataClientStmt{
			Name:    buildQualifiedName(qn),
			Changes: changes,
		})
	} else if ctx.SERVICE() != nil {
		b.statements = append(b.statements, &ast.AlterODataServiceStmt{
			Name:    buildQualifiedName(qn),
			Changes: changes,
		})
	}
}
