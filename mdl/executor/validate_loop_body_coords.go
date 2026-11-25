// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Inside a loop body @position is in the loop BOX's coordinates, from its
// top-left corner, not the canvas's (ako/mxcli#2564): Mendix stores a body node
// that way and describe prints it so. A body written in canvas coordinates
// passed check and exec, and the builder sized the box to reach it, so the box
// grew with the loop's position on the canvas and covered its neighbours
// (ako/mxcli#2583, upstream mendixlabs/mxcli#1935). Measured: a loop at 600,200
// with its body at 600,300 stored Size 710;380; at 1200, 1310;380.
//
// The builder's box runs from its own left/top edge to the body's right/bottom
// edge plus LoopPadding (fitContainerSize). A body in box coordinates starts
// near that edge — the first node at (210, height / 2) — so more than half the
// box lying empty before the body, on either axis, is the signature of canvas
// coordinates. A stated @size is the author's box and is left alone, and so is
// a body the builder places itself.

const loopBodyCoordsRuleID = "MDL-LOOP01"

// loopBodyEmptyMin is how much empty space before the body is tolerated on
// either axis whatever the box's size. A box-coordinate body's first node sits
// at x = 210, its left edge at 150, and an author's own margin may be wider; a
// canvas body near the canvas's left edge, which this lets through, makes a box
// only somewhat too large.
const loopBodyEmptyMin = 300

// checkLoopBodyCoordinates reports a loop or while whose body is positioned in
// canvas coordinates.
func (v *microflowValidator) checkLoopBodyCoordinates(s ast.MicroflowStatement) {
	var body []ast.MicroflowStatement
	var what string
	switch stmt := s.(type) {
	case *ast.LoopStmt:
		body = stmt.Body
		what = fmt.Sprintf("loop $%s in $%s", stmt.LoopVariable, stmt.ListVariable)
		if stmt.ListPath != "" {
			what = fmt.Sprintf("loop $%s in %s", stmt.LoopVariable, stmt.ListPath)
		}
	case *ast.WhileStmt:
		body = stmt.Body
		what = "while loop"
	default:
		return
	}
	if ann := ast.StatementAnnotations(s); ann != nil && ann.Size != nil && ann.Size.Width > 0 && ann.Size.Height > 0 {
		return
	}
	n := 0
	var minX, minY, maxX, maxY int
	var first *ast.Position
	for _, b := range body {
		ann := ast.StatementAnnotations(b)
		if ann == nil || ann.Position == nil {
			continue
		}
		p := ann.Position
		if first == nil {
			first = p
		}
		if n == 0 {
			minX, minY, maxX, maxY = p.X, p.Y, p.X, p.Y
		} else {
			minX, minY = min(minX, p.X), min(minY, p.Y)
			maxX, maxY = max(maxX, p.X), max(maxY, p.Y)
		}
		n++
	}
	if n == 0 {
		return
	}
	// The box fitContainerSize would compute, each node taken as an activity.
	width := max(maxX+ActivityWidth/2+LoopPadding, MinLoopWidth)
	height := max(maxY+ActivityHeight/2+LoopPadding, MinLoopHeight)
	emptyLeft := minX - ActivityWidth/2
	emptyTop := minY - ActivityHeight/2
	wasteX := emptyLeft > loopBodyEmptyMin && emptyLeft > width/2
	wasteY := emptyTop > loopBodyEmptyMin && emptyTop > height/2
	if !wasteX && !wasteY {
		return
	}
	// Moving the body so its top-left node sits where the builder would put it.
	// Once one axis shows canvas coordinates both are canvas, so the other
	// moves too where more than half the box is empty before it.
	dx, dy := 0, 0
	if emptyLeft > width/2 {
		dx = minX - 210
	}
	if emptyTop > height/2 {
		dy = minY - 80
	}
	v.addViolation(loopBodyCoordsRuleID, linter.SeverityWarning,
		fmt.Sprintf("the body of `%s` looks positioned in canvas coordinates: its box would be %dx%d with %dpx empty "+
			"before the body, and the box is centred on the loop, so it covers the activities around it", what, width, height,
			max(emptyLeft, emptyTop)),
		fmt.Sprintf("Inside a loop body @position is in the loop box's coordinates, from its top-left corner "+
			"(ako/mxcli#2564). Subtract (%d, %d) from every body @position: @position(%d, %d) becomes @position(%d, %d). "+
			"Or leave the body's @position out and let mxcli place it, or state the box with @size(w, h).",
			dx, dy, first.X, first.Y, first.X-dx, first.Y-dy))
}
