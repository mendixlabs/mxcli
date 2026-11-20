// SPDX-License-Identifier: Apache-2.0

package executor

// Labelling merges for DESCRIBE, so a rejoin can be written down.
//
// A Mendix ExclusiveMerge has no name. MDL's `merge <label>` / `join <label>`
// invents one for the length of a description, which is what lets DESCRIBE emit
// a graph whose paths do not nest.
//
// The case this exists for is an ERROR path that rejoins the normal one. Before
// it, `collectErrorHandlerStatements` stopped dead at the merge and emitted an
// empty `on error … begin end error` block — MDL that re-executes to a DIFFERENT graph, with
// no warning. Measured: repointing SUB_Feedback_SendToServer's error edge from
// the tail merge to an upstream one produced byte-identical MDL, and executing
// it reproduced the tail-merge graph.
//
// Scope is deliberately narrow. Only merges an error handler rejoins are
// labelled — not every irreducible split, which still gets the #923 warning.
// A label emitted where the nested description already reproduces the graph
// would be noise, and worse, would churn the describe output of every project.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mergeLabels maps an ExclusiveMerge to the label DESCRIBE will give it, and
// records which of those merges are CROSSED.
//
// The zero value is the ordinary case and every lookup on it is a miss, so
// callers need no nil check.
//
// The crossed set is carried here rather than threaded as a second parameter
// because the label alone does not say how a merge is emitted, and the two
// emissions are opposites:
//
//   - a rejoin merge is DECLARED where the traversal meets it, because the
//     normal path owns it and only an error handler needs to name it;
//   - a crossed merge is reached by two or more branches of the same split, so
//     each branch ends in `join <label>` and the declaration is emitted once,
//     after the branches close.
type mergeLabels struct {
	byID    map[model.ID]string
	crossed map[model.ID]bool
	// inline names the one-input merges of the main line, which a description
	// spells as written: `join <label>; merge <label>;`. nil describes them by
	// walking through, as every caller outside DESCRIBE MICROFLOW still does.
	inline *inlineMerges
}

// inlineMerges is the third emission, for a merge with ONE way in. Nothing
// needs to name such a merge — it joins nothing — but `exec` writes one from
// ordinary MDL (`join finish; merge finish;`), and a description that walked
// through it then warned that re-executing it deleted the node
// (mendixlabs/mxcli#923).
//
// Only on the main line (indent 0): a `merge` there follows a `join` with no
// open path, so the builder draws exactly the one edge. In a branch body it
// would also receive the branch's fall-through edge, and MDL-FLOW04 bans
// labels in a loop body, so a one-input merge there is still walked through
// and still warned about.
//
// Labels are handed out in the order the description reaches the merges, so
// the same flow always describes the same way.
type inlineMerges struct {
	eligible map[model.ID]bool
	byID     map[model.ID]string
}

// newInlineMerges finds the merges of oc's own collection (not a loop body's)
// that exactly one normal flow enters and no flow of any kind enters twice.
func newInlineMerges(oc *microflows.MicroflowObjectCollection) *inlineMerges {
	out := &inlineMerges{eligible: map[model.ID]bool{}, byID: map[model.ID]string{}}
	if oc == nil {
		return out
	}
	in, errIn := map[model.ID]int{}, map[model.ID]int{}
	for _, f := range allFlows(oc) {
		if f == nil {
			continue
		}
		in[f.DestinationID]++
		if f.IsErrorHandler {
			errIn[f.DestinationID]++
		}
	}
	for _, o := range oc.Objects {
		if _, ok := o.(*microflows.ExclusiveMerge); ok && in[o.GetID()] == 1 && errIn[o.GetID()] == 0 {
			out.eligible[o.GetID()] = true
		}
	}
	return out
}

// inlineLabel returns the label a one-input merge on the main line is
// described under, minting the next one the first time it is asked.
func (m mergeLabels) inlineLabel(id model.ID) (string, bool) {
	if m.inline == nil || !m.inline.eligible[id] {
		return "", false
	}
	if l, ok := m.inline.byID[id]; ok {
		return l, true
	}
	l := fmt.Sprintf("junction%d", len(m.inline.byID)+1)
	m.inline.byID[id] = l
	return l, true
}

// isInlined reports whether the description spelled this merge inline.
func (m mergeLabels) isInlined(id model.ID) bool {
	if m.inline == nil {
		return false
	}
	_, ok := m.inline.byID[id]
	return ok
}

func (m mergeLabels) of(id model.ID) (string, bool) {
	l, ok := m.byID[id]
	return l, ok
}

// isCrossed reports whether a branch reaching this merge must `join` it rather
// than fall into it.
func (m mergeLabels) isCrossed(id model.ID) bool { return m.crossed[id] }

// len is the number of labelled merges, for tests and for the empty check.
func (m mergeLabels) len() int { return len(m.byID) }

// labelRejoinMerges finds the merges that an error handler reaches and that the
// normal path also reaches — the ones a nested description cannot spell — and
// gives each a stable label.
//
// "The normal path also reaches it" is the discriminating half. An error handler
// whose own path happens to end at a merge of its own is ordinary and already
// describes correctly; it is the SHARED merge that makes the graph irreducible,
// because the handler has to say "carry on where the main path is".
func labelRejoinMerges(col *microflows.MicroflowObjectCollection) mergeLabels {
	if col == nil {
		return mergeLabels{}
	}

	objects := map[model.ID]microflows.MicroflowObject{}
	var startID model.ID
	for _, o := range col.Objects {
		if o == nil {
			continue
		}
		objects[o.GetID()] = o
		if _, ok := o.(*microflows.StartEvent); ok {
			startID = o.GetID()
		}
	}

	normalSucc := map[model.ID][]model.ID{}
	var errorFlows []*microflows.SequenceFlow
	for _, f := range col.Flows {
		if f == nil {
			continue
		}
		if f.IsErrorHandler {
			errorFlows = append(errorFlows, f)
			continue
		}
		normalSucc[f.OriginID] = append(normalSucc[f.OriginID], f.DestinationID)
	}
	if len(errorFlows) == 0 {
		return mergeLabels{}
	}

	reachable := map[model.ID]bool{}
	var walk func(model.ID)
	walk = func(id model.ID) {
		if id == "" || reachable[id] {
			return
		}
		reachable[id] = true
		for _, s := range normalSucc[id] {
			walk(s)
		}
	}
	walk(startID)

	// The merge each handler settles on: the first one reachable from where the
	// error edge lands, following the handler's own path.
	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{}
	for _, f := range col.Flows {
		if f != nil {
			flowsByOrigin[f.OriginID] = append(flowsByOrigin[f.OriginID], f)
		}
	}

	incoming := incomingCounts(flowsByOrigin)
	needsLabel := map[model.ID]bool{}
	for _, ef := range errorFlows {
		m := firstMergeFrom(ef.DestinationID, objects, normalSucc, incoming)
		if m == "" || !reachable[m] {
			continue
		}
		// A handler that falls through is written without a label, as it was
		// authored (#750): the merge is the guarded activity's own successor.
		if fallThroughRejoinMerge(ef.OriginID, flowsByOrigin, objects) == m {
			continue
		}
		needsLabel[m] = true
	}
	if len(needsLabel) == 0 {
		return mergeLabels{}
	}

	// Label in position order so the same graph always describes the same way —
	// map iteration order would make DESCRIBE output unstable, which turns every
	// re-describe into a spurious diff.
	ids := make([]model.ID, 0, len(needsLabel))
	for id := range needsLabel {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		pi, pj := objects[ids[i]].GetPosition(), objects[ids[j]].GetPosition()
		if pi.X != pj.X {
			return pi.X < pj.X
		}
		if pi.Y != pj.Y {
			return pi.Y < pj.Y
		}
		return ids[i] < ids[j]
	})

	out := mergeLabels{byID: make(map[model.ID]string, len(ids))}
	for i, id := range ids {
		out.byID[id] = fmt.Sprintf("rejoin%d", i+1)
	}
	return out
}

// firstMergeFrom walks forward from a node over normal edges and returns the
// first ExclusiveMerge it meets, or "" if the path terminates without one.
//
// Breadth-first, so "first" means nearest rather than whichever branch the walk
// happened to take.
//
// A merge with a single way in joins nothing — Studio Pro leaves them behind
// when a flow is re-routed — so it is walked through, not settled on
// (isNoOpMerge). Settling on one is how a handler's retry path back to a loop
// header was cut short (Evora: SnowflakeRESTSQL.GET_v1_RetrievePartition).
func firstMergeFrom(
	start model.ID,
	objects map[model.ID]microflows.MicroflowObject,
	succ map[model.ID][]model.ID,
	incoming map[model.ID]int,
) model.ID {
	if start == "" {
		return ""
	}
	if _, ok := objects[start].(*microflows.ExclusiveMerge); ok && !isNoOpMerge(start, objects, incoming) {
		return start
	}
	seen := map[model.ID]bool{start: true}
	queue := []model.ID{start}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, next := range succ[id] {
			if seen[next] {
				continue
			}
			seen[next] = true
			if _, ok := objects[next].(*microflows.ExclusiveMerge); ok && !isNoOpMerge(next, objects, incoming) {
				return next
			}
			queue = append(queue, next)
		}
	}
	return ""
}

// isNoOpMerge reports whether id is an ExclusiveMerge with exactly one way in:
// a junction of nothing, which no path needs to name.
func isNoOpMerge(id model.ID, objects map[model.ID]microflows.MicroflowObject, incoming map[model.ID]int) bool {
	_, isMerge := objects[id].(*microflows.ExclusiveMerge)
	return isMerge && incoming[id] == 1
}

// incomingCounts counts every flow — error flows included — into each object.
func incomingCounts(flowsByOrigin map[model.ID][]*microflows.SequenceFlow) map[model.ID]int {
	out := map[model.ID]int{}
	for _, flows := range flowsByOrigin {
		for _, f := range flows {
			if f != nil {
				out[f.DestinationID]++
			}
		}
	}
	return out
}

// mergeDeclarationLines renders `merge <label>;` with the merge's stored
// position, so describe → exec → describe is a fixed point.
//
// Without the @position the rebuild places the merge wherever the layout cursor
// happens to be, and the SECOND description reports a different coordinate —
// a diff on every re-describe of an unchanged microflow, which is exactly what
// ADR-0008's idempotence is for.
//
// A canonical DESCRIBE leaves the position out when the layout engine would put
// the merge there anyway (layout, see derivedFlowLayout): the rebuild then lands
// it on the same spot without being told.
func mergeDeclarationLines(indent int, label string, obj microflows.MicroflowObject, layout *flowLayoutKeep, flowsByOrigin map[model.ID][]*microflows.SequenceFlow) []string {
	pad := strings.Repeat("  ", indent)
	var out []string
	if obj != nil && layout.keepsPosition(obj.GetID()) {
		p := obj.GetPosition()
		out = append(out, fmt.Sprintf("%s@position(%d, %d)", pad, p.X, p.Y))
	}
	// The side the flow LEAVES the merge on, when it is not the default right.
	// An if whose closing merge is this one prints that side as its own
	// `@anchor(from: …)` (mergeExitAnchor, #767), but once the merge is declared
	// on its own the rebuilt if has no closing merge to apply it to, and the
	// exit came back from the right (mendixlabs/mxcli#1414). The declaration is
	// the statement that owns the edge, so it carries the side.
	if obj != nil {
		for _, flow := range flowsByOrigin[obj.GetID()] {
			if flow == nil || flow.IsErrorHandler {
				continue
			}
			if flow.OriginConnectionIndex != AnchorRight {
				out = append(out, fmt.Sprintf("%s@anchor(from: %s)", pad, anchorSideKeyword(flow.OriginConnectionIndex)))
			}
			break
		}
	}
	return append(out, pad+"merge "+label+";")
}

// droppedMergeWarnings flags every ExclusiveMerge the description does not
// represent — and which a describe → exec round trip therefore DELETES.
//
// Three shapes are represented and must stay quiet, because between them they
// cover every merge in a real project that survives the round trip:
//
//   - the join point of an exclusive/inheritance split, rendered implicitly by
//     `end if` / `end split`;
//   - a labelled error rejoin, rendered explicitly as `merge <label>`; and
//   - any merge with two or more incoming flows, which is a genuine convergence
//     the describer renders as the continuation of whatever construct it closes.
//
// That last one is not redundant with the first, and the case that proves it is
// a split where one branch `return`s: findMergeForSplit needs a join common to
// ALL branches, so it finds none, yet the merge is still emitted as the
// continuation after `end split` and survives. Measured on
// Administration.ManageMyAccount (Administration 4.3.2), whose merge keeps its
// $ID across describe → exec; without the in-degree clause it is a false
// positive, and a warning that cries wolf on ordinary Marketplace code is worse
// than no warning.
//
// What is left — a merge with a SINGLE incoming path — is walked straight
// through by the describer and represented by nothing at all. Measured across
// the microflows of a blank 11.14 app plus FeedbackModule: every one-input
// merge is deleted by describe → exec (7 of 7) and every other survivor keeps
// its $ID. It is behaviourally harmless (a one-input merge is a no-op) but it
// deletes a node the user drew, which is what guard-don't-drop exists to
// prevent. MDL-FLOW01 does not cover it: the graph is perfectly reducible, so
// the irreducibility detector is right to stay silent and something else has to
// speak.
//
// Deliberately NOT covered here: a merge lost to the flattening of an
// IRREDUCIBLE graph (one two-input merge of
// FeedbackModule.SUB_Feedback_SendToServer goes this way). That microflow
// already carries MDL-FLOW01, which says the description is not equivalent and
// must not be re-executed at all — a strictly stronger statement than this
// warning, and the right owner for it.
//
// Loop bodies are recursed into because a LoopedActivity owns its own object
// collection and its own traversal; without that every in-loop if/else merge
// would report as dropped. A loop's collection holds the body's OBJECTS but not
// its flows: Mendix stores every flow of the microflow in the microflow's own
// collection. So each body is judged against the flows of the whole microflow
// — taken against its own collection alone, every merge in a loop body had
// in-degree 0 and was reported dropped (ako/mxcli#942).
func droppedMergeWarnings(ctx *ExecContext, oc *microflows.MicroflowObjectCollection, labels mergeLabels) []string {
	return droppedMergeWarningsIn(ctx, oc, allFlows(oc), labels)
}

// allFlows is every sequence flow of a microflow: its own collection's and,
// should a loop's collection hold any, those too.
func allFlows(oc *microflows.MicroflowObjectCollection) []*microflows.SequenceFlow {
	if oc == nil {
		return nil
	}
	flows := append([]*microflows.SequenceFlow(nil), oc.Flows...)
	for _, o := range oc.Objects {
		if loop, ok := o.(*microflows.LoopedActivity); ok {
			flows = append(flows, allFlows(loop.ObjectCollection)...)
		}
	}
	return flows
}

func droppedMergeWarningsIn(ctx *ExecContext, oc *microflows.MicroflowObjectCollection, flows []*microflows.SequenceFlow, labels mergeLabels) []string {
	if oc == nil {
		return nil
	}

	activityMap := map[model.ID]microflows.MicroflowObject{}
	for _, o := range oc.Objects {
		if o != nil {
			activityMap[o.GetID()] = o
		}
	}

	// Merges that a split joins on are spelled by `end if` / `end split`.
	represented := map[model.ID]bool{}
	flowsByOrigin := make(map[model.ID][]*microflows.SequenceFlow)
	for _, f := range flows {
		if f != nil {
			flowsByOrigin[f.OriginID] = append(flowsByOrigin[f.OriginID], f)
		}
	}
	for _, mergeID := range findSplitMergePointsForGraph(ctx, activityMap, flowsByOrigin) {
		represented[mergeID] = true
	}

	// A merge two or more paths arrive at is a real convergence and is rendered
	// as the continuation of whatever construct closes there — including the
	// split-with-a-returning-branch that findSplitMergePoints cannot pair up.
	inDegree := map[model.ID]int{}
	for _, f := range flows {
		if f != nil {
			inDegree[f.DestinationID]++
		}
	}

	var out []string
	for _, o := range oc.Objects {
		merge, ok := o.(*microflows.ExclusiveMerge)
		if !ok {
			continue
		}
		if represented[merge.GetID()] || inDegree[merge.GetID()] >= 2 {
			continue
		}
		if _, labelled := labels.of(merge.GetID()); labelled || labels.isInlined(merge.GetID()) {
			continue
		}
		// `create or modify` compares this description with the stored flow's
		// own, and neither has the merge, so it leaves the node in place; only a
		// rebuild — `create or replace`, or an mdl 0 fallback — draws the flow
		// without it. The warning used to say re-executing deleted it, which
		// stopped being true when modify learned to write nothing (#923).
		p := merge.GetPosition()
		out = append(out, fmt.Sprintf(
			"-- WARNING: the merge at (%d, %d) is not represented in this description - "+
				"it joins no decision and no error handler. `create or modify` from this MDL leaves it in place, "+
				"but anything that rebuilds the flow from it (`create or replace`, an mdl 0 fallback) removes it. "+
				"The microflow behaves the same either way (a merge with one incoming path is a no-op), "+
				"but the node disappears from the diagram (mendixlabs/mxcli#923)",
			p.X, p.Y))
	}

	// A loop body is described by its own traversal, so its merges are judged
	// against its own collection.
	for _, o := range oc.Objects {
		if loop, ok := o.(*microflows.LoopedActivity); ok {
			out = append(out, droppedMergeWarningsIn(ctx, loop.ObjectCollection, flows, labels)...)
		}
	}
	return out
}

// fallThroughRejoinMerge returns the merge where the custom error handler of
// source rejoins the normal path when that rejoin is a FALL-THROUGH — the shape
// `on error begin … end error;` with no `join` builds — and "" otherwise.
//
// A fall-through is exactly this, and anything looser is a goto the labels must
// keep spelling:
//
//   - the source's only normal flow goes straight to the merge;
//   - the merge has two incoming flows, that one and the handler's;
//   - the handler's own path settles on that merge (firstMergeFrom), so its
//     body is what the description prints inside the braces.
//
// Before #750 this shape came back as `join rejoin1;` in the handler and a
// `merge rejoin1;` after the activity — correct, but not what anyone wrote.
func fallThroughRejoinMerge(
	source model.ID,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	objects map[model.ID]microflows.MicroflowObject,
) model.ID {
	var errFlow *microflows.SequenceFlow
	var normal []*microflows.SequenceFlow
	for _, f := range flowsByOrigin[source] {
		if f == nil {
			continue
		}
		if f.IsErrorHandler {
			errFlow = f
			continue
		}
		normal = append(normal, f)
	}
	if errFlow == nil || len(normal) != 1 {
		return ""
	}
	m := normal[0].DestinationID
	if _, ok := objects[m].(*microflows.ExclusiveMerge); !ok {
		return ""
	}
	incoming := 0
	for _, flows := range flowsByOrigin {
		for _, f := range flows {
			if f != nil && f.DestinationID == m {
				incoming++
			}
		}
	}
	if incoming != 2 {
		return ""
	}
	succ := map[model.ID][]model.ID{}
	for origin, flows := range flowsByOrigin {
		for _, f := range flows {
			if f != nil && !f.IsErrorHandler {
				succ[origin] = append(succ[origin], f.DestinationID)
			}
		}
	}
	if firstMergeFrom(errFlow.DestinationID, objects, succ, incomingCounts(flowsByOrigin)) != m {
		return ""
	}
	return m
}
