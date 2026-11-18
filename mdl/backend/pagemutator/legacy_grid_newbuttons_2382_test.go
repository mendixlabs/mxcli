// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
)

// ako/mxcli#2382 (mendixlabs/mxcli#1889): a legacy grid's control-bar buttons
// and search fields are stored under `NewButtons`, the storage name of
// Forms$GridControlBar.Items and Forms$SearchBar.Items — read from OIDC SSO
// 2.3.2's OIDC.Snip_Configuration. The finder walked `Items`, so
// `set (Caption: …) on actionButton4` was "widget not found", and #1459's
// search-field addressing never reached a real search field either.

func gridActionButton(name, caption string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$GridActionButton"},
		{Key: "Name", Value: name},
		{Key: "CaptionTemplate", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$ClientTemplate"},
			{Key: "Template", Value: legacyGridText(caption)},
		}},
	}
}

func makeStoredLegacyGridPage() bson.D {
	return makeRawPage(bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$DataGrid"},
		{Key: "Name", Value: "grid1"},
		{Key: "DataSource", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$GridXPathSource"},
			{Key: "SearchBar", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$SearchBar"},
				{Key: "NewButtons", Value: bson.A{int32(2),
					legacyGridSearchField("Forms$ComparisonSearchField", "searchField1", "Alias", "OIDC.Client.Alias"),
				}},
			}},
		}},
		{Key: "ControlBar", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$GridControlBar"},
			{Key: "NewButtons", Value: bson.A{int32(2),
				gridActionButton("actionButton1", "New"),
				gridActionButton("actionButton4", "Edit"),
			}},
		}},
		{Key: "Columns", Value: bson.A{int32(2), legacyGridColumn("column1", "Alias", "OIDC.Client.Alias")}},
	})
}

func TestLegacyGridStoredButtonsAndSearchFieldsResolve_Issue2382(t *testing.T) {
	for _, name := range []string{"actionButton4", "searchField1", "column1"} {
		if findBsonWidget(makeStoredLegacyGridPage(), name) == nil {
			t.Errorf("%s not found in a grid stored as Studio Pro stores it", name)
		}
	}
}

func TestLegacyGridButtonCaptionAndDrop_Issue2382(t *testing.T) {
	raw := makeStoredLegacyGridPage()
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("actionButton4", "Caption", "Edit X"); err != nil {
		t.Fatalf("set Caption: %v", err)
	}
	btn := findBsonWidget(raw, "actionButton4").widget
	items := bsonnav.ToBsonA(bsonnav.DGet(bsonnav.DGetDoc(bsonnav.DGetDoc(btn, "CaptionTemplate"), "Template"), "Items"))
	if got := bsonnav.DGetString(items[1].(bson.D), "Text"); got != "Edit X" {
		t.Errorf("caption = %q, want Edit X", got)
	}

	if err := m.DropWidget([]backend.WidgetRef{{Widget: "actionButton4"}}); err != nil {
		t.Fatalf("drop: %v", err)
	}
	cb := bsonnav.DGetDoc(legacyGridDoc(t, raw), "ControlBar")
	if got := itemNames(t, bsonnav.ToBsonA(bsonnav.DGet(cb, "NewButtons"))); len(got) != 1 || got[0] != "actionButton1" {
		t.Errorf("control bar after drop = %v, want [actionButton1]", got)
	}
}
