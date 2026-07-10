// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// TestQualifiedCallInIfCondition covers the describe → check roundtrip case
// where `describe microflow --format mdl` emits a rule / sub-microflow call
// with a qualified name and named arguments as the IF condition. Before the
// grammar was widened to accept `qualifiedName` in `functionCall`, this form
// failed to parse with `mismatched input '(' expecting THEN`, blocking the
// roundtrip for microflows whose ExclusiveSplit uses a RuleSplitCondition.
func TestQualifiedCallInIfCondition(t *testing.T) {
	input := `CREATE OR MODIFY MICROFLOW SyntheticQualifiedCall.Test ($S: String) returns Boolean
BEGIN
  IF SyntheticRules.Strings.IsNotEmpty(String = $S) THEN
    RETURN true;
  ELSE
    RETURN false;
  END IF;
END`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse failed: %v", errs)
	}

	mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("expected CreateMicroflowStmt, got %T", prog.Statements[0])
	}
	if len(mf.Body) == 0 {
		t.Fatalf("empty microflow body")
	}
	ifStmt, ok := mf.Body[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("expected IfStmt, got %T", mf.Body[0])
	}

	call, ok := ifStmt.Condition.(*ast.FunctionCallExpr)
	if !ok {
		t.Fatalf("expected FunctionCallExpr as if-condition, got %T", ifStmt.Condition)
	}
	if call.Name != "SyntheticRules.Strings.IsNotEmpty" {
		t.Errorf("call name = %q, want %q", call.Name, "SyntheticRules.Strings.IsNotEmpty")
	}
	if len(call.Arguments) != 1 {
		t.Fatalf("expected 1 argument, got %d", len(call.Arguments))
	}

	// Named argument `String = $S` parses as an equality BinaryExpr: this is
	// how the describer already emits RuleSplitCondition parameter mappings
	// and how the re-parse preserves the textual form.
	bin, ok := call.Arguments[0].(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr argument, got %T", call.Arguments[0])
	}
	if bin.Operator != "=" {
		t.Errorf("operator = %q, want %q", bin.Operator, "=")
	}
	if id, ok := bin.Left.(*ast.IdentifierExpr); !ok || id.Name != "String" {
		t.Errorf("left = %+v, want IdentifierExpr{String}", bin.Left)
	}
	if v, ok := bin.Right.(*ast.VariableExpr); !ok || v.Name != "S" {
		t.Errorf("right = %+v, want VariableExpr{S}", bin.Right)
	}
}

// TestQualifiedCallPositionalArgs keeps the bare `Module.Func($arg1, $arg2)`
// form parseable (no named args) — common for sub-microflow calls that take
// a single entity / list argument and appear as the IF condition.
func TestQualifiedCallPositionalArgs(t *testing.T) {
	input := `CREATE OR MODIFY MICROFLOW M.Test ($L: list of M.Item) returns Boolean
BEGIN
  IF M.HasItems($L) THEN
    RETURN true;
  ELSE
    RETURN false;
  END IF;
END`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse failed: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	ifStmt := mf.Body[0].(*ast.IfStmt)
	call, ok := ifStmt.Condition.(*ast.FunctionCallExpr)
	if !ok {
		t.Fatalf("expected FunctionCallExpr, got %T", ifStmt.Condition)
	}
	if call.Name != "M.HasItems" {
		t.Errorf("call name = %q, want %q", call.Name, "M.HasItems")
	}
	if len(call.Arguments) != 1 {
		t.Fatalf("expected 1 argument, got %d", len(call.Arguments))
	}
	if v, ok := call.Arguments[0].(*ast.VariableExpr); !ok || v.Name != "L" {
		t.Errorf("arg = %+v, want VariableExpr{L}", call.Arguments[0])
	}
}
