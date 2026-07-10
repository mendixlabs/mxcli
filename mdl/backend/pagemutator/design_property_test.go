// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/bsonnav"
)

// makeStyleableWidget builds a widget with a Forms$Appearance sub-document and
// an empty (marker-only) DesignProperties array, matching serializeAppearance.
func makeStyleableWidget(name string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Pages$DivContainer"},
		{Key: "Name", Value: name},
		{Key: "Appearance", Value: bson.D{
			{Key: "$Type", Value: "Forms$Appearance"},
			{Key: "Class", Value: ""},
			{Key: "DesignProperties", Value: bson.A{int32(3)}},
			{Key: "DynamicClasses", Value: ""},
			{Key: "Style", Value: ""},
		}},
	}
}

// designPropEntries returns the DesignPropertyValue entries (marker stripped) for a widget.
func designPropEntries(t *testing.T, rawData bson.D, widgetName string) []any {
	t.Helper()
	result := findBsonWidget(rawData, widgetName)
	if result == nil {
		t.Fatalf("widget %q not found", widgetName)
	}
	app := bsonnav.DGetDoc(result.widget, "Appearance")
	if app == nil {
		t.Fatalf("widget %q has no Appearance", widgetName)
	}
	return bsonnav.DGetArrayElements(bsonnav.DGet(app, "DesignProperties"))
}

// findEntry returns the DesignPropertyValue entry with the given Key, or nil.
func findEntry(entries []any, key string) bson.D {
	for _, el := range entries {
		if entry, ok := el.(bson.D); ok && bsonnav.DGetString(entry, "Key") == key {
			return entry
		}
	}
	return nil
}

func TestSetDesignProperty_ToggleOn_Appends(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	if err := m.SetDesignProperty("ctn1", "Full width", "toggle", ""); err != nil {
		t.Fatalf("SetDesignProperty failed: %v", err)
	}

	entries := designPropEntries(t, rawData, "ctn1")
	if len(entries) != 1 {
		t.Fatalf("expected 1 design property, got %d", len(entries))
	}
	entry := findEntry(entries, "Full width")
	if entry == nil {
		t.Fatal("expected entry for 'Full width'")
	}
	if bsonnav.DGetString(entry, "$Type") != designPropertyEntryType {
		t.Errorf("expected entry $Type=%q, got %q", designPropertyEntryType, bsonnav.DGetString(entry, "$Type"))
	}
	val := bsonnav.DGetDoc(entry, "Value")
	if val == nil || bsonnav.DGetString(val, "$Type") != toggleDesignPropertyType {
		t.Errorf("expected toggle value, got %#v", bsonnav.DGet(entry, "Value"))
	}
}

func TestSetDesignProperty_Option_Appends(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	if err := m.SetDesignProperty("ctn1", "Spacing bottom", "option", "Large"); err != nil {
		t.Fatalf("SetDesignProperty failed: %v", err)
	}

	entry := findEntry(designPropEntries(t, rawData, "ctn1"), "Spacing bottom")
	if entry == nil {
		t.Fatal("expected entry for 'Spacing bottom'")
	}
	val := bsonnav.DGetDoc(entry, "Value")
	if val == nil || bsonnav.DGetString(val, "$Type") != optionDesignPropertyType {
		t.Fatalf("expected option value, got %#v", bsonnav.DGet(entry, "Value"))
	}
	if bsonnav.DGetString(val, "Option") != "Large" {
		t.Errorf("expected Option='Large', got %q", bsonnav.DGetString(val, "Option"))
	}
}

func TestSetDesignProperty_Custom_Appends(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	// ToggleButtonGroup/ColorPicker values use the "custom" value-type.
	if err := m.SetDesignProperty("ctn1", "Flex container", "custom", "Horizontal (row)"); err != nil {
		t.Fatalf("SetDesignProperty failed: %v", err)
	}

	val := bsonnav.DGetDoc(findEntry(designPropEntries(t, rawData, "ctn1"), "Flex container"), "Value")
	if val == nil || bsonnav.DGetString(val, "$Type") != customDesignPropertyType {
		t.Fatalf("expected custom value type, got %#v", bsonnav.DGet(findEntry(designPropEntries(t, rawData, "ctn1"), "Flex container"), "Value"))
	}
	if bsonnav.DGetString(val, "Value") != "Horizontal (row)" {
		t.Errorf("expected custom Value='Horizontal (row)', got %q", bsonnav.DGetString(val, "Value"))
	}
}

func TestSetDesignProperty_UpdatesExistingKeyInPlace(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	// First set as toggle, then re-set the same key as an option.
	if err := m.SetDesignProperty("ctn1", "Mode", "toggle", ""); err != nil {
		t.Fatalf("first set failed: %v", err)
	}
	if err := m.SetDesignProperty("ctn1", "Mode", "option", "Compact"); err != nil {
		t.Fatalf("second set failed: %v", err)
	}

	entries := designPropEntries(t, rawData, "ctn1")
	if len(entries) != 1 {
		t.Fatalf("expected key updated in place (1 entry), got %d", len(entries))
	}
	val := bsonnav.DGetDoc(findEntry(entries, "Mode"), "Value")
	if bsonnav.DGetString(val, "$Type") != optionDesignPropertyType || bsonnav.DGetString(val, "Option") != "Compact" {
		t.Errorf("expected option 'Compact', got %#v", val)
	}
}

// An option-typed set must overwrite a stale Custom value with an
// OptionDesignPropertyValue. ToggleButtonGroup values are Option, not Custom;
// writing Custom triggers Studio Pro CE6084, so re-applying must repair it.
func TestSetDesignProperty_OptionOverwritesCustom(t *testing.T) {
	w := makeStyleableWidget("ctn1")
	app := bsonnav.DGetDoc(w, "Appearance")
	bsonnav.DSetArray(app, "DesignProperties", []any{
		bson.D{
			{Key: "$Type", Value: designPropertyEntryType},
			{Key: "Key", Value: "Flex container"},
			{Key: "Value", Value: bson.D{
				{Key: "$Type", Value: customDesignPropertyType},
				{Key: "Value", Value: "Horizontal (row)"},
			}},
		},
	})
	rawData := makeRawPage(w)
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	if err := m.SetDesignProperty("ctn1", "Flex container", "option", "Horizontal (row)"); err != nil {
		t.Fatalf("SetDesignProperty failed: %v", err)
	}

	val := bsonnav.DGetDoc(findEntry(designPropEntries(t, rawData, "ctn1"), "Flex container"), "Value")
	if bsonnav.DGetString(val, "$Type") != optionDesignPropertyType {
		t.Errorf("expected Custom overwritten with Option, got %q", bsonnav.DGetString(val, "$Type"))
	}
	if bsonnav.DGetString(val, "Option") != "Horizontal (row)" {
		t.Errorf("expected Option='Horizontal (row)', got %q", bsonnav.DGetString(val, "Option"))
	}
}

func TestRemoveDesignProperty(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	_ = m.SetDesignProperty("ctn1", "A", "toggle", "")
	_ = m.SetDesignProperty("ctn1", "B", "toggle", "")

	if err := m.RemoveDesignProperty("ctn1", "A"); err != nil {
		t.Fatalf("RemoveDesignProperty failed: %v", err)
	}

	entries := designPropEntries(t, rawData, "ctn1")
	if len(entries) != 1 || findEntry(entries, "A") != nil || findEntry(entries, "B") == nil {
		t.Fatalf("expected only 'B' to remain, got %d entries", len(entries))
	}
}

func TestClearDesignProperties(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	_ = m.SetDesignProperty("ctn1", "A", "toggle", "")
	_ = m.SetDesignProperty("ctn1", "B", "option", "X")

	if err := m.ClearDesignProperties("ctn1"); err != nil {
		t.Fatalf("ClearDesignProperties failed: %v", err)
	}

	if entries := designPropEntries(t, rawData, "ctn1"); len(entries) != 0 {
		t.Fatalf("expected all design properties cleared, got %d", len(entries))
	}
	// Marker must be preserved so the array still serializes correctly.
	result := findBsonWidget(rawData, "ctn1")
	arr := bsonnav.ToBsonA(bsonnav.DGet(bsonnav.DGetDoc(result.widget, "Appearance"), "DesignProperties"))
	if len(arr) != 1 {
		t.Fatalf("expected marker-only array, got %d elements", len(arr))
	}
	if _, ok := arr[0].(int32); !ok {
		t.Errorf("expected int32 marker preserved, got %T", arr[0])
	}
}

func TestSetDesignProperty_WidgetNotFound(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetDesignProperty("nope", "A", "toggle", ""); err == nil {
		t.Fatal("expected error for nonexistent widget")
	}
}

func TestSetDesignProperty_NoAppearance(t *testing.T) {
	w := bson.D{
		{Key: "$Type", Value: "Pages$DivContainer"},
		{Key: "Name", Value: "ctn1"},
	}
	rawData := makeRawPage(w)
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetDesignProperty("ctn1", "A", "toggle", ""); err == nil {
		t.Fatal("expected error when widget has no Appearance")
	}
}
