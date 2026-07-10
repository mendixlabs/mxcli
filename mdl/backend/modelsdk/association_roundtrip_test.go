// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// TestUpdateDomainModel_PreservesAssociationType guards the CREATE OR MODIFY
// ASSOCIATION corruption: that path re-serializes the whole domain model via
// UpdateDomainModel→assocToGen, which reads Type/Owner off the semantic model.
// If assocFromGen drops them, every *other* association loses Type/Owner and
// Studio Pro can't load the domain model ("cannot destructure property 'child'").
func TestUpdateDomainModel_PreservesAssociationType(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	parent := &domainmodel.Entity{Name: "ZzParent", Persistable: true}
	child := &domainmodel.Entity{Name: "ZzChild", Persistable: true}
	if err := b.CreateEntity(dm.ID, parent); err != nil {
		t.Fatalf("CreateEntity parent: %v", err)
	}
	if err := b.CreateEntity(dm.ID, child); err != nil {
		t.Fatalf("CreateEntity child: %v", err)
	}
	if err := b.CreateAssociation(dm.ID, &domainmodel.Association{
		Name: "ZzChild_ZzParent", ParentID: child.ID, ChildID: parent.ID,
		Type: "Reference", Owner: "Default",
	}); err != nil {
		t.Fatalf("CreateAssociation: %v", err)
	}

	// Read the domain model back and re-persist it unchanged — exactly what
	// CREATE OR MODIFY ASSOCIATION does for a different association.
	dm2, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel(2): %v", err)
	}
	var found *domainmodel.Association
	for _, a := range dm2.Associations {
		if a.Name == "ZzChild_ZzParent" {
			found = a
		}
	}
	if found == nil {
		t.Fatal("association not found on read")
	}
	if found.Type != "Reference" || found.Owner != "Default" {
		t.Fatalf("read lost Type/Owner: Type=%q Owner=%q (assocFromGen regression)", found.Type, found.Owner)
	}
	if err := b.UpdateDomainModel(dm2); err != nil {
		t.Fatalf("UpdateDomainModel: %v", err)
	}

	// Reopen: the association must still carry Type/Owner.
	b3 := New()
	if err := b3.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b3.Disconnect() })
	dm3, _ := b3.GetDomainModel(mod.ID)
	for _, a := range dm3.Associations {
		if a.Name == "ZzChild_ZzParent" {
			if a.Type != "Reference" || a.Owner != "Default" {
				t.Fatalf("UpdateDomainModel wiped Type/Owner: Type=%q Owner=%q", a.Type, a.Owner)
			}
			return
		}
	}
	t.Fatal("association missing after UpdateDomainModel")
}

// TestGetDomainModel_ReadsCrossAssociations guards against cross-module
// associations being invisible to the reader. They live in the gen
// DomainModel's separate CrossAssociations collection (not AssociationsItems),
// so domainModelFromGen must read both — otherwise SHOW/LIST ASSOCIATIONS and
// DESCRIBE MODULE silently drop every cross-module association.
//
// Also verifies the cross association survives a subsequent UpdateDomainModel
// (CREATE OR MODIFY ASSOCIATION's re-persist path), which rebuilds only the
// Entities/Associations collections and must leave CrossAssociations intact.
func TestGetDomainModel_ReadsCrossAssociations(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}

	// Local FROM entity in MyFirstModule; the TO entity is referenced BY_NAME in
	// another module (Administration.Account exists in the fixture).
	from := &domainmodel.Entity{Name: "ZzOrder", Persistable: true}
	if err := b.CreateEntity(dm.ID, from); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	ca := &domainmodel.CrossModuleAssociation{
		Name: "ZzOrder_Account", ParentID: from.ID, ChildRef: "Administration.Account",
		Type: "Reference", Owner: "Default", StorageFormat: "Column",
	}
	if err := b.CreateCrossAssociation(dm.ID, ca); err != nil {
		t.Fatalf("CreateCrossAssociation: %v", err)
	}

	// Reopen and confirm the reader surfaces it with its fields intact.
	assertCross := func(t *testing.T, b *Backend) *domainmodel.CrossModuleAssociation {
		t.Helper()
		got, err := b.GetDomainModel(mod.ID)
		if err != nil {
			t.Fatalf("GetDomainModel: %v", err)
		}
		for _, c := range got.CrossAssociations {
			if c.Name == "ZzOrder_Account" {
				return c
			}
		}
		t.Fatalf("cross association not surfaced by reader (have %d): %+v", len(got.CrossAssociations), got.CrossAssociations)
		return nil
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	got := assertCross(t, b2)
	if got.ChildRef != "Administration.Account" {
		t.Errorf("ChildRef not round-tripped: %q", got.ChildRef)
	}
	if got.Type != "Reference" || got.Owner != "Default" {
		t.Errorf("Type/Owner not round-tripped: Type=%q Owner=%q", got.Type, got.Owner)
	}

	// Re-persist the domain model (UpdateDomainModel) — the cross association
	// must survive even though UpdateDomainModel only rebuilds Associations.
	dm2, _ := b2.GetDomainModel(mod.ID)
	if err := b2.UpdateDomainModel(dm2); err != nil {
		t.Fatalf("UpdateDomainModel: %v", err)
	}
	b3 := New()
	if err := b3.Connect(proj); err != nil {
		t.Fatalf("reconnect(3): %v", err)
	}
	t.Cleanup(func() { _ = b3.Disconnect() })
	assertCross(t, b3)
}
