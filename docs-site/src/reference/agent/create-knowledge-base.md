# CREATE KNOWLEDGE BASE

## Synopsis

```sql
CREATE [ OR MODIFY ] KNOWLEDGE BASE module.Name (
    Provider: MxCloudGenAI,
    key: @module.KeyConstant
);

DROP KNOWLEDGE BASE module.Name;
```

Requires Mendix 11.9+.

## Description

Creates an agent-editor Knowledge Base document. A knowledge base connects an agent to a vector knowledge base for retrieval-augmented generation (RAG). It references a String constant that holds the Mendix Cloud GenAI Portal resource key for the knowledge base.

Knowledge bases are used inside `CREATE AGENT` body blocks to attach retrieval capabilities to an agent.

If `OR MODIFY` is specified and the knowledge base already exists, its properties are updated in place. The document UUID is preserved.

## Parameters

`module.Name`
:   The qualified name of the knowledge base document.

`Provider: MxCloudGenAI`
:   The provider. `MxCloudGenAI` is the only supported value; `mxcli check` warns on any other (MDL-AGENT01).

`key: module.KeyConstant`
:   A qualified reference to a String constant that holds the Portal resource key for the knowledge base.

## Examples

### Create a knowledge base

```sql
mdl 1;
CREATE CONSTANT MyModule."KBKey"
    ( Type: String, DefaultValue: '' );

CREATE KNOWLEDGE BASE MyModule."ProductDocs" (
    Provider: MxCloudGenAI,
    key: @MyModule.KBKey
);

```

### Idempotent upsert

```sql
CREATE OR MODIFY KNOWLEDGE BASE MyModule."ProductDocs" (
    Provider: MxCloudGenAI,
    key: @MyModule.KBKey
);

```

## See Also

[CREATE AGENT](create-agent.md), [CREATE AI MODEL](create-model.md)
