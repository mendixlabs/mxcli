// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/widgets/mpk"
)

func TestGenerateFromMPK_BasicTypes(t *testing.T) {
	ResetPlaceholderCounter()

	def := &mpk.WidgetDefinition{
		ID:      "com.example.Widget",
		Name:    "Test Widget",
		Version: "1.0.0",
		Properties: []mpk.PropertyDef{
			{Key: "label", Type: "string", Caption: "Label"},
			{Key: "enabled", Type: "boolean", Caption: "Enabled", DefaultValue: "true"},
			{Key: "count", Type: "integer", Caption: "Count", DefaultValue: "0"},
			{Key: "value", Type: "expression", Caption: "Value"},
			{Key: "attr", Type: "attribute", Caption: "Attribute"},
		},
	}

	tmpl := GenerateFromMPK(def)

	if tmpl == nil {
		t.Fatal("GenerateFromMPK returned nil")
	}
	if tmpl.WidgetID != def.ID {
		t.Errorf("WidgetID = %q, want %q", tmpl.WidgetID, def.ID)
	}
	if tmpl.Name != def.Name {
		t.Errorf("Name = %q, want %q", tmpl.Name, def.Name)
	}
	if tmpl.Version != def.Version {
		t.Errorf("Version = %q, want %q", tmpl.Version, def.Version)
	}
	if tmpl.Type == nil {
		t.Fatal("Type is nil")
	}
	if tmpl.Object == nil {
		t.Fatal("Object is nil")
	}

	if got := tmpl.Type["$Type"]; got != "CustomWidgets$CustomWidgetType" {
		t.Errorf("Type.$Type = %v, want CustomWidgets$CustomWidgetType", got)
	}

	objType, ok := tmpl.Type["ObjectType"].(map[string]any)
	if !ok {
		t.Fatal("Type.ObjectType missing or wrong type")
	}
	propTypes, ok := objType["PropertyTypes"].([]any)
	if !ok {
		t.Fatal("ObjectType.PropertyTypes missing or wrong type")
	}
	nonMarkerPropTypes := 0
	for _, pt := range propTypes {
		if _, isFloat := pt.(float64); !isFloat {
			nonMarkerPropTypes++
		}
	}
	if nonMarkerPropTypes != 5 {
		t.Errorf("PropertyTypes count = %d, want 5", nonMarkerPropTypes)
	}

	objProps, ok := tmpl.Object["Properties"].([]any)
	if !ok {
		t.Fatal("Object.Properties missing or wrong type")
	}
	nonMarkerProps := 0
	for _, p := range objProps {
		if _, isFloat := p.(float64); !isFloat {
			nonMarkerProps++
		}
	}
	if nonMarkerProps != 5 {
		t.Errorf("Properties count = %d, want 5", nonMarkerProps)
	}
}

func TestGenerateFromMPK_TypePointerCrossReference(t *testing.T) {
	ResetPlaceholderCounter()

	def := &mpk.WidgetDefinition{
		ID:   "com.example.Widget",
		Name: "Test Widget",
		Properties: []mpk.PropertyDef{
			{Key: "mode", Type: "enumeration", Caption: "Mode", DefaultValue: "fast"},
		},
	}

	tmpl := GenerateFromMPK(def)

	objType := tmpl.Type["ObjectType"].(map[string]any)
	objTypeID := objType["$ID"].(string)
	propTypes := objType["PropertyTypes"].([]any)
	var ptID string
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		if ptMap["PropertyKey"] == "mode" {
			ptID = ptMap["$ID"].(string)
			break
		}
	}
	if ptID == "" {
		t.Fatal("PropertyType for 'mode' not found")
	}

	objProps := tmpl.Object["Properties"].([]any)
	var propTypePointer string
	for _, p := range objProps {
		pMap, ok := p.(map[string]any)
		if !ok {
			continue
		}
		propTypePointer = pMap["TypePointer"].(string)
		break
	}
	if propTypePointer != ptID {
		t.Errorf("Property.TypePointer = %q, want PropertyType.$ID %q", propTypePointer, ptID)
	}

	// Also verify the top-level objectMap TypePointer → WidgetObjectType.$ID
	gotObjTP := tmpl.Object["TypePointer"].(string)
	if gotObjTP != objTypeID {
		t.Errorf("Object.TypePointer = %q, want ObjectType.$ID %q", gotObjTP, objTypeID)
	}
}

func TestGenerateFromMPK_NestedObject(t *testing.T) {
	ResetPlaceholderCounter()

	def := &mpk.WidgetDefinition{
		ID:   "com.example.Widget",
		Name: "Test Widget",
		Properties: []mpk.PropertyDef{
			{
				Key:     "columns",
				Type:    "object",
				Caption: "Columns",
				IsList:  true,
				Children: []mpk.PropertyDef{
					{Key: "header", Type: "string", Caption: "Header"},
					{Key: "attr", Type: "attribute", Caption: "Attribute"},
				},
			},
		},
	}

	tmpl := GenerateFromMPK(def)

	objType := tmpl.Type["ObjectType"].(map[string]any)
	propTypes := objType["PropertyTypes"].([]any)
	var columnsPT map[string]any
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		if ptMap["PropertyKey"] == "columns" {
			columnsPT = ptMap
			break
		}
	}
	if columnsPT == nil {
		t.Fatal("columns PropertyType not found")
	}

	vt, ok := columnsPT["ValueType"].(map[string]any)
	if !ok {
		t.Fatal("ValueType missing on columns property")
	}
	nestedObjType, ok := vt["ObjectType"].(map[string]any)
	if !ok {
		t.Fatal("ObjectType missing on columns ValueType — nested object not built")
	}
	nestedPTs, ok := nestedObjType["PropertyTypes"].([]any)
	if !ok {
		t.Fatal("nested PropertyTypes missing")
	}
	nestedCount := 0
	for _, npt := range nestedPTs {
		if _, isFloat := npt.(float64); !isFloat {
			nestedCount++
		}
	}
	if nestedCount != 2 {
		t.Errorf("nested PropertyTypes count = %d, want 2", nestedCount)
	}
}

func TestGenerateFromMPK_UnknownTypeSkipped(t *testing.T) {
	ResetPlaceholderCounter()

	def := &mpk.WidgetDefinition{
		ID:   "com.example.Widget",
		Name: "Test Widget",
		Properties: []mpk.PropertyDef{
			{Key: "good", Type: "string", Caption: "Good"},
			{Key: "bad", Type: "unknownXmlType", Caption: "Bad"},
		},
	}

	tmpl := GenerateFromMPK(def)

	objType := tmpl.Type["ObjectType"].(map[string]any)
	propTypes := objType["PropertyTypes"].([]any)
	count := 0
	for _, pt := range propTypes {
		if _, isFloat := pt.(float64); !isFloat {
			count++
		}
	}
	if count != 1 {
		t.Errorf("PropertyTypes count = %d, want 1 (unknown type skipped)", count)
	}
}

func TestGenerateFromMPK_PlaceholderIDsRemapped(t *testing.T) {
	ResetPlaceholderCounter()

	def := &mpk.WidgetDefinition{
		ID:   "com.example.Widget",
		Name: "Test Widget",
		Properties: []mpk.PropertyDef{
			{Key: "label", Type: "string", Caption: "Label"},
		},
	}

	tmpl := GenerateFromMPK(def)

	callCount := 0
	idGen := func() string {
		callCount++
		return strings.Repeat("f", 32)
	}

	templateCacheLock.Lock()
	templateCache["com.example.Widget"] = tmpl
	templateCacheLock.Unlock()
	defer func() {
		templateCacheLock.Lock()
		delete(templateCache, "com.example.Widget")
		templateCacheLock.Unlock()
	}()

	bsonType, bsonObj, _, _, _, err := GetTemplateFullBSON("com.example.Widget", idGen, "")
	if err != nil {
		t.Fatalf("GetTemplateFullBSON: %v", err)
	}
	if containsPlaceholderID(bsonType) {
		t.Error("placeholder IDs leaked in bsonType")
	}
	if bsonObj != nil && containsPlaceholderID(bsonObj) {
		t.Error("placeholder IDs leaked in bsonObj")
	}
	if callCount == 0 {
		t.Error("idGen was never called — IDs were not remapped")
	}
}
