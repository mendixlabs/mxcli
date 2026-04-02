# REST Client Skill

Use this skill when creating, modifying, or calling Consumed REST Clients in MDL.

## When to Use

- Creating consumed REST service definitions (`CREATE REST CLIENT`)
- Calling REST service operations from microflows (`SEND REST REQUEST`)
- Listing or inspecting REST clients (`SHOW REST CLIENTS`, `DESCRIBE REST CLIENT`)

## JSON Structures

Define JSON structures from a sample snippet before creating import/export mappings.

```sql
CREATE JSON STRUCTURE Module.Name SNIPPET '{"id": 1, "name": "John"}';

-- Multi-line (use $$ quoting)
CREATE JSON STRUCTURE Module.Name SNIPPET $${ "id": 1, "items": [{"name": "A"}] }$$;

-- With documentation
CREATE JSON STRUCTURE Module.Name COMMENT 'API response' SNIPPET '...';

-- Custom name mapping for non-English keys
CREATE JSON STRUCTURE Module.Name SNIPPET $${"kvkNummer": "123"}$$
CUSTOM NAME MAP ('kvkNummer' AS 'ChamberOfCommerceNumber');

-- Idempotent update
CREATE OR REPLACE JSON STRUCTURE Module.Name SNIPPET '...';

-- Browse and delete
SHOW JSON STRUCTURES;
DESCRIBE JSON STRUCTURE Module.Name;
DROP JSON STRUCTURE Module.Name;
```

Type inference: ISO 8601 strings → DateTime, integers → Integer, decimals → Decimal, booleans → Boolean. Snippet is auto-formatted when stored.

## CREATE REST CLIENT

```sql
CREATE REST CLIENT Module.ClientName
BASE URL 'https://api.example.com'
AUTHENTICATION NONE
BEGIN
  OPERATION GetData
    METHOD GET
    PATH '/data'
    HEADER 'Accept' = 'application/json'
    RESPONSE NONE;
END;
```

### Authentication

```sql
-- No authentication
AUTHENTICATION NONE

-- Basic auth with literal credentials
AUTHENTICATION BASIC (USERNAME = 'user', PASSWORD = 'secret')
```

**IMPORTANT**: `$Variable` references in authentication (e.g., `USERNAME = $MyConstant`) are serialized as `Rest$ConstantValue` and require the constant to exist in the project. Use literal strings in examples to avoid CE7073.

### Operations

Each operation defines one HTTP endpoint call:

```sql
OPERATION OperationName
    METHOD GET|POST|PUT|PATCH|DELETE
    PATH '/path/{paramName}'
    [PARAMETER $paramName: Type]       -- Path parameters
    [QUERY $queryParam: Type]          -- Query parameters
    [HEADER 'Name' = 'value']          -- HTTP headers
    [BODY JSON FROM $Variable]         -- Request body (POST/PUT/PATCH)
    [BODY FILE FROM $Variable]         -- File upload
    [TIMEOUT seconds]                  -- Default: 300
    RESPONSE NONE|JSON AS $var|STRING AS $var|FILE AS $var|STATUS AS $var;
```

### Mendix Validation Rules (mx check)

Follow these rules to avoid errors from `mx check`:

| Rule | Error | Fix |
|------|-------|-----|
| Every operation MUST have an Accept header | CE7062 | Auto-added by serializer (`Accept: */*`) if missing |
| POST/PUT/PATCH MUST have a body | CE7064 | Auto-added by serializer (empty JSON body) |
| Dynamic header values (`'Bearer ' + $Token`) are NOT supported | CE7056 | Use static literal values only |
| Auth with `$Variable` requires the constant to exist | CE7073 | Use literal strings or create constants first |
| RESPONSE JSON requires entity mapping for full functionality | CE0061 | Configure entity mapping in Studio Pro after creation |

### Response Handling

The serializer uses `Rest$NoResponseHandling` with ContentType for all response types to avoid CE0061 (entity mapping requirement). This means:

- **RESPONSE NONE** → `Rest$NoResponseHandling` (no ContentType)
- **RESPONSE JSON** → `Rest$NoResponseHandling` with `ContentType: "application/json"`
- **RESPONSE STRING** → `Rest$NoResponseHandling` with `ContentType: "text/plain"`
- **RESPONSE FILE** → `Rest$NoResponseHandling` with `ContentType: "application/octet-stream"`

For full JSON-to-entity mapping (`Rest$ImplicitMappingResponseHandling`), configure the response mapping in Studio Pro. This requires:
1. A non-persistent entity to hold the response data
2. An `ImportMappings$ObjectMappingElement` mapping JSON fields to entity attributes

## SEND REST REQUEST (Microflow Activity)

Calls a consumed REST service operation from a microflow. This creates a `Microflows$RestOperationCallAction` in the BSON.

```sql
-- Fire and forget (RESPONSE NONE operation)
SEND REST REQUEST Module.ServiceName.OperationName;

-- With output variable (RESPONSE JSON — maps to entity)
$Result = SEND REST REQUEST Module.ServiceName.OperationName;

-- With request body (POST/PUT operations)
$Result = SEND REST REQUEST Module.ServiceName.CreateItem
    BODY $NewItem;
```

### CRITICAL: `$latestHttpResponse` System Variable

After every `SEND REST REQUEST`, Mendix automatically populates `$latestHttpResponse` (type `System.HttpResponse`). **Always use this to check call success**:

```sql
-- ✅ CORRECT: check $latestHttpResponse
$RootResult = SEND REST REQUEST Module.Service.GetData;
IF $latestHttpResponse/Content != empty THEN
  -- Process $RootResult
END IF;

-- ❌ WRONG: causes CE0117 (expression error)
IF $RootResult != empty THEN
```

Key attributes on `$latestHttpResponse`:
- `Content` (String) — response body
- `StatusCode` (Integer) — HTTP status code (200, 404, etc.)

### Restrictions

- **No custom error handling** — `ON ERROR CONTINUE` or `ON ERROR ROLLBACK` causes CE6035. The action always uses abort-on-error semantics.
- Operation reference uses three-part qualified name: `Module.ServiceDocument.OperationName`

## REST CALL (Inline HTTP — Different Feature)

`REST CALL` is a separate feature for direct HTTP calls without a REST client document. URL, headers, auth, and body are specified inline in the microflow. See the write-microflows skill for full syntax.

Do NOT mix up:
- `SEND REST REQUEST` → calls consumed REST service operation (`Microflows$RestOperationCallAction`)
- `REST CALL` → inline HTTP call (`Microflows$RestCallAction`)

## Show / Describe

```sql
-- List all consumed REST clients
SHOW REST CLIENTS;
SHOW REST CLIENTS IN ModuleName;

-- Show client definition as MDL
DESCRIBE REST CLIENT Module.ClientName;

-- Delete a client
DROP REST CLIENT Module.ClientName;

-- Create or overwrite existing
CREATE OR MODIFY REST CLIENT Module.ClientName ...
```

## Complete Example

```sql
-- 1. Create the REST client with operations
CREATE REST CLIENT MyModule.PetStoreAPI
BASE URL 'https://petstore.swagger.io/v2'
AUTHENTICATION NONE
BEGIN
  OPERATION ListPets
    METHOD GET
    PATH '/pet/findByStatus'
    QUERY $status: String
    HEADER 'Accept' = 'application/json'
    TIMEOUT 30
    RESPONSE JSON AS $PetList;

  OPERATION GetPet
    METHOD GET
    PATH '/pet/{petId}'
    PARAMETER $petId: Integer
    HEADER 'Accept' = 'application/json'
    RESPONSE JSON AS $Pet;

  OPERATION AddPet
    METHOD POST
    PATH '/pet'
    HEADER 'Content-Type' = 'application/json'
    HEADER 'Accept' = 'application/json'
    BODY JSON FROM $NewPet
    RESPONSE JSON AS $CreatedPet;

  OPERATION RemovePet
    METHOD DELETE
    PATH '/pet/{petId}'
    PARAMETER $petId: Integer
    RESPONSE NONE;
END;

-- 2. Call an operation from a microflow
CREATE MICROFLOW MyModule.ACT_TestPetStore ()
RETURNS Boolean AS $Success
BEGIN
  DECLARE $Success Boolean = false;

  SEND REST REQUEST MyModule.PetStoreAPI.RemovePet;

  IF $latestHttpResponse/StatusCode = 200 THEN
    SET $Success = true;
  END IF;

  RETURN $Success;
END;
```

## BSON Types Reference

| MDL Concept | BSON Type |
|-------------|-----------|
| REST client document | `Rest$ConsumedRestService` |
| Operation | `Rest$RestOperation` |
| GET/DELETE method | `Rest$RestOperationMethodWithoutBody` |
| POST/PUT/PATCH method | `Rest$RestOperationMethodWithBody` |
| JSON body | `Rest$JsonBody` |
| File body | `Rest$StringBody` |
| No response | `Rest$NoResponseHandling` |
| JSON response (with entity mapping) | `Rest$ImplicitMappingResponseHandling` |
| Header | `Rest$HeaderWithValueTemplate` |
| Path parameter | `Rest$OperationParameter` |
| Query parameter | `Rest$QueryParameter` |
| No auth | `null` (AuthenticationScheme field) |
| Basic auth | `Rest$BasicAuthenticationScheme` |
| Microflow action (send request) | `Microflows$RestOperationCallAction` |
| Microflow action (inline HTTP) | `Microflows$RestCallAction` |
