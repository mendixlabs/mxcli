// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"testing"

	"go.starlark.net/starlarkstruct"

	"github.com/JordtenBulte-OLC/mxcli/mdl/exprcheck"
)

func TestRobustExprToStarlark_nil(t *testing.T) {
	v := robustExprToStarlark(nil)
	s, ok := v.(*starlarkstruct.Struct)
	if !ok {
		t.Fatal("expected *starlarkstruct.Struct")
	}
	kind, err := s.Attr("kind")
	if err != nil || kind.String() != `"null"` {
		t.Errorf("expected kind=null, got %v %v", kind, err)
	}
}

func TestRobustExprToStarlark_BinExpr(t *testing.T) {
	node := &exprcheck.BinExpr{
		Op: "!=",
		L:  &exprcheck.VariableExpr{Name: "Status"},
		R:  &exprcheck.QNameExpr{Module: "Mod", Name: "Enum", Sub: "A"},
	}

	v := robustExprToStarlark(node)
	s, ok := v.(*starlarkstruct.Struct)
	if !ok {
		t.Fatal("expected *starlarkstruct.Struct")
	}

	kind, _ := s.Attr("kind")
	if kind.String() != `"bin"` {
		t.Errorf("expected kind=bin, got %v", kind)
	}

	op, _ := s.Attr("op")
	if op.String() != `"!="` {
		t.Errorf("expected op=!=, got %v", op)
	}

	left, err := s.Attr("left")
	if err != nil || left == nil {
		t.Errorf("expected left attr, got %v %v", left, err)
	}
}

func TestRobustExprToStarlark_UnaryExpr(t *testing.T) {
	node := &exprcheck.UnaryExpr{
		Op:      "NOT",
		Operand: &exprcheck.BoolLit{Value: true},
	}

	v := robustExprToStarlark(node)
	s := v.(*starlarkstruct.Struct)
	kind, _ := s.Attr("kind")
	if kind.String() != `"unary"` {
		t.Errorf("expected kind=unary, got %v", kind)
	}
	op, _ := s.Attr("op")
	if op.String() != `"NOT"` {
		t.Errorf("expected op=NOT, got %v", op)
	}
}

func TestRobustExprToStarlark_CallExpr(t *testing.T) {
	node := &exprcheck.CallExpr{
		Name: "not",
		Args: []exprcheck.RobustExpr{&exprcheck.BoolLit{Value: false}},
	}

	v := robustExprToStarlark(node)
	s := v.(*starlarkstruct.Struct)
	kind, _ := s.Attr("kind")
	if kind.String() != `"call"` {
		t.Errorf("expected kind=call, got %v", kind)
	}
	name, _ := s.Attr("name")
	if name.String() != `"not"` {
		t.Errorf("expected name=not, got %v", name)
	}
}

func TestStripXPathBrackets(t *testing.T) {
	cases := []struct{ in, want string }{
		{"[Status = 'Active']", "Status = 'Active'"},
		{"Status = 'Active'", "Status = 'Active'"},
		{"  [foo]  ", "foo"},
		{"", ""},
		// Chained predicates must not be partially stripped.
		{"[a = 1][b = 2]", "[a = 1][b = 2]"},
		{"[a = 1][b = 2][c = 3]", "[a = 1][b = 2][c = 3]"},
	}
	for _, c := range cases {
		got := stripXPathBrackets(c.in)
		if got != c.want {
			t.Errorf("stripXPathBrackets(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
