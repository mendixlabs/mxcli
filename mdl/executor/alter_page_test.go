// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// Helper to build a minimal raw BSON page structure for testing.
func makeRawPage(widgets ...bson.D) bson.D {
	widgetArr := bson.A{int32(2)} // type marker
	for _, w := range widgets {
		widgetArr = append(widgetArr, w)
	}
	return bson.D{
		{Key: "FormCall", Value: bson.D{
			{Key: "Arguments", Value: bson.A{
				int32(2), // type marker
				bson.D{
					{Key: "Widgets", Value: widgetArr},
				},
			}},
		}},
	}
}

func makeWidget(name string, typeName string) bson.D {
	return bson.D{
		{Key: "$Type", Value: typeName},
		{Key: "Name", Value: name},
	}
}

func makeContainerWidget(name string, children ...bson.D) bson.D {
	childArr := bson.A{int32(2)} // type marker
	for _, c := range children {
		childArr = append(childArr, c)
	}
	return bson.D{
		{Key: "$Type", Value: "Pages$DivContainer"},
		{Key: "Name", Value: name},
		{Key: "Widgets", Value: childArr},
	}
}

func TestFindBsonWidget_TopLevel(t *testing.T) {
	w1 := makeWidget("txtName", "Pages$TextBox")
	w2 := makeWidget("txtEmail", "Pages$TextBox")
	rawData := makeRawPage(w1, w2)

	result := findBsonWidget(rawData, "txtName")
	if result == nil {
		t.Fatal("Expected to find txtName")
	}
	if dGetString(result.widget, "Name") != "txtName" {
		t.Errorf("Expected name 'txtName', got %q", dGetString(result.widget, "Name"))
	}
	if result.index != 0 {
		t.Errorf("Expected index 0, got %d", result.index)
	}
}

func TestFindBsonWidget_Nested(t *testing.T) {
	inner := makeWidget("txtInner", "Pages$TextBox")
	container := makeContainerWidget("ctn1", inner)
	rawData := makeRawPage(container)

	result := findBsonWidget(rawData, "txtInner")
	if result == nil {
		t.Fatal("Expected to find txtInner inside container")
	}
	if dGetString(result.widget, "Name") != "txtInner" {
		t.Errorf("Expected name 'txtInner', got %q", dGetString(result.widget, "Name"))
	}
}

func TestFindBsonWidget_NotFound(t *testing.T) {
	w1 := makeWidget("txtName", "Pages$TextBox")
	rawData := makeRawPage(w1)

	result := findBsonWidget(rawData, "nonexistent")
	if result != nil {
		t.Error("Expected nil for nonexistent widget")
	}
}

func TestApplyDropWidget_Single(t *testing.T) {
	w1 := makeWidget("txtName", "Pages$TextBox")
	w2 := makeWidget("txtEmail", "Pages$TextBox")
	w3 := makeWidget("txtPhone", "Pages$TextBox")
	rawData := makeRawPage(w1, w2, w3)

	op := &ast.DropWidgetOp{Targets: []ast.WidgetRef{{Widget: "txtEmail"}}}
	if err := applyDropWidget(rawData, op); err != nil {
		t.Fatalf("applyDropWidget failed: %v", err)
	}

	// Verify txtEmail was removed
	formCall := dGetDoc(rawData, "FormCall")
	args := dGetArrayElements(dGet(formCall, "Arguments"))
	argDoc := args[0].(bson.D)
	widgets := dGetArrayElements(dGet(argDoc, "Widgets"))

	if len(widgets) != 2 {
		t.Fatalf("Expected 2 widgets after drop, got %d", len(widgets))
	}

	name0 := dGetString(widgets[0].(bson.D), "Name")
	name1 := dGetString(widgets[1].(bson.D), "Name")
	if name0 != "txtName" {
		t.Errorf("Expected first widget 'txtName', got %q", name0)
	}
	if name1 != "txtPhone" {
		t.Errorf("Expected second widget 'txtPhone', got %q", name1)
	}
}

func TestApplyDropWidget_Multiple(t *testing.T) {
	w1 := makeWidget("a", "Pages$TextBox")
	w2 := makeWidget("b", "Pages$TextBox")
	w3 := makeWidget("c", "Pages$TextBox")
	rawData := makeRawPage(w1, w2, w3)

	op := &ast.DropWidgetOp{Targets: []ast.WidgetRef{{Widget: "a"}, {Widget: "c"}}}
	if err := applyDropWidget(rawData, op); err != nil {
		t.Fatalf("applyDropWidget failed: %v", err)
	}

	formCall := dGetDoc(rawData, "FormCall")
	args := dGetArrayElements(dGet(formCall, "Arguments"))
	argDoc := args[0].(bson.D)
	widgets := dGetArrayElements(dGet(argDoc, "Widgets"))

	if len(widgets) != 1 {
		t.Fatalf("Expected 1 widget after dropping a and c, got %d", len(widgets))
	}

	name := dGetString(widgets[0].(bson.D), "Name")
	if name != "b" {
		t.Errorf("Expected remaining widget 'b', got %q", name)
	}
}

func TestApplyDropWidget_NotFound(t *testing.T) {
	w1 := makeWidget("txtName", "Pages$TextBox")
	rawData := makeRawPage(w1)

	op := &ast.DropWidgetOp{Targets: []ast.WidgetRef{{Widget: "nonexistent"}}}
	err := applyDropWidget(rawData, op)
	if err == nil {
		t.Fatal("Expected error for nonexistent widget")
	}
}

func TestApplyDropWidget_Nested(t *testing.T) {
	inner1 := makeWidget("txtInner1", "Pages$TextBox")
	inner2 := makeWidget("txtInner2", "Pages$TextBox")
	container := makeContainerWidget("ctn1", inner1, inner2)
	rawData := makeRawPage(container)

	op := &ast.DropWidgetOp{Targets: []ast.WidgetRef{{Widget: "txtInner1"}}}
	if err := applyDropWidget(rawData, op); err != nil {
		t.Fatalf("applyDropWidget failed: %v", err)
	}

	// Verify txtInner1 was removed from container
	result := findBsonWidget(rawData, "txtInner1")
	if result != nil {
		t.Error("txtInner1 should have been removed")
	}

	// txtInner2 should still exist
	result = findBsonWidget(rawData, "txtInner2")
	if result == nil {
		t.Error("txtInner2 should still exist")
	}
}

func TestApplySetProperty_Name(t *testing.T) {
	w1 := makeWidget("txtOld", "Pages$TextBox")
	rawData := makeRawPage(w1)

	op := &ast.SetPropertyOp{
		Target: ast.WidgetRef{Widget: "txtOld"},
		Properties: map[string]interface{}{
			"Name": "txtNew",
		},
	}
	if err := applySetProperty(rawData, op); err != nil {
		t.Fatalf("applySetProperty failed: %v", err)
	}

	// Verify name was changed
	result := findBsonWidget(rawData, "txtNew")
	if result == nil {
		t.Fatal("Expected to find renamed widget 'txtNew'")
	}
}

func TestApplySetProperty_ButtonStyle(t *testing.T) {
	w1 := bson.D{
		{Key: "$Type", Value: "Pages$ActionButton"},
		{Key: "Name", Value: "btnSave"},
		{Key: "ButtonStyle", Value: "Default"},
	}
	rawData := makeRawPage(w1)

	op := &ast.SetPropertyOp{
		Target: ast.WidgetRef{Widget: "btnSave"},
		Properties: map[string]interface{}{
			"ButtonStyle": "Success",
		},
	}
	if err := applySetProperty(rawData, op); err != nil {
		t.Fatalf("applySetProperty failed: %v", err)
	}

	result := findBsonWidget(rawData, "btnSave")
	if result == nil {
		t.Fatal("Expected to find btnSave")
	}
	if dGetString(result.widget, "ButtonStyle") != "Success" {
		t.Errorf("Expected ButtonStyle='Success', got %v", dGet(result.widget, "ButtonStyle"))
	}
}

func TestApplySetProperty_WidgetNotFound(t *testing.T) {
	w1 := makeWidget("txtName", "Pages$TextBox")
	rawData := makeRawPage(w1)

	op := &ast.SetPropertyOp{
		Target: ast.WidgetRef{Widget: "nonexistent"},
		Properties: map[string]interface{}{
			"Name": "new",
		},
	}
	err := applySetProperty(rawData, op)
	if err == nil {
		t.Fatal("Expected error for nonexistent widget")
	}
}

func TestApplySetProperty_PluggableWidget(t *testing.T) {
	// Pluggable widget properties are identified by TypePointer referencing
	// a PropertyType entry in Type.ObjectType.PropertyTypes, NOT by a "Key" field.
	propTypeID := primitive.Binary{Subtype: 0x04, Data: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}}
	w1 := bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "cb1"},
		{Key: "Type", Value: bson.D{
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{
					int32(2), // type marker
					bson.D{
						{Key: "$ID", Value: propTypeID},
						{Key: "PropertyKey", Value: "showLabel"},
					},
				}},
			}},
		}},
		{Key: "Object", Value: bson.D{
			{Key: "Properties", Value: bson.A{
				int32(2), // type marker
				bson.D{
					{Key: "TypePointer", Value: propTypeID},
					{Key: "Value", Value: bson.D{
						{Key: "PrimitiveValue", Value: "yes"},
					}},
				},
			}},
		}},
	}
	rawData := makeRawPage(w1)

	op := &ast.SetPropertyOp{
		Target: ast.WidgetRef{Widget: "cb1"},
		Properties: map[string]interface{}{
			"showLabel": false,
		},
	}
	if err := applySetProperty(rawData, op); err != nil {
		t.Fatalf("applySetProperty failed: %v", err)
	}

	result := findBsonWidget(rawData, "cb1")
	if result == nil {
		t.Fatal("Expected to find cb1")
	}
	obj := dGetDoc(result.widget, "Object")
	props := dGetArrayElements(dGet(obj, "Properties"))
	propDoc := props[0].(bson.D)
	valDoc := dGetDoc(propDoc, "Value")
	if dGetString(valDoc, "PrimitiveValue") != "no" {
		t.Errorf("Expected PrimitiveValue='no', got %v", dGet(valDoc, "PrimitiveValue"))
	}
}

func TestDSetArray_PreservesMarker(t *testing.T) {
	parent := bson.D{
		{Key: "Widgets", Value: bson.A{int32(2), "a", "b"}},
	}
	dSetArray(parent, "Widgets", []any{"x", "y"})

	result := toBsonA(dGet(parent, "Widgets"))
	if len(result) != 3 {
		t.Fatalf("Expected 3 elements (marker + 2), got %d", len(result))
	}
	if result[0] != int32(2) {
		t.Errorf("Expected marker int32(2), got %v", result[0])
	}
	if result[1] != "x" || result[2] != "y" {
		t.Errorf("Expected [x, y], got %v", result[1:])
	}
}

func TestDSetArray_NoMarker(t *testing.T) {
	parent := bson.D{
		{Key: "Widgets", Value: bson.A{"a", "b"}},
	}
	dSetArray(parent, "Widgets", []any{"x"})

	result := toBsonA(dGet(parent, "Widgets"))
	if len(result) != 1 {
		t.Fatalf("Expected 1 element, got %d", len(result))
	}
	if result[0] != "x" {
		t.Errorf("Expected [x], got %v", result)
	}
}

func TestFindBsonWidget_LayoutGrid(t *testing.T) {
	inner := makeWidget("txtInGrid", "Pages$TextBox")
	rawData := bson.D{
		{Key: "FormCall", Value: bson.D{
			{Key: "Arguments", Value: bson.A{
				int32(2),
				bson.D{
					{Key: "Widgets", Value: bson.A{
						int32(2),
						bson.D{
							{Key: "$Type", Value: "Pages$LayoutGrid"},
							{Key: "Name", Value: "lg1"},
							{Key: "Rows", Value: bson.A{
								int32(2),
								bson.D{
									{Key: "Columns", Value: bson.A{
										int32(2),
										bson.D{
											{Key: "Widgets", Value: bson.A{int32(2), inner}},
										},
									}},
								},
							}},
						},
					}},
				},
			}},
		}},
	}

	result := findBsonWidget(rawData, "txtInGrid")
	if result == nil {
		t.Fatal("Expected to find txtInGrid inside LayoutGrid")
	}
}

// ============================================================================
// Snippet BSON tests
// ============================================================================

// Helper to build a minimal raw BSON snippet structure (Studio Pro format).
func makeRawSnippet(widgets ...bson.D) bson.D {
	widgetArr := bson.A{int32(2)} // type marker
	for _, w := range widgets {
		widgetArr = append(widgetArr, w)
	}
	return bson.D{
		{Key: "Widgets", Value: widgetArr},
	}
}

// Helper to build a minimal raw BSON snippet structure (mxcli format).
func makeRawSnippetMxcli(widgets ...bson.D) bson.D {
	widgetArr := bson.A{int32(2)} // type marker
	for _, w := range widgets {
		widgetArr = append(widgetArr, w)
	}
	return bson.D{
		{Key: "Widget", Value: bson.D{
			{Key: "Widgets", Value: widgetArr},
		}},
	}
}

func TestFindBsonWidgetInSnippet_TopLevel(t *testing.T) {
	w1 := makeWidget("txtName", "Pages$TextBox")
	w2 := makeWidget("txtEmail", "Pages$TextBox")
	rawData := makeRawSnippet(w1, w2)

	result := findBsonWidgetInSnippet(rawData, "txtName")
	if result == nil {
		t.Fatal("Expected to find txtName in snippet")
	}
	if dGetString(result.widget, "Name") != "txtName" {
		t.Errorf("Expected 'txtName', got %q", dGetString(result.widget, "Name"))
	}
}

func TestFindBsonWidgetInSnippet_MxcliFormat(t *testing.T) {
	w1 := makeWidget("txtName", "Pages$TextBox")
	rawData := makeRawSnippetMxcli(w1)

	result := findBsonWidgetInSnippet(rawData, "txtName")
	if result == nil {
		t.Fatal("Expected to find txtName in mxcli-format snippet")
	}
}

func TestFindBsonWidgetInSnippet_Nested(t *testing.T) {
	inner := makeWidget("txtInner", "Pages$TextBox")
	container := makeContainerWidget("ctn1", inner)
	rawData := makeRawSnippet(container)

	result := findBsonWidgetInSnippet(rawData, "txtInner")
	if result == nil {
		t.Fatal("Expected to find txtInner nested in snippet")
	}
}

func TestFindBsonWidgetInSnippet_NotFound(t *testing.T) {
	w1 := makeWidget("txtName", "Pages$TextBox")
	rawData := makeRawSnippet(w1)

	result := findBsonWidgetInSnippet(rawData, "nonexistent")
	if result != nil {
		t.Error("Expected nil for nonexistent widget in snippet")
	}
}

func TestApplyDropWidget_Snippet(t *testing.T) {
	w1 := makeWidget("txtName", "Pages$TextBox")
	w2 := makeWidget("txtEmail", "Pages$TextBox")
	rawData := makeRawSnippet(w1, w2)

	op := &ast.DropWidgetOp{Targets: []ast.WidgetRef{{Widget: "txtEmail"}}}
	if err := applyDropWidgetWith(rawData, op, findBsonWidgetInSnippet); err != nil {
		t.Fatalf("applyDropWidgetWith failed: %v", err)
	}

	// Verify txtEmail was removed
	widgets := dGetArrayElements(dGet(rawData, "Widgets"))
	if len(widgets) != 1 {
		t.Fatalf("Expected 1 widget after drop, got %d", len(widgets))
	}
	name := dGetString(widgets[0].(bson.D), "Name")
	if name != "txtName" {
		t.Errorf("Expected remaining widget 'txtName', got %q", name)
	}
}

func TestApplySetProperty_Snippet(t *testing.T) {
	w1 := bson.D{
		{Key: "$Type", Value: "Pages$ActionButton"},
		{Key: "Name", Value: "btnAction"},
		{Key: "ButtonStyle", Value: "Default"},
	}
	rawData := makeRawSnippet(w1)

	op := &ast.SetPropertyOp{
		Target: ast.WidgetRef{Widget: "btnAction"},
		Properties: map[string]interface{}{
			"ButtonStyle": "Danger",
		},
	}
	if err := applySetPropertyWith(rawData, op, findBsonWidgetInSnippet); err != nil {
		t.Fatalf("applySetPropertyWith failed: %v", err)
	}

	result := findBsonWidgetInSnippet(rawData, "btnAction")
	if result == nil {
		t.Fatal("Expected to find btnAction")
	}
	if dGetString(result.widget, "ButtonStyle") != "Danger" {
		t.Errorf("Expected ButtonStyle='Danger', got %v", dGet(result.widget, "ButtonStyle"))
	}
}

func TestFindBsonWidget_DataViewFooter(t *testing.T) {
	footer := makeWidget("btnFooter", "Pages$ActionButton")
	rawData := bson.D{
		{Key: "FormCall", Value: bson.D{
			{Key: "Arguments", Value: bson.A{
				int32(2),
				bson.D{
					{Key: "Widgets", Value: bson.A{
						int32(2),
						bson.D{
							{Key: "$Type", Value: "Pages$DataView"},
							{Key: "Name", Value: "dv1"},
							{Key: "Widgets", Value: bson.A{int32(2)}},
							{Key: "FooterWidgets", Value: bson.A{int32(2), footer}},
						},
					}},
				},
			}},
		}},
	}

	result := findBsonWidget(rawData, "btnFooter")
	if result == nil {
		t.Fatal("Expected to find btnFooter in DataView FooterWidgets")
	}
}

// ============================================================================
// Page context tree tests (#157)
// ============================================================================

func makeWidgetWithID(name string, typeName string, id primitive.Binary) bson.D {
	return bson.D{
		{Key: "$ID", Value: id},
		{Key: "$Type", Value: typeName},
		{Key: "Name", Value: name},
	}
}

func makeBsonID(b byte) primitive.Binary {
	data := make([]byte, 16)
	data[0] = b
	return primitive.Binary{Subtype: 0x04, Data: data}
}

func TestExtractPageParamsFromBSON_EntityParams(t *testing.T) {
	rawData := bson.D{
		{Key: "Parameters", Value: bson.A{
			int32(2), // type marker
			bson.D{
				{Key: "$ID", Value: makeBsonID(0x01)},
				{Key: "$Type", Value: "Forms$PageParameter"},
				{Key: "Name", Value: "Customer"},
				{Key: "ParameterType", Value: bson.D{
					{Key: "$ID", Value: makeBsonID(0x02)},
					{Key: "$Type", Value: "DataTypes$ObjectType"},
					{Key: "Entity", Value: "MyModule.Customer"},
				}},
			},
			bson.D{
				{Key: "$ID", Value: makeBsonID(0x03)},
				{Key: "$Type", Value: "Forms$PageParameter"},
				{Key: "Name", Value: "Order"},
				{Key: "ParameterType", Value: bson.D{
					{Key: "$ID", Value: makeBsonID(0x04)},
					{Key: "$Type", Value: "DataTypes$ObjectType"},
					{Key: "Entity", Value: "MyModule.Order"},
				}},
			},
		}},
	}

	paramScope, paramEntityNames := extractPageParamsFromBSON(rawData)

	if len(paramScope) != 2 {
		t.Fatalf("Expected 2 params, got %d", len(paramScope))
	}
	if paramEntityNames["Customer"] != "MyModule.Customer" {
		t.Errorf("Expected Customer -> MyModule.Customer, got %q", paramEntityNames["Customer"])
	}
	if paramEntityNames["Order"] != "MyModule.Order" {
		t.Errorf("Expected Order -> MyModule.Order, got %q", paramEntityNames["Order"])
	}
	if paramScope["Customer"] == "" {
		t.Error("Expected non-empty ID for Customer param")
	}
}

func TestExtractPageParamsFromBSON_SkipsPrimitiveParams(t *testing.T) {
	rawData := bson.D{
		{Key: "Parameters", Value: bson.A{
			int32(2),
			bson.D{
				{Key: "$ID", Value: makeBsonID(0x01)},
				{Key: "$Type", Value: "Forms$PageParameter"},
				{Key: "Name", Value: "Title"},
				{Key: "ParameterType", Value: bson.D{
					{Key: "$ID", Value: makeBsonID(0x02)},
					{Key: "$Type", Value: "DataTypes$StringType"},
				}},
			},
		}},
	}

	paramScope, paramEntityNames := extractPageParamsFromBSON(rawData)

	if len(paramScope) != 0 {
		t.Errorf("Expected 0 entity params (String is primitive), got %d", len(paramScope))
	}
	if len(paramEntityNames) != 0 {
		t.Errorf("Expected 0 entity param names, got %d", len(paramEntityNames))
	}
}

func TestExtractPageParamsFromBSON_Nil(t *testing.T) {
	paramScope, paramEntityNames := extractPageParamsFromBSON(nil)
	if len(paramScope) != 0 || len(paramEntityNames) != 0 {
		t.Error("Expected empty maps for nil input")
	}
}

func TestExtractWidgetScopeFromBSON_PageFormat(t *testing.T) {
	id1 := makeBsonID(0x10)
	id2 := makeBsonID(0x20)
	rawData := bson.D{
		{Key: "FormCall", Value: bson.D{
			{Key: "Arguments", Value: bson.A{
				int32(2),
				bson.D{
					{Key: "Widgets", Value: bson.A{
						int32(2),
						makeWidgetWithID("dgOrders", "CustomWidgets$CustomWidget", id1),
						makeWidgetWithID("txtName", "Pages$TextBox", id2),
					}},
				},
			}},
		}},
	}

	scope := extractWidgetScopeFromBSON(rawData)

	if len(scope) != 2 {
		t.Fatalf("Expected 2 widgets in scope, got %d", len(scope))
	}
	if scope["dgOrders"] == "" {
		t.Error("Expected dgOrders in widget scope")
	}
	if scope["txtName"] == "" {
		t.Error("Expected txtName in widget scope")
	}
}

func TestExtractWidgetScopeFromBSON_NestedWidgets(t *testing.T) {
	idDv := makeBsonID(0x10)
	idInner := makeBsonID(0x20)
	rawData := bson.D{
		{Key: "FormCall", Value: bson.D{
			{Key: "Arguments", Value: bson.A{
				int32(2),
				bson.D{
					{Key: "Widgets", Value: bson.A{
						int32(2),
						bson.D{
							{Key: "$ID", Value: idDv},
							{Key: "$Type", Value: "Pages$DataView"},
							{Key: "Name", Value: "dvOrder"},
							{Key: "Widgets", Value: bson.A{
								int32(2),
								makeWidgetWithID("txtOrderNum", "Pages$TextBox", idInner),
							}},
						},
					}},
				},
			}},
		}},
	}

	scope := extractWidgetScopeFromBSON(rawData)

	if scope["dvOrder"] == "" {
		t.Error("Expected dvOrder in widget scope")
	}
	if scope["txtOrderNum"] == "" {
		t.Error("Expected txtOrderNum in widget scope (nested in DataView)")
	}
}

func TestExtractWidgetScopeFromBSON_SnippetFormat(t *testing.T) {
	idW := makeBsonID(0x10)
	rawData := bson.D{
		{Key: "Widgets", Value: bson.A{
			int32(2),
			makeWidgetWithID("txtSnippet", "Pages$TextBox", idW),
		}},
	}

	scope := extractWidgetScopeFromBSON(rawData)

	if scope["txtSnippet"] == "" {
		t.Error("Expected txtSnippet in widget scope (snippet format)")
	}
}

func TestExtractWidgetScopeFromBSON_Nil(t *testing.T) {
	scope := extractWidgetScopeFromBSON(nil)
	if len(scope) != 0 {
		t.Error("Expected empty scope for nil input")
	}
}

// ============================================================================
// deriveColumnNameBson regression tests (issue #116)
// ============================================================================

// makePropTypeID builds a primitive.Binary suitable for use as a TypePointer.
func makePropTypeID(b byte) primitive.Binary {
	data := make([]byte, 16)
	data[0] = b
	return primitive.Binary{Subtype: 0x04, Data: data}
}

// propKey builds the map key used by deriveColumnNameBson — the UUID string
// that extractBinaryIDFromDoc produces from a primitive.Binary TypePointer.
func propKey(id primitive.Binary) string {
	return extractBinaryIDFromDoc(id)
}

// TestDeriveColumnNameBson_AttributeBinding verifies attribute-bound columns
// produce the short attribute name (last segment after dot).
func TestDeriveColumnNameBson_AttributeBinding(t *testing.T) {
	typeID := makePropTypeID(0x01)
	propKeyMap := map[string]string{propKey(typeID): "attribute"}

	colDoc := bson.D{
		{Key: "Properties", Value: bson.A{
			int32(2),
			bson.D{
				{Key: "TypePointer", Value: typeID},
				{Key: "Value", Value: bson.D{
					{Key: "AttributeRef", Value: "MyModule.Customer.Description"},
				}},
			},
		}},
	}

	got := deriveColumnNameBson(colDoc, propKeyMap, 0)
	if got != "Description" {
		t.Errorf("expected 'Description', got %q", got)
	}
}

// TestDeriveColumnNameBson_CaptionFallback verifies caption-only columns
// produce the sanitized caption. This exercises the TextTemplate → Template →
// Items[] path that was broken in issue #116.
func TestDeriveColumnNameBson_CaptionFallback(t *testing.T) {
	typeID := makePropTypeID(0x02)
	propKeyMap := map[string]string{propKey(typeID): "header"}

	colDoc := bson.D{
		{Key: "Properties", Value: bson.A{
			int32(2),
			bson.D{
				{Key: "TypePointer", Value: typeID},
				{Key: "Value", Value: bson.D{
					{Key: "TextTemplate", Value: bson.D{
						{Key: "Template", Value: bson.D{
							{Key: "Items", Value: bson.A{
								int32(2),
								bson.D{{Key: "Text", Value: "Order Status"}},
							}},
						}},
					}},
				}},
			},
		}},
	}

	got := deriveColumnNameBson(colDoc, propKeyMap, 0)
	if got != "Order_Status" {
		t.Errorf("expected 'Order_Status', got %q", got)
	}
}

// TestDeriveColumnNameBson_IndexFallback verifies that a column with neither
// attribute nor caption falls back to "col{N}" (1-based).
func TestDeriveColumnNameBson_IndexFallback(t *testing.T) {
	colDoc := bson.D{{Key: "Properties", Value: bson.A{int32(2)}}}
	got := deriveColumnNameBson(colDoc, map[string]string{}, 2)
	if got != "col3" {
		t.Errorf("expected 'col3', got %q", got)
	}
}

// TestSanitizeColumnName_TrimUnderscores verifies that leading/trailing
// underscores are trimmed to match deriveColumnName() on the DESCRIBE side.
func TestSanitizeColumnName_TrimUnderscores(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{" Description ", "Description"},
		{"!Order Status!", "Order_Status"},
		{"Hello World", "Hello_World"},
		{"___", ""}, // all special chars → empty → caller falls through to col{N}
	}
	for _, c := range cases {
		got := sanitizeColumnName(c.input)
		if got != c.want {
			t.Errorf("sanitizeColumnName(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// TestDeriveColumnNameBson_AllSpecialCharCaption verifies that a caption
// composed entirely of non-identifier characters falls back to col{N},
// matching deriveColumnName() on the DESCRIBE side.
func TestDeriveColumnNameBson_AllSpecialCharCaption(t *testing.T) {
	typeID := makePropTypeID(0x03)
	propKeyMap := map[string]string{propKey(typeID): "header"}

	colDoc := bson.D{
		{Key: "Properties", Value: bson.A{
			int32(2),
			bson.D{
				{Key: "TypePointer", Value: typeID},
				{Key: "Value", Value: bson.D{
					{Key: "TextTemplate", Value: bson.D{
						{Key: "Template", Value: bson.D{
							{Key: "Items", Value: bson.A{
								int32(2),
								bson.D{{Key: "Text", Value: "---"}},
							}},
						}},
					}},
				}},
			},
		}},
	}

	got := deriveColumnNameBson(colDoc, propKeyMap, 0)
	if got != "col1" {
		t.Errorf("expected 'col1' for all-special-char caption, got %q", got)
	}
}
