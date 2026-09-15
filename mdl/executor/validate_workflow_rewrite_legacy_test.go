// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"go.mongodb.org/mongo-driver/bson"
)

// legacyShaped returns raw decoded the way the legacy engine's GetRawUnit
// decodes a unit — bson.Unmarshal into map[string]any — which yields arrays as
// primitive.A. Measured end to end: on the legacy engine a `create or modify`
// that restated none of a workflow's two event handlers was written ("Created
// workflow") and deleted them, while the modelsdk engine refused it.
func legacyShaped(t *testing.T, raw map[string]any) map[string]any {
	t.Helper()
	data, err := bson.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := bson.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestWorkflowRewrite_GuardsSeeLegacyShapedUnits(t *testing.T) {
	boundary := map[string]any{
		"$Type": "Workflows$Workflow",
		"Flow": map[string]any{
			"$Type": "Workflows$Flow",
			"Activities": []any{3, map[string]any{
				"$Type":          "Workflows$CallMicroflowTask",
				"BoundaryEvents": []any{2, map[string]any{"$Type": "Workflows$InterruptingTimerBoundaryEvent"}},
			}},
		},
	}
	cases := []struct {
		name string
		raw  map[string]any
		src  string
		want string
	}{
		{"event handler", studioProWorkflow(false, true, false, "ConsensusCompletionCriteria", true), wfRewriteTwoTasks, "workflow.WorkflowEventHandle"},
		{"on-created microflow", studioProWorkflow(true, false, false, "ConsensusCompletionCriteria", true), wfRewriteTwoTasks, "workflow.UserTaskEventHandle"},
		{"AI agent task", studioProWorkflow(false, false, true, "ConsensusCompletionCriteria", true), wfRewriteTwoTasks, "AI agent task"},
		{"majority completion", studioProWorkflow(false, false, false, "MajorityCompletionCriteria", true), wfRewriteTwoTasks, "majority"},
		{"boundary event", boundary, wfNoBoundary, "boundary event"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stmt := parseWorkflowStmt(t, c.src)
			err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, legacyShaped(t, c.raw)), "wf1", "M.W", stmt)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("rewrite over a legacy-decoded unit must be refused naming %q, got %v", c.want, err)
			}
		})
	}

	// The control: the same legacy-shaped unit holding only what a rebuild
	// writes is still allowed, so the conversion does not refuse everything.
	plain := legacyShaped(t, studioProWorkflow(false, false, false, "ConsensusCompletionCriteria", true))
	if err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, plain), "wf1", "M.W", parseWorkflowStmt(t, wfRewriteTwoTasks)); err != nil {
		t.Errorf("a legacy-decoded unit that loses nothing must be allowed, got %v", err)
	}
}

func TestAlterWorkflow_ReplaceSeesLegacyShapedUnits(t *testing.T) {
	prog, errs := visitor.Build(`alter workflow M.W replace activity userTask1 with user task userTask1 'Task' page M.P outcomes 'Fast' { } 'Furious' { };`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.AlterWorkflowStmt)
	raw := legacyShaped(t, studioProWorkflow(true, false, false, "ConsensusCompletionCriteria", true))
	got := validateAlterReplaceKeepsStudioProState(storedReplaceCtx(t, raw), stmt)
	if len(got) != 1 || !strings.Contains(got[0], "workflow.UserTaskEventHandle") {
		t.Errorf("replace over a legacy-decoded unit must be refused, got %v", got)
	}
}
