// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

func wfRuleViolations(vs []linter.Violation, rule string) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == rule {
			out = append(out, v)
		}
	}
	return out
}

const wfTemplateHead = "create workflow M.W parameter $WorkflowContext: M.Item "

// MDL-WF21 (ako/mxcli#1714). Each flagged row was measured with mx check on
// 11.15.0 — CE0720, or CE0716 for {0} — and each clean row builds.
func TestWorkflowTemplates_PlaceholdersAndParameters(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"placeholder beyond the parameters (CE0720)",
			wfTemplateHead + "begin user task R 'R' task name 'Review {1} {2}' with ({1} = $WorkflowContext/Name) outcomes 'Done' { }; end workflow;",
			"CE0720"},
		{"a placeholder with no parameters at all (CE0720)",
			wfTemplateHead + "display 'Doc {1}' begin user task R 'R' outcomes 'Done' { }; end workflow;",
			"{1} but has 0 parameter(s)"},
		{"a caption that is the task name (CE0720)",
			wfTemplateHead + "begin user task R 'Review {1}' outcomes 'Done' { }; end workflow;",
			"task name of user task"},
		{"placeholder {0} (CE0716)",
			wfTemplateHead + "begin user task R 'R' description 'Due {0}' with ({1} = $WorkflowContext/Name) outcomes 'Done' { }; end workflow;",
			"CE0716"},
		{"parameters numbered with a gap",
			wfTemplateHead + "description 'A {1} B {2}' with ({1} = $WorkflowContext/Name, {3} = $WorkflowContext/Name) begin user task R 'R' outcomes 'Done' { }; end workflow;",
			"{1} to {2}, each once"},
		{"in a nested task",
			wfTemplateHead + "begin decision d1 $WorkflowContext/Ok outcomes true -> { user task R 'R' task name 'X {2}' with ({1} = $WorkflowContext/Name) outcomes 'Done' { }; } false -> { }; end workflow;",
			"{2} but has 1"},
		// --- clean ---
		{"filled", wfTemplateHead + "display 'Doc {1}' with ({1} = $WorkflowContext/Name) begin user task R 'R' task name 'Review {1} {1}' with ({1} = $WorkflowContext/Name) outcomes 'Done' { }; end workflow;", ""},
		{"an unused parameter", wfTemplateHead + "begin user task R 'R' description 'Due' with ({1} = $WorkflowContext/Name) outcomes 'Done' { }; end workflow;", ""},
		{"no templates", wfTemplateHead + "display 'Doc' begin user task R 'R' outcomes 'Done' { }; end workflow;", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs := wfRuleViolations(ValidateWorkflow(parseWorkflowStmt(t, c.src)), wfTemplateRule)
			switch {
			case c.want == "" && len(vs) > 0:
				t.Errorf("reported a filled template: %v", vs)
			case c.want != "" && (len(vs) == 0 || !strings.Contains(vs[0].Message+vs[0].Suggestion, c.want)):
				t.Errorf("want a violation naming %q, got %v", c.want, vs)
			}
		})
	}
}

// What a parameter may name, measured with mx check on 11.15.0: the context
// everywhere; $WorkflowInstance in a task's name and description but CE0117 in
// the workflow's display and description; $WorkflowUserTask and $currentUser
// CE0117 in all four (MDL-WF19).
func TestWorkflowTemplates_ExpressionScope(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"instance in the display", wfTemplateHead + "display 'D {1}' with ({1} = $WorkflowInstance/Name) begin user task R 'R' outcomes 'Done' { }; end workflow;", "$WorkflowInstance"},
		{"instance in the description", wfTemplateHead + "description 'D {1}' with ({1} = $WorkflowInstance/Name) begin user task R 'R' outcomes 'Done' { }; end workflow;", "$WorkflowInstance"},
		{"user task in a task name", wfTemplateHead + "begin user task R 'R' task name 'T {1}' with ({1} = $WorkflowUserTask/Name) outcomes 'Done' { }; end workflow;", "$WorkflowUserTask"},
		{"current user in a description", wfTemplateHead + "begin user task R 'R' description 'T {1}' with ({1} = $currentUser/Name) outcomes 'Done' { }; end workflow;", "$currentUser"},
		{"instance in a task name", wfTemplateHead + "begin user task R 'R' task name 'T {1}' with ({1} = $WorkflowInstance/Name) outcomes 'Done' { }; end workflow;", ""},
		{"context in the display", wfTemplateHead + "display 'D {1}' with ({1} = $WorkflowContext/Name) begin user task R 'R' outcomes 'Done' { }; end workflow;", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs := wfRuleViolations(ValidateWorkflow(parseWorkflowStmt(t, c.src)), wfExprScopeRule)
			switch {
			case c.want == "" && len(vs) > 0:
				t.Errorf("reported an expression in scope: %v", vs)
			case c.want != "" && (len(vs) == 0 || !strings.Contains(vs[0].Message, c.want)):
				t.Errorf("want a violation naming %q, got %v", c.want, vs)
			}
		})
	}
}

// ALTER's texts are checked the same way.
func TestWorkflowTemplates_Alter(t *testing.T) {
	alter := func(src string) *ast.AlterWorkflowStmt {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs)
		}
		return prog.Statements[0].(*ast.AlterWorkflowStmt)
	}
	vs := ValidateAlterWorkflow(alter(`alter workflow M.W { set (TaskName: 'Check {1} {2}' with ({1} = $WorkflowContext/Name)) on Review; };`))
	if len(wfRuleViolations(vs, wfTemplateRule)) != 1 {
		t.Errorf("task name {2} with one parameter: %v", vs)
	}
	vs = ValidateAlterWorkflow(alter(`alter workflow M.W { set (Display: 'D {1}' with ({1} = $WorkflowInstance/Name)); };`))
	if len(wfRuleViolations(vs, wfExprScopeRule)) != 1 {
		t.Errorf("display with $WorkflowInstance: %v", vs)
	}
	vs = ValidateAlterWorkflow(alter(`alter workflow M.W { set (Display: 'D {1}' with ({1} = $WorkflowContext/Name)); set (Description: 'T {1}' with ({1} = $WorkflowInstance/Name)) on Review; };`))
	if len(wfRuleViolations(vs, wfTemplateRule))+len(wfRuleViolations(vs, wfExprScopeRule)) != 0 {
		t.Errorf("control: %v", vs)
	}
}

// The builder writes the parameters in placeholder order, with the header's
// declared context name rewritten to $WorkflowContext like every workflow
// expression.
func TestWorkflowTemplates_BuilderNormalizesTheContext(t *testing.T) {
	stmt := parseWorkflowStmt(t, `create workflow M.W parameter $Doc: M.Item
  display 'For {1} {2}' with ({2} = $Doc/Amount, {1} = $doc/Name)
begin
  user task R 'R' task name 'Review {1}' with ({1} = $Doc/Name) outcomes 'Done' { };
end workflow;`)
	wf := &workflows.Workflow{}
	norm := newContextExprNormalizer(stmt.ParameterVar)
	wf.WorkflowNameParams = norm.rewriteAll(workflowTemplateExprs(stmt.DisplayParams))
	if strings.Join(wf.WorkflowNameParams, ",") != "$WorkflowContext/Name,$WorkflowContext/Amount" {
		t.Errorf("normalized display params = %q", wf.WorkflowNameParams)
	}
	task := buildUserTask(stmt.Activities[0].(*ast.WorkflowUserTaskNode))
	autoBindActivitiesInFlow(nil, []workflows.WorkflowActivity{task}, norm)
	if task.TaskName != "Review {1}" || strings.Join(task.TaskNameParams, ",") != "$WorkflowContext/Name" {
		t.Errorf("task name = %q %q", task.TaskName, task.TaskNameParams)
	}
}

// ako/mxcli#1397: a workflow text is a template with the grammar every other
// template has (mdl/texttemplate): `{x}` is CE0712 on 11.13.0 and 11.15.0, and
// `{{1}}` is the escaped text "{1}" — no placeholder — which mxbuild accepts
// with no parameters and MDL-WF21 used to refuse as an unfilled {1}.
func TestWorkflowTemplates_BraceGrammar(t *testing.T) {
	cases := []struct{ name, src, rule, want string }{
		{"a name in braces (CE0712)",
			wfTemplateHead + "display 'Doc {Title}' begin user task R 'R' outcomes 'Done' { }; end workflow;",
			templateGrammarRule, "CE0712"},
		{"an unterminated placeholder in a task description (CE0713)",
			wfTemplateHead + "begin user task R 'R' description 'Due {1' with ({1} = $WorkflowContext/Name) outcomes 'Done' { }; end workflow;",
			templateGrammarRule, "CE0713"},
		{"an escaped placeholder needs no parameter", wfTemplateHead + "display 'Doc {{1}}' begin user task R 'R' outcomes 'Done' { }; end workflow;", "", ""},
		{"escaped braces around a placeholder", wfTemplateHead + "display 'Doc {{{1}}}' with ({1} = $WorkflowContext/Name) begin user task R 'R' outcomes 'Done' { }; end workflow;", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			all := ValidateWorkflow(parseWorkflowStmt(t, c.src))
			got := append(wfRuleViolations(all, wfTemplateRule), wfRuleViolations(all, templateGrammarRule)...)
			switch {
			case c.want == "" && len(got) > 0:
				t.Errorf("reported a template mxbuild accepts: %v", got)
			case c.want != "" && (len(got) != 1 || got[0].RuleID != c.rule || !strings.Contains(got[0].Message, c.want)):
				t.Errorf("want one %s naming %q, got %v", c.rule, c.want, got)
			}
		})
	}
}
