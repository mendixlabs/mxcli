// SPDX-License-Identifier: Apache-2.0

// Event sub-processes and notification events. Measured against mxbuild 11.13.0
// with Studio Pro-saved shapes (ako/TestApp), one construct per workflow,
// verdict = the literal `mx check` line:
//
//	notification or timer start, interrupting or not, body ending in End     0 errors
//	a body with activities, or ending in a jump within its sub-process       0 errors
//	a body with no end                                                       CE0105  (the builder adds an End)
//	a jump into another sub-process, or between one and the main flow        CE6682  MDL-WF05
//	a timer start with no first execution time                               CE0126  MDL-WF14
//	a start event named like another activity                                CE0495  (deduplicated)
//	the boundary-path end marker inside a sub-process                        CE6692  (not authorable)
//	a notification boundary event on a user task, multi-user task, call
//	  microflow, wait for notification or wait for timer                     0 errors
//	an interrupting notification path ending in the end-of-path marker       0 errors
//	an interrupting notification path with no end                            CE0105  (the builder adds the marker)
//	two interrupting boundary events on one activity                         CE6697  MDL-WF15
//	a notification activity in the main flow                                 0 errors
//
// Which mxbuild knows which type, measured by loading each construct alone into
// a blank project of that version (an unknown type is TypeCacheUnknownTypeException,
// and the project does not load):
//
//	                                          11.10   11.11   11.12   11.13
//	notification-started event sub-process    yes     yes     yes     yes     workflows.event_subprocesses (11.8, gen)
//	notification activity / boundary events   no      yes     yes     yes     workflows.notification_events
//	timer-started event sub-process           no      no      no      yes     workflows.timer_event_subprocesses
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// workflowStatementActivities returns every top-level block a statement writes:
// the main body, then each event sub-process body. Rules and guards that walk
// "the workflow's activities" must see both — a microflow called only from a
// sub-process is still a reference, and a boundary event there is still stored.
func workflowStatementActivities(stmt *ast.CreateWorkflowStmt) []ast.WorkflowActivityNode {
	if len(stmt.EventSubProcesses) == 0 {
		return stmt.Activities
	}
	out := append([]ast.WorkflowActivityNode{}, stmt.Activities...)
	for _, esp := range stmt.EventSubProcesses {
		out = append(out, esp.Activities...)
	}
	return out
}

// workflowUsesNotificationEvents reports whether activities hold a notification
// activity or a notification boundary event.
func workflowUsesNotificationEvents(activities []ast.WorkflowActivityNode) bool {
	found := false
	walkWorkflowActivities(activities, func(a ast.WorkflowActivityNode) {
		var events []ast.WorkflowBoundaryEventNode
		switch n := a.(type) {
		case *ast.WorkflowNotificationNode:
			found = true
		case *ast.WorkflowUserTaskNode:
			events = n.BoundaryEvents
		case *ast.WorkflowCallMicroflowNode:
			events = n.BoundaryEvents
		case *ast.WorkflowWaitForNotificationNode:
			events = n.BoundaryEvents
		}
		for _, e := range events {
			if e.EventType == "InterruptingNotification" || e.EventType == "NonInterruptingNotification" {
				found = true
			}
		}
	})
	return found
}

// checkEventSubProcessFeatures refuses what the project's Mendix version cannot
// load (see the table above).
func checkEventSubProcessFeatures(ctx *ExecContext, stmt *ast.CreateWorkflowStmt) error {
	if len(stmt.EventSubProcesses) > 0 {
		if err := checkFeature(ctx, "workflows", "event_subprocesses", "event subprocess",
			"event sub-processes need Mendix 11.8 or later"); err != nil {
			return err
		}
	}
	for _, esp := range stmt.EventSubProcesses {
		if esp.Timer {
			if err := checkFeature(ctx, "workflows", "timer_event_subprocesses", "event subprocess … on … timer",
				"a timer-started event sub-process needs Mendix 11.13 or later — start it with a notification on older projects"); err != nil {
				return err
			}
			break
		}
	}
	if workflowUsesNotificationEvents(workflowStatementActivities(stmt)) {
		if err := checkFeature(ctx, "workflows", "notification_events", "notification activities and notification boundary events",
			"these need Mendix 11.11 or later — use `wait for notification` on older projects"); err != nil {
			return err
		}
	}
	return nil
}

// ValidateWorkflowEventSubProcesses applies MDL-WF14 and MDL-WF15.
func ValidateWorkflowEventSubProcesses(stmt *ast.CreateWorkflowStmt) []linter.Violation {
	loc := workflowLocation(stmt.Name)
	var out []linter.Violation
	for _, esp := range stmt.EventSubProcesses {
		if esp.Timer && strings.TrimSpace(esp.FirstExecutionTime) == "" {
			out = append(out, linter.Violation{
				RuleID:     "MDL-WF14",
				Severity:   linter.SeverityError,
				Location:   loc,
				Message:    fmt.Sprintf("event subprocess %s starts on a timer with no first execution time — the build fails CE0126", esp.Name),
				Suggestion: "Give the timer an expression, e.g. `on interrupting timer 'addDays([%CurrentDateTime%], 1)'`.",
			})
		}
	}
	walkWorkflowActivities(workflowStatementActivities(stmt), func(a ast.WorkflowActivityNode) {
		var events []ast.WorkflowBoundaryEventNode
		var label string
		switch n := a.(type) {
		case *ast.WorkflowUserTaskNode:
			events, label = n.BoundaryEvents, "user task "+workflowUserTaskLabel(n)
		case *ast.WorkflowCallMicroflowNode:
			events, label = n.BoundaryEvents, "call microflow "+workflowCallMicroflowLabel(n)
		case *ast.WorkflowWaitForNotificationNode:
			name := n.Name
			if name == "" {
				name = n.Caption
			}
			events, label = n.BoundaryEvents, fmt.Sprintf("wait for notification '%s'", name)
		}
		interrupting := 0
		for _, e := range events {
			if e.EventType == "InterruptingTimer" || e.EventType == "InterruptingNotification" {
				interrupting++
			}
		}
		if interrupting > 1 {
			out = append(out, linter.Violation{
				RuleID:     "MDL-WF15",
				Severity:   linter.SeverityError,
				Location:   loc,
				Message:    fmt.Sprintf("%s has %d interrupting boundary events — Mendix allows one per activity; the build fails CE6697", label, interrupting),
				Suggestion: "Keep one interrupting boundary event and make the others `non interrupting`.",
			})
		}
	})
	return out
}
