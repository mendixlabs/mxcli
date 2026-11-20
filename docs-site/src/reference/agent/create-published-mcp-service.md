# CREATE PUBLISHED MCP SERVICE

## Synopsis

```text
CREATE [ OR MODIFY ] PUBLISHED MCP SERVICE module.Name [ FOLDER 'path' ] (
    [ ServerName: 'name', ]
    EndpointPath: 'path',
    Version: '1.0.0',
    ProtocolVersion: v2025_03_26
    [, AuthenticationEnabled: true ]
    [, AuthenticationMicroflow: module.Microflow ]
    [, AuthorizationServerUrl: 'https://...' ]
    [, Scopes: 'scope another-scope' ]
    [, Documentation: 'description' ]
) [ {
    tool name ( Microflow: module.Microflow [, Description: '...'] [, Title: '...'] [, Schema: '...'] )
    ...
} ];

ALTER PUBLISHED MCP SERVICE module.Name
    { SET key = value [, ...] | ADD TOOL name ( ... ) | DROP TOOL name } ...;

DESCRIBE PUBLISHED MCP SERVICE module.Name;
LIST PUBLISHED MCP SERVICES [ IN module ];
DROP PUBLISHED MCP SERVICE module.Name;
```

Requires Agent Editor 2.6+ and the MCP Server module 5.x (measured on Studio Pro 11.16 with Agent Editor 2.6.0 and MCP Server 5.4.0).

## Description

Creates a Published MCP Service document: an MCP server the app itself publishes, so external agents (Claude Desktop, other apps) can call its microflows as tools. This is the *model configuration* of an MCP server. At startup AgentEditorCommons reads every published MCP service document and starts it through `MCPServer.CreateMCPServer` and `MCPServer.AddTool`, so each property is one of those actions' parameters.

The *runtime configuration* is the alternative: a startup microflow that calls the same Java actions, which is the only option before MCP Server 5. It is plain microflow logic, written with `call java action` (see the `mcp-server` skill). The two map one to one, which is what a migration between them follows. Prompts exist only in runtime configuration (`MCPServer.AddPrompt`); the document has no place for them.

If `OR MODIFY` is specified and the service exists, it is rewritten in place: a tool keeps its stored `uuid` when its name matches, and keys Agent Editor stores that MDL does not model are kept.

## Parameters

`module.Name`
:   The document name.

`ServerName: 'name'`
:   Optional. The server name MCP clients see. Studio Pro stores it but its editor does not show it: a new document gets the document name, and a duplicated one keeps the original's (`Published_MCP_service_2` serves as `Published_MCP_service`). Omitted, `create` writes the document name and `create or modify` keeps the stored value; `describe` prints it only when it differs from the document name.

`EndpointPath: 'path'`
:   The middle segment of the server's URL, which is `<app URL>/<EndpointPath>/mcp`: the module appends `/mcp`, as the editor's Endpoint field shows (`https://{{applicationURL}}/` + path + `/mcp`). It cannot be `mcp`, end in `/mcp`, or contain `/` (the runtime matches only the first path segment): `check` reports each as MDL-MCPSERVER01.

`Version: '1.0.0'`
:   The server version reported to clients.

`ProtocolVersion: v2025_03_26`
:   The MCP protocol version, a value of the module's `MCPServer.ENUM_ProtocolVersion` (MCP Server 5.4 has only `v2025_03_26`).

`AuthenticationEnabled: true`
:   The editor's *Requires authentication*. Turns authentication on. While it is false, the runtime ignores the three properties below.

`AuthenticationMicroflow: module.Microflow`
:   Authenticates every request. It takes `System.HttpRequest` and/or `MCPServer.McpServer`, returns a `System.User` (or a specialization), and returns empty to refuse the request with 401. Without it, an enabled service accepts every request.

`AuthorizationServerUrl: 'https://...'`
:   The issuer URL of the OAuth authorization server. The server publishes it as Protected Resource Metadata so clients can discover where to sign in. Needs an authentication microflow, which must validate the access token; must be an absolute http(s) URL.

`Scopes: 'a b'`
:   Space-separated OAuth scopes clients must request. Needs an authorization server URL.

`Documentation: 'description'`
:   A description of the service.

`tool name ( ... )`
:   A microflow exposed as an MCP tool, named as clients see it. `Microflow` is required; `Description` is what the calling model reads; `Title` is a display name; `Schema` overrides the JSON input schema the module otherwise derives from the microflow's parameters. A tool microflow takes primitives (named as the schema's properties) and optionally `MCPServer.Tool`, and returns a `String` or an `MCPServer.TextContent`. The editor's tool grid shows Name, Description and Source (the microflow); `Title` and `Schema` are not in it, but the runtime reads both.

## Examples

### Publish two microflows

Served at `<app URL>/orders/mcp`.

```sql
CREATE PUBLISHED MCP SERVICE Shop.OrdersMCP (
    EndpointPath: 'orders',
    Version: '1.0.0',
    ProtocolVersion: v2025_03_26,
    Documentation: 'Order tools for agents'
) {
    tool get_status ( Microflow: Shop.MCPTool_GetStatus, Description: 'Status of one order', Title: 'Get status' )
    tool list_orders ( Microflow: Shop.MCPTool_ListOrders, Description: 'List open orders' )
};
```

### OAuth with Microsoft Entra ID

```sql
CREATE OR MODIFY PUBLISHED MCP SERVICE Shop.OrdersMCP (
    EndpointPath: 'orders',
    Version: '1.0.0',
    ProtocolVersion: v2025_03_26,
    AuthenticationEnabled: true,
    AuthenticationMicroflow: Shop.MCP_ValidateToken,
    AuthorizationServerUrl: 'https://login.microsoftonline.com/my-tenant-id/v2.0',
    Scopes: 'api://orders/mcp.read'
) {
    tool get_status ( Microflow: Shop.MCPTool_GetStatus, Description: 'Status of one order' )
};
```

### Change one property, add and drop a tool

```sql
ALTER PUBLISHED MCP SERVICE Shop.OrdersMCP
    SET Version = '1.1.0'
    ADD TOOL cancel_order ( Microflow: Shop.MCPTool_Cancel, Description: 'Cancel an order' )
    DROP TOOL list_orders;
```

## See Also

[CREATE CONSUMED MCP SERVICE](create-consumed-mcp-service.md), [CREATE AGENT](create-agent.md)
