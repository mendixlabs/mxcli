// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"errors"
	"os"
	"testing"
)

// A merge engine that stops on a decision (`mx merge` exit 2) leaves its
// marker in the unit's index row: ContentsConflicts holds a JSON list instead
// of an empty string, and `mx check` reports CE0137 until it is gone.
const engineMarker = `{"Conflicts":[{"ID":"00000000-0000-0000-0000-000000000000","HeadModification":"Change","MineModification":"Change"}]}`

func setMarker(t *testing.T, w *Writer, unitID, contents string, tree int) {
	t.Helper()
	if _, err := w.reader.db.Exec(`UPDATE Unit SET ContentsConflicts = ?, TreeConflict = ? WHERE UnitID = ?`,
		contents, tree, uuidToBlob(unitID)); err != nil {
		t.Fatal(err)
	}
}

func TestUnitConflictsCleanControl(t *testing.T) {
	w, _ := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	setMarker(t, w, hashTestUnit, "", 0) // the value every unmarked row holds
	got, err := w.ConcreteReader().UnitConflicts()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("an unmarked project lists conflicts: %+v", got)
	}
}

func TestUnitConflictsListsContentsAndTreeMarkers(t *testing.T) {
	w, _ := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	setMarker(t, w, hashTestUnit, engineMarker, 0)
	got, err := w.ConcreteReader().UnitConflicts()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].UnitID != hashTestUnit || got[0].Contents != engineMarker {
		t.Fatalf("contents marker not listed: %+v", got)
	}
	setMarker(t, w, hashTestUnit, "", 1)
	got, err = w.ConcreteReader().UnitConflicts()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Tree != 1 {
		t.Fatalf("tree marker not listed: %+v", got)
	}
}

func TestClearContentsConflictsLeavesTheRowLikeAnUnmarkedOne(t *testing.T) {
	w, _ := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	hash := storedHash(t, w, hashTestUnit)
	setMarker(t, w, hashTestUnit, engineMarker, 0)
	n, err := w.ClearContentsConflicts([]string{hashTestUnit})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("cleared %d row(s), want 1", n)
	}
	var contents string
	if err := w.reader.db.QueryRow(`SELECT ContentsConflicts FROM Unit WHERE UnitID = ?`, uuidToBlob(hashTestUnit)).Scan(&contents); err != nil {
		t.Fatal(err)
	}
	if contents != "" {
		t.Fatalf("ContentsConflicts = %q, want '' (what the engine writes on every unmarked row)", contents)
	}
	if got := storedHash(t, w, hashTestUnit); got != hash {
		t.Fatal("clearing the marker touched ContentsHash")
	}
	if got, _ := w.ConcreteReader().UnitConflicts(); len(got) != 0 {
		t.Fatalf("still listed after clearing: %+v", got)
	}
}

// A tree conflict (deleted on one side, changed on the other) is a different
// decision — which documents exist — and is never cleared by this.
func TestClearContentsConflictsLeavesTreeConflicts(t *testing.T) {
	w, _ := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	setMarker(t, w, hashTestUnit, engineMarker, 1)
	if _, err := w.ClearContentsConflicts([]string{hashTestUnit}); err != nil {
		t.Fatal(err)
	}
	got, _ := w.ConcreteReader().UnitConflicts()
	if len(got) != 1 || got[0].Tree != 1 {
		t.Fatalf("tree conflict cleared or lost: %+v", got)
	}
}

func TestClearContentsConflictsRefusedWhileStudioProOpen(t *testing.T) {
	w, _ := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	setMarker(t, w, hashTestUnit, engineMarker, 0)
	if err := os.WriteFile(w.ConcreteReader().Path()+".lock", nil, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := w.ClearContentsConflicts([]string{hashTestUnit})
	var open *StudioProOpenError
	if !errors.As(err, &open) {
		t.Fatalf("want StudioProOpenError, got %v", err)
	}
	if got, _ := w.ConcreteReader().UnitConflicts(); len(got) != 1 {
		t.Fatal("marker cleared despite the guard")
	}
}
