// SPDX-License-Identifier: Apache-2.0

// Package executor - the lane under the main line.
//
// A guard — `if X then ...; return; end if`, no ELSE — draws its branch in the lane
// below the main line and ends it there, in an end event. Nothing comes back up. The
// main line nevertheless used to wait for it: the next element was placed past the
// branch's far end, so every guard left a stretch of bare line above its own branch.
// Measured on a flow of three guards, each split stood 370px from the activity after
// it, with a 40px gap everywhere else.
//
// The main line now resumes one pitch after the split, over the branch, and the lane
// remembers how far it is taken. Only an element that reaches down into that lane —
// another decision, a loop box, an activity with an error handler under it — has to
// wait for it to clear; a plain activity does not.
//
// An error handler's row is the lane's other occupant: it hangs ErrorHandlerPitch
// under its activity and the main line carries on over it the same way.
package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

const (
	// guardBranchInset is where a guard's branch starts, measured the way the
	// measurer measures: from the split's x to the left edge of the branch's first
	// activity (thenStartX - ActivityWidth/2).
	guardBranchInset = SplitWidth + HorizontalSpacing/2 - ActivityWidth/2

	// laneGap is the clear space kept after a branch before the lane is used again:
	// the same edge-to-edge gap two activities have on the main line.
	laneGap = HorizontalSpacing - ActivityWidth
)

// isGuard reports whether an IF is drawn as a guard: no ELSE, and a THEN that ends
// the flow, so there is no merge and the branch never rejoins the main line.
func isGuard(s *ast.IfStmt) bool {
	return len(s.ElseBody) == 0 && !s.HasElse && lastStmtIsReturn(s.ThenBody)
}

// ifJoinsNothingAtTail reports whether addIfStatement, building s as the last
// statement of a loop body, leaves out its merge: an IF/ELSE whose two branches
// both carry on there ends the iteration either way (mendixlabs/mxcli#884).
func ifJoinsNothingAtTail(s *ast.IfStmt) bool {
	hasElse := s.HasElse || len(s.ElseBody) > 0
	return hasElse && len(s.ThenBody) > 0 && len(s.ElseBody) > 0 &&
		!lastStmtIsReturn(s.ThenBody) && !lastStmtIsReturn(s.ElseBody) &&
		!bodyHasContinuingCustomErrorHandler(s.ThenBody) && !bodyHasContinuingCustomErrorHandler(s.ElseBody) &&
		(s.Annotations == nil || s.Annotations.Merge == nil)
}

// ifHasMerge reports whether addIfStatement draws a merge for s, as it decides
// before building the branches. atTail is the builder's atBodyTail.
func ifHasMerge(s *ast.IfStmt, atTail bool) bool {
	thenReturns := lastStmtIsReturn(s.ThenBody)
	if !(s.HasElse || len(s.ElseBody) > 0) {
		return !thenReturns
	}
	if atTail && ifJoinsNothingAtTail(s) {
		return false
	}
	return !thenReturns && !lastStmtIsReturn(s.ElseBody)
}

// ifMainWidth is how much of the main line an IF without a merge takes, in the
// measurer's terms: from the split's left edge to a point gapBetween's default
// (HorizontalSpacing - ActivityWidth/2) short of where addIfStatement's no-merge
// exit centres the next element. A guard resumes at thenStartX, in the column its
// branch starts in; an IF/ELSE one of whose branches ends the flow resumes past
// the THEN branch, or a pitch after the split.
func (m *layoutMeasurer) ifMainWidth(s *ast.IfStmt) int {
	thenStart := SplitWidth + HorizontalSpacing/2 // from the split's centre
	next := thenStart
	if s.HasElse || len(s.ElseBody) > 0 {
		next = max(HorizontalSpacing, thenStart+m.measureBranch(s.ThenBody).Width+HorizontalSpacing/2)
	}
	return SplitWidth/2 + next - (HorizontalSpacing - ActivityWidth/2)
}

// rejoinsPastRow reports whether stmt's error handler falls through to the main
// line from a row of width row too long to rejoin between the activity and the
// next one: two activities or more. Such a row rejoins at a merge one gap past its
// end, and the main line waits for that merge (rejoinAfterRow).
func rejoinsPastRow(eh *ast.ErrorHandlingClause, row int) bool {
	if !isCustomErrorHandling(eh) || len(eh.Body) == 0 || row <= ActivityWidth {
		return false
	}
	return !lastStmtIsReturn(eh.Body) && !isRetryLoopErrorHandler(eh.Body)
}

// rejoinAfterRow makes the main line wait for the merge a falling-through handler
// row rejoins at, and records where that merge goes (ako/mxcli#1773).
//
// The merge used to stand midway between the guarded activity and the next one.
// The row starts under the activity and runs right, so from two activities on its
// last one was right of that point and the edge up into the merge ran backwards.
// The merge goes one gap past the row instead, as an IF's merge goes one gap past
// its branch, and the next statement one gap past the merge.
func (fb *flowBuilder) rejoinAfterRow(eh *ast.ErrorHandlingClause, activityID model.ID, activityX int) {
	if fb.measurer == nil || eh == nil {
		return
	}
	row := fb.measurer.measureBranch(eh.Body).Width
	if !rejoinsPastRow(eh, row) {
		return
	}
	mergeX := activityX - ActivityWidth/2 + row + laneGap + MergeSize/2
	if fb.rejoinPastRowX == nil {
		fb.rejoinPastRowX = map[model.ID]int{}
	}
	fb.rejoinPastRowX[activityID] = mergeX
	fb.posX = max(fb.posX, mergeX+MergeSize+HorizontalSpacing/2)
}

// reachesBelowMainLine reports whether a statement draws anything under the main
// line's own row of activities. bounds is its measured size.
func reachesBelowMainLine(stmt ast.MicroflowStatement, bounds Bounds) bool {
	if bounds.Height > ActivityHeight {
		return true
	}
	// A custom error handler's body is laid out under its activity and is not part
	// of the activity's measured size.
	return len(getErrorHandlerBody(stmt)) > 0
}

// handlerRowWidth is how wide the row of a statement's custom error handler is,
// measured from the activity's left edge, or 0 when it has no handler body. The
// row's first element is centred under the activity (addErrorHandlerFlow), so
// the row starts where the activity does and runs right like a branch, end
// event included.
func (m *layoutMeasurer) handlerRowWidth(stmt ast.MicroflowStatement) int {
	body := getErrorHandlerBody(stmt)
	if len(body) == 0 {
		return 0
	}
	return m.measureBranch(body).Width
}

// reserveHandlerRow marks the lane under the activity at activityX taken by its
// error handler's row (ako/mxcli#1768, mendixlabs/mxcli#1672).
//
// The row is laid out ErrorHandlerPitch under its activity and runs right, and the
// main line goes on one pitch after the activity regardless — which is right for a
// plain activity, whose row is above the handler's. But the next activity with a
// handler of its own started its row at its own x, inside this one: two consecutive
// REST calls each with a three-element handler put the first handler's end event
// and the second handler's first activity on the same point. clearLowerLane already
// made such a statement wait for a reserved lane; the row just never reserved it.
func (fb *flowBuilder) reserveHandlerRow(activityX int, body []ast.MicroflowStatement) {
	if fb.measurer == nil || len(body) == 0 {
		return
	}
	rowRight := activityX - ActivityWidth/2 + fb.measurer.measureBranch(body).Width
	fb.reserveLowerLane(fb.posY, rowRight+laneGap)
}

// leftHalfOf is the distance from a statement's x to its left edge, as gapBetween
// counts it.
func leftHalfOf(stmt ast.MicroflowStatement) int {
	switch stmt.(type) {
	case *ast.IfStmt, *ast.EnumSplitStmt:
		return SplitWidth / 2
	case *ast.MergeStmt:
		return MergeSize / 2
	}
	return ActivityWidth / 2
}

// reserveLowerLane records that the lane under the main line at centerY is taken up
// to untilX (a left-edge limit: the next element reaching into the lane starts there
// or later).
func (fb *flowBuilder) reserveLowerLane(centerY, untilX int) {
	if fb.lowerLane == nil {
		fb.lowerLane = map[int]int{}
	}
	if untilX > fb.lowerLane[centerY] {
		fb.lowerLane[centerY] = untilX
	}
}

// clearLowerLane moves the cursor right until the statement about to be placed is
// clear of whatever already occupies the lane under this main line. A statement that
// stays on the main line is left where it is.
func (fb *flowBuilder) clearLowerLane(stmt ast.MicroflowStatement) {
	busy, ok := fb.lowerLane[fb.posY]
	if !ok || fb.measurer == nil {
		return
	}
	if !reachesBelowMainLine(stmt, fb.measurer.measureStatement(stmt)) {
		return
	}
	if half := leftHalfOf(stmt); fb.posX-half < busy {
		fb.posX = busy + half
	}
}
