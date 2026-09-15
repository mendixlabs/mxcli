// SPDX-License-Identifier: Apache-2.0

// Checks for the two handler hooks a workflow can declare: a user task's
// on-created microflow and the workflow's event handlers.
//
// Signatures, measured on 11.13.0 (one task or handler per shape, verdict = the
// literal `mx check` line):
//
//	on created (WorkflowUserTask, Ctx)                 0 errors
//	on created (Ctx, WorkflowUserTask)                 0 errors — order is free
//	on created (WorkflowUserTask) / (Ctx) / ()         CE6683
//	on created (WorkflowUserTask, Ctx, String)         CE6683 — exactly two
//	on created (WorkflowUserTask, <specialization>)    CE6683
//	on created returning Boolean                       CE5012
//	event handler (WorkflowEvent, WorkflowRecord,
//	               WorkflowActivityRecord)             0 errors, in any order
//	event handler (WorkflowEvent) / () / (+ String)
//	              / (WorkflowEvent, Ctx)               CE6691
//	event handler listing an invented event type       0 errors (!)
//	event handler with an empty event type list        0 errors
//
// The invented-type row is why event types are checked here at all: mxbuild
// does not, so a misspelt type would be written and never fire.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

const (
	workflowEventEntity          = "System.WorkflowEvent"
	workflowRecordEntity         = "System.WorkflowRecord"
	workflowActivityRecordEntity = "System.WorkflowActivityRecord"
)

// ValidateWorkflowEventTypes reports an event type name that is not a workflow
// event type (MDL-WF12). It needs no project, so a typo is caught by a plain
// `mxcli check`; exec calls it too, because exec is reachable without check.
func ValidateWorkflowEventTypes(stmt *ast.CreateWorkflowStmt) []linter.Violation {
	var out []linter.Violation
	for _, h := range stmt.EventHandlers {
		for _, t := range h.EventTypes {
			if _, ok := canonicalWorkflowEventType(t); ok {
				continue
			}
			out = append(out, linter.Violation{
				RuleID:   "MDL-WF12",
				Severity: linter.SeverityError,
				Location: workflowLocation(stmt.Name),
				Message: fmt.Sprintf("workflow '%s': '%s' is not a workflow event type — mxbuild accepts any name here "+
					"and the handler would never run for it", stmt.Name.String(), t),
				Suggestion: "use one of: " + strings.Join(workflowEventTypeOrder, ", ") +
					" — or `on any workflow event`",
			})
		}
	}
	return out
}

// validateWorkflowEventHandlers checks the header's handlers against the
// project: each microflow resolves and has the handler signature, and each event
// type exists in the project's Mendix version.
func validateWorkflowEventHandlers(ctx *ExecContext, s *ast.CreateWorkflowStmt, sc *scriptContext) []string {
	if ctx == nil || ctx.Backend == nil || !ctx.Connected() || len(s.EventHandlers) == 0 {
		return nil
	}
	var errs []string
	pv := ctx.Backend.ProjectVersion()
	var known map[string]bool
	for _, h := range s.EventHandlers {
		label := workflowEventHandlerLabel(h)
		if qn := h.Microflow.String(); qn != "" && qn != "." {
			if known == nil {
				known = buildMicroflowQualifiedNames(ctx)
			}
			if !known[qn] && (sc == nil || !sc.microflows[qn]) {
				errs = append(errs, fmt.Sprintf("microflow not found: %s (referenced by %s)", qn, label))
			}
		}
		if pv == nil {
			continue
		}
		if h.AnyEvent {
			if _, _, ok := allWorkflowEventTypes(pv.MajorVersion, pv.MinorVersion, pv.PatchVersion); !ok {
				errs = append(errs, fmt.Sprintf(
					"%s: which event types Mendix %d.%d.%d has is not known to mxcli — Studio Pro stores the list, not "+
						"\"any\", and the set is measured only from 11.6.0 on; name the types instead: "+
						"`on workflow events (UserTaskStarted, UserTaskEnded) microflow …`",
					label, pv.MajorVersion, pv.MinorVersion, pv.PatchVersion))
			}
			continue
		}
		for _, t := range h.EventTypes {
			name, ok := canonicalWorkflowEventType(t)
			if !ok {
				continue // MDL-WF12
			}
			if lastAbsent, missing := workflowEventTypeMissingIn(name, pv.MajorVersion, pv.MinorVersion, pv.PatchVersion); missing {
				errs = append(errs, fmt.Sprintf(
					"%s: event type %s does not exist in Mendix %d.%d.%d (it is absent from %s)",
					label, name, pv.MajorVersion, pv.MinorVersion, pv.PatchVersion, lastAbsent))
			}
		}
	}
	if c := newWorkflowTaskSignatureChecker(ctx, sc); c != nil {
		for _, h := range s.EventHandlers {
			errs = appendIfSet(errs, c.checkEventHandler(workflowEventHandlerLabel(h), h.Microflow.String()))
		}
	}
	return errs
}

func workflowEventHandlerLabel(h ast.WorkflowEventHandlerNode) string {
	if h.Description != "" {
		return fmt.Sprintf("workflow event handler '%s'", h.Description)
	}
	return "workflow event handler " + h.Microflow.String()
}

// checkOnCreated reports an on-created microflow that is not exactly
// System.WorkflowUserTask plus the context entity (CE6683), or that returns a
// value (CE5012). A generalization of the context entity is let through: it is
// accepted for targeting, unmeasured here, and refusing it would be a guess.
func (c *workflowTaskSignatureChecker) checkOnCreated(label, mfQN, contextEntity string) string {
	if mfQN == "" || mfQN == "." {
		return ""
	}
	sig, ok := c.flowSignature(mfQN)
	if !ok || sig == nil {
		return ""
	}
	if contextEntity != "" && contextEntity != "." && c.pairMismatch(sig, workflowUserTaskEntity, contextEntity) {
		return fmt.Sprintf(
			"%s: on-created microflow %s takes %s — it must take exactly two parameters, %s and the workflow's "+
				"context entity %s, in either order; the build fails CE6683",
			label, mfQN, describeFlowParams(sig), workflowUserTaskEntity, contextEntity)
	}
	// Only a return type the script declared is known; a stored flow's is not
	// read, and unknown is not wrong.
	if sig.ReturnKind != ast.TypeUnknown && sig.ReturnKind != ast.TypeVoid {
		return fmt.Sprintf(
			"%s: on-created microflow %s returns %s — it must return nothing; the build fails CE5012",
			label, mfQN, sig.ReturnKind.String())
	}
	return ""
}

// pairMismatch reports a signature that is provably not (fixed, context) in
// either order — the shape targeting and on-created microflows share.
func (c *workflowTaskSignatureChecker) pairMismatch(sig *flowSignature, fixedEntity, contextEntity string) bool {
	if len(sig.Params) != 2 {
		return true
	}
	for i, p := range sig.Params {
		if !strings.EqualFold(p.Entity, fixedEntity) {
			continue
		}
		other := sig.Params[1-i]
		if other.Entity == "" {
			continue
		}
		if c.contextAssignableTo(contextEntity, other.Entity) != inheritanceNo {
			return false
		}
	}
	return true
}

// checkEventHandler reports a handler microflow that does not take exactly the
// three workflow event records (CE6691).
func (c *workflowTaskSignatureChecker) checkEventHandler(label, mfQN string) string {
	if mfQN == "" || mfQN == "." {
		return ""
	}
	sig, ok := c.flowSignature(mfQN)
	if !ok || sig == nil {
		return ""
	}
	want := []string{workflowEventEntity, workflowRecordEntity, workflowActivityRecordEntity}
	matches := len(sig.Params) == len(want)
	if matches {
		seen := map[string]bool{}
		for _, p := range sig.Params {
			seen[strings.ToLower(p.Entity)] = true
		}
		for _, w := range want {
			matches = matches && seen[strings.ToLower(w)]
		}
	}
	if matches {
		return ""
	}
	return fmt.Sprintf(
		"%s: microflow %s takes %s — a workflow event handler must take exactly %s, in any order; the build fails CE6691",
		label, mfQN, describeFlowParams(sig), strings.Join(want, ", "))
}
