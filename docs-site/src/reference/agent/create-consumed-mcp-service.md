# CREATE CONSUMED MCP SERVICE

## Synopsis

```sql
CREATE [ OR MODIFY ] CONSUMED MCP SERVICE module.Name (
    ProtocolVersion: v2025_03_26,
    Endpoint: @module.UrlConstant,
    [ AuthenticationMicroflow: module.GetHeaders, ]
    version: '1.0'
    [, ConnectionTimeoutSeconds: 30 ]
    [, documentation: 'description' ]
);

DROP CONSUMED MCP SERVICE module.Name;
```

Requires Mendix 11.9+.

## Description

Creates a Consumed MCP Service document. A consumed MCP service represents a remote tool server that implements the Model Context Protocol (MCP). Once registered, agents can use the tools provided by the MCP server.

Consumed MCP services are referenced inside `CREATE AGENT` body blocks using the `MCP SERVICE` keyword.

If `OR MODIFY` is specified and the service already exists, its properties are updated in place. The document UUID is preserved.

## Parameters

`module.Name`
:   The qualified name of the consumed MCP service document.

`ProtocolVersion: v2025_03_26`
:   The MCP protocol version. Use the token form (e.g. `v2025_03_26`), not a quoted string.

`Endpoint: @module.UrlConstant`
:   The String constant holding the MCP server's URL (e.g. `https://example.com/mcp`). Required in practice: at startup Agent Editor reads the constant's value and skips a service whose constant has none ("No value could be found for the local configuration for the constant"), so the agents that use it get no tools. `mxcli check` warns on a `CREATE` without it (MDL-AGENT03), `check --references` and `exec` on an `OR MODIFY` that leaves the service without one, and `mxcli lint` on a stored service without one (MPR015); `check --references` and `exec` refuse a constant that does not exist or is not a String. Set the value per environment. On `OR MODIFY`, left out, the stored endpoint stays.

`AuthenticationMicroflow: module.GetHeaders`
:   Optional. The microflow that supplies credentials. MCP Client calls it with no arguments and sends the `List of System.HttpHeader` it returns with each request (build the list with `MCPClient.HttpHeader_CreateAndAddToList`). `check --references` refuses one with parameters or another return type. On `OR MODIFY`, left out, the stored microflow stays; `ALTER … SET AuthenticationMicroflow = ''` removes it.

`version: '1.0'`
:   The service version as a quoted string.

`ConnectionTimeoutSeconds: 30`
:   Optional. Connection timeout in seconds. Defaults to 30.

`documentation: 'description'`
:   Optional. A human-readable description of the MCP service.

## Examples

### Register an MCP service

```sql
mdl 1;
CREATE CONSTANT MyModule.WebSearchUrl (Type: String, DefaultValue: 'https://example.com/mcp');

CREATE CONSUMED MCP SERVICE MyModule."WebSearch" (
    ProtocolVersion: v2025_03_26,
    Endpoint: @MyModule.WebSearchUrl,
    version: '1.0',
    ConnectionTimeoutSeconds: 30,
    documentation: 'Web search MCP server for research tasks'
);

```

### Idempotent upsert

```sql
CREATE OR MODIFY CONSUMED MCP SERVICE MyModule."WebSearch" (
    ProtocolVersion: v2025_03_26,
    version: '1.1',
    ConnectionTimeoutSeconds: 60,
    documentation: 'Updated web search MCP service'
);

```

## See Also

[CREATE AGENT](create-agent.md), [CREATE AI MODEL](create-model.md)
