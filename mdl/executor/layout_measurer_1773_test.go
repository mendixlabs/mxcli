// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The measurer sizes a branch or a loop body before the builder places it, and
// whatever follows the body — an IF's closing merge, the box around a loop — is
// placed from that measurement. Where the two disagree an object lands on or
// behind its neighbour, and `layout flows` reports the flow as laid out, because
// it uses the same builder (ako/mxcli#1773).

// decisionShapes are the decision forms the builder lays out differently: with a
// merge, as a guard, and with one branch ending the flow and so no merge.
var decisionShapes = []struct{ name, body string }{
	{"an if/else followed by an activity", `
  if $Flag then
    log info node 'M' 'yes';
  else
    log info node 'M' 'no';
  end if;
  log info node 'M' 'next';`},
	{"an if without else followed by an activity", `
  if $Flag then
    log info node 'M' 'yes';
    log info node 'M' 'yes, again';
  end if;
  log info node 'M' 'next';`},
	{"a guard followed by an activity", `
  if $Flag then
    log info node 'M' 'refused';
    return;
  end if;
  log info node 'M' 'next';`},
	{"an if/else whose else ends the flow", `
  if $Flag then
    log info node 'M' 'yes';
    log info node 'M' 'yes, again';
  else
    log info node 'M' 'no';
    return;
  end if;
  log info node 'M' 'next';`},
	{"an if/else whose then ends the flow", `
  if $Flag then
    log info node 'M' 'refused';
    return;
  else
    log info node 'M' 'no';
    log info node 'M' 'no, again';
    log info node 'M' 'no, and again';
  end if;
  log info node 'M' 'next';`},
	{"two decisions back to back", `
  if $Flag then
    log info node 'M' 'yes';
  end if;
  if $Other then
    log info node 'M' 'other';
  else
    log info node 'M' 'not other';
  end if;
  log info node 'M' 'next';`},
	{"a decision that ends the body", `
  log info node 'M' 'first';
  if $Flag then
    log info node 'M' 'yes';
  else
    log info node 'M' 'no';
  end if;`},
}

func TestDecisionsNeverOverlapTheirNeighbours(t *testing.T) {
	for _, shape := range decisionShapes {
		for _, ctx := range allShapeContexts {
			t.Run(shape.name+" / "+ctx.name, func(t *testing.T) {
				assertNoSharedPlace(t, handlerShapeFlow(ctx.wrap(shape.body)), true)
			})
		}
	}
}

func TestDecisionsAreMeasuredAsBuilt(t *testing.T) {
	for _, shape := range decisionShapes {
		t.Run(shape.name, func(t *testing.T) { assertLoopMeasuredAsBuilt(t, shape.body) })
	}
}

// The reported shape: an if/else followed by an activity inside a THEN branch.
// The enclosing merge stood 5px inside that activity. It must keep the gap two
// activities keep, 40px edge to edge — the literal is the measurement, not the
// builder's constants.
func TestMergeAfterNestedDecisionClearsTheActivityBeforeIt(t *testing.T) {
	oc := buildLayout(t, handlerShapeFlow(`
  if $Other then
    if $Flag then
      log info node 'M' 'yes';
    else
      log info node 'M' 'no';
    end if;
    log info node 'M' 'last in then';
  else
    log info node 'M' 'else';
  end if;
  log info node 'M' 'after';`))

	// The outer merge is the right-most merge; the activity it follows is the
	// right-most activity left of it on its line.
	var merge *microflows.ExclusiveMerge
	for _, o := range oc.Objects {
		if m, ok := o.(*microflows.ExclusiveMerge); ok && (merge == nil || m.Position.X > merge.Position.X) {
			merge = m
		}
	}
	if merge == nil {
		t.Fatal("no merge")
	}
	lastRight := -1 << 31
	for _, o := range oc.Objects {
		a, ok := o.(*microflows.ActionActivity)
		if !ok || a.Position.Y != merge.Position.Y || a.Position.X > merge.Position.X {
			continue
		}
		lastRight = max(lastRight, a.Position.X+a.Size.Width/2)
	}
	if gap := merge.Position.X - MergeSize/2 - lastRight; gap != 40 {
		t.Errorf("closing merge at x=%d: %dpx from the activity before it, want 40", merge.Position.X, gap)
	}
}

// Shape 2 in isolation: a join draws nothing, so a handler row of one activity
// and a join is one activity wide.
func TestJoinIsMeasuredAsNothing(t *testing.T) {
	m := &layoutMeasurer{}
	if w := m.measureStatement(&ast.JoinStmt{Label: "recovered"}).Width; w != 0 {
		t.Errorf("a join measured %dpx wide, it draws nothing", w)
	}
	row := []ast.MicroflowStatement{&ast.MfCommitStmt{}, &ast.JoinStmt{Label: "recovered"}}
	if w := m.measureBranch(row).Width; w != 120 {
		t.Errorf("an activity and a join measured %dpx, want 120 (the activity alone)", w)
	}
}

// Shape 3: a handler of two or more statements that falls through rejoins the
// main line at a merge. That merge stood between the guarded activity and the next
// one, left of the row's last activity, so the edge into it ran backwards. It goes
// one gap past the row instead, and the next activity one gap past the merge.
func TestRejoiningHandlerMergesPastItsRow(t *testing.T) {
	oc := buildLayout(t, handlerShapeFlow(`
  retrieve $A from M.Item on error without rollback begin
    log error node 'M' 'a1';
    log error node 'M' 'a2';
    log error node 'M' 'a3';
  end error;
  log info node 'M' 'next';`))
	got := map[string]bool{}
	for _, o := range oc.Objects {
		got[canvasLabel(o)] = true
	}
	for _, want := range []string{
		"ActionActivity 360;200", // retrieve $A
		"ActionActivity 360;330", // a1, straight under it
		"ActionActivity 520;330", // a2
		"ActionActivity 680;330", // a3: the row ends at 740
		"ExclusiveMerge 800;200", // 40px past the row
		"ActionActivity 920;200", // next: 40px past the merge
	} {
		if !got[want] {
			t.Errorf("missing %s", want)
		}
	}
}
