// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2128 (mendixlabs/mxcli#1809): `mxcli diff` of an unchanged
// snippet's describe output was not clean, and exec rewrote it. Two more values
// no MDL statement states, measured on TestApp's WorkflowCommons snippets:
//   - a screen-reader label stored as a ClientTemplate whose one translation
//     has empty text (Snip_Assignee_Search's textBox3), written null;
//   - an attribute visibility over a System enumeration with values mxcli's
//     metadata does not have (System.WorkflowActivityType on 11.14: 25 stored
//     conditions, 17 written).

func translatedEmptyTemplate() bson.D {
	empty := bson.D{{Key: "$ID", Value: id16(0x33)}, {Key: "$Type", Value: "Texts$Translation"}, {Key: "LanguageCode", Value: "en_US"}, {Key: "Text", Value: ""}}
	return bson.D{
		{Key: "$ID", Value: id16(0x30)},
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Fallback", Value: textEmpty(0x31)},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Template", Value: bson.D{{Key: "$ID", Value: id16(0x32)}, {Key: "$Type", Value: "Texts$Text"}, {Key: "Items", Value: bson.A{int32(3), empty}}}},
	}
}

func TestUnexpressed_LabelWithAnEmptyTranslation(t *testing.T) {
	assertElided(t, "empty-text translation", widget("x", nil), widget("x", translatedEmptyTemplate()))
	// Control: a translation with text is a label, and dropping it lands.
	withText := translatedEmptyTemplate()
	tpl := docLookup(withText, "Template").(bson.D)
	item := docLookup(tpl, "Items").(bson.A)[1].(bson.D)
	for i := range item {
		if item[i].Key == "Text" {
			item[i].Value = "Search"
		}
	}
	assertLands(t, "label with text", widget("x", nil), widget("x", withText))
}

func TestUnexpressed_ConditionsForValuesMxcliDoesNotKnow(t *testing.T) {
	stored := visibility(condition(0x20, "Start", false), condition(0x21, "UserTask", true),
		condition(0x22, "AIAgentTask", false), condition(0x23, "(empty)", false))
	written := visibility(condition(0x20, "Start", false), condition(0x21, "UserTask", true), condition(0x22, "(empty)", false))
	assertElided(t, "stored values unknown to the writer", written, stored)
	// Controls: a flag changed on a value both know is a real change, and so is
	// a value the writer has that the stored list does not.
	assertLands(t, "flag changed", visibility(condition(0x20, "Start", true), condition(0x21, "UserTask", true), condition(0x22, "(empty)", false)), stored)
	assertLands(t, "new value", visibility(condition(0x20, "Start", false), condition(0x21, "UserTask", true), condition(0x22, "Other", false), condition(0x23, "(empty)", false)), stored)
}
