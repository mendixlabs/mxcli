// SPDX-License-Identifier: Apache-2.0

package ast

// CreateWorkflowStmt represents: CREATE WORKFLOW Module.Name ...
type CreateWorkflowStmt struct {
	CreateGuard             // `create … if not exists` (ako/mxcli#731)
	Folder           string // Folder path within module (empty = leave placement alone)
	Name             QualifiedName
	CreateOrModify   bool
	Documentation    string
	DocumentationSet bool // see mendixlabs/mxcli#1018: absent preserves, empty clears

	// Context parameter entity
	ParameterVar    string        // e.g. "$WorkflowContext"
	ParameterEntity QualifiedName // e.g. Module.Entity

	// Display metadata
	DisplayName string // from DISPLAY 'text'
	Description string // from DESCRIPTION 'text'
	// DisplayParams and DescriptionParams fill the {n} placeholders of the two
	// texts: `display 'Approval for {1}' with ({1} = $WorkflowContext/Title)`.
	DisplayParams     []WorkflowTemplateParam
	DescriptionParams []WorkflowTemplateParam
	ExportLevel       string // "Hidden" or "API", from EXPORT LEVEL identifier

	// Optional metadata
	OverviewPage QualifiedName // qualified name of overview page
	DueDate      string        // due date expression

	// Workflow event handlers, in statement order.
	EventHandlers []WorkflowEventHandlerNode

	// Annotation is the note attached to the workflow's start, from the header
	// clause `annotation '…'` (ako/mxcli#707).
	Annotation string

	// Activities
	Activities []WorkflowActivityNode

	// Event sub-processes, written after the main body.
	EventSubProcesses []WorkflowEventSubProcessNode
}

// WorkflowEventSubProcessNode is `event subprocess <name> ['<caption>'] on
// [non] interrupting notification|timer … { … };`.
type WorkflowEventSubProcessNode struct {
	WorkflowActivityAnnotation
	Name         string
	Caption      string
	Interrupting bool
	Timer        bool   // a timer start; otherwise a notification start
	StartName    string // the start event's name; "" = derived from Name
	StartCaption string
	// FirstExecutionTime is a timer start's expression.
	FirstExecutionTime string
	// Recurrence is a non-interrupting timer start's `repeat every …`.
	Recurrence *WorkflowRecurrenceNode
	Activities []WorkflowActivityNode
}

// WorkflowRecurrenceNode is `repeat every <interval> <unit> at most <max> times`
// on a non-interrupting timer (ako/mxcli#1716). Unit is the word as written —
// "day", "days", … — checked by MDL-WF22.
type WorkflowRecurrenceNode struct {
	Interval      int
	Unit          string
	MaxExecutions int
}

// WorkflowEventHandlerNode is one `on workflow events (…) microflow M as '…'` or
// `on any workflow event microflow M as '…'` header clause.
type WorkflowEventHandlerNode struct {
	AnyEvent    bool          // `on any workflow event`: every type the project version knows
	EventTypes  []string      // the named types, as written; empty when AnyEvent
	Microflow   QualifiedName // the handler microflow
	Description string        // from AS 'text'
}

func (s *CreateWorkflowStmt) isStatement() {}

// DropWorkflowStmt represents: DROP WORKFLOW Module.Name
type DropWorkflowStmt struct {
	DropGuard
	Name QualifiedName
}

func (s *DropWorkflowStmt) isStatement() {}

// WorkflowActivityNode is the interface for workflow activity AST nodes.
type WorkflowActivityNode interface {
	workflowActivityNode()
}

// WorkflowActivityAnnotation is the note attached to an activity, written
// `@annotation '…'` before it (ako/mxcli#707). Every activity node embeds it.
type WorkflowActivityAnnotation struct {
	Annotation string
}

// ActivityAnnotation returns the attached note, "" when there is none.
func (a *WorkflowActivityAnnotation) ActivityAnnotation() string { return a.Annotation }

// SetActivityAnnotation sets the attached note.
func (a *WorkflowActivityAnnotation) SetActivityAnnotation(s string) { a.Annotation = s }

// AnnotatedWorkflowActivity is an activity node that carries an attached note.
type AnnotatedWorkflowActivity interface {
	ActivityAnnotation() string
	SetActivityAnnotation(string)
}

// WorkflowUserTaskNode represents a USER TASK activity.
type WorkflowUserTaskNode struct {
	WorkflowActivityAnnotation
	Name            string // identifier name
	Caption         string // display caption
	Page            QualifiedName
	Targeting       WorkflowTargetingNode
	OnCreated       QualifiedName // ON CREATED MICROFLOW: runs when the task is created
	Entity          QualifiedName // user task entity
	DueDate         string        // DUE DATE expression
	Outcomes        []WorkflowUserTaskOutcomeNode
	IsMultiUser     bool                        // Issue #8: true if MULTI USER TASK
	BoundaryEvents  []WorkflowBoundaryEventNode // Issue #7
	TaskDescription string                      // from DESCRIPTION 'text'
	// TaskName is `task name '…'`, what the task inbox shows; "" means the
	// caption, which is what a task without the clause is given.
	TaskName              string
	TaskNameParams        []WorkflowTemplateParam
	TaskDescriptionParams []WorkflowTemplateParam
	AutoAssign            bool // `auto assign single user` (AutoAssignSingleTargetUser)

	// Multi-user task only.
	Participants  *WorkflowParticipantsNode   // `participants …`; nil = all
	Completion    *WorkflowCompletionRuleNode // `decide by …`; nil = consensus on the first outcome
	AwaitAllUsers bool                        // `await all users`
}

// WorkflowParticipantsNode is `participants all | N | N percent`.
type WorkflowParticipantsNode struct {
	Kind  string // "all", "number" or "percent"
	Value int
}

// WorkflowCompletionRuleNode is `decide by …` on a multi-user task.
type WorkflowCompletionRuleNode struct {
	Rule          string // "consensus", "majority", "threshold", "veto" or "microflow"
	Majority      string // "more than half" or "most chosen"
	Threshold     int
	ThresholdUnit string // "percent" or "votes"
	Fallback      string // outcome named by `fallback '…'`; "" when absent
	HasFallback   bool
	Veto          string // outcome named by `veto '…'`
	Microflow     QualifiedName
}

func (n *WorkflowUserTaskNode) workflowActivityNode() {}

// WorkflowTargetingNode represents user targeting strategy.
// Kind: "microflow", "xpath", "group_microflow", "group_xpath", or ""
type WorkflowTargetingNode struct {
	Kind      string        // "microflow", "xpath", "group_microflow", "group_xpath", or ""
	Microflow QualifiedName // for microflow targeting (user or group)
	XPath     string        // for xpath targeting (user or group)
}

// WorkflowUserTaskOutcomeNode represents an outcome of a user task.
type WorkflowUserTaskOutcomeNode struct {
	Caption    string
	Activities []WorkflowActivityNode
}

// WorkflowCallMicroflowNode represents a CALL MICROFLOW activity.
type WorkflowCallMicroflowNode struct {
	WorkflowActivityAnnotation
	Name              string // explicit activity name (`as <name>`); see ako/mxcli#408
	Agent             bool   // `call agent microflow`: an AI agent task (Workflows$AIAgentTaskActivity)
	Microflow         QualifiedName
	Caption           string
	Outcomes          []WorkflowConditionOutcomeNode
	ParameterMappings []WorkflowParameterMappingNode // Issue #10
	BoundaryEvents    []WorkflowBoundaryEventNode    // Issue #7
}

func (n *WorkflowCallMicroflowNode) workflowActivityNode() {}

// WorkflowCallWorkflowNode represents a CALL WORKFLOW activity.
type WorkflowCallWorkflowNode struct {
	WorkflowActivityAnnotation
	Name              string // explicit activity name (`as <name>`); see ako/mxcli#408
	Workflow          QualifiedName
	Caption           string
	ParameterMappings []WorkflowParameterMappingNode
	BoundaryEvents    []WorkflowBoundaryEventNode // mendixlabs/mxcli#1532
}

func (n *WorkflowCallWorkflowNode) workflowActivityNode() {}

// WorkflowDecisionNode represents a DECISION activity.
type WorkflowDecisionNode struct {
	WorkflowActivityAnnotation
	Name       string // explicit activity name; see ako/mxcli#408
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
	WorkflowActivityAnnotation
	Name    string // explicit activity name; see ako/mxcli#408
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
	WorkflowActivityAnnotation
	Target  string // name of target activity
	Caption string
}

func (n *WorkflowJumpToNode) workflowActivityNode() {}

// WorkflowWaitForTimerNode represents a WAIT FOR TIMER activity.
type WorkflowWaitForTimerNode struct {
	WorkflowActivityAnnotation
	Name            string // explicit activity name; see ako/mxcli#408
	DelayExpression string
	Caption         string
}

func (n *WorkflowWaitForTimerNode) workflowActivityNode() {}

// WorkflowWaitForNotificationNode represents a WAIT FOR NOTIFICATION activity.
type WorkflowWaitForNotificationNode struct {
	WorkflowActivityAnnotation
	Name           string // explicit activity name; see ako/mxcli#408
	Caption        string
	BoundaryEvents []WorkflowBoundaryEventNode // Issue #7
}

func (n *WorkflowWaitForNotificationNode) workflowActivityNode() {}

// WorkflowNotificationNode is `notification [<name>] [comment '<caption>']`, an
// intermediate notification event.
type WorkflowNotificationNode struct {
	WorkflowActivityAnnotation
	Name    string
	Caption string
}

func (n *WorkflowNotificationNode) workflowActivityNode() {}

// WorkflowEndNode is `end workflow [comment '<caption>']` inside a branch: it
// ends the whole workflow there. The main flow's End is not a node — the body's
// closing `end workflow` is it.
type WorkflowEndNode struct {
	Caption string
}

func (n *WorkflowEndNode) workflowActivityNode() {}

// WorkflowReturnNode is a `return;` written in a workflow body. Nothing is built
// from it: it exists so MDL-WF11 can tell a microflow author that a workflow ends
// with `end workflow;`, and so exec can refuse it rather than drop it — dropped,
// the branch would fall through, the fault `end workflow` exists to prevent.
type WorkflowReturnNode struct{}

func (n *WorkflowReturnNode) workflowActivityNode() {}

// WorkflowBoundaryEventNode represents a BOUNDARY EVENT clause on a user task.
// Issue #7
type WorkflowBoundaryEventNode struct {
	// EventType is "InterruptingTimer", "NonInterruptingTimer", "Timer",
	// "InterruptingNotification" or "NonInterruptingNotification".
	EventType  string
	Delay      string                 // ISO duration expression e.g. "${PT1H}"
	Name       string                 // notification events: what `notify workflow … target` names
	Caption    string                 // notification events
	Activities []WorkflowActivityNode // Sub-flow activities inside the boundary event
	// Recurrence is a timer's `repeat every …` (a non-interrupting one's only).
	Recurrence *WorkflowRecurrenceNode
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

// AlterWorkflowStmt represents: ALTER WORKFLOW Module.Name operations...
type AlterWorkflowStmt struct {
	Name       QualifiedName
	Operations []AlterWorkflowOp
}

func (s *AlterWorkflowStmt) isStatement() {}

// AlterWorkflowOp is the interface for ALTER WORKFLOW operations.
type AlterWorkflowOp interface {
	alterWorkflowOp()
}

// SetWorkflowPropertyOp sets a workflow-level property.
type SetWorkflowPropertyOp struct {
	Property string        // "DISPLAY", "DESCRIPTION", "EXPORT_LEVEL", "DUE_DATE", "OVERVIEW_PAGE", "PARAMETER"
	Value    string        // string value
	Entity   QualifiedName // for OVERVIEW_PAGE / PARAMETER: the qualified name
	// Params fills a DISPLAY or DESCRIPTION text's placeholders.
	Params []WorkflowTemplateParam
}

func (o *SetWorkflowPropertyOp) alterWorkflowOp() {}

// WorkflowTemplateParam is one `{n} = <expression>` of a workflow text template
// (ako/mxcli#1714). The expression is kept as written, as every workflow
// expression is.
type WorkflowTemplateParam struct {
	Index      int
	Expression string
}

// SetActivityPropertyOp sets a property on a named activity.
type SetActivityPropertyOp struct {
	ActivityRef string        // activity caption
	AtPosition  int           // positional disambiguation (0 = no disambiguation)
	Property    string        // "PAGE", "DESCRIPTION", "TASK_NAME", "TARGETING_MICROFLOW", "TARGETING_XPATH", "DUE_DATE"
	Value       string        // string value
	PageName    QualifiedName // for PAGE property
	Microflow   QualifiedName // for TARGETING MICROFLOW
	// Params fills a TASK_NAME or DESCRIPTION text's placeholders.
	Params []WorkflowTemplateParam
}

func (o *SetActivityPropertyOp) alterWorkflowOp() {}

// InsertAfterOp inserts activities after a named activity (linear position).
// The old `insert after X <activity>` form carries exactly one; the generic
// `insert after X { … }` any number.
type InsertAfterOp struct {
	ActivityRef   string
	AtPosition    int
	NewActivities []WorkflowActivityNode
}

func (o *InsertAfterOp) alterWorkflowOp() {}

// InsertBeforeOp inserts activities before a named activity: the generic
// ALTER's `insert before X { … }`, which the old form had no spelling for.
type InsertBeforeOp struct {
	ActivityRef   string
	AtPosition    int
	NewActivities []WorkflowActivityNode
}

func (o *InsertBeforeOp) alterWorkflowOp() {}

// DropActivityOp removes a linear activity from the flow graph.
type DropActivityOp struct {
	ActivityRef string
	AtPosition  int
}

func (o *DropActivityOp) alterWorkflowOp() {}

// ReplaceActivityOp swaps an activity in place, preserving edges.
type ReplaceActivityOp struct {
	ActivityRef   string
	AtPosition    int
	NewActivities []WorkflowActivityNode
}

func (o *ReplaceActivityOp) alterWorkflowOp() {}

// InsertOutcomeOp adds a new outcome to a UserTask.
type InsertOutcomeOp struct {
	OutcomeName string
	ActivityRef string
	AtPosition  int
	Activities  []WorkflowActivityNode
}

func (o *InsertOutcomeOp) alterWorkflowOp() {}

// DropOutcomeOp removes an outcome from a UserTask.
type DropOutcomeOp struct {
	OutcomeName string
	ActivityRef string
	AtPosition  int
}

func (o *DropOutcomeOp) alterWorkflowOp() {}

// InsertPathOp adds a new path to a ParallelSplit. PathNumber is the `path n`
// the generic form may write, which must be the split's next path; 0 when it
// is left out (and always for the old `insert path on X`).
type InsertPathOp struct {
	ActivityRef string
	AtPosition  int
	PathNumber  int
	Activities  []WorkflowActivityNode
}

func (o *InsertPathOp) alterWorkflowOp() {}

// DropPathOp removes a path from a ParallelSplit.
type DropPathOp struct {
	PathCaption string
	ActivityRef string
	AtPosition  int
}

func (o *DropPathOp) alterWorkflowOp() {}

// InsertBranchOp adds a new branch to a Decision.
type InsertBranchOp struct {
	Condition   string
	ActivityRef string
	AtPosition  int
	Activities  []WorkflowActivityNode
}

func (o *InsertBranchOp) alterWorkflowOp() {}

// DropBranchOp removes a branch from a Decision.
type DropBranchOp struct {
	BranchName  string
	ActivityRef string
	AtPosition  int
}

func (o *DropBranchOp) alterWorkflowOp() {}

// InsertBoundaryEventOp adds a boundary event to an activity.
type InsertBoundaryEventOp struct {
	ActivityRef string
	AtPosition  int
	EventType   string // "InterruptingTimer", "NonInterruptingTimer"
	Delay       string
	Caption     string
	Recurrence  *WorkflowRecurrenceNode
	Activities  []WorkflowActivityNode
}

func (o *InsertBoundaryEventOp) alterWorkflowOp() {}

// DropBoundaryEventOp removes a boundary event from an activity.
type DropBoundaryEventOp struct {
	ActivityRef string
	AtPosition  int
	// Index is `boundary event N`: the N-th event, 1-based, in describe order;
	// 0 when the statement names none (ako/mxcli#808).
	Index int
}

func (o *DropBoundaryEventOp) alterWorkflowOp() {}
