# Workflow Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/386)

## Setup

> See [AGENT-TESTING.md](./AGENT-TESTING.md) for build, execution methods, and verification patterns.

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
  parameter $WorkflowContext: MyModule.Request
  display 'Approval Workflow'
  description 'Handles approval requests'
  export level Hidden
  overview page MyModule.WorkflowOverview
  due date '7d'
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
  user task ut1 'Review Request'
    page MyModule.ReviewPage
    description 'Review the submitted request'
    targeting microflow MyModule.GetReviewers
    due date '3d'
  outcomes
    'Approve' { }
    'Reject' { }
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

### 2.7 Jump to activity

```
describe workflow <Module.WorkflowWithJump>;
```

**Expected:** `jump to 'ActivityCaption';` in body.

> **Verify:** Actual describe output may differ — `delay` keyword may not appear; activity ref may need IDENTIFIER not string.

### 2.8 Wait for timer

```
describe workflow <Module.WorkflowWithTimer>;
```

**Expected:** `wait for timer delay '2d';` or similar duration expression.

> **Verify:** Actual describe output may differ — `delay` keyword may not appear; activity ref may need IDENTIFIER not string.

### 2.9 Wait for notification

```
describe workflow <Module.WorkflowWithNotification>;
```

**Expected:** `wait for notification;` in body.

### 2.10 Annotation

```
describe workflow <Module.AnnotatedWorkflow>;
```

**Expected:** `@annotation 'text'` on activities or at workflow level.

### 2.11 Outcome blocks with nested activities

```
describe workflow <Module.WorkflowWithNestedOutcomes>;
```

**Expected:** Each outcome contains one or more activities. Nesting preserved in indentation.

### 2.12 Non-existent workflow

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
  parameter $WorkflowContext: MyModule.Request
begin
end workflow;
```

**Expected:** Created. `show workflows in MyModule` includes it. Implicit Start and End activities generated.

### 3.2 Implicit Start/End auto-behavior

```
create workflow MyModule.WF_Implicit
  parameter $WorkflowContext: MyModule.Request
begin
  call microflow MyModule.DoSomething;
end workflow;
```

**Expected:** `describe` shows Start and End activities implicitly. No need to declare them.

### 3.3 Parameter auto-binding

```
create workflow MyModule.WF_AutoBind
  parameter $WorkflowContext: MyModule.Request
begin
  user task ut1 'Review'
    page MyModule.ReviewPage
  outcomes
    'Done' { }
  end task;
end workflow;
```

**Expected:** Parameter entity `MyModule.Request` auto-binds as `$WorkflowContext`. Accessible in expressions and activity targeting.

### 3.4 Duplicate name deduplication (CE0495)

```
create workflow MyModule.WF_DupNames
  parameter $WorkflowContext: MyModule.Request
begin
  call microflow MyModule.Step1;
  call microflow MyModule.Step1;
end workflow;
```

**Expected:** Compiler event CE0495 — duplicate activity caption. Second activity receives deduplicated caption (e.g., `Step1 2`).

### 3.5 Duplicate workflow (without modifier)

```
create workflow MyModule.WF_Simple
  parameter $WorkflowContext: MyModule.Request
begin
end workflow;
```

**Expected:** Error — workflow already exists.

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

### 5.3 SET export level property

```
alter workflow MyModule.WF_Full
  set export level Public;
```

**Expected:** `describe` shows `export level Public`.

### 5.4 SET due date property

```
alter workflow MyModule.WF_Full
  set due date '30d';
```

**Expected:** `describe` shows updated due date.

### 5.5 SET overview page property

```
alter workflow MyModule.WF_Full
  set overview page MyModule.NewOverviewPage;
```

**Expected:** `describe` shows new overview page.

### 5.6 SET parameter property

```
alter workflow MyModule.WF_Full
  set parameter $WorkflowContext: MyModule.NewParameterEntity;
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

### 5.9 SET ACTIVITY targeting microflow

```
alter workflow MyModule.WF_Full
  set activity 'Review' targeting microflow MyModule.NewTargeting;
```

**Expected:** Targeting microflow updated.

### 5.10 SET ACTIVITY targeting xpath

```
alter workflow MyModule.WF_Full
  set activity 'Review' targeting xpath '[Status = ''Active'']';
```

**Expected:** Targeting XPath updated.

### 5.11 SET ACTIVITY due date

```
alter workflow MyModule.WF_Full
  set activity 'Review' due date '5d';
```

**Expected:** Activity due date updated.

### 5.12 SET ACTIVITY with @N for disambiguation

```
alter workflow MyModule.WF_DupNames
  set activity 'Step1' @2 description 'Second step';
```

**Expected:** Only the second `Step1` activity updated.

### 5.13 INSERT AFTER activity

```
alter workflow MyModule.WF_Full
  insert after 'Review'
    call microflow MyModule.PostReviewAction;
```

**Expected:** New activity inserted after `Review`. `describe` shows correct ordering.

### 5.14 INSERT AFTER with @N

```
alter workflow MyModule.WF_DupNames
  insert after 'Step1' @1
    call microflow MyModule.AfterFirstStep;
```

**Expected:** Activity inserted after the first `Step1` only.

### 5.15 INSERT AFTER with full activity definition

```
alter workflow MyModule.WF_Full
  insert after 'Review'
    user task ut1 'Secondary Review'
      page MyModule.SecondaryPage
      due date '2d'
    outcomes
      'Approve' { }
      'Reject' { }
    end task;
```

**Expected:** Full user task definition inserted.

### 5.16 DROP ACTIVITY

```
alter workflow MyModule.WF_Full
  drop activity 'Secondary Review';
```

**Expected:** Activity removed. `describe` no longer shows it.

### 5.17 DROP ACTIVITY with @N

```
alter workflow MyModule.WF_DupNames
  drop activity 'Step1' @2;
```

**Expected:** Only the second `Step1` removed.

### 5.18 REPLACE ACTIVITY

```
alter workflow MyModule.WF_Full
  replace activity 'Review' with
    user task ut1 'Enhanced Review'
      page MyModule.EnhancedPage
      targeting microflow MyModule.NewTargeting
      due date '4d'
    outcomes
      'Approve' { }
      'Reject' { }
      'Escalate' { }
    end task;
```

**Expected:** Old activity replaced. `describe` shows `Enhanced Review` with three outcomes.

### 5.19 REPLACE ACTIVITY with @N

```
alter workflow MyModule.WF_DupNames
  replace activity 'Step1' @1 with
    call microflow MyModule.ReplacedStep;
```

**Expected:** Only the first `Step1` replaced.

### 5.20 INSERT OUTCOME on user task

```
alter workflow MyModule.WF_Full
  insert outcome 'Escalate' on 'Enhanced Review' { };
```

**Expected:** New outcome added. `describe` shows three outcomes.

### 5.21 DROP OUTCOME on user task

```
alter workflow MyModule.WF_Full
  drop outcome 'Escalate' on 'Enhanced Review';
```

**Expected:** Outcome removed. `describe` shows two outcomes.

### 5.22 INSERT BOUNDARY EVENT

```
alter workflow MyModule.WF_Full
  insert boundary event on activity 'Enhanced Review'
    interrupting timer delay '7d'
      call microflow MyModule.OnEscalation;
```

**Expected:** Boundary event added to user task.

### 5.23 DROP BOUNDARY EVENT

```
alter workflow MyModule.WF_Full
  drop boundary event interrupting timer on activity 'Enhanced Review';
```

**Expected:** Boundary event removed.

### 5.24 INSERT non-interrupting BOUNDARY EVENT

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

> **Known issue:** Roundtrip loses activity captions — `-- Comment` style captions revert to the microflow name after DROP + re-CREATE.

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

**Expected:** Error — ambiguous caption. Use `@N` to disambiguate.

### 8.6 Invalid @N index

```
alter workflow MyModule.WF_DupNames
  set activity 'Step1' @99 description 'Out of range';
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
  parameter $WorkflowContext: NonExistent.Entity
begin
end workflow;
```

**Expected:** Error — entity not found.

> **Known issue:** Non-existent parameter entity is currently accepted without validation. No error is raised.

---

## Activity Coverage Table

| Activity Type | Test Case | Section |
|---------------|-----------|---------|
| user task | 2.2, 3.3 | DESCRIBE, CREATE |
| multi user task | 2.3 | DESCRIBE |
| call microflow (no params) | 2.4 | DESCRIBE |
| call microflow (with params) | 2.5 | DESCRIBE |
| call workflow | 2.6 | DESCRIBE |
| jump to | 2.7 | DESCRIBE |
| wait for timer | 2.8 | DESCRIBE |
| wait for notification | 2.9 | DESCRIBE |
| annotation | 2.10 | DESCRIBE |

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
| 2.7 | DESCRIBE | Jump to | | | | |
| 2.8 | DESCRIBE | Wait for timer | | | | |
| 2.9 | DESCRIBE | Wait for notification | | | | |
| 2.10 | DESCRIBE | Annotation | | | | |
| 2.11 | DESCRIBE | Nested outcomes | | | | |
| 2.12 | DESCRIBE | Not found | | | | |
| 3.1 | CREATE | Minimal | | | | |
| 3.2 | CREATE | Implicit Start/End | | | | |
| 3.3 | CREATE | Parameter auto-binding | | | | |
| 3.4 | CREATE | Duplicate dedup (CE0495) | | | | |
| 3.5 | CREATE | Duplicate error | | | | |
| 4.1 | DROP | Existing | | | | |
| 4.2 | DROP | Non-existent | | | | |
| 5.1 | ALTER | SET display | | | | |
| 5.2 | ALTER | SET description | | | | |
| 5.3 | ALTER | SET export level | | | | |
| 5.4 | ALTER | SET due date | | | | |
| 5.5 | ALTER | SET overview page | | | | |
| 5.6 | ALTER | SET parameter | | | | |
| 5.7 | ALTER | SET ACTIVITY page | | | | |
| 5.8 | ALTER | SET ACTIVITY description | | | | |
| 5.9 | ALTER | SET ACTIVITY targeting_mf | | | | |
| 5.10 | ALTER | SET ACTIVITY targeting xpath | | | | |
| 5.11 | ALTER | SET ACTIVITY due date | | | | |
| 5.12 | ALTER | SET ACTIVITY @N | | | | |
| 5.13 | ALTER | INSERT AFTER | | | | |
| 5.14 | ALTER | INSERT AFTER @N | | | | |
| 5.15 | ALTER | INSERT full activity def | | | | |
| 5.16 | ALTER | DROP ACTIVITY | | | | |
| 5.17 | ALTER | DROP ACTIVITY @N | | | | |
| 5.18 | ALTER | REPLACE ACTIVITY | | | | |
| 5.19 | ALTER | REPLACE ACTIVITY @N | | | | |
| 5.20 | ALTER | INSERT OUTCOME | | | | |
| 5.21 | ALTER | DROP OUTCOME | | | | |
| 5.22 | ALTER | INSERT BOUNDARY EVENT | | | | |
| 5.23 | ALTER | DROP BOUNDARY EVENT | | | | |
| 5.24 | ALTER | INSERT non-interrupting BE | | | | |
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
| 8.6 | FAILURE | Invalid @N | | | | |
| 8.7 | FAILURE | DROP last outcome | | | | |
| 8.8 | FAILURE | DROP last path | | | | |
| 8.9 | FAILURE | INSERT AFTER non-existent | | | | |
| 8.10 | FAILURE | Bad parameter entity | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
