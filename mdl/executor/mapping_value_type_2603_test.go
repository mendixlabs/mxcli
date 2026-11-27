// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A mapping value bound to the system member createdDate (or changedDate) got
// DataType String: the member is not in the entity's attribute list, so the
// resolver fell back to its default, and mx check failed CE0227 + CE5015
// against the DateTime schema element (ako/mxcli#2603, upstream
// mendixlabs/mxcli#1954).
func TestResolveAttributeType_SystemDates_2603(t *testing.T) {
	b := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			return &model.Module{BaseElement: model.BaseElement{ID: "m"}, Name: name}, nil
		},
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) {
			return &domainmodel.DomainModel{Entities: []*domainmodel.Entity{{
				Name: "Entity", HasCreatedDate: true, HasChangedDate: true,
				Attributes: []*domainmodel.Attribute{{Name: "Key", Type: &domainmodel.AutoNumberAttributeType{}}},
			}}}, nil
		},
	}
	for member, want := range map[string]string{"createdDate": "DateTime", "changedDate": "DateTime", "Key": "AutoNumber"} {
		if got := resolveAttributeType("M.Entity", member, b); got != want {
			t.Errorf("%s: %q, want %q", member, got, want)
		}
	}
	// Control: an unknown member keeps the documented String fallback.
	if got := resolveAttributeType("M.Entity", "Nope", b); got != "String" {
		t.Errorf("unknown member: %q, want String", got)
	}
}
