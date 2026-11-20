// SPDX-License-Identifier: Apache-2.0

package marketplace

import "testing"

// ako/mxcli#2368 (mendixlabs/mxcli#1883): `update --no-baseline` skipped the
// whole local-edit check, and with it the search for associations other modules
// own into the module — which needs only the project, not the baseline. The
// update deleted MyFirstModule's association without naming it.
func TestSnapshotDependents_FindsAssociationsWithoutABaseline(t *testing.T) {
	target := copyFixture(t)

	// Control: the fixture has none, so a hit below is the association's doing.
	deps, err := SnapshotDependents(target, "Administration", testBackend)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 0 {
		t.Fatalf("control: fixture already has dependents %v", deps)
	}

	execMDL(t, target, crossModuleAssocMDL)
	deps, err = SnapshotDependents(target, "Administration", testBackend)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0].Key.Name != crossModuleAssoc || deps[0].MDL == "" {
		t.Fatalf("dependents = %+v; want %s with its MDL", deps, crossModuleAssoc)
	}
}
