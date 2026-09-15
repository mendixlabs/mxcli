// SPDX-License-Identifier: Apache-2.0

// Ending a workflow from inside a branch — `end workflow` — and `return`, the
// spelling a microflow author reaches for. See
// docs/11-proposals/PROPOSAL_workflow_end_activity.md.
//
// Every rule here is a placement measured against mxbuild 11.13.0 with the real
// syntax, one workflow per shape, verdict = the literal `mx check` line:
//
//	End closing an outcome / decision branch / call-microflow outcome   0 errors
//	End closing an interrupting boundary path, at any depth              0 errors
//	End under a parallel split, at any depth                             CE1844  MDL-WF08
//	End under a non-interrupting boundary path, at any depth             CE1844  MDL-WF08
//	an activity after End in the same block                              CE6671  MDL-WF09
//	every path of an activity ends (End or jump, also via a nested
//	  decision), then another activity — or the main flow's own End      CE6689  MDL-WF10
//
// A single-outcome user task whose outcome holds only `end workflow` is CE1876
// and CE6689 at once; it is reported once, as MDL-WF02.
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// endBody is one block of activities and where it sits. The End rules are about
// blocks — what is last in one, what is under a split — which the per-activity
// walkWorkflowActivities cannot answer.
type endBody struct {
	activities           []ast.WorkflowActivityNode
	mainFlow             bool // the top-level body, followed by the implicit End
	underSplit           bool // inside a parallel-split path, at any depth
	underNonInterrupting bool // inside a non-interrupting boundary-event path, at any depth
}

// collectEndBodies lists b and every block nested in it, carrying the context
// down: once under a split or a non-interrupting boundary, always under one.
func collectEndBodies(b endBody, out *[]endBody) {
	*out = append(*out, b)
	child := func(acts []ast.WorkflowActivityNode, split, nonInterrupting bool) {
		collectEndBodies(endBody{
			activities:           acts,
			underSplit:           b.underSplit || split,
			underNonInterrupting: b.underNonInterrupting || nonInterrupting,
		}, out)
	}
	boundary := func(events []ast.WorkflowBoundaryEventNode) {
		for _, e := range events {
			child(e.Activities, false, nonInterruptingEventType(e.EventType))
		}
	}
	for _, a := range b.activities {
		switch n := a.(type) {
		case *ast.WorkflowUserTaskNode:
			for _, o := range n.Outcomes {
				child(o.Activities, false, false)
			}
			boundary(n.BoundaryEvents)
		case *ast.WorkflowDecisionNode:
			for _, o := range n.Outcomes {
				child(o.Activities, false, false)
			}
		case *ast.WorkflowCallMicroflowNode:
			for _, o := range n.Outcomes {
				child(o.Activities, false, false)
			}
			boundary(n.BoundaryEvents)
		case *ast.WorkflowParallelSplitNode:
			for _, p := range n.Paths {
				child(p.Activities, true, false)
			}
		case *ast.WorkflowWaitForNotificationNode:
			boundary(n.BoundaryEvents)
		}
	}
}

// nonInterruptingEventType reports whether a boundary event of this kind runs
// alongside its activity (timer or notification).
func nonInterruptingEventType(eventType string) bool {
	return (&workflows.BoundaryEvent{EventType: eventType}).IsNonInterrupting()
}

// flowEnds reports whether control can never leave the end of a block.
func flowEnds(acts []ast.WorkflowActivityNode) bool {
	return len(acts) > 0 && activityEnds(acts[len(acts)-1])
}

// activityEnds reports whether no path continues past an activity: it is an End
// or a jump, or it branches and every branch ends. Measured, a jump ends a path
// as surely as an End, and termination reaches through a nested decision.
// Boundary events do not count — the task's own outcomes still continue.
func activityEnds(a ast.WorkflowActivityNode) bool {
	switch n := a.(type) {
	case *ast.WorkflowEndNode, *ast.WorkflowJumpToNode:
		return true
	case *ast.WorkflowUserTaskNode:
		// A single outcome may not hold activities at all (CE1876, MDL-WF02);
		// counting it here reported one mistake twice.
		if len(n.Outcomes) < 2 {
			return false
		}
		for _, o := range n.Outcomes {
			if !flowEnds(o.Activities) {
				return false
			}
		}
		return true
	case *ast.WorkflowDecisionNode:
		return conditionOutcomesEnd(n.Outcomes)
	case *ast.WorkflowCallMicroflowNode:
		return conditionOutcomesEnd(n.Outcomes)
	}
	return false
}

func conditionOutcomesEnd(outcomes []ast.WorkflowConditionOutcomeNode) bool {
	if len(outcomes) == 0 {
		return false
	}
	for _, o := range outcomes {
		if !flowEnds(o.Activities) {
			return false
		}
	}
	return true
}

// branchingLabel names an activity that has paths, "" for one that does not.
func branchingLabel(a ast.WorkflowActivityNode) string {
	switch n := a.(type) {
	case *ast.WorkflowUserTaskNode:
		return "user task " + workflowUserTaskLabel(n)
	case *ast.WorkflowDecisionNode:
		return "decision " + workflowDecisionLabel(n)
	case *ast.WorkflowCallMicroflowNode:
		return "call microflow " + workflowCallMicroflowLabel(n)
	}
	return ""
}

// ValidateWorkflowEnds applies MDL-WF08..11 to a CREATE WORKFLOW body.
func ValidateWorkflowEnds(stmt *ast.CreateWorkflowStmt) []linter.Violation {
	var bodies []endBody
	collectEndBodies(endBody{activities: stmt.Activities, mainFlow: true}, &bodies)
	// An event sub-process body is its own flow: `end workflow` may close it,
	// and one that already ends takes no implicit End (the builder adds none).
	for _, esp := range stmt.EventSubProcesses {
		collectEndBodies(endBody{activities: esp.Activities}, &bodies)
	}
	return checkEndBodies(bodies, workflowLocation(stmt.Name))
}

// ValidateAlterWorkflowEnds applies the same rules to what ALTER WORKFLOW
// inserts, with the context the statement itself decides: a path body is under
// a split, a non-interrupting boundary body is what it says. What only the
// stored workflow knows — the target already sitting under one — is
// validateAlterWorkflowEndAncestry's.
func ValidateAlterWorkflowEnds(stmt *ast.AlterWorkflowStmt) []linter.Violation {
	var bodies []endBody
	for _, op := range stmt.Operations {
		switch o := op.(type) {
		case *ast.InsertAfterOp:
			collectEndBodies(endBody{activities: []ast.WorkflowActivityNode{o.NewActivity}}, &bodies)
		case *ast.ReplaceActivityOp:
			collectEndBodies(endBody{activities: []ast.WorkflowActivityNode{o.NewActivity}}, &bodies)
		case *ast.InsertOutcomeOp:
			collectEndBodies(endBody{activities: o.Activities}, &bodies)
		case *ast.InsertBranchOp:
			collectEndBodies(endBody{activities: o.Activities}, &bodies)
		case *ast.InsertPathOp:
			collectEndBodies(endBody{activities: o.Activities, underSplit: true}, &bodies)
		case *ast.InsertBoundaryEventOp:
			collectEndBodies(endBody{activities: o.Activities, underNonInterrupting: nonInterruptingEventType(o.EventType)}, &bodies)
		}
	}
	return checkEndBodies(bodies, workflowLocation(stmt.Name))
}

func workflowLocation(name ast.QualifiedName) linter.Location {
	return linter.Location{Module: name.Module, DocumentType: "workflow", DocumentName: name.Name}
}

func checkEndBodies(bodies []endBody, loc linter.Location) []linter.Violation {
	var out []linter.Violation
	add := func(rule, msg, suggestion string) {
		out = append(out, linter.Violation{
			RuleID: rule, Severity: linter.SeverityError, Location: loc, Message: msg, Suggestion: suggestion,
		})
	}
	for _, b := range bodies {
		for i, a := range b.activities {
			last := i == len(b.activities)-1
			switch n := a.(type) {
			case *ast.WorkflowReturnNode:
				suggestion := "Write `end workflow;`. Like a microflow's `return` it ends the whole workflow, not the block it is written in."
				if b.mainFlow {
					suggestion = "Remove it: the main flow already ends at the body's own `end workflow`."
				}
				add("MDL-WF11", "`return` is how a microflow ends — a workflow ends with `end workflow;`", suggestion)

			case *ast.WorkflowEndNode:
				switch {
				case b.underSplit:
					add("MDL-WF08",
						"`end workflow` inside a parallel split path — one path cannot end the workflow while the other paths are still running; the build fails CE1844",
						"End the workflow after the split instead, deciding on what the paths recorded. A jump out of a path is refused too (CE6682).")
				case b.underNonInterrupting:
					add("MDL-WF08",
						"`end workflow` inside a non-interrupting boundary event path — that path runs alongside the task instead of replacing it, so it cannot end the workflow; the build fails CE1844",
						"Make the boundary event `interrupting` if the timeout should end the workflow, or end it from one of the task's outcomes.")
				}
				if !last {
					add("MDL-WF09",
						"an activity after `end workflow` in the same block can never run — the build fails CE6671 (\"an end activity can only be placed at the end of a path\")",
						"Put `end workflow;` last in its block, and move what came after it before it.")
				}

			default:
				label := branchingLabel(a)
				if label == "" || !activityEnds(n) {
					continue
				}
				switch {
				case !last:
					add("MDL-WF10",
						fmt.Sprintf("every path of %s ends the workflow or jumps away, so the activities after it can never run — the build fails CE6689", label),
						"Let at least one path continue (a block without `end workflow` or `jump to` carries on after the activity), or remove what follows.")
				case b.mainFlow:
					add("MDL-WF10",
						fmt.Sprintf("every path of %s ends the workflow or jumps away, so the workflow's own End after it can never be reached — the build fails CE6689", label),
						"Let at least one path continue: a path that reaches the end of the workflow needs no `end workflow`.")
				}
			}
		}
		if b.mainFlow && len(b.activities) > 0 {
			if j, ok := b.activities[len(b.activities)-1].(*ast.WorkflowJumpToNode); ok {
				add("MDL-WF10",
					fmt.Sprintf("the main flow ends in `jump to %s` — a jump must end a path, and the workflow's own End follows it and can never be reached; the build fails CE6679 and CE6689", j.Target),
					"Put the jump inside an outcome, so that another path reaches the end of the workflow.")
			}
		}
	}
	return out
}

// validateAlterWorkflowEndAncestry refuses an ALTER that puts `end workflow`
// somewhere the statement cannot show is illegal: its target activity already
// sits under a parallel split or a non-interrupting boundary event in the stored
// workflow (CE1844). What the statement does show — an inserted path, an
// inserted non-interrupting boundary — ValidateAlterWorkflowEnds reports.
func validateAlterWorkflowEndAncestry(ctx *ExecContext, s *ast.AlterWorkflowStmt) []string {
	wf := findStoredWorkflow(ctx, s.Name)
	if wf == nil || wf.Flow == nil {
		return nil
	}
	var errs []string
	check := func(op, ref string, atPos int, acts []ast.WorkflowActivityNode) {
		if countAuthoredEnds(acts) == 0 {
			return
		}
		place, ok := resolveStoredPlacement(wf.Flow, ref, atPos)
		if !ok {
			return
		}
		switch {
		case place.underSplit:
			errs = append(errs, fmt.Sprintf(
				"%s on '%s' puts `end workflow` inside a parallel split path — '%s' is in one in the stored workflow, "+
					"and one path cannot end the workflow while the others run; the build fails CE1844 [MDL-WF08]", op, ref, ref))
		case place.underNonInterrupting:
			errs = append(errs, fmt.Sprintf(
				"%s on '%s' puts `end workflow` inside a non-interrupting boundary event path — '%s' is in one in the stored "+
					"workflow, and that path runs alongside its task rather than ending it; the build fails CE1844 [MDL-WF08]", op, ref, ref))
		}
	}
	for _, op := range s.Operations {
		switch o := op.(type) {
		case *ast.InsertAfterOp:
			check("insert after", o.ActivityRef, o.AtPosition, []ast.WorkflowActivityNode{o.NewActivity})
		case *ast.ReplaceActivityOp:
			check("replace activity", o.ActivityRef, o.AtPosition, []ast.WorkflowActivityNode{o.NewActivity})
		case *ast.InsertOutcomeOp:
			check("insert outcome", o.ActivityRef, o.AtPosition, o.Activities)
		case *ast.InsertBranchOp:
			check("insert condition", o.ActivityRef, o.AtPosition, o.Activities)
		case *ast.InsertBoundaryEventOp:
			if !nonInterruptingEventType(o.EventType) { // that one is reported without a project
				check("insert boundary event", o.ActivityRef, o.AtPosition, o.Activities)
			}
		}
	}
	return errs
}

type storedPlacement struct {
	underSplit           bool
	underNonInterrupting bool
}

// resolveStoredPlacement finds an activity the way resolveStoredActivity does —
// by Name or Caption, in the same traversal order, @N among several, nothing
// when ambiguous — and reports where it sits.
func resolveStoredPlacement(flow *workflows.Flow, ref string, atPos int) (storedPlacement, bool) {
	var matches []storedPlacement
	collectStoredPlacements(flow, ref, storedPlacement{}, &matches)
	switch {
	case len(matches) == 0:
		return storedPlacement{}, false
	case atPos > 0:
		if atPos > len(matches) {
			return storedPlacement{}, false
		}
		return matches[atPos-1], true
	case len(matches) == 1:
		return matches[0], true
	}
	return storedPlacement{}, false
}

func collectStoredPlacements(flow *workflows.Flow, ref string, at storedPlacement, out *[]storedPlacement) {
	if flow == nil {
		return
	}
	boundary := func(events []*workflows.BoundaryEvent) {
		for _, b := range events {
			if b != nil {
				nested := at
				nested.underNonInterrupting = at.underNonInterrupting || b.IsNonInterrupting()
				collectStoredPlacements(b.Flow, ref, nested, out)
			}
		}
	}
	for _, a := range flow.Activities {
		if a == nil {
			continue
		}
		if a.GetName() == ref || a.GetCaption() == ref {
			*out = append(*out, at)
		}
		switch t := a.(type) {
		case *workflows.ParallelSplitActivity:
			inPath := at
			inPath.underSplit = true
			for _, o := range t.Outcomes {
				if o != nil {
					collectStoredPlacements(o.Flow, ref, inPath, out)
				}
			}
		case *workflows.UserTask:
			for _, o := range t.Outcomes {
				if o != nil {
					collectStoredPlacements(o.Flow, ref, at, out)
				}
			}
			boundary(t.BoundaryEvents)
		case *workflows.CallMicroflowTask:
			for _, o := range t.Outcomes {
				collectStoredPlacements(o.GetFlow(), ref, at, out)
			}
			boundary(t.BoundaryEvents)
		case *workflows.WaitForNotificationActivity:
			boundary(t.BoundaryEvents)
		default:
			for _, f := range storedNestedFlows(a) {
				collectStoredPlacements(f, ref, at, out)
			}
		}
	}
}

// refuseWorkflowReturn is exec's guard against `return;` in a workflow body.
// Nothing is built from it, so without this `exec --no-check` would drop it —
// and a dropped `return` in a branch falls through into the main flow, the
// exact fault `end workflow` exists to prevent. MDL-WF11 reports the same
// thing at check time.
func refuseWorkflowReturn(activities []ast.WorkflowActivityNode) error {
	found := false
	walkWorkflowActivities(activities, func(a ast.WorkflowActivityNode) {
		if _, ok := a.(*ast.WorkflowReturnNode); ok {
			found = true
		}
	})
	if !found {
		return nil
	}
	return mdlerrors.NewValidationf("`return` is how a microflow ends — a workflow ends with `end workflow;`, " +
		"which ends the whole workflow rather than the branch it is written in [MDL-WF11]")
}

// alterWorkflowAddedActivities returns the activities an ALTER WORKFLOW statement
// introduces, in operation order.
func alterWorkflowAddedActivities(s *ast.AlterWorkflowStmt) []ast.WorkflowActivityNode {
	var added []ast.WorkflowActivityNode
	for _, op := range s.Operations {
		switch o := op.(type) {
		case *ast.InsertAfterOp:
			added = append(added, o.NewActivity)
		case *ast.ReplaceActivityOp:
			added = append(added, o.NewActivity)
		case *ast.InsertOutcomeOp:
			added = append(added, o.Activities...)
		case *ast.InsertPathOp:
			added = append(added, o.Activities...)
		case *ast.InsertBranchOp:
			added = append(added, o.Activities...)
		case *ast.InsertBoundaryEventOp:
			added = append(added, o.Activities...)
		}
	}
	return added
}
