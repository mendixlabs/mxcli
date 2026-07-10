// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestShowPublishedRestServices_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	svc := &model.PublishedRestService{
		BaseElement: model.BaseElement{ID: nextID("prs")},
		ContainerID: mod.ID,
		Name:        "OrderAPI",
		Path:        "/rest/orders/v1",
		Version:     "1.0",
		Resources: []*model.PublishedRestResource{
			{Name: "Orders"},
			{Name: "Items"},
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPublishedRestServicesFunc: func() ([]*model.PublishedRestService, error) {
			return []*model.PublishedRestService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listPublishedRestServices(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "QualifiedName")
	assertContainsStr(t, out, "MyModule.OrderAPI")
	assertContainsStr(t, out, "(1 published rest services)")
}

func TestDescribePublishedRestService_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	svc := &model.PublishedRestService{
		BaseElement: model.BaseElement{ID: nextID("prs")},
		ContainerID: mod.ID,
		Name:        "OrderAPI",
		Path:        "/rest/orders/v1",
		Version:     "1.0",
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPublishedRestServicesFunc: func() ([]*model.PublishedRestService, error) {
			return []*model.PublishedRestService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describePublishedRestService(ctx, ast.QualifiedName{Module: "MyModule", Name: "OrderAPI"}))

	out := buf.String()
	assertContainsStr(t, out, "create or modify published rest service")
	assertContainsStr(t, out, "MyModule.OrderAPI")
}

func TestDescribePublishedRestService_NotFound(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPublishedRestServicesFunc: func() ([]*model.PublishedRestService, error) {
			return nil, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, describePublishedRestService(ctx, ast.QualifiedName{Module: "X", Name: "NoSuch"}))
}

func TestShowPublishedRestServices_FilterByModule(t *testing.T) {
	mod := mkModule("Sales")
	svc := &model.PublishedRestService{
		BaseElement: model.BaseElement{ID: nextID("prs")},
		ContainerID: mod.ID,
		Name:        "SalesAPI",
		Path:        "/rest/sales/v1",
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPublishedRestServicesFunc: func() ([]*model.PublishedRestService, error) {
			return []*model.PublishedRestService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listPublishedRestServices(ctx, "Sales"))
	assertContainsStr(t, buf.String(), "Sales.SalesAPI")
}
