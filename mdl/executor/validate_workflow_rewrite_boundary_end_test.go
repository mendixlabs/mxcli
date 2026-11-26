// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// An interrupting boundary event's path always ends the workflow: the writer
// appends the End, and Studio Pro normalizes an interrupting path to one. MDL
// does not state it, so it is implicit, like an event sub-process's closing
// End. The rewrite guard counted it as an `end workflow` the statement had
// dropped, so `create or modify` refused every workflow with an interrupting
// timer that its own describe output restated exactly — on 11.16 as on 11.17.
func boundaryEndWorkflow(branchEnd bool) map[string]any {
	path := []any{3,
		map[string]any{"$Type": "Workflows$CallMicroflowActivity", "Name": "escalate", "Microflow": "M.Escalate"},
		map[string]any{"$Type": "Workflows$EndWorkflowActivity", "Name": "end2"},
	}
	outcomeFlow := []any{3}
	if branchEnd {
		outcomeFlow = append(outcomeFlow, map[string]any{"$Type": "Workflows$EndWorkflowActivity", "Name": "end3"})
	}
	return map[string]any{
		"$Type": "Workflows$Workflow",
		"Flow": map[string]any{"$Type": "Workflows$Flow", "Activities": []any{3,
			map[string]any{"$Type": "Workflows$StartWorkflowActivity", "Name": "start1"},
			map[string]any{
				"$Type": "Workflows$SingleUserTaskActivity", "Name": "Review",
				"Outcomes": []any{3, map[string]any{"$Type": "Workflows$UserTaskOutcome", "Value": "Done",
					"Flow": map[string]any{"$Type": "Workflows$Flow", "Activities": outcomeFlow}}},
				"BoundaryEvents": []any{2, map[string]any{
					"$Type": "Workflows$InterruptingTimerBoundaryEvent", "Caption": "Escalate",
					"FirstExecutionTime": "addDays([%CurrentDateTime%], 3)",
					"Flow":               map[string]any{"$Type": "Workflows$Flow", "Activities": path},
				}},
			},
			map[string]any{"$Type": "Workflows$EndWorkflowActivity", "Name": "end1"},
		}},
	}
}

func TestWorkflowRewrite_InterruptingBoundaryEndIsImplicit(t *testing.T) {
	stmt := parseWorkflowStmt(t, `create or modify workflow M.W parameter $C: M.Ctx
begin
  user task Review 'Review' page M.P outcomes 'Done' { }
    boundary event interrupting timer addDays([%CurrentDateTime%], 3) caption 'Escalate' {
      call microflow M.Escalate;
    };
end workflow;`)
	if err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, boundaryEndWorkflow(false)), "wf1", "M.W", stmt); err != nil {
		t.Fatalf("a statement restating the workflow exactly was refused: %v", err)
	}
	// The control: an End a branch really states is still guarded.
	err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, boundaryEndWorkflow(true)), "wf1", "M.W", stmt)
	if err == nil || !strings.Contains(err.Error(), "1 stored `end workflow`") {
		t.Fatalf("a dropped branch `end workflow` must still be refused, got %v", err)
	}
}
