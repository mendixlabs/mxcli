// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

func TestShowModules_Mock(t *testing.T) {
	mod1 := mkModule("MyModule")
	mod2 := mkModule("System")

	unitID := nextID("unit")
	units := []*types.UnitInfo{{ID: unitID, ContainerID: mod1.ID}}

	h := mkHierarchy(mod1, mod2)
	withContainer(h, unitID, mod1.ID)

	ent := mkEntity(mod1.ID, "Customer")
	dm := mkDomainModel(mod1.ID, ent)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod1, mod2}, nil },
		ListUnitsFunc:        func() ([]*types.UnitInfo, error) { return units, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listModules(ctx))

	out := buf.String()
	assertContainsStr(t, out, "MyModule")
	assertContainsStr(t, out, "System")
	assertContainsStr(t, out, "(2 modules)")
}

// Not-connected covered in cmd_notconnected_mock_test.go

func TestShowModules_BackendError(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return nil, fmt.Errorf("connection lost") },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy()))
	assertError(t, listModules(ctx))
}

func TestShowModules_JSON(t *testing.T) {
	mod := mkModule("App")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListUnitsFunc:        func() ([]*types.UnitInfo, error) { return nil, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h), withFormat(FormatJSON))
	assertNoError(t, listModules(ctx))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "App")
}
