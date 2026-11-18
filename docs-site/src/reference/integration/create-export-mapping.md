# CREATE EXPORT MAPPING

## Synopsis

```sql
[ /** description */ ]
CREATE [ OR MODIFY ] EXPORT MAPPING module.Name
    [ FOLDER 'folder_path' ]
    [ WITH JSON STRUCTURE module.JsonStructure ]
    [ WITH XML SCHEMA module.XmlSchema ]
    [ NULL VALUES { LeaveOutElement | SendAsNil } ]
{
    module.Entity {
        jsonField = Attribute,
        ...
        [ Assoc/module.ChildEntity AS alias {
            childField = ChildAttr,
            ...
        } ]
    }
};
```

## Description

Creates an export mapping that serialises Mendix entity objects to JSON or XML.

An export mapping is invoked in a microflow using `EXPORT TO MAPPING module.Mapping($Object)`. The result is a JSON or XML string.

### NULL VALUES

Controls how `empty` attribute values are serialised:

| Value | Behaviour |
|-------|-----------|
| `LeaveOutElement` | Omit the JSON key entirely (default) |
| `SendAsNil` | Include the key with a `null` JSON value |

### Nested Objects and Arrays

Child entities are nested using the association path followed by `AS alias`:

```
Assoc/module.ChildEntity AS alias { ... }
```

For arrays, Studio Pro's domain model convention is to use an intermediate container entity (no attributes) between the root and the item entity. See the example below.

### OR MODIFY

If `OR MODIFY` is specified and the mapping already exists, it is updated in place. The document UUID is preserved, which protects runtime microflow references that call `EXPORT TO MAPPING`.

## Parameters

`module.Name`
:   The qualified name of the export mapping.

`FOLDER 'folder_path'`
:   Optional. Places the document in the named module folder, creating missing folders in the path. On `CREATE OR MODIFY` this **moves** an existing document; omitting the clause leaves placement alone rather than returning the document to the module root. See [MOVE](../organization/move.md).

`/** description */`
:   Optional. The documentation of the export mapping, as a doc comment before the statement. `lint` QUAL002 reports a export mapping without it, and `DESCRIBE` prints it back. On `CREATE OR MODIFY`, no doc comment keeps the stored documentation and an empty `/** */` clears it.

`WITH JSON STRUCTURE module.JsonStructure`
:   Associates the mapping with the named JSON structure.

`WITH XML SCHEMA module.XmlSchema`
:   Associates the mapping with the named XML schema.

    The reference is **resolved** against the project: a name that matches no XML
    schema is refused by `mxcli check -p` and by `exec`, naming the schemas that
    exist. mxbuild otherwise reports it as CE1613 "The selected XML schema 'X' no
    longer exists" at the end of a build.

    A project holding **no** XML schemas disables the check rather than refusing
    every mapping — there is no `CREATE XML SCHEMA` in MDL, so an XML schema is
    only ever something imported into the project by hand, and having none is the
    ordinary case. The same resolution applies to `WITH JSON STRUCTURE`.

    MDL cannot state the XML half of such a mapping — the schema element it is
    rooted at, or the schema path of each element — and mxcli does not read the
    XSD to derive them. So `CREATE OR MODIFY` of an existing XML-schema mapping
    that carries them is accepted only when the statement is the mapping's own
    `DESCRIBE` output, which reports `Unchanged` and writes nothing; any other
    rewrite is refused rather than written without its root element (which
    mxbuild reports as CE0270 "No root element could be found in the schema").
    Edit such a mapping in Studio Pro.

`NULL VALUES LeaveOutElement | SendAsNil`
:   Controls null serialisation. Defaults to `LeaveOutElement`.

`jsonField = Attribute`
:   Maps entity attribute `Attribute` to the JSON key `jsonField`.

`Assoc/module.ChildEntity AS alias`
:   Maps associated objects to a nested JSON object or array using the named alias.
    The association must connect the entity the element sits under to the element's own entity (in either direction; an end may be a generalization of either entity). Any other association is refused by `check` (MDL100) because Mendix reports CE0295 "Association '...' is not allowed".

## Examples

### Simple flat mapping

```sql
CREATE EXPORT MAPPING MyModule.EMM_Pet
    WITH JSON STRUCTURE MyModule.JSON_Pet
{
    MyModule.PetResponse {
        id     = PetId,
        name   = Name,
        status = Status
    }
};
```

### With null values option

```sql
CREATE EXPORT MAPPING MyModule.EMM_PetNullable
    WITH JSON STRUCTURE MyModule.JSON_Pet
    NULL VALUES SendAsNil
{
    MyModule.PetResponse {
        id     = PetId,
        name   = Name,
        status = Status
    }
};
```

### Nested object with array (container + item pattern)

```sql
-- Domain model for export:
--   ExRoot → ExCustomer (object), ExRoot → ExItems (container) → ExItemsItem (array item)

CREATE EXPORT MAPPING MyModule.EMM_Order
    WITH JSON STRUCTURE MyModule.JSON_Order
{
    MyModule.ExRoot {
        orderId = OrderId,
        MyModule.ExCustomer_ExRoot/MyModule.ExCustomer AS customer {
            name  = Name,
            email = Email
        },
        MyModule.ExItems_ExRoot/MyModule.ExItems AS items {
            MyModule.ExItemsItem_ExItems/MyModule.ExItemsItem AS ItemsItem {
                sku      = Sku,
                quantity = Quantity,
                price    = Price
            }
        }
    }
};
```

### Idempotent update

```sql
CREATE OR MODIFY EXPORT MAPPING MyModule.EMM_Pet
    WITH JSON STRUCTURE MyModule.JSON_Pet
{
    MyModule.PetResponse {
        id     = PetId,
        name   = Name,
        status = Status
    }
};
```

## Notes

- Export mappings use a different entity structure than import mappings for the same JSON. Arrays in export require an intermediate container entity (no attributes) between the root entity and the array item entity.
- Import and export mappings cannot share the same domain model for nested JSON because the FK direction and entity structure differ.

## See Also

[CREATE JSON STRUCTURE](create-json-structure.md), [CREATE IMPORT MAPPING](create-import-mapping.md)
