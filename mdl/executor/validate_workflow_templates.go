// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/texttemplate"
)

// MDL-WF21: a workflow text whose placeholders its parameters do not fill
// (ako/mxcli#1714). The workflow's display and description, and a user task's
// task name and description, are templates: `'Review {1}' with ({1} =
// $WorkflowContext/Title)`. Measured with mx check on 11.15.0:
//
//	'Review {1} {2}' with one parameter    CE0720 "Place holder index 2 is greater than 1, the number of parameter(s)."
//	'Review {1}' with none                 CE0720 (index 1 is greater than 0) — the workflow's display too
//	'Review {0}'                           CE0716 "Place holder indices start at one (1)."
//	'Review' with one parameter            clean (an unused parameter)
//	'Review {1} {1}' with one              clean
//	'Review {{1}}' with none               clean: {{ escapes, so it is the text "{1}"
//	'Review {Title}'                       CE0712, as in every template (MDL-TEMPLATE02, ako/mxcli#1397)
//
// The parameters are stored by position, so `with ({1} = …, {3} = …)` has no
// meaning to write: a numbering other than {1} to {n}, each once, is refused
// with the rest. What a parameter's expression may name is MDL-WF19's.
const wfTemplateRule = "MDL-WF21"

// wfTemplateSite is one text template a workflow statement writes.
type wfTemplateSite struct {
	where  string // "the task name of user task 'Review'"
	text   string
	params []ast.WorkflowTemplateParam
}

// checkWorkflowTemplates reports each template whose placeholders and
// parameters do not match.
func checkWorkflowTemplates(sites []wfTemplateSite, loc linter.Location) []linter.Violation {
	var out []linter.Violation
	report := func(msg, suggestion string) {
		out = append(out, linter.Violation{
			RuleID: wfTemplateRule, Severity: linter.SeverityError, Location: loc,
			Message: msg, Suggestion: suggestion,
		})
	}
	for _, s := range sites {
		n := len(s.params)
		indices := make([]int, 0, n)
		for _, p := range s.params {
			indices = append(indices, p.Index)
		}
		sort.Ints(indices)
		for i, idx := range indices {
			if idx != i+1 {
				report(fmt.Sprintf("%s numbers its parameters %s — they are stored by position, so they must be %s, each once",
					s.where, placeholderList(indices), placeholderRange(n)),
					fmt.Sprintf("Number them {1} to {%d} in the order the text uses them.", n))
				break
			}
		}
		// The text's own grammar first, as mxbuild reads it: one problem per
		// text, and the count only for a text that has none (ako/mxcli#1397).
		// An escaped `{{1}}` is the text "{1}", no placeholder.
		refs, p := texttemplate.Placeholders(s.text)
		if p != nil && p.Code == "CE0716" {
			report(fmt.Sprintf("%s has the placeholder {0} — MxBuild rejects it (CE0716 \"Place holder indices start at one (1).\")", s.where),
				"Number the placeholders from {1}.")
			continue
		}
		if p != nil {
			msg, fix := templateTextRefusal(s.where, s.text, n)
			out = append(out, linter.Violation{
				RuleID: templateGrammarRule, Severity: linter.SeverityError, Location: loc,
				Message: msg, Suggestion: fix,
			})
			continue
		}
		maxRef := 0
		for _, idx := range refs {
			if idx > maxRef {
				maxRef = idx
			}
		}
		if maxRef > n {
			report(fmt.Sprintf("%s uses the placeholder {%d} but has %d parameter(s) — MxBuild rejects it (CE0720 \"Place holder index %d is greater than %d, the number of parameter(s).\")",
				s.where, maxRef, n, maxRef, n),
				fmt.Sprintf("Fill every placeholder: `'…' with ({1} = $WorkflowContext/Attr%s)`.", moreParams(maxRef)))
		}
	}
	return out
}

func placeholderList(indices []int) string {
	parts := make([]string, len(indices))
	for i, idx := range indices {
		parts[i] = fmt.Sprintf("{%d}", idx)
	}
	return strings.Join(parts, ", ")
}

// placeholderRange is "{1}" or "{1} to {n}".
func placeholderRange(n int) string {
	if n == 1 {
		return "{1}"
	}
	return fmt.Sprintf("{1} to {%d}", n)
}

func moreParams(n int) string {
	if n <= 1 {
		return ""
	}
	return fmt.Sprintf(", …, {%d} = …", n)
}

// workflowTemplateSites collects the templates a CREATE WORKFLOW writes: the
// header's display and description, and every user task's task name and
// description, nested ones included.
func workflowTemplateSites(stmt *ast.CreateWorkflowStmt) []wfTemplateSite {
	sites := []wfTemplateSite{
		{where: "the workflow's display", text: stmt.DisplayName, params: stmt.DisplayParams},
		{where: "the workflow's description", text: stmt.Description, params: stmt.DescriptionParams},
	}
	return append(sites, userTaskTemplateSites(workflowStatementActivities(stmt))...)
}

func userTaskTemplateSites(acts []ast.WorkflowActivityNode) []wfTemplateSite {
	var sites []wfTemplateSite
	walkWorkflowActivities(acts, func(a ast.WorkflowActivityNode) {
		n, ok := a.(*ast.WorkflowUserTaskNode)
		if !ok {
			return
		}
		label := workflowUserTaskLabel(n)
		name := n.TaskName
		if name == "" {
			name = n.Caption // what a task without `task name` is given
		}
		sites = append(sites,
			wfTemplateSite{where: "the task name of user task " + label, text: name, params: n.TaskNameParams},
			wfTemplateSite{where: "the description of user task " + label, text: n.TaskDescription, params: n.TaskDescriptionParams})
	})
	return sites
}

// alterWorkflowTemplateSites collects the templates an ALTER WORKFLOW writes.
func alterWorkflowTemplateSites(stmt *ast.AlterWorkflowStmt) []wfTemplateSite {
	sites := userTaskTemplateSites(alterWorkflowAddedActivities(stmt))
	for _, op := range stmt.Operations {
		switch o := op.(type) {
		case *ast.SetWorkflowPropertyOp:
			if o.Property == "display" || o.Property == "description" {
				sites = append(sites, wfTemplateSite{where: "the workflow's " + o.Property, text: o.Value, params: o.Params})
			}
		case *ast.SetActivityPropertyOp:
			if o.Property == "description" || o.Property == "task_name" {
				sites = append(sites, wfTemplateSite{where: "the " + strings.ReplaceAll(o.Property, "_", " ") + " of " + o.ActivityRef, text: o.Value, params: o.Params})
			}
		}
	}
	return sites
}

// workflowTemplateExprSites are a template's parameters as MDL-WF19 sites:
// header ones see only the context (measured on 11.15.0: $WorkflowInstance is
// CE0117 in the display and description, accepted in a task's name and
// description; $WorkflowUserTask and $currentUser are CE0117 in all four).
func workflowTemplateExprSites(sites []wfTemplateSite, header func(wfTemplateSite) bool) []wfExprSite {
	var out []wfExprSite
	for _, s := range sites {
		for _, p := range s.params {
			out = append(out, wfExprSite{where: fmt.Sprintf("{%d} of %s", p.Index, s.where), expr: p.Expression, header: header(s)})
		}
	}
	return out
}

func isWorkflowHeaderTemplate(s wfTemplateSite) bool {
	return strings.HasPrefix(s.where, "the workflow's ")
}
