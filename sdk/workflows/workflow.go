// SPDX-License-Identifier: Apache-2.0

// Package workflows provides types for Mendix workflows.
package workflows

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
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
	OverviewPage        string   `json:"overviewPage,omitempty"`        // Qualified name of overview page
	DueDate             string   `json:"dueDate,omitempty"`             // Due date expression
	AdminPage           string   `json:"adminPage,omitempty"`           // Qualified name of admin page

	// Annotation
	Annotation string `json:"annotation,omitempty"` // Annotation description text

	// Context parameter
	Parameter *WorkflowParameter `json:"parameter,omitempty"`

	// Flow contains the workflow activities
	Flow *Flow `json:"flow,omitempty"`
}

// GetName returns the workflow's name.
func (w *Workflow) GetName() string {
	return w.Name
}

// GetContainerID returns the ID of the containing folder/module.
func (w *Workflow) GetContainerID() model.ID {
	return w.ContainerID
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
	DueDate         string             `json:"dueDate,omitempty"`         // Due date expression
	UserTaskEntity  string             `json:"userTaskEntity,omitempty"`  // Qualified name of user task entity
	OnCreated       string             `json:"onCreated,omitempty"`       // Microflow called on task creation
	BoundaryEvents  []*BoundaryEvent   `json:"boundaryEvents,omitempty"`  // Boundary events (e.g., timers)
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

// EndOfParallelSplitPathActivity marks the end of a parallel split path (auto-generated by Mendix).
type EndOfParallelSplitPathActivity struct {
	BaseWorkflowActivity
}

// ActivityType returns the type name.
func (a *EndOfParallelSplitPathActivity) ActivityType() string { return "EndOfParallelSplitPath" }

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
	Caption    string `json:"caption,omitempty"`
	Flow       *Flow  `json:"flow,omitempty"`       // Activities triggered by the boundary event
	TimerDelay string `json:"timerDelay,omitempty"` // Timer delay expression (for timer boundary events)
	EventType  string `json:"eventType,omitempty"`  // e.g. "InterruptingTimer", "NonInterruptingTimer"
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
