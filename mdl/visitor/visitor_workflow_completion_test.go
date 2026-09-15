// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// Every `participants`, `decide by` and `await all users` form lands on its
// fields, and the sub-rules keep their numbers and names out of the task's own
// positional reads (page, targeting, outcomes stay where they were).
func TestWorkflowVisitor_MultiUserTaskCompletion(t *testing.T) {
	stmt := buildWorkflowStmt(t, `create workflow M.W
begin
  multi user task consensusTask 'c' page M.P decide by consensus fallback 'Approve' outcomes 'Approve' { } 'Reject' { };
  multi user task majorityHalf 'c' page M.P participants all decide by majority more than half fallback 'Approve' outcomes 'Approve' { } 'Reject' { };
  multi user task majorityMost 'c' page M.P decide by majority most chosen outcomes 'Approve' { } 'Reject' { };
  multi user task thresholdPct 'c' page M.P targeting microflow M.Targets participants 80 percent decide by threshold 60 percent fallback 'Reject' outcomes 'Approve' { } 'Reject' { };
  multi user task thresholdVotes 'c' page M.P participants 3 decide by threshold 2 votes fallback 'Reject' await all users outcomes 'Approve' { } 'Reject' { };
  multi user task vetoTask 'c' page M.P decide by veto 'Reject' await all users outcomes 'Approve' { } 'Reject' { };
  multi user task microflowTask 'c' page M.P decide by microflow M.Decide outcomes 'Approve' { } 'Reject' { };
  multi user task plain 'c' page M.P outcomes 'Approve' { } 'Reject' { };
end workflow;`)
	tasks := map[string]*ast.WorkflowUserTaskNode{}
	for _, a := range stmt.Activities {
		n := a.(*ast.WorkflowUserTaskNode)
		tasks[n.Name] = n
		if n.Page.String() != "M.P" || len(n.Outcomes) != 2 {
			t.Errorf("%s: page %s, %d outcomes — the new clauses disturbed the positional reads", n.Name, n.Page, len(n.Outcomes))
		}
	}

	check := func(name string, want ast.WorkflowCompletionRuleNode) {
		t.Helper()
		got := tasks[name].Completion
		if got == nil || *got != want {
			t.Errorf("%s completion = %+v, want %+v", name, got, want)
		}
	}
	check("consensusTask", ast.WorkflowCompletionRuleNode{Rule: "consensus", Fallback: "Approve", HasFallback: true})
	check("majorityHalf", ast.WorkflowCompletionRuleNode{Rule: "majority", Majority: "more than half", Fallback: "Approve", HasFallback: true})
	check("majorityMost", ast.WorkflowCompletionRuleNode{Rule: "majority", Majority: "most chosen"})
	check("thresholdPct", ast.WorkflowCompletionRuleNode{Rule: "threshold", Threshold: 60, ThresholdUnit: "percent", Fallback: "Reject", HasFallback: true})
	check("thresholdVotes", ast.WorkflowCompletionRuleNode{Rule: "threshold", Threshold: 2, ThresholdUnit: "votes", Fallback: "Reject", HasFallback: true})
	check("vetoTask", ast.WorkflowCompletionRuleNode{Rule: "veto", Veto: "Reject"})
	check("microflowTask", ast.WorkflowCompletionRuleNode{Rule: "microflow", Microflow: ast.QualifiedName{Module: "M", Name: "Decide"}})

	participants := map[string]*ast.WorkflowParticipantsNode{
		"majorityHalf":   {Kind: "all"},
		"thresholdPct":   {Kind: "percent", Value: 80},
		"thresholdVotes": {Kind: "number", Value: 3},
	}
	for name, n := range tasks {
		want := participants[name]
		got := n.Participants
		if (want == nil) != (got == nil) || (want != nil && *want != *got) {
			t.Errorf("%s participants = %+v, want %+v", name, got, want)
		}
		wantAwait := name == "thresholdVotes" || name == "vetoTask"
		if n.AwaitAllUsers != wantAwait {
			t.Errorf("%s await = %v, want %v", name, n.AwaitAllUsers, wantAwait)
		}
	}
	if tasks["plain"].Completion != nil {
		t.Error("a task without `decide by` must have no completion rule")
	}
	if tasks["thresholdPct"].Targeting.Microflow.String() != "M.Targets" {
		t.Errorf("targeting microflow = %s", tasks["thresholdPct"].Targeting.Microflow)
	}
}
