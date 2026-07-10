// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for workflows. Workflows previously
// received no semantic validation at all (CreateWorkflowStmt has no case in
// validateWithContext), so several constructs passed `mxcli check` but were
// rejected by MxBuild, each costing a build round-trip. These heuristics catch
// the syntax-only cases up front. See
// docs/11-proposals/PROPOSAL_check_mxbuild_gap_heuristics.md.
package executor

import (
	"fmt"
	"regexp"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

// wfOutcomeIdentRe matches a valid Mendix EnumerationValueIdentifier: a bare
// identifier (no spaces or punctuation). Decision / call-microflow outcome names
// must be enum value identifiers; free text like 'Confirmed closed' is rejected
// by MxBuild.
var wfOutcomeIdentRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateWorkflow checks a workflow for constructs that pass parsing but are
// rejected by MxBuild, without requiring a project connection.
//
//   - MDL-WF01: user task without a page (CE1834)
//   - MDL-WF02: single-outcome user task containing nested activities (CE1876)
//   - MDL-WF03: decision / call-microflow outcome that is not a valid
//     enumeration value identifier
func ValidateWorkflow(stmt *ast.CreateWorkflowStmt) []linter.Violation {
	var out []linter.Violation
	loc := linter.Location{
		Module:       stmt.Name.Module,
		DocumentType: "workflow",
		DocumentName: stmt.Name.Name,
	}
	walkWorkflowActivities(stmt.Activities, func(a ast.WorkflowActivityNode) {
		switch n := a.(type) {
		case *ast.WorkflowUserTaskNode:
			label := workflowUserTaskLabel(n)
			// MDL-WF01 — user task without a page.
			if n.Page.Module == "" && n.Page.Name == "" {
				out = append(out, linter.Violation{
					RuleID:     "MDL-WF01",
					Severity:   linter.SeverityError,
					Location:   loc,
					Message:    fmt.Sprintf("user task %s has no page — MxBuild requires the Page property (CE1834)", label),
					Suggestion: "Add a page typed to System.WorkflowUserTask, e.g. `page Module.TaskPage`.",
				})
			}
			// MDL-WF02 — a single outcome must not carry a nested flow.
			if len(n.Outcomes) == 1 && len(n.Outcomes[0].Activities) > 0 {
				out = append(out, linter.Violation{
					RuleID:     "MDL-WF02",
					Severity:   linter.SeverityError,
					Location:   loc,
					Message:    fmt.Sprintf("user task %s has a single outcome with nested activities — MxBuild rejects this (CE1876)", label),
					Suggestion: "Move the activities to the workflow's main flow after the user task, or add a second outcome.",
				})
			}
		case *ast.WorkflowDecisionNode:
			out = append(out, checkWorkflowOutcomeNames(n.Outcomes, "decision", loc)...)
		case *ast.WorkflowCallMicroflowNode:
			out = append(out, checkWorkflowOutcomeNames(n.Outcomes, "call microflow", loc)...)
		}
	})
	return out
}

// checkWorkflowOutcomeNames flags condition-outcome values (decision / call
// microflow branches) that are not valid enumeration value identifiers (MDL-WF03).
func checkWorkflowOutcomeNames(outcomes []ast.WorkflowConditionOutcomeNode, kind string, loc linter.Location) []linter.Violation {
	var out []linter.Violation
	for _, o := range outcomes {
		if o.Value == "" || wfOutcomeIdentRe.MatchString(o.Value) {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:     "MDL-WF03",
			Severity:   linter.SeverityError,
			Location:   loc,
			Message:    fmt.Sprintf("%s outcome '%s' is not a valid enumeration value identifier — MxBuild rejects outcome names with spaces or punctuation", kind, o.Value),
			Suggestion: "Use a bare identifier (e.g. 'ConfirmedClosed'); a decision branches on the enumeration returned by its expression, so outcome names must match that enum's value identifiers.",
		})
	}
	return out
}

// workflowUserTaskLabel returns a human-readable label for a user task.
func workflowUserTaskLabel(n *ast.WorkflowUserTaskNode) string {
	if n.Name != "" {
		return "'" + n.Name + "'"
	}
	if n.Caption != "" {
		return "'" + n.Caption + "'"
	}
	return "(unnamed)"
}

// walkWorkflowActivities visits every activity node in a workflow, recursing
// into all nested activity flows (outcomes, parallel-split paths, boundary
// events).
func walkWorkflowActivities(acts []ast.WorkflowActivityNode, visit func(ast.WorkflowActivityNode)) {
	for _, a := range acts {
		if a == nil {
			continue
		}
		visit(a)
		switch n := a.(type) {
		case *ast.WorkflowUserTaskNode:
			for _, o := range n.Outcomes {
				walkWorkflowActivities(o.Activities, visit)
			}
			for _, b := range n.BoundaryEvents {
				walkWorkflowActivities(b.Activities, visit)
			}
		case *ast.WorkflowDecisionNode:
			for _, o := range n.Outcomes {
				walkWorkflowActivities(o.Activities, visit)
			}
		case *ast.WorkflowCallMicroflowNode:
			for _, o := range n.Outcomes {
				walkWorkflowActivities(o.Activities, visit)
			}
			for _, b := range n.BoundaryEvents {
				walkWorkflowActivities(b.Activities, visit)
			}
		case *ast.WorkflowParallelSplitNode:
			for _, p := range n.Paths {
				walkWorkflowActivities(p.Activities, visit)
			}
		case *ast.WorkflowWaitForNotificationNode:
			for _, b := range n.BoundaryEvents {
				walkWorkflowActivities(b.Activities, visit)
			}
		}
	}
}
