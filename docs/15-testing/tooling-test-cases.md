# Tooling Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/301)

## Test Projects

Demo apps from [Mendix App Gallery](https://appgallery.mendixcloud.com/):

| App | Studio Pro | Modules | Entities | Microflows |
|-----|-----------|---------|----------|------------|
| Lato Enquiry Management | 11.4.0 | <N> | <N> | <N> |
| Evora - Factory Management | 10.24.15 | <N> | <N> | <N> |
| Lato Product Inventory | 11.2.0 | <N> | <N> | <N> |

---

## Setup

### 1. Download test apps

1. Go to [Mendix App Gallery](https://appgallery.mendixcloud.com/)
2. Download each demo app listed above
3. Open each `.mpk` in Studio Pro to extract the `.mpr` file

### 2. Build mxcli

```bash
make build && make test && make lint-go
```

### 3. Smoke test

```bash
APPS_DIR=<path-to-extracted-apps>
for mpr in "$APPS_DIR"/*/*.mpr; do
  echo "=== $(basename $(dirname $mpr)) ==="
  echo "show entities;" > /tmp/smoke.mdl
  mxcli exec /tmp/smoke.mdl -p "$mpr" 2>&1 | tail -1
done
```

Expected: count line `(N entities)` for each project.

### 4. Interactive testing

```bash
mxcli repl -p <path-to-app>/EnquiriesManagement.mpr
```

### 5. Script-based testing

```bash
mxcli exec test-sequence.mdl -p <mpr>
```

Write operations modify the `.mpr` file **in place**.

> **IMPORTANT:** Always run destructive tests against a **copy** of the project folder,
> never the original. The `.mpr` file references other files in the project directory.
> Dropped documents cannot be recovered — there is no undo.
>
> ```bash
> # Before each destructive test session
> cp -r MyProject MyProject-test
> mxcli repl -p MyProject-test/MyProject.mpr
> ```

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

## 2. DIFF (MDL Script)

### 2.1 Basic diff — unified format

Create a script `changes.mdl` that adds an entity:

```
CREATE PERSISTENT ENTITY MyModule.NewThing (
  Name: string(200)
);
```

```
DIFF 'changes.mdl';
```

**Expected:** Unified diff output. Summary line: `1 new, 0 modified, 0 unchanged`.

### 2.2 Diff — side format

```
SET format = side;
DIFF 'changes.mdl';
```

**Expected:** Side-by-side diff output. Same summary counts.

### 2.3 Diff — struct format

```
SET format = struct;
DIFF 'changes.mdl';
```

**Expected:** Structural diff output showing entity tree. Same summary counts.

### 2.4 Diff with modifications

Create a script that modifies an existing entity (via `CREATE OR MODIFY`):

```
CREATE OR MODIFY PERSISTENT ENTITY MyModule.Existing (
  Name: string(200),
  NewAttr: integer
);
```

```
DIFF 'modify.mdl';
```

**Expected:** Shows `NewAttr` as added. Summary: `0 new, 1 modified, 0 unchanged`.

### 2.5 Diff — CREATE ENTITY

```
DIFF 'create-entity.mdl';
```

**Expected:** New entity shown in diff output.

### 2.6 Diff — VIEW ENTITY

Diff a script containing a `CREATE VIEW ENTITY` statement.

**Expected:** View entity and OQL source shown in diff.

### 2.7 Diff — ENUMERATION

Diff a script containing a `CREATE ENUMERATION` statement.

**Expected:** Enumeration values shown in diff.

### 2.8 Diff — ASSOCIATION

Diff a script containing a `CREATE ASSOCIATION` statement.

**Expected:** Association details shown in diff.

### 2.9 Diff — MICROFLOW

Diff a script containing microflow changes.

**Expected:** Microflow actions shown in diff.

### 2.10 Diff — NANOFLOW

Diff a script containing nanoflow changes.

**Expected:** Nanoflow actions shown in diff.

### 2.11 Diff — mixed script

Create a script with multiple CREATE and MODIFY statements across types.

```
DIFF 'mixed.mdl';
```

**Expected:** Summary: `N new, N modified, N unchanged`. Each change listed by type.

### 2.12 Diff — non-existent script

```
DIFF 'missing.mdl';
```

**Expected:** Error — file not found.

### 2.13 Diff — empty script

```
DIFF 'empty.mdl';
```

**Expected:** Summary: `0 new, 0 modified, 0 unchanged`.

---

## 3. DIFF LOCAL (Git-based)

### 3.1 Diff local — HEAD

```
DIFF LOCAL 'HEAD';
```

**Expected:** Shows changes in working tree vs last commit. Compares `.mxunit` files via git diff. Same format options as DIFF.

### 3.2 Diff local — branch range

```
DIFF LOCAL 'main..feature';
```

**Expected:** Shows all model changes between `main` and `feature` branches.

### 3.3 Diff local — unified format (default)

```
DIFF LOCAL 'HEAD';
```

**Expected:** Unified diff output with `+`/`-` lines for changed model elements.

### 3.4 Diff local — side format

```
SET format = side;
DIFF LOCAL 'HEAD';
```

**Expected:** Side-by-side format.

### 3.5 Diff local — struct format

```
SET format = struct;
DIFF LOCAL 'HEAD';
```

**Expected:** Structural diff of model tree.

### 3.6 Diff local — MPR v1 rejection

Open a project with MPR v1 (pre-Mendix 10.18).

```
DIFF LOCAL 'HEAD';
```

**Expected:** Error — DIFF LOCAL requires MPR v2 (Mendix 10.18+).

### 3.7 Diff local — no changes

```
DIFF LOCAL 'HEAD';
```

Run on a clean working tree with no uncommitted changes.

**Expected:** Summary: `0 new, 0 modified, 0 unchanged`.

### 3.8 Diff local — invalid ref

```
DIFF LOCAL 'nonexistent-branch';
```

**Expected:** Error — git ref not found.

---

## 4. LINT

### 4.1 Lint all modules

```
LINT;
```

**Expected:** Table of findings with columns: `Rule | Severity | Document | Message`. Summary: `N finding(s)`.

### 4.2 Lint single module

```
LINT MyModule;
```

**Expected:** Only findings from `MyModule`.

### 4.3 Lint — JSON format

```
LINT FORMAT json;
```

**Expected:** JSON array of finding objects with fields: `rule`, `severity`, `document`, `message`, `location`.

### 4.4 Lint — SARIF format

```
LINT FORMAT sarif;
```

**Expected:** Valid SARIF v2.1.0 JSON. Contains `runs[0].results` with finding entries.

### 4.5 Show lint rules

```
SHOW LINT RULES;
```

**Expected:** Table listing all built-in and custom rules with columns: `Rule | Severity | Description | Source`.

### 4.6 Built-in rule — NamingConvention

Create an entity or microflow with non-standard naming.

**Expected:** `LINT` reports `NamingConvention` violation.

### 4.7 Built-in rule — EmptyMicroflow

Create a microflow with only start and end nodes.

**Expected:** `LINT` reports `EmptyMicroflow` finding.

### 4.8 Built-in rule — DomainModelSize

Use a module with many entities.

**Expected:** If threshold exceeded, `LINT` reports `DomainModelSize` warning.

### 4.9 Built-in rule — ValidationFeedback

Create an entity attribute with `not null` but no validation message.

**Expected:** `LINT` reports `ValidationFeedback` finding.

### 4.10 Built-in rule — ImageSource

Use an image widget referencing a missing or oversized image.

**Expected:** `LINT` reports `ImageSource` finding.

### 4.11 Built-in rule — MissingTranslations

Use a project with multiple languages and missing translation strings.

**Expected:** `LINT` reports `MissingTranslations` finding.

### 4.12 Custom Starlark rule

Place a `.star` rule file in `.claude/lint-rules/`. Run `LINT`.

**Expected:** Custom rule appears in `SHOW LINT RULES`. Findings from custom rule included in output.

### 4.13 Config — excludeModules

Create `.mdllint.yaml` with:

```yaml
excludeModules:
  - Administration
  - System
```

Run `LINT`.

**Expected:** No findings from `Administration` or `System` modules.

### 4.14 Lint — clean project

Run on a project with no violations.

**Expected:** `0 finding(s)`.

---

## 5. ELK DOMAIN MODEL

### 5.1 Full module domain model

```
ELK DOMAIN MODEL MyModule;
```

**Expected:** JSON output with top-level keys: `entities`, `associations`, `generalizations`, `sourceMap`. Each entity has `id`, `name`, `category`, `attributes`.

### 5.2 Entity categories

Verify `category` field across entity types:

| Category | Condition |
|----------|-----------|
| `persistent` | Normal persistent entity |
| `nonpersistent` | Non-persistent entity |
| `external` | External/OData entity |
| `view` | View entity |

### 5.3 Focused entity view

```
ELK DOMAIN MODEL MyModule.Customer;
```

**Expected:** JSON with only `MyModule.Customer` and its directly connected entities via associations/generalizations.

### 5.4 Focused view — view entity

```
ELK DOMAIN MODEL MyModule.ViewEntity;
```

**Expected:** Delegates to OQL plan for view entities. Output includes source entities from OQL query.

### 5.5 Associations in output

**Expected:** Each association object contains `id`, `name`, `parent`, `child`, `type` (`Reference` / `ReferenceSet`).

### 5.6 Generalizations in output

**Expected:** Each generalization contains `child` and `parent` entity references.

### 5.7 Source map

**Expected:** `sourceMap` maps entity/association IDs to qualified names for client-side lookups.

### 5.8 Empty module

```
ELK DOMAIN MODEL EmptyModule;
```

**Expected:** JSON with empty `entities`, `associations`, `generalizations` arrays.

### 5.9 Non-existent module

```
ELK DOMAIN MODEL FakeModule;
```

**Expected:** Error — module not found.

---

## 6. ELK MICROFLOW

### 6.1 Basic microflow

```
ELK MICROFLOW MyModule.ACT_Simple;
```

**Expected:** JSON with `nodes`, `edges`, `sourceMap`. Nodes include at least `start` and `end` types.

### 6.2 Node types

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

### 6.3 Edge structure

**Expected:** Each edge has `source`, `target`, optional `label` (for split conditions).

### 6.4 Loop node — compound

```
ELK MICROFLOW MyModule.ACT_WithLoop;
```

**Expected:** Loop node contains `children` (inner nodes) and `innerEdges`. Loop body rendered as sub-graph.

### 6.5 Source map

**Expected:** `sourceMap` maps node IDs to action names/descriptions.

### 6.6 Non-existent microflow

```
ELK MICROFLOW MyModule.Fake;
```

**Expected:** Error — microflow not found.

---

## 7. MERMAID

### 7.1 Entity / domain model — erDiagram

```
MERMAID ENTITY MyModule;
```

**Expected:** Mermaid `erDiagram` syntax. Entities as blocks, associations as relationships. Metadata comments: `%% @colors`, `%% @nodeinfo`.

### 7.2 Domain model alias

```
MERMAID DOMAINMODEL MyModule;
```

**Expected:** Same output as `MERMAID ENTITY MyModule`.

### 7.3 Microflow — flowchart LR

```
MERMAID MICROFLOW MyModule.ACT_Process;
```

**Expected:** Mermaid `flowchart LR` syntax. Start/end as rounded nodes, decisions as diamonds, actions as rectangles. Metadata comments present.

### 7.4 Nanoflow — flowchart LR

```
MERMAID NANOFLOW MyModule.NAF_Validate;
```

**Expected:** Same format as microflow. `flowchart LR` with nanoflow-specific nodes.

### 7.5 Page — block-beta

```
MERMAID PAGE MyModule.HomePage;
```

**Expected:** Mermaid `block-beta` syntax representing page layout. Widgets as blocks.

### 7.6 Metadata comments — @colors

**Expected:** Output includes `%% @colors` comment with color assignments for entity categories or node types.

### 7.7 Metadata comments — @nodeinfo

**Expected:** Output includes `%% @nodeinfo` comment with node metadata (IDs, types).

### 7.8 Non-existent document

```
MERMAID MICROFLOW MyModule.Fake;
```

**Expected:** Error — document not found.

---

## 8. CONTRACT

### 8.1 Show contract entities

```
SHOW CONTRACT ENTITIES FROM MyModule.MyService;
```

**Expected:** Table of published/consumed entities in the service contract.

### 8.2 Show contract actions

```
SHOW CONTRACT ACTIONS FROM MyModule.MyService;
```

**Expected:** Table of published microflow actions.

### 8.3 Show contract channels

```
SHOW CONTRACT CHANNELS FROM MyModule.MyService;
```

**Expected:** Table of message channels.

### 8.4 Show contract messages

```
SHOW CONTRACT MESSAGES FROM MyModule.MyService;
```

**Expected:** Table of message definitions.

### 8.5 Describe contract entity

```
DESCRIBE CONTRACT ENTITY MyModule.MyService.Customer;
```

**Expected:** Detailed entity description: attributes, types, key fields.

### 8.6 Describe contract entity — MDL format

```
DESCRIBE CONTRACT ENTITY MyModule.MyService.Customer FORMAT mdl;
```

**Expected:** MDL-syntax output for the contract entity.

### 8.7 Create external entities — all

```
CREATE EXTERNAL ENTITIES FROM MyModule.MyService;
```

**Expected:** External entities created in the service's module. `show entities` lists them as `External` type.

### 8.8 Create external entities — into specific module

```
CREATE EXTERNAL ENTITIES FROM MyModule.MyService INTO TargetModule;
```

**Expected:** External entities created in `TargetModule` instead of default.

### 8.9 Create external entities — selective

```
CREATE EXTERNAL ENTITIES FROM MyModule.MyService ENTITIES (Customer, Order);
```

**Expected:** Only `Customer` and `Order` external entities created.

### 8.10 Non-existent service

```
SHOW CONTRACT ENTITIES FROM MyModule.Fake;
```

**Expected:** Error — service not found.

---

## 9. SHOW DESIGN PROPERTIES / DESCRIBE STYLING / ALTER STYLING

### 9.1 Show all design properties

```
SHOW DESIGN PROPERTIES;
```

**Expected:** Table of available design properties with columns: `WidgetType | Property | Values`.

### 9.2 Show design properties — filtered

```
SHOW DESIGN PROPERTIES FOR TextBox;
```

**Expected:** Only design properties applicable to `TextBox` widget.

### 9.3 Describe styling on page

```
DESCRIBE STYLING ON PAGE MyModule.HomePage;
```

**Expected:** Lists all widgets on the page with their current `Class`, `Style`, and design property values.

### 9.4 Describe styling on snippet

```
DESCRIBE STYLING ON SNIPPET MyModule.Header;
```

**Expected:** Same format, scoped to snippet.

### 9.5 Describe styling — specific widget

```
DESCRIBE STYLING ON PAGE MyModule.HomePage WIDGET 'saveButton';
```

**Expected:** Styling for only the named widget.

### 9.6 Alter styling — set class and style

```
ALTER STYLING ON PAGE MyModule.HomePage WIDGET 'saveButton' SET Class='btn-primary mx-2', Style='color: red;';
```

**Expected:** Widget updated. `DESCRIBE STYLING` confirms new values.

### 9.7 Alter styling — clear design properties

```
ALTER STYLING ON PAGE MyModule.HomePage WIDGET 'saveButton' SET Class='btn-default' CLEAR DESIGN PROPERTIES;
```

**Expected:** Class set, all design properties cleared. `DESCRIBE STYLING` shows empty design properties.

### 9.8 Alter styling on snippet

```
ALTER STYLING ON SNIPPET MyModule.Header WIDGET 'title' SET Class='text-bold';
```

**Expected:** Snippet widget updated.

### 9.9 Non-existent widget

```
ALTER STYLING ON PAGE MyModule.HomePage WIDGET 'nonexistent' SET Class='x';
```

**Expected:** Error — widget not found.

### 9.10 Non-existent page

```
DESCRIBE STYLING ON PAGE MyModule.Fake;
```

**Expected:** Error — page not found.

---

## 10. SHOW LANGUAGES

### 10.1 Show languages — after catalog refresh

```
REFRESH CATALOG FULL;
SHOW LANGUAGES;
```

**Expected:** Table with columns: `Language | Strings`. Lists all configured languages with translation string counts.

### 10.2 Show languages — without catalog

Start a fresh session without running `REFRESH CATALOG FULL`.

```
SHOW LANGUAGES;
```

**Expected:** Error — requires `REFRESH CATALOG FULL` first.

---

## 11. MULTI-STEP WORKFLOWS

### 11.1 Create → Diff → Rename → Lint

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

Save as `setup.mdl`, then:

```
-- Step 2: Diff the script
DIFF 'setup.mdl';
```

**Expected:** `3 new, 0 modified, 0 unchanged`.

Execute the script, then:

```
-- Step 3: Rename
RENAME ENTITY MyModule.OldProduct TO Product;
```

**Expected:** `Renamed`. References in association updated.

```
-- Step 4: Verify references
DESCRIBE ASSOCIATION MyModule.OldProduct_Category;
```

**Expected:** Association now references `MyModule.Product` (or association also renamed).

```
-- Step 5: Lint
LINT MyModule;
```

**Expected:** No naming violations for the renamed entity.

### 11.2 Full tooling pipeline

1. `ELK DOMAIN MODEL MyModule` — capture baseline
2. Create new entities via script
3. `DIFF 'changes.mdl'` — preview changes
4. Execute script
5. `MERMAID ENTITY MyModule` — visualize updated model
6. `LINT MyModule` — check for issues
7. `RENAME ENTITY MyModule.Temp TO Final` — clean up names
8. `ELK DOMAIN MODEL MyModule` — verify final layout

**Expected:** Each step succeeds. Final ELK output includes all new entities.

---

## 12. FAILURE MODES

### 12.1 Not connected

Run any tooling command without an open `.mpr`:

```
LINT;
```

**Expected:** Error — not connected to a project.

### 12.2 Document not found

```
RENAME MICROFLOW MyModule.NonExistent TO Something;
```

**Expected:** Error — microflow not found.

### 12.3 Unsupported type for rename

```
RENAME WORKFLOW MyModule.Old TO New;
```

**Expected:** Error — unsupported type. Lists supported types.

### 12.4 MPR v1 for DIFF LOCAL

Open an MPR v1 project:

```
DIFF LOCAL 'HEAD';
```

**Expected:** Error — MPR v2 required (Mendix 10.18+).

### 12.5 No catalog for SHOW LANGUAGES

```
SHOW LANGUAGES;
```

**Expected:** Error — run `REFRESH CATALOG FULL` first.

### 12.6 Invalid DIFF script path

```
DIFF '/nonexistent/path/script.mdl';
```

**Expected:** Error — file not found.

### 12.7 LINT with invalid config

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
| DIFF (MDL Script) | x | x | |
| DIFF LOCAL | x | | |
| LINT | x | x | x |
| LINT FORMAT json/sarif | x | | |
| SHOW LINT RULES | x | | |
| ELK DOMAIN MODEL | x | x | x |
| ELK MICROFLOW | x | x | |
| MERMAID ENTITY | x | x | x |
| MERMAID MICROFLOW | x | x | |
| MERMAID NANOFLOW | x | | |
| MERMAID PAGE | x | x | |
| CONTRACT (all) | x | | |
| DESIGN PROPERTIES | x | x | |
| SHOW LANGUAGES | x | | |

Read operations tested on all applicable projects. Write operations on copies of one project.

---

## Automated Test Coverage

| Section | Automated | Manual-only |
|---------|:---------:|:-----------:|
| 1. RENAME | Mock tests | Module rename, prefix scan |
| 2. DIFF (MDL Script) | Mock tests | Mixed scripts, format variants |
| 3. DIFF LOCAL | | All manual (requires git + MPR v2) |
| 4. LINT | Mock tests | Custom Starlark rules, SARIF |
| 5. ELK DOMAIN MODEL | Mock tests | View entity delegation |
| 6. ELK MICROFLOW | Mock tests | Compound loop nodes |
| 7. MERMAID | Mock tests | All format variants |
| 8. CONTRACT | Mock tests | External entity creation |
| 9. DESIGN PROPERTIES | | All manual |
| 10. SHOW LANGUAGES | | All manual (requires catalog) |
| 11. Multi-step | | All manual |
| 12. Failure modes | Partial | MPR v1, missing catalog |

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
| 2.1 | DIFF | Unified format | | | | |
| 2.2 | DIFF | Side format | | | | |
| 2.3 | DIFF | Struct format | | | | |
| 2.4 | DIFF | Modifications | | | | |
| 2.5 | DIFF | CREATE ENTITY | | | | |
| 2.6 | DIFF | VIEW ENTITY | | | | |
| 2.7 | DIFF | ENUMERATION | | | | |
| 2.8 | DIFF | ASSOCIATION | | | | |
| 2.9 | DIFF | MICROFLOW | | | | |
| 2.10 | DIFF | NANOFLOW | | | | |
| 2.11 | DIFF | Mixed script | | | | |
| 2.12 | DIFF | Non-existent script | | | | |
| 2.13 | DIFF | Empty script | | | | |
| 3.1 | DIFF LOCAL | HEAD | | | | |
| 3.2 | DIFF LOCAL | Branch range | | | | |
| 3.3 | DIFF LOCAL | Unified format | | | | |
| 3.4 | DIFF LOCAL | Side format | | | | |
| 3.5 | DIFF LOCAL | Struct format | | | | |
| 3.6 | DIFF LOCAL | MPR v1 rejection | | | | |
| 3.7 | DIFF LOCAL | No changes | | | | |
| 3.8 | DIFF LOCAL | Invalid ref | | | | |
| 4.1 | LINT | All modules | | | | |
| 4.2 | LINT | Single module | | | | |
| 4.3 | LINT | JSON format | | | | |
| 4.4 | LINT | SARIF format | | | | |
| 4.5 | LINT | Show rules | | | | |
| 4.6 | LINT | NamingConvention | | | | |
| 4.7 | LINT | EmptyMicroflow | | | | |
| 4.8 | LINT | DomainModelSize | | | | |
| 4.9 | LINT | ValidationFeedback | | | | |
| 4.10 | LINT | ImageSource | | | | |
| 4.11 | LINT | MissingTranslations | | | | |
| 4.12 | LINT | Custom Starlark rule | | | | |
| 4.13 | LINT | excludeModules config | | | | |
| 4.14 | LINT | Clean project | | | | |
| 5.1 | ELK DOMAIN | Full module | | | | |
| 5.2 | ELK DOMAIN | Entity categories | | | | |
| 5.3 | ELK DOMAIN | Focused entity | | | | |
| 5.4 | ELK DOMAIN | Focused view entity | | | | |
| 5.5 | ELK DOMAIN | Associations | | | | |
| 5.6 | ELK DOMAIN | Generalizations | | | | |
| 5.7 | ELK DOMAIN | Source map | | | | |
| 5.8 | ELK DOMAIN | Empty module | | | | |
| 5.9 | ELK DOMAIN | Non-existent module | | | | |
| 6.1 | ELK MICROFLOW | Basic | | | | |
| 6.2 | ELK MICROFLOW | Node types | | | | |
| 6.3 | ELK MICROFLOW | Edge structure | | | | |
| 6.4 | ELK MICROFLOW | Loop compound | | | | |
| 6.5 | ELK MICROFLOW | Source map | | | | |
| 6.6 | ELK MICROFLOW | Not found | | | | |
| 7.1 | MERMAID | Entity erDiagram | | | | |
| 7.2 | MERMAID | DOMAINMODEL alias | | | | |
| 7.3 | MERMAID | Microflow flowchart | | | | |
| 7.4 | MERMAID | Nanoflow flowchart | | | | |
| 7.5 | MERMAID | Page block-beta | | | | |
| 7.6 | MERMAID | @colors metadata | | | | |
| 7.7 | MERMAID | @nodeinfo metadata | | | | |
| 7.8 | MERMAID | Not found | | | | |
| 8.1 | CONTRACT | Show entities | | | | |
| 8.2 | CONTRACT | Show actions | | | | |
| 8.3 | CONTRACT | Show channels | | | | |
| 8.4 | CONTRACT | Show messages | | | | |
| 8.5 | CONTRACT | Describe entity | | | | |
| 8.6 | CONTRACT | Describe FORMAT mdl | | | | |
| 8.7 | CONTRACT | Create external — all | | | | |
| 8.8 | CONTRACT | Create external — INTO | | | | |
| 8.9 | CONTRACT | Create external — selective | | | | |
| 8.10 | CONTRACT | Not found | | | | |
| 9.1 | DESIGN PROPS | Show all | | | | |
| 9.2 | DESIGN PROPS | Show filtered | | | | |
| 9.3 | STYLING | Describe page | | | | |
| 9.4 | STYLING | Describe snippet | | | | |
| 9.5 | STYLING | Describe widget | | | | |
| 9.6 | STYLING | Alter set class/style | | | | |
| 9.7 | STYLING | Alter clear design props | | | | |
| 9.8 | STYLING | Alter snippet widget | | | | |
| 9.9 | STYLING | Widget not found | | | | |
| 9.10 | STYLING | Page not found | | | | |
| 10.1 | LANGUAGES | After catalog refresh | | | | |
| 10.2 | LANGUAGES | Without catalog | | | | |
| 11.1 | MULTI-STEP | Create → Diff → Rename → Lint | | | | |
| 11.2 | MULTI-STEP | Full tooling pipeline | | | | |
| 12.1 | FAILURE | Not connected | | | | |
| 12.2 | FAILURE | Document not found | | | | |
| 12.3 | FAILURE | Unsupported type | | | | |
| 12.4 | FAILURE | MPR v1 for DIFF LOCAL | | | | |
| 12.5 | FAILURE | No catalog for LANGUAGES | | | | |
| 12.6 | FAILURE | Invalid DIFF path | | | | |
| 12.7 | FAILURE | Invalid lint config | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
