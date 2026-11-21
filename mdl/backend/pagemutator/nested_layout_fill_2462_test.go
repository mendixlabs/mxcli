// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// mendixlabs/mxcli#1907 (ako/mxcli#2462): a layout built on another layout
// keeps its widgets in the placeholder fills of a layout call,
// Content.LayoutCall.Arguments[].Widgets (as stored by exec, matching a page's
// FormCall.Arguments). describe printed them, and `alter layout … set … on
// actionButton1` refused "widget not found": the layout finder read
// Content.Widgets only.
func nestedLayoutDoc() bson.D {
	return bson.D{
		{Key: "$Type", Value: "Forms$Layout"},
		{Key: "Name", Value: "ProbeLayout"},
		{Key: "Content", Value: bson.D{
			{Key: "$Type", Value: "Forms$WebLayoutContent"},
			{Key: "LayoutCall", Value: bson.D{
				{Key: "$Type", Value: "Forms$LayoutCall"},
				{Key: "Arguments", Value: bson.A{int32(2), bson.D{
					{Key: "$Type", Value: "Forms$FormCallArgument"},
					{Key: "Parameter", Value: "Atlas_Core.Atlas_Default.Main"},
					{Key: "Widgets", Value: bson.A{int32(2),
						bson.D{
							{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: []byte("0123456789abcdef")}},
							{Key: "$Type", Value: "Forms$ActionButton"},
							{Key: "Name", Value: "actionButton1"},
							{Key: "Tooltip", Value: bson.D{{Key: "$Type", Value: "Texts$Text"}, {Key: "Items", Value: bson.A{int32(3)}}}},
						},
						bson.D{{Key: "$Type", Value: "Forms$Placeholder"}, {Key: "Name", Value: "Main"}},
					}},
				}}},
			}},
		}},
	}
}

func TestNestedLayoutFillIsAddressable_2462(t *testing.T) {
	m := New(nestedLayoutDoc(), model.ID("layout-2462"), &stubWidgetDeps{})
	if !m.FindWidget("actionButton1") {
		t.Fatal("widget in a nested layout's placeholder fill not found")
	}
	if err := m.SetWidgetProperty("actionButton1", "Tooltip", "Go on"); err != nil {
		t.Errorf("set Tooltip on the fill's widget: %v", err)
	}
	if _, ok := m.WidgetScope()["actionButton1"]; !ok {
		t.Error("the fill's widget is not in the layout's widget scope")
	}
	// Control: a name that is nowhere is still not found.
	if m.FindWidget("noSuchWidget") {
		t.Error("an absent widget was found")
	}
}
