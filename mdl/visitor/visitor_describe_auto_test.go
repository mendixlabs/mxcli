// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// TestDescribeAuto_BareName parses to DescribeAuto so the executor can detect the
// document type from the project. This is the form `describe Module.Name` (no type
// keyword) that the REPL/exec/check paths previously rejected with a parse error.
func TestDescribeAuto_BareName(t *testing.T) {
	prog, errs := Build("describe Administration.Account_New;")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.DescribeStmt)
	if !ok {
		t.Fatalf("expected *ast.DescribeStmt, got %T", prog.Statements[0])
	}
	if stmt.ObjectType != ast.DescribeAuto {
		t.Errorf("expected DescribeAuto, got %v", stmt.ObjectType)
	}
	if stmt.Name.Module != "Administration" || stmt.Name.Name != "Account_New" {
		t.Errorf("expected Administration.Account_New, got %s.%s", stmt.Name.Module, stmt.Name.Name)
	}
}

// TestDescribeAuto_TypedFormsWin guards the grammar ordering: every typed DESCRIBE
// form must still parse to its dedicated kind, not be swallowed by the bare
// auto-detect alternative (which is declared last for exactly this reason).
func TestDescribeAuto_TypedFormsWin(t *testing.T) {
	cases := []struct {
		input string
		want  ast.DescribeObjectType
	}{
		{"describe page Administration.Account_New;", ast.DescribePage},
		{"describe entity Administration.Account;", ast.DescribeEntity},
		{"describe microflow Administration.Foo;", ast.DescribeMicroflow},
		{"describe settings;", ast.DescribeSettings},
		{"describe module Administration;", ast.DescribeModule},
		{"describe agent MyMod.MyAgent;", ast.DescribeAgent},
		{"describe data transformer MyMod.T;", ast.DescribeDataTransformer},
	}
	for _, tc := range cases {
		prog, errs := Build(tc.input)
		if len(errs) > 0 {
			t.Errorf("%q: unexpected parse errors: %v", tc.input, errs)
			continue
		}
		if len(prog.Statements) != 1 {
			t.Errorf("%q: expected 1 statement, got %d", tc.input, len(prog.Statements))
			continue
		}
		stmt, ok := prog.Statements[0].(*ast.DescribeStmt)
		if !ok {
			t.Errorf("%q: expected *ast.DescribeStmt, got %T", tc.input, prog.Statements[0])
			continue
		}
		if stmt.ObjectType != tc.want {
			t.Errorf("%q: expected %v, got %v", tc.input, tc.want, stmt.ObjectType)
		}
	}
}
