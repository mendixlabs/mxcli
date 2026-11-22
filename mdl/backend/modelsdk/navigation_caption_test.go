// SPDX-License-Identifier: Apache-2.0

// Pinned against native-pwa-migrator's fixtures/native-phone-minimal — its
// Responsive profile's Menu.Items[…].Caption (this exact call site), its
// AppTitle, and its NativePhone profile's BottomBarItem.Caption are all
// { $Type: "Texts$Text", Items: [3, [{ $Type: "Texts$Translation", … }]] }.
// Walking every Texts$Text.Items occurrence in that fixture and a second,
// unrelated fixture (expr-checker-minimal) found marker 3 in all 7300+
// occurrences and zero exceptions — see navCaptionBson's doc comment.
package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

func TestNavCaptionBson_MatchesTheFixtureShape(t *testing.T) {
	got := navCaptionBson("Home")
	if v, _ := profileKey(got, "$Type"); v != "Texts$Text" {
		t.Errorf("$Type = %v", v)
	}
	items, ok := profileKey(got, "Items")
	if !ok {
		t.Fatal("no Items key at all")
	}
	arr, ok := items.(bson.A)
	if !ok || len(arr) != 2 {
		t.Fatalf("Items = %#v, want [marker, translation]", items)
	}
	if arr[0] != int32(3) {
		t.Errorf("Items marker = %v, want 3 (measured on the fixture, not the old 1)", arr[0])
	}
	translation, ok := arr[1].(bson.D)
	if !ok {
		t.Fatalf("Items[1] is %T, want bson.D", arr[1])
	}
	if v, _ := profileKey(translation, "$Type"); v != "Texts$Translation" {
		t.Errorf("Translation.$Type = %v", v)
	}
	if v, _ := profileKey(translation, "Text"); v != "Home" {
		t.Errorf("Translation.Text = %v", v)
	}
}

// The regression this fixes is on the caption, only reachable through the
// full menu item — assert on the wiring, not just the helper.
func TestNavMenuItemBson_CaptionMarkerMatchesTheFixture(t *testing.T) {
	item, err := navMenuItemBson(types.NavMenuItemSpec{Caption: "Home", Page: "M.Home"})
	if err != nil {
		t.Fatalf("navMenuItemBson: %v", err)
	}
	caption, ok := profileKey(item, "Caption")
	if !ok {
		t.Fatal("no Caption key at all")
	}
	captionDoc, ok := caption.(bson.D)
	if !ok {
		t.Fatalf("Caption is %T, want bson.D", caption)
	}
	items, ok := profileKey(captionDoc, "Items")
	if !ok {
		t.Fatal("Caption: no Items key at all")
	}
	arr, ok := items.(bson.A)
	if !ok || len(arr) == 0 {
		t.Fatalf("Caption.Items = %#v", items)
	}
	if arr[0] != int32(3) {
		t.Errorf("Caption.Items marker = %v, want 3", arr[0])
	}
}
