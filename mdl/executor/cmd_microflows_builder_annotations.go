// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow graph: annotation handling and terminal events
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// getStatementAnnotations extracts the annotations field from any microflow statement.
//
// Reflective (ast.StatementAnnotations), not a type switch: the switch this
// replaced had no case for any workflow or mapping statement, so the builder
// never saw their @position and auto-placed them instead.
func getStatementAnnotations(stmt ast.MicroflowStatement) *ast.ActivityAnnotations {
	return ast.StatementAnnotations(stmt)
}

// stmtOwnAnchor returns the primary FlowAnchors declared on this statement's
// @anchor annotation, or nil if absent. This is the flow-level anchor that
// applies to the single SequenceFlow leaving the statement (and whose To
// applies to the incoming flow when this statement is the destination).
func stmtOwnAnchor(stmt ast.MicroflowStatement) *ast.FlowAnchors {
	ann := getStatementAnnotations(stmt)
	if ann == nil {
		return nil
	}
	return ann.Anchor
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
	if len(ann.Notes) > 0 {
		fb.pendingAnnotations.Notes = append(fb.pendingAnnotations.Notes, ann.Notes...)
	}
	if len(ann.FreeNotes) > 0 {
		fb.pendingAnnotations.FreeNotes = append(fb.pendingAnnotations.FreeNotes, ann.FreeNotes...)
	}
	if ann.Anchor != nil {
		fb.pendingAnnotations.Anchor = ann.Anchor
	}
	if ann.Curve != nil {
		fb.pendingAnnotations.Curve = ann.Curve
	}
	if ann.Merge != nil {
		fb.pendingAnnotations.Merge = ann.Merge
	}
	if ann.TrueBranchAnchor != nil {
		fb.pendingAnnotations.TrueBranchAnchor = ann.TrueBranchAnchor
	}
	if ann.FalseBranchAnchor != nil {
		fb.pendingAnnotations.FalseBranchAnchor = ann.FalseBranchAnchor
	}
	if ann.IteratorAnchor != nil {
		fb.pendingAnnotations.IteratorAnchor = ann.IteratorAnchor
	}
	if ann.BodyTailAnchor != nil {
		fb.pendingAnnotations.BodyTailAnchor = ann.BodyTailAnchor
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

			switch activity := obj.(type) {
			case *microflows.ActionActivity:
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
			case *microflows.ExclusiveSplit:
				// Splits carry a human-readable Caption (e.g. "Right format?")
				// independent of the expression/rule being evaluated.
				if ann.Caption != "" {
					activity.Caption = ann.Caption
				}
			case *microflows.InheritanceSplit:
				if ann.Caption != "" {
					activity.Caption = ann.Caption
				}
			case *microflows.LoopedActivity:
				// LOOP / WHILE activities can carry a caption just like
				// splits and action activities.
				if ann.Caption != "" {
					activity.Caption = ann.Caption
				}
			}

			break
		}
	}

	// @annotation — attach the notes. All of them: an activity can carry more
	// than one, and keeping only the last silently deleted the others (#1077).
	for i, note := range ann.Notes {
		fb.attachAnnotation(note, activityID, i)
	}
}

func (fb *flowBuilder) applyPendingAnnotations(activityID model.ID) {
	if activityID == "" || fb.pendingAnnotations == nil {
		return
	}
	// Record the curve against the ACTIVITY, and apply it to that activity's
	// outgoing flows in one pass once the graph is complete (applyFlowCurves).
	//
	// The alternative — threading it alongside the anchor — would mean touching
	// every one of the seven-odd sites that create a flow (previousStmtAnchor,
	// nextFlowAnchor, the branch and loop variants), and missing one would
	// silently straighten that edge. This hook already runs at every activity,
	// so there is exactly one place to get right. (#884)
	if c := fb.pendingAnnotations.Curve; c != nil {
		if fb.curveByOrigin == nil {
			fb.curveByOrigin = map[model.ID]*ast.FlowCurve{}
		}
		fb.curveByOrigin[activityID] = c
	}
	fb.applyAnnotations(activityID, fb.pendingAnnotations)
	fb.pendingAnnotations = nil
}

// mergePosition returns where a split's implicit merge node goes: the @merge(x,
// y) the statement asked for, or the computed fallback.
//
// The statement's own @position belongs to the SPLIT, which is why the merge
// needs a separate annotation. Every site that creates a merge for a split reads
// through here, so the override cannot be honoured at one and ignored at
// another. (#884)
func mergePosition(ann *ast.ActivityAnnotations, computedX, computedY int) (int, int) {
	if ann == nil || ann.Merge == nil {
		return computedX, computedY
	}
	return ann.Merge.X, ann.Merge.Y
}

// applyFlowCurves stamps each recorded @curve onto the flows leaving that
// activity. Runs once, after the graph is built.
//
// A statement with several outgoing flows (a split) applies the same curve to
// all of them; @anchor's per-branch form has no curve equivalent yet, and
// silently curving only one branch would be worse than curving both.
func (fb *flowBuilder) applyFlowCurves() {
	if len(fb.curveByOrigin) == 0 {
		return
	}
	for _, f := range fb.flows {
		c, ok := fb.curveByOrigin[f.OriginID]
		if !ok || c == nil {
			continue
		}
		if c.From != nil {
			f.OriginControlVector = fmt.Sprintf("%d;%d", c.From.X, c.From.Y)
		}
		if c.To != nil {
			f.DestinationControlVector = fmt.Sprintf("%d;%d", c.To.X, c.To.Y)
		}
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
	fb.lastReturnEndID = endEvent.ID
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

// DefaultAnnotationSize is the note box Studio Pro creates, and what the writer
// uses when the script does not say (`@annotation 'text'` with no `size:`).
var DefaultAnnotationSize = model.Size{Width: 200, Height: 50}

// defaultAnnotationGeometry is where an unplaced note goes: above the activity it
// documents, stacked upwards when several share one activity so the boxes do not
// land on top of each other.
//
// The DESCRIBER calls this too, to decide whether to emit `position:`/`size:` at
// all —
// a value the writer re-derives is omitted, which is what keeps a round-tripped
// note on the short `@annotation 'text'` form and makes only a hand-placed note
// pay for its geometry.
//
// Both sides MUST go through here. Two copies of the formula drift, and the
// failure is silent in the worst way: the describer omits a position the builder
// then re-derives differently, so the note creeps further on every round trip.
// TestAnnotationGeometryDefaultIsSharedByBothSides pins that.
func defaultAnnotationGeometry(activityPos model.Point, index int) (model.Point, model.Size) {
	return model.Point{X: activityPos.X, Y: activityPos.Y - 100 - index*(DefaultAnnotationSize.Height+10)}, DefaultAnnotationSize
}

// attachAnnotation attaches one note to an activity.
//
// A LABELLED note is created once and reused: `@annotation(id: n1, text: '…')`
// followed by `@annotation(id: n1)` on another activity yields ONE Annotation
// with two AnnotationFlows, which is how Mendix stores a note wired to several
// activities. Minting a fresh Annotation per mention is what duplicated the
// reporter's note on every round trip (#1077).
//
// index is the note's ordinal among those attached to this activity, used only
// to place unpositioned notes so they stack instead of overlapping.
func (fb *flowBuilder) attachAnnotation(note ast.MicroflowAnnotation, activityID model.ID, index int) {
	if note.Label != "" {
		if existing, ok := fb.annotationsByLabel[note.Label]; ok {
			if note.Text != "" && note.Text != existing.Caption {
				fb.addError("annotation id '%s' is declared twice with different text (%q, then %q) — "+
					"an id names ONE note; drop the id from the second one to make it a separate note",
					note.Label, existing.Caption, note.Text)
				return
			}
			fb.linkAnnotation(existing.ID, activityID)
			return
		}
		if note.Text == "" {
			fb.addError("@annotation(id: %s) refers to an annotation that has not been declared — "+
				"the first mention must carry the text, as @annotation(id: %s, text: '…')",
				note.Label, note.Label)
			return
		}
	}

	var activityPos model.Point
	for _, obj := range fb.objects {
		if obj.GetID() == activityID {
			activityPos = obj.GetPosition()
			break
		}
	}
	pos, size := defaultAnnotationGeometry(activityPos, index)
	if note.Position != nil {
		pos = model.Point{X: note.Position.X, Y: note.Position.Y}
	}
	if note.Size != nil {
		size = model.Size{Width: note.Size.Width, Height: note.Size.Height}
	}

	fb.linkAnnotation(fb.newAnnotation(note, pos, size).ID, activityID)
}

// attachFreeAnnotation creates a free-floating Annotation not connected to any
// activity. A label on one is accepted and reused, so a note can be shared
// between the canvas and an activity.
func (fb *flowBuilder) attachFreeAnnotation(note ast.MicroflowAnnotation) {
	if note.Label != "" {
		if _, ok := fb.annotationsByLabel[note.Label]; ok {
			// Already created; a free mention adds no flow, so there is
			// nothing left to do.
			return
		}
	}
	pos := model.Point{X: fb.posX, Y: fb.posY - 100}
	if note.Position != nil {
		pos = model.Point{X: note.Position.X, Y: note.Position.Y}
	}
	size := DefaultAnnotationSize
	if note.Size != nil {
		size = model.Size{Width: note.Size.Width, Height: note.Size.Height}
	}
	fb.newAnnotation(note, pos, size)
}

// newAnnotation creates the Annotation object and, when the note is labelled,
// records it so a later mention of the same id attaches to THIS one instead of
// creating another.
func (fb *flowBuilder) newAnnotation(note ast.MicroflowAnnotation, pos model.Point, size model.Size) *microflows.Annotation {
	annotation := &microflows.Annotation{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    pos,
			Size:        size,
		},
		Caption: note.Text,
	}
	fb.objects = append(fb.objects, annotation)
	if note.Label != "" {
		if fb.annotationsByLabel == nil {
			fb.annotationsByLabel = map[string]*microflows.Annotation{}
		}
		fb.annotationsByLabel[note.Label] = annotation
	}
	return annotation
}

func (fb *flowBuilder) linkAnnotation(annotationID, activityID model.ID) {
	fb.annotationFlows = append(fb.annotationFlows, &microflows.AnnotationFlow{
		BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:      annotationID,
		DestinationID: activityID,
	})
}
