// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#872: a run of writes that undo each other — a revoke that removes an
// element and a grant that builds it again — is judged by where it ends. Written
// one by one, the second write was reconciled against a unit that no longer had
// the element, so it landed with a freshly minted $ID and moved the transaction
// id although the unit ended as it began.

const deferredUnitID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

// ruleDoc stands in for a domain model holding one access rule; withRule=false
// is the same unit after the rule was revoked.
func ruleDoc(t *testing.T, withRule bool, ruleID, template string) []byte {
	t.Helper()
	doc := bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob("11111111-1111-1111-1111-111111111111")}},
		{Key: "$Type", Value: "Rest$ConsumedRestService"},
	}
	// In Studio Pro's key order (BaseUrl before Name), which is what the write
	// path stores (ako/mxcli#2467), so the bytes read back compare equal.
	if withRule {
		doc = append(doc, bson.E{Key: "BaseUrl", Value: bson.D{
			{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob(ruleID)}},
			{Key: "$Type", Value: "Rest$ValueTemplate"},
			{Key: "Template", Value: template},
		}})
	}
	doc = append(doc, bson.E{Key: "Name", Value: "Svc"})
	b, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestDeferredRunThatNetsToNothingWritesNothing(t *testing.T) {
	stored := ruleDoc(t, true, "33333333-3333-3333-3333-333333333333", "https://a.test")
	w, unitPath := newV2WriterForCommitTest(t, deferredUnitID, stored)
	seedTransactionTable(t, w, "before-the-run")
	seedUnitRow(t, w, deferredUnitID, "22222222-2222-2222-2222-222222222222", "Documents")

	w.DeferUnitWrites()
	revoked := ruleDoc(t, false, "", "")
	if err := w.UpdateRawUnit(deferredUnitID, revoked); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	// Every read in the run sees the run's own write: the direct read and the
	// listing the executor's GetDomainModel goes through.
	if got, _ := w.reader.GetRawUnitBytes(deferredUnitID); string(got) != string(revoked) {
		t.Error("GetRawUnitBytes does not see the held write")
	}
	refs, err := w.reader.ListUnitsByType("Rest$ConsumedRestService")
	if err != nil || len(refs) != 1 || string(refs[0].Contents) != string(revoked) {
		t.Errorf("the unit listing does not see the held write (err %v, %d units)", err, len(refs))
	}
	if onDisk, _ := os.ReadFile(unitPath); string(onDisk) != string(stored) {
		t.Error("a held write reached disk before the run ended")
	}

	regranted := ruleDoc(t, true, "44444444-4444-4444-4444-444444444444", "https://a.test")
	if err := w.UpdateRawUnit(deferredUnitID, regranted); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := w.FlushDeferredWrites(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if onDisk, _ := os.ReadFile(unitPath); string(onDisk) != string(stored) {
		t.Errorf("a run that ended where it began rewrote %d bytes", differingBytes(onDisk, stored))
	}
	if got := transactionID(t, w); got != "before-the-run" {
		t.Errorf("LastTransactionID = %q after a run that changed nothing", got)
	}
	if _, written := w.WriteStats(); written != 0 {
		t.Errorf("WriteStats counts %d landed writes for a run that changed nothing", written)
	}
	if _, held := w.reader.overlaid(deferredUnitID); held {
		t.Error("the flush left the held bytes in the reader's overlay")
	}
}

// CONTROL: a run that ends somewhere else is written — once — and keeps the
// stored identity of the element it rebuilt.
func TestDeferredRunThatChangesSomethingLandsWithStoredIdentity(t *testing.T) {
	stored := ruleDoc(t, true, "33333333-3333-3333-3333-333333333333", "https://a.test")
	w, unitPath := newV2WriterForCommitTest(t, deferredUnitID, stored)
	seedTransactionTable(t, w, "before-the-run")

	w.DeferUnitWrites()
	if err := w.UpdateRawUnit(deferredUnitID, ruleDoc(t, false, "", "")); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := w.UpdateRawUnit(deferredUnitID, ruleDoc(t, true, "44444444-4444-4444-4444-444444444444", "https://b.test")); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := w.FlushDeferredWrites(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	want := ruleDoc(t, true, "33333333-3333-3333-3333-333333333333", "https://b.test")
	if onDisk, _ := os.ReadFile(unitPath); string(onDisk) != string(want) {
		t.Errorf("want the new template under the stored $ID; %d bytes differ", differingBytes(onDisk, want))
	}
	if got := transactionID(t, w); got == "before-the-run" {
		t.Error("a run that changed the unit did not move the transaction id")
	}
	if _, written := w.WriteStats(); written != 1 {
		t.Errorf("WriteStats counts %d landed writes, want 1 for the whole run", written)
	}
}

// A unit deleted during the run takes its held write with it.
func TestDeferredWriteIsDroppedWithItsUnit(t *testing.T) {
	stored := ruleDoc(t, true, "33333333-3333-3333-3333-333333333333", "https://a.test")
	w, unitPath := newV2WriterForCommitTest(t, deferredUnitID, stored)
	seedTransactionTable(t, w, "before-the-run")

	w.DeferUnitWrites()
	if err := w.UpdateRawUnit(deferredUnitID, ruleDoc(t, false, "", "")); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := w.deleteUnit(deferredUnitID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := w.FlushDeferredWrites(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Errorf("the flush wrote back a deleted unit (stat err %v)", err)
	}
}

// The v1 listing reads contents inline from the Unit table, not through
// readMprContents, so it needs the overlay check of its own. Without it a v1
// project's GetDomainModel reads what was on disk before the run while
// GetRawUnitBytes reads the run's writes — two views of one unit in one run.
func TestDeferredWriteIsSeenByTheV1Listing(t *testing.T) {
	stored := ruleDoc(t, true, "33333333-3333-3333-3333-333333333333", "https://a.test")
	r := newTestReaderV1WithUnit(t, deferredUnitID, stored)
	if _, err := r.db.Exec(`UPDATE Unit SET ContainerID = ?, ContainmentName = 'Documents' WHERE UnitID = ?`,
		uuidToBlob("22222222-2222-2222-2222-222222222222"), uuidToBlob(deferredUnitID)); err != nil {
		t.Fatalf("seed unit row: %v", err)
	}

	held := ruleDoc(t, false, "", "")
	r.SetOverlay(deferredUnitID, held)
	refs, err := r.ListUnitsByType("Rest$ConsumedRestService")
	if err != nil || len(refs) != 1 {
		t.Fatalf("listing: err %v, %d units", err, len(refs))
	}
	if string(refs[0].Contents) != string(held) {
		t.Error("the v1 unit listing does not see the held write")
	}

	// Control: with the overlay cleared the listing reads the stored row again.
	r.ClearOverlay(deferredUnitID)
	refs, err = r.ListUnitsByType("Rest$ConsumedRestService")
	if err != nil || len(refs) != 1 || string(refs[0].Contents) != string(stored) {
		t.Errorf("after ClearOverlay the v1 listing does not read the stored row (err %v)", err)
	}
}
