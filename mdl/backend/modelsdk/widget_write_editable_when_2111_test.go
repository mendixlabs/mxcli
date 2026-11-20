// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2111: "Editable: based on attribute value" is written as the
// visibility settings' attribute form is — Attribute, Conditions [2, …] with
// EditableVisible per value — on a Forms$ConditionalEditabilitySettings.
// Before, only Expression was written, so the attribute form had no way out.
func TestConditionalEditabilityToGen_AttributeConditions(t *testing.T) {
	d := encodeToD(t, conditionalEditabilityToGen(&pages.ConditionalEditabilitySettings{
		Attribute: "EdMod.Ticket.IsLocked",
		Conditions: []pages.ValueCondition{
			{Value: "true", Visible: false},
			{Value: "false", Visible: true},
		},
	}))
	var attr any
	var conds bson.A
	for _, e := range d {
		switch e.Key {
		case "Attribute":
			attr = e.Value
		case "Conditions":
			conds, _ = e.Value.(bson.A)
		}
	}
	if attr != "EdMod.Ticket.IsLocked" {
		t.Errorf("Attribute = %v", attr)
	}
	if len(conds) != 3 || conds[0] != int32(2) {
		t.Fatalf("Conditions = %#v, want [2, cond, cond]", conds)
	}
	second, _ := conds[2].(bson.D)
	got := map[string]any{}
	for _, e := range second {
		got[e.Key] = e.Value
	}
	if got["$Type"] != "Enumerations$Condition" || got["AttributeValue"] != "false" || got["EditableVisible"] != true {
		t.Errorf("condition[1] = %v, want Enumerations$Condition false/editable", got)
	}
}
