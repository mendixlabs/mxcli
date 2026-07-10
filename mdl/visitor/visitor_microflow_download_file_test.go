// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestDownloadFileStatement(t *testing.T) {
	input := `create microflow SampleFiles.ACT_DownloadReport (
  $GeneratedReport: System.FileDocument
)
returns Void
begin
  download file $GeneratedReport show in browser on error rollback;
  return;
end;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Statements))
	}

	mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("expected CreateMicroflowStmt, got %T", prog.Statements[0])
	}
	if len(mf.Body) < 1 {
		t.Fatal("expected microflow body")
	}

	stmt, ok := mf.Body[0].(*ast.DownloadFileStmt)
	if !ok {
		t.Fatalf("expected DownloadFileStmt, got %T", mf.Body[0])
	}
	if stmt.FileDocument != "GeneratedReport" {
		t.Fatalf("FileDocument = %q, want GeneratedReport", stmt.FileDocument)
	}
	if !stmt.ShowInBrowser {
		t.Fatal("ShowInBrowser = false, want true")
	}
	if stmt.ErrorHandling == nil || stmt.ErrorHandling.Type != ast.ErrorHandlingRollback {
		t.Fatalf("ErrorHandling = %#v, want rollback", stmt.ErrorHandling)
	}
}
