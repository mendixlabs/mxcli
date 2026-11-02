// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A write in a WriteTransaction is reconciled against the transaction's view of
// the unit, not the committed one (WriteTransaction.storedBytes). On MPR v2 the
// transaction's own first write sits in a temp file until Commit, and the
// reconcile read went to the committed .mxunit: writing A → B → A elided the
// second write as unchanged, since it matches the committed A, and committed B.

func nameOf(contents []byte) string {
	name, _ := bson.Raw(contents).Lookup("Name").StringValueOK()
	return name
}

func TestV2WriteTransactionReconcilesAgainstItsOwnWrites(t *testing.T) {
	const id = "22222222-3333-4444-5555-666666666666"
	a := unitDoc(t, "A")
	w, path := newV2WriterForCommitTest(t, id, a)

	tx, err := w.BeginWriteTransaction()
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.WriteUnit(id, unitDoc(t, "B")); err != nil {
		t.Fatal(err)
	}
	if err := tx.WriteUnit(id, a); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, a) {
		t.Fatalf("committed unit is %q, want A back: the second write was reconciled against the committed file and elided", nameOf(got))
	}
	if h := storedHash(t, w, id); h != hashOf(a) {
		t.Errorf("ContentsHash does not describe A (B's hash: %v)", h == hashOf(unitDoc(t, "B")))
	}
	if offered, written := w.WriteStats(); offered != 2 || written != 2 {
		t.Errorf("WriteStats = %d offered, %d written; want 2, 2", offered, written)
	}
}

// The other half: a second write equal to the transaction's first is elided,
// and the first is what commits. An over-eager fix (never elide while a write
// is pending) passes A → B → A and fails this.
func TestV2WriteTransactionElidesARepeatOfItsOwnWrite(t *testing.T) {
	const id = "22222222-3333-4444-5555-777777777777"
	w, path := newV2WriterForCommitTest(t, id, unitDoc(t, "A"))
	b := unitDoc(t, "B")

	tx, err := w.BeginWriteTransaction()
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.WriteUnit(id, b); err != nil {
		t.Fatal(err)
	}
	if err := tx.WriteUnit(id, b); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, b) {
		t.Fatalf("committed unit is %q, want B", nameOf(got))
	}
	if offered, written := w.WriteStats(); offered != 2 || written != 1 {
		t.Errorf("WriteStats = %d offered, %d written; want 2, 1: the repeat is no change to what this transaction wrote", offered, written)
	}
}

// v1 gets the transaction's view by reading through wt.tx, which sees its own
// writes. Reading on a second connection would also have ended the v1
// deadlock, and would have had v2's bug.
func TestV1WriteTransactionReconcilesAgainstItsOwnWrites(t *testing.T) {
	id := v1UnitID(1)
	a := v1TestUnit(t, "A")
	w, err := NewWriter(newV1ProjectFile(t, id, a))
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		tx, err := w.BeginWriteTransaction()
		if err == nil {
			err = tx.WriteUnit(id, v1TestUnit(t, "B"))
		}
		if err == nil {
			err = tx.WriteUnit(id, a)
		}
		if err == nil {
			err = tx.Commit()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the transaction did not return within 10s (TestWriteTransactionWriteUnitV1DoesNotDeadlock)")
	}

	got, err := w.reader.GetRawUnitBytes(id)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, a) {
		t.Fatalf("committed unit is %q, want A back", nameOf(got))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}
