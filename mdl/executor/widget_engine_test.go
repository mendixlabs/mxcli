// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/json"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

func TestWidgetDefinitionJSONRoundTrip(t *testing.T) {
	original := WidgetDefinition{
		WidgetID:        "com.mendix.widget.web.combobox.Combobox",
		MDLName:         "COMBOBOX",
		TemplateFile:    "combobox.json",
		DefaultEditable: "Always",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "attributeEnumeration", Source: "Attribute", Operation: "attribute"},
			{PropertyKey: "optionsSourceType", Value: "enumeration", Operation: "primitive"},
		},
		ChildSlots: []ChildSlotMapping{
			{PropertyKey: "content", MDLContainer: "TEMPLATE", Operation: "widgets"},
		},
		Modes: []WidgetMode{
			{
				Name:        "association",
				Condition:   "hasDataSource",
				Description: "Association-based ComboBox with datasource",
				PropertyMappings: []PropertyMapping{
					{PropertyKey: "attributeAssociation", Source: "Attribute", Operation: "association"},
					{PropertyKey: "optionsSourceType", Value: "association", Operation: "primitive"},
				},
				ChildSlots: []ChildSlotMapping{
					{PropertyKey: "menuContent", MDLContainer: "MENU", Operation: "widgets"},
				},
			},
		},
	}

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal WidgetDefinition: %v", err)
	}

	var decoded WidgetDefinition
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("failed to unmarshal WidgetDefinition: %v", err)
	}

	// Verify top-level fields
	if decoded.WidgetID != original.WidgetID {
		t.Errorf("WidgetID: got %q, want %q", decoded.WidgetID, original.WidgetID)
	}
	if decoded.MDLName != original.MDLName {
		t.Errorf("MDLName: got %q, want %q", decoded.MDLName, original.MDLName)
	}
	if decoded.DefaultEditable != original.DefaultEditable {
		t.Errorf("DefaultEditable: got %q, want %q", decoded.DefaultEditable, original.DefaultEditable)
	}

	// Verify property mappings
	if len(decoded.PropertyMappings) != len(original.PropertyMappings) {
		t.Fatalf("PropertyMappings count: got %d, want %d", len(decoded.PropertyMappings), len(original.PropertyMappings))
	}
	if decoded.PropertyMappings[0].Operation != "attribute" {
		t.Errorf("PropertyMappings[0].Operation: got %q, want %q", decoded.PropertyMappings[0].Operation, "attribute")
	}

	// Verify child slots
	if len(decoded.ChildSlots) != 1 {
		t.Fatalf("ChildSlots count: got %d, want 1", len(decoded.ChildSlots))
	}
	if decoded.ChildSlots[0].MDLContainer != "TEMPLATE" {
		t.Errorf("ChildSlots[0].MDLContainer: got %q, want %q", decoded.ChildSlots[0].MDLContainer, "TEMPLATE")
	}

	// Verify modes
	if len(decoded.Modes) != 1 {
		t.Fatalf("Modes count: got %d, want 1", len(decoded.Modes))
	}
	assocMode := decoded.Modes[0]
	if assocMode.Name != "association" {
		t.Errorf("Mode name: got %q, want %q", assocMode.Name, "association")
	}
	if assocMode.Condition != "hasDataSource" {
		t.Errorf("Mode condition: got %q, want %q", assocMode.Condition, "hasDataSource")
	}
	if len(assocMode.PropertyMappings) != 2 {
		t.Errorf("Mode PropertyMappings count: got %d, want 2", len(assocMode.PropertyMappings))
	}
	if len(assocMode.ChildSlots) != 1 {
		t.Errorf("Mode ChildSlots count: got %d, want 1", len(assocMode.ChildSlots))
	}
}

func TestWidgetDefinitionJSONOmitsEmptyOptionalFields(t *testing.T) {
	minimal := WidgetDefinition{
		WidgetID:        "com.example.Widget",
		MDLName:         "MYWIDGET",
		TemplateFile:    "mywidget.json",
		DefaultEditable: "Always",
	}

	encoded, err := json.Marshal(minimal)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	// Verify that empty optional fields are omitted from JSON
	omittedFields := []string{"propertyMappings", "childSlots", "modes"}
	for _, field := range omittedFields {
		if _, exists := raw[field]; exists {
			t.Errorf("expected field %q to be omitted when empty, but it was present", field)
		}
	}

	// Verify required fields are present
	requiredFields := []string{"widgetId", "mdlName", "templateFile", "defaultEditable"}
	for _, field := range requiredFields {
		if _, exists := raw[field]; !exists {
			t.Errorf("expected required field %q to be present, but it was missing", field)
		}
	}
}

func TestOperationRegistryLookupFound(t *testing.T) {
	reg := NewOperationRegistry()

	builtinOps := []string{"attribute", "association", "primitive", "selection", "datasource", "widgets"}
	for _, name := range builtinOps {
		fn := reg.Lookup(name)
		if fn == nil {
			t.Errorf("Lookup(%q) returned nil, want non-nil", name)
		}
	}
}

func TestOperationRegistryLookupNotFound(t *testing.T) {
	reg := NewOperationRegistry()

	fn := reg.Lookup("nonexistent")
	if fn != nil {
		t.Error("Lookup(\"nonexistent\") should return nil")
	}
}

func TestOperationRegistryCustomRegistration(t *testing.T) {
	reg := NewOperationRegistry()

	called := false
	reg.Register("custom", func(obj bson.D, propTypeIDs map[string]pages.PropertyTypeIDEntry, propertyKey string, ctx *BuildContext) bson.D {
		called = true
		return obj
	})

	fn := reg.Lookup("custom")
	if fn == nil {
		t.Fatal("Lookup(\"custom\") returned nil after Register")
	}

	fn(bson.D{}, nil, "test", &BuildContext{})
	if !called {
		t.Error("custom operation was not called")
	}
}

// =============================================================================
// PluggableWidgetEngine Tests
// =============================================================================

func TestEvaluateCondition(t *testing.T) {
	engine := &PluggableWidgetEngine{
		operations: NewOperationRegistry(),
	}

	tests := []struct {
		name      string
		condition string
		widget    *ast.WidgetV3
		expected  bool
	}{
		{
			name:      "hasDataSource with datasource present",
			condition: "hasDataSource",
			widget: &ast.WidgetV3{
				Properties: map[string]any{
					"DataSource": &ast.DataSourceV3{Type: "database", Reference: "Module.Entity"},
				},
			},
			expected: true,
		},
		{
			name:      "hasDataSource without datasource",
			condition: "hasDataSource",
			widget:    &ast.WidgetV3{Properties: map[string]any{}},
			expected:  false,
		},
		{
			name:      "hasAttribute with attribute present",
			condition: "hasAttribute",
			widget:    &ast.WidgetV3{Properties: map[string]any{"Attribute": "Name"}},
			expected:  true,
		},
		{
			name:      "hasAttribute without attribute",
			condition: "hasAttribute",
			widget:    &ast.WidgetV3{Properties: map[string]any{}},
			expected:  false,
		},
		{
			name:      "hasProp with matching prop",
			condition: "hasProp:CaptionAttribute",
			widget:    &ast.WidgetV3{Properties: map[string]any{"CaptionAttribute": "DisplayName"}},
			expected:  true,
		},
		{
			name:      "hasProp without matching prop",
			condition: "hasProp:CaptionAttribute",
			widget:    &ast.WidgetV3{Properties: map[string]any{}},
			expected:  false,
		},
		{
			name:      "unknown condition returns false",
			condition: "unknownCondition",
			widget:    &ast.WidgetV3{Properties: map[string]any{}},
			expected:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := engine.evaluateCondition(tc.condition, tc.widget)
			if result != tc.expected {
				t.Errorf("evaluateCondition(%q) = %v, want %v", tc.condition, result, tc.expected)
			}
		})
	}
}

func TestEvaluateConditionUnknownReturnsFalse(t *testing.T) {
	engine := &PluggableWidgetEngine{
		operations: NewOperationRegistry(),
	}

	w := &ast.WidgetV3{Properties: map[string]any{}}
	result := engine.evaluateCondition("typoCondition", w)

	if result != false {
		t.Errorf("expected false for unknown condition, got %v", result)
	}
}

func TestSelectMappings_NoModes(t *testing.T) {
	engine := &PluggableWidgetEngine{operations: NewOperationRegistry()}

	def := &WidgetDefinition{
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "attr", Source: "Attribute", Operation: "attribute"},
		},
		ChildSlots: []ChildSlotMapping{
			{PropertyKey: "content", MDLContainer: "TEMPLATE", Operation: "widgets"},
		},
	}
	w := &ast.WidgetV3{Properties: map[string]any{}}

	mappings, slots, err := engine.selectMappings(def, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mappings) != 1 || mappings[0].PropertyKey != "attr" {
		t.Errorf("expected 1 mapping with key 'attr', got %v", mappings)
	}
	if len(slots) != 1 || slots[0].PropertyKey != "content" {
		t.Errorf("expected 1 slot with key 'content', got %v", slots)
	}
}

func TestSelectMappings_WithModes(t *testing.T) {
	engine := &PluggableWidgetEngine{operations: NewOperationRegistry()}

	def := &WidgetDefinition{
		Modes: []WidgetMode{
			{
				Name:             "association",
				Condition:        "hasDataSource",
				PropertyMappings: []PropertyMapping{{PropertyKey: "assoc", Operation: "association"}},
			},
			{
				Name:             "default",
				PropertyMappings: []PropertyMapping{{PropertyKey: "enum", Operation: "attribute"}},
			},
		},
	}

	t.Run("matches association mode", func(t *testing.T) {
		w := &ast.WidgetV3{
			Properties: map[string]any{
				"DataSource": &ast.DataSourceV3{Type: "database"},
			},
		}
		mappings, _, err := engine.selectMappings(def, w)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mappings) != 1 || mappings[0].PropertyKey != "assoc" {
			t.Errorf("expected association mode, got %v", mappings)
		}
	})

	t.Run("falls back to default mode", func(t *testing.T) {
		w := &ast.WidgetV3{Properties: map[string]any{}}
		mappings, _, err := engine.selectMappings(def, w)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mappings) != 1 || mappings[0].PropertyKey != "enum" {
			t.Errorf("expected default mode, got %v", mappings)
		}
	})
}

func TestResolveMapping_StaticValue(t *testing.T) {
	engine := &PluggableWidgetEngine{operations: NewOperationRegistry()}

	mapping := PropertyMapping{
		PropertyKey: "optionsSourceType",
		Value:       "association",
		Operation:   "primitive",
	}
	w := &ast.WidgetV3{Properties: map[string]any{}}

	ctx, err := engine.resolveMapping(mapping, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.PrimitiveVal != "association" {
		t.Errorf("expected PrimitiveVal='association', got %q", ctx.PrimitiveVal)
	}
}

func TestResolveMapping_AttributeSource(t *testing.T) {
	pb := &pageBuilder{
		entityContext:    "Module.Entity",
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
	}
	engine := &PluggableWidgetEngine{
		operations:  NewOperationRegistry(),
		pageBuilder: pb,
	}

	mapping := PropertyMapping{
		PropertyKey: "attributeEnumeration",
		Source:      "Attribute",
		Operation:   "attribute",
	}
	w := &ast.WidgetV3{Properties: map[string]any{"Attribute": "Name"}}

	ctx, err := engine.resolveMapping(mapping, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.AttributePath != "Module.Entity.Name" {
		t.Errorf("expected AttributePath='Module.Entity.Name', got %q", ctx.AttributePath)
	}
}

func TestResolveMapping_SelectionWithDefault(t *testing.T) {
	engine := &PluggableWidgetEngine{operations: NewOperationRegistry()}

	mapping := PropertyMapping{
		PropertyKey: "itemSelection",
		Source:      "Selection",
		Operation:   "primitive",
		Default:     "Single",
	}

	t.Run("uses AST value when present", func(t *testing.T) {
		w := &ast.WidgetV3{Properties: map[string]any{"Selection": "Multiple"}}
		ctx, err := engine.resolveMapping(mapping, w)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ctx.PrimitiveVal != "Multiple" {
			t.Errorf("expected PrimitiveVal='Multiple', got %q", ctx.PrimitiveVal)
		}
	})

	t.Run("uses default when AST value empty", func(t *testing.T) {
		w := &ast.WidgetV3{Properties: map[string]any{}}
		ctx, err := engine.resolveMapping(mapping, w)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ctx.PrimitiveVal != "Single" {
			t.Errorf("expected PrimitiveVal='Single', got %q", ctx.PrimitiveVal)
		}
	})
}

func TestResolveMapping_GenericProp(t *testing.T) {
	engine := &PluggableWidgetEngine{operations: NewOperationRegistry()}

	mapping := PropertyMapping{
		PropertyKey: "customProp",
		Source:      "MyCustomProp",
		Operation:   "primitive",
	}
	w := &ast.WidgetV3{Properties: map[string]any{"MyCustomProp": "customValue"}}

	ctx, err := engine.resolveMapping(mapping, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.PrimitiveVal != "customValue" {
		t.Errorf("expected PrimitiveVal='customValue', got %q", ctx.PrimitiveVal)
	}
}

func TestResolveMapping_EmptySource(t *testing.T) {
	engine := &PluggableWidgetEngine{operations: NewOperationRegistry()}

	mapping := PropertyMapping{
		PropertyKey: "someProp",
		Operation:   "primitive",
	}
	w := &ast.WidgetV3{Properties: map[string]any{}}

	ctx, err := engine.resolveMapping(mapping, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.PrimitiveVal != "" || ctx.AttributePath != "" {
		t.Errorf("expected empty context, got %+v", ctx)
	}
}

func TestResolveMapping_CaptionAttribute(t *testing.T) {
	pb := &pageBuilder{
		entityContext:    "Module.Customer",
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
	}
	engine := &PluggableWidgetEngine{
		operations:  NewOperationRegistry(),
		pageBuilder: pb,
	}

	mapping := PropertyMapping{
		PropertyKey: "captionAttr",
		Source:      "CaptionAttribute",
		Operation:   "attribute",
	}
	w := &ast.WidgetV3{Properties: map[string]any{"CaptionAttribute": "FullName"}}

	ctx, err := engine.resolveMapping(mapping, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.AttributePath != "Module.Customer.FullName" {
		t.Errorf("expected 'Module.Customer.FullName', got %q", ctx.AttributePath)
	}
}

func TestResolveMapping_Association(t *testing.T) {
	pb := &pageBuilder{
		entityContext:    "Module.Order",
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
	}
	engine := &PluggableWidgetEngine{
		operations:  NewOperationRegistry(),
		pageBuilder: pb,
	}

	mapping := PropertyMapping{
		PropertyKey: "attributeAssociation",
		Source:      "Association",
		Operation:   "association",
	}
	w := &ast.WidgetV3{Properties: map[string]any{"Attribute": "Order_Customer"}}

	ctx, err := engine.resolveMapping(mapping, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.AssocPath != "Module.Order_Customer" {
		t.Errorf("expected AssocPath='Module.Order_Customer', got %q", ctx.AssocPath)
	}
	if ctx.EntityName != "Module.Order" {
		t.Errorf("expected EntityName='Module.Order', got %q", ctx.EntityName)
	}
}

func TestSetChildWidgets(t *testing.T) {
	val := bson.D{
		{Key: "PrimitiveValue", Value: ""},
		{Key: "Widgets", Value: bson.A{int32(2)}},
		{Key: "XPathConstraint", Value: ""},
	}

	childWidgets := []bson.D{
		{{Key: "$Type", Value: "Forms$TextBox"}, {Key: "Name", Value: "textBox1"}},
		{{Key: "$Type", Value: "Forms$TextBox"}, {Key: "Name", Value: "textBox2"}},
	}

	updated := setChildWidgets(val, childWidgets)

	// Find Widgets field
	for _, elem := range updated {
		if elem.Key == "Widgets" {
			arr, ok := elem.Value.(bson.A)
			if !ok {
				t.Fatal("Widgets value is not bson.A")
			}
			// Should have version marker + 2 widgets
			if len(arr) != 3 {
				t.Errorf("Widgets array length: got %d, want 3", len(arr))
			}
			// First element should be version marker
			if marker, ok := arr[0].(int32); !ok || marker != 2 {
				t.Errorf("Widgets[0]: got %v, want int32(2)", arr[0])
			}
			return
		}
	}
	t.Error("Widgets field not found in result")
}

func TestOpSelection(t *testing.T) {
	// Call the real opSelection function with a properly structured widget BSON.
	typePointerBytes := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	typePointerUUID := types.BlobToUUID(typePointerBytes)

	widgetObj := bson.D{
		{Key: "Properties", Value: bson.A{
			int32(2), // version marker
			bson.D{
				{Key: "TypePointer", Value: typePointerBytes},
				{Key: "Value", Value: bson.D{
					{Key: "PrimitiveValue", Value: ""},
					{Key: "Selection", Value: "None"},
				}},
			},
		}},
	}

	propTypeIDs := map[string]pages.PropertyTypeIDEntry{
		"selectionType": {PropertyTypeID: typePointerUUID},
	}

	ctx := &BuildContext{PrimitiveVal: "Multi"}
	result := opSelection(widgetObj, propTypeIDs, "selectionType", ctx)

	// Extract the updated Value from Properties
	var props bson.A
	for _, elem := range result {
		if elem.Key == "Properties" {
			props = elem.Value.(bson.A)
		}
	}
	prop := props[1].(bson.D) // skip version marker at index 0
	var val bson.D
	for _, elem := range prop {
		if elem.Key == "Value" {
			val = elem.Value.(bson.D)
		}
	}

	selectionFound := false
	for _, elem := range val {
		if elem.Key == "Selection" {
			selectionFound = true
			if elem.Value != "Multi" {
				t.Errorf("Selection: got %q, want %q", elem.Value, "Multi")
			}
		}
		if elem.Key == "PrimitiveValue" {
			if elem.Value != "" {
				t.Errorf("PrimitiveValue should remain empty, got %q", elem.Value)
			}
		}
	}
	if !selectionFound {
		t.Error("Selection field not found in result")
	}
}

func TestOpSelectionEmptyValue(t *testing.T) {
	widgetObj := bson.D{
		{Key: "Properties", Value: bson.A{int32(2)}},
	}
	ctx := &BuildContext{PrimitiveVal: ""}
	result := opSelection(widgetObj, nil, "any", ctx)

	// With empty PrimitiveVal, opSelection returns obj unchanged
	if len(result) != len(widgetObj) {
		t.Errorf("expected unchanged obj, got different length: %d vs %d", len(result), len(widgetObj))
	}
}
