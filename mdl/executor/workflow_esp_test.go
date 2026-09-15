// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

const espWorkflow = `create workflow M.W parameter $C: M.Ctx begin
  notification received comment 'Documents received';
  wait for notification waitApproval
    boundary event interrupting notification withdrawn 'Withdrawn' { end workflow; }
    boundary event non interrupting notification nudge 'Nudge' { call microflow M.Log; };
  user task review 'Review' page M.P outcomes 'Good' { } 'Bad' { };
  event subprocess ESP_Cancel 'Cancel request' on interrupting notification cancelStart 'Cancel received' {
    call microflow M.Log;
  };
  event subprocess ESP_Loop on non interrupting notification loopStart {
    call microflow M.Log as logIt;
    jump to logIt;
  };
  event subprocess ESP_Expire 'Expire' on interrupting timer 'addDays([%CurrentDateTime%], 30)' as expireStart comment 'After 30 days' {
  };
end workflow;`

// Each sub-process flow starts with its start event, and gets an End only when
// its body does not already end: measured, a flow with no end is CE0105 and an
// End after a jump is CE6689.
func TestWorkflowESP_BuildsStudioProShapes(t *testing.T) {
	esps := buildEventSubProcesses(parseWorkflowStmt(t, espWorkflow).EventSubProcesses)
	if len(esps) != 3 {
		t.Fatalf("built %d sub-processes, want 3", len(esps))
	}
	want := []struct {
		storage, startName, startCaption string
		endsWithEnd                      bool
	}{
		{"Workflows$InterruptingNotificationEventSubProcessStartActivity", "cancelStart", "Cancel received", true},
		{"Workflows$NonInterruptingNotificationEventSubProcessStartActivity", "loopStart", "loopStart", false},
		{"Workflows$InterruptingTimerEventSubProcessStartActivity", "expireStart", "After 30 days", true},
	}
	for i, w := range want {
		start := esps[i].Start()
		if start == nil || start.StorageType() != w.storage || start.Name != w.startName || start.Caption != w.startCaption {
			t.Errorf("%s start = %+v, want %s %q %q", esps[i].Name, start, w.storage, w.startName, w.startCaption)
		}
		acts := esps[i].Flow.Activities
		_, isEnd := acts[len(acts)-1].(*workflows.EndWorkflowActivity)
		if isEnd != w.endsWithEnd {
			t.Errorf("%s ends with %T, want an End: %v", esps[i].Name, acts[len(acts)-1], w.endsWithEnd)
		}
	}
	if got := esps[2].Start().FirstExecutionTime; got != "addDays([%CurrentDateTime%], 30)" {
		t.Errorf("timer expression = %q", got)
	}

	unnamed := buildEventSubProcesses(parseWorkflowStmt(t, `create workflow M.W begin
  event subprocess ESP_X on interrupting notification { };
end workflow;`).EventSubProcesses)
	if s := unnamed[0].Start(); s.Name != "ESP_XStart" || s.Caption != "ESP_XStart" {
		t.Errorf("unnamed start = %q/%q, want ESP_XStart", s.Name, s.Caption)
	}
}

// A start event and a notification boundary event share the workflow's one name
// space with its activities — a clash is CE0495, measured.
func TestWorkflowESP_NamesAreUniqueAcrossFlows(t *testing.T) {
	stmt := parseWorkflowStmt(t, `create workflow M.W begin
  user task review 'Review' page M.P outcomes 'Good' { } 'Bad' { }
    boundary event interrupting notification review;
  event subprocess ESP on interrupting notification review { };
end workflow;`)
	acts := buildWorkflowActivities(stmt.Activities)
	esps := buildEventSubProcesses(stmt.EventSubProcesses)
	named := append([]workflows.WorkflowActivity{}, acts...)
	named = append(named, esps[0].Flow.Activities...)
	deduplicateActivityNames(named, "Start", "End")

	task := acts[0].(*workflows.UserTask)
	names := []string{task.Name, task.BoundaryEvents[0].Name, esps[0].Start().Name}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Fatalf("names %v are not unique", names)
		}
		seen[n] = true
	}
	if task.Name != "review" {
		t.Errorf("the user task, named first, must keep its name; got %q", task.Name)
	}
}

// Describe → re-parse → describe is stable for every new construct.
func TestWorkflowESP_DescribeRoundTrips(t *testing.T) {
	format := func(stmt *ast.CreateWorkflowStmt) string {
		lines := formatMainFlowActivities(&workflows.Flow{Activities: buildWorkflowActivities(stmt.Activities)}, "  ")
		lines = append(lines, formatEventSubProcesses(buildEventSubProcesses(stmt.EventSubProcesses), "  ")...)
		return strings.Join(lines, "\n")
	}
	first := format(parseWorkflowStmt(t, espWorkflow))
	for _, want := range []string{
		"notification received comment 'Documents received';",
		"boundary event interrupting notification withdrawn 'Withdrawn'",
		"boundary event non interrupting notification nudge 'Nudge'",
		"event subprocess ESP_Cancel 'Cancel request' on interrupting notification cancelStart 'Cancel received' {",
		"event subprocess ESP_Loop on non interrupting notification loopStart 'loopStart' {",
		"event subprocess ESP_Expire 'Expire' on interrupting timer 'addDays([%CurrentDateTime%], 30)' as expireStart comment 'After 30 days' {",
		"jump to logIt;",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("describe output lacks %q:\n%s", want, first)
		}
	}
	again := format(parseWorkflowStmt(t, "create workflow M.W parameter $C: M.Ctx begin\n"+first+"\nend workflow;"))
	if first != again {
		t.Errorf("describe is not stable:\n--- first\n%s\n--- again\n%s", first, again)
	}
}

// A jump stays in its own flow: into or out of an event sub-process is CE6682;
// within one, to an activity or its start event, builds.
func TestWorkflowESP_JumpScopes(t *testing.T) {
	stmt := parseWorkflowStmt(t, `create workflow M.W begin
  user task t 'T' page M.P outcomes 'Go' { jump to aLog; } 'Stop' { };
  event subprocess A on interrupting notification aStart { call microflow M.Log as aLog; jump to aLog; };
  event subprocess B on interrupting notification bStart { jump to aLog; };
  event subprocess C on interrupting notification cStart { call microflow M.Log as cLog; jump to cStart; };
end workflow;`)
	var msgs []string
	for _, v := range ValidateWorkflowJumpTargets(stmt) {
		msgs = append(msgs, v.Message)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d jump violations, want 2 (main → A, B → A):\n%s", len(msgs), strings.Join(msgs, "\n"))
	}
	for _, m := range msgs {
		if !strings.Contains(m, "CE6682") {
			t.Errorf("cross-flow jump reported as %q, want CE6682", m)
		}
	}
}

// MDL-WF14 (timer start with no expression, CE0126) and MDL-WF15 (two
// interrupting boundary events on one activity, CE6697), both measured.
func TestWorkflowESP_Rules(t *testing.T) {
	rules := func(src string) map[string]int {
		out := map[string]int{}
		for _, v := range ValidateWorkflow(parseWorkflowStmt(t, src)) {
			out[v.RuleID]++
		}
		return out
	}
	if got := rules(`create workflow M.W begin
  event subprocess T on interrupting timer '' { };
end workflow;`); got["MDL-WF14"] != 1 {
		t.Errorf("empty timer: rules %v, want one MDL-WF14", got)
	}
	if got := rules(`create workflow M.W begin
  user task t 'T' page M.P outcomes 'Go' { } 'Stop' { }
    boundary event interrupting notification a
    boundary event interrupting timer 'addDays([%CurrentDateTime%], 1)';
end workflow;`); got["MDL-WF15"] != 1 {
		t.Errorf("two interrupting events: rules %v, want one MDL-WF15", got)
	}
	if got := rules(espWorkflow); got["MDL-WF14"]+got["MDL-WF15"]+got["MDL-WF05"]+got["MDL-WF08"] != 0 {
		t.Errorf("the valid workflow was refused: %v", got)
	}
	// A non-interrupting notification path cannot end the workflow (CE1844), like
	// a non-interrupting timer's.
	if got := rules(`create workflow M.W begin
  user task t 'T' page M.P outcomes 'Go' { } 'Stop' { }
    boundary event non interrupting notification n { end workflow; };
end workflow;`); got["MDL-WF08"] != 1 {
		t.Errorf("end under a non-interrupting notification path: rules %v, want one MDL-WF08", got)
	}
}

const wfRewriteWithESP = `create or modify workflow M.W parameter $C: M.Ctx
begin
  user task review 'Review' page M.P outcomes 'Good' { } 'Bad' { }
    boundary event interrupting notification withdrawn;
  notification received;
  event subprocess ESP on interrupting notification espStart {
    user task nestedTask 'Nested' page M.P outcomes 'Good' { } 'Bad' { }
      boundary event non interrupting notification nudge;
  };
end workflow;`

// storedESPWorkflow is a stored workflow holding what wfRewriteWithESP states:
// a main flow closing with its End, a notification activity, a boundary event
// on the main flow and one inside the sub-process, whose flow closes with its
// own (implicit) End.
func storedESPWorkflow(start string) map[string]any {
	be := func(typ string) map[string]any {
		return map[string]any{"$Type": typ, "Flow": map[string]any{"$Type": "Workflows$Flow", "Activities": []any{3}}}
	}
	return map[string]any{
		"$Type": "Workflows$Workflow",
		"Flow": map[string]any{"$Type": "Workflows$Flow", "Activities": []any{3,
			map[string]any{"$Type": "Workflows$StartWorkflowActivity"},
			map[string]any{"$Type": "Workflows$SingleUserTaskActivity", "Name": "review",
				"BoundaryEvents": []any{2, be("Workflows$InterruptingNotificationBoundaryEvent")}},
			map[string]any{"$Type": "Workflows$NotificationActivity", "Name": "received"},
			map[string]any{"$Type": "Workflows$EndWorkflowActivity"},
		}},
		"EventSubProcesses": []any{2, map[string]any{
			"$Type": "Workflows$EventSubProcess",
			"Flow": map[string]any{"$Type": "Workflows$Flow", "Activities": []any{3,
				map[string]any{"$Type": start, "Name": "espStart"},
				map[string]any{"$Type": "Workflows$SingleUserTaskActivity", "Name": "inner",
					"BoundaryEvents": []any{2, be("Workflows$NonInterruptingNotificationBoundaryEvent")}},
				map[string]any{"$Type": "Workflows$EndWorkflowActivity"},
			}},
		}},
	}
}

// A statement restating everything is allowed: the sub-process's closing End is
// not a stored `end workflow` the statement omits, and the boundary event inside
// the sub-process is counted on both sides.
func TestWorkflowRewrite_RestatedEventSubProcessesPass(t *testing.T) {
	raw := storedESPWorkflow("Workflows$InterruptingNotificationEventSubProcessStartActivity")
	if err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, raw), "wf1", "M.W", parseWorkflowStmt(t, wfRewriteWithESP)); err != nil {
		t.Errorf("a statement restating every construct was refused: %v", err)
	}
}

// Each construct that is not restated is refused, and a sub-process with no
// start event — which MDL cannot state — is refused even when restated.
func TestWorkflowRewrite_EventSubProcessesMustBeRestated(t *testing.T) {
	raw := storedESPWorkflow("Workflows$InterruptingNotificationEventSubProcessStartActivity")
	cases := []struct{ name, src, want string }{
		{"sub-process dropped", strings.Replace(wfRewriteWithESP,
			"  event subprocess ESP on interrupting notification espStart {\n    user task nestedTask 'Nested' page M.P outcomes 'Good' { } 'Bad' { }\n      boundary event non interrupting notification nudge;\n  };\n", "", 1),
			"stored event sub-process"},
		{"notification activity dropped", strings.Replace(wfRewriteWithESP, "  notification received;\n", "", 1), "notification activit"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, raw), "wf1", "M.W", parseWorkflowStmt(t, c.src))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected a refusal naming %q, got %v", c.want, err)
			}
		})
	}

	startless := storedESPWorkflow("Workflows$UserTaskActivity")
	err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, startless), "wf1", "M.W", parseWorkflowStmt(t, wfRewriteWithESP))
	if err == nil || !strings.Contains(err.Error(), "no start event") {
		t.Errorf("a sub-process with no start event must be refused, got %v", err)
	}
}
