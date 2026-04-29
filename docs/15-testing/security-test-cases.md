# Security Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/301)

## Test Projects

Demo apps from [Mendix App Gallery](https://appgallery.mendixcloud.com/):

| App | Studio Pro | Module Roles | User Roles | Demo Users |
|-----|-----------|--------------|------------|------------|
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

### 3. Interactive testing

```bash
mxcli repl -p <path-to-app>/EnquiriesManagement.mpr
```

> **IMPORTANT:** Always run destructive tests against a **copy** of the project folder.
> Security changes (role creation, grant/revoke) modify the `.mpr` in place.
>
> ```bash
> cp -r MyProject MyProject-test
> mxcli repl -p MyProject-test/MyProject.mpr
> ```

---

## 1. SHOW PROJECT SECURITY

### 1.1 View project security settings

```
show project security;
```

**Expected:** Displays SecurityLevel, CheckSecurity, StrictMode, DemoUsersEnabled, GuestAccess, UserRoles count, DemoUsers count, AdminUser, PasswordPolicy.

---

## 2. SHOW MODULE ROLES

### 2.1 List all module roles

```
show module roles;
```

**Expected:** Table with columns `Qualified Name | Module | Role | Description`. Summary `(N module roles)`.

### 2.2 List module roles in a module

```
show module roles in Administration;
```

**Expected:** Only roles from `Administration` module.

---

## 3. SHOW USER ROLES

### 3.1 List all user roles

```
show user roles;
```

**Expected:** Table with columns `Name | Module Roles | Manage All | Check Security`. Summary `(N user roles)`.

---

## 4. SHOW DEMO USERS

### 4.1 List demo users

```
show demo users;
```

**Expected:** Table with columns `User Name | User Roles`.

### 4.2 Demo users disabled

If demo users are disabled:

**Expected:** Message "Demo users are disabled" with hint `alter project security demo users on;`.

---

## 5. SHOW ACCESS ON ENTITY

### 5.1 Entity with access rules

```
show access on Administration.Account;
```

**Expected:** Per-rule output: rule number, roles, rights (create/read/write/delete), default member access, per-member overrides, XPath constraint.

### 5.2 Entity without access rules

```
show access on MyModule.NewEntity;
```

**Expected:** Empty or "No access rules" message.

### 5.3 Missing entity name

```
show access on;
```

**Expected:** Error — entity name required.

### 5.4 Non-existent entity

```
show access on Fake.Entity;
```

**Expected:** Error — entity not found.

---

## 6. SHOW ACCESS ON MICROFLOW/PAGE/NANOFLOW

### 6.1 Microflow access

```
show access on microflow Administration.ACT_CreateAccount;
```

**Expected:** List of allowed module roles. Or "No module roles granted execute access".

### 6.2 Page access

```
show access on page Administration.AccountOverview;
```

**Expected:** List of allowed module roles.

### 6.3 Nanoflow access

```
show access on nanoflow <Module.NF>;
```

**Expected:** Same pattern as microflow.

### 6.4 Workflow access (unsupported)

```
show access on workflow <Module.WF>;
```

**Expected:** Error — workflows do not have document-level AllowedModuleRoles.

---

## 7. SHOW SECURITY MATRIX

### 7.1 Full security matrix

```
show security matrix;
```

**Expected:** Sections: Entity Access (role: CRWD), Microflow Access, Page Access, Workflow Access.

### 7.2 Module-specific matrix

```
show security matrix in Administration;
```

**Expected:** Only security rules for `Administration` module.

---

## 8. DESCRIBE MODULE ROLE

### 8.1 Describe existing role

```
describe module role Administration.Administrator;
```

**Expected:** DDL-like output `create module role ...` + which user roles include it.

### 8.2 Non-existent role

```
describe module role Fake.Role;
```

**Expected:** Error — not found.

---

## 9. DESCRIBE USER ROLE

### 9.1 Describe user role

```
describe user role Administrator;
```

**Expected:** `create user role Administrator (ModuleRoles) [manage all roles];` + description.

---

## 10. DESCRIBE DEMO USER

### 10.1 Describe demo user

```
describe demo user 'admin';
```

**Expected:** `create demo user 'admin' password '***' [entity X] (Roles);`

---

## 11. CREATE MODULE ROLE

### 11.1 Basic role creation

```
create module role MyModule.Viewer;
```

**Expected:** `Created module role: MyModule.Viewer`.

### 11.2 Role with description

```
create module role MyModule.Admin description 'Full administrative access';
```

**Expected:** Role created with description.

### 11.3 Duplicate role

```
create module role MyModule.Viewer;
```

**Expected:** Error — already exists.

### 11.4 Case-insensitive collision

```
create module role MyModule.VIEWER;
```

**Expected:** Error — collision with existing `Viewer` (case-insensitive check).

---

## 12. DROP MODULE ROLE

### 12.1 Drop role

```
drop module role MyModule.Viewer;
```

**Expected:** `Dropped module role: MyModule.Viewer`. Cascade removes role from entity access rules, microflow/page/OData allowed roles, and user roles.

### 12.2 Drop role with cascading output

Create role, grant access, then drop. Verify cascade messages: `Removed X from N ...`.

### 12.3 Drop non-existent role

```
drop module role MyModule.Fake;
```

**Expected:** Error — not found.

---

## 13. CREATE USER ROLE

### 13.1 Basic user role

```
create user role PowerUser (MyModule.Admin, Administration.Administrator);
```

**Expected:** `Created user role: PowerUser`.

### 13.2 User role with manage all

```
create user role SuperAdmin (System.Administrator) manage all roles;
```

**Expected:** User role created with ManageAllRoles flag.

### 13.3 `create or modify` — additive merge

```
create or modify user role PowerUser (MyModule.Viewer);
```

**Expected:** `Modified user role: PowerUser` — adds `MyModule.Viewer` to existing roles.

### 13.4 Duplicate (without `or modify`)

```
create user role PowerUser (MyModule.Admin);
```

**Expected:** Error — already exists.

---

## 14. ALTER USER ROLE

### 14.1 Add module roles

```
alter user role PowerUser add module roles (MyModule.Viewer);
```

**Expected:** Role added.

### 14.2 Remove module roles

```
alter user role PowerUser remove module roles (MyModule.Viewer);
```

**Expected:** Role removed.

### 14.3 Non-existent user role

```
alter user role FakeRole add module roles (MyModule.Admin);
```

**Expected:** Error — user role not found.

---

## 15. DROP USER ROLE

### 15.1 Drop user role

```
drop user role PowerUser;
```

**Expected:** `Dropped user role: PowerUser`.

### 15.2 Drop non-existent

```
drop user role FakeRole;
```

**Expected:** Error — not found.

---

## 16. GRANT ON ENTITY

### 16.1 Full CRUD access

```
grant MyModule.Admin on MyModule.Customer (create, delete, read *, write *);
```

**Expected:** Access rule created with all rights.

### 16.2 Read-only access

```
grant MyModule.Viewer on MyModule.Customer (read *);
```

**Expected:** Read-only rule (no create/delete/write).

### 16.3 Selective member access

```
grant MyModule.User on MyModule.Customer (read (Name, Email), write (Email));
```

**Expected:** Only specified members accessible.

### 16.4 With XPath constraint

```
grant MyModule.User on MyModule.Order (read *, write *) where '[Status = ''Open'']';
```

**Expected:** Rule with XPath constraint.

### 16.5 Multiple roles

```
grant MyModule.User, MyModule.Admin on MyModule.Product (read *);
```

**Expected:** Both roles granted in same rule.

---

## 17. REVOKE ON ENTITY

### 17.1 Full revoke

```
revoke MyModule.Viewer on MyModule.Customer;
```

**Expected:** Entire access rule removed.

### 17.2 Partial revoke (rights)

```
revoke MyModule.User on MyModule.Customer (write (Email));
```

**Expected:** Write access on Email downgraded; other rights preserved.

### 17.3 Revoke non-existent rule

```
revoke MyModule.Viewer on MyModule.UnrulledEntity;
```

**Expected:** "No access rules found matching".

---

## 18. GRANT/REVOKE EXECUTE ON MICROFLOW

### 18.1 Grant execute

```
grant execute on microflow MyModule.MF_Create to MyModule.User, MyModule.Admin;
```

**Expected:** `Granted execute access on ...`.

### 18.2 Idempotent grant

```
grant execute on microflow MyModule.MF_Create to MyModule.User;
```

**Expected:** "All specified roles already have execute access".

### 18.3 Revoke execute

```
revoke execute on microflow MyModule.MF_Create from MyModule.User;
```

**Expected:** `Revoked execute access on ...`.

### 18.4 Revoke non-granted role

```
revoke execute on microflow MyModule.MF_Create from MyModule.Viewer;
```

**Expected:** "None of the specified roles had execute access".

---

## 19. GRANT/REVOKE VIEW ON PAGE

### 19.1 Grant view

```
grant view on page MyModule.Overview to MyModule.User;
```

**Expected:** `Granted view access on ...`.

### 19.2 Revoke view

```
revoke view on page MyModule.Overview from MyModule.User;
```

**Expected:** `Revoked view access on ...`.

---

## 20. CREATE DEMO USER

### 20.1 Basic demo user

```
create demo user 'testuser' password 'Test1234!!' (RegularUser);
```

**Expected:** Demo user created.

### 20.2 `create or modify` — update password and roles

```
create or modify demo user 'testuser' password 'NewPass1234!!' (RegularUser, PowerUser);
```

**Expected:** Password updated, roles merged.

### 20.3 Invalid password (policy violation)

```
create demo user 'weak' password '123' (RegularUser);
```

**Expected:** Error — password policy violation.

### 20.4 Demo users disabled

If demo users are off, attempting to create:

**Expected:** Error or auto-enable suggestion.

---

## 21. DROP DEMO USER

### 21.1 Drop existing

```
drop demo user 'testuser';
```

**Expected:** Demo user removed.

---

## 22. ALTER PROJECT SECURITY

### 22.1 Change security level

```
alter project security level Production;
```

**Expected:** Security level updated.

### 22.2 Enable demo users

```
alter project security demo users on;
```

**Expected:** Demo users enabled.

### 22.3 Disable demo users

```
alter project security demo users off;
```

**Expected:** Demo users disabled.

---

## 23. UPDATE SECURITY

### 23.1 Reconcile all modules

```
update security;
```

**Expected:** Reconciles MemberAccesses on all entity access rules (fixes CE0066).

### 23.2 Reconcile specific module

```
update security in MyModule;
```

**Expected:** Only MyModule access rules reconciled.

---

## 24. MULTI-STEP WORKFLOWS

### 24.1 Full security setup

```
create module role MyModule.Admin description 'Admin role';
create module role MyModule.User description 'Regular user';
create user role Administrator (MyModule.Admin) manage all roles;
create user role RegularUser (MyModule.User);
create demo user 'admin' password 'Admin1234!!' (Administrator);
create demo user 'user1' password 'User1234!!' (RegularUser);

create persistent entity MyModule.Task (Title: string(200) not null);
grant MyModule.Admin on MyModule.Task (create, delete, read *, write *);
grant MyModule.User on MyModule.Task (read *, write (Title));

create microflow MyModule.MF_CreateTask ($Title: String) returns Void
begin
  $T = create MyModule.Task (Title = $Title);
  commit $T;
end;
grant execute on microflow MyModule.MF_CreateTask to MyModule.Admin;
```

**Expected:** Complete security setup. Matrix shows correct permissions.

---

## 25. FAILURE MODES & ERROR RECOVERY

### 25.1 Grant on non-existent entity

```
grant MyModule.Admin on MyModule.Fake (read *);
```

**Expected:** Error — entity not found.

### 25.2 Grant non-existent role

```
grant MyModule.FakeRole on MyModule.Customer (read *);
```

**Expected:** Error — module role not found.

### 25.3 Grant execute on non-existent microflow

```
grant execute on microflow MyModule.Fake to MyModule.Admin;
```

**Expected:** Error — microflow not found.

---

## 26. ROUNDTRIP

### 26.1 Module role roundtrip

1. `create module role SecTest.Auditor description 'Can view audit logs';`
2. `describe module role SecTest.Auditor;` — save output
3. `drop module role SecTest.Auditor;`
4. Execute saved output from step 2
5. `describe module role SecTest.Auditor;`

**Expected:** Output at step 5 matches step 2.

### 26.2 User role roundtrip

1. `create user role Reviewer (SecTest.Viewer);`
2. `describe user role Reviewer;` — save output
3. `drop user role Reviewer;`
4. Execute saved output from step 2
5. `describe user role Reviewer;`

**Expected:** Output at step 5 matches step 2.

### 26.3 Demo user roundtrip

1. `create demo user 'rt_admin' password 'Admin12345!!' (SuperAdmin);`
2. `describe demo user 'rt_admin';` — save output
3. `drop demo user 'rt_admin';`
4. Execute saved output from step 2 (adjust password to meet policy)
5. `describe demo user 'rt_admin';`

**Expected:** Output at step 5 matches step 2 (password masked as `***`).

---

## 27. BOUNDARY & STRESS

### 27.1 Many module roles

Create 50 module roles:

```
create module role SecTest.Role01;
create module role SecTest.Role02;
...
create module role SecTest.Role50;
```

**Expected:** All 50 created. `show module roles in SecTest` lists 50 roles.

### 27.2 Many access rules on one entity

Apply 20 different role grants to a single entity:

```
grant SecTest.Role01 on SecTest.Customer (read *);
grant SecTest.Role02 on SecTest.Customer (read (Name));
...
grant SecTest.Role20 on SecTest.Customer (read *, write *);
```

**Expected:** All 20 rules stored. `show access on SecTest.Customer` lists all.

### 27.3 Long XPath constraint

```
grant SecTest.User on SecTest.Order (read *, write *)
  where '[Status = ''Open'' and (Priority = ''High'' or Priority = ''Medium'') and CreatedDate > ''[%BeginOfCurrentDay%]'' and AssignedTo = ''[%CurrentUser%]'']';
```

**Expected:** XPath stored and displayed correctly in `show access`.

### 27.4 Drop role with cascading across many entities

Create a role, grant on 10 entities, then drop:

```
drop module role SecTest.CascadeTest;
```

**Expected:** All 10 entity access rules cleaned up. Output shows removal counts.

### 27.5 User role with many module roles

```
create user role MegaUser (SecTest.Role01, SecTest.Role02, ... SecTest.Role50);
```

**Expected:** User role created with all 50 module roles assigned.

---

## Test Project Coverage Matrix

| Operation | Lato Enquiry | Evora Factory | Lato Product |
|-----------|:---:|:---:|:---:|
| SHOW PROJECT SECURITY | x | x | x |
| SHOW MODULE ROLES | x | x | x |
| SHOW USER ROLES | x | x | x |
| SHOW DEMO USERS | x | x | |
| SHOW ACCESS | x | x | x |
| SHOW SECURITY MATRIX | x | x | |
| DESCRIBE ROLES | x | x | |
| CREATE MODULE ROLE | x | | |
| DROP MODULE ROLE | x | | |
| CREATE USER ROLE | x | | |
| ALTER USER ROLE | x | | |
| DROP USER ROLE | x | | |
| GRANT ON ENTITY | x | | |
| REVOKE ON ENTITY | x | | |
| GRANT/REVOKE EXECUTE | x | | |
| GRANT/REVOKE VIEW | x | | |
| CREATE DEMO USER | x | | |
| DROP DEMO USER | x | | |
| ALTER PROJECT SECURITY | x | | |
| UPDATE SECURITY | x | | |
| ROUNDTRIP | x | | |
| BOUNDARY & STRESS | x | | |

---

## Automated Test Coverage

| Section | Automated | Manual-only |
|---------|:---------:|:-----------:|
| 1–7. SHOW commands | Mock tests | |
| 8–10. DESCRIBE | Mock tests | |
| 11. CREATE MODULE ROLE | Mock tests | Case collision |
| 12. DROP MODULE ROLE | Mock tests | Cascade verification |
| 13. CREATE USER ROLE | Mock tests | |
| 14. ALTER USER ROLE | Mock tests | |
| 15. DROP USER ROLE | Mock tests | |
| 16. GRANT ON ENTITY | Mock tests | Selective members, XPath |
| 17. REVOKE ON ENTITY | Mock tests | Partial revoke |
| 18. GRANT/REVOKE EXECUTE | Mock tests | Idempotent |
| 19. GRANT/REVOKE VIEW | Mock tests | |
| 20–21. DEMO USERS | Mock tests | Password policy |
| 22. ALTER PROJECT | Mock tests | |
| 23. UPDATE SECURITY | Mock tests | |
| 24. Multi-step | | All manual |
| 25. Failure modes | Partial | |
| 26. Roundtrip | | All manual |
| 27. Boundary & stress | | All manual |

---

## Manual Test Report Template

**Tester:** _______________
**Date:** _______________
**Project:** _______________

| # | Section | Test | Pass | Fail | Skip | Notes |
|---|---------|------|:----:|:----:|:----:|-------|
| 1.1 | PROJECT SEC | Show settings | | | | |
| 2.1 | MODULE ROLES | List all | | | | |
| 2.2 | MODULE ROLES | Filter module | | | | |
| 3.1 | USER ROLES | List all | | | | |
| 4.1 | DEMO USERS | List | | | | |
| 4.2 | DEMO USERS | Disabled | | | | |
| 5.1 | ACCESS ENTITY | With rules | | | | |
| 5.2 | ACCESS ENTITY | No rules | | | | |
| 5.3 | ACCESS ENTITY | Missing name | | | | |
| 5.4 | ACCESS ENTITY | Not found | | | | |
| 6.1 | ACCESS MF | Microflow | | | | |
| 6.2 | ACCESS PAGE | Page | | | | |
| 6.3 | ACCESS NF | Nanoflow | | | | |
| 6.4 | ACCESS WF | Workflow (unsupported) | | | | |
| 7.1 | MATRIX | Full | | | | |
| 7.2 | MATRIX | Module filter | | | | |
| 8.1 | DESC ROLE | Module role | | | | |
| 8.2 | DESC ROLE | Not found | | | | |
| 9.1 | DESC USER ROLE | Describe | | | | |
| 10.1 | DESC DEMO | Describe | | | | |
| 11.1 | CREATE ROLE | Basic | | | | |
| 11.2 | CREATE ROLE | With description | | | | |
| 11.3 | CREATE ROLE | Duplicate | | | | |
| 11.4 | CREATE ROLE | Case collision | | | | |
| 12.1 | DROP ROLE | Basic | | | | |
| 12.2 | DROP ROLE | Cascade | | | | |
| 12.3 | DROP ROLE | Not found | | | | |
| 13.1 | CREATE USER | Basic | | | | |
| 13.2 | CREATE USER | Manage all | | | | |
| 13.3 | CREATE USER | Or modify | | | | |
| 13.4 | CREATE USER | Duplicate | | | | |
| 14.1 | ALTER USER | Add roles | | | | |
| 14.2 | ALTER USER | Remove roles | | | | |
| 14.3 | ALTER USER | Not found | | | | |
| 15.1 | DROP USER | Basic | | | | |
| 15.2 | DROP USER | Not found | | | | |
| 16.1 | GRANT ENTITY | Full CRUD | | | | |
| 16.2 | GRANT ENTITY | Read-only | | | | |
| 16.3 | GRANT ENTITY | Selective members | | | | |
| 16.4 | GRANT ENTITY | XPath | | | | |
| 16.5 | GRANT ENTITY | Multiple roles | | | | |
| 17.1 | REVOKE ENTITY | Full revoke | | | | |
| 17.2 | REVOKE ENTITY | Partial | | | | |
| 17.3 | REVOKE ENTITY | No match | | | | |
| 18.1 | GRANT EXEC | Microflow | | | | |
| 18.2 | GRANT EXEC | Idempotent | | | | |
| 18.3 | REVOKE EXEC | Microflow | | | | |
| 18.4 | REVOKE EXEC | Non-granted | | | | |
| 19.1 | GRANT VIEW | Page | | | | |
| 19.2 | REVOKE VIEW | Page | | | | |
| 20.1 | DEMO USER | Create | | | | |
| 20.2 | DEMO USER | Or modify | | | | |
| 20.3 | DEMO USER | Bad password | | | | |
| 20.4 | DEMO USER | Disabled | | | | |
| 21.1 | DROP DEMO | Existing | | | | |
| 22.1 | ALTER SEC | Level | | | | |
| 22.2 | ALTER SEC | Demo on | | | | |
| 22.3 | ALTER SEC | Demo off | | | | |
| 23.1 | UPDATE SEC | All | | | | |
| 23.2 | UPDATE SEC | Module | | | | |
| 24.1 | MULTI-STEP | Full setup | | | | |
| 25.1 | FAILURE | Bad entity | | | | |
| 25.2 | FAILURE | Bad role | | | | |
| 25.3 | FAILURE | Bad microflow | | | | |
| 26.1 | ROUNDTRIP | Module role | | | | |
| 26.2 | ROUNDTRIP | User role | | | | |
| 26.3 | ROUNDTRIP | Demo user | | | | |
| 27.1 | BOUNDARY | Many module roles | | | | |
| 27.2 | BOUNDARY | Many access rules | | | | |
| 27.3 | BOUNDARY | Long XPath | | | | |
| 27.4 | BOUNDARY | Cascade drop | | | | |
| 27.5 | BOUNDARY | User role many roles | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
