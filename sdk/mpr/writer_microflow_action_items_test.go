// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

func TestSerializeCreateObjectActionItemsUseStorageListMarker(t *testing.T) {
	action := &microflows.CreateObjectAction{
		BaseElement:         model.BaseElement{ID: "create-1"},
		EntityQualifiedName: "SampleModule.Order",
		OutputVariable:      "Order",
		Commit:              microflows.CommitTypeNo,
		InitialMembers: []*microflows.MemberChange{
			{
				BaseElement:            model.BaseElement{ID: "member-1"},
				AttributeQualifiedName: "SampleModule.Order.Name",
				Type:                   microflows.MemberChangeTypeSet,
				Value:                  "'Sample'",
			},
		},
	}

	doc := serializeMicroflowAction(action)

	items, ok := getBSONField(doc, "Items").(bson.A)
	if !ok {
		t.Fatalf("Items is %T, want bson.A", getBSONField(doc, "Items"))
	}
	if len(items) != 2 {
		t.Fatalf("Items length = %d, want marker plus one item", len(items))
	}
	if marker, ok := items[0].(int32); !ok || marker != 2 {
		t.Fatalf("Items marker = %#v, want int32(2)", items[0])
	}
}

func TestSerializeChangeObjectActionItemsUseStorageListMarkerAndDefaultErrorHandling(t *testing.T) {
	action := &microflows.ChangeObjectAction{
		BaseElement:    model.BaseElement{ID: "change-1"},
		ChangeVariable: "Order",
		Commit:         microflows.CommitTypeNo,
		Changes: []*microflows.MemberChange{
			{
				BaseElement:            model.BaseElement{ID: "member-1"},
				AttributeQualifiedName: "SampleModule.Order.Status",
				Type:                   microflows.MemberChangeTypeSet,
				Value:                  "'Processed'",
			},
		},
	}

	doc := serializeMicroflowAction(action)

	if got := getBSONField(doc, "ErrorHandlingType"); got != "Rollback" {
		t.Fatalf("ErrorHandlingType = %#v, want Rollback", got)
	}
	items, ok := getBSONField(doc, "Items").(bson.A)
	if !ok {
		t.Fatalf("Items is %T, want bson.A", getBSONField(doc, "Items"))
	}
	if len(items) != 2 {
		t.Fatalf("Items length = %d, want marker plus one item", len(items))
	}
	if marker, ok := items[0].(int32); !ok || marker != 2 {
		t.Fatalf("Items marker = %#v, want int32(2)", items[0])
	}
}

func TestSerializeCommitActionAlwaysWritesDefaultErrorHandling(t *testing.T) {
	action := &microflows.CommitObjectsAction{
		BaseElement:    model.BaseElement{ID: "commit-1"},
		CommitVariable: "Order",
	}

	doc := serializeMicroflowAction(action)

	if got := getBSONField(doc, "ErrorHandlingType"); got != "Rollback" {
		t.Fatalf("ErrorHandlingType = %#v, want Rollback", got)
	}
}
