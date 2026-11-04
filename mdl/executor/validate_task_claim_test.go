// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance-2: completing an unassigned user task fails only at
// RUNTIME ("You can't complete this user task, it is not assigned to you"), the
// button appears to do nothing, and both checkers pass. About eight minutes of
// runtime archaeology.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func taskClaimWarnings(t *testing.T, body string) []string {
	t.Helper()
	src := "create microflow M.ACT ( $Task: System.WorkflowUserTask )\nbegin\n" + body + "\nend;"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var msgs []string
	for _, v := range ValidateTaskClaimsIn(prog, nil) {
		msgs = append(msgs, v.Message)
	}
	return msgs
}

func TestOutcomeWithoutAClaimIsWarned(t *testing.T) {
	got := taskClaimWarnings(t, "  set task outcome $Task 'Plan';")
	if len(got) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(got), got)
	}
	// The runtime message is the thing someone will search for once it bites.
	if !strings.Contains(got[0], "not assigned to you") {
		t.Errorf("warning does not quote the runtime failure:\n%s", got[0])
	}
}

func TestClaimBeforeOutcomeIsClean(t *testing.T) {
	// The control. This is the shape the report ended up with, and a rule that
	// flagged it would be telling people their working code is broken.
	got := taskClaimWarnings(t, `  change $Task (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
  commit $Task;
  set task outcome $Task 'Plan';`)
	if len(got) != 0 {
		t.Errorf("correct claim-then-complete was warned: %v", got)
	}
}

func TestClaimAfterTheOutcomeStillWarns(t *testing.T) {
	// Order is the whole point — claiming afterwards does not make the completion
	// work, so collecting all claims first and then checking would pass this.
	got := taskClaimWarnings(t, `  set task outcome $Task 'Plan';
  change $Task (System.WorkflowUserTask_Assignees = [%CurrentUser%]);`)
	if len(got) != 1 {
		t.Errorf("a claim written after the outcome was accepted: %v", got)
	}
}

func TestClaimOnADifferentTaskDoesNotCount(t *testing.T) {
	got := taskClaimWarnings(t, `  change $Other (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
  set task outcome $Task 'Plan';`)
	if len(got) != 1 {
		t.Errorf("a claim on another variable satisfied the check: %v", got)
	}
}

func TestClaimInsideABranchCounts(t *testing.T) {
	// A warning must fail quiet. The claim is conditional here and this rule
	// cannot evaluate the condition, so it accepts it rather than reporting a
	// construct that may well be correct.
	got := taskClaimWarnings(t, `  if $Task != empty then
    change $Task (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
  end if;
  set task outcome $Task 'Plan';`)
	if len(got) != 0 {
		t.Errorf("a claim inside a branch was not counted: %v", got)
	}
}

func TestAssigneesSpellingsAreAllRecognised(t *testing.T) {
	// The association parses qualified, bare and quoted, and all three reach the
	// model as the same write. Matching only one spelling would warn about code
	// that is already correct.
	for _, spelling := range []string{
		`System.WorkflowUserTask_Assignees`,
		`System."WorkflowUserTask_Assignees"`,
		`WorkflowUserTask_Assignees`,
	} {
		body := "  change $Task (" + spelling + " = [%CurrentUser%]);\n  set task outcome $Task 'Plan';"
		if got := taskClaimWarnings(t, body); len(got) != 0 {
			t.Errorf("spelling %s was not recognised as a claim: %v", spelling, got)
		}
	}
}

// ako/mxcli#943: the claim made in a called sub-microflow.

func taskClaimWarningsIn(t *testing.T, src string, stored storedClaimSource) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var msgs []string
	for _, v := range validateTaskClaims(prog, stored, nil) {
		msgs = append(msgs, v.Message)
	}
	return msgs
}

const claimingSub = `create microflow M.SUB_Claim ( $T: System.WorkflowUserTask )
begin
  change $T (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
  commit $T;
end;
`

const nonClaimingSub = `create microflow M.SUB_Claim ( $T: System.WorkflowUserTask )
begin
  log info 'nothing';
end;
`

const callerOfSub = `create microflow M.ACT ( $Task: System.WorkflowUserTask )
begin
  call microflow M.SUB_Claim(T = $Task);
  set task outcome $Task 'Plan';
end;
`

func TestClaimInScriptCalleeCounts(t *testing.T) {
	if got := taskClaimWarningsIn(t, claimingSub+callerOfSub, nil); len(got) != 0 {
		t.Errorf("a claim in a called microflow was not counted: %v", got)
	}
}

func TestScriptCalleeThatDoesNotClaimStillWarns(t *testing.T) {
	// The control: the callee is read, not just assumed to claim.
	if got := taskClaimWarningsIn(t, nonClaimingSub+callerOfSub, nil); len(got) != 1 {
		t.Errorf("got %d warnings, want 1 for a callee that does not claim: %v", len(got), got)
	}
}

func TestCalleeClaimingAnotherParameterStillWarns(t *testing.T) {
	src := `create microflow M.SUB_Claim ( $T: System.WorkflowUserTask, $Other: System.WorkflowUserTask )
begin
  change $Other (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
end;
` + callerOfSub
	if got := taskClaimWarningsIn(t, src, nil); len(got) != 1 {
		t.Errorf("a claim on a different callee parameter counted: %v", got)
	}
}

func TestUnresolvableCalleeIsAPossibleClaim(t *testing.T) {
	if got := taskClaimWarningsIn(t, callerOfSub, nil); len(got) != 0 {
		t.Errorf("a call to an unknown microflow passing the task was warned: %v", got)
	}
}

func TestNestedScriptCalleesAndRecursion(t *testing.T) {
	src := `create microflow M.SUB_Inner ( $X: System.WorkflowUserTask )
begin
  change $X (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
end;
create microflow M.SUB_Claim ( $T: System.WorkflowUserTask )
begin
  call microflow M.SUB_Claim(T = $T);
  call microflow M.SUB_Inner(X = $T);
end;
` + callerOfSub
	if got := taskClaimWarningsIn(t, src, nil); len(got) != 0 {
		t.Errorf("a claim two calls deep was not counted: %v", got)
	}
	loop := `create microflow M.SUB_Claim ( $T: System.WorkflowUserTask )
begin
  call microflow M.SUB_Claim(T = $T);
end;
` + callerOfSub
	if got := taskClaimWarningsIn(t, loop, nil); len(got) != 1 {
		t.Errorf("a self-recursive callee that never claims: got %v, want 1 warning", got)
	}
}

type fakeStoredClaims map[string]bool // "flow.param" -> claims

func (f fakeStoredClaims) TaskParameterClaims(flow, param string) (bool, [][2]string, bool) {
	c, ok := f[flow+"."+param]
	return c, nil, ok
}

func TestStoredCalleeIsRead(t *testing.T) {
	if got := taskClaimWarningsIn(t, callerOfSub, fakeStoredClaims{"M.SUB_Claim.T": true}); len(got) != 0 {
		t.Errorf("a stored callee that claims was warned: %v", got)
	}
	if got := taskClaimWarningsIn(t, callerOfSub, fakeStoredClaims{"M.SUB_Claim.T": false}); len(got) != 1 {
		t.Errorf("a stored callee that does not claim: got %v, want 1 warning", got)
	}
}

// ako/mxcli#1810: on Mendix 9 a claim writes WorkflowUserTask_Assignee (the
// association is renamed Assignees in 10.0). It must count as a claim, and the
// suggestion must be the spelling that builds on the project's version —
// Assignees is CE1613 on 9.24.
func TestTaskClaim_NineXAssigneeClaims(t *testing.T) {
	got := taskClaimWarnings(t, `  change $Task (System.WorkflowUserTask_Assignee = [%CurrentUser%]);
  commit $Task;
  set task outcome $Task 'Approve';`)
	if len(got) != 0 {
		t.Errorf("a 9.x claim through WorkflowUserTask_Assignee must count, got %v", got)
	}
}

func TestTaskClaim_SuggestionFollowsVersion(t *testing.T) {
	nine := taskClaimSuggestion("T", func(maj, min int) bool { return 9 > maj || (9 == maj && 24 >= min) })
	if !strings.Contains(nine, "WorkflowUserTask_Assignee =") || strings.Contains(nine, "WorkflowUserTask_Assignees =") {
		t.Errorf("9.24 suggestion must use Assignee only:\n%s", nine)
	}
	eleven := taskClaimSuggestion("T", func(maj, min int) bool { return 11 > maj || (11 == maj && 12 >= min) })
	if !strings.Contains(eleven, "WorkflowUserTask_Assignees =") || strings.Contains(eleven, "WorkflowUserTask_Assignee =") {
		t.Errorf("11.12 suggestion must use Assignees only:\n%s", eleven)
	}
	unknown := taskClaimSuggestion("T", nil)
	if !strings.Contains(unknown, "Assignees =") || !strings.Contains(unknown, "Assignee =") || !strings.Contains(unknown, "9.x") {
		t.Errorf("unknown version must give both spellings with their versions:\n%s", unknown)
	}
}
