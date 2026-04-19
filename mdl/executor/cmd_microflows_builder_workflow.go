// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// wrapAction wraps a MicroflowAction in an ActionActivity with standard positioning.
func (fb *flowBuilder) wrapAction(action microflows.MicroflowAction, errorHandling *ast.ErrorHandlingClause) model.ID {
	activityX := fb.posX
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}
	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	if errorHandling != nil && len(errorHandling.Body) > 0 {
		errorY := fb.posY + VerticalSpacing
		mergeID := fb.addErrorHandlerFlow(activity.ID, activityX, errorHandling.Body)
		fb.handleErrorHandlerMerge(mergeID, activity.ID, errorY)
	}
	return activity.ID
}

func (fb *flowBuilder) addCallWorkflowAction(s *ast.CallWorkflowStmt) model.ID {
	wfQN := s.Workflow.Module + "." + s.Workflow.Name
	ctxVar := ""
	if len(s.Arguments) > 0 {
		ctxVar = fb.exprToString(s.Arguments[0].Value)
		// Strip leading $ if present
		if len(ctxVar) > 0 && ctxVar[0] == '$' {
			ctxVar = ctxVar[1:]
		}
	}

	action := &microflows.WorkflowCallAction{
		BaseElement:             model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:       convertErrorHandlingType(s.ErrorHandling),
		Workflow:                wfQN,
		WorkflowContextVariable: ctxVar,
		OutputVariableName:      s.OutputVariable,
		UseReturnVariable:       s.OutputVariable != "",
	}
	return fb.wrapAction(action, s.ErrorHandling)
}

func (fb *flowBuilder) addGetWorkflowDataAction(s *ast.GetWorkflowDataStmt) model.ID {
	action := &microflows.GetWorkflowDataAction{
		BaseElement:        model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:  convertErrorHandlingType(s.ErrorHandling),
		OutputVariableName: s.OutputVariable,
		Workflow:           s.Workflow.Module + "." + s.Workflow.Name,
		WorkflowVariable:   s.WorkflowVariable,
	}
	return fb.wrapAction(action, s.ErrorHandling)
}

func (fb *flowBuilder) addGetWorkflowsAction(s *ast.GetWorkflowsStmt) model.ID {
	action := &microflows.GetWorkflowsAction{
		BaseElement:                 model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:           convertErrorHandlingType(s.ErrorHandling),
		OutputVariableName:          s.OutputVariable,
		WorkflowContextVariableName: s.WorkflowContextVariableName,
	}
	return fb.wrapAction(action, s.ErrorHandling)
}

func (fb *flowBuilder) addGetWorkflowActivityRecordsAction(s *ast.GetWorkflowActivityRecordsStmt) model.ID {
	action := &microflows.GetWorkflowActivityRecordsAction{
		BaseElement:        model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:  convertErrorHandlingType(s.ErrorHandling),
		OutputVariableName: s.OutputVariable,
		WorkflowVariable:   s.WorkflowVariable,
	}
	return fb.wrapAction(action, s.ErrorHandling)
}

func (fb *flowBuilder) addWorkflowOperationAction(s *ast.WorkflowOperationStmt) model.ID {
	var op microflows.WorkflowOperation
	switch s.OperationType {
	case "ABORT":
		reason := ""
		if s.Reason != nil {
			reason = fb.exprToString(s.Reason)
		}
		op = &microflows.AbortOperation{
			BaseElement:      model.BaseElement{ID: model.ID(types.GenerateID())},
			Reason:           reason,
			WorkflowVariable: s.WorkflowVariable,
		}
	case "CONTINUE":
		op = &microflows.ContinueOperation{
			BaseElement:      model.BaseElement{ID: model.ID(types.GenerateID())},
			WorkflowVariable: s.WorkflowVariable,
		}
	case "PAUSE":
		op = &microflows.PauseOperation{
			BaseElement:      model.BaseElement{ID: model.ID(types.GenerateID())},
			WorkflowVariable: s.WorkflowVariable,
		}
	case "RESTART":
		op = &microflows.RestartOperation{
			BaseElement:      model.BaseElement{ID: model.ID(types.GenerateID())},
			WorkflowVariable: s.WorkflowVariable,
		}
	case "RETRY":
		op = &microflows.RetryOperation{
			BaseElement:      model.BaseElement{ID: model.ID(types.GenerateID())},
			WorkflowVariable: s.WorkflowVariable,
		}
	case "UNPAUSE":
		op = &microflows.UnpauseOperation{
			BaseElement:      model.BaseElement{ID: model.ID(types.GenerateID())},
			WorkflowVariable: s.WorkflowVariable,
		}
	}

	action := &microflows.WorkflowOperationAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: convertErrorHandlingType(s.ErrorHandling),
		Operation:         op,
	}
	return fb.wrapAction(action, s.ErrorHandling)
}

func (fb *flowBuilder) addSetTaskOutcomeAction(s *ast.SetTaskOutcomeStmt) model.ID {
	action := &microflows.SetTaskOutcomeAction{
		BaseElement:          model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:    convertErrorHandlingType(s.ErrorHandling),
		OutcomeValue:         s.OutcomeValue,
		WorkflowTaskVariable: s.WorkflowTaskVariable,
	}
	return fb.wrapAction(action, s.ErrorHandling)
}

func (fb *flowBuilder) addOpenUserTaskAction(s *ast.OpenUserTaskStmt) model.ID {
	action := &microflows.OpenUserTaskAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: convertErrorHandlingType(s.ErrorHandling),
		UserTaskVariable:  s.UserTaskVariable,
	}
	return fb.wrapAction(action, s.ErrorHandling)
}

func (fb *flowBuilder) addNotifyWorkflowAction(s *ast.NotifyWorkflowStmt) model.ID {
	action := &microflows.NotifyWorkflowAction{
		BaseElement:        model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:  convertErrorHandlingType(s.ErrorHandling),
		OutputVariableName: s.OutputVariable,
		WorkflowVariable:   s.WorkflowVariable,
	}
	return fb.wrapAction(action, s.ErrorHandling)
}

func (fb *flowBuilder) addOpenWorkflowAction(s *ast.OpenWorkflowStmt) model.ID {
	action := &microflows.OpenWorkflowAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: convertErrorHandlingType(s.ErrorHandling),
		WorkflowVariable:  s.WorkflowVariable,
	}
	return fb.wrapAction(action, s.ErrorHandling)
}

func (fb *flowBuilder) addLockWorkflowAction(s *ast.LockWorkflowStmt) model.ID {
	action := &microflows.LockWorkflowAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: convertErrorHandlingType(s.ErrorHandling),
		PauseAllWorkflows: s.PauseAllWorkflows,
		WorkflowVariable:  s.WorkflowVariable,
	}
	return fb.wrapAction(action, s.ErrorHandling)
}

func (fb *flowBuilder) addUnlockWorkflowAction(s *ast.UnlockWorkflowStmt) model.ID {
	action := &microflows.UnlockWorkflowAction{
		BaseElement:              model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:        convertErrorHandlingType(s.ErrorHandling),
		ResumeAllPausedWorkflows: s.ResumeAllPausedWorkflows,
		WorkflowVariable:         s.WorkflowVariable,
	}
	return fb.wrapAction(action, s.ErrorHandling)
}
