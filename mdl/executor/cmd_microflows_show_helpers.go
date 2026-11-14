// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow traversal and helper functions.
package executor

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// describedAnnotation is one Annotation object as the describer sees it: its
// text, its canvas geometry — and its IDENTITY, which is the part that used to
// be thrown away here. A note wired to N activities was filed under each target
// as a bare caption, so DESCRIBE emitted N copies and re-executing the output
// created N notes (mendixlabs/mxcli#1077).
type describedAnnotation struct {
	ID       model.ID
	Caption  string
	Position model.Point
	Size     model.Size

	// Shared is true when more than one AnnotationFlow points at this note.
	// Only a shared note needs a label, so the everyday `@annotation 'text'`
	// form is left exactly as it was.
	Shared bool

	// Unstated names, for a note described as free, each link of it that MDL
	// has no form for (to the start event, to another note).
	Unstated []string
}

// annotationEmitter renders the @annotation lines for a describe traversal.
//
// It is stateful on purpose: WHICH mention of a shared note carries the
// declaration and which are references depends on traversal order, and that is
// only known while walking. One emitter per describe call — never a package
// global, so concurrent describes (captureDescribeParallel) stay independent.
type annotationEmitter struct {
	byTarget map[model.ID][]describedAnnotation

	// labels and nextLabel are SHARED by reference with any overlay emitter
	// (see withOverlay), because a note may be declared in the parent
	// collection and referenced from inside a loop body.
	labels    map[model.ID]string
	nextLabel *int

	// layout selects the layout annotations a canonical DESCRIBE keeps: the
	// authored ones, never the ones the layout engine would derive anyway (see
	// derivedFlowLayout). nil keeps every one, which is what the tools that
	// read geometry back — `layout flows`, the ELK view, diff — rely on.
	layout *flowLayoutKeep

	// ctx is the describe's context, for the language its text is quoted in
	// (mdlQuote). nil quotes in the frozen language.
	ctx *ExecContext
}

// quote writes a note's or caption's text as a string literal (mdlQuote).
func (e *annotationEmitter) quote(s string) string {
	if e == nil {
		return mdlQuote(nil, s)
	}
	return mdlQuote(e.ctx, s)
}

// buildAnnotationsByTarget joins AnnotationFlows (destination → activity) with
// the Annotation objects they originate from, keeping each note's identity so a
// shared one can be emitted once and referenced afterwards.
func buildAnnotationsByTarget(oc *microflows.MicroflowObjectCollection) *annotationEmitter {
	zero := 0
	e := &annotationEmitter{
		byTarget:  make(map[model.ID][]describedAnnotation),
		labels:    make(map[model.ID]string),
		nextLabel: &zero,
	}
	if oc == nil {
		return e
	}

	notes := make(map[model.ID]*microflows.Annotation)
	collectAnnotationObjects(oc, notes)

	targetCount := make(map[model.ID]int)
	for _, af := range oc.AnnotationFlows {
		targetCount[af.OriginID]++
	}

	for _, af := range oc.AnnotationFlows {
		note, ok := notes[af.OriginID]
		if !ok || note.Caption == "" {
			continue
		}
		e.byTarget[af.DestinationID] = append(e.byTarget[af.DestinationID], describedAnnotation{
			ID:       note.ID,
			Caption:  note.Caption,
			Position: note.Position,
			Size:     note.Size,
			Shared:   targetCount[af.OriginID] > 1,
		})
	}

	return e
}

func collectAnnotationObjects(oc *microflows.MicroflowObjectCollection, out map[model.ID]*microflows.Annotation) {
	if oc == nil {
		return
	}
	for _, obj := range oc.Objects {
		if annot, ok := obj.(*microflows.Annotation); ok {
			out[annot.ID] = annot
			continue
		}
		if loop, ok := obj.(*microflows.LoopedActivity); ok {
			collectAnnotationObjects(loop.ObjectCollection, out)
		}
	}
}

// withOverlay returns an emitter covering both this emitter's targets and the
// overlay's, sharing the label state by reference so a note declared in the
// parent collection and referenced inside a loop body keeps one label.
//
// Neither input map is mutated. When one side is empty the other's map is
// returned by reference (no copy), which the current callers — emitLoopBody
// with a freshly built overlay — never write to.
func (e *annotationEmitter) withOverlay(overlay *annotationEmitter) *annotationEmitter {
	if e == nil {
		return overlay
	}
	if overlay == nil || len(overlay.byTarget) == 0 {
		return e
	}
	out := &annotationEmitter{labels: e.labels, nextLabel: e.nextLabel, layout: e.layout, ctx: e.ctx}
	if len(e.byTarget) == 0 {
		out.byTarget = overlay.byTarget
		return out
	}
	out.byTarget = make(map[model.ID][]describedAnnotation, len(e.byTarget)+len(overlay.byTarget))
	for id, notes := range e.byTarget {
		out.byTarget[id] = notes
	}
	for id, notes := range overlay.byTarget {
		out.byTarget[id] = append(out.byTarget[id], notes...)
	}
	return out
}

// labelFor returns the emitted label for a note and whether this is its first
// mention (the one that must carry the text).
func (e *annotationEmitter) labelFor(id model.ID) (label string, first bool) {
	if existing, ok := e.labels[id]; ok {
		return existing, false
	}
	// Lazily initialised so a hand-built emitter (tests construct one with just
	// byTarget) cannot panic on a shared note.
	if e.labels == nil {
		e.labels = map[model.ID]string{}
	}
	if e.nextLabel == nil {
		e.nextLabel = new(int)
	}
	*e.nextLabel++
	label = fmt.Sprintf("n%d", *e.nextLabel)
	e.labels[id] = label
	return label, true
}

// lines renders the @annotation lines for one activity.
//
// The short form is kept wherever it is lossless: an unshared note sitting at
// the position and size the writer would re-derive emits `@annotation 'text'`,
// exactly as before. Only a note that is shared, or that has been moved or
// resized on the canvas, pays for the longer form — so this fix does not churn
// the output of every microflow that has a note in it.
func (e *annotationEmitter) lines(target model.ID, activityPos model.Point, targetHeight int, indentStr string) []string {
	if e == nil {
		return nil
	}
	var out []string
	for i, note := range e.byTarget[target] {
		if note.Shared {
			label, first := e.labelFor(note.ID)
			if !first {
				out = append(out, indentStr+fmt.Sprintf("@annotation(id: %s)", label))
				continue
			}
			// A shared note's geometry is always spelled out. Its default
			// depends on which mention created it, and pinning that down would
			// couple the two sides far more tightly than it is worth.
			out = append(out, indentStr+fmt.Sprintf("@annotation(id: %s, text: %s, position: (%d, %d), size: (%d, %d))",
				label, e.quote(note.Caption), note.Position.X, note.Position.Y, note.Size.Width, note.Size.Height))
			continue
		}

		defPos, defSize := defaultAnnotationGeometry(activityPos, i, targetHeight)
		var params []string
		if note.Position != defPos {
			params = append(params, fmt.Sprintf("position: (%d, %d)", note.Position.X, note.Position.Y))
		}
		if note.Size != defSize && note.Size != (model.Size{}) {
			params = append(params, fmt.Sprintf("size: (%d, %d)", note.Size.Width, note.Size.Height))
		}
		if len(params) == 0 {
			out = append(out, indentStr+fmt.Sprintf("@annotation %s", e.quote(note.Caption)))
			continue
		}
		out = append(out, indentStr+fmt.Sprintf("@annotation(text: %s, %s)",
			e.quote(note.Caption), strings.Join(params, ", ")))
	}
	return out
}

// collectFreeAnnotations returns the annotations the describer has no activity
// to print under — notes that sit on the canvas documenting the flow as a
// whole, and notes whose only links end where no `@annotation` is ever printed.
//
// The second kind used to vanish (ako/mxcli#1933, mendixlabs/mxcli#1731): a
// note linked only to the start event, or to another note, counted as linked,
// so it was not free, and its target prints nothing, so it was not attached
// either. Each is now described as a free note, with Unstated naming the link
// MDL cannot say.
func collectFreeAnnotations(oc *microflows.MicroflowObjectCollection) []describedAnnotation {
	if oc == nil {
		return nil
	}

	objects := map[model.ID]microflows.MicroflowObject{}
	var collect func(c *microflows.MicroflowObjectCollection)
	collect = func(c *microflows.MicroflowObjectCollection) {
		if c == nil {
			return
		}
		for _, o := range c.Objects {
			if o == nil {
				continue
			}
			objects[o.GetID()] = o
			if loop, ok := o.(*microflows.LoopedActivity); ok {
				collect(loop.ObjectCollection)
			}
		}
	}
	collect(oc)

	attached := make(map[model.ID]bool)
	unstated := make(map[model.ID][]string)
	for _, af := range oc.AnnotationFlows {
		if af == nil {
			continue
		}
		if kind := unstatableNoteEnd(objects[af.DestinationID]); kind == "" {
			attached[af.OriginID] = true
		} else {
			unstated[af.OriginID] = append(unstated[af.OriginID], kind)
		}
		// A link drawn the other way round (from an element to the note) is
		// not read as attaching the note by buildAnnotationsByTarget either.
		if _, isNote := objects[af.DestinationID].(*microflows.Annotation); isNote {
			if _, fromNote := objects[af.OriginID].(*microflows.Annotation); !fromNote {
				unstated[af.DestinationID] = append(unstated[af.DestinationID], "an element by a line drawn from that element")
			}
		}
	}

	var result []describedAnnotation
	for _, obj := range oc.Objects {
		if annot, ok := obj.(*microflows.Annotation); ok {
			if !attached[annot.ID] && annot.Caption != "" {
				result = append(result, describedAnnotation{
					ID: annot.ID, Caption: annot.Caption,
					Position: annot.Position, Size: annot.Size,
					Unstated: unstated[annot.ID],
				})
			}
		}
	}
	return result
}

// unstatableNoteEnd names the far end of a note's link when DESCRIBE cannot
// print the note there: the start event (no statement of its own) or another
// note. "" for an element that carries its notes as `@annotation` — and for
// one the collection does not hold, which is left to the traversal as before.
func unstatableNoteEnd(o microflows.MicroflowObject) string {
	switch o.(type) {
	case *microflows.StartEvent:
		return "the start event"
	case *microflows.Annotation:
		return "another note"
	}
	return ""
}

func prependFreeAnnotationLines(ctx *ExecContext, oc *microflows.MicroflowObjectCollection, activityLines []string) []string {
	freeAnnots := collectFreeAnnotations(oc)
	if len(freeAnnots) == 0 || len(activityLines) == 0 {
		return activityLines
	}

	prefix := make([]string, 0, len(freeAnnots))
	for _, note := range freeAnnots {
		for _, end := range note.Unstated {
			// A link MDL has no form for is dropped by any rewrite; say so
			// rather than let the round trip lose it quietly.
			prefix = append(prefix, fmt.Sprintf(
				"-- WARNING: the note %s is linked to %s, which MDL cannot state. It is described "+
					"unattached, and re-executing this description drops the link",
				commentLine(mdlQuote(ctx, note.Caption)), end))
		}
		// A free note is attached to nothing, so there is no activity position
		// to derive its default from — the writer places it from wherever the
		// cursor happens to be. Its geometry is therefore always spelled out.
		line := fmt.Sprintf("@annotation(text: %s, position: (%d, %d)", mdlQuote(ctx, note.Caption), note.Position.X, note.Position.Y)
		if note.Size != (model.Size{}) {
			line += fmt.Sprintf(", size: (%d, %d)", note.Size.Width, note.Size.Height)
		}
		prefix = append(prefix, line+")")
	}
	return append(prefix, activityLines...)
}

// commentLine folds the line breaks of a value quoted into a `--` comment
// into a visible ⏎. A break ended the comment mid-quote and the rest of the
// text parsed as MDL, so neither `check` on the description nor the splice's
// comparison against the stored flow could read it (ako/mxcli#2166).
func commentLine(s string) string {
	return commentLineBreaks.Replace(s)
}

var commentLineBreaks = strings.NewReplacer("\r\n", "⏎", "\n", "⏎", "\r", "⏎")

// anchorSideKeyword returns the MDL keyword (top/right/bottom/left) for a
// connection-index value. Returns "" for unknown values.
func anchorSideKeyword(idx int) string {
	switch idx {
	case AnchorTop:
		return "top"
	case AnchorRight:
		return "right"
	case AnchorBottom:
		return "bottom"
	case AnchorLeft:
		return "left"
	default:
		return ""
	}
}

// emitAnchorAnnotation emits an @anchor(...) line describing the incoming and
// outgoing SequenceFlow anchors for this object. Nothing is emitted when the
// object has no attached flows.
//
// For ExclusiveSplit / InheritanceSplit the split has up to two outgoing flows
// (true/false), so instead of the simple from/to form we emit the branch form:
//
//	@anchor(to: X, true: (from: Y, to: Z), false: (from: Y, to: Z))
//
// For LoopedActivity we emit the loop form when any iterator/tail flow exists:
//
//	@anchor(from: X, to: Y, iterator: (from: Y, to: Z), tail: (from: Y, to: Z))
//
// Any of the groups is omitted when its constituent values are both default
// (so a non-annotated split/loop produces no line). Non-split/non-loop objects
// use the simple @anchor(from: X, to: Y) form.
func emitAnchorAnnotation(
	obj microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	lines *[]string,
	indentStr string,
) {
	emitAnchorAnnotationWithActivityMap(obj, flowsByOrigin, flowsByDest, nil, lines, indentStr)
}

func emitAnchorAnnotationWithActivityMap(
	obj microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	activityMap map[model.ID]microflows.MicroflowObject,
	lines *[]string,
	indentStr string,
) {
	id := obj.GetID()

	if _, isSplit := obj.(*microflows.ExclusiveSplit); isSplit {
		emitSplitAnchorAnnotation(id, flowsByOrigin, flowsByDest, lines, indentStr, false,
			mergeExitAnchor(id, flowsByOrigin, activityMap), enumCaseAnchorFragments(id, flowsByOrigin, activityMap)...)
		return
	}
	if _, isSplit := obj.(*microflows.InheritanceSplit); isSplit {
		emitSplitAnchorAnnotation(id, flowsByOrigin, flowsByDest, lines, indentStr, true, "")
		return
	}
	if loop, isLoop := obj.(*microflows.LoopedActivity); isLoop {
		emitLoopAnchorAnnotation(loop, flowsByOrigin, flowsByDest, lines, indentStr)
		return
	}

	var from, to string
	if outgoing := flowsByOrigin[id]; len(outgoing) > 0 {
		for _, flow := range outgoing {
			if isNonWritableLoopBodyTailFlow(id, flow, activityMap) {
				continue
			}
			from = anchorSideKeyword(flow.OriginConnectionIndex)
			break
		}
	}
	defaultTo := anchorSideKeyword(AnchorLeft)
	if incoming := flowsByDest[id]; len(incoming) > 0 {
		to = anchorSideKeyword(incoming[0].DestinationConnectionIndex)
		// An error handler's first activity is entered by the error edge, which
		// the builder draws into its TOP (newErrorHandlerFlow). Judged against
		// the left side, `to: left` there read as the default and was left out,
		// and the rebuild entered the top (mendixlabs/mxcli#991).
		if len(incoming) == 1 && incoming[0].IsErrorHandler {
			defaultTo = anchorSideKeyword(AnchorTop)
		}
	}

	if from == "" && to == "" {
		return
	}
	defaultFrom := anchorSideKeyword(AnchorRight)
	var parts []string
	if from != "" && from != defaultFrom {
		parts = append(parts, "from: "+from)
	}
	if to != "" && to != defaultTo {
		parts = append(parts, "to: "+to)
	}
	if len(parts) == 0 {
		return
	}
	*lines = append(*lines, indentStr+fmt.Sprintf("@anchor(%s)", strings.Join(parts, ", ")))
}

// emitCurveAnnotation emits @curve(...) for the bezier control vectors on the
// flow LEAVING this activity.
//
// Without it the shape of a hand-curved edge does not round-trip: DESCRIBE was
// identical before and after the curve was drawn, so re-executing the output
// reset the line to "0;0" and flattened it. The vectors are stored as "x;y"
// strings on the flow's Microflows$BezierCurve line; "0;0" is straight and is
// omitted, so an untouched flow describes exactly as it did before. (#884)
func emitCurveAnnotation(
	obj microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	activityMap map[model.ID]microflows.MicroflowObject,
	lines *[]string,
	indentStr string,
) {
	outgoing := flowsByOrigin[obj.GetID()]
	var flow *microflows.SequenceFlow
	for _, f := range outgoing {
		if isNonWritableLoopBodyTailFlow(obj.GetID(), f, activityMap) {
			continue
		}
		flow = f
		break
	}
	if flow == nil {
		return
	}
	var parts []string
	if p, ok := controlVectorPair(flow.OriginControlVector); ok {
		parts = append(parts, "from: "+p)
	}
	if p, ok := controlVectorPair(flow.DestinationControlVector); ok {
		parts = append(parts, "to: "+p)
	}
	if len(parts) == 0 {
		return
	}
	*lines = append(*lines, indentStr+fmt.Sprintf("@curve(%s)", strings.Join(parts, ", ")))
}

// emitMergeAnnotation emits @merge(x, y) for the implicit merge node that closes
// a split.
//
// Without it the merge position does not round-trip: the layout pass recomputes
// it on the next exec, so a merge moved off a neighbour goes straight back on
// top of it. The split's own @position belongs to the split, hence a separate
// annotation.
//
// The merge is found by walking forward from EVERY branch of the split and
// taking the first node reachable from all of them that is an ExclusiveMerge.
// The describer's own split→merge map is not in scope at this call site, and
// threading it here would mean touching ten-plus call sites of
// emitObjectAnnotations — the multi-site change that silently skips one. Both
// inputs this needs are already parameters. (upstream #884)
func emitMergeAnnotation(
	obj microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	activityMap map[model.ID]microflows.MicroflowObject,
	layout *flowLayoutKeep,
	lines *[]string,
	indentStr string,
) {
	if activityMap == nil {
		return
	}
	switch obj.(type) {
	case *microflows.ExclusiveSplit, *microflows.InheritanceSplit:
	case *microflows.ActionActivity:
		// An activity has a merge of its own only when its error handler falls
		// through: the description drops the merge's label, so its position
		// rides on the activity (#750).
		if m := fallThroughRejoinMerge(obj.GetID(), flowsByOrigin, activityMap); m != "" {
			p := activityMap[m].GetPosition()
			*lines = append(*lines, indentStr+fmt.Sprintf("@merge(%d, %d)", p.X, p.Y))
		}
		return
	default:
		return
	}
	merge := commonMergeAfter(obj.GetID(), flowsByOrigin, activityMap)
	if merge == nil {
		return
	}
	// A merge with no way out ends a loop body. The builder no longer draws one
	// there (mendixlabs/mxcli#884), so a stored one — an earlier mxcli's, or one
	// a person kept — exists in a rebuild only if the description says so,
	// wherever it stands.
	if !layout.keepsPosition(merge.GetID()) && len(findNormalFlows(flowsByOrigin[merge.GetID()])) > 0 {
		return
	}
	p := merge.GetPosition()
	*lines = append(*lines, indentStr+fmt.Sprintf("@merge(%d, %d)", p.X, p.Y))
}

// closingMergeID is the merge an ExclusiveSplit's description speaks for (its
// @merge and the `from:` of its @anchor), or "" for anything else.
func closingMergeID(
	obj microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	activityMap map[model.ID]microflows.MicroflowObject,
) model.ID {
	if _, ok := obj.(*microflows.ExclusiveSplit); !ok || activityMap == nil {
		return ""
	}
	if merge := commonMergeAfter(obj.GetID(), flowsByOrigin, activityMap); merge != nil {
		return merge.GetID()
	}
	return ""
}

// commonMergeAfter returns the nearest ExclusiveMerge reachable from every
// outgoing branch of splitID, or nil when the branches do not rejoin (each
// returns, say).
//
// Bounded: a microflow canvas is small, but a retry loop makes the flow graph
// cyclic, so the walk carries a visited set per branch and a hard node cap.
func commonMergeAfter(
	splitID model.ID,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	activityMap map[model.ID]microflows.MicroflowObject,
) microflows.MicroflowObject {
	const maxNodes = 500

	branches := flowsByOrigin[splitID]
	if len(branches) < 2 {
		return nil
	}

	// Merges reachable from one branch, in breadth-first order.
	reachable := func(start model.ID) []model.ID {
		var order []model.ID
		seen := map[model.ID]bool{splitID: true}
		queue := []model.ID{start}
		for len(queue) > 0 && len(seen) < maxNodes {
			id := queue[0]
			queue = queue[1:]
			if seen[id] {
				continue
			}
			seen[id] = true
			if _, isMerge := activityMap[id].(*microflows.ExclusiveMerge); isMerge {
				order = append(order, id)
			}
			for _, f := range flowsByOrigin[id] {
				if !seen[f.DestinationID] {
					queue = append(queue, f.DestinationID)
				}
			}
		}
		return order
	}

	first := reachable(branches[0].DestinationID)
	if len(first) == 0 {
		return nil
	}
	inAll := map[model.ID]int{}
	for _, id := range first {
		inAll[id] = 1
	}
	for _, b := range branches[1:] {
		for _, id := range reachable(b.DestinationID) {
			if inAll[id] > 0 {
				inAll[id]++
			}
		}
	}
	// Preserve the first branch's breadth-first order, so "nearest" wins.
	for _, id := range first {
		if inAll[id] == len(branches) {
			return activityMap[id]
		}
	}
	return nil
}

// controlVectorPair renders a stored "x;y" control vector as "(x, y)", reporting
// false for the straight-line default so an untouched edge emits nothing.
func controlVectorPair(v string) (string, bool) {
	if v == "" || v == "0;0" {
		return "", false
	}
	x, y, found := strings.Cut(v, ";")
	if !found {
		return "", false
	}
	xi, errX := strconv.Atoi(strings.TrimSpace(x))
	yi, errY := strconv.Atoi(strings.TrimSpace(y))
	if errX != nil || errY != nil {
		return "", false
	}
	if xi == 0 && yi == 0 {
		return "", false
	}
	return fmt.Sprintf("(%d, %d)", xi, yi), true
}

func isNonWritableLoopBodyTailFlow(originID model.ID, flow *microflows.SequenceFlow, activityMap map[model.ID]microflows.MicroflowObject) bool {
	if flow == nil || activityMap == nil {
		return false
	}
	loop, ok := activityMap[flow.DestinationID].(*microflows.LoopedActivity)
	if !ok || loop.ObjectCollection == nil {
		return false
	}
	for _, obj := range loop.ObjectCollection.Objects {
		if obj.GetID() == originID {
			return true
		}
	}
	return false
}

// emitSplitAnchorAnnotation emits the split form of @anchor — the incoming
// `to: X` plus per-branch `true: (...)` / `false: (...)` — whenever any of the
// three has a non-default value. The rendering matches the grammar accepted
// by parseAnchorAnnotation so describe → exec roundtrips bit-exactly.
//
// The TRUE/FALSE branches are identified by findBranchFlows, which already
// handles every CaseValue variant the parser produces (ExpressionCase,
// EnumerationCase, BooleanCase — both value and pointer forms). Sharing that
// helper keeps the anchor emission consistent with the rest of the describer.
func emitSplitAnchorAnnotation(
	id model.ID,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	lines *[]string,
	indentStr string,
	preserveDefaultIncoming bool,
	mergeExitFrom string,
	caseParts ...string,
) {
	// Incoming flow anchor (where the previous activity's flow lands on the split).
	var inTo string
	if incoming := flowsByDest[id]; len(incoming) > 0 {
		inTo = anchorSideKeyword(incoming[0].DestinationConnectionIndex)
	}

	trueFlow, falseFlow := findBranchFlows(flowsByOrigin[id])

	var trueFrom, trueTo, falseFrom, falseTo string
	if trueFlow != nil {
		trueFrom = anchorSideKeyword(trueFlow.OriginConnectionIndex)
		trueTo = anchorSideKeyword(trueFlow.DestinationConnectionIndex)
	}
	if falseFlow != nil {
		falseFrom = anchorSideKeyword(falseFlow.OriginConnectionIndex)
		falseTo = anchorSideKeyword(falseFlow.DestinationConnectionIndex)
	}

	if inTo == "" && trueFrom == "" && trueTo == "" && falseFrom == "" && falseTo == "" && mergeExitFrom == "" && len(caseParts) == 0 {
		return
	}

	var parts []string
	// `from:` on an if is the flow leaving the statement — out of its closing
	// merge (#767). mergeExitAnchor has already dropped the default side.
	if mergeExitFrom != "" {
		parts = append(parts, "from: "+mergeExitFrom)
	}
	trueDefaultFroms := []string{anchorSideKeyword(AnchorRight), anchorSideKeyword(AnchorBottom)}
	trueDefaultTos := []string{anchorSideKeyword(AnchorLeft)}
	falseDefaultFroms := []string{anchorSideKeyword(AnchorBottom), anchorSideKeyword(AnchorRight)}
	falseDefaultTos := []string{anchorSideKeyword(AnchorTop), anchorSideKeyword(AnchorLeft)}
	splitDefaultIn := anchorSideKeyword(AnchorLeft)
	if inTo != "" && (preserveDefaultIncoming || inTo != splitDefaultIn) {
		parts = append(parts, "to: "+inTo)
	}
	if p := branchAnchorFragmentWithDefaultSides("true", trueFrom, trueTo, trueDefaultFroms, trueDefaultTos); p != "" {
		parts = append(parts, p)
	}
	if p := branchAnchorFragmentWithDefaultSides("false", falseFrom, falseTo, falseDefaultFroms, falseDefaultTos); p != "" {
		parts = append(parts, p)
	}
	parts = append(parts, caseParts...)
	if len(parts) == 0 {
		return
	}
	*lines = append(*lines, indentStr+fmt.Sprintf("@anchor(%s)", strings.Join(parts, ", ")))
}

// mergeExitAnchor returns the side the flow LEAVING an if's closing merge
// starts from, or "" when it is the default (right) or there is no such merge.
//
// The merge has no statement of its own, so this anchor is written as `from:`
// on the if — the same slot `from:` has on any other statement: the flow
// leaving it. Without it a merge-to-split flow that wraps onto a new row
// (bottom of the merge to top of the next decision) re-executed as leaving the
// merge's right side, back across the row above (ako/mxcli#767).
//
// Only an if (a split with true/false branches) has this slot; the merge is
// found the way @merge finds it, so the two annotations name the same node.
func mergeExitAnchor(
	splitID model.ID,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	activityMap map[model.ID]microflows.MicroflowObject,
) string {
	if activityMap == nil {
		return ""
	}
	if trueFlow, falseFlow := findBranchFlows(flowsByOrigin[splitID]); trueFlow == nil && falseFlow == nil && !hasEnumCaseFlow(flowsByOrigin[splitID]) {
		return ""
	}
	merge := commonMergeAfter(splitID, flowsByOrigin, activityMap)
	if merge == nil {
		return ""
	}
	for _, flow := range flowsByOrigin[merge.GetID()] {
		if flow.IsErrorHandler {
			continue
		}
		if flow.OriginConnectionIndex == AnchorRight {
			return ""
		}
		return anchorSideKeyword(flow.OriginConnectionIndex)
	}
	return ""
}

// hasEnumCaseFlow reports whether any of a split's flows carries an
// enumeration case — a CASE, whose `from:` is the flow leaving its merge as on
// an IF (mendixlabs/mxcli#1587).
func hasEnumCaseFlow(flows []*microflows.SequenceFlow) bool {
	for _, f := range flows {
		if _, ok := enumCaseValue(f); ok {
			return true
		}
	}
	return false
}

// enumCaseAnchorFragments returns the `Value: (from: …[, to: …])` entries a
// CASE's @anchor needs: one per outcome whose stored sides differ from what the
// builder would draw for it there (mendixlabs/mxcli#1587). Without them a CASE
// drawn in Studio Pro lost every side on describe -> exec.
//
// The default is computed exactly as addEnumSplit computes it — the branch's
// place in describe order, the branch count, where split and target stand — so
// an ordinary CASE gets no entries. `to:` is only written for a flow into a
// merge: one into an activity is carried by that activity's own `@anchor(to:)`.
func enumCaseAnchorFragments(
	splitID model.ID,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	activityMap map[model.ID]microflows.MicroflowObject,
) []string {
	split := activityMap[splitID]
	if split == nil {
		return nil
	}
	type branch struct {
		dest  model.ID
		flows []*microflows.SequenceFlow
	}
	var branches []branch
	byDest := map[model.ID]int{}
	for _, f := range orderedEnumSplitFlows(findNormalFlows(flowsByOrigin[splitID]), activityMap) {
		if _, ok := enumCaseValue(f); !ok {
			continue
		}
		if i, ok := byDest[f.DestinationID]; ok {
			branches[i].flows = append(branches[i].flows, f)
			continue
		}
		byDest[f.DestinationID] = len(branches)
		branches = append(branches, branch{dest: f.DestinationID, flows: []*microflows.SequenceFlow{f}})
	}
	targetOf := func(id model.ID) model.Point {
		if o := activityMap[id]; o != nil {
			return o.GetPosition()
		}
		return model.Point{}
	}
	ys := make([]int, len(branches))
	for i, b := range branches {
		ys[i] = targetOf(b.dest).Y
	}
	origins := enumSplitOriginAnchors(ys, split.GetPosition().Y)

	var out []string
	for i, b := range branches {
		slot := splitCaseSlot{order: i, origin: -1}
		if origins != nil {
			slot.origin = origins[i]
		}
		_, intoMerge := activityMap[b.dest].(*microflows.ExclusiveMerge)
		for _, f := range b.flows {
			def := &microflows.SequenceFlow{}
			slot.apply(def, split.GetPosition(), targetOf(b.dest))
			var inner []string
			if f.OriginConnectionIndex != def.OriginConnectionIndex {
				inner = append(inner, "from: "+anchorSideKeyword(f.OriginConnectionIndex))
			}
			if intoMerge && f.DestinationConnectionIndex != def.DestinationConnectionIndex {
				inner = append(inner, "to: "+anchorSideKeyword(f.DestinationConnectionIndex))
			}
			if len(inner) > 0 {
				value, _ := enumCaseValue(f)
				out = append(out, formatEnumSplitCaseValue(value)+": ("+strings.Join(inner, ", ")+")")
			}
		}
	}
	return out
}

// branchAnchorFragment builds a `key: (from: X, to: Y)` fragment for a branch
// anchor. Returns "" when both sides are empty (default).
func branchAnchorFragment(label, from, to string) string {
	if from == "" && to == "" {
		return ""
	}
	var inner []string
	if from != "" {
		inner = append(inner, "from: "+from)
	}
	if to != "" {
		inner = append(inner, "to: "+to)
	}
	return fmt.Sprintf("%s: (%s)", label, strings.Join(inner, ", "))
}

// branchAnchorFragmentWithDefaultSides returns the `label: (from: X, to: Y)`
// fragment for a branch anchor, suppressing sides that match the layout
// default and removing the whole fragment when both sides reduce to default.
//
// The function applies suppression in two passes:
//
//  1. Primary suppression — if `from` or `to` is one of the documented
//     defaults for this branch (e.g. true branch defaults to from=right or
//     from=bottom and to=left), zero it out.
//
//  2. Secondary suppression — when ONE side has already been zeroed by pass 1,
//     check whether the surviving side is itself a layout-equivalent default
//     that Studio Pro auto-routes. The combinations were observed against
//     real Studio Pro output: e.g. on a false branch with no FROM, Studio Pro
//     routes to bottom or right automatically; on a true branch with no FROM,
//     Studio Pro routes to bottom when the target sits below the split.
//     Suppressing these prevents the describer from emitting fragments that
//     Studio Pro would have layered identically anyway.
//
// The secondary pass is intentionally order-dependent: it relies on `from` and
// `to` being post-primary-suppression. Paired manual anchors like
// `false: (from: left, to: right)` survive both passes because neither side
// was zeroed by pass 1.
func branchAnchorFragmentWithDefaultSides(label, from, to string, defaultFroms, defaultTos []string) string {
	if containsString(defaultFroms, from) {
		from = ""
	}
	if containsString(defaultTos, to) {
		to = ""
	}
	// Secondary suppression: see function comment above for the reasoning.
	// Inputs to this switch are already post-primary-suppression.
	top := anchorSideKeyword(AnchorTop)
	bottom := anchorSideKeyword(AnchorBottom)
	right := anchorSideKeyword(AnchorRight)
	switch label {
	case "false":
		if to == "" && from == top {
			from = ""
		}
		if from == "" && (to == bottom || to == right) {
			to = ""
		}
	case "true":
		if from == "" && to == bottom {
			to = ""
		}
	}
	return branchAnchorFragment(label, from, to)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// emitLoopAnchorAnnotation emits the loop form of @anchor for a LoopedActivity.
// A LoopedActivity has up to four flows worth describing:
//   - the incoming flow from the previous activity (normal `to:`)
//   - the outgoing flow to the next activity (normal `from:`)
//   - the iterator flow (loop boundary → first body statement), emitted as
//     `iterator: (from: X, to: Y)` when present
//   - the tail flow (last body statement → loop boundary), emitted as
//     `tail: (from: X, to: Y)` when present
//
// The iterator/tail edges are optional — Mendix normally renders them
// implicitly from the LoopedActivity geometry — so they appear only when the
// flows actually exist in the parent collection. This keeps plain loops
// free of `@anchor(iterator: ..., tail: ...)` noise on describe while still
// round-tripping projects that explicitly pinned them via @anchor annotations.
func emitLoopAnchorAnnotation(
	loop *microflows.LoopedActivity,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	lines *[]string,
	indentStr string,
) {
	id := loop.ID

	innerIDs := make(map[model.ID]bool)
	if loop.ObjectCollection != nil {
		for _, o := range loop.ObjectCollection.Objects {
			innerIDs[o.GetID()] = true
		}
	}

	// Outer incoming / outgoing (flows between this loop and its siblings).
	// Only the first non-iterator edge counts as the external connection.
	var outerFrom, outerTo string
	for _, f := range flowsByOrigin[id] {
		if innerIDs[f.DestinationID] {
			continue // iterator edge, handled below
		}
		outerFrom = anchorSideKeyword(f.OriginConnectionIndex)
		break
	}
	for _, f := range flowsByDest[id] {
		if innerIDs[f.OriginID] {
			continue // tail edge, handled below
		}
		outerTo = anchorSideKeyword(f.DestinationConnectionIndex)
		break
	}

	// Iterator edge: loop → inner. There can only be one in a well-formed
	// project; if the builder ever emits more we keep the first for now.
	var iterFrom, iterTo string
	for _, f := range flowsByOrigin[id] {
		if !innerIDs[f.DestinationID] {
			continue
		}
		iterFrom = anchorSideKeyword(f.OriginConnectionIndex)
		iterTo = anchorSideKeyword(f.DestinationConnectionIndex)
		break
	}

	// Tail edge: inner → loop.
	var tailFrom, tailTo string
	for _, f := range flowsByDest[id] {
		if !innerIDs[f.OriginID] {
			continue
		}
		tailFrom = anchorSideKeyword(f.OriginConnectionIndex)
		tailTo = anchorSideKeyword(f.DestinationConnectionIndex)
		break
	}

	if outerFrom == "" && outerTo == "" && iterFrom == "" && iterTo == "" && tailFrom == "" && tailTo == "" {
		return
	}

	var parts []string
	if outerFrom != "" {
		parts = append(parts, "from: "+outerFrom)
	}
	if outerTo != "" {
		parts = append(parts, "to: "+outerTo)
	}
	if p := branchAnchorFragment("iterator", iterFrom, iterTo); p != "" {
		parts = append(parts, p)
	}
	if p := branchAnchorFragment("tail", tailFrom, tailTo); p != "" {
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return
	}
	*lines = append(*lines, indentStr+fmt.Sprintf("@anchor(%s)", strings.Join(parts, ", ")))
}

// captionIsCondition reports whether a decision's caption is its condition
// text, which is what a decision gets when no @caption is written: the builder
// stores one string as both the caption and the expression (addIfStatement,
// the enum case). Printing it would repeat the line below it (R12, #748), and
// leaving it out re-executes to the same caption, since the two are built from
// the same text.
func captionIsCondition(split *microflows.ExclusiveSplit) bool {
	cond, ok := split.SplitCondition.(*microflows.ExpressionSplitCondition)
	return ok && split.Caption == cond.Expression
}

// emitObjectAnnotations emits @position, @caption, @color, @annotation, and
// @anchor lines for a microflow object before its statement.
//
// flowsByOrigin / flowsByDest are optional: when both are non-nil, @anchor
// lines are emitted using them. Passing nil suppresses @anchor emission,
// which keeps small unit tests of this helper self-contained.
func emitObjectAnnotations(
	obj microflows.MicroflowObject,
	lines *[]string,
	indentStr string,
	annotationsByTarget *annotationEmitter,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	activityMap map[model.ID]microflows.MicroflowObject,
) {
	currentID := obj.GetID()

	pos := obj.GetPosition()
	layout := annotationsByTarget.layoutKeep()
	if layout.keepsPosition(currentID) {
		*lines = append(*lines, indentStr+fmt.Sprintf("@position(%d, %d)", pos.X, pos.Y))
	}

	if flowsByOrigin != nil && flowsByDest != nil {
		// @anchor — emit whenever attached flows exist, for roundtrip fidelity.
		// The emitter sorts out the right form (simple / split / loop) based on
		// the object type.
		// An if's anchor also carries the flow out of its closing merge, which
		// has no statement of its own — so a pinned merge keeps the if's anchor.
		if layout.keepsAnchor(currentID) || layout.keepsAnchor(closingMergeID(obj, flowsByOrigin, activityMap)) {
			emitAnchorAnnotationWithActivityMap(obj, flowsByOrigin, flowsByDest, activityMap, lines, indentStr)
		}
		if layout.keepsCurve(currentID) {
			emitCurveAnnotation(obj, flowsByOrigin, activityMap, lines, indentStr)
		}
		emitMergeAnnotation(obj, flowsByOrigin, activityMap, layout, lines, indentStr)
	}

	if activity, ok := obj.(*microflows.ActionActivity); ok {
		if activity.Disabled {
			*lines = append(*lines, indentStr+"@excluded")
		}
		if !activity.AutoGenerateCaption && activity.Caption != "" {
			*lines = append(*lines, indentStr+fmt.Sprintf("@caption %s", annotationsByTarget.quote(activity.Caption)))
		}
		if activity.BackgroundColor != "" && activity.BackgroundColor != "Default" {
			*lines = append(*lines, indentStr+fmt.Sprintf("@color %s", activity.BackgroundColor))
		}
	}

	// An empty caption is printed too: left out, the decision re-executes
	// captioned with its condition (#1254).
	if split, ok := obj.(*microflows.ExclusiveSplit); ok && !captionIsCondition(split) {
		*lines = append(*lines, indentStr+fmt.Sprintf("@caption %s", annotationsByTarget.quote(split.Caption)))
	}
	if split, ok := obj.(*microflows.InheritanceSplit); ok && split.Caption != "" {
		*lines = append(*lines, indentStr+fmt.Sprintf("@caption %s", annotationsByTarget.quote(split.Caption)))
	}
	// No @caption for a LoopedActivity: the metamodel declares none on it, so a
	// stored loop never carries one and this only ever emitted MDL that check
	// would then report as MDL042 (mendixlabs/mxcli#1187).

	// @annotation (attached Annotation objects)
	*lines = append(*lines, annotationsByTarget.lines(currentID, pos, objectHeight(obj), indentStr)...)
}

// emitActivityStatement appends the formatted activity statement (with error handling)
// to the lines slice. It handles ON ERROR CONTINUE/ROLLBACK suffixes and custom error
// handler blocks. This replaces the copy-pasted error handling logic in each traversal function.
func emitActivityStatement(
	ctx *ExecContext,
	obj microflows.MicroflowObject,
	stmt string,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	activityMap map[model.ID]microflows.MicroflowObject,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	lines *[]string,
	indentStr string,
	annotationsByTarget *annotationEmitter,
	labels mergeLabels,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
) {
	if stmt == "" {
		return
	}

	// When the activity is unsupported by the describer (e.g. CallWebServiceAction,
	// CastAction, InheritanceSplit placeholder) we fall back to emitting just an
	// MDL line comment. Decorating that comment with @position/@anchor/@annotation
	// leaves the annotations orphaned — the grammar only accepts `annotation*`
	// as a prefix of a real microflowStatement, so line comments preceded by
	// annotations cause "no viable alternative at input '@position...end'" during
	// exec. Emit the comment on its own instead.
	if strings.HasPrefix(strings.TrimSpace(stmt), "--") {
		*lines = append(*lines, indentStr+stmt)
		// The early return above used to end the function, which also skipped the
		// error-handler traversal further down — so an activity the describer
		// could not render lost its entire error branch without a word (#863).
		// The branch cannot be emitted as live MDL (an `on error begin … end error` block has
		// no statement to attach to here), but it must not disappear either:
		// render it commented-out, so the artifact still shows what the model
		// holds. Guard-don't-drop, in a path that cannot round-trip.
		emitCommentedErrorHandler(
			ctx, obj, flowsByOrigin, activityMap, entityNames, microflowNames, lines, indentStr, annotationsByTarget, labels, sourceMap, headerLineCount)
		return
	}

	// Emit @ annotations before the statement
	emitObjectAnnotations(obj, lines, indentStr, annotationsByTarget, flowsByOrigin, flowsByDest, activityMap)

	currentID := obj.GetID()
	flows := flowsByOrigin[currentID]
	errorHandlerFlow := findErrorHandlerFlow(flows)

	activity, isAction := obj.(*microflows.ActionActivity)
	if !isAction {
		*lines = append(*lines, indentStr+stmt)
		return
	}

	errType := getActionErrorHandlingType(activity)
	suffix := formatErrorHandlingSuffix(errType)

	if errorHandlerFlow != nil && hasCustomErrorHandler(errType) {
		errStmts, errSpans := collectErrorHandlerStatementSpans(
			ctx,
			errorHandlerFlow.DestinationID,
			activityMap, flowsByOrigin, entityNames, microflowNames, annotationsByTarget, labels,
		)

		stmtWithoutSemi := strings.TrimSuffix(strings.TrimSpace(stmt), ";")

		errorSuffix := suffix
		if errorSuffix == "" {
			errorSuffix = " on error without rollback"
		}

		if len(errStmts) == 0 {
			*lines = append(*lines, indentStr+stmtWithoutSemi+errorSuffix+" begin end error;")
		} else {
			*lines = append(*lines, indentStr+stmtWithoutSemi+errorSuffix+" begin")
			recordErrorHandlerSpans(sourceMap, errSpans, len(*lines)+headerLineCount)
			for _, errStmt := range errStmts {
				*lines = append(*lines, indentStr+"  "+errStmt)
			}
			*lines = append(*lines, indentStr+"end error;")
		}
	} else if suffix != "" {
		stmtWithoutSemi := strings.TrimSuffix(strings.TrimSpace(stmt), ";")
		*lines = append(*lines, indentStr+stmtWithoutSemi+suffix+";")
	} else {
		*lines = append(*lines, indentStr+stmt)
	}
}

// emitCommentedErrorHandler renders an activity's error branch as MDL line
// comments. It is the fallback for activities the describer can only emit as a
// comment: the branch has no live statement to hang an `on error begin … end error` block
// off, so the choice is between showing it commented-out and losing it.
//
// Commenting is load-bearing, not cosmetic. Emitting these statements as live
// MDL would be worse than dropping them — on the next exec they would run
// unconditionally in the main flow rather than on failure, quietly changing the
// microflow's behaviour instead of merely shrinking it.
func emitCommentedErrorHandler(
	ctx *ExecContext,
	obj microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	activityMap map[model.ID]microflows.MicroflowObject,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	lines *[]string,
	indentStr string,
	annotationsByTarget *annotationEmitter,
	labels mergeLabels,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
) {
	errorHandlerFlow := findErrorHandlerFlow(flowsByOrigin[obj.GetID()])
	if errorHandlerFlow == nil {
		return
	}

	// The error-handling type lives on the action, which is exactly what is
	// missing here — fall back to the activity level and, failing that, to the
	// shape Studio Pro gives a hand-drawn error flow.
	errType := getActionErrorHandlingType(activity(obj))
	suffix := strings.TrimSpace(formatErrorHandlingSuffix(errType))
	if suffix == "" {
		suffix = "on error without rollback"
	}

	errStmts, errSpans := collectErrorHandlerStatementSpans(
		ctx, errorHandlerFlow.DestinationID, activityMap, flowsByOrigin, entityNames, microflowNames, annotationsByTarget, labels)
	if len(errStmts) == 0 {
		*lines = append(*lines, indentStr+"-- "+suffix+" begin end error;")
		return
	}
	*lines = append(*lines, indentStr+"-- "+suffix+" begin")
	recordErrorHandlerSpans(sourceMap, errSpans, len(*lines)+headerLineCount)
	for _, errStmt := range errStmts {
		*lines = append(*lines, indentStr+"--   "+strings.TrimSpace(errStmt))
	}
	*lines = append(*lines, indentStr+"-- end error;")
}

// activity narrows a MicroflowObject to *ActionActivity, or nil.
func activity(obj microflows.MicroflowObject) *microflows.ActionActivity {
	a, _ := obj.(*microflows.ActionActivity)
	return a
}

// recordSourceMap records the source map entry for a node if sourceMap is non-nil.
func recordSourceMap(sourceMap map[string]elkSourceRange, nodeID model.ID, startLine, endLine int) {
	if sourceMap != nil && endLine >= startLine {
		sourceMap["node-"+string(nodeID)] = elkSourceRange{StartLine: startLine, EndLine: endLine}
	}
}

// traverseFlow recursively traverses the microflow graph and generates MDL statements.
// When sourceMap is non-nil, it also records line ranges for each activity node.
func traverseFlow(
	ctx *ExecContext,
	currentID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	splitMergeMap map[model.ID]model.ID,
	visited map[model.ID]bool,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	lines *[]string,
	indent int,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
	annotationsByTarget *annotationEmitter,
	labels mergeLabels,
) {
	if currentID == "" {
		return
	}

	// A crossed merge emits a `join` EVERY time a path arrives at it — arriving
	// more than once is exactly what makes it crossed — so this is checked
	// before the visited guard, which would otherwise swallow the second
	// branch's join and leave that path falling off the end of the description.
	if labels.isCrossed(currentID) {
		if label, ok := labels.of(currentID); ok {
			*lines = append(*lines, strings.Repeat("  ", indent)+"join "+label+";")
			return
		}
	}

	if visited[currentID] {
		return
	}

	obj := activityMap[currentID]
	if obj == nil {
		return
	}

	// Merge points are normally processed by the matching split's traversal —
	// returning here avoids emitting an `end if;` twice. But a merge can also
	// appear as a pure junction (multiple incoming flows converging outside
	// of an IF), e.g. a manual retry loop where a "Call REST → decide → loop
	// back" pattern routes the back-edge into a merge. Those merges are not
	// values in splitMergeMap, and stopping there drops every activity after
	// them (see issue #281). When that happens, walk through as a pass-
	// through — same as traverseFlowUntilMerge already does for intermediate
	// merges.
	if _, isMerge := obj.(*microflows.ExclusiveMerge); isMerge {
		// A CROSSED merge is reached by several branches, so arriving at one is
		// not a place to continue from — it is a place to stop and say where the
		// path went. The declaration is emitted once, afterwards, by
		// emitCrossedMergeSections. Deliberately does NOT mark visited: every
		// branch that lands here has to emit its own `join`.
		if labels.isCrossed(currentID) {
			if label, ok := labels.of(currentID); ok {
				*lines = append(*lines, strings.Repeat("  ", indent)+"join "+label+";")
				return
			}
		}
		// A merge some error handler rejoins needs a name, and the name has to be
		// DECLARED on the path that owns it — a `join` with no `merge` is MDL that
		// does not execute. Emitted before the paired-with-a-split check so the
		// declaration lands whichever way the merge is reached.
		if label, ok := labels.of(currentID); ok && !visited[currentID] {
			visited[currentID] = true
			*lines = append(*lines, mergeDeclarationLines(indent, label, obj, annotationsByTarget.layoutKeep(), flowsByOrigin)...)
			for _, flow := range findNormalFlows(flowsByOrigin[currentID]) {
				traverseFlow(ctx, flow.DestinationID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
			}
			return
		}
		if isMergePairedWithSplit(currentID, splitMergeMap) {
			return
		}
		if mergeHasLoopBackEdge(currentID, flowsByOrigin) {
			visited[currentID] = true
			*lines = append(*lines, strings.Repeat("  ", indent)+"while true")
			*lines = append(*lines, strings.Repeat("  ", indent)+"begin")
			for _, flow := range findNormalFlows(flowsByOrigin[currentID]) {
				traverseFlowUntilMerge(ctx, flow.DestinationID, currentID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget, labels)
			}
			*lines = append(*lines, strings.Repeat("  ", indent)+"end while;")
			return
		}
		visited[currentID] = true
		// A merge with one way in, on the main line, is spelled the way exec
		// was given it — `join L; merge L;` — so the description rebuilds it
		// instead of warning that it is lost (see inlineMerges).
		if indent == 0 {
			if label, ok := labels.inlineLabel(currentID); ok {
				*lines = append(*lines, "join "+label+";")
				*lines = append(*lines, mergeDeclarationLines(indent, label, obj, annotationsByTarget.layoutKeep(), flowsByOrigin)...)
			}
		}
		for _, flow := range flowsByOrigin[currentID] {
			traverseFlow(ctx, flow.DestinationID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
		}
		return
	}

	visited[currentID] = true

	stmt := formatActivity(ctx, obj, entityNames, microflowNames)
	indentStr := strings.Repeat("  ", indent)

	if _, isSplit := obj.(*microflows.InheritanceSplit); isSplit && len(findNormalFlows(flowsByOrigin[currentID])) > 1 {
		startLine := len(*lines) + headerLineCount
		mergeID := splitMergeMap[currentID]
		emitObjectAnnotations(obj, lines, indentStr, annotationsByTarget, flowsByOrigin, flowsByDest, activityMap)
		emitInheritanceSplitStatement(ctx, currentID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
		recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)
		if mergeID != "" && !joinCrossedMerge(mergeID, labels, lines, indent) {
			visited[mergeID] = true
			for _, flow := range flowsByOrigin[mergeID] {
				traverseFlow(ctx, flow.DestinationID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
			}
		}
		return
	}

	// Handle ExclusiveSplit specially - need to process both branches
	if split, isSplit := obj.(*microflows.ExclusiveSplit); isSplit {
		startLine := len(*lines) + headerLineCount
		flows := flowsByOrigin[currentID]
		mergeID := splitMergeMap[currentID]
		if source, ok := enumSplitSource(ctx, split); ok && hasEnumCaseFlows(flows) {
			emitObjectAnnotations(obj, lines, indentStr, annotationsByTarget, flowsByOrigin, flowsByDest, activityMap)
			emitEnumSplitStatement(ctx, currentID, mergeID, source, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
			recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)
			continueAfterSplitJoin(ctx, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
			return
		}
		// A decision whose true path goes straight on is printed as stored —
		// `if X then else <false path> end if;` — not folded into
		// `if not(X) then …`: executing the folded form stored not(X) with
		// the outcomes swapped, so describe -> exec changed the model
		// (ako/mxcli#1963), and a condition edit had to undo the fold
		// (#1930). The builder builds the unfolded form as Studio Pro stores it.
		trueFlow, falseFlow := findBranchFlows(flows)

		if stmt != "" {
			emitObjectAnnotations(obj, lines, indentStr, annotationsByTarget, flowsByOrigin, flowsByDest, activityMap)
			*lines = append(*lines, indentStr+stmt)
		}

		trueTerminates := branchFlowTerminatesBeforeMerge(trueFlow, mergeID, activityMap, flowsByOrigin, splitMergeMap)
		isGuard := trueTerminates && flowLooksLikeGuardContinuation(ctx, falseFlow, obj, activityMap) && !hasExplicitFalseBranchAnchor(falseFlow)

		if isGuard {
			// A guard's THEN arm is followed by the false continuation.
			restore := fallsOnBefore(ctx, falseFlow, activityMap, flowsByOrigin)
			traverseFlowUntilMerge(ctx, trueFlow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget, labels)
			restore()
			*lines = append(*lines, indentStr+"end if;")
			recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)

			// Continue from the false branch (skip through merge if present).
			// A labelled merge is not skipped: walking past it is how a crossed
			// merge's suffix got printed inline AND in its own section.
			if falseFlow != nil {
				contID := falseFlow.DestinationID
				if _, isMerge := activityMap[contID].(*microflows.ExclusiveMerge); isMerge && !isLabelledMerge(contID, labels) {
					visited[contID] = true
					for _, flow := range flowsByOrigin[contID] {
						contID = flow.DestinationID
						break
					}
				}
				traverseFlow(ctx, contID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
			}
		} else {
			if trueFlow != nil {
				traverseFlowUntilMerge(ctx, trueFlow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget, labels)
			}

			if falseFlow != nil {
				elseLineIdx := len(*lines)
				*lines = append(*lines, indentStr+"else")
				visitedFalseBranch := make(map[model.ID]bool)
				for id := range visited {
					visitedFalseBranch[id] = true
				}
				traverseFlowUntilMerge(ctx, falseFlow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visitedFalseBranch, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget, labels)
				// Remove empty else block. A false branch can point at a
				// continuation already emitted through the true branch, so checking
				// only falseFlow.DestinationID != mergeID is not enough.
				if len(*lines) == elseLineIdx+1 {
					*lines = (*lines)[:elseLineIdx]
				} else {
					foldElseIntoElsif(lines, elseLineIdx, indent, sourceMap, headerLineCount)
				}
			}

			*lines = append(*lines, indentStr+"end if;")
			recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)

			if armFallsInto(currentID, mergeID, flowsByOrigin, labels) {
				continueAfterSplitJoin(ctx, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
			}
		}
		return
	}

	// Handle LoopedActivity specially - need to process loop body
	if loop, isLoop := obj.(*microflows.LoopedActivity); isLoop {
		startLine := len(*lines) + headerLineCount
		if stmt != "" {
			emitObjectAnnotations(obj, lines, indentStr, annotationsByTarget, flowsByOrigin, flowsByDest, activityMap)
			*lines = append(*lines, indentStr+stmt)
		}

		*lines = append(*lines, indentStr+"begin")
		emitLoopBody(ctx, loop, flowsByOrigin, flowsByDest, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)

		*lines = append(*lines, indentStr+loopEndKeyword(loop)+";")
		recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)

		// Continue after the loop
		flows := flowsByOrigin[currentID]
		for _, flow := range flows {
			traverseFlow(ctx, flow.DestinationID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
		}
		return
	}

	// Regular activity
	startLine := len(*lines) + headerLineCount
	normalFlows := findNormalFlows(flowsByOrigin[currentID])
	emitActivityStatement(ctx, obj, stmt, flowsByOrigin, flowsByDest, activityMap, entityNames, microflowNames, lines, indentStr, annotationsByTarget, labels, sourceMap, headerLineCount)
	recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)

	// Follow normal (non-error-handler) outgoing flows
	for _, flow := range normalFlows {
		traverseFlow(ctx, flow.DestinationID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
	}
}

// traverseFlowUntilMerge traverses the flow until reaching a merge point.
// When sourceMap is non-nil, it also records line ranges for each activity node.
func traverseFlowUntilMerge(
	ctx *ExecContext,
	currentID model.ID,
	mergeID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	splitMergeMap map[model.ID]model.ID,
	visited map[model.ID]bool,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	lines *[]string,
	indent int,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
	annotationsByTarget *annotationEmitter,
	labels mergeLabels,
) {
	if currentID == "" || currentID == mergeID || visited[currentID] {
		return
	}

	obj := activityMap[currentID]
	if obj == nil {
		return
	}

	// What follows this branch once it closes: the walk up to mergeID is the
	// branch, and whatever mergeID leads to is printed after it — as is
	// whatever followed the enclosing branch. Nested branches inherit it.
	if ctx != nil && !ctx.describeFallsOn && joinGoesOn(mergeID, activityMap, flowsByOrigin) {
		ctx.describeFallsOn = true
		defer func() { ctx.describeFallsOn = false }()
	}

	// Handle intermediate merge points - traverse through them without outputting
	// anything, UNLESS an error handler rejoins here: then the merge is named, and
	// the name must be declared on the branch that owns it. An empty-handler
	// pattern whose merge sits inside the ELSE branch is the common instance, and
	// the `join` in the handler is dangling MDL without this.
	if _, isMerge := obj.(*microflows.ExclusiveMerge); isMerge {
		// Crossed: this branch ends here and says so. See traverseFlow.
		if labels.isCrossed(currentID) {
			if label, ok := labels.of(currentID); ok {
				*lines = append(*lines, strings.Repeat("  ", indent)+"join "+label+";")
				return
			}
		}
		if label, ok := labels.of(currentID); ok && !visited[currentID] {
			visited[currentID] = true
			*lines = append(*lines, mergeDeclarationLines(indent, label, obj, annotationsByTarget.layoutKeep(), flowsByOrigin)...)
		}
		// A manual loop's header reached inside a branch: the same `while true`
		// traverseFlow writes at the top level, for the same reason. Walked
		// through as a plain merge, the path round the loop came back to an
		// activity already printed and stopped there in silence, so the
		// description had no back-edge at all and re-executed to a flow that
		// ENDS where the original goes round again (Evora's
		// AmazonBedrockConnector.*_Sync pagination loops).
		//
		// Only a merge whose way round avoids what is already being described
		// is a header. A merge on the path BACK to an enclosing header is on a
		// cycle too, but it is that loop's back-edge, not a loop of its own.
		if !visited[currentID] && mergeIsUndescribedLoopHeader(currentID, mergeID, flowsByOrigin, visited) {
			visited[currentID] = true
			*lines = append(*lines, strings.Repeat("  ", indent)+"while true")
			*lines = append(*lines, strings.Repeat("  ", indent)+"begin")
			for _, flow := range findNormalFlows(flowsByOrigin[currentID]) {
				traverseFlowUntilMerge(ctx, flow.DestinationID, currentID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget, labels)
			}
			*lines = append(*lines, strings.Repeat("  ", indent)+"end while;")
			return
		}
		flows := flowsByOrigin[currentID]
		for _, flow := range flows {
			traverseFlowUntilMerge(ctx, flow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
		}
		return
	}

	visited[currentID] = true

	stmt := formatActivity(ctx, obj, entityNames, microflowNames)
	indentStr := strings.Repeat("  ", indent)

	if _, isSplit := obj.(*microflows.InheritanceSplit); isSplit && len(findNormalFlows(flowsByOrigin[currentID])) > 1 {
		startLine := len(*lines) + headerLineCount
		nestedMergeID := splitMergeMap[currentID]
		emitObjectAnnotations(obj, lines, indentStr, annotationsByTarget, flowsByOrigin, flowsByDest, activityMap)
		emitInheritanceSplitStatement(ctx, currentID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
		recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)
		if nestedMergeID != "" && nestedMergeID != mergeID && !joinCrossedMerge(nestedMergeID, labels, lines, indent) {
			visited[nestedMergeID] = true
			for _, flow := range flowsByOrigin[nestedMergeID] {
				traverseFlowUntilMerge(ctx, flow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
			}
		}
		return
	}

	// Handle nested ExclusiveSplit
	if split, isSplit := obj.(*microflows.ExclusiveSplit); isSplit {
		startLine := len(*lines) + headerLineCount
		flows := flowsByOrigin[currentID]
		nestedMergeID := splitMergeMap[currentID]
		if source, ok := enumSplitSource(ctx, split); ok && hasEnumCaseFlows(flows) {
			emitObjectAnnotations(obj, lines, indentStr, annotationsByTarget, flowsByOrigin, flowsByDest, activityMap)
			emitEnumSplitStatement(ctx, currentID, nestedMergeID, source, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
			recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)
			continueAfterNestedSplitJoin(ctx, nestedMergeID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
			return
		}
		trueFlow, falseFlow := findBranchFlows(flows)
		nestedMergeID = resolveNestedMergeID(currentID, nestedMergeID, mergeID, trueFlow, falseFlow, flowsByOrigin)
		// Printed as stored, never folded into not(X) (ako/mxcli#1963).

		if stmt != "" {
			emitObjectAnnotations(obj, lines, indentStr, annotationsByTarget, flowsByOrigin, flowsByDest, activityMap)
			*lines = append(*lines, indentStr+stmt)
		}

		trueTerminates := branchFlowTerminatesBeforeMerge(trueFlow, nestedMergeID, activityMap, flowsByOrigin, splitMergeMap)
		isGuard := trueTerminates && flowLooksLikeGuardContinuation(ctx, falseFlow, obj, activityMap) && !hasExplicitFalseBranchAnchor(falseFlow)

		if isGuard {
			// A guard's THEN arm is followed by the false continuation.
			restore := fallsOnBefore(ctx, falseFlow, activityMap, flowsByOrigin)
			traverseFlowUntilMerge(ctx, trueFlow.DestinationID, nestedMergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget, labels)
			restore()
			*lines = append(*lines, indentStr+"end if;")
			recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)

			// Continue from the false branch (skip through merge if present).
			// Guard: do not cross the outer merge boundary — if the false path
			// leads directly to mergeID, stop here so the activities after the
			// merge are emitted by continueAfterSplitJoin, not inside this branch.
			if falseFlow != nil {
				contID := falseFlow.DestinationID
				if contID != mergeID {
					if _, isMerge := activityMap[contID].(*microflows.ExclusiveMerge); isMerge && !isLabelledMerge(contID, labels) {
						visited[contID] = true
						for _, flow := range flowsByOrigin[contID] {
							contID = flow.DestinationID
							break
						}
					}
					traverseFlowUntilMerge(ctx, contID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
				}
			}
		} else {
			if trueFlow != nil {
				traverseFlowUntilMerge(ctx, trueFlow.DestinationID, nestedMergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget, labels)
			}

			if falseFlow != nil {
				elseLineIdx := len(*lines)
				*lines = append(*lines, indentStr+"else")
				visitedFalseBranch := make(map[model.ID]bool)
				for id := range visited {
					visitedFalseBranch[id] = true
				}
				traverseFlowUntilMerge(ctx, falseFlow.DestinationID, nestedMergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visitedFalseBranch, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget, labels)
				// Remove empty else block
				if len(*lines) == elseLineIdx+1 {
					*lines = (*lines)[:elseLineIdx]
				} else {
					foldElseIntoElsif(lines, elseLineIdx, indent, sourceMap, headerLineCount)
				}
			}

			*lines = append(*lines, indentStr+"end if;")
			recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)

			if armFallsInto(currentID, nestedMergeID, flowsByOrigin, labels) {
				continueAfterNestedSplitJoin(ctx, nestedMergeID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
			}
		}
		return
	}

	// Handle LoopedActivity inside a branch
	if loop, isLoop := obj.(*microflows.LoopedActivity); isLoop {
		startLine := len(*lines) + headerLineCount
		if stmt != "" {
			emitObjectAnnotations(obj, lines, indentStr, annotationsByTarget, flowsByOrigin, flowsByDest, activityMap)
			*lines = append(*lines, indentStr+stmt)
		}

		*lines = append(*lines, indentStr+"begin")
		emitLoopBody(ctx, loop, flowsByOrigin, flowsByDest, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)

		*lines = append(*lines, indentStr+loopEndKeyword(loop)+";")
		recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)

		// Continue after the loop within the branch
		flows := flowsByOrigin[currentID]
		for _, flow := range flows {
			traverseFlowUntilMerge(ctx, flow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
		}
		return
	}

	// Regular activity
	startLine := len(*lines) + headerLineCount
	normalFlows := findNormalFlows(flowsByOrigin[currentID])
	emitActivityStatement(ctx, obj, stmt, flowsByOrigin, flowsByDest, activityMap, entityNames, microflowNames, lines, indentStr, annotationsByTarget, labels, sourceMap, headerLineCount)
	recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)

	// An activity with no way out ends the loop iteration (a loop body is the
	// only place Studio Pro accepts one). When the description goes on after
	// this branch closes — the join leads somewhere, or an enclosing branch's
	// does — closing the branch would send this path into what follows, so it
	// has to say where it ends (Evora: GenAICommons.Trace_GetModelSpanInput,
	// whose collecting arm fell into its siblings' `break`). Events are not
	// included: they end the path by definition.
	if _, isAction := obj.(*microflows.ActionActivity); isAction && len(normalFlows) == 0 && ctx != nil && ctx.describeFallsOn {
		*lines = append(*lines, indentStr+"continue;")
	}

	// Follow normal (non-error-handler) outgoing flows until merge
	for _, flow := range normalFlows {
		traverseFlowUntilMerge(ctx, flow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
	}
}

// fallsOnBefore marks what is about to be walked as followed by next's path,
// when that path goes on, and returns the function that undoes it.
func fallsOnBefore(ctx *ExecContext, next *microflows.SequenceFlow, activityMap map[model.ID]microflows.MicroflowObject, flowsByOrigin map[model.ID][]*microflows.SequenceFlow) func() {
	if ctx == nil || ctx.describeFallsOn || next == nil || !joinGoesOn(next.DestinationID, activityMap, flowsByOrigin) {
		return func() {}
	}
	ctx.describeFallsOn = true
	return func() { ctx.describeFallsOn = false }
}

// joinGoesOn reports whether a path that reaches mergeID goes on to an object
// — rather than ending the loop iteration there, as a merge with no way out (or
// one leading only to more such merges, or to a ContinueEvent) does. A branch
// that stops dead before such a join needs no `continue`: it and the join end
// the iteration alike.
func joinGoesOn(mergeID model.ID, activityMap map[model.ID]microflows.MicroflowObject, flowsByOrigin map[model.ID][]*microflows.SequenceFlow) bool {
	seen := map[model.ID]bool{}
	var goesOn func(id model.ID) bool
	goesOn = func(id model.ID) bool {
		if id == "" || seen[id] {
			return false
		}
		seen[id] = true
		switch activityMap[id].(type) {
		case *microflows.ExclusiveMerge:
		case *microflows.ContinueEvent, nil:
			return false
		default:
			return true
		}
		for _, f := range findNormalFlows(flowsByOrigin[id]) {
			if goesOn(f.DestinationID) {
				return true
			}
		}
		return false
	}
	return goesOn(mergeID)
}

func continueAfterSplitJoin(
	ctx *ExecContext,
	joinID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	splitMergeMap map[model.ID]model.ID,
	visited map[model.ID]bool,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	lines *[]string,
	indent int,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
	annotationsByTarget *annotationEmitter,
	labels mergeLabels,
) {
	if joinID == "" {
		return
	}
	if _, isMerge := activityMap[joinID].(*microflows.ExclusiveMerge); isMerge {
		// Crossed: the fall-through path out of the split ends here and says so.
		// Declaring it inline instead would put the declaration before the
		// section that owns it and orphan the other branches' joins.
		if labels.isCrossed(joinID) {
			if label, ok := labels.of(joinID); ok {
				*lines = append(*lines, strings.Repeat("  ", indent)+"join "+label+";")
				return
			}
		}
		if label, ok := labels.of(joinID); ok && !visited[joinID] {
			*lines = append(*lines, mergeDeclarationLines(indent, label, activityMap[joinID], annotationsByTarget.layoutKeep(), flowsByOrigin)...)
		}
		if emitJoinAsLoopHeader(ctx, joinID, "", activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels) {
			return
		}
		visited[joinID] = true
		for _, flow := range flowsByOrigin[joinID] {
			traverseFlow(ctx, flow.DestinationID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
		}
		return
	}
	traverseFlow(ctx, joinID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
}

func continueAfterNestedSplitJoin(
	ctx *ExecContext,
	joinID model.ID,
	parentMergeID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	splitMergeMap map[model.ID]model.ID,
	visited map[model.ID]bool,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	lines *[]string,
	indent int,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
	annotationsByTarget *annotationEmitter,
	labels mergeLabels,
) {
	if joinID == "" || joinID == parentMergeID {
		return
	}
	if _, isMerge := activityMap[joinID].(*microflows.ExclusiveMerge); isMerge {
		// The same rule as continueAfterSplitJoin, which this lacked: a crossed
		// merge is where the path STOPS and says so. Walked through instead, its
		// suffix came out here and again in its own section (a shared region
		// printed twice), or — when this walk marked it visited first — the
		// section never declared it and other branches' joins dangled.
		if joinCrossedMerge(joinID, labels, lines, indent) {
			return
		}
		if label, ok := labels.of(joinID); ok && !visited[joinID] {
			*lines = append(*lines, mergeDeclarationLines(indent, label, activityMap[joinID], annotationsByTarget.layoutKeep(), flowsByOrigin)...)
		}
		if emitJoinAsLoopHeader(ctx, joinID, parentMergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels) {
			return
		}
		visited[joinID] = true
		for _, flow := range flowsByOrigin[joinID] {
			traverseFlowUntilMerge(ctx, flow.DestinationID, parentMergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
		}
		return
	}
	traverseFlowUntilMerge(ctx, joinID, parentMergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
}

// armFallsInto reports whether some arm of splitID reaches joinID by falling
// out of the `if` — rather than every arm returning, or ending in a `join` of
// a crossed merge first. When none does, nothing arrives at the join after
// `end if`, and a `join` of it there would be an edge out of nothing (a path
// the builder then invents). Only a crossed join can be left unreached this
// way; every other join is reported as reached.
func armFallsInto(splitID, joinID model.ID, flowsByOrigin map[model.ID][]*microflows.SequenceFlow, labels mergeLabels) bool {
	if !labels.isCrossed(joinID) {
		return true
	}
	seen := map[model.ID]bool{splitID: true}
	queue := []model.ID{}
	for _, f := range findNormalFlows(flowsByOrigin[splitID]) {
		queue = append(queue, f.DestinationID)
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if id == joinID {
			return true
		}
		if id == "" || seen[id] || labels.isCrossed(id) {
			continue
		}
		seen[id] = true
		for _, f := range findNormalFlows(flowsByOrigin[id]) {
			queue = append(queue, f.DestinationID)
		}
	}
	return false
}

// emitJoinAsLoopHeader writes a split's join as `while true` when it is also
// the header of a loop — when a path out of it comes back to it without passing
// stopID or anything already printed. Studio Pro draws one merge for both jobs;
// MDL spells them as two statements, the `if` closing and the loop opening.
// Walked on as a plain join, the way round came back to an activity already
// printed and stopped in silence, so the description had no back-edge (Evora:
// SnowflakeRESTSQL.POST_v1_ExecuteStatement, a 202 poll). Reports whether it
// wrote the loop.
func emitJoinAsLoopHeader(
	ctx *ExecContext,
	joinID model.ID,
	stopID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	splitMergeMap map[model.ID]model.ID,
	visited map[model.ID]bool,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	lines *[]string,
	indent int,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
	annotationsByTarget *annotationEmitter,
	labels mergeLabels,
) bool {
	if visited[joinID] || !mergeIsUndescribedLoopHeader(joinID, stopID, flowsByOrigin, visited) {
		return false
	}
	visited[joinID] = true
	*lines = append(*lines, strings.Repeat("  ", indent)+"while true")
	*lines = append(*lines, strings.Repeat("  ", indent)+"begin")
	for _, flow := range findNormalFlows(flowsByOrigin[joinID]) {
		traverseFlowUntilMerge(ctx, flow.DestinationID, joinID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget, labels)
	}
	*lines = append(*lines, strings.Repeat("  ", indent)+"end while;")
	return true
}

// resolveNestedMergeID decides where a nested split's branches stop: at its own
// join, or at the enclosing one (parentMergeID) when the nested join lies past
// the enclosing join.
//
// "Lies past" is the parent reaching the nested join WITHOUT going through the
// split. Inside a `while true` the parent is the loop header and is on a cycle,
// so it reaches everything in the body — but only round the loop, through this
// very split. Counting that path paired the split with the header, and the step
// after its own join was printed inside the THEN branch (mendixlabs/mxcli#1829):
// rebuilt, the ELSE branch skipped it.
func resolveNestedMergeID(
	splitID model.ID,
	nestedMergeID model.ID,
	parentMergeID model.ID,
	trueFlow *microflows.SequenceFlow,
	falseFlow *microflows.SequenceFlow,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
) model.ID {
	if nestedMergeID != "" && parentMergeID != "" && nestedMergeID != parentMergeID &&
		canReachNode(parentMergeID, nestedMergeID, flowsByOrigin, map[model.ID]bool{splitID: true}) {
		for _, flow := range []*microflows.SequenceFlow{trueFlow, falseFlow} {
			if flow == nil {
				continue
			}
			if flow.DestinationID == parentMergeID ||
				canReachNode(flow.DestinationID, parentMergeID, flowsByOrigin, make(map[model.ID]bool)) {
				return parentMergeID
			}
		}
	}
	if nestedMergeID != "" || parentMergeID == "" {
		return nestedMergeID
	}
	for _, flow := range []*microflows.SequenceFlow{trueFlow, falseFlow} {
		if flow == nil {
			continue
		}
		if canReachNode(flow.DestinationID, parentMergeID, flowsByOrigin, make(map[model.ID]bool)) {
			return parentMergeID
		}
	}
	return ""
}

func canReachNode(
	currentID model.ID,
	targetID model.ID,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	visited map[model.ID]bool,
) bool {
	if currentID == "" {
		return false
	}
	if currentID == targetID {
		return true
	}
	if visited[currentID] {
		return false
	}
	visited[currentID] = true
	for _, flow := range findNormalFlows(flowsByOrigin[currentID]) {
		if canReachNode(flow.DestinationID, targetID, flowsByOrigin, visited) {
			return true
		}
	}
	return false
}

// traverseLoopBody traverses activities inside a loop body.
// When sourceMap is non-nil, it also records line ranges for each activity node.
func traverseLoopBody(
	ctx *ExecContext,
	currentID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	splitMergeMap map[model.ID]model.ID,
	visited map[model.ID]bool,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	lines *[]string,
	indent int,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
	annotationsByTarget *annotationEmitter,
	labels mergeLabels,
) {
	// Loop bodies can contain the same structured control flow as top-level
	// microflows. Reuse the main traversal with a loop-local split/merge map so
	// nested IF/ELSE blocks emit `else` / `end if;` correctly.
	loopSplitMergeMap := findSplitMergePointsForGraph(ctx, activityMap, flowsByOrigin)
	traverseFlow(ctx, currentID, activityMap, flowsByOrigin, flowsByDest, loopSplitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, labels)
}

// emitLoopBody processes the inner objects of a LoopedActivity.
// Shared by traverseFlow and traverseLoopBody for both top-level and nested loops.
func emitLoopBody(
	ctx *ExecContext,
	loop *microflows.LoopedActivity,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	lines *[]string,
	indent int,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
	annotationsByTarget *annotationEmitter,
) {
	if loop.ObjectCollection == nil || len(loop.ObjectCollection.Objects) == 0 {
		return
	}

	// A loop body is a fresh iteration: nothing outside it follows its end.
	if ctx != nil {
		prevFallsOn := ctx.describeFallsOn
		ctx.describeFallsOn = false
		defer func() { ctx.describeFallsOn = prevFallsOn }()
	}

	loopAnnotationsByTarget := annotationsByTarget.withOverlay(buildAnnotationsByTarget(loop.ObjectCollection))

	// Build a map of objects in the loop body
	loopActivityMap := make(map[model.ID]microflows.MicroflowObject)
	for _, loopObj := range loop.ObjectCollection.Objects {
		loopActivityMap[loopObj.GetID()] = loopObj
	}
	loopObjectIDs := collectLoopObjectIDs(loop.ObjectCollection)

	// Build flow graph from the loop's own ObjectCollection flows
	loopFlowsByOrigin := make(map[model.ID][]*microflows.SequenceFlow)
	if loop.ObjectCollection != nil {
		for _, flow := range loop.ObjectCollection.Flows {
			loopFlowsByOrigin[flow.OriginID] = append(loopFlowsByOrigin[flow.OriginID], flow)
		}
	}
	// Also include parent flows that originate from loop body objects (for backward compatibility)
	for originID, flows := range flowsByOrigin {
		if loopObjectIDs[originID] {
			for _, flow := range flows {
				loopFlowsByOrigin[originID] = appendSequenceFlowIfMissing(loopFlowsByOrigin[originID], flow)
			}
		}
	}

	// flowsByDest for the loop body mirrors the top-level one: anchor sides
	// depend on where incoming flows land, which is the parent-level map for
	// flows entering the loop boundary and the loop's own for internal edges.
	loopFlowsByDest := make(map[model.ID][]*microflows.SequenceFlow)
	if loop.ObjectCollection != nil {
		for _, flow := range loop.ObjectCollection.Flows {
			loopFlowsByDest[flow.DestinationID] = append(loopFlowsByDest[flow.DestinationID], flow)
		}
	}
	for destID, flows := range flowsByDest {
		if loopObjectIDs[destID] {
			for _, flow := range flows {
				loopFlowsByDest[destID] = appendSequenceFlowIfMissing(loopFlowsByDest[destID], flow)
			}
		}
	}

	// Find the first activity in the loop body (the one with no incoming flow from within the loop)
	incomingCount := make(map[model.ID]int)
	for _, loopObj := range loop.ObjectCollection.Objects {
		if isFlowNode(loopObj) {
			incomingCount[loopObj.GetID()] = 0
		}
	}
	for _, flows := range loopFlowsByOrigin {
		for _, flow := range flows {
			if _, inLoop := loopActivityMap[flow.DestinationID]; inLoop {
				incomingCount[flow.DestinationID]++
			}
		}
	}
	var firstID model.ID
	var firstObj microflows.MicroflowObject
	for id, count := range incomingCount {
		obj := loopActivityMap[id]
		if count == 0 && preferLoopBodyStart(obj, firstObj) {
			firstID = id
			firstObj = obj
		}
	}

	// Traverse the loop body
	if firstID != "" {
		loopVisited := make(map[model.ID]bool)
		// Build split→merge map for ExclusiveSplit handling inside the loop
		loopSplitMergeMap := findSplitMergePoints(ctx, loop.ObjectCollection, loopActivityMap)
		traverseLoopBody(ctx, firstID, loopActivityMap, loopFlowsByOrigin, loopFlowsByDest, loopSplitMergeMap, loopVisited, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, loopAnnotationsByTarget, mergeLabels{})
	}
}

func collectLoopObjectIDs(oc *microflows.MicroflowObjectCollection) map[model.ID]bool {
	result := make(map[model.ID]bool)
	collectLoopObjectIDsInto(oc, result)
	return result
}

func collectLoopObjectIDsInto(oc *microflows.MicroflowObjectCollection, result map[model.ID]bool) {
	if oc == nil {
		return
	}
	for _, obj := range oc.Objects {
		if obj == nil {
			continue
		}
		result[obj.GetID()] = true
		if loop, ok := obj.(*microflows.LoopedActivity); ok {
			collectLoopObjectIDsInto(loop.ObjectCollection, result)
		}
	}
}

func appendSequenceFlowIfMissing(flows []*microflows.SequenceFlow, candidate *microflows.SequenceFlow) []*microflows.SequenceFlow {
	if candidate == nil {
		return flows
	}
	candidateKey := sequenceFlowIdentity(candidate)
	for _, flow := range flows {
		if sequenceFlowIdentity(flow) == candidateKey {
			return flows
		}
	}
	return append(flows, candidate)
}

// sequenceFlowIdentity returns a stable key used to deduplicate flows when
// merging the parent graph into a loop's local map. Production data always
// carries a UUID, so the ID branch is the common path. The composite fallback
// covers test helpers and any other call site that constructs flows without
// IDs — including CaseValue prevents two split branches with the same
// origin/destination but different case values from being mistakenly
// deduplicated.
func sequenceFlowIdentity(flow *microflows.SequenceFlow) string {
	if flow == nil {
		return ""
	}
	if flow.ID != "" {
		return string(flow.ID)
	}
	return fmt.Sprintf("%s>%s:%t:%d:%d:%v", flow.OriginID, flow.DestinationID, flow.IsErrorHandler, flow.OriginConnectionIndex, flow.DestinationConnectionIndex, flow.CaseValue)
}

// isFlowNode reports whether an object participates in the sequence-flow graph,
// and so can legitimately start a loop body.
//
// An Annotation is an object in ObjectCollection.Objects but connects through
// AnnotationFlows, never through Flows — so its incoming-sequence-flow count is
// permanently 0 and it looked like a start candidate to preferLoopBodyStart,
// which picks leftmost-then-topmost. Studio Pro places annotations above/left of
// what they describe, so the annotation usually won, traversal began at a node
// with no outgoing sequence flow, and the entire loop body vanished from
// describe output — silently, at exit 0 (#965). Annotations reach the output
// through annotationsByTarget instead, attached to the activity they point at.
func isFlowNode(obj microflows.MicroflowObject) bool {
	_, isAnnotation := obj.(*microflows.Annotation)
	return !isAnnotation
}

func preferLoopBodyStart(candidate, current microflows.MicroflowObject) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}
	candidatePos := candidate.GetPosition()
	currentPos := current.GetPosition()
	if candidatePos.X != currentPos.X {
		return candidatePos.X < currentPos.X
	}
	return candidatePos.Y < currentPos.Y
}

// isMergePairedWithSplit reports whether an ExclusiveMerge appears as the
// matching end-of-branch point for some ExclusiveSplit recorded in
// splitMergeMap (i.e., it closes an IF/ELSE block). Merges that aren't paired
// — e.g. a junction used as the loop-back target of a manual retry pattern —
// must be traversed as pass-through, otherwise every activity after them is
// dropped from describe output (issue #281).
//
// splitMergeMap is split-ID → merge-ID, so the merge is paired iff it appears
// as a value in the map.
func isMergePairedWithSplit(mergeID model.ID, splitMergeMap map[model.ID]model.ID) bool {
	for _, v := range splitMergeMap {
		if v == mergeID {
			return true
		}
	}
	return false
}

func mergeHasLoopBackEdge(mergeID model.ID, flowsByOrigin map[model.ID][]*microflows.SequenceFlow) bool {
	for _, flow := range findNormalFlows(flowsByOrigin[mergeID]) {
		if reachesObject(flow.DestinationID, mergeID, flowsByOrigin, map[model.ID]bool{}) {
			return true
		}
	}
	return false
}

// mergeIsUndescribedLoopHeader reports whether a merge heads a loop the
// description has not entered yet: some path out of it comes back to it
// without passing the enclosing stop or anything already printed.
func mergeIsUndescribedLoopHeader(mergeID, stopID model.ID, flowsByOrigin map[model.ID][]*microflows.SequenceFlow, visited map[model.ID]bool) bool {
	blocked := map[model.ID]bool{}
	for id, v := range visited {
		if v {
			blocked[id] = true
		}
	}
	if stopID != "" && stopID != mergeID {
		blocked[stopID] = true
	}
	for _, flow := range findNormalFlows(flowsByOrigin[mergeID]) {
		if reachesObject(flow.DestinationID, mergeID, flowsByOrigin, cloneVisited(blocked)) {
			return true
		}
	}
	return false
}

func reachesObject(currentID, targetID model.ID, flowsByOrigin map[model.ID][]*microflows.SequenceFlow, visited map[model.ID]bool) bool {
	if currentID == "" || visited[currentID] {
		return false
	}
	if currentID == targetID {
		return true
	}
	visited[currentID] = true
	for _, flow := range findNormalFlows(flowsByOrigin[currentID]) {
		if reachesObject(flow.DestinationID, targetID, flowsByOrigin, visited) {
			return true
		}
	}
	return false
}

func emitEnumSplitStatement(
	ctx *ExecContext,
	currentID model.ID,
	mergeID model.ID,
	source string,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	splitMergeMap map[model.ID]model.ID,
	visited map[model.ID]bool,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	lines *[]string,
	indent int,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
	annotationsByTarget *annotationEmitter,
	labels mergeLabels,
) {
	indentStr := strings.Repeat("  ", indent)
	*lines = append(*lines, indentStr+"case "+source)

	type enumBranch struct {
		values []string
		flow   *microflows.SequenceFlow
	}
	branches := []enumBranch{}
	branchByDestination := map[model.ID]int{}
	var elseFlow *microflows.SequenceFlow
	for _, flow := range orderedEnumSplitFlowsFor(ctx, findNormalFlows(flowsByOrigin[currentID]), activityMap) {
		caseValue, ok := enumCaseValue(flow)
		if !ok {
			elseFlow = flow
			continue
		}
		if idx, ok := branchByDestination[flow.DestinationID]; ok {
			branches[idx].values = append(branches[idx].values, caseValue)
			continue
		}
		branchByDestination[flow.DestinationID] = len(branches)
		branches = append(branches, enumBranch{values: []string{caseValue}, flow: flow})
	}

	// Bodies traverse at indent+2, one level in from their `when` at indent+1.
	// They used to traverse at indent+1 — the SAME column as the `when` — which
	// made a nested `if`'s `else` land exactly where a reader expects a branch
	// keyword, in output where an `else` on a `case` is an MDL008 error (#913).
	for _, branch := range branches {
		*lines = append(*lines, indentStr+"  when "+formatEnumSplitCaseValues(branch.values)+" then")
		traverseFlowUntilMerge(ctx, branch.flow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, cloneVisited(visited), entityNames, microflowNames, lines, indent+2, sourceMap, headerLineCount, annotationsByTarget, labels)
	}
	if elseFlow != nil {
		*lines = append(*lines, indentStr+"  else")
		traverseFlowUntilMerge(ctx, elseFlow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, cloneVisited(visited), entityNames, microflowNames, lines, indent+2, sourceMap, headerLineCount, annotationsByTarget, labels)
	}

	*lines = append(*lines, indentStr+"end case;")
}

func emitInheritanceSplitStatement(
	ctx *ExecContext,
	currentID model.ID,
	stopID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	splitMergeMap map[model.ID]model.ID,
	visited map[model.ID]bool,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	lines *[]string,
	indent int,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
	annotationsByTarget *annotationEmitter,
	labels mergeLabels,
) {
	split, _ := activityMap[currentID].(*microflows.InheritanceSplit)
	if split == nil {
		return
	}
	varName := split.VariableName
	if !strings.HasPrefix(varName, "$") {
		varName = "$" + varName
	}
	indentStr := strings.Repeat("  ", indent)
	*lines = append(*lines, indentStr+"split type "+varName)

	branchStopID := splitMergeMap[currentID]
	if branchStopID == "" {
		branchStopID = stopID
	}

	var elseFlow *microflows.SequenceFlow
	for _, flow := range orderedInheritanceSplitFlowsFor(ctx, findNormalFlows(flowsByOrigin[currentID]), activityMap) {
		caseName, ok := inheritanceCaseName(flow, entityNames)
		if !ok {
			elseFlow = flow
			continue
		}
		*lines = append(*lines, indentStr+"  when "+caseName+" then")
		traverseFlowUntilMerge(ctx, flow.DestinationID, branchStopID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, cloneVisited(visited), entityNames, microflowNames, lines, indent+2, sourceMap, headerLineCount, annotationsByTarget, labels)
	}
	if elseFlow != nil {
		elseLineIdx := len(*lines)
		*lines = append(*lines, indentStr+"  when (empty) then")
		traverseFlowUntilMerge(ctx, elseFlow.DestinationID, branchStopID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, cloneVisited(visited), entityNames, microflowNames, lines, indent+2, sourceMap, headerLineCount, annotationsByTarget, labels)
		// Remove an empty else block, as the if/else emitters above do. On an
		// object-type decision this flow is the `(empty)` case (for a null
		// object), which the builder emits unconditionally — CE0089 without it —
		// so a split that never had an authored `else` still has the flow. Left
		// in, DESCRIBE printed a bare `else` the author never wrote and a
		// describe→exec roundtrip accumulated one each pass. Exec re-creates the
		// flow, so dropping the empty rendering is lossless.
		if len(*lines) == elseLineIdx+1 {
			*lines = (*lines)[:elseLineIdx]
		}
	}
	*lines = append(*lines, indentStr+"end split;")
}

// enumSplitSource is the MDL subject of an enumeration split: `$Var` /
// `$Obj/Attr` for an expression split, or the rule call for a rule split that
// returns an enumeration — `Mod.Rule(Param = $x)`, spelled as an `if` on a rule
// spells it. A rule split used to fall through to the boolean `if` path, which
// found no true/false flows and printed empty branches and an orphan merge
// (mendixlabs/mxcli#1414).
func enumSplitSource(ctx *ExecContext, split *microflows.ExclusiveSplit) (string, bool) {
	if variable, ok := enumSplitVariable(split); ok {
		return "$" + variable, true
	}
	if split == nil {
		return "", false
	}
	if rule, ok := split.SplitCondition.(*microflows.RuleSplitCondition); ok && rule.RuleQualifiedName != "" {
		return formatSplitCondition(ctx, rule), true
	}
	return "", false
}

// hasEnumSplitSource is enumSplitSource without the rendering.
func hasEnumSplitSource(split *microflows.ExclusiveSplit) bool {
	if _, ok := enumSplitVariable(split); ok {
		return true
	}
	rule, ok := split.SplitCondition.(*microflows.RuleSplitCondition)
	return ok && rule.RuleQualifiedName != ""
}

func enumSplitVariable(split *microflows.ExclusiveSplit) (string, bool) {
	if split == nil {
		return "", false
	}
	cond, ok := split.SplitCondition.(*microflows.ExpressionSplitCondition)
	if !ok {
		return "", false
	}
	expr := strings.TrimSpace(cond.Expression)
	if !strings.HasPrefix(expr, "$") || expr == "$" {
		return "", false
	}
	return strings.TrimPrefix(expr, "$"), true
}

func hasEnumCaseFlows(flows []*microflows.SequenceFlow) bool {
	for _, flow := range flows {
		if value, ok := enumCaseValue(flow); ok && value != "true" && value != "false" {
			return true
		}
	}
	return false
}

func enumCaseValue(flow *microflows.SequenceFlow) (string, bool) {
	if flow == nil || flow.CaseValue == nil {
		return "", false
	}
	switch cv := flow.CaseValue.(type) {
	case *microflows.EnumerationCase:
		return cv.Value, true
	case microflows.EnumerationCase:
		return cv.Value, true
	default:
		return "", false
	}
}

func inheritanceCaseName(flow *microflows.SequenceFlow, entityNames map[model.ID]string) (string, bool) {
	if flow == nil || flow.CaseValue == nil {
		return "", false
	}
	switch cv := flow.CaseValue.(type) {
	case *microflows.InheritanceCase:
		if cv.EntityQualifiedName != "" {
			return cv.EntityQualifiedName, true
		}
		if name := entityNames[cv.EntityID]; name != "" {
			return name, true
		}
	case microflows.InheritanceCase:
		if cv.EntityQualifiedName != "" {
			return cv.EntityQualifiedName, true
		}
		if name := entityNames[cv.EntityID]; name != "" {
			return name, true
		}
	}
	return "", false
}

// orderedEnumSplitFlows puts a split's case flows back in the order they were
// written. Stored flow order does not survive serialization, so the order is read
// from what does: the anchor pair (splitCaseOrder), then how far down the canvas the
// branch sits.
//
// The pair alone used to carry the whole order, one distinct pair per case. A split
// of four or more cases now shares three pairs — top, right and bottom of the split,
// each arriving on the left (enumSplitOriginAnchors) — which the table already ranks
// in that order, and the branches inside a group are stacked top to bottom in case
// order, so their Y finishes the job. A model written with one pair per case never
// reaches the tie-break and reads exactly as before; a split drawn by hand in Studio
// Pro reads side first, then top to bottom.
func orderedEnumSplitFlows(flows []*microflows.SequenceFlow, activityMap map[model.ID]microflows.MicroflowObject) []*microflows.SequenceFlow {
	ordered := append([]*microflows.SequenceFlow(nil), flows...)
	branchY := func(flow *microflows.SequenceFlow) int {
		if flow == nil {
			return 0
		}
		if obj := activityMap[flow.DestinationID]; obj != nil {
			return obj.GetPosition().Y
		}
		return 0
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ri, rj := splitCaseOrder(ordered[i]), splitCaseOrder(ordered[j]); ri != rj {
			return ri < rj
		}
		return branchY(ordered[i]) < branchY(ordered[j])
	})
	return ordered
}

// orderedEnumSplitFlowsFor is orderedEnumSplitFlows for the flow being described.
// In a flow that records case-split columns (ako/mxcli#1832) every outcome after
// the first leaves the split's bottom for the top of its own column: one side
// pair and one depth, so the order is finished by how far right the column
// stands. The tie-break is kept to that style: any other drawing reads as before.
func orderedEnumSplitFlowsFor(ctx *ExecContext, flows []*microflows.SequenceFlow, activityMap map[model.ID]microflows.MicroflowObject) []*microflows.SequenceFlow {
	ordered := orderedEnumSplitFlows(flows, activityMap)
	if describesCaseColumns(ctx) {
		sortColumnsLeftToRight(ordered, splitCaseOrder, activityMap)
	}
	return ordered
}

// orderedInheritanceSplitFlowsFor is orderedInheritanceSplitFlows for the flow
// being described, finished left to right in a flow that records case-split
// columns (see orderedEnumSplitFlowsFor).
func orderedInheritanceSplitFlowsFor(ctx *ExecContext, flows []*microflows.SequenceFlow, activityMap map[model.ID]microflows.MicroflowObject) []*microflows.SequenceFlow {
	ordered := orderedInheritanceSplitFlows(flows)
	if describesCaseColumns(ctx) {
		sortColumnsLeftToRight(ordered, inheritanceSplitCaseOrder, activityMap)
	}
	return ordered
}

func describesCaseColumns(ctx *ExecContext) bool {
	return ctx != nil && ctx.describedFlowLayout != nil && ctx.describedFlowLayout.CaseSplit == CaseSplitColumns
}

// sortColumnsLeftToRight orders flows by their side pair's rank, then by how far
// right their destination stands, then how far down. Columns share a top edge,
// not a centre — a column starting with an end event has its centre higher than
// one starting with an activity — so x decides, not y.
func sortColumnsLeftToRight(flows []*microflows.SequenceFlow, rank func(*microflows.SequenceFlow) int, activityMap map[model.ID]microflows.MicroflowObject) {
	at := func(f *microflows.SequenceFlow) model.Point {
		if f != nil {
			if obj := activityMap[f.DestinationID]; obj != nil {
				return obj.GetPosition()
			}
		}
		return model.Point{}
	}
	sort.SliceStable(flows, func(i, j int) bool {
		if ri, rj := rank(flows[i]), rank(flows[j]); ri != rj {
			return ri < rj
		}
		pi, pj := at(flows[i]), at(flows[j])
		if pi.X != pj.X {
			return pi.X < pj.X
		}
		return pi.Y < pj.Y
	})
}

func orderedInheritanceSplitFlows(flows []*microflows.SequenceFlow) []*microflows.SequenceFlow {
	ordered := append([]*microflows.SequenceFlow(nil), flows...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return inheritanceSplitCaseOrder(ordered[i]) < inheritanceSplitCaseOrder(ordered[j])
	})
	return ordered
}

func splitCaseOrder(flow *microflows.SequenceFlow) int {
	if flow == nil {
		return 1 << 20
	}
	for i, pair := range splitCaseOrderAnchors {
		if flow.OriginConnectionIndex == pair.origin && flow.DestinationConnectionIndex == pair.destination {
			return i
		}
	}
	return (1 << 10) + flow.OriginConnectionIndex*4 + flow.DestinationConnectionIndex
}

func inheritanceSplitCaseOrder(flow *microflows.SequenceFlow) int {
	if flow == nil {
		return 1 << 20
	}
	for i, pair := range inheritanceSplitCaseOrderAnchors {
		if flow.OriginConnectionIndex == pair.origin && flow.DestinationConnectionIndex == pair.destination {
			return i
		}
	}
	return (1 << 10) + flow.OriginConnectionIndex*4 + flow.DestinationConnectionIndex
}

func formatEnumSplitCaseValue(value string) string {
	if value == "" || value == "(empty)" {
		return "(empty)"
	}
	return value
}

func formatEnumSplitCaseValues(values []string) string {
	formatted := make([]string, 0, len(values))
	for _, value := range values {
		formatted = append(formatted, formatEnumSplitCaseValue(value))
	}
	return strings.Join(formatted, ", ")
}

func cloneVisited(visited map[model.ID]bool) map[model.ID]bool {
	cloned := make(map[model.ID]bool, len(visited))
	for id, seen := range visited {
		cloned[id] = seen
	}
	return cloned
}

// findBranchFlows separates flows from a split into TRUE and FALSE branches based on CaseValue.
// Returns (trueFlow, falseFlow). Either may be nil if the branch doesn't exist.
//
// A binary exclusive split has exactly one true and one false branch. When a
// project stores a branch's case value in a shape this matcher doesn't
// recognize (e.g. an ExpressionCase carrying the condition text instead of the
// literal "true"/"false" sentinel), the recognized side is still matched and
// the unrecognized side is left for by-elimination recovery below — otherwise
// the whole branch body would be dropped (DESCRIBE skipping the then part).
func findBranchFlows(flows []*microflows.SequenceFlow) (trueFlow, falseFlow *microflows.SequenceFlow) {
	var unmatched []*microflows.SequenceFlow
	for _, flow := range flows {
		if flow.IsErrorHandler {
			continue
		}
		if flow.CaseValue == nil {
			unmatched = append(unmatched, flow)
			continue
		}
		switch cv := flow.CaseValue.(type) {
		case *microflows.ExpressionCase:
			switch cv.Expression {
			case "true":
				trueFlow = flow
			case "false":
				falseFlow = flow
			default:
				unmatched = append(unmatched, flow)
			}
		case *microflows.EnumerationCase:
			switch cv.Value {
			case "true":
				trueFlow = flow
			case "false":
				falseFlow = flow
			default:
				unmatched = append(unmatched, flow)
			}
		case microflows.EnumerationCase:
			switch cv.Value {
			case "true":
				trueFlow = flow
			case "false":
				falseFlow = flow
			default:
				unmatched = append(unmatched, flow)
			}
		case *microflows.BooleanCase:
			if cv.Value {
				trueFlow = flow
			} else {
				falseFlow = flow
			}
		case microflows.BooleanCase:
			if cv.Value {
				trueFlow = flow
			} else {
				falseFlow = flow
			}
		default:
			unmatched = append(unmatched, flow)
		}
	}

	// By elimination: if exactly one side was recognized and a single other
	// branch was left unmatched, that branch must be the missing side. This
	// keeps the then (or else) body from being dropped when its case value is
	// stored in an unrecognized shape. Only applied for a clean 1-and-1 split —
	// if both sides are unknown we can't tell true from false, so we leave them
	// nil rather than guess.
	if len(unmatched) == 1 {
		if trueFlow == nil && falseFlow != nil {
			trueFlow = unmatched[0]
		} else if falseFlow == nil && trueFlow != nil {
			falseFlow = unmatched[0]
		}
	}
	return trueFlow, falseFlow
}

func flowLooksLikeGuardContinuation(
	ctx *ExecContext,
	flow *microflows.SequenceFlow,
	split microflows.MicroflowObject,
	activityMap map[model.ID]microflows.MicroflowObject,
) bool {
	if flow == nil || split == nil {
		return false
	}
	dest := activityMap[flow.DestinationID]
	if dest == nil {
		return false
	}
	switch dest.(type) {
	case *microflows.EndEvent, *microflows.ErrorEvent:
		return false
	}
	// Builder-generated guard continuations sit on the split's horizontal
	// centerline and use the builder's horizontal split→tail flow. This
	// intentionally relies on mxcli's layout/anchor contract so a real false
	// branch whose activities happen to be aligned with the split is not
	// collapsed into a guard-style continuation during describe.
	if dest.GetPosition().Y == split.GetPosition().Y &&
		flow.OriginConnectionIndex == AnchorRight &&
		flow.DestinationConnectionIndex == AnchorLeft {
		return true
	}
	// A flow drawn with `guard: true-right` (ako/mxcli#1832) continues straight
	// down from the split's bottom instead. Only a flow that records that style is
	// read this way: in any other the same shape is an ELSE column, and describing
	// it as a guard would not rebuild where it is.
	if ctx != nil && ctx.describedFlowLayout != nil && ctx.describedFlowLayout.Guard == GuardTrueRight {
		return dest.GetPosition().X == split.GetPosition().X &&
			dest.GetPosition().Y > split.GetPosition().Y &&
			flow.OriginConnectionIndex == AnchorBottom &&
			flow.DestinationConnectionIndex == AnchorTop
	}
	return false
}

// findErrorHandlerFlow returns the error handler flow from an activity's outgoing flows.
func findErrorHandlerFlow(flows []*microflows.SequenceFlow) *microflows.SequenceFlow {
	for _, flow := range flows {
		if flow.IsErrorHandler {
			return flow
		}
	}
	return nil
}

// findNormalFlows returns all non-error-handler flows from an activity.
func findNormalFlows(flows []*microflows.SequenceFlow) []*microflows.SequenceFlow {
	var result []*microflows.SequenceFlow
	for _, flow := range flows {
		if !flow.IsErrorHandler {
			result = append(result, flow)
		}
	}
	return result
}

func branchFlowTerminatesBeforeMerge(
	flow *microflows.SequenceFlow,
	mergeID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	splitMergeMap map[model.ID]model.ID,
) bool {
	if flow == nil {
		return false
	}
	return objectTerminatesBeforeMerge(flow.DestinationID, mergeID, activityMap, flowsByOrigin, splitMergeMap, map[model.ID]bool{})
}

func branchFlowStartsAtTerminal(flow *microflows.SequenceFlow, activityMap map[model.ID]microflows.MicroflowObject) bool {
	if flow == nil {
		return false
	}
	switch activityMap[flow.DestinationID].(type) {
	case *microflows.EndEvent, *microflows.ErrorEvent:
		return true
	default:
		return false
	}
}

// hasExplicitFalseBranchAnchor reports whether a false-branch sequence flow
// carries anchor metadata that the user explicitly authored. Top→Bottom is
// the non-default pair produced by `@anchor(false: (from: top, to: bottom))`;
// any other combination is either a builder default or a different author
// intention that should not trigger the guard-pattern describer.
//
// Used by the `isGuard` paths in traverseFlow / traverseFlowUntilMerge to
// distinguish a real guard continuation from a branch whose layout should
// stay visible as an explicit `else` in the described MDL. See
// TestHasExplicitFalseBranchAnchor for the exhaustive cases.
func hasExplicitFalseBranchAnchor(flow *microflows.SequenceFlow) bool {
	if flow == nil {
		return false
	}
	return flow.OriginConnectionIndex == AnchorTop && flow.DestinationConnectionIndex == AnchorBottom
}

func objectTerminatesBeforeMerge(
	currentID model.ID,
	mergeID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	splitMergeMap map[model.ID]model.ID,
	visited map[model.ID]bool,
) bool {
	if currentID == "" || currentID == mergeID || visited[currentID] {
		return false
	}
	visited[currentID] = true

	obj := activityMap[currentID]
	switch obj.(type) {
	case *microflows.EndEvent, *microflows.ErrorEvent:
		return true
	case *microflows.ExclusiveSplit, *microflows.InheritanceSplit:
		nestedMergeID := splitMergeMap[currentID]
		if nestedMergeID == "" {
			nestedMergeID = mergeID
		}
		flows := findNormalFlows(flowsByOrigin[currentID])
		if len(flows) == 0 {
			return false
		}
		for _, flow := range flows {
			if !objectTerminatesBeforeMerge(flow.DestinationID, nestedMergeID, activityMap, flowsByOrigin, splitMergeMap, cloneVisited(visited)) {
				return false
			}
		}
		return true
	case *microflows.ExclusiveMerge:
		// A non-matching merge is just an intermediate join. Follow it; only the
		// caller's mergeID is treated as the non-terminal fall-through boundary.
	}

	flows := findNormalFlows(flowsByOrigin[currentID])
	if len(flows) == 0 {
		return false
	}
	for _, flow := range flows {
		if !objectTerminatesBeforeMerge(flow.DestinationID, mergeID, activityMap, flowsByOrigin, splitMergeMap, cloneVisited(visited)) {
			return false
		}
	}
	return true
}

// formatErrorHandlingSuffix returns the ON ERROR suffix for an activity based on its ErrorHandlingType.
// Returns empty string if no special error handling.
//
// Rollback is deliberately absent: it is what convertErrorHandlingType(nil)
// stores for an activity written with no clause, and what the parser falls back
// to when ErrorHandlingType is missing from the BSON. Read-back therefore cannot
// tell an authored `on error rollback` from the default, and emitting it invents
// a clause on activities that never had one (#840). Dropping it is lossless —
// re-executing the output stores Rollback again — while emitting it is not.
//
// The remaining values are only ever set explicitly, so their presence in the
// model always means the author wrote them.
func formatErrorHandlingSuffix(errType microflows.ErrorHandlingType) string {
	switch errType {
	case microflows.ErrorHandlingTypeContinue:
		return " on error continue"
	case microflows.ErrorHandlingTypeCustom:
		return " on error" // Will be followed by block
	case microflows.ErrorHandlingTypeCustomWithoutRollback:
		return " on error without rollback" // Will be followed by block
	default:
		return "" // Rollback (the stored default) and Abort emit nothing
	}
}

// hasCustomErrorHandler returns true if the error handling type requires a custom handler block.
func hasCustomErrorHandler(errType microflows.ErrorHandlingType) bool {
	return errType == microflows.ErrorHandlingTypeCustom || errType == microflows.ErrorHandlingTypeCustomWithoutRollback
}

// getActionErrorHandlingType extracts the ErrorHandlingType from the action inside an ActionActivity.
// Most action types store ErrorHandlingType at the action level, not the activity level.
//
// This gates far more than a suffix. emitActivityStatement only walks an
// activity's error branch when hasCustomErrorHandler() agrees, so an action whose
// type is not reported here loses its ENTIRE `on error begin … end error` block from DESCRIBE
// — silently, and in valid-looking MDL, so a describe→edit→exec round-trip
// deletes the handler from the model (mendixlabs/mxcli#1078).
func getActionErrorHandlingType(activity *microflows.ActionActivity) microflows.ErrorHandlingType {
	if activity == nil || activity.Action == nil {
		return ""
	}

	// The one deliberate exclusion: the field is stored, but Mendix refuses a
	// custom handler on this action (CE6035), so reporting it would render a
	// block that cannot be executed back.
	if _, ok := activity.Action.(*microflows.RestOperationCallAction); ok {
		return ""
	}

	if errType := actionErrorHandlingField(activity.Action); errType != "" {
		return errType
	}
	// Fall back to activity level for action types without ErrorHandlingType field.
	return activity.ErrorHandlingType
}

// actionErrorHandlingField reads ErrorHandlingType off any action that declares it.
//
// Reflection rather than a case per action type — see
// microflows.ActionErrorHandlingType, which this delegates to so DESCRIBE, the
// catalog and the lint rules read the field the same way. The hand-maintained
// switch it replaced had drifted to 17 of the 38 action types that carry the
// field, and every one it missed dropped that activity's whole error branch
// from DESCRIBE.
func actionErrorHandlingField(action microflows.MicroflowAction) microflows.ErrorHandlingType {
	return microflows.ActionErrorHandlingType(action)
}

// collectErrorHandlerStatements traverses the error handler flow and collects statements.
// Returns a slice of MDL statements for the error handler block.
func collectErrorHandlerStatements(
	ctx *ExecContext,
	startID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	annotationsByTarget *annotationEmitter,
	labels mergeLabels,
) []string {
	statements, _ := collectErrorHandlerStatementSpans(ctx, startID, activityMap, flowsByOrigin, entityNames, microflowNames, annotationsByTarget, labels)
	return statements
}

// errorHandlerSpan is where, among the statements of an error handler block,
// one handler-body object is printed: its notes and its statement.
type errorHandlerSpan struct {
	id         model.ID
	start, end int
}

// recordErrorHandlerSpans enters the handler-body objects into the source map,
// base being the absolute line of the block's first statement. Without them a
// handler-body activity has no line at all, so `describe … with handles`
// printed no handle for it and ranked it after every other activity — making
// an `@n` ordinal count differently from the order describe prints in.
func recordErrorHandlerSpans(sourceMap map[string]elkSourceRange, spans []errorHandlerSpan, base int) {
	for _, sp := range spans {
		recordSourceMap(sourceMap, sp.id, base+sp.start, base+sp.end)
	}
}

// collectErrorHandlerStatementSpans is collectErrorHandlerStatements that also
// reports, per object printed, which statements it occupies.
func collectErrorHandlerStatementSpans(
	ctx *ExecContext,
	startID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	annotationsByTarget *annotationEmitter,
	labels mergeLabels,
) ([]string, []errorHandlerSpan) {
	var statements []string
	var spans []errorHandlerSpan
	visited := make(map[model.ID]bool)
	incoming := incomingCounts(flowsByOrigin)
	stopID := firstReachableErrorHandlerMerge(startID, activityMap, flowsByOrigin, incoming)

	// A handler-body activity's annotations are emitted here or nowhere: this
	// traversal is a second, smaller describer and the main one never reaches
	// inside an `on error begin … end error` block. It used to emit the notes
	// alone (#1077) — so @position, @anchor, @curve, @caption, @color and
	// @excluded, all of which the builder honours in a handler, came back
	// missing and describe → exec put a laid-out handler back on
	// auto-placement (mendixlabs/mxcli#991). Same emitter as the main path.
	flowsByDest := map[model.ID][]*microflows.SequenceFlow{}
	for _, flows := range flowsByOrigin {
		for _, f := range flows {
			flowsByDest[f.DestinationID] = append(flowsByDest[f.DestinationID], f)
		}
	}
	notes := func(obj microflows.MicroflowObject, indentStr string) {
		emitObjectAnnotations(obj, &statements, indentStr, annotationsByTarget, flowsByOrigin, flowsByDest, activityMap)
	}
	splitMergeMap := findErrorHandlerSplitMergePoints(ctx, activityMap, flowsByOrigin)

	var traverse func(id model.ID, boundary model.ID, indent int)
	traverse = func(id model.ID, boundary model.ID, indent int) {
		if id == "" {
			return
		}
		// The boundary IS the rejoin in the commonest shape — the error edge
		// lands straight on the shared merge — so the label has to be emitted
		// here, before the boundary check returns. Missing this is why the block
		// came out empty even with the merge check below in place.
		if id == boundary || visited[id] {
			if label, ok := labels.of(id); ok {
				statements = append(statements, strings.Repeat("  ", indent)+"join "+label+";")
			}
			return
		}
		obj := activityMap[id]
		if obj == nil {
			return
		}
		if _, isMerge := obj.(*microflows.ExclusiveMerge); isMerge {
			// The handler rejoins the normal path here. Saying so is the whole
			// point of `join`: without it the traversal stopped and the block
			// came out EMPTY, which re-executes to a different graph — the
			// silent rewrite Phase E is about.
			if label, ok := labels.of(id); ok {
				statements = append(statements, strings.Repeat("  ", indent)+"join "+label+";")
				return
			}
			// A merge with one way in joins nothing: walk on, as
			// firstReachableErrorHandlerMerge did to find where this ends.
			if isNoOpMerge(id, activityMap, incoming) {
				visited[id] = true
				for _, flow := range findNormalFlows(flowsByOrigin[id]) {
					traverse(flow.DestinationID, boundary, indent)
				}
			}
			return
		}
		visited[id] = true

		indentStr := strings.Repeat("  ", indent)
		if _, isSplit := obj.(*microflows.ExclusiveSplit); isSplit {
			stmt := formatActivity(ctx, obj, entityNames, microflowNames)
			if stmt != "" {
				start := len(statements)
				notes(obj, indentStr)
				statements = append(statements, indentStr+stmt)
				spans = append(spans, errorHandlerSpan{id: id, start: start, end: len(statements) - 1})
			}
			nestedMergeID := splitMergeMap[id]
			trueFlow, falseFlow := findBranchFlows(flowsByOrigin[id])
			if trueFlow != nil {
				traverse(trueFlow.DestinationID, nestedMergeID, indent+1)
			}
			if falseFlow != nil {
				elseIdx := len(statements)
				statements = append(statements, indentStr+"else")
				if falseFlow.DestinationID != nestedMergeID {
					traverse(falseFlow.DestinationID, nestedMergeID, indent+1)
				}
				if remap := foldElseIntoElsifLines(&statements, elseIdx, indent); remap != nil {
					for i := range spans {
						if spans[i].end >= elseIdx {
							spans[i].start, spans[i].end = remap(spans[i].start), remap(spans[i].end)
						}
					}
				}
			}
			if stmt != "" {
				statements = append(statements, indentStr+"end if;")
			}
			if nestedMergeID != "" && nestedMergeID != boundary {
				visited[nestedMergeID] = true
				for _, flow := range findNormalFlows(flowsByOrigin[nestedMergeID]) {
					traverse(flow.DestinationID, boundary, indent)
				}
			}
			return
		}

		if loop, isLoop := obj.(*microflows.LoopedActivity); isLoop {
			// The header alone is not a statement: without `begin … end loop;`
			// the description did not parse (ako/mxcli#1456).
			start := len(statements)
			if stmt := formatActivity(ctx, obj, entityNames, microflowNames); stmt != "" {
				notes(obj, indentStr)
				statements = append(statements, indentStr+stmt)
			}
			statements = append(statements, indentStr+"begin")
			emitLoopBody(ctx, loop, flowsByOrigin, flowsByDest, entityNames, microflowNames, &statements, indent, nil, 0, annotationsByTarget)
			statements = append(statements, indentStr+loopEndKeyword(loop)+";")
			spans = append(spans, errorHandlerSpan{id: id, start: start, end: len(statements) - 1})
		} else if stmt := formatActivity(ctx, obj, entityNames, microflowNames); stmt != "" {
			start := len(statements)
			notes(obj, indentStr)
			statements = append(statements, indentStr+stmt)
			spans = append(spans, errorHandlerSpan{id: id, start: start, end: len(statements) - 1})
		}
		for _, flow := range findNormalFlows(flowsByOrigin[id]) {
			traverse(flow.DestinationID, boundary, indent)
		}
	}

	traverse(startID, stopID, 0)
	return statements, spans
}

func findErrorHandlerSplitMergePoints(
	ctx *ExecContext,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
) map[model.ID]model.ID {
	result := make(map[model.ID]model.ID)
	for id, obj := range activityMap {
		if _, isSplit := obj.(*microflows.ExclusiveSplit); !isSplit {
			continue
		}
		if mergeID := findMergeForSplit(ctx, id, flowsByOrigin, activityMap); mergeID != "" {
			result[id] = mergeID
		}
	}
	return result
}

// firstReachableErrorHandlerMerge finds where the handler starting at startID
// joins something else: the first merge with a way in from outside the
// handler. A merge every way into which lies inside the handler — the one
// closing an `if` in the handler body — joins nothing but the handler's own
// branches, and stopping there dropped everything after `end if;` (ako/mxcli#1456).
// A merge with one way in is the degenerate case of the same rule.
func firstReachableErrorHandlerMerge(
	startID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	incoming map[model.ID]int,
) model.ID {
	inHandler := make(map[model.ID]bool)
	for queue := []model.ID{startID}; len(queue) > 0; queue = queue[1:] {
		id := queue[0]
		if id == "" || inHandler[id] {
			continue
		}
		inHandler[id] = true
		for _, flow := range findNormalFlows(flowsByOrigin[id]) {
			queue = append(queue, flow.DestinationID)
		}
	}
	joinsOutside := make(map[model.ID]bool)
	for originID, flows := range flowsByOrigin {
		if inHandler[originID] {
			continue
		}
		for _, f := range flows {
			if f != nil {
				joinsOutside[f.DestinationID] = true
			}
		}
	}

	visited := make(map[model.ID]bool)
	queue := []model.ID{startID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if id == "" || visited[id] {
			continue
		}
		visited[id] = true
		if _, isMerge := activityMap[id].(*microflows.ExclusiveMerge); isMerge && !isNoOpMerge(id, activityMap, incoming) && joinsOutside[id] {
			return id
		}
		for _, flow := range findNormalFlows(flowsByOrigin[id]) {
			queue = append(queue, flow.DestinationID)
		}
	}
	return ""
}

// loopEndKeyword returns "END WHILE" for WHILE loops and "END LOOP" for FOR-EACH loops.
func loopEndKeyword(loop *microflows.LoopedActivity) string {
	if _, isWhile := loop.LoopSource.(*microflows.WhileLoopCondition); isWhile {
		return "end while"
	}
	return "end loop"
}

// --- Executor method wrappers for callers in unmigrated code and tests ---

func (e *Executor) traverseFlow(
	currentID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	splitMergeMap map[model.ID]model.ID,
	visited map[model.ID]bool,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	lines *[]string,
	indent int,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
	annotationsByTarget *annotationEmitter,
) {
	// Legacy wrapper — preserved for tests and unmigrated callers that don't
	// supply flowsByDest. Passing nil suppresses @anchor emission, matching
	// the pre-refactor behaviour. Nil labels likewise: a caller that does not
	// compute them describes exactly as before.
	traverseFlow(e.newExecContext(context.Background()), currentID, activityMap, flowsByOrigin, nil, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget, mergeLabels{})
}

func (e *Executor) collectErrorHandlerStatements(
	startID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	annotationsByTarget *annotationEmitter,
) []string {
	return collectErrorHandlerStatements(e.newExecContext(context.Background()), startID, activityMap, flowsByOrigin, entityNames, microflowNames, annotationsByTarget, mergeLabels{})
}

// joinCrossedMerge emits `join <label>` when id is a crossed merge and reports
// whether it did. Every place a traversal would otherwise walk THROUGH a merge
// asks this first: a crossed merge is reached by several paths, so arriving at
// one is never a place to continue from (see emitCrossedMergeSections).
func joinCrossedMerge(id model.ID, labels mergeLabels, lines *[]string, indent int) bool {
	if !labels.isCrossed(id) {
		return false
	}
	label, ok := labels.of(id)
	if !ok {
		return false
	}
	*lines = append(*lines, strings.Repeat("  ", indent)+"join "+label+";")
	return true
}

// isLabelledMerge reports whether a merge carries a describe label, and so must
// be handed to the traversal (which declares or joins it) rather than skipped.
func isLabelledMerge(id model.ID, labels mergeLabels) bool {
	_, ok := labels.of(id)
	return ok
}
