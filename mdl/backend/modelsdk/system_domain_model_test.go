// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/meta"
)

// TestGetDomainModel_VirtualSystemModule guards DESCRIBE System.*: the System
// module is virtual (no stored domain-model unit), so GetDomainModel must serve
// the injected System domain model for its container ID rather than erroring
// "domain model not found" (GetDomainModel errors on truly-missing modules, which
// the drop-module finalize path relies on — System must be the documented exception).
func TestGetDomainModel_VirtualSystemModule(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	sys := buildSystemDomainModel()
	dm, err := b.GetDomainModel(sys.ContainerID)
	if err != nil {
		t.Fatalf("GetDomainModel(System) errored: %v", err)
	}
	if dm == nil || len(dm.Entities) == 0 {
		t.Fatalf("System domain model empty: %+v", dm)
	}
	// A non-existent module must still error (drop-module finalize relies on it).
	if _, err := b.GetDomainModel("ffffffff-0000-0000-0000-000000000000"); err == nil {
		t.Error("GetDomainModel(bogus) should error, not return nil,nil")
	}
}

// TestSystemDomainModel_IncludesAssociations guards that the virtual System
// domain model carries its platform associations. Without them, SHOW/LIST
// ASSOCIATIONS and DESCRIBE MODULE System silently omitted every System
// association on the modelsdk engine.
func TestSystemDomainModel_IncludesAssociations(t *testing.T) {
	dm := buildSystemDomainModel()
	if len(dm.Associations) != len(meta.SystemAssociations) {
		t.Fatalf("System associations: got %d, want %d", len(dm.Associations), len(meta.SystemAssociations))
	}

	// Parent/Child IDs must use the same synthetic scheme as the entities so the
	// list/describe paths resolve them to qualified names.
	entityIDs := make(map[string]bool, len(dm.Entities))
	for _, e := range dm.Entities {
		entityIDs[string(e.ID)] = true
	}
	var sessionUser bool
	for _, a := range dm.Associations {
		if a.Name == "Session_User" {
			sessionUser = true
			if string(a.ParentID) != "System.Session" || string(a.ChildID) != "System.User" {
				t.Errorf("Session_User Parent/Child IDs wrong: %q -> %q", a.ParentID, a.ChildID)
			}
		}
		if !entityIDs[string(a.ParentID)] {
			t.Errorf("association %s ParentID %q does not match any System entity", a.Name, a.ParentID)
		}
		if !entityIDs[string(a.ChildID)] {
			t.Errorf("association %s ChildID %q does not match any System entity", a.Name, a.ChildID)
		}
	}
	if !sessionUser {
		t.Error("expected System.Session_User association to be present")
	}
}
