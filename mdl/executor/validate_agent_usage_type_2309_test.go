// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2309: Agent Editor reads an agent's UsageType through
// AgentEditorCommons.UsageType_StringToENUM, which accepts Task and Chat in any
// case and nothing else. `UsageType: Conversational` — what the agents skill
// taught — passed check, exec and docker check, and at startup the import
// refused the agent ("Usage Type is not valid for the Agent"); in the
// reporter's app the published MCP server did not start either. An agent with
// none is refused too (AgentFromModel_Validate: "Usage Type is missing").
func TestValidateAgentUsageType_2309(t *testing.T) {
	prog := parseMDL(t, `
create agent M.Conv (UsageType: Conversational, SystemPrompt: 'x');
create agent M.Lower (UsageType: conversational, SystemPrompt: 'x');
create agent M.Task (UsageType: Task, SystemPrompt: 'x');
create agent M.TaskLower (UsageType: task, SystemPrompt: 'x');
create agent M.Chat (UsageType: Chat, SystemPrompt: 'x');
create agent M.Unset (SystemPrompt: 'x');
alter agent M.Task set UsageType = 'SingleCall';
alter agent M.Chat set Description = 'd';
`)
	got := map[string]string{}
	for _, v := range ValidateAgentUsageTypes(prog) {
		if v.RuleID != agentUsageTypeRule {
			t.Errorf("rule %s", v.RuleID)
		}
		got[v.Location.DocumentName] += v.Message + " | " + v.Suggestion
	}
	for name, want := range map[string]string{
		"M.Conv":  `"Conversational"`,
		"M.Lower": "Chat",
		"M.Task":  `"SingleCall"`,
		"M.Unset": "has no UsageType",
	} {
		if !strings.Contains(got[name], want) {
			t.Errorf("%s: %q, want it to mention %q", name, got[name], want)
		}
	}
	for _, name := range []string{"M.TaskLower", "M.Chat"} {
		if got[name] != "" {
			t.Errorf("%s warned: %s", name, got[name])
		}
	}
}
