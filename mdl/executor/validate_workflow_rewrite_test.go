// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/sdk/workflows"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// storedWorkflowCtx returns a context whose raw stored workflow unit carries the
// given nested $Type nodes, so the guard has something to find.
func storedWorkflowCtx(t *testing.T, nested ...string) *ExecContext {
	t.Helper()
	acts := make([]any, 0, len(nested))
	for _, ty := range nested {
		acts = append(acts, map[string]any{"$Type": ty})
	}
	raw := map[string]any{
		"$Type": "Workflows$Workflow",
		"Flow": map[string]any{
			"$Type": "Workflows$Flow",
			"Activities": []any{
				map[string]any{
					"$Type":          "Workflows$CallMicroflowTask",
					"BoundaryEvents": acts,
				},
			},
		},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc:  func(model.ID) (map[string]any, error) { return raw, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

func parseWorkflowStmt(t *testing.T, src string) *ast.CreateWorkflowStmt {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	s, ok := prog.Statements[0].(*ast.CreateWorkflowStmt)
	if !ok {
		t.Fatalf("statement is %T", prog.Statements[0])
	}
	return s
}

const wfNoBoundary = `create or replace workflow M.W parameter $C: M.Ctx
begin
  call microflow M.ACT_Step comment 'Step';
end workflow;`

const wfWithBoundary = `create or replace workflow M.W parameter $C: M.Ctx
begin
  call microflow M.ACT_Step comment 'Step'
    boundary event interrupting timer 'addHours([%CurrentDateTime%], 2)' {
      call microflow M.ACT_Escalate;
    };
end workflow;`

// Issue #948. A rewrite rebuilds the workflow from the statement, so a stored
// boundary event the script does not restate is deleted along with its handler
// flow — measured 1 -> 0 while exec reported success.
func TestWorkflowRewrite_RefusesDroppingBoundaryEvent(t *testing.T) {
	ctx := storedWorkflowCtx(t, "Workflows$InterruptingTimerBoundaryEvent")
	err := checkNoDroppedWorkflowConstructs(ctx, "wf1", "M.W", parseWorkflowStmt(t, wfNoBoundary))
	if err == nil {
		t.Fatal("a rewrite that drops a stored boundary event was allowed")
	}
	if !strings.Contains(err.Error(), "boundary event") {
		t.Errorf("error should name the construct: %v", err)
	}
}

// Control: boundary events ARE authorable, so restating them is the normal way
// to edit such a workflow and must pass.
func TestWorkflowRewrite_AllowsRestatedBoundaryEvent(t *testing.T) {
	ctx := storedWorkflowCtx(t, "Workflows$InterruptingTimerBoundaryEvent")
	if err := checkNoDroppedWorkflowConstructs(ctx, "wf1", "M.W", parseWorkflowStmt(t, wfWithBoundary)); err != nil {
		t.Errorf("a rewrite that restates the boundary event must be allowed: %v", err)
	}
}

// Control: a workflow with nothing to lose is never blocked.
func TestWorkflowRewrite_AllowsWhenNothingStored(t *testing.T) {
	ctx := storedWorkflowCtx(t)
	if err := checkNoDroppedWorkflowConstructs(ctx, "wf1", "M.W", parseWorkflowStmt(t, wfNoBoundary)); err != nil {
		t.Errorf("rewrite of a workflow with no stored constructs must be allowed: %v", err)
	}
}

// An event sub-process cannot be expressed in MDL at all, so restating is not an
// option and any stored one refuses the rewrite outright.
func TestWorkflowRewrite_RefusesEventSubProcessUnconditionally(t *testing.T) {
	ctx := storedWorkflowCtx(t, "Workflows$InterruptingNotificationEventSubProcessStartActivity")
	for name, src := range map[string]string{"restated": wfWithBoundary, "not restated": wfNoBoundary} {
		err := checkNoDroppedWorkflowConstructs(ctx, "wf1", "M.W", parseWorkflowStmt(t, src))
		if err == nil {
			t.Errorf("%s: a stored event sub-process must refuse the rewrite", name)
		} else if !strings.Contains(err.Error(), "event sub-process") {
			t.Errorf("%s: error should name the construct: %v", name, err)
		}
	}
}

// The guard must not depend on the semantic reader: that reader is what was
// blind here, and one sharing its blind spot cannot see what it protects. All
// three timer variants are matched by substring so a later one is caught too.
func TestWorkflowRewrite_MatchesEveryTimerVariant(t *testing.T) {
	for _, ty := range []string{
		"Workflows$InterruptingTimerBoundaryEvent",
		"Workflows$NonInterruptingTimerBoundaryEvent",
		"Workflows$TimerBoundaryEvent",
		"Workflows$SomeFutureBoundaryEvent",
	} {
		ctx := storedWorkflowCtx(t, ty)
		if err := checkNoDroppedWorkflowConstructs(ctx, "wf1", "M.W", parseWorkflowStmt(t, wfNoBoundary)); err == nil {
			t.Errorf("%s was not detected", ty)
		}
	}
}

// An unreadable stored unit is not this guard's business — the rewrite path
// reports its own errors, and failing here would block edits on a bad read.
func TestWorkflowRewrite_UnreadableUnitDoesNotBlock(t *testing.T) {
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return true }}
	ctx, _ := newMockCtx(t, withBackend(mb))
	if err := checkNoDroppedWorkflowConstructs(ctx, "wf1", "M.W", parseWorkflowStmt(t, wfNoBoundary)); err != nil {
		t.Errorf("an unreadable unit must not block the rewrite: %v", err)
	}
}

// DESCRIBE emits MDL that must re-parse. The call-microflow describer emitted
// boundary events BEFORE outcomes, which the grammar rejects
// (workflowCallMicroflowStmt: … OUTCOMES? BOUNDARY EVENT?) — "mismatched input
// 'outcomes' expecting ';'". It was invisible while the default engine could not
// read boundary events back at all.
func TestWorkflowDescribe_BoundaryEventAfterOutcomesReparses(t *testing.T) {
	src := `create workflow M.W parameter $C: M.Ctx
begin
  call microflow M.ACT_Step comment 'Step'
    outcomes
      DEFAULT -> { }
    boundary event interrupting timer 'addHours([%CurrentDateTime%], 2)' {
      call microflow M.ACT_Escalate;
    };
end workflow;`
	if _, errs := visitor.Build(src); len(errs) > 0 {
		t.Fatalf("outcomes-then-boundary-event must parse: %v", errs)
	}

	// The order DESCRIBE used to emit: the grammar rejects it, which is what
	// made the round trip fail.
	bad := `create workflow M.W parameter $C: M.Ctx
begin
  call microflow M.ACT_Step comment 'Step'
    boundary event interrupting timer 'x' { }
    outcomes
      DEFAULT -> { };
end workflow;`
	if _, errs := visitor.Build(bad); len(errs) == 0 {
		t.Error("boundary-event-then-outcomes should NOT parse; if the grammar now " +
			"accepts both orders this test's premise is stale, but the describer " +
			"should still emit the documented order")
	}
}

// rawWorkflowCtx returns a context whose stored workflow unit is exactly raw.
func rawWorkflowCtx(t *testing.T, raw map[string]any) *ExecContext {
	t.Helper()
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc:  func(model.ID) (map[string]any, error) { return raw, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

// studioProWorkflow is the shape of ako/TestApp's workflow.Workflow1 (Mendix
// 11.14.0, 0 errors under mxbuild), reduced to what a rewrite can lose. Each
// option adds one construct MDL cannot express.
func studioProWorkflow(onCreated, handler, agentTask bool, criteria string, fallbackFirst bool) map[string]any {
	onCreatedEvent := map[string]any{"$Type": "Workflows$NoEvent"}
	if onCreated {
		onCreatedEvent = map[string]any{"$Type": "Workflows$MicroflowBasedEvent", "Microflow": "workflow.UserTaskEventHandle"}
	}
	first := map[string]any{"$ID": "outcome-fast", "$Type": "Workflows$UserTaskOutcome", "Value": "Fast"}
	second := map[string]any{"$ID": "outcome-furious", "$Type": "Workflows$UserTaskOutcome", "Value": "Furious"}
	fallback := "outcome-fast"
	if !fallbackFirst {
		fallback = "outcome-furious"
	}
	activities := []any{
		3,
		map[string]any{"$Type": "Workflows$StartWorkflowActivity", "Name": "start1"},
		map[string]any{"$Type": "Workflows$SingleUserTaskActivity", "Name": "userTask1", "OnCreatedEvent": onCreatedEvent},
		map[string]any{
			"$Type":              "Workflows$MultiUserTaskActivity",
			"Name":               "userTask2",
			"OnCreatedEvent":     map[string]any{"$Type": "Workflows$NoEvent"},
			"Outcomes":           []any{3, first, second},
			"CompletionCriteria": map[string]any{"$Type": "Workflows$" + criteria, "FallbackOutcomePointer": fallback},
		},
	}
	if agentTask {
		activities = append(activities, map[string]any{"$Type": "Workflows$AIAgentTaskActivity", "Name": "aiAgentTask1", "Microflow": "workflow.InvokeAgent"})
	}
	activities = append(activities, map[string]any{"$Type": "Workflows$EndWorkflowActivity", "Name": "end1"})
	events := []any{2}
	if handler {
		events = append(events, map[string]any{
			"$Type":                 "Workflows$WorkflowEventHandler",
			"Description":           "OnAnyEvent",
			"EventTypes":            []any{1, "WorkflowCompleted", "UserTaskStarted"},
			"MicroflowEventHandler": map[string]any{"$Type": "Workflows$MicroflowEventHandler", "Microflow": "workflow.WorkflowEventHandle"},
		})
	}
	return map[string]any{
		"$Type":           "Workflows$Workflow",
		"Flow":            map[string]any{"$Type": "Workflows$Flow", "Activities": activities},
		"OnWorkflowEvent": events,
	}
}

const wfRewriteTwoTasks = `create or modify workflow M.W parameter $C: M.Ctx
begin
  user task userTask1 'User Task' page M.P outcomes 'Good' { } 'Bad' { };
  multi user task userTask2 'Multi' page M.P outcomes 'Fast' { } 'Furious' { };
end workflow;`

// Measured on ako/TestApp: a rebuild writes what the statement says — a user
// task's OnCreatedEvent as NoEvent unless it states `on created microflow`,
// OnWorkflowEvent as only the handlers it states, a multi-user task's completion
// criteria as Consensus on its first outcome — and describe prints an AI agent
// task only as a comment. A rewrite that would lose one of these is refused. The
// statement here restates none of them.
func TestWorkflowRewrite_RefusesDroppingStudioProOnlyState(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{"on-created microflow", studioProWorkflow(true, false, false, "ConsensusCompletionCriteria", true), "workflow.UserTaskEventHandle"},
		{"workflow event handler", studioProWorkflow(false, true, false, "ConsensusCompletionCriteria", true), "workflow.WorkflowEventHandle"},
		{"AI agent task", studioProWorkflow(false, false, true, "ConsensusCompletionCriteria", true), "AI agent task"},
		{"majority completion", studioProWorkflow(false, false, false, "MajorityCompletionCriteria", true), "majority"},
		{"veto completion", studioProWorkflow(false, false, false, "VetoCompletionCriteria", true), "veto"},
		{"consensus falling back to another outcome", studioProWorkflow(false, false, false, "ConsensusCompletionCriteria", false), "Furious"},
	}
	stmt := parseWorkflowStmt(t, wfRewriteTwoTasks)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, c.raw), "wf1", "M.W", stmt)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("rewrite must be refused naming %q, got %v", c.want, err)
			}
		})
	}
}

// The controls: what mxcli writes anyway — NoEvent, no handlers, no agent task,
// Consensus falling back to the first outcome — must not be refused, or every
// rewrite of every multi-user workflow would be.
func TestWorkflowRewrite_AllowsWhatARebuildReproduces(t *testing.T) {
	stmt := parseWorkflowStmt(t, wfRewriteTwoTasks)
	raw := studioProWorkflow(false, false, false, "ConsensusCompletionCriteria", true)
	if err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, raw), "wf1", "M.W", stmt); err != nil {
		t.Fatalf("a rewrite that loses nothing must be allowed, got %v", err)
	}
}

// On-created microflows and event handlers are expressible now, so a statement
// that restates them — the shape `describe workflow` emits — is a rewrite that
// loses nothing, and the guard must let it through. Without this the guard would
// block every rewrite of every workflow that uses them.
func TestWorkflowRewrite_AllowsRestatedHandlers(t *testing.T) {
	stmt := parseWorkflowStmt(t, `create or modify workflow M.W parameter $C: M.Ctx
  on workflow events (WorkflowCompleted, UserTaskStarted) microflow workflow.WorkflowEventHandle as 'OnAnyEvent'
begin
  user task userTask1 'User Task' page M.P on created microflow workflow.UserTaskEventHandle outcomes 'Good' { } 'Bad' { };
  multi user task userTask2 'Multi' page M.P outcomes 'Fast' { } 'Furious' { };
end workflow;`)
	raw := studioProWorkflow(true, true, false, "ConsensusCompletionCriteria", true)
	if err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, raw), "wf1", "M.W", stmt); err != nil {
		t.Fatalf("a rewrite that restates its handlers must be allowed, got %v", err)
	}
}

// A handler subscribed to no event types builds (measured, 11.13.0) but has no
// MDL spelling — the grammar requires at least one type — so restating it is not
// possible and the rewrite is refused however many handlers the statement has.
func TestWorkflowRewrite_RefusesHandlerWithNoEventTypes(t *testing.T) {
	stmt := parseWorkflowStmt(t, `create or modify workflow M.W parameter $C: M.Ctx
  on workflow events (WorkflowCompleted) microflow workflow.WorkflowEventHandle as 'OnAnyEvent'
begin
  user task userTask1 'User Task' page M.P outcomes 'Good' { } 'Bad' { };
end workflow;`)
	raw := studioProWorkflow(false, true, false, "ConsensusCompletionCriteria", true)
	handler := raw["OnWorkflowEvent"].([]any)[1].(map[string]any)
	handler["EventTypes"] = []any{1}
	err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, raw), "wf1", "M.W", stmt)
	if err == nil || !strings.Contains(err.Error(), "no event types") {
		t.Fatalf("a handler with no event types must be refused, got %v", err)
	}
}

// storedReplaceCtx is a project holding M.W: the semantic workflow ALTER resolves
// its target against, and the raw unit the REPLACE refusal reads.
func storedReplaceCtx(t *testing.T, raw map[string]any) *ExecContext {
	t.Helper()
	single := &workflows.UserTask{}
	single.Name = "userTask1"
	multi := &workflows.UserTask{IsMulti: true}
	multi.Name = "userTask2"
	wf := &workflows.Workflow{Name: "W", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{single, multi}}}
	wf.ID = "wf1"
	wf.ContainerID = "mod1"
	mod := &model.Module{Name: "M"}
	mod.ID = "mod1"
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
		GetRawUnitFunc:    func(model.ID) (map[string]any, error) { return raw, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

// Measured on ako/TestApp: REPLACE ACTIVITY with an identical user task reset its
// on-created microflow to NoEvent while exec reported "Altered workflow".
func TestAlterWorkflow_ReplaceRefusesToResetStudioProState(t *testing.T) {
	replaceTask := func(name string) *ast.AlterWorkflowStmt {
		clause := ""
		if strings.HasSuffix(name, "+oncreated") {
			name = strings.TrimSuffix(name, "+oncreated")
			clause = ` on created microflow workflow.UserTaskEventHandle`
		}
		prog, errs := visitor.Build(`alter workflow M.W replace activity ` + name + ` with user task ` + name + ` 'Task' page M.P` + clause + ` outcomes 'Fast' { } 'Furious' { };`)
		if len(errs) > 0 {
			t.Fatalf("parse errors: %v", errs)
		}
		return prog.Statements[0].(*ast.AlterWorkflowStmt)
	}
	cases := []struct {
		name, target string
		raw          map[string]any
		want         string // "" = allowed
	}{
		{"on-created microflow", "userTask1", studioProWorkflow(true, false, false, "ConsensusCompletionCriteria", true), "workflow.UserTaskEventHandle"},
		// The replacement restates it, so nothing is lost.
		{"on-created microflow, restated", "userTask1+oncreated", studioProWorkflow(true, false, false, "ConsensusCompletionCriteria", true), ""},
		{"majority rule", "userTask2", studioProWorkflow(false, false, false, "MajorityCompletionCriteria", true), "majority"},
		// Controls: the same replacements where the stored activity holds only
		// what a rebuild writes anyway.
		{"plain user task", "userTask1", studioProWorkflow(false, false, false, "ConsensusCompletionCriteria", true), ""},
		{"consensus on the first outcome", "userTask2", studioProWorkflow(false, false, false, "ConsensusCompletionCriteria", true), ""},
		// A workflow-level handler is not the replaced activity's to lose.
		{"handler elsewhere in the workflow", "userTask1", studioProWorkflow(false, true, false, "ConsensusCompletionCriteria", true), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := validateAlterReplaceKeepsStudioProState(storedReplaceCtx(t, c.raw), replaceTask(c.target))
			switch {
			case c.want == "" && len(got) > 0:
				t.Errorf("must be allowed, got %v", got)
			case c.want != "" && (len(got) != 1 || !strings.Contains(got[0], c.want)):
				t.Errorf("must be refused naming %q, got %v", c.want, got)
			}
		})
	}
}

// The reference workflow has ONE event sub-process, whose flow starts with an
// InterruptingNotificationEventSubProcessStartActivity. A substring count of
// "EventSubProcess" matched both and reported two.
func TestWorkflowRewrite_CountsAnEventSubProcessOnce(t *testing.T) {
	raw := map[string]any{
		"$Type": "Workflows$Workflow",
		"EventSubProcesses": []any{2, map[string]any{
			"$Type": "Workflows$EventSubProcess",
			"Name":  "eventSubProcess1",
			"Flow": map[string]any{"$Type": "Workflows$Flow", "Activities": []any{3,
				map[string]any{"$Type": "Workflows$InterruptingNotificationEventSubProcessStartActivity", "Name": "start"},
				map[string]any{"$Type": "Workflows$EndWorkflowActivity", "Name": "end2"},
			}},
		}},
	}
	err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, raw), "wf1", "M.W", parseWorkflowStmt(t, wfRewriteTwoTasks))
	if err == nil || !strings.Contains(err.Error(), "has 1 stored event sub-process") {
		t.Fatalf("expected one event sub-process to be reported, got %v", err)
	}
}

// storedWorkflowWithEnds returns a context whose stored workflow has the main
// flow's End plus `nested` Ends inside a user task's outcomes, and one of each
// end-of-path marker, which must not be mistaken for an End.
func storedWorkflowWithEnds(t *testing.T, nested int) *ExecContext {
	t.Helper()
	outcomes := make([]any, 0, nested)
	for i := 0; i < nested; i++ {
		outcomes = append(outcomes, map[string]any{
			"$Type": "Workflows$UserTaskOutcome",
			"Flow": map[string]any{
				"$Type":      "Workflows$Flow",
				"Activities": []any{map[string]any{"$Type": "Workflows$EndWorkflowActivity"}},
			},
		})
	}
	raw := map[string]any{
		"$Type": "Workflows$Workflow",
		"Flow": map[string]any{
			"$Type": "Workflows$Flow",
			"Activities": []any{
				map[string]any{"$Type": "Workflows$StartWorkflowActivity"},
				map[string]any{"$Type": "Workflows$SingleUserTaskActivity", "Outcomes": outcomes},
				map[string]any{"$Type": "Workflows$EndOfParallelSplitPathActivity"},
				map[string]any{"$Type": "Workflows$EndOfBoundaryEventPathActivity"},
				map[string]any{"$Type": "Workflows$EndWorkflowActivity"},
			},
		},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc:  func(model.ID) (map[string]any, error) { return raw, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

// A branch that ended the workflow could not be stated in MDL, and describe
// dropped it, so a rewrite deleted it without a word — and the branch then fell
// through into the main flow. Guard-don't-drop, as for boundary events.
func TestWorkflowRewrite_RefusesDroppingNestedEnd(t *testing.T) {
	stmt := parseWorkflowStmt(t, `create or replace workflow M.W parameter $C: M.Ctx
begin
  user task T 'c' page M.P outcomes 'Reject' { } 'Approve' { };
end workflow;`)
	err := checkNoDroppedWorkflowConstructs(storedWorkflowWithEnds(t, 1), "wf1", "M.W", stmt)
	if err == nil || !strings.Contains(err.Error(), "end workflow") {
		t.Fatalf("a rewrite that drops a stored nested End must be refused, got %v", err)
	}
}

func TestWorkflowRewrite_AllowsRestatedNestedEnd(t *testing.T) {
	stmt := parseWorkflowStmt(t, `create or replace workflow M.W parameter $C: M.Ctx
begin
  user task T 'c' page M.P outcomes 'Reject' { end workflow; } 'Approve' { };
end workflow;`)
	if err := checkNoDroppedWorkflowConstructs(storedWorkflowWithEnds(t, 1), "wf1", "M.W", stmt); err != nil {
		t.Fatalf("a rewrite that restates the nested End must be allowed, got %v", err)
	}
}

// The control for both: the main flow's End and the two end-of-path markers are
// always present and are not branch Ends — counting them would refuse every
// rewrite of every workflow.
func TestWorkflowRewrite_MainEndAndPathMarkersAreNotNestedEnds(t *testing.T) {
	stmt := parseWorkflowStmt(t, `create or replace workflow M.W parameter $C: M.Ctx
begin
  user task T 'c' page M.P outcomes 'Reject' { } 'Approve' { };
end workflow;`)
	if err := checkNoDroppedWorkflowConstructs(storedWorkflowWithEnds(t, 0), "wf1", "M.W", stmt); err != nil {
		t.Fatalf("no nested End is stored, so nothing can be dropped, got %v", err)
	}
}
