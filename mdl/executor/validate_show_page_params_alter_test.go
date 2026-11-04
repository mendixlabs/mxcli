// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1940 (mendixlabs/mxcli#1736): `alter page … { add parameters $X }`
// followed in the same script by a show page passing X was refused — by check
// --references and by exec's pre-flight — because the argument check read the
// page as stored before the script. Split into two scripts, both ran.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestValidate_ShowPageSeesParametersAlteredInTheScript(t *testing.T) {
	mod := &model.Module{Name: "M"}
	mod.ID = "mod1"
	stored := &pages.Page{Name: "Edit", Parameters: []*pages.PageParameter{{Name: "District"}, {Name: "Choice"}}}
	stored.ContainerID = "mod1"
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{stored}, nil },
	}
	show := func(page string, names ...string) ast.MicroflowStatement {
		var args []ast.ShowPageArg
		for _, n := range names {
			args = append(args, ast.ShowPageArg{ParamName: n, Value: &ast.VariableExpr{Name: "V"}})
		}
		return &ast.ShowPageStmt{PageName: qn("M", page), Arguments: args}
	}
	alter := func(page string, ops ...ast.AlterPageOperation) ast.Statement {
		return &ast.AlterPageStmt{ContainerType: "PAGE", PageName: qn("M", page), Operations: ops}
	}
	add := func(name string) ast.AlterPageOperation {
		return &ast.AddParameterOp{Parameter: ast.PageParameter{Name: name}}
	}
	drop := func(name string) ast.AlterPageOperation { return &ast.DropParameterOp{ParameterName: name} }

	cases := []struct {
		name    string
		before  []ast.Statement
		call    ast.MicroflowStatement
		wantErr string
	}{
		{"the reported script: a parameter added to a stored page",
			[]ast.Statement{alter("Edit", add("Extra"))}, show("Edit", "District", "Choice", "Extra"), ""},
		{"a parameter dropped from a stored page is no longer declared",
			[]ast.Statement{alter("Edit", drop("Choice"))}, show("Edit", "District", "Choice"),
			`page M.Edit has no parameter "Choice" (declared parameters: District)`},
		{"add then drop across two alters",
			[]ast.Statement{alter("Edit", add("Extra")), alter("Edit", drop("Extra"))}, show("Edit", "Extra"),
			`page M.Edit has no parameter "Extra" (declared parameters: Choice, District)`},
		{"a page created in the script, then altered",
			[]ast.Statement{&ast.CreatePageStmtV3{Name: qn("M", "New")}, alter("New", add("Obj"))}, show("New", "Obj"), ""},
		{"control: no alter, the stored page still refuses an unknown name",
			nil, show("Edit", "Extra"), `page M.Edit has no parameter "Extra" (declared parameters: Choice, District)`},
		{"control: an alter snippet does not change the page of that name",
			[]ast.Statement{&ast.AlterPageStmt{ContainerType: "SNIPPET", PageName: qn("M", "Edit"), Operations: []ast.AlterPageOperation{add("Extra")}}},
			show("Edit", "Extra"), `page M.Edit has no parameter "Extra"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := newMockCtx(t, withBackend(mb))
			body := []ast.MicroflowStatement{tc.call}
			stmts := append(append([]ast.Statement{}, tc.before...), &ast.CreateMicroflowStmt{Name: qn("M", "Caller"), Body: body})
			sc := newScriptContext()
			sc.collectDefinitions(&ast.Program{Statements: stmts})
			errs := validateFlowBodyReferences(ctx, body, sc)
			if tc.wantErr == "" {
				if len(errs) != 0 {
					t.Fatalf("refused: %v", errs)
				}
				return
			}
			if len(errs) != 1 || !strings.Contains(errs[0], tc.wantErr) {
				t.Fatalf("errors = %v, want one containing %q", errs, tc.wantErr)
			}
		})
	}
	// The stored page itself is untouched by the simulation.
	if len(stored.Parameters) != 2 {
		t.Errorf("stored page parameters changed: %d", len(stored.Parameters))
	}
}
