// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// ---------------------------------------------------------------------------
// execMove — not connected
// ---------------------------------------------------------------------------

func TestMove_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return false }}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypePage,
		Name:         ast.QualifiedName{Module: "MyModule", Name: "MyPage"},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not connected")
}

// ---------------------------------------------------------------------------
// execMove — page happy path
// ---------------------------------------------------------------------------

func TestMove_Page_ToFolder(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "MyPage")
	folderID := nextID("folder")
	folders := []*types.FolderInfo{
		{ID: folderID, ContainerID: mod.ID, Name: "Admin"},
	}
	var movedPage *pages.Page
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return folders, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		MovePageFunc:    func(p *pages.Page) error { movedPage = p; return nil },
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypePage,
		Name:         ast.QualifiedName{Module: "MyModule", Name: "MyPage"},
		Folder:       "Admin",
	}))
	if movedPage == nil {
		t.Fatal("Expected MovePage to be called")
	}
	if movedPage.ContainerID != folderID {
		t.Errorf("Expected container %s, got %s", folderID, movedPage.ContainerID)
	}
	assertContainsStr(t, buf.String(), "Moved page")
}

// ---------------------------------------------------------------------------
// execMove — page not found
// ---------------------------------------------------------------------------

func TestMove_Page_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return nil, nil },
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypePage,
		Name:         ast.QualifiedName{Module: "MyModule", Name: "NonExistent"},
		Folder:       "SomeFolder",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}

// ---------------------------------------------------------------------------
// execMove — cross-module move updates references
// ---------------------------------------------------------------------------

func TestMove_Page_CrossModule(t *testing.T) {
	srcMod := mkModule("SrcModule")
	dstMod := mkModule("DstModule")
	pg := mkPage(srcMod.ID, "MyPage")
	var movedPage *pages.Page
	refUpdated := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{srcMod, dstMod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		MovePageFunc:    func(p *pages.Page) error { movedPage = p; return nil },
		UpdateQualifiedNameInAllUnitsFunc: func(old, new string) (int, error) {
			refUpdated = true
			return 3, nil
		},
	}
	h := mkHierarchy(srcMod, dstMod)
	withContainer(h, pg.ContainerID, srcMod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypePage,
		Name:         ast.QualifiedName{Module: "SrcModule", Name: "MyPage"},
		TargetModule: "DstModule",
	}))
	if movedPage == nil {
		t.Fatal("Expected MovePage to be called")
	}
	if !refUpdated {
		t.Error("Expected reference update for cross-module move")
	}
	assertContainsStr(t, buf.String(), "Moved page")
	assertContainsStr(t, buf.String(), "Updated references")
}

// ---------------------------------------------------------------------------
// execMove — unsupported type
// ---------------------------------------------------------------------------

func TestMove_UnsupportedType(t *testing.T) {
	mod := mkModule("MyModule")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execMove(ctx, &ast.MoveStmt{
		DocumentType: "UNKNOWN",
		Name:         ast.QualifiedName{Module: "MyModule", Name: "Thing"},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "unsupported")
}

// ---------------------------------------------------------------------------
// execMove — backend error on move
// ---------------------------------------------------------------------------

func TestMove_Page_BackendError(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "MyPage")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		MovePageFunc:    func(p *pages.Page) error { return fmt.Errorf("disk full") },
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypePage,
		Name:         ast.QualifiedName{Module: "MyModule", Name: "MyPage"},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "move page")
}
