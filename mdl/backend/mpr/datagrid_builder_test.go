// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestDeepCloneWithNewIDs_RegeneratesAllIDs(t *testing.T) {
	origID1 := bsonutil.NewIDBsonBinary()
	origID2 := bsonutil.NewIDBsonBinary()
	origID3 := bsonutil.NewIDBsonBinary()

	doc := bson.D{
		{Key: "$ID", Value: origID1},
		{Key: "$Type", Value: "Forms$TextBox"},
		{Key: "AttributeRef", Value: bson.D{
			{Key: "$ID", Value: origID2},
			{Key: "$Type", Value: "DomainModels$AttributeRef"},
			{Key: "Attribute", Value: "Module.Entity.Name"},
			{Key: "EntityRef", Value: bson.D{
				{Key: "$ID", Value: origID3},
				{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
				{Key: "Entity", Value: "Module.Entity"},
			}},
		}},
		{Key: "Name", Value: "txtName"},
	}

	cloned := deepCloneWithNewIDs(doc)

	if dGetString(cloned, "$Type") != "Forms$TextBox" {
		t.Error("$Type not preserved")
	}
	if dGetString(cloned, "Name") != "txtName" {
		t.Error("Name not preserved")
	}

	clonedID1 := cloned[0].Value
	if binaryEqual(clonedID1, origID1) {
		t.Error("top-level $ID was not regenerated")
	}

	attrRef := dGetDoc(cloned, "AttributeRef")
	if attrRef == nil {
		t.Fatal("AttributeRef missing")
	}
	clonedID2 := attrRef[0].Value
	if binaryEqual(clonedID2, origID2) {
		t.Error("AttributeRef $ID was not regenerated — stale GUID would cause CE1613")
	}
	if dGetString(attrRef, "Attribute") != "Module.Entity.Name" {
		t.Error("Attribute value not preserved")
	}

	entityRef := dGetDoc(attrRef, "EntityRef")
	if entityRef == nil {
		t.Fatal("EntityRef missing")
	}
	clonedID3 := entityRef[0].Value
	if binaryEqual(clonedID3, origID3) {
		t.Error("EntityRef $ID was not regenerated")
	}
	if dGetString(entityRef, "Entity") != "Module.Entity" {
		t.Error("Entity value not preserved")
	}
}

func TestDeepCloneWithNewIDs_HandlesArrays(t *testing.T) {
	origID := bsonutil.NewIDBsonBinary()
	innerID := bsonutil.NewIDBsonBinary()

	doc := bson.D{
		{Key: "$ID", Value: origID},
		{Key: "Items", Value: bson.A{
			int32(2),
			bson.D{
				{Key: "$ID", Value: innerID},
				{Key: "$Type", Value: "SomeType"},
				{Key: "Value", Value: "test"},
			},
		}},
	}

	cloned := deepCloneWithNewIDs(doc)

	items := cloned[1].Value.(bson.A)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	itemDoc := items[1].(bson.D)
	if binaryEqual(itemDoc[0].Value, innerID) {
		t.Error("nested array item $ID was not regenerated")
	}
	if dGetString(itemDoc, "Value") != "test" {
		t.Error("nested value not preserved")
	}
}

func TestDeepCloneWithNewIDs_PreservesNil(t *testing.T) {
	origID := bsonutil.NewIDBsonBinary()

	doc := bson.D{
		{Key: "$ID", Value: origID},
		{Key: "EntityRef", Value: nil},
		{Key: "Name", Value: "test"},
	}

	cloned := deepCloneWithNewIDs(doc)
	if cloned[1].Value != nil {
		t.Error("nil value not preserved")
	}
	if dGetString(cloned, "Name") != "test" {
		t.Error("string value not preserved")
	}
}

// Test helpers

func binaryEqual(a, b any) bool {
	ab, aOk := a.(primitive.Binary)
	bb, bOk := b.(primitive.Binary)
	if !aOk || !bOk {
		return false
	}
	if len(ab.Data) != len(bb.Data) {
		return false
	}
	for i := range ab.Data {
		if ab.Data[i] != bb.Data[i] {
			return false
		}
	}
	return true
}
