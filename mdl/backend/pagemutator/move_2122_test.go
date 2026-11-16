// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2122 (mendixlabs/mxcli#1807): ALTER PAGE had INSERT, REPLACE and
// DROP but no way to move an existing widget, so a legacy classic Data grid —
// which MDL cannot re-author — could not be wrapped in a data view after an
// upgrade. MoveWidget moves the stored subtree as it is.
func movePage() bson.D {
	grid := bson.D{{Key: "$Type", Value: "Forms$DataGrid"}, {Key: "Name", Value: "dgLegacy"},
		{Key: "Marker", Value: "stored bytes"}}
	return makeRawPage(
		bson.D{{Key: "$Type", Value: "Forms$DivContainer"}, {Key: "Name", Value: "ctnMain"},
			{Key: "Widgets", Value: bson.A{int32(2), button("btnA"), grid}}},
		bson.D{{Key: "$Type", Value: "Forms$DataView"}, {Key: "Name", Value: "dvOrder"}},
	)
}

func childNames(t *testing.T, m *Mutator, container string) []string {
	t.Helper()
	r := findBsonWidget(m.rawData, container)
	if r == nil {
		t.Fatalf("%s not found", container)
	}
	var names []string
	for _, el := range bsonnav.DGetArrayElements(bsonnav.DGet(r.widget, "Widgets")) {
		if d, ok := el.(bson.D); ok {
			names = append(names, bsonnav.DGetString(d, "Name"))
		}
	}
	return names
}

func TestMoveWidget_IntoAContainerKeepsItsBytes_Issue2122(t *testing.T) {
	m := New(movePage(), model.ID("page-1"), &stubWidgetDeps{})
	if err := m.MoveWidget("dgLegacy", "dvOrder", backend.InsertPosition("INTO")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(childNames(t, m, "dvOrder"), ","); got != "dgLegacy" {
		t.Errorf("dvOrder holds %q, want dgLegacy", got)
	}
	if got := strings.Join(childNames(t, m, "ctnMain"), ","); got != "btnA" {
		t.Errorf("ctnMain still holds %q, want only btnA", got)
	}
	g := findBsonWidget(m.rawData, "dgLegacy")
	if g == nil || bsonnav.DGetString(g.widget, "Marker") != "stored bytes" {
		t.Error("the moved grid lost its stored content")
	}
}

func TestMoveWidget_BeforeAndAfter_Issue2122(t *testing.T) {
	m := New(movePage(), model.ID("page-1"), &stubWidgetDeps{})
	if err := m.MoveWidget("btnA", "dgLegacy", backend.InsertPosition("AFTER")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(childNames(t, m, "ctnMain"), ","); got != "dgLegacy,btnA" {
		t.Errorf("after: %q, want dgLegacy,btnA", got)
	}
	if err := m.MoveWidget("btnA", "dgLegacy", backend.InsertPosition("BEFORE")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(childNames(t, m, "ctnMain"), ","); got != "btnA,dgLegacy" {
		t.Errorf("before: %q, want btnA,dgLegacy", got)
	}
}

// Refusals leave the tree as it was.
func TestMoveWidget_Refusals_Issue2122(t *testing.T) {
	for _, tc := range []struct{ src, dest, pos, want string }{
		{"ctnMain", "dgLegacy", "INTO", "inside it"},
		{"ctnMain", "ctnMain", "AFTER", "inside it"},
		{"btnA", "dgLegacy", "INTO", "not a container"},
		{"nosuch", "dvOrder", "INTO", "not found"},
	} {
		m := New(movePage(), model.ID("page-1"), &stubWidgetDeps{})
		err := m.MoveWidget(tc.src, tc.dest, backend.InsertPosition(tc.pos))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("move %s %s %s: err %v, want %q", tc.src, tc.pos, tc.dest, err, tc.want)
		}
		if got := strings.Join(childNames(t, m, "ctnMain"), ","); got != "btnA,dgLegacy" {
			t.Errorf("move %s %s %s changed the tree: ctnMain = %q", tc.src, tc.pos, tc.dest, got)
		}
	}
}
