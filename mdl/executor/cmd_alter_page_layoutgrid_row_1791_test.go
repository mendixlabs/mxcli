// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func layoutRowNode(name string) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type: "row", Name: name,
		Children: []*ast.WidgetV3{{
			Type: "column",
			Children: []*ast.WidgetV3{{
				Type: "dynamictext", Name: "t1", Properties: map[string]any{"Content": "x"},
			}},
		}},
	}
}

// ako/mxcli#2071 (mendixlabs/mxcli#1791): `alter page M.P { insert into lgMain
// { row { column { … } } } }` failed — first "row needs a name" (the row was
// built as a widget), then, named, "not a simple container" (InsertWidget has
// no Rows list to append to). A row, named or not, must be built as a layout
// grid row and handed to InsertLayoutGridRows.
func TestAlterPageInsertLayoutGridRow_RoutesToRows(t *testing.T) {
	for _, name := range []string{"", "r2"} {
		t.Run("name="+name, func(t *testing.T) {
			var gotGrid string
			var gotRows []*pages.LayoutGridRow
			mutator := &mock.MockPageMutator{
				InsertLayoutGridRowsFunc: func(grid string, rows []*pages.LayoutGridRow) error {
					gotGrid, gotRows = grid, rows
					return nil
				},
				InsertWidgetFunc: func(string, string, backend.InsertPosition, []pages.Widget) error {
					t.Error("a row went through InsertWidget — it would land in a widget list")
					return nil
				},
			}
			err := alterPageWith(t, mutator, &ast.InsertWidgetOp{
				Position: "INTO",
				Target:   ast.WidgetRef{Widget: "lgMain"},
				Widgets:  []*ast.WidgetV3{layoutRowNode(name)},
			})
			if err != nil {
				t.Fatalf("insert into lgMain { row %s }: %v", name, err)
			}
			if gotGrid != "lgMain" || len(gotRows) != 1 {
				t.Fatalf("InsertLayoutGridRows(%q, %d rows), want (lgMain, 1)", gotGrid, len(gotRows))
			}
			row := gotRows[0]
			if row.TypeName != "Forms$LayoutGridRow" || len(row.Columns) != 1 || len(row.Columns[0].Widgets) != 1 {
				t.Errorf("row = %s with %d columns, want a Forms$LayoutGridRow with one column holding the text", row.TypeName, len(row.Columns))
			}
		})
	}
}

// A row has no stored name, so BEFORE/AFTER cannot place one; and a row and a
// widget go to different lists, so one INSERT cannot mean both. Both are
// refused, naming what to write instead, rather than half-applied.
func TestAlterPageInsertLayoutGridRow_Refusals(t *testing.T) {
	text := &ast.WidgetV3{Type: "dynamictext", Name: "t9", Properties: map[string]any{"Content": "y"}}
	for _, tc := range []struct {
		name     string
		position string
		widgets  []*ast.WidgetV3
		want     string
	}{
		{"after a widget", "AFTER", []*ast.WidgetV3{layoutRowNode("")}, "insert into <layoutgrid>"},
		{"rows mixed with a widget", "INTO", []*ast.WidgetV3{layoutRowNode(""), text}, "mixing `row` blocks"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutator := &mock.MockPageMutator{
				InsertLayoutGridRowsFunc: func(string, []*pages.LayoutGridRow) error {
					t.Error("a refused insert reached InsertLayoutGridRows")
					return nil
				},
				InsertWidgetFunc: func(string, string, backend.InsertPosition, []pages.Widget) error {
					t.Error("a refused insert reached InsertWidget")
					return nil
				},
			}
			err := alterPageWith(t, mutator, &ast.InsertWidgetOp{
				Position: tc.position, Target: ast.WidgetRef{Widget: "t0"}, Widgets: tc.widgets,
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}
