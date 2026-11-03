---
title: Module settings — describe and set the module version and module type
status: proposed
date: 2026-11-03
author: Generated with Claude Code
issue: ako/mxcli#1921
supersedes: show-describe-module-settings.md
depends-on: ako/mxcli#1920
---

# Proposal: Module settings — describe and set the module version and module type

**Status:** Proposed
**Date:** 2026-11-03

## Problem Statement

The General tab of Studio Pro's *Module settings* dialog has four fields:
*Module type* (App module / Add-on module), *Module version*, *Location*
(Module list / Marketplace modules) and *Module ID*. mxcli can describe none of
them and set none of them. `describe module Odata` prints

```mdl
create or modify module Odata;
create or modify module role Odata.User;
```

plus, since #1498, a comment when the module is Protected. Only the *Java
Dependencies* tab has syntax (`alter module … jar dependency`).

Who benefits:

- **Teams versioning their own modules.** A module meant for reuse is exported as
  an `.mpk` with its version, and the version is what a consumer sees when the
  module is updated. Scripts that build or refactor such a module cannot bump the
  version today, and `describe` cannot tell an agent which version it is looking
  at.
- **Authors of add-on modules.** An add-on module hides every element not marked
  *Usable* from the rest of the app (CE1406). Making a module an add-on from MDL
  lets a script build it complete, and `describe` lets an agent see why a
  reference is refused.

This supersedes [`show-describe-module-settings.md`](show-describe-module-settings.md),
a read-only proposal from before the modelsdk engine. Its BSON notes are partly
wrong (`version` lowercase, `ProtectedModuleType: "none"`, neither of which
Studio Pro stores).

## BSON Structure

Measured on ako/TestApp (Mendix 11.16.0 Beta): 24 modules, plus one probe module,
`ModSettingsProbe`, changed one field at a time in Studio Pro and diffed between
saves.

### `Projects$ModuleSettings` — one unit per module, contained by the module

| Key | Observed values | Meaning |
|---|---|---|
| `ExportLevel` | `Source` (every app module), `Protected` (the probe, set to *Add-on module*) | **The module type.** `Source` = App module, `Protected` = Add-on module. |
| `ProtectedModuleType` | `AddOn` on **every** module, app modules included | Which kind of protected module; only meaningful when `ExportLevel` is `Protected`. Metamodel literals `AddOn`, `Solution`; introduced 9.12.0. |
| `Version` | `1.0.0` (every untouched app module), `1.0.1`, `2.3.1` (probe), `4.11.0` (WorkflowCommons) | *Module version*, a plain string; the dialog requires SemVer. |
| `PackageId` | a UUID per module | *Module ID*, shown read-only with an edit pencil. |
| `Checksum` | empty, or base64 on an imported Marketplace module | Studio Pro uses it to tell whether an imported module was modified. |
| `ConvertedChecksum`, `OriginalPackageId`, `BasedOnVersion`, `SolutionIdentifier`, `ExtensionName` | empty everywhere | — |
| `EnableDetailedTroubleshooting` | `true` or `false` | Not on the General tab. |
| `ModuleDependencies` | `null` everywhere | — |
| `JarDependencies` | marker-2 list | Already supported. |

**What a measured edit changed:**

| Dialog edit | Stored change |
|---|---|
| Module version 1.0.1 → 2.3.1 | `ModuleSettings.Version` only |
| Location → Marketplace modules | `Projects$ModuleImpl.FromAppStore` `false` → `true`; `AppStoreGuid`, `AppStoreVersion`, `AppStoreVersionGuid` and `AppStorePackageIdString` stay empty |
| Module type → Add-on | `ExportLevel` `Source` → `Protected`, and nothing else (confirmed by the maintainer in Studio Pro 11.16) |

### `Projects$ModuleImpl` — the module itself

`Name`, `FromAppStore`, `AppStoreGuid`, `AppStoreVersion`, `AppStoreVersionGuid`,
`AppStorePackageIdString`, `IsThemeModule`, `NewSortIndex`. *Location* is
`FromAppStore`. `show modules` already reads `AppStoreVersion` for its `Source`
column (`Marketplace v4.11.0`). A module moved to *Marketplace modules* by hand
has `FromAppStore: true` and **no** App Store version.

### Gotchas

- **`ProtectedModuleType` does not say "add-on".** Every app module stores
  `AddOn` there, so reading it alone makes every module look like an add-on. The
  type is `ExportLevel`.
- **Writing the settings unit must carry what mxcli does not model.** Before
  #1920, the only writer rebuilt the unit and wiped `PackageId` and `Checksum`.
  This proposal adds writes to the same unit, so it depends on #1920's carry.
- **Location and the Marketplace fields are one concept to Studio Pro.**
  `FromAppStore` is also how a real Marketplace import is recognised, so writing
  it is a separate step with its own measurement (see *Later: Location*).

## Proposed MDL Syntax

The module's settings are properties of the `create` statement (R2), and
`alter … set` takes exactly the same keys (R3). Keys are named after the dialog's
fields.

```mdl
-- Create or update a module with its settings
create or modify module Inventory (
  Version: '2.3.1',
  ModuleType: addon,
);

-- Change one setting
alter module Inventory set (Version: '2.4.0');
alter module Inventory set (ModuleType: app);
```

| Key | Values | Stored as | Default (omitted by `describe`, R12) |
|---|---|---|---|
| `Version` | a SemVer string, `'major.minor.patch'` | `ModuleSettings.Version` | `'1.0.0'` |
| `ModuleType` | `app` \| `addon` | `ExportLevel` `Source` \| `Protected`, with `ProtectedModuleType` `AddOn` | `app` |

`describe module` prints the same statement with only the settings that differ
from the defaults, so its output re-runs (R6):

```mdl
create or modify module ModSettingsProbe (
  Version: '2.3.1',
  ModuleType: addon,
);
-- location: Marketplace modules (set in Studio Pro)
```

**Read-only for now, printed as comments:**

- **Location**, only when it is *Marketplace modules*: `-- location: Marketplace
  modules (set in Studio Pro)`. A real import already shows its App Store version
  in `show modules`. It becomes a key in a later step (*Later: Location*), and the
  comment then turns into `Location: marketplace`.
- The **#1498 comment** about elements not marked *Usable* stays, attached to the
  `ModuleType: addon` line it explains, instead of standing alone.

**Not shown:** *Module ID* (`PackageId`) and the checksum. They are identifiers
Studio Pro owns, and printing them adds a line to every module's description
without anything a script could do with it.

**Validation** (shared by `check` and `exec`, so they cannot drift):

- `Version` must be SemVer `major.minor.patch` with an optional pre-release
  suffix. Studio Pro refuses anything else in the dialog.
- An unknown key, a repeated key, or a `ModuleType` other than `app`/`addon` is
  an error from the visitor (R11).
- `ModuleType: addon` below Mendix 9.12 is refused (MDL-VERSION01), the release
  that introduced `ProtectedModuleType`.
- `ModuleType: solution` is not accepted until a solution project has been
  measured (see Open Questions).

`show modules` gains two columns, `Version` and `Type`, so a project's module
versions can be listed in one command.

### Later: Location

*Location* is meant to become writable, as a third key with the same shape:

```mdl
alter module Inventory set (Location: marketplace);   -- or: modulelist
```

It writes `Projects$ModuleImpl.FromAppStore`, a different unit from the other two
keys. It is not in the first implementation, because a hand-made module set to
*Marketplace modules* and a real import both have `FromAppStore: true`, and only
the import carries an App Store GUID and version. Before the key lands, measure:

- whether Studio Pro changes anything else when the location is switched, and
  back again;
- whether moving a **real import** to *Module list* clears or keeps its App Store
  fields, since those fields are what `show modules` and the Marketplace upgrade
  (`PROPOSAL_marketplace_module_upgrade.md`) recognise an import by.

Until then the comment form stays, and `describe` output re-runs unchanged.

## Implementation Plan

Order matters: the write path lands only on top of #1920, and `describe` lands
before the syntax, so the read side can be verified on real projects first.

1. **Read (describe + show).** `describe module` prints `Version` and
   `ModuleType` when they differ from the defaults, and the Location comment.
   `show modules` gains the two columns. No grammar change.
2. **Grammar + visitor.** `createModuleStatement` takes a `( Key: value, … )`
   list after the name (the existing `folder '…'` option stays a clause, R9), and
   a new `alterModuleSetStatement` takes `set ( … )`. Key table and value checks
   in the visitor, following `visitor_rest_call_settings.go`.
3. **Executor + backend.** `execCreateModule` (create and `create or modify`) and
   a new `execAlterModuleSet` write through `UpdateModuleSettings`, which (after
   #1920) writes onto the stored unit and changes only the keys MDL names.
   `CreateModule` writes the stated settings for a new module.
4. **Check.** SemVer and version-gate validation in the shared check path.

### Files to modify/create

| File | Change |
|---|---|
| `mdl/executor/cmd_modules.go` | `describeModule`: settings in the create statement, Location comment; `show modules` columns; `execCreateModule` applies settings; new `execAlterModuleSet` |
| `mdl/grammar/domains/MDLDomainModel.g4` | property list on `createModuleStatement`; `alterModuleSetStatement` |
| `mdl/grammar/MDLParser.g4` | register `alterModuleSetStatement` beside `alterModuleJarDepStatement` |
| `mdl/ast/ast_enumeration.go`, `mdl/ast/ast_module_settings.go` | `CreateModuleStmt.Settings`; `AlterModuleSetStmt` |
| `mdl/visitor/visitor_module_settings.go` *(new)* | key table, value parsing, unknown-key errors |
| `mdl/types/module_settings.go` | no new fields: `Version` and `ExportLevel` are already there |
| `mdl/backend/modelsdk/module_settings_write.go` | none beyond #1920; `CreateModule` takes the stated settings |
| `mdl/backend/modelsdk/module_write.go` | `CreateModule` writes the stated `Version` / `ExportLevel` |
| `mdl/executor/` check path | SemVer and 9.12 gate, shared by `check` and `exec` |
| `docs-site/src/` (modules page), `mxcli syntax module` | the new keys |
| `.claude/skills/mendix/` (module skill, if one covers modules) | `ModuleType: addon` and the *Usable* consequence |

## Version Compatibility

- `Version` and `ExportLevel` are in every supported Mendix version's
  metamodel: no gate for `Version`.
- `ModuleType: addon` writes `ProtectedModuleType`, introduced in 9.12.0, and is
  refused below it.
- The MCP backend has no module-settings write (`unsupported_gen.go`). It returns
  its usual unsupported error, the same as `alter module … jar dependency` today.

## Test Plan

- **Fixture-backed read tests:** the probe module's settings and module units
  (Add-on, 2.3.1, Marketplace location) and an untouched app module's, from
  ako/TestApp. `describe` prints the expected statement and comment for each.
  Control: the app module prints `create or modify module Odata;` alone.
- **Round trip:** describe → exec on a copy writes nothing (elision) for the
  probe, with a control edit (`Version: '2.3.2'`) that does write, so the
  "nothing changed" assertion is not vacuous.
- **Write tests:** `alter module … set (Version: …)` and `set (ModuleType: addon)`
  change exactly `Version` / `ExportLevel`, and every other key of the
  Studio Pro unit stays byte-identical (the #1920 test, extended).
- **Validation:** `check` refuses `Version: '2.3'`, `ModuleType: solution`, an
  unknown key, and `addon` under a 9.11 project version, and `exec` refuses the
  same, with no write.
- **Studio Pro check:** open a project where `alter module … set (ModuleType:
  addon)` ran, confirm the dialog shows *Add-on module* and `mx check` reports no
  new errors; repeat for `app`.
- `mdl-examples/doctype-tests/`: one script that creates a module with settings,
  alters both keys and describes it.

## Open Questions

1. **Solution modules.** `ProtectedModuleType: Solution` exists in the
   metamodel, but the dialog in TestApp offers only App and Add-on. It probably
   appears only in a solution project. Out of scope until one is measured.
2. **What else Location must carry** before it becomes writable (*Later:
   Location*).

### Resolved

- **A new module's defaults in 11.16** are *App module* and `1.0.0`, which is
  what `CreateModule` already writes and what `describe` omits.
- **Switching App ↔ Add-on** changes only `ExportLevel`.
- **Location becomes writable**, in a later step after its measurement.
