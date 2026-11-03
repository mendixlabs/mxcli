// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1735 (ako/mxcli#1939): "There is no MDL statement for the
// Send external object and Delete external object activities."
func TestExternalObjectStatementsParse(t *testing.T) {
	prog, errs := Build(`create microflow Main.ACT ($District: Main.District)
begin
  send external object $District refresh;
  delete external object $District on error rollback;
  delete $District;
end;`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	body := prog.Statements[0].(*ast.CreateMicroflowStmt).Body
	send, ok := body[0].(*ast.SendExternalObjectStmt)
	if !ok || send.Variable != "District" || !send.RefreshInClient {
		t.Errorf("send = %#v", body[0])
	}
	del, ok := body[1].(*ast.DeleteExternalObjectStmt)
	if !ok || del.Variable != "District" || del.RefreshInClient || del.ErrorHandling == nil ||
		del.ErrorHandling.Type != ast.ErrorHandlingRollback {
		t.Errorf("delete external = %#v", body[1])
	}
	if _, ok := body[2].(*ast.DeleteObjectStmt); !ok {
		t.Errorf("a plain delete must still parse as a delete, got %T", body[2])
	}
}
