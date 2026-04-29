# Business Event Service Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/301)

## Test Projects

Demo apps from [Mendix App Gallery](https://appgallery.mendixcloud.com/):

| App | Studio Pro | Business Event Services |
|-----|-----------|------------------------|
| Lato Enquiry Management | 11.4.0 | <N> |
| Evora - Factory Management | 10.24.15 | <N> |
| Lato Product Inventory | 11.2.0 | <N> |

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
  echo "show business event services;" > /tmp/show-bes.mdl
  mxcli exec /tmp/show-bes.mdl -p "$mpr" 2>&1 | tail -1
done
```

Expected: count line `(N business event services)` for each project.

### 4. Interactive testing

```bash
mxcli repl -p <path-to-app>/EnquiriesManagement.mpr
```

### 5. Script-based testing

```bash
mxcli exec test-sequence.mdl -p <mpr>
```

Write operations (CREATE, DROP) modify the `.mpr` file **in place**.

> **IMPORTANT:** Always run destructive tests against a **copy** of the project folder,
> never the original. The `.mpr` file references other files in the project directory.
> Dropped services cannot be recovered — there is no undo.
>
> ```bash
> # Before each destructive test session
> cp -r MyProject MyProject-test
> mxcli repl -p MyProject-test/MyProject.mpr
> ```

---

## 1. SHOW BUSINESS EVENT SERVICES

### 1.1 List all services

```
show business event services;
```

**Expected:** Table with columns `Module | QualifiedName | Service | Messages | Publish | Subscribe`. Summary line `(N business event services)`. Sorted alphabetically.

### 1.2 Filter by module

```
show business event services in MyModule;
```

**Expected:** Only services from `MyModule`. Same column format.

### 1.3 Empty result

```
show business event services in NonExistentModule;
```

**Expected:** `No business event services found.`

---

## 2. SHOW BUSINESS EVENTS

### 2.1 List all business events

```
show business events;
```

**Expected:** Table with columns `Service | Message | Operation | Entity | Attributes`. `Operation` values are `PUBLISH` or `SUBSCRIBE`. Summary line `(N business events)`.

### 2.2 Filter by module

```
show business events in MyModule;
```

**Expected:** Only events from `MyModule`. Same column format.

### 2.3 Empty module

```
show business events in NonExistentModule;
```

**Expected:** Empty result or `No business events found.`

---

## 3. DESCRIBE BUSINESS EVENT SERVICE

### 3.1 Service with publish message

```
describe business event service MyModule.OrderEvents;
```

**Expected:** Full MDL output:
```
create business event service MyModule.OrderEvents
  event_name_prefix 'com.example.order'
  message OrderCreated (
    OrderId: Long,
    CustomerName: String,
    Total: Decimal
  ) publish entity MyModule.OrderCreatedEvent
/
```

### 3.2 Service with subscribe message

```
describe business event service MyModule.PaymentEvents;
```

**Expected:** MDL includes `subscribe` keyword with entity and microflow references:
```
create business event service MyModule.PaymentEvents
  event_name_prefix 'com.example.payment'
  message PaymentReceived (
    PaymentId: Long,
    Amount: Decimal,
    Currency: String
  ) subscribe entity MyModule.PaymentReceivedEvent microflow MyModule.OnPaymentReceived
/
```

### 3.3 Service with multiple messages

```
describe business event service MyModule.InventoryEvents;
```

**Expected:** Multiple `message` blocks in output. Each has its own attribute list, operation keyword, and entity reference.

### 3.4 All attribute types

Verify described output includes attributes of each supported type:

| Type | Example |
|------|---------|
| Long | `OrderId: Long` |
| String | `Name: String` |
| Integer | `Count: Integer` |
| Boolean | `IsActive: Boolean` |
| DateTime | `Timestamp: DateTime` |
| Decimal | `Amount: Decimal` |

### 3.5 Non-existent service

```
describe business event service MyModule.Fake;
```

**Expected:** Error — service not found.

---

## 4. CREATE BUSINESS EVENT SERVICE

### 4.1 Minimal publish service

```
create business event service MyModule.OrderEvents
  event_name_prefix 'com.example.order'
  message OrderCreated (
    OrderId: Long,
    Description: String
  ) publish entity MyModule.OrderCreatedEvent;
```

**Expected:** Service created. `describe business event service MyModule.OrderEvents` matches input.

### 4.2 Subscribe service with microflow

```
create business event service MyModule.PaymentEvents
  event_name_prefix 'com.example.payment'
  message PaymentReceived (
    PaymentId: Long,
    Amount: Decimal,
    Currency: String
  ) subscribe entity MyModule.PaymentReceivedEvent microflow MyModule.OnPaymentReceived;
```

**Expected:** Service created. `describe` shows `subscribe` keyword, entity, and microflow references.

### 4.3 Multiple messages

```
create business event service MyModule.InventoryEvents
  event_name_prefix 'com.example.inventory'
  message StockUpdated (
    ProductId: Long,
    NewQuantity: Integer
  ) publish entity MyModule.StockUpdatedEvent
  message StockDepleted (
    ProductId: Long,
    WarehouseId: String
  ) subscribe entity MyModule.StockDepletedEvent microflow MyModule.OnStockDepleted;
```

**Expected:** Service created with two messages. `describe` shows both message blocks.

### 4.4 All attribute types

```
create business event service MyModule.TypeTest
  event_name_prefix 'com.example.typetest'
  message AllTypes (
    Id: Long,
    Name: String,
    Count: Integer,
    Active: Boolean,
    Timestamp: DateTime,
    Amount: Decimal
  ) publish entity MyModule.AllTypesEvent;
```

**Expected:** Service created. `describe` preserves all six attribute types.

### 4.5 `create or replace` variant

```
create or replace business event service MyModule.OrderEvents
  event_name_prefix 'com.example.order.v2'
  message OrderCreatedV2 (
    OrderId: Long,
    Description: String,
    Priority: Integer
  ) publish entity MyModule.OrderCreatedV2Event;
```

**Expected:** Drops existing service and recreates. `describe` shows updated prefix and message.

### 4.6 `in folder` clause

```
create business event service MyModule.FolderTest
  event_name_prefix 'com.example.foldertest'
  in folder 'Events/Inbound'
  message TestMsg (
    Value: String
  ) publish entity MyModule.TestMsgEvent;
```

**Expected:** Service created in specified folder. Folder auto-created if missing.

### 4.7 Export level defaults to Hidden

```
create business event service MyModule.HiddenCheck
  event_name_prefix 'com.example.hidden'
  message Msg (
    X: Integer
  ) publish entity MyModule.HiddenCheckEvent;
```

**Expected:** Service created. Export level defaults to `Hidden`. Verify via `describe` output or model inspection.

### 4.8 Duplicate service (without `or replace`)

```
create business event service MyModule.OrderEvents
  event_name_prefix 'com.example.order'
  message OrderCreated (
    OrderId: Long
  ) publish entity MyModule.OrderCreatedEvent;
```

**Expected:** Error — service already exists.

---

## 5. DROP BUSINESS EVENT SERVICE

### 5.1 Drop existing service

```
drop business event service MyModule.OrderEvents;
```

**Expected:** Service removed. `describe business event service MyModule.OrderEvents` returns error.

### 5.2 Drop non-existent service

```
drop business event service MyModule.Fake;
```

**Expected:** Error — service not found.

---

## 6. ROUNDTRIP

### 6.1 Publish service roundtrip

```
create business event service RtTest.Events
  event_name_prefix 'com.example.rt'
  message Created (
    Id: Long,
    Name: String,
    Amount: Decimal
  ) publish entity RtTest.CreatedEvent;
```

1. Run `describe business event service RtTest.Events`
2. Copy output
3. Drop: `drop business event service RtTest.Events`
4. Execute copied MDL
5. Run `describe` again

**Expected:** Output identical between step 1 and step 5.

### 6.2 Subscribe service roundtrip

```
create business event service RtTest.SubEvents
  event_name_prefix 'com.example.rtsub'
  message Received (
    PaymentId: Long,
    Status: String
  ) subscribe entity RtTest.ReceivedEvent microflow RtTest.OnReceived;
```

1. `describe business event service RtTest.SubEvents`
2. Drop: `drop business event service RtTest.SubEvents`
3. Execute described MDL
4. `describe` again

**Expected:** Identical output.

---

## 7. MULTI-STEP WORKFLOWS

### 7.1 Create entity → create publish service → verify events

```
create persistent entity MyModule.InvoiceCreatedEvent (
  InvoiceId: Long,
  CustomerName: String,
  TotalAmount: Decimal
);

create business event service MyModule.InvoiceEvents
  event_name_prefix 'com.example.invoice'
  message InvoiceCreated (
    InvoiceId: Long,
    CustomerName: String,
    TotalAmount: Decimal
  ) publish entity MyModule.InvoiceCreatedEvent;

show business events in MyModule;
```

**Expected:** All statements succeed. `show business events` lists `InvoiceCreated` with operation `PUBLISH` and entity `MyModule.InvoiceCreatedEvent`.

### 7.2 Create entity → create subscribe service → verify events

```
create persistent entity MyModule.ShipmentReceivedEvent (
  ShipmentId: Long,
  Carrier: String
);

create business event service MyModule.ShipmentEvents
  event_name_prefix 'com.example.shipment'
  message ShipmentReceived (
    ShipmentId: Long,
    Carrier: String
  ) subscribe entity MyModule.ShipmentReceivedEvent microflow MyModule.OnShipmentReceived;

show business events in MyModule;
```

**Expected:** `show business events` lists `ShipmentReceived` with operation `SUBSCRIBE`.

### 7.3 Create → replace → verify

```
create business event service MyModule.Lifecycle
  event_name_prefix 'com.example.lc.v1'
  message Ping (
    Seq: Integer
  ) publish entity MyModule.PingEvent;

create or replace business event service MyModule.Lifecycle
  event_name_prefix 'com.example.lc.v2'
  message Ping (
    Seq: Integer,
    Timestamp: DateTime
  ) publish entity MyModule.PingEvent;

describe business event service MyModule.Lifecycle;
```

**Expected:** Final `describe` shows v2 prefix and `Timestamp` attribute.

---

## 8. FAILURE MODES & ERROR RECOVERY

### 8.1 Not connected to project

```
show business event services;
```

(Run without `-p` flag or before opening a project.)

**Expected:** Error — not connected to a project.

### 8.2 Service not found

```
describe business event service MyModule.DoesNotExist;
```

**Expected:** Error — service not found.

### 8.3 Service already exists

```
create business event service MyModule.OrderEvents
  event_name_prefix 'com.example.order'
  message Msg (X: Integer) publish entity MyModule.MsgEvent;
create business event service MyModule.OrderEvents
  event_name_prefix 'com.example.order'
  message Msg (X: Integer) publish entity MyModule.MsgEvent;
```

**Expected:** First succeeds. Second returns error — already exists.

### 8.4 Module not found

```
show business event services in FakeModule;
```

**Expected:** `No business event services found.` or error — module not found.

### 8.5 Missing entity reference

```
create business event service MyModule.BadRef
  event_name_prefix 'com.example.badref'
  message Msg (X: Integer) publish entity MyModule.NonExistentEntity;
```

**Expected:** Error — entity not found.

### 8.6 Invalid attribute type

```
create business event service MyModule.BadType
  event_name_prefix 'com.example.badtype'
  message Msg (X: InvalidType) publish entity MyModule.SomeEvent;
```

**Expected:** Error — unknown type. No service created.

---

## Test Project Coverage Matrix

| Operation | Lato Enquiry | Evora Factory | Lato Product |
|-----------|:---:|:---:|:---:|
| SHOW BUSINESS EVENT SERVICES | x | x | x |
| SHOW BUSINESS EVENTS | x | x | x |
| DESCRIBE BUSINESS EVENT SERVICE | x | x | |
| CREATE BUSINESS EVENT SERVICE | x | | |
| DROP BUSINESS EVENT SERVICE | x | | |

Read operations tested on all projects. Write operations on copies of one project.

---

## Automated Test Coverage

| Section | Automated | Manual-only |
|---------|:---------:|:-----------:|
| 1. SHOW BUSINESS EVENT SERVICES | Mock tests | |
| 2. SHOW BUSINESS EVENTS | Mock tests | |
| 3. DESCRIBE BUSINESS EVENT SERVICE | Mock tests | |
| 4. CREATE BUSINESS EVENT SERVICE | Mock tests | Folder, export level |
| 5. DROP BUSINESS EVENT SERVICE | Mock tests | |
| 6. Roundtrip | Roundtrip tests | Complex services |
| 7. Multi-step | | All manual |
| 8. Failure modes | Partial | Edge cases |

---

## Manual Test Report Template

**Tester:** _______________
**Date:** _______________
**Project:** _______________

| # | Section | Test | Pass | Fail | Skip | Notes |
|---|---------|------|:----:|:----:|:----:|-------|
| 1.1 | SHOW BE SERVICES | List all | | | | |
| 1.2 | SHOW BE SERVICES | Filter by module | | | | |
| 1.3 | SHOW BE SERVICES | Empty result | | | | |
| 2.1 | SHOW BE | List all | | | | |
| 2.2 | SHOW BE | Filter by module | | | | |
| 2.3 | SHOW BE | Empty module | | | | |
| 3.1 | DESCRIBE BE SERVICE | Publish message | | | | |
| 3.2 | DESCRIBE BE SERVICE | Subscribe message | | | | |
| 3.3 | DESCRIBE BE SERVICE | Multiple messages | | | | |
| 3.4 | DESCRIBE BE SERVICE | All attribute types | | | | |
| 3.5 | DESCRIBE BE SERVICE | Not found | | | | |
| 4.1 | CREATE BE SERVICE | Minimal publish | | | | |
| 4.2 | CREATE BE SERVICE | Subscribe with microflow | | | | |
| 4.3 | CREATE BE SERVICE | Multiple messages | | | | |
| 4.4 | CREATE BE SERVICE | All attribute types | | | | |
| 4.5 | CREATE BE SERVICE | create or replace | | | | |
| 4.6 | CREATE BE SERVICE | in folder | | | | |
| 4.7 | CREATE BE SERVICE | Export level hidden | | | | |
| 4.8 | CREATE BE SERVICE | Duplicate error | | | | |
| 5.1 | DROP BE SERVICE | Existing | | | | |
| 5.2 | DROP BE SERVICE | Non-existent | | | | |
| 6.1 | ROUNDTRIP | Publish service | | | | |
| 6.2 | ROUNDTRIP | Subscribe service | | | | |
| 7.1 | MULTI-STEP | Entity + publish + verify | | | | |
| 7.2 | MULTI-STEP | Entity + subscribe + verify | | | | |
| 7.3 | MULTI-STEP | Create + replace + verify | | | | |
| 8.1 | FAILURE | Not connected | | | | |
| 8.2 | FAILURE | Service not found | | | | |
| 8.3 | FAILURE | Already exists | | | | |
| 8.4 | FAILURE | Module not found | | | | |
| 8.5 | FAILURE | Missing entity ref | | | | |
| 8.6 | FAILURE | Invalid attribute type | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
