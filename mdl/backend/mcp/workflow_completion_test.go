// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// The constructor fields are Studio Pro 11.14's (ped_get_schema) and the
// mapping is what it stored for ako/TestApp's ZzMxcliExample_Completion2.
func TestMapMultiUserTaskCompletion(t *testing.T) {
	outcomes := []any{"Approve", "Reject"}
	cases := []struct {
		name string
		task workflows.UserTask
		want map[string]any
	}{
		{"no rule sends consensus on the first outcome", workflows.UserTask{},
			map[string]any{"completionCriteria": "Consensus", "fallbackOutcome": "Approve"}},
		{"majority more than half", workflows.UserTask{CompletionCriteria: &workflows.CompletionCriteria{Kind: "Majority", CompletionType: "Absolute", FallbackOutcome: "Reject"}},
			map[string]any{"completionCriteria": "Majority", "majorityType": "MoreThanHalf", "fallbackOutcome": "Reject"}},
		{"majority most chosen", workflows.UserTask{CompletionCriteria: &workflows.CompletionCriteria{Kind: "Majority", CompletionType: "Relative", FallbackOutcome: "Approve"}},
			map[string]any{"completionCriteria": "Majority", "majorityType": "MostChosen", "fallbackOutcome": "Approve"}},
		{"threshold percent with percentage participants", workflows.UserTask{
			CompletionCriteria: &workflows.CompletionCriteria{Kind: "Threshold", CompletionType: "Relative", Threshold: 60, FallbackOutcome: "Reject"},
			TargetUserInput:    &workflows.TargetUserInput{Kind: "Percentage", Percentage: 80}},
			map[string]any{"completionCriteria": "Threshold", "thresholdType": "Percentage", "thresholdValue": 60, "fallbackOutcome": "Reject", "participiantInput": "Percentage", "participiantValue": 80}},
		{"threshold votes with a participant count", workflows.UserTask{
			CompletionCriteria: &workflows.CompletionCriteria{Kind: "Threshold", CompletionType: "Absolute", Threshold: 2, FallbackOutcome: "Reject"},
			TargetUserInput:    &workflows.TargetUserInput{Kind: "Absolute", Amount: 3}},
			map[string]any{"completionCriteria": "Threshold", "thresholdType": "AbsoluteNumber", "thresholdValue": 2, "fallbackOutcome": "Reject", "participiantInput": "AbsoluteNumber", "participiantValue": 3}},
		{"veto waiting for all users", workflows.UserTask{CompletionCriteria: &workflows.CompletionCriteria{Kind: "Veto", VetoOutcome: "Reject"}, AwaitAllUsers: true},
			map[string]any{"completionCriteria": "Veto", "vetoOutcome": "Reject", "awaitAllUsers": true}},
		{"microflow", workflows.UserTask{CompletionCriteria: &workflows.CompletionCriteria{Kind: "Microflow", Microflow: "M.Decide"}},
			map[string]any{"completionCriteria": "Microflow", "microflow": "M.Decide"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := map[string]any{"participiantInput": "AllTargetUsers"}
			task := c.task
			mapMultiUserTaskCompletion(m, &task, outcomes)
			if _, ok := c.want["participiantInput"]; !ok {
				c.want["participiantInput"] = "AllTargetUsers"
			}
			if !reflect.DeepEqual(m, c.want) {
				t.Errorf("mapped %v\nwant   %v", m, c.want)
			}
		})
	}
}

func majorityHalfTask(name, fallback string) *workflows.UserTask {
	task := &workflows.UserTask{
		IsMulti:            true,
		Outcomes:           []*workflows.UserTaskOutcome{{Value: "Approve"}, {Value: "Reject"}},
		CompletionCriteria: &workflows.CompletionCriteria{Kind: "Majority", CompletionType: "Absolute", FallbackOutcome: fallback},
	}
	task.Name = name
	return task
}

// PED's constructor drops the fallback of a more-than-half majority (measured,
// Studio Pro 11.14), so every such task — at any depth — is located by JSON path
// with the index of its fallback outcome.
func TestMoreThanHalfFallbackPaths(t *testing.T) {
	nested := majorityHalfTask("inner", "Approve")
	call := &workflows.CallMicroflowTask{Microflow: "M.Do", Outcomes: []workflows.ConditionOutcome{
		&workflows.BooleanConditionOutcome{Value: true, Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{nested}}},
		&workflows.BooleanConditionOutcome{Value: false, Flow: &workflows.Flow{}},
	}}
	mostChosen := majorityHalfTask("most", "Reject")
	mostChosen.CompletionCriteria.CompletionType = "Relative"
	flow := &workflows.Flow{Activities: []workflows.WorkflowActivity{
		&workflows.StartWorkflowActivity{},
		majorityHalfTask("top", "Reject"),
		call,
		mostChosen,
	}}
	got := moreThanHalfFallbacks(flow, "/flow/activities")
	want := map[string]int{
		"/flow/activities/1":                              1,
		"/flow/activities/2/outcomes/0/flow/activities/0": 0,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paths = %v, want %v", got, want)
	}
}

// After a create, the fallback is read back as the outcome's $ID and set.
func TestApplyMoreThanHalfFallbacks(t *testing.T) {
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		if name == "ped_read_document" {
			paths, _ := args["paths"].([]any)
			if len(paths) == 1 && paths[0] == "/flow/activities/1/outcomes/1/$ID" {
				return `{"results":[{"path":"/flow/activities/1/outcomes/1/$ID","result":"4d2c6f1e-0000-4000-8000-000000000001"}]}`, false
			}
			return `{"results":[{"path":"?","result":""}]}`, false
		}
		return "SUCCESS", false
	})
	b := &Backend{client: f.connectClient(t)}
	wf := &workflows.Workflow{Name: "W", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
		&workflows.StartWorkflowActivity{}, majorityHalfTask("top", "Reject"),
	}}}
	if err := b.applyMoreThanHalfFallbacks("M.W", wf); err != nil {
		t.Fatalf("applyMoreThanHalfFallbacks: %v", err)
	}
	call, ok := f.callByName("ped_update_document")
	if !ok {
		t.Fatal("no ped_update_document sent")
	}
	raw, _ := json.Marshal(call.Args["operations"])
	for _, want := range []string{`"path":"/flow/activities/1/completionCriteria/fallbackOutcome"`, `"value":"4d2c6f1e-0000-4000-8000-000000000001"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("update lacks %s: %s", want, raw)
		}
	}
}
