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
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A decision / call-microflow outcome is stored in
// EnumerationValueConditionOutcome.Value, which Mendix loads through
// EnumerationValueIdentifier.FromString. That parse is strict and it runs at
// LOAD time, before any consistency check: a value it rejects does not produce
// a CE number, it makes the whole project unopenable in Studio Pro and mxbuild
// (`StorageLoadException`, UnitLoader). mxcli wrote the outcome label verbatim,
// so a perfectly ordinary script corrupted the model while `check`, `exec` and
// `describe` all reported success — ako/mxcli#1031, ako/mxcli#1065.
//
// The identifier is qualified to exactly three segments,
// Module.Enumeration.Value. Measured on 11.10.0, one workflow per copy of the
// same app, verdict = the literal mx check line:
//
//	'OutcomeA'             (1 segment)  -> StorageLoadException, project unloadable
//	'Status.OutcomeA'      (2 segments) -> StorageLoadException, project unloadable
//	'WFP.Status.OutcomeA'  (3 segments) -> 0 errors
//
// which agrees with the stored corpus: every EnumerationValueConditionOutcome
// in the demo apps holds Module.Enum.Value (7 of 7 non-empty). The two-segment
// row is the one worth keeping — "qualify it" is ambiguous without it, and
// enum-in-the-same-module is exactly the case an author would shorten.
//
// wfOutcomeQualifiedRe is what Mendix accepts; wfOutcomeIdentRe is only used to
// tell an author who wrote a plausible identifier from one who wrote free text,
// so the two get different advice.
var (
	wfOutcomeQualifiedRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_]*$`)
	wfOutcomeIdentRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
)

// ValidateWorkflow checks a workflow for constructs that pass parsing but are
// rejected by MxBuild, without requiring a project connection.
//
//   - MDL-WF01: user task without a page (CE1834)
//   - MDL-WF02: single-outcome user task containing nested activities (CE1876)
//   - MDL-WF03: decision / call-microflow outcome that is not a qualified
//     enumeration value identifier (Module.Enumeration.Value) — a value the
//     loader rejects makes the project unopenable, not merely un-buildable
//   - MDL-WF06: enumeration outcomes with no empty-valued branch (CE6686)
//   - MDL-WF04: standalone `annotation` in a workflow body (unloadable model)
//   - MDL-WF05: `jump to` a target that names no activity (see validate_workflow_jump.go)
//   - MDL-WF08..11: where `end workflow` may go, and `return` (see validate_workflow_end.go)
func ValidateWorkflow(stmt *ast.CreateWorkflowStmt) []linter.Violation {
	var out []linter.Violation
	loc := linter.Location{
		Module:       stmt.Name.Module,
		DocumentType: "workflow",
		DocumentName: stmt.Name.Name,
	}
	walkWorkflowActivities(workflowStatementActivities(stmt), func(a ast.WorkflowActivityNode) {
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
				msg := fmt.Sprintf("user task %s has a single outcome with nested activities — MxBuild rejects this (CE1876)", label)
				suggestion := "Move the activities to the workflow's main flow after the user task, or add a second outcome."
				// "Move them to the main flow" is wrong advice for an End: moved
				// there it is a parse error, and the workflow ends at its own
				// `end workflow` anyway.
				if onlyEnd(n.Outcomes[0].Activities) {
					msg = fmt.Sprintf("user task %s has a single outcome that holds `end workflow` — a single outcome may not hold activities at all, so MxBuild rejects this (CE1876), and nothing after the task could be reached", label)
					suggestion = "Add a second outcome that continues, if the task is meant to decide whether the workflow ends; otherwise remove `end workflow;` — the workflow ends at its own `end workflow` after the task."
				}
				out = append(out, linter.Violation{
					RuleID:     "MDL-WF02",
					Severity:   linter.SeverityError,
					Location:   loc,
					Message:    msg,
					Suggestion: suggestion,
				})
			}
		case *ast.WorkflowDecisionNode:
			out = append(out, checkWorkflowOutcomeNames(n.Outcomes, "decision", loc)...)
			out = append(out, checkWorkflowEmptyEnumOutcome(n.Outcomes, "decision", workflowDecisionLabel(n), loc)...)
		case *ast.WorkflowCallMicroflowNode:
			out = append(out, checkWorkflowOutcomeNames(n.Outcomes, "call microflow", loc)...)
			out = append(out, checkWorkflowEmptyEnumOutcome(n.Outcomes, "call microflow", workflowCallMicroflowLabel(n), loc)...)
		case *ast.WorkflowAnnotationActivityNode:
			// MDL-WF04 — a standalone annotation is written into the workflow's
			// activity flow, but Mendix constructs every child of that list with a
			// Flow parent, and no annotation type takes one. The result is not a
			// build error but a project Mendix cannot LOAD, so Studio Pro will not
			// open it and `mx check` dies before validating anything.
			out = append(out, linter.Violation{
				RuleID:     "MDL-WF04",
				Severity:   linter.SeverityError,
				Location:   loc,
				Message:    "a standalone `annotation` in a workflow body produces a model Mendix cannot load (the annotation is placed in the activity flow, which accepts only flow elements) — Studio Pro will not open the project",
				Suggestion: "Remove the `annotation` statement. Use an MDL comment (`-- ...`) to keep the note in the script; workflow canvas annotations are not yet writable.",
			})
		}
	})
	out = append(out, ValidateWorkflowJumpTargets(stmt)...)
	out = append(out, ValidateWorkflowEnds(stmt)...)
	out = append(out, ValidateWorkflowEventTypes(stmt)...)
	out = append(out, ValidateWorkflowCompletionRules(stmt)...)
	out = append(out, ValidateWorkflowEventSubProcesses(stmt)...)
	return out
}

// onlyEnd reports whether a block holds nothing but `end workflow`.
func onlyEnd(acts []ast.WorkflowActivityNode) bool {
	if len(acts) != 1 {
		return false
	}
	_, ok := acts[0].(*ast.WorkflowEndNode)
	return ok
}

// checkWorkflowOutcomeNames flags condition-outcome values (decision / call
// microflow branches) that Mendix cannot load as an EnumerationValueIdentifier
// (MDL-WF03). See the regex block above for the measurements.
//
// The empty value is skipped deliberately: an enumeration decision carries one
// extra outcome with Value "" for "none of the above", which Studio Pro writes
// on every enum decision and the loader accepts.
func checkWorkflowOutcomeNames(outcomes []ast.WorkflowConditionOutcomeNode, kind string, loc linter.Location) []linter.Violation {
	var out []linter.Violation
	for _, o := range outcomes {
		if v := checkWorkflowOutcomeValue(o.Value, kind, loc); v != nil {
			out = append(out, *v)
		}
	}
	return out
}

// checkWorkflowOutcomeValue applies MDL-WF03 to a single outcome value. It is
// split out because ALTER WORKFLOW … INSERT BRANCH writes the same field
// through a different door (wfmutator.InsertBranch), and a guard that covered
// only CREATE would leave the corrupting write one statement away.
func checkWorkflowOutcomeValue(value, kind string, loc linter.Location) *linter.Violation {
	if value == "" || wfOutcomeQualifiedRe.MatchString(value) {
		return nil
	}
	// "True" / "False" / "Default" never reach storage as an enumeration value —
	// the builder turns them into a Boolean or Void outcome.
	switch value {
	case "True", "False", "Default":
		return nil
	}

	var why, fix string
	if wfOutcomeIdentRe.MatchString(value) {
		why = "is not fully qualified"
		fix = fmt.Sprintf(
			"Write the outcome as Module.Enumeration.Value (e.g. 'Sales.ENUM_Status.%s'). "+
				"Mendix stores it as an EnumerationValueIdentifier and parses it when the project is LOADED, "+
				"so a short name is not a build error — it makes the project unopenable in Studio Pro and mxbuild. "+
				"Two segments are refused as firmly as one, including when the enumeration is in the same module.",
			lastSegment(value))
	} else {
		why = "is not an enumeration value identifier"
		fix = "Write the outcome as Module.Enumeration.Value. A decision branches on the enumeration returned by " +
			"its expression, so each outcome must name one of that enumeration's values — free text with spaces or " +
			"punctuation is not a name Mendix can resolve."
	}

	return &linter.Violation{
		RuleID:   "MDL-WF03",
		Severity: linter.SeverityError,
		Location: loc,
		Message: fmt.Sprintf(
			"%s outcome '%s' %s — Mendix cannot load a project whose EnumerationValueConditionOutcome.Value "+
				"is not Module.Enumeration.Value (StorageLoadException, no CE number)", kind, value, why),
		Suggestion: fix,
	}
}

// lastSegment returns the part after the final dot, for use in a suggestion.
func lastSegment(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

// checkWorkflowEmptyEnumOutcome flags an activity that branches on an
// enumeration without an outcome for the EMPTY value (MDL-WF06).
//
// Mendix generates one outcome per enumeration value **plus one with an empty
// value**, and mxbuild compares the stored set against that generated set:
// anything else is CE6686 ("The current outcomes of the ... do not match the
// configured expression/microflow. Regenerate the outcomes."). Studio Pro's own
// documents agree — every enum decision in the FactoryManagement demo app
// stores the extra Workflows$EnumerationValueConditionOutcome with Value ”.
//
// Measured on mxbuild 11.10.0, in a blank 11.10.0 app:
//
//   - decision on `$WorkflowContext/Kind` with the two enum values → 1 error,
//     CE6686; adding `” -> { }` → 0 errors.
//   - the same decision on an attribute carrying a REQUIRED (not null)
//     validation rule → still CE6686. The empty outcome is not about whether
//     the value can be empty in practice, which is why this is an error rather
//     than a warning.
//   - a call-microflow activity branching on an enumeration-returning microflow
//     → the same CE6686 ("...of the call microflow activity do not match the
//     configured microflow"), cleared the same way. Hence both call sites.
//   - a boolean decision (true/false) is 0 errors with no empty outcome, so the
//     rule must classify outcomes exactly as buildConditionOutcome does.
//
// mxbuild wants set EQUALITY, so a missing enumeration *value* is CE6686 too
// (measured: Standard + ” on a two-value enum is 1 error). That half needs the
// enumeration's definition and so belongs to the reference pass, not here; this
// rule reports only what is decidable from the statement alone.
func checkWorkflowEmptyEnumOutcome(outcomes []ast.WorkflowConditionOutcomeNode, kind, label string, loc linter.Location) []linter.Violation {
	var enumValues []string
	for _, o := range outcomes {
		switch o.Value {
		case "True", "False":
			// A boolean branch: buildConditionOutcome emits a
			// BooleanConditionOutcome and mxbuild wants exactly true/false.
			return nil
		case "Default":
			// A VoidConditionOutcome — not an enumeration branch.
			continue
		case "":
			// The empty-valued enumeration outcome this rule is about.
			return nil
		default:
			enumValues = append(enumValues, o.Value)
		}
	}
	if len(enumValues) == 0 {
		return nil
	}
	// Quote the CE6686 text MxBuild actually prints for this activity kind, so
	// searching the build output for it lands here.
	ceText := "the current outcomes of the decision activity do not match the configured expression"
	if kind == "call microflow" {
		ceText = "the current outcomes of the call microflow activity do not match the configured microflow"
	}
	return []linter.Violation{{
		RuleID:   "MDL-WF06",
		Severity: linter.SeverityError,
		Location: loc,
		Message: fmt.Sprintf(
			"%s %s branches on an enumeration but has no outcome for the empty value — MxBuild rejects this (CE6686 %q)",
			kind, label, ceText),
		Suggestion: "Add an empty outcome alongside the named values: `'' -> { }`. Mendix generates one outcome per enumeration value plus one for the empty value, and the stored set must match — a required (not null) attribute does not exempt it.",
	}}
}

// workflowDecisionLabel returns a human-readable label for a decision.
func workflowDecisionLabel(n *ast.WorkflowDecisionNode) string {
	switch {
	case n.Name != "":
		return "'" + n.Name + "'"
	case n.Caption != "":
		return "'" + n.Caption + "'"
	case n.Expression != "":
		return "on '" + n.Expression + "'"
	}
	return "(unnamed)"
}

// workflowCallMicroflowLabel returns a human-readable label for a call-microflow
// activity.
func workflowCallMicroflowLabel(n *ast.WorkflowCallMicroflowNode) string {
	if n.Name != "" {
		return "'" + n.Name + "'"
	}
	if qn := n.Microflow.String(); qn != "" && qn != "." {
		return "'" + qn + "'"
	}
	return "(unnamed)"
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

// ValidateAlterWorkflow applies the rules that ALTER WORKFLOW can reach:
// MDL-WF03 to … INSERT CONDITION, and MDL-WF06 to every activity the statement
// introduces. Both writes land in exactly the fields a CREATE-time outcome
// does, so guarding only CREATE would leave each one a keyword away.
//
// The other rules are not simply un-ported. MDL-WF01/WF02 describe a state a
// later op in the same script can still repair (`SET ACTIVITY … PAGE`), and
// MDL-WF05 resolves jump targets against activities the statement cannot see.
// An inserted activity's outcome SET, by contrast, is complete where it is
// written — `INSERT OUTCOME` cannot extend it, since on a decision it writes a
// UserTaskOutcome into a ConditionOutcome list and yields a model Mendix cannot
// load at all (ako/mxcli#415).
func ValidateAlterWorkflow(stmt *ast.AlterWorkflowStmt) []linter.Violation {
	var out []linter.Violation
	loc := linter.Location{
		Module:       stmt.Name.Module,
		DocumentType: "workflow",
		DocumentName: stmt.Name.Name,
	}

	// MDL-WF06 over the activities the statement adds.
	var added []ast.WorkflowActivityNode
	for _, op := range stmt.Operations {
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
	walkWorkflowActivities(added, func(a ast.WorkflowActivityNode) {
		switch n := a.(type) {
		case *ast.WorkflowDecisionNode:
			out = append(out, checkWorkflowEmptyEnumOutcome(n.Outcomes, "decision", workflowDecisionLabel(n), loc)...)
		case *ast.WorkflowCallMicroflowNode:
			out = append(out, checkWorkflowEmptyEnumOutcome(n.Outcomes, "call microflow", workflowCallMicroflowLabel(n), loc)...)
		}
	})

	// MDL-WF03 over the condition an INSERT CONDITION writes.
	for _, op := range stmt.Operations {
		ins, ok := op.(*ast.InsertBranchOp)
		if !ok {
			continue
		}
		// The mutator lower-cases before dispatching, so any casing of these
		// three becomes a Boolean or Void outcome and never reaches the
		// enumeration field. CREATE is stricter — there a quoted 'true' IS
		// written as an enumeration value — so the fold lives here, not in the
		// shared check.
		if strings.EqualFold(ins.Condition, "true") ||
			strings.EqualFold(ins.Condition, "false") ||
			strings.EqualFold(ins.Condition, "default") {
			continue
		}
		if v := checkWorkflowOutcomeValue(ins.Condition, "insert condition", loc); v != nil {
			out = append(out, *v)
		}
	}
	return append(out, ValidateAlterWorkflowEnds(stmt)...)
}
