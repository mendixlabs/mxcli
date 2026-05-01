# Catalog & Code Navigation Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/386)

## Setup

> See [AGENT-TESTING.md](./AGENT-TESTING.md) for build, execution methods, and verification patterns.

---

## 1. REFRESH CATALOG

### 1.1 Fast mode (default)

```
refresh catalog;
```

**Expected:** Builds core tables (modules, entities, attributes, microflows, etc.). Output shows per-table row count + "Catalog ready (Xs)".

### 1.2 Full mode

```
refresh catalog full;
```

**Expected:** Adds activities, widgets, refs, strings, permissions tables.

### 1.3 Full with source

```
refresh catalog full source;
```

**Expected:** Adds source table (MDL text for each document).

### 1.4 Force rebuild

```
refresh catalog force;
```

**Expected:** Ignores cached catalog, rebuilds from scratch.

### 1.5 Background build

```
refresh catalog background;
```

**Expected:** Returns immediately; catalog built in background goroutine.

### 1.6 Caching behavior

1. Run `refresh catalog full;`
2. Exit and re-open same project
3. Run `refresh catalog;`

**Expected:** Second run uses cached `.mxcli/catalog.db` (fast — no rebuild needed).

---

## 2. SHOW CATALOG TABLES

### 2.1 List tables

```
show catalog tables;
```

**Expected:** Table with columns `Table | Count`. Lists 35+ tables including: modules, entities, attributes, microflows, nanoflows, pages, snippets, layouts, enumerations, activities, widgets, xpath_expressions, refs, permissions, workflows, odata_clients, odata_services, etc.

### 2.2 Tables require appropriate build mode

Verify tables marked as full-only show 0 rows if only fast catalog was built.

---

## 3. SHOW CATALOG STATUS

### 3.1 Catalog status

```
show catalog status;
```

**Expected:** Cache path, build mode, build time, build duration, Mendix version, validity status.

---

## 4. DESCRIBE CATALOG TABLE

### 4.1 Describe entities table

```
describe catalog.entities;
```

**Expected:** Column names, types, PK markers + required refresh mode.

### 4.2 Describe activities table

```
describe catalog.activities;
```

**Expected:** Schema for full-mode table.

---

## 5. SELECT FROM CATALOG

### 5.1 Basic select

```
select * from catalog.entities where ModuleName = 'Administration';
```

**Expected:** Row count + table result with entity data from Administration module.

### 5.2 Select with multiple conditions

```
select QualifiedName, EntityType from catalog.entities where EntityType = 'PERSISTENT' and ModuleName = 'FactoryManagement';
```

**Expected:** Filtered results.

### 5.3 Aggregate query

```
select ModuleName, count(*) as cnt from catalog.entities group by ModuleName order by cnt desc;
```

**Expected:** Entity counts per module.

### 5.4 Join across tables

```
select e.QualifiedName, count(a.Name) as attrs
from catalog.entities e
join catalog.attributes a on e.QualifiedName = a.EntityQualifiedName
group by e.QualifiedName
having attrs > 10;
```

**Expected:** Entities with more than 10 attributes.

### 5.5 Full-only table without full build

```
refresh catalog;
select * from catalog.activities;
```

**Expected:** Warning about insufficient build mode. Empty or error.

### 5.6 FTS search on strings table

```
select * from catalog.strings where strings match 'factory';
```

**Expected:** Full-text search results with matching snippets. (Note: use a term present in your test project; `'factory'` works with Evora Factory Management.)

---

## 6. SEARCH

### 6.1 Basic search

```
search 'customer';
```

**Expected:** String matches with columns: `QualifiedName | ObjectType | Match | StringContext | ModuleName`. Match shows `>>>...<<<` highlighting.

### 6.2 Search with special characters

```
search 'user.name';
```

**Expected:** Special chars (`.`, `/`, `-`, `:`) treated as spaces (AND semantics).

### 6.3 Search requiring source mode

If source table exists, additional source matches appear in output.

### 6.4 Search with no results

```
search 'xyznonexistent123';
```

**Expected:** Empty result.

---

## 7. SHOW CALLERS

### 7.1 Direct callers

```
show callers of Administration.ACT_CreateAccount;
```

**Expected:** Table with columns `Caller | Depth`. Shows microflows/pages that call this microflow.

### 7.2 Transitive callers

```
show callers of Administration.ACT_CreateAccount transitive;
```

**Expected:** Recursive callers up to depth 10.

### 7.3 No callers

```
show callers of <Module.UnusedMicroflow>;
```

**Expected:** Empty result.

---

## 8. SHOW CALLEES

### 8.1 Direct callees

```
show callees of <Module.OrchestratorMicroflow>;
```

**Expected:** Microflows called by this microflow.

### 8.2 Transitive callees

```
show callees of <Module.Flow> transitive;
```

**Expected:** Recursive callees up to depth 10.

---

## 9. SHOW REFERENCES

### 9.1 Entity references

```
show references to Administration.Account;
```

**Expected:** Table with columns `SourceType | SourceName | RefKind`. Shows all documents referencing this entity.

### 9.2 Microflow references

```
show references to <Module.Microflow>;
```

**Expected:** Documents referencing this microflow.

---

## 10. SHOW IMPACT

### 10.1 Entity impact

```
show impact of Administration.Account;
```

**Expected:** Summary (count by type) + detailed table of impacted documents.

### 10.2 Microflow impact

```
show impact of <Module.Microflow>;
```

**Expected:** Summary + detail table.

---

## 11. SHOW CONTEXT

### 11.1 Microflow context

```
show context of Administration.ACT_CreateAccount;
```

**Expected:** Markdown output: definition (name, return, params, activities) + entities used + pages shown + called microflows + direct callers.

### 11.2 Entity context

```
show context of Administration.Account;
```

**Expected:** Definition (type, generalization, attributes, indexes) + microflows using it + pages displaying it + related entities.

### 11.3 Page context

```
show context of <Module.Page>;
```

**Expected:** Definition (title, URL, layout, widgets) + entities used + microflows called + shown by.

### 11.4 Context with depth

```
show context of Administration.ACT_CreateAccount depth 3;
```

**Expected:** Deeper recursive resolution of called microflows.

### 11.5 Non-existent document

```
show context of Fake.Missing;
```

**Expected:** Error or empty — document not found in catalog.

---

## 12. SHOW STRUCTURE

### 12.1 Default structure (depth 2)

```
show structure;
```

**Expected:** Module names + element type counts (entities, enums, microflows, etc.). Skips system/marketplace modules. Default depth is 2 (element names with signatures).

> **Note:** To get depth-1 output (counts only), use `show structure depth 1` explicitly.

### 12.2 Depth 2

```
show structure depth 2;
```

**Expected:** Element names with signatures — entity attributes, microflow params → return, enum values.

### 12.3 Depth 3

```
show structure depth 3;
```

**Expected:** Attribute types, parameter names, association delete behavior, constant defaults.

### 12.4 Module filter

```
show structure in Administration;
```

**Expected:** Only `Administration` module structure.

> **Note:** Marketplace and system modules are filtered by default. Use `show structure in <Module> all` to include them, or test with a non-marketplace module.

### 12.5 Include system modules

```
show structure all;
```

**Expected:** All modules including System, Atlas_Core, etc.

---

## 13. MULTI-STEP WORKFLOWS

### 13.1 Create entity → refresh → query catalog

```
create persistent entity MyModule.Indexed (
  Name: string(200) not null
);
refresh catalog;
select * from catalog.entities where QualifiedName = 'MyModule.Indexed';
```

**Expected:** New entity appears in catalog after refresh.

### 13.2 Impact analysis before drop

```
show impact of MyModule.Customer;
drop entity MyModule.Customer;
```

**Expected:** Impact shows affected documents before destructive operation.

---

## 14. FAILURE MODES & ERROR RECOVERY

### 14.1 Query without catalog

Open project, do NOT refresh catalog:
```
select * from catalog.entities;
```

**Expected:** mxcli auto-builds a fast catalog on demand and returns results. No manual `refresh catalog` required.

### 14.2 Invalid SQL

```
select * from catalog.nonexistent;
```

**Expected:** SQL error — no such table.

### 14.3 Search without full catalog

```
refresh catalog;
search 'test';
```

**Expected:** mxcli auto-upgrades to a full catalog build on demand and returns search results. No manual `refresh catalog full` required.

### 14.4 Callers without refs table

```
refresh catalog;
show callers of MyModule.Flow;
```

**Expected:** mxcli auto-upgrades to a full catalog build on demand and returns caller results. No manual `refresh catalog full` required.

---

## Test Project Coverage Matrix

| Operation | Lato Enquiry | Evora Factory | Lato Product |
|-----------|:---:|:---:|:---:|
| REFRESH CATALOG | x | x | x |
| SHOW CATALOG TABLES | x | | |
| SHOW CATALOG STATUS | x | | |
| DESCRIBE CATALOG | x | | |
| SELECT FROM CATALOG | x | x | x |
| SEARCH | x | x | |
| SHOW CALLERS | x | x | |
| SHOW CALLEES | x | x | |
| SHOW REFERENCES | x | x | |
| SHOW IMPACT | x | | |
| SHOW CONTEXT | x | x | |
| SHOW STRUCTURE | x | x | x |

---

## Automated Test Coverage

| Section | Automated | Manual-only |
|---------|:---------:|:-----------:|
| 1. REFRESH CATALOG | Unit tests | Caching, background |
| 2. SHOW TABLES | Unit tests | |
| 3. SHOW STATUS | | All manual |
| 4. DESCRIBE TABLE | | All manual |
| 5. SELECT | Unit tests | Complex queries |
| 6. SEARCH | Unit tests | Edge cases |
| 7. CALLERS | Unit tests | Transitive depth |
| 8. CALLEES | Unit tests | |
| 9. REFERENCES | Unit tests | |
| 10. IMPACT | Unit tests | |
| 11. CONTEXT | Unit tests | Deep depth |
| 12. STRUCTURE | Unit tests | |
| 13. Multi-step | | All manual |
| 14. Failure modes | Partial | |

---

## Manual Test Report Template

**Tester:** _______________
**Date:** _______________
**Project:** _______________

| # | Section | Test | Pass | Fail | Skip | Notes |
|---|---------|------|:----:|:----:|:----:|-------|
| 1.1 | REFRESH | Fast mode | | | | |
| 1.2 | REFRESH | Full mode | | | | |
| 1.3 | REFRESH | Full + source | | | | |
| 1.4 | REFRESH | Force | | | | |
| 1.5 | REFRESH | Background | | | | |
| 1.6 | REFRESH | Caching | | | | |
| 2.1 | TABLES | List tables | | | | |
| 2.2 | TABLES | Build mode check | | | | |
| 3.1 | STATUS | Show status | | | | |
| 4.1 | DESCRIBE | Entities table | | | | |
| 4.2 | DESCRIBE | Activities table | | | | |
| 5.1 | SELECT | Basic | | | | |
| 5.2 | SELECT | Multiple conditions | | | | |
| 5.3 | SELECT | Aggregate | | | | |
| 5.4 | SELECT | Join | | | | |
| 5.5 | SELECT | Full-only warning | | | | |
| 5.6 | SELECT | FTS search | | | | |
| 6.1 | SEARCH | Basic | | | | |
| 6.2 | SEARCH | Special chars | | | | |
| 6.3 | SEARCH | Source mode | | | | |
| 6.4 | SEARCH | No results | | | | |
| 7.1 | CALLERS | Direct | | | | |
| 7.2 | CALLERS | Transitive | | | | |
| 7.3 | CALLERS | None | | | | |
| 8.1 | CALLEES | Direct | | | | |
| 8.2 | CALLEES | Transitive | | | | |
| 9.1 | REFERENCES | Entity | | | | |
| 9.2 | REFERENCES | Microflow | | | | |
| 10.1 | IMPACT | Entity | | | | |
| 10.2 | IMPACT | Microflow | | | | |
| 11.1 | CONTEXT | Microflow | | | | |
| 11.2 | CONTEXT | Entity | | | | |
| 11.3 | CONTEXT | Page | | | | |
| 11.4 | CONTEXT | Depth 3 | | | | |
| 11.5 | CONTEXT | Not found | | | | |
| 12.1 | STRUCTURE | Depth 1 | | | | |
| 12.2 | STRUCTURE | Depth 2 | | | | |
| 12.3 | STRUCTURE | Depth 3 | | | | |
| 12.4 | STRUCTURE | Module filter | | | | |
| 12.5 | STRUCTURE | All modules | | | | |
| 13.1 | MULTI-STEP | Create + refresh + query | | | | |
| 13.2 | MULTI-STEP | Impact before drop | | | | |
| 14.1 | FAILURE | No catalog | | | | |
| 14.2 | FAILURE | Invalid SQL | | | | |
| 14.3 | FAILURE | Search no full | | | | |
| 14.4 | FAILURE | Callers no full | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
