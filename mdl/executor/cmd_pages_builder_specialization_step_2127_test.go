// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#2127 (mendixlabs/mxcli#1808): a data grid column bound to
// `System.Session_User/Administration.Account/FullName` passed check and exec,
// and mxbuild then failed
//
//	CE1613 The selected attribute 'System.Session_User/Administration.Account/FullName' no longer exists.
//
// The path resolver read every non-final segment as an association, so the
// specialization step (`…/Administration.Account/…`) failed the lookup and the
// caller fell back to writing the whole path as one flat attribute name. The
// same happened to a step naming the association's own end
// (`System.Session_User/System.User/Name`).
//
// Studio Pro stores such a step as an EntityRefStep whose DestinationEntity is
// the specialization (measured for data sources, ako/mxcli#2055), and mxbuild
// 11.14.0 accepts it on a column's AttributeRef at 0 errors — so the writer
// emits that step, as `resolveAssociationHops` already does for data sources.
func specializationStepPB(entityContext string) *pageBuilder {
	const (
		sysID     = model.ID("mod-system")
		myID      = model.ID("mod-my")
		sessionID = model.ID("e-session")
		userID    = model.ID("e-user")
		accountID = model.ID("e-account")
		otherID   = model.ID("e-other")
	)
	dms := []*domainmodel.DomainModel{
		{
			ContainerID: sysID,
			Entities: []*domainmodel.Entity{
				{BaseElement: model.BaseElement{ID: sessionID}, Name: "Session"},
				{BaseElement: model.BaseElement{ID: userID}, Name: "User", Attributes: []*domainmodel.Attribute{{Name: "Name"}}},
			},
			Associations: []*domainmodel.Association{
				{Name: "Session_User", ParentID: sessionID, ChildID: userID, Type: domainmodel.AssociationTypeReference},
			},
		},
		{
			ContainerID: myID,
			Entities: []*domainmodel.Entity{
				{
					BaseElement:       model.BaseElement{ID: accountID},
					Name:              "Account",
					GeneralizationRef: "System.User",
					Attributes:        []*domainmodel.Attribute{{Name: "FullName"}},
				},
				{BaseElement: model.BaseElement{ID: otherID}, Name: "Other", Attributes: []*domainmodel.Attribute{{Name: "FullName"}}},
			},
		},
	}
	return &pageBuilder{
		entityContext: entityContext,
		backend: &mock.MockBackend{
			ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return dms, nil },
		},
		execCache: &executorCache{
			hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{sysID: "System", myID: "MyFirstModule"}},
		},
	}
}

func TestResolveAssociationAttributePath_SpecializationStep_2127(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantFinal string
		wantDest  string
	}{
		{"specialization step (the report)", "System.Session_User/MyFirstModule.Account/FullName", "MyFirstModule.Account.FullName", "MyFirstModule.Account"},
		{"inherited attribute through the specialization", "System.Session_User/MyFirstModule.Account/Name", "System.User.Name", "MyFirstModule.Account"},
		{"step naming the association's own end", "System.Session_User/System.User/Name", "System.User.Name", "System.User"},
		{"no step (control)", "System.Session_User/Name", "System.User.Name", "System.User"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			finalQN, steps, ok := specializationStepPB("System.Session").resolveAssociationAttributePath(tc.path)
			if !ok {
				t.Fatalf("path %q was dropped (ok=false) — the column falls back to the flat attribute %q and mxbuild fails CE1613", tc.path, tc.path)
			}
			if finalQN != tc.wantFinal {
				t.Errorf("finalQN = %q, want %q", finalQN, tc.wantFinal)
			}
			if len(steps) != 1 || steps[0].Association != "System.Session_User" || steps[0].DestinationEntity != tc.wantDest {
				t.Errorf("steps = %+v, want one {Association: System.Session_User, DestinationEntity: %s}", steps, tc.wantDest)
			}
		})
	}

	// An entity the hop cannot arrive at — neither an end of the association nor
	// a specialization of one — is not a step the runtime can follow. It stays
	// unresolved rather than being written as a step to the wrong entity.
	t.Run("entity off the association", func(t *testing.T) {
		if _, steps, ok := specializationStepPB("System.Session").resolveAssociationAttributePath("System.Session_User/MyFirstModule.Other/FullName"); ok {
			t.Errorf("resolved %+v for an entity that is not an end or specialization of System.Session_User", steps)
		}
	})
}

// The resolver is shared: a DataGrid 2 column and an input widget's
// `Attribute:` both go through it, so both get the step.
func TestSpecializationStep_ColumnAndInput_2127(t *testing.T) {
	const path = "System.Session_User/MyFirstModule.Account/FullName"

	col, err := specializationStepPB("System.Session").buildColumnSpecFromAST(&ast.WidgetV3{
		Type:       "column",
		Properties: map[string]any{"Attribute": path, "Caption": "User"},
	})
	if err != nil {
		t.Fatalf("column: %v", err)
	}
	if col.Attribute != "MyFirstModule.Account.FullName" {
		t.Errorf("column Attribute = %q, want MyFirstModule.Account.FullName — the flat path is what mxbuild reports as CE1613", col.Attribute)
	}
	if len(col.AttributeRefSteps) != 1 || col.AttributeRefSteps[0].DestinationEntity != "MyFirstModule.Account" {
		t.Errorf("column steps = %+v, want one hop arriving at MyFirstModule.Account", col.AttributeRefSteps)
	}

	attr, steps := specializationStepPB("System.Session").resolveInputAttribute(path)
	if attr != "MyFirstModule.Account.FullName" || len(steps) != 1 || steps[0].DestinationEntity != "MyFirstModule.Account" {
		t.Errorf("input attribute = %q, steps %+v; want MyFirstModule.Account.FullName over one hop to MyFirstModule.Account", attr, steps)
	}
}
