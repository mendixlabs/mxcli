// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"os"
	"testing"
	"time"
)

// TestSnapshotDependents_SeesAWriteInTheSecondOfTheFirstRead is the deterministic
// form of what TestSnapshotDependents_FindsAssociationsWithoutABaseline did by
// chance: the first read builds the catalog, the write follows it, and when both
// land in one second the second read must still see the write. The catalog cache
// compared whole seconds, so it answered from the build made before the write
// and returned no dependents (about one run in six; red on main at cd4076243).
//
// The control is the same sequence with the write a second later, which was
// always seen.
func TestSnapshotDependents_SeesAWriteInTheSecondOfTheFirstRead(t *testing.T) {
	for _, tc := range []struct {
		name  string
		after time.Duration
	}{
		{"write in the same second", 500 * time.Millisecond},
		{"write a second later (control)", 1500 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := copyFixture(t)
			at := time.Now().Truncate(time.Second)
			if err := os.Chtimes(target, at, at); err != nil {
				t.Fatal(err)
			}
			if _, err := SnapshotDependents(target, "Administration", testBackend); err != nil {
				t.Fatal(err)
			}
			execMDL(t, target, crossModuleAssocMDL)
			written := at.Add(tc.after)
			if err := os.Chtimes(target, written, written); err != nil {
				t.Fatal(err)
			}
			deps, err := SnapshotDependents(target, "Administration", testBackend)
			if err != nil {
				t.Fatal(err)
			}
			if len(deps) != 1 || deps[0].Key.Name != crossModuleAssoc {
				t.Fatalf("dependents = %+v; want %s", deps, crossModuleAssoc)
			}
		})
	}
}
