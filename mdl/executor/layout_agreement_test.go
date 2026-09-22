// SPDX-License-Identifier: Apache-2.0

// Package executor - the measurer and the builder have to agree.
//
// Two numbers describe the same element: what measureStatements predicts it will
// occupy, and where the builder actually puts it. Everything that places an element
// beside another one — a merge after its branch, the next statement on the main
// line, a branch lane under the one above — is derived from the first and drawn by
// the second, so a disagreement of a few pixels is not cosmetic: it is an overlap.
//
// Measured on mdl-examples/doctype-tests/02c-complex-layout-examples.mdl, whose six
// flows carry nested decisions, named merges and backward jumps: 15 pairs of
// overlapping elements in 3 of the 6 flows, all from three fixed-size disagreements.
// A split was measured to its branch rather than to the merge that closes it (25px
// short), the advance past a merge came from its centre as though it were an
// activity's width (80px where 120 was needed), and a branch lane was placed half
// the branch's measured HEIGHT below the line although a branch holding a nested IF
// hangs entirely below its own line (50px short).
//
// These tests pin the agreement rather than the pixel values: each asks the builder
// where an element ended up and the measurer what it predicted, and compares.
package executor

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// box is an element's rectangle on the canvas.
type box struct {
	left, right, top, bottom int
	what                     string
}

// boxesOf returns every object the builder placed, as rectangles. Loop children are
// in their own coordinate space inside the box, so a LoopedActivity's contents are
// not part of this list — they cannot collide with anything outside it.
func boxesOf(objects []microflows.MicroflowObject) []box {
	var out []box
	for _, o := range objects {
		if o == nil {
			continue
		}
		w, h := ActivityWidth, ActivityHeight
		switch o.(type) {
		case *microflows.ExclusiveSplit, *microflows.InheritanceSplit:
			w, h = SplitWidth, SplitHeight
		case *microflows.ExclusiveMerge:
			w, h = MergeSize, MergeSize
		case *microflows.StartEvent, *microflows.EndEvent:
			w, h = EventSize, EventSize
		case *microflows.Annotation:
			w, h = DefaultAnnotationSize.Width, DefaultAnnotationSize.Height
		}
		if ws, ok := o.(interface{ GetSize() model.Size }); ok {
			if s := ws.GetSize(); s.Width > 0 && s.Height > 0 {
				w, h = s.Width, s.Height
			}
		}
		p := o.GetPosition()
		out = append(out, box{p.X - w/2, p.X + w/2, p.Y - h/2, p.Y + h/2, fmt.Sprintf("%T at (%d,%d)", o, p.X, p.Y)})
	}
	return out
}

// buildFlow lays out a whole microflow the way exec does.
func buildFlow(stmts []ast.MicroflowStatement) *flowBuilder {
	fb := &flowBuilder{
		posX: 200, posY: 200, baseY: 200, spacing: HorizontalSpacing, allowWrap: true,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
		measurer: &layoutMeasurer{varTypes: map[string]string{}},
	}
	fb.buildFlowGraph(stmts, nil)
	return fb
}

func ifElse(then, els []ast.MicroflowStatement) *ast.IfStmt {
	return &ast.IfStmt{
		Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
		ThenBody:  then,
		ElseBody:  els,
		HasElse:   true,
	}
}

func casesOf(bodies ...[]ast.MicroflowStatement) *ast.EnumSplitStmt {
	s := &ast.EnumSplitStmt{Variable: "Status"}
	for i, b := range bodies {
		s.Cases = append(s.Cases, ast.EnumSplitCase{Values: []string{fmt.Sprintf("V%d", i+1)}, Body: b})
	}
	return s
}

// TestBuiltElementsDoNotOverlap is the invariant the three fixes exist to keep. The
// shapes are the ones the complex-layout examples are built from; each overlapped
// before its fix.
func TestBuiltElementsDoNotOverlap(t *testing.T) {
	shapes := map[string][]ast.MicroflowStatement{
		"an activity after a named merge": {
			&ast.MergeStmt{Label: "rejoin"},
			&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "after"}},
		},
		"an activity after a decision": {
			ifElse(logStatements(2), logStatements(1)),
			&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "after"}},
		},
		// The ELSE branch is several activities long on purpose: a nested decision's
		// own else lane hangs below the THEN line one column further right, so a
		// one-activity ELSE passes underneath it and collides with nothing. It is the
		// SECOND activity of the ELSE that arrives in that column.
		"a decision whose THEN holds another decision": {
			ifElse(
				[]ast.MicroflowStatement{ifElse(logStatements(1), logStatements(1))},
				logStatements(3),
			),
			&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "after"}},
		},
		"two decisions in a row": {
			ifElse(logStatements(1), logStatements(1)),
			ifElse(logStatements(1), logStatements(1)),
		},
		"an activity after a CASE": {
			casesOf(logStatements(1), logStatements(1), logStatements(1)),
			&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "after"}},
		},
		"a CASE whose branches hold decisions": {
			casesOf(
				[]ast.MicroflowStatement{ifElse(logStatements(1), logStatements(1))},
				logStatements(3),
				[]ast.MicroflowStatement{ifElse(logStatements(1), logStatements(1))},
			),
			&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "after"}},
		},
		"a guard, then a decision": {
			guardStmt(),
			ifElse(logStatements(1), logStatements(1)),
		},
		"a loop between two decisions": {
			ifElse(logStatements(1), logStatements(1)),
			&ast.LoopStmt{LoopVariable: "Item", ListVariable: "List", Body: logStatements(2)},
			ifElse(logStatements(1), logStatements(1)),
		},
	}

	for name, stmts := range shapes {
		t.Run(name, func(t *testing.T) {
			boxes := boxesOf(buildFlow(stmts).objects)
			for i := 0; i < len(boxes); i++ {
				for j := i + 1; j < len(boxes); j++ {
					a, b := boxes[i], boxes[j]
					if a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom {
						t.Errorf("%s overlaps %s", a.what, b.what)
					}
				}
			}
		})
	}
}

// TestMeasuredWidthMatchesWhatIsBuilt asks the two halves the same question. The
// measurer's answer is what every merge and every following element is placed from,
// so it may not be short — and a measure that is merely generous wastes the canvas.
func TestMeasuredWidthMatchesWhatIsBuilt(t *testing.T) {
	runs := map[string][]ast.MicroflowStatement{
		"a decision":                 {ifElse(logStatements(2), logStatements(1))},
		"a decision then a decision": {ifElse(logStatements(1), logStatements(1)), ifElse(logStatements(2), logStatements(1))},
		"a decision then an activity": {
			ifElse(logStatements(2), logStatements(1)),
			&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "after"}},
		},
		"a CASE then an activity": {
			casesOf(logStatements(2), logStatements(1)),
			&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "after"}},
		},
		"a guard then an activity": {
			guardStmt(),
			&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "after"}},
		},
	}

	for name, stmts := range runs {
		t.Run(name, func(t *testing.T) {
			fb := buildFlow(stmts)
			// Everything the statements drew: the flow's start event sits one spacing
			// before the run and its final end event one after, so both are dropped —
			// but an end event a BRANCH draws is part of that branch and stays.
			objects := fb.objects
			if len(objects) > 0 {
				if _, isStart := objects[0].(*microflows.StartEvent); isStart {
					objects = objects[1:]
				}
			}
			if n := len(objects); n > 0 {
				if _, isEnd := objects[n-1].(*microflows.EndEvent); isEnd {
					objects = objects[:n-1]
				}
			}
			left, right := 1<<30, -(1 << 30)
			for _, b := range boxesOf(objects) {
				left, right = min(left, b.left), max(right, b.right)
			}
			built := right - left
			measured := (&layoutMeasurer{varTypes: map[string]string{}}).measureStatements(stmts).Width
			if measured != built {
				t.Errorf("measured %d, built %d — a short measure overlaps the next element, a long one wastes canvas",
					measured, built)
			}
		})
	}
}

// TestNamedMergeLeavesTheOrdinaryGap pins a distance, not an absence of overlap: a
// zero gap puts two edges on top of each other without the rectangles intersecting,
// so only a measurement catches it. `merge <label>` advanced half a pitch from the
// merge's CENTRE, which is what an activity's width needs; a merge is 40 wide.
func TestNamedMergeLeavesTheOrdinaryGap(t *testing.T) {
	fb := buildFlow([]ast.MicroflowStatement{
		&ast.MergeStmt{Label: "rejoin"},
		&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "after"}},
	})
	var merge, activity model.Point
	for _, o := range fb.objects {
		switch o.(type) {
		case *microflows.ExclusiveMerge:
			merge = o.GetPosition()
		case *microflows.ActionActivity:
			activity = o.GetPosition()
		}
	}
	if merge.X == 0 || activity.X == 0 {
		t.Fatal("expected a merge and an activity")
	}
	if gap := (activity.X - ActivityWidth/2) - (merge.X + MergeSize/2); gap != laneGap {
		t.Errorf("gap between the merge and the activity after it is %d, want %d — the same "+
			"space two activities have", gap, laneGap)
	}
}
