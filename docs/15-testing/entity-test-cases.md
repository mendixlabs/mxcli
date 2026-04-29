# Entity, Association & Constant Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/301)

## Test Projects

Demo apps from [Mendix App Gallery](https://appgallery.mendixcloud.com/):

| App | Studio Pro | Entities | Associations | Constants |
|-----|-----------|----------|--------------|-----------|
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
  echo "show entities;" > /tmp/show-ent.mdl
  mxcli exec /tmp/show-ent.mdl -p "$mpr" 2>&1 | tail -1
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

Write operations (CREATE, ALTER, DROP) modify the `.mpr` file **in place**.

> **IMPORTANT:** Always run destructive tests against a **copy** of the project folder,
> never the original. The `.mpr` file references other files in the project directory.
> Dropped entities, associations, and constants cannot be recovered — there is no undo.
>
> ```bash
> # Before each destructive test session
> cp -r MyProject MyProject-test
> mxcli repl -p MyProject-test/MyProject.mpr
> ```

---

## 1. SHOW ENTITIES

### 1.1 List all entities

```
show entities;
```

**Expected:** Table with columns `Entity | Type | Attrs | Assocs | Validations | Indexes | Events | AccessRules`. Summary line `(N entities)`. Sorted alphabetically.

### 1.2 List entities in a module

```
show entities in Administration;
```

**Expected:** Only entities from `Administration` module. Same column format.

### 1.3 Generalization column

```
show entities;
```

**Expected:** If any entity has a generalization, an `Extends` column appears between `Type` and `Attrs`. System entities (referenced via generalization) show `-` for counts.

### 1.4 Empty module

```
show entities in NonExistentModule;
```

**Expected:** Error or empty result with `(0 entities)`.

### 1.5 Entity types

Verify `Type` column values across projects:

| Type | Example |
|------|---------|
| Persistent | Most domain entities |
| Non-Persistent | Transient/helper entities |
| External | OData/external entities |
| System | System.User, System.Session |

---

## 2. SHOW ENTITY (single)

### 2.1 Persistent entity

```
show entity Administration.Account;
```

**Expected:** Markdown-style output:
```
**Entity: Administration.Account**

- Persistable: true
- Extends: System.User

| Attribute | Type        |
| --------- | ----------- |
| ...       | ...         |

(N attributes)
```

### 2.2 Non-persistent entity

```
show entity <Module.NonPersistentEntity>;
```

**Expected:** `Persistable: false`. No `Extends` if no generalization.

### 2.3 Non-existent entity

```
show entity Fake.Entity;
```

**Expected:** Error message (not found).

---

## 3. DESCRIBE ENTITY

### 3.1 Basic entity

```
describe entity Administration.Account;
```

**Expected:** Roundtrippable MDL output:
```
create or modify persistent entity Administration.Account extends System.User (
  FullName: string(200),
  IsLocalUser: boolean,
  ...
)
/
```

### 3.2 Entity with all features

Find an entity with indexes, events, validation rules, and access rules. Verify output includes:
- `/** documentation */` block (if documented)
- `@Position(x, y)` annotation
- Attribute types and constraints (`not null error '...'`, `unique error '...'`)
- System pseudo-attributes (`Owner: AutoOwner`, etc.)
- `index (col1, col2 desc)` lines
- `on before|after create|commit|delete|rollback call Module.Mf($currentObject) [raise error]` events
- `grant Role1, Role2 on Module.Entity (create, delete, read *, write (Col1, Col2)) [where 'xpath']` access rules

### 3.3 View entity

```
describe entity <Module.ViewEntity>;
```

**Expected:** Output ends with `as (` followed by indented OQL query, then `)`.

### 3.4 Non-existent entity

```
describe entity Fake.Missing;
```

**Expected:** Error message (not found).

---

## 4. CREATE ENTITY

### 4.1 Minimal persistent entity

```
create persistent entity MyModule.SimpleTag (
  Name: string(50)
);
```

**Expected:** Entity created. `show entity MyModule.SimpleTag` shows 1 attribute.

### 4.2 All attribute types

```
create persistent entity MyModule.AllTypes (
  ProductId: autonumber default 1,
  Name: string(200) not null error 'Name is required',
  Code: string(50) unique error 'Code must be unique',
  Price: decimal not null error 'Price required' default 0.00,
  Stock: integer default 0,
  IsActive: boolean default true,
  ReleaseDate: date,
  CreatedAt: datetime,
  ProductImage: binary,
  Status: enumeration(MyModule.Status)
);
```

**Expected:** Entity created with 10 attributes. Types and constraints preserved in `describe`.

### 4.3 Entity with generalization

```
create persistent entity MyModule.SpecialUser extends System.User (
  Department: string(100)
);
```

**Expected:** Entity created. `show entity` shows `Extends: System.User`.

### 4.4 Non-persistent entity

```
create non-persistent entity MyModule.SearchParams (
  SearchTerm: string(200),
  IncludeInactive: boolean default false
);
```

**Expected:** Entity created. `Persistable: false`.

### 4.5 Entity with indexes

```
create persistent entity MyModule.Order (
  OrderNumber: string(50) not null unique,
  CustomerId: integer,
  OrderDate: datetime
)
index (OrderNumber),
index (CustomerId asc),
index (OrderDate desc);
```

**Expected:** Entity created. `describe` shows 3 index definitions.

### 4.6 Entity with event handlers

```
create persistent entity MyModule.BankAccount (
  IBAN: string(34) not null
)
on before commit call MyModule.ACT_ValidateIBAN($currentObject) raise error;
```

**Expected:** Entity created. `describe` shows event handler.

### 4.7 Entity with system attributes

```
create or modify persistent entity MyModule.AuditedRecord (
  Title: string(200) not null,
  Owner: autoowner,
  ChangedBy: autochangedby,
  CreatedDate: autocreateddate,
  ChangedDate: autochangeddate
);
```

**Expected:** Entity created with system attributes. `describe` shows pseudo-attributes.

### 4.8 `create or modify` — new entity

```
create or modify persistent entity MyModule.Fresh (
  Value: integer
);
```

**Expected:** Creates entity (same as without `or modify`).

### 4.9 `create or modify` — existing entity

```
create or modify persistent entity MyModule.Fresh (
  Value: integer,
  Extra: string(100)
);
```

**Expected:** Updates entity — adds `Extra` attribute, preserves entity ID.

### 4.10 Duplicate entity (without `or modify`)

```
create persistent entity MyModule.SimpleTag (
  Name: string(50)
);
```

**Expected:** Error — entity already exists.

### 4.11 View entity with OQL

```
create view entity MyModule.ActiveProducts (
  Name: string(200),
  Price: decimal
) as
  select p.Name as Name, p.Price as Price
  from MyModule.Product as p
  where p.IsActive = true;
```

**Expected:** View entity created. `describe` shows OQL source.

### 4.12 `create or replace` view entity

```
create or replace view entity MyModule.ActiveProducts (
  Name: string(200),
  Price: decimal,
  Stock: integer
) as
  select p.Name, p.Price, p.Stock
  from MyModule.Product as p
  where p.IsActive = true;
```

**Expected:** Drops and recreates view entity (new ID).

### 4.13 Position annotation

```
@position(300, 400)
create persistent entity MyModule.Positioned (
  Label: string(100)
);
```

**Expected:** Entity created at position (300, 400). `describe` shows `@Position(300, 400)`.

---

## 5. ALTER ENTITY

### 5.1 Add attribute

```
alter entity MyModule.SimpleTag add attribute Description: string(500);
```

**Expected:** Attribute added. `describe` shows both `Name` and `Description`.

### 5.2 Rename attribute

```
alter entity MyModule.SimpleTag rename attribute Name to TagName;
```

**Expected:** Attribute renamed. `describe` shows `TagName`.

### 5.3 Modify attribute type

```
alter entity MyModule.SimpleTag modify attribute TagName: string(200);
```

**Expected:** Attribute type updated to `string(200)`.

### 5.4 Drop attribute

```
alter entity MyModule.SimpleTag drop attribute Description;
```

**Expected:** Attribute removed. Validation rules, index refs, and MemberAccess entries cleaned up.

### 5.5 Set documentation

```
alter entity MyModule.SimpleTag set documentation 'A simple tag entity for testing';
```

**Expected:** `describe` shows `/** A simple tag entity for testing */`.

### 5.6 Set position

```
alter entity MyModule.SimpleTag set position (150, 250);
```

**Expected:** `describe` shows `@Position(150, 250)`.

### 5.7 Add index

```
alter entity MyModule.SimpleTag add index (TagName);
```

**Expected:** `describe` shows `index (TagName)`.

### 5.8 Drop index

```
alter entity MyModule.SimpleTag drop index idx1;
```

**Expected:** Index removed.

### 5.9 Add event handler

```
alter entity MyModule.SimpleTag
  add event handler on before commit call MyModule.ACT_ValidateTag($currentObject) raise error;
```

**Expected:** Event handler added to entity.

### 5.10 Drop event handler

```
alter entity MyModule.SimpleTag drop event handler on before commit;
```

**Expected:** Event handler removed.

### 5.11 Multiple actions

```
alter entity MyModule.SimpleTag
  add attribute Priority: integer default 0
  add index (Priority)
  set documentation 'Updated tag entity';
```

**Expected:** All three changes applied in one statement.

### 5.12 Drop system pseudo-attribute

```
alter entity MyModule.AuditedRecord drop attribute owner;
```

**Expected:** Clears `HasOwner` flag on entity. `describe` no longer shows `Owner: AutoOwner`.

### 5.13 Make attribute calculated

```
alter entity MyModule.SimpleTag modify attribute TagName: string(200) calculated by MyModule.CalcTagName;
```

**Expected:** Attribute marked as calculated. `describe` shows `calculated by MyModule.CalcTagName`.

---

## 6. DROP ENTITY

### 6.1 Drop existing entity

```
drop entity MyModule.SimpleTag;
```

**Expected:** Entity removed. `show entity MyModule.SimpleTag` returns error.

### 6.2 Drop non-existent entity

```
drop entity MyModule.NonExistent;
```

**Expected:** Error — entity not found.

### 6.3 Drop view entity

```
drop entity MyModule.ActiveProducts;
```

**Expected:** View entity and its ViewEntitySourceDocument removed.

### 6.4 Drop entity with references

```
drop entity MyModule.Order;
```

**Expected:** Warning about existing references (associations, pages, microflows). Entity dropped.

---

## 7. SHOW ASSOCIATIONS

### 7.1 List all associations

```
show associations;
```

**Expected:** Table with columns `Qualified Name | Module | Name | Parent | Child | Type | Owner | Storage`. Summary `(N associations)`.

### 7.2 List associations in a module

```
show associations in MyModule;
```

**Expected:** Only associations from `MyModule`. Same format.

### 7.3 Cross-module associations

```
show associations;
```

**Expected:** Cross-module associations listed with full qualified names for both parent and child entities.

---

## 8. SHOW ASSOCIATION (single)

### 8.1 Basic association

```
show association MyModule.Order_Customer;
```

**Expected:**
```
Association: MyModule.Order_Customer
  Type: Reference
  Owner: Default
  Storage: Column
```

### 8.2 Cross-module association

```
show association <Module.CrossModuleAssoc>;
```

**Expected:** Shows `(cross-module)` and full child entity path.

### 8.3 Non-existent association

```
show association Fake.Missing;
```

**Expected:** Error — association not found.

---

## 9. DESCRIBE ASSOCIATION

### 9.1 Reference association

```
describe association MyModule.Order_Customer;
```

**Expected:**
```
create association MyModule.Order_Customer
from MyModule.Order to MyModule.Customer
type Reference
owner Default
delete_behavior DELETE_BUT_KEEP_REFERENCES;
/
```

### 9.2 ReferenceSet association

```
describe association MyModule.Project_Employees;
```

**Expected:** `type ReferenceSet`, `owner Both`.

### 9.3 Association with documentation

Find an association with a doc comment. Verify `/** ... */` appears in output.

---

## 10. CREATE ASSOCIATION

### 10.1 Basic reference (many-to-one)

```
create association MyModule.Order_Customer
from MyModule.Order to MyModule.Customer
type reference
owner default
delete_behavior DELETE_BUT_KEEP_REFERENCES;
```

**Expected:** Association created. `describe` matches input.

### 10.2 ReferenceSet (many-to-many)

```
create association MyModule.Project_Employees
from MyModule.Project to MyModule.Employee
type referenceset
owner both;
```

**Expected:** Association created with `type ReferenceSet`, `owner Both`.

### 10.3 Self-referencing association

```
create association MyModule.Category_Parent
from MyModule.Category to MyModule.Category
type reference
owner default
delete_behavior DELETE_BUT_KEEP_REFERENCES;
```

**Expected:** Association created. Parent and child are same entity.

### 10.4 Cross-module association

```
create association MyModule.Account_User
from MyModule.Account to Administration.Account
type reference
owner default;
```

**Expected:** Association created spanning two modules.

### 10.5 With explicit storage

```
create association MyModule.Tag_Item
from MyModule.Tag to MyModule.Item
type reference
owner default
storage table;
```

**Expected:** Association uses table storage instead of column.

### 10.6 Delete behavior — CASCADE

```
create association MyModule.Parent_Child
from MyModule.Parent to MyModule.Child
type reference
owner default
delete_behavior DELETE_CASCADE;
```

**Expected:** `describe` shows `DELETE_CASCADE`.

### 10.7 Delete behavior — IF_NO_REFERENCES

```
create association MyModule.Strict_Ref
from MyModule.A to MyModule.B
type reference
owner default
delete_behavior DELETE_IF_NO_REFERENCES;
```

**Expected:** `describe` shows `DELETE_IF_NO_REFERENCES`.

### 10.8 Duplicate association

```
create association MyModule.Order_Customer
from MyModule.Order to MyModule.Customer
type reference
owner default;
```

**Expected:** Error — association already exists.

---

## 11. ALTER ASSOCIATION

### 11.1 Change delete behavior

```
alter association MyModule.Order_Customer set delete_behavior DELETE_CASCADE;
```

**Expected:** `describe` shows updated behavior.

### 11.2 Change owner

```
alter association MyModule.Order_Customer set owner both;
```

**Expected:** `describe` shows `owner Both`.

### 11.3 Change storage

```
alter association MyModule.Order_Customer set storage table;
```

**Expected:** `describe` shows `storage table`.

### 11.4 Set comment

```
alter association MyModule.Order_Customer set comment 'Links orders to customers';
```

**Expected:** `describe` shows documentation comment.

---

## 12. DROP ASSOCIATION

### 12.1 Drop existing association

```
drop association MyModule.Order_Customer;
```

**Expected:** Association removed. `show association MyModule.Order_Customer` returns error.

### 12.2 Drop non-existent association

```
drop association MyModule.Fake;
```

**Expected:** Error — not found.

### 12.3 Drop cross-module association

```
drop association MyModule.Account_User;
```

**Expected:** Cross-module association removed.

---

## 13. SHOW CONSTANTS

### 13.1 List all constants

```
show constants;
```

**Expected:** Table with columns `Qualified Name | Module | Name | Folder | Type | Default | Exposed`. Sorted alphabetically.

### 13.2 List constants in a module

```
show constants in MyModule;
```

**Expected:** Only constants from `MyModule`.

### 13.3 Empty module

```
show constants in NonExistentModule;
```

**Expected:** `No constants found.` or error.

---

## 14. SHOW CONSTANT VALUES

### 14.1 All constant values

```
show constant values;
```

**Expected:** Table with columns `Constant | Configuration | Value`. One row per constant for `(default)`, additional rows per configuration override. Values truncated to 60 chars.

### 14.2 Constants in a module

```
show constant values in MyModule;
```

**Expected:** Only constants from `MyModule`. Same format.

---

## 15. DESCRIBE CONSTANT

### 15.1 String constant

```
describe constant MyModule.ServiceEndpoint;
```

**Expected:**
```
create or modify constant MyModule.ServiceEndpoint
  type String
  default 'https://api.example.com/v1'
;
/
```

### 15.2 Boolean constant

```
describe constant MyModule.EnableDebug;
```

**Expected:** `default true` or `default false` (unquoted).

### 15.3 Constant with folder and comment

```
describe constant MyModule.MaxRetries;
```

**Expected:** Output includes `folder 'path/to/folder'` and `comment 'text'` if present.

### 15.4 Exposed constant

```
describe constant MyModule.ExposedConst;
```

**Expected:** Output includes `exposed to client`.

---

## 16. CREATE CONSTANT

### 16.1 String constant

```
create constant MyModule.ServiceEndpoint type String default 'https://api.example.com/v1';
```

**Expected:** Constant created. `describe` matches.

### 16.2 Integer constant

```
create constant MyModule.MaxRetries type Integer default 3;
```

**Expected:** Constant created with integer type and value.

### 16.3 Long constant

```
create constant MyModule.MaxFileSize type Long default 10485760 comment 'Max upload 10MB';
```

**Expected:** Constant with comment.

### 16.4 Decimal constant

```
create constant MyModule.TaxRate type Decimal default 0.21;
```

**Expected:** Decimal value preserved.

### 16.5 Boolean constant

```
create constant MyModule.EnableDebug type Boolean default false;
```

**Expected:** Boolean constant created.

### 16.6 DateTime constant

```
create constant MyModule.LaunchDate type DateTime default '[%BeginOfCurrentDay%]';
```

**Expected:** DateTime constant with token expression.

### 16.7 Enumeration constant

```
create constant MyModule.DefaultStatus type Enumeration(MyModule.Status) default 'Active';
```

**Expected:** Constant with enumeration type.

### 16.8 With folder

```
create constant MyModule.Nested type String default 'value' folder 'Config/SubFolder';
```

**Expected:** Constant placed in specified folder. Folder auto-created if missing.

### 16.9 Exposed to client

```
create constant MyModule.ClientConst type String default 'visible' exposed to client;
```

**Expected:** `describe` shows `exposed to client`.

### 16.10 `create or modify` — existing constant

```
create or modify constant MyModule.MaxRetries type Integer default 5 comment 'Updated';
```

**Expected:** Updates existing constant value and comment.

### 16.11 Duplicate constant (without `or modify`)

```
create constant MyModule.MaxRetries type Integer default 3;
```

**Expected:** Error — constant already exists.

### 16.12 With documentation comment

```
/** Maximum number of API retry attempts */
create constant MyModule.ApiRetries type Integer default 3;
```

**Expected:** Constant created with doc comment.

---

## 17. DROP CONSTANT

### 17.1 Drop existing constant

```
drop constant MyModule.ServiceEndpoint;
```

**Expected:** Constant removed.

### 17.2 Drop non-existent constant

```
drop constant MyModule.Fake;
```

**Expected:** Error — not found.

---

## 18. ROUNDTRIP (BSON)

Test that CREATE → DESCRIBE → CREATE (from output) produces identical results.

### 18.1 Entity roundtrip

```
create persistent entity RtTest.Employee (
  Name: string(200) not null,
  Salary: decimal default 0.00,
  Active: boolean default true
)
index (Name);
```

1. Run `describe entity RtTest.Employee`
2. Copy output (removing leading `create or modify` → just `create`)
3. Drop entity: `drop entity RtTest.Employee`
4. Execute copied MDL
5. Run `describe` again

**Expected:** Output identical between step 1 and step 5.

### 18.2 Association roundtrip

```
create association RtTest.Emp_Dept
from RtTest.Employee to RtTest.Department
type reference
owner default
delete_behavior DELETE_CASCADE;
```

1. `describe association RtTest.Emp_Dept`
2. Drop: `drop association RtTest.Emp_Dept`
3. Execute described MDL
4. `describe` again

**Expected:** Identical output.

### 18.3 Constant roundtrip

```
create constant RtTest.ApiKey type String default 'sk-test-123' comment 'Test key' exposed to client;
```

1. `describe constant RtTest.ApiKey`
2. Drop: `drop constant RtTest.ApiKey`
3. Execute described MDL
4. `describe` again

**Expected:** Identical output.

---

## 19. CATALOG INTEGRATION

### 19.1 Entity in catalog

```
refresh catalog;
select * from catalog_entities where name = 'Account';
```

**Expected:** Entity appears in catalog with correct module, type, attribute count.

### 19.2 Association in catalog

```
select * from catalog_associations where parent_entity = 'MyModule.Order';
```

**Expected:** Associations referencing the entity listed.

### 19.3 Callers/callees for entity

```
show callers of entity Administration.Account;
```

**Expected:** Lists microflows, pages, and other documents referencing this entity.

---

## 20. MULTI-STEP WORKFLOWS

### 20.1 Create entity → add association → grant access

```
create persistent entity MyModule.Product (
  Name: string(200) not null,
  Price: decimal default 0
);

create persistent entity MyModule.Category (
  Label: string(100)
);

create association MyModule.Product_Category
from MyModule.Product to MyModule.Category
type reference
owner default;

create module role MyModule.Manager;
grant Manager on MyModule.Product (create, delete, read *, write *);
grant Manager on MyModule.Category (read *);
```

**Expected:** All statements succeed. `show entities in MyModule` shows both entities with correct association count.

### 20.2 Create constant → modify → verify

```
create constant MyModule.Timeout type Integer default 30;
create or modify constant MyModule.Timeout type Integer default 60 comment 'Increased timeout';
describe constant MyModule.Timeout;
```

**Expected:** Final `describe` shows `default 60` and comment.

---

## 21. FAILURE MODES & ERROR RECOVERY

### 21.1 Invalid attribute type

```
create persistent entity MyModule.Bad (
  Value: invalidtype
);
```

**Expected:** Error — unknown type. No entity created.

### 21.2 Missing module qualification

```
create persistent entity UnqualifiedName (
  X: integer
);
```

**Expected:** Error — module name required.

### 21.3 Alter non-existent attribute

```
alter entity MyModule.Product drop attribute FakeAttr;
```

**Expected:** Error — attribute not found.

### 21.4 Create association with missing entity

```
create association MyModule.Bad_Ref
from MyModule.Nonexistent to MyModule.Product
type reference
owner default;
```

**Expected:** Error — parent entity not found.

### 21.5 Invalid OQL in view entity

```
create view entity MyModule.BadView (
  X: integer
) as
  select invalid syntax here;
```

**Expected:** Error — OQL validation fails. No entity created.

### 21.6 Drop entity referenced by association

```
drop entity MyModule.Customer;
```

**Expected:** Warning about references. Entity still dropped (associations may become orphaned).

---

## 22. BOUNDARY & STRESS

### 22.1 Entity with many attributes (50+)

Create an entity with 50 attributes. Verify `show` and `describe` handle large attribute lists.

### 22.2 Long attribute names

```
create persistent entity MyModule.LongNames (
  ThisIsAVeryLongAttributeNameThatExceedsNormalLength: string(200)
);
```

**Expected:** Created and displayed correctly.

### 22.3 String constant with special characters

```
create constant MyModule.Special type String default 'it''s a "test" with\nnewlines';
```

**Expected:** Escaped characters preserved in describe output.

### 22.4 Maximum string length

```
create persistent entity MyModule.MaxLen (
  BigText: string(1048576)
);
```

**Expected:** Entity created (or error if max exceeded).

### 22.5 Many associations on one entity

Create 10+ associations from one entity to different targets. Verify `show associations` and entity describe handle them.

---

## Test Project Coverage Matrix

| Operation | Lato Enquiry | Evora Factory | Lato Product |
|-----------|:---:|:---:|:---:|
| SHOW ENTITIES | x | x | x |
| SHOW ENTITY | x | x | x |
| DESCRIBE ENTITY | x | x | x |
| CREATE ENTITY | x | | |
| ALTER ENTITY | x | | |
| DROP ENTITY | x | | |
| SHOW ASSOCIATIONS | x | x | x |
| DESCRIBE ASSOCIATION | x | x | |
| CREATE ASSOCIATION | x | | |
| ALTER ASSOCIATION | x | | |
| DROP ASSOCIATION | x | | |
| SHOW CONSTANTS | x | x | x |
| SHOW CONSTANT VALUES | x | x | |
| DESCRIBE CONSTANT | x | x | |
| CREATE CONSTANT | x | | |
| DROP CONSTANT | x | | |

Read operations tested on all projects. Write operations on copies of one project.

---

## Automated Test Coverage

| Section | Automated | Manual-only |
|---------|:---------:|:-----------:|
| 1. SHOW ENTITIES | Mock tests | |
| 2. SHOW ENTITY | Mock tests | |
| 3. DESCRIBE ENTITY | Mock tests | |
| 4. CREATE ENTITY | Mock + roundtrip | |
| 5. ALTER ENTITY | Mock tests | Multi-action |
| 6. DROP ENTITY | Mock tests | |
| 7–9. SHOW/DESCRIBE ASSOCIATION | Mock tests | |
| 10. CREATE ASSOCIATION | Mock tests | Cross-module |
| 11. ALTER ASSOCIATION | Mock tests | |
| 12. DROP ASSOCIATION | Mock tests | |
| 13–14. SHOW CONSTANTS | Mock tests | |
| 15. DESCRIBE CONSTANT | Mock tests | |
| 16. CREATE CONSTANT | Mock tests | |
| 17. DROP CONSTANT | Mock tests | |
| 18. Roundtrip | Roundtrip tests | Complex entities |
| 19. Catalog | | All manual |
| 20. Multi-step | | All manual |
| 21. Failure modes | Partial | Edge cases |
| 22. Boundary | | All manual |

---

## Manual Test Report Template

**Tester:** _______________
**Date:** _______________
**Project:** _______________

| # | Section | Test | Pass | Fail | Skip | Notes |
|---|---------|------|:----:|:----:|:----:|-------|
| 1.1 | SHOW ENTITIES | List all | | | | |
| 1.2 | SHOW ENTITIES | Filter by module | | | | |
| 1.3 | SHOW ENTITIES | Generalization column | | | | |
| 1.4 | SHOW ENTITIES | Empty module | | | | |
| 1.5 | SHOW ENTITIES | Entity types | | | | |
| 2.1 | SHOW ENTITY | Persistent | | | | |
| 2.2 | SHOW ENTITY | Non-persistent | | | | |
| 2.3 | SHOW ENTITY | Not found | | | | |
| 3.1 | DESCRIBE | Basic | | | | |
| 3.2 | DESCRIBE | All features | | | | |
| 3.3 | DESCRIBE | View entity | | | | |
| 3.4 | DESCRIBE | Not found | | | | |
| 4.1 | CREATE | Minimal persistent | | | | |
| 4.2 | CREATE | All types | | | | |
| 4.3 | CREATE | Generalization | | | | |
| 4.4 | CREATE | Non-persistent | | | | |
| 4.5 | CREATE | With indexes | | | | |
| 4.6 | CREATE | Event handlers | | | | |
| 4.7 | CREATE | System attributes | | | | |
| 4.8 | CREATE | create or modify (new) | | | | |
| 4.9 | CREATE | create or modify (update) | | | | |
| 4.10 | CREATE | Duplicate error | | | | |
| 4.11 | CREATE | View entity | | | | |
| 4.12 | CREATE | create or replace view | | | | |
| 4.13 | CREATE | Position annotation | | | | |
| 5.1 | ALTER | Add attribute | | | | |
| 5.2 | ALTER | Rename attribute | | | | |
| 5.3 | ALTER | Modify type | | | | |
| 5.4 | ALTER | Drop attribute | | | | |
| 5.5 | ALTER | Set documentation | | | | |
| 5.6 | ALTER | Set position | | | | |
| 5.7 | ALTER | Add index | | | | |
| 5.8 | ALTER | Drop index | | | | |
| 5.9 | ALTER | Add event handler | | | | |
| 5.10 | ALTER | Drop event handler | | | | |
| 5.11 | ALTER | Multiple actions | | | | |
| 5.12 | ALTER | Drop pseudo-attribute | | | | |
| 5.13 | ALTER | Make calculated | | | | |
| 6.1 | DROP ENTITY | Existing | | | | |
| 6.2 | DROP ENTITY | Non-existent | | | | |
| 6.3 | DROP ENTITY | View entity | | | | |
| 6.4 | DROP ENTITY | With references | | | | |
| 7.1 | SHOW ASSOC | List all | | | | |
| 7.2 | SHOW ASSOC | Filter module | | | | |
| 7.3 | SHOW ASSOC | Cross-module | | | | |
| 8.1 | SHOW ASSOC | Single basic | | | | |
| 8.2 | SHOW ASSOC | Single cross-module | | | | |
| 8.3 | SHOW ASSOC | Not found | | | | |
| 9.1 | DESCRIBE ASSOC | Reference | | | | |
| 9.2 | DESCRIBE ASSOC | ReferenceSet | | | | |
| 9.3 | DESCRIBE ASSOC | With documentation | | | | |
| 10.1 | CREATE ASSOC | Basic reference | | | | |
| 10.2 | CREATE ASSOC | ReferenceSet | | | | |
| 10.3 | CREATE ASSOC | Self-referencing | | | | |
| 10.4 | CREATE ASSOC | Cross-module | | | | |
| 10.5 | CREATE ASSOC | Explicit storage | | | | |
| 10.6 | CREATE ASSOC | DELETE_CASCADE | | | | |
| 10.7 | CREATE ASSOC | DELETE_IF_NO_REFERENCES | | | | |
| 10.8 | CREATE ASSOC | Duplicate error | | | | |
| 11.1 | ALTER ASSOC | Delete behavior | | | | |
| 11.2 | ALTER ASSOC | Change owner | | | | |
| 11.3 | ALTER ASSOC | Change storage | | | | |
| 11.4 | ALTER ASSOC | Set comment | | | | |
| 12.1 | DROP ASSOC | Existing | | | | |
| 12.2 | DROP ASSOC | Non-existent | | | | |
| 12.3 | DROP ASSOC | Cross-module | | | | |
| 13.1 | SHOW CONST | List all | | | | |
| 13.2 | SHOW CONST | Filter module | | | | |
| 13.3 | SHOW CONST | Empty module | | | | |
| 14.1 | CONST VALUES | All values | | | | |
| 14.2 | CONST VALUES | Filter module | | | | |
| 15.1 | DESCRIBE CONST | String | | | | |
| 15.2 | DESCRIBE CONST | Boolean | | | | |
| 15.3 | DESCRIBE CONST | With folder/comment | | | | |
| 15.4 | DESCRIBE CONST | Exposed | | | | |
| 16.1 | CREATE CONST | String | | | | |
| 16.2 | CREATE CONST | Integer | | | | |
| 16.3 | CREATE CONST | Long | | | | |
| 16.4 | CREATE CONST | Decimal | | | | |
| 16.5 | CREATE CONST | Boolean | | | | |
| 16.6 | CREATE CONST | DateTime | | | | |
| 16.7 | CREATE CONST | Enumeration | | | | |
| 16.8 | CREATE CONST | With folder | | | | |
| 16.9 | CREATE CONST | Exposed | | | | |
| 16.10 | CREATE CONST | create or modify | | | | |
| 16.11 | CREATE CONST | Duplicate error | | | | |
| 16.12 | CREATE CONST | Doc comment | | | | |
| 17.1 | DROP CONST | Existing | | | | |
| 17.2 | DROP CONST | Non-existent | | | | |
| 18.1 | ROUNDTRIP | Entity | | | | |
| 18.2 | ROUNDTRIP | Association | | | | |
| 18.3 | ROUNDTRIP | Constant | | | | |
| 19.1 | CATALOG | Entity in catalog | | | | |
| 19.2 | CATALOG | Association in catalog | | | | |
| 19.3 | CATALOG | Callers | | | | |
| 20.1 | MULTI-STEP | Entity + assoc + access | | | | |
| 20.2 | MULTI-STEP | Constant modify chain | | | | |
| 21.1 | FAILURE | Invalid type | | | | |
| 21.2 | FAILURE | Missing module | | | | |
| 21.3 | FAILURE | Alter non-existent attr | | | | |
| 21.4 | FAILURE | Missing entity in assoc | | | | |
| 21.5 | FAILURE | Invalid OQL | | | | |
| 21.6 | FAILURE | Drop referenced entity | | | | |
| 22.1 | BOUNDARY | Many attributes | | | | |
| 22.2 | BOUNDARY | Long names | | | | |
| 22.3 | BOUNDARY | Special characters | | | | |
| 22.4 | BOUNDARY | Max string length | | | | |
| 22.5 | BOUNDARY | Many associations | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
