// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// TestCreateOrReplace_ViewEntityToPersistent_DeletesSourceDoc guards CE6786:
// replacing a view entity with a persistent one (CREATE OR REPLACE PERSISTENT
// ENTITY) must delete the orphaned ViewEntitySourceDocument. execCreateEntity
// only ever produces non-view entities, so the old OQL source doc would dangle.
func TestCreateOrReplace_ViewEntityToPersistent_DeletesSourceDoc(t *testing.T) {
	mod := mkModule("M")
	// Existing entity is a VIEW entity (has an OQL source).
	view := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		ContainerID: mod.ID,
		Name:        "V",
		Source:      "DomainModels$OqlViewEntitySource",
	}
	dm := mkDomainModel(mod.ID, view)

	var deletedModule, deletedDoc string
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return nil, nil },
		UpdateEntityFunc:     func(model.ID, *domainmodel.Entity) error { return nil },
		DeleteViewEntitySourceDocumentByNameFunc: func(moduleName, docName string) error {
			deletedModule, deletedDoc = moduleName, docName
			return nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	// CREATE OR REPLACE PERSISTENT ENTITY M.V (replace maps to CreateOrModify).
	err := execCreateEntity(ctx, &ast.CreateEntityStmt{
		Name:           ast.QualifiedName{Module: "M", Name: "V"},
		Kind:           ast.EntityPersistent,
		CreateOrModify: true,
		Attributes: []ast.Attribute{
			{Name: "Name", Type: ast.DataType{Kind: ast.TypeString, Length: 100}},
		},
	})
	assertNoError(t, err)
	if deletedModule != "M" || deletedDoc != "V" {
		t.Fatalf("expected DeleteViewEntitySourceDocumentByName(M, V); got (%q, %q) — orphaned VESDoc (CE6786)", deletedModule, deletedDoc)
	}
}
