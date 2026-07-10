// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestChangeObjectParsesRefreshModifier(t *testing.T) {
	input := `CREATE MICROFLOW Sales.UpdateCustomer ($Customer: Sales.Customer)
RETURNS Boolean
BEGIN
  CHANGE $Customer (Name = 'Jane') REFRESH;
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	changeStmt, ok := mf.Body[0].(*ast.ChangeObjectStmt)
	if !ok {
		t.Fatalf("Expected ChangeObjectStmt, got %T", mf.Body[0])
	}
	if !changeStmt.RefreshInClient {
		t.Fatal("Expected refresh modifier to set RefreshInClient")
	}
	if len(changeStmt.Changes) != 1 {
		t.Fatalf("Expected one change, got %d", len(changeStmt.Changes))
	}
}

func TestChangeObjectParsesRefreshModifierWithoutMembers(t *testing.T) {
	input := `CREATE MICROFLOW Sales.RefreshCustomer ($Customer: Sales.Customer)
RETURNS Boolean
BEGIN
  CHANGE $Customer REFRESH;
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	changeStmt, ok := mf.Body[0].(*ast.ChangeObjectStmt)
	if !ok {
		t.Fatalf("Expected ChangeObjectStmt, got %T", mf.Body[0])
	}
	if !changeStmt.RefreshInClient {
		t.Fatal("Expected refresh modifier to set RefreshInClient")
	}
}
