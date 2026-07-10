// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

func TestDescribeMermaid_DomainModel_Mock(t *testing.T) {
	mod := mkModule("MyModule")

	// Build domain model first to get its ID, then create entities with dm as container.
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
	}
	ent1 := mkEntity(dm.ID, "Customer")
	ent2 := mkEntity(dm.ID, "Order")
	dm.Entities = []*domainmodel.Entity{ent1, ent2}
	dm.Associations = []*domainmodel.Association{
		mkAssociation(mod.ID, "Order_Customer", ent2.ID, ent1.ID),
	}

	// Hierarchy: entity.ContainerID (dm.ID) -> mod.ID (module)
	// Entities are contained by the domain model; the domain model is contained by the module.
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(moduleID model.ID) (*domainmodel.DomainModel, error) {
			return dm, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{dm}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeMermaid(ctx, "DOMAINMODEL", "MyModule"))

	out := buf.String()
	assertContainsStr(t, out, "erDiagram")
	assertContainsStr(t, out, "Customer")
	assertContainsStr(t, out, "Order")
	assertContainsStr(t, out, "Order_Customer")
}

func TestDescribeMermaid_Microflow_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	mf := &microflows.Microflow{
		BaseElement: model.BaseElement{ID: nextID("mf")},
		ContainerID: mod.ID,
		Name:        "ACT_Process",
	}

	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeMermaid(ctx, "microflow", "MyModule.ACT_Process"))

	out := buf.String()
	assertContainsStr(t, out, "flowchart")
}

func TestDescribeMermaid_Nanoflow_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	nf := &microflows.Nanoflow{
		BaseElement: model.BaseElement{ID: nextID("nf")},
		ContainerID: mod.ID,
		Name:        "NF_Process",
	}

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeMermaid(ctx, "nanoflow", "MyModule.NF_Process"))

	out := buf.String()
	assertContainsStr(t, out, "flowchart")
}

func TestDescribeMermaid_Nanoflow_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return nil, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, describeMermaid(ctx, "nanoflow", "MyModule.NoSuch"))
}

func TestDescribeMermaid_Microflow_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return nil, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, describeMermaid(ctx, "microflow", "MyModule.NoSuch"))
}

func TestDescribeMermaid_Page_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, describeMermaid(ctx, "page", "MyModule.NoSuch"))
}

func TestDescribeMermaid_UnsupportedType(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
	}

	ctx, _ := newMockCtx(t, withBackend(mb))
	err := describeMermaid(ctx, "workflow", "MyModule.Something")
	assertError(t, err)
	assertContainsStr(t, fmt.Sprint(err), "not supported")
}
