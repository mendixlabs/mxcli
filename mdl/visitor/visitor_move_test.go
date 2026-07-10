// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestMovePage_ToFolder(t *testing.T) {
	input := `MOVE PAGE MyModule.OldPage TO FOLDER 'Admin/Pages';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.MoveStmt)
	if !ok {
		t.Fatalf("Expected MoveStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Name != "OldPage" {
		t.Errorf("Got name %s", stmt.Name.Name)
	}
	if stmt.Folder != "Admin/Pages" {
		t.Errorf("Got Folder %q", stmt.Folder)
	}
}

func TestMoveEntity_ToModule(t *testing.T) {
	input := `MOVE ENTITY MyModule.Customer TO OtherModule;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.MoveStmt)
	if !ok {
		t.Fatalf("Expected MoveStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Name != "Customer" {
		t.Errorf("Got name %s", stmt.Name.Name)
	}
	if stmt.TargetModule != "OtherModule" {
		t.Errorf("Got TargetModule %q", stmt.TargetModule)
	}
}
