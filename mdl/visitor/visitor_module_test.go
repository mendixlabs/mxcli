// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestCreateModule(t *testing.T) {
	input := `CREATE MODULE OrderManagement;`
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
	stmt, ok := prog.Statements[0].(*ast.CreateModuleStmt)
	if !ok {
		t.Fatalf("Expected CreateModuleStmt, got %T", prog.Statements[0])
	}
	if stmt.Name != "OrderManagement" {
		t.Errorf("Expected OrderManagement, got %q", stmt.Name)
	}
}
