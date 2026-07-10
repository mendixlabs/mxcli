// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestAlterEnumeration_AddValue(t *testing.T) {
	input := `ALTER ENUMERATION MyModule.Status ADD VALUE Pending CAPTION 'Pending';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.AlterEnumerationStmt)
	if !ok {
		t.Fatalf("Expected AlterEnumerationStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Name != "Status" {
		t.Errorf("Got name %s", stmt.Name.Name)
	}
	if stmt.Operation != ast.AlterEnumAdd {
		t.Errorf("Expected Add, got %v", stmt.Operation)
	}
	if stmt.ValueName != "Pending" {
		t.Errorf("Got ValueName %q", stmt.ValueName)
	}
	if stmt.Caption != "Pending" {
		t.Errorf("Got Caption %q", stmt.Caption)
	}
}

func TestAlterEnumeration_RenameValue(t *testing.T) {
	input := `ALTER ENUMERATION MyModule.Status RENAME VALUE Active TO Enabled;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt := prog.Statements[0].(*ast.AlterEnumerationStmt)
	if stmt.Operation != ast.AlterEnumRename {
		t.Errorf("Expected Rename, got %v", stmt.Operation)
	}
	if stmt.ValueName != "Active" {
		t.Errorf("Got ValueName %q", stmt.ValueName)
	}
	if stmt.NewName != "Enabled" {
		t.Errorf("Got NewName %q", stmt.NewName)
	}
}

func TestAlterEnumeration_DropValue(t *testing.T) {
	input := `ALTER ENUMERATION MyModule.Status DROP VALUE Obsolete;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt := prog.Statements[0].(*ast.AlterEnumerationStmt)
	if stmt.Operation != ast.AlterEnumDrop {
		t.Errorf("Expected Drop, got %v", stmt.Operation)
	}
}

func TestCreateConstant(t *testing.T) {
	input := `CREATE CONSTANT MyModule.MaxRetries TYPE Integer DEFAULT 3 COMMENT 'Max retry count';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateConstantStmt)
	if !ok {
		t.Fatalf("Expected CreateConstantStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Name != "MaxRetries" {
		t.Errorf("Got name %s", stmt.Name.Name)
	}
	if stmt.Comment != "Max retry count" {
		t.Errorf("Got Comment %q", stmt.Comment)
	}
}

func TestCreateConstant_ExposedToClient(t *testing.T) {
	input := `CREATE CONSTANT MyModule.AppUrl TYPE String DEFAULT 'https://example.com' EXPOSED TO CLIENT;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt := prog.Statements[0].(*ast.CreateConstantStmt)
	if !stmt.ExposedToClient {
		t.Error("Expected ExposedToClient true")
	}
}
