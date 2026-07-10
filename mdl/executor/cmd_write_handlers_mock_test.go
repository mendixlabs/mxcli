// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"github.com/JordtenBulte-OLC/mxcli/sdk/security"
)

func TestExecCreateModule_Mock(t *testing.T) {
	called := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return nil, nil // no existing modules
		},
		CreateModuleFunc: func(m *model.Module) error {
			called = true
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	err := execCreateModule(ctx, &ast.CreateModuleStmt{Name: "NewModule"})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Created module: NewModule")
	if !called {
		t.Fatal("CreateModuleFunc was not called")
	}
}

func TestExecDropEnumeration_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	enum := mkEnumeration(mod.ID, "Status", "Active", "Inactive")

	called := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) {
			return []*model.Enumeration{enum}, nil
		},
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		DeleteEnumerationFunc: func(id model.ID) error {
			called = true
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	err := execDropEnumeration(ctx, &ast.DropEnumerationStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "Status"},
	})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Dropped enumeration:")
	if !called {
		t.Fatal("DeleteEnumerationFunc was not called")
	}
}

func TestExecCreateEnumeration_Mock(t *testing.T) {
	mod := mkModule("MyModule")

	h := mkHierarchy(mod)

	called := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) {
			return nil, nil // no duplicates
		},
		CreateEnumerationFunc: func(e *model.Enumeration) error {
			called = true
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execCreateEnumeration(ctx, &ast.CreateEnumerationStmt{
		Name:   ast.QualifiedName{Module: "MyModule", Name: "Color"},
		Values: []ast.EnumValue{{Name: "Red", Caption: "Red"}, {Name: "Blue", Caption: "Blue"}},
	})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Created enumeration:")
	if !called {
		t.Fatal("CreateEnumerationFunc was not called")
	}
}

func TestExecDropEntity_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	ent := mkEntity(mod.ID, "Customer")
	dm := mkDomainModel(mod.ID, ent)

	called := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		GetDomainModelFunc: func(moduleID model.ID) (*domainmodel.DomainModel, error) {
			return dm, nil
		},
		DeleteEntityFunc: func(domainModelID model.ID, entityID model.ID) error {
			called = true
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	err := execDropEntity(ctx, &ast.DropEntityStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "Customer"},
	})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Dropped entity:")
	if !called {
		t.Fatal("DeleteEntityFunc was not called")
	}
}

func TestExecDropMicroflow_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	mf := mkMicroflow(mod.ID, "DoSomething")

	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)

	called := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{mf}, nil
		},
		DeleteMicroflowFunc: func(id model.ID) error {
			called = true
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execDropMicroflow(ctx, &ast.DropMicroflowStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "DoSomething"},
	})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Dropped microflow:")
	if !called {
		t.Fatal("DeleteMicroflowFunc was not called")
	}
}

func TestExecDropPage_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "HomePage")

	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)

	called := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPagesFunc: func() ([]*pages.Page, error) {
			return []*pages.Page{pg}, nil
		},
		DeletePageFunc: func(id model.ID) error {
			called = true
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execDropPage(ctx, &ast.DropPageStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "HomePage"},
	})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Dropped page")
	if !called {
		t.Fatal("DeletePageFunc was not called")
	}
}

func TestExecDropSnippet_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	snp := mkSnippet(mod.ID, "HeaderSnippet")

	h := mkHierarchy(mod)
	withContainer(h, snp.ContainerID, mod.ID)

	called := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) {
			return []*pages.Snippet{snp}, nil
		},
		DeleteSnippetFunc: func(id model.ID) error {
			called = true
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execDropSnippet(ctx, &ast.DropSnippetStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "HeaderSnippet"},
	})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Dropped snippet")
	if !called {
		t.Fatal("DeleteSnippetFunc was not called")
	}
}

func TestExecDropAssociation_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	ent1 := mkEntity(mod.ID, "Order")
	ent2 := mkEntity(mod.ID, "Customer")
	assoc := mkAssociation(mod.ID, "Order_Customer", ent1.ID, ent2.ID)

	dm := mkDomainModel(mod.ID, ent1, ent2)
	dm.Associations = []*domainmodel.Association{assoc}

	called := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		GetDomainModelFunc: func(moduleID model.ID) (*domainmodel.DomainModel, error) {
			return dm, nil
		},
		DeleteAssociationFunc: func(domainModelID model.ID, assocID model.ID) error {
			called = true
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	err := execDropAssociation(ctx, &ast.DropAssociationStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "Order_Customer"},
	})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Dropped association:")
	if !called {
		t.Fatal("DeleteAssociationFunc was not called")
	}
}

func TestExecDropJavaAction_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	jaID := nextID("ja")
	ja := &types.JavaAction{
		BaseElement: model.BaseElement{ID: jaID},
		ContainerID: mod.ID,
		Name:        "MyAction",
	}

	h := mkHierarchy(mod)

	called := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListJavaActionsFunc: func() ([]*types.JavaAction, error) {
			return []*types.JavaAction{ja}, nil
		},
		DeleteJavaActionFunc: func(id model.ID) error {
			called = true
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execDropJavaAction(ctx, &ast.DropJavaActionStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "MyAction"},
	})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Dropped java action:")
	if !called {
		t.Fatal("DeleteJavaActionFunc was not called")
	}
}

func TestExecDropFolder_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	folderID := nextID("folder")
	folder := &types.FolderInfo{
		ID:          folderID,
		ContainerID: mod.ID,
		Name:        "Resources",
	}

	h := mkHierarchy(mod)
	withContainer(h, folderID, mod.ID)

	called := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		ListFoldersFunc: func() ([]*types.FolderInfo, error) {
			return []*types.FolderInfo{folder}, nil
		},
		DeleteFolderFunc: func(id model.ID) error {
			called = true
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execDropFolder(ctx, &ast.DropFolderStmt{
		FolderPath: "Resources",
		Module:     "MyModule",
	})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Dropped folder:")
	if !called {
		t.Fatal("DeleteFolderFunc was not called")
	}
}

func TestExecCreateModule_Mock_AlreadyExists(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{Name: "Existing"}}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, execCreateModule(ctx, &ast.CreateModuleStmt{Name: "Existing"}))
	assertContainsStr(t, buf.String(), "already exists")
}

func TestExecDropEnumeration_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) {
			return nil, nil
		},
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, execDropEnumeration(ctx, &ast.DropEnumerationStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "NonExistent"},
	}))
}

func TestExecDropEntity_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	dm := mkDomainModel(mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		GetDomainModelFunc: func(moduleID model.ID) (*domainmodel.DomainModel, error) {
			return dm, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, execDropEntity(ctx, &ast.DropEntityStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "NonExistent"},
	}))
}

func TestExecDropMicroflow_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, execDropMicroflow(ctx, &ast.DropMicroflowStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "NonExistent"},
	}))
}

func TestExecDropPage_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPagesFunc:   func() ([]*pages.Page, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, execDropPage(ctx, &ast.DropPageStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "NonExistent"},
	}))
}

func TestExecDropSnippet_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, execDropSnippet(ctx, &ast.DropSnippetStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "NonExistent"},
	}))
}

func TestExecDropAssociation_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	dm := mkDomainModel(mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		GetDomainModelFunc: func(moduleID model.ID) (*domainmodel.DomainModel, error) {
			return dm, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, execDropAssociation(ctx, &ast.DropAssociationStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "NonExistent"},
	}))
}

// TestDropThenCreatePreservesMicroflowUnitID is a regression test for the
// MPR corruption bug documented in docs/MXCLI_MPR_CORRUPTION_PROMPT_0015.md.
//
// When a script runs `DROP MICROFLOW X; CREATE OR MODIFY MICROFLOW X ...` in
// the same session, the executor used to delete the Unit row and then insert
// a new one with a freshly generated UUID. Studio Pro treats the rewritten
// ContainerID/UnitID pair as an unrelated document and refuses to open the
// resulting .mpr ("file does not look like a Mendix Studio Pro project").
//
// The fix records the UnitID of dropped microflows on the executor cache and
// reuses it when a subsequent CREATE OR REPLACE/MODIFY targets the same
// qualified name, so the delete+insert behaves like an in-place update.
func TestDropThenCreatePreservesMicroflowUnitID(t *testing.T) {
	mod := mkModule("MyModule")
	mf := mkMicroflow(mod.ID, "DoSomething")
	originalID := mf.ID
	mf.AllowedModuleRoles = []model.ID{"MyModule.Admin", "MyModule.User"}

	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)

	listedMicroflows := []*microflows.Microflow{mf}
	var createdID model.ID
	var createdRoles []model.ID

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return listedMicroflows, nil
		},
		DeleteMicroflowFunc: func(id model.ID) error {
			// Simulate real deletion: hide the microflow from subsequent
			// ListMicroflows calls so CREATE OR MODIFY sees no existing unit
			// (matching the bug reproduction exactly).
			listedMicroflows = nil
			return nil
		},
		CreateMicroflowFunc: func(m *microflows.Microflow) error {
			createdID = m.ID
			createdRoles = cloneRoleIDs(m.AllowedModuleRoles)
			return nil
		},
		GetModuleSecurityFunc: func(moduleID model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{
				BaseElement: model.BaseElement{ID: nextID("ms")},
				ContainerID: moduleID,
			}, nil
		},
		AddModuleRoleFunc: func(moduleSecurityID model.ID, name, description string) error {
			return nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return nil, nil
		},
		ListConsumedRestServicesFunc: func() ([]*model.ConsumedRestService, error) {
			return nil, nil
		},
	}

	// Create ExecContext directly — trackCreatedMicroflow is now a method on ExecContext.
	var out bytesWriter
	ctx := &ExecContext{
		Context: t.Context(),
		Backend: mb,
		Output:  &out,
		Format:  FormatTable,
	}
	withHierarchy(h)(ctx)

	if err := execDropMicroflow(ctx, &ast.DropMicroflowStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "DoSomething"},
	}); err != nil {
		t.Fatalf("DROP MICROFLOW failed: %v", err)
	}

	// The UnitID and ContainerID must have been stashed on the cache before deletion.
	if ctx.Cache == nil || ctx.Cache.droppedMicroflows == nil ||
		ctx.Cache.droppedMicroflows["MyModule.DoSomething"] == nil ||
		ctx.Cache.droppedMicroflows["MyModule.DoSomething"].ID != originalID {
		t.Fatalf("expected droppedMicroflows[MyModule.DoSomething].ID = %q, got cache=%+v",
			originalID, ctx.Cache)
	}

	// CREATE OR MODIFY with the same qualified name must reuse the dropped ID.
	createStmt := &ast.CreateMicroflowStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "DoSomething"},
		CreateOrModify: true,
		Body:           nil, // empty body is fine for this test
	}
	if err := execCreateMicroflow(ctx, createStmt); err != nil {
		t.Fatalf("CREATE OR MODIFY MICROFLOW failed: %v", err)
	}

	if createdID != originalID {
		t.Fatalf("CREATE OR MODIFY must reuse dropped UnitID: got %q, want %q",
			createdID, originalID)
	}
	if len(createdRoles) != 2 || createdRoles[0] != "MyModule.Admin" || createdRoles[1] != "MyModule.User" {
		t.Fatalf("CREATE OR MODIFY must preserve dropped allowed roles: got %v", createdRoles)
	}

	// The cache entry must be consumed so repeated CREATEs don't collide.
	if ctx.Cache != nil && ctx.Cache.droppedMicroflows != nil {
		if _, stillThere := ctx.Cache.droppedMicroflows["MyModule.DoSomething"]; stillThere {
			t.Errorf("droppedMicroflows entry should be cleared after reuse")
		}
	}
}

func TestCreateOrModifyMicroflowPreservesAllowedRoles(t *testing.T) {
	mod := mkModule("MyModule")
	mf := mkMicroflow(mod.ID, "DoSomething")
	mf.AllowedModuleRoles = []model.ID{"MyModule.Admin"}

	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)

	var updatedRoles []model.ID
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{mf}, nil
		},
		UpdateMicroflowFunc: func(updated *microflows.Microflow) error {
			updatedRoles = cloneRoleIDs(updated.AllowedModuleRoles)
			return nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return nil, nil
		},
		ListConsumedRestServicesFunc: func() ([]*model.ConsumedRestService, error) {
			return nil, nil
		},
	}

	exec := New(&bytesWriter{})
	ctx := &ExecContext{
		Context: t.Context(),
		Backend: mb,
		Output:  exec.output,
		Format:  FormatTable,
	}
	withHierarchy(h)(ctx)

	if err := execCreateMicroflow(ctx, &ast.CreateMicroflowStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "DoSomething"},
		CreateOrModify: true,
	}); err != nil {
		t.Fatalf("CREATE OR MODIFY MICROFLOW failed: %v", err)
	}

	if len(updatedRoles) != 1 || updatedRoles[0] != "MyModule.Admin" {
		t.Fatalf("expected existing allowed roles to be preserved, got %v", updatedRoles)
	}
}

// bytesWriter is a trivial io.Writer used to satisfy New() in the regression
// test above. We don't care about captured output for this test.
type bytesWriter struct{}

func (*bytesWriter) Write(p []byte) (int, error) { return len(p), nil }
