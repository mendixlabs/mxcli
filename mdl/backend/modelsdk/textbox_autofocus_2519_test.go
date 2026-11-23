// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2519: the writer hard-coded AutoFocus false on every text box.
func TestTextBoxWritesAutoFocus_2519(t *testing.T) {
	for _, want := range []bool{true, false} {
		tb := &pages.TextBox{AutoFocus: want}
		tb.ID = "tb"
		g, err := widgetToGen(tb)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := (&codec.Encoder{}).Encode(g)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := bson.Raw(raw).Lookup("AutoFocus").BooleanOK()
		if !ok || got != want {
			t.Errorf("AutoFocus %v stored as %v (present %v)", want, got, ok)
		}
	}
}
