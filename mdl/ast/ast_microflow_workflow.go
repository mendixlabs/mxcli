// SPDX-License-Identifier: Apache-2.0

package ast

// CallWorkflowStmt represents: [$Wf =] CALL WORKFLOW Module.WF_Name ($ContextObj)
type CallWorkflowStmt struct {
	OutputVariable string
	Workflow       QualifiedName
	Arguments      []CallArgument
	// ContextVariable carries the positional form `call workflow X ($Ctx)`. A
	// workflow has exactly one context parameter and the model stores only the
	// variable, so DESCRIBE can only emit that form — see the grammar note.
	ContextVariable string
	ErrorHandling   *ErrorHandlingClause
	Annotations     *ActivityAnnotations
}

func (*CallWorkflowStmt) isMicroflowStatement() {}

// GetWorkflowDataStmt represents: [$Data =] GET WORKFLOW DATA $WorkflowVar AS Module.WorkflowName
type GetWorkflowDataStmt struct {
	OutputVariable   string
	WorkflowVariable string
	Workflow         QualifiedName
	ErrorHandling    *ErrorHandlingClause
	Annotations      *ActivityAnnotations
}

func (*GetWorkflowDataStmt) isMicroflowStatement() {}

// GetWorkflowsStmt represents: [$Wfs =] GET WORKFLOWS FOR $ContextObj
type GetWorkflowsStmt struct {
	OutputVariable              string
	WorkflowContextVariableName string
	ErrorHandling               *ErrorHandlingClause
	Annotations                 *ActivityAnnotations
}

func (*GetWorkflowsStmt) isMicroflowStatement() {}

// GetWorkflowActivityRecordsStmt represents: [$Records =] GET WORKFLOW ACTIVITY RECORDS $WorkflowVar
type GetWorkflowActivityRecordsStmt struct {
	OutputVariable   string
	WorkflowVariable string
	ErrorHandling    *ErrorHandlingClause
	Annotations      *ActivityAnnotations
}

func (*GetWorkflowActivityRecordsStmt) isMicroflowStatement() {}

// WorkflowOperationStmt represents: WORKFLOW OPERATION <type> $WorkflowVar [REASON '...']
type WorkflowOperationStmt struct {
	OperationType    string // ABORT, CONTINUE, PAUSE, RESTART, RETRY, UNPAUSE
	WorkflowVariable string
	Reason           Expression // Only for ABORT
	ErrorHandling    *ErrorHandlingClause
	Annotations      *ActivityAnnotations
}

func (*WorkflowOperationStmt) isMicroflowStatement() {}

// SetTaskOutcomeStmt represents: SET TASK OUTCOME $UserTask 'OutcomeName'
type SetTaskOutcomeStmt struct {
	WorkflowTaskVariable string
	OutcomeValue         string
	ErrorHandling        *ErrorHandlingClause
	Annotations          *ActivityAnnotations
}

func (*SetTaskOutcomeStmt) isMicroflowStatement() {}

// OpenUserTaskStmt represents: OPEN USER TASK $UserTask
type OpenUserTaskStmt struct {
	UserTaskVariable string
	ErrorHandling    *ErrorHandlingClause
	Annotations      *ActivityAnnotations
}

func (*OpenUserTaskStmt) isMicroflowStatement() {}

// NotifyWorkflowStmt represents: [$Result =] NOTIFY WORKFLOW $WorkflowVar [TARGET Module.Workflow.Name]
type NotifyWorkflowStmt struct {
	OutputVariable   string
	WorkflowVariable string
	Target           string // Module.Workflow.ElementName; "" when the clause is absent
	ErrorHandling    *ErrorHandlingClause
	Annotations      *ActivityAnnotations
}

func (*NotifyWorkflowStmt) isMicroflowStatement() {}

// OpenWorkflowStmt represents: OPEN WORKFLOW $WorkflowVar
type OpenWorkflowStmt struct {
	WorkflowVariable string
	ErrorHandling    *ErrorHandlingClause
	Annotations      *ActivityAnnotations
}

func (*OpenWorkflowStmt) isMicroflowStatement() {}

// LockWorkflowStmt represents: LOCK WORKFLOW ($WorkflowVar | ALL)
type LockWorkflowStmt struct {
	WorkflowVariable  string
	PauseAllWorkflows bool
	ErrorHandling     *ErrorHandlingClause
	Annotations       *ActivityAnnotations
}

func (*LockWorkflowStmt) isMicroflowStatement() {}

// UnlockWorkflowStmt represents: UNLOCK WORKFLOW ($WorkflowVar | ALL)
type UnlockWorkflowStmt struct {
	WorkflowVariable         string
	ResumeAllPausedWorkflows bool
	ErrorHandling            *ErrorHandlingClause
	Annotations              *ActivityAnnotations
}

func (*UnlockWorkflowStmt) isMicroflowStatement() {}
