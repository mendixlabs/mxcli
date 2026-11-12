// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// mendixlabs/mxcli#1272: check -p re-read every unit file of a type from disk
// on each listing, so a pass that looks a flow up per statement cost
// statements × units file reads. CacheUnitReads holds the contents for the
// span of a pass that writes nothing.

const cacheTestUnit = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

// newListableV2Writer is newV2WriterForCommitTest with the Unit row's
// container columns filled in, which a listing reads.
func newListableV2Writer(t *testing.T, stored []byte) *Writer {
	t.Helper()
	w, _ := newV2WriterForCommitTest(t, cacheTestUnit, stored)
	if _, err := w.reader.db.Exec(`UPDATE Unit SET ContainerID = ?, ContainmentName = 'Folders'`,
		uuidToBlob("11111111-2222-3333-4444-555555555555")); err != nil {
		t.Fatalf("fill unit row: %v", err)
	}
	return w
}

func listTwice(t *testing.T, r *Reader) {
	t.Helper()
	for range 2 {
		units, err := r.ListUnitsByType("Projects$Folder")
		if err != nil || len(units) != 1 {
			t.Fatalf("list: %d units, %v", len(units), err)
		}
	}
}

func TestCacheUnitReads_ReadsEachFileOnce(t *testing.T) {
	w := newListableV2Writer(t, unitDoc(t, "Stored"))
	r := w.reader

	// Control: without the cache every listing reads the file again (the type
	// index reads it once more on the first listing).
	before := r.fileReads.Load()
	listTwice(t, r)
	if n := r.fileReads.Load() - before; n < 2 {
		t.Fatalf("control: two uncached listings read the file %d time(s), want >= 2", n)
	}

	r.InvalidateCache()
	release := r.CacheUnitReads()
	before = r.fileReads.Load()
	listTwice(t, r)
	if n := r.fileReads.Load() - before; n != 1 {
		t.Errorf("cached: two listings read the file %d time(s), want 1", n)
	}

	// Released, each read checks the file again: it is opened, or served by
	// the persisted store after a stat says it is unchanged (#1804).
	release()
	before = r.fileReads.Load() + r.UnitStoreHits()
	listTwice(t, r)
	if n := r.fileReads.Load() + r.UnitStoreHits() - before; n < 2 {
		t.Errorf("released: two listings read the file %d time(s), want >= 2 (the cache must not outlive the pass)", n)
	}
}

func TestCacheUnitReads_NestedReleaseKeepsOuterScope(t *testing.T) {
	w := newListableV2Writer(t, unitDoc(t, "Stored"))
	r := w.reader
	outer := r.CacheUnitReads()
	defer outer()
	r.CacheUnitReads()() // an inner pass ends inside the outer one

	before := r.fileReads.Load()
	listTwice(t, r)
	if n := r.fileReads.Load() - before; n > 1 {
		t.Errorf("an inner release ended the outer cache: %d reads, want <= 1", n)
	}
}

// A caller that edits the bytes it was handed must not change what the next
// reader sees: os.ReadFile gave every caller its own slice, and so must a hit.
func TestCacheUnitReads_CallerCannotCorruptCache(t *testing.T) {
	stored := unitDoc(t, "Stored")
	w := newListableV2Writer(t, stored)
	r := w.reader
	defer r.CacheUnitReads()()

	for range 2 {
		units, err := r.ListUnitsByType("Projects$Folder")
		if err != nil || len(units) != 1 {
			t.Fatalf("list: %v", err)
		}
		if !bytes.Equal(units[0].Contents, stored) {
			t.Fatalf("listing returned bytes that differ from the stored unit")
		}
		for i := range units[0].Contents {
			units[0].Contents[i] = 0
		}
	}
}

// A write while the cache is held is seen by the next read.
func TestCacheUnitReads_WriteIsSeen(t *testing.T) {
	w := newListableV2Writer(t, unitDoc(t, "Stored"))
	r := w.reader
	defer r.CacheUnitReads()()
	listTwice(t, r)

	written := unitDoc(t, "Written")
	if err := w.UpdateRawUnit(cacheTestUnit, written); err != nil {
		t.Fatalf("update: %v", err)
	}
	units, err := r.ListUnitsByType("Projects$Folder")
	if err != nil || len(units) != 1 {
		t.Fatalf("list: %v", err)
	}
	if !bytes.Equal(units[0].Contents, written) {
		t.Errorf("a read after a write returned the cached bytes from before it")
	}
}

// A write by another process is read from disk even while the cache is held,
// and even when that writer left the unit's row hash as it was: a cached unit
// is keyed by its row hash, which only shows a rewrite by a writer that moves
// it (ako/mxcli#2160), so what SyncWithDisk reports as a foreign change drops
// the cached bytes outright (ako/mxcli#2121).
func TestCacheUnitReads_AnotherProcessesWriteIsSeen(t *testing.T) {
	w := newListableV2Writer(t, unitDoc(t, "Stored"))
	r := w.reader
	defer r.CacheUnitReads()()
	listTwice(t, r)
	before := r.fileReads.Load()
	listTwice(t, r)
	if n := r.fileReads.Load() - before; n != 0 {
		t.Fatalf("control: the listing is not served from the cache (%d file reads), so the test below proves nothing", n)
	}

	// The other process: rewrites the unit file, and commits to the .mpr
	// without touching the unit's ContentsHash.
	foreign := unitDoc(t, "Rewritten elsewhere")
	file := filepath.Join(r.contentsDir, cacheTestUnit[0:2], cacheTestUnit[2:4], cacheTestUnit+".mxunit")
	if err := os.WriteFile(file, foreign, 0o644); err != nil {
		t.Fatal(err)
	}
	other := otherProcess(t, r.path)
	if _, err := other.Exec(`CREATE TABLE probe2160 (x)`); err != nil {
		t.Fatal(err)
	}
	_ = other.Close()

	if got := r.SyncWithDisk(); got != DiskChanged {
		t.Fatalf("SyncWithDisk: %v, want changed", got)
	}
	units, err := r.ListUnitsByType("Projects$Folder")
	if err != nil || len(units) != 1 {
		t.Fatalf("list: %v", err)
	}
	if !bytes.Equal(units[0].Contents, foreign) {
		t.Errorf("after another process rewrote the unit, the listing still returned the cached bytes")
	}
}

// The decoder is on the path of every listing and only reads the bytes it is
// given, so while the cache is held it is handed the cache's own slice: no
// copy per unit per listing, and the same slice each time, which is how the
// decode cache sees that a unit is unchanged without comparing its bytes
// (ako/mxcli#2160). Every other caller still gets bytes of its own.
func TestCacheUnitReads_SharedListingCopiesNothing(t *testing.T) {
	w := newListableV2Writer(t, unitDoc(t, "Stored"))
	r := w.reader
	defer r.CacheUnitReads()()

	shared := func() []byte {
		t.Helper()
		units, err := r.ListUnitsByTypeShared("Projects$Folder", nil)
		if err != nil || len(units) != 1 {
			t.Fatalf("shared list: %d units, %v", len(units), err)
		}
		return units[0].Contents
	}
	first := shared()
	copies := r.UnitContentCopies()
	second := shared()
	if n := r.UnitContentCopies() - copies; n != 0 {
		t.Errorf("a shared listing copied %d unit(s) out of the cache", n)
	}
	if &first[0] != &second[0] {
		t.Error("two shared listings of an unchanged unit returned different slices")
	}

	// Control: the ordinary listing copies, and its bytes are its own.
	copies = r.UnitContentCopies()
	units, err := r.ListUnitsByType("Projects$Folder")
	if err != nil || len(units) != 1 {
		t.Fatalf("list: %v", err)
	}
	if n := r.UnitContentCopies() - copies; n != 1 {
		t.Errorf("control: the ordinary listing made %d copies, want 1", n)
	}
	if &units[0].Contents[0] == &first[0] {
		t.Error("the ordinary listing handed out the cache's own slice")
	}

	// A rewrite is still seen: the unit's row hash moved, so its slice is new.
	written := unitDoc(t, "Written")
	if err := w.UpdateRawUnit(cacheTestUnit, written); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := shared(); !bytes.Equal(got, written) {
		t.Error("a shared listing after a write returned the bytes from before it")
	}
}
