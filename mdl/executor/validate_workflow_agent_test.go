// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Rows of the mxbuild spike (11.13.0): the one shape a call microflow accepts and
// an AI agent task does not is a microflow with no parameters.
func TestWorkflowAgentTask_MicroflowNeedsAParameter(t *testing.T) {
	wf := func(body string) string {
		return `create workflow M.W parameter $C: M.Ctx begin ` + body + ` end workflow;`
	}
	cases := []struct{ name, src, want string }{
		{"agent microflow with parameters", wf(`call agent microflow M.OC_Good as ag with (pa = '$WorkflowTask', pb = '$WorkflowContext');`), ""},
		{"agent microflow without parameters", wf(`call agent microflow M.OC_None as ag;`), "CE1590"},
		// The control: the identical call as a plain call microflow builds.
		{"plain call microflow without parameters", wf(`call microflow M.OC_None as cm;`), ""},
		{"nested in an outcome", wf(`call agent microflow M.OC_Good as ag with (pa = '$WorkflowTask', pb = '$WorkflowContext') outcomes true -> { call agent microflow M.OC_None as nestedAgent; } false -> { };`), "CE1590"},
		{"script microflow without parameters", `create microflow M.ScriptAgent () begin end;
` + wf(`call agent microflow M.ScriptAgent as ag;`), "CE1590"},
		{"alter insert", `alter workflow M.Stored insert after T call agent microflow M.OC_None as ag;`, "CE1590"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := checkScript(t, wfHandlerCtx(t, 11, 13, 0), c.src)
			if c.want == "" {
				if strings.Contains(got, "CE1590") {
					t.Errorf("expected no CE1590, got:\n%s", got)
				}
				return
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("expected %s, got:\n%s", c.want, got)
			}
		})
	}
}

// The AI agent task arrived with Mendix 11.9; the registry entry is what exec's
// version gate reads.
func TestWorkflowAgentTask_VersionGate(t *testing.T) {
	if err := checkFeature(versionCtx(t, 11, 8, 0), "workflows", "ai_agent_task", "call agent microflow", "hint"); err == nil {
		t.Error("11.8 must refuse an AI agent task")
	}
	if err := checkFeature(versionCtx(t, 11, 9, 0), "workflows", "ai_agent_task", "call agent microflow", "hint"); err != nil {
		t.Errorf("11.9 must accept an AI agent task, got %v", err)
	}
	nested := parseWorkflowStmt(t, `create workflow M.W begin
  call microflow M.A outcomes true -> { call agent microflow M.B; } false -> { };
end workflow;`)
	if !workflowUsesAgentTask(nested.Activities) {
		t.Error("an agent task nested in an outcome must trip the version gate")
	}
	if workflowUsesAgentTask(parseWorkflowStmt(t, `create workflow M.W begin call microflow M.A; end workflow;`).Activities) {
		t.Error("a plain call microflow must not trip the version gate")
	}
}

// Describe output re-parses as an agent task — describe used to print one only as
// a `-- [Workflows$AIAgentTaskActivity]` comment, which a rewrite then deleted.
func TestWorkflowAgentTask_DescribeRoundTrips(t *testing.T) {
	task := &workflows.CallMicroflowTask{
		IsAgent:           true,
		Microflow:         "workflow.InvokeAgent",
		ParameterMappings: []*workflows.ParameterMapping{{Parameter: "workflow.InvokeAgent.workfow1context", Expression: "$WorkflowContext"}},
	}
	task.Name = "aiAgentTask1"
	task.Caption = "AI Agent Task"
	// Through the flow formatter, which places the statement terminator ahead of
	// the trailing caption comment.
	out := strings.Join(formatSingleActivity(task, "  "), "\n")
	// The caption is authored (not the microflow's name), so it is described as a
	// comment clause, which re-executes into the caption.
	if !strings.Contains(out, "call agent microflow workflow.InvokeAgent as aiAgentTask1 comment 'AI Agent Task' with (workfow1context = '$WorkflowContext')") {
		t.Fatalf("describe = %q", out)
	}
	stmt := parseWorkflowStmt(t, "create workflow M.W\nbegin\n"+out+"\nend workflow;")
	cm, ok := stmt.Activities[0].(*ast.WorkflowCallMicroflowNode)
	if !ok || !cm.Agent || cm.Name != "aiAgentTask1" {
		t.Errorf("re-parsed = %+v", stmt.Activities[0])
	}
}

// A statement restating the stored agent task loses nothing and must pass the
// rewrite guard; the unrestated case is refused (TestWorkflowRewrite_RefusesDroppingStudioProOnlyState).
func TestWorkflowRewrite_AllowsRestatedAgentTask(t *testing.T) {
	stmt := parseWorkflowStmt(t, `create or modify workflow M.W parameter $C: M.Ctx
begin
  call agent microflow workflow.InvokeAgent as aiAgentTask1 with (workfow1context = '$WorkflowContext');
  user task userTask1 'User Task' page M.P outcomes 'Good' { } 'Bad' { };
  multi user task userTask2 'Multi' page M.P outcomes 'Fast' { } 'Furious' { };
end workflow;`)
	raw := studioProWorkflow(false, false, true, "ConsensusCompletionCriteria", true)
	if err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, raw), "wf1", "M.W", stmt); err != nil {
		t.Fatalf("a rewrite restating its agent task must be allowed, got %v", err)
	}
}
