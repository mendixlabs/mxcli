# Full-Text Search

The `SEARCH` command performs full-text search across all strings and source code in a Mendix project. It searches captions, messages, the documentation of project elements (entities, attributes, constants, pages and more), and MDL source.

## Syntax

```sql
SEARCH '<keyword>';
```

## Examples

```sql
-- Search for a keyword
SEARCH 'validation';

-- Search for an entity name
SEARCH 'Customer';

-- Search for error messages
SEARCH 'cannot be empty';

-- Search for specific patterns
SEARCH 'CurrentDateTime';
```

## CLI Usage

```bash
# Basic search
mxcli search -p app.mpr "validation"

# Quiet mode (no headers/formatting), suitable for piping
mxcli search -p app.mpr "validation" -q

# Output as names (type<TAB>name per line)
mxcli search -p app.mpr "validation" -q --format names

# Output as JSON array
mxcli search -p app.mpr "validation" --json

# Exit 1 when anything matches (0 when nothing does)
mxcli search -p app.mpr "validation" --fail-on-match

# Leave out Marketplace modules
mxcli search -p app.mpr "validation" --exclude-marketplace
```

## Output Formats

| Flag | Description |
|------|-------------|
| `--format table` (default) | Human-readable tables of string and source matches |
| `--format names` | `type<TAB>name` per line, suitable for piping |
| `--json` | JSON array of search results |

`--json` is the same flag every query command takes (`refs`, `callers`,
`callees`, `impact`, `context`, `show`, `describe`, `-c "select …"`). With it,
stdout carries the JSON document and nothing else — connection and catalog
progress go to stderr, an empty result is `[]`, and an error is a message on
stderr with a non-zero exit. `--format json` is still accepted as a deprecated
spelling of `--json`.

The `-q` (quiet) flag suppresses headers and formatting, making output suitable for piping to other commands.

## Piping Search Results

Search results can be piped to other mxcli commands for powerful workflows:

```bash
# Find the first microflow matching "error" and describe it
mxcli search -p app.mpr "error" -q --format names | head -1 | awk '{print $2}' | \
  xargs mxcli describe -p app.mpr microflow
```

## What Gets Searched

`SEARCH` looks in two indexes, and says which one a hit came from.

**Strings** (a full catalog, built automatically):

- Every translatable text: page titles, widget captions and labels, enumeration value captions, button and menu captions, show-message and validation-feedback texts, in every language
- Log messages and log node names of microflows
- Page URLs and the paths, resources and operation summaries of published REST services
- Workflow names, descriptions, task names and activity captions
- Element names: the qualified name is a searchable column too, so `SEARCH 'Customer'` also returns the rows of elements named Customer that have some indexed text
- **Documentation of project elements.** A hit's `ObjectType` names the element and its `StringContext` is `documentation`:

  | Documented element | `ObjectType` |
  |--------------------|--------------|
  | Entity | `ENTITY` |
  | Attribute (qualified `Module.Entity.Attribute`) | `ATTRIBUTE` |
  | Association | `ASSOCIATION` |
  | Enumeration | `ENUMERATION` |
  | Constant | `CONSTANT` |
  | Page, snippet, layout, building block, page template | `PAGE`, `SNIPPET`, `LAYOUT`, `BUILDING_BLOCK`, `PAGE_TEMPLATE` |
  | Microflow, nanoflow, rule | `MICROFLOW`, `NANOFLOW`, `RULE` |
  | Workflow | `WORKFLOW` |
  | Java action, JavaScript action | `JAVA_ACTION`, `JAVASCRIPT_ACTION` |
  | Microflow parameter, Java action parameter (qualified `Module.Flow.Parameter`) | `MICROFLOW_PARAMETER`, `JAVA_ACTION_PARAMETER` |
  | Consumed and published REST service, business event service | `CONSUMED_REST_SERVICE`, `PUBLISHED_REST_SERVICE`, `BUSINESS_EVENT_SERVICE` |
  | Import mapping, export mapping, JSON structure, data transformer | `IMPORT_MAPPING`, `EXPORT_MAPPING`, `JSON_STRUCTURE`, `DATA_TRANSFORMER` |
  | Scheduled event, task queue, regular expression, image and icon collection, menu | `SCHEDULED_EVENT`, `TASK_QUEUE`, `REGULAR_EXPRESSION`, `IMAGE_COLLECTION`, `ICON_COLLECTION`, `MENU` |
  | Agent, AI model, knowledge base, consumed MCP service | `AGENT`, `AI_MODEL`, `KNOWLEDGE_BASE`, `CONSUMED_MCP_SERVICE` |
  | A module's domain model documentation | `MODULE`, with `StringContext` `domain_model_documentation` |

  This is the complete set: every document type whose catalog table carries a documentation text. Not covered: the documentation of an enumeration value (Mendix has no such field) and the summary text of a remote service's contract entities (written by the remote service, not by the project).

**MDL source** (only after `refresh catalog full source`): microflow, nanoflow and page bodies, expressions and the other definitions as MDL text. Until it is built, a warning on stderr says only strings were searched.

## Failing on a match

`--fail-on-match` turns `search` into a gate. The results are printed as usual (a JSON array under `--json`), and the exit status carries the answer:

| Exit status | Meaning |
|-------------|---------|
| `0` | No hit |
| `1` | At least one hit |
| `2` | The search could not run: an unknown project, or a query the index rejects (an unbalanced `"`, say) |

```bash
# Refuse the change if a placeholder is left anywhere in the project's text
mxcli search -p app.mpr "TODO" --fail-on-match --json
```

Without the flag a hit exits `0`, as before.

## Marketplace modules

By default every module is searched, Marketplace modules included, so existing results do not change. `--exclude-marketplace` leaves out hits inside modules downloaded from the Marketplace, which is what an audit of the app's own text usually wants: the vendor's documentation is not yours to edit. A Marketplace module's own documentation and domain model documentation are left out with the rest of it.

## Comparison with Catalog Queries

`SEARCH` provides quick keyword-based search, while catalog queries (`SELECT FROM CATALOG.*`) provide structured SQL-based queries with filtering, joining, and aggregation. Use `SEARCH` for quick discovery and catalog queries for precise analysis.

```sql
-- Quick search: find anything mentioning "Customer"
SEARCH 'Customer';

-- Precise query: find entities named Customer with their module
SELECT Name, ModuleName FROM CATALOG.ENTITIES WHERE Name LIKE '%Customer%';
```
