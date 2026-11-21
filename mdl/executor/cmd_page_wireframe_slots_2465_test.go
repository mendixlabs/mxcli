// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"testing"
)

// ako/mxcli#2465 (mendixlabs/mxcli#1910): `describe --format elk page` listed
// a DataGrid2's columns as caption and binding only. The widgets in a column's
// custom content and filter, the control bar and the empty-state placeholder,
// which plain describe prints, were missing from the tree (8 of 20 widgets on
// the reported page), so a class audit over the JSON could not see them.

func wireframeNames2465(nodes []wireframeNode, acc []string) []string {
	for _, n := range nodes {
		if n.Name != "" {
			acc = append(acc, n.Name)
		}
		acc = wireframeNames2465(n.Children, acc)
		for _, c := range n.Columns {
			acc = wireframeNames2465(c.Children, acc)
			acc = wireframeNames2465(c.Filter, acc)
		}
		for _, s := range n.Slots {
			acc = wireframeNames2465(s.Children, acc)
		}
		for _, r := range n.Rows {
			for _, c := range r.Columns {
				acc = wireframeNames2465(c.Children, acc)
			}
		}
		for _, t := range n.TabPages {
			acc = wireframeNames2465(t.Children, acc)
		}
	}
	return acc
}

func TestWireframeKeepsDataGrid2Slots_2465(t *testing.T) {
	text := func(name string) rawWidget {
		return rawWidget{Type: "Forms$DynamicText", Name: name, Content: name}
	}
	grid := rawWidget{
		Type: "CustomWidgets$CustomWidget", Name: "dgOrders", RenderMode: "datagrid2",
		DataGridColumns: []rawDataGridColumn{
			{Caption: "Number", Attribute: "Number", FilterWidgets: []rawWidget{
				{Type: "Forms$Container", Name: "cntFilter", Children: []rawWidget{
					{Type: "CustomWidgets$CustomWidget", Name: "tfNumber", RenderMode: "textfilter"},
				}},
			}},
			{Caption: "Status", ShowContentAs: "customContent", ContentWidgets: []rawWidget{text("txtStatus")},
				FilterWidgets: []rawWidget{{Type: "CustomWidgets$CustomWidget", Name: "ddStatus", RenderMode: "dropdownfilter"}}},
		},
		ControlBar: []rawWidget{{Type: "Forms$ActionButton", Name: "btnNew", Caption: "New"}},
		ChildSlots: []rawChildSlot{{Keyword: "emptyplaceholder", PropertyKey: "emptyPlaceholder", Widgets: []rawWidget{text("txtEmpty")}}},
	}
	node := rawWidgetToWireframe(grid, &wireframeCounter{})
	got := wireframeNames2465([]wireframeNode{node}, nil)
	sort.Strings(got)
	want := []string{"btnNew", "cntFilter", "ddStatus", "dgOrders", "tfNumber", "txtEmpty", "txtStatus"}
	if len(got) != len(want) {
		t.Fatalf("widgets in the elk tree = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("widgets in the elk tree = %v, want %v", got, want)
		}
	}
	// The slot is named as describe names it, so the tree says where a widget is.
	if len(node.Slots) != 2 || node.Slots[0].Name != "controlbar" || node.Slots[1].Name != "emptyplaceholder" {
		t.Errorf("slots = %+v, want controlbar then emptyplaceholder", node.Slots)
	}
	// Columns keep their caption and binding, as before.
	if node.Columns[0].Caption != "Number" || node.Columns[0].Binding != "Number" {
		t.Errorf("column = %+v", node.Columns[0])
	}
}
