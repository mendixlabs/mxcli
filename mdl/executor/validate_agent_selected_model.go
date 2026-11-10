// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// agentSelectedModelRule warns on an agent created without a selected model
// (ako/mxcli#2031).
//
// Agent Editor 2.2+ imports an agent only with one: AgentFromModel_Validate
// requires SelectedModelID, and AgentFromModel_CreateUpdateAgent retrieves the
// deployed model where Model = SelectedModelID (measured in AgentEditorCommons
// 2.2.1 and 2.6.0; 1.1.0 does not read it). The refusal comes at startup, from
// ASU_AgentEditor, and it stops the import of every agent document, not just
// this one. A create or modify is not flagged: left out, it keeps the stored
// selected model.
const agentSelectedModelRule = "MDL-AGENT02"

// ValidateAgentSelectedModel warns on a create agent with no SelectedModel.
func ValidateAgentSelectedModel(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.CreateAgentStmt)
		if !ok || s.CreateOrModify || (s.SelectedModel != nil && *s.SelectedModel != "") {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   agentSelectedModelRule,
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf("agent %s has no SelectedModel: Agent Editor 2.2+ refuses to import it at startup "+
				"(\"Model with name  is referenced but could not be found\"), and that stops the import of every agent", s.Name),
			Location: linter.Location{DocumentType: "agent", DocumentName: s.Name.String()},
			Suggestion: "Add SelectedModel: '<model id>', the id of a model the Model document's resource offers " +
				"(e.g. 'eu.anthropic.claude-sonnet-5'). With the app running, MxGenAIConnector.MxCloudDeployedModel " +
				"lists them. Only Agent Editor 1.x ignores it.",
		})
	}
	return out
}
