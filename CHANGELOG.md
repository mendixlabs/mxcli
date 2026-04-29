# Changelog

All notable changes to mxcli will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **mxcli catalog search** — Search Mendix Catalog for data sources and services with filters for service type, environment, and ownership (#213)
- **Local file metadata for OData clients** — `CREATE ODATA CLIENT` now supports `file://` URLs and relative paths for `MetadataUrl`, enabling offline development, reproducible testing, and version-pinned contracts (#206)
- **Path normalization** — Relative paths in `MetadataUrl` are automatically converted to absolute `file://` URLs for Studio Pro compatibility
- **ServiceUrl validation** — `ServiceUrl` parameter must now be a constant reference (e.g., `@Module.ConstantName`) to enforce Mendix best practice
- **Shared URL utilities** — `internal/pathutil` package with `NormalizeURL()`, `URIToPath()`, and `PathFromURL()` for reuse across components
- **@anchor sequence flow annotation** — optional `@anchor(from: X, to: Y)` annotation on microflow statements that pins the side of the activity box a SequenceFlow attaches to (top / right / bottom / left). Split (`if`) statements support the combined form `@anchor(to: X, true: (from: ..., to: ...), false: (from: ..., to: ...))` so the incoming and per-branch outgoing anchors survive describe → exec round-trip. When omitted, the builder derives the anchor from the visual flow direction (existing behaviour is unchanged). Keeps the flow diagram stable across patches when an agent-generated microflow is applied back to a project
- **@anchor loop form** — `LOOP`/`WHILE` statements accept `@anchor(iterator: (...), tail: (...))` in the grammar so authoring tools can forward-propagate the intent. Today the builder deliberately does not translate those into SequenceFlows: Studio Pro rejects edges between a `LoopedActivity` and its body statements with CE0709 ("Sequence flow is not accepted by origin or destination"), since the iterator icon is drawn implicitly from the loop geometry. Reserving the grammar slot keeps scripts forward-compatible with any future Mendix capability
- **OpenAPI import for REST clients** — `CREATE REST CLIENT` now accepts `OpenAPI: 'path/or/url'` to auto-generate a consumed REST service document from an OpenAPI 3.0 spec (JSON or YAML); operations, path/query parameters, request bodies, response types, resource groups (tags), and Basic auth are derived automatically; spec content is stored in `OpenApiFile` for Studio Pro parity (#207)
- **DESCRIBE CONTRACT OPERATION FROM OPENAPI** — Preview what would be generated from an OpenAPI spec without writing to the project

- **Flow bug fixes** — Module existence validation for SHOW NANOFLOWS and SHOW MICROFLOWS, numeric return literals no longer get spurious `$` prefix, empty flow names rejected at create time, `NanoflowCallAction` error handling type resolved correctly, `not()` expression spacing preserved on roundtrip, JavaScript action call rendering in DESCRIBE output
- **Nanoflow diff support** — `mxcli diff` now detects and displays nanoflow changes (previously silently skipped)
- **Nanoflow validation parity** — `mxcli check` now runs full body validation on nanoflows (undeclared variables, missing returns, branch scoping) via shared `validateFlowBody` helper; `allNames()` includes nanoflows for forward-reference detection
- **DownloadFileStmt denylist** — `DOWNLOAD FILE` added to nanoflow disallowed action list
- **JavaScript action MDL syntax** — `call javascript action Module.ActionName(params)` now fully supported in CREATE NANOFLOW/MICROFLOW bodies: grammar, parser, builder, serializer, and roundtrip
- **LSP snippet completions** — Added `CREATE NANOFLOW (with params)`, `CALL MICROFLOW`, `CALL NANOFLOW`, `CALL JAVASCRIPT ACTION`, `CALL JAVA ACTION` snippets
- **Flow builder cache** — `lookupMicroflowReturnType` and `lookupNanoflowReturnType` now cache results with lazy-load bool flags, avoiding repeated `ListNanoflows`/`ListMicroflows` calls; nanoflow lookup uses `GetRawUnitByName` fast path
- **Parser fixes** — Negative literals no longer greedily consumed by lexer (`-?` removed from `NUMBER_LITERAL`); XPath multi-predicate brackets correctly split and re-wrapped; multi-line string literals in change/create object expressions escaped; `ExclusiveSplit` (if/else) inside loop bodies now emits correct `end if;`
- **Empty action params** — JS and Java action parameters with empty/nil values are omitted from DESCRIBE output instead of rendering as `...` or bare `= )`
- **Association retrieve roundtrip fidelity** — `retrieve $X from $Y/Module.Association` syntax preserved on roundtrip (previously converted to `from Entity where Assoc = $Y`)
- **DESCRIBE empty-then optimization** — If/else blocks with empty true branches are swapped and condition negated for readable output

### Changed

- **MDL string literal escapes** — `mdlQuote`/`unquoteString` now treat `\n`, `\r`, `\t`, and `\\` inside single-quoted literals as escape sequences (previously a literal backslash followed by the letter). This is a compatibility break for any MDL script that intentionally embedded a raw `\n` / `\t` / `\\` as two characters; such scripts must now double the backslash (`\\n` to preserve the two-character form). Applies to `LOG` messages, `@caption`/`@annotation` text, and other string literals round-tripped via the describer.

## [0.7.0] - 2026-04-21

### Added

- **Agent Editor** — CREATE/DROP Agent, Knowledge Base, Consumed MCP Service, and Model documents; read support for all four types; DESCRIBE MODULE WITH ALL includes agent-editor documents
- **Consumed REST Client v2** — Redesigned syntax with full mapping support, parameter support for SEND REST REQUEST, BODY JSON FROM clause roundtrip, and TRANSFORM microflow action (JSLT/XSLT, Mendix 11.9+)
- **Platform Authentication** — `mxcli auth login/logout/status/list` with PAT scheme for mendix.com; credentials stored at `~/.mxcli/auth.json` (mode 0600), MENDIX_PAT env override
- **Marketplace Browsing** — `mxcli marketplace search/info/versions` with `--min-mendix` compatibility filtering
- **Entity Event Handlers** — Full MDL support for before/after create/change/delete event handlers with entity parameter validation
- **System Attributes** — AutoOwner, AutoChangedBy, and other audit pseudo-types; ALTER ENTITY ADD/DROP ATTRIBUTE for system attributes
- **ALTER PUBLISHED REST SERVICE** — Full in-place modification of published REST services (#161)
- **GRANT/REVOKE ACCESS on PUBLISHED REST SERVICE** (#162)
- **GitHub Copilot support** — First-class Copilot integration in `mxcli init`
- **Unified --json output** — All commands support structured JSON output (#134); `mxcli check --format json/sarif` outputs structured results
- **OData TripPin bulk-import** — Executable bulk-import example with @Constant syntax for ServiceUrl
- **Backend Abstraction** — `ExecContext` with typed backend interfaces, dispatch registry replacing type-switch, mutation backends (`page_mutator`, `widget_builder`, `datagrid_builder`, `workflow_mutator`) decoupled from `sdk/mpr`
- **mdl/types package** — Shared types and utilities extracted from `sdk/mpr` (EDMX, AsyncAPI, ID, navigation, infrastructure, JSON utils)
- **bsonutil package** — BSON utility functions (IDToBsonBinary, BsonBinaryToID, NewIDBsonBinary)
- **Mock-based handler tests** — 189 tests across 33 files covering all executor command handlers
- **OperationRegistry extensibility** — Pluggable operation registry with ContainerSnippet constant

### Fixed

- REST client BASIC auth uses correct `Rest$ConstantValue` BSON key (#200)
- ConnectionIndex lost on roundtrip (int64 vs int32 type mismatch) (#204)
- OData: ByAssociation DataSource serialization for DataGrid 2, capability annotations for entity/association CRUD (#201), bulk-create NPEs for primitive collections, derived/abstract/contained entities, and navigation associations (#143)
- UUID v4 version/variant bits in `GenerateDeterministicID`; panic on invalid UUID in `IDToBsonBinary`
- Cascade-delete associations on DROP ENTITY and DROP ODATA CLIENT
- Reserved keywords now allowed as module names in CREATE MODULE
- Quoted identifiers accepted in CREATE MODULE
- Find, Filter, ListRange list operations parsed and rendered (#212)
- DESCRIBE REST CLIENT resolves constant credentials to literal values (#192)
- DESCRIBE microflow roundtrip issues; eliminate redundant Merge nodes when IF branch returns
- COLUMN name falls back to attribute + scope association lookup by module (#202)
- Schema-level external `<Annotations>` blocks parsed in OData $metadata
- OData ServiceUrl validated as constant reference
- Agent-editor commands conformed to backend abstraction

### Changed

- Executor fully decoupled from storage layer — all BSON writes go through mutation backends (PRs #225, #237, #238, #239)
- All executor handlers migrated to free functions using `ExecContext` (removed 233 unused wrapper methods)
- `show*` executor functions renamed to `list*` for consistency
- Type aliases added in `sdk/mpr` for backward compatibility after shared-type extraction

## [0.6.0] - 2026-04-09

### Added

- **RENAME** — Automatic reference refactoring when renaming entities, attributes, associations, and other elements
- **CREATE EXTERNAL ENTITIES** — Bulk import entities from OData contracts (#143)
- **@excluded Annotation** — Mark documents and microflow activities as excluded, with Excluded column in catalog and `[EXCLUDED]` indicator in LIST
- **LIST Alias** — LIST as alias for SHOW in MDL and CLI
- **ALTER WORKFLOW** — Full activity manipulation (INSERT, DROP, REPLACE) for workflow definitions
- **Primitive Page Parameters** — Support for String, Integer, and other primitive types in page parameters
- **DataGrid Column Targeting** — Addressable columns in ALTER PAGE via dotted refs (e.g., `DataGrid.ColumnName`)
- **diff-local --ref** — Accept git ranges directly via `--ref` for comparing arbitrary revisions
- **Virtual System Module** — Complete module listing including System module
- **PasswordPolicy.ValidatePassword** — Demo user password validation against project policy
- **Multiple XPath Predicates** — Support `[cond1][cond2]` in WHERE clauses
- **DESCRIBE Enhancements** — Missing types added to mxcli describe command, view entity Source object preservation
- **Proposals** — Bulk external action support from OData contracts, RENAME with reference refactoring

### Fixed

- INTO clause in CREATE EXTERNAL ENTITIES not routing to target module
- Mendix 11.9.0 integration test failures
- Demo user password updated to meet 12-char policy
- JSON number type inference and mxcli new locale duplicates
- BSON properties aligned with Mendix schema for mx diff compatibility
- View entity Source object ID preserved with CREATE OR MODIFY in DESCRIBE

### Changed

- Refactored large files: executor.go (4 files), init.go (3 files), tui/app.go (4 files), cmd_entities.go (3 files)
- Simplified diff-local to accept git ranges via `--ref` directly (removed `--base` flag)
- Pre-warmed name lookup maps to eliminate O(n²) BSON parsing in catalog source
- Updated CI to test against Mendix 11.9.0
- Documentation updates: LIST preferred over SHOW, execution modes, DataGrid column targeting, IMAGE datasource properties

## [0.5.0] - 2026-04-06

### Added

- **Import/Export Mappings** — CREATE/DESCRIBE/DROP IMPORT MAPPING and EXPORT MAPPING with JSON Structure integration, array mapping, and BSON roundtrip
- **IMPORT FROM MAPPING / EXPORT TO MAPPING** — Microflow actions for mapping-based data transformation
- **JSON Structure FOLDER** — FOLDER clause for organizing JSON Structures into folders
- **DESCRIBE NANOFLOW** — Display nanoflow activities, control flows, and return type
- **Pluggable Widget Engine v2** — Redesigned widget engine with 25+ new widget templates (accordion, maps, charts, timeline, etc.), filter widget migration, and `generateDefJSON` property mapping
- **WidgetDemo** — Baseline scripts and widget analysis tools for widget testing
- **mxcli new** — Create Mendix projects from scratch (downloads MxBuild, creates project, runs init, installs Linux mxcli binary)
- **setup mxcli** — Download platform-specific mxcli binary from GitHub releases
- **Podman Support** — Podman as Docker alternative with devcontainer configuration (#34)
- **Catalog Tables** — Import/export mapping catalog tables for project metadata queries
- **Project Tree** — Missing document types added to project tree and syntax highlighting
- **GRANT Additive** — GRANT is now additive with partial REVOKE for entity access
- **Version Pre-checks** — Executor commands validate Mendix version before BSON writes
- **SHOW FEATURES** — Display version registry feature availability
- **SHOW LANGUAGES** — Language listing and QUAL005 missing translations linter rule
- **Proposals** — Design proposals for i18n, workflow improvements, and multi-project tree view
- **BSON Tooling Guide** — Contributor documentation for BSON debugging workflow
- **CONTRIBUTING.md** — Rewritten with accurate project references

### Fixed

- CE1613 and Studio Pro crash from invalid CrossAssociation BSON (ParentConnection/ChildConnection fields) (#50)
- Import/export mapping BSON alignment with Studio Pro (JsonPath, ExposedName, ObjectHandling, array elements)
- Sort translation map iteration in all serializers for deterministic output
- Docker and diaglog tests cross-platform compatibility (macOS Unix socket paths)
- Roundtrip test stability with idempotency strategy
- Version gates for Mendix 10.24 nightly test failures and 11.0+-only MOVE commands
- Nanoflow BSON parsing for activities, flows, and return type
- mxcli new MPR filename detection from create-project
- Bun setup in nightly and release workflows for vscode-ext build
- Replace unreleased Mendix 11.9.0 with 11.8.0 in CI workflows

### Changed

- Redesigned import/export mapping syntax (v2) with comma separators
- Bumped dependencies: esbuild 0.28.0, typescript 6.0.2, sqlite 1.48.1, go-runewidth 0.0.22, @vscode/vsce 3.7.1
- Bumped CI actions: checkout v6, deploy-pages v5, upload-pages-artifact v4
- Bumped mdbook to v0.5.2 with musl for aarch64
- PR review checklist requires working MDL examples for syntax changes

## [0.4.0] - 2026-03-31

### Added

- **SEND REST REQUEST** — Microflow action for consumed REST services with full BSON serialization roundtrip
- **Pluggable Image Widget** — Full roundtrip support for `com.mendix.widget.web.image.Image` with Studio Pro-extracted templates
- **ALTER PAGE SET Url** — Change page URLs via MDL
- **ALTER PAGE SET Layout** — Switch page layout via MDL
- **ALTER ENTITY SET POSITION** — Set entity position in domain model diagrams
- **VISIBLE IF / EDITABLE IF** — Conditional visibility and editability with XPath expressions, plus TabletWidth/PhoneWidth properties
- **EXECUTE DATABASE QUERY** — Microflow action for static, dynamic, and parameterized SQL with runtime connection override
- **Contract Browsing** — SHOW/DESCRIBE CONTRACT ENTITIES/ACTIONS from cached OData $metadata, CONTRACT CHANNELS/MESSAGES from AsyncAPI
- **Integration Catalog** — 7 new catalog tables (rest_clients, rest_operations, published_rest_services, external_entities, external_actions, business_events, contract tables)
- **SHOW EXTERNAL ACTIONS / PUBLISHED REST SERVICES** — Integration pane commands
- **SHOW CONSTANT VALUES** — Display constant values and catalog tables
- **CREATE/DROP CONFIGURATION** — Configuration management with constant overrides
- **JavaScript Actions** — NDSL/MDL support for JavaScript action definitions
- **DROP/MOVE FOLDER** — Remove empty folders and reorganize project structure
- **GALLERY Columns** — DesktopColumns/TabletColumns/PhoneColumns properties
- **Forward-Reference Hints** — Helpful error messages when exec fails on later-defined objects
- **IMAGE FROM FILE** — Image collection syntax for file-based images
- **OpenSSF Baseline Level 1** — Security foundations and CodeQL fixes
- **Multi-Agent Merge Proposal** — Design proposal for parallel agent work on Mendix projects
- **Documentation Site** — mdBook-based site with tutorials, language reference, migration guide, and internals
- **Tool Integrations** — Added support for OpenCode, Mistral Vibe, and GitHub Copilot in `mxcli init`
- **TUI Enhancements** — Agent channel (Unix socket), UX improvements, auto-create module support
- **Custom Widget AIGC Skill** — Skill for AI-generated custom pluggable widgets
- **AI Issue Triage** — GitHub Actions workflow for automated issue classification
- **Daily Project Digest** — Scheduled workflow for project activity summaries

### Fixed

- Skip null TextTemplate in opTextTemplate to avoid CE0463 widget definition errors
- Set Editable to Conditional and fix Visible XPath expression serialization
- REST client BSON serialization field ordering and roundtrip correctness
- Image widget template extraction (imageObject defaults, Parameters version marker, Texts$Translation)
- Escape single quotes in page DESCRIBE output via `mdlQuote()`
- Resolve association/attribute and entity/enumeration ambiguity in MDL parser
- LSP diagnostics for editable `mendix-mdl://` documents
- Gallery CE0463 by re-extracting template and fixing augmentation
- DataGrid2 column name derivation from attribute or caption
- ComboBox association EntityRef via IndirectEntityRef with association path
- XPath tokens written unquoted to prevent CE0161
- Long type written as `DataTypes$LongType` instead of IntegerType
- Date as distinct type from DateTime throughout the pipeline
- MPR version detection using DB schema and `_FormatVersion` field
- Recurse into loop bodies when extracting catalog references
- CodeQL symlink path traversal alerts in tar extraction
- Multiple TUI data races and agent channel stability fixes

### Changed

- Bumped dependencies: pgx v5.9.1, zap v1.27.1, go-runewidth v0.0.21, cobra v1.10.2, mongo-driver v1.17.9, sqlite v1.48.0
- Refactored Visible/Editable syntax to `visible: [xpath]` and `editable: [xpath]`
- Used dedicated CWTest module in custom widget examples
- Always-quoted identifiers in MDL to prevent reserved keyword conflicts
- Added scope & atomicity and documentation sections to PR review checklist

## [0.3.0] - 2026-03-26

### Added

- **TUI** — Interactive terminal UI (`mxcli tui`) with yazi-style Miller columns, BSON/MDL preview, search, tabs, command palette (`:` key), session restore (`-c`), and mouse support
- **Workflows** — Full CREATE/DESCRIBE WORKFLOW support with activities (UserTask, Decision, CallMicroflow, CallWorkflow, Jump, WaitForTimer, ParallelSplit, BoundaryEvent), BSON round-trip, and ANNOTATION statements
- **Consumed REST Clients** — SHOW/DESCRIBE/CREATE consumed REST services with BSON writer and mx check validation
- **Image Collections** — SHOW/DESCRIBE/CREATE/DROP IMAGE COLLECTION with BSON writer and Kitty/iTerm2/Sixel inline image rendering in TUI
- **WHILE Loops** — WHILE loop support in microflows with examples
- **ALTER PAGE Variables** — ALTER PAGE ADD/DROP VARIABLE support (Phase 3)
- **XPath** — Dedicated XPath expression grammar, catalog table population, and skills reference
- **BSON Tools** — `bson dump --format ndsl`, `bson compare` with smart array matching, `bson discover` for field coverage analysis
- **Documentation Site** — mdBook-based site with full language reference, tutorials, and internals documentation
- **Anti-pattern Detection** — `mxcli check` detects nested loops and empty list anti-patterns (issue #21)
- **CREATE OR MODIFY** — Additive upsert for USER ROLE and DEMO USER
- **AI PR Review** — GitHub Actions workflow using GitHub Models API for automated pull request review
- **RETRIEVE FROM $Variable** — Support for in-memory and NPE list association traversal (issue #22)
- **Constants** — Constant syntax help topic, LSP snippet, and CREATE OR MODIFY examples
- **UnknownElement Fallback** — Table-driven parser registries with graceful fallback for unrecognized BSON types (issue #19)

### Fixed

- MPR corruption from dangling GUIDs after attribute drop/add (#4)
- BSON field ordering loss in ALTER PAGE operations (#3)
- ALTER PAGE SET Attribute property support (issue #10)
- ALTER PAGE REPLACE deep GUID regeneration for stale $ID fields (issue #9)
- Quoted identifiers not resolved in page widget references (issue #8)
- DATAGRID placeholder ID leak during template augmentation (issue #6)
- COMBOBOX association EntityRef via IndirectEntityRef with association path
- Page/layout unit type mismatch (Forms$ vs Pages$ prefix)
- VIEW entity types, constant value BSON, and test error detection
- False positive OQL type inference for CASE expressions
- RETRIEVE using DatabaseRetrieveSource for reverse Reference association traversal
- RETURNS Void treated as void return type like Nothing
- ANNOTATION keyword added to annotationName grammar rule
- System entity types and RETURN keyword formatting in microflows
- 10 CodeQL security alerts
- XPath token quoting for `[%CurrentDateTime%]` (#1)
- DROP MODULE/ROLE cascade-removes module roles from user roles
- Security script CE0066 entity access out-of-date errors
- Slow integration tests with build tags and TestMain (issue #16)
- Docker run failing on fresh projects (issue #13)

### Changed

- Aligned `mxcli check` and `mxcli lint` reporting with shared Violation format (issue #10)
- Promoted BSON commands from debug-only to release build
- Auto-discover `.mpr` file when `-p` is omitted
- Moved `bson/` and `tui/` packages under `cmd/mxcli/` for better encapsulation
- Consolidated show-describe proposals into `docs/11-proposals/` with archive
- Documented association ParentPointer/ChildPointer semantics in CLAUDE.md
- Normalized CRLF to LF in bug reports via `.gitattributes`

## [0.2.0] - 2026-03-15

### Added

- **CI/CD** — GitHub Actions workflow for build, test, and lint on push; release workflow for tagged versions
- **Makefile Lint Targets** — `make lint`, `make lint-go` (fmt + vet), `make lint-ts` (tsc --noEmit)
- **Playwright Testing** — Browser name config support, port-offset fixes, project directory CWD for session discovery
- **VS Code Extension** — Project tree auto-refresh via file watchers, association cardinality label fix

### Fixed

- Enum truncation, DROP+CREATE cache invalidation, duplicate variable detection, subfolder enum resolution
- IMPORT FK column NULL fallback and entity attribute validation
- Docker exec using host port instead of container-internal port
- AGGREGATE syntax in skills docs
- Association cardinality labels in domain model diagrams
- 3 MDL bugs and standardized enum DEFAULT syntax

### Changed

- Default to always-quoted identifiers in MDL to prevent reserved keyword conflicts
- Communication Style section in generated CLAUDE.md for human-readable change descriptions
- Shortened mxcli startup warning to single line
- Chromium system dependencies added to devcontainer Dockerfile

## [0.1.0] - 2026-03-13

First public release.

### Added

- **MDL Language** — SQL-like syntax (Mendix Definition Language) for querying and modifying Mendix projects
- **Domain Model** — CREATE/ALTER/DROP ENTITY, CREATE ASSOCIATION, attribute types, indexes, validation rules
- **Microflows & Nanoflows** — 60+ activity types, loops, error handling, expressions, parameters
- **Pages** — 50+ widget types, CREATE/ALTER PAGE/SNIPPET, DataGrid, DataView, ListView, pluggable widgets
- **Page Variables** — `variables: { $name: type = 'expression' }` in page/snippet headers for column visibility and conditional logic
- **Security** — Module roles, entity access rules, GRANT/REVOKE, UPDATE SECURITY reconciliation
- **Navigation** — Navigation profiles, menu items, home pages, login pages
- **Enumerations** — CREATE/ALTER/DROP ENUMERATION with localized values
- **Business Events** — CREATE/DROP business event services
- **Project Settings** — SHOW/DESCRIBE/ALTER for runtime, language, and theme settings
- **Database Connections** — CREATE/DESCRIBE DATABASE CONNECTION for Database Connector module
- **Full-text Search** — SEARCH across all strings, messages, captions, labels, and MDL source
- **Code Navigation** — SHOW CALLERS/CALLEES/REFERENCES/IMPACT/CONTEXT for cross-reference analysis
- **Catalog Queries** — SQL-based querying of project metadata via CATALOG tables
- **Linting** — 14 built-in rules + 27 Starlark rules across MDL, SEC, QUAL, ARCH, DESIGN, CONV categories
- **Report** — Scored best practices report with category breakdown (`mxcli report`)
- **Testing** — `.test.mdl` / `.test.md` test files with Docker-based runtime validation
- **Diff** — Compare MDL scripts against project state, git diff for MPR v2 projects
- **External SQL** — Direct queries against PostgreSQL, Oracle, SQL Server with credential isolation
- **Data Import** — IMPORT FROM external DB into Mendix app PostgreSQL with batch insert and ID generation
- **Connector Generation** — Auto-generate Database Connector MDL from external schema discovery
- **OQL** — Query running Mendix runtime via admin API
- **Docker Build** — `mxcli docker build` with PAD patching
- **VS Code Extension** — Syntax highlighting, diagnostics, completion, hover, go-to-definition, symbols, folding
- **LSP Server** — `mxcli lsp --stdio` for editor integration
- **Multi-tool Init** — `mxcli init` with support for Claude Code, Cursor, Continue.dev, Windsurf, Aider
- **Dev Container** — `mxcli init` generates `.devcontainer/` configuration for sandboxed AI agent development
- **MPR v1/v2** — Automatic format detection, read/write support for both formats
- **Fluent API** — High-level Go API (`api/` package) for programmatic model manipulation
