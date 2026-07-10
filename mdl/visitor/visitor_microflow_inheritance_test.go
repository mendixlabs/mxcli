// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestMicroflowParsing_InheritanceSplitAndCastAction(t *testing.T) {
	input := `CREATE MICROFLOW Sample.Route ($Input: Sample.BaseInput)
RETURNS Boolean
BEGIN
  SPLIT TYPE $Input
  CASE Sample.SpecializedInput
    CAST $SpecificInput;
    RETURN true;
  ELSE
    RETURN false;
  END SPLIT;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	split, ok := mf.Body[0].(*ast.InheritanceSplitStmt)
	if !ok {
		t.Fatalf("first body statement: got %T, want *ast.InheritanceSplitStmt", mf.Body[0])
	}
	if split.Variable != "Input" {
		t.Fatalf("split variable = %q, want Input", split.Variable)
	}
	if len(split.Cases) != 1 || split.Cases[0].Entity.String() != "Sample.SpecializedInput" {
		t.Fatalf("split cases = %#v, want Sample.SpecializedInput", split.Cases)
	}
	cast, ok := split.Cases[0].Body[0].(*ast.CastObjectStmt)
	if !ok {
		t.Fatalf("case body[0]: got %T, want *ast.CastObjectStmt", split.Cases[0].Body[0])
	}
	if cast.OutputVariable != "SpecificInput" || cast.ObjectVariable != "" {
		t.Fatalf("cast vars: got output=%q object=%q", cast.OutputVariable, cast.ObjectVariable)
	}
	if len(split.ElseBody) != 1 {
		t.Fatalf("else body length = %d, want 1", len(split.ElseBody))
	}
}

func TestMicroflowParsing_CastWithSourceVariable(t *testing.T) {
	input := `CREATE MICROFLOW Sample.Cast ($Input: Sample.BaseInput)
BEGIN
  $SpecificInput = CAST $Input;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	cast, ok := mf.Body[0].(*ast.CastObjectStmt)
	if !ok {
		t.Fatalf("body[0]: got %T, want *ast.CastObjectStmt", mf.Body[0])
	}
	if cast.OutputVariable != "SpecificInput" || cast.ObjectVariable != "Input" {
		t.Fatalf("cast vars: got output=%q object=%q", cast.OutputVariable, cast.ObjectVariable)
	}
}
