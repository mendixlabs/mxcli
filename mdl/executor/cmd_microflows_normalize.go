// SPDX-License-Identifier: Apache-2.0

package executor

// Mode 3 of PROPOSAL_structured_microflow_description.md: render an irreducible
// but RECOMBINABLE graph by folding its branch guards into one condition,
// instead of flattening it into MDL that means something else.
//
// The approach is deliberately not a second describer. It rewrites a COPY of
// the graph into the equivalent properly-nested one and hands that to the
// describer that already exists, so Mode 3 inherits every activity renderer,
// annotation and layout rule for free and cannot drift from Mode 1. What the
// describer sees is an ordinary `if <folded> then … end if`.
//
// Opt-in, and that is load-bearing rather than a convenience: the output
// re-executes to a DIFFERENT graph — same behaviour, fewer nodes, different
// layout. Someone describing a microflow to change one activity must not have
// their canvas rebuilt as a side effect.

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/microflowgraph"
	"github.com/mendixlabs/mxcli/mdl/microflownorm"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// normalizeCollection folds every recombinable split it can and returns the
// rewritten collection plus one note per fold, for the description's header.
//
// It never mutates the input: the stored model is not touched by a DESCRIBE.
// A split it cannot fold is left exactly as it was, so the output degrades to
// Mode 1 (with the existing MDL-FLOW01 warning) rather than to something wrong.
func normalizeCollection(oc *microflows.MicroflowObjectCollection) (*microflows.MicroflowObjectCollection, []string) {
	if oc == nil {
		return nil, nil
	}
	findings := microflowgraph.Analyze(oc.Objects, oc.Flows)
	if len(findings) == 0 {
		return oc, nil
	}

	work := shallowCopyCollection(oc)
	var notes []string
	for _, f := range findings {
		if f.Class != microflowgraph.Recombinable || len(f.Entries) != 1 {
			// Interleaved needs a duplicated activity or an invented boolean
			// (Böhm-Jacopini), and neither is a description. Left alone.
			continue
		}
		note, err := foldSplit(work, f)
		if err != nil {
			notes = append(notes, fmt.Sprintf(
				"-- NOTE: the decision at %s could not be normalized (%v); it is rendered as-is",
				positionOf(f.Split), err))
			continue
		}
		notes = append(notes, note)
	}
	return work, notes
}

func positionOf(obj microflows.MicroflowObject) string {
	if obj == nil {
		return "an unknown position"
	}
	p := obj.GetPosition()
	return fmt.Sprintf("(%d, %d)", p.X, p.Y)
}

// foldSplit rewrites one recombinable split in place within `work`.
//
// The shape being folded, with S the split, E the single entry to the shared
// suffix and J the split's post-dominator:
//
//	S …only splits and merges… ─→ E   (the shared suffix)
//	                            └─→ J   (everything that misses it)
//
// becomes
//
//	S(folded) ─true→ E
//	          └false→ J
//
// which is properly nested and describes as an ordinary `if`.
func foldSplit(work *microflows.MicroflowObjectCollection, f microflowgraph.Finding) (string, error) {
	entry := f.Entries[0]
	join := f.JoinID
	if join == "" {
		return "", fmt.Errorf("the branches never rejoin")
	}

	nodes := map[model.ID]microflows.MicroflowObject{}
	for _, o := range work.Objects {
		if o != nil {
			nodes[o.GetID()] = o
		}
	}
	succ := map[model.ID][]*microflows.SequenceFlow{}
	for _, fl := range work.Flows {
		if fl != nil && !fl.IsErrorHandler {
			succ[fl.OriginID] = append(succ[fl.OriginID], fl)
		}
	}

	// The region is everything strictly between the split and the entry.
	region, err := collectRegion(f.SplitID, entry, join, nodes, succ)
	if err != nil {
		return "", err
	}

	cond, err := microflownorm.RegionCondition(region, f.SplitID, entry)
	if err != nil {
		return "", err
	}
	if lit, ok := cond.(microflownorm.Lit); ok && !bool(lit) {
		return "", fmt.Errorf("the shared suffix is unreachable")
	}

	split, ok := nodes[f.SplitID].(*microflows.ExclusiveSplit)
	if !ok {
		return "", fmt.Errorf("only an exclusive split can carry a folded condition")
	}

	// Rewrite: the split keeps its identity, position and annotations, and
	// gains the folded guard.
	split.SplitCondition = &microflows.ExpressionSplitCondition{Expression: cond.String()}
	// The stored caption is Studio Pro's label for the ORIGINAL guard. Keeping
	// it next to the folded one puts two different conditions on one decision,
	// with the wrong one in the more prominent place.
	split.Caption = ""

	// Drop every node of the region except the split itself, and every flow
	// that starts inside it.
	drop := map[model.ID]bool{}
	for id := range region {
		if id != f.SplitID {
			drop[id] = true
		}
	}
	var keptObjects []microflows.MicroflowObject
	for _, o := range work.Objects {
		if o != nil && drop[o.GetID()] {
			continue
		}
		keptObjects = append(keptObjects, o)
	}
	var keptFlows []*microflows.SequenceFlow
	for _, fl := range work.Flows {
		if fl == nil {
			continue
		}
		if drop[fl.OriginID] || drop[fl.DestinationID] || fl.OriginID == f.SplitID {
			continue
		}
		keptFlows = append(keptFlows, fl)
	}

	keptFlows = append(keptFlows,
		branchFlow(f.SplitID, entry, true),
		branchFlow(f.SplitID, join, false),
	)
	// The entry merge now has one predecessor (this split) and is a pure
	// pass-through the describer would walk straight through — and that
	// describe → exec would then delete. Splice it out here instead, so the
	// graph Mode 3 renders is exactly the graph re-executing its output builds.
	keptObjects, keptFlows = spliceRedundantMerge(entry, keptObjects, keptFlows)

	work.Objects = keptObjects
	work.Flows = keptFlows

	return fmt.Sprintf(
		"-- NOTE: the decision at %s was NORMALIZED - its branches were folded into one condition. "+
			"This is equivalent to the microflow but is NOT its shape: re-executing this MDL builds "+
			"a graph with fewer nodes and a different layout (mendixlabs/mxcli#923, Mode 3)",
		positionOf(f.Split)), nil
}

// collectRegion walks from the split to the entry, gathering the nodes to fold
// and refusing anything that would make folding unsound.
//
// Two refusals, both of which would otherwise produce a plausible wrong guard:
// an activity in the region (folding would move its side effect — see
// microflownorm.ErrImpureRegion) and a rule-based split (its condition is a
// rule call, which cannot be written inside a Mendix boolean expression, so
// there is no text to fold it into).
func collectRegion(
	splitID, entry, join model.ID,
	nodes map[model.ID]microflows.MicroflowObject,
	succ map[model.ID][]*microflows.SequenceFlow,
) (microflownorm.Region, error) {
	region := microflownorm.Region{}
	seen := map[model.ID]bool{}

	var walk func(id model.ID) error
	walk = func(id model.ID) error {
		if id == entry || id == join || id == "" {
			return nil
		}
		if seen[id] {
			return nil
		}
		seen[id] = true

		obj, ok := nodes[id]
		if !ok {
			return fmt.Errorf("the branches leave the microflow before rejoining")
		}

		switch n := obj.(type) {
		case *microflows.ExclusiveSplit:
			expr, ok := n.SplitCondition.(*microflows.ExpressionSplitCondition)
			if !ok {
				return fmt.Errorf("a decision in the region is rule-based, so its guard cannot be folded into an expression")
			}
			t, fl := findBranchFlows(succ[id])
			if t == nil || fl == nil {
				return fmt.Errorf("a decision in the region does not have both branches")
			}
			region[id] = microflownorm.Node{
				Kind:      microflownorm.KindSplit,
				Condition: expr.Expression,
				True:      t.DestinationID,
				False:     fl.DestinationID,
			}
			if err := walk(t.DestinationID); err != nil {
				return err
			}
			return walk(fl.DestinationID)

		case *microflows.ExclusiveMerge:
			out := succ[id]
			if len(out) != 1 {
				return fmt.Errorf("a merge in the region does not have exactly one successor")
			}
			region[id] = microflownorm.Node{Kind: microflownorm.KindMerge, Next: out[0].DestinationID}
			return walk(out[0].DestinationID)

		default:
			// An activity. Folding would run it under a different condition,
			// or not at all.
			return microflownorm.ErrImpureRegion
		}
	}

	if err := walk(splitID); err != nil {
		return nil, err
	}
	return region, nil
}

func branchFlow(from, to model.ID, isTrue bool) *microflows.SequenceFlow {
	expr := "false"
	if isTrue {
		expr = "true"
	}
	return &microflows.SequenceFlow{
		OriginID:      from,
		DestinationID: to,
		CaseValue:     &microflows.ExpressionCase{Expression: expr},
	}
}

// shallowCopyCollection copies the slices and the objects the fold mutates, so
// normalizing never reaches the stored model. Objects the fold does not touch
// are shared, which is safe because DESCRIBE only reads them.
func shallowCopyCollection(oc *microflows.MicroflowObjectCollection) *microflows.MicroflowObjectCollection {
	out := &microflows.MicroflowObjectCollection{
		Objects: make([]microflows.MicroflowObject, 0, len(oc.Objects)),
		Flows:   make([]*microflows.SequenceFlow, 0, len(oc.Flows)),
	}
	for _, o := range oc.Objects {
		if split, ok := o.(*microflows.ExclusiveSplit); ok {
			clone := *split
			out.Objects = append(out.Objects, &clone)
			continue
		}
		out.Objects = append(out.Objects, o)
	}
	out.Flows = append(out.Flows, oc.Flows...)
	return out
}

// spliceRedundantMerge removes a merge left with a single incoming and a single
// outgoing flow, rewiring its predecessor straight to its successor.
//
// Folding routinely creates one: the entry to the shared suffix had several
// predecessors before the fold and has exactly one after it. Leaving it in
// would make Mode 3's output describe a node that its own re-execution drops.
func spliceRedundantMerge(
	id model.ID,
	objects []microflows.MicroflowObject,
	flows []*microflows.SequenceFlow,
) ([]microflows.MicroflowObject, []*microflows.SequenceFlow) {
	isMerge := false
	for _, o := range objects {
		if o != nil && o.GetID() == id {
			_, isMerge = o.(*microflows.ExclusiveMerge)
			break
		}
	}
	if !isMerge {
		return objects, flows
	}

	var incoming, outgoing []*microflows.SequenceFlow
	for _, fl := range flows {
		if fl == nil {
			continue
		}
		if fl.DestinationID == id {
			incoming = append(incoming, fl)
		}
		if fl.OriginID == id {
			outgoing = append(outgoing, fl)
		}
	}
	if len(incoming) != 1 || len(outgoing) != 1 {
		return objects, flows
	}

	// Rewire the one predecessor past the merge, preserving its branch label.
	incoming[0].DestinationID = outgoing[0].DestinationID

	var keptObjects []microflows.MicroflowObject
	for _, o := range objects {
		if o != nil && o.GetID() == id {
			continue
		}
		keptObjects = append(keptObjects, o)
	}
	var keptFlows []*microflows.SequenceFlow
	for _, fl := range flows {
		if fl == outgoing[0] {
			continue
		}
		keptFlows = append(keptFlows, fl)
	}
	return keptObjects, keptFlows
}
