// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
)

func TestJavaActionAuthoringRejected(t *testing.T) {
	b := &Backend{}
	ja := &javaactions.JavaAction{Name: "JA_DoThing"}
	for _, err := range []error{b.CreateJavaAction(ja), b.UpdateJavaAction(ja)} {
		if err == nil || !strings.Contains(err.Error(), "not authorable") {
			t.Errorf("java action authoring should be rejected with a clear error, got %v", err)
		}
	}
}
