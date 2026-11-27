// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Studio Pro names a workflow's implicit activities itself — start1, end1,
// end2, … — and describe prints no name for them: the main flow's start and
// end are implied by `begin … end workflow`, an `end workflow` in a branch and
// the end of an event sub-process have no name clause, and neither do a jump or
// the end-of-path markers. `create or modify` rebuilt them as Start, End, End2,
// so running a Studio Pro workflow's own describe output renamed every one of
// them (#743, ako/TestApp workflow.Workflow1). ADR-0012: what the statement
// cannot state is carried from what is stored.
//
// The pairing is structural. Within a pair of corresponding flows, an activity
// the statement names is paired with the stored activity of that name and type;
// an activity it cannot name is paired with the stored activity of the same
// type at the same ordinal among that type (the first End with the first End).
// The flows nested in a paired activity — outcomes, then boundary events — are
// paired by position when both sides have the same number of them. Anything
// that does not pair keeps the name the rebuild gives it, and the
// deduplication that runs afterwards still holds every name to CE0495.

// carryWorkflowActivityNames carries the stored names of the activities in
// declared that describe cannot name, and recurses into the flows nested in
// every activity that pairs.
func carryWorkflowActivityNames(declared, stored []workflows.WorkflowActivity) {
	named := map[string]workflows.WorkflowActivity{}
	unnamed := map[string][]workflows.WorkflowActivity{}
	for _, a := range stored {
		if a == nil {
			continue
		}
		if unnamedWorkflowActivity(a) {
			k := fmt.Sprintf("%T", a)
			unnamed[k] = append(unnamed[k], a)
		} else if a.GetName() != "" {
			named[a.GetName()] = a
		}
	}
	ordinal := map[string]int{}
	for _, a := range declared {
		if a == nil {
			continue
		}
		var match workflows.WorkflowActivity
		k := fmt.Sprintf("%T", a)
		if unnamedWorkflowActivity(a) {
			i := ordinal[k]
			ordinal[k]++
			if i < len(unnamed[k]) {
				match = unnamed[k][i]
				if n := match.GetName(); n != "" {
					a.SetName(n)
				}
			}
		} else if m, ok := named[a.GetName()]; ok && fmt.Sprintf("%T", m) == k {
			match = m
		}
		if match == nil {
			continue
		}
		carryBoundaryEventNames(a, match)
		// Outcome flows and boundary-event flows are paired separately: a
		// stored call microflow holds a void outcome the rebuild does not, and
		// counting both in one list made every boundary path of such an
		// activity miss its stored names — the End closing it then took a
		// name another End already had (CE0495).
		if d, s := workflowFlowSlots(a), workflowFlowSlots(match); len(d) == len(s) {
			for i := range d {
				if d[i] != nil && s[i] != nil {
					carryWorkflowActivityNames(d[i].Activities, s[i].Activities)
				}
			}
		}
		if d, s := activityBoundaryEvents(a), activityBoundaryEvents(match); len(d) == len(s) {
			for i := range d {
				if d[i] != nil && s[i] != nil && d[i].Flow != nil && s[i].Flow != nil {
					carryWorkflowActivityNames(d[i].Flow.Activities, s[i].Flow.Activities)
				}
			}
		}
	}
}

// carryEventSubProcessActivityNames pairs event sub-processes by name, which
// describe prints, and carries the names inside each.
func carryEventSubProcessActivityNames(declared, stored []*workflows.EventSubProcess) {
	byName := map[string]*workflows.EventSubProcess{}
	for _, esp := range stored {
		if esp != nil && esp.Name != "" {
			byName[esp.Name] = esp
		}
	}
	for _, esp := range declared {
		if esp == nil || esp.Flow == nil {
			continue
		}
		if s, ok := byName[esp.Name]; ok && s.Flow != nil {
			carryWorkflowActivityNames(esp.Flow.Activities, s.Flow.Activities)
		}
	}
}

// unnamedWorkflowActivity reports whether describe prints no name for act, so
// the statement cannot state it.
func unnamedWorkflowActivity(act workflows.WorkflowActivity) bool {
	switch act.(type) {
	case *workflows.StartWorkflowActivity, *workflows.EndWorkflowActivity,
		*workflows.EndOfParallelSplitPathActivity, *workflows.EndOfBoundaryEventPathActivity,
		*workflows.JumpToActivity:
		return true
	}
	return false
}

// workflowFlowSlots lists the outcome flows nested in act in a fixed order,
// keeping an absent flow as nil so the two sides of a pair line up by
// position. Boundary-event flows are paired on their own (see
// carryWorkflowActivityNames). nestedFlows skips absent flows, which suits a walk
// but not a pairing: the rebuild leaves an empty outcome's flow nil where the
// stored one has an empty Flow.
func workflowFlowSlots(act workflows.WorkflowActivity) []*workflows.Flow {
	var flows []*workflows.Flow
	conditions := func(outcomes []workflows.ConditionOutcome) {
		for _, o := range outcomes {
			if o == nil {
				flows = append(flows, nil)
				continue
			}
			flows = append(flows, o.GetFlow())
		}
	}
	switch a := act.(type) {
	case *workflows.CallMicroflowTask:
		conditions(a.Outcomes)
	case *workflows.SystemTask:
		conditions(a.Outcomes)
	case *workflows.ExclusiveSplitActivity:
		conditions(a.Outcomes)
	case *workflows.UserTask:
		for _, o := range a.Outcomes {
			if o == nil {
				flows = append(flows, nil)
				continue
			}
			flows = append(flows, o.Flow)
		}
	case *workflows.ParallelSplitActivity:
		for _, o := range a.Outcomes {
			if o == nil {
				flows = append(flows, nil)
				continue
			}
			flows = append(flows, o.Flow)
		}
	}
	return flows
}

// carryBoundaryEventNames gives each of declared's timer boundary events the
// name its stored counterpart has, pairing them by position when both sides
// hold the same kinds in the same order. Mendix 11.17 names timers
// (boundaryEvent1, …) and MDL cannot state the name, so without this a rewrite
// renumbered them (ako/mxcli#2616). A notification event's name is its MDL
// name and is not carried.
func carryBoundaryEventNames(declared, stored workflows.WorkflowActivity) {
	d, s := activityBoundaryEvents(declared), activityBoundaryEvents(stored)
	if len(d) == 0 || len(d) != len(s) {
		return
	}
	for i := range d {
		if d[i] == nil || s[i] == nil || d[i].EventType != s[i].EventType {
			return
		}
	}
	for i := range d {
		if !d[i].IsNotification() && d[i].Name == "" {
			d[i].Name = s[i].Name
		}
	}
}
