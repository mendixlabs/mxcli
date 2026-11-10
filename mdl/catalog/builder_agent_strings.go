// SPDX-License-Identifier: Apache-2.0

package catalog

import "github.com/mendixlabs/mxcli/mdl/types"

// agentStrings is the text of an agent the strings index holds
// (ako/mxcli#2075): its description, system and user prompts, and the
// descriptions of its tools and knowledge base tools, with the title a tool
// call shows the user. Before, only the agent's documentation was indexed,
// so the most important text in an agentic app was not searchable.
//
// None of it is a Texts$Text: it lives in the document's JSON Contents, so
// the translatable walk cannot see it and it has no language. A tool row's
// ElementId is the tool's id, which agent_tools rows carry too; the agent's
// own id when the tool has none.
//
// A published MCP service's tools are described to the client's model the
// same way: each tool's description and title is a row of the service.
func agentStrings(d agentDocRefs) []stringRow {
	var out []stringRow
	add := func(value, context, elementID string) {
		if value == "" {
			return
		}
		out = append(out, stringRow{
			QualifiedName: d.source, ObjectType: d.sourceType,
			StringValue: value, StringContext: context,
			ElementID: elementID, ModuleName: d.moduleName,
		})
	}
	if svc, ok := d.doc.(*types.PublishedMCPService); ok {
		for _, t := range svc.Tools {
			id := t.UUID
			if id == "" {
				id = string(svc.ID)
			}
			add(t.Description, "mcp_tool_description", id)
			add(t.Title, "mcp_tool_title", id)
		}
		return out
	}
	agent, ok := d.doc.(*types.Agent)
	if !ok {
		return nil
	}
	agentID := string(agent.ID)
	elementOf := func(id string) string {
		if id != "" {
			return id
		}
		return agentID
	}
	add(agent.Description, "agent_description", agentID)
	add(agent.SystemPrompt, "system_prompt", agentID)
	add(agent.UserPrompt, "user_prompt", agentID)
	for _, t := range agent.Tools {
		id := elementOf(t.ID)
		add(t.Description, "tool_description", id)
		if t.DisplayTitle != nil {
			add(*t.DisplayTitle, "tool_display_title", id)
		}
		if t.DisplayDescription != nil {
			add(*t.DisplayDescription, "tool_display_description", id)
		}
	}
	for _, k := range agent.KBTools {
		add(k.Description, "knowledge_base_description", elementOf(k.ID))
	}
	return out
}
