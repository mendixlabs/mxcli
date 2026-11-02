# Built-in Rules

mxcli ships with built-in rules implemented in Go. These rules are fast and always available.

The **lint rules** below run with `mxcli lint`. There is also a separate group of **check-time rules** that run with `mxcli check` — see [Check-time Rules](#check-time-rules-mxcli-check) at the bottom of this page.

## MDL Rules

| Rule | Description |
|------|-------------|
| **MDL001** | Naming conventions -- Checks entity, attribute, and microflow naming patterns |
| **MDL002** | Empty microflows -- Detects microflows with no activities |
| **MDL003** | Domain model size -- Warns when a module has too many entities |
| **MDL004** | Validation feedback -- Checks for proper validation feedback usage |
| **MDL005** | Image source -- Validates image widget source configuration |
| **MDL006** | Empty containers -- Detects container widgets with no children |
| **MDL007** | Page navigation security -- Checks that pages called from microflows have appropriate access rules |

## Quality Rules

| Rule | Description |
|------|-------------|
| **QUAL006** | Required caption without the default language -- A tab page caption (page, snippet or layout) with no text in the project's `DefaultLanguageCode`. MxBuild refuses it with CE4899 "Empty caption. [German, Germany]". Typically left behind by changing the default language after the pages were written. Error. Measured on Mendix 11.14: of the caption kinds tried (page title, group box, buttons, column header, label, dynamic text, title, enumeration value, menu item, message) only the tab page caption is required; page templates and building blocks are not checked |

## Security Rules

| Rule | Description |
|------|-------------|
| **SEC001** | Entity access rules -- Ensures persistent entities have access rules defined |
| **SEC002** | Password policy -- Checks for secure password configuration |
| **SEC003** | Demo users -- Warns about demo users in production security level |
| **SEC010** | Unreadable page data -- A data grid, gallery, list view or combo box lists an entity that a user role allowed on the page cannot read (including its generalizations and specializations). At Production security it renders empty for that role and nothing else reports it. Info when the role reads only through an XPath-constrained rule; System entities use access measured from a deployed model, so a combo box over System.User gets its own message (it lists only the current user) |
| **SEC011** | XPath through an unreadable association -- A page data source's XPath crosses an association that a user role allowed on the page may not read on the entity the step starts from (an association is a member of its FROM entity's rules, and of its TO entity's when the owner is Both). At Production security the page fails to open with "No access rights for Module$Entity/Module.Association", while mxbuild reports nothing. Error. Entity access-rule XPaths are not checked: the runtime does not apply member access there (measured, 11.15) |
| **SEC012** | Reachable page not readable -- Predicts mxbuild's CE2729: a page a user role can reach shows an attribute, or lists an entity, the role may not read. Reached means: a menu item (if the role may view the page), the role's home page (its role-based one, else the profile's; checked even when the role may not view it), and from there show-page actions, snippet calls, and microflows or nanoflows the role may execute, with their sub-microflows. Error. Measured against mxbuild 11.13 and 11.15; association reads, System entities, Marketplace modules and pages reachable only by URL are left out |

## Convention Rules

| Rule | Description |
|------|-------------|
| **CONV011** | No commit in loop -- Detects COMMIT statements, and CREATE / CHANGE with a COMMIT clause, inside LOOP blocks (performance anti-pattern) |
| **CONV012** | Exclusive split captions -- Checks that decision branches have meaningful captions |
| **CONV013** | Error handling on external calls -- Ensures external service calls have error handling |
| **CONV014** | No continue error handling -- Warns against using CONTINUE error handling without logging |
| **CONV020** | Created object never persisted -- A microflow creates an object of a persistent entity without commit, and never commits, returns or passes it on, so the object is discarded when the microflow ends ([details](#conv020-details)) |
| **CONV021** | Create/change then commit -- A Create or Change object activity without commit, directly followed by a Commit of the same object; one create/change with Commit set does both ([details](#conv021-details)) |
| **MDL-FLOW01** | Un-describable branch structure -- Decision branches that re-enter each other's paths, so `DESCRIBE MICROFLOW` cannot render them as nested `IF`s without changing what they mean |
| **MDL-MAP04** | First over an object-rooted import mapping -- An `import from mapping` or `rest call … returns mapping` activity already in the model stores Studio Pro's *First* (`ForceSingleOccurrence` or `Range.SingleObject`) over a mapping that returns one object. `mx check` reports 0 errors; the activity throws `key not found: Path(QName(None,),None,)` when it runs. `mxcli check` refuses the same statement in a script under the same ID; this finds activities written before that check existed, or with `exec --no-check`. A list-rooted mapping's *First* is legitimate and is not reported |

### CONV020 details {#conv020-details}

`$Item = create Shop.Item (...)` without `commit` makes an object in memory only.
If the microflow then neither commits it nor hands it to anything that could,
the object is gone when the microflow ends. Nothing else reports this: `check`,
`mx check` and the build all pass, and the microflow runs without error. In the
report that prompted the rule, an after-startup seed microflow logged
"Seeded 20" and the table stayed empty.

The rule reports a create only when **all** of these hold:

- it is in a **microflow** (not a nanoflow or a rule) and its commit setting is
  **No**;
- the entity is **persistent** (a non-persistent entity is meant to be
  temporary), and is not a System or Marketplace entity;
- nothing in the microflow, on any path, uses the variable except to read one
  of its members (`$Item/Name`) or to change it without commit.

Any other mention counts as passing the object on and keeps the rule quiet:
`commit`, `change … commit`, returning it, an argument to a microflow, Java or
JavaScript action, workflow, mapping or page, adding it to a list, using it as
an association value on another object, `delete`, `rollback`, or the bare
variable anywhere in an expression. The check is flow-insensitive: a commit on
one branch only, or only in an error handler, counts, so that case is not
reported. When the rule cannot tell, it stays quiet.

### CONV021 details {#conv021-details}

```sql
$Order = create Shop.Order (Code = 'A1');
commit $Order;
```

is two activities doing what one does. Studio Pro's Create object and Change
object activities have a **Commit** option (Yes, Yes without events, No), and the
expected way to model "create and save" or "change and save" is that option, not
a separate Commit activity:

```sql
$Order = create Shop.Order (Code = 'A1') commit;
```

The suggestion carries the commit's settings over: a Commit with events becomes
Commit **Yes**, one without events **Yes without events** (`commit without
events`), and **Refresh in client** is kept if either activity had it (`refresh`).

The rule reports a pair only when merging it cannot change what the microflow
does:

- it is in a **microflow**, the create/change has Commit **No**, and the commit
  names the object it creates or changes;
- the commit **directly** follows it on the normal flow (not an error handler
  flow), and no other flow leads into the commit;
- neither activity has **custom error handling**, which merging would move or
  drop.

## Running Built-in Rules

Built-in rules run automatically with `mxcli lint`:

```bash
# Run all rules (built-in + Starlark)
mxcli lint -p app.mpr

# List all rules to see which are built-in
mxcli lint -p app.mpr --list-rules
```

## Excluding Modules

System and marketplace modules often trigger false positives. Exclude them:

```bash
mxcli lint -p app.mpr --exclude System --exclude Administration
```

## Check-time Rules (`mxcli check`)

These rules run with `mxcli check` (and the LSP, for real-time diagnostics) rather than `mxcli lint`. They cover pluggable-widget authoring and typed project-settings values.

| Rule | Where it fires | Description |
|------|----------------|-------------|
| **MDL-WIDGET01** | `mxcli check` + LSP | Unknown property key on a pluggable widget. The property is not in the widget's `.def.json`. Catches typos like `optionsSourcType` (missing `e`) before MxBuild does. Suggests the nearest known key. |
| **MDL-WIDGET02** | `mxcli check --post-migration` | Legacy native widget found on a project that has a pluggable replacement available. Reports each occurrence with the qualified document name, widget instance name, and the recommended pluggable widget. |
| **MDL-SET01** | `mxcli check` + LSP | Non-integer value for an Integer-typed project setting (`HttpPortNumber`, `ServerPortNumber`, `MaxJavaHeapSize`, `BcryptCost`, `DefaultTaskParallelism`, `WorkflowEngineParallelism`). These used to be skipped silently while the statement still reported success. |
| **MDL-I18N01** | `mxcli check -p` | A script that changes `DefaultLanguageCode` leaves a tab page caption — stored in the project, or created by the script before the change — with no text in the new default. MxBuild reports CE4899 "Empty caption". See [Error Messages](../appendixes/error-messages.md#mdl-i18n01-a-tab-page-caption-without-the-default-language-ce4899). |
| **MDL-SET02** | `mxcli check` + LSP | Value other than `true` / `false` for a Boolean-typed project setting (`AllowUserMultipleSessions`). Anything else was silently stored as `false`. |
| **MDL-SET04** | `mxcli check` + LSP | `ALTER SETTINGS` section or key that does not exist (`alter settings nosuchsection (…)`, a misspelled `ExtraJvmParams`), or a verb the section does not define. Reported with the message `exec` refuses it with, and the valid keys; `check` used to pass these and leave `exec` to stop halfway through the script. |

Run `mxcli check --help` for usage. See [Error Messages → MDL-WIDGET01 / MDL-WIDGET02](../appendixes/error-messages.md#mdl-widget01-unknown-pluggable-widget-property) for cause-and-solution detail.
