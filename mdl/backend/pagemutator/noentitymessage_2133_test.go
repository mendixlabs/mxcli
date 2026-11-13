// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// ako/mxcli#2133 (mendixlabs/mxcli#1814): `set (NoEntityMessage: '…') on dv`
// was refused as "not a property of this built-in widget". A data view stores
// it as a Texts$Text; the setter writes the text into it, or creates one where
// the stored data view has none.
func TestSetNoEntityMessageOnADataView(t *testing.T) {
	for _, stored := range []any{nil, bson.D{{Key: "$Type", Value: "Texts$Text"}, {Key: "Items", Value: bson.A{int32(3)}}}} {
		dv := bson.D{{Key: "$Type", Value: "Forms$DataView"}, {Key: "Name", Value: "dv"}, {Key: "NoEntityMessage", Value: stored}}
		handled, err := setInputWidgetPropertyMut(dv, "NoEntityMessage", "Select an item")
		if !handled || err != nil {
			t.Fatalf("stored %v: handled=%v err=%v", stored, handled, err)
		}
		msg := bsonnav.DGetDoc(dv, "NoEntityMessage")
		items := bsonnav.ToBsonA(bsonnav.DGet(msg, "Items"))
		if len(items) != 2 || bsonnav.DGetString(items[1].(bson.D), "Text") != "Select an item" {
			t.Errorf("stored %v: NoEntityMessage = %v", stored, msg)
		}
	}
	// Control: any other widget does not take it here.
	if handled, _ := setInputWidgetPropertyMut(bson.D{{Key: "$Type", Value: "Forms$TextBox"}}, "NoEntityMessage", "x"); handled {
		t.Error("a text box took NoEntityMessage")
	}
}
