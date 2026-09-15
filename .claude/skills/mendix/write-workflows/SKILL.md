---
name: write-workflows
description: "Author Mendix workflows in MDL — user tasks, decisions, parallel splits, jumps, waits and boundary events, with CREATE, ALTER and DROP. Use when building a business process with human steps, timers or parallel branches."
---

# Mendix Workflows Skill

Guidance for **authoring** workflows in Mendix projects with MDL — not just
reading them. `CREATE WORKFLOW` / `DROP WORKFLOW` / `ALTER WORKFLOW` are fully
supported and build in Studio Pro. Workflows are **not** read-only in mxcli; do
not punt workflow creation to Studio Pro.

## When to Use This Skill

- Creating a business process: approvals, reviews, multi-step tasks with user
  interaction, timers, and parallel branches.
- Adding/removing/reordering activities in an existing workflow (`ALTER WORKFLOW`).
- Regenerating a workflow from `DESCRIBE WORKFLOW` output (round-trippable).

A workflow is a `Workflows$Workflow` unit driven by a **context entity**: the
persistent entity each workflow instance is about (the `Expense` being approved,
the `LeaveRequest` being reviewed). User tasks render a page bound to
`System.WorkflowUserTask`.

## Syntax — CREATE WORKFLOW

The header options are **order-sensitive** (parameter → display → description →
export level → overview page → due date → event handlers), and the body **must**
close with `END WORKFLOW`.

```sql
create workflow Module.ApprovalFlow
  parameter $Context: Module.Request        -- REQUIRED: must be a $-variable + context entity
  display 'Request Approval'                 -- optional human-readable name
  description 'Approves incoming requests'   -- optional
  export level Hidden                        -- optional: Hidden | API (default Hidden)
  overview page Module.WF_Overview           -- optional admin overview page
  on workflow events (UserTaskStarted, UserTaskEnded)   -- optional, repeatable
    microflow Module.ACT_AuditTask as 'Task audit'
  on any workflow event microflow Module.ACT_LogEvent   -- every type this Mendix version has
begin
  -- activities here, each terminated with ;
end workflow;
```

**Two gotchas that trip up first attempts:**

- `PARAMETER` takes a **`$`-variable then a context entity**: `parameter $Context:
  Module.Entity`. `parameter Module.Entity` and `parameter name: Module.Entity`
  both fail (`expecting VARIABLE`).
- The body closer is `end workflow`, **not** `end`. `end;` fails (`missing
  WORKFLOW`).

**The context is always stored as `WorkflowContext`.** Whatever you name the
variable in the header, mxcli writes the parameter as `WorkflowContext`, so
`$WorkflowContext/Attribute` is the canonical way to reach it in an expression.
The name you declared (`$Context` above) and any casing of the canonical name
(`$workflowContext`) are rewritten to it on write — in decision conditions, user
task due dates and XPath targeting, wait-for-timer delays, and `with (…)`
parameter mappings. Anything else is an undefined variable and Mendix fails the
build with `CE0117 "Error(s) in expression."`.

`create or replace workflow …` and `create or modify workflow …` are supported.

## Activities

Every activity statement ends with `;`. Blocks `{ … }` nest a sub-flow.

```sql
create or replace workflow Module.ApprovalFlow
  parameter $Context: Module.Request
begin
  -- User task: renders a page, offers named outcomes (branches)
  user task Review 'Review the request'
    page Module.ReviewPage
    targeting users microflow Module.ACT_Reviewers   -- or: targeting users xpath '[Active = true()]'
    on created microflow Module.ACT_AssignReviewer   -- optional: runs when the task is created
    description 'Please review'
    outcomes
      'Approve' { call microflow Module.ACT_Process; }
      'Reject'  { call microflow Module.ACT_Notify; };

  -- Multi user task: same clauses, one task per targeted user
  multi user task GroupSignoff 'Group sign-off'
    page Module.ReviewPage
    outcomes 'Done' { };

  -- Call a microflow (server logic); optional name, parameter mapping + outcomes
  call microflow Module.ACT_Validate as callMicroflow1
    with (Module.ACT_Validate.Item = '$WorkflowContext');

  -- Decision: a boolean or enum exclusive split. The name is optional; give one
  -- when a `jump to` targets it.
  decision decision1 '$WorkflowContext/Total > 1000'
    outcomes
      true  -> { call microflow Module.ACT_Escalate; }
      false -> { call microflow Module.ACT_AutoApprove; };

  -- An enum decision: each outcome is a FULLY QUALIFIED enumeration value
  -- (Module.Enumeration.Value), plus one '' outcome for "none of the above".
  decision decision2 '$WorkflowContext/Status'
    outcomes
      'Module.ENUM_Status.Approved' -> { }
      'Module.ENUM_Status.Rejected' -> { }
      '' -> { };

  -- Parallel split: independent branches run concurrently
  parallel split split1
    path 1 { call microflow Module.ACT_Notify; }
    path 2 { call microflow Module.ACT_Log; };

  -- Wait for a timer, then continue (duration is a Mendix expression)
  wait for timer timer1 'addHours([%CurrentDateTime%], 1)';

  -- Wait for an external notification (e.g. an event)
  wait for notification waitForNotification1;

  -- An intermediate notification event (Mendix 11.11+): what `notify workflow`
  -- targets by name
  notification DocumentsReceived comment 'Documents received';

  -- Loop back, or stop the whole workflow, from inside an outcome. A `jump to`
  -- and an `end workflow` must each END their path, so neither can close the
  -- main flow itself (CE6679 / CE6671).
  user task Confirm 'Confirm the booking'
    page Module.ReviewPage
    outcomes
      'Redo'   { jump to Review; }
      'Cancel' { end workflow comment 'Cancelled'; }
      'Done'   { };

  -- Call a sub-workflow
  call workflow Module.SubProcess as callWorkflow1 comment 'delegate';
end workflow;
```

> **Do NOT use `annotation '...'` in a workflow body.** It parses, but the
> annotation is written into the workflow's activity flow, which Mendix loads by
> constructing every child with a `Flow` parent — no annotation type takes one, so
> the resulting `.mpr` **cannot be loaded at all**: Studio Pro will not open the
> project and `mx check` fails before validating anything. `mxcli` now refuses the
> statement (MDL-WF04) at both check and exec time. Keep the note as an MDL comment
> (`-- ...`); workflow canvas annotations are not yet writable.

**Boundary events** attach a timer to a user task / call-microflow / wait:

```sql
create or replace workflow Module.WithBoundary
  parameter $Context: Module.Request
begin
  user task Review 'Review'
    page Module.ReviewPage
    outcomes 'Done' { }
    boundary event interrupting timer 'addDays([%CurrentDateTime%], 3)' {
      call microflow Module.ACT_Escalate;
    };
end workflow;
```

- **Name the kind** — `interrupting` or `non interrupting`. A bare `boundary event
  timer` writes a type no Mendix 11 runtime has: `check` and mxbuild pass, and the
  runtime then **refuses to start the application** ("Class
  'Workflows$TimerBoundaryEvent' could not be found"). mxcli refuses the bare form
  on Mendix 11 (MDL-WF07).
- **The delay is a DateTime expression**, such as `'addDays([%CurrentDateTime%], 3)'`
  — not an ISO duration like `'P3D'`.
- **Every boundary path must end** in a jump, an end, or Mendix's end-of-path
  marker, and mxcli now appends the marker for you — so a path may end in a
  `call microflow`, as above. Without it the two kinds fail in different places:
  an interrupting path is **CE0105** at build, and a non-interrupting one builds
  cleanly and then stops the runtime from starting ("Expected the flow to end with
  an end event"). Use `jump to <task>` when the path should return to the task.
- **A notification boundary event** (Mendix 11.11+) fires when `notify workflow`
  targets it, so it takes a **name** instead of a delay:
  `boundary event interrupting notification Withdrawn 'Request withdrawn' { end workflow; }`.
  The name is unique in the workflow. Only one interrupting boundary event per
  activity, of either kind (CE6697, MDL-WF15). `alter workflow … insert boundary
  event` cannot add one yet — restate the workflow.
- **Over MCP (`--mcp`), Studio Pro dictates how a notification path ends**, which
  mxbuild does not: an interrupting one ends in `end workflow;` (in `jump to` inside
  a parallel split), a non-interrupting one runs to its end. mxcli refuses the
  other shapes with that remedy, because Studio Pro's constructor would rewrite or
  reject them.

**Event sub-processes** are flows outside the main flow, written after the main body.
A notification (11.8+) or a timer (11.13+) starts one while the workflow runs;
`interrupting` cancels every active path first, `non interrupting` runs alongside:

```sql
create or modify workflow HR.Leave
  parameter $Context: HR.Request
begin
  user task Review 'Review' page HR.ReviewPage outcomes 'Approve' { } 'Reject' { };

  event subprocess ESP_Cancel 'Cancel request'
    on interrupting notification espCancelStart 'Cancel received' {
    call microflow HR.ACT_LogCancel;
  };
  event subprocess ESP_Reminder 'Daily reminder'
    on non interrupting timer 'addDays([%CurrentDateTime%], 1)' as espReminderStart {
    call microflow HR.ACT_Remind;
  };
end workflow;
```

- **The End is implicit**, as in the main flow: mxcli appends one unless the body
  already ends (`end workflow`, a `jump to`, or branches that all end). A body with
  no end is CE0105.
- **`jump to` stays inside its sub-process** — a jump to its own activities or its
  start event builds; into another sub-process, or between one and the main flow,
  is CE6682 (MDL-WF05).
- **A timer start needs its expression** (CE0126, MDL-WF14).
- **Names are shared with the main flow**: a start event named like an activity is
  CE0495, so mxcli makes it unique.

## DROP WORKFLOW

```sql
drop workflow Module.ApprovalFlow;
```

## ALTER WORKFLOW

In-place edits go through the workflow mutator — no full rewrite. Supports
`SET` properties, and `INSERT` / `DROP` / `REPLACE` of activities, outcomes,
parallel paths, decision conditions, and boundary events. Reference an activity
by its name (or an auto-named one by its caption in quotes).

Each operation is its **own statement** — there is no `{ … }` wrapper, and `SET`
uses no `=` (`set display 'X'`, not `set display = 'X'`):

```sql
alter workflow Module.ApprovalFlow set display 'Updated Approval';
alter workflow Module.ApprovalFlow set activity Review page Module.AltReviewPage;
alter workflow Module.ApprovalFlow insert after Review call microflow Module.ACT_Log;
alter workflow Module.ApprovalFlow replace activity ACT_Validate with call microflow Module.ACT_Process;
```

Consecutive `set`s may chain in one statement:
`alter workflow Module.ApprovalFlow set display 'X' set description 'Y';`

See `mdl-examples/doctype-tests/24-workflow-examples.mdl` for the full ALTER
surface (insert path, drop path, insert condition, boundary events).

**The INSERT op has to match the activity kind.** An activity's outcome list is
typed, and each op writes exactly one outcome type into it:

| Op | Writes | Only on |
|----|--------|---------|
| `insert outcome '<name>' on X { }` | `UserTaskOutcome` | a user task |
| `insert condition '<Module.Enum.Value>' on X { }` | `…ConditionOutcome` | a decision, a call microflow |
| `insert path on X { }` | `ParallelSplitOutcome` | a parallel split |
| `insert boundary event on X interrupting timer '<expr>' { }` | a boundary event | user task, call microflow, call workflow, wait for notification |

Aim one at the wrong kind and the outcome lands in a list that cannot hold it,
which is **not** a build error: the project stops **loading**, so Studio Pro will
not open it and `mx check` dies before it validates anything (ako/mxcli#415).
mxcli refuses all of these now — at `check --references` and at `exec`, which
call the same function — and the refusal names the op that fits the target. The
`drop` ops are unaffected: removing a branch cannot write a wrong type, and it
leaves an ordinary build error (`CE6686`) rather than an unloadable project.

## DESCRIBE round-trip

`DESCRIBE WORKFLOW Module.Name` emits **executable, re-runnable** MDL — user
tasks, decisions, splits, jump-to targets, wait activities and boundary events
all come back as statements (not comments). You can learn the exact syntax by
describing a Studio-Pro-authored workflow, and `describe → drop → exec`
reproduces a workflow that builds. (The implicit start/end activities are
omitted, as they are re-synthesised on create.)

Event sub-processes come back as `event subprocess … on …` blocks after the main
body, and notification activities and notification boundary events as statements.

## Activity names, and why `jump to` depends on them

Mendix stores `JumpToActivity.TargetActivity` as an activity **name string**, not
a pointer — so a jump is only as good as the name it aims at. Every activity type
takes an optional explicit name (`as <name>` for the two call activities, a bare
name for the rest); without one mxcli derives it from the caption, or from the
called document for `call microflow` / `call workflow`.

That default is fine for a workflow written from scratch, and it is why two
decisions sharing a caption used to collide on one name. It is **not** fine when
reproducing a workflow Studio Pro authored: Studio Pro names activities by type
and ordinal — `decision1`, `split1`, `callMicroflow1`, `userTask1`,
`waitForNotification1` — with no relation to the caption. `describe workflow`
emits the stored name whenever it is not derivable, so the jump wiring survives a
re-execution; before that it did not, and a `jump to decision1` reached MxBuild as
a jump to itself (**CE6681**, "not possible to jump to end activities or jump-to
activities" — an error naming a different fault). See ako/mxcli#408.

`mxcli check` resolves every jump against the activity names the script itself
declares (**MDL-WF05**) and lists the valid targets when one misses.

## Rewriting an existing workflow

`CREATE OR REPLACE|MODIFY WORKFLOW` **rebuilds the workflow from the statement**,
so anything the script does not restate is deleted — including each boundary
event's whole handler flow. This is the failure that costs real work: it is not
reported by `mx check` afterwards, because the result is a perfectly valid
workflow that simply no longer does what it did.

mxcli refuses the two cases where that would lose something:

- **more stored event sub-processes or notification activities than the statement
  declares** — restate them; a sub-process with no start event, which MDL cannot
  state, is refused outright;
- **more stored boundary events than the statement declares** — restate them and
  the rewrite proceeds, which is what `describe workflow` now emits for you;
- **more stored workflow event handlers, or user tasks with an on-created
  microflow, than the statement declares** — the same: restate them.

The safe way to change one activity in a workflow carrying hand-placed structure
is `ALTER WORKFLOW`, which mutates in place and touches nothing else.

## Microflow statements for workflow tasks

These run **inside a microflow** (not in the workflow body) and drive a running
workflow / its tasks. They are easy to miss — there is no `complete task`:

- `set task outcome $Task 'Approve';` — completes a `System.WorkflowUserTask` with a
  named outcome. This is how a microflow (e.g. a task page's button) finishes a task
  and does the domain work; the outcome branches still record which one was chosen.
- `$Notified = notify workflow $Wf target Module.Workflow.Name;` resumes the element
  it names — a notification-started event sub-process, a notification activity, a
  notification boundary event or a wait for notification. **The target is
  required**: a notify without one fails the build (CE0166, MDL-WF16). Name the
  element as `Module.Workflow.ElementName`; mxcli works out which kind it is and
  refuses one a notification cannot reach (a timer start, a user task).
- `open user task $Task`, `lock workflow $Wf`, and
  `workflow operation abort|pause|restart|retry|continue $Wf` are also statements.

A common shape: the task page's buttons call a microflow that does the change and
then `set task outcome $Task '<Outcome>'`, leaving the workflow's outcome branch
bodies empty.

### Claim the task before completing it

**`set task outcome` on a task nobody has claimed fails at runtime**, and it fails
quietly — the button appears to do nothing and the only trace is in the runtime log:

```
ERROR - Client: You can't complete this user task, it is not assigned to you.
```

`mxcli check` and `mx check` both pass; the build is clean. `mxcli check` now warns
about it (**MDL-WORKFLOW10**), but the platform rule is worth knowing rather than
being told.

The trap is that **`targeting xpath` / `targeting microflow` decides who may SEE a
task — it does not assign it.** There is no `assign task` statement; claiming is a
plain write to the Assignees association, and it must come first:

```sql
create microflow Module.ACT_CompleteTask ( $Task: System.WorkflowUserTask )
begin
  change $Task (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
  commit $Task;
  set task outcome $Task 'Plan';
end;
```

If the task is claimed somewhere else — earlier in the process, or in a microflow
this one calls — the warning does not apply.

Related: **`WorkflowUserTask.Name` holds the task's CAPTION, not the activity
name.** A task declared `user task "ReviewAndPlan" 'Review and plan'` stores
`Name = 'Review and plan'`, so routing an inbox on the activity name silently never
matches. Route on your own entity's status instead.

## System-module documents are read from the runtime, not the .mpr

`describe enumeration System.WorkflowUserTaskState` and `show enumerations in System`
return nothing — the System module's **enumerations** are not in the project file, so
mxcli cannot resolve them. Constrain on an attribute instead (`[EndTime = empty]`
selects open tasks) rather than naming a System enum value. System **entities** are
documented in `system-module`.

## Platform rules

- **Some workflow state has no MDL spelling, and a rewrite refuses rather than
  reset it.** An event sub-process and a workflow event handler subscribed to no
  event types are set in Studio Pro.
  `create or modify` on a workflow that holds any of them is refused with the
  list, and so is `alter workflow … replace activity` on an activity that holds
  one. Change such a workflow with `alter workflow … set activity …` (it edits
  the stored document and keeps the rest) or in Studio Pro.

- **`end workflow` ends the whole workflow from inside a branch** — the workflow
  counterpart of a microflow's `return`. `return;` itself is refused in a workflow
  (`MDL-WF11`): inside a `{ }` block it reads as "leave this block", which is
  exactly the fallthrough `end workflow` prevents. Measured placement rules
  (mxbuild 11.13, both engines), all checked without a project:
  - legal as the **last** statement of a user-task outcome, a decision branch, a
    call-microflow outcome or an **interrupting** boundary-event path, at any depth;
  - refused under a **parallel split** or a **non-interrupting** boundary-event
    path, at any depth — `CE1844`, `MDL-WF08` (a path cannot end the workflow
    while the others run; jumping out of a path is refused too, `CE6682`);
  - refused with anything after it in its block — `CE6671`, `MDL-WF09`;
  - when **every** path of an activity ends — in `end workflow` or `jump to`,
    also through a nested decision — nothing may follow it, not even the end of
    the main flow: `CE6689`, `MDL-WF10`. Let one path continue; a path that
    reaches the end of the workflow needs no `end workflow`.
  - The main flow needs none: the body's closing `end workflow` is its End.
  An outcome left **empty** does not stop anything — it rejoins the main flow.
  `comment '…'` sets the End's caption, as on every workflow activity.

- **A multi-user task says who must respond and how their outcomes decide**,
  in this clause order before `outcomes`:
  `participants all | <n> | <n> percent`, then `decide by …`, then
  `await all users`. The rules (`decide by`):
  `consensus fallback '<outcome>'`, `majority more than half fallback '…'`,
  `majority most chosen fallback '…'`, `threshold <n> percent|votes fallback '…'`,
  `veto '<outcome>'`, `microflow Module.Decide`. Omitted means all participants,
  consensus falling back to the first outcome, and not waiting. Measured on
  mxbuild 11.13:
  - consensus, majority and threshold **need a fallback** (`CE1866`) and a veto
    needs its outcome (`CE1867`); `check` refuses a missing one, and a name
    that is not one of the task's outcomes (`MDL-WF13`);
  - the decision microflow must **return String** (`CE5012`); its parameters
    are free;
  - thresholds and participant counts are **not range-checked** by the build
    (0, 101 percent, more votes than users all build), so check them yourself.
  A rewrite that does not restate a stored rule, participant count or `await all
  users` is refused — each omitted clause would reset it.
- **An AI agent task is `call agent microflow`** (Mendix 11.9+) — the call
  microflow statement stored as `Workflows$AIAgentTaskActivity`, with the same
  `as`, `comment`, `with (…)`, `outcomes` and boundary events. The microflow is
  where the agent is invoked. Measured on mxbuild 11.13 against the identical
  call microflow, one rule differs: **its microflow must take a parameter**
  (`CE1590 "Missing parameter"`), usually the context object mapped with
  `with (Param = '$WorkflowContext')`. Return Boolean or an enumeration to
  branch on the answer.
- **Handler microflows have fixed signatures** (measured, mxbuild 11.13):
  - `on created microflow` takes exactly `System.WorkflowUserTask` and the context
    entity, in either order — anything else is `CE6683` — and returns nothing
    (`CE5012`).
  - A workflow event handler takes exactly `System.WorkflowEvent`,
    `System.WorkflowRecord` and `System.WorkflowActivityRecord`, in any order
    (`CE6691`).
  - **Event type names are not checked by the build** — an invented one builds at
    0 errors and never fires. mxcli refuses an unknown name (`MDL-WF12`) and a type
    the project's version does not have. The list is in `mxcli syntax
    workflow.event-handlers`.
  - `on any workflow event` stores the full list for the project's version (Studio
    Pro stores a list, not a flag), so it needs Mendix 11.6+; name the types on
    older projects.
- A user task needs a **task page** to be useful; without one Mendix flags the
  task (`CE1834`). Bind the page to `System.WorkflowUserTask`.
- **The task page takes the TASK, not the workflow's context object.** It must
  declare a `System.WorkflowUserTask` parameter: a page with no parameters is
  `CE7410`, a page whose parameters are all something else (the usual mistake:
  the context entity) is `CE7412`. Other parameters may sit alongside the task
  one — that builds clean. Multi-user tasks follow the same rule.
- **A targeting microflow takes exactly two parameters: `System.Workflow` and
  the workflow's context entity**, in either order. One parameter, none, or a
  third is `CE6677`. The context parameter may be typed to a *generalization* of
  the context entity, not a specialization. `targeting groups microflow` takes
  the same two and returns a list of `System.WorkflowGroup`; users targeting
  returns a list of `System.User`.
- `mxcli check --references` reports both signatures **before anything is
  written**, for pages and microflows in the project or created earlier in the
  same script (measured on Mendix 11.13; not applied to older projects). `exec`
  refuses the workflow statement itself, so the workflow is never written — but
  the statements before it in the script already are. Run `check --references`
  first. Plain `mxcli check` without a project cannot see these.
- A user task / decision with a single outcome and no activity can trip
  `CE1876` — give each branch a body or a distinct outcome.
- **An enum decision's outcome must be `Module.Enumeration.Value`.** Mendix
  stores it as an `EnumerationValueIdentifier` and parses it when the project is
  **loaded**, before any consistency check — so a short name is not a build
  error with a CE number, it leaves a project Studio Pro and mxbuild cannot open
  (`StorageLoadException`). Measured: `'Approved'` and `'Status.Approved'` both
  make the project unloadable; `'Sales.ENUM_Status.Approved'` checks at 0
  errors. Shortening it because the enumeration is in the same module does not
  work. `mxcli check` refuses all three of these as `MDL-WF03`, and `exec`
  refuses to run a script it flags.
- **An enum decision also needs one `'' -> { }` outcome** for "none of the
  above": Mendix generates one outcome per enumeration value plus the empty one,
  and MxBuild compares the stored set against that, so anything else is `CE6686`
  ("Regenerate the outcomes"). `check` reports a missing one as `MDL-WF06`. It
  applies equally to a `call microflow` activity branching on an enumeration
  return, and to a decision introduced by `ALTER WORKFLOW … INSERT AFTER` /
  `REPLACE ACTIVITY`. A **required (`not null`) attribute does not exempt it** —
  measured, the empty outcome is still required. Boolean (`true`/`false`)
  decisions do not take one.
- **A `with (...)` parameter value is a quoted string**, not a bare variable:
  `with (Request = '$WorkflowContext')`. The unquoted spelling used elsewhere in
  MDL is a syntax error here (it used to crash the binary — ako/mxcli#1023).
- The context **Parameter entity must be persistent**.
- Write the context variable as **`$WorkflowContext`**, matching the parameter
  name exactly. Mendix expressions are case-sensitive on 11.9+, so a lowercase
  `$workflowContext` is an undefined variable and yields `CE0117`.

## Observing a running workflow

A workflow's characteristic failures are **runtime** failures — an instance that
starts and stops, a task that never reaches an inbox, a task page that renders
blank. None of them is visible to `mxcli check`, `mxcli lint` or `mx check`,
which all validate the model rather than the data the model no longer matches.
So do not stop at "it builds".

Everything needed is already a skill — read the one you need rather than
hand-rolling admin-API calls:

| To see | Read |
|---|---|
| Live instances and open tasks (OQL against the running app) | [`verify-with-oql`](../verify-with-oql/SKILL.md), [`write-oql-queries`](../write-oql-queries/SKILL.md) |
| The exception that stopped an instance | [`analyze-runtime`](../analyze-runtime/SKILL.md) — `run --local` tees the runtime log to `<projectDir>/.mxcli/runtime.log` |
| `System.Workflow` / `System.WorkflowUserTask` / `System.WorkflowDefinition` shapes | [`system-module`](../system-module/SKILL.md) |
| Driving a task end to end and asserting the result | [`test-app`](../test-app/SKILL.md), [`run-local`](../run-local/SKILL.md) |
| Raw admin API, incl. `POST /dev/preview_execute_oql` | [`runtime-admin-api`](../runtime-admin-api/SKILL.md) |

Two traps worth knowing before you start:

- **The declared return type is not what the runtime checks.** A workflow-called
  microflow whose end event returns a value while the microflow declares no
  return type fails at instance start with `Trying to compare
  VoidConditionValue$('') to BooleanValue('true')`. `mxcli check` catches this as
  **MDL004** — so do not skip it, and do not reach for `--no-check` to get past
  it. Read the message in the order it is written: the receiver is the stored
  outcome's condition, the argument is what the microflow actually returned.
- **A parked instance is not a failed one.** A wait or timer branch is supposed
  to sit there. Check the branch before calling it a hang.

## Validate before presenting

```bash
./bin/mxcli check script.mdl                      # syntax + activity grammar
./bin/mxcli check script.mdl -p app.mpr --references   # entity/page/microflow refs exist
```

Then `show workflows` (lists the workflow, its parameter entity, and activity
count) and, if Docker is available, `mxcli docker build -p app.mpr` for the full
Studio-Pro validation.
