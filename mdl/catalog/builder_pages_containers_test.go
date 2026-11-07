// SPDX-License-Identifier: Apache-2.0

package catalog

import "testing"

// ako/mxcli#1895: the widget walk descends only into the container keys it
// names, so a widget under any other key was absent from the widgets table
// while the XPath walk — which descends into everything — found it. On
// FactoryManagement every widget inside a legacy Forms$Table (16 of them,
// including the MxModelReflection admin pages) was missing: the table had one
// row and nothing below it, and 5 page XPath rows named a widget with no row.
//
// The keys are the stored ones (modelsdk/gen/pages): a container part holds a
// widget list under Widgets, or a single Widget in older models.

// A table cell is not a widget: a widget in a cell is the table's child.
func TestWidgetTree_TableCells(t *testing.T) {
	page := wid("tbl", "table1", "Forms$Table", map[string]any{
		"Cells": bsonList(
			map[string]any{
				"$ID": "c1", "$Type": "Forms$TableCell",
				"Widgets": bsonList(wid("lv", "listView1", "Forms$ListView", map[string]any{
					"DataSource": map[string]any{
						"$ID": "ds", "$Type": "Forms$ListViewXPathSource",
						"XPathConstraint": "[Name != empty]",
					},
				})),
			},
			map[string]any{
				"$ID": "c2", "$Type": "Forms$TableCell",
				"Widget": wid("lbl", "label1", "Forms$Label", nil),
			},
		),
	})
	assertTree(t, extractWidgetsRecursive(page), map[string]tp{
		"table1":    {"", 0},
		"listView1": {"table1", 1},
		"label1":    {"table1", 1},
	})
}

// A scroll container's regions are not widgets either.
func TestWidgetTree_ScrollContainerRegions(t *testing.T) {
	region := func(id string, w map[string]any) map[string]any {
		return map[string]any{"$ID": id, "$Type": "Forms$ScrollContainerRegion", "Widgets": bsonList(w)}
	}
	page := wid("sc", "scrollContainer1", "Forms$ScrollContainer", map[string]any{
		"CenterRegion": region("rc", wid("dv", "dataView1", "Forms$DataView", nil)),
		"Top":          region("rt", wid("l1", "labelTop", "Forms$Label", nil)),
		"Left":         region("rl", wid("l2", "labelLeft", "Forms$Label", nil)),
		"Right":        region("rr", wid("l3", "labelRight", "Forms$Label", nil)),
		"Bottom": map[string]any{"$ID": "rb", "$Type": "Forms$ScrollContainerRegion",
			"Widget": wid("l4", "labelBottom", "Forms$Label", nil)},
	})
	assertTree(t, extractWidgetsRecursive(page), map[string]tp{
		"scrollContainer1": {"", 0},
		"dataView1":        {"scrollContainer1", 1},
		"labelTop":         {"scrollContainer1", 1},
		"labelLeft":        {"scrollContainer1", 1},
		"labelRight":       {"scrollContainer1", 1},
		"labelBottom":      {"scrollContainer1", 1},
	})
}

// A header and a split pane hold their widgets directly, as a list or a single
// widget per side.
func TestWidgetTree_HeaderAndSplitPane(t *testing.T) {
	page := wid("sp", "splitPane1", "Forms$SplitPane", map[string]any{
		"FirstWidgets": bsonList(wid("hd", "header1", "Forms$Header", map[string]any{
			"LeftWidgets": bsonList(wid("hl", "labelLeft", "Forms$Label", nil)),
			"RightWidget": wid("hr", "labelRight", "Forms$Label", nil),
		})),
		"SecondWidget": wid("ab", "actionButton1", "Forms$ActionButton", nil),
	})
	assertTree(t, extractWidgetsRecursive(page), map[string]tp{
		"splitPane1":    {"", 0},
		"header1":       {"splitPane1", 1},
		"labelLeft":     {"header1", 2},
		"labelRight":    {"header1", 2},
		"actionButton1": {"splitPane1", 1},
	})
}
