// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2023: a Provider no Studio Pro document holds warns, at check
// time and without a project; a measured one does not.
func TestValidateAgentProviders(t *testing.T) {
	prog := parseMDL(t, `
create ai model M.Typo (Provider: TotallyMadeUp, Key: @M.K);
create ai model M.Case (Provider: mxcloudgenai, Key: @M.K);
create ai model M.Good (Provider: MxCloudGenAI, Key: @M.K);
create ai model M.Default (Key: @M.K);
create knowledge base M.KB (Provider: AzureAIFoundry, Key: @M.K);
create knowledge base M.GoodKB (Provider: MxCloudGenAI, Key: @M.K);
alter ai model M.Good set Provider = 'Other';
alter knowledge base M.GoodKB set ModelName = 'x';
`)
	got := map[string]string{}
	for _, v := range ValidateAgentProviders(prog) {
		if v.RuleID != agentProviderRule {
			t.Errorf("rule %s", v.RuleID)
		}
		got[v.Location.DocumentName] += v.Message + " | " + v.Suggestion
	}
	for name, want := range map[string]string{
		"M.Typo": "TotallyMadeUp",
		"M.Case": "case-sensitive",
		"M.KB":   "not supported for a knowledge base",
		"M.Good": `"Other"`,
	} {
		if !strings.Contains(got[name], want) {
			t.Errorf("%s: %q, want it to mention %q", name, got[name], want)
		}
	}
	// Controls: the measured value, an unstated one (the default is
	// MxCloudGenAI) and an alter that leaves the provider alone say nothing.
	for _, name := range []string{"M.Default", "M.GoodKB"} {
		if got[name] != "" {
			t.Errorf("%s warned: %s", name, got[name])
		}
	}
	if len(got) != 4 {
		t.Errorf("warned on %d documents, want 4: %v", len(got), got)
	}
}
