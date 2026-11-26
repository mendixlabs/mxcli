// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Mendix 11.17 names timer boundary events (boundaryEvent1, …) and MDL cannot
// state the name, so `create or modify` carries the stored one; and a
// boundary path's implicit End keeps its stored name even when the activity
// also holds an outcome the rebuild does not (a call microflow's void outcome),
// which used to throw the pairing of its flows off (ako/mxcli#2616).
func TestCarryWorkflowNames_BoundaryEvents_2616(t *testing.T) {
	end := func(name string) *workflows.EndWorkflowActivity {
		e := &workflows.EndWorkflowActivity{}
		e.Name = name
		return e
	}
	timer := func(name string, path ...workflows.WorkflowActivity) *workflows.BoundaryEvent {
		return &workflows.BoundaryEvent{Name: name, EventType: "InterruptingTimer", Flow: &workflows.Flow{Activities: path}}
	}
	stored := &workflows.CallMicroflowTask{
		Outcomes:       []workflows.ConditionOutcome{&workflows.VoidConditionOutcome{Flow: &workflows.Flow{}}},
		BoundaryEvents: []*workflows.BoundaryEvent{timer("boundaryEvent7", end("End3"))},
	}
	stored.Name = "WF_Act"
	declared := &workflows.CallMicroflowTask{
		BoundaryEvents: []*workflows.BoundaryEvent{timer("", end("End"))},
	}
	declared.Name = "WF_Act"

	carryWorkflowActivityNames([]workflows.WorkflowActivity{declared}, []workflows.WorkflowActivity{stored})
	be := declared.BoundaryEvents[0]
	if be.Name != "boundaryEvent7" {
		t.Errorf("timer name = %q, want the stored boundaryEvent7", be.Name)
	}
	if got := be.Flow.Activities[0].GetName(); got != "End3" {
		t.Errorf("boundary path End = %q, want the stored End3 (the void outcome must not break the pairing)", got)
	}
}
