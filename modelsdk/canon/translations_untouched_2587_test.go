// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A text the write did not touch keeps exactly the languages it had
// (ako/mxcli#2587). ALTER PAGE edits the stored BSON in place, so an untouched
// combobox comes back byte for byte, $IDs and all. But an insert shifts every
// path after it, which turns positional pairing off, and a pluggable widget's
// texts sit under its Properties list, which turns the named-element pairing
// off. Left with the source pairing, the one combobox whose screen-reader
// texts had no Dutch picked up the Dutch of the three that did.
func comboWith(id byte, name string, text bson.D) bson.D {
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "$ID", Value: bin(id)},
		{Key: "Name", Value: name},
		{Key: "Object", Value: bson.D{
			{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
			{Key: "$ID", Value: bin(id + 1)},
			{Key: "Properties", Value: bson.A{int32(2), bson.D{
				{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
				{Key: "$ID", Value: bin(id + 2)},
				{Key: "Value", Value: bson.D{
					{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
					{Key: "$ID", Value: bin(id + 3)},
					{Key: "TextTemplate", Value: bson.D{
						{Key: "$Type", Value: "Forms$ClientTemplate"},
						{Key: "$ID", Value: bin(id + 4)},
						{Key: "Template", Value: text},
					}},
				}},
			}}},
		}},
	}
}

func textWithID(id byte, items ...bson.D) bson.D {
	arr := bson.A{int32(3)}
	for _, it := range items {
		arr = append(arr, it)
	}
	return bson.D{{Key: "$Type", Value: "Texts$Text"}, {Key: "$ID", Value: bin(id)}, {Key: "Items", Value: arr}}
}

// comboText returns lang→text of the named combobox's one text.
func comboText(t *testing.T, raw []byte, name string) map[string]string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, w := range docLookup(doc, "Widgets").(bson.A)[1:] {
		wd := w.(bson.D)
		if docLookup(wd, "Name") != name {
			continue
		}
		props := docLookup(docLookup(wd, "Object").(bson.D), "Properties").(bson.A)
		val := docLookup(props[1].(bson.D), "Value").(bson.D)
		return textTranslations(docLookup(docLookup(val, "TextTemplate").(bson.D), "Template").(bson.D))
	}
	t.Fatalf("no widget %q", name)
	return nil
}

func TestCarryTranslations_LeavesAnUntouchedTextAlone_2587(t *testing.T) {
	translated := func() bson.D {
		return comboWith(10, "comboBox1", textWithID(20, tr(21, "en_US", "Clear selection"), tr(22, "nl_NL", "Selectie wissen")))
	}
	englishOnly := func() bson.D {
		return comboWith(30, "comboBox2", textWithID(40, tr(41, "en_US", "Clear selection")))
	}
	stored := pageOf(t, translated(), englishOnly())
	// The insert: a new widget ahead of both, which moves every path.
	written := pageOf(t, button(60, "inserted", widgetText(tr(62, "en_US", "New"))), translated(), englishOnly())

	out := CarryTranslations(written, stored)
	if got := comboText(t, out, "comboBox2"); len(got) != 1 || got["en_US"] != "Clear selection" {
		t.Errorf("untouched comboBox2 = %v, want only its stored en_US", got)
	}
	if got := comboText(t, out, "comboBox1"); got["nl_NL"] != "Selectie wissen" {
		t.Errorf("comboBox1 = %v, want its own nl_NL kept", got)
	}

	// The control: a text the rebuild minted afresh (no stored $ID) is still
	// paired by its source string, which is what the source pairing is for.
	rebuilt := pageOf(t, button(60, "inserted", widgetText(tr(62, "en_US", "New"))), translated(),
		comboWith(70, "comboBox3", textWithID(80, tr(81, "en_US", "Clear selection"))))
	out = CarryTranslations(rebuilt, stored)
	if got := comboText(t, out, "comboBox3"); got["nl_NL"] != "Selectie wissen" {
		t.Errorf("rebuilt comboBox3 = %v, want nl_NL carried by source", got)
	}
}
