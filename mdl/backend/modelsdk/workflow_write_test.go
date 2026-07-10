// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

// TestCreateWorkflow_RoundTrip creates a workflow with a user task (one outcome)
// and a microflow task in the flow, then confirms it round-trips through
// ListWorkflows (name, parameter entity, and the activity tree).
func TestCreateWorkflow_RoundTrip(t *testing.T) {
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

	userTask := &workflows.UserTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Review", Caption: "Review"},
		Page:                 "MyFirstModule.ReviewPage",
		Outcomes:             []*workflows.UserTaskOutcome{{Value: "Approved"}},
	}
	mfTask := &workflows.CallMicroflowTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Notify", Caption: "Notify"},
		Microflow:            "MyFirstModule.ACT_Notify",
	}
	wf := &workflows.Workflow{
		ContainerID:  mod.ID,
		Name:         "ZzFlow",
		WorkflowName: "Zz Flow",
		Parameter:    &workflows.WorkflowParameter{EntityRef: "MyFirstModule.Ctx"},
		Flow: &workflows.Flow{
			Activities: []workflows.WorkflowActivity{
				&workflows.StartWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Start"}},
				userTask,
				mfTask,
				&workflows.EndWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "End"}},
			},
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
		t.Fatalf("ListWorkflows: %v", err)
	}
	var got *workflows.Workflow
	for _, w := range all {
		if w.Name == "ZzFlow" {
			got = w
			break
		}
	}
	if got == nil {
		t.Fatalf("workflow ZzFlow not found after create")
	}
	if got.Parameter == nil || got.Parameter.EntityRef != "MyFirstModule.Ctx" {
		t.Errorf("parameter entity not round-tripped: %+v", got.Parameter)
	}
	if got.Flow == nil || len(got.Flow.Activities) != 4 {
		t.Fatalf("flow activities = %d, want 4", len(activitiesOf(got)))
	}
}

func activitiesOf(w *workflows.Workflow) []workflows.WorkflowActivity {
	if w.Flow == nil {
		return nil
	}
	return w.Flow.Activities
}
