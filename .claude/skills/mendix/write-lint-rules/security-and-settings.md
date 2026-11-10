# Security and settings structs

The structs returned by the security and settings query functions of the
[write-lint-rules](SKILL.md) skill. They read the project itself through the
MPR reader, so they need no particular catalog depth.

### user_role
| Property | Type | Example |
|----------|------|---------|
| `name` | string | `"Administrator"` |
| `description` | string | The role's documentation in Project Security; `""` when none |
| `is_anonymous` | bool | True if this is the anonymous/guest role |
| `module_roles` | list of string | `["Sales.Admin", "HR.Viewer"]` |
| `check_security` | bool | Studio Pro's per-role "Check security" flag |
| `manage_all_roles` | bool | The role may hand out every user role |
| `manage_users_without_roles` | bool | The role may manage users that have no role |
| `manageable_roles` | list of string | The user roles this role may hand out, when not all |

### module_role
| Property | Type | Example |
|----------|------|---------|
| `name` | string | `"Sales.Admin"` — qualified module role name |
| `module_name` | string | `"Sales"` |
| `description` | string | Module role description |

### role_mapping
| Property | Type | Example |
|----------|------|---------|
| `user_role_name` | string | `"Administrator"` |
| `module_role_name` | string | `"Sales.Admin"` |
| `module_name` | string | `"Sales"` |

### reference
| Property | Type | Example |
|----------|------|---------|
| `source_type` | string | The document the edge comes FROM, upper-case: `"MICROFLOW"`, `"NANOFLOW"`, `"RULE"`, `"PAGE"`, `"SNIPPET"`, `"LAYOUT"`, `"ENTITY"`, `"ASSOCIATION"`, `"WORKFLOW"`, `"NAVIGATION"`, `"SCHEDULED_EVENT"`, `"PUBLISHED_REST_OPERATION"`, `"PUBLISHED_REST_SERVICE"`, `"PUBLISHED_ODATA_SERVICE"`, `"CONSUMED_ODATA_SERVICE"`, `"BUSINESS_EVENT_SERVICE"`, `"PROJECT_SETTINGS"`, `"IMPORT_MAPPING"`, `"EXPORT_MAPPING"`, `"MESSAGE_DEFINITION_COLLECTION"`, `"MESSAGE_DEFINITION"`, `"AGENT"`, `"AI_MODEL"`, `"KNOWLEDGE_BASE"`, `"PUBLISHED_MCP_SERVICE"` |
| `source_id` | string | Source UUID |
| `source_name` | string | `"Sales.ACT_Customer_Create"` |
| `target_type` | string | What it points AT, upper-case: `"ENTITY"`, `"ASSOCIATION"`, `"MICROFLOW"`, `"NANOFLOW"`, `"RULE"`, `"PAGE"`, `"SNIPPET"`, `"LAYOUT"`, `"WORKFLOW"`, `"WIDGET"`, `"JAVA_ACTION"`, `"JAVASCRIPT_ACTION"`, `"REST_OPERATION"`, `"REGULAR_EXPRESSION"`, `"ATTRIBUTE"`, `"ENUMERATION"`, `"ENUMERATION_VALUE"`, `"IMPORT_MAPPING"`, `"EXPORT_MAPPING"`, `"JSON_STRUCTURE"`, `"AGENT"`, `"AI_MODEL"`, `"KNOWLEDGE_BASE"`, `"CONSUMED_MCP_SERVICE"`, `"CONSTANT"`. `WIDGET`, `ATTRIBUTE`, `ENUMERATION`, `ENUMERATION_VALUE` and `JSON_STRUCTURE` are only ever targets; `SCHEDULED_EVENT`, `PROJECT_SETTINGS` and the service types only ever sources |
| `target_id` | string | Target UUID |
| `target_name` | string | `"Sales.Customer"`; three-part for an attribute or an enumeration value: `"Sales.Order.Total"`, `"Sales.OrderStatus.Open"` |
| `ref_kind` | string | How it references: `"call"`, `"create"`, `"retrieve"`, `"change"`, `"delete"`, `"commit"` (a commit action, or a create/change that commits — beside its `"create"`/`"change"` edge; a commit of a variable whose entity the flow cannot tell has no edge), `"show_page"`, `"snippet_call"` (a page, snippet or layout places a snippet), `"datasource"`, `"action"`, `"layout"`, `"parameter"` (a page, snippet or flow parameter's entity type), `"return"`, `"generalize"`, `"associate"`, `"home_page"`, `"login_page"`, `"menu_item"`, `"calculate"`, `"schedule"`, `"validate"`, `"settings"`, `"widget"`, `"sync"`, `"publish"` (a published REST operation, OData resource or action, a published MCP service's tool microflow, or a service's authentication microflow), `"event"` (an entity event or a business event handler), `"member"` (binds/reads/writes an attribute, navigates an association, or reaches the entity an association step arrives at), `"xpath"` (an XPath constraint names it, or reaches the entity through an association), `"type"` (typed as an enumeration; or an entity named as a type: an action's result variable, a list a flow creates, an entity-type argument to a Java or JavaScript action), `"value"` (an expression names an enumeration value; an ENUMERATION target when the value named does not exist), `"mapping"` (an import/export mapping maps the entity, or a message definition exposes it), `"split"` (a decision branches on an object being the entity), `"mapping_call"` (a flow applies an import or export mapping: an import/export action, or a REST or web service call's request or response mapping), `"structure"` (a mapping is built on a JSON structure), `"tool"` (an agent draws on a knowledge base or consumed MCP service; an agent's tool microflow, nanoflow or agent is a `"call"`, as is a microflow or agent document passed as a Java action parameter), `"model"` (an agent runs on an AI model), `"key"` (an AI model or knowledge base reads its key from a constant) — lower-case, unlike the types above. Attribute names used only through a variable in a free-text expression (`$Order/Total`) have no edge |
| `module_name` | string | Source module |

### project_security

Returned by `project_security()`. Returns `none` if no MPR reader is available.

| Property | Type | Description |
|----------|------|-------------|
| `security_level` | string | `"CheckNothing"` (Off), `"CheckFormsAndMicroflows"` (Prototype), `"CheckEverything"` (Production) |
| `enable_demo_users` | bool | Whether demo users are enabled |
| `enable_guest_access` | bool | Whether anonymous/guest access is enabled |
| `check_security` | bool | Whether security checking is active |
| `strict_mode` | bool | Strict security mode |
| `anonymous_user_role` | string | Name of the project's guest user role, the role anonymous users get. Read `enable_guest_access` too: the role name can stay set while guest access is off |
| `admin_user_name` | string | Name of the administrator account: `"MxAdmin"`. Its password is deliberately not exposed |
| `admin_user_role` | string | The administrator account's user role: `"Administrator"` |
| `password_policy` | struct | Nested password policy settings |

#### password_policy (nested in project_security)
| Property | Type | Description |
|----------|------|-------------|
| `min_length` | int | Minimum password length |
| `require_digit` | bool | Must contain a digit |
| `require_mixed_case` | bool | Must contain upper and lower case |
| `require_symbol` | bool | Must contain a symbol |

### language

Returned by `languages()`, one per language enabled in the project settings. A per-language check skips `strings()` rows whose `language` is not among these codes.

| Property | Type | Description |
|----------|------|-------------|
| `code` | string | Language code: `"en_US"`, `"nl_NL"` |
| `is_default` | bool | Whether this is the project's default language |
| `check_completeness` | bool | Whether Studio Pro checks this language's translations for completeness |

### project_settings

Returned by `project_settings()`. A setting the project does not store reads as `""`, `0` or `false`.

| Property | Type | Description |
|----------|------|-------------|
| `default_language` | string | Default language code: `"en_US"` |
| `use_oql_version_2` | bool | Runtime → *Use OQL version 2* |
| `java_version` | string | `"Java21"` |
| `hash_algorithm` | string | `"BCrypt"`, `"SSHA256"`, … |
| `default_time_zone` | string | `"Europe/Amsterdam"`; `""` when unset |
| `rounding_mode` | string | `"HalfUp"`, `"HalfEven"` |
| `decimal_scale` | int | Scale of Decimal attributes |
| `optimistic_locking` | bool | Runtime → *Enable optimistic locking* |
| `foreign_key_constraints` | bool | Runtime → *Use database foreign key constraints* |
| `web_client` | string | Web UI → client: `"Yes"` (React), `"No"` (classic), `"MigrationMode"` |
