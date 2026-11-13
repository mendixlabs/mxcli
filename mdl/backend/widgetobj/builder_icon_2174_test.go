// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2174: an icon-typed property is stored as the WidgetValue's Icon
// element, of the kind the icon is — the same elements an action button uses.
func TestIconElement(t *testing.T) {
	for _, tc := range []struct {
		icon     *pages.Icon
		wantType string
		key      string
		want     any
	}{
		{&pages.Icon{Kind: types.MenuIconCollection, Image: "Atlas_Core.Atlas.wrench"}, types.MenuIconStorageType(types.MenuIconCollection), "Image", "Atlas_Core.Atlas.wrench"},
		{&pages.Icon{Kind: types.MenuIconGlyph, Code: 57377}, types.MenuIconStorageType(types.MenuIconGlyph), "Code", int32(57377)},
	} {
		el := iconElement(tc.icon)
		got := map[string]any{}
		for _, e := range el {
			got[e.Key] = e.Value
		}
		if got["$Type"] != tc.wantType || got[tc.key] != tc.want {
			t.Errorf("iconElement(%+v) = %v", tc.icon, el)
		}
	}
	// Controls: an icon naming nothing writes nothing.
	for _, ic := range []*pages.Icon{nil, {Kind: types.MenuIconGlyph}, {Kind: types.MenuIconCollection}} {
		if el := iconElement(ic); el != nil {
			t.Errorf("iconElement(%+v) = %v, want nil", ic, el)
		}
	}
	_ = bson.D{}
}
