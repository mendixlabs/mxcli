// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// TestDeleteModule_RemovesChildUnits proves the recursive delete leaves no orphans:
// CreateModule writes a module plus its DomainModel/ModuleSecurity/ModuleSettings
// child units; after DeleteModule none of them (nor the module) may remain. An
// orphaned child unit crashes Studio Pro on load, so this is the load-bearing
// invariant of DeleteModule.
func TestDeleteModule_RemovesChildUnits(t *testing.T) {
	proj := copyFixture(t)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	m := &model.Module{Name: "ZzDeleteMe"}
	if err := b.CreateModule(m); err != nil {
		t.Fatalf("CreateModule: %v", err)
	}

	// The module plus its child units now exist.
	childIDs, err := b.collectDescendantUnitIDs(string(m.ID))
	if err != nil {
		t.Fatalf("collectDescendantUnitIDs: %v", err)
	}
	if len(childIDs) == 0 {
		t.Fatal("expected the new module to have child units (DomainModel/Security/Settings)")
	}

	if err := b.DeleteModule(m.ID); err != nil {
		t.Fatalf("DeleteModule: %v", err)
	}

	// Reopen and confirm neither the module nor any of its former children survive.
	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	if mod, _ := b2.GetModuleByName("ZzDeleteMe"); mod != nil {
		t.Errorf("module ZzDeleteMe still present after delete")
	}
	units, err := b2.ListUnits()
	if err != nil {
		t.Fatalf("ListUnits: %v", err)
	}
	live := make(map[string]bool, len(units))
	for _, u := range units {
		live[string(u.ID)] = true
	}
	if live[string(m.ID)] {
		t.Errorf("module unit %s still present", m.ID)
	}
	for _, c := range childIDs {
		if live[c] {
			t.Errorf("orphaned child unit %s survived module delete", c)
		}
	}
}
