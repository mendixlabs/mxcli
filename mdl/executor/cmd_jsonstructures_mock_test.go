// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestShowJsonStructures_Mock(t *testing.T) {
	mod := mkModule("OrderMgmt")
	js := &types.JsonStructure{
		BaseElement: model.BaseElement{ID: nextID("js")},
		ContainerID: mod.ID,
		Name:        "OrderSchema",
	}

	h := mkHierarchy(mod)
	withContainer(h, js.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListJsonStructuresFunc: func() ([]*types.JsonStructure, error) { return []*types.JsonStructure{js}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listJsonStructures(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "json Structure")
	assertContainsStr(t, out, "OrderMgmt.OrderSchema")
}

func TestShowJsonStructures_FilterByModule(t *testing.T) {
	mod1 := mkModule("OrderMgmt")
	mod2 := mkModule("Other")
	js1 := &types.JsonStructure{
		BaseElement: model.BaseElement{ID: nextID("js")},
		ContainerID: mod1.ID,
		Name:        "OrderSchema",
	}
	js2 := &types.JsonStructure{
		BaseElement: model.BaseElement{ID: nextID("js")},
		ContainerID: mod2.ID,
		Name:        "OtherSchema",
	}

	h := mkHierarchy(mod1, mod2)
	withContainer(h, js1.ContainerID, mod1.ID)
	withContainer(h, js2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListJsonStructuresFunc: func() ([]*types.JsonStructure, error) { return []*types.JsonStructure{js1, js2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listJsonStructures(ctx, "OrderMgmt"))

	out := buf.String()
	assertContainsStr(t, out, "OrderMgmt.OrderSchema")
	assertNotContainsStr(t, out, "Other.OtherSchema")
}

func TestDescribeJsonStructure_Mock(t *testing.T) {
	mod := mkModule("OrderMgmt")
	js := &types.JsonStructure{
		BaseElement: model.BaseElement{ID: nextID("js")},
		ContainerID: mod.ID,
		Name:        "OrderSchema",
	}

	h := mkHierarchy(mod)
	withContainer(h, js.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListJsonStructuresFunc: func() ([]*types.JsonStructure, error) { return []*types.JsonStructure{js}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeJsonStructure(ctx, ast.QualifiedName{Module: "OrderMgmt", Name: "OrderSchema"}))
	assertContainsStr(t, buf.String(), "create or modify json structure")
}

func TestDescribeJsonStructure_NotFound(t *testing.T) {
	mod := mkModule("OrderMgmt")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListJsonStructuresFunc: func() ([]*types.JsonStructure, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, describeJsonStructure(ctx, ast.QualifiedName{Module: "OrderMgmt", Name: "NoSuch"}))
}
