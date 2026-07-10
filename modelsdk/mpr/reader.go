// SPDX-License-Identifier: Apache-2.0

// Package mpr provides functionality for reading and writing Mendix project files (.mpr).
package mpr

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr/version"

	_ "modernc.org/sqlite"
)

// MPRVersion represents the MPR file format version.
type MPRVersion int

const (
	// MPRVersionV1 is the original single-file format.
	MPRVersionV1 MPRVersion = 1
	// MPRVersionV2 uses mprcontents folder (Mendix 10.18+).
	MPRVersionV2 MPRVersion = 2
)

// Reader provides methods to read Mendix project files.
type Reader struct {
	path           string
	db             *sql.DB
	version        MPRVersion
	contentsDir    string
	readOnly       bool
	ownsDB         bool
	projectVersion *version.ProjectVersion

	// Cache for unit metadata to avoid repeated file reads
	unitCache      []cachedUnit
	unitCacheValid bool

	// contentCache stores raw BSON bytes per unit ID (MPR v2 only).
	// Populated on first read; survives across requests when the Reader is
	// held persistently by the per-MPR daemon. Cleared by InvalidateCache.
	// nil means caching is disabled (zero cost on the normal per-request path).
	contentCache map[string][]byte

	// overlay holds unit bytes injected by BufferedUnitStore so that reads
	// within the same import file see buffered (uncommitted) writes.
	// nil means no overlay is active — zero cost on the normal path.
	overlay map[string][]byte
}

// cachedUnit stores metadata about a unit for fast filtering.
type cachedUnit struct {
	ID              string
	ContainerID     string
	ContainmentName string
	Type            string
}

// OpenOptions configures how the MPR file is opened.
type OpenOptions struct {
	// ReadOnly opens the database in read-only mode.
	ReadOnly bool
}

// Open opens an MPR file for reading.
func Open(path string) (*Reader, error) {
	return OpenWithOptions(path, OpenOptions{ReadOnly: true})
}

// OpenWithOptions opens an MPR file with the specified options.
func OpenWithOptions(path string, opts OpenOptions) (*Reader, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("mpr file not found: %s", path)
	}

	r := &Reader{
		path:     path,
		readOnly: opts.ReadOnly,
	}

	// Check for MPR v2 (mprcontents folder)
	dir := filepath.Dir(path)
	contentsDir := filepath.Join(dir, "mprcontents")
	if stat, err := os.Stat(contentsDir); err == nil && stat.IsDir() {
		r.version = MPRVersionV2
		r.contentsDir = contentsDir
	} else {
		r.version = MPRVersionV1
	}

	// Open SQLite database
	dsn := path
	if opts.ReadOnly {
		dsn = fmt.Sprintf("file:%s?mode=ro", path)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Limit to single connection to avoid lock contention with SQLite
	db.SetMaxOpenConns(1)

	// Set busy timeout to prevent SQLITE_BUSY errors during multi-statement
	// script execution (e.g., 12+ CREATE PAGE commands in sequence)
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set busy_timeout: %w", err)
	}

	r.db = db
	r.ownsDB = true

	// Detect project version from metadata
	pv, err := version.DetectFromDB(db)
	if err != nil {
		r.Close()
		return nil, fmt.Errorf("failed to detect project version: %w", err)
	}
	r.projectVersion = pv

	// Reconcile version detection: the folder-based check can fail if the .mpr
	// file was copied without the mprcontents/ folder. Check the actual DB schema
	// to determine whether the Unit table has a Contents column. If it doesn't,
	// we must use v2 code paths to avoid "no such column: Contents" errors.
	if r.version == MPRVersionV1 && !r.unitTableHasContents() {
		dir := filepath.Dir(path)
		contentsDir := filepath.Join(dir, "mprcontents")
		r.version = MPRVersionV2
		r.contentsDir = contentsDir

		// The mprcontents/ directory is missing even though the schema is v2.
		// This means the project was opened without its unit files — any write
		// operation would produce an inconsistent project where existing SQLite
		// unit rows have no corresponding .mxunit files on disk.
		// Refuse write access to prevent silent corruption.
		if !opts.ReadOnly {
			var unitCount int
			_ = db.QueryRow("SELECT COUNT(*) FROM Unit").Scan(&unitCount)
			if unitCount > 0 {
				r.Close()
				return nil, fmt.Errorf(
					"mpr: project %q has v2 schema (%d units registered) but mprcontents/ directory is missing at %s; "+
						"restore mprcontents/ before opening for writing, or use mxcli mpr-pack to-v1 to convert to a self-contained file",
					path, unitCount, contentsDir,
				)
			}
		}
	}

	// Verify it's a valid MPR file
	if err := r.verify(); err != nil {
		r.Close()
		return nil, err
	}

	return r, nil
}

// ListUnitHashes returns a map from unit UUID string to its ContentsHash for
// every unit that has a non-empty ContentsHash in the Unit table.
// The key format matches what gen-typed elements return from .ID().
func (r *Reader) ListUnitHashes() (map[string]string, error) {
	rows, err := r.db.Query(`SELECT UnitID, ContentsHash FROM Unit WHERE ContentsHash != ''`)
	if err != nil {
		return nil, fmt.Errorf("query unit hashes: %w", err)
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var blob []byte
		var hash string
		if err := rows.Scan(&blob, &hash); err != nil {
			continue
		}
		if len(blob) == 16 {
			result[blobToUUID(blob)] = hash
		}
	}
	return result, nil
}

// Close closes the reader and releases resources.
func (r *Reader) Close() error {
	if r.db != nil && r.ownsDB {
		return r.db.Close()
	}
	return nil
}

// OpenWithDB creates a Reader that reuses an existing *sql.DB connection.
// The caller owns the db and is responsible for closing it.
// contentsDir should be the mprcontents folder path (v2) or empty string (v1).
func OpenWithDB(db *sql.DB, path, contentsDir string) (*Reader, error) {
	r := &Reader{
		path:     path,
		ownsDB:   false,
		readOnly: false,
	}
	if contentsDir != "" {
		r.version = MPRVersionV2
		r.contentsDir = contentsDir
	} else {
		r.version = MPRVersionV1
	}
	r.db = db
	pv, err := version.DetectFromDB(db)
	if err != nil {
		return nil, fmt.Errorf("detect project version: %w", err)
	}
	r.projectVersion = pv
	if err := r.verify(); err != nil {
		return nil, err
	}
	return r, nil
}

// unitTableHasContents checks whether the Unit table has a Contents column.
// MPR v2 schemas (Mendix 10.18+) drop this column; v1 schemas have it.
func (r *Reader) unitTableHasContents() bool {
	rows, err := r.db.Query("PRAGMA table_info(Unit)")
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue *string
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			continue
		}
		if name == "Contents" {
			return true
		}
	}
	return false
}

// Path returns the path to the MPR file.
func (r *Reader) Path() string {
	return r.path
}

// Version returns the MPR file format version.
func (r *Reader) Version() MPRVersion {
	return r.version
}

// ContentsDir returns the path to the mprcontents directory for v2 format.
// Returns empty string for v1 format.
func (r *Reader) ContentsDir() string {
	return r.contentsDir
}

// DB returns the underlying database connection.
func (r *Reader) DB() *sql.DB {
	return r.db
}

// ListAllUnitIDs returns all unit UUIDs from the Unit table.
func (r *Reader) ListAllUnitIDs() ([]string, error) {
	rows, err := r.db.Query("SELECT UnitID FROM Unit")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var unitID []byte
		if err := rows.Scan(&unitID); err != nil {
			return nil, fmt.Errorf("scanning unit ID: %w", err)
		}
		ids = append(ids, BlobToUUID(unitID))
	}
	return ids, rows.Err()
}

// ProjectVersion returns the Mendix project version information.
func (r *Reader) ProjectVersion() *version.ProjectVersion {
	return r.projectVersion
}

// verify checks that the file is a valid MPR database.
func (r *Reader) verify() error {
	// Check for Unit table which is required
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = 'Unit'").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to query tables: %w", err)
	}
	if count == 0 {
		return errors.New("not a valid MPR file: Unit table not found")
	}
	return nil
}

// GetProjectRootID returns the ID of the project root unit.
// The project root is the unit where UnitID equals ContainerID.
func (r *Reader) GetProjectRootID() (string, error) {
	var unitID []byte
	err := r.db.QueryRow("SELECT UnitID FROM Unit WHERE UnitID = ContainerID").Scan(&unitID)
	if err != nil {
		return "", fmt.Errorf("failed to get project root: %w", err)
	}
	return blobToUUID(unitID), nil
}

// GetMendixVersion returns the Mendix version used to create the project.
func (r *Reader) GetMendixVersion() (string, error) {
	var version string
	// Try new schema first
	err := r.db.QueryRow("SELECT _ProductVersion FROM _MetaData LIMIT 1").Scan(&version)
	if err != nil {
		// Try old schema
		err = r.db.QueryRow("SELECT MendixVersion FROM _MetaData LIMIT 1").Scan(&version)
		if err != nil {
			return "", fmt.Errorf("failed to get Mendix version: %w", err)
		}
	}
	return version, nil
}

// SetOverlay registers in-memory bytes for unitID so that GetRawUnitBytes
// returns them without hitting disk. Called by BufferedUnitStore.Write.
func (r *Reader) SetOverlay(unitID string, data []byte) {
	if r.overlay == nil {
		r.overlay = make(map[string][]byte)
	}
	r.overlay[unitID] = data
}

// ClearOverlay removes a single unitID from the overlay.
func (r *Reader) ClearOverlay(unitID string) {
	delete(r.overlay, unitID)
}

// ClearAllOverlays removes all overlay entries. Called on DisableImportBuffer.
func (r *Reader) ClearAllOverlays() {
	r.overlay = nil
}

// GetRawUnitBytes returns the raw BSON bytes for a unit identified by its UUID string.
func (r *Reader) GetRawUnitBytes(unitID string) ([]byte, error) {
	// Fast path: return buffered bytes injected by BufferedUnitStore if present.
	if len(r.overlay) > 0 {
		if data, ok := r.overlay[unitID]; ok {
			return data, nil
		}
	}
	if r.version == MPRVersionV2 {
		// For v2, we need the swapped UUID to build the file path.
		// The unitID is in standard UUID format; readMprContents expects
		// the swapped format used by blobToUUID during cache building.
		// Convert: UUID string → blob → swapped UUID.
		blob := uuidToBlob(unitID)
		if blob == nil {
			return nil, fmt.Errorf("invalid unit ID: %s", unitID)
		}
		swapped := blobToUUIDSwapped(blob)
		return r.readMprContents(swapped)
	}

	// V1: read from database
	blob := uuidToBlob(unitID)
	if blob == nil {
		return nil, fmt.Errorf("invalid unit ID: %s", unitID)
	}
	var contents []byte
	err := r.db.QueryRow("SELECT Contents FROM Unit WHERE UnitID = ?", blob).Scan(&contents)
	if err != nil {
		return nil, fmt.Errorf("unit not found: %s", unitID)
	}
	return contents, nil
}

// blobToUUID converts a 16-byte blob to a UUID string using Microsoft GUID format.
// The first 3 groups are little-endian (byte-swapped), last 2 groups are big-endian.
// This is the standard format used by Mendix for all UUID representations.
func blobToUUID(blob []byte) string {
	if len(blob) != 16 {
		return hex.EncodeToString(blob)
	}
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		blob[3], blob[2], blob[1], blob[0],
		blob[5], blob[4],
		blob[7], blob[6],
		blob[8], blob[9],
		blob[10], blob[11], blob[12], blob[13], blob[14], blob[15])
}

// blobToUUIDSwapped converts a 16-byte blob to a UUID string using Microsoft GUID format.
// The first 3 groups are little-endian (byte-swapped), last 2 groups are big-endian.
// This is the format used by Mendix for file naming in mprcontents folder.
func blobToUUIDSwapped(blob []byte) string {
	if len(blob) != 16 {
		return hex.EncodeToString(blob)
	}
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		blob[3], blob[2], blob[1], blob[0],
		blob[5], blob[4],
		blob[7], blob[6],
		blob[8], blob[9],
		blob[10], blob[11], blob[12], blob[13], blob[14], blob[15])
}
