// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/internal/testutil"
)

// ako/mxcli#2043: GetRawUnitByName read and decoded every unit of the type on
// each call. Building a microflow resolves each `call microflow` target
// through it, so one large flow in a large project scanned every microflow
// once per distinct callee — past the 5-minute statement limit, whose abandoned
// statement then crashed exec. A lookup now reads the one unit an index names,
// and verifies it; only a miss scans.

func openPedAppReader(t *testing.T) (*Reader, string) {
	t.Helper()
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, "../../testdata/pedapp"); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(dst, "PedApp.mpr")
	r, err := Open(mpr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r, dst
}

func unitReads(r *Reader) int64 { return r.fileReads.Load() + r.UnitStoreHits() }

func TestGetRawUnitByName_ReadsOneUnitOnceIndexed(t *testing.T) {
	r, _ := openPedAppReader(t)
	mfs, err := r.listUnitsByType("Microflows$Microflow")
	if err != nil || len(mfs) < 5 {
		t.Fatalf("fixture: %d microflows, %v", len(mfs), err)
	}

	// A lookup resolves module names, which reads the module and folder units
	// on every call, before and after the index; that is not the type's units.
	before := unitReads(r)
	if _, _, err := r.buildModuleResolution(); err != nil {
		t.Fatal(err)
	}
	resolution := unitReads(r) - before

	// The first lookup scans the type, as before.
	if _, err := r.GetRawUnitByName("microflow", "Administration.ChangePassword"); err != nil {
		t.Fatal(err)
	}
	// Every later one — another name included — reads the unit it returns.
	for _, name := range []string{"Administration.NewAccount", "Administration.ChangeMyPassword", "Administration.NewAccount"} {
		before := unitReads(r)
		info, err := r.GetRawUnitByName("microflow", name)
		if err != nil || info.QualifiedName != name {
			t.Fatalf("lookup %s: %+v, %v", name, info, err)
		}
		if n := unitReads(r) - before - resolution; n > 1 {
			t.Errorf("lookup %s read %d microflow units, want at most 1 of %d", name, n, len(mfs))
		}
	}
}

// Control on the index: the scan it replaces reads every unit of the type.
func TestGetRawUnitByName_ScanReadsEveryUnit(t *testing.T) {
	r, _ := openPedAppReader(t)
	mfs, _ := r.listUnitsByType("Microflows$Microflow")
	before := unitReads(r)
	if _, _, err := r.buildModuleResolution(); err != nil {
		t.Fatal(err)
	}
	resolution := unitReads(r) - before
	before = unitReads(r)
	if _, err := r.GetRawUnitByName("microflow", "Administration.NoSuchFlow"); err == nil {
		t.Fatal("found a flow that does not exist")
	}
	if n := unitReads(r) - before - resolution; n < int64(len(mfs)) {
		t.Errorf("a miss read %d units, want the full scan of %d", n, len(mfs))
	}
}

// An index entry is never trusted: a unit renamed behind the reader's back
// (Studio Pro saving the project) is not found under its old name, and is
// found under its new one.
func TestGetRawUnitByName_IndexSurvivesAnExternalRename(t *testing.T) {
	r, dir := openPedAppReader(t)
	old, err := r.GetRawUnitByName("microflow", "Administration.NewAccount")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mprcontents", old.ID[0:2], old.ID[2:4], old.ID+".mxunit")
	var doc bson.D
	if err := bson.Unmarshal(old.Contents, &doc); err != nil {
		t.Fatal(err)
	}
	for i := range doc {
		if doc[i].Key == "Name" {
			doc[i].Value = "NewAccountRenamed"
		}
	}
	renamed, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, renamed, 0o644); err != nil {
		t.Fatal(err)
	}

	if info, err := r.GetRawUnitByName("microflow", "Administration.NewAccount"); err == nil {
		t.Errorf("the old name still resolves after the rename: %s", info.QualifiedName)
	}
	info, err := r.GetRawUnitByName("microflow", "Administration.NewAccountRenamed")
	if err != nil || !strings.HasSuffix(info.QualifiedName, "NewAccountRenamed") || info.ID != old.ID {
		t.Errorf("the new name: %+v, %v", info, err)
	}
}
