// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// TestGetRawUnit_V1 verifies that GetRawUnit works on v1 MPR files (Mendix < 10.18)
// where UnitID is stored as a 16-byte GUID blob in SQLite.
// Regression test for https://github.com/mendixlabs/mxcli/issues/705
func TestGetRawUnit_V1(t *testing.T) {
	mprPath := "testdata/v1-project/App.mpr"

	reader, err := Open(mprPath)
	if err != nil {
		t.Fatalf("failed to open v1 MPR: %v", err)
	}
	defer reader.Close()

	if reader.Version() != MPRVersionV1 {
		t.Fatalf("expected MPR v1, got v%d", reader.Version())
	}

	// ListAllUnitIDs works correctly (uses blobToUUID internally)
	ids, err := reader.ListAllUnitIDs()
	if err != nil {
		t.Fatalf("ListAllUnitIDs: %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("expected at least one unit ID")
	}

	// GetRawUnit must be able to retrieve any unit by the ID that ListAllUnitIDs returns.
	// Before the fix, this always returned "no rows in result set" on v1 MPRs.
	for _, id := range ids {
		raw, err := reader.GetRawUnit(model.ID(id))
		if err != nil {
			t.Errorf("GetRawUnit(%s): %v", id, err)
			continue
		}
		if raw == nil {
			t.Errorf("GetRawUnit(%s): returned nil map", id)
			continue
		}
		if _, ok := raw["$Type"]; !ok {
			t.Errorf("GetRawUnit(%s): BSON missing $Type field", id)
		}
	}
}
