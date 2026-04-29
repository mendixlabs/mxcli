# Nanoflows vs Microflows

Nanoflows are client-side logic flows that execute in the user's browser or on mobile devices. They share the same MDL syntax as microflows but have a different set of capabilities and restrictions.

## Key Differences

| Aspect | Microflow | Nanoflow |
|--------|-----------|----------|
| **Execution** | Server-side (Mendix runtime) | Client-side (browser/mobile) |
| **Database access** | Full (retrieve, commit, delete) | No direct database access |
| **Transactions** | Supported (with rollback) | Not supported |
| **Java actions** | Supported | Not supported |
| **JavaScript actions** | Not supported | Supported |
| **Show page** | Supported | Supported |
| **Close page** | Supported | Supported |
| **Network** | Requires server round-trip | No network call (fast) |
| **Offline** | Not available offline | Available offline |
| **Error handling** | `ON ERROR` blocks | Per-action `ON ERROR` (no `ErrorEvent`) |

## When to Use Which

**Use a microflow when you need to:**
- Retrieve data from or commit data to the database
- Call external web services or REST APIs
- Execute Java actions
- Perform batch operations on large data sets
- Use transactions with rollback support

**Use a nanoflow when you need to:**
- Respond quickly to user actions without server delay
- Perform client-side validation
- Toggle UI state (show/hide elements)
- Navigate between pages
- Work offline on mobile devices

## CREATE NANOFLOW Syntax

```sql
CREATE [OR MODIFY] NANOFLOW <Module.Name>
  [FOLDER '<path>']
BEGIN
  [<declarations>]
  [<activities>]
  [RETURN <value>;]
END;
```

The syntax is identical to `CREATE MICROFLOW` except for the keyword.

## Supported Activities in Nanoflows

### Object Operations

```sql
-- Create an object (in memory only)
$Item = CREATE Sales.CartItem (
  Quantity = 1,
  ProductName = $Product/Name
);

-- Change an object in memory
CHANGE $Item (Quantity = $Item/Quantity + 1);
```

### Calling Other Flows

```sql
-- Call another nanoflow
$Result = CALL NANOFLOW Sales.NAV_ValidateCart (Cart = $Cart);

-- Call a microflow (triggers server round-trip)
$ServerResult = CALL MICROFLOW Sales.ACT_SubmitOrder (Order = $Order);

-- Call a JavaScript action
$HasNetwork = CALL JAVASCRIPT ACTION NanoflowCommons.HasConnectivity();
```

### UI Activities

```sql
-- Show a page
SHOW PAGE Sales.CartDetail ($Cart = $Cart);

-- Close the current page
CLOSE PAGE;
```

### Validation

```sql
VALIDATION FEEDBACK $Item/Quantity MESSAGE 'Quantity must be at least 1';
```

### Logging

```sql
LOG INFO 'Cart updated with ' + toString($ItemCount) + ' items';
```

### Control Flow

```sql
IF $Cart/ItemCount = 0 THEN
  VALIDATION FEEDBACK $Cart/ItemCount MESSAGE 'Cart is empty';
  RETURN false;
ELSE
  SHOW PAGE Sales.Checkout ($Cart = $Cart);
  RETURN true;
END IF;
```

## Activities NOT Available in Nanoflows

The following activities are server-only and cannot be used in nanoflows:

- `CALL JAVA ACTION` — Java actions cannot run client-side
- `ErrorEvent` / `RAISE ERROR` — error events are not available in nanoflows
- `DOWNLOAD FILE` — file downloads require server-side processing
- `CALL REST SERVICE` / `SEND REST REQUEST` — REST calls are server-side
- `IMPORT FROM MAPPING` / `EXPORT TO MAPPING` — mapping operations are server-side
- `EXECUTE DATABASE QUERY` — direct SQL requires server
- `TRANSFORM JSON` — JSON transformations are server-side
- `SHOW HOME PAGE` — home page navigation is server-side
- `CALL EXTERNAL ACTION` — external actions are server-side
- All **workflow actions** (call/open workflow, set task outcome, user task, etc.)

> **Note:** Per-action error handling (`on error continue`) IS supported in nanoflows. Only `ErrorEvent` (raise error as a standalone flow action) is forbidden. Note that `on error rollback` is syntactically valid but only rolls back in-memory changes — nanoflows have no database transactions.

## SHOW and DESCRIBE

```sql
SHOW NANOFLOWS
SHOW NANOFLOWS IN MyModule
DESCRIBE NANOFLOW MyModule.NAV_ShowDetails
```

## DROP

```sql
DROP NANOFLOW MyModule.NAV_ShowDetails;
```

## Folder Organization

```sql
CREATE NANOFLOW Sales.NAV_OpenCart
FOLDER 'Navigation'
BEGIN
  SHOW PAGE Sales.Cart_Overview ();
END;
```

```sql
MOVE NANOFLOW Sales.NAV_OpenCart TO FOLDER 'UI/Navigation';
```

## Example: Client-Side Validation

```sql
CREATE NANOFLOW Sales.NAV_ValidateOrder
FOLDER 'Validation'
BEGIN
  DECLARE $Order Sales.Order;
  DECLARE $IsValid Boolean = true;

  IF $Order/CustomerName = empty THEN
    VALIDATION FEEDBACK $Order/CustomerName MESSAGE 'Customer name is required';
    SET $IsValid = false;
  END IF;

  IF $Order/TotalAmount <= 0 THEN
    VALIDATION FEEDBACK $Order/TotalAmount MESSAGE 'Total must be greater than zero';
    SET $IsValid = false;
  END IF;

  RETURN $IsValid;
END;
```

## Example: Page Navigation

```sql
CREATE NANOFLOW Sales.NAV_GoToOrderDetail
BEGIN
  DECLARE $Order Sales.Order;

  SHOW PAGE Sales.Order_Detail ($Order = $Order);
END;
```

## Security

Nanoflow access control uses GRANT/REVOKE to specify which module roles can execute a nanoflow. See [Grant & Revoke](./grant-revoke.md) and [Document Access](./document-access.md) for full syntax and examples.
