// SPDX-License-Identifier: Apache-2.0

// Package microflows provides types for Mendix microflows and nanoflows.
package microflows

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// Microflow represents a microflow in the Mendix model.
type Microflow struct {
	model.BaseElement
	ContainerID              model.ID `json:"containerId"`
	Name                     string   `json:"name"`
	Documentation            string   `json:"documentation,omitempty"`
	AllowConcurrentExecution bool     `json:"allowConcurrentExecution"`
	MarkAsUsed               bool     `json:"markAsUsed"`
	Excluded                 bool     `json:"excluded"`

	// Return type
	ReturnType         DataType `json:"returnType,omitempty"`
	ReturnVariableName string   `json:"returnVariableName,omitempty"` // Variable name for return value (e.g., "$Result")

	// Parameters
	Parameters []*MicroflowParameter `json:"parameters,omitempty"`

	// Flow elements
	ObjectCollection *MicroflowObjectCollection `json:"objectCollection,omitempty"`

	// Allowed module roles for execution
	AllowedModuleRoles []model.ID `json:"allowedModuleRoles,omitempty"`

	// Concurrent execution settings
	ConcurrentExecutionSettings *ConcurrentExecutionSettings `json:"concurrentExecutionSettings,omitempty"`
}

// GetName returns the microflow's name.
func (m *Microflow) GetName() string {
	return m.Name
}

// GetContainerID returns the ID of the containing folder/module.
func (m *Microflow) GetContainerID() model.ID {
	return m.ContainerID
}

// Nanoflow represents a nanoflow in the Mendix model.
// Nanoflows run on the client side and have restrictions on which activities can be used.
type Nanoflow struct {
	model.BaseElement
	ContainerID        model.ID   `json:"containerId"`
	Name               string     `json:"name"`
	Documentation      string     `json:"documentation,omitempty"`
	MarkAsUsed         bool       `json:"markAsUsed"`
	Excluded           bool       `json:"excluded"`
	AllowedModuleRoles []model.ID `json:"allowedModuleRoles,omitempty"`

	// Return type
	ReturnType DataType `json:"returnType,omitempty"`

	// Parameters
	Parameters []*MicroflowParameter `json:"parameters,omitempty"`

	// Flow elements
	ObjectCollection *MicroflowObjectCollection `json:"objectCollection,omitempty"`
}

// GetName returns the nanoflow's name.
func (n *Nanoflow) GetName() string {
	return n.Name
}

// GetContainerID returns the ID of the containing folder/module.
func (n *Nanoflow) GetContainerID() model.ID {
	return n.ContainerID
}

// Rule represents a rule in the Mendix model.
type Rule struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	Documentation string   `json:"documentation,omitempty"`

	// Return type (always boolean)
	ReturnType DataType `json:"returnType,omitempty"`

	// Parameters
	Parameters []*MicroflowParameter `json:"parameters,omitempty"`

	// Flow elements
	ObjectCollection *MicroflowObjectCollection `json:"objectCollection,omitempty"`
}

// GetName returns the rule's name.
func (r *Rule) GetName() string {
	return r.Name
}

// GetContainerID returns the ID of the containing folder/module.
func (r *Rule) GetContainerID() model.ID {
	return r.ContainerID
}

// MicroflowParameter represents a parameter of a microflow.
type MicroflowParameter struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	Documentation string   `json:"documentation,omitempty"`
	Type          DataType `json:"type"`
}

// GetName returns the parameter's name.
func (p *MicroflowParameter) GetName() string {
	return p.Name
}

// GetContainerID returns the ID of the containing microflow.
func (p *MicroflowParameter) GetContainerID() model.ID {
	return p.ContainerID
}

// MicroflowObjectCollection contains all objects and flows in a microflow.
type MicroflowObjectCollection struct {
	model.BaseElement
	Objects         []MicroflowObject `json:"objects,omitempty"`
	Flows           []*SequenceFlow   `json:"flows,omitempty"`
	AnnotationFlows []*AnnotationFlow `json:"annotationFlows,omitempty"`
}

// MicroflowObject is the base interface for all microflow objects.
type MicroflowObject interface {
	GetID() model.ID
	GetPosition() model.Point
	SetPosition(p model.Point)
}

// BaseMicroflowObject provides common fields for microflow objects.
type BaseMicroflowObject struct {
	model.BaseElement
	Position       model.Point `json:"position"`
	Size           model.Size  `json:"size,omitempty"`
	RelativeMiddle model.Point `json:"relativeMiddle,omitempty"`
}

// GetSize returns the object's box size.
func (o *BaseMicroflowObject) GetSize() model.Size {
	return o.Size
}

// SetSize sets the object's box size.
func (o *BaseMicroflowObject) SetSize(s model.Size) {
	o.Size = s
}

// GetPosition returns the object's position.
func (o *BaseMicroflowObject) GetPosition() model.Point {
	return o.Position
}

// SetPosition sets the object's position.
func (o *BaseMicroflowObject) SetPosition(p model.Point) {
	o.Position = p
}

// SequenceFlow represents a flow connection between objects.
type SequenceFlow struct {
	model.BaseElement
	OriginID                   model.ID  `json:"originId"`
	DestinationID              model.ID  `json:"destinationId"`
	OriginConnectionIndex      int       `json:"originConnectionIndex"`
	DestinationConnectionIndex int       `json:"destinationConnectionIndex"`
	CaseValue                  CaseValue `json:"caseValue,omitempty"`
	IsErrorHandler             bool      `json:"isErrorHandler,omitempty"`
	OriginControlVector        string    `json:"originControlVector,omitempty"`
	DestinationControlVector   string    `json:"destinationControlVector,omitempty"`
}

// CaseValue represents a case value for a decision flow.
type CaseValue interface {
	isCaseValue()
}

// NoCase represents no case (default flow).
type NoCase struct {
	model.BaseElement
}

func (NoCase) isCaseValue() {}

// EnumerationCase represents an enumeration case value.
type EnumerationCase struct {
	model.BaseElement
	Value string `json:"value"`
}

func (EnumerationCase) isCaseValue() {}

// InheritanceCase represents an inheritance/type case value.
type InheritanceCase struct {
	model.BaseElement
	EntityID            model.ID `json:"entityId"`
	EntityQualifiedName string   `json:"entityQualifiedName,omitempty"`
}

func (InheritanceCase) isCaseValue() {}

// BooleanCase represents a boolean case value.
type BooleanCase struct {
	model.BaseElement
	Value bool `json:"value"`
}

func (BooleanCase) isCaseValue() {}

// ExpressionCase represents an expression-based case value (true/false branches).
type ExpressionCase struct {
	model.BaseElement
	Expression string `json:"expression"`
}

func (ExpressionCase) isCaseValue() {}

// Annotation represents an annotation in a microflow.
type Annotation struct {
	BaseMicroflowObject
	Caption string `json:"caption"`
}

// AnnotationFlow connects an annotation to an object.
type AnnotationFlow struct {
	model.BaseElement
	OriginID      model.ID `json:"originId"`
	DestinationID model.ID `json:"destinationId"`
}

// Events

// StartEvent represents the start of a microflow.
type StartEvent struct {
	BaseMicroflowObject
}

// EndEvent represents the end of a microflow.
type EndEvent struct {
	BaseMicroflowObject
	ReturnValue string `json:"returnValue,omitempty"`
}

// ContinueEvent represents a continue in a loop.
type ContinueEvent struct {
	BaseMicroflowObject
}

// BreakEvent represents a break in a loop.
type BreakEvent struct {
	BaseMicroflowObject
}

// ErrorEvent represents an error throw in a microflow.
type ErrorEvent struct {
	BaseMicroflowObject
}

// Decisions and Control Flow

// ExclusiveSplit represents an exclusive decision (if/else).
type ExclusiveSplit struct {
	BaseMicroflowObject
	Caption           string            `json:"caption,omitempty"`
	Documentation     string            `json:"documentation,omitempty"`
	SplitCondition    SplitCondition    `json:"splitCondition,omitempty"`
	ErrorHandlingType ErrorHandlingType `json:"errorHandlingType,omitempty"`
}

// ExclusiveMerge represents a merge point for exclusive splits.
type ExclusiveMerge struct {
	BaseMicroflowObject
}

// InheritanceSplit represents a type-based decision.
type InheritanceSplit struct {
	BaseMicroflowObject
	Caption           string            `json:"caption,omitempty"`
	Documentation     string            `json:"documentation,omitempty"`
	VariableName      string            `json:"variableName"`
	ErrorHandlingType ErrorHandlingType `json:"errorHandlingType,omitempty"`
}

// SplitCondition represents the condition for a split.
type SplitCondition interface {
	isSplitCondition()
}

// ExpressionSplitCondition represents an expression-based split condition.
type ExpressionSplitCondition struct {
	model.BaseElement
	Expression string `json:"expression"`
}

func (ExpressionSplitCondition) isSplitCondition() {}

// RuleSplitCondition represents a rule-based split condition.
// In Mendix BSON the rule is referenced by qualified name under the
// "RuleCall.Microflow" field (rules and microflows share a namespace).
type RuleSplitCondition struct {
	model.BaseElement
	RuleQualifiedName string                      `json:"ruleQualifiedName,omitempty"`
	ParameterMappings []*RuleCallParameterMapping `json:"parameterMappings,omitempty"`
}

func (RuleSplitCondition) isSplitCondition() {}

// RuleCallParameterMapping maps a parameter to a value.
// ParameterName is the fully qualified parameter name (e.g. "Module.Rule.ParamName")
// as stored in BSON; used when the describer renders the rule call.
type RuleCallParameterMapping struct {
	model.BaseElement
	ParameterID   model.ID `json:"parameterId"`
	ParameterName string   `json:"parameterName,omitempty"`
	Argument      string   `json:"argument"`
}

// LoopSource is the source for a LoopedActivity. Either IterableList (FOR EACH) or WhileLoopCondition (WHILE).
type LoopSource interface {
	isLoopSource()
}

// LoopedActivity represents a loop construct (FOR EACH or WHILE).
type LoopedActivity struct {
	BaseMicroflowObject
	Caption           string                     `json:"caption,omitempty"`
	Documentation     string                     `json:"documentation,omitempty"`
	LoopSource        LoopSource                 `json:"loopSource,omitempty"`
	ObjectCollection  *MicroflowObjectCollection `json:"objectCollection,omitempty"`
	ErrorHandlingType ErrorHandlingType          `json:"errorHandlingType,omitempty"`
}

// IterableList represents the source for a FOR EACH loop iteration.
type IterableList struct {
	model.BaseElement
	ListVariableName string `json:"listVariableName"` // The list to iterate over
	VariableName     string `json:"variableName"`     // The iterator variable name
}

func (*IterableList) isLoopSource() {}

// WhileLoopCondition represents the source for a WHILE loop.
type WhileLoopCondition struct {
	model.BaseElement
	WhileExpression string `json:"whileExpression"` // The condition expression
}

func (*WhileLoopCondition) isLoopSource() {}

// ErrorHandlingType represents how errors are handled.
type ErrorHandlingType string

const (
	ErrorHandlingTypeAbort                 ErrorHandlingType = "Abort"
	ErrorHandlingTypeContinue              ErrorHandlingType = "Continue"
	ErrorHandlingTypeCustom                ErrorHandlingType = "Custom"
	ErrorHandlingTypeCustomWithoutRollback ErrorHandlingType = "CustomWithoutRollBack"
	ErrorHandlingTypeRollback              ErrorHandlingType = "Rollback"
)

// Activities

// Activity is the base interface for all activities.
type Activity interface {
	MicroflowObject
	IsActivity()
}

// BaseActivity provides common fields for activities.
type BaseActivity struct {
	BaseMicroflowObject
	Caption             string            `json:"caption,omitempty"`
	Documentation       string            `json:"documentation,omitempty"`
	ErrorHandlingType   ErrorHandlingType `json:"errorHandlingType,omitempty"`
	AutoGenerateCaption bool              `json:"autoGenerateCaption"`
	BackgroundColor     string            `json:"backgroundColor,omitempty"`
	Disabled            bool              `json:"disabled"` // @excluded in MDL
}

// IsActivity marks this as an activity.
func (a *BaseActivity) IsActivity() {}

// ActionActivity wraps an action.
type ActionActivity struct {
	BaseActivity
	Action MicroflowAction `json:"action,omitempty"`
}

// ConcurrentExecutionSettings represents settings for concurrent execution.
type ConcurrentExecutionSettings struct {
	model.BaseElement
	Enabled         bool `json:"enabled"`
	NumberOfThreads int  `json:"numberOfThreads,omitempty"`
}
