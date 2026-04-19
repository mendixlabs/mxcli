// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow graph: annotation handling and terminal events
package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// getStatementAnnotations extracts the annotations field from any microflow statement.
func getStatementAnnotations(stmt ast.MicroflowStatement) *ast.ActivityAnnotations {
	switch s := stmt.(type) {
	case *ast.DeclareStmt:
		return s.Annotations
	case *ast.MfSetStmt:
		return s.Annotations
	case *ast.ReturnStmt:
		return s.Annotations
	case *ast.RaiseErrorStmt:
		return s.Annotations
	case *ast.CreateObjectStmt:
		return s.Annotations
	case *ast.ChangeObjectStmt:
		return s.Annotations
	case *ast.MfCommitStmt:
		return s.Annotations
	case *ast.DeleteObjectStmt:
		return s.Annotations
	case *ast.RollbackStmt:
		return s.Annotations
	case *ast.RetrieveStmt:
		return s.Annotations
	case *ast.IfStmt:
		return s.Annotations
	case *ast.LoopStmt:
		return s.Annotations
	case *ast.LogStmt:
		return s.Annotations
	case *ast.CallMicroflowStmt:
		return s.Annotations
	case *ast.CallJavaActionStmt:
		return s.Annotations
	case *ast.ExecuteDatabaseQueryStmt:
		return s.Annotations
	case *ast.CallExternalActionStmt:
		return s.Annotations
	case *ast.BreakStmt:
		return s.Annotations
	case *ast.ContinueStmt:
		return s.Annotations
	case *ast.ListOperationStmt:
		return s.Annotations
	case *ast.AggregateListStmt:
		return s.Annotations
	case *ast.CreateListStmt:
		return s.Annotations
	case *ast.AddToListStmt:
		return s.Annotations
	case *ast.RemoveFromListStmt:
		return s.Annotations
	case *ast.ShowPageStmt:
		return s.Annotations
	case *ast.ClosePageStmt:
		return s.Annotations
	case *ast.ShowHomePageStmt:
		return s.Annotations
	case *ast.ShowMessageStmt:
		return s.Annotations
	case *ast.ValidationFeedbackStmt:
		return s.Annotations
	case *ast.RestCallStmt:
		return s.Annotations
	default:
		return nil
	}
}

// mergeStatementAnnotations extracts annotations from a statement and merges into pendingAnnotations.
func (fb *flowBuilder) mergeStatementAnnotations(stmt ast.MicroflowStatement) {
	ann := getStatementAnnotations(stmt)
	if ann == nil {
		return
	}
	if fb.pendingAnnotations == nil {
		fb.pendingAnnotations = &ast.ActivityAnnotations{}
	}
	if ann.Position != nil {
		fb.pendingAnnotations.Position = ann.Position
	}
	if ann.Caption != "" {
		fb.pendingAnnotations.Caption = ann.Caption
	}
	if ann.Color != "" {
		fb.pendingAnnotations.Color = ann.Color
	}
	if ann.AnnotationText != "" {
		fb.pendingAnnotations.AnnotationText = ann.AnnotationText
	}
}

// applyAnnotations applies pending annotations to the activity identified by activityID.
// Note: @position is already applied before the activity is created (in addStatement),
// so this method only handles @caption, @color, and @annotation.
func (fb *flowBuilder) applyAnnotations(activityID model.ID, ann *ast.ActivityAnnotations) {
	if ann == nil {
		return
	}

	// Find the object by ID for @caption, @color, and @excluded
	if ann.Caption != "" || ann.Color != "" || ann.Excluded {
		for _, obj := range fb.objects {
			if obj.GetID() != activityID {
				continue
			}

			// @caption, @color, and @excluded — only applicable to ActionActivity
			if activity, ok := obj.(*microflows.ActionActivity); ok {
				if ann.Caption != "" {
					activity.Caption = ann.Caption
					activity.AutoGenerateCaption = false
				}
				if ann.Color != "" {
					activity.BackgroundColor = ann.Color
				}
				if ann.Excluded {
					activity.Disabled = true
				}
			}

			break
		}
	}

	// @annotation — attach an annotation object
	if ann.AnnotationText != "" {
		fb.attachAnnotation(ann.AnnotationText, activityID)
	}
}

// addEndEventWithReturn creates an EndEvent with the specified return value.
// This produces an actual EndEvent activity in the flow graph, allowing RETURN
// to work correctly inside IF/ELSE branches and error handler bodies.
func (fb *flowBuilder) addEndEventWithReturn(s *ast.ReturnStmt) model.ID {
	retVal := ""
	if s.Value != nil {
		retVal = fb.exprToString(s.Value)
	}

	endEvent := &microflows.EndEvent{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: fb.posX, Y: fb.posY},
			Size:        model.Size{Width: EventSize, Height: EventSize},
		},
		ReturnValue: retVal,
	}

	fb.objects = append(fb.objects, endEvent)
	fb.endsWithReturn = true
	fb.posX += fb.spacing / 2
	return endEvent.ID
}

// addErrorEvent creates an ErrorEvent to terminate the flow with an error.
// Used by RAISE ERROR statement in custom error handlers.
func (fb *flowBuilder) addErrorEvent() model.ID {
	errorEvent := &microflows.ErrorEvent{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: fb.posX, Y: fb.posY},
			Size:        model.Size{Width: EventSize, Height: EventSize},
		},
	}

	fb.objects = append(fb.objects, errorEvent)
	fb.endsWithReturn = true // Mark as terminated (no merge needed)
	fb.posX += fb.spacing / 2
	return errorEvent.ID
}

// attachAnnotation creates an Annotation object positioned above the given activity
// and connects them with an AnnotationFlow.
func (fb *flowBuilder) attachAnnotation(text string, activityID model.ID) {
	// Find the activity's position to place annotation above it
	var actX, actY int
	for _, obj := range fb.objects {
		if obj.GetID() == activityID {
			pos := obj.GetPosition()
			actX = pos.X
			actY = pos.Y
			break
		}
	}

	annotation := &microflows.Annotation{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: actX, Y: actY - 100},
			Size:        model.Size{Width: 200, Height: 50},
		},
		Caption: text,
	}
	fb.objects = append(fb.objects, annotation)

	fb.annotationFlows = append(fb.annotationFlows, &microflows.AnnotationFlow{
		BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:      annotation.ID,
		DestinationID: activityID,
	})
}

// attachFreeAnnotation creates a free-floating Annotation not connected to any activity.
func (fb *flowBuilder) attachFreeAnnotation(text string) {
	annotation := &microflows.Annotation{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: fb.posX, Y: fb.posY - 100},
			Size:        model.Size{Width: 200, Height: 50},
		},
		Caption: text,
	}
	fb.objects = append(fb.objects, annotation)
}
