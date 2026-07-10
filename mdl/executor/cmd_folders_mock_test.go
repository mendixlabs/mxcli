// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// ---------------------------------------------------------------------------
// execDropFolder
// ---------------------------------------------------------------------------

func TestDropFolder_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execDropFolder(ctx, &ast.DropFolderStmt{FolderPath: "Resources", Module: "MyModule"})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not connected")
}

func TestDropFolder_ModuleNotFound(t *testing.T) {
	mod := mkModule("OtherModule")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execDropFolder(ctx, &ast.DropFolderStmt{FolderPath: "Resources", Module: "MyModule"})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}

func TestDropFolder_FolderNotFound(t *testing.T) {
	mod := mkModule("MyModule")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execDropFolder(ctx, &ast.DropFolderStmt{FolderPath: "NonExistent", Module: "MyModule"})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}

func TestDropFolder_Success(t *testing.T) {
	mod := mkModule("MyModule")
	folderID := nextID("folder")
	folders := []*types.FolderInfo{
		{ID: folderID, ContainerID: mod.ID, Name: "Resources"},
	}
	deleteCalled := false
	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListModulesFunc:  func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:  func() ([]*types.FolderInfo, error) { return folders, nil },
		DeleteFolderFunc: func(id model.ID) error { deleteCalled = true; return nil },
	}
	h := mkHierarchy(mod)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execDropFolder(ctx, &ast.DropFolderStmt{FolderPath: "Resources", Module: "MyModule"}))
	if !deleteCalled {
		t.Error("expected DeleteFolder to be called")
	}
	assertContainsStr(t, buf.String(), "Dropped folder")
	assertContainsStr(t, buf.String(), "Resources")
}

func TestDropFolder_BackendError(t *testing.T) {
	mod := mkModule("MyModule")
	folderID := nextID("folder")
	folders := []*types.FolderInfo{
		{ID: folderID, ContainerID: mod.ID, Name: "Resources"},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListModulesFunc:  func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:  func() ([]*types.FolderInfo, error) { return folders, nil },
		DeleteFolderFunc: func(id model.ID) error { return fmt.Errorf("folder not empty") },
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execDropFolder(ctx, &ast.DropFolderStmt{FolderPath: "Resources", Module: "MyModule"})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "delete folder")
}

func TestDropFolder_NestedPath(t *testing.T) {
	mod := mkModule("MyModule")
	parentFolderID := nextID("folder")
	childFolderID := nextID("folder")
	folders := []*types.FolderInfo{
		{ID: parentFolderID, ContainerID: mod.ID, Name: "Resources"},
		{ID: childFolderID, ContainerID: parentFolderID, Name: "Images"},
	}
	var deletedID model.ID
	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListModulesFunc:  func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:  func() ([]*types.FolderInfo, error) { return folders, nil },
		DeleteFolderFunc: func(id model.ID) error { deletedID = id; return nil },
	}
	h := mkHierarchy(mod)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execDropFolder(ctx, &ast.DropFolderStmt{FolderPath: "Resources/Images", Module: "MyModule"}))
	if deletedID != childFolderID {
		t.Errorf("expected to delete child folder %s, got %s", childFolderID, deletedID)
	}
	assertContainsStr(t, buf.String(), "Dropped folder")
}

// ---------------------------------------------------------------------------
// execMoveFolder
// ---------------------------------------------------------------------------

func TestMoveFolder_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execMoveFolder(ctx, &ast.MoveFolderStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "Resources"},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not connected")
}

func TestMoveFolder_ToModule(t *testing.T) {
	srcMod := mkModule("MyModule")
	dstMod := mkModule("OtherModule")
	folderID := nextID("folder")
	folders := []*types.FolderInfo{
		{ID: folderID, ContainerID: srcMod.ID, Name: "Resources"},
	}
	var movedTo model.ID
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{srcMod, dstMod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return folders, nil },
		MoveFolderFunc:  func(id, target model.ID) error { movedTo = target; return nil },
	}
	h := mkHierarchy(srcMod, dstMod)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execMoveFolder(ctx, &ast.MoveFolderStmt{
		Name:         ast.QualifiedName{Module: "MyModule", Name: "Resources"},
		TargetModule: "OtherModule",
	}))
	if movedTo != dstMod.ID {
		t.Errorf("expected move to %s, got %s", dstMod.ID, movedTo)
	}
	assertContainsStr(t, buf.String(), "Moved folder")
	assertContainsStr(t, buf.String(), "OtherModule")
}

func TestMoveFolder_ToFolder(t *testing.T) {
	mod := mkModule("MyModule")
	srcFolderID := nextID("folder")
	dstFolderID := nextID("folder")
	folders := []*types.FolderInfo{
		{ID: srcFolderID, ContainerID: mod.ID, Name: "OldFolder"},
		{ID: dstFolderID, ContainerID: mod.ID, Name: "NewParent"},
	}
	var movedTo model.ID
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return folders, nil },
		MoveFolderFunc:  func(id, target model.ID) error { movedTo = target; return nil },
	}
	h := mkHierarchy(mod)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execMoveFolder(ctx, &ast.MoveFolderStmt{
		Name:         ast.QualifiedName{Module: "MyModule", Name: "OldFolder"},
		TargetFolder: "NewParent",
	}))
	if movedTo != dstFolderID {
		t.Errorf("expected move to %s, got %s", dstFolderID, movedTo)
	}
	assertContainsStr(t, buf.String(), "Moved folder")
}
