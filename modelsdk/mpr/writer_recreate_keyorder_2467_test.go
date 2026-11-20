// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The write path stores units in Studio Pro's key order (ako/mxcli#2467). A
// delete+insert that puts back what it removed still writes the file, so on a
// unit an older mxcli stored in its own order it would reorder every key while
// changing nothing. It lands the stored bytes instead; the control is a real
// change, which lands sorted.
func unsortedRestDoc(t *testing.T, name string) []byte {
	t.Helper()
	b, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob("11111111-1111-1111-1111-111111111111")}},
		{Key: "$Type", Value: "Rest$ConsumedRestService"},
		{Key: "Name", Value: name},
		{Key: "BaseUrl", Value: bson.D{
			{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob("33333333-3333-3333-3333-333333333333")}},
			{Key: "$Type", Value: "Rest$ValueTemplate"},
			{Key: "Template", Value: "https://example.test/api"},
		}},
	})
	if err != nil {
		t.Fatalf("marshal unit: %v", err)
	}
	return b
}

func recreateAndRead(t *testing.T, stored, rebuilt []byte) bson.D {
	t.Helper()
	const unitID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	const containerID = "22222222-2222-2222-2222-222222222222"
	w, unitPath := newV2WriterForCommitTest(t, unitID, stored)
	seedTransactionTable(t, w, "the-original-transaction")
	seedUnitRow(t, w, unitID, containerID, "Documents")
	if err := w.deleteUnit(unitID); err != nil {
		t.Fatalf("delete unit: %v", err)
	}
	if err := w.insertUnit(unitID, containerID, "Documents", "Rest$ConsumedRestService", rebuilt); err != nil {
		t.Fatalf("insert unit: %v", err)
	}
	got, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read unit back: %v", err)
	}
	if string(got) == string(stored) {
		return nil
	}
	var doc bson.D
	if err := bson.Unmarshal(got, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestRecreateOfUnsortedUnitKeepsItsBytes_2467(t *testing.T) {
	stored := unsortedRestDoc(t, "Svc")
	if doc := recreateAndRead(t, stored, unsortedRestDoc(t, "Svc")); doc != nil {
		t.Errorf("a delete+insert that changed nothing reordered the stored unit: keys %v", topKeys(doc))
	}
	// Control: a real change lands, in Studio Pro's order.
	doc := recreateAndRead(t, stored, unsortedRestDoc(t, "Renamed"))
	if doc == nil {
		t.Fatal("a delete+insert that renamed the unit did not land")
	}
	if got := topKeys(doc); len(got) != 4 || got[2] != "BaseUrl" || got[3] != "Name" {
		t.Errorf("changed unit keys = %v, want $ID $Type BaseUrl Name", got)
	}
}

func topKeys(d bson.D) []string {
	out := make([]string, len(d))
	for i, e := range d {
		out[i] = e.Key
	}
	return out
}
