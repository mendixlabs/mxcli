// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// TestFindEntity_ResolvesSystemModuleEntity guards against issue #610:
// `create association ... to System.User` passed `mxcli check`/`diff` but failed
// at `mxcli exec` with "child entity not found: System.User".
//
// The virtual System domain model is appended by the reader but is not a real
// unit, so its container (the System module) is absent from the hierarchy's
// container-parent map. The buggy findEntity resolved the module name via
// h.FindModuleID(dm.ID) — which, for the virtual DM, fell back to the DM's own
// ID and produced an empty module name, so System.User never matched. The fix
// resolves the module name from dm.ContainerID, matching buildEntityQualifiedNames.
func TestFindEntity_ResolvesSystemModuleEntity(t *testing.T) {
	const (
		systemModuleID = model.ID("00000000-0000-0000-0000-000000000001")
		systemDMID     = model.ID("00000000-0000-0000-0000-000000000002")
	)

	// Hierarchy knows the System module exists, but — like production for the
	// virtual System DM — there is NO container-parent link from the domain
	// model unit up to the module. This is the condition that broke findEntity.
	h := &ContainerHierarchy{
		moduleIDs:       map[model.ID]bool{systemModuleID: true},
		moduleNames:     map[model.ID]string{systemModuleID: "System"},
		containerParent: map[model.ID]model.ID{},
		folderNames:     map[model.ID]string{},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{
				{
					BaseElement: model.BaseElement{ID: systemDMID},
					ContainerID: systemModuleID,
					Entities: []*domainmodel.Entity{
						{BaseElement: model.BaseElement{ID: "sys-user"}, Name: "User"},
					},
				},
			}, nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	ent, err := findEntity(ctx, "System", "User")
	if err != nil {
		t.Fatalf("findEntity(System, User) returned error: %v — System-module entities must resolve (issue #610)", err)
	}
	if ent == nil || ent.Name != "User" {
		t.Fatalf("findEntity(System, User) = %+v, want entity named User", ent)
	}
}
