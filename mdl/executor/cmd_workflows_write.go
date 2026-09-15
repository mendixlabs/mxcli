// SPDX-License-Identifier: Apache-2.0

// Package executor - CREATE/DROP WORKFLOW commands
package executor

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// execCreateWorkflow handles CREATE WORKFLOW statements.
func execCreateWorkflow(ctx *ExecContext, s *ast.CreateWorkflowStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// A standalone `annotation` lands in the workflow's activity flow, which
	// Mendix loads by constructing every child with a Flow parent — no annotation
	// type takes one, so the written .mpr cannot be LOADED at all (Studio Pro
	// won't open the project and `mx check` dies before validating anything).
	// Refuse here as well as at check time (MDL-WF04): emitting a structurally
	// invalid unit takes down the whole project, not one document. (issuetracker #15)
	if hasStandaloneWorkflowAnnotation(s.Activities) {
		return mdlerrors.NewUnsupported(
			"a standalone `annotation` in a workflow body would produce a model Mendix cannot load " +
				"(the annotation is placed in the activity flow, which accepts only flow elements) — " +
				"remove it, or keep the note as an MDL comment (`-- ...`) [MDL-WF04]")
	}

	// Same for a jump whose target names no activity. A jump target is the only
	// INTRA-document reference a workflow has, and validateWorkflowStatementRefs
	// resolves only external ones (microflows, pages, entities), so nothing
	// looked at it: the jump was written pointing at itself and surfaced under
	// the native validator as CE6681, an error describing a different fault
	// (mendixlabs/mxcli#1005). Refused with the check-time rule's own function so
	// the two cannot drift, and here as well as at check time for the #833
	// reason — exec is reachable without check.
	if vs := ValidateWorkflowJumpTargets(s); len(vs) > 0 {
		return mdlerrors.NewValidationf("workflow '%s': %s\n  → %s",
			s.Name.String(), vs[0].Message, vs[0].Suggestion)
	}

	// A `return;` builds nothing, so exec would drop it and the branch it ends
	// would fall through. Refused here for the #833 reason — exec is reachable
	// without check — as well as at check time (MDL-WF11).
	if err := refuseWorkflowReturn(s.Activities); err != nil {
		return err
	}

	// Refuse a broken reference here as well as at check time. `check
	// --references` reports these, but exec runs a different pass and wrote the
	// workflow anyway, so a script that skipped check produced a model the build
	// rejects with CE1613 (issue #943). Same placement and reasoning as the
	// microflow handler's validateMicroflowRules call (issue #833).
	//
	// Note this runs BEFORE findOrCreateModule, which auto-creates a module on
	// demand: without it a typo'd module name silently produced a new module
	// rather than an error.
	if vs := ValidateWorkflowCompletionRules(s); len(vs) > 0 {
		return mdlerrors.NewValidationf("%s\n  → %s", vs[0].Message, vs[0].Suggestion)
	}
	if vs := ValidateWorkflowEventSubProcesses(s); len(vs) > 0 {
		return mdlerrors.NewValidationf("%s\n  → %s", vs[0].Message, vs[0].Suggestion)
	}
	if vs := ValidateWorkflowEventTypes(s); len(vs) > 0 {
		return mdlerrors.NewValidationf("%s\n  → %s", vs[0].Message, vs[0].Suggestion)
	}
	if len(s.EventHandlers) > 0 {
		if err := checkFeature(ctx, "workflows", "event_handlers", "on workflow events",
			"remove the `on … workflow event` clauses, or upgrade the project"); err != nil {
			return err
		}
	}
	if workflowUsesAgentTask(workflowStatementActivities(s)) {
		if err := checkFeature(ctx, "workflows", "ai_agent_task", "call agent microflow",
			"AI agent tasks need Mendix 11.9 or later — use `call microflow` on older projects"); err != nil {
			return err
		}
	}
	if err := checkEventSubProcessFeatures(ctx, s); err != nil {
		return err
	}

	if refErrors := validateWorkflowStatementRefs(ctx, s, nil); len(refErrors) > 0 {
		return mdlerrors.NewValidationf("workflow '%s' has reference errors:\n  - %s",
			s.Name.String(), strings.Join(refErrors, "\n  - "))
	}

	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	// Check if workflow already exists
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	existingWorkflows, err := ctx.Backend.ListWorkflows()
	if err != nil {
		return mdlerrors.NewBackend("list workflows", err)
	}

	var existingID model.ID
	var existingContainer model.ID
	// Excluded is model state, not script state, and a module may hold an
	// excluded twin of this name — target the live workflow and carry its
	// exclusion forward (#914).
	existingExcluded := false
	var existingDocumentation string
	var existingHandlers []*workflows.WorkflowEventHandler
	haveExistingWf := false
	if existing, ok := pickLive(existingWorkflows,
		func(w *workflows.Workflow) bool {
			return h.GetModuleName(h.FindModuleID(w.ContainerID)) == s.Name.Module && w.Name == s.Name.Name
		},
		func(w *workflows.Workflow) bool { return w.Excluded },
	); ok {
		if !s.CreateOrModify {
			return mdlerrors.NewAlreadyExistsMsg("workflow", s.Name.Module+"."+s.Name.Name, "workflow '"+s.Name.Module+"."+s.Name.Name+"' already exists (use create or modify to overwrite)")
		}
		existingID = existing.ID
		existingExcluded = existing.Excluded
		existingContainer = existing.ContainerID
		existingDocumentation = existing.Documentation
		existingHandlers = existing.EventHandlers
		haveExistingWf = true

		// Refuse a rewrite that would delete a stored construct this statement
		// does not restate (guard-don't-drop, ADR-0005) — issue #948.
		if err := checkNoDroppedWorkflowConstructs(ctx, existingID, s.Name.String(), s); err != nil {
			return err
		}
	}

	containerID, err := containerForDocument(ctx, module.ID, s.Folder, existingContainer)
	if err != nil {
		return err
	}

	wf := &workflows.Workflow{}
	wf.Excluded = existingExcluded
	wf.ContainerID = containerID
	wf.Name = s.Name.Name
	wf.Documentation = s.Documentation
	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if haveExistingWf {
		wf.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, existingDocumentation)
	}

	// Parameter
	if s.ParameterEntity.Module != "" {
		wf.Parameter = &workflows.WorkflowParameter{
			EntityRef: s.ParameterEntity.Module + "." + s.ParameterEntity.Name,
		}
		wf.Parameter.ID = model.ID(generateWorkflowUUID())
	}

	// Overview page
	if s.OverviewPage.Module != "" {
		wf.OverviewPage = s.OverviewPage.Module + "." + s.OverviewPage.Name
	}

	// Display metadata
	wf.WorkflowName = s.DisplayName
	wf.WorkflowDescription = s.Description
	if s.ExportLevel != "" {
		wf.ExportLevel = s.ExportLevel
	}

	// Due date
	wf.DueDate = s.DueDate

	handlers, err := buildWorkflowEventHandlers(ctx, s.EventHandlers, existingHandlers)
	if err != nil {
		return err
	}
	wf.EventHandlers = handlers

	// Build activities with implicit start/end
	flow := &workflows.Flow{}
	flow.ID = model.ID(generateWorkflowUUID())

	// Add implicit start activity
	startAct := &workflows.StartWorkflowActivity{}
	startAct.ID = model.ID(generateWorkflowUUID())
	startAct.Caption = "Start"
	startAct.Name = "Start"

	// Add implicit end activity
	endAct := &workflows.EndWorkflowActivity{}
	endAct.ID = model.ID(generateWorkflowUUID())
	endAct.Caption = "End"
	endAct.Name = "End"

	// Build user-defined activities
	userActivities := buildWorkflowActivities(s.Activities)

	// Auto-bind microflow/workflow parameters and sanitize names
	autoBindWorkflowParameters(ctx, userActivities, s.ParameterVar)

	// Event sub-processes. Their activities share the workflow's one namespace —
	// a start event named like a main-flow activity is CE0495 — so they are
	// deduplicated together with the main flow's.
	wf.EventSubProcesses = buildEventSubProcesses(s.EventSubProcesses)
	named := append([]workflows.WorkflowActivity{}, userActivities...)
	for _, esp := range wf.EventSubProcesses {
		autoBindWorkflowParameters(ctx, esp.Flow.Activities, s.ParameterVar)
		named = append(named, esp.Flow.Activities...)
	}

	// Deduplicate activity names to avoid CE0495
	// The implicit Start and End take part: an `end workflow` inside a branch is
	// named after its caption, "End" by default, and colliding with the main
	// flow's End is CE0495 "Duplicate name 'End'".
	deduplicateActivityNames(named, startAct.Name, endAct.Name)

	// Compose: start + user activities + end
	flow.Activities = make([]workflows.WorkflowActivity, 0, len(userActivities)+2)
	flow.Activities = append(flow.Activities, startAct)
	flow.Activities = append(flow.Activities, userActivities...)
	flow.Activities = append(flow.Activities, endAct)

	wf.Flow = flow

	if existingID != "" {
		// In-place update: preserve UUID so references and BSON git-diff stay stable.
		wf.ID = existingID
		if err := ctx.Backend.UpdateWorkflow(wf); err != nil {
			return mdlerrors.NewBackend("update workflow", err)
		}
		if _, err := applyDocumentFolder(ctx, wf.ID, existingContainer, containerID); err != nil {
			return err
		}
	} else {
		if err := ctx.Backend.CreateWorkflow(wf); err != nil {
			return mdlerrors.NewBackend("create workflow", err)
		}
	}

	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Created workflow: %s.%s\n", s.Name.Module, s.Name.Name)
	return nil
}

// execDropWorkflow handles DROP WORKFLOW statements.
func execDropWorkflow(ctx *ExecContext, s *ast.DropWorkflowStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	wfs, err := ctx.Backend.ListWorkflows()
	if err != nil {
		return mdlerrors.NewBackend("list workflows", err)
	}

	for _, wf := range wfs {
		modID := h.FindModuleID(wf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == s.Name.Module && wf.Name == s.Name.Name {
			if err := ctx.Backend.DeleteWorkflow(wf.ID); err != nil {
				return mdlerrors.NewBackend("delete workflow", err)
			}
			invalidateHierarchy(ctx)
			fmt.Fprintf(ctx.Output, "Dropped workflow: %s.%s\n", s.Name.Module, s.Name.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFound("workflow", s.Name.Module+"."+s.Name.Name)
}

// generateWorkflowUUID generates a UUID for workflow elements.
func generateWorkflowUUID() string {
	return types.GenerateID()
}

// buildWorkflowActivities converts AST activity nodes to SDK workflow activities.
func buildWorkflowActivities(nodes []ast.WorkflowActivityNode) []workflows.WorkflowActivity {
	var activities []workflows.WorkflowActivity
	for _, node := range nodes {
		act := buildWorkflowActivity(node)
		if act != nil {
			activities = append(activities, act)
		}
	}
	return activities
}

// buildBoundaryEvents converts AST boundary event nodes to SDK boundary events.
func buildBoundaryEvents(nodes []ast.WorkflowBoundaryEventNode) []*workflows.BoundaryEvent {
	var events []*workflows.BoundaryEvent
	for _, be := range nodes {
		event := &workflows.BoundaryEvent{
			EventType:  be.EventType,
			TimerDelay: be.Delay,
			Name:       be.Name,
			Caption:    be.Caption,
		}
		if event.IsNotification() {
			// Mendix requires both: the name is what a notify action targets
			// (unique in the workflow, made so by deduplicateActivityNames).
			if event.Name == "" {
				event.Name = "NotificationEvent"
			}
			if event.Caption == "" {
				event.Caption = event.Name
			}
		}
		event.ID = model.ID(generateWorkflowUUID())
		if len(be.Activities) > 0 {
			event.Flow = &workflows.Flow{
				Activities: buildWorkflowActivities(be.Activities),
			}
			event.Flow.ID = model.ID(generateWorkflowUUID())
		}
		// A boundary path must end in a jump, an end or Mendix's end-of-path
		// marker: interrupting is CE0105 without it, and non-interrupting builds
		// cleanly and then stops the runtime from starting at all — see
		// workflows.EndBoundaryEventPath.
		event.Flow = workflows.EndBoundaryEventPath(event.Flow, newWorkflowID)
		events = append(events, event)
	}
	return events
}

// buildWorkflowActivity converts a single AST activity node to an SDK workflow activity.
func buildWorkflowActivity(node ast.WorkflowActivityNode) workflows.WorkflowActivity {
	switch n := node.(type) {
	case *ast.WorkflowUserTaskNode:
		return buildUserTask(n)
	case *ast.WorkflowCallMicroflowNode:
		return buildCallMicroflowTask(n)
	case *ast.WorkflowCallWorkflowNode:
		return buildCallWorkflowActivity(n)
	case *ast.WorkflowDecisionNode:
		return buildExclusiveSplit(n)
	case *ast.WorkflowParallelSplitNode:
		return buildParallelSplit(n)
	case *ast.WorkflowJumpToNode:
		return buildJumpTo(n)
	case *ast.WorkflowWaitForTimerNode:
		return buildWaitForTimer(n)
	case *ast.WorkflowWaitForNotificationNode:
		return buildWaitForNotification(n)
	case *ast.WorkflowNotificationNode:
		return buildNotificationActivity(n)
	case *ast.WorkflowEndNode:
		return buildEndWorkflow(n)
	case *ast.WorkflowAnnotationActivityNode:
		return buildAnnotationActivity(n)
	default:
		return nil
	}
}

// buildWorkflowEventHandlers turns the header's handler clauses into stored
// handlers. `any workflow event` becomes the list the project version knows,
// because that is what Studio Pro stores; a named list is written in Studio
// Pro's order. A handler's documentation cannot be written from MDL, so a
// rewrite carries it from the stored handler with the same microflow and
// description.
func buildWorkflowEventHandlers(ctx *ExecContext, nodes []ast.WorkflowEventHandlerNode, stored []*workflows.WorkflowEventHandler) ([]*workflows.WorkflowEventHandler, error) {
	var out []*workflows.WorkflowEventHandler
	for _, n := range nodes {
		h := &workflows.WorkflowEventHandler{
			Description: n.Description,
			Microflow:   n.Microflow.String(),
		}
		h.ID = model.ID(generateWorkflowUUID())
		if n.AnyEvent {
			pv := ctx.Backend.ProjectVersion()
			if pv == nil {
				return nil, mdlerrors.NewUnsupported("`on any workflow event` needs the project's Mendix version, which is not available")
			}
			types, _, ok := allWorkflowEventTypes(pv.MajorVersion, pv.MinorVersion, pv.PatchVersion)
			if !ok {
				return nil, mdlerrors.NewUnsupported(fmt.Sprintf(
					"`on any workflow event`: the event types of Mendix %d.%d.%d are not known — name them instead",
					pv.MajorVersion, pv.MinorVersion, pv.PatchVersion))
			}
			h.EventTypes = types
		} else {
			var names []string
			for _, t := range n.EventTypes {
				if name, ok := canonicalWorkflowEventType(t); ok {
					names = append(names, name)
				}
			}
			h.EventTypes = sortWorkflowEventTypes(names)
		}
		for _, sh := range stored {
			if sh != nil && strings.EqualFold(sh.Microflow, h.Microflow) && sh.Description == h.Description {
				h.Documentation = sh.Documentation
				break
			}
		}
		out = append(out, h)
	}
	return out, nil
}

// buildTargetUserInput maps `participants …`; nil (omitted) stays nil — all users.
func buildTargetUserInput(p *ast.WorkflowParticipantsNode) *workflows.TargetUserInput {
	if p == nil {
		return nil
	}
	switch p.Kind {
	case "number":
		return &workflows.TargetUserInput{Kind: "Absolute", Amount: p.Value}
	case "percent":
		return &workflows.TargetUserInput{Kind: "Percentage", Percentage: p.Value}
	default:
		return &workflows.TargetUserInput{Kind: "All"}
	}
}

// buildCompletionCriteria maps `decide by …`; nil (omitted) stays nil — consensus
// falling back to the first outcome, which is what a rebuild has always written.
// `more than half` / `percent` are Studio Pro's Absolute majority and Relative
// threshold (measured on ako/TestApp, 11.14.0).
func buildCompletionCriteria(r *ast.WorkflowCompletionRuleNode) *workflows.CompletionCriteria {
	if r == nil {
		return nil
	}
	switch r.Rule {
	case "majority":
		ct := "Relative"
		if r.Majority == "more than half" {
			ct = "Absolute"
		}
		return &workflows.CompletionCriteria{Kind: "Majority", CompletionType: ct, FallbackOutcome: r.Fallback}
	case "threshold":
		ct := "Absolute"
		if r.ThresholdUnit == "percent" {
			ct = "Relative"
		}
		return &workflows.CompletionCriteria{Kind: "Threshold", CompletionType: ct, Threshold: r.Threshold, FallbackOutcome: r.Fallback}
	case "veto":
		return &workflows.CompletionCriteria{Kind: "Veto", VetoOutcome: r.Veto}
	case "microflow":
		return &workflows.CompletionCriteria{Kind: "Microflow", Microflow: r.Microflow.String()}
	default:
		return &workflows.CompletionCriteria{Kind: "Consensus", FallbackOutcome: r.Fallback}
	}
}

func buildUserTask(n *ast.WorkflowUserTaskNode) *workflows.UserTask {
	task := &workflows.UserTask{}
	task.ID = model.ID(generateWorkflowUUID())
	task.Name = n.Name
	task.Caption = n.Caption
	task.DueDate = n.DueDate
	task.TaskDescription = n.TaskDescription
	task.IsMulti = n.IsMultiUser
	if n.IsMultiUser {
		task.AwaitAllUsers = n.AwaitAllUsers
		task.TargetUserInput = buildTargetUserInput(n.Participants)
		task.CompletionCriteria = buildCompletionCriteria(n.Completion)
	}

	if n.Page.Module != "" {
		task.Page = n.Page.Module + "." + n.Page.Name
	}

	if n.Entity.Module != "" {
		task.UserTaskEntity = n.Entity.Module + "." + n.Entity.Name
	}

	if n.OnCreated.Module != "" {
		task.OnCreated = n.OnCreated.String()
	}

	// Targeting
	switch n.Targeting.Kind {
	case "microflow":
		task.UserSource = &workflows.MicroflowBasedUserSource{
			Microflow: n.Targeting.Microflow.Module + "." + n.Targeting.Microflow.Name,
		}
	case "xpath":
		task.UserSource = &workflows.XPathBasedUserSource{
			XPath: n.Targeting.XPath,
		}
	case "group_microflow":
		task.UserSource = &workflows.MicroflowGroupSource{
			Microflow: n.Targeting.Microflow.Module + "." + n.Targeting.Microflow.Name,
		}
	case "group_xpath":
		task.UserSource = &workflows.XPathGroupSource{
			XPath: n.Targeting.XPath,
		}
	}

	// Outcomes
	for _, outcomeNode := range n.Outcomes {
		outcome := &workflows.UserTaskOutcome{
			Name:    outcomeNode.Caption,
			Caption: outcomeNode.Caption,
			Value:   outcomeNode.Caption,
		}
		outcome.ID = model.ID(generateWorkflowUUID())

		if len(outcomeNode.Activities) > 0 {
			outcome.Flow = &workflows.Flow{
				Activities: buildWorkflowActivities(outcomeNode.Activities),
			}
			outcome.Flow.ID = model.ID(generateWorkflowUUID())
		}

		task.Outcomes = append(task.Outcomes, outcome)
	}

	// BoundaryEvents (Issue #7)
	task.BoundaryEvents = buildBoundaryEvents(n.BoundaryEvents)

	return task
}

func buildCallMicroflowTask(n *ast.WorkflowCallMicroflowNode) *workflows.CallMicroflowTask {
	task := &workflows.CallMicroflowTask{IsAgent: n.Agent}
	task.ID = model.ID(generateWorkflowUUID())
	task.Name = n.Microflow.Name
	task.Caption = n.Caption
	task.Microflow = n.Microflow.Module + "." + n.Microflow.Name

	if task.Caption == "" {
		task.Caption = task.Name
	}
	// An explicit `as <name>` overrides the microflow-derived name, but only
	// after the caption fallback above — the caption should still read as the
	// microflow, not as the activity id. Mendix resolves `jump to` by name
	// (ako/mxcli#408).
	if n.Name != "" {
		task.Name = n.Name
	}

	for _, outcomeNode := range n.Outcomes {
		outcome := buildConditionOutcome(outcomeNode)
		if outcome != nil {
			task.Outcomes = append(task.Outcomes, outcome)
		}
	}

	// Parameter mappings (Issue #10)
	// BSON requires fully qualified: Module.Microflow.ParamName
	mfQN := n.Microflow.Module + "." + n.Microflow.Name
	for _, pm := range n.ParameterMappings {
		task.ParameterMappings = append(task.ParameterMappings, &workflows.ParameterMapping{
			Parameter:  mfQN + "." + pm.Parameter,
			Expression: pm.Expression,
		})
	}

	// BoundaryEvents (Issue #7)
	task.BoundaryEvents = buildBoundaryEvents(n.BoundaryEvents)

	return task
}

func buildCallWorkflowActivity(n *ast.WorkflowCallWorkflowNode) *workflows.CallWorkflowActivity {
	act := &workflows.CallWorkflowActivity{}
	act.ID = model.ID(generateWorkflowUUID())
	act.Name = n.Workflow.Name
	act.Caption = n.Caption
	act.Workflow = n.Workflow.Module + "." + n.Workflow.Name

	if act.Caption == "" {
		act.Caption = act.Name
	}
	if n.Name != "" {
		act.Name = n.Name
	}

	// Auto-bind $WorkflowContext parameter expression
	act.ParameterExpression = "$WorkflowContext"

	// Explicit parameter mappings from MDL WITH clause
	// BSON requires fully qualified: Module.Workflow.ParamName
	wfQN := n.Workflow.Module + "." + n.Workflow.Name
	for _, pm := range n.ParameterMappings {
		mapping := &workflows.ParameterMapping{
			Parameter:  wfQN + "." + pm.Parameter,
			Expression: pm.Expression,
		}
		mapping.BaseElement.ID = model.ID(types.GenerateID())
		act.ParameterMappings = append(act.ParameterMappings, mapping)
	}

	return act
}

func buildExclusiveSplit(n *ast.WorkflowDecisionNode) *workflows.ExclusiveSplitActivity {
	act := &workflows.ExclusiveSplitActivity{}
	act.ID = model.ID(generateWorkflowUUID())
	// Same case-sensitivity trap as CALL MICROFLOW parameter mappings (#845):
	// the context parameter is named "WorkflowContext", so a user-written
	// `$workflowContext` is an undefined variable and mx check reports CE0117.
	act.Expression = normalizeWorkflowContextExpr(n.Expression)
	act.Caption = n.Caption

	if act.Caption == "" {
		act.Caption = "Decision"
	}
	// An explicit name is the activity's identity: Mendix resolves `jump to` by
	// JumpToActivity.TargetActivity, which stores a name. Falling back to the
	// caption keeps existing scripts unchanged (ako/mxcli#408).
	act.Name = act.Caption
	if n.Name != "" {
		act.Name = n.Name
	}

	// Detect boolean decision (has TRUE or FALSE outcomes).
	// The Mendix 11 runtime only supports BooleanConditionOutcome and
	// EnumerationValueConditionOutcome — VoidConditionOutcome (DEFAULT) is rejected.
	isBooleanDecision := false
	for _, o := range n.Outcomes {
		if o.Value == "True" || o.Value == "False" {
			isBooleanDecision = true
			break
		}
	}

	for _, outcomeNode := range n.Outcomes {
		if isBooleanDecision && outcomeNode.Value == "Default" {
			// Skip DEFAULT on boolean decisions — runtime rejects VoidConditionOutcome.
			continue
		}
		outcome := buildConditionOutcome(outcomeNode)
		if outcome != nil {
			act.Outcomes = append(act.Outcomes, outcome)
		}
	}

	return act
}

func buildConditionOutcome(n ast.WorkflowConditionOutcomeNode) workflows.ConditionOutcome {
	var subFlow *workflows.Flow
	if len(n.Activities) > 0 {
		subFlow = &workflows.Flow{
			Activities: buildWorkflowActivities(n.Activities),
		}
		subFlow.ID = model.ID(generateWorkflowUUID())
	}

	switch n.Value {
	case "True":
		o := &workflows.BooleanConditionOutcome{Value: true, Flow: subFlow}
		o.ID = model.ID(generateWorkflowUUID())
		return o
	case "False":
		o := &workflows.BooleanConditionOutcome{Value: false, Flow: subFlow}
		o.ID = model.ID(generateWorkflowUUID())
		return o
	case "Default":
		o := &workflows.VoidConditionOutcome{Flow: subFlow}
		o.ID = model.ID(generateWorkflowUUID())
		return o
	default:
		// Enumeration value
		o := &workflows.EnumerationValueConditionOutcome{Value: n.Value, Flow: subFlow}
		o.ID = model.ID(generateWorkflowUUID())
		return o
	}
}

// newWorkflowID adapts generateWorkflowUUID to the id factory the workflows
// package takes.
func newWorkflowID() model.ID { return model.ID(generateWorkflowUUID()) }

func buildParallelSplit(n *ast.WorkflowParallelSplitNode) *workflows.ParallelSplitActivity {
	act := &workflows.ParallelSplitActivity{}
	act.ID = model.ID(generateWorkflowUUID())
	act.Caption = n.Caption
	if act.Caption == "" {
		act.Caption = "Parallel split"
	}
	act.Name = act.Caption
	if n.Name != "" {
		act.Name = n.Name
	}

	for _, pathNode := range n.Paths {
		outcome := &workflows.ParallelSplitOutcome{}
		outcome.ID = model.ID(generateWorkflowUUID())
		if len(pathNode.Activities) > 0 {
			outcome.Flow = &workflows.Flow{
				Activities: buildWorkflowActivities(pathNode.Activities),
			}
			outcome.Flow.ID = model.ID(generateWorkflowUUID())
		}
		// Every path ends with Mendix's end-of-path marker, or the engine skips
		// its contents at runtime — see workflows.EndParallelSplitPath.
		outcome.Flow = workflows.EndParallelSplitPath(outcome.Flow, newWorkflowID)
		act.Outcomes = append(act.Outcomes, outcome)
	}

	return act
}

// jumpActivityName is the base name every jump activity gets; deduplication
// appends a counter, giving JumpTo, JumpTo2, ...
//
// It used to be the TARGET's name, which made the jump a second activity
// carrying that name. Mendix resolves TargetActivity by name, so the jump could
// resolve to itself — the build then fails CE6681 ("not possible to jump to end
// activities or jump-to activities"), an error naming a different fault
// (mendixlabs/mxcli#1005). Whether it did depended on flow order, because
// deduplication renames the SECOND activity it meets with a given name:
//
//	backward jump (target first)  jump becomes StepB2, target keeps StepB  — worked
//	forward  jump (jump first)    jump KEEPS StepB, target becomes StepB2  — broke
//
// so a jump to a perfectly valid activity was also affected, which is why fixing
// only the unresolved-target case would not have been enough.
const jumpActivityName = "JumpTo"

func buildJumpTo(n *ast.WorkflowJumpToNode) *workflows.JumpToActivity {
	act := &workflows.JumpToActivity{}
	act.ID = model.ID(generateWorkflowUUID())
	act.Name = jumpActivityName
	act.Caption = n.Caption
	act.TargetActivity = n.Target

	if act.Caption == "" {
		act.Caption = n.Target
	}

	return act
}

func buildWaitForTimer(n *ast.WorkflowWaitForTimerNode) *workflows.WaitForTimerActivity {
	act := &workflows.WaitForTimerActivity{}
	act.ID = model.ID(generateWorkflowUUID())
	// A delay may reference a date attribute on the workflow context (#845).
	act.DelayExpression = normalizeWorkflowContextExpr(n.DelayExpression)
	act.Caption = n.Caption

	if act.Caption == "" {
		act.Caption = "Wait for timer"
	}
	act.Name = act.Caption
	if n.Name != "" {
		act.Name = n.Name
	}

	return act
}

func buildWaitForNotification(n *ast.WorkflowWaitForNotificationNode) *workflows.WaitForNotificationActivity {
	act := &workflows.WaitForNotificationActivity{}
	act.ID = model.ID(generateWorkflowUUID())
	act.Caption = n.Caption

	if act.Caption == "" {
		act.Caption = "Wait for notification"
	}
	act.Name = act.Caption
	if n.Name != "" {
		act.Name = n.Name
	}

	// BoundaryEvents (Issue #7)
	act.BoundaryEvents = buildBoundaryEvents(n.BoundaryEvents)

	return act
}

// buildNotificationActivity builds an intermediate notification event. Mendix
// requires a name (CE0725).
func buildNotificationActivity(n *ast.WorkflowNotificationNode) *workflows.NotificationActivity {
	act := &workflows.NotificationActivity{}
	act.ID = model.ID(generateWorkflowUUID())
	act.Name = n.Name
	if act.Name == "" {
		act.Name = "Notification"
	}
	act.Caption = n.Caption
	if act.Caption == "" {
		act.Caption = act.Name
	}
	return act
}

// buildEventSubProcesses builds each event sub-process's flow: its start event,
// the body, and — as in the main flow — an implicit End when the body does not
// already end. Measured on mxbuild 11.13.0: a flow with no end is CE0105, and one
// that ends in a jump, or in branches that all end, takes no End after it
// (CE6689 otherwise).
func buildEventSubProcesses(nodes []ast.WorkflowEventSubProcessNode) []*workflows.EventSubProcess {
	var out []*workflows.EventSubProcess
	for _, n := range nodes {
		esp := &workflows.EventSubProcess{Name: n.Name, Caption: n.Caption}
		esp.ID = model.ID(generateWorkflowUUID())

		start := &workflows.EventSubProcessStartActivity{
			Interrupting:       n.Interrupting,
			Timer:              n.Timer,
			FirstExecutionTime: n.FirstExecutionTime,
		}
		start.ID = model.ID(generateWorkflowUUID())
		start.Name = n.StartName
		if start.Name == "" {
			start.Name = n.Name + "Start"
		}
		start.Caption = n.StartCaption
		if start.Caption == "" {
			start.Caption = start.Name
		}

		flow := &workflows.Flow{}
		flow.ID = model.ID(generateWorkflowUUID())
		flow.Activities = append([]workflows.WorkflowActivity{start}, buildWorkflowActivities(n.Activities)...)
		if !flowEnds(n.Activities) {
			end := &workflows.EndWorkflowActivity{}
			end.ID = model.ID(generateWorkflowUUID())
			end.Caption = "End"
			end.Name = "End"
			flow.Activities = append(flow.Activities, end)
		}
		esp.Flow = flow
		out = append(out, esp)
	}
	return out
}

func buildEndWorkflow(n *ast.WorkflowEndNode) *workflows.EndWorkflowActivity {
	act := &workflows.EndWorkflowActivity{}
	act.ID = model.ID(generateWorkflowUUID())
	act.Caption = n.Caption

	if act.Caption == "" {
		act.Caption = "End"
	}
	// The name is NOT the caption, unlike the other activities. A caption is
	// free text — `end workflow comment 'Rejected by manager'` named the End
	// "Rejected by manager", and mxbuild refused it as CE7247 "The name ... is
	// not valid". An End is never referenced by name (it cannot be a jump
	// target, CE6681), so a fixed base name, made unique by
	// deduplicateActivityNames, removes the problem instead of sanitising it.
	act.Name = "End"

	return act
}

// deduplicateActivityNames ensures all activity names within a workflow are unique.
// Mendix Studio Pro requires unique activity names (CE0495).
func deduplicateActivityNames(activities []workflows.WorkflowActivity, reserved ...string) {
	nameCount := make(map[string]int)
	// Names already held by activities outside this list — the main flow's
	// implicit Start and End — count as seen, so nothing in the list takes them.
	for _, name := range reserved {
		nameCount[name]++
	}
	// Two passes, jumps LAST.
	//
	// A jump is not a jump target (Mendix refuses that, CE6681), so it has no
	// claim on a name a real activity wants. Letting it compete in flow order is
	// how a FORWARD jump used to take its target's name and push the target to
	// <name>2 — leaving the jump pointing at itself even though the target
	// existed (mendixlabs/mxcli#1005). Naming jumps last means only jumps are
	// ever suffixed, whatever they are called and wherever they appear.
	deduplicateActivityNamesInFlow(activities, nameCount, false)
	deduplicateActivityNamesInFlow(activities, nameCount, true)
}

// deduplicateActivityNamesInFlow recursively deduplicates activity names. Both
// passes walk the whole tree; jumpPass selects which activities are renamed, so
// a jump nested in an outcome flow is still reached in the second pass.
func deduplicateActivityNamesInFlow(activities []workflows.WorkflowActivity, nameCount map[string]int, jumpPass bool) {
	for _, act := range activities {
		switch act.(type) {
		case *workflows.UserTask, *workflows.CallMicroflowTask, *workflows.CallWorkflowActivity,
			*workflows.ExclusiveSplitActivity, *workflows.ParallelSplitActivity,
			*workflows.WaitForTimerActivity, *workflows.WaitForNotificationActivity,
			*workflows.EndWorkflowActivity,
			*workflows.NotificationActivity, *workflows.EventSubProcessStartActivity,
			// The end-of-path markers carry names too, and Mendix holds them to the
			// same uniqueness rule: every path of every split used to be written as
			// "EndOfParallelSplitPath", which mxbuild refuses as CE0495.
			*workflows.EndOfParallelSplitPathActivity, *workflows.EndOfBoundaryEventPathActivity:
			if !jumpPass {
				act.SetName(uniqueName(act.GetName(), nameCount))
			}
		}
		// A notification boundary event's name is unique in the workflow too — it
		// is what a notify action targets.
		if !jumpPass {
			for _, be := range activityBoundaryEvents(act) {
				if be != nil && be.IsNotification() {
					be.Name = uniqueName(be.Name, nameCount)
				}
			}
		}
		switch act.(type) {
		case *workflows.JumpToActivity:
			if jumpPass {
				act.SetName(uniqueName(act.GetName(), nameCount))
			}
		}

		// Same nested-flow enumeration as the auto-bind walk, for the same
		// reason: this switch was missing the enum-outcome and boundary-event
		// branches too, so a name collision inside one went undetected.
		for _, f := range nestedFlows(act) {
			deduplicateActivityNamesInFlow(f.Activities, nameCount, jumpPass)
		}
	}
}

// uniqueName returns a unique name by appending a number if the name was seen before.
func uniqueName(name string, nameCount map[string]int) string {
	nameCount[name]++
	count := nameCount[name]
	if count == 1 {
		return name
	}
	return fmt.Sprintf("%s%d", name, count)
}

func buildAnnotationActivity(n *ast.WorkflowAnnotationActivityNode) *workflows.WorkflowAnnotationActivity {
	a := &workflows.WorkflowAnnotationActivity{}
	a.ID = model.ID(types.GenerateID())
	a.Description = n.Text
	return a
}

// sanitizeActivityName converts a display caption to a valid Mendix identifier.
// Mendix names must start with a letter/underscore and contain only letters, digits, underscores.
func sanitizeActivityName(name string) string {
	var b strings.Builder
	for i, r := range name {
		if unicode.IsLetter(r) || r == '_' {
			b.WriteRune(r)
		} else if unicode.IsDigit(r) && i > 0 {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' {
			b.WriteRune('_')
		}
	}
	result := b.String()
	if result == "" {
		return "activity"
	}
	return result
}

// activityBoundaryEvents returns the boundary events an activity carries.
func activityBoundaryEvents(act workflows.WorkflowActivity) []*workflows.BoundaryEvent {
	switch a := act.(type) {
	case *workflows.UserTask:
		return a.BoundaryEvents
	case *workflows.CallMicroflowTask:
		return a.BoundaryEvents
	case *workflows.CallWorkflowActivity:
		return a.BoundaryEvents
	case *workflows.WaitForNotificationActivity:
		return a.BoundaryEvents
	}
	return nil
}

// nestedFlows returns every flow nested inside a workflow activity: condition
// outcomes (a decision's and a call-microflow's), user-task outcomes, parallel
// split paths, and boundary-event bodies.
//
// It exists because both tree walks over a workflow — autoBindActivitiesInFlow
// and deduplicateActivityNamesInFlow — used to enumerate the nested flows
// themselves, with a type switch per outcome kind. Each switch was missing
// cases: auto-bind never entered an EnumerationValueConditionOutcome or any
// boundary event, so a `call microflow` inside a decision's enum branch reached
// Mendix with no parameter mappings and no outcomes (CE6685 + CE6686,
// ako/mxcli#417) while the identical activity in the MAIN flow was wired.
// Enumerating the flows in ONE place is what stops the next walk from
// re-acquiring the gap; the ConditionOutcome interface already exposes GetFlow,
// so no outcome kind can be silently skipped here.
func nestedFlows(act workflows.WorkflowActivity) []*workflows.Flow {
	var flows []*workflows.Flow
	add := func(f *workflows.Flow) {
		if f != nil {
			flows = append(flows, f)
		}
	}
	addConditions := func(outcomes []workflows.ConditionOutcome) {
		for _, o := range outcomes {
			if o != nil {
				add(o.GetFlow())
			}
		}
	}
	addBoundary := func(events []*workflows.BoundaryEvent) {
		for _, be := range events {
			if be != nil {
				add(be.Flow)
			}
		}
	}

	switch a := act.(type) {
	case *workflows.CallMicroflowTask:
		addConditions(a.Outcomes)
		addBoundary(a.BoundaryEvents)
	case *workflows.SystemTask:
		addConditions(a.Outcomes)
	case *workflows.ExclusiveSplitActivity:
		addConditions(a.Outcomes)
	case *workflows.UserTask:
		for _, o := range a.Outcomes {
			if o != nil {
				add(o.Flow)
			}
		}
		addBoundary(a.BoundaryEvents)
	case *workflows.ParallelSplitActivity:
		for _, o := range a.Outcomes {
			if o != nil {
				add(o.Flow)
			}
		}
	case *workflows.CallWorkflowActivity:
		addBoundary(a.BoundaryEvents)
	case *workflows.WaitForNotificationActivity:
		addBoundary(a.BoundaryEvents)
	}
	return flows
}

// autoBindWorkflowParameters resolves microflow/workflow parameters and generates
// ParameterMappings, default outcomes, and sanitized names for workflow activities.
// declaredContextVar is the variable name from the workflow header's
// `parameter $X:` clause, used to alias `$X` onto the stored context name.
func autoBindWorkflowParameters(ctx *ExecContext, activities []workflows.WorkflowActivity, declaredContextVar string) {
	autoBindActivitiesInFlow(ctx, activities, newContextExprNormalizer(declaredContextVar))
}

func autoBindActivitiesInFlow(ctx *ExecContext, activities []workflows.WorkflowActivity, norm contextExprNormalizer) {
	for _, act := range activities {
		switch a := act.(type) {
		case *workflows.CallMicroflowTask:
			autoBindCallMicroflow(ctx, a, norm)
		case *workflows.CallWorkflowActivity:
			autoBindCallWorkflow(ctx, a)
		case *workflows.UserTask:
			// Sanitize name
			a.Name = sanitizeActivityName(a.Name)
			a.DueDate = norm.rewrite(a.DueDate)
			if xp, ok := a.UserSource.(*workflows.XPathBasedUserSource); ok {
				xp.XPath = norm.rewrite(xp.XPath)
			}
		case *workflows.ParallelSplitActivity:
			// Sanitize name (spaces not allowed)
			a.Name = sanitizeActivityName(a.Name)
		case *workflows.ExclusiveSplitActivity:
			a.Name = sanitizeActivityName(a.Name)
			// A decision's condition is an expression over the workflow context,
			// and the only in-scope variable is that context. It was left verbatim
			// while call-microflow parameter mappings were normalized, so the
			// documented `$workflowContext` (and the user's own declared parameter
			// name) reached Mendix as undefined variables → CE0117 (issuetracker
			// #17). Normalize it the same way.
			a.Expression = norm.rewrite(a.Expression)
		case *workflows.WaitForNotificationActivity:
			a.Name = sanitizeActivityName(a.Name)
		case *workflows.WaitForTimerActivity:
			a.Name = sanitizeActivityName(a.Name)
			a.DelayExpression = norm.rewrite(a.DelayExpression)
		case *workflows.JumpToActivity:
			a.Name = sanitizeActivityName(a.Name)
		}

		// Every nested flow, whatever the activity — enumerating them here rather
		// than per case is what keeps a decision's enum branch and a boundary
		// event body from being skipped (ako/mxcli#417).
		for _, f := range nestedFlows(act) {
			autoBindActivitiesInFlow(ctx, f.Activities, norm)
		}
	}
}

// autoBindCallMicroflow resolves microflow parameters and auto-generates ParameterMappings.
// Ensures default outcomes that MATCH the target microflow's return type, which the
// Mendix 11.9+ CallMicroflowActivity requires (CE6686: "outcomes do not match the
// configured microflow"). The pre-11.9 CallMicroflowTask tolerated a lone
// VoidConditionOutcome regardless of return type, which is why this used to be
// hardcoded to Void (FINDINGS #39 regression).
func autoBindCallMicroflow(ctx *ExecContext, task *workflows.CallMicroflowTask, norm contextExprNormalizer) {
	// Sanitize name
	task.Name = sanitizeActivityName(task.Name)

	// Normalize the workflow-context variable in explicit parameter mappings to the
	// actual context parameter name ("WorkflowContext"). Mendix expressions are
	// case-sensitive on 11.9+, so a user-written `$workflowContext` is an undefined
	// variable → CE0117 (FINDINGS #39 regression). The pre-11.9 class did not flag it.
	for _, pm := range task.ParameterMappings {
		pm.Expression = norm.rewrite(pm.Expression)
	}

	// Look up the target microflow — needed both for return-type-matched outcomes
	// and for parameter auto-binding.
	var targetMF *microflows.Microflow
	if mfs, err := ctx.Backend.ListMicroflows(); err == nil {
		if h, err := getHierarchy(ctx); err == nil {
			for _, mf := range mfs {
				if h.GetModuleName(h.FindModuleID(mf.ContainerID))+"."+mf.Name == task.Microflow {
					targetMF = mf
					break
				}
			}
		}
	}

	// Auto-generate outcomes matching the microflow's return type when none given.
	if len(task.Outcomes) == 0 {
		task.Outcomes = defaultCallMicroflowOutcomes(targetMF)
	}

	// Auto-bind parameters (context) if no explicit mappings were given.
	if len(task.ParameterMappings) == 0 && targetMF != nil {
		for _, param := range targetMF.Parameters {
			mapping := &workflows.ParameterMapping{
				Parameter:  task.Microflow + "." + param.Name,
				Expression: "$WorkflowContext",
			}
			mapping.BaseElement.ID = model.ID(types.GenerateID())
			task.ParameterMappings = append(task.ParameterMappings, mapping)
		}
	}
}

// defaultCallMicroflowOutcomes builds the default outcome set for a call-microflow
// activity based on the target microflow's return type: Boolean → true/false
// branches; anything else (void, entity, string, …) → a single default outcome.
// Enumeration returns would need one outcome per value; until that is wired, they
// fall through to the single-default form (still a Void outcome).
func defaultCallMicroflowOutcomes(mf *microflows.Microflow) []workflows.ConditionOutcome {
	newFlow := func() *workflows.Flow {
		f := &workflows.Flow{}
		f.BaseElement.ID = model.ID(types.GenerateID())
		return f
	}
	if mf != nil && mf.ReturnType != nil && mf.ReturnType.GetTypeName() == "Boolean" {
		yes := &workflows.BooleanConditionOutcome{Value: true, Flow: newFlow()}
		yes.BaseElement.ID = model.ID(types.GenerateID())
		no := &workflows.BooleanConditionOutcome{Value: false, Flow: newFlow()}
		no.BaseElement.ID = model.ID(types.GenerateID())
		return []workflows.ConditionOutcome{yes, no}
	}
	o := &workflows.VoidConditionOutcome{Flow: newFlow()}
	o.BaseElement.ID = model.ID(types.GenerateID())
	return []workflows.ConditionOutcome{o}
}

// workflowContextVar is the name mxcli always gives the workflow context
// parameter. Mendix expressions are case-sensitive, so every reference to the
// context has to match it exactly or the build fails CE0117.
const workflowContextVar = "WorkflowContext"

// normalizeWorkflowContextExpr rewrites a case-insensitive `$workflowContext`
// reference to the exact context parameter name `$WorkflowContext`. In a workflow
// the only in-scope variable is the context, so this is unambiguous.
func normalizeWorkflowContextExpr(expr string) string {
	return workflowContextRe.ReplaceAllString(expr, "$$"+workflowContextVar)
}

var workflowContextRe = regexp.MustCompile(`(?i)\$workflowcontext`)

// contextExprNormalizer makes every way an author can name the workflow context
// resolve to the one name it is actually stored under.
//
// `create workflow … parameter $Ctx: Module.Entity` lets the author pick a
// variable name, but mxcli stores the parameter as `WorkflowContext` regardless,
// so `$Ctx` would reach Mendix as an undefined variable. The declared name is
// therefore aliased onto the real one, and casing is normalized on top
// (issuetracker #17).
type contextExprNormalizer struct{ alias *regexp.Regexp }

// newContextExprNormalizer builds a normalizer for the variable the author
// declared in the workflow header (with or without the `$` sigil; empty means
// "no alias, normalize casing only").
func newContextExprNormalizer(declared string) contextExprNormalizer {
	name := strings.TrimPrefix(declared, "$")
	if name == "" || strings.EqualFold(name, workflowContextVar) {
		return contextExprNormalizer{}
	}
	return contextExprNormalizer{alias: regexp.MustCompile(`(?i)\$` + regexp.QuoteMeta(name) + `\b`)}
}

// rewrite returns expr with every context reference spelled `$WorkflowContext`.
func (n contextExprNormalizer) rewrite(expr string) string {
	if expr == "" {
		return expr
	}
	if n.alias != nil {
		expr = n.alias.ReplaceAllString(expr, "$$"+workflowContextVar)
	}
	return normalizeWorkflowContextExpr(expr)
}

// autoBindCallWorkflow resolves workflow parameters and generates ParameterMappings.
func autoBindCallWorkflow(ctx *ExecContext, act *workflows.CallWorkflowActivity) {
	// Sanitize name
	act.Name = sanitizeActivityName(act.Name)

	// Skip if already has parameter mappings (explicit from MDL WITH clause)
	if len(act.ParameterMappings) > 0 {
		return
	}

	// Look up the target workflow to check its parameter
	wfs, err := ctx.Backend.ListWorkflows()
	if err != nil {
		return
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return
	}

	for _, wf := range wfs {
		modID := h.FindModuleID(wf.ContainerID)
		modName := h.GetModuleName(modID)
		qualifiedName := modName + "." + wf.Name
		if qualifiedName != act.Workflow {
			continue
		}

		// If the target workflow has a parameter, generate ParameterMappings
		if wf.Parameter != nil && wf.Parameter.EntityRef != "" {
			act.ParameterExpression = "$WorkflowContext"
			// Generate WorkflowCallParameterMapping: Parameter = Workflow.ParamName
			paramName := qualifiedName + ".WorkflowContext"
			mapping := &workflows.ParameterMapping{
				Parameter:  paramName,
				Expression: "$WorkflowContext",
			}
			mapping.BaseElement.ID = model.ID(types.GenerateID())
			act.ParameterMappings = append(act.ParameterMappings, mapping)
		}
		break
	}
}

// hasStandaloneWorkflowAnnotation reports whether any activity flow in the
// workflow (including nested outcome / path / boundary-event flows) contains a
// standalone `annotation` statement. See execCreateWorkflow for why it is
// refused rather than written.
func hasStandaloneWorkflowAnnotation(acts []ast.WorkflowActivityNode) bool {
	found := false
	walkWorkflowActivities(acts, func(a ast.WorkflowActivityNode) {
		if _, ok := a.(*ast.WorkflowAnnotationActivityNode); ok {
			found = true
		}
	})
	return found
}
