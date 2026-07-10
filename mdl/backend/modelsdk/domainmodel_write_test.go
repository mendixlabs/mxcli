// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// copyFixture copies the (v2, mprcontents-backed) fixture project into a temp
// dir so write tests never mutate the shared testdata.
func copyFixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS("../../../testdata/expr-checker")); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return filepath.Join(dst, "minimal.mpr")
}

// TestWriteSlice_CreateEntity is the Phase-2 write-path proof: create an entity
// through the codec engine, reopen the project, and confirm it persisted with
// the expected properties. (BSON parity vs legacy is validated separately by the
// engine-diff write harness.)
func TestWriteSlice_CreateEntity(t *testing.T) {
	proj := copyFixture(t)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName(MyFirstModule) = %v, %v", mod, err)
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		t.Fatalf("GetDomainModel = %v, %v", dm, err)
	}
	before := len(dm.Entities)

	if err := b.CreateEntity(dm.ID, &domainmodel.Entity{Name: "SliceTest", Persistable: true}); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	// Reopen with a fresh backend to confirm the write hit disk.
	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	dm2, err := b2.GetDomainModel(mod.ID)
	if err != nil || dm2 == nil {
		t.Fatalf("GetDomainModel after reopen = %v, %v", dm2, err)
	}
	if len(dm2.Entities) != before+1 {
		t.Fatalf("entity count = %d, want %d (before+1)", len(dm2.Entities), before+1)
	}
	var found *domainmodel.Entity
	for _, e := range dm2.Entities {
		if e.Name == "SliceTest" {
			found = e
		}
	}
	if found == nil {
		t.Fatal("created entity 'SliceTest' not found after reopen")
	}
	if !found.Persistable {
		t.Error("created entity should be persistable")
	}
}
