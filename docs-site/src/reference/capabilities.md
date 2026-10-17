# Capabilities Overview

Everything mxcli can do, organized by use case.

## Read: Explore and understand your project

| Capability | Command | Notes |
|---|---|---|
| List modules | `LIST MODULES` | With marketplace version info |
| List entities | `LIST ENTITIES [IN Module]` | With attribute/association counts |
| List microflows | `LIST MICROFLOWS [IN Module]` | With parameter/activity counts |
| List pages | `LIST PAGES [IN Module]` | With widget counts |
| List enumerations | `LIST ENUMERATIONS [IN Module]` | |
| List constants | `LIST CONSTANTS [IN Module]` | |
| List workflows | `LIST WORKFLOWS [IN Module]` | |
| List nanoflows | `LIST NANOFLOWS [IN Module]` | |
| List layouts | `LIST LAYOUTS [IN Module]` | |
| List snippets | `LIST SNIPPETS [IN Module]` | |
| Compact overview | `DESCRIBE STRUCTURE [DEPTH 1\|2\|3]` | Tree view of entire project |
| Describe any document | `DESCRIBE ENTITY\|MICROFLOW\|PAGE ...` | Full MDL output (re-executable) |
| Full-text search | `SEARCH 'keyword'` | Across all strings and source |
| Show languages | `LIST LANGUAGES` | All languages in the project |
| Describe app security | `DESCRIBE APP SECURITY` | Security overview |
| Show access rules | `LIST ACCESS ON [ENTITY] Module.Name` | Entity/microflow/page/nanoflow access |
| Show settings | `LIST SETTINGS` | Project-level settings |

## Write: Create and modify documents

### Domain Model

| Capability | Command | Status |
|---|---|---|
| Create entity | `CREATE PERSISTENT ENTITY` | Full support |
| Create non-persistent entity | `CREATE NON-PERSISTENT ENTITY` | Full support |
| Create view entity (OQL) | `CREATE VIEW ENTITY ... AS SELECT` | 10.18+ |
| Modify entity | `ALTER ENTITY ... ADD/MODIFY/DROP ATTRIBUTE` | Full support |
| Create association | `CREATE ASSOCIATION ... FROM ... TO` | Full support |
| Create enumeration | `CREATE ENUMERATION` | Full support |
| Create/modify idempotently | `CREATE OR MODIFY ENTITY` | Full support |

### Microflows and Nanoflows

| Capability | Command | Status |
|---|---|---|
| Create microflow | `CREATE MICROFLOW ... BEGIN ... END` | 60+ activity types |
| Create nanoflow | `CREATE NANOFLOW ... BEGIN ... END` | Client-side flows |
| CRUD operations | `CREATE`, `CHANGE`, `COMMIT`, `DELETE` | Object manipulation |
| Retrieve | `RETRIEVE ... FROM ... WHERE` | Database/association queries |
| Control flow | `IF/THEN/ELSE`, `LOOP`, `WHILE` | Including nested |
| Call flows | `CALL MICROFLOW`, `CALL NANOFLOW` | With parameters |
| Show page | `LIST PAGE Module.Page(...)` | With page parameters (9.7+) |
| REST requests | `SEND REST REQUEST` | GET/POST/PUT/DELETE |
| Database queries | `EXECUTE DATABASE QUERY` | External databases |
| Log messages | `LOG INFO\|WARNING\|ERROR` | With templates |
| Error handling | `RAISE ERROR`, error handlers | On activities |
| Validation | `VALIDATION FEEDBACK` | Field-level validation |

### Pages

| Capability | Command | Status |
|---|---|---|
| Create page | `CREATE PAGE ... { widgets }` | 50+ widget types |
| Create snippet | `CREATE SNIPPET` | Reusable widget fragments |
| Modify page | `ALTER PAGE ... SET/INSERT/DROP/REPLACE` | In-place modifications |
| Built-in widgets | TEXTBOX, TEXTAREA, DATEPICKER, etc. | All standard widgets |
| Layout widgets | LAYOUTGRID, CONTAINER, GROUPBOX | With responsive columns |
| Display widgets | DYNAMICTEXT, IMAGE | Including pluggable Image |
| Data widgets | DATAVIEW, LISTVIEW | With datasource binding |
| Pluggable widgets | DATAGRID2, GALLERY, COMBOBOX | Template-based |
| Action buttons | ACTIONBUTTON | Save, cancel, microflow, page |
| Navigation | NAVIGATIONLIST | With items and actions |

### Security

| Capability | Command | Status |
|---|---|---|
| Create module roles | `GRANT ... TO Role` | Entity/microflow/page access |
| Create user roles | via security commands | Map to module roles |
| Entity access rules | `GRANT READ/WRITE ON Entity TO Role` | With XPath constraints |
| Demo users | `CREATE DEMO USER` | For testing |

### Integration

| Capability | Command | Status |
|---|---|---|
| REST clients | `CREATE CONSUMED REST SERVICE` | Consumed REST services |
| OpenAPI import | `CREATE CONSUMED REST SERVICE (OpenAPI: '...')` | Generate REST client from OpenAPI 3.0 spec |
| Business events | `CREATE BUSINESS EVENT SERVICE` | Event definitions |
| Database connections | `CREATE DATABASE CONNECTION` | External SQL |
| OData services | `LIST CONSUMED ODATA SERVICES/SERVICES` | Read-only browsing |

### Navigation and Settings

| Capability | Command | Status |
|---|---|---|
| Navigation profiles | `ALTER NAVIGATION` | Home pages, menus |
| Project settings | `ALTER SETTINGS` | Java version, theme, etc. |

### Workflows

| Capability | Command | Status |
|---|---|---|
| Create workflow | `CREATE WORKFLOW` | User tasks, decisions |
| Parallel splits | Supported | Fork/join patterns |

## Analyze: Code intelligence and quality

| Capability | Command | Notes |
|---|---|---|
| Cross-references | `LIST CALLERS/CALLEES OF` | Who calls what |
| Impact analysis | `LIST IMPACT OF Module.Entity` | What breaks if I change this |
| Transitive callers | `LIST CALLERS OF ... TRANSITIVE` | Full call chain |
| Linting | `mxcli lint -p app.mpr` | 19 built-in + 31 Starlark rules |
| Best practices report | `mxcli report -p app.mpr` | Scored report with categories |
| Missing translations | QUAL005 linter rule | Detects incomplete translations |
| Catalog queries | `SELECT ... FROM CATALOG.tables` | SQL over project metadata |
| Validation | `mxcli check -p app.mpr` | Syntax + optional `mx check` |
| Diff | `mxcli diff -p app.mpr changes.mdl` | Compare script vs project |

## Automate: CI/CD and scripting

| Capability | Command | Notes |
|---|---|---|
| Inline commands | `mxcli -p app.mpr -c "CMD1; CMD2; CMD3"` | Semicolon-separated, single connection |
| Script files | `mxcli exec script.mdl -p app.mpr` | Run MDL files |
| Stdin piping | `echo "CMD" \| mxcli -p app.mpr` | Quiet mode, no prompts |
| Docker build | `mxcli docker build -p app.mpr` | Build MDA in container |
| Docker check | `mxcli docker check -p app.mpr` | Validate in container |
| Testing | `mxcli test tests/ -p app.mpr [--local] [--watch] [--attach]` | `.test.mdl` / `.test.md`; `--local` needs no Docker, `--watch` keeps the runtime warm (~2s per run), `--attach` runs against an app already up |
| SARIF output | `mxcli lint --format sarif` | For CI integration |
| New project | `mxcli new <name> --version X.Y.Z` | Create project from scratch with all tooling |
| Init project | `mxcli init` | Set up `.claude/` with skills |
| Setup mxcli | `mxcli setup mxcli [--os linux]` | Download platform-specific mxcli binary |
| Platform login | `mxcli auth login` | Store a Mendix PAT for marketplace access |
| Marketplace browse | `mxcli marketplace search/info/versions` | Discover content; `--min-mendix` compatibility filter |
| Marketplace download | `mxcli marketplace download <id> [--version X]` | Fetch a content version's `.mpk` to disk |
| Marketplace install | `mxcli marketplace install <id> -p app.mpr` | Widget → `widgets/`; new module → `mx module-import`; existing module reported, not updated |
| Marketplace package | `mxcli marketplace package <Module> -p app.mpr [-o dir]` | Export a module as a `.mpk` via `mx create-module-package` (module's own libs only; warns about other modules' files); installs back with `install --file` |

## Known Limitations

| Area | Limitation | Workaround |
|---|---|---|
| Design properties (Atlas v3) | Requires Mendix 11.0+ | Use CSS classes on 10.x |
| REST query parameters | Requires Mendix 11.0+ | Build query string manually on 10.x |
| Concurrent editing | Not supported — a file-based write is refused while Studio Pro has the project open (`<project>.mpr.lock` present) | Close Studio Pro before mxcli writes, or write through it with `--mcp`; `exec --force` overrides |
| Widget template drift | CE0463 on version mismatch | MPK augmentation handles most cases |
| Marketplace module update | `install` reports an existing module and does not update it | `mxcli marketplace diff` / `update <id> -p app.mpr --to <version>` (preserves entity identity and role grants; local edits are refused or saved with `--save-edits`) |
| 47 of 52 metamodel domains | Not yet implemented | REST, OData write, etc. pending |

## Version Compatibility

See [Version Compatibility](../appendixes/version-compatibility.md) for detailed per-version feature availability.
