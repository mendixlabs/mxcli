// SPDX-License-Identifier: Apache-2.0

package settingsoverlay

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
)

// ako/mxcli#1286: the order is rebuilt from names. A name already stored keeps
// its entry ($ID included), so an unchanged order writes back unchanged; a new
// one gets a Settings$ThemeModuleEntry; a name left out is dropped.
func TestThemeModuleOrder_KeepsStoredEntries_Issue1286(t *testing.T) {
	coreID := bsonutil.IDToBsonBinary("aaaaaaaa-0000-4000-8000-000000000001")
	raw := bson.A{int32(3),
		bson.M{"$ID": coreID, "$Type": "Settings$ThemeModuleEntry", "ModuleName": "Atlas_Core"},
		bson.M{"$ID": bsonutil.NewIDBsonBinary(), "$Type": "Settings$ThemeModuleEntry", "ModuleName": "Old"},
	}
	out := ThemeModuleOrder([]string{"Atlas_Core", "BrandTheme"}, raw)
	if len(out) != 3 || out[0] != int32(3) {
		t.Fatalf("got %v, want the marker and two entries", out)
	}
	core, _ := AsMap(out[1])
	if core["ModuleName"] != "Atlas_Core" || idKey(core["$ID"]) != idKey(coreID) {
		t.Errorf("the stored Atlas_Core entry was not kept: %v", core)
	}
	brand, _ := AsMap(out[2])
	if brand["ModuleName"] != "BrandTheme" || brand["$Type"] != "Settings$ThemeModuleEntry" || brand["$ID"] == nil {
		t.Errorf("new entry: %v", brand)
	}
	if empty := ThemeModuleOrder(nil, raw); len(empty) != 1 {
		t.Errorf("an empty order should be the marker alone, got %v", empty)
	}
}
