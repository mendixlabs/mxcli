// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2328 (mendixlabs/mxcli#1871): an activity's documentation had no
// MDL spelling — describe never printed it and a script could not set it — so
// an activity replaced in place by `create or modify` lost it with no way to
// carry it from the script. `@documentation 'text'` now sets it on an action,
// a decision and a loop, and describe prints it.
func TestActivityDocumentationAnnotation_Builds_2328(t *testing.T) {
	fb := buildFlowFromMDL(t, "@documentation 'Reads the open orders'\n"+
		"$L = call microflow M.SUB_GetOrders();\n"+
		"@documentation 'Any open?'\n"+
		"if $L != empty then\n  return;\nend if;")
	var action, split string
	for _, o := range fb.objects {
		switch x := o.(type) {
		case *microflows.ActionActivity:
			action = x.Documentation
		case *microflows.ExclusiveSplit:
			split = x.Documentation
		}
	}
	if action != "Reads the open orders" || split != "Any open?" {
		t.Errorf("documentation: action %q, decision %q", action, split)
	}
}

func TestActivityDocumentationAnnotation_Described_2328(t *testing.T) {
	var lines []string
	act := &microflows.ActionActivity{}
	act.Documentation = "Line one\nit's here"
	emitObjectAnnotations(act, &lines, "  ", nil, nil, nil, nil)
	got := strings.Join(lines, "\n")
	if !strings.Contains(got, "@documentation 'Line one\nit''s here'") {
		t.Fatalf("describe printed %q", got)
	}
	// It parses back to the same text.
	prog, errs := visitor.Build("create microflow M.F ()\nbegin\n" + got + "\n  $x = 1;\nend;")
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, got)
	}
	ann := ast.StatementAnnotations(prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0])
	if ann == nil || ann.Documentation == nil || *ann.Documentation != "Line one\nit's here" {
		t.Errorf("parsed back %+v", ann)
	}
}

// A statement that leaves @documentation out states nothing about it, so
// `create or modify` keeps the stored activity — and its documentation — as
// it does a node's geometry. One that states a different text is a change.
func TestActivityDocumentationUnstatedMatches_2328(t *testing.T) {
	doc, other := "Reads the open orders", "Something else"
	stored := &ast.ActivityAnnotations{Documentation: &doc}
	if !declaredMatches(&ast.ActivityAnnotations{}, stored) {
		t.Error("a statement without @documentation does not match its documented stored activity")
	}
	if !declaredMatches(&ast.ActivityAnnotations{Documentation: &doc}, stored) {
		t.Error("the same @documentation does not match")
	}
	if declaredMatches(&ast.ActivityAnnotations{Documentation: &other}, stored) {
		t.Error("a different @documentation matched as unchanged")
	}
	if sameIgnoringLayout(&ast.ActivityAnnotations{Documentation: &other}, stored) {
		t.Error("a documentation change was taken for a layout-only change")
	}
}
