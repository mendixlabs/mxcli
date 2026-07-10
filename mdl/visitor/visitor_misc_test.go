// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestSearch(t *testing.T) {
	input := `SEARCH 'customer';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.SearchStmt)
	if !ok {
		t.Fatalf("Expected SearchStmt, got %T", prog.Statements[0])
	}
	if stmt.Query != "customer" {
		t.Errorf("Got Query %q", stmt.Query)
	}
}

func TestExecuteScript(t *testing.T) {
	input := `EXECUTE SCRIPT 'myscript.mdl';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.ExecuteScriptStmt)
	if !ok {
		t.Fatalf("Expected ExecuteScriptStmt, got %T", prog.Statements[0])
	}
	if stmt.Path != "myscript.mdl" {
		t.Errorf("Got Path %q", stmt.Path)
	}
}

func TestHelp(t *testing.T) {
	input := `help;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	_, ok := prog.Statements[0].(*ast.HelpStmt)
	if !ok {
		t.Fatalf("Expected HelpStmt, got %T", prog.Statements[0])
	}
}

func TestSessionSet_StringValue(t *testing.T) {
	for _, tc := range []struct {
		input   string
		key     string
		wantVal string
	}{
		{`SET format = json;`, "format", "json"},
		{`SET format = 'table';`, "format", "table"},
	} {
		prog, errs := Build(tc.input)
		if len(errs) > 0 {
			t.Errorf("%s: parse errors: %v", tc.input, errs)
			continue
		}
		stmt, ok := prog.Statements[0].(*ast.SetStmt)
		if !ok {
			t.Fatalf("%s: expected SetStmt, got %T", tc.input, prog.Statements[0])
		}
		if stmt.Key != tc.key {
			t.Errorf("%s: Key = %q, want %q", tc.input, stmt.Key, tc.key)
		}
		if stmt.Value != tc.wantVal {
			t.Errorf("%s: Value = %v, want %q", tc.input, stmt.Value, tc.wantVal)
		}
	}
}

func TestSessionSet_BoolValue(t *testing.T) {
	prog, errs := Build(`SET debug = true;`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.SetStmt)
	if !ok {
		t.Fatalf("Expected SetStmt, got %T", prog.Statements[0])
	}
	if stmt.Key != "debug" {
		t.Errorf("Key = %q, want %q", stmt.Key, "debug")
	}
	if stmt.Value != true {
		t.Errorf("Value = %v, want true", stmt.Value)
	}
}

func TestSessionSet_NumberValue(t *testing.T) {
	prog, errs := Build(`SET limit = 100;`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.SetStmt)
	if !ok {
		t.Fatalf("Expected SetStmt, got %T", prog.Statements[0])
	}
	if stmt.Key != "limit" {
		t.Errorf("Key = %q, want %q", stmt.Key, "limit")
	}
	if stmt.Value != int64(100) {
		t.Errorf("Value = %v (%T), want int64(100)", stmt.Value, stmt.Value)
	}
}

func TestUpdate(t *testing.T) {
	input := `UPDATE;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	_, ok := prog.Statements[0].(*ast.UpdateStmt)
	if !ok {
		t.Fatalf("Expected UpdateStmt, got %T", prog.Statements[0])
	}
}
