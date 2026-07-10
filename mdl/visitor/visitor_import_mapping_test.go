// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestCreateImportMapping(t *testing.T) {
	input := `CREATE IMPORT MAPPING MyModule.PetMapping WITH JSON STRUCTURE MyModule.PetSchema {
		CREATE MyModule.Pet {
			PetId = id KEY,
			Name = name
		}
	};`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateImportMappingStmt)
	if !ok {
		t.Fatalf("Expected CreateImportMappingStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Name != "PetMapping" {
		t.Errorf("Got name %s", stmt.Name.Name)
	}
	if stmt.SchemaKind != "JSON_STRUCTURE" {
		t.Errorf("Got SchemaKind %q", stmt.SchemaKind)
	}
	if stmt.SchemaRef.Name != "PetSchema" {
		t.Errorf("Got SchemaRef %s", stmt.SchemaRef.Name)
	}
	if stmt.RootElement == nil {
		t.Fatal("Expected non-nil RootElement")
	}
}

func TestCreateImportMapping_OrModify(t *testing.T) {
	prog, errs := Build(`CREATE OR MODIFY IMPORT MAPPING MyModule.PetMapping WITH JSON STRUCTURE MyModule.PetSchema { CREATE MyModule.Pet { Id = id KEY } };`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateImportMappingStmt)
	if !ok {
		t.Fatalf("Expected CreateImportMappingStmt, got %T", prog.Statements[0])
	}
	if !stmt.CreateOrModify {
		t.Error("Expected CreateOrModify=true")
	}
}
