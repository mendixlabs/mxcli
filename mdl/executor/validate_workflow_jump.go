// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// jumpTargetRule is MDL-WF05: `jump to <name>` must name an activity in the
// same workflow.
//
// A jump target is the ONLY intra-document reference a workflow has. External
// references — microflows, pages, entities — are resolved twice (check
// --references, and again in execCreateWorkflow), but nothing looked at a jump,
// so an unresolved target was written out and surfaced only under the native
// validator as CE6681 "It is not possible to jump to end activities or jump-to
// activities" — an error describing a different fault, because Mendix resolves
// TargetActivity by NAME and the only activity carrying the missing name was the
// jump itself. See mendixlabs/mxcli#1005.
const jumpTargetRule = "MDL-WF05"

// ValidateWorkflowJumpTargets reports every `jump to` whose target does not name
// an activity in the workflow.
//
// The candidate names are computed by running the real builders over the AST and
// reading the names off the result, not by re-deriving them here: a call
// microflow activity is named after the microflow it calls, a user task after
// its declared name, and a second copy of those rules would drift from the
// writer and produce exactly the false refusals this rule exists to prevent.
// None of it needs a project — every name comes from the script.
func ValidateWorkflowJumpTargets(stmt *ast.CreateWorkflowStmt) []linter.Violation {
	if stmt == nil {
		return nil
	}

	// Each flow is its own jump scope. Measured on mxbuild 11.13.0: a jump inside
	// an event sub-process to an activity of the same sub-process, or to its start
	// event, builds; a jump into another sub-process, or between one and the main
	// flow, is CE6682 ("not possible to jump into or out of a Event sub-process
	// path").
	type jumpScope struct {
		label      string
		activities []ast.WorkflowActivityNode
		targets    map[string]bool
	}
	mainTargets := map[string]bool{}
	collectJumpableNames(buildWorkflowActivities(stmt.Activities), mainTargets)
	scopes := []jumpScope{{label: "the main flow", activities: stmt.Activities, targets: mainTargets}}
	for i, esp := range buildEventSubProcesses(stmt.EventSubProcesses) {
		targets := map[string]bool{}
		collectJumpableNames(esp.Flow.Activities, targets)
		scopes = append(scopes, jumpScope{
			label:      "event subprocess " + stmt.EventSubProcesses[i].Name,
			activities: stmt.EventSubProcesses[i].Activities,
			targets:    targets,
		})
	}

	loc := linter.Location{
		Module:       stmt.Name.Module,
		DocumentType: "workflow",
		DocumentName: stmt.Name.Name,
	}

	var out []linter.Violation
	for _, sc := range scopes {
		walkWorkflowActivities(sc.activities, func(a ast.WorkflowActivityNode) {
			n, ok := a.(*ast.WorkflowJumpToNode)
			if !ok || n.Target == "" || sc.targets[n.Target] {
				return
			}
			for _, other := range scopes {
				if other.targets[n.Target] {
					out = append(out, linter.Violation{
						RuleID:   jumpTargetRule,
						Severity: linter.SeverityError,
						Location: loc,
						Message: fmt.Sprintf("jump target %q is in %s, but the jump is in %s — Mendix refuses a jump into or "+
							"out of an event sub-process; the build fails CE6682", n.Target, other.label, sc.label),
						Suggestion: "Jump only within the same flow. An event sub-process ends with `end workflow;`, and is started by notifying its start event.",
					})
					return
				}
			}
			out = append(out, linter.Violation{
				RuleID:   jumpTargetRule,
				Severity: linter.SeverityError,
				Location: loc,
				Message: fmt.Sprintf("jump target %q does not match any activity in this workflow — "+
					"Mendix resolves a jump by activity name, so this is written as a jump to itself and "+
					"the build fails with CE6681 (\"not possible to jump to end activities or jump-to activities\")",
					n.Target),
				Suggestion: jumpTargetSuggestion(sc.targets),
			})
		})
	}
	return out
}

// jumpTargetSuggestion lists what the author could have meant. The names are not
// obvious from the script — a `call microflow M.SUB_Check` activity is named
// SUB_Check, not by anything written at the call site — which is most of why the
// wrong name is easy to reach in the first place.
func jumpTargetSuggestion(targets map[string]bool) string {
	if len(targets) == 0 {
		return "This workflow has no activity a jump can target."
	}
	names := make([]string, 0, len(targets))
	for n := range targets {
		names = append(names, n)
	}
	sort.Strings(names)
	return "Valid targets in this workflow: " + strings.Join(names, ", ") +
		". A `call microflow` activity is named after the microflow it calls."
}

// collectJumpableNames gathers the names of every activity a jump may target,
// recursing into outcome, path and boundary-event flows.
//
// Jump activities are deliberately excluded: Mendix refuses a jump to a jump
// (CE6681), so accepting one here would let the rule bless the exact model it
// exists to prevent.
func collectJumpableNames(acts []workflows.WorkflowActivity, out map[string]bool) {
	add := func(name string) {
		if name != "" {
			out[name] = true
			// autoBindWorkflowParameters sanitises some names before they are
			// stored, so a jump written against either spelling resolves.
			out[sanitizeActivityName(name)] = true
		}
	}
	for _, act := range acts {
		switch a := act.(type) {
		case *workflows.UserTask:
			add(a.Name)
			for _, o := range a.Outcomes {
				if o.Flow != nil {
					collectJumpableNames(o.Flow.Activities, out)
				}
			}
			collectBoundaryEventNames(a.BoundaryEvents, out)
		case *workflows.CallMicroflowTask:
			add(a.Name)
			collectConditionOutcomeNames(a.Outcomes, out)
		case *workflows.SystemTask:
			add(a.Name)
		case *workflows.CallWorkflowActivity:
			add(a.Name)
		case *workflows.ExclusiveSplitActivity:
			add(a.Name)
			collectConditionOutcomeNames(a.Outcomes, out)
		case *workflows.ParallelSplitActivity:
			add(a.Name)
			for _, o := range a.Outcomes {
				if o.Flow != nil {
					collectJumpableNames(o.Flow.Activities, out)
				}
			}
		case *workflows.WaitForTimerActivity:
			add(a.Name)
		case *workflows.NotificationActivity:
			add(a.Name)
		case *workflows.EventSubProcessStartActivity:
			// Measured: a jump to its own start event builds (it resets the
			// sub-process to waiting).
			add(a.Name)
		case *workflows.WaitForNotificationActivity:
			add(a.Name)
			collectBoundaryEventNames(a.BoundaryEvents, out)
		}
	}
}

func collectConditionOutcomeNames(outcomes []workflows.ConditionOutcome, out map[string]bool) {
	for _, outcome := range outcomes {
		switch o := outcome.(type) {
		case *workflows.BooleanConditionOutcome:
			if o.Flow != nil {
				collectJumpableNames(o.Flow.Activities, out)
			}
		case *workflows.EnumerationValueConditionOutcome:
			if o.Flow != nil {
				collectJumpableNames(o.Flow.Activities, out)
			}
		case *workflows.VoidConditionOutcome:
			if o.Flow != nil {
				collectJumpableNames(o.Flow.Activities, out)
			}
		}
	}
}

func collectBoundaryEventNames(events []*workflows.BoundaryEvent, out map[string]bool) {
	for _, e := range events {
		if e != nil && e.Flow != nil {
			collectJumpableNames(e.Flow.Activities, out)
		}
	}
}
