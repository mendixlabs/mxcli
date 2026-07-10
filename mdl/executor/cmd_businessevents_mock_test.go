// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestShowBusinessEventServices_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	svc := &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: nextID("bes")},
		ContainerID: mod.ID,
		Name:        "OrderEvents",
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return []*model.BusinessEventService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listBusinessEventServices(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "QualifiedName")
	assertContainsStr(t, out, "MyModule.OrderEvents")
}

func TestShowBusinessEventClients_Mock(t *testing.T) {
	ctx, buf := newMockCtx(t)
	assertNoError(t, listBusinessEventClients(ctx, ""))
	assertContainsStr(t, buf.String(), "not yet implemented")
}

func TestDescribeBusinessEventService_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	svc := &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: nextID("bes")},
		ContainerID: mod.ID,
		Name:        "OrderEvents",
		Definition: &model.BusinessEventDefinition{
			ServiceName:     "com.example.orders",
			EventNamePrefix: "order",
			Channels: []*model.BusinessEventChannel{
				{
					ChannelName: "ch1",
					Messages: []*model.BusinessEventMessage{
						{MessageName: "OrderCreated"},
					},
				},
			},
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return []*model.BusinessEventService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeBusinessEventService(ctx, ast.QualifiedName{Module: "MyModule", Name: "OrderEvents"}))

	out := buf.String()
	assertContainsStr(t, out, "create or modify business event service")
	assertContainsStr(t, out, "MyModule.OrderEvents")
}

func TestDescribeBusinessEventService_NotFound(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return nil, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, describeBusinessEventService(ctx, ast.QualifiedName{Module: "X", Name: "NoSuch"}))
}

func TestShowBusinessEventServices_FilterByModule(t *testing.T) {
	mod := mkModule("Orders")
	svc := &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: nextID("bes")},
		ContainerID: mod.ID,
		Name:        "OrderEvents",
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return []*model.BusinessEventService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listBusinessEventServices(ctx, "Orders"))
	assertContainsStr(t, buf.String(), "Orders.OrderEvents")
}
