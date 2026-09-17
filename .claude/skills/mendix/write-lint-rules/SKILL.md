---
name: write-lint-rules
description: "Write custom Starlark lint rules in .claude/lint-rules/ that run beside the built-ins under `mxcli lint`. Use when a project convention should be enforced automatically."
---

# Writing Custom Starlark Lint Rules

Custom lint rules are written in Starlark (a Python-like language) and placed in `.claude/lint-rules/` as `.star` files. They run alongside the built-in rules when `mxcli lint -p app.mpr` is executed.

## Rule File Structure

Every `.star` file must define metadata constants and a `check()` function:

```python
RULE_ID = "CUSTOM001"          # unique identifier
RULE_NAME = "MyRule"           # Short display name
description = "What it checks" # One-line description
CATEGORY = "security"          # Category: naming, quality, design, security, etc.
SEVERITY = "warning"           # hint, info, warning, error

def check():
    violations = []
    # ... iterate data, find issues, append violations ...
    return violations
```

### Catalog data requirements (`refs_to`, `cycles`, …)

Some builtins need a deeper catalog than the default fast build:

- `refs_to` / `refs_from` need **`REFRESH CATALOG FULL`** (the `refs` table).
- The graph-analysis builtins (`cycles`, `module_dependencies`, `community_of`,
  `layer_of`, `centrality`, `god_nodes`, `integration_surface`) need
  **`REFRESH CATALOG COMMUNITIES`** (the `graph_*` tables).

You don't have to do anything: `mxcli lint` (and the `LINT` statement)
**auto-detect** these builtins in your rule's source and build the catalog at the
required depth automatically. If a helper hides the call from the source scan, or
you want to be explicit, declare it:

```python
REQUIRES = ["full"]          # or ["communities"] — raises the auto-detected depth
```

Without this, a rule that queries `refs`/`graph_*` under a fast build would
silently return empty results (issue #721).

## Available Query Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `entities()` | list of entity | All non-system entities |
| `microflows()` | list of microflow | All non-system microflows |
| `pages()` | list of page | All non-system pages |
| `enumerations()` | list of enumeration | All non-system enumerations |
| `constants()` | list of constant | All non-system constants |
| `widgets()` | list of widget | All non-system widgets |
| `snippets()` | list of snippet | All non-system snippets |
| `scheduled_events()` | list of scheduled_event | All non-system scheduled events (requires MPR reader) |
| `rest_clients()` | list of rest_client | Consumed REST service documents (excluding platform modules) |
| `rest_operations()` | list of rest_operation | Operations on consumed REST services, including their `timeout` |
| `attributes_for(entity_qualified_name)` | list of attribute | Attributes for a specific entity |
| `activities_for(microflow_qualified_name)` | list of activity | Activities for a microflow (requires FULL catalog) |
| `permissions()` | list of permission | All permissions across all element types |
| `permissions_for(entity_qualified_name)` | list of permission | Access rules for a specific entity |
| `refs_to(target_name)` | list of reference | Cross-references *to* a target |
| `refs_from(source_name)` | list of reference | Cross-references *from* a source (outbound) |
| `user_roles()` | list of user_role | User roles from project security |
| `module_roles()` | list of module_role | All module roles (deduplicated from role mappings) |
| `role_mappings()` | list of role_mapping | User role to module role assignments |
| `project_security()` | project_security or None | Project-level security settings (requires MPR reader) |
| `xpath_expressions()` | list of xpath_expression | All XPath constraint expressions in the catalog (access rules, retrieve actions, widgets) |

### Graph-analysis functions (architecture rules)

These expose the dependency-graph facts so you can enforce your **own**
architecture policy (layering, allowed module dependencies, no cycles, coupling
budgets). They require `refresh catalog communities` to have populated the graph
tables; otherwise they return empty/None (the rule degrades gracefully — it does
not fail). In a session, run `refresh catalog communities` before `lint`.

| Function | Returns | Description |
|----------|---------|-------------|
| `layer_of(asset)` | int or None | Topological layer sequence number (no opinion on ordering) |
| `community_of(asset)` | struct{id, label} or None | The asset's detected community (bounded context) |
| `cycles()` | list of struct{id, size, members} | Dependency cycles (SCCs > 1 node) |
| `module_dependencies()` | list of struct{source_module, target_module, ref_kind, edges} | Directed module→module edges |
| `centrality(asset)` | struct{in, out, total, pagerank, betweenness} or None | Centrality of an asset |
| `god_nodes(metric="degree"\|"pagerank"\|"betweenness", min=N)` | list of struct{asset, object_type, module_name, degree, pagerank, betweenness} | High-centrality assets above a threshold |
| `integration_surface()` | list of struct{source_community, target_community, ref_kind, edges, mechanism} | Cross-community contract edges (for app-splitting) |

Example — a team enforcing *its own* strict layering (mxcli ships no such rule):

```python
RULE_ID = "ARCH900"
RULE_NAME = "Layering"
DESCRIPTION = "A module may only depend on lower or equal layers"
CATEGORY = "architecture"
SEVERITY = "error"

def check():
    out = []
    for d in module_dependencies():
        if d.ref_kind in ("layout", "show_page"):  # ignore UI navigation
            continue
        ls, lt = layer_of(d.source_module + ".x"), layer_of(d.target_module + ".x")
        # (resolve a real asset per module in practice; shown simplified)
        if ls != None and lt != None and ls < lt:
            out.append(violation(message = "%s depends upward on %s" % (d.source_module, d.target_module)))
    return out
```

Another team bans a specific dependency:

```python
def check():
    return [violation(message = "Payments must not depend on Reporting")
            for d in module_dependencies()
            if d.source_module == "Payments" and d.target_module == "Reporting"]
```

## Object Properties

### entity
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Document UUID |
| `name` | string | `"Customer"` |
| `qualified_name` | string | `"Sales.Customer"` |
| `module_name` | string | `"Sales"` |
| `folder` | string | `"DomainModel"` — folder path within module |
| `entity_type` | string | `"persistent"`, `"NonPersistent"`, `"view"` |
| `description` | string | Documentation text |
| `generalization` | string | Parent entity qualified name |
| `attribute_count` | int | Number of attributes |
| `access_rule_count` | int | Number of access rules |
| `validation_rule_count` | int | Number of validation rules |
| `has_event_handlers` | bool | True if entity has event handlers |
| `is_external` | bool | True if entity is from an external service |

### microflow
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Document UUID |
| `name` | string | `"ACT_Customer_Create"` |
| `qualified_name` | string | `"Sales.ACT_Customer_Create"` |
| `module_name` | string | `"Sales"` |
| `folder` | string | `"microflows/Customer"` — folder path within module |
| `microflow_type` | string | `"microflow"` or `"nanoflow"` |
| `description` | string | Documentation text |
| `return_type` | string | Return type |
| `parameter_count` | int | Number of parameters |
| `activity_count` | int | Number of activities |
| `complexity` | int | McCabe cyclomatic complexity |

### page
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Document UUID |
| `name` | string | `"Customer_Overview"` |
| `qualified_name` | string | `"Sales.Customer_Overview"` |
| `module_name` | string | `"Sales"` |
| `folder` | string | `"pages/Customer"` — folder path within module |
| `title` | string | Page title |
| `url` | string | Page URL |
| `description` | string | Documentation text |
| `widget_count` | int | Number of widgets |

### enumeration
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Document UUID |
| `name` | string | `"OrderStatus"` |
| `qualified_name` | string | `"Sales.OrderStatus"` |
| `module_name` | string | `"Sales"` |
| `folder` | string | `"enumerations"` — folder path within module |
| `description` | string | Documentation text |
| `value_count` | int | Number of enum values |

### constant
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Document UUID |
| `name` | string | `"AppBaseUrl"` |
| `qualified_name` | string | `"MyModule.AppBaseUrl"` |
| `module_name` | string | `"MyModule"` |
| `folder` | string | `"constants"` — folder path within module |
| `description` | string | Documentation text |
| `default_value` | string | `"https://example.com"` |
| `exposed_to_client` | bool | `true` if constant is exposed to client |

### widget
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Widget UUID |
| `name` | string | Widget name |
| `widget_type` | string | `"dataview"`, `"listview"`, etc. |
| `container_id` | string | Container UUID |
| `container_qualified_name` | string | `"Sales.Customer_Overview"` |
| `container_type` | string | `"page"` or `"snippet"` |
| `module_name` | string | `"Sales"` |
| `entity_ref` | string | Referenced entity qualified name |
| `attribute_ref` | string | Referenced attribute path |
| `microflow_ref` | string | Action/datasource microflow qualified name (e.g. a microflow-datasource ListView), else `""` |
| `nanoflow_ref` | string | Action/datasource nanoflow qualified name, else `""` |

### snippet
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Document UUID |
| `name` | string | `"SNIPPET_CustomerCard"` |
| `qualified_name` | string | `"Sales.SNIPPET_CustomerCard"` |
| `module_name` | string | `"Sales"` |
| `folder` | string | `"snippets"` — folder path within module |
| `widget_count` | int | Number of widgets |

### scheduled_event
| Property | Type | Example |
|----------|------|---------|
| `name` | string | `"SE_NightlyCleanup"` |
| `qualified_name` | string | `"MyModule.SE_NightlyCleanup"` |
| `module_name` | string | `"MyModule"` |
| `microflow_name` | string | `"MyModule.MF_NightlyCleanup"` — resolved from catalog; raw UUID when catalog not built |
| `interval_seconds` | int | `86400` — `0` for unrecognised interval type |
| `enabled` | bool | `True` if the event is active |

### xpath_expression

Returned by `xpath_expressions()`. Each row represents one XPath constraint used in a retrieve action, access rule, or widget data source.

| Property | Type | Example |
|----------|------|---------|
| `id` | string | Row UUID |
| `document_type` | string | `"MICROFLOW"`, `"NANOFLOW"`, `"DOMAIN_MODEL"`, `"PAGE"`, `"SNIPPET"` |
| `document_id` | string | Owning document UUID |
| `document_qualified_name` | string | `"MyApp.GetActiveItems"` |
| `component_type` | string | `"RETRIEVE_ACTION"`, `"ACCESS_RULE"`, `"WIDGET"` |
| `component_id` | string | Component UUID |
| `component_name` | string | Activity/rule name (may be empty) |
| `xpath_expression` | string | Raw XPath string, may include outer `[ ]` |
| `target_entity` | string | Qualified name of entity being queried, e.g. `"MyApp.Order"` |
| `referenced_entities` | string | Comma-separated qualified names of entities referenced by the XPath |
| `is_parameterized` | bool | True when the XPath contains `$variable` references |
| `usage_type` | string | `"RETRIEVE"`, `"SECURITY"`, `"DATASOURCE"` |
| `module_name` | string | `"MyApp"` |

### expr

Returned by `parse_xpath(s)`. Every node has a `kind` field; additional fields depend on the kind.

| `kind` | Additional fields | Description |
|--------|-------------------|-------------|
| `"bin"` | `op` (string), `left` (expr), `right` (expr) | Binary operator: `=`, `!=`, `<`, `>`, `<=`, `>=`, `and`, `or` |
| `"unary"` | `op` (string), `operand` (expr) | Unary operator: `not`, `-` |
| `"call"` | `name` (string), `args` (list of expr) | Function call, e.g. `contains(…)`, `length(…)` |
| `"string"` | `value` (string) | String literal |
| `"number"` | `value` (string) | Numeric literal (kept as string to preserve precision) |
| `"bool"` | `value` (bool) | `true` or `false` |
| `"empty"` | — | Mendix `empty` keyword |
| `"variable"` | `name` (string) | `$ParameterName` |
| `"attr_path"` | `variable` (string), `path` (list of string) | `$Obj/Association/Attribute` |
| `"qname"` | `module` (string), `name` (string), `sub` (string) | Qualified name, e.g. `MyApp.Status.Active` |
| `"paren"` | `inner` (expr) | Parenthesised expression |
| `"if"` | `cond` (expr), `then` (expr), `else_` (expr) | If-then-else expression |
| `"constant"` | `qname` (string) | Mendix constant reference, e.g. `[%MyConst%]` |
| `"token"` | `token` (string), `arg` (string) | Mendix token expression, e.g. `[%CurrentUser%]` |
| `"recovered"` | `source` (string), `reason` (string) | Parse failure — node carries the raw source fragment |
| `"null"` | — | Nil / missing node |
| `"unknown"` | — | Unrecognised AST node type |

**Walking an expr tree:** check `node.kind` and recurse into child fields. Leaf kinds (no child nodes) are: `string`, `number`, `bool`, `empty`, `variable`, `qname`, `constant`, `token`, `recovered`, `null`, `unknown`.

Example — count `not(…)` calls in an XPath (using `parse_xpath`):

```python
def count_not(node):
    if node.kind in ("null", "unknown", "recovered", "string", "number",
                     "bool", "empty", "variable", "qname", "constant", "token"):
        return 0
    if node.kind == "call" and node.name == "not":
        return 1 + sum([count_not(a) for a in node.args])
    if node.kind == "call":
        return sum([count_not(a) for a in node.args])
    if node.kind == "bin":
        return count_not(node.left) + count_not(node.right)
    if node.kind == "unary":
        return count_not(node.operand)
    if node.kind == "paren":
        return count_not(node.inner)
    if node.kind == "if":
        return count_not(node.cond) + count_not(node.then) + count_not(node.else_)
    if node.kind == "attr_path":
        return 0
    return 0
```

### attribute
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Attribute UUID |
| `name` | string | `"Name"` |
| `entity_id` | string | Parent entity UUID |
| `entity_qualified_name` | string | `"Sales.Customer"` |
| `module_name` | string | `"Sales"` |
| `data_type` | string | `"string"`, `"integer"`, `"datetime"`, etc. |
| `length` | int | Field length (for strings) |
| `is_unique` | bool | Has unique constraint |
| `is_required` | bool | Is required |
| `default_value` | string | Default value |
| `is_calculated` | bool | True if attribute is calculated (virtual) |
| `description` | string | Documentation text |

### activity
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Activity UUID |
| `name` | string | Activity name |
| `caption` | string | Activity caption |
| `activity_type` | string | `"ActionActivity"`, `"ExclusiveSplit"`, `"LoopedActivity"`, etc. |
| `action_type` | string | `"CreateChangeAction"`, `"CommitAction"`, `"ShowFormAction"`, etc. |
| `microflow_id` | string | Parent microflow UUID |
| `microflow_qualified_name` | string | `"Sales.ACT_Customer_Create"` |
| `module_name` | string | `"Sales"` |
| `entity_ref` | string | Referenced entity qualified name |

### rest_client
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Document UUID |
| `name` | string | `"CustomerApi"` |
| `qualified_name` | string | `"Sales.CustomerApi"` |
| `module_name` | string | `"Sales"` |
| `folder` | string | Folder path within module |
| `base_url` | string | `"https://api.example.com/v1"` |
| `auth_scheme` | string | Authentication scheme, empty when none |
| `operation_count` | int | Number of operations on the service |
| `documentation` | string | Documentation text |

### rest_operation
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Operation UUID |
| `service_id` | string | Owning service UUID |
| `service_qualified_name` | string | `"Sales.CustomerApi"` |
| `name` | string | `"GetCustomer"` |
| `http_method` | string | `"GET"`, `"POST"`, … |
| `path` | string | `"/customers/{id}"` |
| `parameter_count` | int | Number of parameters |
| `has_body` | bool | True when the request carries a body |
| `response_type` | string | Response type name |
| `timeout` | int | Configured timeout in milliseconds; `0` when none is set |
| `module_name` | string | `"Sales"` |

### permission

Returned by `permissions()` (all types) or `permissions_for()` (entity-specific).

| Property | Type | Example |
|----------|------|---------|
| `module_role_name` | string | `"Admin"` |
| `element_type` | string | `"entity"`, `"microflow"`, `"page"`, `"ODATA_SERVICE"` (from `permissions()` only) |
| `element_name` | string | `"Sales.Customer"` |
| `module_name` | string | `"Sales"` |
| `entity_name` | string | `"Sales.Customer"` (from `permissions_for()` only) |
| `access_type` | string | `"create"`, `"read"`, `"write"`, `"delete"`, `"execute"`, `"view"`, `"access"`, `"MEMBER_READ"`, `"MEMBER_WRITE"` |
| `member_name` | string | Attribute name (for MEMBER_READ/MEMBER_WRITE) |
| `xpath_constraint` | string | XPath constraint or empty |
| `is_constrained` | bool | True if XPath constraint is set |

### user_role
| Property | Type | Example |
|----------|------|---------|
| `name` | string | `"Administrator"` |
| `is_anonymous` | bool | True if this is the anonymous/guest role |
| `module_roles` | list of string | `["Sales.Admin", "HR.Viewer"]` |

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
| `source_type` | string | `"microflow"`, `"page"`, etc. |
| `source_id` | string | Source UUID |
| `source_name` | string | `"Sales.ACT_Customer_Create"` |
| `target_type` | string | `"entity"`, `"microflow"`, etc. |
| `target_id` | string | Target UUID |
| `target_name` | string | `"Sales.Customer"` |
| `ref_kind` | string | Reference kind |
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
| `password_policy` | struct | Nested password policy settings |

#### password_policy (nested in project_security)
| Property | Type | Description |
|----------|------|-------------|
| `min_length` | int | Minimum password length |
| `require_digit` | bool | Must contain a digit |
| `require_mixed_case` | bool | Must contain upper and lower case |
| `require_symbol` | bool | Must contain a symbol |

## Helper Functions

| Function | Description |
|----------|-------------|
| `violation(message, location?, suggestion?)` | Create a violation to return |
| `location(module, document_type, document_name, document_id?)` | Create a location for a violation |
| `parse_xpath(s)` | Parse a raw XPath/expression string and return its AST as an `expr` struct tree. Outer `[ ]` are stripped automatically. Parse failures produce a `recovered` root node rather than raising. |
| `is_pascal_case(s)` | Returns True if string is PascalCase |
| `is_camel_case(s)` | Returns True if string is camelCase |
| `matches(s, pattern)` | Returns True if string matches regex |

## Common Patterns

### Pattern 1: Iterate entities and check a property

```python
RULE_ID = "SEC001"
RULE_NAME = "NoEntityAccessRules"
description = "persistent entities should have access rules"
CATEGORY = "security"
SEVERITY = "warning"

def check():
    violations = []
    for e in entities():
        if e.entity_type == "persistent" and not e.is_external and e.access_rule_count == 0:
            violations.append(violation(
                message="persistent entity '{}' has no access rules".format(e.qualified_name),
                location=location(module=e.module_name, document_type="entity", document_name=e.name),
                suggestion="grant <role> on {} (read *)".format(e.qualified_name),
            ))
    return violations
```

### Pattern 2: Check project-level security settings

```python
RULE_ID = "SEC002"
RULE_NAME = "WeakPasswordPolicy"
description = "password policy should require at least 8 characters"
CATEGORY = "security"
SEVERITY = "warning"

def check():
    sec = project_security()
    if sec == none:
        return []
    if sec.password_policy.min_length < 8:
        return [violation(
            message="password minimum length is {} (recommended: 8+)".format(sec.password_policy.min_length),
            location=location(module="", document_type="security", document_name="ProjectSecurity"),
            suggestion="alter project security password POLICY minimum length 8",
        )]
    return []
```

### Pattern 3: Check cross-references

```python
RULE_ID = "CUSTOM003"
RULE_NAME = "UnreferencedEntity"
description = "entities should be referenced by at least one microflow or page"
CATEGORY = "quality"
SEVERITY = "info"

def check():
    violations = []
    for e in entities():
        refs = refs_to(e.qualified_name)
        if len(refs) == 0:
            violations.append(violation(
                message="entity '{}' is not referenced anywhere".format(e.qualified_name),
                location=location(module=e.module_name, document_type="entity", document_name=e.name),
            ))
    return violations
```

### Pattern 4: Check attributes of entities

```python
RULE_ID = "CUSTOM004"
RULE_NAME = "RequiredStringLength"
description = "string attributes should have a length limit"
CATEGORY = "design"
SEVERITY = "warning"

def check():
    violations = []
    for e in entities():
        for attr in attributes_for(e.qualified_name):
            if attr.data_type == "string" and attr.length == 0:
                violations.append(violation(
                    message="string attribute '{}.{}' has unlimited length".format(e.name, attr.name),
                    location=location(module=e.module_name, document_type="entity", document_name=e.name),
                ))
    return violations
```

## Validation

Test your rule by running the linter:

```bash
mxcli lint -p app.mpr --list-rules   # Verify rule is loaded
mxcli lint -p app.mpr                 # run all rules including yours
```

If a `.star` file has syntax errors, a warning is printed and the rule is skipped.
