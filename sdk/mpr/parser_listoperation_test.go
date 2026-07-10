// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestParseListOperation_FindByAttribute(t *testing.T) {
	raw := map[string]any{
		"$Type":      "Microflows$Find",
		"$ID":        nil,
		"ListName":   "Orders",
		"Attribute":  "MyModule.Order.Status",
		"Expression": "'Active'",
	}
	op := parseListOperation(raw)
	got, ok := op.(*microflows.FindByAttributeOperation)
	if !ok {
		t.Fatalf("expected *FindByAttributeOperation, got %T", op)
	}
	if got.ListVariable != "Orders" {
		t.Errorf("ListVariable: got %q, want %q", got.ListVariable, "Orders")
	}
	if got.Attribute != "MyModule.Order.Status" {
		t.Errorf("Attribute: got %q, want %q", got.Attribute, "MyModule.Order.Status")
	}
	if got.Expression != "'Active'" {
		t.Errorf("Expression: got %q, want %q", got.Expression, "'Active'")
	}
}

func TestParseListOperation_FindByAssociation(t *testing.T) {
	raw := map[string]any{
		"$Type":       "Microflows$Find",
		"$ID":         nil,
		"ListName":    "Orders",
		"Association": "MyModule.Order_Customer",
		"Expression":  "$Customer",
	}
	op := parseListOperation(raw)
	got, ok := op.(*microflows.FindByAttributeOperation)
	if !ok {
		t.Fatalf("expected *FindByAttributeOperation, got %T", op)
	}
	if got.Association != "MyModule.Order_Customer" {
		t.Errorf("Association: got %q, want %q", got.Association, "MyModule.Order_Customer")
	}
}

func TestParseListOperation_FilterByAttribute(t *testing.T) {
	raw := map[string]any{
		"$Type":      "Microflows$Filter",
		"$ID":        nil,
		"ListName":   "Orders",
		"Attribute":  "MyModule.Order.IsActive",
		"Expression": "true",
	}
	op := parseListOperation(raw)
	got, ok := op.(*microflows.FilterByAttributeOperation)
	if !ok {
		t.Fatalf("expected *FilterByAttributeOperation, got %T", op)
	}
	if got.ListVariable != "Orders" {
		t.Errorf("ListVariable: got %q, want %q", got.ListVariable, "Orders")
	}
	if got.Attribute != "MyModule.Order.IsActive" {
		t.Errorf("Attribute: got %q, want %q", got.Attribute, "MyModule.Order.IsActive")
	}
}

func TestParseListOperation_Range(t *testing.T) {
	raw := map[string]any{
		"$Type":    "Microflows$ListRange",
		"$ID":      nil,
		"ListName": "Orders",
		"CustomRange": map[string]any{
			"$Type":            "Microflows$CustomRange",
			"OffsetExpression": "0",
			"LimitExpression":  "10",
		},
	}
	op := parseListOperation(raw)
	got, ok := op.(*microflows.ListRangeOperation)
	if !ok {
		t.Fatalf("expected *ListRangeOperation, got %T", op)
	}
	if got.ListVariable != "Orders" {
		t.Errorf("ListVariable: got %q, want %q", got.ListVariable, "Orders")
	}
	if got.OffsetExpression != "0" {
		t.Errorf("OffsetExpression: got %q, want %q", got.OffsetExpression, "0")
	}
	if got.LimitExpression != "10" {
		t.Errorf("LimitExpression: got %q, want %q", got.LimitExpression, "10")
	}
}
