# Mapping & Data Transformer Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/386)

## Setup

> See [AGENT-TESTING.md](./AGENT-TESTING.md) for build, execution methods, and verification patterns.

**Note:** All commands in this document are **read-only** (SHOW and DESCRIBE). No fixture copy needed.

---

## 1. SHOW IMPORT MAPPINGS

### 1.1 List all import mappings
```
show import mappings;
```
**Expected:** Table with columns: Import Mapping, Name, Schema Source, Elements. Count matches Studio Pro.

### 1.2 Filter by module
```
show import mappings in MyModule;
```
**Expected:** Only import mappings from `MyModule`.

### 1.3 Empty module
```
show import mappings in ModuleWithNoMappings;
```
**Expected:** Empty result, no error.

### 1.4 Non-existent module
```
show import mappings in NonExistentModule;
```
**Expected:** Error message.

### 1.5 Column accuracy
Pick 5+ import mappings. Verify Schema Source and Elements columns match Studio Pro.

---

## 2. DESCRIBE IMPORT MAPPING

### 2.1 Simple import mapping
```
describe import mapping Module.SimpleMapping;
```
**Expected:** Valid MDL output in the form:
```
create import mapping Module.SimpleMapping with json structure Module.Schema {
  create Module.Entity {
    Attr = jsonField key,
  }
};
```

### 2.2 Element rendering — object root
Verify the root element uses handling keyword and entity reference:
```
create Module.Entity {
  ...
}
```
Valid handling keywords: `create` (default), `find`, `find or create`.

### 2.3 Element rendering — nested object
Verify nested elements use association path and `jsonKey`:
```
create Assoc/Entity = jsonKey {
  ...
}
```

### 2.4 Element rendering — value mapping
Verify leaf values render as:
```
Attr = jsonField key,
```
The `key` suffix marks the key attribute.

### 2.5 Handling keyword — find
```
describe import mapping Module.FindMapping;
```
**Expected:** Root element uses `find Module.Entity { ... }`.

### 2.6 Handling keyword — find or create
```
describe import mapping Module.FindOrCreateMapping;
```
**Expected:** Root element uses `find or create Module.Entity { ... }`.

### 2.7 Handling keyword — create (default)
```
describe import mapping Module.CreateMapping;
```
**Expected:** Root element uses `create Module.Entity { ... }`.

### 2.8 Multiple root elements
Test import mapping with multiple top-level mapped entities. Verify all rendered.

### 2.9 Deeply nested structure (3+ levels)
Verify correct indentation and nesting for mappings with 3+ levels of nested objects.

### 2.10 Non-existent import mapping
```
describe import mapping Module.DoesNotExist;
```
**Expected:** Clear error message.

---

## 3. SHOW EXPORT MAPPINGS

### 3.1 List all export mappings
```
show export mappings;
```
**Expected:** Table with columns: Export Mapping, Name, Schema Source, Elements. Count matches Studio Pro.

### 3.2 Filter by module
```
show export mappings in MyModule;
```
**Expected:** Only export mappings from `MyModule`.

### 3.3 Empty module
```
show export mappings in ModuleWithNoMappings;
```
**Expected:** Empty result, no error.

### 3.4 Non-existent module
```
show export mappings in NonExistentModule;
```
**Expected:** Error message.

### 3.5 Column accuracy
Pick 5+ export mappings. Verify Schema Source and Elements columns match Studio Pro.

---

## 4. DESCRIBE EXPORT MAPPING

### 4.1 Simple export mapping
```
describe export mapping Module.SimpleExport;
```
**Expected:** Valid MDL output in the form:
```
create export mapping Module.SimpleExport with json structure Module.Schema null values <option> {
  Entity {
    jsonField = Attr,
    Assoc/Entity as jsonKey {
      ...
    }
  }
};
```

### 4.2 Assignment direction
Verify export mappings use reversed assignment compared to import: `jsonField = Attr` (not `Attr = jsonField`).

### 4.3 Nested element — `as` keyword
Verify nested elements use the `as` keyword:
```
Assoc/Entity as jsonKey {
  ...
}
```

### 4.4 No handling keyword
Verify export mapping elements omit handling keywords (`create`, `find`, `find or create`). These apply only to import mappings.

### 4.5 NullValueOption — default (LeaveOutElement)
```
describe export mapping Module.DefaultNullExport;
```
**Expected:** `null values` clause omitted when option is `LeaveOutElement` (the default).

### 4.6 NullValueOption — non-default
```
describe export mapping Module.NullAsDefaultExport;
```
**Expected:** `null values` clause present, e.g. `null values send default`.

### 4.7 Multiple root entities
Test export mapping with multiple top-level entities. Verify all rendered.

### 4.8 Deeply nested structure (3+ levels)
Verify correct indentation and nesting.

### 4.9 Non-existent export mapping
```
describe export mapping Module.DoesNotExist;
```
**Expected:** Clear error message.

---

## 5. SHOW DATA TRANSFORMERS

### 5.1 Filter by module
```
show data transformers in MyModule;
```
**Expected:** Only data transformers from `MyModule`.

### 5.2 Empty module
```
show data transformers in ModuleWithNoTransformers;
```
**Expected:** Empty result, no error.

### 5.3 Non-existent module
```
show data transformers in NonExistentModule;
```
**Expected:** Error message.

### 5.4 Feature gate — Mendix 10.x project
```
mxcli repl -p <evora-10.24.15.mpr>
show data transformers;
```
**Expected:** Error indicating data transformers require Mendix 11.9+ (feature gate: `integration/data_transformer`).

---

## 6. DESCRIBE DATA TRANSFORMER

### 6.1 Non-existent data transformer
```
describe data transformer Module.DoesNotExist;
```
**Expected:** Clear error message.

### 6.2 Feature gate — Mendix 10.x project
```
mxcli repl -p <evora-10.24.15.mpr>
describe data transformer Module.SomeTransformer;
```
**Expected:** Error indicating data transformers require Mendix 11.9+ (feature gate: `integration/data_transformer`).

---

## 7. FAILURE MODES

### 7.1 Not connected
Run any SHOW or DESCRIBE command without opening a project.
```
show import mappings;
```
**Expected:** Error about not being connected to a project.

### 7.2 Import mapping not found
```
describe import mapping Module.DoesNotExist;
```
**Expected:** Clear error: mapping not found.

### 7.3 Export mapping not found
```
describe export mapping Module.DoesNotExist;
```
**Expected:** Clear error: mapping not found.

### 7.4 Data transformer not found
```
describe data transformer Module.DoesNotExist;
```
**Expected:** Clear error: data transformer not found.

### 7.5 Feature gate violation — data transformer on <11.9
Run SHOW DATA TRANSFORMERS and DESCRIBE DATA TRANSFORMER on Evora (10.24.15).
**Expected:** Both commands produce a clear error referencing the `integration/data_transformer` feature gate and Mendix 11.9+ requirement.

### 7.6 Error message quality
For each error scenario, verify the message includes:
- **What** went wrong
- **Which** entity (qualified name)
- **Actionable guidance** where applicable

Scenarios: not-found (DESCRIBE import/export/transformer), not-connected (all commands), feature gate violation (data transformer).

---

## Test Project Coverage Matrix

| Category | Enquiries (11.4.0) | Evora Factory (10.24.15) | Lato Inventory (11.2.0) |
|---|---|---|---|
| SHOW IMPORT MAPPINGS count | Verify | Verify | Verify |
| SHOW EXPORT MAPPINGS count | Verify | Verify | Verify |
| SHOW DATA TRANSFORMERS | Verify (if 11.9+) | Feature gate error | Verify (if 11.9+) |
| DESCRIBE IMPORT MAPPING (sample 5+) | Diverse schemas | Diverse schemas | Diverse schemas |
| DESCRIBE EXPORT MAPPING (sample 5+) | Diverse schemas | Diverse schemas | Diverse schemas |
| DESCRIBE DATA TRANSFORMER (sample) | If available | Feature gate error | If available |
| Filter by module | All 3 types | All 3 types | All 3 types |
| Nested structures (3+ levels) | Import + Export | Import + Export | Import + Export |
| Handling keywords (import) | create/find/find or create | Same | Same |
| NullValueOption (export) | Default + non-default | Same | Same |
| Dollar-quoting (transformer) | Multi-line expressions | N/A (10.x) | Multi-line expressions |
| Error scenarios | Not-found for each type | Feature gate + not-found | Not-found for each type |

---

## Automated Test Coverage

| Area | Tests | Status |
|---|---|---|
| SHOW IMPORT MAPPINGS | None | **Gap** |
| SHOW IMPORT MAPPINGS IN module | None | **Gap** |
| DESCRIBE IMPORT MAPPING | None | **Gap** |
| SHOW EXPORT MAPPINGS | None | **Gap** |
| SHOW EXPORT MAPPINGS IN module | None | **Gap** |
| DESCRIBE EXPORT MAPPING | None | **Gap** |
| SHOW DATA TRANSFORMERS | None | **Gap** |
| SHOW DATA TRANSFORMERS IN module | None | **Gap** |
| DESCRIBE DATA TRANSFORMER | None | **Gap** |
| Feature gate (data transformer) | None | **Gap** |
| Error handling (not-found) | None | **Gap** |
| Error handling (not-connected) | None | **Gap** |

Manual testing priority:
1. SHOW counts for import/export mappings across all 3 projects
2. DESCRIBE accuracy against Studio Pro for import and export mappings
3. Handling keywords (create/find/find or create) on import mappings
4. NullValueOption rendering on export mappings
5. Data transformer feature gate enforcement on Evora (10.x)
6. Dollar-quoting for multi-line transformer expressions

---

## Manual Test Report Template

Copy and fill in after running manual tests.

```markdown
## Manual Testing

**Date:** YYYY-MM-DD
**Build:** `make build && make test && make lint-go` — PASS

### Test Projects

| App | Studio Pro | Import Mappings | Export Mappings | Data Transformers |
|-----|-----------|-----------------|-----------------|-------------------|
| Lato Enquiry Management | 11.4.0 | ✅ _n_ | ✅ _n_ | ✅ _n_ / N/A |
| Evora Factory Management | 10.24.15 | ✅ _n_ | ✅ _n_ | ✅ Feature gate |
| Lato Product Inventory | 11.2.0 | ✅ _n_ | ✅ _n_ | ✅ _n_ / N/A |

### Command Coverage

| Command | Tested | Notes |
|---------|--------|-------|
| SHOW IMPORT MAPPINGS | ✅/❌ | |
| SHOW IMPORT MAPPINGS IN module | ✅/❌ | |
| DESCRIBE IMPORT MAPPING | ✅/❌ | |
| SHOW EXPORT MAPPINGS | ✅/❌ | |
| SHOW EXPORT MAPPINGS IN module | ✅/❌ | |
| DESCRIBE EXPORT MAPPING | ✅/❌ | |
| SHOW DATA TRANSFORMERS | ✅/❌ | |
| SHOW DATA TRANSFORMERS IN module | ✅/❌ | |
| DESCRIBE DATA TRANSFORMER | ✅/❌ | |

### Import Mapping DESCRIBE Accuracy

| Mapping | Schema Source | Handling Keyword | Nested Levels | Match Studio Pro |
|---------|-------------|-----------------|---------------|-----------------|
| Module.Name | JSON/XML | create/find/find or create | _n_ | ✅/❌ |
| ... | | | | |

### Export Mapping DESCRIBE Accuracy

| Mapping | Schema Source | NullValueOption | Nested Levels | Match Studio Pro |
|---------|-------------|----------------|---------------|-----------------|
| Module.Name | JSON/XML | default/non-default | _n_ | ✅/❌ |
| ... | | | | |

### Data Transformer DESCRIBE Accuracy

| Transformer | Source Type | Steps | Dollar-Quoting | Match Studio Pro |
|-------------|-----------|-------|----------------|-----------------|
| Module.Name | JSON/OQL/... | _n_ | Yes/No/N/A | ✅/❌ |
| ... | | | | |

### Failure Modes (§7)

| Scenario | Result | Notes |
|----------|--------|-------|
| 7.1 Not connected | ✅/❌ | |
| 7.2 Import mapping not found | ✅/❌ | |
| 7.3 Export mapping not found | ✅/❌ | |
| 7.4 Data transformer not found | ✅/❌ | |
| 7.5 Feature gate (10.x) | ✅/❌ | |
| 7.6 Error message quality | ✅/❌ | |

### Issues Found

1. (none / describe issues here)
```
