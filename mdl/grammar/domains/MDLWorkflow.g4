/**
 * MDL Workflow Grammar — CREATE WORKFLOW, ALTER WORKFLOW.
 */
parser grammar MDLWorkflow;

options { tokenVocab = MDLLexer; }

// =============================================================================
// CREATE WORKFLOW
// =============================================================================

/**
 * Create a workflow with activities.
 */
createWorkflowStatement
    : WORKFLOW qualifiedName
      (FOLDER folder=STRING_LITERAL)?
      (PARAMETER VARIABLE COLON qualifiedName)?
      (DISPLAY display=STRING_LITERAL)?
      (DESCRIPTION description=STRING_LITERAL)?
      (EXPORT LEVEL (IDENTIFIER | API))?
      (OVERVIEW PAGE qualifiedName)?
      (DUE DATE_TYPE dueDate=STRING_LITERAL)?
      workflowEventHandlerClause*
      BEGIN workflowMainBody workflowEventSubProcess* END WORKFLOW SEMICOLON? SLASH?
    ;

/**
 * An event sub-process: a flow outside the main flow that its own start event
 * triggers while the workflow runs — a `notify workflow … target <start>`, or a
 * timer. Interrupting cancels every active path first; non-interrupting runs
 * alongside. Written after the main body, because Studio Pro stores them in the
 * workflow's EventSubProcesses list, not in its flow.
 *
 * The body's End is implicit, as in the main flow: the builder appends one when
 * the body does not already end (in an End, a jump, or branches that all end).
 */
workflowEventSubProcess
    : EVENT SUBPROCESS workflowActivityName STRING_LITERAL?
      ON (INTERRUPTING | NON INTERRUPTING) workflowEventSubProcessTrigger
      LBRACE workflowBody RBRACE SEMICOLON
    ;

/**
 * The start event. A notification start is what `notify workflow … target`
 * names; a timer start takes the first-execution-time expression, which Mendix
 * requires (CE0126).
 */
workflowEventSubProcessTrigger
    : NOTIFICATION workflowActivityName? STRING_LITERAL?
    | TIMER STRING_LITERAL (AS workflowActivityName)? (COMMENT STRING_LITERAL)?
    ;

/**
 * A workflow event handler: a microflow the runtime calls when one of the named
 * workflow events happens. Studio Pro stores the event types as an explicit list
 * even when every one is ticked, so `any workflow event` is written as the list
 * the project's Mendix version knows. The optional `as` string is the handler's
 * description, which is how Studio Pro tells handlers apart.
 */
workflowEventHandlerClause
    : ON ANY WORKFLOW EVENT MICROFLOW qualifiedName (AS STRING_LITERAL)?
    | ON WORKFLOW EVENTS LPAREN IDENTIFIER (COMMA IDENTIFIER)* RPAREN MICROFLOW qualifiedName (AS STRING_LITERAL)?
    ;

/**
 * The top-level body. It cannot hold `end workflow;` as a statement: there those
 * words close the body, and they ARE the main flow's End — Mendix refuses an End
 * anywhere else in the main flow (CE6671). Keeping the statement out of this rule
 * is what lets it exist at all; the first workflow grammar dropped it for the
 * conflict with the closer. See docs/11-proposals/PROPOSAL_workflow_end_activity.md.
 */
workflowMainBody
    : (workflowActivityStmt | workflowReturnStmt SEMICOLON)*
    ;

/**
 * A brace body: an outcome, a decision branch, a parallel path, a boundary-event
 * path, or an ALTER insert. `end workflow;` is accepted in every one and refused
 * by check rules where Mendix refuses it (MDL-WF08/09/10), because a platform
 * rule reported as a parse error reads as "not implemented".
 */
workflowBody
    : (workflowActivityStmt | workflowEndStmt SEMICOLON | workflowReturnStmt SEMICOLON)*
    ;

/** Ends the whole workflow from inside a branch; `comment` sets the End's caption. */
workflowEndStmt
    : END WORKFLOW (COMMENT STRING_LITERAL)?
    ;

/**
 * `return` belongs to microflows. It is parsed only so MDL-WF11 can say that a
 * workflow ends with `end workflow;` — it is the spelling a microflow author, or
 * an LLM, reaches for — and exec refuses it rather than dropping it.
 */
workflowReturnStmt
    : RETURN
    ;

workflowActivityStmt
    : workflowUserTaskStmt SEMICOLON
    | workflowCallMicroflowStmt SEMICOLON
    | workflowCallWorkflowStmt SEMICOLON
    | workflowDecisionStmt SEMICOLON
    | workflowParallelSplitStmt SEMICOLON
    | workflowJumpToStmt SEMICOLON
    | workflowWaitForTimerStmt SEMICOLON
    | workflowWaitForNotificationStmt SEMICOLON
    | workflowNotificationStmt SEMICOLON
    | workflowAnnotationStmt SEMICOLON
    ;

/**
 * An activity's explicit name. Mendix resolves `jump to` by
 * JumpToActivity.TargetActivity, which stores an activity NAME, and Studio Pro
 * names every activity by type and ordinal (decision1, split1, callMicroflow1)
 * independently of its caption. Without a name slot a described workflow's jump
 * wiring could not be re-executed. See ako/mxcli#408.
 */
workflowActivityName
    : IDENTIFIER
    | QUOTED_IDENTIFIER
    ;

workflowUserTaskStmt
    : USER TASK (IDENTIFIER | QUOTED_IDENTIFIER) STRING_LITERAL
      (PAGE qualifiedName)?
      (TARGETING (USERS | GROUPS)? MICROFLOW qualifiedName)?
      (TARGETING (USERS | GROUPS)? XPATH STRING_LITERAL)?
      (ON CREATED MICROFLOW qualifiedName)?
      (ENTITY qualifiedName)?
      (DUE DATE_TYPE STRING_LITERAL)?
      (DESCRIPTION STRING_LITERAL)?
      (OUTCOMES workflowUserTaskOutcome+)?
      (BOUNDARY EVENT workflowBoundaryEventClause ((BOUNDARY EVENT)? workflowBoundaryEventClause)*)?
    | MULTI USER TASK (IDENTIFIER | QUOTED_IDENTIFIER) STRING_LITERAL
      (PAGE qualifiedName)?
      (TARGETING (USERS | GROUPS)? MICROFLOW qualifiedName)?
      (TARGETING (USERS | GROUPS)? XPATH STRING_LITERAL)?
      (ON CREATED MICROFLOW qualifiedName)?
      (ENTITY qualifiedName)?
      (DUE DATE_TYPE STRING_LITERAL)?
      (DESCRIPTION STRING_LITERAL)?
      workflowParticipantsClause?
      workflowCompletionClause?
      (AWAIT ALL USERS)?
      (OUTCOMES workflowUserTaskOutcome+)?
      (BOUNDARY EVENT workflowBoundaryEventClause ((BOUNDARY EVENT)? workflowBoundaryEventClause)*)?
    ;

/**
 * How many of a multi-user task's targeted users must respond (TargetUserInput).
 * Omitted means all of them. A sub-rule, so its number stays out of the task's
 * positional reads.
 */
workflowParticipantsClause
    : PARTICIPANTS ALL
    | PARTICIPANTS NUMBER_LITERAL PERCENT_KW?
    ;

/**
 * How a multi-user task turns its participants' outcomes into one outcome
 * (CompletionCriteria). Omitted means consensus falling back to the first
 * outcome. The fallback is optional here and required by check (CE1866): a
 * platform rule reported as a parse error reads as "not implemented".
 */
workflowCompletionClause
    : DECIDE BY CONSENSUS workflowFallbackClause?
    | DECIDE BY MAJORITY MORE_KW THAN HALF workflowFallbackClause?
    | DECIDE BY MAJORITY MOST CHOSEN workflowFallbackClause?
    | DECIDE BY THRESHOLD NUMBER_LITERAL (PERCENT_KW | VOTES) workflowFallbackClause?
    | DECIDE BY VETO STRING_LITERAL
    | DECIDE BY MICROFLOW qualifiedName
    ;

workflowFallbackClause
    : FALLBACK STRING_LITERAL
    ;

/**
 * One boundary event. An activity's clauses may each repeat `boundary event`
 * (the form describe emits and the syntax topic documents) or share one
 * (`boundary event interrupting timer '…' non interrupting timer '…'`). The
 * grammar accepted only the shared form, so the describe output of an activity
 * with two boundary events did not parse.
 */
workflowBoundaryEventClause
    : INTERRUPTING TIMER STRING_LITERAL? (LBRACE workflowBody RBRACE)?
    | NON INTERRUPTING TIMER STRING_LITERAL? (LBRACE workflowBody RBRACE)?
    | TIMER STRING_LITERAL? (LBRACE workflowBody RBRACE)?
    // A notification boundary event is triggered by `notify workflow … target
    // <name>`, so its name is what matters; the string is its caption.
    | INTERRUPTING NOTIFICATION workflowActivityName? STRING_LITERAL? (LBRACE workflowBody RBRACE)?
    | NON INTERRUPTING NOTIFICATION workflowActivityName? STRING_LITERAL? (LBRACE workflowBody RBRACE)?
    ;

workflowUserTaskOutcome
    : STRING_LITERAL LBRACE workflowBody RBRACE
    ;

/**
 * `call agent microflow` is the AI agent task (Mendix 11.9+): stored as
 * Workflows$AIAgentTaskActivity, the same shape as a call-microflow activity, and
 * run by the workflow engine as an agent step. The microflow is where the agent is
 * invoked.
 */
workflowCallMicroflowStmt
    : CALL AGENT? MICROFLOW qualifiedName (AS workflowActivityName)? (COMMENT STRING_LITERAL)?
      (WITH LPAREN workflowParameterMapping (COMMA workflowParameterMapping)* RPAREN)?
      (OUTCOMES workflowConditionOutcome+)?
      (BOUNDARY EVENT workflowBoundaryEventClause ((BOUNDARY EVENT)? workflowBoundaryEventClause)*)?
    ;

workflowParameterMapping
    : qualifiedName EQUALS STRING_LITERAL
    ;

workflowCallWorkflowStmt
    : CALL WORKFLOW qualifiedName (AS workflowActivityName)? (COMMENT STRING_LITERAL)?
      (WITH LPAREN workflowParameterMapping (COMMA workflowParameterMapping)* RPAREN)?
    ;

workflowDecisionStmt
    : DECISION workflowActivityName? STRING_LITERAL? (COMMENT STRING_LITERAL)?
      (OUTCOMES workflowConditionOutcome+)?
    ;

workflowConditionOutcome
    : (TRUE | FALSE | STRING_LITERAL | DEFAULT) ARROW LBRACE workflowBody RBRACE
    ;

workflowParallelSplitStmt
    : PARALLEL SPLIT workflowActivityName? (COMMENT STRING_LITERAL)?
      workflowParallelPath+
    ;

workflowParallelPath
    : PATH NUMBER_LITERAL LBRACE workflowBody RBRACE
    ;

workflowJumpToStmt
    : JUMP TO (IDENTIFIER | QUOTED_IDENTIFIER) (COMMENT STRING_LITERAL)?
    ;

workflowWaitForTimerStmt
    : WAIT FOR TIMER workflowActivityName? STRING_LITERAL? (COMMENT STRING_LITERAL)?
    ;

workflowWaitForNotificationStmt
    : WAIT FOR NOTIFICATION workflowActivityName? (COMMENT STRING_LITERAL)?
      (BOUNDARY EVENT workflowBoundaryEventClause ((BOUNDARY EVENT)? workflowBoundaryEventClause)*)?
    ;

/**
 * An intermediate notification event on a flow (Workflows$NotificationActivity):
 * the point a `notify workflow … target <name>` reaches.
 */
workflowNotificationStmt
    : NOTIFICATION workflowActivityName? (COMMENT STRING_LITERAL)?
    ;

workflowAnnotationStmt
    : ANNOTATION STRING_LITERAL
    ;

// =============================================================================
// ALTER WORKFLOW
// =============================================================================

alterWorkflowAction
    : SET workflowSetProperty
    | SET ACTIVITY alterActivityRef activitySetProperty
    | INSERT AFTER alterActivityRef workflowActivityStmt
    | DROP ACTIVITY alterActivityRef
    | REPLACE ACTIVITY alterActivityRef WITH workflowActivityStmt
    | INSERT OUTCOME STRING_LITERAL ON alterActivityRef LBRACE workflowBody RBRACE
    | INSERT PATH ON alterActivityRef LBRACE workflowBody RBRACE
    | DROP OUTCOME STRING_LITERAL ON alterActivityRef
    | DROP PATH STRING_LITERAL ON alterActivityRef
    | INSERT BOUNDARY EVENT ON alterActivityRef workflowBoundaryEventClause
    | DROP BOUNDARY EVENT ON alterActivityRef
    | INSERT CONDITION STRING_LITERAL ON alterActivityRef LBRACE workflowBody RBRACE
    | DROP CONDITION STRING_LITERAL ON alterActivityRef
    ;

workflowSetProperty
    : DISPLAY STRING_LITERAL
    | DESCRIPTION STRING_LITERAL
    | EXPORT LEVEL (IDENTIFIER | API)
    | DUE DATE_TYPE STRING_LITERAL
    | OVERVIEW PAGE qualifiedName
    | PARAMETER VARIABLE COLON qualifiedName
    ;

activitySetProperty
    : PAGE qualifiedName
    | DESCRIPTION STRING_LITERAL
    | TARGETING MICROFLOW qualifiedName
    | TARGETING XPATH STRING_LITERAL
    | DUE DATE_TYPE STRING_LITERAL
    ;

alterActivityRef
    : identifierOrKeyword (AT NUMBER_LITERAL)?
    | STRING_LITERAL (AT NUMBER_LITERAL)?
    ;
