// SPDX-License-Identifier: Apache-2.0

package syntax

func init() {
	Register(SyntaxFeature{
		Path:     "workflow",
		Features: []string{"workflows/basic"},
		Summary:  "Multi-step business processes with user tasks, decisions, and parallel paths",
		Keywords: []string{
			"workflow", "business process", "approval", "review",
			"user task", "decision", "parallel",
		},
		Syntax:  "CREATE WORKFLOW Module.Name\n  PARAMETER $Context: Module.Entity\nBEGIN\n  <activities>\nEND WORKFLOW;",
		Example: "CREATE WORKFLOW HR.LeaveApproval\n  PARAMETER $Context: HR.LeaveRequest\nBEGIN\n  USER TASK Review 'Review request'\n    PAGE HR.ReviewPage\n    OUTCOMES 'Approve' { } 'Reject' { };\nEND WORKFLOW;",
		SeeAlso: []string{"workflow.user-task", "workflow.decision", "workflow.parallel-split"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.show",
		Summary: "List and describe existing workflows",
		Keywords: []string{
			"list workflows", "list workflows", "describe workflow",
		},
		Syntax:  "LIST WORKFLOWS;\nLIST WORKFLOWS IN <module>;\nDESCRIBE WORKFLOW Module.Name;",
		Example: "LIST WORKFLOWS IN HR;\nDESCRIBE WORKFLOW HR.LeaveApproval;",
	})

	Register(SyntaxFeature{
		Path:     "workflow.create",
		Features: []string{"workflows/annotations", "workflows/overview_page"},
		Summary:  "Create a new workflow definition with activities and flow",
		Keywords: []string{
			"create workflow", "new workflow", "define workflow",
			"parameter", "overview page", "due date",
		},
		Syntax: "CREATE [OR MODIFY] WORKFLOW Module.Name\n" +
			"  [FOLDER 'path']\n" +
			"  PARAMETER $Context: Module.Entity\n" +
			"  [DISPLAY '<name>' [WITH ({1} = <expression>, ...)]]\n" +
			"  [DESCRIPTION '<text>' [WITH ({1} = <expression>, ...)]]\n" +
			"  [EXPORT LEVEL Hidden | API]\n" +
			"  [OVERVIEW PAGE Module.OverviewPage]\n" +
			"  [DUE DATE <expression>]\n" +
			"  [ON WORKFLOW EVENTS (<type>, ...) MICROFLOW Module.Handler [AS '<description>']]...\n" +
			"  [ON ANY WORKFLOW EVENT MICROFLOW Module.Handler [AS '<description>']]...\n" +
			"BEGIN\n  <activities>\nEND WORKFLOW;\n\n" +
			"-- The header clauses are a SET: write them in ANY order, each at most\n" +
			"-- once. (Before ako/mxcli#586 the order above was mandatory and writing\n" +
			"-- one out of place was a token error naming neither the clause nor the\n" +
			"-- rule.) The event handlers are the exception and may repeat.\n" +
			"-- A clause written twice is reported by name, e.g.\n" +
			"--   duplicate DISPLAY clause on workflow M.W (already given on line 3)\n\n" +
			"-- DISPLAY and DESCRIPTION are templates: WITH fills their {1}, {2}, ...\n" +
			"-- from expressions over $WorkflowContext only ($WorkflowInstance is CE0117\n" +
			"-- here, MDL-WF19). A placeholder no parameter fills is CE0720 (MDL-WF21).\n\n" +
			"-- The OVERVIEW PAGE must accept a System.Workflow parameter, or the\n" +
			"-- build fails CE7410 \"The selected page should accept a parameter of\n" +
			"-- type 'Workflow'\" (measured on mxbuild 11.6.6).",
		Example: "CREATE WORKFLOW Module.ApprovalFlow\n  PARAMETER $Context: Module.Request\n  DISPLAY 'Approval for {1}' WITH ({1} = $Context/Title)\n  OVERVIEW PAGE Module.WF_Overview\nBEGIN\n  USER TASK ReviewTask 'Review the request'\n    PAGE Module.ReviewPage\n    OUTCOMES 'Approve' { } 'Reject' { };\nEND WORKFLOW;",
		SeeAlso: []string{"workflow.user-task", "workflow.event-handlers", "workflow.decision", "workflow.drop"},
	})

	Register(SyntaxFeature{
		Path:     "workflow.event-handlers",
		Features: []string{"workflows/event_handlers"},
		Summary:  "Run a microflow when workflow events happen (task started, workflow completed, ...)",
		Keywords: []string{
			"event handler", "workflow event", "on workflow events", "any workflow event",
			"audit", "on created", "on-created microflow", "WorkflowEvent", "OnWorkflowEvent",
		},
		// Every rule below is an mxbuild 11.13.0 measurement. The invented-type row
		// is the one worth saying out loud: the build does not check type names.
		Syntax: "-- Workflow header, before BEGIN; repeat for more handlers:\n" +
			"ON WORKFLOW EVENTS (<type>, ...) MICROFLOW Module.Handler [AS '<description>']\n" +
			"ON ANY WORKFLOW EVENT MICROFLOW Module.Handler [AS '<description>']\n\n" +
			"-- The handler microflow takes exactly these three, in any order (else CE6691):\n" +
			"--   System.WorkflowEvent, System.WorkflowRecord, System.WorkflowActivityRecord\n\n" +
			"-- Event types (names are checked by mxcli — mxbuild accepts any name,\n" +
			"-- and a misspelt type never fires; MDL-WF12):\n" +
			"--   WorkflowCompleted WorkflowInitiated WorkflowRestarted WorkflowFailed WorkflowAborted\n" +
			"--   WorkflowPaused WorkflowUnpaused WorkflowRetried WorkflowUpdated WorkflowUpgraded\n" +
			"--   WorkflowConflicted WorkflowResolved WorkflowJumpToOptionApplied\n" +
			"--   StartEventExecuted EndEventExecuted DecisionExecuted JumpExecuted\n" +
			"--   ParallelSplitExecuted ParallelMergeExecuted CallWorkflowStarted CallWorkflowEnded\n" +
			"--   CallMicroflowStarted CallMicroflowEnded WaitForNotificationStarted WaitForNotificationEnded\n" +
			"--   WaitForTimerStarted WaitForTimerEnded UserTaskStarted MultiUserTaskOutcomeSelected UserTaskEnded\n" +
			"--   NonInterruptingTimerEventExecuted InterruptingTimerEventExecuted\n" +
			"--   11.10+: AIAgentTaskStarted AIAgentTaskEnded\n" +
			"--           NonInterruptingNotificationEventSubProcessStartExecuted\n" +
			"--           InterruptingNotificationEventSubProcessStartExecuted\n" +
			"--   11.13+: NotificationStarted NotificationEnded\n" +
			"--           NonInterruptingNotificationEventExecuted InterruptingNotificationEventExecuted\n" +
			"--           NonInterruptingTimerEventSubProcessStartExecuted InterruptingTimerEventSubProcessStartExecuted\n\n" +
			"-- ANY WORKFLOW EVENT stores every type the project's Mendix version has (Studio Pro\n" +
			"-- stores the list, not a flag), so it needs Mendix 11.6+; name the types before that.\n\n" +
			"-- A user task's on-created microflow (see workflow.user-task) takes exactly\n" +
			"-- System.WorkflowUserTask and the context entity, in either order (else CE6683),\n" +
			"-- and returns nothing (else CE5012).",
		Example: "mdl 1;\n" +
			"CREATE MICROFLOW HR.ACT_AuditTask (\n" +
			"  $WorkflowEvent: System.WorkflowEvent,\n" +
			"  $WorkflowRecord: System.WorkflowRecord,\n" +
			"  $WorkflowActivityRecord: System.WorkflowActivityRecord\n" +
			") BEGIN END;\n\n" +
			"CREATE WORKFLOW HR.LeaveApproval\n" +
			"  PARAMETER $Request: HR.LeaveRequest\n" +
			"  ON WORKFLOW EVENTS (UserTaskStarted, UserTaskEnded) MICROFLOW HR.ACT_AuditTask AS 'Task audit'\n" +
			"  ON ANY WORKFLOW EVENT MICROFLOW HR.ACT_LogEvent AS 'OnAnyEvent'\n" +
			"BEGIN\n" +
			"  USER TASK Review 'Review the request'\n" +
			"    PAGE HR.ReviewPage\n" +
			"    ON CREATED MICROFLOW HR.ACT_AssignReviewer\n" +
			"    OUTCOMES 'Approve' { } 'Reject' { };\n" +
			"END WORKFLOW;",
		SeeAlso: []string{"workflow.create", "workflow.user-task"},
	})

	Register(SyntaxFeature{
		Path:     "workflow.user-task",
		Features: []string{"workflows/user_task"},
		Summary:  "User task activity — assigns work to users with outcomes",
		Keywords: []string{
			"user task", "human task", "assign", "assignee", "auto assign", "auto-assign",
			"outcomes", "approve", "reject", "page",
		},
		// The task page's parameter rule is the platform's, not MDL's, and it was
		// nowhere in this help — a team building workflows from MDL found it by
		// failing a build. Measured on 11.13: no parameters is CE7410, parameters
		// without a WorkflowUserTask one is CE7412, and a WorkflowUserTask
		// parameter PLUS others builds clean, so the help must not say "exactly".
		Syntax: "[MULTI] USER TASK <name> '<caption>'\n" +
			"  PAGE Module.TaskPage\n" +
			"  [TARGETING [USERS | GROUPS] MICROFLOW Module.MF | TARGETING [USERS | GROUPS] XPATH [<xpath>]]\n" +
			"  [AUTO ASSIGN SINGLE USER]  -- assign the task when targeting yields exactly one user\n" +
			"  [ON CREATED MICROFLOW Module.MF]  -- (System.WorkflowUserTask, <context entity>), returns nothing\n" +
			"  [ENTITY Module.Entity]\n" +
			"  [DUE DATE <expression>]\n" +
			"  [TASK NAME '<text>' [WITH ({1} = <expression>, ...)]]  -- what the task inbox shows; default: the caption\n" +
			"  [DESCRIPTION '<text>' [WITH ({1} = <expression>, ...)]]\n" +
			"  OUTCOMES '<outcome1>' { <activities> } '<outcome2>' { <activities> };\n\n" + "-- The clauses are a SET: write them in ANY order, each at most once\n" +
			"-- (ako/mxcli#586). The two TARGETING spellings are ONE clause — a task\n" +
			"-- stores one user source — so writing both is refused rather than\n" +
			"-- letting the second silently win. OUTCOMES and BOUNDARY EVENT are\n" +
			"-- list-valued and may repeat.\n\n" +
			"-- TASK NAME and DESCRIPTION are templates: WITH fills {1}, {2}, ... from\n" +
			"-- expressions over $WorkflowContext and $WorkflowInstance ($WorkflowUserTask\n" +
			"-- is CE0117, MDL-WF19); a placeholder no parameter fills is CE0720\n" +
			"-- (MDL-WF21). ALTER sets them with SET (TaskName: '…' WITH (…)) ON <task>.\n\n" +
			"-- AUTO ASSIGN SINGLE USER is Studio Pro's \"Auto-assign when targeting\n" +
			"-- results in one user\". Omitted, it is off, as Studio Pro creates a task;\n" +
			"-- CREATE OR MODIFY refuses to turn a stored one off unless restated.\n\n" +
			"-- The task page is opened with the TASK, not with the workflow's context\n" +
			"-- object, so it must take a System.WorkflowUserTask parameter:\n" +
			"--   page with no parameters              -> CE7410\n" +
			"--   page without a WorkflowUserTask one  -> CE7412\n" +
			"-- Other parameters may sit alongside it.",
		Example: "-- The task page takes the task:\n" +
			"CREATE PAGE HR.ReviewPage (\n  title: 'Review',\n  layout: Atlas_Core.Atlas_Default,\n  params: ( $WorkflowUserTask: System.WorkflowUserTask )\n) { };\n\n" +
			"USER TASK ReviewTask 'Review the request'\n  PAGE HR.ReviewPage\n  TARGETING USERS XPATH [Active = true()]\n  TASK NAME 'Review {1}' WITH ({1} = $WorkflowContext/Title)\n  OUTCOMES 'Approve' { } 'Reject' { };",
		SeeAlso: []string{"workflow.user-task.targeting", "workflow.multi-user-task", "workflow.create"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.user-task.targeting",
		Summary: "Control who can pick up a user task — microflow or XPath based",
		Keywords: []string{
			"targeting", "user targeting", "who can execute",
			"assignee", "candidate", "xpath", "microflow",
			"task assignment", "user filter", "workflow group",
			"targeting microflow parameters", "targeting signature",
		},
		// The targeting microflow's signature was undocumented, and both of its
		// traps are ones a reading of the error text gets wrong. Measured on
		// 11.13: exactly TWO parameters (one, none, or a third is CE6677), in
		// EITHER order, and the context parameter may be a GENERALIZATION of the
		// context entity but not a specialization. Users and groups share it.
		Syntax: "TARGETING [USERS] MICROFLOW Module.MF    -- returns a List of System.User\n" +
			"TARGETING GROUPS MICROFLOW Module.MF     -- returns a List of System.WorkflowGroup\n" +
			"TARGETING [USERS | GROUPS] XPATH [<xpath-expression>]\n\n" +
			"-- The XPath is written in brackets, as everywhere else, so quotes inside\n" +
			"-- it are written once. The quoted form XPATH '[...]' still parses and\n" +
			"-- warns MDL-DEPR031; `mxcli fmt --upgrade` rewrites it.\n\n" +
			"-- The constraint is evaluated on the user (System.User, or the App Settings\n" +
			"-- > Workflows user entity), so it names the user's members directly:\n" +
			"-- [Name = 'x'], [System.UserRoles = '[%UserRole_Manager%]']. An entity as\n" +
			"-- the first step ([System.User/Name = 'x']) is CE0161; check -p reports it.\n\n" +
			"-- A targeting microflow takes EXACTLY two parameters, in either order:\n" +
			"--   System.Workflow\n" +
			"--   the workflow's context entity, or a generalization of it\n" +
			"-- One parameter, none, a third, or a specialization of the context\n" +
			"-- entity is CE6677.",
		Example: "-- XPath targeting: only active managers\nUSER TASK Approve 'Approve request'\n  TARGETING USERS XPATH [System.UserRoles = '[%UserRole_Manager%]' and Active = true()]\n  OUTCOMES 'Done' { };\n\n" +
			"-- Microflow targeting: the microflow takes the workflow AND its context object\n" +
			"CREATE MICROFLOW HR.GetApprovers ($Workflow: System.Workflow, $Request: HR.Request)\nRETURNS List of System.User AS $Approvers\nBEGIN\n  RETRIEVE $Approvers FROM System.User;\n  RETURN $Approvers;\nEND;\n\n" +
			"USER TASK Approve 'Approve request'\n  TARGETING MICROFLOW HR.GetApprovers\n  OUTCOMES 'Done' { };",
		Features: []string{"workflows/user_task", "workflows/group_targeting"},
		SeeAlso:  []string{"workflow.user-task", "workflow.multi-user-task"},
	})

	Register(SyntaxFeature{
		Path:     "workflow.multi-user-task",
		Features: []string{"workflows/multi_user_task", "workflows/multi_user_task_majority_threshold", "workflows/multi_user_task_await_all", "workflows/multi_user_task_microflow_rule"},
		Summary:  "Multi-user task — one activity that several users each act on",
		Keywords: []string{
			"multi user task", "multi-user task", "several users", "multiple assignees",
			"assessors", "voting", "parallel approval", "group approval",
			"decide by", "consensus", "majority", "threshold", "veto", "fallback",
			"participants", "await all users", "completion criteria",
		},
		// The grammar has accepted MULTI USER TASK since issue #8, and a team
		// shipped one fanning out to eight assessors — but there was no topic for
		// it, so it read as unsupported. It takes USER TASK's clauses and obeys the
		// same page rule (measured: a context-typed page is CE7412 on a multi-user
		// task too) and the same targeting rule.
		Syntax: "MULTI USER TASK <name> '<caption>'\n" +
			"  PAGE Module.TaskPage\n" +
			"  [TARGETING [USERS | GROUPS] MICROFLOW Module.MF | TARGETING [USERS | GROUPS] XPATH [<xpath>]]\n" +
			"  [AUTO ASSIGN SINGLE USER]  -- assign the task when targeting yields exactly one user\n" +
			"  [ON CREATED MICROFLOW Module.MF]  -- (System.WorkflowUserTask, <context entity>), returns nothing\n" +
			"  [ENTITY Module.Entity]\n" +
			"  [DUE DATE <expression>]\n" +
			"  [DESCRIPTION '<text>']\n" +
			"  [PARTICIPANTS ALL | <n> | <n> PERCENT]\n" +
			"  [DECIDE BY <rule>]\n" +
			"  [AWAIT ALL USERS]\n" +
			"  OUTCOMES '<outcome1>' { <activities> } '<outcome2>' { <activities> };\n\n" +
			"-- Rules:\n" +
			"--   DECIDE BY CONSENSUS FALLBACK '<outcome>'\n" +
			"--   DECIDE BY MAJORITY MORE THAN HALF FALLBACK '<outcome>'\n" +
			"--   DECIDE BY MAJORITY MOST CHOSEN FALLBACK '<outcome>'\n" +
			"--   DECIDE BY THRESHOLD <n> PERCENT | <n> VOTES FALLBACK '<outcome>'\n" +
			"--   DECIDE BY VETO '<outcome>'\n" +
			"--   DECIDE BY MICROFLOW Module.Decide      -- returns String (else CE5012)\n" +
			"-- The fallback is required for consensus, majority and threshold (CE1866); a\n" +
			"-- veto needs its outcome (CE1867); both must name one of the task's outcomes\n" +
			"-- (MDL-WF13). Omitted: all participants, consensus on the first outcome, not\n" +
			"-- waiting. The build does not range-check thresholds or participant counts.\n" +
			"-- Same page and targeting rules as USER TASK, and the same clause rule:\n" +
			"-- ANY order, each at most once (ako/mxcli#586).",
		Example: "MULTI USER TASK ExpertAssessment 'Expert assessment'\n  PAGE MOC.AssessmentPage\n  TARGETING MICROFLOW MOC.GetAssessors\n  PARTICIPANTS 80 PERCENT\n  DECIDE BY THRESHOLD 60 PERCENT FALLBACK 'Reject'\n  AWAIT ALL USERS\n  OUTCOMES 'Approve' { } 'Reject' { };",
		SeeAlso: []string{"workflow.user-task", "workflow.user-task.targeting"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.decision",
		Summary: "Decision activity — conditional branching based on expression outcomes",
		Keywords: []string{
			"decision", "conditional", "branch", "if", "condition",
			"exclusive gateway", "XOR",
		},
		// A decision outcome needs the arrow ('Under 1000' -> { }); a USER TASK
		// outcome does not ('OK' { }). The two read alike but are separate
		// grammar rules, so the arrow is easy to drop — this entry did, and
		// taught the broken form until TestExamplesParse started checking it.
		//
		// The outcome VALUE is not free text either: Mendix stores it as an
		// EnumerationValueIdentifier and parses it when the project is loaded,
		// so anything but Module.Enumeration.Value leaves a project Studio Pro
		// cannot open. This entry taught 'Under 1000' / 'Over 1000' — both of
		// which corrupt the model (ako/mxcli#1031, ako/mxcli#1065).
		Syntax: "-- Boolean decision:\n" +
			"DECISION [<name>] <boolean-expression> [CAPTION '<text>']\n  OUTCOMES TRUE -> { <activities> } FALSE -> { <activities> };\n\n" +
			"-- Enumeration decision — each outcome is a QUALIFIED enum value,\n" +
			"-- plus one '' outcome for 'none of the above' (without it: CE6686):\n" +
			"DECISION [<name>] <enum-expression> [CAPTION '<text>']\n  OUTCOMES 'Module.Enumeration.Value' -> { <activities> } ... '' -> { };\n\n" +
			"-- The expression is bare (R5). The older string form, DECISION '<expression>',\n" +
			"-- is the deprecated alias MDL-DEPR080, as it is for timers and due dates.",
		Example: "-- Boolean\nDECISION decision1 $WorkflowContext/Amount > 1000\n  OUTCOMES\n    TRUE -> {\n      USER TASK ManagerApproval 'Manager must approve'\n        OUTCOMES 'OK' { };\n    }\n    FALSE -> { };\n\n" +
			"-- Enumeration: the value must be Module.Enumeration.Value.\n-- A bare 'Approved' makes the project UNLOADABLE, not merely invalid.\nDECISION decision2 $WorkflowContext/Status\n  OUTCOMES\n    'Sales.ENUM_Status.Approved' -> { }\n    'Sales.ENUM_Status.Rejected' -> { }\n    '' -> { };",
		SeeAlso: []string{"workflow.create", "workflow.parallel-split"},
	})

	Register(SyntaxFeature{
		Path:     "workflow.parallel-split",
		Features: []string{"workflows/parallel_split"},
		Summary:  "Parallel split — execute multiple paths concurrently",
		Keywords: []string{
			"parallel", "concurrent", "split", "fork", "join",
			"parallel gateway", "AND",
		},
		Syntax:  "PARALLEL SPLIT [<name>] [CAPTION '<text>']\n  PATH 1 { <activities> }\n  PATH 2 { <activities> };",
		Example: "PARALLEL SPLIT\n  PATH 1 {\n    USER TASK LegalReview 'Legal review'\n      OUTCOMES 'Done' { };\n  }\n  PATH 2 {\n    USER TASK TechReview 'Technical review'\n      OUTCOMES 'Done' { };\n  };",
		SeeAlso: []string{"workflow.decision", "workflow.create"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.call-microflow",
		Summary: "Call a microflow as a workflow activity",
		Keywords: []string{
			"call microflow", "microflow task", "automated step",
			"system task",
		},
		// R4 (ako/mxcli#751): arguments are bound as at every call site, right
		// after the callee, the expression bare. The older `WITH (P = '<expr>')`
		// after the caption is the deprecated alias MDL-DEPR008.
		Syntax: "CALL MICROFLOW Module.MF[(<Param> = <expression>, ...)] [AS <name>] [CAPTION '<text>']\n" +
			"  [OUTCOMES '<outcome>' -> { <activities> } ...];\n\n" +
			"-- WITH (<Param> = '<expression>'), the expression in a string, is the\n" +
			"-- deprecated spelling of the argument list (MDL-DEPR008).\n" +
			"-- CAPTION sets the caption the activity shows; COMMENT '<text>', its old\n" +
			"-- spelling on every workflow activity, is a deprecated alias (MDL-DEPR104).\n\n" +
			"-- An argument can name only $WorkflowContext and $WorkflowInstance (the\n" +
			"-- running workflow, System.Workflow). $Workflow, $currentUser and the like\n" +
			"-- are CE0117 at build (MDL-WF19). With no argument list, a System.Workflow\n" +
			"-- parameter is bound to $WorkflowInstance and every other to the context.",
		Example: "CALL MICROFLOW HR.SendNotification\n  CAPTION 'Notify manager';\n\n-- Arguments are named by their BARE parameter name:\nCALL MICROFLOW HR.Escalate(Request = $WorkflowContext) AS callMicroflow1;\n\n" +
			"-- A System.Workflow parameter takes the running workflow:\nCALL MICROFLOW HR.ACT_Audit(Workflow = $WorkflowInstance, Request = $WorkflowContext);",
		SeeAlso: []string{"workflow.create", "workflow.call-workflow", "workflow.ai-agent-task"},
	})

	Register(SyntaxFeature{
		Path:     "workflow.ai-agent-task",
		Features: []string{"workflows/ai_agent_task"},
		Summary:  "AI agent task — a workflow step that runs an agent through a microflow (Mendix 11.9+)",
		Keywords: []string{
			"ai agent task", "agent task", "call agent", "call agent microflow",
			"agent", "llm", "genai", "AIAgentTaskActivity",
		},
		// Measured on mxbuild 11.13.0 against the same activity written as a call
		// microflow: the only difference in what builds is CE1590 for a microflow
		// with no parameters.
		Syntax: "CALL AGENT MICROFLOW Module.MF[(<Param> = <expression>, ...)] [AS <name>] [CAPTION '<text>']\n" +
			"  [OUTCOMES <true|false|'Module.Enum.Value'|''> -> { <activities> } ...]\n" +
			"  [BOUNDARY EVENT ...];\n\n" +
			"-- The same statement as CALL MICROFLOW, stored as an AI agent task. The microflow\n" +
			"-- is where the agent is invoked; it must take at least one parameter (else CE1590),\n" +
			"-- usually the workflow's context object. Return Boolean or an enumeration to\n" +
			"-- branch on the agent's answer with OUTCOMES. Needs Mendix 11.9+.",
		Example: "CREATE MICROFLOW HR.ACT_ClassifyRequest ($Request: HR.LeaveRequest)\n" +
			"RETURNS Boolean AS $Urgent\nBEGIN\n  -- call the agent here\n  RETURN false;\nEND;\n\n" +
			"CALL AGENT MICROFLOW HR.ACT_ClassifyRequest(Request = $WorkflowContext) AS aiAgentTask1\n" +
			"  CAPTION 'Classify the request'\n" +
			"  OUTCOMES true -> { USER TASK Expedite 'Expedite' PAGE HR.TaskPage OUTCOMES 'Done' { }; }\n" +
			"           false -> { };",
		SeeAlso: []string{"workflow.call-microflow", "agents"},
	})

	Register(SyntaxFeature{
		Path:     "workflow.call-workflow",
		Features: []string{"workflows/call_workflow_activity", "workflows/call_workflow_parameter_mappings", "workflows/workflow_parameter"},
		Summary:  "Call a sub-workflow from within a workflow",
		Keywords: []string{
			"call workflow", "sub-workflow", "nested workflow",
		},
		Syntax: "CALL WORKFLOW Module.WF[(WorkflowContext = <expression>)] [AS <name>] [CAPTION '<text>']\n" +
			"  [BOUNDARY EVENT ...];\n\n" +
			"-- A workflow has exactly one parameter and Mendix stores it as WorkflowContext,\n" +
			"-- whatever name the called workflow's `PARAMETER $Name: ...` header declares — so\n" +
			"-- the mapping key is always WorkflowContext. Any other key is CE1613 (MDL-WF18).\n" +
			"-- Omitted, the call passes the caller's own $WorkflowContext.\n\n" +
			"-- This is the workflow ACTIVITY. To start a workflow from a microflow\n-- ($Wf = CALL WORKFLOW Module.WF ($Obj);) see microflow.workflow-actions.",
		Example: "CALL WORKFLOW HR.SubApproval CAPTION 'Delegate to sub-process';\n\n" +
			"CALL WORKFLOW HR.SubApproval(WorkflowContext = $WorkflowContext) AS callWf1;\n\n" +
			"-- pass an associated object: the path names the association AND its target entity\n" +
			"CALL WORKFLOW HR.ReviewLine(WorkflowContext = $WorkflowContext/HR.Request_Line/HR.Line) AS callWf2;\n\n" +
			"-- put a deadline on the child without moving the timer into it\n" +
			"CALL WORKFLOW HR.SubApproval AS callWf3\n" +
			"  BOUNDARY EVENT INTERRUPTING TIMER addDays([%CurrentDateTime%], 5) { END WORKFLOW; };",
		SeeAlso: []string{"workflow.create", "workflow.call-microflow", "workflow.boundary-event", "microflow.workflow-actions"},
	})

	Register(SyntaxFeature{
		Path:     "workflow.jump-to",
		Features: []string{"workflows/jump_to", "workflows/wait_for_timer"},
		Summary:  "Jump to another activity — and the activity names it resolves against",
		Keywords: []string{
			"jump", "jump to", "goto", "loop back", "activity name",
		},
		// Mendix stores JumpToActivity.TargetActivity as an activity NAME, not a
		// pointer, so the jump is only as good as the name. Studio Pro names
		// activities by type and ordinal regardless of caption (decision1,
		// split1, callMicroflow1); mxcli derives a name when none is given, which
		// is why an explicit one matters when reproducing a stored workflow.
		Syntax: "JUMP TO <activity-name> [CAPTION '<text>'];\n\n" +
			"-- name the target so the jump resolves:\n" +
			"DECISION <name> ['<caption>'] ...\nPARALLEL SPLIT <name> ...\n" +
			"WAIT FOR TIMER <name> ...\nWAIT FOR NOTIFICATION <name>\n" +
			"CALL MICROFLOW Module.MF AS <name>\nCALL WORKFLOW Module.WF AS <name>",
		Example: "DECISION decision1 $WorkflowContext/Total > 1000\n" +
			"  OUTCOMES\n    true -> { }\n    false -> { };\n\n" +
			"PARALLEL SPLIT split1\n  PATH 1 { JUMP TO decision1; }\n  PATH 2 { };",
		SeeAlso: []string{"workflow.create", "workflow.decision", "workflow.parallel-split", "workflow.end"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.end",
		Summary: "End the workflow from inside a branch — the workflow counterpart of a microflow's return",
		Keywords: []string{
			"end workflow", "end event", "end activity", "stop workflow", "terminate",
			"early exit", "reject ends the workflow", "return",
		},
		// Every placement rule below is an mxbuild measurement (11.13.0), not a
		// reading of the error text. See docs/11-proposals/PROPOSAL_workflow_end_activity.md.
		Syntax: "end workflow [caption '<caption>'];\n\n" +
			"-- Legal as the LAST statement of a user-task outcome, a decision branch,\n" +
			"-- a call-microflow outcome or an interrupting boundary-event path, at any depth.\n" +
			"-- It ends the WHOLE workflow, not the block it is written in.\n" +
			"--\n" +
			"-- Refused:\n" +
			"--   under a parallel split or a non-interrupting boundary path   CE1844  MDL-WF08\n" +
			"--   followed by anything in the same block                        CE6671  MDL-WF09\n" +
			"--   when every path of an activity ends and more follows          CE6689  MDL-WF10\n" +
			"--   `return;` — a microflow's spelling                            MDL-WF11\n" +
			"--\n" +
			"-- The main flow needs none: the body's own closing `end workflow` is its End.\n" +
			"-- `caption` sets the End's caption on the canvas. An End is not a jump target.",
		Example: "USER TASK Review 'Review the request'\n" +
			"  PAGE HR.ReviewPage\n" +
			"  OUTCOMES\n" +
			"    'Approve' { }\n" +
			"    'Reject' {\n" +
			"      CALL MICROFLOW HR.ACT_NotifyRejected;\n" +
			"      END WORKFLOW CAPTION 'Rejected';\n" +
			"    }\n" +
			"  BOUNDARY EVENT INTERRUPTING TIMER addDays([%CurrentDateTime%], 5) {\n" +
			"    END WORKFLOW CAPTION 'Expired';\n" +
			"  };",
		SeeAlso: []string{"workflow.user-task", "workflow.decision", "workflow.boundary-event", "workflow.jump-to"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.drop",
		Summary: "Delete a workflow definition",
		Keywords: []string{
			"drop workflow", "delete workflow", "remove workflow",
		},
		Syntax:  "DROP WORKFLOW Module.Name;",
		Example: "DROP WORKFLOW HR.LeaveApproval;",
		SeeAlso: []string{"workflow.create"},
	})

	Register(SyntaxFeature{
		Path:     "workflow.catalog",
		Features: []string{"workflows/groups"},
		Summary:  "Query workflow metadata via catalog tables",
		Keywords: []string{
			"catalog", "query workflows", "workflow metadata",
			"cross-reference", "callers", "callees",
		},
		Syntax:  "REFRESH CATALOG FULL;\nSELECT * FROM CATALOG.WORKFLOWS;\nLIST CALLERS OF Module.WorkflowName;\nLIST REFERENCES TO Module.WorkflowName;",
		Example: "REFRESH CATALOG FULL;\nSELECT QualifiedName, ActivityCount, UserTaskCount\n  FROM CATALOG.WORKFLOWS WHERE UserTaskCount > 0;",
		SeeAlso: []string{"workflow.show"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.boundary-event",
		Summary: "Attach boundary events (timer) to user tasks, calls and waits for timeouts",
		Keywords: []string{
			"boundary event", "timer", "timeout", "deadline",
			"SLA", "escalation",
		},
		// The kind is REQUIRED on Mendix 11. A bare `TIMER` writes
		// Workflows$TimerBoundaryEvent, a type no 11.x runtime has: check and
		// mxbuild pass, and the runtime refuses to start the application ("Class
		// 'Workflows$TimerBoundaryEvent' could not be found"). This entry showed
		// the bare form, with 'P3D' — not a valid timer expression — as its delay.
		// ako/view-entity-examples FINDINGS §7.
		Syntax:   "-- inline, as a clause of a USER TASK or CALL MICROFLOW (after OUTCOMES),\n-- a CALL WORKFLOW or a WAIT FOR NOTIFICATION:\nBOUNDARY EVENT (INTERRUPTING | NON INTERRUPTING) TIMER <datetime-expression> [REPEAT EVERY <n> <unit> AT MOST <m> TIMES] [CAPTION '<caption>'] { <activities> }\nBOUNDARY EVENT (INTERRUPTING | NON INTERRUPTING) NOTIFICATION <name> ['<caption>'] { <activities> }\n\n-- A notification boundary event (Mendix 11.11+) is triggered by `notify workflow`\n-- targeting its name, so the name is unique in the workflow. ALTER cannot insert\n-- one yet; restate the workflow instead.\n\n-- or add a timer to an existing task:\nALTER WORKFLOW <wf> { INSERT INTO <task> { BOUNDARY EVENT (INTERRUPTING | NON INTERRUPTING) TIMER <datetime-expression> [CAPTION '<caption>'] { <activities> } } };\n\n-- REPEAT EVERY repeats a NON INTERRUPTING timer (a reminder): unit minute(s),\n-- hour(s), day(s), week(s) or month(s); the count includes the first firing\n-- and is at least 2. MDL-WF22 refuses what Studio Pro refuses (mx check does\n-- not): an interrupting timer, another unit, fewer than 2.\n\n-- An activity carries at most 5 boundary events (mx check CE6696);\n-- MDL-WF23 refuses a sixth.\n\n-- Name the kind: a bare TIMER is refused on Mendix 11 (MDL-WF07) — it writes a\n-- type the runtime cannot load. The delay is an expression that yields a\n-- DateTime, e.g. addDays([%CurrentDateTime%], 3). mxcli ends every boundary\n-- path with Mendix's end-of-path marker, so a path may end in a call; use\n-- JUMP TO to return to the task instead.",
		Example:  "user task ReviewTask 'Review'\n  page Module.WF_Review\n  outcomes 'Done' { }\n  boundary event interrupting timer addDays([%CurrentDateTime%], 3) caption 'Escalate' {\n    call microflow Module.WF_Escalate;\n  };",
		Features: []string{"workflows/timer_boundary_event", "workflows/interrupting_timer_boundary_event"},
		SeeAlso:  []string{"workflow.user-task", "workflow.notification", "workflow.event-subprocess"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.event-subprocess",
		Summary: "Event sub-processes — flows a notification or timer starts while the workflow runs",
		Keywords: []string{
			"event subprocess", "event sub-process", "esp", "cancel workflow",
			"interrupting", "non interrupting", "notification start", "timer start",
		},
		Syntax: "-- after the main body, before END WORKFLOW:\n" +
			"EVENT SUBPROCESS <name> ['<caption>']\n" +
			"  ON (INTERRUPTING | NON INTERRUPTING) NOTIFICATION [<start>] ['<start caption>']\n" +
			"  { <activities> };\n" +
			"EVENT SUBPROCESS <name> ['<caption>']\n" +
			"  ON (INTERRUPTING | NON INTERRUPTING) TIMER <first-execution-time> [REPEAT EVERY <n> <unit> AT MOST <m> TIMES] [AS <start>] [CAPTION '<start caption>']\n" +
			"  { <activities> };\n\n" +
			"-- Interrupting cancels every active path first; non-interrupting runs alongside.\n" +
			"-- A notification start is what `notify workflow` targets. The body's End is\n" +
			"-- implicit, as in the main flow; `jump to` stays inside its own sub-process\n" +
			"-- (CE6682, MDL-WF05). A timer needs its expression (CE0126, MDL-WF14).\n" +
			"-- Mendix 11.8+ for notification starts, 11.13+ for timer starts.",
		Example: "create workflow HR.Leave parameter $Ctx: HR.Request\n" +
			"begin\n" +
			"  user task Review 'Review' page HR.ReviewPage outcomes 'Approve' { } 'Reject' { };\n" +
			"  event subprocess ESP_Cancel 'Cancel request'\n" +
			"    on interrupting notification espCancelStart 'Cancel received' {\n" +
			"    call microflow HR.ACT_LogCancel;\n" +
			"  };\n" +
			"  event subprocess ESP_Reminder 'Daily reminder'\n" +
			"    on non interrupting timer addDays([%CurrentDateTime%], 1) as espReminderStart {\n" +
			"    call microflow HR.ACT_Remind;\n" +
			"  };\n" +
			"end workflow;",
		Features: []string{"workflows/event_subprocesses", "workflows/timer_event_subprocesses", "workflows/recurring_timer_event_subprocesses"},
		SeeAlso:  []string{"workflow.notification", "workflow.boundary-event", "workflow.create"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.notification",
		Summary: "Notification activity — an intermediate notification event on a flow",
		Keywords: []string{
			"notification", "notification activity", "intermediate event", "notify",
		},
		Syntax: "NOTIFICATION [<name>] [CAPTION '<caption>'];\n\n" +
			"-- The point a `notify workflow` action reaches (Workflows$NotificationActivity,\n" +
			"-- Mendix 11.11+). `wait for notification` is the older activity for the same\n" +
			"-- purpose and works on every version.",
		Example:  "notification DocumentsReceived caption 'Documents received';",
		Features: []string{"workflows/notification_events", "workflows/wait_for_notification", "workflows/interrupting_notification_event_subprocess"},
		SeeAlso:  []string{"workflow.event-subprocess", "workflow.boundary-event", "workflow.notify"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.notify",
		Summary: "Notify a running workflow — resume the element the notification targets",
		Keywords: []string{
			"notify workflow", "notify", "notify target", "resume workflow", "trigger event sub-process",
		},
		Syntax: "[$Notified =] NOTIFY WORKFLOW $Workflow TARGET Module.Workflow.ElementName;\n\n" +
			"-- A microflow statement. The target names a notification-started event\n" +
			"-- sub-process's start, a notification activity, a notification boundary event\n" +
			"-- or a wait for notification; mxcli looks up which it is and refuses an element\n" +
			"-- a notification cannot reach (a timer start, a user task). The target is\n" +
			"-- required: without one the build fails CE0166 (MDL-WF16). $Notified is a\n" +
			"-- Boolean. Before Mendix 11.7 only a wait for notification can be named.",
		Example:  "$Cancelled = notify workflow $Workflow target HR.Leave.espCancelStart;",
		Features: []string{"workflows/notify_workflow", "workflows/notify_target"},
		SeeAlso:  []string{"workflow.event-subprocess", "workflow.notification", "workflow.boundary-event"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.set-task-outcome",
		Summary: "Complete a user task from a microflow with a named outcome",
		Keywords: []string{
			"set task outcome", "task outcome", "complete task", "complete user task",
			"outcome variable", "dynamic outcome", "claim and complete", "claim task",
			"MDL-WORKFLOW10", "task page", "outcome button", "MDL-WF20",
		},
		Syntax: "SET TASK OUTCOME $UserTask 'OutcomeName';\n\n" +
			"-- On a task page the same is a button action, with no microflow:\n" +
			"--   actionbutton btnApprove (Caption: 'Approve', Action: complete task 'Approve')\n" +
			"-- A task page needs one per outcome; check warns MDL-WF20 when a task's\n" +
			"-- page cannot complete one of its outcomes. Either way the user must\n" +
			"-- have claimed the task.\n\n" +
			"-- A microflow statement (not a nanoflow one). $UserTask is a\n" +
			"-- System.WorkflowUserTask; the outcome is one of that task's outcomes.\n" +
			"-- The outcome is a quoted LITERAL, by design: Mendix stores it by name\n" +
			"-- (SetTaskOutcomeAction.Outcome is a reference to the outcome, resolved\n" +
			"-- when the app is built), so it cannot be a variable or an expression.\n" +
			"-- `set task outcome $Task $Outcome;` is a parse error that says so.\n\n" +
			"-- A shared claim-and-complete flow therefore takes the outcome as a\n" +
			"-- parameter and branches once per outcome, each with its own literal.\n" +
			"-- Claim the task first: completing an unclaimed task fails at runtime\n" +
			"-- (\"it is not assigned to you\"); check warns MDL-WORKFLOW10.",
		Example: "create microflow Approvals.ACT_CompleteTask (\n" +
			"  $Task: System.WorkflowUserTask,\n  $Outcome: String\n)\nbegin\n" +
			"  change $Task (System.WorkflowUserTask_Assignees = [%CurrentUser%]);\n" +
			"  commit $Task;\n" +
			"  if $Outcome = 'Approve' then\n" +
			"    set task outcome $Task 'Approve';\n" +
			"  else\n" +
			"    set task outcome $Task 'Reject';\n" +
			"  end if;\n" +
			"end;",
		SeeAlso: []string{"workflow.user-task", "workflow.notify"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.alter",
		Summary: "Modify an existing workflow — change properties, add/remove activities",
		Keywords: []string{
			"alter workflow", "modify workflow", "update workflow",
			"add activity", "drop activity", "replace activity",
			"insert before", "insert into", "insert outcome", "insert path",
		},
		// The generic ALTER (ADR-0012, ako/mxcli#712): the operations go in
		// { }, a target is an activity's name or 'caption' with @n to choose one
		// of several matches (an ambiguous one is refused with the matches
		// listed), and a fragment is written exactly as in CREATE WORKFLOW.
		// The old per-action forms (`SET DISPLAY '…'`, `SET ACTIVITY X …`,
		// `INSERT OUTCOME 'x' ON X { … }`, `DROP PATH 'Path 2' ON X`) still
		// parse and warn MDL-DEPR140-149; `mxcli fmt --upgrade` rewrites them.
		//
		// What `insert into` adds must fit the activity: an outcome list is
		// typed per activity kind — a user task takes `outcomes 'x' { … }`, a
		// decision or call microflow `outcomes <value> -> { … }`, a parallel
		// split `path { … }`. The wrong one is refused (it would leave a project
		// Mendix cannot LOAD, ako/mxcli#415).
		Syntax: "ALTER WORKFLOW Module.Name {\n" +
			"  SET ( Display: '<text>', Description: '<text>', ExportLevel: API | Hidden,\n" +
			"        DueDate: <expression>, OverviewPage: Module.Page, Parameter: $WorkflowContext: Module.Entity,\n" +
			"        Annotation: '<note>' );\n" +
			"  SET ( Page: Module.Page, Description: '<text>', TaskName: '<text>', DueDate: <expression>,\n" +
			"        Targeting: microflow Module.Mf | Targeting: xpath [<xpath>], Annotation: '<note>' ) ON <activity>;\n" +
			"  INSERT BEFORE | AFTER <activity> { <activities> }\n" +
			"  INSERT INTO <user-task> { OUTCOMES '<name>' { <activities> } }\n" +
			"  INSERT INTO <decision | call-microflow> { OUTCOMES true | false | default | '<value>' -> { <activities> } }\n" +
			"  INSERT INTO <parallel-split> { PATH [<next number>] { <activities> } }\n" +
			"  INSERT INTO <activity> { BOUNDARY EVENT (INTERRUPTING | NON INTERRUPTING) TIMER <datetime-expression> { <activities> } }\n" +
			"  REPLACE <activity> WITH { <activities> }\n" +
			"  DROP <activity>, <activity> OUTCOME '<name>' | true | false | default,\n" +
			"       <parallel-split> PATH <n>, <activity> BOUNDARY EVENT;\n" +
			"};\n\n" +
			"-- <activity> is a name (ReviewTask) or a caption ('Review the request'),\n" +
			"-- optionally @n to choose one of several matches: describe workflow prints the names.\n" +
			"-- Annotation is the note CREATE writes: the header's ANNOTATION '…' without ON, the\n" +
			"-- @annotation '…' before the activity with ON; '' removes it.",
		Example: "alter workflow HR.LeaveApproval {\n" +
			"  set (DueDate: addDays([%CurrentDateTime%], 7));\n" +
			"  set (Page: HR.ReviewPage) on ReviewTask;\n" +
			"  insert after ReviewTask { call microflow HR.NotifyHR; }\n" +
			"  insert into ReviewTask { outcomes 'Rejected' { } }\n" +
			"  insert into Triage { outcomes 'HR.Status.Urgent' -> { } }\n" +
			"  insert into NotifyAll { path { } }\n" +
			"  drop ObsoleteStep;\n" +
			"};",
		SeeAlso: []string{"workflow.create", "workflow.drop"},
	})
}
