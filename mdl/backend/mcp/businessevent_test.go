// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestBusinessEventServiceAuthoringRejected(t *testing.T) {
	b := &Backend{}
	svc := &model.BusinessEventService{Name: "Svc"}
	for _, err := range []error{b.CreateBusinessEventService(svc), b.UpdateBusinessEventService(svc)} {
		if err == nil || !strings.Contains(err.Error(), "not authorable") {
			t.Errorf("business event service authoring should be rejected with a clear error, got %v", err)
		}
	}
}
