// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestCreateDatabaseConnection(t *testing.T) {
	input := `CREATE DATABASE CONNECTION MyModule.MyDB TYPE 'PostgreSQL' HOST 'localhost' PORT 5432 DATABASE 'mydb' USERNAME 'user' PASSWORD 'pass';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateDatabaseConnectionStmt)
	if !ok {
		t.Fatalf("Expected CreateDatabaseConnectionStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Name != "MyDB" {
		t.Errorf("Got name %s", stmt.Name.Name)
	}
	if stmt.DatabaseType != "PostgreSQL" {
		t.Errorf("Got DatabaseType %q", stmt.DatabaseType)
	}
	if stmt.Host != "localhost" {
		t.Errorf("Got Host %q", stmt.Host)
	}
	if stmt.Port != 5432 {
		t.Errorf("Got Port %d", stmt.Port)
	}
	if stmt.UserName != "user" {
		t.Errorf("Got UserName %q", stmt.UserName)
	}
}
