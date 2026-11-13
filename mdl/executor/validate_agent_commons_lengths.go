// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"unicode/utf8"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// agentCommonsLengthRule warns on an agent value longer than the Agent Commons
// attribute it is copied into (ako/mxcli#2211).
//
// At startup AgentEditorCommons.AgentFromModel_CreateUpdateAgent copies each
// agent document into AgentCommons entities whose String attributes have fixed
// lengths (measured in Agent Editor 2.7.0 on Mendix 11.16). A longer value fails
// that import ("tried setting a value of length 203") and Agent Commons keeps
// the previous version of the agent, so the app runs with stale behaviour while
// check, exec and mx check are all clean. It warns, like MDL-AGENT01..03,
// because the lengths belong to the AgentCommons module the project has
// installed, which check does not read and an upgrade may widen.
const agentCommonsLengthRule = "MDL-AGENT04"

// Agent Commons attribute lengths, in characters (Agent Editor 2.7.0).
const (
	agentCommonsTitleLen           = 50   // AgentCommons.Agent.Title, from the document name
	agentCommonsDescriptionLen     = 200  // AgentCommons.Agent.Description
	agentCommonsToolNameLen        = 200  // AgentCommons.Tool.Name
	agentCommonsToolTitleLen       = 200  // AgentCommons.Tool.DisplayTitle
	agentCommonsToolDescriptionLen = 4096 // AgentCommons.Tool.Description
)

// ValidateAgentCommonsLengths warns on a create [or modify] agent whose name,
// Description, or a tool's name, display title or description is longer than
// Agent Commons stores. Lengths are counted in characters, not bytes.
func ValidateAgentCommonsLengths(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.CreateAgentStmt)
		if !ok {
			continue
		}
		check := func(what, value, attr string, max int) {
			n := utf8.RuneCountInString(value)
			if n <= max {
				return
			}
			out = append(out, linter.Violation{
				RuleID:   agentCommonsLengthRule,
				Severity: linter.SeverityWarning,
				Message: fmt.Sprintf("agent %s: %s is %d characters, longer than %s (%d): Agent Editor's import "+
					"fails at startup and the previous version of the agent stays in use", s.Name, what, n, attr, max),
				Location: linter.Location{DocumentType: "agent", DocumentName: s.Name.String()},
				Suggestion: fmt.Sprintf("Shorten it to %d characters or fewer. mxbuild does not read agent documents, "+
					"so the build is green and only the app log shows the failed import.", max),
			})
		}
		check("the name "+s.Name.Name, s.Name.Name, "AgentCommons.Agent.Title", agentCommonsTitleLen)
		check("the Description", s.Description, "AgentCommons.Agent.Description", agentCommonsDescriptionLen)
		for _, t := range s.Tools {
			// An MCP block's name is the service document, not a tool name
			// Agent Commons stores.
			if t.ToolType != "MCP" {
				check("tool "+t.Name+"'s name", t.Name, "AgentCommons.Tool.Name", agentCommonsToolNameLen)
			}
			if t.DisplayTitle != nil {
				check("tool "+t.Name+"'s DisplayTitle", *t.DisplayTitle, "AgentCommons.Tool.DisplayTitle", agentCommonsToolTitleLen)
			}
			check("tool "+t.Name+"'s Description", t.Description, "AgentCommons.Tool.Description", agentCommonsToolDescriptionLen)
		}
	}
	return out
}
