// SPDX-License-Identifier: Apache-2.0

package syntax

func init() {
	Register(SyntaxFeature{
		Path:    "workflow",
		Summary: "Multi-step business processes with user tasks, decisions, and parallel paths",
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
			"list workflows", "show workflows", "describe workflow",
		},
		Syntax:  "SHOW WORKFLOWS;\nSHOW WORKFLOWS IN <module>;\nDESCRIBE WORKFLOW Module.Name;",
		Example: "SHOW WORKFLOWS IN HR;\nDESCRIBE WORKFLOW HR.LeaveApproval;",
	})

	Register(SyntaxFeature{
		Path:    "workflow.create",
		Summary: "Create a new workflow definition with activities and flow",
		Keywords: []string{
			"create workflow", "new workflow", "define workflow",
			"parameter", "overview page", "due date",
		},
		Syntax:  "CREATE [OR MODIFY] WORKFLOW Module.Name\n  [FOLDER 'path']\n  PARAMETER $Context: Module.Entity\n  [OVERVIEW PAGE Module.OverviewPage]\n  [DUE DATE '<expression>']\n  [ON WORKFLOW EVENTS (<type>, ...) MICROFLOW Module.Handler [AS '<description>']]...\n  [ON ANY WORKFLOW EVENT MICROFLOW Module.Handler [AS '<description>']]...\nBEGIN\n  <activities>\nEND WORKFLOW;",
		Example: "CREATE WORKFLOW Module.ApprovalFlow\n  PARAMETER $Context: Module.Request\n  OVERVIEW PAGE Module.WF_Overview\nBEGIN\n  USER TASK ReviewTask 'Review the request'\n    PAGE Module.ReviewPage\n    OUTCOMES 'Approve' { } 'Reject' { };\nEND WORKFLOW;",
		SeeAlso: []string{"workflow.user-task", "workflow.event-handlers", "workflow.decision", "workflow.drop"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.event-handlers",
		Summary: "Run a microflow when workflow events happen (task started, workflow completed, ...)",
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
		Example: "CREATE MICROFLOW HR.ACT_AuditTask (\n" +
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
		Path:    "workflow.user-task",
		Summary: "User task activity — assigns work to users with outcomes",
		Keywords: []string{
			"user task", "human task", "assign", "assignee",
			"outcomes", "approve", "reject", "page",
		},
		// The task page's parameter rule is the platform's, not MDL's, and it was
		// nowhere in this help — a team building workflows from MDL found it by
		// failing a build. Measured on 11.13: no parameters is CE7410, parameters
		// without a WorkflowUserTask one is CE7412, and a WorkflowUserTask
		// parameter PLUS others builds clean, so the help must not say "exactly".
		Syntax: "[MULTI] USER TASK <name> '<caption>'\n" +
			"  PAGE Module.TaskPage\n" +
			"  [TARGETING [USERS | GROUPS] MICROFLOW Module.MF | TARGETING [USERS | GROUPS] XPATH '<xpath>']\n" +
			"  [ON CREATED MICROFLOW Module.MF]  -- (System.WorkflowUserTask, <context entity>), returns nothing\n" +
			"  [ENTITY Module.Entity]\n" +
			"  [DUE DATE '<expression>']\n" +
			"  [DESCRIPTION '<text>']\n" +
			"  OUTCOMES '<outcome1>' { <activities> } '<outcome2>' { <activities> };\n\n" +
			"-- The task page is opened with the TASK, not with the workflow's context\n" +
			"-- object, so it must take a System.WorkflowUserTask parameter:\n" +
			"--   page with no parameters              -> CE7410\n" +
			"--   page without a WorkflowUserTask one  -> CE7412\n" +
			"-- Other parameters may sit alongside it.",
		Example: "-- The task page takes the task:\n" +
			"CREATE PAGE HR.ReviewPage (\n  title: 'Review',\n  layout: Atlas_Core.Atlas_Default,\n  params: { $WorkflowUserTask: System.WorkflowUserTask }\n) { };\n\n" +
			"USER TASK ReviewTask 'Review the request'\n  PAGE HR.ReviewPage\n  TARGETING XPATH '[Module.Employee/Active = true()]'\n  OUTCOMES 'Approve' { } 'Reject' { };",
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
			"TARGETING [USERS | GROUPS] XPATH '<xpath-expression>'\n\n" +
			"-- A targeting microflow takes EXACTLY two parameters, in either order:\n" +
			"--   System.Workflow\n" +
			"--   the workflow's context entity, or a generalization of it\n" +
			"-- One parameter, none, a third, or a specialization of the context\n" +
			"-- entity is CE6677.",
		Example: "-- XPath targeting: only active managers\nUSER TASK Approve 'Approve request'\n  TARGETING XPATH '[HR.Employee/Role = \"Manager\" and Active = true()]'\n  OUTCOMES 'Done' { };\n\n" +
			"-- Microflow targeting: the microflow takes the workflow AND its context object\n" +
			"CREATE MICROFLOW HR.GetApprovers ($Workflow: System.Workflow, $Request: HR.Request)\nRETURNS List of System.User AS $Approvers\nBEGIN\n  RETRIEVE $Approvers FROM System.User;\n  RETURN $Approvers;\nEND;\n\n" +
			"USER TASK Approve 'Approve request'\n  TARGETING MICROFLOW HR.GetApprovers\n  OUTCOMES 'Done' { };",
		MinVersion: "9.0.0",
		SeeAlso:    []string{"workflow.user-task", "workflow.multi-user-task"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.multi-user-task",
		Summary: "Multi-user task — one activity that several users each act on",
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
			"  [TARGETING [USERS | GROUPS] MICROFLOW Module.MF | TARGETING [USERS | GROUPS] XPATH '<xpath>']\n" +
			"  [ON CREATED MICROFLOW Module.MF]  -- (System.WorkflowUserTask, <context entity>), returns nothing\n" +
			"  [ENTITY Module.Entity]\n" +
			"  [DUE DATE '<expression>']\n" +
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
			"-- Same page and targeting rules as USER TASK.",
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
			"DECISION [<name>] '<boolean-expression>' [COMMENT '<text>']\n  OUTCOMES TRUE -> { <activities> } FALSE -> { <activities> };\n\n" +
			"-- Enumeration decision — each outcome is a QUALIFIED enum value,\n" +
			"-- plus one '' outcome for 'none of the above' (without it: CE6686):\n" +
			"DECISION [<name>] '<enum-expression>' [COMMENT '<text>']\n  OUTCOMES 'Module.Enumeration.Value' -> { <activities> } ... '' -> { };",
		Example: "-- Boolean\nDECISION decision1 '$WorkflowContext/Amount > 1000'\n  OUTCOMES\n    TRUE -> {\n      USER TASK ManagerApproval 'Manager must approve'\n        OUTCOMES 'OK' { };\n    }\n    FALSE -> { };\n\n" +
			"-- Enumeration: the value must be Module.Enumeration.Value.\n-- A bare 'Approved' makes the project UNLOADABLE, not merely invalid.\nDECISION decision2 '$WorkflowContext/Status'\n  OUTCOMES\n    'Sales.ENUM_Status.Approved' -> { }\n    'Sales.ENUM_Status.Rejected' -> { }\n    '' -> { };",
		SeeAlso: []string{"workflow.create", "workflow.parallel-split"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.parallel-split",
		Summary: "Parallel split — execute multiple paths concurrently",
		Keywords: []string{
			"parallel", "concurrent", "split", "fork", "join",
			"parallel gateway", "AND",
		},
		Syntax:  "PARALLEL SPLIT [<name>] [COMMENT '<text>']\n  PATH 1 { <activities> }\n  PATH 2 { <activities> };",
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
		// The WITH values are QUOTED — the grammar takes a string literal there,
		// not a bare variable. Omitting the clause from this entry is how an
		// author ends up writing the unquoted form (ako/mxcli#1023).
		Syntax:  "CALL MICROFLOW Module.MF [AS <name>] [COMMENT '<text>']\n  [WITH (<Param> = '<expression>', ...)]\n  [OUTCOMES '<outcome>' -> { <activities> } ...];",
		Example: "CALL MICROFLOW HR.SendNotification\n  COMMENT 'Notify manager';\n\n-- Parameter values are quoted, and named by their BARE parameter name:\nCALL MICROFLOW HR.Escalate AS callMicroflow1\n  WITH (Request = '$WorkflowContext');",
		SeeAlso: []string{"workflow.create", "workflow.call-workflow", "workflow.ai-agent-task"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.ai-agent-task",
		Summary: "AI agent task — a workflow step that runs an agent through a microflow (Mendix 11.9+)",
		Keywords: []string{
			"ai agent task", "agent task", "call agent", "call agent microflow",
			"agent", "llm", "genai", "AIAgentTaskActivity",
		},
		// Measured on mxbuild 11.13.0 against the same activity written as a call
		// microflow: the only difference in what builds is CE1590 for a microflow
		// with no parameters.
		Syntax: "CALL AGENT MICROFLOW Module.MF [AS <name>] [COMMENT '<text>']\n" +
			"  [WITH (<Param> = '<expression>', ...)]\n" +
			"  [OUTCOMES <true|false|'Module.Enum.Value'|''> -> { <activities> } ...]\n" +
			"  [BOUNDARY EVENT ...];\n\n" +
			"-- The same statement as CALL MICROFLOW, stored as an AI agent task. The microflow\n" +
			"-- is where the agent is invoked; it must take at least one parameter (else CE1590),\n" +
			"-- usually the workflow's context object. Return Boolean or an enumeration to\n" +
			"-- branch on the agent's answer with OUTCOMES. Needs Mendix 11.9+.",
		Example: "CREATE MICROFLOW HR.ACT_ClassifyRequest ($Request: HR.LeaveRequest)\n" +
			"RETURNS Boolean AS $Urgent\nBEGIN\n  -- call the agent here\n  RETURN false;\nEND;\n\n" +
			"CALL AGENT MICROFLOW HR.ACT_ClassifyRequest AS aiAgentTask1 COMMENT 'Classify the request'\n" +
			"  WITH (Request = '$WorkflowContext')\n" +
			"  OUTCOMES true -> { USER TASK Expedite 'Expedite' PAGE HR.TaskPage OUTCOMES 'Done' { }; }\n" +
			"           false -> { };",
		SeeAlso: []string{"workflow.call-microflow", "agents"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.call-workflow",
		Summary: "Call a sub-workflow from within a workflow",
		Keywords: []string{
			"call workflow", "sub-workflow", "nested workflow",
		},
		Syntax:  "CALL WORKFLOW Module.WF [AS <name>] [COMMENT '<text>']\n  [WITH (<Param> = '<expression>', ...)];",
		Example: "CALL WORKFLOW HR.SubApproval COMMENT 'Delegate to sub-process';\n\n-- Parameter values are quoted:\nCALL WORKFLOW HR.SubApproval AS callWf1\n  WITH (Request = '$WorkflowContext');",
		SeeAlso: []string{"workflow.create", "workflow.call-microflow"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.jump-to",
		Summary: "Jump to another activity — and the activity names it resolves against",
		Keywords: []string{
			"jump", "jump to", "goto", "loop back", "activity name",
		},
		// Mendix stores JumpToActivity.TargetActivity as an activity NAME, not a
		// pointer, so the jump is only as good as the name. Studio Pro names
		// activities by type and ordinal regardless of caption (decision1,
		// split1, callMicroflow1); mxcli derives a name when none is given, which
		// is why an explicit one matters when reproducing a stored workflow.
		Syntax: "JUMP TO <activity-name> [COMMENT '<text>'];\n\n" +
			"-- name the target so the jump resolves:\n" +
			"DECISION <name> ['<caption>'] ...\nPARALLEL SPLIT <name> ...\n" +
			"WAIT FOR TIMER <name> ...\nWAIT FOR NOTIFICATION <name>\n" +
			"CALL MICROFLOW Module.MF AS <name>\nCALL WORKFLOW Module.WF AS <name>",
		Example: "DECISION decision1 '$WorkflowContext/Total > 1000'\n" +
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
		Syntax: "end workflow [comment '<caption>'];\n\n" +
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
			"-- `comment` sets the End's caption on the canvas. An End is not a jump target.",
		Example: "USER TASK Review 'Review the request'\n" +
			"  PAGE HR.ReviewPage\n" +
			"  OUTCOMES\n" +
			"    'Approve' { }\n" +
			"    'Reject' {\n" +
			"      CALL MICROFLOW HR.ACT_NotifyRejected;\n" +
			"      END WORKFLOW COMMENT 'Rejected';\n" +
			"    }\n" +
			"  BOUNDARY EVENT INTERRUPTING TIMER 'addDays([%CurrentDateTime%], 5)' {\n" +
			"    END WORKFLOW COMMENT 'Expired';\n" +
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
		Path:    "workflow.catalog",
		Summary: "Query workflow metadata via catalog tables",
		Keywords: []string{
			"catalog", "query workflows", "workflow metadata",
			"cross-reference", "callers", "callees",
		},
		Syntax:  "REFRESH CATALOG FULL;\nSELECT * FROM CATALOG.WORKFLOWS;\nSHOW CALLERS OF Module.WorkflowName;\nSHOW REFERENCES TO Module.WorkflowName;",
		Example: "REFRESH CATALOG FULL;\nSELECT QualifiedName, ActivityCount, UserTaskCount\n  FROM CATALOG.WORKFLOWS WHERE UserTaskCount > 0;",
		SeeAlso: []string{"workflow.show"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.boundary-event",
		Summary: "Attach boundary events (timer) to user tasks for timeouts",
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
		Syntax:     "-- inline, as a clause of a USER TASK (after OUTCOMES):\nBOUNDARY EVENT (INTERRUPTING | NON INTERRUPTING) TIMER '<datetime-expression>' { <activities> }\nBOUNDARY EVENT (INTERRUPTING | NON INTERRUPTING) NOTIFICATION <name> ['<caption>'] { <activities> }\n\n-- A notification boundary event (Mendix 11.11+) is triggered by `notify workflow`\n-- targeting its name, so the name is unique in the workflow. ALTER cannot insert\n-- one yet; restate the workflow instead.\n\n-- or add a timer to an existing task:\nALTER WORKFLOW <wf> INSERT BOUNDARY EVENT ON <task> (INTERRUPTING | NON INTERRUPTING) TIMER '<datetime-expression>' { <activities> }\n\n-- Name the kind: a bare TIMER is refused on Mendix 11 (MDL-WF07) — it writes a\n-- type the runtime cannot load. The delay is an expression that yields a\n-- DateTime, e.g. 'addDays([%CurrentDateTime%], 3)'. mxcli ends every boundary\n-- path with Mendix's end-of-path marker, so a path may end in a call; use\n-- JUMP TO to return to the task instead.",
		Example:    "user task ReviewTask 'Review'\n  page Module.WF_Review\n  outcomes 'Done' { }\n  boundary event interrupting timer 'addDays([%CurrentDateTime%], 3)' {\n    call microflow Module.WF_Escalate;\n  };",
		MinVersion: "10.6.0",
		SeeAlso:    []string{"workflow.user-task", "workflow.notification", "workflow.event-subprocess"},
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
			"  ON (INTERRUPTING | NON INTERRUPTING) TIMER '<first-execution-time>' [AS <start>] [COMMENT '<start caption>']\n" +
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
			"    on non interrupting timer 'addDays([%CurrentDateTime%], 1)' as espReminderStart {\n" +
			"    call microflow HR.ACT_Remind;\n" +
			"  };\n" +
			"end workflow;",
		MinVersion: "11.8.0",
		SeeAlso:    []string{"workflow.notification", "workflow.boundary-event", "workflow.create"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.notification",
		Summary: "Notification activity — an intermediate notification event on a flow",
		Keywords: []string{
			"notification", "notification activity", "intermediate event", "notify",
		},
		Syntax: "NOTIFICATION [<name>] [COMMENT '<caption>'];\n\n" +
			"-- The point a `notify workflow` action reaches (Workflows$NotificationActivity,\n" +
			"-- Mendix 11.11+). `wait for notification` is the older activity for the same\n" +
			"-- purpose and works on every version.",
		Example:    "notification DocumentsReceived comment 'Documents received';",
		MinVersion: "11.11.0",
		SeeAlso:    []string{"workflow.event-subprocess", "workflow.boundary-event", "workflow.notify"},
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
		Example:    "$Cancelled = notify workflow $Workflow target HR.Leave.espCancelStart;",
		MinVersion: "11.7.0",
		SeeAlso:    []string{"workflow.event-subprocess", "workflow.notification", "workflow.boundary-event"},
	})

	Register(SyntaxFeature{
		Path:    "workflow.alter",
		Summary: "Modify an existing workflow — change properties, add/remove activities",
		Keywords: []string{
			"alter workflow", "modify workflow", "update workflow",
			"add activity", "drop activity", "replace activity",
		},
		// SET properties are keyword-led phrases, not `name = value` assignments:
		// `SET DUE DATE '<expr>'`, `SET DISPLAY '<text>'`, `SET OVERVIEW PAGE
		// Module.Page`. The `= ` this entry used to show does not parse.
		// INSERT names the anchor first and the activity second — INSERT AFTER
		// <name> <activity> — and there is no BEFORE. DROP and REPLACE take the
		// ACTIVITY keyword. This entry previously showed the operand order
		// reversed, advertised a BEFORE that does not exist, and omitted
		// ACTIVITY, so none of it parsed.
		//
		// The four INSERT ops that add to an activity's outcome list each write
		// ONE outcome type, and the list is typed per activity kind — INSERT
		// OUTCOME only on a user task, INSERT PATH only on a parallel split,
		// INSERT CONDITION only on a decision or call microflow. Aiming one at
		// the wrong kind used to produce a project Mendix could not LOAD
		// (ako/mxcli#415); it is refused now, but the entry documented only two
		// of the ops, which is how an author reached for the wrong one.
		Syntax:  "ALTER WORKFLOW Module.Name SET DISPLAY '<text>';\nALTER WORKFLOW Module.Name SET DUE DATE '<expression>';\nALTER WORKFLOW Module.Name SET OVERVIEW PAGE Module.Page;\nALTER WORKFLOW Module.Name SET ACTIVITY <name> <property>;\nALTER WORKFLOW Module.Name INSERT AFTER <name> <activity>;\nALTER WORKFLOW Module.Name DROP ACTIVITY <name>;\nALTER WORKFLOW Module.Name REPLACE ACTIVITY <name> WITH <activity>;\nALTER WORKFLOW Module.Name INSERT OUTCOME '<name>' ON <user-task> { <activities> };\nALTER WORKFLOW Module.Name DROP OUTCOME '<name>' ON <activity>;\nALTER WORKFLOW Module.Name INSERT CONDITION '<Module.Enum.Value>' ON <decision|call-microflow> { <activities> };\nALTER WORKFLOW Module.Name INSERT PATH ON <parallel-split> { <activities> };\nALTER WORKFLOW Module.Name INSERT BOUNDARY EVENT ON <activity> (INTERRUPTING | NON INTERRUPTING) TIMER '<datetime-expression>' { <activities> };",
		Example: "ALTER WORKFLOW HR.LeaveApproval SET DUE DATE 'addDays([%CurrentDateTime%], 7)';\nALTER WORKFLOW HR.LeaveApproval INSERT AFTER ReviewTask\n  CALL MICROFLOW HR.NotifyHR;\nALTER WORKFLOW HR.LeaveApproval DROP ACTIVITY ObsoleteStep;\n\n-- The INSERT op has to match the activity kind: an outcome list is typed,\n-- and the wrong one is refused (it would leave a project Mendix cannot open).\nALTER WORKFLOW HR.LeaveApproval INSERT OUTCOME 'Rejected' ON ReviewTask { };\nALTER WORKFLOW HR.LeaveApproval INSERT CONDITION 'HR.Status.Urgent' ON Triage { };\nALTER WORKFLOW HR.LeaveApproval INSERT PATH ON NotifyAll { };",
		SeeAlso: []string{"workflow.create", "workflow.drop"},
	})
}
