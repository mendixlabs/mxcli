// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
	"go.mongodb.org/mongo-driver/bson"
)

// loadWorkflowBSON loads a workflow BSON fixture from testdata/workflows/<name>.bson.
func loadWorkflowBSON(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "workflows", name+".bson"))
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal fixture %s: %v", name, err)
	}
	return raw
}

// workflowActivities returns the activity slice (skipping the array marker) from a flow map.
func workflowActivities(t *testing.T, flowRaw map[string]any) []map[string]any {
	t.Helper()
	arr, ok := flowRaw["Activities"].(bson.A)
	if !ok {
		t.Fatalf("Activities is not bson.A, got %T", flowRaw["Activities"])
	}
	var acts []map[string]any
	for _, item := range arr[1:] { // skip marker at index 0
		m := toMap(item)
		if m != nil {
			acts = append(acts, m)
		}
	}
	return acts
}

func TestParseWorkflowParameter_FromFixture(t *testing.T) {
	raw := loadWorkflowBSON(t, "WorkflowBaseline.Workflow")
	paramRaw := toMap(raw["Parameter"])
	if paramRaw == nil {
		t.Fatal("fixture has no Parameter")
	}

	param := parseWorkflowParameter(paramRaw)
	if param == nil {
		t.Fatal("parseWorkflowParameter returned nil")
	}
	if param.EntityRef != "WorkflowBaseline.Entity" {
		t.Errorf("EntityRef = %q, want %q", param.EntityRef, "WorkflowBaseline.Entity")
	}
}

func TestParseWorkflowFlow_FromFixture_ActivityCount(t *testing.T) {
	raw := loadWorkflowBSON(t, "WorkflowBaseline.Workflow")
	flowRaw := toMap(raw["Flow"])
	if flowRaw == nil {
		t.Fatal("fixture has no Flow")
	}

	flow := parseWorkflowFlow(flowRaw)
	if flow == nil {
		t.Fatal("parseWorkflowFlow returned nil")
	}
	// Fixture has: Start, SingleUserTask, MultiUserTask, CallMicroflow, ParallelSplit, ExclusiveSplit, End
	if len(flow.Activities) != 7 {
		t.Errorf("len(Activities) = %d, want 7", len(flow.Activities))
	}
}

func TestParseWorkflowActivity_FromFixture_StartIsFirst(t *testing.T) {
	raw := loadWorkflowBSON(t, "WorkflowBaseline.Workflow")
	flowRaw := toMap(raw["Flow"])
	acts := workflowActivities(t, flowRaw)

	activity := parseWorkflowActivity(acts[0])
	if _, ok := activity.(*workflows.StartWorkflowActivity); !ok {
		t.Errorf("activities[0] = %T, want *workflows.StartWorkflowActivity", activity)
	}
}

func TestParseWorkflowActivity_FromFixture_UserTask(t *testing.T) {
	raw := loadWorkflowBSON(t, "WorkflowBaseline.Workflow")
	flowRaw := toMap(raw["Flow"])
	acts := workflowActivities(t, flowRaw)

	// activities[1] is SingleUserTaskActivity in fixture
	activity := parseWorkflowActivity(acts[1])
	userTask, ok := activity.(*workflows.UserTask)
	if !ok {
		t.Fatalf("activities[1] = %T, want *workflows.UserTask", activity)
	}
	if userTask.Name != "userTask1" {
		t.Errorf("Name = %q, want %q", userTask.Name, "userTask1")
	}
}

func TestParseWorkflowActivity_FromFixture_CallMicroflow(t *testing.T) {
	raw := loadWorkflowBSON(t, "WorkflowBaseline.Workflow")
	flowRaw := toMap(raw["Flow"])
	acts := workflowActivities(t, flowRaw)

	// activities[3] is CallMicroflowTask in fixture
	activity := parseWorkflowActivity(acts[3])
	callMf, ok := activity.(*workflows.CallMicroflowTask)
	if !ok {
		t.Fatalf("activities[3] = %T, want *workflows.CallMicroflowTask", activity)
	}
	if callMf.Microflow != "WorkflowBaseline.Microflow" {
		t.Errorf("Microflow = %q, want %q", callMf.Microflow, "WorkflowBaseline.Microflow")
	}
}

func TestParseWorkflowActivity_FromFixture_EndIsLast(t *testing.T) {
	raw := loadWorkflowBSON(t, "WorkflowBaseline.Workflow")
	flowRaw := toMap(raw["Flow"])
	acts := workflowActivities(t, flowRaw)

	last := parseWorkflowActivity(acts[len(acts)-1])
	if _, ok := last.(*workflows.EndWorkflowActivity); !ok {
		t.Errorf("last activity = %T, want *workflows.EndWorkflowActivity", last)
	}
}

func TestParseUserTaskOutcome_FromFixture(t *testing.T) {
	raw := loadWorkflowBSON(t, "WorkflowBaseline.Workflow")
	flowRaw := toMap(raw["Flow"])
	acts := workflowActivities(t, flowRaw)

	// activities[1] is SingleUserTaskActivity with one outcome
	outcomesRaw := acts[1]["Outcomes"]
	arr, ok := outcomesRaw.(bson.A)
	if !ok || len(arr) < 2 {
		t.Fatalf("expected Outcomes array with marker+1 element, got %T len=%d", outcomesRaw, len(arr))
	}
	outcomeMap := toMap(arr[1]) // skip marker
	if outcomeMap == nil {
		t.Fatal("outcome element is nil")
	}

	outcome := parseUserTaskOutcome(outcomeMap)
	if outcome == nil {
		t.Fatal("parseUserTaskOutcome returned nil")
	}
	if outcome.Value != "Outcome" {
		t.Errorf("Value = %q, want %q", outcome.Value, "Outcome")
	}
}

func TestParseParameterMappings_FromFixture(t *testing.T) {
	raw := loadWorkflowBSON(t, "WorkflowBaseline.Workflow")
	flowRaw := toMap(raw["Flow"])
	acts := workflowActivities(t, flowRaw)

	// activities[3] is CallMicroflowTask with 1 parameter mapping
	mappingsRaw := acts[3]["ParameterMappings"]
	mappings := parseParameterMappings(mappingsRaw)
	if len(mappings) != 1 {
		t.Fatalf("len(mappings) = %d, want 1", len(mappings))
	}
}

func TestParseWorkflowFlow_FromFixture_SubWorkflow(t *testing.T) {
	raw := loadWorkflowBSON(t, "WorkflowBaseline.Sub_Workflow")
	flowRaw := toMap(raw["Flow"])
	if flowRaw == nil {
		t.Fatal("Sub_Workflow fixture has no Flow")
	}

	flow := parseWorkflowFlow(flowRaw)
	if flow == nil {
		t.Fatal("parseWorkflowFlow returned nil")
	}
	if len(flow.Activities) != 2 {
		t.Errorf("Sub_Workflow has %d activities, want exactly 2 (Start+End)", len(flow.Activities))
	}
}

func TestParseWorkflowParameter_Nil(t *testing.T) {
	param := parseWorkflowParameter(nil)
	if param != nil {
		t.Errorf("parseWorkflowParameter(nil) = %v, want nil", param)
	}
}

func TestParseWorkflowActivity_UnknownType(t *testing.T) {
	raw := map[string]any{
		"$Type": "Workflows$SomeUnknownFutureActivity",
		"$ID":   "abc123",
		"Name":  "mystery",
	}
	activity := parseWorkflowActivity(raw)
	generic, ok := activity.(*workflows.GenericWorkflowActivity)
	if !ok {
		t.Fatalf("unknown type = %T, want *workflows.GenericWorkflowActivity", activity)
	}
	if generic.Name != "mystery" {
		t.Errorf("Name = %q, want %q", generic.Name, "mystery")
	}
}

func TestParseUserTask_UserTargeting_XPath(t *testing.T) {
	raw := map[string]any{
		"$Type":   "Workflows$SingleUserTaskActivity",
		"$ID":     "ut-001",
		"Name":    "reviewTask",
		"Caption": "Review Request",
		"UserTargeting": map[string]any{
			"$Type":           "Workflows$XPathUserTargeting",
			"$ID":             "tgt-001",
			"XPathConstraint": "[System.UserRoles = '[%UserRole_Manager%]']",
		},
	}
	task := parseUserTask(raw)
	if task == nil {
		t.Fatal("parseUserTask returned nil")
	}
	xpathSource, ok := task.UserSource.(*workflows.XPathBasedUserSource)
	if !ok {
		t.Fatalf("UserSource = %T, want *workflows.XPathBasedUserSource", task.UserSource)
	}
	if xpathSource.XPath != "[System.UserRoles = '[%UserRole_Manager%]']" {
		t.Errorf("XPath = %q, want %q", xpathSource.XPath, "[System.UserRoles = '[%UserRole_Manager%]']")
	}
}

func TestParseUserTask_UserTargeting_Microflow(t *testing.T) {
	raw := map[string]any{
		"$Type":   "Workflows$SingleUserTaskActivity",
		"$ID":     "ut-002",
		"Name":    "approvalTask",
		"Caption": "Approval",
		"UserTargeting": map[string]any{
			"$Type":     "Workflows$MicroflowUserTargeting",
			"$ID":       "tgt-002",
			"Microflow": "MyModule.GetTargetUsers",
		},
	}
	task := parseUserTask(raw)
	if task == nil {
		t.Fatal("parseUserTask returned nil")
	}
	mfSource, ok := task.UserSource.(*workflows.MicroflowBasedUserSource)
	if !ok {
		t.Fatalf("UserSource = %T, want *workflows.MicroflowBasedUserSource", task.UserSource)
	}
	if mfSource.Microflow != "MyModule.GetTargetUsers" {
		t.Errorf("Microflow = %q, want %q", mfSource.Microflow, "MyModule.GetTargetUsers")
	}
}

func TestParseUserTask_UserTargeting_NoTargeting(t *testing.T) {
	raw := map[string]any{
		"$Type":   "Workflows$SingleUserTaskActivity",
		"$ID":     "ut-003",
		"Name":    "simpleTask",
		"Caption": "Simple",
		"UserTargeting": map[string]any{
			"$Type": "Workflows$NoUserTargeting",
			"$ID":   "tgt-003",
		},
	}
	task := parseUserTask(raw)
	if task == nil {
		t.Fatal("parseUserTask returned nil")
	}
	if _, ok := task.UserSource.(*workflows.NoUserSource); !ok {
		t.Errorf("UserSource = %T, want *workflows.NoUserSource", task.UserSource)
	}
}

func TestParseUserTask_UserTargeting_GroupMicroflow(t *testing.T) {
	raw := map[string]any{
		"$Type":   "Workflows$SingleUserTaskActivity",
		"$ID":     "ut-005",
		"Name":    "groupTask",
		"Caption": "Group Review",
		"UserTargeting": map[string]any{
			"$Type":     "Workflows$MicroflowGroupTargeting",
			"$ID":       "tgt-005",
			"Microflow": "MyModule.GetTargetGroups",
		},
	}
	task := parseUserTask(raw)
	if task == nil {
		t.Fatal("parseUserTask returned nil")
	}
	groupSource, ok := task.UserSource.(*workflows.MicroflowGroupSource)
	if !ok {
		t.Fatalf("UserSource = %T, want *workflows.MicroflowGroupSource", task.UserSource)
	}
	if groupSource.Microflow != "MyModule.GetTargetGroups" {
		t.Errorf("Microflow = %q, want %q", groupSource.Microflow, "MyModule.GetTargetGroups")
	}
}

func TestParseUserTask_UserTargeting_GroupXPath(t *testing.T) {
	raw := map[string]any{
		"$Type":   "Workflows$SingleUserTaskActivity",
		"$ID":     "ut-006",
		"Name":    "groupXPathTask",
		"Caption": "Group XPath Review",
		"UserTargeting": map[string]any{
			"$Type":           "Workflows$XPathGroupTargeting",
			"$ID":             "tgt-006",
			"XPathConstraint": "[GroupType = 'Reviewers']",
		},
	}
	task := parseUserTask(raw)
	if task == nil {
		t.Fatal("parseUserTask returned nil")
	}
	groupSource, ok := task.UserSource.(*workflows.XPathGroupSource)
	if !ok {
		t.Fatalf("UserSource = %T, want *workflows.XPathGroupSource", task.UserSource)
	}
	if groupSource.XPath != "[GroupType = 'Reviewers']" {
		t.Errorf("XPath = %q, want %q", groupSource.XPath, "[GroupType = 'Reviewers']")
	}
}

func TestParseUserTask_LegacyUserSource_StillWorks(t *testing.T) {
	raw := map[string]any{
		"$Type":   "Workflows$SingleUserTaskActivity",
		"$ID":     "ut-004",
		"Name":    "legacyTask",
		"Caption": "Legacy",
		"UserSource": map[string]any{
			"$Type":     "Workflows$MicroflowBasedUserSource",
			"$ID":       "src-001",
			"Microflow": "OldModule.OldMicroflow",
		},
	}
	task := parseUserTask(raw)
	if task == nil {
		t.Fatal("parseUserTask returned nil")
	}
	mfSource, ok := task.UserSource.(*workflows.MicroflowBasedUserSource)
	if !ok {
		t.Fatalf("UserSource = %T, want *workflows.MicroflowBasedUserSource", task.UserSource)
	}
	if mfSource.Microflow != "OldModule.OldMicroflow" {
		t.Errorf("Microflow = %q, want %q", mfSource.Microflow, "OldModule.OldMicroflow")
	}
}

func TestParseBoundaryEvents_EmptyArray(t *testing.T) {
	// nil input
	events := parseBoundaryEvents(nil)
	if len(events) != 0 {
		t.Errorf("parseBoundaryEvents(nil) len = %d, want 0", len(events))
	}
	// marker-only array (bson.A with just the int32 marker)
	events = parseBoundaryEvents(bson.A{int32(2)})
	if len(events) != 0 {
		t.Errorf("parseBoundaryEvents(marker-only) len = %d, want 0", len(events))
	}
}

func TestParseBoundaryEvents_TimerEvent(t *testing.T) {
	eventMap := map[string]any{
		"$Type":              "Workflows$InterruptingTimerBoundaryEvent",
		"$ID":                "be-001",
		"Caption":            "Timeout",
		"FirstExecutionTime": "PT1H",
	}
	events := parseBoundaryEvents(bson.A{int32(2), eventMap})
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.EventType != "InterruptingTimer" {
		t.Errorf("EventType = %q, want %q", ev.EventType, "InterruptingTimer")
	}
	if ev.TimerDelay != "PT1H" {
		t.Errorf("TimerDelay = %q, want %q", ev.TimerDelay, "PT1H")
	}
	if ev.Caption != "Timeout" {
		t.Errorf("Caption = %q, want %q", ev.Caption, "Timeout")
	}
}
