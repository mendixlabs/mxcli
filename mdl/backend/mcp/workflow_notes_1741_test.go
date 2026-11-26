// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// A `create workflow` over MCP keeps its notes (ako/mxcli#1741): each
// activity's goes in the constructor's `annotation`, which PED's
// workflowActivityArgs declares for every activity and turns into a
// Workflows$Annotation. The start's note — the header `annotation '…'` — is the
// start activity's. Before, no case passed one and the workflow was created
// with every note gone, reporting success.
func TestMapWorkflowContent_PassesActivityNotes_1741(t *testing.T) {
	start := &workflows.StartWorkflowActivity{BaseWorkflowActivity: wfBase("Start", "Start")}
	start.Annotation = "Starts when an order is placed"
	notify := &workflows.NotificationActivity{BaseWorkflowActivity: wfBase("received", "Documents received")}
	notify.Annotation = "Wakes the reviewer"
	plain := &workflows.NotificationActivity{BaseWorkflowActivity: wfBase("plain", "No note")}
	inSplit := &workflows.NotificationActivity{BaseWorkflowActivity: wfBase("inSplit", "In a split")}
	inSplit.Annotation = "Nested"
	wf := &workflows.Workflow{Name: "W", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
		start, notify, plain,
		&workflows.ParallelSplitActivity{BaseWorkflowActivity: wfBase("split", "split"),
			Outcomes: []*workflows.ParallelSplitOutcome{{Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{inSplit}}}}},
		&workflows.EndWorkflowActivity{BaseWorkflowActivity: wfBase("End", "End")},
	}}}
	content, err := (&Backend{}).mapWorkflowContent(wf, true)
	if err != nil {
		t.Fatal(err)
	}
	notes := map[string]any{}
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if name, ok := n["name"].(string); ok {
				if note, ok := n["annotation"]; ok {
					notes[name] = note
				}
			}
			for _, c := range n {
				walk(c)
			}
		case []any:
			for _, c := range n {
				walk(c)
			}
		}
	}
	walk(content)
	for name, want := range map[string]string{"Start": "Starts when an order is placed", "received": "Wakes the reviewer", "inSplit": "Nested"} {
		if notes[name] != want {
			t.Errorf("%s annotation = %v, want %q", name, notes[name], want)
		}
	}
	if _, ok := notes["plain"]; ok {
		t.Errorf("an activity with no note was sent an annotation: %v", notes["plain"])
	}
}

// PED's EventSubProcess constructor has no annotation: a note on one is refused,
// naming it, instead of dropped.
func TestMapWorkflowContent_RefusesEventSubProcessNote_1741(t *testing.T) {
	wf := &workflows.Workflow{Name: "W",
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{&workflows.EndWorkflowActivity{BaseWorkflowActivity: wfBase("End", "End")}}},
		EventSubProcesses: []*workflows.EventSubProcess{{Name: "ESP_Expire", Caption: "Expire", Annotation: "Cleans up",
			Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
				&workflows.EventSubProcessStartActivity{BaseWorkflowActivity: wfBase("s", "s"), Interrupting: true, Timer: true, FirstExecutionTime: "addDays([%CurrentDateTime%], 1)"},
				&workflows.EndWorkflowActivity{BaseWorkflowActivity: wfBase("End2", "End")},
			}}}},
	}
	_, err := (&Backend{}).mapWorkflowContent(wf, true)
	if err == nil || !strings.Contains(err.Error(), "ESP_Expire") || !strings.Contains(err.Error(), "note") {
		t.Fatalf("err = %v, want a refusal naming the event sub-process's note", err)
	}
}
