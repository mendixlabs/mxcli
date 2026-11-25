// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2564: a loop body's @position is in the loop box's coordinates,
// and the box is sized to reach it. A script that then moved the body — to fix
// a box drawn too big, say — could not: `create or modify` refused a node
// moved inside a loop body (MDL-V1-REBUILD), and when the moved activity was
// also spelled another way than describe prints it, the in-place replace drew
// it where it was stored and reported the flow unchanged. The bad box stuck.
//
// The splice now moves a body node to where the script draws it, and a box the
// script gives no @size is fitted to the moved body the way `create` sizes it,
// so the flow reads as a fresh create of the same script would.

const mv2564Flow = `mdl 1;
create or modify microflow MyFirstModule.%s ($Items: List of FeedbackModule.Feedback)
begin
  @start(60, 200)
  @position(600, 200)
  loop $It in $Items
  begin
    @position(%d, %d)
    %s
  end loop;
  @position(1300, 200)
  return;
end;`

func TestSpliceLoopBodyMove2564(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	// The reporter's script: the body drawn at the canvas point under the
	// loop, which in the box's coordinates is 600px in, so the box is 710 wide.
	const described = "log node 'X' '{1}' with ({1} = $It/Subject);"
	for _, tc := range []struct{ name, stmt string }{
		{"MV2564Described", described},
		// Spelled otherwise than describe prints it: replaced in place.
		{"MV2564Spelled", "log info node 'X' $It/Subject;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := agreeExec(t, exec, fmt.Sprintf(mv2564Flow, tc.name, 600, 300, tc.stmt)); err != nil {
				t.Fatalf("setup: %v\n%s", err, out.String())
			}
			before := loopOf(t, loopEditFlowOf(t, exec, tc.name))
			if before.Size.Width != 710 {
				t.Fatalf("setup: the box is %v, want 710 wide", before.Size)
			}

			moved := fmt.Sprintf(mv2564Flow, tc.name, 150, 100, tc.stmt)
			if err := agreeExec(t, exec, moved); err != nil {
				t.Fatalf("moving the body node is refused: %v\n%s", err, out.String())
			}
			mf := loopEditFlowOf(t, exec, tc.name)
			loop := loopOf(t, mf)
			if loop.ID != before.ID {
				t.Error("the loop was rebuilt; only its body node moves")
			}
			var at model.Point
			for _, o := range loop.ObjectCollection.Objects {
				at = o.GetPosition()
			}
			if at != (model.Point{X: 150, Y: 100}) {
				t.Errorf("the body node is at %v, want (150, 100)", at)
			}

			// The box a fresh create of the moved script draws.
			fresh := tc.name + "Fresh"
			if err := agreeExec(t, exec, strings.Replace(moved, tc.name, fresh, 1)); err != nil {
				t.Fatal(err)
			}
			want := loopOf(t, loopEditFlowOf(t, exec, fresh))
			if loop.Size != want.Size || loop.Position != want.Position {
				t.Errorf("the box is %v at %v; a create of the same script draws %v at %v", loop.Size, loop.Position, want.Size, want.Position)
			}
			assertLoopBodies(t, mf)

			out.Reset()
			if err := agreeExec(t, exec, moved); err != nil {
				t.Fatal(err)
			}
			if s := out.String(); strings.Contains(s, "Modified") {
				t.Errorf("a re-run writes again:\n%s", s)
			}
		})
	}
}

// `mxcli layout flows` reported a loop moved and left its box at the stored
// size: the rebuild it lays out from kept describe's @size (ako/mxcli#2369),
// and the write carried the stored size back as a rebuild's is. It now refits
// the box to the laid-out body, and a second run has nothing to change.
func TestLayoutFlowsRefitsLoopBox2564(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const name = "MV2564Layout"
	if err := agreeExec(t, exec, fmt.Sprintf(mv2564Flow, name, 600, 300, "log node 'X' 'a';")); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	qn := ast.QualifiedName{Module: "MyFirstModule", Name: name}
	res, err := exec.LayoutFlow("microflow", qn, false)
	if err != nil || res.Refused != "" {
		t.Fatalf("layout: %v %s", err, res.Refused)
	}
	mf := loopEditFlowOf(t, exec, name)
	loop := loopOf(t, mf)
	var body model.Point
	for _, o := range loop.ObjectCollection.Objects {
		body = o.GetPosition()
	}
	// The body's right and bottom edges plus the padding.
	want := model.Size{Width: max(body.X+ActivityWidth/2+LoopPadding, MinLoopWidth), Height: max(body.Y+ActivityHeight/2+LoopPadding, MinLoopHeight)}
	if loop.Size != want {
		t.Errorf("after layout the box is %v around a body at %v, want %v", loop.Size, body, want)
	}
	assertLoopBodies(t, mf)
	again, err := exec.LayoutFlow("microflow", qn, true)
	if err != nil || again.Changed() {
		t.Errorf("a second layout would change the flow again: %+v %v", again, err)
	}
}
