// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1914: a call external action argument's Associations to include,
// written as `include associations ( … )` after the argument.
package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func externalActionArgs(t *testing.T, call string) ([]ast.CallArgument, []error) {
	t.Helper()
	prog, errs := Build("create microflow M.F ($Cmd: M.Cmd)\nbegin\n" + call + "\nend;")
	if len(errs) > 0 {
		return nil, errs
	}
	body := prog.Statements[0].(*ast.CreateMicroflowStmt).Body
	for _, st := range body {
		if c, ok := st.(*ast.CallExternalActionStmt); ok {
			return c.Arguments, nil
		}
	}
	t.Fatalf("no call external action in %q", call)
	return nil, nil
}

func TestIncludeAssociationsClause(t *testing.T) {
	args, errs := externalActionArgs(t, `$R = call external action M.Svc.Create(
  command = $Cmd include associations (Odata.Quantity (Odata.Unit, Odata.Price (Odata.Currency)), Odata.Lines),
  additional = empty
);`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if len(args) != 2 {
		t.Fatalf("args = %d, want 2", len(args))
	}
	if got := args[0].Value.(*ast.VariableExpr).Name; got != "Cmd" {
		t.Errorf("the argument value is %q, want Cmd (the clause is not part of it)", got)
	}
	ic := args[0].IncludeAssociations
	if ic == nil || len(ic.Associations) != 2 {
		t.Fatalf("tree = %+v, want two roots", ic)
	}
	qty := ic.Associations[0]
	if qty.Name.String() != "Odata.Quantity" || len(qty.Children) != 2 ||
		qty.Children[0].Name.String() != "Odata.Unit" || len(qty.Children[0].Children) != 0 ||
		qty.Children[1].Name.String() != "Odata.Price" || len(qty.Children[1].Children) != 1 ||
		qty.Children[1].Children[0].Name.String() != "Odata.Currency" {
		t.Errorf("nesting lost: %+v", qty)
	}
	if ic.Associations[1].Name.String() != "Odata.Lines" {
		t.Errorf("second root = %s", ic.Associations[1].Name)
	}
	if args[1].IncludeAssociations != nil {
		t.Errorf("an argument without the clause has a tree: %+v", args[1].IncludeAssociations)
	}
}

func TestIncludeAssociationsClause_EmptyStatesNone(t *testing.T) {
	args, errs := externalActionArgs(t, `call external action M.Svc.Create(command = $Cmd include associations ());`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if ic := args[0].IncludeAssociations; ic == nil || len(ic.Associations) != 0 {
		t.Errorf("an empty clause must be present and empty, got %+v", ic)
	}
}

func TestIncludeAssociationsClause_Refusals(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"unqualified association", `call external action M.Svc.Create(command = $Cmd include associations (Quantity));`,
			"`Quantity` in include associations is not qualified"},
		{"unqualified nested association", `call external action M.Svc.Create(command = $Cmd include associations (M.Quantity (Unit)));`,
			"`Unit` in include associations is not qualified"},
		{"on another call", `call microflow M.Other(Cmd = $Cmd include associations (M.Quantity));`,
			"only call external action has"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := externalActionArgs(t, tc.src)
			joined := ""
			for _, e := range errs {
				joined += e.Error() + "\n"
			}
			if !strings.Contains(joined, tc.want) {
				t.Errorf("errors = %q, want %q", joined, tc.want)
			}
		})
	}
}

// "include" is a keyword now; it must still parse as a name.
func TestIncludeIsStillAName(t *testing.T) {
	if _, errs := Build("create persistent entity M.E (Include: Boolean);"); len(errs) > 0 {
		t.Fatalf("an attribute called Include no longer parses: %v", errs)
	}
}
