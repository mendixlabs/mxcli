// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

func TestSerializeJsonStructure_Basic(t *testing.T) {
	w := &Writer{}
	js := &model.JsonStructure{
		BaseElement: model.BaseElement{
			ID:       "test-js-id",
			TypeName: "JsonStructures$JsonStructure",
		},
		ContainerID: "test-module-id",
		Name:        "PetResponse",
		JsonSnippet: `{"id": 1, "name": "Fido"}`,
		ExportLevel: "Hidden",
	}

	data, err := w.serializeJsonStructure(js)
	if err != nil {
		t.Fatalf("serializeJsonStructure: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	assertField(t, raw, "$Type", "JsonStructures$JsonStructure")
	assertField(t, raw, "Name", "PetResponse")
	assertField(t, raw, "ExportLevel", "Hidden")
	assertField(t, raw, "JsonSnippet", `{"id": 1, "name": "Fido"}`)

	if raw["Elements"] == nil {
		t.Error("Elements: expected non-nil")
	}
}

func TestSerializeJsonStructure_DefaultExportLevel(t *testing.T) {
	w := &Writer{}
	js := &model.JsonStructure{
		BaseElement: model.BaseElement{ID: "test-js-default"},
		ContainerID: "test-module-id",
		Name:        "SomeStructure",
		// ExportLevel intentionally omitted — should default to "Hidden"
	}

	data, err := w.serializeJsonStructure(js)
	if err != nil {
		t.Fatalf("serializeJsonStructure: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	assertField(t, raw, "ExportLevel", "Hidden")
}

func TestSerializeJsonStructure_WithElements(t *testing.T) {
	w := &Writer{}
	js := &model.JsonStructure{
		BaseElement: model.BaseElement{ID: "test-js-elems"},
		ContainerID: "test-module-id",
		Name:        "WithElements",
		ExportLevel: "Hidden",
		Elements: []*model.JsonElement{
			{
				BaseElement:   model.BaseElement{ID: "root-id"},
				ExposedName:   "Root",
				ElementType:   "Object",
				PrimitiveType: "Unknown",
				MaxOccurs:     1,
				MaxLength:     -1,
				Path:          "(Object)",
				Children: []*model.JsonElement{
					{
						BaseElement:   model.BaseElement{ID: "id-elem-id"},
						ExposedName:   "Id",
						ElementType:   "Value",
						PrimitiveType: "Integer",
						MaxOccurs:     1,
						MaxLength:     -1,
						Path:          "(Object)|id",
					},
				},
			},
		},
	}

	data, err := w.serializeJsonStructure(js)
	if err != nil {
		t.Fatalf("serializeJsonStructure: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	// Elements array must have version marker + 1 element
	elems := extractBsonArray(raw["Elements"])
	if len(elems) != 1 {
		t.Fatalf("Elements: expected 1 element, got %d", len(elems))
	}

	rootElem, ok := elems[0].(map[string]any)
	if !ok {
		t.Fatalf("Elements[0]: expected map, got %T", elems[0])
	}
	assertField(t, rootElem, "$Type", "JsonStructures$JsonElement")
	assertField(t, rootElem, "ExposedName", "Root")
	assertField(t, rootElem, "ElementType", "Object")

	// Root should have 1 child
	children := extractBsonArray(rootElem["Children"])
	if len(children) != 1 {
		t.Fatalf("Root.Children: expected 1, got %d", len(children))
	}
	child, ok := children[0].(map[string]any)
	if !ok {
		t.Fatalf("Children[0]: expected map, got %T", children[0])
	}
	assertField(t, child, "ExposedName", "Id")
	assertField(t, child, "PrimitiveType", "Integer")
}

// --- DeriveJsonElementsFromSnippet ---

func TestDeriveJsonElementsFromSnippet_SimpleObject(t *testing.T) {
	snippet := `{"id": 1, "name": "test", "active": true}`
	elements, err := DeriveJsonElementsFromSnippet(snippet)
	if err != nil {
		t.Fatalf("DeriveJsonElementsFromSnippet: %v", err)
	}

	if len(elements) != 1 {
		t.Fatalf("expected 1 root element, got %d", len(elements))
	}

	root := elements[0]
	if root.ElementType != "Object" {
		t.Errorf("root.ElementType = %q, want %q", root.ElementType, "Object")
	}
	if root.ExposedName != "Root" {
		t.Errorf("root.ExposedName = %q, want %q", root.ExposedName, "Root")
	}
	if root.MinOccurs != 1 {
		t.Errorf("root.MinOccurs = %d, want 1 (root must be required)", root.MinOccurs)
	}

	// Children are sorted alphabetically: Active, Id, Name
	if len(root.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(root.Children))
	}

	byName := map[string]*model.JsonElement{}
	for _, c := range root.Children {
		byName[c.ExposedName] = c
	}

	if pt := byName["Id"].PrimitiveType; pt != "Integer" {
		t.Errorf("Id.PrimitiveType = %q, want Integer", pt)
	}
	if pt := byName["Name"].PrimitiveType; pt != "String" {
		t.Errorf("Name.PrimitiveType = %q, want String", pt)
	}
	if pt := byName["Active"].PrimitiveType; pt != "Boolean" {
		t.Errorf("Active.PrimitiveType = %q, want Boolean", pt)
	}
}

func TestDeriveJsonElementsFromSnippet_WithArray(t *testing.T) {
	snippet := `{"pets": [{"id": 1, "name": "Fido"}]}`
	elements, err := DeriveJsonElementsFromSnippet(snippet)
	if err != nil {
		t.Fatalf("DeriveJsonElementsFromSnippet: %v", err)
	}

	root := elements[0]
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}

	petsElem := root.Children[0]
	if petsElem.ElementType != "Array" {
		t.Errorf("pets.ElementType = %q, want Array", petsElem.ElementType)
	}
	if petsElem.ExposedName != "Pets" {
		t.Errorf("pets.ExposedName = %q, want Pets", petsElem.ExposedName)
	}

	// Array item element must be an Object with MaxOccurs = -1
	if len(petsElem.Children) != 1 {
		t.Fatalf("expected 1 array item child, got %d", len(petsElem.Children))
	}
	itemElem := petsElem.Children[0]
	if itemElem.ElementType != "Object" {
		t.Errorf("array item.ElementType = %q, want Object", itemElem.ElementType)
	}
	if itemElem.MaxOccurs != -1 {
		t.Errorf("array item.MaxOccurs = %d, want -1 (unbounded)", itemElem.MaxOccurs)
	}
}

func TestDeriveJsonElementsFromSnippet_DecimalValue(t *testing.T) {
	snippet := `{"price": 19.99}`
	elements, err := DeriveJsonElementsFromSnippet(snippet)
	if err != nil {
		t.Fatalf("DeriveJsonElementsFromSnippet: %v", err)
	}

	root := elements[0]
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}
	price := root.Children[0]
	if price.PrimitiveType != "Decimal" {
		t.Errorf("price.PrimitiveType = %q, want Decimal", price.PrimitiveType)
	}
}

func TestDeriveJsonElementsFromSnippet_InvalidJson(t *testing.T) {
	_, err := DeriveJsonElementsFromSnippet(`not valid json`)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// --- FormatJsonSnippet ---

func TestFormatJsonSnippet_Pretty(t *testing.T) {
	compact := `{"id":1,"name":"test"}`
	formatted, err := FormatJsonSnippet(compact)
	if err != nil {
		t.Fatalf("FormatJsonSnippet: %v", err)
	}

	if !strings.Contains(formatted, "\n") {
		t.Error("expected multi-line formatted output")
	}
	if !strings.Contains(formatted, `"id"`) {
		t.Error(`expected "id" field in formatted output`)
	}
	if !strings.Contains(formatted, `"name"`) {
		t.Error(`expected "name" field in formatted output`)
	}
}

func TestFormatJsonSnippet_AlreadyFormatted(t *testing.T) {
	pretty := "{\n  \"id\": 1\n}"
	formatted, err := FormatJsonSnippet(pretty)
	if err != nil {
		t.Fatalf("FormatJsonSnippet: %v", err)
	}
	// Should still be valid JSON
	if !strings.Contains(formatted, `"id"`) {
		t.Error(`expected "id" in output`)
	}
}

func TestFormatJsonSnippet_InvalidJson(t *testing.T) {
	_, err := FormatJsonSnippet(`{bad json}`)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
