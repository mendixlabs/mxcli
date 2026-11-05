# OData and Integration Statements

Statements for managing OData services, external entities, and browsing service contracts.

Mendix supports consuming and publishing OData services. Consumed services (OData clients) connect to remote OData endpoints and make external entity types available for use in your application. Published services expose your domain model entities as OData endpoints for other applications to consume.

## OData Client Statements

| Statement | Description |
|-----------|-------------|
| [CREATE CONSUMED ODATA SERVICE](create-odata-client.md) | Create a consumed OData service (auto-fetches $metadata) |
| [ALTER CONSUMED ODATA SERVICE](alter-odata-client.md) | Modify OData client properties |
| [DROP CONSUMED ODATA SERVICE](drop-odata-client.md) | Remove a consumed OData service |

## OData Service Statements (Published)

| Statement | Description |
|-----------|-------------|
| [CREATE PUBLISHED ODATA SERVICE](create-odata-service.md) | Publish entities as an OData endpoint |
| [ALTER PUBLISHED ODATA SERVICE](alter-odata-service.md) | Modify published service properties |
| [DROP PUBLISHED ODATA SERVICE](drop-odata-service.md) | Remove a published OData service |

## External Entity Statements

| Statement | Description |
|-----------|-------------|
| [CREATE EXTERNAL ENTITY](create-external-entity.md) | Import an entity from a consumed OData service |
| [CREATE EXTERNAL ENTITIES](#create-external-entities) | Bulk-create external entities from cached $metadata |

## CREATE EXTERNAL ENTITIES

Bulk-create external entities from a consumed OData service's cached `$metadata`.

```sql
mdl 1;
-- Create all entity types from the contract
CREATE EXTERNAL ENTITIES FROM Module.Service;

-- Create into a specific module
CREATE EXTERNAL ENTITIES FROM Module.Service INTO TargetModule;

-- Filter to specific entities
CREATE EXTERNAL ENTITIES FROM Module.Service ENTITIES (Customer, Order);

-- Idempotent — update existing entities
CREATE OR MODIFY EXTERNAL ENTITIES FROM Module.Service;
```

Each entity is named after its OData **entity type**, as Studio Pro names it: an
entity type `Part` exposed as entity set `Parts` becomes `Module.Part`. The entity
set is kept as the entity's remote entity set. A re-import finds an entity by the
service and entity set it maps first, so an entity that an earlier mxcli version
named after the set (`Module.Parts`) is updated in place and keeps its name.

### Complex types are flattened

The Mendix domain model has no complex types. A property typed as an OData
`ComplexType` is imported as one attribute per leaf — the same thing Studio Pro
does — named `<property>_<leaf>` and read over the OData path `<property>/<leaf>`:

| $metadata | Mendix attribute | Remote name |
|-----------|------------------|-------------|
| `MaxQty` of type `Shared.Uom.Quantity` { `UoMNId`, `QuantityValue` } | `MaxQty_UoMNId`, `MaxQty_QuantityValue` | `MaxQty/UoMNId`, `MaxQty/QuantityValue` |

The complex type may live in any `Schema` in the document — it is resolved by
qualified name, so two namespaces may each declare a `Quantity`.

Three properties of a complex type are **not** importable. Each is reported by
name and reason rather than dropped, so an import that loses something says so:

- **Inherited properties.** Only a complex type's own properties are imported.
  Where `AirportLocation` derives from `Location`, the inherited `Address` is not
  reachable through it (`CE6615`) — though the same `Address` is imported
  normally through a property typed `Location` directly.
- **Leaves Mendix cannot represent**, such as `Edm.GeographyPoint` (`CE6622`).
- **A complex type nested in a complex type** — flattening is one level deep.

Two more consequences:

- **Flattened attributes are read-only.** Mendix treats an external entity that
  contains them as readable and deletable only, whatever the entity set's
  `InsertRestrictions` / `UpdateRestrictions` say. Marking them creatable or
  updatable is `CE6630`.
- **They are filterable and sortable only where their entity is** — that is, on
  an entity backed by an entity set. On a derived or contained type they are
  neither. `CE6630` fires in both directions here, so this follows the contract
  rather than a fixed answer.

### Navigation properties become associations

Each navigation property whose target is also imported becomes an association.
A navigation property with a `Partner` is one end of a bidirectional
relationship, and Mendix does not accept its ends as separate associations
(`CE6618` "Navigation '…' is not available in the OData service"):

| Navigation | Association |
|------------|-------------|
| No `Partner` | One per navigation property — `Reference`, or `ReferenceSet` for a collection |
| One-to-many pair (`Lines` / `ParentBuffer`) | One `Reference` from the single-valued end, naming both navigation properties |
| Many-to-many or one-to-one pair | None. Mendix offers no such navigation; the import lists the pair as not imported |

A re-import of a module that an earlier mxcli version imported the old way lists
the associations that fail with `CE6618` as `drop association …;` lines. Run them,
then run the import again.

## Calling an External Action

```sql
$Response = call external action Odata.OrderService.CreateBuffer(
  command = $Cmd include associations (Odata.Quantity (Odata.Unit)),
  additional = nothing
);
```

`include associations ( … )` after an argument is Studio Pro's **Associations to
include**: the associations whose objects are serialised into the OData request
with that argument. It is a tree. A name followed by `( … )` lists the
associations included under it, reached from the entity at its far end. Without
the clause, a sub-object linked to the argument is not sent, and nothing fails
until the service receives the request.

- Which end of each association the object is on is worked out from the domain
  model, so the clause names only the associations.
- A self-association is refused, because its direction cannot be derived. Tick it
  in Studio Pro instead. `describe` prints a tree that holds one as a comment.
- `include associations ()` removes them all. To remove some, restate the tree
  without them. The clause is always the whole tree.
- Left out on `create or modify`, the stored tree is **kept**, and `check -p` and
  `exec` warn with **MDL-ODATA10**. Deleting the clause does not remove the
  associations.
- `describe microflow` prints the clause, so describe → exec restates the tree.
- A non-empty clause needs Mendix 10.11 or later.

## Contract Browsing Statements

Browse available assets from cached service contracts without network access.

| Statement | Description |
|-----------|-------------|
| `LIST CONTRACT ENTITIES FROM Module.Service` | List entity types from cached $metadata |
| `LIST CONTRACT ACTIONS FROM Module.Service` | List actions/functions from cached $metadata |
| `DESCRIBE CONTRACT ENTITY Module.Service.Entity` | Show entity properties, types, keys |
| `DESCRIBE CONTRACT ENTITY Module.Service.Entity FORMAT mdl` | Generate CREATE EXTERNAL ENTITY |
| `DESCRIBE CONTRACT ACTION Module.Service.Action` | Show action parameters and return type |
| `LIST CONTRACT CHANNELS FROM Module.Service` | List channels from cached AsyncAPI |
| `LIST CONTRACT MESSAGES FROM Module.Service` | List messages from cached AsyncAPI |
| `DESCRIBE CONTRACT MESSAGE Module.Service.Message` | Show message payload properties |

## Related Show/Describe Statements

| Statement | Syntax |
|-----------|--------|
| Show consumed odata services | `LIST CONSUMED ODATA SERVICES [IN module]` |
| Show published odata services | `LIST PUBLISHED ODATA SERVICES [IN module]` |
| Show external entities | `LIST EXTERNAL ENTITIES [IN module]` |
| Show external actions | `LIST EXTERNAL ACTIONS [IN module]` |
| Describe consumed odata service | `DESCRIBE CONSUMED ODATA SERVICE Module.Name` |
| Describe published odata service | `DESCRIBE PUBLISHED ODATA SERVICE Module.Name` |
| Describe external entity | `DESCRIBE EXTERNAL ENTITY Module.Name` |

## Catalog Tables

After `REFRESH CATALOG`, the following tables are available for SQL queries:

| Table | Contents |
|-------|----------|
| `CATALOG.CONSUMED_ODATA_SERVICES` | Consumed OData services (old name: `ODATA_CLIENTS`) |
| `CATALOG.PUBLISHED_ODATA_SERVICES` | Published OData services (old name: `ODATA_SERVICES`) |
| `CATALOG.EXTERNAL_ENTITIES` | Imported external entities |
| `CATALOG.EXTERNAL_ACTIONS` | External actions used in microflows |
| `CATALOG.CONTRACT_ENTITIES` | Entity types from cached $metadata |
| `CATALOG.CONTRACT_ACTIONS` | Actions/functions from cached $metadata |
| `CATALOG.CONTRACT_MESSAGES` | Messages from cached AsyncAPI |

### Example: Find available entities not yet imported

```sql
SELECT ce.EntityName, ce.ServiceQualifiedName, ce.PropertyCount
FROM CATALOG.CONTRACT_ENTITIES ce
LEFT JOIN CATALOG.EXTERNAL_ENTITIES ee
  ON ce.ServiceQualifiedName = ee.ServiceName
  AND ce.EntityName = ee.RemoteName
WHERE ee.Id IS NULL;
```
