// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"strings"
)

// ExitAlterStatement dispatches ALTER statements to their specific handlers.
// Sub-types (PAGE, SNIPPET, STYLING, WORKFLOW) are handled by dedicated visitor files;
// OData ALTER is handled inline below.
func (b *Builder) ExitAlterStatement(ctx *parser.AlterStatementContext) {
	// The generic ALTER <type> Module.Name { … } (ADR-0012). Only the page
	// family is on it so far.
	if ctx.AlterDocumentType() != nil {
		b.exitAlterDocumentStatement(ctx)
		return
	}
	if ctx.MICROFLOW() != nil || ctx.NANOFLOW() != nil {
		b.exitAlterFlowStatement(ctx)
		return
	}
	// ALTER NAVIGATION / ALTER MENU: item-level menu edits.
	if ctx.NAVIGATION() != nil {
		b.exitAlterNavigationStatement(ctx)
		return
	}
	if ctx.MENU_KW() != nil {
		b.exitAlterMenuStatement(ctx)
		return
	}

	// Handle ALTER PAGES … SET LAYOUT (the bulk repoint)
	if sub := ctx.AlterPagesStylingStatement(); sub != nil {
		b.exitAlterPagesStylingStatement(sub.(*parser.AlterPagesStylingStatementContext))
		return
	}
	if sub := ctx.AlterPagesLayoutStatement(); sub != nil {
		b.exitAlterPagesLayoutStatement(sub.(*parser.AlterPagesLayoutStatementContext))
		return
	}

	// Handle ALTER STYLING
	if ctx.STYLING() != nil {
		b.exitAlterStylingStatement(ctx)
		return
	}

	// Handle ALTER WORKFLOW
	if ctx.WORKFLOW() != nil && (len(ctx.AllAlterWorkflowAction()) > 0 || len(ctx.AllAlterWorkflowOperation()) > 0) {
		b.exitAlterWorkflowStatement(ctx)
		return
	}

	// Handle ALTER PUBLISHED REST SERVICE
	if ctx.PUBLISHED() != nil && ctx.REST() != nil && ctx.SERVICE() != nil {
		b.exitAlterPublishedRestServiceStatement(ctx)
		return
	}

	// Handle ALTER CONSUMED REST SERVICE
	if ctx.ConsumedRestServiceKw() != nil {
		b.exitAlterRestClientStatement(ctx)
		return
	}

	// Handle ALTER PUBLISHED MCP SERVICE (ako/mxcli#2020)
	if ctx.PUBLISHED() != nil && ctx.MCP() != nil && ctx.SERVICE() != nil {
		b.exitAlterPublishedMCPServiceStatement(ctx)
		return
	}

	// Handle agent-editor ALTER statements
	if ctx.AiModelKw() != nil || ctx.AGENT() != nil ||
		(ctx.KNOWLEDGE() != nil && ctx.BASE() != nil) ||
		(ctx.CONSUMED() != nil && ctx.MCP() != nil && ctx.SERVICE() != nil) {
		b.exitAlterAgentEditorStatement(ctx)
		return
	}

	if ctx.ConsumedODataServiceKw() == nil && ctx.PublishedODataServiceKw() == nil {
		return // Not an OData alter - handled elsewhere
	}

	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	changes := make(map[string]any)
	var auth struct {
		set   bool
		types []string
		mf    string
	}
	// One property, in either spelling: `set ( Key: value, … )` or the old
	// `set Key = value, …` (MDL-DEPR061). Both build the same statement.
	set := func(name string, value parser.IOdataPropertyValueContext, expr parser.IExpressionContext) {
		if ctx.ConsumedODataServiceKw() != nil && isODataClientExpressionProp(name) {
			// Expression-typed: the expression as written (see visitor_odata_expression.go).
			changes[name], _ = b.odataExpressionValue(value, expr)
			return
		}
		if value != nil {
			changes[name] = odataValueText(value.(*parser.OdataPropertyValueContext))
		}
	}
	if pl, ok := ctx.OdataAlterPropertyList().(*parser.OdataAlterPropertyListContext); ok && pl != nil {
		for _, propCtx := range pl.AllOdataPropertyAssignment() {
			prop := propCtx.(*parser.OdataPropertyAssignmentContext)
			name := identifierOrKeywordText(prop.IdentifierOrKeyword())
			if ctx.PublishedODataServiceKw() != nil && strings.EqualFold(name, "authentication") {
				if types, mf, ok := b.odataAuthenticationProperty(prop); ok {
					auth.set, auth.types, auth.mf = true, types, mf
				}
				continue
			}
			set(name, prop.OdataPropertyValue(), prop.Expression())
		}
	}
	for _, propCtx := range ctx.AllOdataAlterAssignment() {
		prop := propCtx.(*parser.OdataAlterAssignmentContext)
		set(identifierOrKeywordText(prop.IdentifierOrKeyword()), prop.OdataPropertyValue(), prop.Expression())
	}
	b.recordODataAlterAssignments(ctx)

	if ctx.ConsumedODataServiceKw() != nil {
		b.statements = append(b.statements, &ast.AlterODataClientStmt{
			Name:    buildQualifiedName(qn),
			Changes: changes,
		})
	} else if ctx.PublishedODataServiceKw() != nil {
		b.statements = append(b.statements, &ast.AlterODataServiceStmt{
			Name:                buildQualifiedName(qn),
			Changes:             changes,
			AuthenticationSet:   auth.set,
			AuthenticationTypes: auth.types,
			AuthMicroflow:       auth.mf,
		})
	}
}
