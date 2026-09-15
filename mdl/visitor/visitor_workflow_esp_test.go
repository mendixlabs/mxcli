// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// Event sub-processes after the main body, both trigger kinds, with and without
// the optional names and captions.
func TestWorkflowVisitor_EventSubProcesses(t *testing.T) {
	stmt := buildWorkflowStmt(t, `create workflow M.W
begin
  user task review 'Review' page M.P outcomes 'Good' { } 'Bad' { };
  event subprocess ESP_Cancel 'Cancel request' on interrupting notification cancelStart 'Cancel received' {
    call microflow M.Log;
  };
  event subprocess ESP_Audit on non interrupting notification {
  };
  event subprocess ESP_Expire 'Expire' on interrupting timer 'addDays([%CurrentDateTime%], 30)' as expireStart comment 'After 30 days' {
    end workflow;
  };
  event subprocess ESP_Remind on non interrupting timer 'addDays([%CurrentDateTime%], 1)' {
  };
end workflow;`)
	if len(stmt.Activities) != 1 {
		t.Fatalf("main body has %d activities, want 1 — a sub-process leaked into it", len(stmt.Activities))
	}
	if len(stmt.EventSubProcesses) != 4 {
		t.Fatalf("got %d event sub-processes, want 4", len(stmt.EventSubProcesses))
	}
	strip := func(n ast.WorkflowEventSubProcessNode) ast.WorkflowEventSubProcessNode {
		n.Activities = nil
		return n
	}
	want := []ast.WorkflowEventSubProcessNode{
		{Name: "ESP_Cancel", Caption: "Cancel request", Interrupting: true, StartName: "cancelStart", StartCaption: "Cancel received"},
		{Name: "ESP_Audit"},
		{Name: "ESP_Expire", Caption: "Expire", Interrupting: true, Timer: true, StartName: "expireStart", StartCaption: "After 30 days", FirstExecutionTime: "addDays([%CurrentDateTime%], 30)"},
		{Name: "ESP_Remind", Timer: true, FirstExecutionTime: "addDays([%CurrentDateTime%], 1)"},
	}
	for i, w := range want {
		if got := strip(stmt.EventSubProcesses[i]); !reflect.DeepEqual(got, w) {
			t.Errorf("sub-process %d = %+v, want %+v", i, got, w)
		}
	}
	if _, ok := stmt.EventSubProcesses[0].Activities[0].(*ast.WorkflowCallMicroflowNode); !ok {
		t.Errorf("ESP_Cancel body = %T, want the call microflow", stmt.EventSubProcesses[0].Activities[0])
	}
	if _, ok := stmt.EventSubProcesses[2].Activities[0].(*ast.WorkflowEndNode); !ok {
		t.Errorf("ESP_Expire body = %T, want `end workflow`", stmt.EventSubProcesses[2].Activities[0])
	}
}

// A notification activity and notification boundary events: the string after a
// notification event's name is its caption, while a timer's is still its delay.
func TestWorkflowVisitor_NotificationEvents(t *testing.T) {
	stmt := buildWorkflowStmt(t, `create workflow M.W
begin
  notification received comment 'Documents received';
  notification;
  user task review 'Review' page M.P outcomes 'Good' { } 'Bad' { }
    boundary event interrupting notification withdrawn 'Withdrawn' { end workflow; }
    boundary event non interrupting notification nudge
    boundary event non interrupting timer 'addHours([%CurrentDateTime%], 1)';
end workflow;`)
	if got, ok := stmt.Activities[0].(*ast.WorkflowNotificationNode); !ok || *got != (ast.WorkflowNotificationNode{Name: "received", Caption: "Documents received"}) {
		t.Errorf("first activity = %#v", stmt.Activities[0])
	}
	if got, ok := stmt.Activities[1].(*ast.WorkflowNotificationNode); !ok || *got != (ast.WorkflowNotificationNode{}) {
		t.Errorf("bare notification = %#v", stmt.Activities[1])
	}
	task := stmt.Activities[2].(*ast.WorkflowUserTaskNode)
	if len(task.BoundaryEvents) != 3 {
		t.Fatalf("got %d boundary events, want 3", len(task.BoundaryEvents))
	}
	check := func(i int, eventType, name, caption, delay string) {
		t.Helper()
		be := task.BoundaryEvents[i]
		if be.EventType != eventType || be.Name != name || be.Caption != caption || be.Delay != delay {
			t.Errorf("boundary event %d = %s/%q/%q/%q, want %s/%q/%q/%q", i, be.EventType, be.Name, be.Caption, be.Delay, eventType, name, caption, delay)
		}
	}
	check(0, "InterruptingNotification", "withdrawn", "Withdrawn", "")
	check(1, "NonInterruptingNotification", "nudge", "", "")
	check(2, "NonInterruptingTimer", "", "", "addHours([%CurrentDateTime%], 1)")
	if len(task.BoundaryEvents[0].Activities) != 1 {
		t.Errorf("withdrawn body has %d activities, want 1", len(task.BoundaryEvents[0].Activities))
	}
}
