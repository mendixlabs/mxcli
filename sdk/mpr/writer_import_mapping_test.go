// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// TestSerializeImportMapping_TypeNames verifies the critical $Type naming convention.
// The correct names are "ImportMappings$ObjectMappingElement" and
// "ImportMappings$ValueMappingElement" — the namespace prefix is never repeated in the
// element name. Using the wrong name causes TypeCacheUnknownTypeException in Studio Pro.
func TestSerializeImportMapping_TypeNames(t *testing.T) {
	w := &Writer{}
	im := &model.ImportMapping{
		BaseElement: model.BaseElement{
			ID:       "test-im-id",
			TypeName: "ImportMappings$ImportMapping",
		},
		ContainerID: "test-module-id",
		Name:        "ImportPetResponse",
		ExportLevel: "Hidden",
		Elements: []*model.ImportMappingElement{
			{
				BaseElement: model.BaseElement{ID: "obj-elem-id"},
				Kind:        "Object",
				ExposedName: "",
				Entity:      "MyModule.Pet",
				Children: []*model.ImportMappingElement{
					{
						BaseElement: model.BaseElement{ID: "val-id-elem"},
						Kind:        "Value",
						ExposedName: "id",
						Attribute:   "MyModule.Pet.Id",
						DataType:    "Integer",
						IsKey:       true,
					},
					{
						BaseElement: model.BaseElement{ID: "val-name-elem"},
						Kind:        "Value",
						ExposedName: "name",
						Attribute:   "MyModule.Pet.Name",
						DataType:    "String",
					},
				},
			},
		},
	}

	data, err := w.serializeImportMapping(im)
	if err != nil {
		t.Fatalf("serializeImportMapping: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	assertField(t, raw, "$Type", "ImportMappings$ImportMapping")
	assertField(t, raw, "Name", "ImportPetResponse")

	elems := extractBsonArray(raw["Elements"])
	if len(elems) != 1 {
		t.Fatalf("Elements: expected 1, got %d", len(elems))
	}

	objElem, ok := elems[0].(map[string]any)
	if !ok {
		t.Fatalf("Elements[0]: expected map, got %T", elems[0])
	}
	// CRITICAL: must NOT be "ImportMappings$ImportObjectMappingElement"
	assertField(t, objElem, "$Type", "ImportMappings$ObjectMappingElement")
	assertField(t, objElem, "Entity", "MyModule.Pet")
	assertField(t, objElem, "ObjectHandling", "Create")

	children := extractBsonArray(objElem["Children"])
	if len(children) != 2 {
		t.Fatalf("Children: expected 2, got %d", len(children))
	}

	valElem, ok := children[0].(map[string]any)
	if !ok {
		t.Fatalf("Children[0]: expected map, got %T", children[0])
	}
	// CRITICAL: must NOT be "ImportMappings$ImportValueMappingElement"
	assertField(t, valElem, "$Type", "ImportMappings$ValueMappingElement")
	assertField(t, valElem, "Attribute", "MyModule.Pet.Id")

	// IsKey must be true on the first (key) element
	if valElem["IsKey"] != true {
		t.Errorf("IsKey: expected true, got %v", valElem["IsKey"])
	}
}

func TestSerializeImportMapping_RequiredFields(t *testing.T) {
	w := &Writer{}
	im := &model.ImportMapping{
		BaseElement: model.BaseElement{ID: "test-im-required"},
		ContainerID: "test-module-id",
		Name:        "MinimalMapping",
	}

	data, err := w.serializeImportMapping(im)
	if err != nil {
		t.Fatalf("serializeImportMapping: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	// These fields must be present with defaults — verified against Studio Pro-created BSON.
	// Missing fields cause CE errors when opening in Studio Pro.
	for _, field := range []string{
		"UseSubtransactionsForMicroflows",
		"PublicName",
		"XsdRootElementName",
		"OperationName",
		"ServiceName",
		"WsdlFile",
	} {
		if _, ok := raw[field]; !ok {
			t.Errorf("missing required field: %s", field)
		}
	}

	// ParameterType must be a sub-document with $Type DataTypes$UnknownType
	pt, ok := raw["ParameterType"].(map[string]any)
	if !ok {
		t.Fatalf("ParameterType: expected map, got %T", raw["ParameterType"])
	}
	assertField(t, pt, "$Type", "DataTypes$UnknownType")
}

func TestSerializeImportMapping_DefaultExportLevel(t *testing.T) {
	w := &Writer{}
	im := &model.ImportMapping{
		BaseElement: model.BaseElement{ID: "test-im-default-export"},
		ContainerID: "test-module-id",
		Name:        "DefaultExportLevelMapping",
		// ExportLevel intentionally omitted
	}

	data, err := w.serializeImportMapping(im)
	if err != nil {
		t.Fatalf("serializeImportMapping: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	assertField(t, raw, "ExportLevel", "Hidden")
}

func TestSerializeImportMapping_WithJsonStructureRef(t *testing.T) {
	w := &Writer{}
	im := &model.ImportMapping{
		BaseElement:   model.BaseElement{ID: "test-im-js-ref"},
		ContainerID:   "test-module-id",
		Name:          "MappingWithSchema",
		JsonStructure: "MyModule.PetJsonStructure",
	}

	data, err := w.serializeImportMapping(im)
	if err != nil {
		t.Fatalf("serializeImportMapping: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	assertField(t, raw, "JsonStructure", "MyModule.PetJsonStructure")
}

// TestSerializeImportValueDataType_AllTypes verifies that all supported data types
// map to the correct DataTypes$* BSON $Type values.
func TestSerializeImportValueDataType_AllTypes(t *testing.T) {
	tests := []struct {
		input    string
		wantType string
	}{
		{"String", "DataTypes$StringType"},
		{"Integer", "DataTypes$IntegerType"},
		{"Long", "DataTypes$IntegerType"},
		{"Decimal", "DataTypes$DecimalType"},
		{"Boolean", "DataTypes$BooleanType"},
		{"DateTime", "DataTypes$DateTimeType"},
		{"Binary", "DataTypes$BinaryType"},
		{"", "DataTypes$StringType"}, // unknown falls back to String
	}

	for _, tc := range tests {
		result := serializeImportValueDataType(tc.input)

		var found string
		for _, kv := range result {
			if kv.Key == "$Type" {
				found, _ = kv.Value.(string)
				break
			}
		}
		if found != tc.wantType {
			t.Errorf("serializeImportValueDataType(%q): $Type = %q, want %q",
				tc.input, found, tc.wantType)
		}
	}
}
