# REFRESH CATALOG

The `REFRESH CATALOG` command builds or rebuilds the catalog index from the current project state. The catalog must be refreshed before running catalog queries or cross-reference navigation commands.

## Syntax

```sql
-- Basic refresh: builds metadata tables
REFRESH CATALOG;

-- Full refresh: builds metadata + cross-references + source
REFRESH CATALOG FULL;

-- Force rebuild: ignores cached catalog
REFRESH CATALOG FULL FORCE;
```

## Refresh Levels

### Basic Refresh

```sql
REFRESH CATALOG;
```

Builds the core metadata tables: entities, attributes, associations, microflows, pages, enumerations, and other element types. This is sufficient for structural queries like "list all entities in a module" or "find pages with a specific URL."

### Full Refresh

```sql
REFRESH CATALOG FULL;
```

Builds everything in the basic refresh plus:

- **Cross-reference data** -- Which elements reference which other elements
- **Source content** -- Microflow activity content, expression strings, widget properties
- **Call graph** -- Caller/callee relationships between elements

This level is required for:
- `LIST CALLERS OF`
- `LIST CALLEES OF`
- `LIST REFERENCES TO`
- `LIST IMPACT OF`
- `DESCRIBE CONTEXT OF`
- `SEARCH`
- Full-text catalog queries

### Force Rebuild

```sql
REFRESH CATALOG FULL FORCE;
```

Ignores the cached `.mxcli/catalog.db` file and rebuilds from scratch. Use this when the catalog appears stale or corrupt.

## CLI Usage

```bash
# Quick refresh
mxcli -p app.mpr -c "REFRESH CATALOG"

# Full refresh with cross-references
mxcli -p app.mpr -c "REFRESH CATALOG FULL"

# Force rebuild
mxcli -p app.mpr -c "REFRESH CATALOG FULL FORCE"
```

## Caching

The catalog is cached in `.mxcli/catalog.db` next to the MPR file. On subsequent refreshes, mxcli checks whether the project has changed and only rebuilds if necessary (unless `FORCE` is specified).

### Progress output

A build prints a line per table (`✓ Entities: 111`) only when its output is a terminal. Piped, logged or run by an agent, it prints nothing per table while it is quick, and the latest progress line every 15 seconds while it is slow (`… building catalog, source 250/300 documents (32s)`), so a build of several minutes still shows it is running. Lines that carry information, such as the incremental summary and documents that could not be described, print either way.

`-q` / `--quiet` works on every command: it drops the warning banner and the connection and status messages, and sends what catalog progress remains to stderr.

### Incremental rebuilds

A rebuild after a save does not start from scratch. The cache records which document every row came from, and the next rebuild of the same level (fast, `FULL` or `FULL SOURCE`) starts from the cached catalog and produces again only the rows of:

- the documents that changed, were added or were deleted (a document moved to another existing folder counts as changed);
- the documents whose references name one of them, before or after the change. A document's rows can depend on what it names: what a delete removes depends on the association it retrieved over. A specialization counts as naming what its generalization holds.

The rows of every other document stay as they are, and only the documents being rebuilt are read and decoded. On a 4,753-document project a refresh after one edit takes under a second, against several seconds for a full build. The progress output then shows one line, `incremental: N units rebuilt, M carried over`, instead of a count per table.

The rebuild is a full one, as before, when:

- something changes that no per-document pass reads: a folder, a module, the project settings, the security settings or the navigation;
- the cache was written by another mxcli build or at another level;
- `FORCE` is given.

An incremental rebuild produces exactly the rows a full one would. Only the internal row IDs (the `Id` column of `refs` and `permissions`) and the order of rows in an unsorted query can differ.

With the MDL source index, an entity in a document being rebuilt is described again only when its own definition changed, not when another entity of its module did. A change to an association or annotation describes every entity of that module again, and a change to a document that is never described (a Java action, a constant, an image collection) describes everything.

### The unit store (MPR v2)

An MPR v2 project keeps each document in its own file under `mprcontents/`, and a refresh reads all of them. mxcli keeps a compressed copy of those files in `.mxcli/cache/unit-contents.db` and reads a document from its own file only when the file has changed since mxcli last read it (its size or modification time differs, or the `.mpr` records another content hash). A refresh after a save therefore opens one or two files instead of thousands. That matters most on Windows, where a virus scanner inspects every file a process opens.

The store is a cache: deleting it costs one slower refresh, and a store that cannot be read or written is ignored. It is about half the size of `mprcontents/`, and its directory has its own `.gitignore`, so it is never committed even where `.mxcli/` is not ignored.

## When to Refresh

- **After opening a project** -- Run `REFRESH CATALOG` to enable queries
- **Before cross-reference navigation** -- Run `REFRESH CATALOG FULL` to enable CALLERS/CALLEES/IMPACT
- **After making changes** -- Refresh to update the catalog with new elements
- **If results seem stale** -- Use `REFRESH CATALOG FULL FORCE` to force a rebuild

> **FULL mode populates the analytic tables.** Plain `REFRESH CATALOG` (fast mode)
> leaves `CATALOG.ACTIVITIES`, `CATALOG.REFS`, `CATALOG.XPATH_EXPRESSIONS`, and
> `CATALOG.WIDGETS` **empty** — a fast-mode query against them warns *"requires
> refresh catalog full"*. Run `REFRESH CATALOG FULL` to fill them (activity rows
> carry the model GUIDs, cross-reference edges, and XPath expressions).
