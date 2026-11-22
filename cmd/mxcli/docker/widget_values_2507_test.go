// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2507: a CE0463 caused by a script value in a property the widget's
// editorConfig hides was labelled "stale widget definition — `mxcli fix
// widgets` fixes it". update-widgets clears it by resetting the property to
// its default, so the value the script set is dropped without a word. The
// widgets are compared before and after, by property key.

func id2507(b byte) bson.Binary {
	return bson.Binary{Subtype: 0, Data: []byte{b, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, b}}
}

// chessBoard2507 is a CustomWidgets$CustomWidget named name whose
// showNotationInput and notationLanguage hold the given values.
func chessBoard2507(name, showInput, language string) bson.D {
	propType := func(pid byte, key string) bson.D {
		return bson.D{{Key: "$ID", Value: id2507(pid)}, {Key: "$Type", Value: "CustomWidgets$WidgetPropertyType"}, {Key: "PropertyKey", Value: key}}
	}
	prop := func(vid, pid byte, primitive string) bson.D {
		return bson.D{{Key: "$ID", Value: id2507(vid)}, {Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
			{Key: "TypePointer", Value: id2507(pid)},
			{Key: "Value", Value: bson.D{{Key: "$ID", Value: id2507(vid + 100)}, {Key: "$Type", Value: "CustomWidgets$WidgetValue"},
				{Key: "PrimitiveValue", Value: primitive}}}}
	}
	return bson.D{
		{Key: "$ID", Value: id2507(1)}, {Key: "$Type", Value: "CustomWidgets$CustomWidget"}, {Key: "Name", Value: name},
		{Key: "Type", Value: bson.D{{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "ObjectType", Value: bson.D{{Key: "$Type", Value: "CustomWidgets$WidgetObjectType"},
				{Key: "PropertyTypes", Value: bson.A{int32(2), propType(10, "showNotationInput"), propType(11, "notationLanguage"), propType(12, "fen")}}}}}},
		{Key: "Object", Value: bson.D{{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
			{Key: "Properties", Value: bson.A{int32(2), prop(20, 10, showInput), prop(21, 11, language), prop(22, 12, "FEN")}}}},
	}
}

func page2507(t *testing.T, widgets ...bson.D) harvestedUnit {
	t.Helper()
	items := bson.A{int32(2)}
	for _, w := range widgets {
		items = append(items, w)
	}
	raw, err := bson.Marshal(bson.D{{Key: "$Type", Value: "Forms$Page"}, {Key: "Name", Value: "Settings_Edit"}, {Key: "Widgets", Value: items}})
	if err != nil {
		t.Fatal(err)
	}
	return harvestedUnit{contents: raw, unitType: "Forms$Page"}
}

func TestWidgetValueChangesNameTheResetProperties_2507(t *testing.T) {
	before := map[string]harvestedUnit{"p": page2507(t, chessBoard2507("previewBoard", "false", "nl"), chessBoard2507("mainBoard", "true", "en"))}
	// update-widgets resets the hidden properties on previewBoard only, and
	// renumbers every $ID (which is no value change).
	afterBoard := chessBoard2507("previewBoard", "true", "en")
	afterBoard[0].Value = id2507(99)
	after := map[string]harvestedUnit{"p": page2507(t, afterBoard, chessBoard2507("mainBoard", "true", "en"))}

	got := widgetValueChanges(widgetPropertyValues(before), widgetPropertyValues(after))
	if len(got) != 1 || got[0].Widget != "previewBoard" || got[0].Document != "Settings_Edit" {
		t.Fatalf("changes = %+v, want previewBoard only", got)
	}
	if len(got[0].Properties) != 2 || got[0].Properties[0] != "notationLanguage" || got[0].Properties[1] != "showNotationInput" {
		t.Errorf("properties = %v, want notationLanguage, showNotationInput", got[0].Properties)
	}
	// Control: the same project before and after changes nothing.
	if c := widgetValueChanges(widgetPropertyValues(before), widgetPropertyValues(before)); len(c) != 0 {
		t.Errorf("unchanged project reports %+v", c)
	}
}

// update-widgets re-saves values with their keys sorted (a DataGrid2's
// DataSource, measured on mxchess with 11.15.0): the same value, not a reset.
func TestKeyOrderIsNoValueChange_2507(t *testing.T) {
	a := bson.D{{Key: "$Type", Value: "Forms$DataViewSource"}, {Key: "ForceFullObjects", Value: false}, {Key: "EntityRef", Value: "M.E"}}
	b := bson.D{{Key: "$Type", Value: "Forms$DataViewSource"}, {Key: "EntityRef", Value: "M.E"}, {Key: "ForceFullObjects", Value: false}}
	if canonicalValue(a) != canonicalValue(b) {
		t.Errorf("key order made a change:\n%s\n%s", canonicalValue(a), canonicalValue(b))
	}
	// Control: a different value is still one.
	c := bson.D{{Key: "$Type", Value: "Forms$DataViewSource"}, {Key: "EntityRef", Value: "M.Other"}, {Key: "ForceFullObjects", Value: false}}
	if canonicalValue(a) == canonicalValue(c) {
		t.Error("a different entity is no change")
	}
}

func TestCE0463WidgetName_2507(t *testing.T) {
	for at, want := range map[string]string{
		"Chess Board 'previewBoard'": "previewBoard",
		"Data grid 2 'dgOrders'":     "dgOrders",
		"Page 'Home'":                "Home",
		"somewhere":                  "",
	} {
		if got := ce0463WidgetName(at); got != want {
			t.Errorf("%q: %q, want %q", at, got, want)
		}
	}
}
