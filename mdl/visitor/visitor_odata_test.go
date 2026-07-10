// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestCreateODataClient_Basic(t *testing.T) {
	input := `CREATE ODATA CLIENT MyModule.PetStore (
		Version: '1.0',
		ODataVersion: OData4,
		MetadataUrl: 'https://petstore.example.com/odata/$metadata'
	);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.CreateODataClientStmt)
	if !ok {
		t.Fatalf("Expected CreateODataClientStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Module != "MyModule" || stmt.Name.Name != "PetStore" {
		t.Errorf("Expected MyModule.PetStore, got %s.%s", stmt.Name.Module, stmt.Name.Name)
	}
	if stmt.MetadataUrl != "https://petstore.example.com/odata/$metadata" {
		t.Errorf("Got MetadataUrl %q", stmt.MetadataUrl)
	}
	if stmt.Version != "1.0" {
		t.Errorf("Got Version %q", stmt.Version)
	}
}

func TestCreateODataService(t *testing.T) {
	input := `CREATE ODATA SERVICE MyModule.ProductAPI (
		Path: '/odata/v1/products',
		Version: '1.0.0',
		ODataVersion: OData4
	) AUTHENTICATION Basic, Session
	{
		PUBLISH ENTITY MyModule.Product AS 'Products'
			(ReadMode: 'FromDatabase')
			EXPOSE (Name, Price);
	};`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateODataServiceStmt)
	if !ok {
		t.Fatalf("Expected CreateODataServiceStmt, got %T", prog.Statements[0])
	}
	if stmt.Path != "/odata/v1/products" {
		t.Errorf("Got Path %q", stmt.Path)
	}
	if len(stmt.AuthenticationTypes) != 2 {
		t.Fatalf("Expected 2 auth types, got %d", len(stmt.AuthenticationTypes))
	}
	if len(stmt.Entities) != 1 {
		t.Fatalf("Expected 1 entity, got %d", len(stmt.Entities))
	}
	if stmt.Entities[0].ExposedName != "Products" {
		t.Errorf("Got ExposedName %q", stmt.Entities[0].ExposedName)
	}
	if len(stmt.Entities[0].Members) != 2 {
		t.Errorf("Expected 2 members, got %d", len(stmt.Entities[0].Members))
	}
}

func TestCreateExternalEntity(t *testing.T) {
	input := `CREATE EXTERNAL ENTITY MyModule.RemoteProduct FROM ODATA CLIENT MyModule.PetStore
		(EntitySet: 'Products', RemoteName: 'Product', Countable: true)
		(Name: String(200), Price: Decimal);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateExternalEntityStmt)
	if !ok {
		t.Fatalf("Expected CreateExternalEntityStmt, got %T", prog.Statements[0])
	}
	if stmt.ServiceRef.Name != "PetStore" {
		t.Errorf("Got ServiceRef %s", stmt.ServiceRef.Name)
	}
	if stmt.EntitySet == nil || *stmt.EntitySet != "Products" {
		t.Errorf("Got EntitySet %v", stmt.EntitySet)
	}
	if stmt.RemoteName == nil || *stmt.RemoteName != "Product" {
		t.Errorf("Got RemoteName %v", stmt.RemoteName)
	}
	if stmt.Countable == nil || !*stmt.Countable {
		t.Errorf("Expected Countable true, got %v", stmt.Countable)
	}
	if len(stmt.Attributes) != 2 {
		t.Errorf("Expected 2 attributes, got %d", len(stmt.Attributes))
	}
}

func TestCreateExternalEntities(t *testing.T) {
	input := `CREATE EXTERNAL ENTITIES FROM MyModule.PetStore INTO Integration ENTITIES (Product, Category);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateExternalEntitiesStmt)
	if !ok {
		t.Fatalf("Expected CreateExternalEntitiesStmt, got %T", prog.Statements[0])
	}
	if stmt.ServiceRef.Name != "PetStore" {
		t.Errorf("Got ServiceRef %s", stmt.ServiceRef.Name)
	}
	if stmt.TargetModule != "Integration" {
		t.Errorf("Got TargetModule %q", stmt.TargetModule)
	}
	if len(stmt.EntityNames) != 2 {
		t.Fatalf("Expected 2 entity names, got %d", len(stmt.EntityNames))
	}
	if stmt.EntityNames[0] != "Product" || stmt.EntityNames[1] != "Category" {
		t.Errorf("Got %v", stmt.EntityNames)
	}
}
