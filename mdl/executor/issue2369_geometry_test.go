// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2369: drop + create from DESCRIBE's own output resized Studio Pro's
// merges (90×60 → 40×40) and loop boxes, moved the merge of a type split with a
// returning case, and doubled a merge shared by an `if` and the `if` nested in
// it. Each test builds real MDL through the parser and the builder.

func build2369(t *testing.T, body string) *microflows.MicroflowObjectCollection {
	t.Helper()
	src := "create microflow M.F ($P: String, $L: List of M.E)\nbegin\n" + body + "end;\n"
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

func merges2369(coll *microflows.MicroflowObjectCollection) []*microflows.ExclusiveMerge {
	var out []*microflows.ExclusiveMerge
	for _, o := range coll.Objects {
		if m, ok := o.(*microflows.ExclusiveMerge); ok {
			out = append(out, m)
		}
	}
	return out
}

func TestIfMergeKeepsItsStatedSize(t *testing.T) {
	coll := build2369(t, "  @merge(900, 400, 90, 60)\n  if $P = 'a' then\n"+
		"    log info node 'x' 'a';\n  else\n    log info node 'x' 'b';\n  end if;\n"+
		"  log info node 'x' 'c';\n")
	ms := merges2369(coll)
	if len(ms) != 1 {
		t.Fatalf("found %d merges, want 1", len(ms))
	}
	if ms[0].Size != (model.Size{Width: 90, Height: 60}) || ms[0].Position != (model.Point{X: 900, Y: 400}) {
		t.Errorf("merge %v at %v, want 90×60 at (900, 400)", ms[0].Size, ms[0].Position)
	}
}

func TestLoopKeepsItsStatedSize(t *testing.T) {
	coll := build2369(t, "  @position(400, 200)\n  @size(990, 270)\n  loop $I in $L\n"+
		"  begin\n    log info node 'x' 'a';\n  end loop;\n")
	for _, o := range coll.Objects {
		if l, ok := o.(*microflows.LoopedActivity); ok {
			if l.Size != (model.Size{Width: 990, Height: 270}) {
				t.Errorf("loop box %v, want 990×270 — @size must override the computed box", l.Size)
			}
			return
		}
	}
	t.Fatal("no loop built")
}

func TestLabelledMergeKeepsItsStatedSize(t *testing.T) {
	coll := build2369(t, "  if $P = 'a' then\n    join m1;\n  else\n    join m1;\n  end if;\n"+
		"  @position(700, 200)\n  @size(70, 50)\n  merge m1;\n  log info node 'x' 'c';\n")
	for _, m := range merges2369(coll) {
		if m.Position == (model.Point{X: 700, Y: 200}) {
			if m.Size != (model.Size{Width: 70, Height: 50}) {
				t.Errorf("merge m1 is %v, want 70×50", m.Size)
			}
			return
		}
	}
	t.Fatal("no merge at (700, 200)")
}

func TestWhileTrueHeadMergeKeepsItsStatedSize(t *testing.T) {
	body := []ast.MicroflowStatement{&ast.WhileStmt{
		Condition:   &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
		Annotations: &ast.ActivityAnnotations{Size: &ast.BoxSize{Width: 90, Height: 60}},
		Body: []ast.MicroflowStatement{
			&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "retry"}},
			&ast.ContinueStmt{},
		},
	}}
	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, measurer: &layoutMeasurer{}}
	ms := merges2369(fb.buildFlowGraph(body, nil))
	if len(ms) != 1 || ms[0].Size != (model.Size{Width: 90, Height: 60}) {
		t.Fatalf("loop-head merges %v, want one of 90×60", ms)
	}
}

// One merge closing both an `if` and the `if` nested in its branch: DESCRIBE
// writes the same @merge on both, which must build one merge, not two stacked.
func TestSharedMergeOfNestedIfsIsBuiltOnce(t *testing.T) {
	coll := build2369(t, "  @merge(900, 200, 90, 60)\n  if $P != empty then\n"+
		"    @merge(900, 200, 90, 60)\n    if $P = 'a' then\n      log info node 'x' 'a';\n"+
		"    else\n      log info node 'x' 'b';\n    end if;\n  else\n"+
		"    log info node 'x' 'c';\n  end if;\n  log info node 'x' 'd';\n")
	ms := merges2369(coll)
	if len(ms) != 1 {
		t.Fatalf("found %d merges, want 1 — coincident merges must be folded into one", len(ms))
	}
	var in int
	for _, f := range coll.Flows {
		if f.DestinationID == ms[0].ID {
			in++
		}
	}
	if in != 3 {
		t.Errorf("merge has %d incoming flows, want 3 (two inner branches and the outer else)", in)
	}
	ids := map[model.ID]bool{}
	for _, o := range coll.Objects {
		ids[o.GetID()] = true
	}
	for _, f := range coll.Flows {
		if !ids[f.OriginID] || !ids[f.DestinationID] {
			t.Errorf("flow %s → %s points at a removed object", f.OriginID, f.DestinationID)
		}
	}
}

// Two merges at different places are two merges.
func TestDistinctNestedMergesAreKept(t *testing.T) {
	coll := build2369(t, "  @merge(900, 200)\n  if $P != empty then\n"+
		"    @merge(700, 200)\n    if $P = 'a' then\n      log info node 'x' 'a';\n"+
		"    else\n      log info node 'x' 'b';\n    end if;\n  else\n"+
		"    log info node 'x' 'c';\n  end if;\n  log info node 'x' 'd';\n")
	if n := len(merges2369(coll)); n != 2 {
		t.Fatalf("found %d merges, want 2", n)
	}
}

// A type split with a returning case: the builder still closes the remaining
// cases with a merge, so describe must find that merge for @merge.
func TestTypeSplitMergeFoundPastAReturningCase(t *testing.T) {
	split := &microflows.InheritanceSplit{}
	split.ID = "split"
	ret := &microflows.EndEvent{}
	ret.ID = "ret"
	a := &microflows.ActionActivity{}
	a.ID = "a"
	merge := &microflows.ExclusiveMerge{}
	merge.ID = "merge"
	activityMap := map[model.ID]microflows.MicroflowObject{split.ID: split, ret.ID: ret, a.ID: a, merge.ID: merge}
	flows := map[model.ID][]*microflows.SequenceFlow{
		split.ID: {
			{OriginID: split.ID, DestinationID: ret.ID},
			{OriginID: split.ID, DestinationID: a.ID},
			{OriginID: split.ID, DestinationID: merge.ID},
		},
		a.ID: {{OriginID: a.ID, DestinationID: merge.ID}},
	}
	if commonMergeAfter(split.ID, flows, activityMap) != nil {
		t.Fatal("control: commonMergeAfter needs every branch, and one returns")
	}
	if !buildsMergeForContinuingBranches(split, flows) {
		t.Fatal("a type split is closed by a merge when any case continues")
	}
	if got := mergeAfterContinuingBranches(split.ID, flows, activityMap); got == nil || got.GetID() != merge.ID {
		t.Fatalf("found %v, want the merge the continuing cases share", got)
	}
	// One continuing path alone does not identify the closing merge.
	flows[split.ID] = flows[split.ID][:2]
	if got := mergeAfterContinuingBranches(split.ID, flows, activityMap); got != nil {
		t.Errorf("found %s with one continuing case, want nil", got.GetID())
	}
	// A boolean if with a returning branch builds no merge.
	if buildsMergeForContinuingBranches(&microflows.ExclusiveSplit{}, nil) {
		t.Error("a boolean split must not use the continuing-branch search")
	}
}

func TestLabelledMergeDeclarationWritesSize(t *testing.T) {
	m := &microflows.ExclusiveMerge{}
	m.ID = "m"
	m.Size = model.Size{Width: 90, Height: 60}
	got := strings.Join(mergeDeclarationLines(0, "m1", m, nil, nil), "\n")
	if !strings.Contains(got, "@size(90, 60)") {
		t.Errorf("declaration %q lacks @size(90, 60)", got)
	}
	m.Size = model.Size{Width: MergeSize, Height: MergeSize}
	if got := strings.Join(mergeDeclarationLines(0, "m1", m, nil, nil), "\n"); strings.Contains(got, "@size") {
		t.Errorf("a default 40×40 merge must not print @size: %q", got)
	}
}
