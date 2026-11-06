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
    : WORKFLOW ifNotExists? qualifiedName
      workflowHeaderClause*
      BEGIN workflowMainBody workflowEventSubProcess* END WORKFLOW SEMICOLON? SLASH?
    ;

/**
 * One header clause. The clauses used to be a fixed SEQUENCE of optional
 * groups, so each was optional but its POSITION was not: `display` after
 * `description` failed with `mismatched input 'DISPLAY' expecting {ON, BEGIN,
 * EXPORT, DUE, OVERVIEW}`, which names neither the clause nor the rule, and the
 * author had to reverse-engineer the order from the failure. They are now a
 * set — any order, each at most once, which is how the rest of MDL reads
 * (ADR-0003). The at-most-once half is NOT in the grammar: a repeated clause is
 * reported by `checkWorkflowClausesAtMostOnce` in the visitor, which can name
 * the clause instead of pointing at a token. See ako/mxcli#586.
 */
workflowHeaderClause
    : FOLDER folder=STRING_LITERAL
    | PARAMETER VARIABLE COLON qualifiedName
    // `with ({1} = <expression>, …)` fills the placeholders, as a log message's
    // template does (ako/mxcli#1714).
    | DISPLAY display=STRING_LITERAL templateParams?
    | DESCRIPTION description=STRING_LITERAL templateParams?
    // HIDDEN_KW is listed beside IDENTIFIER because `Hidden` used to lex as an
    // identifier and stopped when the microflow clauses made it a keyword.
    // Anything matching a bare IDENTIFIER here is one token away from the same
    // break — the hazard `identifierOrKeyword` exists to absorb, which this
    // rule bypasses by taking IDENTIFIER directly.
    | EXPORT LEVEL (IDENTIFIER | API | HIDDEN_KW)
    | OVERVIEW PAGE qualifiedName
    | DUE DATE_TYPE dueDate=workflowExpression
    // The note attached to the workflow's start (ako/mxcli#707); an activity's
    // is `@annotation '…'` before it.
    | ANNOTATION annotationText=STRING_LITERAL
    | workflowEventHandlerClause
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
    : annotation* EVENT SUBPROCESS workflowActivityName STRING_LITERAL?
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
    | TIMER workflowExpression workflowRecurrence? (AS workflowActivityName)? workflowCaption?
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

/**
 * An activity's caption, the text Studio Pro shows on it (R9). `comment` was
 * the first spelling: it set the caption, not a comment, which misled the
 * reader into taking it for an annotation, so it is a registered alias.
 */
workflowCaption
    : CAPTION STRING_LITERAL
    | COMMENT /* @alias MDL-DEPR104 */ STRING_LITERAL
    ;

/** Ends the whole workflow from inside a branch; `caption` sets the End's caption. */
workflowEndStmt
    : END WORKFLOW workflowCaption?
    ;

/**
 * `return` belongs to microflows. It is parsed only so MDL-WF11 can say that a
 * workflow ends with `end workflow;` — it is the spelling a microflow author, or
 * an LLM, reaches for — and exec refuses it rather than dropping it.
 */
workflowReturnStmt
    : RETURN
    ;

// `@annotation '…'` before an activity is the note Studio Pro attaches to it,
// as it is before a microflow activity (ako/mxcli#707). The standalone
// `annotation '…';` is a different thing, a sticky note, and MDL-WF04 refuses it.
workflowActivityStmt
    : annotation* workflowUserTaskStmt SEMICOLON
    | annotation* workflowCallMicroflowStmt SEMICOLON
    | annotation* workflowCallWorkflowStmt SEMICOLON
    | annotation* workflowDecisionStmt SEMICOLON
    | annotation* workflowParallelSplitStmt SEMICOLON
    | annotation* workflowJumpToStmt SEMICOLON
    | annotation* workflowWaitForTimerStmt SEMICOLON
    | annotation* workflowWaitForNotificationStmt SEMICOLON
    | annotation* workflowNotificationStmt SEMICOLON
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

/**
 * A Mendix expression a workflow stores: a decision's condition, a timer's
 * delay or first execution time, a due date. R5 (ako/mxcli#753): written bare,
 * like every other expression in MDL — `decision $WorkflowContext/Total > 1000`.
 *
 * The string form (`decision '$WorkflowContext/Total > 1000'`) is the
 * deprecated spelling of the same thing: its CONTENT is the expression, as it
 * always was. It is listed first so a lone string keeps that reading; no slot
 * here takes a string-valued expression (a condition is Boolean or an
 * enumeration, a timer or due date a DateTime), so nothing written bare means
 * a string either.
 */
workflowExpression
    : STRING_LITERAL /* @alias MDL-DEPR080 */
    | expression
    ;

/**
 * A timer boundary event's delay. The delay is optional and the next boundary
 * event may follow without repeating `boundary event`, so a bare delay must not
 * start with the `non` of `non interrupting timer`, a word the expression
 * grammar admits as a name: `interrupting timer non interrupting timer 'x'` is
 * two events, the first without a delay, as it was when only a string could be
 * the delay (ako/mxcli#753). Nor may it start with the `caption` (or its alias
 * `comment`) of a timer with no delay: `interrupting timer caption 'Escalate'`,
 * or with its `repeat`.
 */
workflowTimerDelay
    : {p.GetTokenStream().LA(1) != MDLParserNON && p.GetTokenStream().LA(1) != MDLParserCAPTION && p.GetTokenStream().LA(1) != MDLParserCOMMENT && p.GetTokenStream().LA(1) != MDLParserREPEAT}? workflowExpression
    ;

/**
 * A non-interrupting timer's recurrence, Workflows$LinearRecurrence
 * (ako/mxcli#1716): after its first firing the timer fires every <interval>
 * <unit>, <max> times in all — the first firing counts, as Studio Pro's
 * MaxExecutions does. The unit is a name (minute(s) … month(s)), not a
 * keyword, and the visitor and MDL-WF22 check it.
 */
workflowRecurrence
    : REPEAT EVERY interval=NUMBER_LITERAL unit=identifierOrKeyword AT_KW MOST maxExecutions=NUMBER_LITERAL TIMES
    ;

/**
 * A user task. Its clauses are a SET, not a sequence — see
 * `workflowHeaderClause` for why, and `checkWorkflowClausesAtMostOnce` for the
 * half of the old rule the grammar no longer carries.
 *
 * The two alternatives keep their own clause rules rather than collapsing into
 * `MULTI?`, so a single-user task still refuses `participants`, `decide by` and
 * `await all users` — relaxing the ORDER must not also relax the vocabulary.
 */
workflowUserTaskStmt
    : USER TASK (IDENTIFIER | QUOTED_IDENTIFIER) STRING_LITERAL
      workflowUserTaskClause*
    | MULTI USER TASK (IDENTIFIER | QUOTED_IDENTIFIER) STRING_LITERAL
      workflowMultiUserTaskClause*
    ;

/** A clause every user task accepts. */
workflowUserTaskClause
    : PAGE qualifiedName
    | TARGETING (USERS | GROUPS)? MICROFLOW qualifiedName
    | TARGETING (USERS | GROUPS)? XPATH xpathConstraint+
    | TARGETING (USERS | GROUPS)? XPATH STRING_LITERAL /* @alias MDL-DEPR031 */
    | ON CREATED MICROFLOW qualifiedName
    | ENTITY qualifiedName
    | DUE DATE_TYPE workflowExpression
    | DESCRIPTION STRING_LITERAL templateParams?
    // `task name '…'`: what the task inbox shows, when it is not the caption.
    // `name` is not a keyword — it is an attribute name everywhere — so the
    // word is read as an identifier and checked by the visitor (ako/mxcli#1714).
    | TASK taskNameWord=IDENTIFIER STRING_LITERAL templateParams?
    | OUTCOMES workflowUserTaskOutcome+
    | BOUNDARY EVENT workflowBoundaryEventClause ((BOUNDARY EVENT)? workflowBoundaryEventClause)*
    | AUTO ASSIGN SINGLE USER   // "Auto-assign when targeting results in one user"
    ;

/** The above, plus the three clauses only a multi user task has. */
workflowMultiUserTaskClause
    : workflowUserTaskClause
    | workflowParticipantsClause
    | workflowCompletionClause
    | AWAIT ALL USERS
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
    : INTERRUPTING TIMER workflowTimerDelay? workflowRecurrence? workflowCaption? (LBRACE workflowBody RBRACE)?
    | NON INTERRUPTING TIMER workflowTimerDelay? workflowRecurrence? workflowCaption? (LBRACE workflowBody RBRACE)?
    | TIMER workflowTimerDelay? workflowRecurrence? workflowCaption? (LBRACE workflowBody RBRACE)?
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
    : CALL AGENT? MICROFLOW qualifiedName workflowCallArguments? (AS workflowActivityName)? workflowCaption?
      (WITH /* @alias MDL-DEPR008 */ LPAREN workflowParameterMapping (COMMA workflowParameterMapping)* RPAREN)?
      (OUTCOMES workflowConditionOutcome+)?
      (BOUNDARY EVENT workflowBoundaryEventClause ((BOUNDARY EVENT)? workflowBoundaryEventClause)*)?
    ;

// R4: a workflow call binds its arguments like every other call site,
// `(Param = expression)` right after the callee, the expression bare.
// `with (Param = '<expression>')`, the expression in a string, is the
// deprecated spelling of the same mapping.
workflowCallArguments
    : LPAREN (workflowCallArgument (COMMA workflowCallArgument)*)? RPAREN
    ;

workflowCallArgument
    : parameterName EQUALS expression
    ;

workflowParameterMapping
    : qualifiedName EQUALS STRING_LITERAL
    ;

// A call-workflow activity owns boundary events like a call-microflow one
// (Workflows$CallWorkflowActivity.boundaryEvents, 10.14+). The reader, writer,
// describer and ALTER all handled them; only this rule did not, so describe
// output of such a workflow did not re-parse (mendixlabs/mxcli#1532).
workflowCallWorkflowStmt
    : CALL WORKFLOW qualifiedName workflowCallArguments? (AS workflowActivityName)? workflowCaption?
      (WITH /* @alias MDL-DEPR008 */ LPAREN workflowParameterMapping (COMMA workflowParameterMapping)* RPAREN)?
      (BOUNDARY EVENT workflowBoundaryEventClause ((BOUNDARY EVENT)? workflowBoundaryEventClause)*)?
    ;

workflowDecisionStmt
    : DECISION workflowActivityName? workflowExpression? workflowCaption?
      (OUTCOMES workflowConditionOutcome+)?
    ;

workflowConditionOutcome
    : (TRUE | FALSE | STRING_LITERAL | DEFAULT) ARROW LBRACE workflowBody RBRACE
    ;

workflowParallelSplitStmt
    : PARALLEL SPLIT workflowActivityName? workflowCaption?
      workflowParallelPath+
    ;

workflowParallelPath
    : PATH NUMBER_LITERAL LBRACE workflowBody RBRACE
    ;

workflowJumpToStmt
    : JUMP TO (IDENTIFIER | QUOTED_IDENTIFIER) workflowCaption?
    ;

workflowWaitForTimerStmt
    : WAIT FOR TIMER workflowActivityName? workflowExpression? workflowCaption?
    ;

workflowWaitForNotificationStmt
    : WAIT FOR NOTIFICATION workflowActivityName? workflowCaption?
      (BOUNDARY EVENT workflowBoundaryEventClause ((BOUNDARY EVENT)? workflowBoundaryEventClause)*)?
    ;

/**
 * An intermediate notification event on a flow (Workflows$NotificationActivity):
 * the point a `notify workflow … target <name>` reaches.
 */
workflowNotificationStmt
    : NOTIFICATION workflowActivityName? workflowCaption?
    ;

workflowAnnotationStmt
    : ANNOTATION STRING_LITERAL
    ;

// =============================================================================
// ALTER WORKFLOW
// =============================================================================

/**
 * `alter workflow` operations: the generic ALTER (ADR-0012 decision 2,
 * ako/mxcli#712).
 *
 * ```mdl
 * alter workflow Shop.OrderApproval {
 *   set (Display: 'Order approval', DueDate: addDays([%CurrentDateTime%], 3));
 *   set (Page: Shop.TaskPage, Targeting: xpath [Active = true()]) on ReviewOrder;
 *   insert after ReviewOrder { call microflow Shop.ACT_Notify; }
 *   insert before 'Review the order'@2 { wait for notification Ready; }
 *   insert into ReviewOrder { outcomes 'Escalate' { call microflow Shop.ACT_Escalate; } }
 *   insert into decision1 { outcomes default -> { } }
 *   insert into split1 { path { call microflow Shop.ACT_Log; } }
 *   insert into ReviewOrder { boundary event interrupting timer addHours([%CurrentDateTime%], 4) { } }
 *   replace ReviewOrder with { user task ReviewOrder caption 'Review' outcomes 'Done' { }; }
 *   drop ReviewOrder outcome 'Reject', split1 path 2, ReviewOrder boundary event;
 *   drop callMicroflow1;
 * }
 * ```
 *
 * A target is an activity's name or its quoted caption, with `@n` to choose
 * one of several matches; the workflow's resolver (wfmutator) refuses an
 * ambiguous one and lists the matches. A fragment is written exactly as
 * `create workflow` writes it: activities in `{ }`, and for `insert into` the
 * activity's own `outcomes`, `path` or `boundary event` clause.
 */
alterWorkflowOperation
    : SET LPAREN alterWorkflowAssignment (COMMA alterWorkflowAssignment)* RPAREN (ON alterTarget)? SEMICOLON?
    | INSERT (AFTER | BEFORE) alterTarget alterWorkflowFragment SEMICOLON?
    | INSERT INTO alterTarget LBRACE alterWorkflowMember+ RBRACE SEMICOLON?
    | REPLACE alterTarget WITH alterWorkflowFragment SEMICOLON?
    | DROP alterWorkflowTarget (COMMA alterWorkflowTarget)* SEMICOLON?
    ;

// Activities, as the body of an outcome in `create workflow` writes them.
alterWorkflowFragment
    : LBRACE workflowBody RBRACE
    ;

// What `insert into <activity>` adds: the activity's own clause, as `create
// workflow` writes it. A path's number may be left out; when written it must
// be the next one.
alterWorkflowMember
    : OUTCOMES (workflowUserTaskOutcome | workflowConditionOutcome)+
    | PATH NUMBER_LITERAL? LBRACE workflowBody RBRACE
    | BOUNDARY EVENT workflowBoundaryEventClause
    ;

// A drop target: an activity, or one member of it — a user task's or a
// decision's outcome, a parallel split's path, the activity's boundary event.
alterWorkflowTarget
    : alterTarget alterWorkflowTargetMember?
    ;

// `boundary event N` is the N-th boundary event in the order `describe workflow`
// prints them, like a parallel split's `path N` (ako/mxcli#808); without N it
// is the activity's only one.
alterWorkflowTargetMember
    : OUTCOME (STRING_LITERAL | TRUE | FALSE | DEFAULT)
    | PATH NUMBER_LITERAL
    | BOUNDARY EVENT NUMBER_LITERAL?
    ;

// `Key: value`, the key naming a workflow property (Display, Description,
// ExportLevel, DueDate, OverviewPage, Parameter, Annotation) or, with `on
// <activity>`, an activity property (Page, Description, TaskName, Targeting,
// DueDate, Annotation). Which keys exist
// is the visitor's call, so an unknown key is refused with the list.
alterWorkflowAssignment
    : identifierOrKeyword COLON alterWorkflowValue
    ;

alterWorkflowValue
    : VARIABLE COLON qualifiedName                  // Parameter: $WorkflowContext: M.Ctx
    | MICROFLOW qualifiedName                       // Targeting: microflow M.Target
    | XPATH xpathConstraint+                        // Targeting: xpath [Active = true()]
    | XPATH STRING_LITERAL /* @alias MDL-DEPR031 */ // Targeting: xpath '[Active = true()]'
    | qualifiedName                                 // Page: M.P, ExportLevel: API
    | STRING_LITERAL templateParams                 // Display: 'x {1}' with ({1} = …)
    | workflowExpression                            // Display: 'x', DueDate: addDays(…)
    ;

// The old per-action forms, each a registered alias of the generic operation
// it spells (MDL-DEPR140-149); `fmt --upgrade` rewrites them.
alterWorkflowAction
    : SET /* @alias MDL-DEPR140 */ workflowSetProperty
    | SET ACTIVITY /* @alias MDL-DEPR141 */ alterActivityRef activitySetProperty
    | INSERT AFTER /* @alias MDL-DEPR142 */ alterActivityRef workflowActivityStmt
    | DROP ACTIVITY /* @alias MDL-DEPR143 */ alterActivityRef
    | REPLACE ACTIVITY /* @alias MDL-DEPR144 */ alterActivityRef WITH workflowActivityStmt
    | INSERT OUTCOME /* @alias MDL-DEPR145 */ STRING_LITERAL ON alterActivityRef LBRACE workflowBody RBRACE
    | INSERT PATH /* @alias MDL-DEPR146 */ ON alterActivityRef LBRACE workflowBody RBRACE
    | DROP OUTCOME /* @alias MDL-DEPR149 */ STRING_LITERAL ON alterActivityRef
    | DROP PATH /* @alias MDL-DEPR149 */ STRING_LITERAL ON alterActivityRef
    | INSERT BOUNDARY EVENT /* @alias MDL-DEPR148 */ ON alterActivityRef workflowBoundaryEventClause
    | DROP BOUNDARY EVENT /* @alias MDL-DEPR149 */ ON alterActivityRef
    | INSERT CONDITION /* @alias MDL-DEPR147 */ STRING_LITERAL ON alterActivityRef LBRACE workflowBody RBRACE
    | DROP CONDITION /* @alias MDL-DEPR149 */ STRING_LITERAL ON alterActivityRef
    ;

workflowSetProperty
    : DISPLAY STRING_LITERAL
    | DESCRIPTION STRING_LITERAL
    | EXPORT LEVEL (IDENTIFIER | API | HIDDEN_KW)
    | DUE DATE_TYPE workflowExpression
    | OVERVIEW PAGE qualifiedName
    | PARAMETER VARIABLE COLON qualifiedName
    ;

activitySetProperty
    : PAGE qualifiedName
    | DESCRIPTION STRING_LITERAL
    | TARGETING MICROFLOW qualifiedName
    | TARGETING XPATH xpathConstraint+
    | TARGETING XPATH STRING_LITERAL /* @alias MDL-DEPR031 */
    | DUE DATE_TYPE workflowExpression
    ;

alterActivityRef
    : identifierOrKeyword (AT NUMBER_LITERAL)?
    | STRING_LITERAL (AT NUMBER_LITERAL)?
    ;
