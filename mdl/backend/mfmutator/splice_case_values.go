// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// SetCaseValues puts the values of a `case` on its branches, in place
// (ako/mxcli#2618): adding a new enumeration value to a `when`, or moving one
// from one `when` to another, changes only which branch a value's flow
// enters; a `when` taken out takes the activities on its path with it, and a
// new `when` gets a path of its own into the case's merge. Every other node
// keeps its $ID and place, and so does every flow whose value stays where it
// was.
//
// branches are the declared branches: each with its values, the values of the
// stored branch it is (nil for a new one), and a new one's body (nil when it
// has none). removed holds the values of the stored branches no declared one
// is. A branch is the node its values' flows enter (as describe reads it): a
// value that moves has its flow re-pointed to its new branch, entering it at
// the side the branch's other flows do; a new value gets a flow drawn like
// its branch's first one; a value no branch declares any more has its flow
// taken out.
func (m *Mutator) SetCaseValues(target model.ID, branches []backend.CaseBranchEdit, removed [][]string) error {
	g := m.graph()
	x, err := g.node(target)
	if err != nil {
		return err
	}
	if x.typ != "Microflows$ExclusiveSplit" {
		return fmt.Errorf("cannot set the case values of %s: it is not a decision", describeNode(x))
	}
	byValue := splitFlowsByValue(g, x)
	flowFor := func(values []string) (flowRef, bool) {
		for _, v := range values {
			if f, ok := byValue[caseKey(v)]; ok {
				return f, true
			}
		}
		return flowRef{}, false
	}
	var merge *node
	needsMerge := len(removed) > 0
	for _, b := range branches {
		if b.Stored == nil {
			needsMerge = true
		}
	}
	if needsMerge {
		for _, b := range branches {
			if f, ok := flowFor(b.Stored); ok {
				if n, _, err := branchPath(g, x, f); err == nil {
					merge = n
					break
				}
			}
		}
		if merge == nil {
			return fmt.Errorf("cannot add or take out a branch of %s: none of its branches is a path of plain "+
				"activities into the merge the branches meet at, so there is no merge to find", describeNode(x))
		}
	}

	// A branch taken out takes the activities on its path, and the notes on
	// them alone. Its values' flows stay for the value step below, which
	// re-points the ones a branch still declares and takes out the rest.
	for _, values := range removed {
		f, ok := flowFor(values)
		if !ok {
			return fmt.Errorf("cannot take out the branch %s of %s: no flow leaves it for it", strings.Join(values, ", "), describeNode(x))
		}
		end, path, err := branchPath(g, x, f)
		if err != nil {
			return fmt.Errorf("cannot take out the branch %s of %s: %w", strings.Join(values, ", "), describeNode(x), err)
		}
		if end != merge {
			return fmt.Errorf("cannot take out the branch %s of %s: it does not rejoin the other branches at %s",
				strings.Join(values, ", "), describeNode(x), describeNode(merge))
		}
		if err := m.removePath(g, path); err != nil {
			return err
		}
		g = m.graph()
	}

	// A new branch: a flow for its first value into the merge, with its body
	// spliced onto that flow as an insert lays out a fragment.
	for _, b := range branches {
		if b.Stored != nil {
			continue
		}
		if len(b.Values) == 0 {
			return fmt.Errorf("cannot add a branch to %s without a value", describeNode(x))
		}
		// Its first value's stored flow, if another branch had the value, is
		// re-pointed into the merge, so the value keeps one flow; otherwise a
		// new one is drawn.
		var id model.ID
		g = m.graph()
		if f, ok := splitFlowsByValue(g, x)[caseKey(b.Values[0])]; ok {
			id = model.ID(f.id)
			setPointer(f.doc, "DestinationPointer", model.ID(merge.id))
			setInt(f.doc, "OriginConnectionIndex", 2)
			setInt(f.doc, "DestinationConnectionIndex", 2)
		} else {
			id = model.ID(types.GenerateID())
			if err := m.addFlow(&microflows.SequenceFlow{
				BaseElement:                model.BaseElement{ID: id},
				OriginID:                   model.ID(x.id),
				DestinationID:              model.ID(merge.id),
				OriginConnectionIndex:      2,
				DestinationConnectionIndex: 2,
				CaseValue:                  &microflows.EnumerationCase{Value: b.Values[0]},
			}); err != nil {
				return err
			}
		}
		if b.Body != nil && len(b.Body.Objects) > 0 {
			g = m.graph()
			// Drawn as a new row under the branches, rather than on the line
			// from the split to the merge, which runs through them.
			if err := placeBelowBranches(g, x, merge, b.Body); err != nil {
				return err
			}
			f := flowByIDIn(g, string(id))
			if f == nil {
				return fmt.Errorf("the flow for the new branch %s is not in the unit", b.Values[0])
			}
			if err := m.spliceOnFlow(g, *f, b.Body); err != nil {
				return fmt.Errorf("cannot add the branch %s to %s: %w", strings.Join(b.Values, ", "), describeNode(x), err)
			}
		}
	}
	g = m.graph()
	byValue = splitFlowsByValue(g, x)

	// Each branch's entry, and the flow that draws it.
	entry := make([]flowRef, len(branches))
	declared := make([][]string, len(branches))
	for i, b := range branches {
		ref := b.Stored
		if ref == nil {
			ref = b.Values[:1]
		}
		f, ok := flowFor(ref)
		if !ok {
			return fmt.Errorf("cannot set the case values of %s: no flow leaves it for %s", describeNode(x), strings.Join(ref, ", "))
		}
		entry[i], declared[i] = f, b.Values
	}
	declaredAt := map[string]int{}
	for i, values := range declared {
		if len(values) == 0 {
			return fmt.Errorf("cannot set the case values of %s: branch %d would have no value", describeNode(x), i+1)
		}
		for _, v := range values {
			if j, dup := declaredAt[caseKey(v)]; dup && j != i {
				return fmt.Errorf("cannot set the case values of %s: %s is on two branches", describeNode(x), v)
			}
			declaredAt[caseKey(v)] = i
		}
	}

	drop := map[string]bool{}
	for key, f := range byValue {
		if _, ok := declaredAt[key]; !ok {
			drop[f.id] = true
		}
	}
	dropNodes := map[string]bool{}
	for i, values := range declared {
		want := entry[i]
		destIdx, destVec := flowEnd(want.doc, "Destination")
		originIdx, originVec := flowEnd(want.doc, "Origin")
		dest := g.nodes[want.dest]
		if dest == nil {
			return fmt.Errorf("cannot set the case values of %s: a branch enters a node that is not in the flow", describeNode(x))
		}
		into := model.ID(dest.id)
		switch {
		case len(values) > 1 && dest.typ != "Microflows$ExclusiveMerge":
			// An activity takes one incoming flow (CE0709): the branch's
			// values enter a merge put in front of it, as `create` draws a
			// `when A, B`.
			mergeID, err := m.branchMerge(x, dest, destIdx, destVec)
			if err != nil {
				return err
			}
			into, destIdx, destVec = mergeID, destIdx, ""
		case len(values) == 1 && dest.typ == "Microflows$ExclusiveMerge" && branchMergeOf(g, x, dest):
			// The merge only joined this branch's values; with one left it
			// would join nothing (mendixlabs/mxcli#923), so the value enters
			// what the merge led to.
			out, _ := g.outgoing(dest.id)
			if len(out) == 1 {
				into = model.ID(out[0].dest)
				destIdx, destVec = flowEnd(out[0].doc, "Destination")
				drop[out[0].id] = true
				dropNodes[dest.id] = true
			}
		}
		for _, v := range values {
			f, ok := byValue[caseKey(v)]
			if ok {
				if model.ID(f.dest) != into {
					// Drawn like the branch it joins: out of the split at
					// the side that branch's flows leave from.
					if f.dest != want.dest {
						setInt(f.doc, "OriginConnectionIndex", originIdx)
						setVector(f.doc, "OriginControlVector", originVec)
					}
					setPointer(f.doc, "DestinationPointer", into)
					setInt(f.doc, "DestinationConnectionIndex", destIdx)
					setVector(f.doc, "DestinationControlVector", destVec)
				}
				continue
			}
			if err := m.addFlow(&microflows.SequenceFlow{
				BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
				OriginID:                   model.ID(x.id),
				DestinationID:              into,
				OriginConnectionIndex:      originIdx,
				DestinationConnectionIndex: destIdx,
				OriginControlVector:        originVec,
				DestinationControlVector:   destVec,
				CaseValue:                  &microflows.EnumerationCase{Value: v},
			}); err != nil {
				return err
			}
		}
	}
	if len(drop) > 0 {
		m.removeFlows(drop)
	}
	for id := range dropNodes {
		if err := m.removeObject(g.nodes[id]); err != nil {
			return err
		}
	}
	return nil
}

// branchMergeOf reports whether merge only joins flows that leave split: the
// merge `create` draws in front of a `when A, B` branch, not the one the
// branches meet at.
func branchMergeOf(g *graph, split, merge *node) bool {
	in := g.incoming(merge.id)
	if len(in) == 0 {
		return false
	}
	for _, f := range in {
		if f.origin != split.id {
			return false
		}
	}
	return true
}

// branchMerge puts a merge in front of dest, between it and split, with a flow
// from the merge into dest at the side the branch entered it (destIdx), and
// returns the merge's $ID. The merge is drawn halfway between the two.
func (m *Mutator) branchMerge(split, dest *node, destIdx int, destVec string) (model.ID, error) {
	id := model.ID(types.GenerateID())
	at := model.Point{X: (split.pos.X + dest.pos.X) / 2, Y: (split.pos.Y + dest.pos.Y) / 2}
	merge := &microflows.ExclusiveMerge{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: id},
		Position:    at,
		Size:        model.Size{Width: branchMergeSize, Height: branchMergeSize},
	}}
	d, err := m.deps.SerializeObject(merge)
	if err != nil {
		return "", fmt.Errorf("serialize merge: %w", err)
	}
	oc := m.collection(dest.loop)
	if oc == nil {
		return "", fmt.Errorf("%s is in no object collection", describeNode(dest))
	}
	if !setArray(oc, "Objects", append(arrayElements(dGet(oc, "Objects")), d)) {
		return "", fmt.Errorf("the object collection has no Objects list")
	}
	if err := m.addFlow(&microflows.SequenceFlow{
		BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:                   id,
		DestinationID:              model.ID(dest.id),
		OriginConnectionIndex:      (destIdx + 2) % 4,
		DestinationConnectionIndex: destIdx,
		DestinationControlVector:   destVec,
	}); err != nil {
		return "", err
	}
	return id, nil
}

// branchMergeSize is the size `create` draws a merge at.
const branchMergeSize = 40

// enumFlowValue is the enumeration value a split's flow carries, and false for
// a flow with none. An enumeration case with an empty Value is `(empty)`.
func enumFlowValue(f flowRef) (string, bool) {
	cases := arrayElements(dGet(f.doc, "CaseValues"))
	if d := dDoc(f.doc, "NewCaseValue"); d != nil {
		cases = append([]any{d}, cases...)
	}
	for _, el := range cases {
		d, ok := el.(bson.D)
		if !ok || dString(d, "$Type") != "Microflows$EnumerationCase" {
			continue
		}
		if v := dString(d, "Value"); v != "" {
			return v, true
		}
		return "(empty)", true
	}
	return "", false
}

// caseKey is a value as WHEN values are matched: case-insensitively, quotes
// and the parentheses of `(empty)` ignored.
func caseKey(v string) string {
	v = strings.ToLower(strings.Trim(v, "\"`"))
	return strings.Trim(v, "()")
}

// splitFlowsByValue maps each value on a flow leaving the split x to its flow.
func splitFlowsByValue(g *graph, x *node) map[string]flowRef {
	normal, _ := g.outgoing(x.id)
	out := map[string]flowRef{}
	for _, f := range normal {
		if v, ok := enumFlowValue(f); ok {
			out[caseKey(v)] = f
		}
	}
	return out
}

// casePath is a branch's own part of the flow: the merge in front of it, if
// any, the activities on it, and the flows between them and into the merge
// the branches meet at. Its values' flows are not part of it.
type casePath struct {
	nodes []*node
	flows map[string]bool
}

// branchPath walks a `case` branch from its flow f to the merge the branches
// meet at, and returns that merge and the branch's own path. Only a path of
// plain activities, as a decision's path the splice takes out, is walked;
// one that returns, raises, holds a decision or a loop, has an error handler
// or is joined from elsewhere is refused.
func branchPath(g *graph, x *node, f flowRef) (*node, casePath, error) {
	p := casePath{flows: map[string]bool{}}
	n := g.nodes[f.dest]
	if n != nil && n.typ == "Microflows$ExclusiveMerge" && branchMergeOf(g, x, n) {
		out, _ := g.outgoing(n.id)
		if len(out) != 1 {
			return nil, p, fmt.Errorf("the merge in front of it has %d outgoing flows", len(out))
		}
		p.nodes = append(p.nodes, n)
		p.flows[out[0].id] = true
		n = g.nodes[out[0].dest]
	}
	for n != nil && n.typ == "Microflows$ActionActivity" {
		if in := g.incoming(n.id); len(in) != 1 {
			return nil, p, fmt.Errorf("%d flows enter %s, so a path from elsewhere joins it there", len(in), describeNode(n))
		}
		out, errs := g.outgoing(n.id)
		if len(errs) > 0 {
			return nil, p, fmt.Errorf("%s has an error handler", describeNode(n))
		}
		if len(out) != 1 {
			return nil, p, fmt.Errorf("%s has %d outgoing flows, not one", describeNode(n), len(out))
		}
		p.nodes = append(p.nodes, n)
		p.flows[out[0].id] = true
		n = g.nodes[out[0].dest]
	}
	if n == nil {
		return nil, p, fmt.Errorf("it leads to an object that is not in the flow")
	}
	switch n.typ {
	case "Microflows$ExclusiveMerge":
		return n, p, nil
	case "Microflows$EndEvent":
		return nil, p, fmt.Errorf("it ends in a return")
	case "Microflows$ErrorEvent":
		return nil, p, fmt.Errorf("it ends in a raise error")
	default:
		return nil, p, fmt.Errorf("it reaches %s, not the merge its branches meet at", describeNode(n))
	}
}

// removePath takes a branch's own path out: its nodes, the flows between
// them, the lines of the notes on them and the notes attached to nothing
// else.
func (m *Mutator) removePath(g *graph, p casePath) error {
	gone := map[string]bool{}
	for _, n := range p.nodes {
		gone[n.id] = true
	}
	drop := map[string]bool{}
	for id := range p.flows {
		drop[id] = true
	}
	var notes []*node
	seen := map[string]bool{}
	for _, n := range p.nodes {
		for _, af := range g.annotationFlows(n.id) {
			drop[af.id] = true
			other := af.origin
			if other == n.id {
				other = af.dest
			}
			note := g.nodes[other]
			if note == nil || note.typ != "Microflows$Annotation" || seen[note.id] {
				continue
			}
			seen[note.id] = true
			alone := true
			for _, nf := range g.annotationFlows(note.id) {
				o := nf.origin
				if o == note.id {
					o = nf.dest
				}
				if !gone[o] {
					alone = false
				}
			}
			if alone {
				notes = append(notes, note)
			}
		}
	}
	m.removeFlows(drop)
	for _, n := range append(append([]*node(nil), p.nodes...), notes...) {
		if err := m.removeObject(n); err != nil {
			return err
		}
	}
	return nil
}

func flowByIDIn(g *graph, id string) *flowRef {
	for i := range g.flows {
		if g.flows[i].id == id {
			return &g.flows[i]
		}
	}
	return nil
}

// branchRowGap is how far below the lowest branch a new one is drawn.
const branchRowGap = 100

// placeBelowBranches translates a new branch's body to a row of its own under
// everything drawn between the split x and the merge, starting a column to
// the right of the split, and marks it placed so nothing is moved for it.
func placeBelowBranches(g *graph, x, merge *node, frag *backend.MicroflowFragment) error {
	fb, err := fragmentGeometry(frag)
	if err != nil {
		return err
	}
	lowest := max(x.pos.Y+x.size.Y/2, merge.pos.Y+merge.size.Y/2)
	for _, n := range g.order {
		if n.loop != x.loop || n.typ == "Microflows$Annotation" || n.typ == "Microflows$MicroflowParameter" {
			continue
		}
		if n.pos.X > x.pos.X && n.pos.X < merge.pos.X {
			lowest = max(lowest, n.pos.Y+n.size.Y/2)
		}
	}
	h := fb.max.Y - fb.min.Y
	want := point{x.pos.X + x.size.X/2 + branchRowGap, lowest + branchRowGap + h/2}
	off := point{want.X - fb.min.X, want.Y - (fb.min.Y + h/2)}
	for _, obj := range frag.Objects {
		if frag.Pinned[obj.GetID()] {
			continue
		}
		p := obj.GetPosition()
		obj.SetPosition(model.Point{X: p.X + off.X, Y: p.Y + off.Y})
	}
	frag.Placed = true
	return nil
}
