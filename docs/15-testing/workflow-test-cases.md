# Workflow Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/301)

## Test Projects

Demo apps from [Mendix App Gallery](https://appgallery.mendixcloud.com/):

| App | Studio Pro | Workflows | User Tasks | Decisions |
|-----|-----------|-----------|------------|-----------|
| Lato Enquiry Management | 11.4.0 | <N> | <N> | <N> |
| Evora - Factory Management | 10.24.15 | <N> | <N> | <N> |
| Lato Product Inventory | 11.2.0 | <N> | <N> | <N> |

---

## Setup

### 1. Download test apps

1. Go to [Mendix App Gallery](https://appgallery.mendixcloud.com/)
2. Download each demo app listed above
3. Open each `.mpk` in Studio Pro to extract the `.mpr` file

### 2. Build mxcli

```bash
make build && make test && make lint-go
```

### 3. Smoke test

```bash
APPS_DIR=<path-to-extracted-apps>
for mpr in "$APPS_DIR"/*/*.mpr; do
  echo "=== $(basename $(dirname $mpr)) ==="
  echo "show workflows;" > /tmp/show-wf.mdl
  mxcli exec /tmp/show-wf.mdl -p "$mpr" 2>&1 | tail -1
done
```

Expected: count line `(N workflows)` for each project.

### 4. Interactive testing

```bash
mxcli repl -p <path-to-app>/EnquiriesManagement.mpr
```

### 5. Script-based testing

```bash
mxcli exec test-sequence.mdl -p <mpr>
```

> **IMPORTANT:** Always run destructive tests against a **copy** of the project folder.
> Dropped workflows cannot be recovered.
>
> ```bash
> cp -r MyProject MyProject-test
> mxcli repl -p MyProject-test/MyProject.mpr
> ```

---

## 1. SHOW WORKFLOWS

### 1.1 List all workflows

```
show workflows;
```

**Expected:** Table with columns `Qualified Name | Activities | User Tasks | Decisions | Parameter Entity`. Summary `(N workflows)`.

### 1.2 Filter by module

```
show workflows in Administration;
```

**Expected:** Only workflows from `Administration` module.

### 1.3 Empty module

```
show workflows in NonExistentModule;
```

**Expected:** Empty result or `(0 workflows)`.

---

## 2. DESCRIBE WORKFLOW

### 2.1 Simple workflow

```
describe workflow MyModule.ApprovalWorkflow;
```

**Expected:** Full MDL output with header properties:

```
create or modify workflow MyModule.ApprovalWorkflow
  parameter MyModule.Request
  display 'Approval Workflow'
  description 'Handles approval requests'
  export_level Hidden
  overview_page MyModule.WorkflowOverview
  due_date '7d'
begin
  ...
end workflow;
/
```

### 2.2 User task activity

```
describe workflow <Module.WorkflowWithUserTask>;
```

**Expected:** Body contains user task with page, targeting, due date:

```
  user task 'Review Request'
    page MyModule.ReviewPage
    description 'Review the submitted request'
    targeting_microflow MyModule.GetReviewers
    due_date '3d'
  outcomes
    'Approve'
    'Reject'
  end task;
```

### 2.3 Multi user task activity

```
describe workflow <Module.WorkflowWithMultiUserTask>;
```

**Expected:** `multi user task` keyword. Outcomes listed.

### 2.4 Call microflow — without parameters

```
describe workflow <Module.WorkflowWithMfCall>;
```

**Expected:** Body contains:

```
  call microflow MyModule.ProcessRequest;
```

### 2.5 Call microflow — with parameters

```
describe workflow <Module.WorkflowWithParamMfCall>;
```

**Expected:** Body contains:

```
  call microflow MyModule.SendNotification($WorkflowContext);
```

### 2.6 Call workflow activity

```
describe workflow <Module.WorkflowWithSubWorkflow>;
```

**Expected:** Body contains `call workflow MyModule.SubWorkflow;`.

### 2.7 Boolean decision

```
describe workflow <Module.WorkflowWithBoolDecision>;
```

**Expected:** Decision with `true`/`false` outcomes. Each outcome contains nested activities:

```
  decision 'Is Approved'
    expression $WorkflowContext/IsApproved
  outcomes
    true
      call microflow MyModule.OnApproved;
    false
      call microflow MyModule.OnRejected;
  end decision;
```

### 2.8 Enum decision

```
describe workflow <Module.WorkflowWithEnumDecision>;
```

**Expected:** Decision outcomes match enum values:

```
  decision 'Check Status'
    expression $WorkflowContext/Status
  outcomes
    'Draft'
      ...
    'Submitted'
      ...
    'Archived'
      ...
  end decision;
```

### 2.9 Parallel split

```
describe workflow <Module.WorkflowWithParallelSplit>;
```

**Expected:** Parallel split with named paths:

```
  parallel split 'Parallel Approvals'
  paths
    path 'Finance Review'
      user task 'Finance Approval' ...
    path 'Legal Review'
      user task 'Legal Approval' ...
  end split;
```

### 2.10 Jump to activity

```
describe workflow <Module.WorkflowWithJump>;
```

**Expected:** `jump to 'ActivityCaption';` in body.

### 2.11 Wait for timer

```
describe workflow <Module.WorkflowWithTimer>;
```

**Expected:** `wait for timer delay '2d';` or similar duration expression.

### 2.12 Wait for notification

```
describe workflow <Module.WorkflowWithNotification>;
```

**Expected:** `wait for notification;` in body.

### 2.13 Annotation

```
describe workflow <Module.AnnotatedWorkflow>;
```

**Expected:** `@annotation 'text'` on activities or at workflow level.

### 2.14 Boundary event — interrupting timer

```
describe workflow <Module.WorkflowWithInterruptingTimer>;
```

**Expected:** Boundary event attached to a user task:

```
  user task 'Review'
    ...
  outcomes
    'Approve'
      ...
  boundary events
    interrupting timer delay '5d'
      call microflow MyModule.OnTimeout;
  end task;
```

### 2.15 Boundary event — non-interrupting timer

```
describe workflow <Module.WorkflowWithNonInterruptingTimer>;
```

**Expected:** `non-interrupting timer delay '3d'` in boundary events block.

### 2.16 Outcome blocks with nested activities

```
describe workflow <Module.WorkflowWithNestedOutcomes>;
```

**Expected:** Each outcome contains one or more activities. Nesting preserved in indentation.

### 2.17 Non-existent workflow

```
describe workflow Fake.Missing;
```

**Expected:** Error — not found.

> **Note:** GRANT/REVOKE EXECUTE ON WORKFLOW is not supported. Mendix workflows do not have document-level `AllowedModuleRoles`. Workflow access is controlled via trigger microflows. The security-test-cases doc covers GRANT EXECUTE on microflows/nanoflows.

---

## 3. CREATE WORKFLOW

### 3.1 Minimal workflow

```
create workflow MyModule.WF_Simple
  parameter MyModule.Request
begin
end workflow;
```

**Expected:** Created. `show workflows in MyModule` includes it. Implicit Start and End activities generated.

### 3.2 Full syntax with all optional clauses

```
create workflow MyModule.WF_Full
  parameter MyModule.Request
  display 'Full Workflow'
  description 'A workflow with all optional clauses'
  export_level Hidden
  overview_page MyModule.OverviewPage
  due_date '14d'
begin
  user task 'Review'
    page MyModule.ReviewPage
    description 'Initial review'
    targeting_microflow MyModule.GetReviewers
    due_date '3d'
  outcomes
    'Approve'
      call microflow MyModule.OnApproved;
    'Reject'
      call microflow MyModule.OnRejected;
  end task;
end workflow;
```

**Expected:** All header properties and body activities preserved. `describe` output matches.

### 3.3 CREATE OR REPLACE variant

```
create or replace workflow MyModule.WF_Simple
  parameter MyModule.Request
begin
  user task 'New Task'
    page MyModule.TaskPage
  outcomes
    'Done'
  end task;
end workflow;
```

**Expected:** Drops and recreates. New body replaces old.

### 3.4 Multiple activity types in body

```
create workflow MyModule.WF_Complex
  parameter MyModule.Request
begin
  call microflow MyModule.ValidateRequest;
  decision 'Is Valid'
    expression $WorkflowContext/IsValid
  outcomes
    true
      user task 'Approve'
        page MyModule.ApprovalPage
        targeting_xpath '[Module.Request_Approver = '[%CurrentUser%]']'
        due_date '5d'
      outcomes
        'Approved'
          call microflow MyModule.FinalizeApproval;
        'Rejected'
          call microflow MyModule.NotifyRejection;
      end task;
    false
      call microflow MyModule.RejectInvalid;
  end decision;
end workflow;
```

**Expected:** Decision with nested user task and outcomes created.

### 3.5 Parallel split in body

```
create workflow MyModule.WF_Parallel
  parameter MyModule.Request
begin
  parallel split 'Multi-Approval'
  paths
    path 'Manager Review'
      user task 'Manager Approval'
        page MyModule.ManagerPage
      outcomes
        'Approve'
        'Reject'
      end task;
    path 'Finance Review'
      user task 'Finance Approval'
        page MyModule.FinancePage
      outcomes
        'Approve'
        'Reject'
      end task;
  end split;
end workflow;
```

**Expected:** Parallel split with two paths created. Each path contains a user task.

### 3.6 Implicit Start/End auto-behavior

```
create workflow MyModule.WF_Implicit
  parameter MyModule.Request
begin
  call microflow MyModule.DoSomething;
end workflow;
```

**Expected:** `describe` shows Start and End activities implicitly. No need to declare them.

### 3.7 Parameter auto-binding

```
create workflow MyModule.WF_AutoBind
  parameter MyModule.Request
begin
  user task 'Review'
    page MyModule.ReviewPage
  outcomes
    'Done'
  end task;
end workflow;
```

**Expected:** Parameter entity `MyModule.Request` auto-binds as `$WorkflowContext`. Accessible in expressions and activity targeting.

### 3.8 Duplicate name deduplication (CE0495)

```
create workflow MyModule.WF_DupNames
  parameter MyModule.Request
begin
  call microflow MyModule.Step1;
  call microflow MyModule.Step1;
end workflow;
```

**Expected:** Compiler event CE0495 — duplicate activity caption. Second activity receives deduplicated caption (e.g., `Step1 2`).

### 3.9 Boolean decision skips Default outcome (CE6686)

```
create workflow MyModule.WF_BoolDecision
  parameter MyModule.Request
begin
  decision 'Check Flag'
    expression $WorkflowContext/IsActive
  outcomes
    true
      call microflow MyModule.OnActive;
    false
      call microflow MyModule.OnInactive;
  end decision;
end workflow;
```

**Expected:** No Default outcome generated for boolean decisions. Compiler event CE6686 suppressed.

### 3.10 Duplicate workflow (without modifier)

```
create workflow MyModule.WF_Simple
  parameter MyModule.Request
begin
end workflow;
```

**Expected:** Error — workflow already exists.

### 3.11 Boundary events in CREATE

```
create workflow MyModule.WF_WithBoundary
  parameter MyModule.Request
begin
  user task 'Long Review'
    page MyModule.ReviewPage
    due_date '7d'
  outcomes
    'Approve'
    'Reject'
  boundary events
    interrupting timer delay '10d'
      call microflow MyModule.EscalateTimeout;
    non-interrupting timer delay '5d'
      call microflow MyModule.SendReminder;
  end task;
end workflow;
```

**Expected:** Both boundary events created on the user task. `describe` shows both.

---

## 4. DROP WORKFLOW

### 4.1 Drop existing workflow

```
drop workflow MyModule.WF_Simple;
```

**Expected:** Workflow removed. Not in `show workflows`.

### 4.2 Drop non-existent workflow

```
drop workflow MyModule.Fake;
```

**Expected:** Error — not found.

---

## 5. ALTER WORKFLOW (Mendix 9.12+)

> ALTER WORKFLOW requires Mendix 9.12+. Test against a project opened with Studio Pro 9.12 or later.

### 5.1 SET display property

```
alter workflow MyModule.WF_Full
  set display 'Updated Display Name';
```

**Expected:** `describe` shows new display name.

### 5.2 SET description property

```
alter workflow MyModule.WF_Full
  set description 'Updated description text';
```

**Expected:** `describe` shows new description.

### 5.3 SET export_level property

```
alter workflow MyModule.WF_Full
  set export_level Public;
```

**Expected:** `describe` shows `export_level Public`.

### 5.4 SET due_date property

```
alter workflow MyModule.WF_Full
  set due_date '30d';
```

**Expected:** `describe` shows updated due date.

### 5.5 SET overview_page property

```
alter workflow MyModule.WF_Full
  set overview_page MyModule.NewOverviewPage;
```

**Expected:** `describe` shows new overview page.

### 5.6 SET parameter property

```
alter workflow MyModule.WF_Full
  set parameter MyModule.NewParameterEntity;
```

**Expected:** `describe` shows updated parameter entity.

### 5.7 SET ACTIVITY page

```
alter workflow MyModule.WF_Full
  set activity 'Review' page MyModule.NewReviewPage;
```

**Expected:** User task `Review` points to new page. `describe` confirms.

### 5.8 SET ACTIVITY description

```
alter workflow MyModule.WF_Full
  set activity 'Review' description 'Updated task description';
```

**Expected:** `describe` shows new description on the activity.

### 5.9 SET ACTIVITY targeting_microflow

```
alter workflow MyModule.WF_Full
  set activity 'Review' targeting_microflow MyModule.NewTargeting;
```

**Expected:** Targeting microflow updated.

### 5.10 SET ACTIVITY targeting_xpath

```
alter workflow MyModule.WF_Full
  set activity 'Review' targeting_xpath '[Status = ''Active'']';
```

**Expected:** Targeting XPath updated.

### 5.11 SET ACTIVITY due_date

```
alter workflow MyModule.WF_Full
  set activity 'Review' due_date '5d';
```

**Expected:** Activity due date updated.

### 5.12 SET ACTIVITY with AT POSITION for disambiguation

```
alter workflow MyModule.WF_DupNames
  set activity 'Step1' at position 2 description 'Second step';
```

**Expected:** Only the second `Step1` activity updated.

### 5.13 INSERT AFTER activity

```
alter workflow MyModule.WF_Full
  insert after 'Review'
    call microflow MyModule.PostReviewAction;
```

**Expected:** New activity inserted after `Review`. `describe` shows correct ordering.

### 5.14 INSERT AFTER with AT POSITION

```
alter workflow MyModule.WF_DupNames
  insert after 'Step1' at position 1
    call microflow MyModule.AfterFirstStep;
```

**Expected:** Activity inserted after the first `Step1` only.

### 5.15 INSERT AFTER with full activity definition

```
alter workflow MyModule.WF_Full
  insert after 'Review'
    user task 'Secondary Review'
      page MyModule.SecondaryPage
      due_date '2d'
    outcomes
      'Approve'
      'Reject'
    end task;
```

**Expected:** Full user task definition inserted.

### 5.16 DROP ACTIVITY

```
alter workflow MyModule.WF_Full
  drop activity 'Secondary Review';
```

**Expected:** Activity removed. `describe` no longer shows it.

### 5.17 DROP ACTIVITY with AT POSITION

```
alter workflow MyModule.WF_DupNames
  drop activity 'Step1' at position 2;
```

**Expected:** Only the second `Step1` removed.

### 5.18 REPLACE ACTIVITY

```
alter workflow MyModule.WF_Full
  replace activity 'Review' with
    user task 'Enhanced Review'
      page MyModule.EnhancedPage
      targeting_microflow MyModule.NewTargeting
      due_date '4d'
    outcomes
      'Approve'
      'Reject'
      'Escalate'
    end task;
```

**Expected:** Old activity replaced. `describe` shows `Enhanced Review` with three outcomes.

### 5.19 REPLACE ACTIVITY with AT POSITION

```
alter workflow MyModule.WF_DupNames
  replace activity 'Step1' at position 1 with
    call microflow MyModule.ReplacedStep;
```

**Expected:** Only the first `Step1` replaced.

### 5.20 INSERT OUTCOME on user task

```
alter workflow MyModule.WF_Full
  insert outcome 'Escalate' on activity 'Enhanced Review';
```

**Expected:** New outcome added. `describe` shows three outcomes.

### 5.21 DROP OUTCOME on user task

```
alter workflow MyModule.WF_Full
  drop outcome 'Escalate' on activity 'Enhanced Review';
```

**Expected:** Outcome removed. `describe` shows two outcomes.

### 5.22 INSERT PATH on parallel split

```
alter workflow MyModule.WF_Parallel
  insert path 'HR Review' on activity 'Multi-Approval'
    user task 'HR Approval'
      page MyModule.HRPage
    outcomes
      'Approve'
      'Reject'
    end task;
```

**Expected:** Third path added to parallel split.

### 5.23 DROP PATH on parallel split

```
alter workflow MyModule.WF_Parallel
  drop path 'HR Review' on activity 'Multi-Approval';
```

**Expected:** Path removed. `describe` shows two paths.

### 5.24 INSERT BRANCH on decision

```
alter workflow MyModule.WF_BoolDecision
  insert branch 'Maybe' on activity 'Check Flag'
    call microflow MyModule.OnMaybe;
```

**Expected:** New branch added to decision (applicable for enum decisions).

### 5.25 DROP BRANCH on decision

```
alter workflow MyModule.WF_BoolDecision
  drop branch 'Maybe' on activity 'Check Flag';
```

**Expected:** Branch removed.

### 5.26 INSERT BOUNDARY EVENT

```
alter workflow MyModule.WF_Full
  insert boundary event on activity 'Enhanced Review'
    interrupting timer delay '7d'
      call microflow MyModule.OnEscalation;
```

**Expected:** Boundary event added to user task.

### 5.27 DROP BOUNDARY EVENT

```
alter workflow MyModule.WF_Full
  drop boundary event interrupting timer on activity 'Enhanced Review';
```

**Expected:** Boundary event removed.

### 5.28 INSERT non-interrupting BOUNDARY EVENT

```
alter workflow MyModule.WF_Full
  insert boundary event on activity 'Enhanced Review'
    non-interrupting timer delay '3d'
      call microflow MyModule.SendReminder;
```

**Expected:** Non-interrupting timer added. `describe` shows both event types if interrupting timer remains.

---

## 6. ROUNDTRIP

### 6.1 Simple roundtrip

1. `describe workflow MyModule.WF_Full` → capture MDL
2. `drop workflow MyModule.WF_Full`
3. Execute captured MDL
4. `describe workflow MyModule.WF_Full` → capture again

**Expected:** Output identical between step 1 and step 4.

### 6.2 Complex roundtrip

Create a workflow with: user tasks, decisions (boolean + enum), parallel split, boundary events, call microflow, call workflow. Run DESCRIBE → DROP → execute → DESCRIBE.

**Expected:** All activity types, outcomes, paths, and boundary events preserved.

---

## 7. MULTI-STEP

### 7.1 Create workflow with user tasks → ALTER to add decisions

1. Create workflow with two user tasks
2. `alter workflow ... insert after 'Task1'` a boolean decision
3. `describe` → verify decision between Task1 and Task2

### 7.2 ALTER to insert boundary events

1. Start from §7.1 result
2. `alter workflow ... insert boundary event on activity 'Task2'` with interrupting timer
3. `describe` → verify boundary event on Task2

### 7.3 Full multi-step → DESCRIBE → verify structure

1. Start from §7.2 result
2. `alter workflow ... insert path` on a new parallel split
3. `alter workflow ... insert outcome` on a user task
4. `describe` → verify complete structure matches all modifications

---

## 8. FAILURE MODES

### 8.1 Not connected

```
describe workflow MyModule.WF_Full;
```

Run without opening a project.

**Expected:** Error — not connected to a project.

### 8.2 Workflow not found

```
describe workflow MyModule.DoesNotExist;
```

**Expected:** Error — workflow not found.

### 8.3 Feature gate — ALTER on pre-9.12 project

```
alter workflow MyModule.WF_Full
  set display 'New Name';
```

Run against a Mendix project older than 9.12.

**Expected:** Error indicating ALTER WORKFLOW requires Mendix 9.12+.

### 8.4 Activity caption not found

```
alter workflow MyModule.WF_Full
  set activity 'NonExistentTask' page MyModule.SomePage;
```

**Expected:** Error — activity with caption `NonExistentTask` not found.

### 8.5 Duplicate ambiguous caption without AT POSITION

```
alter workflow MyModule.WF_DupNames
  set activity 'Step1' description 'Which one?';
```

**Expected:** Error — ambiguous caption. Use `AT POSITION N` to disambiguate.

### 8.6 Invalid AT POSITION index

```
alter workflow MyModule.WF_DupNames
  set activity 'Step1' at position 99 description 'Out of range';
```

**Expected:** Error — position out of range.

### 8.7 DROP last outcome on user task

```
alter workflow MyModule.WF_Full
  drop outcome 'Approve' on activity 'Review';
alter workflow MyModule.WF_Full
  drop outcome 'Reject' on activity 'Review';
```

**Expected:** Error when dropping the last outcome — user task requires at least one.

### 8.8 DROP last path on parallel split

**Expected:** Error — parallel split requires at least two paths.

### 8.9 INSERT AFTER non-existent activity

```
alter workflow MyModule.WF_Full
  insert after 'GhostActivity'
    call microflow MyModule.Something;
```

**Expected:** Error — activity not found.

### 8.10 CREATE with non-existent parameter entity

```
create workflow MyModule.WF_BadParam
  parameter NonExistent.Entity
begin
end workflow;
```

**Expected:** Error — entity not found.

---

## Activity Coverage Table

| Activity Type | Test Case | Section |
|---------------|-----------|---------|
| user task | 2.2, 3.2 | DESCRIBE, CREATE |
| multi user task | 2.3 | DESCRIBE |
| call microflow (no params) | 2.4 | DESCRIBE |
| call microflow (with params) | 2.5 | DESCRIBE |
| call workflow | 2.6 | DESCRIBE |
| decision (boolean) | 2.7, 3.9 | DESCRIBE, CREATE |
| decision (enum) | 2.8 | DESCRIBE |
| parallel split | 2.9, 3.5 | DESCRIBE, CREATE |
| jump to | 2.10 | DESCRIBE |
| wait for timer | 2.11 | DESCRIBE |
| wait for notification | 2.12 | DESCRIBE |
| annotation | 2.13 | DESCRIBE |
| boundary event (interrupting) | 2.14, 3.11 | DESCRIBE, CREATE |
| boundary event (non-interrupting) | 2.15, 3.11 | DESCRIBE, CREATE |

---

## Test Project Coverage Matrix

| Operation | Lato Enquiry | Evora Factory | Lato Product |
|-----------|:---:|:---:|:---:|
| SHOW WORKFLOWS | x | x | x |
| DESCRIBE WORKFLOW | x | x | x |
| CREATE WORKFLOW | x | | |
| DROP WORKFLOW | x | | |
| ALTER WORKFLOW | x | | |
| Roundtrip | x | x | x |
| Multi-step | x | | |

---

## Automated Test Coverage

| Section | Automated | Manual-only |
|---------|:---------:|:-----------:|
| 1. SHOW WORKFLOWS | Mock tests | |
| 2. DESCRIBE WORKFLOW | Mock tests | Boundary events, annotations |
| 3. CREATE WORKFLOW | Mock tests | Complex bodies, auto-behaviors |
| 4. DROP WORKFLOW | Mock tests | |
| 5. ALTER WORKFLOW | | All manual |
| 6. Roundtrip | | All manual |
| 7. Multi-step | | All manual |
| 8. Failure modes | Partial | Feature gate, ambiguous captions |

---

## Manual Test Report Template

**Tester:** _______________
**Date:** _______________
**Project:** _______________

| # | Section | Test | Pass | Fail | Skip | Notes |
|---|---------|------|:----:|:----:|:----:|-------|
| 1.1 | SHOW WF | List all | | | | |
| 1.2 | SHOW WF | Filter module | | | | |
| 1.3 | SHOW WF | Empty module | | | | |
| 2.1 | DESCRIBE | Simple | | | | |
| 2.2 | DESCRIBE | User task | | | | |
| 2.3 | DESCRIBE | Multi user task | | | | |
| 2.4 | DESCRIBE | Call mf (no params) | | | | |
| 2.5 | DESCRIBE | Call mf (with params) | | | | |
| 2.6 | DESCRIBE | Call workflow | | | | |
| 2.7 | DESCRIBE | Boolean decision | | | | |
| 2.8 | DESCRIBE | Enum decision | | | | |
| 2.9 | DESCRIBE | Parallel split | | | | |
| 2.10 | DESCRIBE | Jump to | | | | |
| 2.11 | DESCRIBE | Wait for timer | | | | |
| 2.12 | DESCRIBE | Wait for notification | | | | |
| 2.13 | DESCRIBE | Annotation | | | | |
| 2.14 | DESCRIBE | Boundary (interrupting) | | | | |
| 2.15 | DESCRIBE | Boundary (non-interrupting) | | | | |
| 2.16 | DESCRIBE | Nested outcomes | | | | |
| 2.17 | DESCRIBE | Not found | | | | |
| 3.1 | CREATE | Minimal | | | | |
| 3.2 | CREATE | Full syntax | | | | |
| 3.3 | CREATE | CREATE OR REPLACE | | | | |
| 3.4 | CREATE | Multiple activity types | | | | |
| 3.5 | CREATE | Parallel split | | | | |
| 3.6 | CREATE | Implicit Start/End | | | | |
| 3.7 | CREATE | Parameter auto-binding | | | | |
| 3.8 | CREATE | Duplicate dedup (CE0495) | | | | |
| 3.9 | CREATE | Bool decision skip Default (CE6686) | | | | |
| 3.10 | CREATE | Duplicate error | | | | |
| 3.11 | CREATE | Boundary events | | | | |
| 4.1 | DROP | Existing | | | | |
| 4.2 | DROP | Non-existent | | | | |
| 5.1 | ALTER | SET display | | | | |
| 5.2 | ALTER | SET description | | | | |
| 5.3 | ALTER | SET export_level | | | | |
| 5.4 | ALTER | SET due_date | | | | |
| 5.5 | ALTER | SET overview_page | | | | |
| 5.6 | ALTER | SET parameter | | | | |
| 5.7 | ALTER | SET ACTIVITY page | | | | |
| 5.8 | ALTER | SET ACTIVITY description | | | | |
| 5.9 | ALTER | SET ACTIVITY targeting_mf | | | | |
| 5.10 | ALTER | SET ACTIVITY targeting_xpath | | | | |
| 5.11 | ALTER | SET ACTIVITY due_date | | | | |
| 5.12 | ALTER | SET ACTIVITY AT POSITION | | | | |
| 5.13 | ALTER | INSERT AFTER | | | | |
| 5.14 | ALTER | INSERT AFTER AT POSITION | | | | |
| 5.15 | ALTER | INSERT full activity def | | | | |
| 5.16 | ALTER | DROP ACTIVITY | | | | |
| 5.17 | ALTER | DROP ACTIVITY AT POSITION | | | | |
| 5.18 | ALTER | REPLACE ACTIVITY | | | | |
| 5.19 | ALTER | REPLACE ACTIVITY AT POSITION | | | | |
| 5.20 | ALTER | INSERT OUTCOME | | | | |
| 5.21 | ALTER | DROP OUTCOME | | | | |
| 5.22 | ALTER | INSERT PATH | | | | |
| 5.23 | ALTER | DROP PATH | | | | |
| 5.24 | ALTER | INSERT BRANCH | | | | |
| 5.25 | ALTER | DROP BRANCH | | | | |
| 5.26 | ALTER | INSERT BOUNDARY EVENT | | | | |
| 5.27 | ALTER | DROP BOUNDARY EVENT | | | | |
| 5.28 | ALTER | INSERT non-interrupting BE | | | | |
| 6.1 | ROUNDTRIP | Simple | | | | |
| 6.2 | ROUNDTRIP | Complex | | | | |
| 7.1 | MULTI-STEP | Add decisions | | | | |
| 7.2 | MULTI-STEP | Add boundary events | | | | |
| 7.3 | MULTI-STEP | Full verify | | | | |
| 8.1 | FAILURE | Not connected | | | | |
| 8.2 | FAILURE | Workflow not found | | | | |
| 8.3 | FAILURE | Feature gate (pre-9.12) | | | | |
| 8.4 | FAILURE | Activity caption not found | | | | |
| 8.5 | FAILURE | Ambiguous caption | | | | |
| 8.6 | FAILURE | Invalid AT POSITION | | | | |
| 8.7 | FAILURE | DROP last outcome | | | | |
| 8.8 | FAILURE | DROP last path | | | | |
| 8.9 | FAILURE | INSERT AFTER non-existent | | | | |
| 8.10 | FAILURE | Bad parameter entity | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
