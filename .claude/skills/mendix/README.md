# Mendix MDL Skills

Skills for writing Mendix Definition Language (MDL) code correctly.

## Quick Reference (Cheat Sheets)

Start here for quick syntax lookups:

| Skill | Purpose | Use When |
|-------|---------|----------|
| [cheatsheet-variables.md](cheatsheet-variables.md) | Variable declaration syntax | Declaring variables, fixing declaration errors |
| [cheatsheet-errors.md](cheatsheet-errors.md) | Common errors and fixes | Debugging MDL syntax errors |

## Syntax Reference (By Document Type)

Detailed syntax for each MDL document type:

| Skill | Purpose | Use When |
|-------|---------|----------|
| [mdl-entities.md](mdl-entities.md) | Entity, attribute, association syntax | Creating domain models |
| [write-microflows.md](write-microflows.md) | Microflow syntax reference | Writing microflow logic |
| [write-oql-queries.md](write-oql-queries.md) | OQL query syntax | Creating VIEW entities |
| [create-page.md](create-page.md) | Page and widget syntax | Creating pages |
| [fragments.md](fragments.md) | Fragment (reusable widget group) syntax | Reusing widget patterns across pages |

## Patterns (By Use Case)

Common implementation patterns:

| Skill | Purpose | Use When |
|-------|---------|----------|
| [patterns-crud.md](patterns-crud.md) | Create/Read/Update/Delete patterns | Building CRUD functionality |
| [patterns-data-processing.md](patterns-data-processing.md) | Loops, aggregates, batch processing | Processing lists of data |
| [validation-microflows.md](validation-microflows.md) | Validation feedback patterns | Building form validation |

## Integration Skills

External system integration:

| Skill | Purpose | Use When |
|-------|---------|----------|
| [database-connections.md](database-connections.md) | Mendix Database Connector | Connecting to Oracle, PostgreSQL, etc. via JDBC |
| [demo-data.md](demo-data.md) | Demo data & IMPORT | Seeding data, `IMPORT FROM` bulk import from external DB |
| [rest-client.md](rest-client.md) | REST API consumption | Calling external REST APIs via consumed REST client documents |
| [rest-call-from-json.md](rest-call-from-json.md) | REST CALL end-to-end | JSON Structure → Entities → Import Mapping → REST CALL microflow |
| [java-actions.md](java-actions.md) | Custom Java actions | Extending with Java code |

## Page Patterns

Page-specific patterns:

| Skill | Purpose | Use When |
|-------|---------|----------|
| [overview-pages.md](overview-pages.md) | List/grid pages | Building overview screens |
| [master-detail-pages.md](master-detail-pages.md) | Master-detail layouts | Building selection-based UIs |
| [bulk-widget-updates.md](bulk-widget-updates.md) | Bulk widget property updates | Changing widget settings across pages |

## Specialized Skills

| Skill | Purpose | Use When |
|-------|---------|----------|
| [generate-domain-model.md](generate-domain-model.md) | Complete domain model generation | Generating full domain models |
| [create-custom-widget.md](create-custom-widget.md) | Custom pluggable widget AIGC | Creating custom React widgets from scratch |
| [debug-bson.md](debug-bson.md) | BSON debugging | Troubleshooting SDK issues |

---

## Skill Loading Guide

### For LLMs (Claude Code)

Load skills based on the task:

| User Request | Load These Skills |
|--------------|-------------------|
| "Create entity/domain model" | `mdl-entities.md` |
| "Write microflow" | `write-microflows.md`, `cheatsheet-variables.md` |
| "Create validation" | `validation-microflows.md`, `patterns-crud.md` |
| "Add CRUD operations" | `patterns-crud.md` |
| "Process list of items" | `patterns-data-processing.md` |
| "Fix MDL error" | `cheatsheet-errors.md` |
| "Import data from database" | `demo-data.md` |
| "Call a REST API / integrate JSON endpoint" | `rest-call-from-json.md` |
| "Create JSON structure / import mapping" | `rest-call-from-json.md` |
| "Seed/populate test data" | `demo-data.md` |
| "Update widget properties" | `bulk-widget-updates.md` |
| "Change widgets in bulk" | `bulk-widget-updates.md` |
| "Reuse widgets across pages" | `fragments.md` |
| "Define a fragment" | `fragments.md` |
| "Create custom widget" | `create-custom-widget.md` |
| "Build a pluggable widget" | `create-custom-widget.md` |

### For Error Recovery

When encountering errors:

| Error Type | Load This Skill |
|------------|-----------------|
| Variable not declared | `cheatsheet-variables.md` |
| Entity type syntax | `cheatsheet-errors.md` |
| Association path error | `cheatsheet-errors.md` |
| Microflow structure error | `write-microflows.md` |
| OQL syntax error | `write-oql-queries.md` |

---

## Common Mistakes Summary

| Mistake | Frequency | Quick Fix |
|---------|-----------|-----------|
| SET without DECLARE | High | Add `DECLARE $var Type = value;` before SET |
| Missing AS for entity | High | Use `DECLARE $var AS Module.Entity;` |
| Unqualified association | Medium | Use `$var/Module.Assoc/Attr` |
| String enum comparison | Medium | Use `Module.Enum.Value` not `'string'` |
| Missing RETURN | Low | Add `RETURN $value;` at end |

---

## Integration Points

### REPL Integration
```bash
mdl> help variables    # Load cheatsheet-variables.md
mdl> help errors       # Load cheatsheet-errors.md
mdl> help crud         # Load patterns-crud.md
```

### Check Command
```bash
mxcli check script.mdl -p app.mpr --references
```

### Linter
```bash
mxcli lint script.mdl
```

---

## Maintenance

When updating skills:
1. Keep each skill focused (100-200 lines target)
2. Include working code examples
3. Document common mistakes with fixes
4. Update this README when adding new skills
5. Test examples with actual MDL execution
