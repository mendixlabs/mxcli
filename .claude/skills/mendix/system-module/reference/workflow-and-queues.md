# Workflow engine, task queues and scheduled events

Supporting reference for [system-module](../SKILL.md).

## 5. Workflow Engine

Mendix workflows use a rich set of System entities. These are managed by the runtime but can be queried and displayed in pages.

### System.Workflow

A running workflow instance.

| Attribute | Type | Description |
|-----------|------|-------------|
| Name | String | Workflow instance name |
| Description | String | Workflow description |
| StartTime | DateTime | When the workflow started |
| EndTime | DateTime | When the workflow ended |
| DueDate | DateTime | Workflow deadline |
| CanBeRestarted | Boolean | Whether restart is allowed |
| CanBeContinued | Boolean | Whether continue is allowed |
| CanApplyJumpTo | Boolean | Whether jump-to is allowed |
| State | Enum (WorkflowState) | Current state |
| Reason | String | Reason for current state (e.g., abort reason) |

| Association | Target | Type | Description |
|-------------|--------|------|-------------|
| Workflow_WorkflowDefinition | System.WorkflowDefinition | Many-to-One | The workflow template |
| Workflow_ParentWorkflow | System.Workflow | Many-to-One | Parent (for sub-workflows) |

### System.WorkflowDefinition

A workflow template as defined in the model.

| Attribute | Type | Description |
|-----------|------|-------------|
| Name | String | Definition name |
| Title | String | Display title |
| IsObsolete | Boolean | Whether superseded by newer version |
| IsLocked | Boolean | Whether locked for editing |

### System.WorkflowUserTask

An active user task waiting for completion.

| Attribute | Type | Description |
|-----------|------|-------------|
| Name | String | Task name |
| Description | String | Task description |
| StartTime | DateTime | When the task became active |
| DueDate | DateTime | Task deadline |
| EndTime | DateTime | When the task was completed |
| Outcome | String | Selected outcome |
| State | Enum (WorkflowUserTaskState) | Task state |
| CompletionType | Enum (WorkflowUserTaskCompletionType) | How consensus is determined |

| Association | Target | Type | Description |
|-------------|--------|------|-------------|
| WorkflowUserTask_TargetUsers | System.User | Many-to-Many | Eligible users |
| WorkflowUserTask_Assignees | System.User | Many-to-Many | Actually assigned users |
| WorkflowUserTask_Workflow | System.Workflow | Many-to-One | Parent workflow |
| WorkflowUserTask_WorkflowUserTaskDefinition | System.WorkflowUserTaskDefinition | Many-to-One | Task template |
| WorkflowUserTask_TargetGroups | System.WorkflowGroup | Many-to-Many | Eligible user groups |

### System.WorkflowUserTaskDefinition

| Attribute | Type | Description |
|-----------|------|-------------|
| Name | String | Task definition name |
| IsObsolete | Boolean | Whether superseded |

| Association | Target | Type |
|-------------|--------|------|
| WorkflowUserTaskDefinition_WorkflowDefinition | System.WorkflowDefinition | Many-to-One |

### System.WorkflowGroup

A named group of users for task assignment.

| Attribute | Type | Description |
|-----------|------|-------------|
| Name | String | Group name |
| Description | String | Group description |

| Association | Target | Type |
|-------------|--------|------|
| WorkflowGroup_User | System.User | Many-to-Many |

### System.WorkflowUserTaskOutcome

Records who selected which outcome on an active user task.

| Attribute | Type | Description |
|-----------|------|-------------|
| Outcome | String | Selected outcome value |
| Time | DateTime | When the outcome was selected |

| Association | Target | Type |
|-------------|--------|------|
| WorkflowUserTaskOutcome_WorkflowUserTask | System.WorkflowUserTask | Many-to-One |
| WorkflowUserTaskOutcome_User | System.User | Many-to-One |

### System.WorkflowEvent

Audit events during workflow execution.

| Attribute | Type | Description |
|-----------|------|-------------|
| EventTime | DateTime | When the event occurred |
| EventType | Enum (WorkflowEventType) | Type of event |

| Association | Target | Type |
|-------------|--------|------|
| WorkflowEvent_Initiator | System.User | Many-to-One |

### System.WorkflowRecord

Snapshot/audit record of a workflow instance.

| Attribute | Type | Description |
|-----------|------|-------------|
| WorkflowKey | String | Workflow instance key |
| Name | String | Workflow name |
| Description | String | Workflow description |
| State | Enum (WorkflowState) | State at time of record |
| StartTime | DateTime | Workflow start time |
| DueDate | DateTime | Workflow due date |
| EndTime | DateTime | Workflow end time |
| Reason | String | State reason |

| Association | Target | Type |
|-------------|--------|------|
| WorkflowRecord_Workflow | System.Workflow | Many-to-One |
| WorkflowRecord_Owner | System.User | Many-to-One |
| WorkflowRecord_WorkflowDefinition | System.WorkflowDefinition | Many-to-One |

### System.WorkflowActivityRecord

Detailed audit of each workflow activity execution.

| Attribute | Type | Description |
|-----------|------|-------------|
| ModelGUID | String | Activity GUID in model |
| ActivityKey | String | Unique activity key |
| PreviousActivityKey | String | Key of preceding activity |
| ActivityType | Enum (WorkflowActivityType) | Type of activity |
| Caption | String | Activity caption |
| State | Enum (WorkflowActivityExecutionState) | Execution state |
| StartTime | DateTime | When activity started |
| EndTime | DateTime | When activity ended |
| Outcome | String | Activity outcome |
| MicroflowName | String | Called microflow (if applicable) |
| TaskName | String | User task name (if applicable) |
| TaskDescription | String | User task description |
| TaskDueDate | DateTime | User task due date |
| TaskCompletionType | Enum (WorkflowUserTaskCompletionType) | How task consensus works |
| TaskRequiredUsers | Integer | Number of required users |
| TaskKey | String | User task key |
| Reason | String | State reason |

| Association | Target | Type |
|-------------|--------|------|
| WorkflowActivityRecord_PreviousActivity | System.WorkflowActivityRecord | Many-to-One |
| WorkflowActivityRecord_Actor | System.User | Many-to-One |
| WorkflowActivityRecord_SubWorkflow | System.WorkflowRecord | Many-to-One |
| WorkflowActivityRecord_UserTask | System.WorkflowUserTask | Many-to-One |
| WorkflowActivityRecord_WorkflowUserTaskDefinition | System.WorkflowUserTaskDefinition | Many-to-One |
| WorkflowActivityRecord_TaskTargetedUsers | System.User | Many-to-Many |
| WorkflowActivityRecord_TaskAssignedUsers | System.User | Many-to-Many |
| WorkflowActivityRecord_TaskTargetedGroups | System.WorkflowGroup | Many-to-Many |
| WorkflowActivityRecord_WorkflowSubProcess | System.WorkflowSubProcess | Many-to-One (11.8+) |
| WorkflowActivityRecord_WorkflowSubProcessDefinition | System.WorkflowSubProcessDefinition | Many-to-One (11.8+) |

### System.WorkflowSubProcess

A running or finished sub-process of a workflow. Mendix 11.8 and later
only: in an older project every name in this section is CE1613 "no longer exists".

| Attribute | Type | Description |
|-----------|------|-------------|
| Caption | String(200) | Sub-process caption |
| StartTime | DateTime | When it started |
| EndTime | DateTime | When it ended |
| State | Enum (WorkflowSubProcessState) | Execution state |
| Reason | String | State reason |

| Association | Target | Type |
|-------------|--------|------|
| WorkflowSubProcess_Workflow | System.Workflow | Many-to-One |
| WorkflowSubProcess_WorkflowSubProcessDefinition | System.WorkflowSubProcessDefinition | Many-to-One |

`WorkflowUserTask_WorkflowSubProcess`, `WorkflowEndedUserTask_WorkflowSubProcess`
and `WorkflowCurrentActivity_WorkflowSubProcess` point at it from a task or activity.
Read its caption from an activity record with
`$WorkflowActivityRecord/System.WorkflowActivityRecord_WorkflowSubProcess/Caption`.

### System.WorkflowSubProcessDefinition

The definition of a sub-process (Mendix 11.8 and later).

| Attribute | Type | Description |
|-----------|------|-------------|
| Caption | String(200) | Definition caption |
| IsObsolete | Boolean | Removed from the model |
| ModelGUID | String(36) | Sub-process GUID in the model |

| Association | Target | Type |
|-------------|--------|------|
| WorkflowSubProcessDefinition_WorkflowDefinition | System.WorkflowDefinition | Many-to-One |

`WorkflowUserTaskDefinition_WorkflowSubProcessDefinition` points at it from a user
task definition.

### System.WorkflowActivityDetails

Metadata about a workflow activity (used for jump-to navigation).

| Attribute | Type | Description |
|-----------|------|-------------|
| ActivityId | String | Activity identifier |
| ActivityCaption | String | Display caption |
| ActivityType | Enum (WorkflowActivityType) | Activity type |
| ExistsInCurrentVersion | Boolean | Whether activity exists in current model version |

### System.WorkflowCurrentActivity

Current activity state within a workflow (used for jump-to).

| Attribute | Type | Description |
|-----------|------|-------------|
| Action | Enum (WorkflowCurrentActivityAction) | DoNothing or JumpTo |

| Association | Target | Type |
|-------------|--------|------|
| WorkflowCurrentActivity_ActivityDetails | System.WorkflowActivityDetails | Many-to-One |
| WorkflowCurrentActivity_ApplicableTargets | System.WorkflowActivityDetails | Many-to-Many |
| WorkflowCurrentActivity_JumpToTarget | System.WorkflowActivityDetails | Many-to-One |

### System.WorkflowJumpToDetails

Details for jump-to operations.

| Attribute | Type | Description |
|-----------|------|-------------|
| Error | String | Error message if jump-to failed |

| Association | Target | Type |
|-------------|--------|------|
| WorkflowJumpToDetails_Workflow | System.Workflow | Many-to-One |
| WorkflowJumpToDetails_CurrentActivities | System.WorkflowCurrentActivity | Many-to-Many |

### System.WorkflowEndedUserTask

Completed/archived user tasks.

| Attribute | Type | Description |
|-----------|------|-------------|
| Name | String | Task name |
| Description | String | Task description |
| StartTime | DateTime | When task started |
| DueDate | DateTime | Task deadline |
| EndTime | DateTime | When task ended |
| Outcome | String | Final outcome |
| State | Enum (WorkflowUserTaskState) | Final state |
| CompletionType | Enum (WorkflowUserTaskCompletionType) | How consensus was determined |
| UserTaskKey | String | Unique task key |

| Association | Target | Type |
|-------------|--------|------|
| WorkflowEndedUserTask_Assignees | System.User | Many-to-Many |
| WorkflowEndedUserTask_TargetUsers | System.User | Many-to-Many |
| WorkflowEndedUserTask_WorkflowUserTaskDefinition | System.WorkflowUserTaskDefinition | Many-to-One |
| WorkflowEndedUserTask_Workflow | System.Workflow | Many-to-One |
| WorkflowEndedUserTask_TargetGroups | System.WorkflowGroup | Many-to-Many |

### System.WorkflowEndedUserTaskOutcome

Individual outcome votes on ended user tasks.

| Attribute | Type | Description |
|-----------|------|-------------|
| Outcome | String | Selected outcome |
| Time | DateTime | When outcome was selected |

| Association | Target | Type |
|-------------|--------|------|
| WorkflowEndedUserTaskOutcome_User | System.User | Many-to-One |
| WorkflowEndedUserTaskOutcome_WorkflowEndedUserTask | System.WorkflowEndedUserTask | Many-to-One |

---
## 6. Task Queues & Scheduled Events

### System.QueuedTask

A task waiting to execute or currently running in a task queue.

| Attribute | Type | Description |
|-----------|------|-------------|
| Sequence | Long | Task sequence number |
| Status | Enum (QueueTaskStatus) | Current status |
| QueueId | String | Queue identifier |
| QueueName | String | Queue display name |
| ContextType | Enum (ContextType) | Execution context: System, User, Anonymous, ScheduledEvent |
| ContextData | String | Serialized context |
| MicroflowName | String | Microflow to execute |
| UserActionName | String | Java action to execute |
| Arguments | String | Serialized arguments |
| XASId | String | Cluster node identifier |
| ThreadId | Long | Execution thread ID |
| Created | DateTime | When task was queued |
| StartAt | DateTime | Scheduled start time |
| Started | DateTime | Actual start time |
| Retried | Long | Number of retry attempts |
| Retry | String | Retry configuration |
| ScheduledEventName | String | Associated scheduled event name |

### System.ProcessedQueueTask

Completed tasks (audit trail). Same attributes as `QueuedTask` plus:

| Attribute | Type | Description |
|-----------|------|-------------|
| Finished | DateTime | When task finished |
| Duration | Long | Execution duration in milliseconds |
| ErrorMessage | String | Error message if task failed |

### System.ScheduledEventInformation

Runtime information about scheduled events.

| Attribute | Type | Description |
|-----------|------|-------------|
| Name | String | Scheduled event name |
| Description | String | Event description |
| StartTime | DateTime | Last start time |
| EndTime | DateTime | Last end time |
| Status | Enum (EventStatus) | Running, Completed, Error, Stopped |

| Association | Target | Type |
|-------------|--------|------|
| ScheduledEventInformation_XASInstance | System.XASInstance | Many-to-One |

### System.XASInstance

Cluster node information (for multi-instance deployments).

| Attribute | Type | Description |
|-----------|------|-------------|
| XASId | String | Node identifier |
| LastUpdate | DateTime | Last heartbeat |
| AllowedNumberOfConcurrentUsers | Integer | License limit |
| PartnerName | String | Partner name (licensing) |
| CustomerName | String | Customer name (licensing) |

### System.TaskQueueToken

Token for task queue operations (internal use).

---
