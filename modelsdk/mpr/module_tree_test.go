// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func treeGID(last byte) bson.Binary {
	b := make([]byte, 16)
	b[15] = last
	return bson.Binary{Subtype: 0, Data: b}
}

// Only modules and folders hold units: a document with a property that happens
// to share a containment's name is one unit and keeps that property.
func TestModuleTreeUnits_DocumentKeepsAPropertyNamedLikeAContainment(t *testing.T) {
	doc := bson.D{
		{Key: "$ID", Value: treeGID(7)},
		{Key: "$Type", Value: "Constants$Constant"},
		{Key: "Documents", Value: bson.A{int32(3), bson.D{{Key: "$ID", Value: treeGID(8)}, {Key: "$Type", Value: "Some$Part"}}}},
	}
	var units []TreeUnit
	if err := splitTreeUnit(doc, "", "Documents", &units); err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("got %d units, want the document alone", len(units))
	}
	var own bson.D
	if err := bson.Unmarshal(units[0].Contents, &own); err != nil {
		t.Fatal(err)
	}
	if treeLookup(own, "Documents") == nil {
		t.Error("the document lost its Documents property")
	}
}
