// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/bsonnav"
	"github.com/JordtenBulte-OLC/mxcli/model"
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
	if bsonnav.DGetString(result.widget, "Name") != "txtName" {
		t.Errorf("Expected name 'txtName', got %q", bsonnav.DGetString(result.widget, "Name"))
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
	if bsonnav.DGetString(result.widget, "Name") != "txtInner" {
		t.Errorf("Expected name 'txtInner', got %q", bsonnav.DGetString(result.widget, "Name"))
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

func TestDropWidget_Single(t *testing.T) {
	w1 := makeWidget("txtName", "Pages$TextBox")
	w2 := makeWidget("txtEmail", "Pages$TextBox")
	w3 := makeWidget("txtPhone", "Pages$TextBox")
	rawData := makeRawPage(w1, w2, w3)

	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	refs := []backend.WidgetRef{{Widget: "txtEmail"}}
	if err := m.DropWidget(refs); err != nil {
		t.Fatalf("DropWidget failed: %v", err)
	}

	// Verify txtEmail was removed
	formCall := bsonnav.DGetDoc(rawData, "FormCall")
	args := bsonnav.DGetArrayElements(bsonnav.DGet(formCall, "Arguments"))
	argDoc := args[0].(bson.D)
	widgets := bsonnav.DGetArrayElements(bsonnav.DGet(argDoc, "Widgets"))

	if len(widgets) != 2 {
		t.Fatalf("Expected 2 widgets after drop, got %d", len(widgets))
	}

	name0 := bsonnav.DGetString(widgets[0].(bson.D), "Name")
	name1 := bsonnav.DGetString(widgets[1].(bson.D), "Name")
	if name0 != "txtName" {
		t.Errorf("Expected first widget 'txtName', got %q", name0)
	}
	if name1 != "txtPhone" {
		t.Errorf("Expected second widget 'txtPhone', got %q", name1)
	}
}

func TestDropWidget_Multiple(t *testing.T) {
	w1 := makeWidget("a", "Pages$TextBox")
	w2 := makeWidget("b", "Pages$TextBox")
	w3 := makeWidget("c", "Pages$TextBox")
	rawData := makeRawPage(w1, w2, w3)

	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	refs := []backend.WidgetRef{{Widget: "a"}, {Widget: "c"}}
	if err := m.DropWidget(refs); err != nil {
		t.Fatalf("DropWidget failed: %v", err)
	}

	formCall := bsonnav.DGetDoc(rawData, "FormCall")
	args := bsonnav.DGetArrayElements(bsonnav.DGet(formCall, "Arguments"))
	argDoc := args[0].(bson.D)
	widgets := bsonnav.DGetArrayElements(bsonnav.DGet(argDoc, "Widgets"))

	if len(widgets) != 1 {
		t.Fatalf("Expected 1 widget after dropping a and c, got %d", len(widgets))
	}

	name := bsonnav.DGetString(widgets[0].(bson.D), "Name")
	if name != "b" {
		t.Errorf("Expected remaining widget 'b', got %q", name)
	}
}

func TestDropWidget_NotFound(t *testing.T) {
	w1 := makeWidget("txtName", "Pages$TextBox")
	rawData := makeRawPage(w1)

	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	refs := []backend.WidgetRef{{Widget: "nonexistent"}}
	err := m.DropWidget(refs)
	if err == nil {
		t.Fatal("Expected error for nonexistent widget")
	}
}

func TestDropWidget_Nested(t *testing.T) {
	inner1 := makeWidget("txtInner1", "Pages$TextBox")
	inner2 := makeWidget("txtInner2", "Pages$TextBox")
	container := makeContainerWidget("ctn1", inner1, inner2)
	rawData := makeRawPage(container)

	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	refs := []backend.WidgetRef{{Widget: "txtInner1"}}
	if err := m.DropWidget(refs); err != nil {
		t.Fatalf("DropWidget failed: %v", err)
	}

	// Verify txtInner1 was removed
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

func TestSetWidgetProperty_Name(t *testing.T) {
	w1 := makeWidget("txtOld", "Pages$TextBox")
	rawData := makeRawPage(w1)

	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("txtOld", "Name", "txtNew"); err != nil {
		t.Fatalf("SetWidgetProperty failed: %v", err)
	}

	// Verify name was changed
	result := findBsonWidget(rawData, "txtNew")
	if result == nil {
		t.Fatal("Expected to find renamed widget 'txtNew'")
	}
}

// TestSetWidgetProperty_DynamicClasses locks in the DynamicClasses ALTER fix: `alter page ... set
// DynamicClasses` writes the expression into the widget's Forms$Appearance
// (previously it fell through to the pluggable-property setter and hard-errored
// "widget has no pluggable Object" on core widgets).
func TestSetWidgetProperty_DynamicClasses(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	expr := "if $currentObject/Name = 'x' then 'c--x' else ''"
	if err := m.SetWidgetProperty("ctn1", "DynamicClasses", expr); err != nil {
		t.Fatalf("SetWidgetProperty(DynamicClasses) failed: %v", err)
	}

	result := findBsonWidget(rawData, "ctn1")
	if result == nil {
		t.Fatal("widget 'ctn1' not found")
	}
	app := bsonnav.DGetDoc(result.widget, "Appearance")
	if app == nil {
		t.Fatal("widget 'ctn1' has no Appearance")
	}
	if got := bsonnav.DGetString(app, "DynamicClasses"); got != expr {
		t.Errorf("Appearance.DynamicClasses = %q, want %q", got, expr)
	}
}

// makeVisibilityWidget builds a widget with a null ConditionalVisibilitySettings
// slot (as Studio Pro writes it when unset), matching the widgets that support
// conditional visibility.
func makeVisibilityWidget(name string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Pages$DivContainer"},
		{Key: "Name", Value: name},
		{Key: "ConditionalVisibilitySettings", Value: nil},
	}
}

// TestSetWidgetProperty_VisibleExpression locks in the fix for the silently
// dropped string/boolean `Visible` form: `alter page ... set Visible = "<expr>"`
// (and `= false`) writes a ConditionalVisibilitySettings node rather than a bare
// (ignored) "Visible" string; `= true` clears it.
func TestSetWidgetProperty_VisibleExpression(t *testing.T) {
	t.Run("expression string", func(t *testing.T) {
		rawData := makeRawPage(makeVisibilityWidget("ctn1"))
		m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
		expr := "$currentObject/Name = 'Astute'"
		if err := m.SetWidgetProperty("ctn1", "Visible", expr); err != nil {
			t.Fatalf("SetWidgetProperty(Visible) failed: %v", err)
		}
		cvs := bsonnav.DGetDoc(findBsonWidget(rawData, "ctn1").widget, "ConditionalVisibilitySettings")
		if cvs == nil {
			t.Fatal("expected ConditionalVisibilitySettings node")
		}
		if got := bsonnav.DGetString(cvs, "Expression"); got != expr {
			t.Errorf("Expression = %q, want %q", got, expr)
		}
		// The bare (wrong) Visible field must not be written.
		if v := bsonnav.DGet(findBsonWidget(rawData, "ctn1").widget, "Visible"); v != nil {
			t.Errorf("unexpected bare Visible field: %v", v)
		}
	})

	t.Run("static false hides", func(t *testing.T) {
		rawData := makeRawPage(makeVisibilityWidget("ctn1"))
		m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
		if err := m.SetWidgetProperty("ctn1", "Visible", false); err != nil {
			t.Fatalf("SetWidgetProperty(Visible=false) failed: %v", err)
		}
		cvs := bsonnav.DGetDoc(findBsonWidget(rawData, "ctn1").widget, "ConditionalVisibilitySettings")
		if cvs == nil || bsonnav.DGetString(cvs, "Expression") != "false" {
			t.Errorf("expected ConditionalVisibilitySettings Expression=false, got %v", cvs)
		}
	})

	t.Run("static true clears", func(t *testing.T) {
		w := makeVisibilityWidget("ctn1")
		// Pre-populate a settings node, then set Visible=true to clear it.
		bsonnav.DSet(w, "ConditionalVisibilitySettings", bson.D{{Key: "Expression", Value: "false"}})
		rawData := makeRawPage(w)
		m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
		if err := m.SetWidgetProperty("ctn1", "Visible", true); err != nil {
			t.Fatalf("SetWidgetProperty(Visible=true) failed: %v", err)
		}
		if cvs := bsonnav.DGet(findBsonWidget(rawData, "ctn1").widget, "ConditionalVisibilitySettings"); cvs != nil {
			t.Errorf("expected ConditionalVisibilitySettings cleared to nil, got %v", cvs)
		}
	})
}

func TestSetWidgetProperty_ButtonStyle(t *testing.T) {
	w1 := bson.D{
		{Key: "$Type", Value: "Pages$ActionButton"},
		{Key: "Name", Value: "btnSave"},
		{Key: "ButtonStyle", Value: "Default"},
	}
	rawData := makeRawPage(w1)

	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("btnSave", "ButtonStyle", "Success"); err != nil {
		t.Fatalf("SetWidgetProperty failed: %v", err)
	}

	result := findBsonWidget(rawData, "btnSave")
	if result == nil {
		t.Fatal("Expected to find btnSave")
	}
	if bsonnav.DGetString(result.widget, "ButtonStyle") != "Success" {
		t.Errorf("Expected ButtonStyle='Success', got %v", bsonnav.DGet(result.widget, "ButtonStyle"))
	}
}

func TestSetWidgetProperty_WidgetNotFound(t *testing.T) {
	w1 := makeWidget("txtName", "Pages$TextBox")
	rawData := makeRawPage(w1)

	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	err := m.SetWidgetProperty("nonexistent", "Name", "new")
	if err == nil {
		t.Fatal("Expected error for nonexistent widget")
	}
}

func TestSetWidgetProperty_PluggableWidget(t *testing.T) {
	propTypeID := primitive.Binary{Subtype: 0x04, Data: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}}
	w1 := bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "cb1"},
		{Key: "Type", Value: bson.D{
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{
					int32(2),
					bson.D{
						{Key: "$ID", Value: propTypeID},
						{Key: "PropertyKey", Value: "showLabel"},
					},
				}},
			}},
		}},
		{Key: "Object", Value: bson.D{
			{Key: "Properties", Value: bson.A{
				int32(2),
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

	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("cb1", "showLabel", false); err != nil {
		t.Fatalf("SetWidgetProperty failed: %v", err)
	}

	result := findBsonWidget(rawData, "cb1")
	if result == nil {
		t.Fatal("Expected to find cb1")
	}
	obj := bsonnav.DGetDoc(result.widget, "Object")
	props := bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties"))
	propDoc := props[0].(bson.D)
	valDoc := bsonnav.DGetDoc(propDoc, "Value")
	if bsonnav.DGetString(valDoc, "PrimitiveValue") != "no" {
		t.Errorf("Expected PrimitiveValue='no', got %v", bsonnav.DGet(valDoc, "PrimitiveValue"))
	}
}

func TestDSetArray_PreservesMarker(t *testing.T) {
	parent := bson.D{
		{Key: "Widgets", Value: bson.A{int32(2), "a", "b"}},
	}
	bsonnav.DSetArray(parent, "Widgets", []any{"x", "y"})

	result := bsonnav.ToBsonA(bsonnav.DGet(parent, "Widgets"))
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
	bsonnav.DSetArray(parent, "Widgets", []any{"x"})

	result := bsonnav.ToBsonA(bsonnav.DGet(parent, "Widgets"))
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

func makeRawSnippet(widgets ...bson.D) bson.D {
	widgetArr := bson.A{int32(2)}
	for _, w := range widgets {
		widgetArr = append(widgetArr, w)
	}
	return bson.D{
		{Key: "Widgets", Value: widgetArr},
	}
}

func makeRawSnippetMxcli(widgets ...bson.D) bson.D {
	widgetArr := bson.A{int32(2)}
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
	if bsonnav.DGetString(result.widget, "Name") != "txtName" {
		t.Errorf("Expected 'txtName', got %q", bsonnav.DGetString(result.widget, "Name"))
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

func TestDropWidget_Snippet(t *testing.T) {
	w1 := makeWidget("txtName", "Pages$TextBox")
	w2 := makeWidget("txtEmail", "Pages$TextBox")
	rawData := makeRawSnippet(w1, w2)

	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidgetInSnippet}
	refs := []backend.WidgetRef{{Widget: "txtEmail"}}
	if err := m.DropWidget(refs); err != nil {
		t.Fatalf("DropWidget failed: %v", err)
	}

	// Verify txtEmail was removed
	widgets := bsonnav.DGetArrayElements(bsonnav.DGet(rawData, "Widgets"))
	if len(widgets) != 1 {
		t.Fatalf("Expected 1 widget after drop, got %d", len(widgets))
	}
	name := bsonnav.DGetString(widgets[0].(bson.D), "Name")
	if name != "txtName" {
		t.Errorf("Expected remaining widget 'txtName', got %q", name)
	}
}

func TestSetWidgetProperty_Snippet(t *testing.T) {
	w1 := bson.D{
		{Key: "$Type", Value: "Pages$ActionButton"},
		{Key: "Name", Value: "btnAction"},
		{Key: "ButtonStyle", Value: "Default"},
	}
	rawData := makeRawSnippet(w1)

	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidgetInSnippet}
	if err := m.SetWidgetProperty("btnAction", "ButtonStyle", "Danger"); err != nil {
		t.Fatalf("SetWidgetProperty failed: %v", err)
	}

	result := findBsonWidgetInSnippet(rawData, "btnAction")
	if result == nil {
		t.Fatal("Expected to find btnAction")
	}
	if bsonnav.DGetString(result.widget, "ButtonStyle") != "Danger" {
		t.Errorf("Expected ButtonStyle='Danger', got %v", bsonnav.DGet(result.widget, "ButtonStyle"))
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
// Page context tree tests
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
			int32(2),
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

func TestFindWidget(t *testing.T) {
	w1 := makeWidget("txtName", "Pages$TextBox")
	rawData := makeRawPage(w1)

	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if !m.FindWidget("txtName") {
		t.Error("Expected FindWidget to return true for existing widget")
	}
	if m.FindWidget("nonexistent") {
		t.Error("Expected FindWidget to return false for nonexistent widget")
	}
}

func TestParamScope(t *testing.T) {
	rawData := bson.D{
		{Key: "Parameters", Value: bson.A{
			int32(2),
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
		}},
	}

	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	ids, names := m.ParamScope()

	if len(ids) != 1 {
		t.Fatalf("Expected 1 param, got %d", len(ids))
	}
	if names["Customer"] != "MyModule.Customer" {
		t.Errorf("Expected MyModule.Customer, got %q", names["Customer"])
	}
	// Verify ID is a valid model.ID (non-empty)
	if ids["Customer"] == model.ID("") {
		t.Error("Expected non-empty ID")
	}
}

// ---------------------------------------------------------------------------
// SetLayout tests
// ---------------------------------------------------------------------------

// makePageWithLayout builds a minimal page BSON doc with a FormCall pointing
// to the given layout and argument parameters.
func makePageWithLayout(layoutQN string, params ...string) bson.D {
	args := bson.A{int32(3)}
	for _, p := range params {
		args = append(args, bson.D{
			{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
			{Key: "$Type", Value: "Pages$FormCallArgument"},
			{Key: "Parameter", Value: layoutQN + "." + p},
		})
	}
	return bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Pages$FormCall"},
		{Key: "FormCall", Value: bson.D{
			{Key: "Form", Value: layoutQN},
			{Key: "Arguments", Value: args},
		}},
	}
}

func makePageMutator(rawData bson.D) *Mutator {
	return &Mutator{rawData: rawData, containerType: "page", widgetFinder: findBsonWidget}
}

func TestSetLayout_Basic(t *testing.T) {
	page := makePageWithLayout("MyModule.OldLayout", "Content", "Header")
	m := makePageMutator(page)

	if err := m.SetLayout("MyModule.NewLayout", nil); err != nil {
		t.Fatalf("SetLayout failed: %v", err)
	}

	formCall := bsonnav.DGetDoc(m.rawData, "FormCall")
	if got := bsonnav.DGetString(formCall, "Form"); got != "MyModule.NewLayout" {
		t.Errorf("Form = %q, want MyModule.NewLayout", got)
	}

	// Verify parameters were remapped
	args := bsonnav.DGetArrayElements(bsonnav.DGet(formCall, "Arguments"))
	for _, a := range args {
		aDoc := a.(bson.D)
		param := bsonnav.DGetString(aDoc, "Parameter")
		if !strings.HasPrefix(param, "MyModule.NewLayout.") {
			t.Errorf("Parameter %q should start with MyModule.NewLayout.", param)
		}
	}
}

func TestSetLayout_WithParamMappings(t *testing.T) {
	page := makePageWithLayout("MyModule.OldLayout", "Content", "Header")
	m := makePageMutator(page)

	mappings := map[string]string{
		"Content": "MainArea",
		"Header":  "TopBar",
	}
	if err := m.SetLayout("MyModule.NewLayout", mappings); err != nil {
		t.Fatalf("SetLayout with mappings failed: %v", err)
	}

	formCall := bsonnav.DGetDoc(m.rawData, "FormCall")
	args := bsonnav.DGetArrayElements(bsonnav.DGet(formCall, "Arguments"))
	paramValues := make(map[string]bool)
	for _, a := range args {
		aDoc := a.(bson.D)
		paramValues[bsonnav.DGetString(aDoc, "Parameter")] = true
	}
	if !paramValues["MyModule.NewLayout.MainArea"] {
		t.Error("Expected MyModule.NewLayout.MainArea in remapped params")
	}
	if !paramValues["MyModule.NewLayout.TopBar"] {
		t.Error("Expected MyModule.NewLayout.TopBar in remapped params")
	}
}

func TestSetLayout_SameLayout_Noop(t *testing.T) {
	page := makePageWithLayout("MyModule.SameLayout", "Content")
	m := makePageMutator(page)

	if err := m.SetLayout("MyModule.SameLayout", nil); err != nil {
		t.Fatalf("SetLayout same layout failed: %v", err)
	}

	// Should be a no-op — form unchanged
	formCall := bsonnav.DGetDoc(m.rawData, "FormCall")
	if got := bsonnav.DGetString(formCall, "Form"); got != "MyModule.SameLayout" {
		t.Errorf("Form = %q, want MyModule.SameLayout", got)
	}
}

func TestSetLayout_Snippet_Error(t *testing.T) {
	page := makePageWithLayout("MyModule.Layout", "Content")
	m := &Mutator{rawData: page, containerType: "snippet", widgetFinder: findBsonWidget}

	err := m.SetLayout("MyModule.NewLayout", nil)
	if err == nil {
		t.Fatal("Expected error for snippet")
	}
	if !strings.Contains(err.Error(), "snippet") {
		t.Errorf("Error = %q, want to mention snippet", err.Error())
	}
}

func TestSetLayout_NoFormCall_Error(t *testing.T) {
	page := bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Pages$Page"},
	}
	m := makePageMutator(page)

	err := m.SetLayout("MyModule.NewLayout", nil)
	if err == nil {
		t.Fatal("Expected error for missing FormCall")
	}
}

func TestSetLayout_EmptyForm_Error(t *testing.T) {
	page := bson.D{
		{Key: "FormCall", Value: bson.D{
			{Key: "Form", Value: ""},
			{Key: "Arguments", Value: bson.A{int32(3)}},
		}},
	}
	m := makePageMutator(page)

	err := m.SetLayout("MyModule.NewLayout", nil)
	if err == nil {
		t.Fatal("Expected error when current layout cannot be determined")
	}
}

// ============================================================================
// deriveColumnNameBson / sanitizeColumnName regression tests (issue #116)
// ============================================================================

func makePropTypeID116(b byte) primitive.Binary {
	data := make([]byte, 16)
	data[0] = b
	return primitive.Binary{Subtype: 0x04, Data: data}
}

func TestDeriveColumnNameBson_AttributeBinding(t *testing.T) {
	typeID := makePropTypeID116(0x01)
	propKeyMap := map[string]string{bsonnav.ExtractBinaryIDFromDoc(typeID): "attribute"}

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

func TestDeriveColumnNameBson_CaptionFallback(t *testing.T) {
	typeID := makePropTypeID116(0x02)
	propKeyMap := map[string]string{bsonnav.ExtractBinaryIDFromDoc(typeID): "header"}

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

func TestDeriveColumnNameBson_AllSpecialCharCaptionFallsBackToColN(t *testing.T) {
	typeID := makePropTypeID116(0x03)
	propKeyMap := map[string]string{bsonnav.ExtractBinaryIDFromDoc(typeID): "header"}

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

func TestDeriveColumnNameBson_IndexFallback(t *testing.T) {
	colDoc := bson.D{{Key: "Properties", Value: bson.A{int32(2)}}}
	got := deriveColumnNameBson(colDoc, map[string]string{}, 2)
	if got != "col3" {
		t.Errorf("expected 'col3', got %q", got)
	}
}

func TestSanitizeColumnName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Description", "Description"},
		{" Description ", "Description"},
		{"Order Status", "Order_Status"},
		{"!Order Status!", "Order_Status"},
		{"Hello World", "Hello_World"},
		{"___", ""},
		{"---", ""},
	}
	for _, c := range cases {
		got := sanitizeColumnName(c.input)
		if got != c.want {
			t.Errorf("sanitizeColumnName(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// --- Page-level pop-up dimension SET (issue #661) -------------------------

func TestSetPageLevel_PopupWidth(t *testing.T) {
	rawData := makeRawPage()
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	// Integer literals arrive from the visitor as Go int.
	if err := m.SetWidgetProperty("", "PopupWidth", 800); err != nil {
		t.Fatalf("SetWidgetProperty PopupWidth failed: %v", err)
	}
	got := bsonnav.DGet(m.rawData, "PopupWidth")
	if got != int64(800) {
		t.Errorf("PopupWidth = %v (%T), want int64(800)", got, got)
	}
}

func TestSetPageLevel_PopupHeight(t *testing.T) {
	rawData := makeRawPage()
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	if err := m.SetWidgetProperty("", "PopupHeight", 480); err != nil {
		t.Fatalf("SetWidgetProperty PopupHeight failed: %v", err)
	}
	if got := bsonnav.DGet(m.rawData, "PopupHeight"); got != int64(480) {
		t.Errorf("PopupHeight = %v (%T), want int64(480)", got, got)
	}
}

// Issue #714 — page-level SET Class/Style writes the page's Forms$Appearance.
func TestSetPageLevel_ClassAndStyle(t *testing.T) {
	// No Appearance present yet → one is created.
	m := &Mutator{rawData: makeRawPage(), widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("", "Class", "my-page"); err != nil {
		t.Fatalf("SET Class: %v", err)
	}
	if err := m.SetWidgetProperty("", "Style", "padding: 10px"); err != nil {
		t.Fatalf("SET Style: %v", err)
	}
	ap := bsonnav.DGetDoc(m.rawData, "Appearance")
	if ap == nil {
		t.Fatal("Appearance not created")
	}
	if got := bsonnav.DGet(ap, "Class"); got != "my-page" {
		t.Errorf("Appearance.Class = %v, want my-page", got)
	}
	if got := bsonnav.DGet(ap, "Style"); got != "padding: 10px" {
		t.Errorf("Appearance.Style = %v, want 'padding: 10px'", got)
	}

	// Existing Appearance with a Class → updated in place.
	raw := makeRawPage()
	raw = append(raw, bson.E{Key: "Appearance", Value: bson.D{
		{Key: "$Type", Value: "Forms$Appearance"}, {Key: "Class", Value: "old"}, {Key: "Style", Value: ""},
	}})
	m2 := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
	if err := m2.SetWidgetProperty("", "Class", "new"); err != nil {
		t.Fatalf("SET Class (update): %v", err)
	}
	if got := bsonnav.DGet(bsonnav.DGetDoc(m2.rawData, "Appearance"), "Class"); got != "new" {
		t.Errorf("updated Appearance.Class = %v, want new", got)
	}

	// A non-string value is rejected.
	m3 := &Mutator{rawData: makeRawPage(), widgetFinder: findBsonWidget}
	if err := m3.SetWidgetProperty("", "Class", 42); err == nil {
		t.Error("non-string Class should be rejected")
	}
}

func TestSetPageLevel_PopupWidth_Float(t *testing.T) {
	rawData := makeRawPage()
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	// A value written with a decimal point arrives as float64; whole values coerce.
	if err := m.SetWidgetProperty("", "PopupWidth", float64(640)); err != nil {
		t.Fatalf("SetWidgetProperty PopupWidth float failed: %v", err)
	}
	if got := bsonnav.DGet(m.rawData, "PopupWidth"); got != int64(640) {
		t.Errorf("PopupWidth = %v (%T), want int64(640)", got, got)
	}
}

func TestSetPageLevel_PopupResizable(t *testing.T) {
	rawData := makeRawPage()
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	if err := m.SetWidgetProperty("", "PopupResizable", true); err != nil {
		t.Fatalf("SetWidgetProperty PopupResizable failed: %v", err)
	}
	if got := bsonnav.DGet(m.rawData, "PopupResizable"); got != true {
		t.Errorf("PopupResizable = %v, want true", got)
	}
}

// SET PopupWidth = 0 is valid — 0 is Studio Pro's default (auto-size) and must be
// stored verbatim, not rejected (issue #713).
func TestSetPageLevel_PopupWidth_Zero(t *testing.T) {
	rawData := makeRawPage()
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("", "PopupWidth", 0); err != nil {
		t.Fatalf("SET PopupWidth = 0 should be accepted, got: %v", err)
	}
	if got := bsonnav.DGet(m.rawData, "PopupWidth"); got != int64(0) {
		t.Errorf("PopupWidth = %v (%T), want int64(0)", got, got)
	}
}

func TestSetPageLevel_Popup_Invalid(t *testing.T) {
	cases := []struct {
		name  string
		prop  string
		value any
	}{
		{"negative width", "PopupWidth", -1},
		{"negative height", "PopupHeight", -10},
		{"non-whole float", "PopupWidth", 12.5},
		{"non-number width", "PopupWidth", "wide"},
		{"non-bool resizable", "PopupResizable", "yes"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rawData := makeRawPage()
			m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
			if err := m.SetWidgetProperty("", c.prop, c.value); err == nil {
				t.Errorf("expected error for %s = %v, got nil", c.prop, c.value)
			}
		})
	}
}

func TestSetPageLevel_UnsupportedProperty(t *testing.T) {
	rawData := makeRawPage()
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("", "NotARealProperty", 1); err == nil {
		t.Fatal("expected error for unsupported page-level property, got nil")
	}
}

// Issue #627 — SET VisibleIf builds a ConditionalVisibilitySettings node carrying
// the expression, replacing the null slot. Editable on a widget without the slot
// is rejected.
func TestSetWidgetProperty_VisibleIf(t *testing.T) {
	w := bson.D{
		{Key: "$Type", Value: "Forms$DivContainer"},
		{Key: "Name", Value: "ctn"},
		{Key: "ConditionalVisibilitySettings", Value: nil},
	}
	rawData := makeRawPage(w)
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	if err := m.SetWidgetProperty("ctn", "VisibleIf", "$currentObject/Name != ''"); err != nil {
		t.Fatalf("SetWidgetProperty VisibleIf: %v", err)
	}
	res := findBsonWidget(rawData, "ctn")
	cvs, ok := bsonnav.DGet(res.widget, "ConditionalVisibilitySettings").(bson.D)
	if !ok {
		t.Fatalf("ConditionalVisibilitySettings not a doc: %T", bsonnav.DGet(res.widget, "ConditionalVisibilitySettings"))
	}
	if got := bsonnav.DGetString(cvs, "Expression"); got != "$currentObject/Name != ''" {
		t.Errorf("Expression = %q", got)
	}
	if bsonnav.DGetString(cvs, "$Type") != "Forms$ConditionalVisibilitySettings" {
		t.Errorf("wrong $Type: %v", bsonnav.DGet(cvs, "$Type"))
	}
}

func TestSetWidgetProperty_EditableIf_Unsupported(t *testing.T) {
	// A container has no ConditionalEditabilitySettings slot → reject.
	w := bson.D{
		{Key: "$Type", Value: "Forms$DivContainer"},
		{Key: "Name", Value: "ctn"},
		{Key: "ConditionalVisibilitySettings", Value: nil},
	}
	rawData := makeRawPage(w)
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("ctn", "EditableIf", "$currentObject/Active"); err == nil {
		t.Fatal("expected error setting EditableIf on a container, got nil")
	}
}
