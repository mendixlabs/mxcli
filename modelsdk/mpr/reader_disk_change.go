// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"os"
)

// DiskChange is what SyncWithDisk found.
type DiskChange int

const (
	// DiskUnchanged: nothing but this reader has written to the project since
	// the last look.
	DiskUnchanged DiskChange = iota
	// DiskChanged: another connection — a second mxcli, Studio Pro — committed
	// to the .mpr. The reader has already dropped what it kept.
	DiskChanged
	// DiskReplaced: the .mpr at the reader's path is no longer the file the
	// reader holds open (a git checkout writes a new file and renames it into
	// place). The open connection would go on reading the old file, so the
	// owner must reopen the project.
	DiskReplaced
)

// diskStamp is what says the project on disk may have changed under a reader
// (ako/mxcli#2121). dataVersion is SQLite's PRAGMA data_version: it moves when
// ANOTHER connection commits, never for this reader's own writes (the reader
// holds one connection, SetMaxOpenConns(1)), so a session's own writes do not
// read as foreign ones and do not throw the caches away (#2094). file is the
// .mpr's identity, for the replacement data_version cannot see.
type diskStamp struct {
	dataVersion int64
	file        os.FileInfo
	ok          bool
}

func (r *Reader) diskStampNow() diskStamp {
	var s diskStamp
	if r.db == nil {
		return s
	}
	if err := r.db.QueryRow(`PRAGMA data_version`).Scan(&s.dataVersion); err != nil {
		return s
	}
	if r.path != "" {
		if fi, err := os.Stat(r.path); err == nil {
			s.file = fi
		}
	}
	s.ok = true
	return s
}

// markDiskBaseline records the project's current state as what this reader
// has seen. Called on open.
func (r *Reader) markDiskBaseline() {
	r.diskMu.Lock()
	r.diskSeen = r.diskStampNow()
	r.diskMu.Unlock()
}

// SyncWithDisk tells whether something other than this reader changed the
// project since the last call (or since the reader was opened), and when it
// did, drops every view of the project the reader keeps, so the next read
// sees what is on disk. It costs one PRAGMA and one stat, not a read of the
// project, so a session can call it before every statement.
//
// What it cannot see: a change on MPR v2 to mprcontents/ alone, with the .mpr
// untouched. Every writer known — mxcli, Studio Pro, a git checkout of a
// commit — moves the Unit table's ContentsHash with the file, and unit bytes
// are read from their file on each read anyway, so only the index of names
// would lag.
func (r *Reader) SyncWithDisk() DiskChange {
	now := r.diskStampNow()
	r.diskMu.Lock()
	seen := r.diskSeen
	r.diskSeen = now
	r.diskMu.Unlock()

	if seen.file != nil && now.file != nil && !os.SameFile(seen.file, now.file) {
		r.dropDiskViews()
		return DiskReplaced
	}
	if !now.ok || !seen.ok || now.dataVersion != seen.dataVersion {
		r.dropDiskViews()
		return DiskChanged
	}
	return DiskUnchanged
}

// dropDiskViews forgets everything the reader keeps that another writer can
// have invalidated without telling it: the unit index, the v1 listing and the
// names it kept by ContentsHash (another writer need not move the hash), the
// name index and cached unit bytes. Overlays are this session's own pending
// bytes and stay.
func (r *Reader) dropDiskViews() {
	r.InvalidateCache()
	// InvalidateCache keeps the cached unit bytes, each valid for the row
	// hash it was read under — which is sound for this reader's own writes
	// and not for another writer's, who need not move the hash.
	r.contentMu.Lock()
	if r.contentCache != nil {
		clear(r.contentCache)
	}
	r.contentMu.Unlock()
	r.v1TypesMu.Lock()
	clear(r.v1Headers)
	r.v1TypesMu.Unlock()
}
