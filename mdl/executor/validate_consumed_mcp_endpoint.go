// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// consumedMCPEndpointRule warns on a consumed MCP service created without an
// endpoint (ako/mxcli#2060).
//
// AgentEditorCommons' ConsumedMCPService_CreateUpdateFromModel reads the
// endpoint constant's value and, when there is none, logs "No value could be
// found for the local configuration for the constant" and skips the service:
// an agent that uses it gets none of its tools. Measured in AgentEditorCommons
// 1.1.0, 2.2.1 and 2.6.0, which all map endpoint/qualifiedName to
// MCPEndpointConstant. A create or modify is not flagged: left out, it keeps
// the stored endpoint.
const consumedMCPEndpointRule = "MDL-AGENT03"

// ValidateConsumedMCPEndpoint warns on a create consumed mcp service with no
// Endpoint.
func ValidateConsumedMCPEndpoint(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.CreateConsumedMCPServiceStmt)
		if !ok || s.CreateOrModify || s.Endpoint != nil {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   consumedMCPEndpointRule,
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf("consumed mcp service %s has no Endpoint: Agent Editor skips it at startup "+
				"(\"No value could be found for the local configuration for the constant\"), so it connects nowhere", s.Name),
			Location: linter.Location{DocumentType: "consumed mcp service", DocumentName: s.Name.String()},
			Suggestion: "Add Endpoint: @Module.UrlConstant, a String constant holding the server URL " +
				"(e.g. 'https://example.com/mcp'); set its value per environment.",
		})
	}
	return out
}
