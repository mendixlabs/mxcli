// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow graph: sequence flow constructors and error handler flows
package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// convertErrorHandlingType converts AST error handling type to SDK error handling type.
func convertErrorHandlingType(eh *ast.ErrorHandlingClause) microflows.ErrorHandlingType {
	if eh == nil {
		return microflows.ErrorHandlingTypeRollback
	}
	switch eh.Type {
	case ast.ErrorHandlingContinue:
		return microflows.ErrorHandlingTypeContinue
	case ast.ErrorHandlingRollback:
		return microflows.ErrorHandlingTypeRollback
	case ast.ErrorHandlingCustom:
		return microflows.ErrorHandlingTypeCustom
	case ast.ErrorHandlingCustomWithoutRollback:
		return microflows.ErrorHandlingTypeCustomWithoutRollback
	default:
		return microflows.ErrorHandlingTypeRollback
	}
}

// newErrorHandlerFlow creates a SequenceFlow with IsErrorHandler=true,
// connecting from the bottom of the source activity to the left of the error handler.
func newErrorHandlerFlow(originID, destinationID model.ID) *microflows.SequenceFlow {
	return &microflows.SequenceFlow{
		BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:                   originID,
		DestinationID:              destinationID,
		OriginConnectionIndex:      AnchorBottom,
		DestinationConnectionIndex: AnchorLeft,
		IsErrorHandler:             true,
	}
}

// addErrorHandlerFlow builds error handler activities from the given body statements,
// positions them below the source activity, and connects them with an error handler flow.
// Returns the last activity ID if the error handler should merge back to the main flow.
// Returns empty model.ID if the error handler terminates (via RAISE ERROR or RETURN).
func (fb *flowBuilder) addErrorHandlerFlow(sourceActivityID model.ID, sourceX int, errorBody []ast.MicroflowStatement) model.ID {
	if len(errorBody) == 0 {
		return ""
	}

	// Position error handler below the main flow
	errorY := fb.posY + VerticalSpacing
	errorX := sourceX

	// Build error handler activities
	errBuilder := &flowBuilder{
		posX:         errorX,
		posY:         errorY,
		baseY:        errorY,
		spacing:      HorizontalSpacing,
		varTypes:     fb.varTypes,
		declaredVars: fb.declaredVars,
		measurer:     fb.measurer,
		backend:      fb.backend,
		hierarchy:    fb.hierarchy,
		restServices: fb.restServices,
	}

	var lastErrID model.ID
	for _, stmt := range errorBody {
		actID := errBuilder.addStatement(stmt)
		if actID != "" {
			if lastErrID == "" {
				// Connect source activity to first error handler activity
				fb.flows = append(fb.flows, newErrorHandlerFlow(sourceActivityID, actID))
			} else {
				errBuilder.flows = append(errBuilder.flows, newHorizontalFlow(lastErrID, actID))
			}
			if errBuilder.nextConnectionPoint != "" {
				lastErrID = errBuilder.nextConnectionPoint
				errBuilder.nextConnectionPoint = ""
			} else {
				lastErrID = actID
			}
		}
	}

	// Append error handler objects and flows to the main builder
	fb.objects = append(fb.objects, errBuilder.objects...)
	fb.flows = append(fb.flows, errBuilder.flows...)

	// If the error handler ends with RAISE ERROR or RETURN, it terminates there.
	// Otherwise, return the last activity ID so caller can create a merge.
	if errBuilder.endsWithReturn {
		return "" // Error handler terminates, no merge needed
	}
	return lastErrID // Error handler should merge back to main flow
}

// handleErrorHandlerMerge creates an EndEvent for error handlers that want to merge back.
// This is a fallback until full merge support is implemented. Caller should pass
// the ID returned by addErrorHandlerFlow and the error handler Y position.
func (fb *flowBuilder) handleErrorHandlerMerge(lastErrID model.ID, activityID model.ID, errorY int) {
	if lastErrID == "" {
		return // No merge needed (error handler terminates with RETURN or RAISE ERROR)
	}
	// Error handler doesn't end with RETURN/RAISE — create EndEvent to terminate the path.
	// When the microflow has a return type, use the return value from a prior RETURN statement
	// if available to avoid "Return value required" errors. If no RETURN has been seen yet,
	// fall back to empty (works for void microflows).
	endEvent := &microflows.EndEvent{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: fb.posX, Y: errorY},
			Size:        model.Size{Width: EventSize, Height: EventSize},
		},
		ReturnValue: fb.returnValue,
	}
	fb.objects = append(fb.objects, endEvent)
	fb.flows = append(fb.flows, newHorizontalFlow(lastErrID, endEvent.ID))
}

// newHorizontalFlow creates a SequenceFlow with anchors for horizontal left-to-right connection
func newHorizontalFlow(originID, destinationID model.ID) *microflows.SequenceFlow {
	return &microflows.SequenceFlow{
		BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:                   originID,
		DestinationID:              destinationID,
		OriginConnectionIndex:      AnchorRight, // Connect from right side of origin
		DestinationConnectionIndex: AnchorLeft,  // Connect to left side of destination
	}
}

// newHorizontalFlowWithCase creates a horizontal SequenceFlow with a boolean case value (for splits)
func newHorizontalFlowWithCase(originID, destinationID model.ID, caseValue string) *microflows.SequenceFlow {
	flow := newHorizontalFlow(originID, destinationID)
	flow.CaseValue = microflows.EnumerationCase{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		Value:       caseValue, // "true" or "false" as string
	}
	return flow
}

// newDownwardFlowWithCase creates a SequenceFlow going down from origin (Bottom) to destination (Left)
// Used when TRUE path goes below the main line
func newDownwardFlowWithCase(originID, destinationID model.ID, caseValue string) *microflows.SequenceFlow {
	return &microflows.SequenceFlow{
		BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:                   originID,
		DestinationID:              destinationID,
		OriginConnectionIndex:      AnchorBottom, // Connect from bottom of origin (going down)
		DestinationConnectionIndex: AnchorLeft,   // Connect to left side of destination
		CaseValue: microflows.EnumerationCase{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Value:       caseValue, // "true" or "false" as string
		},
	}
}

// newUpwardFlow creates a SequenceFlow going up from origin (Right) to destination (Top)
// Used when returning from a lower branch to merge
func newUpwardFlow(originID, destinationID model.ID) *microflows.SequenceFlow {
	return &microflows.SequenceFlow{
		BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:                   originID,
		DestinationID:              destinationID,
		OriginConnectionIndex:      AnchorRight,  // Connect from right side of origin
		DestinationConnectionIndex: AnchorBottom, // Connect to bottom of destination (going up)
	}
}

// lastStmtIsReturn checks if the last statement in a body is a RETURN statement.
func lastStmtIsReturn(stmts []ast.MicroflowStatement) bool {
	if len(stmts) == 0 {
		return false
	}
	_, ok := stmts[len(stmts)-1].(*ast.ReturnStmt)
	return ok
}
