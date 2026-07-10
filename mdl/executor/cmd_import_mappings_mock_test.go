// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestShowImportMappings_Mock(t *testing.T) {
	mod := mkModule("Integration")
	im := &model.ImportMapping{
		BaseElement: model.BaseElement{ID: nextID("im")},
		ContainerID: mod.ID,
		Name:        "ImportOrders",
	}

	h := mkHierarchy(mod)
	withContainer(h, im.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListImportMappingsFunc: func() ([]*model.ImportMapping, error) { return []*model.ImportMapping{im}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listImportMappings(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "Import Mapping")
	assertContainsStr(t, out, "Integration.ImportOrders")
}

func TestShowImportMappings_FilterByModule(t *testing.T) {
	mod1 := mkModule("Integration")
	mod2 := mkModule("Other")
	im1 := &model.ImportMapping{
		BaseElement: model.BaseElement{ID: nextID("im")},
		ContainerID: mod1.ID,
		Name:        "ImportOrders",
	}
	im2 := &model.ImportMapping{
		BaseElement: model.BaseElement{ID: nextID("im")},
		ContainerID: mod2.ID,
		Name:        "ImportOther",
	}

	h := mkHierarchy(mod1, mod2)
	withContainer(h, im1.ContainerID, mod1.ID)
	withContainer(h, im2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListImportMappingsFunc: func() ([]*model.ImportMapping, error) { return []*model.ImportMapping{im1, im2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listImportMappings(ctx, "Integration"))

	out := buf.String()
	assertContainsStr(t, out, "Integration.ImportOrders")
	assertNotContainsStr(t, out, "Other.ImportOther")
}

func TestDescribeImportMapping_Mock(t *testing.T) {
	mod := mkModule("Integration")
	im := &model.ImportMapping{
		BaseElement: model.BaseElement{ID: nextID("im")},
		ContainerID: mod.ID,
		Name:        "ImportOrders",
	}

	h := mkHierarchy(mod)
	withContainer(h, im.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetImportMappingByQualifiedNameFunc: func(moduleName, name string) (*model.ImportMapping, error) {
			return im, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeImportMapping(ctx, ast.QualifiedName{Module: "Integration", Name: "ImportOrders"}))
	assertContainsStr(t, buf.String(), "create import mapping")
}

func TestDescribeImportMapping_NotFound(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetImportMappingByQualifiedNameFunc: func(moduleName, name string) (*model.ImportMapping, error) {
			return nil, fmt.Errorf("import mapping not found: %s.%s", moduleName, name)
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, describeImportMapping(ctx, ast.QualifiedName{Module: "Integration", Name: "NoSuch"}))
}

func TestCreateImportMapping_OrModify_PreservesID(t *testing.T) {
	mod := mkModule("Integration")
	existingID := nextID("im")
	existing := &model.ImportMapping{
		BaseElement: model.BaseElement{ID: existingID},
		ContainerID: mod.ID,
		Name:        "ImportOrders",
	}
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)

	var updatedID model.ID
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetImportMappingByQualifiedNameFunc: func(moduleName, name string) (*model.ImportMapping, error) {
			if moduleName == "Integration" && name == "ImportOrders" {
				return existing, nil
			}
			return nil, nil
		},
		UpdateImportMappingFunc: func(im *model.ImportMapping) error {
			updatedID = im.ID
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execCreateImportMapping(ctx, &ast.CreateImportMappingStmt{
		Name:           ast.QualifiedName{Module: "Integration", Name: "ImportOrders"},
		SchemaKind:     "JSON_STRUCTURE",
		SchemaRef:      ast.QualifiedName{Module: "Integration", Name: "PetSchema"},
		CreateOrModify: true,
	})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Modified import mapping")
	if updatedID != existingID {
		t.Errorf("UpdateImportMapping called with ID %q, want %q", updatedID, existingID)
	}
}

func TestCreateImportMapping_AlreadyExists_NoOrModify(t *testing.T) {
	mod := mkModule("Integration")
	existing := &model.ImportMapping{
		BaseElement: model.BaseElement{ID: nextID("im")},
		ContainerID: mod.ID,
		Name:        "ImportOrders",
	}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetImportMappingByQualifiedNameFunc: func(moduleName, name string) (*model.ImportMapping, error) {
			return existing, nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execCreateImportMapping(ctx, &ast.CreateImportMappingStmt{
		Name: ast.QualifiedName{Module: "Integration", Name: "ImportOrders"},
	})
	assertError(t, err)
}
