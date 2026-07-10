// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func mfWithLoopAnnotations(ann *ast.ActivityAnnotations) *ast.CreateMicroflowStmt {
	return &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "MF"},
		Body: []ast.MicroflowStatement{
			&ast.LoopStmt{
				LoopVariable: "Item",
				ListVariable: "Items",
				Annotations:  ann,
				Body:         []ast.MicroflowStatement{},
			},
		},
	}
}

func loopHasMDL042(stmt *ast.CreateMicroflowStmt) bool {
	for _, v := range ValidateMicroflow(stmt) {
		if v.RuleID == "MDL042" {
			return true
		}
	}
	return false
}

// TestValidateMicroflow_CaptionOnLoopWarns guards MDL042: @caption on a loop is
// silently dropped (Mendix loops have no Caption property), so check must warn.
func TestValidateMicroflow_CaptionOnLoopWarns(t *testing.T) {
	if !loopHasMDL042(mfWithLoopAnnotations(&ast.ActivityAnnotations{Caption: "Process things"})) {
		t.Error("expected MDL042 warning for @caption on a loop")
	}
}

// @annotation (the supported way to label a loop) must NOT warn.
func TestValidateMicroflow_AnnotationOnLoopNoWarn(t *testing.T) {
	if loopHasMDL042(mfWithLoopAnnotations(&ast.ActivityAnnotations{AnnotationText: "Process things"})) {
		t.Error("MDL042 must not fire for @annotation on a loop")
	}
}

// A loop with no annotations must not warn.
func TestValidateMicroflow_PlainLoopNoWarn(t *testing.T) {
	if loopHasMDL042(mfWithLoopAnnotations(nil)) {
		t.Error("MDL042 must not fire for a plain loop")
	}
}
