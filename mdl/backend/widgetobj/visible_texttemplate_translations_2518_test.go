// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2518: a conditional text template that is VISIBLE and unset is
// written with the property's shipped default text when it declares one.
// `mx update-widgets` writes the Drop-down filter's visible emptyOptionCaption
// as en_US "None" / nl_NL "Niets" (mx 11.14.0); an empty template there is
// CE0463 just as a null one is.
func TestVisibleConditionalTextTemplateGetsDeclaredTranslations(t *testing.T) {
	const (
		multiID   = "77777777-7777-7777-7777-777777777777" // boolean "multiSelect"
		captionID = "88888888-8888-8888-8888-888888888888" // TextTemplate "emptyOptionCaption"
	)
	mkProp := func(id, primitiveVal string) bson.D {
		return bson.D{
			{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
			{Key: "TypePointer", Value: types.UUIDToBlob(id)},
			{Key: "Value", Value: bson.D{
				{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
				{Key: "PrimitiveValue", Value: primitiveVal},
				{Key: "TextTemplate", Value: nil},
			}},
		}
	}
	ids := map[string]pages.PropertyTypeIDEntry{
		"multiSelect": {PropertyTypeID: multiID, ValueType: "Boolean"},
		"emptyOptionCaption": {PropertyTypeID: captionID, ValueType: "TextTemplate",
			DefaultTranslations: []pages.PropertyTranslation{{LanguageCode: "en_US", Text: "None"}, {LanguageCode: "nl_NL", Text: "Niets"}}},
	}
	rules := []types.WidgetVisibilityRule{{
		PropertyKey: "emptyOptionCaption",
		HiddenWhen:  &types.WidgetVisibilityCondition{PropertyKey: "multiSelect", Operator: "truthy"},
	}}
	object := func(multi string) bson.D {
		return bson.D{{Key: "Properties", Value: bson.A{int32(2), mkProp(multiID, multi), mkProp(captionID, "")}}}
	}

	tt, ok := templateOf(t, ApplyVisibilityRules(object("false"), ids, rules), captionID).(bson.D)
	if !ok {
		t.Fatal("a visible conditional text template was left null")
	}
	tmpl, _ := docField(tt, "Template").(bson.D)
	items, _ := docField(tmpl, "Items").(bson.A)
	var got []string
	for _, it := range items {
		if d, ok := it.(bson.D); ok {
			got = append(got, docField(d, "LanguageCode").(string)+"="+docField(d, "Text").(string))
		}
	}
	if len(got) != 2 || got[0] != "en_US=None" || got[1] != "nl_NL=Niets" {
		t.Errorf("visible emptyOptionCaption = %v, want the declared translations [en_US=None nl_NL=Niets]", got)
	}

	// Control: hidden, it is null whatever it declares.
	if tt := templateOf(t, ApplyVisibilityRules(object("true"), ids, rules), captionID); tt != nil {
		t.Errorf("a hidden text template was filled: %v", tt)
	}
}

func docField(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}
