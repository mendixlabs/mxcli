// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// TestResolveMemberChange_CrossModuleAssociationKeepsAssociationSlot guards
// against describe → exec → describe dropping cross-module associations into
// the Attribute slot on CREATE/CHANGE actions. A CREATE
// `TargetMod.Entity (OwnerMod.Assoc = $Ref)` refers to an association whose
// owning module (`OwnerMod`) is not the create target's module
// (`TargetMod`). Before the fix the resolver only looked up the target's
// module, failed to find the association, and fell through to
// `AttributeQualifiedName`, causing Studio Pro to raise CE1613 "selected
// attribute no longer exists" on the re-opened MPR.
func TestResolveMemberChange_CrossModuleAssociationKeepsAssociationSlot(t *testing.T) {
	brandModuleID := model.ID("synthetic-brand-module")
	ownerModuleID := model.ID("synthetic-owner-module")

	backend := &mock.MockBackend{
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			switch name {
			case "SyntheticBrand":
				return &model.Module{BaseElement: model.BaseElement{ID: brandModuleID}, Name: name}, nil
			case "SyntheticOwner":
				return &model.Module{BaseElement: model.BaseElement{ID: ownerModuleID}, Name: name}, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			switch id {
			case brandModuleID:
				return &domainmodel.DomainModel{
					ContainerID: brandModuleID,
					Entities: []*domainmodel.Entity{
						{Name: "Brand"},
					},
				}, nil
			case ownerModuleID:
				return &domainmodel.DomainModel{
					ContainerID: ownerModuleID,
					Associations: []*domainmodel.Association{
						{Name: "Company_Brand", Type: domainmodel.AssociationTypeReference},
					},
				}, nil
			}
			return nil, nil
		},
	}

	fb := &flowBuilder{backend: backend}
	var mc microflows.MemberChange
	fb.resolveMemberChange(&mc, "SyntheticOwner.Company_Brand", "SyntheticBrand.Brand")

	if mc.AssociationQualifiedName != "SyntheticOwner.Company_Brand" {
		t.Errorf("AssociationQualifiedName = %q, want %q — cross-module association dropped",
			mc.AssociationQualifiedName, "SyntheticOwner.Company_Brand")
	}
	if mc.AttributeQualifiedName != "" {
		t.Errorf("AttributeQualifiedName = %q, want empty — association leaked into attribute slot",
			mc.AttributeQualifiedName)
	}
}
