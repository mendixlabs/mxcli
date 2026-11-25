// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// agentUsageTypeRule warns on an agent UsageType Agent Editor's runtime import
// does not accept (ako/mxcli#2309).
//
// AgentEditorCommons.UsageType_StringToENUM maps `task` to Single_Call and
// `chat` to Conversational, in any case, and refuses every other value —
// "Conversational" included, the enumeration's own name for Chat. mxbuild does
// not read agent documents, so such a value writes, builds and round-trips;
// the first thing to read it is the import at startup, which refuses the agent
// ("Usage Type is not valid for the Agent"), and one with none ("Usage Type is
// missing for the Agent"). In the reporter's app the published
// MCP server did not start either. A warning, like MDL-AGENT01: a newer Agent
// Editor may add a usage type, and a value mxcli has not read must stay
// writable.
const agentUsageTypeRule = "MDL-AGENT05"

// measuredUsageTypes are the values UsageType_StringToENUM accepts (Agent
// Editor 2.7.0), compared case-insensitively.
var measuredUsageTypes = []string{"Task", "Chat"}

// ValidateAgentUsageTypes warns on a UsageType outside the measured values, on
// create and on alter … set UsageType.
func ValidateAgentUsageTypes(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	check := func(name ast.QualifiedName, usage string) {
		if strings.TrimSpace(usage) == "" {
			// AgentFromModel_Validate refuses this too: "Usage Type is missing
			// for the Agent". The statement is the whole agent (usageType is
			// rebuilt, not carried), so leaving it out stores none.
			out = append(out, linter.Violation{
				RuleID:   agentUsageTypeRule,
				Severity: linter.SeverityWarning,
				Message: fmt.Sprintf("agent %s has no UsageType: at startup Agent Editor's import refuses it "+
					"(\"Usage Type is missing for the Agent\")", name),
				Location:   linter.Location{DocumentType: "agent", DocumentName: name.String()},
				Suggestion: "Add UsageType: Task (a single call) or UsageType: Chat (a conversation).",
			})
			return
		}
		for _, k := range measuredUsageTypes {
			if strings.EqualFold(usage, k) {
				return
			}
		}
		suggestion := "Use Task (a single call) or Chat (a conversation)."
		if strings.EqualFold(usage, "Conversational") || strings.EqualFold(usage, "Conversation") {
			suggestion = "Write Chat: Agent Editor stores a conversational agent's usage type as Chat."
		} else if strings.EqualFold(usage, "Single_Call") || strings.EqualFold(usage, "SingleCall") {
			suggestion = "Write Task: Agent Editor stores a single-call agent's usage type as Task."
		}
		out = append(out, linter.Violation{
			RuleID:   agentUsageTypeRule,
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf("agent %s: UsageType %q is not one Agent Editor imports (Task or Chat): at startup "+
				"the import refuses the agent", name, usage),
			Location: linter.Location{DocumentType: "agent", DocumentName: name.String()},
			Suggestion: suggestion + " mxbuild does not read the value, so it builds and only fails when " +
				"Agent Editor imports the agent at startup.",
		})
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateAgentStmt:
			check(s.Name, s.UsageType)
		case *ast.AlterAgentStmt:
			for k, v := range s.Sets {
				if strings.EqualFold(k, "usagetype") {
					check(s.Name, v)
				}
			}
		}
	}
	return out
}
