// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow graph: graph construction and statement dispatch
package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// buildFlowGraph converts AST statements to a Microflow flow graph.
// Note: posY represents the CENTER LINE for element alignment, not the top position.
func (fb *flowBuilder) buildFlowGraph(stmts []ast.MicroflowStatement, returns *ast.MicroflowReturnType) *microflows.MicroflowObjectCollection {
	// Initialize maps if not set
	if fb.measurer == nil {
		fb.measurer = &layoutMeasurer{varTypes: fb.varTypes}
	}
	if fb.declaredVars == nil {
		fb.declaredVars = make(map[string]string)
	}
	// Set return value expression for error handler EndEvents
	if returns != nil && returns.Variable != "" {
		fb.returnValue = "$" + returns.Variable
	}
	// Set baseY for branch restoration (this is the center line)
	fb.baseY = fb.posY

	// Pre-scan: if the first statement carries an @position annotation, shift the
	// StartEvent to be one spacing unit to the left of that position so it doesn't
	// end up behind activities that use explicit coordinates.
	for _, stmt := range stmts {
		if ann := getStatementAnnotations(stmt); ann != nil && ann.Position != nil {
			fb.posX = ann.Position.X - fb.spacing
			fb.posY = ann.Position.Y
			fb.baseY = fb.posY
			break
		}
	}

	// Create StartEvent - Position is the CENTER point (RelativeMiddlePoint in Mendix)
	startEvent := &microflows.StartEvent{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: fb.posX, Y: fb.posY},
			Size:        model.Size{Width: EventSize, Height: EventSize},
		},
	}
	fb.objects = append(fb.objects, startEvent)
	lastID := startEvent.ID

	fb.posX += fb.spacing

	// Process each statement
	// pendingCase holds the case value for the NEXT flow (set by merge-less splits)
	pendingCase := ""
	for _, stmt := range stmts {
		activityID := fb.addStatement(stmt)
		if activityID != "" {
			// If there are pending annotations, apply them to this activity
			if fb.pendingAnnotations != nil {
				fb.applyAnnotations(activityID, fb.pendingAnnotations)
				fb.pendingAnnotations = nil
			}
			// Connect to previous object with horizontal SequenceFlow
			if pendingCase != "" {
				fb.flows = append(fb.flows, newHorizontalFlowWithCase(lastID, activityID, pendingCase))
				pendingCase = ""
			} else {
				fb.flows = append(fb.flows, newHorizontalFlow(lastID, activityID))
			}
			// For compound statements (IF, LOOP), the exit point differs from entry point
			if fb.nextConnectionPoint != "" {
				lastID = fb.nextConnectionPoint
				fb.nextConnectionPoint = ""
				// Save nextFlowCase for the NEXT iteration's flow creation
				pendingCase = fb.nextFlowCase
				fb.nextFlowCase = ""
			} else {
				lastID = activityID
			}
		}
	}

	// Handle leftover pending annotations (free-floating annotation text)
	if fb.pendingAnnotations != nil {
		if fb.pendingAnnotations.AnnotationText != "" {
			fb.attachFreeAnnotation(fb.pendingAnnotations.AnnotationText)
		}
		fb.pendingAnnotations = nil
	}

	// Create EndEvent only if the flow doesn't already end with RETURN EndEvent(s)
	// (e.g., when both branches of an IF/ELSE end with RETURN, EndEvents are already created)
	if !fb.endsWithReturn {
		fb.posX += fb.spacing / 2
		fb.posY = fb.baseY // Ensure end event is on the happy path center line
		endEvent := &microflows.EndEvent{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: EventSize, Height: EventSize},
			},
			ReturnValue: fb.returnValue,
		}

		fb.objects = append(fb.objects, endEvent)

		// Connect last activity to end event
		if pendingCase != "" {
			fb.flows = append(fb.flows, newHorizontalFlowWithCase(lastID, endEvent.ID, pendingCase))
		} else {
			fb.flows = append(fb.flows, newHorizontalFlow(lastID, endEvent.ID))
		}
	}

	return &microflows.MicroflowObjectCollection{
		BaseElement:     model.BaseElement{ID: model.ID(types.GenerateID())},
		Objects:         fb.objects,
		Flows:           fb.flows,
		AnnotationFlows: fb.annotationFlows,
	}
}

// addStatement converts an AST statement to a microflow activity and returns its ID.
func (fb *flowBuilder) addStatement(stmt ast.MicroflowStatement) model.ID {
	// Extract annotations from the statement and merge into pendingAnnotations
	fb.mergeStatementAnnotations(stmt)

	// Apply @position before creating the activity so it's placed at the right position
	if fb.pendingAnnotations != nil && fb.pendingAnnotations.Position != nil {
		fb.posX = fb.pendingAnnotations.Position.X
		fb.posY = fb.pendingAnnotations.Position.Y
	}

	switch s := stmt.(type) {
	case *ast.DeclareStmt:
		return fb.addCreateVariableAction(s)
	case *ast.MfSetStmt:
		return fb.addChangeVariableAction(s)
	case *ast.ReturnStmt:
		return fb.addEndEventWithReturn(s)
	case *ast.RaiseErrorStmt:
		return fb.addErrorEvent()
	case *ast.LogStmt:
		return fb.addLogMessageAction(s)
	case *ast.CreateObjectStmt:
		return fb.addCreateObjectAction(s)
	case *ast.ChangeObjectStmt:
		return fb.addChangeObjectAction(s)
	case *ast.RetrieveStmt:
		return fb.addRetrieveAction(s)
	case *ast.MfCommitStmt:
		return fb.addCommitAction(s)
	case *ast.DeleteObjectStmt:
		return fb.addDeleteAction(s)
	case *ast.RollbackStmt:
		return fb.addRollbackAction(s)
	case *ast.IfStmt:
		return fb.addIfStatement(s)
	case *ast.LoopStmt:
		return fb.addLoopStatement(s)
	case *ast.WhileStmt:
		return fb.addWhileStatement(s)
	case *ast.ListOperationStmt:
		return fb.addListOperationAction(s)
	case *ast.AggregateListStmt:
		return fb.addAggregateListAction(s)
	case *ast.CreateListStmt:
		return fb.addCreateListAction(s)
	case *ast.AddToListStmt:
		return fb.addAddToListAction(s)
	case *ast.RemoveFromListStmt:
		return fb.addRemoveFromListAction(s)
	case *ast.CallMicroflowStmt:
		return fb.addCallMicroflowAction(s)
	case *ast.CallJavaActionStmt:
		return fb.addCallJavaActionAction(s)
	case *ast.ExecuteDatabaseQueryStmt:
		return fb.addExecuteDatabaseQueryAction(s)
	case *ast.CallExternalActionStmt:
		return fb.addCallExternalActionAction(s)
	case *ast.ShowPageStmt:
		return fb.addShowPageAction(s)
	case *ast.ClosePageStmt:
		return fb.addClosePageAction(s)
	case *ast.ShowHomePageStmt:
		return fb.addShowHomePageAction(s)
	case *ast.ShowMessageStmt:
		return fb.addShowMessageAction(s)
	case *ast.ValidationFeedbackStmt:
		return fb.addValidationFeedbackAction(s)
	case *ast.RestCallStmt:
		return fb.addRestCallAction(s)
	case *ast.SendRestRequestStmt:
		return fb.addSendRestRequestAction(s)
	case *ast.ImportFromMappingStmt:
		return fb.addImportFromMappingAction(s)
	case *ast.ExportToMappingStmt:
		return fb.addExportToMappingAction(s)
	case *ast.TransformJsonStmt:
		return fb.addTransformJsonAction(s)
	// Workflow microflow actions
	case *ast.CallWorkflowStmt:
		return fb.addCallWorkflowAction(s)
	case *ast.GetWorkflowDataStmt:
		return fb.addGetWorkflowDataAction(s)
	case *ast.GetWorkflowsStmt:
		return fb.addGetWorkflowsAction(s)
	case *ast.GetWorkflowActivityRecordsStmt:
		return fb.addGetWorkflowActivityRecordsAction(s)
	case *ast.WorkflowOperationStmt:
		return fb.addWorkflowOperationAction(s)
	case *ast.SetTaskOutcomeStmt:
		return fb.addSetTaskOutcomeAction(s)
	case *ast.OpenUserTaskStmt:
		return fb.addOpenUserTaskAction(s)
	case *ast.NotifyWorkflowStmt:
		return fb.addNotifyWorkflowAction(s)
	case *ast.OpenWorkflowStmt:
		return fb.addOpenWorkflowAction(s)
	case *ast.LockWorkflowStmt:
		return fb.addLockWorkflowAction(s)
	case *ast.UnlockWorkflowStmt:
		return fb.addUnlockWorkflowAction(s)
	default:
		// For now, skip unknown statement types
		return ""
	}
}
