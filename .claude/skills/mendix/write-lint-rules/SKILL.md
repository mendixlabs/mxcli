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
DESCRIPTION = "What it checks" # One-line description
CATEGORY = "security"          # Category: naming, quality, design, security, etc.
SEVERITY = "warning"           # hint, info, warning, error

def check():
    violations = []
    # ... iterate data, find issues, append violations ...
    return violations
```

### Catalog data requirements (`widgets`, `refs_to`, `cycles`, …)

Some builtins need a deeper catalog than the default fast build:

- `refs_to`, `refs_from`, `widgets`, `xpath_expressions`, `activities_for`,
  `permissions`, `permissions_for`, `strings` and the `widget_count` field of a
  page or snippet need **`REFRESH CATALOG FULL`** — the `refs`, `widgets`,
  `xpath_expressions`, `activities`, `permissions` and `strings` tables and the
  widget counts are only written by a full build.
- The graph-analysis builtins (`cycles`, `module_dependencies`, `community_of`,
  `layer_of`, `centrality`, `god_nodes`, `integration_surface`) need
  **`REFRESH CATALOG COMMUNITIES`** (the `graph_*` tables).

You don't have to do anything: `mxcli lint` (and the `LINT` statement)
**auto-detect** these builtins (a call `name(`) and `.widget_count` in your
rule's source and build the catalog at the required depth automatically. If the
scan cannot see it — e.g. `getattr(p, "widget_count")`, or a builtin passed
around by name — or you want to be explicit, declare it:

```python
REQUIRES = ["full"]          # or ["communities"] — raises the auto-detected depth
```

Without this, a rule that reads a full-only table under a fast build gets
`[]` / `0` with no warning and reports a clean pass (issue #721).

## Available Query Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `entities()` | list of entity | All non-system entities |
| `microflows()` | list of microflow | All non-system microflows, nanoflows **and rules** — they share one catalog table. Name the document with `document_noun_title`, never a hardcoded `"Microflow"` |
| `pages()` | list of page | All non-system pages |
| `enumerations()` | list of enumeration | All non-system enumerations |
| `constants()` | list of constant | All non-system constants |
| `widgets()` | list of widget | All non-system page and snippet widgets (full catalog — auto-detected) |
| `snippets()` | list of snippet | All non-system snippets |
| `scheduled_events()` | list of scheduled_event | All non-system scheduled events (requires MPR reader) |
| `queues()` | list of queue | All non-system task queues |
| `java_actions()` | list of java_action | All non-system, non-marketplace Java actions, each carrying its parameters |
| `database_connections()` | list of database_connection | All non-system external database connections |
| `documents()` | list of document | Every App Explorer document outside System and Marketplace modules, as one uniform projection with `folder` — for rules about where a document *lives* |
| `documentable_elements()` | list of documentable | Every element that can carry documentation, across all document types, with its `description` — for documentation sweeps. Leaves out microflows and Java actions; use `microflows()` / `java_actions()` for those |
| `navigation_targets()` | list of navigation_target | Every page a navigation profile routes to: profile home pages, role home pages and menu items. Login and not-found pages are excluded |
| `rest_clients()` | list of rest_client | Consumed REST service documents (excluding platform modules) |
| `rest_operations()` | list of rest_operation | Operations on consumed REST services, including their `timeout` |
| `attributes_for(entity_qualified_name)` | list of attribute | Attributes for a specific entity |
| `activities_for(microflow_qualified_name, nested = False)` | list of activity | Activities of a microflow, nanoflow or rule, in flow order (full catalog — auto-detected). By default only the top level: a loop is one activity and its body is left out. `nested = True` adds every object inside a loop, at any depth, right after its loop, with `parent_loop_id` and `loop_depth` set |
| `permissions()` | list of permission | All permissions across all element types (full catalog — auto-detected) |
| `permissions_for(entity_qualified_name)` | list of permission | Access rules for a specific entity (full catalog — auto-detected) |
| `refs_to(target_name)` | list of reference | Cross-references *to* a target (full catalog — auto-detected) |
| `refs_from(source_name)` | list of reference | Cross-references *from* a source (outbound) (full catalog — auto-detected) |
| `user_roles()` | list of user_role | User roles from project security |
| `module_roles()` | list of module_role | All module roles (deduplicated from role mappings) |
| `role_mappings()` | list of role_mapping | User role to module role assignments |
| `project_security()` | project_security or None | Project-level security settings (requires MPR reader) |
| `languages()` | list of language | The languages **enabled** in the project settings, in settings order (requires MPR reader; `[]` without one). Use it to scope per-language checks: `strings()` has a row for every stored translation, including languages the project never enabled |
| `xpath_expressions()` | list of xpath_expression | All XPath constraint expressions in the catalog (access rules, retrieve actions, widgets) (full catalog — auto-detected) |
| `modules()` | list of module | The user's modules (not System, not Marketplace), with their domain model's documentation |
| `associations()` | list of association | All non-system associations, same-module and cross-module, with the delete behaviour of both ends |
| `entity_event_handlers()` | list of entity_event_handler | Every before/after event handler on a non-system entity: which moment, which event, which microflow |
| `navigation_menu_items()` | list of navigation_menu_item | Every navigation menu item of every profile, at every depth. Navigation belongs to the project, so no module filter applies |
| `jar_dependencies()` | list of jar_dependency | Maven dependencies declared by non-system, non-marketplace modules |
| `strings(language = None)` | list of catalog_string | User-facing and documentary text, one row per text and language; pass `language` (`"nl_NL"`) to narrow. An untranslated language has **no row**. Needs a FULL catalog, which `mxcli lint` builds automatically for a rule that calls it |
| `layouts()` | list of layout | All non-system layouts |
| `published_rest_operations()` | list of published_rest_operation | Operations of published REST services, with the microflow behind each |

The fields of `module`, `association`, `entity_event_handler`,
`navigation_menu_item`, `jar_dependency`, `catalog_string`, `layout` and
`published_rest_operation` are in [catalog-tables.md](catalog-tables.md).

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
| `module_cycles()` | list of struct{id, size, members} | Module-level dependency cycles over every reference kind; `members` are module names. Use this, not `cycles()`, for "no circular module dependencies" — modules can reference each other through documents that form no asset-level cycle |
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

> **The example values below are the real ones — do not adapt their case or their
> spelling.** A filter on a value the catalog never emits is silent: the rule
> compiles, runs, matches nothing and reports a clean pass. Two traps in
> particular:
>
> - **Case is not cosmetic.** Document and element kinds are upper-case
>   (`"MICROFLOW"`, `"ENTITY"`, `"READ"`), attribute data types are TitleCase
>   (`"String"`, `"DateTime"`), and `ref_kind` is lower-case (`"call"`,
>   `"show_page"`). Guessing wrong matches zero rows.
> - **`action_type` is the SDK name, never Mendix's BSON storage name.** The
>   catalog reports `ShowPageAction` / `ClosePageAction` / `CreateObjectAction` /
>   `CommitObjectsAction`; the storage names `ShowFormAction`, `CloseFormAction`,
>   `CreateChangeAction` and `CommitAction` that appear in `.mpr` documents never
>   reach a rule. A rule that allow-lists the storage names flags every microflow
>   that opens a page — the inversion measured at 49% false positives in
>   mendixlabs/mxcli#1027. The one exception is **`widget.action_type`**, which
>   is the raw stored type of a page action (`"Forms$DeleteClientAction"`) —
>   page actions have no SDK-name mapping in the catalog.
>
> To check a value against your own project rather than trusting any list:
>
> ```bash
> sqlite3 .mxcli/catalog.db "SELECT DISTINCT ActionType FROM activities;"
> sqlite3 .mxcli/catalog.db "SELECT DISTINCT SourceType, TargetType, RefKind FROM refs;"
> ```
>
> Absence from your project means the construct is not used there; a value absent
> from the tables below is one the catalog never produces anywhere.

### entity
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Document UUID |
| `name` | string | `"Customer"` |
| `qualified_name` | string | `"Sales.Customer"` |
| `module_name` | string | `"Sales"` |
| `folder` | string | `"DomainModel"` — folder path within module |
| `entity_type` | string | exactly `"Persistent"`, `"NonPersistent"` or `"View"` — any other spelling matches nothing and the rule silently reports nothing |
| `description` | string | Documentation text |
| `generalization` | string | Parent entity qualified name |
| `attribute_count` | int | Number of attributes |
| `access_rule_count` | int | Number of access rules |
| `validation_rule_count` | int | Number of validation rules |
| `has_event_handlers` | bool | True if entity has event handlers |
| `is_external` | bool | True if entity is from an external service |
| `has_created_date` | bool | True if the entity stores `createdDate` (an audit member, not counted in `attribute_count`) |
| `has_changed_date` | bool | True if the entity stores `changedDate` |
| `has_owner` | bool | True if the entity stores `owner` |
| `has_changed_by` | bool | True if the entity stores `changedBy` |

### microflow
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Document UUID |
| `name` | string | `"ACT_Customer_Create"` |
| `qualified_name` | string | `"Sales.ACT_Customer_Create"` |
| `module_name` | string | `"Sales"` |
| `folder` | string | `"microflows/Customer"` — folder path within module |
| `microflow_type` | string | exactly `"MICROFLOW"`, `"NANOFLOW"` or `"RULE"` — upper-case, unlike `entity_type`. `microflows()` yields all three flavours, so a rule meant for microflows only must filter on `"MICROFLOW"` |
| `description` | string | Documentation text |
| `return_type` | string | Return type |
| `parameter_count` | int | Number of parameters |
| `activity_count` | int | Number of activities at the top level of the flow, excluding start/end events and merges. A loop counts as one; its body is not counted |
| `total_activity_count` | int | `activity_count` plus every activity inside a loop, at any depth — the size of the flow including loop bodies. Equal to `activity_count` for a flow without loops |
| `complexity` | int | McCabe cyclomatic complexity |
| `document_noun` | string | `"microflow"`, `"nanoflow"` or `"rule"` — for mid-sentence use in a message |
| `document_noun_title` | string | `"Microflow"`, `"Nanoflow"` or `"Rule"` — for `document_type=` and a message that opens with it |

### page
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Document UUID |
| `name` | string | `"Customer_Overview"` |
| `qualified_name` | string | `"Sales.Customer_Overview"` |
| `module_name` | string | `"Sales"` |
| `folder` | string | `"pages/Customer"` — folder path within module |
| `title` | string | Page title in the project's default language (else en_US, else the lowest-sorted non-empty language); `""` when the page has none |
| `url` | string | Page URL |
| `description` | string | Documentation text |
| `widget_count` | int | Number of widgets (full catalog — auto-detected) |

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

**widget** — the struct returned by `widgets()` — identity, references, tree
position (`parent_widget_id`, `depth`), appearance (`class_name`, `style`) and
primary action (`action_type`, `has_confirmation`) — is documented in
[catalog-tables.md](catalog-tables.md#widget), with an example rule.

### snippet
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Document UUID |
| `name` | string | `"SNIPPET_CustomerCard"` |
| `qualified_name` | string | `"Sales.SNIPPET_CustomerCard"` |
| `module_name` | string | `"Sales"` |
| `folder` | string | `"snippets"` — folder path within module |
| `widget_count` | int | Number of widgets (full catalog — auto-detected) |

### scheduled_event
| Property | Type | Example |
|----------|------|---------|
| `name` | string | `"SE_NightlyCleanup"` |
| `qualified_name` | string | `"MyModule.SE_NightlyCleanup"` |
| `module_name` | string | `"MyModule"` |
| `microflow_name` | string | `"MyModule.MF_NightlyCleanup"` — resolved from catalog; raw UUID when catalog not built |
| `interval_seconds` | int | `86400` — `0` for unrecognised interval type |
| `repeat` | string | Schedule variant: `"Minute"`, `"Hour"`, `"Day"`, `"Week"`, `"MonthDate"`, `"MonthWeekday"`, `"YearDate"` or `"YearWeekday"`; `""` when the event has no schedule |
| `on_overlap` | string | `"DelayNext"` or `"SkipNext"` — what happens when a run is still going at the next start |
| `time_zone` | string | Time zone the schedule is evaluated in |
| `enabled` | bool | `True` if the event is active |

### queue
| Property | Type | Example |
|----------|------|---------|
| `name` | string | `"ImportQueue"` |
| `qualified_name` | string | `"Sales.ImportQueue"` |
| `module_name` | string | `"Sales"` |
| `parallelism` | string | `"3"` — an **expression**, stored as a string; do not assume it parses as an integer |
| `cluster_wide` | bool | `True` if parallelism applies across the cluster rather than per node |

### java_action
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Document UUID |
| `name` | string | `"JA_ParseJson"` |
| `qualified_name` | string | `"Sales.JA_ParseJson"` |
| `module_name` | string | `"Sales"` |
| `folder` | string | Folder path within module |
| `documentation` | string | Documentation text |
| `description` | string | Same as `documentation`, so a rule sweeping mixed document kinds can read one field name |
| `export_level` | string | `"Hidden"` or `"API"` |
| `return_type` | string | Return type |
| `parameter_count` | int | Number of parameters |
| `parameters` | list of java_action_parameter | The action's parameters, in order |

#### java_action_parameter (nested in java_action)
| Property | Type | Example |
|----------|------|---------|
| `name` | string | `"InputString"` |
| `description` | string | Parameter documentation |
| `parameter_type` | string | Parameter type |
| `is_required` | bool | `True` if the parameter is required |

### database_connection
| Property | Type | Example |
|----------|------|---------|
| `id` | string | Document UUID |
| `name` | string | `"LegacyDB"` |
| `qualified_name` | string | `"Integration.LegacyDB"` |
| `module_name` | string | `"Integration"` |
| `folder` | string | Folder path within module |
| `database_type` | string | Database engine of the connection |
| `query_count` | int | Number of queries defined on the connection |

### document
Returned by `documents()`.

| Property | Type | Example |
|----------|------|---------|
| `kind` | string | Catalog object type, upper-case: `"MICROFLOW"`, `"PAGE"`, `"WORKFLOW"`, … |
| `name` | string | `"Customer_Overview"` |
| `qualified_name` | string | `"Sales.Customer_Overview"` |
| `module_name` | string | `"Sales"` |
| `folder` | string | Folder path within module; `""` means directly in the module root |

### documentable
Returned by `documentable_elements()`.

| Property | Type | Example |
|----------|------|---------|
| `kind` | string | Mendix term, TitleCase: `"Page"`, `"Enumeration"`, `"Workflow"`, … |
| `name` | string | `"OrderStatus"` |
| `qualified_name` | string | `"Sales.OrderStatus"` |
| `module_name` | string | `"Sales"` |
| `description` | string | Documentation text, whichever of the element's Documentation/Description properties holds it |

### navigation_target
Returned by `navigation_targets()`.

| Property | Type | Example |
|----------|------|---------|
| `profile` | string | Navigation profile: `"Responsive"`, `"Phone"`, `"Tablet"`, … |
| `kind` | string | `"home"`, `"role_home"` or `"menu"` |
| `role` | string | User role, for a `"role_home"` target; `""` otherwise |
| `caption` | string | Menu item caption, for a `"menu"` target; `""` otherwise |
| `page` | string | Qualified name of the target page |

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
| `data_type` | string | `"String"`, `"Integer"`, `"Long"`, `"Decimal"`, `"Boolean"`, `"DateTime"`, `"Date"`, `"Enumeration"`, `"AutoNumber"`, `"Binary"`, `"HashedString"` |
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
| `name` | string | The `action_type` for an action activity, otherwise the `activity_type` |
| `caption` | string | The stored caption: an activity's, a split's (`"Is amount big?"`), or an annotation's text. Empty for objects Mendix stores no caption for (start/end events, merges, loops). When `auto_generate_caption` is true this is Studio Pro's stored placeholder (typically `"Activity"`), not the caption Studio Pro shows |
| `auto_generate_caption` | bool | Action activity: whether Studio Pro generates the caption. False for other objects |
| `description` | string | The documentation of an action activity, split or loop |
| `activity_type` | string | `"ActionActivity"`, `"ExclusiveSplit"`, `"ExclusiveMerge"`, `"LoopedActivity"`, `"InheritanceSplit"`, `"StartEvent"`, `"EndEvent"`, `"Annotation"` |
| `action_type` | string | The action inside an `ActionActivity`: `"CreateObjectAction"`, `"ChangeObjectAction"`, `"CommitObjectsAction"`, `"DeleteObjectAction"`, `"RetrieveAction"`, `"MicroflowCallAction"`, `"NanoflowCallAction"`, `"ShowPageAction"`, `"ClosePageAction"`, `"LogMessageAction"`, `"JavaActionCallAction"`, `"JavaScriptActionCallAction"`, `"RestCallAction"`, `"WebServiceCallAction"`. Empty for an activity that is not an action |
| `microflow_id` | string | Parent microflow UUID |
| `microflow_qualified_name` | string | `"Sales.ACT_Customer_Create"` |
| `module_name` | string | `"Sales"` |
| `entity_ref` | string | Entity qualified name, for a create object, a database retrieve, and a delete — the entity of the deleted variable when the flow types it (a parameter, a create or retrieve output, a loop iterator); empty when it cannot |
| `service_ref` | string | Called service document (REST / web service / OData client); empty when the activity calls none |
| `action_ref` | string | Operation or action within that service; for a microflow, nanoflow, Java action or JavaScript action call, the called document, e.g. `"Sales.SUB_Process"` (`service_ref` then empty). Empty when the activity calls nothing |
| `queue_ref` | string | Microflow or Java action call run in a task queue: the queue, e.g. `"Sales.OrderQueue"`. The call runs asynchronously, outside the loop and transaction it appears in. Empty for a call that runs in place |
| `use_request_timeout` | bool | Call REST service or Call web service: whether "Use a timeout" is enabled. False for other action types |
| `timeout_expression` | string | Call REST service or Call web service: the timeout in seconds, stored as an expression, e.g. `"300"` |
| `parent_loop_id` | string | `id` of the loop the activity is inside; empty at the top level. Only set with `activities_for(…, nested = True)` |
| `loop_depth` | int | Number of loops around the activity: 0 at the top level, 1 directly inside a loop, 2 in a loop inside a loop |
| `condition_expression` | string | Exclusive split: the condition expression, e.g. `"$Order/Amount > 10"`. Empty for a rule-based split |
| `condition_rule` | string | Exclusive split calling a rule: the rule's qualified name, e.g. `"Sales.IsValidOrder"` |
| `error_handling_type` | string | The stored error handling of an action, loop or split: exactly `"Rollback"`, `"Custom"`, `"CustomWithoutRollBack"` (capital **B**), `"Continue"` or `"Abort"`. Empty for objects without error handling |
| `log_level` | string | Log message: exactly `"Trace"`, `"Debug"`, `"Info"`, `"Warning"`, `"Error"` or `"Critical"` |
| `log_node_expression` | string | Log message: the log node as stored, an expression — `"'MyNode'"` (a quoted string literal) or `"getKey(Sales.LogNodes.Orders)"` |
| `log_message` | string | Log message: the message template, e.g. `"Amount is {1}"` |
| `commit_type` | string | Create or change object: exactly `"Yes"`, `"YesWithoutEvents"` or `"No"`. Empty for other actions |
| `with_events` | bool | True for a commit action with events, and for a create or change object with `commit_type` `"Yes"` |
| `retrieve_source` | string | Retrieve: exactly `"database"` or `"association"`. For `"database"`, `entity_ref` is the retrieved entity |

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
| `element_type` | string | `"ENTITY"`, `"MICROFLOW"`, `"PAGE"`, `"ODATA_SERVICE"` (from `permissions()` only) |
| `element_name` | string | `"Sales.Customer"` |
| `module_name` | string | `"Sales"` |
| `entity_name` | string | `"Sales.Customer"` (from `permissions_for()` only) |
| `access_type` | string | `"CREATE"`, `"READ"`, `"WRITE"`, `"DELETE"` (entity), `"EXECUTE"` (microflow), `"VIEW"` (page), `"ACCESS"` (OData service), `"MEMBER_READ"`, `"MEMBER_WRITE"` |
| `member_name` | string | Attribute name (for MEMBER_READ/MEMBER_WRITE) |
| `xpath_constraint` | string | XPath constraint or empty |
| `is_constrained` | bool | True if XPath constraint is set |
| `default_member_access_rights` | string | The rule's "default rights for new members": `"None"`, `"ReadOnly"` or `"ReadWrite"`. Empty for non-entity permissions |

### user_role
| Property | Type | Example |
|----------|------|---------|
| `name` | string | `"Administrator"` |
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
| `source_type` | string | The document the edge comes FROM, upper-case: `"MICROFLOW"`, `"NANOFLOW"`, `"RULE"`, `"PAGE"`, `"SNIPPET"`, `"ENTITY"`, `"ASSOCIATION"`, `"WORKFLOW"`, `"NAVIGATION"`, `"SCHEDULED_EVENT"`, `"PUBLISHED_REST_OPERATION"`, `"PROJECT_SETTINGS"`, `"IMPORT_MAPPING"`, `"EXPORT_MAPPING"` |
| `source_id` | string | Source UUID |
| `source_name` | string | `"Sales.ACT_Customer_Create"` |
| `target_type` | string | What it points AT, upper-case: `"ENTITY"`, `"ASSOCIATION"`, `"MICROFLOW"`, `"NANOFLOW"`, `"RULE"`, `"PAGE"`, `"LAYOUT"`, `"WORKFLOW"`, `"WIDGET"`, `"JAVA_ACTION"`, `"JAVASCRIPT_ACTION"`, `"REST_OPERATION"`, `"REGULAR_EXPRESSION"`, `"ATTRIBUTE"`, `"ENUMERATION"`, `"ENUMERATION_VALUE"`. `LAYOUT`, `WIDGET`, `ATTRIBUTE`, `ENUMERATION` and `ENUMERATION_VALUE` are only ever targets; `SCHEDULED_EVENT` and `PROJECT_SETTINGS` only ever sources |
| `target_id` | string | Target UUID |
| `target_name` | string | `"Sales.Customer"`; three-part for an attribute or an enumeration value: `"Sales.Order.Total"`, `"Sales.OrderStatus.Open"` |
| `ref_kind` | string | How it references: `"call"`, `"create"`, `"retrieve"`, `"change"`, `"delete"`, `"commit"` (a commit action, or a create/change that commits — beside its `"create"`/`"change"` edge; a commit of a variable whose entity the flow cannot tell has no edge), `"show_page"`, `"datasource"`, `"action"`, `"layout"`, `"parameter"`, `"return"`, `"generalize"`, `"associate"`, `"home_page"`, `"login_page"`, `"menu_item"`, `"calculate"`, `"schedule"`, `"validate"`, `"settings"`, `"widget"`, `"sync"`, `"publish"`, `"event"`, `"member"` (binds/reads/writes an attribute or navigates an association), `"xpath"` (an XPath constraint names it), `"type"` (typed as an enumeration), `"value"` (an expression names an enumeration value), `"mapping"` (an import/export mapping maps the entity) — lower-case, unlike the types above. Attribute names used only through a variable in a free-text expression (`$Order/Total`) have no edge |
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

## Helper Functions

| Function | Description |
|----------|-------------|
| `violation(message, location?, suggestion?)` | Create a violation to return |
| `location(module, document_type, document_name, document_id?)` | Create a location for a violation |
| `parse_xpath(s)` | Parse a raw XPath/expression string and return its AST as an `expr` struct tree. Outer `[ ]` are stripped automatically. Parse failures produce a `recovered` root node rather than raising. |
| `is_pascal_case(s)` | Returns True if string is PascalCase |
| `is_camel_case(s)` | Returns True if string is camelCase |
| `matches(s, pattern)` | Returns True if string matches regex |
| `get_option(key, default?)` | The rule's option `key` from the `options:` block under its rule ID in `.claude/lint-config.yaml`, or `default` (`None` if omitted) when unset |
| `struct(**kwargs)` | Build an ad-hoc struct, e.g. `struct(name="x", count=1)`, to group values inside a rule |

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
        if e.entity_type == "Persistent" and not e.is_external and e.access_rule_count == 0:
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
            suggestion="alter app security password POLICY minimum length 8",
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
