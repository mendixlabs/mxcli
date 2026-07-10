// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestShowConstants_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	c1 := mkConstant(mod.ID, "AppURL", "String", "https://example.com")
	c2 := mkConstant(mod.ID, "MaxRetries", "Integer", "3")
	c2.ExposedToClient = true

	h := mkHierarchy(mod)
	withContainer(h, c1.ContainerID, mod.ID)
	withContainer(h, c2.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListConstantsFunc: func() ([]*model.Constant, error) { return []*model.Constant{c1, c2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listConstants(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "MyModule.AppURL")
	assertContainsStr(t, out, "MyModule.MaxRetries")
	assertContainsStr(t, out, "https://example.com")
	assertContainsStr(t, out, "Yes")
	assertContainsStr(t, out, "(2 constants)")
}

func TestShowConstants_Mock_FilterByModule(t *testing.T) {
	mod1 := mkModule("Alpha")
	mod2 := mkModule("Beta")
	c1 := mkConstant(mod1.ID, "Key1", "String", "val1")
	c2 := mkConstant(mod2.ID, "Key2", "Integer", "42")

	h := mkHierarchy(mod1, mod2)
	withContainer(h, c1.ContainerID, mod1.ID)
	withContainer(h, c2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListConstantsFunc: func() ([]*model.Constant, error) { return []*model.Constant{c1, c2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listConstants(ctx, "Beta"))

	out := buf.String()
	assertNotContainsStr(t, out, "Alpha.Key1")
	assertContainsStr(t, out, "Beta.Key2")
	assertContainsStr(t, out, "(1 constants)")
}

func TestShowConstants_Mock_Empty(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListConstantsFunc: func() ([]*model.Constant, error) { return nil, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy()))
	assertNoError(t, listConstants(ctx, ""))
	assertContainsStr(t, buf.String(), "No constants found")
}

func TestDescribeConstant_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	c := mkConstant(mod.ID, "AppURL", "String", "https://example.com")

	h := mkHierarchy(mod)
	withContainer(h, c.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListConstantsFunc: func() ([]*model.Constant, error) { return []*model.Constant{c}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeConstant(ctx, ast.QualifiedName{Module: "MyModule", Name: "AppURL"}))

	out := buf.String()
	assertContainsStr(t, out, "create or modify constant MyModule.AppURL")
	assertContainsStr(t, out, "String")
}

func TestDescribeConstant_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListConstantsFunc: func() ([]*model.Constant, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := describeConstant(ctx, ast.QualifiedName{Module: "MyModule", Name: "Missing"})
	assertError(t, err)
}

// Backend error: cmd_error_mock_test.go (TestShowConstants_Mock_BackendError)
// JSON: cmd_json_mock_test.go (TestShowConstants_Mock_JSON)
