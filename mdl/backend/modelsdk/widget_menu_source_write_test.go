// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A tree or menu bar can draw its items from a menu document instead of a
// profile: Atlas_Core.Tablet_Sidebar's tree carries
// MenuSource → Forms$MenuDocumentSource, Menu "Atlas_Core.Tablet_Menu".
// It used to be written as the Responsive profile whatever the widget named, so
// a copy of Tablet_Sidebar showed the desktop menu (mendixlabs/mxcli#1189).
func TestMenuWidgetsToGen_WriteAMenuDocumentSource(t *testing.T) {
	for _, w := range []pages.Widget{
		&pages.NavigationTree{BaseWidget: pages.BaseWidget{Name: "nav"}, MenuDocument: "Atlas_Core.Tablet_Menu"},
		&pages.MenuBar{BaseWidget: pages.BaseWidget{Name: "bar"}, MenuDocument: "Atlas_Core.Tablet_Menu"},
	} {
		g, err := widgetToGen(w)
		if err != nil {
			t.Fatal(err)
		}
		doc := encodeToMap(t, g)
		src, ok := doc["MenuSource"].(map[string]any)
		if !ok {
			t.Fatalf("%v: MenuSource missing; keys = %v", doc["$Type"], keysOf(doc))
		}
		if src["$Type"] != "Forms$MenuDocumentSource" {
			t.Errorf("%v: MenuSource $Type = %v, want Forms$MenuDocumentSource", doc["$Type"], src["$Type"])
		}
		if src["Menu"] != "Atlas_Core.Tablet_Menu" {
			t.Errorf("%v: Menu = %v, want Atlas_Core.Tablet_Menu", doc["$Type"], src["Menu"])
		}
		if _, has := src["NavigationProfile"]; has {
			t.Errorf("%v: a menu document source carries no NavigationProfile", doc["$Type"])
		}
	}
}
