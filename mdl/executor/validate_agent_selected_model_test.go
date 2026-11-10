// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2031: an agent created without a selected model is refused by
// ASU_AgentEditor (Agent Editor 2.2+), and one refused agent stops the import
// of every agent document. check says so before exec.
func TestValidateAgentSelectedModel(t *testing.T) {
	prog := parseMDL(t, `
create agent M.Missing (UsageType: Task, Model: M.Model);
create agent M.Empty (UsageType: Task, Model: M.Model, SelectedModel: '');
create agent M.Good (UsageType: Task, Model: M.Model, SelectedModel: 'eu.anthropic.claude-sonnet-5');
create or modify agent M.Kept (UsageType: Task, Model: M.Model);
`)
	got := map[string]string{}
	for _, v := range ValidateAgentSelectedModel(prog) {
		if v.RuleID != agentSelectedModelRule {
			t.Errorf("rule %s", v.RuleID)
		}
		got[v.Location.DocumentName] = v.Message + " | " + v.Suggestion
	}
	for _, name := range []string{"M.Missing", "M.Empty"} {
		if !strings.Contains(got[name], "SelectedModel") {
			t.Errorf("%s: %q, want a SelectedModel warning", name, got[name])
		}
	}
	// Controls: a stated model, and a create or modify (which keeps the stored
	// one when the statement leaves it out), say nothing.
	for _, name := range []string{"M.Good", "M.Kept"} {
		if got[name] != "" {
			t.Errorf("%s warned: %s", name, got[name])
		}
	}
}
