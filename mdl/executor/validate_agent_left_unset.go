// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// validateAgentDocumentsLeftUnset is the project half of MDL-AGENT02 and
// MDL-AGENT03 (ako/mxcli#2089): a create or modify that leaves an agent with
// no selected model, or a consumed MCP service with no endpoint. Both warn,
// except an agent that names a Model: that is Studio Pro's AE018, returned as
// an error so exec refuses it (ako/mxcli#2289).
//
// The static rules skip create or modify, because a key it leaves out keeps
// the stored value. But it is also how project scripts and describe create a
// document, and a new one has nothing to keep. Only the project can tell the
// two apart, so this pass, run by check --references and by exec, replays the
// script in statement order over what is stored: a drop leaves nothing to
// keep, an earlier create or alter decides what a later statement keeps.
// A plain create is the static rules' case and is not reported twice.
//
// The agent half applies only when AgentEditorCommons reads selectedModel
// (2.2 and later); every measured version reads the endpoint.
func validateAgentDocumentsLeftUnset(ctx *ExecContext, prog *ast.Program, sc *scriptContext) []error {
	if ctx == nil || prog == nil || sc == nil || !ctx.Connected() {
		return nil
	}
	var errs []error
	agentHas := map[string]bool{}
	svcHas := map[string]bool{}
	readsSelected := -1 // unknown until an agent needs it
	for i, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateAgentStmt:
			qn := s.Name.String()
			has, ok := false, false
			if s.SelectedModel != nil {
				has, ok = *s.SelectedModel != "", true
			} else if s.CreateOrModify {
				if has, ok = agentHas[qn]; !ok {
					if a := findAgentEditorAgent(ctx, s.Name.Module, s.Name.Name); a != nil {
						has = a.SelectedModel != nil && a.SelectedModel.ID != ""
					}
				}
			}
			agentHas[qn] = has
			if has || !s.CreateOrModify {
				continue
			}
			if readsSelected < 0 {
				readsSelected = 0
				if agentEditorReadsSelectedModel(ctx) {
					readsSelected = 1
				}
			}
			if readsSelected != 1 {
				continue
			}
			msg := fmt.Sprintf("%s: %s. Add SelectedModel: '<model id>'",
				agentSelectedModelRule, agentSelectedModelMissing("is left with", qn, s.Model != nil))
			if s.Model != nil {
				errs = append(errs, fmt.Errorf("statement %d: %s", i+1, msg))
			} else {
				sc.warnings = append(sc.warnings, msg)
			}
		case *ast.AlterAgentStmt:
			for k, v := range s.Sets {
				if strings.EqualFold(k, "SelectedModel") {
					agentHas[s.Name.String()] = v != ""
				}
			}
		case *ast.DropAgentStmt:
			agentHas[s.Name.String()] = false
		case *ast.CreateConsumedMCPServiceStmt:
			qn := s.Name.String()
			has, ok := s.Endpoint != nil, s.Endpoint != nil
			if !ok && s.CreateOrModify {
				if has, ok = svcHas[qn]; !ok {
					if c := findAgentEditorConsumedMCPService(ctx, s.Name.Module, s.Name.Name); c != nil {
						has = c.Endpoint != nil && c.Endpoint.QualifiedName != ""
					}
				}
			}
			svcHas[qn] = has
			if has || !s.CreateOrModify {
				continue
			}
			sc.warnings = append(sc.warnings, fmt.Sprintf(
				"%s: consumed mcp service %s is left with no Endpoint: Agent Editor skips it at startup "+
					"(\"No value could be found for the local configuration for the constant\"), so it connects nowhere. "+
					"Add Endpoint: @Module.UrlConstant", consumedMCPEndpointRule, qn))
		case *ast.AlterConsumedMCPServiceStmt:
			for k, v := range s.Changes {
				if strings.EqualFold(k, "Endpoint") {
					svcHas[s.Name.String()] = v != ""
				}
			}
		case *ast.DropConsumedMCPServiceStmt:
			svcHas[s.Name.String()] = false
		}
	}
	return errs
}

// agentEditorReadsSelectedModel reports whether the project's
// AgentEditorCommons requires an agent's selectedModel: 2.2.1 and 2.6.0
// validate it, 1.1.0 does not read it (ako/mxcli#2031). A project whose
// version is unknown (no Marketplace version recorded) counts as reading it,
// as every current release does.
func agentEditorReadsSelectedModel(ctx *ExecContext) bool {
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return true
	}
	for _, m := range modules {
		if m.Name == "AgentEditorCommons" && m.AppStoreVersion != "" {
			return compareVersion(m.AppStoreVersion, "2.2.0") >= 0
		}
	}
	return true
}
