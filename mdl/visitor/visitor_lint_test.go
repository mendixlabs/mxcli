// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestLint_All(t *testing.T) {
	input := `LINT;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.LintStmt)
	if !ok {
		t.Fatalf("Expected LintStmt, got %T", prog.Statements[0])
	}
	if stmt.Target != nil {
		t.Error("Expected nil Target")
	}
	if stmt.ShowRules {
		t.Error("Expected ShowRules false")
	}
}

func TestLint_ModuleOnly(t *testing.T) {
	input := `LINT MyModule.*;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.LintStmt)
	if !ok {
		t.Fatalf("Expected LintStmt, got %T", prog.Statements[0])
	}
	if !stmt.ModuleOnly {
		t.Error("Expected ModuleOnly true")
	}
	if stmt.Target == nil {
		t.Fatal("Expected non-nil Target")
	}
	if stmt.Target.Name != "MyModule" {
		t.Errorf("Expected Target.Name %q, got %q", "MyModule", stmt.Target.Name)
	}
	if stmt.Target.Module != "" {
		t.Errorf("Expected Target.Module %q, got %q", "", stmt.Target.Module)
	}
}

func TestLint_WithFormat(t *testing.T) {
	input := `LINT * FORMAT JSON;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.LintStmt)
	if !ok {
		t.Fatalf("Expected LintStmt, got %T", prog.Statements[0])
	}
	if stmt.Format != "json" {
		t.Errorf("Expected json format, got %q", stmt.Format)
	}
}

func TestShowLintRules(t *testing.T) {
	input := `SHOW LINT RULES;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.LintStmt)
	if !ok {
		t.Fatalf("Expected LintStmt, got %T", prog.Statements[0])
	}
	if !stmt.ShowRules {
		t.Error("Expected ShowRules true")
	}
}
