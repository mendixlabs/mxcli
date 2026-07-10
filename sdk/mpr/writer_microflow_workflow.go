// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"

	"go.mongodb.org/mongo-driver/bson"
)

func serializeWorkflowCallAction(a *microflows.WorkflowCallAction) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$WorkflowCallAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "OutputVariableName", Value: a.OutputVariableName},
		{Key: "UseReturnVariable", Value: a.UseReturnVariable},
		{Key: "Workflow", Value: a.Workflow},
		{Key: "WorkflowContextVariable", Value: a.WorkflowContextVariable},
	}
}

func serializeGetWorkflowDataAction(a *microflows.GetWorkflowDataAction) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$GetWorkflowDataAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "OutputVariableName", Value: a.OutputVariableName},
		{Key: "Workflow", Value: a.Workflow},
		{Key: "WorkflowVariable", Value: a.WorkflowVariable},
	}
}

func serializeGetWorkflowsAction(a *microflows.GetWorkflowsAction) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$GetWorkflowsAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "OutputVariableName", Value: a.OutputVariableName},
		{Key: "WorkflowContextVariableName", Value: a.WorkflowContextVariableName},
	}
}

func serializeGetWorkflowActivityRecordsAction(a *microflows.GetWorkflowActivityRecordsAction) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$GetWorkflowActivityRecordsAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "OutputVariableName", Value: a.OutputVariableName},
		{Key: "WorkflowVariable", Value: a.WorkflowVariable},
	}
}

func serializeWorkflowOperationAction(a *microflows.WorkflowOperationAction) bson.D {
	var opDoc bson.D
	if a.Operation != nil {
		switch op := a.Operation.(type) {
		case *microflows.AbortOperation:
			reasonDoc := bson.D{
				{Key: "$ID", Value: idToBsonBinary(GenerateID())},
				{Key: "$Type", Value: "Microflows$StringTemplate"},
				{Key: "Text", Value: op.Reason},
			}
			opDoc = bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(op.ID))},
				{Key: "$Type", Value: "Microflows$AbortOperation"},
				{Key: "Reason", Value: reasonDoc},
				{Key: "WorkflowVariable", Value: op.WorkflowVariable},
			}
		case *microflows.ContinueOperation:
			opDoc = bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(op.ID))},
				{Key: "$Type", Value: "Microflows$ContinueOperation"},
				{Key: "WorkflowVariable", Value: op.WorkflowVariable},
			}
		case *microflows.PauseOperation:
			opDoc = bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(op.ID))},
				{Key: "$Type", Value: "Microflows$PauseOperation"},
				{Key: "WorkflowVariable", Value: op.WorkflowVariable},
			}
		case *microflows.RestartOperation:
			opDoc = bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(op.ID))},
				{Key: "$Type", Value: "Microflows$RestartOperation"},
				{Key: "WorkflowVariable", Value: op.WorkflowVariable},
			}
		case *microflows.RetryOperation:
			opDoc = bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(op.ID))},
				{Key: "$Type", Value: "Microflows$RetryOperation"},
				{Key: "WorkflowVariable", Value: op.WorkflowVariable},
			}
		case *microflows.UnpauseOperation:
			opDoc = bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(op.ID))},
				{Key: "$Type", Value: "Microflows$UnpauseOperation"},
				{Key: "WorkflowVariable", Value: op.WorkflowVariable},
			}
		}
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$WorkflowOperationAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "Operation", Value: opDoc},
	}
}

func serializeSetTaskOutcomeAction(a *microflows.SetTaskOutcomeAction) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$SetTaskOutcomeAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "OutcomeValue", Value: a.OutcomeValue},
		{Key: "WorkflowTaskVariable", Value: a.WorkflowTaskVariable},
	}
}

func serializeOpenUserTaskAction(a *microflows.OpenUserTaskAction) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$OpenUserTaskAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "UserTaskVariable", Value: a.UserTaskVariable},
	}
}

func serializeNotifyWorkflowAction(a *microflows.NotifyWorkflowAction) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$NotifyWorkflowAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "OutputVariableName", Value: a.OutputVariableName},
		{Key: "WorkflowVariable", Value: a.WorkflowVariable},
	}
}

func serializeOpenWorkflowAction(a *microflows.OpenWorkflowAction) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$OpenWorkflowAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "WorkflowVariable", Value: a.WorkflowVariable},
	}
}

func serializeLockWorkflowAction(a *microflows.LockWorkflowAction) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$LockWorkflowAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "PauseAllWorkflows", Value: a.PauseAllWorkflows},
	}
	if !a.PauseAllWorkflows {
		selDoc := serializeWorkflowSelection(a.Workflow, a.WorkflowVariable)
		doc = append(doc, bson.E{Key: "WorkflowSelection", Value: selDoc})
	}
	return doc
}

func serializeUnlockWorkflowAction(a *microflows.UnlockWorkflowAction) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$UnlockWorkflowAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "ResumeAllPausedWorkflows", Value: a.ResumeAllPausedWorkflows},
	}
	if !a.ResumeAllPausedWorkflows {
		selDoc := serializeWorkflowSelection(a.Workflow, a.WorkflowVariable)
		doc = append(doc, bson.E{Key: "WorkflowSelection", Value: selDoc})
	}
	return doc
}

func serializeWorkflowSelection(workflow, workflowVariable string) bson.D {
	if workflow != "" {
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "Workflows$WorkflowDefinitionNameSelection"},
			{Key: "Workflow", Value: workflow},
		}
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(GenerateID())},
		{Key: "$Type", Value: "Workflows$WorkflowDefinitionObjectSelection"},
		{Key: "WorkflowDefinitionVariable", Value: workflowVariable},
	}
}
