// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2094 (mendixlabs/mxcli#1795): `exec` and `check` took 0.3–0.9 s
// per statement on a v1 project of 3,400 units, whatever the statement
// changed. Every typed listing re-read the metadata of the whole Unit table to
// revalidate the type index, and a statement makes many listings; a lookup by
// name (ListUnitHeaders) read the full contents of every unit of the type. The
// table is now scanned again only when it may have changed — a write through
// this reader, or a commit by another connection or process — and a unit's
// Name / Excluded are read once per version of its contents.

// cachedListingFixture is newTypedListingFixture with a ContentsHash on every
// unit, as a real project has (Studio Pro fills it on every v1 unit,
// ako/mxcli#1159).
func cachedListingFixture(t *testing.T, types []string) (*typedListingFixture, string) {
	t.Helper()
	f := newTypedListingFixture(t, types)
	path := f.r.path
	// Without the path the file-stat signal is off, so these tests prove the
	// other two on their own: data_version for another connection's commit,
	// InvalidateCache for this reader's writes. (On Windows a file's size and
	// time can lag while it is open; data_version does not.)
	f.r.path = ""
	rows, err := f.db.Query(`SELECT UnitID, Contents FROM Unit`)
	if err != nil {
		t.Fatal(err)
	}
	type row struct{ id, contents []byte }
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.contents); err != nil {
			t.Fatal(err)
		}
		all = append(all, r)
	}
	rows.Close()
	for _, r := range all {
		if _, err := f.db.Exec(`UPDATE Unit SET ContentsHash = ? WHERE UnitID = ?`, contentsHashOf(r.contents), r.id); err != nil {
			t.Fatal(err)
		}
	}
	return f, path
}

// otherProcess opens the project file on a connection of its own, as Studio
// Pro or a second mxcli would.
func otherProcess(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func microflowIDs(t *testing.T, r *Reader) []string {
	t.Helper()
	units, err := r.listUnitsByType("Microflows$Microflow")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, u := range units {
		ids = append(ids, u.ID)
	}
	return ids
}

func TestListUnitsByTypeV1ScansTableOncePerChange_Issue2094(t *testing.T) {
	f, path := cachedListingFixture(t, fixtureTypes(10, 3, "Microflows$Microflow", "Forms$Page"))

	before := f.r.metaScans.Load()
	for range 3 {
		microflowIDs(t, f.r)
		if _, err := f.r.listUnitTypesV1(); err != nil {
			t.Fatal(err)
		}
	}
	if scans := f.r.metaScans.Load() - before; scans != 1 {
		t.Errorf("six listings of an unchanged table scanned it %d times; want 1", scans)
	}

	// A commit by another connection is seen by the next listing.
	other := otherProcess(t, path)
	if _, err := other.Exec(`INSERT INTO Unit (UnitID, ContainerID, ContainmentName, Contents) VALUES (?, ?, ?, ?)`,
		uuidToBlob(v1UnitID(50)), uuidToBlob(v1UnitID(0)), "Documents", v1UnitContents(t, 50, "Microflows$Microflow", 10)); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprint([]string{v1UnitID(1), v1UnitID(4), v1UnitID(7), v1UnitID(10), v1UnitID(50)})
	if got := fmt.Sprint(microflowIDs(t, f.r)); got != want {
		t.Errorf("after another process inserted a microflow: got %s, want %s", got, want)
	}

	// So is a write through this reader's own connection followed by the
	// invalidation every writer path makes.
	if _, err := f.db.Exec(`DELETE FROM Unit WHERE UnitID = ?`, uuidToBlob(v1UnitID(4))); err != nil {
		t.Fatal(err)
	}
	f.r.InvalidateCache()
	want = fmt.Sprint([]string{v1UnitID(1), v1UnitID(7), v1UnitID(10), v1UnitID(50)})
	if got := fmt.Sprint(microflowIDs(t, f.r)); got != want {
		t.Errorf("after deleting a microflow: got %s, want %s", got, want)
	}
}

// renamed is unit i's fixture document (v1UnitContents, pad 8000) with a
// different Name of the same length, so the blob length — the type index's
// change detector — does not move.
func renamed(t *testing.T, i int, typeName, name string) []byte {
	t.Helper()
	b, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: uuidToBlob(v1UnitID(i))}},
		{Key: "$Type", Value: typeName},
		{Key: "Name", Value: name},
		{Key: "Payload", Value: bytes.Repeat([]byte{'x'}, 8000)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func headerNames(t *testing.T, r *Reader, typeName string) string {
	t.Helper()
	hs, err := r.ListUnitHeaders(typeName)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, h := range hs {
		out = append(out, h.ID[len(h.ID)-2:]+"="+h.Name)
	}
	return fmt.Sprint(out)
}

func TestListUnitHeadersV1ReadsEachBlobOnce_Issue2094(t *testing.T) {
	f, path := cachedListingFixture(t, fixtureTypes(10, 3, "Microflows$Microflow", "Forms$Page"))

	if got, want := headerNames(t, f.r, "Microflows$Microflow"), "[01=Unit1 04=Unit4 07=Unit7 10=Unit10]"; got != want {
		t.Fatalf("headers: %s, want %s", got, want)
	}
	before := f.r.blobReads.Load()
	headerNames(t, f.r, "Microflows$Microflow")
	if reads := f.r.blobReads.Load() - before; reads != 0 {
		t.Errorf("naming the microflows again read %d blobs; want 0", reads)
	}

	// Renamed through this reader's connection, same length: the hash moves.
	doc := renamed(t, 4, "Microflows$Microflow", "UnitX")
	if len(doc) != len(v1UnitContents(t, 4, "Microflows$Microflow", 8000)) {
		t.Fatal("the rename must keep the blob length")
	}
	if _, err := f.db.Exec(`UPDATE Unit SET Contents = ?, ContentsHash = ? WHERE UnitID = ?`,
		doc, contentsHashOf(doc), uuidToBlob(v1UnitID(4))); err != nil {
		t.Fatal(err)
	}
	f.r.InvalidateCache()
	before = f.r.blobReads.Load()
	if got, want := headerNames(t, f.r, "Microflows$Microflow"), "[01=Unit1 04=UnitX 07=Unit7 10=Unit10]"; got != want {
		t.Errorf("after a rename: %s, want %s", got, want)
	}
	if reads := f.r.blobReads.Load() - before; reads != 1 {
		t.Errorf("after renaming one microflow the listing read %d blobs; want 1", reads)
	}

	// Renamed by another process that left the hash alone: nothing about the
	// row says it changed, so a commit from elsewhere drops every name.
	doc = renamed(t, 7, "Microflows$Microflow", "UnitY")
	other := otherProcess(t, path)
	if _, err := other.Exec(`UPDATE Unit SET Contents = ? WHERE UnitID = ?`, doc, uuidToBlob(v1UnitID(7))); err != nil {
		t.Fatal(err)
	}
	if got, want := headerNames(t, f.r, "Microflows$Microflow"), "[01=Unit1 04=UnitX 07=UnitY 10=Unit10]"; got != want {
		t.Errorf("after another process renamed a microflow: %s, want %s", got, want)
	}
}

// A unit without a ContentsHash has nothing to tell a same-length rewrite by,
// so its name is read every time rather than remembered.
func TestListUnitHeadersV1WithoutHashRereads_Issue2094(t *testing.T) {
	f := newTypedListingFixture(t, fixtureTypes(4, 2, "Microflows$Microflow", "Forms$Page"))
	f.r.path = filepath.Join(t.TempDir(), "absent.mpr")
	headerNames(t, f.r, "Microflows$Microflow")
	doc := renamed(t, 1, "Microflows$Microflow", "UnitZ")
	if _, err := f.db.Exec(`UPDATE Unit SET Contents = ? WHERE UnitID = ?`, doc, uuidToBlob(v1UnitID(1))); err != nil {
		t.Fatal(err)
	}
	f.r.InvalidateCache()
	if got, want := headerNames(t, f.r, "Microflows$Microflow"), "[01=UnitZ 03=Unit3]"; got != want {
		t.Errorf("after a rename without a hash: %s, want %s", got, want)
	}
}

// The writer's v1 update is the one write path that used to leave the reader
// uninformed, when every listing rescanned the table anyway.
func TestUpdateUnitV1InvalidatesListing_Issue2094(t *testing.T) {
	f, _ := cachedListingFixture(t, fixtureTypes(4, 2, "Microflows$Microflow", "Forms$Page"))
	headerNames(t, f.r, "Microflows$Microflow")
	if err := NewWriterWithReader(f.r).UpdateRawUnit(v1UnitID(3), renamed(t, 3, "Microflows$Microflow", "UnitW")); err != nil {
		t.Fatal(err)
	}
	if got, want := headerNames(t, f.r, "Microflows$Microflow"), "[01=Unit1 03=UnitW]"; got != want {
		t.Errorf("after the writer renamed a microflow: %s, want %s", got, want)
	}
}
