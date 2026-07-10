// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestSeriesDataSourceMatchesMode(t *testing.T) {
	for _, c := range []struct {
		key     string
		dataSet string
		want    bool
	}{
		{"staticDataSource", "static", true},
		{"staticDataSource", "dynamic", false},
		{"dynamicDataSource", "dynamic", true},
		{"dynamicDataSource", "static", false},
		{"staticDataSource", "", true}, // empty dataSet == static
	} {
		if got := seriesDataSourceMatchesMode(c.key, c.dataSet); got != c.want {
			t.Errorf("seriesDataSourceMatchesMode(%q, %q) = %v, want %v", c.key, c.dataSet, got, c.want)
		}
	}
}

func TestIsChartSeriesContainer(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"SERIES", true},
		{"series", true},
		{"LINE", true},
		{"GROUP", false},   // Accordion
		{"COLUMNS", false}, // DataGrid
		{"MARKER", false},  // Maps
		{"", false},
	} {
		if got := isChartSeriesContainer(c.in); got != c.want {
			t.Errorf("isChartSeriesContainer(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestChartSeriesTextTemplateVisible encodes the widget.xml/editorConfig rule a
// chart series follows: a texttemplate sub-property is visible (→ empty
// ClientTemplate, not null, else CE0463) when its dataSet mode matches AND, for
// a datasource-bound property, that datasource is configured. Verified against
// mx check on Mendix 11.6.6 (bug 9a: vA has no datasource → only staticName
// visible; vB configures staticDataSource → staticName + staticTooltipHoverText).
func TestChartSeriesTextTemplateVisible(t *testing.T) {
	// A stand-in configured datasource value (only presence of the key matters).
	configured := func(keys ...string) map[string]pages.DataSource {
		m := map[string]pages.DataSource{}
		for _, k := range keys {
			m[k] = &pages.DatabaseSource{}
		}
		return m
	}
	tt := func(key, ds string) ItemPropertyMapping {
		return ItemPropertyMapping{PropertyKey: key, Operation: "texttemplate", DataSource: ds}
	}

	cases := []struct {
		name    string
		ip      ItemPropertyMapping
		dataSet string
		cfg     map[string]pages.DataSource
		want    bool
	}{
		// vA: dataSet=static, no datasource configured.
		{"vA staticName visible", tt("staticName", ""), "static", nil, true},
		{"vA staticTooltip hidden (ds not configured)", tt("staticTooltipHoverText", "staticDataSource"), "static", nil, false},
		{"vA dynamicName hidden (wrong mode)", tt("dynamicName", "dynamicDataSource"), "static", nil, false},
		{"vA dynamicTooltip hidden (wrong mode)", tt("dynamicTooltipHoverText", "dynamicDataSource"), "static", nil, false},
		// vB: dataSet=static, staticDataSource configured.
		{"vB staticName visible", tt("staticName", ""), "static", configured("staticDataSource"), true},
		{"vB staticTooltip visible (ds configured)", tt("staticTooltipHoverText", "staticDataSource"), "static", configured("staticDataSource"), true},
		{"vB dynamicName still hidden", tt("dynamicName", "dynamicDataSource"), "static", configured("staticDataSource"), false},
		// dynamic mode: static* hidden, dynamic* subject to their datasource.
		{"dyn staticName hidden", tt("staticName", ""), "dynamic", nil, false},
		{"dyn dynamicName visible when dynamicDataSource configured", tt("dynamicName", "dynamicDataSource"), "dynamic", configured("dynamicDataSource"), true},
		{"dyn dynamicName hidden when dynamicDataSource missing", tt("dynamicName", "dynamicDataSource"), "dynamic", nil, false},
		// default (empty) dataSet behaves as static.
		{"default dataSet treated as static", tt("staticName", ""), "", nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := chartSeriesTextTemplateVisible(c.ip, c.dataSet, c.cfg); got != c.want {
				t.Errorf("chartSeriesTextTemplateVisible(%s, dataSet=%q) = %v, want %v",
					c.ip.PropertyKey, c.dataSet, got, c.want)
			}
		})
	}
}
