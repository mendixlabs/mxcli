# REST Call from JSON Payload — End-to-End Skill

Use this skill to generate the full stack of Mendix integration artifacts from a JSON payload:
JSON Structure → Non-persistent entities → Import Mapping → REST CALL microflow.

## Overview — Four Steps

1. **CREATE JSON STRUCTURE** — store the raw payload and derive the element tree
2. **CREATE ENTITY** (non-persistent) — one per JSON object type, with attributes per JSON field
3. **CREATE IMPORT MAPPING** — link JSON structure elements to entities and attributes
4. **CREATE MICROFLOW** — inline REST CALL that invokes the import mapping

---

## Step 1 — JSON Structure

```sql
CREATE JSON STRUCTURE Module.JSON_MyStructure
  FROM '{"key": "value", "count": 1}';
```

- The executor **formats** the snippet (pretty-print) then **refreshes** (derives element tree) automatically.
- The snippet must be valid JSON; use single quotes around it in MDL.
- Escape single quotes inside the snippet by doubling them: `''`.
- The derived element tree must stay consistent with the snippet — the executor sorts JSON object keys alphabetically to match `json.MarshalIndent` output.

**Verify** after creation:
```sql
DESCRIBE JSON STRUCTURE Module.JSON_MyStructure;
-- Should show: element tree under "-- Element tree:" comment
```

---

## Step 2 — Non-Persistent Entities

Derive one entity per JSON object type. Name them after what they represent (not after JSON keys).

```sql
CREATE ENTITY Module.MyRootObject (NON_PERSISTENT)
  stringField   : String
  intField      : Integer
  decimalField  : Decimal
  boolField     : Boolean DEFAULT false;

CREATE ENTITY Module.MyNestedObject (NON_PERSISTENT)
  name : String
  code : String;

CREATE ASSOCIATION Module.MyRootObject_MyNestedObject
  FROM Module.MyRootObject
  TO Module.MyNestedObject;
```

**Rules:**
- All string fields: bare `String` (no length — unlimited)
- All number fields: `Integer`, `Decimal`, or `Long` — remove defaults for optional fields
- Boolean fields **require** `DEFAULT true|false`
- `NON_PERSISTENT` — these entities are not stored in the database
- One association per parent→child relationship; name it `Parent_Child`

---

## Step 3 — Import Mapping

```sql
CREATE IMPORT MAPPING Module.IMM_MyMapping
  FROM JSON STRUCTURE Module.JSON_MyStructure
{
  "" -> Module.MyRootObject (Create) {
    nestedKey -> Module.MyNestedObject (Create) VIA Module.MyRootObject_MyNestedObject {
      name -> name (String)
      code -> code (String)
    }
    stringField -> stringField (String)
    intField    -> intField    (Integer)
  }
};
```

**Syntax rules:**
- Root element uses `""` (empty string) as the JSON key — it maps the top-level object
- Object mappings: `jsonKey -> Module.Entity (Create|Find|FindOrCreate)`
- Value mappings: `jsonKey -> attributeName (String|Integer|Long|Decimal|Boolean|DateTime)`
- `VIA Module.Association` — required when mapping a nested object reachable via an association
- No semicolons between child elements inside `{}`

**Verify** after creation — check Schema elements are ticked in Studio Pro:
- Open the import mapping in Studio Pro
- All JSON structure elements should appear ticked in the Schema elements panel
- If not ticked: JsonPath mismatch between import mapping and JSON structure elements

---

## Step 4 — REST CALL Microflow

Place the microflow in the `[Pages]/Operations/` folder or `Private/` depending on whether it is public.

```sql
CREATE MICROFLOW Module.GET_MyData ()
BEGIN
  @position(-5, 200)
  DECLARE $baseUrl String = 'https://api.example.com';
  @position(185, 200)
  DECLARE $endpoint String = $baseUrl + '/path';
  @position(375, 200)
  $Result = REST CALL GET '{1}' WITH ({1} = $endpoint)
    HEADER 'Accept' = 'application/json'
    TIMEOUT 300
    RETURNS MAPPING Module.IMM_MyMapping AS Module.MyRootObject ON ERROR ROLLBACK;
  @position(565, 200)
  LOG INFO NODE 'Integration' 'Retrieved result' WITH ();
END;
/
```

**Key points:**
- `@position` annotations control the canvas layout — StartEvent is auto-placed 150px to the left of the first annotated activity
- The output variable name is **automatically derived** from the entity name in `AS Module.MyEntity` — do NOT hardcode it on the left side; the executor overrides it
- Single vs list result is **automatically detected**: if the JSON structure's root element is an Object, the variable type is `ObjectType` (single); if Array, `ListType` (list)
- `ON ERROR ROLLBACK` — standard error handling for integration calls

**For list responses** (JSON root is an array):
```sql
  $Results = REST CALL GET '{1}' WITH ({1} = $endpoint)
    HEADER 'Accept' = 'application/json'
    TIMEOUT 300
    RETURNS MAPPING Module.IMM_MyMapping AS Module.MyItem ON ERROR ROLLBACK;
  @position(565, 200)
  $Count = COUNT($MyItem);
```

---

## Complete Example — Bible Verse API

```sql
-- Step 1: JSON Structure
CREATE JSON STRUCTURE Integrations.JSON_BibleVerse
  FROM '{"translation":{"identifier":"web","name":"World English Bible","language":"English","language_code":"eng","license":"Public Domain"},"random_verse":{"book_id":"1SA","book":"1 Samuel","chapter":17,"verse":49,"text":"David put his hand in his bag, took a stone, and slung it."}}';

-- Step 2: Entities
CREATE ENTITY Integrations.BibleApiResponse (NON_PERSISTENT);

CREATE ENTITY Integrations.BibleTranslation (NON_PERSISTENT)
  identifier    : String
  name          : String
  language      : String
  language_code : String
  license       : String;

CREATE ENTITY Integrations.BibleVerse (NON_PERSISTENT)
  book_id : String
  book    : String
  chapter : Integer
  verse   : Integer
  text    : String;

CREATE ASSOCIATION Integrations.BibleApiResponse_BibleTranslation
  FROM Integrations.BibleApiResponse
  TO Integrations.BibleTranslation;

CREATE ASSOCIATION Integrations.BibleApiResponse_BibleVerse
  FROM Integrations.BibleApiResponse
  TO Integrations.BibleVerse;

-- Step 3: Import Mapping
CREATE IMPORT MAPPING Integrations.IMM_BibleVerse
  FROM JSON STRUCTURE Integrations.JSON_BibleVerse
{
  "" -> Integrations.BibleApiResponse (Create) {
    translation -> Integrations.BibleTranslation (Create) VIA Integrations.BibleApiResponse_BibleTranslation {
      identifier    -> identifier    (String)
      language      -> language      (String)
      language_code -> language_code (String)
      license       -> license       (String)
      name          -> name          (String)
    }
    random_verse -> Integrations.BibleVerse (Create) VIA Integrations.BibleApiResponse_BibleVerse {
      book    -> book    (String)
      book_id -> book_id (String)
      chapter -> chapter (Integer)
      text    -> text    (String)
      verse   -> verse   (Integer)
    }
  }
};

-- Step 4: Microflow
CREATE MICROFLOW Integrations.GET_BibleVerse_Random ()
BEGIN
  @position(-5, 200)
  DECLARE $baseUrl String = 'https://bible-api.com';
  @position(185, 200)
  DECLARE $endpoint String = $baseUrl + '/data/web/random';
  @position(375, 200)
  $Result = REST CALL GET '{1}' WITH ({1} = $endpoint)
    HEADER 'Accept' = 'application/json'
    TIMEOUT 300
    RETURNS MAPPING Integrations.IMM_BibleVerse AS Integrations.BibleApiResponse ON ERROR ROLLBACK;
  @position(565, 200)
  LOG INFO NODE 'Integration' 'Retrieved Bible verse' WITH ();
END;
/
```

---

## Gotchas and Common Errors

| Symptom | Cause | Fix |
|---------|-------|-----|
| Studio Pro "not consistent with snippet" | JSON element tree keys not in alphabetical order | Executor sorts keys; re-derive from snippet |
| Schema elements not ticked in import mapping | JsonPath mismatch | Named object elements use `(Object)\|key`, NOT `(Object)\|key\|(Object)` |
| Import mapping not linked in REST call | Wrong BSON field name | Use `ReturnValueMapping`, not `Mapping` |
| Studio Pro shows "List of X" but mapping returns single X | `ForceSingleOccurrence` not set | Executor auto-detects from JSON structure root element type |
| StartEvent behind first activities | Default posX=200 vs @position(-5,...) | Fixed: executor pre-scans for first @position and shifts StartEvent left |
| `TypeCacheUnknownTypeException` | Wrong BSON `$Type` names | `ImportMappings$ObjectMappingElement` / `ImportMappings$ValueMappingElement` (no `Import` prefix) |
| Attribute not found in Studio Pro | Attribute not fully qualified | Must be `Module.Entity.AttributeName` in the BSON |

---

## Naming Conventions (MES)

| Artifact | Pattern | Example |
|----------|---------|---------|
| JSON Structure | `JSON_<ApiName>` | `JSON_BibleVerse` |
| Import Mapping | `IMM_<ApiName>` | `IMM_BibleVerse` |
| Root entity | Describes the API response | `BibleApiResponse` |
| Nested entities | Describes the domain concept | `BibleVerse`, `BibleTranslation` |
| Microflow | `METHOD_Resource_Operation` | `GET_BibleVerse_Random` |
| Folder | `Private/` for mappings/structures, `Operations/` for public microflows | — |
