# Business Event Service Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/386)

## Setup

> See [AGENT-TESTING.md](./AGENT-TESTING.md) for build, execution methods, and verification patterns.

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

## 4. DROP BUSINESS EVENT SERVICE

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

## 5. MULTI-STEP WORKFLOWS

### 5.1 Create entity → create publish service → verify events

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

### 5.2 Create entity → create subscribe service → verify events

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

### 5.3 Create → replace → verify

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

## 6. FAILURE MODES & ERROR RECOVERY

### 6.1 Not connected to project

```
show business event services;
```

(Run without `-p` flag or before opening a project.)

**Expected:** Error — not connected to a project.

### 6.2 Service not found

```
describe business event service MyModule.DoesNotExist;
```

**Expected:** Error — service not found.

### 6.3 Service already exists

```
create business event service MyModule.OrderEvents
  event_name_prefix 'com.example.order'
  message Msg (X: Integer) publish entity MyModule.MsgEvent;
create business event service MyModule.OrderEvents
  event_name_prefix 'com.example.order'
  message Msg (X: Integer) publish entity MyModule.MsgEvent;
```

**Expected:** First succeeds. Second returns error — already exists.

### 6.4 Module not found

```
show business event services in FakeModule;
```

**Expected:** `No business event services found.` or error — module not found.

### 6.5 Missing entity reference

```
create business event service MyModule.BadRef
  event_name_prefix 'com.example.badref'
  message Msg (X: Integer) publish entity MyModule.NonExistentEntity;
```

**Expected:** Error — entity not found.

### 6.6 Invalid attribute type

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
| DROP BUSINESS EVENT SERVICE | x | | |

Read operations tested on all projects. Write operations on copies of one project.

---

## Automated Test Coverage

| Section | Automated | Manual-only |
|---------|:---------:|:-----------:|
| 1. SHOW BUSINESS EVENT SERVICES | Mock tests | |
| 2. SHOW BUSINESS EVENTS | Mock tests | |
| 3. DESCRIBE BUSINESS EVENT SERVICE | Mock tests | |
| 4. DROP BUSINESS EVENT SERVICE | Mock tests | |
| 5. Multi-step | | All manual |
| 6. Failure modes | Partial | Edge cases |

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
| 4.1 | DROP BE SERVICE | Existing | | | | |
| 4.2 | DROP BE SERVICE | Non-existent | | | | |
| 5.1 | MULTI-STEP | Entity + publish + verify | | | | |
| 5.2 | MULTI-STEP | Entity + subscribe + verify | | | | |
| 5.3 | MULTI-STEP | Create + replace + verify | | | | |
| 6.1 | FAILURE | Not connected | | | | |
| 6.2 | FAILURE | Service not found | | | | |
| 6.3 | FAILURE | Already exists | | | | |
| 6.4 | FAILURE | Module not found | | | | |
| 6.5 | FAILURE | Missing entity ref | | | | |
| 6.6 | FAILURE | Invalid attribute type | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
