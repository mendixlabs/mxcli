// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#1897, measured on TestApp's WorkflowCommons pages: two more
// differences no MDL statement can state made a describe → create or replace
// round trip rewrite an unchanged page.
//   - An expression stored with a trailing newline ("true\n", a visibility
//     condition ending in "\n"): MDL writes expressions bare, so the
//     whitespace cannot be expressed and came back trimmed.
//   - An empty screen-reader label: Studio Pro stores a ClientTemplate with no
//     text and no parameters, the writer null. Both mean "no label".
// Each is carried from the stored element of the same $ID, so the write is
// elided; a real change still lands.

func textEmpty(id byte) bson.D {
	return bson.D{{Key: "$ID", Value: id16(id)}, {Key: "$Type", Value: "Texts$Text"}, {Key: "Items", Value: bson.A{int32(3)}}}
}

func emptyTemplate() bson.D {
	return bson.D{
		{Key: "$ID", Value: id16(0x30)},
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Fallback", Value: textEmpty(0x31)},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Template", Value: textEmpty(0x32)},
	}
}

func widget(expr string, label any) []byte {
	w := bson.D{
		{Key: "$ID", Value: id16(0x10)},
		{Key: "$Type", Value: "Forms$TextArea"},
		{Key: "ConditionalVisibilitySettings", Value: bson.D{
			{Key: "$ID", Value: id16(0x11)},
			{Key: "$Type", Value: "Forms$ConditionalVisibilitySettings"},
			{Key: "Expression", Value: expr},
		}},
		{Key: "ScreenReaderLabel", Value: label},
	}
	b, _ := bson.Marshal(bson.D{{Key: "$ID", Value: id16(0xAA)}, {Key: "$Type", Value: "Forms$Page"}, {Key: "Widgets", Value: bson.A{int32(2), w}}})
	return b
}

func TestUnexpressed_RoundTripIsElided(t *testing.T) {
	stored := widget("$currentObject/TotalCount > 0\n", emptyTemplate())
	written := widget("$currentObject/TotalCount > 0", nil)
	if eq, _ := Equal(written, stored); eq {
		t.Fatal("control: the documents should differ before the carries")
	}
	if _, unchanged := Reconcile(written, stored); !unchanged {
		t.Error("a round trip that changed nothing was not elided")
	}
}

// Controls: a real change to either property still lands.
func TestUnexpressed_ARealChangeLands(t *testing.T) {
	stored := widget("$currentObject/TotalCount > 0\n", emptyTemplate())
	if _, unchanged := Reconcile(widget("$currentObject/TotalCount > 1", nil), stored); unchanged {
		t.Error("a changed expression was elided")
	}
	label := emptyTemplate()
	label[2].Value = bson.D{{Key: "$ID", Value: id16(0x31)}, {Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{int32(3), bson.D{{Key: "$ID", Value: id16(0x40)}, {Key: "$Type", Value: "Texts$Translation"}, {Key: "LanguageCode", Value: "en_US"}, {Key: "Text", Value: "Comment"}}}}}
	if _, unchanged := Reconcile(widget("$currentObject/TotalCount > 0", nil), widget("$currentObject/TotalCount > 0\n", label)); unchanged {
		t.Error("removing a label that has text was elided")
	}
}
