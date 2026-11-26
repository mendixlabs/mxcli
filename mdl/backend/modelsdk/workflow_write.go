// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
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
	// EventSubProcesses empties as [2]: Studio Pro 11.14 stores it on a workflow
	// that has none (ako/TestApp). encodeWorkflow suppresses the empty list for a
	// project that does not declare the property (before 11.8.0).
	codec.RegisterTypeDefaults("Workflows$Workflow", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"OnWorkflowEvent": 2, "EventSubProcesses": 2},
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
	return usesCallMicroflowActivityName(b.ProjectVersion())
}

func usesCallMicroflowActivityName(pv *types.ProjectVersion) bool {
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
	contents, err := encodeWorkflow(wf, b.ProjectVersion())
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
	contents, err := encodeWorkflow(wf, b.ProjectVersion())
	if err != nil {
		return fmt.Errorf("UpdateWorkflow: encode: %w", err)
	}
	// A rewrite without an `export level` clause keeps the stored level (#816).
	contents, err = b.keepStoredExportLevelUnlessSet(string(wf.ID), wf.ExportLevel, contents)
	if err != nil {
		return fmt.Errorf("UpdateWorkflow: %w", err)
	}
	return b.writer.UpdateRawUnit(string(wf.ID), contents)
}

// workflowEventSubProcessesMajor/Minor is the version that introduced
// Workflows$Workflow.eventSubProcesses (modelsdk/gen/workflows/version.go).
const workflowEventSubProcessesMajor, workflowEventSubProcessesMinor = 11, 8

// encodeWorkflow builds and serializes a workflow for a project of this
// version. CreateWorkflow and UpdateWorkflow both go through it, so a version
// guard cannot be applied on one path and forgotten on the other.
//
// An empty EventSubProcesses list is what Studio Pro stores on a workflow with
// none (ako/TestApp, 11.14.0); omitting it made every rewrite of a Studio Pro
// workflow delete the key (#743). It is written only when the project declares
// the property: a key the project's metamodel does not know makes the document
// unopenable in Studio Pro, and mxbuild does not catch it (see
// codec.Encoder.OmitKeys). An unreadable version omits it, like the page guards.
//
// Below 11.3 the main flow's start and end are not stored, and below 11.1 no
// activity stores RelativeMiddlePoint/Size (ako/mxcli#1754; see
// workflowStoresStartAndEnd and workflowDeclaresActivityGeometry).
func encodeWorkflow(wf *workflows.Workflow, pv *types.ProjectVersion) ([]byte, error) {
	if !workflowStoresStartAndEnd(pv) {
		wf = withImplicitStartAndEnd(wf)
	}
	g := workflowToGen(wf)
	applyCallMicroflowStorageName(g, usesCallMicroflowActivityName(pv))
	enc := &codec.Encoder{}
	if len(wf.EventSubProcesses) == 0 && !workflowSupportsEventSubProcesses(pv) {
		// Only the empty default is suppressed: event sub-processes the script
		// declares are written whatever the version (the feature check refuses
		// them on an older project before this is reached).
		enc.OmitKeys = map[string]map[string]bool{"Workflows$Workflow": {"EventSubProcesses": true}}
	}
	raw, err := enc.Encode(g)
	if err != nil || workflowDeclaresActivityGeometry(pv) {
		return raw, err
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	return bson.Marshal(withoutWorkflowActivityGeometry(d))
}

// workflowStoresStartAndEnd reports whether the main flow's start and closing
// end are stored activities. Workflows$StartWorkflowActivity is introduced in
// 11.1.0 (mendixmodelsdk 4.117.0), and a 10.24 or 11.0 project holding one does
// not load (TypeCacheUnknownTypeException). 11.1 and 11.2 have the type but keep
// the main flow's start and end implicit — `mx convert` of a 10.24 workflow adds
// neither, and with them mx check reports CE6671 and CE6689 — and 11.3 is where
// the conversion adds both (ako/mxcli#1754). An unread version keeps the current
// shape.
func workflowStoresStartAndEnd(pv *types.ProjectVersion) bool {
	return pv == nil || pv.IsAtLeast(11, 3)
}

// workflowDeclaresActivityGeometry reports whether workflow activities store
// RelativeMiddlePoint and Size, which Workflows$WorkflowActivity declares from
// 11.1.0 (modelsdk/gen/workflows/version.go). The property-set pass cannot strip
// them below that: they are declared on the supertype, which it does not look
// up, and "0;0" is a value, which it would refuse rather than drop.
func workflowDeclaresActivityGeometry(pv *types.ProjectVersion) bool {
	return pv == nil || pv.IsAtLeast(11, 1)
}

// withImplicitStartAndEnd returns wf without its main flow's start and closing
// end, for a version that keeps them implicit. The note on the start becomes the
// workflow's own Annotation, which is where those versions keep the header's
// note (`mx convert` to 11.3 moves it onto the start). An End inside a branch
// stays: EndWorkflowActivity exists from 9.0.2 and is what ends that path. wf is
// not modified.
func withImplicitStartAndEnd(wf *workflows.Workflow) *workflows.Workflow {
	if wf.Flow == nil || len(wf.Flow.Activities) == 0 {
		return wf
	}
	out, flow := *wf, *wf.Flow
	acts := flow.Activities
	if start, ok := acts[0].(*workflows.StartWorkflowActivity); ok {
		if start.Annotation != "" {
			out.Annotation = start.Annotation
		}
		acts = acts[1:]
	}
	if n := len(acts); n > 0 {
		if _, ok := acts[n-1].(*workflows.EndWorkflowActivity); ok {
			acts = acts[:n-1]
		}
	}
	flow.Activities = acts
	out.Flow = &flow
	return &out
}

// withoutWorkflowActivityGeometry removes RelativeMiddlePoint and Size from
// every Workflows$ element in d, for a version that does not declare them.
func withoutWorkflowActivityGeometry(d bson.D) bson.D {
	workflowType := false
	for _, e := range d {
		if e.Key == "$Type" {
			t, _ := e.Value.(string)
			workflowType = strings.HasPrefix(t, "Workflows$")
		}
	}
	out := d[:0]
	for _, e := range d {
		if workflowType && (e.Key == "RelativeMiddlePoint" || e.Key == "Size") {
			continue
		}
		switch v := e.Value.(type) {
		case bson.D:
			e.Value = withoutWorkflowActivityGeometry(v)
		case bson.A:
			for i, el := range v {
				if sub, ok := el.(bson.D); ok {
					v[i] = withoutWorkflowActivityGeometry(sub)
				}
			}
		}
		out = append(out, e)
	}
	return out
}

// workflowSupportsEventSubProcesses reports whether the project declares
// Workflows$Workflow.eventSubProcesses, introduced in 11.8.0
// (modelsdk/gen/workflows/version.go).
func workflowSupportsEventSubProcesses(pv *types.ProjectVersion) bool {
	return pv != nil && pv.IsAtLeast(11, 8)
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
	// The overview page is stored under AdminPage (Mendix renamed the property in
	// 9.11.0 — see workflows.Workflow.OverviewPage). Only this spelling is
	// written: a mxcli-authored workflow already carries WorkflowV2, introduced
	// in 11.1.0, so there is no reachable project for which the pre-9.11
	// OverviewPage key would be right, and writing both as a hedge is what the
	// overlay rule in CLAUDE.md forbids.
	if wf.OverviewPage != "" {
		addPart(g, "AdminPage", pageReferenceElem(wf.OverviewPage))
	}
	if wf.Annotation != "" {
		addPart(g, "Annotation", annotationElem(wf.Annotation))
	}
	addStr(g, "Documentation", wf.Documentation)
	addStr(g, "DueDate", wf.DueDate)
	// EventSubProcesses: a marker-2 list, written empty through the Studio Pro
	// defaults when there are none; workflowEncoder omits it before 11.8.
	if len(wf.EventSubProcesses) > 0 {
		esps := make([]element.Element, 0, len(wf.EventSubProcesses))
		for _, esp := range wf.EventSubProcesses {
			esps = append(esps, eventSubProcessToGen(esp))
		}
		addPartList(g, "EventSubProcesses", esps)
	}
	addBool(g, "Excluded", wf.Excluded)
	// The statement's `export level`, when it has one (#816): the constant this
	// wrote made the clause a no-op on create and on every rewrite.
	addStr(g, "ExportLevel", orDefault(wf.ExportLevel, "Hidden"))
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
	addPart(g, "WorkflowDescription", workflowStringTemplate(wf.WorkflowDescription, wf.WorkflowDescriptionParams))
	// WorkflowMetaData: null (via NullFields).
	addPart(g, "WorkflowName", workflowStringTemplate(wf.WorkflowName, wf.WorkflowNameParams))
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
		if a.Recurrence != nil && a.Timer && !a.Interrupting {
			addPart(g, "Recurrence", linearRecurrenceToGen(a.Recurrence))
		}
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
	addBool(g, "AutoAssignSingleTargetUser", a.AutoAssignSingleTargetUser)
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
	addStr(g, "RelativeMiddlePoint", workflowZeroPoint)
	addStr(g, "Size", workflowZeroPoint)
	addPart(g, "TaskDescription", workflowStringTemplate(a.TaskDescription, a.TaskDescriptionParams))
	taskName := a.TaskName
	if taskName == "" {
		taskName = a.Caption
	}
	addPart(g, "TaskName", workflowStringTemplate(taskName, a.TaskNameParams))
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
	addStr(g, "RelativeMiddlePoint", workflowZeroPoint)
	addStr(g, "Size", workflowZeroPoint)
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
	addStr(g, "RelativeMiddlePoint", workflowZeroPoint)
	addStr(g, "Size", workflowZeroPoint)
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
		addPart(oc, "Flow", outcomeFlowToGen(o.Flow))
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
	addStr(g, "RelativeMiddlePoint", workflowZeroPoint)
	addStr(g, "Size", workflowZeroPoint)
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
	addStr(g, "RelativeMiddlePoint", workflowZeroPoint)
	addStr(g, "Size", workflowZeroPoint)
	return g
}

func userTaskOutcomeToGen(o *workflows.UserTaskOutcome) element.Element {
	g := newElem("Workflows$UserTaskOutcome", string(o.ID))
	addPart(g, "Flow", outcomeFlowToGen(o.Flow))
	addFreshPersistentID(g)
	addStr(g, "Value", o.Value)
	return g
}

func conditionOutcomeToGen(outcome workflows.ConditionOutcome) element.Element {
	switch o := outcome.(type) {
	case *workflows.BooleanConditionOutcome:
		g := newElem("Workflows$BooleanConditionOutcome", outcomeID(o.ID))
		addPart(g, "Flow", outcomeFlowToGen(o.Flow))
		addFreshPersistentID(g)
		addBool(g, "Value", o.Value)
		return g
	case *workflows.EnumerationValueConditionOutcome:
		g := newElem("Workflows$EnumerationValueConditionOutcome", outcomeID(o.ID))
		addPart(g, "Flow", outcomeFlowToGen(o.Flow))
		addFreshPersistentID(g)
		addStr(g, "Value", o.Value)
		return g
	case *workflows.VoidConditionOutcome:
		g := newElem("Workflows$VoidConditionOutcome", outcomeID(o.ID))
		addPart(g, "Flow", outcomeFlowToGen(o.Flow))
		// Workflows$Outcome.persistentId (10.21.0), as on every other outcome.
		addFreshPersistentID(g)
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
		// A timer's Name exists from Mendix 11.17 (ako/mxcli#2616) and is only
		// ever read from such a project; canon.RaiseBoundaryEventContainers
		// names the ones without.
		if ev.IsNotification() || ev.Name != "" {
			addStr(g, "Name", ev.Name)
		}
		addFreshPersistentID(g)
		// Recurrence: null on NonInterrupting (via NullFields) unless it repeats.
		if ev.Recurrence != nil && typeName == "Workflows$NonInterruptingTimerBoundaryEvent" {
			addPart(g, "Recurrence", linearRecurrenceToGen(ev.Recurrence))
		}
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

// workflowStringTemplate builds a workflow text: a Microflows$StringTemplate
// whose Parameters are Microflows$TemplateParameter children, one Expression
// each — the shape mx check parses (an invalid expression is CE0117 at the
// task, 11.15.0) — or an empty list via MandatoryListMarkers (ako/mxcli#1714).
func workflowStringTemplate(text string, params []string) element.Element {
	return stringTemplateElem(text, params)
}

// addActivityBaseFields adds the common Annotation(null)/PersistentId/
// RelativeMiddlePoint/Size fields shared by the simpler activity types.
func addActivityBaseFields(g *element.Base, annotation string) {
	if annotation != "" {
		addPart(g, "Annotation", annotationElem(annotation))
	}
	addFreshPersistentID(g)
	addStr(g, "RelativeMiddlePoint", workflowZeroPoint)
	addStr(g, "Size", workflowZeroPoint)
}

// workflowZeroPoint is the RelativeMiddlePoint and Size Studio Pro stores on
// every workflow activity (measured on ako/TestApp, 11.14.0: 14 of 14). The
// workflow editor lays activities out itself; an empty string is not a point.
const workflowZeroPoint = "0;0"

// outcomeFlowToGen is an outcome's Flow. Workflows$Outcome.flow is Required, and
// Studio Pro stores an empty Flow on an outcome that leads nowhere (ako/TestApp,
// 11.14.0, every UserTaskOutcome and VoidConditionOutcome); omitting it made
// every rewrite of a Studio Pro workflow drop them (#743).
func outcomeFlowToGen(flow *workflows.Flow) element.Element {
	if flow == nil {
		flow = &workflows.Flow{}
	}
	return flowToGen(flow)
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

// linearRecurrenceToGen builds a Workflows$LinearRecurrence (ako/mxcli#1716):
// the shape mx check accepts on 11.15.0, with IntervalType read by the runtime
// as the scheduled-event enumeration (Minute, Hour, Day, Week, Month).
func linearRecurrenceToGen(r *workflows.Recurrence) element.Element {
	g := newElem("Workflows$LinearRecurrence", "")
	addInt32(g, "Interval", int32(r.Interval))
	addStr(g, "IntervalType", r.IntervalType)
	addInt32(g, "MaxExecutions", int32(r.MaxExecutions))
	return g
}
