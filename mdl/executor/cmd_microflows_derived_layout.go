// SPDX-License-Identifier: Apache-2.0

// Package executor - canonical DESCRIBE of a flow's layout: authored positions
// are kept, derived ones are left out.
package executor

import (
	"fmt"
	"io"
	"sort"
	"sync/atomic"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A canonical DESCRIBE emits what the author wrote and nothing else (R12,
// ADR-0010). For a flow's layout that means: a position, anchor or curve the
// layout engine would produce on its own is DERIVED and is left out; one it
// would not produce was placed by someone and is kept. @start has worked this
// way since #951; this extends the rule to @position, @merge, @anchor and
// @curve.
//
// "Would the engine produce it" is not answered by comparing a coordinate with a
// formula. The engine places each statement relative to the one before it, so
// whether a node's position is derivable depends on which of its predecessors
// are pinned. The question is answered the only way that cannot drift from the
// engine: describe the flow with a candidate set of annotations, build it again
// exactly as `create or modify` would — nothing is written — and compare the
// result with the stored flow, node by node. A node that lands somewhere else
// gets its annotation back, and the round repeats until nothing moves.
//
// So the guarantee is by construction: when the loop ends, executing the
// description reproduces every stored position, anchor and curve that the full
// description reproduced. Omitting an annotation can never move a node, which
// is the property a Studio Pro-drawn flow needs — its layout is authored, and
// re-executing its description must leave it where the person put it.
//
// Anything that stops the check — a description that does not parse back, a
// rebuild that fails or does not pair with the stored graph, a loop that does
// not settle — keeps every annotation, which is the pre-#748 description.

// flowLayoutKeep is the set of layout annotations a canonical DESCRIBE keeps.
// A nil *flowLayoutKeep keeps everything.
type flowLayoutKeep struct {
	position map[model.ID]bool // @position on an object; @merge / `merge` for a merge
	anchor   map[model.ID]bool // @anchor on an object
	curve    map[model.ID]bool // @curve on an object
	start    bool              // @start
}

func newFlowLayoutKeep() *flowLayoutKeep {
	return &flowLayoutKeep{
		position: map[model.ID]bool{},
		anchor:   map[model.ID]bool{},
		curve:    map[model.ID]bool{},
	}
}

func (k *flowLayoutKeep) keepsPosition(id model.ID) bool { return k == nil || k.position[id] }
func (k *flowLayoutKeep) keepsAnchor(id model.ID) bool   { return k == nil || k.anchor[id] }
func (k *flowLayoutKeep) keepsCurve(id model.ID) bool    { return k == nil || k.curve[id] }

// startLines returns the @start line a description needs. With no keep set it
// is the stored-geometry rule of startAnnotationLines; with one, the start is
// emitted exactly when the rebuild would put it elsewhere.
func (k *flowLayoutKeep) startLines(oc *microflows.MicroflowObjectCollection) []string {
	if k == nil {
		return startAnnotationLines(oc)
	}
	if !k.start || oc == nil {
		return nil
	}
	for _, o := range oc.Objects {
		if se, ok := o.(*microflows.StartEvent); ok {
			p := se.GetPosition()
			return []string{fmt.Sprintf("@start(%d, %d)", p.X, p.Y)}
		}
	}
	return nil
}

// layoutKeep is the keep set a describe traversal carries, nil-safe so a
// hand-built emitter in a unit test keeps everything.
func (e *annotationEmitter) layoutKeep() *flowLayoutKeep {
	if e == nil {
		return nil
	}
	return e.layout
}

func describeLayoutOf(ctx *ExecContext) *flowLayoutKeep {
	if ctx == nil {
		return nil
	}
	return ctx.describeLayout
}

// useDerivedFlowLayout makes the flow body the caller is about to render a
// canonical one, and returns the function that restores the context. flowType
// is "microflow" or "nanoflow"; mf is the flow (a nanoflow wrapped as one).
func useDerivedFlowLayout(ctx *ExecContext, flowType string, mf *microflows.Microflow, name ast.QualifiedName,
	entityNames, microflowNames map[model.ID]string) func() {
	// The body rendered next is the flow the derivation analysed, so it reuses
	// that analysis too.
	prevAnalysis := ctx.flowAnalysis
	if ctx.flowAnalysis == nil {
		ctx.flowAnalysis = newFlowAnalysisMemo()
	}
	keep := derivedFlowLayout(ctx, flowType, mf, name, entityNames, microflowNames)
	prev := ctx.describeLayout
	ctx.describeLayout = keep
	return func() { ctx.describeLayout, ctx.flowAnalysis = prev, prevAnalysis }
}

// gatedLayoutRounds bounds the rounds that pin only the FIRST node of each run
// of misplaced ones. A flow drawn entirely by hand would otherwise take one
// round per node on its longest path; after this many the rest is pinned at
// once, which costs at most some annotations a node downstream of a hand-placed
// one could have done without.
const gatedLayoutRounds = 6

// derivedFlowLayout returns the layout annotations a canonical description of
// mf keeps, or nil to keep them all. See the comment at the top of this file.
func derivedFlowLayout(ctx *ExecContext, flowType string, mf *microflows.Microflow, name ast.QualifiedName,
	entityNames, microflowNames map[model.ID]string) *flowLayoutKeep {
	if ctx == nil || ctx.Backend == nil || mf == nil || mf.ObjectCollection == nil {
		return nil
	}
	// The rebuild's own output — an expose warning, say — belongs to an exec
	// nobody ran.
	// The builds go through a read cache: they rebuild the same flow over an
	// unchanged project, and write nothing.
	// The rounds render the same stored flow again and again, so its graph
	// analysis is done once for all of them (mendixlabs/mxcli#1301).
	prevOut, prevLayout, prevBackend, prevAnalysis := ctx.Output, ctx.describeLayout, ctx.Backend, ctx.flowAnalysis
	ctx.Output = io.Discard
	ctx.Backend = newLayoutCheckBackend(ctx.Backend)
	ctx.flowAnalysis = flowAnalysisOf(ctx).forRebuild()
	defer func() {
		ctx.Output, ctx.describeLayout, ctx.Backend, ctx.flowAnalysis = prevOut, prevLayout, prevBackend, prevAnalysis
	}()

	stored := indexFlowGraph(mf.ObjectCollection)
	successors := layoutSuccessors(stored)
	keep := newFlowLayoutKeep()
	// A note standing on the canvas is told apart from a note on the first
	// statement only by an activity annotation after it (the visitor's
	// hasLaterActivityAnnotation). Without one, a free note comes back attached
	// — so the statement the start flows into keeps its @position.
	if len(collectFreeAnnotations(mf.ObjectCollection)) > 0 {
		for _, f := range stored.outgoing[stored.start] {
			keep.position[f.DestinationID] = true
		}
	}

	// Every round that does not return pins at least one more annotation, and
	// there are at most three per object plus @start.
	maxRounds := 3*len(stored.object) + 2
	for round := 0; round < maxRounds; round++ {
		derivedLayoutRounds.Add(1)
		ctx.describeLayout = keep
		src := renderMicroflowMDL(ctx, flowType, mf, name, entityNames, microflowNames, nil)
		rebuilt, err := rebuildDescribedFlow(ctx, flowType, src)
		if err != nil {
			return nil
		}
		p := &flowPairing{
			stored:  stored,
			rebuilt: indexFlowGraph(rebuilt),
			objects: map[model.ID]model.ID{},
			flows:   map[model.ID]model.ID{},
		}
		if err := p.walk(); err != nil {
			return nil
		}

		// Positions first: an anchor or the start is judged against settled
		// positions, since both are placed relative to their neighbours.
		if misplaced := misplacedObjects(p, keep); len(misplaced) > 0 {
			pin := misplaced
			if round < gatedLayoutRounds {
				if first := firstOfRuns(misplaced, successors); len(first) > 0 {
					pin = first
				}
			}
			for _, id := range pin {
				keep.position[id] = true
			}
			continue
		}
		if !pinMisroutedFlows(p, keep) {
			return keep
		}
	}
	return nil
}

// derivedLayoutRounds counts the derivation's rounds, and splitMergeAnalyses
// how often a flow's split/merge structure is worked out — the per-round cost
// mendixlabs/mxcli#1301 was about. Only tests read them.
var derivedLayoutRounds, splitMergeAnalyses atomic.Int64

// flowAnalysisMemo holds what a description's rendering works out from the
// stored flow alone, for the rounds of one derivation. None of it depends on
// which layout annotations a round keeps, and the analysis grows faster than
// the flow: re-done per round it was most of describe's cost on a hand-laid
// flow, which takes up to gatedLayoutRounds+2 rounds (#1301).
//
// It is keyed on the stored collection, and lives for one canonical describe —
// the derivation and the body printed after it — over a flow nothing modifies
// meanwhile. A nil memo — every other render — computes afresh.
type flowAnalysisMemo struct {
	splitMerge map[*microflows.MicroflowObjectCollection]map[model.ID]model.ID
	labels     map[*microflows.MicroflowObjectCollection]mergeLabels

	// rebuildOnly marks a derivation round's render, whose text is rebuilt and
	// never shown. It leaves out the body warnings: they are comments, which
	// the rebuild does not read, and working them out repeats the analysis.
	rebuildOnly bool

	// voids answers whether a called flow returns nothing, for every call the
	// description renders (describedCallIsVoid). A pointer, so a derivation
	// round's view shares it.
	voids **voidCodeActions
}

// forRebuild is the memo a derivation round renders with: the same analyses,
// without the warnings.
func (m *flowAnalysisMemo) forRebuild() *flowAnalysisMemo {
	if m == nil {
		m = newFlowAnalysisMemo()
	}
	view := *m
	view.rebuildOnly = true
	return &view
}

func newFlowAnalysisMemo() *flowAnalysisMemo {
	return &flowAnalysisMemo{
		splitMerge: map[*microflows.MicroflowObjectCollection]map[model.ID]model.ID{},
		labels:     map[*microflows.MicroflowObjectCollection]mergeLabels{},
		voids:      new(*voidCodeActions),
	}
}

func flowAnalysisOf(ctx *ExecContext) *flowAnalysisMemo {
	if ctx == nil {
		return nil
	}
	return ctx.flowAnalysis
}

// splitMergePoints is findSplitMergePoints through the memo. Callers only read
// the map.
func (m *flowAnalysisMemo) splitMergePoints(oc *microflows.MicroflowObjectCollection, compute func() map[model.ID]model.ID) map[model.ID]model.ID {
	if m == nil {
		return compute()
	}
	if r, ok := m.splitMerge[oc]; ok {
		return r
	}
	r := compute()
	m.splitMerge[oc] = r
	return r
}

// mergeLabels is the labelling of a flow's merges through the memo.
func (m *flowAnalysisMemo) mergeLabels(oc *microflows.MicroflowObjectCollection) mergeLabels {
	compute := func() mergeLabels {
		l := mergeAllLabels(labelRejoinMerges(oc), labelCrossedMerges(oc))
		l.inline = newInlineMerges(oc)
		return l
	}
	if m == nil {
		return compute()
	}
	if r, ok := m.labels[oc]; ok {
		return r
	}
	r := compute()
	m.labels[oc] = r
	return r
}

// rebuildDescribedFlow builds the flow a description defines, the way `create
// or modify` would build it, and returns its objects. Nothing is written.
func rebuildDescribedFlow(ctx *ExecContext, flowType, src string) (*microflows.MicroflowObjectCollection, error) {
	prog, errs := visitor.Build(describedSource(ctx, src))
	if len(errs) > 0 {
		return nil, fmt.Errorf("description does not parse back: %v", errs[0])
	}
	opts := buildFlowOpts{ResetLayout: true, Quiet: true}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			if flowType != "microflow" {
				continue
			}
			built, err := buildMicroflowFromStmt(ctx, s, opts)
			if err != nil {
				return nil, err
			}
			return built.Microflow.ObjectCollection, nil
		case *ast.CreateNanoflowStmt:
			if flowType != "nanoflow" {
				continue
			}
			built, err := buildNanoflowFromStmt(ctx, s, opts)
			if err != nil {
				return nil, err
			}
			return built.Nanoflow.ObjectCollection, nil
		}
	}
	return nil, fmt.Errorf("description holds no %s", flowType)
}

// misplacedObjects returns the stored objects, in document order, that the
// rebuild put somewhere else and whose position is not pinned yet. Notes are
// left to their own emitter, which already writes a position only when it
// differs from the default; the start event is judged with the flows.
func misplacedObjects(p *flowPairing, keep *flowLayoutKeep) []model.ID {
	var out []model.ID
	for _, sid := range p.stored.order {
		bid, ok := p.objects[sid]
		if !ok || keep.position[sid] {
			continue
		}
		switch p.stored.object[sid].(type) {
		case *microflows.Annotation, *microflows.StartEvent:
			continue
		}
		if p.stored.object[sid].GetPosition() != p.rebuilt.object[bid].GetPosition() {
			out = append(out, sid)
		}
	}
	return out
}

// firstOfRuns keeps the misplaced objects no other misplaced object leads to.
//
// The engine places a statement relative to the one before it, so when a node
// was moved by hand everything after it lands somewhere else too — not because
// it was moved, but because its predecessor was. Pinning only the first of such
// a run and building again tells the two apart. In a cycle every member leads
// to every other, so nothing is first; the caller then pins them all.
func firstOfRuns(misplaced []model.ID, successors map[model.ID][]model.ID) []model.ID {
	downstream := map[model.ID]bool{}
	for _, from := range misplaced {
		seen := map[model.ID]bool{from: true}
		queue := append([]model.ID(nil), successors[from]...)
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			if seen[id] {
				continue
			}
			seen[id] = true
			downstream[id] = true
			queue = append(queue, successors[id]...)
		}
	}
	var out []model.ID
	for _, id := range misplaced {
		if !downstream[id] {
			out = append(out, id)
		}
	}
	return out
}

// layoutSuccessors is what each object's placement leads to: the destinations
// of its sequence flows, and for a loop the objects in its body, which are
// placed inside it.
func layoutSuccessors(g *flowGraphIndex) map[model.ID][]model.ID {
	out := map[model.ID][]model.ID{}
	for id, flows := range g.outgoing {
		for _, f := range flows {
			out[id] = append(out[id], f.DestinationID)
		}
	}
	for id, o := range g.object {
		loop, ok := o.(*microflows.LoopedActivity)
		if !ok || loop.ObjectCollection == nil {
			continue
		}
		for _, inner := range loop.ObjectCollection.Objects {
			out[id] = append(out[id], inner.GetID())
		}
	}
	return out
}

// pinMisroutedFlows pins the @start, @anchor and @curve annotations whose
// absence changes where the rebuild starts or routes a flow, and reports
// whether it pinned anything new.
//
// A flow's anchors are written on both of its ends — the origin's `from`, a
// split's or loop's branch groups, the destination's `to` — so a misrouted flow
// pins the @anchor of both. Its curve is written on the origin. A flow through
// a merge the description leaves out (a pass-through merge) is not judged: the
// rebuild has one flow where the model has two, and no annotation can say more.
func pinMisroutedFlows(p *flowPairing, keep *flowLayoutKeep) bool {
	grew := false
	pin := func(set map[model.ID]bool, id model.ID) {
		if !set[id] {
			set[id] = true
			grew = true
		}
	}
	if !keep.start && p.stored.start != "" {
		if bid, ok := p.objects[p.stored.start]; ok &&
			p.stored.object[p.stored.start].GetPosition() != p.rebuilt.object[bid].GetPosition() {
			keep.start = true
			grew = true
		}
	}
	through := map[model.ID]bool{}
	for _, m := range p.passThrough {
		through[m.inFlow], through[m.outFlow] = true, true
	}
	ids := make([]model.ID, 0, len(p.flows))
	for sid := range p.flows {
		ids = append(ids, sid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, sid := range ids {
		if through[sid] {
			continue
		}
		sf, bf := p.stored.flow[sid], p.rebuilt.flow[p.flows[sid]]
		if sf == nil || bf == nil {
			continue
		}
		if sf.OriginConnectionIndex != bf.OriginConnectionIndex ||
			sf.DestinationConnectionIndex != bf.DestinationConnectionIndex {
			pin(keep.anchor, sf.OriginID)
			pin(keep.anchor, sf.DestinationID)
		}
		if orZeroVector(sf.OriginControlVector) != orZeroVector(bf.OriginControlVector) ||
			orZeroVector(sf.DestinationControlVector) != orZeroVector(bf.DestinationControlVector) {
			pin(keep.curve, sf.OriginID)
		}
	}
	return grew
}
