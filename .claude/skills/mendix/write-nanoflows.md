# Mendix Nanoflow Skill

This skill provides guidance for writing Mendix nanoflows in MDL syntax. Nanoflows share syntax with microflows but execute client-side with restricted capabilities.

## When to Use This Skill

Use this skill when:
- Writing CREATE NANOFLOW statements
- Debugging nanoflow validation errors
- Understanding nanoflow restrictions vs microflows

## Key Differences from Microflows

| Aspect | Microflow | Nanoflow |
|--------|-----------|----------|
| **Execution** | Server-side | Client-side (browser/mobile) |
| **Database access** | Full | No direct access |
| **Transactions** | Supported | Not supported |
| **Java actions** | Supported | Not supported |
| **JavaScript actions** | Not supported | Supported |
| **File downloads** | Supported | Not supported |
| **Error handling** | Full `ON ERROR` blocks | Per-action `ON ERROR` supported; `ErrorEvent` (raise error) forbidden |
| **Offline** | Not available | Available |

## Nanoflow Structure

```mdl
/**
 * Nanoflow description
 *
 * @param $Parameter1 Description
 * @returns Description of return value
 */
CREATE [OR MODIFY] NANOFLOW Module.NAV_Name (
  $Parameter1: type
)
RETURNS ReturnType AS $Result
FOLDER 'FolderPath'
BEGIN
  -- Nanoflow logic here
  RETURN $Result;
END;
```

## Naming Convention

Nanoflow names use the `NAV_` prefix by convention:
- `NAV_ValidateCart` — client-side validation
- `NAV_ShowDetails` — page navigation
- `NAV_ToggleFilter` — UI state toggle

## Supported Activities

### Object Operations (in-memory only)
```mdl
$Item = CREATE Sales.CartItem (Quantity = 1);
CHANGE $Item (Quantity = $Item/Quantity + 1);
```

### Calling Other Flows
```mdl
$Result = CALL NANOFLOW Sales.NAV_ValidateCart (Cart = $Cart);
$ServerResult = CALL MICROFLOW Sales.ACT_SubmitOrder (Order = $Order);
$JsResult = CALL JAVASCRIPT ACTION MyModule.MyJsAction (Param = $Value);
```

### UI Activities
```mdl
SHOW PAGE Sales.CartDetail ($Cart = $Cart);
CLOSE PAGE;
VALIDATION FEEDBACK $Item/Quantity MESSAGE 'Quantity must be at least 1';
```

### Logging and Variables
```mdl
LOG INFO 'Cart updated with ' + toString($ItemCount) + ' items';
DECLARE $IsValid Boolean = true;
SET $IsValid = false;
```

### Control Flow
```mdl
IF $Cart/ItemCount = 0 THEN
  VALIDATION FEEDBACK $Cart/ItemCount MESSAGE 'Cart is empty';
  RETURN false;
ELSE
  SHOW PAGE Sales.Checkout ($Cart = $Cart);
  RETURN true;
END IF;
```

## Disallowed Activities

These will produce validation errors (12 case branches covering 22 action types in `nanoflow_validation.go`):
- `ErrorEvent` / `RAISE ERROR` — not available in nanoflows
- `CALL JAVA ACTION` — Java actions cannot run client-side
- `EXECUTE DATABASE QUERY` — direct SQL requires server
- `CALL EXTERNAL ACTION` — external actions are server-side
- `SHOW HOME PAGE` — home page navigation is server-side
- `CALL REST SERVICE` / `SEND REST REQUEST` — REST calls are server-side
- `IMPORT FROM MAPPING` / `EXPORT TO MAPPING` — mapping operations are server-side
- `TRANSFORM JSON` — JSON transformations are server-side
- `DOWNLOAD FILE` — file downloads require server-side processing
- All **workflow actions** (11 types: CallWorkflow, OpenWorkflow, SetTaskOutcome, OpenUserTask, etc.)

**Note:** Object operations (CREATE, CHANGE, COMMIT, DELETE, RETRIEVE) ARE allowed in nanoflows — they operate in-memory on the client.

## Return Type Restrictions

Binary return type is NOT allowed in nanoflows.

## Security (GRANT/REVOKE)

```mdl
GRANT EXECUTE ON NANOFLOW Shop.NAV_Filter TO Shop.User, Shop.Admin;
REVOKE EXECUTE ON NANOFLOW Shop.NAV_Filter FROM Shop.User;
```

## Management Commands

```mdl
SHOW NANOFLOWS
SHOW NANOFLOWS IN MyModule
DESCRIBE NANOFLOW MyModule.NAV_ShowDetails
DROP NANOFLOW MyModule.NAV_ShowDetails;
MOVE NANOFLOW Sales.NAV_OpenCart TO FOLDER 'UI/Navigation';
```

## Common Mistakes

1. **Using Java actions** — Use CALL JAVASCRIPT ACTION instead.
2. **Using ErrorEvent** — Nanoflows cannot raise errors directly. Handle errors per-action with ON ERROR.
3. **Expecting transactions** — Nanoflows have no rollback. Design for idempotency.
4. **File operations** — DOWNLOAD FILE is server-only.
5. **Binary return types** — Not supported in nanoflows.
6. **REST/external calls** — REST calls and external actions are server-only.

## Validation Checklist

- [ ] No ErrorEvent / raise error
- [ ] No Java action calls
- [ ] No REST calls, external action calls, or database queries
- [ ] No file download operations
- [ ] No import/export mapping or JSON transformation
- [ ] No workflow actions
- [ ] No show home page
- [ ] No binary return type
- [ ] Parameters and return types are nanoflow-compatible
- [ ] JavaDoc documentation present
- [ ] NAV_ naming prefix used
