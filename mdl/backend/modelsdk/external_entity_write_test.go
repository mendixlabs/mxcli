// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genDm "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/domainmodels"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"go.mongodb.org/mongo-driver/bson"
)

// encodeEntity runs an entity through the write adapters exactly as CreateEntity
// does (entityToGen → assignEntityIDs → encode the owning DomainModel) and returns
// the encoded entity as a decoded BSON map for inspection.
func encodeEntity(t *testing.T, e *domainmodel.Entity, module string) map[string]any {
	t.Helper()
	dm := genDm.NewDomainModel()
	dm.SetID(element.ID("00000000-0000-0000-0000-0000000000d0"))
	ge := entityToGen(e, module, 11)
	assignEntityIDs(ge)
	dm.AddEntities(ge)

	raw, err := (&codec.Encoder{}).Encode(dm)
	if err != nil {
		t.Fatalf("encode domain model: %v", err)
	}
	var m map[string]any
	if err := bson.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	entities := extractArray(m["Entities"])
	if len(entities) == 0 {
		t.Fatal("no entities encoded")
	}
	em, _ := entities[len(entities)-1].(map[string]any)
	if em == nil {
		t.Fatalf("entity is not a map: %T", entities[len(entities)-1])
	}
	return em
}

// extractArray returns the object elements of a Mendix typed BSON array, skipping
// the leading int32 storage-list marker.
func extractArray(v any) []any {
	arr, ok := v.(bson.A)
	if !ok {
		return nil
	}
	var out []any
	for _, e := range arr {
		if _, isInt := e.(int32); isInt {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Issue #718: a top-level external entity must serialize with a
// Rest$ODataRemoteEntitySource (Key + capabilities) and its attributes must use
// Rest$ODataMappedValue — not a plain persistent entity with StoredValue, which
// the modelsdk backend produced before the fix.
func TestEntityToGen_ODataRemoteEntitySource(t *testing.T) {
	e := &domainmodel.Entity{
		Name:              "Airlines",
		Source:            "Rest$ODataRemoteEntitySource",
		RemoteServiceName: "TripPinTest.TripPinRW",
		RemoteEntityName:  "Airline",
		RemoteEntitySet:   "Airlines",
		Persistable:       true,
		Creatable:         true,
		Countable:         true,
		SkipSupported:     true,
		TopSupported:      true,
		RemoteKeyParts: []*domainmodel.RemoteKeyPart{
			{Name: "AirlineCode", RemoteName: "AirlineCode", RemoteType: "Edm.String", Type: &domainmodel.StringAttributeType{Length: 100}},
		},
		Attributes: []*domainmodel.Attribute{
			{Name: "AirlineCode", RemoteName: "AirlineCode", RemoteType: "Edm.String", Filterable: true, Sortable: true, Creatable: true, Type: &domainmodel.StringAttributeType{Length: 100}},
			{Name: "Name", RemoteName: "Name", RemoteType: "Edm.String", Filterable: true, Sortable: true, Type: &domainmodel.StringAttributeType{Length: 0}},
		},
	}
	em := encodeEntity(t, e, "TripPinTest")

	src, ok := em["Source"].(map[string]any)
	if !ok {
		t.Fatalf("Source missing or not a map: %T (entity serialized as plain persistent — the regression)", em["Source"])
	}
	if src["$Type"] != "Rest$ODataRemoteEntitySource" {
		t.Fatalf("Source.$Type = %v, want Rest$ODataRemoteEntitySource", src["$Type"])
	}
	if src["EntitySet"] != "Airlines" || src["RemoteName"] != "Airline" {
		t.Errorf("Source EntitySet/RemoteName = %v/%v", src["EntitySet"], src["RemoteName"])
	}
	if src["SourceDocument"] != "TripPinTest.TripPinRW" {
		t.Errorf("Source.SourceDocument = %v, want the consumed service ref", src["SourceDocument"])
	}
	key, ok := src["Key"].(map[string]any)
	if !ok || key["$Type"] != "Rest$ODataKey" {
		t.Fatalf("Source.Key missing/wrong (CE6010): %v", src["Key"])
	}
	if parts := extractArray(key["Parts"]); len(parts) != 1 {
		t.Errorf("Key.Parts count = %d, want 1", len(parts))
	}
	// Every attribute must be backed by a Rest$ODataMappedValue (CE6612).
	// String attribute types must always emit Length explicitly — including 0
	// (= unlimited). If Length is omitted, Studio Pro applies its UI default of
	// 200, which contradicts the OData service's "unlimited" type (CE6621).
	for _, it := range extractArray(em["Attributes"]) {
		am, _ := it.(map[string]any)
		val, _ := am["Value"].(map[string]any)
		if val["$Type"] != "Rest$ODataMappedValue" {
			t.Errorf("attr %v Value.$Type = %v, want Rest$ODataMappedValue", am["Name"], val["$Type"])
		}
		typ, _ := am["NewType"].(map[string]any)
		if typ["$Type"] == "DomainModels$StringAttributeType" {
			if _, ok := typ["Length"]; !ok {
				t.Errorf("attr %v StringAttributeType omits Length — Studio Pro defaults to 200 (CE6621)", am["Name"])
			}
		}
	}
}

// A derived/abstract type (no entity set) must serialize as Rest$ODataEntityTypeSource
// and be non-persistent.
func TestEntityToGen_ODataEntityTypeSource(t *testing.T) {
	e := &domainmodel.Entity{
		Name:              "Trip",
		Source:            "Rest$ODataEntityTypeSource",
		RemoteServiceName: "TripPinTest.TripPinRW",
		RemoteEntityName:  "Trip",
		Persistable:       false,
		RemoteKeyParts: []*domainmodel.RemoteKeyPart{
			{Name: "TripId", RemoteName: "TripId", RemoteType: "Edm.Int32", Type: &domainmodel.IntegerAttributeType{}},
		},
		Attributes: []*domainmodel.Attribute{
			{Name: "Name", RemoteName: "Name", RemoteType: "Edm.String", Type: &domainmodel.StringAttributeType{}},
		},
	}
	em := encodeEntity(t, e, "TripPinTest")
	src, ok := em["Source"].(map[string]any)
	if !ok || src["$Type"] != "Rest$ODataEntityTypeSource" {
		t.Fatalf("Source = %v, want Rest$ODataEntityTypeSource", em["Source"])
	}
	if src["EntityTypeName"] != "Trip" || src["SourceDocument"] != "TripPinTest.TripPinRW" {
		t.Errorf("EntityTypeName/SourceDocument = %v/%v", src["EntityTypeName"], src["SourceDocument"])
	}
}

// A primitive-collection NPE serializes Rest$ODataPrimitiveCollectionEntitySource
// and its single attribute uses Rest$ODataMappedPrimitiveCollectionValue.
func TestEntityToGen_ODataPrimitiveCollection(t *testing.T) {
	e := &domainmodel.Entity{
		Name:              "TripTag",
		Source:            "Rest$ODataPrimitiveCollectionEntitySource",
		RemoteServiceName: "TripPinTest.TripPinRW",
		Persistable:       false,
		Attributes: []*domainmodel.Attribute{
			{Name: "Tag", RemoteName: "Tag", RemoteType: "Edm.String", IsPrimitiveCollection: true, Type: &domainmodel.StringAttributeType{}},
		},
	}
	em := encodeEntity(t, e, "TripPinTest")
	src, ok := em["Source"].(map[string]any)
	if !ok || src["$Type"] != "Rest$ODataPrimitiveCollectionEntitySource" {
		t.Fatalf("Source = %v, want Rest$ODataPrimitiveCollectionEntitySource", em["Source"])
	}
	attrs := extractArray(em["Attributes"])
	if len(attrs) != 1 {
		t.Fatalf("attr count = %d", len(attrs))
	}
	am, _ := attrs[0].(map[string]any)
	val, _ := am["Value"].(map[string]any)
	if val["$Type"] != "Rest$ODataMappedPrimitiveCollectionValue" {
		t.Errorf("Value.$Type = %v, want Rest$ODataMappedPrimitiveCollectionValue", val["$Type"])
	}
}

// Issue #718: an association between external entities must carry a
// Rest$ODataRemoteAssociationSource, not a null Source.
func TestAssocToGen_ODataRemoteAssociationSource(t *testing.T) {
	a := &domainmodel.Association{
		Name:                           "Trip_Airline",
		ParentID:                       "11111111-1111-1111-1111-111111111111",
		ChildID:                        "22222222-2222-2222-2222-222222222222",
		Type:                           "Reference",
		Owner:                          "Default",
		Source:                         "Rest$ODataRemoteAssociationSource",
		RemoteParentNavigationProperty: "Trips",
		RemoteChildNavigationProperty:  "Airline",
		Navigability2:                  "BothDirections",
	}
	ga := assocToGen(a)
	assignAssociationIDs(ga)
	raw, err := (&codec.Encoder{}).Encode(ga)
	if err != nil {
		t.Fatalf("encode association: %v", err)
	}
	var m map[string]any
	if err := bson.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	src, ok := m["Source"].(map[string]any)
	if !ok {
		t.Fatalf("association Source missing/null (the regression): %T", m["Source"])
	}
	if src["$Type"] != "Rest$ODataRemoteAssociationSource" {
		t.Fatalf("Source.$Type = %v, want Rest$ODataRemoteAssociationSource", src["$Type"])
	}
	if src["RemoteParentNavigationProperty"] != "Trips" || src["RemoteChildNavigationProperty"] != "Airline" {
		t.Errorf("nav props = %v/%v", src["RemoteParentNavigationProperty"], src["RemoteChildNavigationProperty"])
	}
	if src["Navigability2"] != "BothDirections" {
		t.Errorf("Navigability2 = %v", src["Navigability2"])
	}
}
