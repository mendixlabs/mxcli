// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// Issue #723 §B — auto-layout invariants.
//
// The report described four placement defects for @position-less microflows:
// parameters overlapping the flow, the start event too far left, an exclusive
// merge landing to the RIGHT of the node it feeds (a backward, right-to-left
// connector), and the single end event not being the right-most object.
//
// Against the current builder these do not occur — the branch/merge machinery
// places merges after their branches and the end event last. These tests lock
// the two structurally-checkable invariants so the defects can't silently
// regress:
//
//	(1) the terminal EndEvent is the right-most object;
//	(2) every top-level sequence flow into an ExclusiveMerge is forward
//	    (origin.X <= merge.X) — i.e. no backward merge-in connector.
//
// Layout has no `mx check` oracle, so these invariants are the guard.

// buildLayout parses a single-microflow script and returns its top-level flow
// graph (loop-body objects live in nested collections and are not returned).
func buildLayout(t *testing.T, src string) *microflows.MicroflowObjectCollection {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var mf *ast.CreateMicroflowStmt
	for _, s := range prog.Statements {
		if m, ok := s.(*ast.CreateMicroflowStmt); ok {
			mf = m
			break
		}
	}
	if mf == nil {
		t.Fatal("no microflow in script")
	}
	// Mirror the real create-path start position (cmd_microflows_create.go).
	fb := &flowBuilder{posX: 200, posY: 200, spacing: HorizontalSpacing}
	return fb.buildFlowGraph(mf.Body, mf.ReturnType)
}

func layoutObjectWidth(o microflows.MicroflowObject) int {
	switch v := o.(type) {
	case *microflows.StartEvent:
		return v.Size.Width
	case *microflows.EndEvent:
		return v.Size.Width
	case *microflows.ExclusiveSplit:
		return v.Size.Width
	case *microflows.ExclusiveMerge:
		return v.Size.Width
	case *microflows.ActionActivity:
		return v.Size.Width
	default:
		return ActivityWidth
	}
}

// assertLayoutInvariants checks §B invariants (1) and (2) on a flow graph.
func assertLayoutInvariants(t *testing.T, name string, oc *microflows.MicroflowObjectCollection) {
	t.Helper()
	posX := map[string]int{}
	merges := map[string]bool{}
	var endID string
	endCount := 0
	rightMostID, rightMost := "", -1<<31
	for _, o := range oc.Objects {
		id := string(o.GetID())
		x := o.GetPosition().X
		posX[id] = x
		if right := x + layoutObjectWidth(o)/2; right > rightMost {
			rightMost, rightMostID = right, id
		}
		switch o.(type) {
		case *microflows.EndEvent:
			endID = id
			endCount++
		case *microflows.ExclusiveMerge:
			merges[id] = true
		}
	}

	// (1) The single terminal EndEvent is the right-most object.
	if endCount == 1 && rightMostID != endID {
		t.Errorf("%s: EndEvent is not the right-most object (right-most is %s at X=%d) — §B 'end event not right-most'",
			name, describeLayoutObj(oc, rightMostID), rightMost)
	}

	// (2) Every top-level flow into a merge is forward (no backward connector).
	for _, f := range oc.Flows {
		if !merges[string(f.DestinationID)] {
			continue
		}
		ox, ook := posX[string(f.OriginID)]
		mx, mok := posX[string(f.DestinationID)]
		if !ook || !mok {
			continue // loop-internal endpoint not in the top-level collection
		}
		if ox > mx {
			t.Errorf("%s: backward merge-in connector: origin X=%d > merge X=%d — §B 'merge lands right of the node it feeds'",
				name, ox, mx)
		}
	}
}

func describeLayoutObj(oc *microflows.MicroflowObjectCollection, id string) string {
	for _, o := range oc.Objects {
		if string(o.GetID()) == id {
			return typeShort(o)
		}
	}
	return id
}

func typeShort(o microflows.MicroflowObject) string {
	switch o.(type) {
	case *microflows.StartEvent:
		return "StartEvent"
	case *microflows.EndEvent:
		return "EndEvent"
	case *microflows.ExclusiveSplit:
		return "ExclusiveSplit"
	case *microflows.ExclusiveMerge:
		return "ExclusiveMerge"
	case *microflows.LoopedActivity:
		return "LoopedActivity"
	default:
		return "ActionActivity"
	}
}

func TestLayoutInvariants_NoBackwardMergeAndEndRightmost(t *testing.T) {
	cases := []struct{ name, src string }{
		{"seq+if/else+tail", `create microflow M.T ($In: M.Order) begin
retrieve $c from M.Customer;
if $c/Active then
  log 'yes';
else
  log 'no';
end if;
log 'done';
end;`},
		{"if/else long-else", `create microflow M.LE ($In: M.Order) begin
if $In/Active then
  log 'a';
else
  log 'b1';
  log 'b2';
  log 'b3';
end if;
log 'tail';
end;`},
		{"if-no-else guard", `create microflow M.G ($In: M.Order) begin
if $In/Active then
  log 'a';
end if;
log 'after';
end;`},
		{"nested-if in then", `create microflow M.N ($In: M.Order) begin
if $In/Active then
  if $In/Ready then
    log 'x';
  else
    log 'y';
  end if;
else
  log 'z';
end if;
log 'tail';
end;`},
		{"loop in else", `create microflow M.LE2 ($In: M.Order, $L: list of M.Order) begin
if $In/Active then
  log 'a';
else
  loop $x in $L
  begin
    log 'inner1';
    log 'inner2';
  end loop;
end if;
log 'tail';
end;`},
	}
	for _, c := range cases {
		oc := buildLayout(t, c.src)
		assertLayoutInvariants(t, c.name, oc)
	}
}
