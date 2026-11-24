// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// upstream #884. The implicit merge that closes a split was placed by the layout
// pass and was unaddressable — the statement's own @position belongs to the
// SPLIT — so end-if joins routinely landed on top of a neighbouring activity and
// there was no way to move them.
//
// @merge(x, y) positions it, and DESCRIBE emits it so the position survives a
// rewrite. Emitting is the half that makes it real: without it the layout pass
// recomputes on the next exec and puts the merge straight back.

func TestMergePositionHonoursTheAnnotation(t *testing.T) {
	if x, y := mergePosition(nil, 700, 200); x != 700 || y != 200 {
		t.Errorf("no annotations = (%d, %d), want the computed (700, 200)", x, y)
	}
	if x, y := mergePosition(&ast.ActivityAnnotations{}, 700, 200); x != 700 || y != 200 {
		t.Errorf("no @merge = (%d, %d), want the computed (700, 200)", x, y)
	}
	ann := &ast.ActivityAnnotations{Merge: &ast.Position{X: 900, Y: 400}}
	if x, y := mergePosition(ann, 700, 200); x != 900 || y != 400 {
		t.Errorf("@merge(900, 400) = (%d, %d), want (900, 400)", x, y)
	}
}

// The wiring: MDL text through the real builder must put the merge where asked.
// Calling mergePosition directly would pass even with every call site removed.
func TestMergeReachesTheCanvasThroughTheBuilder(t *testing.T) {
	src := "create microflow M.F ($P: String)\nreturns String as $R\nbegin\n" +
		"  @position(200, 200)\n  @merge(900, 400)\n  if $P = 'a' then\n" +
		"    @position(400, 120)\n    declare $X String = 'x';\n  else\n" +
		"    @position(400, 280)\n    declare $Y String = 'y';\n  end if;\n" +
		"  declare $R String = $P;\n  return $R;\nend;\n"
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
	coll := fb.buildFlowGraph(mf.Body, mf.ReturnType)

	var merges int
	for _, o := range coll.Objects {
		m, ok := o.(*microflows.ExclusiveMerge)
		if !ok {
			continue
		}
		merges++
		p := m.GetPosition()
		if p.X != 900 || p.Y != 400 {
			t.Errorf("merge at (%d, %d), want (900, 400) — @merge reached the AST but the "+
				"builder site is not reading through mergePosition", p.X, p.Y)
		}
	}
	if merges != 1 {
		t.Fatalf("found %d merges, want 1", merges)
	}
}

// commonMergeAfter locates a split's merge from the flow graph, because the
// describer's split→merge map is not in scope where annotations are emitted.
func TestCommonMergeAfter(t *testing.T) {
	split := &microflows.ExclusiveSplit{}
	split.ID = model.ID("split")
	thenAct := &microflows.ActionActivity{}
	thenAct.ID = model.ID("then")
	elseAct := &microflows.ActionActivity{}
	elseAct.ID = model.ID("else")
	merge := &microflows.ExclusiveMerge{}
	merge.ID = model.ID("merge")
	merge.Position = model.Point{X: 900, Y: 400}

	activityMap := map[model.ID]microflows.MicroflowObject{
		split.ID: split, thenAct.ID: thenAct, elseAct.ID: elseAct, merge.ID: merge,
	}
	flows := map[model.ID][]*microflows.SequenceFlow{
		split.ID:   {{OriginID: split.ID, DestinationID: thenAct.ID}, {OriginID: split.ID, DestinationID: elseAct.ID}},
		thenAct.ID: {{OriginID: thenAct.ID, DestinationID: merge.ID}},
		elseAct.ID: {{OriginID: elseAct.ID, DestinationID: merge.ID}},
	}

	got := commonMergeAfter(split.ID, flows, activityMap)
	if got == nil {
		t.Fatal("no merge found for a split whose branches both reach one")
	}
	if got.GetID() != merge.ID {
		t.Errorf("found %s, want %s", got.GetID(), merge.ID)
	}

	// A branch that returns instead of rejoining: no common merge, no annotation.
	flows[elseAct.ID] = nil
	if got := commonMergeAfter(split.ID, flows, activityMap); got != nil {
		t.Errorf("found %s, want nil — one branch does not reach the merge, so it does not "+
			"close this split", got.GetID())
	}
}

// A retry loop makes the flow graph cyclic; the walk must terminate.
func TestCommonMergeAfterTerminatesOnACycle(t *testing.T) {
	split := &microflows.ExclusiveSplit{}
	split.ID = model.ID("split")
	a := &microflows.ActionActivity{}
	a.ID = model.ID("a")
	b := &microflows.ActionActivity{}
	b.ID = model.ID("b")

	activityMap := map[model.ID]microflows.MicroflowObject{split.ID: split, a.ID: a, b.ID: b}
	flows := map[model.ID][]*microflows.SequenceFlow{
		split.ID: {{OriginID: split.ID, DestinationID: a.ID}, {OriginID: split.ID, DestinationID: b.ID}},
		a.ID:     {{OriginID: a.ID, DestinationID: b.ID}},
		b.ID:     {{OriginID: b.ID, DestinationID: a.ID}}, // cycle
	}
	done := make(chan microflows.MicroflowObject, 1)
	go func() { done <- commonMergeAfter(split.ID, flows, activityMap) }()
	select {
	case got := <-done:
		if got != nil {
			t.Errorf("found %v, want nil — there is no merge in this graph", got.GetID())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("commonMergeAfter did not terminate on a cyclic flow graph")
	}
}

// The describe wiring: through emitObjectAnnotations, not the emitter directly.
func TestMergeIsEmittedByTheAnnotationEmitter(t *testing.T) {
	split := &microflows.ExclusiveSplit{}
	split.ID = model.ID("split")
	split.Position = model.Point{X: 200, Y: 200}
	thenAct := &microflows.ActionActivity{}
	thenAct.ID = model.ID("then")
	elseAct := &microflows.ActionActivity{}
	elseAct.ID = model.ID("else")
	merge := &microflows.ExclusiveMerge{}
	merge.ID = model.ID("merge")
	merge.Position = model.Point{X: 900, Y: 400}

	activityMap := map[model.ID]microflows.MicroflowObject{
		split.ID: split, thenAct.ID: thenAct, elseAct.ID: elseAct, merge.ID: merge,
	}
	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		split.ID:   {{OriginID: split.ID, DestinationID: thenAct.ID}, {OriginID: split.ID, DestinationID: elseAct.ID}},
		thenAct.ID: {{OriginID: thenAct.ID, DestinationID: merge.ID}},
		elseAct.ID: {{OriginID: elseAct.ID, DestinationID: merge.ID}},
	}

	var lines []string
	emitObjectAnnotations(split, &lines, "", nil, flowsByOrigin,
		map[model.ID][]*microflows.SequenceFlow{}, activityMap)

	var found bool
	for _, l := range lines {
		if l == "@merge(900, 400)" {
			found = true
		}
	}
	if !found {
		t.Errorf("emitObjectAnnotations produced %v — without the @merge line the position is "+
			"recomputed on the next exec and the merge goes back on top of its neighbour", lines)
	}
}

// A plain activity is not a split and must not get a @merge line.
func TestMergeIsNotEmittedForANonSplit(t *testing.T) {
	act := &microflows.ActionActivity{}
	act.ID = model.ID("a")
	var lines []string
	emitMergeAnnotation(act, map[model.ID][]*microflows.SequenceFlow{}, map[model.ID]microflows.MicroflowObject{}, nil, nil, &lines, "")
	if len(lines) != 0 {
		t.Errorf("emitted %v for a plain activity, want nothing", lines)
	}
}
