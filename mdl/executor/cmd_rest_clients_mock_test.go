// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestShowRestClients_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	svc := &model.ConsumedRestService{
		BaseElement: model.BaseElement{ID: nextID("crs")},
		ContainerID: mod.ID,
		Name:        "WeatherAPI",
		BaseUrl:     "https://api.weather.com",
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListConsumedRestServicesFunc: func() ([]*model.ConsumedRestService, error) {
			return []*model.ConsumedRestService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listRestClients(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "QualifiedName")
	assertContainsStr(t, out, "MyModule.WeatherAPI")
}

func TestDescribeRestClient_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	svc := &model.ConsumedRestService{
		BaseElement: model.BaseElement{ID: nextID("crs")},
		ContainerID: mod.ID,
		Name:        "WeatherAPI",
		BaseUrl:     "https://api.weather.com",
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListConsumedRestServicesFunc: func() ([]*model.ConsumedRestService, error) {
			return []*model.ConsumedRestService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeRestClient(ctx, ast.QualifiedName{Module: "MyModule", Name: "WeatherAPI"}))

	out := buf.String()
	assertContainsStr(t, out, "create rest client")
	assertContainsStr(t, out, "MyModule.WeatherAPI")
}

func TestDescribeRestClient_NotFound(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListConsumedRestServicesFunc: func() ([]*model.ConsumedRestService, error) {
			return nil, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, describeRestClient(ctx, ast.QualifiedName{Module: "X", Name: "NoSuch"}))
}

func TestShowRestClients_FilterByModule(t *testing.T) {
	mod := mkModule("Integrations")
	svc := &model.ConsumedRestService{
		BaseElement: model.BaseElement{ID: nextID("crs")},
		ContainerID: mod.ID,
		Name:        "PaymentAPI",
		BaseUrl:     "https://api.payment.com",
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListConsumedRestServicesFunc: func() ([]*model.ConsumedRestService, error) {
			return []*model.ConsumedRestService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listRestClients(ctx, "Integrations"))
	assertContainsStr(t, buf.String(), "Integrations.PaymentAPI")
}

func restClientProjectVersion() *types.ProjectVersion {
	return &types.ProjectVersion{ProductVersion: "10.6.0", MajorVersion: 10, MinorVersion: 6}
}

func TestCreateRestClient_OrModify_SaysModified(t *testing.T) {
	mod := mkModule("MyModule")
	existingID := nextID("crs")
	existing := &model.ConsumedRestService{
		BaseElement: model.BaseElement{ID: existingID},
		ContainerID: mod.ID,
		Name:        "WeatherAPI",
	}
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ProjectVersionFunc: func() *types.ProjectVersion { return restClientProjectVersion() },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		ListConsumedRestServicesFunc: func() ([]*model.ConsumedRestService, error) {
			return []*model.ConsumedRestService{existing}, nil
		},
		DeleteConsumedRestServiceFunc: func(id model.ID) error { return nil },
		CreateConsumedRestServiceFunc: func(svc *model.ConsumedRestService) error { return nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateRestClientStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "WeatherAPI"},
		BaseUrl:        "https://api.weather.com",
		CreateOrModify: true,
	}
	assertNoError(t, createRestClient(ctx, stmt))
	assertContainsStr(t, buf.String(), "Modified rest client")
}

func TestCreateRestClient_New_SaysCreated(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ProjectVersionFunc: func() *types.ProjectVersion { return restClientProjectVersion() },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		ListConsumedRestServicesFunc: func() ([]*model.ConsumedRestService, error) {
			return nil, nil
		},
		CreateConsumedRestServiceFunc: func(svc *model.ConsumedRestService) error { return nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateRestClientStmt{
		Name:    ast.QualifiedName{Module: "MyModule", Name: "WeatherAPI"},
		BaseUrl: "https://api.weather.com",
	}
	assertNoError(t, createRestClient(ctx, stmt))
	assertContainsStr(t, buf.String(), "Created rest client")
}
