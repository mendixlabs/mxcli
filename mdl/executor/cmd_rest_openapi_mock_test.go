// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// minimalOpenAPISpec is a self-contained OpenAPI 3.0 fixture used in all
// OpenAPI import tests. No network access required.
const minimalOpenAPISpec = `{
  "openapi": "3.0.0",
  "info": { "title": "Pet Store", "version": "1.0.0" },
  "servers": [{ "url": "https://api.example.com/v1" }],
  "paths": {
    "/pets": {
      "get": {
        "operationId": "listPets",
        "summary": "List all pets",
        "tags": ["pets"],
        "parameters": [
          { "name": "limit", "in": "query", "schema": { "type": "integer" } }
        ],
        "responses": { "200": { "description": "OK" } }
      }
    }
  },
  "components": {
    "securitySchemes": {
      "basicAuth": { "type": "http", "scheme": "basic" }
    }
  },
  "security": [{ "basicAuth": [] }]
}`

// newOpenAPIMockBackend returns a MockBackend pre-wired for OpenAPI import
// tests: write-connected, a project version that satisfies the REST client
// feature gate, and no pre-existing REST clients.
func newOpenAPIMockBackend() *mock.MockBackend {
	return &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{
				ProductVersion: "10.6.0",
				MajorVersion:   10,
				MinorVersion:   6,
				PatchVersion:   0,
			}
		},
		ListModulesFunc: func() ([]*model.Module, error) {
			return nil, nil
		},
		ListConsumedRestServicesFunc: func() ([]*model.ConsumedRestService, error) {
			return nil, nil
		},
	}
}

func TestCreateRestClientFromSpec_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(minimalOpenAPISpec))
	}))
	defer srv.Close()

	mod := mkModule("PetModule")
	h := mkHierarchy(mod)

	var created *model.ConsumedRestService
	mb := newOpenAPIMockBackend()
	mb.ListModulesFunc = func() ([]*model.Module, error) {
		return []*model.Module{mod}, nil
	}
	mb.CreateConsumedRestServiceFunc = func(svc *model.ConsumedRestService) error {
		created = svc
		return nil
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateRestClientStmt{
		Name:        ast.QualifiedName{Module: "PetModule", Name: "PetStoreAPI"},
		OpenApiPath: srv.URL,
	}

	assertNoError(t, createRestClient(ctx, stmt))

	if created == nil {
		t.Fatal("CreateConsumedRestService was not called")
	}
	if created.Name != "PetStoreAPI" {
		t.Errorf("expected service name PetStoreAPI, got %s", created.Name)
	}
	if created.BaseUrl != "https://api.example.com/v1" {
		t.Errorf("expected base URL from spec, got %s", created.BaseUrl)
	}
	if len(created.Operations) == 0 {
		t.Error("expected at least one operation from spec")
	}
	if created.OpenApiContent == "" {
		t.Error("expected OpenApiContent to be populated")
	}
	assertContainsStr(t, buf.String(), "PetModule.PetStoreAPI")
}

func TestCreateRestClientFromSpec_BaseUrlOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(minimalOpenAPISpec))
	}))
	defer srv.Close()

	mod := mkModule("PetModule")
	h := mkHierarchy(mod)

	var created *model.ConsumedRestService
	mb := newOpenAPIMockBackend()
	mb.ListModulesFunc = func() ([]*model.Module, error) {
		return []*model.Module{mod}, nil
	}
	mb.CreateConsumedRestServiceFunc = func(svc *model.ConsumedRestService) error {
		created = svc
		return nil
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateRestClientStmt{
		Name:        ast.QualifiedName{Module: "PetModule", Name: "PetStoreStaging"},
		OpenApiPath: srv.URL,
		BaseUrl:     "https://staging.example.com/v1",
	}

	assertNoError(t, createRestClient(ctx, stmt))

	if created == nil {
		t.Fatal("CreateConsumedRestService was not called")
	}
	if created.BaseUrl != "https://staging.example.com/v1" {
		t.Errorf("expected overridden BaseUrl, got %s", created.BaseUrl)
	}
}

func TestCreateRestClientFromSpec_OrModifyPreservesID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(minimalOpenAPISpec))
	}))
	defer srv.Close()

	mod := mkModule("PetModule")
	h := mkHierarchy(mod)
	existingID := nextID("rest")
	existing := &model.ConsumedRestService{
		BaseElement: model.BaseElement{ID: existingID},
		ContainerID: mod.ID,
		Name:        "PetStoreAPI",
	}
	withContainer(h, existing.ContainerID, mod.ID)

	var deletedID model.ID
	var created *model.ConsumedRestService
	mb := newOpenAPIMockBackend()
	mb.ListModulesFunc = func() ([]*model.Module, error) {
		return []*model.Module{mod}, nil
	}
	mb.ListConsumedRestServicesFunc = func() ([]*model.ConsumedRestService, error) {
		return []*model.ConsumedRestService{existing}, nil
	}
	mb.DeleteConsumedRestServiceFunc = func(id model.ID) error {
		deletedID = id
		return nil
	}
	mb.CreateConsumedRestServiceFunc = func(svc *model.ConsumedRestService) error {
		created = svc
		return nil
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateRestClientStmt{
		Name:           ast.QualifiedName{Module: "PetModule", Name: "PetStoreAPI"},
		OpenApiPath:    srv.URL,
		CreateOrModify: true,
	}
	assertNoError(t, createRestClient(ctx, stmt))

	if deletedID != existingID {
		t.Errorf("expected existing service to be deleted, got deletedID=%v", deletedID)
	}
	if created == nil {
		t.Fatal("CreateConsumedRestService was not called")
	}
	if created.ID != existingID {
		t.Errorf("expected recreated service to reuse existing ID %v, got %v", existingID, created.ID)
	}
	assertContainsStr(t, buf.String(), "Modified rest client")
}

func TestCreateRestClientFromSpec_DuplicateWithoutOrModify(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(minimalOpenAPISpec))
	}))
	defer srv.Close()

	mod := mkModule("PetModule")
	h := mkHierarchy(mod)
	existing := &model.ConsumedRestService{
		BaseElement: model.BaseElement{ID: nextID("rest")},
		ContainerID: mod.ID,
		Name:        "PetStoreAPI",
	}
	withContainer(h, existing.ContainerID, mod.ID)

	mb := newOpenAPIMockBackend()
	mb.ListModulesFunc = func() ([]*model.Module, error) {
		return []*model.Module{mod}, nil
	}
	mb.ListConsumedRestServicesFunc = func() ([]*model.ConsumedRestService, error) {
		return []*model.ConsumedRestService{existing}, nil
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateRestClientStmt{
		Name:        ast.QualifiedName{Module: "PetModule", Name: "PetStoreAPI"},
		OpenApiPath: srv.URL,
	}

	assertError(t, createRestClient(ctx, stmt))
}

func TestCreateRestClientFromSpec_ListError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(minimalOpenAPISpec))
	}))
	defer srv.Close()

	mod := mkModule("PetModule")
	h := mkHierarchy(mod)

	mb := newOpenAPIMockBackend()
	mb.ListModulesFunc = func() ([]*model.Module, error) {
		return []*model.Module{mod}, nil
	}
	mb.ListConsumedRestServicesFunc = func() ([]*model.ConsumedRestService, error) {
		return nil, mdlerrors.NewBackend("list rest clients", nil)
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateRestClientStmt{
		Name:        ast.QualifiedName{Module: "PetModule", Name: "PetStoreAPI"},
		OpenApiPath: srv.URL,
	}

	assertError(t, createRestClient(ctx, stmt))
}

func TestDescribeContractFromOpenAPI_Mock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(minimalOpenAPISpec))
	}))
	defer srv.Close()

	ctx, buf := newMockCtx(t)
	stmt := &ast.DescribeContractFromOpenAPIStmt{SpecPath: srv.URL}

	assertNoError(t, describeContractFromOpenAPI(ctx, stmt))

	out := buf.String()
	assertContainsStr(t, out, "create rest client")
	assertContainsStr(t, out, "https://api.example.com/v1")
	assertContainsStr(t, out, "listPets")
}
