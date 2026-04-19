// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// makeWorkflowDoc builds a minimal workflow BSON document for testing.
func makeWorkflowDoc(activities ...bson.D) bson.D {
	actArr := bson.A{int32(3)}
	for _, a := range activities {
		actArr = append(actArr, a)
	}
	return bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$Workflow"},
		{Key: "Title", Value: "Test Workflow"},
		{Key: "WorkflowName", Value: bson.D{
			{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Text", Value: "Test Workflow"},
		}},
		{Key: "WorkflowDescription", Value: bson.D{
			{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Text", Value: "Original description"},
		}},
		{Key: "Flow", Value: bson.D{
			{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
			{Key: "$Type", Value: "Workflows$Flow"},
			{Key: "Activities", Value: actArr},
		}},
	}
}

func makeWfActivity(typeName, caption, name string) bson.D {
	return bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: typeName},
		{Key: "Caption", Value: caption},
		{Key: "Name", Value: name},
	}
}

func makeWfActivityWithBoundaryEvents(caption string, events ...bson.D) bson.D {
	evtArr := bson.A{int32(3)}
	for _, e := range events {
		evtArr = append(evtArr, e)
	}
	return bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$UserTask"},
		{Key: "Caption", Value: caption},
		{Key: "Name", Value: "task1"},
		{Key: "BoundaryEvents", Value: evtArr},
	}
}

func makeWfBoundaryEvent(typeName string) bson.D {
	return bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: typeName},
		{Key: "Caption", Value: ""},
	}
}

// newMutator creates a mprWorkflowMutator for testing (no real backend).
func newMutator(doc bson.D) *mprWorkflowMutator {
	return &mprWorkflowMutator{rawData: doc}
}

// --- SetProperty tests ---

func TestWorkflowMutator_SetProperty_Display(t *testing.T) {
	doc := makeWorkflowDoc()
	m := newMutator(doc)

	if err := m.SetProperty("DISPLAY", "New Title"); err != nil {
		t.Fatalf("SetProperty DISPLAY failed: %v", err)
	}

	if got := dGetString(m.rawData, "Title"); got != "New Title" {
		t.Errorf("Title = %q, want %q", got, "New Title")
	}
	wfName := dGetDoc(m.rawData, "WorkflowName")
	if wfName == nil {
		t.Fatal("WorkflowName is nil")
	}
	if got := dGetString(wfName, "Text"); got != "New Title" {
		t.Errorf("WorkflowName.Text = %q, want %q", got, "New Title")
	}
}

func TestWorkflowMutator_SetProperty_Display_NilSubDoc(t *testing.T) {
	doc := bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$Workflow"},
		{Key: "Title", Value: "Old"},
		{Key: "Flow", Value: bson.D{
			{Key: "Activities", Value: bson.A{int32(3)}},
		}},
	}
	m := newMutator(doc)

	if err := m.SetProperty("DISPLAY", "Created Title"); err != nil {
		t.Fatalf("SetProperty DISPLAY with nil sub-doc failed: %v", err)
	}

	if got := dGetString(m.rawData, "Title"); got != "Created Title" {
		t.Errorf("Title = %q, want %q", got, "Created Title")
	}
	wfName := dGetDoc(m.rawData, "WorkflowName")
	if wfName == nil {
		t.Fatal("WorkflowName should have been auto-created")
	}
	if got := dGetString(wfName, "Text"); got != "Created Title" {
		t.Errorf("WorkflowName.Text = %q, want %q", got, "Created Title")
	}
}

func TestWorkflowMutator_SetProperty_Description(t *testing.T) {
	doc := makeWorkflowDoc()
	m := newMutator(doc)

	if err := m.SetProperty("DESCRIPTION", "Updated desc"); err != nil {
		t.Fatalf("SetProperty DESCRIPTION failed: %v", err)
	}

	wfDesc := dGetDoc(m.rawData, "WorkflowDescription")
	if wfDesc == nil {
		t.Fatal("WorkflowDescription is nil")
	}
	if got := dGetString(wfDesc, "Text"); got != "Updated desc" {
		t.Errorf("WorkflowDescription.Text = %q, want %q", got, "Updated desc")
	}
}

func TestWorkflowMutator_SetProperty_Description_NilSubDoc(t *testing.T) {
	doc := bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$Workflow"},
		{Key: "Title", Value: "Test"},
		{Key: "Flow", Value: bson.D{
			{Key: "Activities", Value: bson.A{int32(3)}},
		}},
	}
	m := newMutator(doc)

	if err := m.SetProperty("DESCRIPTION", "New desc"); err != nil {
		t.Fatalf("SetProperty DESCRIPTION with nil sub-doc failed: %v", err)
	}

	wfDesc := dGetDoc(m.rawData, "WorkflowDescription")
	if wfDesc == nil {
		t.Fatal("WorkflowDescription should have been auto-created")
	}
	if got := dGetString(wfDesc, "Text"); got != "New desc" {
		t.Errorf("WorkflowDescription.Text = %q, want %q", got, "New desc")
	}
}

func TestWorkflowMutator_SetProperty_Unsupported(t *testing.T) {
	doc := makeWorkflowDoc()
	m := newMutator(doc)

	err := m.SetProperty("UNKNOWN_PROP", "x")
	if err == nil {
		t.Fatal("Expected error for unsupported property")
	}
	if !strings.Contains(err.Error(), "unsupported workflow property") {
		t.Errorf("Error = %q, want to contain 'unsupported workflow property'", err.Error())
	}
}

func TestWorkflowMutator_SetProperty_ExportLevel(t *testing.T) {
	doc := makeWorkflowDoc()
	doc = append(doc, bson.E{Key: "ExportLevel", Value: "Usable"})
	m := newMutator(doc)

	if err := m.SetProperty("EXPORT_LEVEL", "Hidden"); err != nil {
		t.Fatalf("SetProperty EXPORT_LEVEL failed: %v", err)
	}
	if got := dGetString(m.rawData, "ExportLevel"); got != "Hidden" {
		t.Errorf("ExportLevel = %q, want %q", got, "Hidden")
	}
}

// --- findActivityByCaption tests ---

func TestWorkflowMutator_FindActivity_Found(t *testing.T) {
	act1 := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act2 := makeWfActivity("Workflows$UserTask", "Approve", "task2")
	m := newMutator(makeWorkflowDoc(act1, act2))

	result, err := m.findActivityByCaption("Approve", 0)
	if err != nil {
		t.Fatalf("findActivityByCaption failed: %v", err)
	}
	if got := dGetString(result, "Caption"); got != "Approve" {
		t.Errorf("Caption = %q, want %q", got, "Approve")
	}
}

func TestWorkflowMutator_FindActivity_ByName(t *testing.T) {
	act1 := makeWfActivity("Workflows$UserTask", "Review", "ReviewTask")
	m := newMutator(makeWorkflowDoc(act1))

	result, err := m.findActivityByCaption("ReviewTask", 0)
	if err != nil {
		t.Fatalf("findActivityByCaption by name failed: %v", err)
	}
	if got := dGetString(result, "Name"); got != "ReviewTask" {
		t.Errorf("Name = %q, want %q", got, "ReviewTask")
	}
}

func TestWorkflowMutator_FindActivity_NotFound(t *testing.T) {
	act1 := makeWfActivity("Workflows$UserTask", "Review", "task1")
	m := newMutator(makeWorkflowDoc(act1))

	_, err := m.findActivityByCaption("NonExistent", 0)
	if err == nil {
		t.Fatal("Expected error for missing activity")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error = %q, want to contain 'not found'", err.Error())
	}
}

func TestWorkflowMutator_FindActivity_Ambiguous(t *testing.T) {
	act1 := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act2 := makeWfActivity("Workflows$UserTask", "Review", "task2")
	m := newMutator(makeWorkflowDoc(act1, act2))

	_, err := m.findActivityByCaption("Review", 0)
	if err == nil {
		t.Fatal("Expected error for ambiguous activity")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("Error = %q, want to contain 'ambiguous'", err.Error())
	}
}

func TestWorkflowMutator_FindActivity_AtPosition(t *testing.T) {
	act1 := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act2 := makeWfActivity("Workflows$UserTask", "Review", "task2")
	m := newMutator(makeWorkflowDoc(act1, act2))

	result, err := m.findActivityByCaption("Review", 2)
	if err != nil {
		t.Fatalf("findActivityByCaption @2 failed: %v", err)
	}
	if got := dGetString(result, "Name"); got != "task2" {
		t.Errorf("Name = %q, want %q", got, "task2")
	}
}

func TestWorkflowMutator_FindActivity_AtPosition_OutOfRange(t *testing.T) {
	act1 := makeWfActivity("Workflows$UserTask", "Review", "task1")
	m := newMutator(makeWorkflowDoc(act1))

	_, err := m.findActivityByCaption("Review", 5)
	if err == nil {
		t.Fatal("Expected error for out-of-range position")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error = %q, want to contain 'not found'", err.Error())
	}
}

// --- DropActivity tests ---

func TestWorkflowMutator_DropActivity(t *testing.T) {
	act1 := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act2 := makeWfActivity("Workflows$UserTask", "Approve", "task2")
	act3 := makeWfActivity("Workflows$UserTask", "Finalize", "task3")
	m := newMutator(makeWorkflowDoc(act1, act2, act3))

	if err := m.DropActivity("Approve", 0); err != nil {
		t.Fatalf("DropActivity failed: %v", err)
	}

	flow := dGetDoc(m.rawData, "Flow")
	activities := dGetArrayElements(dGet(flow, "Activities"))
	if len(activities) != 2 {
		t.Fatalf("Expected 2 activities after drop, got %d", len(activities))
	}
	name0 := dGetString(activities[0].(bson.D), "Caption")
	name1 := dGetString(activities[1].(bson.D), "Caption")
	if name0 != "Review" {
		t.Errorf("First activity caption = %q, want %q", name0, "Review")
	}
	if name1 != "Finalize" {
		t.Errorf("Second activity caption = %q, want %q", name1, "Finalize")
	}
}

func TestWorkflowMutator_DropActivity_NotFound(t *testing.T) {
	act1 := makeWfActivity("Workflows$UserTask", "Review", "task1")
	m := newMutator(makeWorkflowDoc(act1))

	err := m.DropActivity("NonExistent", 0)
	if err == nil {
		t.Fatal("Expected error for dropping nonexistent activity")
	}
}

// --- DropBoundaryEvent tests ---

func TestWorkflowMutator_DropBoundaryEvent_Single(t *testing.T) {
	evt := makeWfBoundaryEvent("Workflows$InterruptingTimerBoundaryEvent")
	act := makeWfActivityWithBoundaryEvents("Review", evt)
	m := newMutator(makeWorkflowDoc(act))

	if err := m.DropBoundaryEvent("Review", 0); err != nil {
		t.Fatalf("DropBoundaryEvent failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	events := dGetArrayElements(dGet(actDoc, "BoundaryEvents"))
	if len(events) != 0 {
		t.Errorf("Expected 0 boundary events after drop, got %d", len(events))
	}
}

func TestWorkflowMutator_DropBoundaryEvent_Multiple(t *testing.T) {
	evt1 := makeWfBoundaryEvent("Workflows$InterruptingTimerBoundaryEvent")
	evt2 := makeWfBoundaryEvent("Workflows$NonInterruptingTimerBoundaryEvent")
	act := makeWfActivityWithBoundaryEvents("Review", evt1, evt2)
	m := newMutator(makeWorkflowDoc(act))

	if err := m.DropBoundaryEvent("Review", 0); err != nil {
		t.Fatalf("DropBoundaryEvent failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	events := dGetArrayElements(dGet(actDoc, "BoundaryEvents"))
	if len(events) != 1 {
		t.Fatalf("Expected 1 boundary event after drop, got %d", len(events))
	}
	remaining := events[0].(bson.D)
	if got := dGetString(remaining, "$Type"); got != "Workflows$NonInterruptingTimerBoundaryEvent" {
		t.Errorf("Remaining event type = %q, want NonInterruptingTimerBoundaryEvent", got)
	}
}

func TestWorkflowMutator_DropBoundaryEvent_NoEvents(t *testing.T) {
	act := makeWfActivityWithBoundaryEvents("Review")
	m := newMutator(makeWorkflowDoc(act))

	err := m.DropBoundaryEvent("Review", 0)
	if err == nil {
		t.Fatal("Expected error when dropping from activity with no boundary events")
	}
	if !strings.Contains(err.Error(), "no boundary events") {
		t.Errorf("Error = %q, want to contain 'no boundary events'", err.Error())
	}
}

// --- findActivityIndex tests ---

func TestWorkflowMutator_FindActivityIndex(t *testing.T) {
	act1 := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act2 := makeWfActivity("Workflows$UserTask", "Approve", "task2")
	m := newMutator(makeWorkflowDoc(act1, act2))

	idx, activities, flow, err := m.findActivityIndex("Approve", 0)
	if err != nil {
		t.Fatalf("findActivityIndex failed: %v", err)
	}
	if idx != 1 {
		t.Errorf("index = %d, want 1", idx)
	}
	if len(activities) != 2 {
		t.Errorf("activities length = %d, want 2", len(activities))
	}
	if flow == nil {
		t.Error("flow should not be nil")
	}
}

func TestWorkflowMutator_FindActivityIndex_NoFlow(t *testing.T) {
	doc := bson.D{
		{Key: "$Type", Value: "Workflows$Workflow"},
	}
	m := newMutator(doc)

	_, _, _, err := m.findActivityIndex("Review", 0)
	if err == nil {
		t.Fatal("Expected error for doc without Flow")
	}
	if !strings.Contains(err.Error(), "no Flow") {
		t.Errorf("Error = %q, want to contain 'no Flow'", err.Error())
	}
}

// --- collectAllActivityNames tests ---

func TestWorkflowMutator_CollectAllActivityNames(t *testing.T) {
	act1 := makeWfActivity("Workflows$UserTask", "Review", "ReviewTask")
	act2 := makeWfActivity("Workflows$UserTask", "Approve", "ApproveTask")
	m := newMutator(makeWorkflowDoc(act1, act2))

	names := m.collectAllActivityNames()
	if !names["ReviewTask"] {
		t.Error("Expected ReviewTask in names")
	}
	if !names["ApproveTask"] {
		t.Error("Expected ApproveTask in names")
	}
	if names["NonExistent"] {
		t.Error("NonExistent should not be in names")
	}
}

func TestWorkflowMutator_CollectAllActivityNames_NoFlow(t *testing.T) {
	doc := bson.D{{Key: "$Type", Value: "Workflows$Workflow"}}
	m := newMutator(doc)

	names := m.collectAllActivityNames()
	if len(names) != 0 {
		t.Errorf("Expected empty names map, got %d entries", len(names))
	}
}

// --- SetActivityProperty tests ---

func TestWorkflowMutator_SetActivityProperty_DueDate(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act = append(act, bson.E{Key: "DueDate", Value: ""})
	m := newMutator(makeWorkflowDoc(act))

	if err := m.SetActivityProperty("Review", 0, "DUE_DATE", "${PT48H}"); err != nil {
		t.Fatalf("SetActivityProperty DUE_DATE failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	if got := dGetString(actDoc, "DueDate"); got != "${PT48H}" {
		t.Errorf("DueDate = %q, want %q", got, "${PT48H}")
	}
}

func TestWorkflowMutator_SetActivityProperty_Unsupported(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Review", "task1")
	m := newMutator(makeWorkflowDoc(act))

	err := m.SetActivityProperty("Review", 0, "INVALID", "x")
	if err == nil {
		t.Fatal("Expected error for unsupported activity property")
	}
	if !strings.Contains(err.Error(), "unsupported activity property") {
		t.Errorf("Error = %q, want to contain 'unsupported activity property'", err.Error())
	}
}

// --- DropOutcome tests ---

func TestWorkflowMutator_DropOutcome_NotFound(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act = append(act, bson.E{Key: "Outcomes", Value: bson.A{int32(3)}})
	m := newMutator(makeWorkflowDoc(act))

	err := m.DropOutcome("Review", 0, "NonExistent")
	if err == nil {
		t.Fatal("Expected error for dropping nonexistent outcome")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error = %q, want to contain 'not found'", err.Error())
	}
}

// --- bsonArrayMarker constant test ---

func TestWorkflowMutator_BsonArrayMarkerConstant(t *testing.T) {
	if bsonArrayMarker != int32(3) {
		t.Errorf("bsonArrayMarker = %v, want int32(3)", bsonArrayMarker)
	}
}
