// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2534: a Send REST request activity never had its ErrorHandlingType
// written, so `on error begin … end error` was stored as Rollback. Measured on
// 10.24.11, 11.12.6 and 11.16.1: both custom handlers build; continue is CE6035
// (refused at check time by MDL076).
func TestRestOperationCallAction_WritesErrorHandlingType_2534(t *testing.T) {
	for _, tc := range []struct {
		set  microflows.ErrorHandlingType
		want string
	}{
		{microflows.ErrorHandlingTypeCustom, "Custom"},
		{microflows.ErrorHandlingTypeCustomWithoutRollback, "CustomWithoutRollBack"},
		// Control: no clause keeps the default the activity always had.
		{"", "Rollback"},
	} {
		g := restOperationCallActionToGen(&microflows.RestOperationCallAction{
			ErrorHandlingType: tc.set,
			Operation:         "M.Api.GetStatus",
		})
		raw, err := (&codec.Encoder{}).Encode(g)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		got, _ := bson.Raw(raw).Lookup("ErrorHandlingType").StringValueOK()
		if got != tc.want {
			t.Errorf("ErrorHandlingType %q stored as %q, want %q", tc.set, got, tc.want)
		}
	}
}
