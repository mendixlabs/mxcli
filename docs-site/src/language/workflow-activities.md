# Activity Types

Workflow activities are the building blocks of a workflow definition. Each activity type serves a different purpose, from waiting for user input to calling microflows or branching execution.

## User Task

A user task pauses the workflow until a user completes it. Each outcome resumes the workflow down a different path.

```sql
USER TASK <name> '<caption>'
  [PAGE <Module>.<Page>]
  [TARGETING MICROFLOW <Module>.<Microflow>]
  [ON CREATED MICROFLOW <Module>.<Microflow>]
  OUTCOMES '<outcome>' { <activities> } ['<outcome>' { <activities> }] ...;
```

| Element | Description |
|---------|-------------|
| `<name>` | Internal activity name |
| `<caption>` | Display label shown to users |
| `PAGE` | The page opened when the user acts on the task |
| `TARGETING MICROFLOW` | Microflow that determines which users see the task |
| `ON CREATED MICROFLOW` | Microflow run when the task is created — for example to assign it. It takes exactly `System.WorkflowUserTask` and the workflow's context entity, in either order (else CE6683), and returns nothing (else CE5012) |
| `OUTCOMES` | Named outcomes, each with a block of follow-up activities |

Example:

```sql
USER TASK ReviewTask 'Review the request'
  PAGE Approval.ReviewPage
  TARGETING MICROFLOW Approval.ACT_GetReviewers
  ON CREATED MICROFLOW Approval.ACT_AssignReviewer
  OUTCOMES 'Approve' {
    CALL MICROFLOW Approval.ACT_Approve;
  } 'Reject' {
    CALL MICROFLOW Approval.ACT_Reject;
    END WORKFLOW;
  };
```

## Multi-User Task

A multi-user task gives the same task to several users and combines their
outcomes into one. Three clauses, before `OUTCOMES`, say how:

```sql
MULTI USER TASK <name> '<caption>'
  PAGE <Module>.<Page>
  [PARTICIPANTS ALL | <n> | <n> PERCENT]
  [DECIDE BY <rule>]
  [AWAIT ALL USERS]
  OUTCOMES '<outcome>' { <activities> } ...;
```

| Rule | Completes when | Needs |
|------|----------------|-------|
| `CONSENSUS` | everyone chose the same outcome | `FALLBACK '<outcome>'` |
| `MAJORITY MORE THAN HALF` | more than half chose one outcome | `FALLBACK '<outcome>'` |
| `MAJORITY MOST CHOSEN` | one outcome was chosen most | `FALLBACK '<outcome>'` |
| `THRESHOLD <n> PERCENT` / `<n> VOTES` | an outcome reaches the threshold | `FALLBACK '<outcome>'` |
| `VETO '<outcome>'` | anyone chooses the veto outcome | — |
| `MICROFLOW <Module>.<Name>` | the microflow returns the outcome (String) | — |

Example:

```sql
MULTI USER TASK Vote 'Vote on the request'
  PAGE HR.VotePage
  PARTICIPANTS 80 PERCENT
  DECIDE BY THRESHOLD 60 PERCENT FALLBACK 'Reject'
  AWAIT ALL USERS
  OUTCOMES 'Approve' { } 'Reject' { };
```

Omitted, the task needs all participants, decides by consensus falling back to
its first outcome, and does not wait for everyone. The fallback is required for
consensus, majority and threshold, and the build does not check threshold or
participant numbers against each other — `THRESHOLD 5 VOTES` with three users
builds and never completes.

## Call Microflow

Execute a microflow as part of the workflow. Optionally specify a comment and outcomes:

```sql
CALL MICROFLOW <Module>.<Name> [COMMENT '<text>']
  [OUTCOMES '<outcome>' { <activities> } ...];
```

Example:

```sql
CALL MICROFLOW HR.ACT_SendNotification COMMENT 'Notify the applicant';
```

## AI Agent Task

A step that runs an AI agent (Mendix 11.9 or later). It is written like `CALL
MICROFLOW` with `AGENT` added, and takes the same name, comment, parameter
mappings, outcomes and boundary events. The microflow is where the agent is
invoked — build agents with the Studio Pro Agent Editor, or `CREATE AGENT`.

```sql
CALL AGENT MICROFLOW <Module>.<Name> [AS <name>] [COMMENT '<text>']
  [WITH (<Param> = '<expression>', ...)]
  [OUTCOMES <true|false|'Module.Enum.Value'|''> -> { <activities> } ...];
```

Example — branch on the agent's answer:

```sql
CALL AGENT MICROFLOW HR.ACT_ClassifyRequest AS aiAgentTask1 COMMENT 'Classify the request'
  WITH (Request = '$WorkflowContext')
  OUTCOMES true -> {
    USER TASK Expedite 'Expedite the request' PAGE HR.TaskPage OUTCOMES 'Done' { };
  } false -> { };
```

The microflow must take at least one parameter — usually the workflow's context
object — or the build fails with CE1590. Return Boolean or an enumeration to
branch with `OUTCOMES`; return nothing for a single path.

## Call Workflow

Start a sub-workflow:

```sql
CALL WORKFLOW <Module>.<Name> [COMMENT '<text>'];
```

Example:

```sql
CALL WORKFLOW HR.BackgroundCheck COMMENT 'Run background check sub-process';
```

## Decision

Branch the workflow based on a condition. Each outcome contains a block of activities:

```sql
DECISION ['<caption>']
  OUTCOMES '<outcome>' { <activities> } ['<outcome>' { <activities> }] ...;
```

Example:

```sql
DECISION 'Order value over $1000?'
  OUTCOMES 'Yes' {
    USER TASK ManagerApproval 'Manager must approve'
      PAGE Shop.ApprovalPage
      OUTCOMES 'Approved' { } 'Rejected' { END; };
  } 'No' { };
```

## Parallel Split

Execute multiple paths concurrently. The workflow continues after all paths complete:

```sql
PARALLEL SPLIT
  PATH 1 { <activities> }
  PATH 2 { <activities> }
  [PATH 3 { <activities> }] ...;
```

Example:

```sql
PARALLEL SPLIT
  PATH 1 {
    USER TASK LegalReview 'Legal review'
      PAGE Legal.ReviewPage
      OUTCOMES 'Approved' { };
  }
  PATH 2 {
    USER TASK FinanceReview 'Finance review'
      PAGE Finance.ReviewPage
      OUTCOMES 'Approved' { };
  };
```

## Jump To

Jump to a named activity elsewhere in the workflow (creates a loop or skip):

```sql
JUMP TO <activity-name>;
```

Example:

```sql
JUMP TO ReviewTask;
```

## Wait for Timer

Pause the workflow until a timer expression evaluates:

```sql
WAIT FOR TIMER ['<expression>'];
```

Example:

```sql
WAIT FOR TIMER 'addDays([%CurrentDateTime%], 3)';
```

## Wait for Notification

Pause the workflow until an external notification resumes it:

```sql
WAIT FOR NOTIFICATION;
```

## Notification

An intermediate notification event (Mendix 11.11+): the point a `NOTIFY WORKFLOW` action targets by name.

```sql
NOTIFICATION [<name>] [COMMENT '<caption>'];
```

A **notification boundary event** attaches the same trigger to a user task, call microflow or wait. It takes a name instead of a timer delay:

```sql
USER TASK Review 'Review'
  PAGE HR.ReviewPage
  OUTCOMES 'Approve' { } 'Reject' { }
  BOUNDARY EVENT INTERRUPTING NOTIFICATION Withdrawn 'Request withdrawn' {
    END WORKFLOW;
  };
```

An activity may carry only one interrupting boundary event (CE6697). `ALTER WORKFLOW … INSERT BOUNDARY EVENT` cannot add a notification boundary event yet.

A microflow reaches any of these with `NOTIFY WORKFLOW`, naming the element:

```sql
$Notified = NOTIFY WORKFLOW $Workflow TARGET HR.Leave.Withdrawn;
```

The target is required — a notify without one fails the build (CE0166). It may name a notification activity, a notification boundary event, a wait for notification, or the start of a notification-started event sub-process; mxcli works out which, and refuses anything a notification cannot reach.

## Event Sub-Processes

A flow outside the main flow, started by its own start event while the workflow runs. Written after the main body, before `END WORKFLOW`:

```sql
EVENT SUBPROCESS <name> ['<caption>']
  ON (INTERRUPTING | NON INTERRUPTING) NOTIFICATION [<start>] ['<start caption>']
  { <activities> };

EVENT SUBPROCESS <name> ['<caption>']
  ON (INTERRUPTING | NON INTERRUPTING) TIMER '<first-execution-time>' [AS <start>] [COMMENT '<start caption>']
  { <activities> };
```

Example:

```sql
EVENT SUBPROCESS ESP_Cancel 'Cancel request'
  ON INTERRUPTING NOTIFICATION espCancelStart 'Cancel received' {
  CALL MICROFLOW HR.ACT_LogCancel;
};
```

- **Interrupting** cancels every active path before the sub-process runs; **non-interrupting** runs alongside the main flow.
- The body's End is implicit, as in the main flow. A body that already ends (in `END WORKFLOW`, a `JUMP TO`, or branches that all end) gets none.
- A `JUMP TO` must stay inside its own sub-process (CE6682).
- A timer start needs its expression (CE0126).
- Versions: notification starts need Mendix 11.8+, timer starts 11.13+.

## End

Terminate the current workflow path:

```sql
END;
```

Typically used inside an outcome block to stop the workflow after a rejection or cancellation.

## Summary Table

| Activity | Purpose | Pauses Workflow? |
|----------|---------|-----------------|
| `USER TASK` | Wait for human action | Yes |
| `CALL MICROFLOW` | Execute server logic | No |
| `CALL AGENT MICROFLOW` | Run an AI agent step (11.9+) | No |
| `CALL WORKFLOW` | Start sub-workflow | Depends on sub-workflow |
| `DECISION` | Branch on condition | No |
| `PARALLEL SPLIT` | Concurrent execution | Yes (waits for all paths) |
| `JUMP TO` | Go to named activity | No |
| `WAIT FOR TIMER` | Delay execution | Yes |
| `WAIT FOR NOTIFICATION` | Wait for external signal | Yes |
| `NOTIFICATION` | Intermediate notification event (11.11+) | Yes |
| `EVENT SUBPROCESS` | Flow started by a notification or timer (11.8+) | No (runs beside or replaces the main flow) |
| `END` | Terminate path | N/A |

## See Also

- [Workflows](./workflows.md) -- overview and when to use workflows
- [Workflow Structure](./workflow-structure.md) -- CREATE WORKFLOW syntax
- [Workflow vs Microflow](./workflow-vs-microflow.md) -- choosing between workflows and microflows
