// SPDX-License-Identifier: Apache-2.0

// Package executor - CREATE/DROP WORKFLOW commands
package executor

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// execCreateWorkflow handles CREATE WORKFLOW statements.
func execCreateWorkflow(ctx *ExecContext, s *ast.CreateWorkflowStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
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
	for _, existing := range existingWorkflows {
		modID := h.FindModuleID(existing.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == s.Name.Module && existing.Name == s.Name.Name {
			if !s.CreateOrModify {
				return mdlerrors.NewAlreadyExistsMsg("workflow", s.Name.Module+"."+s.Name.Name, "workflow '"+s.Name.Module+"."+s.Name.Name+"' already exists (use CREATE OR REPLACE to overwrite)")
			}
			existingID = existing.ID
			break
		}
	}

	wf := &workflows.Workflow{}
	wf.ContainerID = module.ID
	wf.Name = s.Name.Name
	wf.Documentation = s.Documentation

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
	autoBindWorkflowParameters(ctx, userActivities)

	// Deduplicate activity names to avoid CE0495
	deduplicateActivityNames(userActivities)

	// Compose: start + user activities + end
	flow.Activities = make([]workflows.WorkflowActivity, 0, len(userActivities)+2)
	flow.Activities = append(flow.Activities, startAct)
	flow.Activities = append(flow.Activities, userActivities...)
	flow.Activities = append(flow.Activities, endAct)

	wf.Flow = flow

	if existingID != "" {
		// Delete existing and recreate
		if err := ctx.Backend.DeleteWorkflow(existingID); err != nil {
			return mdlerrors.NewBackend("delete existing workflow", err)
		}
	}

	if err := ctx.Backend.CreateWorkflow(wf); err != nil {
		return mdlerrors.NewBackend("create workflow", err)
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
		}
		event.ID = model.ID(generateWorkflowUUID())
		if len(be.Activities) > 0 {
			event.Flow = &workflows.Flow{
				Activities: buildWorkflowActivities(be.Activities),
			}
			event.Flow.ID = model.ID(generateWorkflowUUID())
		}
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
	case *ast.WorkflowEndNode:
		return buildEndWorkflow(n)
	case *ast.WorkflowAnnotationActivityNode:
		return buildAnnotationActivity(n)
	default:
		return nil
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

	if n.Page.Module != "" {
		task.Page = n.Page.Module + "." + n.Page.Name
	}

	if n.Entity.Module != "" {
		task.UserTaskEntity = n.Entity.Module + "." + n.Entity.Name
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
	task := &workflows.CallMicroflowTask{}
	task.ID = model.ID(generateWorkflowUUID())
	task.Name = n.Microflow.Name
	task.Caption = n.Caption
	task.Microflow = n.Microflow.Module + "." + n.Microflow.Name

	if task.Caption == "" {
		task.Caption = task.Name
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
	act.Expression = n.Expression
	act.Caption = n.Caption

	if act.Caption == "" {
		act.Caption = "Decision"
	}
	act.Name = act.Caption

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

func buildParallelSplit(n *ast.WorkflowParallelSplitNode) *workflows.ParallelSplitActivity {
	act := &workflows.ParallelSplitActivity{}
	act.ID = model.ID(generateWorkflowUUID())
	act.Caption = n.Caption
	if act.Caption == "" {
		act.Caption = "Parallel split"
	}
	act.Name = act.Caption

	for _, pathNode := range n.Paths {
		outcome := &workflows.ParallelSplitOutcome{}
		outcome.ID = model.ID(generateWorkflowUUID())
		if len(pathNode.Activities) > 0 {
			outcome.Flow = &workflows.Flow{
				Activities: buildWorkflowActivities(pathNode.Activities),
			}
			outcome.Flow.ID = model.ID(generateWorkflowUUID())
		}
		act.Outcomes = append(act.Outcomes, outcome)
	}

	return act
}

func buildJumpTo(n *ast.WorkflowJumpToNode) *workflows.JumpToActivity {
	act := &workflows.JumpToActivity{}
	act.ID = model.ID(generateWorkflowUUID())
	act.Name = n.Target
	act.Caption = n.Caption
	act.TargetActivity = n.Target

	if act.Caption == "" {
		act.Caption = act.Name
	}

	return act
}

func buildWaitForTimer(n *ast.WorkflowWaitForTimerNode) *workflows.WaitForTimerActivity {
	act := &workflows.WaitForTimerActivity{}
	act.ID = model.ID(generateWorkflowUUID())
	act.DelayExpression = n.DelayExpression
	act.Caption = n.Caption

	if act.Caption == "" {
		act.Caption = "Wait for timer"
	}
	act.Name = act.Caption

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

	// BoundaryEvents (Issue #7)
	act.BoundaryEvents = buildBoundaryEvents(n.BoundaryEvents)

	return act
}

func buildEndWorkflow(n *ast.WorkflowEndNode) *workflows.EndWorkflowActivity {
	act := &workflows.EndWorkflowActivity{}
	act.ID = model.ID(generateWorkflowUUID())
	act.Caption = n.Caption

	if act.Caption == "" {
		act.Caption = "End"
	}
	act.Name = act.Caption

	return act
}

// deduplicateActivityNames ensures all activity names within a workflow are unique.
// Mendix Studio Pro requires unique activity names (CE0495).
func deduplicateActivityNames(activities []workflows.WorkflowActivity) {
	nameCount := make(map[string]int)
	deduplicateActivityNamesInFlow(activities, nameCount)
}

// deduplicateActivityNamesInFlow recursively deduplicates activity names.
func deduplicateActivityNamesInFlow(activities []workflows.WorkflowActivity, nameCount map[string]int) {
	for _, act := range activities {
		switch a := act.(type) {
		case *workflows.UserTask:
			a.Name = uniqueName(a.Name, nameCount)
			for _, outcome := range a.Outcomes {
				if outcome.Flow != nil {
					deduplicateActivityNamesInFlow(outcome.Flow.Activities, nameCount)
				}
			}
		case *workflows.CallMicroflowTask:
			a.Name = uniqueName(a.Name, nameCount)
			for _, outcome := range a.Outcomes {
				switch o := outcome.(type) {
				case *workflows.BooleanConditionOutcome:
					if o.Flow != nil {
						deduplicateActivityNamesInFlow(o.Flow.Activities, nameCount)
					}
				case *workflows.EnumerationValueConditionOutcome:
					if o.Flow != nil {
						deduplicateActivityNamesInFlow(o.Flow.Activities, nameCount)
					}
				case *workflows.VoidConditionOutcome:
					if o.Flow != nil {
						deduplicateActivityNamesInFlow(o.Flow.Activities, nameCount)
					}
				}
			}
		case *workflows.CallWorkflowActivity:
			a.Name = uniqueName(a.Name, nameCount)
		case *workflows.ExclusiveSplitActivity:
			a.Name = uniqueName(a.Name, nameCount)
			for _, outcome := range a.Outcomes {
				switch o := outcome.(type) {
				case *workflows.BooleanConditionOutcome:
					if o.Flow != nil {
						deduplicateActivityNamesInFlow(o.Flow.Activities, nameCount)
					}
				case *workflows.EnumerationValueConditionOutcome:
					if o.Flow != nil {
						deduplicateActivityNamesInFlow(o.Flow.Activities, nameCount)
					}
				case *workflows.VoidConditionOutcome:
					if o.Flow != nil {
						deduplicateActivityNamesInFlow(o.Flow.Activities, nameCount)
					}
				}
			}
		case *workflows.ParallelSplitActivity:
			a.Name = uniqueName(a.Name, nameCount)
			for _, outcome := range a.Outcomes {
				if outcome.Flow != nil {
					deduplicateActivityNamesInFlow(outcome.Flow.Activities, nameCount)
				}
			}
		case *workflows.JumpToActivity:
			a.Name = uniqueName(a.Name, nameCount)
		case *workflows.WaitForTimerActivity:
			a.Name = uniqueName(a.Name, nameCount)
		case *workflows.WaitForNotificationActivity:
			a.Name = uniqueName(a.Name, nameCount)
		case *workflows.EndWorkflowActivity:
			a.Name = uniqueName(a.Name, nameCount)
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

// autoBindWorkflowParameters resolves microflow/workflow parameters and generates
// ParameterMappings, default outcomes, and sanitized names for workflow activities.
func autoBindWorkflowParameters(ctx *ExecContext, activities []workflows.WorkflowActivity) {
	autoBindActivitiesInFlow(ctx, activities)
}

func autoBindActivitiesInFlow(ctx *ExecContext, activities []workflows.WorkflowActivity) {
	for _, act := range activities {
		switch a := act.(type) {
		case *workflows.CallMicroflowTask:
			autoBindCallMicroflow(ctx, a)
			// Recurse into outcomes
			for _, outcome := range a.Outcomes {
				switch o := outcome.(type) {
				case *workflows.BooleanConditionOutcome:
					if o.Flow != nil {
						autoBindActivitiesInFlow(ctx, o.Flow.Activities)
					}
				case *workflows.VoidConditionOutcome:
					if o.Flow != nil {
						autoBindActivitiesInFlow(ctx, o.Flow.Activities)
					}
				}
			}
		case *workflows.CallWorkflowActivity:
			autoBindCallWorkflow(ctx, a)
		case *workflows.UserTask:
			// Sanitize name
			a.Name = sanitizeActivityName(a.Name)
			for _, outcome := range a.Outcomes {
				if outcome.Flow != nil {
					autoBindActivitiesInFlow(ctx, outcome.Flow.Activities)
				}
			}
		case *workflows.ParallelSplitActivity:
			// Sanitize name (spaces not allowed)
			a.Name = sanitizeActivityName(a.Name)
			for _, outcome := range a.Outcomes {
				if outcome.Flow != nil {
					autoBindActivitiesInFlow(ctx, outcome.Flow.Activities)
				}
			}
		case *workflows.ExclusiveSplitActivity:
			a.Name = sanitizeActivityName(a.Name)
			for _, outcome := range a.Outcomes {
				switch o := outcome.(type) {
				case *workflows.BooleanConditionOutcome:
					if o.Flow != nil {
						autoBindActivitiesInFlow(ctx, o.Flow.Activities)
					}
				case *workflows.VoidConditionOutcome:
					if o.Flow != nil {
						autoBindActivitiesInFlow(ctx, o.Flow.Activities)
					}
				}
			}
		case *workflows.WaitForNotificationActivity:
			a.Name = sanitizeActivityName(a.Name)
		case *workflows.WaitForTimerActivity:
			a.Name = sanitizeActivityName(a.Name)
		case *workflows.JumpToActivity:
			a.Name = sanitizeActivityName(a.Name)
		}
	}
}

// autoBindCallMicroflow resolves microflow parameters and auto-generates ParameterMappings.
func autoBindCallMicroflow(ctx *ExecContext, task *workflows.CallMicroflowTask) {
	// Sanitize name
	task.Name = sanitizeActivityName(task.Name)

	// Skip if already has parameter mappings
	if len(task.ParameterMappings) > 0 {
		return
	}

	// Look up the microflow to get its parameters
	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return
	}

	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		qualifiedName := modName + "." + mf.Name
		if qualifiedName != task.Microflow {
			continue
		}

		// Found the microflow — bind parameters
		for _, param := range mf.Parameters {
			paramQualifiedName := qualifiedName + "." + param.Name
			mapping := &workflows.ParameterMapping{
				Parameter:  paramQualifiedName,
				Expression: "$WorkflowContext",
			}
			mapping.BaseElement.ID = model.ID(types.GenerateID())
			task.ParameterMappings = append(task.ParameterMappings, mapping)
		}

		// Auto-generate Default outcome if no outcomes specified
		if len(task.Outcomes) == 0 {
			outcome := &workflows.VoidConditionOutcome{
				Flow: &workflows.Flow{},
			}
			outcome.BaseElement.ID = model.ID(types.GenerateID())
			outcome.Flow.BaseElement.ID = model.ID(types.GenerateID())
			task.Outcomes = append(task.Outcomes, outcome)
		}
		break
	}
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
