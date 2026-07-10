// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

// workflowViolations parses MDL, runs ValidateWorkflow on every workflow
// statement, and returns (ruleID, message) pairs.
func workflowViolations(t *testing.T, src string) [][2]string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var out [][2]string
	for _, stmt := range prog.Statements {
		if wf, ok := stmt.(*ast.CreateWorkflowStmt); ok {
			for _, v := range ValidateWorkflow(wf) {
				out = append(out, [2]string{v.RuleID, v.Message})
			}
		}
	}
	return out
}

func hasRule(vs [][2]string, ruleID string) bool {
	for _, v := range vs {
		if v[0] == ruleID {
			return true
		}
	}
	return false
}

const wfPreamble = `create persistent entity WF.Ctx ( Total: decimal );
create page WF.TaskPage ( title: 'T', layout: Atlas_Core.PopupLayout ) { };
`

// MDL-WF01 — a user task without a page is flagged.
func TestValidateWorkflow_UserTaskWithoutPage(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  user task ReviewOrder 'Review'
    outcomes 'Done' { }
  ;
end workflow;`
	vs := workflowViolations(t, src)
	if !hasRule(vs, "MDL-WF01") {
		t.Fatalf("expected MDL-WF01 for user task without a page, got %v", vs)
	}
}

// A user task WITH a page does not trigger MDL-WF01.
func TestValidateWorkflow_UserTaskWithPageClean(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  user task ReviewOrder 'Review'
    page WF.TaskPage
    outcomes 'Done' { }
  ;
end workflow;`
	if vs := workflowViolations(t, src); hasRule(vs, "MDL-WF01") {
		t.Fatalf("user task with a page should not trigger MDL-WF01, got %v", vs)
	}
}

// MDL-WF02 — a single outcome carrying nested activities is flagged.
func TestValidateWorkflow_SingleOutcomeWithActivities(t *testing.T) {
	src := wfPreamble + `create microflow WF.ACT ($Ctx: WF.Ctx) begin return; end;
create workflow WF.W parameter $Ctx: WF.Ctx
begin
  user task ReviewOrder 'Review'
    page WF.TaskPage
    outcomes 'Done' { call microflow WF.ACT; }
  ;
end workflow;`
	vs := workflowViolations(t, src)
	if !hasRule(vs, "MDL-WF02") {
		t.Fatalf("expected MDL-WF02 for single outcome with activities, got %v", vs)
	}
}

// Two outcomes, one with activities, is allowed (not a single-outcome task).
func TestValidateWorkflow_MultipleOutcomesClean(t *testing.T) {
	src := wfPreamble + `create microflow WF.ACT ($Ctx: WF.Ctx) begin return; end;
create workflow WF.W parameter $Ctx: WF.Ctx
begin
  user task ReviewOrder 'Review'
    page WF.TaskPage
    outcomes
      'Approve' { call microflow WF.ACT; }
      'Reject' { }
  ;
end workflow;`
	if vs := workflowViolations(t, src); hasRule(vs, "MDL-WF02") {
		t.Fatalf("multi-outcome task should not trigger MDL-WF02, got %v", vs)
	}
}

// MDL-WF03 — a decision outcome that is not a valid identifier (has a space) is flagged.
func TestValidateWorkflow_FreeTextDecisionOutcome(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  decision '$Ctx/Total > 1000'
    outcomes
      'Reopened' -> { }
      'Confirmed closed' -> { }
  ;
end workflow;`
	vs := workflowViolations(t, src)
	if !hasRule(vs, "MDL-WF03") {
		t.Fatalf("expected MDL-WF03 for free-text decision outcome, got %v", vs)
	}
	// 'Reopened' is a valid identifier and must NOT be flagged; only 'Confirmed closed'.
	var wf03 int
	for _, v := range vs {
		if v[0] == "MDL-WF03" {
			wf03++
			if !strings.Contains(v[1], "Confirmed closed") {
				t.Errorf("MDL-WF03 should name 'Confirmed closed', got %q", v[1])
			}
		}
	}
	if wf03 != 1 {
		t.Fatalf("expected exactly one MDL-WF03 (only 'Confirmed closed'), got %d in %v", wf03, vs)
	}
}

// A boolean decision (true/false) is clean.
func TestValidateWorkflow_BooleanDecisionClean(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  decision '$Ctx/Total > 1000'
    outcomes
      true -> { }
      false -> { }
  ;
end workflow;`
	if vs := workflowViolations(t, src); hasRule(vs, "MDL-WF03") {
		t.Fatalf("boolean decision should not trigger MDL-WF03, got %v", vs)
	}
}
