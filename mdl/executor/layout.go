// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow layout algorithm
//
// Layout principles:
// 1. Left-to-right flow: Happy path goes straight horizontally
// 2. False/alternate paths below: ELSE branches go down, then merge back
// 3. Auto-sized containers: Loop boxes sized to fit content + padding
// 4. Horizontal connections: Connection lines are straight where possible
package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Layout constants
const (
	// Activity dimensions
	ActivityWidth  = 120
	ActivityHeight = 60

	// Split/merge dimensions
	SplitWidth  = 90
	SplitHeight = 60
	MergeSize   = 40

	// Start/End event dimensions
	EventSize = 20

	// Spacing
	HorizontalSpacing = 160 // Space between activities horizontally (edge-to-edge ~40px)
	VerticalSpacing   = 90  // Space between the branches of a type split, and RowGap
	// ErrorHandlerPitch is how far under its activity an error handler's row
	// is centred. Studio Pro prints an activity's output variable and type
	// under the box (about 35px, see RowGap), and at VerticalSpacing (90) the
	// handler's top edge sat 30px below the activity, inside that text
	// (mendixlabs/mxcli#1598, case 3). 130 is Studio Pro's own: the median of
	// 160 distinct handler rows straight under their activity in ako/TestApp,
	// PedApp and seven customer apps, quartiles 125..150.
	ErrorHandlerPitch = 130
	BranchGap         = 40 // Minimum edge-to-edge gap between parallel branches
	LoopPadding       = 50 // Padding inside loop boxes
	// LoopIteratorSpace is the column a foreach loop keeps on its left for the
	// iterator icon and the list variable's label. A WHILE has no iterator and no
	// column.
	LoopIteratorSpace = 100
	MinLoopWidth      = 200
	MinLoopHeight     = 100
)

// Bounds represents the bounding box of a layout element
type Bounds struct {
	Width  int
	Height int
}

// layoutMeasurer calculates dimensions of microflow statements
type layoutMeasurer struct {
	varTypes map[string]string
	// layout is the style the flow is measured for; nil is the default
	// (flowlayout.go). It must be the builder's, or the measured sizes are those
	// of a different drawing.
	layout *flowLayout
}

// measureStatements calculates the total bounds for a list of statements.
// Spacing is only added between pairs of non-zero-width elements so that
// zero-width statements (e.g. ReturnStmt, which produces no visual box)
// don't artificially inflate the measured width.
func (m *layoutMeasurer) measureStatements(stmts []ast.MicroflowStatement) Bounds {
	return m.measureRun(stmts, false)
}

// measureRun measures a statement run. atTail says the run is a loop body, whose
// last statement the builder builds with atBodyTail set: an IF/ELSE there joins
// nothing and draws no merge.
func (m *layoutMeasurer) measureRun(stmts []ast.MicroflowStatement, atTail bool) Bounds {
	if len(stmts) == 0 {
		return Bounds{Width: 0, Height: 0}
	}

	// cursor is the right edge of what sits on the main line; laneBusy is how far the
	// lane underneath it is taken by a guard's branch (see lowerLane in
	// layout_lanes.go), which the main line does not wait for.
	cursor, laneBusy, extent := 0, 0, 0
	maxHeight := ActivityHeight

	var prev ast.MicroflowStatement
	for i, stmt := range stmts {
		tail := atTail && i == len(stmts)-1
		bounds := m.measureStatement(stmt)
		if ifStmt, ok := stmt.(*ast.IfStmt); ok && tail {
			bounds = m.measureIf(ifStmt, true)
		}
		maxHeight = max(maxHeight, bounds.Height)
		if bounds.Width == 0 {
			continue
		}
		left := cursor
		if prev != nil {
			left += gapBetween(prev, stmt)
		} else {
			// The builder centres the first element where an activity's centre
			// would be, so the run is measured from that activity's left edge. A
			// split is narrower than an activity and starts inside it
			// (ako/mxcli#1773).
			left = max(ActivityWidth/2-leftHalfOf(stmt), 0)
		}
		if left < laneBusy && reachesBelowMainLine(stmt, bounds) {
			left = laneBusy
		}
		if guard, ok := stmt.(*ast.IfStmt); ok && isGuard(guard) && m.layoutStyle().Guard == GuardTrueRight {
			return m.measureGuardTrueRight(guard, stmts[i+1:], left, extent, maxHeight)
		}
		if ifStmt, ok := stmt.(*ast.IfStmt); ok && !ifHasMerge(ifStmt, tail) {
			// No merge: the main line resumes where addIfStatement's no-merge
			// exit puts it, not past the far end of the branches, and the lane
			// under it is taken up to the branches' end.
			//
			// The cursor is where the next element is placed from, not something
			// drawn, so it is not part of the extent: an IF that ends the run
			// reaches only as far as its split and its branches.
			branchRight := left + bounds.Width
			cursor = left + m.ifMainWidth(ifStmt)
			laneBusy = max(laneBusy, branchRight+laneGap)
			extent = max(extent, branchRight, left+SplitWidth)
			prev = stmt
			continue
		}
		cursor = left + bounds.Width
		if row := m.handlerRowWidth(stmt); row > 0 {
			// The handler's row hangs in the same lane, from the activity's left
			// edge, and the main line goes on above it — as reserveHandlerRow
			// records it for the builder.
			rowRight := left + row
			laneBusy = max(laneBusy, rowRight+laneGap)
			extent = max(extent, rowRight)
			if rejoinsPastRow(stmtErrorHandling(stmt), row) {
				// A row that falls through rejoins at a merge one gap past it,
				// and the main line goes on from that merge (rejoinAfterRow).
				cursor = max(cursor, rowRight+laneGap+MergeSize)
			}
		}
		extent = max(extent, cursor)
		prev = stmt
	}

	return Bounds{Width: extent, Height: maxHeight}
}

// measureBranch measures a branch body including the end event a trailing RETURN
// draws. measureStatements gives RETURN no width, because on the main line the end
// event is the flow's own; in a branch it is an extra element one pitch past the last
// activity, and a merge placed by the activities alone lands on top of it.
func (m *layoutMeasurer) measureBranch(stmts []ast.MicroflowStatement) Bounds {
	b := m.measureStatements(stmts)
	if !lastStmtDrawsEndEvent(stmts) {
		return b
	}
	if b.Width == 0 {
		// A bare RETURN: the end event stands where the first activity would.
		b.Width = ActivityWidth/2 + EventSize/2
	} else {
		b.Width += HorizontalSpacing - ActivityWidth/2 + EventSize/2
	}
	return b
}

// lastStmtDrawsEndEvent reports whether a branch ends in a statement that draws
// its own event one pitch past the last activity: RETURN, RAISE ERROR, BREAK or
// CONTINUE. A `join` ends the path too but draws nothing — its edge goes to a
// merge placed elsewhere — and a decision whose every branch ends draws its events
// inside its own branches, which its own measurement already holds. Counting
// either as an end event measured a handler row ending in a join at about twice
// its drawn width (ako/mxcli#1773).
func lastStmtDrawsEndEvent(stmts []ast.MicroflowStatement) bool {
	if len(stmts) == 0 {
		return false
	}
	switch stmts[len(stmts)-1].(type) {
	case *ast.ReturnStmt, *ast.RaiseErrorStmt, *ast.BreakStmt, *ast.ContinueStmt:
		return true
	}
	return false
}

// gapBetween is the empty space the builder actually leaves between two consecutive
// elements of a run, edge to edge.
//
// It used to be HorizontalSpacing for every pair. But HorizontalSpacing is a
// centre-to-centre pitch — the builder does `posX += spacing` and centres each
// activity on posX — so adding it on top of both widths counted the activity twice:
// two activities were measured 400 wide and occupy 280. Every IF sized its branch
// from that, so a merge sat a whole activity further right per extra statement in
// the branch, and the main line ran empty underneath it.
//
// Only the pairs whose arithmetic is known exactly are tightened; anything else
// keeps the old, generous gap, because a merge placed short of its branch's content
// is worse than a merge placed long.
func gapBetween(prev, next ast.MicroflowStatement) int {
	// Distance from prev's measured right edge to the centre the builder gives next.
	var toNextCentre int
	switch prev.(type) {
	case *ast.EnumSplitStmt, *ast.InheritanceSplitStmt:
		return HorizontalSpacing
	case *ast.MergeStmt:
		// addMergeStatement advances half a pitch from the merge's centre, which
		// leaves an activity after it touching it: no gap, and no floor either,
		// or the run is measured wider than it is drawn.
		return max(HorizontalSpacing/2-MergeSize/2-leftHalfOf(next), 0)
	default:
		// A simple activity, a loop box or an IF: after each, the builder centres
		// the next element one activity-gap plus half an activity past the right
		// edge. An IF's right edge is its merge's (posX = mergeX + MergeSize +
		// HorizontalSpacing/2); one without a merge is given a main-line width
		// that ends where this puts the next element (ifMainWidth).
		toNextCentre = HorizontalSpacing - ActivityWidth/2
	}
	// How far next reaches left of the centre it is given.
	return max(toNextCentre-leftHalfOf(next), 20)
}

// measureStatementsSpan returns the horizontal extent a statement run actually
// occupies once laid out, for runs where that can be derived exactly.
//
// measureStatements sums every element's full width and adds HorizontalSpacing
// between them. HorizontalSpacing is a centre-to-centre pitch — the builder does
// `posX += spacing` and centres each activity on posX — so counting it *on top of*
// each width over-measures a run of n simple activities by (n-1)*ActivityWidth.
// Sizing a loop box from that left it far wider than its contents: 880px around
// 440px of activities for a three-statement body (mendixlabs/mxcli#790).
//
// The correction applies only when every element is a simple activity, whose pitch
// is exactly HorizontalSpacing. A compound element (IF/split, nested loop) advances
// posX by geometry this function cannot reproduce without duplicating the builder —
// guessing there under-sizes the box and pushes activities outside it, which is
// worse than a box that is too wide. Such runs fall back to measureStatements.
func (m *layoutMeasurer) measureStatementsSpan(stmts []ast.MicroflowStatement) Bounds {
	count := 0
	maxHeight := ActivityHeight
	for _, stmt := range stmts {
		b := m.measureStatement(stmt)
		maxHeight = max(maxHeight, b.Height)
		if b.Width == 0 {
			continue
		}
		if b.Width != ActivityWidth {
			return m.measureStatements(stmts) // compound element — cannot place exactly
		}
		count++
	}
	if count == 0 {
		return Bounds{Width: 0, Height: maxHeight}
	}
	// n activities centred HorizontalSpacing apart span from the first centre minus
	// half a width to the last centre plus half a width.
	return Bounds{Width: (count-1)*HorizontalSpacing + ActivityWidth, Height: maxHeight}
}

// measureStatement calculates the bounds for a single statement
func (m *layoutMeasurer) measureStatement(stmt ast.MicroflowStatement) Bounds {
	switch s := stmt.(type) {
	case *ast.IfStmt:
		return m.measureIfStatement(s)
	case *ast.EnumSplitStmt:
		return m.measureEnumSplitStatement(s)
	case *ast.InheritanceSplitStmt:
		return m.measureInheritanceSplitStatement(s)
	case *ast.LoopStmt:
		return m.measureLoopStatement(s)
	case *ast.WhileStmt:
		return m.measureWhileStatement(s)
	case *ast.ReturnStmt:
		// Return doesn't add visual element (handled by EndEvent)
		return Bounds{Width: 0, Height: 0}
	case *ast.JoinStmt:
		// A join draws nothing: its edge goes to a merge declared elsewhere.
		return Bounds{Width: 0, Height: 0}
	case *ast.MergeStmt:
		return Bounds{Width: MergeSize, Height: MergeSize}
	default:
		// Simple activities have fixed size, but a custom error handler's row
		// hangs under the activity. Heights are read as symmetric about the
		// centre line, so the row's depth counts on both sides; without it an
		// IF laid its ELSE row on top of a THEN activity's handler row.
		if len(getErrorHandlerBody(stmt)) > 0 {
			return Bounds{Width: ActivityWidth, Height: 2*ErrorHandlerPitch + ActivityHeight}
		}
		return Bounds{Width: ActivityWidth, Height: ActivityHeight}
	}
}

func (m *layoutMeasurer) measureEnumSplitStatement(s *ast.EnumSplitStmt) Bounds {
	if m.layoutStyle().CaseSplit == CaseSplitColumns {
		bodies := make([][]ast.MicroflowStatement, 0, len(s.Cases)+1)
		for _, c := range s.Cases {
			bodies = append(bodies, c.Body)
		}
		if len(s.ElseBody) > 0 {
			bodies = append(bodies, s.ElseBody)
		}
		if len(bodies) > 1 {
			shift := 0
			if len(s.Cases) > 0 && len(s.Cases[0].Values) > 1 {
				shift = firstCaseGroupShift
			}
			return m.measureCaseColumns(bodies, SplitWidth, shift)
		}
	}
	maxBranchWidth := 0
	var branchHeights []int
	for _, c := range s.Cases {
		bounds := m.measureBranch(c.Body)
		maxBranchWidth = max(maxBranchWidth, bounds.Width)
		branchHeights = append(branchHeights, max(bounds.Height, ActivityHeight))
	}
	if len(s.ElseBody) > 0 {
		bounds := m.measureBranch(s.ElseBody)
		maxBranchWidth = max(maxBranchWidth, bounds.Width)
		branchHeights = append(branchHeights, max(bounds.Height, ActivityHeight))
	}
	if maxBranchWidth == 0 {
		maxBranchWidth = HorizontalSpacing / 2
	}
	if len(branchHeights) == 0 {
		branchHeights = []int{ActivityHeight}
	}

	totalHeight := 0
	for _, h := range branchHeights {
		totalHeight += h
	}
	totalHeight += (len(branchHeights) - 1) * BranchGap

	width := SplitWidth + HorizontalSpacing/2 + maxBranchWidth + MergeSize
	return Bounds{Width: width, Height: totalHeight}
}

func (m *layoutMeasurer) measureInheritanceSplitStatement(s *ast.InheritanceSplitStmt) Bounds {
	if m.layoutStyle().CaseSplit == CaseSplitColumns {
		bodies := make([][]ast.MicroflowStatement, 0, len(s.Cases)+1)
		for _, c := range s.Cases {
			bodies = append(bodies, c.Body)
		}
		if len(s.ElseBody) > 0 {
			bodies = append(bodies, s.ElseBody)
		}
		if len(bodies) > 1 {
			return m.measureCaseColumns(bodies, ActivityWidth, 0)
		}
	}
	maxBranchWidth := 0
	branchCount := len(s.Cases)
	for _, c := range s.Cases {
		bounds := m.measureStatements(c.Body)
		maxBranchWidth = max(maxBranchWidth, bounds.Width)
	}
	if len(s.ElseBody) > 0 {
		bounds := m.measureStatements(s.ElseBody)
		maxBranchWidth = max(maxBranchWidth, bounds.Width)
		branchCount++
	}
	if maxBranchWidth == 0 {
		maxBranchWidth = HorizontalSpacing / 2
	}
	if branchCount == 0 {
		branchCount = 1
	}

	width := ActivityWidth + HorizontalSpacing/2 + maxBranchWidth + HorizontalSpacing/2 + MergeSize
	height := ActivityHeight + (branchCount-1)*VerticalSpacing
	return Bounds{Width: width, Height: height}
}

// measureIfStatement calculates bounds for IF/ELSE
// Layout strategy matches addIfStatement:
// - IF with ELSE: TRUE path horizontal, FALSE path below
// - IF without ELSE: FALSE path horizontal, TRUE path below
func (m *layoutMeasurer) measureIfStatement(s *ast.IfStmt) Bounds {
	return m.measureIf(s, false)
}

// measureIf measures an IF from the split's LEFT edge to the right edge of what
// it draws furthest right: its merge, or without one the end of its branches.
// atTail is the builder's atBodyTail. It used to be measured from the split's
// centre, as if that were its left edge, and came out 45px short: the merge of an
// enclosing IF stood inside the activity after it (ako/mxcli#1773).
func (m *layoutMeasurer) measureIf(s *ast.IfStmt, atTail bool) Bounds {
	// Measure THEN branch
	thenBounds := m.measureBranch(s.ThenBody)

	// Measure ELSE branch
	elseBounds := m.measureBranch(s.ElseBody)

	// Width: split + max(then, else) + merge + spacing
	branchWidth := max(thenBounds.Width, elseBounds.Width)
	// If branches are empty, we still need some width for the flow lines
	if branchWidth == 0 {
		branchWidth = HorizontalSpacing / 2
	}

	elseColumn := m.layoutStyle().IfElse == IfElseColumn && (s.HasElse || len(s.ElseBody) > 0)
	var column Bounds
	if elseColumn {
		column = m.measureColumn(s.ElseBody)
		branchWidth = elseColumnBranchWidth(thenBounds, column)
		if branchWidth == 0 {
			branchWidth = HorizontalSpacing / 2
		}
	}

	// addIfStatement centres the merge at splitX + SplitWidth +
	// HorizontalSpacing/2 + branchWidth; the split reaches SplitWidth/2 left of
	// splitX.
	totalWidth := SplitWidth/2 + SplitWidth + HorizontalSpacing/2 + branchWidth + MergeSize/2
	// Without a merge — a guard, `if X then ...; return; end if`, or an IF/ELSE
	// one of whose branches ends the flow — the IF reaches to the end of its
	// branches, which start guardBranchInset right of the split's centre.
	// Measuring a merge and its spacing that are never drawn left 120px of empty
	// main line after every guard nested in a branch.
	if !ifHasMerge(s, atTail) {
		totalWidth = SplitWidth/2 + guardBranchInset + branchWidth
	}

	// Height depends on layout strategy
	var totalHeight int
	if elseColumn {
		// The column hangs under the split, below the TRUE body.
		// Reported as if centred on the line, as every Bounds is read: twice
		// what reaches below it (see measureGuardTrueRight).
		totalHeight = 2 * (max(thenBounds.Height, ActivityHeight)/2 + BranchGap + max(column.Height, ActivityHeight))
	} else if len(s.ElseBody) > 0 {
		// IF WITH ELSE: TRUE path horizontal (main line), FALSE path below
		// Height = THEN branch height + gap + ELSE branch height
		thenHeight := max(thenBounds.Height, ActivityHeight)
		elseHeight := max(elseBounds.Height, ActivityHeight)
		totalHeight = thenHeight + BranchGap + elseHeight
	} else {
		// IF WITHOUT ELSE: FALSE path horizontal (main line), TRUE path below
		// Height = main activity height + gap + THEN branch height
		thenHeight := max(thenBounds.Height, ActivityHeight)
		totalHeight = ActivityHeight + BranchGap + thenHeight
	}

	return Bounds{Width: totalWidth, Height: totalHeight}
}

// measureLoopStatement calculates bounds for LOOP
func (m *layoutMeasurer) measureLoopStatement(s *ast.LoopStmt) Bounds {
	// Measure loop body
	bodyBounds := m.measureRun(s.Body, true)

	// Loop box size: body + padding on all sides, plus the iterator column the
	// builder reserves on the left. Leaving the column out measured every foreach
	// loop 100px short, so the merge of an IF holding one landed inside the
	// loop's box (mendixlabs/mxcli#1598, case 4).
	width := max(bodyBounds.Width+2*LoopPadding+LoopIteratorSpace, MinLoopWidth)
	height := max(bodyBounds.Height+2*LoopPadding, MinLoopHeight)

	return Bounds{Width: width, Height: height}
}

// measureWhileStatement calculates bounds for WHILE
func (m *layoutMeasurer) measureWhileStatement(s *ast.WhileStmt) Bounds {
	bodyBounds := m.measureRun(s.Body, true)
	width := max(bodyBounds.Width+2*LoopPadding, MinLoopWidth)
	height := max(bodyBounds.Height+2*LoopPadding, MinLoopHeight)
	return Bounds{Width: width, Height: height}
}

// LayoutContext holds the current position during layout
type LayoutContext struct {
	X        int // Current X position
	Y        int // Current Y position (baseline for THEN path)
	BaseY    int // Original Y for returning after ELSE branch
	VarTypes map[string]string
}

// NewLayoutContext creates a new layout context
func NewLayoutContext(startX, startY int) *LayoutContext {
	return &LayoutContext{
		X:        startX,
		Y:        startY,
		BaseY:    startY,
		VarTypes: make(map[string]string),
	}
}

// Advance moves X position forward by given amount
func (ctx *LayoutContext) Advance(dx int) {
	ctx.X += dx
}

// AdvanceToNext moves to the next activity position
func (ctx *LayoutContext) AdvanceToNext() {
	ctx.X += HorizontalSpacing
}

// Note: Position in Mendix is stored as RelativeMiddlePoint, which IS the center
// of the element. No offset calculations needed - just use the center coordinates directly.

// Connection anchor indices for SequenceFlow
// These determine which side of an element the connection attaches to
const (
	AnchorTop    = 0
	AnchorRight  = 1
	AnchorBottom = 2
	AnchorLeft   = 3
)

// containerBounds returns the bounding box actually occupied by a container's
// children, in the container's own coordinate space. Each object contributes
// Position ± Size/2, since a Mendix position is the element's centre.
func containerBounds(objects []microflows.MicroflowObject) (minX, minY, maxX, maxY int, n int) {
	for _, o := range objects {
		if o == nil {
			continue
		}
		p := o.GetPosition()
		var sz model.Size
		if withSize, ok := o.(interface{ GetSize() model.Size }); ok {
			sz = withSize.GetSize()
		}
		l, t := p.X-sz.Width/2, p.Y-sz.Height/2
		r, b := p.X+sz.Width/2, p.Y+sz.Height/2
		if n == 0 {
			minX, minY, maxX, maxY = l, t, r, b
		} else {
			minX, minY = min(minX, l), min(minY, t)
			maxX, maxY = max(maxX, r), max(maxY, b)
		}
		n++
	}
	return
}

// fitContainerSize sizes a container box to the children it actually holds
// (mendixlabs/mxcli#884 problem 1).
//
// The box used to come from measureStatementsSpan — a pre-pass over the AST run
// BEFORE the body was built — so it was a function of statement COUNT alone.
// Varying only the children's positions left it unchanged: two activities at
// x=150/310, at x=1500/2000 and at x=160/170 all produced 480;160, and in the
// second case both children sat entirely outside their own container. The model
// carries no geometry rules, so `mx check` reports nothing.
//
// Sizing happens AFTER the body is built, which is what makes an explicit
// `@position` on a child effective. Nesting falls out of the recursion: an inner
// loop is sized when its own addLoopStatement returns, so by the time the outer
// container measures it, its Size is already correct.
//
// The children are NOT translated. Their positions round-trip through DESCRIBE
// as `@position`, so moving them would make a describe→exec cycle produce
// different coordinates than it read — and under ADR-0008 that turns an
// otherwise-quiet re-run into a write. The box grows to fit the contents
// instead, which is also what a hand-laid-out loop needs.
func fitContainerSize(objects []microflows.MicroflowObject, leftInset, minWidth, minHeight int) (width, height int) {
	_, _, maxX, maxY, n := containerBounds(objects)
	if n == 0 {
		return minWidth, minHeight
	}
	width = max(maxX+LoopPadding, minWidth)
	height = max(maxY+LoopPadding, minHeight)
	_ = leftInset
	return width, height
}
