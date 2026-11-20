// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genNav "github.com/mendixlabs/mxcli/modelsdk/gen/navigation"
)

// ako/mxcli#980: a native profile's bottom bar item carries a client action
// and an icon like a menu item, and the reader took only its legacy Page — so
// describe showed a nanoflow tab as an item with no action and no icon.
func TestNativeProfile_ReadsBottomBarActionAndIcon(t *testing.T) {
	text := func(s string) bson.D {
		return bson.D{{Key: "$Type", Value: "Texts$Text"}, {Key: "Items", Value: bson.A{int32(3),
			bson.D{{Key: "$Type", Value: "Texts$Translation"}, {Key: "LanguageCode", Value: "en_US"}, {Key: "Text", Value: s}}}}}
	}
	raw, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Navigation$NativeNavigationProfile"},
		{Key: "Name", Value: "NativePhone"},
		{Key: "BottomBarItems", Value: bson.A{int32(2), bson.D{
			{Key: "$Type", Value: "NativePages$BottomBarItem"},
			{Key: "Action", Value: bson.D{{Key: "$Type", Value: "Forms$CallNanoflowClientAction"},
				{Key: "DisabledDuringExecution", Value: true}, {Key: "Nanoflow", Value: "M.ShowReports"}}},
			{Key: "Caption", Value: text("Reports")},
			{Key: "Icon", Value: bson.D{{Key: "$Type", Value: "Forms$IconCollectionIcon"}, {Key: "Image", Value: "Atlas_Core.Atlas.home"}}},
			{Key: "Page", Value: ""},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	p := nativeNavProfileFromGen(el.(*genNav.NativeNavigationProfile))
	if len(p.MenuItems) != 1 {
		t.Fatalf("bottom bar items = %d, want 1", len(p.MenuItems))
	}
	it := p.MenuItems[0]
	if it.Caption != "Reports" || it.ActionDoc["Nanoflow"] != "M.ShowReports" {
		t.Errorf("action not read: caption %q doc %v", it.Caption, it.ActionDoc)
	}
	if it.Icon != "Atlas_Core.Atlas.home" || it.IconType != "Forms$IconCollectionIcon" {
		t.Errorf("icon not read: %q %q", it.IconType, it.Icon)
	}
}

// A native profile stores its offline sync configs as a web profile does; the
// writer used to ignore a native statement's sync block.
func TestNativeProfile_WritesSync(t *testing.T) {
	doc := bson.D{{Key: "$Type", Value: "Navigation$NativeNavigationProfile"}, {Key: "Name", Value: "NativePhone"},
		{Key: "OfflineEntityConfigs", Value: bson.A{navMarkerItems}}}
	out, err := navPatchNativeProfile(doc, types.NavigationProfileSpec{HasSync: true,
		OfflineEntities: []types.NavOfflineEntitySpec{{Entity: "M.Order", SyncMode: "All"}}})
	if err != nil {
		t.Fatal(err)
	}
	cfgs := navGetArray(out, "OfflineEntityConfigs")
	if len(cfgs) != 2 || navGetString(cfgs[1].(bson.D), "Entity") != "M.Order" {
		t.Errorf("sync not written: %v", cfgs)
	}
	// CONTROL: no sync block leaves the stored list alone.
	out, err = navPatchNativeProfile(doc, types.NavigationProfileSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if len(navGetArray(out, "OfflineEntityConfigs")) != 1 {
		t.Errorf("a statement without sync changed the list")
	}
}
