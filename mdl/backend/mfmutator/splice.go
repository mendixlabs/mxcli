// SPDX-License-Identifier: Apache-2.0

package mfmutator

// # The graph splice (plan item 4.2b)
//
// An `alter microflow` operation edits the STORED document, never a rebuild of
// it. The rebuild (`UpdateMicroflow`, then re-pairing IDs by type and position)
// is what deleted merges, reset curves and moved $IDs onto other nodes on a
// Studio Pro-authored flow (ADR-0012, Context). So the splice works on the raw
// BSON tree of the unit as read from storage:
//
//   - the fragment's objects are the only elements it builds, and they are
//     appended to the collection that holds the target;
//   - the sequence flows around the target are rewired by changing their
//     pointers, connection sides and the control vector at the rewired end —
//     every other property of a rewired flow (its $ID, case value, error-handler
//     flag, the curve at its other end) is kept;
//   - nodes are moved only to make room, and only by a translation;
//   - nothing else is touched, and no stored $ID is ever rewritten. A removed
//     element leaves no reference behind: Save refuses a document in which any
//     binary still names an element that was removed (CLAUDE.md rule 1), and
//     one in which two elements share an $ID;
//   - an activity replaced by one of the same kind is edited, not replaced:
//     the replacement takes the stored activity's $IDs, and every reference to
//     it is rewritten with them in the same pass (splice_edit.go,
//     ako/mxcli#1761).
//
// What the splice cannot do safely it refuses, with the reason: an insert
// after a decision (which branch?), before an activity several flows enter
// (which path?), a drop of an activity with an error handler, or its replace
// by anything but one activity handling errors the same way (the handler would
// be orphaned; mendixlabs/mxcli#1489).
//
// Inside a loop body the splice inserts, drops and replaces plain activities
// (ako/mxcli#1693): a body node's coordinates are relative to the loop box, so
// room is made among the body's own nodes, and the box grows when the body no
// longer fits (growLoop) — with what is drawn past it moving along, as for an
// insert at top level. Anything else in a body — a decision, a nested loop, a
// return — is a change of the body's shape, which ReplaceLoopBody makes by
// rebuilding the body inside the stored loop.

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Deps supplies the engine-specific steps: turning a new object or flow into
// its stored BSON form, and writing the unit back. Everything between — which
// flows to rewire, where to place, what to remove — is decided here, once, for
// every storage engine.
type Deps interface {
	SerializeObject(obj microflows.MicroflowObject) (bson.D, error)
	SerializeSequenceFlow(f *microflows.SequenceFlow) (bson.D, error)
	SerializeAnnotationFlow(f *microflows.AnnotationFlow) (bson.D, error)
	// SerializeDocument encodes a whole declared document — a
	// *microflows.Microflow or *microflows.Nanoflow — exactly as the rebuild
	// would write it, with every element given an $ID. SetHeader copies its
	// header properties and parameters from it.
	SerializeDocument(declared any) (bson.D, error)
	// SaveUnit writes the patched unit. The implementation must go through the
	// storage engine's reconciling write (canon.Reconcile), like every write.
	SaveUnit(unitID string, contents []byte) error
}

// Mutator splices fragments into one microflow or nanoflow unit.
type Mutator struct {
	deps   Deps
	unitID model.ID
	doc    bson.D
	// removed holds the $IDs of every element an operation took out, so Save
	// can prove that nothing still points at one of them.
	removed map[string]bool
	// pinned are the points the script states a node is drawn at (Pin).
	pinned map[point]bool
	// pinnedIn are the same for the nodes of a loop body, by the loop's $ID,
	// in the loop box's coordinates (PinInLoop).
	pinnedIn map[string]map[point]bool
	// carried maps the $ID a same-kind replacement was built with to the
	// stored $ID it takes over, put on at Bytes (ako/mxcli#1761,
	// splice_edit.go); edited are the stored activities a Replace edited so.
	carried map[string][]byte
	edited  map[string]bool
}

var _ backend.MicroflowMutator = (*Mutator)(nil)

// New returns a Mutator over a decoded Microflows$Microflow or
// Microflows$Nanoflow unit.
func New(doc bson.D, unitID model.ID, deps Deps) (*Mutator, error) {
	switch t := dString(doc, "$Type"); t {
	case "Microflows$Microflow", "Microflows$Nanoflow":
	default:
		return nil, fmt.Errorf("unit %s is a %s, not a microflow or nanoflow", unitID, t)
	}
	if dDoc(doc, "ObjectCollection") == nil {
		return nil, fmt.Errorf("unit %s has no object collection", unitID)
	}
	return &Mutator{deps: deps, unitID: unitID, doc: doc, removed: map[string]bool{},
		carried: map[string][]byte{}, edited: map[string]bool{}}, nil
}

// Bytes returns the patched unit, after the integrity checks Save applies.
// An activity a replace edited in place takes its stored identity back first
// (applyCarried).
func (m *Mutator) Bytes() ([]byte, error) {
	m.applyCarried()
	if err := m.checkIntegrity(); err != nil {
		return nil, err
	}
	return bson.Marshal(m.doc)
}

// Save writes the patched unit through Deps.
func (m *Mutator) Save() error {
	out, err := m.Bytes()
	if err != nil {
		return err
	}
	return m.deps.SaveUnit(string(m.unitID), out)
}

// ---------------------------------------------------------------------------
// The graph view
// ---------------------------------------------------------------------------

// node is one object of the flow as stored.
type node struct {
	id   string // model ID form (types.BlobToUUID of the $ID)
	typ  string
	doc  bson.D
	pos  point
	size point
	// loop is the $ID of the loop whose body holds the node, "" at top level.
	loop string
}

// flowRef is one entry of the unit's Flows list.
type flowRef struct {
	id     string
	typ    string
	doc    bson.D
	origin string
	dest   string
	isErr  bool
}

func (f flowRef) isSequence() bool { return f.typ == "Microflows$SequenceFlow" }

type graph struct {
	nodes map[string]*node
	// order is the nodes in storage order, so a shift visits them the same
	// way every run.
	order []*node
	flows []flowRef
}

func (m *Mutator) graph() *graph {
	g := &graph{nodes: map[string]*node{}}
	var walk func(oc bson.D, loop string)
	walk = func(oc bson.D, loop string) {
		for _, el := range arrayElements(dGet(oc, "Objects")) {
			d, ok := el.(bson.D)
			if !ok {
				continue
			}
			n := &node{
				id:   binaryID(dGet(d, "$ID")),
				typ:  dString(d, "$Type"),
				doc:  d,
				pos:  parsePoint(dString(d, "RelativeMiddlePoint")),
				size: parsePoint(dString(d, "Size")),
				loop: loop,
			}
			if n.id != "" {
				g.nodes[n.id] = n
				g.order = append(g.order, n)
			}
			if n.typ == "Microflows$LoopedActivity" {
				if inner := dDoc(d, "ObjectCollection"); inner != nil {
					walk(inner, n.id)
				}
			}
		}
	}
	walk(dDoc(m.doc, "ObjectCollection"), "")
	for _, el := range arrayElements(dGet(m.doc, "Flows")) {
		d, ok := el.(bson.D)
		if !ok {
			continue
		}
		isErr, _ := dGet(d, "IsErrorHandler").(bool)
		g.flows = append(g.flows, flowRef{
			id:     binaryID(dGet(d, "$ID")),
			typ:    dString(d, "$Type"),
			doc:    d,
			origin: binaryID(dGet(d, "OriginPointer")),
			dest:   binaryID(dGet(d, "DestinationPointer")),
			isErr:  isErr,
		})
	}
	return g
}

// outgoing returns the sequence flows leaving id, split into normal and
// error-handler flows.
func (g *graph) outgoing(id string) (normal, errs []flowRef) {
	for _, f := range g.flows {
		if f.isSequence() && f.origin == id {
			if f.isErr {
				errs = append(errs, f)
			} else {
				normal = append(normal, f)
			}
		}
	}
	return normal, errs
}

// incoming returns the sequence flows entering id.
func (g *graph) incoming(id string) []flowRef {
	var out []flowRef
	for _, f := range g.flows {
		if f.isSequence() && f.dest == id {
			out = append(out, f)
		}
	}
	return out
}

// annotationFlows returns the annotation flows attached to id.
func (g *graph) annotationFlows(id string) []flowRef {
	var out []flowRef
	for _, f := range g.flows {
		if !f.isSequence() && (f.origin == id || f.dest == id) {
			out = append(out, f)
		}
	}
	return out
}

// node returns the stored top-level node id. A node inside a loop body is
// refused: the operations that call it set what a node at top level is (where
// it is drawn, a decision's condition, an end event's value or notes), and
// inside a body they would have to be laid out against the loop box.
func (g *graph) node(id model.ID) (*node, error) {
	n, err := g.anyNode(id)
	if err != nil {
		return nil, err
	}
	if n.loop != "" {
		return nil, fmt.Errorf("%s is inside a loop body, where the splice inserts, drops and replaces plain "+
			"activities only; replace the loop's body instead: `replace body of <loop> with begin … end;`, "+
			"addressing the loop by its handle from `describe … with handles`", describeNode(n))
	}
	return n, nil
}

// anyNode returns the stored node id, at top level or inside a loop body.
func (g *graph) anyNode(id model.ID) (*node, error) {
	n, ok := g.nodes[string(id)]
	if !ok {
		return nil, fmt.Errorf("activity %s is not in the stored flow (was it dropped by an earlier operation?)", id)
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Operations
// ---------------------------------------------------------------------------

// InsertAfter splices frag onto the flow leaving target.
func (m *Mutator) InsertAfter(target model.ID, frag *backend.MicroflowFragment) error {
	g := m.graph()
	x, err := g.anyNode(target)
	if err != nil {
		return err
	}
	if err := inBodyFragment(x, frag); err != nil {
		return err
	}
	normal, _ := g.outgoing(x.id)
	switch {
	case len(normal) == 0 && x.loop != "":
		// The last node of a loop body: the iteration ends there, so the
		// fragment goes after it and ends the iteration instead.
		return m.appendInLoop(g, x, frag)
	case len(normal) == 0:
		return fmt.Errorf("nothing follows %s: it ends the flow; insert before it instead", describeNode(x))
	case len(normal) > 1:
		return fmt.Errorf("%s has %d outgoing flows (it is a decision): insert after it would not say which branch; "+
			"insert before the first activity of the branch instead", describeNode(x), len(normal))
	}
	return m.spliceOnFlow(g, normal[0], frag)
}

// InsertBefore splices frag onto the flow entering target.
func (m *Mutator) InsertBefore(target model.ID, frag *backend.MicroflowFragment) error {
	g := m.graph()
	y, err := g.anyNode(target)
	if err != nil {
		return err
	}
	if err := inBodyFragment(y, frag); err != nil {
		return err
	}
	in := g.incoming(y.id)
	switch {
	case len(in) == 0 && y.loop != "":
		// The first node of a loop body: an iteration starts there, so it
		// starts at the fragment instead.
		return m.prependInLoop(g, y, frag)
	case len(in) == 0:
		return fmt.Errorf("no flow enters %s; there is no path to insert on", describeNode(y))
	case len(in) > 1:
		return fmt.Errorf("%d flows enter %s: insert before it would not say on which path; "+
			"insert after one of its predecessors instead", len(in), describeNode(y))
	}
	return m.spliceOnFlow(g, in[0], frag)
}

// spliceOnFlow puts frag on flow f (origin X, destination Y): f keeps its
// origin end and now enters the fragment's entry; a new flow leaves the
// fragment's exit and enters Y where f used to.
func (m *Mutator) spliceOnFlow(g *graph, f flowRef, frag *backend.MicroflowFragment) error {
	x, y := g.nodes[f.origin], g.nodes[f.dest]
	if x == nil || y == nil {
		return fmt.Errorf("flow %s points at an object that is not in the flow", f.id)
	}
	if x.loop != y.loop {
		return fmt.Errorf("flow %s runs between a loop body and the rest of the flow", f.id)
	}
	// Inside a loop body the fragment is laid out among the body's own nodes,
	// in the loop box's coordinates, and the box is fitted to it afterwards.
	scope := x.loop
	before := m.contentBounds(scope)
	fb, err := fragmentGeometry(frag)
	if err != nil {
		return err
	}
	var entrySide, exitSide int
	if frag.Placed {
		// The script stated where the fragment goes: it stays where the
		// builder put it, nothing is moved to make room, and each new end
		// faces the node it connects to.
		if entrySide, exitSide, err = placedSides(frag, x, y); err != nil {
			return err
		}
	} else if entrySide, exitSide, err = m.makeRoom(g, fb, f, x, y); err != nil {
		return err
	}
	destIdx, destVec := flowEnd(f.doc, "Destination")
	if err := m.addFragmentIn(frag, scope); err != nil {
		return err
	}
	// f now enters the fragment.
	setPointer(f.doc, "DestinationPointer", frag.Entry)
	setInt(f.doc, "DestinationConnectionIndex", entrySide)
	setVector(f.doc, "DestinationControlVector", sideVector(entrySide))
	if c := frag.InCurve; c != nil {
		setVector(f.doc, "OriginControlVector", c.Origin)
		setVector(f.doc, "DestinationControlVector", c.Destination)
	}
	// And a new flow carries on from the fragment to Y.
	outOrigin, outDest := sideVector(exitSide), destVec
	if c := frag.OutCurve; c != nil {
		outOrigin, outDest = c.Origin, c.Destination
	}
	if err := m.addFlow(&microflows.SequenceFlow{
		BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:                   frag.Exit,
		DestinationID:              model.ID(y.id),
		OriginConnectionIndex:      exitSide,
		DestinationConnectionIndex: destIdx,
		OriginControlVector:        outOrigin,
		DestinationControlVector:   outDest,
		CaseValue:                  frag.ExitCase,
	}); err != nil {
		return err
	}
	return m.fitLoopTo(g, scope, before)
}

// makeRoom places the fragment in the gap between x and y, moving everything
// past the gap along when it does not fit, and returns the sides its new ends
// connect at.
func (m *Mutator) makeRoom(g *graph, fb *fragmentBox, f flowRef, x, y *node) (entrySide, exitSide int, err error) {
	ax, s, err := flowAxis(x, y)
	if err != nil {
		return 0, 0, err
	}

	// Make room: the fragment plus a gap on either side has to fit between
	// X's and Y's facing edges; what is missing is added by moving everything
	// past the midpoint of that gap further along the axis.
	a := x.pos.get(ax) + s*x.size.get(ax)/2
	b := y.pos.get(ax) - s*y.size.get(ax)/2
	need := fb.length(ax) + 2*minGap
	scope := x.loop
	if deficit := need - s*(b-a); deficit > 0 {
		cut := float64(a+b) / 2
		if p := m.pinnedPast(g, scope, ax, s, cut, ""); p != nil {
			blocked := ", and making it would move " + describeNode(p) + ","
			if p == y {
				blocked = ","
			}
			return 0, 0, fmt.Errorf("the inserted statements need %dpx more room than there is between %s and %s%s "+
				"which the script draws where it is; %s", deficit, describeNode(x), describeNode(y), blocked, roomAdvice)
		}
		m.shiftIn(g, scope, ax, s, cut, s*deficit, "")
		b += s * deficit
	}

	// Place the fragment centred in the gap, its entry level with the flow.
	cross := ax.other()
	mid := point{}
	mid = mid.set(ax, (a+b)/2)
	mid = mid.set(cross, (x.pos.get(cross)+y.pos.get(cross))/2)
	fb.placeCentred(ax, mid)
	if err := fb.checkRoomIn(g, scope, "", f.id); err != nil {
		return 0, 0, err
	}
	return side(ax, -s), side(ax, s), nil
}

// placedSides returns the sides a placed fragment's new flow ends connect at:
// its entry's side facing x, which the flow into it comes from, and its exit's
// side facing y, which the flow out of it goes to.
func placedSides(frag *backend.MicroflowFragment, x, y *node) (entrySide, exitSide int, err error) {
	entry, exit := fragmentNode(frag, frag.Entry), fragmentNode(frag, frag.Exit)
	if entry == nil || exit == nil {
		return 0, 0, fmt.Errorf("the fragment's entry or exit is not one of its objects")
	}
	ax, s, err := flowAxis(x, entry)
	if err != nil {
		return 0, 0, err
	}
	ax2, s2, err := flowAxis(exit, y)
	if err != nil {
		return 0, 0, err
	}
	return side(ax, -s), side(ax2, s2), nil
}

// fragmentNode is a fragment object seen as a node of the graph, for geometry.
func fragmentNode(frag *backend.MicroflowFragment, id model.ID) *node {
	for _, obj := range frag.Objects {
		if obj.GetID() == id {
			p := obj.GetPosition()
			return &node{id: string(id), typ: strings.TrimPrefix(fmt.Sprintf("%T", obj), "*microflows."),
				pos: point{p.X, p.Y}, size: objectSize(obj)}
		}
	}
	return nil
}

// Replace puts frag where target is: every flow that entered target enters
// the fragment, the flow that left it leaves the fragment's exit, and
// annotations attached to target are attached to the fragment's entry.
func (m *Mutator) Replace(target model.ID, frag *backend.MicroflowFragment) error {
	g := m.graph()
	if n, ok := g.nodes[string(target)]; ok && n.loop != "" {
		if err := inBodyFragment(n, frag); err != nil {
			return err
		}
		return m.replaceInLoop(g, n, frag)
	}
	x, err := g.node(target)
	if err != nil {
		return err
	}
	if isDecision(x) {
		return m.replaceDecision(g, x, frag)
	}
	if _, errs := g.outgoing(x.id); len(errs) > 0 && x.typ == "Microflows$ActionActivity" {
		return m.replaceKeepingHandler(g, x, frag)
	}
	out, err := removable(g, x, "replace")
	if err != nil {
		return err
	}
	// One activity of the same kind is an edit of the stored one, which keeps
	// its identity (ako/mxcli#1761).
	if edited, err := m.editInPlace(g, x, frag); edited || err != nil {
		return err
	}
	y := g.nodes[out.dest]
	if y == nil {
		return fmt.Errorf("flow %s points at an object that is not in the flow", out.id)
	}
	fb, err := fragmentGeometry(frag)
	if err != nil {
		return err
	}
	// A placed fragment stays where the script stated (ako/mxcli#818).
	if !frag.Placed {
		ax, s, err := flowAxis(x, y)
		if err != nil {
			return err
		}
		// The fragment starts where target started; if it is longer, everything
		// past target moves along by the difference.
		near := x.pos.get(ax) - s*x.size.get(ax)/2
		if extra := fb.length(ax) - x.size.get(ax); extra > 0 {
			cut := float64(x.pos.get(ax))
			if p := m.pinnedPast(g, "", ax, s, cut, x.id); p != nil {
				return fmt.Errorf("the replacing statements are %dpx longer than %s, and making room for them would move %s, "+
					"which the script draws where it is; %s", extra, describeNode(x), describeNode(p), roomAdvice)
			}
			m.shift(g, ax, s, cut, s*extra, x.id)
		}
		centre := point{}
		centre = centre.set(ax, near+s*fb.length(ax)/2)
		centre = centre.set(ax.other(), x.pos.get(ax.other()))
		fb.placeCentred(ax, centre)
		if err := fb.checkRoom(g, x.id, ""); err != nil {
			return err
		}
	}

	if err := m.addFragment(frag); err != nil {
		return err
	}
	for _, in := range g.incoming(x.id) {
		setPointer(in.doc, "DestinationPointer", frag.Entry)
	}
	setPointer(out.doc, "OriginPointer", frag.Exit)
	if frag.ExitCase != nil {
		if err := m.setCase(out.doc, frag.ExitCase); err != nil {
			return err
		}
	}
	for _, af := range g.annotationFlows(x.id) {
		key := "DestinationPointer"
		if af.origin == x.id {
			key = "OriginPointer"
		}
		setPointer(af.doc, key, frag.Entry)
	}
	m.removeFlows(g.bodyFlows(x.id))
	return m.removeObject(x)
}

// replaceInLoop replaces an activity inside a loop body. By one activity it is
// swapped in place (ako/mxcli#1106): the new activity takes the stored one's
// place in the loop's collection, is drawn where it was at the size it was,
// and the flows that entered and left it now enter and leave the new one, so
// nothing else in the body moves; of the same kind, it also takes the stored
// one's identity (ako/mxcli#1761). By several (ako/mxcli#1693) the fragment
// starts where the activity started, what is past it in the body moves along
// when the fragment is longer, and the loop box grows when the body no longer
// fits it. The activity may be the body's first (no flow in) or last (no flow
// out): the fragment then starts or ends the iteration in its place.
func (m *Mutator) replaceInLoop(g *graph, x *node, frag *backend.MicroflowFragment) error {
	if x.typ != "Microflows$ActionActivity" {
		return fmt.Errorf("cannot replace %s inside a loop body: only an activity can be", describeNode(x))
	}
	// The body's last activity ends it, with no flow out.
	normal, errs := g.outgoing(x.id)
	if len(errs) > 0 {
		return fmt.Errorf("cannot replace %s: it has an error handler, which would be left with no activity; "+
			"replace the loop's body instead", describeNode(x))
	}
	if len(normal) > 1 {
		return fmt.Errorf("cannot replace %s: it has %d outgoing flows, not one", describeNode(x), len(normal))
	}
	loop := g.nodes[x.loop]
	if loop == nil {
		return fmt.Errorf("%s is in a loop that is not in the flow", describeNode(x))
	}
	if singleActivity(frag) {
		if d, err := m.serializeInPlace(x, frag); err != nil {
			return err
		} else if dString(d, "$Type") == x.typ {
			return m.swapKeeping(g, x, dDoc(loop.doc, "ObjectCollection"), d, frag.Entry)
		}
	}
	in := g.incoming(x.id)
	before := m.contentBounds(x.loop)
	fb, err := fragmentGeometry(frag)
	if err != nil {
		return err
	}
	if !frag.Placed {
		ax, s := bodyAxis(g, x, normal, in)
		// The fragment starts where x started; if it is longer, what is past
		// x in the body moves along by the difference.
		near := x.pos.get(ax) - s*x.size.get(ax)/2
		if extra := fb.length(ax) - x.size.get(ax); extra > 0 {
			m.shiftIn(g, x.loop, ax, s, float64(x.pos.get(ax)), s*extra, x.id)
		}
		centre := point{}
		centre = centre.set(ax, near+s*fb.length(ax)/2)
		centre = centre.set(ax.other(), x.pos.get(ax.other()))
		fb.placeCentred(ax, centre)
		if err := fb.checkRoomIn(g, x.loop, x.id, ""); err != nil {
			return err
		}
	}
	if err := m.addFragmentIn(frag, x.loop); err != nil {
		return err
	}
	for _, f := range in {
		setPointer(f.doc, "DestinationPointer", frag.Entry)
	}
	if len(normal) == 1 {
		setPointer(normal[0].doc, "OriginPointer", frag.Exit)
	}
	for _, af := range g.annotationFlows(x.id) {
		key := "DestinationPointer"
		if af.origin == x.id {
			key = "OriginPointer"
		}
		setPointer(af.doc, key, frag.Entry)
	}
	if err := m.removeObject(x); err != nil {
		return err
	}
	return m.fitLoopTo(g, x.loop, before)
}

// inBodyFragment refuses to put into a loop body a fragment that is not a run
// of plain activities: one with a decision, a loop, an end or error event, an
// error handler or a case on its exit. Such a fragment changes the body's
// shape — where its paths split, end or rejoin — which the splice does not lay
// out inside a loop box; ReplaceLoopBody rebuilds the body instead. The
// executor refuses the same fragments first, with the statement that makes the
// change; this is the engine's own guard.
func inBodyFragment(x *node, frag *backend.MicroflowFragment) error {
	if x.loop == "" || frag == nil {
		return nil
	}
	if frag.ExitCase != nil {
		return fmt.Errorf("%s is inside a loop body, where the splice puts plain activities only, not a decision; "+
			"replace the loop's body instead", describeNode(x))
	}
	for _, obj := range frag.Objects {
		switch obj.(type) {
		case *microflows.ActionActivity, *microflows.Annotation:
		default:
			return fmt.Errorf("%s is inside a loop body, where the splice puts plain activities only, not a %s; "+
				"replace the loop's body instead", describeNode(x), strings.TrimPrefix(fmt.Sprintf("%T", obj), "*microflows."))
		}
	}
	for _, f := range frag.Flows {
		if f.IsErrorHandler {
			return fmt.Errorf("%s is inside a loop body, where the splice puts plain activities only, not one with an "+
				"error handler; replace the loop's body instead", describeNode(x))
		}
	}
	return nil
}

// bodyAxis is the direction a loop body runs through x: along the flow that
// leaves it, else the one that enters it, else left to right (x is the body's
// only node).
func bodyAxis(g *graph, x *node, out, in []flowRef) (axis, int) {
	if len(out) > 0 {
		if y := g.nodes[out[0].dest]; y != nil {
			if ax, s, err := flowAxis(x, y); err == nil {
				return ax, s
			}
		}
	}
	if len(in) > 0 {
		if w := g.nodes[in[0].origin]; w != nil {
			if ax, s, err := flowAxis(w, x); err == nil {
				return ax, s
			}
		}
	}
	return axisX, 1
}

// appendInLoop puts frag after x, the last node of a loop body: x gets a flow
// into the fragment, whose exit ends the iteration now. The fragment is drawn
// past x along the direction the body runs, and the loop box is fitted to it.
func (m *Mutator) appendInLoop(g *graph, x *node, frag *backend.MicroflowFragment) error {
	if x.typ != "Microflows$ActionActivity" && x.typ != "Microflows$LoopedActivity" {
		return fmt.Errorf("%s ends the loop body, and only an activity or a loop can be followed there", describeNode(x))
	}
	before := m.contentBounds(x.loop)
	fb, err := fragmentGeometry(frag)
	if err != nil {
		return err
	}
	ax, s := bodyAxis(g, x, nil, g.incoming(x.id))
	originSide, entrySide := side(ax, s), side(ax, -s)
	if frag.Placed {
		entry := fragmentNode(frag, frag.Entry)
		if entry == nil {
			return fmt.Errorf("the fragment's entry is not one of its objects")
		}
		pax, ps, err := flowAxis(x, entry)
		if err != nil {
			return err
		}
		originSide, entrySide = side(pax, ps), side(pax, -ps)
	} else {
		centre := point{}
		centre = centre.set(ax, x.pos.get(ax)+s*(x.size.get(ax)/2+minGap+fb.length(ax)/2))
		centre = centre.set(ax.other(), x.pos.get(ax.other()))
		fb.placeCentred(ax, centre)
		if err := fb.checkRoomIn(g, x.loop, "", ""); err != nil {
			return err
		}
	}
	if err := m.addFragmentIn(frag, x.loop); err != nil {
		return err
	}
	if err := m.addFlow(&microflows.SequenceFlow{
		BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:                   model.ID(x.id),
		DestinationID:              frag.Entry,
		OriginConnectionIndex:      originSide,
		DestinationConnectionIndex: entrySide,
		OriginControlVector:        sideVector(originSide),
		DestinationControlVector:   sideVector(entrySide),
	}); err != nil {
		return err
	}
	return m.fitLoopTo(g, x.loop, before)
}

// prependInLoop puts frag before y, the first node of a loop body: the
// fragment starts the iteration now, and its exit gets a flow into y. y and
// everything past its near edge move along to make room, and the loop box is
// fitted to the body.
func (m *Mutator) prependInLoop(g *graph, y *node, frag *backend.MicroflowFragment) error {
	before := m.contentBounds(y.loop)
	fb, err := fragmentGeometry(frag)
	if err != nil {
		return err
	}
	out, _ := g.outgoing(y.id)
	ax, s := bodyAxis(g, y, out, nil)
	exitSide, destSide := side(ax, s), side(ax, -s)
	if frag.Placed {
		exit := fragmentNode(frag, frag.Exit)
		if exit == nil {
			return fmt.Errorf("the fragment's exit is not one of its objects")
		}
		pax, ps, err := flowAxis(exit, y)
		if err != nil {
			return err
		}
		exitSide, destSide = side(pax, ps), side(pax, -ps)
	} else {
		near := y.pos.get(ax) - s*y.size.get(ax)/2
		m.shiftIn(g, y.loop, ax, s, float64(near)-float64(s)/2, s*(fb.length(ax)+minGap), "")
		centre := point{}
		centre = centre.set(ax, near+s*fb.length(ax)/2)
		centre = centre.set(ax.other(), y.pos.get(ax.other()))
		fb.placeCentred(ax, centre)
		if err := fb.checkRoomIn(g, y.loop, "", ""); err != nil {
			return err
		}
	}
	if err := m.addFragmentIn(frag, y.loop); err != nil {
		return err
	}
	if err := m.addFlow(&microflows.SequenceFlow{
		BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:                   frag.Exit,
		DestinationID:              model.ID(y.id),
		OriginConnectionIndex:      exitSide,
		DestinationConnectionIndex: destSide,
		OriginControlVector:        sideVector(exitSide),
		DestinationControlVector:   sideVector(destSide),
	}); err != nil {
		return err
	}
	return m.fitLoopTo(g, y.loop, before)
}

// dropInLoop removes the activity x from a loop body. Between two nodes it is
// a drop like any other: the flows that entered it enter its successor. As the
// body's first node its successor starts the iteration; as its last, its
// predecessor ends it, which only an activity or a loop can (a decision would
// be left with a path to nowhere); as its only node the body is left empty,
// which Mendix accepts (mx check 0 errors on 11.13). Nothing else in the body
// moves: the gap stays, as it does at top level.
func (m *Mutator) dropInLoop(g *graph, x *node) error {
	if x.typ != "Microflows$ActionActivity" {
		return fmt.Errorf("cannot drop %s from a loop body: only an activity can be, because a decision, a loop "+
			"or an event changes the shape of the body; replace the loop's body instead", describeNode(x))
	}
	normal, errs := g.outgoing(x.id)
	if len(errs) > 0 {
		return fmt.Errorf("cannot drop %s: it has an error handler, which would be left with no activity; "+
			"replace the loop's body instead", describeNode(x))
	}
	in := g.incoming(x.id)
	drop := map[string]bool{}
	switch {
	case len(normal) > 1:
		return fmt.Errorf("cannot drop %s: it has %d outgoing flows, not one", describeNode(x), len(normal))
	case len(normal) == 1 && len(in) > 0:
		out := normal[0]
		destIdx, destVec := flowEnd(out.doc, "Destination")
		destPtr := dGet(out.doc, "DestinationPointer")
		for _, f := range in {
			if f.origin == out.dest {
				return fmt.Errorf("dropping %s would leave a flow from an object to itself", describeNode(x))
			}
		}
		for _, f := range in {
			setBinary(f.doc, "DestinationPointer", destPtr)
			setInt(f.doc, "DestinationConnectionIndex", destIdx)
			setVector(f.doc, "DestinationControlVector", destVec)
		}
		drop[out.id] = true
	case len(normal) == 1:
		// The first node: its successor starts the iteration, which only a
		// node no other flow enters can.
		out := normal[0]
		if n := len(g.incoming(out.dest)); n != 1 {
			return fmt.Errorf("cannot drop %s, the first activity of the loop body: %d flows enter the node after it, "+
				"which would then start the body", describeNode(x), n)
		}
		drop[out.id] = true
	case len(in) > 1:
		return fmt.Errorf("cannot drop %s, the last activity of the loop body: %d flows enter it, and each path "+
			"would then end where it came from", describeNode(x), len(in))
	case len(in) == 1:
		// The last node: its predecessor ends the iteration.
		w := g.nodes[in[0].origin]
		if w == nil {
			return fmt.Errorf("flow %s points at an object that is not in the flow", in[0].id)
		}
		wOut, _ := g.outgoing(w.id)
		if (w.typ != "Microflows$ActionActivity" && w.typ != "Microflows$LoopedActivity") || len(wOut) != 1 {
			return fmt.Errorf("cannot drop %s, the last activity of the loop body: %s before it would then end the "+
				"body, and only an activity or a loop can; replace the loop's body instead", describeNode(x), describeNode(w))
		}
		drop[in[0].id] = true
	}
	for _, af := range g.annotationFlows(x.id) {
		drop[af.id] = true
	}
	m.removeFlows(drop)
	return m.removeObject(x)
}

// replaceKeepingHandler replaces a top-level activity that has a custom error
// handler by one activity handling errors the same way, in place
// (mendixlabs/mxcli#1489): the new activity takes the stored one's place, is
// drawn where it was at the size it was, and every flow that entered or left
// it — its error flow included — now enters or leaves the new one; of the
// same kind, it takes the stored one's identity too (ako/mxcli#1761). The
// handler's own activities, and the merge its path rejoins at, are not
// touched: changing one of them is a change of its own, addressed by its own
// handle. Anything else — more than one activity, or one that does not raise
// to a custom handler of the same kind — would leave the stored handler with
// no activity, and is refused.
func (m *Mutator) replaceKeepingHandler(g *graph, x *node, frag *backend.MicroflowFragment) error {
	// The notes the replacement states come with it (a create or modify
	// states every activity's notes; the stored ones were removed first).
	frag, notes := withoutNotes(frag)
	if !singleActivity(frag) {
		return fmt.Errorf("cannot replace %s: it has an error handler, which the splice keeps only when the activity "+
			"is replaced by one activity handling errors the same way (the handler's own activities are changed by "+
			"their own handles); to change or take out the handler itself, drop the microflow and create it in the "+
			"same script", describeNode(x))
	}
	stored := storedErrorHandling(x.doc)
	got := microflows.ObjectErrorHandlingType(frag.Objects[0])
	switch {
	case !microflows.IsCustomErrorHandling(got):
		return fmt.Errorf("cannot replace %s: it has an error handler (%s), and the replacement states none, so the "+
			"handler would be left with no activity; state the handler on the replacement as `describe` prints it",
			describeNode(x), errorHandlingClause(stored))
	case string(got) != stored:
		return fmt.Errorf("cannot replace %s: its error handler is `%s`, and the replacement's is `%s`; the splice keeps "+
			"the stored handler, so state it as `describe` prints it", describeNode(x), errorHandlingClause(stored),
			errorHandlingClause(string(got)))
	}
	d, err := m.serializeInPlace(x, frag)
	if err != nil {
		return err
	}
	if t := dString(d, "$Type"); t != x.typ {
		return fmt.Errorf("cannot replace %s by a %s: it has an error handler, which the splice keeps only on an "+
			"activity", describeNode(x), t)
	}
	if err := m.swapKeeping(g, x, dDoc(m.doc, "ObjectCollection"), d, frag.Entry); err != nil {
		return err
	}
	// The notes move with the activity, from where the builder drew it to
	// where the stored one is.
	return m.addNotesAt(notes, x.pos)
}

// fragmentNotes are the notes a one-activity fragment attaches to its
// activity: the note objects and their lines, as a fragment of their own.
type fragmentNotes struct {
	frag *backend.MicroflowFragment
	// entryAt is where the builder drew the activity the notes are placed
	// relative to.
	entryAt model.Point
}

// withoutNotes splits frag into the fragment without its notes and the notes,
// when every object but the entry is a note and every note line runs between
// a note and the entry. Any other fragment is returned whole, with no notes.
func withoutNotes(frag *backend.MicroflowFragment) (*backend.MicroflowFragment, *fragmentNotes) {
	if frag == nil || len(frag.AnnotationFlows) == 0 {
		return frag, nil
	}
	rest := *frag
	rest.Objects, rest.AnnotationFlows = nil, nil
	notes := &backend.MicroflowFragment{Pinned: frag.Pinned}
	var entryAt model.Point
	isNote := map[model.ID]bool{}
	for _, obj := range frag.Objects {
		if _, ok := obj.(*microflows.Annotation); ok {
			notes.Objects = append(notes.Objects, obj)
			isNote[obj.GetID()] = true
			continue
		}
		if obj.GetID() == frag.Entry {
			entryAt = obj.GetPosition()
		}
		rest.Objects = append(rest.Objects, obj)
	}
	for _, af := range frag.AnnotationFlows {
		if !(isNote[af.OriginID] && af.DestinationID == frag.Entry) && !(isNote[af.DestinationID] && af.OriginID == frag.Entry) {
			return frag, nil
		}
		notes.AnnotationFlows = append(notes.AnnotationFlows, af)
	}
	return &rest, &fragmentNotes{frag: notes, entryAt: entryAt}
}

// singleActivity reports whether frag is one object and nothing else: no
// flows, no notes, no case on its exit.
func singleActivity(frag *backend.MicroflowFragment) bool {
	return frag != nil && len(frag.Objects) == 1 && len(frag.Flows) == 0 && len(frag.AnnotationFlows) == 0 &&
		frag.ExitCase == nil && frag.Entry == frag.Objects[0].GetID() && frag.Exit == frag.Entry
}

// serializeInPlace encodes frag's one object as the replacement of x: drawn
// where x is, at x's size (a Studio Pro activity may have been resized).
func (m *Mutator) serializeInPlace(x *node, frag *backend.MicroflowFragment) (bson.D, error) {
	obj := frag.Objects[0]
	obj.SetPosition(model.Point{X: x.pos.X, Y: x.pos.Y})
	d, err := m.deps.SerializeObject(obj)
	if err != nil {
		return nil, fmt.Errorf("serialize %T: %w", obj, err)
	}
	if d == nil {
		return nil, fmt.Errorf("cannot write a %T into a flow", obj)
	}
	dSet(d, "RelativeMiddlePoint", x.pos.String())
	dSet(d, "Size", x.size.String())
	return d, nil
}

// swapInPlace puts d where x is in the collection oc, and points every flow
// that entered or left x — normal, error or annotation — at d's $ID, id.
// Nothing else moves.
func (m *Mutator) swapInPlace(g *graph, x *node, oc bson.D, d bson.D, id model.ID) error {
	objs := arrayElements(dGet(oc, "Objects"))
	found := false
	for i, el := range objs {
		if od, ok := el.(bson.D); ok && binaryID(dGet(od, "$ID")) == x.id {
			objs[i], found = d, true
			break
		}
	}
	if !found || !setArray(oc, "Objects", objs) {
		return fmt.Errorf("%s is not in its collection", describeNode(x))
	}
	m.markRemoved(x.doc)
	for _, f := range g.flows {
		switch {
		case f.origin == x.id:
			setPointer(f.doc, "OriginPointer", id)
		case f.dest == x.id:
			setPointer(f.doc, "DestinationPointer", id)
		}
	}
	return nil
}

// storedErrorHandling is the error handling a stored activity's action has.
func storedErrorHandling(d bson.D) string {
	if a := dDoc(d, "Action"); a != nil {
		if eh := dString(a, "ErrorHandlingType"); eh != "" {
			return eh
		}
	}
	return dString(d, "ErrorHandlingType")
}

// errorHandlingClause is how MDL spells an error handling type.
func errorHandlingClause(eh string) string {
	switch microflows.ErrorHandlingType(eh) {
	case microflows.ErrorHandlingTypeCustom:
		return "on error begin … end error"
	case microflows.ErrorHandlingTypeCustomWithoutRollback:
		return "on error without rollback begin … end error"
	case microflows.ErrorHandlingTypeContinue:
		return "on error continue"
	case "", microflows.ErrorHandlingTypeRollback, microflows.ErrorHandlingTypeAbort:
		return "no on error clause"
	}
	return eh
}

// setCase gives a stored flow the case value cv, in the form the engine
// writes it: the case properties of a flow serialized with cv replace the
// stored ones, and the flow keeps its $ID and everything else. The stored
// case element (a NoCase) is taken out with them.
func (m *Mutator) setCase(d bson.D, cv microflows.CaseValue) error {
	tmp, err := m.deps.SerializeSequenceFlow(&microflows.SequenceFlow{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		CaseValue:   cv,
	})
	if err != nil {
		return fmt.Errorf("serialize case value: %w", err)
	}
	set := false
	for _, key := range []string{"CaseValues", "NewCaseValue"} {
		v := dGet(tmp, key)
		if v == nil || dGet(d, key) == nil {
			continue
		}
		m.markRemoved(dGet(d, key))
		dSet(d, key, v)
		set = true
	}
	if !set {
		return fmt.Errorf("the flow leaving the fragment needs a case value, and the stored flow %s has nowhere to keep one",
			binaryID(dGet(d, "$ID")))
	}
	return nil
}

// RemoveNotes takes out the annotations attached to target, with their
// lines. A note also attached to another object is refused: it would be taken
// from that object too.
func (m *Mutator) RemoveNotes(target model.ID) error {
	g := m.graph()
	// An activity in a loop body has notes like any other (ako/mxcli#1693):
	// the note sits in the loop's collection, its line in the unit's flows.
	x, err := g.anyNode(target)
	if err != nil {
		return err
	}
	drop, seen := map[string]bool{}, map[string]bool{}
	var notes []*node
	for _, af := range g.annotationFlows(x.id) {
		other := af.origin
		if other == x.id {
			other = af.dest
		}
		n := g.nodes[other]
		if n == nil || n.typ != "Microflows$Annotation" {
			return fmt.Errorf("the annotation line %s of %s does not lead to an annotation", af.id, describeNode(x))
		}
		for _, f := range g.annotationFlows(n.id) {
			if f.origin != x.id && f.dest != x.id {
				return fmt.Errorf("a note on %s is also attached to another object; changing it here would change it there too — "+
					"edit that note in Studio Pro", describeNode(x))
			}
			drop[f.id] = true
		}
		if !seen[n.id] {
			seen[n.id] = true
			notes = append(notes, n)
		}
	}
	m.removeFlows(drop)
	for _, n := range notes {
		if err := m.removeObject(n); err != nil {
			return err
		}
	}
	return nil
}

// AddNotes attaches the notes frag holds to target, in place: target, its
// flows and every other element stay. frag is one stand-in object — what the
// builder drew the notes against — and the notes and their lines to it; each
// unpinned note is moved by the stand-in's offset from target, a pinned one
// stays where the script put it, and each line is pointed at target.
func (m *Mutator) AddNotes(target model.ID, frag *backend.MicroflowFragment) error {
	g := m.graph()
	x, err := g.node(target)
	if err != nil {
		return err
	}
	rest, notes := withoutNotes(frag)
	if notes == nil || len(rest.Objects) != 1 || rest.Objects[0].GetID() != frag.Entry || len(rest.Flows) > 0 {
		return fmt.Errorf("the notes for %s are not a set of notes attached to one object", describeNode(x))
	}
	for _, n := range notes.frag.Objects {
		if !notes.frag.Pinned[n.GetID()] {
			p := n.GetPosition()
			n.SetPosition(model.Point{X: p.X + x.pos.X - notes.entryAt.X, Y: p.Y + x.pos.Y - notes.entryAt.Y})
		}
	}
	for _, af := range notes.frag.AnnotationFlows {
		if af.OriginID == frag.Entry {
			af.OriginID = model.ID(x.id)
		}
		if af.DestinationID == frag.Entry {
			af.DestinationID = model.ID(x.id)
		}
	}
	return m.addFragment(notes.frag)
}

// SetFreeNotes replaces the flow's free notes — top-level annotations no line
// attaches to anything — by the free notes frag holds, written where the
// builder drew them. Every other element stays.
func (m *Mutator) SetFreeNotes(frag *backend.MicroflowFragment) error {
	g := m.graph()
	wired := map[string]bool{}
	for _, f := range g.flows {
		if !f.isSequence() {
			wired[f.origin], wired[f.dest] = true, true
		}
	}
	for _, n := range g.order {
		if n.typ == "Microflows$Annotation" && n.loop == "" && !wired[n.id] {
			if err := m.removeObject(n); err != nil {
				return err
			}
		}
	}
	if frag == nil {
		return nil
	}
	attached := map[model.ID]bool{}
	for _, af := range frag.AnnotationFlows {
		attached[af.OriginID], attached[af.DestinationID] = true, true
	}
	notes := &backend.MicroflowFragment{}
	for _, obj := range frag.Objects {
		if _, ok := obj.(*microflows.Annotation); ok && !attached[obj.GetID()] {
			notes.Objects = append(notes.Objects, obj)
		}
	}
	return m.addFragment(notes)
}

// Drop removes target and joins the flows that entered it to the object it
// led to. Annotation lines attached to it go with it; the annotations stay.
func (m *Mutator) Drop(target model.ID) error {
	g := m.graph()
	x, err := g.anyNode(target)
	if err != nil {
		return err
	}
	if x.loop != "" {
		return m.dropInLoop(g, x)
	}
	if isDecision(x) {
		return m.dropDecision(g, x)
	}
	out, err := removable(g, x, "drop")
	if err != nil {
		return err
	}
	destIdx, destVec := flowEnd(out.doc, "Destination")
	destPtr := dGet(out.doc, "DestinationPointer")
	for _, in := range g.incoming(x.id) {
		if in.origin == out.dest {
			return fmt.Errorf("dropping %s would leave a flow from an object to itself", describeNode(x))
		}
	}
	for _, in := range g.incoming(x.id) {
		setBinary(in.doc, "DestinationPointer", destPtr)
		setInt(in.doc, "DestinationConnectionIndex", destIdx)
		setVector(in.doc, "DestinationControlVector", destVec)
	}
	drop := g.bodyFlows(x.id)
	drop[out.id] = true
	for _, af := range g.annotationFlows(x.id) {
		drop[af.id] = true
	}
	m.removeFlows(drop)
	return m.removeObject(x)
}

// SetReturnValue sets the value an end event returns, in place: the end event
// keeps its $ID, its position, the flows into it and the notes attached to
// it, and no other element changes. It is how a changed `return` is written
// (ako/mxcli#805): an end event cannot be dropped or replaced like an activity,
// because it ends a path, but its value is a property like an activity's
// caption.
func (m *Mutator) SetReturnValue(target model.ID, value string) error {
	g := m.graph()
	x, err := g.node(target)
	if err != nil {
		return err
	}
	if x.typ != "Microflows$EndEvent" {
		return fmt.Errorf("cannot set the return value of %s: only an end event returns one", describeNode(x))
	}
	if !dSet(x.doc, "ReturnValue", value) {
		return fmt.Errorf("%s stores no ReturnValue", describeNode(x))
	}
	return nil
}

// SetCondition sets the expression a decision branches on, and its caption,
// in place (ako/mxcli#888): the decision keeps its $ID, its flows and the case
// values on them, so both of its paths stay as drawn. Only an expression
// decision has an expression to set; one that calls a rule is refused, since
// turning it into an expression (or back) replaces its condition element.
func (m *Mutator) SetCondition(target model.ID, expression, caption string) error {
	g := m.graph()
	x, err := g.node(target)
	if err != nil {
		return err
	}
	if x.typ != "Microflows$ExclusiveSplit" {
		return fmt.Errorf("cannot set the condition of %s: only a decision has one", describeNode(x))
	}
	cond := dDoc(x.doc, "SplitCondition")
	if cond == nil || dString(cond, "$Type") != "Microflows$ExpressionSplitCondition" {
		return fmt.Errorf("cannot set the condition of %s: it calls a rule, and the splice sets an expression's text only", describeNode(x))
	}
	if !dSet(cond, "Expression", expression) {
		return fmt.Errorf("%s stores no Expression", describeNode(x))
	}
	if !dSet(x.doc, "Caption", caption) {
		return fmt.Errorf("%s stores no Caption", describeNode(x))
	}
	return nil
}

// Move sets where target is drawn (ako/mxcli#818): a stated @position or
// @start that differs from the stored one. Only the node's RelativeMiddlePoint
// changes. Its flows keep their ends, sides and control vectors — a control
// vector is relative to its end, so a curve keeps its shape — and no other
// node moves. A node inside a loop body is refused: its coordinates are
// relative to the loop box, and alter does not edit inside a loop yet.
func (m *Mutator) Move(target model.ID, to model.Point) error {
	g := m.graph()
	x, err := g.node(target)
	if err != nil {
		return err
	}
	if !dSet(x.doc, "RelativeMiddlePoint", point{to.X, to.Y}.String()) {
		return fmt.Errorf("%s stores no position", describeNode(x))
	}
	return nil
}

// Pin records that the script states a node is drawn at at
// (mendixlabs/mxcli#1598, case 6). A point rather than a node: the node there
// may be one a later fragment places, at the position its statement states.
func (m *Mutator) Pin(at model.Point) error {
	if m.pinned == nil {
		m.pinned = map[point]bool{}
	}
	m.pinned[point{at.X, at.Y}] = true
	return nil
}

// PinInLoop is Pin for a node of loop's body, at a point in the loop box's
// coordinates (ako/mxcli#1693): making room in that body refuses to move it.
func (m *Mutator) PinInLoop(loop model.ID, at model.Point) error {
	if m.pinnedIn == nil {
		m.pinnedIn = map[string]map[point]bool{}
	}
	if m.pinnedIn[string(loop)] == nil {
		m.pinnedIn[string(loop)] = map[point]bool{}
	}
	m.pinnedIn[string(loop)][point{at.X, at.Y}] = true
	return nil
}

// isPinned reports whether the script states that a node of scope is drawn
// at p.
func (m *Mutator) isPinned(scope string, p point) bool {
	if scope == "" {
		return m.pinned[p]
	}
	return m.pinnedIn[scope][p]
}

// bodyFlows returns the flows that run inside loop's body, at any depth. They
// are stored in the unit's Flows list, not in the loop, so taking the loop out
// has to take them too; left behind they would point at removed objects.
func (g *graph) bodyFlows(loop string) map[string]bool {
	inside := func(id string) bool {
		for n := g.nodes[id]; n != nil && n.loop != ""; n = g.nodes[n.loop] {
			if n.loop == loop {
				return true
			}
		}
		return false
	}
	out := map[string]bool{}
	for _, f := range g.flows {
		if inside(f.origin) || inside(f.dest) {
			out[f.id] = true
		}
	}
	return out
}

// removable checks that x can be taken out of the flow and returns the one
// flow that leaves it.
func removable(g *graph, x *node, verb string) (flowRef, error) {
	switch x.typ {
	case "Microflows$ActionActivity", "Microflows$LoopedActivity":
	default:
		return flowRef{}, fmt.Errorf("cannot %s %s: only an activity, a loop or a decision of the shape `alter … insert` "+
			"puts in can be, because an end event or a merge is where the flow's paths end or meet", verb, describeNode(x))
	}
	normal, errs := g.outgoing(x.id)
	if len(errs) > 0 {
		// create or modify reaches this same refusal, so naming it would send
		// the reader in a circle (mendixlabs/mxcli#1489): what takes a
		// handler out is the deliberate rebuild.
		return flowRef{}, fmt.Errorf("cannot %s %s: it has an error handler, which would be left with no activity; "+
			"the splice takes no handler out — to rebuild the flow deliberately, drop the microflow and create it "+
			"in the same script", verb, describeNode(x))
	}
	if len(normal) != 1 {
		return flowRef{}, fmt.Errorf("cannot %s %s: it has %d outgoing flows, not one", verb, describeNode(x), len(normal))
	}
	return normal[0], nil
}

// ---------------------------------------------------------------------------
// Tree edits
// ---------------------------------------------------------------------------

// addFragment serializes the fragment and appends it: its objects to the top
// level collection (appended, so no stored object changes list position), its
// flows to the Flows list.
func (m *Mutator) addFragment(frag *backend.MicroflowFragment) error {
	return m.addFragmentIn(frag, "")
}

// collection returns the object collection of scope: the loop with that $ID,
// or the flow's own for "". nil when there is no such loop.
func (m *Mutator) collection(scope string) bson.D {
	if scope == "" {
		return dDoc(m.doc, "ObjectCollection")
	}
	if l := m.graph().nodes[scope]; l != nil && l.typ == "Microflows$LoopedActivity" {
		return dDoc(l.doc, "ObjectCollection")
	}
	return nil
}

// addFragmentIn is addFragment into the collection of scope — a loop body,
// whose objects the loop holds while its flows, like every flow of the unit,
// are in the unit's Flows list.
func (m *Mutator) addFragmentIn(frag *backend.MicroflowFragment, scope string) error {
	oc := m.collection(scope)
	if oc == nil {
		return fmt.Errorf("loop %s has no object collection", scope)
	}
	objs := arrayElements(dGet(oc, "Objects"))
	for _, obj := range frag.Objects {
		d, err := m.deps.SerializeObject(obj)
		if err != nil {
			return fmt.Errorf("serialize %T: %w", obj, err)
		}
		if d == nil {
			return fmt.Errorf("cannot write a %T into a flow", obj)
		}
		objs = append(objs, d)
	}
	if !setArray(oc, "Objects", objs) {
		return fmt.Errorf("the object collection has no Objects list")
	}
	for _, f := range frag.Flows {
		if err := m.addFlow(f); err != nil {
			return err
		}
	}
	for _, af := range frag.AnnotationFlows {
		d, err := m.deps.SerializeAnnotationFlow(af)
		if err != nil {
			return fmt.Errorf("serialize annotation flow: %w", err)
		}
		if err := m.appendFlowDoc(d); err != nil {
			return err
		}
	}
	return nil
}

func (m *Mutator) addFlow(f *microflows.SequenceFlow) error {
	d, err := m.deps.SerializeSequenceFlow(f)
	if err != nil {
		return fmt.Errorf("serialize sequence flow: %w", err)
	}
	return m.appendFlowDoc(d)
}

func (m *Mutator) appendFlowDoc(d bson.D) error {
	if d == nil {
		return fmt.Errorf("a flow serialized to nothing")
	}
	flows := arrayElements(dGet(m.doc, "Flows"))
	flows = append(flows, d)
	if !setArray(m.doc, "Flows", flows) {
		return fmt.Errorf("the unit has no Flows list")
	}
	return nil
}

func (m *Mutator) removeFlows(ids map[string]bool) {
	var keep []any
	for _, el := range arrayElements(dGet(m.doc, "Flows")) {
		if d, ok := el.(bson.D); ok && ids[binaryID(dGet(d, "$ID"))] {
			m.markRemoved(d)
			continue
		}
		keep = append(keep, el)
	}
	setArray(m.doc, "Flows", keep)
}

// removeObject takes x out of the collection that holds it.
func (m *Mutator) removeObject(x *node) error {
	oc := m.collection(x.loop)
	var keep []any
	found := false
	for _, el := range arrayElements(dGet(oc, "Objects")) {
		if d, ok := el.(bson.D); ok && binaryID(dGet(d, "$ID")) == x.id {
			m.markRemoved(d)
			found = true
			continue
		}
		keep = append(keep, el)
	}
	if !found {
		return fmt.Errorf("%s is not in its collection", describeNode(x))
	}
	setArray(oc, "Objects", keep)
	return nil
}

// markRemoved records the $ID of d and of every element nested in it.
func (m *Mutator) markRemoved(v any) {
	switch t := v.(type) {
	case bson.D:
		for _, e := range t {
			if e.Key == "$ID" {
				if b, ok := e.Value.(bson.Binary); ok {
					m.removed[string(b.Data)] = true
				}
				continue
			}
			m.markRemoved(e.Value)
		}
	case bson.A:
		for _, el := range t {
			m.markRemoved(el)
		}
	}
}

// shift moves every top-level node past the cut (along axis ax, in the
// direction of s) by delta, but the parameters (shifted). The sweep keeps
// everything on either side of the cut exactly as it was relative to its
// neighbours, so only the flows that cross the cut get longer; a flow's
// control vectors are relative to its ends, so no curve changes. skip is a
// node that is about to be removed.
func (m *Mutator) shift(g *graph, ax axis, s int, cut float64, delta int, skip string) {
	m.shiftIn(g, "", ax, s, cut, delta, skip)
}

// shiftIn is shift among the nodes of scope: the loop body with that $ID, or
// the top level for "". A body node's coordinates are relative to its loop
// box, so a shift inside a body moves the body's own nodes only; the box is
// fitted to them afterwards (fitLoopTo).
func (m *Mutator) shiftIn(g *graph, scope string, ax axis, s int, cut float64, delta int, skip string) {
	for _, n := range g.order {
		if !shifted(n, scope, ax, s, cut, skip) {
			continue
		}
		n.pos = n.pos.set(ax, n.pos.get(ax)+delta)
		dSet(n.doc, "RelativeMiddlePoint", n.pos.String())
	}
}

// shifted reports whether shift moves n: a node of scope past the cut that is
// not a parameter. A parameter is not on the line the room is made on: it
// belongs to the flow as a whole, and Studio Pro authors draw it above the
// start event (63 of 71 in PedApp and JTSBootLogboek at the start's x, none
// within 60px of the line). Moved, it drifted right with every insert and
// describe printed a @position the script never stated (ako/mxcli#1547).
func shifted(n *node, scope string, ax axis, s int, cut float64, skip string) bool {
	return n.loop == scope && n.id != skip && n.typ != "Microflows$MicroflowParameter" &&
		float64(s)*(float64(n.pos.get(ax))-cut) > 0
}

// pinnedPast returns the first node shift would move that is drawn at a point
// the script states (Pin), or nil. Moving it would make the stored flow say
// something other than the script — a later fragment placed at its own stated
// position would be left behind on the wrong side of a node that moved, and
// re-running the script would move the node back (mendixlabs/mxcli#1598,
// case 6) — so the caller refuses instead. A node of a loop body is pinned in
// its loop box's coordinates (PinInLoop).
func (m *Mutator) pinnedPast(g *graph, scope string, ax axis, s int, cut float64, skip string) *node {
	for _, n := range g.order {
		if m.isPinned(scope, n.pos) && shifted(n, scope, ax, s, cut, skip) {
			return n
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Loop boxes
// ---------------------------------------------------------------------------

// loopPadding is the room mxcli's layout leaves between a loop box's edge and
// the body inside it (the executor's LoopPadding).
const loopPadding = 50

// bounds is the box around a collection's objects, in its coordinates.
type bounds struct {
	min, max point
	n        int
}

// contentBounds returns the box around the objects of scope's collection —
// notes included, as mxcli's layout sizes a loop box (fitContainerSize). It
// reads the stored documents, so objects an operation has just added count.
// The zero value for the top level, which has no box.
func (m *Mutator) contentBounds(scope string) bounds {
	var b bounds
	if scope == "" {
		return b
	}
	for _, el := range arrayElements(dGet(m.collection(scope), "Objects")) {
		d, ok := el.(bson.D)
		if !ok {
			continue
		}
		p, sz := parsePoint(dString(d, "RelativeMiddlePoint")), parsePoint(dString(d, "Size"))
		lo := point{p.X - sz.X/2, p.Y - sz.Y/2}
		hi := point{p.X + sz.X/2, p.Y + sz.Y/2}
		if b.n == 0 {
			b.min, b.max = lo, hi
		} else {
			b.min = point{min(b.min.X, lo.X), min(b.min.Y, lo.Y)}
			b.max = point{max(b.max.X, hi.X), max(b.max.Y, hi.Y)}
		}
		b.n++
	}
	return b
}

// fitLoopTo grows the box of the loop scope when an edit of its body —
// before is the body's box before it — left the body reaching past it
// (ako/mxcli#1693). A body may reach as close to the box's edge as it did
// before, or loopPadding, whichever is closer: a Studio Pro-drawn body that
// sat tight in its box is not a reason to grow. A body that still fits leaves
// the box as it is. "" (an edit at top level) has no box to fit.
func (m *Mutator) fitLoopTo(g *graph, scope string, before bounds) error {
	if scope == "" {
		return nil
	}
	l := g.nodes[scope]
	if l == nil {
		return fmt.Errorf("loop %s is not in the flow", scope)
	}
	after := m.contentBounds(scope)
	if after.n == 0 {
		return nil
	}
	allowMin := point{loopPadding, loopPadding}
	allowMax := point{l.size.X - loopPadding, l.size.Y - loopPadding}
	if before.n > 0 {
		allowMin = point{min(allowMin.X, before.min.X), min(allowMin.Y, before.min.Y)}
		allowMax = point{max(allowMax.X, before.max.X), max(allowMax.Y, before.max.Y)}
	}
	lo := point{max(0, allowMin.X-after.min.X), max(0, allowMin.Y-after.min.Y)}
	hi := point{max(0, after.max.X-allowMax.X), max(0, after.max.Y-allowMax.Y)}
	if lo == (point{}) && hi == (point{}) {
		return nil
	}
	ax, s := loopAxis(g, l)
	near, far := lo.get(ax), hi.get(ax)
	if s < 0 {
		near, far = far, near
	}
	if near > 0 {
		return fmt.Errorf("the body of %s would need %dpx more room on the side its flow enters from; "+
			"replace the loop's body instead, or make room in Studio Pro first", describeNode(l), near)
	}
	cross := ax.other()
	grow := point{}
	grow = grow.set(ax, far)
	grow = grow.set(cross, 2*max(lo.get(cross), hi.get(cross)))
	_, err := m.growLoop(g, l, grow, true)
	return err
}

// loopAxis is the line a loop sits on: the direction of the flow that enters
// it, by the side it enters at, else of the one that leaves it, else left to
// right. The box grows along that line on its far side only, so the flows into
// and out of the loop stay level and its predecessor stays where it is.
func loopAxis(g *graph, l *node) (axis, int) {
	for _, f := range g.incoming(l.id) {
		switch idx, _ := flowEnd(f.doc, "Destination"); idx {
		case sideLeft:
			return axisX, 1
		case sideRight:
			return axisX, -1
		case sideTop:
			return axisY, 1
		case sideBottom:
			return axisY, -1
		}
	}
	normal, _ := g.outgoing(l.id)
	for _, f := range normal {
		switch idx, _ := flowEnd(f.doc, "Origin"); idx {
		case sideRight:
			return axisX, 1
		case sideLeft:
			return axisX, -1
		case sideBottom:
			return axisY, 1
		case sideTop:
			return axisY, -1
		}
	}
	return axisX, 1
}

// growLoop enlarges the box of loop l by grow (the width and height to add).
// Along the line the loop sits on (loopAxis) the box grows on its far side,
// its near edge staying where it is, and everything in the loop's own
// collection past its far edge moves along — as the nodes past an inserted
// fragment do. Across that line it grows on both sides alike, so its centre
// stays on the line; nothing moves to make that room, and a box that would be
// drawn over another node is refused. A loop in another loop's body may make
// that box grow in turn.
//
// It returns where the box's old top-left corner is in the grown box. With
// keepBody, the body's own nodes are moved by that much, so they stay where
// they were drawn on the canvas.
func (m *Mutator) growLoop(g *graph, l *node, grow point, keepBody bool) (point, error) {
	if grow.X <= 0 && grow.Y <= 0 {
		return point{}, nil
	}
	ax, s := loopAxis(g, l)
	cross := ax.other()
	far := max(0, grow.get(ax))
	far += far % 2 // the centre moves by half of it
	half := (max(0, grow.get(cross)) + 1) / 2
	parentBefore := m.contentBounds(l.loop)

	size, pos := l.size, l.pos
	size = size.set(ax, size.get(ax)+far)
	size = size.set(cross, size.get(cross)+2*half)
	pos = pos.set(ax, pos.get(ax)+s*far/2)

	if pos != l.pos && m.isPinned(l.loop, l.pos) {
		return point{}, fmt.Errorf("%s has to grow by %dpx to fit its body, which moves its centre, and the script "+
			"draws it where it is; %s", describeNode(l), far, roomAdvice)
	}
	if far > 0 {
		cut := float64(l.pos.get(ax) + s*l.size.get(ax)/2)
		if p := m.pinnedPast(g, l.loop, ax, s, cut, l.id); p != nil {
			return point{}, fmt.Errorf("%s has to grow by %dpx to fit its body, and making room for it would move %s, "+
				"which the script draws where it is; %s", describeNode(l), far, describeNode(p), roomAdvice)
		}
		m.shiftIn(g, l.loop, ax, s, cut, s*far, l.id)
	}
	if half > 0 {
		lo := point{pos.X - size.X/2, pos.Y - size.Y/2}
		hi := point{pos.X + size.X/2, pos.Y + size.Y/2}
		for _, n := range g.order {
			// A parameter is not on the line either: it belongs to the flow as
			// a whole and sits above the start event, and mxcli's own layout
			// draws a tall loop over it (measured: a nested loop with a
			// decision, built by `create`, spans the parameters' row).
			if n.loop != l.loop || n.id == l.id || n.typ == "Microflows$MicroflowParameter" {
				continue
			}
			nlo := point{n.pos.X - n.size.X/2, n.pos.Y - n.size.Y/2}
			nhi := point{n.pos.X + n.size.X/2, n.pos.Y + n.size.Y/2}
			if nlo.X < hi.X && lo.X < nhi.X && nlo.Y < hi.Y && lo.Y < nhi.Y {
				return point{}, fmt.Errorf("there is no free room for %s to grow to fit its body: at (%d, %d)-(%d, %d) "+
					"it would be drawn over %s; move that object aside in Studio Pro first", describeNode(l),
					lo.X, lo.Y, hi.X, hi.Y, describeNode(n))
			}
		}
	}
	l.size, l.pos = size, pos
	dSet(l.doc, "RelativeMiddlePoint", pos.String())
	dSet(l.doc, "Size", size.String())

	off := point{}
	off = off.set(cross, half)
	if s < 0 {
		off = off.set(ax, far)
	}
	if keepBody && off != (point{}) {
		m.translateBody(g, l, off)
	}
	return off, m.fitLoopTo(g, l.loop, parentBefore)
}

// translateBody moves every node directly in loop l's body by off, in the
// loop box's coordinates: the box grew on its top or left side, and the body
// stays where it was drawn on the canvas.
func (m *Mutator) translateBody(g *graph, l *node, off point) {
	for _, el := range arrayElements(dGet(dDoc(l.doc, "ObjectCollection"), "Objects")) {
		d, ok := el.(bson.D)
		if !ok {
			continue
		}
		p := parsePoint(dString(d, "RelativeMiddlePoint"))
		dSet(d, "RelativeMiddlePoint", point{p.X + off.X, p.Y + off.Y}.String())
	}
	for _, n := range g.order {
		if n.loop == l.id {
			n.pos = parsePoint(dString(n.doc, "RelativeMiddlePoint"))
		}
	}
}

// ReplaceLoopBody replaces everything inside the stored loop target by the
// body of the loop frag holds (ako/mxcli#1693): the loop itself — its $ID, its
// position, its iterator and list, the flows into and out of it and the notes
// on it — stays, and only what is inside it is rebuilt. frag is the fragment
// the builder made of the loop restated with the new body: one loop object,
// and the flows and note lines of its body. The new body is laid out as the
// builder laid it out, centred across the stored box; a box too small for it
// grows (growLoop), one large enough keeps its size.
//
// A note outside the loop attached to a node in its body, or a flow between
// the body and the rest of the flow, would be left pointing at nothing, and is
// refused.
func (m *Mutator) ReplaceLoopBody(target model.ID, frag *backend.MicroflowFragment) error {
	g := m.graph()
	l, err := g.anyNode(target)
	if err != nil {
		return err
	}
	if l.typ != "Microflows$LoopedActivity" {
		return fmt.Errorf("cannot replace the body of %s: only a loop has one", describeNode(l))
	}
	var built *microflows.LoopedActivity
	for _, obj := range frag.Objects {
		lp, ok := obj.(*microflows.LoopedActivity)
		if !ok || obj.GetID() != frag.Entry || built != nil {
			return fmt.Errorf("the new body of %s is not one loop's body", describeNode(l))
		}
		built = lp
	}
	if built == nil {
		return fmt.Errorf("the new body of %s is not one loop's body", describeNode(l))
	}
	inBody := func(id string) bool {
		for n := g.nodes[id]; n != nil && n.loop != ""; n = g.nodes[n.loop] {
			if n.loop == l.id {
				return true
			}
		}
		return false
	}
	for _, f := range g.flows {
		in1, in2 := inBody(f.origin), inBody(f.dest)
		if in1 == in2 {
			continue
		}
		inner := g.nodes[f.origin]
		if in2 {
			inner = g.nodes[f.dest]
		}
		if !f.isSequence() {
			return fmt.Errorf("cannot replace the body of %s: a note outside the loop is attached to %s in its body, "+
				"and would be left attached to nothing; move the note into the loop or detach it in Studio Pro first",
				describeNode(l), describeNode(inner))
		}
		return fmt.Errorf("cannot replace the body of %s: flow %s runs between %s in its body and the rest of the flow",
			describeNode(l), f.id, describeNode(inner))
	}
	d, err := m.deps.SerializeObject(built)
	if err != nil {
		return fmt.Errorf("serialize the new body: %w", err)
	}
	newBody := arrayElements(dGet(dDoc(d, "ObjectCollection"), "Objects"))
	builtSize := point{built.Size.Width, built.Size.Height}

	// Out with the stored body: its objects (nested loops' bodies with them)
	// and every flow and note line inside it.
	oc := dDoc(l.doc, "ObjectCollection")
	if oc == nil {
		return fmt.Errorf("%s has no object collection", describeNode(l))
	}
	if _, ok := dGet(oc, "Objects").(bson.A); !ok {
		// An empty body may be stored without its list; it is written with
		// the marker every object list of a loop carries.
		oc = append(oc, bson.E{Key: "Objects", Value: bson.A{int32(3)}})
		dSet(l.doc, "ObjectCollection", oc)
	}
	for _, el := range arrayElements(dGet(oc, "Objects")) {
		m.markRemoved(el)
	}
	m.removeFlows(g.bodyFlows(l.id))
	if !setArray(oc, "Objects", nil) {
		return fmt.Errorf("%s has no Objects list", describeNode(l))
	}
	for _, n := range g.order {
		if inBody(n.id) {
			delete(g.nodes, n.id)
		}
	}

	// The box: as stored where the new body fits it, grown where it does not.
	grow := point{max(0, builtSize.X-l.size.X), max(0, builtSize.Y-l.size.Y)}
	if _, err := m.growLoop(g, l, grow, false); err != nil {
		return err
	}
	// The builder drew the body in a box of builtSize, from its left edge and
	// centred across it; centred across the stored box it stays so.
	dy := (l.size.Y - builtSize.Y) / 2
	for _, el := range newBody {
		if bd, ok := el.(bson.D); ok && dy != 0 {
			p := parsePoint(dString(bd, "RelativeMiddlePoint"))
			dSet(bd, "RelativeMiddlePoint", point{p.X, p.Y + dy}.String())
		}
	}
	if !setArray(oc, "Objects", newBody) {
		return fmt.Errorf("%s has no Objects list", describeNode(l))
	}
	for _, f := range frag.Flows {
		if err := m.addFlow(f); err != nil {
			return err
		}
	}
	for _, af := range frag.AnnotationFlows {
		ad, err := m.deps.SerializeAnnotationFlow(af)
		if err != nil {
			return fmt.Errorf("serialize annotation flow: %w", err)
		}
		if err := m.appendFlowDoc(ad); err != nil {
			return err
		}
	}
	return nil
}

// isDecision reports whether n is a split: a decision on a condition or by
// type.
func isDecision(n *node) bool {
	return n.typ == "Microflows$ExclusiveSplit" || n.typ == "Microflows$InheritanceSplit"
}

// InLoopBody reports whether the stored object id is inside a loop body.
func (m *Mutator) InLoopBody(id model.ID) bool {
	n, ok := m.graph().nodes[string(id)]
	return ok && n.loop != ""
}

// roomAdvice is the way out of a refused shift.
const roomAdvice = "state @position on the new statements, where there is room for them, or leave unstated the positions " +
	"of what comes after them (@position, @merge, a note's position:) so that it can move along"

// checkIntegrity is the rule-1 guard: no binary anywhere in the unit may
// still name an element that was removed, and no two elements may share an
// $ID. Either would make the document unopenable, and neither is caught by
// anything cheaper than Studio Pro.
func (m *Mutator) checkIntegrity() error {
	seen := map[string]bool{}
	var dup, dangling string
	var walk func(v any, key string)
	walk = func(v any, key string) {
		switch t := v.(type) {
		case bson.D:
			for _, e := range t {
				walk(e.Value, e.Key)
			}
		case bson.A:
			for _, el := range t {
				walk(el, key)
			}
		case bson.Binary:
			if len(t.Data) != 16 {
				return
			}
			k := string(t.Data)
			if key == "$ID" {
				if seen[k] && dup == "" {
					dup = types.BlobToUUID(t.Data)
				}
				seen[k] = true
				return
			}
			if m.removed[k] && dangling == "" {
				dangling = key + " -> " + types.BlobToUUID(t.Data)
			}
		}
	}
	walk(m.doc, "")
	if dup != "" {
		return fmt.Errorf("refusing to write: two elements would share $ID %s", dup)
	}
	if dangling != "" {
		return fmt.Errorf("refusing to write: a reference still points at a removed element (%s)", dangling)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Geometry
// ---------------------------------------------------------------------------

// minGap is the free space kept on each side of an inserted fragment, the
// edge-to-edge distance mxcli's own layout leaves between activities.
const minGap = 40

type axis int

const (
	axisX axis = iota
	axisY
)

func (a axis) other() axis { return 1 - a }

type point struct{ X, Y int }

func (p point) get(a axis) int {
	if a == axisX {
		return p.X
	}
	return p.Y
}

func (p point) set(a axis, v int) point {
	if a == axisX {
		p.X = v
	} else {
		p.Y = v
	}
	return p
}

func (p point) String() string { return strconv.Itoa(p.X) + ";" + strconv.Itoa(p.Y) }

func parsePoint(s string) point {
	x, y, ok := strings.Cut(s, ";")
	if !ok {
		return point{}
	}
	px, _ := strconv.Atoi(strings.TrimSpace(x))
	py, _ := strconv.Atoi(strings.TrimSpace(y))
	return point{px, py}
}

// flowAxis is the direction a flow from x to y runs: the axis along which the
// centres are further apart, and the sign along it.
func flowAxis(x, y *node) (axis, int, error) {
	dx, dy := y.pos.X-x.pos.X, y.pos.Y-x.pos.Y
	switch {
	case dx == 0 && dy == 0:
		return 0, 0, fmt.Errorf("%s and %s are drawn on the same spot; cannot tell which way the flow runs", describeNode(x), describeNode(y))
	case abs(dx) >= abs(dy):
		return axisX, sign(dx), nil
	default:
		return axisY, sign(dy), nil
	}
}

// Connection indexes, as Mendix numbers the sides of a box.
const (
	sideTop    = 0
	sideRight  = 1
	sideBottom = 2
	sideLeft   = 3
)

// side is the side of a box that faces direction s along ax.
func side(ax axis, s int) int {
	switch {
	case ax == axisX && s > 0:
		return sideRight
	case ax == axisX:
		return sideLeft
	case s > 0:
		return sideBottom
	default:
		return sideTop
	}
}

// sideVector is the control vector Studio Pro draws a straight flow end with:
// perpendicular to the side, pointing out of the box.
func sideVector(sd int) string {
	switch sd {
	case sideTop:
		return "0;-15"
	case sideRight:
		return "15;0"
	case sideBottom:
		return "0;15"
	default:
		return "-15;0"
	}
}

// fragmentBox is the fragment's layout as the builder produced it, to be
// translated into place.
type fragmentBox struct {
	frag     *backend.MicroflowFragment
	min, max point
}

func fragmentGeometry(frag *backend.MicroflowFragment) (*fragmentBox, error) {
	if frag == nil || len(frag.Objects) == 0 || frag.Entry == "" || frag.Exit == "" {
		return nil, fmt.Errorf("the fragment is empty")
	}
	fb := &fragmentBox{frag: frag}
	first := true
	for _, obj := range frag.Objects {
		// A note sits off the line it documents, so it takes no room on it.
		// Counted, a note at a stated position 1300px away made the box that
		// wide and shifted the rest of the row by 1180px; a note wider than its
		// activity shifted it by the difference (mendixlabs/mxcli#1598).
		if _, isNote := obj.(*microflows.Annotation); isNote {
			continue
		}
		p, sz := obj.GetPosition(), objectSize(obj)
		lo := point{p.X - sz.X/2, p.Y - sz.Y/2}
		hi := point{p.X + sz.X/2, p.Y + sz.Y/2}
		if first {
			fb.min, fb.max, first = lo, hi, false
			continue
		}
		fb.min = point{min(fb.min.X, lo.X), min(fb.min.Y, lo.Y)}
		fb.max = point{max(fb.max.X, hi.X), max(fb.max.Y, hi.Y)}
	}
	return fb, nil
}

func (fb *fragmentBox) length(ax axis) int { return fb.max.get(ax) - fb.min.get(ax) }

// placeCentred translates the fragment so that its box is centred on c along
// ax, and its entry object sits on c across it.
func (fb *fragmentBox) placeCentred(ax axis, c point) {
	var entry point
	for _, obj := range fb.frag.Objects {
		if obj.GetID() == fb.frag.Entry {
			entry = point{obj.GetPosition().X, obj.GetPosition().Y}
		}
	}
	d := point{}
	d = d.set(ax, c.get(ax)-(fb.min.get(ax)+fb.max.get(ax))/2)
	d = d.set(ax.other(), c.get(ax.other())-entry.get(ax.other()))
	for _, obj := range fb.frag.Objects {
		// A stated canvas position is where the script wants the object, not a
		// place relative to the fragment.
		if fb.frag.Pinned[obj.GetID()] {
			continue
		}
		p := obj.GetPosition()
		obj.SetPosition(model.Point{X: p.X + d.X, Y: p.Y + d.Y})
	}
	fb.min = point{fb.min.X + d.X, fb.min.Y + d.Y}
	fb.max = point{fb.max.X + d.X, fb.max.Y + d.Y}
}

// checkRoom refuses a placement that would draw the fragment over an object
// that stays where it is. The shift clears the space past the cut; an object
// that already sat inside the gap (a branch drawn below the main line, say)
// is not moved, and drawing over it would hide it in Studio Pro. skip is the
// object a replace removes; onFlow the flow an insert goes on, which runs to
// the fragment's entry afterwards rather than across it.
func (fb *fragmentBox) checkRoom(g *graph, skip, onFlow string) error {
	return fb.checkRoomIn(g, "", skip, onFlow)
}

// checkRoomIn is checkRoom among the nodes of scope: a loop body, in its
// box's coordinates, or the top level for "".
func (fb *fragmentBox) checkRoomIn(g *graph, scope, skip, onFlow string) error {
	for _, n := range g.order {
		if n.loop != scope || n.id == skip {
			continue
		}
		lo := point{n.pos.X - n.size.X/2, n.pos.Y - n.size.Y/2}
		hi := point{n.pos.X + n.size.X/2, n.pos.Y + n.size.Y/2}
		if lo.X < fb.max.X && fb.min.X < hi.X && lo.Y < fb.max.Y && fb.min.Y < hi.Y {
			return fmt.Errorf("there is no free room for the fragment: at (%d, %d)-(%d, %d) it would be drawn over %s; "+
				"move that object aside in Studio Pro first", fb.min.X, fb.min.Y, fb.max.X, fb.max.Y, describeNode(n))
		}
	}
	return fb.checkBranches(g, skip, onFlow)
}

// checkBranches refuses a placement whose return branches would be drawn
// across a stored flow (ako/mxcli#888). A return in the fragment ends its own
// path, off the line the fragment is spliced into: the builder draws its end
// event beside that line, and the flow to it leaves its decision sideways. The
// end event is checked with the rest of the fragment by checkRoom; what
// checkRoom cannot see is a stored flow running through that space — a branch
// below the line that rejoins past the gap, which the shift has just
// stretched across it. Such a flow is approximated by the straight line
// between the centres of its ends, which is how a flow between two aligned
// nodes is drawn.
func (fb *fragmentBox) checkBranches(g *graph, skip, onFlow string) error {
	objs := map[model.ID]microflows.MicroflowObject{}
	for _, obj := range fb.frag.Objects {
		objs[obj.GetID()] = obj
	}
	for _, f := range fb.frag.Flows {
		end, ok := objs[f.DestinationID].(*microflows.EndEvent)
		if !ok || objs[f.OriginID] == nil {
			continue
		}
		r := branchArea(objs[f.OriginID], end)
		for _, sf := range g.flows {
			if !sf.isSequence() || sf.id == onFlow || sf.origin == skip || sf.dest == skip {
				continue
			}
			a, b := g.nodes[sf.origin], g.nodes[sf.dest]
			if a == nil || b == nil || a.loop != "" || b.loop != "" {
				continue
			}
			if segmentCrosses(a.pos, b.pos, r) {
				p := end.GetPosition()
				return fmt.Errorf("there is no free room for the return at (%d, %d): its branch would be drawn across the "+
					"flow from %s to %s; move them aside in Studio Pro first", p.X, p.Y, describeNode(a), describeNode(b))
			}
		}
	}
	return nil
}

// branchArea is the space the path from a decision to one of its end events
// takes: from the side of the decision that faces the end event to the far
// side of the end event, across the full width between them.
func branchArea(from microflows.MicroflowObject, end *microflows.EndEvent) [2]point {
	o, e := from.GetPosition(), end.GetPosition()
	os, es := objectSize(from), objectSize(end)
	lo := point{min(o.X, e.X-es.X/2), min(o.Y, e.Y-es.Y/2)}
	hi := point{max(o.X, e.X+es.X/2), max(o.Y, e.Y+es.Y/2)}
	// Leave the decision's own box out: the flow that enters it comes in
	// along the line the fragment is spliced into.
	switch dx, dy := e.X-o.X, e.Y-o.Y; {
	case abs(dy) >= abs(dx) && dy > 0:
		lo.Y = o.Y + os.Y/2
	case abs(dy) >= abs(dx):
		hi.Y = o.Y - os.Y/2
	case dx > 0:
		lo.X = o.X + os.X/2
	default:
		hi.X = o.X - os.X/2
	}
	return [2]point{lo, hi}
}

// segmentCrosses reports whether the segment from a to b passes through the
// inside of the rectangle r (Liang-Barsky clipping).
func segmentCrosses(a, b point, r [2]point) bool {
	t0, t1 := 0.0, 1.0
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	clip := func(p, q float64) bool {
		switch {
		case p == 0:
			return q > 0
		case p < 0:
			t := q / p
			if t > t1 {
				return false
			}
			t0 = max(t0, t)
		default:
			t := q / p
			if t < t0 {
				return false
			}
			t1 = min(t1, t)
		}
		return true
	}
	ax, ay := float64(a.X), float64(a.Y)
	return clip(-dx, ax-float64(r[0].X)) && clip(dx, float64(r[1].X)-ax) &&
		clip(-dy, ay-float64(r[0].Y)) && clip(dy, float64(r[1].Y)-ay) && t0 < t1
}

func objectSize(obj microflows.MicroflowObject) point {
	if s, ok := obj.(interface{ GetSize() model.Size }); ok {
		sz := s.GetSize()
		return point{sz.Width, sz.Height}
	}
	return point{}
}

// flowEnd reads a flow's connection index and control vector at one end
// ("Origin" or "Destination").
func flowEnd(d bson.D, end string) (int, string) {
	idx := 0
	switch v := dGet(d, end+"ConnectionIndex").(type) {
	case int32:
		idx = int(v)
	case int64:
		idx = int(v)
	case int:
		idx = v
	}
	vec := ""
	if line := dDoc(d, "Line"); line != nil {
		vec = dString(line, end+"ControlVector")
	}
	return idx, vec
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sign(v int) int {
	if v < 0 {
		return -1
	}
	return 1
}

func describeNode(n *node) string {
	name := strings.TrimPrefix(n.typ, "Microflows$")
	if c := dString(n.doc, "Caption"); c != "" && name != "ActionActivity" {
		name += " '" + c + "'"
	}
	return fmt.Sprintf("the %s at (%d, %d)", name, n.pos.X, n.pos.Y)
}

// ---------------------------------------------------------------------------
// bson.D helpers. The unit is decoded with bson v1 into bson.D, whose nested
// documents are bson.D and whose arrays are bson.A with Mendix's leading
// int32 list marker.
// ---------------------------------------------------------------------------

func dGet(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func dDoc(d bson.D, key string) bson.D {
	v, _ := dGet(d, key).(bson.D)
	return v
}

func dString(d bson.D, key string) string {
	v, _ := dGet(d, key).(string)
	return v
}

func dSet(d bson.D, key string, v any) bool {
	for i := range d {
		if d[i].Key == key {
			d[i].Value = v
			return true
		}
	}
	return false
}

// arrayElements returns the elements of a Mendix list, without its marker.
func arrayElements(v any) []any {
	a, ok := v.(bson.A)
	if !ok || len(a) == 0 {
		return nil
	}
	if _, isMarker := a[0].(int32); isMarker {
		return append([]any(nil), a[1:]...)
	}
	return append([]any(nil), a...)
}

// setArray replaces a list's elements, keeping its stored marker.
func setArray(d bson.D, key string, elements []any) bool {
	a, ok := dGet(d, key).(bson.A)
	if !ok {
		return false
	}
	out := bson.A{}
	if len(a) > 0 {
		if marker, isMarker := a[0].(int32); isMarker {
			out = append(out, marker)
		}
	}
	return dSet(d, key, append(out, elements...))
}

func binaryID(v any) string {
	b, ok := v.(bson.Binary)
	if !ok || len(b.Data) != 16 {
		return ""
	}
	return types.BlobToUUID(b.Data)
}

// setPointer points key at the element id, keeping the binary subtype the
// stored pointer uses.
func setPointer(d bson.D, key string, id model.ID) {
	sub := byte(0)
	if b, ok := dGet(d, key).(bson.Binary); ok {
		sub = b.Subtype
	}
	dSet(d, key, bson.Binary{Subtype: sub, Data: types.UUIDToBlob(string(id))})
}

func setBinary(d bson.D, key string, v any) {
	if b, ok := v.(bson.Binary); ok {
		dSet(d, key, bson.Binary{Subtype: b.Subtype, Data: bytes.Clone(b.Data)})
	}
}

// setInt writes an integer property with the width it is stored in.
func setInt(d bson.D, key string, v int) {
	switch dGet(d, key).(type) {
	case int64:
		dSet(d, key, int64(v))
	default:
		dSet(d, key, int32(v))
	}
}

// setVector sets a control vector on the flow's line. A flow without a
// Bezier line (a pre-10 document) has nothing to set.
func setVector(d bson.D, key, v string) {
	if v == "" {
		return
	}
	if line := dDoc(d, "Line"); line != nil {
		dSet(line, key, v)
	}
}
