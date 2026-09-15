// SPDX-License-Identifier: Apache-2.0

// Checks for a multi-user task's completion rule (`decide by …`).
//
// Measured on mxbuild 11.13.0, one multi-user task per shape with only its
// CompletionCriteria / TargetUserInput patched:
//
//	consensus, majority (either kind), threshold without a fallback   CE1866
//	veto without a veto outcome                                       CE1867
//	microflow rule with no microflow                                  CE0113
//	microflow returning Boolean                                       CE5012
//	microflow with no / context / WorkflowUserTask parameters         0 errors
//	threshold 0 or 101 percent, 0 or 5 votes                          0 errors
//	participants 0 or 150 percent, 0 users                            0 errors
//
// So check refuses a missing or unknown fallback / veto outcome and a decision
// microflow that provably does not return String. Out-of-range numbers build, and
// refusing them would be a guess.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateWorkflowCompletionRules applies MDL-WF13 to a CREATE WORKFLOW body.
func ValidateWorkflowCompletionRules(stmt *ast.CreateWorkflowStmt) []linter.Violation {
	return completionRuleViolations(workflowStatementActivities(stmt), workflowLocation(stmt.Name))
}

// completionRuleViolations reports MDL-WF13 for every multi-user task, at any
// depth, whose `decide by` names no usable outcome.
func completionRuleViolations(activities []ast.WorkflowActivityNode, loc linter.Location) []linter.Violation {
	var out []linter.Violation
	walkWorkflowActivities(activities, func(a ast.WorkflowActivityNode) {
		n, ok := a.(*ast.WorkflowUserTaskNode)
		if !ok || !n.IsMultiUser || n.Completion == nil {
			return
		}
		label := workflowUserTaskLabel(n)
		var outcomes []string
		for _, o := range n.Outcomes {
			outcomes = append(outcomes, o.Caption)
		}
		isOutcome := func(v string) bool {
			for _, o := range outcomes {
				if o == v {
					return true
				}
			}
			return false
		}
		add := func(msg, suggestion string) {
			out = append(out, linter.Violation{
				RuleID: "MDL-WF13", Severity: linter.SeverityError, Location: loc, Message: msg, Suggestion: suggestion,
			})
		}
		known := "'" + strings.Join(outcomes, "', '") + "'"
		r := n.Completion
		switch r.Rule {
		case "consensus", "majority", "threshold":
			if !r.HasFallback {
				add(fmt.Sprintf("multi user task %s decides by %s without a fallback outcome — Mendix requires one for this rule; the build fails CE1866", label, r.Rule),
					fmt.Sprintf("Add `fallback '<outcome>'`, naming one of %s.", known))
			} else if !isOutcome(r.Fallback) {
				add(fmt.Sprintf("multi user task %s falls back to '%s', which is not one of its outcomes (%s); the build fails CE1866", label, r.Fallback, known),
					"Name one of the task's own outcomes in `fallback '…'`.")
			}
		case "veto":
			if !isOutcome(r.Veto) {
				add(fmt.Sprintf("multi user task %s vetoes with '%s', which is not one of its outcomes (%s); the build fails CE1867", label, r.Veto, known),
					"Name one of the task's own outcomes in `decide by veto '…'`.")
			}
		}
	})
	return out
}

// checkDecisionMicroflow reports a `decide by microflow` whose microflow provably
// does not return String (CE5012). Only a return type the script declares is
// known; a stored flow's is not read, and unknown is not wrong.
func (c *workflowTaskSignatureChecker) checkDecisionMicroflow(label, mfQN string) string {
	if mfQN == "" || mfQN == "." {
		return ""
	}
	sig, ok := c.flowSignature(mfQN)
	if !ok || sig == nil || sig.ReturnKind == ast.TypeUnknown || sig.ReturnKind == ast.TypeString {
		return ""
	}
	return fmt.Sprintf(
		"%s: decision microflow %s returns %s — a multi-user task's decision microflow must return String (the chosen outcome); the build fails CE5012",
		label, mfQN, sig.ReturnKind.String())
}
