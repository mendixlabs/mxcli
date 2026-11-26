// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Mendix 11.17 gives every page a popup close behaviour: UseOldPopupCloseAction
// and a PopupOnCloseAction (a Forms$CancelChangesClientAction), measured on an
// 11.16.1 page after mx convert and on all 16 pages of a blank 11.17.0 project
// (ako/mxcli#2612). MDL cannot state either, so a rewrite of an existing page
// must keep what is stored — a page set to the old behaviour, or to another
// close action, keeps it — rather than take the default a fresh page gets.
func TestCarryStoredPopupClose_2612(t *testing.T) {
	stored := mustMarshal(t, bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Name", Value: "Orders"},
		{Key: "PopupOnCloseAction", Value: bson.D{
			{Key: "$Type", Value: "Forms$NoAction"},
			{Key: "DisabledDuringExecution", Value: true},
		}},
		{Key: "UseOldPopupCloseAction", Value: true},
	})
	rebuilt := mustMarshal(t, bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Name", Value: "Orders"},
	})
	out := carryStoredPopupClose(rebuilt, stored)
	var doc bson.D
	if err := bson.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	got := bson.Raw(out)
	if v, ok := got.Lookup("UseOldPopupCloseAction").BooleanOK(); !ok || !v {
		t.Errorf("UseOldPopupCloseAction = %v, want the stored true", got.Lookup("UseOldPopupCloseAction"))
	}
	if typ := got.Lookup("PopupOnCloseAction", "$Type").StringValue(); typ != "Forms$NoAction" {
		t.Errorf("PopupOnCloseAction $Type = %q, want the stored Forms$NoAction", typ)
	}

	// The control: a stored page from before 11.17 has neither key, and the
	// rebuild gains none — below 11.17 they are not in the metamodel.
	old := mustMarshal(t, bson.D{{Key: "$Type", Value: "Forms$Page"}, {Key: "Name", Value: "Orders"}})
	if out := carryStoredPopupClose(rebuilt, old); string(out) != string(rebuilt) {
		t.Errorf("a pre-11.17 page gained keys: %s", bson.Raw(out))
	}
	// A rebuild that states a key itself keeps its own value.
	own := mustMarshal(t, bson.D{{Key: "$Type", Value: "Forms$Page"}, {Key: "UseOldPopupCloseAction", Value: false}})
	if v := bson.Raw(carryStoredPopupClose(own, stored)).Lookup("UseOldPopupCloseAction").Boolean(); v {
		t.Error("a value the rebuild wrote was overwritten by the stored one")
	}
}
