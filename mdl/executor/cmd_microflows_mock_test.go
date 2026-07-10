// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestShowMicroflows_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	mf := mkMicroflow(mod.ID, "ACT_CreateOrder")

	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listMicroflows(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "MyModule.ACT_CreateOrder")
	assertContainsStr(t, out, "(1 microflows)")
}

func TestShowMicroflows_Mock_FilterByModule(t *testing.T) {
	mod1 := mkModule("Sales")
	mod2 := mkModule("HR")
	mf1 := mkMicroflow(mod1.ID, "ACT_Sell")
	mf2 := mkMicroflow(mod2.ID, "ACT_Hire")

	h := mkHierarchy(mod1, mod2)
	withContainer(h, mf1.ContainerID, mod1.ID)
	withContainer(h, mf2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf1, mf2}, nil },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod1, mod2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listMicroflows(ctx, "HR"))

	out := buf.String()
	assertNotContainsStr(t, out, "Sales.ACT_Sell")
	assertContainsStr(t, out, "HR.ACT_Hire")
}

func TestShowNanoflows_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	nf := mkNanoflow(mod.ID, "NF_Validate")

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listNanoflows(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "MyModule.NF_Validate")
	assertContainsStr(t, out, "(1 nanoflows)")
}

func TestDescribeMicroflow_Mock_Minimal(t *testing.T) {
	mod := mkModule("MyModule")
	mf := mkMicroflow(mod.ID, "ACT_DoSomething")

	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeMicroflow(ctx, ast.QualifiedName{Module: "MyModule", Name: "ACT_DoSomething"}))

	out := buf.String()
	assertContainsStr(t, out, "create or modify microflow MyModule.ACT_DoSomething")
}

func TestDescribeMicroflow_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return nil, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := describeMicroflow(ctx, ast.QualifiedName{Module: "MyModule", Name: "Missing"})
	assertError(t, err)
}

// Backend error: cmd_error_mock_test.go (TestShowMicroflows_Mock_BackendError, TestShowNanoflows_Mock_BackendError)
// JSON: cmd_json_mock_test.go (TestShowMicroflows_Mock_JSON, TestShowNanoflows_Mock_JSON)

// --- OBS-2: Module not found error for SHOW MICROFLOWS ---

func TestShowMicroflows_Mock_ModuleNotFound(t *testing.T) {
	mod := mkModule("Sales")

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb))
	err := listMicroflows(ctx, "NonExistent")
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}

// --- OBS-8: Empty microflow name validation ---

func TestCreateMicroflow_Mock_EmptyName(t *testing.T) {
	mod := mkModule("MyModule")

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb))
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: ""},
	}
	err := execCreateMicroflow(ctx, stmt)
	assertError(t, err)
	assertContainsStr(t, err.Error(), "must not be empty")
}
