# CREATE BUSINESS EVENT SERVICE

## Synopsis

```sql
CREATE [ OR MODIFY ] BUSINESS EVENT SERVICE module.Name
(
    ServiceName: 'service_name',
    EventNamePrefix: 'prefix'
)
{
    MESSAGE MessageName ( attr: Type [, ...] ) {
        PUBLISH ( Entity: module.PBE_Entity ) ;
        SUBSCRIBE ( Entity: module.CBE_Entity [, Microflow: module.Handler] ) ;
    } ;
    [ ... ]
}
```

## Description

Creates a business event service that defines one or more messages for event-driven communication between applications.

Each message has a name, a set of typed attributes, and its operations as children: PUBLISH, SUBSCRIBE, or both. A publish operation means this application sends events of that type. A subscribe operation means this application receives and handles them. A message the application both publishes and consumes itself (an in-app bus, a request/reply loop) has both operations.

Declare each message once. The same name declared twice, once per operation, stores two messages of one name; `mx check` accepts it but the deployment fails ("An item with the same key has already been added"), so `check` refuses it.

The older keyword-clause spelling, `MESSAGE X (…) PUBLISH ENTITY M.E MICROFLOW M.H;`, still parses as a deprecated alias (MDL-DEPR075), and `mxcli fmt --upgrade` rewrites it.

The `OR MODIFY` option updates the existing service in-place, preserving its UUID so that references from other documents remain valid. `OR REPLACE` is also accepted as a synonym.

### Service Properties

The service declaration includes two required properties:

- **ServiceName** -- The logical service name used for event routing on the Mendix Business Events broker.
- **EventNamePrefix** -- A prefix prepended to message names to form the fully qualified event name. Can be empty (`''`).

### Message Attributes

Each message defines zero or more attributes with the following supported types:

| Type | Description |
|------|-------------|
| `String`, `String(n)`, `String(unlimited)` | Text value. The length must match the linked entity attribute's; a bare `String` stores no length, which the build reads as 200 |
| `Integer` | 32-bit integer |
| `Long` | 64-bit integer |
| `Boolean` | True/false |
| `DateTime` | Date and time |
| `Decimal` | Arbitrary-precision decimal |

### Entity Mapping

Each operation's `Entity` property links it to a Mendix entity. For a subscribe operation, incoming events are mapped to instances of this entity and handled by its `Microflow`. For a publish operation, entity instances are serialized into outgoing events.

## Parameters

`module.Name`
:   Qualified name of the business event service (`Module.ServiceName`).

`ServiceName: 'service_name'`
:   The logical service identifier used by the Business Events broker.

`EventNamePrefix: 'prefix'`
:   A prefix for event names. Use an empty string (`''`) for no prefix.

`MESSAGE MessageName`
:   The name of a message within the service.

`attr: Type`
:   An attribute definition within a message. Multiple attributes are comma-separated.

`PUBLISH ( Entity: … )`
:   This application publishes (sends) this message type.

`SUBSCRIBE ( Entity: …, Microflow: … )`
:   This application subscribes to (receives) this message type, handled by the microflow.

`Entity: module.Entity`
:   The entity linked to the operation for data mapping.

## Examples

Create a service that publishes order events:

```sql
CREATE BUSINESS EVENT SERVICE Shop.OrderEvents
(
    ServiceName: 'com.example.shop.orders',
    EventNamePrefix: 'shop'
)
{
    MESSAGE OrderCreated (OrderId: Long, CustomerName: String, Total: Decimal) {
        PUBLISH (Entity: Shop.Order);
    };
    MESSAGE OrderShipped (OrderId: Long, TrackingNumber: String) {
        PUBLISH (Entity: Shop.Shipment);
    };
};
```

Create a service that subscribes to external events:

```sql
CREATE BUSINESS EVENT SERVICE Inventory.StockUpdates
(
    ServiceName: 'com.example.warehouse.stock',
    EventNamePrefix: ''
)
{
    MESSAGE StockChanged (ProductId: Long, NewQuantity: Integer) {
        SUBSCRIBE (Entity: Inventory.StockLevel, Microflow: Inventory.HandleStockChanged);
    };
};
```

Publish and consume one message in the same application (an in-app bus):

```sql
CREATE BUSINESS EVENT SERVICE Agents.AgentBus
(
    ServiceName: 'AgentBus',
    EventNamePrefix: ''
)
{
    MESSAGE AgentMessage (TaskId: String(36), Text: String(unlimited)) {
        PUBLISH (Entity: Agents.PBE_AgentMessage);
        SUBSCRIBE (Entity: Agents.CBE_AgentMessage, Microflow: Agents.SUB_OnMessage);
    };
};
```

Replace an existing service:

```sql
CREATE OR MODIFY BUSINESS EVENT SERVICE Shop.OrderEvents
(
    ServiceName: 'com.example.shop.orders',
    EventNamePrefix: 'shop'
)
{
    MESSAGE OrderCreated (OrderId: Long, CustomerName: String, Total: Decimal) { PUBLISH (); };
    MESSAGE OrderCancelled (OrderId: Long, Reason: String) { PUBLISH (); };
};
```

## See Also

[DROP BUSINESS EVENT SERVICE](drop-business-event-service.md)
