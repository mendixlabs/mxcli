// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/widgets/mpk"
)

func TestAugmentTemplate_AddMissing(t *testing.T) {
	ResetPlaceholderCounter()

	// Create a minimal template with one Enumeration property
	tmpl := &WidgetTemplate{
		WidgetID: "test.Widget",
		Type: map[string]any{
			"$ID":   "type0001",
			"$Type": "CustomWidgets$CustomWidgetType",
			"ObjectType": map[string]any{
				"$ID":   "objtype0001",
				"$Type": "CustomWidgets$WidgetObjectType",
				"PropertyTypes": []any{
					float64(2),
					map[string]any{
						"$ID":         "pt0001",
						"$Type":       "CustomWidgets$WidgetPropertyType",
						"Caption":     "Source",
						"Category":    "General",
						"Description": "",
						"IsDefault":   false,
						"PropertyKey": "source",
						"ValueType": map[string]any{
							"$ID":                         "vt0001",
							"$Type":                       "CustomWidgets$WidgetValueType",
							"Type":                        "Enumeration",
							"DefaultValue":                "a",
							"Required":                    true,
							"IsList":                      false,
							"DataSourceProperty":          "",
							"EnumerationValues":           []any{float64(2)},
							"ObjectType":                  nil,
							"ReturnType":                  nil,
							"AllowNonPersistableEntities": false,
							"AllowedTypes":                []any{float64(1)},
							"AssociationTypes":            []any{float64(1)},
							"DefaultType":                 "None",
							"EntityProperty":              "",
							"IsLinked":                    false,
							"IsMetaData":                  false,
							"IsPath":                      "No",
							"Multiline":                   false,
							"OnChangeProperty":            "",
							"ParameterIsList":             false,
							"PathType":                    "None",
							"SelectableObjectsProperty":   "",
							"SelectionTypes":              []any{float64(1)},
							"SetLabel":                    false,
							"Translations":                []any{float64(2)},
							"ActionVariables":             []any{float64(2)},
						},
					},
				},
			},
		},
		Object: map[string]any{
			"$ID":   "obj0001",
			"$Type": "CustomWidgets$WidgetObject",
			"Properties": []any{
				float64(2),
				map[string]any{
					"$ID":         "prop0001",
					"$Type":       "CustomWidgets$WidgetProperty",
					"TypePointer": "pt0001",
					"Value": map[string]any{
						"$ID":            "val0001",
						"$Type":          "CustomWidgets$WidgetValue",
						"PrimitiveValue": "a",
						"TypePointer":    "vt0001",
						"Action": map[string]any{
							"$ID":                     "act0001",
							"$Type":                   "Forms$NoAction",
							"DisabledDuringExecution": true,
						},
						"AttributeRef":      nil,
						"DataSource":        nil,
						"EntityRef":         nil,
						"Expression":        "",
						"Form":              "",
						"Icon":              nil,
						"Image":             "",
						"Microflow":         "",
						"Nanoflow":          "",
						"Objects":           []any{float64(2)},
						"Selection":         "None",
						"SourceVariable":    nil,
						"TextTemplate":      nil,
						"TranslatableValue": nil,
						"Widgets":           []any{float64(2)},
						"XPathConstraint":   "",
					},
				},
			},
		},
	}

	// MPK definition has source + an extra boolean property
	def := &mpk.WidgetDefinition{
		ID:      "test.Widget",
		Name:    "Test Widget",
		Version: "1.0.0",
		Properties: []mpk.PropertyDef{
			{Key: "source", Type: "enumeration", DefaultValue: "a"},
			{Key: "clearable", Type: "boolean", Caption: "Clearable", DefaultValue: "true"},
		},
	}

	err := AugmentTemplate(tmpl, def)
	if err != nil {
		t.Fatalf("AugmentTemplate failed: %v", err)
	}

	// Check that PropertyTypes now has 2 entries (plus the marker)
	objType := tmpl.Type["ObjectType"].(map[string]any)
	propTypes := objType["PropertyTypes"].([]any)

	propertyTypeCount := 0
	for _, pt := range propTypes {
		if _, ok := pt.(map[string]any); ok {
			propertyTypeCount++
		}
	}
	if propertyTypeCount != 2 {
		t.Errorf("expected 2 PropertyTypes, got %d", propertyTypeCount)
	}

	// Check that the new PropertyType has correct key
	newPT := propTypes[len(propTypes)-1].(map[string]any)
	if newPT["PropertyKey"] != "clearable" {
		t.Errorf("expected PropertyKey 'clearable', got %v", newPT["PropertyKey"])
	}

	// Check that Object.Properties also has 2 entries
	objProps := tmpl.Object["Properties"].([]any)
	propertyCount := 0
	for _, p := range objProps {
		if _, ok := p.(map[string]any); ok {
			propertyCount++
		}
	}
	if propertyCount != 2 {
		t.Errorf("expected 2 Properties, got %d", propertyCount)
	}
}

func TestAugmentTemplate_RemoveStale(t *testing.T) {
	ResetPlaceholderCounter()

	// Template has 2 properties: source and oldProp
	tmpl := &WidgetTemplate{
		WidgetID: "test.Widget",
		Type: map[string]any{
			"$ID":   "type0001",
			"$Type": "CustomWidgets$CustomWidgetType",
			"ObjectType": map[string]any{
				"$ID":   "objtype0001",
				"$Type": "CustomWidgets$WidgetObjectType",
				"PropertyTypes": []any{
					float64(2),
					map[string]any{
						"$ID":         "pt0001",
						"$Type":       "CustomWidgets$WidgetPropertyType",
						"PropertyKey": "source",
						"ValueType": map[string]any{
							"$ID":   "vt0001",
							"$Type": "CustomWidgets$WidgetValueType",
							"Type":  "Enumeration",
						},
					},
					map[string]any{
						"$ID":         "pt0002",
						"$Type":       "CustomWidgets$WidgetPropertyType",
						"PropertyKey": "oldProp",
						"ValueType": map[string]any{
							"$ID":   "vt0002",
							"$Type": "CustomWidgets$WidgetValueType",
							"Type":  "Boolean",
						},
					},
				},
			},
		},
		Object: map[string]any{
			"$ID":   "obj0001",
			"$Type": "CustomWidgets$WidgetObject",
			"Properties": []any{
				float64(2),
				map[string]any{
					"$ID":         "prop0001",
					"$Type":       "CustomWidgets$WidgetProperty",
					"TypePointer": "pt0001",
				},
				map[string]any{
					"$ID":         "prop0002",
					"$Type":       "CustomWidgets$WidgetProperty",
					"TypePointer": "pt0002",
				},
			},
		},
	}

	// MPK only has source (oldProp was removed)
	def := &mpk.WidgetDefinition{
		ID: "test.Widget",
		Properties: []mpk.PropertyDef{
			{Key: "source", Type: "enumeration"},
		},
	}

	err := AugmentTemplate(tmpl, def)
	if err != nil {
		t.Fatalf("AugmentTemplate failed: %v", err)
	}

	// Check PropertyTypes: should have 1 entry
	objType := tmpl.Type["ObjectType"].(map[string]any)
	propTypes := objType["PropertyTypes"].([]any)
	propertyTypeCount := 0
	for _, pt := range propTypes {
		if ptMap, ok := pt.(map[string]any); ok {
			propertyTypeCount++
			if ptMap["PropertyKey"] == "oldProp" {
				t.Error("oldProp should have been removed from PropertyTypes")
			}
		}
	}
	if propertyTypeCount != 1 {
		t.Errorf("expected 1 PropertyType, got %d", propertyTypeCount)
	}

	// Check Properties: should have 1 entry
	objProps := tmpl.Object["Properties"].([]any)
	propertyCount := 0
	for _, p := range objProps {
		if pMap, ok := p.(map[string]any); ok {
			propertyCount++
			if pMap["TypePointer"] == "pt0002" {
				t.Error("property with TypePointer pt0002 should have been removed")
			}
		}
	}
	if propertyCount != 1 {
		t.Errorf("expected 1 Property, got %d", propertyCount)
	}
}

func TestAugmentTemplate_NoChange(t *testing.T) {
	ResetPlaceholderCounter()

	tmpl := &WidgetTemplate{
		WidgetID: "test.Widget",
		Type: map[string]any{
			"ObjectType": map[string]any{
				"PropertyTypes": []any{
					float64(2),
					map[string]any{
						"$ID":         "pt0001",
						"PropertyKey": "source",
						"ValueType": map[string]any{
							"$ID":  "vt0001",
							"Type": "Enumeration",
						},
					},
				},
			},
		},
		Object: map[string]any{
			"Properties": []any{
				float64(2),
				map[string]any{
					"TypePointer": "pt0001",
				},
			},
		},
	}

	// MPK has exactly the same property
	def := &mpk.WidgetDefinition{
		ID: "test.Widget",
		Properties: []mpk.PropertyDef{
			{Key: "source", Type: "enumeration"},
		},
	}

	err := AugmentTemplate(tmpl, def)
	if err != nil {
		t.Fatalf("AugmentTemplate failed: %v", err)
	}

	// Should still have exactly 1 property type
	objType := tmpl.Type["ObjectType"].(map[string]any)
	propTypes := objType["PropertyTypes"].([]any)
	count := 0
	for _, pt := range propTypes {
		if _, ok := pt.(map[string]any); ok {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 PropertyType (no change), got %d", count)
	}
}

func TestAugmentTemplate_SystemPropsIgnored(t *testing.T) {
	ResetPlaceholderCounter()

	// Template has Label system property
	tmpl := &WidgetTemplate{
		WidgetID: "test.Widget",
		Type: map[string]any{
			"ObjectType": map[string]any{
				"PropertyTypes": []any{
					float64(2),
					map[string]any{
						"$ID":         "pt0001",
						"PropertyKey": "source",
						"ValueType":   map[string]any{"$ID": "vt0001", "Type": "Enumeration"},
					},
					map[string]any{
						"$ID":         "pt0002",
						"PropertyKey": "Label",
						"ValueType":   map[string]any{"$ID": "vt0002", "Type": "System"},
					},
				},
			},
		},
		Object: map[string]any{
			"Properties": []any{
				float64(2),
				map[string]any{"TypePointer": "pt0001"},
				map[string]any{"TypePointer": "pt0002"},
			},
		},
	}

	// MPK has source + Label system prop (Label should not be removed even though
	// it's only in SystemProps, not Properties)
	def := &mpk.WidgetDefinition{
		ID: "test.Widget",
		Properties: []mpk.PropertyDef{
			{Key: "source", Type: "enumeration"},
		},
		SystemProps: []mpk.PropertyDef{
			{Key: "Label", IsSystem: true},
		},
	}

	err := AugmentTemplate(tmpl, def)
	if err != nil {
		t.Fatalf("AugmentTemplate failed: %v", err)
	}

	// Label should still be in PropertyTypes (system props are not touched)
	objType := tmpl.Type["ObjectType"].(map[string]any)
	propTypes := objType["PropertyTypes"].([]any)
	count := 0
	for _, pt := range propTypes {
		if _, ok := pt.(map[string]any); ok {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 PropertyTypes (Label preserved), got %d", count)
	}
}

func TestAugmentTemplate_NilInputs(t *testing.T) {
	// Should not panic
	err := AugmentTemplate(nil, nil)
	if err != nil {
		t.Errorf("expected nil error for nil inputs, got: %v", err)
	}

	err = AugmentTemplate(&WidgetTemplate{}, nil)
	if err != nil {
		t.Errorf("expected nil error for nil def, got: %v", err)
	}
}

func TestXmlTypeToBSONType(t *testing.T) {
	tests := []struct {
		xml  string
		bson string
	}{
		{"attribute", "Attribute"},
		{"expression", "Expression"},
		{"textTemplate", "TextTemplate"},
		{"widgets", "Widgets"},
		{"enumeration", "Enumeration"},
		{"boolean", "Boolean"},
		{"integer", "Integer"},
		{"datasource", "DataSource"},
		{"action", "Action"},
		{"selection", "Selection"},
		{"association", "Association"},
		{"object", "Object"},
		{"string", "String"},
		{"decimal", "Decimal"},
		{"unknownType", ""},
	}

	for _, tt := range tests {
		result := xmlTypeToBSONType(tt.xml)
		if result != tt.bson {
			t.Errorf("xmlTypeToBSONType(%q) = %q, want %q", tt.xml, result, tt.bson)
		}
	}
}

func TestDeepCloneTemplate(t *testing.T) {
	original := &WidgetTemplate{
		WidgetID: "test.Widget",
		Name:     "Test",
		Type: map[string]any{
			"key": "value",
			"nested": map[string]any{
				"inner": "data",
			},
		},
		Object: map[string]any{
			"prop": "val",
		},
	}

	clone, err := deepCloneTemplate(original)
	if err != nil {
		t.Fatalf("deepCloneTemplate failed: %v", err)
	}

	// Modify clone
	clone.Type["key"] = "modified"
	clone.Type["nested"].(map[string]any)["inner"] = "modified"

	// Original should be unchanged
	if original.Type["key"] != "value" {
		t.Error("original Type was modified")
	}
	if original.Type["nested"].(map[string]any)["inner"] != "data" {
		t.Error("original nested Type was modified")
	}
}

func TestCreatePropertyPair_TextTemplate(t *testing.T) {
	ResetPlaceholderCounter()

	p := mpk.PropertyDef{
		Key:     "myTextProp",
		Type:    "textTemplate",
		Caption: "My Text",
	}

	pt, prop := createPropertyPair(p, "TextTemplate")
	if pt == nil {
		t.Fatal("PropertyType should not be nil")
	}
	if prop == nil {
		t.Fatal("Property should not be nil")
	}

	// Check that the Value has a TextTemplate (not nil)
	val := prop["Value"].(map[string]any)
	tt := val["TextTemplate"]
	if tt == nil {
		t.Fatal("TextTemplate should not be nil for textTemplate type")
	}
	ttMap := tt.(map[string]any)
	if ttMap["$Type"] != "Forms$ClientTemplate" {
		t.Errorf("expected Forms$ClientTemplate, got %v", ttMap["$Type"])
	}
}

func TestAugmentTemplate_WithRealTemplate(t *testing.T) {
	// Load the actual combobox template and augment with a mock definition
	// that adds one extra boolean property
	tmpl, err := GetTemplate("com.mendix.widget.web.combobox.Combobox")
	if err != nil {
		t.Fatalf("GetTemplate failed: %v", err)
	}
	if tmpl == nil {
		t.Skip("ComboBox template not available")
	}

	ResetPlaceholderCounter()
	clone, err := deepCloneTemplate(tmpl)
	if err != nil {
		t.Fatalf("deepCloneTemplate failed: %v", err)
	}

	// Count original properties
	objType := clone.Type["ObjectType"].(map[string]any)
	propTypes := objType["PropertyTypes"].([]any)
	originalCount := 0
	for _, pt := range propTypes {
		if _, ok := pt.(map[string]any); ok {
			originalCount++
		}
	}

	// Create a definition that has all existing properties plus one new one
	def := &mpk.WidgetDefinition{
		ID:      "com.mendix.widget.web.combobox.Combobox",
		Version: "3.0.0",
	}

	// Copy existing properties from template
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		key, _ := ptMap["PropertyKey"].(string)
		vt, _ := ptMap["ValueType"].(map[string]any)
		vtType := ""
		if vt != nil {
			vtType, _ = vt["Type"].(string)
		}
		if vtType == "System" {
			def.SystemProps = append(def.SystemProps, mpk.PropertyDef{Key: key, IsSystem: true})
		} else {
			xmlType := bsonTypeToXmlType(vtType)
			def.Properties = append(def.Properties, mpk.PropertyDef{Key: key, Type: xmlType})
		}
	}

	// Add one new property
	def.Properties = append(def.Properties, mpk.PropertyDef{
		Key:          "newTestProperty",
		Type:         "boolean",
		Caption:      "New Test Property",
		DefaultValue: "false",
	})

	err = AugmentTemplate(clone, def)
	if err != nil {
		t.Fatalf("AugmentTemplate failed: %v", err)
	}

	// Check that we now have originalCount + 1 property types
	updatedPropTypes := objType["PropertyTypes"].([]any)
	newCount := 0
	for _, pt := range updatedPropTypes {
		if _, ok := pt.(map[string]any); ok {
			newCount++
		}
	}
	if newCount != originalCount+1 {
		t.Errorf("expected %d PropertyTypes, got %d", originalCount+1, newCount)
	}

	// Count original Object.Properties (may differ from PropertyTypes due to system props)
	origObjProps := tmpl.Object["Properties"].([]any)
	origPropCount := 0
	for _, p := range origObjProps {
		if _, ok := p.(map[string]any); ok {
			origPropCount++
		}
	}

	// Check the Properties in Object also increased by 1
	objProps := clone.Object["Properties"].([]any)
	propCount := 0
	for _, p := range objProps {
		if _, ok := p.(map[string]any); ok {
			propCount++
		}
	}
	if propCount != origPropCount+1 {
		t.Errorf("expected %d Properties, got %d", origPropCount+1, propCount)
	}
}

// TestAugmentTemplate_NoPlaceholderLeakAfterBSONConversion verifies that after
// augmentation and BSON conversion, no placeholder IDs remain. This is the bug
// from issue #6: regenerateNestedIDs overwrote ValueType.$ID after newVTID was
// captured, causing Value.TypePointer to reference an unmapped placeholder.
func TestAugmentTemplate_NoPlaceholderLeakAfterBSONConversion(t *testing.T) {
	ResetPlaceholderCounter()

	tmpl, err := GetTemplate("com.mendix.widget.web.combobox.Combobox")
	if err != nil {
		t.Fatalf("GetTemplate failed: %v", err)
	}
	if tmpl == nil {
		t.Skip("ComboBox template not available")
	}

	clone, err := deepCloneTemplate(tmpl)
	if err != nil {
		t.Fatalf("deepCloneTemplate failed: %v", err)
	}

	// Build a definition with existing properties + one new one
	objType := clone.Type["ObjectType"].(map[string]any)
	propTypes := objType["PropertyTypes"].([]any)
	def := &mpk.WidgetDefinition{
		ID:      "com.mendix.widget.web.combobox.Combobox",
		Version: "3.0.0",
	}
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		key, _ := ptMap["PropertyKey"].(string)
		vt, _ := ptMap["ValueType"].(map[string]any)
		vtType := ""
		if vt != nil {
			vtType, _ = vt["Type"].(string)
		}
		if vtType == "System" {
			def.SystemProps = append(def.SystemProps, mpk.PropertyDef{Key: key, IsSystem: true})
		} else {
			xmlType := bsonTypeToXmlType(vtType)
			def.Properties = append(def.Properties, mpk.PropertyDef{Key: key, Type: xmlType})
		}
	}
	def.Properties = append(def.Properties, mpk.PropertyDef{
		Key:          "extraBoolProp",
		Type:         "boolean",
		Caption:      "Extra Bool",
		DefaultValue: "false",
	})

	if err := AugmentTemplate(clone, def); err != nil {
		t.Fatalf("AugmentTemplate failed: %v", err)
	}

	// Now run the full BSON conversion pipeline (same as GetTemplateFullBSON)
	counter := 0
	idGen := func() string {
		counter++
		return fmt.Sprintf("bbbbbbbb0000000000000000%08x", counter)
	}

	idMapping := make(map[string]string)
	collectIDs(clone.Type, idGen, idMapping)
	if clone.Object != nil {
		collectIDs(clone.Object, idGen, idMapping)
	}

	var objectTypeID string
	propertyTypeIDs := make(map[string]PropertyTypeIDEntry)
	bsonType := jsonToBSONWithMappingAndObjectType(clone.Type, idMapping, propertyTypeIDs, &objectTypeID)
	bsonObject := jsonToBSONObjectWithMapping(clone.Object, idMapping)

	if containsPlaceholderID(bsonType) {
		t.Error("placeholder ID leak in Type BSON after augmentation")
	}
	if containsPlaceholderID(bsonObject) {
		t.Error("placeholder ID leak in Object BSON after augmentation — issue #6 regression")
	}
}

// bsonTypeToXmlType is a test helper to reverse the mapping.
func bsonTypeToXmlType(bsonType string) string {
	switch bsonType {
	case "Attribute":
		return "attribute"
	case "Expression":
		return "expression"
	case "TextTemplate":
		return "textTemplate"
	case "Widgets":
		return "widgets"
	case "Enumeration":
		return "enumeration"
	case "Boolean":
		return "boolean"
	case "Integer":
		return "integer"
	case "DataSource":
		return "datasource"
	case "Action":
		return "action"
	case "Selection":
		return "selection"
	case "Association":
		return "association"
	case "Object":
		return "object"
	case "String":
		return "string"
	case "Decimal":
		return "decimal"
	default:
		return "string"
	}
}

func TestAugmentTemplate_NestedObjectType_AddMissing(t *testing.T) {
	ResetPlaceholderCounter()

	// Template has an Object-type property "columns" with one nested property "header"
	tmpl := &WidgetTemplate{
		WidgetID: "test.DataGrid",
		Type: map[string]any{
			"$ID":   "type0001",
			"$Type": "CustomWidgets$CustomWidgetType",
			"ObjectType": map[string]any{
				"$ID":   "objtype0001",
				"$Type": "CustomWidgets$WidgetObjectType",
				"PropertyTypes": []any{
					float64(2),
					map[string]any{
						"$ID":         "pt-columns",
						"$Type":       "CustomWidgets$WidgetPropertyType",
						"Caption":     "Columns",
						"Category":    "General",
						"Description": "",
						"IsDefault":   false,
						"PropertyKey": "columns",
						"ValueType": map[string]any{
							"$ID":      "vt-columns",
							"$Type":    "CustomWidgets$WidgetValueType",
							"Type":     "Object",
							"IsList":   true,
							"Required": false,
							"ObjectType": map[string]any{
								"$ID":   "nested-objtype",
								"$Type": "CustomWidgets$WidgetObjectType",
								"PropertyTypes": []any{
									float64(2),
									map[string]any{
										"$ID":         "npt-header",
										"$Type":       "CustomWidgets$WidgetPropertyType",
										"Caption":     "Header",
										"Category":    "General",
										"Description": "",
										"IsDefault":   false,
										"PropertyKey": "header",
										"ValueType": map[string]any{
											"$ID":          "nvt-header",
											"$Type":        "CustomWidgets$WidgetValueType",
											"Type":         "TextTemplate",
											"DefaultValue": "",
											"Required":     false,
											"IsList":       false,
										},
									},
								},
							},
							"DefaultValue":                "",
							"DataSourceProperty":          "",
							"EnumerationValues":           []any{float64(2)},
							"ReturnType":                  nil,
							"AllowNonPersistableEntities": false,
							"AllowedTypes":                []any{float64(1)},
							"AssociationTypes":            []any{float64(1)},
							"DefaultType":                 "None",
							"EntityProperty":              "",
							"IsLinked":                    false,
						},
					},
				},
			},
		},
		Object: map[string]any{
			"$ID":   "obj0001",
			"$Type": "CustomWidgets$WidgetObject",
			"Properties": []any{
				float64(2),
				map[string]any{
					"$ID":         "prop-columns",
					"$Type":       "CustomWidgets$WidgetProperty",
					"TypePointer": "pt-columns",
					"Value": map[string]any{
						"$ID":         "val-columns",
						"$Type":       "CustomWidgets$WidgetValue",
						"TypePointer": "vt-columns",
						"Objects": []any{
							float64(2),
							map[string]any{
								"$ID":         "col-obj-1",
								"$Type":       "CustomWidgets$WidgetObject",
								"TypePointer": "nested-objtype",
								"Properties": []any{
									float64(2),
									map[string]any{
										"$ID":         "col-prop-header",
										"$Type":       "CustomWidgets$WidgetProperty",
										"TypePointer": "npt-header",
										"Value": map[string]any{
											"$ID":         "col-val-header",
											"$Type":       "CustomWidgets$WidgetValue",
											"TypePointer": "nvt-header",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// .mpk defines "columns" with two children: "header" (existing) and "tooltip" (new)
	def := &mpk.WidgetDefinition{
		ID:   "test.DataGrid",
		Name: "DataGrid",
		Properties: []mpk.PropertyDef{
			{
				Key:  "columns",
				Type: "object",
				Children: []mpk.PropertyDef{
					{Key: "header", Type: "textTemplate", Caption: "Header"},
					{Key: "tooltip", Type: "textTemplate", Caption: "Tooltip"},
				},
			},
		},
	}

	err := AugmentTemplate(tmpl, def)
	if err != nil {
		t.Fatalf("AugmentTemplate failed: %v", err)
	}

	// Verify nested ObjectType now has 2 PropertyTypes
	objType, _ := getMapField(tmpl.Type, "ObjectType")
	propTypes, _ := getArrayField(objType, "PropertyTypes")
	// Find the columns PropertyType
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		if ptMap["PropertyKey"] != "columns" {
			continue
		}
		vt, _ := getMapField(ptMap, "ValueType")
		nestedOT, _ := getMapField(vt, "ObjectType")
		nestedPTs, _ := getArrayField(nestedOT, "PropertyTypes")

		// Count non-marker entries
		count := 0
		hasTooltip := false
		for _, npt := range nestedPTs {
			nptMap, ok := npt.(map[string]any)
			if !ok {
				continue
			}
			count++
			if nptMap["PropertyKey"] == "tooltip" {
				hasTooltip = true
			}
		}
		if count != 2 {
			t.Errorf("expected 2 nested PropertyTypes, got %d", count)
		}
		if !hasTooltip {
			t.Error("expected nested PropertyType 'tooltip' to be added")
		}
	}

	// Verify nested Object Properties also has the new property
	objProps, _ := getArrayField(tmpl.Object, "Properties")
	for _, prop := range objProps {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		if propMap["TypePointer"] != "pt-columns" {
			continue
		}
		val, _ := getMapField(propMap, "Value")
		objects, _ := getArrayField(val, "Objects")
		for _, obj := range objects {
			objMap, ok := obj.(map[string]any)
			if !ok {
				continue
			}
			nestedProps, _ := getArrayField(objMap, "Properties")
			count := 0
			for _, np := range nestedProps {
				if _, ok := np.(map[string]any); ok {
					count++
				}
			}
			if count != 2 {
				t.Errorf("expected 2 nested Object Properties, got %d", count)
			}
		}
	}
}

func TestResetGeneratedCacheExists(t *testing.T) {
	// Should compile — confirms ResetGeneratedCache is present
	ResetGeneratedCache()
}

func TestGetTemplateFullBSONAcceptsDataSourceProperty(t *testing.T) {
	// GetTemplateFullBSON should use getOrGenerateTemplate internally
	// (compile-time: no test for runtime without real MPK)
	_, _, _, _, _, err := GetTemplateFullBSON("nonexistent", func() string { return "" }, "")
	// nil result is OK for unknown widget; error should be nil
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
