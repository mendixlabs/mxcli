// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"os"
	"testing"
)

// ako/mxcli#2121: SyncWithDisk is what a long-lived session asks before each
// statement. It must report another connection's commit, the replacement of
// the .mpr by a new file, and nothing for the reader's own writes — or every
// statement would throw the caches away (#2094).
func TestSyncWithDisk_Issue2121(t *testing.T) {
	path := copyV1Fixture(t)
	r, err := OpenWithOptions(path, OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if got := r.SyncWithDisk(); got != DiskUnchanged {
		t.Fatalf("right after open: %v, want unchanged", got)
	}
	if _, err := r.db.Exec(`CREATE TABLE probe2121 (x)`); err != nil {
		t.Fatal(err)
	}
	if got := r.SyncWithDisk(); got != DiskUnchanged {
		t.Errorf("after the reader's own commit: %v, want unchanged", got)
	}

	other := otherProcess(t, path)
	if _, err := other.Exec(`INSERT INTO probe2121 VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if got := r.SyncWithDisk(); got != DiskChanged {
		t.Errorf("after another connection's commit: %v, want changed", got)
	}
	if got := r.SyncWithDisk(); got != DiskUnchanged {
		t.Errorf("asked again with nothing new: %v, want unchanged", got)
	}
	_ = other.Close()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".new", b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".new", path); err != nil {
		t.Fatal(err)
	}
	if got := r.SyncWithDisk(); got != DiskReplaced {
		t.Errorf("after the .mpr was replaced by a new file: %v, want replaced", got)
	}
}
