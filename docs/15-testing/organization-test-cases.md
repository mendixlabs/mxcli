# Organization Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/301)

## Test Projects

Demo apps from [Mendix App Gallery](https://appgallery.mendixcloud.com/):

| App | Studio Pro | Modules | Marketplace Modules |
|-----|-----------|---------|---------------------|
| Lato Enquiry Management | 11.4.0 | 15+ | 5+ |
| Evora - Factory Management | 10.24.15 | 20+ | 8+ |
| Lato Product Inventory | 11.2.0 | 12+ | 4+ |

Total: 3 projects with varying module counts and marketplace dependencies.

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
  echo "show modules;" > /tmp/show-mod.mdl
  mxcli exec /tmp/show-mod.mdl -p "$mpr" 2>&1 | head -5
done
```

Expected: Module list with columns for each app. No errors.

### 4. Interactive testing

```bash
mxcli repl -p <path-to-app>/EnquiriesManagement.mpr
```

### 5. Script-based testing

```bash
mxcli exec test-sequence.mdl -p <mpr>
```

Write operations (CREATE, DROP, MOVE) modify the `.mpr` file **in place**.

> **WARNING:** Always run destructive tests against a **copy** of the project folder,
> never the original. The `.mpr` file references other files in the project directory.
> Dropped modules and their contents cannot be recovered — there is no undo, no git history,
> and no Studio Pro autosave for `.mpr` files.
>
> ```bash
> # Before each destructive test session
> cp -r MyProject MyProject-test
> mxcli repl -p MyProject-test/MyProject.mpr
> ```

---

## 1. SHOW MODULES

### 1.1 List all modules

```
show modules;
```

**Expected:** Table with columns: Module, Source, Entities, Enums, Pages, Snippets, Microflows, Nanoflows, Workflows, Constants, JavaActions, PubREST, PubOData, ConOData, BizEvents, ExtDB. Rows sorted alphabetically by module name.

### 1.2 Marketplace module source display

```
show modules;
```

**Expected:** Marketplace modules display `Marketplace vX.Y` in the Source column. App modules display `App` or similar.

### 1.3 Column count accuracy

Pick 3 modules. Cross-check entity count, microflow count, and page count against Studio Pro.

**Expected:** All counts match Studio Pro.

### 1.4 Zero-count modules

Find a module with no nanoflows or no workflows.

**Expected:** Zero displayed, not blank or null.

### 1.5 Empty project

Open a project with a single empty module.

```
show modules;
```

**Expected:** One row, all counts zero except module name.

### 1.6 Large project

Run on Evora project (20+ modules).

**Expected:** All modules listed. No truncation. Alphabetical order maintained.

---

## 2. DESCRIBE MODULE

### 2.1 Basic describe

```
describe module MyModule;
```

**Expected:** Output is valid MDL: `create module MyModule;`. No contents listed.

### 2.2 Non-existent module

```
describe module NonExistentModule;
```

**Expected:** Clear error message.

### 2.3 Marketplace module

```
describe module Atlas_Core;
```

**Expected:** Valid MDL output. Source metadata preserved.

---

## 3. DESCRIBE MODULE WITH ALL

### 3.1 Full dump — dependency order

```
describe module MyModule with all;
```

**Expected:** Full MDL dump of module contents in dependency order:
1. Enumerations
2. Constants
3. Entities (with attributes and validation rules)
4. Associations
5. Microflows
6. Java actions
7. Pages
8. Snippets
9. Layouts
10. Database connections
11. Business events
12. OData services
13. Agents

### 3.2 Roundtrip — describe and recreate

1. `describe module MyModule with all;` → capture MDL
2. `drop module MyModule;`
3. Execute captured MDL
4. `describe module MyModule with all;` → capture again
5. Diff the two outputs

**Expected:** Identical or cosmetic-only differences.

### 3.3 Empty module

```
create module EmptyTest;
describe module EmptyTest with all;
```

**Expected:** Output contains `create module EmptyTest;` and nothing else.

### 3.4 Module with all document types

Find a module with entities, enums, microflows, nanoflows, pages, constants, and java actions. Run `describe module X with all;`.

**Expected:** All document types present in output. Dependency order respected — enums before entities that reference them, entities before microflows that use them.

### 3.5 Cross-module references in dump

Module A references entities from Module B.

**Expected:** References use qualified names (`ModuleB.EntityName`). No unresolved references in output.

---

## 4. CREATE MODULE

### 4.1 Create new module

```
create module TestOrg;
```

**Expected:** Module created. Appears in `show modules;`.

### 4.2 Idempotent create

```
create module TestOrg;
create module TestOrg;
```

**Expected:** No error on second create. Module exists once.

### 4.3 Create with special characters in name

```
create module My_Module_V2;
```

**Expected:** Created successfully with underscores and digits.

### 4.4 Verify in show modules

```
create module BrandNew;
show modules;
```

**Expected:** `BrandNew` appears with all counts at zero.

### 4.5 Write guard

Attempt CREATE without opening a project for writing.

**Expected:** Error about not being connected.

---

## 5. DROP MODULE

### 5.1 Drop empty module

```
create module DropTest;
drop module DropTest;
show modules;
```

**Expected:** Module removed. Not in `show modules`.

### 5.2 Cascade delete — all contents removed

1. Create module with entities, microflows, pages
2. `drop module TestModule;`
3. `show modules;` — module gone
4. `show entities;` — no entities from dropped module
5. `show microflows;` — no microflows from dropped module

**Expected:** All contained documents deleted.

### 5.3 Module role cleanup from user roles

1. Create module with module role
2. Grant module role to a user role
3. `drop module TestModule;`
4. Verify user role no longer references the dropped module role

**Expected:** Module roles removed from all user role assignments.

### 5.4 Themesource cleanup

Drop a module that has themesource files.

**Expected:** Corresponding `themesource/` directory contents cleaned up.

### 5.5 Drop non-existent module

```
drop module NonExistentModule;
```

**Expected:** Clear error message.

### 5.6 Drop marketplace module

```
drop module Atlas_Core;
```

**Expected:** Succeeds (or warns about marketplace status). All contents removed.

### 5.7 Write guard

**Expected:** Error if no project open for writing.

---

## 6. DROP FOLDER

### 6.1 Drop empty folder

```
drop folder 'TestFolder' in MyModule;
```

**Expected:** Folder removed. No error.

### 6.2 Drop non-empty folder

```
drop folder 'FolderWithContents' in MyModule;
```

**Expected:** Error — folder must be empty.

### 6.3 Drop nested folder

```
drop folder 'Parent/Child' in MyModule;
```

**Expected:** Only the `Child` folder removed. `Parent` remains.

### 6.4 Drop non-existent folder

```
drop folder 'DoesNotExist' in MyModule;
```

**Expected:** Clear error message.

### 6.5 Drop folder in non-existent module

```
drop folder 'SomeFolder' in NonExistent;
```

**Expected:** Error — module not found.

### 6.6 Write guard

**Expected:** Error if no project open for writing.

---

## 7. MOVE FOLDER

### 7.1 Move folder to another module

```
move folder MyModule.SubFolder to TargetModule;
```

**Expected:** Folder and all contents moved. References updated.

### 7.2 Move folder to specific target path

```
move folder MyModule.SubFolder to folder 'NewParent/NewChild' in TargetModule;
```

**Expected:** Folder placed at target path. Target folders auto-created if needed.

### 7.3 Move folder within same module

```
move folder MyModule.FolderA to folder 'Reorganized' in MyModule;
```

**Expected:** Folder relocated within same module.

### 7.4 Move non-existent folder

```
move folder MyModule.NonExistent to TargetModule;
```

**Expected:** Clear error message.

### 7.5 Move to non-existent module

```
move folder MyModule.SubFolder to NonExistent;
```

**Expected:** Error — target module not found.

### 7.6 Write guard

**Expected:** Error if no project open for writing.

---

## 8. MOVE (Documents)

### 8.1 Move page to another module

```
move page MyModule.MyPage to TargetModule;
```

**Expected:** Page moved. Qualified name becomes `TargetModule.MyPage`.

### 8.2 Move page to folder in another module

```
move page MyModule.MyPage to folder 'Pages/Imported' in TargetModule;
```

**Expected:** Page placed in target folder. Folder auto-created if needed.

### 8.3 Move microflow

```
move microflow MyModule.MyFlow to TargetModule;
```

**Expected:** Microflow moved. Cross-module callers updated.

### 8.4 Move nanoflow

```
move nanoflow MyModule.MyNano to TargetModule;
```

**Expected:** Nanoflow moved. References updated.

### 8.5 Move entity

```
move entity MyModule.MyEntity to TargetModule;
```

**Expected:** Entity moved. Association conversion handled — cross-module associations become cross-module references.

### 8.6 Move enumeration

```
move enumeration MyModule.StatusEnum to TargetModule;
```

**Expected:** Enumeration moved. All attributes using this enum updated.

### 8.7 Move snippet

```
move snippet MyModule.MySnippet to TargetModule;
```

**Expected:** Snippet moved. Pages referencing the snippet updated.

### 8.8 Move constant

```
move constant MyModule.MyConstant to TargetModule;
```

**Expected:** Constant moved. Microflow expressions referencing the constant updated.

### 8.9 Move database connection

```
move database connection MyModule.MyDBConn to TargetModule;
```

**Expected:** Database connection moved.

### 8.10 Cross-module reference updates

1. Create entity in ModuleA
2. Create microflow in ModuleB that retrieves from ModuleA.Entity
3. Move entity to ModuleC
4. Describe microflow in ModuleB

**Expected:** Microflow reference updated to `ModuleC.Entity`.

### 8.11 Role remapping on move

1. Create microflow in ModuleA with granted roles from ModuleA
2. Move microflow to ModuleB

**Expected:** Module role references remapped or warning issued about cross-module role references.

### 8.12 Move to non-existent module

```
move page MyModule.MyPage to NonExistent;
```

**Expected:** Error — target module not found.

### 8.13 Unsupported move type

Attempt to move a document type not in the supported list.

**Expected:** Clear error — unsupported move type.

### 8.14 Write guard

**Expected:** Error if no project open for writing.

---

## 9. SHOW STRUCTURE

### 9.1 Default depth (module level)

```
show structure;
```

**Expected:** Tree listing of all modules. Requires catalog.

### 9.2 Depth 1

```
show structure depth 1;
```

**Expected:** Modules only. No folder contents.

### 9.3 Depth 2

```
show structure depth 2;
```

**Expected:** Modules and their top-level folders/documents.

### 9.4 Depth 3

```
show structure depth 3;
```

**Expected:** Modules, folders, and nested subfolders with documents.

### 9.5 Filter by module

```
show structure in MyModule;
```

**Expected:** Structure of single module only.

### 9.6 ALL flag

```
show structure all;
```

**Expected:** Full tree with all depths expanded.

### 9.7 Empty module structure

```
create module EmptyMod;
show structure in EmptyMod;
```

**Expected:** Module listed with no children. No error.

### 9.8 Catalog required

Run `show structure;` without catalog loaded.

**Expected:** Error or auto-build of catalog.

### 9.9 Non-existent module filter

```
show structure in NonExistent;
```

**Expected:** Clear error message.

---

## 10. MULTI-STEP WORKFLOWS

### 10.1 Full module lifecycle

```
create module TestLifecycle;
create entity TestLifecycle.Customer (Name : String, Email : String);
create microflow TestLifecycle.GetCustomers () returns List of TestLifecycle.Customer
begin
  $Result = retrieve TestLifecycle.Customer;
end;
create nanoflow TestLifecycle.ShowCustomer (Input : TestLifecycle.Customer)
begin
  show page TestLifecycle.CustomerDetail (Input);
end;
show structure in TestLifecycle;
show modules;
```

**Expected:** Module contains entity, microflow, nanoflow. Structure shows correct hierarchy. Module counts accurate.

### 10.2 Move documents then verify references

1. Create ModuleA with entity and microflow referencing entity
2. Move entity to ModuleB
3. Describe microflow in ModuleA — verify reference updated to ModuleB.Entity
4. Show structure in ModuleA — entity gone
5. Show structure in ModuleB — entity present

**Expected:** All references intact after move.

### 10.3 Drop module after moving contents out

1. Create ModuleA with entities, microflows, pages
2. Move all documents to ModuleB
3. Drop ModuleA
4. Verify ModuleB contains all moved documents
5. Show modules — ModuleA gone

**Expected:** Clean drop after contents moved out.

### 10.4 Create → populate → describe with all → drop → recreate from describe

1. Create module with 3 entities, 2 enums, 2 microflows, 1 constant
2. `describe module TestMod with all;` → capture MDL
3. `drop module TestMod;`
4. Execute captured MDL
5. `describe module TestMod with all;` → capture again
6. Diff outputs

**Expected:** Identical or cosmetic-only differences. Full roundtrip preserved.

### 10.5 Folder organization workflow

1. Create module with documents in root
2. Create folder structure via document creation with `folder` clause
3. Move documents into folders
4. Show structure — verify hierarchy
5. Move folder to another module
6. Show structure — verify new location

**Expected:** Folder hierarchy maintained through all operations.

### 10.6 Cross-module reorganization

1. Create 3 modules: Source, TargetA, TargetB
2. Populate Source with entities, microflows, pages
3. Move entities to TargetA
4. Move microflows to TargetB
5. Describe microflows in TargetB — verify entity references point to TargetA
6. Drop Source (now empty)

**Expected:** All cross-module references correct after reorganization.

---

## 11. FAILURE MODES & ERROR RECOVERY

### 11.1 Not connected

```
create module Test;
```

**Expected:** Error — not connected to project.

### 11.2 Module not found

```
describe module NonExistent;
drop module NonExistent;
show structure in NonExistent;
```

**Expected:** Clear error for each command. Includes module name in message.

### 11.3 Folder not empty on drop

```
drop folder 'FolderWithDocs' in MyModule;
```

**Expected:** Error — folder is not empty. Lists or hints at contents.

### 11.4 Unsupported move type

Attempt to move a document type not in: PAGE, MICROFLOW, SNIPPET, NANOFLOW, ENTITY, ENUMERATION, CONSTANT, DATABASE CONNECTION.

**Expected:** Error — unsupported document type for move.

### 11.5 Move to self

```
move page MyModule.MyPage to MyModule;
```

**Expected:** No-op or clear message — already in target module.

### 11.6 Drop module with cross-module references

1. ModuleA.Entity referenced by ModuleB.Microflow
2. `drop module ModuleA;`

**Expected:** Module dropped. Dangling references in ModuleB handled (warning or graceful error on describe).

### 11.7 Batch abort on error

```
create module Good1;
drop module NonExistent;
create module Good2;
```

**Expected:** `Good1` created, `NonExistent` fails, `Good2` NOT created — batch aborts on first error.

> Batch mode (`mxcli exec`) is fail-fast. REPL mode continues on error per-line.

### 11.8 Error message quality

For each error scenario, verify the message includes:
- **What** went wrong
- **Which** module/folder/document (qualified name)
- **Actionable guidance** where applicable

Scenarios: not-found (DESCRIBE, DROP, MOVE, SHOW STRUCTURE IN), not-connected (CREATE, DROP, MOVE), folder not empty, unsupported move type.

---

## 12. FOLDER CREATION (implicit)

### 12.1 Auto-create via document creation

```
create microflow MyModule.TestFlow () folder 'NewFolder/Nested' begin end;
```

**Expected:** Folders `NewFolder` and `NewFolder/Nested` auto-created. Microflow placed in `NewFolder/Nested`.

### 12.2 Existing folder reuse

```
create microflow MyModule.Flow1 () folder 'Shared' begin end;
create microflow MyModule.Flow2 () folder 'Shared' begin end;
```

**Expected:** Both microflows in same `Shared` folder. No duplicate folder.

### 12.3 Deep nesting

```
create microflow MyModule.DeepFlow () folder 'A/B/C/D' begin end;
show structure in MyModule;
```

**Expected:** Four-level folder hierarchy created. Structure shows correct nesting.

---

## Test Project Coverage Matrix

| Category | Enquiries | Evora Factory | Lato Inventory |
|---|---|---|---|
| SHOW MODULES count | Verify total | Verify total | Verify total |
| SHOW MODULES column accuracy (sample 3) | Cross-check Studio Pro | Cross-check Studio Pro | Cross-check Studio Pro |
| Marketplace source display | Verify `Marketplace vX.Y` | Verify `Marketplace vX.Y` | Verify `Marketplace vX.Y` |
| DESCRIBE MODULE (sample 3) | App + marketplace | App + marketplace | App + marketplace |
| DESCRIBE MODULE WITH ALL (sample 2) | Roundtrip test | Roundtrip test | Roundtrip test |
| CREATE MODULE | New module | New module | New module |
| DROP MODULE cascade | Module with contents | Module with contents | Module with contents |
| SHOW STRUCTURE depth 1/2/3 | All depths | All depths | All depths |
| MOVE documents (sample 3 types) | Page, microflow, entity | Page, microflow, entity | Page, microflow, entity |
| MOVE FOLDER | Folder with contents | Folder with contents | Folder with contents |
| DROP FOLDER | Empty + non-empty | Empty + non-empty | Empty + non-empty |
| Multi-step workflows (§10) | Full lifecycle | Full lifecycle | Full lifecycle |
| Failure modes (§11) | Error scenarios | Error scenarios | Error scenarios |

---

## Automated Test Coverage

| Area | Tests | Status |
|---|---|---|
| SHOW MODULES | Integration + mock | Covered |
| DESCRIBE MODULE | Integration + mock | Covered |
| DESCRIBE MODULE WITH ALL | Integration | Covered |
| CREATE MODULE | Integration + mock | Covered |
| DROP MODULE | Integration + mock | Covered |
| DROP MODULE cascade | Integration | **Partial** |
| DROP FOLDER | Mock | Covered |
| MOVE FOLDER | Mock | Covered |
| MOVE PAGE | Integration + mock | Covered |
| MOVE MICROFLOW | Integration + mock | Covered |
| MOVE NANOFLOW | Integration + mock | Covered |
| MOVE ENTITY | Integration + mock | Covered |
| MOVE ENUMERATION | Mock | Covered |
| MOVE SNIPPET | Mock | Covered |
| MOVE CONSTANT | Mock | Covered |
| MOVE DATABASE CONNECTION | Mock | Covered |
| SHOW STRUCTURE | Integration | Covered |
| SHOW STRUCTURE depth levels | None | **Gap** |
| Cross-module reference updates | Partial | **Mostly manual** |
| Role remapping on move | None | **Manual only** |
| Themesource cleanup on drop | None | **Manual only** |
| Multi-step workflows (§10) | None | **Manual only** |
| Failure modes (§11) | Partial | **Mostly manual** |
| Folder auto-creation (§12) | Partial | **Mostly manual** |

Manual testing priority:
1. DROP MODULE cascade — verify all contents removed, roles cleaned, themesource deleted
2. MOVE documents — cross-module reference updates for each supported type
3. DESCRIBE MODULE WITH ALL roundtrip — full dump and recreate
4. Multi-step workflows (§10) — highest interaction bug risk
5. SHOW STRUCTURE depth levels — verify tree accuracy at each depth

---

## Manual Test Report Template

Copy and fill in after running manual tests.

```markdown
## Manual Testing

**Date:** YYYY-MM-DD
**Build:** `make build && make test && make lint-go` — PASS

### Test Projects

| App | Studio Pro | Modules | SHOW count | DESCRIBE sample | WITH ALL roundtrip |
|-----|-----------|---------|------------|-----------------|-------------------|
| Lato Enquiry Management | 11.4.0 | _n_ | ✅ _n_ | ✅ _n_/_n_ | ✅ _n_/_n_ |
| Evora Factory Management | 10.24.15 | _n_ | ✅ _n_ | ✅ _n_/_n_ | ✅ _n_/_n_ |
| Lato Product Inventory | 11.2.0 | _n_ | ✅ _n_ | ✅ _n_/_n_ | ✅ _n_/_n_ |

### Command Coverage

| Command | Tested | Notes |
|---------|--------|-------|
| SHOW MODULES | ✅/❌ | |
| DESCRIBE MODULE | ✅/❌ | |
| DESCRIBE MODULE WITH ALL | ✅/❌ | |
| CREATE MODULE | ✅/❌ | |
| DROP MODULE | ✅/❌ | |
| DROP MODULE cascade | ✅/❌ | |
| DROP FOLDER | ✅/❌ | |
| DROP FOLDER (non-empty) | ✅/❌ | |
| MOVE FOLDER | ✅/❌ | |
| MOVE PAGE | ✅/❌ | |
| MOVE MICROFLOW | ✅/❌ | |
| MOVE NANOFLOW | ✅/❌ | |
| MOVE ENTITY | ✅/❌ | |
| MOVE ENUMERATION | ✅/❌ | |
| MOVE SNIPPET | ✅/❌ | |
| MOVE CONSTANT | ✅/❌ | |
| MOVE DATABASE CONNECTION | ✅/❌ | |
| SHOW STRUCTURE | ✅/❌ | |
| SHOW STRUCTURE DEPTH 1/2/3 | ✅/❌ | |
| SHOW STRUCTURE IN module | ✅/❌ | |
| SHOW STRUCTURE ALL | ✅/❌ | |

### SHOW MODULES Column Accuracy

| Module | Column | Studio Pro | mxcli | Match |
|--------|--------|-----------|-------|-------|
| _name_ | Entities | _n_ | _n_ | ✅/❌ |
| _name_ | Microflows | _n_ | _n_ | ✅/❌ |
| _name_ | Pages | _n_ | _n_ | ✅/❌ |

### DESCRIBE MODULE WITH ALL Roundtrip

```
Total: _n_ modules tested
Passed: _n_
Failed: _n_ (list failures below)
```

### Move Document Results

| Type | Source → Target | References Updated | Roles Remapped | Result |
|------|----------------|-------------------|----------------|--------|
| PAGE | _M.Name → M2_ | ✅/❌ | ✅/❌/N/A | ✅/❌ |
| MICROFLOW | _M.Name → M2_ | ✅/❌ | ✅/❌/N/A | ✅/❌ |
| ENTITY | _M.Name → M2_ | ✅/❌ | N/A | ✅/❌ |

### Multi-Step Workflows (§10)

| Scenario | Result | Notes |
|----------|--------|-------|
| 10.1 Full module lifecycle | ✅/❌ | |
| 10.2 Move then verify refs | ✅/❌ | |
| 10.3 Drop after move-out | ✅/❌ | |
| 10.4 Describe-drop-recreate roundtrip | ✅/❌ | |
| 10.5 Folder organization | ✅/❌ | |
| 10.6 Cross-module reorganization | ✅/❌ | |

### Failure Modes (§11)

| Scenario | Result | Notes |
|----------|--------|-------|
| 11.1 Not connected | ✅/❌ | |
| 11.2 Module not found | ✅/❌ | |
| 11.3 Folder not empty | ✅/❌ | |
| 11.4 Unsupported move type | ✅/❌ | |
| 11.5 Move to self | ✅/❌ | |
| 11.6 Drop with cross-module refs | ✅/❌ | |
| 11.7 Batch abort | ✅/❌ | |

### Issues Found

1. (none / describe issues here)
```
