// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// MDL-WF23: more boundary events on one activity than Studio Pro allows
// (ako/mxcli#2242, mendixlabs/mxcli#1856). mx check 11.12.4 reports CE6696
// "An activity is not allowed to have more than 5 boundary events." on a user
// task carrying six; check and exec let it through and wrote a model with a
// consistency error. The stored-side half — `insert boundary event` onto an
// activity that already has five — is refused by the workflow mutator, which is
// the only place that sees the stored count.
const wfBoundaryCountRule = "MDL-WF23"

func checkWorkflowBoundaryEventCounts(acts []ast.WorkflowActivityNode, inserts map[string]int, loc linter.Location) []linter.Violation {
	var out []linter.Violation
	report := func(where string, n int) {
		out = append(out, linter.Violation{
			RuleID: wfBoundaryCountRule, Severity: linter.SeverityError, Location: loc,
			Message: fmt.Sprintf("%s has %d boundary events — an activity has at most %d (mx check CE6696 \"An activity is not allowed to have more than %d boundary events.\")",
				where, n, workflows.MaxBoundaryEventsPerActivity, workflows.MaxBoundaryEventsPerActivity),
			Suggestion: fmt.Sprintf("Keep at most %d boundary events on the activity: combine timers, or use a non-interrupting timer that repeats (`repeat every … at most n times`).", workflows.MaxBoundaryEventsPerActivity),
		})
	}
	walkWorkflowActivities(acts, func(a ast.WorkflowActivityNode) {
		if n := len(workflowActivityBoundaryEvents(a)); n > workflows.MaxBoundaryEventsPerActivity {
			report(boundaryHostLabel(a), n)
		}
	})
	for ref, n := range inserts {
		if n > workflows.MaxBoundaryEventsPerActivity {
			report("the activity '"+ref+"' (by this statement's inserts alone)", n)
		}
	}
	return out
}

// workflowBoundaryCountViolations is MDL-WF23 for a CREATE WORKFLOW.
func workflowBoundaryCountViolations(stmt *ast.CreateWorkflowStmt, loc linter.Location) []linter.Violation {
	return checkWorkflowBoundaryEventCounts(workflowStatementActivities(stmt), nil, loc)
}

// alterWorkflowBoundaryCountViolations is MDL-WF23 for an ALTER WORKFLOW: the
// activities it adds, and its `insert boundary event` ops counted per activity.
func alterWorkflowBoundaryCountViolations(stmt *ast.AlterWorkflowStmt, loc linter.Location) []linter.Violation {
	inserts := map[string]int{}
	for _, op := range stmt.Operations {
		switch o := op.(type) {
		case *ast.InsertBoundaryEventOp:
			inserts[o.ActivityRef]++
		case *ast.DropBoundaryEventOp:
			if inserts[o.ActivityRef] > 0 {
				inserts[o.ActivityRef]--
			}
		}
	}
	return checkWorkflowBoundaryEventCounts(alterWorkflowAddedActivities(stmt), inserts, loc)
}
