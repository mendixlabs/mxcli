// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// The element types and field names below are Studio Pro 11.14's own
// (ped_get_schema): an AI agent task is Workflows$AIAgentTaskActivity with the
// call-microflow fields, a user task's onCreatedEvent is a MicroflowBasedEvent,
// and a workflow carries onWorkflowEvent handlers. The mapper used to refuse the
// first and hard-code NoEvent and no handlers for the other two, so a workflow
// written over MCP silently lost its on-created microflows and handlers.
func TestMapAIAgentTask(t *testing.T) {
	agent := &workflows.CallMicroflowTask{IsAgent: true, Microflow: "M.InvokeAgent"}
	agent.Name = "aiAgentTask1"
	agent.Caption = "Classify"
	agent.Outcomes = []workflows.ConditionOutcome{&workflows.VoidConditionOutcome{}}
	m, err := mapWorkflowActivity(agent)
	if err != nil {
		t.Fatalf("mapWorkflowActivity(agent): %v", err)
	}
	if m["$Type"] != "Workflows$AIAgentTaskActivity" || m["microflow"] != "M.InvokeAgent" || m["name"] != "aiAgentTask1" {
		t.Errorf("agent task = %+v", m)
	}

	plain := &workflows.CallMicroflowTask{Microflow: "M.Do"}
	plain.Name = "callDo"
	pm, err := mapWorkflowActivity(plain)
	if err != nil {
		t.Fatal(err)
	}
	if pm["$Type"] != "Workflows$CallMicroflowActivity" {
		t.Errorf("plain call microflow $Type = %v", pm["$Type"])
	}
}

func TestMapUserTaskOnCreatedEvent(t *testing.T) {
	for _, multi := range []bool{false, true} {
		ut := &workflows.UserTask{IsMulti: multi, Page: "M.TaskPage", OnCreated: "M.ACT_Assign"}
		ut.Name = "review"
		ut.Outcomes = []*workflows.UserTaskOutcome{{Value: "Done"}}
		m, err := mapWorkflowActivity(ut)
		if err != nil {
			t.Fatalf("multi=%v: %v", multi, err)
		}
		want := map[string]any{"$Type": "Workflows$MicroflowBasedEvent", "microflow": "M.ACT_Assign"}
		if !reflect.DeepEqual(m["onCreatedEvent"], want) {
			t.Errorf("multi=%v onCreatedEvent = %v, want %v", multi, m["onCreatedEvent"], want)
		}

		ut.OnCreated = ""
		m, _ = mapWorkflowActivity(ut)
		if !reflect.DeepEqual(m["onCreatedEvent"], map[string]any{"$Type": "Workflows$NoEvent"}) {
			t.Errorf("multi=%v without on-created = %v, want NoEvent", multi, m["onCreatedEvent"])
		}
	}
}

func TestMapWorkflowEventHandlers(t *testing.T) {
	b := &Backend{}
	wf := &workflows.Workflow{
		Name:      "W",
		Parameter: &workflows.WorkflowParameter{EntityRef: "M.Ctx"},
		EventHandlers: []*workflows.WorkflowEventHandler{{
			Description: "Task audit",
			EventTypes:  []string{"UserTaskStarted", "UserTaskEnded"},
			Microflow:   "M.ACT_Audit",
		}},
	}
	content, err := b.mapWorkflow(wf)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := content["onWorkflowEvent"].([]any)
	if len(got) != 1 {
		t.Fatalf("onWorkflowEvent = %v, want one handler", content["onWorkflowEvent"])
	}
	want := map[string]any{
		"$Type":         "Workflows$WorkflowEventHandler",
		"description":   "Task audit",
		"documentation": "",
		"eventTypes":    []any{"UserTaskStarted", "UserTaskEnded"},
		"microflowEventHandler": map[string]any{
			"$Type":     "Workflows$MicroflowEventHandler",
			"microflow": "M.ACT_Audit",
		},
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("handler = %v\nwant      %v", got[0], want)
	}

	none, _ := b.mapWorkflow(&workflows.Workflow{Name: "W"})
	if _, ok := none["onWorkflowEvent"]; ok {
		t.Error("a workflow without handlers must not send onWorkflowEvent")
	}
}
