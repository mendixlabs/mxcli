// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// The legacy engine is being retired, but it must not turn an AI agent task into
// a plain call microflow on the way through: same document shape, agent $Type.
func TestWorkflowAgentTask_LegacySerializeAndParse(t *testing.T) {
	doc := serializeCallMicroflowTask(&workflows.CallMicroflowTask{IsAgent: true, Microflow: "M.InvokeAgent"})
	if got := getBSONField(doc, "$Type"); got != "Workflows$AIAgentTaskActivity" {
		t.Errorf("$Type = %v, want Workflows$AIAgentTaskActivity", got)
	}
	plain := serializeCallMicroflowTask(&workflows.CallMicroflowTask{Microflow: "M.Plain"})
	if got := getBSONField(plain, "$Type"); got != "Workflows$CallMicroflowTask" {
		t.Errorf("plain $Type = %v", got)
	}

	act := parseWorkflowActivity(map[string]any{
		"$Type":     "Workflows$AIAgentTaskActivity",
		"Name":      "aiAgentTask1",
		"Microflow": "M.InvokeAgent",
	})
	cm, ok := act.(*workflows.CallMicroflowTask)
	if !ok || !cm.IsAgent || cm.Microflow != "M.InvokeAgent" {
		t.Errorf("parsed = %#v", act)
	}
}
