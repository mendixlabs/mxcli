// SPDX-License-Identifier: Apache-2.0

// Rewriting a microflow from its own DESCRIBE output moved its workflow actions:
// `@position` was parsed and then dropped, because the two type switches that
// carried a statement's annotations — setStatementAnnotations in the visitor and
// getStatementAnnotations here — had no case for any workflow statement. The
// builder then auto-placed each action after the stored start position, so the
// first one landed on the end event. LOG statements round-tripped fine; they
// were in both switches. Both now read and write the field by reflection.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func TestPositionAnnotationPlacesEveryActionStatement(t *testing.T) {
	for _, tc := range []struct{ name, stmt string }{
		{"call workflow", "$Wf2 = call workflow W.WF ($Obj);"},
		{"get workflow data", "$Data = get workflow data $Workflow as W.WF;"},
		{"get workflows", "$Wfs = get workflows for $Obj;"},
		{"get workflow activity records", "$Recs = get workflow activity records $Workflow;"},
		{"workflow operation abort", "workflow operation abort $Workflow reason 'stop';"},
		{"workflow operation pause", "workflow operation pause $Workflow;"},
		{"set task outcome", "set task outcome $Task 'Approve';"},
		{"open user task", "open user task $Task;"},
		{"notify workflow", "$Ok = notify workflow $Workflow;"},
		{"open workflow", "open workflow $Workflow;"},
		{"lock workflow", "lock workflow $Workflow;"},
		{"lock workflow all", "lock workflow all;"},
		{"unlock workflow", "unlock workflow $Workflow;"},
		{"unlock workflow all", "unlock workflow all;"},
		{"import from mapping", "$T = import from mapping W.IMM($Body);"},
		{"export to mapping", "$Json = export to mapping W.EXM($Obj);"},
		{"transform json", "$Out = transform $In with W.Transformer;"},
		// Control: in both switches before the fix, so it never drifted.
		{"log (control)", "log info node 'NT' 'hello';"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "create microflow W.MF ( $Workflow: System.Workflow )\nbegin\n" +
				"  @position(740, 320)\n  " + tc.stmt + "\nend;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
			if !ok || len(mf.Body) != 1 {
				t.Fatalf("expected a microflow with one statement, got %T", prog.Statements[0])
			}

			fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, measurer: &layoutMeasurer{}}
			id := fb.addStatement(mf.Body[0])
			for _, obj := range fb.objects {
				if obj.GetID() != id {
					continue
				}
				if got := obj.GetPosition(); got.X != 740 || got.Y != 320 {
					t.Errorf("%T placed at (%d, %d), want the annotated (740, 320)", mf.Body[0], got.X, got.Y)
				}
				return
			}
			t.Fatalf("addStatement(%T) returned an id that is not among the built objects", mf.Body[0])
		})
	}
}
