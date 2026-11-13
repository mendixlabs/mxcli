// SPDX-License-Identifier: Apache-2.0

// Package workflows provides types for Mendix workflows.
package workflows

import (
	"strings"

	"github.com/mendixlabs/mxcli/model"
)

// Workflow represents a workflow in the Mendix model.
type Workflow struct {
	model.BaseElement
	ContainerID         model.ID `json:"containerId"`
	Name                string   `json:"name"`
	Documentation       string   `json:"documentation,omitempty"`
	ExportLevel         string   `json:"exportLevel,omitempty"`
	Excluded            bool     `json:"excluded"`
	WorkflowName        string   `json:"workflowName,omitempty"`        // Template string for display name
	WorkflowDescription string   `json:"workflowDescription,omitempty"` // Template string for description
	// The expressions filling {1}, {2}, … of the two templates, in order.
	WorkflowNameParams        []string `json:"workflowNameParams,omitempty"`
	WorkflowDescriptionParams []string `json:"workflowDescriptionParams,omitempty"`

	// OverviewPage is the qualified name of the workflow's overview page — the
	// `overview page` clause. It is stored under the BSON key **AdminPage**, as a
	// Workflows$PageReference child: Mendix DELETED the `overviewPage` property
	// in 9.11.0 and INTRODUCED `adminPage` in the same release (measured in the
	// Model SDK's own StructureVersionInfo, mendixmodelsdk 4.115.0
	// src/gen/workflows.js; generated/metamodel declares AdminPage and no
	// OverviewPage at all).
	//
	// There used to be a second field, AdminPage, holding the same thing under
	// the storage name. Nothing ever set it and nothing ever read this one back,
	// so `create workflow … overview page X` reported success and stored
	// nothing. One concept gets one field; the storage name stays in the storage
	// adapter, per ADR-0005.
	OverviewPage string `json:"overviewPage,omitempty"`

	DueDate string `json:"dueDate,omitempty"` // Due date expression

	// Annotation is the workflow's own note. Studio Pro 11.15 shows it nowhere:
	// the note on the start is the start activity's (StartNote). mxcli wrote
	// the header `annotation '…'` here before ako/mxcli#1717.
	Annotation string `json:"annotation,omitempty"` // Annotation description text

	// Context parameter
	Parameter *WorkflowParameter `json:"parameter,omitempty"`

	// EventHandlers are the workflow's OnWorkflowEvent handlers, in stored order.
	EventHandlers []*WorkflowEventHandler `json:"eventHandlers,omitempty"`

	// Flow contains the workflow activities
	Flow *Flow `json:"flow,omitempty"`

	// EventSubProcesses are the flows outside the main flow, in stored order.
	EventSubProcesses []*EventSubProcess `json:"eventSubProcesses,omitempty"`
}

// GetName returns the workflow's name.
func (w *Workflow) GetName() string {
	return w.Name
}

// GetContainerID returns the ID of the containing folder/module.
func (w *Workflow) GetContainerID() model.ID {
	return w.ContainerID
}

// WorkflowEventHandler is a Workflows$WorkflowEventHandler: a microflow the
// runtime calls for each of the listed workflow event types.
type WorkflowEventHandler struct {
	model.BaseElement
	Description   string   `json:"description,omitempty"`   // how Studio Pro names the handler
	Documentation string   `json:"documentation,omitempty"` // not authorable from MDL; carried
	EventTypes    []string `json:"eventTypes,omitempty"`    // WorkflowEventType values, stored order
	Microflow     string   `json:"microflow,omitempty"`     // qualified name of the handler microflow
}

// WorkflowParameter represents the context parameter of a workflow.
type WorkflowParameter struct {
	model.BaseElement
	EntityRef string `json:"entityRef,omitempty"` // Qualified name of the context entity
}

// Flow represents a container of workflow activities.
type Flow struct {
	model.BaseElement
	Activities []WorkflowActivity `json:"activities,omitempty"`
}

// WorkflowActivity is the interface for all workflow activity types.
type WorkflowActivity interface {
	GetID() model.ID
	GetName() string
	SetName(string)
	GetCaption() string
	ActivityType() string
}

// BaseWorkflowActivity provides common fields for all workflow activities.
type BaseWorkflowActivity struct {
	model.BaseElement
	Name       string `json:"name,omitempty"`
	Caption    string `json:"caption,omitempty"`
	Annotation string `json:"annotation,omitempty"` // Annotation description text
}

// GetID returns the activity's ID.
func (a *BaseWorkflowActivity) GetID() model.ID {
	return a.ID
}

// GetName returns the activity's name.
func (a *BaseWorkflowActivity) GetName() string {
	return a.Name
}

// SetName sets the activity's name.
func (a *BaseWorkflowActivity) SetName(name string) {
	a.Name = name
}

// GetCaption returns the activity's caption.
// StartNote is the note on the workflow's start, which describe prints as the
// header's `annotation '…'`: the start activity's, as Studio Pro stores and
// shows it — the workflow editor draws the start's note from
// StartWorkflowActivity.Annotation and the workflow's own Annotation nowhere
// (11.15.0, ako/mxcli#1717) — or else the workflow's own, where mxcli wrote it
// before.
func (w *Workflow) StartNote() string {
	if w.Flow != nil {
		for _, a := range w.Flow.Activities {
			if s, ok := a.(*StartWorkflowActivity); ok && s.Annotation != "" {
				return s.Annotation
			}
		}
	}
	return w.Annotation
}

// SetAnnotation sets the note attached to the activity.
func (a *BaseWorkflowActivity) SetAnnotation(s string) {
	a.Annotation = s
}

func (a *BaseWorkflowActivity) GetCaption() string {
	return a.Caption
}

// StartWorkflowActivity represents the start of a workflow.
type StartWorkflowActivity struct {
	BaseWorkflowActivity
}

// ActivityType returns the type name.
func (a *StartWorkflowActivity) ActivityType() string { return "StartWorkflow" }

// EndWorkflowActivity represents the end of a workflow.
type EndWorkflowActivity struct {
	BaseWorkflowActivity
}

// ActivityType returns the type name.
func (a *EndWorkflowActivity) ActivityType() string { return "EndWorkflow" }

// UserTask represents a user task in a workflow.
type UserTask struct {
	BaseWorkflowActivity
	IsMulti         bool               `json:"isMulti,omitempty"`         // true if Workflows$MultiUserTaskActivity
	Page            string             `json:"page,omitempty"`            // Qualified name of the task page
	UserSource      UserSource         `json:"userSource,omitempty"`      // Who should handle the task
	Outcomes        []*UserTaskOutcome `json:"outcomes,omitempty"`        // Task outcomes
	TaskName        string             `json:"taskName,omitempty"`        // Template string for task display name
	TaskDescription string             `json:"taskDescription,omitempty"` // Template string for task description
	// The expressions filling {1}, {2}, … of the two templates, in order.
	TaskNameParams        []string         `json:"taskNameParams,omitempty"`
	TaskDescriptionParams []string         `json:"taskDescriptionParams,omitempty"`
	DueDate               string           `json:"dueDate,omitempty"`        // Due date expression
	UserTaskEntity        string           `json:"userTaskEntity,omitempty"` // Qualified name of user task entity
	OnCreated             string           `json:"onCreated,omitempty"`      // Microflow called on task creation
	BoundaryEvents        []*BoundaryEvent `json:"boundaryEvents,omitempty"` // Boundary events (e.g., timers)

	// AutoAssignSingleTargetUser is Studio Pro's "Auto-assign when targeting
	// results in one user". Off by default, as Studio Pro creates a task.
	AutoAssignSingleTargetUser bool `json:"autoAssignSingleTargetUser,omitempty"`

	// Multi-user task only.
	CompletionCriteria *CompletionCriteria `json:"completionCriteria,omitempty"` // nil = consensus on the first outcome
	TargetUserInput    *TargetUserInput    `json:"targetUserInput,omitempty"`    // nil = all targeted users
	AwaitAllUsers      bool                `json:"awaitAllUsers,omitempty"`
}

// CompletionCriteria is how a multi-user task turns its participants' outcomes
// into one outcome. Outcomes are named by value; storage points at their $ID.
type CompletionCriteria struct {
	Kind            string `json:"kind"`                     // Consensus, Majority, Threshold, Veto or Microflow
	CompletionType  string `json:"completionType,omitempty"` // Absolute or Relative (Majority, Threshold)
	Threshold       int    `json:"threshold,omitempty"`      // percent (Relative) or votes (Absolute)
	FallbackOutcome string `json:"fallbackOutcome,omitempty"`
	VetoOutcome     string `json:"vetoOutcome,omitempty"`
	Microflow       string `json:"microflow,omitempty"` // must return String (CE5012)
}

// TargetUserInput is how many of a multi-user task's targeted users must respond.
type TargetUserInput struct {
	Kind       string `json:"kind"` // All, Absolute or Percentage
	Amount     int    `json:"amount,omitempty"`
	Percentage int    `json:"percentage,omitempty"`
}

// ActivityType returns the type name.
func (a *UserTask) ActivityType() string { return "UserTask" }

// SystemTask represents a system task (call microflow) in a workflow.
type SystemTask struct {
	BaseWorkflowActivity
	Microflow         string              `json:"microflow,omitempty"` // Qualified name of the microflow to call
	Outcomes          []ConditionOutcome  `json:"outcomes,omitempty"`  // Condition-based outcomes
	ParameterMappings []*ParameterMapping `json:"parameterMappings,omitempty"`
}

// ActivityType returns the type name.
func (a *SystemTask) ActivityType() string { return "SystemTask" }

// CallMicroflowTask represents a call-microflow activity in a workflow.
type CallMicroflowTask struct {
	BaseWorkflowActivity
	// IsAgent marks an AI agent task (Workflows$AIAgentTaskActivity, Mendix
	// 11.9+). It stores exactly the call-microflow shape under a different
	// $Type, so it is this type with a flag rather than a type of its own —
	// every walker, validator and catalog edge applies to it unchanged.
	IsAgent           bool                `json:"isAgent,omitempty"`
	Microflow         string              `json:"microflow,omitempty"` // Qualified name of the microflow to call
	Outcomes          []ConditionOutcome  `json:"outcomes,omitempty"`  // Condition-based outcomes
	ParameterMappings []*ParameterMapping `json:"parameterMappings,omitempty"`
	BoundaryEvents    []*BoundaryEvent    `json:"boundaryEvents,omitempty"` // Boundary events (e.g., timers)
}

// ActivityType returns the type name.
func (a *CallMicroflowTask) ActivityType() string { return "CallMicroflow" }

// CallWorkflowActivity represents calling a sub-workflow.
type CallWorkflowActivity struct {
	BaseWorkflowActivity
	Workflow            string              `json:"workflow,omitempty"`            // Qualified name of the workflow to call
	ParameterExpression string              `json:"parameterExpression,omitempty"` // Expression for context parameter
	ParameterMappings   []*ParameterMapping `json:"parameterMappings,omitempty"`   // Parameter mappings for the workflow call
	BoundaryEvents      []*BoundaryEvent    `json:"boundaryEvents,omitempty"`      // Boundary events (e.g., timers)
}

// ActivityType returns the type name.
func (a *CallWorkflowActivity) ActivityType() string { return "CallWorkflow" }

// ExclusiveSplitActivity represents a decision (exclusive split) in a workflow.
type ExclusiveSplitActivity struct {
	BaseWorkflowActivity
	Expression string             `json:"expression,omitempty"` // Decision expression
	Outcomes   []ConditionOutcome `json:"outcomes,omitempty"`   // Condition-based outcomes
}

// ActivityType returns the type name.
func (a *ExclusiveSplitActivity) ActivityType() string { return "Decision" }

// ParallelSplitActivity represents a parallel split in a workflow.
type ParallelSplitActivity struct {
	BaseWorkflowActivity
	Outcomes []*ParallelSplitOutcome `json:"outcomes,omitempty"` // Parallel branches
}

// ActivityType returns the type name.
func (a *ParallelSplitActivity) ActivityType() string { return "ParallelSplit" }

// JumpToActivity represents a jump to another activity in a workflow.
type JumpToActivity struct {
	BaseWorkflowActivity
	TargetActivity string `json:"targetActivity,omitempty"` // Name of target activity
}

// ActivityType returns the type name.
func (a *JumpToActivity) ActivityType() string { return "JumpTo" }

// WaitForTimerActivity represents waiting for a timer.
type WaitForTimerActivity struct {
	BaseWorkflowActivity
	DelayExpression string `json:"delayExpression,omitempty"`
}

// ActivityType returns the type name.
func (a *WaitForTimerActivity) ActivityType() string { return "WaitForTimer" }

// WaitForNotificationActivity represents waiting for a notification.
type WaitForNotificationActivity struct {
	BaseWorkflowActivity
	BoundaryEvents []*BoundaryEvent `json:"boundaryEvents,omitempty"` // Boundary events (e.g., timers)
}

// ActivityType returns the type name.
func (a *WaitForNotificationActivity) ActivityType() string { return "WaitForNotification" }

// NotificationActivity is an intermediate notification event on a flow — the
// point a `notify workflow … target` reaches.
type NotificationActivity struct {
	BaseWorkflowActivity
}

// ActivityType returns the type name.
func (a *NotificationActivity) ActivityType() string { return "Notification" }

// EventSubProcess is a flow outside the main flow, started by its own start event
// while the workflow runs. Its flow's first activity is the
// EventSubProcessStartActivity.
type EventSubProcess struct {
	model.BaseElement
	Name       string `json:"name,omitempty"`
	Caption    string `json:"caption,omitempty"`
	Annotation string `json:"annotation,omitempty"`
	Flow       *Flow  `json:"flow,omitempty"`
}

// Start returns the sub-process's start event, nil when its flow has none.
func (e *EventSubProcess) Start() *EventSubProcessStartActivity {
	if e == nil || e.Flow == nil || len(e.Flow.Activities) == 0 {
		return nil
	}
	s, _ := e.Flow.Activities[0].(*EventSubProcessStartActivity)
	return s
}

// EventSubProcessStartActivity is one of Mendix's four event sub-process start
// types: interrupting or not, triggered by a notification or a timer.
type EventSubProcessStartActivity struct {
	BaseWorkflowActivity
	Interrupting       bool   `json:"interrupting"`
	Timer              bool   `json:"timer"`
	FirstExecutionTime string `json:"firstExecutionTime,omitempty"` // timer starts only
	// Recurrence repeats a non-interrupting timer start; nil fires it once.
	Recurrence *Recurrence `json:"recurrence,omitempty"`
}

// ActivityType returns the type name, e.g. "InterruptingNotificationEventSubProcessStart".
func (a *EventSubProcessStartActivity) ActivityType() string {
	return strings.TrimSuffix(strings.TrimPrefix(a.StorageType(), "Workflows$"), "Activity")
}

// StorageType returns the $Type the start event is stored under.
func (a *EventSubProcessStartActivity) StorageType() string {
	kind := "Notification"
	if a.Timer {
		kind = "Timer"
	}
	prefix := "NonInterrupting"
	if a.Interrupting {
		prefix = "Interrupting"
	}
	return "Workflows$" + prefix + kind + "EventSubProcessStartActivity"
}

// EventSubProcessStartFromStorageType is StorageType's inverse; ok is false for
// any other $Type.
func EventSubProcessStartFromStorageType(typeName string) (interrupting, timer, ok bool) {
	switch typeName {
	case "Workflows$InterruptingNotificationEventSubProcessStartActivity":
		return true, false, true
	case "Workflows$NonInterruptingNotificationEventSubProcessStartActivity":
		return false, false, true
	case "Workflows$InterruptingTimerEventSubProcessStartActivity":
		return true, true, true
	case "Workflows$NonInterruptingTimerEventSubProcessStartActivity":
		return false, true, true
	}
	return false, false, false
}

// EndOfParallelSplitPathActivity marks the end of a parallel split path (auto-generated by Mendix).
type EndOfParallelSplitPathActivity struct {
	BaseWorkflowActivity
}

// ActivityType returns the type name.
func (a *EndOfParallelSplitPathActivity) ActivityType() string { return "EndOfParallelSplitPath" }

// EndParallelSplitPath returns a flow that ends with the end-of-path marker.
//
// The marker is not decoration. Mendix's workflow engine executes a parallel
// split path up to its EndOfParallelSplitPathActivity; a path stored without one
// runs from the split straight to a synthesised end, and every activity inside
// it is skipped — no error at check, at build, or at runtime, and no activity
// record for the skipped steps. Measured on Mendix 11.14.0 with the marker as the
// only variable between two instances of the same workflow: without it, neither
// path's call-microflow ran; with it, both did (ako/view-entity-examples §6).
//
// It is idempotent, and it does not append after an activity that already ends
// the path (a jump, or an end-of-workflow), where a marker would be unreachable.
// A nil flow — an empty path — gets a flow holding only the marker, which is how
// an empty path is stored.
//
// The marker's type is introduced in Mendix 10.21; before it a path is its
// activities alone, and a project holding the marker does not load. The marker
// is added on every version here and dropped below 10.21 on the way to storage
// (canon.LowerToProjectShape), so the builders need not know the version.
func EndParallelSplitPath(flow *Flow, newID func() model.ID) *Flow {
	if flow == nil {
		flow = &Flow{}
		flow.ID = newID()
	}
	if n := len(flow.Activities); n > 0 {
		switch flow.Activities[n-1].(type) {
		case *EndOfParallelSplitPathActivity, *JumpToActivity, *EndWorkflowActivity:
			return flow
		}
	}
	end := &EndOfParallelSplitPathActivity{}
	end.ID = newID()
	end.Caption = "End of parallel split path"
	end.Name = "EndOfParallelSplitPath"
	flow.Activities = append(flow.Activities, end)
	return flow
}

// EndBoundaryEventPath returns a flow that ends with the boundary-event
// end-of-path marker — the boundary-event counterpart of EndParallelSplitPath,
// and for a sharper reason.
//
// A boundary event path that does not end in a jump, an end-of-workflow or this
// marker fails in two different places depending on the event kind, measured on
// Mendix 11.14.0:
//
//   - interrupting: mxbuild refuses it, CE0105 "Call microflow cannot be the
//     last object of a flow, it should end with a jump or end activity";
//   - non-interrupting: mxbuild ACCEPTS it at 0 errors, and the runtime then
//     refuses to START THE WHOLE APPLICATION — "Expected the flow to end with an
//     end event. But it ends with ModelCallMicroflowActivity(…)".
//
// Appending the marker cleared both. mxbuild also confirms it is a placed,
// validated activity: out of position it is CE6692 ("can only be placed at the
// end of a boundary path"), and two with one name are CE0495.
func EndBoundaryEventPath(flow *Flow, newID func() model.ID) *Flow {
	if flow == nil {
		flow = &Flow{}
		flow.ID = newID()
	}
	if n := len(flow.Activities); n > 0 {
		switch flow.Activities[n-1].(type) {
		case *EndOfBoundaryEventPathActivity, *JumpToActivity, *EndWorkflowActivity:
			return flow
		}
	}
	end := &EndOfBoundaryEventPathActivity{}
	end.ID = newID()
	end.Caption = "End of boundary event path"
	end.Name = "EndOfBoundaryEventPath"
	flow.Activities = append(flow.Activities, end)
	return flow
}

// EndInterruptingBoundaryPaths gives each interrupting boundary-event path that
// runs to its last activity the End Studio Pro writes there, in place of the
// end-of-path marker EndBoundaryEventPath appended, wherever an End may stand
// (ako/mxcli#1712).
//
// The marker is Studio Pro's terminator for a NON-interrupting path; for an
// interrupting one its constructor writes an End outside a parallel split and a
// jump inside one. mxbuild builds both, and on the 11.13.0 runtime both complete
// the instance without running what follows the activity — so this changes what
// Studio Pro shows, not what runs. Measured clean on 11.15.0: an End closing an
// interrupting path in the main flow, under a decision outcome, on a call
// microflow and in an event sub-process.
//
// endAllowed is false where an End is CE1844: under a parallel split path or a
// non-interrupting boundary path, at any depth (PROPOSAL_workflow_end_activity,
// rows 5, 6 and 13). A path there keeps the marker, which builds: Studio Pro
// would aim a jump, and only the author knows where. A bare `timer` (no kind)
// keeps it too.
func EndInterruptingBoundaryPaths(activities []WorkflowActivity, endAllowed bool, newID func() model.ID) {
	for _, act := range activities {
		for _, be := range boundaryEventsOf(act) {
			if be == nil || be.Flow == nil {
				continue
			}
			interrupting := strings.HasPrefix(be.EventType, "Interrupting")
			if interrupting && endAllowed {
				be.Flow.Activities = EndInterruptingPath(be.Flow.Activities, newID)
			}
			EndInterruptingBoundaryPaths(be.Flow.Activities, endAllowed && !be.IsNonInterrupting(), newID)
		}
		switch a := act.(type) {
		case *UserTask:
			for _, o := range a.Outcomes {
				if o != nil && o.Flow != nil {
					EndInterruptingBoundaryPaths(o.Flow.Activities, endAllowed, newID)
				}
			}
		case *CallMicroflowTask:
			endInConditionOutcomes(a.Outcomes, endAllowed, newID)
		case *SystemTask:
			endInConditionOutcomes(a.Outcomes, endAllowed, newID)
		case *ExclusiveSplitActivity:
			endInConditionOutcomes(a.Outcomes, endAllowed, newID)
		case *ParallelSplitActivity:
			for _, o := range a.Outcomes {
				if o != nil && o.Flow != nil {
					EndInterruptingBoundaryPaths(o.Flow.Activities, false, newID)
				}
			}
		}
	}
}

func endInConditionOutcomes(outcomes []ConditionOutcome, endAllowed bool, newID func() model.ID) {
	for _, o := range outcomes {
		if o != nil && o.GetFlow() != nil {
			EndInterruptingBoundaryPaths(o.GetFlow().Activities, endAllowed, newID)
		}
	}
}

// boundaryEventsOf is the boundary events an activity owns.
func boundaryEventsOf(act WorkflowActivity) []*BoundaryEvent {
	switch a := act.(type) {
	case *UserTask:
		return a.BoundaryEvents
	case *CallMicroflowTask:
		return a.BoundaryEvents
	case *CallWorkflowActivity:
		return a.BoundaryEvents
	case *WaitForNotificationActivity:
		return a.BoundaryEvents
	}
	return nil
}

// EndInterruptingPath ends an interrupting boundary path the way Studio Pro
// does: an End, replacing the end-of-path marker if one is last, unless the path
// already ends in a jump or an End.
func EndInterruptingPath(activities []WorkflowActivity, newID func() model.ID) []WorkflowActivity {
	if n := len(activities); n > 0 {
		switch activities[n-1].(type) {
		case *JumpToActivity, *EndWorkflowActivity:
			return activities
		case *EndOfBoundaryEventPathActivity:
			activities = activities[:n-1]
		}
	}
	end := &EndWorkflowActivity{}
	end.ID = newID()
	end.Caption = "End"
	end.Name = "End"
	return append(activities, end)
}

// EndOfBoundaryEventPathActivity marks the end of a boundary event path (auto-generated by Mendix).
type EndOfBoundaryEventPathActivity struct {
	BaseWorkflowActivity
}

// ActivityType returns the type name.
func (a *EndOfBoundaryEventPathActivity) ActivityType() string { return "EndOfBoundaryEventPath" }

// WorkflowAnnotationActivity represents a standalone annotation (sticky note) on the workflow canvas.
type WorkflowAnnotationActivity struct {
	BaseWorkflowActivity
	Description string `json:"description,omitempty"`
}

// ActivityType returns the type name.
func (a *WorkflowAnnotationActivity) ActivityType() string { return "WorkflowAnnotation" }

// GenericWorkflowActivity is a fallback for unknown activity types.
type GenericWorkflowActivity struct {
	BaseWorkflowActivity
	TypeString string `json:"typeString,omitempty"`
}

// ActivityType returns the type name.
func (a *GenericWorkflowActivity) ActivityType() string { return a.TypeString }

// ============================================================================
// Outcomes
// ============================================================================

// UserTaskOutcome represents an outcome of a user task.
type UserTaskOutcome struct {
	model.BaseElement
	Name    string `json:"name,omitempty"`
	Caption string `json:"caption,omitempty"`
	Value   string `json:"value,omitempty"` // The outcome value (required, must be unique)
	Flow    *Flow  `json:"flow,omitempty"`  // Activities that follow this outcome
}

// ConditionOutcome is the interface for condition-based outcomes.
type ConditionOutcome interface {
	GetName() string
	GetFlow() *Flow
}

// BooleanConditionOutcome represents a boolean condition outcome.
type BooleanConditionOutcome struct {
	model.BaseElement
	Value bool  `json:"value"`
	Flow  *Flow `json:"flow,omitempty"`
}

// GetName returns a display name for the outcome.
func (o *BooleanConditionOutcome) GetName() string {
	if o.Value {
		return "true"
	}
	return "false"
}

// GetFlow returns the flow for this outcome.
func (o *BooleanConditionOutcome) GetFlow() *Flow { return o.Flow }

// EnumerationValueConditionOutcome represents an enumeration-based condition outcome.
type EnumerationValueConditionOutcome struct {
	model.BaseElement
	Value string `json:"value,omitempty"` // Enumeration value qualified name
	Flow  *Flow  `json:"flow,omitempty"`
}

// GetName returns the enumeration value name as a single-quoted MDL string literal.
func (o *EnumerationValueConditionOutcome) GetName() string {
	escaped := strings.ReplaceAll(o.Value, "'", "''")
	return "'" + escaped + "'"
}

// GetFlow returns the flow for this outcome.
func (o *EnumerationValueConditionOutcome) GetFlow() *Flow { return o.Flow }

// VoidConditionOutcome represents a default/else outcome.
type VoidConditionOutcome struct {
	model.BaseElement
	Flow *Flow `json:"flow,omitempty"`
}

// GetName returns a display name for the default outcome.
func (o *VoidConditionOutcome) GetName() string { return "DEFAULT" }

// GetFlow returns the flow for this outcome.
func (o *VoidConditionOutcome) GetFlow() *Flow { return o.Flow }

// ParallelSplitOutcome represents a branch in a parallel split.
type ParallelSplitOutcome struct {
	model.BaseElement
	Flow *Flow `json:"flow,omitempty"`
}

// ============================================================================
// Boundary Events
// ============================================================================

// BoundaryEvent represents a boundary event attached to a workflow activity.
type BoundaryEvent struct {
	model.BaseElement
	Name       string `json:"name,omitempty"` // notification events: what `notify workflow … target` names
	Caption    string `json:"caption,omitempty"`
	Flow       *Flow  `json:"flow,omitempty"`       // Activities triggered by the boundary event
	TimerDelay string `json:"timerDelay,omitempty"` // Timer delay expression (for timer boundary events)
	// EventType is "InterruptingTimer", "NonInterruptingTimer", "Timer",
	// "InterruptingNotification" or "NonInterruptingNotification".
	EventType string `json:"eventType,omitempty"`
	// Recurrence repeats a non-interrupting timer; nil fires it once.
	Recurrence *Recurrence `json:"recurrence,omitempty"`
}

// Recurrence is a non-interrupting timer's repetition, Workflows$LinearRecurrence
// (ako/mxcli#1716): after its first firing it fires every Interval IntervalType,
// MaxExecutions times in all — the first firing included. Studio Pro allows
// IntervalType Minute, Hour, Day, Week or Month (MW0005), Interval at least 1
// and MaxExecutions at least 2 (MW0004); mx check enforces none of them.
type Recurrence struct {
	IntervalType  string `json:"intervalType"`
	Interval      int    `json:"interval"`
	MaxExecutions int    `json:"maxExecutions"`
}

// recurrenceUnits maps the unit words MDL accepts, singular and plural, to the
// stored IntervalType: the scheduled-event enumeration the runtime reads it as
// (com.mendix.model.scheduledevents.ModelIntervalType, 11.15.0). Second and
// Year are in that enumeration but not allowed for a recurrence.
var recurrenceUnits = map[string]string{
	"second": "Second", "minute": "Minute", "hour": "Hour", "day": "Day",
	"week": "Week", "month": "Month", "year": "Year",
}

// RecurrenceIntervalType is the stored IntervalType for a unit word — "day",
// "Days", … — and whether the word names one.
func RecurrenceIntervalType(word string) (string, bool) {
	w := strings.ToLower(word)
	if t, ok := recurrenceUnits[w]; ok {
		return t, true
	}
	t, ok := recurrenceUnits[strings.TrimSuffix(w, "s")]
	return t, ok
}

// RecurrenceUnitWord is how MDL writes an IntervalType after a count:
// "day" for 1, "days" otherwise.
func RecurrenceUnitWord(intervalType string, count int) string {
	w := strings.ToLower(intervalType)
	if count != 1 {
		w += "s"
	}
	return w
}

// boundaryEventStorageTypes maps EventType to the stored $Type. Every writer
// reads it from here: three copies of this switch defaulted an unknown kind to
// an interrupting TIMER, so a notification event would have been written as one.
var boundaryEventStorageTypes = map[string]string{
	"InterruptingTimer":           "Workflows$InterruptingTimerBoundaryEvent",
	"NonInterruptingTimer":        "Workflows$NonInterruptingTimerBoundaryEvent",
	"Timer":                       "Workflows$TimerBoundaryEvent",
	"InterruptingNotification":    "Workflows$InterruptingNotificationBoundaryEvent",
	"NonInterruptingNotification": "Workflows$NonInterruptingNotificationBoundaryEvent",
}

// MaxBoundaryEventsPerActivity is the most boundary events one activity may
// carry. mx check 11.12.4 reports CE6696 "An activity is not allowed to have
// more than 5 boundary events." on a sixth (ako/mxcli#2242).
const MaxBoundaryEventsPerActivity = 5

// BoundaryEventStorageType returns the $Type for an EventType. An empty
// EventType is an interrupting timer, which is what the writers always assumed;
// ok is false for anything else unknown.
func BoundaryEventStorageType(eventType string) (typeName string, ok bool) {
	if eventType == "" {
		eventType = "InterruptingTimer"
	}
	typeName, ok = boundaryEventStorageTypes[eventType]
	return typeName, ok
}

// BoundaryEventTypeFromStorage is BoundaryEventStorageType's inverse.
func BoundaryEventTypeFromStorage(typeName string) (eventType string, ok bool) {
	for k, v := range boundaryEventStorageTypes {
		if v == typeName {
			return k, true
		}
	}
	return "", false
}

// IsNotification reports whether the event is triggered by a notification.
func (b *BoundaryEvent) IsNotification() bool {
	return strings.HasSuffix(b.EventType, "Notification")
}

// IsNonInterrupting reports whether the event runs alongside its activity.
func (b *BoundaryEvent) IsNonInterrupting() bool {
	return strings.HasPrefix(b.EventType, "NonInterrupting")
}

// ============================================================================
// User Sources
// ============================================================================

// UserSource is the interface for user targeting strategies.
type UserSource interface {
	UserSourceType() string
}

// NoUserSource indicates no user targeting.
type NoUserSource struct{}

// UserSourceType returns the type name.
func (s *NoUserSource) UserSourceType() string { return "None" }

// MicroflowBasedUserSource targets users via a microflow.
type MicroflowBasedUserSource struct {
	Microflow string `json:"microflow,omitempty"` // Qualified name of the targeting microflow
}

// UserSourceType returns the type name.
func (s *MicroflowBasedUserSource) UserSourceType() string { return "Microflow" }

// XPathBasedUserSource targets users via an XPath expression.
type XPathBasedUserSource struct {
	XPath string `json:"xpath,omitempty"`
}

// UserSourceType returns the type name.
func (s *XPathBasedUserSource) UserSourceType() string { return "XPath" }

// MicroflowGroupSource targets workflow groups via a microflow.
type MicroflowGroupSource struct {
	Microflow string `json:"microflow,omitempty"` // Qualified name of the targeting microflow
}

// UserSourceType returns the type name.
func (s *MicroflowGroupSource) UserSourceType() string { return "GroupMicroflow" }

// XPathGroupSource targets workflow groups via an XPath expression.
type XPathGroupSource struct {
	XPath string `json:"xpath,omitempty"`
}

// UserSourceType returns the type name.
func (s *XPathGroupSource) UserSourceType() string { return "GroupXPath" }

// ============================================================================
// Parameter Mapping
// ============================================================================

// ParameterMapping maps a parameter to an expression value.
type ParameterMapping struct {
	model.BaseElement
	Parameter  string `json:"parameter,omitempty"`  // Parameter name
	Expression string `json:"expression,omitempty"` // Expression for the value
}
