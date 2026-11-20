// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// mendixlabs/mxcli#1819: a layout's top-level `Excluded: true` was never read,
// so CATALOG.layouts could not report it. Read the stored flag back, with the
// as-written false as the control.
func TestListLayouts_ReadsExcluded(t *testing.T) {
	b, l := layoutCanvasFixture(t)

	find := func() bool {
		t.Helper()
		ls, err := b.ListLayouts()
		if err != nil {
			t.Fatalf("ListLayouts: %v", err)
		}
		for _, x := range ls {
			if x.ID == l.ID {
				return x.Excluded
			}
		}
		t.Fatalf("layout %s not listed", l.Name)
		return false
	}
	if find() {
		t.Fatal("control: a freshly written layout reads as excluded")
	}

	raw, err := b.reader.GetRawUnitBytes(string(l.ID))
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	bsonnav.DSet(d, "Excluded", true)
	out, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.writer.UpdateRawUnit(string(l.ID), out); err != nil {
		t.Fatal(err)
	}
	if !find() {
		t.Error("a layout stored with Excluded: true reads as not excluded")
	}
}
