// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Timer boundary events over MCP, measured against Studio Pro 11.14: neither
// timer constructor takes firstExecutionTime (every event reported "Missing value
// for parameter 'Timer'"), the interrupting one normalizes its path's terminator,
// a single user task's constructor drops its boundary events, and an `add` to a
// boundaryEvents list does not keep the order the events were added in.

func timerPath(acts ...workflows.WorkflowActivity) *workflows.Flow {
	return &workflows.Flow{Activities: acts}
}

func timerJump(name, target string) *workflows.JumpToActivity {
	return &workflows.JumpToActivity{BaseWorkflowActivity: wfBase(name, name), TargetActivity: target}
}

func timerEnd(name string) *workflows.EndWorkflowActivity {
	return &workflows.EndWorkflowActivity{BaseWorkflowActivity: wfBase(name, "End")}
}

func timerMarker(name string) *workflows.EndOfBoundaryEventPathActivity {
	return &workflows.EndOfBoundaryEventPathActivity{BaseWorkflowActivity: wfBase(name, "")}
}

func waitWith(name string, events ...*workflows.BoundaryEvent) *workflows.WaitForNotificationActivity {
	return &workflows.WaitForNotificationActivity{BaseWorkflowActivity: wfBase(name, name), BoundaryEvents: events}
}

func inSplit(acts ...workflows.WorkflowActivity) *workflows.ParallelSplitActivity {
	return &workflows.ParallelSplitActivity{BaseWorkflowActivity: wfBase("split", "split"),
		Outcomes: []*workflows.ParallelSplitOutcome{{Flow: timerPath(acts...)}}}
}

// boundaryEventStore is the part of a fake PED that holds boundaryEvents lists.
// An `add` to one PREPENDS a new element with a fresh persistentId — the order
// Studio Pro 11.14 produced for three index-less adds (I, NI-A, NI-B stored as
// NI-B, NI-A, I) — so code that assumes an added event lands at the end, or in
// the order it was added, addresses the wrong event.
type boundaryEventStore struct {
	lists map[string][]map[string]any
	next  int
}

func newBoundaryEventStore(initial map[string][]map[string]any) *boundaryEventStore {
	s := &boundaryEventStore{lists: map[string][]map[string]any{}}
	for p, l := range initial {
		s.lists[p] = l
	}
	return s
}

// handle answers a boundaryEvents read or records a boundaryEvents add; ok is
// false for anything else.
func (s *boundaryEventStore) handle(name string, args map[string]any) (string, bool) {
	switch name {
	case "ped_read_document":
		p, _ := args["paths"].([]any)[0].(string)
		if !strings.HasSuffix(p, "/boundaryEvents") {
			return "", false
		}
		raw, _ := json.Marshal(s.lists[p])
		if s.lists[p] == nil {
			raw = []byte("[]")
		}
		return fmt.Sprintf(`{"results":[{"path":%q,"result":%s}]}`, p, raw), true
	case "ped_update_document":
		for _, o := range args["operations"].([]any) {
			entry := o.(map[string]any)
			p := entry["path"].(string)
			op := entry["operation"].(map[string]any)
			if op["type"] != "add" || !strings.HasSuffix(p, "/boundaryEvents") {
				continue
			}
			s.next++
			el := map[string]any{"$Type": op["value"].(map[string]any)["$Type"], "persistentId": fmt.Sprintf("pid-%d", s.next)}
			s.lists[p] = append([]map[string]any{el}, s.lists[p]...)
		}
	}
	return "", false
}

// Which path endings are sent, and which are refused before sending. Outside a
// split an interrupting timer may end in an End or a jump (the jump is restored
// after the write); the marker gets an End appended and is refused. Inside a split
// only a jump survives. A non-interrupting timer path must run to the marker.
func TestMapWorkflowContent_TimerPathEndings(t *testing.T) {
	timer := func(kind string, flow *workflows.Flow) *workflows.BoundaryEvent {
		return &workflows.BoundaryEvent{EventType: kind, TimerDelay: "addDays([%CurrentDateTime%], 1)", Flow: flow}
	}
	cases := []struct {
		name   string
		be     *workflows.BoundaryEvent
		split  bool
		refuse string // substring of the refusal; "" when sent
	}{
		{"interrupting ending in end workflow", timer("InterruptingTimer", timerPath(timerEnd("e"))), false, ""},
		{"interrupting ending in a jump", timer("InterruptingTimer", timerPath(timerJump("j", "w"))), false, ""},
		{"interrupting running to its end", timer("InterruptingTimer", timerPath(timerMarker("m"))), false, "`end workflow;` or `jump to <activity>`"},
		{"interrupting in a split ending in a jump", timer("InterruptingTimer", timerPath(timerJump("j", "w"))), true, ""},
		{"interrupting in a split running to its end", timer("InterruptingTimer", timerPath(timerMarker("m"))), true, "`jump to <activity>`"},
		{"interrupting in a split ending in end workflow", timer("InterruptingTimer", timerPath(timerEnd("e"))), true, "`jump to <activity>`"},
		{"non-interrupting running to its end", timer("NonInterruptingTimer", timerPath(timerMarker("m"))), false, ""},
		{"non-interrupting ending in a jump", timer("NonInterruptingTimer", timerPath(timerJump("j", "w"))), false, "remove the final `jump to`"},
		{"non-interrupting in a split ending in a jump", timer("NonInterruptingTimer", timerPath(timerJump("j", "w"))), true, "remove the final `jump to`"},
	}
	for _, c := range cases {
		var act workflows.WorkflowActivity = waitWith("w", c.be)
		if c.split {
			act = inSplit(act)
		}
		content, err := (&Backend{}).mapWorkflowContent(&workflows.Workflow{Name: "W", Flow: timerPath(act)}, true)
		switch {
		case c.refuse == "" && err != nil:
			t.Errorf("%s: refused: %v", c.name, err)
		case c.refuse != "" && err == nil:
			t.Errorf("%s: sent, want a refusal naming %s", c.name, c.refuse)
		case c.refuse != "" && !strings.Contains(err.Error(), c.refuse):
			t.Errorf("%s: refusal %q does not name %s", c.name, err, c.refuse)
		case c.refuse != "" && !strings.Contains(err.Error(), "timer boundary event on w"):
			t.Errorf("%s: refusal %q does not name the activity the event is on", c.name, err)
		}
		if err != nil {
			continue
		}
		wait := content["flow"].(map[string]any)["activities"].([]any)[0].(map[string]any)
		if c.split {
			wait = wait["outcomes"].([]any)[0].(map[string]any)["flow"].(map[string]any)["activities"].([]any)[0].(map[string]any)
		}
		el := wait["boundaryEvents"].([]any)[0].(map[string]any)
		if _, has := el["firstExecutionTime"]; has {
			t.Errorf("%s: firstExecutionTime is not a constructor property and must be set afterwards: %v", c.name, el)
		}
		flag, has := el["isInsideOfParallelSplit"]
		if interrupting := !c.be.IsNonInterrupting(); interrupting != has || (has && flag != c.split) {
			t.Errorf("%s: isInsideOfParallelSplit = %v (present %v), want %v on an interrupting event only", c.name, flag, has, c.split)
		}
	}
}

// recordedOps flattens every ped_update_document the fake received into one line
// per op, in order, each call's ops preceded by a "--" separator.
func recordedOps(t *testing.T, f *fakePED) []string {
	t.Helper()
	var out []string
	for _, c := range f.calls {
		if c.Name != "ped_update_document" {
			continue
		}
		out = append(out, "--")
		for _, o := range c.Args["operations"].([]any) {
			entry := o.(map[string]any)
			op := entry["operation"].(map[string]any)
			line := fmt.Sprintf("%s %s", op["type"], entry["path"])
			if idx, ok := op["index"]; ok {
				line += fmt.Sprintf(" @%v", idx)
			}
			if v, ok := op["value"]; ok {
				raw, _ := json.Marshal(v)
				line += " " + string(raw)
			}
			out = append(out, line)
		}
	}
	return out
}

func diffOps(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ops:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// Every timer event's delay is set after the write, at any depth, and an
// interrupting timer's final jump outside a split — which the constructor replaced
// with an End — is put back: the End removed, the jump added. Inside a split the
// jump is kept by the constructor and left alone. Measured live: both the set and
// the remove+add are stored, and the document reports "No errors found."
func TestApplyTimerBoundaryEvents(t *testing.T) {
	delay := func(n int) string { return fmt.Sprintf("addDays([%%CurrentDateTime%%], %d)", n) }
	wf := &workflows.Workflow{
		Name: "W",
		Flow: timerPath(
			&workflows.StartWorkflowActivity{BaseWorkflowActivity: wfBase("Start", "Start")},
			waitWith("back", &workflows.BoundaryEvent{EventType: "InterruptingTimer", TimerDelay: delay(1),
				Flow: timerPath(waitWith("inner", &workflows.BoundaryEvent{EventType: "NonInterruptingTimer", TimerDelay: delay(2), Flow: timerPath(timerMarker("m1"))}),
					timerJump("jumpBack", "back"))}),
			waitWith("ended", &workflows.BoundaryEvent{EventType: "InterruptingTimer", TimerDelay: delay(3), Flow: timerPath(timerEnd("e1"))}),
			inSplit(waitWith("splitWait", &workflows.BoundaryEvent{EventType: "InterruptingTimer", TimerDelay: delay(4), Flow: timerPath(timerJump("jumpSplit", "splitWait"))})),
			waitWith("notified", &workflows.BoundaryEvent{EventType: "InterruptingNotification", Name: "n", Caption: "n", Flow: timerPath(timerEnd("e2"))}),
		),
		EventSubProcesses: []*workflows.EventSubProcess{{Name: "ESP", Flow: timerPath(
			&workflows.EventSubProcessStartActivity{BaseWorkflowActivity: wfBase("s", "s")},
			waitWith("espWait", &workflows.BoundaryEvent{EventType: "NonInterruptingTimer", TimerDelay: delay(5), Flow: timerPath(timerMarker("m2"))}),
		)}},
	}
	f := newFakePED(t, func(string, map[string]any) (string, bool) { return "SUCCESS", false })
	b := &Backend{client: f.connectClient(t)}
	if err := b.applyTimerBoundaryEvents("M.W", wf, nil); err != nil {
		t.Fatal(err)
	}
	diffOps(t, recordedOps(t, f), []string{
		"--",
		`set /flow/activities/1/boundaryEvents/0/firstExecutionTime "addDays([%CurrentDateTime%], 1)"`,
		`remove /flow/activities/1/boundaryEvents/0/flow/activities @1`,
		`add /flow/activities/1/boundaryEvents/0/flow/activities {"$Type":"Workflows$JumpToActivity","caption":"jumpBack","isInsideOfParallelSplit":false,"name":"jumpBack","targetActivity":"back"}`,
		`set /flow/activities/1/boundaryEvents/0/flow/activities/0/boundaryEvents/0/firstExecutionTime "addDays([%CurrentDateTime%], 2)"`,
		`set /flow/activities/2/boundaryEvents/0/firstExecutionTime "addDays([%CurrentDateTime%], 3)"`,
		`set /flow/activities/3/outcomes/0/flow/activities/0/boundaryEvents/0/firstExecutionTime "addDays([%CurrentDateTime%], 4)"`,
		`set /eventSubProcesses/0/flow/activities/1/boundaryEvents/0/firstExecutionTime "addDays([%CurrentDateTime%], 5)"`,
	})

	// Nothing to finish, nothing sent.
	f2 := newFakePED(t, func(string, map[string]any) (string, bool) { return "SUCCESS", false })
	b2 := &Backend{client: f2.connectClient(t)}
	if err := b2.applyTimerBoundaryEvents("M.W", &workflows.Workflow{Name: "W", Flow: timerPath(waitWith("x"))}, nil); err != nil {
		t.Fatal(err)
	}
	if len(f2.calls) != 0 {
		t.Errorf("a workflow without timer events sent %v", f2.calls)
	}
}

// A rewrite re-adds a single user task's boundary events one at a time, reading
// where each one landed, and only then finishes the timers — at the stored
// positions, not the statement's. Measured live: the task's stored boundaryEvents
// is [] after the write, and the re-added [interrupting, non-interrupting] came
// back as [non-interrupting, interrupting], so finishing by statement index set
// each delay on the other event while ped_check_errors reported no errors.
func TestUpdateWorkflow_TimerOnSingleUserTaskIsReaddedThenFinished(t *testing.T) {
	store := newBoundaryEventStore(nil)
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		if text, ok := store.handle(name, args); ok {
			return text, false
		}
		switch name {
		case "ped_check_errors":
			return "No errors found.", false
		case "ped_read_document":
			p := args["paths"].([]any)[0].(string)
			if p == "/flow/activities" {
				return `{"results":[{"path":"/flow/activities","result":[
					{"$Type":"Workflows$StartWorkflowActivity"},{"$Type":"Workflows$EndWorkflowActivity"}]}]}`, false
			}
			return `{"results":[{"path":"` + p + `","result":[]}]}`, false
		}
		return "SUCCESS", false
	})
	mod := &model.Module{Name: "M"}
	mod.ID = "mod1"
	b := &Backend{client: f.connectClient(t), sessionModules: []*model.Module{mod}}
	task := &workflows.UserTask{BaseWorkflowActivity: wfBase("review", "Review"), Page: "M.TaskPage",
		BoundaryEvents: []*workflows.BoundaryEvent{
			{EventType: "InterruptingTimer", TimerDelay: "addDays([%CurrentDateTime%], 4)", Flow: timerPath(timerEnd("e"))},
			{EventType: "NonInterruptingTimer", TimerDelay: "addDays([%CurrentDateTime%], 5)", Flow: timerPath(timerMarker("m"))},
		}}
	wf := &workflows.Workflow{Name: "WF", Flow: timerPath(
		&workflows.StartWorkflowActivity{BaseWorkflowActivity: wfBase("Start", "Start")},
		task,
		&workflows.EndWorkflowActivity{BaseWorkflowActivity: wfBase("End", "End")},
	)}
	wf.ContainerID = "mod1"
	wf.ID = "wfid"
	if err := b.UpdateWorkflow(wf); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	var tail []string
	for _, line := range recordedOps(t, f) {
		if line == "--" || strings.Contains(line, "/boundaryEvents") {
			tail = append(tail, line)
		}
	}
	// The flow rewrite (first update) carries no boundary-event path of its own.
	for len(tail) > 1 && tail[0] == "--" && tail[1] == "--" {
		tail = tail[1:]
	}
	diffOps(t, tail, []string{
		"--",
		`add /flow/activities/1/boundaryEvents {"$Type":"Workflows$InterruptingTimerBoundaryEvent","caption":"","flow":{"$Type":"Workflows$Flow","activities":[{"$Type":"Workflows$EndWorkflowActivity","caption":"End","name":"e"}]},"isInsideOfParallelSplit":false}`,
		"--",
		`add /flow/activities/1/boundaryEvents {"$Type":"Workflows$NonInterruptingTimerBoundaryEvent","caption":"","flow":{"$Type":"Workflows$Flow","activities":[{"$Type":"Workflows$EndOfBoundaryEventPathActivity","caption":"","name":"m"}]},"recurrence":null}`,
		"--",
		// The store prepends, so the interrupting event (added first) now sits at 1.
		`set /flow/activities/1/boundaryEvents/1/firstExecutionTime "addDays([%CurrentDateTime%], 4)"`,
		`set /flow/activities/1/boundaryEvents/0/firstExecutionTime "addDays([%CurrentDateTime%], 5)"`,
	})
}

// ALTER WORKFLOW … INSERT BOUNDARY EVENT follows the same rules as a create: the
// event is added without its delay, found where it landed by its persistentId, and
// then finished there — its delay set, a final jump outside a split put back after
// the constructor swapped it for an End.
func TestWFInsertBoundaryEvent_InterruptingTimerJump(t *testing.T) {
	f, m := wfMutatorFake(t)
	jump := timerJump("jumpBack", "ReviewOrder")
	if err := m.InsertBoundaryEvent("ReviewOrder", 0, "InterruptingTimer", "addDays([%CurrentDateTime%], 3)", []workflows.WorkflowActivity{jump}); err != nil {
		t.Fatal(err)
	}
	// The activity already had one event; the store prepends, so the new one is at 0.
	diffOps(t, recordedOps(t, f), []string{
		"--",
		`add /flow/activities/0/boundaryEvents {"$Type":"Workflows$InterruptingTimerBoundaryEvent","caption":"","flow":{"$Type":"Workflows$Flow","activities":[{"$Type":"Workflows$JumpToActivity","caption":"jumpBack","name":"jumpBack","targetActivity":"ReviewOrder"}]},"isInsideOfParallelSplit":false}`,
		"--",
		`set /flow/activities/0/boundaryEvents/0/firstExecutionTime "addDays([%CurrentDateTime%], 3)"`,
		`remove /flow/activities/0/boundaryEvents/0/flow/activities @0`,
		`add /flow/activities/0/boundaryEvents/0/flow/activities {"$Type":"Workflows$JumpToActivity","caption":"jumpBack","isInsideOfParallelSplit":false,"name":"jumpBack","targetActivity":"ReviewOrder"}`,
	})
}

// An interrupting timer inserted with no path (`{ }`, which gets the end-of-path
// marker) is refused before anything is sent, naming the endings that work.
func TestWFInsertBoundaryEvent_InterruptingTimerRunningToItsEndIsRefused(t *testing.T) {
	f, m := wfMutatorFake(t)
	err := m.InsertBoundaryEvent("ReviewOrder", 0, "InterruptingTimer", "addDays([%CurrentDateTime%], 3)", nil)
	if err == nil || !strings.Contains(err.Error(), "`end workflow;` or `jump to <activity>`") || !strings.Contains(err.Error(), "on ReviewOrder") {
		t.Fatalf("want a refusal naming the endings and the activity, got %v", err)
	}
	if _, sent := f.callByName("ped_update_document"); sent {
		t.Error("a refused insert still sent an update")
	}
}

// Resolving the activity records whether it sits under a parallel split, so an
// inserted interrupting timer there says so and keeps its jump.
func TestWFInsertBoundaryEvent_InsideSplit(t *testing.T) {
	results := map[string]string{
		"/flow/activities":                              `[{"$Type":"Workflows$ParallelSplitActivity","name":"split"}]`,
		"/flow/activities/0/outcomes":                   `[{"$Type":"Workflows$ParallelSplitOutcome","flow":{"$Type":"Workflows$Flow"}}]`,
		"/flow/activities/0/outcomes/0/flow/activities": `[{"$Type":"Workflows$WaitForNotificationActivity","name":"splitWait"}]`,
	}
	store := newBoundaryEventStore(nil)
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		if text, ok := store.handle(name, args); ok {
			return text, false
		}
		if name != "ped_read_document" {
			return "SUCCESS", false
		}
		p := args["paths"].([]any)[0].(string)
		v, ok := results[p]
		if !ok {
			v = "null"
		}
		return fmt.Sprintf(`{"results":[{"path":%q,"result":%s}]}`, p, v), false
	})
	m := &mcpWorkflowMutator{backend: &Backend{client: f.connectClient(t)}, moduleName: "M", workflowName: "WF"}
	jump := timerJump("jumpSplit", "splitWait")
	if err := m.InsertBoundaryEvent("splitWait", 0, "InterruptingTimer", "addDays([%CurrentDateTime%], 4)", []workflows.WorkflowActivity{jump}); err != nil {
		t.Fatal(err)
	}
	const ev = "/flow/activities/0/outcomes/0/flow/activities/0/boundaryEvents"
	diffOps(t, recordedOps(t, f), []string{
		"--",
		`add ` + ev + ` {"$Type":"Workflows$InterruptingTimerBoundaryEvent","caption":"","flow":{"$Type":"Workflows$Flow","activities":[{"$Type":"Workflows$JumpToActivity","caption":"jumpSplit","name":"jumpSplit","targetActivity":"splitWait"}]},"isInsideOfParallelSplit":true}`,
		"--",
		`set ` + ev + `/0/firstExecutionTime "addDays([%CurrentDateTime%], 4)"`,
	})
}
