// SPDX-License-Identifier: Apache-2.0

package adapters

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func src(s string) ast.Expression { return &ast.SourceExpr{Source: s} }

func dt(k ast.DataTypeKind) ast.DataType { return ast.DataType{Kind: k} }

func countE009(r *Result) int {
	n := 0
	for _, h := range r.Hints {
		if h.Code == "E009" {
			n++
		}
	}
	return n
}

// DECLARE, SET and RETURN each check the expression against the type the
// position was declared with, resolved from the AST rather than the catalog.
// Measured on MxBuild 11.16.1: all three are CE0117 for a mismatch
// (see exprcheck.TestTargetKindMatrix for the matrix itself).
func TestTargetKindWiredForDeclareSetReturn(t *testing.T) {
	flow := func(body ...ast.MicroflowStatement) *ast.CreateMicroflowStmt {
		return &ast.CreateMicroflowStmt{
			Name:       ast.QualifiedName{Module: "M", Name: "F"},
			ReturnType: &ast.MicroflowReturnType{Type: dt(ast.TypeInteger)},
			Body:       body,
		}
	}
	cases := []struct {
		name string
		body []ast.MicroflowStatement
		want int
	}{
		{"declare mismatch", []ast.MicroflowStatement{
			&ast.DeclareStmt{Variable: "X", Type: dt(ast.TypeInteger), InitialValue: src("'abc'")},
			&ast.ReturnStmt{Value: src("$X")},
		}, 1},
		{"declare ok", []ast.MicroflowStatement{
			&ast.DeclareStmt{Variable: "X", Type: dt(ast.TypeInteger), InitialValue: src("1")},
			&ast.ReturnStmt{Value: src("$X")},
		}, 0},
		{"set mismatch", []ast.MicroflowStatement{
			&ast.DeclareStmt{Variable: "X", Type: dt(ast.TypeInteger), InitialValue: src("0")},
			&ast.MfSetStmt{Target: "X", Value: src("'abc'")},
			&ast.ReturnStmt{Value: src("$X")},
		}, 1},
		{"set widening Integer to Decimal is fine", []ast.MicroflowStatement{
			&ast.DeclareStmt{Variable: "D", Type: dt(ast.TypeDecimal), InitialValue: src("0")},
			&ast.MfSetStmt{Target: "D", Value: src("1")},
			&ast.ReturnStmt{Value: src("1")},
		}, 0},
		{"set Decimal into Integer", []ast.MicroflowStatement{
			&ast.DeclareStmt{Variable: "X", Type: dt(ast.TypeInteger), InitialValue: src("0")},
			&ast.MfSetStmt{Target: "X", Value: src("1.5")},
			&ast.ReturnStmt{Value: src("$X")},
		}, 1},
		{"return mismatch", []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: src("'abc'")},
		}, 1},
		{"return in a branch", []ast.MicroflowStatement{
			&ast.IfStmt{
				Condition: src("true"),
				ThenBody:  []ast.MicroflowStatement{&ast.ReturnStmt{Value: src("true")}},
				ElseBody:  []ast.MicroflowStatement{&ast.ReturnStmt{Value: src("1")}},
			},
		}, 1},
		// MDL041 already reports a COMPUTED Decimal assigned to an Integer by
		// DECLARE or SET; reporting it here too would print one problem twice.
		{"computed Decimal into Integer declare is MDL041's", []ast.MicroflowStatement{
			&ast.DeclareStmt{Variable: "X", Type: dt(ast.TypeInteger), InitialValue: src("1.5 * 2")},
			&ast.ReturnStmt{Value: src("$X")},
		}, 0},
		{"computed Decimal into Integer set is MDL041's", []ast.MicroflowStatement{
			&ast.DeclareStmt{Variable: "X", Type: dt(ast.TypeInteger), InitialValue: src("0")},
			&ast.MfSetStmt{Target: "X", Value: src("1 div 2")},
			&ast.ReturnStmt{Value: src("$X")},
		}, 0},
		// MDL041 does not look at a RETURN, a bare Decimal literal, or a Decimal variable.
		{"computed Decimal returned as Integer is not MDL041's", []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: src("1.5 * 2")},
		}, 1},
		{"Decimal variable into Integer declare is not MDL041's", []ast.MicroflowStatement{
			&ast.DeclareStmt{Variable: "D", Type: dt(ast.TypeDecimal), InitialValue: src("0")},
			&ast.DeclareStmt{Variable: "X", Type: dt(ast.TypeInteger), InitialValue: src("$D")},
			&ast.ReturnStmt{Value: src("$X")},
		}, 1},
		// An attribute path is typed by the attribute, not by a declared variable;
		// the CHANGE/CREATE slots own that.
		{"set of an attribute path is not this rule", []ast.MicroflowStatement{
			&ast.MfSetStmt{Target: "Order/Status", Value: src("'abc'")},
			&ast.ReturnStmt{Value: src("1")},
		}, 0},
		// A variable the scope cannot type stays silent.
		{"set of an unknown variable", []ast.MicroflowStatement{
			&ast.MfSetStmt{Target: "Nope", Value: src("'abc'")},
			&ast.ReturnStmt{Value: src("1")},
		}, 0},
		// An entity-typed declare is not a primitive target.
		{"entity declare is not checked", []ast.MicroflowStatement{
			&ast.DeclareStmt{Variable: "O", Type: ast.DataType{Kind: ast.TypeEntity, EntityRef: &ast.QualifiedName{Module: "M", Name: "E"}}, InitialValue: src("empty")},
			&ast.ReturnStmt{Value: src("1")},
		}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := NewCheckAdapter(nil).CheckMicroflow(flow(c.body...))
			if got := countE009(r); got != c.want {
				t.Errorf("E009 count = %d, want %d: %+v", got, c.want, r.Hints)
			}
		})
	}
}

// A flow with no return type (or Nothing) has nothing to check a RETURN against.
func TestTargetKindReturnWithoutType(t *testing.T) {
	for _, rt := range []*ast.MicroflowReturnType{nil, {Type: dt(ast.TypeVoid)}} {
		stmt := &ast.CreateMicroflowStmt{
			Name:       ast.QualifiedName{Module: "M", Name: "F"},
			ReturnType: rt,
			Body:       []ast.MicroflowStatement{&ast.ReturnStmt{Value: src("'abc'")}},
		}
		if got := countE009(NewCheckAdapter(nil).CheckMicroflow(stmt)); got != 0 {
			t.Errorf("return type %+v: E009 count = %d, want 0", rt, got)
		}
	}
}

// A nanoflow is the same walk with the same declared types.
func TestTargetKindNanoflow(t *testing.T) {
	stmt := &ast.CreateNanoflowStmt{
		Name:       ast.QualifiedName{Module: "M", Name: "N"},
		ReturnType: &ast.MicroflowReturnType{Type: dt(ast.TypeString)},
		Body:       []ast.MicroflowStatement{&ast.ReturnStmt{Value: src("1")}},
	}
	if got := countE009(NewCheckAdapter(nil).CheckNanoflow(stmt)); got != 1 {
		t.Errorf("E009 count = %d, want 1", got)
	}
}
