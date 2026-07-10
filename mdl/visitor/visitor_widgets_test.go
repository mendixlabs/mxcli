// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestUpdateWidgets(t *testing.T) {
	input := `UPDATE WIDGETS SET 'showLabel' = false WHERE WidgetType LIKE '%textbox%';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.UpdateWidgetsStmt)
	if !ok {
		t.Fatalf("Expected UpdateWidgetsStmt, got %T", prog.Statements[0])
	}
	if len(stmt.Assignments) != 1 {
		t.Fatalf("Expected 1 assignment, got %d", len(stmt.Assignments))
	}
	if stmt.Assignments[0].PropertyPath != "showLabel" {
		t.Errorf("Got PropertyPath %q", stmt.Assignments[0].PropertyPath)
	}
	if len(stmt.Filters) != 1 {
		t.Fatalf("Expected 1 filter, got %d", len(stmt.Filters))
	}
	if stmt.DryRun {
		t.Error("Expected DryRun false")
	}
}

func TestUpdateWidgets_DryRun(t *testing.T) {
	input := `UPDATE WIDGETS SET 'editable' = true WHERE WidgetType = 'TextBox' IN MyModule DRY RUN;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	if len(prog.Statements) == 0 {
		t.Fatal("Expected at least one statement")
	}
	stmt, ok := prog.Statements[0].(*ast.UpdateWidgetsStmt)
	if !ok {
		t.Fatalf("Expected *ast.UpdateWidgetsStmt, got %T", prog.Statements[0])
	}
	if !stmt.DryRun {
		t.Error("Expected DryRun true")
	}
	if stmt.InModule != "MyModule" {
		t.Errorf("Got InModule %q", stmt.InModule)
	}
}
