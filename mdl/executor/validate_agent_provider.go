// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// agentProviderRule warns on an agent-editor Provider no Studio Pro document
// has been seen to hold (ako/mxcli#2023).
//
// The value is free-form in MDL and nothing downstream reads it: mxbuild holds
// no agent-editor strings, so `Provider: MxCloudGenAl` writes, builds and
// round-trips, and only Studio Pro opening the document shows the typo. It
// warns rather than refuses, because a new Agent Editor adds providers (the
// docs name Azure AI Foundry for models since Agents Kit 2.1) and a value
// mxcli has not measured yet must stay writable.
const agentProviderRule = "MDL-AGENT01"

// measuredModelProviders are the Provider values read from Studio Pro-saved
// Model documents: mx-test-projects (FactoryManagement, ICXGraphPilot,
// EvoraICX and two more, Mendix 11.10-11.16). Add a value here only from a
// saved document.
var measuredModelProviders = []string{"MxCloudGenAI"}

// knowledgeBaseProvider is the one provider Agent Editor supports for a
// knowledge base, per its documentation and every measured document.
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
			Suggestion: suggestion + " Nothing but Studio Pro reads the value, so a typo builds and " +
				"only shows when the document is opened there.",
		})
	}
	modelWhy := "is not a provider seen in a Studio Pro-saved model (" + strings.Join(measuredModelProviders, ", ") + ")"
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
