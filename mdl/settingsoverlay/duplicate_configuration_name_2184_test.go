// SPDX-License-Identifier: Apache-2.0

package settingsoverlay

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/model"
)

// Studio Pro accepts two server configurations with the same name (a merge
// brings them in). The overlay matched configurations by name only, so both
// modelled configurations landed on the second stored document: the unit held
// its $ID twice and every settings write was refused, including writes that had
// nothing to do with configurations (ako/mxcli#2184, mendixlabs/mxcli#1837).
func TestConfigurations_DuplicateNamesKeepTheirOwnDocuments(t *testing.T) {
	const firstID = "b6fc893f-0000-4000-8000-000000000001"
	const secondID = "563bb0f4-0000-4000-8000-000000000002"
	part := map[string]any{
		"Configurations": bson.A{
			int32(3),
			bson.M{"$ID": bsonutil.IDToBsonBinary(firstID), "Name": "Default", "HttpPortNumber": int64(8080), "Marker": "first"},
			bson.M{"$ID": bsonutil.IDToBsonBinary(secondID), "Name": "Default", "HttpPortNumber": int64(8080), "Marker": "second"},
		},
	}
	first := &model.ServerConfiguration{Name: "Default", HttpPortNumber: 8080}
	first.ID = firstID
	second := &model.ServerConfiguration{Name: "Default", HttpPortNumber: 9090}
	second.ID = secondID
	cs := &model.ConfigurationSettings{Configurations: []*model.ServerConfiguration{first, second}}

	cfgs := ArrayElements(Configurations(cs, part)["Configurations"])
	if len(cfgs) != 2 {
		t.Fatalf("got %d configurations, want 2", len(cfgs))
	}
	for i, want := range []struct {
		id, marker string
		port       int64
	}{{firstID, "first", 8080}, {secondID, "second", 9090}} {
		c := cfgs[i]
		if got := idKey(c["$ID"]); got != idKey(bsonutil.IDToBsonBinary(want.id)) {
			t.Errorf("configuration %d: $ID = %v, want %s", i, c["$ID"], want.id)
		}
		if c["Marker"] != want.marker {
			t.Errorf("configuration %d overlaid onto the %v document, want the %s one", i, c["Marker"], want.marker)
		}
		if c["HttpPortNumber"] != want.port {
			t.Errorf("configuration %d: HttpPortNumber = %v, want %d", i, c["HttpPortNumber"], want.port)
		}
	}
}

// Without stored ids on the model (a configuration built by hand), name matching
// still pairs each modelled configuration with a distinct stored document.
func TestConfigurations_DuplicateNamesWithoutIDsPairInOrder(t *testing.T) {
	part := map[string]any{
		"Configurations": bson.A{
			int32(3),
			bson.M{"$ID": "a", "Name": "Default", "Marker": "first"},
			bson.M{"$ID": "b", "Name": "default", "Marker": "second"},
		},
	}
	cs := &model.ConfigurationSettings{Configurations: []*model.ServerConfiguration{
		{Name: "Default"}, {Name: "Default"},
	}}
	cfgs := ArrayElements(Configurations(cs, part)["Configurations"])
	if len(cfgs) != 2 || cfgs[0]["Marker"] != "first" || cfgs[1]["Marker"] != "second" {
		t.Fatalf("got %v, want the first and second stored documents in order", cfgs)
	}
}
