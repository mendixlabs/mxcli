// SPDX-License-Identifier: Apache-2.0

package changedatacapture

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// This file guards the properties added to types.go by hand (ako/mxcli#2384 §4).
//
// types.go is generated and says DO NOT EDIT, but the generator is not in-tree,
// so the additions live in the checked-in file. A regeneration — or anyone
// "restoring" the file — drops them silently: the document still decodes, the
// structure still looks right, and only the GUIDs are gone. These tests make
// that loud.

const (
	entityGUID = "057964e1-fd78-4e74-bd66-c74a06131511"
	attrGUID   = "f3bcaa9f-4ed8-46de-9b7d-dad81425bc33"
	assocGUID  = "bae3f172-aebb-49a6-ad92-783f53cf5014"
	zeroGUID   = "00000000-0000-0000-0000-000000000000"
	pointerID  = "70e40d90-b3a5-4cd2-acfa-ab8158b9bc89"
)

// A GUID is stored as BSON binary. Decoded through property.DecodeString — the
// generator's default for a string-typed field — StringValueOK() returns
// ("", false) and the false is discarded, so every GUID read back EMPTY on every
// Studio Pro-authored document, with no error anywhere.
//
// That is the CDC baseline's whole purpose: mendixsystem$entity.id and
// mendixsystem$attribute.id hold these verbatim, so a write path built on an
// empty read would detach every tracked entity from its table.
func TestVersionedEntityChangeSource_EntityGuidDecodesFromBinary(t *testing.T) {
	raw, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "ChangeDataCapture$VersionedEntityChangeSource"},
		{Key: "EntityGuid", Value: mmpr.IDToBsonBinary(entityGUID)},
		{Key: "ExposedName", Value: "Order"},
	})
	if err != nil {
		t.Fatal(err)
	}
	o := initVersionedEntityChangeSource()
	o.InitFromRaw(bson.Raw(raw))

	if got := o.EntityGuid(); got != entityGUID {
		t.Errorf("EntityGuid = %q, want %q — a binary GUID read as a string comes back empty", got, entityGUID)
	}
	if o.ExposedName() != "Order" {
		t.Errorf("ExposedName = %q; the rest of the element must still decode", o.ExposedName())
	}
}

func TestVersionedProperty_GuidsDecodeFromBinary(t *testing.T) {
	for _, tc := range []struct {
		name        string
		assoc       string
		wantAssoc   string
		whatItMeans string
	}{
		{"association property", assocGUID, assocGUID, "the GUID of the TO entity"},
		{"attribute property", zeroGUID, zeroGUID, "the all-zero GUID Studio Pro writes for an attribute"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := bson.Marshal(bson.D{
				{Key: "$Type", Value: "ChangeDataCapture$VersionedProperty"},
				{Key: "PropertyGuid", Value: mmpr.IDToBsonBinary(attrGUID)},
				{Key: "AssociatedEntityGuid", Value: mmpr.IDToBsonBinary(tc.assoc)},
				{Key: "ExposedName", Value: "OrderId"},
				{Key: "Type", Value: "Int64"},
			})
			if err != nil {
				t.Fatal(err)
			}
			o := initVersionedProperty()
			o.InitFromRaw(bson.Raw(raw))

			if got := o.PropertyGuid(); got != attrGUID {
				t.Errorf("PropertyGuid = %q, want %q", got, attrGUID)
			}
			if got := o.AssociatedEntityGuid(); got != tc.wantAssoc {
				t.Errorf("AssociatedEntityGuid = %q, want %q (%s)", got, tc.wantAssoc, tc.whatItMeans)
			}
			if o.Type() != "Int64" {
				t.Errorf("Type = %q, want Int64", o.Type())
			}
		})
	}
}

// The live half of the document points at the frozen half. mxbuild does not
// validate these pointers, so nothing downstream would report their loss.
func TestLiveSideKeepsItsBaselinePointers(t *testing.T) {
	srcRaw, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "ChangeDataCapture$EntityChangeSource"},
		{Key: "Entity", Value: "CdcPublish.Order"},
		{Key: "ExposedName", Value: "Order"},
		{Key: "VersionedSourcePointer", Value: mmpr.IDToBsonBinary(pointerID)},
	})
	if err != nil {
		t.Fatal(err)
	}
	src := initEntityChangeSource()
	src.InitFromRaw(bson.Raw(srcRaw))
	if got := src.VersionedSourceRefID(); got != element.ID(pointerID) {
		t.Errorf("VersionedSourcePointer = %q, want %q", got, pointerID)
	}

	for _, kind := range []string{"AttributeProperty", "AssociationProperty"} {
		t.Run(kind, func(t *testing.T) {
			propRaw, err := bson.Marshal(bson.D{
				{Key: "$Type", Value: "ChangeDataCapture$" + kind},
				{Key: "ExposedName", Value: "OrderId"},
				{Key: "VersionedPropertyPointer", Value: mmpr.IDToBsonBinary(attrGUID)},
			})
			if err != nil {
				t.Fatal(err)
			}
			var got element.ID
			if kind == "AttributeProperty" {
				o := initAttributeProperty()
				o.InitFromRaw(bson.Raw(propRaw))
				got = o.VersionedPropertyRefID()
			} else {
				o := initAssociationProperty()
				o.InitFromRaw(bson.Raw(propRaw))
				got = o.VersionedPropertyRefID()
			}
			if got != element.ID(attrGUID) {
				t.Errorf("VersionedPropertyPointer = %q, want %q", got, attrGUID)
			}
		})
	}
}

// AppName is the second segment of every topic the service publishes to
// (cdc.<AppName>.<ExposedName>.<Major>.<space>), and mxbuild reports CE1577 on
// an empty one — so losing it is both a build error and, if it ever got past
// one, a publisher shouting into a topic nobody reads.
func TestEntityChangeDataCaptureService_AppName(t *testing.T) {
	raw, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "ChangeDataCapture$EntityChangeDataCaptureService"},
		{Key: "Name", Value: "OrdersCDCService"},
		{Key: "ServiceName", Value: "OrdersCDCService"},
		{Key: "AppName", Value: "TestApp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	o := initEntityChangeDataCaptureService()
	o.InitFromRaw(bson.Raw(raw))

	if got := o.AppName(); got != "TestApp" {
		t.Errorf("AppName = %q, want TestApp", got)
	}
}

// A GUID written back has to reach storage as the same bytes it was read as:
// the runtime keys the database on it, so a re-encode that shifts a byte
// re-points an entity at nothing. codec.BinaryToUUID and mpr.IDToBsonBinary are
// the pair, and this pins them against the real values from the fixture.
func TestGuidBinaryRoundTripsExactly(t *testing.T) {
	for _, guid := range []string{entityGUID, attrGUID, assocGUID, zeroGUID, pointerID} {
		bin := mmpr.IDToBsonBinary(guid)
		if got := codec.BinaryToUUID(bin.Data); got != guid {
			t.Errorf("round trip changed the GUID: %q -> %q", guid, got)
		}
		if bin.Subtype != 0 {
			t.Errorf("%s: subtype = %d, want 0 (what Studio Pro writes)", guid, bin.Subtype)
		}
	}
}
