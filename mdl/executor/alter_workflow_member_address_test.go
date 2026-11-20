// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ako/mxcli#791: `drop userTask1 path 1` deleted the user task's first outcome
// and reported "Altered workflow". The mutators now refuse it; this pass makes
// `check --references` refuse it too, against the stored workflow.
func TestAlterWorkflow_DropPathOnNonSplitIsRefusedAtCheck(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string // "" = no member-address error
	}{
		{`alter workflow Sales.WF { drop task1 path 1 };`, "not a parallel split"},
		{`alter workflow Sales.WF { drop decision9 path 1 };`, "not a parallel split"},
		{`alter workflow Sales.WF { drop callMicroflow1 path 1 };`, "not a parallel split"},
		{`alter workflow Sales.WF drop path 'Path 1' on task1;`, "not a parallel split"},
		// Control: a split's path is what the op is for.
		{`alter workflow Sales.WF { drop split1 path 2 };`, ""},
		{`alter workflow Sales.WF { drop split1 path 3 };`, "has 2 paths"},
		{`alter workflow Sales.WF drop path '' on split1;`, "names no path"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			ctx, _ := wfKindFixture(t)
			errs := strings.Join(alterWfRefErrors(t, ctx, tc.src), "\n")
			if tc.want == "" {
				if strings.Contains(errs, "drop path") {
					t.Fatalf("refused a valid drop: %s", errs)
				}
				return
			}
			if !strings.Contains(errs, tc.want) {
				t.Fatalf("want %q, got: %s", tc.want, errs)
			}
		})
	}
}

// boundaryDropCtx serves one user task holding n boundary events.
func boundaryDropCtx(t *testing.T, n int, mut *mock.MockWorkflowMutator) (*ExecContext, *bytes.Buffer) {
	t.Helper()
	mod := mkModule("Sales")
	wf := mkWorkflow(mod.ID, "WF")
	task := &workflows.UserTask{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
		BaseElement: model.BaseElement{ID: "a1"}, Name: "task1"}}
	for i := 0; i < n; i++ {
		task.BoundaryEvents = append(task.BoundaryEvents, &workflows.BoundaryEvent{EventType: "InterruptingTimer"})
	}
	wf.Flow = &workflows.Flow{Activities: []workflows.WorkflowActivity{task}}
	h := mkHierarchy(mod)
	withContainer(h, wf.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:             func() bool { return true },
		ListWorkflowsFunc:           func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
		OpenWorkflowForMutationFunc: func(model.ID) (backend.WorkflowMutator, error) { return mut, nil },
	}
	var out bytes.Buffer
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	ctx.Output = &out
	return ctx, &out
}

// `drop X boundary event` has no way to say WHICH event, and the mutators drop
// the first. With several, that is a guess: under mdl 1 it is refused, under
// mdl 0 it still drops the first, with the MDL-V1-BOUNDARYDROP warning.
func TestAlterWorkflow_DropBoundaryEventAmongSeveral(t *testing.T) {
	const src = `alter workflow Sales.WF { drop task1 boundary event };`
	for _, tc := range []struct {
		name    string
		events  int
		v       langver.Version
		dropped bool
		refused bool
		warned  bool
	}{
		{"one event, mdl 0", 1, langver.V0, true, false, false},
		{"one event, mdl 1", 1, langver.V1, true, false, false},
		{"two events, mdl 0", 2, langver.V0, true, false, true},
		{"two events, mdl 1", 2, langver.V1, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dropped := false
			mut := &mock.MockWorkflowMutator{
				DropBoundaryEventFunc: func(string, int, int) error { dropped = true; return nil },
			}
			ctx, out := boundaryDropCtx(t, tc.events, mut)
			ctx.LanguageVersion = tc.v
			prog, perrs := visitor.Build(src)
			if len(perrs) > 0 {
				t.Fatal(perrs)
			}
			err := execAlterWorkflow(ctx, prog.Statements[0].(*ast.AlterWorkflowStmt))
			if refused := err != nil; refused != tc.refused {
				t.Fatalf("refused = %v (%v), want %v", refused, err, tc.refused)
			}
			if tc.refused && !strings.Contains(err.Error(), "has 2 boundary events") {
				t.Errorf("refusal does not name the count: %v", err)
			}
			if dropped != tc.dropped {
				t.Errorf("dropped = %v, want %v", dropped, tc.dropped)
			}
			if warned := strings.Contains(out.String(), "MDL-V1-BOUNDARYDROP"); warned != tc.warned {
				t.Errorf("warned = %v, want %v; output:\n%s", warned, tc.warned, out.String())
			}
		})
	}
}

// ako/mxcli#808: `boundary event N` names one event among several, in describe
// order, under either language version; out of range is refused before the
// mutator is reached.
func TestAlterWorkflow_DropBoundaryEventByIndex(t *testing.T) {
	for _, tc := range []struct {
		src     string
		v       langver.Version
		index   int // index handed to the mutator; 0 = not reached
		refused string
	}{
		{`alter workflow Sales.WF { drop task1 boundary event 2 };`, langver.V1, 2, ""},
		{`alter workflow Sales.WF { drop task1 boundary event 1 };`, langver.V0, 1, ""},
		{`alter workflow Sales.WF { drop task1 boundary event 3 };`, langver.V1, 0, "has 2 boundary event"},
		{`alter workflow Sales.WF { drop task1 boundary event 3 };`, langver.V0, 0, "has 2 boundary event"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			got := 0
			mut := &mock.MockWorkflowMutator{
				DropBoundaryEventFunc: func(_ string, _ int, index int) error { got = index; return nil },
			}
			ctx, out := boundaryDropCtx(t, 2, mut)
			ctx.LanguageVersion = tc.v
			prog, perrs := visitor.Build(tc.src)
			if len(perrs) > 0 {
				t.Fatal(perrs)
			}
			err := execAlterWorkflow(ctx, prog.Statements[0].(*ast.AlterWorkflowStmt))
			if tc.refused != "" {
				if err == nil || !strings.Contains(err.Error(), tc.refused) {
					t.Fatalf("want refusal %q, got %v", tc.refused, err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if got != tc.index {
				t.Errorf("mutator got index %d, want %d", got, tc.index)
			}
			if strings.Contains(out.String(), "MDL-V1-BOUNDARYDROP") {
				t.Errorf("an addressed drop is not a guess; got warning:\n%s", out.String())
			}
		})
	}
}

func TestVisitor_BoundaryEventIndexZeroRefused(t *testing.T) {
	if _, errs := visitor.Build(`alter workflow Sales.WF { drop task1 boundary event 0 };`); len(errs) == 0 {
		t.Fatal("boundary event 0 accepted; indexes are 1-based")
	}
}

// ako/mxcli#808: a Name match wins over caption matches without @N, as in the
// mutator (preferNameMatches). A jump captioned with its target's name used to
// make the reference ambiguous here, so the check-time guards went silent.
func TestResolveStoredActivity_PrefersName(t *testing.T) {
	task := &workflows.UserTask{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
		BaseElement: model.BaseElement{ID: "a1"}, Name: "task1", Caption: "Review"}}
	jump := &workflows.JumpToActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
		BaseElement: model.BaseElement{ID: "a2"}, Name: "jump1", Caption: "task1"}}
	flow := &workflows.Flow{Activities: []workflows.WorkflowActivity{task, jump}}
	if got := resolveStoredActivity(flow, "task1", 0); got != task {
		t.Fatalf("task1 resolved to %v, want the task named task1", got)
	}
	// @N still counts every match in describe order.
	if got := resolveStoredActivity(flow, "task1", 2); got != jump {
		t.Fatalf("task1@2 resolved to %v, want the jump", got)
	}
	// Two caption matches and no name match stay ambiguous.
	other := &workflows.JumpToActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
		BaseElement: model.BaseElement{ID: "a3"}, Name: "jump2", Caption: "Review"}}
	flow.Activities = append(flow.Activities, other)
	if got := resolveStoredActivity(flow, "Review", 0); got != nil {
		t.Fatalf("ambiguous caption resolved to %v", got)
	}
}
