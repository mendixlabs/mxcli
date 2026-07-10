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
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
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
	VerticalSpacing   = 90  // Space between branches for error-handler flows
	BranchGap         = 40  // Minimum edge-to-edge gap between parallel branches
	LoopPadding       = 50  // Padding inside loop boxes
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
}

// measureStatements calculates the total bounds for a list of statements.
// Spacing is only added between pairs of non-zero-width elements so that
// zero-width statements (e.g. ReturnStmt, which produces no visual box)
// don't artificially inflate the measured width.
func (m *layoutMeasurer) measureStatements(stmts []ast.MicroflowStatement) Bounds {
	if len(stmts) == 0 {
		return Bounds{Width: 0, Height: 0}
	}

	totalWidth := 0
	maxHeight := ActivityHeight

	for _, stmt := range stmts {
		bounds := m.measureStatement(stmt)
		maxHeight = max(maxHeight, bounds.Height)
		if bounds.Width == 0 {
			continue
		}
		if totalWidth > 0 {
			totalWidth += HorizontalSpacing
		}
		totalWidth += bounds.Width
	}

	return Bounds{Width: totalWidth, Height: maxHeight}
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
	default:
		// Simple activities have fixed size
		return Bounds{Width: ActivityWidth, Height: ActivityHeight}
	}
}

func (m *layoutMeasurer) measureEnumSplitStatement(s *ast.EnumSplitStmt) Bounds {
	maxBranchWidth := 0
	var branchHeights []int
	for _, c := range s.Cases {
		bounds := m.measureStatements(c.Body)
		maxBranchWidth = max(maxBranchWidth, bounds.Width)
		branchHeights = append(branchHeights, max(bounds.Height, ActivityHeight))
	}
	if len(s.ElseBody) > 0 {
		bounds := m.measureStatements(s.ElseBody)
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

	width := SplitWidth + HorizontalSpacing/2 + maxBranchWidth + HorizontalSpacing/2 + MergeSize
	return Bounds{Width: width, Height: totalHeight}
}

func (m *layoutMeasurer) measureInheritanceSplitStatement(s *ast.InheritanceSplitStmt) Bounds {
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
	// Measure THEN branch
	thenBounds := m.measureStatements(s.ThenBody)

	// Measure ELSE branch
	elseBounds := m.measureStatements(s.ElseBody)

	// Width: split + max(then, else) + merge + spacing
	branchWidth := max(thenBounds.Width, elseBounds.Width)
	// If branches are empty, we still need some width for the flow lines
	if branchWidth == 0 {
		branchWidth = HorizontalSpacing / 2
	}

	totalWidth := SplitWidth + HorizontalSpacing/2 + branchWidth + HorizontalSpacing/2 + MergeSize

	// Height depends on layout strategy
	var totalHeight int
	if len(s.ElseBody) > 0 {
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
	bodyBounds := m.measureStatements(s.Body)

	// Loop box size: body + padding on all sides
	width := max(bodyBounds.Width+2*LoopPadding, MinLoopWidth)
	height := max(bodyBounds.Height+2*LoopPadding, MinLoopHeight)

	return Bounds{Width: width, Height: height}
}

// measureWhileStatement calculates bounds for WHILE
func (m *layoutMeasurer) measureWhileStatement(s *ast.WhileStmt) Bounds {
	bodyBounds := m.measureStatements(s.Body)
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
