// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"

	"go.mongodb.org/mongo-driver/bson"
)

// Mendix 11.12+ rejects any storage object whose first BSON property is not
// "$ID" (System.InvalidOperationException: "Expected '$ID' as the first
// property of a storage object, but got '...'"). The writer historically built
// many objects as bson.M (a Go map), which bson.Marshal serializes in random
// key order, so "$ID" only landed first by luck. These tests pin the invariant:
// every serialized storage object must have "$ID" first, with no duplicate keys.

// validateStorageOrder walks a decoded BSON value (bson.D / bson.A, nesting
// preserved by unmarshalling into bson.D) and asserts that every document which
// looks like a storage object ($ID and/or $Type present) lists "$ID" first, and
// that no document contains duplicate keys (the hazard when a literal default is
// later "overwritten" via append).
func validateStorageOrder(t *testing.T, label string, v any) {
	t.Helper()
	switch d := v.(type) {
	case bson.D:
		seen := make(map[string]bool, len(d))
		hasID, hasType := false, false
		for i, e := range d {
			if seen[e.Key] {
				t.Errorf("%s: duplicate key %q in storage object", label, e.Key)
			}
			seen[e.Key] = true
			switch e.Key {
			case "$ID":
				hasID = true
				if i != 0 {
					t.Errorf("%s: $ID is at index %d, must be the first property", label, i)
				}
			case "$Type":
				hasType = true
			}
			validateStorageOrder(t, label+"."+e.Key, e.Value)
		}
		if (hasID || hasType) && (len(d) == 0 || d[0].Key != "$ID") {
			t.Errorf("%s: storage object does not start with $ID", label)
		}
	case bson.A:
		for i, e := range d {
			validateStorageOrder(t, fmt.Sprintf("%s[%d]", label, i), e)
		}
	}
}

// marshalAndValidate round-trips a value through BSON bytes (the real on-the-wire
// form) and validates ordering of the decoded document.
func marshalAndValidate(t *testing.T, label string, v any) {
	t.Helper()
	raw, err := bson.Marshal(v)
	if err != nil {
		t.Fatalf("%s: marshal failed: %v", label, err)
	}
	var decoded bson.D
	if err := bson.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("%s: unmarshal failed: %v", label, err)
	}
	validateStorageOrder(t, label, decoded)
}

func TestStorageObjects_IDIsFirstProperty(t *testing.T) {
	w := &Writer{}

	// Module-tree serializers: created on every project/module create and
	// shared by both engines — the universal offenders in the 11.12 nightly.
	t.Run("Module", func(t *testing.T) {
		mod := &model.Module{Name: "MyModule"}
		mod.ID = "mod-1"
		b, err := w.serializeModule(mod)
		if err != nil {
			t.Fatal(err)
		}
		var d bson.D
		if err := bson.Unmarshal(b, &d); err != nil {
			t.Fatal(err)
		}
		validateStorageOrder(t, "Module", d)
	})

	t.Run("Folder", func(t *testing.T) {
		folder := &model.Folder{Name: "Pages"}
		folder.ID = "f-1"
		b, err := w.serializeFolder(folder)
		if err != nil {
			t.Fatal(err)
		}
		var d bson.D
		if err := bson.Unmarshal(b, &d); err != nil {
			t.Fatal(err)
		}
		validateStorageOrder(t, "Folder", d)
	})

	t.Run("ModuleSecurity", func(t *testing.T) {
		b, err := w.serializeModuleSecurity("ms-1")
		if err != nil {
			t.Fatal(err)
		}
		var d bson.D
		if err := bson.Unmarshal(b, &d); err != nil {
			t.Fatal(err)
		}
		validateStorageOrder(t, "ModuleSecurity", d)
	})

	t.Run("ModuleSettings", func(t *testing.T) {
		b, err := w.serializeModuleSettings("set-1")
		if err != nil {
			t.Fatal(err)
		}
		var d bson.D
		if err := bson.Unmarshal(b, &d); err != nil {
			t.Fatal(err)
		}
		validateStorageOrder(t, "ModuleSettings", d)
	})

	// Domain-model associations (lossy CREATE OR MODIFY had wiped these before).
	t.Run("Association", func(t *testing.T) {
		a := &domainmodel.Association{
			Name:     "Order_Customer",
			ParentID: "child-id",
			ChildID:  "parent-id",
			Type:     domainmodel.AssociationTypeReference,
			Owner:    domainmodel.AssociationOwnerDefault,
		}
		a.ID = "assoc-1"
		marshalAndValidate(t, "Association", serializeAssociation(a))
	})

	t.Run("CrossAssociation", func(t *testing.T) {
		ca := &domainmodel.CrossModuleAssociation{
			Name:     "Order_Customer",
			ParentID: "child-id",
			ChildRef: "Other.Customer",
			Type:     domainmodel.AssociationTypeReference,
			Owner:    domainmodel.AssociationOwnerDefault,
		}
		ca.ID = "xassoc-1"
		marshalAndValidate(t, "CrossAssociation", serializeCrossAssociation(ca))
	})

	// Entity + Attribute: these were bson.D but in Studio Pro field order
	// (Name-first, $ID mid-document), which 11.12 rejects. validateStorageOrder
	// recurses, so this also covers the nested AccessRule and MemberAccess (which
	// were $Type-first). Regression guard for the 11.12 nightly `got 'Name'`.
	t.Run("Entity", func(t *testing.T) {
		attr := &domainmodel.Attribute{Name: "Amount", Type: &domainmodel.IntegerAttributeType{}}
		attr.ID = "attr-1"
		ma := &domainmodel.MemberAccess{AttributeName: "Amount", AccessRights: domainmodel.MemberAccessRightsReadWrite}
		ma.ID = "ma-1"
		ar := &domainmodel.AccessRule{
			ModuleRoleNames: []string{"Mod.User"},
			AllowRead:       true,
			MemberAccesses:  []*domainmodel.MemberAccess{ma},
		}
		ar.ID = "ar-1"
		e := &domainmodel.Entity{
			Name:        "Order",
			Persistable: true,
			Attributes:  []*domainmodel.Attribute{attr},
			AccessRules: []*domainmodel.AccessRule{ar},
		}
		e.ID = "ent-1"
		marshalAndValidate(t, "Entity", serializeEntity(e, "Mod", nil))
	})

	t.Run("Attribute", func(t *testing.T) {
		attr := &domainmodel.Attribute{Name: "Amount", Type: &domainmodel.IntegerAttributeType{}}
		attr.ID = "attr-2"
		marshalAndValidate(t, "Attribute", serializeAttribute(attr, false))
	})

	// Business-event tree: $ID was added dynamically after a $Type-first literal.
	t.Run("BusinessEventDefinition", func(t *testing.T) {
		def := &model.BusinessEventDefinition{
			ServiceName: "Svc",
			Channels: []*model.BusinessEventChannel{{
				ChannelName: "Ch",
				Messages: []*model.BusinessEventMessage{{
					MessageName: "Msg",
					Attributes: []*model.BusinessEventAttribute{
						{AttributeName: "A1", AttributeType: "String"},
						{AttributeName: "A2", AttributeType: "DateTime"},
					},
				}},
			}},
		}
		marshalAndValidate(t, "BusinessEventDefinition", serializeBusinessEventDefinition(def))
	})

	// Database-connector query tree: $ID added dynamically; nested DataType/SqlDataType.
	t.Run("DBQuery", func(t *testing.T) {
		q := &model.DatabaseQuery{
			Name: "Q1",
			SQL:  "SELECT 1",
			TableMappings: []*model.DatabaseTableMapping{{
				Entity:    "Mod.Ent",
				TableName: "ent",
				Columns:   []*model.DatabaseColumnMapping{{Attribute: "Name", ColumnName: "name"}},
			}},
			Parameters: []*model.DatabaseQueryParameter{{ParameterName: "p1"}},
		}
		marshalAndValidate(t, "DBQuery", serializeDBQuery(q))
	})

	// Server configuration: $ID added dynamically; nested ConstantValue list.
	t.Run("ServerConfiguration", func(t *testing.T) {
		cfg := &model.ServerConfiguration{
			Name: "Default",
			ConstantValues: []*model.ConstantValue{
				{ConstantId: "Mod.C1", Value: "v"},
			},
		}
		marshalAndValidate(t, "ServerConfiguration", serializeServerConfiguration(cfg))
	})
}
