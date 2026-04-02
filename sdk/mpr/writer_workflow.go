// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"

	"go.mongodb.org/mongo-driver/bson"
)

// CreateWorkflow creates a new workflow in the MPR.
func (w *Writer) CreateWorkflow(wf *workflows.Workflow) error {
	if wf.ID == "" {
		wf.ID = model.ID(generateUUID())
	}
	wf.TypeName = "Workflows$Workflow"

	contents, err := w.serializeWorkflow(wf)
	if err != nil {
		return fmt.Errorf("failed to serialize workflow: %w", err)
	}

	return w.insertUnit(string(wf.ID), string(wf.ContainerID), "Documents", "Workflows$Workflow", contents)
}

// DeleteWorkflow deletes a workflow from the MPR.
func (w *Writer) DeleteWorkflow(id model.ID) error {
	return w.deleteUnit(string(id))
}

func (w *Writer) serializeWorkflow(wf *workflows.Workflow) ([]byte, error) {
	// AdminPage is a PartProperty (object or null), not a string.
	// When empty, it must be null, not "".
	var adminPageValue any
	if wf.AdminPage != "" {
		adminPageValue = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Workflows$PageReference"},
			{Key: "Page", Value: wf.AdminPage},
		}
	}

	// Annotation is a PartProperty (object or null).
	var annotationValue any
	if wf.Annotation != "" {
		annotationValue = serializeAnnotation(wf.Annotation)
	}

	// Flow
	var flowValue bson.D
	if wf.Flow != nil {
		flowValue = serializeWorkflowFlow(wf.Flow)
	} else {
		emptyFlow := &workflows.Flow{}
		emptyFlow.ID = model.ID(generateUUID())
		flowValue = serializeWorkflowFlow(emptyFlow)
	}

	// Title defaults to workflow display name or Name
	title := wf.WorkflowName
	if title == "" {
		title = wf.Name
	}

	// Build doc in alphabetical key order matching Studio Pro BSON layout
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(wf.ID))},
		{Key: "$Type", Value: "Workflows$Workflow"},
		{Key: "AdminPage", Value: adminPageValue},
		{Key: "Annotation", Value: annotationValue},
		{Key: "Documentation", Value: wf.Documentation},
		{Key: "DueDate", Value: wf.DueDate},
		{Key: "Excluded", Value: wf.Excluded},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "Flow", Value: flowValue},
		{Key: "Name", Value: wf.Name},
		{Key: "OnWorkflowEvent", Value: bson.A{int32(2)}},
	}

	// Parameter
	if wf.Parameter != nil {
		doc = append(doc, bson.E{Key: "Parameter", Value: serializeWorkflowParameter(wf.Parameter)})
	}

	doc = append(doc,
		bson.E{Key: "PersistentId", Value: idToBsonBinary(generateUUID())},
		bson.E{Key: "Title", Value: title},
		bson.E{Key: "WorkflowDescription", Value: serializeWorkflowStringTemplate(wf.WorkflowDescription)},
		bson.E{Key: "WorkflowMetaData", Value: nil},
		bson.E{Key: "WorkflowName", Value: serializeWorkflowStringTemplate(wf.WorkflowName)},
		bson.E{Key: "WorkflowV2", Value: false},
	)

	// NOTE: OverviewPage was deleted in Mendix 9.11.0 — do not serialize it.
	// NOTE: AllowedModuleRoles is not present in Studio Pro BSON — omitted.

	return bson.Marshal(doc)
}

// serializeWorkflowStringTemplate creates a minimal Mendix StringTemplate BSON structure for workflows.
func serializeWorkflowStringTemplate(text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Microflows$StringTemplate"},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Text", Value: text},
	}
}

// serializeWorkflowParameter serializes a workflow parameter.
// Since Mendix 9.10.0, EntityRef (PartProperty) was replaced by Entity (ByNameReferenceProperty).
func serializeWorkflowParameter(param *workflows.WorkflowParameter) bson.D {
	paramID := string(param.ID)
	if paramID == "" {
		paramID = generateUUID()
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(paramID)},
		{Key: "$Type", Value: "Workflows$Parameter"},
		{Key: "Entity", Value: param.EntityRef},
		{Key: "Name", Value: "WorkflowContext"},
	}
}

// serializeAnnotation serializes a workflow annotation if non-empty.
func serializeAnnotation(annotation string) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Workflows$Annotation"},
		{Key: "Description", Value: annotation},
	}
}

// appendActivityBaseFields appends common activity fields to a BSON doc.
// If annotation is non-empty, it serializes as an object; otherwise null.
func appendActivityBaseFields(doc bson.D, annotation string) bson.D {
	var annotationValue any
	if annotation != "" {
		annotationValue = serializeAnnotation(annotation)
	}
	return append(doc,
		bson.E{Key: "Annotation", Value: annotationValue},
		bson.E{Key: "PersistentId", Value: idToBsonBinary(generateUUID())},
		bson.E{Key: "RelativeMiddlePoint", Value: ""},
		bson.E{Key: "Size", Value: ""},
	)
}

// serializeBoundaryEvents serializes boundary events for workflow activities.
func serializeBoundaryEvents(events []*workflows.BoundaryEvent) bson.A {
	arr := bson.A{int32(2)} // array type marker (BoundaryEvents use marker 2)
	for _, event := range events {
		eventID := string(event.ID)
		if eventID == "" {
			eventID = generateUUID()
		}

		typeName := "Workflows$InterruptingTimerBoundaryEvent"
		switch event.EventType {
		case "NonInterruptingTimer":
			typeName = "Workflows$NonInterruptingTimerBoundaryEvent"
		case "Timer":
			typeName = "Workflows$TimerBoundaryEvent"
		case "InterruptingTimer":
			typeName = "Workflows$InterruptingTimerBoundaryEvent"
		}

		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(eventID)},
			{Key: "$Type", Value: typeName},
			{Key: "Caption", Value: event.Caption},
		}

		if event.TimerDelay != "" {
			doc = append(doc, bson.E{Key: "FirstExecutionTime", Value: event.TimerDelay})
		}

		if event.Flow != nil {
			doc = append(doc, bson.E{Key: "Flow", Value: serializeWorkflowFlow(event.Flow)})
		}

		doc = append(doc, bson.E{Key: "PersistentId", Value: idToBsonBinary(generateUUID())})

		if typeName == "Workflows$NonInterruptingTimerBoundaryEvent" {
			doc = append(doc, bson.E{Key: "Recurrence", Value: nil})
		}

		arr = append(arr, doc)
	}
	return arr
}

// emptyBoundaryEvents returns an empty boundary events array marker.
func emptyBoundaryEvents() bson.A {
	return bson.A{int32(2)}
}

// serializeWorkflowFlow serializes a workflow flow with its activities.
func serializeWorkflowFlow(flow *workflows.Flow) bson.D {
	flowID := string(flow.ID)
	if flowID == "" {
		flowID = generateUUID()
	}

	activities := bson.A{int32(3)} // array type marker
	for _, act := range flow.Activities {
		actDoc := serializeWorkflowActivity(act)
		if actDoc != nil {
			activities = append(activities, actDoc)
		}
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(flowID)},
		{Key: "$Type", Value: "Workflows$Flow"},
		{Key: "Activities", Value: activities},
	}
}

// serializeWorkflowActivity dispatches to the correct serializer.
func serializeWorkflowActivity(act workflows.WorkflowActivity) bson.D {
	switch a := act.(type) {
	case *workflows.UserTask:
		return serializeUserTask(a)
	case *workflows.CallMicroflowTask:
		return serializeCallMicroflowTask(a)
	case *workflows.CallWorkflowActivity:
		return serializeCallWorkflowActivity(a)
	case *workflows.ExclusiveSplitActivity:
		return serializeExclusiveSplit(a)
	case *workflows.ParallelSplitActivity:
		return serializeParallelSplit(a)
	case *workflows.JumpToActivity:
		return serializeJumpTo(a)
	case *workflows.WaitForTimerActivity:
		return serializeWaitForTimer(a)
	case *workflows.WaitForNotificationActivity:
		return serializeWaitForNotification(a)
	case *workflows.StartWorkflowActivity:
		return serializeStartWorkflow(a)
	case *workflows.EndWorkflowActivity:
		return serializeEndWorkflow(a)
	case *workflows.WorkflowAnnotationActivity:
		return serializeWorkflowAnnotationActivity(a)
	default:
		return nil
	}
}

func activityID(a *workflows.BaseWorkflowActivity) string {
	if string(a.ID) != "" {
		return string(a.ID)
	}
	return generateUUID()
}

func serializeUserTask(a *workflows.UserTask) bson.D {
	// UserTask was deleted in Mendix 10.12.0, replaced by SingleUserTaskActivity.
	typeName := "Workflows$SingleUserTaskActivity"
	if a.IsMulti {
		typeName = "Workflows$MultiUserTaskActivity"
	}
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(activityID(&a.BaseWorkflowActivity))},
		{Key: "$Type", Value: typeName},
	}

	// Annotation (null or object)
	var annotationValue any
	if a.Annotation != "" {
		annotationValue = serializeAnnotation(a.Annotation)
	}
	doc = append(doc, bson.E{Key: "Annotation", Value: annotationValue})

	// AutoAssignSingleTargetUser
	doc = append(doc, bson.E{Key: "AutoAssignSingleTargetUser", Value: false})

	// AwaitAllUsers (MultiUserTaskActivity only)
	if a.IsMulti {
		doc = append(doc, bson.E{Key: "AwaitAllUsers", Value: false})
	}

	// BoundaryEvents (always present, even if empty)
	if len(a.BoundaryEvents) > 0 {
		doc = append(doc, bson.E{Key: "BoundaryEvents", Value: serializeBoundaryEvents(a.BoundaryEvents)})
	} else {
		doc = append(doc, bson.E{Key: "BoundaryEvents", Value: emptyBoundaryEvents()})
	}

	doc = append(doc,
		bson.E{Key: "Caption", Value: a.Caption},
	)

	// CompletionCriteria (MultiUserTaskActivity only) — must reference first outcome ID
	if a.IsMulti {
		// Pre-assign ID to first outcome so FallbackOutcomePointer can reference it
		if len(a.Outcomes) > 0 && a.Outcomes[0].ID == "" {
			a.Outcomes[0].ID = model.ID(generateUUID())
		}
		fallbackID := ""
		if len(a.Outcomes) > 0 {
			fallbackID = string(a.Outcomes[0].ID)
		} else {
			fallbackID = generateUUID()
		}
		doc = append(doc, bson.E{Key: "CompletionCriteria", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Workflows$ConsensusCompletionCriteria"},
			{Key: "FallbackOutcomePointer", Value: idToBsonBinary(fallbackID)},
		}})
	}

	doc = append(doc,
		bson.E{Key: "DueDate", Value: a.DueDate},
		bson.E{Key: "Name", Value: a.Name},
	)

	// OnCreatedEvent (NoEvent)
	doc = append(doc, bson.E{Key: "OnCreatedEvent", Value: bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Workflows$NoEvent"},
	}})

	// Outcomes
	outcomes := bson.A{int32(3)}
	for _, outcome := range a.Outcomes {
		outcomes = append(outcomes, serializeUserTaskOutcome(outcome))
	}
	doc = append(doc, bson.E{Key: "Outcomes", Value: outcomes})

	doc = append(doc,
		bson.E{Key: "PersistentId", Value: idToBsonBinary(generateUUID())},
		bson.E{Key: "RelativeMiddlePoint", Value: ""},
		bson.E{Key: "Size", Value: ""},
	)

	// TaskDescription
	doc = append(doc, bson.E{Key: "TaskDescription", Value: serializeWorkflowStringTemplate(a.TaskDescription)})

	// TaskName
	taskName := a.TaskName
	if taskName == "" {
		taskName = a.Caption
	}
	doc = append(doc, bson.E{Key: "TaskName", Value: serializeWorkflowStringTemplate(taskName)})

	// TaskPage (PageReference - required, never null)
	doc = append(doc, bson.E{Key: "TaskPage", Value: bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Workflows$PageReference"},
		{Key: "Page", Value: a.Page},
	}})

	// TargetUserInput (MultiUserTaskActivity only) — always AllUserInput
	if a.IsMulti {
		doc = append(doc, bson.E{Key: "TargetUserInput", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Workflows$AllUserInput"},
		}})
	}

	// UserTargeting (NoUserTargeting when not specified)
	if a.UserSource != nil {
		doc = append(doc, bson.E{Key: "UserTargeting", Value: serializeUserTargeting(a.UserSource)})
	} else {
		doc = append(doc, bson.E{Key: "UserTargeting", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Workflows$NoUserTargeting"},
		}})
	}

	return doc
}

func serializeUserTargeting(source workflows.UserSource) bson.D {
	switch s := source.(type) {
	case *workflows.MicroflowBasedUserSource:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Workflows$MicroflowUserTargeting"},
			{Key: "Microflow", Value: s.Microflow},
		}
	case *workflows.XPathBasedUserSource:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Workflows$XPathUserTargeting"},
			{Key: "XPathConstraint", Value: s.XPath},
		}
	default:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Workflows$NoUserTargeting"},
		}
	}
}

func serializeUserTaskOutcome(outcome *workflows.UserTaskOutcome) bson.D {
	outcomeID := string(outcome.ID)
	if outcomeID == "" {
		outcomeID = generateUUID()
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(outcomeID)},
		{Key: "$Type", Value: "Workflows$UserTaskOutcome"},
	}

	if outcome.Flow != nil {
		doc = append(doc, bson.E{Key: "Flow", Value: serializeWorkflowFlow(outcome.Flow)})
	}

	doc = append(doc,
		bson.E{Key: "PersistentId", Value: idToBsonBinary(generateUUID())},
		bson.E{Key: "Value", Value: outcome.Value},
	)

	return doc
}

func serializeCallMicroflowTask(a *workflows.CallMicroflowTask) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(activityID(&a.BaseWorkflowActivity))},
		{Key: "$Type", Value: "Workflows$CallMicroflowTask"},
	}

	// Annotation
	var annotationValue any
	if a.Annotation != "" {
		annotationValue = serializeAnnotation(a.Annotation)
	}
	doc = append(doc, bson.E{Key: "Annotation", Value: annotationValue})

	// BoundaryEvents (always present)
	if len(a.BoundaryEvents) > 0 {
		doc = append(doc, bson.E{Key: "BoundaryEvents", Value: serializeBoundaryEvents(a.BoundaryEvents)})
	} else {
		doc = append(doc, bson.E{Key: "BoundaryEvents", Value: emptyBoundaryEvents()})
	}

	doc = append(doc,
		bson.E{Key: "Caption", Value: a.Caption},
		bson.E{Key: "Microflow", Value: a.Microflow},
		bson.E{Key: "Name", Value: a.Name},
	)

	// Outcomes
	outcomes := bson.A{int32(3)}
	for _, outcome := range a.Outcomes {
		outcomes = append(outcomes, serializeConditionOutcome(outcome))
	}
	doc = append(doc, bson.E{Key: "Outcomes", Value: outcomes})

	// ParameterMappings (always present)
	mappings := bson.A{int32(2)}
	for _, pm := range a.ParameterMappings {
		pmID := string(pm.ID)
		if pmID == "" {
			pmID = generateUUID()
		}
		mappings = append(mappings, bson.D{
			{Key: "$ID", Value: idToBsonBinary(pmID)},
			{Key: "$Type", Value: "Workflows$MicroflowCallParameterMapping"},
			{Key: "Expression", Value: pm.Expression},
			{Key: "Parameter", Value: pm.Parameter},
		})
	}
	doc = append(doc, bson.E{Key: "ParameterMappings", Value: mappings})

	doc = append(doc,
		bson.E{Key: "PersistentId", Value: idToBsonBinary(generateUUID())},
		bson.E{Key: "RelativeMiddlePoint", Value: ""},
		bson.E{Key: "Size", Value: ""},
	)

	return doc
}

func serializeCallWorkflowActivity(a *workflows.CallWorkflowActivity) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(activityID(&a.BaseWorkflowActivity))},
		{Key: "$Type", Value: "Workflows$CallWorkflowActivity"},
	}

	// Annotation
	var annotationValue any
	if a.Annotation != "" {
		annotationValue = serializeAnnotation(a.Annotation)
	}
	doc = append(doc, bson.E{Key: "Annotation", Value: annotationValue})

	// BoundaryEvents (always present)
	if len(a.BoundaryEvents) > 0 {
		doc = append(doc, bson.E{Key: "BoundaryEvents", Value: serializeBoundaryEvents(a.BoundaryEvents)})
	} else {
		doc = append(doc, bson.E{Key: "BoundaryEvents", Value: emptyBoundaryEvents()})
	}

	doc = append(doc,
		bson.E{Key: "Caption", Value: a.Caption},
		bson.E{Key: "ExecuteAsync", Value: false},
		bson.E{Key: "Name", Value: a.Name},
	)

	// ParameterMappings (always present, marker int32(2))
	paramMappings := bson.A{int32(2)}
	for _, pm := range a.ParameterMappings {
		pmID := string(pm.ID)
		if pmID == "" {
			pmID = generateUUID()
		}
		paramMappings = append(paramMappings, bson.D{
			{Key: "$ID", Value: idToBsonBinary(pmID)},
			{Key: "$Type", Value: "Workflows$WorkflowCallParameterMapping"},
			{Key: "Expression", Value: pm.Expression},
			{Key: "Parameter", Value: pm.Parameter},
		})
	}
	doc = append(doc, bson.E{Key: "ParameterMappings", Value: paramMappings})

	doc = append(doc,
		bson.E{Key: "PersistentId", Value: idToBsonBinary(generateUUID())},
		bson.E{Key: "RelativeMiddlePoint", Value: ""},
		bson.E{Key: "Size", Value: ""},
		bson.E{Key: "Workflow", Value: a.Workflow},
	)

	return doc
}

func serializeExclusiveSplit(a *workflows.ExclusiveSplitActivity) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(activityID(&a.BaseWorkflowActivity))},
		{Key: "$Type", Value: "Workflows$ExclusiveSplitActivity"},
	}

	doc = appendActivityBaseFields(doc, a.Annotation)

	doc = append(doc,
		bson.E{Key: "Caption", Value: a.Caption},
		bson.E{Key: "Expression", Value: a.Expression},
		bson.E{Key: "Name", Value: a.Name},
	)

	outcomes := bson.A{int32(3)}
	for _, outcome := range a.Outcomes {
		outcomes = append(outcomes, serializeConditionOutcome(outcome))
	}
	doc = append(doc, bson.E{Key: "Outcomes", Value: outcomes})

	return doc
}

func serializeConditionOutcome(outcome workflows.ConditionOutcome) bson.D {
	switch o := outcome.(type) {
	case *workflows.BooleanConditionOutcome:
		outcomeID := string(o.ID)
		if outcomeID == "" {
			outcomeID = generateUUID()
		}
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(outcomeID)},
			{Key: "$Type", Value: "Workflows$BooleanConditionOutcome"},
			{Key: "Value", Value: o.Value},
		}
		if o.Flow != nil {
			doc = append(doc, bson.E{Key: "Flow", Value: serializeWorkflowFlow(o.Flow)})
		}
		return doc
	case *workflows.EnumerationValueConditionOutcome:
		outcomeID := string(o.ID)
		if outcomeID == "" {
			outcomeID = generateUUID()
		}
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(outcomeID)},
			{Key: "$Type", Value: "Workflows$EnumerationValueConditionOutcome"},
			{Key: "Value", Value: o.Value},
		}
		if o.Flow != nil {
			doc = append(doc, bson.E{Key: "Flow", Value: serializeWorkflowFlow(o.Flow)})
		}
		return doc
	case *workflows.VoidConditionOutcome:
		outcomeID := string(o.ID)
		if outcomeID == "" {
			outcomeID = generateUUID()
		}
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(outcomeID)},
			{Key: "$Type", Value: "Workflows$VoidConditionOutcome"},
		}
		if o.Flow != nil {
			doc = append(doc, bson.E{Key: "Flow", Value: serializeWorkflowFlow(o.Flow)})
		}
		return doc
	default:
		return nil
	}
}

func serializeParallelSplit(a *workflows.ParallelSplitActivity) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(activityID(&a.BaseWorkflowActivity))},
		{Key: "$Type", Value: "Workflows$ParallelSplitActivity"},
	}

	doc = appendActivityBaseFields(doc, a.Annotation)

	doc = append(doc,
		bson.E{Key: "Caption", Value: a.Caption},
		bson.E{Key: "Name", Value: a.Name},
	)

	outcomes := bson.A{int32(3)}
	for _, outcome := range a.Outcomes {
		outcomeID := string(outcome.ID)
		if outcomeID == "" {
			outcomeID = generateUUID()
		}
		outDoc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(outcomeID)},
			{Key: "$Type", Value: "Workflows$ParallelSplitOutcome"},
		}
		if outcome.Flow != nil {
			outDoc = append(outDoc, bson.E{Key: "Flow", Value: serializeWorkflowFlow(outcome.Flow)})
		}
		outDoc = append(outDoc, bson.E{Key: "PersistentId", Value: idToBsonBinary(generateUUID())})
		outcomes = append(outcomes, outDoc)
	}
	doc = append(doc, bson.E{Key: "Outcomes", Value: outcomes})

	return doc
}

func serializeJumpTo(a *workflows.JumpToActivity) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(activityID(&a.BaseWorkflowActivity))},
		{Key: "$Type", Value: "Workflows$JumpToActivity"},
	}

	doc = appendActivityBaseFields(doc, a.Annotation)

	doc = append(doc,
		bson.E{Key: "Caption", Value: a.Caption},
		bson.E{Key: "Name", Value: a.Name},
		bson.E{Key: "TargetActivity", Value: a.TargetActivity},
	)

	return doc
}

func serializeWaitForTimer(a *workflows.WaitForTimerActivity) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(activityID(&a.BaseWorkflowActivity))},
		{Key: "$Type", Value: "Workflows$WaitForTimerActivity"},
	}

	doc = appendActivityBaseFields(doc, a.Annotation)

	doc = append(doc,
		bson.E{Key: "Caption", Value: a.Caption},
		bson.E{Key: "Delay", Value: a.DelayExpression},
		bson.E{Key: "Name", Value: a.Name},
	)

	return doc
}

func serializeWaitForNotification(a *workflows.WaitForNotificationActivity) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(activityID(&a.BaseWorkflowActivity))},
		{Key: "$Type", Value: "Workflows$WaitForNotificationActivity"},
	}

	// Annotation
	var annotationValue any
	if a.Annotation != "" {
		annotationValue = serializeAnnotation(a.Annotation)
	}
	doc = append(doc, bson.E{Key: "Annotation", Value: annotationValue})

	// BoundaryEvents (always present)
	if len(a.BoundaryEvents) > 0 {
		doc = append(doc, bson.E{Key: "BoundaryEvents", Value: serializeBoundaryEvents(a.BoundaryEvents)})
	} else {
		doc = append(doc, bson.E{Key: "BoundaryEvents", Value: emptyBoundaryEvents()})
	}

	doc = append(doc,
		bson.E{Key: "Caption", Value: a.Caption},
		bson.E{Key: "Name", Value: a.Name},
		bson.E{Key: "PersistentId", Value: idToBsonBinary(generateUUID())},
		bson.E{Key: "RelativeMiddlePoint", Value: ""},
		bson.E{Key: "Size", Value: ""},
	)

	return doc
}

func serializeStartWorkflow(a *workflows.StartWorkflowActivity) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(activityID(&a.BaseWorkflowActivity))},
		{Key: "$Type", Value: "Workflows$StartWorkflowActivity"},
	}

	doc = appendActivityBaseFields(doc, a.Annotation)

	doc = append(doc,
		bson.E{Key: "Caption", Value: a.Caption},
		bson.E{Key: "Name", Value: a.Name},
	)

	return doc
}

func serializeEndWorkflow(a *workflows.EndWorkflowActivity) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(activityID(&a.BaseWorkflowActivity))},
		{Key: "$Type", Value: "Workflows$EndWorkflowActivity"},
	}

	doc = appendActivityBaseFields(doc, a.Annotation)

	doc = append(doc,
		bson.E{Key: "Caption", Value: a.Caption},
		bson.E{Key: "Name", Value: a.Name},
	)

	return doc
}

func serializeWorkflowAnnotationActivity(a *workflows.WorkflowAnnotationActivity) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(activityID(&a.BaseWorkflowActivity))},
		{Key: "$Type", Value: "Workflows$Annotation"},
		{Key: "Description", Value: a.Description},
	}
	doc = append(doc, bson.E{Key: "PersistentId", Value: idToBsonBinary(generateUUID())})
	doc = append(doc, bson.E{Key: "RelativeMiddlePoint", Value: ""})
	doc = append(doc, bson.E{Key: "Size", Value: ""})
	return doc
}
