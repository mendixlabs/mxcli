// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2106, measured on the Marketplace modules FactoryManagement ships
// (AgentCommons 4.4.0, MxGenAIConnector 7.4.0) in a Mendix 11.16.1 project:
// describe → exec rewrote pages that had not changed, because of two more
// values MDL does not state.
//   - A widget's TabIndex. Studio Pro stores what the author set (1 on a footer
//     button, -1 on a combo box); the writer writes 0.
//   - The parameters of an input widget's label template. A check box labelled
//     '{1}' with its parameter bound to an expression or attribute came back
//     with no parameter — the label would read "{1}".

func tabWidget(tab int32, labelParams bson.A, labelText string) []byte {
	params := bson.A{int32(2)}
	params = append(params, labelParams...)
	w := bson.D{
		{Key: "$ID", Value: id16(0x10)},
		{Key: "$Type", Value: "Forms$CheckBox"},
		{Key: "LabelTemplate", Value: bson.D{
			{Key: "$ID", Value: id16(0x20)},
			{Key: "$Type", Value: "Forms$ClientTemplate"},
			{Key: "Fallback", Value: textEmpty(0x21)},
			{Key: "Parameters", Value: params},
			{Key: "Template", Value: bson.D{
				{Key: "$ID", Value: id16(0x22)},
				{Key: "$Type", Value: "Texts$Text"},
				{Key: "Items", Value: bson.A{int32(3), bson.D{
					{Key: "$ID", Value: id16(0x23)},
					{Key: "$Type", Value: "Texts$Translation"},
					{Key: "LanguageCode", Value: "en_US"},
					{Key: "Text", Value: labelText},
				}}},
			}},
		}},
		{Key: "TabIndex", Value: tab},
	}
	b, _ := bson.Marshal(bson.D{{Key: "$ID", Value: id16(0xAA)}, {Key: "$Type", Value: "Forms$Page"}, {Key: "Widgets", Value: bson.A{int32(2), w}}})
	return b
}

func labelParam(expr string) bson.D {
	return bson.D{
		{Key: "$ID", Value: id16(0x40)},
		{Key: "$Type", Value: "Forms$ClientTemplateParameter"},
		{Key: "AttributeRef", Value: nil},
		{Key: "Expression", Value: expr},
	}
}

func TestUnexpressed_TabIndexIsCarried(t *testing.T) {
	stored := tabWidget(1, nil, "Save")
	written := tabWidget(0, nil, "Save")
	if eq, _ := Equal(written, stored); eq {
		t.Fatal("control: the documents should differ before the carry")
	}
	if _, unchanged := Reconcile(written, stored); !unchanged {
		t.Error("a TabIndex MDL cannot state was reset to 0")
	}
	// Studio Pro stores TabIndex as int64; the writer writes int32.
	var doc bson.D
	_ = bson.Unmarshal(tabWidget(-1, nil, "Save"), &doc)
	w := doc[2].Value.(bson.A)[1].(bson.D)
	for i := range w {
		if w[i].Key == "TabIndex" {
			w[i].Value = int64(-1)
		}
	}
	stored64, _ := bson.Marshal(doc)
	if _, unchanged := Reconcile(written, stored64); !unchanged {
		t.Error("a TabIndex stored as int64 (Studio Pro's width) was reset to 0")
	}
}

func TestUnexpressed_LabelParametersAreCarried(t *testing.T) {
	stored := tabWidget(0, bson.A{labelParam("if $currentObject/EnableAll then 'Deselect all' else 'Select all'")}, "{1}")
	written := tabWidget(0, nil, "{1}")
	if eq, _ := Equal(written, stored); eq {
		t.Fatal("control: the documents should differ before the carry")
	}
	if _, unchanged := Reconcile(written, stored); !unchanged {
		t.Error("the label's parameter was dropped; the label would read {1}")
	}
}

// A real change lands: other label text, or parameters the statement does
// write, are the statement's.
func TestUnexpressed_LabelChangesStillLand(t *testing.T) {
	stored := tabWidget(0, bson.A{labelParam("'a'")}, "{1}")
	if _, unchanged := Reconcile(tabWidget(0, nil, "Plain label"), stored); unchanged {
		t.Error("a label whose text changed was elided")
	}
	if _, unchanged := Reconcile(tabWidget(0, bson.A{labelParam("'b'")}, "{1}"), stored); unchanged {
		t.Error("a label whose parameter changed was elided")
	}
}
