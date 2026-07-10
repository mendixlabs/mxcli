// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestAddToListAcceptsExpressionValue(t *testing.T) {
	input := `CREATE MICROFLOW Sales.CollectLabels ($Order: Sales.Order)
RETURNS Boolean
BEGIN
  DECLARE $Labels List of String = empty;
  ADD $Order/Number TO $Labels;
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
	addStmt, ok := mf.Body[1].(*ast.AddToListStmt)
	if !ok {
		t.Fatalf("Expected AddToListStmt, got %T", mf.Body[1])
	}
	if addStmt.List != "Labels" {
		t.Fatalf("List = %q, want Labels", addStmt.List)
	}
	path, ok := addStmt.Value.(*ast.AttributePathExpr)
	if !ok {
		t.Fatalf("Value = %T, want AttributePathExpr", addStmt.Value)
	}
	if path.Variable != "Order" || len(path.Path) != 1 || path.Path[0] != "Number" {
		t.Fatalf("Value path = %#v, want $Order/Number", path)
	}
}

func TestAddToListKeepsSimpleVariableCompatibility(t *testing.T) {
	input := `CREATE MICROFLOW Sales.CollectOrders ($Order: Sales.Order)
RETURNS Boolean
BEGIN
  DECLARE $Orders List of Sales.Order = empty;
  ADD $Order TO $Orders;
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
	addStmt, ok := mf.Body[1].(*ast.AddToListStmt)
	if !ok {
		t.Fatalf("Expected AddToListStmt, got %T", mf.Body[1])
	}
	if addStmt.Item != "Order" {
		t.Fatalf("Item = %q, want Order", addStmt.Item)
	}
	varExpr, ok := addStmt.Value.(*ast.VariableExpr)
	if !ok {
		t.Fatalf("Value = %T, want VariableExpr", addStmt.Value)
	}
	if varExpr.Name != "Order" {
		t.Fatalf("Value.Name = %q, want Order", varExpr.Name)
	}
}
