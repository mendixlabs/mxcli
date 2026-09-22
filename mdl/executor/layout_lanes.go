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
package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

const (
	// guardBranchInset is where a guard's branch starts, measured the way the
	// measurer measures: from the split's LEFT EDGE to the left edge of the branch's
	// first activity (thenStartX - ActivityWidth/2).
	guardBranchInset = SplitWidth/2 + SplitWidth + HorizontalSpacing/2 - ActivityWidth/2

	// guardMainWidth is what a guard takes of the main line, in the measurer's terms:
	// gapBetween adds HorizontalSpacing/2 after an IF, and the builder puts the next
	// element at thenStartX = split x + SplitWidth + HorizontalSpacing/2.
	guardMainWidth = SplitWidth

	// laneGap is the clear space kept after a branch before the lane is used again:
	// the same edge-to-edge gap two activities have on the main line.
	laneGap = HorizontalSpacing - ActivityWidth
)

// isGuard reports whether an IF is drawn as a guard: no ELSE, and a THEN that ends
// the flow, so there is no merge and the branch never rejoins the main line.
func isGuard(s *ast.IfStmt) bool {
	return len(s.ElseBody) == 0 && !s.HasElse && lastStmtIsReturn(s.ThenBody)
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

// leftHalfOf is the distance from a statement's x to its left edge, as gapBetween
// counts it.
func leftHalfOf(stmt ast.MicroflowStatement) int {
	switch stmt.(type) {
	case *ast.IfStmt, *ast.EnumSplitStmt:
		return SplitWidth / 2
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

// lowestBetween is the bottom edge of the objects in fb.objects[start:end], or the
// fallback when that range holds none. end may be -1 for "everything since start".
//
// A branch's measured height says how TALL it is, and the builder places the next
// lane half that height below the line — which is only right when the content is
// centred on its line. A nested IF is not: its own else lane hangs entirely below,
// so a branch measured 160 tall occupies 30 above its line and 130 below, and the
// lane placed on the measurement landed 50px into it. The branch is already built by
// the time the next lane is placed, so this measures it instead of modelling it.
func (fb *flowBuilder) lowestBetween(start, end, fallback int) int {
	if end < 0 || end > len(fb.objects) {
		end = len(fb.objects)
	}
	bottom := fallback
	for i := start; i < end; i++ {
		o := fb.objects[i]
		if o == nil {
			continue
		}
		h := ActivityHeight
		if ws, ok := o.(interface{ GetSize() model.Size }); ok {
			if hh := ws.GetSize().Height; hh > 0 {
				h = hh
			}
		}
		if b := o.GetPosition().Y + h/2; b > bottom {
			bottom = b
		}
	}
	return bottom
}
