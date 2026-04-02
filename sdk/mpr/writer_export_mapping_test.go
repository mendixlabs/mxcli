// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// TestSerializeExportMapping_TypeNames verifies the critical $Type naming convention.
// The correct names are "ExportMappings$ObjectMappingElement" and
// "ExportMappings$ValueMappingElement" — the namespace prefix is never repeated.
// Using "ExportMappings$ExportObjectMappingElement" causes TypeCacheUnknownTypeException.
func TestSerializeExportMapping_TypeNames(t *testing.T) {
	w := &Writer{}
	em := &model.ExportMapping{
		BaseElement: model.BaseElement{
			ID:       "test-em-id",
			TypeName: "ExportMappings$ExportMapping",
		},
		ContainerID:     "test-module-id",
		Name:            "ExportPetRequest",
		ExportLevel:     "Hidden",
		NullValueOption: "LeaveOutElement",
		Elements: []*model.ExportMappingElement{
			{
				BaseElement: model.BaseElement{ID: "obj-elem-id"},
				Kind:        "Object",
				ExposedName: "Root",
				Entity:      "MyModule.Pet",
				JsonPath:    "(Object)",
				Children: []*model.ExportMappingElement{
					{
						BaseElement: model.BaseElement{ID: "val-id-elem"},
						Kind:        "Value",
						ExposedName: "id",
						Attribute:   "MyModule.Pet.Id",
						DataType:    "Integer",
						JsonPath:    "(Object)|id",
					},
					{
						BaseElement: model.BaseElement{ID: "val-name-elem"},
						Kind:        "Value",
						ExposedName: "name",
						Attribute:   "MyModule.Pet.Name",
						DataType:    "String",
						JsonPath:    "(Object)|name",
					},
				},
			},
		},
	}

	data, err := w.serializeExportMapping(em)
	if err != nil {
		t.Fatalf("serializeExportMapping: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	assertField(t, raw, "$Type", "ExportMappings$ExportMapping")
	assertField(t, raw, "Name", "ExportPetRequest")
	assertField(t, raw, "NullValueOption", "LeaveOutElement")

	elems := extractBsonArray(raw["Elements"])
	if len(elems) != 1 {
		t.Fatalf("Elements: expected 1, got %d", len(elems))
	}

	objElem, ok := elems[0].(map[string]any)
	if !ok {
		t.Fatalf("Elements[0]: expected map, got %T", elems[0])
	}
	// CRITICAL: must NOT be "ExportMappings$ExportObjectMappingElement"
	assertField(t, objElem, "$Type", "ExportMappings$ObjectMappingElement")
	assertField(t, objElem, "Entity", "MyModule.Pet")

	children := extractBsonArray(objElem["Children"])
	if len(children) != 2 {
		t.Fatalf("Children: expected 2, got %d", len(children))
	}

	valElem, ok := children[0].(map[string]any)
	if !ok {
		t.Fatalf("Children[0]: expected map, got %T", children[0])
	}
	// CRITICAL: must NOT be "ExportMappings$ExportValueMappingElement"
	assertField(t, valElem, "$Type", "ExportMappings$ValueMappingElement")
}

func TestSerializeExportMapping_DefaultNullValueOption(t *testing.T) {
	w := &Writer{}
	em := &model.ExportMapping{
		BaseElement: model.BaseElement{ID: "test-em-default-null"},
		ContainerID: "test-module-id",
		Name:        "DefaultNullMapping",
		// NullValueOption intentionally omitted — should default to "LeaveOutElement"
	}

	data, err := w.serializeExportMapping(em)
	if err != nil {
		t.Fatalf("serializeExportMapping: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	assertField(t, raw, "NullValueOption", "LeaveOutElement")
	assertField(t, raw, "ExportLevel", "Hidden")
}

func TestSerializeExportMapping_RequiredFields(t *testing.T) {
	w := &Writer{}
	em := &model.ExportMapping{
		BaseElement: model.BaseElement{ID: "test-em-required"},
		ContainerID: "test-module-id",
		Name:        "MinimalExportMapping",
	}

	data, err := w.serializeExportMapping(em)
	if err != nil {
		t.Fatalf("serializeExportMapping: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	// These fields must be present — verified against Studio Pro-created BSON.
	for _, field := range []string{
		"PublicName",
		"XsdRootElementName",
		"IsHeaderParameter",
		"ParameterName",
		"OperationName",
		"ServiceName",
		"WsdlFile",
	} {
		if _, ok := raw[field]; !ok {
			t.Errorf("missing required field: %s", field)
		}
	}
}

func TestSerializeExportMapping_WithJsonStructureRef(t *testing.T) {
	w := &Writer{}
	em := &model.ExportMapping{
		BaseElement:   model.BaseElement{ID: "test-em-js-ref"},
		ContainerID:   "test-module-id",
		Name:          "ExportWithSchema",
		JsonStructure: "MyModule.PetJsonStructure",
	}

	data, err := w.serializeExportMapping(em)
	if err != nil {
		t.Fatalf("serializeExportMapping: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	assertField(t, raw, "JsonStructure", "MyModule.PetJsonStructure")
}

func TestSerializeExportMapping_NullValueOptionSendAsNil(t *testing.T) {
	w := &Writer{}
	em := &model.ExportMapping{
		BaseElement:     model.BaseElement{ID: "test-em-send-nil"},
		ContainerID:     "test-module-id",
		Name:            "SendNilMapping",
		NullValueOption: "SendAsNil",
	}

	data, err := w.serializeExportMapping(em)
	if err != nil {
		t.Fatalf("serializeExportMapping: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	assertField(t, raw, "NullValueOption", "SendAsNil")
}
