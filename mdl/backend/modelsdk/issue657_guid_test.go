// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestIssue657_AlterPreservesEntityGUIDs guards GitHub issue #657: ALTER ENTITY
// ADD ATTRIBUTE must not change any entity's DataStorageGuid — neither the ALTER
// target (rebuilt via entityToGen, which the semantic model can't supply a GUID
// for) nor its siblings (which pass through as raw gen elements). The entity GUID
// is the stable cross-reference identity; rewriting it dangles inherited members
// and grid columns in other modules (CE1613). The executor routes ALTER ADD
// ATTRIBUTE through Backend.UpdateEntity (cmd_entities.go), so we exercise that.
func TestIssue657_AlterPreservesEntityGUIDs(t *testing.T) {
	proj := copyFixture(t)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Find a domain model with at least two entities so we can verify both the
	// ALTER target and an untouched sibling.
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	var dm *domainmodel.DomainModel
	for _, d := range dms {
		if len(d.Entities) >= 2 {
			dm = d
			break
		}
	}
	if dm == nil {
		t.Skip("no domain model with >= 2 entities in fixture")
	}
	targetID := dm.Entities[0].ID
	siblingID := dm.Entities[1].ID

	// The entity GUID lives in the raw BSON under the "GUID" key (binary), not in
	// the misnamed DataStorageGuid property — read it from raw so we observe what
	// actually persists.
	guidOf := func(b *Backend, dmID, entID model.ID) string {
		t.Helper()
		gdm, err := b.loadDomainModelGen(dmID)
		if err != nil {
			t.Fatalf("loadDomainModelGen: %v", err)
		}
		ge := findGenEntity(gdm, entID)
		if ge == nil {
			t.Fatalf("gen entity %s not found", entID)
		}
		return rawKeyHex(t, ge.Raw(), "GUID")
	}
	idOf := func(b *Backend, dmID, entID model.ID) string {
		t.Helper()
		gdm, err := b.loadDomainModelGen(dmID)
		if err != nil {
			t.Fatalf("loadDomainModelGen: %v", err)
		}
		ge := findGenEntity(gdm, entID)
		if ge == nil {
			t.Fatalf("gen entity %s not found", entID)
		}
		return rawKeyHex(t, ge.Raw(), "$ID")
	}

	targetGUIDBefore := guidOf(b, dm.ID, targetID)
	siblingGUIDBefore := guidOf(b, dm.ID, siblingID)
	if targetGUIDBefore == "" || siblingGUIDBefore == "" {
		t.Fatalf("fixture entities have empty GUIDs (target=%q sibling=%q); cannot verify preservation", targetGUIDBefore, siblingGUIDBefore)
	}
	if targetGUIDBefore == idOf(b, dm.ID, targetID) {
		t.Fatalf("fixture target GUID equals its $ID; need GUID != $ID to detect the #657 conflation")
	}

	// Simulate ALTER ENTITY ADD ATTRIBUTE on the target: append an attribute and
	// persist via UpdateEntity (the path cmd_entities.go uses).
	target := dm.Entities[0]
	target.Attributes = append(target.Attributes, &domainmodel.Attribute{
		Name: "Issue657NewAttr",
		Type: &domainmodel.StringAttributeType{Length: 20},
	})
	if err := b.UpdateEntity(dm.ID, target); err != nil {
		t.Fatalf("UpdateEntity: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	// Reopen and confirm the write hit disk with GUIDs intact.
	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	if got := guidOf(b2, dm.ID, targetID); got != targetGUIDBefore {
		t.Errorf("target entity GUID changed on ALTER: before=%q after=%q (issue #657)", targetGUIDBefore, got)
	}
	if got := guidOf(b2, dm.ID, siblingID); got != siblingGUIDBefore {
		t.Errorf("sibling entity GUID changed on ALTER: before=%q after=%q (issue #657)", siblingGUIDBefore, got)
	}

	// And the ALTER must actually have landed — GUID preservation is worthless if
	// the new attribute was dropped.
	dms2, err := b2.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels after reopen: %v", err)
	}
	var found bool
	for _, d := range dms2 {
		if d.ID != dm.ID {
			continue
		}
		for _, e := range d.Entities {
			if e.ID != targetID {
				continue
			}
			for _, a := range e.Attributes {
				if a.Name == "Issue657NewAttr" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("new attribute Issue657NewAttr not persisted on ALTER target")
	}
}

// rawKeyHex returns the hex of the binary value at key in raw, or "" if absent.
func rawKeyHex(t *testing.T, raw bson.Raw, key string) string {
	t.Helper()
	if raw == nil {
		return ""
	}
	rv, err := raw.LookupErr(key)
	if err != nil {
		return ""
	}
	_, data := rv.Binary()
	return fmt.Sprintf("%x", data)
}
