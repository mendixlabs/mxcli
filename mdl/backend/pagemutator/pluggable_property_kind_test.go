// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// kindedPluggableWidget builds a CustomWidget with one property keyed propKey,
// whose PropertyType declares ValueType.Type = kind, and whose stored Value
// carries every variant field at once — as a real WidgetValue does. The
// TextTemplate holds oldText when withTemplate is set, and is null otherwise.
func kindedPluggableWidget(propKey, kind, oldText string, withTemplate bool) bson.D {
	typeID := primitive.Binary{Subtype: 0x04, Data: []byte{
		0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28,
		0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30,
	}}
	var tmpl any
	if withTemplate {
		tmpl = bson.D{
			{Key: "$Type", Value: "Forms$ClientTemplate"},
			{Key: "Parameters", Value: bson.A{int32(2)}},
			{Key: "Template", Value: bson.D{
				{Key: "$Type", Value: "Texts$Text"},
				{Key: "Items", Value: bson.A{int32(3), bson.D{
					{Key: "$Type", Value: "Texts$Translation"},
					{Key: "LanguageCode", Value: "en_US"},
					{Key: "Text", Value: oldText},
				}}},
			}},
		}
	}
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "img1"},
		{Key: "Type", Value: bson.D{
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{int32(2), bson.D{
					{Key: "$ID", Value: typeID},
					{Key: "PropertyKey", Value: propKey},
					{Key: "ValueType", Value: bson.D{{Key: "Type", Value: kind}}},
				}}},
			}},
		}},
		{Key: "Object", Value: bson.D{
			{Key: "Properties", Value: bson.A{int32(2), bson.D{
				{Key: "TypePointer", Value: typeID},
				{Key: "Value", Value: bson.D{
					{Key: "Expression", Value: ""},
					{Key: "PrimitiveValue", Value: ""},
					{Key: "TextTemplate", Value: tmpl},
				}},
			}}},
		}},
	}
}

func kindedValue(t *testing.T, w bson.D) bson.D {
	t.Helper()
	props := bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(w, "Object"), "Properties"))
	return bsonnav.DGetDoc(props[0].(bson.D), "Value")
}

func templateText(t *testing.T, valDoc bson.D) string {
	t.Helper()
	tmpl := bsonnav.DGetDoc(valDoc, "TextTemplate")
	if tmpl == nil {
		return ""
	}
	items := bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(tmpl, "Template"), "Items"))
	for _, it := range items {
		if d, ok := it.(bson.D); ok && bsonnav.DGetString(d, "$Type") == "Texts$Translation" {
			return bsonnav.DGetString(d, "Text")
		}
	}
	return ""
}

// mendixlabs/mxcli#1201: `ALTER PAGE … SET ImageUrl = … ON img1` printed
// "Altered page" and changed nothing. The pluggable Image widget's `imageUrl`
// is a TextTemplate-kind property; the setter wrote PrimitiveValue, which
// DESCRIBE, mx check and the runtime never read.
func TestSetPluggableProperty_TextTemplateKindWritesTheTemplate(t *testing.T) {
	w := kindedPluggableWidget("imageUrl", "TextTemplate", "https://a.example/x.png", true)
	if err := setPluggableWidgetPropertyMut(w, "ImageUrl", "https://b.example/y.png"); err != nil {
		t.Fatalf("set: %v", err)
	}
	val := kindedValue(t, w)
	if got := templateText(t, val); got != "https://b.example/y.png" {
		t.Errorf("TextTemplate text = %q, want the new URL", got)
	}
	if pv := bsonnav.DGetString(val, "PrimitiveValue"); pv != "" {
		t.Errorf("value also written to PrimitiveValue (%q), a field nothing reads for this kind", pv)
	}
}

// A null TextTemplate is how a template stores while its condition hides it
// (#574). ALTER does not re-run visibility, so writing there would put text in
// a pruned slot; it is refused rather than reported as success.
func TestSetPluggableProperty_NullTextTemplateIsRefused(t *testing.T) {
	w := kindedPluggableWidget("imageUrl", "TextTemplate", "", false)
	err := setPluggableWidgetPropertyMut(w, "ImageUrl", "https://b.example/y.png")
	if err == nil {
		t.Fatal("expected an error for a null (hidden) text template, got success")
	}
	if tmpl := bsonnav.DGet(kindedValue(t, w), "TextTemplate"); tmpl != nil {
		t.Errorf("text template was created in a hidden slot: %v", tmpl)
	}
}

func TestSetPluggableProperty_ExpressionKindWritesExpression(t *testing.T) {
	w := kindedPluggableWidget("visibleExpr", "Expression", "", false)
	if err := setPluggableWidgetPropertyMut(w, "visibleExpr", "$currentObject/Active"); err != nil {
		t.Fatalf("set: %v", err)
	}
	val := kindedValue(t, w)
	if got := bsonnav.DGetString(val, "Expression"); got != "$currentObject/Active" {
		t.Errorf("Expression = %q", got)
	}
	if pv := bsonnav.DGetString(val, "PrimitiveValue"); pv != "" {
		t.Errorf("value written to PrimitiveValue (%q) for an Expression-kind property", pv)
	}
}

// Structured kinds cannot be expressed as a plain SET value. Before, they took
// a string in PrimitiveValue and reported success.
func TestSetPluggableProperty_StructuredKindsAreRefused(t *testing.T) {
	for _, kind := range []string{"Image", "Icon", "Action", "DataSource", "Attribute", "Widgets"} {
		w := kindedPluggableWidget("prop", kind, "", false)
		err := setPluggableWidgetPropertyMut(w, "prop", "x")
		if err == nil {
			t.Errorf("%s: expected a refusal, got success", kind)
			continue
		}
		if !strings.Contains(err.Error(), kind) {
			t.Errorf("%s: error should name the kind, got: %v", kind, err)
		}
		if pv := bsonnav.DGetString(kindedValue(t, w), "PrimitiveValue"); pv != "" {
			t.Errorf("%s: wrote PrimitiveValue %q despite refusing", kind, pv)
		}
	}
}

// Primitives keep landing in PrimitiveValue.
func TestSetPluggableProperty_PrimitiveKindUnchanged(t *testing.T) {
	w := kindedPluggableWidget("widthUnit", "Enumeration", "", false)
	if err := setPluggableWidgetPropertyMut(w, "widthUnit", "pixels"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if pv := bsonnav.DGetString(kindedValue(t, w), "PrimitiveValue"); pv != "pixels" {
		t.Errorf("PrimitiveValue = %q, want pixels", pv)
	}
}

// A bracketed value arrives as []string and %v fused its tokens into one
// string written as success (#750). No pluggable property takes a list.
func TestSetPluggableProperty_ListValueIsRefused(t *testing.T) {
	w := kindedPluggableWidget("visibleExpr", "Expression", "", false)
	if err := setPluggableWidgetPropertyMut(w, "visibleExpr", []string{"if", "$x", "then"}); err == nil {
		t.Fatal("expected a list value to be refused")
	}
}
