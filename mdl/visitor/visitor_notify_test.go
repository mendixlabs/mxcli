// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `target` takes the element's three-part name; without the clause the target is
// empty (and MDL-WF16 refuses it at check).
func TestNotifyWorkflowStatement_Target(t *testing.T) {
	prog, errs := Build(`create microflow M.Notify ($Workflow: System.Workflow)
begin
  $Notified = notify workflow $Workflow target HR.Leave.espCancelStart;
  notify workflow $Workflow target HR."Leave".withdrawn;
  notify workflow $Workflow;
end;`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	want := []ast.NotifyWorkflowStmt{
		{OutputVariable: "Notified", WorkflowVariable: "Workflow", Target: "HR.Leave.espCancelStart"},
		{WorkflowVariable: "Workflow", Target: "HR.Leave.withdrawn"},
		{WorkflowVariable: "Workflow"},
	}
	if len(mf.Body) != len(want) {
		t.Fatalf("body has %d statements, want %d", len(mf.Body), len(want))
	}
	for i, w := range want {
		got, ok := mf.Body[i].(*ast.NotifyWorkflowStmt)
		if !ok {
			t.Fatalf("statement %d is %T", i, mf.Body[i])
		}
		if got.OutputVariable != w.OutputVariable || got.WorkflowVariable != w.WorkflowVariable || got.Target != w.Target {
			t.Errorf("statement %d = %+v, want %+v", i, *got, w)
		}
	}
}
