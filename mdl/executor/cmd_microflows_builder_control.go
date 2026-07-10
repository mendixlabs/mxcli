// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow graph: IF/ELSE and LOOP control flow builders
package executor

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// addIfStatement creates an IF/THEN/ELSE statement using ExclusiveSplit and ExclusiveMerge.
// Layout strategy:
// - IF with ELSE: TRUE path goes horizontal (happy path), FALSE path goes below
// - IF without ELSE: TRUE path goes below, FALSE path goes horizontal (happy path)
// When a branch ends with RETURN, it terminates at its own EndEvent and does not
// connect to the merge. When both branches end with RETURN, no merge is created.
func (fb *flowBuilder) addIfStatement(s *ast.IfStmt) model.ID {
	// First, measure the branches to know how much space they need
	thenBounds := fb.measurer.measureStatements(s.ThenBody)
	elseBounds := fb.measurer.measureStatements(s.ElseBody)

	// Calculate branch width (max of both branches)
	branchWidth := max(thenBounds.Width, elseBounds.Width)
	if branchWidth == 0 {
		branchWidth = HorizontalSpacing / 2
	}

	// Check if branches end with RETURN (creating their own EndEvents)
	thenReturns := lastStmtIsReturn(s.ThenBody)
	hasElseBody := s.HasElse || len(s.ElseBody) > 0
	elseReturns := hasElseBody && lastStmtIsReturn(s.ElseBody)
	bothReturn := hasElseBody && thenReturns && elseReturns
	thenNeedsErrorMerge := thenReturns && bodyHasContinuingCustomErrorHandler(s.ThenBody)
	elseNeedsErrorMerge := elseReturns && bodyHasContinuingCustomErrorHandler(s.ElseBody)

	// Save/restore endsWithReturn around branch processing to avoid
	// a branch's RETURN affecting the parent flow state prematurely
	savedEndsWithReturn := fb.endsWithReturn

	// Capture an incoming empty custom error handler whose source is the
	// previous call activity (set by finishCustomErrorHandler via
	// registerEmptyCustomErrorHandlerWithSkip) when the call has a
	// non-empty output variable (skipVar). The Studio Pro authored pattern
	// wires the error edge into the IF's ELSE branch — at the first
	// activity in the ELSE body — so the fallback path handles both "no
	// value" and "error" cases. Left un-rejoined,
	// terminatePendingErrorHandlersAtEnd would synthesise a phantom
	// EndEvent; naively rejoining at the split would bypass the output
	// variable declaration and trigger CE0108 "variable not in scope" on
	// subsequent activities.
	//
	// We consume the pending state here so the rejoin is emitted in the
	// `lastElseID == ""` block below (when the first ELSE activity is
	// created); the corresponding builder fields are cleared so downstream
	// logic does not see the handler twice.
	pendingElseErrorOrigin := model.ID("")
	if fb.errorHandlerSource != "" && fb.errorHandlerTailFrom == fb.errorHandlerSource &&
		fb.errorHandlerSkipVar != "" && fb.errorHandlerTailIsSource &&
		hasElseBody && len(s.ElseBody) > 0 &&
		bothReturn && // only when both branches terminate — a continuation in THEN would need a different rejoin target
		statementReferencesVar(s, fb.errorHandlerSkipVar) &&
		firstElseIsLeafActivity(s.ElseBody) {
		pendingElseErrorOrigin = fb.errorHandlerSource
		fb.errorHandlerSource = ""
		fb.errorHandlerTailFrom = ""
		fb.errorHandlerSkipVar = ""
		fb.errorHandlerTailIsSource = false
	}

	// Save current center line position
	splitX := fb.posX
	centerY := fb.posY // This is the center line for the happy path

	// Decide whether the IF condition is a rule call or a plain expression.
	// A rule-based split must be serialized as Microflows$RuleSplitCondition;
	// emitting ExpressionSplitCondition for a rule call causes Studio Pro to
	// raise CE0117 "Error(s) in expression".
	caption := fb.exprToString(s.Condition)
	splitCondition := fb.buildSplitCondition(s.Condition, caption)

	split := &microflows.ExclusiveSplit{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: splitX, Y: centerY},
			Size:        model.Size{Width: SplitWidth, Height: SplitHeight},
		},
		Caption:           caption,
		SplitCondition:    splitCondition,
		ErrorHandlingType: fb.ehType(nil),
	}
	fb.objects = append(fb.objects, split)
	splitID := split.ID

	// Apply this IF's own annotations (e.g. @caption, @annotation) to the split
	// BEFORE recursing into branch bodies. Otherwise a nested statement's annotations
	// would overwrite fb.pendingAnnotations (shared state) and the outer loop in
	// buildFlowGraph would then attach the wrong caption/annotation to this split.
	//
	// Capture per-branch anchor overrides before pendingAnnotations is cleared so
	// the TRUE/FALSE flows emitted below can pick them up.
	var trueBranchAnchor, falseBranchAnchor *ast.FlowAnchors
	if fb.pendingAnnotations != nil {
		trueBranchAnchor = fb.pendingAnnotations.TrueBranchAnchor
		falseBranchAnchor = fb.pendingAnnotations.FalseBranchAnchor
		fb.applyAnnotations(splitID, fb.pendingAnnotations)
		fb.pendingAnnotations = nil
	}

	// Calculate merge position (after the longest branch)
	mergeX := splitX + SplitWidth + HorizontalSpacing/2 + branchWidth + HorizontalSpacing/2

	// Determine if the merge would have 2+ incoming edges (non-redundant).
	// Skip merge when only one branch flows into it (the other returns).
	needMerge := false
	if !bothReturn {
		if hasElseBody {
			needMerge = (!thenReturns && !elseReturns) || thenNeedsErrorMerge || elseNeedsErrorMerge
		} else {
			needMerge = !thenReturns // THEN continues + FALSE path → 2 inputs
		}
	}

	var mergeID model.ID
	if needMerge {
		merge := &microflows.ExclusiveMerge{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: mergeX, Y: centerY},
				Size:        model.Size{Width: MergeSize, Height: MergeSize},
			},
		}
		fb.objects = append(fb.objects, merge)
		mergeID = merge.ID
	}

	thenStartX := splitX + SplitWidth + HorizontalSpacing/2
	var noMergeExitID model.ID
	var noMergeExitCase string
	var noMergeExitAnchor *ast.FlowAnchors

	if hasElseBody {
		// IF WITH ELSE: TRUE path horizontal (happy path), FALSE path below
		fb.posX = thenStartX
		fb.posY = centerY
		fb.endsWithReturn = false

		var lastThenID model.ID
		var prevThenAnchor *ast.FlowAnchors
		var pendingThenCase string
		var pendingThenAnchor *ast.FlowAnchors
		for i, stmt := range s.ThenBody {
			thisAnchor := stmtOwnAnchor(stmt)
			actID := fb.addStatement(stmt)
			if actID != "" {
				fb.applyPendingAnnotations(actID)
				if lastThenID == "" {
					// First statement in THEN - connect from split with "true" case.
					// Origin: trueBranchAnchor.From (if set) — anchor on the split side.
					// Destination: prefer the first statement's own @anchor(to: ...) if it
					// has one; otherwise fall back to trueBranchAnchor.To.
					flow := newHorizontalFlowWithCase(splitID, actID, "true")
					applyUserAnchors(flow, trueBranchAnchor, branchDestinationAnchor(trueBranchAnchor, thisAnchor))
					fb.flows = append(fb.flows, flow)
				} else {
					var flow *microflows.SequenceFlow
					originAnchor := prevThenAnchor
					destAnchor := thisAnchor
					if pendingThenCase != "" || pendingThenAnchor != nil {
						originAnchor, destAnchor = pendingFlowAnchors(prevThenAnchor, pendingThenAnchor, thisAnchor)
						flow = newHorizontalFlowWithCase(lastThenID, actID, pendingThenCase)
						if pendingThenCase == "" {
							flow = newHorizontalFlow(lastThenID, actID)
						}
						pendingThenCase = ""
						pendingThenAnchor = nil
					} else {
						flow = newHorizontalFlow(lastThenID, actID)
					}
					applyUserAnchors(flow, originAnchor, destAnchor)
					fb.flows = append(fb.flows, flow)
					if fb.emptyErrorHandlerFrom == lastThenID {
						fb.addPendingErrorHandlerFlowForStatement(lastThenID, actID, stmt, statementsReferenceVar(s.ThenBody[i+1:], fb.errorHandlerSkipVar))
					}
				}
				prevThenAnchor = thisAnchor
				// For nested compound statements, use their exit point
				if fb.nextConnectionPoint != "" {
					lastThenID = fb.nextConnectionPoint
					fb.nextConnectionPoint = ""
					pendingThenCase = fb.nextFlowCase
					fb.nextFlowCase = ""
					pendingThenAnchor = fb.nextFlowAnchor
					fb.nextFlowAnchor = nil
				} else {
					lastThenID = actID
				}
			}
		}

		// Connect THEN body to merge only if it doesn't end with RETURN and a merge exists.
		// When needMerge=false, the continuing branch is wired up by the parent via
		// nextConnectionPoint/nextFlowCase, so we must not emit a dangling flow here.
		if !thenReturns && needMerge {
			if lastThenID != "" {
				var flow *microflows.SequenceFlow
				originAnchor := prevThenAnchor
				destAnchor := (*ast.FlowAnchors)(nil)
				if pendingThenCase != "" || pendingThenAnchor != nil {
					originAnchor, destAnchor = pendingFlowAnchors(prevThenAnchor, pendingThenAnchor, nil)
					flow = newHorizontalFlowWithCase(lastThenID, mergeID, pendingThenCase)
					if pendingThenCase == "" {
						flow = newHorizontalFlow(lastThenID, mergeID)
					}
				} else {
					flow = newHorizontalFlow(lastThenID, mergeID)
				}
				applyUserAnchors(flow, originAnchor, destAnchor)
				fb.flows = append(fb.flows, flow)
			} else {
				// Empty THEN body - connect split directly to merge with true case.
				// Pass trueBranchAnchor as destination too so the @anchor(true: (..., to: Y))
				// from the describer round-trips into the merge side of the flow.
				flow := newHorizontalFlowWithCase(splitID, mergeID, "true")
				applyUserAnchors(flow, trueBranchAnchor, trueBranchAnchor)
				fb.flows = append(fb.flows, flow)
			}
		} else if thenReturns && needMerge {
			fb.addPendingErrorHandlerFlowTo(mergeID)
		}

		// Process ELSE body (below the THEN path)
		thenH := max(thenBounds.Height, ActivityHeight)
		elseH := max(elseBounds.Height, ActivityHeight)
		elseCenterY := centerY + thenH/2 + BranchGap + elseH/2
		fb.posX = thenStartX
		fb.posY = elseCenterY
		fb.endsWithReturn = false

		var lastElseID model.ID
		var prevElseAnchor *ast.FlowAnchors
		var pendingElseCase string
		var pendingElseAnchor *ast.FlowAnchors
		for i, stmt := range s.ElseBody {
			thisAnchor := stmtOwnAnchor(stmt)
			actID := fb.addStatement(stmt)
			if actID != "" {
				fb.applyPendingAnnotations(actID)
				if lastElseID == "" {
					// First statement in ELSE - connect from split going down (false path).
					// Same compositional rule as the THEN branch.
					flow := newDownwardFlowWithCase(splitID, actID, "false")
					applyUserAnchors(flow, falseBranchAnchor, branchDestinationAnchor(falseBranchAnchor, thisAnchor))
					fb.flows = append(fb.flows, flow)
					// Rejoin a pending empty custom error handler from the
					// previous call activity at the first ELSE activity.
					// Mendix authors the error edge directly into the ELSE
					// fallback path (same entry point as the split `false`
					// branch), preserving output-variable scope while giving
					// the error case the same handling as the happy-path
					// fallback.
					if pendingElseErrorOrigin != "" {
						fb.addEmptyErrorHandlerRejoinFlowFrom(splitID, pendingElseErrorOrigin, actID)
						pendingElseErrorOrigin = ""
					}
				} else {
					var flow *microflows.SequenceFlow
					originAnchor := prevElseAnchor
					destAnchor := thisAnchor
					if pendingElseCase != "" || pendingElseAnchor != nil {
						originAnchor, destAnchor = pendingFlowAnchors(prevElseAnchor, pendingElseAnchor, thisAnchor)
						flow = newHorizontalFlowWithCase(lastElseID, actID, pendingElseCase)
						if pendingElseCase == "" {
							flow = newHorizontalFlow(lastElseID, actID)
						}
						pendingElseCase = ""
						pendingElseAnchor = nil
					} else {
						flow = newHorizontalFlow(lastElseID, actID)
					}
					applyUserAnchors(flow, originAnchor, destAnchor)
					fb.flows = append(fb.flows, flow)
					if fb.emptyErrorHandlerFrom == lastElseID {
						fb.addPendingErrorHandlerFlowForStatement(lastElseID, actID, stmt, statementsReferenceVar(s.ElseBody[i+1:], fb.errorHandlerSkipVar))
					}
				}
				prevElseAnchor = thisAnchor
				// For nested compound statements, use their exit point
				if fb.nextConnectionPoint != "" {
					lastElseID = fb.nextConnectionPoint
					fb.nextConnectionPoint = ""
					pendingElseCase = fb.nextFlowCase
					fb.nextFlowCase = ""
					pendingElseAnchor = fb.nextFlowAnchor
					fb.nextFlowAnchor = nil
				} else {
					lastElseID = actID
				}
			}
		}

		// Connect ELSE body to merge only if it doesn't end with RETURN and a merge exists.
		// When needMerge=false, the continuing branch is handled by the parent; emitting
		// a flow with an empty mergeID would create an orphan SequenceFlow.
		if !elseReturns && needMerge {
			if lastElseID != "" {
				flow := newUpwardFlow(lastElseID, mergeID)
				originAnchor := prevElseAnchor
				destAnchor := (*ast.FlowAnchors)(nil)
				if pendingElseCase != "" || pendingElseAnchor != nil {
					originAnchor, destAnchor = pendingFlowAnchors(prevElseAnchor, pendingElseAnchor, nil)
					if pendingElseCase != "" {
						flow.CaseValue = microflows.EnumerationCase{
							BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
							Value:       pendingElseCase,
						}
					}
				}
				applyUserAnchors(flow, originAnchor, destAnchor)
				fb.flows = append(fb.flows, flow)
			} else {
				// Explicit empty ELSE body: the split still needs a configured
				// false case when both branches converge at the merge.
				flow := newDownwardFlowWithCase(splitID, mergeID, "false")
				applyUserAnchors(flow, falseBranchAnchor, falseBranchAnchor)
				fb.flows = append(fb.flows, flow)
			}
		} else if elseReturns && needMerge {
			fb.addPendingErrorHandlerFlowTo(mergeID)
		}
		if !needMerge {
			if thenReturns && !elseReturns {
				if lastElseID != "" {
					noMergeExitID = lastElseID
					noMergeExitCase = pendingElseCase
					if pendingElseAnchor != nil {
						noMergeExitAnchor = pendingElseAnchor
					} else {
						noMergeExitAnchor = prevElseAnchor
					}
				} else {
					noMergeExitID = splitID
					noMergeExitCase = "false"
					noMergeExitAnchor = falseBranchAnchor
				}
			} else if elseReturns && !thenReturns {
				if lastThenID != "" {
					noMergeExitID = lastThenID
					noMergeExitCase = pendingThenCase
					if pendingThenAnchor != nil {
						noMergeExitAnchor = pendingThenAnchor
					} else {
						noMergeExitAnchor = prevThenAnchor
					}
				} else {
					noMergeExitID = splitID
					noMergeExitCase = "true"
					noMergeExitAnchor = trueBranchAnchor
				}
			}
		}
	} else {
		// IF WITHOUT ELSE: FALSE path horizontal (happy path), TRUE path below
		// This keeps the "do nothing" path straight and the "do something" path below

		if needMerge {
			// FALSE path: connect split directly to merge horizontally.
			// Pass falseBranchAnchor as destination too so @anchor(false: (..., to: Y))
			// round-trips through the merge side of the split-to-merge flow.
			flow := newHorizontalFlowWithCase(splitID, mergeID, "false")
			applyUserAnchors(flow, falseBranchAnchor, falseBranchAnchor)
			fb.flows = append(fb.flows, flow)
		}
		// When !needMerge (thenReturns): FALSE flow is deferred — the parent will
		// connect splitID to the next activity with nextFlowCase="false".

		// TRUE path: goes below the main line
		thenH := max(thenBounds.Height, ActivityHeight)
		thenCenterY := centerY + ActivityHeight/2 + BranchGap + thenH/2
		fb.posX = thenStartX
		fb.posY = thenCenterY
		fb.endsWithReturn = false

		var lastThenID model.ID
		var prevThenAnchor *ast.FlowAnchors
		var pendingThenCase string
		var pendingThenAnchor *ast.FlowAnchors
		for _, stmt := range s.ThenBody {
			thisAnchor := stmtOwnAnchor(stmt)
			actID := fb.addStatement(stmt)
			if actID != "" {
				fb.applyPendingAnnotations(actID)
				if lastThenID == "" {
					// First statement in THEN - connect from split going down with "true" case
					flow := newDownwardFlowWithCase(splitID, actID, "true")
					applyUserAnchors(flow, trueBranchAnchor, branchDestinationAnchor(trueBranchAnchor, thisAnchor))
					fb.flows = append(fb.flows, flow)
				} else {
					var flow *microflows.SequenceFlow
					originAnchor := prevThenAnchor
					destAnchor := thisAnchor
					if pendingThenCase != "" || pendingThenAnchor != nil {
						originAnchor, destAnchor = pendingFlowAnchors(prevThenAnchor, pendingThenAnchor, thisAnchor)
						flow = newHorizontalFlowWithCase(lastThenID, actID, pendingThenCase)
						if pendingThenCase == "" {
							flow = newHorizontalFlow(lastThenID, actID)
						}
						pendingThenCase = ""
						pendingThenAnchor = nil
					} else {
						flow = newHorizontalFlow(lastThenID, actID)
					}
					applyUserAnchors(flow, originAnchor, destAnchor)
					fb.flows = append(fb.flows, flow)
				}
				prevThenAnchor = thisAnchor
				// For nested compound statements, use their exit point
				if fb.nextConnectionPoint != "" {
					lastThenID = fb.nextConnectionPoint
					fb.nextConnectionPoint = ""
					pendingThenCase = fb.nextFlowCase
					fb.nextFlowCase = ""
					pendingThenAnchor = fb.nextFlowAnchor
					fb.nextFlowAnchor = nil
				} else {
					lastThenID = actID
				}
			}
		}

		// Connect THEN body to merge only if it doesn't end with RETURN and a merge exists.
		// With no ELSE + thenReturns, needMerge=false and the FALSE path is threaded through
		// the parent — any flow emitted here would dangle with mergeID="".
		if !thenReturns && needMerge {
			if lastThenID != "" {
				flow := newUpwardFlow(lastThenID, mergeID)
				originAnchor := prevThenAnchor
				destAnchor := (*ast.FlowAnchors)(nil)
				if pendingThenCase != "" || pendingThenAnchor != nil {
					originAnchor, destAnchor = pendingFlowAnchors(prevThenAnchor, pendingThenAnchor, nil)
					if pendingThenCase != "" {
						flow.CaseValue = microflows.EnumerationCase{
							BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
							Value:       pendingThenCase,
						}
					}
				}
				applyUserAnchors(flow, originAnchor, destAnchor)
				fb.flows = append(fb.flows, flow)
			} else {
				// Empty THEN body - connect split directly to merge going down and back up.
				// Pass trueBranchAnchor as destination too so @anchor(true: (..., to: Y))
				// round-trips into the merge side of the flow.
				flow := newDownwardFlowWithCase(splitID, mergeID, "true")
				applyUserAnchors(flow, trueBranchAnchor, trueBranchAnchor)
				fb.flows = append(fb.flows, flow)
			}
		}
		if !needMerge {
			noMergeExitID = splitID
			noMergeExitCase = "false"
			noMergeExitAnchor = falseBranchAnchor
		}
	}

	// If both branches end with RETURN, the flow terminates here
	if bothReturn {
		fb.endsWithReturn = true
		return splitID
	}

	// Restore endsWithReturn - a single branch returning doesn't end the overall flow
	fb.endsWithReturn = savedEndsWithReturn

	if needMerge {
		// Update position to after the merge, on the happy path center line
		fb.posX = mergeX + MergeSize + HorizontalSpacing/2
		fb.posY = centerY
		fb.nextConnectionPoint = mergeID
	} else {
		// No merge: the split's continuing branch connects directly to the next activity.
		// Position after the split, past the downward branch's horizontal extent.
		afterSplit := splitX + SplitWidth + HorizontalSpacing
		afterBranch := thenStartX + thenBounds.Width + HorizontalSpacing/2
		if !hasElseBody {
			fb.posX = max(afterSplit, afterBranch)
		} else {
			fb.posX = max(afterSplit, afterBranch)
		}
		fb.posY = centerY
		if noMergeExitID != "" {
			fb.nextConnectionPoint = noMergeExitID
			fb.nextFlowCase = noMergeExitCase
			fb.nextFlowAnchor = noMergeExitAnchor
		} else {
			// Defensive fallback: the no-merge path above always records the
			// continuing branch tail in noMergeExitID. If future branch handling
			// changes violate that invariant, continue from the split rather than
			// leaving the parent disconnected.
			fb.nextConnectionPoint = splitID
		}
	}

	return splitID
}

// firstElseIsLeafActivity reports whether the ELSE body's first statement is
// a plain activity (not a compound split/if/loop). Rejoining an error flow
// onto a compound statement's internal split/merge would need extra plumbing
// that the caller is not equipped for; fall back to the default phantom-
// EndEvent terminator in that case.
func firstElseIsLeafActivity(stmts []ast.MicroflowStatement) bool {
	if len(stmts) == 0 {
		return false
	}
	switch stmts[0].(type) {
	case *ast.IfStmt, *ast.LoopStmt, *ast.WhileStmt, *ast.EnumSplitStmt:
		return false
	}
	return true
}

func bodyHasContinuingCustomErrorHandler(stmts []ast.MicroflowStatement) bool {
	for _, stmt := range stmts {
		if eh := statementErrorHandling(stmt); eh != nil {
			if isContinuingCustomErrorHandler(eh) || bodyHasContinuingCustomErrorHandler(eh.Body) {
				return true
			}
		}
		switch s := stmt.(type) {
		case *ast.IfStmt:
			if bodyHasContinuingCustomErrorHandler(s.ThenBody) || bodyHasContinuingCustomErrorHandler(s.ElseBody) {
				return true
			}
		case *ast.LoopStmt:
			if bodyHasContinuingCustomErrorHandler(s.Body) {
				return true
			}
		case *ast.WhileStmt:
			if bodyHasContinuingCustomErrorHandler(s.Body) {
				return true
			}
		}
	}
	return false
}

func isContinuingCustomErrorHandler(eh *ast.ErrorHandlingClause) bool {
	if eh == nil {
		return false
	}
	if eh.Type != ast.ErrorHandlingCustom && eh.Type != ast.ErrorHandlingCustomWithoutRollback {
		return false
	}
	return len(eh.Body) == 0 || !lastStmtIsReturn(eh.Body)
}

// addLoopStatement creates a LOOP statement using LoopedActivity.
// Layout: Auto-sizes the loop box to fit content with padding
func (fb *flowBuilder) addLoopStatement(s *ast.LoopStmt) model.ID {
	// Snapshot & clear this loop's own annotations so the body's recursive
	// addStatement calls can't consume them. We re-apply to the loop activity
	// after it's created below.
	savedLoopAnnotations := fb.pendingAnnotations
	fb.pendingAnnotations = nil

	// First, measure the loop body to determine size
	bodyBounds := fb.measurer.measureStatements(s.Body)

	// Calculate loop box size with padding
	// Extra width for iterator icon and its label (100 pixels)
	iteratorSpace := 100
	loopWidth := max(bodyBounds.Width+2*LoopPadding+iteratorSpace, MinLoopWidth)
	loopHeight := max(bodyBounds.Height+2*LoopPadding, MinLoopHeight)

	// Inner positioning: activities start after the iterator icon on the left,
	// and are centred vertically within the loop box so that branching content
	// (IF/CASE) centred on innerStartY stays within [0, loopHeight].
	innerStartX := LoopPadding + iteratorSpace
	innerStartY := loopHeight / 2

	loopLeftX := fb.posX
	loopCenterX := loopLeftX + loopWidth/2
	if s.Annotations != nil && s.Annotations.Position != nil {
		loopCenterX = s.Annotations.Position.X
		loopLeftX = loopCenterX - loopWidth/2
	}

	// Add loop variable to varTypes with element type derived from list type
	// If $ProductList is "List of MfTest.Product", then $Product is "MfTest.Product"
	if fb.varTypes != nil {
		listType := fb.varTypes[s.ListVariable]
		if after, ok := strings.CutPrefix(listType, "List of "); ok {
			elementType := after
			fb.varTypes[s.LoopVariable] = elementType
		}
	}

	// Build nested ObjectCollection for loop body
	loopBuilder := &flowBuilder{
		posX:         innerStartX,
		posY:         innerStartY,
		baseY:        innerStartY,
		spacing:      HorizontalSpacing,
		varTypes:     fb.varTypes,     // Share variable scope
		declaredVars: fb.declaredVars, // Share declared vars (fixes nil map panic)
		measurer:     fb.measurer,     // Share measurer
		backend:      fb.backend,      // Share backend
		hierarchy:    fb.hierarchy,    // Share hierarchy
		restServices: fb.restServices, // Share REST services for parameter classification
		isNanoflow:   fb.isNanoflow,
	}

	// Process loop body statements and connect them with flows.
	var lastBodyID model.ID
	for _, stmt := range s.Body {
		actID := loopBuilder.addStatement(stmt)
		if actID != "" {
			loopBuilder.applyPendingAnnotations(actID)
			if lastBodyID != "" {
				loopBuilder.flows = append(loopBuilder.flows, newHorizontalFlow(lastBodyID, actID))
			}
			// Handle nextConnectionPoint for compound statements (nested IF, etc.)
			if loopBuilder.nextConnectionPoint != "" {
				lastBodyID = loopBuilder.nextConnectionPoint
				loopBuilder.nextConnectionPoint = ""
			} else {
				lastBodyID = actID
			}
		}
	}

	// Create LoopedActivity with calculated size
	// Position is the CENTER point (RelativeMiddlePoint in Mendix)
	loop := &microflows.LoopedActivity{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: loopCenterX, Y: fb.posY},
			Size:        model.Size{Width: loopWidth, Height: loopHeight},
		},
		LoopSource: &microflows.IterableList{
			BaseElement:      model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariableName: s.ListVariable,
			VariableName:     s.LoopVariable,
		},
		ObjectCollection: &microflows.MicroflowObjectCollection{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Objects:     loopBuilder.objects,
			Flows:       nil, // Internal flows go at top-level, not inside the loop's ObjectCollection
		},
		ErrorHandlingType: fb.ehType(nil),
	}

	// @anchor(iterator: ..., tail: ...) parses and survives on
	// savedLoopAnnotations for forward compatibility, but we deliberately do
	// not serialise either edge as a SequenceFlow: Studio Pro rejects loop→body
	// and body→loop with CE0709 "Sequence flow is not accepted by origin or
	// destination", since the iterator icon is drawn implicitly by the loop
	// geometry.

	fb.objects = append(fb.objects, loop)

	// Add the internal flows to the parent's flows (top-level), not inside loop
	// This is how Mendix stores them - all flows at the microflow level
	fb.flows = append(fb.flows, loopBuilder.flows...)
	fb.annotationFlows = append(fb.annotationFlows, loopBuilder.annotationFlows...)

	// Re-apply this loop's own annotations now that its activity exists.
	if savedLoopAnnotations != nil {
		fb.applyAnnotations(loop.ID, savedLoopAnnotations)
	}

	fb.posX = loopLeftX + loopWidth + HorizontalSpacing

	return loop.ID
}

func isManualWhileTrueCandidate(s *ast.WhileStmt) bool {
	if s == nil || containsBreakForCurrentLoop(s.Body) || (!containsContinueForCurrentLoop(s.Body) && !containsTerminalStmt(s.Body)) {
		return false
	}
	lit, ok := s.Condition.(*ast.LiteralExpr)
	if !ok || lit.Kind != ast.LiteralBoolean {
		return false
	}
	value, ok := lit.Value.(bool)
	return ok && value
}

func containsBreakForCurrentLoop(stmts []ast.MicroflowStatement) bool {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.BreakStmt:
			return true
		case *ast.IfStmt:
			if containsBreakForCurrentLoop(s.ThenBody) || containsBreakForCurrentLoop(s.ElseBody) {
				return true
			}
		case *ast.LoopStmt, *ast.WhileStmt:
			// A break inside a nested loop exits that nested loop, not this
			// manual while-true back-edge.
			continue
		}
	}
	return false
}

func statementErrorHandling(stmt ast.MicroflowStatement) *ast.ErrorHandlingClause {
	switch s := stmt.(type) {
	case *ast.RetrieveStmt:
		return s.ErrorHandling
	case *ast.CreateObjectStmt:
		return s.ErrorHandling
	case *ast.MfCommitStmt:
		return s.ErrorHandling
	case *ast.DeleteObjectStmt:
		return s.ErrorHandling
	case *ast.CallMicroflowStmt:
		return s.ErrorHandling
	case *ast.CallNanoflowStmt:
		return s.ErrorHandling
	case *ast.CallJavaActionStmt:
		return s.ErrorHandling
	case *ast.CallJavaScriptActionStmt:
		return s.ErrorHandling
	case *ast.CallWebServiceStmt:
		return s.ErrorHandling
	case *ast.ExecuteDatabaseQueryStmt:
		return s.ErrorHandling
	case *ast.CallExternalActionStmt:
		return s.ErrorHandling
	case *ast.DownloadFileStmt:
		return s.ErrorHandling
	case *ast.RestCallStmt:
		return s.ErrorHandling
	case *ast.SendRestRequestStmt:
		return s.ErrorHandling
	case *ast.ImportFromMappingStmt:
		return s.ErrorHandling
	case *ast.ExportToMappingStmt:
		return s.ErrorHandling
	case *ast.TransformJsonStmt:
		return s.ErrorHandling
	default:
		return nil
	}
}

// containsContinueForCurrentLoop reports whether stmts contain a continue
// that targets the enclosing loop — i.e. one that is NOT inside a nested
// LoopStmt/WhileStmt. Nested loops trap their own continues just like they
// trap their own breaks, so the scan stops at nested-loop boundaries.
// This mirrors containsBreakForCurrentLoop in intent; it differs only in
// which statement type it looks for.
func containsContinueForCurrentLoop(stmts []ast.MicroflowStatement) bool {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ContinueStmt:
			return true
		case *ast.IfStmt:
			if containsContinueForCurrentLoop(s.ThenBody) || containsContinueForCurrentLoop(s.ElseBody) {
				return true
			}
		case *ast.LoopStmt, *ast.WhileStmt:
			continue
		}
	}
	return false
}

func (fb *flowBuilder) addManualWhileTrueStatement(s *ast.WhileStmt) model.ID {
	mergeX := fb.posX
	mergeY := fb.posY
	if s.Annotations != nil && s.Annotations.Position != nil {
		mergeX = s.Annotations.Position.X
		mergeY = s.Annotations.Position.Y
	}

	merge := &microflows.ExclusiveMerge{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: mergeX, Y: mergeY},
			Size:        model.Size{Width: MergeSize, Height: MergeSize},
		},
	}
	fb.objects = append(fb.objects, merge)
	if fb.pendingAnnotations != nil {
		fb.applyAnnotations(merge.ID, fb.pendingAnnotations)
		fb.pendingAnnotations = nil
	}

	savedBackTarget := fb.manualLoopBackTarget
	fb.manualLoopBackTarget = merge.ID
	defer func() { fb.manualLoopBackTarget = savedBackTarget }()

	fb.posX = mergeX + MergeSize + HorizontalSpacing/2
	fb.posY = mergeY

	lastBodyID := merge.ID
	var pendingBodyCase string
	var pendingBodyAnchor *ast.FlowAnchors
	var prevBodyAnchor *ast.FlowAnchors
	for _, stmt := range s.Body {
		thisAnchor := stmtOwnAnchor(stmt)
		actID := fb.addStatement(stmt)
		if actID == "" {
			continue
		}
		if fb.pendingAnnotations != nil {
			fb.applyAnnotations(actID, fb.pendingAnnotations)
			fb.pendingAnnotations = nil
		}
		if lastBodyID != "" && lastBodyID != actID {
			var flow *microflows.SequenceFlow
			if pendingBodyCase != "" {
				flow = newHorizontalFlowWithCase(lastBodyID, actID, pendingBodyCase)
				if pendingBodyAnchor != nil {
					prevBodyAnchor = pendingBodyAnchor
				}
				pendingBodyCase = ""
				pendingBodyAnchor = nil
			} else {
				flow = newHorizontalFlow(lastBodyID, actID)
			}
			applyUserAnchors(flow, prevBodyAnchor, thisAnchor)
			fb.flows = append(fb.flows, flow)
		}
		prevBodyAnchor = thisAnchor
		if fb.nextConnectionPoint != "" {
			lastBodyID = fb.nextConnectionPoint
			fb.nextConnectionPoint = ""
			pendingBodyCase = fb.nextFlowCase
			fb.nextFlowCase = ""
			pendingBodyAnchor = fb.nextFlowAnchor
			fb.nextFlowAnchor = nil
		} else {
			lastBodyID = actID
		}
	}

	if lastBodyID != "" && lastBodyID != merge.ID && !lastStmtIsReturn(s.Body) {
		var flow *microflows.SequenceFlow
		if pendingBodyCase != "" {
			flow = newHorizontalFlowWithCase(lastBodyID, merge.ID, pendingBodyCase)
		} else {
			flow = newHorizontalFlow(lastBodyID, merge.ID)
		}
		applyUserAnchors(flow, pendingBodyAnchor, pendingBodyAnchor)
		fb.flows = append(fb.flows, flow)
	}
	fb.endsWithReturn = true

	return merge.ID
}

// addWhileStatement creates a WHILE loop using LoopedActivity with WhileLoopCondition.
// Layout matches addLoopStatement but without iterator icon space.
func (fb *flowBuilder) addWhileStatement(s *ast.WhileStmt) model.ID {
	if isManualWhileTrueCandidate(s) {
		return fb.addManualWhileTrueStatement(s)
	}

	// Snapshot & clear this WHILE's own annotations so the body's recursive
	// addStatement calls can't consume them (see addLoopStatement).
	savedWhileAnnotations := fb.pendingAnnotations
	fb.pendingAnnotations = nil

	bodyBounds := fb.measurer.measureStatements(s.Body)

	loopWidth := max(bodyBounds.Width+2*LoopPadding, MinLoopWidth)
	loopHeight := max(bodyBounds.Height+2*LoopPadding, MinLoopHeight)

	innerStartX := LoopPadding
	innerStartY := LoopPadding + ActivityHeight/2

	loopLeftX := fb.posX
	loopCenterX := loopLeftX + loopWidth/2
	if s.Annotations != nil && s.Annotations.Position != nil {
		loopCenterX = s.Annotations.Position.X
		loopLeftX = loopCenterX - loopWidth/2
	}

	loopBuilder := &flowBuilder{
		posX:         innerStartX,
		posY:         innerStartY,
		baseY:        innerStartY,
		spacing:      HorizontalSpacing,
		varTypes:     fb.varTypes,
		declaredVars: fb.declaredVars,
		measurer:     fb.measurer,
		backend:      fb.backend,
		hierarchy:    fb.hierarchy,
		restServices: fb.restServices,
		isNanoflow:   fb.isNanoflow,
	}

	var lastBodyID model.ID
	for _, stmt := range s.Body {
		actID := loopBuilder.addStatement(stmt)
		if actID != "" {
			loopBuilder.applyPendingAnnotations(actID)
			if lastBodyID != "" {
				loopBuilder.flows = append(loopBuilder.flows, newHorizontalFlow(lastBodyID, actID))
			}
			if loopBuilder.nextConnectionPoint != "" {
				lastBodyID = loopBuilder.nextConnectionPoint
				loopBuilder.nextConnectionPoint = ""
			} else {
				lastBodyID = actID
			}
		}
	}

	whileExpr := fb.exprToString(s.Condition)

	loop := &microflows.LoopedActivity{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: loopCenterX, Y: fb.posY},
			Size:        model.Size{Width: loopWidth, Height: loopHeight},
		},
		LoopSource: &microflows.WhileLoopCondition{
			BaseElement:     model.BaseElement{ID: model.ID(types.GenerateID())},
			WhileExpression: whileExpr,
		},
		ObjectCollection: &microflows.MicroflowObjectCollection{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Objects:     loopBuilder.objects,
			Flows:       nil,
		},
		ErrorHandlingType: fb.ehType(nil),
	}

	// See addLoopStatement — @anchor(iterator/tail) is parsed but not
	// serialised, since Studio Pro does not permit explicit edges between a
	// LoopedActivity and its body statements.

	fb.objects = append(fb.objects, loop)
	fb.flows = append(fb.flows, loopBuilder.flows...)
	fb.annotationFlows = append(fb.annotationFlows, loopBuilder.annotationFlows...)

	if savedWhileAnnotations != nil {
		fb.applyAnnotations(loop.ID, savedWhileAnnotations)
	}

	fb.posX = loopLeftX + loopWidth + HorizontalSpacing

	return loop.ID
}

func (fb *flowBuilder) addBreakEvent() model.ID {
	event := &microflows.BreakEvent{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: fb.posX, Y: fb.posY},
			Size:        model.Size{Width: EventSize, Height: EventSize},
		},
	}
	fb.objects = append(fb.objects, event)
	fb.posX += fb.spacing
	return event.ID
}

func (fb *flowBuilder) addContinueEvent() model.ID {
	if fb.manualLoopBackTarget != "" {
		return fb.manualLoopBackTarget
	}

	event := &microflows.ContinueEvent{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: fb.posX, Y: fb.posY},
			Size:        model.Size{Width: EventSize, Height: EventSize},
		},
	}
	fb.objects = append(fb.objects, event)
	fb.posX += fb.spacing
	return event.ID
}
