// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The notify action maps onto ped_get_schema's NotifyWorkflowAction (Studio Pro
// 11.14): its target reference sits under `activity`, or `boundaryEvent` for a
// boundary-event target. It was unmapped, so a notify could not be written over
// MCP at all.
func TestMapMicroflowAction_NotifyWorkflow(t *testing.T) {
	cases := []struct {
		target microflows.NotifyTarget
		want   map[string]any
	}{
		{microflows.NotifyTarget{TypeName: "Workflows$NotifyNotificationActivityTarget", Name: "M.W.received"},
			map[string]any{"$Type": "Workflows$NotifyNotificationActivityTarget", "activity": "M.W.received"}},
		{microflows.NotifyTarget{TypeName: "Workflows$NotifyNotificationBoundaryEventTarget", Name: "M.W.withdrawn"},
			map[string]any{"$Type": "Workflows$NotifyNotificationBoundaryEventTarget", "boundaryEvent": "M.W.withdrawn"}},
	}
	for _, c := range cases {
		target := c.target
		m, err := mapMicroflowAction(&microflows.NotifyWorkflowAction{WorkflowVariable: "Workflow", OutputVariableName: "Notified", Target: &target})
		if err != nil {
			t.Fatal(err)
		}
		if m["$Type"] != "Microflows$NotifyWorkflowAction" || m["workflowVariable"] != "Workflow" || m["outputVariableName"] != "Notified" {
			t.Errorf("action mapped as %v", m)
		}
		if got, _ := m["notifyTarget"].(map[string]any); !reflect.DeepEqual(got, c.want) {
			t.Errorf("notifyTarget = %v, want %v", got, c.want)
		}
	}
}
