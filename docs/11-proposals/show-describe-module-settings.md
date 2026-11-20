---
title: SHOW/DESCRIBE Module Settings
status: superseded
---

# Proposal: SHOW/DESCRIBE Module Settings

> **Status: SUPERSEDED** by [`PROPOSAL_module_settings.md`](./PROPOSAL_module_settings.md) (ako/mxcli#1921), which measures the storage on Mendix 11.16 and adds the write side. This proposal is retained for historical context only; its BSON notes are not reliable (`version` is stored as `Version`, and `ProtectedModuleType` is never `none`).

## Overview

**Document type:** `Projects$ModuleSettings`
**Prevalence:** 97 across test projects (28 Enquiries, 39 Evora, 30 Lato) — one per module
**Priority:** Medium — every module has one, useful for identifying App Store modules and versions

Module Settings contain metadata about a module: its version, whether it came from the Mendix Marketplace, its protection level, and JAR dependencies. This is separate from Module Security (which is already implemented).

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | No | — |
| **Parser** | No | — |
| **Reader** | No | — |
| **Generated metamodel** | No | Not in generated types (simple project-level type) |

## BSON Structure (from test projects)

```
Projects$ModuleSettings:
  version: string (e.g., "2.3.0")
  BasedOnVersion: string
  ExportLevel: string ("source")
  ExtensionName: string
  ProtectedModuleType: string ("AddOn", "none", etc.)
  SolutionIdentifier: string
  JarDependencies: [] (array of JAR refs)
```

Note: There is one `ModuleSettings` per module, stored as a separate unit. The `ModuleImpl` unit (also one per module) stores: `AppStoreGuid`, `AppStoreVersion`, `FromAppStore`, `IsThemeModule`, `Name`.

## Proposed MDL Syntax

### SHOW MODULE SETTINGS

```
show module settings [in module]
```

| Module | Version | Based On | From App Store | App Store Version | Protected | Theme Module |
|--------|---------|----------|----------------|-------------------|-----------|--------------|

This combines data from both `ModuleSettings` and `ModuleImpl` to give a complete picture per module.

### DESCRIBE MODULE SETTINGS

```
describe module settings module
```

Output format:

```
-- Module Settings: Atlas_Core
module settings Atlas_Core
  version '3.0.9'
  BASED on version '3.0.8'
  from APP store
    GUID 'abc-123'
    APP store version '3.0.9'
  PROTECTED type AddOn
  THEME module;
/
```

For user-created modules with minimal settings:

```
-- Module Settings: MyFirstModule
module settings MyFirstModule
  version ''
  PROTECTED type none;
/
```

## Implementation Steps

### 1. Add Model Types (model/types.go)

```go
type ModuleSettings struct {
    ContainerID       model.ID
    ModuleName        string
    version           string
    BasedOnVersion    string
    ExportLevel       string
    ProtectedModuleType string
    SolutionIdentifier  string
    ExtensionName     string
    // from ModuleImpl:
    FromAppStore      bool
    AppStoreGuid      string
    AppStoreVersion   string
    IsThemeModule     bool
}
```

### 2. Add Parser (sdk/mpr/parser_misc.go)

Parse both `Projects$ModuleSettings` and correlate with `Projects$ModuleImpl` data (already parsed for modules).

### 3. Add Reader

```go
func (r *Reader) ListModuleSettings() ([]*model.ModuleSettings, error)
```

### 4. Add AST, Grammar, Visitor, Executor

Standard pattern. Reuse existing `module` token; add `settings` context for `show module settings`.

Note: `show settings` already exists for Project Settings. Use `show module settings` to disambiguate.

## Testing

- Verify against all 3 test projects
- Check that App Store modules are correctly identified
