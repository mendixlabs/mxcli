// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

func buildCallWorkflowStatement(ctx parser.ICallWorkflowStatementContext) *ast.CallWorkflowStmt {
	if ctx == nil {
		return nil
	}
	c := ctx.(*parser.CallWorkflowStatementContext)
	stmt := &ast.CallWorkflowStmt{}

	if v := c.VARIABLE(); v != nil {
		stmt.OutputVariable = strings.TrimPrefix(v.GetText(), "$")
	}
	if qn := c.QualifiedName(); qn != nil {
		stmt.Workflow = buildQualifiedName(qn)
	}
	if argList := c.CallArgumentList(); argList != nil {
		stmt.Arguments = buildCallArgumentList(argList)
	}
	if errClause := c.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}

func buildGetWorkflowDataStatement(ctx parser.IGetWorkflowDataStatementContext) *ast.GetWorkflowDataStmt {
	if ctx == nil {
		return nil
	}
	c := ctx.(*parser.GetWorkflowDataStatementContext)
	stmt := &ast.GetWorkflowDataStmt{}

	vars := c.AllVARIABLE()
	// First VARIABLE is the output variable (if there's an EQUALS), last is the workflow variable
	if c.EQUALS() != nil && len(vars) >= 2 {
		stmt.OutputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
		stmt.WorkflowVariable = strings.TrimPrefix(vars[1].GetText(), "$")
	} else if len(vars) >= 1 {
		stmt.WorkflowVariable = strings.TrimPrefix(vars[0].GetText(), "$")
	}
	if qn := c.QualifiedName(); qn != nil {
		stmt.Workflow = buildQualifiedName(qn)
	}
	if errClause := c.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}

func buildGetWorkflowsStatement(ctx parser.IGetWorkflowsStatementContext) *ast.GetWorkflowsStmt {
	if ctx == nil {
		return nil
	}
	c := ctx.(*parser.GetWorkflowsStatementContext)
	stmt := &ast.GetWorkflowsStmt{}

	vars := c.AllVARIABLE()
	if c.EQUALS() != nil && len(vars) >= 2 {
		stmt.OutputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
		stmt.WorkflowContextVariableName = strings.TrimPrefix(vars[1].GetText(), "$")
	} else if len(vars) >= 1 {
		stmt.WorkflowContextVariableName = strings.TrimPrefix(vars[0].GetText(), "$")
	}
	if errClause := c.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}

func buildGetWorkflowActivityRecordsStatement(ctx parser.IGetWorkflowActivityRecordsStatementContext) *ast.GetWorkflowActivityRecordsStmt {
	if ctx == nil {
		return nil
	}
	c := ctx.(*parser.GetWorkflowActivityRecordsStatementContext)
	stmt := &ast.GetWorkflowActivityRecordsStmt{}

	vars := c.AllVARIABLE()
	if c.EQUALS() != nil && len(vars) >= 2 {
		stmt.OutputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
		stmt.WorkflowVariable = strings.TrimPrefix(vars[1].GetText(), "$")
	} else if len(vars) >= 1 {
		stmt.WorkflowVariable = strings.TrimPrefix(vars[0].GetText(), "$")
	}
	if errClause := c.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}

func buildWorkflowOperationStatement(ctx parser.IWorkflowOperationStatementContext) *ast.WorkflowOperationStmt {
	if ctx == nil {
		return nil
	}
	c := ctx.(*parser.WorkflowOperationStatementContext)
	stmt := &ast.WorkflowOperationStmt{}

	if opType := c.WorkflowOperationType(); opType != nil {
		ot := opType.(*parser.WorkflowOperationTypeContext)
		v := ot.VARIABLE()
		if v != nil {
			stmt.WorkflowVariable = strings.TrimPrefix(v.GetText(), "$")
		}

		if ot.ABORT() != nil {
			stmt.OperationType = "abort"
			if expr := ot.Expression(); expr != nil {
				stmt.Reason = buildExpression(expr)
			}
		} else if ot.CONTINUE() != nil {
			stmt.OperationType = "continue"
		} else if ot.PAUSE() != nil {
			stmt.OperationType = "pause"
		} else if ot.RESTART() != nil {
			stmt.OperationType = "restart"
		} else if ot.RETRY() != nil {
			stmt.OperationType = "retry"
		} else if ot.UNPAUSE() != nil {
			stmt.OperationType = "unpause"
		}
	}
	if errClause := c.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}

func buildSetTaskOutcomeStatement(ctx parser.ISetTaskOutcomeStatementContext) *ast.SetTaskOutcomeStmt {
	if ctx == nil {
		return nil
	}
	c := ctx.(*parser.SetTaskOutcomeStatementContext)
	stmt := &ast.SetTaskOutcomeStmt{}

	if v := c.VARIABLE(); v != nil {
		stmt.WorkflowTaskVariable = strings.TrimPrefix(v.GetText(), "$")
	}
	if str := c.STRING_LITERAL(); str != nil {
		stmt.OutcomeValue = unquoteString(str.GetText())
	}
	if errClause := c.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}

func buildOpenUserTaskStatement(ctx parser.IOpenUserTaskStatementContext) *ast.OpenUserTaskStmt {
	if ctx == nil {
		return nil
	}
	c := ctx.(*parser.OpenUserTaskStatementContext)
	stmt := &ast.OpenUserTaskStmt{}

	if v := c.VARIABLE(); v != nil {
		stmt.UserTaskVariable = strings.TrimPrefix(v.GetText(), "$")
	}
	if errClause := c.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}

func buildNotifyWorkflowStatement(ctx parser.INotifyWorkflowStatementContext) *ast.NotifyWorkflowStmt {
	if ctx == nil {
		return nil
	}
	c := ctx.(*parser.NotifyWorkflowStatementContext)
	stmt := &ast.NotifyWorkflowStmt{}

	vars := c.AllVARIABLE()
	if c.EQUALS() != nil && len(vars) >= 2 {
		stmt.OutputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
		stmt.WorkflowVariable = strings.TrimPrefix(vars[1].GetText(), "$")
	} else if len(vars) >= 1 {
		stmt.WorkflowVariable = strings.TrimPrefix(vars[0].GetText(), "$")
	}
	if errClause := c.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}

func buildOpenWorkflowStatement(ctx parser.IOpenWorkflowStatementContext) *ast.OpenWorkflowStmt {
	if ctx == nil {
		return nil
	}
	c := ctx.(*parser.OpenWorkflowStatementContext)
	stmt := &ast.OpenWorkflowStmt{}

	if v := c.VARIABLE(); v != nil {
		stmt.WorkflowVariable = strings.TrimPrefix(v.GetText(), "$")
	}
	if errClause := c.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}

func buildLockWorkflowStatement(ctx parser.ILockWorkflowStatementContext) *ast.LockWorkflowStmt {
	if ctx == nil {
		return nil
	}
	c := ctx.(*parser.LockWorkflowStatementContext)
	stmt := &ast.LockWorkflowStmt{}

	if c.ALL() != nil {
		stmt.PauseAllWorkflows = true
	} else if v := c.VARIABLE(); v != nil {
		stmt.WorkflowVariable = strings.TrimPrefix(v.GetText(), "$")
	}
	if errClause := c.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}

func buildUnlockWorkflowStatement(ctx parser.IUnlockWorkflowStatementContext) *ast.UnlockWorkflowStmt {
	if ctx == nil {
		return nil
	}
	c := ctx.(*parser.UnlockWorkflowStatementContext)
	stmt := &ast.UnlockWorkflowStmt{}

	if c.ALL() != nil {
		stmt.ResumeAllPausedWorkflows = true
	} else if v := c.VARIABLE(); v != nil {
		stmt.WorkflowVariable = strings.TrimPrefix(v.GetText(), "$")
	}
	if errClause := c.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}
