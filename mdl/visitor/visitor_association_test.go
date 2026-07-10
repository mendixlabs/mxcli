// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestCreateAssociation(t *testing.T) {
	input := `CREATE ASSOCIATION MyModule.Order_Customer FROM MyModule.Order TO MyModule.Customer TYPE REFERENCE OWNER DEFAULT;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateAssociationStmt)
	if !ok {
		t.Fatalf("Expected CreateAssociationStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Module != "MyModule" || stmt.Name.Name != "Order_Customer" {
		t.Errorf("Got name %s.%s", stmt.Name.Module, stmt.Name.Name)
	}
	if stmt.Parent.Name != "Order" {
		t.Errorf("Got parent %s", stmt.Parent.Name)
	}
	if stmt.Child.Name != "Customer" {
		t.Errorf("Got child %s", stmt.Child.Name)
	}
	if stmt.Type != ast.AssocReference {
		t.Errorf("Expected Reference type, got %v", stmt.Type)
	}
}

func TestCreateAssociation_ReferenceSet(t *testing.T) {
	input := `CREATE ASSOCIATION MyModule.Order_Product FROM MyModule.Order TO MyModule.Product TYPE REFERENCE_SET OWNER BOTH;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt := prog.Statements[0].(*ast.CreateAssociationStmt)
	if stmt.Type != ast.AssocReferenceSet {
		t.Errorf("Expected ReferenceSet, got %v", stmt.Type)
	}
	if stmt.Owner != ast.OwnerBoth {
		t.Errorf("Expected OwnerBoth, got %v", stmt.Owner)
	}
}

func TestAlterAssociation_SetOwner(t *testing.T) {
	input := `ALTER ASSOCIATION MyModule.Order_Customer SET OWNER BOTH;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.AlterAssociationStmt)
	if !ok {
		t.Fatalf("Expected AlterAssociationStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Name != "Order_Customer" {
		t.Errorf("Got name %s", stmt.Name.Name)
	}
	if stmt.Owner != ast.OwnerBoth {
		t.Errorf("Expected OwnerBoth, got %v", stmt.Owner)
	}
}

func TestCreateAssociation_OrModify(t *testing.T) {
	prog, errs := Build(`CREATE OR MODIFY ASSOCIATION MyModule.Order_Customer FROM MyModule.Order TO MyModule.Customer TYPE REFERENCE;`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateAssociationStmt)
	if !ok {
		t.Fatalf("Expected CreateAssociationStmt, got %T", prog.Statements[0])
	}
	if !stmt.CreateOrModify {
		t.Error("Expected CreateOrModify=true")
	}
}
