// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestMicroflowParsing(t *testing.T) {
	input := `CREATE MICROFLOW MyModule.HelloWorld ()
RETURNS String
BEGIN
  DECLARE $greeting String = 'Hello, World!'
  RETURN $greeting
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("Expected CreateMicroflowStmt, got %T", prog.Statements[0])
	}

	t.Logf("Microflow name: %s.%s", stmt.Name.Module, stmt.Name.Name)
	t.Logf("Body statements: %d", len(stmt.Body))
	for i, s := range stmt.Body {
		t.Logf("  Statement %d: %T", i, s)
	}

	if len(stmt.Body) != 2 {
		t.Errorf("Expected 2 body statements (DECLARE and RETURN), got %d", len(stmt.Body))
	}
}

func TestErrorMessageParsing(t *testing.T) {
	input := `CREATE PERSISTENT ENTITY DmTest.Cars (
		CarId: String NOT NULL ERROR 'Car ID is required',
		Name: String(100) UNIQUE ERROR 'Name must be unique'
	);`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	// Check that the statement has the correct error messages
	stmt := prog.Statements[0]
	t.Logf("Statement type: %T", stmt)
	t.Logf("Statement: %+v", stmt)
}

func TestRequiredErrorMessage(t *testing.T) {
	input := `CREATE PERSISTENT ENTITY DmTest.Cars (
		CarId: String REQUIRED ERROR 'Car ID is required'
	);`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	t.Logf("Statement: %+v", prog.Statements[0])
}

func TestPositionAnnotation(t *testing.T) {
	input := `@Position(150, 210)
CREATE PERSISTENT ENTITY DmTest.Cars (
	CarId: String NOT NULL
);`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt := prog.Statements[0].(*ast.CreateEntityStmt)
	if stmt.Position == nil {
		t.Fatal("Expected position to be set")
	}
	if stmt.Position.X != 150 || stmt.Position.Y != 210 {
		t.Errorf("Expected position (150, 210), got (%d, %d)", stmt.Position.X, stmt.Position.Y)
	}
	t.Logf("Position: (%d, %d)", stmt.Position.X, stmt.Position.Y)
}

func TestUnquoteString_DecodesEscapes(t *testing.T) {
	input := `'Line 1\nLine 2\tTabbed\\Path''s'`
	got := unquoteString(input)
	want := "Line 1\nLine 2\tTabbed\\Path's"
	if got != want {
		t.Fatalf("unquoteString(%q) = %q, want %q", input, got, want)
	}
}

func TestIndexParsing(t *testing.T) {
	input := `CREATE PERSISTENT ENTITY DmTest.Cars (
	CarId: String NOT NULL,
	Brand: String
)
INDEX (CarId);`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt := prog.Statements[0].(*ast.CreateEntityStmt)
	if len(stmt.Indexes) != 1 {
		t.Fatalf("Expected 1 index, got %d", len(stmt.Indexes))
	}
	t.Logf("Index columns: %+v", stmt.Indexes[0].Columns)
}

func TestDocCommentWithMultilineStatement(t *testing.T) {
	input := `/**
 * Test entity for cars
 */
CREATE PERSISTENT ENTITY DmTest.Cars (
	/** The car identifier */
	CarId: String NOT NULL,
	Name: String(100) UNIQUE
);`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt := prog.Statements[0].(*ast.CreateEntityStmt)
	t.Logf("Full statement: %+v", stmt)

	// Check entity documentation
	if stmt.Documentation == "" {
		t.Errorf("Expected entity documentation to be set, got empty string")
	} else {
		t.Logf("Entity documentation: %s", stmt.Documentation)
	}

	// Check attribute documentation
	if len(stmt.Attributes) > 0 && stmt.Attributes[0].Documentation == "" {
		t.Error("Expected attribute documentation to be set")
	} else if len(stmt.Attributes) > 0 {
		t.Logf("Attribute documentation: %s", stmt.Attributes[0].Documentation)
	}
}

// --- CALL Statement Parameter Tests ---

func TestCallUnifiedParamSyntax(t *testing.T) {
	// Test CALL with parameter names without $ prefix
	input := `CREATE MICROFLOW TestModule.CallTest () RETURNS String
	BEGIN
		$Result = CALL MICROFLOW TestModule.OtherMf (Input = 'Hello', Quantity = 5);
		RETURN $Result;
	END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("Expected CreateMicroflowStmt, got %T", prog.Statements[0])
	}

	// Verify the microflow has a CALL statement in its body
	if len(stmt.Body) < 2 {
		t.Fatalf("Expected at least 2 body statements, got %d", len(stmt.Body))
	}

	// Check the first statement is a CallMicroflowStmt (with result variable)
	callStmt, ok := stmt.Body[0].(*ast.CallMicroflowStmt)
	if !ok {
		t.Fatalf("Expected CallMicroflowStmt, got %T", stmt.Body[0])
	}

	if len(callStmt.Arguments) != 2 {
		t.Errorf("Expected 2 arguments, got %d", len(callStmt.Arguments))
	}

	// Verify argument names (without $ prefix)
	for _, arg := range callStmt.Arguments {
		t.Logf("Argument: %s = %v", arg.Name, arg.Value)
	}

	t.Log("CALL with unified parameter syntax (no $ prefix) parsed successfully")
}

func TestCallWithDollarPrefix_BackwardCompat(t *testing.T) {
	// Test CALL with parameter names with $ prefix (old syntax, should still work)
	input := `CREATE MICROFLOW TestModule.CallTestOld () RETURNS String
	BEGIN
		$Result = CALL MICROFLOW TestModule.OtherMf ($Input = 'Hello');
		RETURN $Result;
	END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	_, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("Expected CreateMicroflowStmt, got %T", prog.Statements[0])
	}

	t.Log("CALL with $prefix parameter syntax (backward compat) parsed successfully")
}

// --- LOG Template Tests ---

func TestLogWithTemplateSyntax(t *testing.T) {
	// Test LOG with WITH template syntax
	input := `CREATE MICROFLOW TestModule.LogTest ($OrderNumber: String) RETURNS Boolean
	BEGIN
		LOG INFO NODE 'OrderService' 'Processing order: {1}' WITH ({1} = $OrderNumber);
		RETURN true;
	END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("Expected CreateMicroflowStmt, got %T", prog.Statements[0])
	}

	// Verify the microflow has a LOG statement in its body
	if len(stmt.Body) < 2 {
		t.Fatalf("Expected at least 2 body statements, got %d", len(stmt.Body))
	}

	// Check the first statement is a LogStmt
	logStmt, ok := stmt.Body[0].(*ast.LogStmt)
	if !ok {
		t.Fatalf("Expected LogStmt, got %T", stmt.Body[0])
	}

	if logStmt.Level != ast.LogInfo {
		t.Errorf("Expected log level INFO, got %s", logStmt.Level)
	}

	nodeLit, ok := logStmt.Node.(*ast.LiteralExpr)
	if !ok || nodeLit.Kind != ast.LiteralString || nodeLit.Value != "OrderService" {
		t.Fatalf("Expected node string literal 'OrderService', got %#v", logStmt.Node)
	}

	if len(logStmt.Template) != 1 {
		t.Errorf("Expected 1 template parameter, got %d", len(logStmt.Template))
	}

	if len(logStmt.Template) > 0 {
		t.Logf("Template param: {%d} = %v", logStmt.Template[0].Index, logStmt.Template[0].Value)
	}

	t.Log("LOG with WITH template syntax parsed successfully")
}

func TestLogWithMultipleParams(t *testing.T) {
	// Test LOG with multiple template parameters
	input := `CREATE MICROFLOW TestModule.LogMultiTest ($OrderNum: String, $Customer: String, $Total: Decimal) RETURNS Boolean
	BEGIN
		LOG INFO NODE 'OrderService' 'Order {1} for {2} totaling {3}' WITH ({1} = $OrderNum, {2} = $Customer, {3} = toString($Total));
		RETURN true;
	END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("Expected CreateMicroflowStmt, got %T", prog.Statements[0])
	}

	// Check the first statement is a LogStmt
	logStmt, ok := stmt.Body[0].(*ast.LogStmt)
	if !ok {
		t.Fatalf("Expected LogStmt, got %T", stmt.Body[0])
	}

	if len(logStmt.Template) != 3 {
		t.Errorf("Expected 3 template parameters, got %d", len(logStmt.Template))
	}

	for _, param := range logStmt.Template {
		t.Logf("Template param: {%d} = %v", param.Index, param.Value)
	}

	t.Log("LOG with multiple template params parsed successfully")
}

func TestLogWithNodeExpressionConstant(t *testing.T) {
	input := `CREATE MICROFLOW TestModule.LogNodeExprTest () RETURNS Boolean
	BEGIN
		LOG INFO NODE @TestModule.SecurityLogNode 'User added';
		RETURN true;
	END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("Expected CreateMicroflowStmt, got %T", prog.Statements[0])
	}

	logStmt, ok := stmt.Body[0].(*ast.LogStmt)
	if !ok {
		t.Fatalf("Expected LogStmt, got %T", stmt.Body[0])
	}

	node, ok := logStmt.Node.(*ast.ConstantRefExpr)
	if !ok {
		t.Fatalf("Expected ConstantRefExpr for node, got %T", logStmt.Node)
	}
	if node.QualifiedName.Module != "TestModule" || node.QualifiedName.Name != "SecurityLogNode" {
		t.Fatalf("Unexpected node constant: %s.%s", node.QualifiedName.Module, node.QualifiedName.Name)
	}
}

// =============================================================================
// Bug Report Tests - Verify correct behavior for reported issues
// =============================================================================

// TestIfThenWithMultipleActions verifies that actions in IF/THEN blocks
// are correctly placed in the ThenBody, not the ElseBody.
// Bug Report: "IF/THEN Block Inversion"
func TestIfThenWithMultipleActions(t *testing.T) {
	input := `CREATE MICROFLOW Test.VAL_Test ($Product: Test.Product)
RETURNS Boolean AS $IsValid
BEGIN
  DECLARE $IsValid Boolean = true;

  IF $Product/Name = '' THEN
    LOG INFO 'Name is empty';
    SET $IsValid = false;
  END IF;

  RETURN $IsValid;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	// Find the IF statement (should be second in body, after DECLARE)
	var ifStmt *ast.IfStmt
	for _, s := range stmt.Body {
		if ifs, ok := s.(*ast.IfStmt); ok {
			ifStmt = ifs
			break
		}
	}

	if ifStmt == nil {
		t.Fatal("Expected to find IF statement in microflow body")
	}

	// Verify actions are in THEN body, not ELSE body
	if len(ifStmt.ThenBody) != 2 {
		t.Errorf("Expected 2 statements in THEN body, got %d", len(ifStmt.ThenBody))
	}
	if len(ifStmt.ElseBody) != 0 {
		t.Errorf("Expected 0 statements in ELSE body, got %d", len(ifStmt.ElseBody))
	}

	// Verify the first action is LOG
	if _, ok := ifStmt.ThenBody[0].(*ast.LogStmt); !ok {
		t.Errorf("Expected first THEN statement to be LogStmt, got %T", ifStmt.ThenBody[0])
	}

	// Verify the second action is SET
	if _, ok := ifStmt.ThenBody[1].(*ast.MfSetStmt); !ok {
		t.Errorf("Expected second THEN statement to be MfSetStmt, got %T", ifStmt.ThenBody[1])
	}

	t.Log("IF/THEN with multiple actions parsed correctly - actions in THEN body")
}

// TestIfThenElse verifies that IF/THEN/ELSE places actions in correct branches.
func TestIfThenElse(t *testing.T) {
	input := `CREATE MICROFLOW Test.TestIfElse ($Value: Integer)
RETURNS String AS $Result
BEGIN
  DECLARE $Result String = '';

  IF $Value > 100 THEN
    SET $Result = 'High';
  ELSE
    SET $Result = 'Low';
  END IF;

  RETURN $Result;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	var ifStmt *ast.IfStmt
	for _, s := range stmt.Body {
		if ifs, ok := s.(*ast.IfStmt); ok {
			ifStmt = ifs
			break
		}
	}

	if ifStmt == nil {
		t.Fatal("Expected to find IF statement")
	}

	// Verify THEN has 1 action
	if len(ifStmt.ThenBody) != 1 {
		t.Errorf("Expected 1 statement in THEN body, got %d", len(ifStmt.ThenBody))
	}

	// Verify ELSE has 1 action
	if len(ifStmt.ElseBody) != 1 {
		t.Errorf("Expected 1 statement in ELSE body, got %d", len(ifStmt.ElseBody))
	}

	// Verify THEN action sets 'High'
	thenSet, ok := ifStmt.ThenBody[0].(*ast.MfSetStmt)
	if !ok {
		t.Fatalf("Expected MfSetStmt in THEN, got %T", ifStmt.ThenBody[0])
	}
	if lit, ok := thenSet.Value.(*ast.LiteralExpr); ok && lit.Kind == ast.LiteralString {
		if lit.Value != "High" {
			t.Errorf("Expected THEN to set 'High', got '%v'", lit.Value)
		}
	}

	// Verify ELSE action sets 'Low'
	elseSet, ok := ifStmt.ElseBody[0].(*ast.MfSetStmt)
	if !ok {
		t.Fatalf("Expected MfSetStmt in ELSE, got %T", ifStmt.ElseBody[0])
	}
	if lit, ok := elseSet.Value.(*ast.LiteralExpr); ok && lit.Kind == ast.LiteralString {
		if lit.Value != "Low" {
			t.Errorf("Expected ELSE to set 'Low', got '%v'", lit.Value)
		}
	}

	t.Log("IF/THEN/ELSE parsed correctly - actions in correct branches")
}

func TestIfThenEmptyElsePreservesElsePresence(t *testing.T) {
	input := `create microflow Test.EmptyElse()
returns String
begin
  if $latestHttpResponse != empty then
    return 'error';
  else
  end if;
  return empty;
end;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if len(stmt.Body) == 0 {
		t.Fatal("expected microflow body")
	}
	ifStmt, ok := stmt.Body[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("expected first statement to be IfStmt, got %T", stmt.Body[0])
	}
	if !ifStmt.HasElse {
		t.Fatal("expected explicit empty else to be preserved")
	}
	if len(ifStmt.ElseBody) != 0 {
		t.Fatalf("empty else body length = %d, want 0", len(ifStmt.ElseBody))
	}
}

// TestValidationFeedbackInsideIf verifies VALIDATION FEEDBACK works inside IF blocks.
// Bug Report: "VALIDATION FEEDBACK Not Recognized"
func TestValidationFeedbackInsideIf(t *testing.T) {
	input := `CREATE MICROFLOW Test.VAL_Product ($Product: Test.Product)
RETURNS Boolean AS $IsValid
BEGIN
  DECLARE $IsValid Boolean = true;

  IF $Product/Name = '' THEN
    SET $IsValid = false;
    VALIDATION FEEDBACK $Product/Name MESSAGE 'Name is required';
  END IF;

  RETURN $IsValid;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	var ifStmt *ast.IfStmt
	for _, s := range stmt.Body {
		if ifs, ok := s.(*ast.IfStmt); ok {
			ifStmt = ifs
			break
		}
	}

	if ifStmt == nil {
		t.Fatal("Expected to find IF statement")
	}

	// Verify THEN has 2 actions
	if len(ifStmt.ThenBody) != 2 {
		t.Errorf("Expected 2 statements in THEN body, got %d", len(ifStmt.ThenBody))
	}

	// Verify the second action is VALIDATION FEEDBACK
	valFeedback, ok := ifStmt.ThenBody[1].(*ast.ValidationFeedbackStmt)
	if !ok {
		t.Fatalf("Expected ValidationFeedbackStmt, got %T", ifStmt.ThenBody[1])
	}

	// Verify the attribute path
	if valFeedback.AttributePath == nil {
		t.Fatal("Expected AttributePath to be set")
	}
	if valFeedback.AttributePath.Variable != "Product" {
		t.Errorf("Expected variable 'Product', got '%s'", valFeedback.AttributePath.Variable)
	}
	if len(valFeedback.AttributePath.Path) == 0 || valFeedback.AttributePath.Path[0] != "Name" {
		t.Errorf("Expected attribute 'Name', got '%v'", valFeedback.AttributePath.Path)
	}

	t.Log("VALIDATION FEEDBACK inside IF block parsed correctly")
}

func TestValidationFeedbackObjectAndAssociationTargets(t *testing.T) {
	input := `CREATE MICROFLOW Sales.ValidateOrder ($OrderForm: Sales.OrderForm)
RETURNS Boolean
BEGIN
  VALIDATION FEEDBACK $OrderForm MESSAGE 'Select a value';
  VALIDATION FEEDBACK $OrderForm/Sales.OrderForm_Customer MESSAGE 'Select a customer';
  RETURN false;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if len(stmt.Body) < 2 {
		t.Fatalf("Expected at least two body statements, got %d", len(stmt.Body))
	}

	objectFeedback, ok := stmt.Body[0].(*ast.ValidationFeedbackStmt)
	if !ok {
		t.Fatalf("Expected object-only validation feedback, got %T", stmt.Body[0])
	}
	if objectFeedback.AttributePath == nil {
		t.Fatal("Expected object-only AttributePath to be set")
	}
	if objectFeedback.AttributePath.Variable != "OrderForm" {
		t.Errorf("Expected object-only variable 'OrderForm', got %q", objectFeedback.AttributePath.Variable)
	}
	if len(objectFeedback.AttributePath.Path) != 0 || len(objectFeedback.AttributePath.Segments) != 0 {
		t.Errorf("Expected object-only validation feedback to have no path, got path=%v segments=%v",
			objectFeedback.AttributePath.Path, objectFeedback.AttributePath.Segments)
	}

	associationFeedback, ok := stmt.Body[1].(*ast.ValidationFeedbackStmt)
	if !ok {
		t.Fatalf("Expected association validation feedback, got %T", stmt.Body[1])
	}
	if associationFeedback.AttributePath == nil {
		t.Fatal("Expected association AttributePath to be set")
	}
	if associationFeedback.AttributePath.Variable != "OrderForm" {
		t.Errorf("Expected association variable 'OrderForm', got %q", associationFeedback.AttributePath.Variable)
	}
	if len(associationFeedback.AttributePath.Path) != 1 || associationFeedback.AttributePath.Path[0] != "Sales.OrderForm_Customer" {
		t.Errorf("Expected association path Sales.OrderForm_Customer, got %v", associationFeedback.AttributePath.Path)
	}
	if len(associationFeedback.AttributePath.Segments) != 1 ||
		associationFeedback.AttributePath.Segments[0].Separator != "/" ||
		associationFeedback.AttributePath.Segments[0].Name != "Sales.OrderForm_Customer" {
		t.Errorf("Expected slash-qualified association segment, got %v", associationFeedback.AttributePath.Segments)
	}
}

func TestSharedAttributePathKeepsQualifiedSlashSegment(t *testing.T) {
	input := `CREATE MICROFLOW Sales.UpdateOrder ($Order: Sales.Order)
RETURNS Boolean
BEGIN
  SET $Order/Sales.Order_Customer = empty;
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)
	setStmt, ok := stmt.Body[0].(*ast.MfSetStmt)
	if !ok {
		t.Fatalf("Expected MfSetStmt, got %T", stmt.Body[0])
	}
	if setStmt.Target != "$Order/Sales.Order_Customer" {
		t.Fatalf("Target = %q, want $Order/Sales.Order_Customer", setStmt.Target)
	}
}

// TestRollbackStatement verifies the ROLLBACK statement parses correctly.
func TestRollbackStatement(t *testing.T) {
	input := `CREATE MICROFLOW Test.TestRollback ($Order: Test.Order)
RETURNS Boolean AS $Success
BEGIN
  CHANGE $Order (Status = 'Modified');
  ROLLBACK $Order;
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	// Find the ROLLBACK statement (should be second, after CHANGE)
	var rollbackStmt *ast.RollbackStmt
	for _, s := range stmt.Body {
		if rs, ok := s.(*ast.RollbackStmt); ok {
			rollbackStmt = rs
			break
		}
	}

	if rollbackStmt == nil {
		t.Fatal("Expected to find ROLLBACK statement")
	}

	if rollbackStmt.Variable != "Order" {
		t.Errorf("Expected variable 'Order', got '%s'", rollbackStmt.Variable)
	}
	if rollbackStmt.RefreshInClient {
		t.Error("Expected RefreshInClient to be false")
	}

	t.Log("ROLLBACK statement parsed correctly")
}

// TestRollbackWithRefresh verifies ROLLBACK REFRESH parses correctly.
func TestRollbackWithRefresh(t *testing.T) {
	input := `CREATE MICROFLOW Test.TestRollback ($Order: Test.Order)
RETURNS Boolean AS $Success
BEGIN
  ROLLBACK $Order REFRESH;
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	var rollbackStmt *ast.RollbackStmt
	for _, s := range stmt.Body {
		if rs, ok := s.(*ast.RollbackStmt); ok {
			rollbackStmt = rs
			break
		}
	}

	if rollbackStmt == nil {
		t.Fatal("Expected to find ROLLBACK statement")
	}

	if rollbackStmt.Variable != "Order" {
		t.Errorf("Expected variable 'Order', got '%s'", rollbackStmt.Variable)
	}
	if !rollbackStmt.RefreshInClient {
		t.Error("Expected RefreshInClient to be true")
	}

	t.Log("ROLLBACK REFRESH statement parsed correctly")
}

// TestCommitWithRefresh verifies COMMIT REFRESH parses correctly.
func TestCommitWithRefresh(t *testing.T) {
	input := `CREATE MICROFLOW Test.TestCommit ($Order: Test.Order)
RETURNS Boolean AS $Success
BEGIN
  COMMIT $Order REFRESH;
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	var commitStmt *ast.MfCommitStmt
	for _, s := range stmt.Body {
		if cs, ok := s.(*ast.MfCommitStmt); ok {
			commitStmt = cs
			break
		}
	}

	if commitStmt == nil {
		t.Fatal("Expected to find COMMIT statement")
	}

	if commitStmt.Variable != "Order" {
		t.Errorf("Expected variable 'Order', got '%s'", commitStmt.Variable)
	}
	if !commitStmt.RefreshInClient {
		t.Error("Expected RefreshInClient to be true")
	}
	if commitStmt.WithEvents {
		t.Error("Expected WithEvents to be false")
	}
}

// TestCommitWithEventsAndRefresh verifies COMMIT WITH EVENTS REFRESH parses correctly.
func TestCommitWithEventsAndRefresh(t *testing.T) {
	input := `CREATE MICROFLOW Test.TestCommit ($Order: Test.Order)
RETURNS Boolean AS $Success
BEGIN
  COMMIT $Order WITH EVENTS REFRESH;
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	var commitStmt *ast.MfCommitStmt
	for _, s := range stmt.Body {
		if cs, ok := s.(*ast.MfCommitStmt); ok {
			commitStmt = cs
			break
		}
	}

	if commitStmt == nil {
		t.Fatal("Expected to find COMMIT statement")
	}

	if commitStmt.Variable != "Order" {
		t.Errorf("Expected variable 'Order', got '%s'", commitStmt.Variable)
	}
	if !commitStmt.RefreshInClient {
		t.Error("Expected RefreshInClient to be true")
	}
	if !commitStmt.WithEvents {
		t.Error("Expected WithEvents to be true")
	}
}

// TestNestedIfStatements verifies nested IF statements parse correctly.
func TestNestedIfStatements(t *testing.T) {
	input := `CREATE MICROFLOW Test.TestNested ($Value: Integer)
RETURNS String AS $Result
BEGIN
  DECLARE $Result String = '';

  IF $Value > 0 THEN
    IF $Value > 100 THEN
      SET $Result = 'Large positive';
    ELSE
      SET $Result = 'Small positive';
    END IF;
  ELSE
    SET $Result = 'Non-positive';
  END IF;

  RETURN $Result;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	// Find outer IF
	var outerIf *ast.IfStmt
	for _, s := range stmt.Body {
		if ifs, ok := s.(*ast.IfStmt); ok {
			outerIf = ifs
			break
		}
	}

	if outerIf == nil {
		t.Fatal("Expected to find outer IF statement")
	}

	// Outer THEN should contain nested IF
	if len(outerIf.ThenBody) != 1 {
		t.Fatalf("Expected 1 statement in outer THEN, got %d", len(outerIf.ThenBody))
	}

	innerIf, ok := outerIf.ThenBody[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("Expected nested IfStmt in outer THEN, got %T", outerIf.ThenBody[0])
	}

	// Inner IF should have THEN and ELSE
	if len(innerIf.ThenBody) != 1 {
		t.Errorf("Expected 1 statement in inner THEN, got %d", len(innerIf.ThenBody))
	}
	if len(innerIf.ElseBody) != 1 {
		t.Errorf("Expected 1 statement in inner ELSE, got %d", len(innerIf.ElseBody))
	}

	// Outer ELSE should have 1 action
	if len(outerIf.ElseBody) != 1 {
		t.Errorf("Expected 1 statement in outer ELSE, got %d", len(outerIf.ElseBody))
	}

	t.Log("Nested IF statements parsed correctly")
}

// TestRetrieveWithLimit verifies RETRIEVE with LIMIT parses correctly.
func TestRetrieveWithLimit(t *testing.T) {
	input := `CREATE MICROFLOW Test.TestRetrieve ()
RETURNS Boolean AS $Success
BEGIN
  DECLARE $Product Test.Product;
  RETRIEVE $Product FROM Test.Product WHERE IsActive = true LIMIT 1;
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	var retrieveStmt *ast.RetrieveStmt
	for _, s := range stmt.Body {
		if rs, ok := s.(*ast.RetrieveStmt); ok {
			retrieveStmt = rs
			break
		}
	}

	if retrieveStmt == nil {
		t.Fatal("Expected to find RETRIEVE statement")
	}

	if retrieveStmt.Limit != "1" {
		t.Errorf("Expected Limit '1', got %q", retrieveStmt.Limit)
	}

	t.Log("RETRIEVE with LIMIT parsed correctly")
}

// TestDeclareEntityWithoutAS verifies entity declaration without AS keyword.
func TestDeclareEntityWithoutAS(t *testing.T) {
	input := `CREATE MICROFLOW Test.TestDeclare ()
RETURNS Boolean AS $Success
BEGIN
  DECLARE $Product Test.Product;
  DECLARE $List List of Test.Product = empty;
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	// Should have 3 statements: 2 DECLARE and 1 RETURN
	if len(stmt.Body) != 3 {
		t.Errorf("Expected 3 body statements, got %d", len(stmt.Body))
	}

	// First DECLARE should be entity type (at AST level, bare qualified names parse as enumerations
	// since they're syntactically indistinguishable - semantic analysis determines actual type)
	decl1, ok := stmt.Body[0].(*ast.DeclareStmt)
	if !ok {
		t.Fatalf("Expected DeclareStmt, got %T", stmt.Body[0])
	}
	if decl1.Variable != "Product" {
		t.Errorf("Expected variable 'Product', got '%s'", decl1.Variable)
	}
	// Bare qualified name parses as TypeEnumeration with EnumRef (since entity and enum look the same)
	if decl1.Type.EnumRef == nil {
		t.Error("Expected qualified name reference in EnumRef")
	}
	if decl1.Type.EnumRef != nil && decl1.Type.EnumRef.String() != "Test.Product" {
		t.Errorf("Expected 'Test.Product', got '%s'", decl1.Type.EnumRef.String())
	}

	// Second DECLARE should be list type
	decl2, ok := stmt.Body[1].(*ast.DeclareStmt)
	if !ok {
		t.Fatalf("Expected DeclareStmt, got %T", stmt.Body[1])
	}
	if decl2.Variable != "List" {
		t.Errorf("Expected variable 'List', got '%s'", decl2.Variable)
	}
	if decl2.Type.Kind != ast.TypeListOf {
		t.Error("Expected list type")
	}

	t.Log("DECLARE without AS keyword parsed correctly")
}

// TestAlterEntityAddAttribute verifies ALTER ENTITY ADD ATTRIBUTE produces correct AST.
func TestAlterEntityAddAttribute(t *testing.T) {
	input := `ALTER ENTITY MyModule.Customer
  ADD ATTRIBUTE Email: String(200) NOT NULL
  ADD ATTRIBUTE Age: Integer;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 2 {
		t.Fatalf("Expected 2 statements (one per action), got %d", len(prog.Statements))
	}

	// First: ADD ATTRIBUTE Email
	stmt1, ok := prog.Statements[0].(*ast.AlterEntityStmt)
	if !ok {
		t.Fatalf("Expected AlterEntityStmt, got %T", prog.Statements[0])
	}
	if stmt1.Name.Module != "MyModule" || stmt1.Name.Name != "Customer" {
		t.Errorf("Expected MyModule.Customer, got %s", stmt1.Name)
	}
	if stmt1.Operation != ast.AlterEntityAddAttribute {
		t.Errorf("Expected AlterEntityAddAttribute, got %d", stmt1.Operation)
	}
	if stmt1.Attribute == nil {
		t.Fatal("Expected attribute, got nil")
	}
	if stmt1.Attribute.Name != "Email" {
		t.Errorf("Expected Email, got %s", stmt1.Attribute.Name)
	}
	if stmt1.Attribute.Type.Kind != ast.TypeString {
		t.Errorf("Expected String type, got %d", stmt1.Attribute.Type.Kind)
	}
	if !stmt1.Attribute.NotNull {
		t.Error("Expected NOT NULL constraint")
	}

	// Second: ADD ATTRIBUTE Age
	stmt2, ok := prog.Statements[1].(*ast.AlterEntityStmt)
	if !ok {
		t.Fatalf("Expected AlterEntityStmt, got %T", prog.Statements[1])
	}
	if stmt2.Attribute == nil || stmt2.Attribute.Name != "Age" {
		t.Errorf("Expected Age attribute")
	}
	if stmt2.Attribute.Type.Kind != ast.TypeInteger {
		t.Errorf("Expected Integer type, got %d", stmt2.Attribute.Type.Kind)
	}

	t.Log("ALTER ENTITY ADD ATTRIBUTE parsed correctly")
}

// TestAlterEntityDropRenameAttribute verifies DROP and RENAME operations.
func TestAlterEntityDropRenameAttribute(t *testing.T) {
	input := `ALTER ENTITY MyModule.Customer
  RENAME ATTRIBUTE OldName TO NewName
  DROP ATTRIBUTE Obsolete;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 2 {
		t.Fatalf("Expected 2 statements, got %d", len(prog.Statements))
	}

	// RENAME
	stmt1, ok := prog.Statements[0].(*ast.AlterEntityStmt)
	if !ok {
		t.Fatalf("Expected AlterEntityStmt, got %T", prog.Statements[0])
	}
	if stmt1.Operation != ast.AlterEntityRenameAttribute {
		t.Errorf("Expected RenameAttribute, got %d", stmt1.Operation)
	}
	if stmt1.AttributeName != "OldName" || stmt1.NewName != "NewName" {
		t.Errorf("Expected OldName -> NewName, got %s -> %s", stmt1.AttributeName, stmt1.NewName)
	}

	// DROP
	stmt2, ok := prog.Statements[1].(*ast.AlterEntityStmt)
	if !ok {
		t.Fatalf("Expected AlterEntityStmt, got %T", prog.Statements[1])
	}
	if stmt2.Operation != ast.AlterEntityDropAttribute {
		t.Errorf("Expected DropAttribute, got %d", stmt2.Operation)
	}
	if stmt2.AttributeName != "Obsolete" {
		t.Errorf("Expected Obsolete, got %s", stmt2.AttributeName)
	}

	t.Log("ALTER ENTITY DROP/RENAME ATTRIBUTE parsed correctly")
}

func TestEnhanceErrorMessage_Apostrophe(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		wantHint bool // expect apostrophe hint
	}{
		{
			name:     "short lowercase fragment s (it's)",
			msg:      "mismatched input 's' expecting {';', ','}",
			wantHint: true,
		},
		{
			name:     "short lowercase fragment ll (you'll)",
			msg:      "mismatched input 'll' expecting {';', ','}",
			wantHint: true,
		},
		{
			name:     "short lowercase fragment t (don't)",
			msg:      "mismatched input 't' expecting {';', ','}",
			wantHint: true,
		},
		{
			name:     "short lowercase fragment re (you're)",
			msg:      "extraneous input 're' expecting {';', ','}",
			wantHint: true,
		},
		{
			name:     "short lowercase fragment ve (we've)",
			msg:      "mismatched input 've' expecting {';', ','}",
			wantHint: true,
		},
		{
			name:     "missing at pattern (it's)",
			msg:      "missing END at 's'",
			wantHint: true,
		},
		{
			name:     "missing at pattern (don't)",
			msg:      "missing ';' at 't'",
			wantHint: true,
		},
		{
			name:     "token recognition error unbalanced quote",
			msg:      "token recognition error at: '';",
			wantHint: true,
		},
		{
			name:     "long token is not apostrophe",
			msg:      "mismatched input 'SELECT' expecting {';', ','}",
			wantHint: false,
		},
		{
			name:     "uppercase token is not apostrophe",
			msg:      "mismatched input 'IF' expecting {';', ','}",
			wantHint: false,
		},
		{
			name:     "number token is not apostrophe",
			msg:      "mismatched input '42' expecting {';', ','}",
			wantHint: false,
		},
		{
			name:     "unrelated error",
			msg:      "no viable alternative at input 'CREATE PERSISTENT'",
			wantHint: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := enhanceErrorMessage(tt.msg)
			hasHint := result != tt.msg
			if hasHint != tt.wantHint {
				if tt.wantHint {
					t.Errorf("expected apostrophe hint but got none.\n  input:  %s\n  output: %s", tt.msg, result)
				} else {
					t.Errorf("unexpected apostrophe hint.\n  input:  %s\n  output: %s", tt.msg, result)
				}
			}
			if tt.wantHint && hasHint {
				if !strings.Contains(result, "''") {
					t.Errorf("hint should mention '' escape, got: %s", result)
				}
			}
		})
	}
}

func TestParseError_UnescapedApostrophe(t *testing.T) {
	// This MDL contains an unescaped apostrophe in a string literal.
	// The parser should produce an error with an apostrophe hint.
	input := `CREATE MICROFLOW MyModule.Test ()
RETURNS String
BEGIN
  DECLARE $msg String = 'it's broken';
  RETURN $msg;
END;`

	_, errs := Build(input)
	if len(errs) == 0 {
		t.Fatal("expected parse errors for unescaped apostrophe")
	}

	// At least one error should contain the apostrophe hint
	found := false
	for _, err := range errs {
		if strings.Contains(err.Error(), "unescaped apostrophe") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected apostrophe hint in error messages, got:\n")
		for _, err := range errs {
			t.Errorf("  %v", err)
		}
	}
}

func TestEnhanceErrorMessage_QuotedGrantAttribute(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		wantHint bool
	}{
		{
			name:     "READ with quoted attribute",
			msg:      `line 1:51 no viable alternative at input 'READ"Attr1"'`,
			wantHint: true,
		},
		{
			name:     "WRITE with quoted attribute",
			msg:      `line 1:75 no viable alternative at input 'WRITE"Attr1"'`,
			wantHint: true,
		},
		{
			name:     "quoted string where access right expected",
			msg:      `mismatched input '"Attr2"' expecting {CREATE, DELETE, READ, WRITE}`,
			wantHint: true,
		},
		{
			name:     "unrelated error does not trigger",
			msg:      `no viable alternative at input 'CREATE PERSISTENT'`,
			wantHint: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := enhanceErrorMessage(tt.msg)
			hasHint := strings.Contains(result, "Attribute-level GRANT")
			if hasHint != tt.wantHint {
				t.Errorf("expected hint=%v\n  input:  %s\n  output: %s", tt.wantHint, tt.msg, result)
			}
		})
	}
}

func TestEnhanceErrorMessage_EnumEquals(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		wantHint bool
	}{
		{"enum value equals", `mismatched input '=' expecting ')'`, true},
		{"attribute default equals (different msg)", `no viable alternative at input 'string'`, false},
		{"unrelated mismatched", `mismatched input 'foo' expecting ';'`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := enhanceErrorMessage(tt.msg)
			hasHint := strings.Contains(result, "Enumeration values do not use '='")
			if hasHint != tt.wantHint {
				t.Errorf("expected hint=%v\n  input:  %s\n  output: %s", tt.wantHint, tt.msg, result)
			}
		})
	}
}

// TestParseError_EnumEqualsHint confirms the hint reaches the surface for the
// full statement, and that a quoted value name is NOT the problem — the `=` is.
func TestParseError_EnumEqualsHint(t *testing.T) {
	_, errs := Build(`CREATE ENUMERATION Mod."ENUM_X" ( "Grade1" = 'Grade 1' );`)
	if len(errs) == 0 {
		t.Fatal("expected a parse error for the enum '=' form")
	}
	joined := ""
	for _, e := range errs {
		joined += e.Error() + "\n"
	}
	if !strings.Contains(joined, "Enumeration values do not use '='") {
		t.Errorf("expected enum-equals hint, got: %s", joined)
	}
	// The quoted value name + space caption must parse cleanly (no error).
	if _, ok := Build(`CREATE ENUMERATION Mod."ENUM_X" ( "Grade1" 'Grade 1' );`); len(ok) != 0 {
		t.Errorf("quoted enum value name + caption should parse, got errors: %v", ok)
	}
}

func TestParseError_QuotedGrantAttribute(t *testing.T) {
	input := `GRANT Mod.Role ON Mod.Entity (READ "Attr1", "Attr2");`
	_, errs := Build(input)
	if len(errs) == 0 {
		t.Fatal("expected parse errors for quoted GRANT attribute")
	}
	found := false
	for _, err := range errs {
		if strings.Contains(err.Error(), "Attribute-level GRANT") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected GRANT-attribute hint in error messages, got:\n")
		for _, err := range errs {
			t.Errorf("  %v", err)
		}
	}
}

// TestParseError_MisplacedExtends verifies that an EXTENDS clause placed after
// the attribute parentheses produces an actionable hint about ordering (#4).
func TestParseError_MisplacedExtends(t *testing.T) {
	for _, input := range []string{
		`create entity Mod.Child (Name: String) extends Mod.Parent;`,
		`create persistent entity Mod.Child (Name: String) generalization Mod.Parent;`,
	} {
		_, errs := Build(input)
		if len(errs) == 0 {
			t.Fatalf("expected parse errors for misplaced EXTENDS in %q", input)
		}
		found := false
		for _, err := range errs {
			if strings.Contains(err.Error(), "must come BEFORE the attribute") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected misplaced-EXTENDS hint for %q, got:", input)
			for _, err := range errs {
				t.Errorf("  %v", err)
			}
		}
	}
}

// TestEnumDefaultQuotedIdentifier verifies that quoted identifiers in enum
// DEFAULT values are unquoted correctly (issue #11 / BUG-004).
// e.g. DEFAULT MaisonElegance."FormSubmissionStatus".StatusNew should store
// MaisonElegance.FormSubmissionStatus.StatusNew (quotes stripped).
func TestEnumDefaultQuotedIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantDflt string
	}{
		{
			name: "quoted enum name in DEFAULT",
			input: `CREATE PERSISTENT ENTITY MaisonElegance.FormSubmission (
				SubmissionStatus: Enumeration(MaisonElegance."FormSubmissionStatus") DEFAULT MaisonElegance."FormSubmissionStatus".StatusNew
			);`,
			wantDflt: "MaisonElegance.FormSubmissionStatus.StatusNew",
		},
		{
			name: "unquoted enum name in DEFAULT (unchanged)",
			input: `CREATE PERSISTENT ENTITY MaisonElegance.FormSubmission (
				SubmissionStatus: Enumeration(MaisonElegance.FormSubmissionStatus) DEFAULT MaisonElegance.FormSubmissionStatus.StatusNew
			);`,
			wantDflt: "MaisonElegance.FormSubmissionStatus.StatusNew",
		},
		{
			name: "backtick-quoted enum name in DEFAULT",
			input: "CREATE PERSISTENT ENTITY Test.MyEntity (\n" +
				"\tStatus: Enumeration(Test.`MyEnum`) DEFAULT Test.`MyEnum`.Active\n" +
				");",
			wantDflt: "Test.MyEnum.Active",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, errs := Build(tt.input)
			if len(errs) > 0 {
				for _, err := range errs {
					t.Errorf("Parse error: %v", err)
				}
				return
			}

			if len(prog.Statements) != 1 {
				t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
			}

			stmt, ok := prog.Statements[0].(*ast.CreateEntityStmt)
			if !ok {
				t.Fatalf("Expected CreateEntityStmt, got %T", prog.Statements[0])
			}

			if len(stmt.Attributes) != 1 {
				t.Fatalf("Expected 1 attribute, got %d", len(stmt.Attributes))
			}

			attr := stmt.Attributes[0]
			if !attr.HasDefault {
				t.Fatal("Expected HasDefault to be true")
			}

			got := fmt.Sprintf("%v", attr.DefaultValue)
			if got != tt.wantDflt {
				t.Errorf("DefaultValue = %q, want %q", got, tt.wantDflt)
			}
		})
	}
}

func TestSQLGenerateConnector(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantAlias  string
		wantModule string
		wantTables []string
		wantViews  []string
		wantExec   bool
	}{
		{
			name:       "basic",
			input:      `SQL mydb GENERATE CONNECTOR INTO HRModule;`,
			wantAlias:  "mydb",
			wantModule: "HRModule",
		},
		{
			name:       "with tables",
			input:      `SQL mydb GENERATE CONNECTOR INTO HRModule TABLES (employees, departments);`,
			wantAlias:  "mydb",
			wantModule: "HRModule",
			wantTables: []string{"employees", "departments"},
		},
		{
			name:       "with views",
			input:      `SQL mydb GENERATE CONNECTOR INTO HRModule VIEWS (active_users_v);`,
			wantAlias:  "mydb",
			wantModule: "HRModule",
			wantViews:  []string{"active_users_v"},
		},
		{
			name:       "tables and views with exec",
			input:      `SQL mydb GENERATE CONNECTOR INTO HRModule TABLES (employees) VIEWS (summary_v) EXEC;`,
			wantAlias:  "mydb",
			wantModule: "HRModule",
			wantTables: []string{"employees"},
			wantViews:  []string{"summary_v"},
			wantExec:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, errs := Build(tt.input)
			if len(errs) > 0 {
				t.Fatalf("Parse errors: %v", errs)
			}
			if len(prog.Statements) != 1 {
				t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
			}
			stmt, ok := prog.Statements[0].(*ast.SQLGenerateConnectorStmt)
			if !ok {
				t.Fatalf("Expected SQLGenerateConnectorStmt, got %T", prog.Statements[0])
			}
			if stmt.Alias != tt.wantAlias {
				t.Errorf("Alias = %q, want %q", stmt.Alias, tt.wantAlias)
			}
			if stmt.Module != tt.wantModule {
				t.Errorf("Module = %q, want %q", stmt.Module, tt.wantModule)
			}
			if stmt.Exec != tt.wantExec {
				t.Errorf("Exec = %v, want %v", stmt.Exec, tt.wantExec)
			}
			if tt.wantTables != nil {
				if len(stmt.Tables) != len(tt.wantTables) {
					t.Errorf("Tables count = %d, want %d", len(stmt.Tables), len(tt.wantTables))
				}
				for i, want := range tt.wantTables {
					if i < len(stmt.Tables) && stmt.Tables[i] != want {
						t.Errorf("Tables[%d] = %q, want %q", i, stmt.Tables[i], want)
					}
				}
			}
			if tt.wantViews != nil {
				if len(stmt.Views) != len(tt.wantViews) {
					t.Errorf("Views count = %d, want %d", len(stmt.Views), len(tt.wantViews))
				}
				for i, want := range tt.wantViews {
					if i < len(stmt.Views) && stmt.Views[i] != want {
						t.Errorf("Views[%d] = %q, want %q", i, stmt.Views[i], want)
					}
				}
			}
		})
	}
}

func TestCreateDemoUserWithEntity(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedEntity string
		expectedUser   string
		expectedRoles  []string
	}{
		{
			name:           "with explicit entity",
			input:          `CREATE DEMO USER 'admin' PASSWORD '1' ENTITY Administration.Account (Administrator);`,
			expectedEntity: "Administration.Account",
			expectedUser:   "admin",
			expectedRoles:  []string{"Administrator"},
		},
		{
			name:           "without entity clause",
			input:          `CREATE DEMO USER 'admin' PASSWORD '1' (Administrator);`,
			expectedEntity: "",
			expectedUser:   "admin",
			expectedRoles:  []string{"Administrator"},
		},
		{
			name:           "with entity and multiple roles",
			input:          `CREATE DEMO USER 'test' PASSWORD 'pwd' ENTITY MyModule.AppUser (User, Admin);`,
			expectedEntity: "MyModule.AppUser",
			expectedUser:   "test",
			expectedRoles:  []string{"User", "Admin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, errs := Build(tt.input)
			if len(errs) > 0 {
				for _, err := range errs {
					t.Errorf("Parse error: %v", err)
				}
				return
			}

			if len(prog.Statements) != 1 {
				t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
			}

			stmt, ok := prog.Statements[0].(*ast.CreateDemoUserStmt)
			if !ok {
				t.Fatalf("Expected CreateDemoUserStmt, got %T", prog.Statements[0])
			}

			if stmt.UserName != tt.expectedUser {
				t.Errorf("UserName = %q, want %q", stmt.UserName, tt.expectedUser)
			}
			if stmt.Entity != tt.expectedEntity {
				t.Errorf("Entity = %q, want %q", stmt.Entity, tt.expectedEntity)
			}
			if len(stmt.UserRoles) != len(tt.expectedRoles) {
				t.Fatalf("UserRoles count = %d, want %d", len(stmt.UserRoles), len(tt.expectedRoles))
			}
			for i, role := range stmt.UserRoles {
				if role != tt.expectedRoles[i] {
					t.Errorf("UserRoles[%d] = %q, want %q", i, role, tt.expectedRoles[i])
				}
			}
		})
	}
}

func TestCalculatedAttributeParsing(t *testing.T) {
	input := `CREATE PERSISTENT ENTITY DmTest.Product (
		Name: String(200) NOT NULL,
		Price: Decimal,
		FullPrice: Decimal CALCULATED
	);`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt := prog.Statements[0].(*ast.CreateEntityStmt)
	if len(stmt.Attributes) != 3 {
		t.Fatalf("Expected 3 attributes, got %d", len(stmt.Attributes))
	}

	// Name should NOT be calculated
	if stmt.Attributes[0].Calculated {
		t.Error("Name attribute should not be calculated")
	}

	// Price should NOT be calculated
	if stmt.Attributes[1].Calculated {
		t.Error("Price attribute should not be calculated")
	}

	// FullPrice should be calculated
	if !stmt.Attributes[2].Calculated {
		t.Error("FullPrice attribute should be calculated")
	}
	// FullPrice should NOT have a microflow reference (bare CALCULATED)
	if stmt.Attributes[2].CalculatedMicroflow != nil {
		t.Error("FullPrice attribute should not have a microflow reference with bare CALCULATED")
	}
}

func TestCalculatedAttributeWithMicroflow(t *testing.T) {
	input := `CREATE PERSISTENT ENTITY DmTest.Product (
		Name: String(200) NOT NULL,
		FullPrice: Decimal CALCULATED DmTest.CalcFullPrice
	);`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt := prog.Statements[0].(*ast.CreateEntityStmt)
	if len(stmt.Attributes) != 2 {
		t.Fatalf("Expected 2 attributes, got %d", len(stmt.Attributes))
	}

	// FullPrice should be calculated with microflow reference
	fullPrice := stmt.Attributes[1]
	if !fullPrice.Calculated {
		t.Error("FullPrice attribute should be calculated")
	}
	if fullPrice.CalculatedMicroflow == nil {
		t.Fatal("FullPrice attribute should have a microflow reference")
	}
	if fullPrice.CalculatedMicroflow.Module != "DmTest" {
		t.Errorf("Expected microflow module 'DmTest', got '%s'", fullPrice.CalculatedMicroflow.Module)
	}
	if fullPrice.CalculatedMicroflow.Name != "CalcFullPrice" {
		t.Errorf("Expected microflow name 'CalcFullPrice', got '%s'", fullPrice.CalculatedMicroflow.Name)
	}
}

func TestCalculatedAttributeWithByKeyword(t *testing.T) {
	input := `CREATE PERSISTENT ENTITY DmTest.Product (
		Name: String(200) NOT NULL,
		FullPrice: Decimal CALCULATED BY DmTest.CalcFullPrice
	);`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt := prog.Statements[0].(*ast.CreateEntityStmt)
	if len(stmt.Attributes) != 2 {
		t.Fatalf("Expected 2 attributes, got %d", len(stmt.Attributes))
	}

	fullPrice := stmt.Attributes[1]
	if !fullPrice.Calculated {
		t.Error("FullPrice attribute should be calculated")
	}
	if fullPrice.CalculatedMicroflow == nil {
		t.Fatal("FullPrice attribute should have a microflow reference")
	}
	if fullPrice.CalculatedMicroflow.Module != "DmTest" {
		t.Errorf("Expected microflow module 'DmTest', got '%s'", fullPrice.CalculatedMicroflow.Module)
	}
	if fullPrice.CalculatedMicroflow.Name != "CalcFullPrice" {
		t.Errorf("Expected microflow name 'CalcFullPrice', got '%s'", fullPrice.CalculatedMicroflow.Name)
	}
}

func TestCalculatedAttributeOnNonPersistentEntity(t *testing.T) {
	// This should parse successfully - validation happens at executor level
	input := `CREATE NON-PERSISTENT ENTITY DmTest.TempCalc (
		Value: Decimal CALCULATED BY DmTest.CalcValue
	);`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt := prog.Statements[0].(*ast.CreateEntityStmt)
	if stmt.Kind != ast.EntityNonPersistent {
		t.Error("Expected non-persistent entity")
	}
	if !stmt.Attributes[0].Calculated {
		t.Error("Value attribute should be calculated")
	}
}

func TestAnnotationBeforePositionIsFreeFloating(t *testing.T) {
	input := `CREATE MICROFLOW Synthetic.Check ()
RETURNS Boolean AS $Success
BEGIN
  @annotation 'free note'
  @position(100, 200)
  LOG INFO NODE 'SyntheticLog' 'message';
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)
	logStmt, ok := stmt.Body[0].(*ast.LogStmt)
	if !ok {
		t.Fatalf("Expected LogStmt, got %T", stmt.Body[0])
	}
	if logStmt.Annotations == nil {
		t.Fatal("expected annotations")
	}
	if got := logStmt.Annotations.FreeAnnotations; len(got) != 1 || got[0] != "free note" {
		t.Fatalf("free annotations = %#v, want [free note]", got)
	}
	if logStmt.Annotations.AnnotationText != "" {
		t.Fatalf("attached annotation = %q, want empty", logStmt.Annotations.AnnotationText)
	}
}

func TestMultipleAnnotationsBeforePositionStayFreeFloating(t *testing.T) {
	input := `CREATE MICROFLOW Synthetic.Check ()
RETURNS Boolean AS $Success
BEGIN
  @annotation 'first free note'
  @annotation 'second free note'
  @annotation 'third free note'
  @position(100, 200)
  LOG INFO NODE 'SyntheticLog' 'message';
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)
	logStmt, ok := stmt.Body[0].(*ast.LogStmt)
	if !ok {
		t.Fatalf("Expected LogStmt, got %T", stmt.Body[0])
	}
	if logStmt.Annotations == nil {
		t.Fatal("expected annotations")
	}
	want := []string{"first free note", "second free note", "third free note"}
	if got := logStmt.Annotations.FreeAnnotations; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("free annotations = %#v, want %#v", got, want)
	}
	if logStmt.Annotations.AnnotationText != "" {
		t.Fatalf("attached annotation = %q, want empty", logStmt.Annotations.AnnotationText)
	}
}

func TestAnnotationAfterPositionStaysAttached(t *testing.T) {
	input := `CREATE MICROFLOW Synthetic.Check ()
RETURNS Boolean AS $Success
BEGIN
  @position(100, 200)
  @annotation 'attached note'
  LOG INFO NODE 'SyntheticLog' 'message';
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)
	logStmt, ok := stmt.Body[0].(*ast.LogStmt)
	if !ok {
		t.Fatalf("Expected LogStmt, got %T", stmt.Body[0])
	}
	if logStmt.Annotations == nil {
		t.Fatal("expected annotations")
	}
	if logStmt.Annotations.AnnotationText != "attached note" {
		t.Fatalf("attached annotation = %q, want attached note", logStmt.Annotations.AnnotationText)
	}
	if len(logStmt.Annotations.FreeAnnotations) != 0 {
		t.Fatalf("free annotations = %#v, want empty", logStmt.Annotations.FreeAnnotations)
	}
}

func TestMicroflowPositionAnnotationAcceptsNegativeCoordinates(t *testing.T) {
	input := `CREATE MICROFLOW Synthetic.Check ()
BEGIN
  @position(-150, -210)
  LOG INFO NODE 'SyntheticLog' 'message';
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)
	logStmt, ok := stmt.Body[0].(*ast.LogStmt)
	if !ok {
		t.Fatalf("Expected LogStmt, got %T", stmt.Body[0])
	}
	if logStmt.Annotations == nil || logStmt.Annotations.Position == nil {
		t.Fatal("expected position annotation")
	}
	if got := *logStmt.Annotations.Position; got.X != -150 || got.Y != -210 {
		t.Fatalf("position = (%d, %d), want (-150, -210)", got.X, got.Y)
	}
}

func TestCallJavaActionAcceptsEmptyArguments(t *testing.T) {
	input := `CREATE MICROFLOW Synthetic.Check ()
RETURNS Boolean AS $Success
BEGIN
  $Total = CALL JAVA ACTION Synthetic.Recalculate(CompanyId = empty, RecalculateAll = true, ItemList = empty);
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)
	callStmt, ok := stmt.Body[0].(*ast.CallJavaActionStmt)
	if !ok {
		t.Fatalf("body[0] = %T, want *ast.CallJavaActionStmt", stmt.Body[0])
	}
	for _, idx := range []int{0, 2} {
		lit, ok := callStmt.Arguments[idx].Value.(*ast.LiteralExpr)
		if !ok {
			t.Fatalf("argument %d value = %T, want *ast.LiteralExpr", idx, callStmt.Arguments[idx].Value)
		}
		if lit.Kind != ast.LiteralNull {
			t.Fatalf("argument %d kind = %v, want LiteralNull", idx, lit.Kind)
		}
	}
}

func TestDeclareAndLogTemplatePreserveMultilineSourceWhitespace(t *testing.T) {
	input := `CREATE MICROFLOW Synthetic.Check ()
RETURNS Boolean AS $Success
BEGIN
  DECLARE $Endpoint String = @Synthetic.Endpoint
+ '/items?page=' + toString($Page)
+ '&token=' + (if $Token!=empty then $Token/Value
else
'');
  LOG INFO NODE 'SyntheticLog' 'Processed {1} items for {2}' WITH ({1} = toString($Count)

, {2} = $Endpoint);
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)
	decl, ok := stmt.Body[0].(*ast.DeclareStmt)
	if !ok {
		t.Fatalf("Expected DeclareStmt, got %T", stmt.Body[0])
	}
	logStmt, ok := stmt.Body[1].(*ast.LogStmt)
	if !ok {
		t.Fatalf("Expected LogStmt, got %T", stmt.Body[1])
	}

	declSource, ok := decl.InitialValue.(*ast.SourceExpr)
	if !ok {
		t.Fatalf("Expected SourceExpr declare value, got %T", decl.InitialValue)
	}
	wantDecl := "@Synthetic.Endpoint\n+ '/items?page=' + toString($Page)\n+ '&token=' + (if $Token!=empty then $Token/Value\nelse\n'')"
	if declSource.Source != wantDecl {
		t.Fatalf("declare source = %q, want %q", declSource.Source, wantDecl)
	}

	firstParam, ok := logStmt.Template[0].Value.(*ast.SourceExpr)
	if !ok {
		t.Fatalf("Expected SourceExpr first log template param, got %T", logStmt.Template[0].Value)
	}
	if firstParam.Source != "toString($Count)\n\n" {
		t.Fatalf("template param source = %q, want trailing blank line", firstParam.Source)
	}
}

func TestActionExpressionSlotsPreserveSourceWhitespace(t *testing.T) {
	input := `CREATE MICROFLOW Synthetic.PreserveExpressionSlots (
  $Input: String,
  $Count: Integer,
  $Flag: Boolean,
  $OtherFlag: Boolean
)
RETURNS Boolean AS $Result
BEGIN
  $Created = CREATE Synthetic.Entity (Name = if $Count=0 then 'zero'
else 'many'
, Description = 'sample');
  CHANGE $Created (Name = 'updated'
, Description = 'sample');
  $Loaded = CALL JAVA ACTION Synthetic.LoadValue(envVarName = 'SYNTHETIC_VALUE'
, defaultValue = @Synthetic.DefaultValue);
  IF isMatch( $Input, '[0-9]+') THEN
    SHOW MESSAGE 'Value {1}' TYPE Information OBJECTS [if $Flag then 'enabled'
else 'disabled'
, $Input];
  END IF;
  RETURN $Flag
and
$OtherFlag;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)

	createStmt := mf.Body[0].(*ast.CreateObjectStmt)
	if source, ok := createStmt.Changes[0].Value.(*ast.SourceExpr); !ok {
		t.Fatalf("expected create assignment SourceExpr, got %T", createStmt.Changes[0].Value)
	} else if !strings.Contains(source.Source, "\nelse 'many'\n") {
		t.Fatalf("create assignment source lost line breaks: %q", source.Source)
	}

	changeStmt := mf.Body[1].(*ast.ChangeObjectStmt)
	if source, ok := changeStmt.Changes[0].Value.(*ast.SourceExpr); !ok {
		t.Fatalf("expected change assignment SourceExpr, got %T", changeStmt.Changes[0].Value)
	} else if !strings.Contains(source.Source, "\n") {
		t.Fatalf("change assignment source lost trailing separator whitespace: %q", source.Source)
	}

	callStmt := mf.Body[2].(*ast.CallJavaActionStmt)
	if source, ok := callStmt.Arguments[0].Value.(*ast.SourceExpr); !ok {
		t.Fatalf("expected call argument SourceExpr, got %T", callStmt.Arguments[0].Value)
	} else if !strings.Contains(source.Source, "\n") {
		t.Fatalf("call argument source lost trailing separator whitespace: %q", source.Source)
	}

	ifStmt := mf.Body[3].(*ast.IfStmt)
	if source, ok := ifStmt.Condition.(*ast.SourceExpr); !ok {
		t.Fatalf("expected if condition SourceExpr, got %T", ifStmt.Condition)
	} else if source.Source != "isMatch( $Input, '[0-9]+')" {
		t.Fatalf("if condition source = %q", source.Source)
	}

	showStmt := ifStmt.ThenBody[0].(*ast.ShowMessageStmt)
	if source, ok := showStmt.TemplateArgs[0].(*ast.SourceExpr); !ok {
		t.Fatalf("expected show-message argument SourceExpr, got %T", showStmt.TemplateArgs[0])
	} else if !strings.Contains(source.Source, "\nelse 'disabled'\n") {
		t.Fatalf("show-message argument source lost line breaks: %q", source.Source)
	}

	returnStmt := mf.Body[4].(*ast.ReturnStmt)
	if source, ok := returnStmt.Value.(*ast.SourceExpr); !ok {
		t.Fatalf("expected return SourceExpr, got %T", returnStmt.Value)
	} else if source.Source != "$Flag\nand\n$OtherFlag" {
		t.Fatalf("return source = %q", source.Source)
	}
}

func TestRetrieveXPathPreservesOriginalPathSpacing(t *testing.T) {
	input := `CREATE MICROFLOW Synthetic.PreserveXPathPathSpacing ()
BEGIN
  RETRIEVE $Items FROM Synthetic.Item
    WHERE Synthetic.Item_Group/Synthetic.Group/Synthetic.Group_Company = $Company;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	retrieveStmt := mf.Body[0].(*ast.RetrieveStmt)
	source, ok := retrieveStmt.Where.(*ast.SourceExpr)
	if !ok {
		t.Fatalf("expected retrieve XPath SourceExpr, got %T", retrieveStmt.Where)
	}
	want := "Synthetic.Item_Group/Synthetic.Group/Synthetic.Group_Company = $Company"
	if source.Source != want {
		t.Fatalf("XPath source = %q, want %q", source.Source, want)
	}
}

func TestRetrieveMultipleXPathPredicatesPreservePredicateBoundaries(t *testing.T) {
	input := `CREATE MICROFLOW Synthetic.PreserveXPathPredicates (
  $Token: Synthetic.Token
)
BEGIN
  RETRIEVE $Items FROM Synthetic.Item
    WHERE [CreatedAt > $Token/CreatedAt or (CreatedAt = $Token/CreatedAt and ItemId > $Token/ItemId)]
    [CreatedAt < '[%CurrentDateTime%]']
    [ExternalId != empty];
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	retrieveStmt := mf.Body[0].(*ast.RetrieveStmt)
	source, ok := retrieveStmt.Where.(*ast.SourceExpr)
	if !ok {
		t.Fatalf("expected retrieve XPath SourceExpr, got %T", retrieveStmt.Where)
	}
	want := "[CreatedAt > $Token/CreatedAt or (CreatedAt = $Token/CreatedAt and ItemId > $Token/ItemId)][CreatedAt < '[%CurrentDateTime%]'][ExternalId != empty]"
	if source.Source != want {
		t.Fatalf("XPath source = %q, want %q", source.Source, want)
	}
}

func TestTrailingExpressionWhitespacePreservedForRoundtripSlots(t *testing.T) {
	input := `CREATE MICROFLOW Synthetic.PreserveTrailingExpressionWhitespace (
  $Object: Synthetic.Entity
)
RETURNS Boolean AS $Result
BEGIN
  DECLARE $Prefix String = 'Hello'
;
  SET $Prefix = substring($Prefix, 0, 1)
;
  CHANGE $Object (Name = $Prefix );
  LOG INFO NODE @Synthetic.LogNode
 'Processed {1}' WITH ({1} = $Prefix );
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	decl := mf.Body[0].(*ast.DeclareStmt)
	if source, ok := decl.InitialValue.(*ast.SourceExpr); !ok {
		t.Fatalf("expected declare initial value SourceExpr, got %T", decl.InitialValue)
	} else if source.Source != "'Hello'\n" {
		t.Fatalf("declare source = %q", source.Source)
	}

	setStmt := mf.Body[1].(*ast.MfSetStmt)
	if source, ok := setStmt.Value.(*ast.SourceExpr); !ok {
		t.Fatalf("expected set value SourceExpr, got %T", setStmt.Value)
	} else if source.Source != "substring($Prefix, 0, 1)\n" {
		t.Fatalf("set source = %q", source.Source)
	}

	changeStmt := mf.Body[2].(*ast.ChangeObjectStmt)
	if source, ok := changeStmt.Changes[0].Value.(*ast.SourceExpr); !ok {
		t.Fatalf("expected change value SourceExpr, got %T", changeStmt.Changes[0].Value)
	} else if source.Source != "$Prefix " {
		t.Fatalf("change source = %q", source.Source)
	}

	logStmt := mf.Body[3].(*ast.LogStmt)
	if source, ok := logStmt.Node.(*ast.SourceExpr); !ok {
		t.Fatalf("expected log node SourceExpr, got %T", logStmt.Node)
	} else if source.Source != "@Synthetic.LogNode\n" {
		t.Fatalf("log node source = %q", source.Source)
	}
	if source, ok := logStmt.Template[0].Value.(*ast.SourceExpr); !ok {
		t.Fatalf("expected log template SourceExpr, got %T", logStmt.Template[0].Value)
	} else if source.Source != "$Prefix " {
		t.Fatalf("template source = %q", source.Source)
	}
}

func TestRetrieveRangeExpressionsPreserveTrailingWhitespace(t *testing.T) {
	input := `CREATE MICROFLOW Synthetic.PreserveRetrieveRangeWhitespace (
  $UseFirstPage: Boolean,
  $PageSize: Integer,
  $PageNumber: Integer
)
RETURNS List OF Synthetic.Entity
BEGIN
  RETRIEVE $Items FROM Synthetic.Entity
    LIMIT $PageSize

    OFFSET IF $UseFirstPage THEN 0
ELSE
$PageSize * ($PageNumber - 1)
;
  RETURN $Items;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	retrieveStmt, ok := mf.Body[0].(*ast.RetrieveStmt)
	if !ok {
		t.Fatalf("expected RetrieveStmt, got %T", mf.Body[0])
	}
	if retrieveStmt.Limit != "$PageSize\n" {
		t.Fatalf("limit source = %q", retrieveStmt.Limit)
	}
	wantOffset := "IF $UseFirstPage THEN 0\nELSE\n$PageSize * ($PageNumber - 1)\n"
	if retrieveStmt.Offset != wantOffset {
		t.Fatalf("offset source = %q, want %q", retrieveStmt.Offset, wantOffset)
	}
}

func TestShouldPreserveExpressionSourceIgnoresStringLiteralPunctuation(t *testing.T) {
	if shouldPreserveExpressionSource("'Processed {1} items!'") {
		t.Fatal("plain string literal punctuation should not force SourceExpr preservation")
	}
	if shouldPreserveExpressionSource("'Owner''s item: {1}'") {
		t.Fatal("escaped quotes and colon inside a string literal should not force SourceExpr preservation")
	}
	if !shouldPreserveExpressionSource("$Token!=empty") {
		t.Fatal("compact operators outside string literals should preserve source")
	}
	if !shouldPreserveExpressionSource("substring($Text,0,find($Text, '.'))") {
		t.Fatal("compact comma-separated arguments should preserve source")
	}
	if !shouldPreserveExpressionSource("not($Object/Flag)") {
		t.Fatal("compact not() expressions should preserve source")
	}
}

func TestRenameModule_ObjectTypeIsLowercase(t *testing.T) {
	prog, errs := Build("RENAME MODULE OldMod TO NewMod;")
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.RenameStmt)
	if !ok {
		t.Fatalf("Expected *ast.RenameStmt, got %T", prog.Statements[0])
	}
	if stmt.ObjectType != "module" {
		t.Errorf("Expected ObjectType %q, got %q — executor switch uses lowercase", "module", stmt.ObjectType)
	}
	if stmt.Name.Module != "OldMod" {
		t.Errorf("Expected old name OldMod, got %s", stmt.Name.Module)
	}
	if stmt.NewName != "NewMod" {
		t.Errorf("Expected new name NewMod, got %s", stmt.NewName)
	}
}

func TestCreateEnumeration_DocCommentCarriedToAST(t *testing.T) {
	input := `/**
 * Priority levels for tasks.
 */
create enumeration DmTest.Priority (
  LOW 'Low',
  HIGH 'High'
);`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.CreateEnumerationStmt)
	if !ok {
		t.Fatalf("Expected *ast.CreateEnumerationStmt, got %T", prog.Statements[0])
	}
	if stmt.Documentation != "Priority levels for tasks." {
		t.Errorf("Documentation = %q, want %q — enum-level JavaDoc must reach the AST so the writer can persist it", stmt.Documentation, "Priority levels for tasks.")
	}
}

func TestCreateOrReplaceEnumeration_FlagsCreateOrModify(t *testing.T) {
	input := `create or replace enumeration DmTest.Priority (
  LOW 'Low'
);`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.CreateEnumerationStmt)
	if !ok {
		t.Fatalf("Expected *ast.CreateEnumerationStmt, got %T", prog.Statements[0])
	}
	if !stmt.CreateOrModify {
		t.Error("CREATE OR REPLACE should set CreateOrModify=true so the executor takes the update path")
	}
}
