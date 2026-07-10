// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

func TestSerializeListOperation_FindByAttribute(t *testing.T) {
	doc := serializeListOperation(&microflows.FindByAttributeOperation{
		BaseElement:  model.BaseElement{ID: "operation-id"},
		ListVariable: "Items",
		Attribute:    "Demo.Item.Code",
		Expression:   "$IteratorItem/ExternalCode",
	})
	fields := listOperationDocMap(doc)

	if got := fields["$Type"]; got != "Microflows$Find" {
		t.Fatalf("$Type = %v, want Microflows$Find", got)
	}
	if got := fields["Attribute"]; got != "Demo.Item.Code" {
		t.Fatalf("Attribute = %v, want Demo.Item.Code", got)
	}
	if got := fields["Expression"]; got != "$IteratorItem/ExternalCode" {
		t.Fatalf("Expression = %v, want $IteratorItem/ExternalCode", got)
	}
	if got := fields["ListName"]; got != "Items" {
		t.Fatalf("ListName = %v, want Items", got)
	}
}

func TestSerializeListOperation_FilterByAssociation(t *testing.T) {
	doc := serializeListOperation(&microflows.FilterByAttributeOperation{
		BaseElement:  model.BaseElement{ID: "operation-id"},
		ListVariable: "Items",
		Association:  "Demo.Item_Category",
		Expression:   "$Category",
	})
	fields := listOperationDocMap(doc)

	if got := fields["$Type"]; got != "Microflows$Filter" {
		t.Fatalf("$Type = %v, want Microflows$Filter", got)
	}
	if got := fields["Association"]; got != "Demo.Item_Category" {
		t.Fatalf("Association = %v, want Demo.Item_Category", got)
	}
	if got := fields["Expression"]; got != "$Category" {
		t.Fatalf("Expression = %v, want $Category", got)
	}
}

func listOperationDocMap(doc bson.D) map[string]any {
	fields := make(map[string]any, len(doc))
	for _, elem := range doc {
		fields[elem.Key] = elem.Value
	}
	return fields
}
