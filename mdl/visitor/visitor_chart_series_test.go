// SPDX-License-Identifier: Apache-2.0

// Chart series datasource authoring (bug 9a): a pluggable-widget object-list
// item (chart `series`) must accept a datasource-typed sub-property
// (`staticDataSource: database ...`) and a double-quoted attribute value
// (`staticXAttribute: "X"`). Before the grammar/visitor fix these failed to
// parse ("extraneous input 'from'").
package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestChartSeriesDataSourceParses(t *testing.T) {
	input := `CREATE PAGE M.Dash (Title: 'D') {
		PLUGGABLEWIDGET 'com.mendix.widget.web.columnchart.ColumnChart' chart1 {
			series s1 (
				dataSet: static,
				staticDataSource: database from M.StatusView,
				staticXAttribute: "StatusValue",
				staticYAttribute: "StatusCount",
				staticName: 'Counts'
			)
		}
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("Expected CreatePageStmtV3, got %T", prog.Statements[0])
	}
	chart := findWidgetV3(stmt.Widgets, "chart1")
	if chart == nil {
		t.Fatal("chart1 not found")
	}
	series := findChildByName2(chart.Children, "s1")
	if series == nil {
		t.Fatal("series s1 not found")
	}

	// The datasource sub-property must be captured as a *ast.DataSourceV3, not
	// a dropped/stringified value.
	ds, ok := series.Properties["staticDataSource"].(*ast.DataSourceV3)
	if !ok {
		t.Fatalf("staticDataSource: expected *ast.DataSourceV3, got %T (%v)",
			series.Properties["staticDataSource"], series.Properties["staticDataSource"])
	}
	if ds.Type != "database" || ds.Reference != "M.StatusView" {
		t.Errorf("staticDataSource: got type=%q ref=%q, want database/M.StatusView", ds.Type, ds.Reference)
	}

	// Quoted attribute values must unquote to the bare attribute name.
	if got := series.Properties["staticXAttribute"]; got != "StatusValue" {
		t.Errorf("staticXAttribute: got %q, want StatusValue", got)
	}
	if got := series.Properties["staticYAttribute"]; got != "StatusCount" {
		t.Errorf("staticYAttribute: got %q, want StatusCount", got)
	}
}
