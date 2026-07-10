// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// workflowMicroflowActionToGen builds the workflow-related microflow action
// sub-elements (call workflow, get workflow data/records, workflow operation,
// set task outcome, open user task / workflow, notify, lock/unlock). Mirrors
// sdk/mpr/writer_microflow_workflow.go field-for-field. All are flat string/bool
// actions except WorkflowOperationAction (a polymorphic Operation sub-doc) and
// Lock/UnlockWorkflowAction (an optional WorkflowSelection sub-doc).
func workflowMicroflowActionToGen(act microflows.MicroflowAction) element.Element {
	switch a := act.(type) {
	case *microflows.WorkflowCallAction:
		g := newElem("Microflows$WorkflowCallAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addStr(g, "OutputVariableName", a.OutputVariableName)
		addBool(g, "UseReturnVariable", a.UseReturnVariable)
		addStr(g, "Workflow", a.Workflow)
		addStr(g, "WorkflowContextVariable", a.WorkflowContextVariable)
		return g
	case *microflows.GetWorkflowDataAction:
		g := newElem("Microflows$GetWorkflowDataAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addStr(g, "OutputVariableName", a.OutputVariableName)
		addStr(g, "Workflow", a.Workflow)
		addStr(g, "WorkflowVariable", a.WorkflowVariable)
		return g
	case *microflows.GetWorkflowsAction:
		g := newElem("Microflows$GetWorkflowsAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addStr(g, "OutputVariableName", a.OutputVariableName)
		addStr(g, "WorkflowContextVariableName", a.WorkflowContextVariableName)
		return g
	case *microflows.GetWorkflowActivityRecordsAction:
		g := newElem("Microflows$GetWorkflowActivityRecordsAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addStr(g, "OutputVariableName", a.OutputVariableName)
		addStr(g, "WorkflowVariable", a.WorkflowVariable)
		return g
	case *microflows.WorkflowOperationAction:
		g := newElem("Microflows$WorkflowOperationAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		if op := workflowOperationToGen(a.Operation); op != nil {
			addPart(g, "Operation", op)
		}
		return g
	case *microflows.SetTaskOutcomeAction:
		g := newElem("Microflows$SetTaskOutcomeAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addStr(g, "OutcomeValue", a.OutcomeValue)
		addStr(g, "WorkflowTaskVariable", a.WorkflowTaskVariable)
		return g
	case *microflows.OpenUserTaskAction:
		g := newElem("Microflows$OpenUserTaskAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addStr(g, "UserTaskVariable", a.UserTaskVariable)
		return g
	case *microflows.NotifyWorkflowAction:
		g := newElem("Microflows$NotifyWorkflowAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addStr(g, "OutputVariableName", a.OutputVariableName)
		addStr(g, "WorkflowVariable", a.WorkflowVariable)
		return g
	case *microflows.OpenWorkflowAction:
		g := newElem("Microflows$OpenWorkflowAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addStr(g, "WorkflowVariable", a.WorkflowVariable)
		return g
	case *microflows.LockWorkflowAction:
		g := newElem("Microflows$LockWorkflowAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addBool(g, "PauseAllWorkflows", a.PauseAllWorkflows)
		if !a.PauseAllWorkflows {
			addPart(g, "WorkflowSelection", workflowSelectionToGen(a.Workflow, a.WorkflowVariable))
		}
		return g
	case *microflows.UnlockWorkflowAction:
		g := newElem("Microflows$UnlockWorkflowAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addBool(g, "ResumeAllPausedWorkflows", a.ResumeAllPausedWorkflows)
		if !a.ResumeAllPausedWorkflows {
			addPart(g, "WorkflowSelection", workflowSelectionToGen(a.Workflow, a.WorkflowVariable))
		}
		return g
	default:
		return nil
	}
}

// workflowOperationToGen builds the polymorphic Operation sub-element of a
// WorkflowOperationAction. The abort operation carries a StringTemplate reason.
func workflowOperationToGen(op microflows.WorkflowOperation) element.Element {
	switch o := op.(type) {
	case *microflows.AbortOperation:
		g := newElem("Microflows$AbortOperation", string(o.ID))
		reason := newElem("Microflows$StringTemplate", "")
		addStr(reason, "Text", o.Reason)
		addPart(g, "Reason", reason)
		addStr(g, "WorkflowVariable", o.WorkflowVariable)
		return g
	case *microflows.ContinueOperation:
		g := newElem("Microflows$ContinueOperation", string(o.ID))
		addStr(g, "WorkflowVariable", o.WorkflowVariable)
		return g
	case *microflows.PauseOperation:
		g := newElem("Microflows$PauseOperation", string(o.ID))
		addStr(g, "WorkflowVariable", o.WorkflowVariable)
		return g
	case *microflows.RestartOperation:
		g := newElem("Microflows$RestartOperation", string(o.ID))
		addStr(g, "WorkflowVariable", o.WorkflowVariable)
		return g
	case *microflows.RetryOperation:
		g := newElem("Microflows$RetryOperation", string(o.ID))
		addStr(g, "WorkflowVariable", o.WorkflowVariable)
		return g
	case *microflows.UnpauseOperation:
		g := newElem("Microflows$UnpauseOperation", string(o.ID))
		addStr(g, "WorkflowVariable", o.WorkflowVariable)
		return g
	default:
		return nil
	}
}

// workflowSelectionToGen builds a Workflows$WorkflowDefinition*Selection: by name
// (a workflow qualified name) or by object (a workflow variable).
func workflowSelectionToGen(workflow, workflowVariable string) element.Element {
	if workflow != "" {
		g := newElem("Workflows$WorkflowDefinitionNameSelection", "")
		addStr(g, "Workflow", workflow)
		return g
	}
	g := newElem("Workflows$WorkflowDefinitionObjectSelection", "")
	addStr(g, "WorkflowDefinitionVariable", workflowVariable)
	return g
}
