# Tooling Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/386)

## Setup

> See [AGENT-TESTING.md](./AGENT-TESTING.md) for build, execution methods, and verification patterns.

---

## 1. RENAME

### 1.1 Rename entity — dry run

```
RENAME ENTITY MyModule.OldEntity TO NewEntity DRY RUN;
```

**Expected:** Output starts with `Would rename`. Includes reference count. No changes written to `.mpr`.

### 1.2 Rename entity — live

```
RENAME ENTITY MyModule.OldEntity TO NewEntity;
```

**Expected:** Output: `Renamed MyModule.OldEntity to MyModule.NewEntity`. Second line: `Updated N reference(s)`.

### 1.3 Rename microflow

```
RENAME MICROFLOW MyModule.ACT_OldName TO ACT_NewName;
```

**Expected:** `Renamed` + `Updated N reference(s)`. Verify with `show microflows in MyModule`.

### 1.4 Rename nanoflow

```
RENAME NANOFLOW MyModule.NAF_OldName TO NAF_NewName;
```

**Expected:** `Renamed` + `Updated N reference(s)`.

### 1.5 Rename page

```
RENAME PAGE MyModule.OldPage TO NewPage;
```

**Expected:** `Renamed` + `Updated N reference(s)`.

### 1.6 Rename enumeration

```
RENAME ENUMERATION MyModule.OldEnum TO NewEnum;
```

**Expected:** `Renamed` + `Updated N reference(s)`. Attributes using this enumeration updated.

### 1.7 Rename association

```
RENAME ASSOCIATION MyModule.Old_Assoc TO New_Assoc;
```

**Expected:** `Renamed` + `Updated N reference(s)`.

### 1.8 Rename constant

```
RENAME CONSTANT MyModule.OLD_CONST TO NEW_CONST;
```

**Expected:** `Renamed` + `Updated N reference(s)`.

### 1.9 Rename module

```
RENAME MODULE OldModule TO NewModule;
```

**Expected:** `Renamed` + reference count. All qualified names with `OldModule.` prefix updated to `NewModule.`.

### 1.10 Rename module — prefix scan

```
RENAME MODULE Administration TO Admin DRY RUN;
```

**Expected:** `Would rename`. Reference count includes all documents, entities, microflows, pages, etc. that use `Administration.` prefix.

### 1.11 Rename non-existent entity

```
RENAME ENTITY MyModule.DoesNotExist TO Something;
```

**Expected:** Error — entity not found.

### 1.12 Rename unsupported type

```
RENAME SNIPPET MyModule.Old TO New;
```

**Expected:** Error — unsupported type for rename.

### 1.13 Dry run produces no side effects

1. Run `describe entity MyModule.Target`
2. Run `RENAME ENTITY MyModule.Target TO Temp DRY RUN`
3. Run `describe entity MyModule.Target`

**Expected:** Output from step 1 and step 3 identical. Entity name unchanged.

---

## 2. LINT

### 2.1 Lint all modules

```
LINT;
```

**Expected:** Table of findings with columns: `Rule | Severity | Document | Message`. Summary: `N finding(s)`.

### 2.2 Lint single module

```
LINT MyModule;
```

**Expected:** Only findings from `MyModule`.

### 2.3 Lint — JSON format

```
LINT FORMAT json;
```

**Expected:** JSON array of finding objects with fields: `rule`, `severity`, `document`, `message`, `location`.

### 2.4 Lint — SARIF format

```
LINT FORMAT sarif;
```

**Expected:** Valid SARIF v2.1.0 JSON. Contains `runs[0].results` with finding entries.

### 2.5 Show lint rules

```
SHOW LINT RULES;
```

**Expected:** Table listing all built-in and custom rules with columns: `Rule | Severity | Description | Source`.

### 2.6 Built-in rule — NamingConvention

Create an entity or microflow with non-standard naming.

**Expected:** `LINT` reports `NamingConvention` violation.

### 2.7 Built-in rule — EmptyMicroflow

Create a microflow with only start and end nodes.

**Expected:** `LINT` reports `EmptyMicroflow` finding.

### 2.8 Built-in rule — DomainModelSize

Use a module with many entities.

**Expected:** If threshold exceeded, `LINT` reports `DomainModelSize` warning.

### 2.9 Built-in rule — ValidationFeedback

Create an entity attribute with `not null` but no validation message.

**Expected:** `LINT` reports `ValidationFeedback` finding.

### 2.10 Built-in rule — ImageSource

Use an image widget referencing a missing or oversized image.

**Expected:** `LINT` reports `ImageSource` finding.

### 2.11 Built-in rule — MissingTranslations

Use a project with multiple languages and missing translation strings.

**Expected:** `LINT` reports `MissingTranslations` finding.

### 2.12 Custom Starlark rule

Place a `.star` rule file in `.claude/lint-rules/`. Run `LINT`.

**Expected:** Custom rule appears in `SHOW LINT RULES`. Findings from custom rule included in output.

### 2.13 Config — excludeModules

Create `.mdllint.yaml` with:

```yaml
excludeModules:
  - Administration
  - System
```

Run `LINT`.

**Expected:** No findings from `Administration` or `System` modules.

### 2.14 Lint — clean project

Run on a project with no violations.

**Expected:** `0 finding(s)`.

---

## 3. ELK DOMAIN MODEL

> **Note:** ELK output is available via CLI only: `mxcli describe -p app.mpr --format elk entity Module.Entity`.
> There is no REPL-level `ELK DOMAIN MODEL` command.

### 3.1 Full module domain model (focused on entity)

```bash
mxcli describe -p app.mpr --format elk entity MyModule.Customer
```

**Expected:** JSON output with top-level keys: `entities`, `associations`, `generalizations`, `sourceMap`. Contains `MyModule.Customer` and its directly connected entities. Each entity has `id`, `name`, `category`, `attributes`.

### 3.2 Entity categories

Verify `category` field across entity types:

| Category | Condition |
|----------|-----------|
| `persistent` | Normal persistent entity |
| `nonpersistent` | Non-persistent entity |
| `external` | External/OData entity |
| `view` | View entity |

### 3.3 System overview (module-level)

```bash
mxcli describe -p app.mpr --format elk systemoverview MyModule
```

**Expected:** JSON with `modules` array, each containing `id`, `name`, `entityCount`, `microflowCount`, `pageCount`.

### 3.4 Focused view — view entity

```bash
mxcli describe -p app.mpr --format elk entity MyModule.ViewEntity
```

**Expected:** Delegates to OQL plan for view entities. Output includes source entities from OQL query.

### 3.5 Associations in output

**Expected:** Each association object contains `id`, `name`, `parent`, `child`, `type` (`Reference` / `ReferenceSet`).

### 3.6 Generalizations in output

**Expected:** Each generalization contains `child` and `parent` entity references.

### 3.7 Source map

**Expected:** `sourceMap` maps entity/association IDs to qualified names for client-side lookups.

### 3.8 Empty module

```bash
mxcli describe -p app.mpr --format elk entity EmptyModule.SomeEntity
```

**Expected:** JSON with empty `entities`, `associations`, `generalizations` arrays (or error if no entity exists).

### 3.9 Non-existent entity

```bash
mxcli describe -p app.mpr --format elk entity FakeModule.Fake
```

**Expected:** Error — entity/module not found.

---

## 4. ELK MICROFLOW

> **Note:** ELK microflow output is available via CLI only: `mxcli describe -p app.mpr --format elk microflow Module.MF`.

### 4.1 Basic microflow

```bash
mxcli describe -p app.mpr --format elk microflow MyModule.ACT_Simple
```

**Expected:** JSON with `nodes`, `edges`, `sourceMap`. Nodes include at least `start` and `end` types.

### 4.2 Node types

Verify node `type` values across different microflows:

| Type | Meaning |
|------|---------|
| `start` | Start event |
| `end` | End event |
| `continue` | Continue event |
| `break` | Break event |
| `error` | Error handler |
| `split` | Decision / exclusive split |
| `merge` | Merge point |
| `loop` | Loop activity (compound) |
| `action` | Activity (action call, object operation, etc.) |

### 4.3 Edge structure

**Expected:** Each edge has `source`, `target`, optional `label` (for split conditions).

### 4.4 Loop node — compound

```bash
mxcli describe -p app.mpr --format elk microflow MyModule.ACT_WithLoop
```

**Expected:** Loop node contains `children` (inner nodes) and `innerEdges`. Loop body rendered as sub-graph.

### 4.5 Source map

**Expected:** `sourceMap` maps node IDs to action names/descriptions.

### 4.6 Non-existent microflow

```bash
mxcli describe -p app.mpr --format elk microflow MyModule.Fake
```

**Expected:** Error — microflow not found.

---

## 5. MERMAID

> **Note:** Mermaid output is available via CLI only: `mxcli describe -p app.mpr --format mermaid <type> <name>`.

### 5.1 Entity / domain model — erDiagram

```bash
mxcli describe -p app.mpr --format mermaid entity MyModule.Customer
```

**Expected:** Mermaid `erDiagram` syntax. Entities as blocks, associations as relationships. Metadata comments: `%% @colors`, `%% @nodeinfo`.

### 5.2 Microflow — flowchart LR

```bash
mxcli describe -p app.mpr --format mermaid microflow MyModule.ACT_Process
```

**Expected:** Mermaid `flowchart LR` syntax. Start/end as rounded nodes, decisions as diamonds, actions as rectangles. Metadata comments present.

### 5.3 Nanoflow — flowchart LR

```bash
mxcli describe -p app.mpr --format mermaid nanoflow MyModule.NAF_Validate
```

**Expected:** Same format as microflow. `flowchart LR` with nanoflow-specific nodes.

### 5.4 Page — block-beta

```bash
mxcli describe -p app.mpr --format mermaid page MyModule.HomePage
```

**Expected:** Mermaid `block-beta` syntax representing page layout. Widgets as blocks.

### 5.5 Metadata comments — @colors

**Expected:** Output includes `%% @colors` comment with color assignments for entity categories or node types.

### 5.6 Metadata comments — @nodeinfo

**Expected:** Output includes `%% @nodeinfo` comment with node metadata (IDs, types).

### 5.7 Non-existent document

```bash
mxcli describe -p app.mpr --format mermaid microflow MyModule.Fake
```

**Expected:** Error — document not found.

---

## 6. CONTRACT

### 6.1 Show contract entities

```
SHOW CONTRACT ENTITIES FROM MyModule.MyService;
```

**Expected:** Table of published/consumed entities in the service contract.

### 6.2 Show contract actions

```
SHOW CONTRACT ACTIONS FROM MyModule.MyService;
```

**Expected:** Table of published microflow actions.

### 6.3 Show contract channels

```
SHOW CONTRACT CHANNELS FROM MyModule.MyService;
```

**Expected:** Table of message channels.

### 6.4 Show contract messages

```
SHOW CONTRACT MESSAGES FROM MyModule.MyService;
```

**Expected:** Table of message definitions.

### 6.5 Describe contract entity

```
DESCRIBE CONTRACT ENTITY MyModule.MyService.Customer;
```

**Expected:** Detailed entity description: attributes, types, key fields.

### 6.6 Describe contract entity — MDL format

```
DESCRIBE CONTRACT ENTITY MyModule.MyService.Customer FORMAT mdl;
```

**Expected:** MDL-syntax output for the contract entity.

### 6.7 Create external entities — all

```
CREATE EXTERNAL ENTITIES FROM MyModule.MyService;
```

**Expected:** External entities created in the service's module. `show entities` lists them as `External` type.

### 6.8 Create external entities — into specific module

```
CREATE EXTERNAL ENTITIES FROM MyModule.MyService INTO TargetModule;
```

**Expected:** External entities created in `TargetModule` instead of default.

### 6.9 Create external entities — selective

```
CREATE EXTERNAL ENTITIES FROM MyModule.MyService ENTITIES (Customer, Order);
```

**Expected:** Only `Customer` and `Order` external entities created.

### 6.10 Non-existent service

```
SHOW CONTRACT ENTITIES FROM MyModule.Fake;
```

**Expected:** Error — service not found.

---

## 7. SHOW DESIGN PROPERTIES / DESCRIBE STYLING / ALTER STYLING

### 7.1 Show all design properties

```
SHOW DESIGN PROPERTIES;
```

**Expected:** Table of available design properties with columns: `WidgetType | Property | Values`.

### 7.2 Show design properties — filtered

```
SHOW DESIGN PROPERTIES FOR TextBox;
```

**Expected:** Only design properties applicable to `TextBox` widget.

### 7.3 Describe styling on page

```
DESCRIBE STYLING ON PAGE MyModule.HomePage;
```

**Expected:** Lists all widgets on the page with their current `Class`, `Style`, and design property values.

### 7.4 Describe styling on snippet

```
DESCRIBE STYLING ON SNIPPET MyModule.Header;
```

**Expected:** Same format, scoped to snippet.

### 7.5 Describe styling — specific widget

```
DESCRIBE STYLING ON PAGE MyModule.HomePage WIDGET saveButton;
```

**Expected:** Styling for only the named widget.

### 7.6 Alter styling — set class and style

```
ALTER STYLING ON PAGE MyModule.HomePage WIDGET saveButton SET Class='btn-primary mx-2', Style='color: red;';
```

**Expected:** Widget updated. `DESCRIBE STYLING` confirms new values.

### 7.7 Alter styling — clear design properties

```
ALTER STYLING ON PAGE MyModule.HomePage WIDGET saveButton SET Class='btn-default' CLEAR DESIGN PROPERTIES;
```

**Expected:** Class set, all design properties cleared. `DESCRIBE STYLING` shows empty design properties.

### 7.8 Alter styling on snippet

```
ALTER STYLING ON SNIPPET MyModule.Header WIDGET title SET Class='text-bold';
```

**Expected:** Snippet widget updated.

### 7.9 Non-existent widget

```
ALTER STYLING ON PAGE MyModule.HomePage WIDGET nonexistent SET Class='x';
```

**Expected:** Error — widget not found.

### 7.10 Non-existent page

```
DESCRIBE STYLING ON PAGE MyModule.Fake;
```

**Expected:** Error — page not found.

---

## 8. SHOW LANGUAGES

### 8.1 Show languages — after catalog refresh

```
REFRESH CATALOG FULL;
SHOW LANGUAGES;
```

**Expected:** Table with columns: `Language | Strings`. Lists all configured languages with translation string counts.

### 8.2 Show languages — without catalog

Start a fresh session without running `REFRESH CATALOG FULL`.

```
SHOW LANGUAGES;
```

**Expected:** Error — requires `REFRESH CATALOG FULL` first.

---

## 9. MULTI-STEP WORKFLOWS

### 9.1 Create → Rename → Lint

```mdl
-- Step 1: Create entities
CREATE PERSISTENT ENTITY MyModule.OldProduct (
  Name: string(200),
  Price: decimal
);

CREATE PERSISTENT ENTITY MyModule.Category (
  Label: string(100)
);

CREATE ASSOCIATION MyModule.OldProduct_Category
FROM MyModule.OldProduct TO MyModule.Category
TYPE reference OWNER default;
```

Save as `setup.mdl` and execute the script, then:

```
-- Step 2: Rename
RENAME ENTITY MyModule.OldProduct TO Product;
```

**Expected:** `Renamed`. References in association updated.

```
-- Step 3: Verify references
DESCRIBE ASSOCIATION MyModule.OldProduct_Category;
```

**Expected:** Association now references `MyModule.Product` (or association also renamed).

```
-- Step 4: Lint
LINT MyModule;
```

**Expected:** No naming violations for the renamed entity.

### 9.2 Full tooling pipeline

1. `mxcli describe --format elk entity MyModule.Customer` — capture baseline
2. Create new entities via script
3. Execute script
4. `mxcli describe --format mermaid entity MyModule.Customer` — visualize updated model
5. `LINT MyModule` — check for issues
6. `RENAME ENTITY MyModule.Temp TO Final` — clean up names
7. `mxcli describe --format elk entity MyModule.Customer` — verify final layout

**Expected:** Each step succeeds. Final ELK output includes all new entities.

---

## 10. FAILURE MODES

### 10.1 Not connected

Run any tooling command without an open `.mpr`:

```
LINT;
```

**Expected:** Error — not connected to a project.

### 10.2 Document not found

```
RENAME MICROFLOW MyModule.NonExistent TO Something;
```

**Expected:** Error — microflow not found.

### 10.3 No catalog for SHOW LANGUAGES

```
SHOW LANGUAGES;
```

**Expected:** Error — run `REFRESH CATALOG FULL` first.

### 10.4 LINT with invalid config

Create a malformed `.mdllint.yaml`:

```yaml
excludeModules: "not-a-list"
```

**Expected:** Error or warning about invalid config format.

---

## Test Project Coverage Matrix

| Operation | Lato Enquiry | Evora Factory | Lato Product |
|-----------|:---:|:---:|:---:|
| RENAME (all types) | x | x | |
| RENAME DRY RUN | x | | |
| RENAME MODULE | x | | |
| LINT | x | x | x |
| LINT FORMAT json/sarif | x | | |
| SHOW LINT RULES | x | | |
| ELK entity (CLI --format elk) | x | x | x |
| ELK microflow (CLI --format elk) | x | x | |
| MERMAID entity (CLI --format mermaid) | x | x | x |
| MERMAID microflow (CLI) | x | x | |
| MERMAID nanoflow (CLI) | x | | |
| MERMAID page (CLI) | x | x | |
| CONTRACT (all) | x | | |
| DESIGN PROPERTIES | x | x | |
| SHOW LANGUAGES | x | | |

Read operations tested on all applicable projects. Write operations on copies of one project.

---

## Automated Test Coverage

| Section | Automated | Manual-only |
|---------|:---------:|:-----------:|
| 1. RENAME | Mock tests | Module rename, prefix scan |
| 2. LINT | Mock tests | Custom Starlark rules, SARIF |
| 3. ELK DOMAIN MODEL | CLI only | All (--format elk on describe) |
| 4. ELK MICROFLOW | CLI only | All (--format elk on describe) |
| 5. MERMAID | CLI only | All (--format mermaid on describe) |
| 6. CONTRACT | Mock tests | External entity creation |
| 7. DESIGN PROPERTIES | | All manual |
| 8. SHOW LANGUAGES | | All manual (requires catalog) |
| 9. Multi-step | | All manual |
| 10. Failure modes | Partial | Missing catalog, invalid config |

---

## Manual Test Report Template

**Tester:** _______________
**Date:** _______________
**Project:** _______________

| # | Section | Test | Pass | Fail | Skip | Notes |
|---|---------|------|:----:|:----:|:----:|-------|
| 1.1 | RENAME | Entity dry run | | | | |
| 1.2 | RENAME | Entity live | | | | |
| 1.3 | RENAME | Microflow | | | | |
| 1.4 | RENAME | Nanoflow | | | | |
| 1.5 | RENAME | Page | | | | |
| 1.6 | RENAME | Enumeration | | | | |
| 1.7 | RENAME | Association | | | | |
| 1.8 | RENAME | Constant | | | | |
| 1.9 | RENAME | Module | | | | |
| 1.10 | RENAME | Module prefix scan | | | | |
| 1.11 | RENAME | Not found | | | | |
| 1.12 | RENAME | Unsupported type | | | | |
| 1.13 | RENAME | Dry run no side effects | | | | |
| 2.1 | LINT | All modules | | | | |
| 2.2 | LINT | Single module | | | | |
| 2.3 | LINT | JSON format | | | | |
| 2.4 | LINT | SARIF format | | | | |
| 2.5 | LINT | Show rules | | | | |
| 2.6 | LINT | NamingConvention | | | | |
| 2.7 | LINT | EmptyMicroflow | | | | |
| 2.8 | LINT | DomainModelSize | | | | |
| 2.9 | LINT | ValidationFeedback | | | | |
| 2.10 | LINT | ImageSource | | | | |
| 2.11 | LINT | MissingTranslations | | | | |
| 2.12 | LINT | Custom Starlark rule | | | | |
| 2.13 | LINT | excludeModules config | | | | |
| 2.14 | LINT | Clean project | | | | |
| 3.1 | ELK DOMAIN | Focused entity | | | | |
| 3.2 | ELK DOMAIN | Entity categories | | | | |
| 3.3 | ELK DOMAIN | System overview | | | | |
| 3.4 | ELK DOMAIN | Focused view entity | | | | |
| 3.5 | ELK DOMAIN | Associations | | | | |
| 3.6 | ELK DOMAIN | Generalizations | | | | |
| 3.7 | ELK DOMAIN | Source map | | | | |
| 3.8 | ELK DOMAIN | Empty module | | | | |
| 3.9 | ELK DOMAIN | Non-existent entity | | | | |
| 4.1 | ELK MICROFLOW | Basic | | | | |
| 4.2 | ELK MICROFLOW | Node types | | | | |
| 4.3 | ELK MICROFLOW | Edge structure | | | | |
| 4.4 | ELK MICROFLOW | Loop compound | | | | |
| 4.5 | ELK MICROFLOW | Source map | | | | |
| 4.6 | ELK MICROFLOW | Not found | | | | |
| 5.1 | MERMAID | Entity erDiagram | | | | |
| 5.2 | MERMAID | Microflow flowchart | | | | |
| 5.3 | MERMAID | Nanoflow flowchart | | | | |
| 5.4 | MERMAID | Page block-beta | | | | |
| 5.5 | MERMAID | @colors metadata | | | | |
| 5.6 | MERMAID | @nodeinfo metadata | | | | |
| 5.7 | MERMAID | Not found | | | | |
| 6.1 | CONTRACT | Show entities | | | | |
| 6.2 | CONTRACT | Show actions | | | | |
| 6.3 | CONTRACT | Show channels | | | | |
| 6.4 | CONTRACT | Show messages | | | | |
| 6.5 | CONTRACT | Describe entity | | | | |
| 6.6 | CONTRACT | Describe FORMAT mdl | | | | |
| 6.7 | CONTRACT | Create external — all | | | | |
| 6.8 | CONTRACT | Create external — INTO | | | | |
| 6.9 | CONTRACT | Create external — selective | | | | |
| 6.10 | CONTRACT | Not found | | | | |
| 7.1 | DESIGN PROPS | Show all | | | | |
| 7.2 | DESIGN PROPS | Show filtered | | | | |
| 7.3 | STYLING | Describe page | | | | |
| 7.4 | STYLING | Describe snippet | | | | |
| 7.5 | STYLING | Describe widget | | | | |
| 7.6 | STYLING | Alter set class/style | | | | |
| 7.7 | STYLING | Alter clear design props | | | | |
| 7.8 | STYLING | Alter snippet widget | | | | |
| 7.9 | STYLING | Widget not found | | | | |
| 7.10 | STYLING | Page not found | | | | |
| 8.1 | LANGUAGES | After catalog refresh | | | | |
| 8.2 | LANGUAGES | Without catalog | | | | |
| 9.1 | MULTI-STEP | Create → Diff → Rename → Lint | | | | |
| 9.2 | MULTI-STEP | Full tooling pipeline | | | | |
| 10.1 | FAILURE | Not connected | | | | |
| 10.2 | FAILURE | Document not found | | | | |
| 10.3 | FAILURE | No catalog for LANGUAGES | | | | |
| 10.4 | FAILURE | Invalid lint config | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
