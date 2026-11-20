# Catalog-table builtins: object properties

The structs returned by `modules()`, `folders()`, `annotations()`, `associations()`, `entity_event_handlers()`,
`navigation_menu_items()`, `jar_dependencies()`, `jar_files()`, `strings()`, `layouts()`,
`published_rest_operations()` and `widgets()`. The functions themselves are listed in
[SKILL.md](SKILL.md) under "Available Query Functions". Every builtin leaves out
System and Marketplace modules, except `navigation_menu_items()`, whose rows
belong to the project rather than to a module.

### module
Returned by `modules()`.

| Property | Type | Example |
|----------|------|---------|
| `id` | string | Module UUID |
| `name` | string | `"Sales"` |
| `domain_model_documentation` | string | The module's domain model's documentation (Studio Pro's Documentation pane with nothing selected on the canvas). A Mendix module has no documentation of its own; this is the module-level text an author can write. `""` when none |

### folder
Returned by `folders()`. One per folder, empty folders included (ako/mxcli#1436).

| Property | Type | Example |
|----------|------|---------|
| `id` | string | Folder UUID |
| `name` | string | `"Order"` |
| `module_name` | string | `"Sales"` |
| `path` | string | `"Objects/Order"` — spelled as a document's `folder`, so `f.path == d.folder` matches the documents in it. A name that itself holds `/` is not escaped |
| `parent_path` | string | `"Objects"`; `""` for a folder directly in the module |
| `depth` | int | `1` for a folder directly in the module; counted from the parents, not from the slashes in `path` |
| `document_count` | int | Documents directly in this folder, not in its subfolders |
| `folder_count` | int | Folders directly in this folder |

An empty folder has `document_count == 0 and folder_count == 0`. A folder
with `document_count == 0` alone may still hold subfolders.

### annotation
Returned by `annotations()`. One per domain model annotation (ako/mxcli#1300).

| Property | Type | Example |
|----------|------|---------|
| `id` | string | Annotation UUID |
| `module_name` | string | `"Sales"` — the module whose domain model holds it |
| `caption` | string | `"TODO split sales\nper region"` — the full text, newlines kept |
| `x` | int | `60` — left edge on the domain model canvas |
| `y` | int | `40` — top edge on the canvas |
| `width` | int | `400` |

A "no TODO notes" rule needs both this and the microflow annotations, which
are activities: `activities_for(flow)` with `activity_type == "Annotation"`.

### association
Returned by `associations()`.

| Property | Type | Example |
|----------|------|---------|
| `name` | string | `"OrderLine_Order"` |
| `qualified_name` | string | `"Sales.OrderLine_Order"` |
| `module_name` | string | `"Sales"` |
| `from_entity` | string | Qualified name of the FROM entity (the one that owns the reference): `"Sales.OrderLine"` |
| `to_entity` | string | Qualified name of the TO entity: `"Sales.Order"`; for a cross-module association, the other module's entity |
| `type` | string | `"Reference"` or `"ReferenceSet"` |
| `owner` | string | `"Default"` or `"Both"` |
| `storage_format` | string | `"Column"` or `"Table"` |
| `description` | string | Documentation text |
| `to_delete_behavior` | string | The raw Mendix value on the TO end (Mendix's `ChildDeleteBehavior`, the end MDL's `on delete` clause sets): `"DeleteMeButKeepReferences"` (the default), `"DeleteMeAndReferences"`, `"DeleteMeIfNoReferences"`. Mendix always stores a value, so *set explicitly* can only mean *not the default* |
| `from_delete_behavior` | string | The same for the FROM end (Mendix's `ParentDeleteBehavior`; Studio Pro only): `"DeleteMeButKeepReferences"`, `"DeleteMeAndReferences"`, `"DeleteMeIfNoReferences"` |
| `to_delete_error_message` | string | The message shown when a `"DeleteMeIfNoReferences"` delete on the TO end is refused; `""` otherwise |
| `from_delete_error_message` | string | The same for the FROM end |

### entity_event_handler
Returned by `entity_event_handlers()`.

| Property | Type | Example |
|----------|------|---------|
| `entity` | string | Qualified entity name: `"Sales.Order"` |
| `module_name` | string | `"Sales"` |
| `moment` | string | `"Before"` or `"After"` |
| `event` | string | `"Create"`, `"Commit"`, `"Delete"` or `"RollBack"` — Mendix's capital B; `"Rollback"` matches nothing |
| `microflow` | string | Qualified name of the handler microflow |
| `raise_error_on_false` | bool | A `"Before"` handler returning false aborts the event |
| `pass_event_object` | bool | The microflow receives the object |

### navigation_menu_item
Returned by `navigation_menu_items()`.

| Property | Type | Example |
|----------|------|---------|
| `profile` | string | Navigation profile: `"Responsive"`, `"Phone"`, `"Tablet"`, … |
| `item_path` | string | Position in the menu, dot-separated per level: `"0"`, `"0.2"` |
| `depth` | int | `0` for a top-level item |
| `caption` | string | `"Orders"` |
| `action_type` | string | `"PageAction"`, `"MicroflowAction"`, `"SignOutAction"`, `"OpenLinkAction"`, `"NoAction"`; any other action is its stored type, e.g. `"Forms$CallNanoflowClientAction"` |
| `target_page` | string | Qualified page name, for a `"PageAction"`; `""` otherwise |
| `target_microflow` | string | Qualified microflow name, for a `"MicroflowAction"`; `""` otherwise |

### jar_dependency
Returned by `jar_dependencies()`.

| Property | Type | Example |
|----------|------|---------|
| `module_name` | string | `"Sales"` |
| `group_id` | string | `"org.apache.commons"` |
| `artifact_id` | string | `"commons-lang3"` |
| `version` | string | `"3.14.0"` |
| `coordinate` | string | `"org.apache.commons:commons-lang3"` — group and artifact, without the version, so it identifies the library across versions |
| `is_included` | bool | Studio Pro's "Included" setting; False when the dependency is declared but not included |

### jar_file
Returned by `jar_files()` (ako/mxcli#1304). Read from the project folder at
each call: the jars are not in the `.mpr`, so the catalog does not hold them.

| Property | Type | Example |
|----------|------|---------|
| `folder` | string | `"userlib"` or `"vendorlib"` |
| `file_name` | string | `"commons-lang3-3.12.0.jar"` |
| `name` | string | `"commons-lang3"` — the file name up to the last `-` followed by a digit, so `log4j-1.2-api-2.17.1.jar` is `log4j-1.2-api` |
| `version` | string | `"3.12.0"`; `""` when the file name carries no version |
| `required_by` | list of string | `["MyFirstModule"]` — modules with a `<file_name>.<Module>.RequiredLib` marker beside the jar, as a Marketplace module ships one |

The same `name` in two versions is one library on the classpath twice:

```python
def check():
    seen = {}
    for j in jar_files():
        seen.setdefault(j.name, []).append(j)
    out = []
    for name, jars in seen.items():
        versions = sorted({j.version for j in jars})
        if len(versions) > 1:
            out.append(violation(message = "%s on the classpath in versions %s" % (name, ", ".join(versions)),
                                 location = location(module = "", document_type = "project", document_name = "")))
    return out
```

### catalog_string
Returned by `strings(language = None)`.

| Property | Type | Example |
|----------|------|---------|
| `qualified_name` | string | The document the text belongs to: `"Sales.Order_Overview"` |
| `object_type` | string | Catalog object type, upper-case: `"PAGE"`, `"SNIPPET"`, `"LAYOUT"`, `"MICROFLOW"`, `"NANOFLOW"`, `"WORKFLOW"`, `"ENUMERATION"`, `"MENU_DOCUMENT"`, … |
| `value` | string | The text itself |
| `context` | string | What the text is. Translatable text names its stored type and property: `"Forms$Page.Title"`, `"Forms$TabPage.Caption"`, `"Enumerations$EnumerationValue.Caption"`; the rest a lower-case label: `"documentation"` (of an entity, attribute, constant, page, … — `object_type` says which; a module's domain model documentation is `"domain_model_documentation"`), `"page_url"`, `"log_node"`, `"log_message"`, … |
| `language` | string | `"en_US"`; `""` for text that is not translatable (documentation, URLs) |
| `element_id` | string | UUID of the element carrying the text |
| `widget_id` | string | UUID of the widget around the text, matching `widgets().id`; `""` outside a widget (a page's own title). A tab page's caption names the tab page, which `widgets()` does not list. A button's caption row has `element_id` of its `Forms$ClientTemplate` and its confirmation rows that of a `Forms$ConfirmationInfo`, so collect a widget's texts in every language by `widget_id`, not `element_id` |
| `module_name` | string | `"Sales"` |

### layout
Returned by `layouts()`.

| Property | Type | Example |
|----------|------|---------|
| `name` | string | `"Atlas_Default"` |
| `qualified_name` | string | `"Atlas_Core.Atlas_Default"` |
| `module_name` | string | `"Atlas_Core"` |
| `folder` | string | Folder path within module |
| `layout_type` | string | `"Responsive"`, `"Phone"`, `"Tablet"`, `"Popup"`, `"ModalPopup"`, `"Default"`, `"Legacy"` |
| `platform` | string | `"Web"` or `"Native"` — the platform the layout (and every page on it) renders on. `layout_type` cannot tell: a native popup shares its value with a web one. React-client errors (CE0582) apply to `"Web"` only |
| `description` | string | Documentation text |
| `class_name` | string | The layout's own `Class` (Appearance), e.g. `"layout-atlas layout-atlas-responsive-default"`, else `""` (full catalog — auto-detected). Its widgets are `widgets()` rows with `container_type` `"LAYOUT"` |
| `style` | string | The layout's own inline `Style` (Appearance), else `""` (full catalog) |
| `dynamic_classes` | string | The layout's own `Dynamic classes` expression, else `""` (full catalog) |

### published_rest_operation
Returned by `published_rest_operations()`.

| Property | Type | Example |
|----------|------|---------|
| `service` | string | Qualified service name: `"Sales.OrderApi"` |
| `resource` | string | `"orders"` |
| `http_method` | string | As Mendix stores it, TitleCase — unlike `rest_operation`: `"Get"`, `"Post"`, `"Put"`, `"Patch"`, `"Delete"` |
| `path` | string | `"/{id}"` |
| `summary` | string | Operation summary |
| `microflow` | string | Qualified name of the microflow that implements the operation |
| `deprecated` | bool | Marked deprecated |
| `module_name` | string | `"Sales"` |

### workflow
Returned by `workflows()`.

| Property | Type | Example |
|----------|------|---------|
| `id` | string | Workflow ID |
| `qualified_name` | string | `"Sales.ApproveOrder"` |
| `name` | string | `"ApproveOrder"` |
| `module_name` | string | `"Sales"` |
| `description` | string | Documentation text |
| `parameter_entity` | string | Qualified name of the context entity: `"Sales.Order"` |
| `activity_count` | int | Activities in every flow of the workflow, boundary-event paths and event sub-processes included |
| `user_task_count` | int | User tasks and multi-user tasks |
| `microflow_call_count` | int | System tasks and call-microflow activities |
| `decision_count` | int | Decisions |

### workflow_activity
Returned by `workflow_activities()`, each workflow's activities in walk order: the main flow, each activity followed by the paths of its boundary events, then the event sub-processes. A slot that names nothing is `""`.

| Property | Type | Example |
|----------|------|---------|
| `id` | string | Activity ID |
| `workflow_qualified_name` | string | `"Sales.ApproveOrder"` |
| `module_name` | string | `"Sales"` |
| `name` | string | `"ReviewOrder"` |
| `caption` | string | `"Review the order"` |
| `activity_type` | string | `"UserTask"`, `"MultiUserTask"`, `"SystemTask"`, `"CallMicroflow"`, `"AIAgentTask"`, `"CallWorkflow"`, `"Decision"`, `"ParallelSplit"`, `"JumpTo"`, `"WaitForTimer"`, `"WaitForNotification"`, `"StartWorkflow"`, `"EndWorkflow"`, … |
| `microflow` | string | The microflow a `SystemTask`, `CallMicroflow` or `AIAgentTask` runs |
| `user_source_microflow` | string | A user task's targeting microflow (users or groups) |
| `on_created_microflow` | string | A user task's on-created microflow |
| `completion_microflow` | string | A multi-user task's completion microflow |
| `page` | string | A user task's page |
| `user_task_entity` | string | A user task's task entity |
| `called_workflow` | string | The workflow a `CallWorkflow` activity calls |

### widget
Returned by `widgets()` (full catalog — auto-detected).

| Property | Type | Example |
|----------|------|---------|
| `id` | string | Widget UUID |
| `name` | string | Widget name |
| `widget_type` | string | The widget's storage type, e.g. `"Forms$DataView"`, `"Forms$DivContainer"`, `"Forms$ActionButton"`; a pluggable widget's id, e.g. `"com.mendix.widget.web.datagrid.Datagrid"` |
| `container_id` | string | Container UUID |
| `container_qualified_name` | string | `"Sales.Customer_Overview"` |
| `container_type` | string | `"PAGE"`, `"SNIPPET"` or `"LAYOUT"`. A layout's widgets reach every page on it, so a styling rule that skips `"LAYOUT"` misses the widest-reaching cases; a page-only rule (a data source, a page parameter) should filter on `"PAGE"` |
| `module_name` | string | `"Sales"` |
| `entity_ref` | string | Referenced entity qualified name |
| `attribute_ref` | string | Referenced attribute path |
| `microflow_ref` | string | Action/datasource microflow qualified name (e.g. a microflow-datasource ListView), else `""`. **One name only**: a widget that names several — a data grid 2 with a microflow datasource *and* an on-click microflow — shows the alphabetically smallest. Every one is in `refs` (kind `action`), so ask `refs_to(name)` whether a microflow is used, never this column. The same holds for `entity_ref`, `nanoflow_ref` and `page_ref` |
| `nanoflow_ref` | string | Action/datasource nanoflow qualified name, else `""` |
| `page_ref` | string | The page the widget's action opens (show page, create object then open page), else `""` |
| `snippet_ref` | string | The snippet a snippet call places, else `""`. Its edge in `refs` is `snippet_call` to a `SNIPPET` |
| `parent_widget_id` | string | `id` of the nearest catalogued ancestor widget; `""` at the page or snippet root. Wrappers the catalog skips (the synthetic `conditionalVisibilityWidget…` container) and non-widget holders (layout grid rows and columns, tab pages, a pluggable widget's properties and object-list items) are transparent: a widget in a layout grid column or a data grid 2 column has the grid as its parent |
| `depth` | int | Number of catalogued ancestors: `0` at the page or snippet root. A list view **template** is a catalogued row of its own, so a widget inside one is two below the list view. Depth does **not** cross a snippet call: a snippet's widgets start at `0` in the snippet, whichever page calls it |
| `class_name` | string | The widget's `Class` (Appearance), e.g. `"card mx-2"`, else `""`. Named `class_name` because `class` is a Starlark keyword |
| `style` | string | The inline `Style` (Appearance), e.g. `"width:100%;"`, else `""` |
| `dynamic_classes` | string | The `Dynamic classes` expression (Appearance), else `""` |
| `action_type` | string | Stored type of the widget's primary action — the button's action, else a container's on-click action, else a list view's or image's click action: `"Forms$DeleteClientAction"`, `"Forms$MicroflowAction"`, `"Forms$CallNanoflowClientAction"`, `"Forms$FormAction"` (show page), `"Forms$SaveChangesClientAction"`, `"Forms$CancelChangesClientAction"`, `"Forms$ClosePageClientAction"`, `"Forms$CreateObjectClientAction"`, `"Forms$OpenLinkClientAction"`, `"Forms$NoAction"`; `""` for a widget with no action property. Pluggable widgets' actions (inside their property bag) are not read |
| `has_confirmation` | bool | The primary action asks for confirmation. Only a microflow, nanoflow or workflow call can; a delete action (`"Forms$DeleteClientAction"`) has no confirmation setting at all, so "a delete must confirm" is enforced as "no button uses the delete action directly — call a microflow or nanoflow with a confirmation" |
| `caption` | string | `"Delete sale"` — the widget's own caption in the project's default language (buttons and labels; an input's label and a dynamic text's content are not its caption); `""` when it has none. Other languages: `strings(language = …)` rows with `widget_id == w.id` |
| `tooltip` | string | The tooltip in the project's default language; `""` when none is set |

```python
# Inline styles, classes outside an allow-list, and direct delete buttons.
ALLOWED = ["btn-primary", "card", "mx-2"]
def check():
    out = []
    for w in widgets():
        loc = location(module=w.module_name, document_type=w.container_type.lower(),
                       document_name=w.container_qualified_name.split(".")[-1],
                       document_id=w.container_id)
        if w.style != "":
            out.append(violation(message="inline style on " + w.name, location=loc))
        for c in w.class_name.split(" "):
            if c != "" and c not in ALLOWED:
                out.append(violation(message="class '" + c + "' not allowed", location=loc))
        if w.action_type == "Forms$DeleteClientAction":
            out.append(violation(message=w.name + " deletes directly; call a microflow with a confirmation",
                                 location=loc))
    return out
```

### permission

Returned by `permissions()` (all types) or `permissions_for()` (entity-specific).

| Property | Type | Example |
|----------|------|---------|
| `module_role_name` | string | `"Sales.Admin"` — qualified, so it compares directly against `user_role.module_roles` |
| `element_type` | string | `"ENTITY"`, `"MICROFLOW"`, `"NANOFLOW"`, `"PAGE"`, `"ODATA_SERVICE"` (from `permissions()` only) |
| `element_name` | string | `"Sales.Customer"` |
| `module_name` | string | `"Sales"` |
| `entity_name` | string | `"Sales.Customer"` (from `permissions_for()` only) |
| `access_type` | string | `"CREATE"`, `"READ"`, `"WRITE"`, `"DELETE"` (entity), `"EXECUTE"` (microflow, nanoflow), `"VIEW"` (page), `"ACCESS"` (OData service), `"MEMBER_READ"`, `"MEMBER_WRITE"` |
| `member_name` | string | Attribute name (for MEMBER_READ/MEMBER_WRITE) |
| `xpath_constraint` | string | XPath constraint or empty |
| `is_constrained` | bool | True if XPath constraint is set |
| `default_member_access_rights` | string | The rule's "default rights for new members": `"None"`, `"ReadOnly"` or `"ReadWrite"`. Empty for non-entity permissions |
| `access_rule_id` | string | The entity access rule the row comes from: rows sharing it are one rule, so a rule can count an entity's access rules or list one rule's module roles exactly. Empty for a page, flow or service permission |
| `access_rule_index` | int | That access rule's position on the entity, from 0; `-1` when `access_rule_id` is empty |
