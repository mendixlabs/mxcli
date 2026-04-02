// SPDX-License-Identifier: Apache-2.0

package ast

// CreateWorkflowStmt represents: CREATE WORKFLOW Module.Name ...
type CreateWorkflowStmt struct {
	Name           QualifiedName
	CreateOrModify bool
	Documentation  string

	// Context parameter entity
	ParameterVar    string        // e.g. "$WorkflowContext"
	ParameterEntity QualifiedName // e.g. Module.Entity

	// Display metadata
	DisplayName string // from DISPLAY 'text'
	Description string // from DESCRIPTION 'text'
	ExportLevel string // "Hidden" or "API", from EXPORT LEVEL identifier

	// Optional metadata
	OverviewPage QualifiedName // qualified name of overview page
	DueDate      string        // due date expression

	// Activities
	Activities []WorkflowActivityNode
}

func (s *CreateWorkflowStmt) isStatement() {}

// DropWorkflowStmt represents: DROP WORKFLOW Module.Name
type DropWorkflowStmt struct {
	Name QualifiedName
}

func (s *DropWorkflowStmt) isStatement() {}

// WorkflowActivityNode is the interface for workflow activity AST nodes.
type WorkflowActivityNode interface {
	workflowActivityNode()
}

// WorkflowUserTaskNode represents a USER TASK activity.
type WorkflowUserTaskNode struct {
	Name            string // identifier name
	Caption         string // display caption
	Page            QualifiedName
	Targeting       WorkflowTargetingNode
	Entity          QualifiedName // user task entity
	DueDate         string        // DUE DATE expression
	Outcomes        []WorkflowUserTaskOutcomeNode
	IsMultiUser     bool                        // Issue #8: true if MULTI USER TASK
	BoundaryEvents  []WorkflowBoundaryEventNode // Issue #7
	TaskDescription string                      // from DESCRIPTION 'text'
}

func (n *WorkflowUserTaskNode) workflowActivityNode() {}

// WorkflowTargetingNode represents user targeting strategy.
type WorkflowTargetingNode struct {
	Kind      string        // "microflow", "xpath", or ""
	Microflow QualifiedName // for microflow targeting
	XPath     string        // for xpath targeting
}

// WorkflowUserTaskOutcomeNode represents an outcome of a user task.
type WorkflowUserTaskOutcomeNode struct {
	Caption    string
	Activities []WorkflowActivityNode
}

// WorkflowCallMicroflowNode represents a CALL MICROFLOW activity.
type WorkflowCallMicroflowNode struct {
	Microflow         QualifiedName
	Caption           string
	Outcomes          []WorkflowConditionOutcomeNode
	ParameterMappings []WorkflowParameterMappingNode // Issue #10
	BoundaryEvents    []WorkflowBoundaryEventNode    // Issue #7
}

func (n *WorkflowCallMicroflowNode) workflowActivityNode() {}

// WorkflowCallWorkflowNode represents a CALL WORKFLOW activity.
type WorkflowCallWorkflowNode struct {
	Workflow          QualifiedName
	Caption           string
	ParameterMappings []WorkflowParameterMappingNode
}

func (n *WorkflowCallWorkflowNode) workflowActivityNode() {}

// WorkflowDecisionNode represents a DECISION activity.
type WorkflowDecisionNode struct {
	Expression string // decision expression
	Caption    string
	Outcomes   []WorkflowConditionOutcomeNode
}

func (n *WorkflowDecisionNode) workflowActivityNode() {}

// WorkflowConditionOutcomeNode represents an outcome of a decision or call microflow.
type WorkflowConditionOutcomeNode struct {
	Value      string // "True", "False", "Default", or enumeration value
	Activities []WorkflowActivityNode
}

// WorkflowParallelSplitNode represents a PARALLEL SPLIT activity.
type WorkflowParallelSplitNode struct {
	Caption string
	Paths   []WorkflowParallelPathNode
}

func (n *WorkflowParallelSplitNode) workflowActivityNode() {}

// WorkflowParallelPathNode represents a path in a parallel split.
type WorkflowParallelPathNode struct {
	PathNumber int
	Activities []WorkflowActivityNode
}

// WorkflowJumpToNode represents a JUMP TO activity.
type WorkflowJumpToNode struct {
	Target  string // name of target activity
	Caption string
}

func (n *WorkflowJumpToNode) workflowActivityNode() {}

// WorkflowWaitForTimerNode represents a WAIT FOR TIMER activity.
type WorkflowWaitForTimerNode struct {
	DelayExpression string
	Caption         string
}

func (n *WorkflowWaitForTimerNode) workflowActivityNode() {}

// WorkflowWaitForNotificationNode represents a WAIT FOR NOTIFICATION activity.
type WorkflowWaitForNotificationNode struct {
	Caption        string
	BoundaryEvents []WorkflowBoundaryEventNode // Issue #7
}

func (n *WorkflowWaitForNotificationNode) workflowActivityNode() {}

// WorkflowEndNode represents an END activity.
type WorkflowEndNode struct {
	Caption string
}

func (n *WorkflowEndNode) workflowActivityNode() {}

// WorkflowBoundaryEventNode represents a BOUNDARY EVENT clause on a user task.
// Issue #7
type WorkflowBoundaryEventNode struct {
	EventType  string                 // "InterruptingTimer", "NonInterruptingTimer", "Timer"
	Delay      string                 // ISO duration expression e.g. "${PT1H}"
	Activities []WorkflowActivityNode // Sub-flow activities inside the boundary event
}

// WorkflowAnnotationActivityNode represents an ANNOTATION activity in a workflow.
// Issue #9
type WorkflowAnnotationActivityNode struct {
	Text string
}

func (n *WorkflowAnnotationActivityNode) workflowActivityNode() {}

// WorkflowParameterMappingNode represents a parameter mapping in a CALL MICROFLOW WITH clause.
// Issue #10
type WorkflowParameterMappingNode struct {
	Parameter  string // parameter name (by-name reference)
	Expression string // Mendix expression string
}
