// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2095 (mendixlabs/mxcli#1796): a flow argument naming an enclosing
// data view that shows a page or snippet parameter (`Item = $dvOuter`) was
// written as PageVariable { Widget: dvOuter } alone, and mx check reported
// CE0115 "The arguments that are passed to microflow 'ModV.ACT_Open' do not
// match the expected parameters and need to be refreshed." Studio Pro wants the
// pair the data view's `$dvOuter.Attr` binding already stores (#826): Widget
// plus the parameter it shows. A data view whose object comes from a microflow
// keeps Widget alone, which passes (variant 8 of the report).
func TestFlowArgNamingParameterDataViewCarriesTheParameter(t *testing.T) {
	for _, tc := range []struct {
		name                string
		snippet             bool
		dataViews           map[string]pages.DataSource
		value               string
		wantVar, wantKind   string
		wantParam, wantSlot string
	}{
		{
			name:      "data view showing a page parameter",
			dataViews: map[string]pages.DataSource{"dvOuter": &pages.DataViewSource{ParameterName: "Item"}},
			value:     "$dvOuter", wantVar: "$dvOuter", wantKind: "widget",
			wantParam: "Item", wantSlot: "",
		},
		{
			name:      "data view showing a snippet parameter",
			snippet:   true,
			dataViews: map[string]pages.DataSource{"dvSnip": &pages.DataViewSource{ParameterName: "Item", IsSnippetParameter: true}},
			value:     "$dvSnip", wantVar: "$dvSnip", wantKind: "widget",
			wantParam: "Item", wantSlot: "snippet",
		},
		{
			// Control (variant 8): no parameter behind the data view, Widget alone.
			name:      "data view whose object comes from a microflow",
			dataViews: map[string]pages.DataSource{"dvMf": &pages.MicroflowSource{}},
			value:     "$dvMf", wantVar: "$dvMf", wantKind: "widget",
		},
		{
			// Control (#1560): a grid's selection stays Widget alone.
			name:  "a grid's selection",
			value: "$grid2", wantVar: "$grid2", wantKind: "widget",
		},
		{
			// Control: the parameter itself is unchanged.
			name:      "the page parameter",
			dataViews: map[string]pages.DataSource{"dvOuter": &pages.DataViewSource{ParameterName: "Item"}},
			value:     "$Item", wantVar: "$Item", wantKind: "parameter",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pb := builderWithParams("Item")
			pb.isSnippet = tc.snippet
			widgets := map[string]bool{"grid2": true}
			for name, ds := range tc.dataViews {
				widgets[name] = true
				defer pb.registerDataViewVariable(name, ds)()
			}
			pb.isDocumentWidget = func(name string) bool { return widgets[name] }

			got := pb.flowArgsToParameterMappings([]ast.FlowArgV3{{Name: "Item", Value: tc.value}})
			if len(got) != 1 {
				t.Fatalf("%d mappings, want 1", len(got))
			}
			m := got[0]
			if m.Variable != tc.wantVar || m.VariableKind != tc.wantKind ||
				m.WidgetParameter != tc.wantParam || m.WidgetParameterKind != tc.wantSlot {
				t.Errorf("mapping = {Variable %q, Kind %q, WidgetParameter %q, WidgetParameterKind %q}, want {%q, %q, %q, %q}",
					m.Variable, m.VariableKind, m.WidgetParameter, m.WidgetParameterKind,
					tc.wantVar, tc.wantKind, tc.wantParam, tc.wantSlot)
			}
		})
	}
}

// The read half: the stored pair describes as the data view, so describe ->
// exec writes the same pair back instead of the parameter alone.
func TestDescribeFlowArgReadsDataViewParameterPairAsTheWidget(t *testing.T) {
	pair := map[string]any{
		"$Type": "Forms$PageVariable", "LocalVariable": "", "PageParameter": "Item",
		"SnippetParameter": "", "SubKey": "", "UseAllPages": false, "Widget": "dvOuter",
	}
	if got := flowArgVariableValue(pair); got != "$dvOuter" {
		t.Errorf("pair described as %q, want $dvOuter", got)
	}
	// Controls: the parameter alone and the widget alone read as before.
	if got := flowArgVariableValue(map[string]any{"PageParameter": "Item", "Widget": ""}); got != "$Item" {
		t.Errorf("page parameter described as %q, want $Item", got)
	}
	if got := flowArgVariableValue(map[string]any{"Widget": "grid2"}); got != "$grid2" {
		t.Errorf("selection described as %q, want $grid2", got)
	}
}
