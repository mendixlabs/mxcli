// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1328: "@excluded on microflow activities does not seem to
// work anymore" — `@excluded declare $Variable Boolean = false;` was written
// with the activity enabled.
func TestExcludedActivityIsDisabled_Issue1328(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"declare", "@excluded declare $Variable Boolean = false; return;"},
		{"log", "@excluded log info node 'x' 'hi'; return;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := visitor.Build("create or modify microflow M.F () begin " + tc.body + " end;")
			for _, e := range errs {
				t.Fatalf("parse error: %v", e)
			}
			mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
			fb := &flowBuilder{
				posX: 100, posY: 100, spacing: HorizontalSpacing,
				varTypes: map[string]string{}, declaredVars: map[string]string{},
			}
			fb.buildFlowGraph(mf.Body, nil)
			var acts []*microflows.ActionActivity
			for _, obj := range fb.objects {
				if a, ok := obj.(*microflows.ActionActivity); ok {
					acts = append(acts, a)
				}
			}
			if len(acts) != 1 {
				t.Fatalf("want 1 activity, got %d", len(acts))
			}
			if !acts[0].Disabled {
				t.Errorf("activity written enabled: @excluded was dropped")
			}
		})
	}
}

// mergeStatementAnnotations copies ActivityAnnotations field by field, and
// #1328 was one field it skipped. Every field set on a statement must survive
// the merge unless it is deliberately consumed elsewhere (listed below), so a
// field added later fails here instead of being dropped in silence.
func TestMergeStatementAnnotationsCopiesEveryField(t *testing.T) {
	notMerged := map[string]string{
		"Start":          "read off the first statement directly (buildFlowGraph)",
		"InvalidCurves":  "validation only",
		"InvalidAnchors": "validation only",
		"InvalidNotes":   "validation only",
		"UnknownNames":   "validation only",
	}
	pos := &ast.Position{X: 1, Y: 2}
	anchors := &ast.FlowAnchors{From: ast.AnchorSideRight, To: ast.AnchorSideLeft}
	doc := "d"
	full := &ast.ActivityAnnotations{
		Position: pos, Caption: "c", CaptionSet: true, Color: "Green", Documentation: &doc,
		Notes:     []ast.MicroflowAnnotation{{Text: "n"}},
		FreeNotes: []ast.MicroflowAnnotation{{Text: "f"}},
		Excluded:  true, Anchor: anchors,
		TrueBranchAnchor: anchors, FalseBranchAnchor: anchors,
		IteratorAnchor: anchors, BodyTailAnchor: anchors,
		CaseAnchors: []ast.CaseAnchor{{Value: "A", Anchors: anchors}},
		Curve:       &ast.FlowCurve{From: pos, To: pos}, Merge: pos, Start: pos,
		InvalidCurves: []string{"x"}, InvalidAnchors: []string{"x"},
		InvalidNotes: []string{"x"}, UnknownNames: []string{"x"},
	}
	fb := &flowBuilder{}
	fb.mergeStatementAnnotations(&ast.LogStmt{Annotations: full})

	src, got := reflect.ValueOf(full).Elem(), reflect.ValueOf(fb.pendingAnnotations).Elem()
	for i := 0; i < src.NumField(); i++ {
		name := src.Type().Field(i).Name
		if _, skip := notMerged[name]; skip {
			continue
		}
		if src.Field(i).IsZero() {
			t.Fatalf("test fixture leaves %s zero — set it so the merge is exercised", name)
		}
		if !reflect.DeepEqual(src.Field(i).Interface(), got.Field(i).Interface()) {
			t.Errorf("mergeStatementAnnotations dropped %s", name)
		}
	}
}
