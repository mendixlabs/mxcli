// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// publishedRestService returns a service with one operation, bound to microflow Api.Op.
func publishedRestService(path string) *model.PublishedRestService {
	return &model.PublishedRestService{
		Name: "Orders",
		Resources: []*model.PublishedRestResource{{
			Name:       "orders",
			Operations: []*model.PublishedRestOperation{{HTTPMethod: "GET", Path: path, Microflow: "Api.Op"}},
		}},
	}
}

// TestDeriveOperationParameters_AsStudioPro: a microflow parameter named in the path is a
// path parameter, an object or a list is the body, System.HttpRequest / HttpResponse are
// not operation parameters, and anything else is a query parameter. Before, only the path
// placeholders were written, and mx check reported CE0350 for every other parameter.
func TestDeriveOperationParameters_AsStudioPro(t *testing.T) {
	mod := mkModule("Api")
	mf := &microflows.Microflow{
		ContainerID: mod.ID,
		Name:        "Op",
		Parameters: []*microflows.MicroflowParameter{
			{Name: "id", Type: &microflows.IntegerType{}},
			{Name: "orderNumber", Type: &microflows.StringType{}},
			{Name: "status", Type: &microflows.EnumerationType{EnumerationQualifiedName: "Api.Status"}},
			{Name: "item", Type: &microflows.ObjectType{EntityQualifiedName: "Api.Item"}},
			{Name: "lines", Type: &microflows.ListType{EntityQualifiedName: "Api.Line"}},
			{Name: "httpRequest", Type: &microflows.ObjectType{EntityQualifiedName: "System.HttpRequest"}},
			{Name: "httpResponse", Type: &microflows.ObjectType{EntityQualifiedName: "System.HttpResponse"}},
		},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	svc := publishedRestService("items/{id}")
	deriveOperationParameters(ctx, svc)

	got := map[string]model.PublishedRestOperationParameter{}
	for _, p := range svc.Resources[0].Operations[0].OperationParameters {
		got[p.Name] = *p
	}
	want := map[string]model.PublishedRestOperationParameter{
		"id":          {Name: "id", ParameterType: "Path", DataType: "Integer"},
		"orderNumber": {Name: "orderNumber", ParameterType: "Query", DataType: "String"},
		"status":      {Name: "status", ParameterType: "Query", DataType: "Enumeration", QualifiedName: "Api.Status"},
		"item":        {Name: "item", ParameterType: "Body", DataType: "Object", QualifiedName: "Api.Item"},
		"lines":       {Name: "lines", ParameterType: "Body", DataType: "List", QualifiedName: "Api.Line"},
	}
	if len(got) != len(want) {
		t.Fatalf("parameters = %v, want %v", got, want)
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("parameter %s = %+v, want %+v", name, got[name], w)
		}
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected output: %q", buf.String())
	}
}

// TestDeriveOperationParameters_WarnsWhenTheMicroflowIsMissing: a service written before
// its microflow cannot know the microflow's parameters. It keeps the path parameters, as
// before, and says why the rest will fail mx check.
func TestDeriveOperationParameters_WarnsWhenTheMicroflowIsMissing(t *testing.T) {
	mod := mkModule("Api")
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return nil, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	svc := publishedRestService("items/{id}")
	deriveOperationParameters(ctx, svc)

	if n := len(svc.Resources[0].Operations[0].OperationParameters); n != 0 {
		t.Errorf("derived %d parameters from a missing microflow", n)
	}
	assertContainsStr(t, buf.String(), "microflow Api.Op not found")
	assertContainsStr(t, buf.String(), "CE0350")
}

// TestCreateAndAlterPublishedRestService_DeriveParameters: both statements that write a
// service hand the backend operations that carry their derived parameters. ALTER writes
// every operation again, so it has to derive them for the operations it did not touch too.
func TestCreateAndAlterPublishedRestService_DeriveParameters(t *testing.T) {
	mod := mkModule("Api")
	mf := &microflows.Microflow{
		ContainerID: mod.ID,
		Name:        "Op",
		Parameters:  []*microflows.MicroflowParameter{{Name: "orderNumber", Type: &microflows.StringType{}}},
	}
	var written *model.PublishedRestService
	var stored []*model.PublishedRestService
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		ListPublishedRestServicesFunc: func() ([]*model.PublishedRestService, error) {
			return stored, nil
		},
		CreatePublishedRestServiceFunc: func(svc *model.PublishedRestService) error {
			written = svc
			return nil
		},
		UpdatePublishedRestServiceFunc: func(svc *model.PublishedRestService) error {
			written = svc
			return nil
		},
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	op := func() []*ast.PublishedRestOperationDef {
		return []*ast.PublishedRestOperationDef{{HTTPMethod: "GET", Path: "status",
			Microflow: ast.QualifiedName{Module: "Api", Name: "Op"}}}
	}
	queryParams := func(svc *model.PublishedRestService) []string {
		var names []string
		for _, res := range svc.Resources {
			for _, o := range res.Operations {
				for _, p := range o.OperationParameters {
					if p.ParameterType == "Query" {
						names = append(names, res.Name+":"+p.Name)
					}
				}
			}
		}
		return names
	}

	assertNoError(t, execCreatePublishedRestService(ctx, &ast.CreatePublishedRestServiceStmt{
		Name: ast.QualifiedName{Module: "Api", Name: "Orders"}, Path: "rest/orders/v1",
		Resources: []*ast.PublishedRestResourceDef{{Name: "orders", Operations: op()}},
	}))
	if got := queryParams(written); len(got) != 1 || got[0] != "orders:orderNumber" {
		t.Fatalf("create wrote query parameters %v, want [orders:orderNumber]", got)
	}

	// The service as it reads back: the reader does not read operation parameters.
	written.ID = "prs-1"
	written.ContainerID = mod.ID
	for _, o := range written.Resources[0].Operations {
		o.OperationParameters = nil
	}
	stored = []*model.PublishedRestService{written}
	assertNoError(t, execAlterPublishedRestService(ctx, &ast.AlterPublishedRestServiceStmt{
		Name:    ast.QualifiedName{Module: "Api", Name: "Orders"},
		Actions: []ast.PublishedRestAlterAction{&ast.PublishedRestAddResourceAction{Resource: &ast.PublishedRestResourceDef{Name: "more", Operations: op()}}},
	}))
	got := queryParams(written)
	if len(got) != 2 || got[0] != "orders:orderNumber" || got[1] != "more:orderNumber" {
		t.Fatalf("alter wrote query parameters %v, want [orders:orderNumber more:orderNumber]", got)
	}
}
