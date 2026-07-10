// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestCreateJsonStructure(t *testing.T) {
	input := `CREATE JSON STRUCTURE MyModule.PetSchema SNIPPET '{"id": 1, "name": "test"}';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateJsonStructureStmt)
	if !ok {
		t.Fatalf("Expected CreateJsonStructureStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Name != "PetSchema" {
		t.Errorf("Got name %s", stmt.Name.Name)
	}
	if stmt.JsonSnippet != `{"id": 1, "name": "test"}` {
		t.Errorf("Got JsonSnippet %q", stmt.JsonSnippet)
	}
}
