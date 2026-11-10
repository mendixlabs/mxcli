// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// AgentWithoutSelectedModelRule reports a stored agent with no selected model
// (ako/mxcli#2089, #2031).
//
// Agent Editor 2.2+ imports an agent only with one: AgentFromModel_Validate
// requires SelectedModelID (measured in AgentEditorCommons 2.2.1 and 2.6.0;
// 1.1.0 does not read it). The refusal comes at startup, from ASU_AgentEditor,
// and stops the import of every agent document. `mx check` and the build are
// green. `mxcli check` warns as MDL-AGENT02 when a script writes one; this rule
// finds the ones a project already holds, whoever wrote them.
type AgentWithoutSelectedModelRule struct{}

// NewAgentWithoutSelectedModelRule creates the rule.
func NewAgentWithoutSelectedModelRule() *AgentWithoutSelectedModelRule {
	return &AgentWithoutSelectedModelRule{}
}

func (r *AgentWithoutSelectedModelRule) ID() string       { return "MPR014" }
func (r *AgentWithoutSelectedModelRule) Name() string     { return "AgentWithoutSelectedModel" }
func (r *AgentWithoutSelectedModelRule) Category() string { return "correctness" }
func (r *AgentWithoutSelectedModelRule) DefaultSeverity() linter.Severity {
	return linter.SeverityWarning
}

func (r *AgentWithoutSelectedModelRule) Description() string {
	return "Checks for agents with no selected model, which Agent Editor 2.2+ refuses to import at startup, stopping the import of every agent"
}

// Check reports each agent without a selected model, unless the project's
// AgentEditorCommons predates 2.2 and does not read it.
func (r *AgentWithoutSelectedModelRule) Check(ctx *linter.LintContext) []linter.Violation {
	if v := ctx.MarketplaceModuleVersion("AgentEditorCommons"); v != "" && versionBefore(v, 2, 2) {
		return nil
	}
	var out []linter.Violation
	for a := range ctx.Agents() {
		if a.Setting != "" {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   r.ID(),
			Severity: r.DefaultSeverity(),
			Message: fmt.Sprintf("agent %s has no selected model: Agent Editor 2.2+ refuses to import it at startup "+
				"(\"Model with name  is referenced but could not be found\"), and that stops the import of every agent", a.QualifiedName),
			Location:   linter.Location{Module: a.ModuleName, DocumentType: "agent", DocumentName: a.QualifiedName},
			Suggestion: fmt.Sprintf("alter agent %s set SelectedModel = '<model id>', or pick the model in the Agent Editor", a.QualifiedName),
		})
	}
	return out
}

// ConsumedMCPServiceWithoutEndpointRule reports a stored consumed MCP service
// with no endpoint constant (ako/mxcli#2089, #2060).
//
// AgentEditorCommons' ConsumedMCPService_CreateUpdateFromModel skips a service
// whose endpoint constant has no value ("No value could be found for the local
// configuration for the constant"), so an agent that uses it gets none of its
// tools. Measured in AgentEditorCommons 1.1.0, 2.2.1 and 2.6.0. `mxcli check`
// warns as MDL-AGENT03 when a script writes one.
type ConsumedMCPServiceWithoutEndpointRule struct{}

// NewConsumedMCPServiceWithoutEndpointRule creates the rule.
func NewConsumedMCPServiceWithoutEndpointRule() *ConsumedMCPServiceWithoutEndpointRule {
	return &ConsumedMCPServiceWithoutEndpointRule{}
}

func (r *ConsumedMCPServiceWithoutEndpointRule) ID() string { return "MPR015" }
func (r *ConsumedMCPServiceWithoutEndpointRule) Name() string {
	return "ConsumedMCPServiceWithoutEndpoint"
}
func (r *ConsumedMCPServiceWithoutEndpointRule) Category() string {
	return "correctness"
}
func (r *ConsumedMCPServiceWithoutEndpointRule) DefaultSeverity() linter.Severity {
	return linter.SeverityWarning
}

func (r *ConsumedMCPServiceWithoutEndpointRule) Description() string {
	return "Checks for consumed MCP services with no endpoint constant, which Agent Editor skips at startup, so they connect nowhere"
}

// Check reports each consumed MCP service without an endpoint.
func (r *ConsumedMCPServiceWithoutEndpointRule) Check(ctx *linter.LintContext) []linter.Violation {
	var out []linter.Violation
	for s := range ctx.ConsumedMCPServices() {
		if s.Setting != "" {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   r.ID(),
			Severity: r.DefaultSeverity(),
			Message: fmt.Sprintf("consumed mcp service %s has no endpoint: Agent Editor skips it at startup "+
				"(\"No value could be found for the local configuration for the constant\"), so it connects nowhere", s.QualifiedName),
			Location:   linter.Location{Module: s.ModuleName, DocumentType: "consumed mcp service", DocumentName: s.QualifiedName},
			Suggestion: fmt.Sprintf("alter consumed mcp service %s set Endpoint = @Module.UrlConstant, a String constant holding the server URL", s.QualifiedName),
		})
	}
	return out
}

// versionBefore reports whether a dotted version is before major.minor.
func versionBefore(v string, major, minor int) bool {
	parts := strings.SplitN(v, ".", 3)
	maj, _ := strconv.Atoi(parts[0])
	mn := 0
	if len(parts) > 1 {
		mn, _ = strconv.Atoi(parts[1])
	}
	return maj < major || (maj == major && mn < minor)
}
