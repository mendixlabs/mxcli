// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// TestBuildFlowGraph_ManualWhileTrueIgnoresNestedLoopContinue pins issue #404:
// a `while true` whose only `continue` lives inside a nested collection loop
// must NOT be classified as a manual back-edge candidate. The outer flow
// should be built as a regular LoopedActivity (with a WhileLoopCondition).
//
// Before the fix, containsContinueStmt recursed into nested LoopStmt bodies
// asymmetrically with containsBreakForCurrentLoop, so isManualWhileTrueCandidate
// returned true and the outer while was rebuilt as an ExclusiveMerge back-edge,
// creating an unconditional infinite loop in the BSON graph.
func TestBuildFlowGraph_ManualWhileTrueIgnoresNestedLoopContinue(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.WhileStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			Body: []ast.MicroflowStatement{
				&ast.LoopStmt{
					LoopVariable: "item",
					ListVariable: "items",
					Body: []ast.MicroflowStatement{
						&ast.ContinueStmt{},
					},
				},
				// No outer-scope continue / return / raise: the outer while
				// has no terminal signal of its own.
			},
		},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		measurer:     &layoutMeasurer{},
		varTypes:     map[string]string{"items": "List of Sample.Item"},
		declaredVars: map[string]string{"items": "List of Sample.Item"},
	}
	oc := fb.buildFlowGraph(body, nil)

	var (
		outerLoop  *microflows.LoopedActivity
		mergeCount int
	)
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.LoopedActivity:
			// The first looped activity at this scope is the outer while.
			if outerLoop == nil {
				outerLoop = o
			}
		case *microflows.ExclusiveMerge:
			mergeCount++
		}
	}

	if outerLoop == nil {
		t.Fatal("outer `while true` must be built as a LoopedActivity, not an ExclusiveMerge back-edge")
	}
	if outerLoop.LoopSource == nil {
		t.Fatal("outer LoopedActivity must have a LoopSource (WhileLoopCondition for `while true`)")
	}
	if mergeCount != 0 {
		t.Errorf("manual back-edge ExclusiveMerge must not be emitted; got %d ExclusiveMerge node(s)", mergeCount)
	}
}
