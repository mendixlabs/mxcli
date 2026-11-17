---
title: Kafka event integration — consumed business events and change data capture
status: proposed
date: 2026-11-17
related:
  - archive/PROPOSAL_business_events_support.md
  - data/kafka-events/
  - docs/03-development/MODELSDK_ENGINE_ARCHITECTURE.md
---

# Kafka event integration — consumed business events and change data capture

Mendix has two features that ride on a Kafka-compatible broker: **business events**
(an app publishes and subscribes to CloudEvents through the Business Events
marketplace module) and **change data capture** (CDC, beta: the runtime streams
entity changes). mxcli authors the *publishing* half of business events today and
nothing else. This proposal covers the rest:

| Part | What | Today |
|---|---|---|
| A | Read side: `describe` of a consumed business event service, `list`/`describe` of CDC services, catalog | consumer describe is broken; CDC invisible |
| B | Author a consumed business event service (contract + subscriptions) | not possible |
| C | Author a CDC service (sources, exposed attributes/associations, baseline) | not possible |
| D | Run-configuration Kafka settings (`Settings$KafkaConfiguration`) | not possible |

**Written to be built without Studio Pro.** Every BSON shape below was captured from
a Studio Pro 11.16.1-authored project, and the raw units are committed under
[`data/kafka-events/`](data/kafka-events/). The `mx check` behaviour each design
decision rests on was measured (§6) and can be re-run on Linux with a downloaded
mxbuild. What still needs Studio Pro is listed in §9 and is *not* on the critical
path.

## 1. How it works at runtime (why the model is small)

Measured by disassembling the Business Events module's `userlib/Business_Events-all.jar`
(module from the ComposableFactory test project, Feb 2024; newer module versions add
BYOK constants but the model contract is the same):

- **Topic is per channel, not per event.** `ConfigurationProvider.getConsumerTopicName`
  resolves, in order: the `BusinessEvents.ClientConfiguration` config map (Mendix
  Cloud Event Broker, per-event `topic`/`partialTopic`), else
  `businessevents.<channel>.<EventBrokerSpace>`, else the `ChannelName` constant.
  `<channel>` is the `ChannelName` UUID stored in the publisher's definition.
- **One Kafka consumer per subscribed event**, group id
  `<projectId>-<serviceName>-<eventName>` (normalised, lowercased; prefixed variants for
  a space or a config-map prefix). Every subscription reads the whole channel topic and
  filters on CloudEvent `type == eventName` (`shouldCallMicroFlow`), logging others as
  skipped. App instances share a group, so an event is handled once per app.
- `enable.auto.commit=false`; `auto.offset.reset` earliest/latest from "subscribe from
  beginning". Poll 250 ms → map the CloudEvent onto the consumed entity → **commit it to
  the database** → run the handler microflow with `executeInBackground` on task queue
  `BusinessEvents.Consumer_Queue` → `commitSync()` offsets. At-least-once; deserialisation
  failures go to `BusinessEvents.DeadLetterQueue`.

**Consequence for mxcli:** a subscription in the model is only *(message name, consumed
entity, handler microflow)*. Topic, group, offsets and credentials are derived at runtime
from the module's constants and the contract's channel id. MDL needs no Kafka settings
for business events. (The constants are already authorable with
`alter settings constant @BusinessEvents.ServerUrl …`.)

## 2. Fixtures

All from `github.com/ako/TestApp` commit `619864b`, Mendix 11.16.1, every element
authored in Studio Pro. `*.mxunit` is the raw unit; `*.json` is a lossless decode
(binary GUIDs as base64, list markers kept as the first array element).

| File | Content |
|---|---|
| `be-publisher-OrdersEventService.{mxunit,json}` | publisher service (what mxcli already writes) |
| `be-consumer-OrdersEventClient.{mxunit,json}` | **consumer** service: `Definition: null`, AsyncAPI `Document`, one `subscribe` operation |
| `asyncapi-OrdersEventService.yaml` | the consumer's `Document`, extracted |
| `cdc-OrdersCDCService.{mxunit,json}` | **CDC** service: 2 sources, attributes + association, baseline |
| `domainmodel-CdcPublish.{mxunit,json}` | entities/attributes/association whose GUIDs the CDC baseline references |
| `domainmodel-BusinessEventsClient.{mxunit,json}` | the consumed entity `CBE_OrderPlaceEvent` |
| `testapp-describe.mdl` | `describe` of every related element, including today's defective consumer describe |

These are the golden inputs for codec and executor tests (copy them into
`testdata/` of the package that tests them; do not read `docs/` from a test).

## 3. BSON — consumed business event service

Same document type as the publisher, `BusinessEvents$BusinessEventService`, in a
different shape (from `be-consumer-OrdersEventClient.json`):

```json
{
  "$Type": "BusinessEvents$BusinessEventService",
  "Name": "OrdersEventClient",
  "Definition": null,
  "Document": "asyncapi: 2.2.0\ninfo:\n  title: \"OrdersEventService\" ...",
  "Documentation": "", "Excluded": false, "ExportLevel": "Hidden",
  "OperationImplementations": [2, {
    "$Type": "BusinessEvents$ServiceOperation",
    "MessageName": "OrderPlaceEvent",
    "Operation": "subscribe",
    "Entity": "BusinessEventsClient.CBE_OrderPlaceEvent",
    "Microflow": "BusinessEventsClient.HandleBE_OrderPlaceEvent"
  }],
  "SourceApi": null
}
```

Facts the implementation depends on:

- **Publisher vs consumer is "Definition present" vs "Definition null + Document".**
  The publisher stores `Document: ""`; the AsyncAPI text exists only on the consumer
  (Studio Pro's *Export AsyncAPI document* → *import* in the consuming app).
- `Operation` is the lower-case string `"subscribe"` / `"publish"`; `Entity` and
  `Microflow` are by-name references. The publisher writes `Microflow: ""`.
- `OperationImplementations` list marker is **2**.
- The **contract is parsed by mxbuild**: an empty `Document` with `Definition: null`
  makes `mx check` throw `System.Exception: business events service should have
  definitions` (measured, §6). A `MessageName` that is not in the contract **passes**
  `mx check` (measured) — so mxcli's own check must catch it.
- Studio Pro generated, when the subscription was ticked: a **persistent** entity
  `CBE_<Message>` `extends BusinessEvents.ConsumedBusinessEvent` with one attribute per
  payload field (no access rules), and a handler microflow `HandleBE_<Message>` with a
  single parameter of that entity, returning nothing. See `testapp-describe.mdl`. Its
  publisher counterpart is a **non-persistent** `PBE_<Message>` extending
  `BusinessEvents.PublishedBusinessEvent`.

### The AsyncAPI document Studio Pro generates

`asyncapi-OrdersEventService.yaml` is the reference rendering. Structure:
`asyncapi: 2.2.0`; `info.title` = service name, `version: "1.0.0"`,
`x-mendix-generated: true`; one entry under `channels` per channel, keyed by the
32-hex `ChannelName`, with `subscribe.operationId: receive<Message>Events` (AsyncAPI 2
"subscribe" = consumers may subscribe; the publisher's message has `CanSubscribe: true`);
`components.messages.<Message>` with `payload.$ref` to
`components.schemas.<Message>Payload`; fixed `messageTraits.commonHeaders` (CloudEvents
headers) and `operationTraits.kafka` blocks — copy those verbatim from the fixture,
including the `\r\n` inside the `summary` strings.

Payload type mapping. Only `Long → {type: integer, format: int64}` is in the fixture;
the rest are inferred from the format strings present in
`Mendix.Modeler.BusinessEvents.dll` (`int32`, `int64`, `number`, `boolean`,
`date-time`, enum handling) and **must be confirmed against a Studio Pro export before
the generator claims them** (§9):

| Mendix attribute | AsyncAPI schema (to confirm except Long) |
|---|---|
| Long | `type: integer`, `format: int64` ✔ measured |
| Integer | `type: integer`, `format: int32` |
| Decimal | `type: number` |
| Boolean | `type: boolean` |
| String | `type: string` |
| DateTime | `type: string`, `format: date-time` |
| Enumeration | `type: string` + `enum: [...]` (`MessageAttribute.EnumerationDefinition`) |

## 4. BSON — CDC service

`ChangeDataCapture$EntityChangeDataCaptureService` (document, from
`cdc-OrdersCDCService.json`, abridged):

```json
{
  "$Type": "ChangeDataCapture$EntityChangeDataCaptureService",
  "Name": "OrdersCDCService", "ServiceName": "OrdersCDCService", "AppName": "TestApp",
  "Documentation": "", "Excluded": false, "ExportLevel": "Hidden",
  "Sources": [3, {
    "$Type": "ChangeDataCapture$EntityChangeSource",
    "Entity": "CdcPublish.Order", "ExposedName": "Order",
    "VersionedSourcePointer": "<$ID of the baseline source>",
    "Properties": [3,
      { "$Type": "ChangeDataCapture$AttributeProperty",
        "Attribute": "CdcPublish.Order.OrderId", "ExposedName": "OrderId",
        "VersionedPropertyPointer": "<$ID of the baseline property>" },
      { "$Type": "ChangeDataCapture$AssociationProperty",
        "Association": "CdcPublish.Order_Customer", "ExposedName": "Order_Customer",
        "VersionedPropertyPointer": "..." } ] }],
  "Baseline": {
    "$Type": "ChangeDataCapture$VersionedEntityChangeDataCaptureService",
    "Version": { "$Type": "ChangeDataCapture$Version", "Major": 1, "Minor": 1 },
    "Sources": [2, {
      "$Type": "ChangeDataCapture$VersionedEntityChangeSource",
      "EntityGuid": "<Order's GUID>", "ExposedName": "Order",
      "Version": { "Major": 1, "Minor": 1 },
      "Properties": [2,
        { "$Type": "ChangeDataCapture$VersionedProperty",
          "PropertyGuid": "<OrderId attribute GUID>", "ExposedName": "OrderId",
          "Type": "Int64", "AssociatedEntityGuid": "<all-zero GUID>" },
        { "...": "...", "PropertyGuid": "<Order_Customer association GUID>",
          "Type": "ArrayOfInt64", "AssociatedEntityGuid": "<Customer's GUID>" } ] }]
  }
}
```

Facts (each verified against `domainmodel-CdcPublish.json`):

- **`EntityGuid` / `PropertyGuid` are the domain model's `GUID`s** of the entity,
  attribute or association — not `$ID`s. For an association, `AssociatedEntityGuid`
  is the GUID of the **TO** entity (`ChildPointer` side: `Order_Customer from Order to
  Customer` → Customer's GUID); for an attribute it is the all-zero GUID (16 zero
  bytes). All are BSON binary subtype 3/4, like every GUID.
- `VersionedSourcePointer` / `VersionedPropertyPointer` are **intra-unit pointers** to the
  baseline elements' `$ID`s (CLAUDE.md: never renumber an `$ID` without its pointers).
- List markers: live `Sources` and `Properties` are **3**; baseline `Sources` and
  `Properties` are **2**.
- Baseline `Type` enum (gen `PropertyType`): `ArrayOfInt64 Binary Boolean DateTime
  Decimal Enumeration Int32 Int64 String`. Measured mapping: Long→`Int64`,
  String→`String`, Decimal→`Decimal`, reference association→`ArrayOfInt64` (yes, even
  for a single reference). Integer→`Int32`, Boolean, DateTime, Enumeration, Binary are
  the obvious remaining rows — confirm (§9); AutoNumber and HashString unknown (refuse).
- HAM (11.16 metamodel) shows two runtime-only, non-writable properties on
  `EntityChangeSource`: `channelName` and `debeziumSchemaContent` — the runtime
  publishes Debezium-format change events. Do not write them.
- Studio Pro's own MCP schema (`ped_get_schema`) annotates: *serviceName defaults to
  the document name*; *appName is set automatically by the model*; *exposedName
  defaults to the entity name without module prefix*; *never author the versioned
  types* (they are model-managed). mxcli has no model to manage them, so it must.

### Gaps in the vendored gen (`modelsdk/gen/changedatacapture`)

The gen predates 11.16 and is missing properties Studio Pro writes:

| Type | Missing (storage name) | HAM name |
|---|---|---|
| `EntityChangeDataCaptureService` | `AppName` (string) | `appName` |
| `EntityChangeSource` | `VersionedSourcePointer` (pointer) | `versionedSource` |
| `AttributeProperty`, `AssociationProperty` | `VersionedPropertyPointer` (pointer) | `versionedProperty` |
| `VersionedProperty` | `AssociatedEntityGuid` (binary GUID) | `associatedEntityGuid` |

The generator is not in-tree (MODELSDK_ENGINE_ARCHITECTURE.md §"gen is vendored"), so
add them **by hand to both `init<Type>` and `InitFromRaw`**, house-style commented, and
`gofmt`. CLAUDE.md rule 4 (write Studio Pro's exact property set) makes this required,
not optional: `mx check` tolerates the omission, Studio Pro may not.

## 5. Proposed MDL

### 5.1 Consumed business event service (part B)

Same noun as the publisher — it is the same document type, and Studio Pro calls both
"business event service". What makes it a consumer is that its property list names a
**contract** instead of `ServiceName`/`EventNamePrefix`, following the image-collection
precedent (`image logo ( File: 'path' )` or `( Data: '…' )`, describe always printing
`Data`):

```sql
mdl 1;
-- the contract of a publisher in this project, rendered by mxcli
create or modify business event service BusinessEventsClient.OrdersEventClient
(
  ContractFrom: CdcPublish.OrdersEventService
)
{
  subscribe OrderPlaceEvent
    entity BusinessEventsClient.CBE_OrderPlaceEvent
    microflow BusinessEventsClient.HandleBE_OrderPlaceEvent;
};

-- another app's contract: the AsyncAPI document Studio Pro exported
create business event service Orders.OrdersEventClient
(
  ContractFile: 'contracts/OrdersEventService.yaml'
)
{
  subscribe OrderPlaceEvent entity Orders.CBE_OrderPlaceEvent microflow Orders.HandleBE_OrderPlaceEvent;
};

-- what describe prints, whichever key created it
create or modify business event service Orders.OrdersEventClient
(
  Contract: $$
asyncapi: 2.2.0
info:
  title: "OrdersEventService"
...
$$
)
{
  subscribe OrderPlaceEvent entity Orders.CBE_OrderPlaceEvent microflow Orders.HandleBE_OrderPlaceEvent;
};
```

- Exactly one of `Contract` / `ContractFile` / `ContractFrom`; any of them together with
  `ServiceName`, `EventNamePrefix` or a `message` in the body is an error from the
  visitor (R11 key table).
- `ContractFile` resolves like an image's `File:` (relative to the script) and stores the
  text verbatim as `Document`; `ContractFrom` renders the AsyncAPI from that publisher's
  `Definition` (§3). Either way the stored `Document` *is* the contract — nothing later
  depends on the file or the publisher.
- `Contract:` takes the lexer's existing `DOLLAR_STRING` (`$$ … $$`, already used by a
  Java action's `extra code`); the property-list value rule needs to accept that token.
  `describe` always prints `Contract: $$ … $$`, so its output needs no files and
  round-trips. Check refuses a contract containing `$$` rather than escape it.
- `subscribe <Message> entity … [microflow …]` writes one `ServiceOperation`.
  The consumed entity and handler are ordinary MDL, declared before the service
  (`create persistent entity … extends BusinessEvents.ConsumedBusinessEvent (…)`,
  `create microflow …`). mxcli does not generate them implicitly — statements stay
  self-contained (design-mdl-syntax §3); a skill shows the three-statement recipe.
- `create or modify` carries the operation `$ID`s by message name, as
  `businessEventIdentityCarry` already does for the publisher.

**Check rules** (new `MDL-BE*` codes; mx does not catch these):

| Rule | Condition |
|---|---|
| contract parses | the YAML is AsyncAPI 2.x with at least one channel (else mxbuild crashes) |
| message exists | each `subscribe` names a message in `components.messages` (mx passes it, §6) |
| entity shape | entity is persistent, `extends BusinessEvents.ConsumedBusinessEvent`, and has an attribute of the mapped type for each payload property |
| handler shape | microflow has exactly one parameter, of the consumed entity |
| module present | the `BusinessEvents` marketplace module is in the project |

### 5.2 CDC service (part C)

```sql
mdl 1;
create or modify change data capture service CdcPublish.OrdersCDCService
(
  ServiceName: 'OrdersCDCService'
)
{
  entity CdcPublish.Order (
    OrderId,
    CustomerName,
    ProductName,
    Price as 'PriceAmount',      -- exposed name only when it differs
    Order_Customer               -- an association of the entity
  );
  entity CdcPublish.Customer as 'Client' (CustomerId, CustomerName, Address);
};

drop change data capture service CdcPublish.OrdersCDCService;
list change data capture services [in CdcPublish];
describe change data capture service CdcPublish.OrdersCDCService;
```

- `ServiceName` defaults to the document name; `AppName` is not MDL — written as the
  project name (see §10) and preserved on modify.
- A member name resolves to an attribute or association of that entity (attributes win;
  ambiguity is a check error).
- **The baseline is computed, not authored.** After building the live sources, mxcli
  rebuilds `Baseline` so it is structurally equal to them (this is what Studio Pro's
  *accept changes* does) and sets the pointers. Version policy, since `mx check` ignores
  the numbers (§6) and they are a contract signal to consumers:
  - create: every source and the service at `1.0`;
  - modify, only additions to a source: that source `minor+1`;
  - modify with a removal, a rename of an exposed name, or a type change: that source
    `major+1, minor 0`;
  - the service version takes the same bump as its largest source bump;
  - unchanged sources keep their stored version and baseline element `$ID`s.
  The fixture is consistent with this (Order is 1.1 after its association was added;
  Customer 1.0; service 1.1), but the rule is inferred — §9.
- Gate: Mendix ≥ 11.10 (where `Baseline` was introduced and CE1585 enforces it). Below
  that, refuse with a message rather than guess the older shape.

### 5.3 Kafka run configuration (part D)

Studio Pro's *App Settings → Configurations → Kafka* is `Settings$KafkaConfiguration`
on `Settings$Configuration` (HAM: `bootstrapServers`, `username`, `password`; stored
keys `BootstrapServers`, `Username`, `Password`, nested under `KafkaConfiguration`).
Extend the existing statement:

```sql
alter settings configuration 'Default' (
  KafkaBootstrapServers: 'localhost:9092',
  KafkaUsername: '',
  KafkaPassword: ''
);
```

Overlay rule (MODELSDK_ENGINE_ARCHITECTURE.md, "never invent a key"): write only when
the stored configuration already carries a `KafkaConfiguration` part; otherwise refuse
with the Mendix version that introduced it.

### 5.4 Read side (part A — do first)

- **Defect today:** `describe business event service BusinessEventsClient.OrdersEventClient`
  prints only `create or modify business event service …` — no contract, no
  subscription, no property list, no `;`. Measured: `mxcli exec` of that output is a
  parse error (`mismatched input '<EOF>' expecting '('`), so nothing is lost, but the
  round-trip is broken and an LLM reading it learns nothing. Fix with 5.1's describe
  form; until 5.1's grammar lands, print the contract and subscriptions as comments.
  Note `create or modify` of a *publisher* over an existing consumer must refuse, not
  overwrite the contract.
- `list business event services` gains a `Kind` column (published / consumed).
- `list business event clients` (today: prints "not yet implemented") is removed or
  becomes a filter for consumed services — see §10.
- CDC: `list` / `describe`, a catalog table `change_data_capture_services` (+ a sources
  table), `RefKind`s for entity/attribute/association references so `show callers` /
  impact analysis see CDC exposure, and a row in `MDL_FEATURE_MATRIX.md`.
- Catalog: bump `CatalogSchemaVersion`.

## 6. Measured `mx check` behaviour (mxbuild 11.16.1)

On a copy of the fixture project; each variant mutates one unit's BSON. Control: the
unmodified copy → **0 errors**.

| Variant | Result |
|---|---|
| CDC: `Baseline = null`, pointers removed | **CE1585** "There are changes in your 'OrdersCDCService' document. Please accept the changes or undo them." |
| CDC: as above and `AppName` removed | **CE1577** "'' is not allowed. App name should start with a letter …" + CE1585 |
| CDC: pointers removed, baseline kept | 0 errors |
| CDC: every `Version` reset to 1.0 | 0 errors |
| CDC: one live property removed, baseline kept | **CE1585** |
| CDC: one live `ExposedName` changed, baseline kept | **CE1585** |
| BE consumer: `Document = ""` | mxbuild throws `System.Exception: business events service should have definitions` |
| BE consumer: `MessageName` not in contract | 0 errors |

So: the baseline must mirror the live sources exactly (set of properties and exposed
names); version numbers and pointers are not validated by mxbuild (write them anyway,
§4); `AppName` must be a valid app name; a consumer must carry a contract.

To re-run on Linux: `mxcli setup mxbuild --version 11.16.1`, restore the fixture units
into an `mxcli new` 11.16.1 project with matching modules/entities (or build the
entities from `testapp-describe.mdl`), mutate with a few lines of Go over
`bson.D`, and `mxcli docker check`.

## 7. Implementation plan

| Step | Files | Notes |
|---|---|---|
| A1 consumer describe/list | `mdl/executor/cmd_businessevents.go` | fix the lossy describe first; it is a data-loss bug on round-trip |
| A2 CDC read | new `mdl/backend/modelsdk/cdc_read.go`, `mdl/backend/service.go` (interface), mock, `mdl/executor/cmd_cdc.go`, catalog builder + tables, registry | gen types exist; add the 4 missing properties (§4) first so reads see them |
| B1 grammar/AST | `MDLParser.g4` (`Contract`/`ContractFile`/`ContractFrom` keys, `DOLLAR_STRING` as a property value, `subscribe` body item), `ast_businessevents.go`, `visitor_businessevents.go` | |
| B2 AsyncAPI renderer | new `mdl/businessevents/asyncapi.go` | golden test = `asyncapi-OrdersEventService.yaml`, byte-for-byte |
| B3 write | `mdl/backend/modelsdk/business_event_write.go` | `Definition` null, `Document`, subscribe ops; identity carry by message name |
| B4 check rules | `mdl/executor/validate_*` | §5.1 table |
| C1 grammar/AST/visitor | new `ast_cdc.go`, `visitor_cdc.go` | |
| C2 write | new `mdl/backend/modelsdk/cdc_write.go` | live sources + computed baseline + pointers; GUIDs read from the domain model; wire through `canon.Reconcile`; consider a `canon.identityFields` row (identity = `Entity` / `Attribute` / `Association` by name) |
| C3 version policy | `cdc_write.go` | §5.2 |
| D1 settings | settings overlay | §5.3 |
| docs | `docs-site/src/`, `cmd/mxcli/syntax/`, a skill `.claude/skills/mendix/business-events/SKILL.md` | the consumer recipe and the runtime facts in §1 |

MCP backend: out of scope here. Studio Pro 11.16.1's PED has **no** create path for
`BusinessEvents$BusinessEventService` (`ped_read_document` answers "Unknown document
type") but **does** return a constructor schema for
`ChangeDataCapture$EntityChangeDataCaptureService` (`name`, `appName`, `serviceName`),
so CDC over `--mcp` is likely feasible as a follow-up — it needs a live Studio Pro.

## 8. Test plan

- **Codec:** decode each fixture `.mxunit`; re-encode with no change → byte-identical
  (proves the hand-added gen properties and list markers).
- **Writer, golden:** build the consumer and CDC services from MDL against an in-memory
  project containing the fixture domain models; compare to the fixture JSON ignoring
  `$ID` values (but checking pointer consistency: every `Versioned*Pointer` resolves to a
  baseline `$ID` in the same unit).
- **GUID test with a control** (CLAUDE.md §GUID): the fixture entities are Studio
  Pro-authored, so `GUID != $ID` — assert `EntityGuid == entity.GUID` *and* `!= entity.$ID`,
  so a writer that used `$ID` fails.
- **Modify:** `create or modify` adding `ProductName` to a source keeps every other
  baseline `$ID`, bumps that source's minor, and is elided entirely when re-run unchanged
  (ADR-0008 conditional write; include a control that a real change does write).
- **Describe round-trip:** describe → exec → describe is a fixed point for both
  documents; `mdl-examples/doctype-tests/` gets a consumer and a CDC script.
- **Integration (`-tags integration`):** each §6 variant as a test — the valid writes pass
  `mx check`; a writer forced to omit the baseline reproduces CE1585 (the control that
  proves the test detects the defect).

## 9. Needs Studio Pro later (not blocking)

1. AsyncAPI rendering of Integer, Decimal, Boolean, String, DateTime and Enumeration
   payload fields: add one attribute of each to `OrdersEventService`, export, diff
   against the generator. Until then the renderer refuses those types for
   `ContractFrom` (`Contract` / `ContractFile` are unaffected).
2. CDC version policy (§5.2): add, rename and remove a property in Studio Pro, accept,
   and record the versions it picks.
3. CDC `Type` for Integer/Boolean/DateTime/Enumeration/Binary/AutoNumber/HashString.
4. Whether Studio Pro (not just mxbuild) opens a document with the pointers present but
   pointing at mxcli-minted baseline `$ID`s — expected yes; confirm once.
5. What registers the subscriptions at startup: no microflow in the project calls
   `BusinessEvents.ConsumeBusinessEvent`, and no 11.15 runtime jar references it by
   name. Not needed to author the model; needed for the skill's troubleshooting section.

## 10. Open questions

1. **`AppName`:** the fixture holds the project name (`TestApp`). Write the `.mpr`
   basename, or read it from somewhere authoritative (project settings)? Must satisfy
   CE1577's pattern either way.
2. **`list business event clients`:** remove it, or keep it as `list … services` filtered
   to consumed ones? (Recommendation: remove; one noun, a `Kind` column.)
3. Should a service with `ContractFrom: X` also *create* the
   `CBE_` entity and `HandleBE_` microflow when they are absent, as Studio Pro does?
   Recommendation: no (explicit statements), but `mxcli` could offer a
   `--scaffold`-style generator later.
