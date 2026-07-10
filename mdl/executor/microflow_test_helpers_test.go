// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// --- Helper constructors ---

func mkID(s string) model.ID { return model.ID(s) }

func mkObj(id string) microflows.BaseMicroflowObject {
	return microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: mkID(id)},
	}
}

func mkFlow(origin, dest string) *microflows.SequenceFlow {
	return &microflows.SequenceFlow{
		OriginID:      mkID(origin),
		DestinationID: mkID(dest),
	}
}

func mkErrorFlow(origin, dest string) *microflows.SequenceFlow {
	return &microflows.SequenceFlow{
		OriginID:       mkID(origin),
		DestinationID:  mkID(dest),
		IsErrorHandler: true,
	}
}

func mkBranchFlow(origin, dest string, cv microflows.CaseValue) *microflows.SequenceFlow {
	return &microflows.SequenceFlow{
		OriginID:      mkID(origin),
		DestinationID: mkID(dest),
		CaseValue:     cv,
	}
}

func newTestExecutor() *Executor {
	return New(&bytes.Buffer{})
}

// --- Test assertion helpers ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !contains(got, want) {
		t.Errorf("expected %q to contain %q", got, want)
	}
}
