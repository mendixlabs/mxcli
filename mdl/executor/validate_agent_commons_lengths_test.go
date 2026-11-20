// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"
)

// ako/mxcli#2211: Agent Editor Commons copies an agent into Agent Commons
// entities with fixed lengths at startup. A longer value fails the import and
// the previous version of the agent stays in use; check, exec and mx check are
// all clean. check says so before exec.
func TestValidateAgentCommonsLengths(t *testing.T) {
	desc := func(n int, r string) string { return strings.Repeat(r, n) }
	name := func(n int) string { return "A" + strings.Repeat("x", n-1) }
	prog := parseMDL(t, fmt.Sprintf(`
create agent M.DescLong (UsageType: Task, Model: M.Model, SelectedModel: 'm', Description: '%s');
create agent M.DescFits (UsageType: Task, Model: M.Model, SelectedModel: 'm', Description: '%s');
create or modify agent M.DescLongCoM (UsageType: Task, Model: M.Model, Description: '%s');
create agent M.DescUnicodeFits (UsageType: Task, Model: M.Model, SelectedModel: 'm', Description: '%s');
create agent M.DescUnicodeLong (UsageType: Task, Model: M.Model, SelectedModel: 'm', Description: '%s');
create agent M.%s (UsageType: Task, Model: M.Model, SelectedModel: 'm');
create agent M.%s (UsageType: Task, Model: M.Model, SelectedModel: 'm');
create agent M.ToolLong (UsageType: Task, Model: M.Model, SelectedModel: 'm')
{
  TOOL Fits ( Document: M.Flow, Description: '%s', Enabled: true, DisplayTitle: '%s' )
  TOOL Long ( Document: M.Flow, Description: '%s', Enabled: true, DisplayTitle: '%s' )
};
create agent M.ToolNameLong (UsageType: Task, Model: M.Model, SelectedModel: 'm')
{
  TOOL %s ( Document: M.Flow, Description: 'd', Enabled: true )
};
`,
		desc(201, "a"), desc(200, "a"), desc(201, "a"),
		desc(200, "é"), desc(201, "é"),
		name(51), name(50),
		desc(4096, "d"), desc(200, "t"), desc(4097, "d"), desc(201, "t"),
		name(201),
	))
	got := map[string][]string{}
	for _, v := range ValidateAgentCommonsLengths(prog) {
		if v.RuleID != agentCommonsLengthRule {
			t.Errorf("rule %s", v.RuleID)
		}
		got[v.Location.DocumentName] = append(got[v.Location.DocumentName], v.Message)
	}
	want := func(doc, attr string) {
		t.Helper()
		for _, m := range got[doc] {
			if strings.Contains(m, attr) && strings.Contains(m, "previous") {
				return
			}
		}
		t.Errorf("%s: %q, want a finding naming %s", doc, got[doc], attr)
	}
	none := func(doc string) {
		t.Helper()
		if len(got[doc]) != 0 {
			t.Errorf("%s: unexpected %q", doc, got[doc])
		}
	}
	want("M.DescLong", "AgentCommons.Agent.Description (200)")
	want("M.DescLongCoM", "AgentCommons.Agent.Description (200)")
	// 201 two-byte characters: counted as characters, not 402 bytes.
	want("M.DescUnicodeLong", "AgentCommons.Agent.Description (200)")
	want("M."+name(51), "AgentCommons.Agent.Title (50)")
	want("M.ToolLong", "AgentCommons.Tool.DisplayTitle (200)")
	want("M.ToolLong", "AgentCommons.Tool.Description (4096)")
	want("M.ToolNameLong", "AgentCommons.Tool.Name (200)")
	if n := len(got["M.ToolLong"]); n != 2 {
		t.Errorf("M.ToolLong: %d findings, want 2 (only the Long tool): %q", n, got["M.ToolLong"])
	}
	// Controls: at the limit, nothing.
	none("M.DescFits")
	none("M.DescUnicodeFits")
	none("M." + name(50))
}
