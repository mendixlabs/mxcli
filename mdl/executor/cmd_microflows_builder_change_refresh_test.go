// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestChangeObjectBuilderWritesRefreshInClient(t *testing.T) {
	fb := &flowBuilder{}

	fb.addChangeObjectAction(&ast.ChangeObjectStmt{
		Variable:        "Customer",
		RefreshInClient: true,
		Changes: []ast.ChangeItem{
			{Attribute: "Name", Value: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "Jane"}},
		},
	})

	action := lastChangeObjectAction(t, fb)
	if !action.RefreshInClient {
		t.Fatal("Expected builder to write RefreshInClient")
	}
}

func lastChangeObjectAction(t *testing.T, fb *flowBuilder) *microflows.ChangeObjectAction {
	t.Helper()

	if len(fb.objects) == 0 {
		t.Fatal("Expected builder to create an action activity")
	}
	activity, ok := fb.objects[len(fb.objects)-1].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("Last object = %T, want ActionActivity", fb.objects[len(fb.objects)-1])
	}
	action, ok := activity.Action.(*microflows.ChangeObjectAction)
	if !ok {
		t.Fatalf("Action = %T, want ChangeObjectAction", activity.Action)
	}
	return action
}
