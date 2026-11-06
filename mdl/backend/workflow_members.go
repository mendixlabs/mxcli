// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"fmt"
	"strconv"
	"strings"
)

// WorkflowMemberList is one of the typed lists an ALTER WORKFLOW member op
// reads or writes on an activity. An activity's Outcomes list holds a
// different element type per activity kind (the metamodel types it:
// UserTaskOutcome on a user task, ParallelSplitOutcome on a parallel split,
// ConditionOutcome on a decision or a call microflow), so an op that addresses
// one kind's list must not be let loose on another's: ako/mxcli#415 wrote the
// wrong element type (a project Mendix cannot load), and ako/mxcli#791 read
// `path 1` as "the first outcome" of a user task and deleted it.
type WorkflowMemberList int

const (
	// WorkflowUserTaskOutcomes is a user task's (single or multi) outcomes.
	WorkflowUserTaskOutcomes WorkflowMemberList = iota
	// WorkflowParallelPaths is a parallel split's paths.
	WorkflowParallelPaths
	// WorkflowConditionOutcomes is a decision's or a call microflow's outcomes.
	WorkflowConditionOutcomes
	// WorkflowBoundaryEvents is an activity's boundary events.
	WorkflowBoundaryEvents
)

// what the list holds, as the refusal names it.
func (l WorkflowMemberList) String() string {
	switch l {
	case WorkflowUserTaskOutcomes:
		return "user task outcomes"
	case WorkflowParallelPaths:
		return "parallel split paths"
	case WorkflowConditionOutcomes:
		return "condition outcomes"
	case WorkflowBoundaryEvents:
		return "boundary events"
	}
	return "members"
}

// workflowMemberLists is, per activity storage $Type, the member lists its
// document declares (modelsdk/gen/workflows). A type absent from this table is
// one this build does not know, and is left alone rather than refused.
var workflowMemberLists = map[string][]WorkflowMemberList{
	"Workflows$SingleUserTaskActivity":      {WorkflowUserTaskOutcomes, WorkflowBoundaryEvents},
	"Workflows$UserTaskActivity":            {WorkflowUserTaskOutcomes, WorkflowBoundaryEvents},
	"Workflows$UserTask":                    {WorkflowUserTaskOutcomes, WorkflowBoundaryEvents},
	"Workflows$MultiUserTaskActivity":       {WorkflowUserTaskOutcomes, WorkflowBoundaryEvents},
	"Workflows$ParallelSplitActivity":       {WorkflowParallelPaths},
	"Workflows$ExclusiveSplitActivity":      {WorkflowConditionOutcomes},
	"Workflows$CallMicroflowTask":           {WorkflowConditionOutcomes, WorkflowBoundaryEvents},
	"Workflows$CallMicroflowActivity":       {WorkflowConditionOutcomes, WorkflowBoundaryEvents},
	"Workflows$AIAgentTaskActivity":         {WorkflowConditionOutcomes, WorkflowBoundaryEvents},
	"Workflows$CallWorkflowActivity":        {WorkflowBoundaryEvents},
	"Workflows$WaitForNotificationActivity": {WorkflowBoundaryEvents},
	"Workflows$JumpToActivity":              nil,
	"Workflows$WaitForTimerActivity":        nil,
	"Workflows$NotificationActivity":        nil,
	"Workflows$EndWorkflowActivity":         nil,
	"Workflows$StartWorkflowActivity":       nil,
	"Workflows$Annotation":                  nil,
}

// CheckWorkflowMemberList refuses an op on list aimed at an activity of the
// given storage $Type whose document does not declare that list. ref is the
// activity as the author addressed it, and op the member as they wrote it.
func CheckWorkflowMemberList(storageType, ref, op string, list WorkflowMemberList) error {
	lists, known := workflowMemberLists[storageType]
	if !known {
		return nil
	}
	for _, l := range lists {
		if l == list {
			return nil
		}
	}
	kind := WorkflowActivityKind(storageType)
	if storageType == "Workflows$UserTask" {
		kind = "user task"
	}
	if list == WorkflowParallelPaths {
		return fmt.Errorf("%s: %q is a %s, not a parallel split, so it has no paths to address — "+
			"a %s's branches are its outcomes (`outcome '<value>'`)", op, ref, kind, kind)
	}
	return fmt.Errorf("%s: %q is a %s, which cannot hold %s", op, ref, kind, list)
}

// ParallelPathIndex resolves a parallel split path's address — "Path N", as
// describe numbers them from 1 in stored order — to its index in a split
// holding count paths. It refuses anything that names no single path rather
// than choosing one.
func ParallelPathIndex(caption, ref string, count int) (int, error) {
	if strings.TrimSpace(caption) == "" {
		return 0, fmt.Errorf("drop path: an empty caption names no path on parallel split %q — "+
			"write `drop <split> path <n>`, numbered from 1 as `describe workflow` prints them", ref)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(caption, "Path "))
	if err != nil || !strings.HasPrefix(caption, "Path ") {
		return 0, fmt.Errorf("drop path: %q names no path on parallel split %q — a path is addressed by its number, "+
			"`drop <split> path <n>`, numbered from 1 as `describe workflow` prints them", caption, ref)
	}
	if n < 1 || n > count {
		return 0, fmt.Errorf("drop path: parallel split %q has %d paths, so there is no path %d", ref, count, n)
	}
	return n - 1, nil
}

// BoundaryEventIndex is the 0-based position of `boundary event N` among an
// activity's count boundary events (ako/mxcli#808). index 0 — no number written
// — is the first, the old behaviour the executor gates (MDL-V1-BOUNDARYDROP);
// a number past the last is refused rather than clamped.
func BoundaryEventIndex(ref string, index, count int) (int, error) {
	if index == 0 {
		return 0, nil
	}
	if index < 1 || index > count {
		return 0, fmt.Errorf("activity %q has %d boundary event(s); `boundary event %d` names none of them", ref, count, index)
	}
	return index - 1, nil
}
