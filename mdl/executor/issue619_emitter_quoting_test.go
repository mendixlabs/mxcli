// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

// Issue #619 — DESCRIBE emitters must quote element names that collide with a
// reserved keyword so the output re-parses through `mxcli check`. These four
// positions emit into grammar rules that already accept QUOTED_IDENTIFIER, so
// wrapping the name in mdlIdent is sufficient (no grammar change needed).

func TestWorkflowJumpTo_QuotesReservedTarget(t *testing.T) {
	activity := &workflows.JumpToActivity{TargetActivity: "List"}
	activity.Name = "jumpAct1"

	output := strings.Join(formatSingleActivity(activity, ""), "\n")

	if !strings.Contains(output, `jump to "List" comment`) {
		t.Errorf("expected reserved jump-to target to be quoted, got:\n%s", output)
	}
}

func TestWorkflowJumpTo_LeavesPlainTargetUnquoted(t *testing.T) {
	activity := &workflows.JumpToActivity{TargetActivity: "Review"}
	activity.Name = "jumpAct1"

	output := strings.Join(formatSingleActivity(activity, ""), "\n")

	if !strings.Contains(output, "jump to Review comment") {
		t.Errorf("expected non-reserved jump-to target to stay unquoted, got:\n%s", output)
	}
}

func TestWorkflowUserTask_QuotesReservedName(t *testing.T) {
	task := &workflows.UserTask{}
	task.Name = "Value"
	task.Caption = "Approve"

	output := strings.Join(formatUserTask(task, ""), "\n")

	if !strings.Contains(output, `user task "Value" 'Approve'`) {
		t.Errorf("expected reserved user-task name to be quoted, got:\n%s", output)
	}
}

func TestFragmentWidget_QuotesReservedName(t *testing.T) {
	widget := &ast.WidgetV3{
		Type:       "container",
		Name:       "List",
		Properties: map[string]any{},
		Children:   []*ast.WidgetV3{},
	}

	var buf bytes.Buffer
	outputASTWidgetMDL(&buf, widget, 0)

	if !strings.Contains(buf.String(), `container "List"`) {
		t.Errorf("expected reserved fragment widget name to be quoted, got:\n%s", buf.String())
	}
}

func TestNavigationListItem_QuotesReservedName(t *testing.T) {
	w := rawWidget{
		Type: "Pages$NavigationList",
		Name: "nav1",
		Children: []rawWidget{
			{Type: "Pages$NavigationListItem", Name: "List"},
		},
	}

	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}
	outputWidgetMDLV3(ctx, w, 0)

	if !strings.Contains(buf.String(), `item "List"`) {
		t.Errorf("expected reserved navigationlist item name to be quoted, got:\n%s", buf.String())
	}
}
