# Navigation & Settings Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/386)

## Setup

> See [AGENT-TESTING.md](./AGENT-TESTING.md) for build, execution methods, and verification patterns.

---

## 1. SHOW NAVIGATION

### 1.1 List all navigation profiles

```
show navigation;
```

**Expected:** Table with columns `Profile | Kind | HomePage | LoginPage | MenuItems | RoleHomes`. Kind shows navigation type with `(native)` suffix for native profiles. Summary `(N navigation profiles)`.

### 1.2 Verify Kind column format

Inspect each row in the output.

**Expected:** Kind values like `Responsive`, `Tablet (native)`, `Phone (native)`. The `(native)` suffix appears only on native navigation profiles.

---

## 2. SHOW NAVIGATION MENU

### 2.1 Menu tree for a profile

```
show navigation menu Responsive;
```

**Expected:** Tree output with indentation showing menu hierarchy. Each leaf shows target: page or microflow qualified name.

### 2.2 Nested menu items

```
show navigation menu Responsive;
```

**Expected:** Nested items indented under parent menu groups. Format:

```
Administration
  Account Overview → page Administration.AccountOverview
  My Account → page Administration.MyAccount
```

### 2.3 Menu with microflow target

If a menu item targets a microflow:

**Expected:** Line shows `-> MF:Module.MicroflowName`.

### 2.4 Non-existent profile

```
show navigation menu FakeProfile;
```

**Expected:** ~~Error — navigation profile not found.~~ Returns empty output with exit code 0 instead of an error. **Known inconsistency.**

---

## 3. SHOW NAVIGATION HOMES

### 3.1 View home pages per profile

```
show navigation homes;
```

**Expected:** Per-profile output showing Default Home page and Role-Based Homes. Each role-based entry shows the role name and target page.

### 3.2 Profile with no role-based homes

If a profile has only a default home:

**Expected:** Default Home listed. Role-Based Homes section empty or absent.

---

## 4. DESCRIBE NAVIGATION

### 4.1 Describe a profile

```
describe navigation Responsive;
```

**Expected:** Full MDL output:

```mdl
create or replace navigation Responsive
  home page Module.HomePage
  login page Module.LoginPage
  not found page Module.NotFoundPage
  menu (
    menu item 'Overview' page Module.Overview;
    menu 'Admin' (
      menu item 'Users' page Administration.AccountOverview;
    );
  );
```

### 4.2 Profile with microflow home

If the home target is a microflow:

**Expected:** MDL shows `home microflow Module.MicroflowName` instead of `home page`.

### 4.3 Profile with role-based homes

**Expected:** MDL includes `home page Module.Page for Role.Name` entries.

### 4.4 Non-existent profile

```
describe navigation FakeProfile;
```

**Expected:** Error — navigation profile not found.

---

## 5. CREATE OR REPLACE NAVIGATION

### 5.1 Replace home page

```
create or replace navigation Responsive
  home page MyModule.NewHome;
```

**Expected:** Home page updated. Verify with `describe navigation Responsive`.

### 5.2 Replace login page

```
create or replace navigation Responsive
  home page MyModule.Home
  login page MyModule.NewLogin;
```

**Expected:** Login page updated. Omitted clauses (menu, not found page) cleared.

### 5.3 Full replacement semantics — omitted clauses clear

```
create or replace navigation Responsive
  home page MyModule.Home;
```

**Expected:** Login page, not found page, ~~and menu all~~ cleared. Verify with `describe navigation Responsive` — only `home page` present.

> **Note:** Menu is **NOT** cleared when omitted from `CREATE OR REPLACE NAVIGATION`. Only login page and role-based homes are cleared. This is a known inconsistency.

### 5.4 Replace menu block

```
create or replace navigation Responsive
  home page MyModule.Home
  menu (
    menu item 'Dashboard' page MyModule.Dashboard;
  );
```

**Expected:** Existing menu replaced entirely. Only `Dashboard` item remains.

### 5.5 Nested menu items

```
create or replace navigation Responsive
  home page MyModule.Home
  menu (
    menu 'Admin' (
      menu item 'Users' page Administration.AccountOverview;
      menu item 'Settings' page Administration.Settings;
    );
    menu item 'Home' page MyModule.Home;
  );
```

**Expected:** Menu contains nested `Admin` group with two children and top-level `Home` item.

### 5.6 Microflow menu target

```
create or replace navigation Responsive
  home page MyModule.Home
  menu (
    menu item 'Run Report' microflow MyModule.MF_RunReport;
  );
```

**Expected:** Menu item targets microflow. Verify with `describe navigation Responsive`.

### 5.7 Role-based homes

```
create or replace navigation Responsive
  home page MyModule.Home
  home page MyModule.AdminHome for Administration.Administrator;
```

**Expected:** Default home and role-based home set. Verify with `show navigation homes`.

### 5.8 Microflow home

```
create or replace navigation Responsive
  home microflow MyModule.MF_Startup;
```

**Expected:** Home targets microflow. Verify with `describe navigation Responsive`.

---

## 6. SHOW SETTINGS

### 6.1 View all settings

```
show settings;
```

**Expected:** Output organized in sections: Model Settings, Configuration, Language Settings, Workflow Settings.

### 6.2 Not connected

Run without an open project:

**Expected:** Error — not connected to a project.

---

## 7. DESCRIBE SETTINGS

### 7.1 Full MDL output

```
describe settings;
```

**Expected:** Full MDL for all settings sections. Includes model properties, configurations with constants, language settings, and workflow settings.

---

## 8. ALTER SETTINGS MODEL

### 8.1 Set AfterStartupMicroflow

```
alter settings model AfterStartupMicroflow = 'MyModule.MF_OnStart';
```

**Expected:** Property updated. Verify with `show settings`.

### 8.2 Set BeforeShutdownMicroflow

```
alter settings model BeforeShutdownMicroflow = 'MyModule.MF_OnShutdown';
```

**Expected:** Property updated.

### 8.3 Set HealthCheckMicroflow

```
alter settings model HealthCheckMicroflow = 'MyModule.MF_HealthCheck';
```

**Expected:** Property updated.

### 8.4 Set HashAlgorithm

```
alter settings model HashAlgorithm = 'BCrypt';
```

**Expected:** Property updated.

### 8.5 Set BcryptCost

```
alter settings model BcryptCost = '12';
```

**Expected:** Property updated.

### 8.6 Set JavaVersion

```
alter settings model JavaVersion = 'Java21';
```

**Expected:** Property updated.

### 8.7 Set RoundingMode

```
alter settings model RoundingMode = 'HalfUp';
```

**Expected:** Property updated.

### 8.8 Set AllowUserMultipleSessions

```
alter settings model AllowUserMultipleSessions = 'true';
```

**Expected:** Property updated.

### 8.9 Set ScheduledEventTimeZoneCode

```
alter settings model ScheduledEventTimeZoneCode = 'Europe/Amsterdam';
```

**Expected:** Property updated.

### 8.10 Unknown property

```
alter settings model FakeProperty = 'value';
```

**Expected:** Error — unknown property.

---

## 9. ALTER SETTINGS LANGUAGE

### 9.1 Set DefaultLanguageCode

```
alter settings language DefaultLanguageCode = 'en_US';
```

**Expected:** Default language updated. Verify with `show settings`.

### 9.2 Invalid language code

```
alter settings language DefaultLanguageCode = 'xx_XX';
```

**Expected:** ~~Error — invalid language code.~~ Invalid language codes are accepted without validation. **Known inconsistency.**

---

## 10. ALTER SETTINGS WORKFLOWS

### 10.1 Set UserEntity

```
alter settings workflows UserEntity = 'Administration.Account';
```

**Expected:** Workflow user entity updated.

### 10.2 Set DefaultTaskParallelism

```
alter settings workflows DefaultTaskParallelism = '5';
```

**Expected:** Property updated.

### 10.3 Set WorkflowEngineParallelism

```
alter settings workflows WorkflowEngineParallelism = '10';
```

**Expected:** Property updated.

---

## 11. ALTER SETTINGS CONFIGURATION

### 11.1 Set DatabaseType

```
alter settings configuration 'default' DatabaseType = 'PostgreSQL';
```

**Expected:** Property updated. Verify with `describe settings`.

### 11.2 Set DatabaseUrl

```
alter settings configuration 'default' DatabaseUrl = 'jdbc:postgresql://localhost:5432/mydb';
```

**Expected:** Property updated.

### 11.3 Set DatabaseName

```
alter settings configuration 'default' DatabaseName = 'mydb';
```

**Expected:** Property updated.

### 11.4 Set DatabaseUserName

```
alter settings configuration 'default' DatabaseUserName = 'admin';
```

**Expected:** Property updated.

### 11.5 Set DatabasePassword

```
alter settings configuration 'default' DatabasePassword = 'secret';
```

**Expected:** Property updated.

### 11.6 Set HttpPortNumber

```
alter settings configuration 'default' HttpPortNumber = '8080';
```

**Expected:** Property updated.

### 11.7 Set ServerPortNumber

```
alter settings configuration 'default' ServerPortNumber = '8090';
```

**Expected:** Property updated.

### 11.8 Set ApplicationRootUrl

```
alter settings configuration 'default' ApplicationRootUrl = 'http://localhost:8080/';
```

**Expected:** Property updated.

### 11.9 Non-existent configuration

```
alter settings configuration 'FakeConfig' DatabaseType = 'PostgreSQL';
```

**Expected:** Error — configuration not found.

### 11.10 Unknown property

```
alter settings configuration 'default' FakeProperty = 'value';
```

**Expected:** Error — unknown property.

---

## 12. ALTER SETTINGS CONSTANT

### 12.1 Set constant value in configuration

```
alter settings constant 'MyModule.MY_CONSTANT' value 'hello' in configuration 'default';
```

**Expected:** Constant value set. Verify with `describe settings`.

### 12.2 Update existing constant value

```
alter settings constant 'MyModule.MY_CONSTANT' value 'updated' in configuration 'default';
```

**Expected:** Constant value updated.

### 12.3 Drop constant from configuration

```
alter settings constant 'MyModule.MY_CONSTANT' drop in configuration 'default';
```

**Expected:** Constant removed from configuration.

### 12.4 Non-existent constant

```
alter settings constant 'MyModule.FAKE_CONST' value 'x' in configuration 'default';
```

**Expected:** ~~Error — constant not found.~~ Non-existent constant names are accepted without validation (creates a phantom entry in the configuration). **Known inconsistency.**

### 12.5 Non-existent configuration

```
alter settings constant 'MyModule.MY_CONSTANT' value 'x' in configuration 'FakeConfig';
```

**Expected:** Error — configuration not found.

---

## 13. CREATE CONFIGURATION

### 13.1 Create basic configuration

```
create configuration 'staging';
```

**Expected:** Configuration created. Verify with `show settings`.

### 13.2 Create configuration with properties

```
create configuration 'production'
  DatabaseType = 'PostgreSQL',
  HttpPortNumber = '8080',
  ApplicationRootUrl = 'https://app.example.com/';
```

**Expected:** Configuration created with specified properties.

### 13.3 Duplicate configuration

```
create configuration 'default';
```

**Expected:** Error — configuration already exists.

---

## 14. DROP CONFIGURATION

### 14.1 Drop existing configuration

```
drop configuration 'staging';
```

**Expected:** Configuration removed. Verify with `show settings`.

### 14.2 Drop non-existent configuration

```
drop configuration 'FakeConfig';
```

**Expected:** Error — configuration not found.

---

## 15. MULTI-STEP WORKFLOWS

### 15.1 Full navigation setup

```
create or replace navigation Responsive
  home page MyModule.Dashboard
  login page MyModule.Login
  menu (
    menu item 'Dashboard' page MyModule.Dashboard;
    menu 'Admin' (
      menu item 'Users' page Administration.AccountOverview;
    );
  );
```

Verify:

```
show navigation;
show navigation menu Responsive;
show navigation homes;
describe navigation Responsive;
```

**Expected:** All four commands reflect the created navigation state.

### 15.2 Full settings workflow

```
create configuration 'staging';
alter settings configuration 'staging' DatabaseType = 'PostgreSQL';
alter settings configuration 'staging' DatabaseUrl = 'jdbc:postgresql://staging:5432/app';
alter settings configuration 'staging' HttpPortNumber = '9090';
alter settings constant 'MyModule.API_KEY' value 'staging-key' in configuration 'staging';
describe settings;
```

**Expected:** `describe settings` output includes `staging` configuration with all set properties and the constant value.

### 15.3 Replace navigation, verify omitted clauses cleared

```
create or replace navigation Responsive
  home page MyModule.Home
  login page MyModule.Login
  menu (
    menu item 'A' page MyModule.PageA;
    menu item 'B' page MyModule.PageB;
  );
```

Then replace with minimal:

```
create or replace navigation Responsive
  home page MyModule.Home;
```

```
describe navigation Responsive;
```

**Expected:** Login page, ~~menu,~~ and not found page ~~all~~ absent from MDL output. Menu is **NOT** cleared when omitted (see §5.3 note).

---

## 16. FAILURE MODES & ERROR RECOVERY

### 16.1 Not connected

Run any navigation or settings command without an open project.

**Expected:** Error — not connected to a project.

### 16.2 Profile not found

```
show navigation menu NonExistent;
```

**Expected:** ~~Error — navigation profile not found.~~ Returns empty output with exit code 0 instead of an error. **Known inconsistency** (same as §2.4).

### 16.3 Unknown model property

```
alter settings model BadProp = 'value';
```

**Expected:** Error — unknown property.

### 16.4 Configuration not found

```
alter settings configuration 'NoSuch' DatabaseType = 'PostgreSQL';
```

**Expected:** Error — configuration not found.

### 16.5 Constant not found

```
alter settings constant 'Fake.Const' value 'x' in configuration 'default';
```

**Expected:** Error — constant not found.

---

## 17. ROUNDTRIP

### 17.1 Navigation profile roundtrip

1. `describe navigation Responsive;` — save output
2. Modify: `create or replace navigation Responsive ...` with a different home page
3. `describe navigation Responsive;` — verify change
4. Execute saved MDL from step 1
5. `describe navigation Responsive;`

**Expected:** Output at step 5 matches step 1 — original home page restored.

---

## 18. BOUNDARY & STRESS

### 18.1 Many constants in one configuration

Set 50 constants in a single configuration:

```
alter settings constant 'MyModule.Const01' value 'v01' in configuration 'default';
alter settings constant 'MyModule.Const02' value 'v02' in configuration 'default';
...
alter settings constant 'MyModule.Const50' value 'v50' in configuration 'default';
```

**Expected:** All 50 constants set. No overflow.

### 18.2 Long constant value

```
alter settings constant 'MyModule.Description' value '<10000-character string>' in configuration 'default';
```

**Expected:** Value stored and retrievable.

### 18.3 Navigation with many menu items

Create a navigation profile with 20+ menu items, some nested 3 levels deep.

**Expected:** `show navigation menu` and `describe navigation` handle all items.

### 18.4 All navigation profiles at once

Verify `show navigation` lists all profile types (Responsive, Tablet_web, Phone_web, Native_phone, Native_tablet) when all exist.

**Expected:** All profiles listed.

---

## Test Project Coverage Matrix

| Operation | Lato Enquiry | Evora Factory | Lato Product |
|-----------|:---:|:---:|:---:|
| SHOW NAVIGATION | x | x | x |
| SHOW NAVIGATION MENU | x | x | x |
| SHOW NAVIGATION HOMES | x | x | x |
| DESCRIBE NAVIGATION | x | x | x |
| CREATE OR REPLACE NAV | x | | |
| SHOW SETTINGS | x | x | x |
| DESCRIBE SETTINGS | x | x | x |
| ALTER SETTINGS MODEL | x | | |
| ALTER SETTINGS LANGUAGE | x | | |
| ALTER SETTINGS WORKFLOWS | x | | |
| ALTER SETTINGS CONFIG | x | | |
| ALTER SETTINGS CONSTANT | x | | |
| CREATE CONFIGURATION | x | | |
| DROP CONFIGURATION | x | | |
| Multi-step workflows | x | | |
| Failure modes | x | | |
| Roundtrip | x | | |
| Boundary & stress | x | | |

---

## Automated Test Coverage

| Section | Automated | Manual-only |
|---------|:---------:|:-----------:|
| 1. SHOW NAVIGATION | Mock tests | Kind suffix format |
| 2. SHOW NAVIGATION MENU | Mock tests | Deep nesting |
| 3. SHOW NAVIGATION HOMES | Mock tests | |
| 4. DESCRIBE NAVIGATION | Mock tests | Role-based homes MDL |
| 5. CREATE OR REPLACE NAV | Mock tests | Omitted-clause clearing, nested menus |
| 6. SHOW SETTINGS | Mock tests | |
| 7. DESCRIBE SETTINGS | Mock tests | |
| 8. ALTER SETTINGS MODEL | Mock tests | All 9 properties |
| 9. ALTER SETTINGS LANGUAGE | Mock tests | Invalid code |
| 10. ALTER SETTINGS WORKFLOWS | Mock tests | |
| 11. ALTER SETTINGS CONFIG | Mock tests | All 8 properties |
| 12. ALTER SETTINGS CONSTANT | Mock tests | Drop constant |
| 13. CREATE CONFIGURATION | Mock tests | With properties |
| 14. DROP CONFIGURATION | Mock tests | |
| 15. Multi-step | | All manual |
| 16. Failure modes | Partial | |
| 17. Roundtrip | | All manual |
| 18. Boundary & stress | | All manual |

---

## Manual Test Report Template

**Tester:** _______________
**Date:** _______________
**Project:** _______________

| # | Section | Test | Pass | Fail | Skip | Notes |
|---|---------|------|:----:|:----:|:----:|-------|
| 1.1 | SHOW NAV | List profiles | | | | |
| 1.2 | SHOW NAV | Kind column format | | | | |
| 2.1 | NAV MENU | Menu tree | | | | |
| 2.2 | NAV MENU | Nested items | | | | |
| 2.3 | NAV MENU | Microflow target | | | | |
| 2.4 | NAV MENU | Profile not found | | | | |
| 3.1 | NAV HOMES | Per-profile homes | | | | |
| 3.2 | NAV HOMES | No role homes | | | | |
| 4.1 | DESC NAV | Full MDL | | | | |
| 4.2 | DESC NAV | Microflow home | | | | |
| 4.3 | DESC NAV | Role-based homes | | | | |
| 4.4 | DESC NAV | Not found | | | | |
| 5.1 | ALTER NAV | Replace home | | | | |
| 5.2 | ALTER NAV | Replace login | | | | |
| 5.3 | ALTER NAV | Omitted clauses clear | | | | |
| 5.4 | ALTER NAV | Replace menu | | | | |
| 5.5 | ALTER NAV | Nested menus | | | | |
| 5.6 | ALTER NAV | Microflow menu | | | | |
| 5.7 | ALTER NAV | Role-based homes | | | | |
| 5.8 | ALTER NAV | Microflow home | | | | |
| 6.1 | SHOW SETTINGS | All sections | | | | |
| 6.2 | SHOW SETTINGS | Not connected | | | | |
| 7.1 | DESC SETTINGS | Full MDL | | | | |
| 8.1 | MODEL | AfterStartup | | | | |
| 8.2 | MODEL | BeforeShutdown | | | | |
| 8.3 | MODEL | HealthCheck | | | | |
| 8.4 | MODEL | HashAlgorithm | | | | |
| 8.5 | MODEL | BcryptCost | | | | |
| 8.6 | MODEL | JavaVersion | | | | |
| 8.7 | MODEL | RoundingMode | | | | |
| 8.8 | MODEL | MultiSessions | | | | |
| 8.9 | MODEL | TimeZone | | | | |
| 8.10 | MODEL | Unknown property | | | | |
| 9.1 | LANGUAGE | DefaultLanguageCode | | | | |
| 9.2 | LANGUAGE | Invalid code | | | | |
| 10.1 | WORKFLOWS | UserEntity | | | | |
| 10.2 | WORKFLOWS | TaskParallelism | | | | |
| 10.3 | WORKFLOWS | EngineParallelism | | | | |
| 11.1 | CONFIG | DatabaseType | | | | |
| 11.2 | CONFIG | DatabaseUrl | | | | |
| 11.3 | CONFIG | DatabaseName | | | | |
| 11.4 | CONFIG | DatabaseUserName | | | | |
| 11.5 | CONFIG | DatabasePassword | | | | |
| 11.6 | CONFIG | HttpPortNumber | | | | |
| 11.7 | CONFIG | ServerPortNumber | | | | |
| 11.8 | CONFIG | ApplicationRootUrl | | | | |
| 11.9 | CONFIG | Not found | | | | |
| 11.10 | CONFIG | Unknown property | | | | |
| 12.1 | CONSTANT | Set value | | | | |
| 12.2 | CONSTANT | Update value | | | | |
| 12.3 | CONSTANT | Drop | | | | |
| 12.4 | CONSTANT | Const not found | | | | |
| 12.5 | CONSTANT | Config not found | | | | |
| 13.1 | CREATE CFG | Basic | | | | |
| 13.2 | CREATE CFG | With properties | | | | |
| 13.3 | CREATE CFG | Duplicate | | | | |
| 14.1 | DROP CFG | Existing | | | | |
| 14.2 | DROP CFG | Not found | | | | |
| 15.1 | MULTI-STEP | Navigation setup | | | | |
| 15.2 | MULTI-STEP | Settings workflow | | | | |
| 15.3 | MULTI-STEP | Omitted clauses | | | | |
| 16.1 | FAILURE | Not connected | | | | |
| 16.2 | FAILURE | Profile not found | | | | |
| 16.3 | FAILURE | Unknown property | | | | |
| 16.4 | FAILURE | Config not found | | | | |
| 16.5 | FAILURE | Constant not found | | | | |
| 17.1 | ROUNDTRIP | Navigation profile | | | | |
| 18.1 | BOUNDARY | Many constants | | | | |
| 18.2 | BOUNDARY | Long constant value | | | | |
| 18.3 | BOUNDARY | Many menu items | | | | |
| 18.4 | BOUNDARY | All nav profiles | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
