// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// Header handler clauses sit between the header options and BEGIN. They carry
// their own qualified names, so the header's positional name reading (parameter
// entity, overview page) must be unaffected by them.
func TestWorkflowVisitor_EventHandlers(t *testing.T) {
	input := `create workflow M.W
  parameter $C: M.Ctx
  overview page M.Overview
  on workflow events (UserTaskStarted, UserTaskEnded) microflow M.ACT_Audit as 'Task audit'
  on any workflow event microflow M.ACT_Log
begin
  user task T 'c' outcomes 'a' { };
end workflow;`
	stmt := buildWorkflowStmt(t, input)

	if got := stmt.ParameterEntity.String(); got != "M.Ctx" {
		t.Errorf("parameter entity = %q", got)
	}
	if got := stmt.OverviewPage.String(); got != "M.Overview" {
		t.Errorf("overview page = %q", got)
	}
	if len(stmt.EventHandlers) != 2 {
		t.Fatalf("handlers = %d, want 2", len(stmt.EventHandlers))
	}
	named := stmt.EventHandlers[0]
	if named.AnyEvent || !reflect.DeepEqual(named.EventTypes, []string{"UserTaskStarted", "UserTaskEnded"}) ||
		named.Microflow.String() != "M.ACT_Audit" || named.Description != "Task audit" {
		t.Errorf("named handler = %+v", named)
	}
	any := stmt.EventHandlers[1]
	if !any.AnyEvent || len(any.EventTypes) != 0 || any.Microflow.String() != "M.ACT_Log" || any.Description != "" {
		t.Errorf("any handler = %+v", any)
	}
}

// MICROFLOW occurs in both `targeting … microflow` and `on created microflow`,
// and qualified names are read by position, so each combination must land its
// names on the right fields.
func TestWorkflowVisitor_UserTaskOnCreated(t *testing.T) {
	input := `create workflow M.W
begin
  user task T 'c' page M.P targeting microflow M.Targets on created microflow M.OnCreated entity M.TaskEnt outcomes 'a' { };
  user task U 'u' on created microflow M.OnlyCreated outcomes 'a' { };
  multi user task V 'v' page M.P targeting groups microflow M.Groups outcomes 'a' { };
  multi user task X 'x' page M.P targeting xpath '[true()]' on created microflow M.XCreated outcomes 'a' { };
end workflow;`
	stmt := buildWorkflowStmt(t, input)
	if len(stmt.Activities) != 4 {
		t.Fatalf("activities = %d, want 4", len(stmt.Activities))
	}
	task := func(i int) *ast.WorkflowUserTaskNode {
		n, ok := stmt.Activities[i].(*ast.WorkflowUserTaskNode)
		if !ok {
			t.Fatalf("activity %d is %T", i, stmt.Activities[i])
		}
		return n
	}

	tk := task(0)
	if tk.Page.String() != "M.P" || tk.Targeting.Kind != "microflow" || tk.Targeting.Microflow.String() != "M.Targets" ||
		tk.OnCreated.String() != "M.OnCreated" || tk.Entity.String() != "M.TaskEnt" {
		t.Errorf("T = page %s, targeting %s %s, on created %s, entity %s",
			tk.Page, tk.Targeting.Kind, tk.Targeting.Microflow, tk.OnCreated, tk.Entity)
	}
	u := task(1)
	if u.Targeting.Kind != "" || u.OnCreated.String() != "M.OnlyCreated" || u.Page.Module != "" {
		t.Errorf("U = targeting %q, on created %s, page %s", u.Targeting.Kind, u.OnCreated, u.Page)
	}
	v := task(2)
	if v.Targeting.Kind != "group_microflow" || v.Targeting.Microflow.String() != "M.Groups" || v.OnCreated.Module != "" {
		t.Errorf("V = targeting %q %s, on created %s", v.Targeting.Kind, v.Targeting.Microflow, v.OnCreated)
	}
	x := task(3)
	if !x.IsMultiUser || x.Targeting.Kind != "xpath" || x.OnCreated.String() != "M.XCreated" {
		t.Errorf("X = multi %v, targeting %q, on created %s", x.IsMultiUser, x.Targeting.Kind, x.OnCreated)
	}
}

func buildWorkflowStmt(t *testing.T, input string) *ast.CreateWorkflowStmt {
	t.Helper()
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateWorkflowStmt)
	if !ok {
		t.Fatalf("statement is %T", prog.Statements[0])
	}
	return stmt
}
