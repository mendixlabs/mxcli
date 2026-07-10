// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestWorkflowVisitor_BoundaryEventInterrupting(t *testing.T) {
	input := `CREATE WORKFLOW M.TestWF
BEGIN
  USER TASK act1 'Caption'
    OUTCOMES 'Done' { }
    BOUNDARY EVENT INTERRUPTING TIMER '${PT1H}';
END WORKFLOW;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.CreateWorkflowStmt)
	if !ok {
		t.Fatalf("Expected CreateWorkflowStmt, got %T", prog.Statements[0])
	}

	if len(stmt.Activities) == 0 {
		t.Fatal("Expected at least 1 activity")
	}

	userTask, ok := stmt.Activities[0].(*ast.WorkflowUserTaskNode)
	if !ok {
		t.Fatalf("Expected WorkflowUserTaskNode, got %T", stmt.Activities[0])
	}

	if len(userTask.BoundaryEvents) == 0 {
		t.Fatal("Expected at least 1 boundary event")
	}

	be := userTask.BoundaryEvents[0]
	if be.EventType != "InterruptingTimer" {
		t.Errorf("Expected EventType 'InterruptingTimer', got %q", be.EventType)
	}
	if be.Delay != "${PT1H}" {
		t.Errorf("Expected Delay '${PT1H}', got %q", be.Delay)
	}
}

// TestWorkflowVisitor_UserTask_QuotedIdentifier is the regression test for
// issue #403: user task names written as "quoted-identifiers" caused a nil
// pointer dereference in buildWorkflowUserTask because the grammar only
// accepted bare IDENTIFIER tokens.
func TestWorkflowVisitor_UserTask_QuotedIdentifier(t *testing.T) {
	input := `CREATE WORKFLOW Module.WF_Test
BEGIN
  USER TASK "ut1" 'Review'
    PAGE Module.Page
  OUTCOMES
    'Approve' { }
    'Reject' { };
END WORKFLOW;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.CreateWorkflowStmt)
	if !ok {
		t.Fatalf("Expected CreateWorkflowStmt, got %T", prog.Statements[0])
	}
	if len(stmt.Activities) == 0 {
		t.Fatal("Expected at least 1 activity")
	}
	userTask, ok := stmt.Activities[0].(*ast.WorkflowUserTaskNode)
	if !ok {
		t.Fatalf("Expected WorkflowUserTaskNode, got %T", stmt.Activities[0])
	}
	// Quotes must be stripped: "ut1" → ut1
	if userTask.Name != "ut1" {
		t.Errorf("Expected task name %q, got %q", "ut1", userTask.Name)
	}
	if userTask.Caption != "Review" {
		t.Errorf("Expected caption %q, got %q", "Review", userTask.Caption)
	}
	if len(userTask.Outcomes) != 2 {
		t.Fatalf("Expected 2 outcomes, got %d", len(userTask.Outcomes))
	}
}

// TestWorkflowVisitor_JumpTo_QuotedIdentifier is the regression test for
// the JUMP TO "name" variant of the same underlying issue.
func TestWorkflowVisitor_JumpTo_QuotedIdentifier(t *testing.T) {
	input := `CREATE WORKFLOW Module.WF_Test
BEGIN
  USER TASK "target" 'Target task'
    OUTCOMES 'Done' { };
  JUMP TO "target";
END WORKFLOW;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt, ok := prog.Statements[0].(*ast.CreateWorkflowStmt)
	if !ok {
		t.Fatalf("Expected CreateWorkflowStmt, got %T", prog.Statements[0])
	}

	// Find the JumpTo activity
	var jumpTo *ast.WorkflowJumpToNode
	for _, act := range stmt.Activities {
		if jt, ok := act.(*ast.WorkflowJumpToNode); ok {
			jumpTo = jt
			break
		}
	}
	if jumpTo == nil {
		t.Fatal("Expected a WorkflowJumpToNode in activities")
	}
	// Quotes must be stripped: "target" → target
	if jumpTo.Target != "target" {
		t.Errorf("Expected jump target %q, got %q", "target", jumpTo.Target)
	}
}

func TestWorkflowVisitor_BoundaryEventNonInterrupting(t *testing.T) {
	input := `CREATE WORKFLOW M.TestWF
BEGIN
  USER TASK act1 'Caption'
    OUTCOMES 'Done' { }
    BOUNDARY EVENT NON INTERRUPTING TIMER '${PT2H}';
END WORKFLOW;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)
	userTask := stmt.Activities[0].(*ast.WorkflowUserTaskNode)

	if len(userTask.BoundaryEvents) == 0 {
		t.Fatal("Expected at least 1 boundary event")
	}

	be := userTask.BoundaryEvents[0]
	if be.EventType != "NonInterruptingTimer" {
		t.Errorf("Expected EventType 'NonInterruptingTimer', got %q", be.EventType)
	}
	if be.Delay != "${PT2H}" {
		t.Errorf("Expected Delay '${PT2H}', got %q", be.Delay)
	}
}

func TestWorkflowVisitor_BoundaryEventTimerBare(t *testing.T) {
	input := `CREATE WORKFLOW M.TestWF
BEGIN
  USER TASK act1 'Caption'
    OUTCOMES 'Done' { }
    BOUNDARY EVENT TIMER;
END WORKFLOW;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)
	userTask := stmt.Activities[0].(*ast.WorkflowUserTaskNode)

	if len(userTask.BoundaryEvents) == 0 {
		t.Fatal("Expected at least 1 boundary event")
	}

	be := userTask.BoundaryEvents[0]
	if be.EventType != "Timer" {
		t.Errorf("Expected EventType 'Timer', got %q", be.EventType)
	}
	if be.Delay != "" {
		t.Errorf("Expected empty Delay, got %q", be.Delay)
	}
}

func TestWorkflowVisitor_AnnotationRoundTrip(t *testing.T) {
	input := `CREATE WORKFLOW M.TestWF
BEGIN
  ANNOTATION 'This is a test note';
  USER TASK act1 'Do something'
    OUTCOMES 'Done' { };
END WORKFLOW;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)
	if len(stmt.Activities) < 2 {
		t.Fatalf("Expected at least 2 activities, got %d", len(stmt.Activities))
	}

	ann, ok := stmt.Activities[0].(*ast.WorkflowAnnotationActivityNode)
	if !ok {
		t.Fatalf("Expected WorkflowAnnotationActivityNode, got %T", stmt.Activities[0])
	}
	if ann.Text != "This is a test note" {
		t.Errorf("Expected annotation text 'This is a test note', got %q", ann.Text)
	}
}

func TestWorkflowVisitor_BoundaryEventWithSubFlow(t *testing.T) {
	input := `CREATE WORKFLOW M.TestWF
BEGIN
  WAIT FOR NOTIFICATION
    BOUNDARY EVENT INTERRUPTING TIMER '${PT1H}' {
      CALL MICROFLOW M.HandleTimeout;
    };
END WORKFLOW;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)
	if len(stmt.Activities) == 0 {
		t.Fatal("Expected at least 1 activity")
	}

	waitNode, ok := stmt.Activities[0].(*ast.WorkflowWaitForNotificationNode)
	if !ok {
		t.Fatalf("Expected WorkflowWaitForNotificationNode, got %T", stmt.Activities[0])
	}

	if len(waitNode.BoundaryEvents) == 0 {
		t.Fatal("Expected at least 1 boundary event")
	}

	be := waitNode.BoundaryEvents[0]
	if be.EventType != "InterruptingTimer" {
		t.Errorf("Expected EventType 'InterruptingTimer', got %q", be.EventType)
	}
	if be.Delay != "${PT1H}" {
		t.Errorf("Expected Delay '${PT1H}', got %q", be.Delay)
	}
	if len(be.Activities) == 0 {
		t.Fatal("Expected sub-flow activities in boundary event")
	}
	callMf, ok := be.Activities[0].(*ast.WorkflowCallMicroflowNode)
	if !ok {
		t.Fatalf("Expected WorkflowCallMicroflowNode in sub-flow, got %T", be.Activities[0])
	}
	if callMf.Microflow.Name != "HandleTimeout" {
		t.Errorf("Expected microflow name 'HandleTimeout', got %q", callMf.Microflow.Name)
	}
}

func TestWorkflowVisitor_MultiUserTask(t *testing.T) {
	input := `CREATE WORKFLOW M.TestWF
BEGIN
  MULTI USER TASK act1 'Caption'
    PAGE M.ReviewPage
    OUTCOMES 'Approve' { };
END WORKFLOW;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)

	if len(stmt.Activities) == 0 {
		t.Fatal("Expected at least 1 activity")
	}

	userTask, ok := stmt.Activities[0].(*ast.WorkflowUserTaskNode)
	if !ok {
		t.Fatalf("Expected WorkflowUserTaskNode, got %T", stmt.Activities[0])
	}

	if !userTask.IsMultiUser {
		t.Error("Expected IsMultiUser to be true")
	}
	if userTask.Page.Module != "M" || userTask.Page.Name != "ReviewPage" {
		t.Errorf("Expected Page M.ReviewPage, got %s.%s", userTask.Page.Module, userTask.Page.Name)
	}
}

func TestWorkflowVisitor_ParameterMappingWith(t *testing.T) {
	input := `CREATE WORKFLOW M.TestWF
BEGIN
  CALL MICROFLOW M.CalcDiscount
    WITH (Amount = '$WorkflowContext/Amount')
    OUTCOMES
      TRUE -> { }
      FALSE -> { };
END WORKFLOW;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)

	if len(stmt.Activities) == 0 {
		t.Fatal("Expected at least 1 activity")
	}

	callMf, ok := stmt.Activities[0].(*ast.WorkflowCallMicroflowNode)
	if !ok {
		t.Fatalf("Expected WorkflowCallMicroflowNode, got %T", stmt.Activities[0])
	}

	if len(callMf.ParameterMappings) == 0 {
		t.Fatal("Expected at least 1 parameter mapping")
	}

	pm := callMf.ParameterMappings[0]
	if pm.Parameter != "Amount" {
		t.Errorf("Expected Parameter 'Amount', got %q", pm.Parameter)
	}
	if pm.Expression != "$WorkflowContext/Amount" {
		t.Errorf("Expected Expression '$WorkflowContext/Amount', got %q", pm.Expression)
	}
}

func TestWorkflowVisitor_CallWorkflowWithParams(t *testing.T) {
	input := `CREATE WORKFLOW M.TestWF
BEGIN
  CALL WORKFLOW M.SubWorkflow COMMENT 'Call sub' WITH (WorkflowContext = '$WorkflowContext');
END WORKFLOW;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)

	if len(stmt.Activities) == 0 {
		t.Fatal("Expected at least 1 activity")
	}

	callWf, ok := stmt.Activities[0].(*ast.WorkflowCallWorkflowNode)
	if !ok {
		t.Fatalf("Expected WorkflowCallWorkflowNode, got %T", stmt.Activities[0])
	}

	if callWf.Caption != "Call sub" {
		t.Errorf("Expected Caption 'Call sub', got %q", callWf.Caption)
	}

	if len(callWf.ParameterMappings) != 1 {
		t.Fatalf("Expected 1 parameter mapping, got %d", len(callWf.ParameterMappings))
	}

	pm := callWf.ParameterMappings[0]
	if pm.Parameter != "WorkflowContext" {
		t.Errorf("Expected Parameter 'WorkflowContext', got %q", pm.Parameter)
	}
	if pm.Expression != "$WorkflowContext" {
		t.Errorf("Expected Expression '$WorkflowContext', got %q", pm.Expression)
	}
}

func TestWorkflowVisitor_UserTaskDueDate(t *testing.T) {
	input := `CREATE WORKFLOW M.TestWF
BEGIN
  USER TASK task1 'My Task'
    ENTITY M.TaskContext
    DUE DATE 'PT24H'
    OUTCOMES 'Done' { };
END WORKFLOW;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)
	userTask, ok := stmt.Activities[0].(*ast.WorkflowUserTaskNode)
	if !ok {
		t.Fatalf("Expected WorkflowUserTaskNode, got %T", stmt.Activities[0])
	}

	if userTask.DueDate != "PT24H" {
		t.Errorf("Expected DueDate 'PT24H', got %q", userTask.DueDate)
	}
}

func TestWorkflowVisitor_UserTaskDueDateWithXPath(t *testing.T) {
	input := `CREATE WORKFLOW M.TestWF
BEGIN
  USER TASK task1 'My Task'
    TARGETING XPATH '[Assignee = $currentUser]'
    ENTITY M.TaskContext
    DUE DATE 'PT48H'
    OUTCOMES 'Done' { };
END WORKFLOW;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)
	userTask, ok := stmt.Activities[0].(*ast.WorkflowUserTaskNode)
	if !ok {
		t.Fatalf("Expected WorkflowUserTaskNode, got %T", stmt.Activities[0])
	}

	if userTask.Targeting.Kind != "xpath" {
		t.Errorf("Expected Targeting.Kind 'xpath', got %q", userTask.Targeting.Kind)
	}
	if userTask.DueDate != "PT48H" {
		t.Errorf("Expected DueDate 'PT48H', got %q", userTask.DueDate)
	}
}

func TestWorkflowVisitor_Annotation(t *testing.T) {
	input := `CREATE WORKFLOW M.TestWF
BEGIN
  ANNOTATION 'This is a workflow note';
END WORKFLOW;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)

	if len(stmt.Activities) == 0 {
		t.Fatal("Expected at least 1 activity")
	}

	ann, ok := stmt.Activities[0].(*ast.WorkflowAnnotationActivityNode)
	if !ok {
		t.Fatalf("Expected WorkflowAnnotationActivityNode, got %T", stmt.Activities[0])
	}

	if ann.Text != "This is a workflow note" {
		t.Errorf("Expected Text 'This is a workflow note', got %q", ann.Text)
	}
}

func TestWorkflowVisitor_DisplayDescriptionExportLevel(t *testing.T) {
	input := `CREATE WORKFLOW Module.Test
  PARAMETER $ctx: Module.Entity
  DISPLAY 'My Display Name'
  DESCRIPTION 'My description'
  EXPORT LEVEL Hidden
BEGIN
END WORKFLOW`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.CreateWorkflowStmt)
	if !ok {
		t.Fatalf("Expected CreateWorkflowStmt, got %T", prog.Statements[0])
	}

	if stmt.DisplayName != "My Display Name" {
		t.Errorf("Expected DisplayName 'My Display Name', got %q", stmt.DisplayName)
	}
	if stmt.Description != "My description" {
		t.Errorf("Expected Description 'My description', got %q", stmt.Description)
	}
	if stmt.ExportLevel != "Hidden" {
		t.Errorf("Expected ExportLevel 'Hidden', got %q", stmt.ExportLevel)
	}
}

func TestWorkflowVisitor_DisplayOnly(t *testing.T) {
	input := `CREATE WORKFLOW Module.Test
  DISPLAY 'Just a display name'
BEGIN
END WORKFLOW`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)

	if stmt.DisplayName != "Just a display name" {
		t.Errorf("Expected DisplayName 'Just a display name', got %q", stmt.DisplayName)
	}
	if stmt.Description != "" {
		t.Errorf("Expected empty Description, got %q", stmt.Description)
	}
	if stmt.ExportLevel != "" {
		t.Errorf("Expected empty ExportLevel, got %q", stmt.ExportLevel)
	}
}

func TestWorkflowVisitor_DescriptionWithoutDisplay(t *testing.T) {
	input := `CREATE WORKFLOW Module.Test
  DESCRIPTION 'Only description'
BEGIN
END WORKFLOW`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)

	if stmt.DisplayName != "" {
		t.Errorf("Expected empty DisplayName, got %q", stmt.DisplayName)
	}
	if stmt.Description != "Only description" {
		t.Errorf("Expected Description 'Only description', got %q", stmt.Description)
	}
}

func TestWorkflowVisitor_ExportLevelAPI(t *testing.T) {
	input := `CREATE WORKFLOW Module.Test
  EXPORT LEVEL API
BEGIN
END WORKFLOW`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)

	if stmt.ExportLevel != "API" {
		t.Errorf("Expected ExportLevel 'API', got %q", stmt.ExportLevel)
	}
}

func TestWorkflowVisitor_AllMetadataWithDueDate(t *testing.T) {
	input := `CREATE WORKFLOW Module.Test
  PARAMETER $ctx: Module.Entity
  DISPLAY 'Approval Workflow'
  DESCRIPTION 'Handles the approval process'
  EXPORT LEVEL Hidden
  DUE DATE 'addDays([%%CurrentDateTime%%], 7)'
BEGIN
END WORKFLOW`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)

	if stmt.DisplayName != "Approval Workflow" {
		t.Errorf("Expected DisplayName 'Approval Workflow', got %q", stmt.DisplayName)
	}
	if stmt.Description != "Handles the approval process" {
		t.Errorf("Expected Description 'Handles the approval process', got %q", stmt.Description)
	}
	if stmt.ExportLevel != "Hidden" {
		t.Errorf("Expected ExportLevel 'Hidden', got %q", stmt.ExportLevel)
	}
	if stmt.DueDate != "addDays([%%CurrentDateTime%%], 7)" {
		t.Errorf("Expected DueDate 'addDays([%%%%CurrentDateTime%%%%], 7)', got %q", stmt.DueDate)
	}
}

func TestWorkflowVisitor_UserTaskDescription(t *testing.T) {
	input := `CREATE WORKFLOW M.TestWF
BEGIN
  USER TASK review 'Review'
    PAGE M.ReviewPage
    DESCRIPTION 'Please review carefully'
    OUTCOMES 'Done' { };
END WORKFLOW;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)
	userTask := stmt.Activities[0].(*ast.WorkflowUserTaskNode)

	if userTask.TaskDescription != "Please review carefully" {
		t.Errorf("TaskDescription = %q, want %q", userTask.TaskDescription, "Please review carefully")
	}
}

// ============================================================================
// ALTER WORKFLOW Tests
// ============================================================================

func TestAlterWorkflow_SetDisplay(t *testing.T) {
	input := `ALTER WORKFLOW M.MyWF SET DISPLAY 'New Display Name';`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.AlterWorkflowStmt)
	if !ok {
		t.Fatalf("Expected AlterWorkflowStmt, got %T", prog.Statements[0])
	}

	if stmt.Name.Module != "M" || stmt.Name.Name != "MyWF" {
		t.Errorf("Expected M.MyWF, got %s.%s", stmt.Name.Module, stmt.Name.Name)
	}

	if len(stmt.Operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(stmt.Operations))
	}

	op, ok := stmt.Operations[0].(*ast.SetWorkflowPropertyOp)
	if !ok {
		t.Fatalf("Expected SetWorkflowPropertyOp, got %T", stmt.Operations[0])
	}
	if op.Property != "display" {
		t.Errorf("Expected Property 'DISPLAY', got %q", op.Property)
	}
	if op.Value != "New Display Name" {
		t.Errorf("Expected Value 'New Display Name', got %q", op.Value)
	}
}

func TestAlterWorkflow_MultipleOperations(t *testing.T) {
	input := `ALTER WORKFLOW M.ApprovalWF
  SET DISPLAY 'Updated Approval'
  SET DESCRIPTION 'Updated description'
  SET EXPORT LEVEL Hidden
  SET DUE DATE 'PT48H';`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	if len(stmt.Operations) != 4 {
		t.Fatalf("Expected 4 operations, got %d", len(stmt.Operations))
	}

	// Check each operation
	checks := []struct {
		prop  string
		value string
	}{
		{"display", "Updated Approval"},
		{"description", "Updated description"},
		{"export_level", "Hidden"},
		{"due_date", "PT48H"},
	}

	for i, check := range checks {
		op, ok := stmt.Operations[i].(*ast.SetWorkflowPropertyOp)
		if !ok {
			t.Fatalf("Operation %d: expected SetWorkflowPropertyOp, got %T", i, stmt.Operations[i])
		}
		if op.Property != check.prop {
			t.Errorf("Operation %d: expected Property %q, got %q", i, check.prop, op.Property)
		}
		if op.Value != check.value {
			t.Errorf("Operation %d: expected Value %q, got %q", i, check.value, op.Value)
		}
	}
}

func TestAlterWorkflow_SetActivityPage(t *testing.T) {
	input := `ALTER WORKFLOW M.WF SET ACTIVITY 'Review' PAGE M.NewReviewPage;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	if len(stmt.Operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(stmt.Operations))
	}

	op, ok := stmt.Operations[0].(*ast.SetActivityPropertyOp)
	if !ok {
		t.Fatalf("Expected SetActivityPropertyOp, got %T", stmt.Operations[0])
	}
	if op.Property != "page" {
		t.Errorf("Expected Property 'PAGE', got %q", op.Property)
	}
	if op.ActivityRef != "Review" {
		t.Errorf("Expected ActivityRef 'Review', got %q", op.ActivityRef)
	}
	if op.PageName.Module != "M" || op.PageName.Name != "NewReviewPage" {
		t.Errorf("Expected PageName M.NewReviewPage, got %s.%s", op.PageName.Module, op.PageName.Name)
	}
}

func TestAlterWorkflow_SetActivityWithAtPosition(t *testing.T) {
	input := `ALTER WORKFLOW M.WF SET ACTIVITY 'Review' @ 2 DESCRIPTION 'Updated desc';`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	op := stmt.Operations[0].(*ast.SetActivityPropertyOp)

	if op.ActivityRef != "Review" {
		t.Errorf("Expected ActivityRef 'Review', got %q", op.ActivityRef)
	}
	if op.AtPosition != 2 {
		t.Errorf("Expected AtPosition 2, got %d", op.AtPosition)
	}
	if op.Property != "description" {
		t.Errorf("Expected Property 'DESCRIPTION', got %q", op.Property)
	}
	if op.Value != "Updated desc" {
		t.Errorf("Expected Value 'Updated desc', got %q", op.Value)
	}
}

func TestAlterWorkflow_InsertAfter(t *testing.T) {
	input := `ALTER WORKFLOW M.WF INSERT AFTER 'Review' CALL MICROFLOW M.SendNotification;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	op, ok := stmt.Operations[0].(*ast.InsertAfterOp)
	if !ok {
		t.Fatalf("Expected InsertAfterOp, got %T", stmt.Operations[0])
	}
	if op.ActivityRef != "Review" {
		t.Errorf("Expected ActivityRef 'Review', got %q", op.ActivityRef)
	}

	callMf, ok := op.NewActivity.(*ast.WorkflowCallMicroflowNode)
	if !ok {
		t.Fatalf("Expected WorkflowCallMicroflowNode, got %T", op.NewActivity)
	}
	if callMf.Microflow.Name != "SendNotification" {
		t.Errorf("Expected Microflow 'SendNotification', got %q", callMf.Microflow.Name)
	}
}

func TestAlterWorkflow_DropActivity(t *testing.T) {
	input := `ALTER WORKFLOW M.WF DROP ACTIVITY 'OldStep';`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	op, ok := stmt.Operations[0].(*ast.DropActivityOp)
	if !ok {
		t.Fatalf("Expected DropActivityOp, got %T", stmt.Operations[0])
	}
	if op.ActivityRef != "OldStep" {
		t.Errorf("Expected ActivityRef 'OldStep', got %q", op.ActivityRef)
	}
}

func TestAlterWorkflow_ReplaceActivity(t *testing.T) {
	input := `ALTER WORKFLOW M.WF REPLACE ACTIVITY 'OldStep' WITH CALL MICROFLOW M.NewStep;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	op, ok := stmt.Operations[0].(*ast.ReplaceActivityOp)
	if !ok {
		t.Fatalf("Expected ReplaceActivityOp, got %T", stmt.Operations[0])
	}
	if op.ActivityRef != "OldStep" {
		t.Errorf("Expected ActivityRef 'OldStep', got %q", op.ActivityRef)
	}

	_, ok = op.NewActivity.(*ast.WorkflowCallMicroflowNode)
	if !ok {
		t.Fatalf("Expected WorkflowCallMicroflowNode, got %T", op.NewActivity)
	}
}

func TestAlterWorkflow_InsertOutcome(t *testing.T) {
	input := `ALTER WORKFLOW M.WF INSERT OUTCOME 'Rejected' ON 'Review' {
  CALL MICROFLOW M.HandleRejection;
};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	op, ok := stmt.Operations[0].(*ast.InsertOutcomeOp)
	if !ok {
		t.Fatalf("Expected InsertOutcomeOp, got %T", stmt.Operations[0])
	}
	if op.OutcomeName != "Rejected" {
		t.Errorf("Expected OutcomeName 'Rejected', got %q", op.OutcomeName)
	}
	if op.ActivityRef != "Review" {
		t.Errorf("Expected ActivityRef 'Review', got %q", op.ActivityRef)
	}
	if len(op.Activities) != 1 {
		t.Fatalf("Expected 1 activity in outcome body, got %d", len(op.Activities))
	}
}

func TestAlterWorkflow_DropOutcome(t *testing.T) {
	input := `ALTER WORKFLOW M.WF DROP OUTCOME 'Rejected' ON 'Review';`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	op, ok := stmt.Operations[0].(*ast.DropOutcomeOp)
	if !ok {
		t.Fatalf("Expected DropOutcomeOp, got %T", stmt.Operations[0])
	}
	if op.OutcomeName != "Rejected" {
		t.Errorf("Expected OutcomeName 'Rejected', got %q", op.OutcomeName)
	}
	if op.ActivityRef != "Review" {
		t.Errorf("Expected ActivityRef 'Review', got %q", op.ActivityRef)
	}
}

func TestAlterWorkflow_InsertAndDropPath(t *testing.T) {
	input := `ALTER WORKFLOW M.WF
  INSERT PATH ON 'Split1' {
    CALL MICROFLOW M.PathAction;
  }
  DROP PATH 'OldPath' ON 'Split1';`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	if len(stmt.Operations) != 2 {
		t.Fatalf("Expected 2 operations, got %d", len(stmt.Operations))
	}

	insertOp, ok := stmt.Operations[0].(*ast.InsertPathOp)
	if !ok {
		t.Fatalf("Expected InsertPathOp, got %T", stmt.Operations[0])
	}
	if insertOp.ActivityRef != "Split1" {
		t.Errorf("Expected ActivityRef 'Split1', got %q", insertOp.ActivityRef)
	}
	if len(insertOp.Activities) != 1 {
		t.Fatalf("Expected 1 activity in path body, got %d", len(insertOp.Activities))
	}

	dropOp, ok := stmt.Operations[1].(*ast.DropPathOp)
	if !ok {
		t.Fatalf("Expected DropPathOp, got %T", stmt.Operations[1])
	}
	if dropOp.PathCaption != "OldPath" {
		t.Errorf("Expected PathCaption 'OldPath', got %q", dropOp.PathCaption)
	}
}

func TestAlterWorkflow_InsertBoundaryEvent(t *testing.T) {
	input := `ALTER WORKFLOW M.WF INSERT BOUNDARY EVENT ON 'task1' INTERRUPTING TIMER '${PT1H}' {
  CALL MICROFLOW M.HandleTimeout;
};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	op, ok := stmt.Operations[0].(*ast.InsertBoundaryEventOp)
	if !ok {
		t.Fatalf("Expected InsertBoundaryEventOp, got %T", stmt.Operations[0])
	}
	if op.ActivityRef != "task1" {
		t.Errorf("Expected ActivityRef 'task1', got %q", op.ActivityRef)
	}
	if op.EventType != "InterruptingTimer" {
		t.Errorf("Expected EventType 'InterruptingTimer', got %q", op.EventType)
	}
	if op.Delay != "${PT1H}" {
		t.Errorf("Expected Delay '${PT1H}', got %q", op.Delay)
	}
	if len(op.Activities) != 1 {
		t.Fatalf("Expected 1 activity in boundary event body, got %d", len(op.Activities))
	}
}

func TestAlterWorkflow_DropBoundaryEvent(t *testing.T) {
	input := `ALTER WORKFLOW M.WF DROP BOUNDARY EVENT ON 'task1';`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	op, ok := stmt.Operations[0].(*ast.DropBoundaryEventOp)
	if !ok {
		t.Fatalf("Expected DropBoundaryEventOp, got %T", stmt.Operations[0])
	}
	if op.ActivityRef != "task1" {
		t.Errorf("Expected ActivityRef 'task1', got %q", op.ActivityRef)
	}
}

func TestAlterWorkflow_InsertAndDropCondition(t *testing.T) {
	input := `ALTER WORKFLOW M.WF
  INSERT CONDITION '$Amount > 1000' ON 'decision1' {
    CALL MICROFLOW M.HighValueApproval;
  }
  DROP CONDITION 'LowValue' ON 'decision1';`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	if len(stmt.Operations) != 2 {
		t.Fatalf("Expected 2 operations, got %d", len(stmt.Operations))
	}

	insertOp, ok := stmt.Operations[0].(*ast.InsertBranchOp)
	if !ok {
		t.Fatalf("Expected InsertBranchOp, got %T", stmt.Operations[0])
	}
	if insertOp.Condition != "$Amount > 1000" {
		t.Errorf("Expected Condition '$Amount > 1000', got %q", insertOp.Condition)
	}
	if insertOp.ActivityRef != "decision1" {
		t.Errorf("Expected ActivityRef 'decision1', got %q", insertOp.ActivityRef)
	}

	dropOp, ok := stmt.Operations[1].(*ast.DropBranchOp)
	if !ok {
		t.Fatalf("Expected DropBranchOp, got %T", stmt.Operations[1])
	}
	if dropOp.BranchName != "LowValue" {
		t.Errorf("Expected BranchName 'LowValue', got %q", dropOp.BranchName)
	}
}

func TestAlterWorkflow_SetParameter(t *testing.T) {
	input := `ALTER WORKFLOW M.WF SET PARAMETER $ctx: M.NewEntity;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	op, ok := stmt.Operations[0].(*ast.SetWorkflowPropertyOp)
	if !ok {
		t.Fatalf("Expected SetWorkflowPropertyOp, got %T", stmt.Operations[0])
	}
	if op.Property != "parameter" {
		t.Errorf("Expected Property 'PARAMETER', got %q", op.Property)
	}
	if op.Value != "$ctx" {
		t.Errorf("Expected Value '$ctx', got %q", op.Value)
	}
	if op.Entity.Module != "M" || op.Entity.Name != "NewEntity" {
		t.Errorf("Expected Entity M.NewEntity, got %s.%s", op.Entity.Module, op.Entity.Name)
	}
}

func TestAlterWorkflow_SetOverviewPage(t *testing.T) {
	input := `ALTER WORKFLOW M.WF SET OVERVIEW PAGE M.AdminPage;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	op, ok := stmt.Operations[0].(*ast.SetWorkflowPropertyOp)
	if !ok {
		t.Fatalf("Expected SetWorkflowPropertyOp, got %T", stmt.Operations[0])
	}
	if op.Property != "overview_page" {
		t.Errorf("Expected Property 'OVERVIEW_PAGE', got %q", op.Property)
	}
	if op.Entity.Module != "M" || op.Entity.Name != "AdminPage" {
		t.Errorf("Expected Entity M.AdminPage, got %s.%s", op.Entity.Module, op.Entity.Name)
	}
}

func TestAlterWorkflow_SetActivityTargeting(t *testing.T) {
	input := `ALTER WORKFLOW M.WF
  SET ACTIVITY 'Review' TARGETING MICROFLOW M.GetReviewers
  SET ACTIVITY 'Review' TARGETING XPATH '[Role = ''Manager'']';`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	if len(stmt.Operations) != 2 {
		t.Fatalf("Expected 2 operations, got %d", len(stmt.Operations))
	}

	mfOp := stmt.Operations[0].(*ast.SetActivityPropertyOp)
	if mfOp.Property != "targeting_microflow" {
		t.Errorf("Expected Property 'TARGETING_MICROFLOW', got %q", mfOp.Property)
	}
	if mfOp.Microflow.Name != "GetReviewers" {
		t.Errorf("Expected Microflow 'GetReviewers', got %q", mfOp.Microflow.Name)
	}

	xpOp := stmt.Operations[1].(*ast.SetActivityPropertyOp)
	if xpOp.Property != "targeting_xpath" {
		t.Errorf("Expected Property 'TARGETING_XPATH', got %q", xpOp.Property)
	}
	if xpOp.Value != "[Role = 'Manager']" {
		t.Errorf("Expected Value \"[Role = 'Manager']\", got %q", xpOp.Value)
	}
}

// TestAlterWorkflow_DropBoundaryEvent_KeywordActivityRef verifies that using a keyword
// token (e.g. "activity") as an activity reference in DROP BOUNDARY EVENT does not panic
// — regression for issue #430 where AlterActivityRef() returned nil causing a panic via
// a bare type assertion with no nil guard.
func TestAlterWorkflow_DropBoundaryEvent_KeywordActivityRef(t *testing.T) {
	// "activity" is a keyword token; alterActivityRef now uses identifierOrKeyword so
	// this must parse without panicking and produce a DropBoundaryEventOp with ref "activity".
	input := `ALTER WORKFLOW M.WF DROP BOUNDARY EVENT ON activity;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt, ok := prog.Statements[0].(*ast.AlterWorkflowStmt)
	if !ok || len(stmt.Operations) == 0 {
		t.Fatal("Expected AlterWorkflowStmt with at least 1 operation")
	}
	op, ok := stmt.Operations[0].(*ast.DropBoundaryEventOp)
	if !ok {
		t.Fatalf("Expected DropBoundaryEventOp, got %T", stmt.Operations[0])
	}
	if op.ActivityRef != "activity" {
		t.Errorf("ActivityRef = %q, want %q", op.ActivityRef, "activity")
	}
}

// TestAlterWorkflow_DropBoundaryEvent_NilGuard verifies that a malformed
// DROP BOUNDARY EVENT (activity ref missing) does not panic — it must return
// nil silently rather than dereferencing a nil AlterActivityRef context.
func TestAlterWorkflow_DropBoundaryEvent_NilGuard(t *testing.T) {
	// Intentionally omit the activity ref to trigger ANTLR error recovery.
	// The visitor must not panic; it is acceptable to get a parse error or to
	// produce 0 operations (the nil-guard returns nil from buildAlterWorkflowAction).
	input := `ALTER WORKFLOW M.WF DROP BOUNDARY EVENT ON ;`

	// Must not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic in visitor for malformed DROP BOUNDARY EVENT: %v", r)
		}
	}()
	Build(input) //nolint:errcheck
}

func TestAlterWorkflow_SetActivityDueDate(t *testing.T) {
	input := `ALTER WORKFLOW M.WF SET ACTIVITY 'Review' DUE DATE 'PT72H';`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	op := stmt.Operations[0].(*ast.SetActivityPropertyOp)
	if op.Property != "due_date" {
		t.Errorf("Expected Property 'DUE_DATE', got %q", op.Property)
	}
	if op.Value != "PT72H" {
		t.Errorf("Expected Value 'PT72H', got %q", op.Value)
	}
}
