// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// agentSelectedModelRule flags an agent created without a selected model
// (ako/mxcli#2031).
//
// Agent Editor 2.2+ imports an agent only with one: AgentFromModel_Validate
// requires SelectedModelID, and AgentFromModel_CreateUpdateAgent retrieves the
// deployed model where Model = SelectedModelID (measured in AgentEditorCommons
// 2.2.1 and 2.6.0; 1.1.0 does not read it). The refusal comes at startup, from
// ASU_AgentEditor, and it stops the import of every agent document, not just
// this one. An agent that names a Model is also Studio Pro's AE018 ("Model
// version is required for an agent"), so then it is an error and refuses exec
// (ako/mxcli#2289); with no Model at all it stays a warning. A create or modify
// is left to validateAgentDocumentsLeftUnset, which can see whether there is a
// stored selected model to keep (#2089).
const agentSelectedModelRule = "MDL-AGENT02"

// agentSelectedModelMissing is the rule's message, shared by the static and
// the project check. hasModel picks the error wording.
func agentSelectedModelMissing(verb string, name string, hasModel bool) string {
	msg := fmt.Sprintf("agent %s %s no SelectedModel: Agent Editor 2.2+ refuses to import it at startup "+
		"(\"Model with name  is referenced but could not be found\"), and that stops the import of every agent", name, verb)
	if hasModel {
		msg += "; Studio Pro reports AE018 \"Model version is required for an agent\""
	}
	return msg
}

// ValidateAgentSelectedModel flags a create agent with no SelectedModel: an
// error when it names a Model, a warning when it does not.
func ValidateAgentSelectedModel(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.CreateAgentStmt)
		if !ok || s.CreateOrModify || (s.SelectedModel != nil && *s.SelectedModel != "") {
			continue
		}
		sev := linter.SeverityWarning
		if s.Model != nil {
			sev = linter.SeverityError
		}
		out = append(out, linter.Violation{
			RuleID:   agentSelectedModelRule,
			Severity: sev,
			Message:  agentSelectedModelMissing("has", s.Name.String(), s.Model != nil),
			Location: linter.Location{DocumentType: "agent", DocumentName: s.Name.String()},
			Suggestion: "Add SelectedModel: '<model id>', the id of a model the Model document's resource offers " +
				"(e.g. 'eu.anthropic.claude-sonnet-5'). With the app running, MxGenAIConnector.MxCloudDeployedModel " +
				"lists them. Only Agent Editor 1.x ignores it.",
		})
	}
	return out
}
