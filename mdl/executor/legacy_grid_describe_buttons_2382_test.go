// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// ako/mxcli#2382: describe printed a legacy grid's columns but none of its
// control-bar buttons or search fields: it read ControlBar.Items and
// SearchBar.Items, and both lists are stored as `NewButtons`.
func TestParseLegacyGrid_StoredNewButtons_Issue2382(t *testing.T) {
	ctx, _ := newMockCtx(t)
	text := func(s string) map[string]any {
		return map[string]any{"$Type": "Texts$Text", "Items": []any{int32(3),
			map[string]any{"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": s}}}
	}
	grid := map[string]any{
		"$Type": "Forms$DataGrid",
		"ControlBar": map[string]any{"NewButtons": []any{int32(2),
			map[string]any{"$Type": "Forms$GridActionButton", "Name": "actionButton4",
				"CaptionTemplate": map[string]any{"$Type": "Forms$ClientTemplate", "Template": text("Edit")}},
		}},
		"DataSource": map[string]any{"SearchBar": map[string]any{"NewButtons": []any{int32(2),
			map[string]any{"$Type": "Forms$ComparisonSearchField", "Name": "searchField1", "Caption": text("Alias")},
		}}},
	}
	g := parseLegacyGrid(ctx, grid)
	if len(g.ControlBar) != 1 || g.ControlBar[0].Name != "actionButton4" || g.ControlBar[0].Caption != "Edit" {
		t.Errorf("control bar = %+v, want actionButton4 'Edit'", g.ControlBar)
	}
	if len(g.SearchFields) != 1 || g.SearchFields[0].Name != "searchField1" {
		t.Errorf("search fields = %+v, want searchField1", g.SearchFields)
	}
}
