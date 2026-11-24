// SPDX-License-Identifier: Apache-2.0

// Package executor - `mxcli layout flows`: re-arrange an existing microflow or
// nanoflow with the same layout engine CREATE uses.
package executor

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Laying out a flow is a rebuild whose only product is coordinates.
//
// The layout engine is the flow builder: it places each activity while it
// creates it, so there is no separate pass to call on a stored graph. Rather
// than grow a second engine that works on stored graphs — and drifts from the
// first — the stored flow is described to MDL, stripped of every layout
// annotation, and built again exactly as CREATE builds it. That build is thrown
// away except for its geometry, which is paired back onto the STORED objects and
// patched into the stored BSON.
//
// Patching the stored bytes, rather than writing the rebuilt flow, is the point:
// every $ID, every GUID and every property MDL cannot express stays as it is.
// Only RelativeMiddlePoint, Size, the flows' connection indexes and their bezier
// vectors change. So a flow laid out here is laid out exactly as CREATE would
// lay out its DESCRIBE with the annotations removed, and nothing else about it
// moves.
//
// The pairing is also the safety check. The rebuilt graph must match the stored
// one object for object (same kind, same branch structure); if describe → build
// does not reproduce the stored graph, the flow does not round-trip through MDL
// and is refused rather than laid out by guesswork.

// FlowLayoutResult reports what laying out one flow did.
type FlowLayoutResult struct {
	Kind    string // "microflow" or "nanoflow"
	Name    string // qualified name
	Objects int    // objects in the flow
	Moved   int    // objects whose position or size changed
	Flows   int    // sequence flows whose anchors or curve changed
	// Restyled says the flow's @mxcli-layout documentation line changed: it is
	// now laid out in a different style than the one it recorded.
	Restyled bool
	Refused  string // why the flow was left alone; empty when it was laid out
}

// Changed reports whether laying the flow out changes anything.
func (r FlowLayoutResult) Changed() bool { return r.Moved > 0 || r.Flows > 0 || r.Restyled }

// LayoutFlow lays out one microflow or nanoflow (kind "microflow" or
// "nanoflow"). With dryRun, it computes the result and writes nothing.
//
// A flow that cannot be laid out safely is reported in Refused, not as an
// error: a batch over a module should skip it and carry on.
func (e *Executor) LayoutFlow(kind string, name ast.QualifiedName, dryRun bool) (FlowLayoutResult, error) {
	return layoutFlow(e.newExecContext(context.Background()), kind, name, dryRun)
}

// Modules lists the connected project's modules.
func (e *Executor) Modules() ([]*model.Module, error) {
	ctx := e.newExecContext(context.Background())
	if !ctx.Connected() {
		return nil, mdlerrors.NewNotConnected()
	}
	return ctx.Backend.ListModules()
}

// ListFlowNames returns the qualified names of the live microflows and
// nanoflows in a module, sorted.
func (e *Executor) ListFlowNames(moduleName string) (mfs []string, nfs []string, err error) {
	ctx := e.newExecContext(context.Background())
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, nil, mdlerrors.NewBackend("build hierarchy", err)
	}
	all, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return nil, nil, mdlerrors.NewBackend("list microflows", err)
	}
	for _, mf := range all {
		if !mf.Excluded && h.GetModuleName(h.FindModuleID(mf.ContainerID)) == moduleName {
			mfs = append(mfs, moduleName+"."+mf.Name)
		}
	}
	allNf, err := ctx.Backend.ListNanoflows()
	if err != nil {
		return nil, nil, mdlerrors.NewBackend("list nanoflows", err)
	}
	for _, nf := range allNf {
		if !nf.Excluded && h.GetModuleName(h.FindModuleID(nf.ContainerID)) == moduleName {
			nfs = append(nfs, moduleName+"."+nf.Name)
		}
	}
	sort.Strings(mfs)
	sort.Strings(nfs)
	return mfs, nfs, nil
}

func layoutFlow(ctx *ExecContext, kind string, name ast.QualifiedName, dryRun bool) (FlowLayoutResult, error) {
	res := FlowLayoutResult{Kind: kind, Name: name.String()}
	if !dryRun && !ctx.ConnectedForWrite() {
		return res, mdlerrors.NewNotConnectedWrite()
	}

	stored, err := storedFlow(ctx, kind, name)
	if err != nil {
		return res, err
	}
	rebuilt, err := rebuildFlowLayout(ctx, kind, name)
	if err != nil {
		res.Refused = err.Error()
		return res, nil
	}

	plan, err := pairFlowLayout(stored, rebuilt)
	if err != nil {
		res.Refused = err.Error()
		return res, nil
	}
	res.Objects = plan.objectCount

	raw, err := ctx.Backend.GetRawUnitBytes(stored.id)
	if err != nil {
		return res, mdlerrors.NewBackend("read "+kind+" "+name.String(), err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return res, mdlerrors.NewBackend("parse "+kind+" "+name.String(), err)
	}
	patched := plan.apply(doc)
	res.Moved, res.Flows = plan.movedObjects, plan.changedFlows
	// The flow now records the style it was laid out in (flowlayout_config.go).
	// Only the marker line changes; the rest of the documentation is the stored
	// text, not the description's normal form of it.
	style, err := ctx.ProjectFlowLayout()
	if err != nil {
		return res, fmt.Errorf("layout config: %w", err)
	}
	res.Restyled = setLayoutString(patched, "Documentation", withFlowLayoutMarker(stored.documentation, *style))
	if !res.Changed() || dryRun {
		return res, nil
	}

	out, err := bson.Marshal(patched)
	if err != nil {
		return res, mdlerrors.NewBackend("encode "+kind+" "+name.String(), err)
	}
	// A patch of the stored bytes, not a rebuild: the loop boxes it fitted to
	// their bodies are meant, and carrying the stored sizes back (as a rebuild
	// is) kept every one at its old size while reporting it moved
	// (ako/mxcli#2564).
	if err := ctx.Backend.UpdateRawUnitPatch(string(stored.id), out); err != nil {
		return res, mdlerrors.NewBackend("write "+kind+" "+name.String(), err)
	}
	return res, nil
}

// layoutGraph is the part of a flow the layout pairing needs, the same for a
// microflow and a nanoflow.
type layoutGraph struct {
	id            model.ID
	parameters    []*microflows.MicroflowParameter
	objects       *microflows.MicroflowObjectCollection
	documentation string
}

func storedFlow(ctx *ExecContext, kind string, name ast.QualifiedName) (*layoutGraph, error) {
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, mdlerrors.NewBackend("build hierarchy", err)
	}
	inModule := func(containerID model.ID) bool {
		return h.GetModuleName(h.FindModuleID(containerID)) == name.Module
	}
	switch kind {
	case "microflow":
		all, err := ctx.Backend.ListMicroflows()
		if err != nil {
			return nil, mdlerrors.NewBackend("list microflows", err)
		}
		mf, ok := pickLive(all,
			func(m *microflows.Microflow) bool { return m.Name == name.Name && inModule(m.ContainerID) },
			func(m *microflows.Microflow) bool { return m.Excluded })
		if !ok {
			return nil, mdlerrors.NewNotFound("microflow", name.String())
		}
		return &layoutGraph{id: mf.ID, parameters: mf.Parameters, objects: mf.ObjectCollection, documentation: mf.Documentation}, nil
	case "nanoflow":
		all, err := ctx.Backend.ListNanoflows()
		if err != nil {
			return nil, mdlerrors.NewBackend("list nanoflows", err)
		}
		nf, ok := pickLive(all,
			func(n *microflows.Nanoflow) bool { return n.Name == name.Name && inModule(n.ContainerID) },
			func(n *microflows.Nanoflow) bool { return n.Excluded })
		if !ok {
			return nil, mdlerrors.NewNotFound("nanoflow", name.String())
		}
		return &layoutGraph{id: nf.ID, parameters: nf.Parameters, objects: nf.ObjectCollection, documentation: nf.Documentation}, nil
	}
	return nil, fmt.Errorf("cannot lay out a %s", kind)
}

// rebuildFlowLayout describes the stored flow, strips its layout annotations and
// builds it again the way CREATE would — without writing anything.
func rebuildFlowLayout(ctx *ExecContext, kind string, name ast.QualifiedName) (*layoutGraph, error) {
	var mdl string
	var err error
	if kind == "nanoflow" {
		mdl, _, err = describeNanoflowToString(ctx, name)
	} else {
		mdl, _, err = describeMicroflowToString(ctx, name)
	}
	if err != nil {
		return nil, err
	}

	prog, errs := visitor.Build(describedSource(ctx, mdl))
	if len(errs) > 0 {
		return nil, fmt.Errorf("its description does not parse back (%v), so it does not round-trip through MDL", errs[0])
	}

	// Laid out in the project's style (or --style), not the one the flow records:
	// re-styling a flow is what this is for.
	opts := buildFlowOpts{ResetLayout: true, IgnoreRecordedLayout: true}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			stripFlowLayout(s)
			built, err := buildMicroflowFromStmt(ctx, s, opts)
			if err != nil {
				return nil, fmt.Errorf("rebuilding it from its description failed: %w", err)
			}
			mf := built.Microflow
			return &layoutGraph{id: mf.ID, parameters: mf.Parameters, objects: mf.ObjectCollection}, nil
		case *ast.CreateNanoflowStmt:
			stripFlowLayout(s)
			built, err := buildNanoflowFromStmt(ctx, s, opts)
			if err != nil {
				return nil, fmt.Errorf("rebuilding it from its description failed: %w", err)
			}
			nf := built.Nanoflow
			return &layoutGraph{id: nf.ID, parameters: nf.Parameters, objects: nf.ObjectCollection}, nil
		}
	}
	return nil, fmt.Errorf("its description holds no %s definition", kind)
}

// stripFlowLayout clears every layout annotation in a CREATE MICROFLOW or
// CREATE NANOFLOW statement, so the builder places everything itself.
//
// What goes is exactly what DESCRIBE emits to pin geometry: @position (on
// activities, parameters and notes), @anchor in all its forms, @curve, @merge and
// @start. A note keeps its size, which is its content's box rather than its
// placement; everything else in the statement is left alone.
//
// Reflective because statements nest bodies in a dozen places — branches, loops,
// splits' cases, error handlers — and a hand-written walk that missed one would
// leave that body pinned where it was.
func stripFlowLayout(stmt ast.Statement) {
	clearLayoutAnnotations(reflect.ValueOf(stmt), map[uintptr]bool{})
}

var (
	activityAnnotationsType = reflect.TypeOf(&ast.ActivityAnnotations{})
	microflowNoteType       = reflect.TypeOf(ast.MicroflowAnnotation{})
	microflowParamType      = reflect.TypeOf(ast.MicroflowParam{})
)

func clearLayoutAnnotations(v reflect.Value, seen map[uintptr]bool) {
	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() || seen[v.Pointer()] {
			return
		}
		seen[v.Pointer()] = true
		if v.Type() == activityAnnotationsType {
			ann := v.Interface().(*ast.ActivityAnnotations)
			ann.Position = nil
			ann.Anchor = nil
			ann.TrueBranchAnchor = nil
			ann.FalseBranchAnchor = nil
			ann.IteratorAnchor = nil
			ann.BodyTailAnchor = nil
			ann.Curve = nil
			ann.Merge = nil
			ann.Start = nil
			// A loop's box and a merge's box are layout too (ako/mxcli#2369):
			// kept, the rebuild drew every loop at its stored size, so a box
			// too big for its body stayed that size (ako/mxcli#2564).
			ann.Size = nil
			ann.MergeSize = nil
		}
		clearLayoutAnnotations(v.Elem(), seen)
	case reflect.Interface:
		if !v.IsNil() {
			clearLayoutAnnotations(v.Elem(), seen)
		}
	case reflect.Struct:
		if v.CanSet() {
			switch v.Type() {
			case microflowNoteType:
				v.FieldByName("Position").Set(reflect.Zero(v.FieldByName("Position").Type()))
			case microflowParamType:
				v.FieldByName("Position").Set(reflect.Zero(v.FieldByName("Position").Type()))
			}
		}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			if t.Field(i).IsExported() {
				clearLayoutAnnotations(v.Field(i), seen)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			clearLayoutAnnotations(v.Index(i), seen)
		}
	}
}

// layoutPlan is the geometry to patch onto the stored flow, keyed by the stored
// element's normalised $ID.
type layoutPlan struct {
	objects map[string]objectGeometry
	flows   map[string]flowGeometry

	objectCount  int
	movedObjects int
	changedFlows int
}

type objectGeometry struct {
	position model.Point
	size     *model.Size // nil leaves the stored size alone (parameters, notes)
}

type flowGeometry struct {
	originIndex, destinationIndex int
	originVector, destVector      string
}

// normID puts an element ID in the form both the typed model and the raw BSON
// reduce to: lower-case hex without dashes.
func normID(id string) string {
	return strings.ToLower(strings.ReplaceAll(id, "-", ""))
}

// pairFlowLayout matches the rebuilt flow to the stored one and returns the
// geometry to carry across. Any stored object the walk cannot pair is a refusal.
func pairFlowLayout(stored, rebuilt *layoutGraph) (*layoutPlan, error) {
	if stored.objects == nil || rebuilt.objects == nil {
		return nil, fmt.Errorf("it has no flow to lay out")
	}
	p := &flowPairing{
		stored:  indexFlowGraph(stored.objects),
		rebuilt: indexFlowGraph(rebuilt.objects),
		objects: map[model.ID]model.ID{},
		flows:   map[model.ID]model.ID{},
	}
	if err := p.walk(); err != nil {
		return nil, err
	}

	plan := &layoutPlan{objects: map[string]objectGeometry{}, flows: map[string]flowGeometry{}}
	for sid, bid := range p.objects {
		so, bo := p.stored.object[sid], p.rebuilt.object[bid]
		size := objectSize(bo)
		g := objectGeometry{position: bo.GetPosition(), size: &size}
		if _, isNote := so.(*microflows.Annotation); isNote {
			g.size = nil
		}
		plan.objects[normID(string(sid))] = g
	}
	for sid, bid := range p.flows {
		bf := p.rebuilt.flow[bid]
		plan.flows[normID(string(sid))] = flowGeometry{
			originIndex:      bf.OriginConnectionIndex,
			destinationIndex: bf.DestinationConnectionIndex,
			originVector:     orZeroVector(bf.OriginControlVector),
			destVector:       orZeroVector(bf.DestinationControlVector),
		}
	}

	for _, m := range p.passThrough {
		placePassThroughMerge(plan, m, p.rebuilt)
	}

	// Parameters pair by name. One the rebuild placed without an annotation
	// gets the position the writer derives from its index.
	byName := map[string]model.Point{}
	for i, bp := range rebuilt.parameters {
		pos := microflows.DerivedParameterPosition(i, rebuilt.parameters)
		if bp.Position != nil {
			pos = *bp.Position
		}
		byName[bp.Name] = pos
	}
	for _, sp := range stored.parameters {
		pos, ok := byName[sp.Name]
		if !ok {
			return nil, fmt.Errorf("its parameter $%s did not survive the rebuild", sp.Name)
		}
		plan.objects[normID(string(sp.ID))] = objectGeometry{position: pos}
	}
	plan.objectCount = len(p.objects) + len(p.passThrough) + len(stored.parameters)
	return plan, nil
}

// placePassThroughMerge puts a merge the rebuild does not have halfway along the
// rebuilt edge it sits on, and splits that edge's anchors between the two stored
// flows around it: the one in keeps the rebuilt flow's origin side, the one out
// keeps its destination side, and they meet at the merge on the side facing
// each. A chain of such merges shares one point, which is rare enough not to
// spread out.
func placePassThroughMerge(plan *layoutPlan, m passThroughMerge, rebuilt *flowGraphIndex) {
	bf := rebuilt.flow[m.rebuilt]
	from := rebuilt.object[bf.OriginID].GetPosition()
	to := rebuilt.object[bf.DestinationID].GetPosition()

	// The edge enters its destination from the left or right (a horizontal
	// run) or from the top or bottom (a vertical drop); the merge sits on the
	// straight part leading in.
	pos := model.Point{X: (from.X + to.X) / 2, Y: to.Y}
	if bf.DestinationConnectionIndex == AnchorTop || bf.DestinationConnectionIndex == AnchorBottom {
		pos = model.Point{X: to.X, Y: (from.Y + to.Y) / 2}
	}
	plan.objects[normID(string(m.merge))] = objectGeometry{position: pos, size: &model.Size{Width: MergeSize, Height: MergeSize}}

	plan.flows[normID(string(m.inFlow))] = flowGeometry{
		originIndex:      bf.OriginConnectionIndex,
		destinationIndex: bf.DestinationConnectionIndex,
		originVector:     "0;0",
		destVector:       "0;0",
	}
	plan.flows[normID(string(m.outFlow))] = flowGeometry{
		originIndex:      oppositeAnchor(bf.DestinationConnectionIndex),
		destinationIndex: bf.DestinationConnectionIndex,
		originVector:     "0;0",
		destVector:       "0;0",
	}
}

func oppositeAnchor(side int) int {
	switch side {
	case AnchorTop:
		return AnchorBottom
	case AnchorBottom:
		return AnchorTop
	case AnchorLeft:
		return AnchorRight
	}
	return AnchorLeft
}

// layoutRawID reads a stored element's $ID in the typed model's form. The binary
// is a .NET GUID, whose first three groups are little-endian, so its plain hex
// is NOT the UUID string the decoder produced.
func layoutRawID(d bson.D) (string, bool) {
	for _, e := range d {
		if e.Key != "$ID" {
			continue
		}
		switch v := e.Value.(type) {
		case bson.Binary:
			return types.BlobToUUID(v.Data), true
		case []byte:
			return types.BlobToUUID(v), true
		case string:
			return v, true
		}
	}
	return "", false
}

func orZeroVector(v string) string {
	if v == "" {
		return "0;0"
	}
	return v
}

func objectSize(o microflows.MicroflowObject) model.Size {
	if s, ok := o.(interface{ GetSize() model.Size }); ok {
		return s.GetSize()
	}
	return model.Size{}
}

// flowGraphIndex is a flow's objects and edges, flattened across loop bodies.
type flowGraphIndex struct {
	object   map[model.ID]microflows.MicroflowObject
	flow     map[model.ID]*microflows.SequenceFlow
	outgoing map[model.ID][]*microflows.SequenceFlow
	incoming map[model.ID]int
	// notesOn maps an object to the notes wired to it; freeNotes are the ones
	// wired to nothing.
	notesOn   map[model.ID][]*microflows.Annotation
	freeNotes []*microflows.Annotation
	start     model.ID
	// order is every object in document order, for deterministic reporting.
	order []model.ID
}

func indexFlowGraph(oc *microflows.MicroflowObjectCollection) *flowGraphIndex {
	g := &flowGraphIndex{
		object:   map[model.ID]microflows.MicroflowObject{},
		flow:     map[model.ID]*microflows.SequenceFlow{},
		outgoing: map[model.ID][]*microflows.SequenceFlow{},
		incoming: map[model.ID]int{},
		notesOn:  map[model.ID][]*microflows.Annotation{},
	}
	var notes []*microflows.Annotation
	var annFlows []*microflows.AnnotationFlow
	var addCollection func(c *microflows.MicroflowObjectCollection)
	addCollection = func(c *microflows.MicroflowObjectCollection) {
		if c == nil {
			return
		}
		for _, o := range c.Objects {
			g.object[o.GetID()] = o
			g.order = append(g.order, o.GetID())
			switch t := o.(type) {
			case *microflows.StartEvent:
				if g.start == "" {
					g.start = t.ID
				}
			case *microflows.LoopedActivity:
				addCollection(t.ObjectCollection)
			case *microflows.Annotation:
				notes = append(notes, t)
			}
		}
		for _, f := range c.Flows {
			g.flow[f.ID] = f
			g.outgoing[f.OriginID] = append(g.outgoing[f.OriginID], f)
			g.incoming[f.DestinationID]++
		}
		annFlows = append(annFlows, c.AnnotationFlows...)
	}
	addCollection(oc)

	wired := map[model.ID]bool{}
	for _, af := range annFlows {
		note, target := af.OriginID, af.DestinationID
		if _, isNote := g.object[note].(*microflows.Annotation); !isNote {
			note, target = target, note
		}
		n, ok := g.object[note].(*microflows.Annotation)
		if !ok {
			continue
		}
		g.notesOn[target] = append(g.notesOn[target], n)
		wired[n.ID] = true
	}
	for _, n := range notes {
		if !wired[n.ID] {
			g.freeNotes = append(g.freeNotes, n)
		}
	}
	return g
}

// flowPairing walks the stored and rebuilt graphs in step from their start
// events, pairing each object with its counterpart and each flow with the flow
// that leaves the paired origin on the same branch.
type flowPairing struct {
	stored, rebuilt *flowGraphIndex
	objects         map[model.ID]model.ID // stored → rebuilt
	flows           map[model.ID]model.ID
	queue           [][2]model.ID
	// passThrough are stored merges the rebuild has no node for — see
	// skipPassThroughMerges.
	passThrough []passThroughMerge
}

// passThroughMerge is a stored ExclusiveMerge with one flow in and one flow out.
// It joins nothing, so DESCRIBE leaves it out and the rebuild has no node to
// pair it with. It is placed on the rebuilt edge it sits on instead.
type passThroughMerge struct {
	merge   model.ID
	inFlow  model.ID // stored flow into the merge
	outFlow model.ID // stored flow out of it
	rebuilt model.ID // the rebuilt flow the whole chain stands for
}

func (p *flowPairing) walk() error {
	if p.stored.start == "" || p.rebuilt.start == "" {
		return fmt.Errorf("it has no start event")
	}
	if err := p.pair(p.stored.start, p.rebuilt.start); err != nil {
		return err
	}
	for len(p.queue) > 0 {
		next := p.queue[0]
		p.queue = p.queue[1:]
		if err := p.visit(next[0], next[1]); err != nil {
			return err
		}
	}

	// Notes are not on the control flow: pair the ones wired to a paired object
	// by their caption, and the free ones by caption in order.
	for sid, bid := range p.objects {
		if err := p.pairNotes(p.stored.notesOn[sid], p.rebuilt.notesOn[bid]); err != nil {
			return err
		}
	}
	if err := p.pairNotes(p.stored.freeNotes, p.rebuilt.freeNotes); err != nil {
		return err
	}

	for _, id := range p.stored.order {
		if _, ok := p.objects[id]; !ok && !p.isPassThrough(id) {
			return fmt.Errorf("its %s did not survive the rebuild, so it does not round-trip through MDL",
				describeLayoutObject(p.stored.object[id]))
		}
	}
	if len(p.objects) != len(p.rebuilt.object) {
		return fmt.Errorf("rebuilding it from its description made %d objects where it has %d, so it does not round-trip through MDL",
			len(p.rebuilt.object), len(p.stored.object))
	}
	for id := range p.stored.flow {
		if _, ok := p.flows[id]; !ok {
			return fmt.Errorf("one of its sequence flows did not survive the rebuild, so it does not round-trip through MDL")
		}
	}
	return nil
}

func (p *flowPairing) pair(sid, bid model.ID) error {
	if prev, ok := p.objects[sid]; ok {
		if prev != bid {
			return fmt.Errorf("its %s joins branches differently after a rebuild, so it does not round-trip through MDL",
				describeLayoutObject(p.stored.object[sid]))
		}
		return nil
	}
	so, bo := p.stored.object[sid], p.rebuilt.object[bid]
	if so == nil || bo == nil {
		return fmt.Errorf("a sequence flow points at an object that is not in the flow")
	}
	if layoutKind(so) != layoutKind(bo) {
		return fmt.Errorf("its %s came back as a %s after a rebuild, so it does not round-trip through MDL",
			describeLayoutObject(so), describeLayoutObject(bo))
	}
	p.objects[sid] = bid
	p.queue = append(p.queue, [2]model.ID{sid, bid})
	return nil
}

func (p *flowPairing) visit(sid, bid model.ID) error {
	// A loop's body is not reached by a flow: its entry is the body object
	// nothing flows into.
	if sl, ok := p.stored.object[sid].(*microflows.LoopedActivity); ok {
		bl := p.rebuilt.object[bid].(*microflows.LoopedActivity)
		se, be := loopEntries(sl, p.stored), loopEntries(bl, p.rebuilt)
		if len(se) != len(be) || len(se) > 1 {
			return fmt.Errorf("its loop body does not round-trip through MDL")
		}
		if len(se) == 1 {
			if err := p.pair(se[0], be[0]); err != nil {
				return err
			}
		}
	}

	sOut, bOut := p.stored.outgoing[sid], p.rebuilt.outgoing[bid]
	if len(sOut) != len(bOut) {
		return fmt.Errorf("its %s has %d outgoing flows after a rebuild where it has %d, so it does not round-trip through MDL",
			describeLayoutObject(p.stored.object[sid]), len(bOut), len(sOut))
	}
	used := make([]bool, len(bOut))
	for _, sf := range sOut {
		key := flowBranchKey(sf)
		match := -1
		for i, bf := range bOut {
			if !used[i] && flowBranchKey(bf) == key {
				match = i
				break
			}
		}
		if match < 0 {
			return fmt.Errorf("its %s has no %s branch after a rebuild, so it does not round-trip through MDL",
				describeLayoutObject(p.stored.object[sid]), key)
		}
		used[match] = true
		bf := bOut[match]
		p.flows[sf.ID] = bf.ID
		dest := p.skipPassThroughMerges(sf, bf)
		if err := p.pair(dest, bf.DestinationID); err != nil {
			return err
		}
	}
	return nil
}

// skipPassThroughMerges follows a stored flow through any merges the rebuild
// does not have, recording each, and returns the object the rebuilt flow's
// destination stands for.
func (p *flowPairing) skipPassThroughMerges(sf, bf *microflows.SequenceFlow) model.ID {
	dest := sf.DestinationID
	for {
		if _, isMerge := p.stored.object[dest].(*microflows.ExclusiveMerge); !isMerge {
			return dest
		}
		if _, alsoMerge := p.rebuilt.object[bf.DestinationID].(*microflows.ExclusiveMerge); alsoMerge {
			return dest
		}
		out := p.stored.outgoing[dest]
		if p.stored.incoming[dest] != 1 || len(out) != 1 {
			return dest
		}
		p.passThrough = append(p.passThrough, passThroughMerge{
			merge: dest, inFlow: sf.ID, outFlow: out[0].ID, rebuilt: bf.ID,
		})
		p.flows[out[0].ID] = bf.ID
		sf = out[0]
		dest = sf.DestinationID
	}
}

func (p *flowPairing) isPassThrough(id model.ID) bool {
	for _, m := range p.passThrough {
		if m.merge == id {
			return true
		}
	}
	return false
}

func (p *flowPairing) pairNotes(stored, rebuilt []*microflows.Annotation) error {
	used := make([]bool, len(rebuilt))
	for _, sn := range stored {
		if _, done := p.objects[sn.ID]; done {
			continue
		}
		for i, bn := range rebuilt {
			if !used[i] && bn.Caption == sn.Caption {
				if prev, ok := reverseLookup(p.objects, bn.ID); ok && prev != sn.ID {
					continue
				}
				used[i] = true
				p.objects[sn.ID] = bn.ID
				break
			}
		}
	}
	return nil
}

func reverseLookup(m map[model.ID]model.ID, v model.ID) (model.ID, bool) {
	for k, x := range m {
		if x == v {
			return k, true
		}
	}
	return "", false
}

// loopEntries returns the objects in a loop body that no sequence flow enters,
// leaving out notes, which are never on the control flow.
func loopEntries(loop *microflows.LoopedActivity, g *flowGraphIndex) []model.ID {
	if loop.ObjectCollection == nil {
		return nil
	}
	var out []model.ID
	for _, o := range loop.ObjectCollection.Objects {
		if _, isNote := o.(*microflows.Annotation); isNote {
			continue
		}
		if g.incoming[o.GetID()] == 0 {
			out = append(out, o.GetID())
		}
	}
	return out
}

// flowBranchKey names the branch a sequence flow takes out of its origin: the
// error handler, a split's case, or the one plain flow.
func flowBranchKey(f *microflows.SequenceFlow) string {
	prefix := ""
	if f.IsErrorHandler {
		prefix = "error "
	}
	switch c := f.CaseValue.(type) {
	case nil:
		return prefix + "default"
	case microflows.NoCase, *microflows.NoCase:
		return prefix + "default"
	case microflows.EnumerationCase:
		return prefix + "'" + c.Value + "'"
	case *microflows.EnumerationCase:
		return prefix + "'" + c.Value + "'"
	case microflows.BooleanCase:
		return fmt.Sprintf("%s'%t'", prefix, c.Value)
	case *microflows.BooleanCase:
		return fmt.Sprintf("%s'%t'", prefix, c.Value)
	case microflows.ExpressionCase:
		return prefix + "'" + c.Expression + "'"
	case *microflows.ExpressionCase:
		return prefix + "'" + c.Expression + "'"
	case microflows.InheritanceCase:
		return prefix + inheritanceKey(c)
	case *microflows.InheritanceCase:
		return prefix + inheritanceKey(*c)
	}
	return prefix + fmt.Sprintf("%T", f.CaseValue)
}

func inheritanceKey(c microflows.InheritanceCase) string {
	if c.EntityQualifiedName != "" {
		return "'" + c.EntityQualifiedName + "'"
	}
	return "'" + string(c.EntityID) + "'"
}

// layoutKind is what two paired objects must agree on: their node type and, for
// an activity, the action it runs.
func layoutKind(o microflows.MicroflowObject) string {
	k := fmt.Sprintf("%T", o)
	if a, ok := o.(*microflows.ActionActivity); ok && a.Action != nil {
		k += "/" + fmt.Sprintf("%T", a.Action)
	}
	return k
}

func describeLayoutObject(o microflows.MicroflowObject) string {
	if o == nil {
		return "object"
	}
	t := fmt.Sprintf("%T", o)
	if i := strings.LastIndex(t, "."); i >= 0 {
		t = t[i+1:]
	}
	if a, ok := o.(*microflows.ActionActivity); ok && a.Action != nil {
		at := fmt.Sprintf("%T", a.Action)
		if i := strings.LastIndex(at, "."); i >= 0 {
			at = at[i+1:]
		}
		if a.Caption != "" {
			return fmt.Sprintf("%s activity '%s'", at, a.Caption)
		}
		return at + " activity"
	}
	return t
}

// apply patches the plan onto a stored unit document and counts what changed.
// Only keys the stored element already has are rewritten, so the patch never
// invents a property the stored document's Mendix version does not have.
func (plan *layoutPlan) apply(doc bson.D) bson.D {
	plan.movedObjects, plan.changedFlows = 0, 0
	out, _ := plan.patch(doc).(bson.D)
	return out
}

func (plan *layoutPlan) patch(node any) any {
	switch v := node.(type) {
	case bson.D:
		out := make(bson.D, len(v))
		copy(out, v)
		if id, ok := layoutRawID(out); ok {
			if g, ok := plan.objects[normID(id)]; ok {
				changed := setLayoutString(out, "RelativeMiddlePoint", fmt.Sprintf("%d;%d", g.position.X, g.position.Y))
				if g.size != nil {
					changed = setLayoutString(out, "Size", fmt.Sprintf("%d;%d", g.size.Width, g.size.Height)) || changed
				}
				if changed {
					plan.movedObjects++
				}
			} else if g, ok := plan.flows[normID(id)]; ok {
				if plan.patchFlow(out, g) {
					plan.changedFlows++
				}
			}
		}
		for i, e := range out {
			out[i] = bson.E{Key: e.Key, Value: plan.patch(e.Value)}
		}
		return out
	case bson.A:
		out := make(bson.A, len(v))
		for i, item := range v {
			out[i] = plan.patch(item)
		}
		return out
	}
	return node
}

func (plan *layoutPlan) patchFlow(f bson.D, g flowGeometry) bool {
	changed := setLayoutInt(f, "OriginConnectionIndex", g.originIndex)
	changed = setLayoutInt(f, "DestinationConnectionIndex", g.destinationIndex) || changed
	// Mendix 10+ keeps the vectors on a BezierCurve line; older versions on the
	// flow itself.
	for i, e := range f {
		if e.Key != "Line" {
			continue
		}
		if line, ok := e.Value.(bson.D); ok {
			line = append(bson.D(nil), line...)
			changed = setLayoutString(line, "OriginControlVector", g.originVector) || changed
			changed = setLayoutString(line, "DestinationControlVector", g.destVector) || changed
			f[i].Value = line
		}
	}
	changed = setLayoutString(f, "OriginBezierVector", g.originVector) || changed
	changed = setLayoutString(f, "DestinationBezierVector", g.destVector) || changed
	return changed
}

func setLayoutString(d bson.D, key, value string) bool {
	for i, e := range d {
		if e.Key != key {
			continue
		}
		if cur, ok := e.Value.(string); ok && cur == value {
			return false
		}
		d[i].Value = value
		return true
	}
	return false
}

// setLayoutInt rewrites an integer key keeping the stored BSON integer width.
func setLayoutInt(d bson.D, key string, value int) bool {
	for i, e := range d {
		if e.Key != key {
			continue
		}
		switch cur := e.Value.(type) {
		case int32:
			if int(cur) == value {
				return false
			}
			d[i].Value = int32(value)
		case int64:
			if int(cur) == value {
				return false
			}
			d[i].Value = int64(value)
		default:
			d[i].Value = int32(value)
		}
		return true
	}
	return false
}
