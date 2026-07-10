// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestShowEnumerations_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	enum := mkEnumeration(mod.ID, "Color", "Red", "Green", "Blue")

	h := mkHierarchy(mod)
	withContainer(h, enum.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listEnumerations(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "MyModule.Color")
	assertContainsStr(t, out, "| 3")
	assertContainsStr(t, out, "(1 enumerations)")
}

func TestShowEnumerations_Mock_FilterByModule(t *testing.T) {
	mod1 := mkModule("Alpha")
	mod2 := mkModule("Beta")
	e1 := mkEnumeration(mod1.ID, "Color", "Red")
	e2 := mkEnumeration(mod2.ID, "Size", "S", "M")

	h := mkHierarchy(mod1, mod2)
	withContainer(h, e1.ContainerID, mod1.ID)
	withContainer(h, e2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{e1, e2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listEnumerations(ctx, "Beta"))

	out := buf.String()
	assertNotContainsStr(t, out, "Alpha.Color")
	assertContainsStr(t, out, "Beta.Size")
	assertContainsStr(t, out, "(1 enumerations)")
}

func TestDescribeEnumeration_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	enum := &model.Enumeration{
		BaseElement: model.BaseElement{ID: nextID("enum")},
		ContainerID: mod.ID,
		Name:        "Status",
		Values: []model.EnumerationValue{
			{BaseElement: model.BaseElement{ID: nextID("ev")}, Name: "Active", Caption: &model.Text{Translations: map[string]string{"en_US": "Active"}}},
			{BaseElement: model.BaseElement{ID: nextID("ev")}, Name: "Inactive", Caption: &model.Text{Translations: map[string]string{"en_US": "Inactive"}}},
		},
	}

	h := mkHierarchy(mod)
	withContainer(h, enum.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeEnumeration(ctx, ast.QualifiedName{Module: "MyModule", Name: "Status"}))

	out := buf.String()
	assertContainsStr(t, out, "create or modify enumeration MyModule.Status")
	assertContainsStr(t, out, "Active")
	assertContainsStr(t, out, "Inactive")
}

func TestDescribeEnumeration_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := describeEnumeration(ctx, ast.QualifiedName{Module: "MyModule", Name: "Missing"})
	assertError(t, err)
}

// Issue #391 — DROP ENUMERATION with an unqualified name must error when
// the name matches enumerations in multiple modules, not silently drop one.
func TestDropEnumeration_AmbiguousUnqualified_Issue391(t *testing.T) {
	mod1 := mkModule("Mod1")
	mod2 := mkModule("Mod2")
	e1 := mkEnumeration(mod1.ID, "Status", "Active", "Inactive")
	e2 := mkEnumeration(mod2.ID, "Status", "Open", "Closed")

	h := mkHierarchy(mod1, mod2)
	withContainer(h, e1.ContainerID, mod1.ID)
	withContainer(h, e2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod1, mod2}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{e1, e2}, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execDropEnumeration(ctx, &ast.DropEnumerationStmt{
		Name: ast.QualifiedName{Name: "Status"}, // unqualified
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "ambiguous")
}

func TestDropEnumeration_Qualified_Success(t *testing.T) {
	mod1 := mkModule("Mod1")
	mod2 := mkModule("Mod2")
	e1 := mkEnumeration(mod1.ID, "Status", "Active")
	e2 := mkEnumeration(mod2.ID, "Status", "Open")
	deleted := ""

	h := mkHierarchy(mod1, mod2)
	withContainer(h, e1.ContainerID, mod1.ID)
	withContainer(h, e2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod1, mod2}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{e1, e2}, nil },
		DeleteEnumerationFunc: func(id model.ID) error {
			deleted = string(id)
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execDropEnumeration(ctx, &ast.DropEnumerationStmt{
		Name: ast.QualifiedName{Module: "Mod1", Name: "Status"},
	})
	assertNoError(t, err)
	if deleted != string(e1.ID) {
		t.Errorf("expected Mod1.Status (id=%s) to be deleted, got %s", e1.ID, deleted)
	}
}

// Issue #390 — CREATE ENUMERATION must reject duplicate value names.
func TestCreateEnumeration_DuplicateValueName_Issue390(t *testing.T) {
	mod := mkModule("M")
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execCreateEnumeration(ctx, &ast.CreateEnumerationStmt{
		Name: ast.QualifiedName{Module: "M", Name: "E"},
		Values: []ast.EnumValue{
			{Name: "A", Caption: "First"},
			{Name: "A", Caption: "Second"},
		},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "duplicate")
}

// Backend error: cmd_error_mock_test.go (TestShowEnumerations_Mock_BackendError)
// JSON: cmd_json_mock_test.go (TestShowEnumerations_Mock_JSON)
