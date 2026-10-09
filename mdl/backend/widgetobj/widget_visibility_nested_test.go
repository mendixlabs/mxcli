// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// TestApplyPropertyVisibility_NestedObjectListItem locks in the File Uploader
// 2.5.0 fix: `allowedFileFormats` items carry a required TextTemplate
// `typeFormatDescription` that is hidden when the item's `configMode` is
// "simple". Studio Pro stores null there; a populated template is CE0463. The
// rule is nested (ListPropertyKey set) and must be evaluated per item, against
// that item's own configMode.
func TestApplyPropertyVisibility_NestedObjectListItem(t *testing.T) {
	const (
		listID  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		modeID  = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		descID  = "cccccccc-cccc-cccc-cccc-cccccccccccc"
		objType = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	)
	populated := bson.D{{Key: "$Type", Value: "Forms$ClientTemplate"}}
	mkProp := func(id, prim string, tt any) bson.D {
		return bson.D{
			{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
			{Key: "TypePointer", Value: types.UUIDToBlob(id)},
			{Key: "Value", Value: bson.D{
				{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
				{Key: "PrimitiveValue", Value: prim},
				{Key: "TextTemplate", Value: tt},
			}},
		}
	}
	mkItem := func(mode string) bson.D {
		return bson.D{
			{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
			{Key: "TypePointer", Value: types.UUIDToBlob(objType)},
			{Key: "Properties", Value: bson.A{int32(2), mkProp(modeID, mode, nil), mkProp(descID, "", populated)}},
		}
	}
	listProp := bson.D{
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: types.UUIDToBlob(listID)},
		{Key: "Value", Value: bson.D{
			{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
			{Key: "Objects", Value: bson.A{int32(2), mkItem("simple"), mkItem("advanced")}},
		}},
	}
	ob := &Builder{
		object: bson.D{{Key: "Properties", Value: bson.A{int32(2), listProp}}},
		propertyTypeIDs: map[string]pages.PropertyTypeIDEntry{
			"allowedFileFormats": {PropertyTypeID: listID, ValueType: "Object", ObjectTypeID: objType,
				NestedPropertyIDs: map[string]pages.PropertyTypeIDEntry{
					"configMode":            {PropertyTypeID: modeID, ValueType: "Enumeration"},
					"typeFormatDescription": {PropertyTypeID: descID, ValueType: "TextTemplate", Required: true},
				}},
		},
	}
	ob.ApplyPropertyVisibility([]types.WidgetVisibilityRule{{
		PropertyKey:     "typeFormatDescription",
		ListPropertyKey: "allowedFileFormats",
		HiddenWhen:      &types.WidgetVisibilityCondition{PropertyKey: "configMode", Operator: "eq", Value: "simple"},
	}})

	var items bson.A
	for _, e := range ob.object {
		if e.Key == "Properties" {
			val := findField(t, e.Value.(bson.A)[1].(bson.D), "Value").(bson.D)
			items = findField(t, val, "Objects").(bson.A)
		}
	}
	descOf := func(item bson.D) any {
		props := findField(t, item, "Properties").(bson.A)
		return findField(t, findField(t, props[2].(bson.D), "Value").(bson.D), "TextTemplate")
	}
	if tt := descOf(items[1].(bson.D)); tt != nil {
		t.Errorf("simple item: typeFormatDescription TextTemplate = %v, want nil", tt)
	}
	if tt := descOf(items[2].(bson.D)); tt == nil {
		t.Errorf("advanced item: typeFormatDescription TextTemplate was nulled; it is visible and must be kept")
	}
}
