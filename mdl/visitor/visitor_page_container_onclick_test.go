// SPDX-License-Identifier: Apache-2.0

// Issue #603: a CONTAINER is clickable. Both the canonical Action: keyword and
// the OnClick: alias must parse the action value behind it into the widget's
// Action property (the executor reads GetAction() for the container's
// OnClickAction).

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestContainerOnClickAliasParsesAsAction(t *testing.T) {
	for _, prop := range []string{"OnClick", "Action"} {
		t.Run(prop, func(t *testing.T) {
			input := `CREATE PAGE M.P (Title: 'P') {
				CONTAINER box (` + prop + `: MICROFLOW M.ACT_Foo, Class: 'clickable') {
					DYNAMICTEXT t (Content: 'x')
				}
			};`

			prog, errs := Build(input)
			if len(errs) > 0 {
				for _, err := range errs {
					t.Errorf("Parse error: %v", err)
				}
				t.FailNow()
			}

			stmt, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
			if !ok {
				t.Fatalf("Expected CreatePageStmtV3, got %T", prog.Statements[0])
			}
			box := findChildByName2(stmt.Widgets, "box")
			if box == nil {
				t.Fatal("container 'box' not found")
			}
			action := box.GetAction()
			if action == nil {
				t.Fatalf("%s: container Action is nil — the click action was dropped", prop)
			}
			if action.Type != "microflow" || action.Target != "M.ACT_Foo" {
				t.Errorf("%s: got type=%q target=%q, want microflow/M.ACT_Foo", prop, action.Type, action.Target)
			}
		})
	}
}
