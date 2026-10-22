// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow graph: IF/ELSE and LOOP control flow builders
package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// addIfStatement creates an IF/THEN/ELSE statement using ExclusiveSplit and ExclusiveMerge.
// Layout strategy:
// - IF with ELSE: TRUE path goes horizontal (happy path), FALSE path goes below
// - IF without ELSE: TRUE path goes below, FALSE path goes horizontal (happy path)
// When a branch ends with RETURN, it terminates at its own EndEvent and does not
// connect to the merge. When both branches end with RETURN, no merge is created.
func (fb *flowBuilder) addIfStatement(s *ast.IfStmt) model.ID {
	// First, measure the branches to know how much space they need
	thenBounds := fb.measurer.measureBranch(s.ThenBody)
	elseBounds := fb.measurer.measureBranch(s.ElseBody)

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

	// The last statement of a loop body: an IF/ELSE whose branches both carry
	// on there joins nothing, since both end the iteration, and a loop-body
	// activity with no outgoing flow is how Mendix ends one. Its merge had no
	// way out, Studio Pro authors delete it by hand, and every re-run drew it
	// again (mendixlabs/mxcli#884). Both branches must hold something — a split
	// needs a flow per case — and an explicit @merge keeps the merge, which is
	// how DESCRIBE carries a stored one through a rebuild.
	atTail := fb.atBodyTail
	fb.atBodyTail = false
	tailWithoutMerge := atTail && hasElseBody && len(s.ThenBody) > 0 && len(s.ElseBody) > 0 &&
		!thenReturns && !elseReturns &&
		!bodyHasContinuingCustomErrorHandler(s.ThenBody) && !bodyHasContinuingCustomErrorHandler(s.ElseBody) &&
		(s.Annotations == nil || s.Annotations.Merge == nil)

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
		// Records the @curve too, which applyAnnotations does not (#1414).
		fb.applyPendingAnnotations(splitID)
	}

	// Calculate merge position (after the longest branch)
	// thenStartX (below) is the CENTRE of the branch's first activity and branchWidth
	// is measured edge to edge, so the branch ends at thenStartX - ActivityWidth/2 +
	// branchWidth. The merge goes one ordinary gap past that; a further half pitch used
	// to be added here, which left 120px before every merge against 40px elsewhere.
	mergeX := splitX + SplitWidth + HorizontalSpacing/2 + branchWidth

	// Determine if the merge would have 2+ incoming edges (non-redundant).
	// Skip merge when only one branch flows into it (the other returns).
	needMerge := false
	if !bothReturn && !tailWithoutMerge {
		if hasElseBody {
			// A fall-through handler in a returning branch rejoins inside that
			// branch, after its failing activity, so it needs no merge here.
			needMerge = !thenReturns && !elseReturns
		} else {
			needMerge = !thenReturns // THEN continues + FALSE path → 2 inputs
		}
	}

	var mergeID model.ID
	if needMerge {
		mergeX, mergeY := mergePosition(s.Annotations, mergeX, centerY)
		merge := &microflows.ExclusiveMerge{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: mergeX, Y: mergeY},
				Size:        model.Size{Width: MergeSize, Height: MergeSize},
			},
		}
		fb.objects = append(fb.objects, merge)
		mergeID = merge.ID
	}

	thenStartX := splitX + SplitWidth + HorizontalSpacing/2
	var noMergeExitID model.ID
	var branchHandlers []pendingErrorHandlerState
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
		outerHandlers := fb.enterBranchErrorHandlers()
		for i, stmt := range s.ThenBody {
			thisAnchor := stmtOwnAnchor(stmt)
			fb.atBodyTail = tailWithoutMerge && i == len(s.ThenBody)-1
			actID := fb.addStatement(stmt)
			fb.atBodyTail = false
			if fb.takeBranchJoin(lastThenID, splitID, "true", pendingThenCase, prevThenAnchor, trueBranchAnchor) {
				pendingThenCase, pendingThenAnchor = "", nil
				continue
			}
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
					fb.addBranchErrorHandlerFlowForStatement(thenReturns, lastThenID, actID, stmt, statementsReferenceVar(s.ThenBody[i+1:], fb.errorHandlerSkipVar))
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
				fb.addPendingErrorHandlerFlowForStatement(lastThenID, mergeID, nil)
			} else {
				// Empty THEN body - connect split directly to merge with true case.
				// Pass trueBranchAnchor as destination too so the @anchor(true: (..., to: Y))
				// from the describer round-trips into the merge side of the flow.
				flow := newHorizontalFlowWithCase(splitID, mergeID, "true")
				applyUserAnchors(flow, trueBranchAnchor, trueBranchAnchor)
				fb.flows = append(fb.flows, flow)
			}
		}
		branchHandlers = fb.takeBranchErrorHandlers(outerHandlers)

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
		outerHandlers = fb.enterBranchErrorHandlers()
		for i, stmt := range s.ElseBody {
			thisAnchor := stmtOwnAnchor(stmt)
			fb.atBodyTail = tailWithoutMerge && i == len(s.ElseBody)-1
			actID := fb.addStatement(stmt)
			fb.atBodyTail = false
			if fb.takeBranchJoin(lastElseID, splitID, "false", pendingElseCase, prevElseAnchor, falseBranchAnchor) {
				pendingElseCase, pendingElseAnchor = "", nil
				continue
			}
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
					fb.addBranchErrorHandlerFlowForStatement(elseReturns, lastElseID, actID, stmt, statementsReferenceVar(s.ElseBody[i+1:], fb.errorHandlerSkipVar))
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
				fb.addPendingErrorHandlerFlowForStatement(lastElseID, mergeID, nil)
			} else {
				// Explicit empty ELSE body: the split still needs a configured
				// false case when both branches converge at the merge.
				flow := newDownwardFlowWithCase(splitID, mergeID, "false")
				applyUserAnchors(flow, falseBranchAnchor, falseBranchAnchor)
				fb.flows = append(fb.flows, flow)
			}
		}
		branchHandlers = append(branchHandlers, fb.takeBranchErrorHandlers(outerHandlers)...)
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
		outerHandlers := fb.enterBranchErrorHandlers()
		for i, stmt := range s.ThenBody {
			thisAnchor := stmtOwnAnchor(stmt)
			actID := fb.addStatement(stmt)
			if fb.takeBranchJoin(lastThenID, splitID, "true", pendingThenCase, prevThenAnchor, trueBranchAnchor) {
				pendingThenCase, pendingThenAnchor = "", nil
				continue
			}
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
					fb.addBranchErrorHandlerFlowForStatement(thenReturns, lastThenID, actID, stmt, statementsReferenceVar(s.ThenBody[i+1:], fb.errorHandlerSkipVar))
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
				fb.addPendingErrorHandlerFlowForStatement(lastThenID, mergeID, nil)
			} else {
				// Empty THEN body - connect split directly to merge going down and back up.
				// Pass trueBranchAnchor as destination too so @anchor(true: (..., to: Y))
				// round-trips into the merge side of the flow.
				flow := newDownwardFlowWithCase(splitID, mergeID, "true")
				applyUserAnchors(flow, trueBranchAnchor, trueBranchAnchor)
				fb.flows = append(fb.flows, flow)
			}
		}
		branchHandlers = fb.takeBranchErrorHandlers(outerHandlers)
		if !needMerge {
			noMergeExitID = splitID
			noMergeExitCase = "false"
			noMergeExitAnchor = falseBranchAnchor
		}
	}

	// A handler a branch could not settle — one with a result, in a branch that
	// returns — rejoins on the flow leaving the IF. When both branches return
	// nothing leaves it, and the split never starts another flow, so the
	// handler ends at an end event as before.
	exitID := noMergeExitID
	if needMerge {
		exitID = mergeID
	} else if exitID == "" {
		exitID = splitID
	}
	fb.queueBranchErrorHandlersAt(branchHandlers, exitID)

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
		// `@anchor(from: X)` on an if is the flow leaving it, which is the flow
		// out of its merge (#767). Only the origin side: the if's `to:` is its
		// incoming flow and must not become the next statement's.
		fb.nextFlowAnchor = mergeExitFlowAnchor(s)
	} else {
		// No merge: the split's continuing branch connects directly to the next activity.
		// Position after the split, past the downward branch's horizontal extent.
		afterSplit := splitX + HorizontalSpacing
		afterBranch := thenStartX + thenBounds.Width + HorizontalSpacing/2
		if !hasElseBody {
			// A guard: the branch is in the lane below and ends there, so the main
			// line resumes right after the split — in the column the branch starts
			// in, so the two line up — and the lane is marked taken.
			fb.posX = thenStartX
			fb.reserveLowerLane(centerY, thenStartX-ActivityWidth/2+thenBounds.Width+laneGap)
		} else {
			fb.posX = max(afterSplit, afterBranch)
			fb.reserveLowerLane(centerY, thenStartX-ActivityWidth/2+branchWidth+laneGap)
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

// mergeExitFlowAnchor is the origin anchor an if's or a case's own
// `@anchor(from: X)` gives the flow leaving its closing merge, or nil when it
// sets none.
func mergeExitFlowAnchor(s ast.MicroflowStatement) *ast.FlowAnchors {
	own := stmtOwnAnchor(s)
	if own == nil || own.From == ast.AnchorSideUnset {
		return nil
	}
	return &ast.FlowAnchors{From: own.From, To: ast.AnchorSideUnset}
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
	bodyBounds := fb.measurer.measureStatementsSpan(s.Body)

	// Calculate loop box size with padding, plus the iterator column. The
	// measurer uses the same constant, so an IF holding this loop measures it at
	// the width it is built at.
	loopWidth := max(bodyBounds.Width+2*LoopPadding+LoopIteratorSpace, MinLoopWidth)
	loopHeight := max(bodyBounds.Height+2*LoopPadding, MinLoopHeight)

	// Inner positioning: activities start after the iterator icon on the left,
	// and are centred vertically within the loop box so that branching content
	// (IF/CASE) centred on innerStartY stays within [0, loopHeight].
	//
	// A position is a CENTRE, so the first activity has to start half its own
	// width past the iterator column; at LoopPadding+LoopIteratorSpace its left edge
	// landed 60px inside that column, on top of the iterator's icon and the list
	// variable's label.
	innerStartX := LoopPadding + LoopIteratorSpace + ActivityWidth/2
	innerStartY := loopHeight / 2

	// posX is where the builder would CENTRE the next element. A loop box placed
	// with its left edge there started 100px after the previous activity, where a
	// neighbouring activity would have started 40px after it; half an activity to
	// the left puts the box's edge where an activity's edge would be.
	loopLeftX := fb.posX - ActivityWidth/2
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
		textLang:     fb.textLang,
		posX:         innerStartX,
		posY:         innerStartY,
		baseY:        innerStartY,
		spacing:      HorizontalSpacing,
		varTypes:     fb.varTypes,     // Share variable scope
		declaredVars: fb.declaredVars, // Share declared vars (fixes nil map panic)
		measurer:     fb.measurer,     // Share measurer
		backend:      fb.backend,      // Share backend
		quiet:        fb.quiet,
		hierarchy:    fb.hierarchy,    // Share hierarchy
		restServices: fb.restServices, // Share REST services for parameter classification
		isNanoflow:   fb.isNanoflow,
		self:         fb.self,
		scriptFlows:  fb.scriptFlows,
		// Share the note registry, so a note declared outside the loop and
		// referenced on a body activity attaches to the SAME Annotation rather
		// than being refused. The describer emits exactly that (its label state
		// is shared across the loop overlay), so a builder that refused it would
		// reject its own DESCRIBE output.
		//
		// The resulting shape — Annotation in the parent collection, both
		// AnnotationFlows in the parent collection, one destination inside the
		// loop — was measured at 0 errors on mxbuild 11.13. mxbuild is the only
		// oracle available here; Studio Pro is stricter in general, though this
		// is the same split mxcli already ships for an ordinary loop-body note,
		// whose flow is hoisted to the parent a few lines below. (#1077)
		annotationsByLabel: fb.annotationsByLabel,
	}

	// Process loop body statements and connect them with flows.
	// pendingCase carries the deferred case value a merge-less split leaves for
	// the NEXT flow — e.g. the FALSE branch of `if X then break`, whose split has
	// no merge. Mirrors buildFlowGraph; without it a decision inside a loop loses
	// its false flow entirely and mx check reports CE0079 (ledger #52).
	var lastBodyID model.ID
	pendingCase := ""
	for i, stmt := range s.Body {
		loopBuilder.atBodyTail = i == len(s.Body)-1
		actID := loopBuilder.addStatement(stmt)
		loopBuilder.atBodyTail = false
		if actID != "" {
			loopBuilder.applyPendingAnnotations(actID)
			if lastBodyID != "" {
				if pendingCase != "" {
					loopBuilder.flows = append(loopBuilder.flows, newHorizontalFlowWithCase(lastBodyID, actID, pendingCase))
				} else {
					loopBuilder.flows = append(loopBuilder.flows, newHorizontalFlow(lastBodyID, actID))
				}
			}
			pendingCase = ""
			// Handle nextConnectionPoint for compound statements (nested IF, etc.)
			if loopBuilder.nextConnectionPoint != "" {
				lastBodyID = loopBuilder.nextConnectionPoint
				loopBuilder.nextConnectionPoint = ""
				pendingCase = loopBuilder.nextFlowCase
				loopBuilder.nextFlowCase = ""
			} else {
				lastBodyID = actID
			}
		}
	}
	// A merge-less split as the last body element (e.g. `if X then break`) leaves a
	// deferred branch (its FALSE case) with nowhere to go — the loop-body analog of
	// falling off the microflow end. Wire it to a Continue event: "didn't
	// break/return, so go to the next iteration" — the valid Mendix representation,
	// and the missing false flow that made mx check report CE0079 (ledger #52).
	if pendingCase != "" && lastBodyID != "" {
		loopBuilder.posX += HorizontalSpacing
		continueID := loopBuilder.addContinueEvent()
		loopBuilder.flows = append(loopBuilder.flows, newHorizontalFlowWithCase(lastBodyID, continueID, pendingCase))
	}

	// Size the box from the children that were actually built, not from the
	// statement count measured before the body existed (#884 problem 1). Recompute
	// the centre too: the width just changed under it.
	loopWidth, loopHeight = fitContainerSize(loopBuilder.objects, innerStartX, MinLoopWidth, MinLoopHeight)
	loopCenterX = loopLeftX + loopWidth/2
	if s.Annotations != nil && s.Annotations.Position != nil {
		loopCenterX = s.Annotations.Position.X
		loopLeftX = loopCenterX - loopWidth/2
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
	// The body's @curves belong on those flows, and are applied by whichever
	// builder ends up owning them; left on loopBuilder they were never applied
	// (mendixlabs/mxcli#1597).
	fb.adoptCurves(loopBuilder)
	fb.annotationFlows = append(fb.annotationFlows, loopBuilder.annotationFlows...)
	// Refusals raised in the body were collected into the sub-builder and
	// dropped on the floor, so exec reported success on a flow it had silently
	// written wrong (#1077).
	fb.errors = append(fb.errors, loopBuilder.errors...)
	if fb.annotationsByLabel == nil {
		fb.annotationsByLabel = loopBuilder.annotationsByLabel
	}

	// Re-apply this loop's own annotations now that its activity exists.
	if savedLoopAnnotations != nil {
		fb.applyAnnotations(loop.ID, savedLoopAnnotations)
	}

	// The next element is centred on posX, so an activity placed a full
	// HorizontalSpacing past the box's right edge sat 100px away from it, against
	// the 40px that separates two activities. Half an activity plus that same gap
	// puts it where a neighbour on the main line would be.
	fb.posX = loopLeftX + loopWidth + ActivityWidth/2 + (HorizontalSpacing - ActivityWidth)

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
	case *ast.SynchronizeStmt:
		return s.ErrorHandling
	case *ast.RestCallStmt:
		return s.ErrorHandling
	case *ast.SendRestRequestStmt:
		return s.ErrorHandling
	case *ast.SendEmailStmt:
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

	bodyBounds := fb.measurer.measureStatementsSpan(s.Body)

	loopWidth := max(bodyBounds.Width+2*LoopPadding, MinLoopWidth)
	loopHeight := max(bodyBounds.Height+2*LoopPadding, MinLoopHeight)

	// A child's Position is its CENTRE, so the first one's centre must sit half an
	// activity in from the padding or its left edge hangs outside the box. This
	// read `LoopPadding` alone and put every while loop's first activity at x=50
	// with its left edge at -10 — MPR011 on every while loop mxcli wrote, single
	// level included (ako/mxcli#645).
	//
	// The doc comment above says the layout "matches addLoopStatement but without
	// iterator icon space", and dropping the iterator space is right; taking
	// ActivityWidth/2 with it was not, because that term is not iterator space —
	// it is what converts a centre to a left edge. The Y line below always had it.
	innerStartX := LoopPadding + ActivityWidth/2
	innerStartY := LoopPadding + ActivityHeight/2

	// posX is where the builder would CENTRE the next element. A loop box placed
	// with its left edge there started 100px after the previous activity, where a
	// neighbouring activity would have started 40px after it; half an activity to
	// the left puts the box's edge where an activity's edge would be.
	loopLeftX := fb.posX - ActivityWidth/2
	loopCenterX := loopLeftX + loopWidth/2
	if s.Annotations != nil && s.Annotations.Position != nil {
		loopCenterX = s.Annotations.Position.X
		loopLeftX = loopCenterX - loopWidth/2
	}

	loopBuilder := &flowBuilder{
		textLang:     fb.textLang,
		posX:         innerStartX,
		posY:         innerStartY,
		baseY:        innerStartY,
		spacing:      HorizontalSpacing,
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
		// Share the note registry, so a note declared outside the loop and
		// referenced on a body activity attaches to the SAME Annotation rather
		// than being refused. The describer emits exactly that (its label state
		// is shared across the loop overlay), so a builder that refused it would
		// reject its own DESCRIBE output.
		//
		// The resulting shape — Annotation in the parent collection, both
		// AnnotationFlows in the parent collection, one destination inside the
		// loop — was measured at 0 errors on mxbuild 11.13. mxbuild is the only
		// oracle available here; Studio Pro is stricter in general, though this
		// is the same split mxcli already ships for an ordinary loop-body note,
		// whose flow is hoisted to the parent a few lines below. (#1077)
		annotationsByLabel: fb.annotationsByLabel,
	}

	// Body bookkeeping is addLoopStatement's, verbatim: a WHILE body is a loop
	// body, so the same merge-less split needs the same handling. pendingCase
	// carries the deferred case a split with no merge leaves for the NEXT flow —
	// the FALSE branch of `if X then break`. Dropping it shipped the decision with
	// only its true flow, which is CE0079 (#893 case 3); the LOOP builder had this
	// since ledger #52 and this one did not, so the two disagreed on the same body.
	var lastBodyID model.ID
	pendingCase := ""
	for i, stmt := range s.Body {
		loopBuilder.atBodyTail = i == len(s.Body)-1
		actID := loopBuilder.addStatement(stmt)
		loopBuilder.atBodyTail = false
		if actID != "" {
			loopBuilder.applyPendingAnnotations(actID)
			if lastBodyID != "" {
				if pendingCase != "" {
					loopBuilder.flows = append(loopBuilder.flows, newHorizontalFlowWithCase(lastBodyID, actID, pendingCase))
				} else {
					loopBuilder.flows = append(loopBuilder.flows, newHorizontalFlow(lastBodyID, actID))
				}
			}
			pendingCase = ""
			if loopBuilder.nextConnectionPoint != "" {
				lastBodyID = loopBuilder.nextConnectionPoint
				loopBuilder.nextConnectionPoint = ""
				pendingCase = loopBuilder.nextFlowCase
				loopBuilder.nextFlowCase = ""
			} else {
				lastBodyID = actID
			}
		}
	}
	// A merge-less split as the last body element leaves its deferred FALSE branch
	// with nowhere to go. Wire it to a Continue event — "didn't break, so go to the
	// next iteration" — which is the valid Mendix representation and the missing
	// false flow itself (ledger #52, applied here for #893).
	if pendingCase != "" && lastBodyID != "" {
		loopBuilder.posX += HorizontalSpacing
		continueID := loopBuilder.addContinueEvent()
		loopBuilder.flows = append(loopBuilder.flows, newHorizontalFlowWithCase(lastBodyID, continueID, pendingCase))
	}

	whileExpr := fb.exprToString(s.Condition)

	// Size from the built children, as addLoopStatement does (#884 problem 1).
	loopWidth, loopHeight = fitContainerSize(loopBuilder.objects, innerStartX, MinLoopWidth, MinLoopHeight)
	loopCenterX = loopLeftX + loopWidth/2
	if s.Annotations != nil && s.Annotations.Position != nil {
		loopCenterX = s.Annotations.Position.X
		loopLeftX = loopCenterX - loopWidth/2
	}

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
	fb.adoptCurves(loopBuilder) // see addLoopStatement (mendixlabs/mxcli#1597)
	fb.annotationFlows = append(fb.annotationFlows, loopBuilder.annotationFlows...)
	// Refusals raised in the body were collected into the sub-builder and
	// dropped on the floor, so exec reported success on a flow it had silently
	// written wrong (#1077).
	fb.errors = append(fb.errors, loopBuilder.errors...)
	if fb.annotationsByLabel == nil {
		fb.annotationsByLabel = loopBuilder.annotationsByLabel
	}

	if savedWhileAnnotations != nil {
		fb.applyAnnotations(loop.ID, savedWhileAnnotations)
	}

	// The next element is centred on posX, so an activity placed a full
	// HorizontalSpacing past the box's right edge sat 100px away from it, against
	// the 40px that separates two activities. Half an activity plus that same gap
	// puts it where a neighbour on the main line would be.
	fb.posX = loopLeftX + loopWidth + ActivityWidth/2 + (HorizontalSpacing - ActivityWidth)

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
