// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// ako/mxcli#2180 (mendixlabs/mxcli#1833): PlatformDesignSystem's Button and
// Form label declare property keys of their own named `caption`, `label`,
// `buttonStyle` and `icon`. ALTER resolved those names as core properties
// first, so the widget's own key was never reached:
//
//	set (caption: 'Save') on btn      -> widget has no Caption property
//	set (label: 'Name') on fl         -> a core LabelTemplate beside the own `label`
//	set (buttonStyle: 'outline') ...  -> "Altered page", nothing written

type pdsProp struct {
	key, kind string
	id        byte
}

// pdsShapedWidget is a CustomWidget whose definition declares props, each with
// a stored value carrying a TextTemplate (for TextTemplate kinds) and a
// PrimitiveValue. It also stores `LabelTemplate: null`, as every CustomWidget
// does, and no Caption, CaptionTemplate or ButtonStyle key.
func pdsShapedWidget(props ...pdsProp) bson.D {
	ptypes := bson.A{int32(2)}
	values := bson.A{int32(2)}
	for _, p := range props {
		id := bson.Binary{Subtype: 0x04, Data: []byte{p.id, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}}
		ptypes = append(ptypes, bson.D{
			{Key: "$ID", Value: id},
			{Key: "PropertyKey", Value: p.key},
			{Key: "ValueType", Value: bson.D{{Key: "Type", Value: p.kind}}},
		})
		var tmpl any
		if p.kind == "TextTemplate" {
			tmpl = bson.D{
				{Key: "$Type", Value: "Forms$ClientTemplate"},
				{Key: "Parameters", Value: bson.A{int32(2)}},
				{Key: "Template", Value: bson.D{
					{Key: "$Type", Value: "Texts$Text"},
					{Key: "Items", Value: bson.A{int32(3), bson.D{
						{Key: "$Type", Value: "Texts$Translation"},
						{Key: "LanguageCode", Value: "en_US"},
						{Key: "Text", Value: "old"},
					}}},
				}},
			}
		}
		values = append(values, bson.D{
			{Key: "TypePointer", Value: id},
			{Key: "Value", Value: bson.D{
				{Key: "Expression", Value: ""},
				{Key: "PrimitiveValue", Value: "old"},
				{Key: "TextTemplate", Value: tmpl},
			}},
		})
	}
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "btn"},
		{Key: "LabelTemplate", Value: nil},
		{Key: "Type", Value: bson.D{
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "ObjectType", Value: bson.D{{Key: "PropertyTypes", Value: ptypes}}},
		}},
		{Key: "Object", Value: bson.D{{Key: "Properties", Value: values}}},
	}
}

// ownValue returns the stored Value of the widget's own property key.
func ownValue(t *testing.T, w bson.D, key string) bson.D {
	t.Helper()
	keys := buildPropKeyMap(w)
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(w, "Object"), "Properties")) {
		d := p.(bson.D)
		if keys[bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(d, "TypePointer"))] == key {
			return bsonnav.DGetDoc(d, "Value")
		}
	}
	t.Fatalf("no stored value for %q", key)
	return nil
}

func pdsButton() bson.D {
	return pdsShapedWidget(
		pdsProp{"caption", "TextTemplate", 0x31},
		pdsProp{"label", "TextTemplate", 0x32},
		pdsProp{"buttonStyle", "Enumeration", 0x33},
		pdsProp{"icon", "String", 0x34},
		pdsProp{"tooltip", "TextTemplate", 0x35}, // as a Slider declares one
	)
}

func TestSetPluggableOwnCoreName_WritesTheWidgetsOwnKey(t *testing.T) {
	cases := []struct {
		name, prop, key string
		value           any
		read            func(t *testing.T, v bson.D) string
	}{
		{"caption", "caption", "caption", "Save", templateText},
		{"Caption as describe prints it", "Caption", "caption", "Save", templateText},
		{"label", "label", "label", "Name", templateText},
		{"buttonStyle", "buttonStyle", "buttonStyle", "outline",
			func(t *testing.T, v bson.D) string { return bsonnav.DGetString(v, "PrimitiveValue") }},
		{"ButtonStyle", "ButtonStyle", "buttonStyle", "fillBlue",
			func(t *testing.T, v bson.D) string { return bsonnav.DGetString(v, "PrimitiveValue") }},
		{"icon", "icon", "icon", "bookmark",
			func(t *testing.T, v bson.D) string { return bsonnav.DGetString(v, "PrimitiveValue") }},
		{"tooltip", "Tooltip", "tooltip", "Hint", templateText},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := pdsButton()
			if err := setRawWidgetPropertyMut(w, tc.prop, tc.value); err != nil {
				t.Fatalf("set %s: %v", tc.prop, err)
			}
			if got := tc.read(t, ownValue(t, w, tc.key)); got != tc.value {
				t.Errorf("own %q = %q, want %q", tc.key, got, tc.value)
			}
			// Nothing core is invented beside it.
			if bsonnav.DGet(w, "LabelTemplate") != nil {
				t.Errorf("a core LabelTemplate was written: %v", bsonnav.DGet(w, "LabelTemplate"))
			}
			for _, k := range []string{"ButtonStyle", "Caption", "CaptionTemplate"} {
				if hasKey(w, k) {
					t.Errorf("core key %s was added to a CustomWidget", k)
				}
			}
		})
	}
}

// The widget's own enumeration is the one a value is judged against: a key it
// does not declare stays refused by the pluggable setter's existing rules
// rather than being stored (or silently dropped) under a core name.
func TestSetPluggableOwnCoreName_UnknownOwnKeyStillErrors(t *testing.T) {
	w := pdsShapedWidget(pdsProp{"value", "TextTemplate", 0x41})
	err := setRawWidgetPropertyMut(w, "caption", "x")
	if err == nil || !strings.Contains(err.Error(), "Caption") {
		t.Fatalf("a pluggable widget without an own `caption` keeps the core refusal, got %v", err)
	}
}

// Control: a pluggable widget that declares the Label SYSTEM property (a combo
// box) — `label` is the widget-level label, written to LabelTemplate as before.
func TestSetPluggableOwnCoreName_SystemLabelKeepsCoreRoute(t *testing.T) {
	w := pdsShapedWidget(pdsProp{"Label", "System", 0x51})
	tmpl := bson.D{
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Template", Value: bson.D{
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3), bson.D{
				{Key: "$Type", Value: "Texts$Translation"},
				{Key: "LanguageCode", Value: "en_US"},
				{Key: "Text", Value: "old"},
			}}},
		}},
	}
	bsonnav.DSet(w, "LabelTemplate", tmpl)
	if err := setRawWidgetPropertyMut(w, "label", "New"); err != nil {
		t.Fatal(err)
	}
	if got := templateText(t, bson.D{{Key: "TextTemplate", Value: bsonnav.DGet(w, "LabelTemplate")}}); got != "New" {
		t.Errorf("core LabelTemplate = %q, want New", got)
	}
}

// Control: a core action button keeps its Caption and ButtonStyle routes.
func TestSetPluggableOwnCoreName_CoreButtonUnchanged(t *testing.T) {
	w := bson.D{
		{Key: "$Type", Value: "Forms$ActionButton"},
		{Key: "Name", Value: "ab"},
		{Key: "ButtonStyle", Value: "Default"},
		{Key: "CaptionTemplate", Value: bson.D{
			{Key: "$Type", Value: "Forms$ClientTemplate"},
			{Key: "Parameters", Value: bson.A{int32(2)}},
			{Key: "Template", Value: bson.D{
				{Key: "$Type", Value: "Texts$Text"},
				{Key: "Items", Value: bson.A{int32(3), bson.D{
					{Key: "$Type", Value: "Texts$Translation"},
					{Key: "LanguageCode", Value: "en_US"},
					{Key: "Text", Value: "old"},
				}}},
			}},
		}},
	}
	if err := setRawWidgetPropertyMut(w, "caption", "Go"); err != nil {
		t.Fatal(err)
	}
	if err := setRawWidgetPropertyMut(w, "buttonStyle", "Primary"); err != nil {
		t.Fatal(err)
	}
	if got := templateText(t, bson.D{{Key: "TextTemplate", Value: bsonnav.DGet(w, "CaptionTemplate")}}); got != "Go" {
		t.Errorf("CaptionTemplate = %q, want Go", got)
	}
	if got := bsonnav.DGetString(w, "ButtonStyle"); got != "Primary" {
		t.Errorf("ButtonStyle = %q, want Primary", got)
	}
}
