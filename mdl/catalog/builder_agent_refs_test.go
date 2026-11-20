// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2074: an agent-editor document produced no refs, so an agent's
// tool microflow showed "(no callers found)" and GRAPH_DEAD_ASSETS listed it
// as unused although the agent calls it in production. And a microflow passed
// as a microflow-typed Java action parameter (ActionMicroflow of
// ChatContext_Create_ForAgent, AuthenticationMicroflow of CreateMCPServer) was
// no reference either.

// resolveFixture is what the catalog knows of each name: the object type the
// tables hold it under.
func resolveFixture(qn string) string {
	return map[string]string{
		"Desk.Tool_GetOrder":    RefObjectMicroflow,
		"Desk.NF_Local":         RefObjectNanoflow,
		"Desk.Triage":           RefObjectAgent,
		"Desk.Jira":             RefObjectConsumedMCPService,
		"Desk.Docs":             RefObjectKnowledgeBase,
		"Desk.Gpt":              RefObjectAIModel,
		"Desk.GenAIKey":         RefObjectConstant,
		"Desk.Ticket":           RefObjectEntity,
		"Desk.Chat_Action":      RefObjectMicroflow,
		"Desk.SupportAgent":     RefObjectAgent,
		"Desk.MCP_Authenticate": RefObjectMicroflow,
	}[qn]
}

func edgeStrings(rs []docRef) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.kind+" "+r.targetType+" "+r.target)
	}
	sort.Strings(out)
	return out
}

func TestAgentEditorRefs_Issue2074(t *testing.T) {
	agent := &types.Agent{
		Tools: []types.AgentTool{
			{Name: "GetOrder", ToolType: "Microflow", Document: &types.DocRef{QualifiedName: "Desk.Tool_GetOrder"}},
			{Name: "Local", Document: &types.DocRef{QualifiedName: "Desk.NF_Local"}},
			{Name: "Triage", Document: &types.DocRef{QualifiedName: "Desk.Triage"}},
			{Name: "Jira", ToolType: "MCP", Document: &types.DocRef{QualifiedName: "Desk.Jira"}},
			{Name: "Unset", ToolType: "Microflow"}, // a tool with no document yet: no edge
			// a tool whose document the catalog does not hold: the edge is still
			// recorded, typed from its ToolType, as a call to a missing microflow is
			{Name: "Gone", ToolType: "Microflow", Document: &types.DocRef{QualifiedName: "Desk.Tool_Deleted"}},
		},
		KBTools: []types.AgentKBTool{{Name: "Docs", Document: &types.DocRef{QualifiedName: "Desk.Docs"}}},
		Model:   &types.DocRef{QualifiedName: "Desk.Gpt"},
		Entity:  &types.DocRef{QualifiedName: "Desk.Ticket"},
	}
	want := []string{
		"call AGENT Desk.Triage",
		"call MICROFLOW Desk.Tool_Deleted",
		"call MICROFLOW Desk.Tool_GetOrder",
		"call NANOFLOW Desk.NF_Local",
		"model AI_MODEL Desk.Gpt",
		"tool CONSUMED_MCP_SERVICE Desk.Jira",
		"tool KNOWLEDGE_BASE Desk.Docs",
		"type ENTITY Desk.Ticket",
	}
	if got := edgeStrings(agentEditorRefs(agent, resolveFixture)); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("agent edges:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	key := &types.ConstantRef{QualifiedName: "Desk.GenAIKey"}
	for _, doc := range []any{&types.Model{Key: key}, &types.KnowledgeBase{Key: key}} {
		if got := edgeStrings(agentEditorRefs(doc, resolveFixture)); strings.Join(got, "|") != "key CONSTANT Desk.GenAIKey" {
			t.Errorf("%T edges = %v, want the key constant", doc, got)
		}
	}
	// Control: a model with no key set has no edge.
	if got := agentEditorRefs(&types.Model{}, resolveFixture); len(got) != 0 {
		t.Errorf("model without a key: %v", got)
	}
}

func TestCodeActionParameterRefs_Issue2074(t *testing.T) {
	action := &microflows.JavaActionCallAction{
		JavaAction: "AgentCommons.ChatContext_Create_ForAgent",
		ParameterMappings: []*microflows.JavaActionParameterMapping{
			{Parameter: "ActionMicroflow", Value: &microflows.MicroflowParameterValue{Microflow: "Desk.Chat_Action"}},
			{Parameter: "Agent", Value: &microflows.CustomBlobDocumentCodeActionParameterValue{CustomDocument: "Desk.SupportAgent"}},
			{Parameter: "Unset", Value: &microflows.MicroflowParameterValue{}},                                    // Studio Pro's placeholder: no edge
			{Parameter: "Text", Value: &microflows.BasicCodeActionParameterValue{Argument: "'Desk.Chat_Action'"}}, // control: a string, not a reference
		},
	}
	want := "call AGENT Desk.SupportAgent|call MICROFLOW Desk.Chat_Action"
	if got := strings.Join(edgeStrings(codeActionParameterRefs(action, resolveFixture)), "|"); got != want {
		t.Errorf("java action parameter edges = %q, want %q", got, want)
	}
	// The same holds for the AuthenticationMicroflow of CreateMCPServer.
	mcp := &microflows.JavaActionCallAction{ParameterMappings: []*microflows.JavaActionParameterMapping{
		{Parameter: "AuthenticationMicroflow", Value: &microflows.MicroflowParameterValue{Microflow: "Desk.MCP_Authenticate"}},
	}}
	if got := strings.Join(edgeStrings(codeActionParameterRefs(mcp, resolveFixture)), "|"); got != "call MICROFLOW Desk.MCP_Authenticate" {
		t.Errorf("CreateMCPServer edges = %q", got)
	}
}
