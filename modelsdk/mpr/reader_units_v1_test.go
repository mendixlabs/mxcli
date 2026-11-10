// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	_ "modernc.org/sqlite"
)

// typedListingFixture is a minimal MPR v1 Unit table with units of several types.
type typedListingFixture struct {
	db  *sql.DB
	r   *Reader
	ids []string // in insertion order
}

func v1UnitID(i int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", i) }

// v1UnitContents is a unit document shaped like Studio Pro's: $ID, then $Type,
// then a payload big enough to spill into SQLite overflow pages.
func v1UnitContents(t *testing.T, i int, typeName string, pad int) []byte {
	t.Helper()
	b, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: uuidToBlob(v1UnitID(i))}},
		{Key: "$Type", Value: typeName},
		{Key: "Name", Value: fmt.Sprintf("Unit%d", i)},
		{Key: "Payload", Value: bytes.Repeat([]byte{'x'}, pad)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newTypedListingFixture(t *testing.T, types []string) *typedListingFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v1.mpr")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE Unit (
		UnitID BLOB PRIMARY KEY NOT NULL, ContainerID BLOB, ContainmentName TEXT,
		TreeConflict LONG, ContentsHash TEXT, ContentsConflicts TEXT, Contents BLOB)`); err != nil {
		t.Fatal(err)
	}
	f := &typedListingFixture{db: db, r: &Reader{db: db, version: MPRVersionV1, path: path}}
	for i, ty := range types {
		f.insert(t, i+1, ty, 8000)
	}
	return f
}

func (f *typedListingFixture) insert(t *testing.T, i int, typeName string, pad int) {
	t.Helper()
	id := v1UnitID(i)
	if _, err := f.db.Exec(`INSERT INTO Unit (UnitID, ContainerID, ContainmentName, Contents) VALUES (?, ?, ?, ?)`,
		uuidToBlob(id), uuidToBlob(v1UnitID(0)), "Documents", v1UnitContents(t, i, typeName, pad)); err != nil {
		t.Fatal(err)
	}
	f.ids = append(f.ids, id)
}

func fixtureTypes(n, every int, match, other string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = other
		if i%every == 0 {
			out[i] = match
		}
	}
	return out
}

// A typed listing on MPR v1 must not read the contents of units of other types.
// It used to load every blob in the project and filter afterwards, which on a
// 140 MB project made every listing a full read of the file (ako/mxcli: a plain
// `describe microflow` on Evora did fourteen of them).
func TestListUnitsByTypeV1ReadsOnlyMatchingBlobs(t *testing.T) {
	f := newTypedListingFixture(t, fixtureTypes(60, 10, "Microflows$Microflow", "Forms$Page"))

	for round := range 2 {
		before := f.r.blobReads.Load()
		units, err := f.r.listUnitsByType("Microflows$Microflow")
		if err != nil {
			t.Fatal(err)
		}
		if len(units) != 6 {
			t.Fatalf("round %d: got %d microflows, want 6", round, len(units))
		}
		if reads := f.r.blobReads.Load() - before; reads > int64(len(units)) {
			t.Errorf("round %d: listing 6 of 60 units read %d unit blobs; want at most 6", round, reads)
		}
	}
}

// The filtered listing must return exactly what the read-everything listing
// returned: same units, same order, same bytes, same metadata.
func TestListUnitsByTypeV1MatchesFullScan(t *testing.T) {
	types := []string{"Forms$Page", "Forms$PageTemplate", "Microflows$Microflow", "Forms$Page",
		"Microflows$Nanoflow", "Microflows$Microflow", "Projects$Folder", "Forms$Page"}
	f := newTypedListingFixture(t, types)

	all, err := f.r.listUnitsByType("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(types) {
		t.Fatalf("unfiltered listing: %d units, want %d", len(all), len(types))
	}
	for _, ty := range []string{"Forms$Page", "Forms$PageTemplate", "Microflows$Microflow", "Microflows$Nanoflow", "Projects$Folder", "Nope$Nothing"} {
		var want []rawUnit
		for _, u := range all {
			if u.Type == ty {
				want = append(want, u)
			}
		}
		got, err := f.r.listUnitsByType(ty)
		if err != nil {
			t.Fatal(err)
		}
		assertSameUnits(t, ty, got, want)
	}
}

// The type index must not go stale even after a write nobody announced: the
// writes here go through the reader's own handle without InvalidateCache, and
// the kept scan of the table (ako/mxcli#2094) must still notice the file moved.
func TestListUnitsByTypeV1SeesWritesWithoutInvalidate(t *testing.T) {
	f := newTypedListingFixture(t, fixtureTypes(10, 3, "Microflows$Microflow", "Forms$Page"))
	if _, err := f.r.listUnitsByType("Microflows$Microflow"); err != nil { // warm the index
		t.Fatal(err)
	}

	// New unit, deleted unit, rewritten unit (type and length change).
	f.insert(t, 100, "Microflows$Microflow", 50)
	if _, err := f.db.Exec(`DELETE FROM Unit WHERE UnitID = ?`, uuidToBlob(v1UnitID(1))); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE Unit SET Contents = ? WHERE UnitID = ?`,
		v1UnitContents(t, 2, "Microflows$Microflow", 10), uuidToBlob(v1UnitID(2))); err != nil {
		t.Fatal(err)
	}

	got, err := f.r.listUnitsByType("Microflows$Microflow")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, u := range got {
		ids = append(ids, u.ID)
	}
	want := []string{v1UnitID(2), v1UnitID(4), v1UnitID(7), v1UnitID(10), v1UnitID(100)}
	if fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Errorf("after writes: got %v, want %v", ids, want)
	}
}

// A $Type that is not within the peeked prefix (a document with a long leading
// field) falls back to reading the unit, rather than dropping it.
func TestListUnitsByTypeV1TypeBeyondPeek(t *testing.T) {
	f := newTypedListingFixture(t, []string{"Forms$Page"})
	doc, err := bson.Marshal(bson.D{
		{Key: "Leading", Value: string(bytes.Repeat([]byte{'y'}, 4*v1TypePeekBytes))},
		{Key: "$Type", Value: "Microflows$Microflow"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO Unit (UnitID, ContainerID, ContainmentName, Contents) VALUES (?, ?, ?, ?)`,
		uuidToBlob(v1UnitID(9)), uuidToBlob(v1UnitID(0)), "Documents", doc); err != nil {
		t.Fatal(err)
	}
	got, err := f.r.listUnitsByType("Microflows$Microflow")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != v1UnitID(9) || !bytes.Equal(got[0].Contents, doc) {
		t.Fatalf("unit with late $Type: got %d units", len(got))
	}
}

// An overlaid unit is typed and returned from its in-memory bytes.
func TestListUnitsByTypeV1HonoursOverlay(t *testing.T) {
	f := newTypedListingFixture(t, []string{"Forms$Page", "Microflows$Microflow"})
	held := v1UnitContents(t, 2, "Microflows$Microflow", 3)
	f.r.SetOverlay(v1UnitID(2), held)
	got, err := f.r.listUnitsByType("Microflows$Microflow")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !bytes.Equal(got[0].Contents, held) {
		t.Fatalf("overlay not honoured: %d units", len(got))
	}
}

func assertSameUnits(t *testing.T, label string, got, want []rawUnit) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d units, want %d", label, len(got), len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.ID != w.ID || g.ContainerID != w.ContainerID || g.ContainmentName != w.ContainmentName ||
			g.Type != w.Type || !bytes.Equal(g.Contents, w.Contents) {
			t.Errorf("%s[%d]: got %s/%s/%s/%s, want %s/%s/%s/%s", label, i,
				g.ID, g.ContainerID, g.ContainmentName, g.Type, w.ID, w.ContainerID, w.ContainmentName, w.Type)
		}
	}
}

// Against a real Studio Pro v1 project: every typed listing equals the
// unfiltered listing filtered by type.
func TestListUnitsByTypeV1MatchesFullScanOnRealProject(t *testing.T) {
	r, err := OpenWithOptions(v1Fixture, OpenOptions{ReadOnly: true})
	if err != nil {
		t.Skipf("v1 fixture unavailable: %v", err)
	}
	defer r.Close()
	if r.version != MPRVersionV1 {
		t.Fatalf("fixture is not v1")
	}
	all, err := r.listUnitsByType("")
	if err != nil {
		t.Fatal(err)
	}
	byType := map[string][]rawUnit{}
	for _, u := range all {
		byType[u.Type] = append(byType[u.Type], u)
	}
	if len(byType) < 5 {
		t.Fatalf("fixture has only %d unit types", len(byType))
	}
	for ty, want := range byType {
		before := r.blobReads.Load()
		got, err := r.listUnitsByType(ty)
		if err != nil {
			t.Fatal(err)
		}
		assertSameUnits(t, ty, got, want)
		if reads := r.blobReads.Load() - before; reads > int64(len(want)) {
			t.Errorf("%s: read %d blobs for %d units", ty, reads, len(want))
		}
	}
}

// ListUnits needs the project's shape, not its documents: on v1 it must not
// read unit contents, and must report what the full listing reports.
func TestListUnitsV1ReadsNoBlobs(t *testing.T) {
	f := newTypedListingFixture(t, fixtureTypes(30, 4, "Microflows$Microflow", "Forms$Page"))
	all, err := f.r.listUnitsByType("")
	if err != nil {
		t.Fatal(err)
	}
	before := f.r.blobReads.Load()
	infos, err := f.r.ListUnits()
	if err != nil {
		t.Fatal(err)
	}
	if reads := f.r.blobReads.Load() - before; reads != 0 {
		t.Errorf("ListUnits read %d unit blobs; want 0", reads)
	}
	if len(infos) != len(all) {
		t.Fatalf("ListUnits: %d units, want %d", len(infos), len(all))
	}
	for i, u := range all {
		g := infos[i]
		if string(g.ID) != u.ID || string(g.ContainerID) != u.ContainerID || g.ContainmentName != u.ContainmentName || g.Type != u.Type {
			t.Errorf("ListUnits[%d] = %+v, want %s/%s/%s/%s", i, g, u.ID, u.ContainerID, u.ContainmentName, u.Type)
		}
	}
}
