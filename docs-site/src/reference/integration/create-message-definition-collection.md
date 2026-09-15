# CREATE MESSAGE DEFINITION COLLECTION

## Synopsis

```sql
CREATE [ OR MODIFY ] MESSAGE DEFINITION COLLECTION module.Name
    [ FOLDER 'folder_path' ]
(
    DEFINITION DefName FOR module.Entity [ AS 'ExposedName' ] (
        AttributeName [ AS 'ExposedName' ] [ EXAMPLE 'text' ],
        module.Association/module.TargetEntity [ AS 'ExposedName' ] ( ... ),
        ...
    ),
    ...
);
```

## Description

Creates a message definition collection — one of the four sources an import or
export mapping can be bound to, and the only one besides a JSON structure that
MDL can create.

Unlike an XML schema (which holds an imported `.xsd`) or an imported web service
(which holds a WSDL), a message definition holds nothing external. It is a
**selection over the domain model**: every element names an entity, an attribute
or an association. A mapping then binds to `module.Collection.Definition`.

### Members

A **bare name** is an attribute. An **`Association/TargetEntity`** pair is an
association with its own member list — the same discriminator import and export
mappings use.

Naming the association's target entity is required, and not decoration. The
stored cardinality is derived from the **direction of traversal** and the
**association's type** together:

| traversal | `Reference` | `ReferenceSet` |
|---|---|---|
| from the association's FROM entity (following the reference) | a single object | a list |
| from its TO entity (the reverse) | a list | a list |

So a `Reference` gives a single object one way and a list the other, while a
`ReferenceSet` is a list in both directions — a set is many at both ends.

An association that connects the two entities in **neither** direction is
refused rather than guessed at. The two halves of the rule fail differently if
you do get one wrong, which is useful when diagnosing: a wrong *direction*
builds cleanly and silently exposes a list as a single object, while a wrong
*type* is caught by mxbuild as CE6524 on the definition ("The occurrence of
'...' has changed") plus CE0295 on any mapping element bound to it.

**Inherited attributes** are named exactly like the entity's own; mxcli resolves
each to the entity that declares it, which is what Mendix stores.

### What is derived

Almost everything. A statement says only what a person chooses — the collection's
name, each definition's name, its root entity, the members, and the occasional
rename. Occurrence bounds, nillability, precision, element types, paths, item
names and the primitive type all come from the domain model.

`EXAMPLE 'text'` sets an element's sample value, the one other authored field.

### What is not guessed

Studio Pro **pluralises** a repeating element's exposed name (`Order` →
`Orders`). mxcli defaults to the entity's own name and lets `AS 'Orders'` say
otherwise: reproducing English inflection needs `-y → -ies` and an
already-plural detector, and a name the author writes beats one a heuristic
guesses.

## Examples

```sql
CREATE MESSAGE DEFINITION COLLECTION Sales.MD_Order
    FOLDER 'Messages'
(
    DEFINITION OrderMessage FOR Sales.Order AS 'Orders' (
        OrderId,
        TotalAmount AS 'Total',
        Sales.OrderLine_Order/Sales.OrderLine AS 'Lines' ( Sku, Quantity ),
        Sales.Order_Customer/Sales.Customer ( FirstName, LastName )
    ),
    DEFINITION CustomerOrders FOR Sales.Customer AS 'Customers' (
        FirstName,
        Sales.Order_Customer/Sales.Order AS 'Orders' ( OrderId )
    )
);

CREATE IMPORT MAPPING Sales.IMM_Order
    WITH MESSAGE DEFINITION Sales.MD_Order.OrderMessage
{ create Sales.Order { OrderId = OrderId } };
```

## Editing without restating

Definitions nest deeply, so a whole-document rewrite is a poor tool for "expose
one more attribute".

```sql
ALTER MESSAGE DEFINITION Sales.MD_Order.OrderMessage ADD MEMBER TotalAmount;
ALTER MESSAGE DEFINITION Sales.MD_Order.OrderMessage ADD MEMBER LastName IN Customer;
ALTER MESSAGE DEFINITION Sales.MD_Order.OrderMessage SET MEMBER TotalAmount AS 'GrandTotal';
ALTER MESSAGE DEFINITION Sales.MD_Order.OrderMessage DROP MEMBER Sku IN Lines;

ALTER MESSAGE DEFINITION COLLECTION Sales.MD_Order ADD DEFINITION Line FOR Sales.Line ( Sku );
ALTER MESSAGE DEFINITION COLLECTION Sales.MD_Order RENAME DEFINITION Line TO OrderLine;
ALTER MESSAGE DEFINITION COLLECTION Sales.MD_Order DROP DEFINITION IF EXISTS OrderLine;
```

The definition is addressed as `module.Collection.Definition` — the same
three-part reference `WITH MESSAGE DEFINITION` takes.

`IN <path>` reaches a nested member, written in **exposed names**. `SET` changes
only the exposed name; it is not a model rename, which is why the verb is not
`RENAME`.

Dropping or renaming a definition a mapping still references is refused, naming
the mappings.

## Other statements

```sql
SHOW MESSAGE DEFINITION COLLECTIONS [ IN module ];
DESCRIBE MESSAGE DEFINITION COLLECTION module.Name;
DROP MESSAGE DEFINITION COLLECTION module.Name;
```

`DESCRIBE` emits re-executable MDL. `OR MODIFY` preserves the document UUID, so
mappings bound to it keep resolving.

## See Also

[CREATE IMPORT MAPPING](create-import-mapping.md), [CREATE EXPORT MAPPING](create-export-mapping.md), [CREATE JSON STRUCTURE](create-json-structure.md)
