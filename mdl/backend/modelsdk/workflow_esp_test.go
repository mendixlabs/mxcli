// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"os"
	"testing"

	genWf "github.com/mendixlabs/mxcli/modelsdk/gen/workflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// The workflow Studio Pro 11.14 saved for ako/TestApp's
// ZzMxcliExample_EventSubProcesses: one event sub-process per start kind, a
// notification activity, and both notification boundary events on a user task.
const eventSubProcessFixture = "testdata/TestApp.ZzMxcliExample_EventSubProcesses.bson"

type espWant struct {
	caption, startName, startCaption, firstExecutionTime string
	interrupting, timer                                  bool
}

// The reader decodes the constructs gen has no type for — the two timer starts,
// the notification activity, the notification boundary events — off the raw
// document, and none of them is lost or read as a generic activity.
func TestReadEventSubProcessesFromStudioProFixture(t *testing.T) {
	raw, err := os.ReadFile(eventSubProcessFixture)
	if err != nil {
		t.Fatal(err)
	}
	g := genWf.NewWorkflow()
	g.InitFromRaw(bson.Raw(raw))
	wf := workflowFromGen(g, "")

	want := map[string]espWant{
		"ESP_Cancel":        {"Cancel request", "espCancelStart", "Cancel received", "", true, false},
		"ESP_Audit":         {"Audit update", "espAuditStart", "Audit received", "", false, false},
		"ESP_Expire":        {"Expire", "espExpireStart", "After 30 days", "addDays([%CurrentDateTime%], 30)", true, true},
		"ESP_DailyReminder": {"Daily reminder", "espReminderStart", "Every day", "addDays([%CurrentDateTime%], 1)", false, true},
	}
	if len(wf.EventSubProcesses) != len(want) {
		t.Fatalf("read %d event sub-processes, want %d", len(wf.EventSubProcesses), len(want))
	}
	for _, esp := range wf.EventSubProcesses {
		w, ok := want[esp.Name]
		if !ok {
			t.Errorf("unexpected sub-process %q", esp.Name)
			continue
		}
		start := esp.Start()
		if start == nil {
			t.Errorf("%s: no start event read", esp.Name)
			continue
		}
		got := espWant{esp.Caption, start.Name, start.Caption, start.FirstExecutionTime, start.Interrupting, start.Timer}
		if got != w {
			t.Errorf("%s = %+v, want %+v", esp.Name, got, w)
		}
		if _, ok := esp.Flow.Activities[len(esp.Flow.Activities)-1].(*workflows.EndWorkflowActivity); !ok {
			t.Errorf("%s: flow does not close with its End", esp.Name)
		}
	}

	var notification *workflows.NotificationActivity
	var review *workflows.UserTask
	for _, a := range wf.Flow.Activities {
		switch v := a.(type) {
		case *workflows.NotificationActivity:
			notification = v
		case *workflows.UserTask:
			review = v
		case *workflows.GenericWorkflowActivity:
			t.Errorf("activity %q read as generic %s", v.Name, v.TypeString)
		}
	}
	if notification == nil || notification.Name != "notifyReceived" || notification.Caption != "Documents received" {
		t.Errorf("notification activity = %+v", notification)
	}
	if review == nil || len(review.BoundaryEvents) != 2 {
		t.Fatalf("review boundary events not read: %+v", review)
	}
	byName := map[string]*workflows.BoundaryEvent{}
	for _, be := range review.BoundaryEvents {
		byName[be.Name] = be
	}
	if be := byName["beNudge"]; be == nil || be.EventType != "NonInterruptingNotification" || be.Caption != "Nudge" || be.Flow == nil {
		t.Errorf("beNudge = %+v", be)
	}
	if be := byName["beWithdrawn"]; be == nil || be.EventType != "InterruptingNotification" || be.Caption != "Withdrawn" || be.Flow == nil {
		t.Errorf("beWithdrawn = %+v", be)
	} else if _, ok := be.Flow.Activities[len(be.Flow.Activities)-1].(*workflows.EndWorkflowActivity); !ok {
		t.Errorf("beWithdrawn flow does not end with its End: %+v", be.Flow.Activities)
	}
}

// Every construct survives a write and a fresh read, stored the way Studio Pro
// stores it: EventSubProcesses and BoundaryEvents as marker-2 lists, and a
// notification boundary event with the Name a notify action targets.
func TestCreateWorkflow_EventSubProcessesRoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	base := func(name, caption string) workflows.BaseWorkflowActivity {
		return workflows.BaseWorkflowActivity{Name: name, Caption: caption}
	}
	end := func(name string) *workflows.EndWorkflowActivity {
		return &workflows.EndWorkflowActivity{BaseWorkflowActivity: base(name, "End")}
	}
	wait := &workflows.WaitForNotificationActivity{
		BaseWorkflowActivity: base("waitApproval", "Wait for approval"),
		BoundaryEvents: []*workflows.BoundaryEvent{
			{EventType: "InterruptingNotification", Name: "withdrawn", Caption: "Withdrawn",
				Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{end("withdrawnEnd")}}},
			{EventType: "NonInterruptingNotification", Name: "nudge", Caption: "Nudge",
				Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{&workflows.EndOfBoundaryEventPathActivity{BaseWorkflowActivity: base("nudgeEnd", "")}}}},
		},
	}
	wf := &workflows.Workflow{
		ContainerID: mod.ID, Name: "ZzEventSubProcesses",
		Parameter: &workflows.WorkflowParameter{EntityRef: "MyFirstModule.Ctx"},
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.StartWorkflowActivity{BaseWorkflowActivity: base("Start", "Start")},
			&workflows.NotificationActivity{BaseWorkflowActivity: base("received", "Documents received")},
			wait,
			end("End"),
		}},
		EventSubProcesses: []*workflows.EventSubProcess{
			{Name: "ESP_Cancel", Caption: "Cancel request", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
				&workflows.EventSubProcessStartActivity{BaseWorkflowActivity: base("cancelStart", "Cancel received"), Interrupting: true},
				end("cancelEnd"),
			}}},
			{Name: "ESP_Remind", Caption: "Daily reminder", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
				&workflows.EventSubProcessStartActivity{BaseWorkflowActivity: base("remindStart", "Every day"), Timer: true, FirstExecutionTime: "addDays([%CurrentDateTime%], 1)"},
				end("remindEnd"),
			}}},
		},
	}
	if err := b.CreateWorkflow(wf); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	all, err := b2.ListWorkflows()
	if err != nil {
		t.Fatal(err)
	}
	var got *workflows.Workflow
	for _, w := range all {
		if w.Name == "ZzEventSubProcesses" {
			got = w
		}
	}
	if got == nil {
		t.Fatal("workflow not found after reconnect")
	}

	if len(got.EventSubProcesses) != 2 {
		t.Fatalf("read back %d event sub-processes, want 2", len(got.EventSubProcesses))
	}
	if s := got.EventSubProcesses[0].Start(); s == nil || !s.Interrupting || s.Timer || s.Name != "cancelStart" || s.Caption != "Cancel received" {
		t.Errorf("ESP_Cancel start = %+v", s)
	}
	if s := got.EventSubProcesses[1].Start(); s == nil || s.Interrupting || !s.Timer || s.FirstExecutionTime != "addDays([%CurrentDateTime%], 1)" {
		t.Errorf("ESP_Remind start = %+v", s)
	}
	if n, ok := got.Flow.Activities[1].(*workflows.NotificationActivity); !ok || n.Name != "received" || n.Caption != "Documents received" {
		t.Errorf("notification activity read back as %#v", got.Flow.Activities[1])
	}
	gotWait, ok := got.Flow.Activities[2].(*workflows.WaitForNotificationActivity)
	if !ok || len(gotWait.BoundaryEvents) != 2 {
		t.Fatalf("wait for notification read back as %#v", got.Flow.Activities[2])
	}
	for i, w := range wait.BoundaryEvents {
		be := gotWait.BoundaryEvents[i]
		if be.EventType != w.EventType || be.Name != w.Name || be.Caption != w.Caption || be.Flow == nil {
			t.Errorf("boundary event %d = %+v, want %s %q %q with its flow", i, be, w.EventType, w.Name, w.Caption)
		}
	}

	raw, err := b2.GetRawUnit(got.ID)
	if err != nil {
		t.Fatal(err)
	}
	esps, _ := raw["EventSubProcesses"].([]any)
	if len(esps) != 3 || fmt.Sprint(esps[0]) != "2" {
		t.Errorf("EventSubProcesses stored as %v, want a marker-2 list of two", esps)
	}
	for _, a := range raw["Flow"].(map[string]any)["Activities"].([]any) {
		m, ok := a.(map[string]any)
		if !ok || m["Name"] != "waitApproval" {
			continue
		}
		events, _ := m["BoundaryEvents"].([]any)
		if len(events) != 3 || fmt.Sprint(events[0]) != "2" {
			t.Fatalf("BoundaryEvents stored as %v, want a marker-2 list of two", events)
		}
		first := events[1].(map[string]any)
		if first["$Type"] != "Workflows$InterruptingNotificationBoundaryEvent" || first["Name"] != "withdrawn" {
			t.Errorf("first boundary event stored as %v", first)
		}
		if _, hasTimer := first["FirstExecutionTime"]; hasTimer {
			t.Errorf("a notification boundary event must not carry a timer: %v", first)
		}
	}
}
