# DESCRIBE STRUCTURE

## Synopsis

    DESCRIBE STRUCTURE [DEPTH 1|2|3] [IN <module>] [ALL]

## Description

Shows a hierarchical overview of the project structure. The output depth and scope can be controlled with optional clauses. By default, displays depth 2 (documents with signatures) for user modules only, excluding system and marketplace modules.

This is useful for getting a quick birds-eye view of what a project contains without examining each element individually.

## Parameters

*DEPTH*
: Controls the level of detail in the output. `1` shows one line per module with element counts. `2` (default) shows documents with their signatures. `3` shows full detail including typed attributes and named parameters. At depth 2 and 3 a module's documents are nested under the folders they sit in (`Orders/`, then `Actions/` inside it), after the documents at the module root; entities stay at the root, as the domain model has no folders.

*module*
: When `IN <module>` is specified, restricts output to a single module. Implicitly uses depth 2 unless overridden.

*ALL*
: Include system and marketplace modules in the output. Without this flag, only user-created modules are shown.

## Examples

Show module-level summary (one line per module):

```sql
DESCRIBE STRUCTURE DEPTH 1;
```

Show default structure (documents with signatures, user modules only):

```sql
DESCRIBE STRUCTURE;
```

Describe structure of a single module:

```sql
DESCRIBE STRUCTURE IN MyFirstModule;
```

Show full detail including all modules:

```sql
DESCRIBE STRUCTURE DEPTH 3 ALL;
```

## See Also

[LIST MODULES](list-modules.md), [LIST ENTITIES](list-entities.md), [LIST MICROFLOWS](list-microflows.md), [LIST PAGES](list-pages.md)
