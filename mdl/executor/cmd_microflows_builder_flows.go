// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow graph: sequence flow constructors and error handler flows
package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// convertErrorHandlingType converts AST error handling type to SDK error handling type.
func convertErrorHandlingType(eh *ast.ErrorHandlingClause) microflows.ErrorHandlingType {
	if eh == nil {
		return microflows.ErrorHandlingTypeRollback
	}
	switch eh.Type {
	case ast.ErrorHandlingContinue:
		return microflows.ErrorHandlingTypeContinue
	case ast.ErrorHandlingRollback:
		return microflows.ErrorHandlingTypeRollback
	case ast.ErrorHandlingCustom:
		return microflows.ErrorHandlingTypeCustom
	case ast.ErrorHandlingCustomWithoutRollback:
		return microflows.ErrorHandlingTypeCustomWithoutRollback
	default:
		return microflows.ErrorHandlingTypeRollback
	}
}

// ehType returns the error handling type for an activity in this flow context.
// Nanoflows default to "Abort" because they have no transactions; microflows
// default to "Rollback". An explicit ON ERROR clause always overrides the default.
//
// Most builders want THIS, not explicitErrorHandling below, and the two are not
// interchangeable — picking the wrong one is a silent CE6035. Which is right
// depends entirely on what the call site did before:
//
//   - A builder that already supplied a default here (every create/change/log/
//     page/message/validation activity) must keep using ehType. Returning empty
//     discards the flow flavour, and the writer's literal "Rollback" is CE6035 on
//     every un-annotated activity in a NANOFLOW, whose default is Abort. That is
//     mendixlabs/mxcli#1078's regression: green unit suite, 11 errors under
//     `make test-integration`.
//   - Retrieve and Delete use explicitErrorHandling because their writers emitted
//     a hardcoded "Rollback" that those two actions accept in every flow flavour,
//     so empty is a no-op there.
//
// Same helper pair, opposite correct answer. Ask what the old expression returned
// in EVERY context before replacing it, not just the one under test.
func (fb *flowBuilder) ehType(eh *ast.ErrorHandlingClause) microflows.ErrorHandlingType {
	if fb.isNanoflow && eh == nil {
		return microflows.ErrorHandlingTypeAbort
	}
	return convertErrorHandlingType(eh)
}

// explicitErrorHandling returns the handling type for an action ONLY when the
// author wrote an ON ERROR clause, and the empty string otherwise.
//
// The distinction is the whole safety of carrying this property at all. The
// writers have always emitted a literal for these actions, and the value the
// builder would otherwise supply is not always one Mendix accepts: a NANOFLOW's
// no-clause default is Abort (ehType), and Abort is CE6035 on Log, Create,
// Change, Commit, MicroflowCall and Aggregate — measured on 11.14.0 by setting
// each in a stored document and building. Returning empty for "no clause" leaves
// every un-annotated activity exactly as it was.
func explicitErrorHandling(fb *flowBuilder, eh *ast.ErrorHandlingClause) microflows.ErrorHandlingType {
	if eh == nil {
		return ""
	}
	return fb.ehType(eh)
}

func isCustomErrorHandling(eh *ast.ErrorHandlingClause) bool {
	return eh != nil && (eh.Type == ast.ErrorHandlingCustom || eh.Type == ast.ErrorHandlingCustomWithoutRollback)
}

// rejoinMergePosition is where the merge a fall-through handler of source
// rejoins at goes: the @merge written on the source, or the computed fallback.
func (fb *flowBuilder) rejoinMergePosition(source model.ID, computed model.Point) model.Point {
	if p := fb.rejoinMergeAt[source]; p != nil {
		return model.Point{X: p.X, Y: p.Y}
	}
	return computed
}

// mergeOnEdge places a merge inserted into the normal edge from originID to
// destinationID: midway between the right edge of the one and the left edge of
// the other, on the line of the node it enters.
//
// The fallback — the layout cursor less half a spacing — is only right while
// the cursor still points at the successor, and by the time a rejoin is wired
// the successor has been placed and the cursor moved past it. The merge then
// landed ON an end event (same centre, the end inside the 40px merge) and
// RIGHT of an activity, so the edge into it ran backwards (mendixlabs/mxcli#884,
// ako/mxcli#1273). The fallback still covers an edge that does not run left to
// right, such as one that wraps onto the next row.
func (fb *flowBuilder) mergeOnEdge(originID, destinationID model.ID, fallback model.Point) model.Point {
	origin, dest := fb.objectByID(originID), fb.objectByID(destinationID)
	if origin == nil || dest == nil {
		return fallback
	}
	right := origin.GetPosition().X + objectSize(origin).Width/2
	left := dest.GetPosition().X - objectSize(dest).Width/2
	if left <= right {
		return fallback
	}
	return model.Point{X: (right + left) / 2, Y: dest.GetPosition().Y}
}

// objectByID returns this builder's object with the given ID, or nil.
func (fb *flowBuilder) objectByID(id model.ID) microflows.MicroflowObject {
	for i := len(fb.objects) - 1; i >= 0; i-- {
		if o := fb.objects[i]; o != nil && o.GetID() == id {
			return o
		}
	}
	return nil
}

func isEmptyCustomErrorHandler(eh *ast.ErrorHandlingClause) bool {
	if eh == nil || len(eh.Body) != 0 {
		return false
	}
	return eh.Type == ast.ErrorHandlingCustom || eh.Type == ast.ErrorHandlingCustomWithoutRollback
}

func (fb *flowBuilder) finishCustomErrorHandler(activityID model.ID, activityX int, eh *ast.ErrorHandlingClause, outputVar string) {
	if eh == nil {
		return
	}
	// The statement's own annotations are still pending while it is built, so a
	// @merge on it — the rejoin merge's position (#750) — is read here.
	if ann := fb.pendingAnnotations; ann != nil && ann.Merge != nil && isCustomErrorHandling(eh) {
		if fb.rejoinMergeAt == nil {
			fb.rejoinMergeAt = map[model.ID]*ast.Position{}
		}
		fb.rejoinMergeAt[activityID] = ann.Merge
	}
	if len(eh.Body) > 0 {
		// Retry-loop pattern: error body ends with an IF whose non-terminating
		// branch should loop back to the source activity (Studio Pro authors
		// this as a merge placed before the source, with the handler tail
		// returning to that merge). MDL cannot express the loop-back directly,
		// so we detect the shape and wire the topology ourselves.
		if isRetryLoopErrorHandler(eh.Body) {
			fb.buildRetryLoopErrorHandler(activityID, activityX, eh.Body)
			return
		}
		mergeID := fb.addErrorHandlerFlow(activityID, activityX, eh.Body)
		fb.handleErrorHandlerMergeWithSkip(mergeID, activityID, outputVar)
		return
	}
	fb.registerEmptyCustomErrorHandlerWithSkip(activityID, eh, outputVar)
}

// isRetryLoopErrorHandler reports whether the error-handler body looks like
// a retry loop: the last statement is an IF whose else branch terminates
// (via RAISE ERROR or RETURN) and whose then branch continues. That shape
// mirrors the Studio Pro retry pattern where the non-terminating branch
// loops back to the source activity to re-attempt it.
func isRetryLoopErrorHandler(body []ast.MicroflowStatement) bool {
	if len(body) == 0 {
		return false
	}
	ifStmt, ok := body[len(body)-1].(*ast.IfStmt)
	if !ok {
		return false
	}
	if len(ifStmt.ThenBody) == 0 || len(ifStmt.ElseBody) == 0 {
		return false
	}
	thenTerminates := bodyTerminates(ifStmt.ThenBody)
	elseTerminates := bodyTerminates(ifStmt.ElseBody)
	// Exactly one branch must terminate (the "error" branch) and one must
	// continue (the "retry" branch). If both or neither terminate, the shape
	// is some other IF and the standard merge-forward path still applies.
	return thenTerminates != elseTerminates
}

// bodyTerminates reports whether a statement body ends with a terminator
// (RAISE ERROR, RETURN, or nested IF where every branch terminates).
func bodyTerminates(body []ast.MicroflowStatement) bool {
	if len(body) == 0 {
		return false
	}
	last := body[len(body)-1]
	switch s := last.(type) {
	case *ast.RaiseErrorStmt:
		return true
	case *ast.ReturnStmt:
		return true
	case *ast.IfStmt:
		if len(s.ElseBody) == 0 {
			return false
		}
		return bodyTerminates(s.ThenBody) && bodyTerminates(s.ElseBody)
	}
	return false
}

// buildRetryLoopErrorHandler wires a retry-loop topology for an error handler
// whose body ends with a terminating/continuing IF. The non-terminating
// branch's tail loops back to a new merge placed before the source activity;
// the merge then feeds into the source. The outer loop consumes
// fb.incomingRedirect so the normal inbound flow also terminates at the
// merge instead of directly at the source.
func (fb *flowBuilder) buildRetryLoopErrorHandler(sourceActivityID model.ID, sourceX int, errorBody []ast.MicroflowStatement) {
	// Build the handler activities with tracking of where the non-terminating
	// branch's tail ends up. We reuse addErrorHandlerFlow; it returns the tail
	// that would have forwarded to the next main-path activity. For a retry
	// loop, that tail is the continue-branch exit of the trailing IF.
	tail := fb.addErrorHandlerFlow(sourceActivityID, sourceX, errorBody)
	if tail.id == "" {
		// The handler terminates unexpectedly — nothing to loop back.
		return
	}

	// Insert a merge just left of the source activity, on the main flow row.
	merge := &microflows.ExclusiveMerge{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: sourceX - fb.spacing/2, Y: fb.baseY},
			Size:        model.Size{Width: MergeSize, Height: MergeSize},
		},
	}
	fb.objects = append(fb.objects, merge)

	// Merge -> source (normal flow into the REST/microflow call)
	fb.flows = append(fb.flows, newHorizontalFlow(merge.ID, sourceActivityID))

	// Handler tail -> merge (loop-back). Authored Studio Pro flows mark this
	// edge as a normal SequenceFlow (not IsErrorHandler) — the error flow
	// marker only applies to the SOURCE → first-handler-activity edge, which
	// addErrorHandlerFlow already emitted. Using a plain horizontal flow here
	// avoids triggering spurious CE0136/CE0019 diagnostics that fire when a
	// retrieve's start variable is deemed to flow through an error edge.
	loopFlow := newHorizontalFlow(tail.id, merge.ID)
	if tail.caseValue != "" {
		applyDeferredFlowCase(loopFlow, tail.caseValue, tail.flowAnchor)
	} else if tail.flowAnchor != nil {
		applyDeferredFlowCase(loopFlow, "", tail.flowAnchor)
	}
	fb.flows = append(fb.flows, loopFlow)

	// Next inbound normal flow must terminate at the merge, not at the source.
	fb.incomingRedirect = merge.ID
}

func (fb *flowBuilder) registerEmptyCustomErrorHandlerWithSkip(activityID model.ID, eh *ast.ErrorHandlingClause, skipVar string) {
	if !isEmptyCustomErrorHandler(eh) {
		return
	}
	fb.queueActivePendingErrorHandler()
	if skipVar == "" {
		fb.emptyErrorHandlerFrom = activityID
		return
	}
	fb.errorHandlerSource = activityID
	fb.errorHandlerTailFrom = activityID
	fb.errorHandlerSkipVar = skipVar
	fb.errorHandlerTailIsSource = true
}

type pendingErrorHandlerState struct {
	emptyFrom    model.ID
	tailFrom     model.ID
	source       model.ID
	skipVar      string
	tailCase     string
	tailAnchor   *ast.FlowAnchors
	tailIsSource bool
	returnValue  string
}

func (s pendingErrorHandlerState) activeIsEmpty() bool {
	return s.emptyFrom == "" && s.tailFrom == "" && s.source == "" && s.skipVar == ""
}

func (fb *flowBuilder) activePendingErrorHandler() pendingErrorHandlerState {
	return pendingErrorHandlerState{
		emptyFrom:    fb.emptyErrorHandlerFrom,
		tailFrom:     fb.errorHandlerTailFrom,
		source:       fb.errorHandlerSource,
		skipVar:      fb.errorHandlerSkipVar,
		tailCase:     fb.errorHandlerTailCase,
		tailAnchor:   fb.errorHandlerTailAnchor,
		tailIsSource: fb.errorHandlerTailIsSource,
		returnValue:  fb.errorHandlerReturnValue,
	}
}

func (fb *flowBuilder) setActivePendingErrorHandler(state pendingErrorHandlerState) {
	fb.emptyErrorHandlerFrom = state.emptyFrom
	fb.errorHandlerTailFrom = state.tailFrom
	fb.errorHandlerSource = state.source
	fb.errorHandlerSkipVar = state.skipVar
	fb.errorHandlerTailCase = state.tailCase
	fb.errorHandlerTailAnchor = state.tailAnchor
	fb.errorHandlerTailIsSource = state.tailIsSource
	fb.errorHandlerReturnValue = state.returnValue
}

func (fb *flowBuilder) queueActivePendingErrorHandler() {
	state := fb.activePendingErrorHandler()
	if state.activeIsEmpty() {
		return
	}
	fb.pendingErrorHandlers = append(fb.pendingErrorHandlers, state)
	fb.setActivePendingErrorHandler(pendingErrorHandlerState{})
}

func (fb *flowBuilder) rewritePendingErrorHandlers(rewrite func(pendingErrorHandlerState) pendingErrorHandlerState) {
	queue := fb.pendingErrorHandlers[:0]
	for _, state := range fb.pendingErrorHandlers {
		state = rewrite(state)
		if !state.activeIsEmpty() {
			queue = append(queue, state)
		}
	}
	fb.pendingErrorHandlers = queue

	active := rewrite(fb.activePendingErrorHandler())
	fb.setActivePendingErrorHandler(active)
}

// branchErrorHandlerScope holds the handlers still pending when a branch of a
// split starts: they belong to the activities before the split, not to it.
type branchErrorHandlerScope struct {
	active pendingErrorHandlerState
	queued []pendingErrorHandlerState
}

// enterBranchErrorHandlers sets the pending handlers aside while a branch is
// built, so the branch settles its own handlers on its own flows, as the top
// level does. A fall-through handler in a branch was left pending past the
// branch and rejoined on the flow INTO the split (a retry loop when the call
// assigns a variable) or ended the microflow (mendixlabs/mxcli#1565).
func (fb *flowBuilder) enterBranchErrorHandlers() branchErrorHandlerScope {
	outer := branchErrorHandlerScope{active: fb.activePendingErrorHandler(), queued: fb.pendingErrorHandlers}
	fb.setActivePendingErrorHandler(pendingErrorHandlerState{})
	fb.pendingErrorHandlers = nil
	return outer
}

// takeBranchErrorHandlers restores the handlers set aside and returns those of
// the branch it could not settle.
func (fb *flowBuilder) takeBranchErrorHandlers(outer branchErrorHandlerScope) []pendingErrorHandlerState {
	inner := fb.pendingErrorHandlers
	if active := fb.activePendingErrorHandler(); !active.activeIsEmpty() {
		inner = append(inner, active)
	}
	fb.pendingErrorHandlers = outer.queued
	fb.setActivePendingErrorHandler(outer.active)
	return inner
}

// queueBranchErrorHandlersAt queues the handlers the branches of a split could
// not settle, to rejoin on the flow that leaves the split from exitID — its
// merge, or the last activity of its one continuing branch — which the parent
// draws later. A handler with a result settles on whatever flow comes next,
// and the next flow the parent draws is the one INTO the split: a loop.
func (fb *flowBuilder) queueBranchErrorHandlersAt(handlers []pendingErrorHandlerState, exitID model.ID) {
	for _, state := range handlers {
		if state.skipVar != "" && state.tailFrom != "" {
			state.skipVar = ""
			state.source = exitID
		}
		fb.pendingErrorHandlers = append(fb.pendingErrorHandlers, state)
	}
}

// settleBranchErrorHandlersOn settles handlers a branch left pending on the
// flow that leaves it, originID → destinationID, which must already be drawn:
// a handler on the branch's last activity rejoins there. It returns those it
// could not settle.
func (fb *flowBuilder) settleBranchErrorHandlersOn(handlers []pendingErrorHandlerState, originID, destinationID model.ID) []pendingErrorHandlerState {
	if len(handlers) == 0 {
		return nil
	}
	outer := fb.enterBranchErrorHandlers()
	fb.pendingErrorHandlers = handlers
	fb.addPendingErrorHandlerFlowForStatement(originID, destinationID, nil)
	return fb.takeBranchErrorHandlers(outer)
}

// addBranchErrorHandlerFlowForStatement settles the branch's handlers on a flow
// drawn inside the branch, as addPendingErrorHandlerFlowForStatement does at the
// top level. In a branch that ends in a return, a handler on an activity with a
// result is left for the code after the split: it is the shared error path of a
// branch whose remaining statements build on the result
// (TestBuildFlowGraph_OutputHandlerInReturningBranchSkipsDerivedSuccessTail).
func (fb *flowBuilder) addBranchErrorHandlerFlowForStatement(branchReturns bool, originID, destinationID model.ID, stmt ast.MicroflowStatement, futureReferencesSkipVar bool) {
	fb.rewritePendingErrorHandlers(func(state pendingErrorHandlerState) pendingErrorHandlerState {
		if branchReturns && state.skipVar != "" {
			return state
		}
		return fb.addPendingErrorHandlerFlowForState(state, originID, destinationID, stmt, futureReferencesSkipVar)
	})
}

func (fb *flowBuilder) addPendingErrorHandlerFlowForStatement(originID, destinationID model.ID, stmt ast.MicroflowStatement, futureReferencesSkipVar ...bool) {
	futureReferences := len(futureReferencesSkipVar) > 0 && futureReferencesSkipVar[0]
	fb.rewritePendingErrorHandlers(func(state pendingErrorHandlerState) pendingErrorHandlerState {
		return fb.addPendingErrorHandlerFlowForState(state, originID, destinationID, stmt, futureReferences)
	})
}

func (fb *flowBuilder) addPendingErrorHandlerFlowForState(state pendingErrorHandlerState, originID, destinationID model.ID, stmt ast.MicroflowStatement, futureReferencesSkipVar bool) pendingErrorHandlerState {
	if destinationID == "" {
		return state
	}
	if state.emptyFrom != "" {
		if state.emptyFrom != originID {
			return state
		}
		fb.addEmptyErrorHandlerRejoinFlowFrom(originID, state.emptyFrom, destinationID)
		state.emptyFrom = ""
	}
	if state.tailFrom == "" {
		return state
	}
	if state.source != "" && destinationID == state.source {
		return state
	}
	if state.skipVar != "" {
		if statementReferencesVar(stmt, state.skipVar) {
			if !fb.hasDeclaredReturnValue() {
				if derivedVar := outputDerivedVariable(stmt, state.skipVar); derivedVar != "" {
					state.skipVar = derivedVar
				}
				return state
			}
			return state
		}
		if futureReferencesSkipVar {
			return state
		}
		fb.addErrorHandlerRejoinFlowForState(state, originID, destinationID)
		return pendingErrorHandlerState{}
	}
	if state.source != "" && state.source == originID {
		fb.addErrorHandlerRejoinFlowForState(state, originID, destinationID)
		return pendingErrorHandlerState{}
	}
	return state
}

type errorHandlerTail struct {
	id         model.ID
	caseValue  string
	flowAnchor *ast.FlowAnchors
}

func (fb *flowBuilder) addEmptyErrorHandlerRejoinFlowFrom(normalOriginID, errorOriginID, destinationID model.ID) {
	existingIdx := -1
	for i := len(fb.flows) - 1; i >= 0; i-- {
		flow := fb.flows[i]
		if !flow.IsErrorHandler && flow.OriginID == normalOriginID && flow.DestinationID == destinationID {
			existingIdx = i
			break
		}
	}
	if existingIdx == -1 {
		if mergeID := fb.findExistingRejoinMerge(normalOriginID, destinationID); mergeID != "" {
			fb.flows = append(fb.flows, newErrorHandlerFlow(errorOriginID, mergeID))
			return
		}
		fb.flows = append(fb.flows, newErrorHandlerFlow(errorOriginID, destinationID))
		return
	}

	existing := fb.flows[existingIdx]
	fb.flows = append(fb.flows[:existingIdx], fb.flows[existingIdx+1:]...)

	merge := &microflows.ExclusiveMerge{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    fb.rejoinMergePosition(errorOriginID, fb.mergeOnEdge(normalOriginID, destinationID, model.Point{X: fb.posX - HorizontalSpacing/2, Y: fb.baseY})),
			Size:        model.Size{Width: MergeSize, Height: MergeSize},
		},
	}
	fb.objects = append(fb.objects, merge)

	normalFlow := newHorizontalFlow(normalOriginID, merge.ID)
	normalFlow.OriginConnectionIndex = existing.OriginConnectionIndex
	normalFlow.CaseValue = existing.CaseValue
	fb.flows = append(fb.flows, normalFlow)
	fb.flows = append(fb.flows, newErrorHandlerFlow(errorOriginID, merge.ID))

	mergeFlow := newHorizontalFlow(merge.ID, destinationID)
	mergeFlow.DestinationConnectionIndex = existing.DestinationConnectionIndex
	fb.flows = append(fb.flows, mergeFlow)
}

func (fb *flowBuilder) addErrorHandlerRejoinFlowForState(state pendingErrorHandlerState, originID, destinationID model.ID) {
	existingIdx := -1
	for i := len(fb.flows) - 1; i >= 0; i-- {
		flow := fb.flows[i]
		if !flow.IsErrorHandler && flow.OriginID == originID && flow.DestinationID == destinationID {
			existingIdx = i
			break
		}
	}
	if existingIdx == -1 {
		if mergeID := fb.findExistingRejoinMerge(originID, destinationID); mergeID != "" {
			if state.tailIsSource {
				fb.flows = append(fb.flows, newErrorHandlerFlow(state.tailFrom, mergeID))
			} else {
				flow := newUpwardFlow(state.tailFrom, mergeID)
				applyDeferredFlowCase(flow, state.tailCase, state.tailAnchor)
				fb.flows = append(fb.flows, flow)
			}
			return
		}
		if state.tailIsSource {
			fb.flows = append(fb.flows, newErrorHandlerFlow(state.tailFrom, destinationID))
		} else {
			flow := newHorizontalFlow(state.tailFrom, destinationID)
			applyDeferredFlowCase(flow, state.tailCase, state.tailAnchor)
			fb.flows = append(fb.flows, flow)
		}
		return
	}

	existing := fb.flows[existingIdx]
	fb.flows = append(fb.flows[:existingIdx], fb.flows[existingIdx+1:]...)

	merge := &microflows.ExclusiveMerge{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    fb.rejoinMergePosition(state.source, fb.mergeOnEdge(originID, destinationID, model.Point{X: fb.posX - HorizontalSpacing/2, Y: fb.baseY})),
			Size:        model.Size{Width: MergeSize, Height: MergeSize},
		},
	}
	fb.objects = append(fb.objects, merge)

	normalFlow := newHorizontalFlow(originID, merge.ID)
	normalFlow.OriginConnectionIndex = existing.OriginConnectionIndex
	normalFlow.CaseValue = existing.CaseValue
	fb.flows = append(fb.flows, normalFlow)
	if state.tailIsSource {
		fb.flows = append(fb.flows, newErrorHandlerFlow(state.tailFrom, merge.ID))
	} else {
		flow := newUpwardFlow(state.tailFrom, merge.ID)
		applyDeferredFlowCase(flow, state.tailCase, state.tailAnchor)
		fb.flows = append(fb.flows, flow)
	}

	mergeFlow := newHorizontalFlow(merge.ID, destinationID)
	mergeFlow.DestinationConnectionIndex = existing.DestinationConnectionIndex
	fb.flows = append(fb.flows, mergeFlow)
}

func applyDeferredFlowCase(flow *microflows.SequenceFlow, caseValue string, anchor *ast.FlowAnchors) {
	if flow == nil {
		return
	}
	if caseValue != "" {
		flow.CaseValue = microflows.EnumerationCase{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Value:       caseValue,
		}
	}
	applyUserAnchors(flow, anchor, anchor)
}

func (fb *flowBuilder) findExistingRejoinMerge(originID, destinationID model.ID) model.ID {
	// Error-handler rejoins are rare and microflows are small enough that an
	// O(objects*flows) scan keeps the write path simpler than maintaining an
	// incremental merge index.
	for _, flow := range fb.flows {
		if flow.OriginID != originID || flow.IsErrorHandler {
			continue
		}
		if !fb.isExclusiveMerge(flow.DestinationID) {
			continue
		}
		for _, mergeFlow := range fb.flows {
			if mergeFlow.OriginID == flow.DestinationID && mergeFlow.DestinationID == destinationID && !mergeFlow.IsErrorHandler {
				return flow.DestinationID
			}
		}
	}
	return ""
}

func (fb *flowBuilder) isExclusiveMerge(id model.ID) bool {
	for _, obj := range fb.objects {
		if obj.GetID() != id {
			continue
		}
		_, ok := obj.(*microflows.ExclusiveMerge)
		return ok
	}
	return false
}

func statementReferencesVar(stmt ast.MicroflowStatement, varName string) bool {
	if stmt == nil || varName == "" {
		return false
	}
	for _, ref := range errorHandlerStatementVarRefs(stmt) {
		if ref == varName {
			return true
		}
	}
	return false
}

func statementsReferenceVar(stmts []ast.MicroflowStatement, varName string) bool {
	if varName == "" {
		return false
	}
	for _, stmt := range stmts {
		if statementReferencesVar(stmt, varName) {
			return true
		}
	}
	return false
}

func callArgumentVarRefs(args []ast.CallArgument) []string {
	var refs []string
	for _, arg := range args {
		refs = append(refs, exprVarRefs(arg.Value)...)
	}
	return refs
}

func templateParamVarRefs(params []ast.TemplateParam) []string {
	var refs []string
	for _, param := range params {
		refs = append(refs, exprVarRefs(param.Value)...)
	}
	return refs
}

func errorHandlerStatementVarRefs(stmt ast.MicroflowStatement) []string {
	var refs []string
	switch s := stmt.(type) {
	case *ast.DeclareStmt:
		refs = append(refs, exprVarRefs(s.InitialValue)...)
	case *ast.ReturnStmt:
		refs = append(refs, exprVarRefs(s.Value)...)
	case *ast.LogStmt:
		refs = append(refs, exprVarRefs(s.Node)...)
		refs = append(refs, exprVarRefs(s.Message)...)
		refs = append(refs, templateParamVarRefs(s.Template)...)
	case *ast.ShowMessageStmt:
		refs = append(refs, exprVarRefs(s.Message)...)
		for _, arg := range s.TemplateArgs {
			refs = append(refs, exprVarRefs(arg)...)
		}
	case *ast.IfStmt:
		refs = append(refs, exprVarRefs(s.Condition)...)
		refs = append(refs, errorHandlerStatementsVarRefs(s.ThenBody)...)
		refs = append(refs, errorHandlerStatementsVarRefs(s.ElseBody)...)
	case *ast.WhileStmt:
		refs = append(refs, exprVarRefs(s.Condition)...)
		refs = append(refs, errorHandlerStatementsVarRefs(s.Body)...)
	case *ast.LoopStmt:
		refs = append(refs, s.ListVariable)
		refs = append(refs, errorHandlerStatementsVarRefs(s.Body)...)
	case *ast.MfSetStmt:
		refs = append(refs, extractVarName(s.Target))
		refs = append(refs, exprVarRefs(s.Value)...)
	case *ast.ChangeObjectStmt:
		refs = append(refs, s.Variable)
		for _, change := range s.Changes {
			refs = append(refs, exprVarRefs(change.Value)...)
		}
	case *ast.CreateObjectStmt:
		for _, change := range s.Changes {
			refs = append(refs, exprVarRefs(change.Value)...)
		}
	case *ast.RetrieveStmt:
		if s.StartVariable != "" {
			refs = append(refs, s.StartVariable)
		}
		refs = append(refs, exprVarRefs(s.Where)...)
	case *ast.CallMicroflowStmt:
		refs = append(refs, callArgumentVarRefs(s.Arguments)...)
	case *ast.CallNanoflowStmt:
		refs = append(refs, callArgumentVarRefs(s.Arguments)...)
	case *ast.CallJavaActionStmt:
		refs = append(refs, callArgumentVarRefs(s.Arguments)...)
	case *ast.CallJavaScriptActionStmt:
		refs = append(refs, callArgumentVarRefs(s.Arguments)...)
	case *ast.CallWebServiceStmt:
		refs = append(refs, exprVarRefs(s.Timeout)...)
	case *ast.ExecuteDatabaseQueryStmt:
		refs = append(refs, callArgumentVarRefs(s.Arguments)...)
		refs = append(refs, callArgumentVarRefs(s.ConnectionArguments)...)
	case *ast.CallExternalActionStmt:
		refs = append(refs, callArgumentVarRefs(s.Arguments)...)
	case *ast.RestCallStmt:
		refs = append(refs, exprVarRefs(s.URL)...)
		refs = append(refs, templateParamVarRefs(s.URLParams)...)
		for _, header := range s.Headers {
			refs = append(refs, exprVarRefs(header.Value)...)
		}
		if s.Auth != nil {
			refs = append(refs, exprVarRefs(s.Auth.Username)...)
			refs = append(refs, exprVarRefs(s.Auth.Password)...)
		}
		if s.Body != nil {
			refs = append(refs, exprVarRefs(s.Body.Template)...)
			refs = append(refs, templateParamVarRefs(s.Body.TemplateParams)...)
			if s.Body.SourceVariable != "" {
				refs = append(refs, s.Body.SourceVariable)
			}
		}
		refs = append(refs, exprVarRefs(s.Timeout)...)
	case *ast.SendEmailStmt:
		for _, e := range s.Expressions() {
			refs = append(refs, exprVarRefs(e)...)
		}
		if s.Attachment != "" {
			refs = append(refs, s.Attachment)
		}
	case *ast.SendRestRequestStmt:
		for _, param := range s.Parameters {
			refs = append(refs, sourceAttributeVarRefs(param.Expression)...)
		}
		if s.BodyVariable != "" {
			refs = append(refs, s.BodyVariable)
		}
	case *ast.ImportFromMappingStmt:
		if s.SourceVariable != "" {
			refs = append(refs, s.SourceVariable)
		}
	case *ast.ExportToMappingStmt:
		if s.SourceVariable != "" {
			refs = append(refs, s.SourceVariable)
		}
	case *ast.TransformJsonStmt:
		if s.InputVariable != "" {
			refs = append(refs, s.InputVariable)
		}
	case *ast.MfCommitStmt:
		refs = append(refs, s.Variable)
	case *ast.DeleteObjectStmt:
		refs = append(refs, s.Variable)
	case *ast.DownloadFileStmt:
		refs = append(refs, s.FileDocument)
	case *ast.SynchronizeStmt:
		// Specific mode names existing object/list variables; All and
		// Unsynchronized name none.
		refs = append(refs, s.Variables...)
	case *ast.ValidationFeedbackStmt:
		if s.AttributePath != nil {
			refs = append(refs, s.AttributePath.Variable)
		}
		refs = append(refs, exprVarRefs(s.Message)...)
		for _, arg := range s.TemplateArgs {
			refs = append(refs, exprVarRefs(arg)...)
		}
	case *ast.AddToListStmt:
		if s.Value != nil {
			refs = append(refs, exprVarRefs(s.Value)...)
		} else if s.Item != "" {
			refs = append(refs, s.Item)
		}
		refs = append(refs, s.List)
	case *ast.RemoveFromListStmt:
		refs = append(refs, s.Item, s.List)
	case *ast.ClearListStmt:
		refs = append(refs, s.List)
	}
	return refs
}

func outputDerivedVariable(stmt ast.MicroflowStatement, sourceVar string) string {
	declare, ok := stmt.(*ast.DeclareStmt)
	if !ok || declare.Variable == "" {
		return ""
	}
	for _, ref := range exprVarRefs(declare.InitialValue) {
		if ref == sourceVar {
			return declare.Variable
		}
	}
	return ""
}

func errorHandlerStatementsVarRefs(stmts []ast.MicroflowStatement) []string {
	var refs []string
	for _, stmt := range stmts {
		refs = append(refs, errorHandlerStatementVarRefs(stmt)...)
	}
	return refs
}

// newErrorHandlerFlow creates a SequenceFlow with IsErrorHandler=true,
// connecting from the bottom of the source activity to the top of the handler.
// Studio Pro lays custom error handlers below their source, so the destination
// anchor enters from above rather than from the normal left-side continuation.
func newErrorHandlerFlow(originID, destinationID model.ID) *microflows.SequenceFlow {
	return &microflows.SequenceFlow{
		BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:                   originID,
		DestinationID:              destinationID,
		OriginConnectionIndex:      AnchorBottom,
		DestinationConnectionIndex: AnchorTop,
		IsErrorHandler:             true,
	}
}

// addErrorHandlerFlow builds error handler activities from the given body statements,
// positions them below the source activity, and connects them with an error handler flow.
// Returns the last activity ID if the error handler should merge back to the main flow.
// Returns empty model.ID if the error handler terminates (via RAISE ERROR or RETURN).
func (fb *flowBuilder) addErrorHandlerFlow(sourceActivityID model.ID, sourceX int, errorBody []ast.MicroflowStatement) errorHandlerTail {
	if len(errorBody) == 0 {
		return errorHandlerTail{}
	}

	// Position error handler below the main flow
	errorY := fb.posY + ErrorHandlerPitch
	errorX := sourceX

	// Build error handler activities
	errBuilder := &flowBuilder{
		textLang:     fb.textLang,
		posX:         errorX,
		posY:         errorY,
		baseY:        errorY,
		spacing:      HorizontalSpacing,
		returnType:   fb.returnType,
		varTypes:     fb.varTypes,
		declaredVars: fb.declaredVars,
		measurer:     fb.measurer,
		backend:      fb.backend,
		quiet:        fb.quiet,
		hierarchy:    fb.hierarchy,
		restServices: fb.restServices,
		isNanoflow:   fb.isNanoflow,
		self:         fb.self,
		scriptFlows:  fb.scriptFlows,
		// A handler's activities are merged into the PARENT's object collection
		// below, so a note declared outside the handler and referenced inside it
		// (or the reverse) lands in one collection — sharing the registry is
		// sound here in a way it is not across a loop boundary (#1077).
		annotationsByLabel: fb.annotationsByLabel,
		// Same collection, so the same label table: `join recovered` inside a
		// handler must find the `merge recovered` declared on the main path.
		// This is the one rejoin MDL could not spell before, and the reason the
		// registry is a shared pointer rather than per-builder state.
		labelReg: fb.labels(),
	}

	var lastErrID model.ID
	var lastErrCase string
	var lastErrAnchor *ast.FlowAnchors
	// prevOwnAnchor is the @anchor of the handler statement before this one,
	// whose `from:` is the side the edge between them leaves. The loop below
	// built its edges with the default sides and never applied either end, so
	// an @anchor inside a handler parsed, passed check and exec, and did
	// nothing (mendixlabs/mxcli#991).
	var prevOwnAnchor *ast.FlowAnchors
	for _, stmt := range errorBody {
		thisAnchor := stmtOwnAnchor(stmt)
		actID := errBuilder.addStatement(stmt)
		if errBuilder.pendingJoin != nil {
			// A handler whose FIRST statement is a join has no activity of its
			// own: the ERROR edge itself has to land on the merge, so it is
			// created here rather than recorded as an ordinary join edge.
			if lastErrID == "" {
				label := errBuilder.pendingJoin.Label
				errBuilder.pendingJoin = nil
				m := errBuilder.mergeForLabel(label)
				fb.flows = append(fb.flows, newErrorHandlerFlow(sourceActivityID, m.ID))
				// Wired here rather than by resolveJoins, so it still counts as
				// handled — otherwise the unwired-body-loop guard would report a
				// join that was in fact wired.
				errBuilder.labels().handled++
				continue
			}
			errBuilder.takePendingJoin(lastErrID, lastErrCase, lastErrAnchor)
			continue
		}
		if actID != "" {
			errBuilder.applyPendingAnnotations(actID)
			if lastErrID == "" {
				// Connect source activity to first error handler activity. The
				// statement's `to:` is the side its incoming edge — this one —
				// enters; the side the edge leaves the source has no spelling.
				flow := newErrorHandlerFlow(sourceActivityID, actID)
				applyUserAnchors(flow, nil, thisAnchor)
				fb.flows = append(fb.flows, flow)
			} else {
				flow := newHorizontalFlow(lastErrID, actID)
				applyUserAnchors(flow, prevOwnAnchor, thisAnchor)
				errBuilder.flows = append(errBuilder.flows, flow)
			}
			prevOwnAnchor = thisAnchor
			if errBuilder.nextConnectionPoint != "" {
				lastErrID = errBuilder.nextConnectionPoint
				lastErrCase = errBuilder.nextFlowCase
				lastErrAnchor = errBuilder.nextFlowAnchor
				errBuilder.nextConnectionPoint = ""
				errBuilder.nextFlowCase = ""
				errBuilder.nextFlowAnchor = nil
				// A compound statement steers its own exits.
				prevOwnAnchor = nil
			} else {
				lastErrID = actID
				lastErrCase = ""
				// The edge that rejoins the main flow leaves this statement, so
				// it takes the statement's `from:` — and only that: its `to:`
				// belongs to the edge coming in.
				lastErrAnchor = originOnly(thisAnchor)
			}
		}
	}

	// Append error handler objects and flows to the main builder.
	//
	// annotationFlows and errors are part of that: without them a note written
	// inside `on error begin … end error` arrived as an Annotation with no edge — a
	// free-floating sticky note instead of one attached to the activity — and a
	// refusal raised in the handler body never reached the caller (#1077).
	fb.objects = append(fb.objects, errBuilder.objects...)
	fb.flows = append(fb.flows, errBuilder.flows...)
	fb.annotationFlows = append(fb.annotationFlows, errBuilder.annotationFlows...)
	fb.errors = append(fb.errors, errBuilder.errors...)
	// The handler's objects land in this collection, so its stated notes are
	// this builder's to pin too.
	for id := range errBuilder.statedNotes {
		if fb.statedNotes == nil {
			fb.statedNotes = map[model.ID]bool{}
		}
		fb.statedNotes[id] = true
	}
	// A @curve inside the handler is recorded on errBuilder, and applied by the
	// builder that owns the flows once its graph is complete — this one, since
	// the handler's flows (and the edge that rejoins the main flow) land here.
	// Left on errBuilder it was never applied (mendixlabs/mxcli#991).
	fb.adoptCurves(errBuilder)
	if fb.annotationsByLabel == nil {
		fb.annotationsByLabel = errBuilder.annotationsByLabel
	}
	// A `return` in the handler drew its end event into this collection too,
	// so it is one of this builder's returns: the splice keeps it as a new end
	// event of the flow rather than refusing it as an end the builder added
	// (ako/mxcli#905). lastReturnEndID is left alone — it is the fallback
	// target of the main path's own handlers.
	for id := range errBuilder.returnEndIDs {
		if fb.returnEndIDs == nil {
			fb.returnEndIDs = map[model.ID]bool{}
		}
		fb.returnEndIDs[id] = true
	}

	// If the error handler ends with RAISE ERROR or RETURN, it terminates there.
	// Otherwise, return the last activity ID so caller can create a merge.
	if errBuilder.endsWithReturn {
		return errorHandlerTail{} // Error handler terminates, no merge needed
	}
	return errorHandlerTail{id: lastErrID, caseValue: lastErrCase, flowAnchor: lastErrAnchor} // Error handler should merge back to main flow
}

// handleErrorHandlerMerge creates an EndEvent for error handlers that want to merge back.
// This is a fallback until full merge support is implemented. Caller should pass
// the tail returned by addErrorHandlerFlow and the error handler Y position.
func (fb *flowBuilder) handleErrorHandlerMerge(tail errorHandlerTail, activityID model.ID, errorY int) {
	_ = errorY
	fb.handleErrorHandlerMergeWithSkip(tail, activityID, "")
}

func (fb *flowBuilder) handleErrorHandlerMergeWithSkip(tail errorHandlerTail, activityID model.ID, skipVar string) {
	if tail.id == "" {
		return // No merge needed (error handler terminates with RETURN or RAISE ERROR)
	}
	fb.queueActivePendingErrorHandler()
	fb.errorHandlerSource = activityID
	fb.errorHandlerTailFrom = tail.id
	fb.errorHandlerSkipVar = skipVar
	fb.errorHandlerTailCase = tail.caseValue
	fb.errorHandlerTailAnchor = tail.flowAnchor
	fb.errorHandlerTailIsSource = false
	fb.errorHandlerReturnValue = fb.returnValue
}

// newHorizontalFlow creates a SequenceFlow with anchors for horizontal left-to-right connection
func newHorizontalFlow(originID, destinationID model.ID) *microflows.SequenceFlow {
	return &microflows.SequenceFlow{
		BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:                   originID,
		DestinationID:              destinationID,
		OriginConnectionIndex:      AnchorRight, // Connect from right side of origin
		DestinationConnectionIndex: AnchorLeft,  // Connect to left side of destination
	}
}

// newHorizontalFlowWithCase creates a horizontal SequenceFlow with a boolean case value (for splits)
func newHorizontalFlowWithCase(originID, destinationID model.ID, caseValue string) *microflows.SequenceFlow {
	flow := newHorizontalFlow(originID, destinationID)
	flow.CaseValue = caseValueForFlow(caseValue)
	return flow
}

func newHorizontalFlowWithEnumCase(originID, destinationID model.ID, caseValue string) *microflows.SequenceFlow {
	flow := newHorizontalFlow(originID, destinationID)
	flow.CaseValue = microflows.EnumerationCase{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		Value:       caseValue,
	}
	return flow
}

func newHorizontalFlowWithInheritanceCase(originID, destinationID model.ID, entity string) *microflows.SequenceFlow {
	flow := newHorizontalFlow(originID, destinationID)
	flow.CaseValue = &microflows.InheritanceCase{
		BaseElement:         model.BaseElement{ID: model.ID(types.GenerateID())},
		EntityQualifiedName: entity,
	}
	return flow
}

// newDownwardFlowWithCase creates a SequenceFlow going down from origin (Bottom) to destination (Left)
// Used when TRUE path goes below the main line
func newDownwardFlowWithCase(originID, destinationID model.ID, caseValue string) *microflows.SequenceFlow {
	return &microflows.SequenceFlow{
		BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:                   originID,
		DestinationID:              destinationID,
		OriginConnectionIndex:      AnchorBottom, // Connect from bottom of origin (going down)
		DestinationConnectionIndex: AnchorLeft,   // Connect to left side of destination
		CaseValue:                  caseValueForFlow(caseValue),
	}
}

func caseValueForFlow(caseValue string) microflows.CaseValue {
	switch caseValue {
	case "true":
		return &microflows.ExpressionCase{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Expression:  "true",
		}
	case "false":
		return &microflows.ExpressionCase{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Expression:  "false",
		}
	default:
		return microflows.EnumerationCase{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Value:       caseValue,
		}
	}
}

func newDownwardFlowWithInheritanceCase(originID, destinationID model.ID, entity string) *microflows.SequenceFlow {
	return &microflows.SequenceFlow{
		BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:                   originID,
		DestinationID:              destinationID,
		OriginConnectionIndex:      AnchorBottom,
		DestinationConnectionIndex: AnchorLeft,
		CaseValue: &microflows.InheritanceCase{
			BaseElement:         model.BaseElement{ID: model.ID(types.GenerateID())},
			EntityQualifiedName: entity,
		},
	}
}

// newUpwardFlow creates a SequenceFlow going up from origin (Right) to destination (Top)
// Used when returning from a lower branch to merge
func newUpwardFlow(originID, destinationID model.ID) *microflows.SequenceFlow {
	return &microflows.SequenceFlow{
		BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:                   originID,
		DestinationID:              destinationID,
		OriginConnectionIndex:      AnchorRight,  // Connect from right side of origin
		DestinationConnectionIndex: AnchorBottom, // Connect to bottom of destination (going up)
	}
}

// applyUserAnchors overrides a SequenceFlow's Origin/Destination connection
// indices with user-specified values from @anchor annotations. An AnchorSideUnset
// value leaves the builder-chosen default in place.
//
// Arguments are taken as *ast.FlowAnchors pointers for the origin and
// destination statements. Either can be nil (no annotation on that side).
// If both are non-nil, the origin's From and destination's To are applied.
func applyUserAnchors(flow *microflows.SequenceFlow, origin *ast.FlowAnchors, destination *ast.FlowAnchors) {
	if flow == nil {
		return
	}
	if origin != nil && origin.From != ast.AnchorSideUnset {
		flow.OriginConnectionIndex = int(origin.From)
	}
	if destination != nil && destination.To != ast.AnchorSideUnset {
		flow.DestinationConnectionIndex = int(destination.To)
	}
}

// originOnly is an anchor's `from:` alone, for an edge whose destination the
// statement does not own.
func originOnly(a *ast.FlowAnchors) *ast.FlowAnchors {
	if a == nil || a.From == ast.AnchorSideUnset {
		return nil
	}
	return &ast.FlowAnchors{From: a.From, To: ast.AnchorSideUnset}
}

func branchDestinationAnchor(branchAnchor, stmtAnchor *ast.FlowAnchors) *ast.FlowAnchors {
	// The split branch annotation owns the incoming edge to the first branch
	// activity. If it specifies `to`, it must win over the first statement's
	// own anchor; the statement anchor applies to that activity's outgoing
	// edge, not to the split->statement flow.
	if branchAnchor != nil && branchAnchor.To != ast.AnchorSideUnset {
		return branchAnchor
	}
	return stmtAnchor
}

func pendingFlowAnchors(previousAnchor, pendingAnchor, stmtAnchor *ast.FlowAnchors) (*ast.FlowAnchors, *ast.FlowAnchors) {
	if pendingAnchor == nil {
		return previousAnchor, stmtAnchor
	}
	return pendingAnchor, branchDestinationAnchor(pendingAnchor, stmtAnchor)
}

// lastStmtIsReturn reports whether execution of a body is guaranteed to terminate
// (via RETURN, RAISE ERROR, BREAK, or CONTINUE) on every path — i.e. control can
// never fall off the end of the body into the parent flow.
//
// Terminal statements: ReturnStmt, RaiseErrorStmt, BreakStmt, ContinueStmt. An
// Branching statements are terminal iff every branch is present and recursively
// terminal. A LoopStmt is never terminal — BREAK can exit the loop even if the
// body returns.
//
// Naming kept for history; the predicate is really "last stmt is a guaranteed
// terminator". Missing this case causes the outer IF to emit a dangling
// continuation flow (duplicate "true" edge + orphan EndEvent), which Studio Pro
// rejects as "Sequence contains no matching element" when diffing.
func lastStmtIsReturn(stmts []ast.MicroflowStatement) bool {
	if len(stmts) == 0 {
		return false
	}
	return isTerminalStmt(stmts[len(stmts)-1])
}

func isTerminalStmt(stmt ast.MicroflowStatement) bool {
	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.RaiseErrorStmt:
		return true
	case *ast.BreakStmt:
		return true
	case *ast.ContinueStmt:
		return true
	case *ast.JoinStmt:
		// A join ends the path at a named merge, so a branch ending in one owes
		// no flow to the enclosing IF's merge — exactly like a RETURN.
		return true
	case *ast.IfStmt:
		if len(s.ElseBody) == 0 {
			return false
		}
		return lastStmtIsReturn(s.ThenBody) && lastStmtIsReturn(s.ElseBody)
	case *ast.WhileStmt:
		return isManualWhileTrueCandidate(s)
	case *ast.EnumSplitStmt:
		if len(s.Cases) == 0 {
			return false
		}
		if len(s.ElseBody) > 0 && !lastStmtIsReturn(s.ElseBody) {
			return false
		}
		for _, c := range s.Cases {
			if !lastStmtIsReturn(c.Body) {
				return false
			}
		}
		// Every reachable branch terminates, so the split has no continuation
		// to thread into the parent flow. This intentionally diverges from
		// `bodyReturns` in validate_microflow.go: that predicate treats an
		// enum split without an `else` as non-terminal because authored MDL
		// is expected to supply a default branch covering missing values.
		// Here we also accept described-from-MPR graphs where Studio Pro
		// produced an exhaustive set of value cases without a default flow —
		// in both no-else and with-else forms the split terminates once we
		// reach this point.
		return true
	case *ast.InheritanceSplitStmt:
		if len(s.Cases) == 0 || len(s.ElseBody) == 0 || !lastStmtIsReturn(s.ElseBody) {
			return false
		}
		for _, c := range s.Cases {
			if !lastStmtIsReturn(c.Body) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func containsTerminalStmt(stmts []ast.MicroflowStatement) bool {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ReturnStmt, *ast.RaiseErrorStmt:
			return true
		case *ast.IfStmt:
			if containsTerminalStmt(s.ThenBody) || containsTerminalStmt(s.ElseBody) {
				return true
			}
		case *ast.LoopStmt:
			if containsTerminalStmt(s.Body) {
				return true
			}
		case *ast.WhileStmt:
			if containsTerminalStmt(s.Body) {
				return true
			}
		}
	}
	return false
}

// mergeOverConnectedEndEvents gives every end event that more than one path
// reaches an exclusive merge to join them at.
//
// An end event accepts exactly ONE incoming sequence flow — joining two paths
// is what a merge is for — so a second flow into one is CE0709 "Sequence flow
// is not accepted by origin or destination". Only mxbuild catches it: the
// document is otherwise well formed, so `mxcli check` passes and the project
// still opens, which is how this survived a describe → exec round trip of a
// whole project with everything else green.
//
// The shape that produced it is an empty `on error … begin end error` handler inside a
// branch whose sibling also returns:
//
//	if … then
//	  $r = call microflow M.Sub() on error without rollback begin end error;
//	  return $r;          -- the normal path reaches the end event
//	else
//	  return 'no';        -- and so does this one
//	end if;
//
// It runs as a post-pass rather than at the site that wires the error flow,
// because the two colliding flows are created by unrelated builders in either
// order: the error flow lands first and the branch's flow arrives afterwards,
// so neither site can see the collision. Same reasoning as applyFlowCurves.
//
// Merging is what Studio Pro writes here, and it is also what keeps the
// microflow's single return value — a second end event would need one of its
// own, and MDL never said what it should be.
//
// Scoped to end events on purpose. Other node types also accept one inbound
// flow, so the same collision is possible in principle, but an end event is the
// only one measured to occur (1 microflow in 41 across the audit corpus) and
// widening the rewrite to nodes with real inbound semantics — a loop, a merge's
// own feeders — without a case to test it on would be a guess.
func (fb *flowBuilder) mergeOverConnectedEndEvents() {
	endEvents := make(map[model.ID]bool)
	for _, obj := range fb.objects {
		if ev, ok := obj.(*microflows.EndEvent); ok {
			endEvents[ev.ID] = true
		}
	}
	if len(endEvents) == 0 {
		return
	}

	// First-seen order, so the merges are created deterministically rather than
	// in map order — a shuffled object list is a spurious diff on every write.
	var order []model.ID
	inbound := make(map[model.ID][]int)
	for i, flow := range fb.flows {
		if flow == nil || !endEvents[flow.DestinationID] {
			continue
		}
		if _, seen := inbound[flow.DestinationID]; !seen {
			order = append(order, flow.DestinationID)
		}
		inbound[flow.DestinationID] = append(inbound[flow.DestinationID], i)
	}

	for _, endID := range order {
		idxs := inbound[endID]
		if len(idxs) < 2 {
			continue
		}
		// On the edge of the path that arrives along the end event's own line, so
		// the merge does not land on the end event (see mergeOnEdge).
		pos := model.Point{X: fb.posX - HorizontalSpacing/2, Y: fb.baseY}
		if end := fb.objectByID(endID); end != nil {
			for _, i := range idxs {
				if o := fb.objectByID(fb.flows[i].OriginID); o != nil && o.GetPosition().Y == end.GetPosition().Y {
					pos = fb.mergeOnEdge(o.GetID(), endID, pos)
					break
				}
			}
		}
		merge := &microflows.ExclusiveMerge{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    pos,
				Size:        model.Size{Width: MergeSize, Height: MergeSize},
			},
		}
		fb.objects = append(fb.objects, merge)

		// The merge inherits how the first path entered the end event; the
		// re-pointed flows lose it, because a connection index describes an
		// edge's landing on a node and these now land on the merge.
		destIndex := fb.flows[idxs[0]].DestinationConnectionIndex
		for _, i := range idxs {
			fb.flows[i].DestinationID = merge.ID
			fb.flows[i].DestinationConnectionIndex = 0
		}
		mergeFlow := newHorizontalFlow(merge.ID, endID)
		mergeFlow.DestinationConnectionIndex = destIndex
		fb.flows = append(fb.flows, mergeFlow)
	}
}
