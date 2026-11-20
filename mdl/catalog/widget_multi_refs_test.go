// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1513: a widget that names two microflows in its own BSON —
// a Data grid 2 with `datasource: microflow Tmp.Rows` and `onClick: call
// microflow Tmp.Open(…)` — recorded only the lexicographically smallest one.
// The refs projection reads the widget's single MicroflowRef column, so the
// other microflow had no inbound reference: QUAL004 reported the datasource
// microflow "not called from anywhere. Remove if unused", and `show callers`
// found nothing. Same for a gallery or list view with a datasource and a click.
package catalog

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// dataGrid2 is a Data grid 2 as stored: a microflow datasource and an on-click
// microflow, both inside the widget's own Object, plus a column header widget
// (a child, whose refs must stay its own).
func dataGrid2(name, datasourceMF, onClickMF string) map[string]any {
	return map[string]any{
		"$Type": "CustomWidgets$CustomWidget",
		"$ID":   "id-" + name,
		"Name":  name,
		"Type":  map[string]any{"WidgetId": "com.mendix.widget.web.datagrid.Datagrid"},
		"Object": map[string]any{
			"Properties": []any{int32(2),
				map[string]any{"Value": map[string]any{
					"DataSource": map[string]any{
						"$Type":             "Forms$MicroflowSource",
						"MicroflowSettings": map[string]any{"Microflow": datasourceMF},
					},
				}},
				map[string]any{"Value": map[string]any{
					"Action": map[string]any{
						"$Type":             "Forms$MicroflowAction",
						"MicroflowSettings": map[string]any{"Microflow": onClickMF},
					},
				}},
			},
		},
	}
}

func TestScanWidgetAllRefsKeepsEveryMicroflow(t *testing.T) {
	// Open sorts before Rows, so the datasource is the one the old scan dropped.
	w := dataGrid2("dg", "Tmp.Rows", "Tmp.Open")
	got := scanWidgetAllRefs(w)
	if want := []string{"Tmp.Open", "Tmp.Rows"}; !reflect.DeepEqual(got.Microflows, want) {
		t.Errorf("microflows = %v, want %v", got.Microflows, want)
	}
	// The single column keeps its documented choice — the smallest — for
	// `show widgets` and Starlark's microflow_ref.
	if _, mf, _, _, _ := scanWidgetOwnRefs(w); mf != "Tmp.Open" {
		t.Errorf("single MicroflowRef = %q, want Tmp.Open (the smallest)", mf)
	}
	// Duplicates collapse.
	if got := scanWidgetAllRefs(dataGrid2("dg", "Tmp.Same", "Tmp.Same")); !reflect.DeepEqual(got.Microflows, []string{"Tmp.Same"}) {
		t.Errorf("duplicate microflow not collapsed: %v", got.Microflows)
	}
}

// End to end through the builder: buildPages indexes the widget, buildReferences
// projects the refs. Both microflows must be reachable from the page, once each.
func TestWidgetWithDatasourceAndClickRefsBothMicroflows(t *testing.T) {
	const modID = model.ID("mod-tmp")
	pg := &pages.Page{Name: "Overview"}
	pg.ID = "pg-1"
	pg.ContainerID = modID

	raw := map[string]any{
		"$Type": "Forms$Page",
		"FormCall": map[string]any{
			"Arguments": []any{int32(2), map[string]any{
				"Widgets": []any{int32(2),
					dataGrid2("dgItems", "Tmp.Rows", "Tmp.Open"),
					// A second widget whose primary pick is Tmp.Rows: the edge
					// must still be recorded once, not twice.
					dataGrid2("dgOther", "Tmp.Rows", "Tmp.Zeta"),
				},
			}},
		},
	}

	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	tx, err := cat.CatalogDB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	b := &Builder{
		catalog: cat,
		reader: &mock.MockBackend{
			ListPagesFunc:  func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
			GetRawUnitFunc: func(model.ID) (map[string]any, error) { return raw, nil },
			GetNavigationFunc: func() (*types.NavigationDocument, error) {
				return &types.NavigationDocument{}, nil
			},
		},
		snapshot: &Snapshot{ID: "snap-1"},
		hierarchy: &hierarchy{
			moduleIDs:       map[model.ID]bool{modID: true},
			moduleNames:     map[model.ID]string{modID: "Tmp"},
			containerParent: map[model.ID]model.ID{},
			folderNames:     map[model.ID]string{},
		},
		tx:       wrapTx(tx),
		fullMode: true,
	}
	if err := b.buildPages(); err != nil {
		t.Fatalf("buildPages: %v", err)
	}
	if err := b.buildReferences(); err != nil {
		t.Fatalf("buildReferences: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	for _, mf := range []string{"Tmp.Rows", "Tmp.Open", "Tmp.Zeta"} {
		got := refsTo(t, cat, mf)
		want := refRow{"PAGE", "Tmp.Overview", "MICROFLOW", mf, "action"}
		if got[want] != 1 || len(got) != 1 {
			t.Errorf("refs to %s = %v, want exactly one %v", mf, got, want)
		}
	}
}
