// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"log"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
)

// idToBsonBinary converts a UUID string to BSON Binary format.
// Mendix stores IDs as Binary with Subtype 0.
func idToBsonBinary(id string) bson.Binary {
	blob := uuidToBlob(id)
	if blob == nil || len(blob) != 16 {
		// Generate a new UUID if the provided one is invalid
		blob = uuidToBlob(generateUUID())
	}
	return bson.Binary{
		Subtype: 0x00,
		Data:    blob,
	}
}

// Writer provides methods to write Mendix project files.
type Writer struct {
	reader *Reader
	// sessionBuf, when non-nil, diverts updateUnit calls to a buffering
	// callback instead of going to disk. Used by import-style flows that
	// want to batch many unit updates into a single transaction.
	sessionBuf func(unitID string, contents []byte) error

	// writesOffered / writesLanded count what reached reconcileWithStored and how
	// much of it survived no-op elision (ADR-0008). The executor reads them to
	// tell "Modified X" from "X was already in sync" — without them, re-running a
	// script that changes nothing still announces a write for every statement,
	// which is how the churn in #910 was misdiagnosed. The backend adds its own
	// non-unit writes (generated .java/.js source) to this total.
	writesOffered int
	writesLanded  int

	// removedUnits holds what this session deleted, keyed by unit ID, so a unit
	// RE-INSERTED under the same ID can be reconciled against what it replaced.
	// See carryIdentityFromRemovedUnit.
	removedUnits map[string]removedUnit

	// deferred holds unit updates while a run of statements is judged by its
	// net result (writer_deferred.go); nil when no run is open.
	deferred      map[string]deferredWrite
	deferredOrder []string
	// simulating says the deferred run is a simulation (writer_simulated.go):
	// it is discarded, never flushed, and any write it cannot hold is refused.
	simulating bool
}

// removedUnit is everything about a deleted unit that a re-insert has to be
// compared against: its bytes, its row, and the project's transaction id at the
// moment it went. The last one is what lets a delete+insert that nets to nothing
// leave the .mpr alone entirely.
type removedUnit struct {
	contents        []byte
	containerID     []byte
	containmentName string
	transactionID   string
}

// maxRemovedUnitsRemembered caps the delete→insert carry. A script that drops a
// module deletes thousands of units and re-inserts none of them, so the map is
// bounded rather than allowed to hold a whole project in memory. Past the cap
// nothing further is recorded and the carry degrades to the behaviour that
// existed before it — a churned re-insert, not a wrong one.
const maxRemovedUnitsRemembered = 1024

// WriteStats reports how many unit writes this session offered to storage and
// how many were not elided as no-ops.
func (w *Writer) WriteStats() (offered, written int) {
	return w.writesOffered, w.writesLanded
}

// SetSessionBuf installs a callback that intercepts every updateUnit call.
// While set, updateUnit short-circuits to fn and skips all disk/SQLite work.
// Callers (typically batch-import flows) are responsible for flushing the
// buffered bytes back to disk before calling ClearSessionBuf.
//
// Pass nil to disable; ClearSessionBuf is the preferred way to do that.
func (w *Writer) SetSessionBuf(fn func(unitID string, contents []byte) error) {
	w.sessionBuf = fn
}

// ClearSessionBuf removes any installed sessionBuf callback so subsequent
// updateUnit calls resume normal disk-backed writes.
func (w *Writer) ClearSessionBuf() {
	w.sessionBuf = nil
}

// NewWriter creates a new writer from a reader opened in read-write mode.
func NewWriter(path string) (*Writer, error) {
	reader, err := OpenWithOptions(path, OpenOptions{ReadOnly: false})
	if err != nil {
		return nil, err
	}
	return &Writer{reader: reader}, nil
}

// NewWriterFromDB creates a Writer that reuses an existing *sql.DB connection.
// The caller owns the db lifecycle; this Writer's Close() does not close the db.
// contentsDir should be the mprcontents folder path (v2) or empty string (v1).
func NewWriterFromDB(db *sql.DB, path, contentsDir string) (*Writer, error) {
	reader, err := OpenWithDB(db, path, contentsDir)
	if err != nil {
		return nil, fmt.Errorf("open reader with shared db: %w", err)
	}
	return &Writer{reader: reader}, nil
}

// NewWriterWithReader creates a Writer that reuses an existing Reader instead of
// opening a second reader. This ensures cache invalidation (called by insertUnit
// via w.reader.InvalidateCache()) propagates to the same Reader object that callers
// hold for listing — so writes are immediately visible to reads on the same backend.
func NewWriterWithReader(r *Reader) *Writer {
	return &Writer{reader: r}
}

// Close closes the writer, writing any update a deferred run still holds.
func (w *Writer) Close() error {
	flushErr := w.FlushDeferredWrites()
	if err := w.reader.Close(); err != nil {
		return err
	}
	return flushErr
}

// Reader returns the underlying reader as a UnitReader interface.
func (w *Writer) Reader() UnitReader {
	return w.reader
}

// ConcreteReader returns the underlying *Reader for callers that need
// concrete methods not exposed by UnitReader (e.g. codec.Store).
func (w *Writer) ConcreteReader() *Reader {
	return w.reader
}

// Transaction support

// Transaction represents a database transaction.
type Transaction struct {
	tx     *sql.Tx
	writer *Writer
}

// BeginTransaction starts a new transaction.
func (w *Writer) BeginTransaction() (*Transaction, error) {
	tx, err := w.reader.db.Begin()
	if err != nil {
		return nil, err
	}
	return &Transaction{tx: tx, writer: w}, nil
}

// Commit commits the transaction.
func (t *Transaction) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction.
func (t *Transaction) Rollback() error {
	return t.tx.Rollback()
}

// WriteTransaction provides atomic write operations for MPR v2 format.
// It coordinates database and file system changes to ensure consistency.
type WriteTransaction struct {
	tx           *sql.Tx
	writer       *Writer
	pendingFiles []pendingFile
	finalized    []finalizedFile
	committed    bool
}

type pendingFile struct {
	unitID    string
	tempPath  string
	finalPath string
}

// finalizedFile records a rename that has already happened, so it can be undone
// if a later step of the same Commit fails. backupPath is empty when the unit
// had no file on disk to preserve.
type finalizedFile struct {
	pendingFile
	backupPath string
}

// BeginWriteTransaction starts a new write transaction.
// For v2 format, this coordinates both database and file writes.
func (w *Writer) BeginWriteTransaction() (*WriteTransaction, error) {
	tx, err := w.reader.db.Begin()
	if err != nil {
		return nil, err
	}
	return &WriteTransaction{
		tx:           tx,
		writer:       w,
		pendingFiles: make([]pendingFile, 0),
	}, nil
}

// storedBytes reads a unit as this transaction sees it, which is what a write
// in it has to be reconciled against. On v1 that is a read through wt.tx: the
// pool's one connection is the one wt.tx holds, so reading through the reader
// waited for it forever, and the transaction's own reads see its writes. On v2
// the transaction's own write sits in a temp file until Commit, so the latest
// pending file for the unit comes first; reconciling against the committed
// .mxunit instead elided A → B → A's second write and committed B.
func (wt *WriteTransaction) storedBytes(unitID string) ([]byte, error) {
	blob := uuidToBlob(unitID)
	for i := len(wt.pendingFiles) - 1; i >= 0; i-- {
		if pf := wt.pendingFiles[i]; bytes.Equal(uuidToBlob(pf.unitID), blob) {
			return os.ReadFile(pf.tempPath)
		}
	}
	return wt.writer.reader.rawUnitBytes(wt.tx, unitID)
}

// WriteUnit writes a unit within the transaction.
// The actual file write is deferred until Commit.
func (wt *WriteTransaction) WriteUnit(unitID string, contents []byte) error {
	// Same no-op elision as updateUnit. This is a second write path into the
	// same storage (codec.Store.SaveUnit / FlushUnits reach it), so leaving it
	// out would mean the codec engine's own document writes kept churning while
	// UpdateRawUnit's stopped — a difference no caller could reason about.
	//
	// The stored bytes are the transaction's view of the unit (storedBytes).
	contents, unchanged, err := wt.writer.reconcileAgainst(unitID, contents, wt.storedBytes)
	if err != nil {
		return err
	}
	if unchanged {
		return nil
	}
	if err := wt.writer.guardWrite(); err != nil {
		return err
	}

	unitIDBlob := uuidToBlob(unitID)

	if wt.writer.reader.version == MPRVersionV2 {
		swappedUUID := blobToUUIDSwapped(unitIDBlob)

		// Create directory if needed
		dir := filepath.Join(wt.writer.reader.contentsDir, swappedUUID[0:2], swappedUUID[2:4])
		finalPath := filepath.Join(dir, swappedUUID+".mxunit")
		if err := journalFile(finalPath); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		// Write to temp file first
		tempPath := finalPath + ".tmp"

		if err := os.WriteFile(tempPath, contents, 0644); err != nil {
			return fmt.Errorf("failed to write temp file: %w", err)
		}

		wt.pendingFiles = append(wt.pendingFiles, pendingFile{
			unitID:    unitID,
			tempPath:  tempPath,
			finalPath: finalPath,
		})

		// Update hash in DB
		_, err := wt.tx.Exec(`
			UPDATE Unit SET ContentsHash = ? WHERE UnitID = ?
		`, contentsHashOf(contents), unitIDBlob)
		return err
	}

	// V1: contents live in the row, and so does their hash. Studio Pro fills
	// ContentsHash on every v1 unit too (ako/mxcli#1159), so it moves with them.
	_, err = wt.tx.Exec(`
		UPDATE Unit SET Contents = ?, ContentsHash = ? WHERE UnitID = ?
	`, contents, contentsHashOf(contents), unitIDBlob)
	return err
}

// Commit commits the transaction.
//
// For v2 the unit files are renamed into place FIRST and the database is
// committed only once every rename has succeeded; any failure undoes the
// renames and rolls the transaction back, so the two either move together or
// not at all. The order matters: committing first and then renaming leaves the
// Unit table — ContentsHash included — describing bytes that are not on disk,
// and the old code warned about that and returned nil, so SaveUnit and
// FlushUnits reported a write that had not happened (upstream #954). A rename
// fails for ordinary environmental reasons — on Windows a .mxunit held open
// without FILE_SHARE_DELETE by an editor, an indexer or a sync client is enough.
//
// This also matches updateUnit, the single-unit path, which has always failed
// hard when it could not put a unit's bytes on disk.
func (wt *WriteTransaction) Commit() error {
	if wt.committed {
		return fmt.Errorf("transaction already committed")
	}

	if err := wt.finalizeFiles(); err != nil {
		wt.undoFinalizedFiles()
		wt.cleanupTempFiles()
		_ = wt.tx.Rollback()
		return err
	}

	if err := wt.tx.Commit(); err != nil {
		wt.undoFinalizedFiles()
		wt.cleanupTempFiles()
		return err
	}

	wt.discardBackups()
	wt.committed = true
	// Invalidate reader cache so next read sees the updated data
	wt.writer.reader.InvalidateCache()
	return nil
}

// finalizeFiles renames each pending temp file into place, first moving the file
// it replaces aside so undoFinalizedFiles can put it back. It stops at the first
// failure, leaving wt.finalized describing exactly what has to be undone.
func (wt *WriteTransaction) finalizeFiles() error {
	seen := make(map[string]bool, len(wt.pendingFiles))
	for _, pf := range wt.pendingFiles {
		// A unit written twice in one transaction shares a temp path, so the
		// first rename already carried the latest bytes; a second would move the
		// file just written aside as if it were the stored one.
		if seen[pf.finalPath] {
			continue
		}
		seen[pf.finalPath] = true

		backupPath := ""
		if _, err := os.Stat(pf.finalPath); err == nil {
			backupPath = pf.finalPath + ".bak"
			if err := os.Rename(pf.finalPath, backupPath); err != nil {
				return fmt.Errorf("finalize unit %s: cannot move %s aside: %w",
					pf.unitID, pf.finalPath, err)
			}
		}
		if err := os.Rename(pf.tempPath, pf.finalPath); err != nil {
			if backupPath != "" {
				_ = os.Rename(backupPath, pf.finalPath)
			}
			return fmt.Errorf("finalize unit %s: cannot write %s: %w",
				pf.unitID, pf.finalPath, err)
		}
		wt.finalized = append(wt.finalized, finalizedFile{pendingFile: pf, backupPath: backupPath})
	}
	return nil
}

// undoFinalizedFiles reverses finalizeFiles, newest first: the new bytes go back
// to their temp path (for cleanupTempFiles to remove) and the file that was
// moved aside returns to its own name.
func (wt *WriteTransaction) undoFinalizedFiles() {
	for i := len(wt.finalized) - 1; i >= 0; i-- {
		f := wt.finalized[i]
		if err := os.Rename(f.finalPath, f.tempPath); err != nil {
			log.Printf("mpr.undo_failed: path=%s error=%s", f.finalPath, err.Error())
		}
		if f.backupPath != "" {
			if err := os.Rename(f.backupPath, f.finalPath); err != nil {
				log.Printf("mpr.restore_failed: path=%s error=%s", f.finalPath, err.Error())
			}
		}
	}
	wt.finalized = nil
}

// discardBackups drops the moved-aside files once the commit has succeeded and
// they can no longer be needed. A leftover is inert — nothing reads a path that
// is not <unit>.mxunit — so a failure here is logged, not returned.
func (wt *WriteTransaction) discardBackups() {
	for _, f := range wt.finalized {
		if f.backupPath == "" {
			continue
		}
		if err := os.Remove(f.backupPath); err != nil {
			log.Printf("mpr.backup_cleanup_failed: path=%s error=%s", f.backupPath, err.Error())
		}
	}
	wt.finalized = nil
}

// Rollback rolls back the transaction and cleans up temp files.
func (wt *WriteTransaction) Rollback() error {
	if wt.committed {
		return fmt.Errorf("transaction already committed")
	}

	// Clean up temp files
	wt.cleanupTempFiles()

	// Rollback database
	return wt.tx.Rollback()
}

func (wt *WriteTransaction) cleanupTempFiles() {
	for _, pf := range wt.pendingFiles {
		os.Remove(pf.tempPath)
	}
}

// generateUUID generates a new UUID v4 for model elements.
// Returns format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant is 10

	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[0], b[1], b[2], b[3],
		b[4], b[5],
		b[6], b[7],
		b[8], b[9],
		b[10], b[11], b[12], b[13], b[14], b[15])
}

// DeleteChildUnits recursively deletes all units whose ContainerID matches parentID.
func (w *Writer) DeleteChildUnits(parentID string) error {
	return w.deleteChildUnitsRecursive(parentID)
}

func (w *Writer) deleteChildUnitsRecursive(parentID string) error {
	parentBlob := uuidToBlob(parentID)
	if parentBlob == nil {
		return fmt.Errorf("invalid parent ID: %s", parentID)
	}

	rows, err := w.reader.db.Query("SELECT UnitID FROM Unit WHERE ContainerID = ? AND UnitID != ContainerID", parentBlob)
	if err != nil {
		return err
	}
	defer rows.Close()

	var childIDs []string
	for rows.Next() {
		var childBlob []byte
		if err := rows.Scan(&childBlob); err != nil {
			return err
		}
		childIDs = append(childIDs, blobToUUID(childBlob))
	}

	for _, childID := range childIDs {
		if err := w.deleteChildUnitsRecursive(childID); err != nil {
			return err
		}
		if err := w.deleteUnit(childID); err != nil {
			return err
		}
	}

	return nil
}

// uuidToBlob converts a UUID string to a 16-byte blob in Microsoft GUID format.
// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
// Microsoft GUID format byte-swaps the first 3 groups (little-endian):
// - First 4 bytes: reversed
// - Next 2 bytes: reversed
// - Next 2 bytes: reversed
// - Last 8 bytes: unchanged
func uuidToBlob(uuid string) []byte {
	if uuid == "" {
		return nil
	}
	// Remove dashes
	var clean strings.Builder
	for _, c := range uuid {
		if c != '-' {
			clean.WriteString(string(c))
		}
	}
	// Decode hex to bytes
	decoded, err := hex.DecodeString(clean.String())
	if err != nil || len(decoded) != 16 {
		return nil
	}
	// Swap bytes to Microsoft GUID format
	blob := make([]byte, 16)
	// First 4 bytes: reversed
	blob[0] = decoded[3]
	blob[1] = decoded[2]
	blob[2] = decoded[1]
	blob[3] = decoded[0]
	// Next 2 bytes: reversed
	blob[4] = decoded[5]
	blob[5] = decoded[4]
	// Next 2 bytes: reversed
	blob[6] = decoded[7]
	blob[7] = decoded[6]
	// Last 8 bytes: unchanged
	copy(blob[8:], decoded[8:])
	return blob
}

// ---------------------------------------------------------------------------
// Unit CRUD operations (merged from writer_units.go)
// ---------------------------------------------------------------------------

// updateTransactionID updates the _Transaction table with a new UUID.
// Studio Pro uses this to detect external changes during F4 sync.
// Only applies to MPR v2 projects (Mendix >= 10.18).
func (w *Writer) updateTransactionID() {
	if w.reader.version != MPRVersionV2 {
		return
	}
	newID := generateUUID()
	_, _ = w.reader.db.Exec(`UPDATE _Transaction SET LastTransactionID = ?`, newID)
}

// placeholderBinaryPrefix is the GUID-swapped byte pattern for placeholder IDs generated
// by sdk/widgets/augment.go placeholderID(). These are "aa000000000000000000000000XXXXXX"
// hex strings which, after hex decode + GUID byte-swap, produce 16-byte blobs whose first
// 13 bytes are \x00\x00\x00\xaa followed by 9 zero bytes.
var placeholderBinaryPrefix = []byte{0x00, 0x00, 0x00, 0xaa, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

// placeholderStringPrefix is the ASCII prefix of a placeholder ID that leaked as a string.
var placeholderStringBytes = []byte("aa000000000000000000000000")

// validateNoPlaceholderIDs scans raw BSON bytes for leaked placeholder IDs.
// Returns an error if any placeholder pattern is found.
func validateNoPlaceholderIDs(unitID string, contents []byte) error {
	if bytes.Contains(contents, placeholderBinaryPrefix) {
		return fmt.Errorf("placeholder ID leak detected in unit %s: binary aa000000-prefix ID found in BSON contents", unitID)
	}
	if bytes.Contains(contents, placeholderStringBytes) {
		return fmt.Errorf("placeholder ID leak detected in unit %s: string aa000000-prefix ID found in BSON contents", unitID)
	}
	return nil
}

func (w *Writer) insertUnit(unitID, containerID, containmentName, unitType string, contents []byte) error {
	if err := validateNoPlaceholderIDs(unitID, contents); err != nil {
		return err
	}
	if err := canon.DuplicateElementIDError(unitID, contents); err != nil {
		return err
	}
	if err := canon.BareAttributeRefError(unitID, contents); err != nil {
		return err
	}
	if err := w.guardWrite(); err != nil {
		return err
	}

	// Convert UUID strings to 16-byte blobs for database
	unitIDBlob := uuidToBlob(unitID)
	containerIDBlob := uuidToBlob(containerID)

	if unitIDBlob == nil {
		return fmt.Errorf("invalid unit ID (not a valid UUID): %q", unitID)
	}
	if containerIDBlob == nil {
		return fmt.Errorf("invalid container ID (not a valid UUID): %q", containerID)
	}

	contents, normErr := w.normalizeForStorage(unitID, contents)
	if normErr != nil {
		return normErr
	}
	contents, restoreTransactionID := w.carryIdentityFromRemovedUnit(
		unitID, containerIDBlob, containmentName, contents)

	if w.reader.version == MPRVersionV2 {
		// Get swapped UUID for file path
		swappedUUID := blobToUUIDSwapped(unitIDBlob)

		// Create directory structure: mprcontents/XX/YY/
		dir := filepath.Join(w.reader.contentsDir, swappedUUID[0:2], swappedUUID[2:4])
		filePath := filepath.Join(dir, swappedUUID+".mxunit")
		if err := journalFile(filePath); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		// Write content file
		if err := os.WriteFile(filePath, contents, 0644); err != nil {
			return fmt.Errorf("failed to write unit file: %w", err)
		}

		// Insert reference to database
		_, err := w.reader.db.Exec(`
			INSERT INTO Unit (UnitID, ContainerID, ContainmentName, TreeConflict, ContentsHash, ContentsConflicts)
			VALUES (?, ?, ?, 0, ?, '')
		`, unitIDBlob, containerIDBlob, containmentName, contentsHashOf(contents))
		if err != nil {
			// Clean up the file we just wrote — otherwise it becomes an orphan
			os.Remove(filePath)
			return err
		}
		w.reader.InvalidateCache()
		w.updateTransactionID()
		w.restoreTransactionID(restoreTransactionID)
		return nil
	}

	// MPR v1: Store directly in database, with the hash Studio Pro stores
	// beside the contents on every v1 unit (ako/mxcli#1159; this wrote '').
	// Try new schema first (without Type column - Mendix 11.6.2+)
	_, err := w.reader.db.Exec(`
		INSERT INTO Unit (UnitID, ContainerID, ContainmentName, TreeConflict, ContentsHash, ContentsConflicts, Contents)
		VALUES (?, ?, ?, 0, ?, '', ?)
	`, unitIDBlob, containerIDBlob, containmentName, contentsHashOf(contents), contents)
	if err != nil {
		// Try old schema with Type column
		_, err = w.reader.db.Exec(`
			INSERT INTO Unit (UnitID, ContainerID, ContainmentName, Type, Contents)
			VALUES (?, ?, ?, ?, ?)
		`, unitIDBlob, containerIDBlob, containmentName, unitType, contents)
	}
	if err == nil {
		w.reader.InvalidateCache()
	}
	return err
}

func (w *Writer) updateUnit(unitID string, contents []byte, opts ...canon.Option) error {
	if err := validateNoPlaceholderIDs(unitID, contents); err != nil {
		return err
	}
	// Two elements sharing an $ID make the whole project unopenable, so the
	// bytes are checked BEFORE reconciliation rather than after: an elided
	// write is not a safe one, it is merely one that did not happen this time,
	// and the same contents would be offered again on the next run. Checking
	// here also covers the paths that reach storage without a rebuild —
	// UpdateRawUnit and ALTER's targeted patches — which is where a duplicate
	// is most likely to arrive unnoticed. (ako/mxcli-captrack #2)
	if err := canon.DuplicateElementIDError(unitID, contents); err != nil {
		return err
	}
	// Same reasoning, same place: a bare DomainModels$AttributeRef makes the
	// project unloadable, and ALTER PAGE's patches reach here without passing
	// the page encoder that also refuses one (canon/attributeref.go).
	if err := canon.BareAttributeRefError(unitID, contents); err != nil {
		return err
	}

	// Session mode: divert to in-memory buffer and skip all disk/SQLite work.
	// The caller (e.g. ImportProject) is responsible for flushing later.
	if w.sessionBuf != nil {
		return w.sessionBuf(unitID, contents)
	}
	// A run whose writes are judged by its net result holds the update; the
	// run's flush reconciles it against what was stored before the run.
	if w.holdDeferred(unitID, contents, opts) {
		return nil
	}

	contents, unchanged, err := w.reconcileWithStored(unitID, contents, opts...)
	if err != nil {
		return err
	}
	if unchanged {
		return nil
	}
	if err := w.guardWrite(); err != nil {
		return err
	}

	// Convert UUID string to 16-byte blob
	unitIDBlob := uuidToBlob(unitID)

	if w.reader.version == MPRVersionV2 {
		// Get swapped UUID for file path
		swappedUUID := blobToUUIDSwapped(unitIDBlob)

		// Build file path: mprcontents/XX/YY/UUID.mxunit
		filePath := filepath.Join(
			w.reader.contentsDir,
			swappedUUID[0:2],
			swappedUUID[2:4],
			swappedUUID+".mxunit",
		)

		// Write to a temp file then atomically rename into place.
		// Direct WriteFile would overwrite the shared inode when the target
		// is a hard link (e.g. in test fixtures), corrupting the source file.
		// Rename replaces the directory entry, leaving the linked inode intact.
		if err := journalFile(filePath); err != nil {
			return err
		}
		tmpPath := filePath + ".tmp"
		if err := os.WriteFile(tmpPath, contents, 0644); err != nil {
			return fmt.Errorf("failed to write unit temp file: %w", err)
		}
		if err := os.Rename(tmpPath, filePath); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("failed to rename unit file: %w", err)
		}

		// Update ContentsHash in database
		_, err := w.reader.db.Exec(`
			UPDATE Unit SET ContentsHash = ? WHERE UnitID = ?
		`, contentsHashOf(contents), unitIDBlob)
		if err == nil {
			w.reader.InvalidateCache()
			w.updateTransactionID()
		}
		return err
	}

	// MPR v1: Update in database. ContentsHash goes in the same statement:
	// Studio Pro fills it on every v1 unit and reads it to see what changed,
	// and mx check never looks at it (ako/mxcli#1159).
	_, err = w.reader.db.Exec(`
		UPDATE Unit SET Contents = ?, ContentsHash = ? WHERE UnitID = ?
	`, contents, contentsHashOf(contents), unitIDBlob)
	return err
}

// reconcileWithStored applies the shared no-op-elision policy (canon.Reconcile,
// ADR-0008 decision 1) to a write against this project.
func (w *Writer) reconcileWithStored(unitID string, contents []byte, opts ...canon.Option) (out []byte, unchanged bool, err error) {
	return w.reconcileAgainst(unitID, contents, w.reader.GetRawUnitBytes, opts...)
}

// reconcileAgainst is reconcileWithStored reading the stored unit with
// readStored, so a write inside a transaction can read through it.
func (w *Writer) reconcileAgainst(unitID string, contents []byte, readStored func(string) ([]byte, error), opts ...canon.Option) (out []byte, unchanged bool, err error) {
	w.writesOffered++
	// Normalised before comparing, so a rebuild of a Studio Pro document that
	// differed from it only by the properties the writer leaves out is elided.
	contents, err = w.normalizeForStorage(unitID, contents)
	if err != nil {
		return nil, false, err
	}
	stored, readErr := readStored(unitID)
	if readErr != nil {
		w.writesLanded++
		return contents, false, nil // new unit, or unreadable — write it
	}
	out, unchanged = canon.Reconcile(contents, stored, opts...)

	// The storage-GUID guard runs here, on the reconciled bytes, because it is
	// the transplant inside Reconcile that establishes which written element is
	// which stored one: before it, every rebuilt element carries a freshly minted
	// $ID that matches nothing. An element that came out of Reconcile sharing an
	// $ID with a stored element but carrying a different GUID is a rewrite that
	// dropped the database's identity for it (#1119) — refuse rather than write.
	//
	// Placed in reconcileWithStored rather than beside the duplicate-$ID guard in
	// updateUnit so that BOTH choke points get it: WriteTransaction.WriteUnit is
	// how codec.Store reaches storage, and a guard on only one of them is the
	// inconsistency CLAUDE.md's "adding a write path means wiring it to
	// canon.Reconcile" exists to prevent.
	//
	// A deliberate identity transplant (the marketplace module update) is the one
	// write whose purpose is to move GUIDs, and it says so.
	if !canon.OwnsStorageGUIDs(opts...) {
		if guardErr := canon.StorageGUIDError(unitID, out, stored); guardErr != nil {
			return nil, false, guardErr
		}
	}

	if !unchanged {
		w.writesLanded++
	}
	return out, unchanged, nil
}

// rememberRemovedUnit captures a unit's bytes on the way out, so an insert of
// the same unit ID later in this session can be reconciled against them.
//
// Read before the row and file go, obviously, and best-effort: a unit whose
// bytes cannot be read is simply not remembered, which costs a churned
// re-insert and never a wrong one.
func (w *Writer) rememberRemovedUnit(unitID string) {
	if len(w.removedUnits) >= maxRemovedUnitsRemembered {
		return
	}
	contents, err := w.reader.GetRawUnitBytes(unitID)
	if err != nil || len(contents) == 0 {
		return
	}
	rec := removedUnit{contents: append([]byte(nil), contents...)}
	_ = w.reader.db.QueryRow(
		`SELECT ContainerID, ContainmentName FROM Unit WHERE UnitID = ?`, uuidToBlob(unitID),
	).Scan(&rec.containerID, &rec.containmentName)
	_ = w.reader.db.QueryRow(`SELECT LastTransactionID FROM _Transaction`).Scan(&rec.transactionID)

	if w.removedUnits == nil {
		w.removedUnits = make(map[string]removedUnit)
	}
	w.removedUnits[unitID] = rec
}

// carryIdentityFromRemovedUnit applies the shared reconciliation policy to a
// unit being re-inserted under the ID of one this session deleted.
//
// # Why an insert needs this at all
//
// Several `create or modify` handlers are implemented as delete + insert under
// the preserved unit ID rather than as an update — the consumed REST client is
// the measured one (ako/mxcli#556): re-running an identical statement rewrote 9
// element $IDs in a 1,128-byte unit, on every run, forever. The document was
// never different; canon was simply never asked, because updateUnit is where
// Reconcile is called and a delete+insert does not go through it.
//
// # What it does and does not do
//
// It carries identity, translations and element $IDs exactly as updateUnit
// does, so a re-inserted unit that means the same thing lands byte-identical.
// It does NOT elide: the row and the file are already gone, so something has to
// be written back whatever the bytes say. That asymmetry is the whole reason
// this is a separate function from reconcileWithStored rather than a flag on it.
//
// The write is still counted, and counted as landed only when the bytes moved,
// so ReportMutation can tell a real rewrite from one that restored what it
// removed.
//
// A $Type that changed means the new unit is not the old one; canon's pairing
// refuses to carry anything across that, so nothing special is needed here.
func (w *Writer) carryIdentityFromRemovedUnit(
	unitID string, containerID []byte, containmentName string, contents []byte,
) (out []byte, restoreTransactionID string) {
	prev, ok := w.removedUnits[unitID]
	if !ok {
		return contents, ""
	}
	delete(w.removedUnits, unitID)

	w.writesOffered++
	out, _ = canon.Reconcile(contents, prev.contents)

	// A delete+insert that put back exactly what it removed changed nothing, so
	// the project's transaction id — which is how Studio Pro decides there is
	// something to re-sync — must not move either. The test is deliberately the
	// strict one (identical bytes and an identical row), not canon's "unchanged":
	// the file is written back regardless, so the only question worth asking is
	// whether what landed is what was there.
	if bytes.Equal(out, prev.contents) &&
		bytes.Equal(containerID, prev.containerID) &&
		containmentName == prev.containmentName {
		return out, prev.transactionID
	}
	w.writesLanded++
	return out, ""
}

// restoreTransactionID puts back the transaction id a no-op delete+insert bumped
// twice. Best-effort, like updateTransactionID itself, and a no-op for v1
// projects and for any insert that was not a recreate.
func (w *Writer) restoreTransactionID(id string) {
	if id == "" || w.reader.version != MPRVersionV2 {
		return
	}
	_, _ = w.reader.db.Exec(`UPDATE _Transaction SET LastTransactionID = ?`, id)
}

// UpdateRawUnit saves raw BSON bytes for a unit, bypassing deserialization.
// Used by ALTER PAGE to modify the BSON widget tree directly.
func (w *Writer) UpdateRawUnit(unitID string, contents []byte) error {
	return w.updateUnit(unitID, contents)
}

// UpdateRawUnitOwningTranslations is UpdateRawUnit for a write that started from
// the stored bytes and is authoritative about every translation in them — so the
// stored translations must NOT be carried back onto it.
//
// The ordinary path carries them, because a rebuild from MDL can only express
// one string per text and would otherwise delete the rest (PROPOSAL_translations
// slice 1). A targeted patch is the opposite case: a translation missing from its
// output is missing on purpose. Without this, `create or replace translations`
// reported removing 22 translations and every one was silently restored.
func (w *Writer) UpdateRawUnitOwningTranslations(unitID string, contents []byte) error {
	return w.updateUnit(unitID, contents, canon.ContentsOwnTranslations())
}

// UpdateRawUnitPatch is UpdateRawUnit for a write that PATCHED the stored bytes
// in place rather than rebuilding them — the graph splice of `alter microflow`.
// Such a write already carries every stored $ID and translation it means to
// keep, so neither is carried back: re-pairing element $IDs structurally would
// move identities onto other elements after a drop (canon.ContentsOwnElementIDs),
// and carrying translations would undo a deliberate removal. Elision and the
// storage-GUID guard apply as for every write.
func (w *Writer) UpdateRawUnitPatch(unitID string, contents []byte) error {
	return w.updateUnit(unitID, contents, canon.ContentsOwnElementIDs(), canon.ContentsOwnTranslations())
}

// UpdateRawUnitOwningStorageGUIDs is UpdateRawUnit for a write that deliberately
// transplants storage GUIDs onto elements that keep their $ID — the marketplace
// module update, which carries a module's existing GUIDs onto the documents
// replacing it so the next deploy does not destroy that module's data.
//
// That is the exact pattern the #1119 guard refuses, and rightly: for every other
// write it means the database's identity for an element was dropped. Only a
// caller that captured those GUIDs from the stored model and is putting them back
// may use this. See canon.ContentsOwnStorageGUIDs.
func (w *Writer) UpdateRawUnitOwningStorageGUIDs(unitID string, contents []byte) error {
	return w.updateUnit(unitID, contents, canon.ContentsOwnStorageGUIDs())
}

// InsertUnit creates a new unit in the project database.
// This is the exported version of insertUnit for use by TreeWriter and other packages.
func (w *Writer) InsertUnit(unitID, containerID, containmentName, unitType string, contents []byte) error {
	return w.insertUnit(unitID, containerID, containmentName, unitType, contents)
}

// DeleteUnit removes a unit from the project database.
// This is the exported version of deleteUnit for use by TreeWriter and other packages.
func (w *Writer) DeleteUnit(unitID string) error {
	return w.deleteUnit(unitID)
}

// MoveUnit reparents a unit to a new container (used by MOVE for top-level
// documents like enumerations/constants/microflows). The container lives in the
// Unit table; contents files are keyed by UnitID, so only the row changes.
//
// Counted in WriteStats like any other write, and elided when the unit is
// already in that container. Both matter: a move is the one mutation that
// changes a document's placement without changing a byte of its contents, so
// without the count ReportMutation calls a real move "Unchanged", and without
// the elision re-running an already-applied script dirties the .mpr — the two
// halves of ADR-0008's guarantee, applied to the row instead of the blob.
func (w *Writer) MoveUnit(unitID, newContainerID string) error {
	w.writesOffered++
	target := uuidToBlob(newContainerID)
	if stored, err := w.containerOfUnit(unitID); err == nil && bytes.Equal(stored, target) {
		return nil
	}
	if err := w.guardWrite(); err != nil {
		return err
	}
	_, err := w.reader.db.Exec(`UPDATE Unit SET ContainerID = ? WHERE UnitID = ?`,
		target, uuidToBlob(unitID))
	if err == nil {
		w.writesLanded++
		w.reader.InvalidateCache()
	}
	return err
}

// containerOfUnit reads a unit's stored ContainerID blob. Compared as bytes
// rather than as a formatted UUID so the check cannot disagree with the write
// about spelling or byte order.
func (w *Writer) containerOfUnit(unitID string) ([]byte, error) {
	var stored []byte
	err := w.reader.db.QueryRow(`SELECT ContainerID FROM Unit WHERE UnitID = ?`,
		uuidToBlob(unitID)).Scan(&stored)
	return stored, err
}

func (w *Writer) deleteUnit(unitID string) error {
	// Convert UUID string to 16-byte blob
	unitIDBlob := uuidToBlob(unitID)
	if unitIDBlob == nil {
		return fmt.Errorf("invalid unit ID: %s", unitID)
	}
	if err := w.guardWrite(); err != nil {
		return err
	}
	w.dropDeferred(unitID)
	w.rememberRemovedUnit(unitID)

	if w.reader.version == MPRVersionV2 {
		// Get swapped UUID for file path
		swappedUUID := blobToUUIDSwapped(unitIDBlob)

		// Delete external file
		subDir1 := swappedUUID[0:2]
		subDir2 := swappedUUID[2:4]
		filePath := filepath.Join(w.reader.contentsDir, subDir1, subDir2, swappedUUID+".mxunit")
		if err := journalFile(filePath); err != nil {
			return err
		}
		os.Remove(filePath) // Ignore error if file doesn't exist

		// Clean up empty parent directories (YY/, then XX/)
		dir2 := filepath.Join(w.reader.contentsDir, subDir1, subDir2)
		os.Remove(dir2) // Only succeeds if empty
		dir1 := filepath.Join(w.reader.contentsDir, subDir1)
		os.Remove(dir1) // Only succeeds if empty
	}

	result, err := w.reader.db.Exec(`DELETE FROM Unit WHERE UnitID = ?`, unitIDBlob)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("unit not found in database: %s", unitID)
	}

	w.reader.InvalidateCache()
	w.updateTransactionID()
	return nil
}

// UpdateUnitContainer changes the ContainerID of a unit, moving it to a new parent.
func (w *Writer) UpdateUnitContainer(unitID, newContainerID string) error {
	unitIDBlob := uuidToBlob(unitID)
	if unitIDBlob == nil {
		return fmt.Errorf("invalid unit ID: %s", unitID)
	}
	containerIDBlob := uuidToBlob(newContainerID)
	if containerIDBlob == nil {
		return fmt.Errorf("invalid container ID: %s", newContainerID)
	}
	if err := w.guardWrite(); err != nil {
		return err
	}

	result, err := w.reader.db.Exec(`UPDATE Unit SET ContainerID = ? WHERE UnitID = ?`, containerIDBlob, unitIDBlob)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("unit not found: %s", unitID)
	}

	w.reader.InvalidateCache()
	w.updateTransactionID()
	return nil
}

// normalizeForStorage gives every element exactly the property set Studio
// Pro writes for its $Type on this project's version: keys the version does not
// declare yet are stripped when empty and refused when they hold a value
// (canon.StripUndeclaredProperties), then missing ones are added
// (canon.CompletePropertySets). It runs on every unit that reaches storage —
// insert, update, and the transaction path, both through reconcileWithStored —
// because a key too many or too few from ANY writer makes the document
// unopenable or unmergeable (mendixlabs/mxcli#1373), and a fix per writer is
// the list that never ends.
//
// Every registered list then gets the marker Studio Pro writes for it
// (canon.NormalizeListMarkers). The encoder writes that marker for a list it
// builds; a list a rewrite passes through as stored bytes — an unchanged
// pluggable widget, an existing element's clean property, a mutator's in-place
// edit — kept whatever an older build wrote, on every later rewrite
// (ako/mxcli#1640).
func (w *Writer) normalizeForStorage(unitID string, contents []byte) ([]byte, error) {
	out, err := normalizePropertySetsFor(w.reader.ProjectVersion(), unitID, contents)
	if err != nil {
		return nil, err
	}
	return canon.NormalizeListMarkers(out), nil
}
