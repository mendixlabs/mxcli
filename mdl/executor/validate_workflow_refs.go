// SPDX-License-Identifier: Apache-2.0

// Reference-based (project-connected) validation for workflows. Runs under
// `check --references`, where the target microflows can be introspected. See
// validate_workflow.go for the syntax-only (no-project) checks.
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// validateWorkflowParameterMappings checks that each workflow "call microflow"
// activity maps every parameter of its target microflow. Mendix rejects an
// unmapped parameter (CE6677 — "should accept parameters ...") and the workflow
// would fail at the activity, but mxcli check passed it (FINDINGS #40). Microflows
// created in the same script are skipped (not yet queryable); a target microflow
// that isn't in the project is left to the missing-reference check.
func validateWorkflowParameterMappings(ctx *ExecContext, s *ast.CreateWorkflowStmt, sc *scriptContext) []string {
	if ctx == nil || ctx.Backend == nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return nil
	}
	paramsByMF := make(map[string][]string, len(mfs))
	for _, mf := range mfs {
		names := make([]string, 0, len(mf.Parameters))
		for _, p := range mf.Parameters {
			names = append(names, p.Name)
		}
		paramsByMF[h.GetQualifiedName(mf.ContainerID, mf.Name)] = names
	}

	var errs []string
	walkWorkflowActivities(workflowStatementActivities(s), func(act ast.WorkflowActivityNode) {
		cm, ok := act.(*ast.WorkflowCallMicroflowNode)
		if !ok {
			return
		}
		mfQN := cm.Microflow.String()
		if sc != nil && sc.microflows[mfQN] {
			return // created in the same script — cannot introspect its parameters
		}
		want, known := paramsByMF[mfQN]
		if !known {
			return // target microflow not in project; the missing-ref check covers it
		}
		mapped := make(map[string]bool, len(cm.ParameterMappings))
		for _, pm := range cm.ParameterMappings {
			mapped[pm.Parameter] = true
		}
		for _, name := range want {
			if !mapped[name] {
				errs = append(errs, fmt.Sprintf(
					"call microflow '%s': parameter '%s' is not mapped — Mendix requires every parameter of a workflow call-microflow to be mapped (add `with (%s = ...)`)",
					mfQN, name, name))
			}
		}
	})
	return errs
}

// validateWorkflowReferences checks every qualified name a workflow's activities
// point at against the project (and against what the same script creates).
//
// This is the "missing-reference check" validateWorkflowParameterMappings above
// says it defers to, and which did not exist: a workflow calling a microflow
// that was nowhere in the project passed `check --references` with "All
// references valid" and was written by exec, leaving the Mendix validator
// (CE1613) as the only thing that noticed. The identical mistake inside a plain
// microflow body was caught, because validateFlowBodyReferences runs for
// microflows and nanoflows only (issue #943).
//
// The lookups and message wording deliberately match validateFlowBodyReferences,
// so the same mistake reads the same way wherever it is made.
func validateWorkflowReferences(ctx *ExecContext, activities []ast.WorkflowActivityNode, sc *scriptContext) []string {
	if ctx == nil || !ctx.Connected() || len(activities) == 0 {
		return nil
	}

	// The lookups are built lazily: most workflows reference only microflows, and
	// each build is a full backend list.
	var microflowNames, workflowNames, pageNames map[string]bool
	knownMicroflow := func(qn string) bool {
		if microflowNames == nil {
			microflowNames = buildMicroflowQualifiedNames(ctx)
		}
		return microflowNames[qn] || (sc != nil && sc.microflows[qn])
	}
	knownWorkflow := func(qn string) bool {
		if workflowNames == nil {
			workflowNames = buildWorkflowQualifiedNames(ctx)
		}
		return workflowNames[qn] || (sc != nil && sc.workflows[qn])
	}
	knownPage := func(qn string) bool {
		if pageNames == nil {
			pageNames = buildPageQualifiedNames(ctx)
		}
		return pageNames[qn] || (sc != nil && sc.pages[qn])
	}

	var errs []string
	seen := map[string]bool{} // one report per distinct reference
	report := func(kind, qn, via string) {
		if qn == "" || seen[kind+qn] {
			return
		}
		// A System.* target is provided by the runtime and never appears in the
		// project, the same exemption validateFlowBodyReferences makes for Java
		// actions. Reporting it would be a guaranteed false positive.
		if isBuiltinModuleEntity(qualifiedNameModule(qn)) {
			return
		}
		seen[kind+qn] = true
		errs = append(errs, fmt.Sprintf("%s not found: %s (referenced by %s)", kind, qn, via))
	}

	walkWorkflowActivities(activities, func(act ast.WorkflowActivityNode) {
		switch n := act.(type) {
		case *ast.WorkflowCallMicroflowNode:
			if qn := n.Microflow.String(); qn != "." && !knownMicroflow(qn) {
				report("microflow", qn, "call microflow")
			}
		case *ast.WorkflowCallWorkflowNode:
			if qn := n.Workflow.String(); qn != "." && !knownWorkflow(qn) {
				report("workflow", qn, "call workflow")
			}
		case *ast.WorkflowUserTaskNode:
			if qn := n.Page.String(); qn != "." && qn != "" && !knownPage(qn) {
				report("page", qn, "user task page")
			}
			// Targeting by microflow, for users or for groups; the XPath variants
			// carry no qualified name.
			if qn := n.Targeting.Microflow.String(); qn != "." && qn != "" && !knownMicroflow(qn) {
				report("microflow", qn, "user task targeting")
			}
			if qn := n.OnCreated.String(); qn != "." && qn != "" && !knownMicroflow(qn) {
				report("microflow", qn, "user task on created")
			}
			if n.Completion != nil && n.Completion.Rule == "microflow" {
				if qn := n.Completion.Microflow.String(); qn != "." && qn != "" && !knownMicroflow(qn) {
					report("microflow", qn, "multi user task decide by microflow")
				}
			}
		}
	})
	return errs
}

// validateWorkflowStatementRefs is the CREATE WORKFLOW entry point: the context
// entity and the module the workflow is being created in, plus every activity
// reference.
//
// The module check matters more than it looks. exec creates a module on demand,
// so without it a typo'd module name silently produced a new module rather than
// an error — `check` is the only thing standing in the way, and it did this for
// a microflow but not for a workflow.
func validateWorkflowStatementRefs(ctx *ExecContext, s *ast.CreateWorkflowStmt, sc *scriptContext) []string {
	if ctx == nil || !ctx.Connected() {
		return nil
	}
	var errs []string
	if s.Name.Module != "" && (sc == nil || !sc.modules[s.Name.Module]) {
		if _, err := findModule(ctx, s.Name.Module); err != nil {
			errs = append(errs, fmt.Sprintf("module not found: %s", s.Name.Module))
		}
	}
	if qn := s.ParameterEntity.String(); qn != "" && qn != "." {
		if !isBuiltinModuleEntity(s.ParameterEntity.Module) {
			known := buildEntityQualifiedNames(ctx)
			if !known[qn] && (sc == nil || !sc.entities[qn]) {
				errs = append(errs, fmt.Sprintf("entity not found: %s (referenced by workflow parameter)", qn))
			}
		}
	}
	// Event sub-process bodies reference microflows, pages and entities too.
	activities := workflowStatementActivities(s)
	errs = append(errs, bareTimerBoundaryEventErrors(ctx, activities, 0)...)
	errs = append(errs, validateWorkflowReferences(ctx, activities, sc)...)
	errs = append(errs, validateWorkflowEventHandlers(ctx, s, sc)...)
	// Then the signatures of the page and targeting microflow each user task
	// hands work to — names that resolve can still be the wrong shape (CE7410,
	// CE7412, CE6677). A target that did not resolve is skipped there, so it is
	// reported once, above.
	return append(errs, validateWorkflowTaskSignatures(ctx, activities, s.ParameterEntity.String(), sc)...)
}

// bareTimerBoundaryEventErrors refuses `boundary event timer` without a kind
// (MDL-WF07) on Mendix 11 and later.
//
// The bare form writes Workflows$TimerBoundaryEvent. No 11.x runtime has that
// class — measured by its absence from the 11.10, 11.13 and 11.14 runtime
// bundles, which carry only the interrupting and non-interrupting variants — and
// the consequence is the worst available: `mxcli check` and mxbuild both pass,
// and the runtime refuses to START the application:
//
//	Class 'Workflows$TimerBoundaryEvent' could not be found
//
// This is not a document failing to load; it is every user of the app locked
// out by one boundary event. The form was also mxcli's own documented example.
//
// Gated on 11, because that is where it is measured: whether a 10.x runtime
// knew the type is unknown here, and refusing it there would be a guess.
// extraBare counts bare events the caller found outside the activity list — an
// ALTER op's own event — and is added to the walk's.
func bareTimerBoundaryEventErrors(ctx *ExecContext, activities []ast.WorkflowActivityNode, extraBare int) []string {
	if ctx == nil || ctx.Backend == nil {
		return nil
	}
	pv := ctx.Backend.ProjectVersion()
	if pv == nil || !pv.IsAtLeast(11, 0) {
		return nil
	}
	n := extraBare
	count := func(events []ast.WorkflowBoundaryEventNode) {
		for _, e := range events {
			if e.EventType == "Timer" {
				n++
			}
		}
	}
	walkWorkflowActivities(activities, func(a ast.WorkflowActivityNode) {
		switch t := a.(type) {
		case *ast.WorkflowUserTaskNode:
			count(t.BoundaryEvents)
		case *ast.WorkflowCallMicroflowNode:
			count(t.BoundaryEvents)
		case *ast.WorkflowWaitForNotificationNode:
			count(t.BoundaryEvents)
		}
	})
	if n == 0 {
		return nil
	}
	return []string{fmt.Sprintf(
		"%d boundary event(s) written as a bare `timer` — on Mendix %s that stores Workflows$TimerBoundaryEvent, a type the runtime does not have, "+
			"so check and mxbuild pass and the application then refuses to start (\"Class 'Workflows$TimerBoundaryEvent' could not be found\"). "+
			"Write `boundary event interrupting timer` or `boundary event non interrupting timer` [MDL-WF07]",
		n, pv.String())}
}

// validateAlterWorkflowRefs validates ALTER WORKFLOW, which had no case in the
// validation switch at all and so fell through to "skip validation" — it got
// nothing, not even a check that the workflow it targets exists.
func validateAlterWorkflowRefs(ctx *ExecContext, s *ast.AlterWorkflowStmt, sc *scriptContext) []string {
	if ctx == nil || !ctx.Connected() {
		return nil
	}
	var errs []string
	qn := s.Name.String()
	if qn != "" && qn != "." && !isBuiltinModuleEntity(s.Name.Module) {
		known := buildWorkflowQualifiedNames(ctx)
		if !known[qn] && (sc == nil || !sc.workflows[qn]) {
			errs = append(errs, fmt.Sprintf("workflow not found: %s", qn))
		}
	}

	// MDL-WF07: a bare `timer` is refused here too — INSERT BOUNDARY EVENT writes
	// the same unloadable type, and so does any boundary event nested in an
	// inserted activity.
	{
		bare := 0
		var nested []ast.WorkflowActivityNode
		for _, op := range s.Operations {
			switch o := op.(type) {
			case *ast.InsertBoundaryEventOp:
				if o.EventType == "Timer" {
					bare++
				}
				nested = append(nested, o.Activities...)
			case *ast.InsertAfterOp:
				nested = append(nested, o.NewActivity)
			case *ast.ReplaceActivityOp:
				nested = append(nested, o.NewActivity)
			case *ast.InsertOutcomeOp:
				nested = append(nested, o.Activities...)
			case *ast.InsertPathOp:
				nested = append(nested, o.Activities...)
			case *ast.InsertBranchOp:
				nested = append(nested, o.Activities...)
			}
		}
		errs = append(errs, bareTimerBoundaryEventErrors(ctx, nested, bare)...)
	}

	// Every op that introduces activities gets the same reference check a CREATE
	// body gets. Ops that only remove or rename something introduce no reference.
	var added []ast.WorkflowActivityNode
	for _, op := range s.Operations {
		switch o := op.(type) {
		case *ast.InsertAfterOp:
			added = append(added, o.NewActivity)
		case *ast.ReplaceActivityOp:
			added = append(added, o.NewActivity)
		case *ast.InsertOutcomeOp:
			added = append(added, o.Activities...)
		case *ast.InsertPathOp:
			added = append(added, o.Activities...)
		case *ast.InsertBranchOp:
			added = append(added, o.Activities...)
		case *ast.InsertBoundaryEventOp:
			added = append(added, o.Activities...)
		case *ast.SetActivityPropertyOp:
			// SET PAGE / SET TARGETING MICROFLOW name something that must exist.
			if p := o.PageName.String(); p != "" && p != "." && !isBuiltinModuleEntity(o.PageName.Module) {
				known := buildPageQualifiedNames(ctx)
				if !known[p] && (sc == nil || !sc.pages[p]) {
					errs = append(errs, fmt.Sprintf("page not found: %s (referenced by set activity page)", p))
				}
			}
			if m := o.Microflow.String(); m != "" && m != "." && !isBuiltinModuleEntity(o.Microflow.Module) {
				known := buildMicroflowQualifiedNames(ctx)
				if !known[m] && (sc == nil || !sc.microflows[m]) {
					errs = append(errs, fmt.Sprintf("microflow not found: %s (referenced by set activity targeting)", m))
				}
			}
		}
	}
	// An inserting op must also be aimed at an activity whose document can hold
	// what it writes — ako/mxcli#415, where it could not and the project stopped
	// loading. Same function for both passes, so `check --references` and `exec`
	// cannot drift.
	errs = append(errs, validateAlterWorkflowActivityKinds(ctx, s)...)
	// REPLACE ACTIVITY rebuilds the activity from the statement, so state MDL
	// cannot express — an on-created microflow, a completion rule — would be
	// reset; refused like the same loss in a whole-workflow rewrite.
	errs = append(errs, validateAlterReplaceKeepsStudioProState(ctx, s)...)

	// And an inserted `end workflow` must not land under a parallel split or a
	// non-interrupting boundary event that only the stored workflow shows.
	errs = append(errs, validateAlterWorkflowEndAncestry(ctx, s)...)

	errs = append(errs, validateWorkflowReferences(ctx, added, sc)...)
	return append(errs, validateAlterWorkflowTaskSignatures(ctx, s, added, sc)...)
}
