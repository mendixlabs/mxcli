// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// mendixlabs/mxcli#1735 (ako/mxcli#1939). The reference is Studio Pro 11.16's
// own Odata.ODataSendDeleteMF in ako/TestApp (commit 31a34ca, "odata send
// delete example"): its two activities, verbatim but for $ID.
var (
	studioProSend = bson.D{
		{Key: "$ID", Value: "send-1"},
		{Key: "$Type", Value: "Microflows$SendExternalObject"},
		{Key: "ErrorHandlingType", Value: "Rollback"},
		{Key: "RefreshInClient", Value: false},
		{Key: "VariableNameToBeSent", Value: "NewOrders"},
	}
	studioProDelete = bson.D{
		{Key: "$ID", Value: "del-1"},
		{Key: "$Type", Value: "Microflows$DeleteExternalObject"},
		{Key: "DeleteVariableName", Value: "NewOrders"},
		{Key: "ErrorHandlingType", Value: "Rollback"},
		{Key: "RefreshInClient", Value: false},
	}
)

// The reader takes Studio Pro's activities as the two actions, not as
// unknown ones describe prints as a comment.
func TestExternalObject_ReadsStudioProActivities(t *testing.T) {
	if s, ok := decodeAction(t, studioProSend).(*microflows.SendExternalObjectAction); !ok || s.Variable != "NewOrders" ||
		s.ErrorHandlingType != microflows.ErrorHandlingTypeRollback || s.RefreshInClient {
		t.Errorf("send read as %#v", decodeAction(t, studioProSend))
	}
	if d, ok := decodeAction(t, studioProDelete).(*microflows.DeleteExternalObjectAction); !ok || d.Variable != "NewOrders" ||
		d.ErrorHandlingType != microflows.ErrorHandlingTypeRollback || d.RefreshInClient {
		t.Errorf("delete read as %#v", decodeAction(t, studioProDelete))
	}
}

// What mxcli writes for `send external object $NewOrders;` and `delete
// external object $NewOrders;` is Studio Pro's property set with Studio Pro's
// values — the error handling stored as Rollback, not left out as a plain
// delete's is.
func TestExternalObject_WritesStudioProPropertySet(t *testing.T) {
	send := &microflows.SendExternalObjectAction{Variable: "NewOrders"}
	send.ID = model.ID("send-1")
	del := &microflows.DeleteExternalObjectAction{Variable: "NewOrders"}
	del.ID = model.ID("del-1")
	for _, tc := range []struct {
		name string
		got  bson.D
		want bson.D
	}{
		{"send", encodeElement(t, microflowActionToGen(send)), studioProSend},
		{"delete", encodeElement(t, microflowActionToGen(del)), studioProDelete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := map[string]any{}
			for _, e := range tc.want {
				if e.Key != "$ID" {
					want[e.Key] = e.Value
				}
			}
			got := map[string]any{}
			for _, e := range tc.got {
				if e.Key != "$ID" {
					got[e.Key] = e.Value
				}
			}
			if len(got) != len(want) {
				t.Fatalf("property set = %v, want Studio Pro's %v", got, want)
			}
			for k, v := range want {
				if got[k] != v {
					t.Errorf("%s = %v, want Studio Pro's %v (got %v)", k, got[k], v, got)
				}
			}
		})
	}
}
