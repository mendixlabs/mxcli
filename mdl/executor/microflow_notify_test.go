// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// notifyTestBuilder returns a flow builder whose project holds M.W: every kind of
// element a notify can and cannot reach, on the given Mendix minor of 11.
func notifyTestBuilder(minor int) *flowBuilder {
	base := func(name string) workflows.BaseWorkflowActivity { return workflows.BaseWorkflowActivity{Name: name} }
	flow := func(acts ...workflows.WorkflowActivity) *workflows.Flow { return &workflows.Flow{Activities: acts} }
	wf := &workflows.Workflow{
		Name: "W",
		Flow: flow(
			&workflows.NotificationActivity{BaseWorkflowActivity: base("received")},
			&workflows.WaitForNotificationActivity{BaseWorkflowActivity: base("waitApproval"), BoundaryEvents: []*workflows.BoundaryEvent{
				{EventType: "InterruptingNotification", Name: "withdrawn"},
				{EventType: "InterruptingTimer", Name: "timeout"},
			}},
			&workflows.UserTask{BaseWorkflowActivity: base("review"), Outcomes: []*workflows.UserTaskOutcome{
				{Value: "Go", Flow: flow(&workflows.NotificationActivity{BaseWorkflowActivity: base("nestedReceived")})},
			}},
		),
		EventSubProcesses: []*workflows.EventSubProcess{
			{Name: "ESP_Cancel", Flow: flow(&workflows.EventSubProcessStartActivity{BaseWorkflowActivity: base("cancelStart"), Interrupting: true})},
			{Name: "ESP_Audit", Flow: flow(&workflows.EventSubProcessStartActivity{BaseWorkflowActivity: base("auditStart")})},
			{Name: "ESP_Expire", Flow: flow(&workflows.EventSubProcessStartActivity{BaseWorkflowActivity: base("expireStart"), Interrupting: true, Timer: true})},
		},
	}
	wf.ContainerID = "mod1"
	mb := &mock.MockBackend{
		ListWorkflowsFunc:  func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
		ProjectVersionFunc: func() *types.ProjectVersion { return &types.ProjectVersion{MajorVersion: 11, MinorVersion: minor} },
	}
	h := &ContainerHierarchy{moduleIDs: map[model.ID]bool{"mod1": true}, moduleNames: map[model.ID]string{"mod1": "M"}}
	return &flowBuilder{backend: mb, hierarchy: h}
}

// The statement names an element; the stored target type follows from what the
// element is (ako/TestApp, Studio Pro 11.14), at any depth and in sub-processes.
// An element no notification reaches is refused, naming what it is.
func TestNotifyTarget_ResolvesEachKind(t *testing.T) {
	cases := []struct {
		target, wantType, wantError string
	}{
		{"M.W.received", "Workflows$NotifyNotificationActivityTarget", ""},
		{"M.W.nestedReceived", "Workflows$NotifyNotificationActivityTarget", ""},
		{"M.W.waitApproval", "Workflows$NotifyWaitForNotificationActivityTarget", ""},
		{"M.W.withdrawn", "Workflows$NotifyNotificationBoundaryEventTarget", ""},
		{"M.W.cancelStart", "Workflows$InterruptingNotificationEventSubProcessStartActivityTarget", ""},
		{"M.W.auditStart", "Workflows$NonInterruptingNotificationEventSubProcessStartActivityTarget", ""},
		{"M.W.expireStart", "", "timer-started"},
		{"M.W.timeout", "", "timer boundary event"},
		{"M.W.review", "", "UserTask"},
		{"M.W.nothing", "", "no element named"},
		{"M.Other.received", "", "workflow not found"},
		{"M.W", "", "Module.Workflow.Name"},
	}
	for _, c := range cases {
		t.Run(c.target, func(t *testing.T) {
			fb := notifyTestBuilder(14)
			action := &microflows.NotifyWorkflowAction{WorkflowVariable: "Workflow"}
			fb.setNotifyTarget(action, c.target)
			errs := strings.Join(fb.errors, "\n")
			if c.wantError != "" {
				if !strings.Contains(errs, c.wantError) || action.Target != nil {
					t.Errorf("want an error containing %q and no target, got %q / %+v", c.wantError, errs, action.Target)
				}
				return
			}
			if errs != "" || action.Target == nil || action.Target.TypeName != c.wantType || action.Target.Name != c.target {
				t.Errorf("target = %+v, errors %q; want %s %s", action.Target, errs, c.wantType, c.target)
			}
		})
	}

	fb := notifyTestBuilder(14)
	fb.setNotifyTarget(&microflows.NotifyWorkflowAction{}, "M.W.nothing")
	for _, name := range []string{"auditStart", "cancelStart", "received", "waitApproval", "withdrawn"} {
		if !strings.Contains(strings.Join(fb.errors, ""), name) {
			t.Errorf("the unknown-name error does not list %s: %v", name, fb.errors)
		}
	}
}

// Before Mendix 11.7 the action names its wait for notification directly
// (Activity); there is no NotifyTarget to write.
func TestNotifyTarget_BeforeElevenSevenIsTheActivity(t *testing.T) {
	fb := notifyTestBuilder(6)
	action := &microflows.NotifyWorkflowAction{}
	fb.setNotifyTarget(action, "M.W.waitApproval")
	if action.Activity != "M.W.waitApproval" || action.Target != nil || len(fb.errors) != 0 {
		t.Errorf("11.6 notify = %+v, errors %v", action, fb.errors)
	}
}

// MDL-WF16, measured on the 11.6, 11.10 and 11.13 mxbuilds: a notify with no
// target fails the build (CE0166). Refused at check without a project, and by
// exec, which enforces the rule.
func TestNotifyWorkflow_MDLWF16(t *testing.T) {
	parse := func(body string) *ast.CreateMicroflowStmt {
		prog, errs := visitor.Build("create microflow M.N ($Workflow: System.Workflow)\nbegin\n" + body + "\nend;")
		if len(errs) > 0 {
			t.Fatalf("parse errors: %v", errs)
		}
		return prog.Statements[0].(*ast.CreateMicroflowStmt)
	}
	cases := []struct {
		body string
		want string // the MDL-WF16 message expected; "" = no violation
	}{
		{"notify workflow $Workflow;", "names no target"},
		{"notify workflow $Workflow target M.W;", "must name the element inside its workflow"},
		{"$Notified = notify workflow $Workflow target M.W.received;", ""},
	}
	for _, c := range cases {
		stmt := parse(c.body)
		var msgs []string
		for _, v := range ValidateMicroflow(stmt) {
			if v.RuleID == "MDL-WF16" {
				msgs = append(msgs, v.Message)
			}
		}
		got := strings.Join(msgs, "\n")
		if c.want == "" && got != "" || c.want != "" && (len(msgs) != 1 || !strings.Contains(got, c.want)) {
			t.Errorf("%s: MDL-WF16 = %q, want one violation containing %q", c.body, got, c.want)
		}
		err := validateMicroflowRules(stmt)
		if refused := err != nil && strings.Contains(err.Error(), "MDL-WF16"); refused != (c.want != "") {
			t.Errorf("%s: exec refusal = %v, want refused=%v", c.body, err, c.want != "")
		}
	}
}

// Describe emits the target, so a rewrite from describe output keeps it; it used
// to print a bare `notify workflow $W;`, which the build refuses.
func TestNotifyWorkflow_DescribeRoundTripsTarget(t *testing.T) {
	for _, action := range []*microflows.NotifyWorkflowAction{
		{WorkflowVariable: "Workflow", OutputVariableName: "Notified",
			Target: &microflows.NotifyTarget{TypeName: "Workflows$NotifyNotificationBoundaryEventTarget", Name: "M.W.withdrawn"}},
		{WorkflowVariable: "Workflow", Activity: "M.W.waitApproval"},
	} {
		line := formatAction(nil, action, nil, nil)
		prog, errs := visitor.Build("create microflow M.N ($Workflow: System.Workflow)\nbegin\n" + line + "\nend;")
		if len(errs) > 0 {
			t.Fatalf("%q does not parse: %v", line, errs)
		}
		stmt := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.NotifyWorkflowStmt)
		want := action.Activity
		if action.Target != nil {
			want = action.Target.Name
		}
		if stmt.Target != want || stmt.OutputVariable != action.OutputVariableName {
			t.Errorf("%q re-parsed as %+v, want target %s", line, *stmt, want)
		}
	}
}
