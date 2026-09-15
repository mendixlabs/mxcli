// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
	"go.mongodb.org/mongo-driver/bson"
)

// The handler shape is ako/TestApp's workflow.Workflow1 (Studio Pro 11.14.0):
// OnWorkflowEvent is a marker-2 list, each handler's EventTypes a marker-1 list
// of strings, and the microflow sits in a nested MicroflowEventHandler.
func TestSerializeWorkflowEventHandlers_Shape(t *testing.T) {
	arr := serializeWorkflowEventHandlers([]*workflows.WorkflowEventHandler{{
		Description: "Task audit",
		EventTypes:  []string{"UserTaskStarted", "UserTaskEnded"},
		Microflow:   "M.ACT_Audit",
	}})
	if len(arr) != 2 || arr[0] != int32(2) {
		t.Fatalf("OnWorkflowEvent = %v, want marker 2 and one handler", arr)
	}
	h, ok := arr[1].(bson.D)
	if !ok {
		t.Fatalf("handler is %T", arr[1])
	}
	var keys []string
	for _, e := range h {
		keys = append(keys, e.Key)
	}
	if want := []string{"$ID", "$Type", "Description", "Documentation", "EventTypes", "MicroflowEventHandler"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("handler keys = %v, want %v", keys, want)
	}
	if got := getBSONField(h, "$Type"); got != "Workflows$WorkflowEventHandler" {
		t.Errorf("$Type = %v", got)
	}
	if got := getBSONField(h, "EventTypes"); !reflect.DeepEqual(got, bson.A{int32(1), "UserTaskStarted", "UserTaskEnded"}) {
		t.Errorf("EventTypes = %v", got)
	}
	mh, ok := getBSONField(h, "MicroflowEventHandler").(bson.D)
	if !ok || getBSONField(mh, "$Type") != "Workflows$MicroflowEventHandler" || getBSONField(mh, "Microflow") != "M.ACT_Audit" {
		t.Errorf("MicroflowEventHandler = %v", mh)
	}
}

func TestSerializeWorkflowEventHandlers_NoneIsTheBareMarker(t *testing.T) {
	if got := serializeWorkflowEventHandlers(nil); !reflect.DeepEqual(got, bson.A{int32(2)}) {
		t.Errorf("OnWorkflowEvent = %v, want [2]", got)
	}
}

func TestSerializeOnCreatedEvent(t *testing.T) {
	none := serializeOnCreatedEvent("")
	if getBSONField(none, "$Type") != "Workflows$NoEvent" || getBSONField(none, "Microflow") != nil {
		t.Errorf("no microflow = %v, want a bare NoEvent", none)
	}
	ev := serializeOnCreatedEvent("M.ACT_Assign")
	if getBSONField(ev, "$Type") != "Workflows$MicroflowBasedEvent" || getBSONField(ev, "Microflow") != "M.ACT_Assign" {
		t.Errorf("microflow = %v", ev)
	}
}

// The parser read OnCreatedEvent as a string, which a stored document never is,
// so every on-created microflow read back as none — and a describe of it lost
// it. Round-tripped through real BSON bytes, as the reader sees them.
func TestParseWorkflow_OnCreatedAndHandlersRoundTrip(t *testing.T) {
	doc := bson.D{
		{Key: "$Type", Value: "Workflows$Workflow"},
		{Key: "OnWorkflowEvent", Value: serializeWorkflowEventHandlers([]*workflows.WorkflowEventHandler{
			{Description: "OnAnyEvent", Documentation: "kept", EventTypes: []string{"WorkflowCompleted"}, Microflow: "M.ACT_Log"},
		})},
		{Key: "Task", Value: bson.D{
			{Key: "$Type", Value: "Workflows$SingleUserTaskActivity"},
			{Key: "Name", Value: "userTask1"},
			{Key: "OnCreatedEvent", Value: serializeOnCreatedEvent("M.ACT_Assign")},
		}},
	}
	data, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}

	handlers := parseWorkflowEventHandlers(raw["OnWorkflowEvent"])
	if len(handlers) != 1 {
		t.Fatalf("handlers = %d, want 1", len(handlers))
	}
	h := handlers[0]
	if h.Description != "OnAnyEvent" || h.Documentation != "kept" || h.Microflow != "M.ACT_Log" ||
		!reflect.DeepEqual(h.EventTypes, []string{"WorkflowCompleted"}) {
		t.Errorf("handler = %+v", h)
	}

	task := parseUserTask(toMap(raw["Task"]))
	if task.OnCreated != "M.ACT_Assign" {
		t.Errorf("OnCreated = %q, want M.ACT_Assign", task.OnCreated)
	}
}
