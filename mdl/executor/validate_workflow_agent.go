// SPDX-License-Identifier: Apache-2.0

// Checks for the AI agent task (`call agent microflow`, Workflows$AIAgentTaskActivity).
//
// Measured on mxbuild 11.13.0 by writing each shape as a call-microflow activity
// and switching only its $Type, so the two columns differ in nothing else:
//
//	agent microflow                         call microflow   AI agent task
//	(Ctx), mapped                           0 errors         0 errors
//	() — no parameters                      0 errors         CE1590 "Missing parameter"
//	(Ctx, String), both mapped              0 errors         0 errors
//	(System.Workflow)                       0 errors         0 errors
//	returns Boolean, true/false outcomes    0 errors         0 errors
//	returns an enumeration, value outcomes  0 errors         0 errors
//	interrupting timer boundary event       0 errors         0 errors
//
// So an agent task is a call-microflow activity with one extra rule: its
// microflow must take a parameter.
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// checkAgentMicroflow reports an AI agent task whose microflow takes no
// parameters (CE1590). A microflow whose signature is unknown is left alone.
func (c *workflowTaskSignatureChecker) checkAgentMicroflow(label, mfQN string) string {
	if mfQN == "" || mfQN == "." {
		return ""
	}
	sig, ok := c.flowSignature(mfQN)
	if !ok || sig == nil || len(sig.Params) > 0 {
		return ""
	}
	return fmt.Sprintf(
		"%s: microflow %s takes no parameters — an AI agent task's microflow must take at least one, usually the "+
			"workflow's context object mapped with `with (<Param> = '$WorkflowContext')`; the build fails CE1590",
		label, mfQN)
}

// workflowUsesAgentTask reports whether any activity, at any depth, is an AI
// agent task — what the Mendix 11.9 version gate applies to.
func workflowUsesAgentTask(activities []ast.WorkflowActivityNode) bool {
	found := false
	walkWorkflowActivities(activities, func(a ast.WorkflowActivityNode) {
		if cm, ok := a.(*ast.WorkflowCallMicroflowNode); ok && cm.Agent {
			found = true
		}
	})
	return found
}

func countAuthoredAgentTasks(activities []ast.WorkflowActivityNode) int {
	n := 0
	walkWorkflowActivities(activities, func(a ast.WorkflowActivityNode) {
		if cm, ok := a.(*ast.WorkflowCallMicroflowNode); ok && cm.Agent {
			n++
		}
	})
	return n
}
