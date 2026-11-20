// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A custom error handler is drawn as a row under its activity, ErrorHandlerPitch
// below the main line. The main line does not wait for that row — the statement
// after the activity is one pitch further on, however long the row is — so when
// that next statement had a handler too, its row started inside the first one
// (ako/mxcli#1768, mendixlabs/mxcli#1672). Reported, from `describe … with
// handles`: the first handler's end event and the second handler's first
// activity both on 680;330.
const twoHandlersFlow = `create microflow MyModule.SUB_TwoHandlers ($Url: String) returns MyModule.Result as $Result
begin
  $First = call rest service get '{1}' with ({1} = $Url) (Timeout: 30) returns response on error without rollback begin
    $FirstFailed = create MyModule.Result (Code = 1);
    log error node 'MyModule' 'first failed';
    return $FirstFailed;
  end error;
  declare $Body String = $First/Content;
  $Second = call rest service get '{1}' with ({1} = $Body) (Timeout: 30) returns response on error without rollback begin
    $SecondFailed = create MyModule.Result (Code = 2);
    log error node 'MyModule' 'second failed';
    return $SecondFailed;
  end error;
  $Done = create MyModule.Result (IsDone = true);
  return $Done;
end;`

// canvasBoxOf is an object's box on its canvas: a Mendix position is the centre.
func canvasBoxOf(o microflows.MicroflowObject) canvasBox {
	p, sz := o.GetPosition(), objectSize(o)
	return canvasBox{p.X - sz.Width/2, p.Y - sz.Height/2, p.X + sz.Width/2, p.Y + sz.Height/2}
}

func canvasLabel(o microflows.MicroflowObject) string {
	p := o.GetPosition()
	return fmt.Sprintf("%s %d;%d", typeShort(o), p.X, p.Y)
}

// canvasOverlaps lists every pair of objects that share space on the same canvas.
// A loop's children are positioned in the loop's own frame, so each loop body is
// its own canvas, checked separately; notes are left out.
func canvasOverlaps(objects []microflows.MicroflowObject) []string {
	var out []string
	var plane []microflows.MicroflowObject
	for _, o := range objects {
		if _, ok := o.(*microflows.Annotation); ok {
			continue
		}
		plane = append(plane, o)
		if loop, ok := o.(*microflows.LoopedActivity); ok && loop.ObjectCollection != nil {
			for _, inner := range canvasOverlaps(loop.ObjectCollection.Objects) {
				out = append(out, "in loop: "+inner)
			}
		}
	}
	for i := range plane {
		for j := i + 1; j < len(plane); j++ {
			a, b := canvasBoxOf(plane[i]), canvasBoxOf(plane[j])
			if a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom {
				out = append(out, canvasLabel(plane[i])+" overlaps "+canvasLabel(plane[j]))
			}
		}
	}
	return out
}

func TestSecondErrorHandlerRowClearsTheFirst(t *testing.T) {
	oc := buildLayout(t, twoHandlersFlow)
	if overlaps := canvasOverlaps(oc.Objects); len(overlaps) > 0 {
		t.Errorf("objects share a place:\n  %s", strings.Join(overlaps, "\n  "))
	}

	got := map[string]bool{}
	for _, o := range oc.Objects {
		got[canvasLabel(o)] = true
	}
	// Everything up to the second handler stays where it was: the first activity,
	// its row, and the plain activity after it, which does not reach into the lane
	// and so does not wait for it.
	for _, want := range []string{
		"ActionActivity 360;200", // $First
		"ActionActivity 360;330", // $FirstFailed
		"ActionActivity 520;330", // log error 'first failed'
		"EndEvent 680;330",       // return $FirstFailed
		"ActionActivity 520;200", // $Body
	} {
		if !got[want] {
			t.Errorf("missing %s — a position that did not overlap anything moved", want)
		}
	}
	// $Second reaches into the lane with its own row, so it waits for the first row
	// to clear: that row ends at 690 (the end event's right edge), plus the 40px gap
	// two activities keep, plus half an activity. The literal is the measurement,
	// not the builder's constants, so it cannot agree with whatever they say.
	for _, want := range []string{
		"ActionActivity 790;200", // $Second
		"ActionActivity 790;330", // $SecondFailed, straight under its activity
	} {
		if !got[want] {
			t.Errorf("missing %s", want)
		}
	}
}

// handlerRuns are runs of activities whose error handler rows share a lane. The
// builder places a handler row the same way wherever the run is — on the main
// line, in a loop body, in either branch of a decision — so each is checked in
// all of them.
var handlerRuns = []struct {
	name, body string
}{
	{name: "two handlers with a plain activity between", body: `
  retrieve $A from M.Item on error without rollback begin
    log error node 'M' 'a1';
    log error node 'M' 'a2';
    return;
  end error;
  log info node 'M' 'between';
  retrieve $B from M.Item on error without rollback begin
    log error node 'M' 'b1';
    log error node 'M' 'b2';
    return;
  end error;
  log info node 'M' 'done';`},
	{name: "the first handler longer", body: `
  retrieve $A from M.Item on error without rollback begin
    log error node 'M' 'a1';
    log error node 'M' 'a2';
    log error node 'M' 'a3';
    return;
  end error;
  log info node 'M' 'between';
  retrieve $B from M.Item on error without rollback begin
    log error node 'M' 'b1';
    log error node 'M' 'b2';
    return;
  end error;
  log info node 'M' 'done';`},
	{name: "three handlers back to back", body: `
  retrieve $A from M.Item on error without rollback begin
    log error node 'M' 'a1';
    return;
  end error;
  retrieve $B from M.Item on error without rollback begin
    log error node 'M' 'b1';
    log error node 'M' 'b2';
    return;
  end error;
  retrieve $C from M.Item on error without rollback begin
    log error node 'M' 'c1';
    return;
  end error;
  log info node 'M' 'done';`},
	// ako/mxcli#1773, shape 2: a join draws nothing, so a row that ends in one
	// is as wide as its activities. It was measured as an activity plus an end
	// event, and reserved the lane for about twice its drawn width. `check`
	// refuses a join inside a loop (MDL-FLOW04), but the builder measures and
	// places a run the same way wherever it is, and the box it fits around a loop
	// body is the exact oracle for the run's width.
	{name: "handlers that end in joins", body: `
  retrieve $A from M.Item on error without rollback begin
    log error node 'M' 'a1';
    join recovered;
  end error;
  retrieve $B from M.Item on error without rollback begin
    log error node 'M' 'b1';
    log error node 'M' 'b2';
    join recovered;
  end error;
  log info node 'M' 'between';
  merge recovered;
  log info node 'M' 'done';`},
	// ako/mxcli#1773, shape 3: a row of three that falls through.
	{name: "a long handler that rejoins", body: `
  retrieve $A from M.Item on error without rollback begin
    log error node 'M' 'a1';
    log error node 'M' 'a2';
    log error node 'M' 'a3';
  end error;
  log info node 'M' 'next';
  log info node 'M' 'done';`},
	{name: "handlers that rejoin the main line", body: `
  retrieve $A from M.Item on error without rollback begin
    log error node 'M' 'a1';
    log error node 'M' 'a2';
  end error;
  retrieve $B from M.Item on error without rollback begin
    log error node 'M' 'b1';
    log error node 'M' 'b2';
  end error;
  log info node 'M' 'done';`},
}

// lanePartners put a handler row next to the lane's other occupants: a guard's
// branch, a decision's ELSE row, a handler row inside a decision's branch.
//
// They run in every context the handler runs do. Inside a branch the enclosing
// merge is placed from the measured width of the decision, which used to count
// the split's x as its left edge and so came out 45px short: `if … else … end
// if; log …` in a THEN branch put the closing merge 5px into that log
// (ako/mxcli#1773). Until that was fixed these ran on the main line and in loop
// bodies only.
var lanePartners = []struct{ name, body string }{
	{"a guard after a handler", `
  retrieve $A from M.Item on error without rollback begin
    log error node 'M' 'a1';
    log error node 'M' 'a2';
    return;
  end error;
  if $Flag then
    log info node 'M' 'refused';
    return;
  end if;
  log info node 'M' 'done';`},
	{"a decision after a handler", `
  retrieve $A from M.Item on error without rollback begin
    log error node 'M' 'a1';
    log error node 'M' 'a2';
    return;
  end error;
  if $Flag then
    log info node 'M' 'yes';
  else
    log info node 'M' 'no';
  end if;
  log info node 'M' 'done';`},
	{"a handler after a guard", `
  if $Flag then
    log info node 'M' 'refused';
    log info node 'M' 'refused, details';
    return;
  end if;
  retrieve $B from M.Item on error without rollback begin
    log error node 'M' 'b1';
    return;
  end error;
  log info node 'M' 'done';`},
	{"a handler row in a branch, a handler after the decision", `
  if $Flag then
    retrieve $A from M.Item on error without rollback begin
      log error node 'M' 'a1';
      log error node 'M' 'a2';
      log error node 'M' 'a3';
      return;
    end error;
  else
    log info node 'M' 'no';
  end if;
  retrieve $B from M.Item on error without rollback begin
    log error node 'M' 'b1';
    return;
  end error;
  log info node 'M' 'done';`},
}

type shapeContext struct {
	name string
	wrap func(body string) string
}

var (
	onMainLine = shapeContext{"main line", func(b string) string { return b }}
	inLoopBody = shapeContext{"loop body", func(b string) string {
		return "loop $It in $Items begin\n" + b + "\nend loop;\nlog info node 'M' 'after';"
	}}
	inThenBranch = shapeContext{"then branch", func(b string) string {
		return "if $Other then\n" + b + "\nelse\n  log info node 'M' 'else';\nend if;\nlog info node 'M' 'after';"
	}}
	inElseBranch = shapeContext{"else branch", func(b string) string {
		return "if $Other then\n  log info node 'M' 'then';\nelse\n" + b + "\nend if;\nlog info node 'M' 'after';"
	}}
	inThenWithoutElse = shapeContext{"then branch without else", func(b string) string {
		return "if $Other then\n" + b + "\nend if;\nlog info node 'M' 'after';"
	}}
)

// allShapeContexts is every place the builder puts a statement run.
var allShapeContexts = []shapeContext{onMainLine, inLoopBody, inThenBranch, inElseBranch, inThenWithoutElse}

func handlerShapeFlow(body string) string {
	return "create microflow M.F ($Flag: Boolean, $Other: Boolean, $Items: list of M.Item) begin\n" + body + "\nend;"
}

// assertNoSharedPlace builds a flow and checks that no two of its objects share a
// place on any canvas. The builder places each statement and the measurer sizes
// the branch or loop it sits in; when the two disagree about where a statement
// went, a merge lands on it, which this catches too.
func assertNoSharedPlace(t *testing.T, src string, checkMergeDirection bool) {
	t.Helper()
	oc := buildLayout(t, src)
	if overlaps := canvasOverlaps(oc.Objects); len(overlaps) > 0 {
		t.Errorf("objects share a place:\n  %s", strings.Join(overlaps, "\n  "))
	}
	if checkMergeDirection {
		assertLayoutInvariants(t, t.Name(), oc)
	}
}

func TestErrorHandlerRowsNeverOverlap(t *testing.T) {
	for _, run := range handlerRuns {
		for _, ctx := range allShapeContexts {
			t.Run(run.name+" / "+ctx.name, func(t *testing.T) {
				// A handler that falls through used to rejoin at a merge placed
				// between its activity and the next one, left of a row of two or
				// more, so the edge into it ran backwards; this skipped the
				// direction check for it until ako/mxcli#1773.
				assertNoSharedPlace(t, handlerShapeFlow(ctx.wrap(run.body)), true)
			})
		}
	}
	for _, shape := range lanePartners {
		for _, ctx := range allShapeContexts {
			t.Run(shape.name+" / "+ctx.name, func(t *testing.T) {
				assertNoSharedPlace(t, handlerShapeFlow(ctx.wrap(shape.body)), true)
			})
		}
	}
}

// The measurer sizes a container before its body exists; the builder sizes the
// box from what it built, handler rows included. A loop around each run must come
// out exactly as wide as it was measured, or an IF holding the loop places its
// merge inside the box (the mendixlabs/mxcli#1598 class) — and once a handler row
// makes the next one wait, the measurer has to know about the wait as well as the
// builder.
func TestLoopAroundHandlerRowsIsMeasuredAsBuilt(t *testing.T) {
	for _, run := range handlerRuns {
		t.Run(run.name, func(t *testing.T) { assertLoopMeasuredAsBuilt(t, run.body) })
	}
	// The lane partners hold decisions, which the measurer used to measure 45px
	// short (ako/mxcli#1773).
	for _, shape := range lanePartners {
		t.Run(shape.name, func(t *testing.T) { assertLoopMeasuredAsBuilt(t, shape.body) })
	}
}

// assertLoopMeasuredAsBuilt wraps body in a loop and compares the loop's measured
// width with the width the builder gives the box it draws around what it built.
func assertLoopMeasuredAsBuilt(t *testing.T, body string) {
	t.Helper()
	prog, errs := visitor.Build(handlerShapeFlow("loop $It in $Items begin\n" + body + "\nend loop;"))
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	var loopStmt *ast.LoopStmt
	for _, s := range mf.Body {
		if l, ok := s.(*ast.LoopStmt); ok {
			loopStmt = l
		}
	}
	measured := (&layoutMeasurer{}).measureStatement(loopStmt)

	fb := &flowBuilder{posX: 200, posY: 200, spacing: HorizontalSpacing}
	oc := fb.buildFlowGraph(mf.Body, mf.ReturnType)
	var built model.Size
	for _, o := range oc.Objects {
		if l, ok := o.(*microflows.LoopedActivity); ok {
			built = l.Size
		}
	}
	if built.Width != measured.Width {
		t.Errorf("loop built %dpx wide, measured %dpx", built.Width, measured.Width)
	}
}
