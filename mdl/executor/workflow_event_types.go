// SPDX-License-Identifier: Apache-2.0

// The workflow event types a handler can subscribe to, per Mendix version.
//
// Studio Pro stores a handler's event types as an explicit list, even when every
// box is ticked (ako/TestApp, 11.14.0: a handler named "OnAnyEvent" stores all 42
// in the order below). So `on any workflow event` cannot be a flag — it has to be
// written as the list the project's version knows, and that list grows: Mendix
// added types for AI agent tasks and event sub-processes.
//
// The table is measured, not read off release notes: each point is the set of
// type names present in that mxbuild's Mendix.Modeler.*.dll. mxbuild is no help
// at check time — measured on 11.13.0, a handler listing an invented type builds
// at 0 errors — so a misspelt type would be written silently and dropped or
// refused later by Studio Pro. That is why names are validated here.
package executor

import "fmt"

// workflowEventTypeOrder is every known type, in Studio Pro's stored order.
var workflowEventTypeOrder = []string{
	"WorkflowCompleted", "WorkflowInitiated", "WorkflowRestarted", "WorkflowFailed", "WorkflowAborted",
	"WorkflowPaused", "WorkflowUnpaused", "WorkflowRetried", "WorkflowUpdated", "WorkflowUpgraded",
	"WorkflowConflicted", "WorkflowResolved", "WorkflowJumpToOptionApplied",
	"StartEventExecuted", "EndEventExecuted", "DecisionExecuted", "JumpExecuted",
	"ParallelSplitExecuted", "ParallelMergeExecuted",
	"CallWorkflowStarted", "CallWorkflowEnded", "CallMicroflowStarted", "CallMicroflowEnded",
	"AIAgentTaskStarted", "AIAgentTaskEnded",
	"NotificationStarted", "NotificationEnded",
	"WaitForNotificationStarted", "WaitForNotificationEnded", "WaitForTimerStarted", "WaitForTimerEnded",
	"UserTaskStarted", "MultiUserTaskOutcomeSelected", "UserTaskEnded",
	"NonInterruptingTimerEventExecuted", "InterruptingTimerEventExecuted",
	"NonInterruptingNotificationEventSubProcessStartExecuted", "InterruptingNotificationEventSubProcessStartExecuted",
	"NonInterruptingNotificationEventExecuted", "InterruptingNotificationEventExecuted",
	"NonInterruptingTimerEventSubProcessStartExecuted", "InterruptingTimerEventSubProcessStartExecuted",
}

// workflowEventTypeVersion is a Mendix version at which the type set was measured.
type workflowEventTypeVersion struct{ major, minor, patch int }

func (v workflowEventTypeVersion) atMost(major, minor, patch int) bool {
	if v.major != major {
		return v.major < major
	}
	if v.minor != minor {
		return v.minor < minor
	}
	return v.patch <= patch
}

func (v workflowEventTypeVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
}

// workflowEventTypePoints are the measured versions, oldest first, each with the
// types first seen there. A type is known from its point on.
var workflowEventTypePoints = []struct {
	version workflowEventTypeVersion
	added   []string
}{
	{workflowEventTypeVersion{11, 6, 0}, []string{
		"WorkflowCompleted", "WorkflowInitiated", "WorkflowRestarted", "WorkflowFailed", "WorkflowAborted",
		"WorkflowPaused", "WorkflowUnpaused", "WorkflowRetried", "WorkflowUpdated", "WorkflowUpgraded",
		"WorkflowConflicted", "WorkflowResolved", "WorkflowJumpToOptionApplied",
		"StartEventExecuted", "EndEventExecuted", "DecisionExecuted", "JumpExecuted",
		"ParallelSplitExecuted", "ParallelMergeExecuted",
		"CallWorkflowStarted", "CallWorkflowEnded", "CallMicroflowStarted", "CallMicroflowEnded",
		"WaitForNotificationStarted", "WaitForNotificationEnded", "WaitForTimerStarted", "WaitForTimerEnded",
		"UserTaskStarted", "MultiUserTaskOutcomeSelected", "UserTaskEnded",
		"NonInterruptingTimerEventExecuted", "InterruptingTimerEventExecuted",
	}},
	{workflowEventTypeVersion{11, 10, 0}, []string{
		"AIAgentTaskStarted", "AIAgentTaskEnded",
		"NonInterruptingNotificationEventSubProcessStartExecuted", "InterruptingNotificationEventSubProcessStartExecuted",
	}},
	{workflowEventTypeVersion{11, 13, 0}, []string{
		"NotificationStarted", "NotificationEnded",
		"NonInterruptingNotificationEventExecuted", "InterruptingNotificationEventExecuted",
		"NonInterruptingTimerEventSubProcessStartExecuted", "InterruptingTimerEventSubProcessStartExecuted",
	}},
}

// workflowEventTypeSince returns the index of the point where name was first
// measured, or -1 for a name that is not a workflow event type at all.
func workflowEventTypeSince(name string) int {
	for i, p := range workflowEventTypePoints {
		for _, n := range p.added {
			if n == name {
				return i
			}
		}
	}
	return -1
}

// canonicalWorkflowEventType returns the stored spelling of name, matched
// case-insensitively (MDL keywords are), and whether it is a known type.
func canonicalWorkflowEventType(name string) (string, bool) {
	for _, n := range workflowEventTypeOrder {
		if equalFoldASCII(n, name) {
			return n, true
		}
	}
	return "", false
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

// workflowEventTypeMissingIn reports whether a known type is measured ABSENT in
// the given version: the version is at or below the last point before the type
// appeared. Between that point and the one where it was seen, the answer is not
// known, and an explicitly named type is let through rather than refused.
func workflowEventTypeMissingIn(name string, major, minor, patch int) (lastAbsent string, missing bool) {
	since := workflowEventTypeSince(name)
	if since <= 0 {
		return "", false
	}
	prev := workflowEventTypePoints[since-1].version
	if !prev.atMost(major, minor, patch) {
		// Older than the last measured absence: absent too, types are not removed.
		return prev.String(), true
	}
	if prev == (workflowEventTypeVersion{major, minor, patch}) {
		return prev.String(), true
	}
	return "", false
}

// allWorkflowEventTypes returns every type known at the given version, in stored
// order: the set at the newest measured point not above it. ok is false when the
// version is older than every measured point — the set there is not known, and
// guessing would write types the version may not have.
func allWorkflowEventTypes(major, minor, patch int) (types []string, basis string, ok bool) {
	last := -1
	for i, p := range workflowEventTypePoints {
		if p.version.atMost(major, minor, patch) {
			last = i
		}
	}
	if last < 0 {
		return nil, "", false
	}
	for _, n := range workflowEventTypeOrder {
		if since := workflowEventTypeSince(n); since >= 0 && since <= last {
			types = append(types, n)
		}
	}
	return types, workflowEventTypePoints[last].version.String(), true
}

// sortWorkflowEventTypes returns the given stored spellings de-duplicated and in
// Studio Pro's stored order, so an explicit list writes what Studio Pro would.
func sortWorkflowEventTypes(names []string) []string {
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
	}
	out := make([]string, 0, len(seen))
	for _, n := range workflowEventTypeOrder {
		if seen[n] {
			out = append(out, n)
		}
	}
	return out
}
