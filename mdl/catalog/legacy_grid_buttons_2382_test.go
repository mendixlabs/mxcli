// SPDX-License-Identifier: Apache-2.0

package catalog

import "testing"

// ako/mxcli#2382: a legacy grid's control-bar buttons are stored under
// ControlBar.NewButtons; `show widgets` listed none of them, so a tool could
// not find the button to re-point after a microflow was renamed.
func TestWalkWidget_LegacyGridControlBarButtons_Issue2382(t *testing.T) {
	grid := map[string]any{
		"$Type": "Forms$DataGrid",
		"Name":  "grid1",
		"ControlBar": map[string]any{
			"$Type": "Forms$GridControlBar",
			"NewButtons": []any{int32(2),
				map[string]any{"$Type": "Forms$GridActionButton", "Name": "actionButton4"},
			},
		},
	}
	got := map[string]string{}
	for _, w := range walkWidget(grid, "", 0) {
		got[w.Name] = w.WidgetType
	}
	if got["actionButton4"] != "Forms$GridActionButton" {
		t.Errorf("widgets = %v, want actionButton4 listed", got)
	}
	if got["grid1"] != "Forms$DataGrid" {
		t.Errorf("widgets = %v, want grid1 listed", got)
	}
}
