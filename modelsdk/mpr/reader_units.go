// SPDX-License-Identifier: Apache-2.0

// Package mpr - Unit listing infrastructure for Reader.
package mpr

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ResolveModuleName walks the container hierarchy upward until it finds a module.
// This is necessary because in MPR v2 projects, documents live inside folders,
// so a document's direct ContainerID is a folder, not the module.
func ResolveModuleName(containerID string, moduleMap map[string]string, containerParent map[string]string) string {
	current := containerID
	for range 20 {
		if name, ok := moduleMap[current]; ok {
			return name
		}
		parent, ok := containerParent[current]
		if !ok || parent == current {
			break
		}
		current = parent
	}
	return ""
}

// BuildContainerParent builds a map of unit ID → parent container ID for hierarchy walking.
func (r *Reader) BuildContainerParent() (map[string]string, error) {
	units, err := r.ListUnits()
	if err != nil {
		return nil, err
	}
	containerParent := make(map[string]string, len(units))
	for _, u := range units {
		containerParent[string(u.ID)] = string(u.ContainerID)
	}
	return containerParent, nil
}

// rawUnit holds raw unit data from the database.
type rawUnit struct {
	ID              string
	ContainerID     string
	ContainmentName string
	Type            string
	Contents        []byte
}

// UnitRef holds unit metadata returned by ListUnitsByType.
type UnitRef struct {
	ID          string
	ContainerID string
	Type        string // BSON $Type (e.g. "Microflows$Microflow")
	Contents    []byte
}

// ListUnitsByType returns all units matching the given BSON $Type prefix.
// This is the exported version for use by TreeWriter and other packages.
func (r *Reader) ListUnitsByType(typePrefix string) ([]UnitRef, error) {
	units, err := r.listUnitsByType(typePrefix)
	if err != nil {
		return nil, err
	}
	result := make([]UnitRef, len(units))
	for i, u := range units {
		result[i] = UnitRef{ID: u.ID, ContainerID: u.ContainerID, Type: u.Type, Contents: u.Contents}
	}
	return result, nil
}

// UnitHeaderRef is a unit's identity and its top-level Name and Excluded.
type UnitHeaderRef struct {
	ID          string
	ContainerID string
	Name        string
	Excluded    bool
}

// ListUnitHeaders lists the units of exactly typeName with their Name and
// Excluded. On MPR v2 these come from the unit index, so a unit whose bytes the
// index has seen is not opened: naming every folder or microflow used to open
// every one of their files on each call (ako/mxcli#1791). MPR v1 reads the
// blobs, which live in the one database file.
func (r *Reader) ListUnitHeaders(typeName string) ([]UnitHeaderRef, error) {
	return r.ListUnitHeadersWhere(typeName, nil)
}

// ListUnitHeadersWhere is ListUnitHeaders for the units whose Name keep
// accepts (nil accepts all), in the same order. A lookup of one document by
// name is a filter over the names; listing every header of the type and
// discarding all but one copied the header of every microflow in the project
// several times per statement — half of what a `create or modify microflow`
// allocated in a project of 5,000 of them (ako/mxcli#2160).
func (r *Reader) ListUnitHeadersWhere(typeName string, keep func(name string) bool) ([]UnitHeaderRef, error) {
	out, err := r.listStoredUnitHeaders(typeName, keep)
	if err != nil {
		return nil, err
	}
	for _, u := range r.packagedUnitsOf(typeName, "", nil) {
		e := unitIndexEntryOf(u.Contents)
		if keep != nil && !keep(e.Name) {
			continue
		}
		out = append(out, UnitHeaderRef{ID: u.ID, ContainerID: u.ContainerID, Name: e.Name, Excluded: e.Excluded})
	}
	return out, nil
}

// listStoredUnitHeaders is ListUnitHeadersWhere for the units the .mpr stores.
func (r *Reader) listStoredUnitHeaders(typeName string, keep func(name string) bool) ([]UnitHeaderRef, error) {
	if r.version != MPRVersionV2 {
		all, err := r.listUnitHeadersV1(typeName)
		if err != nil || keep == nil {
			return all, err
		}
		var out []UnitHeaderRef
		for _, h := range all {
			if keep(h.Name) {
				out = append(out, h)
			}
		}
		return out, nil
	}
	if !r.unitCacheValid {
		if err := r.buildUnitCache(); err != nil {
			return nil, err
		}
	}
	var out []UnitHeaderRef
	for _, cu := range r.unitCache {
		if cu.Type != typeName {
			continue
		}
		name, excluded := cu.Name, cu.Excluded
		if held, ok := r.overlaid(cu.ID); ok {
			// Held after the cache was built: the held bytes are the unit now.
			e := unitIndexEntryOf(held)
			if e.Type != typeName {
				continue
			}
			name, excluded = e.Name, e.Excluded
		}
		if keep != nil && !keep(name) {
			continue
		}
		out = append(out, UnitHeaderRef{ID: cu.ID, ContainerID: cu.ContainerID, Name: name, Excluded: excluded})
	}
	return out, nil
}

// UnitByID returns the unit whose ID is id, reading that unit's contents only.
// ok is false when the project has no such unit. A unit's ID is its
// document's $ID, so a lookup of one document by ID needs no listing: the
// catalog's source build described each microflow through a listing of them
// all and opened millions of files on a large project (ako/mxcli#1789).
func (r *Reader) UnitByID(id string) (unit UnitRef, ok bool, err error) {
	if u, ok := r.packagedUnit(id); ok {
		return UnitRef{ID: u.ID, ContainerID: u.ContainerID, Type: u.Type, Contents: u.Contents}, true, nil
	}
	if r.version != MPRVersionV2 {
		return r.unitByIDV1(id)
	}
	if !r.unitCacheValid {
		if err := r.buildUnitCache(); err != nil {
			return UnitRef{}, false, err
		}
	}
	for _, cu := range r.unitCache {
		if cu.ID != id {
			continue
		}
		contents, err := r.readMprContents(cu.ID)
		if err != nil {
			return UnitRef{}, false, err
		}
		return UnitRef{ID: cu.ID, ContainerID: cu.ContainerID, Type: cu.Type, Contents: contents}, true, nil
	}
	return UnitRef{}, false, nil
}

// unitByIDV1 is UnitByID on MPR v1, where the contents are a column.
func (r *Reader) unitByIDV1(id string) (UnitRef, bool, error) {
	blob := uuidToBlob(id)
	if blob == nil {
		return UnitRef{}, false, nil
	}
	var containerID, contents []byte
	err := r.db.QueryRow("SELECT ContainerID, Contents FROM Unit WHERE UnitID = ?", blob).Scan(&containerID, &contents)
	if errors.Is(err, sql.ErrNoRows) {
		return UnitRef{}, false, nil
	}
	if err != nil {
		return UnitRef{}, false, err
	}
	if held, ok := r.overlaid(id); ok {
		contents = held
	}
	return UnitRef{ID: id, ContainerID: blobToUUID(containerID), Type: getTypeFromContents(contents), Contents: contents}, true, nil
}

// ListUnitsByTypeInContainer returns the units of exactly typeName whose
// ContainerID is containerID, reading the contents of those units only: a
// lookup of one module's domain model must not read every module's
// (mendixlabs/mxcli#1616).
func (r *Reader) ListUnitsByTypeInContainer(typeName, containerID string) ([]UnitRef, error) {
	var units []rawUnit
	var err error
	if r.version == MPRVersionV2 {
		units, err = r.listUnitsByTypeV2In(typeName, containerID)
	} else {
		units, err = r.listUnitsByTypeV1In(typeName, containerID)
	}
	if err != nil {
		return nil, err
	}
	units = append(units, r.packagedUnitsOf(typeName, containerID, nil)...)
	result := make([]UnitRef, len(units))
	for i, u := range units {
		result[i] = UnitRef{ID: u.ID, ContainerID: u.ContainerID, Type: u.Type, Contents: u.Contents}
	}
	return result, nil
}

// ListUnitsByTypeWhere is ListUnitsByType for the units whose ID keep
// accepts. On v2 the others' contents are never read: a catalog build that
// rebuilds a few units reads only those (ako/mxcli#1804).
func (r *Reader) ListUnitsByTypeWhere(typeName string, keep func(id string) bool) ([]UnitRef, error) {
	units, err := r.listUnitsByTypeWhere(typeName, keep)
	if err != nil {
		return nil, err
	}
	result := make([]UnitRef, len(units))
	for i, u := range units {
		result[i] = UnitRef{ID: u.ID, ContainerID: u.ContainerID, Type: u.Type, Contents: u.Contents}
	}
	return result, nil
}

// listUnitsByTypeWhere is listUnitsByType for the units keep accepts (nil
// keeps all), filtered before any v2 contents are read.
func (r *Reader) listUnitsByTypeWhere(typeName string, keep func(id string) bool) ([]rawUnit, error) {
	if keep == nil {
		return r.listUnitsByType(typeName)
	}
	packaged := r.packagedUnitsOf(typeName, "", keep)
	if r.version == MPRVersionV2 {
		units, err := r.listUnitsByTypeV2Where(typeName, "", keep)
		if err != nil {
			return nil, err
		}
		return append(units, packaged...), nil
	}
	units, err := r.listUnitsByTypeV1(typeName)
	if err != nil {
		return nil, err
	}
	kept := units[:0]
	for _, u := range units {
		if keep(u.ID) {
			kept = append(kept, u)
		}
	}
	return append(kept, packaged...), nil
}

// listUnitsByType returns all units of exactly the given storage type. An empty
// typeName returns every unit.
//
// Exact, not prefix: Mendix storage names nest (`Forms$Page` is a prefix of
// `Forms$PageTemplate`), so a prefix match silently folds one document type into
// another. See the note on the same function in sdk/mpr.
//
// The units of the project's module packages (packaged_modules.go) are listed
// with the stored ones.
func (r *Reader) listUnitsByType(typeName string) ([]rawUnit, error) {
	units, err := r.listStoredUnitsByType(typeName)
	if err != nil {
		return nil, err
	}
	return append(units, r.packagedUnitsOf(typeName, "", nil)...), nil
}

// listStoredUnitsByType is listUnitsByType for the units the .mpr stores.
func (r *Reader) listStoredUnitsByType(typeName string) ([]rawUnit, error) {
	if r.version == MPRVersionV2 {
		return r.listUnitsByTypeV2(typeName)
	}
	return r.listUnitsByTypeV1(typeName)
}

// listUnitTypes returns every unit's metadata and $Type without reading the
// contents of any unit whose type is already indexed (Contents is nil): on v2
// the index is keyed by ContentsHash and persisted (reader_units_v2.go), on v1
// by blob length (reader_units_v1.go). For callers that need the project's
// shape, not its documents.
func (r *Reader) listUnitTypes() ([]rawUnit, error) {
	if r.version != MPRVersionV2 {
		units, err := r.listUnitTypesV1()
		if err != nil {
			return nil, err
		}
		return append(units, r.packagedUnitsOf("", "", nil)...), nil
	}
	if !r.unitCacheValid {
		if err := r.buildUnitCache(); err != nil {
			return nil, err
		}
	}
	units := make([]rawUnit, 0, len(r.unitCache))
	for _, cu := range r.unitCache {
		units = append(units, rawUnit{ID: cu.ID, ContainerID: cu.ContainerID, ContainmentName: cu.ContainmentName, Type: cu.Type})
	}
	return append(units, r.packagedUnitsOf("", "", nil)...), nil
}

// listUnitsByTypeV2 handles MPR v2 format (contents in mprcontents folder).
// Uses caching to avoid reading every file for each query.
func (r *Reader) listUnitsByTypeV2(typeName string) ([]rawUnit, error) {
	return r.listUnitsByTypeV2In(typeName, "")
}

// listUnitsByTypeV2In is listUnitsByTypeV2 restricted to one container ("" for
// any), filtered before any contents are read.
func (r *Reader) listUnitsByTypeV2In(typeName, containerID string) ([]rawUnit, error) {
	return r.listUnitsByTypeV2Where(typeName, containerID, nil)
}

// ListUnitsByTypeShared is ListUnitsByTypeWhere for a caller that only reads
// what it is given: on MPR v2, while the content cache is held, a unit's
// Contents is the cache's own slice rather than a copy of it. The caller must
// not modify it.
//
// It is for mprread's decoder, which is on the path of every listing. A copy
// per unit per listing was a tenth of a script's time once the listings
// themselves were cached, and most of what the garbage collector then had to
// clear (ako/mxcli#2160). Handing out the same slice each time also lets the
// decode cache see that a unit is unchanged without comparing its bytes.
func (r *Reader) ListUnitsByTypeShared(typeName string, keep func(id string) bool) ([]UnitRef, error) {
	if r.version != MPRVersionV2 {
		return r.ListUnitsByTypeWhere(typeName, keep)
	}
	units, err := r.listUnitsByTypeV2Shared(typeName, "", keep, true)
	if err != nil {
		return nil, err
	}
	units = append(units, r.packagedUnitsOf(typeName, "", keep)...)
	result := make([]UnitRef, len(units))
	for i, u := range units {
		result[i] = UnitRef{ID: u.ID, ContainerID: u.ContainerID, Type: u.Type, Contents: u.Contents}
	}
	return result, nil
}

// listUnitsByTypeV2Where is listUnitsByTypeV2In for the units keep accepts
// (nil keeps all).
func (r *Reader) listUnitsByTypeV2Where(typeName, containerID string, keep func(id string) bool) ([]rawUnit, error) {
	return r.listUnitsByTypeV2Shared(typeName, containerID, keep, false)
}

// listUnitsByTypeV2Shared is listUnitsByTypeV2Where; with shared, contents
// served from the content cache are not copied (see ListUnitsByTypeShared).
func (r *Reader) listUnitsByTypeV2Shared(typeName, containerID string, keep func(id string) bool, shared bool) ([]rawUnit, error) {
	if !r.unitCacheValid {
		if err := r.buildUnitCache(); err != nil {
			return nil, err
		}
	}

	// Filter by type using cache, only read contents for matching units.
	var units []rawUnit
	for _, cu := range r.unitCache {
		if containerID != "" && cu.ContainerID != containerID {
			continue
		}
		if keep != nil && !keep(cu.ID) {
			continue
		}
		if typeName == "" || cu.Type == typeName {
			contents, err := r.readMprContentsShared(cu.ID, shared)
			if err != nil {
				continue
			}
			units = append(units, rawUnit{
				ID:              cu.ID,
				ContainerID:     cu.ContainerID,
				ContainmentName: cu.ContainmentName,
				Type:            cu.Type,
				Contents:        contents,
			})
		}
	}
	return units, nil
}

// buildUnitCache reads all unit metadata once and caches it. A unit's $Type
// comes from the type index when its ContentsHash is known there, so only new
// and changed units are opened (reader_units_v2.go, mendixlabs/mxcli#1603).
func (r *Reader) buildUnitCache() error {
	r.unitIndexBuilds.Add(1)
	r.announceColdUnitIndex()
	rows, err := r.db.Query(`
		SELECT UnitID, ContainerID, ContainmentName, COALESCE(ContentsHash, '')
		FROM Unit
	`)
	if err != nil {
		return fmt.Errorf("failed to query units: %w", err)
	}
	defer rows.Close()

	r.v2TypesMu.Lock()
	defer r.v2TypesMu.Unlock()

	r.unitCache = nil
	hashes := make(map[string]struct{})
	store := r.unitStore()
	rowHashes := make(map[string]string)
	opened := 0
	for rows.Next() {
		var unitID, containerID []byte
		var containmentName, hash string

		if err := rows.Scan(&unitID, &containerID, &containmentName, &hash); err != nil {
			return fmt.Errorf("failed to scan unit row: %w", err)
		}

		unitUUID := blobToUUID(unitID)
		rowHashes[unitUUID] = hash
		if store != nil {
			store.noteRowHash(unitUUID, hash)
		}
		var entry unitIndexEntry
		if held, ok := r.overlaid(unitUUID); ok {
			// Bytes held in memory are what the unit is now; its stored hash
			// describes the file it will replace.
			entry = unitIndexEntryOf(held)
		} else if known, ok := r.knownUnit(hash); ok {
			entry = known
		} else {
			contents, err := r.readMprContentsUnder(unitUUID, hash, true, false)
			if err != nil {
				continue
			}
			opened++
			entry = unitIndexEntryOf(contents)
			r.learnUnit(contentsHash(contents), entry)
		}
		if hash != "" {
			hashes[hash] = struct{}{}
		}
		r.unitCache = append(r.unitCache, cachedUnit{
			ID:              unitUUID,
			ContainerID:     blobToUUID(containerID),
			ContainmentName: containmentName,
			Type:            entry.Type,
			Name:            entry.Name,
			Excluded:        entry.Excluded,
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to scan units: %w", err)
	}

	if store != nil {
		store.setRowHashes(rowHashes)
	}
	r.contentMu.Lock()
	r.rowHashes = rowHashes
	r.contentMu.Unlock()
	first := r.v2Hashes == nil
	r.v2Hashes = hashes
	if first || opened >= unitTypeIndexPersistAfter {
		r.persistUnitTypeIndex()
	}
	r.unitCacheValid = true
	return nil
}

// InvalidateCache marks the unit cache as invalid. Should be called after any
// write operation.
//
// The content cache is not emptied. Each entry is valid for the ContentsHash
// it was read under, and a write changes the row hash of the unit it rewrote
// and of no other; emptying the cache made every listing after every write
// read every unit file of its type again (ako/mxcli#2160). What is dropped is
// the hashes: until the next buildUnitCache reads them back, no entry is known
// to be current, so none is served.
func (r *Reader) InvalidateCache() {
	r.unitCacheValid = false
	r.invalidateV1Listing()
	r.clearNameIndex()
	r.contentMu.Lock()
	r.rowHashes = nil
	r.contentMu.Unlock()
}

// noteUnitWritten brings the reader's list of units up to date with a write
// the writer has just committed to an MPR v2 project: a unit inserted, or an
// existing unit's contents replaced. It does for one row what InvalidateCache
// followed by buildUnitCache does for all of them.
//
// Every write used to mark the list stale, and the next read scanned the whole
// Unit table to rebuild it: once per statement of a script, for a write that
// added or changed one row (ako/mxcli#2160).
//
// Anything it cannot account for falls back to InvalidateCache: a list that is
// already stale, an insert of a unit the list already has, an update of one it
// does not. Deletes, moves and transactions are not handled here at all, and
// invalidate as before.
func (r *Reader) noteUnitWritten(unitIDBlob, containerIDBlob []byte, containmentName string, contents []byte, hash string, inserted bool) {
	if r.version != MPRVersionV2 || !r.unitCacheValid {
		r.InvalidateCache()
		return
	}
	id := blobToUUID(unitIDBlob)
	entry := unitIndexEntryOf(contents)

	r.v2TypesMu.Lock()
	at := -1
	for i := range r.unitCache {
		if r.unitCache[i].ID == id {
			at = i
			break
		}
	}
	if inserted == (at >= 0) || r.v2Hashes == nil {
		r.v2TypesMu.Unlock()
		r.InvalidateCache()
		return
	}
	if inserted {
		r.unitCache = append(r.unitCache, cachedUnit{
			ID:              id,
			ContainerID:     blobToUUID(containerIDBlob),
			ContainmentName: containmentName,
			Type:            entry.Type,
			Name:            entry.Name,
			Excluded:        entry.Excluded,
		})
	} else {
		cu := &r.unitCache[at]
		cu.Type, cu.Name, cu.Excluded = entry.Type, entry.Name, entry.Excluded
	}
	r.learnUnit(hash, entry)
	if hash != "" {
		r.v2Hashes[hash] = struct{}{}
	}
	r.v2TypesMu.Unlock()

	r.contentMu.Lock()
	known := r.rowHashes != nil
	if known {
		r.rowHashes[id] = hash
	}
	r.contentMu.Unlock()
	if !known {
		r.InvalidateCache()
		return
	}
	if store := r.unitStore(); store != nil {
		store.noteRowHash(id, hash)
	}
	// What InvalidateCache drops besides the list: a name may now be found
	// somewhere else.
	r.clearNameIndex()
}

// EnableContentCache activates the in-memory content cache for this reader.
// Call once after Connect in persistent daemon mode. The cache survives across
// requests; InvalidateCache empties it (but keeps caching active) on writes.
func (r *Reader) EnableContentCache() {
	r.contentMu.Lock()
	defer r.contentMu.Unlock()
	if r.contentCache == nil {
		r.contentCache = make(map[string]cachedContent)
	}
	r.cacheScopes = -1
}

// CacheUnitReads holds every unit read from mprcontents/ in memory until
// release is called, so a pass that looks documents up again and again reads
// each file once (mendixlabs/mxcli#1272: check -p re-read every microflow of
// the project several times per `create or modify` it predicted). Scopes
// nest; the cache is dropped when the outermost one is released, so it never
// outlives the pass — a long session must not keep serving bytes Studio Pro
// has since rewritten. A write through this reader retires the unit it
// rewrote: an entry is served only while its unit's row hash is the one it
// was read under.
//
// MPR v1 reads its contents from SQLite, which caches its own pages; there
// the scope changes nothing.
func (r *Reader) CacheUnitReads() (release func()) {
	r.contentMu.Lock()
	defer r.contentMu.Unlock()
	if r.cacheScopes < 0 {
		return func() {} // cached for the reader's lifetime
	}
	if r.contentCache == nil {
		r.contentCache = make(map[string]cachedContent)
	}
	r.cacheScopes++
	var once sync.Once
	return func() {
		once.Do(func() {
			r.contentMu.Lock()
			last := false
			if r.cacheScopes > 0 {
				if r.cacheScopes--; r.cacheScopes == 0 {
					r.contentCache = nil
					last = true
				}
			}
			r.contentMu.Unlock()
			// A pass over the project has just read what it needed: keep it
			// for the next process (reader_unit_store.go).
			if store := r.unitStore(); last && store != nil {
				store.flush()
			}
		})
	}
}

// UnitFileReads is how many unit files this reader has read from
// mprcontents/. For tests.
func (r *Reader) UnitFileReads() int64 { return r.fileReads.Load() }

// UnitIndexBuilds is how many times this reader has scanned the Unit table to
// rebuild its list of units. For tests.
func (r *Reader) UnitIndexBuilds() int64 { return r.unitIndexBuilds.Load() }

// UnitContentCopies is how many times this reader copied a unit's bytes out
// of its content cache for a caller. For tests.
func (r *Reader) UnitContentCopies() int64 { return r.contentCopies.Load() }

// UnitStoreHits is how many unit reads this reader served from the persisted
// store instead of a file. For tests.
func (r *Reader) UnitStoreHits() int64 {
	if store := r.unitStore(); store != nil {
		store.mu.Lock()
		defer store.mu.Unlock()
		return store.hits
	}
	return 0
}

// unitStore is the reader's persisted unit store, or nil when there is none:
// MPR v1 holds its units in the .mpr, and a reader without a project path
// has nowhere to keep one.
func (r *Reader) unitStore() *unitStore {
	r.storeOnce.Do(func() {
		if r.version == MPRVersionV2 && r.contentsDir != "" {
			r.store = newUnitStore(r.path)
		}
	})
	return r.store
}

// readMprContents reads content from the mprcontents folder for v2 format.
// The path is: mprcontents/XX/YY/UUID.mxunit where XX and YY are first two chars of UUID.
//
// While the content cache is on (EnableContentCache, CacheUnitReads) a unit is
// read from disk once and served from memory after that, as a copy: every
// caller gets bytes of its own, as os.ReadFile gave it, so one that edits them
// cannot change what the next caller reads.
func (r *Reader) readMprContents(unitUUID string) ([]byte, error) {
	return r.readMprContentsShared(unitUUID, false)
}

// readMprContentsShared is readMprContents; with shared, bytes served from
// the content cache are the cache's own slice, which the caller must not
// modify (see ListUnitsByTypeShared).
func (r *Reader) readMprContentsShared(unitUUID string, shared bool) ([]byte, error) {
	if len(unitUUID) < 4 {
		return nil, fmt.Errorf("invalid unit UUID: %s", unitUUID)
	}

	// Bytes held in memory (an import buffer, or a deferred run of writes) are
	// what the unit currently is; the listings read through here too, so a
	// held write is seen by every read, not only by GetRawUnitBytes.
	if data, ok := r.overlaid(unitUUID); ok {
		return data, nil
	}

	r.contentMu.Lock()
	rowHash, known := r.rowHashes[unitUUID]
	r.contentMu.Unlock()
	return r.readMprContentsUnder(unitUUID, rowHash, known, shared)
}

// readMprContentsUnder is readMprContents for a caller that has the unit's
// row hash in hand (buildUnitCache, which is reading the Unit table and has
// not published its hashes yet). rowHash is the only key the read may be
// cached or served under; known is false when the unit's current hash is not
// known, and the read then goes to the file and is not kept. An empty hash is
// a key like any other: a row without one gets one from the first write.
// With shared, what is returned while caching is the slice the cache holds.
func (r *Reader) readMprContentsUnder(unitUUID, rowHash string, known, shared bool) ([]byte, error) {
	r.contentMu.Lock()
	caching := r.contentCache != nil && known
	cached, ok := r.contentCache[unitUUID]
	r.contentMu.Unlock()
	if ok && caching && cached.hash == rowHash {
		if shared {
			return cached.data, nil
		}
		r.contentCopies.Add(1)
		return bytes.Clone(cached.data), nil
	}

	// Build path: mprcontents/XX/YY/UUID.mxunit
	path := filepath.Join(
		r.contentsDir,
		unitUUID[0:2],
		unitUUID[2:4],
		unitUUID+".mxunit",
	)
	// An unchanged unit is read from the persisted store rather than its
	// own file, which on Windows is what a virus scanner inspects (#1804).
	store := r.unitStore()
	var fi os.FileInfo
	if store != nil {
		stored, info, ok := store.lookup(unitUUID, path)
		if ok {
			if caching {
				held := stored
				if !shared {
					held = bytes.Clone(stored)
				}
				r.contentMu.Lock()
				if r.contentCache != nil {
					r.contentCache[unitUUID] = cachedContent{hash: rowHash, data: held}
				}
				r.contentMu.Unlock()
			}
			return stored, nil
		}
		fi = info
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r.fileReads.Add(1)

	// One copy serves both the in-memory cache and the store: neither hands
	// out the bytes it holds, only copies of them.
	held := bytes.Clone(data)
	if caching {
		r.contentMu.Lock()
		if r.contentCache != nil {
			r.contentCache[unitUUID] = cachedContent{hash: rowHash, data: held}
		}
		r.contentMu.Unlock()
	}
	if store != nil {
		store.record(unitUUID, fi, held)
	}
	if shared && caching {
		return held, nil
	}
	return data, nil
}

// contentsHash is a unit's ContentsHash as Studio Pro and the writer compute
// it: base64(SHA-256(contents)).
func contentsHash(contents []byte) string {
	sum := sha256.Sum256(contents)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// getTypeFromContents extracts the $Type field from BSON contents.
// Uses bson.Raw.LookupErr for O(1) field extraction instead of unmarshalling
// the entire document into map[string]any.
func getTypeFromContents(contents []byte) string {
	if len(contents) == 0 {
		return ""
	}
	val, err := bson.Raw(contents).LookupErr("$Type")
	if err != nil {
		return ""
	}
	s, ok := val.StringValueOK()
	if !ok {
		return ""
	}
	return s
}

// RawUnitInfo contains information about a raw unit for BSON debugging.
// Aliased to mdl/types.RawUnitInfo so reader_raw.go methods and modelsdk/codec
// consumers share a single concrete struct.
type RawUnitInfo = types.RawUnitInfo
