// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	_ "modernc.org/sqlite"
)

// upstream #954. WriteTransaction.Commit commits the SQLite transaction first
// and then renames the .mxunit temp files into place, warning and continuing
// when a rename fails. The Unit row (ContentsHash included) then describes
// contents that are not on disk, and Commit's nil return tells every caller —
// codec.Store.SaveUnit and FlushUnits, and through them the whole modelsdk
// engine — that the write landed.
//
// The reporter's trigger is a Windows file lock: a .mxunit held open without
// FILE_SHARE_DELETE (an editor, an indexer, a sync client) makes os.Rename onto
// it fail with "Access is denied". These tests reproduce the same branch
// portably by removing the temp file between WriteUnit and Commit — what fails
// the rename is not the point, only that it can.

// newV2WriterForCommitTest builds a minimal MPR v2 writer over a temp SQLite DB
// and mprcontents folder, seeded with one unit holding storedBSON.
// Returns the writer and the unit's on-disk path.
func newV2WriterForCommitTest(t *testing.T, unitID string, storedBSON []byte) (*Writer, string) {
	t.Helper()

	root := t.TempDir()
	dbPath := filepath.Join(root, "app.mpr")
	contentsDir := filepath.Join(root, "mprcontents")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE Unit (
			UnitID BLOB PRIMARY KEY NOT NULL,
			ContainerID BLOB,
			ContainmentName TEXT,
			TreeConflict LONG,
			ContentsHash TEXT,
			ContentsConflicts TEXT
		)
	`); err != nil {
		t.Fatalf("create Unit table: %v", err)
	}

	blob := uuidToBlob(unitID)
	swapped := blobToUUIDSwapped(blob)
	unitPath := filepath.Join(contentsDir, swapped[0:2], swapped[2:4], swapped+".mxunit")
	if err := os.MkdirAll(filepath.Dir(unitPath), 0755); err != nil {
		t.Fatalf("mkdir unit dir: %v", err)
	}
	if err := os.WriteFile(unitPath, storedBSON, 0644); err != nil {
		t.Fatalf("seed unit file: %v", err)
	}

	if _, err := db.Exec(
		`INSERT INTO Unit (UnitID, ContentsHash) VALUES (?, ?)`,
		blob, hashOf(storedBSON),
	); err != nil {
		t.Fatalf("insert unit row: %v", err)
	}

	r := &Reader{
		path:        dbPath,
		db:          db,
		version:     MPRVersionV2,
		contentsDir: contentsDir,
	}
	return NewWriterWithReader(r), unitPath
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// unitDoc builds a BSON document that canon.Reconcile can walk, so the write
// under test goes through the same elision path a real unit does.
func unitDoc(t *testing.T, name string) []byte {
	t.Helper()
	b, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob("11111111-1111-1111-1111-111111111111")}},
		{Key: "$Type", Value: "Projects$Folder"},
		{Key: "Name", Value: name},
	})
	if err != nil {
		t.Fatalf("marshal unit: %v", err)
	}
	return b
}

func storedHash(t *testing.T, w *Writer, unitID string) string {
	t.Helper()
	var got string
	if err := w.reader.db.QueryRow(
		`SELECT ContentsHash FROM Unit WHERE UnitID = ?`, uuidToBlob(unitID),
	).Scan(&got); err != nil {
		t.Fatalf("read ContentsHash: %v", err)
	}
	return got
}

// TestCommitFailsWhenAUnitFileCannotBeFinalized is the #954 assertion: a Commit
// that could not put a unit's bytes on disk must not report success, and must
// name what is stale.
func TestCommitFailsWhenAUnitFileCannotBeFinalized(t *testing.T) {
	const unitID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	stored := unitDoc(t, "Before")
	updated := unitDoc(t, "After")

	w, unitPath := newV2WriterForCommitTest(t, unitID, stored)

	wt, err := w.BeginWriteTransaction()
	if err != nil {
		t.Fatalf("begin write transaction: %v", err)
	}
	if err := wt.WriteUnit(unitID, updated); err != nil {
		t.Fatalf("write unit: %v", err)
	}
	if len(wt.pendingFiles) != 1 {
		t.Fatalf("pendingFiles = %d, want 1 (the write was elided?)", len(wt.pendingFiles))
	}

	// Stand-in for the Windows lock: make the rename fail.
	if err := os.Remove(wt.pendingFiles[0].tempPath); err != nil {
		t.Fatalf("remove temp file: %v", err)
	}

	commitErr := wt.Commit()

	if commitErr == nil {
		t.Fatal("Commit returned nil after failing to finalize a unit file: " +
			"every caller reads nil as a completed write")
	}
	if !strings.Contains(commitErr.Error(), unitID) {
		t.Errorf("Commit error %q does not name the unit %s", commitErr, unitID)
	}

	// The whole point of failing: the project is left as it was, so the Unit row
	// and the file still agree. Asserting the error alone would pass against a
	// version that reported the failure and still left the DB ahead of the disk.
	onDisk, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read unit file: %v", err)
	}
	if string(onDisk) != string(stored) {
		t.Error("unit file changed despite the failed commit")
	}
	if got, want := storedHash(t, w, unitID), hashOf(stored); got != want {
		t.Errorf("ContentsHash = %q, want the stored unit's %q: the database "+
			"describes contents that are not on disk", got, want)
	}
}

// TestCommitLeavesNoUnitFinalizedWhenAnotherFails covers what FlushUnits
// promises — "saves multiple units atomically in a single transaction". A unit
// whose rename succeeded must be put back when a later one fails, or a batch
// write lands half a model.
func TestCommitLeavesNoUnitFinalizedWhenAnotherFails(t *testing.T) {
	const goodID = "11111111-2222-3333-4444-555555555555"
	const badID = "66666666-7777-8888-9999-aaaaaaaaaaaa"
	stored := unitDoc(t, "Before")
	updated := unitDoc(t, "After")

	w, goodPath := newV2WriterForCommitTest(t, goodID, stored)
	badPath := seedUnit(t, w, badID, stored)

	wt, err := w.BeginWriteTransaction()
	if err != nil {
		t.Fatalf("begin write transaction: %v", err)
	}
	for _, id := range []string{goodID, badID} {
		if err := wt.WriteUnit(id, updated); err != nil {
			t.Fatalf("write unit %s: %v", id, err)
		}
	}
	if len(wt.pendingFiles) != 2 {
		t.Fatalf("pendingFiles = %d, want 2", len(wt.pendingFiles))
	}

	// Fail the second rename only; the first is already free to succeed.
	if err := os.Remove(wt.pendingFiles[1].tempPath); err != nil {
		t.Fatalf("remove temp file: %v", err)
	}

	if err := wt.Commit(); err == nil {
		t.Fatal("Commit returned nil after failing to finalize the second unit")
	}

	for _, p := range []string{goodPath, badPath} {
		onDisk, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if string(onDisk) != string(stored) {
			t.Errorf("%s was left finalized after the transaction failed", p)
		}
	}
	for _, id := range []string{goodID, badID} {
		if got, want := storedHash(t, w, id), hashOf(stored); got != want {
			t.Errorf("ContentsHash for %s = %q, want %q", id, got, want)
		}
	}
	assertNoStrayFiles(t, filepath.Dir(goodPath))
	assertNoStrayFiles(t, filepath.Dir(badPath))
}

// seedUnit adds a second unit to a writer built by newV2WriterForCommitTest.
func seedUnit(t *testing.T, w *Writer, unitID string, storedBSON []byte) string {
	t.Helper()
	blob := uuidToBlob(unitID)
	swapped := blobToUUIDSwapped(blob)
	path := filepath.Join(w.reader.contentsDir, swapped[0:2], swapped[2:4], swapped+".mxunit")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir unit dir: %v", err)
	}
	if err := os.WriteFile(path, storedBSON, 0644); err != nil {
		t.Fatalf("seed unit file: %v", err)
	}
	if _, err := w.reader.db.Exec(
		`INSERT INTO Unit (UnitID, ContentsHash) VALUES (?, ?)`, blob, hashOf(storedBSON),
	); err != nil {
		t.Fatalf("insert unit row: %v", err)
	}
	return path
}

// assertNoStrayFiles fails if the directory holds anything but .mxunit files —
// a leftover .tmp or .bak is debris the next run would have to reason about.
func assertNoStrayFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".mxunit") {
			t.Errorf("stray file left in %s: %s", dir, e.Name())
		}
	}
}

// TestCommitSucceedsWhenEveryUnitFileIsFinalized is the false-positive control:
// the ordinary path must stay quiet and must still put the bytes on disk.
func TestCommitSucceedsWhenEveryUnitFileIsFinalized(t *testing.T) {
	const unitID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeef"
	stored := unitDoc(t, "Before")
	updated := unitDoc(t, "After")

	w, unitPath := newV2WriterForCommitTest(t, unitID, stored)

	wt, err := w.BeginWriteTransaction()
	if err != nil {
		t.Fatalf("begin write transaction: %v", err)
	}
	if err := wt.WriteUnit(unitID, updated); err != nil {
		t.Fatalf("write unit: %v", err)
	}
	if err := wt.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	onDisk, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read unit file: %v", err)
	}
	if string(onDisk) != string(updated) {
		t.Error("unit file was not updated on the success path")
	}
	if got, want := storedHash(t, w, unitID), hashOf(updated); got != want {
		t.Errorf("ContentsHash = %q, want %q", got, want)
	}
	if _, err := os.Stat(wt.pendingFiles[0].tempPath); !os.IsNotExist(err) {
		t.Error("temp file survived a successful commit")
	}
	assertNoStrayFiles(t, filepath.Dir(unitPath))
}
