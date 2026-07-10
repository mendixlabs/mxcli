// SPDX-License-Identifier: Apache-2.0

package wfmutator

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/bsonnav"
)

// testWfDeps is a minimal Deps for unit tests: it serializes an activity to a
// bson.D carrying just $Type/Caption/Name (enough for the structural assertions)
// and discards Save output.
type testWfDeps struct{}

func (testWfDeps) SerializeWorkflowActivity(a workflows.WorkflowActivity) bson.D {
	return bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$SingleUserTaskActivity"},
		{Key: "Caption", Value: a.GetCaption()},
		{Key: "Name", Value: a.GetName()},
	}
}

func (testWfDeps) SaveUnit(string, []byte) error { return nil }

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

// newMutator creates a Mutator for testing with the minimal test deps.
func newMutator(doc bson.D) *Mutator {
	return New(doc, "", testWfDeps{})
}

// --- SetProperty tests ---

func TestWorkflowMutator_SetProperty_Display(t *testing.T) {
	doc := makeWorkflowDoc()
	m := newMutator(doc)

	if err := m.SetProperty("display", "New Title"); err != nil {
		t.Fatalf("SetProperty display failed: %v", err)
	}

	if got := bsonnav.DGetString(m.rawData, "Title"); got != "New Title" {
		t.Errorf("Title = %q, want %q", got, "New Title")
	}
	wfName := bsonnav.DGetDoc(m.rawData, "WorkflowName")
	if wfName == nil {
		t.Fatal("WorkflowName is nil")
	}
	if got := bsonnav.DGetString(wfName, "Text"); got != "New Title" {
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

	if err := m.SetProperty("display", "Created Title"); err != nil {
		t.Fatalf("SetProperty display with nil sub-doc failed: %v", err)
	}

	if got := bsonnav.DGetString(m.rawData, "Title"); got != "Created Title" {
		t.Errorf("Title = %q, want %q", got, "Created Title")
	}
	wfName := bsonnav.DGetDoc(m.rawData, "WorkflowName")
	if wfName == nil {
		t.Fatal("WorkflowName should have been auto-created")
	}
	if got := bsonnav.DGetString(wfName, "Text"); got != "Created Title" {
		t.Errorf("WorkflowName.Text = %q, want %q", got, "Created Title")
	}
}

func TestWorkflowMutator_SetProperty_Description(t *testing.T) {
	doc := makeWorkflowDoc()
	m := newMutator(doc)

	if err := m.SetProperty("description", "Updated desc"); err != nil {
		t.Fatalf("SetProperty description failed: %v", err)
	}

	wfDesc := bsonnav.DGetDoc(m.rawData, "WorkflowDescription")
	if wfDesc == nil {
		t.Fatal("WorkflowDescription is nil")
	}
	if got := bsonnav.DGetString(wfDesc, "Text"); got != "Updated desc" {
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

	if err := m.SetProperty("description", "New desc"); err != nil {
		t.Fatalf("SetProperty description with nil sub-doc failed: %v", err)
	}

	wfDesc := bsonnav.DGetDoc(m.rawData, "WorkflowDescription")
	if wfDesc == nil {
		t.Fatal("WorkflowDescription should have been auto-created")
	}
	if got := bsonnav.DGetString(wfDesc, "Text"); got != "New desc" {
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

	if err := m.SetProperty("export_level", "Hidden"); err != nil {
		t.Fatalf("SetProperty EXPORT_LEVEL failed: %v", err)
	}
	if got := bsonnav.DGetString(m.rawData, "ExportLevel"); got != "Hidden" {
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
	if got := bsonnav.DGetString(result, "Caption"); got != "Approve" {
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
	if got := bsonnav.DGetString(result, "Name"); got != "ReviewTask" {
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
	if got := bsonnav.DGetString(result, "Name"); got != "task2" {
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

	flow := bsonnav.DGetDoc(m.rawData, "Flow")
	activities := bsonnav.DGetArrayElements(bsonnav.DGet(flow, "Activities"))
	if len(activities) != 2 {
		t.Fatalf("Expected 2 activities after drop, got %d", len(activities))
	}
	name0 := bsonnav.DGetString(activities[0].(bson.D), "Caption")
	name1 := bsonnav.DGetString(activities[1].(bson.D), "Caption")
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
	events := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "BoundaryEvents"))
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
	events := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "BoundaryEvents"))
	if len(events) != 1 {
		t.Fatalf("Expected 1 boundary event after drop, got %d", len(events))
	}
	remaining := events[0].(bson.D)
	if got := bsonnav.DGetString(remaining, "$Type"); got != "Workflows$NonInterruptingTimerBoundaryEvent" {
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

	if err := m.SetActivityProperty("Review", 0, "due_date", "${PT48H}"); err != nil {
		t.Fatalf("SetActivityProperty DUE_DATE failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	if got := bsonnav.DGetString(actDoc, "DueDate"); got != "${PT48H}" {
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

// ---------------------------------------------------------------------------
// Helper: create a concrete WorkflowActivity for Insert/Replace tests
// ---------------------------------------------------------------------------

func makeTestWorkflowActivity(name, caption string) workflows.WorkflowActivity {
	return &workflows.UserTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			Name:    name,
			Caption: caption,
		},
	}
}

// makeWfActivityWithOutcomes builds an activity with named outcomes.
func makeWfActivityWithOutcomes(caption, name string, outcomes ...bson.D) bson.D {
	arr := bson.A{int32(3)}
	for _, o := range outcomes {
		arr = append(arr, o)
	}
	return bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$UserTask"},
		{Key: "Caption", Value: caption},
		{Key: "Name", Value: name},
		{Key: "Outcomes", Value: arr},
	}
}

func makeOutcome(typeName, value string) bson.D {
	d := bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: typeName},
	}
	if value != "" {
		d = append(d, bson.E{Key: "Value", Value: value})
	}
	return d
}

func makeBoolOutcome(val bool) bson.D {
	return bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$BooleanConditionOutcome"},
		{Key: "Value", Value: val},
	}
}

func makeVoidConditionOutcome() bson.D {
	return bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$VoidConditionOutcome"},
	}
}

// getActivities returns the activity BSON docs from the workflow flow.
func getActivities(doc bson.D) []bson.D {
	flow := bsonnav.DGetDoc(doc, "Flow")
	if flow == nil {
		return nil
	}
	elems := bsonnav.DGetArrayElements(bsonnav.DGet(flow, "Activities"))
	var result []bson.D
	for _, e := range elems {
		if d, ok := e.(bson.D); ok {
			result = append(result, d)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// InsertAfterActivity tests
// ---------------------------------------------------------------------------

func TestWorkflowMutator_InsertAfterActivity(t *testing.T) {
	act1 := makeWfActivity("Workflows$UserTask", "First", "task1")
	act2 := makeWfActivity("Workflows$UserTask", "Last", "task2")
	m := newMutator(makeWorkflowDoc(act1, act2))

	newAct := makeTestWorkflowActivity("inserted", "Inserted")
	if err := m.InsertAfterActivity("First", 0, []workflows.WorkflowActivity{newAct}); err != nil {
		t.Fatalf("InsertAfterActivity failed: %v", err)
	}

	acts := getActivities(m.rawData)
	if len(acts) != 3 {
		t.Fatalf("Expected 3 activities, got %d", len(acts))
	}
	if got := bsonnav.DGetString(acts[0], "Caption"); got != "First" {
		t.Errorf("acts[0].Caption = %q, want First", got)
	}
	// Inserted activity should be at position 1
	if got := bsonnav.DGetString(acts[1], "Name"); got != "inserted" {
		t.Errorf("acts[1].Name = %q, want inserted", got)
	}
	if got := bsonnav.DGetString(acts[2], "Caption"); got != "Last" {
		t.Errorf("acts[2].Caption = %q, want Last", got)
	}
}

func TestWorkflowMutator_InsertAfterActivity_NotFound(t *testing.T) {
	m := newMutator(makeWorkflowDoc(makeWfActivity("Workflows$UserTask", "Only", "task1")))
	err := m.InsertAfterActivity("Missing", 0, []workflows.WorkflowActivity{makeTestWorkflowActivity("new", "New")})
	if err == nil {
		t.Fatal("Expected error for missing activity")
	}
}

// ---------------------------------------------------------------------------
// ReplaceActivity tests
// ---------------------------------------------------------------------------

func TestWorkflowMutator_ReplaceActivity(t *testing.T) {
	act1 := makeWfActivity("Workflows$UserTask", "First", "task1")
	act2 := makeWfActivity("Workflows$UserTask", "ToReplace", "task2")
	act3 := makeWfActivity("Workflows$UserTask", "Last", "task3")
	m := newMutator(makeWorkflowDoc(act1, act2, act3))

	repA := makeTestWorkflowActivity("repA", "ReplacementA")
	repB := makeTestWorkflowActivity("repB", "ReplacementB")
	if err := m.ReplaceActivity("ToReplace", 0, []workflows.WorkflowActivity{repA, repB}); err != nil {
		t.Fatalf("ReplaceActivity failed: %v", err)
	}

	acts := getActivities(m.rawData)
	if len(acts) != 4 {
		t.Fatalf("Expected 4 activities, got %d", len(acts))
	}
	if got := bsonnav.DGetString(acts[0], "Caption"); got != "First" {
		t.Errorf("acts[0] = %q, want First", got)
	}
	if got := bsonnav.DGetString(acts[1], "Name"); got != "repA" {
		t.Errorf("acts[1].Name = %q, want repA", got)
	}
	if got := bsonnav.DGetString(acts[2], "Name"); got != "repB" {
		t.Errorf("acts[2].Name = %q, want repB", got)
	}
	if got := bsonnav.DGetString(acts[3], "Caption"); got != "Last" {
		t.Errorf("acts[3] = %q, want Last", got)
	}
}

func TestWorkflowMutator_ReplaceActivity_NotFound(t *testing.T) {
	m := newMutator(makeWorkflowDoc(makeWfActivity("Workflows$UserTask", "Only", "task1")))
	err := m.ReplaceActivity("Missing", 0, []workflows.WorkflowActivity{makeTestWorkflowActivity("new", "New")})
	if err == nil {
		t.Fatal("Expected error for missing activity")
	}
}

// ---------------------------------------------------------------------------
// InsertOutcome tests
// ---------------------------------------------------------------------------

func TestWorkflowMutator_InsertOutcome(t *testing.T) {
	act := makeWfActivityWithOutcomes("Review", "task1")
	m := newMutator(makeWorkflowDoc(act))

	if err := m.InsertOutcome("Review", 0, "Approved", nil); err != nil {
		t.Fatalf("InsertOutcome failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	if len(outcomes) != 1 {
		t.Fatalf("Expected 1 outcome, got %d", len(outcomes))
	}
	oDoc, ok := outcomes[0].(bson.D)
	if !ok {
		t.Fatal("Outcome is not bson.D")
	}
	if got := bsonnav.DGetString(oDoc, "Value"); got != "Approved" {
		t.Errorf("Outcome Value = %q, want Approved", got)
	}
	if got := bsonnav.DGetString(oDoc, "$Type"); got != "Workflows$UserTaskOutcome" {
		t.Errorf("Outcome $Type = %q, want Workflows$UserTaskOutcome", got)
	}
}

func TestWorkflowMutator_InsertOutcome_WithActivities(t *testing.T) {
	act := makeWfActivityWithOutcomes("Review", "task1")
	m := newMutator(makeWorkflowDoc(act))

	subAct := makeTestWorkflowActivity("sub1", "SubTask")
	if err := m.InsertOutcome("Review", 0, "Rejected", []workflows.WorkflowActivity{subAct}); err != nil {
		t.Fatalf("InsertOutcome with activities failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	if len(outcomes) != 1 {
		t.Fatalf("Expected 1 outcome, got %d", len(outcomes))
	}
	oDoc := outcomes[0].(bson.D)
	flow := bsonnav.DGetDoc(oDoc, "Flow")
	if flow == nil {
		t.Fatal("Expected Flow on outcome with activities")
	}
}

func TestWorkflowMutator_InsertOutcome_ActivityNotFound(t *testing.T) {
	m := newMutator(makeWorkflowDoc(makeWfActivity("Workflows$UserTask", "Only", "task1")))
	err := m.InsertOutcome("Missing", 0, "x", nil)
	if err == nil {
		t.Fatal("Expected error for missing activity")
	}
}

// ---------------------------------------------------------------------------
// DropOutcome tests (existing test covers NotFound; add success case)
// ---------------------------------------------------------------------------

func TestWorkflowMutator_DropOutcome_ByValue(t *testing.T) {
	outcome1 := makeOutcome("Workflows$UserTaskOutcome", "Approve")
	outcome2 := makeOutcome("Workflows$UserTaskOutcome", "Reject")
	act := makeWfActivityWithOutcomes("Review", "task1", outcome1, outcome2)
	m := newMutator(makeWorkflowDoc(act))

	if err := m.DropOutcome("Review", 0, "Approve"); err != nil {
		t.Fatalf("DropOutcome failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	if len(outcomes) != 1 {
		t.Fatalf("Expected 1 remaining outcome, got %d", len(outcomes))
	}
	oDoc := outcomes[0].(bson.D)
	if got := bsonnav.DGetString(oDoc, "Value"); got != "Reject" {
		t.Errorf("Remaining outcome = %q, want Reject", got)
	}
}

func TestWorkflowMutator_DropOutcome_Default(t *testing.T) {
	voidOutcome := makeVoidConditionOutcome()
	namedOutcome := makeOutcome("Workflows$UserTaskOutcome", "Approve")
	act := makeWfActivityWithOutcomes("Review", "task1", voidOutcome, namedOutcome)
	m := newMutator(makeWorkflowDoc(act))

	if err := m.DropOutcome("Review", 0, "Default"); err != nil {
		t.Fatalf("DropOutcome Default failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	if len(outcomes) != 1 {
		t.Fatalf("Expected 1 remaining outcome, got %d", len(outcomes))
	}
}

// ---------------------------------------------------------------------------
// InsertPath tests
// ---------------------------------------------------------------------------

func TestWorkflowMutator_InsertPath(t *testing.T) {
	act := makeWfActivityWithOutcomes("Split", "split1")
	act[1] = bson.E{Key: "$Type", Value: "Workflows$ParallelSplitActivity"}
	m := newMutator(makeWorkflowDoc(act))

	if err := m.InsertPath("Split", 0, "", nil); err != nil {
		t.Fatalf("InsertPath failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Split", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	if len(outcomes) != 1 {
		t.Fatalf("Expected 1 path, got %d", len(outcomes))
	}
	oDoc := outcomes[0].(bson.D)
	if got := bsonnav.DGetString(oDoc, "$Type"); got != "Workflows$ParallelSplitOutcome" {
		t.Errorf("Path $Type = %q, want Workflows$ParallelSplitOutcome", got)
	}
}

func TestWorkflowMutator_InsertPath_WithActivities(t *testing.T) {
	act := makeWfActivityWithOutcomes("Split", "split1")
	act[1] = bson.E{Key: "$Type", Value: "Workflows$ParallelSplitActivity"}
	m := newMutator(makeWorkflowDoc(act))

	subAct := makeTestWorkflowActivity("path_act", "PathAct")
	if err := m.InsertPath("Split", 0, "", []workflows.WorkflowActivity{subAct}); err != nil {
		t.Fatalf("InsertPath with activities failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Split", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	oDoc := outcomes[0].(bson.D)
	flow := bsonnav.DGetDoc(oDoc, "Flow")
	if flow == nil {
		t.Fatal("Expected Flow on path with activities")
	}
}

// ---------------------------------------------------------------------------
// DropPath tests
// ---------------------------------------------------------------------------

func TestWorkflowMutator_DropPath_ByCaption(t *testing.T) {
	path1 := bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$ParallelSplitOutcome"},
	}
	path2 := bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$ParallelSplitOutcome"},
	}
	act := makeWfActivityWithOutcomes("Split", "split1", path1, path2)
	act[1] = bson.E{Key: "$Type", Value: "Workflows$ParallelSplitActivity"}
	m := newMutator(makeWorkflowDoc(act))

	if err := m.DropPath("Split", 0, "Path 1"); err != nil {
		t.Fatalf("DropPath failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Split", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	if len(outcomes) != 1 {
		t.Fatalf("Expected 1 remaining path, got %d", len(outcomes))
	}
}

func TestWorkflowMutator_DropPath_EmptyCaption_DropsLast(t *testing.T) {
	path1 := bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$ParallelSplitOutcome"},
		{Key: "Tag", Value: "first"},
	}
	path2 := bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$ParallelSplitOutcome"},
		{Key: "Tag", Value: "second"},
	}
	act := makeWfActivityWithOutcomes("Split", "split1", path1, path2)
	m := newMutator(makeWorkflowDoc(act))

	if err := m.DropPath("Split", 0, ""); err != nil {
		t.Fatalf("DropPath empty caption failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Split", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	if len(outcomes) != 1 {
		t.Fatalf("Expected 1 remaining path, got %d", len(outcomes))
	}
	oDoc := outcomes[0].(bson.D)
	if got := bsonnav.DGetString(oDoc, "Tag"); got != "first" {
		t.Errorf("Remaining path Tag = %q, want first", got)
	}
}

func TestWorkflowMutator_DropPath_NotFound(t *testing.T) {
	act := makeWfActivityWithOutcomes("Split", "split1")
	m := newMutator(makeWorkflowDoc(act))

	err := m.DropPath("Split", 0, "Path 99")
	if err == nil {
		t.Fatal("Expected error for missing path")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error = %q, want 'not found'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// InsertBranch tests
// ---------------------------------------------------------------------------

func TestWorkflowMutator_InsertBranch_True(t *testing.T) {
	act := makeWfActivityWithOutcomes("Decision", "dec1")
	act[1] = bson.E{Key: "$Type", Value: "Workflows$ExclusiveSplitActivity"}
	m := newMutator(makeWorkflowDoc(act))

	if err := m.InsertBranch("Decision", 0, "true", nil); err != nil {
		t.Fatalf("InsertBranch true failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Decision", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	if len(outcomes) != 1 {
		t.Fatalf("Expected 1 branch, got %d", len(outcomes))
	}
	oDoc := outcomes[0].(bson.D)
	if got := bsonnav.DGetString(oDoc, "$Type"); got != "Workflows$BooleanConditionOutcome" {
		t.Errorf("Branch $Type = %q, want BooleanConditionOutcome", got)
	}
	if v, ok := bsonnav.DGet(oDoc, "Value").(bool); !ok || !v {
		t.Error("Expected Value=true on boolean branch")
	}
}

func TestWorkflowMutator_InsertBranch_False(t *testing.T) {
	act := makeWfActivityWithOutcomes("Decision", "dec1")
	m := newMutator(makeWorkflowDoc(act))

	if err := m.InsertBranch("Decision", 0, "false", nil); err != nil {
		t.Fatalf("InsertBranch false failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Decision", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	oDoc := outcomes[0].(bson.D)
	if v, ok := bsonnav.DGet(oDoc, "Value").(bool); !ok || v {
		t.Error("Expected Value=false on boolean branch")
	}
}

func TestWorkflowMutator_InsertBranch_Default(t *testing.T) {
	act := makeWfActivityWithOutcomes("Decision", "dec1")
	m := newMutator(makeWorkflowDoc(act))

	if err := m.InsertBranch("Decision", 0, "default", nil); err != nil {
		t.Fatalf("InsertBranch default failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Decision", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	oDoc := outcomes[0].(bson.D)
	if got := bsonnav.DGetString(oDoc, "$Type"); got != "Workflows$VoidConditionOutcome" {
		t.Errorf("Branch $Type = %q, want VoidConditionOutcome", got)
	}
}

func TestWorkflowMutator_InsertBranch_Enum(t *testing.T) {
	act := makeWfActivityWithOutcomes("Decision", "dec1")
	m := newMutator(makeWorkflowDoc(act))

	if err := m.InsertBranch("Decision", 0, "MyModule.Status.Active", nil); err != nil {
		t.Fatalf("InsertBranch enum failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Decision", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	oDoc := outcomes[0].(bson.D)
	if got := bsonnav.DGetString(oDoc, "$Type"); got != "Workflows$EnumerationValueConditionOutcome" {
		t.Errorf("Branch $Type = %q, want EnumerationValueConditionOutcome", got)
	}
	if got := bsonnav.DGetString(oDoc, "Value"); got != "MyModule.Status.Active" {
		t.Errorf("Branch Value = %q, want MyModule.Status.Active", got)
	}
}

func TestWorkflowMutator_InsertBranch_WithActivities(t *testing.T) {
	act := makeWfActivityWithOutcomes("Decision", "dec1")
	m := newMutator(makeWorkflowDoc(act))

	subAct := makeTestWorkflowActivity("branch_act", "BranchAct")
	if err := m.InsertBranch("Decision", 0, "true", []workflows.WorkflowActivity{subAct}); err != nil {
		t.Fatalf("InsertBranch with activities failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Decision", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	oDoc := outcomes[0].(bson.D)
	flow := bsonnav.DGetDoc(oDoc, "Flow")
	if flow == nil {
		t.Fatal("Expected Flow on branch with activities")
	}
}

// ---------------------------------------------------------------------------
// DropBranch tests
// ---------------------------------------------------------------------------

func TestWorkflowMutator_DropBranch_True(t *testing.T) {
	trueOutcome := makeBoolOutcome(true)
	falseOutcome := makeBoolOutcome(false)
	act := makeWfActivityWithOutcomes("Decision", "dec1", trueOutcome, falseOutcome)
	m := newMutator(makeWorkflowDoc(act))

	if err := m.DropBranch("Decision", 0, "true"); err != nil {
		t.Fatalf("DropBranch true failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Decision", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	if len(outcomes) != 1 {
		t.Fatalf("Expected 1 remaining, got %d", len(outcomes))
	}
	oDoc := outcomes[0].(bson.D)
	if v, ok := bsonnav.DGet(oDoc, "Value").(bool); !ok || v {
		t.Error("Remaining branch should be false")
	}
}

func TestWorkflowMutator_DropBranch_False(t *testing.T) {
	trueOutcome := makeBoolOutcome(true)
	falseOutcome := makeBoolOutcome(false)
	act := makeWfActivityWithOutcomes("Decision", "dec1", trueOutcome, falseOutcome)
	m := newMutator(makeWorkflowDoc(act))

	if err := m.DropBranch("Decision", 0, "false"); err != nil {
		t.Fatalf("DropBranch false failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Decision", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	if len(outcomes) != 1 {
		t.Fatalf("Expected 1 remaining, got %d", len(outcomes))
	}
}

func TestWorkflowMutator_DropBranch_Default(t *testing.T) {
	voidOutcome := makeVoidConditionOutcome()
	enumOutcome := makeOutcome("Workflows$EnumerationValueConditionOutcome", "Active")
	act := makeWfActivityWithOutcomes("Decision", "dec1", voidOutcome, enumOutcome)
	m := newMutator(makeWorkflowDoc(act))

	if err := m.DropBranch("Decision", 0, "default"); err != nil {
		t.Fatalf("DropBranch default failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Decision", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	if len(outcomes) != 1 {
		t.Fatalf("Expected 1 remaining, got %d", len(outcomes))
	}
	oDoc := outcomes[0].(bson.D)
	if got := bsonnav.DGetString(oDoc, "Value"); got != "Active" {
		t.Errorf("Remaining = %q, want Active", got)
	}
}

func TestWorkflowMutator_DropBranch_Enum(t *testing.T) {
	enum1 := makeOutcome("Workflows$EnumerationValueConditionOutcome", "Active")
	enum2 := makeOutcome("Workflows$EnumerationValueConditionOutcome", "Inactive")
	act := makeWfActivityWithOutcomes("Decision", "dec1", enum1, enum2)
	m := newMutator(makeWorkflowDoc(act))

	if err := m.DropBranch("Decision", 0, "Active"); err != nil {
		t.Fatalf("DropBranch enum failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Decision", 0)
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	if len(outcomes) != 1 {
		t.Fatalf("Expected 1 remaining, got %d", len(outcomes))
	}
	oDoc := outcomes[0].(bson.D)
	if got := bsonnav.DGetString(oDoc, "Value"); got != "Inactive" {
		t.Errorf("Remaining = %q, want Inactive", got)
	}
}

func TestWorkflowMutator_DropBranch_NotFound(t *testing.T) {
	act := makeWfActivityWithOutcomes("Decision", "dec1")
	m := newMutator(makeWorkflowDoc(act))

	err := m.DropBranch("Decision", 0, "Missing")
	if err == nil {
		t.Fatal("Expected error for missing branch")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error = %q, want 'not found'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// InsertBoundaryEvent tests
// ---------------------------------------------------------------------------

func TestWorkflowMutator_InsertBoundaryEvent_InterruptingTimer(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act = append(act, bson.E{Key: "BoundaryEvents", Value: bson.A{int32(3)}})
	m := newMutator(makeWorkflowDoc(act))

	if err := m.InsertBoundaryEvent("Review", 0, "InterruptingTimer", "PT1H", nil); err != nil {
		t.Fatalf("InsertBoundaryEvent failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	events := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "BoundaryEvents"))
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	eDoc := events[0].(bson.D)
	if got := bsonnav.DGetString(eDoc, "$Type"); got != "Workflows$InterruptingTimerBoundaryEvent" {
		t.Errorf("Event $Type = %q, want InterruptingTimerBoundaryEvent", got)
	}
	if got := bsonnav.DGetString(eDoc, "FirstExecutionTime"); got != "PT1H" {
		t.Errorf("FirstExecutionTime = %q, want PT1H", got)
	}
}

func TestWorkflowMutator_InsertBoundaryEvent_NonInterruptingTimer(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act = append(act, bson.E{Key: "BoundaryEvents", Value: bson.A{int32(3)}})
	m := newMutator(makeWorkflowDoc(act))

	if err := m.InsertBoundaryEvent("Review", 0, "NonInterruptingTimer", "PT30M", nil); err != nil {
		t.Fatalf("InsertBoundaryEvent NonInterrupting failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	events := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "BoundaryEvents"))
	eDoc := events[0].(bson.D)
	if got := bsonnav.DGetString(eDoc, "$Type"); got != "Workflows$NonInterruptingTimerBoundaryEvent" {
		t.Errorf("Event $Type = %q, want NonInterruptingTimerBoundaryEvent", got)
	}
	// NonInterrupting should have Recurrence field
	found := false
	for _, e := range eDoc {
		if e.Key == "Recurrence" {
			found = true
		}
	}
	if !found {
		t.Error("Expected Recurrence field on NonInterruptingTimerBoundaryEvent")
	}
}

func TestWorkflowMutator_InsertBoundaryEvent_WithActivities(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act = append(act, bson.E{Key: "BoundaryEvents", Value: bson.A{int32(3)}})
	m := newMutator(makeWorkflowDoc(act))

	subAct := makeTestWorkflowActivity("evt_act", "EventAct")
	if err := m.InsertBoundaryEvent("Review", 0, "Timer", "", []workflows.WorkflowActivity{subAct}); err != nil {
		t.Fatalf("InsertBoundaryEvent with activities failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	events := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "BoundaryEvents"))
	eDoc := events[0].(bson.D)
	flow := bsonnav.DGetDoc(eDoc, "Flow")
	if flow == nil {
		t.Fatal("Expected Flow on boundary event with activities")
	}
}

func TestWorkflowMutator_InsertBoundaryEvent_NoDelay(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act = append(act, bson.E{Key: "BoundaryEvents", Value: bson.A{int32(3)}})
	m := newMutator(makeWorkflowDoc(act))

	if err := m.InsertBoundaryEvent("Review", 0, "Timer", "", nil); err != nil {
		t.Fatalf("InsertBoundaryEvent no delay failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	events := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "BoundaryEvents"))
	eDoc := events[0].(bson.D)
	for _, e := range eDoc {
		if e.Key == "FirstExecutionTime" {
			t.Error("FirstExecutionTime should not be present when delay is empty")
		}
	}
}

// ---------------------------------------------------------------------------
// SetActivityProperty — additional property types
// ---------------------------------------------------------------------------

func TestWorkflowMutator_SetActivityProperty_Page_New(t *testing.T) {
	// TaskPage key present with nil value — should be replaced with a new PageReference.
	act := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act = append(act, bson.E{Key: "TaskPage", Value: nil})
	m := newMutator(makeWorkflowDoc(act))

	if err := m.SetActivityProperty("Review", 0, "page", "MyModule.TaskPage"); err != nil {
		t.Fatalf("SetActivityProperty page failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	taskPage := bsonnav.DGetDoc(actDoc, "TaskPage")
	if taskPage == nil {
		t.Fatal("Expected TaskPage to be set")
	}
	if got := bsonnav.DGetString(taskPage, "Page"); got != "MyModule.TaskPage" {
		t.Errorf("Page = %q, want MyModule.TaskPage", got)
	}
}

func TestWorkflowMutator_SetActivityProperty_Page_MissingKey(t *testing.T) {
	// Regression test: dSet silently failed when TaskPage key was absent.
	// Fixed by appending the key to the activity and replacing it in the BSON tree.
	act := makeWfActivity("Workflows$UserTask", "Review", "task1")
	// No TaskPage field at all
	m := newMutator(makeWorkflowDoc(act))

	// No error returned, but the set is silently lost
	if err := m.SetActivityProperty("Review", 0, "page", "MyModule.TaskPage"); err != nil {
		t.Fatalf("SetActivityProperty page failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	taskPage := bsonnav.DGetDoc(actDoc, "TaskPage")
	if taskPage == nil {
		t.Fatal("TaskPage should be set even when key was absent")
	}
	if got := bsonnav.DGetString(taskPage, "Page"); got != "MyModule.TaskPage" {
		t.Errorf("Page = %q, want MyModule.TaskPage", got)
	}
}

func TestWorkflowMutator_SetActivityProperty_Page_MissingKey_NestedSubFlow(t *testing.T) {
	// Exercises the recursive replaceActivity path: the target activity lives
	// inside an outcome's sub-flow, not at the top level.
	// Use distinct $IDs so replaceActivity cannot accidentally match the parent.
	parentID := primitive.Binary{Subtype: 0x04, Data: []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}
	nestedID := primitive.Binary{Subtype: 0x04, Data: []byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}

	nestedAct := bson.D{
		{Key: "$ID", Value: nestedID},
		{Key: "$Type", Value: "Workflows$UserTask"},
		{Key: "Caption", Value: "NestedReview"},
		{Key: "Name", Value: "nested1"},
	}
	// No TaskPage field at all on the nested activity.

	outcome := bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$BooleanOutcome"},
		{Key: "Flow", Value: bson.D{
			{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
			{Key: "$Type", Value: "Workflows$Flow"},
			{Key: "Activities", Value: bson.A{int32(3), nestedAct}},
		}},
	}
	parentAct := bson.D{
		{Key: "$ID", Value: parentID},
		{Key: "$Type", Value: "Workflows$Decision"},
		{Key: "Caption", Value: "Check"},
		{Key: "Name", Value: "decision1"},
		{Key: "Outcomes", Value: bson.A{int32(3), outcome}},
	}
	m := newMutator(makeWorkflowDoc(parentAct))

	if err := m.SetActivityProperty("NestedReview", 0, "PAGE", "MyModule.NestedPage"); err != nil {
		t.Fatalf("SetActivityProperty PAGE on nested activity failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("NestedReview", 0)
	taskPage := bsonnav.DGetDoc(actDoc, "TaskPage")
	if taskPage == nil {
		t.Fatal("TaskPage should be set on nested activity even when key was absent")
	}
	if got := bsonnav.DGetString(taskPage, "Page"); got != "MyModule.NestedPage" {
		t.Errorf("Page = %q, want MyModule.NestedPage", got)
	}

	// Verify parent decision still has its Outcomes intact.
	parentDoc, _ := m.findActivityByCaption("Check", 0)
	if parentDoc == nil {
		t.Fatal("parent decision activity should still exist")
	}
	if outcomes := bsonnav.DGet(parentDoc, "Outcomes"); outcomes == nil {
		t.Fatal("parent decision Outcomes should still be present")
	}
}

func TestWorkflowMutator_SetActivityProperty_Page_Existing(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act = append(act, bson.E{Key: "TaskPage", Value: bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$PageReference"},
		{Key: "Page", Value: "OldModule.OldPage"},
	}})
	m := newMutator(makeWorkflowDoc(act))

	if err := m.SetActivityProperty("Review", 0, "page", "NewModule.NewPage"); err != nil {
		t.Fatalf("SetActivityProperty page update failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	taskPage := bsonnav.DGetDoc(actDoc, "TaskPage")
	if got := bsonnav.DGetString(taskPage, "Page"); got != "NewModule.NewPage" {
		t.Errorf("Page = %q, want NewModule.NewPage", got)
	}
}

func TestWorkflowMutator_SetActivityProperty_Description(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act = append(act, bson.E{Key: "TaskDescription", Value: bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Text", Value: "old"},
	}})
	m := newMutator(makeWorkflowDoc(act))

	if err := m.SetActivityProperty("Review", 0, "description", "new desc"); err != nil {
		t.Fatalf("SetActivityProperty description failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	taskDesc := bsonnav.DGetDoc(actDoc, "TaskDescription")
	if got := bsonnav.DGetString(taskDesc, "Text"); got != "new desc" {
		t.Errorf("Text = %q, want 'new desc'", got)
	}
}

func TestWorkflowMutator_SetActivityProperty_TargetingMicroflow(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act = append(act, bson.E{Key: "UserTargeting", Value: nil})
	m := newMutator(makeWorkflowDoc(act))

	if err := m.SetActivityProperty("Review", 0, "targeting_microflow", "MyModule.AssignReviewer"); err != nil {
		t.Fatalf("SetActivityProperty TARGETING_MICROFLOW failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	targeting := bsonnav.DGetDoc(actDoc, "UserTargeting")
	if targeting == nil {
		t.Fatal("Expected UserTargeting to be set")
	}
	if got := bsonnav.DGetString(targeting, "$Type"); got != "Workflows$MicroflowUserTargeting" {
		t.Errorf("$Type = %q, want MicroflowUserTargeting", got)
	}
	if got := bsonnav.DGetString(targeting, "Microflow"); got != "MyModule.AssignReviewer" {
		t.Errorf("Microflow = %q, want MyModule.AssignReviewer", got)
	}
}

func TestWorkflowMutator_SetActivityProperty_TargetingXPath(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act = append(act, bson.E{Key: "UserTargeting", Value: nil})
	m := newMutator(makeWorkflowDoc(act))

	if err := m.SetActivityProperty("Review", 0, "targeting_xpath", "[Role = 'Admin']"); err != nil {
		t.Fatalf("SetActivityProperty TARGETING_XPATH failed: %v", err)
	}

	actDoc, _ := m.findActivityByCaption("Review", 0)
	targeting := bsonnav.DGetDoc(actDoc, "UserTargeting")
	if targeting == nil {
		t.Fatal("Expected UserTargeting to be set")
	}
	if got := bsonnav.DGetString(targeting, "$Type"); got != "Workflows$XPathUserTargeting" {
		t.Errorf("$Type = %q, want XPathUserTargeting", got)
	}
}

// ---------------------------------------------------------------------------
// SetPropertyWithEntity tests
// ---------------------------------------------------------------------------

func TestWorkflowMutator_SetPropertyWithEntity_OverviewPage(t *testing.T) {
	doc := makeWorkflowDoc()
	doc = append(doc, bson.E{Key: "AdminPage", Value: nil})
	m := newMutator(doc)

	if err := m.SetPropertyWithEntity("overview_page", "MyModule.OverviewPage", ""); err != nil {
		t.Fatalf("SetPropertyWithEntity OVERVIEW_PAGE failed: %v", err)
	}

	adminPage := bsonnav.DGetDoc(m.rawData, "AdminPage")
	if adminPage == nil {
		t.Fatal("Expected AdminPage to be set")
	}
	if got := bsonnav.DGetString(adminPage, "Page"); got != "MyModule.OverviewPage" {
		t.Errorf("Page = %q, want MyModule.OverviewPage", got)
	}
}

func TestWorkflowMutator_SetPropertyWithEntity_OverviewPage_Clear(t *testing.T) {
	doc := makeWorkflowDoc()
	doc = append(doc, bson.E{Key: "AdminPage", Value: bson.D{
		{Key: "Page", Value: "OldPage"},
	}})
	m := newMutator(doc)

	if err := m.SetPropertyWithEntity("overview_page", "", ""); err != nil {
		t.Fatalf("SetPropertyWithEntity clear failed: %v", err)
	}

	if v := bsonnav.DGet(m.rawData, "AdminPage"); v != nil {
		t.Error("Expected AdminPage to be nil after clear")
	}
}

func TestWorkflowMutator_SetPropertyWithEntity_Parameter_New(t *testing.T) {
	doc := makeWorkflowDoc()
	doc = append(doc, bson.E{Key: "Parameter", Value: nil})
	m := newMutator(doc)

	if err := m.SetPropertyWithEntity("parameter", "WorkflowContext", "MyModule.Order"); err != nil {
		t.Fatalf("SetPropertyWithEntity parameter failed: %v", err)
	}

	param := bsonnav.DGetDoc(m.rawData, "Parameter")
	if param == nil {
		t.Fatal("Expected Parameter to be set")
	}
	if got := bsonnav.DGetString(param, "Entity"); got != "MyModule.Order" {
		t.Errorf("Entity = %q, want MyModule.Order", got)
	}
}

func TestWorkflowMutator_SetPropertyWithEntity_Parameter_Update(t *testing.T) {
	doc := makeWorkflowDoc()
	doc = append(doc, bson.E{Key: "Parameter", Value: bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$Parameter"},
		{Key: "Entity", Value: "OldModule.OldEntity"},
		{Key: "Name", Value: "WorkflowContext"},
	}})
	m := newMutator(doc)

	if err := m.SetPropertyWithEntity("parameter", "WorkflowContext", "NewModule.NewEntity"); err != nil {
		t.Fatalf("SetPropertyWithEntity parameter update failed: %v", err)
	}

	param := bsonnav.DGetDoc(m.rawData, "Parameter")
	if got := bsonnav.DGetString(param, "Entity"); got != "NewModule.NewEntity" {
		t.Errorf("Entity = %q, want NewModule.NewEntity", got)
	}
}

func TestWorkflowMutator_SetPropertyWithEntity_Parameter_Clear(t *testing.T) {
	doc := makeWorkflowDoc()
	doc = append(doc, bson.E{Key: "Parameter", Value: bson.D{
		{Key: "Entity", Value: "Something"},
	}})
	m := newMutator(doc)

	if err := m.SetPropertyWithEntity("parameter", "", ""); err != nil {
		t.Fatalf("SetPropertyWithEntity parameter clear failed: %v", err)
	}

	if v := bsonnav.DGet(m.rawData, "Parameter"); v != nil {
		t.Error("Expected Parameter to be nil after clear")
	}
}

func TestWorkflowMutator_SetPropertyWithEntity_Unsupported(t *testing.T) {
	m := newMutator(makeWorkflowDoc())
	err := m.SetPropertyWithEntity("INVALID", "x", "y")
	if err == nil {
		t.Fatal("Expected error for unsupported property")
	}
}
