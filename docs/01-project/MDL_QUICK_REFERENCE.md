# MDL Syntax Quick Reference

Complete syntax reference for MDL (Mendix Definition Language). This is the authoritative reference for all MDL statement syntax.

For task-specific guidance, see the skill files listed in [CLAUDE.md](../CLAUDE.md#important-before-writing-mdl-scripts-or-working-with-data).

## Entity Generalization (EXTENDS)

**CRITICAL: EXTENDS goes BEFORE the opening parenthesis, not after!**

```sql
-- Correct: EXTENDS before (
create persistent entity Module.ProductPhoto extends System.Image (
  PhotoCaption: string(200)
);

-- Wrong: EXTENDS after ) = parse error!
create persistent entity Module.Photo (
  PhotoCaption: string(200)
) extends System.Image;
```

## Modules

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show modules | `show modules;` | List all modules |
| Describe module | `describe module ModuleName;` | All contents (entities, microflows, pages, etc.) |
| Create module | `create module ModuleName;` | |
| Drop module | `drop module ModuleName;` | |
| Rename module | `rename module OldName to NewName;` | Updates all qualified name references |

## Domain Model

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Create entity | `create [or modify] persistent\|non-persistent entity Module.Name (attrs);` | Persistent is default |
| Create with extends | `create persistent entity Module.Name extends Parent.Entity (attrs);` | EXTENDS before `(` |
| Create with auditing | `create persistent entity Module.Name (attrs, owner: autoowner, ChangedBy: autochangedby, CreatedDate: autocreateddate, ChangedDate: autochangeddate);` | Pseudo-types like AutoNumber |
| Create view entity | `create view entity Module.Name (attrs) as select ...;` | OQL-backed read-only |
| Create external entity | `create external entity Module.Name from odata client Module.Client (...) (attrs);` | From consumed OData |
| Create external entities | `create [or modify] external entities from Module.Client [into module] [entities (...)];` | Bulk from $metadata |
| Drop entity | `drop entity Module.Name;` | |
| Describe entity | `describe entity Module.Name;` | Full MDL output |
| Describe enumeration | `describe enumeration Module.Name;` | Full MDL output |
| Rename entity | `rename entity Module.Old to New;` | Updates all references |
| Rename enumeration | `rename enumeration Module.Old to New;` | Updates attribute type refs |
| Rename association | `rename association Module.Old to New;` | Updates all references |
| Show entities | `show entities [in module];` | List all or filter by module |
| Create enumeration | `create [or modify] enumeration Module.Name (Value1 'caption', ...);` | |
| Drop enumeration | `drop enumeration Module.Name;` | |
| Create association | `create association Module.Name from Parent to Child type reference\|ReferenceSet [owner default\|both] [delete_behavior ...];` | |
| Drop association | `drop association Module.Name;` | |

## ALTER ENTITY

Modifies an existing entity without full replacement.

| Operation | Syntax | Notes |
|-----------|--------|-------|
| Add attributes | `alter entity Module.Name add (attr: type [constraints]);` | One or more attributes |
| Drop attributes | `alter entity Module.Name drop (AttrName, ...);` | |
| Modify attributes | `alter entity Module.Name modify (attr: NewType [constraints]);` | Change type/constraints |
| Rename attribute | `alter entity Module.Name rename OldName to NewName;` | |
| Add index | `alter entity Module.Name add index (Col1 [asc\|desc], ...);` | |
| Drop index | `alter entity Module.Name drop index (Col1, ...);` | |
| Add event handler | `alter entity Module.Name add event handler on before commit call Mod.MF($currentObject) [raise error];` | `($currentObject)` or `()`, RAISE ERROR only on BEFORE |
| Drop event handler | `alter entity Module.Name drop event handler on before commit;` | |
| Set documentation | `alter entity Module.Name set documentation 'text';` | |
| Set position | `alter entity Module.Name set position (100, 200);` | Canvas position |
| Add system attribute | `alter entity Module.Name add attribute owner: autoowner;` | Same syntax as regular attributes |
| Drop system attribute | `alter entity Module.Name drop attribute owner;` | Drop by system attribute name |

**Example:**
```sql
alter entity Sales.Customer
  add (Phone: string(50), Notes: string(unlimited));

alter entity Sales.Customer
  rename Phone to PhoneNumber;

alter entity Sales.Customer
  add index (Email);
```

## Constants

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show constants | `show constants [in module];` | List all or filter by module |
| Show constant values | `show constant values [in module];` | Compare values across configurations |
| Describe constant | `describe constant Module.Name;` | Full MDL output |
| Create constant | `create [or modify] constant Module.Name type DataType default 'value';` | String, Integer, Boolean, etc. |
| Drop constant | `drop constant Module.Name;` | |

**Example:**
```sql
create constant MyModule.ApiBaseUrl type string default 'https://api.example.com';
create constant MyModule.MaxRetries type integer default 3;
create constant MyModule.EnableLogging type boolean default true;
```

## OData Clients, Services & External Entities

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show OData clients | `show odata clients [in module];` | Consumed OData services |
| Describe OData client | `describe odata client Module.Name;` | Full MDL output |
| Create OData client | `create [or modify] odata client Module.Name (...);` | Version, MetadataUrl, Timeout, etc. |
| Alter OData client | `alter odata client Module.Name set key = value;` | |
| Drop OData client | `drop odata client Module.Name;` | |
| Show OData services | `show odata services [in module];` | Published OData services |
| Describe OData service | `describe odata service Module.Name;` | Full MDL output |
| Create OData service | `create [or modify] odata service Module.Name (...) authentication ... { publish entity ... };` | |
| Alter OData service | `alter odata service Module.Name set key = value;` | |
| Drop OData service | `drop odata service Module.Name;` | |
| Show external entities | `show external entities [in module];` | OData-backed entities |
| Show external actions | `show external actions [in module];` | Actions used in microflows |
| Create external entity | `create [or modify] external entity Module.Name from odata client Module.Client (...) (attrs);` | |
| Create external entities | `create [or modify] external entities from Module.Client [into module] [entities (...)];` | Bulk from $metadata |
| Grant OData access | `grant access on odata service Module.Name to Module.Role, ...;` | |
| Revoke OData access | `revoke access on odata service Module.Name from Module.Role, ...;` | |
| Show contract entities | `show contract entities from Module.Client;` | Browse cached $metadata |
| Show contract actions | `show contract actions from Module.Client;` | Browse cached $metadata |
| Describe contract entity | `describe contract entity Module.Client.Entity [format mdl];` | Properties, types, keys |
| Describe contract action | `describe contract action Module.Client.Action [format mdl];` | Parameters, return type |
| Show contract channels | `show contract channels from Module.Service;` | Browse cached AsyncAPI |
| Show contract messages | `show contract messages from Module.Service;` | Browse cached AsyncAPI |
| Describe contract message | `describe contract message Module.Service.Message;` | Message payload properties |
| Query contract entities | `select * from CATALOG.CONTRACT_ENTITIES;` | Requires REFRESH CATALOG |
| Query contract actions | `select * from CATALOG.CONTRACT_ACTIONS;` | Requires REFRESH CATALOG |
| Query contract messages | `select * from CATALOG.CONTRACT_MESSAGES;` | Requires REFRESH CATALOG |

**OData Client Example:**
```sql
-- HTTP(S) URL (fetches metadata from remote service)
create odata client MyModule.ExternalAPI (
  Version: '1.0',
  ODataVersion: OData4,
  MetadataUrl: 'https://api.example.com/odata/v4/$metadata',
  timeout: 300
);

-- Local file with absolute file:// URI
CREATE ODATA CLIENT MyModule.LocalService (
  Version: '1.0',
  ODataVersion: OData4,
  MetadataUrl: 'file:///path/to/metadata.xml',
  Timeout: 300
);

-- Local file with relative path (normalized to absolute file:// in model)
CREATE ODATA CLIENT MyModule.LocalService2 (
  Version: '1.0',
  ODataVersion: OData4,
  MetadataUrl: './metadata/service.xml',
  Timeout: 300,
  ServiceUrl: '@MyModule.ServiceLocation'  -- Must be a constant reference
);
```

**Note:** `MetadataUrl` supports three formats:
- `https://...` or `http://...` — fetches from HTTP(S) endpoint
- `file:///abs/path` — reads from local absolute path
- `./path` or `path/file.xml` — reads from local relative path, **normalized to absolute `file://` in the model** for Studio Pro compatibility

**Important:** `ServiceUrl` must always be a constant reference starting with `@` (e.g., `@Module.ConstantName`). Create a constant first:
```sql
CREATE CONSTANT MyModule.ServiceLocation TYPE String DEFAULT 'https://api.example.com/odata/v4/';
```

**OData Service Example:**
```sql
create odata service MyModule.CustomerAPI (
  path: '/odata/customers',
  version: '1.0.0',
  ODataVersion: OData4,
  namespace: 'MyModule.Customers'
)
authentication basic, session
{
  publish entity MyModule.Customer as 'Customers' (
    ReadMode: source,
    InsertMode: source,
    UpdateMode: not_supported,
    DeleteMode: not_supported,
    UsePaging: Yes,
    PageSize: 100
  )
  expose (Name, Email, Phone);
};
```

## Microflows & Nanoflows

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show microflows | `show microflows [in module];` | List all or filter by module |
| Show nanoflows | `show nanoflows [in module];` | List all or filter by module |
| Describe microflow | `describe microflow Module.Name;` | Full MDL with activities |
| Describe nanoflow | `describe nanoflow Module.Name;` | Full MDL with activities |
| Rename microflow | `rename microflow Module.Old to New;` | Updates all references |
| Rename nanoflow | `rename nanoflow Module.Old to New;` | Updates all references |
| Rename page | `rename page Module.Old to New;` | Updates all references |
| Rename constant | `rename constant Module.Old to New;` | Updates all references |
| Drop microflow | `drop microflow Module.Name;` | |
| Drop nanoflow | `drop nanoflow Module.Name;` | |
| Create nanoflow | `create [or modify] nanoflow Module.Name (params) returns type [folder 'path'] begin ... end;` | Same body syntax as microflows |
| Move nanoflow | `move nanoflow Module.Name to folder 'path';` | |
| Nanoflow restrictions | N/A | No Java actions, ErrorEvent, REST calls, database queries, external actions, download file, workflow actions, import/export mappings, JSON transformation, show home page |

## Microflows - Supported Statements

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Variable declaration | `declare $Var type = value;` | Primitives: String, Integer, Boolean, Decimal, DateTime |
| Entity declaration | `declare $entity Module.Entity;` | No AS keyword, no = empty |
| List declaration | `declare $list list of Module.Entity = empty;` | |
| Assignment | `set $Var = expression;` | Variable must be declared first |
| Create object | `$Var = create Module.Entity (attr = value);` | |
| Change object | `change $entity (attr = value);` | |
| Commit | `commit $entity [with events] [refresh];` | |
| Delete | `delete $entity;` | |
| Rollback | `rollback $entity [refresh];` | Reverts uncommitted changes |
| Retrieve (DB) | `retrieve $Var from Module.Entity [where condition];` | Database XPath retrieve |
| Retrieve (Assoc) | `retrieve $list from $Parent/Module.AssocName;` | Retrieve by association |
| Call microflow | `$Result = call microflow Module.Name (Param = $value);` | |
| Call nanoflow | `$Result = call nanoflow Module.Name (Param = $value);` | |
| Call JS action | `$Result = call javascript action Module.Name (Param = $value);` | JavaScript action (nanoflow/microflow) |
| Call Java action | `$Result = call java action Module.Name (Param = $value);` | Java action (microflow only) |
| Show page | `show page Module.PageName ($Param = $value);` | Also accepts `(Param: $value)` |
| Close page | `close page;` | |
| Download file | `download file $FileDocument [show in browser];` | Streams a `System.FileDocument` |
| Validation | `validation feedback $entity/attribute message 'message';` | Requires attribute path + MESSAGE |
| Log | `log info\|warning\|error [node 'name'] 'message';` | |
| Position | `@position(x, y)` | Canvas position (before activity) |
| Caption | `@caption 'text'` | Custom caption (before activity) |
| Color | `@color Green` | Background color (before activity) |
| Annotation | `@annotation 'text'` | Visual note attached to next activity |
| IF | `if condition then ... [else ...] end if;` | |
| LOOP | `loop $item in $list begin ... end loop;` | FOR EACH over list |
| WHILE | `while condition begin ... end while;` | Condition-based loop |
| Return | `return $value;` | Required at end of every flow path |
| Execute DB query | `$Result = execute database query Module.Conn.Query;` | 3-part name; supports DYNAMIC, params, CONNECTION override |
| Import mapping | `[$Var =] import from mapping Module.IMM($SourceVar);` | Apply import mapping to string variable |
| Export mapping | `$Var = export to mapping Module.EMM($EntityVar);` | Apply export mapping to entity, returns string |
| Error handling | `... on error continue\|rollback\|{ handler };` | Not supported on EXECUTE DATABASE QUERY |

## Microflows - NOT Supported (Will Cause Parse Errors)

| Unsupported | Use Instead | Notes |
|-------------|-------------|-------|
| `case ... when ... end case` | Nested `if ... else ... end if` | Switch not implemented |
| `TRY ... CATCH ... end TRY` | `on error { ... }` blocks | Use error handlers on specific activities |

**Notes:**
- `retrieve ... limit n` IS supported. `limit 1` returns a single entity, otherwise returns a list.
- `rollback $entity [refresh];` IS supported. Rolls back uncommitted changes to an object.

## Project Organization

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Microflow folder | `folder 'path'` (before BEGIN) | `create microflow ... folder 'ACT' begin ... end;` |
| Page folder | `folder: 'path'` (in properties) | `create page ... (folder: 'pages/Detail') { ... }` |
| Drop folder | `drop folder 'path' in module;` | Folder must be empty |
| Move folder | `move folder Module.FolderName to folder 'path';` | Target folders auto-created |
| Move to folder | `move page\|microflow\|snippet\|nanoflow\|enumeration Module.Name to folder 'path';` | Folders created automatically |
| Move to module root | `move page Module.Name to module;` | Removes from folder |
| Move across modules | `move page Old.Name to NewModule;` | **Breaks by-name references** — use `show impact of` first |
| Move to folder in other module | `move page Old.Name to folder 'path' in NewModule;` | |
| Move entity to module | `move entity Old.Name to NewModule;` | Entities don't support folders |

Nested folders use `/` separator: `'Parent/Child/Grandchild'`. Missing folders are auto-created.

## Security Management

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show project security | `show project security;` | Displays security level, admin, demo users |
| Show module roles | `show module roles [in module];` | All roles or filtered by module |
| Show user roles | `show user roles;` | Project-level user roles |
| Show demo users | `show demo users;` | Configured demo users |
| Show access on element | `show access on microflow\|nanoflow\|page\|entity Mod.Name;` | Which roles can access |
| Show security matrix | `show security matrix [in module];` | Full access overview |
| Create module role | `create module role Mod.Role [description 'text'];` | |
| Drop module role | `drop module role Mod.Role;` | |
| Create user role | `create user role Name (Mod.Role, ...) [manage all roles];` | Aggregates module roles |
| Alter user role | `alter user role Name add\|remove module roles (Mod.Role, ...);` | |
| Drop user role | `drop user role Name;` | |
| Grant microflow access | `grant execute on microflow Mod.MF to Mod.Role, ...;` | |
| Revoke microflow access | `revoke execute on microflow Mod.MF from Mod.Role, ...;` | |
| Grant nanoflow access | `grant execute on nanoflow Mod.NF to Mod.Role, ...;` | |
| Revoke nanoflow access | `revoke execute on nanoflow Mod.NF from Mod.Role, ...;` | |
| Grant page access | `grant view on page Mod.Page to Mod.Role, ...;` | |
| Revoke page access | `revoke view on page Mod.Page from Mod.Role, ...;` | |
| Grant entity access | `grant Mod.Role on Mod.Entity (create, delete, read *, write *);` | Additive — merges with existing |
| Revoke entity access | `revoke Mod.Role on Mod.Entity;` | Full revoke — removes entire rule |
| Revoke entity access (partial) | `revoke Mod.Role on Mod.Entity (read (attr));` | Partial — downgrades specific rights |
| Set security level | `alter project security level off\|prototype\|production;` | |
| Toggle demo users | `alter project security demo users on\|off;` | |
| Create demo user | `create demo user 'name' password 'pass' [entity Module.Entity] (UserRole, ...);` | |
| Drop demo user | `drop demo user 'name';` | |

## Workflows

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show workflows | `show workflows [in module];` | List all or filter by module |
| Describe workflow | `describe workflow Module.Name;` | Full MDL output |
| Create workflow | `create [or modify] workflow Module.Name parameter $Ctx: Module.Entity begin ... end workflow;` | See activity types below |
| Drop workflow | `drop workflow Module.Name;` | |

**Workflow Activity Types:**
- `user task <name> '<caption>' [page Mod.Page] [targeting [users|groups] microflow Mod.MF] [targeting [users|groups] xpath '<expr>'] [outcomes '<out>' { } ...];`
- `call microflow Mod.MF [comment '<text>'] [outcomes '<out>' { } ...];`
- `call workflow Mod.WF [comment '<text>'];`
- `decision ['<caption>'] outcomes '<out>' { } ...;`
- `parallel split path 1 { } path 2 { };`
- `jump to <activity-name>;`
- `wait for timer ['<expr>'];`
- `wait for notification;`
- `end;`

**Example:**
```sql
create workflow Module.ApprovalFlow
  parameter $context: Module.Request
  overview page Module.WorkflowOverview
begin
  user task ReviewTask 'Review the request'
    page Module.ReviewPage
    outcomes 'Approve' { } 'Reject' { };
end workflow;
```

## ALTER WORKFLOW

Modify an existing workflow's properties, activities, outcomes, paths, conditions, and boundary events without full replacement.

| Operation | Syntax | Notes |
|-----------|--------|-------|
| Set display name | `set display 'name'` | Workflow-level display name |
| Set description | `set description 'text'` | Workflow-level description |
| Set export level | `set export level api\|Hidden` | Visibility level |
| Set due date | `set due date 'expr'` | Workflow-level due date expression |
| Set overview page | `set overview page Module.Page` | Workflow overview page |
| Set parameter | `set parameter $Var: Module.Entity` | Workflow context parameter |
| Set activity page | `set activity name page Module.Page` | Change user task page |
| Set activity description | `set activity name description 'text'` | Activity description |
| Set activity targeting | `set activity name targeting [users\|groups] microflow Module.MF` | Target user/group assignment |
| Set activity XPath | `set activity name targeting [users\|groups] xpath '[expr]'` | XPath targeting |
| Set activity due date | `set activity name due date 'expr'` | Activity-level due date |
| Insert activity | `insert after name call microflow Module.MF` | Insert after named activity |
| Drop activity | `drop activity name` | Remove activity by name |
| Replace activity | `replace activity name with activity` | Replace activity in-place |
| Insert outcome | `insert outcome 'name' on activity { body }` | Add outcome to user task/decision |
| Drop outcome | `drop outcome 'name' on activity` | Remove outcome |
| Insert path | `insert path on activity { body }` | Add path to parallel split |
| Drop path | `drop path 'name' on activity` | Remove parallel split path |
| Insert condition | `insert condition 'name' on activity { body }` | Add decision branch |
| Drop condition | `drop condition 'name' on activity` | Remove decision branch |
| Insert boundary event | `insert boundary event on activity interrupting timer ['expr'] { body }` | Add boundary timer |
| Drop boundary event | `drop boundary event on activity` | Remove boundary event |

**Activity references** can be identifiers (`ReviewOrder`) or string literals (`'Review the order'`). Use `@N` suffix for positional disambiguation when multiple activities share a name (e.g., `ACT_Process@2`).

**Multiple actions** can be combined in a single ALTER statement.

**Example:**
```sql
-- Set workflow-level properties
alter workflow Module.OrderApproval
  set display 'Updated Order Approval'
  set description 'Updated description';

-- Modify an activity
alter workflow Module.OrderApproval
  set activity ReviewOrder page Module.AlternatePage;

-- Insert and drop activities
alter workflow Module.OrderApproval
  insert after ReviewOrder call microflow Module.ACT_Escalate;
alter workflow Module.OrderApproval
  drop activity ACT_Notify@1;

-- Manage outcomes on a user task
alter workflow Module.OrderApproval
  insert outcome 'Escalate' on ReviewOrder {
    call microflow Module.ACT_Review;
  };
alter workflow Module.OrderApproval
  drop outcome 'Hold' on ReviewOrder;

-- Boundary events
alter workflow Module.OrderApproval
  insert boundary event on ReviewOrder interrupting timer 'addHours([%CurrentDateTime%], 2)' {
    call microflow Module.ACT_BoundaryHandler;
    jump to ReviewOrder;
  };
alter workflow Module.OrderApproval
  drop boundary event on ReviewOrder;
```

**Tip:** Run `describe workflow Module.Name` first to see activity names.

## Project Structure

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Structure overview | `show structure;` | Depth 2 (elements with signatures), user modules only |
| Module counts | `show structure depth 1;` | One line per module with element counts |
| Full types | `show structure depth 3;` | Typed attributes, named parameters |
| Filter by module | `show structure in ModuleName;` | Single module only |
| Include all modules | `show structure depth 1 all;` | Include system/marketplace modules |

## Navigation

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show navigation | `show navigation;` | Summary of all profiles |
| Show menu tree | `show navigation menu [Profile];` | Menu tree for profile or all |
| Show home pages | `show navigation homes;` | Home page assignments across profiles |
| Describe navigation | `describe navigation [Profile];` | Full MDL output (round-trippable) |
| Create/replace navigation | `create or replace navigation Profile ...;` | Full replacement of profile |

**Navigation Example:**
```sql
create or replace navigation Responsive
  home page MyModule.Home_Web
  home page MyModule.AdminHome for MyModule.Administrator
  login page Administration.Login
  not found page MyModule.Custom404
  menu (
    menu item 'Home' page MyModule.Home_Web;
    menu 'Admin' (
      menu item 'Users' page Administration.Account_Overview;
    );
  );
```

## Project Settings

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show settings | `show settings;` | Overview of all settings parts |
| Describe settings | `describe settings;` | Full MDL output (round-trippable) |
| Alter model settings | `alter settings model key = value;` | AfterStartupMicroflow, HashAlgorithm, JavaVersion, etc. |
| Alter configuration | `alter settings configuration 'Name' key = value;` | DatabaseType, DatabaseUrl, HttpPortNumber, etc. |
| Alter constant | `alter settings constant 'Name' value 'val' in configuration 'cfg';` | Override constant per configuration |
| Drop constant override | `alter settings drop constant 'Name' in configuration 'cfg';` | Reset to default value |
| Create configuration | `create configuration 'Name' [key = value, ...];` | New server configuration |
| Drop configuration | `drop configuration 'Name';` | Remove a configuration |
| Alter language | `alter settings LANGUAGE key = value;` | DefaultLanguageCode |
| Alter workflows | `alter settings workflows key = value;` | UserEntity, DefaultTaskParallelism |

## Business Events

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show services | `show business events;` | List all business event services |
| Show in module | `show business events in module;` | Filter by module |
| Describe service | `describe business event service Module.Name;` | Full MDL output |
| Create service | `create business event service Module.Name (...) { message ... };` | See help topic for full syntax |
| Drop service | `drop business event service Module.Name;` | Delete a service |

## Agents

AI agent document types (Model, Knowledge Base, Consumed MCP Service, Agent) require
the `AgentEditorCommons` marketplace module and Mendix 11.9+.

**Model**

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List models | `list models [in module];` | Also `show models` |
| Describe model | `describe model Module.Name;` | Full MDL output |
| Create model | `create model Module.Name (Provider: MxCloudGenAI, key: Module.Const);` | Key must be a String constant |
| Drop model | `drop model Module.Name;` | |

**Knowledge Base**

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List knowledge bases | `list knowledge bases [in module];` | Also `show knowledge bases` |
| Describe knowledge base | `describe knowledge base Module.Name;` | Full MDL output |
| Create knowledge base | `create knowledge base Module.Name (Provider: MxCloudGenAI, key: Module.Const);` | Key must be a String constant |
| Drop knowledge base | `drop knowledge base Module.Name;` | |

**Consumed MCP Service**

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List MCP services | `list consumed mcp services [in module];` | Also `show consumed mcp services` |
| Describe MCP service | `describe consumed mcp service Module.Name;` | Full MDL output |
| Create MCP service | `create consumed mcp service Module.Name (ProtocolVersion: v2025_03_26, version: '1.0', ConnectionTimeoutSeconds: 30, documentation: 'text');` | |
| Drop MCP service | `drop consumed mcp service Module.Name;` | |

**Agent**

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List agents | `list agents [in module];` | Also `show agents` |
| Describe agent | `describe agent Module.Name;` | Full MDL output, re-executable |
| Create agent | See example below | Requires a Model document |
| Drop agent | `drop agent Module.Name;` | Drop agents before their Model/KB/MCP dependencies |

```sql
create agent Module.MyAgent (
  UsageType: task,
  model: Module.MyModel,
  MaxTokens: 4096,
  Temperature: 0.7,
  TopP: 0.9,
  ToolChoice: Auto,
  description: 'Agent description',
  variables: ("Language": EntityAttribute),
  SystemPrompt: $$You are a helpful assistant.
Respond in {{Language}}.$$,
  UserPrompt: 'Ask me anything.'
)
{
  mcp service Module.WebSearch {
    Enabled: true
  }

  knowledge base KBAlias {
    source: Module.ProductDocs,
    collection: 'product-docs',
    MaxResults: 5,
    description: 'Product documentation',
    Enabled: true
  }

  tool MyMicroflowTool {
    description: 'Fetch customer data',
    Enabled: true
  }
};
```

**Notes:**
- `variables: ("key": EntityAttribute)` binds entity attributes; `("key": string)` binds a plain string.
- Use `$$...$$` dollar-quoting for multi-line SystemPrompt/UserPrompt values.
- Drop agents before dropping their referenced Model, Knowledge Base, or MCP Service.
- Portal-populated metadata fields (`DisplayName`, `KeyName`, `KeyID`, `Environment`, `ResourceName`, `DeepLinkURL`) are managed by the portal and should not be set manually.

## Image Collections

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show collections | `show image collection [in module];` | List all or filter by module |
| Describe collection | `describe image collection Module.Name;` | Full MDL output with embedded images |
| Create collection | `create image collection Module.Name [export level 'Hidden'\|'Public'] [comment 'text'] [(image Name from file 'path', ...)];` | With or without images |
| Drop collection | `drop image collection Module.Name;` | Removes collection and all embedded images |

**Export levels:** `'Hidden'` (default, internal to module), `'Public'` (accessible from other modules).

## Consumed REST Services

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show clients | `show rest clients [in module];` | List all or filter by module |
| Describe client | `describe rest client Module.Name;` | Re-executable CREATE |
| Create client | See syntax below | Property-based `{}` syntax |
| Create or modify | `create or modify rest client ...` | Replaces existing |
| Drop client | `drop rest client Module.Name;` | |
| Import from OpenAPI | See OpenAPI import below | Auto-generate from spec |
| Preview OpenAPI | `describe contract operation from openapi 'path';` | Preview without writing |

```sql
create rest client Module.Api (
  BaseUrl: 'https://api.example.com',
  authentication: none
)
{
  operation GetItems {
    method: get,
    path: '/items/{id}',
    parameters: ($id: string),
    query: ($filter: string),
    headers: ('Accept' = 'application/json'),
    timeout: 30,
    response: json as $Result
  }

  operation CreateItem {
    method: post,
    path: '/items',
    headers: ('Content-Type' = 'application/json'),
    body: mapping Module.ItemRequest {
      name = Name,
      price = Price,
    },
    response: mapping Module.ItemResponse {
      Id = id,
      status = status,
    }
  }
};
```

**Body types:** `json from $var`, `template '...'`, `mapping entity { jsonField = attr, ... }`
**Response types:** `json as $var`, `string as $var`, `file as $var`, `status as $var`, `none`, `mapping entity { attr = jsonField, ... }`
**Authentication:** `none`, `basic (username: '...', password: '...')`

### OpenAPI Import

Generate a consumed REST service document directly from an OpenAPI 3.0 spec (JSON or YAML):

```sql
-- From a local file (relative to the .mpr file)
create or modify rest client CapitalModule.CapitalAPI (
  OpenAPI: 'specs/capital.json'
);

-- From a URL
create or modify rest client PetStoreModule.PetStoreAPI (
  OpenAPI: 'https://petstore3.swagger.io/api/v3/openapi.json'
);

-- Override the base URL from the spec (e.g. point at staging instead of prod)
create or modify rest client PetStoreModule.PetStoreStaging (
  OpenAPI: 'https://petstore3.swagger.io/api/v3/openapi.json',
  BaseUrl: 'https://staging.petstore.example.com/api/v3'
);

-- Preview without writing to the project
describe contract operation from openapi 'specs/capital.json';
```

Operations, path/query parameters, headers, request body, response type, resource groups (from OpenAPI `tags`), and Basic auth are all derived automatically. The spec is stored inside the REST client document for Studio Pro parity.

`BaseUrl` is optional. When omitted, the base URL is taken from `servers[0].url` in the spec. When provided, it overrides that value — useful when the spec points at production but you want to import against a different environment.

## Published REST Services

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show services | `show published rest services [in module];` | List all or filter by module |
| Describe service | `describe published rest service Module.Name;` | Re-executable CREATE statement |
| Create service | See below | |
| Create or replace | `create or replace published rest service ...` | Replaces existing service |
| Alter service | `alter published rest service Module.Name set path = '...', version = '...';` | SET supports Path, Version, ServiceName |
| Add resource | `alter published rest service Module.Name add resource 'name' { ... };` | Operation block uses CREATE syntax |
| Drop resource | `alter published rest service Module.Name drop resource 'name';` | |
| Drop service | `drop published rest service Module.Name;` | |
| Grant access | `grant access on published rest service Module.Name to Module.Role, ...;` | Adds module roles to AllowedRoles |
| Revoke access | `revoke access on published rest service Module.Name from Module.Role, ...;` | |

```sql
create published rest service Module.MyAPI (
  path: 'rest/api/v1',
  version: '1.0.0',
  ServiceName: 'My API',
  folder: 'Integration/REST'
)
{
  resource 'orders' {
    get '' microflow Module.GetAllOrders;
    get '{id}' microflow Module.GetOrderById;
    post '' microflow Module.CreateOrder;
    put '{id}' microflow Module.UpdateOrder;
    delete '{id}' microflow Module.DeleteOrder;
  }
  resource 'customers' {
    get '' microflow Module.GetAllCustomers;
  }
};
```

**Properties:** `path` (required), `version`, `ServiceName`, `folder`
**HTTP methods:** `get`, `post`, `put`, `delete`, `patch`
**Operation paths:** Empty string `''` for the root, `'{paramName}'` for path parameters. Do NOT start or end with `/`.
**Path parameters:** Must match a microflow parameter exactly (case-sensitive). E.g., `'{id}'` requires the microflow to have parameter `$id`.
**Operation modifiers:** `deprecated`, `import mapping Module.Name`, `export mapping Module.Name`, `commit Yes|No`

## Data Transformers

Requires Mendix 11.9+. Steps: `jslt`, `xslt`. Single-line: `jslt '...'`. Multi-line: `jslt $$ ... $$`.

| Statement | Syntax | Notes |
|-----------|--------|-------|
| List transformers | `list data transformers [in module];` | |
| Describe transformer | `describe data transformer Module.Name;` | Re-executable CREATE |
| Create transformer | See syntax below | |
| Drop transformer | `drop data transformer Module.Name;` | |

```sql
create data transformer Module.WeatherTransform
source json '{"latitude": 51.9, "current": {"temp": 12.8}}'
{
  jslt $$
{
  "lat": .latitude,
  "temp": .current.temp
}
  $$;
};
```

## JSON Structures

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show structures | `show json structures [in module];` | List all or filter by module |
| Describe structure | `describe json structure Module.Name;` | Re-executable CREATE OR REPLACE + element tree |
| Create structure | `create json structure Module.Name [comment 'text'] snippet '...json...';` | Element tree auto-built from snippet |
| Create (multi-line) | `create json structure Module.Name snippet $${ "key": "value" }$$;` | Dollar-quoted snippet for readability |
| Create or replace | `create or replace json structure Module.Name snippet '...';` | Idempotent — preferred for AI agents |
| Create with name map | `create json structure Module.Name snippet '...' CUSTOM NAME map ('jsonKey' as 'CustomName', ...);` | Override auto-generated ExposedNames |
| Drop structure | `drop json structure Module.Name;` | |

## Import Mappings

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show mappings | `show import mappings [in module];` | List all or filter by module |
| Describe mapping | `describe import mapping Module.Name;` | Re-executable CREATE statement |
| Create mapping | See below | Assignment syntax: `attr = jsonField` |
| Drop mapping | `drop import mapping Module.Name;` | |

```sql
create import mapping Module.IMM_Pet
  with json structure Module.JSON_Pet
{
  create Module.PetResponse {
    PetId = id key,
    Name = name,
    status = status
  }
};
```

**Object handling:** `create` (default), `find` (requires KEY), `find or create`

**Nested objects:** Use association path `Assoc/entity = jsonKey`:
```sql
create Module.OrderResponse_CustomerInfo/Module.CustomerInfo = customer {
  Email = email,
  Name = name
}
```

## Export Mappings

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show mappings | `show export mappings [in module];` | List all or filter by module |
| Describe mapping | `describe export mapping Module.Name;` | Re-executable CREATE statement |
| Create mapping | See below | Assignment syntax: `jsonField = attr` |
| Drop mapping | `drop export mapping Module.Name;` | |

```sql
create export mapping Module.EMM_Pet
  with json structure Module.JSON_Pet
  null values LeaveOutElement
{
  Module.PetResponse {
    id = PetId,
    name = Name,
    status = status
  }
};
```

**Nested objects:** Use association path `Assoc/entity as jsonKey`:
```sql
Module.OrderResponse_CustomerInfo/Module.CustomerInfo as customer {
  email = Email,
  name = Name
}
```

## Java Actions

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Show Java actions | `show java actions [in module];` | List all or filtered by module |
| Describe Java action | `describe java action Module.Name;` | Full MDL output with signature |
| Create Java action | `create java action Module.Name(params) returns type as $$ ... $$;` | Inline Java code |
| Create with type params | `create java action Module.Name(EntityType: entity <pEntity>, Obj: pEntity) ...;` | Generic type parameters |
| Create exposed action | `... exposed as 'caption' in 'Category' as $$ ... $$;` | Toolbox-visible in Studio Pro |
| Drop Java action | `drop java action Module.Name;` | Delete a Java action |
| Call from microflow | `$Result = call java action Module.Name(Param = value);` | Inside BEGIN...END |

**Parameter Types:** `string`, `integer`, `long`, `decimal`, `boolean`, `datetime`, `Module.Entity`, `list of Module.Entity`, `enum Module.EnumName`, `enumeration(Module.EnumName)`, `stringtemplate(sql)`, `stringtemplate(Oql)`, `entity <pEntity>` (type parameter declaration), bare `pEntity` (type parameter reference).

**Type Parameters** allow generic entity handling. `entity <pEntity>` declares the type parameter inline and becomes the entity type selector; bare `pEntity` parameters receive entity instances:
```sql
create java action Module.Validate(
  EntityType: entity <pEntity> not null,
  InputObject: pEntity not null
) returns boolean
exposed as 'Validate Entity' in 'Validation'
as $$
return InputObject != null;
$$;
```

## Pages

MDL uses explicit property declarations for pages:

| Element | Syntax | Example |
|---------|-----------|---------|
| Page properties | `(key: value, ...)` | `(title: 'Edit', layout: Atlas_Core.Atlas_Default)` |
| Page variables | `variables: { $name: type = 'expr' }` | `variables: { $show: boolean = 'true' }` |
| Widget name | Required after type | `textbox txtName (...)` |
| Attribute binding | `attribute: AttrName` | `textbox txt (label: 'Name', attribute: Name)` |
| Variable binding | `datasource: $Var` | `dataview dv (datasource: $Product) { ... }` |
| Action binding | `action: type` | `actionbutton btn (caption: 'Save', action: save_changes)` |
| Microflow action | `action: microflow Name(Param: val)` | `action: microflow Mod.ACT_Process(Order: $Order)` |
| Database source | `datasource: database entity` | `datagrid dg (datasource: database Module.Entity)` |
| Selection binding | `datasource: selection widget` | `dataview dv (datasource: selection galleryList)` |
| CSS class | `class: 'classes'` | `container c (class: 'card mx-spacing-top-large')` |
| Inline style | `style: 'css'` | `container c (style: 'padding: 16px;')` |
| Design properties | `designproperties: [...]` | `container c (designproperties: ['Spacing top': 'Large', 'full width': on])` |
| Width (pixels) | `width: integer` | `image img (width: 200)` |
| Height (pixels) | `height: integer` | `image img (height: 150)` |
| Page size | `PageSize: integer` | `datagrid dg (PageSize: 25)` |
| Pagination mode | `Pagination: mode` | `datagrid dg (Pagination: virtualScrolling)` |
| Paging position | `PagingPosition: pos` | `datagrid dg (PagingPosition: both)` |
| Paging buttons | `ShowPagingButtons: mode` | `datagrid dg (ShowPagingButtons: auto)` |

**DataGrid Column Properties:**

| Property | Values | Default | Example |
|----------|--------|---------|---------|
| `attribute` | attribute name | (required) | `attribute: Price` |
| `caption` | string | attribute name | `caption: 'Unit Price'` |
| `Alignment` | `left`, `center`, `right` | `left` | `Alignment: right` |
| `WrapText` | `true`, `false` | `false` | `WrapText: true` |
| `Sortable` | `true`, `false` | `true`/`false` | `Sortable: false` |
| `Resizable` | `true`, `false` | `true` | `Resizable: false` |
| `Draggable` | `true`, `false` | `true` | `Draggable: false` |
| `Hidable` | `yes`, `hidden`, `no` | `yes` | `Hidable: no` |
| `ColumnWidth` | `autofill`, `autoFit`, `manual` | `autofill` | `ColumnWidth: manual` |
| `Size` | integer (px) | `1` | `Size: 200` |
| `visible` | expression string | `true` | `visible: '$showColumn'` (page variable, not $currentObject) |
| `DynamicCellClass` | expression string | (empty) | `DynamicCellClass: 'if(...) then ... else ...'` |
| `tooltip` | text string | (empty) | `tooltip: 'Price in USD'` |

**Page Example:**
```sql
create page MyModule.Customer_Edit
(
  params: { $Customer: MyModule.Customer },
  title: 'Edit Customer',
  layout: Atlas_Core.PopupLayout
)
{
  dataview dvCustomer (datasource: $Customer) {
    textbox txtName (label: 'Name', attribute: Name)
    textbox txtEmail (label: 'Email', attribute: Email)
    combobox cbStatus (label: 'Status', attribute: status)

    footer footer1 {
      actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
      actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
    }
  }
}
```

**Widget Properties:**

| Property | Syntax | Notes |
|----------|--------|-------|
| DesktopWidth | `column col (desktopwidth: 8)` | 1-12 or AutoFill |
| TabletWidth | `column col (tabletwidth: 6)` | 1-12 or AutoFill (default: auto) |
| PhoneWidth | `column col (phonewidth: 12)` | 1-12 or AutoFill (default: auto) |
| Visible | `textbox txt (visible: [IsActive])` | Conditional visibility (XPath expression) |
| Editable | `textbox txt (editable: [status != 'Closed'])` | Conditional editability (XPath expression) |

**Supported Widgets:**
- Layout: `layoutgrid`, `row`, `column`, `container`, `customcontainer`
- Input: `textbox`, `textarea`, `checkbox`, `radiobuttons`, `datepicker`, `combobox`
- Display: `dynamictext`, `datagrid`, `gallery`, `listview`, `image`, `staticimage`, `dynamicimage`
- Actions: `actionbutton`, `linkbutton`, `navigationlist`
- Structure: `dataview`, `header`, `footer`, `controlbar`, `snippetcall`

## ALTER PAGE / ALTER SNIPPET

Modify an existing page or snippet's widget tree in-place without full `create or replace`. Works directly on the raw BSON tree, preserving unsupported widget types.

| Operation | Syntax | Notes |
|-----------|--------|-------|
| Set property | `set caption = 'New' on widgetName` | Single property on a widget |
| Set multiple | `set (caption = 'Save', buttonstyle = success) on btn` | Multiple properties at once |
| Page-level set | `set title = 'New title'` | No ON clause for page properties |
| Insert after | `insert after widgetName { widgets }` | Add widgets after target |
| Insert before | `insert before widgetName { widgets }` | Add widgets before target |
| Drop widgets | `drop widget name1, name2` | Remove widgets by name |
| Replace widget | `replace widgetName with { widgets }` | Replace widget subtree |
| Pluggable prop | `set 'showLabel' = false on cbStatus` | Quoted name for pluggable widgets |
| Set column prop | `set caption = 'New' on dgGrid.colName` | Dotted ref targets DataGrid column |
| Drop column | `drop widget dgGrid.colName` | Remove a DataGrid column |
| Insert column | `insert after dgGrid.colName { column ... }` | Add column to DataGrid |
| Add variable | `add variables $name: type = 'expr'` | Add a page variable |
| Drop variable | `drop variables $name` | Remove a page variable |
| Set layout | `set layout = Module.LayoutName` | Change page layout, auto-maps placeholders |
| Set layout + map | `set layout = Module.Layout map (Old as New)` | Explicit placeholder mapping |

**Supported SET properties:** Caption, Label, ButtonStyle, Class, Style, Editable, Visible, Name, Title (page-level), Layout (page-level), and quoted pluggable widget properties.

**Example:**
```sql
alter page Module.EditPage {
  set (caption = 'Save & Close', buttonstyle = success) on btnSave;
  drop widget txtUnused;
  insert after txtEmail {
    textbox txtPhone (label: 'Phone', attribute: Phone)
  }
};

alter snippet Module.NavMenu {
  set caption = 'Dashboard' on btnHome
};
```

**Tip:** Run `describe page Module.PageName` first to see widget names.

## Reserved Words and Quoted Identifiers

Most MDL keywords now work **unquoted** as entity names, attribute names, parameter names, and module names. Common words like `caption`, `check`, `content`, `format`, `index`, `label`, `range`, `select`, `source`, `status`, `text`, `title`, `type`, `value`, `item`, `version`, `production`, etc. are all valid without quoting.

Only structural MDL keywords require quoting: `create`, `delete`, `begin`, `end`, `return`, `entity`, `module`.

**Quoted identifiers** escape any reserved word (double-quotes or backticks):
```sql
describe entity "combobox"."CategoryTreeVE";
show entities in "combobox";
create persistent entity Module.VATRate ("create": datetime, Rate: decimal);
```

Both double-quote (ANSI SQL) and backtick (MySQL) styles are supported. You can mix quoted and unquoted parts: `"combobox".CategoryTreeVE`.

**Boolean attributes** auto-default to `false` when no `default` is specified.

**CALCULATED** marks an attribute as calculated (not stored). Use `calculated by Module.Microflow` to specify the calculation microflow. Calculated attributes derive their value from a microflow at runtime.

**ButtonStyle** supports all values: `primary`, `default`, `success`, `danger`, `warning`, `info`.

## External SQL Statements

Direct SQL query execution against external databases (PostgreSQL, Oracle, SQL Server). Credentials are isolated — DSN never appears in session output or logs.

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Connect | `sql connect <driver> '<dsn>' as <alias>;` | Drivers: `postgres`, `oracle`, `sqlserver` |
| Disconnect | `sql disconnect <alias>;` | Closes connection |
| List connections | `sql connections;` | Shows alias + driver only (no DSN) |
| Show tables | `sql <alias> show tables;` | Lists user tables |
| Show views | `sql <alias> show views;` | Lists user views |
| Show functions | `sql <alias> show FUNCTIONS;` | Lists functions and procedures |
| Describe table | `sql <alias> describe <table>;` | Shows columns, types, nullability |
| Query | `sql <alias> <any-sql>;` | Raw SQL passthrough |
| Import | `import from <alias> query '<sql>' into Module.Entity map (...) [link (...)] [batch n] [limit n];` | Insert external data into Mendix app DB |
| Generate connector | `sql <alias> generate connector into <module> [tables (...)] [views (...)] [exec];` | Generate Database Connector MDL from schema |

```sql
-- Connect to PostgreSQL
sql connect postgres 'postgres://user:pass@localhost:5432/mydb' as source;

-- Explore schema
sql source show tables;
sql source describe users;

-- Query data
sql source select * from users where active = true limit 10;

-- Import external data into Mendix app database
import from source query 'SELECT name, email FROM employees'
  into HRModule.Employee
  map (name as Name, email as Email);

-- Import with association linking
import from source query 'SELECT name, dept_name FROM employees'
  into HR.Employee
  map (name as Name)
  link (dept_name to Employee_Department on Name);

-- Generate Database Connector from schema
sql source generate connector into HRModule;
sql source generate connector into HRModule tables (employees, departments) exec;

-- Manage connections
sql connections;
sql disconnect source;
```

CLI subcommand: `mxcli sql --driver postgres --dsn '...' "select 1"` (see `mxcli syntax sql`). Supported drivers: `postgres` (pg, postgresql), `oracle` (ora), `sqlserver` (mssql).

## Catalog & Search

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Refresh catalog | `refresh catalog;` | Rebuild basic metadata tables |
| Refresh with refs | `refresh catalog full;` | Include cross-references and source |
| Show catalog tables | `show catalog tables;` | List available queryable tables |
| Query catalog | `select ... from CATALOG.<table> [where ...];` | SQL against project metadata |
| Show callers | `show callers of Module.Name;` | What calls this element |
| Show callees | `show callees of Module.Name;` | What this element calls |
| Show references | `show references of Module.Name;` | All references to/from |
| Show impact | `show impact of Module.Name;` | Impact analysis |
| Show context | `show context of Module.Name;` | Surrounding context |
| Full-text search | `search '<keyword>';` | Search across all strings and source |

Cross-reference commands require `refresh catalog full` to populate reference data.

## Connection & Session

| Statement | Syntax | Notes |
|-----------|--------|-------|
| Connect | `connect local '/path/to/app.mpr';` | Open a Mendix project |
| Disconnect | `disconnect;` | Close current project |
| Status | `status;` | Show connection info |
| Refresh | `refresh;` | Reload project from disk |
| Commit | `commit [message 'text'];` | Save changes to MPR |
| Set variable | `set key = value;` | Session variable (e.g., `output_format = 'json'`) |
| Exit | `EXIT;` | Close REPL session |

## CLI Commands

| Command | Syntax | Notes |
|---------|--------|-------|
| Interactive REPL | `mxcli` | Interactive MDL shell |
| Execute command | `mxcli -p app.mpr -c "show entities"` | Single command |
| JSON output | `mxcli -p app.mpr -c "show entities" --json` | JSON for any command |
| Execute script | `mxcli exec script.mdl -p app.mpr` | Script file |
| Check syntax | `mxcli check script.mdl` | Parse-only validation |
| Check references | `mxcli check script.mdl -p app.mpr --references` | With reference validation |
| Lint project | `mxcli lint -p app.mpr [--format json\|sarif]` | 14 built-in + 27 Starlark rules |
| Report | `mxcli report -p app.mpr [--format markdown\|json\|html]` | Best practices report |
| Test | `mxcli test tests/ -p app.mpr` | `.test.mdl` / `.test.md` files |
| Diff script | `mxcli diff -p app.mpr changes.mdl` | Compare script vs project |
| Diff local | `mxcli diff-local -p app.mpr --ref head` | Git diff for MPR v2 |
| OQL | `mxcli oql -p app.mpr "select ..."` | Query running Mendix runtime |
| External SQL | `mxcli sql --driver postgres --dsn '...' "select 1"` | Direct database query |
| Docker build | `mxcli docker build -p app.mpr` | Build with PAD patching |
| Docker check | `mxcli docker check -p app.mpr` | Validate with `mx check` |
| Diagnostics | `mxcli diag [--bundle]` | Session logs, version info |
| New project | `mxcli new <name> --version X.Y.Z` | Create project from scratch with all tooling |
| Init project | `mxcli init /path/to/project` | Add AI tooling to existing project |
| Setup mxcli | `mxcli setup mxcli [--os linux]` | Download platform-specific mxcli binary |
| LSP server | `mxcli lsp --stdio` | Language server for VS Code |

## ANTLR4 Parser Architecture

The MDL parser uses ANTLR4 for grammar definition, enabling cross-language grammar sharing (Go, TypeScript, Java, Python).

**Regenerating the parser** (after modifying `MDLLexer.g4` or `MDLParser.g4`):
```bash
# Option 1: use make from project root (recommended)
make grammar

# Option 2: run directly in grammar directory
cd mdl/grammar
antlr4 -Dlanguage=Go -package parser -o parser MDLLexer.g4 MDLParser.g4
```

**Parser pipeline:**
1. `MDLLexer.g4` + `MDLParser.g4` → Split ANTLR4 grammar (tokens + rules, case-insensitive keywords)
2. `parser/` → Generated lexer/parser code
3. `visitor/` → ANTLR listener builds AST from parse tree
4. `ast/` → Strongly-typed AST nodes
5. `executor/` → Executes AST against modelsdk-go API

**Key design decisions:**
- ANTLR4 chosen over parser combinators for cross-language grammar sharing
- Case-insensitive keywords using ANTLR fragment rules
- Listener pattern (not visitor) for building AST
- Type assertions required for accessing concrete ANTLR context types
