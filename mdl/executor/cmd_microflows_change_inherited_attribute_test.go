// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestResolveMemberChange_BareInheritedAttributeUsesDeclaringEntity(t *testing.T) {
	appModuleID := model.ID("synthetic-app-module")
	baseModuleID := model.ID("synthetic-base-module")
	backend := &mock.MockBackend{
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			switch name {
			case "SyntheticApp":
				return &model.Module{BaseElement: model.BaseElement{ID: appModuleID}, Name: name}, nil
			case "SyntheticBase":
				return &model.Module{BaseElement: model.BaseElement{ID: baseModuleID}, Name: name}, nil
			default:
				return nil, nil
			}
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			switch id {
			case appModuleID:
				return &domainmodel.DomainModel{
					ContainerID: appModuleID,
					Entities: []*domainmodel.Entity{
						{
							Name:              "Account",
							GeneralizationRef: "SyntheticBase.User",
						},
					},
				}, nil
			case baseModuleID:
				return &domainmodel.DomainModel{
					ContainerID: baseModuleID,
					Entities: []*domainmodel.Entity{
						{
							Name: "User",
							Attributes: []*domainmodel.Attribute{
								{Name: "CanUseWebService", Type: &domainmodel.BooleanAttributeType{}},
							},
						},
					},
				}, nil
			default:
				return nil, nil
			}
		},
	}

	fb := &flowBuilder{backend: backend}
	mc := &microflows.MemberChange{}

	fb.resolveMemberChange(mc, "CanUseWebService", "SyntheticApp.Account")

	if got, want := mc.AttributeQualifiedName, "SyntheticBase.User.CanUseWebService"; got != want {
		t.Fatalf("AttributeQualifiedName = %q, want %q", got, want)
	}
	if mc.AssociationQualifiedName != "" {
		t.Fatalf("AssociationQualifiedName = %q, want empty", mc.AssociationQualifiedName)
	}
}
