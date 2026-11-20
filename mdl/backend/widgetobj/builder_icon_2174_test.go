// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2174: an `ImageType: icon` image stores its icon as the
// WidgetValue's `Icon` element on `imageIcon` — the element a button's Icon
// holds. Measured on ConversationalUI.Snippet_Message_Content (11.16).

func iconValue() bson.D {
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
		{Key: "Icon", Value: nil},
		{Key: "Image", Value: ""},
		{Key: "PrimitiveValue", Value: ""},
	}
}

func TestSetIconValue_WritesEachKind_2174(t *testing.T) {
	cases := []struct {
		icon      *pages.Icon
		wantType  string
		wantKey   string
		wantValue any
	}{
		{&pages.Icon{Kind: types.MenuIconImage, Image: "ConversationalUI.Image_collection.Traces_icons"},
			"Forms$ImageIcon", "Image", "ConversationalUI.Image_collection.Traces_icons"},
		{&pages.Icon{Kind: types.MenuIconCollection, Image: "Atlas_Core.Atlas_Filled.user"},
			"Forms$IconCollectionIcon", "Image", "Atlas_Core.Atlas_Filled.user"},
		{&pages.Icon{Kind: types.MenuIconGlyph, Code: 57377},
			"Forms$GlyphIcon", "Code", int32(57377)},
	}
	for _, c := range cases {
		t.Run(c.wantType, func(t *testing.T) {
			in := iconValue()
			got := setIconValue(in, iconElement(c.icon))
			if len(got) != len(in) {
				t.Fatalf("key count changed: %d -> %d", len(in), len(got))
			}
			icon, ok := get(got, "Icon").(bson.D)
			if !ok {
				t.Fatalf("Icon = %#v, want an element", get(got, "Icon"))
			}
			if v := get(icon, "$Type"); v != c.wantType {
				t.Errorf("$Type = %v, want %s", v, c.wantType)
			}
			if v := get(icon, c.wantKey); v != c.wantValue {
				t.Errorf("%s = %#v, want %#v", c.wantKey, v, c.wantValue)
			}
			if get(icon, "$ID") == nil {
				t.Error("icon element has no $ID")
			}
			if v := get(got, "Image"); v != "" {
				t.Errorf("Image was disturbed: %v", v)
			}
		})
	}
}

// An icon that identifies nothing writes no element rather than an invisible one.
func TestIconElement_RefusesAnEmptyIcon_2174(t *testing.T) {
	for _, ic := range []*pages.Icon{nil,
		{Kind: types.MenuIconGlyph},
		{Kind: types.MenuIconImage},
		{Kind: types.MenuIconUnknown, Image: "M.C.I"}} {
		if e := iconElement(ic); e != nil {
			t.Errorf("iconElement(%+v) = %v, want nil", ic, e)
		}
	}
}

// A value node with no Icon key is not icon-typed; none is added (CE0463 shape).
func TestSetIconValue_NeverAddsTheKey_2174(t *testing.T) {
	in := bson.D{{Key: "$Type", Value: "CustomWidgets$WidgetValue"}, {Key: "Image", Value: ""}}
	got := setIconValue(in, iconElement(&pages.Icon{Kind: types.MenuIconImage, Image: "M.C.I"}))
	if len(got) != len(in) || get(got, "Icon") != nil {
		t.Errorf("an Icon key was added: %v", got)
	}
}
