// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

func widthTestID(b byte) bson.Binary {
	return bson.Binary{Subtype: 0x00, Data: bytes.Repeat([]byte{b}, 16)}
}

// slotWidget is a pluggable widget shaped like a Fieldset holding a text box in
// its `content` slot, with a second property `legend` that the definition below
// no longer declares, so a sync removes it. Integers are stored the way
// Studio Pro and `exec` store them: int32 markers and fields, one int64.
func fieldsetWithSlotTextBox() bson.D {
	textBox := bson.D{
		{Key: "$ID", Value: widthTestID(0x20)},
		{Key: "$Type", Value: "Forms$TextBox"},
		{Key: "DecimalPrecision", Value: int64(2)},
		{Key: "MaxLengthCode", Value: int32(-1)},
		{Key: "Name", Value: "tbInner"},
		{Key: "TabIndex", Value: int32(0)},
		{Key: "Weight", Value: float64(0.5)},
	}
	prop := func(id, typeID byte, value bson.D) bson.D {
		return bson.D{
			{Key: "$ID", Value: widthTestID(id)},
			{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
			{Key: "TypePointer", Value: widthTestID(typeID)},
			{Key: "Value", Value: value},
		}
	}
	propType := func(id byte, key, typ string) bson.D {
		return bson.D{
			{Key: "$ID", Value: widthTestID(id)},
			{Key: "$Type", Value: "CustomWidgets$WidgetPropertyType"},
			{Key: "Caption", Value: key},
			{Key: "Category", Value: "General"},
			{Key: "Description", Value: ""},
			{Key: "IsDefault", Value: false},
			{Key: "PropertyKey", Value: key},
			{Key: "ValueType", Value: bson.D{
				{Key: "$ID", Value: widthTestID(id + 1)},
				{Key: "$Type", Value: "CustomWidgets$WidgetValueType"},
				{Key: "EnumerationValues", Value: bson.A{int32(2)}},
				{Key: "Translations", Value: bson.A{int32(2)}},
				{Key: "Type", Value: typ},
			}},
		}
	}
	return bson.D{
		{Key: "$ID", Value: widthTestID(0x01)},
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "fs"},
		{Key: "Object", Value: bson.D{
			{Key: "$ID", Value: widthTestID(0x02)},
			{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
			{Key: "Properties", Value: bson.A{
				int32(2),
				prop(0x03, 0x10, bson.D{
					{Key: "$ID", Value: widthTestID(0x04)},
					{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
					{Key: "PrimitiveValue", Value: "Legend"},
					{Key: "Widgets", Value: bson.A{int32(2)}},
				}),
				prop(0x05, 0x12, bson.D{
					{Key: "$ID", Value: widthTestID(0x06)},
					{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
					{Key: "PrimitiveValue", Value: ""},
					{Key: "Widgets", Value: bson.A{int32(2), textBox}},
				}),
			}},
		}},
		{Key: "Type", Value: bson.D{
			{Key: "$ID", Value: widthTestID(0x07)},
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "ObjectType", Value: bson.D{
				{Key: "$ID", Value: widthTestID(0x08)},
				{Key: "$Type", Value: "CustomWidgets$WidgetObjectType"},
				{Key: "PropertyTypes", Value: bson.A{
					int32(2),
					propType(0x10, "legend", "String"),
					propType(0x12, "content", "Widgets"),
				}},
			}},
			{Key: "WidgetId", Value: "com.mendix.widget.web.fieldset.Fieldset"},
		}},
	}
}

// ako/mxcli#1976 (mendixlabs/mxcli#1748): the map form turned every int32 and
// int64 into float64 and nothing turned them back, so a synced widget was
// written with doubles throughout, and a text box in its slot read
// MaxLengthCode -1.0 as "unlimited" (CE6510). The round trip with no
// reconciliation must be byte-identical for every numeric width Mendix stores,
// a genuine double included.
func TestWidgetRoundTripKeepsNumericWidths(t *testing.T) {
	widget := fieldsetWithSlotTextBox()
	original, err := bson.Marshal(widget)
	if err != nil {
		t.Fatal(err)
	}
	back, ok := mapToWidgetDoc(widgetToMap(widget)).(bson.D)
	if !ok {
		t.Fatal("mapToWidgetDoc did not return a bson.D")
	}
	encoded, err := bson.Marshal(back)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, encoded) {
		t.Errorf("round trip changed the document:\n got %v", back)
	}
}

// The same, through the sync itself: dropping the stale `legend` property must
// leave the text box in the slot, and every marker, at its stored width.
func TestWidgetSyncKeepsNumericWidthsInSlot(t *testing.T) {
	def := &mpk.WidgetDefinition{
		ID:          "com.mendix.widget.web.fieldset.Fieldset",
		IsPluggable: true,
		Properties:  []mpk.PropertyDef{{Key: "content", Type: "widgets", Caption: "content", Category: "General"}},
	}
	got, ok := applyToWidget(fieldsetWithSlotTextBox(), def)
	if !ok {
		t.Fatal("applyToWidget reported no change")
	}
	raw, err := bson.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var doubles []string
	walkRaw(bson.Raw(raw), "", func(path string, v bson.RawValue) {
		if v.Type == bson.TypeDouble && path != "Object.Properties.1.Value.Widgets.1.Weight" {
			doubles = append(doubles, path)
		}
	})
	if len(doubles) > 0 {
		t.Errorf("sync wrote integers as doubles at: %v", doubles)
	}
	if bytes.Contains(raw, []byte("legend")) {
		t.Error("the stale property was not removed; the test does not exercise a change")
	}
}

// walkRaw visits every leaf of a raw document with its dotted path.
func walkRaw(doc bson.Raw, prefix string, visit func(string, bson.RawValue)) {
	elems, _ := doc.Elements()
	for _, e := range elems {
		path := e.Key()
		if prefix != "" {
			path = prefix + "." + path
		}
		v := e.Value()
		switch v.Type {
		case bson.TypeEmbeddedDocument:
			walkRaw(v.Document(), path, visit)
		case bson.TypeArray:
			walkRaw(bson.Raw(v.Array()), path, visit)
		default:
			visit(path, v)
		}
	}
}
