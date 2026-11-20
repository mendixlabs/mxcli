// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// ako/mxcli#1302 catalogues a layout's widgets (ContainerType LAYOUT) so lint
// rules can read them. UPDATE WIDGETS selects its targets from the same table;
// a read-side change must not widen what it writes, so a layout widget
// matching the filter is left alone.
func TestUpdateWidgetsLeavesLayoutWidgetsAlone_Issue1302(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	for _, q := range []string{
		`INSERT INTO widgets_data (Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType, ModuleName)
			VALUES ('w1', 'ctnPage', 'Forms$DivContainer', 'pg', 'M.Page', 'PAGE', 'M'),
			       ('w2', 'ctnSnippet', 'Forms$DivContainer', 'sn', 'M.Snip', 'SNIPPET', 'M'),
			       ('w3', 'ctnLayout', 'Forms$DivContainer', 'lay', 'M.Layout', 'LAYOUT', 'M')`,
	} {
		if _, err := cat.CatalogDB().Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	got, err := findMatchingWidgets(&ExecContext{Catalog: cat}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, w := range got {
		names[w.Name] = true
	}
	if !names["ctnPage"] || !names["ctnSnippet"] {
		t.Errorf("page and snippet widgets must still match, got %v", names)
	}
	if names["ctnLayout"] {
		t.Errorf("a layout widget is a target of UPDATE WIDGETS: %v", names)
	}
}
