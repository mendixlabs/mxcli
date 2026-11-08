// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// agentProviderRule warns on an agent-editor Provider that Agent Editor's
// runtime import does not accept (ako/mxcli#2023).
//
// The value is free-form in MDL and mxbuild holds no agent-editor strings, so
// `Provider: MxCloudGenAl` writes, builds and round-trips. The first thing to
// read it is ASU_AgentEditor at startup, whose import fails with "Model
// provider is not one of the allowed types". It warns rather than refuses,
// because a new Agent Editor adds providers and a value mxcli has not read yet
// must stay writable.
const agentProviderRule = "MDL-AGENT01"

// measuredModelProviders are the values AgentEditorCommons'
// ImportDeployedModel.getImportMicroflow switches on: MxCloudGenAI and Azure
// (Azure AI Foundry, through OpenAIConnector) in Agent Editor 2.2.1 and the
// version ICXGraphPilot ships; Fuse added by 2.7.0. Any other value fails the
// import. Add a value here only from that switch or a saved document.
var measuredModelProviders = []string{"MxCloudGenAI", "Azure", "Fuse"}

// knowledgeBaseProvider is the one provider ImportConsumedKnowledgeBase
// accepts, in every Agent Editor version measured.
const knowledgeBaseProvider = "MxCloudGenAI"

// ValidateAgentProviders warns on a Model or Knowledge Base Provider outside
// the measured values, on create and on alter … set Provider.
func ValidateAgentProviders(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	warn := func(kind string, name ast.QualifiedName, provider string, known []string, why string) {
		for _, k := range known {
			if provider == k {
				return
			}
		}
		suggestion := "Use " + strings.Join(known, " or ") + "."
		for _, k := range known {
			if strings.EqualFold(provider, k) {
				suggestion = fmt.Sprintf("Write it as %s: the value is case-sensitive.", k)
			}
		}
		out = append(out, linter.Violation{
			RuleID:   agentProviderRule,
			Severity: linter.SeverityWarning,
			Message:  fmt.Sprintf("%s %s: Provider %q %s", kind, name, provider, why),
			Location: linter.Location{DocumentType: kind, DocumentName: name.String()},
			Suggestion: suggestion + " mxbuild does not read the value, so a typo builds and " +
				"only fails when ASU_AgentEditor imports the document at startup.",
		})
	}
	modelWhy := "is not a provider Agent Editor imports (" + strings.Join(measuredModelProviders, ", ") + ")"
	kbWhy := "is not supported for a knowledge base: Agent Editor supports only " + knowledgeBaseProvider
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateModelStmt:
			if s.Provider != "" {
				warn("ai model", s.Name, s.Provider, measuredModelProviders, modelWhy)
			}
		case *ast.AlterModelStmt:
			if p, ok := changedProvider(s.Changes); ok {
				warn("ai model", s.Name, p, measuredModelProviders, modelWhy)
			}
		case *ast.CreateKnowledgeBaseStmt:
			if s.Provider != "" {
				warn("knowledge base", s.Name, s.Provider, []string{knowledgeBaseProvider}, kbWhy)
			}
		case *ast.AlterKnowledgeBaseStmt:
			if p, ok := changedProvider(s.Changes); ok {
				warn("knowledge base", s.Name, p, []string{knowledgeBaseProvider}, kbWhy)
			}
		}
	}
	return out
}

func changedProvider(changes map[string]string) (string, bool) {
	for k, v := range changes {
		if strings.EqualFold(k, "provider") {
			return v, true
		}
	}
	return "", false
}
