# Enumeration Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/301)

## Test Projects

Demo apps from [Mendix App Gallery](https://appgallery.mendixcloud.com/):

| App | Studio Pro | Enumerations |
|-----|-----------|--------------|
| Lato Enquiry Management | 11.4.0 | <N> |
| Evora - Factory Management | 10.24.15 | <N> |
| Lato Product Inventory | 11.2.0 | <N> |

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
  echo "show enumerations;" > /tmp/show-enum.mdl
  mxcli exec /tmp/show-enum.mdl -p "$mpr" 2>&1 | tail -1
done
```

Expected: count line `(N enumerations)` for each project.

### 4. Interactive testing

```bash
mxcli repl -p <path-to-app>/EnquiriesManagement.mpr
```

### 5. Script-based testing

```bash
mxcli exec test-sequence.mdl -p <mpr>
```

> **IMPORTANT:** Always run destructive tests against a **copy** of the project folder.
> Dropped enumerations cannot be recovered.
>
> ```bash
> cp -r MyProject MyProject-test
> mxcli repl -p MyProject-test/MyProject.mpr
> ```

---

## 1. SHOW ENUMERATIONS

### 1.1 List all enumerations

```
show enumerations;
```

**Expected:** Table with columns `Qualified Name | Module | Name | Folder | Values`. Summary `(N enumerations)`. Sorted alphabetically.

### 1.2 List enumerations in a module

```
show enumerations in Administration;
```

**Expected:** Only enumerations from `Administration` module.

### 1.3 Empty module

```
show enumerations in NonExistentModule;
```

**Expected:** Empty result or `(0 enumerations)`.

---

## 2. DESCRIBE ENUMERATION

### 2.1 Basic enumeration

```
describe enumeration MyModule.OrderStatus;
```

**Expected:**
```
create or modify enumeration MyModule.OrderStatus (
  PENDING 'Pending',
  PROCESSING 'Processing',
  SHIPPED 'Shipped',
  DELIVERED 'Delivered',
  CANCELLED 'Cancelled'
);
/
```

### 2.2 Enumeration with documentation

```
describe enumeration MyModule.DocumentedEnum;
```

**Expected:** Output starts with `/** ... */` documentation block.

### 2.3 Enumeration with value documentation

```
describe enumeration MyModule.OrderStatus;
```

**Expected:** Per-value doc comments: `/** Order received, awaiting processing */` above value.

### 2.4 Non-existent enumeration

```
describe enumeration Fake.Missing;
```

**Expected:** Error — enumeration not found.

---

## 3. CREATE ENUMERATION

### 3.1 Simple enumeration

```
create enumeration MyModule.Color (
  RED 'Red',
  GREEN 'Green',
  BLUE 'Blue'
);
```

**Expected:** `Created enumeration: MyModule.Color`. `show enumerations in MyModule` includes it with `Values: 3`.

### 3.2 Enumeration with documentation

```
/** Priority levels for task ordering */
create enumeration MyModule.Priority (
  /** Highest urgency */
  CRITICAL 'Critical',
  HIGH 'High',
  MEDIUM 'Medium',
  LOW 'Low'
);
```

**Expected:** Created. `describe` shows documentation blocks.

### 3.3 Single-value enumeration

```
create enumeration MyModule.YesNo (
  YES 'Yes'
);
```

**Expected:** Created with 1 value.

### 3.4 Quoted identifier (reserved word)

```
create enumeration MyModule.UserType (
  "user" 'Standard User',
  ADMIN 'Administrator'
);
```

**Expected:** Created. Value name `user` (reserved word) accepted when quoted.

### 3.5 Reserved word — unquoted

```
create enumeration MyModule.BadEnum (
  class 'Class Value'
);
```

**Expected:** Error (CE7247) — `enumeration value 'class' is a reserved word`. Suggests renaming to `class_` or `IsClass`.

### 3.6 Other reserved words

Test these unquoted values trigger CE7247:

| Value | Reserved |
|-------|----------|
| `null` | Java keyword |
| `true` | Java keyword |
| `enum` | Java keyword |
| `context` | Mendix reserved |
| `guid` | Mendix reserved |
| `changeddate` | Mendix reserved |
| `currentuser` | Mendix reserved |

### 3.7 `create or modify` — new enumeration

```
create or modify enumeration MyModule.Fresh (
  A 'Alpha',
  B 'Beta'
);
```

**Expected:** Creates enumeration (same as without `or modify`).

### 3.8 `create or modify` — existing enumeration

```
create or modify enumeration MyModule.Color (
  RED 'Red',
  GREEN 'Green',
  BLUE 'Blue',
  YELLOW 'Yellow'
);
```

**Expected:** Replaces existing enumeration. `describe` shows 4 values.

### 3.9 Duplicate (without `or modify`)

```
create enumeration MyModule.Color (
  X 'X'
);
```

**Expected:** Error — `enumeration already exists: MyModule.Color (use create or modify to update)`.

### 3.10 Module auto-creation

```
create enumeration NewModule.Status (
  ACTIVE 'Active'
);
```

**Expected:** Both module and enumeration created.

---

## 4. DROP ENUMERATION

### 4.1 Drop existing enumeration

```
drop enumeration MyModule.Color;
```

**Expected:** `Dropped enumeration: MyModule.Color`.

### 4.2 Drop non-existent enumeration

```
drop enumeration MyModule.Fake;
```

**Expected:** Error — not found.

### 4.3 Drop by name only (no module qualifier)

```
drop enumeration Color;
```

**Expected:** Matches by name if unambiguous. Error if multiple enumerations share name.

---

## 5. ROUNDTRIP (BSON)

### 5.1 Enumeration roundtrip

```
create enumeration RtTest.Status (
  ACTIVE 'Active',
  INACTIVE 'Inactive',
  ARCHIVED 'Archived'
);
```

1. `describe enumeration RtTest.Status`
2. Drop: `drop enumeration RtTest.Status`
3. Execute described MDL (change `create or modify` to `create`)
4. `describe` again

**Expected:** Output identical between step 1 and step 4.

### 5.2 Roundtrip with documentation

```
/** Payment status tracking */
create enumeration RtTest.PayStatus (
  /** Awaiting payment */
  PENDING 'Pending',
  PAID 'Paid',
  REFUNDED 'Refunded'
);
```

1. `describe` → drop → recreate from output → `describe`

**Expected:** Documentation preserved through roundtrip.

---

## 6. MULTI-STEP WORKFLOWS

### 6.1 Create enumeration → use in entity

```
create enumeration MyModule.TaskStatus (
  TODO 'To Do',
  IN_PROGRESS 'In Progress',
  DONE 'Done'
);

create persistent entity MyModule.Task (
  Title: string(200) not null,
  Status: enumeration(MyModule.TaskStatus) default MyModule.TaskStatus.TODO
);
```

**Expected:** Both created. Entity attribute references enumeration. `describe entity` shows `enumeration(MyModule.TaskStatus)`.

### 6.2 Replace enumeration → verify entity still works

```
create or modify enumeration MyModule.TaskStatus (
  TODO 'To Do',
  IN_PROGRESS 'In Progress',
  DONE 'Done',
  BLOCKED 'Blocked'
);

describe entity MyModule.Task;
```

**Expected:** Entity still references `MyModule.TaskStatus`. New value `BLOCKED` available.

---

## 7. FAILURE MODES & ERROR RECOVERY

### 7.1 Empty value list

```
create enumeration MyModule.Empty ();
```

**Expected:** Error — at least one value required (or parser error).

### 7.2 Duplicate value names

```
create enumeration MyModule.Dup (
  A 'First',
  A 'Second'
);
```

**Expected:** Error — duplicate value name.

### 7.3 Drop enumeration used by entity attribute

```
drop enumeration MyModule.TaskStatus;
```

**Expected:** Enumeration dropped (no referential integrity enforcement — may leave entity attribute orphaned). Verify `describe entity MyModule.Task` still works but shows broken reference or generic type.

### 7.4 Case sensitivity in reserved word check

```
create enumeration MyModule.CaseTest (
  NULL 'Null Value',
  True 'True Value'
);
```

**Expected:** Error — reserved word check is case-insensitive.

---

## 8. BOUNDARY & STRESS

### 8.1 Many values (50+)

Create an enumeration with 50 values. Verify `show` and `describe` handle it.

### 8.2 Long captions

```
create enumeration MyModule.LongCaptions (
  VAL1 'This is a very long caption that exceeds what would normally be displayed in a UI dropdown'
);
```

**Expected:** Created and described correctly.

### 8.3 Special characters in captions

```
create enumeration MyModule.Special (
  A 'It''s a "test" value',
  B 'Line1\nLine2'
);
```

**Expected:** Escaped characters preserved in describe output.

---

## Test Project Coverage Matrix

| Operation | Lato Enquiry | Evora Factory | Lato Product |
|-----------|:---:|:---:|:---:|
| SHOW ENUMERATIONS | x | x | x |
| DESCRIBE ENUMERATION | x | x | x |
| CREATE ENUMERATION | x | | |
| DROP ENUMERATION | x | | |

---

## Automated Test Coverage

| Section | Automated | Manual-only |
|---------|:---------:|:-----------:|
| 1. SHOW ENUMERATIONS | Mock tests | |
| 2. DESCRIBE ENUMERATION | Mock tests | |
| 3. CREATE ENUMERATION | Mock tests | Reserved words, quoted identifiers |
| 4. DROP ENUMERATION | Mock tests | |
| 5. Roundtrip | | All manual |
| 6. Multi-step | | All manual |
| 7. Failure modes | Partial (reserved) | Empty list, duplicates |
| 8. Boundary | | All manual |

---

## Manual Test Report Template

**Tester:** _______________
**Date:** _______________
**Project:** _______________

| # | Section | Test | Pass | Fail | Skip | Notes |
|---|---------|------|:----:|:----:|:----:|-------|
| 1.1 | SHOW | List all | | | | |
| 1.2 | SHOW | Filter module | | | | |
| 1.3 | SHOW | Empty module | | | | |
| 2.1 | DESCRIBE | Basic | | | | |
| 2.2 | DESCRIBE | With documentation | | | | |
| 2.3 | DESCRIBE | Value documentation | | | | |
| 2.4 | DESCRIBE | Not found | | | | |
| 3.1 | CREATE | Simple | | | | |
| 3.2 | CREATE | With documentation | | | | |
| 3.3 | CREATE | Single value | | | | |
| 3.4 | CREATE | Quoted reserved word | | | | |
| 3.5 | CREATE | Unquoted reserved word | | | | |
| 3.6 | CREATE | Other reserved words | | | | |
| 3.7 | CREATE | create or modify (new) | | | | |
| 3.8 | CREATE | create or modify (update) | | | | |
| 3.9 | CREATE | Duplicate error | | | | |
| 3.10 | CREATE | Module auto-creation | | | | |
| 4.1 | DROP | Existing | | | | |
| 4.2 | DROP | Non-existent | | | | |
| 4.3 | DROP | By name only | | | | |
| 5.1 | ROUNDTRIP | Basic | | | | |
| 5.2 | ROUNDTRIP | With documentation | | | | |
| 6.1 | MULTI-STEP | Create + use in entity | | | | |
| 6.2 | MULTI-STEP | Replace + verify entity | | | | |
| 7.1 | FAILURE | Empty value list | | | | |
| 7.2 | FAILURE | Duplicate values | | | | |
| 7.3 | FAILURE | Drop used enumeration | | | | |
| 7.4 | FAILURE | Case-insensitive reserved | | | | |
| 8.1 | BOUNDARY | Many values | | | | |
| 8.2 | BOUNDARY | Long captions | | | | |
| 8.3 | BOUNDARY | Special characters | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
