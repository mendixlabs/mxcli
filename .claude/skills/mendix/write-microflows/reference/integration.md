# REST, SOAP, Java actions and files

Supporting reference for [write-microflows](../SKILL.md).

## Legacy SOAP Web Service Calls

`call web service` preserves legacy Mendix SOAP activities. Prefer REST clients
for new integrations; this syntax exists mainly so existing projects can
round-trip without dropping SOAP actions.

```mdl
-- Structured form. Resolved SOAP references use normal qualified names.
$Root = call web service SampleSOAP.OrderService
operation FetchSampleItems
send mapping SampleSOAP.OrderRequest from $Request
receive mapping SampleSOAP.OrderResponse
timeout 30
on error rollback;

-- Quoted raw IDs are accepted when old project references are dangling or unavailable.
$Root = call web service 'sample-service-id'
operation FetchSampleItems
receive mapping 'sample-receive-mapping-id';

-- Raw escape hatch emitted for unsupported SOAP fields.
$Root = call web service raw 'AQID';
```

### The request body: arguments OR a send mapping, never both

A call stores **one** request body (`Microflows$RequestBodyHandling`), so the two
forms are alternatives. Asking for both is refused as **MDL-SOAP01** by
`mxcli check` and by `exec` — the same function runs in each.

**Arguments** bind the operation's parameters, in the same `(Name = value)` form
every other call statement uses:

```mdl
$Order = call web service Clients.OrderSoapClient
operation GetOrder (OrderId = $Customer/OrderId)
receive mapping Clients.SoapOrdersImportMapping;
```

Mendix stores each one under a `ParameterPath`
(`http%3A//www.example.com/:GetOrder|OrderId`) built from the operation's request
body element. mxcli reads that element off the consumed service document and
builds the path, so the script names only the parameter. Two consequences:

- **The consumed service must be present and declare the operation.** An
  operation mxcli cannot resolve is refused rather than written with a made-up
  path — a wrong path reproduces the same error with different text in it.
- **A misspelled parameter name cannot be caught by `mxcli check`.** The names
  live in the WSDL's inline schema, which mxcli does not parse; the error arrives
  from mxbuild as **CE0178** "Body parameter mapping needs to be refreshed" —
  which is also what you get if you omit arguments an operation requires.

**A send mapping** builds the whole body from an export mapping, and needs the
variable it maps **from**:

```mdl
call web service Clients.OrderSoapClient
operation SaveOrder
send mapping Clients.SoapOrderExportMapping from $NewSaveOrder;
```

`from $var` is not optional. An export mapping maps an object and Mendix stores
which one; without it the call builds as **CE0369** "Cannot use simple request
body, as the operation's body is complex".

### Why DESCRIBE sometimes still shows base64

`describe microflow` renders a SOAP call structurally only when re-executing that
MDL would reproduce the stored document exactly. A call configured beyond what
MDL spells — HTTP authentication, a custom location, SOAP headers, a per-parameter
export mapping, or a result typed from the WSDL rather than from an import
mapping — keeps the `call web service raw '<base64>'` form, which round-trips
byte for byte. That is deliberate: rendering it structurally would silently
normalise the call on the next `exec`.

**Design note:** the raw payload is base64-encoded BSON for the complete action
and is authoritative on re-exec. Treat this as round-trip support, not a
recommended authoring format for new integrations.
## REST Service Calls

MDL supports two patterns for calling REST APIs from microflows:

### SEND REST REQUEST — Consumed REST Service Operations

Calls an operation defined in a consumed REST service (created via `create consumed rest service`). The URL, headers, authentication, and response mapping are configured in the REST client document — the microflow only references the operation.

```mdl
-- Fire and forget (RESPONSE NONE operation)
send rest request Module.ServiceName.OperationName;

-- With output variable (RESPONSE JSON operation — maps to entity)
$Result = send rest request Module.ServiceName.OperationName;

-- With path/query parameters, bound as at every call site: `Param = expression`
$Result = send rest request Module.ServiceName.GetItem
    with (id = $ItemId, lang = 'en');

-- With request body (POST/PUT operations)
$Result = send rest request Module.ServiceName.CreateItem
    body $NewItem;
```

**CRITICAL: `$latestHttpResponse` system variable**

After every `send rest request`, Mendix automatically populates `$latestHttpResponse` (type `System.HttpResponse`). Use this to check call success — do **NOT** check the output variable directly:

```mdl
-- ✅ CORRECT: check $latestHttpResponse
$RootResult = send rest request Module.Service.GetData;
if $latestHttpResponse/Content != empty then
  -- Process $RootResult (the mapped entity)
end if;

-- ❌ WRONG: checking the output variable directly causes CE0117
if $RootResult != empty then  -- ERROR!
```

**Key attributes on `$latestHttpResponse`:**
- `Content` (String) — response body as string. **Capital `C`** — it is inherited from the parent entity `System.HttpMessage`, so `DESCRIBE ENTITY System.HttpResponse` does not list it. Lowercase `$latestHttpResponse/content` fails CE0117.
- `StatusCode` (Integer) — HTTP status code (200, 404, etc.)

**Restrictions:**
- `send rest request` takes a custom error handler, `on error [without rollback] begin … end error`, like any call action. Only `on error continue` is refused (CE6035, MDL076 at check time) — measured on 10.24, 11.12 and 11.16.
- The operation must be defined via `create consumed rest service` with a three-part qualified name: `Module.ServiceDocument.OperationName`.

### REST CALL — Inline HTTP Calls

Direct HTTP call with URL, headers, auth, body, and response handling specified inline. Useful for one-off calls or when no REST client document exists.

```mdl
-- Simple GET returning string
$response = call rest service get 'https://api.example.com/data' (
    Headers: ('Accept': 'application/json'),
    Timeout: 30,
) returns string;

-- POST with JSON body
$response = call rest service post 'https://api.example.com/items' (
    Headers: ('Content-Type': 'application/json', 'Accept': 'application/json'),
    Body: template '{{"name": "{1}", "value": {2}}' with (
        {1} = $ItemName,
        {2} = toString($ItemValue)
    ),
    Timeout: 30,
) returns string
    on error continue;

-- POST a body built elsewhere: an expression sends its VALUE. It is stored as
-- the template '{1}' with the expression as its parameter (what Studio Pro
-- stores, and what describe prints). `Body: template '$Payload'` would send
-- the literal text "$Payload". An expression URL works the same way.
-- An expression followed by `with (…)` is refused (MDL-REST05).
$response = call rest service post @MyModule.BaseUrl + '/items' (
    Headers: ('Content-Type': 'application/json'),
    Body: $Payload,
    Timeout: 30,
) returns string;

-- POST a BINARY body (upload a file document's contents)
-- The expression is the FileDocument's Contents MEMBER, not the document
-- itself, and the content type goes on a header. A consumed REST CLIENT
-- document has no binary body — `Body: file from $Doc` there is refused as
-- MDL-REST02 — so binary uploads belong here.
$response = call rest service post 'https://api.example.com/upload' (
    Headers: ('ContentType': 'application/pdf'),
    Body: binary $Doc/Contents,
    Timeout: 300,
) returns response;

-- GET with URL template parameters
$response = call rest service get 'https://api.example.com/users/{1}' with (
    {1} = toString($UserId)
) (
    Headers: ('Accept': 'application/json'),
) returns string;

-- With basic authentication
$response = call rest service get 'https://api.example.com/secure' (
    Headers: ('Accept': 'application/json'),
    Authentication: basic (Username: $username, Password: $password),
    Timeout: 30,
) returns string;

-- DELETE (no response)
call rest service delete 'https://api.example.com/items/{1}' with (
    {1} = $ItemId
)
    returns nothing
    on error continue;
```

**REST CALL response types:**
- `returns string` — response body as string variable
- `returns nothing` — ignore response (`returns none` is the deprecated second spelling, MDL-DEPR024)
- `returns response` — returns `System.HttpResponse` object
- `returns mapping Module.ImportMapping as Module.Entity` — single object result
- `returns mapping Module.ImportMapping as list of Module.Entity` — list result
- `returns Module.MyFile` — store the body in a **file document**

**The file document form takes a specialization, never `System.FileDocument`
itself.** Mendix rejects the base type as a return type with `CE0362`, and
MDL064 reports it before the write. Create one first:

```mdl
mdl 1;
create persistent entity MyModule.MyFile extends System.FileDocument ();

create microflow MyModule.ACT_Download ($Location: String)
begin
  $file = call rest service get '{1}' with ({1} = $Location) (
    Headers: ('Accept': 'application/octet-stream'),
    Timeout: 300,
  ) returns MyModule.MyFile;
end;
```

There is **no** equivalent for an HttpResponse specialization: Mendix allows only
`User`, `FileDocument`, `Image` and `Paging` to be specialized (`CE1540`), so
`returns response` already names the only type that result can have.

**Pick `as` vs `as list of` based on the call site, not the mapping shape.** The same import mapping can yield either a single object or a list — Studio Pro stores the cardinality on the microflow's `ImportMappingCall` (`Range.SingleObject` + `ForceSingleOccurrence`). Use `as Module.Entity` when the response is a single object (the mapping may still be list-typed; Studio Pro binds the first item). Use `as list of Module.Entity` when the response should bind a list. Mismatching the cardinality with the surrounding code produces `mx check` `CE0117` at the End event or `CE0013` / `CE0100` on downstream loop / aggregate / list-operation activities.

**REST CALL supports full error handling** (`on error continue`, `on error rollback`, custom error handlers).
## Execute Database Query
```mdl
-- Static query (3-part name: Module.Connection.Query)
$Results = execute database query Module.Conn.QueryName;

-- Dynamic SQL override
$Results = execute database query Module.Conn.QueryName
  dynamic 'SELECT * FROM table LIMIT 10';

-- Parameterized query (names must match query PARAMETER definitions)
$Results = execute database query Module.Conn.QueryName
  (paramName = $Variable);

-- Runtime connection override
$Results = execute database query Module.Conn.QueryName
  connection (DBSource = $url, DBUsername = $user, DBPassword = $Pass);

-- Fire-and-forget (no output variable)
execute database query Module.Conn.QueryName;
```
**Note:** Only `on error rollback` is supported (the default). `on error continue` is not available for this action.

## File Downloads

Use `download file` to stream a `System.FileDocument` from a microflow. Add
`show in browser` when the action should open the file inline instead of forcing
a download.

```mdl
download file $GeneratedReport show in browser;
download file $GeneratedExport;
```
## Empty Java-Action Argument (`empty`)

When `describe` round-trips a Java-action call that has an unbound parameter
in Studio Pro, it emits `empty` as the argument value. In this Java-action
argument context, `empty` preserves the
underlying empty `BasicCodeActionParameterValue.Argument` so that the next
`describe → exec → describe` cycle stays symmetric.

```mdl
$Total = call java action SampleModule.Recalculate(
  CompanyId       = empty,
  RecalculateAll  = true,
  ItemList        = empty
);
```

New scripts should bind every parameter to a real expression. Use `empty`
for a Java-action argument only when regenerating MDL from an existing project
that already had an unbound parameter.
## Microflow-Typed Java-Action Parameters

Some Java actions take a **microflow** — a callback the action invokes later.
`MCPServer.AddTool` (`ExecutingMicroflow`) and `MCPServer.CreateMCPServer`
(`AuthenticationMicroflow`) are the ones you meet first. Pass the microflow's
qualified name as a quoted string; mxcli resolves the parameter's declared type
from the Java action and stores a microflow reference, not a string literal.

```mdl
$Tool = call java action MCPServer.AddTool(
  McpServer          = $Server,
  Name               = 'memory_add',
  Description        = 'Stores a memory',
  ExecutingMicroflow = 'MyModule.MF_MemoryAdd',
  Schema             = ''
);
```

`DESCRIBE JAVA ACTION` prints such a parameter's type as the bare word
`Microflow` (`Nanoflow` for JavaScript actions), and that spelling is what
`CREATE JAVA ACTION` accepts, so the round-trip is stable.

## Text-Template and Document Java-Action Parameters

Two more parameter types take something other than an expression. Check the
action's signature with `DESCRIBE JAVA ACTION` first.

A **text template** — `StringTemplate(Text)` or `StringTemplate(Sql)`, such as
`BusinessEvents.PublishBusinessEvent_V2`'s `Subject` — is written like a log
message: a string is the template text, and `with ({1} = …)` fills its
placeholders. Any other expression becomes the template `'{1}'` with that
expression as its argument.

```mdl
call java action BusinessEvents.PublishBusinessEvent_V2(
  Subject       = 'Order {1} shipped ({2} items)' with ({1} = $Order/Number, {2} = $Order/Lines),
  BusinessEvent = $Event
);
```

Each argument is stored with the type of its result, and mxbuild checks the
two match (CE0117). The five types a template takes are String, Integer,
Decimal, Boolean and DateTime. An enumeration is refused, so pass
`getCaption($Order/Status)` or `getKey(…)` instead.

A **document** parameter — `DESCRIBE JAVA ACTION` prints its type as
`Document(<type>)`, for example `Document(agenteditor.agent)` for
AgentEditorCommons' `Agent` — takes the document's qualified name, never an
expression:

```mdl
$Response = call java action AgentEditorCommons.Agent_CallDocument_WithoutHistory(
  Agent                 = MyModule.PaymentReminderAgent,
  OptionalContextObject = $Invoice
);
```

`check --references` reports a missing agent-editor document; mxbuild reports
one as CE1613, and a document of another type as CE0115. `CREATE JAVA ACTION`
cannot declare a document parameter, so `describe` output for such an action
does not re-execute: the parser stops at `Document(…)` instead of rewriting
the parameter as a String.
