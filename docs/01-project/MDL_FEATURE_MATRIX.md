# MDL Feature Completeness Matrix

This document tracks the implementation status of each MDL language feature across all support dimensions.
When adding a new MDL feature, use this matrix as a checklist to ensure complete coverage.

**Legend:** Y = Yes | N = No | P = Partial | - = Not Applicable

## Core Document Types

| Feature | SHOW | DESCRIBE | CREATE | OR MODIFY | DROP | ALTER | Examples | Tests | Catalog | REFS | LSP | Skills | Help | Viz | REPL | Syntax | Starlark |
|---------|------|----------|--------|-----------|------|-------|----------|-------|---------|------|-----|--------|------|-----|------|--------|----------|
| **Entities** | Y | Y | Y | Y | Y | Y | 01 | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y |
| **Associations** | Y | Y | Y | N | Y | Y | 01 | Y | N | Y | Y | Y | Y | Y | Y | Y | N |
| **Enumerations** | Y | Y | Y | Y | Y | Y | 01 | Y | Y | N | Y | Y | Y | N | Y | Y | Y |
| **Microflows** | Y | Y | Y | Y | Y | N | 02 | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y |
| **Nanoflows** | Y | Y | Y | Y | Y | N | Y | Y | Y | Y | Y | Y | Y | Y | P | N | N |
| **Pages** | Y | Y | Y | N | Y | Y | 03 | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y |
| **Snippets** | Y | Y | Y | N | Y | Y | 03 | Y | Y | Y | Y | Y | Y | N | Y | Y | Y |
| **Layouts** | Y | Y | N | N | N | N | N | N | Y | Y | Y | N | Y | N | Y | N | N |
| **Java Actions** | Y | Y | Y | N | Y | N | 07 | Y | Y | Y | Y | Y | Y | N | Y | Y | N |
| **Constants** | Y | Y | Y | Y | Y | N | 09 | Y | N | P | Y | N | Y | N | P | N | N |
| **OData Clients** | Y | Y | Y | Y | Y | Y | 10 | Y | Y | P | Y | Y | Y | N | Y | Y | N |
| **OData Services** | Y | Y | Y | Y | Y | Y | 10 | Y | Y | Y | Y | Y | Y | N | Y | Y | N |
| **External Entities** | Y | Y | Y | Y | N | N | 10 | Y | Y | Y | Y | Y | Y | N | P | Y | N |
| **Modules** | Y | Y | Y | N | Y | N | all | Y | Y | Y | Y | Y | Y | N | Y | N | N |
| **Navigation** | Y | Y | Y | - | - | Y | 11 | N | Y | Y | Y | Y | Y | N | N | Y | N |
| **Business Events** | Y | Y | Y | N | Y | N | 13 | N | Y | N | Y | N | Y | N | Y | Y | N |
| **Project Settings** | Y | Y | - | - | - | Y | N | N | Y | Y | Y | N | Y | N | N | Y | P |

## Security Features

| Feature | SHOW | DESCRIBE | CREATE | OR MODIFY | DROP | ALTER | Examples | Tests | Catalog | REFS | LSP | Skills | Help | Viz | REPL | Syntax | Starlark |
|---------|------|----------|--------|-----------|------|-------|----------|-------|---------|------|-----|--------|------|-----|------|--------|----------|
| **Module Roles** | Y | Y | Y | N | Y | N | 08 | Y | N | Y | Y | Y | Y | N | N | Y | Y |
| **User Roles** | Y | Y | Y | N | Y | Y | 08 | Y | N | Y | Y | Y | Y | N | N | Y | Y |
| **Demo Users** | Y | Y | Y | N | Y | N | 08 | Y | N | N | Y | Y | Y | N | N | Y | N |
| **Project Security** | Y | - | - | - | - | Y | 08 | Y | Y | Y | Y | Y | Y | N | N | Y | Y |
| **Entity Access** | P | N | Y | P | Y | P | 08 | Y | N | Y | Y | Y | Y | N | N | Y | Y |
| **Microflow Access** | Y | N | Y | P | Y | P | 08 | Y | N | Y | Y | Y | Y | N | N | Y | Y |
| **Nanoflow Access** | Y | N | Y | P | Y | P | N | Y | N | Y | Y | Y | Y | N | N | N | N |
| **Page Access** | Y | N | Y | P | Y | P | 08 | Y | N | Y | Y | Y | Y | N | N | Y | Y |

## Project Organization

| Feature | SHOW | DESCRIBE | CREATE | OR MODIFY | DROP | ALTER | Examples | Tests | Catalog | REFS | LSP | Skills | Help | Viz | REPL | Syntax | Starlark |
|---------|------|----------|--------|-----------|------|-------|----------|-------|---------|------|-----|--------|------|-----|------|--------|----------|
| **Folders** | N | N | P | N | N | N | N | P | N | N | P | Y | Y | - | N | N | N |
| **MOVE** | - | - | - | - | - | - | N | P | N | N | P | Y | Y | - | N | Y | N |

## External SQL & Data

| Feature | Syntax | Examples | Tests | Help | Notes |
|---------|--------|----------|-------|------|-------|
| **SQL Connect/Disconnect** | `sql connect <driver> '<dsn>' as <alias>` | Y | Y | Y | PostgreSQL, Oracle, SQL Server |
| **SQL Query** | `sql <alias> <any-sql>` | Y | Y | Y | Raw SQL passthrough |
| **SQL Schema Discovery** | `sql <alias> show tables/views/FUNCTIONS` | Y | Y | Y | Schema exploration |
| **SQL Describe** | `sql <alias> describe <table>` | Y | Y | Y | Column metadata |
| **Import** | `import from <alias> query '...' into ... map (...)` | Y | Y | Y | Batch insert with ID generation |
| **Generate Connector** | `sql <alias> generate connector into <module>` | Y | Y | Y | Database Connector MDL generation |

## Catalog & Analysis

| Feature | Syntax | Tests | Help | Notes |
|---------|--------|-------|------|-------|
| **Catalog Refresh** | `refresh catalog [full]` | Y | Y | Builds queryable metadata tables |
| **Catalog Query** | `select ... from CATALOG.<table>` | Y | Y | SQL against project metadata |
| **Cross-References** | `show callers/callees/references/impact/context of` | Y | Y | Requires `refresh catalog full` |
| **Full-Text Search** | `search '<keyword>'` | Y | Y | Across all strings and source |
| **Linting** | `mxcli lint -p app.mpr` | Y | Y | 14 built-in + 27 Starlark rules |
| **Report** | `mxcli report -p app.mpr` | Y | Y | Scored best practices report |
| **Widget Discovery** | `show widgets [in module] [where ...]` | Y | Y | Experimental |
| **Widget Update** | `update widgets set ... where ...` | Y | Y | Bulk pluggable widget updates |

## Column Definitions

| Column | Description |
|--------|-------------|
| **SHOW** | `show <type> [in module]` lists all instances in a table |
| **DESCRIBE** | `describe <type> Module.Name` outputs full MDL definition |
| **CREATE** | `create <type>` creates a new instance |
| **OR MODIFY** | `create or modify <type>` supports idempotent upsert |
| **DROP** | `drop <type> Module.Name` deletes an instance |
| **ALTER** | `alter <type>` modifies without full replacement |
| **Examples** | MDL example file exists in `mdl-examples/doctype-tests/` (number = file prefix) |
| **Tests** | Roundtrip or executor tests exist in `mdl/executor/*_test.go` |
| **Catalog** | Dedicated table in `mdl/catalog/tables.go` for SQL querying |
| **REFS** | Tracked in catalog REFS table for impact analysis (`show impact of`, `show references to`) |
| **LSP** | LSP support: completions, hover, go-to-definition |
| **Skills** | Claude skill file exists in `cmd/mxcli/skills/` |
| **Help** | Documented in `cmd/mxcli/help.go` interactive help output |
| **Viz** | Mermaid diagram via `mxcli describe --format mermaid` and VS Code "Show Diagram" webview |
| **REPL** | Dynamic autocomplete in `mdl/repl/repl.go` and `mdl/executor/autocomplete.go` |
| **Syntax** | Help topic available via `mxcli syntax <topic>` (embedded `.txt` files in `cmd/mxcli/help_topics/`) |
| **Starlark** | Query function exposed in `mdl/linter/starlark.go` for custom lint rules (e.g., `entities()`, `microflows()`) |

## Gaps and Priorities

### Missing CREATE OR MODIFY

These types support CREATE but not the idempotent OR MODIFY variant:

- **Associations** — Would allow idempotent association creation
- **Pages** — No OR MODIFY; use ALTER PAGE for modifications
- **Snippets** — No OR MODIFY; use ALTER SNIPPET for modifications
- **Java Actions** — Would allow updating parameter signatures
- **Module Roles** — Would allow updating description
- **User Roles** — Has ALTER but not OR MODIFY
- **Demo Users** — Would allow updating password/roles
- **Modules** — Would be a no-op if module exists

### Missing Catalog Tables

These types have no dedicated catalog table for SQL querying:

- **Associations** — Queryable only via entity relationships
- **Constants** — Not queryable via SELECT
- **Module Roles / User Roles / Demo Users** — User/module role mappings in `CATALOG.ROLE_MAPPINGS`; demo users not in catalog

### Missing Help Documentation

These types are not covered in `help.go` output:

- **Constants** — No help topic

### Missing Skills

- **Layouts** — Read-only, no skill needed
- **Constants** — No dedicated skill

### Missing Tests

- **Navigation** — No roundtrip tests yet (manual testing only)

### Missing Examples

- **Layouts** — Read-only, no example needed
- **Folders / MOVE** — No dedicated example file

### Missing REPL Autocomplete

- **Constants** — DESCRIBE CONSTANT exists but no dynamic name completion function
- **Navigation** — No REPL autocomplete for navigation profiles
- **Project Settings** — Static commands only, no element completion
- **Security features** — No REPL autocomplete for roles, access grants
- **Nanoflows** — No dedicated GetNanoflowNames completer (only keyword completion)

### Missing Syntax Topics

- **Constants** — No `mxcli syntax constant` topic
- **Nanoflows** — No dedicated syntax topic (covered by microflow topic)
- **Layouts** — Read-only, no syntax topic
- **Modules** — No dedicated syntax topic

### Missing Starlark APIs

- **Associations** — Not queryable from Starlark rules
- **Java Actions** — Not queryable from Starlark rules
- **Constants** — Not queryable from Starlark rules
- **OData Clients/Services** — Not queryable from Starlark rules
- **Navigation** — Not queryable from Starlark rules
- **Business Events** — Not queryable from Starlark rules
- **Nanoflows** — Not independently queryable (included in `microflows()` results)
- **Layouts** — Not queryable from Starlark rules
- **Modules** — Module dependencies available in Go context but not exposed to Starlark

### Missing Visualizations

Mermaid diagram support (`mxcli describe --format mermaid` + VS Code "Show Diagram" webview) exists for:

- **Domain Model** (Entities/Associations) — `erDiagram` with attributes, cardinality, generalizations
- **Microflows** — `flowchart TD` with activities, splits, merge points, case labels
- **Nanoflows** — `flowchart TD` with activities, splits, merge points (same logic as microflows)
- **Pages** — `block-beta` with widget tree structure

Not yet implemented:

- **Enumerations** — Could render as a simple table or list diagram
- **Snippets** — Same widget tree logic as pages, not yet wired up
- **Call graphs** — `show context of` / `show callers of` as directed graphs
- **Module overview** — Combined ER + dependency diagram

### Not Yet Implemented

Document types that exist in Mendix but have no MDL support:

| Feature | SHOW | DESCRIBE | CREATE | OR MODIFY | DROP | ALTER | Examples | Tests | Catalog | REFS | LSP | Skills | Help | Viz | REPL | Syntax | Starlark | Notes |
|---------|------|----------|--------|-----------|------|-------|----------|-------|---------|------|-----|--------|------|-----|------|--------|----------|-------|
| **Microflow activities** | - | - | P | - | - | - | 02 | Y | P | P | P | Y | Y | - | - | - | P | 60+ activities supported; some edge cases missing |
| **Building blocks** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Reusable page building blocks |
| **Styling** | P | P | P | N | N | N | N | N | N | N | N | P | N | P | N | N | N | Class/Style/DesignProperties on widgets; full theme system not yet |
| **Extensions** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Mendix extensions / add-ons |
| **Custom JS actions** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | JavaScript actions for nanoflows |
| **Custom widgets** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Pluggable widget packages |
| **REST publish** | Y | Y | Y | N | Y | N | N | N | Y | N | P | N | Y | N | N | Y | N | Published REST services (CREATE/DROP/SHOW/DESCRIBE) |
| **REST consume (v2)** | N | N | N | N | N | N | 06 | N | N | N | N | Y | N | N | N | N | N | Consumed REST services; partial grammar exists |
| **Web service publish** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Published SOAP web services |
| **Web service consume** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Consumed SOAP web services |
| **Ext. DB connector** | N | N | N | N | N | N | 05 | N | N | N | N | Y | N | N | N | N | N | External database connections |
| **Import mappings** | Y | Y | Y | N | Y | N | 06 | Y | N | N | P | Y | N | N | N | Y | N | JSON import mappings with v2 syntax |
| **Export mappings** | Y | Y | Y | N | Y | N | 06 | Y | N | N | P | Y | N | N | N | Y | N | JSON export mappings with v2 syntax |
| **JSON transformations** | Y | Y | Y | Y | Y | N | 20 | Y | N | N | P | N | N | N | N | N | N | JSON structure definitions |
| **Message definitions** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Message definition documents |
| **XML schemas** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Imported XML schema documents |
| **Workflows** | Y | Y | Y | N | Y | N | N | N | Y | Y | N | N | Y | N | N | N | N | SHOW/DESCRIBE/CREATE/DROP implemented; GRANT/REVOKE removed (workflows lack AllowedModuleRoles) |
| **Module settings** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Module-level configuration |
| **Image collection** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Image document collections |
| **Icon collection** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Icon/glyph collections |
| **Task queue** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Background task queue config |
| **Rules** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Microflow rules (decision logic) |
| **Regular expressions** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Reusable regex definitions |
| **Scheduled events** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Timer-triggered microflows |
| **Data importer** | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | Excel/CSV data import documents |

## Checklist for New Features

When adding a new MDL document type, ensure all dimensions are covered:

- [ ] **Grammar** — Add tokens to `MDLLexer.g4`, rules to `MDLParser.g4`, regenerate parser
- [ ] **AST** — Add statement types in `mdl/ast/` (Show, Describe, Create, Drop)
- [ ] **Visitor** — Add listener methods in `mdl/visitor/` to build AST from parse tree
- [ ] **Executor** — Add execution handlers in `mdl/executor/`
  - [ ] SHOW handler (list all, filter by module)
  - [ ] DESCRIBE handler (output MDL format)
  - [ ] CREATE handler (with OR MODIFY support)
  - [ ] DROP handler
  - [ ] ALTER handler (if applicable)
- [ ] **Catalog** — Add table in `mdl/catalog/tables.go` and builder in `builder_modules.go`
- [ ] **REFS** — Track cross-references in `refs` table for impact analysis
- [ ] **LSP** — Add completions in `cmd/mxcli/lsp_completions_gen.go`, hover/definition in `lsp.go`
- [ ] **REPL** — Add autocomplete entries in `mdl/repl/repl.go` (prefix completer) and `mdl/executor/autocomplete.go` (dynamic name completions)
- [ ] **Syntax** — Add help topic file in `cmd/mxcli/help_topics/<topic>.txt` and register in `cmd/mxcli/help.go`
- [ ] **Starlark** — Expose query function in `mdl/linter/starlark.go` (e.g., `my_types()`) and conversion in `context.go`
- [ ] **Help** — Document in `cmd/mxcli/help.go`
- [ ] **CLAUDE.md** — Add to syntax quick reference
- [ ] **Examples** — Create `mdl-examples/doctype-tests/NN-<feature>-examples.mdl`
- [ ] **Tests** — Add roundtrip tests in `mdl/executor/roundtrip_test.go`
- [ ] **Skills** — Create or update skill file in `cmd/mxcli/skills/`
- [ ] **VS Code** — Ensure syntax highlighting covers new keywords in `vscode-mdl/`
- [ ] **Viz** — Add Mermaid diagram generator in `mdl/executor/cmd_mermaid.go` (if visual representation is useful)
- [ ] **Init docs** — Update generated CLAUDE.md template in `cmd/mxcli/init.go`
