// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

func init() {
	// Flow.Activities and every Outcomes list serialize with the typed-array
	// marker 3 (populated case keyed by the leading child $Type). Register marker 3
	// for every activity and outcome $Type that can lead such a list.
	for _, t := range []string{
		"Workflows$SingleUserTaskActivity", "Workflows$MultiUserTaskActivity",
		"Workflows$CallMicroflowTask", "Workflows$CallMicroflowActivity",
		"Workflows$AIAgentTaskActivity",
		"Workflows$CallWorkflowActivity",
		"Workflows$ExclusiveSplitActivity", "Workflows$ParallelSplitActivity",
		"Workflows$JumpToActivity", "Workflows$WaitForTimerActivity",
		"Workflows$WaitForNotificationActivity", "Workflows$StartWorkflowActivity",
		"Workflows$EndWorkflowActivity", "Workflows$Annotation",
		"Workflows$EndOfParallelSplitPathActivity", "Workflows$EndOfBoundaryEventPathActivity",
		"Workflows$UserTaskOutcome", "Workflows$BooleanConditionOutcome",
		"Workflows$EnumerationValueConditionOutcome", "Workflows$VoidConditionOutcome",
		"Workflows$ParallelSplitOutcome",
		// Measured on ako/TestApp (11.14.0): a notification activity leads a flow
		// like any other, and a start activity leads every event sub-process flow.
		"Workflows$NotificationActivity",
		"Workflows$InterruptingNotificationEventSubProcessStartActivity",
		"Workflows$NonInterruptingNotificationEventSubProcessStartActivity",
		"Workflows$InterruptingTimerEventSubProcessStartActivity",
		"Workflows$NonInterruptingTimerEventSubProcessStartActivity",
	} {
		codec.RegisterListMarker(t, 3)
	}
	// BoundaryEvents, ParameterMappings and EventSubProcesses serialize with marker 2.
	for _, t := range []string{
		"Workflows$TimerBoundaryEvent", "Workflows$InterruptingTimerBoundaryEvent",
		"Workflows$NonInterruptingTimerBoundaryEvent",
		"Workflows$InterruptingNotificationBoundaryEvent", "Workflows$NonInterruptingNotificationBoundaryEvent",
		"Workflows$MicroflowCallParameterMapping", "Workflows$WorkflowCallParameterMapping",
		"Workflows$WorkflowEventHandler",
		"Workflows$EventSubProcess",
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
	// Both the pre-11.9 CallMicroflowTask and the 11.9+ CallMicroflowActivity
	// storage names share the same shape (see applyCallMicroflowStorageName), and
	// so does the 11.9+ AIAgentTaskActivity (ako/TestApp, 11.14.0).
	for _, t := range []string{"Workflows$CallMicroflowTask", "Workflows$CallMicroflowActivity", "Workflows$AIAgentTaskActivity"} {
		codec.RegisterTypeDefaults(t, codec.TypeDefaults{
			MandatoryListMarkers: map[string]int32{"Outcomes": 3, "BoundaryEvents": 2, "ParameterMappings": 2},
			NullFields:           []string{"Annotation"},
		})
	}
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
		"Workflows$EndOfParallelSplitPathActivity", "Workflows$EndOfBoundaryEventPathActivity",
		// Each stores Annotation: null (ako/TestApp, 11.14.0).
		"Workflows$NotificationActivity", "Workflows$EventSubProcess",
		"Workflows$InterruptingNotificationEventSubProcessStartActivity",
		"Workflows$NonInterruptingNotificationEventSubProcessStartActivity",
		"Workflows$InterruptingTimerEventSubProcessStartActivity",
		"Workflows$NonInterruptingTimerEventSubProcessStartActivity",
		"Workflows$InterruptingNotificationBoundaryEvent", "Workflows$NonInterruptingNotificationBoundaryEvent",
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

// Workflow "call microflow" activity storage names. Mendix 11.9 (WOR-2802) split
// MicroflowBasedActivity into CallMicroflowActivity + AIAgentTaskActivity, renaming
// the on-disk $Type from the older CallMicroflowTask. Writing the pre-11.9 name to
// an 11.9+ project makes the runtime fail to load the *entire* model with
// "Class 'Workflows$CallMicroflowTask' could not be found" — both checkers pass, so
// the failure only surfaces at boot (FINDINGS #39). The semantic model uses one
// activity type; only the emitted $Type differs, so we build with the legacy name
// and rewrite the tree here when targeting 11.9+.
const (
	callMicroflowTaskType     = "Workflows$CallMicroflowTask"
	callMicroflowActivityType = "Workflows$CallMicroflowActivity"
	aiAgentTaskActivityType   = "Workflows$AIAgentTaskActivity"
)

// useCallMicroflowActivityName reports whether the target project is Mendix 11.9+
// and therefore expects the CallMicroflowActivity storage name.
func (b *Backend) useCallMicroflowActivityName() bool {
	pv := b.ProjectVersion()
	return pv != nil && pv.IsAtLeast(11, 9)
}

// applyCallMicroflowStorageName rewrites every CallMicroflowTask $Type in the tree
// to the 11.9+ CallMicroflowActivity name when useActivity is set. No-op otherwise.
func applyCallMicroflowStorageName(root element.Element, useActivity bool) {
	if !useActivity || root == nil {
		return
	}
	element.Walk(root, func(e element.Element) bool {
		if e.TypeName() == callMicroflowTaskType {
			if s, ok := e.(interface{ SetTypeName(string) }); ok {
				s.SetTypeName(callMicroflowActivityType)
			}
		}
		return true
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
	g := workflowToGen(wf)
	applyCallMicroflowStorageName(g, b.useCallMicroflowActivityName())
	contents, err := (&codec.Encoder{}).Encode(g)
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
	g := workflowToGen(wf)
	applyCallMicroflowStorageName(g, b.useCallMicroflowActivityName())
	contents, err := (&codec.Encoder{}).Encode(g)
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
	// EventSubProcesses: a marker-2 list. Studio Pro 11.14 writes it empty too,
	// but the property only exists from 11.8, so an empty one is not invented.
	if len(wf.EventSubProcesses) > 0 {
		esps := make([]element.Element, 0, len(wf.EventSubProcesses))
		for _, esp := range wf.EventSubProcesses {
			esps = append(esps, eventSubProcessToGen(esp))
		}
		addPartList(g, "EventSubProcesses", esps)
	}
	addBool(g, "Excluded", wf.Excluded)
	addStr(g, "ExportLevel", "Hidden")
	flow := wf.Flow
	if flow == nil {
		flow = &workflows.Flow{}
	}
	addPart(g, "Flow", flowToGen(flow))
	addStr(g, "Name", wf.Name)
	// OnWorkflowEvent: a marker-2 list; empty via MandatoryListMarkers.
	if len(wf.EventHandlers) > 0 {
		handlers := make([]element.Element, 0, len(wf.EventHandlers))
		for _, h := range wf.EventHandlers {
			handlers = append(handlers, workflowEventHandlerToGen(h))
		}
		addPartList(g, "OnWorkflowEvent", handlers)
	}
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

// workflowEventHandlerToGen builds a Workflows$WorkflowEventHandler in the key
// order ako/TestApp (11.14.0) stores: Description, Documentation, EventTypes (a
// marker-1 string list), MicroflowEventHandler.
func workflowEventHandlerToGen(h *workflows.WorkflowEventHandler) element.Element {
	g := newElem("Workflows$WorkflowEventHandler", string(h.ID))
	addStr(g, "Description", h.Description)
	addStr(g, "Documentation", h.Documentation)
	addStrList(g, "EventTypes", h.EventTypes)
	mh := newElem("Workflows$MicroflowEventHandler", "")
	addStr(mh, "Microflow", h.Microflow)
	addPart(g, "MicroflowEventHandler", mh)
	return g
}

// onCreatedEventToGen builds a user task's OnCreatedEvent: the microflow when one
// is set, the NoEvent marker otherwise.
func onCreatedEventToGen(microflow string) element.Element {
	if microflow == "" {
		return newElem("Workflows$NoEvent", "")
	}
	ev := newElem("Workflows$MicroflowBasedEvent", "")
	addStr(ev, "Microflow", microflow)
	return ev
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
	case *workflows.EndOfParallelSplitPathActivity:
		return simpleActivityToGen("Workflows$EndOfParallelSplitPathActivity", &a.BaseWorkflowActivity)
	case *workflows.EndOfBoundaryEventPathActivity:
		return simpleActivityToGen("Workflows$EndOfBoundaryEventPathActivity", &a.BaseWorkflowActivity)
	case *workflows.WorkflowAnnotationActivity:
		return annotationActivityToGen(a)
	case *workflows.NotificationActivity:
		return simpleActivityToGen("Workflows$NotificationActivity", &a.BaseWorkflowActivity)
	case *workflows.EventSubProcessStartActivity:
		g := newElem(a.StorageType(), activityID(&a.BaseWorkflowActivity))
		addActivityBaseFields(g, a.Annotation)
		addStr(g, "Caption", a.Caption)
		if a.Timer {
			addStr(g, "FirstExecutionTime", a.FirstExecutionTime)
		}
		addStr(g, "Name", a.Name)
		return g
	default:
		return nil
	}
}

// eventSubProcessToGen builds a Workflows$EventSubProcess in the key order
// ako/TestApp (11.14.0) stores: Annotation, Caption, Flow, Name, PersistentId.
func eventSubProcessToGen(esp *workflows.EventSubProcess) element.Element {
	g := newElem("Workflows$EventSubProcess", activityIDOrFresh(string(esp.ID)))
	if esp.Annotation != "" {
		addPart(g, "Annotation", annotationElem(esp.Annotation))
	}
	addStr(g, "Caption", esp.Caption)
	flow := esp.Flow
	if flow == nil {
		flow = &workflows.Flow{}
	}
	addPart(g, "Flow", flowToGen(flow))
	addStr(g, "Name", esp.Name)
	addFreshPersistentID(g)
	return g
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
		addBool(g, "AwaitAllUsers", a.AwaitAllUsers)
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
		addPart(g, "CompletionCriteria", completionCriteriaToGen(a))
	}
	addStr(g, "DueDate", a.DueDate)
	addStr(g, "Name", a.Name)
	addPart(g, "OnCreatedEvent", onCreatedEventToGen(a.OnCreated))

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
		addPart(g, "TargetUserInput", targetUserInputToGen(a.TargetUserInput))
	}
	addPart(g, "UserTargeting", userTargetingToGen(a.UserSource))
	return g
}

// completionCriteriaToGen builds a multi-user task's CompletionCriteria in the
// shape ako/TestApp (Studio Pro 11.14.0) stores: FallbackOutcomePointer and
// VetoOutcomePointer hold the $ID of one of the task's own outcomes. With no rule
// it is consensus falling back to the first outcome, as every rebuild has written.
// An outcome name that matches none is left unset rather than pointed at a guess;
// check reports it (CE1866 / CE1867).
func completionCriteriaToGen(a *workflows.UserTask) element.Element {
	outcomeID := func(value string) (model.ID, bool) {
		for _, o := range a.Outcomes {
			if o.Value == value || (o.Value == "" && o.Caption == value) {
				return o.ID, true
			}
		}
		return "", false
	}
	cc := a.CompletionCriteria
	if cc == nil {
		g := newElem("Workflows$ConsensusCompletionCriteria", "")
		if len(a.Outcomes) > 0 {
			addIDRef(g, "FallbackOutcomePointer", a.Outcomes[0].ID)
		}
		return g
	}
	setFallback := func(g *element.Base) {
		if id, ok := outcomeID(cc.FallbackOutcome); ok && cc.FallbackOutcome != "" {
			addIDRef(g, "FallbackOutcomePointer", id)
		}
	}
	switch cc.Kind {
	case "Majority":
		g := newElem("Workflows$MajorityCompletionCriteria", "")
		addStr(g, "CompletionType", cc.CompletionType)
		setFallback(g)
		return g
	case "Threshold":
		g := newElem("Workflows$ThresholdCompletionCriteria", "")
		addStr(g, "CompletionType", cc.CompletionType)
		setFallback(g)
		addInt32(g, "Threshold", int32(cc.Threshold))
		return g
	case "Veto":
		g := newElem("Workflows$VetoCompletionCriteria", "")
		if id, ok := outcomeID(cc.VetoOutcome); ok {
			addIDRef(g, "VetoOutcomePointer", id)
		}
		return g
	case "Microflow":
		g := newElem("Workflows$MicroflowCompletionCriteria", "")
		addStr(g, "Microflow", cc.Microflow)
		return g
	default:
		g := newElem("Workflows$ConsensusCompletionCriteria", "")
		setFallback(g)
		return g
	}
}

// targetUserInputToGen builds a multi-user task's TargetUserInput.
func targetUserInputToGen(t *workflows.TargetUserInput) element.Element {
	if t == nil {
		return newElem("Workflows$AllUserInput", "")
	}
	switch t.Kind {
	case "Absolute":
		g := newElem("Workflows$AbsoluteAmountUserInput", "")
		addInt32(g, "Amount", int32(t.Amount))
		return g
	case "Percentage":
		g := newElem("Workflows$PercentageAmountUserInput", "")
		addInt32(g, "Percentage", int32(t.Percentage))
		return g
	default:
		return newElem("Workflows$AllUserInput", "")
	}
}

func callMicroflowTaskToGen(a *workflows.CallMicroflowTask) element.Element {
	typeName := callMicroflowTaskType
	if a.IsAgent {
		// Same document shape; applyCallMicroflowStorageName renames only the
		// call-microflow type, so this one is written as is.
		typeName = aiAgentTaskActivityType
	}
	g := newElem(typeName, activityID(&a.BaseWorkflowActivity))
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
			flow := flowToGen(o.Flow)
			addPart(oc, "Flow", flow)
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
		typeName, ok := workflows.BoundaryEventStorageType(ev.EventType)
		if !ok {
			// The reader and the builder only produce known kinds; writing an
			// unknown one as some other kind would mistype it silently.
			continue
		}
		g := newElem(typeName, activityIDOrFresh(string(ev.ID)))
		addStr(g, "Caption", ev.Caption)
		if ev.TimerDelay != "" && !ev.IsNotification() {
			addStr(g, "FirstExecutionTime", ev.TimerDelay)
		}
		if ev.Flow != nil {
			addPart(g, "Flow", flowToGen(ev.Flow))
		}
		if ev.IsNotification() {
			addStr(g, "Name", ev.Name)
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
