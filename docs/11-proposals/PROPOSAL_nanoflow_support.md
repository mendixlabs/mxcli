# Proposal: Comprehensive Nanoflow Support

## Overview

**Status:** Implemented
**Priority:** High — nanoflows are heavily used (223 across test projects) and CLI parity with microflows is expected.

Full nanoflow feature surface in mxcli: CREATE, DROP, CALL, GRANT/REVOKE, SHOW, SHOW ACCESS, DESCRIBE, DESCRIBE MERMAID, and validation. Supersedes the earlier `show-describe-nanoflows.md` proposal which focused only on DESCRIBE/DROP.

## Background

Nanoflows execute client-side (browser or native app). In the Mendix metamodel, `Nanoflow` inherits from `MicroflowBase` (not `ServerSideMicroflow`), sharing the same flow structure (`MicroflowObjectCollection`, `SequenceFlow`, action types) but with restricted action set and different properties.

### Nanoflow vs Microflow Model

| Property | Nanoflow | Microflow |
|----------|----------|-----------|
| Inheritance | `MicroflowBase` (direct) | `ServerSideMicroflow` → `MicroflowBase` |
| `AllowedModuleRoles` | Yes (design-time only) | Yes (runtime enforced) |
| `ApplyEntityAccess` | No | Yes |
| `AllowConcurrentExecution` | No | Yes |
| `ConcurrencyErrorMessage` | No | Yes |
| `MicroflowActionInfo` | No | Yes |
| `WorkflowActionInfo` | No | Yes |
| `Url*` / `StableId` | No | Yes |
| `UseListParameterByReference` | Yes (default true) | No |
| Allowed return types | No `Binary`, no `Float` | All types |
| `ErrorEvent` | Forbidden | Allowed |
| Expression context | `ClientExpressionContext` | `MicroflowExpressionContext` |
| Predefined variables | `$latestError` (String) | (microflow-specific set) |

### Action Restrictions

**Allowed in nanoflows**: ChangeVariable, AggregateList, CreateVariable, Rollback, Retrieve, Delete, CreateChange, Commit, Cast, Change, LogMessage, ListOperation, CreateList, ChangeList, MicroflowCall, ValidationFeedback, ShowPage, ShowMessage, CloseForm, NanoflowCall, JavaScriptActionCall, Synchronize, CancelSynchronization, ClearFromClient, and others not explicitly denied.

**Disallowed** (enforced by `checkDisallowedNanoflowAction` in `nanoflow_validation.go`, 12 case branches covering 22 action types): RaiseError/ErrorEvent, JavaActionCall, RestCall, SendRestRequest, ImportFromMapping, ExportToMapping, CallExternalAction, DownloadFile, ShowHomePage, TransformJson, ExecuteDatabaseQuery, and 11 workflow action types (CallWorkflow, GetWorkflowData, GetWorkflows, GetWorkflowActivityRecords, WorkflowOperation, SetTaskOutcome, OpenUserTask, NotifyWorkflow, OpenWorkflow, LockWorkflow, UnlockWorkflow).

## Supported Commands

| Command | Description |
|---------|-------------|
| `CREATE NANOFLOW Module.Name(params) RETURNS type BEGIN ... END` | Create a nanoflow with body, parameters, return type |
| `CREATE OR MODIFY NANOFLOW ...` | Create or update existing nanoflow |
| `DROP NANOFLOW Module.Name` | Delete a nanoflow |
| `CALL NANOFLOW Module.Name(args)` | Call a nanoflow from within a flow body (valid in both microflows and nanoflows) |
| `GRANT EXECUTE ON NANOFLOW Module.Name TO RoleList` | Grant module role access |
| `REVOKE EXECUTE ON NANOFLOW Module.Name FROM RoleList` | Revoke module role access |
| `SHOW NANOFLOWS [IN module]` | List nanoflows with activity counts |
| `SHOW ACCESS ON NANOFLOW Module.Name` | Display allowed module roles |
| `DESCRIBE NANOFLOW Module.Name` | Output MDL representation |
| `DESCRIBE MERMAID NANOFLOW Module.Name` | Render Mermaid flowchart |
| `RENAME NANOFLOW Module.Old TO New` | Rename a nanoflow |
| `MOVE NANOFLOW Module.Name TO FOLDER 'path'` | Move to a different folder |

## Grammar

### CREATE NANOFLOW

```antlr
createNanoflowStatement
    : NANOFLOW qualifiedName
      LPAREN microflowParameterList? RPAREN
      microflowReturnType?
      microflowOptions?
      BEGIN microflowBody END SEMICOLON? SLASH?
    ;
```

Reuses all microflow sub-rules (parameters, return type, options, body). The `microflowBody` rule is shared between microflows and nanoflows — `CALL NANOFLOW` is valid in both contexts since microflows can call nanoflows. Nanoflow-specific action restrictions are enforced at the executor level.

### CALL NANOFLOW + GRANT/REVOKE

```antlr
callNanoflowStatement
    : (VARIABLE EQUALS)? CALL NANOFLOW qualifiedName
      LPAREN callArgumentList? RPAREN onErrorClause?
    ;

grantNanoflowAccessStatement
    : GRANT EXECUTE ON NANOFLOW qualifiedName TO moduleRoleList
    ;

revokeNanoflowAccessStatement
    : REVOKE EXECUTE ON NANOFLOW qualifiedName FROM moduleRoleList
    ;
```

## Validation Rules

1. **Disallowed actions** — Type-switch rejects 12 case branches covering 22 microflow-only action types with descriptive error messages (11 workflow actions collapsed into multi-type case)
2. **ErrorEvent forbidden** — `ErrorEvent is not allowed in nanoflows`
3. **Binary return type rejected** — `Binary return type is not allowed in nanoflows`
4. **Recursive validation** — Checks compound statements (IF/LOOP/WHILE bodies) and error handling blocks
5. **Cross-reference validation** — Checks that `call nanoflow Module.Name` targets exist

## SDK Types

- `NanoflowCallAction` — calls a nanoflow with parameter mappings and optional result variable
- `NanoflowCall` — references the target nanoflow by qualified name
- `NanoflowCallParameterMapping` — maps arguments to parameters
- `AllowedModuleRoles` on `Nanoflow` — list of module role IDs with access

## Not Planned (by design)

| Feature | Reason |
|---------|--------|
| HOME NANOFLOW (navigation) | Home page/microflow is server-side |
| MENU ITEM NANOFLOW | Menu items use server-side navigation |
| Workflow CALL NANOFLOW | Workflow activities are server-side |
| Published REST NANOFLOW handler | REST operations are server-side |

## Known Issues

1. **Float return type** — `ast.TypeFloat` does not exist in the AST, so Float can never be used as a return type. No validation needed. [risk: none]

## Future Work

| Feature | Priority | Notes |
|---------|----------|-------|
| Roundtrip tests with real `.mpr` baselines | P2 | CREATE → DESCRIBE → re-CREATE verification against App Gallery demos |
| SynchronizeAction | P3 | `synchronize` action for offline nanoflows |
| Web/Native platform mixing check | P3 | CE6051 validation |

### Completed (formerly Future Work)

| Feature | Status | Notes |
|---------|--------|-------|
| JavaScriptActionCall syntax | Done | `call javascript action Module.ActionName(params)` fully supported in grammar, parser, builder, serializer, and roundtrip |
| ELK layout | Done | `describe nanoflow --format elk` produces valid JSON layout |

## Testing

Test plan: `docs/15-testing/nanoflow-test-cases.md` (19 sections, 100+ test cases covering all commands, validation, BSON round-trip, catalog, and edge cases).

Test projects: App Gallery demos — Lato Enquiry Management (79 nanoflows), Evora Factory Management (93 nanoflows), Lato Product Inventory (51 nanoflows). Total 223.
