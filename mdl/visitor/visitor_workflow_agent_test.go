// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `call agent microflow` is the AI agent task; everything after AGENT is the
// call-microflow statement, so name, comment, mappings and outcomes land the same.
func TestWorkflowVisitor_CallAgentMicroflow(t *testing.T) {
	stmt := buildWorkflowStmt(t, `create workflow M.W
begin
  call agent microflow M.InvokeAgent as aiAgentTask1 comment 'Classify' with (Request = '$WorkflowContext')
    outcomes true -> { } false -> { };
  call microflow M.Plain;
end workflow;`)
	if len(stmt.Activities) != 2 {
		t.Fatalf("activities = %d, want 2", len(stmt.Activities))
	}
	agent, ok := stmt.Activities[0].(*ast.WorkflowCallMicroflowNode)
	if !ok {
		t.Fatalf("activity 0 is %T", stmt.Activities[0])
	}
	if !agent.Agent || agent.Name != "aiAgentTask1" || agent.Caption != "Classify" ||
		agent.Microflow.String() != "M.InvokeAgent" || len(agent.ParameterMappings) != 1 || len(agent.Outcomes) != 2 {
		t.Errorf("agent task = %+v", agent)
	}
	plain := stmt.Activities[1].(*ast.WorkflowCallMicroflowNode)
	if plain.Agent {
		t.Error("a plain call microflow must not be an agent task")
	}
}
