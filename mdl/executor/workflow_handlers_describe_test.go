// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

func versionCtx(t *testing.T, major, minor, patch int) *ExecContext {
	t.Helper()
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{MajorVersion: major, MinorVersion: minor, PatchVersion: patch}
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

// Describe output has to parse back into the same handlers — a describe that
// does not re-execute is how #466's boundary events went unnoticed.
func TestWorkflowHandlers_DescribeRoundTrips(t *testing.T) {
	all1113, _, _ := allWorkflowEventTypes(11, 13, 0)
	all1110, _, _ := allWorkflowEventTypes(11, 10, 0)
	handlers := []*workflows.WorkflowEventHandler{
		{Description: "OnAnyEvent", EventTypes: all1113, Microflow: "M.Log"},
		{Description: "Task audit", EventTypes: []string{"UserTaskStarted", "UserTaskEnded"}, Microflow: "M.Audit"},
		{EventTypes: all1110, Microflow: "M.Older"}, // a subset at 11.13: written out
	}
	ctx := versionCtx(t, 11, 13, 0)
	lines := formatWorkflowEventHandlers(ctx, handlers)
	out := strings.Join(lines, "\n")

	for _, want := range []string{
		"  on any workflow event microflow M.Log as 'OnAnyEvent'",
		"  on workflow events (UserTaskStarted, UserTaskEnded) microflow M.Audit as 'Task audit'",
		"  ) microflow M.Older",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("describe lacks %q:\n%s", want, out)
		}
	}

	stmt := parseWorkflowStmt(t, "create workflow M.W\n"+out+"\nbegin\nend workflow;")
	if len(stmt.EventHandlers) != 3 {
		t.Fatalf("re-parsed %d handlers, want 3:\n%s", len(stmt.EventHandlers), out)
	}
	rebuilt, err := buildWorkflowEventHandlers(ctx, stmt.EventHandlers, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, h := range handlers {
		r := rebuilt[i]
		if r.Description != h.Description || r.Microflow != h.Microflow || !reflect.DeepEqual(r.EventTypes, h.EventTypes) {
			t.Errorf("handler %d: rebuilt %+v, described %+v", i, r, h)
		}
	}
}

// The same 36-type list is "any" at 11.10 and not at 11.13; describe follows the
// project version so a re-execute stores the same list.
func TestWorkflowHandlers_DescribeAnyFollowsProjectVersion(t *testing.T) {
	all1110, _, _ := allWorkflowEventTypes(11, 10, 0)
	h := []*workflows.WorkflowEventHandler{{EventTypes: all1110, Microflow: "M.Log"}}
	if got := strings.Join(formatWorkflowEventHandlers(versionCtx(t, 11, 10, 0), h), "\n"); got != "  on any workflow event microflow M.Log" {
		t.Errorf("at 11.10: %q", got)
	}
	if got := strings.Join(formatWorkflowEventHandlers(versionCtx(t, 11, 13, 0), h), "\n"); strings.Contains(got, "any") {
		t.Errorf("at 11.13 the 11.10 list is not every type: %q", got)
	}
}

// An empty list has no MDL spelling; describe must not emit a clause that parses
// into something else.
func TestWorkflowHandlers_DescribeEmptyTypeListIsAComment(t *testing.T) {
	got := formatWorkflowEventHandlers(versionCtx(t, 11, 13, 0), []*workflows.WorkflowEventHandler{{Description: "Idle", Microflow: "M.Log"}})
	if len(got) != 1 || !strings.HasPrefix(strings.TrimSpace(got[0]), "--") {
		t.Errorf("describe = %v, want one comment line", got)
	}
}

func TestWorkflowHandlers_DescribeOnCreated(t *testing.T) {
	task := &workflows.UserTask{
		Page:       "M.TaskPage",
		UserSource: &workflows.MicroflowBasedUserSource{Microflow: "M.Targets"},
		OnCreated:  "M.Assign",
		Outcomes:   []*workflows.UserTaskOutcome{{Value: "Done"}},
	}
	task.Name = "Review"
	task.Caption = "Review"
	out := strings.Join(formatUserTask(task, "  "), "\n")
	targeting := strings.Index(out, "targeting users microflow M.Targets")
	onCreated := strings.Index(out, "on created microflow M.Assign")
	outcomes := strings.Index(out, "outcomes")
	if targeting < 0 || onCreated < targeting || outcomes < onCreated {
		t.Fatalf("clauses out of grammar order:\n%s", out)
	}
	stmt := parseWorkflowStmt(t, "create workflow M.W\nbegin\n"+out+";\nend workflow;")
	ut, ok := stmt.Activities[0].(*ast.WorkflowUserTaskNode)
	if !ok || ut.OnCreated.String() != "M.Assign" || ut.Targeting.Microflow.String() != "M.Targets" {
		t.Errorf("re-parsed task = %+v", stmt.Activities[0])
	}
}

// A named list is written in Studio Pro's order whatever order it was typed in,
// and a handler's documentation — which MDL cannot write — is carried from the
// stored handler with the same microflow and description.
func TestWorkflowHandlers_BuildOrdersAndCarriesDocumentation(t *testing.T) {
	nodes := []ast.WorkflowEventHandlerNode{{
		EventTypes:  []string{"usertaskended", "WorkflowCompleted", "UserTaskEnded"},
		Microflow:   ast.QualifiedName{Module: "M", Name: "Audit"},
		Description: "Task audit",
	}}
	stored := []*workflows.WorkflowEventHandler{{Description: "Task audit", Microflow: "M.Audit", Documentation: "Why we audit"}}
	got, err := buildWorkflowEventHandlers(versionCtx(t, 11, 13, 0), nodes, stored)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"WorkflowCompleted", "UserTaskEnded"}; !reflect.DeepEqual(got[0].EventTypes, want) {
		t.Errorf("types = %v, want %v", got[0].EventTypes, want)
	}
	if got[0].Documentation != "Why we audit" {
		t.Errorf("documentation = %q, want it carried", got[0].Documentation)
	}
}

func TestWorkflowHandlers_BuildAnyBelowMeasuredVersionIsRefused(t *testing.T) {
	nodes := []ast.WorkflowEventHandlerNode{{AnyEvent: true, Microflow: ast.QualifiedName{Module: "M", Name: "Log"}}}
	if _, err := buildWorkflowEventHandlers(versionCtx(t, 10, 24, 0), nodes, nil); err == nil {
		t.Error("`on any workflow event` below 11.6 must be refused, not guessed")
	}
}
