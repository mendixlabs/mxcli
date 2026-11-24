// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1935: a new loop's box was sized from its body's ABSOLUTE
// canvas coordinates, so moving the loop 600px right grew the box by exactly
// 600 (body at 600,300 → 710;380; body at 1200,300 → 1310;380) and the box,
// being centred on the loop, swallowed its neighbours. The box must come from
// the body's bounds RELATIVE to the loop, and a body activity's @position —
// a canvas coordinate everywhere else in the language — must be translated
// into the loop's own frame before it is stored.

func loopOf1935(t *testing.T, coll *microflows.MicroflowObjectCollection) *microflows.LoopedActivity {
	t.Helper()
	for _, o := range coll.Objects {
		if l, ok := o.(*microflows.LoopedActivity); ok {
			return l
		}
	}
	t.Fatal("no loop built")
	return nil
}

func build1935(t *testing.T, body string) *microflows.MicroflowObjectCollection {
	t.Helper()
	src := "create microflow M.F ($L: List of M.E)\nbegin\n" + body + "end;\n"
	prog, errs := visitor.Build(src)
	if len(errs) != 0 {
		t.Fatalf("parse: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	fb := &flowBuilder{
		posX: 100, posY: 200, spacing: HorizontalSpacing,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
		measurer: &layoutMeasurer{},
	}
	return fb.buildFlowGraph(mf.Body, mf.ReturnType)
}

// The issue's own reproduction, twice, differing only in the loop's x.
func TestLoopBoxDoesNotGrowWithLoopPosition(t *testing.T) {
	small := loopOf1935(t, build1935(t, "  @position(600, 200)\n  loop $I in $L\n  begin\n    @position(600, 300)\n    log info node 'x' 'a';\n  end loop;\n"))
	large := loopOf1935(t, build1935(t, "  @position(1200, 200)\n  loop $I in $L\n  begin\n    @position(1200, 300)\n    log info node 'x' 'a';\n  end loop;\n"))
	if small.Size != large.Size {
		t.Errorf("moving the loop 600px changed its box: %v vs %v — the box must be sized from the body relative to the loop, not from canvas coordinates",
			small.Size, large.Size)
	}
}

// A body activity's stored position is relative to the loop's frame, so the
// rendered child must sit where the script placed it and inside its container
// (what MPR011 checks).
func TestLoopBodyPositionStoredRelativeToLoopFrame(t *testing.T) {
	coll := build1935(t, "  @position(600, 200)\n  loop $I in $L\n  begin\n    @position(600, 300)\n    log info node 'x' 'a';\n  end loop;\n")
	loop := loopOf1935(t, coll)
	if len(loop.ObjectCollection.Objects) != 1 {
		t.Fatalf("loop holds %d objects, want 1", len(loop.ObjectCollection.Objects))
	}
	child := loop.ObjectCollection.Objects[0]
	p := child.GetPosition()
	left, top := p.X-ActivityWidth/2, p.Y-ActivityHeight/2
	right, bottom := p.X+ActivityWidth/2, p.Y+ActivityHeight/2
	if left < 0 || top < 0 || right > loop.Size.Width || bottom > loop.Size.Height {
		t.Errorf("child stored at %v escapes its container %v — the canvas @position was not translated into the loop's frame", p, loop.Size)
	}
	// The authored canvas spot was (600, 300) with the loop centred at
	// (600, 200): relative to the frame the child must keep that offset —
	// horizontally centred on the loop, half a box below its centre.
	if p.X != loop.Size.Width/2 {
		t.Errorf("child x %d, want %d (authored on the loop's centre line)", p.X, loop.Size.Width/2)
	}
	wantY := loop.Size.Height/2 + 100 // authored 100px below the loop's centre
	if p.Y != wantY {
		t.Errorf("child y %d, want %d (authored 100px below the loop's centre)", p.Y, wantY)
	}
}

// The while form shares the machinery; moving it must not grow its box either.
func TestWhileBoxDoesNotGrowWithLoopPosition(t *testing.T) {
	small := loopOf1935(t, build1935(t, "  @position(600, 200)\n  while true\n  begin\n    @position(600, 300)\n    log info node 'x' 'a';\n    break;\n  end while;\n"))
	large := loopOf1935(t, build1935(t, "  @position(1200, 200)\n  while true\n  begin\n    @position(1200, 300)\n    log info node 'x' 'a';\n    break;\n  end while;\n"))
	if small.Size != large.Size {
		t.Errorf("moving the while 600px changed its box: %v vs %v", small.Size, large.Size)
	}
}

// Auto-placed bodies keep the box they always had: no @position anywhere, the
// statement count alone must still set the size (the #884 contract).
func TestLoopBoxAutoPlacedBodyUnchanged(t *testing.T) {
	loop := loopOf1935(t, build1935(t, "  loop $I in $L\n  begin\n    log info node 'x' 'a';\n  end loop;\n"))
	if loop.Size.Width < MinLoopWidth || loop.Size.Height < MinLoopHeight {
		t.Errorf("auto-placed loop box %v below the minimum %dx%d", loop.Size, MinLoopWidth, MinLoopHeight)
	}
	for _, child := range loop.ObjectCollection.Objects {
		p := child.GetPosition()
		if p.X == 0 && p.Y == 0 {
			continue
		}
		if p.X+ActivityWidth/2 > loop.Size.Width || p.Y+ActivityHeight/2 > loop.Size.Height {
			t.Errorf("auto-placed child at %v escapes its container %v", p, loop.Size)
		}
	}
}

// An explicit @size still wins over the computed box (the #2369 contract).
func TestLoopExplicitSizeStillWins(t *testing.T) {
	loop := loopOf1935(t, build1935(t, "  @position(600, 200)\n  @size(990, 270)\n  loop $I in $L\n  begin\n    @position(600, 300)\n    log info node 'x' 'a';\n  end loop;\n"))
	if loop.Size != (model.Size{Width: 990, Height: 270}) {
		t.Errorf("loop box %v, want the @size 990×270", loop.Size)
	}
}
