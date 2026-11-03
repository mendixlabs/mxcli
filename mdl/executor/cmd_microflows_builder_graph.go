// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow graph: graph construction and statement dispatch
package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// buildFlowGraph converts AST statements to a Microflow flow graph.
// Note: posY represents the CENTER LINE for element alignment, not the top position.
func (fb *flowBuilder) buildFlowGraph(stmts []ast.MicroflowStatement, returns *ast.MicroflowReturnType) *microflows.MicroflowObjectCollection {
	// Initialize maps if not set
	if fb.measurer == nil {
		fb.measurer = &layoutMeasurer{varTypes: fb.varTypes}
	}
	if fb.measurer.layout == nil {
		fb.measurer.layout = fb.layout
	}
	if fb.declaredVars == nil {
		fb.declaredVars = make(map[string]string)
	}
	if fb.listInputVariables == nil {
		fb.listInputVariables = collectListInputVariables(stmts)
	}
	if fb.objectInputVariables == nil {
		fb.objectInputVariables = collectObjectInputVariables(stmts)
	}
	// Set return value expression for error handler EndEvents
	fb.returnType = returns
	if returns != nil && returns.Variable != "" {
		fb.returnValue = "$" + returns.Variable
	}
	// Set baseY for branch restoration (this is the center line)
	fb.baseY = fb.posY

	// If the first statement carries an @position annotation, shift the StartEvent
	// to be one spacing unit to the left of that position so it doesn't end up
	// behind activities that use explicit coordinates.
	//
	// The first statement only. This used to take the first statement that had an
	// @position, wherever it was, so a position on a later statement moved the
	// start — and every unplaced statement before it, which follows the start — to
	// that statement's place: `declare $A …; @position(840, 200) declare $B …`
	// stored both at 840;200 (ako/mxcli#1768). Statements before a placed one keep
	// the places the builder derives for them, as alter's patch already assumed
	// (patchDiff.startEvent reads declared[0]).
	if len(stmts) > 0 {
		if ann := getStatementAnnotations(stmts[0]); ann != nil && ann.Position != nil {
			fb.posX = ann.Position.X - fb.spacing
			fb.posY = ann.Position.Y
			fb.baseY = fb.posY
		}
	}

	// Create StartEvent - Position is the CENTER point (RelativeMiddlePoint in Mendix)
	//
	// Three sources, weakest first. The derived spot above is the fallback; a
	// hand-placed position carried over from the flow being replaced beats it
	// (#884); an explicit @start(x, y) beats both, because it is the one source
	// that states the position rather than inferring it (#951).
	startX, startY := fb.posX, fb.posY
	if fb.startPosition != nil {
		startX, startY = fb.startPosition.X, fb.startPosition.Y
	}
	for _, stmt := range stmts {
		ann := getStatementAnnotations(stmt)
		if ann == nil || ann.Start == nil {
			continue
		}
		startX, startY = ann.Start.X, ann.Start.Y
		break
	}
	startEvent := &microflows.StartEvent{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: startX, Y: startY},
			Size:        model.Size{Width: EventSize, Height: EventSize},
		},
	}
	fb.objects = append(fb.objects, startEvent)
	lastID := startEvent.ID

	fb.posX += fb.spacing
	// The main line starts here, at the first statement's column. Rows wrap back to
	// it (layout_rows.go); a flow that never reaches MaxRowWidth never notices.
	fb.noteRowStart()

	// Process each statement
	// pendingCase holds the case value for the NEXT flow (set by merge-less splits)
	// pendingFlowAnchor carries branch anchors from a guard-pattern IF so the
	// deferred split→nextActivity flow honours @anchor(true: ..., false: ...).
	pendingCase := ""
	var pendingFlowAnchor *ast.FlowAnchors
	for i, stmt := range stmts {
		// Snapshot the current statement's anchor annotation before addStatement
		// can reset pendingAnnotations via recursive processing. The incoming
		// side (To) is applied when this statement is the destination of the
		// flow we're about to create; the outgoing side (From) is stashed in
		// previousStmtAnchor so the NEXT iteration can apply it.
		stmtAnchor := stmtOwnAnchor(stmt)

		// Whether the path is still open, read BEFORE the statement runs. A
		// `merge` reached after a RETURN or a `join` must not also gain a
		// fall-through edge from whatever came before — that path already ended,
		// and wiring it produced a duplicate flow into the merge.
		pathOpen := !fb.endsWithReturn

		// Wrap the main line before placing a statement that would run past the
		// readable width. A statement carrying its own @position places itself, so
		// it is left alone and becomes the start of the current row instead.
		if x, ok := ownPositionX(stmt); ok {
			// The statement places itself, so this is where the current row now
			// begins; wrapping measures from here on.
			fb.row.startX = x
			fb.row.firstObject = len(fb.objects)
		} else if fb.shouldWrap(stmt, stmts[i:]) {
			fb.wrapRow(startsWithNote(stmt))
		}

		activityID := fb.addStatement(stmt)
		if fb.takePendingJoin(lastID, pendingCase, fb.previousStmtAnchor) {
			pendingCase = ""
			fb.previousStmtAnchor = nil
			continue
		}
		if activityID != "" && !pathOpen {
			// Resuming after a terminator: this statement starts a new path, so
			// it gets no inbound edge here. Only a `merge` can legitimately be
			// reached that way — it is the one construct other paths join.
			fb.applyPendingAnnotations(activityID)
			lastID = activityID
			pendingCase = ""
			pendingFlowAnchor = nil
			fb.previousStmtAnchor = stmtAnchor
			continue
		}
		if activityID != "" {
			fb.applyPendingAnnotations(activityID)
			// Connect to previous object with horizontal SequenceFlow.
			// When incomingRedirect is set (retry-loop error handler built a
			// merge before the activity), the inbound flow terminates at the
			// merge instead of the activity itself.
			inboundTarget := activityID
			if fb.incomingRedirect != "" {
				inboundTarget = fb.incomingRedirect
				fb.incomingRedirect = ""
			}
			var flow *microflows.SequenceFlow
			if pendingCase != "" {
				flow = newHorizontalFlowWithCase(lastID, inboundTarget, pendingCase)
				pendingCase = ""
			} else {
				flow = newHorizontalFlow(lastID, inboundTarget)
			}
			// Prefer the pendingFlowAnchor (carried from a guard-pattern IF's
			// branch) over the previous statement's own anchor — it encodes
			// exactly the @anchor(true/false: ...) the user asked for on the
			// deferred flow. When the pending anchor is present it applies to
			// both From (origin side on the split) and To (the side of the
			// continuing activity), unless the incoming statement explicitly
			// overrides its own To.
			originAnchor, destAnchor := pendingFlowAnchors(fb.previousStmtAnchor, pendingFlowAnchor, stmtAnchor)
			pendingFlowAnchor = nil
			// An edge that crosses from one row to the next leaves the bottom of
			// the last element and enters the left of the first — anchored right
			// to left it is drawn straight back across the row above, through
			// whatever sits between the two columns.
			if from, to, wraps := fb.takeWrapAnchors(); wraps && originAnchor == nil && destAnchor == nil {
				flow.OriginConnectionIndex = from
				flow.DestinationConnectionIndex = to
			}
			applyUserAnchors(flow, originAnchor, destAnchor)
			fb.flows = append(fb.flows, flow)
			fb.addPendingErrorHandlerFlowForStatement(lastID, activityID, stmt, statementsReferenceVar(stmts[i+1:], fb.errorHandlerSkipVar))
			fb.previousStmtAnchor = stmtAnchor

			// For compound statements (IF, LOOP), the exit point differs from entry point
			if fb.nextConnectionPoint != "" {
				lastID = fb.nextConnectionPoint
				fb.nextConnectionPoint = ""
				// Save nextFlowCase / nextFlowAnchor for the NEXT iteration's flow creation
				pendingCase = fb.nextFlowCase
				fb.nextFlowCase = ""
				pendingFlowAnchor = fb.nextFlowAnchor
				fb.nextFlowAnchor = nil
				// Compound statements control their own internal anchors; don't
				// let the outer From leak into the flow leaving the merge.
				fb.previousStmtAnchor = nil
			} else {
				lastID = activityID
			}
		}
	}

	// Handle leftover pending annotations (free-floating annotation text)
	if fb.pendingAnnotations != nil {
		// Free annotations are standalone Annotation objects. Flush them before
		// creating the activity so they do not get attached to it; buildFlowGraph
		// has a final leftover flush for annotations with no following activity.
		for _, note := range fb.pendingAnnotations.FreeNotes {
			fb.attachFreeAnnotation(note)
		}
		// An attached note with no activity left to attach to is a free one.
		for _, note := range fb.pendingAnnotations.Notes {
			fb.attachFreeAnnotation(note)
		}
		fb.pendingAnnotations = nil
	}

	// Create EndEvent only if the flow doesn't already end with RETURN EndEvent(s)
	// (e.g., when both branches of an IF/ELSE end with RETURN, EndEvents are already created)
	if !fb.endsWithReturn {
		fb.posX += fb.spacing / 2
		fb.posY = fb.baseY // Ensure end event is on the happy path center line
		endEvent := &microflows.EndEvent{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: EventSize, Height: EventSize},
			},
			ReturnValue: fb.returnValue,
		}

		fb.objects = append(fb.objects, endEvent)
		fb.fallThroughEndID = endEvent.ID

		// Connect last activity to end event
		var endFlow *microflows.SequenceFlow
		if pendingCase != "" {
			endFlow = newHorizontalFlowWithCase(lastID, endEvent.ID, pendingCase)
		} else {
			endFlow = newHorizontalFlow(lastID, endEvent.ID)
		}
		originAnchor := fb.previousStmtAnchor
		if pendingFlowAnchor != nil {
			originAnchor = pendingFlowAnchor
			pendingFlowAnchor = nil
		}
		applyUserAnchors(endFlow, originAnchor, nil)
		fb.flows = append(fb.flows, endFlow)
		var endStmt ast.MicroflowStatement
		if len(fb.returnValue) > 1 && fb.returnValue[0] == '$' {
			endStmt = &ast.ReturnStmt{Value: &ast.VariableExpr{Name: fb.returnValue[1:]}}
		}
		fb.addPendingErrorHandlerFlowForStatement(lastID, endEvent.ID, endStmt)
		fb.terminatePendingErrorHandlersAtEnd(returns)
		fb.previousStmtAnchor = nil
	} else {
		fb.terminatePendingErrorHandlersAtEnd(returns)
	}

	// Stamp @curve onto each annotated activity's outgoing flows. Done once here,
	// after every flow exists, rather than at the seven-odd sites that create
	// one. (#884)
	fb.applyFlowCurves()

	// Give any end event that two paths reach a merge to join them at. Also
	// here, and for the same reason as the curves: the two flows can be created
	// by unrelated builders in either order, so no single creation site can see
	// the collision.
	fb.mergeOverConnectedEndEvents()

	// Named join points last: a `join` may reference a `merge` declared later,
	// so no edge can be drawn until every statement has been seen. Deliberately
	// after mergeOverConnectedEndEvents — a join lands on a merge, never on an
	// end event, so it cannot create the over-connection that pass fixes.
	fb.resolveJoins()

	// Rows last: every object exists and every container has its final size, so
	// this is the first point where a row's real depth is known.
	fb.separateRows()

	// Note lines after everything has moved: the sides that face each other
	// depend on where the note and its element finally sit.
	orientAnnotationFlows(fb.objects, fb.annotationFlows)

	return &microflows.MicroflowObjectCollection{
		BaseElement:     model.BaseElement{ID: model.ID(types.GenerateID())},
		Objects:         fb.objects,
		Flows:           fb.flows,
		AnnotationFlows: fb.annotationFlows,
	}
}

func collectListInputVariables(stmts []ast.MicroflowStatement) map[string]bool {
	inputs := make(map[string]bool)
	var walk func([]ast.MicroflowStatement)
	walk = func(body []ast.MicroflowStatement) {
		for _, stmt := range body {
			switch s := stmt.(type) {
			case *ast.ListOperationStmt:
				if s.InputVariable != "" {
					inputs[s.InputVariable] = true
				}
			case *ast.AggregateListStmt:
				if s.InputVariable != "" {
					inputs[s.InputVariable] = true
				}
			case *ast.LoopStmt:
				if s.ListVariable != "" {
					inputs[s.ListVariable] = true
				}
				walk(s.Body)
			case *ast.AddToListStmt:
				// With an association target, List names an object (#1288).
				if s.List != "" && s.Association == "" {
					inputs[s.List] = true
				}
			case *ast.RemoveFromListStmt:
				if s.List != "" && s.Association == "" {
					inputs[s.List] = true
				}
			case *ast.ClearListStmt:
				if s.List != "" {
					inputs[s.List] = true
				}
			case *ast.WhileStmt:
				walk(s.Body)
			case *ast.IfStmt:
				walk(s.ThenBody)
				walk(s.ElseBody)
			case *ast.CallMicroflowStmt:
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.CallJavaActionStmt:
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.CreateObjectStmt:
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.ChangeObjectStmt:
				// ChangeObjectStmt has no error-handler clause.
			case *ast.MfCommitStmt:
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.DeleteObjectStmt:
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.SendExternalObjectStmt:
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.DeleteExternalObjectStmt:
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.RestCallStmt:
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.SendEmailStmt:
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.SendRestRequestStmt:
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.ImportFromMappingStmt:
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.ExportToMappingStmt:
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			}
		}
	}
	walk(stmts)
	return inputs
}

func collectObjectInputVariables(stmts []ast.MicroflowStatement) map[string]bool {
	inputs := make(map[string]bool)
	var walkExpr func(ast.Expression)
	walkExpr = func(expr ast.Expression) {
		switch e := expr.(type) {
		case *ast.AttributePathExpr:
			if e.Variable != "" {
				inputs[e.Variable] = true
			}
		case *ast.BinaryExpr:
			walkExpr(e.Left)
			walkExpr(e.Right)
		case *ast.UnaryExpr:
			walkExpr(e.Operand)
		case *ast.FunctionCallExpr:
			for _, arg := range e.Arguments {
				walkExpr(arg)
			}
		case *ast.ParenExpr:
			walkExpr(e.Inner)
		case *ast.IfThenElseExpr:
			walkExpr(e.Condition)
			walkExpr(e.ThenExpr)
			walkExpr(e.ElseExpr)
		}
	}

	var walk func([]ast.MicroflowStatement)
	walk = func(body []ast.MicroflowStatement) {
		for _, stmt := range body {
			switch s := stmt.(type) {
			case *ast.MfSetStmt:
				walkExpr(s.Value)
			case *ast.ReturnStmt:
				walkExpr(s.Value)
			case *ast.LogStmt:
				walkExpr(s.Node)
				walkExpr(s.Message)
				for _, param := range s.Template {
					walkExpr(param.Value)
				}
			case *ast.CreateObjectStmt:
				for _, change := range s.Changes {
					walkExpr(change.Value)
				}
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.ChangeObjectStmt:
				if s.Variable != "" {
					inputs[s.Variable] = true
				}
				for _, change := range s.Changes {
					walkExpr(change.Value)
				}
			case *ast.RetrieveStmt:
				walkExpr(s.Where)
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.IfStmt:
				walkExpr(s.Condition)
				walk(s.ThenBody)
				walk(s.ElseBody)
			case *ast.LoopStmt:
				walk(s.Body)
			case *ast.WhileStmt:
				walkExpr(s.Condition)
				walk(s.Body)
			case *ast.ListOperationStmt:
				walkExpr(s.Condition)
			case *ast.AggregateListStmt:
				walkExpr(s.Expression)
			case *ast.AddToListStmt:
				if s.Association != "" {
					// `add $X to $Obj/Assoc` changes $Obj; $X may be an
					// object or a list, so it says nothing either way (#1288).
					inputs[s.List] = true
				} else if s.Value != nil {
					for _, ref := range exprVarRefs(s.Value) {
						inputs[ref] = true
					}
				} else if s.Item != "" {
					inputs[s.Item] = true
				}
			case *ast.RemoveFromListStmt:
				if s.Association != "" {
					inputs[s.List] = true
				}
			case *ast.CallMicroflowStmt:
				for _, arg := range s.Arguments {
					walkExpr(arg.Value)
				}
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.CallJavaActionStmt:
				for _, arg := range s.Arguments {
					walkExpr(arg.Value)
				}
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.ExecuteDatabaseQueryStmt:
				for _, arg := range s.Arguments {
					walkExpr(arg.Value)
				}
				for _, arg := range s.ConnectionArguments {
					walkExpr(arg.Value)
				}
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.CallExternalActionStmt:
				for _, arg := range s.Arguments {
					walkExpr(arg.Value)
				}
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.RestCallStmt:
				walkExpr(s.URL)
				for _, param := range s.URLParams {
					walkExpr(param.Value)
				}
				for _, header := range s.Headers {
					walkExpr(header.Value)
				}
				if s.Body != nil {
					walkExpr(s.Body.Template)
					for _, param := range s.Body.TemplateParams {
						walkExpr(param.Value)
					}
				}
				walkExpr(s.Timeout)
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.SendEmailStmt:
				for _, e := range s.Expressions() {
					walkExpr(e)
				}
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.SendRestRequestStmt:
				for _, param := range s.Parameters {
					for _, ref := range sourceAttributeVarRefs(param.Expression) {
						inputs[ref] = true
					}
				}
				if s.BodyVariable != "" {
					inputs[s.BodyVariable] = true
				}
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.ImportFromMappingStmt:
				if s.SourceVariable != "" {
					inputs[s.SourceVariable] = true
				}
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			case *ast.ExportToMappingStmt:
				if s.SourceVariable != "" {
					inputs[s.SourceVariable] = true
				}
				if s.ErrorHandling != nil {
					walk(s.ErrorHandling.Body)
				}
			}
		}
	}
	walk(stmts)
	return inputs
}

func sourceAttributeVarRefs(source string) []string {
	var refs []string
	for i := 0; i < len(source); i++ {
		if source[i] != '$' {
			continue
		}
		j := i + 1
		for j < len(source) {
			c := source[j]
			if c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
				j++
				continue
			}
			break
		}
		if j > i+1 && j < len(source) && source[j] == '/' {
			refs = append(refs, source[i+1:j])
		}
		i = j
	}
	return refs
}

func (fb *flowBuilder) terminatePendingErrorHandlersAtEnd(returns *ast.MicroflowReturnType) {
	fb.rewritePendingErrorHandlers(func(state pendingErrorHandlerState) pendingErrorHandlerState {
		if state.emptyFrom != "" {
			if returns != nil && returns.Type.Kind != ast.TypeVoid && fb.lastReturnEndID != "" {
				fb.flows = append(fb.flows, newErrorHandlerFlow(state.emptyFrom, fb.lastReturnEndID))
			} else {
				endID := fb.addTerminalEndEventForPendingHandler(returns, state.returnValue)
				fb.flows = append(fb.flows, newErrorHandlerFlow(state.emptyFrom, endID))
			}
			state.emptyFrom = ""
			state.returnValue = ""
		}
		if state.tailFrom != "" {
			if returns != nil && returns.Type.Kind != ast.TypeVoid && state.returnValue == "" && fb.lastReturnEndID != "" {
				if state.tailIsSource {
					fb.flows = append(fb.flows, newErrorHandlerFlow(state.tailFrom, fb.lastReturnEndID))
				} else {
					fb.flows = append(fb.flows, newHorizontalFlow(state.tailFrom, fb.lastReturnEndID))
				}
			} else {
				endID := fb.addTerminalEndEventForPendingHandler(returns, state.returnValue)
				if state.tailIsSource {
					fb.flows = append(fb.flows, newErrorHandlerFlow(state.tailFrom, endID))
				} else {
					fb.flows = append(fb.flows, newHorizontalFlow(state.tailFrom, endID))
				}
			}
			state.source = ""
			state.tailFrom = ""
			state.skipVar = ""
			state.tailIsSource = false
			state.returnValue = ""
		}
		return state
	})
}

func (fb *flowBuilder) addTerminalEndEventForPendingHandler(returns *ast.MicroflowReturnType, returnValue string) model.ID {
	if returnValue == "" && returns != nil && returns.Type.Kind != ast.TypeVoid {
		returnValue = fb.returnValue
	}
	end := &microflows.EndEvent{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: fb.posX + HorizontalSpacing/2, Y: fb.baseY + ErrorHandlerPitch},
			Size:        model.Size{Width: EventSize, Height: EventSize},
		},
		ReturnValue: returnValue,
	}
	fb.objects = append(fb.objects, end)
	return end.ID
}

// addStatement converts an AST statement to a microflow activity and returns its ID.
func (fb *flowBuilder) addStatement(stmt ast.MicroflowStatement) model.ID {
	// The loop-body tail is this statement's alone: only an IF reads it, and a
	// CASE or anything else must not leave it set for an IF nested inside,
	// which is not at the tail (atBodyTail).
	if _, isIf := stmt.(*ast.IfStmt); !isIf {
		fb.atBodyTail = false
	}

	// Extract annotations from the statement and merge into pendingAnnotations
	fb.mergeStatementAnnotations(stmt)

	// Apply @position before creating the activity so it's placed at the right position
	if fb.pendingAnnotations != nil && fb.pendingAnnotations.Position != nil {
		fb.posX = fb.pendingAnnotations.Position.X
		fb.posY = fb.pendingAnnotations.Position.Y
	} else {
		fb.clearLowerLane(stmt)
	}
	if fb.pendingAnnotations != nil {
		for _, note := range fb.pendingAnnotations.FreeNotes {
			fb.attachFreeAnnotation(note)
		}
		fb.pendingAnnotations.FreeNotes = nil
	}

	switch s := stmt.(type) {
	case *ast.DeclareStmt:
		return fb.addCreateVariableAction(s)
	case *ast.EnumSplitStmt:
		return fb.addEnumSplit(s)
	case *ast.InheritanceSplitStmt:
		return fb.addInheritanceSplit(s)
	case *ast.CastObjectStmt:
		return fb.addCastAction(s)
	case *ast.MfSetStmt:
		if idx := strings.IndexByte(s.Target, '/'); idx >= 0 {
			// Path target (e.g. "$Order/Sales.Order_Customer") → ChangeObjectAction
			varName := strings.TrimPrefix(s.Target[:idx], "$")
			assoc := s.Target[idx+1:]
			return fb.addChangeObjectAction(&ast.ChangeObjectStmt{
				Variable:    varName,
				Changes:     []ast.ChangeItem{{Attribute: assoc, Value: s.Value}},
				Annotations: s.Annotations,
			})
		}
		if fb.isListVariable(s.Target) {
			return fb.addReplaceListAction(s)
		}
		fb.refuseSetOnObject(s.Target)
		return fb.addChangeVariableAction(s)
	case *ast.ReturnStmt:
		return fb.addEndEventWithReturn(s)
	case *ast.RaiseErrorStmt:
		return fb.addErrorEvent()
	case *ast.BreakStmt:
		return fb.addBreakEvent()
	case *ast.ContinueStmt:
		return fb.addContinueEvent()
	case *ast.MergeStmt:
		return fb.addMergeStatement(s)
	case *ast.JoinStmt:
		return fb.addJoinStatement(s)
	case *ast.LogStmt:
		return fb.addLogMessageAction(s)
	case *ast.CreateObjectStmt:
		return fb.addCreateObjectAction(s)
	case *ast.ChangeObjectStmt:
		return fb.addChangeObjectAction(s)
	case *ast.RetrieveStmt:
		return fb.addRetrieveAction(s)
	case *ast.MfCommitStmt:
		return fb.addCommitAction(s)
	case *ast.DeleteObjectStmt:
		return fb.addDeleteAction(s)
	case *ast.SendExternalObjectStmt:
		return fb.addSendExternalObjectAction(s)
	case *ast.DeleteExternalObjectStmt:
		return fb.addDeleteExternalObjectAction(s)
	case *ast.RollbackStmt:
		return fb.addRollbackAction(s)
	case *ast.IfStmt:
		return fb.addIfStatement(s)
	case *ast.LoopStmt:
		return fb.addLoopStatement(s)
	case *ast.WhileStmt:
		return fb.addWhileStatement(s)
	case *ast.ListOperationStmt:
		return fb.addListOperationAction(s)
	case *ast.AggregateListStmt:
		return fb.addAggregateListAction(s)
	case *ast.CreateListStmt:
		return fb.addCreateListAction(s)
	case *ast.AddToListStmt:
		return fb.addAddToListAction(s)
	case *ast.RemoveFromListStmt:
		return fb.addRemoveFromListAction(s)
	case *ast.ClearListStmt:
		return fb.addClearListAction(s)
	case *ast.CallMicroflowStmt:
		return fb.addCallMicroflowAction(s)
	case *ast.CallNanoflowStmt:
		return fb.addCallNanoflowAction(s)
	case *ast.CallJavaActionStmt:
		return fb.addCallJavaActionAction(s)
	case *ast.CallJavaScriptActionStmt:
		return fb.addCallJavaScriptActionAction(s)
	case *ast.CallWebServiceStmt:
		return fb.addCallWebServiceAction(s)
	case *ast.ExecuteDatabaseQueryStmt:
		return fb.addExecuteDatabaseQueryAction(s)
	case *ast.CallExternalActionStmt:
		return fb.addCallExternalActionAction(s)
	case *ast.ShowPageStmt:
		return fb.addShowPageAction(s)
	case *ast.ClosePageStmt:
		return fb.addClosePageAction(s)
	case *ast.ShowHomePageStmt:
		return fb.addShowHomePageAction(s)
	case *ast.ShowMessageStmt:
		return fb.addShowMessageAction(s)
	case *ast.DownloadFileStmt:
		return fb.addDownloadFileAction(s)
	case *ast.SynchronizeStmt:
		return fb.addSynchronizeAction(s)
	case *ast.ValidationFeedbackStmt:
		return fb.addValidationFeedbackAction(s)
	case *ast.RestCallStmt:
		return fb.addRestCallAction(s)
	case *ast.SendEmailStmt:
		return fb.addSendEmailAction(s)
	case *ast.SendRestRequestStmt:
		return fb.addSendRestRequestAction(s)
	case *ast.ImportFromMappingStmt:
		return fb.addImportFromMappingAction(s)
	case *ast.ExportToMappingStmt:
		return fb.addExportToMappingAction(s)
	case *ast.TransformJsonStmt:
		return fb.addTransformJsonAction(s)
	// Workflow microflow actions
	case *ast.CallWorkflowStmt:
		return fb.addCallWorkflowAction(s)
	case *ast.GetWorkflowDataStmt:
		return fb.addGetWorkflowDataAction(s)
	case *ast.GetWorkflowsStmt:
		return fb.addGetWorkflowsAction(s)
	case *ast.GetWorkflowActivityRecordsStmt:
		return fb.addGetWorkflowActivityRecordsAction(s)
	case *ast.WorkflowOperationStmt:
		return fb.addWorkflowOperationAction(s)
	case *ast.SetTaskOutcomeStmt:
		return fb.addSetTaskOutcomeAction(s)
	case *ast.OpenUserTaskStmt:
		return fb.addOpenUserTaskAction(s)
	case *ast.NotifyWorkflowStmt:
		return fb.addNotifyWorkflowAction(s)
	case *ast.OpenWorkflowStmt:
		return fb.addOpenWorkflowAction(s)
	case *ast.LockWorkflowStmt:
		return fb.addLockWorkflowAction(s)
	case *ast.UnlockWorkflowStmt:
		return fb.addUnlockWorkflowAction(s)
	default:
		// For now, skip unknown statement types
		return ""
	}
}
