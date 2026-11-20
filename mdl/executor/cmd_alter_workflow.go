// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// execAlterWorkflow handles ALTER WORKFLOW Module.Name { operations }.
func execAlterWorkflow(ctx *ExecContext, s *ast.AlterWorkflowStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Version pre-check: the workflow document's own floor, and the added
	// activities' features (statementVersionGates).
	if err := checkVersionGates(ctx, s); err != nil {
		return err
	}

	if vs := completionRuleViolations(alterWorkflowAddedActivities(s), workflowLocation(s.Name)); len(vs) > 0 {
		return mdlerrors.NewValidationf("%s\n  → %s", vs[0].Message, vs[0].Suggestion)
	}
	if vs := callWorkflowParameterViolations(alterWorkflowAddedActivities(s), workflowLocation(s.Name)); len(vs) > 0 {
		return mdlerrors.NewValidationf("%s [%s]\n  → %s", vs[0].Message, vs[0].RuleID, vs[0].Suggestion)
	}

	// Same exec-side guard as CREATE WORKFLOW: ALTER had no reference validation
	// at all, so an inserted activity could name a microflow that exists nowhere
	// and still be written (issue #943).
	if err := refuseWorkflowReturn(alterWorkflowAddedActivities(s)); err != nil {
		return err
	}
	// MDL-WF19, as for CREATE (mendixlabs/mxcli#1529).
	if vs := checkWorkflowExprScope(alterWorkflowExprSites(s), "", workflowLocation(s.Name)); len(vs) > 0 {
		return mdlerrors.NewValidationf("workflow '%s': %s\n  → %s", s.Name.String(), vs[0].Message, vs[0].Suggestion)
	}
	if vs := checkWorkflowTemplates(alterWorkflowTemplateSites(s), workflowLocation(s.Name)); len(vs) > 0 {
		return mdlerrors.NewValidationf("workflow '%s': %s [%s]\n  → %s", s.Name.String(), vs[0].Message, vs[0].RuleID, vs[0].Suggestion)
	}
	if vs := checkWorkflowRecurrences(alterWorkflowRecurrenceSites(s), workflowLocation(s.Name)); len(vs) > 0 {
		return mdlerrors.NewValidationf("workflow '%s': %s [%s]\n  → %s", s.Name.String(), vs[0].Message, vs[0].RuleID, vs[0].Suggestion)
	}
	if vs := alterWorkflowBoundaryCountViolations(s, workflowLocation(s.Name)); len(vs) > 0 {
		return mdlerrors.NewValidationf("workflow '%s': %s [%s]\n  → %s", s.Name.String(), vs[0].Message, vs[0].RuleID, vs[0].Suggestion)
	}
	if refErrors := validateAlterWorkflowRefs(ctx, s, nil); len(refErrors) > 0 {
		return mdlerrors.NewValidationf("workflow '%s' has reference errors:\n  - %s",
			s.Name.String(), strings.Join(refErrors, "\n  - "))
	}
	if err := checkBoundaryEventDrops(ctx, s); err != nil {
		return err
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find workflow by qualified name
	allWorkflows, err := ctx.Backend.ListWorkflows()
	if err != nil {
		return mdlerrors.NewBackend("list workflows", err)
	}

	var wfID model.ID
	for _, wf := range allWorkflows {
		modID := h.FindModuleID(wf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == s.Name.Module && wf.Name == s.Name.Name {
			wfID = wf.ID
			break
		}
	}
	if wfID == "" {
		return mdlerrors.NewNotFound("workflow", s.Name.Module+"."+s.Name.Name)
	}

	// Open mutator
	mutator, err := ctx.Backend.OpenWorkflowForMutation(wfID)
	if err != nil {
		return mdlerrors.NewBackend("open workflow for mutation", err)
	}

	// Resolve every operation's target before changing anything (the generic
	// ALTER, ADR-0012): a miss or an ambiguity refuses the statement, listing
	// the matches, instead of leaving the operations before it applied.
	if err := resolveAlterWorkflowTargets(mutator, s); err != nil {
		return err
	}

	// Where an inserted interrupting boundary path may end in an End, as
	// Studio Pro ends it (ako/mxcli#1712).
	ends := newAlterEndPlacement(ctx, s)

	// Apply operations sequentially
	for _, op := range s.Operations {
		switch o := op.(type) {
		case *ast.SetWorkflowPropertyOp:
			switch o.Property {
			case "overview_page":
				// overview_page uses Entity as the page qualified name (Value is unused).
				qn := o.Entity.Module + "." + o.Entity.Name
				if qn == "." {
					qn = ""
				}
				if err := mutator.SetPropertyWithEntity(o.Property, qn, qn); err != nil {
					return mdlerrors.NewBackend("set "+o.Property, err)
				}
			case "parameter":
				// PARAMETER uses Value as the variable name and Entity as the entity qualified name.
				qn := o.Entity.Module + "." + o.Entity.Name
				if qn == "." {
					qn = ""
				}
				if err := mutator.SetPropertyWithEntity(o.Property, o.Value, qn); err != nil {
					return mdlerrors.NewBackend("set "+o.Property, err)
				}
			default:
				value := o.Value
				if o.Property == "due_date" {
					value = normalizeWorkflowContextExpr(value) // MDL-WF19's accepted casings
				}
				if err := mutator.SetProperty(o.Property, value); err != nil {
					return mdlerrors.NewBackend("set "+o.Property, err)
				}
				// A text states its whole value: its parameters, or none
				// (ako/mxcli#1714).
				if o.Property == "display" || o.Property == "description" {
					if err := mutator.SetTemplateParameters("", 0, o.Property, alterTemplateExprs(o.Params)); err != nil {
						return mdlerrors.NewBackend("set "+o.Property, err)
					}
				}
			}

		case *ast.SetActivityPropertyOp:
			value := o.Value
			switch o.Property {
			case "page":
				value = o.PageName.Module + "." + o.PageName.Name
			case "targeting_microflow":
				value = o.Microflow.Module + "." + o.Microflow.Name
			case "due_date":
				value = normalizeWorkflowContextExpr(value) // MDL-WF19's accepted casings
			}
			if err := mutator.SetActivityProperty(o.ActivityRef, o.AtPosition, o.Property, value); err != nil {
				return mdlerrors.NewBackend("set activity", err)
			}
			if o.Property == "description" || o.Property == "task_name" {
				if err := mutator.SetTemplateParameters(o.ActivityRef, o.AtPosition, o.Property, alterTemplateExprs(o.Params)); err != nil {
					return mdlerrors.NewBackend("set activity", err)
				}
			}

		case *ast.InsertAfterOp:
			acts := buildAndBindActivities(ctx, o.NewActivities)
			if len(acts) == 0 {
				return mdlerrors.NewValidation("failed to build new activity")
			}
			ends.endInserted(acts, o.ActivityRef, o.AtPosition)
			if err := mutator.InsertAfterActivity(o.ActivityRef, o.AtPosition, acts); err != nil {
				return mdlerrors.NewBackend("insert after", err)
			}

		case *ast.InsertBeforeOp:
			acts := buildAndBindActivities(ctx, o.NewActivities)
			if len(acts) == 0 {
				return mdlerrors.NewValidation("failed to build new activity")
			}
			ends.endInserted(acts, o.ActivityRef, o.AtPosition)
			if err := mutator.InsertBeforeActivity(o.ActivityRef, o.AtPosition, acts); err != nil {
				return mdlerrors.NewBackend("insert before", err)
			}

		case *ast.DropActivityOp:
			if err := mutator.DropActivity(o.ActivityRef, o.AtPosition); err != nil {
				return mdlerrors.NewBackend("drop activity", err)
			}

		case *ast.ReplaceActivityOp:
			acts := buildAndBindActivities(ctx, o.NewActivities)
			if len(acts) == 0 {
				return mdlerrors.NewValidation("failed to build replacement activity")
			}
			ends.endInserted(acts, o.ActivityRef, o.AtPosition)
			if err := mutator.ReplaceActivity(o.ActivityRef, o.AtPosition, acts); err != nil {
				return mdlerrors.NewBackend("replace activity", err)
			}

		case *ast.InsertOutcomeOp:
			acts := buildAndBindActivities(ctx, o.Activities)
			ends.endInserted(acts, o.ActivityRef, o.AtPosition)
			if err := mutator.InsertOutcome(o.ActivityRef, o.AtPosition, o.OutcomeName, acts); err != nil {
				return mdlerrors.NewBackend("insert outcome", err)
			}

		case *ast.DropOutcomeOp:
			if err := mutator.DropOutcome(o.ActivityRef, o.AtPosition, o.OutcomeName); err != nil {
				return mdlerrors.NewBackend("drop outcome", err)
			}

		case *ast.InsertPathOp:
			acts := buildAndBindActivities(ctx, o.Activities)
			if err := mutator.InsertPath(o.ActivityRef, o.AtPosition, "", acts); err != nil {
				return mdlerrors.NewBackend("insert path", err)
			}

		case *ast.DropPathOp:
			if err := mutator.DropPath(o.ActivityRef, o.AtPosition, o.PathCaption); err != nil {
				return mdlerrors.NewBackend("drop path", err)
			}

		case *ast.InsertBranchOp:
			acts := buildAndBindActivities(ctx, o.Activities)
			ends.endInserted(acts, o.ActivityRef, o.AtPosition)
			if err := mutator.InsertBranch(o.ActivityRef, o.AtPosition, o.Condition, acts); err != nil {
				return mdlerrors.NewBackend("insert branch", err)
			}

		case *ast.DropBranchOp:
			if err := mutator.DropBranch(o.ActivityRef, o.AtPosition, o.BranchName); err != nil {
				return mdlerrors.NewBackend("drop branch", err)
			}

		case *ast.InsertBoundaryEventOp:
			// The op carries no name, and a notification boundary event is only
			// reachable by its name. Refused rather than written unnamed.
			if (&workflows.BoundaryEvent{EventType: o.EventType}).IsNotification() {
				return mdlerrors.NewUnsupported("insert boundary event: a notification boundary event cannot be inserted with ALTER WORKFLOW yet — " +
					"restate the workflow with CREATE OR MODIFY WORKFLOW, which accepts `boundary event interrupting notification <name> '<caption>' { … }`")
			}
			acts := ends.endInsertedBoundaryPath(buildAndBindActivities(ctx, o.Activities), o.EventType, o.ActivityRef, o.AtPosition)
			if err := mutator.InsertBoundaryEvent(o.ActivityRef, o.AtPosition, o.EventType, normalizeWorkflowContextExpr(o.Delay), o.Caption, recurrenceFromNode(o.Recurrence), acts); err != nil {
				return mdlerrors.NewBackend("insert boundary event", err)
			}

		case *ast.DropBoundaryEventOp:
			if err := mutator.DropBoundaryEvent(o.ActivityRef, o.AtPosition, o.Index); err != nil {
				return mdlerrors.NewBackend("drop boundary event", err)
			}

		default:
			return mdlerrors.NewUnsupported(fmt.Sprintf("unknown alter workflow operation type: %T", op))
		}
	}

	// Save
	if err := mutator.Save(); err != nil {
		return mdlerrors.NewBackend("save modified workflow", err)
	}

	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Altered workflow %s.%s\n", s.Name.Module, s.Name.Name)
	return nil
}

// alterWorkflowOpTarget returns the activity an operation addresses, as the
// generic target it was written as; ok is false for a workflow-level SET.
func alterWorkflowOpTarget(op ast.AlterWorkflowOp) (backend.AlterTarget, bool) {
	var ref string
	var pos int
	switch o := op.(type) {
	case *ast.SetActivityPropertyOp:
		ref, pos = o.ActivityRef, o.AtPosition
	case *ast.InsertAfterOp:
		ref, pos = o.ActivityRef, o.AtPosition
	case *ast.InsertBeforeOp:
		ref, pos = o.ActivityRef, o.AtPosition
	case *ast.DropActivityOp:
		ref, pos = o.ActivityRef, o.AtPosition
	case *ast.ReplaceActivityOp:
		ref, pos = o.ActivityRef, o.AtPosition
	case *ast.InsertOutcomeOp:
		ref, pos = o.ActivityRef, o.AtPosition
	case *ast.DropOutcomeOp:
		ref, pos = o.ActivityRef, o.AtPosition
	case *ast.InsertPathOp:
		ref, pos = o.ActivityRef, o.AtPosition
	case *ast.DropPathOp:
		ref, pos = o.ActivityRef, o.AtPosition
	case *ast.InsertBranchOp:
		ref, pos = o.ActivityRef, o.AtPosition
	case *ast.DropBranchOp:
		ref, pos = o.ActivityRef, o.AtPosition
	case *ast.InsertBoundaryEventOp:
		ref, pos = o.ActivityRef, o.AtPosition
	case *ast.DropBoundaryEventOp:
		ref, pos = o.ActivityRef, o.AtPosition
	default:
		return backend.AlterTarget{}, false
	}
	// A name and a caption are compared alike (a name wins); the target
	// carries the text as a name so the resolver's messages print it bare.
	return backend.AlterTarget{Path: []string{ref}, Ordinal: pos}, true
}

// resolveAlterWorkflowTargets resolves every operation's target against the
// stored workflow as it stands before the statement. An operation that targets
// an activity an earlier operation of the same statement inserted is resolved
// when it is applied instead, by the mutator's own lookup.
func resolveAlterWorkflowTargets(resolver backend.AlterTargetResolver, s *ast.AlterWorkflowStmt) error {
	inserted := map[string]bool{}
	for _, op := range s.Operations {
		if t, ok := alterWorkflowOpTarget(op); ok && !inserted[backend.WorkflowTargetText(t)] {
			if _, err := resolver.ResolveAlterTarget(t); err != nil {
				return mdlerrors.NewValidationf("alter workflow %s: %v", s.Name.String(), err)
			}
		}
		for _, n := range alterWorkflowOpNewNames(op) {
			inserted[n] = true
		}
	}
	return nil
}

// alterWorkflowOpNewNames lists the names and captions of the activities an
// operation adds at the top of its fragment.
func alterWorkflowOpNewNames(op ast.AlterWorkflowOp) []string {
	var acts []ast.WorkflowActivityNode
	switch o := op.(type) {
	case *ast.InsertAfterOp:
		acts = o.NewActivities
	case *ast.InsertBeforeOp:
		acts = o.NewActivities
	case *ast.ReplaceActivityOp:
		acts = o.NewActivities
	case *ast.InsertOutcomeOp:
		acts = o.Activities
	case *ast.InsertPathOp:
		acts = o.Activities
	case *ast.InsertBranchOp:
		acts = o.Activities
	case *ast.InsertBoundaryEventOp:
		acts = o.Activities
	}
	var out []string
	for _, a := range acts {
		name, caption := workflowActivityNodeLabels(a)
		out = append(out, name, caption)
	}
	return out
}

// buildAndBindActivities builds workflow activities from AST nodes and auto-binds parameters.
func buildAndBindActivities(ctx *ExecContext, nodes []ast.WorkflowActivityNode) []workflows.WorkflowActivity {
	acts := buildWorkflowActivities(nodes)
	// ALTER carries no `parameter $X:` header, so there is no author-declared
	// alias to honour — casing normalization only.
	autoBindActivitiesInFlow(ctx, acts, contextExprNormalizer{})
	return acts
}

// workflowActivityNodeLabels reads an activity node's Name and Caption, the
// two texts a target can match. Every activity node carries them under those
// field names; a node without one yields "".
func workflowActivityNodeLabels(a ast.WorkflowActivityNode) (name, caption string) {
	v := reflect.ValueOf(a)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return "", ""
	}
	if f := v.FieldByName("Name"); f.IsValid() && f.Kind() == reflect.String {
		name = f.String()
	}
	if f := v.FieldByName("Caption"); f.IsValid() && f.Kind() == reflect.String {
		caption = f.String()
	}
	return name, caption
}

// alterTemplateExprs is an ALTER's template parameters in placeholder order,
// in the casings MDL-WF19 accepts. ALTER has no header alias to rewrite: the
// stored context is $WorkflowContext.
func alterTemplateExprs(params []ast.WorkflowTemplateParam) []string {
	exprs := workflowTemplateExprs(params)
	for i, e := range exprs {
		exprs[i] = normalizeWorkflowContextExpr(e)
	}
	return exprs
}
