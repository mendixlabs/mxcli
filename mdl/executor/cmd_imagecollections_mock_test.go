// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestShowImageCollections_Mock(t *testing.T) {
	mod := mkModule("Icons")
	ic := &types.ImageCollection{
		BaseElement: model.BaseElement{ID: nextID("ic")},
		ContainerID: mod.ID,
		Name:        "AppIcons",
		ExportLevel: "Hidden",
	}

	h := mkHierarchy(mod)
	withContainer(h, ic.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListImageCollectionsFunc: func() ([]*types.ImageCollection, error) { return []*types.ImageCollection{ic}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listImageCollections(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "Image Collection")
	assertContainsStr(t, out, "Icons.AppIcons")
}

func TestShowImageCollections_FilterByModule(t *testing.T) {
	mod1 := mkModule("Icons")
	mod2 := mkModule("Other")
	ic1 := &types.ImageCollection{
		BaseElement: model.BaseElement{ID: nextID("ic")},
		ContainerID: mod1.ID,
		Name:        "AppIcons",
		ExportLevel: "Hidden",
	}
	ic2 := &types.ImageCollection{
		BaseElement: model.BaseElement{ID: nextID("ic")},
		ContainerID: mod2.ID,
		Name:        "OtherIcons",
		ExportLevel: "Hidden",
	}

	h := mkHierarchy(mod1, mod2)
	withContainer(h, ic1.ContainerID, mod1.ID)
	withContainer(h, ic2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListImageCollectionsFunc: func() ([]*types.ImageCollection, error) { return []*types.ImageCollection{ic1, ic2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listImageCollections(ctx, "Icons"))

	out := buf.String()
	assertContainsStr(t, out, "Icons.AppIcons")
	assertNotContainsStr(t, out, "Other.OtherIcons")
}

func TestDescribeImageCollection_NotFound(t *testing.T) {
	mod := mkModule("Icons")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListImageCollectionsFunc: func() ([]*types.ImageCollection, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, describeImageCollection(ctx, ast.QualifiedName{Module: "Icons", Name: "NoSuch"}))
}

func TestDescribeImageCollection_Mock(t *testing.T) {
	mod := mkModule("Icons")
	ic := &types.ImageCollection{
		BaseElement: model.BaseElement{ID: nextID("ic")},
		ContainerID: mod.ID,
		Name:        "AppIcons",
		ExportLevel: "Hidden",
	}

	h := mkHierarchy(mod)
	withContainer(h, ic.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListImageCollectionsFunc: func() ([]*types.ImageCollection, error) { return []*types.ImageCollection{ic}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeImageCollection(ctx, ast.QualifiedName{Module: "Icons", Name: "AppIcons"}))

	out := buf.String()
	assertContainsStr(t, out, "create or modify image collection")
}

func TestCreateImageCollection_AlreadyExists_Error(t *testing.T) {
	mod := mkModule("MyModule")
	existing := &types.ImageCollection{
		BaseElement: model.BaseElement{ID: nextID("ic")},
		ContainerID: mod.ID,
		Name:        "Icons",
	}
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListModulesFunc:          func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListImageCollectionsFunc: func() ([]*types.ImageCollection, error) { return []*types.ImageCollection{existing}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	err := execCreateImageCollection(ctx, &ast.CreateImageCollectionStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "Icons"},
	})
	assertError(t, err)
}

func TestCreateImageCollection_OrModify_PreservesIDAndUpdates(t *testing.T) {
	mod := mkModule("MyModule")
	existing := &types.ImageCollection{
		BaseElement: model.BaseElement{ID: nextID("ic")},
		ContainerID: mod.ID,
		Name:        "Icons",
	}
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)

	updated := false
	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListModulesFunc:          func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListImageCollectionsFunc: func() ([]*types.ImageCollection, error) { return []*types.ImageCollection{existing}, nil },
		UpdateImageCollectionFunc: func(ic *types.ImageCollection) error {
			if ic.ID != existing.ID {
				t.Errorf("expected existing ID %s, got %s", existing.ID, ic.ID)
			}
			updated = true
			return nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))

	err := execCreateImageCollection(ctx, &ast.CreateImageCollectionStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "Icons"},
		CreateOrModify: true,
	})
	assertNoError(t, err)
	if !updated {
		t.Error("expected existing collection to be updated")
	}
	assertContainsStr(t, buf.String(), "Modified image collection")
}
