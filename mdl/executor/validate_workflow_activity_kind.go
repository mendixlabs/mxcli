// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// An activity's outcome list is TYPED, and the three inserting ALTER WORKFLOW
// ops each write exactly one outcome type into it:
//
//	INSERT OUTCOME    -> Workflows$UserTaskOutcome
//	INSERT PATH       -> Workflows$ParallelSplitOutcome
//	INSERT CONDITION  -> Workflows$BooleanConditionOutcome / VoidConditionOutcome /
//	                     EnumerationValueConditionOutcome
//
// and `generated/metamodel` types the receiving list per activity:
//
//	SingleUserTaskActivity.Outcomes  []*WorkflowsUserTaskOutcome
//	MultiUserTaskActivity.Outcomes   []*WorkflowsUserTaskOutcome
//	ExclusiveSplitActivity.Outcomes  []*WorkflowsConditionOutcome
//	CallMicroflowTask.Outcomes       []*WorkflowsConditionOutcome
//	ParallelSplitActivity.Outcomes   []*WorkflowsParallelSplitOutcome
//
// None of the three ops checked what it was pointed at, so any of the six
// mismatches wrote an outcome the list cannot hold (ako/mxcli#415). Measured on
// mxbuild 11.10.0 against a project sitting at 0 errors, every one of them
// leaves a model Mendix cannot LOAD — `mx check` dies at "Loading the mpr file"
// before validating anything, and Studio Pro will not open the project:
//
//	INSERT OUTCOME on a decision / parallel split -> System.InvalidCastException
//	INSERT PATH / INSERT CONDITION on a wrong kind -> System.InvalidOperationException
//
// This is the MDL-WF04 class, and the remedy is the one that class always
// takes: refuse. Writing "the right type instead" is not available here — the
// author asked for a specific branch shape, and on a decision an added
// enumeration outcome would have to complete the whole value set or MxBuild
// reports CE6686 (MDL-WF06). There is already a correct op for each activity
// kind, so the refusal names it.
//
// BoundaryEvents is a second typed list on the same activities, declared by
// only four types (CallMicroflowTask, CallWorkflowActivity, MultiUserTask,
// SingleUserTask, WaitForNotification). That one did not corrupt: measured, an
// INSERT BOUNDARY EVENT on a decision printed "Altered workflow" and left the
// document byte-identical — a silent no-op rather than a load failure. It is
// refused here for the other reason: a statement that reports success and
// changes nothing is the "no silent side effects" rule, and the honest answer
// is that a decision has nowhere to put it.
//
// The DROP ops cannot write a wrong type, but one of them can ADDRESS the wrong
// thing. DROP PATH numbers "Path N" into the target's Outcomes list, whatever
// the target is, so `drop userTask1 path 1` removed the user task's first
// outcome and reported "Altered workflow" (ako/mxcli#791) — the project stays
// loadable, which is what made it silent. It is refused here on anything that
// is not a parallel split, as the mutators refuse it, and a path number the
// split does not have is refused rather than left to a lookup miss. DROP
// OUTCOME and DROP CONDITION address by value; the mutators match a value only
// against an outcome that stores one (an empty value used to hit the first
// path or void outcome), so they are not covered here.

// workflowOutcomeSlot is the list an ALTER WORKFLOW op writes into.
type workflowOutcomeSlot int

const (
	slotUserTaskOutcome workflowOutcomeSlot = iota
	slotParallelSplitOutcome
	slotConditionOutcome
	slotBoundaryEvent
)

// workflowOpSlot describes one inserting op for the diagnostic.
type workflowOpSlot struct {
	op      string              // the fragment `insert into` takes, in its canonical spelling
	slot    workflowOutcomeSlot // which typed list it writes into
	writes  string              // the BSON $Type it writes
	accepts string              // the activity kinds whose list holds that type
}

var workflowOpSlots = map[workflowOutcomeSlot]workflowOpSlot{
	slotUserTaskOutcome: {
		op: "outcomes '<name>' { … }", slot: slotUserTaskOutcome,
		writes: "Workflows$UserTaskOutcome", accepts: "a user task",
	},
	slotParallelSplitOutcome: {
		op: "path { … }", slot: slotParallelSplitOutcome,
		writes: "Workflows$ParallelSplitOutcome", accepts: "a parallel split",
	},
	slotConditionOutcome: {
		op: "outcomes <value> -> { … }", slot: slotConditionOutcome,
		writes: "Workflows$…ConditionOutcome", accepts: "a decision or a call microflow",
	},
	slotBoundaryEvent: {
		op: "boundary event …", slot: slotBoundaryEvent,
		writes: "a boundary event", accepts: "a user task, call microflow, call workflow or wait for notification",
	},
}

// activityAcceptsSlot reports whether this activity's document declares the
// list the op writes into. Typed against the semantic model rather than a table
// of storage-name strings, so a new activity kind cannot silently inherit the
// wrong answer by being absent from a list.
func activityAcceptsSlot(a workflows.WorkflowActivity, slot workflowOutcomeSlot) bool {
	switch a.(type) {
	case *workflows.UserTask:
		return slot == slotUserTaskOutcome || slot == slotBoundaryEvent
	case *workflows.ExclusiveSplitActivity:
		return slot == slotConditionOutcome
	case *workflows.CallMicroflowTask, *workflows.SystemTask:
		return slot == slotConditionOutcome || slot == slotBoundaryEvent
	case *workflows.ParallelSplitActivity:
		return slot == slotParallelSplitOutcome
	case *workflows.CallWorkflowActivity, *workflows.WaitForNotificationActivity:
		return slot == slotBoundaryEvent
	}
	// An activity kind this build does not model (GenericWorkflowActivity, or
	// one added by a newer Mendix) is left alone: refusing on a kind we cannot
	// reason about would reject scripts that are fine.
	return true
}

// opForActivity names the inserting op that DOES write into this activity's
// outcome list. A refusal that only says what the author's op takes leaves them
// to work out the remedy; every one of these targets has a correct op.
func opForActivity(a workflows.WorkflowActivity) string {
	switch a.(type) {
	case *workflows.UserTask:
		return "outcomes '<name>' { … }"
	case *workflows.ExclusiveSplitActivity, *workflows.CallMicroflowTask, *workflows.SystemTask:
		return "outcomes '<Module.Enumeration.Value>' -> { … }"
	case *workflows.ParallelSplitActivity:
		return "path { … }"
	}
	return ""
}

// describeActivityKind names an activity the way an author would.
func describeActivityKind(a workflows.WorkflowActivity) string {
	switch t := a.(type) {
	case *workflows.UserTask:
		if t.IsMulti {
			return "a multi user task"
		}
		return "a user task"
	case *workflows.ExclusiveSplitActivity:
		return "a decision"
	case *workflows.CallMicroflowTask, *workflows.SystemTask:
		return "a call microflow"
	case *workflows.ParallelSplitActivity:
		return "a parallel split"
	case *workflows.CallWorkflowActivity:
		return "a call workflow"
	case *workflows.WaitForNotificationActivity:
		return "a wait for notification"
	}
	return "a " + a.ActivityType() + " activity"
}

// validateAlterWorkflowActivityKinds refuses an inserting op aimed at an
// activity whose document has nowhere to put what the op writes.
//
// It is deliberately silent when the target cannot be resolved: an activity
// this pass cannot find is one the mutator reports on, with better information
// (not found, or ambiguous with the @N remedy). Refusing on a failed lookup
// would turn a resolution problem into a type problem and reject valid scripts.
func validateAlterWorkflowActivityKinds(ctx *ExecContext, s *ast.AlterWorkflowStmt) []string {
	wf := findStoredWorkflow(ctx, s.Name)
	if wf == nil || wf.Flow == nil {
		return nil
	}

	var errs []string
	check := func(ref string, atPos int, slot workflowOutcomeSlot) {
		target := resolveStoredActivity(wf.Flow, ref, atPos)
		if target == nil || activityAcceptsSlot(target, slot) {
			return
		}
		spec := workflowOpSlots[slot]
		kind := describeActivityKind(target)
		remedy := fmt.Sprintf("%s takes %s", spec.op, spec.accepts)
		if alt := opForActivity(target); alt != "" && slot != slotBoundaryEvent {
			remedy = fmt.Sprintf("use `insert into %s { %s }` to add a branch to %s (%s takes %s)",
				ref, alt, kind, spec.op, spec.accepts)
		}
		errs = append(errs, fmt.Sprintf(
			"`insert into %s { %s }` is refused: it writes %s, and %s cannot hold one — the project would not LOAD "+
				"(Studio Pro will not open it and `mx check` dies before validating anything); %s",
			ref, spec.op, spec.writes, kind, remedy))
	}

	added := map[workflows.WorkflowActivity]int{} // paths this statement already added, per split
	for _, op := range s.Operations {
		switch o := op.(type) {
		case *ast.InsertOutcomeOp:
			check(o.ActivityRef, o.AtPosition, slotUserTaskOutcome)
		case *ast.InsertPathOp:
			check(o.ActivityRef, o.AtPosition, slotParallelSplitOutcome)
			if split, ok := resolveStoredActivity(wf.Flow, o.ActivityRef, o.AtPosition).(*workflows.ParallelSplitActivity); ok {
				next := len(split.Outcomes) + added[split] + 1
				added[split]++
				if o.PathNumber > 0 && o.PathNumber != next {
					errs = append(errs, fmt.Sprintf(
						"insert into '%s' { path %d { … } } is refused: '%s' has %d paths, so a path added here is path %d — "+
							"write `path %d`, or leave the number out to append one",
						o.ActivityRef, o.PathNumber, o.ActivityRef, next-1, next, next))
				}
			}
		case *ast.DropPathOp:
			target := resolveStoredActivity(wf.Flow, o.ActivityRef, o.AtPosition)
			if target == nil {
				continue
			}
			split, ok := target.(*workflows.ParallelSplitActivity)
			if !ok {
				if !activityAcceptsSlot(target, slotParallelSplitOutcome) {
					kind := describeActivityKind(target)
					errs = append(errs, fmt.Sprintf(
						"`drop %s path …` is refused: '%s' is %s, not a parallel split, so it has no paths — "+
							"the number would pick one of its outcomes instead; drop an outcome by its value "+
							"(`drop %s outcome '<value>'`)",
						o.ActivityRef, o.ActivityRef, kind, o.ActivityRef))
				}
				continue
			}
			if _, err := backend.ParallelPathIndex(o.PathCaption, o.ActivityRef, len(split.Outcomes)+added[split]); err != nil {
				errs = append(errs, err.Error())
				continue
			}
			added[split]--
		case *ast.InsertBranchOp:
			check(o.ActivityRef, o.AtPosition, slotConditionOutcome)
		case *ast.InsertBoundaryEventOp:
			check(o.ActivityRef, o.AtPosition, slotBoundaryEvent)
		case *ast.InsertBeforeOp:
			// A flow begins with its start activity; anything before it is
			// CE9526 "Main process in workflow should start with a start event".
			if _, ok := resolveStoredActivity(wf.Flow, o.ActivityRef, o.AtPosition).(*workflows.StartWorkflowActivity); ok {
				errs = append(errs, fmt.Sprintf(
					"`insert before %s` is refused: '%s' is the workflow's start activity, and a flow must begin with it "+
						"(mx check CE9526); use `insert after %s { … }` to add activities at the start of the flow",
					o.ActivityRef, o.ActivityRef, o.ActivityRef))
			}
		}
	}
	return errs
}

// findStoredWorkflow returns the stored workflow for a qualified name, or nil.
func findStoredWorkflow(ctx *ExecContext, qn ast.QualifiedName) *workflows.Workflow {
	if ctx == nil || !ctx.Connected() {
		return nil
	}
	all, err := ctx.Backend.ListWorkflows()
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	for _, wf := range all {
		if wf.Name != qn.Name {
			continue
		}
		if h.GetModuleName(h.FindModuleID(wf.ContainerID)) == qn.Module {
			return wf
		}
	}
	return nil
}

// resolveStoredActivity mirrors the mutator's own addressing
// (wfmutator.findActivityByCaption): match on Name or Caption, search nested
// flows too, and use @N to pick among several. Without @N a Name match wins
// over caption matches, as the mutator's preferNameMatches does — without that
// a jump captioned with its target's name made the reference look ambiguous
// here, and every check-time guard went silent on a target the mutator
// resolved (ako/mxcli#808). It returns nil rather than guessing when the
// reference is ambiguous, so the mutator's message wins.
func resolveStoredActivity(flow *workflows.Flow, ref string, atPos int) workflows.WorkflowActivity {
	var matches []workflows.WorkflowActivity
	collectStoredActivities(flow, ref, &matches)
	if atPos == 0 {
		var named []workflows.WorkflowActivity
		for _, a := range matches {
			if a.GetName() == ref {
				named = append(named, a)
			}
		}
		if len(named) > 0 {
			matches = named
		}
	}
	switch {
	case len(matches) == 0:
		return nil
	case atPos > 0:
		if atPos > len(matches) {
			return nil
		}
		return matches[atPos-1]
	case len(matches) == 1:
		return matches[0]
	}
	return nil
}

func collectStoredActivities(flow *workflows.Flow, ref string, out *[]workflows.WorkflowActivity) {
	if flow == nil {
		return
	}
	for _, a := range flow.Activities {
		if a == nil {
			continue
		}
		if a.GetName() == ref || a.GetCaption() == ref {
			*out = append(*out, a)
		}
		for _, nested := range storedNestedFlows(a) {
			collectStoredActivities(nested, ref, out)
		}
	}
}

// storedNestedFlows returns every sub-flow an activity owns. Condition outcomes
// go through the ConditionOutcome interface rather than a type switch over the
// three implementations — the same reasoning as the auto-bind walk, where a
// per-variant switch is how the enumeration branch got missed (ako/mxcli#417).
func storedNestedFlows(a workflows.WorkflowActivity) []*workflows.Flow {
	var out []*workflows.Flow
	add := func(f *workflows.Flow) {
		if f != nil {
			out = append(out, f)
		}
	}
	switch t := a.(type) {
	case *workflows.UserTask:
		for _, o := range t.Outcomes {
			add(o.Flow)
		}
		for _, b := range t.BoundaryEvents {
			add(b.Flow)
		}
	case *workflows.ExclusiveSplitActivity:
		for _, o := range t.Outcomes {
			add(o.GetFlow())
		}
	case *workflows.CallMicroflowTask:
		for _, o := range t.Outcomes {
			add(o.GetFlow())
		}
		for _, b := range t.BoundaryEvents {
			add(b.Flow)
		}
	case *workflows.SystemTask:
		for _, o := range t.Outcomes {
			add(o.GetFlow())
		}
	case *workflows.ParallelSplitActivity:
		for _, o := range t.Outcomes {
			add(o.Flow)
		}
	case *workflows.CallWorkflowActivity:
		for _, b := range t.BoundaryEvents {
			add(b.Flow)
		}
	case *workflows.WaitForNotificationActivity:
		for _, b := range t.BoundaryEvents {
			add(b.Flow)
		}
	}
	return out
}
