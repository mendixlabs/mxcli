// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// TestInheritanceSplitAlwaysEmitsMergeWhenBranchContinues guards against a
// describe/exec roundtrip regression where `addStructuredInheritanceSplit`
// used to take a "no-merge shortcut" when exactly one non-split branch
// continued: it wired the parent's next statement directly to the
// continuing case's tail. Two things broke:
//
//  1. Re-describe emitted the parent's continuation inside the case body
//     (visually burying statements in the wrong scope).
//  2. Studio Pro raised CE0079 ("condition value should be configured for
//     an outgoing flow") on terminating branches because their cases had
//     no merge to converge on.
func TestInheritanceSplitAlwaysEmitsMergeWhenBranchContinues(t *testing.T) {
	fb := &flowBuilder{
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
	}

	// An InheritanceSplit with one continuing case (`CastedA`) and a
	// terminating else that returns. Before the fix this took the no-merge
	// shortcut because branchTails == 1 && !fromSplit.
	fb.addStructuredInheritanceSplit(&ast.InheritanceSplitStmt{
		Variable: "obj",
		Cases: []ast.InheritanceSplitCase{
			{
				Entity: ast.QualifiedName{Module: "M", Name: "CastedA"},
				Body: []ast.MicroflowStatement{
					&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "continue"}},
				},
			},
		},
		ElseBody: []ast.MicroflowStatement{
			&ast.ReturnStmt{},
		},
	})

	var merge *microflows.ExclusiveMerge
	for _, obj := range fb.objects {
		if m, ok := obj.(*microflows.ExclusiveMerge); ok {
			merge = m
		}
	}
	if merge == nil {
		t.Fatal("expected ExclusiveMerge to be created when one branch continues — no-merge shortcut regression")
	}
}
