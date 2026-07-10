// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// TestSetAttributeObjectsSetsLinkedAttrChoice locks in the fix for the
// textfilter envelope drift (#605 / 11.9→11.10): a filter widget given an
// explicit `attributes` list must select attrChoice="linked", not the "auto"
// default. attrChoice="auto" alongside a populated attributes list is flagged
// CE0463 by Studio Pro 11.10+.
func TestSetAttributeObjectsSetsLinkedAttrChoice(t *testing.T) {
	const (
		attrChoiceID  = "11111111-1111-1111-1111-111111111111"
		attributesID  = "22222222-2222-2222-2222-222222222222"
		attrSubID     = "33333333-3333-3333-3333-333333333333"
		attrValTypeID = "44444444-4444-4444-4444-444444444444"
		objTypeID     = "55555555-5555-5555-5555-555555555555"
	)

	mkProp := func(id string, value bson.D) bson.D {
		return bson.D{
			{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
			{Key: "TypePointer", Value: types.UUIDToBlob(id)},
			{Key: "Value", Value: value},
		}
	}
	newBuilder := func() *Builder {
		return &Builder{
			widgetID: "com.mendix.widget.web.datagridtextfilter.DatagridTextFilter",
			object: bson.D{{Key: "Properties", Value: bson.A{
				int32(2),
				mkProp(attrChoiceID, bson.D{{Key: "PrimitiveValue", Value: "auto"}}),
				mkProp(attributesID, bson.D{{Key: "Objects", Value: bson.A{int32(2)}}}),
			}}},
			propertyTypeIDs: map[string]pages.PropertyTypeIDEntry{
				"attrChoice": {PropertyTypeID: attrChoiceID, ValueType: "Enumeration"},
				"attributes": {
					PropertyTypeID: attributesID,
					ObjectTypeID:   objTypeID,
					NestedPropertyIDs: map[string]pages.PropertyTypeIDEntry{
						"attribute": {PropertyTypeID: attrSubID, ValueTypeID: attrValTypeID},
					},
				},
			},
		}
	}

	primitiveOf := func(t *testing.T, ob *Builder, key string) string {
		t.Helper()
		id := normalizeID(ob.propertyTypeIDs[key].PropertyTypeID)
		for _, elem := range ob.object {
			if elem.Key != "Properties" {
				continue
			}
			for _, item := range elem.Value.(bson.A) {
				prop, ok := item.(bson.D)
				if !ok || propertyTypePointerID(prop) != id {
					continue
				}
				val := findField(t, prop, "Value").(bson.D)
				if s, ok := findField(t, val, "PrimitiveValue").(string); ok {
					return s
				}
			}
		}
		t.Fatalf("property %q not found", key)
		return ""
	}

	t.Run("explicit attributes -> attrChoice linked", func(t *testing.T) {
		ob := newBuilder()
		ob.SetAttributeObjects("attributes", []string{"Mod.Ent.Name"})
		if got := primitiveOf(t, ob, "attrChoice"); got != "linked" {
			t.Errorf("attrChoice = %q, want linked (explicit attributes present)", got)
		}
	})

	t.Run("no attributes -> attrChoice untouched (auto)", func(t *testing.T) {
		ob := newBuilder()
		ob.SetAttributeObjects("attributes", nil) // early-returns, no change
		if got := primitiveOf(t, ob, "attrChoice"); got != "auto" {
			t.Errorf("attrChoice = %q, want auto (no attributes set)", got)
		}
	})
}
