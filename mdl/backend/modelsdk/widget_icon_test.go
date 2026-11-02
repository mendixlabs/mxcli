// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// iconToGen emitted Forms$IconCollectionIcon unconditionally, which was fine
// while that was also the only icon anyone could author, and wrong the moment
// DESCRIBE started reading real projects: a stored image icon round-tripped into
// a custom-icon reference and failed the build with CE1613
// (mendixlabs/mxcli#1059).
//
// These assert on the ENCODED document rather than on the struct, because the
// defect was entirely in what reached BSON — the model carried the right thing
// the whole time.
func TestIconToGen_EncodesEachElement(t *testing.T) {
	cases := []struct {
		name      string
		icon      *pages.Icon
		wantType  string
		wantImage string
		wantCode  int32
	}{{
		name:      "collection",
		icon:      &pages.Icon{Kind: types.MenuIconCollection, Image: "Atlas_Core.Atlas_Filled.pencil"},
		wantType:  "Forms$IconCollectionIcon",
		wantImage: "Atlas_Core.Atlas_Filled.pencil",
	}, {
		// The reported case. Identical spelling to the row above, different
		// document — so only the kind can separate them.
		name:      "image",
		icon:      &pages.Icon{Kind: types.MenuIconImage, Image: "DesignSystem.Icons_SVG.edit"},
		wantType:  "Forms$ImageIcon",
		wantImage: "DesignSystem.Icons_SVG.edit",
	}, {
		name:     "glyph",
		icon:     &pages.Icon{Kind: types.MenuIconGlyph, Code: 57377},
		wantType: "Forms$GlyphIcon",
		wantCode: 57377,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el := iconToGen(tc.icon)
			if el == nil {
				t.Fatal("no icon element was built")
			}
			encoded, err := (&codec.Encoder{}).Encode(el)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			raw := bson.Raw(encoded)
			if got := lookupString(t, raw, "$Type"); got != tc.wantType {
				t.Errorf("$Type = %q, want %q", got, tc.wantType)
			}
			if tc.wantImage != "" {
				if got := lookupString(t, raw, "Image"); got != tc.wantImage {
					t.Errorf("Image = %q, want %q", got, tc.wantImage)
				}
			}
			if tc.wantCode != 0 {
				val, err := raw.LookupErr("Code")
				if err != nil {
					t.Fatalf("no Code on a glyph icon — the only thing identifying it: %v", err)
				}
				// int64 is what Studio Pro stores: 570 of 570 Forms$GlyphIcon
				// codes across nine Studio Pro-authored projects (ako/mxcli#1264).
				got, ok := val.Int64OK()
				if !ok {
					t.Fatalf("Code is %v, want an int64", val.Type)
				}
				if got != int64(tc.wantCode) {
					t.Errorf("Code = %d, want %d", got, tc.wantCode)
				}
			}
		})
	}
}

// CONTROL: an icon that identifies nothing produces no element at all, rather
// than one nobody can see. A bare Forms$GlyphIcon builds at 0 errors and renders
// nothing, so this failure would otherwise be invisible.
func TestIconToGen_AnIconIdentifyingNothingBuildsNoElement(t *testing.T) {
	for _, icon := range []*pages.Icon{
		{Kind: types.MenuIconGlyph},
		{Kind: types.MenuIconImage},
		{Kind: types.MenuIconCollection},
		{Kind: types.MenuIconKind("Forms$SomeFuture")},
	} {
		if el := iconToGen(icon); el != nil {
			t.Errorf("%+v built an element, want none", icon)
		}
	}
}

func lookupString(t *testing.T, raw bson.Raw, key string) string {
	t.Helper()
	val, err := raw.LookupErr(key)
	if err != nil {
		t.Fatalf("no %s on the icon element: %v", key, err)
	}
	s, ok := val.StringValueOK()
	if !ok {
		t.Fatalf("%s is not a string", key)
	}
	return s
}
