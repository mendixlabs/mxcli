// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
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
create agent M.NoModel (UsageType: Task);
`)
	got := map[string]string{}
	sev := map[string]linter.Severity{}
	for _, v := range ValidateAgentSelectedModel(prog) {
		if v.RuleID != agentSelectedModelRule {
			t.Errorf("rule %s", v.RuleID)
		}
		got[v.Location.DocumentName] = v.Message + " | " + v.Suggestion
		sev[v.Location.DocumentName] = v.Severity
	}
	for _, name := range []string{"M.Missing", "M.Empty", "M.NoModel"} {
		if !strings.Contains(got[name], "SelectedModel") {
			t.Errorf("%s: %q, want a SelectedModel violation", name, got[name])
		}
	}
	// ako/mxcli#2289: an agent with a Model and no selected model is Studio
	// Pro's AE018 ("Model version is required for an agent"), so it is an
	// error that refuses exec. With no Model at all there is nothing to pick a
	// version of, and it stays a warning.
	for _, name := range []string{"M.Missing", "M.Empty"} {
		if sev[name] != linter.SeverityError {
			t.Errorf("%s: severity %v, want error (AE018)", name, sev[name])
		}
		if !strings.Contains(got[name], "AE018") {
			t.Errorf("%s: %q, want it to name AE018", name, got[name])
		}
	}
	if sev["M.NoModel"] != linter.SeverityWarning {
		t.Errorf("M.NoModel: severity %v, want warning", sev["M.NoModel"])
	}
	// Controls: a stated model, and a create or modify (which keeps the stored
	// one when the statement leaves it out), say nothing.
	for _, name := range []string{"M.Good", "M.Kept"} {
		if got[name] != "" {
			t.Errorf("%s warned: %s", name, got[name])
		}
	}
}
