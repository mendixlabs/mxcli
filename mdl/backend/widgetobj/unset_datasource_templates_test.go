// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1967 (mendixlabs/mxcli#1745): the reporter's widget pairs
//
//	<property key="logicItems" type="datasource" isList="true" required="false">
//	<property key="logicText" type="textTemplate" dataSource="logicItems" required="false">
//
// An unset logicText whose logicItems is unset is null in Studio Pro; mxcli
// wrote an empty Forms$ClientTemplate, CE0463. With logicItems set, an unset
// logicText stays an empty template (the reporter's control (c)).
func TestNullTemplatesOfUnsetDataSources(t *testing.T) {
	const (
		dsID     = "11111111-1111-1111-1111-111111111111"
		textID   = "22222222-2222-2222-2222-222222222222"
		colTxtID = "33333333-3333-3333-3333-333333333333"
	)
	ids := map[string]pages.PropertyTypeIDEntry{
		"logicItems":  {PropertyTypeID: dsID, ValueType: "DataSource"},
		"logicText":   {PropertyTypeID: textID, ValueType: "TextTemplate", DataSourceProperty: "logicItems"},
		"columnTitle": {PropertyTypeID: colTxtID, ValueType: "TextTemplate", DataSourceProperty: "../datasource"},
	}
	authored := BuildClientTemplateWithTextAndParams("Rule {1}", nil)
	prop := func(id string, ds, tt any) bson.D {
		return bson.D{
			{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
			{Key: "TypePointer", Value: types.UUIDToBlob(id)},
			{Key: "Value", Value: bson.D{
				{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
				{Key: "DataSource", Value: ds},
				{Key: "TextTemplate", Value: tt},
			}},
		}
	}
	object := func(ds, text any) bson.D {
		return bson.D{{Key: "Properties", Value: bson.A{int32(2),
			prop(dsID, ds, nil), prop(textID, nil, text), prop(colTxtID, nil, BuildEmptyClientTemplate()),
		}}}
	}
	templateOf := func(obj bson.D, id string) any {
		for _, item := range obj[0].Value.(bson.A)[1:] {
			p := item.(bson.D)
			if propertyTypePointerID(p) == normalizeID(id) {
				for _, e := range widgetValueOfProperty(p) {
					if e.Key == "TextTemplate" {
						return e.Value
					}
				}
			}
		}
		t.Fatalf("no property %s", id)
		return nil
	}
	setDS := bson.D{{Key: "$Type", Value: "CustomWidgets$CustomWidgetXPathSource"}}

	if got := templateOf(NullTemplatesOfUnsetDataSources(object(nil, BuildEmptyClientTemplate()), ids), textID); got != nil {
		t.Errorf("(a) data source unset, template unset: TextTemplate = %v, want null", got)
	}
	if got := templateOf(NullTemplatesOfUnsetDataSources(object(setDS, BuildEmptyClientTemplate()), ids), textID); got == nil {
		t.Error("(c) data source set, template unset: nulled; Studio Pro keeps the empty template")
	}
	if got := templateOf(NullTemplatesOfUnsetDataSources(object(nil, authored), ids), textID); got == nil {
		t.Error("an authored template was dropped")
	}
	if got := templateOf(NullTemplatesOfUnsetDataSources(object(nil, BuildEmptyClientTemplate()), ids), colTxtID); got == nil {
		t.Error("a template bound to the enclosing widget's data source was nulled")
	}
}
