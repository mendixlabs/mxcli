// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// TestIfEmptyElseBodyWithContinuingThenEmitsFalseFlowToMerge is a regression
// guard for CE0079 ("The 'false' condition value should be configured in
// properties for an outgoing flow").
//
// Pattern: an IF with THEN that continues and an explicitly empty ELSE body.
// Both branches must feed a merge, but an empty ELSE has no lastElseID to wire.
func TestIfEmptyElseBodyWithContinuingThenEmitsFalseFlowToMerge(t *testing.T) {
	fb := &flowBuilder{
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
	}

	fb.addIfStatement(&ast.IfStmt{
		Condition: &ast.VariableExpr{Name: "Flag"},
		ThenBody: []ast.MicroflowStatement{
			&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "in-then"}},
		},
		// The source had an explicit ELSE token, but no statements in that
		// branch. That must still build a false split->merge flow.
		HasElse: true,
	})

	var split *microflows.ExclusiveSplit
	var merge *microflows.ExclusiveMerge
	for _, obj := range fb.objects {
		switch o := obj.(type) {
		case *microflows.ExclusiveSplit:
			split = o
		case *microflows.ExclusiveMerge:
			merge = o
		}
	}
	if split == nil {
		t.Fatal("expected ExclusiveSplit to be created")
	}
	if merge == nil {
		t.Fatal("expected ExclusiveMerge to be created")
	}

	var hasFalseFlow bool
	for _, flow := range fb.flows {
		if flow.OriginID != split.ID || flow.DestinationID != merge.ID {
			continue
		}
		if flowCaseString(flow.CaseValue) == "false" {
			hasFalseFlow = true
		}
	}
	if !hasFalseFlow {
		t.Fatal("missing split->merge flow with case \"false\"")
	}
}
