// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func parseWorkflowCallAction(raw map[string]any) *microflows.WorkflowCallAction {
	action := &microflows.WorkflowCallAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.Workflow = extractString(raw["Workflow"])
	action.WorkflowContextVariable = extractString(raw["WorkflowContextVariable"])
	action.OutputVariableName = extractString(raw["OutputVariableName"])
	action.UseReturnVariable = extractBool(raw["UseReturnVariable"], false)
	return action
}

func parseGetWorkflowDataAction(raw map[string]any) *microflows.GetWorkflowDataAction {
	action := &microflows.GetWorkflowDataAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.OutputVariableName = extractString(raw["OutputVariableName"])
	action.Workflow = extractString(raw["Workflow"])
	action.WorkflowVariable = extractString(raw["WorkflowVariable"])
	return action
}

func parseGetWorkflowsAction(raw map[string]any) *microflows.GetWorkflowsAction {
	action := &microflows.GetWorkflowsAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.OutputVariableName = extractString(raw["OutputVariableName"])
	action.WorkflowContextVariableName = extractString(raw["WorkflowContextVariableName"])
	return action
}

func parseGetWorkflowActivityRecordsAction(raw map[string]any) *microflows.GetWorkflowActivityRecordsAction {
	action := &microflows.GetWorkflowActivityRecordsAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.OutputVariableName = extractString(raw["OutputVariableName"])
	action.WorkflowVariable = extractString(raw["WorkflowVariable"])
	return action
}

func parseWorkflowOperationAction(raw map[string]any) *microflows.WorkflowOperationAction {
	action := &microflows.WorkflowOperationAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))

	if opRaw, ok := raw["Operation"].(map[string]any); ok {
		action.Operation = parseWorkflowOperation(opRaw)
	}
	return action
}

func parseWorkflowOperation(raw map[string]any) microflows.WorkflowOperation {
	typeName := extractString(raw["$Type"])
	wfVar := extractString(raw["WorkflowVariable"])

	switch typeName {
	case "Microflows$AbortOperation":
		op := &microflows.AbortOperation{}
		op.ID = model.ID(extractBsonID(raw["$ID"]))
		op.WorkflowVariable = wfVar
		// Reason is a StringTemplate
		if reason, ok := raw["Reason"].(map[string]any); ok {
			op.Reason = extractString(reason["Text"])
		}
		return op
	case "Microflows$ContinueOperation":
		op := &microflows.ContinueOperation{}
		op.ID = model.ID(extractBsonID(raw["$ID"]))
		op.WorkflowVariable = wfVar
		return op
	case "Microflows$PauseOperation":
		op := &microflows.PauseOperation{}
		op.ID = model.ID(extractBsonID(raw["$ID"]))
		op.WorkflowVariable = wfVar
		return op
	case "Microflows$RestartOperation":
		op := &microflows.RestartOperation{}
		op.ID = model.ID(extractBsonID(raw["$ID"]))
		op.WorkflowVariable = wfVar
		return op
	case "Microflows$RetryOperation":
		op := &microflows.RetryOperation{}
		op.ID = model.ID(extractBsonID(raw["$ID"]))
		op.WorkflowVariable = wfVar
		return op
	case "Microflows$UnpauseOperation":
		op := &microflows.UnpauseOperation{}
		op.ID = model.ID(extractBsonID(raw["$ID"]))
		op.WorkflowVariable = wfVar
		return op
	}
	return nil
}

func parseSetTaskOutcomeAction(raw map[string]any) *microflows.SetTaskOutcomeAction {
	action := &microflows.SetTaskOutcomeAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.OutcomeValue = extractString(raw["OutcomeValue"])
	action.WorkflowTaskVariable = extractString(raw["WorkflowTaskVariable"])
	return action
}

func parseOpenUserTaskAction(raw map[string]any) *microflows.OpenUserTaskAction {
	action := &microflows.OpenUserTaskAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.UserTaskVariable = extractString(raw["UserTaskVariable"])
	return action
}

func parseNotifyWorkflowAction(raw map[string]any) *microflows.NotifyWorkflowAction {
	action := &microflows.NotifyWorkflowAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.OutputVariableName = extractString(raw["OutputVariableName"])
	action.WorkflowVariable = extractString(raw["WorkflowVariable"])
	return action
}

func parseOpenWorkflowAction(raw map[string]any) *microflows.OpenWorkflowAction {
	action := &microflows.OpenWorkflowAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.WorkflowVariable = extractString(raw["WorkflowVariable"])
	return action
}

func parseLockWorkflowAction(raw map[string]any) *microflows.LockWorkflowAction {
	action := &microflows.LockWorkflowAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.PauseAllWorkflows = extractBool(raw["PauseAllWorkflows"], false)

	if sel, ok := raw["WorkflowSelection"].(map[string]any); ok {
		selType := extractString(sel["$Type"])
		switch selType {
		case "Workflows$WorkflowDefinitionNameSelection":
			action.Workflow = extractString(sel["Workflow"])
		case "Workflows$WorkflowDefinitionObjectSelection":
			action.WorkflowVariable = extractString(sel["WorkflowDefinitionVariable"])
		}
	}
	return action
}

func parseUnlockWorkflowAction(raw map[string]any) *microflows.UnlockWorkflowAction {
	action := &microflows.UnlockWorkflowAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.ResumeAllPausedWorkflows = extractBool(raw["ResumeAllPausedWorkflows"], false)

	if sel, ok := raw["WorkflowSelection"].(map[string]any); ok {
		selType := extractString(sel["$Type"])
		switch selType {
		case "Workflows$WorkflowDefinitionNameSelection":
			action.Workflow = extractString(sel["Workflow"])
		case "Workflows$WorkflowDefinitionObjectSelection":
			action.WorkflowVariable = extractString(sel["WorkflowDefinitionVariable"])
		}
	}
	return action
}
