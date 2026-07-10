// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestActionFromGen_WebServiceCall_Raw guards the legacy SOAP CALL WEB SERVICE
// action. When the action carries a field the structured describe form can't
// represent (e.g. HttpConfiguration), it must surface RawBSON so the renderer
// emits `call web service raw '<base64>'` rather than "-- Empty action".
func TestActionFromGen_WebServiceCall_Raw(t *testing.T) {
	act := decodeAction(t, bson.D{
		{Key: "$ID", Value: "ws-1"},
		{Key: "$Type", Value: "Microflows$CallWebServiceAction"},
		{Key: "ErrorHandlingType", Value: "CustomWithoutRollBack"},
		{Key: "ImportedService", Value: "Launchpad.LamaCloudIntegrationV2"},
		{Key: "OperationName", Value: "PushAppDeployment"},
		// An unsupported field forces the raw fallback.
		{Key: "HttpConfiguration", Value: bson.D{{Key: "$Type", Value: "Microflows$HttpConfiguration"}}},
	})
	ws, ok := act.(*microflows.WebServiceCallAction)
	if !ok {
		t.Fatalf("actionFromGen → %T, want *microflows.WebServiceCallAction", act)
	}
	if string(ws.ServiceID) != "Launchpad.LamaCloudIntegrationV2" || ws.OperationName != "PushAppDeployment" {
		t.Errorf("structured fields = {%q, %q}", ws.ServiceID, ws.OperationName)
	}
	if len(ws.RawBSON) == 0 {
		t.Error("RawBSON empty, want set (HttpConfiguration is an unsupported field → raw fallback)")
	}
}

// TestActionFromGen_WebServiceCall_NoRaw confirms a fully-structured action (only
// describable fields) does NOT set RawBSON, so the renderer uses the readable
// `call web service …` form, matching legacy's supported-key set.
func TestActionFromGen_WebServiceCall_NoRaw(t *testing.T) {
	act := decodeAction(t, bson.D{
		{Key: "$ID", Value: "ws-2"},
		{Key: "$Type", Value: "Microflows$CallWebServiceAction"},
		{Key: "ErrorHandlingType", Value: "Rollback"},
		{Key: "ImportedService", Value: "Mod.Service"},
		{Key: "OperationName", Value: "Op"},
	})
	ws := act.(*microflows.WebServiceCallAction)
	if len(ws.RawBSON) != 0 {
		t.Errorf("RawBSON set, want empty (all fields are supported → structured form)")
	}
}
