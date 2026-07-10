// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
	"go.mongodb.org/mongo-driver/bson"
)

func getBSONField(doc bson.D, key string) any {
	for _, e := range doc {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func assertArrayMarker(t *testing.T, doc bson.D, field string, wantMarker int32) {
	t.Helper()
	arr, ok := getBSONField(doc, field).(bson.A)
	if !ok {
		t.Fatalf("%s is not bson.A", field)
	}
	if len(arr) == 0 {
		t.Fatalf("%s is empty", field)
	}
	marker, ok := arr[0].(int32)
	if !ok {
		t.Fatalf("%s[0] is %T, want int32", field, arr[0])
	}
	if marker != wantMarker {
		t.Errorf("%s[0] = %d, want %d", field, marker, wantMarker)
	}
}

// --- Array marker tests: verify correct int32 markers prevent CE errors ---

func TestSerializeWorkflowFlow_ActivitiesMarker(t *testing.T) {
	flow := &workflows.Flow{
		BaseElement: model.BaseElement{ID: "flow-1"},
		Activities: []workflows.WorkflowActivity{
			&workflows.StartWorkflowActivity{
				BaseWorkflowActivity: workflows.BaseWorkflowActivity{
					BaseElement: model.BaseElement{ID: "start-1"},
					Name:        "Start",
				},
			},
		},
	}
	doc := serializeWorkflowFlow(flow)
	assertArrayMarker(t, doc, "Activities", int32(3))
}

func TestSerializeWorkflowFlow_EmptyActivities(t *testing.T) {
	flow := &workflows.Flow{BaseElement: model.BaseElement{ID: "flow-empty"}}
	doc := serializeWorkflowFlow(flow)
	assertArrayMarker(t, doc, "Activities", int32(3))
	arr := getBSONField(doc, "Activities").(bson.A)
	if len(arr) != 1 {
		t.Errorf("empty Activities length = %d, want 1 (marker only)", len(arr))
	}
}

func TestSerializeUserTask_OutcomesMarker(t *testing.T) {
	task := &workflows.UserTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "ut-1"},
			Name:        "ReviewTask",
		},
		Outcomes: []*workflows.UserTaskOutcome{
			{BaseElement: model.BaseElement{ID: "out-1"}, Value: "Approve"},
		},
	}
	doc := serializeUserTask(task)
	assertArrayMarker(t, doc, "Outcomes", int32(3))
}

func TestSerializeUserTask_BoundaryEventsMarker(t *testing.T) {
	task := &workflows.UserTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "ut-2"},
			Name:        "Task",
		},
	}
	doc := serializeUserTask(task)
	assertArrayMarker(t, doc, "BoundaryEvents", int32(2))
}

func TestSerializeCallMicroflowTask_ParameterMappingsMarker(t *testing.T) {
	task := &workflows.CallMicroflowTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "cmt-1"},
			Name:        "CallMF",
		},
		Microflow: "MyModule.DoSomething",
		ParameterMappings: []*workflows.ParameterMapping{
			{BaseElement: model.BaseElement{ID: "pm-1"}, Parameter: "InputParam", Expression: "$WorkflowContext"},
		},
	}
	doc := serializeCallMicroflowTask(task)
	assertArrayMarker(t, doc, "ParameterMappings", int32(2))
}

func TestSerializeUserTaskOutcome_ValueField(t *testing.T) {
	outcome := &workflows.UserTaskOutcome{
		BaseElement: model.BaseElement{ID: "uto-1"},
		Value:       "Approve",
	}
	doc := serializeUserTaskOutcome(outcome)

	if getBSONField(doc, "Value") != "Approve" {
		t.Errorf("Value = %v, want %q", getBSONField(doc, "Value"), "Approve")
	}
	if getBSONField(doc, "Caption") != nil {
		t.Error("UserTaskOutcome must not have 'Caption' key")
	}
	if getBSONField(doc, "Name") != nil {
		t.Error("UserTaskOutcome must not have 'Name' key")
	}
}

func TestSerializeWorkflowParameter_EntityAsString(t *testing.T) {
	param := &workflows.WorkflowParameter{
		BaseElement: model.BaseElement{ID: "param-1"},
		EntityRef:   "MyModule.Customer",
	}
	doc := serializeWorkflowParameter(param)

	entity, ok := getBSONField(doc, "Entity").(string)
	if !ok {
		t.Fatalf("Entity is %T, want string", getBSONField(doc, "Entity"))
	}
	if entity != "MyModule.Customer" {
		t.Errorf("Entity = %q, want %q", entity, "MyModule.Customer")
	}
}

// --- P0 bug regression tests ---

func TestSerializeUserTask_AutoAssignSingleTargetUserDefaultsFalse(t *testing.T) {
	task := &workflows.UserTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "ut-auto"},
			Name:        "Task",
		},
	}
	doc := serializeUserTask(task)
	val := getBSONField(doc, "AutoAssignSingleTargetUser")
	if val != false {
		t.Errorf("AutoAssignSingleTargetUser = %v, want false", val)
	}
}

func TestSerializeUserTask_DueDateUsedFromStruct(t *testing.T) {
	task := &workflows.UserTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "ut-due"},
			Name:        "Task",
		},
		DueDate: "addDays([%CurrentDateTime%], 7)",
	}
	doc := serializeUserTask(task)
	val, _ := getBSONField(doc, "DueDate").(string)
	if val != "addDays([%CurrentDateTime%], 7)" {
		t.Errorf("DueDate = %q, want %q", val, "addDays([%CurrentDateTime%], 7)")
	}
}

func TestSerializeBoundaryEvents_NonInterruptingTimerHasRecurrenceNull(t *testing.T) {
	events := []*workflows.BoundaryEvent{
		{
			BaseElement: model.BaseElement{ID: "be-1"},
			EventType:   "NonInterruptingTimer",
			TimerDelay:  "addDays([%CurrentDateTime%], 1)",
		},
	}
	arr := serializeBoundaryEvents(events)
	// arr[0] is int32(2) marker, arr[1] is the event doc
	if len(arr) < 2 {
		t.Fatal("expected 2 elements in boundary events array")
	}
	doc, ok := arr[1].(bson.D)
	if !ok {
		t.Fatalf("arr[1] is %T, want bson.D", arr[1])
	}
	// Recurrence must exist with nil value
	found := false
	for _, e := range doc {
		if e.Key == "Recurrence" {
			found = true
			if e.Value != nil {
				t.Errorf("Recurrence = %v, want nil", e.Value)
			}
		}
	}
	if !found {
		t.Error("Recurrence field missing from NonInterruptingTimerBoundaryEvent")
	}
}

// --- P1: Multi-User Task missing fields ---

func TestSerializeMultiUserTask_AwaitAllUsersPresentAndFalse(t *testing.T) {
	task := &workflows.UserTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "mut-1"},
			Name:        "MultiTask",
		},
		IsMulti: true,
	}
	doc := serializeUserTask(task)
	val := getBSONField(doc, "AwaitAllUsers")
	if val == nil {
		t.Error("AwaitAllUsers field missing from MultiUserTaskActivity")
		return
	}
	if val != false {
		t.Errorf("AwaitAllUsers = %v, want false", val)
	}
}

func TestSerializeMultiUserTask_TargetUserInputPresent(t *testing.T) {
	task := &workflows.UserTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "mut-2"},
			Name:        "MultiTask",
		},
		IsMulti: true,
	}
	doc := serializeUserTask(task)
	val := getBSONField(doc, "TargetUserInput")
	if val == nil {
		t.Error("TargetUserInput field missing from MultiUserTaskActivity")
		return
	}
	tui, ok := val.(bson.D)
	if !ok {
		t.Fatalf("TargetUserInput is %T, want bson.D", val)
	}
	typeVal, _ := getBSONField(tui, "$Type").(string)
	if typeVal != "Workflows$AllUserInput" {
		t.Errorf("TargetUserInput.$Type = %q, want %q", typeVal, "Workflows$AllUserInput")
	}
}

func TestSerializeMultiUserTask_CompletionCriteriaPresent(t *testing.T) {
	task := &workflows.UserTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "mut-3"},
			Name:        "MultiTask",
		},
		IsMulti: true,
		Outcomes: []*workflows.UserTaskOutcome{
			{BaseElement: model.BaseElement{ID: "out-a"}, Value: "Approve"},
		},
	}
	doc := serializeUserTask(task)
	val := getBSONField(doc, "CompletionCriteria")
	if val == nil {
		t.Error("CompletionCriteria field missing from MultiUserTaskActivity")
		return
	}
	cc, ok := val.(bson.D)
	if !ok {
		t.Fatalf("CompletionCriteria is %T, want bson.D", val)
	}
	typeVal, _ := getBSONField(cc, "$Type").(string)
	if typeVal != "Workflows$ConsensusCompletionCriteria" {
		t.Errorf("CompletionCriteria.$Type = %q, want %q", typeVal, "Workflows$ConsensusCompletionCriteria")
	}
	// FallbackOutcomePointer must be a UUID binary
	ptr := getBSONField(cc, "FallbackOutcomePointer")
	if ptr == nil {
		t.Error("CompletionCriteria.FallbackOutcomePointer missing")
	}
}

func TestSerializeSingleUserTask_NoMultiFields(t *testing.T) {
	task := &workflows.UserTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "sut-1"},
			Name:        "SingleTask",
		},
		IsMulti: false,
	}
	doc := serializeUserTask(task)
	if getBSONField(doc, "AwaitAllUsers") != nil {
		t.Error("SingleUserTask must not have AwaitAllUsers field")
	}
	if getBSONField(doc, "CompletionCriteria") != nil {
		t.Error("SingleUserTask must not have CompletionCriteria field")
	}
	if getBSONField(doc, "TargetUserInput") != nil {
		t.Error("SingleUserTask must not have TargetUserInput field")
	}
}

// --- P2: CallWorkflowActivity must not emit ParameterExpression ---

func TestSerializeCallWorkflowActivity_NoParameterExpressionField(t *testing.T) {
	act := &workflows.CallWorkflowActivity{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "cwa-1"},
			Name:        "callWorkflow1",
		},
		Workflow:            "MyModule.SubFlow",
		ParameterExpression: "$WorkflowContext",
	}
	doc := serializeCallWorkflowActivity(act)
	for _, e := range doc {
		if e.Key == "ParameterExpression" {
			t.Error("CallWorkflowActivity must not emit ParameterExpression field (not in Studio Pro BSON)")
			return
		}
	}
}

// --- Fixture-based roundtrip: parse real BSON → serialize → verify markers preserved ---

func TestSerializeWorkflowFlow_RoundtripFromFixture(t *testing.T) {
	raw := loadWorkflowBSON(t, "WorkflowBaseline.Workflow")
	flowRaw := toMap(raw["Flow"])
	if flowRaw == nil {
		t.Fatal("fixture has no Flow")
	}

	// Parse real workflow from fixture
	flow := parseWorkflowFlow(flowRaw)
	if flow == nil {
		t.Fatal("parseWorkflowFlow returned nil")
	}

	// Serialize back to BSON
	doc := serializeWorkflowFlow(flow)

	// Verify array markers survive the roundtrip
	assertArrayMarker(t, doc, "Activities", int32(3))

	// Re-marshal and re-parse to verify full roundtrip
	data, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var reparsedRaw map[string]any
	if err := bson.Unmarshal(data, &reparsedRaw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	reparsed := parseWorkflowFlow(reparsedRaw)
	if reparsed == nil {
		t.Fatal("re-parse returned nil")
	}
	if len(reparsed.Activities) != len(flow.Activities) {
		t.Errorf("roundtrip Activities count = %d, want %d", len(reparsed.Activities), len(flow.Activities))
	}
	// Verify first activity type is preserved
	if _, ok := reparsed.Activities[0].(*workflows.StartWorkflowActivity); !ok {
		t.Errorf("roundtrip Activities[0] = %T, want *workflows.StartWorkflowActivity", reparsed.Activities[0])
	}
	// Verify last activity type is preserved
	last := reparsed.Activities[len(reparsed.Activities)-1]
	if _, ok := last.(*workflows.EndWorkflowActivity); !ok {
		t.Errorf("roundtrip last activity = %T, want *workflows.EndWorkflowActivity", last)
	}
}
