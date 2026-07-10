// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

func TestShowAssociations_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	ent1 := mkEntity(mod.ID, "Order")
	ent2 := mkEntity(mod.ID, "Customer")
	assoc := mkAssociation(mod.ID, "Order_Customer", ent1.ID, ent2.ID)

	dm := &domainmodel.DomainModel{
		BaseElement:  model.BaseElement{ID: nextID("dm")},
		ContainerID:  mod.ID,
		Entities:     []*domainmodel.Entity{ent1, ent2},
		Associations: []*domainmodel.Association{assoc},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listAssociations(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "MyModule.Order_Customer")
	assertContainsStr(t, out, "MyModule.Order")
	assertContainsStr(t, out, "MyModule.Customer")
	assertContainsStr(t, out, "Reference")
	assertContainsStr(t, out, "(1 associations)")
}

func TestShowAssociations_Mock_FilterByModule(t *testing.T) {
	mod1 := mkModule("Sales")
	mod2 := mkModule("HR")
	ent1 := mkEntity(mod1.ID, "Order")
	ent2 := mkEntity(mod1.ID, "Product")
	ent3 := mkEntity(mod2.ID, "Employee")
	ent4 := mkEntity(mod2.ID, "Department")

	dm1 := &domainmodel.DomainModel{
		BaseElement:  model.BaseElement{ID: nextID("dm")},
		ContainerID:  mod1.ID,
		Entities:     []*domainmodel.Entity{ent1, ent2},
		Associations: []*domainmodel.Association{mkAssociation(mod1.ID, "Order_Product", ent1.ID, ent2.ID)},
	}
	dm2 := &domainmodel.DomainModel{
		BaseElement:  model.BaseElement{ID: nextID("dm")},
		ContainerID:  mod2.ID,
		Entities:     []*domainmodel.Entity{ent3, ent4},
		Associations: []*domainmodel.Association{mkAssociation(mod2.ID, "Employee_Dept", ent3.ID, ent4.ID)},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod1, mod2}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm1, dm2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listAssociations(ctx, "HR"))

	out := buf.String()
	assertNotContainsStr(t, out, "Sales.Order_Product")
	assertContainsStr(t, out, "HR.Employee_Dept")
	assertContainsStr(t, out, "(1 associations)")
}

// NOTE: listAssociations and describeAssociation have no Connected() guard.
// They call backend directly — error propagation is the only failure mode.

func TestShowAssociations_BackendError(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return nil, fmt.Errorf("connection lost") },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, listAssociations(ctx, ""))
}

func TestShowAssociations_JSON(t *testing.T) {
	mod := mkModule("App")
	ent1 := mkEntity(mod.ID, "A")
	ent2 := mkEntity(mod.ID, "B")
	assoc := mkAssociation(mod.ID, "A_B", ent1.ID, ent2.ID)

	dm := &domainmodel.DomainModel{
		BaseElement:  model.BaseElement{ID: nextID("dm")},
		ContainerID:  mod.ID,
		Entities:     []*domainmodel.Entity{ent1, ent2},
		Associations: []*domainmodel.Association{assoc},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON))
	assertNoError(t, listAssociations(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "A_B")
}

func TestCreateAssociation_OrModify_UpdatesInPlace(t *testing.T) {
	mod := mkModule("MyModule")
	ent1 := mkEntity(mod.ID, "Order")
	ent2 := mkEntity(mod.ID, "Customer")
	assocID := nextID("assoc")
	existingAssoc := mkAssociation(mod.ID, "Order_Customer", ent1.ID, ent2.ID)
	existingAssoc.ID = assocID

	dm := &domainmodel.DomainModel{
		BaseElement:  model.BaseElement{ID: nextID("dm")},
		ContainerID:  mod.ID,
		Entities:     []*domainmodel.Entity{ent1, ent2},
		Associations: []*domainmodel.Association{existingAssoc},
	}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	withContainer(h, ent1.ContainerID, dm.ID)
	withContainer(h, ent2.ContainerID, dm.ID)

	updateCalled := false
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		UpdateDomainModelFunc: func(d *domainmodel.DomainModel) error {
			updateCalled = true
			return nil
		},
		ReconcileMemberAccessesFunc: func(dmID model.ID, moduleName string) (int, error) { return 0, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execCreateAssociation(ctx, &ast.CreateAssociationStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "Order_Customer"},
		Parent:         ast.QualifiedName{Module: "MyModule", Name: "Order"},
		Child:          ast.QualifiedName{Module: "MyModule", Name: "Customer"},
		Type:           ast.AssocReference,
		CreateOrModify: true,
	})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Modified association")
	if !updateCalled {
		t.Fatal("UpdateDomainModel was not called")
	}
	// Verify the existing association UUID is preserved
	if existingAssoc.ID != assocID {
		t.Errorf("Association ID changed from %q to %q", assocID, existingAssoc.ID)
	}
}

// Issue #389 — cross-module CREATE ASSOCIATION must also reject duplicates.
func TestCreateAssociation_CrossModule_AlreadyExists_Issue389(t *testing.T) {
	mod1 := mkModule("ModA")
	mod2 := mkModule("ModB")
	ent1 := mkEntity(mod1.ID, "Order")
	ent2 := mkEntity(mod2.ID, "Product")

	existingCA := &domainmodel.CrossModuleAssociation{
		BaseElement: model.BaseElement{ID: nextID("ca")},
		ContainerID: nextID("dm1"),
		Name:        "Order_Product",
		ChildRef:    "ModB.Product",
	}
	dm1 := &domainmodel.DomainModel{
		BaseElement:       model.BaseElement{ID: nextID("dm1")},
		ContainerID:       mod1.ID,
		Entities:          []*domainmodel.Entity{ent1},
		CrossAssociations: []*domainmodel.CrossModuleAssociation{existingCA},
	}
	dm2 := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm2")},
		ContainerID: mod2.ID,
		Entities:    []*domainmodel.Entity{ent2},
	}
	h := mkHierarchy(mod1, mod2)
	withContainer(h, dm1.ID, mod1.ID)
	withContainer(h, dm2.ID, mod2.ID)
	withContainer(h, ent1.ContainerID, dm1.ID)
	withContainer(h, ent2.ContainerID, dm2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod1, mod2}, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{dm1, dm2}, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			if id == mod1.ID {
				return dm1, nil
			}
			return dm2, nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execCreateAssociation(ctx, &ast.CreateAssociationStmt{
		Name:   ast.QualifiedName{Module: "ModA", Name: "Order_Product"},
		Parent: ast.QualifiedName{Module: "ModA", Name: "Order"},
		Child:  ast.QualifiedName{Module: "ModB", Name: "Product"},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "already exists")
}

func TestCreateAssociation_AlreadyExists_NoOrModify(t *testing.T) {
	mod := mkModule("MyModule")
	ent1 := mkEntity(mod.ID, "Order")
	ent2 := mkEntity(mod.ID, "Customer")
	existingAssoc := mkAssociation(mod.ID, "Order_Customer", ent1.ID, ent2.ID)

	dm := &domainmodel.DomainModel{
		BaseElement:  model.BaseElement{ID: nextID("dm")},
		ContainerID:  mod.ID,
		Entities:     []*domainmodel.Entity{ent1, ent2},
		Associations: []*domainmodel.Association{existingAssoc},
	}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	withContainer(h, ent1.ContainerID, dm.ID)
	withContainer(h, ent2.ContainerID, dm.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execCreateAssociation(ctx, &ast.CreateAssociationStmt{
		Name:   ast.QualifiedName{Module: "MyModule", Name: "Order_Customer"},
		Parent: ast.QualifiedName{Module: "MyModule", Name: "Order"},
		Child:  ast.QualifiedName{Module: "MyModule", Name: "Customer"},
	})
	assertError(t, err)
}
