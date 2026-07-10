// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestEnumSplitParsesCasesAndElse(t *testing.T) {
	input := `CREATE MICROFLOW Orders.RouteStatus ($Status: enum Orders.Status)
RETURNS Boolean
BEGIN
  CASE $Status
    WHEN Open, Pending THEN
      RETURN true;
    WHEN (empty) THEN
      RETURN false;
    ELSE
      RETURN false;
  END CASE;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	split, ok := mf.Body[0].(*ast.EnumSplitStmt)
	if !ok {
		t.Fatalf("Expected EnumSplitStmt, got %T", mf.Body[0])
	}
	if split.Variable != "Status" {
		t.Fatalf("Variable = %q, want Status", split.Variable)
	}
	if len(split.Cases) != 2 {
		t.Fatalf("Expected two cases, got %d", len(split.Cases))
	}
	if got := split.Cases[0].Values; len(got) != 2 || got[0] != "Open" || got[1] != "Pending" {
		t.Fatalf("First case values = %v, want [Open Pending]", got)
	}
	if split.Cases[1].Value != "(empty)" {
		t.Fatalf("Second case value = %q, want (empty)", split.Cases[1].Value)
	}
	if len(split.ElseBody) != 1 {
		t.Fatalf("Expected one ELSE statement, got %d", len(split.ElseBody))
	}
}
