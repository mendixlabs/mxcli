// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// A workflow event handler and an on-created microflow survive a write and a
// fresh read, in the shape ako/TestApp (Studio Pro 11.14.0) stores: a marker-2
// OnWorkflowEvent list, marker-1 EventTypes, and a MicroflowBasedEvent on the
// task. The writer used to hard-code NoEvent and an empty handler list.
func TestCreateWorkflow_HandlersAndOnCreatedRoundTrip(t *testing.T) {
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

	task := &workflows.UserTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Review", Caption: "Review"},
		Page:                 "MyFirstModule.ReviewPage",
		OnCreated:            "MyFirstModule.ACT_Assign",
		Outcomes:             []*workflows.UserTaskOutcome{{Value: "Done"}},
	}
	plain := &workflows.UserTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Plain", Caption: "Plain"},
		Outcomes:             []*workflows.UserTaskOutcome{{Value: "Done"}},
	}
	wf := &workflows.Workflow{
		ContainerID: mod.ID,
		Name:        "ZzHandlers",
		Parameter:   &workflows.WorkflowParameter{EntityRef: "MyFirstModule.Ctx"},
		EventHandlers: []*workflows.WorkflowEventHandler{{
			Description: "Task audit",
			EventTypes:  []string{"UserTaskStarted", "UserTaskEnded"},
			Microflow:   "MyFirstModule.ACT_Audit",
		}},
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.StartWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Start"}},
			task,
			plain,
			&workflows.EndWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "End"}},
		}},
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
		t.Fatalf("ListWorkflows: %v", err)
	}
	var got *workflows.Workflow
	for _, w := range all {
		if w.Name == "ZzHandlers" {
			got = w
		}
	}
	if got == nil {
		t.Fatal("workflow not found after reconnect")
	}

	if len(got.EventHandlers) != 1 {
		t.Fatalf("handlers = %d, want 1", len(got.EventHandlers))
	}
	h := got.EventHandlers[0]
	if h.Description != "Task audit" || h.Microflow != "MyFirstModule.ACT_Audit" ||
		!reflect.DeepEqual(h.EventTypes, []string{"UserTaskStarted", "UserTaskEnded"}) {
		t.Errorf("handler = %+v", h)
	}
	onCreated := map[string]string{}
	for _, a := range got.Flow.Activities {
		if ut, ok := a.(*workflows.UserTask); ok {
			onCreated[ut.Name] = ut.OnCreated
		}
	}
	if onCreated["Review"] != "MyFirstModule.ACT_Assign" || onCreated["Plain"] != "" {
		t.Errorf("on-created = %v", onCreated)
	}

	raw, err := b2.GetRawUnit(got.ID)
	if err != nil {
		t.Fatalf("GetRawUnit: %v", err)
	}
	events, _ := raw["OnWorkflowEvent"].([]any)
	if len(events) != 2 || fmt.Sprint(events[0]) != "2" {
		t.Fatalf("OnWorkflowEvent = %v, want marker 2 and one handler", raw["OnWorkflowEvent"])
	}
	stored, _ := events[1].(map[string]any)
	if types, _ := stored["EventTypes"].([]any); len(types) != 3 || fmt.Sprint(types[0]) != "1" {
		t.Errorf("EventTypes = %v, want marker 1 and two types", stored["EventTypes"])
	}
	kinds := map[string]string{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if ev, ok := x["OnCreatedEvent"].(map[string]any); ok {
				name, _ := x["Name"].(string)
				kinds[name] = fmt.Sprint(ev["$Type"], " ", ev["Microflow"])
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(raw)
	if kinds["Review"] != "Workflows$MicroflowBasedEvent MyFirstModule.ACT_Assign" {
		t.Errorf("Review OnCreatedEvent = %q", kinds["Review"])
	}
	if kinds["Plain"] != "Workflows$NoEvent <nil>" {
		t.Errorf("Plain OnCreatedEvent = %q", kinds["Plain"])
	}
}
