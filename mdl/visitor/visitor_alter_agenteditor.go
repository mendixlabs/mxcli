// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// exitAlterAgentEditorStatement builds an Alter*Stmt for the four
// agent-editor document types: MODEL, KNOWLEDGE BASE, CONSUMED MCP
// SERVICE (SET-only), and AGENT (SET + ADD/DROP for collections).
func (b *Builder) exitAlterAgentEditorStatement(ctx *parser.AlterStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}
	name := buildQualifiedName(qn)

	// ALTER AGENT — has SET / ADD / DROP actions.
	if ctx.AGENT() != nil {
		stmt := &ast.AlterAgentStmt{Name: name, Sets: map[string]string{}}
		for _, act := range ctx.AllAlterAgentAction() {
			b.applyAlterAgentAction(stmt, act.(*parser.AlterAgentActionContext))
		}
		b.statements = append(b.statements, stmt)
		return
	}

	// MODEL / KNOWLEDGE BASE / CONSUMED MCP SERVICE — SET-only.
	changes := parseAgentEditorAlterAssignments(ctx.AllAgentEditorAlterAssignment())
	switch {
	case ctx.MODEL() != nil:
		b.statements = append(b.statements, &ast.AlterModelStmt{Name: name, Changes: changes})
	case ctx.KNOWLEDGE() != nil && ctx.BASE() != nil:
		b.statements = append(b.statements, &ast.AlterKnowledgeBaseStmt{Name: name, Changes: changes})
	case ctx.CONSUMED() != nil && ctx.MCP() != nil && ctx.SERVICE() != nil:
		b.statements = append(b.statements, &ast.AlterConsumedMCPServiceStmt{Name: name, Changes: changes})
	}
}

// applyAlterAgentAction mutates stmt with one action (SET / ADD / DROP).
func (b *Builder) applyAlterAgentAction(stmt *ast.AlterAgentStmt, ctx *parser.AlterAgentActionContext) {
	if ctx.SET() != nil {
		for k, v := range parseAgentEditorAlterAssignments(ctx.AllAgentEditorAlterAssignment()) {
			stmt.Sets[k] = v
		}
		return
	}
	if ctx.ADD() != nil && ctx.AgentBodyBlock() != nil {
		b.appendAgentBodyBlock(stmt, ctx.AgentBodyBlock().(*parser.AgentBodyBlockContext))
		return
	}
	if ctx.DROP() != nil {
		switch {
		case ctx.TOOL() != nil:
			if iok := ctx.IdentifierOrKeyword(); iok != nil {
				stmt.DropTools = append(stmt.DropTools, iok.GetText())
			}
		case ctx.MCP() != nil && ctx.SERVICE() != nil:
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.DropMCPServices = append(stmt.DropMCPServices, buildQualifiedName(qn))
			}
		case ctx.KNOWLEDGE() != nil && ctx.BASE() != nil:
			if iok := ctx.IdentifierOrKeyword(); iok != nil {
				stmt.DropKBs = append(stmt.DropKBs, iok.GetText())
			}
		}
	}
}

// appendAgentBodyBlock converts an ADD TOOL / ADD MCP SERVICE / ADD
// KNOWLEDGE BASE block to the matching field on the ALTER AGENT stmt.
// Mirrors the CREATE AGENT body-block parsing in ExitCreateAgentStatement.
func (b *Builder) appendAgentBodyBlock(stmt *ast.AlterAgentStmt, blk *parser.AgentBodyBlockContext) {
	blockProps := parseModelProps(blk.AllModelProperty())

	switch {
	case blk.MCP() != nil && blk.SERVICE() != nil:
		td := ast.AgentToolDef{ToolType: "MCP", Enabled: true}
		if qn := blk.QualifiedName(); qn != nil {
			doc := buildQualifiedName(qn)
			td.Document = &doc
			td.Name = doc.String()
		}
		if v, ok := blockProps["enabled"]; ok {
			td.Enabled = strings.EqualFold(v, "true")
		}
		td.Description = blockProps["description"]
		stmt.AddTools = append(stmt.AddTools, td)

	case blk.KNOWLEDGE() != nil && blk.BASE() != nil:
		kbd := ast.AgentKBToolDef{Enabled: true}
		if iok := blk.IdentifierOrKeyword(); iok != nil {
			kbd.Name = iok.GetText()
		}
		if src, ok := blockProps["source"]; ok {
			sqn := parseQualifiedNameString(src)
			kbd.Source = &sqn
		}
		kbd.Collection = blockProps["collection"]
		kbd.Description = blockProps["description"]
		if v, ok := blockProps["maxresults"]; ok {
			var n int
			fmt.Sscanf(v, "%d", &n)
			kbd.MaxResults = n
		}
		if v, ok := blockProps["enabled"]; ok {
			kbd.Enabled = strings.EqualFold(v, "true")
		}
		stmt.AddKBs = append(stmt.AddKBs, kbd)

	case blk.TOOL() != nil:
		td := ast.AgentToolDef{ToolType: "Microflow", Enabled: true}
		if iok := blk.IdentifierOrKeyword(); iok != nil {
			td.Name = iok.GetText()
		}
		td.Description = blockProps["description"]
		if v, ok := blockProps["enabled"]; ok {
			td.Enabled = strings.EqualFold(v, "true")
		}
		stmt.AddTools = append(stmt.AddTools, td)
	}
}

// parseAgentEditorAlterAssignments extracts key=value pairs from a list of
// agentEditorAlterAssignment contexts. Keys are lowercased; values are the
// literal text (string literals are unquoted, dollar-strings are unwrapped).
func parseAgentEditorAlterAssignments(props []parser.IAgentEditorAlterAssignmentContext) map[string]string {
	m := make(map[string]string, len(props))
	for _, p := range props {
		pc := p.(*parser.AgentEditorAlterAssignmentContext)
		iok := pc.IdentifierOrKeyword()
		if iok == nil {
			continue
		}
		key := strings.ToLower(iok.GetText())
		val := pc.AgentEditorAlterValue()
		if val == nil {
			continue
		}
		m[key] = agentEditorAlterValueText(val.(*parser.AgentEditorAlterValueContext))
	}
	return m
}

func agentEditorAlterValueText(ctx *parser.AgentEditorAlterValueContext) string {
	if sl := ctx.STRING_LITERAL(); sl != nil {
		return unquoteString(sl.GetText())
	}
	if num := ctx.NUMBER_LITERAL(); num != nil {
		return num.GetText()
	}
	if dq := ctx.DOLLAR_STRING(); dq != nil {
		return unquoteDollarString(dq.GetText())
	}
	if bl := ctx.BooleanLiteral(); bl != nil {
		return strings.ToLower(bl.GetText())
	}
	if qn := ctx.QualifiedName(); qn != nil {
		return getQualifiedNameText(qn)
	}
	if iok := ctx.IdentifierOrKeyword(); iok != nil {
		return iok.GetText()
	}
	return ""
}
