// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestShowPageAction_Roundtrip verifies that a ShowPageAction with parameters
// survives BSON serialization/deserialization.
func TestShowPageAction_Roundtrip(t *testing.T) {
	// Build a ShowPageAction with parameter mappings
	action := &microflows.ShowPageAction{
		BaseElement: model.BaseElement{ID: "test-action-id"},
		PageName:    "Sales.Product_NewEdit",
		PageParameterMappings: []*microflows.PageParameterMapping{
			{
				BaseElement: model.BaseElement{ID: "test-mapping-id"},
				Parameter:   "Sales.Product_NewEdit.Product",
				Argument:    "$Product",
			},
		},
	}

	// Serialize to BSON using the writer
	doc := serializeMicroflowAction(action)

	// Marshal to bytes (simulates writing to MPR)
	data, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("failed to marshal BSON: %v", err)
	}

	// Unmarshal back to map (simulates reading from MPR)
	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal BSON: %v", err)
	}

	// Parse using the parser
	parsed := parseShowPageAction(raw)

	// Verify page name
	if parsed.PageName != "Sales.Product_NewEdit" {
		t.Errorf("PageName = %q, want %q", parsed.PageName, "Sales.Product_NewEdit")
	}

	// Verify parameter mappings
	if len(parsed.PageParameterMappings) != 1 {
		t.Fatalf("PageParameterMappings count = %d, want 1", len(parsed.PageParameterMappings))
	}
	pm := parsed.PageParameterMappings[0]
	if pm.Parameter != "Sales.Product_NewEdit.Product" {
		t.Errorf("Parameter = %q, want %q", pm.Parameter, "Sales.Product_NewEdit.Product")
	}
	if pm.Argument != "$Product" {
		t.Errorf("Argument = %q, want %q", pm.Argument, "$Product")
	}
}

func TestShowPageAction_WritesValidPageParameterMapping(t *testing.T) {
	action := &microflows.ShowPageAction{
		BaseElement: model.BaseElement{ID: "test-action-id"},
		PageName:    "Sales.Product_NewEdit",
		PageParameterMappings: []*microflows.PageParameterMapping{
			{
				BaseElement: model.BaseElement{ID: "test-mapping-id"},
				Parameter:   "Sales.Product_NewEdit.Product",
				Argument:    "$Product",
			},
		},
	}

	doc := serializeMicroflowAction(action)
	data, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("failed to marshal BSON: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal BSON: %v", err)
	}

	formSettings := toMap(raw["FormSettings"])
	if formSettings == nil {
		t.Fatal("FormSettings missing")
	}

	mappings, ok := formSettings["ParameterMappings"].(primitive.A)
	if !ok {
		t.Fatalf("ParameterMappings type = %T, want primitive.A", formSettings["ParameterMappings"])
	}
	if len(mappings) != 2 {
		t.Fatalf("ParameterMappings length = %d, want marker plus one mapping", len(mappings))
	}
	if marker, ok := mappings[0].(int32); !ok || marker != 2 {
		t.Fatalf("ParameterMappings marker = %#v, want int32(2)", mappings[0])
	}

	mapping := toMap(mappings[1])
	if mapping == nil {
		t.Fatal("PageParameterMapping missing")
	}
	variable := toMap(mapping["Variable"])
	if variable == nil {
		t.Fatal("Variable is nil; Studio Pro rejects null page parameter mapping variables")
	}
	if got := extractString(variable["$Type"]); got != "Forms$PageVariable" {
		t.Fatalf("Variable $Type = %q, want Forms$PageVariable", got)
	}
}

// TestShowPageAction_RoundtripNoParams verifies that a ShowPageAction without parameters
// survives BSON serialization/deserialization.
func TestShowPageAction_RoundtripNoParams(t *testing.T) {
	action := &microflows.ShowPageAction{
		BaseElement: model.BaseElement{ID: "test-action-id"},
		PageName:    "Sales.Customer_Overview",
	}

	doc := serializeMicroflowAction(action)
	data, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("failed to marshal BSON: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal BSON: %v", err)
	}

	parsed := parseShowPageAction(raw)

	if parsed.PageName != "Sales.Customer_Overview" {
		t.Errorf("PageName = %q, want %q", parsed.PageName, "Sales.Customer_Overview")
	}
	if len(parsed.PageParameterMappings) != 0 {
		t.Errorf("PageParameterMappings count = %d, want 0", len(parsed.PageParameterMappings))
	}
}

// TestShowPageAction_TitleOverride_NotNil verifies that FormSettings.TitleOverride is written as
// an embedded Microflows$TextTemplate object, not nil.  Studio Pro rejects null embedded objects
// on project load (same class of bug as FormSettings.ParameterMappings.Variable — issue #295).
func TestShowPageAction_TitleOverride_NotNil(t *testing.T) {
	action := &microflows.ShowPageAction{
		BaseElement: model.BaseElement{ID: "test-action-id"},
		PageName:    "Sales.Product_NewEdit",
	}

	doc := serializeMicroflowAction(action)
	data, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("failed to marshal BSON: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal BSON: %v", err)
	}

	formSettings := toMap(raw["FormSettings"])
	if formSettings == nil {
		t.Fatal("FormSettings missing")
	}

	to := toMap(formSettings["TitleOverride"])
	if to == nil {
		t.Fatal("TitleOverride is nil; Studio Pro rejects null Microflows$TextTemplate objects (issue #468)")
	}
	if got := extractString(to["$Type"]); got != "Microflows$TextTemplate" {
		t.Fatalf("TitleOverride.$Type = %q, want %q", got, "Microflows$TextTemplate")
	}
}

// TestShowPageAction_RoundtripMultipleParams verifies multiple parameter mappings survive roundtrip.
func TestShowPageAction_RoundtripMultipleParams(t *testing.T) {
	action := &microflows.ShowPageAction{
		BaseElement: model.BaseElement{ID: "test-action-id"},
		PageName:    "Sales.Order_Detail",
		PageParameterMappings: []*microflows.PageParameterMapping{
			{
				BaseElement: model.BaseElement{ID: "mapping-1"},
				Parameter:   "Sales.Order_Detail.Order",
				Argument:    "$Order",
			},
			{
				BaseElement: model.BaseElement{ID: "mapping-2"},
				Parameter:   "Sales.Order_Detail.Customer",
				Argument:    "$Customer",
			},
		},
	}

	doc := serializeMicroflowAction(action)
	data, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("failed to marshal BSON: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal BSON: %v", err)
	}

	parsed := parseShowPageAction(raw)

	if parsed.PageName != "Sales.Order_Detail" {
		t.Errorf("PageName = %q, want %q", parsed.PageName, "Sales.Order_Detail")
	}
	if len(parsed.PageParameterMappings) != 2 {
		t.Fatalf("PageParameterMappings count = %d, want 2", len(parsed.PageParameterMappings))
	}
	if parsed.PageParameterMappings[0].Argument != "$Order" {
		t.Errorf("first Argument = %q, want %q", parsed.PageParameterMappings[0].Argument, "$Order")
	}
	if parsed.PageParameterMappings[1].Argument != "$Customer" {
		t.Errorf("second Argument = %q, want %q", parsed.PageParameterMappings[1].Argument, "$Customer")
	}
}
