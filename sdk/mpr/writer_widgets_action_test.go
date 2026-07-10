// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// getFormSettings extracts FormSettings from a serialized Forms$FormAction document.
func getFormSettings(t *testing.T, doc bson.D) bson.D {
	t.Helper()
	for _, e := range doc {
		if e.Key == "FormSettings" {
			fs, ok := e.Value.(bson.D)
			if !ok {
				t.Fatalf("FormSettings is not bson.D, got %T", e.Value)
			}
			return fs
		}
	}
	t.Fatal("FormSettings not found")
	return nil
}

// getParamMappings extracts ParameterMappings from a FormSettings document.
func getParamMappings(t *testing.T, formSettings bson.D) primitive.A {
	t.Helper()
	for _, e := range formSettings {
		if e.Key == "ParameterMappings" {
			arr, ok := e.Value.(primitive.A)
			if !ok {
				t.Fatalf("ParameterMappings is not primitive.A, got %T", e.Value)
			}
			return arr
		}
	}
	t.Fatal("ParameterMappings not found")
	return nil
}

// TestPageClientAction_ParameterMappings_TypeIndicator verifies that
// Forms$FormAction always serializes ParameterMappings as [2] (type indicator
// only, no inline mapping objects), matching Studio Pro's native format.
//
// Studio Pro infers $currentObject from the enclosing widget context at runtime
// rather than reading explicit Forms$PageParameterMapping objects from BSON.
// Using int32(len) as the array's first element produces an invalid type
// indicator that Studio Pro cannot read, causing CE0115 (issue #296).
func TestPageClientAction_ParameterMappings_TypeIndicator(t *testing.T) {
	action := &pages.PageClientAction{
		BaseElement: model.BaseElement{ID: "action-id"},
		PageName:    "AuditTrail.Log_View",
		ParameterMappings: []*pages.PageClientParameterMapping{
			{
				BaseElement:   model.BaseElement{ID: "mapping-id"},
				ParameterName: "Log",
				Variable:      "$currentObject",
			},
		},
	}

	doc := serializeClientAction(action)
	if doc == nil {
		t.Fatal("serializeClientAction returned nil")
	}

	formSettings := getFormSettings(t, doc)
	mappings := getParamMappings(t, formSettings)

	// Must be exactly [int32(2)] — type indicator only, no inline objects.
	// Studio Pro's reader skips the type indicator (2 or 3) and reads the rest
	// as items; any other first-element value is treated as invalid.
	if len(mappings) != 1 {
		t.Fatalf("ParameterMappings: want exactly 1 element (type indicator), got %d", len(mappings))
	}
	indicator, ok := mappings[0].(int32)
	if !ok {
		t.Fatalf("ParameterMappings[0] is not int32, got %T", mappings[0])
	}
	if indicator != 2 {
		t.Errorf("ParameterMappings type indicator = %d, want 2", indicator)
	}
}

// TestPageClientAction_NoParams_TypeIndicator verifies that a PageClientAction
// without parameter mappings still serializes ParameterMappings as [2].
func TestPageClientAction_NoParams_TypeIndicator(t *testing.T) {
	action := &pages.PageClientAction{
		BaseElement: model.BaseElement{ID: "action-id"},
		PageName:    "Sales.Customer_Overview",
	}

	doc := serializeClientAction(action)
	if doc == nil {
		t.Fatal("serializeClientAction returned nil")
	}

	var bsonType string
	for _, e := range doc {
		if e.Key == "$Type" {
			bsonType, _ = e.Value.(string)
		}
	}
	if bsonType != "Forms$FormAction" {
		t.Errorf("$Type = %q, want %q", bsonType, "Forms$FormAction")
	}

	formSettings := getFormSettings(t, doc)
	mappings := getParamMappings(t, formSettings)
	if len(mappings) != 1 {
		t.Fatalf("ParameterMappings: want [2], got %v", mappings)
	}
}

// TestPageClientAction_RequiredFields verifies that Forms$FormAction includes
// all fields required by Studio Pro: NumberOfPagesToClose2, PagesForSpecializations,
// and FormSettings.TitleOverride.
func TestPageClientAction_RequiredFields(t *testing.T) {
	action := &pages.PageClientAction{
		BaseElement: model.BaseElement{ID: "action-id"},
		PageName:    "Sales.Order_Detail",
		ParameterMappings: []*pages.PageClientParameterMapping{
			{ParameterName: "Order", Variable: "$Order"},
			{ParameterName: "Customer", Variable: "$Customer"},
		},
	}

	doc := serializeClientAction(action)

	fields := map[string]bool{}
	for _, e := range doc {
		fields[e.Key] = true
	}
	for _, required := range []string{"NumberOfPagesToClose2", "PagesForSpecializations"} {
		if !fields[required] {
			t.Errorf("Forms$FormAction missing required field %q", required)
		}
	}

	formSettings := getFormSettings(t, doc)
	fsFields := map[string]bool{}
	for _, e := range formSettings {
		fsFields[e.Key] = true
	}
	if !fsFields["TitleOverride"] {
		t.Errorf("FormSettings missing required field %q", "TitleOverride")
	}

	// TitleOverride must be an embedded Microflows$TextTemplate object, not nil.
	// Studio Pro rejects null embedded objects on project load (same class of bug as issue #295).
	for _, e := range formSettings {
		if e.Key != "TitleOverride" {
			continue
		}
		to, ok := e.Value.(bson.D)
		if !ok || to == nil {
			t.Fatalf("TitleOverride must be a bson.D embedded object, got %T (%v)", e.Value, e.Value)
		}
		var typ string
		for _, f := range to {
			if f.Key == "$Type" {
				typ, _ = f.Value.(string)
			}
		}
		if typ != "Microflows$TextTemplate" {
			t.Errorf("TitleOverride.$Type = %q, want %q", typ, "Microflows$TextTemplate")
		}
	}
}
