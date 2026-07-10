// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// isContentsHashSchemaError returns true when the error looks like SQLite complaining
// about the absence of the ContentsHash column (i.e. an old MPR v1 schema from pre-Mx
// versions that predate ContentsHash). Anything else — a disk-full error, a missing
// UnitID, a rolled-back transaction — must propagate so writes don't silently succeed
// without updating Contents.
func isContentsHashSchemaError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "ContentsHash")
}

// updateTransactionID updates the _Transaction table with a new UUID.
// Studio Pro uses this to detect external changes during F4 sync.
// Only applies to MPR v2 projects (Mendix >= 10.18).
func (w *Writer) updateTransactionID() error {
	if w.reader.version != MPRVersionV2 {
		return nil
	}
	newID := generateUUID()
	_, err := w.reader.db.Exec(`UPDATE _Transaction SET LastTransactionID = ?`, newID)
	return err
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

func contentHashBase64(contents []byte) string {
	hash := sha256.Sum256(contents)
	return base64.StdEncoding.EncodeToString(hash[:])
}

func (w *Writer) insertUnit(unitID, containerID, containmentName, unitType string, contents []byte) error {
	if err := validateNoPlaceholderIDs(unitID, contents); err != nil {
		return err
	}

	// Convert UUID strings to 16-byte blobs for database
	unitIDBlob := uuidToBlob(unitID)
	containerIDBlob := uuidToBlob(containerID)

	if w.reader.version == MPRVersionV2 {
		// Get swapped UUID for file path
		swappedUUID := blobToUUIDSwapped(unitIDBlob)

		// Create directory structure: mprcontents/XX/YY/
		dir := filepath.Join(w.reader.contentsDir, swappedUUID[0:2], swappedUUID[2:4])
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		// Write content file
		filePath := filepath.Join(dir, swappedUUID+".mxunit")
		if err := os.WriteFile(filePath, contents, 0644); err != nil {
			return fmt.Errorf("failed to write unit file: %w", err)
		}

		contentsHash := contentHashBase64(contents)

		// Insert reference to database
		_, err := w.reader.db.Exec(`
			INSERT INTO Unit (UnitID, ContainerID, ContainmentName, TreeConflict, ContentsHash, ContentsConflicts)
			VALUES (?, ?, ?, 0, ?, '')
		`, unitIDBlob, containerIDBlob, containmentName, contentsHash)
		if err != nil {
			// Clean up the file we just wrote — otherwise it becomes an orphan
			os.Remove(filePath)
			return err
		}
		w.reader.InvalidateCache()
		if err := w.updateTransactionID(); err != nil {
			return fmt.Errorf("failed to update transaction ID: %w", err)
		}
		return nil
	}

	// MPR v1: Store directly in database
	contentsHash := contentHashBase64(contents)

	// Try new schema first (without Type column - Mendix 11.6.2+)
	_, err := w.reader.db.Exec(`
		INSERT INTO Unit (UnitID, ContainerID, ContainmentName, TreeConflict, ContentsHash, ContentsConflicts, Contents)
		VALUES (?, ?, ?, 0, ?, '', ?)
	`, unitIDBlob, containerIDBlob, containmentName, contentsHash, contents)
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

func (w *Writer) updateUnit(unitID string, contents []byte) error {
	if err := validateNoPlaceholderIDs(unitID, contents); err != nil {
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

		// Write updated content
		if err := os.WriteFile(filePath, contents, 0644); err != nil {
			return fmt.Errorf("failed to write unit file: %w", err)
		}

		contentsHash := contentHashBase64(contents)
		_, err := w.reader.db.Exec(`
			UPDATE Unit SET ContentsHash = ? WHERE UnitID = ?
		`, contentsHash, unitIDBlob)
		if err == nil {
			w.reader.InvalidateCache()
			if txErr := w.updateTransactionID(); txErr != nil {
				return fmt.Errorf("failed to update transaction ID: %w", txErr)
			}
		}
		return err
	}

	// MPR v1: Update in database
	contentsHash := contentHashBase64(contents)
	_, err := w.reader.db.Exec(`
		UPDATE Unit SET Contents = ?, ContentsHash = ? WHERE UnitID = ?
	`, contents, contentsHash, unitIDBlob)
	if err != nil && isContentsHashSchemaError(err) {
		// Older v1 schemas do not have ContentsHash; retry without it.
		// Any other error (disk full, invalid UnitID, rolled-back tx) propagates.
		_, err = w.reader.db.Exec(`
			UPDATE Unit SET Contents = ? WHERE UnitID = ?
		`, contents, unitIDBlob)
	}
	return err
}

// UpdateRawUnit saves raw BSON bytes for a unit, bypassing deserialization.
// Used by ALTER PAGE to modify the BSON widget tree directly.
func (w *Writer) UpdateRawUnit(unitID string, contents []byte) error {
	return w.updateUnit(unitID, contents)
}

func (w *Writer) deleteUnit(unitID string) error {
	// Convert UUID string to 16-byte blob
	unitIDBlob := uuidToBlob(unitID)
	if unitIDBlob == nil {
		return fmt.Errorf("invalid unit ID: %s", unitID)
	}

	if w.reader.version == MPRVersionV2 {
		// Get swapped UUID for file path
		swappedUUID := blobToUUIDSwapped(unitIDBlob)

		// Delete external file
		subDir1 := swappedUUID[0:2]
		subDir2 := swappedUUID[2:4]
		filePath := filepath.Join(w.reader.contentsDir, subDir1, subDir2, swappedUUID+".mxunit")
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
	if err := w.updateTransactionID(); err != nil {
		return fmt.Errorf("failed to update transaction ID after deleting unit: %w", err)
	}
	return nil
}

func (w *Writer) updateDomainModel(dm *domainmodel.DomainModel) error {
	contents, err := w.serializeDomainModel(dm)
	if err != nil {
		return fmt.Errorf("failed to serialize domain model: %w", err)
	}

	return w.updateUnit(string(dm.ID), contents)
}

// UpdateDomainModel serializes and saves a domain model back to the MPR file.
func (w *Writer) UpdateDomainModel(dm *domainmodel.DomainModel) error {
	return w.updateDomainModel(dm)
}
