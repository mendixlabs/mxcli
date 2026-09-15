// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// An AI agent task is written as Workflows$AIAgentTaskActivity in the shape
// ako/TestApp (Studio Pro 11.14.0) stores — Outcomes marker 3, ParameterMappings
// and BoundaryEvents marker 2 — and reads back as an agent task, not as the
// call-microflow activity it resembles.
func TestCreateWorkflow_AIAgentTaskRoundTrip(t *testing.T) {
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

	agent := &workflows.CallMicroflowTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "aiAgentTask1", Caption: "AI Agent Task"},
		IsAgent:              true,
		Microflow:            "MyFirstModule.InvokeAgent",
		Outcomes:             []workflows.ConditionOutcome{&workflows.VoidConditionOutcome{Flow: &workflows.Flow{}}},
		ParameterMappings:    []*workflows.ParameterMapping{{Parameter: "MyFirstModule.InvokeAgent.Ctx", Expression: "$WorkflowContext"}},
	}
	plain := &workflows.CallMicroflowTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "plain", Caption: "Plain"},
		Microflow:            "MyFirstModule.ACT_Plain",
	}
	wf := &workflows.Workflow{
		ContainerID: mod.ID,
		Name:        "ZzAgent",
		Parameter:   &workflows.WorkflowParameter{EntityRef: "MyFirstModule.Ctx"},
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.StartWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Start"}},
			agent,
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
		if w.Name == "ZzAgent" {
			got = w
		}
	}
	if got == nil {
		t.Fatal("workflow not found after reconnect")
	}
	kinds := map[string]bool{}
	for _, a := range got.Flow.Activities {
		if cm, ok := a.(*workflows.CallMicroflowTask); ok {
			kinds[cm.Name] = cm.IsAgent
			if cm.Name == "aiAgentTask1" && (cm.Microflow != "MyFirstModule.InvokeAgent" || len(cm.ParameterMappings) != 1 || len(cm.Outcomes) != 1) {
				t.Errorf("agent task read back as %+v", cm)
			}
		}
	}
	if !kinds["aiAgentTask1"] || kinds["plain"] {
		t.Errorf("IsAgent after read = %v, want aiAgentTask1 true and plain false", kinds)
	}

	raw, err := b2.GetRawUnit(got.ID)
	if err != nil {
		t.Fatalf("GetRawUnit: %v", err)
	}
	flow, _ := raw["Flow"].(map[string]any)
	acts, _ := flow["Activities"].([]any)
	var stored map[string]any
	for _, a := range acts {
		if m, ok := a.(map[string]any); ok && m["Name"] == "aiAgentTask1" {
			stored = m
		}
	}
	if stored == nil {
		t.Fatal("agent task not in the stored flow")
	}
	if stored["$Type"] != "Workflows$AIAgentTaskActivity" {
		t.Errorf("$Type = %v, want Workflows$AIAgentTaskActivity", stored["$Type"])
	}
	for key, want := range map[string]string{"Outcomes": "3", "ParameterMappings": "2", "BoundaryEvents": "2"} {
		l, _ := stored[key].([]any)
		if len(l) == 0 || fmt.Sprint(l[0]) != want {
			t.Errorf("%s = %v, want marker %s", key, stored[key], want)
		}
	}
}
