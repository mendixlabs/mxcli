// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ako/mxcli#2097 (mendixlabs/mxcli#1798): `create workflow` refuses a user task
// whose only outcome holds activities (MDL-WF02, CE1876), but `alter workflow`
// did not — dropping a stored task's other outcomes, or inserting a task whose
// single outcome holds an activity, passed `check -p` and `exec`, and mx check
// (11.14.0) then reported
//
//	[error] [CE1876] "Single outcome must not contain any activities in its flow." at User task 'Review'
//
// The rule now judges the state the SCRIPT leaves: a later op in the statement
// or a later statement that adds the second outcome silences it, and so does a
// state it cannot establish (no project, task not found).

// singleOutcomeProject serves Sales.WF holding user task Review with outcomes
// Approve { call microflow Notify } and Reject { }.
func singleOutcomeProject(t *testing.T) func() backend.FullBackend {
	t.Helper()
	mod := mkModule("Sales")
	wf := mkWorkflow(mod.ID, "WF")
	notify := &workflows.CallMicroflowTask{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
		BaseElement: model.BaseElement{ID: "a2"}, Name: "Notify", Caption: "Notify"}}
	wf.Flow = &workflows.Flow{Activities: []workflows.WorkflowActivity{
		&workflows.UserTask{
			BaseWorkflowActivity: workflows.BaseWorkflowActivity{
				BaseElement: model.BaseElement{ID: "a1"}, Name: "Review", Caption: "Review"},
			Outcomes: []*workflows.UserTaskOutcome{
				{Value: "Approve", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{notify}}},
				{Value: "Reject"},
			},
		},
	}}
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
	}
	return func() backend.FullBackend { return mb }
}

func singleOutcomeViolations(t *testing.T, src string, project func() backend.FullBackend) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var out []linter.Violation
	for _, v := range validateAlterWorkflowSingleOutcomes(prog, project) {
		if v.RuleID == "MDL-WF02" {
			out = append(out, v)
		}
	}
	return out
}

const wf2097Create = `create workflow Sales.WF2 parameter $WorkflowContext: Sales.Req
begin
  user task Review 'Review' page Sales.P
    outcomes
      'Approve' { call microflow Sales.Notify; }
      'Reject' { };
end workflow;
`

func TestAlterWorkflow_SingleOutcomeWithActivities_2097(t *testing.T) {
	for _, tc := range []struct {
		name      string
		src       string
		project   bool
		want      bool
		wantInMsg string
	}{
		// The reported shape: drop the stored task's other outcome, leaving
		// Approve — which holds Notify — alone.
		{"drop leaves one outcome with activities",
			`alter workflow Sales.WF { drop Review outcome 'Reject' };`, true, true, "'Review'"},
		{"old spelling: drop outcome on",
			`alter workflow Sales.WF drop outcome 'Reject' on Review;`, true, true, "'Review'"},
		// Controls.
		{"drop leaves one EMPTY outcome",
			`alter workflow Sales.WF { drop Review outcome 'Approve' };`, true, false, ""},
		{"a later op in the statement adds the second outcome",
			`alter workflow Sales.WF { drop Review outcome 'Reject'; insert into Review { outcomes 'Escalate' { } } };`, true, false, ""},
		{"a later statement adds the second outcome",
			`alter workflow Sales.WF { drop Review outcome 'Reject' };
			 alter workflow Sales.WF { insert into Review { outcomes 'Escalate' { } } };`, true, false, ""},
		{"a later op empties the remaining outcome",
			`alter workflow Sales.WF { drop Review outcome 'Reject'; drop Notify };`, true, false, ""},
		{"a later op drops the task",
			`alter workflow Sales.WF { drop Review outcome 'Reject'; drop Review };`, true, false, ""},
		{"a later statement drops the workflow",
			`alter workflow Sales.WF { drop Review outcome 'Reject' };
			 drop workflow Sales.WF;`, true, false, ""},
		{"no project: the stored outcomes are unknown",
			`alter workflow Sales.WF { drop Review outcome 'Reject' };`, false, false, ""},
		{"task not found in the stored workflow",
			`alter workflow Sales.WF { drop Missing outcome 'Reject' };`, true, false, ""},
		{"workflow not found in the project",
			`alter workflow Sales.Other { drop Review outcome 'Reject' };`, true, false, ""},

		// An inserted task is decidable from the statement, with or without a project.
		{"inserted task with one outcome holding an activity",
			`alter workflow Sales.WF { insert after Review { user task Verify 'Check' page Sales.P outcomes 'Done' { call microflow Sales.Notify; }; } };`,
			false, true, "'Verify'"},
		{"inserted task, nested in an inserted outcome",
			`alter workflow Sales.WF { insert into Review { outcomes 'Escalate' { user task Verify 'Check' page Sales.P outcomes 'Done' { call microflow Sales.Notify; }; } } };`,
			true, true, "'Verify'"},
		{"inserted task, second outcome added by a later op",
			`alter workflow Sales.WF { insert after Review { user task Verify 'Check' page Sales.P outcomes 'Done' { call microflow Sales.Notify; }; };
			                           insert into Verify { outcomes 'Skip' { } } };`, false, false, ""},
		{"inserted task with one EMPTY outcome",
			`alter workflow Sales.WF { insert after Review { user task Verify 'Check' page Sales.P outcomes 'Done' { }; } };`, false, false, ""},

		// A workflow the script creates is the baseline, so this needs no project.
		{"create then drop the other outcome, no project",
			wf2097Create + `alter workflow Sales.WF2 { drop Review outcome 'Reject' };`, false, true, "'Review'"},
		{"create then drop, then re-add, no project",
			wf2097Create + `alter workflow Sales.WF2 { drop Review outcome 'Reject' };
			alter workflow Sales.WF2 { insert into Review { outcomes 'Later' { } } };`, false, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var project func() backend.FullBackend
			if tc.project {
				project = singleOutcomeProject(t)
			}
			got := singleOutcomeViolations(t, tc.src, project)
			if (len(got) > 0) != tc.want {
				t.Fatalf("MDL-WF02 reported = %v, want %v (%v)", len(got) > 0, tc.want, got)
			}
			if tc.want {
				if len(got) != 1 {
					t.Fatalf("want one MDL-WF02, got %d: %v", len(got), got)
				}
				v := got[0]
				if v.Severity != linter.SeverityError || !strings.Contains(v.Message, "CE1876") || !strings.Contains(v.Message, tc.wantInMsg) {
					t.Fatalf("unexpected violation: %+v", v)
				}
			}
		})
	}
}

// The rule is reached from ValidateProgram, which both `check` and `exec`'s
// preflight run — without a project, for a task the statement inserts.
func TestAlterWorkflow_SingleOutcomeWithActivities_ViaValidateProgram_2097(t *testing.T) {
	prog, errs := visitor.Build(`alter workflow Sales.WF { insert after Review { user task Verify 'Check' page Sales.P outcomes 'Done' { call microflow Sales.Notify; }; } };`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if _, ok := prog.Statements[0].(*ast.AlterWorkflowStmt); !ok {
		t.Fatalf("not an alter workflow: %T", prog.Statements[0])
	}
	n := 0
	for _, v := range ValidateProgram(prog, "") {
		if v.RuleID == "MDL-WF02" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want exactly one MDL-WF02 from ValidateProgram, got %d", n)
	}
}
