// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreatePublishedMCPServiceStatement bridges the grammar to AST
// (ako/mxcli#2020).
func (b *Builder) ExitCreatePublishedMCPServiceStatement(ctx *parser.CreatePublishedMCPServiceStatementContext) {
	stmt := &ast.CreatePublishedMCPServiceStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	stmt.OuterDocumentation, stmt.DocumentationSet = findDocComment(ctx)
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteStringLit(lit)
	}

	b.checkModelProperties(ctx.AllModelProperty(), &publishedMCPServiceSchema)
	props := parseModelProps(ctx.AllModelProperty())
	stmt.ServerName = props["servername"]
	stmt.EndpointPath = props["endpointpath"]
	stmt.Version = props["version"]
	stmt.ProtocolVersion = props["protocolversion"]
	stmt.AuthenticationEnabled = strings.EqualFold(props["authenticationenabled"], "true")
	if v, ok := props["authenticationmicroflow"]; ok {
		qn := parseQualifiedNameString(v)
		stmt.AuthenticationMicroflow = &qn
	}
	stmt.AuthorizationServerURL = props["authorizationserverurl"]
	stmt.Scopes = props["scopes"]
	stmt.InnerDocumentation = props["documentation"]

	if body := ctx.PublishedMCPServiceBody(); body != nil {
		for _, blk := range body.(*parser.PublishedMCPServiceBodyContext).AllPublishedMCPToolBlock() {
			stmt.Tools = append(stmt.Tools, b.publishedMCPToolDef(blk.(*parser.PublishedMCPToolBlockContext)))
		}
	}

	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	b.statements = append(b.statements, stmt)
}

// publishedMCPToolDef reads a `tool name ( … )` block, on create and on
// alter … add tool.
func (b *Builder) publishedMCPToolDef(blk *parser.PublishedMCPToolBlockContext) ast.PublishedMCPToolDef {
	b.checkModelProperties(blk.AllModelProperty(), &publishedMCPToolSchema)
	props := parseModelProps(blk.AllModelProperty())
	td := ast.PublishedMCPToolDef{
		Description: props["description"],
		Title:       props["title"],
		Schema:      props["schema"],
	}
	if iok := blk.IdentifierOrKeyword(); iok != nil {
		td.Name = unquoteIdentifier(iok.GetText())
	}
	if v, ok := props["microflow"]; ok {
		qn := parseQualifiedNameString(v)
		td.Microflow = &qn
	}
	return td
}

// exitAlterPublishedMCPServiceStatement builds ALTER PUBLISHED MCP SERVICE.
func (b *Builder) exitAlterPublishedMCPServiceStatement(ctx *parser.AlterStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}
	stmt := &ast.AlterPublishedMCPServiceStmt{Name: buildQualifiedName(qn), Sets: map[string]string{}}
	for _, a := range ctx.AllAlterPublishedMCPServiceAction() {
		act := a.(*parser.AlterPublishedMCPServiceActionContext)
		switch {
		case act.SET() != nil:
			for k, v := range parseAgentEditorAlterAssignments(act.AllAgentEditorAlterAssignment()) {
				stmt.Sets[k] = v
			}
		case act.ADD() != nil && act.PublishedMCPToolBlock() != nil:
			stmt.AddTools = append(stmt.AddTools, b.publishedMCPToolDef(act.PublishedMCPToolBlock().(*parser.PublishedMCPToolBlockContext)))
		case act.DROP() != nil:
			if iok := act.IdentifierOrKeyword(); iok != nil {
				stmt.DropTools = append(stmt.DropTools, unquoteIdentifier(iok.GetText()))
			}
		}
	}
	b.statements = append(b.statements, stmt)
}
