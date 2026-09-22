// SPDX-License-Identifier: Apache-2.0

package executor

// `merge <label>` / `join <label>` — named join points.
//
// A Mendix ExclusiveMerge stores no name, so the label exists only in MDL. It is
// resolved here, at build time, and never written to the model. That is what
// makes forward and backward references both work: a `join` records an edge
// against a label, and every edge is resolved against the label table once the
// whole graph exists.
//
// Why this is not a nicety. Everything MDL could previously say about control
// flow nested: an `if` opens a block and the block closes. A Mendix microflow is
// a free graph of SequenceFlows, so it can express joins no nesting reproduces —
// two branches landing on one point from different depths — and the commonest
// real instance is an ERROR path rejoining the normal one. Before this, DESCRIBE
// rendered such a graph as an empty `on error … { }` block and a describe → exec
// round trip silently rewrote it (measured: an error edge repointed from the
// tail merge to an upstream one produced byte-identical MDL, and executing that
// MDL reproduced the tail-merge graph). See
// docs/11-proposals/PROPOSAL_structured_microflow_description.md, Phase E.

import (
	"fmt"
	"sort"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// joinEdge is one `join <label>`, pinned to the activity the path had reached.
// The destination is unknown at record time — that is the whole point — so it is
// held as a label and resolved by resolveJoins.
type joinEdge struct {
	origin    model.ID
	label     string
	caseValue string
	anchor    *ast.FlowAnchors
}

// labelRegistry is shared by a flowBuilder and every sub-builder that
// contributes to the SAME MicroflowObjectCollection — today the error-handler
// builder. It is deliberately NOT shared with a loop's sub-builder: a
// LoopedActivity owns its own object collection, and a sequence flow may not
// cross that boundary, so a `join` out of a loop body is refused rather than
// wired to something Studio Pro cannot load.
type labelRegistry struct {
	merges   map[string]*microflows.ExclusiveMerge
	declared map[string]bool // a `merge <label>` statement was seen for this label
	joins    []joinEdge
	// requested counts JoinStmts that addStatement saw; handled counts the ones a
	// body loop actually wired, whether through takePendingJoin or directly. A
	// shortfall is a construct nobody taught to carry a join, which resolveJoins
	// reports rather than letting the path silently stop.
	requested int
	handled   int
}

// labels returns the registry, creating it on first use. A sub-builder that must
// share the parent's labels is given the same pointer at construction.
func (fb *flowBuilder) labels() *labelRegistry {
	if fb.labelReg == nil {
		fb.labelReg = &labelRegistry{
			merges:   map[string]*microflows.ExclusiveMerge{},
			declared: map[string]bool{},
		}
	}
	return fb.labelReg
}

// mergeForLabel returns the ExclusiveMerge for a label, creating it on first
// mention. A forward `join m1` therefore creates the object that the later
// `merge m1` adopts — the merge statement only moves it into place.
func (fb *flowBuilder) mergeForLabel(label string) *microflows.ExclusiveMerge {
	reg := fb.labels()
	if m, ok := reg.merges[label]; ok {
		return m
	}
	m := &microflows.ExclusiveMerge{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: fb.posX, Y: fb.posY},
			Size:        model.Size{Width: MergeSize, Height: MergeSize},
		},
	}
	reg.merges[label] = m
	fb.objects = append(fb.objects, m)
	return m
}

// addMergeStatement handles `merge <label>`.
//
// The merge becomes the current activity, so statements after it flow out of it
// in the ordinary way. Its position comes from the statement's @position when
// there is one; addStatement has already moved fb.posX/posY there.
func (fb *flowBuilder) addMergeStatement(s *ast.MergeStmt) model.ID {
	reg := fb.labels()
	if reg.declared[s.Label] {
		fb.errors = append(fb.errors, fmt.Sprintf(
			"merge %s: declared twice — a label names one join point", s.Label))
		return reg.merges[s.Label].ID
	}
	reg.declared[s.Label] = true

	m := fb.mergeForLabel(s.Label)
	// Adopt the declaration's position even when a forward `join` created the
	// object earlier at whatever the cursor happened to be.
	m.Position = model.Point{X: fb.posX, Y: fb.posY}
	// A merge is MergeSize wide, not an activity's width, so half a pitch from its
	// CENTRE put the next activity's edge exactly on the merge's. Clear the merge
	// first, then leave the ordinary gap — the same arithmetic addIfStatement uses
	// after the merge that closes a split.
	fb.posX += MergeSize + fb.spacing/2
	// A merge is a join point, not a terminator: whatever follows continues from
	// it, so an end event is owed again even if the path that reached here
	// arrived by `join`.
	fb.endsWithReturn = false
	return m.ID
}

// addJoinStatement handles `join <label>`.
//
// It creates no object and returns no activity id: the edge cannot be drawn
// until the enclosing body loop says where the path had got to. The label is
// parked in pendingJoin for that loop to consume via takePendingJoin.
func (fb *flowBuilder) addJoinStatement(s *ast.JoinStmt) model.ID {
	reg := fb.labels()
	reg.requested++
	fb.pendingJoin = s
	// Terminating, like RETURN or BREAK — this path ends at the merge, so the
	// body must not also grow a trailing end event.
	fb.endsWithReturn = true
	return ""
}

// takePendingJoin records the edge for a `join` that addStatement just parked,
// from the activity the caller's path had reached. Returns true when it did, so
// the caller can skip its ordinary flow-creation and move to the next statement.
//
// Every body loop that can contain a `join` must call this immediately after
// addStatement. A loop that does not is caught by resolveJoins rather than
// silently dropping the path.
func (fb *flowBuilder) takePendingJoin(origin model.ID, caseValue string, anchor *ast.FlowAnchors) bool {
	if fb.pendingJoin == nil {
		return false
	}
	label := fb.pendingJoin.Label
	fb.pendingJoin = nil
	if origin == "" {
		fb.labels().handled++
		fb.errors = append(fb.errors, fmt.Sprintf(
			"join %s: nothing precedes it on this path — a join sends the current path to a merge, "+
				"so there has to be a current path", label))
		return true
	}
	reg := fb.labels()
	reg.handled++
	reg.joins = append(reg.joins, joinEdge{origin: origin, label: label, caseValue: caseValue, anchor: anchor})
	return true
}

// resolveJoins turns every recorded label edge into a real SequenceFlow. Called
// once, after the whole graph exists, which is what lets a join precede its
// merge.
func (fb *flowBuilder) resolveJoins() {
	reg := fb.labelReg
	if reg == nil {
		return
	}

	// An undeclared label is a typo, not a lazily-created merge: mergeForLabel
	// has already put an object in the collection for it, so leaving it would
	// ship an orphan merge that mxbuild rejects. Report and drop.
	var undeclared []string
	for label := range reg.merges {
		if !reg.declared[label] {
			undeclared = append(undeclared, label)
		}
	}
	sort.Strings(undeclared)
	for _, label := range undeclared {
		fb.errors = append(fb.errors, fmt.Sprintf(
			"join %s: no `merge %s;` in this microflow", label, label))
		fb.dropObject(reg.merges[label].ID)
		delete(reg.merges, label)
	}

	for _, e := range reg.joins {
		m, ok := reg.merges[e.label]
		if !ok {
			continue // already reported as undeclared
		}
		var flow *microflows.SequenceFlow
		if e.caseValue != "" {
			flow = newHorizontalFlowWithCase(e.origin, m.ID, e.caseValue)
		} else {
			flow = newHorizontalFlow(e.origin, m.ID)
		}
		applyUserAnchors(flow, e.anchor, nil)
		fb.flows = append(fb.flows, flow)
	}

	// The guard that makes a missed body loop loud. `join` inside a construct
	// nobody wired would otherwise vanish: addStatement returns "", the loop
	// creates nothing, and the path just stops.
	if pending := reg.requested - reg.handled; pending > 0 {
		fb.errors = append(fb.errors, fmt.Sprintf(
			"%d `join` statement(s) are in a construct that cannot carry one — "+
				"a join may not cross a loop or while body, because a Mendix loop owns its "+
				"own object collection and a sequence flow cannot leave it", pending))
	}

	// A merge nothing reaches is an orphan for the same reason an undeclared one
	// is: Mendix rejects a merge with no inbound flow.
	fb.dropUnreachedMerges(reg)
}

// dropUnreachedMerges removes declared merges that no flow reaches. Declaring a
// merge and never joining it is a no-op in the author's head and a build error
// in Mendix, so it is reported rather than shipped.
func (fb *flowBuilder) dropUnreachedMerges(reg *labelRegistry) {
	inbound := map[model.ID]int{}
	for _, f := range fb.flows {
		if f != nil {
			inbound[f.DestinationID]++
		}
	}
	var orphans []string
	for label, m := range reg.merges {
		if inbound[m.ID] == 0 {
			orphans = append(orphans, label)
		}
	}
	sort.Strings(orphans)
	for _, label := range orphans {
		fb.errors = append(fb.errors, fmt.Sprintf(
			"merge %s: nothing joins it — every merge needs at least one incoming path", label))
		fb.dropObject(reg.merges[label].ID)
		delete(reg.merges, label)
	}
}

// dropObject removes an object and every flow touching it. Used only for merges
// the build has decided not to ship; a half-removed merge is worse than none,
// because a dangling pointer is what makes a project unopenable.
func (fb *flowBuilder) dropObject(id model.ID) {
	objs := fb.objects[:0]
	for _, o := range fb.objects {
		if o.GetID() != id {
			objs = append(objs, o)
		}
	}
	fb.objects = objs

	flows := fb.flows[:0]
	for _, f := range fb.flows {
		if f != nil && (f.OriginID == id || f.DestinationID == id) {
			continue
		}
		flows = append(flows, f)
	}
	fb.flows = flows
}

// takeBranchJoin is takePendingJoin for a body that hangs off a split. When the
// join is the branch's FIRST statement the path has not reached an activity yet,
// so the edge starts at the split itself and carries the branch's case value —
// `if c then join m1; else … end if;` is a split whose true side goes straight
// to m1.
func (fb *flowBuilder) takeBranchJoin(
	lastInBranch, splitID model.ID,
	branchCase, pendingCase string,
	prevAnchor, branchAnchor *ast.FlowAnchors,
) bool {
	if fb.pendingJoin == nil {
		return false
	}
	if lastInBranch == "" {
		return fb.takePendingJoin(splitID, branchCase, branchAnchor)
	}
	return fb.takePendingJoin(lastInBranch, pendingCase, prevAnchor)
}
