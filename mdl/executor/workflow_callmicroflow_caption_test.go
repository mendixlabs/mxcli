// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// A call microflow's caption was described only as a trailing `-- caption`
// comment, so describe → exec replaced an authored caption with the microflow's
// name — found by the phase 2 end-to-end round trip, where `comment 'Summarise'`
// came back as `-- ACT_Summarise`. An authored caption is now `comment '…'`, which
// re-parses; the derived default is not echoed as one.
func TestFormatCallMicroflow_CaptionRoundTrips(t *testing.T) {
	cases := []struct {
		name, caption, wantClause string
		agent                     bool
	}{
		{"authored caption", "Summarise the order", "comment 'Summarise the order'", false},
		{"authored caption on an agent task", "Classify", "comment 'Classify'", true},
		{"quote escaped", "it's done", "comment 'it''s done'", false},
		{"derived default is not a comment clause", "ACT_Summarise", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			task := &workflows.CallMicroflowTask{
				IsAgent:           c.agent,
				Microflow:         "M.ACT_Summarise",
				ParameterMappings: []*workflows.ParameterMapping{{Parameter: "M.ACT_Summarise.Ctx", Expression: "$WorkflowContext"}},
			}
			task.Name = "step1"
			task.Caption = c.caption
			out := strings.Join(formatSingleActivity(task, "  "), "\n")

			if c.wantClause == "" {
				if strings.Contains(out, " comment ") {
					t.Errorf("derived caption must not become a comment clause: %q", out)
				}
			} else if !strings.Contains(out, c.wantClause) {
				t.Errorf("describe lacks %q: %q", c.wantClause, out)
			}

			stmt := parseWorkflowStmt(t, "create workflow M.W\nbegin\n"+out+"\nend workflow;")
			cm, ok := stmt.Activities[0].(*ast.WorkflowCallMicroflowNode)
			if !ok {
				t.Fatalf("re-parsed %T from %q", stmt.Activities[0], out)
			}
			built := buildCallMicroflowTask(cm)
			if built.Caption != c.caption || built.IsAgent != c.agent || built.Name != "step1" {
				t.Errorf("re-executed caption %q agent %v name %q, want %q %v step1", built.Caption, built.IsAgent, built.Name, c.caption, c.agent)
			}
		})
	}
}
