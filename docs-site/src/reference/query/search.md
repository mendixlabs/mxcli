# SEARCH

## Synopsis

    SEARCH '<keyword>'

## Description

Performs a full-text search across all strings, messages, captions, the documentation of project elements (entities, attributes, constants, pages and more), and MDL source in the project. Uses FTS5 (SQLite full-text search) for fast matching. Returns results grouped by document type and location.

This command requires the catalog to be populated. If search returns no results, run `REFRESH CATALOG FULL` first. The search is also available as a CLI subcommand: `mxcli search -p app.mpr "keyword" --format names|json`. The CLI adds `--exclude-marketplace`; see [Full-Text Search](../../tools/search.md).

## Parameters

*keyword*
: The search term to look for. Must be enclosed in single quotes. Matches against entity names, attribute names, microflow activity captions, page titles, widget labels, string constants, log messages, element documentation, and MDL source text.

## Examples

Search for a keyword:

```sql
SEARCH 'Customer';
```

Search for an error message:

```sql
SEARCH 'is required';
```

Search using the CLI:

```sql
-- From the command line:
-- mxcli search -p app.mpr "Customer" --format names
-- mxcli search -p app.mpr "Customer" --json
```

## See Also

[DESCRIBE STRUCTURE](describe-structure.md), [LIST ENTITIES](list-entities.md), [LIST MICROFLOWS](list-microflows.md)
