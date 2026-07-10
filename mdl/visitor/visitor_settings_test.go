// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestAlterSettings_Model(t *testing.T) {
	input := `ALTER SETTINGS MODEL DefaultLanguage = 'en_US';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.AlterSettingsStmt)
	if !ok {
		t.Fatalf("Expected AlterSettingsStmt, got %T", prog.Statements[0])
	}
	if stmt.Section != "MODEL" {
		t.Errorf("Got Section %q", stmt.Section)
	}
	if stmt.Properties["DefaultLanguage"] != "en_US" {
		t.Errorf("Got %v", stmt.Properties["DefaultLanguage"])
	}
}

func TestAlterSettings_Constant(t *testing.T) {
	input := `ALTER SETTINGS CONSTANT 'MyModule.APIEndpoint' VALUE 'https://prod.example.com';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.AlterSettingsStmt)
	if !ok {
		t.Fatalf("Expected AlterSettingsStmt, got %T", prog.Statements[0])
	}
	if stmt.ConstantId != "MyModule.APIEndpoint" {
		t.Errorf("Got ConstantId %q", stmt.ConstantId)
	}
	if stmt.Value != "https://prod.example.com" {
		t.Errorf("Got Value %q", stmt.Value)
	}
}

func TestAlterSettings_DropConstant(t *testing.T) {
	input := `ALTER SETTINGS DROP CONSTANT 'MyModule.OldConst';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.AlterSettingsStmt)
	if !ok {
		t.Fatalf("Expected AlterSettingsStmt, got %T", prog.Statements[0])
	}
	if !stmt.DropConstant {
		t.Error("Expected DropConstant true")
	}
}

func TestCreateConfiguration(t *testing.T) {
	input := `CREATE CONFIGURATION 'Acceptance' DatabaseHost = 'db.acc.example.com', DatabaseName = 'myapp_acc';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateConfigurationStmt)
	if !ok {
		t.Fatalf("Expected CreateConfigurationStmt, got %T", prog.Statements[0])
	}
	if stmt.Name != "Acceptance" {
		t.Errorf("Got Name %q", stmt.Name)
	}
	if stmt.Properties["DatabaseHost"] != "db.acc.example.com" {
		t.Errorf("Got %v", stmt.Properties["DatabaseHost"])
	}
}
