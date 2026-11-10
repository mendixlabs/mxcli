// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// rowStubDeps serializes a layout grid the way the codec does: a
// Forms$LayoutGrid whose Rows carry a list marker and one doc per row.
type rowStubDeps struct{ stubWidgetDeps }

func (d *rowStubDeps) SerializeWidget(w pages.Widget) bson.D {
	lg, ok := w.(*pages.LayoutGrid)
	if !ok {
		return d.stubWidgetDeps.SerializeWidget(w)
	}
	arr := bson.A{int32(2)}
	for _, r := range lg.Rows {
		arr = append(arr, bson.D{{Key: "$Type", Value: "Forms$LayoutGridRow"}, {Key: "Class", Value: r.Class}})
	}
	return bson.D{{Key: "$Type", Value: "Forms$LayoutGrid"}, {Key: "Rows", Value: arr}}
}

func makeLayoutGridPage(rows ...bson.D) bson.D {
	arr := bson.A{int32(2)}
	for _, r := range rows {
		arr = append(arr, r)
	}
	return makeRawPage(bson.D{
		{Key: "$Type", Value: "Forms$LayoutGrid"},
		{Key: "Name", Value: "lgMain"},
		{Key: "Rows", Value: arr},
	}, bson.D{{Key: "$Type", Value: "Forms$DivContainer"}, {Key: "Name", Value: "div1"}, {Key: "Widgets", Value: bson.A{int32(2)}}})
}

func rowClasses(t *testing.T, m *Mutator) []string {
	t.Helper()
	var out []string
	rows := bsonnav.ToBsonA(bsonnav.DGet(findBsonWidget(m.rawData, "lgMain").widget, "Rows"))
	if len(rows) == 0 || !isListMarker(rows[0]) {
		t.Fatalf("Rows lost its list marker: %v", rows)
	}
	for _, el := range rows[1:] {
		out = append(out, bsonnav.DGetString(el.(bson.D), "Class"))
	}
	return out
}

// mendixlabs/mxcli#1791: rows are appended to the grid's Rows list, after the
// stored ones, keeping its marker.
func TestInsertLayoutGridRows_AppendsAfterStoredRows(t *testing.T) {
	existing := bson.D{{Key: "$Type", Value: "Forms$LayoutGridRow"}, {Key: "Class", Value: "old"}}
	m := New(makeLayoutGridPage(existing), model.ID("page-1"), &rowStubDeps{})
	rows := []*pages.LayoutGridRow{{Class: "new1"}, {Class: "new2"}}
	if err := m.InsertLayoutGridRows("lgMain", rows); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rowClasses(t, m), ","); got != "old,new1,new2" {
		t.Errorf("rows = %s, want old,new1,new2", got)
	}
}

func TestInsertLayoutGridRows_RefusesANonGrid(t *testing.T) {
	m := New(makeLayoutGridPage(), model.ID("page-1"), &rowStubDeps{})
	err := m.InsertLayoutGridRows("div1", []*pages.LayoutGridRow{{}})
	if err == nil || !strings.Contains(err.Error(), "only be added to a layout grid") {
		t.Fatalf("err = %v, want a refusal for a container that is not a layout grid", err)
	}
	if err := m.InsertLayoutGridRows("nope", []*pages.LayoutGridRow{{}}); err == nil {
		t.Fatal("an unknown grid was accepted")
	}
}
