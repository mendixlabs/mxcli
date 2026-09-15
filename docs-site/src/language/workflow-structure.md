# Workflow Structure

A workflow definition consists of a context parameter, an optional overview page, and a sequence of activities. Activities execute in order unless control flow elements (decisions, parallel splits, jumps) alter the sequence.

## CREATE WORKFLOW

```sql
CREATE [OR MODIFY] WORKFLOW <Module>.<Name>
  PARAMETER $<Name>: <Module>.<Entity>
  [OVERVIEW PAGE <Module>.<Page>]
  [ON WORKFLOW EVENTS (<type>, ...) MICROFLOW <Module>.<Microflow> [AS '<description>']] ...
  [ON ANY WORKFLOW EVENT MICROFLOW <Module>.<Microflow> [AS '<description>']] ...
BEGIN
  <activities>
END WORKFLOW;
```

| Element | Description |
|---------|-------------|
| `PARAMETER` | The workflow context object, passed when the workflow is started |
| `OVERVIEW PAGE` | Optional page shown in the workflow admin dashboard |
| `ON WORKFLOW EVENTS` | A microflow run for each listed workflow event; repeatable. See [Workflow Event Handlers](#workflow-event-handlers) |
| Activities | A sequence of user tasks, microflow calls, decisions, etc. |

## Full Example

```sql
CREATE WORKFLOW HR.OnboardEmployee
  PARAMETER $Context: HR.OnboardingRequest
  OVERVIEW PAGE HR.OnboardingOverview
BEGIN
  -- Parallel: IT setup and HR paperwork happen simultaneously
  PARALLEL SPLIT
    PATH 1 {
      USER TASK SetupLaptop 'Set up laptop and accounts'
        PAGE HR.IT_SetupPage
        OUTCOMES 'Done' { };
    }
    PATH 2 {
      USER TASK SignDocuments 'Sign employment documents'
        PAGE HR.DocumentSignPage
        OUTCOMES 'Signed' { };
    };

  -- After both paths complete, manager reviews
  DECISION 'Manager approval required?'
    OUTCOMES 'Yes' {
      USER TASK ManagerReview 'Review onboarding'
        PAGE HR.ManagerReviewPage
        OUTCOMES 'Approve' { } 'Reject' {
          CALL MICROFLOW HR.ACT_RejectOnboarding;
          END WORKFLOW;
        };
    } 'No' { };

  CALL MICROFLOW HR.ACT_CompleteOnboarding;
END WORKFLOW;
```

## Workflow Event Handlers

A handler runs a microflow whenever one of the listed events happens — a task
starting or ending, the workflow completing, failing or being aborted. Use one to
audit a process or react to it without adding activities to the flow.

```sql
CREATE MICROFLOW HR.ACT_AuditTask (
  $WorkflowEvent: System.WorkflowEvent,
  $WorkflowRecord: System.WorkflowRecord,
  $WorkflowActivityRecord: System.WorkflowActivityRecord
) BEGIN END;

CREATE WORKFLOW HR.LeaveApproval
  PARAMETER $Request: HR.LeaveRequest
  ON WORKFLOW EVENTS (UserTaskStarted, UserTaskEnded) MICROFLOW HR.ACT_AuditTask AS 'Task audit'
  ON ANY WORKFLOW EVENT MICROFLOW HR.ACT_LogEvent AS 'OnAnyEvent'
BEGIN
  USER TASK Review 'Review the request'
    PAGE HR.ReviewPage
    OUTCOMES 'Approve' { } 'Reject' { };
END WORKFLOW;
```

- The handler microflow takes exactly `System.WorkflowEvent`,
  `System.WorkflowRecord` and `System.WorkflowActivityRecord`, in any order.
  Anything else fails the build with CE6691.
- **The build does not check event type names**: a handler listing a name that is
  not an event type builds cleanly and never runs for it. `mxcli check` refuses an
  unknown name (MDL-WF12) and, with a project, a type that project's Mendix
  version does not have. `mxcli syntax workflow.event-handlers` lists the types.
- `ON ANY WORKFLOW EVENT` stores every event type the project's Mendix version
  has — Studio Pro stores the list, not a flag — so it needs Mendix 11.6 or later.
  On older projects, name the types.
- `AS '...'` is the handler's description, which is how Studio Pro lists it.

## DROP WORKFLOW

```sql
DROP WORKFLOW HR.OnboardEmployee;
```

## Workflow Access

> **Not supported.** Mendix workflows do not have document-level `AllowedModuleRoles` (unlike microflows and pages). Workflow access is controlled through the microflow that triggers the workflow and UserTask targeting.

## See Also

- [Workflows](./workflows.md) -- overview and when to use workflows
- [Activity Types](./workflow-activities.md) -- details on each activity type
- [Workflow vs Microflow](./workflow-vs-microflow.md) -- choosing the right construct
