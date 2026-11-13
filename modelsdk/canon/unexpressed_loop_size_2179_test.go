// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2179 (mendixlabs/mxcli#1832): drop + create of a flow kept every
// element's $ID and position but wrote each loop at the size mxcli computes,
// so a loop drawn 700×300 in Studio Pro came back 410×130 and overlapped its
// neighbours or left its own activities outside. MDL has no way to state a
// loop's size, so the stored one is carried for the loop of the same $ID — per
// dimension, never smaller than what was written, so a body that grew still
// gets room.
func loopDoc(size string) bson.D {
	return bson.D{
		{Key: "$ID", Value: id16(0x51)},
		{Key: "$Type", Value: "Microflows$LoopedActivity"},
		{Key: "RelativeMiddlePoint", Value: "400;200"},
		{Key: "Size", Value: size},
	}
}

func carriedLoopSize(t *testing.T, written, stored string) string {
	t.Helper()
	w, _ := bson.Marshal(bson.D{{Key: "Objects", Value: bson.A{int32(3), loopDoc(written)}}})
	s, _ := bson.Marshal(bson.D{{Key: "Objects", Value: bson.A{int32(3), loopDoc(stored)}}})
	var out bson.D
	if err := bson.Unmarshal(CarryUnexpressed(w, s), &out); err != nil {
		t.Fatal(err)
	}
	loop := docLookup(out, "Objects").(bson.A)[1].(bson.D)
	return docLookup(loop, "Size").(string)
}

func TestUnexpressed_LoopKeepsItsDrawnSize(t *testing.T) {
	if got := carriedLoopSize(t, "410;130", "700;300"); got != "700;300" {
		t.Errorf("a loop drawn 700;300 was written %s", got)
	}
	// The body grew past the drawn box in one dimension: that one grows.
	if got := carriedLoopSize(t, "410;420", "700;300"); got != "700;420" {
		t.Errorf("grown body: size %s, want 700;420", got)
	}
	// Control: the computed size already covers the stored one — unchanged.
	if got := carriedLoopSize(t, "800;400", "700;300"); got != "800;400" {
		t.Errorf("control: size %s, want 800;400", got)
	}
}

// Control: an element that is not a loop keeps the size written for it.
func TestUnexpressed_LoopSizeOnlyForLoops(t *testing.T) {
	act := func(size string) bson.D {
		return bson.D{{Key: "$ID", Value: id16(0x52)}, {Key: "$Type", Value: "Microflows$ActionActivity"}, {Key: "Size", Value: size}}
	}
	w, _ := bson.Marshal(bson.D{{Key: "Objects", Value: bson.A{int32(3), act("120;60")}}})
	s, _ := bson.Marshal(bson.D{{Key: "Objects", Value: bson.A{int32(3), act("200;90")}}})
	var out bson.D
	_ = bson.Unmarshal(CarryUnexpressed(w, s), &out)
	if got := docLookup(docLookup(out, "Objects").(bson.A)[1].(bson.D), "Size"); got != "120;60" {
		t.Errorf("an activity's size was carried: %v", got)
	}
}
