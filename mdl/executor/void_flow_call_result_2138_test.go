// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2138 (mendixlabs/mxcli#1820): a call to a microflow that returns
// nothing can carry a result variable name — Studio Pro keeps `variable`,
// UseReturnVariable=true, when the called flow stops returning a value, and does
// not show it. Describe printed `$variable = call microflow …`, check passed
// the same statement, and exec stored the name.

// voidCallCtx is a project with MyModule.VoidSub (returns nothing),
// MyModule.StrSub (returns String), and a context to describe a call in.
func voidCallCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("MyModule")
	void := mkMicroflow(mod.ID, "VoidSub")
	void.ReturnType = &microflows.VoidType{}
	str := mkMicroflow(mod.ID, "StrSub")
	str.ReturnType = &microflows.StringType{}
	h := mkHierarchy(mod)
	withContainer(h, void.ContainerID, mod.ID)
	withContainer(h, str.ContainerID, mod.ID)
	byID := map[model.ID]*microflows.Microflow{void.ID: void, str.ID: str}
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{void, str}, nil },
		GetRawUnitBytesFunc: func(id model.ID) ([]byte, error) {
			return []byte(id), nil
		},
		ParseMicroflowBSONFunc: func(contents []byte, unitID, _ model.ID) (*microflows.Microflow, error) {
			return byID[unitID], nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

func callAction(target, result string) *microflows.MicroflowCallAction {
	return &microflows.MicroflowCallAction{
		MicroflowCall:      &microflows.MicroflowCall{Microflow: target},
		UseReturnVariable:  true,
		ResultVariableName: result,
	}
}

func TestDescribeCallToVoidMicroflow_OmitsStaleResult_Issue2138(t *testing.T) {
	ctx := voidCallCtx(t)
	cases := []struct {
		target, want string
	}{
		{"MyModule.VoidSub", "call microflow MyModule.VoidSub();"},
		// Controls: a value is returned, or the flow cannot be found.
		{"MyModule.StrSub", "$variable = call microflow MyModule.StrSub();"},
		{"Elsewhere.Gone", "$variable = call microflow Elsewhere.Gone();"},
	}
	for _, tc := range cases {
		if got := formatAction(ctx, callAction(tc.target, "variable"), nil, nil); got != tc.want {
			t.Errorf("%s: described as %q, want %q", tc.target, got, tc.want)
		}
	}
}

func TestStoredResultVariable_NoneForAVoidFlow_Issue2138(t *testing.T) {
	cases := []struct {
		rt   microflows.DataType
		want string
	}{
		{&microflows.VoidType{}, ""},
		{&microflows.StringType{}, "V"},
		{nil, "V"}, // the flow was not found: keep what the statement says
	}
	for _, tc := range cases {
		if got := storedResultVariable("V", tc.rt); got != tc.want {
			t.Errorf("storedResultVariable(V, %T) = %q, want %q", tc.rt, got, tc.want)
		}
	}
}

func mdl096(vs []linter.Violation) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == voidFlowCallResultRule {
			out = append(out, v)
		}
	}
	return out
}

// The issue's script: check passed it silently.
func TestCheckCallToVoidMicroflowWithResult_MDL096_Issue2138(t *testing.T) {
	voidSub := &ast.CreateMicroflowStmt{Name: qn("MyModule", "VoidSub")}
	strSub := &ast.CreateMicroflowStmt{Name: qn("MyModule", "StrSub"),
		ReturnType: &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeString}}}
	caller := func(target, out string) *ast.CreateMicroflowStmt {
		return &ast.CreateMicroflowStmt{Name: qn("MyModule", "CallsVoid"), Body: []ast.MicroflowStatement{
			&ast.CallMicroflowStmt{OutputVariable: out, MicroflowName: qn("MyModule", target)}}}
	}
	cases := []struct {
		name string
		flow *ast.CreateMicroflowStmt
		want int
	}{
		{"result on a call to a void microflow", caller("VoidSub", "variable"), 1},
		{"control: no result", caller("VoidSub", ""), 0},
		{"control: the callee returns a value", caller("StrSub", "variable"), 0},
		{"control: the callee is unknown", caller("Gone", "variable"), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mdl096(ValidateProgram(&ast.Program{Statements: []ast.Statement{voidSub, strSub, tc.flow}}, ""))
			if len(got) != tc.want {
				t.Fatalf("MDL096 count = %d, want %d: %v", len(got), tc.want, got)
			}
			if tc.want > 0 && (got[0].Severity != linter.SeverityWarning || !strings.Contains(got[0].Message, "returns nothing")) {
				t.Errorf("want a warning naming the void callee, got %v", got[0])
			}
		})
	}
}
