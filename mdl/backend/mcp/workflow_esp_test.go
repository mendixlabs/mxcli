// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

func wfBase(name, caption string) workflows.BaseWorkflowActivity {
	return workflows.BaseWorkflowActivity{Name: name, Caption: caption}
}

// The constructor content carries eventSubProcesses (ped_get_schema, Studio Pro
// 11.14) with each start event first, the notification activity, and each
// interrupting notification boundary event's isInsideOfParallelSplit — true only
// under a split.
func TestMapWorkflowContent_EventSubProcessesAndNotificationEvents(t *testing.T) {
	interrupting := func(name string) *workflows.BoundaryEvent {
		end := &workflows.EndWorkflowActivity{BaseWorkflowActivity: wfBase(name+"End", "End")}
		return &workflows.BoundaryEvent{EventType: "InterruptingNotification", Name: name, Caption: name,
			Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{end}}}
	}
	marker := func(name string) *workflows.Flow {
		return &workflows.Flow{Activities: []workflows.WorkflowActivity{&workflows.EndOfBoundaryEventPathActivity{BaseWorkflowActivity: wfBase(name, "")}}}
	}
	wf := &workflows.Workflow{
		Name: "W",
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.NotificationActivity{BaseWorkflowActivity: wfBase("received", "Documents received")},
			&workflows.WaitForNotificationActivity{BaseWorkflowActivity: wfBase("top", "top"),
				BoundaryEvents: []*workflows.BoundaryEvent{interrupting("topEvent"),
					{EventType: "NonInterruptingNotification", Name: "nudge", Caption: "Nudge", Flow: marker("nudgeEnd")}}},
			&workflows.ParallelSplitActivity{BaseWorkflowActivity: wfBase("split", "split"),
				Outcomes: []*workflows.ParallelSplitOutcome{{Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
					&workflows.WaitForNotificationActivity{BaseWorkflowActivity: wfBase("inSplit", "inSplit"),
						BoundaryEvents: []*workflows.BoundaryEvent{splitEventEndingInJump()}},
				}}}}},
		}},
		EventSubProcesses: []*workflows.EventSubProcess{{Name: "ESP_Expire", Caption: "Expire", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.EventSubProcessStartActivity{BaseWorkflowActivity: wfBase("expireStart", "After 30 days"), Interrupting: true, Timer: true, FirstExecutionTime: "addDays([%CurrentDateTime%], 30)"},
			&workflows.EndWorkflowActivity{BaseWorkflowActivity: wfBase("End", "End")},
		}}}},
	}
	content, err := (&Backend{}).mapWorkflowContent(wf, true)
	if err != nil {
		t.Fatal(err)
	}

	esps, _ := content["eventSubProcesses"].([]any)
	if len(esps) != 1 {
		t.Fatalf("eventSubProcesses = %v", content["eventSubProcesses"])
	}
	esp := esps[0].(map[string]any)
	start := esp["flow"].(map[string]any)["activities"].([]any)[0].(map[string]any)
	if esp["$Type"] != "Workflows$EventSubProcess" || esp["name"] != "ESP_Expire" ||
		start["$Type"] != "Workflows$InterruptingTimerEventSubProcessStartActivity" ||
		start["firstExecutionTime"] != "addDays([%CurrentDateTime%], 30)" {
		t.Errorf("sub-process mapped as %v", esp)
	}

	acts := content["flow"].(map[string]any)["activities"].([]any)
	if n := acts[0].(map[string]any); n["$Type"] != "Workflows$NotificationActivity" || n["name"] != "received" {
		t.Errorf("notification activity mapped as %v", n)
	}
	events := acts[1].(map[string]any)["boundaryEvents"].([]any)
	top, nudge := events[0].(map[string]any), events[1].(map[string]any)
	if top["$Type"] != "Workflows$InterruptingNotificationBoundaryEvent" || top["name"] != "topEvent" || top["isInsideOfParallelSplit"] != false {
		t.Errorf("top-level interrupting event mapped as %v", top)
	}
	if _, has := nudge["isInsideOfParallelSplit"]; has || nudge["name"] != "nudge" {
		t.Errorf("a non-interrupting event takes no isInsideOfParallelSplit: %v", nudge)
	}
	split := acts[2].(map[string]any)["outcomes"].([]any)[0].(map[string]any)["flow"].(map[string]any)["activities"].([]any)[0].(map[string]any)
	if inner := split["boundaryEvents"].([]any)[0].(map[string]any); inner["isInsideOfParallelSplit"] != true {
		t.Errorf("an event under a split must say so: %v", inner)
	}
}

func splitEventEndingInJump() *workflows.BoundaryEvent {
	jump := &workflows.JumpToActivity{BaseWorkflowActivity: wfBase("jumpBack", "jumpBack"), TargetActivity: "inSplit"}
	return &workflows.BoundaryEvent{EventType: "InterruptingNotification", Name: "splitEvent", Caption: "splitEvent",
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{jump}}}
}

// Outside a split Studio Pro's constructor forces an interrupting notification
// path to end in an End (measured: a jump is replaced by one, the marker gets one
// appended and is refused), and a non-interrupting path must end in the marker.
// Each shape it would rewrite is refused before sending.
func TestMapWorkflowContent_NotificationPathsStudioProWouldRewrite(t *testing.T) {
	on := func(be *workflows.BoundaryEvent) *workflows.Workflow {
		return &workflows.Workflow{Name: "W", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.WaitForNotificationActivity{BaseWorkflowActivity: wfBase("w", "w"), BoundaryEvents: []*workflows.BoundaryEvent{be}},
		}}}
	}
	path := func(acts ...workflows.WorkflowActivity) *workflows.Flow { return &workflows.Flow{Activities: acts} }
	jump := &workflows.JumpToActivity{BaseWorkflowActivity: wfBase("j", "j"), TargetActivity: "w"}
	end := &workflows.EndWorkflowActivity{BaseWorkflowActivity: wfBase("e", "End")}
	mark := &workflows.EndOfBoundaryEventPathActivity{BaseWorkflowActivity: wfBase("m", "")}
	cases := []struct {
		name   string
		be     *workflows.BoundaryEvent
		refuse bool
	}{
		{"interrupting ending in end workflow", &workflows.BoundaryEvent{EventType: "InterruptingNotification", Name: "a", Flow: path(end)}, false},
		{"interrupting ending in the marker", &workflows.BoundaryEvent{EventType: "InterruptingNotification", Name: "a", Flow: path(mark)}, true},
		{"interrupting ending in a jump", &workflows.BoundaryEvent{EventType: "InterruptingNotification", Name: "a", Flow: path(jump)}, true},
		{"non-interrupting ending in the marker", &workflows.BoundaryEvent{EventType: "NonInterruptingNotification", Name: "a", Flow: path(mark)}, false},
		{"non-interrupting ending in a jump", &workflows.BoundaryEvent{EventType: "NonInterruptingNotification", Name: "a", Flow: path(jump)}, true},
	}
	for _, c := range cases {
		_, err := (&Backend{}).mapWorkflowContent(on(c.be), true)
		if (err != nil) != c.refuse {
			t.Errorf("%s: refused = %v (%v), want %v", c.name, err != nil, err, c.refuse)
		}
	}
}

// Inside a parallel split Studio Pro wants the interrupting notification path to
// end in a jump (measured: the marker form gets a target-less jump appended and is
// refused). Refused before sending, naming the remedy; the jump form passes.
func TestMapWorkflowContent_InterruptingNotificationInSplitNeedsJump(t *testing.T) {
	split := func(be *workflows.BoundaryEvent) *workflows.Workflow {
		return &workflows.Workflow{Name: "W", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.ParallelSplitActivity{BaseWorkflowActivity: wfBase("split", "split"),
				Outcomes: []*workflows.ParallelSplitOutcome{{Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
					&workflows.WaitForNotificationActivity{BaseWorkflowActivity: wfBase("inSplit", "inSplit"),
						BoundaryEvents: []*workflows.BoundaryEvent{be}},
				}}}}},
		}}}
	}
	marker := &workflows.BoundaryEvent{EventType: "InterruptingNotification", Name: "splitEvent", Caption: "splitEvent",
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{&workflows.EndOfBoundaryEventPathActivity{BaseWorkflowActivity: wfBase("end", "")}}}}
	if _, err := (&Backend{}).mapWorkflowContent(split(marker), true); err == nil || !strings.Contains(err.Error(), "jump to") {
		t.Errorf("a split path ending in the marker must be refused naming `jump to`, got %v", err)
	}
	if _, err := (&Backend{}).mapWorkflowContent(split(splitEventEndingInJump()), true); err != nil {
		t.Errorf("a split path ending in a jump was refused: %v", err)
	}
}

// applyUserTaskBoundaryEvents adds a single user task's boundary events — timer
// and notification, one add each — at any depth, when the constructor left the
// stored list empty, and reports where each landed; it adds nothing when the list
// was kept.
func TestApplyUserTaskBoundaryEvents(t *testing.T) {
	task := func(name string) *workflows.UserTask {
		return &workflows.UserTask{BaseWorkflowActivity: wfBase(name, name),
			BoundaryEvents: []*workflows.BoundaryEvent{
				{EventType: "InterruptingTimer", TimerDelay: "addDays([%CurrentDateTime%], 3)",
					Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{&workflows.EndWorkflowActivity{BaseWorkflowActivity: wfBase(name+"Timeout", "End")}}}},
				{EventType: "NonInterruptingNotification", Name: name + "Nudge", Caption: "Nudge",
					Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{&workflows.EndOfBoundaryEventPathActivity{BaseWorkflowActivity: wfBase(name+"End", "")}}}},
			}}
	}
	multi := task("multi")
	multi.IsMulti = true
	wf := &workflows.Workflow{
		Name: "W",
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.StartWorkflowActivity{BaseWorkflowActivity: wfBase("Start", "Start")},
			task("top"),
			multi,
		}},
		EventSubProcesses: []*workflows.EventSubProcess{{Name: "ESP", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.EventSubProcessStartActivity{BaseWorkflowActivity: wfBase("s", "s")},
			task("nested"),
		}}}},
	}
	const top, nested = "/flow/activities/1", "/eventSubProcesses/0/flow/activities/1"
	for _, storedAlready := range []bool{false, true} {
		var initial map[string][]map[string]any
		if storedAlready {
			kept := []map[string]any{{"$Type": "Workflows$InterruptingTimerBoundaryEvent", "persistentId": "kept"}}
			initial = map[string][]map[string]any{top + "/boundaryEvents": kept, nested + "/boundaryEvents": kept}
		}
		store := newBoundaryEventStore(initial)
		f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
			if text, ok := store.handle(name, args); ok {
				return text, false
			}
			return "SUCCESS", false
		})
		b := &Backend{client: f.connectClient(t)}
		order, err := b.applyUserTaskBoundaryEvents("M.W", wf)
		if err != nil {
			t.Fatal(err)
		}
		added := map[string][]string{}
		for _, c := range f.calls {
			if c.Name != "ped_update_document" {
				continue
			}
			ops := c.Args["operations"].([]any)
			if len(ops) != 1 {
				t.Errorf("events must be added one per update, got %d ops", len(ops))
			}
			for _, o := range ops {
				entry := o.(map[string]any)
				el := entry["operation"].(map[string]any)["value"].(map[string]any)
				added[entry["path"].(string)] = append(added[entry["path"].(string)], el["$Type"].(string))
			}
		}
		both := []string{"Workflows$InterruptingTimerBoundaryEvent", "Workflows$NonInterruptingNotificationBoundaryEvent"}
		want := map[string][]string{top + "/boundaryEvents": both, nested + "/boundaryEvents": both}
		// The store prepends, so the first event added ends up second.
		wantOrder := eventOrder{top: {1, 0}, nested: {1, 0}}
		if storedAlready {
			want, wantOrder = map[string][]string{}, eventOrder{}
		}
		if !reflect.DeepEqual(added, want) {
			t.Errorf("stored already %v: added %v, want %v (a multi-user task keeps its events)", storedAlready, added, want)
		}
		if !reflect.DeepEqual(order, wantOrder) {
			t.Errorf("stored already %v: order %v, want %v", storedAlready, order, wantOrder)
		}
	}
}

// An unknown boundary-event kind is an error, not an interrupting timer.
func TestBoundaryEventElement_UnknownKindIsRefused(t *testing.T) {
	if _, err := boundaryEventElement(&workflows.BoundaryEvent{EventType: "Escalation"}); err == nil {
		t.Error("an unknown kind was mapped")
	}
}

// A rewrite replaces the stored sub-processes with the statement's, as it does
// the event handlers.
func TestUpdateWorkflow_ReplacesEventSubProcesses(t *testing.T) {
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		switch name {
		case "ped_check_errors":
			return "No errors found.", false
		case "ped_read_document":
			paths, _ := args["paths"].([]any)
			if len(paths) == 1 && paths[0] == "/onWorkflowEvent" {
				return `{"results":[{"path":"/onWorkflowEvent","result":[]}]}`, false
			}
			if len(paths) == 1 && paths[0] == "/eventSubProcesses" {
				return `{"results":[{"path":"/eventSubProcesses","result":[{"$Type":"Workflows$EventSubProcess"}]}]}`, false
			}
			return `{"results":[{"path":"/flow/activities","result":[
				{"$Type":"Workflows$StartWorkflowActivity"},
				{"$Type":"Workflows$EndWorkflowActivity"}]}]}`, false
		}
		return "SUCCESS", false
	})
	mod := &model.Module{Name: "M"}
	mod.ID = "mod1"
	b := &Backend{client: f.connectClient(t), sessionModules: []*model.Module{mod}}

	wf := &workflows.Workflow{
		Name: "WF",
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.StartWorkflowActivity{BaseWorkflowActivity: wfBase("Start", "Start")},
			&workflows.EndWorkflowActivity{BaseWorkflowActivity: wfBase("End", "End")},
		}},
		EventSubProcesses: []*workflows.EventSubProcess{{Name: "ESP", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.EventSubProcessStartActivity{BaseWorkflowActivity: wfBase("espStart", "espStart"), Interrupting: true},
			&workflows.EndWorkflowActivity{BaseWorkflowActivity: wfBase("End2", "End")},
		}}}},
	}
	wf.ContainerID = "mod1"
	wf.ID = "wfid"
	if err := b.UpdateWorkflow(wf); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	var ops []any
	for _, c := range f.calls {
		if c.Name == "ped_update_document" {
			ops = append(ops, c.Args["operations"].([]any)...)
		}
	}
	if len(ops) == 0 {
		t.Fatal("no ped_update_document sent")
	}
	var adds, removes int
	for _, o := range ops {
		entry := o.(map[string]any)
		if path, _ := entry["path"].(string); !strings.HasPrefix(path, "/eventSubProcesses") {
			continue
		}
		switch entry["operation"].(map[string]any)["type"] {
		case "add":
			adds++
		case "remove":
			removes++
		}
	}
	if adds != 1 || removes != 1 {
		t.Errorf("expected the stored sub-process removed and the statement's added, got %d adds / %d removes", adds, removes)
	}
}
