// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#556, the second named instance. `create or modify rest client` is a
// DELETE followed by an INSERT under the preserved unit ID, so the write never
// reaches updateUnit and therefore never reaches canon.Reconcile. Measured on a
// real 11.14.0 project, two consecutive identical re-runs of one statement:
//
//	Rest$ConsumedRestService unit, 1,128 bytes before and after
//	143 bytes differ, in 9 runs, every one an element $ID:
//	  /BaseUrl/$ID, /Operations/1/$ID, .../Method/$ID, .../Path/$ID,
//	  .../Headers/1/$ID, .../Headers/1/Value/$ID, .../Parameters/1/$ID,
//	  .../Parameters/1/DataType/$ID, .../ResponseHandling/$ID
//
// Nothing about the document changed — only which UUIDs the rebuild happened to
// mint. Feeding those same two units to canon.TransplantIDs makes them
// byte-identical, so the policy was never wrong here; it simply was not called.
//
// This is CLAUDE.md's second rule about elision, in the shape that is easiest to
// miss: the new write path is not a new function, it is an existing insert
// reached by a delete.

// recreatedDoc builds a two-element document whose inner $ID differs per call,
// standing in for a rebuild that mints fresh identities for everything under the
// root. The root $ID is fixed, because a recreate preserves the unit ID.
func recreatedDoc(t *testing.T, name, innerID string) []byte {
	t.Helper()
	b, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob("11111111-1111-1111-1111-111111111111")}},
		{Key: "$Type", Value: "Rest$ConsumedRestService"},
		{Key: "BaseUrl", Value: bson.D{
			{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob(innerID)}},
			{Key: "$Type", Value: "Rest$ValueTemplate"},
			{Key: "Template", Value: "https://example.test/api"},
		}},
		{Key: "Name", Value: name},
	})
	if err != nil {
		t.Fatalf("marshal unit: %v", err)
	}
	return b
}

func TestRecreatedUnitKeepsTheIdentitiesItWasDeletedWith(t *testing.T) {
	const unitID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	const containerID = "22222222-2222-2222-2222-222222222222"

	stored := recreatedDoc(t, "PartsAvailabilityAPI", "33333333-3333-3333-3333-333333333333")
	rebuilt := recreatedDoc(t, "PartsAvailabilityAPI", "44444444-4444-4444-4444-444444444444")

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
	if string(got) != string(stored) {
		t.Errorf("a delete+insert of identical content rewrote %d of %d bytes — the re-inserted "+
			"unit minted fresh element $IDs, so `git status` never comes back clean and an "+
			"MDL-generated project is not reviewable in version control (ako/mxcli#556)",
			differingBytes(got, stored), len(stored))
	}
	// The bytes are only half of it. `_Transaction.LastTransactionID` is how
	// Studio Pro decides an external change needs re-syncing, and both the
	// delete and the insert bump it — so without restoring it the .mpr itself
	// still shows up in `git status` even once every .mxunit is stable. That is
	// exactly where the fix stood when it was measured on a real project: one
	// changed file left, and the only differing row was this one.
	if got := transactionID(t, w); got != "the-original-transaction" {
		t.Errorf("LastTransactionID = %q after a delete+insert that changed nothing — "+
			"the .mpr is dirty although no unit moved", got)
	}
}

// seedUnitRow fills in the ContainerID and ContainmentName the shared harness
// leaves null. They are load-bearing here: a recreate is only a no-op when the
// ROW came back the same too, so a unit that moved folders must still land.
func seedUnitRow(t *testing.T, w *Writer, unitID, containerID, containmentName string) {
	t.Helper()
	if _, err := w.reader.db.Exec(
		`UPDATE Unit SET ContainerID = ?, ContainmentName = ? WHERE UnitID = ?`,
		uuidToBlob(containerID), containmentName, uuidToBlob(unitID),
	); err != nil {
		t.Fatalf("seed unit row: %v", err)
	}
}

func seedTransactionTable(t *testing.T, w *Writer, id string) {
	t.Helper()
	if _, err := w.reader.db.Exec(`CREATE TABLE _Transaction (LastTransactionID TEXT)`); err != nil {
		t.Fatalf("create _Transaction: %v", err)
	}
	if _, err := w.reader.db.Exec(`INSERT INTO _Transaction (LastTransactionID) VALUES (?)`, id); err != nil {
		t.Fatalf("seed _Transaction: %v", err)
	}
}

func transactionID(t *testing.T, w *Writer) string {
	t.Helper()
	var got string
	if err := w.reader.db.QueryRow(`SELECT LastTransactionID FROM _Transaction`).Scan(&got); err != nil {
		t.Fatalf("read LastTransactionID: %v", err)
	}
	return got
}

// CONTROL: a recreate that really did change something still lands. Without
// this, a carry broken into "always write back the deleted bytes" would pass the
// test above and silently discard the user's edit.
func TestRecreatedUnitStillLandsARealChange(t *testing.T) {
	const unitID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	const containerID = "22222222-2222-2222-2222-222222222222"

	stored := recreatedDoc(t, "PartsAvailabilityAPI", "33333333-3333-3333-3333-333333333333")
	renamed := recreatedDoc(t, "PartsAvailabilityAPIv2", "44444444-4444-4444-4444-444444444444")

	w, unitPath := newV2WriterForCommitTest(t, unitID, stored)
	seedTransactionTable(t, w, "the-original-transaction")
	seedUnitRow(t, w, unitID, containerID, "Documents")
	if err := w.deleteUnit(unitID); err != nil {
		t.Fatalf("delete unit: %v", err)
	}
	if err := w.insertUnit(unitID, containerID, "Documents", "Rest$ConsumedRestService", renamed); err != nil {
		t.Fatalf("insert unit: %v", err)
	}

	got, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read unit back: %v", err)
	}
	var doc bson.D
	if err := bson.Unmarshal(got, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, e := range doc {
		if e.Key == "Name" && e.Value != "PartsAvailabilityAPIv2" {
			t.Fatalf("the rename was discarded: Name = %v", e.Value)
		}
	}
	if storedHash(t, w, unitID) != hashOf(got) {
		t.Error("ContentsHash does not describe the bytes on disk")
	}
	if got := transactionID(t, w); got == "the-original-transaction" {
		t.Error("LastTransactionID was restored although the recreate changed the document — " +
			"Studio Pro would not notice the rewrite")
	}
}

// CONTROL: an ordinary insert — no preceding delete — is untouched. The carry
// must key on a unit this session actually removed, not on a unit ID that
// happens to have been seen.
func TestFreshInsertIsNotAffected(t *testing.T) {
	const unitID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	const otherID = "bbbbbbbb-bbbb-cccc-dddd-eeeeeeeeeeee"
	const containerID = "22222222-2222-2222-2222-222222222222"

	stored := recreatedDoc(t, "Existing", "33333333-3333-3333-3333-333333333333")
	fresh := recreatedDoc(t, "Fresh", "44444444-4444-4444-4444-444444444444")

	w, _ := newV2WriterForCommitTest(t, unitID, stored)
	if err := w.insertUnit(otherID, containerID, "Documents", "Rest$ConsumedRestService", fresh); err != nil {
		t.Fatalf("insert unit: %v", err)
	}
	blob := uuidToBlob(otherID)
	swapped := blobToUUIDSwapped(blob)
	got, err := os.ReadFile(w.reader.contentsDir + "/" + swapped[0:2] + "/" + swapped[2:4] + "/" + swapped + ".mxunit")
	if err != nil {
		t.Fatalf("read new unit: %v", err)
	}
	if string(got) != string(fresh) {
		t.Error("a fresh insert was altered")
	}
}

func differingBytes(a, b []byte) int {
	n := 0
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			n++
		}
	}
	return n + abs(len(a)-len(b))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// CONTROL for the row half of the no-op test: identical bytes but a different
// container is a MOVE, and it has to land. `create or modify rest client
// ... in folder X` is exactly that statement.
func TestRecreatedUnitInAnotherContainerStillLands(t *testing.T) {
	const unitID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	const oldContainer = "22222222-2222-2222-2222-222222222222"
	const newContainer = "55555555-5555-5555-5555-555555555555"

	stored := recreatedDoc(t, "PartsAvailabilityAPI", "33333333-3333-3333-3333-333333333333")
	rebuilt := recreatedDoc(t, "PartsAvailabilityAPI", "44444444-4444-4444-4444-444444444444")

	w, _ := newV2WriterForCommitTest(t, unitID, stored)
	seedTransactionTable(t, w, "the-original-transaction")
	seedUnitRow(t, w, unitID, oldContainer, "Documents")

	if err := w.deleteUnit(unitID); err != nil {
		t.Fatalf("delete unit: %v", err)
	}
	if err := w.insertUnit(unitID, newContainer, "Documents", "Rest$ConsumedRestService", rebuilt); err != nil {
		t.Fatalf("insert unit: %v", err)
	}

	var got []byte
	if err := w.reader.db.QueryRow(
		`SELECT ContainerID FROM Unit WHERE UnitID = ?`, uuidToBlob(unitID),
	).Scan(&got); err != nil {
		t.Fatalf("read ContainerID: %v", err)
	}
	if string(got) != string(uuidToBlob(newContainer)) {
		t.Error("the move was discarded")
	}
	if id := transactionID(t, w); id == "the-original-transaction" {
		t.Error("LastTransactionID was restored although the unit moved folders")
	}
}
