// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// A menu clause replaces a native profile's bottom bar whole. Its item shape is
// pinned against one configured fixture, so a stored item carrying anything
// else is refused rather than rewritten without it (integration of
// mendixlabs/mxcli#1912).
func TestNavPatchNativeProfile_RefusesABottomBarItCannotRestate(t *testing.T) {
	pinned := func(extra ...bson.E) bson.D {
		return append(bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "NativePages$BottomBarItem"},
			{Key: "Caption", Value: navBottomBarCaptionBson("Home")},
			{Key: "Icon", Value: bson.D{{Key: "$Type", Value: "Forms$GlyphIcon"}, {Key: "Code", Value: int32(57377)}}},
			{Key: "Page", Value: "M.Home"},
		}, extra...)
	}
	profile := func(items ...bson.D) bson.D {
		bar := bson.A{int32(2)}
		for _, it := range items {
			bar = append(bar, it)
		}
		return bson.D{{Key: "$Type", Value: "Navigation$NativeNavigationProfile"}, {Key: "Name", Value: "NativePhone"},
			{Key: "BottomBarItems", Value: bar}}
	}
	spec := types.NavigationProfileSpec{HasMenu: true,
		MenuItems: []types.NavMenuItemSpec{{Caption: "Orders", Page: "M.Orders"}}}

	for _, c := range []struct {
		name string
		doc  bson.D
		want string
	}{
		{"an unwritten key", profile(pinned(bson.E{Key: "Visible", Value: true})), `"Visible"`},
		{"an image icon", profile(bson.D{{Key: "$ID", Value: navID()}, {Key: "$Type", Value: "NativePages$BottomBarItem"},
			{Key: "Icon", Value: bson.D{{Key: "$Type", Value: "Forms$ImageIcon"}}}, {Key: "Page", Value: "M.Home"}}), "Forms$ImageIcon"},
		{"another item type", profile(bson.D{{Key: "$Type", Value: "NativePages$SomethingElse"}}), "NativePages$SomethingElse"},
	} {
		if _, err := navPatchNativeProfile(c.doc, spec); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want a refusal naming %s, got %v", c.name, c.want, err)
		}
	}

	// Controls: an empty bottom bar and one of pinned-shape items are replaced.
	for _, doc := range []bson.D{profile(), profile(pinned())} {
		got, err := navPatchNativeProfile(doc, spec)
		if err != nil {
			t.Fatalf("refused a bottom bar it can restate: %v", err)
		}
		bar := navGetArray(got, "BottomBarItems")
		if len(bar) != 2 || navGetString(bar[1].(bson.D), "Page") != "M.Orders" {
			t.Errorf("bottom bar not replaced as the statement says: %v", bar)
		}
	}
}
