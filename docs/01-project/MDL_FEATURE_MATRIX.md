# MDL Feature Completeness Matrix

This document tracks the implementation status of each MDL feature along **two axes**:

1. **Backend coverage** (the section right below) — what each *capability layer* can do:
   Mendix platform → MDL language → MPR backend (on-disk writer) → MCP backend
   (live Studio Pro via PED). This answers "can I do X, and through which path?"
2. **Tooling dimensions** (from *Core Document Types* onward) — for each MDL feature,
   which facets are wired (SHOW/DESCRIBE/CREATE/ALTER, examples, tests, catalog, LSP,
   skills, help, …). This answers "is feature X fully built out as an MDL feature?"

When adding a new MDL feature, use **both** as a checklist to ensure complete coverage.
(The *Agent × tooling at a glance* section just below is a one-slide overview that
sits above both axes — how the whole Maia/mxcli spectrum compares.)

**Legend:** Y = Yes | N = No | P = Partial | - = Not Applicable

## Agent × tooling at a glance (Maia vs mxcli)

One-slide comparison of how a Mendix app can be edited by an AI agent across four
setups: **Maia** (Studio Pro's built-in agent — reads/writes the model through
Studio Pro's *internal* model APIs) → **Maia + MCP** (those same APIs now exposed
via the PED MCP server) → **mxcli** (external CLI, file-based on the on-disk `.mpr`,
project closed) → **mxcli + MCP** (external CLI driving the **live** model while
Studio Pro is open).

The first two columns share Maia's **native reach** — MCP doesn't change
*what Maia can do*; they expose that reach to **external** agents (the bottom
*drivable externally* row). PED is exactly that exposure of Studio Pro's model APIs,
and mxcli (cols 3–4) is the external agent consuming it. The capability rows show
each setup's *own* reach; a Maia–vs–mxcli-live gap on a row is either a PED-subset
limit or just where each agent's implementation has gotten to.

Legend: ✓ full · ◑ partial / subset · ✗ none · — n/a.

| Capability | Maia | Maia + MCP | mxcli (file) | mxcli + MCP |
|------------|:----:|:----------:|:------------:|:-----------:|
| **Authoring** | | | | |
| Domain model — entities, view entities, enums, associations, constraints | ✓ | ✓ | ✓ | ◑¹ |
| Pages (incl. pluggable widgets) | ◑² | ◑² | ✓ | ◑² |
| Microflows | ◑³ | ◑³ | ✓ | ◑³ |
| Workflows | ✓ | ✓ | ✓ | ✓ |
| Theming / styling | ✓ | ✓ | ◑⁴ | ◑⁴ |
| Integrations — REST/OData/SOAP, business events, mappings, mobile | ✗ | ✗ | ◑⁵ | ✗ |
| Security — roles & access | ✗ | ✗ | ✓ | ✗ |
| Java / JS / CSS source | ✓⁶ | ✓⁶ | ✓ | ✓ |
| **Search & analysis** | | | | |
| Impact / lint / catalog SQL / full-text | ✗ | ✗ | ✓ | ✓ |
| Mendix KB search / NL→OQL | ✓ | ✓ | ✗ | ✗ |
| **Reach & workflow** | | | | |
| Validation before writing | ◑⁷ | ◑⁷ | ✓ | ✓ |
| Batch / scriptable / CI | ✗ | ✗ | ✓ | ✓ |
| Live edits in the open IDE | ✓ | ✓ | ✗⁸ | ✓ |
| Drivable by an external agent (headless) | ✗ | ◑⁹ | ✓ | ✓ |

¹ mxcli over MCP can't author indexes or entity validation rules (file mode can);
Maia does constraints, not indexes. &nbsp; ² Pluggable widgets: Maia not yet; mxcli
over MCP does ComboBox / DataGrid 2 / Gallery (growing), all widgets in file mode.
&nbsp; ³ Maia not yet for complex flows; mxcli covers 60+ activities (a few rejected
over MCP — show-page, cast, retrieve sort/range). &nbsp; ⁴ Maia: full theming;
mxcli: per-widget styling (`ALTER STYLING`), not a full theme system. &nbsp; ⁵ None
over MCP or in Maia; mxcli-file has partial REST-publish / mappings / business-event
reads. &nbsp; ⁶ Maia via the `write_file` virtual FS; mxcli edits source on disk
directly (any mode). &nbsp; ⁷ Maia: live, reactive validation in the IDE; mxcli:
static pre-flight (`mxcli check`) *before* writing. &nbsp; ⁸ File mode edits the
on-disk `.mpr`; reopen Studio Pro to see changes. &nbsp; ⁹ MCP exposes Maia's
read + authoring surface (the PED subset) to external agents; Studio Pro 11.16
adds delete (`ped_delete_document`) and run (the `mendix_app` handler), though on
11.16.0 delete fails for callers outside Studio Pro's own agent session. Maia performs all of this natively in-IDE — its
column is about *external* reach.

**Takeaway:** Maia has broad **native** authoring (domain model, pages, microflows,
workflows, theming, source), and **MCP is what exposes that surface to external
agents** — the architectural point. mxcli is that external agent: **mxcli (file)** is
the only setup with *full* authoring (indexes, security, integrations) plus analysis
+ pre-flight validation + batch/CI, at the cost of the project being closed;
**mxcli + MCP** keeps the analysis / validation / scripting while editing
the **live** model (authoring capped to the PED subset, which differs slightly from
Maia's — pluggable widgets yes, theming/source via disk). Maia owns the
conversational + Mendix-KB experience and in-IDE liveness. (Maia capabilities per the
Mendix team, 2026-06; re-confirm before publishing as they evolve.)

## Backend Coverage (Mendix / MDL / MPR / MCP)

The capability layers are a **nested subset stack** — each can do at most what the layer
above it offers:

> **Mendix** (the platform metamodel) ⊇ **MDL** (the subset the language expresses) ⊇
> **MPR** (on-disk BSON writer — the mature default) **&** **MCP** (live PED path — newer, narrower).

Cells are coverage-level only (Y/P/N); the **detail lives in each layer's canonical home**,
linked from the notes — this matrix never restates it:

- **Mendix** ceiling and the metamodel domains MDL does *not* yet expose (REST, mappings,
  47 of 52 domains): [`SDK_EQUIVALENCE.md`](SDK_EQUIVALENCE.md).
- **MDL** syntax for each feature: [`MDL_QUICK_REFERENCE.md`](MDL_QUICK_REFERENCE.md).
- **MCP** per-feature shapes, gaps, and Studio-Pro-version surface:
  [`../03-development/PED_MCP_CAPABILITIES.md`](../03-development/PED_MCP_CAPABILITIES.md).
- **MPR** is the reference backend the executor was built on; it realizes everything MDL
  expresses (read `modelsdk/` and `mdl/backend/modelsdk/` for specifics).

`Mendix` and `MDL` are `Y` for every row below *by construction* (a row exists only because
MDL expresses a Mendix feature); they are kept as columns to make the stack explicit. The
live distinction is **MPR vs MCP**.

### Core document types

| Feature | Mendix | MDL | MPR | MCP | MCP notes (detail → PED_MCP_CAPABILITIES.md) |
|---------|:------:|:---:|:---:|:---:|----------------------------------------------|
| **Modules** | Y | Y | Y | P | CREATE only (flushes to disk); no module DROP |
| **Entities** | Y | Y | Y | Y | CREATE/DROP; ALTER add/drop/rename/docs — MODIFY *type* rejected (PED can't migrate) |
| **Associations** | Y | Y | Y | P | Within-module CREATE/DROP; no custom delete behavior; cross-module / external N |
| **Enumerations** | Y | Y | Y | P | CREATE; DROP via `ped_delete_document` (Studio Pro 11.16+); ALTER (modify values) N |
| **Microflows** | Y | Y | Y | P | Broad CREATE (most actions + control flow); rejects show-page, cast, retrieve sort/range, inheritance/rule splits |
| **Nanoflows** | Y | Y | Y | N | **Not creatable** — PED's `ped_create_document` rejects `Microflows$Nanoflow` (create whitelist covers microflows, not nanoflows); CREATE/ALTER rejected with a clear error. DROP of an existing nanoflow uses `ped_delete_document` (Studio Pro 11.16+). |
| **Pages** | Y | Y | Y | P | CREATE + ALTER via `pg_*`; widget/data-source coverage grows one type at a time |
| **Snippets** | Y | Y | Y | P | ALTER via the page mutator; CREATE not wired |
| **Layouts** | Y | Y | Y | N | Not wired (reads delegate to the local `.mpr`) |
| **View Entities** | Y | Y | Y | P | CREATE; CREATE OR REPLACE fails (no delete-document); DROP orphans the source doc |
| **Workflows** | Y | Y | Y | Y | **Complete** — CREATE / CREATE OR REPLACE / DROP / ALTER, all activity types & ops, any nesting depth |
| **Java Actions** | Y | Y | Y | N | **Not authorable** — PED refuses to create `JavaActions$JavaAction` (backed by a `.java` source the IDE manages); CREATE/ALTER rejected with a clear error. *Calling* a Java action from a microflow IS supported. |
| **Constants** | Y | Y | Y | P | CREATE / CREATE OR MODIFY (value + exposed-to-client) / DROP (`ped_delete_document`, Studio Pro 11.16+); type limited to String/Integer/Decimal/Boolean/DateTime; in-place type change rejected; documentation not carried |
| **OData Clients / Services** | Y | Y | Y | N | Not wired |
| **External Entities** | Y | Y | Y | N | Not wired |
| **Business Events** | Y | Y | Y | P | SHOW/DESCRIBE read the local `.mpr`; DROP via `ped_delete_document` (Studio Pro 11.16+). **CREATE/ALTER blocked** — PED won't create `BusinessEvents$BusinessEventService` (off the create whitelist). The supporting domain model (published-event entities `extends BusinessEvents.PublishedBusinessEvent`, constants) IS creatable. |
| **Navigation** | Y | Y | Y | N | Not wired |
| **Project Settings** | Y | Y | Y | N | Not wired |

### Security

| Feature | Mendix | MDL | MPR | MCP | MCP notes |
|---------|:------:|:---:|:---:|:---:|-----------|
| **Module / user roles, demo users, project security, entity / microflow / nanoflow / page access** | Y | Y | Y | N | **Not authorable via MCP** — PED's document tools reject every `Security$*` type; there is no write path. |

### Project organization

| Feature | Mendix | MDL | MPR | MCP | MCP notes |
|---------|:------:|:---:|:---:|:---:|-----------|
| **Folders** | Y | Y | Y | P | Documents can be **created into** a folder (`create <doc> … folder 'A/B'`, nested ok — the folder auto-materializes). Empty `CREATE FOLDER`, `DROP FOLDER`, `MOVE FOLDER` rejected (PED can't create-empty / delete / re-parent). Pages land at the module root (`pg_write_page` has no folderPath). |
| **MOVE** | Y | Y | Y | N | Blocked — PED can't re-parent an existing document (`folderPath` is not settable). *Not* faked via delete+recreate: that would change the document's `$ID`, dangling references and diverging from the MPR engine's in-place re-parent (backends must yield equivalent project state). Place documents in folders at *create* time instead. |

### External SQL, import, catalog & analysis

| Feature | MPR | MCP | Notes |
|---------|:---:|:---:|-------|
| **External SQL / import / generate connector / catalog / lint / report / search** | - | - | Backend-agnostic: these operate on external databases, the app's runtime DB, or a read-only catalog built from the project — **not** the model-write backend. Available regardless of MPR vs MCP. |

> **Maintenance:** when a feature gains MCP support, flip its **MCP** cell and tighten the note;
> when MDL gains a feature Mendix has but MDL didn't (closing an `SDK_EQUIVALENCE` gap), add a row.
> Keep cells coverage-level — push specifics to the linked homes.
>
> **Follow-up (drift-proofing):** this matrix is hand-maintained, so the **MCP** column can
> fall behind the code. It could instead be *generated*: the MCP backend's
> `unsupportedBackend` base (`mdl/backend/mcp/unsupported_gen.go`) already encodes exactly
> which interface methods are wired vs. rejected — a small tool could diff the implemented
> receivers against that base to emit Y/N per feature. The P-with-gap nuances (e.g. "ALTER
> MODIFY type rejected") can't be auto-derived and would stay hand-written, so realistically
> this is a *hybrid*: generate the Y/N skeleton, annotate the partials by hand. Not yet built;
> flip to generation if hand-maintenance starts drifting.

## Core Document Types

| Feature | SHOW | DESCRIBE | CREATE | OR MODIFY | DROP | ALTER | Examples | Tests | Catalog | REFS | LSP | Skills | Help | Viz | REPL | Syntax | Starlark |
|---------|------|----------|--------|-----------|------|-------|----------|-------|---------|------|-----|--------|------|-----|------|--------|----------|
| **Entities** | Y | Y | Y | Y | Y | Y | 01 | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y |
| **Associations** | Y | Y | Y | N | Y | Y | 01 | Y | N | Y | Y | Y | Y | Y | Y | Y | N |
| **Enumerations** | Y | Y | Y | Y | Y | Y | 01 | Y | Y | N | Y | Y | Y | N | Y | Y | Y |
| **Microflows** | Y | Y | Y | Y | Y | N | 02 | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y |
| **Nanoflows** | Y | Y | Y | Y | Y | N | 02b | Y | Y | Y | Y | Y | Y | Y | P | N | N |
| **Rules** | Y | Y | Y | Y | Y | N | Y | Y | Y | Y | N | Y | Y | N | N | Y | N |
| **Pages** | Y | Y | Y | N | Y | Y | 03 | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y |
| **Snippets** | Y | Y | Y | N | Y | Y | 03 | Y | Y | Y | Y | Y | Y | N | Y | Y | Y |
| **Layouts** | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y | N | Y | Y | N |
| **Java Actions** | Y | Y | Y | N | Y | N | 07 | Y | Y | Y | Y | Y | Y | N | Y | Y | N |
| **Constants** | Y | Y | Y | Y | Y | N | 09 | Y | N | P | Y | N | Y | N | P | N | N |
| **OData Clients** | Y | Y | Y | Y | Y | Y | 10 | Y | Y | P | Y | Y | Y | N | Y | Y | N |
| **OData Services** | Y | Y | Y | Y | Y | Y | 10 | Y | Y | Y | Y | Y | Y | N | Y | Y | N |
| **External Entities** | Y | Y | Y | Y | N | N | 10 | Y | Y | Y | Y | Y | Y | N | P | Y | N |
| **Modules** | Y | Y | Y | N | Y | N | all | Y | Y | Y | Y | Y | Y | N | Y | N | N |
| **Navigation** | Y | Y | Y | - | - | Y | 11 | N | Y | Y | Y | Y | Y | N | N | Y | N |
| **Business Events** | Y | Y | Y | N | Y | N | 13 | N | Y | N | Y | N | Y | N | Y | Y | N |
| **Project Settings** | Y | Y | - | - | - | Y | 14 | N | Y | Y | Y | N | Y | N | N | Y | P |
| **Task Queues** | Y | Y | Y | Y | Y | N | Y | Y | Y | N | N | Y | Y | N | Y | Y | N |
| **Scheduled Events** | Y | Y | Y | Y | Y | N | Y | Y | Y | Y | N | Y | Y | N | Y | Y | N |
| **Database Connections** | Y | Y | Y | Y | Y | N | 05 | Y | Y | N | N | Y | Y | N | Y | Y | N |
| **Regular Expressions** | Y | Y | Y | Y | Y | N | Y | Y | Y | Y | N | N | Y | N | Y | Y | N |
| **Validation Rules** | - | Y | Y | - | - | Y | Y | Y | N | Y | N | N | Y | N | N | Y | N |
| **Menus** | - | Y | Y | Y | Y | N | 26 | Y | N | N | N | N | Y | N | N | Y | N |
| **Image Collections** | Y | Y | Y | N | Y | N | 19 | Y | N | N | N | N | Y | N | Y | Y | N |
| **JavaScript Actions** | Y | Y | Y | N | Y | N | 07b | Y | Y | Y | N | N | Y | N | Y | N | N |
| **Published REST Services** | Y | Y | Y | Y | Y | N | 22 | N | Y | N | P | N | Y | N | N | Y | N |
| **REST Clients** | Y | Y | Y | Y | Y | Y | 06 | Y | Y | P | Y | Y | Y | N | Y | Y | N |
| **Import Mappings** | Y | Y | Y | N | Y | N | 21 | Y | N | N | P | Y | N | N | N | Y | N |
| **Export Mappings** | Y | Y | Y | N | Y | N | 21 | Y | N | N | P | Y | N | N | N | Y | N |
| **JSON Structures** | Y | Y | Y | Y | Y | N | 20 | Y | N | N | P | N | N | N | N | N | N |
| **Message Definitions** | Y | Y | Y | Y | Y | Y | 40 | Y | N | N | N | N | Y | N | N | Y | N |
| **Workflows** | Y | Y | Y | N | Y | Y | 24 | Y | Y | Y | N | Y | Y | N | N | Y | N |
| **AI Agent documents** | Y | Y | Y | N | Y | N | 27 | Y | N | N | N | Y | Y | N | Y | Y | N |
| **Pluggable widgets** | Y | Y | Y | - | Y | Y | 30 | Y | N | N | P | Y | Y | N | N | Y | N |
| **Data Transformers** | Y | Y | Y | N | Y | N | 23 | Y | N | N | N | N | Y | N | Y | Y | N |

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
| **Folders** | N | N | P | N | N | N | 18 | P | N | N | P | Y | Y | - | N | N | N |
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
| **Linting** | `mxcli lint -p app.mpr` | Y | Y | 19 built-in + 31 Starlark rules |
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

- **Constants** — No dedicated skill

### Missing Tests

- **Navigation** — No roundtrip tests yet (manual testing only)

### Missing Examples

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

### Partially Implemented

Features with an MDL surface that does not yet cover the whole document type.

| Feature | SHOW | DESCRIBE | CREATE | OR MODIFY | DROP | ALTER | Examples | Tests | Catalog | REFS | LSP | Skills | Help | Viz | REPL | Syntax | Starlark | Notes |
|---------|------|----------|--------|-----------|------|-------|----------|-------|---------|------|-----|--------|------|-----|------|--------|----------|-------|
| **Microflow activities** | - | - | P | - | - | - | 02 | Y | P | P | P | Y | Y | - | - | - | P | 60+ activities supported; some edge cases missing |
| **Styling** | P | Y | P | N | N | Y | Y | Y | N | N | N | P | N | P | N | N | N | Class/Style/DesignProperties on widgets via ALTER STYLING (#631); `mxcli theme` covers the file side, the full theme system is not modelled |
| **Icon collections** | Y | Y | N | N | N | N | N | Y | N | N | N | Y | Y | N | Y | Y | N | **Read-only by design** — collections ship with Atlas/the theme; DESCRIBE lists each icon's reference form for use in a widget's `icon:` |
| **Module settings** | - | - | - | - | - | Y | N | Y | N | N | N | Y | Y | N | N | Y | N | `ALTER MODULE … ADD JAR DEPENDENCY` + `mxcli sync-java-deps`; other module-level settings are not modelled |
| **REST consume** | Y | Y | Y | Y | Y | Y | 06 | Y | Y | P | Y | Y | Y | N | Y | Y | N | Consumed REST services via `CREATE CONSUMED REST SERVICE`; OpenAPI-driven generation is partial |
| **Building blocks** | Y | N | N | N | N | N | N | Y | N | N | N | N | Y | N | Y | Y | N | **Read-only** — `show building blocks`; authoring one is not supported |

### Not Yet Implemented

Document types that exist in Mendix and have **no** MDL surface at all.

| Feature | Notes |
|---------|-------|
| **XML schemas** | Imported XSD documents |
| **Web service publish / consume** | SOAP. `CALL WEB SERVICE` exists in microflows for a stored service; the service documents themselves are not authorable |
| **Data importer** | Excel/CSV import documents |
| **Extensions** | Mendix extensions / add-ons |
| **Custom widget packages** | Authoring a `.mpk`. Note this is NOT "using a pluggable widget on a page", which is supported — see **Pluggable widgets** above |
| **System text collections** (`Texts$SystemTextCollection`) | Translatable system text |
| **Data sets** | Dataset documents (reporting) |
| **Page templates** | Reusable page starting points offered by Studio Pro's New Page dialog |
| **ML model mappings** | Mapping a model to entities for the ML Kit |
| **Change data capture services** (beta) | CDC service documents |

> **Keeping this honest.** Every row above was checked against the grammar's
> statement rules and `mxcli syntax`, not against memory: a row claiming a gap
> that has since been filled is worse than no table, because it sends people to
> Studio Pro for work mxcli can do. FINDINGS #20 in `ako/mxcli-owid` is the
> worked example — the External Database Connector was recorded here as
> unsupported long after `CREATE DATABASE CONNECTION` shipped, and a reader
> designed around a blocker that no longer existed.
>
> **Where the list of rows comes from matters as much as their values.** This
> table is enumerated from **Studio Pro's own `Add other` menu** — the full set
> of document types the product offers — and not from inverting what mxcli
> happens to support. Inverting mxcli can only rediscover gaps someone already
> wrote down; enumerating the product finds the ones nobody has thought about.
> Doing that pass is what surfaced **data sets, page templates, ML model
> mappings and change-data-capture services**, none of which appeared in any
> mxcli document before. Re-run it against the menu when onboarding a new
> Mendix major.
