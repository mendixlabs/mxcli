// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

const completionVariants = `create workflow M.W parameter $C: M.Ctx begin
  multi user task consensusTask 'c' page M.P decide by consensus fallback 'Reject' outcomes 'Approve' { } 'Reject' { };
  multi user task majorityHalf 'c' page M.P decide by majority more than half fallback 'Approve' outcomes 'Approve' { } 'Reject' { };
  multi user task majorityMost 'c' page M.P decide by majority most chosen fallback 'Reject' outcomes 'Approve' { } 'Reject' { };
  multi user task thresholdPct 'c' page M.P participants 80 percent decide by threshold 60 percent fallback 'Reject' outcomes 'Approve' { } 'Reject' { };
  multi user task thresholdVotes 'c' page M.P participants 3 decide by threshold 2 votes fallback 'Reject' await all users outcomes 'Approve' { } 'Reject' { };
  multi user task vetoTask 'c' page M.P decide by veto 'Reject' await all users outcomes 'Approve' { } 'Reject' { };
  multi user task microflowTask 'c' page M.P decide by microflow M.Decide outcomes 'Approve' { } 'Reject' { };
end workflow;`

func buildCompletionVariants(t *testing.T) map[string]*workflows.UserTask {
	t.Helper()
	out := map[string]*workflows.UserTask{}
	for _, a := range parseWorkflowStmt(t, completionVariants).Activities {
		task := buildUserTask(a.(*ast.WorkflowUserTaskNode))
		out[task.Name] = task
	}
	return out
}

// The MDL words map onto Studio Pro's stored kinds — measured on ako/TestApp
// (11.14.0): "more than half" is an Absolute majority, "most chosen" Relative;
// a percentage threshold is Relative, a vote count Absolute.
func TestWorkflowCompletion_BuildsStudioProShapes(t *testing.T) {
	tasks := buildCompletionVariants(t)
	want := map[string]*workflows.CompletionCriteria{
		"consensusTask":  {Kind: "Consensus", FallbackOutcome: "Reject"},
		"majorityHalf":   {Kind: "Majority", CompletionType: "Absolute", FallbackOutcome: "Approve"},
		"majorityMost":   {Kind: "Majority", CompletionType: "Relative", FallbackOutcome: "Reject"},
		"thresholdPct":   {Kind: "Threshold", CompletionType: "Relative", Threshold: 60, FallbackOutcome: "Reject"},
		"thresholdVotes": {Kind: "Threshold", CompletionType: "Absolute", Threshold: 2, FallbackOutcome: "Reject"},
		"vetoTask":       {Kind: "Veto", VetoOutcome: "Reject"},
		"microflowTask":  {Kind: "Microflow", Microflow: "M.Decide"},
	}
	for name, w := range want {
		if got := tasks[name].CompletionCriteria; !reflect.DeepEqual(got, w) {
			t.Errorf("%s criteria = %+v, want %+v", name, got, w)
		}
	}
	if got := tasks["thresholdPct"].TargetUserInput; !reflect.DeepEqual(got, &workflows.TargetUserInput{Kind: "Percentage", Percentage: 80}) {
		t.Errorf("thresholdPct participants = %+v", got)
	}
	if got := tasks["thresholdVotes"].TargetUserInput; !reflect.DeepEqual(got, &workflows.TargetUserInput{Kind: "Absolute", Amount: 3}) {
		t.Errorf("thresholdVotes participants = %+v", got)
	}
	if !tasks["vetoTask"].AwaitAllUsers || tasks["consensusTask"].AwaitAllUsers {
		t.Error("await all users not carried")
	}
}

// Describe → re-parse → rebuild reproduces every rule, participant count and
// await flag.
func TestWorkflowCompletion_DescribeRoundTrips(t *testing.T) {
	for name, task := range buildCompletionVariants(t) {
		out := strings.Join(formatSingleActivity(task, "  "), "\n")
		stmt := parseWorkflowStmt(t, "create workflow M.W begin\n"+out+"\nend workflow;")
		again := buildUserTask(stmt.Activities[0].(*ast.WorkflowUserTaskNode))
		if !reflect.DeepEqual(again.CompletionCriteria, task.CompletionCriteria) ||
			!reflect.DeepEqual(again.TargetUserInput, task.TargetUserInput) || again.AwaitAllUsers != task.AwaitAllUsers {
			t.Errorf("%s did not round-trip:\n%s\nrebuilt %+v %+v %v", name, out, again.CompletionCriteria, again.TargetUserInput, again.AwaitAllUsers)
		}
	}
}

// What a rebuild writes anyway is not described, so a multi-user task that never
// had these settings describes exactly as it did before they existed.
func TestWorkflowCompletion_DefaultsAreNotDescribed(t *testing.T) {
	task := &workflows.UserTask{
		IsMulti:            true,
		Outcomes:           []*workflows.UserTaskOutcome{{Value: "Approve"}, {Value: "Reject"}},
		CompletionCriteria: &workflows.CompletionCriteria{Kind: "Consensus", FallbackOutcome: "Approve"},
		TargetUserInput:    &workflows.TargetUserInput{Kind: "All"},
	}
	if lines := formatMultiUserTaskCompletion(task, "  "); len(lines) != 0 {
		t.Errorf("defaults described as %v", lines)
	}
	task.CompletionCriteria.FallbackOutcome = "Reject"
	if lines := formatMultiUserTaskCompletion(task, "  "); len(lines) != 1 || !strings.Contains(lines[0], "decide by consensus fallback 'Reject'") {
		t.Errorf("a consensus falling back to another outcome must be described, got %v", lines)
	}
}

// MDL-WF13, measured on mxbuild 11.13.0: no fallback for consensus, majority or
// threshold is CE1866; a veto without a veto outcome is CE1867. A name that is
// not one of the task's outcomes leaves the pointer empty, so it is the same error.
func TestWorkflowCompletion_MDLWF13(t *testing.T) {
	task := func(clause string) string {
		return `create workflow M.W begin multi user task T 'c' page M.P ` + clause + ` outcomes 'Approve' { } 'Reject' { }; end workflow;`
	}
	cases := []struct{ name, src, want string }{
		{"majority without fallback", task(`decide by majority more than half`), "CE1866"},
		{"consensus without fallback", task(`decide by consensus`), "CE1866"},
		{"threshold without fallback", task(`decide by threshold 60 percent`), "CE1866"},
		{"fallback names no outcome", task(`decide by majority most chosen fallback 'Maybe'`), "CE1866"},
		{"veto names no outcome", task(`decide by veto 'Maybe'`), "CE1867"},
		{"microflow needs no fallback", task(`decide by microflow M.Decide`), ""},
		{"valid veto", task(`decide by veto 'Reject'`), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var msgs []string
			for _, v := range ValidateWorkflow(parseWorkflowStmt(t, c.src)) {
				if v.RuleID == "MDL-WF13" {
					msgs = append(msgs, v.Message)
				}
			}
			got := strings.Join(msgs, "\n")
			if c.want == "" && got != "" {
				t.Errorf("expected no MDL-WF13, got %s", got)
			}
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Errorf("expected MDL-WF13 with %s, got %q", c.want, got)
			}
		})
	}
	var all []string
	for _, v := range ValidateWorkflow(parseWorkflowStmt(t, completionVariants)) {
		if v.RuleID == "MDL-WF13" {
			all = append(all, v.Message)
		}
	}
	if len(all) != 0 {
		t.Errorf("every valid variant must pass MDL-WF13, got %v", all)
	}
}

// A decision microflow must return String (CE5012); its parameters are free
// (measured: none, the context entity and System.WorkflowUserTask all build).
func TestWorkflowCompletion_DecisionMicroflow(t *testing.T) {
	wf := func(mf string) string {
		return `create workflow M.W parameter $C: M.Ctx begin multi user task T 'c' decide by microflow ` + mf + ` outcomes 'Approve' { } 'Reject' { }; end workflow;`
	}
	cases := []struct{ name, src, want, notWant string }{
		{"returns Boolean", "create microflow M.DecideBool () returns boolean as $b begin return true; end;\n" + wf("M.DecideBool"), "CE5012", ""},
		{"returns String", "create microflow M.DecideString ( $C: M.Ctx ) returns string as $s begin return 'Approve'; end;\n" + wf("M.DecideString"), "", "CE5012"},
		{"unknown microflow", wf("M.Nope"), "microflow not found: M.Nope", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := checkScript(t, wfHandlerCtx(t, 11, 13, 0), c.src)
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Errorf("expected %q, got:\n%s", c.want, got)
			}
			if c.notWant != "" && strings.Contains(got, c.notWant) {
				t.Errorf("did not expect %q, got:\n%s", c.notWant, got)
			}
		})
	}
}

// storedMultiUserSettings is the Studio Pro workflow fixture with userTask2
// deciding by an Absolute majority, needing 80 percent and waiting for all users.
func storedMultiUserSettings() map[string]any {
	raw := studioProWorkflow(false, false, false, "MajorityCompletionCriteria", true)
	for _, a := range raw["Flow"].(map[string]any)["Activities"].([]any) {
		if m, ok := a.(map[string]any); ok && m["Name"] == "userTask2" {
			m["TargetUserInput"] = map[string]any{"$Type": "Workflows$PercentageAmountUserInput", "Percentage": 80}
			m["AwaitAllUsers"] = true
		}
	}
	return raw
}

// A rewrite writes what the statement says; each omitted clause means its default.
// Participants and await were not guarded at all before this: a rewrite reset
// both without a word.
func TestWorkflowRewrite_MultiUserSettingsMustBeRestated(t *testing.T) {
	stmt := func(clauses string) *ast.CreateWorkflowStmt {
		return parseWorkflowStmt(t, `create or modify workflow M.W parameter $C: M.Ctx
begin
  user task userTask1 'User Task' page M.P outcomes 'Good' { } 'Bad' { };
  multi user task userTask2 'Multi' page M.P `+clauses+` outcomes 'Fast' { } 'Furious' { };
end workflow;`)
	}
	cases := []struct{ name, clauses, want string }{
		{"nothing restated", ``, "decides by majority"},
		{"rule only", `decide by majority more than half fallback 'Fast'`, "80 percent"},
		{"rule and participants", `participants 80 percent decide by majority more than half fallback 'Fast'`, "waits for all users"},
		{"everything restated", `participants 80 percent decide by majority more than half fallback 'Fast' await all users`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, storedMultiUserSettings()), "wf1", "M.W", stmt(c.clauses))
			switch {
			case c.want == "" && err != nil:
				t.Errorf("a rewrite restating everything must be allowed, got %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Errorf("must be refused naming %q, got %v", c.want, err)
			}
		})
	}
}

func TestAlterWorkflow_ReplaceRestatesMultiUserSettings(t *testing.T) {
	replace := func(clauses string) *ast.AlterWorkflowStmt {
		prog, errs := visitor.Build(`alter workflow M.W replace activity userTask2 with multi user task userTask2 'Multi' page M.P ` + clauses + ` outcomes 'Fast' { } 'Furious' { };`)
		if len(errs) > 0 {
			t.Fatalf("parse errors: %v", errs)
		}
		return prog.Statements[0].(*ast.AlterWorkflowStmt)
	}
	ctx := storedReplaceCtx(t, storedMultiUserSettings())
	if got := validateAlterReplaceKeepsStudioProState(ctx, replace(`participants 80 percent decide by majority more than half fallback 'Fast' await all users`)); len(got) != 0 {
		t.Errorf("a replacement restating everything must be allowed, got %v", got)
	}
	got := validateAlterReplaceKeepsStudioProState(ctx, replace(``))
	if len(got) != 1 {
		t.Fatalf("want one refusal, got %v", got)
	}
	for _, want := range []string{"majority", "80 percent", "waits for all users"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("refusal lacks %q: %s", want, got[0])
		}
	}
}
