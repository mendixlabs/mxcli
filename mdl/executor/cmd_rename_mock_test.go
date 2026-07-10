// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

// ---------------------------------------------------------------------------
// Not connected
// ---------------------------------------------------------------------------

func TestRename_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return false }}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execRename(ctx, &ast.RenameStmt{
		ObjectType: "entity",
		Name:       ast.QualifiedName{Module: "MyModule", Name: "OldName"},
		NewName:    "NewName",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not connected")
}

// ---------------------------------------------------------------------------
// Unsupported type
// ---------------------------------------------------------------------------

func TestRename_UnsupportedType(t *testing.T) {
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return true }}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execRename(ctx, &ast.RenameStmt{
		ObjectType: "snippet",
		Name:       ast.QualifiedName{Module: "M", Name: "N"},
		NewName:    "X",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not supported")
}

// ---------------------------------------------------------------------------
// Rename entity — happy path
// ---------------------------------------------------------------------------

func TestRename_Entity_Success(t *testing.T) {
	mod := mkModule("MyModule")
	ent := mkEntity(mod.ID, "OldEntity")
	dm := mkDomainModel(mod.ID, ent)
	dmUpdated := false
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		RenameReferencesFunc: func(old, new string, dryRun bool) ([]types.RenameHit, error) {
			return []types.RenameHit{{UnitID: "u1", Name: "SomeDoc", Count: 2}}, nil
		},
		UpdateDomainModelFunc: func(d *domainmodel.DomainModel) error { dmUpdated = true; return nil },
	}
	h := mkHierarchy(mod)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execRename(ctx, &ast.RenameStmt{
		ObjectType: "entity",
		Name:       ast.QualifiedName{Module: "MyModule", Name: "OldEntity"},
		NewName:    "NewEntity",
	}))
	if !dmUpdated {
		t.Error("Expected UpdateDomainModel to be called")
	}
	assertContainsStr(t, buf.String(), "Renamed entity")
	assertContainsStr(t, buf.String(), "MyModule.OldEntity")
	assertContainsStr(t, buf.String(), "MyModule.NewEntity")
	assertContainsStr(t, buf.String(), "Updated 2 reference(s)")
}

// ---------------------------------------------------------------------------
// Rename entity — not found
// ---------------------------------------------------------------------------

func TestRename_Entity_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	dm := mkDomainModel(mod.ID) // no entities
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execRename(ctx, &ast.RenameStmt{
		ObjectType: "entity",
		Name:       ast.QualifiedName{Module: "MyModule", Name: "Missing"},
		NewName:    "New",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}

// ---------------------------------------------------------------------------
// Rename entity — dry run
// ---------------------------------------------------------------------------

func TestRename_Entity_DryRun(t *testing.T) {
	mod := mkModule("MyModule")
	ent := mkEntity(mod.ID, "OldEntity")
	dm := mkDomainModel(mod.ID, ent)
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		RenameReferencesFunc: func(old, new string, dryRun bool) ([]types.RenameHit, error) {
			if !dryRun {
				t.Error("Expected dryRun=true")
			}
			return []types.RenameHit{{UnitID: "u1", Name: "Page1", UnitType: "Pages$Page", Count: 1}}, nil
		},
	}
	h := mkHierarchy(mod)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execRename(ctx, &ast.RenameStmt{
		ObjectType: "entity",
		Name:       ast.QualifiedName{Module: "MyModule", Name: "OldEntity"},
		NewName:    "NewEntity",
		DryRun:     true,
	}))
	assertContainsStr(t, buf.String(), "Would rename")
	assertContainsStr(t, buf.String(), "Page1")
}

// ---------------------------------------------------------------------------
// Rename microflow (document type) — happy path
// ---------------------------------------------------------------------------

func TestRename_Microflow_Success(t *testing.T) {
	mod := mkModule("MyModule")
	mf := mkMicroflow(mod.ID, "OldMF")
	renameCalled := false
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:    func() ([]*types.FolderInfo, error) { return nil, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		RenameReferencesFunc: func(old, new string, dryRun bool) ([]types.RenameHit, error) {
			return nil, nil
		},
		RenameDocumentByNameFunc: func(mod, old, new string) error {
			renameCalled = true
			return nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execRename(ctx, &ast.RenameStmt{
		ObjectType: "microflow",
		Name:       ast.QualifiedName{Module: "MyModule", Name: "OldMF"},
		NewName:    "NewMF",
	}))
	if !renameCalled {
		t.Error("Expected RenameDocumentByName to be called")
	}
	assertContainsStr(t, buf.String(), "Renamed microflow")
}

// ---------------------------------------------------------------------------
// Rename page — not found
// ---------------------------------------------------------------------------

func TestRename_Page_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return nil, nil },
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execRename(ctx, &ast.RenameStmt{
		ObjectType: "page",
		Name:       ast.QualifiedName{Module: "MyModule", Name: "Missing"},
		NewName:    "New",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}

// ---------------------------------------------------------------------------
// Rename module — happy path
// ---------------------------------------------------------------------------

func TestRename_Module_Success(t *testing.T) {
	mod := mkModule("OldModule")
	moduleUpdated := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		RenameReferencesFunc: func(old, new string, dryRun bool) ([]types.RenameHit, error) {
			return nil, nil
		},
		UpdateModuleFunc: func(m *model.Module) error {
			moduleUpdated = true
			if m.Name != "NewModule" {
				t.Errorf("Expected new name NewModule, got %s", m.Name)
			}
			return nil
		},
	}
	h := mkHierarchy(mod)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execRename(ctx, &ast.RenameStmt{
		ObjectType: "module",
		Name:       ast.QualifiedName{Module: "OldModule"},
		NewName:    "NewModule",
	}))
	if !moduleUpdated {
		t.Error("Expected UpdateModule to be called")
	}
	assertContainsStr(t, buf.String(), "Renamed module")
}

// ---------------------------------------------------------------------------
// Rename association — happy path
// ---------------------------------------------------------------------------

func TestRename_Association_Success(t *testing.T) {
	mod := mkModule("MyModule")
	ent1 := mkEntity(mod.ID, "Parent")
	ent2 := mkEntity(mod.ID, "Child")
	assoc := mkAssociation(mod.ID, "OldAssoc", ent1.ID, ent2.ID)
	dm := mkDomainModel(mod.ID, ent1, ent2)
	dm.Associations = []*domainmodel.Association{assoc}
	dmUpdated := false
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		RenameReferencesFunc: func(old, new string, dryRun bool) ([]types.RenameHit, error) {
			return nil, nil
		},
		UpdateDomainModelFunc: func(d *domainmodel.DomainModel) error { dmUpdated = true; return nil },
	}
	h := mkHierarchy(mod)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execRename(ctx, &ast.RenameStmt{
		ObjectType: "association",
		Name:       ast.QualifiedName{Module: "MyModule", Name: "OldAssoc"},
		NewName:    "NewAssoc",
	}))
	if !dmUpdated {
		t.Error("Expected UpdateDomainModel to be called")
	}
	assertContainsStr(t, buf.String(), "Renamed association")
}

// ---------------------------------------------------------------------------
// Rename association — not found
// ---------------------------------------------------------------------------

func TestRename_Association_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	dm := mkDomainModel(mod.ID) // no associations
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execRename(ctx, &ast.RenameStmt{
		ObjectType: "association",
		Name:       ast.QualifiedName{Module: "MyModule", Name: "Missing"},
		NewName:    "New",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}

// ---------------------------------------------------------------------------
// Rename backend error
// ---------------------------------------------------------------------------

func TestRename_Entity_BackendError(t *testing.T) {
	mod := mkModule("MyModule")
	ent := mkEntity(mod.ID, "Ent")
	dm := mkDomainModel(mod.ID, ent)
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		RenameReferencesFunc: func(old, new string, dryRun bool) ([]types.RenameHit, error) {
			return nil, fmt.Errorf("scan error")
		},
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execRename(ctx, &ast.RenameStmt{
		ObjectType: "entity",
		Name:       ast.QualifiedName{Module: "MyModule", Name: "Ent"},
		NewName:    "New",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "scan references")
}

// ---------------------------------------------------------------------------
// Rename java action — happy path
// ---------------------------------------------------------------------------

func TestRename_JavaAction_Success(t *testing.T) {
	mod := mkModule("MyModule")
	ja := &types.JavaAction{
		BaseElement: model.BaseElement{ID: nextID("ja")},
		ContainerID: mod.ID,
		Name:        "OldHelper",
	}
	documentRenamed := false
	sourceRenamed := false
	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListModulesFunc:     func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:     func() ([]*types.FolderInfo, error) { return nil, nil },
		ListJavaActionsFunc: func() ([]*types.JavaAction, error) { return []*types.JavaAction{ja}, nil },
		RenameReferencesFunc: func(old, new string, dryRun bool) ([]types.RenameHit, error) {
			return []types.RenameHit{{UnitID: "u1", Name: "SomeMF", Count: 1}}, nil
		},
		RenameDocumentByNameFunc: func(module, old, newName string) error {
			documentRenamed = true
			return nil
		},
		RenameJavaSourceFileFunc: func(module, old, newName string) error {
			sourceRenamed = true
			return nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, ja.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execRename(ctx, &ast.RenameStmt{
		ObjectType: "javaaction",
		Name:       ast.QualifiedName{Module: "MyModule", Name: "OldHelper"},
		NewName:    "NewHelper",
	}))
	if !documentRenamed {
		t.Error("Expected RenameDocumentByName to be called")
	}
	if !sourceRenamed {
		t.Error("Expected RenameJavaSourceFile to be called")
	}
	assertContainsStr(t, buf.String(), "Renamed java action")
	assertContainsStr(t, buf.String(), "MyModule.OldHelper")
	assertContainsStr(t, buf.String(), "MyModule.NewHelper")
}

// ---------------------------------------------------------------------------
// Rename java action — not found
// ---------------------------------------------------------------------------

func TestRename_JavaAction_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListModulesFunc:     func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:     func() ([]*types.FolderInfo, error) { return nil, nil },
		ListJavaActionsFunc: func() ([]*types.JavaAction, error) { return nil, nil },
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execRename(ctx, &ast.RenameStmt{
		ObjectType: "javaaction",
		Name:       ast.QualifiedName{Module: "MyModule", Name: "Missing"},
		NewName:    "New",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}

// ---------------------------------------------------------------------------
// Rename workflow — happy path
// ---------------------------------------------------------------------------

func TestRename_Workflow_Success(t *testing.T) {
	mod := mkModule("BPModule")
	wf := mkWorkflow(mod.ID, "OldProcess")
	renameCalled := false
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:   func() ([]*types.FolderInfo, error) { return nil, nil },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
		RenameReferencesFunc: func(old, new string, dryRun bool) ([]types.RenameHit, error) {
			return nil, nil
		},
		RenameDocumentByNameFunc: func(module, old, newName string) error {
			renameCalled = true
			return nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, wf.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execRename(ctx, &ast.RenameStmt{
		ObjectType: "workflow",
		Name:       ast.QualifiedName{Module: "BPModule", Name: "OldProcess"},
		NewName:    "NewProcess",
	}))
	if !renameCalled {
		t.Error("Expected RenameDocumentByName to be called")
	}
	assertContainsStr(t, buf.String(), "Renamed workflow")
	assertContainsStr(t, buf.String(), "BPModule.OldProcess")
	assertContainsStr(t, buf.String(), "BPModule.NewProcess")
}

// ---------------------------------------------------------------------------
// Rename workflow — not found
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Collision detection — renaming to an existing name must fail
// ---------------------------------------------------------------------------

func TestRename_Nanoflow_CollisionError(t *testing.T) {
	mod := mkModule("MyModule")
	nf1 := mkNanoflow(mod.ID, "NF1")
	nf2 := mkNanoflow(mod.ID, "NF2")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) {
			return []*microflows.Nanoflow{nf1, nf2}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, nf1.ContainerID, mod.ID)
	withContainer(h, nf2.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execRename(ctx, &ast.RenameStmt{
		ObjectType: "nanoflow",
		Name:       ast.QualifiedName{Module: "MyModule", Name: "NF1"},
		NewName:    "NF2",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "already exists")
}

func TestRename_Microflow_CollisionError(t *testing.T) {
	mod := mkModule("MyModule")
	mf1 := mkMicroflow(mod.ID, "MF1")
	mf2 := mkMicroflow(mod.ID, "MF2")
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:    func() ([]*types.FolderInfo, error) { return nil, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf1, mf2}, nil },
	}
	h := mkHierarchy(mod)
	withContainer(h, mf1.ContainerID, mod.ID)
	withContainer(h, mf2.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execRename(ctx, &ast.RenameStmt{
		ObjectType: "microflow",
		Name:       ast.QualifiedName{Module: "MyModule", Name: "MF1"},
		NewName:    "MF2",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "already exists")
}

func TestRename_Entity_CollisionError(t *testing.T) {
	mod := mkModule("MyModule")
	ent1 := mkEntity(mod.ID, "EntityA")
	ent2 := mkEntity(mod.ID, "EntityB")
	dm := mkDomainModel(mod.ID, ent1, ent2)
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execRename(ctx, &ast.RenameStmt{
		ObjectType: "entity",
		Name:       ast.QualifiedName{Module: "MyModule", Name: "EntityA"},
		NewName:    "EntityB",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "already exists")
}

func TestRename_Workflow_NotFound(t *testing.T) {
	mod := mkModule("BPModule")
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:   func() ([]*types.FolderInfo, error) { return nil, nil },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return nil, nil },
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execRename(ctx, &ast.RenameStmt{
		ObjectType: "workflow",
		Name:       ast.QualifiedName{Module: "BPModule", Name: "Missing"},
		NewName:    "New",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}
