// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// singleOutcomeFindings runs the ALTER half of MDL-WF02 over a script, against
// the stored workflow when stored is not nil.
func singleOutcomeFindings(t *testing.T, src string, stored *workflows.Workflow) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var project func() backend.FullBackend
	if stored != nil {
		stored.ContainerID = "mod1"
		mod := &model.Module{Name: "WF"}
		mod.ID = "mod1"
		mb := &mock.MockBackend{
			IsConnectedFunc:   func() bool { return true },
			ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
			ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{stored}, nil },
		}
		project = func() backend.FullBackend { return mb }
	}
	return validateAlterWorkflowSingleOutcomes(prog, project)
}

// An ALTER that inserts a user task whose one outcome holds a flow builds
// CE1876 exactly as the same task in a CREATE does; a second outcome added
// later — in the same statement or a later one — repairs it, so only the state
// the script ends in is judged.
func TestAlterWorkflowSingleOutcome_InsertedTask(t *testing.T) {
	const task = `user task Approve 'Approve' page WF.TaskPage outcomes 'Done' { call microflow WF.ACT; };`
	cases := []struct {
		name, src string
		want      bool
	}{
		{"single outcome with an activity", `alter workflow WF.W { insert after Review { ` + task + ` } };`, true},
		{"single outcome holding only end", `alter workflow WF.W { insert after Review { user task Approve 'Approve' page WF.TaskPage outcomes 'Done' { end workflow; }; } };`, true},
		{"second outcome in the same statement", `alter workflow WF.W { insert after Review { ` + task + ` } insert into Approve { outcomes 'Reject' { } } };`, false},
		{"second outcome in a later statement", `alter workflow WF.W { insert after Review { ` + task + ` } };
alter workflow WF.W { insert into Approve { outcomes 'Reject' { } } };`, false},
		{"the activity after the task instead", `alter workflow WF.W { insert after Review { user task Approve 'Approve' page WF.TaskPage outcomes 'Done' { }; call microflow WF.ACT; } };`, false},
		{"two outcomes from the start", `alter workflow WF.W { insert after Review { user task Approve 'Approve' page WF.TaskPage outcomes 'Done' { call microflow WF.ACT; } 'Reject' { }; } };`, false},
		// Without the stored workflow, a later operation on the inserted task
		// may address a stored activity of that name: not judged.
		{"later operation on the task", `alter workflow WF.W { insert after Review { ` + task + ` } };
alter workflow WF.W { drop Approve outcome 'Done'; };`, false},
		{"a later property change on the task", `alter workflow WF.W { insert after Review { ` + task + ` } };
alter workflow WF.W { set (Page: WF.TaskPage) on Approve; };`, true},
		{"nested in an inserted path", `alter workflow WF.W { insert into Split1 { path { ` + task + ` } } };`, true},
		{"a later drop removes it", `alter workflow WF.W { insert after Review { ` + task + ` } };
drop workflow WF.W;`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs := singleOutcomeFindings(t, c.src, nil)
			if got := len(vs) > 0; got != c.want {
				t.Fatalf("reported = %v, want %v: %v", got, c.want, vs)
			}
			if c.want && (vs[0].RuleID != "MDL-WF02" || !strings.Contains(vs[0].Message, "'Approve'") || !strings.Contains(vs[0].Message, "CE1876")) {
				t.Errorf("want MDL-WF02 naming 'Approve' and CE1876, got %v", vs[0])
			}
		})
	}
}

// storedTwoOutcomeWorkflow is WF.W holding user task Review with outcomes
// 'Approve' (calling a microflow when withActivity) and 'Reject' (empty).
func storedTwoOutcomeWorkflow(withActivity bool) *workflows.Workflow {
	approve := &workflows.UserTaskOutcome{Value: "Approve"}
	if withActivity {
		call := &workflows.CallMicroflowTask{}
		call.Name = "Notify"
		approve.Flow = &workflows.Flow{Activities: []workflows.WorkflowActivity{call}}
	}
	review := &workflows.UserTask{Outcomes: []*workflows.UserTaskOutcome{approve, {Value: "Reject"}}}
	review.Name, review.Caption = "Review", "Review the order"
	return &workflows.Workflow{Name: "W", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{review}}}
}

// Whether dropping an outcome leaves a single outcome holding activities is
// only known from the outcomes the task already has: the stored workflow, or
// the script's own CREATE.
func TestAlterWorkflowSingleOutcome_StoredOutcomes(t *testing.T) {
	cases := []struct {
		name, src    string
		withActivity bool
		want         bool
	}{
		{"drop the empty outcome", `alter workflow WF.W { drop Review outcome 'Reject'; };`, true, true},
		{"addressed by caption", `alter workflow WF.W { drop 'Review the order' outcome 'Reject'; };`, true, true},
		{"drop the outcome holding the activity", `alter workflow WF.W { drop Review outcome 'Approve'; };`, true, false},
		{"drop it and add another", `alter workflow WF.W { drop Review outcome 'Reject'; insert into Review { outcomes 'Hold' { } } };`, true, false},
		{"add another in a later statement", `alter workflow WF.W { drop Review outcome 'Reject'; };
alter workflow WF.W { insert into Review { outcomes 'Hold' { } } };`, true, false},
		{"drop the activity too", `alter workflow WF.W { drop Review outcome 'Reject'; drop Notify };`, true, false},
		{"empty outcomes, activity after the task", `alter workflow WF.W { drop Review outcome 'Reject'; insert after Review { call microflow WF.ACT; } };`, false, false},
		{"insert into the remaining outcome", `alter workflow WF.W { drop Review outcome 'Reject'; insert before Notify { call microflow WF.ACT; } };`, true, true},
		{"an unrelated alter", `alter workflow WF.W { insert after Review { call microflow WF.ACT; } };`, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs := singleOutcomeFindings(t, c.src, storedTwoOutcomeWorkflow(c.withActivity))
			if got := len(vs) > 0; got != c.want {
				t.Fatalf("reported = %v, want %v: %v", got, c.want, vs)
			}
			if c.want && !strings.Contains(vs[0].Message, "'Review'") {
				t.Errorf("want the finding to name 'Review', got %v", vs[0])
			}
		})
	}
	// The same drop without a project is not judged: the outcomes are unknown.
	if vs := singleOutcomeFindings(t, `alter workflow WF.W { drop Review outcome 'Reject'; };`, nil); len(vs) > 0 {
		t.Errorf("without a project nothing is known about Review's outcomes, got %v", vs)
	}
}

// A task the stored workflow or the script's CREATE already breaks is that
// document's finding, not the ALTER's: reporting it again on every unrelated
// alter would refuse scripts that do not touch it.
func TestAlterWorkflowSingleOutcome_AlreadyBrokenIsNotReported(t *testing.T) {
	broken := storedTwoOutcomeWorkflow(true)
	broken.Flow.Activities[0].(*workflows.UserTask).Outcomes = broken.Flow.Activities[0].(*workflows.UserTask).Outcomes[:1]
	if vs := singleOutcomeFindings(t, `alter workflow WF.W { insert after Review { call microflow WF.ACT; } };`, broken); len(vs) > 0 {
		t.Errorf("stored task was already broken, got %v", vs)
	}

	created := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  user task Review 'Review' page WF.TaskPage
    outcomes 'Approve' { call microflow WF.ACT; } 'Reject' { }
  ;
end workflow;
`
	if vs := singleOutcomeFindings(t, created+`alter workflow WF.W { drop Review outcome 'Reject'; };`, nil); len(vs) != 1 {
		t.Errorf("dropping the created task's empty outcome: want one finding, got %v", vs)
	}
	if vs := singleOutcomeFindings(t, created+`alter workflow WF.W { insert after Review { call microflow WF.ACT; } };`, nil); len(vs) > 0 {
		t.Errorf("an alter that leaves the created task alone: want none, got %v", vs)
	}
}

// ALTER and CREATE report MDL-WF02 in the same words.
func TestAlterWorkflowSingleOutcome_SameWordingAsCreate(t *testing.T) {
	create := workflowViolations(t, wfPreamble+`create workflow WF.W parameter $Ctx: WF.Ctx
begin
  user task Approve 'Approve' page WF.TaskPage outcomes 'Done' { call microflow WF.ACT; };
end workflow;`)
	alter := singleOutcomeFindings(t, `alter workflow WF.W { insert after Review { user task Approve 'Approve' page WF.TaskPage outcomes 'Done' { call microflow WF.ACT; }; } };`, nil)
	var createMsg string
	for _, v := range create {
		if v[0] == "MDL-WF02" {
			createMsg = v[1]
		}
	}
	if len(alter) != 1 || createMsg == "" || alter[0].Message != createMsg {
		t.Errorf("wording differs:\ncreate: %q\nalter:  %v", createMsg, alter)
	}
}
