// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

func init() {
	// Flow.Activities and every Outcomes list serialize with the typed-array
	// marker 3 (populated case keyed by the leading child $Type). Register marker 3
	// for every activity and outcome $Type that can lead such a list.
	for _, t := range []string{
		"Workflows$SingleUserTaskActivity", "Workflows$MultiUserTaskActivity",
		"Workflows$CallMicroflowTask", "Workflows$CallWorkflowActivity",
		"Workflows$ExclusiveSplitActivity", "Workflows$ParallelSplitActivity",
		"Workflows$JumpToActivity", "Workflows$WaitForTimerActivity",
		"Workflows$WaitForNotificationActivity", "Workflows$StartWorkflowActivity",
		"Workflows$EndWorkflowActivity", "Workflows$Annotation",
		"Workflows$UserTaskOutcome", "Workflows$BooleanConditionOutcome",
		"Workflows$EnumerationValueConditionOutcome", "Workflows$VoidConditionOutcome",
		"Workflows$ParallelSplitOutcome",
	} {
		codec.RegisterListMarker(t, 3)
	}
	// BoundaryEvents and ParameterMappings serialize with marker 2.
	for _, t := range []string{
		"Workflows$TimerBoundaryEvent", "Workflows$InterruptingTimerBoundaryEvent",
		"Workflows$NonInterruptingTimerBoundaryEvent",
		"Workflows$MicroflowCallParameterMapping", "Workflows$WorkflowCallParameterMapping",
	} {
		codec.RegisterListMarker(t, 2)
	}

	// Mandatory empty-list markers + null PartProperties, keyed per $Type. Empty
	// marker-2 lists rely on these (an empty addPartList would default to marker 3).
	codec.RegisterTypeDefaults("Workflows$Workflow", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"OnWorkflowEvent": 2},
		NullFields:           []string{"WorkflowMetaData", "Annotation", "AdminPage"},
	})
	codec.RegisterTypeDefaults("Workflows$Flow", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Activities": 3},
	})
	for _, t := range []string{"Workflows$SingleUserTaskActivity", "Workflows$MultiUserTaskActivity"} {
		codec.RegisterTypeDefaults(t, codec.TypeDefaults{
			MandatoryListMarkers: map[string]int32{"Outcomes": 3, "BoundaryEvents": 2},
			NullFields:           []string{"Annotation"},
		})
	}
	codec.RegisterTypeDefaults("Workflows$CallMicroflowTask", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Outcomes": 3, "BoundaryEvents": 2, "ParameterMappings": 2},
		NullFields:           []string{"Annotation"},
	})
	codec.RegisterTypeDefaults("Workflows$CallWorkflowActivity", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"BoundaryEvents": 2, "ParameterMappings": 2},
		NullFields:           []string{"Annotation"},
	})
	codec.RegisterTypeDefaults("Workflows$ExclusiveSplitActivity", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Outcomes": 3},
		NullFields:           []string{"Annotation"},
	})
	codec.RegisterTypeDefaults("Workflows$ParallelSplitActivity", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Outcomes": 3},
		NullFields:           []string{"Annotation"},
	})
	codec.RegisterTypeDefaults("Workflows$WaitForNotificationActivity", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"BoundaryEvents": 2},
		NullFields:           []string{"Annotation"},
	})
	for _, t := range []string{
		"Workflows$JumpToActivity", "Workflows$WaitForTimerActivity",
		"Workflows$StartWorkflowActivity", "Workflows$EndWorkflowActivity",
	} {
		codec.RegisterTypeDefaults(t, codec.TypeDefaults{NullFields: []string{"Annotation"}})
	}
	codec.RegisterTypeDefaults("Workflows$NonInterruptingTimerBoundaryEvent", codec.TypeDefaults{
		NullFields: []string{"Recurrence"},
	})
	codec.RegisterTypeDefaults("Microflows$StringTemplate", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Parameters": 2},
	})
}

// CreateWorkflow inserts a new Workflows$Workflow document. Mirrors the legacy
// serializer field-for-field via direct-build helpers.
func (b *Backend) CreateWorkflow(wf *workflows.Workflow) error {
	if wf == nil {
		return fmt.Errorf("CreateWorkflow: nil workflow")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateWorkflow: not connected for writing")
	}
	if wf.ID == "" {
		wf.ID = model.ID(mmpr.GenerateID())
	}
	wf.TypeName = "Workflows$Workflow"
	contents, err := (&codec.Encoder{}).Encode(workflowToGen(wf))
	if err != nil {
		return fmt.Errorf("CreateWorkflow: encode: %w", err)
	}
	return b.writer.InsertUnit(string(wf.ID), string(wf.ContainerID), "Documents", "Workflows$Workflow", contents)
}

// UpdateWorkflow rewrites an existing workflow in place (CREATE OR MODIFY).
func (b *Backend) UpdateWorkflow(wf *workflows.Workflow) error {
	if wf == nil {
		return fmt.Errorf("UpdateWorkflow: nil workflow")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateWorkflow: not connected for writing")
	}
	wf.TypeName = "Workflows$Workflow"
	contents, err := (&codec.Encoder{}).Encode(workflowToGen(wf))
	if err != nil {
		return fmt.Errorf("UpdateWorkflow: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(wf.ID), contents)
}

// DeleteWorkflow removes a workflow unit by ID.
func (b *Backend) DeleteWorkflow(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteWorkflow: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

func workflowToGen(wf *workflows.Workflow) element.Element {
	g := newElem("Workflows$Workflow", string(wf.ID))
	if wf.AdminPage != "" {
		addPart(g, "AdminPage", pageReferenceElem(wf.AdminPage))
	}
	if wf.Annotation != "" {
		addPart(g, "Annotation", annotationElem(wf.Annotation))
	}
	addStr(g, "Documentation", wf.Documentation)
	addStr(g, "DueDate", wf.DueDate)
	addBool(g, "Excluded", wf.Excluded)
	addStr(g, "ExportLevel", "Hidden")
	flow := wf.Flow
	if flow == nil {
		flow = &workflows.Flow{}
	}
	addPart(g, "Flow", flowToGen(flow))
	addStr(g, "Name", wf.Name)
	// OnWorkflowEvent: empty marker-2 list (via MandatoryListMarkers).
	if wf.Parameter != nil {
		addPart(g, "Parameter", workflowParameterToGen(wf.Parameter))
	}
	addFreshPersistentID(g)
	title := wf.WorkflowName
	if title == "" {
		title = wf.Name
	}
	addStr(g, "Title", title)
	addPart(g, "WorkflowDescription", workflowStringTemplate(wf.WorkflowDescription))
	// WorkflowMetaData: null (via NullFields).
	addPart(g, "WorkflowName", workflowStringTemplate(wf.WorkflowName))
	addBool(g, "WorkflowV2", false)
	return g
}

func flowToGen(flow *workflows.Flow) element.Element {
	g := newElem("Workflows$Flow", string(flow.ID))
	acts := make([]element.Element, 0, len(flow.Activities))
	for _, a := range flow.Activities {
		if el := activityToGen(a); el != nil {
			acts = append(acts, el)
		}
	}
	if len(acts) > 0 {
		addPartList(g, "Activities", acts)
	}
	return g
}

func activityToGen(act workflows.WorkflowActivity) element.Element {
	switch a := act.(type) {
	case *workflows.UserTask:
		return userTaskToGen(a)
	case *workflows.CallMicroflowTask:
		return callMicroflowTaskToGen(a)
	case *workflows.CallWorkflowActivity:
		return callWorkflowActivityToGen(a)
	case *workflows.ExclusiveSplitActivity:
		return exclusiveSplitToGen(a)
	case *workflows.ParallelSplitActivity:
		return parallelSplitToGen(a)
	case *workflows.JumpToActivity:
		return jumpToToGen(a)
	case *workflows.WaitForTimerActivity:
		return waitForTimerToGen(a)
	case *workflows.WaitForNotificationActivity:
		return waitForNotificationToGen(a)
	case *workflows.StartWorkflowActivity:
		return simpleActivityToGen("Workflows$StartWorkflowActivity", &a.BaseWorkflowActivity)
	case *workflows.EndWorkflowActivity:
		return simpleActivityToGen("Workflows$EndWorkflowActivity", &a.BaseWorkflowActivity)
	case *workflows.WorkflowAnnotationActivity:
		return annotationActivityToGen(a)
	default:
		return nil
	}
}

func userTaskToGen(a *workflows.UserTask) element.Element {
	// UserTask → Single/MultiUserTaskActivity (UserTask was deleted in 10.12.0).
	typeName := "Workflows$SingleUserTaskActivity"
	if a.IsMulti {
		typeName = "Workflows$MultiUserTaskActivity"
	}
	g := newElem(typeName, activityID(&a.BaseWorkflowActivity))
	if a.Annotation != "" {
		addPart(g, "Annotation", annotationElem(a.Annotation))
	}
	addBool(g, "AutoAssignSingleTargetUser", false)
	if a.IsMulti {
		addBool(g, "AwaitAllUsers", false)
	}
	if len(a.BoundaryEvents) > 0 {
		addPartList(g, "BoundaryEvents", boundaryEventsToGen(a.BoundaryEvents))
	}
	addStr(g, "Caption", a.Caption)

	// Pre-assign the first outcome ID so CompletionCriteria can point at it.
	for _, o := range a.Outcomes {
		if o.ID == "" {
			o.ID = model.ID(mmpr.GenerateID())
		}
	}
	if a.IsMulti {
		fallbackID := mmpr.GenerateID()
		if len(a.Outcomes) > 0 {
			fallbackID = string(a.Outcomes[0].ID)
		}
		cc := newElem("Workflows$ConsensusCompletionCriteria", "")
		addIDRef(cc, "FallbackOutcomePointer", model.ID(fallbackID))
		addPart(g, "CompletionCriteria", cc)
	}
	addStr(g, "DueDate", a.DueDate)
	addStr(g, "Name", a.Name)
	addPart(g, "OnCreatedEvent", newElem("Workflows$NoEvent", ""))

	outcomes := make([]element.Element, 0, len(a.Outcomes))
	for _, o := range a.Outcomes {
		outcomes = append(outcomes, userTaskOutcomeToGen(o))
	}
	if len(outcomes) > 0 {
		addPartList(g, "Outcomes", outcomes)
	}
	addFreshPersistentID(g)
	addStr(g, "RelativeMiddlePoint", "")
	addStr(g, "Size", "")
	addPart(g, "TaskDescription", workflowStringTemplate(a.TaskDescription))
	taskName := a.TaskName
	if taskName == "" {
		taskName = a.Caption
	}
	addPart(g, "TaskName", workflowStringTemplate(taskName))
	addPart(g, "TaskPage", pageReferenceElem(a.Page))
	if a.IsMulti {
		addPart(g, "TargetUserInput", newElem("Workflows$AllUserInput", ""))
	}
	addPart(g, "UserTargeting", userTargetingToGen(a.UserSource))
	return g
}

func callMicroflowTaskToGen(a *workflows.CallMicroflowTask) element.Element {
	g := newElem("Workflows$CallMicroflowTask", activityID(&a.BaseWorkflowActivity))
	if a.Annotation != "" {
		addPart(g, "Annotation", annotationElem(a.Annotation))
	}
	if len(a.BoundaryEvents) > 0 {
		addPartList(g, "BoundaryEvents", boundaryEventsToGen(a.BoundaryEvents))
	}
	addStr(g, "Caption", a.Caption)
	addStr(g, "Microflow", a.Microflow)
	addStr(g, "Name", a.Name)

	outcomes := make([]element.Element, 0, len(a.Outcomes))
	for _, o := range a.Outcomes {
		if el := conditionOutcomeToGen(o); el != nil {
			outcomes = append(outcomes, el)
		}
	}
	if len(outcomes) > 0 {
		addPartList(g, "Outcomes", outcomes)
	}
	if mappings := parameterMappingsToGen("Workflows$MicroflowCallParameterMapping", a.ParameterMappings); len(mappings) > 0 {
		addPartList(g, "ParameterMappings", mappings)
	}
	addFreshPersistentID(g)
	addStr(g, "RelativeMiddlePoint", "")
	addStr(g, "Size", "")
	return g
}

func callWorkflowActivityToGen(a *workflows.CallWorkflowActivity) element.Element {
	g := newElem("Workflows$CallWorkflowActivity", activityID(&a.BaseWorkflowActivity))
	if a.Annotation != "" {
		addPart(g, "Annotation", annotationElem(a.Annotation))
	}
	if len(a.BoundaryEvents) > 0 {
		addPartList(g, "BoundaryEvents", boundaryEventsToGen(a.BoundaryEvents))
	}
	addStr(g, "Caption", a.Caption)
	addBool(g, "ExecuteAsync", false)
	addStr(g, "Name", a.Name)
	if mappings := parameterMappingsToGen("Workflows$WorkflowCallParameterMapping", a.ParameterMappings); len(mappings) > 0 {
		addPartList(g, "ParameterMappings", mappings)
	}
	addFreshPersistentID(g)
	addStr(g, "RelativeMiddlePoint", "")
	addStr(g, "Size", "")
	addStr(g, "Workflow", a.Workflow)
	return g
}

func exclusiveSplitToGen(a *workflows.ExclusiveSplitActivity) element.Element {
	g := newElem("Workflows$ExclusiveSplitActivity", activityID(&a.BaseWorkflowActivity))
	addActivityBaseFields(g, a.Annotation)
	addStr(g, "Caption", a.Caption)
	addStr(g, "Expression", a.Expression)
	addStr(g, "Name", a.Name)
	outcomes := make([]element.Element, 0, len(a.Outcomes))
	for _, o := range a.Outcomes {
		if el := conditionOutcomeToGen(o); el != nil {
			outcomes = append(outcomes, el)
		}
	}
	if len(outcomes) > 0 {
		addPartList(g, "Outcomes", outcomes)
	}
	return g
}

func parallelSplitToGen(a *workflows.ParallelSplitActivity) element.Element {
	g := newElem("Workflows$ParallelSplitActivity", activityID(&a.BaseWorkflowActivity))
	addActivityBaseFields(g, a.Annotation)
	addStr(g, "Caption", a.Caption)
	addStr(g, "Name", a.Name)
	outcomes := make([]element.Element, 0, len(a.Outcomes))
	for _, o := range a.Outcomes {
		oc := newElem("Workflows$ParallelSplitOutcome", string(o.ID))
		if o.Flow != nil {
			addPart(oc, "Flow", flowToGen(o.Flow))
		}
		addFreshPersistentID(oc)
		outcomes = append(outcomes, oc)
	}
	if len(outcomes) > 0 {
		addPartList(g, "Outcomes", outcomes)
	}
	return g
}

func jumpToToGen(a *workflows.JumpToActivity) element.Element {
	g := newElem("Workflows$JumpToActivity", activityID(&a.BaseWorkflowActivity))
	addActivityBaseFields(g, a.Annotation)
	addStr(g, "Caption", a.Caption)
	addStr(g, "Name", a.Name)
	addStr(g, "TargetActivity", a.TargetActivity)
	return g
}

func waitForTimerToGen(a *workflows.WaitForTimerActivity) element.Element {
	g := newElem("Workflows$WaitForTimerActivity", activityID(&a.BaseWorkflowActivity))
	addActivityBaseFields(g, a.Annotation)
	addStr(g, "Caption", a.Caption)
	addStr(g, "Delay", a.DelayExpression)
	addStr(g, "Name", a.Name)
	return g
}

func waitForNotificationToGen(a *workflows.WaitForNotificationActivity) element.Element {
	g := newElem("Workflows$WaitForNotificationActivity", activityID(&a.BaseWorkflowActivity))
	if a.Annotation != "" {
		addPart(g, "Annotation", annotationElem(a.Annotation))
	}
	if len(a.BoundaryEvents) > 0 {
		addPartList(g, "BoundaryEvents", boundaryEventsToGen(a.BoundaryEvents))
	}
	addStr(g, "Caption", a.Caption)
	addStr(g, "Name", a.Name)
	addFreshPersistentID(g)
	addStr(g, "RelativeMiddlePoint", "")
	addStr(g, "Size", "")
	return g
}

// simpleActivityToGen handles StartWorkflow/EndWorkflow (caption + name + base).
func simpleActivityToGen(typeName string, a *workflows.BaseWorkflowActivity) element.Element {
	g := newElem(typeName, activityID(a))
	addActivityBaseFields(g, a.Annotation)
	addStr(g, "Caption", a.Caption)
	addStr(g, "Name", a.Name)
	return g
}

func annotationActivityToGen(a *workflows.WorkflowAnnotationActivity) element.Element {
	g := newElem("Workflows$Annotation", activityID(&a.BaseWorkflowActivity))
	addStr(g, "Description", a.Description)
	addFreshPersistentID(g)
	addStr(g, "RelativeMiddlePoint", "")
	addStr(g, "Size", "")
	return g
}

func userTaskOutcomeToGen(o *workflows.UserTaskOutcome) element.Element {
	g := newElem("Workflows$UserTaskOutcome", string(o.ID))
	if o.Flow != nil {
		addPart(g, "Flow", flowToGen(o.Flow))
	}
	addFreshPersistentID(g)
	addStr(g, "Value", o.Value)
	return g
}

func conditionOutcomeToGen(outcome workflows.ConditionOutcome) element.Element {
	switch o := outcome.(type) {
	case *workflows.BooleanConditionOutcome:
		g := newElem("Workflows$BooleanConditionOutcome", outcomeID(o.ID))
		addBool(g, "Value", o.Value)
		if o.Flow != nil {
			addPart(g, "Flow", flowToGen(o.Flow))
		}
		return g
	case *workflows.EnumerationValueConditionOutcome:
		g := newElem("Workflows$EnumerationValueConditionOutcome", outcomeID(o.ID))
		addStr(g, "Value", o.Value)
		if o.Flow != nil {
			addPart(g, "Flow", flowToGen(o.Flow))
		}
		return g
	case *workflows.VoidConditionOutcome:
		g := newElem("Workflows$VoidConditionOutcome", outcomeID(o.ID))
		if o.Flow != nil {
			addPart(g, "Flow", flowToGen(o.Flow))
		}
		return g
	default:
		return nil
	}
}

func boundaryEventsToGen(events []*workflows.BoundaryEvent) []element.Element {
	out := make([]element.Element, 0, len(events))
	for _, ev := range events {
		typeName := "Workflows$InterruptingTimerBoundaryEvent"
		switch ev.EventType {
		case "NonInterruptingTimer":
			typeName = "Workflows$NonInterruptingTimerBoundaryEvent"
		case "Timer":
			typeName = "Workflows$TimerBoundaryEvent"
		}
		g := newElem(typeName, activityIDOrFresh(string(ev.ID)))
		addStr(g, "Caption", ev.Caption)
		if ev.TimerDelay != "" {
			addStr(g, "FirstExecutionTime", ev.TimerDelay)
		}
		if ev.Flow != nil {
			addPart(g, "Flow", flowToGen(ev.Flow))
		}
		addFreshPersistentID(g)
		// Recurrence: null on NonInterrupting (via NullFields).
		out = append(out, g)
	}
	return out
}

func parameterMappingsToGen(typeName string, mappings []*workflows.ParameterMapping) []element.Element {
	out := make([]element.Element, 0, len(mappings))
	for _, pm := range mappings {
		g := newElem(typeName, activityIDOrFresh(string(pm.ID)))
		addStr(g, "Expression", pm.Expression)
		addStr(g, "Parameter", pm.Parameter)
		out = append(out, g)
	}
	return out
}

func userTargetingToGen(source workflows.UserSource) element.Element {
	switch s := source.(type) {
	case *workflows.MicroflowBasedUserSource:
		g := newElem("Workflows$MicroflowUserTargeting", "")
		addStr(g, "Microflow", s.Microflow)
		return g
	case *workflows.XPathBasedUserSource:
		g := newElem("Workflows$XPathUserTargeting", "")
		addStr(g, "XPathConstraint", s.XPath)
		return g
	case *workflows.MicroflowGroupSource:
		g := newElem("Workflows$MicroflowGroupTargeting", "")
		addStr(g, "Microflow", s.Microflow)
		return g
	case *workflows.XPathGroupSource:
		g := newElem("Workflows$XPathGroupTargeting", "")
		addStr(g, "XPathConstraint", s.XPath)
		return g
	default:
		return newElem("Workflows$NoUserTargeting", "")
	}
}

func workflowParameterToGen(param *workflows.WorkflowParameter) element.Element {
	g := newElem("Workflows$Parameter", activityIDOrFresh(string(param.ID)))
	addStr(g, "Entity", param.EntityRef)
	addStr(g, "Name", "WorkflowContext")
	return g
}

// pageReferenceElem builds a Workflows$PageReference (the by-name Page is a string).
func pageReferenceElem(page string) element.Element {
	g := newElem("Workflows$PageReference", "")
	addStr(g, "Page", page)
	return g
}

func annotationElem(text string) element.Element {
	g := newElem("Workflows$Annotation", "")
	addStr(g, "Description", text)
	return g
}

// workflowStringTemplate builds a minimal Microflows$StringTemplate (empty
// Parameters list via MandatoryListMarkers + Text).
func workflowStringTemplate(text string) element.Element {
	g := newElem("Microflows$StringTemplate", "")
	addStr(g, "Text", text)
	return g
}

// addActivityBaseFields adds the common Annotation(null)/PersistentId/
// RelativeMiddlePoint/Size fields shared by the simpler activity types.
func addActivityBaseFields(g *element.Base, annotation string) {
	if annotation != "" {
		addPart(g, "Annotation", annotationElem(annotation))
	}
	addFreshPersistentID(g)
	addStr(g, "RelativeMiddlePoint", "")
	addStr(g, "Size", "")
}

// addFreshPersistentID emits PersistentId as a fresh binary-UUID value (the
// legacy serializer writes a new GUID on every save).
func addFreshPersistentID(g *element.Base) {
	addIDRef(g, "PersistentId", model.ID(mmpr.GenerateID()))
}

func activityID(a *workflows.BaseWorkflowActivity) string {
	return activityIDOrFresh(string(a.ID))
}

func activityIDOrFresh(id string) string {
	if id != "" {
		return id
	}
	return mmpr.GenerateID()
}

func outcomeID(id model.ID) string {
	return activityIDOrFresh(string(id))
}
