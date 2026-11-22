// SPDX-License-Identifier: Apache-2.0

// Pinned against native-pwa-migrator's fixtures/native-phone-minimal: a real
// Studio-Pro-authored NativePhone profile whose one BottomBarItem is
//
//	{ $Type: "NativePages$BottomBarItem",
//	  Caption: { $Type: "Texts$Text", Items: [3, [{ $Type: "Texts$Translation",
//	      LanguageCode: "en_US", Text: "Home" }]] },
//	  Icon: { $Type: "Forms$GlyphIcon", Code: 57377 },
//	  Page: "NativeMobile.Home_Native" }
//
// (marker re-verified directly against the fixture's raw BSON, not copied from
// navCaptionBson — see navBottomBarCaptionBson's doc comment). mxcli writes
// Icon only as a Forms$GlyphIcon (see navBottomBarItemBson's doc comment), so
// these tests check the four keys mxcli owns, glyph-icon acceptance, and the
// rejections that keep it from guessing a shape Studio Pro has never
// confirmed.
package modelsdkbackend

import (
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestNavBottomBarItemBson_MatchesTheFixtureShape(t *testing.T) {
	got, err := navBottomBarItemBson(types.NavMenuItemSpec{Caption: "Home", Page: "NativeMobile.Home_Native"})
	if err != nil {
		t.Fatalf("navBottomBarItemBson: %v", err)
	}
	if v, _ := profileKey(got, "$Type"); v != "NativePages$BottomBarItem" {
		t.Errorf("$Type = %v", v)
	}
	if v, _ := profileKey(got, "Page"); v != "NativeMobile.Home_Native" {
		t.Errorf("Page = %v", v)
	}
	// No icon given — Icon stays an explicit null. See
	// TestNavBottomBarItemBson_AcceptsGlyphIcon for the non-nil case.
	if v, ok := profileKey(got, "Icon"); !ok || v != nil {
		t.Errorf("Icon = %#v, want an explicit null", v)
	}

	caption, ok := profileKey(got, "Caption")
	if !ok {
		t.Fatal("no Caption key at all")
	}
	captionDoc, ok := caption.(bson.D)
	if !ok {
		t.Fatalf("Caption is %T, want bson.D", caption)
	}
	if v, _ := profileKey(captionDoc, "$Type"); v != "Texts$Text" {
		t.Errorf("Caption.$Type = %v", v)
	}
	items, ok := profileKey(captionDoc, "Items")
	if !ok {
		t.Fatal("Caption: no Items key at all")
	}
	arr, ok := items.(bson.A)
	if !ok || len(arr) != 2 {
		t.Fatalf("Caption.Items = %#v, want [marker, translation]", items)
	}
	if arr[0] != int32(3) {
		t.Errorf("Caption.Items marker = %v, want 3 (measured on the fixture, not navCaptionBson's 1)", arr[0])
	}
	translation, ok := arr[1].(bson.D)
	if !ok {
		t.Fatalf("Caption.Items[1] is %T, want bson.D", arr[1])
	}
	if v, _ := profileKey(translation, "$Type"); v != "Texts$Translation" {
		t.Errorf("Translation.$Type = %v", v)
	}
	if v, _ := profileKey(translation, "Text"); v != "Home" {
		t.Errorf("Translation.Text = %v", v)
	}
}

func TestNavBottomBarItemBson_RejectsSubItems(t *testing.T) {
	_, err := navBottomBarItemBson(types.NavMenuItemSpec{
		Caption: "Home", Page: "M.Home",
		Items: []types.NavMenuItemSpec{{Caption: "Sub", Page: "M.Sub"}},
	})
	if err == nil {
		t.Fatal("expected an error for a nested item — the native bottom bar is flat")
	}
}

func TestNavBottomBarItemBson_RejectsMicroflow(t *testing.T) {
	_, err := navBottomBarItemBson(types.NavMenuItemSpec{Caption: "Home", Microflow: "M.DoStuff"})
	if err == nil {
		t.Fatal("expected an error for a microflow target — BottomBarItem has no microflow action")
	}
}

func TestNavBottomBarItemBson_RequiresAPage(t *testing.T) {
	_, err := navBottomBarItemBson(types.NavMenuItemSpec{Caption: "Home"})
	if err == nil {
		t.Fatal("expected an error when neither a page nor a microflow is given")
	}
}

func TestNavBottomBarItemBson_RejectsIcon(t *testing.T) {
	for name, spec := range map[string]types.NavMenuItemSpec{
		"collection icon": {Caption: "Home", Page: "M.Home", Icon: "Atlas_Core.Atlas.home"},
		"image icon":      {Caption: "Home", Page: "M.Home", Icon: "M.Images.Logo", IconKind: types.MenuIconImage},
	} {
		if _, err := navBottomBarItemBson(spec); err == nil {
			t.Errorf("%s: expected an error — only a glyph icon is confirmed for native bottom bar items", name)
		}
	}
}

// Matches the fixture's exact shape: { $Type: "Forms$GlyphIcon", Code: 57377 }.
func TestNavBottomBarItemBson_AcceptsGlyphIcon(t *testing.T) {
	got, err := navBottomBarItemBson(types.NavMenuItemSpec{
		Caption: "Home", Page: "NativeMobile.Home_Native", IconKind: types.MenuIconGlyph, IconCode: 57377,
	})
	if err != nil {
		t.Fatalf("navBottomBarItemBson: %v", err)
	}
	icon, ok := profileKey(got, "Icon")
	if !ok || icon == nil {
		t.Fatal("Icon key missing or null, want a Forms$GlyphIcon")
	}
	d, ok := icon.(bson.D)
	if !ok {
		t.Fatalf("Icon is %T, want bson.D", icon)
	}
	if v, _ := profileKey(d, "$Type"); v != "Forms$GlyphIcon" {
		t.Errorf("Icon.$Type = %v, want Forms$GlyphIcon", v)
	}
	if v, _ := profileKey(d, "Code"); v != int32(57377) {
		t.Errorf("Icon.Code = %v (%T), want int32 57377", v, v)
	}
}

func TestNavPatchNativeProfile_LeavesBottomBarItemsAloneWithoutAMenuBlock(t *testing.T) {
	// Mirrors navPatchWebProfile's own rule for Menu: a CREATE OR REPLACE
	// NAVIGATION statement with no MENU clause must not wipe an existing menu.
	before := newNativeProfileBson("NativePhone")
	wantBefore, ok := profileKey(before, "BottomBarItems")
	if !ok {
		t.Fatal("fixture setup: no BottomBarItems key at all")
	}
	got, err := navPatchNativeProfile(before, types.NavigationProfileSpec{HasMenu: false})
	if err != nil {
		t.Fatalf("navPatchNativeProfile: %v", err)
	}
	gotAfter, ok := profileKey(got, "BottomBarItems")
	if !ok {
		t.Fatal("BottomBarItems key vanished")
	}
	if fmt.Sprintf("%v", wantBefore) != fmt.Sprintf("%v", gotAfter) {
		t.Errorf("BottomBarItems changed without a MENU clause: %v -> %v", wantBefore, gotAfter)
	}
}

func TestNavPatchNativeProfile_BuildsBottomBarItemsFromMenuItems(t *testing.T) {
	doc := newNativeProfileBson("NativePhone")
	got, err := navPatchNativeProfile(doc, types.NavigationProfileSpec{
		HasMenu:   true,
		MenuItems: []types.NavMenuItemSpec{{Caption: "Home", Page: "NativeMobile.Home_Native"}},
	})
	if err != nil {
		t.Fatalf("navPatchNativeProfile: %v", err)
	}
	v, ok := profileKey(got, "BottomBarItems")
	if !ok {
		t.Fatal("no BottomBarItems key at all")
	}
	arr, ok := v.(bson.A)
	if !ok || len(arr) != 2 {
		t.Fatalf("BottomBarItems = %#v, want [marker, item]", v)
	}
	if arr[0] != int32(2) {
		t.Errorf("BottomBarItems marker = %v, want 2", arr[0])
	}
	item, ok := arr[1].(bson.D)
	if !ok {
		t.Fatalf("BottomBarItems[1] is %T, want bson.D", arr[1])
	}
	if v, _ := profileKey(item, "$Type"); v != "NativePages$BottomBarItem" {
		t.Errorf("item $Type = %v", v)
	}
}

func TestNavPatchNativeProfile_PropagatesABottomBarItemError(t *testing.T) {
	doc := newNativeProfileBson("NativePhone")
	_, err := navPatchNativeProfile(doc, types.NavigationProfileSpec{
		HasMenu:   true,
		MenuItems: []types.NavMenuItemSpec{{Caption: "Bad", Microflow: "M.DoStuff"}},
	})
	if err == nil {
		t.Fatal("expected the microflow-target error to propagate out of navPatchNativeProfile")
	}
}

// The executor hands an item's OnClick over as a built client action
// (ako/mxcli#980), not as the legacy Page field, so the bottom bar's page is
// read from a show-page action — and every other action is refused rather than
// written as an item with no page.
func TestNavBottomBarItemBson_ReadsThePageFromTheAction(t *testing.T) {
	got, err := navBottomBarItemBson(types.NavMenuItemSpec{Caption: "Home",
		Action: &pages.PageClientAction{PageName: "NativeMobile.Home_Native"}})
	if err != nil {
		t.Fatalf("navBottomBarItemBson: %v", err)
	}
	if v, _ := profileKey(got, "Page"); v != "NativeMobile.Home_Native" {
		t.Errorf("Page = %v, want the show-page action's page", v)
	}

	for name, spec := range map[string]types.NavMenuItemSpec{
		"microflow action": {Caption: "X", Action: &pages.MicroflowClientAction{MicroflowName: "M.F"}},
		"nanoflow action":  {Caption: "X", Action: &pages.NanoflowClientAction{NanoflowName: "M.N"}},
		"sign out":         {Caption: "X", SignOut: true},
		"page argument": {Caption: "X", Action: &pages.PageClientAction{PageName: "M.P",
			ParameterMappings: []*pages.PageClientParameterMapping{{}}}},
		"no action":   {Caption: "X", Action: &pages.NoClientAction{}},
		"kept action": {Caption: "X", KeepAction: []byte{0}},
	} {
		if _, err := navBottomBarItemBson(spec); err == nil {
			t.Errorf("%s: accepted, want a refusal", name)
		}
	}
}
