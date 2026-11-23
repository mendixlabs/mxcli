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
	if ann.HasCaption() {
		fb.pendingAnnotations.Caption = ann.Caption
		fb.pendingAnnotations.CaptionSet = true
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
	// Missing until #1328: the parser read @excluded and applyAnnotations wrote
	// it, but this hand-written copy skipped it, so every excluded activity was
	// written enabled. TestMergeStatementAnnotationsCopiesEveryField guards the
	// next field added to ActivityAnnotations.
	if ann.Excluded {
		fb.pendingAnnotations.Excluded = true
	}
	if ann.Documentation != nil {
		fb.pendingAnnotations.Documentation = ann.Documentation
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
	if len(ann.CaseAnchors) > 0 {
		fb.pendingAnnotations.CaseAnchors = ann.CaseAnchors
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
	if ann.HasCaption() || ann.Color != "" || ann.Excluded || ann.Documentation != nil {
		for _, obj := range fb.objects {
			if obj.GetID() != activityID {
				continue
			}
			// @documentation (ako/mxcli#2328): the four kinds that store it.
			if ann.Documentation != nil {
				switch a := obj.(type) {
				case *microflows.ActionActivity:
					a.Documentation = *ann.Documentation
				case *microflows.ExclusiveSplit:
					a.Documentation = *ann.Documentation
				case *microflows.InheritanceSplit:
					a.Documentation = *ann.Documentation
				case *microflows.LoopedActivity:
					a.Documentation = *ann.Documentation
				}
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
				// independent of the expression/rule being evaluated. An
				// explicit `@caption ''` clears it, as Studio Pro can (#1254).
				if ann.HasCaption() {
					activity.Caption = ann.Caption
				}
			case *microflows.InheritanceSplit:
				if ann.Caption != "" {
					activity.Caption = ann.Caption
				}
				// No case for *microflows.LoopedActivity. A loop and a while build
				// one, and generated/metamodel -- the arbiter -- declares no Caption
				// on Microflows$LoopedActivity, so there is nowhere for the value to
				// go: the gen writer emits none and the reader can never populate
				// one. Assigning it here wrote a field nothing reads and contradicted
				// MDL042, which tells the author the caption is dropped. The
				// diagnostic is the whole of the support (mendixlabs/mxcli#1187).
			}

			break
		}
	}

	// @annotation — attach the notes. All of them: an activity can carry more
	// than one, and keeping only the last silently deleted the others (#1077).
	//
	// A stored note (`existing:`) takes no place in the stack: it is drawn
	// where it is, and its line is drawn after the fragment's own, so describe
	// counts it after them too (ako/mxcli#1695).
	i := 0
	for _, note := range ann.Notes {
		fb.attachAnnotation(note, activityID, i)
		if note.Existing == "" {
			i++
		}
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

// adoptCurves takes over the @curves a sub-builder recorded, for a caller that
// is taking over its flows. applyFlowCurves runs once, on the builder that owns
// the flows when the graph is complete, so a curve left behind on a loop body's
// or an error handler's builder was never applied: check and exec passed and
// the edge came out straight (mendixlabs/mxcli#991, mendixlabs/mxcli#1597).
// Every site that appends a sub-builder's flows calls this alongside.
func (fb *flowBuilder) adoptCurves(sub *flowBuilder) {
	for id, c := range sub.curveByOrigin {
		if fb.curveByOrigin == nil {
			fb.curveByOrigin = map[model.ID]*ast.FlowCurve{}
		}
		fb.curveByOrigin[id] = c
	}
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
	if fb.returnEndIDs == nil {
		fb.returnEndIDs = map[model.ID]bool{}
	}
	fb.returnEndIDs[endEvent.ID] = true
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
// targetHeight is the height of the element the note belongs to. A note is placed
// above that element's TOP EDGE, not a fixed distance above its centre: a loop is a
// box several hundred pixels tall, so the old centre-relative offset dropped its
// note inside the box, on top of the body it was describing. An ordinary activity
// is ActivityHeight tall and keeps the geometry it has always had.
func defaultAnnotationGeometry(activityPos model.Point, index int, targetHeight int) (model.Point, model.Size) {
	if targetHeight < ActivityHeight {
		targetHeight = ActivityHeight
	}
	above := targetHeight/2 - ActivityHeight/2
	return model.Point{
		X: activityPos.X,
		Y: activityPos.Y - above - 100 - index*(DefaultAnnotationSize.Height+10),
	}, DefaultAnnotationSize
}

// objectHeight is an element's stored height, defaulting to an activity's.
func objectHeight(obj microflows.MicroflowObject) int {
	if withSize, ok := obj.(interface{ GetSize() model.Size }); ok {
		if h := withSize.GetSize().Height; h > 0 {
			return h
		}
	}
	return ActivityHeight
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
	if note.Existing != "" {
		fb.attachStoredNote(note, activityID)
		return
	}
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
				"the first mention must carry the text, as @annotation(id: %s, text: '…')%s",
				note.Label, note.Label, fb.storedNoteHint())
			return
		}
	}

	var activityPos model.Point
	activityHeight := ActivityHeight
	for _, obj := range fb.objects {
		if obj.GetID() == activityID {
			activityPos = obj.GetPosition()
			activityHeight = objectHeight(obj)
			break
		}
	}
	pos, size := defaultAnnotationGeometry(activityPos, index, activityHeight)
	if note.Position != nil {
		pos = model.Point{X: note.Position.X, Y: note.Position.Y}
	}
	if note.Size != nil {
		size = model.Size{Width: note.Size.Width, Height: note.Size.Height}
	}

	fb.linkAnnotation(fb.newAnnotation(note, pos, size).ID, activityID)
}

// recordStatedNote remembers a note whose position the script states. A
// spliced fragment keeps such a note where it is instead of moving it with
// the fragment (MicroflowFragment.Pinned).
func (fb *flowBuilder) recordStatedNote(note ast.MicroflowAnnotation, id model.ID) {
	if note.Position == nil {
		return
	}
	if fb.statedNotes == nil {
		fb.statedNotes = map[model.ID]bool{}
	}
	fb.statedNotes[id] = true
}

// attachFreeAnnotation creates a free-floating Annotation not connected to any
// activity. A label on one is accepted and reused, so a note can be shared
// between the canvas and an activity.
func (fb *flowBuilder) attachFreeAnnotation(note ast.MicroflowAnnotation) {
	if note.Existing != "" {
		// A stored note is on the canvas already; the mention can only mean
		// attaching it, and there is nothing here to attach it to.
		fb.addError("%s", existingNoteUnattached(note.Existing))
		return
	}
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
	fb.recordStatedNote(note, annotation.ID)
	if note.Label != "" {
		if fb.annotationsByLabel == nil {
			fb.annotationsByLabel = map[string]*microflows.Annotation{}
		}
		fb.annotationsByLabel[note.Label] = annotation
	}
	return annotation
}

// orientAnnotationFlows picks, for every note line that has no sides yet, the
// side of the note that faces its element and the side of the element that
// faces the note (mendixlabs/mxcli#1598, case 2). It runs once every object has
// its final position, since a note is placed before rows and containers settle.
//
// Studio Pro's own drawings are the rule: of 523 distinct note lines in
// Studio Pro-authored apps, the element's side is top or bottom whenever the
// two boxes are vertically clear of each other, and left or right only when
// they overlap vertically — 98% agreement. Writing 2 -> 0 for every note drew
// a note below its activity with a line that leaves its own bottom edge.
func orientAnnotationFlows(objects []microflows.MicroflowObject, flows []*microflows.AnnotationFlow) {
	boxes := map[model.ID]canvasBox{}
	collectCanvasBoxes(objects, model.Point{}, boxes)
	for _, af := range flows {
		if af.Sides != nil {
			continue
		}
		note, ok1 := boxes[af.OriginID]
		target, ok2 := boxes[af.DestinationID]
		if !ok1 || !ok2 {
			continue
		}
		af.Sides = facingSides(note, target)
	}
}

// canvasBox is an object's box in absolute canvas coordinates.
type canvasBox struct{ left, top, right, bottom int }

// collectCanvasBoxes records the absolute box of every object, descending into
// loop bodies. A loop's children are positioned relative to the loop box's
// top-left corner, and a note outside a loop may be wired to an activity
// inside it, so comparing raw positions would mix two planes.
func collectCanvasBoxes(objects []microflows.MicroflowObject, origin model.Point, out map[model.ID]canvasBox) {
	for _, o := range objects {
		if o == nil {
			continue
		}
		p, sz := o.GetPosition(), objectSize(o)
		x, y := origin.X+p.X, origin.Y+p.Y
		out[o.GetID()] = canvasBox{x - sz.Width/2, y - sz.Height/2, x + sz.Width/2, y + sz.Height/2}
		if loop, ok := o.(*microflows.LoopedActivity); ok && loop.ObjectCollection != nil {
			collectCanvasBoxes(loop.ObjectCollection.Objects,
				model.Point{X: x - sz.Width/2, Y: y - sz.Height/2}, out)
		}
	}
}

// facingSides returns the sides at which a line from note to target attaches.
// The formula is microflows.FacingSides, which the splice also uses for a line
// it draws from a stored note (ako/mxcli#1695): two copies would drift.
func facingSides(note, target canvasBox) *microflows.ConnectionSides {
	s := microflows.FacingSides(
		microflows.CanvasBox{Left: note.left, Top: note.top, Right: note.right, Bottom: note.bottom},
		microflows.CanvasBox{Left: target.left, Top: target.top, Right: target.right, Bottom: target.bottom})
	return &s
}

func (fb *flowBuilder) linkAnnotation(annotationID, activityID model.ID) {
	fb.annotationFlows = append(fb.annotationFlows, &microflows.AnnotationFlow{
		BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:      annotationID,
		DestinationID: activityID,
	})
}
