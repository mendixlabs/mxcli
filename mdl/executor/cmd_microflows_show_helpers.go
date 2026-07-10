// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow traversal and helper functions.
package executor

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// buildAnnotationsByTarget builds a map from activity ID to annotation captions.
// It joins AnnotationFlows (destination → activity) with Annotation objects (caption).
func buildAnnotationsByTarget(oc *microflows.MicroflowObjectCollection) map[model.ID][]string {
	result := make(map[model.ID][]string)
	if oc == nil {
		return result
	}

	annotCaptions := make(map[model.ID]string)
	collectAnnotationCaptions(oc, annotCaptions)

	// Map each annotation flow's destination (the activity) to the annotation's caption
	for _, af := range oc.AnnotationFlows {
		if caption, ok := annotCaptions[af.OriginID]; ok && caption != "" {
			result[af.DestinationID] = append(result[af.DestinationID], caption)
		}
	}

	return result
}

func collectAnnotationCaptions(oc *microflows.MicroflowObjectCollection, captions map[model.ID]string) {
	if oc == nil {
		return
	}
	for _, obj := range oc.Objects {
		if annot, ok := obj.(*microflows.Annotation); ok {
			captions[annot.ID] = annot.Caption
			continue
		}
		if loop, ok := obj.(*microflows.LoopedActivity); ok {
			collectAnnotationCaptions(loop.ObjectCollection, captions)
		}
	}
}

// mergeAnnotationsByTarget combines parent-level annotations with the
// loop-local overlay so each activity gets every caption that points at it,
// regardless of which collection the annotation flow lives in.
//
// When one side is empty the function returns the other map by reference (no
// copy). The current callers — emitLoopBody passing a freshly built overlay,
// or a freshly inherited parent map — never mutate the result, so aliasing is
// safe. New callers that intend to mutate the result must copy first.
func mergeAnnotationsByTarget(base, overlay map[model.ID][]string) map[model.ID][]string {
	if len(base) == 0 {
		return overlay
	}
	if len(overlay) == 0 {
		return base
	}
	merged := make(map[model.ID][]string, len(base)+len(overlay))
	for id, captions := range base {
		merged[id] = captions
	}
	for id, captions := range overlay {
		merged[id] = append(merged[id], captions...)
	}
	return merged
}

// collectFreeAnnotations returns captions for annotations not referenced by any AnnotationFlow.
func collectFreeAnnotations(oc *microflows.MicroflowObjectCollection) []string {
	if oc == nil {
		return nil
	}

	// Collect annotation IDs that are referenced by flows
	referencedAnnotations := make(map[model.ID]bool)
	for _, af := range oc.AnnotationFlows {
		referencedAnnotations[af.OriginID] = true
	}

	var result []string
	for _, obj := range oc.Objects {
		if annot, ok := obj.(*microflows.Annotation); ok {
			if !referencedAnnotations[annot.ID] && annot.Caption != "" {
				result = append(result, annot.Caption)
			}
		}
	}
	return result
}

func prependFreeAnnotationLines(oc *microflows.MicroflowObjectCollection, activityLines []string) []string {
	freeAnnots := collectFreeAnnotations(oc)
	if len(freeAnnots) == 0 || len(activityLines) == 0 {
		return activityLines
	}

	prefix := make([]string, 0, len(freeAnnots))
	for _, text := range freeAnnots {
		prefix = append(prefix, fmt.Sprintf("@annotation %s", mdlQuote(text)))
	}
	return append(prefix, activityLines...)
}

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
		emitSplitAnchorAnnotation(id, flowsByOrigin, flowsByDest, lines, indentStr, false)
		return
	}
	if _, isSplit := obj.(*microflows.InheritanceSplit); isSplit {
		emitSplitAnchorAnnotation(id, flowsByOrigin, flowsByDest, lines, indentStr, true)
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
	if incoming := flowsByDest[id]; len(incoming) > 0 {
		to = anchorSideKeyword(incoming[0].DestinationConnectionIndex)
	}

	if from == "" && to == "" {
		return
	}
	defaultFrom := anchorSideKeyword(AnchorRight)
	defaultTo := anchorSideKeyword(AnchorLeft)
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

	if inTo == "" && trueFrom == "" && trueTo == "" && falseFrom == "" && falseTo == "" {
		return
	}

	var parts []string
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
	if len(parts) == 0 {
		return
	}
	*lines = append(*lines, indentStr+fmt.Sprintf("@anchor(%s)", strings.Join(parts, ", ")))
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
	annotationsByTarget map[model.ID][]string,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	activityMap map[model.ID]microflows.MicroflowObject,
) {
	currentID := obj.GetID()

	pos := obj.GetPosition()
	*lines = append(*lines, indentStr+fmt.Sprintf("@position(%d, %d)", pos.X, pos.Y))

	if flowsByOrigin != nil && flowsByDest != nil {
		// @anchor — emit whenever attached flows exist, for roundtrip fidelity.
		// The emitter sorts out the right form (simple / split / loop) based on
		// the object type.
		emitAnchorAnnotationWithActivityMap(obj, flowsByOrigin, flowsByDest, activityMap, lines, indentStr)
	}

	if activity, ok := obj.(*microflows.ActionActivity); ok {
		if activity.Disabled {
			*lines = append(*lines, indentStr+"@excluded")
		}
		if !activity.AutoGenerateCaption && activity.Caption != "" {
			*lines = append(*lines, indentStr+fmt.Sprintf("@caption %s", mdlQuote(activity.Caption)))
		}
		if activity.BackgroundColor != "" && activity.BackgroundColor != "Default" {
			*lines = append(*lines, indentStr+fmt.Sprintf("@color %s", activity.BackgroundColor))
		}
	}

	if split, ok := obj.(*microflows.ExclusiveSplit); ok && split.Caption != "" {
		*lines = append(*lines, indentStr+fmt.Sprintf("@caption %s", mdlQuote(split.Caption)))
	}
	if split, ok := obj.(*microflows.InheritanceSplit); ok && split.Caption != "" {
		*lines = append(*lines, indentStr+fmt.Sprintf("@caption %s", mdlQuote(split.Caption)))
	}
	if loop, ok := obj.(*microflows.LoopedActivity); ok && loop.Caption != "" {
		*lines = append(*lines, indentStr+fmt.Sprintf("@caption %s", mdlQuote(loop.Caption)))
	}

	// @annotation (attached Annotation objects)
	if annotationsByTarget != nil {
		for _, caption := range annotationsByTarget[currentID] {
			*lines = append(*lines, indentStr+fmt.Sprintf("@annotation %s", mdlQuote(caption)))
		}
	}
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
	annotationsByTarget map[model.ID][]string,
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
		errStmts := collectErrorHandlerStatements(
			ctx,
			errorHandlerFlow.DestinationID,
			activityMap, flowsByOrigin, entityNames, microflowNames,
		)

		stmtWithoutSemi := strings.TrimSuffix(strings.TrimSpace(stmt), ";")

		errorSuffix := suffix
		if errorSuffix == "" {
			errorSuffix = " on error without rollback"
		}

		if len(errStmts) == 0 {
			*lines = append(*lines, indentStr+stmtWithoutSemi+errorSuffix+" { };")
		} else {
			*lines = append(*lines, indentStr+stmtWithoutSemi+errorSuffix+" {")
			for _, errStmt := range errStmts {
				*lines = append(*lines, indentStr+"  "+errStmt)
			}
			*lines = append(*lines, indentStr+"};")
		}
	} else if suffix != "" {
		stmtWithoutSemi := strings.TrimSuffix(strings.TrimSpace(stmt), ";")
		*lines = append(*lines, indentStr+stmtWithoutSemi+suffix+";")
	} else {
		*lines = append(*lines, indentStr+stmt)
	}
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
	annotationsByTarget map[model.ID][]string,
) {
	if currentID == "" || visited[currentID] {
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
		if isMergePairedWithSplit(currentID, splitMergeMap) {
			return
		}
		if mergeHasLoopBackEdge(currentID, flowsByOrigin) {
			visited[currentID] = true
			*lines = append(*lines, strings.Repeat("  ", indent)+"while true")
			*lines = append(*lines, strings.Repeat("  ", indent)+"begin")
			for _, flow := range findNormalFlows(flowsByOrigin[currentID]) {
				traverseFlowUntilMerge(ctx, flow.DestinationID, currentID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget)
			}
			*lines = append(*lines, strings.Repeat("  ", indent)+"end while;")
			return
		}
		visited[currentID] = true
		for _, flow := range flowsByOrigin[currentID] {
			traverseFlow(ctx, flow.DestinationID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
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
		emitInheritanceSplitStatement(ctx, currentID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
		recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)
		if mergeID != "" {
			visited[mergeID] = true
			for _, flow := range flowsByOrigin[mergeID] {
				traverseFlow(ctx, flow.DestinationID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
			}
		}
		return
	}

	// Handle ExclusiveSplit specially - need to process both branches
	if split, isSplit := obj.(*microflows.ExclusiveSplit); isSplit {
		startLine := len(*lines) + headerLineCount
		flows := flowsByOrigin[currentID]
		mergeID := splitMergeMap[currentID]
		if variable, ok := enumSplitVariable(split); ok && hasEnumCaseFlows(flows) {
			emitObjectAnnotations(obj, lines, indentStr, annotationsByTarget, flowsByOrigin, flowsByDest, activityMap)
			emitEnumSplitStatement(ctx, currentID, mergeID, variable, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
			recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)
			continueAfterSplitJoin(ctx, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
			return
		}
		trueFlow, falseFlow := findBranchFlows(flows)

		// Empty-then swap: when the true branch goes directly to the merge
		// (empty then body) and the false branch has real content, negate
		// the condition and swap branches for more readable output.
		// "if cond then else <body> end if;" → "if not(cond) then <body> end if;"
		if trueFlow != nil && falseFlow != nil && mergeID != "" {
			if trueFlow.DestinationID == mergeID && falseFlow.DestinationID != mergeID {
				stmt = negateIfCondition(stmt)
				trueFlow, falseFlow = falseFlow, trueFlow
			}
		}

		if stmt != "" {
			emitObjectAnnotations(obj, lines, indentStr, annotationsByTarget, flowsByOrigin, flowsByDest, activityMap)
			*lines = append(*lines, indentStr+stmt)
		}

		trueTerminates := branchFlowTerminatesBeforeMerge(trueFlow, mergeID, activityMap, flowsByOrigin, splitMergeMap)
		isGuard := trueTerminates && flowLooksLikeGuardContinuation(falseFlow, obj, activityMap) && !hasExplicitFalseBranchAnchor(falseFlow)

		if isGuard {
			traverseFlowUntilMerge(ctx, trueFlow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget)
			*lines = append(*lines, indentStr+"end if;")
			recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)

			// Continue from the false branch (skip through merge if present)
			if falseFlow != nil {
				contID := falseFlow.DestinationID
				if _, isMerge := activityMap[contID].(*microflows.ExclusiveMerge); isMerge {
					visited[contID] = true
					for _, flow := range flowsByOrigin[contID] {
						contID = flow.DestinationID
						break
					}
				}
				traverseFlow(ctx, contID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
			}
		} else {
			if trueFlow != nil {
				traverseFlowUntilMerge(ctx, trueFlow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget)
			}

			if falseFlow != nil {
				elseLineIdx := len(*lines)
				*lines = append(*lines, indentStr+"else")
				visitedFalseBranch := make(map[model.ID]bool)
				for id := range visited {
					visitedFalseBranch[id] = true
				}
				traverseFlowUntilMerge(ctx, falseFlow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visitedFalseBranch, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget)
				// Remove empty else block. A false branch can point at a
				// continuation already emitted through the true branch, so checking
				// only falseFlow.DestinationID != mergeID is not enough.
				if len(*lines) == elseLineIdx+1 {
					*lines = (*lines)[:elseLineIdx]
				}
			}

			*lines = append(*lines, indentStr+"end if;")
			recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)

			continueAfterSplitJoin(ctx, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
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
			traverseFlow(ctx, flow.DestinationID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
		}
		return
	}

	// Regular activity
	startLine := len(*lines) + headerLineCount
	normalFlows := findNormalFlows(flowsByOrigin[currentID])
	emitActivityStatement(ctx, obj, stmt, flowsByOrigin, flowsByDest, activityMap, entityNames, microflowNames, lines, indentStr, annotationsByTarget)
	recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)

	// Follow normal (non-error-handler) outgoing flows
	for _, flow := range normalFlows {
		traverseFlow(ctx, flow.DestinationID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
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
	annotationsByTarget map[model.ID][]string,
) {
	if currentID == "" || currentID == mergeID || visited[currentID] {
		return
	}

	obj := activityMap[currentID]
	if obj == nil {
		return
	}

	// Handle intermediate merge points - traverse through them without outputting anything
	if _, isMerge := obj.(*microflows.ExclusiveMerge); isMerge {
		flows := flowsByOrigin[currentID]
		for _, flow := range flows {
			traverseFlowUntilMerge(ctx, flow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
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
		emitInheritanceSplitStatement(ctx, currentID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
		recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)
		if nestedMergeID != "" && nestedMergeID != mergeID {
			visited[nestedMergeID] = true
			for _, flow := range flowsByOrigin[nestedMergeID] {
				traverseFlowUntilMerge(ctx, flow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
			}
		}
		return
	}

	// Handle nested ExclusiveSplit
	if split, isSplit := obj.(*microflows.ExclusiveSplit); isSplit {
		startLine := len(*lines) + headerLineCount
		flows := flowsByOrigin[currentID]
		nestedMergeID := splitMergeMap[currentID]
		if variable, ok := enumSplitVariable(split); ok && hasEnumCaseFlows(flows) {
			emitObjectAnnotations(obj, lines, indentStr, annotationsByTarget, flowsByOrigin, flowsByDest, activityMap)
			emitEnumSplitStatement(ctx, currentID, nestedMergeID, variable, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
			recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)
			continueAfterNestedSplitJoin(ctx, nestedMergeID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
			return
		}
		trueFlow, falseFlow := findBranchFlows(flows)
		nestedMergeID = resolveNestedMergeID(nestedMergeID, mergeID, trueFlow, falseFlow, flowsByOrigin)

		// Empty-then swap: negate when true branch is empty but false branch has content.
		// Skip when both branches go directly to merge (both empty).
		if trueFlow != nil && falseFlow != nil && nestedMergeID != "" && nestedMergeID == mergeID {
			if trueFlow.DestinationID == nestedMergeID && falseFlow.DestinationID != nestedMergeID {
				stmt = negateIfCondition(stmt)
				trueFlow, falseFlow = falseFlow, trueFlow
			}
		}

		if stmt != "" {
			emitObjectAnnotations(obj, lines, indentStr, annotationsByTarget, flowsByOrigin, flowsByDest, activityMap)
			*lines = append(*lines, indentStr+stmt)
		}

		trueTerminates := branchFlowTerminatesBeforeMerge(trueFlow, nestedMergeID, activityMap, flowsByOrigin, splitMergeMap)
		isGuard := trueTerminates && flowLooksLikeGuardContinuation(falseFlow, obj, activityMap) && !hasExplicitFalseBranchAnchor(falseFlow)

		if isGuard {
			traverseFlowUntilMerge(ctx, trueFlow.DestinationID, nestedMergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget)
			*lines = append(*lines, indentStr+"end if;")
			recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)

			// Continue from the false branch (skip through merge if present).
			// Guard: do not cross the outer merge boundary — if the false path
			// leads directly to mergeID, stop here so the activities after the
			// merge are emitted by continueAfterSplitJoin, not inside this branch.
			if falseFlow != nil {
				contID := falseFlow.DestinationID
				if contID != mergeID {
					if _, isMerge := activityMap[contID].(*microflows.ExclusiveMerge); isMerge {
						visited[contID] = true
						for _, flow := range flowsByOrigin[contID] {
							contID = flow.DestinationID
							break
						}
					}
					traverseFlowUntilMerge(ctx, contID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
				}
			}
		} else {
			if trueFlow != nil {
				traverseFlowUntilMerge(ctx, trueFlow.DestinationID, nestedMergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget)
			}

			if falseFlow != nil {
				elseLineIdx := len(*lines)
				*lines = append(*lines, indentStr+"else")
				visitedFalseBranch := make(map[model.ID]bool)
				for id := range visited {
					visitedFalseBranch[id] = true
				}
				traverseFlowUntilMerge(ctx, falseFlow.DestinationID, nestedMergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visitedFalseBranch, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget)
				// Remove empty else block
				if len(*lines) == elseLineIdx+1 {
					*lines = (*lines)[:elseLineIdx]
				}
			}

			*lines = append(*lines, indentStr+"end if;")
			recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)

			continueAfterNestedSplitJoin(ctx, nestedMergeID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
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
			traverseFlowUntilMerge(ctx, flow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
		}
		return
	}

	// Regular activity
	startLine := len(*lines) + headerLineCount
	normalFlows := findNormalFlows(flowsByOrigin[currentID])
	emitActivityStatement(ctx, obj, stmt, flowsByOrigin, flowsByDest, activityMap, entityNames, microflowNames, lines, indentStr, annotationsByTarget)
	recordSourceMap(sourceMap, currentID, startLine, len(*lines)+headerLineCount-1)

	// Follow normal (non-error-handler) outgoing flows until merge
	for _, flow := range normalFlows {
		traverseFlowUntilMerge(ctx, flow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
	}
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
	annotationsByTarget map[model.ID][]string,
) {
	if joinID == "" {
		return
	}
	if _, isMerge := activityMap[joinID].(*microflows.ExclusiveMerge); isMerge {
		visited[joinID] = true
		for _, flow := range flowsByOrigin[joinID] {
			traverseFlow(ctx, flow.DestinationID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
		}
		return
	}
	traverseFlow(ctx, joinID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
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
	annotationsByTarget map[model.ID][]string,
) {
	if joinID == "" || joinID == parentMergeID {
		return
	}
	if _, isMerge := activityMap[joinID].(*microflows.ExclusiveMerge); isMerge {
		visited[joinID] = true
		for _, flow := range flowsByOrigin[joinID] {
			traverseFlowUntilMerge(ctx, flow.DestinationID, parentMergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
		}
		return
	}
	traverseFlowUntilMerge(ctx, joinID, parentMergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
}

func resolveNestedMergeID(
	nestedMergeID model.ID,
	parentMergeID model.ID,
	trueFlow *microflows.SequenceFlow,
	falseFlow *microflows.SequenceFlow,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
) model.ID {
	if nestedMergeID != "" && parentMergeID != "" && nestedMergeID != parentMergeID &&
		canReachNode(parentMergeID, nestedMergeID, flowsByOrigin, make(map[model.ID]bool)) {
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
	annotationsByTarget map[model.ID][]string,
) {
	// Loop bodies can contain the same structured control flow as top-level
	// microflows. Reuse the main traversal with a loop-local split/merge map so
	// nested IF/ELSE blocks emit `else` / `end if;` correctly.
	loopSplitMergeMap := findSplitMergePointsForGraph(ctx, activityMap, flowsByOrigin)
	traverseFlow(ctx, currentID, activityMap, flowsByOrigin, flowsByDest, loopSplitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
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
	annotationsByTarget map[model.ID][]string,
) {
	if loop.ObjectCollection == nil || len(loop.ObjectCollection.Objects) == 0 {
		return
	}

	loopAnnotationsByTarget := mergeAnnotationsByTarget(annotationsByTarget, buildAnnotationsByTarget(loop.ObjectCollection))

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
		incomingCount[loopObj.GetID()] = 0
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
		traverseLoopBody(ctx, firstID, loopActivityMap, loopFlowsByOrigin, loopFlowsByDest, loopSplitMergeMap, loopVisited, entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, loopAnnotationsByTarget)
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
	variable string,
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
	annotationsByTarget map[model.ID][]string,
) {
	indentStr := strings.Repeat("  ", indent)
	*lines = append(*lines, indentStr+"case $"+variable)

	type enumBranch struct {
		values []string
		flow   *microflows.SequenceFlow
	}
	branches := []enumBranch{}
	branchByDestination := map[model.ID]int{}
	var elseFlow *microflows.SequenceFlow
	for _, flow := range orderedEnumSplitFlows(findNormalFlows(flowsByOrigin[currentID])) {
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

	for _, branch := range branches {
		*lines = append(*lines, indentStr+"  when "+formatEnumSplitCaseValues(branch.values)+" then")
		traverseFlowUntilMerge(ctx, branch.flow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, cloneVisited(visited), entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget)
	}
	if elseFlow != nil {
		*lines = append(*lines, indentStr+"  else")
		traverseFlowUntilMerge(ctx, elseFlow.DestinationID, mergeID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, cloneVisited(visited), entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget)
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
	annotationsByTarget map[model.ID][]string,
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
	for _, flow := range orderedInheritanceSplitFlows(findNormalFlows(flowsByOrigin[currentID])) {
		caseName, ok := inheritanceCaseName(flow, entityNames)
		if !ok {
			elseFlow = flow
			continue
		}
		*lines = append(*lines, indentStr+"case "+caseName)
		traverseFlowUntilMerge(ctx, flow.DestinationID, branchStopID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, cloneVisited(visited), entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget)
	}
	if elseFlow != nil {
		*lines = append(*lines, indentStr+"else")
		traverseFlowUntilMerge(ctx, elseFlow.DestinationID, branchStopID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, cloneVisited(visited), entityNames, microflowNames, lines, indent+1, sourceMap, headerLineCount, annotationsByTarget)
	}
	*lines = append(*lines, indentStr+"end split;")
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

func orderedEnumSplitFlows(flows []*microflows.SequenceFlow) []*microflows.SequenceFlow {
	ordered := append([]*microflows.SequenceFlow(nil), flows...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return splitCaseOrder(ordered[i]) < splitCaseOrder(ordered[j])
	})
	return ordered
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
	return dest.GetPosition().Y == split.GetPosition().Y &&
		flow.OriginConnectionIndex == AnchorRight &&
		flow.DestinationConnectionIndex == AnchorLeft
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
func formatErrorHandlingSuffix(errType microflows.ErrorHandlingType) string {
	switch errType {
	case microflows.ErrorHandlingTypeContinue:
		return " on error continue"
	case microflows.ErrorHandlingTypeRollback:
		return " on error rollback"
	case microflows.ErrorHandlingTypeCustom:
		return " on error" // Will be followed by block
	case microflows.ErrorHandlingTypeCustomWithoutRollback:
		return " on error without rollback" // Will be followed by block
	default:
		return "" // Abort is the default, no suffix needed
	}
}

// hasCustomErrorHandler returns true if the error handling type requires a custom handler block.
func hasCustomErrorHandler(errType microflows.ErrorHandlingType) bool {
	return errType == microflows.ErrorHandlingTypeCustom || errType == microflows.ErrorHandlingTypeCustomWithoutRollback
}

// getActionErrorHandlingType extracts the ErrorHandlingType from the action inside an ActionActivity.
// Most action types store ErrorHandlingType at the action level, not the activity level.
func getActionErrorHandlingType(activity *microflows.ActionActivity) microflows.ErrorHandlingType {
	if activity == nil || activity.Action == nil {
		return ""
	}

	switch action := activity.Action.(type) {
	case *microflows.MicroflowCallAction:
		return action.ErrorHandlingType
	case *microflows.NanoflowCallAction:
		return action.ErrorHandlingType
	case *microflows.JavaActionCallAction:
		return action.ErrorHandlingType
	case *microflows.JavaScriptActionCallAction:
		return action.ErrorHandlingType
	case *microflows.CallExternalAction:
		return action.ErrorHandlingType
	case *microflows.RestCallAction:
		return action.ErrorHandlingType
	case *microflows.WebServiceCallAction:
		return action.ErrorHandlingType
	case *microflows.RestOperationCallAction:
		return "" // RestOperationCallAction does not support custom error handling (CE6035)
	case *microflows.ExecuteDatabaseQueryAction:
		return action.ErrorHandlingType
	case *microflows.ImportXmlAction:
		return action.ErrorHandlingType
	case *microflows.ExportXmlAction:
		return action.ErrorHandlingType
	case *microflows.CommitObjectsAction:
		return action.ErrorHandlingType
	case *microflows.DownloadFileAction:
		return action.ErrorHandlingType
	default:
		// Fall back to activity level for action types without ErrorHandlingType field
		return activity.ErrorHandlingType
	}
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
) []string {
	var statements []string
	visited := make(map[model.ID]bool)
	stopID := firstReachableErrorHandlerMerge(startID, activityMap, flowsByOrigin)
	splitMergeMap := findErrorHandlerSplitMergePoints(ctx, activityMap, flowsByOrigin)

	var traverse func(id model.ID, boundary model.ID, indent int)
	traverse = func(id model.ID, boundary model.ID, indent int) {
		if id == "" || id == boundary || visited[id] {
			return
		}
		obj := activityMap[id]
		if obj == nil {
			return
		}
		if _, isMerge := obj.(*microflows.ExclusiveMerge); isMerge {
			return
		}
		visited[id] = true

		indentStr := strings.Repeat("  ", indent)
		if _, isSplit := obj.(*microflows.ExclusiveSplit); isSplit {
			stmt := formatActivity(ctx, obj, entityNames, microflowNames)
			if stmt != "" {
				statements = append(statements, indentStr+stmt)
			}
			nestedMergeID := splitMergeMap[id]
			trueFlow, falseFlow := findBranchFlows(flowsByOrigin[id])
			if trueFlow != nil {
				traverse(trueFlow.DestinationID, nestedMergeID, indent+1)
			}
			if falseFlow != nil {
				statements = append(statements, indentStr+"else")
				if falseFlow.DestinationID != nestedMergeID {
					traverse(falseFlow.DestinationID, nestedMergeID, indent+1)
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

		if stmt := formatActivity(ctx, obj, entityNames, microflowNames); stmt != "" {
			statements = append(statements, indentStr+stmt)
		}
		for _, flow := range findNormalFlows(flowsByOrigin[id]) {
			traverse(flow.DestinationID, boundary, indent)
		}
	}

	traverse(startID, stopID, 0)
	return statements
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

func firstReachableErrorHandlerMerge(
	startID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
) model.ID {
	visited := make(map[model.ID]bool)
	queue := []model.ID{startID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if id == "" || visited[id] {
			continue
		}
		visited[id] = true
		if _, isMerge := activityMap[id].(*microflows.ExclusiveMerge); isMerge {
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
	annotationsByTarget map[model.ID][]string,
) {
	// Legacy wrapper — preserved for tests and unmigrated callers that don't
	// supply flowsByDest. Passing nil suppresses @anchor emission, matching
	// the pre-refactor behaviour.
	traverseFlow(e.newExecContext(context.Background()), currentID, activityMap, flowsByOrigin, nil, splitMergeMap, visited, entityNames, microflowNames, lines, indent, sourceMap, headerLineCount, annotationsByTarget)
}

// negateIfCondition transforms "if <cond> then" into "if not(<cond>) then".
// Used by the empty-then swap to produce readable output when Studio Pro stores
// the flow with an inverted condition (true branch empty, false branch has body).
func negateIfCondition(stmt string) string {
	// stmt is always "if <condition> then" from formatActivity.
	const prefix = "if "
	const suffix = " then"
	if strings.HasPrefix(stmt, prefix) && strings.HasSuffix(stmt, suffix) {
		cond := stmt[len(prefix) : len(stmt)-len(suffix)]
		// Avoid double-negation: not(not(x)) → x
		// Only unwrap if the outer parens are balanced (depth returns to 0 at the final char)
		if strings.HasPrefix(cond, "not(") && strings.HasSuffix(cond, ")") {
			inner := cond[4 : len(cond)-1]
			depth := 0
			balanced := true
			for _, ch := range inner {
				if ch == '(' {
					depth++
				} else if ch == ')' {
					depth--
					if depth < 0 {
						balanced = false
						break
					}
				}
			}
			if balanced && depth == 0 {
				return prefix + inner + suffix
			}
		}
		return prefix + "not(" + cond + ")" + suffix
	}
	return stmt
}

func (e *Executor) collectErrorHandlerStatements(
	startID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
) []string {
	return collectErrorHandlerStatements(e.newExecContext(context.Background()), startID, activityMap, flowsByOrigin, entityNames, microflowNames)
}
