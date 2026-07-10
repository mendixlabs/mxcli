// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// TestApplyPropertyVisibility locks in the #574 behavior: a TextTemplate
// property the rules mark as hidden under the widget's current configuration
// must have its TextTemplate nulled, while visible properties and
// non-TextTemplate properties are left untouched.
func TestApplyPropertyVisibility(t *testing.T) {
	const (
		typeID     = "11111111-1111-1111-1111-111111111111" // enum "type"
		videoURLID = "22222222-2222-2222-2222-222222222222" // TextTemplate "videoUrl"
		heightID   = "33333333-3333-3333-3333-333333333333" // string "height" (non-TextTemplate)
	)

	populatedTemplate := bson.D{{Key: "$Type", Value: "Forms$ClientTemplate"}}

	build := func(typeValue string) *Builder {
		mkProp := func(id, primitiveVal string, tt any) bson.D {
			return bson.D{
				{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
				{Key: "TypePointer", Value: types.UUIDToBlob(id)},
				{Key: "Value", Value: bson.D{
					{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
					{Key: "PrimitiveValue", Value: primitiveVal},
					{Key: "TextTemplate", Value: tt},
				}},
			}
		}
		return &Builder{
			widgetID: "com.mendix.widget.web.videoplayer.VideoPlayer",
			object: bson.D{{Key: "Properties", Value: bson.A{
				int32(2),
				mkProp(typeID, typeValue, nil),
				mkProp(videoURLID, "", populatedTemplate),
				mkProp(heightID, "", populatedTemplate), // pretend hidden non-TextTemplate
			}}},
			propertyTypeIDs: map[string]pages.PropertyTypeIDEntry{
				"type":     {PropertyTypeID: typeID, ValueType: "Enumeration"},
				"videoUrl": {PropertyTypeID: videoURLID, ValueType: "TextTemplate"},
				"height":   {PropertyTypeID: heightID, ValueType: "String"},
			},
		}
	}

	rules := []types.WidgetVisibilityRule{
		{PropertyKey: "videoUrl", HiddenWhen: &types.WidgetVisibilityCondition{PropertyKey: "type", Operator: "eq", Value: "expression"}},
		// A rule on a non-TextTemplate property: must be ignored even when hidden.
		{PropertyKey: "height", HiddenWhen: &types.WidgetVisibilityCondition{PropertyKey: "type", Operator: "eq", Value: "expression"}},
	}

	textTemplateOf := func(t *testing.T, ob *Builder, key string) any {
		t.Helper()
		id := ob.propertyTypeIDs[key].PropertyTypeID
		for _, elem := range ob.object {
			if elem.Key != "Properties" {
				continue
			}
			for _, item := range elem.Value.(bson.A) {
				prop, ok := item.(bson.D)
				if !ok {
					continue
				}
				if propertyTypePointerID(prop) != normalizeID(id) {
					continue
				}
				val := findField(t, prop, "Value").(bson.D)
				return findField(t, val, "TextTemplate")
			}
		}
		t.Fatalf("property %q not found", key)
		return nil
	}

	t.Run("hidden TextTemplate property is nulled", func(t *testing.T) {
		ob := build("expression")
		ob.ApplyPropertyVisibility(rules)
		if tt := textTemplateOf(t, ob, "videoUrl"); tt != nil {
			t.Errorf("videoUrl TextTemplate = %v, want nil (hidden when type=expression)", tt)
		}
		// Non-TextTemplate hidden property untouched.
		if tt := textTemplateOf(t, ob, "height"); tt == nil {
			t.Errorf("height TextTemplate was nulled; non-TextTemplate properties must be left untouched")
		}
	})

	t.Run("visible TextTemplate property is preserved", func(t *testing.T) {
		ob := build("dynamic")
		ob.ApplyPropertyVisibility(rules)
		if tt := textTemplateOf(t, ob, "videoUrl"); tt == nil {
			t.Errorf("videoUrl TextTemplate = nil, want preserved (visible when type=dynamic)")
		}
	})
}

func normalizeID(id string) string {
	out := make([]byte, 0, len(id))
	for i := 0; i < len(id); i++ {
		if id[i] != '-' {
			out = append(out, id[i])
		}
	}
	return string(out)
}
