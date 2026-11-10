// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

// Typed listings on MPR v1.
//
// A v1 project keeps every unit's contents in the Unit.Contents blob, and the
// table has no type column: a unit's $Type is only known from its BSON. The
// listing used to read every blob in the file and filter afterwards, so each
// ListMicroflows, ListModules, ListConsumedRestServices… cost a full read of
// the project — 140 MB on a large app, fourteen times in one `describe
// microflow`. MPR v2 never paid this, because buildUnitCache indexes the types
// once and listUnitsByTypeV2 then reads only the matching files.
//
// This is the v1 counterpart of that index. A listing reads the cheap columns
// of every row (ID, container, containment name, length(Contents) — the length
// comes from the record header, no overflow page is touched — and
// ContentsHash) and re-types any unit that is new or whose length moved. A
// unit's $Type is fixed for its lifetime in Mendix, so the length is a
// belt-and-braces change detector, not the thing correctness rests on.
//
// That scan is kept until the table may have changed (unitMetasV1): a write
// through this reader invalidates it, and a commit by any other connection —
// another process, Studio Pro — moves SQLite's data_version. Scanning on every
// listing instead cost 0.3–0.9 s per `exec` statement on a 3,400-unit project,
// because one statement makes dozens of listings (ako/mxcli#2094).
//
// Name and Excluded are kept per unit too (listUnitHeadersV1), keyed by the
// row's length and ContentsHash, which every Studio Pro and mxcli write
// updates. A unit without a hash is read every time, and a commit from another
// connection drops every kept name, since nothing obliges another writer to
// update the hash.
//
// The type itself is peeked from the blob's first v1TypePeekBytes bytes:
// Studio Pro writes `$ID` then `$Type` at the head of every unit. A document
// that does not carry `$Type` within the prefix is read in full.

// v1TypePeekBytes is how much of a unit's BSON is read to find its $Type.
const v1TypePeekBytes = 256

// v1StaleRescan is the number of untyped units above which the index is
// refreshed with one table scan instead of one point query per unit.
const v1StaleRescan = 64

type v1TypeEntry struct {
	typ  string
	size int64
}

// v1UnitMeta is one Unit row without its contents. IDs stay as blobs until a
// unit is returned: formatting a UUID costs more than the rest of the row, and
// a typed listing returns a small fraction of the rows it inspects.
type v1UnitMeta struct {
	blob            []byte
	key             string // string(blob): the type index key
	containerBlob   []byte
	containmentName string
	size            int64  // -1 for NULL contents
	hash            string // ContentsHash; "" when NULL or the column is absent
	held            []byte
	overlaid        bool
}

// listUnitsByTypeV1 handles MPR v1 format (contents in database).
func (r *Reader) listUnitsByTypeV1(typeName string) ([]rawUnit, error) {
	if typeName == "" {
		return r.listAllUnitsV1()
	}
	return r.listUnitsByTypeV1In(typeName, "")
}

// listUnitsByTypeV1In is listUnitsByTypeV1 for a non-empty typeName,
// restricted to one container ("" for any) before any blob is read.
func (r *Reader) listUnitsByTypeV1In(typeName, containerID string) ([]rawUnit, error) {
	if typeName == "" {
		return nil, fmt.Errorf("listUnitsByTypeV1In: empty type name")
	}

	r.v1TypesMu.Lock()
	defer r.v1TypesMu.Unlock()
	metas, err := r.unitMetasV1()
	if err != nil {
		return nil, err
	}
	if err := r.refreshTypeIndexV1(metas); err != nil {
		return nil, err
	}

	var stmt *sql.Stmt
	defer func() {
		if stmt != nil {
			_ = stmt.Close()
		}
	}()
	var units []rawUnit
	// Formatting a UUID costs more than the rest of the row, so the container
	// is compared only for units of the wanted type.
	inContainer := func(m v1UnitMeta) bool {
		return containerID == "" || blobToUUID(m.containerBlob) == containerID
	}
	for _, m := range metas {
		var contents []byte
		var unitType string
		if m.overlaid {
			contents = m.held
			unitType = getTypeFromContents(contents)
			if unitType != typeName || !inContainer(m) {
				continue
			}
		} else {
			unitType = r.v1Types[m.key].typ
			if unitType != typeName || !inContainer(m) {
				continue
			}
			if stmt == nil {
				if stmt, err = r.db.Prepare(`SELECT Contents FROM Unit WHERE UnitID = ?`); err != nil {
					return nil, fmt.Errorf("failed to prepare unit read: %w", err)
				}
			}
			if err := stmt.QueryRow(m.blob).Scan(&contents); err != nil {
				if err == sql.ErrNoRows {
					continue // deleted since the metadata scan
				}
				return nil, fmt.Errorf("failed to read unit %s: %w", blobToUUID(m.blob), err)
			}
			r.blobReads.Add(1)
			// The row may have changed between the metadata scan and this read;
			// type what was actually read, as the full scan did.
			if int64(len(contents)) != m.size {
				unitType = getTypeFromContents(contents)
				r.v1Types[m.key] = v1TypeEntry{typ: unitType, size: int64(len(contents))}
				if unitType != typeName {
					continue
				}
			}
		}
		units = append(units, rawUnit{
			ID:              blobToUUID(m.blob),
			ContainerID:     blobToUUID(m.containerBlob),
			ContainmentName: m.containmentName,
			Type:            unitType,
			Contents:        contents,
		})
	}
	return units, nil
}

// listUnitTypesV1 returns every unit's metadata and type without its
// contents (Contents is nil), in table order.
func (r *Reader) listUnitTypesV1() ([]rawUnit, error) {
	r.v1TypesMu.Lock()
	defer r.v1TypesMu.Unlock()
	metas, err := r.unitMetasV1()
	if err != nil {
		return nil, err
	}
	if err := r.refreshTypeIndexV1(metas); err != nil {
		return nil, err
	}
	units := make([]rawUnit, 0, len(metas))
	for _, m := range metas {
		typ := r.v1Types[m.key].typ
		if m.overlaid {
			typ = getTypeFromContents(m.held)
		}
		units = append(units, rawUnit{ID: blobToUUID(m.blob), ContainerID: blobToUUID(m.containerBlob), ContainmentName: m.containmentName, Type: typ})
	}
	return units, nil
}

// listAllUnitsV1 returns every unit with its contents: an unfiltered listing
// needs every blob anyway, so it reads them in one scan.
func (r *Reader) listAllUnitsV1() ([]rawUnit, error) {
	rows, err := r.db.Query(`
		SELECT UnitID, ContainerID, ContainmentName, Contents
		FROM Unit
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query units: %w", err)
	}
	defer rows.Close()

	var units []rawUnit
	for rows.Next() {
		var unitID, containerID []byte
		var containmentName sql.NullString
		var contents []byte

		if err := rows.Scan(&unitID, &containerID, &containmentName, &contents); err != nil {
			return nil, fmt.Errorf("failed to scan unit row: %w", err)
		}
		r.blobReads.Add(1)

		if held, ok := r.overlaid(blobToUUID(unitID)); ok {
			contents = held
		}
		units = append(units, rawUnit{
			ID:              blobToUUID(unitID),
			ContainerID:     blobToUUID(containerID),
			ContainmentName: containmentName.String,
			Type:            getTypeFromContents(contents),
			Contents:        contents,
		})
	}
	return units, rows.Err()
}

// v1Stamp is what says the Unit table may have changed since a scan.
// dataVersion is SQLite's PRAGMA data_version, which moves when another
// connection commits to the file; the reader holds one connection
// (SetMaxOpenConns(1)), so its own writes do not move it and invalidate
// explicitly instead. The file's size and modification time are a second
// signal, for a write made on this connection without that invalidation.
type v1Stamp struct {
	dataVersion int64
	size        int64
	modNanos    int64
}

// v1StampNow reads the current stamp; ok is false when it cannot be read, and
// the caller then scans as if the table had changed.
func (r *Reader) v1StampNow() (v1Stamp, bool) {
	var s v1Stamp
	if err := r.db.QueryRow(`PRAGMA data_version`).Scan(&s.dataVersion); err != nil {
		return v1Stamp{}, false
	}
	s.size, s.modNanos = -1, -1
	if r.path != "" {
		if fi, err := os.Stat(r.path); err == nil {
			s.size, s.modNanos = fi.Size(), fi.ModTime().UnixNano()
		}
	}
	return s, true
}

// invalidateV1Listing marks the kept Unit table scan stale after a write
// through this reader. The kept names survive: they are checked against each
// row's length and hash, which every write path updates.
func (r *Reader) invalidateV1Listing() {
	r.v1TypesMu.Lock()
	r.v1MetasValid = false
	r.v1TypesMu.Unlock()
}

// unitMetasV1 is every unit's metadata in table order: the kept scan while the
// table cannot have changed, else a fresh one. Overlaid units carry their held
// bytes. The returned slice is the caller's. Caller holds v1TypesMu.
func (r *Reader) unitMetasV1() ([]v1UnitMeta, error) {
	stamp, stampOK := r.v1StampNow()
	if !r.v1MetasValid || !stampOK || stamp != r.v1MetasStamp {
		if r.v1MetasValid && stamp.dataVersion != r.v1MetasStamp.dataVersion {
			// Another connection committed: its writes need not have moved
			// ContentsHash, so no kept name can be trusted.
			clear(r.v1Headers)
		}
		metas, err := r.scanUnitMetaV1()
		if err != nil {
			r.v1MetasValid = false
			return nil, err
		}
		r.v1Metas, r.v1MetasStamp, r.v1MetasValid = metas, stamp, stampOK
	}
	out := make([]v1UnitMeta, len(r.v1Metas))
	copy(out, r.v1Metas)
	if len(r.overlay) > 0 {
		for i := range out {
			out[i].held, out[i].overlaid = r.overlaid(blobToUUID(out[i].blob))
		}
	}
	return out, nil
}

// scanUnitMetaV1 reads every unit's metadata, without its contents or any
// overlay, in table order (the order the full scan returned units in).
func (r *Reader) scanUnitMetaV1() ([]v1UnitMeta, error) {
	r.metaScans.Add(1)
	hashCol := "ContentsHash"
	if r.v1NoHashColumn {
		hashCol = "NULL"
	}
	rows, err := r.db.Query(`SELECT UnitID, ContainerID, ContainmentName, length(Contents), ` + hashCol + ` FROM Unit`)
	if err != nil && !r.v1NoHashColumn && strings.Contains(err.Error(), "no such column") {
		// A project from before Studio Pro stored ContentsHash.
		r.v1NoHashColumn = true
		return r.scanUnitMetaV1()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query units: %w", err)
	}
	defer rows.Close()

	var metas []v1UnitMeta
	for rows.Next() {
		var unitID, containerID []byte
		var containmentName, hash sql.NullString
		var size sql.NullInt64
		if err := rows.Scan(&unitID, &containerID, &containmentName, &size, &hash); err != nil {
			return nil, fmt.Errorf("failed to scan unit row: %w", err)
		}
		m := v1UnitMeta{
			blob:            unitID,
			key:             string(unitID),
			containerBlob:   containerID,
			containmentName: containmentName.String,
			size:            -1,
			hash:            hash.String,
		}
		if size.Valid {
			m.size = size.Int64
		}
		metas = append(metas, m)
	}
	return metas, rows.Err()
}

// v1HeaderEntry is a unit's Name and Excluded, as read at a length and hash.
type v1HeaderEntry struct {
	size     int64
	hash     string
	name     string
	excluded bool
}

// listUnitHeadersV1 is ListUnitHeaders on MPR v1: a unit's blob is read only
// when its name is not kept for its current length and hash. Naming every
// microflow used to read every microflow's contents, on each lookup by name
// (ako/mxcli#2094).
func (r *Reader) listUnitHeadersV1(typeName string) ([]UnitHeaderRef, error) {
	r.v1TypesMu.Lock()
	defer r.v1TypesMu.Unlock()
	metas, err := r.unitMetasV1()
	if err != nil {
		return nil, err
	}
	if err := r.refreshTypeIndexV1(metas); err != nil {
		return nil, err
	}
	if r.v1Headers == nil {
		r.v1Headers = make(map[string]v1HeaderEntry)
	}
	var stmt *sql.Stmt
	defer func() {
		if stmt != nil {
			_ = stmt.Close()
		}
	}()
	var out []UnitHeaderRef
	for _, m := range metas {
		ref := func(name string, excluded bool) UnitHeaderRef {
			return UnitHeaderRef{ID: blobToUUID(m.blob), ContainerID: blobToUUID(m.containerBlob), Name: name, Excluded: excluded}
		}
		if m.overlaid {
			e := unitIndexEntryOf(m.held)
			if e.Type == typeName {
				out = append(out, ref(e.Name, e.Excluded))
			}
			continue
		}
		if r.v1Types[m.key].typ != typeName {
			continue
		}
		if h, ok := r.v1Headers[m.key]; ok && m.hash != "" && h.hash == m.hash && h.size == m.size {
			out = append(out, ref(h.name, h.excluded))
			continue
		}
		if stmt == nil {
			if stmt, err = r.db.Prepare(`SELECT Contents FROM Unit WHERE UnitID = ?`); err != nil {
				return nil, fmt.Errorf("failed to prepare unit read: %w", err)
			}
		}
		var contents []byte
		if err := stmt.QueryRow(m.blob).Scan(&contents); err != nil {
			if err == sql.ErrNoRows {
				continue // deleted since the metadata scan
			}
			return nil, fmt.Errorf("failed to read unit %s: %w", blobToUUID(m.blob), err)
		}
		r.blobReads.Add(1)
		e := unitIndexEntryOf(contents)
		if int64(len(contents)) != m.size {
			// Rewritten since the metadata scan: type what was read, and keep
			// no name for a row whose hash was not read with it.
			r.v1Types[m.key] = v1TypeEntry{typ: e.Type, size: int64(len(contents))}
			delete(r.v1Headers, m.key)
			if e.Type != typeName {
				continue
			}
		} else if m.hash != "" {
			r.v1Headers[m.key] = v1HeaderEntry{size: m.size, hash: m.hash, name: e.Name, excluded: e.Excluded}
		}
		out = append(out, ref(e.Name, e.Excluded))
	}
	return out, nil
}

// refreshTypeIndexV1 types every unit in metas that the index does not hold at
// its current length, and drops units that no longer exist. Caller holds
// v1TypesMu.
func (r *Reader) refreshTypeIndexV1(metas []v1UnitMeta) error {
	if r.v1Types == nil {
		r.v1Types = make(map[string]v1TypeEntry, len(metas))
	}
	var stale []v1UnitMeta
	present := make(map[string]struct{}, len(metas))
	for _, m := range metas {
		present[m.key] = struct{}{}
		if m.overlaid {
			continue
		}
		if e, ok := r.v1Types[m.key]; !ok || e.size != m.size {
			stale = append(stale, m)
		}
	}
	for id := range r.v1Types {
		if _, ok := present[id]; !ok {
			delete(r.v1Types, id)
		}
	}
	if len(stale) == 0 {
		return nil
	}
	if len(stale) > v1StaleRescan {
		return r.peekAllTypesV1()
	}

	stmt, err := r.db.Prepare(`SELECT length(Contents), substr(Contents, 1, ?) FROM Unit WHERE UnitID = ?`)
	if err != nil {
		return fmt.Errorf("failed to prepare unit type peek: %w", err)
	}
	defer stmt.Close()
	for _, m := range stale {
		var size sql.NullInt64
		var prefix []byte
		if err := stmt.QueryRow(v1TypePeekBytes, m.blob).Scan(&size, &prefix); err != nil {
			if err == sql.ErrNoRows {
				delete(r.v1Types, m.key) // deleted since the metadata scan
				continue
			}
			return fmt.Errorf("failed to peek unit %s: %w", blobToUUID(m.blob), err)
		}
		if err := r.recordTypeV1(m.blob, size, prefix); err != nil {
			return err
		}
	}
	return nil
}

// peekAllTypesV1 (re)builds the whole index in one scan of the blob prefixes.
func (r *Reader) peekAllTypesV1() error {
	rows, err := r.db.Query(`SELECT UnitID, length(Contents), substr(Contents, 1, ?) FROM Unit`, v1TypePeekBytes)
	if err != nil {
		return fmt.Errorf("failed to peek unit types: %w", err)
	}
	type peeked struct {
		blob   []byte
		size   sql.NullInt64
		prefix []byte
	}
	var all []peeked
	for rows.Next() {
		var p peeked
		if err := rows.Scan(&p.blob, &p.size, &p.prefix); err != nil {
			rows.Close()
			return fmt.Errorf("failed to scan unit type: %w", err)
		}
		all = append(all, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close() // before recordTypeV1 may query again
	for _, p := range all {
		if err := r.recordTypeV1(p.blob, p.size, p.prefix); err != nil {
			return err
		}
	}
	return nil
}

// recordTypeV1 stores a unit's type from its peeked prefix, reading the whole
// blob when the prefix does not settle it.
func (r *Reader) recordTypeV1(blob []byte, size sql.NullInt64, prefix []byte) error {
	if !size.Valid {
		r.v1Types[string(blob)] = v1TypeEntry{typ: "", size: -1}
		return nil
	}
	typ, ok := peekBSONType(prefix, size.Int64)
	if !ok {
		var contents []byte
		if err := r.db.QueryRow(`SELECT Contents FROM Unit WHERE UnitID = ?`, blob).Scan(&contents); err != nil {
			if err == sql.ErrNoRows {
				delete(r.v1Types, string(blob))
				return nil
			}
			return fmt.Errorf("failed to read unit %s: %w", blobToUUID(blob), err)
		}
		r.blobReads.Add(1)
		typ = getTypeFromContents(contents)
		size.Int64 = int64(len(contents))
	}
	r.v1Types[string(blob)] = v1TypeEntry{typ: typ, size: size.Int64}
	return nil
}

// peekBSONType returns the top-level $Type string of a BSON document of total
// length size, given only its first bytes. ok is false when the prefix does not
// settle it — the caller then reads the whole document. It answers exactly what
// getTypeFromContents would on the full bytes: "" for a document whose length
// header disagrees with its size, or that has no string $Type.
func peekBSONType(prefix []byte, size int64) (typ string, ok bool) {
	if size == 0 {
		return "", true
	}
	if len(prefix) < 4 {
		return "", false
	}
	if int64(int32(binary.LittleEndian.Uint32(prefix))) != size {
		return "", false // malformed header: let the full decode decide
	}
	rest := prefix[4:]
	for len(rest) > 0 {
		if rest[0] == 0x00 {
			// End of the document: settled only if the whole of it was read.
			return "", int64(len(prefix)) == size
		}
		elem, remaining, ok := bsoncore.ReadElement(rest)
		if !ok {
			return "", false // element runs past the prefix
		}
		if bytes.Equal(elem.KeyBytes(), []byte("$Type")) {
			s, isString := elem.Value().StringValueOK()
			if !isString {
				return "", true
			}
			return s, true
		}
		rest = remaining
	}
	return "", false
}
