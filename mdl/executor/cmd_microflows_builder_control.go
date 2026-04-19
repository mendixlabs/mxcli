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
	thenBounds := fb.measurer.measureStatements(s.ThenBody)
	elseBounds := fb.measurer.measureStatements(s.ElseBody)

	// Calculate branch width (max of both branches)
	branchWidth := max(thenBounds.Width, elseBounds.Width)
	if branchWidth == 0 {
		branchWidth = HorizontalSpacing / 2
	}

	// Check if branches end with RETURN (creating their own EndEvents)
	thenReturns := lastStmtIsReturn(s.ThenBody)
	hasElseBody := len(s.ElseBody) > 0
	elseReturns := hasElseBody && lastStmtIsReturn(s.ElseBody)
	bothReturn := hasElseBody && thenReturns && elseReturns

	// Save/restore endsWithReturn around branch processing to avoid
	// a branch's RETURN affecting the parent flow state prematurely
	savedEndsWithReturn := fb.endsWithReturn

	// Save current center line position
	splitX := fb.posX
	centerY := fb.posY // This is the center line for the happy path

	// Create ExclusiveSplit with expression condition
	splitCondition := &microflows.ExpressionSplitCondition{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		Expression:  fb.exprToString(s.Condition),
	}

	split := &microflows.ExclusiveSplit{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: splitX, Y: centerY},
			Size:        model.Size{Width: SplitWidth, Height: SplitHeight},
		},
		Caption:           fb.exprToString(s.Condition),
		SplitCondition:    splitCondition,
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
	}
	fb.objects = append(fb.objects, split)
	splitID := split.ID

	// Calculate merge position (after the longest branch)
	mergeX := splitX + SplitWidth + HorizontalSpacing/2 + branchWidth + HorizontalSpacing/2

	// Determine if the merge would have 2+ incoming edges (non-redundant).
	// Skip merge when only one branch flows into it (the other returns).
	needMerge := false
	if !bothReturn {
		if hasElseBody {
			needMerge = !thenReturns && !elseReturns // both branches continue → 2 inputs
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

	if hasElseBody {
		// IF WITH ELSE: TRUE path horizontal (happy path), FALSE path below
		fb.posX = thenStartX
		fb.posY = centerY
		fb.endsWithReturn = false

		var lastThenID model.ID
		for _, stmt := range s.ThenBody {
			actID := fb.addStatement(stmt)
			if actID != "" {
				if lastThenID == "" {
					// First statement in THEN - connect from split with "true" case
					fb.flows = append(fb.flows, newHorizontalFlowWithCase(splitID, actID, "true"))
				} else {
					fb.flows = append(fb.flows, newHorizontalFlow(lastThenID, actID))
				}
				// For nested compound statements, use their exit point
				if fb.nextConnectionPoint != "" {
					lastThenID = fb.nextConnectionPoint
					fb.nextConnectionPoint = ""
				} else {
					lastThenID = actID
				}
			}
		}

		// Connect THEN body to merge only if it doesn't end with RETURN
		if !thenReturns {
			if lastThenID != "" {
				fb.flows = append(fb.flows, newHorizontalFlow(lastThenID, mergeID))
			} else {
				// Empty THEN body - connect split directly to merge with true case
				fb.flows = append(fb.flows, newHorizontalFlowWithCase(splitID, mergeID, "true"))
			}
		}

		// Process ELSE body (below the THEN path)
		elseCenterY := centerY + VerticalSpacing
		fb.posX = thenStartX
		fb.posY = elseCenterY
		fb.endsWithReturn = false

		var lastElseID model.ID
		for _, stmt := range s.ElseBody {
			actID := fb.addStatement(stmt)
			if actID != "" {
				if lastElseID == "" {
					// First statement in ELSE - connect from split going down (false path)
					fb.flows = append(fb.flows, newDownwardFlowWithCase(splitID, actID, "false"))
				} else {
					fb.flows = append(fb.flows, newHorizontalFlow(lastElseID, actID))
				}
				// For nested compound statements, use their exit point
				if fb.nextConnectionPoint != "" {
					lastElseID = fb.nextConnectionPoint
					fb.nextConnectionPoint = ""
				} else {
					lastElseID = actID
				}
			}
		}

		// Connect ELSE body to merge only if it doesn't end with RETURN
		if !elseReturns {
			if lastElseID != "" {
				fb.flows = append(fb.flows, newUpwardFlow(lastElseID, mergeID))
			}
		}
	} else {
		// IF WITHOUT ELSE: FALSE path horizontal (happy path), TRUE path below
		// This keeps the "do nothing" path straight and the "do something" path below

		if needMerge {
			// FALSE path: connect split directly to merge horizontally
			fb.flows = append(fb.flows, newHorizontalFlowWithCase(splitID, mergeID, "false"))
		}
		// When !needMerge (thenReturns): FALSE flow is deferred — the parent will
		// connect splitID to the next activity with nextFlowCase="false".

		// TRUE path: goes below the main line
		thenCenterY := centerY + VerticalSpacing
		fb.posX = thenStartX
		fb.posY = thenCenterY
		fb.endsWithReturn = false

		var lastThenID model.ID
		for _, stmt := range s.ThenBody {
			actID := fb.addStatement(stmt)
			if actID != "" {
				if lastThenID == "" {
					// First statement in THEN - connect from split going down with "true" case
					fb.flows = append(fb.flows, newDownwardFlowWithCase(splitID, actID, "true"))
				} else {
					fb.flows = append(fb.flows, newHorizontalFlow(lastThenID, actID))
				}
				// For nested compound statements, use their exit point
				if fb.nextConnectionPoint != "" {
					lastThenID = fb.nextConnectionPoint
					fb.nextConnectionPoint = ""
				} else {
					lastThenID = actID
				}
			}
		}

		// Connect THEN body to merge only if it doesn't end with RETURN
		if !thenReturns {
			if lastThenID != "" {
				fb.flows = append(fb.flows, newUpwardFlow(lastThenID, mergeID))
			} else {
				// Empty THEN body - connect split directly to merge going down and back up
				fb.flows = append(fb.flows, newDownwardFlowWithCase(splitID, mergeID, "true"))
			}
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
		fb.nextConnectionPoint = splitID
		// Tell parent to attach the case value on the next flow
		if hasElseBody {
			if thenReturns {
				fb.nextFlowCase = "false"
			} else {
				fb.nextFlowCase = "true"
			}
		} else {
			fb.nextFlowCase = "false" // IF without ELSE: false is the continuing path
		}
	}

	return splitID
}

// addLoopStatement creates a LOOP statement using LoopedActivity.
// Layout: Auto-sizes the loop box to fit content with padding
func (fb *flowBuilder) addLoopStatement(s *ast.LoopStmt) model.ID {
	// First, measure the loop body to determine size
	bodyBounds := fb.measurer.measureStatements(s.Body)

	// Calculate loop box size with padding
	// Extra width for iterator icon and its label (100 pixels)
	iteratorSpace := 100
	loopWidth := max(bodyBounds.Width+2*LoopPadding+iteratorSpace, MinLoopWidth)
	loopHeight := max(bodyBounds.Height+2*LoopPadding, MinLoopHeight)

	// Inner positioning: activities need to be offset from the iterator icon
	// The iterator takes up space in the top-left, so we need extra X offset
	// Looking at working Mendix loops, inner content starts further right
	innerStartX := LoopPadding + iteratorSpace    // Extra offset for iterator icon and label
	innerStartY := LoopPadding + ActivityHeight/2 // Center activities vertically with some padding

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
	}

	// Process loop body statements and connect them with flows
	var lastBodyID model.ID
	for _, stmt := range s.Body {
		actID := loopBuilder.addStatement(stmt)
		if actID != "" {
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
			Position:    model.Point{X: fb.posX + loopWidth/2, Y: fb.posY},
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
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
	}

	fb.objects = append(fb.objects, loop)

	// Add the internal flows to the parent's flows (top-level), not inside loop
	// This is how Mendix stores them - all flows at the microflow level
	fb.flows = append(fb.flows, loopBuilder.flows...)

	fb.posX += loopWidth + HorizontalSpacing

	return loop.ID
}

// addWhileStatement creates a WHILE loop using LoopedActivity with WhileLoopCondition.
// Layout matches addLoopStatement but without iterator icon space.
func (fb *flowBuilder) addWhileStatement(s *ast.WhileStmt) model.ID {
	bodyBounds := fb.measurer.measureStatements(s.Body)

	loopWidth := max(bodyBounds.Width+2*LoopPadding, MinLoopWidth)
	loopHeight := max(bodyBounds.Height+2*LoopPadding, MinLoopHeight)

	innerStartX := LoopPadding
	innerStartY := LoopPadding + ActivityHeight/2

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
	}

	var lastBodyID model.ID
	for _, stmt := range s.Body {
		actID := loopBuilder.addStatement(stmt)
		if actID != "" {
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
			Position:    model.Point{X: fb.posX + loopWidth/2, Y: fb.posY},
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
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
	}

	fb.objects = append(fb.objects, loop)
	fb.flows = append(fb.flows, loopBuilder.flows...)
	fb.posX += loopWidth + HorizontalSpacing

	return loop.ID
}
