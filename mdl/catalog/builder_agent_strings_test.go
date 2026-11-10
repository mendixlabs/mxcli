// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2075: CATALOG.STRINGS indexed only an agent's documentation. Its
// system and user prompts and the descriptions of its tools and knowledge
// bases, the most important text in an agentic app, were not searchable.
func TestAgentStrings_Issue2075(t *testing.T) {
	title := "Issuing a refund"
	empty := ""
	agent := &types.Agent{
		BaseElement:  model.BaseElement{ID: "ag-1"},
		Description:  "Handles refund requests",
		SystemPrompt: "You issue refunds. Never refund more than the order total.",
		UserPrompt:   "Refund order {{OrderId}}",
		Tools: []types.AgentTool{
			{ID: "t1", Name: "IssueRefund", Description: "Refund an order by its id",
				DisplayTitle: &title, DisplayDescription: &empty},
			{Name: "NoID", Description: "A tool without an id"},
			{ID: "t3", Name: "Silent"}, // no description: no row
		},
		KBTools: []types.AgentKBTool{
			{ID: "k1", Name: "Policies", Description: "The refund policy"},
		},
	}
	d := agentDocRefs{owner: "ag-1", sourceType: RefObjectAgent, source: "Shop.RefundAgent", moduleName: "Shop", doc: agent}

	var got []string
	for _, r := range agentStrings(d) {
		got = append(got, strings.Join([]string{r.QualifiedName, r.ObjectType, r.StringContext, r.ElementID, r.ModuleName, r.StringValue}, "|"))
	}
	want := []string{
		"Shop.RefundAgent|AGENT|agent_description|ag-1|Shop|Handles refund requests",
		"Shop.RefundAgent|AGENT|system_prompt|ag-1|Shop|You issue refunds. Never refund more than the order total.",
		"Shop.RefundAgent|AGENT|user_prompt|ag-1|Shop|Refund order {{OrderId}}",
		"Shop.RefundAgent|AGENT|tool_description|t1|Shop|Refund an order by its id",
		"Shop.RefundAgent|AGENT|tool_display_title|t1|Shop|Issuing a refund",
		"Shop.RefundAgent|AGENT|tool_description|ag-1|Shop|A tool without an id",
		"Shop.RefundAgent|AGENT|knowledge_base_description|k1|Shop|The refund policy",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("agent strings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// A published MCP service's tool descriptions are what the client's model reads.
	svc := &types.PublishedMCPService{
		BaseElement: model.BaseElement{ID: "pub-1"},
		Tools: []types.PublishedMCPTool{
			{UUID: "u1", Name: "issue_refund", Description: "Refund an order.", Title: "Issue refund"},
			{Name: "bare"}, // no text: no row
		},
	}
	got = nil
	for _, r := range agentStrings(agentDocRefs{sourceType: RefObjectPublishedMCPService, source: "Shop.RefundMCP", moduleName: "Shop", doc: svc}) {
		got = append(got, strings.Join([]string{r.QualifiedName, r.ObjectType, r.StringContext, r.ElementID, r.StringValue}, "|"))
	}
	if want := "Shop.RefundMCP|PUBLISHED_MCP_SERVICE|mcp_tool_description|u1|Refund an order.\nShop.RefundMCP|PUBLISHED_MCP_SERVICE|mcp_tool_title|u1|Issue refund"; strings.Join(got, "\n") != want {
		t.Errorf("published MCP service strings:\n%s", strings.Join(got, "\n"))
	}

	// Control: a document that is neither has no rows.
	if rows := agentStrings(agentDocRefs{doc: &types.Model{}}); len(rows) != 0 {
		t.Errorf("model rows: %v", rows)
	}
}
