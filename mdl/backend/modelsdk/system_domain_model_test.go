// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/meta"
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

	sys := buildSystemDomainModel(nil)
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
	dm := buildSystemDomainModel(nil)
	if len(dm.Associations) != len(meta.ModelerSystemAssociations()) {
		t.Fatalf("System associations: got %d, want %d", len(dm.Associations), len(meta.ModelerSystemAssociations()))
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

// upstream #972. The virtual System module is what every write path resolves a
// System name against, so an entity that exists only in the runtime's metamodel
// must not appear in it — a model naming one is rejected by mxbuild with CE1613
// "no longer exists".
//
// `Thumbnail_Image` is the one that bit: owned by BOTH ends, so System.Image
// counted as an owner through the child side and every specialization of it
// inherited an access-rule member for it. Measured on mxbuild 11.13.0:
//
//	[CE1613] "The selected association 'System.Thumbnail_Image' no longer exists."
//	         at Access rule of entity 'Img.CaseImage'
//
// The legacy engine's System list never carried the runtime rows and writes a
// clean rule from the same script — the control that pins this to the list
// rather than to the GRANT path reading it.
func TestSystemDomainModel_OmitsRuntimeOnlyEntities(t *testing.T) {
	dm := buildSystemDomainModel(nil)

	entities := make(map[string]bool, len(dm.Entities))
	for _, e := range dm.Entities {
		entities[e.Name] = true
	}
	associations := make(map[string]bool, len(dm.Associations))
	for _, a := range dm.Associations {
		associations[a.Name] = true
	}

	if entities["Thumbnail"] {
		t.Error("System.Thumbnail is runtime-only; the modeler has no such entity")
	}
	if associations["Thumbnail_Image"] {
		t.Error("System.Thumbnail_Image reached the virtual System module — this is #972, and every System.Image specialization inherits it")
	}
	// Controls: the neighbours that ARE real. Without them this passes against a
	// builder that returns an empty domain model.
	for _, name := range []string{"Image", "FileDocument", "User", "Session"} {
		if !entities[name] {
			t.Errorf("System.%s is a real modeler entity and must still be built", name)
		}
	}
	if !associations["UserRoles"] {
		t.Error("System.UserRoles is a real modeler association and must still be built")
	}
}
